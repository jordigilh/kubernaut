"""
Configuration API Routes
Implements configuration and discovery endpoints - BR-HAPI-021 through BR-HAPI-025
"""

import structlog
from fastapi import APIRouter, HTTPException, Depends, Request

from models.api_models import ConfigResponse, ToolsetsResponse, ModelsResponse
from services.holmesgpt_service import HolmesGPTService

logger = structlog.get_logger(__name__)

router = APIRouter()


def get_holmesgpt_service(request: Request) -> HolmesGPTService:
    """Dependency injection for HolmesGPT service"""
    service = getattr(request.app.state, 'holmesgpt_service', None)
    if service is None:
        raise HTTPException(status_code=503, detail="HolmesGPT service not available")
    return service


@router.get("/config", response_model=ConfigResponse)
async def get_configuration(
    holmes_service: HolmesGPTService = Depends(get_holmesgpt_service)
) -> ConfigResponse:
    """Runtime Configuration - BR-HAPI-021"""
    logger.info("⚙️ Getting configuration")

    try:
        config = await holmes_service.get_configuration()

        response = ConfigResponse(**config)

        logger.info("✅ Configuration retrieved",
                   llm_provider=config.get('llm_provider'),
                   toolsets_count=len(config.get('available_toolsets', [])))

        return response

    except Exception as e:
        logger.error("❌ Configuration retrieval failed",
                    error=str(e), exc_info=True)
        raise HTTPException(
            status_code=500,
            detail=f"Configuration unavailable: {str(e)}"
        )


@router.get("/toolsets", response_model=ToolsetsResponse)
async def get_toolsets(
    holmes_service: HolmesGPTService = Depends(get_holmesgpt_service)
) -> ToolsetsResponse:
    """Available Toolsets - BR-HAPI-022, BR-HAPI-033"""
    logger.info("🛠️ Getting available toolsets")

    try:
        toolsets = await holmes_service.get_available_toolsets()

        response = ToolsetsResponse(toolsets=toolsets)

        logger.info("✅ Toolsets retrieved", count=len(toolsets))

        return response

    except Exception as e:
        logger.error("❌ Toolsets retrieval failed",
                    error=str(e), exc_info=True)
        raise HTTPException(
            status_code=500,
            detail=f"Toolsets unavailable: {str(e)}"
        )


@router.get("/models", response_model=ModelsResponse)
async def get_models(
    holmes_service: HolmesGPTService = Depends(get_holmesgpt_service)
) -> ModelsResponse:
    """Supported LLM Models - BR-HAPI-023"""
    logger.info("🤖 Getting supported models")

    try:
        models = await holmes_service.get_supported_models()

        response = ModelsResponse(models=models)

        logger.info("✅ Models retrieved", count=len(models))

        return response

    except Exception as e:
        logger.error("❌ Models retrieval failed",
                    error=str(e), exc_info=True)
        raise HTTPException(
            status_code=500,
            detail=f"Models unavailable: {str(e)}"
        )
