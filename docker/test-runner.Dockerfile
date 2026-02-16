# ADR-027: Multi-Architecture Container Build Strategy with Red Hat UBI Base Images
# Build Stage: Red Hat UBI9 Go Toolset 1.24 (matches go.mod: go 1.24.6)
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder

USER root

# Install test dependencies
# Note: curl-minimal is pre-installed in UBI9, no need to install curl
RUN dnf install -y \
    git \
    make \
    podman \
    gcc \
    && dnf clean all

# Install Kind (for E2E tests)
RUN curl -Lo /usr/local/bin/kind https://github.com/kubernetes-sigs/kind/releases/download/v0.30.0/kind-linux-amd64 && \
    chmod +x /usr/local/bin/kind

# Install kubectl (for E2E tests)
# Use fixed version to avoid network issues fetching latest version
RUN curl -LO "https://dl.k8s.io/release/v1.28.0/bin/linux/amd64/kubectl" && \
    chmod +x kubectl && \
    mv kubectl /usr/local/bin/

# Install ginkgo CLI (compatible with Go 1.24+)
RUN go install github.com/onsi/ginkgo/v2/ginkgo@latest && \
    cp /opt/app-root/src/go/bin/ginkgo /usr/local/bin/

WORKDIR /workspace

# Set Go environment
ENV GOFLAGS="-mod=mod"
ENV CGO_ENABLED=0
ENV PATH="/usr/local/bin:${PATH}"

# Default: Run all tests
CMD ["make", "test-all-services"]

