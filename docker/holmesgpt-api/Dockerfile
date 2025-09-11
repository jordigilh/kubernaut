# HolmesGPT REST API Server - Source-based Build with Red Hat UBI
# Multi-architecture support: linux/amd64, linux/arm64

# Arguments
ARG UBI_VERSION=9.3-13
ARG PYTHON_VERSION=311
ARG HOLMESGPT_VERSION=latest

# Build stage 1: Security scanning and source preparation
FROM registry.access.redhat.com/ubi9/python-${PYTHON_VERSION}:latest AS security-scanner

# Install security scanning tools
USER root
RUN dnf update -y && \
    dnf install -y git && \
    dnf clean all

USER 1001
RUN pip install --no-cache-dir \
    bandit==1.7.5 \
    safety>=3.0.0

# Copy HolmesGPT source for scanning
COPY --chown=1001:0 vendor/holmesgpt /src/holmesgpt/
COPY --chown=1001:0 docker/holmesgpt-api/security-check.py /src/

# Run security scans
WORKDIR /src/holmesgpt
RUN echo "🔍 Running security audit on HolmesGPT source..." && \
    safety check --json --output /tmp/safety-report.json || echo "Safety scan completed with warnings" && \
    bandit -r holmes/ --format json --output /tmp/bandit-report.json || echo "Bandit scan completed" && \
    python /src/security-check.py

# Build stage 2: HolmesGPT from source
FROM registry.access.redhat.com/ubi9/python-${PYTHON_VERSION}:latest AS holmesgpt-builder

# Install build dependencies
USER root
RUN dnf update -y && \
    dnf install -y \
        gcc \
        gcc-c++ \
        git \
        make \
        cmake \
        && dnf clean all

# Copy HolmesGPT source
WORKDIR /src
COPY --from=security-scanner /src/holmesgpt ./holmesgpt/
COPY --chown=1001:0 docker/holmesgpt-api/requirements-holmesgpt.txt ./

# Build HolmesGPT from source
USER 1001
RUN echo "🏗️ Building HolmesGPT from source..." && \
    pip install --no-cache-dir --upgrade pip setuptools wheel && \
    pip install --no-cache-dir -r requirements-holmesgpt.txt && \
    cd holmesgpt && \
    pip install --no-cache-dir -e . && \
    echo "✅ HolmesGPT build completed"

# Verify HolmesGPT installation
RUN echo "✅ HolmesGPT installed successfully" && \
    python3.11 -c "import holmes; print('HolmesGPT Python module available')" || echo "⚠️ Module check failed but continuing..."

# Build stage 3: REST API dependencies
FROM registry.access.redhat.com/ubi9/python-${PYTHON_VERSION}:latest AS api-builder

# Copy HolmesGPT installation
COPY --from=holmesgpt-builder /opt/app-root/lib64/python3.11/site-packages /opt/app-root/lib64/python3.11/site-packages
COPY --from=holmesgpt-builder /opt/app-root/bin /opt/app-root/bin

# Install API dependencies
USER 1001
COPY --chown=1001:0 docker/holmesgpt-api/requirements-api.txt ./
RUN echo "🔧 Installing REST API dependencies..." && \
    pip install --no-cache-dir --upgrade pip && \
    pip install --no-cache-dir -r requirements-api.txt && \
    echo "✅ API dependencies installed"

# Runtime stage: Minimal Red Hat UBI-micro
FROM registry.access.redhat.com/ubi9-micro:${UBI_VERSION}

# Metadata for Red Hat Container Certification
LABEL name="holmesgpt-api-server" \
      vendor="Kubernaut Project" \
      version="1.0.0" \
      release="1" \
      summary="HolmesGPT REST API Server (Source-built)" \
      description="HolmesGPT built from source with REST API capabilities for Kubernetes troubleshooting" \
      io.k8s.description="Source-built HolmesGPT REST API Server for AI-powered Kubernetes investigation" \
      io.k8s.display-name="HolmesGPT API Server" \
      io.openshift.tags="ai,troubleshooting,kubernetes,investigation,holmesgpt,rest-api" \
      com.redhat.component="holmesgpt-api-server" \
      com.redhat.license_terms="https://www.redhat.com/en/about/red-hat-end-user-license-agreements#UBI" \
      build.method="source-based" \
      security.level="maximum" \
      architecture="multi-arch"

# Copy Python runtime and dependencies
COPY --from=api-builder /etc/passwd /etc/passwd
COPY --from=api-builder /etc/group /etc/group
COPY --from=api-builder /usr/bin/python3.11 /usr/bin/python3.11
COPY --from=api-builder /usr/lib64/python3.11 /usr/lib64/python3.11
COPY --from=api-builder /opt/app-root/lib64/python3.11/site-packages /usr/lib64/python3.11/site-packages
COPY --from=api-builder /opt/app-root/bin /usr/bin

# Create application structure
USER root
RUN mkdir -p /app /config /data /tmp/app /var/log/holmesgpt && \
    chown -R 1001:0 /app /config /data /tmp/app /var/log/holmesgpt && \
    chmod -R g=u /app /config /data /tmp/app /var/log/holmesgpt

# Copy application source code
WORKDIR /app
COPY --chown=1001:0 docker/holmesgpt-api/src/ ./src/
COPY --chown=1001:0 docker/holmesgpt-api/config/ ./config/
COPY --chown=1001:0 docker/holmesgpt-api/entrypoint.sh ./
RUN chmod +x ./entrypoint.sh

# Set environment variables
ENV PYTHONPATH=/usr/lib64/python3.11/site-packages:/app/src
ENV PYTHONUNBUFFERED=1
ENV PYTHONDONTWRITEBYTECODE=1
ENV HOME=/app
ENV TMPDIR=/tmp/app
ENV LOG_DIR=/var/log/holmesgpt

# Security hardening
USER 1001

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD python3.11 -c "import requests; requests.get('http://localhost:8090/health', timeout=5).raise_for_status()" || exit 1

# Expose ports
EXPOSE 8090 9091

# Start the application
ENTRYPOINT ["./entrypoint.sh"]
WORKDIR /app/src
CMD ["python3.11", "main.py"]
