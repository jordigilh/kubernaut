# Data Storage Service - Multi-Architecture Dockerfile using Red Hat UBI9
# Supports: linux/amd64, linux/arm64
# Based on: ADR-027 (Multi-Architecture Build Strategy with Red Hat UBI)

# Build stage - Red Hat UBI9 Go 1.24 toolset
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 AS builder

# Auto-detect target architecture from --platform flag
# Podman/Docker automatically set TARGETARCH when --platform is specified
ARG TARGETARCH
ARG GOOS=linux
# Use TARGETARCH if set (multi-arch build), otherwise detect from runtime
ARG GOARCH=${TARGETARCH:-amd64}

# Switch to root for package installation
USER root

# Install build dependencies
RUN dnf update -y && \
	dnf install -y git ca-certificates tzdata && \
	dnf clean all

# Switch back to default user for security
USER 1001

# Set working directory
WORKDIR /opt/app-root/src

# Copy go mod files
COPY --chown=1001:0 go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY --chown=1001:0 . .

# Build the Data Storage service binary
# CGO_ENABLED=0 for static linking (no C dependencies)
# Uses pgx pure-Go PostgreSQL driver (not lib/pq which requires CGO)
# GOOS and GOARCH from build args for multi-architecture support
RUN CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
	-ldflags='-w -s -extldflags "-static"' \
	-a -installsuffix cgo \
	-o data-storage \
	./cmd/datastorage/main.go

# Runtime stage - Red Hat UBI9 minimal runtime image
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Install runtime dependencies
RUN microdnf update -y && \
	microdnf install -y ca-certificates tzdata && \
	microdnf clean all

# Create non-root user for security
RUN useradd -r -u 1001 -g root data-storage-user

# Copy the binary from builder stage
COPY --from=builder /opt/app-root/src/data-storage /usr/local/bin/data-storage

# Set proper permissions
RUN chmod +x /usr/local/bin/data-storage

# Switch to non-root user for security
USER data-storage-user

# Expose ports (HTTP + Metrics)
EXPOSE 8080 9090

# Health check using HTTP endpoint
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
	CMD ["/usr/bin/curl", "-f", "http://localhost:8080/health"] || exit 1

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/data-storage"]

# Default: no arguments (rely on environment variables or mounted ConfigMap)
# Configuration can be provided via:
#   1. Environment variables (recommended for Kubernetes)
#   2. ConfigMap mounted at /etc/data-storage/config.yaml
#   3. Command-line flag: --config /path/to/config.yaml
CMD []

# Red Hat UBI9 compatible metadata labels (REQUIRED per ADR-027)
LABEL name="kubernaut-data-storage" \
	vendor="Kubernaut" \
	version="0.1.0" \
	release="1" \
	summary="Kubernaut Data Storage Service - Audit Trail Persistence" \
	description="A microservice component of Kubernaut that provides persistent storage for remediation audit trails, dual-write to PostgreSQL and vector databases, with pgvector integration for semantic search capabilities." \
	maintainer="jgil@redhat.com" \
	component="data-storage" \
	part-of="kubernaut" \
	io.k8s.description="Data Storage Service for audit trail persistence and vector search" \
	io.k8s.display-name="Kubernaut Data Storage Service" \
	io.openshift.tags="kubernaut,data-storage,audit,postgres,pgvector,database,persistence,microservice"


