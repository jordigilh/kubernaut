# Multi-stage build for kubernetes executor service using Red Hat UBI9 Go toolset
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 AS builder

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

# Build the kubernetes executor service binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
	-ldflags='-w -s -extldflags "-static"' \
	-a -installsuffix cgo \
	-o executor-service \
	./cmd/executor-service

# Final stage - Red Hat UBI9 minimal runtime image
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Install runtime dependencies
RUN microdnf update -y && \
	microdnf install -y ca-certificates tzdata && \
	microdnf clean all

# Create non-root user for security
RUN useradd -r -u 1001 -g root executor-user

# Copy the binary from builder stage
COPY --from=builder /opt/app-root/src/executor-service /usr/local/bin/executor-service

# Set proper permissions
RUN chmod +x /usr/local/bin/executor-service

# Switch to non-root user for security
USER executor-user

# Expose ports (HTTP, Metrics, Health)
EXPOSE 8084 9094 8085

# Health check using the binary
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
	CMD ["/usr/local/bin/executor-service", "--health-check"] || exit 1

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/executor-service"]

# Red Hat UBI9 compatible metadata labels
LABEL name="kubernaut-executor-service" \
	vendor="Kubernaut" \
	version="1.0.0" \
	release="1" \
	summary="Kubernaut Kubernetes Executor Service - K8s Operations Microservice" \
	description="A microservice component of Kubernaut that handles Kubernetes operations, safety validation, multi-cluster management, and resource lifecycle management with fault isolation and independent scaling capabilities." \
	maintainer="kubernaut-team@example.com" \
	component="k8s-executor" \
	part-of="kubernaut" \
	io.k8s.description="Kubernaut Kubernetes Executor Service for K8s operations" \
	io.k8s.display-name="Kubernaut Kubernetes Executor Service" \
	io.openshift.tags="kubernaut,kubernetes,executor,operations,microservice"
