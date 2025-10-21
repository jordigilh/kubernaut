# HolmesGPT API - Multi-Architecture Dockerfile using Red Hat UBI9
# Supports: linux/amd64, linux/arm64
# Based on: ADR-027 (Multi-Architecture Build Strategy)

# Build stage - Red Hat UBI9 Python 3.12
FROM registry.access.redhat.com/ubi9/python-312:latest AS builder

# Switch to root for package installation
USER root

# Install build dependencies
RUN dnf update -y && \
	dnf install -y gcc gcc-c++ git ca-certificates tzdata && \
	dnf clean all

# Switch back to default user for security
USER 1001

# Set working directory
WORKDIR /opt/app-root/src

# Copy HolmesGPT SDK dependencies (referenced in requirements.txt)
COPY --chown=1001:0 dependencies/ ../dependencies/

# Copy requirements first for layer caching
COPY --chown=1001:0 holmesgpt-api/requirements.txt ./

# Upgrade pip and install Python dependencies
RUN pip install --no-cache-dir --upgrade pip && \
	pip install --no-cache-dir -r requirements.txt

# Runtime stage - Red Hat UBI9 Python 3.12
FROM registry.access.redhat.com/ubi9/python-312:latest

# Install runtime dependencies
USER root
RUN dnf update -y && \
	dnf install -y ca-certificates tzdata && \
	dnf clean all

# Set working directory (UBI9 Python already has UID 1001 user)
WORKDIR /opt/app-root/src

# Copy Python dependencies from builder (includes HolmesGPT SDK in site-packages)
COPY --from=builder /opt/app-root/lib/python3.12/site-packages /opt/app-root/lib/python3.12/site-packages
COPY --from=builder /opt/app-root/bin /opt/app-root/bin

# Copy application code
COPY --chown=1001:0 holmesgpt-api/src/ ./src/
COPY --chown=1001:0 holmesgpt-api/requirements.txt ./

# Create necessary directories with correct permissions
RUN mkdir -p /tmp /opt/app-root/.cache && \
	chown -R 1001:0 /opt/app-root /tmp && \
	chmod -R g=u /opt/app-root /tmp

# Switch to non-root user for security
USER 1001

# Environment variables
ENV PYTHONUNBUFFERED=1 \
	PYTHONDONTWRITEBYTECODE=1 \
	PATH="/opt/app-root/bin:${PATH}" \
	HOME=/opt/app-root

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
	CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8080/health')" || exit 1

# Run FastAPI with uvicorn
CMD ["uvicorn", "src.main:app", "--host", "0.0.0.0", "--port", "8080", "--workers", "4"]

# Red Hat UBI9 compatible metadata labels
LABEL name="kubernaut-holmesgpt-api" \
	vendor="Kubernaut" \
	version="1.0.0" \
	release="1" \
	summary="HolmesGPT API - AI-Powered Recovery Analysis Service" \
	description="A Python microservice component of Kubernaut that provides AI-powered recovery analysis for Kubernetes remediation using HolmesGPT SDK and LLM integration." \
	maintainer="jgil@redhat.com" \
	component="ai-analysis" \
	part-of="kubernaut" \
	io.k8s.description="HolmesGPT API Service for AI-powered Kubernetes remediation analysis" \
	io.k8s.display-name="Kubernaut HolmesGPT API" \
	io.openshift.tags="kubernaut,holmesgpt,ai,llm,recovery,python,microservice"
