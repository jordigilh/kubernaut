"""
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
"""

"""
Kubernaut Embedding Service - FastAPI Application

Text-to-vector embedding service using sentence-transformers.
Deployed as sidecar container in Data Storage pod.

Endpoints:
- POST /api/v1/embed: Generate embedding vector
- GET /health: Health check
- GET /metrics: Prometheus metrics
- GET /: Service information

Model: all-mpnet-base-v2 (768 dimensions, 92% accuracy)
Port: 8086
"""

import time
import logging
from contextlib import asynccontextmanager
from typing import Optional

from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import PlainTextResponse, JSONResponse
from prometheus_client import Counter, Histogram, generate_latest, CONTENT_TYPE_LATEST

from src.service import EmbeddingService
from src.models import EmbedRequest, EmbedResponse, HealthResponse, ErrorResponse
from src.config import Config


# Configure logging
logging.basicConfig(
    level=Config.LOG_LEVEL,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# Prometheus metrics
embedding_requests_total = Counter(
    'embedding_requests_total',
    'Total number of embedding requests',
    ['status']  # success, error
)

embedding_duration_seconds = Histogram(
    'embedding_duration_seconds',
    'Embedding generation duration in seconds',
    buckets=[0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1.0, 2.5, 5.0]
)

# Global service instance (initialized in lifespan)
service: Optional[EmbeddingService] = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """
    FastAPI lifespan context manager.

    Handles service initialization and cleanup:
    - Startup: Load embedding model into memory
    - Shutdown: Cleanup resources
    """
    global service

    # Startup
    logger.info("Starting Kubernaut Embedding Service")
    logger.info(f"Model: {Config.MODEL_NAME}")
    logger.info(f"Device: {Config.DEVICE}")
    logger.info(f"Port: {Config.PORT}")

    try:
        service = EmbeddingService()
        logger.info("Embedding service initialized successfully")
    except Exception as e:
        logger.error(f"Failed to initialize embedding service: {e}")
        raise

    yield

    # Shutdown
    logger.info("Shutting down Kubernaut Embedding Service")
    service = None


# Create FastAPI application
app = FastAPI(
    title="Kubernaut Embedding Service",
    description="Text-to-vector embedding service using sentence-transformers",
    version="1.0.0",
    lifespan=lifespan
)


@app.get("/health", response_model=HealthResponse)
async def health():
    """
    Health check endpoint.

    Returns service status and model information.
    Used by Kubernetes liveness and readiness probes.

    Returns:
        HealthResponse: Service health status

    Example:
        GET /health

        Response:
        {
          "status": "healthy",
          "model": "all-mpnet-base-v2",
          "dimensions": 768
        }
    """
    if service is None:
        raise HTTPException(
            status_code=503,
            detail="Service not initialized"
        )

    return HealthResponse(
        status="healthy",
        model="all-mpnet-base-v2",
        dimensions=768
    )


@app.post("/api/v1/embed", response_model=EmbedResponse)
async def embed(request: EmbedRequest):
    """
    Generate embedding vector for text.

    Args:
        request: EmbedRequest with text field

    Returns:
        EmbedResponse: 768-dimensional embedding vector with metadata

    Raises:
        HTTPException 400: Invalid request (empty text, too long)
        HTTPException 500: Embedding generation failed
        HTTPException 503: Service not initialized

    Performance:
        - Latency: p95 ~50ms, p99 ~100ms
        - Throughput: ~20 requests/second

    Example:
        POST /api/v1/embed
        {
          "text": "OOMKilled pod in production namespace"
        }

        Response:
        {
          "embedding": [0.1, 0.2, ..., 0.9],  // 768 dimensions
          "dimensions": 768,
          "model": "all-mpnet-base-v2"
        }
    """
    if service is None:
        embedding_requests_total.labels(status='error').inc()
        raise HTTPException(
            status_code=503,
            detail="Service not initialized"
        )

    start_time = time.time()

    try:
        # Generate embedding
        embedding = service.embed(request.text)

        # Record metrics
        duration = time.time() - start_time
        embedding_duration_seconds.observe(duration)
        embedding_requests_total.labels(status='success').inc()

        logger.debug(f"Generated embedding in {duration:.3f}s for text: {request.text[:50]}...")

        return EmbedResponse(
            embedding=embedding,
            dimensions=768,
            model="all-mpnet-base-v2"
        )

    except ValueError as e:
        # Client error (invalid input)
        embedding_requests_total.labels(status='error').inc()
        logger.warning(f"Invalid request: {e}")
        raise HTTPException(status_code=400, detail=str(e))

    except Exception as e:
        # Server error (embedding generation failed)
        embedding_requests_total.labels(status='error').inc()
        logger.error(f"Embedding generation failed: {e}")
        raise HTTPException(
            status_code=500,
            detail=f"Failed to generate embedding: {str(e)}"
        )


@app.get("/metrics")
async def metrics():
    """
    Prometheus metrics endpoint.

    Returns metrics in Prometheus text format:
    - embedding_requests_total: Total requests by status
    - embedding_duration_seconds: Request duration histogram

    Used by Prometheus scraper for monitoring.

    Example:
        GET /metrics

        Response (text/plain):
        # HELP embedding_requests_total Total number of embedding requests
        # TYPE embedding_requests_total counter
        embedding_requests_total{status="success"} 42.0
        embedding_requests_total{status="error"} 2.0
        ...
    """
    return PlainTextResponse(
        generate_latest(),
        media_type=CONTENT_TYPE_LATEST
    )


@app.get("/")
async def root():
    """
    Service information endpoint.

    Returns basic service information and available endpoints.

    Example:
        GET /

        Response:
        {
          "service": "Kubernaut Embedding Service",
          "version": "1.0.0",
          "model": "all-mpnet-base-v2",
          "dimensions": 768,
          "endpoints": [...]
        }
    """
    return {
        "service": "Kubernaut Embedding Service",
        "version": "1.0.0",
        "model": "all-mpnet-base-v2",
        "dimensions": 768,
        "endpoints": {
            "embed": "POST /api/v1/embed",
            "health": "GET /health",
            "metrics": "GET /metrics",
            "info": "GET /"
        },
        "documentation": "/docs"
    }


@app.exception_handler(Exception)
async def global_exception_handler(request: Request, exc: Exception):
    """
    Global exception handler for unhandled errors.

    Logs error and returns structured error response.
    """
    logger.error(f"Unhandled exception: {exc}", exc_info=True)
    embedding_requests_total.labels(status='error').inc()

    return JSONResponse(
        status_code=500,
        content={
            "error": "Internal server error",
            "detail": str(exc)
        }
    )


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(
        "src.main:app",
        host=Config.HOST,
        port=Config.PORT,
        log_level=Config.LOG_LEVEL.lower(),
        reload=False  # Disable reload in production
    )

