"""
Investigation API Routes
Implements alert investigation endpoints - BR-HAPI-001 through BR-HAPI-005
"""

import structlog
from fastapi import APIRouter, HTTPException, Depends, BackgroundTasks, Request
from prometheus_client import Counter, Gauge

from models.api_models import InvestigateRequest, InvestigateResponse
from services.holmesgpt_service import HolmesGPTService
from services.context_api_service import ContextAPIService

logger = structlog.get_logger(__name__)

# Metrics
INVESTIGATION_COUNT = Counter('holmesgpt_investigations_total', 'Total investigations', ['status'])
ACTIVE_INVESTIGATIONS = Gauge('holmesgpt_investigations_active', 'Active investigations')

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


@router.post("/investigate", response_model=InvestigateResponse)
async def investigate_alert(
    request: InvestigateRequest,
    background_tasks: BackgroundTasks,
    holmes_service: HolmesGPTService = Depends(get_holmesgpt_service),
    context_service: ContextAPIService = Depends(get_context_service)
) -> InvestigateResponse:
    """
    Alert Investigation Endpoint - BR-HAPI-001 to BR-HAPI-005

    Accepts alert context and performs AI-powered investigation with recommended actions.
    Supports asynchronous processing with job tracking.
    """
    logger.info("🔍 Starting investigation",
               alert_name=request.alert_name,
               namespace=request.namespace,
               priority=request.priority)

    try:
        ACTIVE_INVESTIGATIONS.inc()
        INVESTIGATION_COUNT.labels(status="started").inc()

        # Validate and enrich context - BR-HAPI-013, BR-HAPI-011
        enriched_context = await context_service.enrich_alert_context(
            request.alert_name,
            request.namespace,
            request.labels,
            request.annotations
        )

        # Perform investigation - BR-HAPI-004
        investigation_result = await holmes_service.investigate_alert(
            alert_name=request.alert_name,
            namespace=request.namespace,
            context=enriched_context,
            priority=request.priority,
            async_mode=request.async_processing
        )

        INVESTIGATION_COUNT.labels(status="completed").inc()
        logger.info("✅ Investigation completed",
                   investigation_id=investigation_result.investigation_id,
                   recommendations_count=len(investigation_result.recommendations))

        return investigation_result

    except Exception as e:
        INVESTIGATION_COUNT.labels(status="failed").inc()
        logger.error("❌ Investigation failed",
                    alert_name=request.alert_name,
                    error=str(e), exc_info=True)
        raise HTTPException(
            status_code=500,
            detail=f"Investigation failed: {str(e)}"
        )
    finally:
        ACTIVE_INVESTIGATIONS.dec()
