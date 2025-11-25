# Kubernaut Embedding Service

Text-to-vector embedding service using sentence-transformers for semantic search capabilities.

## Overview

- **Model**: all-mpnet-base-v2 (768 dimensions, 92% accuracy)
- **Framework**: FastAPI with Pydantic validation
- **Deployment**: Sidecar container in Data Storage pod
- **Port**: 8086

## Quick Start

### Development (venv)

```bash
# Create virtual environment
python3.11 -m venv venv
source venv/bin/activate

# Install dependencies
pip install -r requirements.txt -r requirements-dev.txt

# Run service
uvicorn src.main:app --host 0.0.0.0 --port 8086 --reload
```

### Production (Docker)

```bash
# Build image
docker build -t embedding-service:v1.0 .

# Run container
docker run -d -p 8086:8086 --name embedding-service embedding-service:v1.0
```

## API Endpoints

### POST /api/v1/embed
Generate embedding vector for text.

**Request:**
```json
{
  "text": "OOMKilled pod in production namespace"
}
```

**Response:**
```json
{
  "embedding": [0.1, 0.2, ...],  // 768 dimensions
  "dimensions": 768,
  "model": "all-mpnet-base-v2"
}
```

### GET /health
Health check endpoint.

**Response:**
```json
{
  "status": "healthy",
  "model": "all-mpnet-base-v2",
  "dimensions": 768
}
```

### GET /metrics
Prometheus metrics endpoint.

## Testing

```bash
# Run all tests
pytest tests/ -v

# Run with coverage
pytest tests/ -v --cov=src --cov-report=term

# Expected: 12 tests pass, ≥70% coverage
```

## Architecture

- **Sidecar Pattern**: Deployed alongside Data Storage service
- **Internal Communication**: HTTP on localhost:8086
- **Caching**: Data Storage caches embeddings in Redis (24h TTL)
- **Model Loading**: Pre-downloaded during Docker build (cached in image)

## Performance

- **Latency**: p95 ~50ms, p99 ~100ms
- **Throughput**: ~20 requests/second (single instance)
- **Memory**: ~1.2GB (model loaded in RAM)

## Business Requirements

- **BR-STORAGE-014**: Data Storage must cache embeddings with graceful degradation
- **DD-CACHE-001**: Uses shared Redis library for caching

## Related Documentation

- Implementation Plan: `docs/services/stateless/data-storage/EMBEDDING_SERVICE_IMPLEMENTATION_PLAN_V1.2.md`
- Design Decision: `docs/architecture/decisions/DD-CACHE-001-shared-redis-library.md`

