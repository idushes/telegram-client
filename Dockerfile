FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o telegram-client .

# Create a minimal image
FROM alpine:latest

# Install required packages
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from the builder stage
COPY --from=builder /app/telegram-client .

# Expose the MCP server port
EXPOSE 8080

# Command to run the executable
CMD ["./telegram-client"] 