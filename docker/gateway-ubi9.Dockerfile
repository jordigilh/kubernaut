# Gateway Service - Multi-Architecture Dockerfile using Red Hat UBI9
# Supports: linux/amd64, linux/arm64
# Based on: ADR-027 (Multi-Architecture Build Strategy with Red Hat UBI)
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 AS builder

# Build arguments for multi-architecture support
ARG TARGETOS=linux
ARG TARGETARCH

# Switch to root for package installation
USER root

# Install additional build dependencies if needed
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

# Build the gateway service binary
# CGO_ENABLED=0 for static linking (no C dependencies)
# GOOS and GOARCH from build args for multi-architecture support
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
	-ldflags='-w -s -extldflags "-static"' \
	-a -installsuffix cgo \
	-o gateway \
	./cmd/gateway

# Final stage - Red Hat UBI9 minimal runtime image
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Install runtime dependencies
RUN microdnf update -y && \
	microdnf install -y ca-certificates tzdata && \
	microdnf clean all

# Create non-root user for security
RUN useradd -r -u 1001 -g root gateway-user

# Copy timezone data and CA certificates (already available in UBI9)
# UBI9 already includes these, but we ensure they're available

# Copy the binary from builder stage
COPY --from=builder /opt/app-root/src/gateway /usr/local/bin/gateway

# Set proper permissions
RUN chmod +x /usr/local/bin/gateway

# Switch to non-root user for security
USER gateway-user

# Expose ports (HTTP, Metrics, Health)
EXPOSE 8080 9090 8081

# Health check using HTTP endpoint
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
	CMD ["/usr/bin/curl", "-f", "http://localhost:8080/health"] || exit 1

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/gateway"]

# Red Hat UBI9 compatible metadata labels
LABEL name="kubernaut-gateway" \
	vendor="Kubernaut" \
	version="0.1.0" \
	release="1" \
	summary="Kubernaut Gateway Service - Signal Ingestion & Processing" \
	description="A microservice component of Kubernaut that handles signal ingestion from multiple sources (Prometheus AlertManager, Kubernetes Events), deduplication, storm detection, priority assignment, and CRD creation for the remediation pipeline. Multi-architecture support (amd64/arm64) per ADR-027." \
	maintainer="jgil@redhat.com" \
	component="gateway" \
	part-of="kubernaut" \
	io.k8s.description="Gateway Service for signal ingestion, deduplication, and CRD creation" \
	io.k8s.display-name="Kubernaut Gateway Service" \
	io.openshift.tags="kubernaut,gateway,webhook,signal,alertmanager,prometheus,kubernetes-events,microservice"
