# SignalProcessing Controller Dockerfile
# Multi-stage build for minimal production image
#
# Build: docker build -f docker/signalprocessing.Dockerfile -t kubernaut-signalprocessing:latest .
# Run: docker run -p 9090:9090 -p 8081:8081 kubernaut-signalprocessing:latest

# Stage 1: Build
FROM golang:1.24-alpine AS builder

WORKDIR /workspace

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go.mod and go.sum first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o signalprocessing-controller ./cmd/signalprocessing

# Stage 2: Runtime
FROM alpine:3.19

WORKDIR /

# Install CA certificates for HTTPS calls
RUN apk add --no-cache ca-certificates

# Copy binary from builder
COPY --from=builder /workspace/signalprocessing-controller /signalprocessing-controller

# Create non-root user
RUN adduser -D -u 65532 nonroot
USER 65532:65532

# Expose ports
# 9090 - Prometheus metrics
# 8081 - Health probes (liveness/readiness)
EXPOSE 9090 8081

ENTRYPOINT ["/signalprocessing-controller"]

