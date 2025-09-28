# Multi-stage build for workflow orchestrator service using Red Hat UBI9 Go toolset
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

# Build the workflow orchestrator service binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
	-ldflags='-w -s -extldflags "-static"' \
	-a -installsuffix cgo \
	-o workflow-service \
	./cmd/workflow-service

# Final stage - Red Hat UBI9 minimal runtime image
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Install runtime dependencies
RUN microdnf update -y && \
	microdnf install -y ca-certificates tzdata && \
	microdnf clean all

# Create non-root user for security
RUN useradd -r -u 1001 -g root workflow-user

# Copy the binary from builder stage
COPY --from=builder /opt/app-root/src/workflow-service /usr/local/bin/workflow-service

# Set proper permissions
RUN chmod +x /usr/local/bin/workflow-service

# Switch to non-root user for security
USER workflow-user

# Expose ports (HTTP, Metrics, Health)
EXPOSE 8083 9093 8084

# Health check using the binary
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
	CMD ["/usr/local/bin/workflow-service", "--health-check"] || exit 1

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/workflow-service"]

# Red Hat UBI9 compatible metadata labels
LABEL name="kubernaut-workflow-service" \
	vendor="Kubernaut" \
	version="1.0.0" \
	release="1" \
	summary="Kubernaut Workflow Orchestrator Service - Workflow Execution Microservice" \
	description="A microservice component of Kubernaut that handles workflow execution, multi-step orchestration, dependency resolution, and state management with fault isolation and independent scaling capabilities." \
	maintainer="kubernaut-team@example.com" \
	component="workflow-orchestrator" \
	part-of="kubernaut" \
	io.k8s.description="Kubernaut Workflow Orchestrator Service for workflow execution" \
	io.k8s.display-name="Kubernaut Workflow Orchestrator Service" \
	io.openshift.tags="kubernaut,workflow,orchestration,execution,microservice"
