"""
Chat API Routes
Implements interactive chat endpoints - BR-HAPI-006 through BR-HAPI-010
"""

import structlog
from fastapi import APIRouter, HTTPException, Depends, Request

from models.api_models import ChatRequest, ChatResponse
from services.holmesgpt_service import HolmesGPTService
from services.context_api_service import ContextAPIService
from services.auth_service import User
from api.routes.auth import get_current_active_user

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


@router.post("/chat", response_model=ChatResponse)
async def interactive_chat(
    request: ChatRequest,
    current_user: User = Depends(get_current_active_user),  # BR-HAPI-007: Requires authentication
    holmes_service: HolmesGPTService = Depends(get_holmesgpt_service),
    context_service: ContextAPIService = Depends(get_context_service)
) -> ChatResponse:
    """
    Interactive Chat Endpoint - BR-HAPI-006 to BR-HAPI-010

    Maintains conversation context and supports streaming responses.
    """
    logger.info("💬 Chat request",
               session_id=request.session_id,
               message_length=len(request.message))

    try:
        # Get enhanced context if needed - BR-HAPI-011
        context = None
        if request.include_context:
            context = await context_service.get_current_context(
                namespace=request.namespace,
                include_metrics=request.include_metrics
            )

        # Process chat message - BR-HAPI-007, BR-HAPI-008
        chat_response = await holmes_service.process_chat(
            message=request.message,
            session_id=request.session_id,
            context=context,
            stream=request.stream
        )

        logger.info("✅ Chat response generated",
                   session_id=request.session_id,
                   response_length=len(chat_response.response))

        return chat_response

    except Exception as e:
        logger.error("❌ Chat processing failed",
                    session_id=request.session_id,
                    error=str(e), exc_info=True)
        raise HTTPException(
            status_code=500,
            detail=f"Chat processing failed: {str(e)}"
        )
