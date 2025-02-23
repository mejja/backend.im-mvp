package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// DeploymentPayload represents the JSON payload from the client.
type DeploymentPayload struct {
	UserID     string `json:"userID"`
	CommitHash string `json:"commitHash"`
	RepoURL    string `json:"repoURL"`
	// Extend with additional fields if needed.
}

// Upgrader for WebSocket connections.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// SafeConn wraps a websocket connection with a mutex for safe concurrent writes.
type SafeConn struct {
	Conn  *websocket.Conn
	Mutex sync.Mutex
}

// WriteJSON safely writes JSON to the WebSocket connection.
func (s *SafeConn) WriteJSON(v interface{}) error {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	return s.Conn.WriteJSON(v)
}

// sendWebSocketMessage sends and logs a message back to the client.
func sendWebSocketMessage(sconn *SafeConn, event, message string) {
	response := map[string]string{
		"event":   event,
		"message": message,
	}
	// Log the message being sent
	log.Printf("Sending WebSocket message: %v", response)
	if err := sconn.WriteJSON(response); err != nil {
		log.Printf("Error sending websocket message: %v", err)
	}
}

// generateNamespace returns a unique namespace name.
func generateNamespace(userID, repoURL, commitHash string) string {
	hash := sha256.Sum256([]byte(repoURL))
	hashStr := hex.EncodeToString(hash[:])[:8]
	return fmt.Sprintf("%s-%s-%s", userID, hashStr, commitHash)
}

// runCommand executes a command with a given timeout and returns its output.
func runCommand(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// applyK8sTemplate applies a Kubernetes YAML template using a bash script.
func applyK8sTemplate(templatePath, namespace string, substitutions map[string]string) error {
	args := []string{templatePath, namespace}
	for key, value := range substitutions {
		args = append(args, fmt.Sprintf("%s=%s", key, value))
	}
	output, err := runCommand(30*time.Second, "/scripts/apply-template.sh", args...)
	if err != nil {
		log.Printf("Error applying template: %v\nOutput: %s", err, output)
	}
	return err
}

func generatePVCName(namespace string) string {
	return namespace
}

// monitorTestPod polls the status of the test pod until it is "Running" or "Succeeded", or times out.
// TODO: Replace with a robust implementation using client-go or similar.
func monitorTestPod(namespace, podName string) (bool, error) {
	timeout := time.After(2 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return false, fmt.Errorf("timeout waiting for pod %s in namespace %s", podName, namespace)
		case <-ticker.C:
			output, err := runCommand(10*time.Second, "kubectl", "get", "pod", podName, "-n", namespace, "-o", "jsonpath={.status.phase}")
			if err != nil {
				log.Printf("Error checking status for pod %s: %v", podName, err)
				continue
			}
			log.Printf("Pod %s status: %s", podName, output)
			if output == "Running" || output == "Succeeded" {
				return true, nil
			} else if output == "Failed" {
				return false, fmt.Errorf("pod %s in namespace %s has failed", podName, namespace)
			}
		}
	}
}

// cleanupTestPod deletes the test pod.
func cleanupTestPod(namespace, podName string) {
	output, err := runCommand(30*time.Second, "kubectl", "delete", "pod", podName, "-n", namespace)
	if err != nil {
		log.Printf("Error cleaning up pod %s in namespace %s: %v\nOutput: %s", podName, namespace, err, output)
	} else {
		log.Printf("Successfully cleaned up pod %s in namespace %s", podName, namespace)
	}
}

// generateEndpoint returns the production endpoint URL.
func generateEndpoint(namespace string) string {
	return fmt.Sprintf("https://%s.yourdomain.com", namespace)
}

// handleDeployment processes the payload and orchestrates the workflow.
func handleDeployment(sconn *SafeConn, payload DeploymentPayload) {
	namespace := generateNamespace(payload.UserID, payload.RepoURL, payload.CommitHash)
	log.Printf("Using namespace: %s", namespace)

	// Create namespace.
	if output, err := runCommand(30*time.Second, "kubectl", "create", "namespace", namespace); err != nil {
		sendWebSocketMessage(sconn, "deployment_error", fmt.Sprintf("Failed to create namespace: %v\nOutput: %s", err, output))
		return
	}

	// Deploy test pod.
	pvcName := generatePVCName(namespace)
	substitutions := map[string]string{
		"PVCName":   pvcName,
		"Namespace": namespace,
		"RepoURL":   payload.RepoURL,
	}
	if err := applyK8sTemplate("/templates/test-pod.yaml", namespace, substitutions); err != nil {
		sendWebSocketMessage(sconn, "deployment_error", "Failed to deploy test pod: "+err.Error())
		return
	}

	// Monitor test pod.
	passed, err := monitorTestPod(namespace, "test-app")
	if !passed || err != nil {
		sendWebSocketMessage(sconn, "test_failure", fmt.Sprintf("Tests failed: %v", err))
		return
	}

	// Deploy production pods.
	if err := applyK8sTemplate("/templates/prod-pod.yaml", namespace, map[string]string{"Namespace": namespace}); err != nil {
		sendWebSocketMessage(sconn, "deployment_error", "Failed to deploy production pods: "+err.Error())
		return
	}

	// Delay cleanup of the test pod (non-blocking).
	go func() {
		time.Sleep(60 * time.Second)
		cleanupTestPod(namespace, "test-app")
	}()

	// Generate endpoint and send success message.
	endpoint := generateEndpoint(namespace)
	sendWebSocketMessage(sconn, "deployment_success", fmt.Sprintf("Deployment successful! Your app is live at: %s", endpoint))
}

// wsHandler handles incoming WebSocket connections.
func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}
	defer conn.Close()

	sconn := &SafeConn{Conn: conn}
	for {
		var payload DeploymentPayload
		if err := conn.ReadJSON(&payload); err != nil {
			log.Printf("Error reading JSON: %v", err)
			break
		}
		log.Printf("Received payload: %+v", payload)
		go handleDeployment(sconn, payload)
	}
}

func main() {
	http.HandleFunc("/ws", wsHandler)
	log.Println("WebSocket server listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
