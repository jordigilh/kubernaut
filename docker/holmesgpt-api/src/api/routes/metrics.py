"""
Metrics API Routes
Implements Prometheus metrics endpoint - BR-HAPI-016 through BR-HAPI-020
"""

import structlog
from fastapi import APIRouter, Response, Depends, Request
from fastapi.responses import PlainTextResponse

from services.metrics_service import MetricsService, get_metrics_service
from services.auth_service import User, Permission
from api.routes.auth import get_current_active_user
from config import get_settings

logger = structlog.get_logger(__name__)

router = APIRouter()


def require_metrics_permission(current_user: User = Depends(get_current_active_user)) -> User:
    """Require permission to view metrics"""
    if not current_user.has_permission(Permission.VIEW_HEALTH):
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Insufficient permissions to view metrics"
        )
    return current_user


@router.get("/metrics", response_class=PlainTextResponse, summary="Prometheus Metrics")
async def get_metrics(
    metrics_service: MetricsService = Depends(get_metrics_service)
):
    """
    Export Prometheus metrics in text format.

    **Required permissions**: None (public endpoint for Prometheus scraping)

    Returns metrics in Prometheus exposition format for scraping by monitoring systems.
    This endpoint is typically accessed by Prometheus without authentication.
    """
    try:
        metrics_data = metrics_service.get_metrics()

        logger.debug("Metrics exported successfully")

        return Response(
            content=metrics_data,
            media_type=metrics_service.get_content_type()
        )

    except Exception as e:
        logger.error(f"Failed to export metrics: {e}", exc_info=True)
        return Response(
            content="# Failed to export metrics\n",
            media_type="text/plain; charset=utf-8"
        )


@router.get("/metrics/health", summary="Metrics Health Check")
async def metrics_health(
    current_user: User = Depends(require_metrics_permission),
    metrics_service: MetricsService = Depends(get_metrics_service)
):
    """
    Check the health of the metrics system.

    **Required permissions**: VIEW_HEALTH
    """
    try:
        # Basic health check - ensure metrics service is responding
        test_metrics = metrics_service.get_metrics()

        return {
            "status": "healthy",
            "message": "Metrics service is operational",
            "metrics_count": len([line for line in test_metrics.split('\n') if line and not line.startswith('#')])
        }

    except Exception as e:
        logger.error(f"Metrics health check failed: {e}")
        return {
            "status": "unhealthy",
            "message": f"Metrics service error: {str(e)}"
        }
