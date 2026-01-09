# Webhooks Service - Multi-Architecture Dockerfile using Red Hat UBI9
# Supports: linux/amd64, linux/arm64
# Based on: ADR-027 (Multi-Architecture Build Strategy with Red Hat UBI)

# Build stage - Red Hat UBI9 Go 1.25 toolset (latest stable)
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder

# Auto-detect target architecture from --platform flag
# Podman/Docker automatically set TARGETARCH when --platform is specified
ARG TARGETARCH
ARG GOOS=linux
# Use TARGETARCH if set (multi-arch build), otherwise detect from runtime
ARG GOARCH=${TARGETARCH:-amd64}
# Support coverage profiling for E2E tests (E2E_COVERAGE_COLLECTION.md)
ARG GOFLAGS=""

# Switch to root for package installation
USER root

# Install build dependencies (dnf update required for security compliance)
RUN dnf update -y && \
	dnf install -y git ca-certificates tzdata && \
	dnf clean all

# Switch back to default user for security
USER 1001

# Set working directory
WORKDIR /opt/app-root/src

# Copy go mod files
COPY --chown=1001:0 go.mod go.sum ./

# Copy source code
COPY --chown=1001:0 . .

# Build the Webhooks service binary
# CGO_ENABLED=0 for static linking (no C dependencies)
# GOOS and GOARCH from build args for multi-architecture support
# GOFLAGS can include -cover for E2E coverage profiling (E2E_COVERAGE_COLLECTION.md)
# -mod=mod: Automatically download dependencies during build (no separate go mod download step)
#
# DD-TEST-007: Coverage build uses SIMPLE flags (per SP team guidance)
# - Coverage: No -ldflags, -a, or -installsuffix (breaks coverage instrumentation)
# - Production: Keep all optimizations for size/performance
# NOTE: vendor/ excluded in .dockerignore, so we use -mod=mod
# Toolchain pinned to go1.25.3 in go.mod to match UBI9 go-toolset:1.25
RUN if [ "${GOFLAGS}" = "-cover" ]; then \
	echo "Building with coverage instrumentation (simple build per DD-TEST-007)..."; \
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} GOFLAGS=${GOFLAGS} go build \
	-mod=mod \
	-o webhooks \
	./cmd/webhooks/main.go; \
	else \
	echo "Building production binary (with symbol stripping)..."; \
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
COPY --from=builder /opt/app-root/src/webhooks /usr/local/bin/webhooks

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



