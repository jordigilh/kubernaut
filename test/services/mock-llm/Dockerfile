# Mock LLM Service - Multi-Architecture Dockerfile using Red Hat UBI9
# Supports: linux/amd64, linux/arm64
#
# Zero external dependencies - uses Python standard library only
# Minimal image size optimized for test environments

# Single-stage build (no external dependencies to install)
FROM registry.access.redhat.com/ubi9/python-312:latest

# Install only essential runtime dependencies
USER root
RUN dnf install -y --setopt=install_weak_deps=False ca-certificates tzdata && \
    dnf clean all && \
    rm -rf /var/cache/dnf /var/cache/yum

# Switch back to default user for security
USER 1001

# Set working directory
WORKDIR /opt/app-root/src

# Copy Mock LLM source code
COPY --chown=1001:0 test/services/mock-llm/src ./src

# Expose port 8080 (standard for Python apps in UBI)
EXPOSE 8080

# Set Python path to find src module
ENV PYTHONUNBUFFERED=1
ENV PYTHONPATH=/opt/app-root/src

# Health check using built-in health endpoint
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8080/health').read()" || exit 1

# Run the mock LLM server
CMD ["python", "-m", "src"]

# Metadata labels
LABEL name="mock-llm-service" \
      version="1.0.0" \
      description="Standalone OpenAI-compatible mock LLM for testing" \
      maintainer="Jordi Gil" \
      license="Apache-2.0" \
      summary="Mock LLM Service - Zero dependency test endpoint"
