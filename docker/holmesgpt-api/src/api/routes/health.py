"""
Health and Management API Routes
Implements health, readiness, and status endpoints - BR-HAPI-016 through BR-HAPI-020
"""

import time
from typing import Dict, Any

import structlog
from fastapi import APIRouter, HTTPException, Depends, Request

from models.api_models import HealthResponse, StatusResponse
from services.holmesgpt_service import HolmesGPTService
from services.context_api_service import ContextAPIService

logger = structlog.get_logger(__name__)

router = APIRouter()


def get_holmesgpt_service(request: Request) -> HolmesGPTService:
    """Dependency injection for HolmesGPT service"""
    service = getattr(request.app.state, 'holmesgpt_service', None)
    if service is None:
        raise HTTPException(status_code=503, detail="HolmesGPT service not available")
    return service


def get_context_service(request: Request) -> ContextAPIService:
    """Dependency injection for Context API service"""
    service = getattr(request.app.state, 'context_api_service', None)
    if service is None:
        raise HTTPException(status_code=503, detail="Context API service not available")
    return service


@router.get("/health", response_model=HealthResponse)
async def health_check(
    holmes_service: HolmesGPTService = Depends(get_holmesgpt_service),
    context_service: ContextAPIService = Depends(get_context_service)
) -> HealthResponse:
    """
    Comprehensive Health Check - BR-HAPI-016, BR-HAPI-019

    Monitors SDK health, LLM connectivity, and resource usage.
    """
    logger.info("🏥 Performing health check")

    try:
        # Check all service components
        sdk_healthy = await holmes_service.health_check()
        context_healthy = await context_service.health_check()

        overall_health = sdk_healthy and context_healthy

        services = {
            "holmesgpt_sdk": "healthy" if sdk_healthy else "unhealthy",
            "context_api": "healthy" if context_healthy else "unhealthy"
        }

        status = "healthy" if overall_health else "degraded"

        logger.info("✅ Health check completed",
                   status=status,
                   services=services)

        return HealthResponse(
            status=status,
            timestamp=time.time(),
            services=services,
            version="1.0.0"
        )

    except Exception as e:
        logger.error("❌ Health check failed", error=str(e), exc_info=True)
        return HealthResponse(
            status="unhealthy",
            timestamp=time.time(),
            services={"error": str(e)},
            version="1.0.0"
        )


@router.get("/ready")
async def readiness_check(request: Request) -> Dict[str, Any]:
    """Kubernetes Readiness Probe - BR-HAPI-017"""
    logger.info("🎯 Performing readiness check")

    try:
        # Simple readiness check - services are initialized
        holmesgpt_ready = hasattr(request.app.state, 'holmesgpt_service') and \
                         request.app.state.holmesgpt_service is not None
        context_ready = hasattr(request.app.state, 'context_api_service') and \
                       request.app.state.context_api_service is not None

        ready = holmesgpt_ready and context_ready

        result = {
            "ready": ready,
            "timestamp": time.time(),
            "status": "ready" if ready else "not_ready",
            "services": {
                "holmesgpt_service": "ready" if holmesgpt_ready else "not_ready",
                "context_api_service": "ready" if context_ready else "not_ready"
            }
        }

        logger.info("✅ Readiness check completed", ready=ready)
        return result

    except Exception as e:
        logger.error("❌ Readiness check failed", error=str(e), exc_info=True)
        raise HTTPException(status_code=503, detail=f"Service not ready: {str(e)}")


@router.get("/status", response_model=StatusResponse)
async def service_status(
    holmes_service: HolmesGPTService = Depends(get_holmesgpt_service)
) -> StatusResponse:
    """Service Status and Capabilities - BR-HAPI-020"""
    logger.info("📊 Getting service status")

    try:
        capabilities = await holmes_service.get_capabilities()

        response = StatusResponse(
            service="holmesgpt-api",
            version="1.0.0",
            status="running",
            capabilities=capabilities,
            timestamp=time.time()
        )

        logger.info("✅ Service status retrieved",
                   capabilities=capabilities)

        return response

    except Exception as e:
        logger.error("❌ Status check failed", error=str(e), exc_info=True)
        raise HTTPException(status_code=500, detail=f"Status unavailable: {str(e)}")
