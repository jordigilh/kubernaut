# Multi-stage build for WorkflowExecution controller
# CRITICAL (Jan 9, 2026): ALL Red Hat UBI9 go-toolset versions crash on ARM64
# - go-toolset:1.24 (Go 1.24.6) → taggedPointerPack fatal error ❌
# - go-toolset:1.25 (Go 1.25.3) → taggedPointerPack fatal error ❌
# - golang:1.25-bookworm (Go 1.25.5) → Works ✅
#
# Root cause: Red Hat's ARM64 Go runtime has pointer tagging bug
# Workaround: Use upstream Go builder mirrored to quay.io (avoids Docker Hub rate limits)
# ADR-028 Compliance: Image mirrored to approved quay.io/jordigilh/* registry
# Runtime stage still uses UBI9 for full Red Hat compliance
# See: docs/handoff/WE_E2E_RUNTIME_CRASH_JAN09.md
FROM quay.io/jordigilh/golang:1.25-bookworm AS builder

# Install build dependencies
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

# Build the WorkflowExecution controller binary
# -mod=mod: Automatically download dependencies during build (per DD-BUILD-001)
# DD-TEST-007: Support E2E coverage instrumentation
# When GOFLAGS=-cover, use simple build (no aggressive flags that break coverage)
# Otherwise, use production build with all optimizations
ARG GOFLAGS=""
ARG GOOS=linux
ARG GOARCH=amd64

RUN if [ "${GOFLAGS}" = "-cover" ]; then \
	echo "🔬 Building with E2E coverage instrumentation (DD-TEST-007)..."; \
	echo "   Simple build (no -a, -installsuffix, -extldflags)"; \
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} GOFLAGS=${GOFLAGS} go build \
	-mod=mod \
	-o workflowexecution \
	./cmd/workflowexecution; \
	else \
	echo "🚀 Production build with optimizations..."; \
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
	-mod=mod \
	-ldflags='-w -s -extldflags "-static"' \
	-a -installsuffix cgo \
	-o workflowexecution \
	./cmd/workflowexecution; \
	fi

# Final stage - Red Hat UBI9 minimal runtime image
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Install runtime dependencies
RUN microdnf update -y && \
	microdnf install -y ca-certificates tzdata && \
	microdnf clean all

# Create non-root user for security
RUN useradd -r -u 1001 -g root workflowexecution-user

# Copy the binary from builder stage
COPY --from=builder /workspace/workflowexecution /usr/local/bin/workflowexecution

# Set proper permissions
RUN chmod +x /usr/local/bin/workflowexecution

# Switch to non-root user for security
USER workflowexecution-user

# Expose ports (HTTP, Metrics, Health)
EXPOSE 8080 9090 8081

# Health check using the binary
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
	CMD ["/usr/local/bin/workflowexecution", "--health-check"] || exit 1

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/workflowexecution"]

# Red Hat UBI9 compatible metadata labels
LABEL name="kubernaut-workflowexecution" \
	vendor="Kubernaut" \
	version="1.0.0" \
	release="1" \
	summary="Kubernaut WorkflowExecution Controller - Kubernetes CRD Controller" \
	description="A Kubernetes controller component of Kubernaut that manages WorkflowExecution CRDs, handles workflow execution lifecycle, integrates with Tekton Pipelines, and manages workflow state transitions with fault isolation capabilities." \
	maintainer="jgil@redhat.com" \
	component="workflowexecution-controller" \
	part-of="kubernaut" \
	io.k8s.description="Kubernaut WorkflowExecution Controller for Kubernetes workflow execution" \
	io.k8s.display-name="Kubernaut WorkflowExecution Controller" \
	io.openshift.tags="kubernaut,kubernetes,controller,workflow,execution,tekton"


