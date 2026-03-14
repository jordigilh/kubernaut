# Multi-arch origin-oauth-proxy (OpenShift OAuth Proxy)
# Builds: quay.io/jordigilh/ose-oauth-proxy:latest
# Architectures: linux/amd64, linux/arm64
#
# Source: https://github.com/openshift/oauth-proxy
# Based on upstream Dockerfile with Red Hat UBI base images
# Uses publicly accessible registry.access.redhat.com (no auth required)

# Build stage - Red Hat UBI 10 with Go 1.25
FROM registry.access.redhat.com/ubi10/go-toolset:1.25 AS builder

WORKDIR /workspace

# Clone oauth-proxy source
USER root
RUN git clone --depth 1 --branch master https://github.com/openshift/oauth-proxy.git /workspace && \
    chown -R 1001:0 /workspace

USER 1001
WORKDIR /workspace

# Build oauth-proxy (Go toolset handles GOARCH automatically)
RUN go build -o oauth-proxy .

# Runtime stage - Red Hat UBI 10 Minimal
FROM registry.access.redhat.com/ubi10/ubi-minimal:latest

# Copy binary from builder
COPY --from=builder /workspace/oauth-proxy /usr/bin/oauth-proxy

# Run as non-root
USER 1001

ENTRYPOINT ["/usr/bin/oauth-proxy"]

# Metadata (Red Hat labels standard)
LABEL name="kubernaut-ose-oauth-proxy" \
      vendor="Kubernaut" \
      version="latest" \
      summary="Multi-arch OpenShift OAuth Proxy for E2E Tests" \
      description="OpenShift OAuth Proxy built from source with ARM64 support for Kubernaut SOC2 E2E testing" \
      maintainer="jgil@redhat.com" \
      io.k8s.description="OAuth proxy for Kubernetes ServiceAccount token validation" \
      io.k8s.display-name="OpenShift OAuth Proxy" \
      io.openshift.tags="oauth,proxy,authentication"

