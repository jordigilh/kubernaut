# Multi-stage build for intelligence service using Red Hat UBI9 Go toolset
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

# Build the intelligence service binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
	-ldflags='-w -s -extldflags "-static"' \
	-a -installsuffix cgo \
	-o intelligence-service \
	./cmd/intelligence-service

# Final stage - Red Hat UBI9 minimal runtime image
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Install runtime dependencies
RUN microdnf update -y && \
	microdnf install -y ca-certificates tzdata && \
	microdnf clean all

# Create non-root user for security
RUN useradd -r -u 1001 -g root intelligence-user

# Copy the binary from builder stage
COPY --from=builder /opt/app-root/src/intelligence-service /usr/local/bin/intelligence-service

# Set proper permissions
RUN chmod +x /usr/local/bin/intelligence-service

# Switch to non-root user for security
USER intelligence-user

# Expose ports (HTTP, Metrics, Health)
EXPOSE 8086 9096 8087

# Health check using the binary
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
	CMD ["/usr/local/bin/intelligence-service", "--health-check"] || exit 1

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/intelligence-service"]

# Red Hat UBI9 compatible metadata labels
LABEL name="kubernaut-intelligence-service" \
	vendor="Kubernaut" \
	version="1.0.0" \
	release="1" \
	summary="Kubernaut Intelligence Service - Pattern Discovery Microservice" \
	description="A microservice component of Kubernaut that handles pattern discovery, ML analytics, clustering, anomaly detection, and trend analysis with fault isolation and independent scaling capabilities." \
	maintainer="kubernaut-team@example.com" \
	component="intelligence-processor" \
	part-of="kubernaut" \
	io.k8s.description="Kubernaut Intelligence Service for pattern discovery" \
	io.k8s.display-name="Kubernaut Intelligence Service" \
	io.openshift.tags="kubernaut,intelligence,patterns,analytics,microservice"
