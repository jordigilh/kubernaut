# HolmesGPT API - Multi-Architecture Dockerfile using Red Hat UBI9
# Supports: linux/amd64, linux/arm64
# Based on: ADR-027 (Multi-Architecture Build Strategy)

# Build stage - Red Hat UBI9 Python 3.12
FROM registry.access.redhat.com/ubi9/python-312:latest AS builder

# Switch to root for package installation
USER root

# Install only essential build dependencies (skip dnf update to speed up build)
RUN dnf install -y --setopt=install_weak_deps=False gcc gcc-c++ git ca-certificates tzdata && \
	dnf clean all && \
	rm -rf /var/cache/dnf /var/cache/yum

# Switch back to default user for security
USER 1001

# Set working directory
WORKDIR /opt/app-root/src

# Copy HolmesGPT SDK dependencies (referenced in requirements.txt)
# NOTE: No trailing slash on source - copies directory itself, not just contents
COPY --chown=1001:0 dependencies ../dependencies

# Copy DataStorage Python client for installation
COPY --chown=1001:0 holmesgpt-api/src/clients/datastorage ./clients/datastorage

# Copy requirements first for layer caching
COPY --chown=1001:0 holmesgpt-api/requirements.txt ./

# Upgrade pip and install Python dependencies
# Install DataStorage client first, then requirements
# Remove unnecessary files after installation to reduce layer size
RUN pip install --no-cache-dir --upgrade pip && \
	pip install --no-cache-dir ./clients/datastorage && \
	pip install --no-cache-dir -r requirements.txt && \
	find /opt/app-root/lib/python3.12/site-packages -type d -name "tests" -exec rm -rf {} + 2>/dev/null || true && \
	find /opt/app-root/lib/python3.12/site-packages -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true && \
	find /opt/app-root/lib/python3.12/site-packages -name "*.pyc" -delete && \
	find /opt/app-root/lib/python3.12/site-packages -name "*.pyo" -delete

# Runtime stage - Red Hat UBI9 Python 3.12
FROM registry.access.redhat.com/ubi9/python-312:latest

# Install only essential runtime dependencies (skip dnf update to reduce size)
# kubectl is required for HolmesGPT kubernetes/core toolset (pod investigation)
USER root
ARG TARGETARCH
ARG KUBECTL_VERSION=v1.31.4
RUN dnf install -y --setopt=install_weak_deps=False ca-certificates tzdata && \
	dnf clean all && \
	rm -rf /var/cache/dnf /var/cache/yum && \
	curl -fsSL -o /usr/local/bin/kubectl \
		"https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" && \
	chmod +x /usr/local/bin/kubectl

# Set working directory (UBI9 Python already has UID 1001 user)
WORKDIR /opt/app-root/src

# Copy Python dependencies from builder (includes HolmesGPT SDK in site-packages)
# CRITICAL: Copy BOTH lib and lib64 - some packages install to lib64 (e.g., uvicorn, httptools)
COPY --from=builder /opt/app-root/lib/python3.12/site-packages /opt/app-root/lib/python3.12/site-packages
COPY --from=builder /opt/app-root/lib64/python3.12/site-packages /opt/app-root/lib64/python3.12/site-packages
COPY --from=builder /opt/app-root/bin /opt/app-root/bin

# Copy application code
COPY --chown=1001:0 holmesgpt-api/src/ ./src/
COPY --chown=1001:0 holmesgpt-api/requirements.txt ./
COPY --chown=1001:0 holmesgpt-api/entrypoint.sh ./

# Create necessary directories with correct permissions and clean up
RUN mkdir -p /tmp /opt/app-root/.cache && \
	chown -R 1001:0 /opt/app-root /tmp && \
	chmod -R g=u /opt/app-root /tmp && \
	chmod +x ./entrypoint.sh && \
	# Additional cleanup to reduce image size
	find /opt/app-root/lib/python3.12/site-packages -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true && \
	find /opt/app-root/lib/python3.12/site-packages -name "*.pyc" -delete 2>/dev/null || true && \
	find /opt/app-root/lib/python3.12/site-packages -name "*.pyo" -delete 2>/dev/null || true

# Switch to non-root user for security
USER 1001

# Environment variables
# CRITICAL: PYTHONPATH ensures Python 3.12 finds packages in /opt/app-root/lib/python3.12/site-packages
ENV PYTHONUNBUFFERED=1 \
	PYTHONDONTWRITEBYTECODE=1 \
	PATH="/opt/app-root/bin:${PATH}" \
	PYTHONPATH="/opt/app-root/src:/opt/app-root/lib/python3.12/site-packages:${PYTHONPATH}" \
	HOME=/opt/app-root

# Expose port
EXPOSE 8080

# Health check (use python3.12 explicitly - UBI9 python-312 image defaults to python3.9)
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
	CMD python3.12 -c "import urllib.request; urllib.request.urlopen('http://localhost:8080/health')" || exit 1

# Use entrypoint script to handle --config flag (ADR-030 - matches Go services)
ENTRYPOINT ["./entrypoint.sh"]
CMD []

# Red Hat UBI9 compatible metadata labels
LABEL name="kubernaut-holmesgpt-api" \
	vendor="Kubernaut" \
	version="1.0.0" \
	release="1" \
	summary="HolmesGPT API - AI-Powered Recovery Analysis Service" \
	description="A Python microservice component of Kubernaut that provides AI-powered recovery analysis for Kubernaut using HolmesGPT SDK and LLM integration." \
	maintainer="jgil@redhat.com" \
	component="holmesgpt-api" \
	part-of="kubernaut" \
	io.k8s.description="HolmesGPT API Service for Kubernaut" \
	io.k8s.display-name="Kubernaut HolmesGPT API" \
	io.openshift.tags="kubernaut,holmesgpt,ai,llm,recovery,python,microservice"
