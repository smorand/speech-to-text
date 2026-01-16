# syntax=docker/dockerfile:1

# Multi-stage build for small final image
# Stage 1: Build the Go binary
FROM golang:1.24-alpine AS builder

# Install git for fetching dependencies and ca-certificates for HTTPS
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go module files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations for size
# CGO_ENABLED=0 for static linking
# -ldflags="-s -w" strips debug info and DWARF tables
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /app/bin/speech-to-text \
    ./cmd/speech-to-text

# Stage 2: Minimal runtime image
FROM gcr.io/distroless/static-debian12:nonroot

# Copy CA certificates for HTTPS connections
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary
COPY --from=builder /app/bin/speech-to-text /speech-to-text

# Set working directory
WORKDIR /

# Expose port 8080 (Cloud Run default)
EXPOSE 8080

# Set environment variables
# Cloud Run sets PORT automatically, but we set defaults
ENV HOST=0.0.0.0
ENV PORT=8080

# Run as non-root user (distroless:nonroot already runs as uid 65532)
USER nonroot:nonroot

# Health check for local testing (Cloud Run uses HTTP probes)
# Note: distroless has no shell, so we rely on Cloud Run's health probes
HEALTHCHECK NONE

# Run the MCP server
# The "serve" command starts the HTTP server
ENTRYPOINT ["/speech-to-text"]
CMD ["serve"]
