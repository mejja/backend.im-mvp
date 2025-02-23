FROM golang:1.18-alpine
WORKDIR /app

# Install required packages: bash, curl, gettext (for envsubst)
RUN apk add --no-cache bash curl gettext

# Download kubectl (using command substitution)
RUN curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && \
    chmod +x kubectl && \
    mv kubectl /usr/local/bin/

# Copy go.mod and go.sum from the control folder in the project root.
COPY control/go.mod control/go.sum ./
RUN go mod download

# Copy the rest of the control code.
COPY control/ ./

# Build the binary.
RUN go build -o control-server .

# Copy additional directories required at runtime.
COPY control/scripts/ /scripts/
COPY ./templates/ /templates/

EXPOSE 8080
CMD ["./control-server"]
