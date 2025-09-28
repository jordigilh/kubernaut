# Multi-stage build for webhook service using Red Hat UBI9 Go toolset
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

# Build the gateway service binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
	-ldflags='-w -s -extldflags "-static"' \
	-a -installsuffix cgo \
	-o gateway-service \
	./cmd/gateway-service

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
COPY --from=builder /opt/app-root/src/gateway-service /usr/local/bin/gateway-service

# Set proper permissions
RUN chmod +x /usr/local/bin/gateway-service

# Switch to non-root user for security
USER gateway-user

# Expose ports (HTTP, Metrics, Health)
EXPOSE 8080 9090 8081

# Health check using the binary
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
	CMD ["/usr/local/bin/gateway-service", "--health-check"] || exit 1

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/gateway-service"]

# Red Hat UBI9 compatible metadata labels
LABEL name="kubernaut-gateway-service" \
	vendor="Kubernaut" \
	version="1.0.0" \
	release="1" \
	summary="Kubernaut Gateway Service - HTTP Gateway, Webhook Processing & Security Microservice" \
	description="A microservice component of Kubernaut that handles HTTP gateway operations, webhook processing (Prometheus AlertManager, Grafana), authentication, authorization, rate limiting, and security enforcement with fault isolation and independent scaling capabilities." \
	maintainer="kubernaut-team@example.com" \
	component="gateway-processor" \
	part-of="kubernaut" \
	io.k8s.description="Kubernaut Gateway Service for HTTP gateway, webhook processing, and security" \
	io.k8s.display-name="Kubernaut Gateway Service" \
	io.openshift.tags="kubernaut,gateway,webhook,security,http,alertmanager,microservice"
