#!/bin/bash
set -euo pipefail

# Usage: apply-template.sh <templatePath> <namespace> KEY=VALUE ...
if [ "$#" -lt 2 ]; then
  echo "Usage: $0 <templatePath> <namespace> KEY=VALUE ..."
  exit 1
fi

TEMPLATE_PATH="$1"
NAMESPACE="$2"
shift 2

# Verify the template file exists.
if [ ! -f "$TEMPLATE_PATH" ]; then
  echo "Error: Template file '$TEMPLATE_PATH' not found." >&2
  exit 1
fi

# Export substitution variables for envsubst.
for pair in "$@"; do
  KEY=$(echo "$pair" | cut -d'=' -f1)
  VALUE=$(echo "$pair" | cut -d'=' -f2-)
  export "$KEY"="$VALUE"
done

# Define temporary file for the processed template.
TMP_TEMPLATE="/tmp/template.yaml"

echo "Processing template '$TEMPLATE_PATH' for namespace '$NAMESPACE'..."
if ! envsubst < "$TEMPLATE_PATH" > "$TMP_TEMPLATE"; then
  echo "Error processing template with envsubst" >&2
  exit 1
fi

echo "Applying Kubernetes template in namespace '$NAMESPACE'..."
if ! kubectl apply -f "$TMP_TEMPLATE" -n "$NAMESPACE"; then
  echo "Error applying template with kubectl" >&2
  exit 1
fi

echo "Template applied successfully."
