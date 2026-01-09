# Webhooks Service - Multi-Architecture Dockerfile using Red Hat UBI9
# Supports: linux/amd64, linux/arm64
# Based on: ADR-027 (Multi-Architecture Build Strategy with Red Hat UBI)
# Per ADR-028-EXCEPTION-001: Using upstream Go builder mirrored to quay.io for ARM64 compatibility

# Build stage - Upstream Go 1.25 (ADR-028-EXCEPTION-001: ARM64 runtime fix)
# FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder
# REASON: ARM64 crash with taggedPointerPack bug
FROM quay.io/jordigilh/golang:1.25-bookworm AS builder
# Using ADR-028 compliant mirror for ARM64 compatibility

# Build arguments for multi-architecture support
ARG GOFLAGS=""
ARG GOOS=linux
ARG GOARCH=amd64

# Install build dependencies (upstream Debian image uses apt-get)
RUN apt-get update && \
	apt-get install -y git ca-certificates tzdata && \
	apt-get clean && \
	rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /workspace

# Copy go mod files
COPY go.mod go.sum ./

# Copy source code
COPY . .

# Build the Webhooks service binary
# Per ADR-028-EXCEPTION-001: Using upstream Go 1.25.5 for ARM64 compatibility
# -mod=mod: Automatically download dependencies during build (DD-BUILD-001)
# DD-TEST-007: Coverage build uses SIMPLE flags
# - Coverage: No -ldflags, -a, or -installsuffix (breaks coverage instrumentation)
# - Production: Keep all optimizations for size/performance
RUN if [ "${GOFLAGS}" = "-cover" ]; then \
	echo "🔬 Building with E2E coverage instrumentation (DD-TEST-007)..."; \
	echo "   Simple build (no -a, -installsuffix, -extldflags)"; \
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} GOFLAGS=${GOFLAGS} go build \
	-mod=mod \
	-o webhooks \
	./cmd/webhooks/main.go; \
	else \
	echo "🚀 Production build with optimizations..."; \
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
	-mod=mod \
	-ldflags='-w -s -extldflags "-static"' \
	-a -installsuffix cgo \
	-o webhooks \
	./cmd/webhooks/main.go; \
	fi

# Runtime stage - Red Hat UBI9 minimal runtime image
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Install runtime dependencies
RUN microdnf update -y && \
	microdnf install -y ca-certificates tzdata && \
	microdnf clean all

# Create non-root user for security
RUN useradd -r -u 1001 -g root webhooks-user

# Copy the binary from builder stage
COPY --from=builder /workspace/webhooks /usr/local/bin/webhooks

# Set proper permissions
RUN chmod +x /usr/local/bin/webhooks

# Switch to non-root user for security
USER webhooks-user

# Expose webhooks port (HTTPS only per DD-WEBHOOK-001)
EXPOSE 9443

# Health check using HTTPS endpoint
# Note: Webhook server provides /healthz and /readyz endpoints
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
	CMD ["/usr/bin/curl", "-f", "-k", "https://localhost:9443/healthz"] || exit 1

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/webhooks"]

# Default: no arguments (rely on environment variables or CLI flags)
# Configuration can be provided via:
#   1. Environment variables (recommended for Kubernetes)
#   2. CLI flags: --webhook-port 9443 --data-storage-url http://datastorage:8080
#   3. TLS certs mounted at /tmp/k8s-webhook-server/serving-certs
CMD []

# Red Hat UBI9 compatible metadata labels (REQUIRED per ADR-027)
LABEL name="kubernaut-webhooks" \
	vendor="Kubernaut" \
	version="0.1.0" \
	release="1" \
	summary="Kubernaut Webhooks Service - Kubernetes Admission Webhooks for SOC2 Attribution" \
	description="A microservice component of Kubernaut that provides Kubernetes admission webhooks for capturing authenticated user identity for operational decisions (SOC2 CC8.1 attribution). Handles WorkflowExecution block clearance, RemediationApprovalRequest approval/rejection, and NotificationRequest deletion attribution." \
	maintainer="jgil@redhat.com" \
	component="webhooks" \
	part-of="kubernaut" \
	io.k8s.description="Webhooks Service for SOC2 CC8.1 operator attribution" \
	io.k8s.display-name="Kubernaut Webhooks Service" \
	io.openshift.tags="kubernaut,webhooks,admission,authentication,soc2,audit,attribution,microservice"



