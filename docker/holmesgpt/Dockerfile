# HolmesGPT Container for Go Bindings Integration
FROM python:3.11-slim

# Install system dependencies
RUN apt-get update && apt-get install -y \
    curl \
    git \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# Install kubectl for Kubernetes integration
RUN curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" \
    && chmod +x kubectl \
    && mv kubectl /usr/local/bin/

# Install HolmesGPT
RUN pip install --no-cache-dir holmesgpt

# Create working directories
RUN mkdir -p /shared /runbooks /app

# Set working directory
WORKDIR /app

# Create entrypoint script
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Set environment variables
ENV PYTHONUNBUFFERED=1
ENV HOLMES_WORKING_DIR=/shared
ENV KUBECONFIG=/root/.kube/config

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD holmes --version || exit 1

# Default command (keep container running)
CMD ["/app/entrypoint.sh"]

