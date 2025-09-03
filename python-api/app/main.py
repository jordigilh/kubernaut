"""
FastAPI REST API service for HolmesGPT integration.

This service provides REST endpoints to interact with HolmesGPT either through
direct Python module imports or CLI wrapping, offering a scalable API layer
for alert investigation and remediation.
"""

import os
import logging
import asyncio
from contextlib import asynccontextmanager
from typing import Dict, Any

import uvicorn
from fastapi import FastAPI, HTTPException, Depends, BackgroundTasks, Request, Response
from fastapi.middleware.cors import CORSMiddleware
from fastapi.middleware.gzip import GZipMiddleware
from fastapi.responses import JSONResponse
from prometheus_client import generate_latest, CONTENT_TYPE_LATEST, Counter, Histogram, Gauge
from prometheus_client import start_http_server

from app.config import Settings, get_settings
from app.services.holmes_service import HolmesGPTService
from app.models.requests import AskRequest, InvestigateRequest, HealthCheckRequest
from app.models.responses import (
    AskResponse, InvestigateResponse, HealthCheckResponse,
    ErrorResponse, ServiceInfoResponse
)
from app.utils.logging import setup_logging
from app.utils.metrics import MetricsManager

# Metrics
REQUEST_COUNT = Counter('holmesgpt_api_requests_total', 'Total requests', ['method', 'endpoint', 'status'])
REQUEST_DURATION = Histogram('holmesgpt_api_request_duration_seconds', 'Request duration', ['method', 'endpoint'])
ACTIVE_CONNECTIONS = Gauge('holmesgpt_api_active_connections', 'Active connections')
HOLMES_OPERATIONS = Counter('holmesgpt_operations_total', 'HolmesGPT operations', ['operation', 'status'])
HOLMES_DURATION = Histogram('holmesgpt_operation_duration_seconds', 'HolmesGPT operation duration', ['operation'])

# Global service instance
holmes_service: HolmesGPTService = None
metrics_manager: MetricsManager = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan manager."""
    global holmes_service, metrics_manager

    settings = get_settings()
    logger = logging.getLogger("holmesgpt-api")

    try:
        # Initialize metrics
        if settings.metrics_enabled:
            metrics_manager = MetricsManager()
            if settings.metrics_port != settings.port:
                # Start separate metrics server
                start_http_server(settings.metrics_port)
                logger.info(f"Metrics server started on port {settings.metrics_port}")

        # Initialize HolmesGPT service
        logger.info("Initializing HolmesGPT service...")
        holmes_service = HolmesGPTService(settings)
        await holmes_service.initialize()
        logger.info("HolmesGPT service initialized successfully")

        # Verify service health
        health_status = await holmes_service.health_check()
        if not health_status.healthy:
            logger.warning(f"HolmesGPT service health check failed: {health_status.message}")

        yield

    except Exception as e:
        logger.error(f"Failed to initialize application: {e}")
        raise
    finally:
        # Cleanup - handle failures gracefully
        if holmes_service:
            try:
                await holmes_service.cleanup()
                logger.info("HolmesGPT service cleanup completed")
            except Exception as cleanup_error:
                logger.error(f"Holmes service cleanup failed: {cleanup_error}")
                # Continue gracefully - don't propagate cleanup errors


def create_app() -> FastAPI:
    """Create and configure FastAPI application."""
    settings = get_settings()

    # Setup logging
    setup_logging(settings.log_level, settings.log_format)
    logger = logging.getLogger("holmesgpt-api")

    # Create FastAPI app
    app = FastAPI(
        title="HolmesGPT REST API",
        description="REST API service for HolmesGPT alert investigation and remediation",
        version="1.0.0",
        lifespan=lifespan,
        docs_url="/docs" if settings.enable_docs else None,
        redoc_url="/redoc" if settings.enable_docs else None,
    )

    # Add middleware
    app.add_middleware(GZipMiddleware, minimum_size=1000)

    if settings.enable_cors:
        app.add_middleware(
            CORSMiddleware,
            allow_origins=settings.cors_origins,
            allow_credentials=True,
            allow_methods=["*"],
            allow_headers=["*"],
        )

    # Request tracking middleware
    @app.middleware("http")
    async def track_requests(request: Request, call_next):
        start_time = asyncio.get_event_loop().time()
        ACTIVE_CONNECTIONS.inc()

        try:
            response = await call_next(request)
            duration = asyncio.get_event_loop().time() - start_time

            REQUEST_COUNT.labels(
                method=request.method,
                endpoint=request.url.path,
                status=response.status_code
            ).inc()
            REQUEST_DURATION.labels(
                method=request.method,
                endpoint=request.url.path
            ).observe(duration)

            return response
        except Exception as e:
            duration = asyncio.get_event_loop().time() - start_time
            REQUEST_COUNT.labels(
                method=request.method,
                endpoint=request.url.path,
                status=500
            ).inc()
            REQUEST_DURATION.labels(
                method=request.method,
                endpoint=request.url.path
            ).observe(duration)
            raise
        finally:
            ACTIVE_CONNECTIONS.dec()

    # Add simple root endpoint for testing
    @app.get("/")
    async def root():
        """Simple root endpoint for health and testing."""
        return {
            "name": "HolmesGPT REST API",
            "version": "1.0.0",
            "status": "running",
            "service": "holmesgpt-api",
            "features": {
                "direct_import": True,
                "cli_fallback": True,
                "async_operations": True,
                "metrics": True,
                "health_checks": True,
                "alert_investigation": True,
                "action_recommendation": True,
                "kubernetes_context": True,
                "action_history": True
            }
        }

    return app


# Create app instance
app = create_app()


async def get_holmes_service() -> HolmesGPTService:
    """Get the global HolmesGPT service instance."""
    if holmes_service is None:
        raise HTTPException(status_code=503, detail="HolmesGPT service not initialized")
    return holmes_service


@app.get("/", response_model=ServiceInfoResponse)
async def root():
    """Get service information."""
    settings = get_settings()
    return ServiceInfoResponse(
        name="HolmesGPT REST API",
        version="1.0.0",
        description="REST API service for HolmesGPT alert investigation and remediation",
        status="running",
        features={
            "direct_import": settings.holmes_direct_import,
            "cli_fallback": settings.holmes_cli_fallback,
            "async_operations": True,
            "metrics": settings.metrics_enabled,
            "health_checks": True,
        }
    )


@app.post("/ask", response_model=AskResponse)
async def ask(
    request: AskRequest,
    background_tasks: BackgroundTasks,
    service: HolmesGPTService = Depends(get_holmes_service)
):
    """
    Ask HolmesGPT a question and get recommendations.

    This endpoint allows you to ask HolmesGPT any question about your
    infrastructure, alerts, or operations. HolmesGPT will analyze the
    question and provide actionable recommendations.
    """
    start_time = asyncio.get_event_loop().time()

    try:
        HOLMES_OPERATIONS.labels(operation="ask", status="started").inc()

        # Execute ask operation
        response = await service.ask(
            prompt=request.prompt,
            context=request.context,
            options=request.options
        )

        duration = asyncio.get_event_loop().time() - start_time
        HOLMES_DURATION.labels(operation="ask").observe(duration)
        HOLMES_OPERATIONS.labels(operation="ask", status="success").inc()

        # Log operation for analytics (background task)
        background_tasks.add_task(
            log_operation,
            operation="ask",
            duration=duration,
            success=True,
            metadata={"prompt_length": len(request.prompt), "confidence": response.confidence}
        )

        return response

    except Exception as e:
        duration = asyncio.get_event_loop().time() - start_time
        HOLMES_DURATION.labels(operation="ask").observe(duration)
        HOLMES_OPERATIONS.labels(operation="ask", status="error").inc()

        # Log error (background task)
        background_tasks.add_task(
            log_operation,
            operation="ask",
            duration=duration,
            success=False,
            metadata={"error": str(e)}
        )

        logging.getLogger("holmesgpt-api").error(f"Ask operation failed: {e}")
        raise HTTPException(status_code=500, detail=f"Ask operation failed: {str(e)}")


@app.post("/investigate", response_model=InvestigateResponse)
async def investigate(
    request: InvestigateRequest,
    background_tasks: BackgroundTasks,
    service: HolmesGPTService = Depends(get_holmes_service)
):
    """
    Investigate an alert and get detailed analysis and remediation steps.

    This endpoint analyzes a specific alert, gathering context from your
    monitoring systems and providing detailed investigation results with
    step-by-step remediation recommendations.
    """
    start_time = asyncio.get_event_loop().time()

    try:
        HOLMES_OPERATIONS.labels(operation="investigate", status="started").inc()

        # Execute investigation
        response = await service.investigate(
            alert=request.alert,
            context=request.context,
            options=request.options
        )

        duration = asyncio.get_event_loop().time() - start_time
        HOLMES_DURATION.labels(operation="investigate").observe(duration)
        HOLMES_OPERATIONS.labels(operation="investigate", status="success").inc()

        # Log operation for analytics (background task)
        background_tasks.add_task(
            log_operation,
            operation="investigate",
            duration=duration,
            success=True,
            metadata={
                "alert_name": request.alert.name,
                "alert_severity": request.alert.severity,
                "confidence": response.confidence,
                "recommendations_count": len(response.recommendations)
            }
        )

        return response

    except Exception as e:
        duration = asyncio.get_event_loop().time() - start_time
        HOLMES_DURATION.labels(operation="investigate").observe(duration)
        HOLMES_OPERATIONS.labels(operation="investigate", status="error").inc()

        # Log error (background task)
        background_tasks.add_task(
            log_operation,
            operation="investigate",
            duration=duration,
            success=False,
            metadata={"error": str(e), "alert_name": request.alert.name}
        )

        logging.getLogger("holmesgpt-api").error(f"Investigation failed: {e}")
        raise HTTPException(status_code=500, detail=f"Investigation failed: {str(e)}")


@app.get("/health", response_model=HealthCheckResponse)
async def health():
    """
    Comprehensive health check endpoint.

    Returns the health status of the API service and all its dependencies,
    including HolmesGPT service, external APIs, and system resources.
    """
    try:
        if holmes_service is None:
            return HealthCheckResponse(
                healthy=False,
                status="service_not_initialized",
                message="HolmesGPT service not initialized",
                checks={},
                timestamp=asyncio.get_event_loop().time()
            )

        # Perform comprehensive health check
        health_status = await holmes_service.health_check()

        # Always return the health status - let the client decide based on "healthy" field
        return health_status

    except HTTPException:
        # Re-raise HTTPExceptions (like the ones we created above)
        raise
    except Exception as e:
        logging.getLogger("holmesgpt-api").error(f"Health check failed: {e}")
        return HealthCheckResponse(
            healthy=False,
            status="health_check_failed",
            message=f"Health check failed: {str(e)}",
            checks={},
            timestamp=asyncio.get_event_loop().time()
        )


@app.get("/ready")
async def readiness():
    """
    Readiness check endpoint.

    Returns 200 when the service is ready to accept requests.
    This is typically used by Kubernetes readiness probes.
    """
    try:
        # Simple readiness check - verify holmes_service is available
        if holmes_service is None:
            raise HTTPException(status_code=503, detail={"ready": False, "message": "Service not ready"})

        return {"ready": True, "message": "Service is ready", "timestamp": asyncio.get_event_loop().time()}

    except HTTPException:
        raise
    except Exception as e:
        logging.getLogger("holmesgpt-api").error(f"Readiness check failed: {e}")
        raise HTTPException(status_code=503, detail={"ready": False, "message": f"Readiness check failed: {str(e)}"})


@app.get("/metrics")
async def metrics():
    """Prometheus metrics endpoint."""
    from fastapi import Response

    settings = get_settings()
    if not settings.metrics_enabled:
        raise HTTPException(status_code=404, detail="Metrics not enabled")

    metrics_output = generate_latest()
    return Response(content=metrics_output, media_type=CONTENT_TYPE_LATEST)


@app.get("/service/info", response_model=Dict[str, Any])
async def service_info(service: HolmesGPTService = Depends(get_holmes_service)):
    """Get detailed service and HolmesGPT information."""
    try:
        info = await service.get_service_info()
        return info
    except Exception as e:
        logging.getLogger("holmesgpt-api").error(f"Failed to get service info: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to get service info: {str(e)}")


@app.post("/service/reload")
async def reload_service(service: HolmesGPTService = Depends(get_holmes_service)):
    """Reload HolmesGPT service configuration."""
    try:
        await service.reload()
        return {"status": "success", "message": "Service reloaded successfully"}
    except Exception as e:
        logging.getLogger("holmesgpt-api").error(f"Failed to reload service: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to reload service: {str(e)}")


@app.exception_handler(Exception)
async def global_exception_handler(request: Request, exc: Exception):
    """Global exception handler."""
    logging.getLogger("holmesgpt-api").error(f"Unhandled exception: {exc}", exc_info=True)
    return JSONResponse(
        status_code=500,
        content=ErrorResponse(
            error="internal_server_error",
            message=f"Internal server error: {str(exc)}",
            details={}
        ).model_dump()
    )


async def log_operation(operation: str, duration: float, success: bool, metadata: Dict[str, Any]):
    """Log operation for analytics (background task)."""
    try:
        logger = logging.getLogger("holmesgpt-api.analytics")
        logger.info(
            f"Operation completed",
            extra={
                "operation": operation,
                "duration": duration,
                "success": success,
                "metadata": metadata
            }
        )
    except Exception as e:
        logging.getLogger("holmesgpt-api").warning(f"Failed to log operation: {e}")


if __name__ == "__main__":
    settings = get_settings()
    uvicorn.run(
        "app.main:app",
        host=settings.host,
        port=settings.port,
        reload=settings.debug_mode,
        log_level=settings.log_level.lower(),
        access_log=True,
        workers=1 if settings.debug_mode else settings.workers,
    )

