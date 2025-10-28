# Multi-stage Dockerfile for remediationprocessor controller
# Based on Red Hat Universal Base Image 9 (UBI9)

# Build stage
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 AS builder

# Build arguments
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG MODULE_PATH=github.com/jordigilh/kubernaut

# Set working directory (UBI9 go-toolset default)
WORKDIR /opt/app-root/src

# Copy go module files
COPY --chown=1001:0 go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY --chown=1001:0 . .

# Build the controller binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} \
	go build -a -o remediationprocessor \
	-ldflags="-X ${MODULE_PATH}/internal/version.Version=${VERSION} \
	-X ${MODULE_PATH}/internal/version.Commit=${COMMIT} \
	-X ${MODULE_PATH}/internal/version.BuildDate=${BUILD_DATE}" \
	./cmd/remediationprocessor

# Runtime stage
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Red Hat UBI9 Labels (13 required labels)
LABEL name="kubernaut-remediationprocessor" \
	vendor="Kubernaut Project" \
	version="${VERSION}" \
	release="1" \
	summary="RemediationProcessor controller for Kubernaut" \
	description="Kubernetes controller for remediation processing and enrichment in Kubernaut AIOps platform" \
	maintainer="kubernaut-dev@jordigilh.com" \
	url="https://github.com/jordigilh/kubernaut" \
	distribution-scope="public" \
	architecture="multi" \
	vcs-type="git" \
	vcs-ref="${COMMIT}" \
	build-date="${BUILD_DATE}"

# Install minimal runtime dependencies
RUN microdnf install -y ca-certificates shadow-utils && \
	microdnf clean all

# Create non-root user (UID 1001 is Red Hat UBI9 convention)
RUN useradd -u 1001 -r -g 0 -M -d /app -s /sbin/nologin \
	-c "remediationprocessor user" remediationprocessor

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder --chown=1001:0 /opt/app-root/src/remediationprocessor /app/remediationprocessor

# Copy configuration (if exists)
# COPY --chown=1001:0 config/ /etc/remediationprocessor/

# Switch to non-root user
USER 1001

# Expose ports
# Metrics port
EXPOSE 8080
# Health check port
EXPOSE 8081

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
	CMD ["/app/remediationprocessor", "--health-check"]

# Set entrypoint
ENTRYPOINT ["/app/remediationprocessor"]

# Default arguments
CMD ["--config=/etc/remediationprocessor/config.yaml"]

# Build info
ARG VERSION
ARG COMMIT
ARG BUILD_DATE
ENV VERSION=${VERSION} \
	COMMIT=${COMMIT} \
	BUILD_DATE=${BUILD_DATE}

# Multi-architecture support
# Supports: linux/amd64, linux/arm64
# Build with: docker buildx build --platform linux/amd64,linux/arm64

