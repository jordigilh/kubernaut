# Notification Controller - Multi-Architecture Dockerfile using Red Hat UBI9
# Supports: linux/amd64, linux/arm64
# Based on: ADR-027 (Multi-Architecture Build Strategy with Red Hat UBI)

# Build stage - Red Hat UBI9 Go 1.24 toolset
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 AS builder

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
COPY --chown=1001:0 api/ api/
COPY --chown=1001:0 cmd/notification/ cmd/notification/
COPY --chown=1001:0 internal/controller/notification/ internal/controller/notification/
COPY --chown=1001:0 pkg/notification/ pkg/notification/

# Build the notification controller binary
# CGO_ENABLED=0 for static linking (no C dependencies)
# GOOS=linux for Linux targets
# GOARCH will be set automatically by podman's --platform flag
RUN CGO_ENABLED=0 GOOS=linux go build \
	-ldflags='-w -s -extldflags "-static"' \
	-a -installsuffix cgo \
	-o manager \
	./cmd/notification/main.go

# Runtime stage - Red Hat UBI9 minimal runtime image
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Install runtime dependencies
RUN microdnf update -y && \
	microdnf install -y ca-certificates tzdata && \
	microdnf clean all

# Create non-root user for security
RUN useradd -r -u 1001 -g root notification-controller-user

# Copy the binary from builder stage
COPY --from=builder /opt/app-root/src/manager /usr/local/bin/manager

# Set proper permissions
RUN chmod +x /usr/local/bin/manager

# Switch to non-root user for security
USER notification-controller-user

# Expose ports (controller + health)
EXPOSE 8080 8081

# Health check using the built-in health endpoint
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
	CMD ["/usr/bin/curl", "-f", "http://localhost:8081/healthz"] || exit 1

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/manager"]

# Default: no arguments (configuration via environment variables or Kubernetes ConfigMaps)
# Do NOT copy config files into the image - use ConfigMaps for runtime configuration
CMD []

# Red Hat UBI9 compatible metadata labels (REQUIRED per ADR-027)
LABEL name="kubernaut-notification-controller" \
	vendor="Kubernaut" \
	version="1.1.0" \
	release="1" \
	summary="Kubernaut Notification Controller - CRD-based Notification Management" \
	description="A Kubernetes controller component of Kubernaut that manages NotificationRequest custom resources for delivering notifications to multiple channels (Console, Slack) with automatic retry, exponential backoff, and at-least-once delivery guarantees." \
	maintainer="jgil@redhat.com" \
	component="notification-controller" \
	part-of="kubernaut" \
	io.k8s.description="Notification Controller for Kubernetes-native notification delivery" \
	io.k8s.display-name="Kubernaut Notification Controller" \
	io.openshift.tags="kubernaut,notification,controller,crd,kubernetes,slack,console"

