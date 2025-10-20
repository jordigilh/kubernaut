# Multi-stage build for HolmesGPT API service using Red Hat UBI9 Python
# Design Decision: DD-HOLMESGPT-012 - Minimal Internal Service Architecture

# ========================================
# Stage 1: Builder (Install dependencies)
# ========================================
FROM registry.access.redhat.com/ubi9/python-311:latest AS builder

# Switch to root for package installation
USER root

# Install system dependencies (including git for pip install from git)
RUN dnf update -y && \
	dnf install -y git ca-certificates tzdata && \
	dnf clean all

# Switch back to default user for security
USER 1001

# Set working directory
WORKDIR /opt/app-root/src

# Copy dependency files first (for layer caching)
COPY --chown=1001:0 requirements.txt ./

# Install Python dependencies
# Note: requirements.txt includes HolmesGPT SDK from git
RUN pip install --no-cache-dir --upgrade pip && \
	pip install --no-cache-dir -r requirements.txt

# Copy application source code
COPY --chown=1001:0 src ./src
COPY --chown=1001:0 pytest.ini ./

# ========================================
# Stage 2: Runtime (Minimal production image)
# ========================================
FROM registry.access.redhat.com/ubi9/python-311:latest

# Switch to root for package installation
USER root

# Install runtime dependencies only
RUN dnf update -y && \
	dnf install -y ca-certificates tzdata && \
	dnf clean all

# Switch back to default user for security
USER 1001

# Set working directory
WORKDIR /opt/app-root/src

# Copy Python dependencies from builder (includes HolmesGPT SDK)
COPY --from=builder /opt/app-root/lib/python3.11/site-packages /opt/app-root/lib/python3.11/site-packages

# Copy application code from builder
COPY --from=builder --chown=1001:0 /opt/app-root/src/src ./src

# Set Python path to include dependencies
ENV PYTHONPATH=/opt/app-root/src:/opt/app-root/lib/python3.11/site-packages
ENV PATH=/opt/app-root/bin:$PATH

# Expose port for FastAPI (internal service)
# Design Decision: DD-HOLMESGPT-012 - Single port for internal service
EXPOSE 8080

# Health check using FastAPI health endpoint
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
	CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8080/health').read()" || exit 1

# Set entrypoint to run FastAPI with uvicorn
ENTRYPOINT ["python", "-m", "uvicorn", "src.main:app"]
CMD ["--host", "0.0.0.0", "--port", "8080"]

# Red Hat UBI9 compatible metadata labels
LABEL name="kubernaut-holmesgpt-api" \
	vendor="Kubernaut" \
	version="1.0.0" \
	summary="HolmesGPT API Service - Minimal Internal Service" \
	description="FastAPI wrapper around HolmesGPT SDK for Kubernaut-specific investigations" \
	io.k8s.description="HolmesGPT API Service for recovery and post-execution analysis" \
	io.k8s.display-name="HolmesGPT API" \
	io.openshift.tags="kubernaut,ai,holmesgpt,fastapi,python"

