# Notification Controller Dockerfile
# Multi-stage build for minimal production image

# Stage 1: Build the notification controller binary
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make ca-certificates

# Set working directory
WORKDIR /workspace

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY api/ api/
COPY cmd/notification/ cmd/notification/
COPY internal/controller/notification/ internal/controller/notification/
COPY pkg/notification/ pkg/notification/

# Build the controller binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -a -installsuffix cgo \
    -ldflags="-w -s" \
    -o manager \
    cmd/notification/main.go

# Stage 2: Create minimal runtime image
FROM gcr.io/distroless/static:nonroot

# Set working directory
WORKDIR /

# Copy the binary from builder
COPY --from=builder /workspace/manager .

# Use nonroot user (uid 65532)
USER 65532:65532

# Expose ports
EXPOSE 8080 8081

# Set entrypoint
ENTRYPOINT ["/manager"]

