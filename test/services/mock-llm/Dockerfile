# Mock LLM Service - Standalone Container
# Based on UBI9 Python 3.12 for Red Hat ecosystem compatibility
FROM registry.access.redhat.com/ubi9/python-312:latest

# Switch to root for setup
USER root

# Install runtime dependencies (minimal, UBI already includes most)
RUN dnf install -y --setopt=install_weak_deps=False \
    ca-certificates \
    tzdata && \
    dnf clean all && \
    rm -rf /var/cache/dnf /var/cache/yum

# Switch back to non-root user
USER 1001

WORKDIR /opt/app-root/src

# Copy source code (build context is test/services/mock-llm/)
COPY --chown=1001:0 src ./src

# Expose port 8080
EXPOSE 8080

# Set Python environment
ENV PYTHONUNBUFFERED=1
ENV PYTHONPATH=/opt/app-root/src

# Health check
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8080/health').read()" || exit 1

# Run the server
CMD ["python", "-m", "src"]

# Labels
LABEL name="mock-llm-service" \
      version="1.0.0" \
      description="Standalone OpenAI-compatible mock LLM for testing" \
      maintainer="Jordi Gil" \
      license="Apache-2.0" \
      summary="Mock LLM Service - Zero dependency test endpoint"
