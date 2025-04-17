FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application with platform-specific flags
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o telegram-client .

# Create a minimal image for the same architecture
FROM alpine:latest

# Install required packages
RUN apk --no-cache add ca-certificates

# Set the working directory
WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/telegram-client .

# Create session directory
# This is used when ETCD_ENDPOINT is not provided
RUN mkdir -p /app/session && chmod 700 /app/session

# Expose the MCP server port
EXPOSE 8080

# Environment variables can be set at runtime:
# - MCP_SERVER_PORT: Port for the MCP server
# - PHONE: Phone number for Telegram authentication
# - APP_ID: Telegram API App ID
# - APP_HASH: Telegram API App Hash
# - ETCD_ENDPOINT: Optional ETCD endpoint for session storage

# Command to run the executable
CMD ["./telegram-client"] 