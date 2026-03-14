# Kubernaut Must-Gather Diagnostic Collection Tool
# BR-PLATFORM-001: Enterprise diagnostic collection following OpenShift must-gather pattern
# Base: UBI10 Standard (includes tar, gzip, findutils, coreutils pre-installed)
# Tools: kubectl, jq for Kubernetes diagnostics

FROM registry.access.redhat.com/ubi10/ubi:latest

ARG APP_VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

# OCI standard labels
LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
      org.opencontainers.image.version="${APP_VERSION}" \
      org.opencontainers.image.revision="${GIT_COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.title="kubernaut-must-gather"

LABEL maintainer="Kubernaut Platform Team" \
      description="Kubernaut must-gather diagnostic collection tool" \
      io.k8s.description="Collects diagnostic data from Kubernaut platform for support and troubleshooting" \
      io.k8s.display-name="Kubernaut Must-Gather" \
      io.openshift.tags="kubernaut,diagnostics,must-gather" \
      version="1.0.0"

# UBI Standard already includes: tar, gzip, findutils, util-linux, coreutils
# Only install additional tools we need

# Install kubectl (Kubernetes 1.31 - compatible with Kubernetes 1.30-1.32)
# Use TARGETARCH to download correct architecture binary
ARG TARGETARCH
RUN curl -LO "https://dl.k8s.io/release/v1.31.0/bin/linux/${TARGETARCH}/kubectl" && \
    chmod +x kubectl && \
    mv kubectl /usr/local/bin/kubectl

# Install jq for JSON processing (from EPEL or direct download)
# Try dnf first (available in UBI10 repos), fallback to direct download
ARG TARGETARCH
RUN dnf install -y jq || \
    (if [ "${TARGETARCH}" = "arm64" ]; then JQ_ARCH="arm64"; else JQ_ARCH="amd64"; fi && \
     curl -L "https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-linux-${JQ_ARCH}" -o /usr/local/bin/jq && \
     chmod +x /usr/local/bin/jq) && \
    dnf clean all

# Verify tools are available (skip kubectl version check due to runtime issues in build context)
RUN jq --version && \
    tar --version && \
    gzip --version && \
    test -x /usr/local/bin/kubectl

# Copy must-gather collection scripts
COPY gather.sh /usr/bin/gather
COPY collectors/ /usr/share/must-gather/collectors/
COPY sanitizers/ /usr/share/must-gather/sanitizers/
COPY utils/ /usr/share/must-gather/utils/

# Make scripts executable
RUN chmod +x /usr/bin/gather && \
    chmod +x /usr/share/must-gather/collectors/*.sh && \
    chmod +x /usr/share/must-gather/sanitizers/*.sh && \
    chmod +x /usr/share/must-gather/utils/*.sh

# Create output directory
RUN mkdir -p /must-gather

# Set working directory
WORKDIR /must-gather

# Default command
ENTRYPOINT ["/usr/bin/gather"]

