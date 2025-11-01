# Data Storage Service - Multi-Architecture Build
# Base: Red Hat UBI9 (ADR-027: Multi-Architecture Build Strategy)
# Architectures: linux/amd64, linux/arm64

# =============================================================================
# STAGE 1: BUILDER - Build the Go binary
# =============================================================================
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest AS builder

# Build arguments
ARG GOARCH
ARG GOOS=linux
ARG VERSION=v1.0.0

# Labels
LABEL name="kubernaut-data-storage-builder" \
	vendor="Kubernaut" \
	version="${VERSION}" \
	architecture="${GOARCH}" \
	summary="Data Storage Service Builder" \
	description="Multi-architecture builder for Kubernaut Data Storage Service"

# Install build dependencies
RUN microdnf install -y golang && microdnf clean all

# Set working directory
WORKDIR /workspace

# Copy go module files first (layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
# CGO_ENABLED=0: Static binary (no C dependencies)
# -ldflags="-w -s": Strip debug information (smaller binary)
RUN CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} \
	go build \
	-ldflags="-w -s -X main.Version=${VERSION} -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
	-o /data-storage \
	./cmd/data-storage

# Verify binary was built
RUN ls -lh /data-storage && file /data-storage

# =============================================================================
# STAGE 2: RUNTIME - Minimal runtime image
# =============================================================================
FROM registry.access.redhat.com/ubi9/ubi-micro:latest

# Build arguments (for labels)
ARG GOARCH
ARG VERSION=v1.0.0

# Labels (OCI standard + Red Hat specific)
LABEL name="kubernaut-data-storage" \
	vendor="Kubernaut" \
	version="${VERSION}" \
	architecture="${GOARCH}" \
	summary="Data Storage Service" \
	description="REST API Gateway for PostgreSQL database access in Kubernaut" \
	io.k8s.description="Data Storage Service provides REST API endpoints for querying incident and audit data" \
	io.k8s.display-name="Kubernaut Data Storage Service" \
	io.openshift.tags="kubernaut,data-storage,api-gateway" \
	maintainer="Kubernaut Team"

# Copy binary from builder
COPY --from=builder /data-storage /data-storage

# Create non-root user
# UID 1001 is standard for UBI images
RUN useradd -r -u 1001 -s /sbin/nologin datastorage-user

# Switch to non-root user
USER 1001

# Expose ports
# 8080: HTTP API
# 9090: Prometheus metrics
EXPOSE 8080 9090

# Health check (optional but recommended)
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
	CMD ["/data-storage", "healthcheck"] || exit 1

# Set entrypoint
ENTRYPOINT ["/data-storage"]

# Default command (can be overridden)
CMD ["-config", "/etc/datastorage/config.yaml"]


