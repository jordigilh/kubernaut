# Multi-stage build for notification service using Red Hat UBI9 Go toolset
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder

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

# Copy source code
COPY --chown=1001:0 . .

# Build the notification service binary
# -mod=mod: Automatically download dependencies during build (per DD-BUILD-001)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
	-mod=mod \
	-ldflags='-w -s -extldflags "-static"' \
	-a -installsuffix cgo \
	-o notification-service \
	./cmd/notification-service

# Final stage - Red Hat UBI9 minimal runtime image
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Install runtime dependencies
RUN microdnf update -y && \
	microdnf install -y ca-certificates tzdata && \
	microdnf clean all

# Create non-root user for security
RUN useradd -r -u 1001 -g root notification-user

# Copy the binary from builder stage
COPY --from=builder /opt/app-root/src/notification-service /usr/local/bin/notification-service

# Set proper permissions
RUN chmod +x /usr/local/bin/notification-service

# Switch to non-root user for security
USER notification-user

# Expose ports (HTTP, Metrics, Health)
EXPOSE 8089 9099 8090

# Health check using the binary
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
	CMD ["/usr/local/bin/notification-service", "--health-check"] || exit 1

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/notification-service"]

# Red Hat UBI9 compatible metadata labels
LABEL name="kubernaut-notification-service" \
	vendor="Kubernaut" \
	version="1.0.0" \
	release="1" \
	summary="Kubernaut Notification Service - Multi-Channel Notifications Microservice" \
	description="A microservice component of Kubernaut that handles multi-channel notifications, notification templates, delivery tracking, and integration with Slack, Teams, Email, and PagerDuty with fault isolation and independent scaling capabilities." \
	maintainer="kubernaut-team@example.com" \
	component="notification-processor" \
	part-of="kubernaut" \
	io.k8s.description="Kubernaut Notification Service for multi-channel notifications" \
	io.k8s.display-name="Kubernaut Notification Service" \
	io.openshift.tags="kubernaut,notification,slack,teams,email,microservice"
