# Kubernaut Embedding Service - Multi-stage Docker Build
# Base: Red Hat Universal Base Image (UBI) 9 with Python 3.11
# Model: all-mpnet-base-v2 (pre-downloaded during build)

# ============================================
# Stage 1: Builder
# ============================================
FROM registry.access.redhat.com/ubi9/python-311:latest AS builder

# Set working directory
WORKDIR /app

# Copy requirements first (layer caching)
COPY requirements.txt .

# Install dependencies
# Note: sentence-transformers will download model on first use
RUN pip install --no-cache-dir --upgrade pip && \
    pip install --no-cache-dir -r requirements.txt

# Copy application code
COPY src/ ./src/

# Pre-download embedding model (cached in image)
# This prevents cold-start latency in production
# Set cache to known location that will be copied to runtime stage
ENV TRANSFORMERS_CACHE=/opt/app-root/.cache
ENV HF_HOME=/opt/app-root/.cache
RUN python3 -c "from sentence_transformers import SentenceTransformer; \
    model = SentenceTransformer('sentence-transformers/all-mpnet-base-v2'); \
    print('Model downloaded successfully')"

# ============================================
# Stage 2: Runtime
# ============================================
# NOTE: ubi9/python-311-minimal doesn't exist, use full python-311 or standard python
FROM registry.access.redhat.com/ubi9/python-311:latest

# Metadata
LABEL maintainer="Kubernaut Team" \
      description="Text-to-vector embedding service using sentence-transformers" \
      model="all-mpnet-base-v2" \
      dimensions="768" \
      version="1.0.0"

# Set working directory
WORKDIR /app

# Copy Python dependencies from builder
COPY --from=builder /opt/app-root/lib/python3.11/site-packages /opt/app-root/lib/python3.11/site-packages

# Copy application code
COPY --from=builder /app/src ./src

# Copy pre-downloaded model from builder
COPY --from=builder /opt/app-root/.cache /opt/app-root/.cache

# Set environment variables
ENV PYTHONUNBUFFERED=1 \
    EMBEDDING_SERVICE_HOST=0.0.0.0 \
    EMBEDDING_SERVICE_PORT=8086 \
    EMBEDDING_DEVICE=cpu \
    LOG_LEVEL=INFO \
    TRANSFORMERS_CACHE=/opt/app-root/.cache

# Expose port
EXPOSE 8086

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD python3 -c "import requests; requests.get('http://localhost:8086/health').raise_for_status()"

# Run as non-root user (UID 1000)
USER 1000

# Start service
CMD ["python3", "-m", "uvicorn", "src.main:app", "--host", "0.0.0.0", "--port", "8086"]

