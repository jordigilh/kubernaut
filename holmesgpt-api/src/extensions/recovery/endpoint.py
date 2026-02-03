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
Recovery Analysis FastAPI Endpoint

Business Requirements: BR-HAPI-001 to 050 (Recovery Analysis)
Design Decision: DD-RECOVERY-003 (Recovery API Endpoint)

This module contains the FastAPI router and endpoint for recovery analysis.
"""

import logging
from typing import Dict, Any
from fastapi import APIRouter, HTTPException, Request, status
from src.models.recovery_models import RecoveryRequest, RecoveryResponse
from .llm_integration import analyze_recovery
from src.middleware.user_context import get_authenticated_user  # DD-AUTH-006
from src.metrics import get_global_metrics  # BR-HAPI-011, BR-HAPI-301

logger = logging.getLogger(__name__)

router = APIRouter()


@router.post(
    "/recovery/analyze",
    status_code=status.HTTP_200_OK,
    response_model=RecoveryResponse,
    response_model_exclude_none=True,  # E2E-HAPI-023/024: Exclude None values (selected_workflow, alternative_workflows)
    responses={
        200: {"description": "Successful Response - Recovery analyzed with workflow selection"},
        400: {"description": "Bad Request - Invalid input format or missing required fields"},
        401: {"description": "Unauthorized - Missing or invalid authentication token"},
        403: {"description": "Forbidden - Insufficient permissions (SAR check failed)"},
        422: {"description": "Validation Error - Request body validation failed"},
        500: {"description": "Internal Server Error - LLM or workflow catalog failure"}
    }
)
async def recovery_analyze_endpoint(recovery_req: RecoveryRequest, request: Request) -> RecoveryResponse:
    """
    Analyze failed action and provide recovery strategies

    Business Requirement: BR-HAPI-001 (Recovery analysis endpoint)
    Design Decision: DD-WORKFLOW-002 v2.4 - WorkflowCatalogToolset via SDK
    Design Decision: DD-AUTH-006 (User attribution for LLM cost tracking)

    Called by: AIAnalysis Controller (for recovery attempts after workflow failure)
    """
    # DD-AUTH-006: Extract authenticated user for logging/audit
    user = get_authenticated_user(request)
    logger.info({
        "event": "recovery_analysis_requested",
        "user": user,
        "endpoint": "/recovery/analyze",
        "purpose": "LLM cost tracking and audit trail"
    })

    # BR-AI-080: Input validation (E2E-HAPI-018)
    # Validate recovery_attempt_number >= 1 when is_recovery_attempt=true
    if recovery_req.is_recovery_attempt and recovery_req.recovery_attempt_number is not None:
        if recovery_req.recovery_attempt_number < 1:
            raise HTTPException(
                status_code=400,
                detail=f"recovery_attempt_number must be >= 1, got {recovery_req.recovery_attempt_number}"
            )

    # DEBUG: Log what we receive (BR-HAPI-197 investigation)
    logger.info(f"🔍 DEBUG: Recovery request received - signal_type={recovery_req.signal_type!r}")

    request_data = recovery_req.model_dump() if hasattr(recovery_req, 'model_dump') else recovery_req.dict()

    # DEBUG: Log request_data dict
    logger.info(f"🔍 DEBUG: Request dict - signal_type={request_data.get('signal_type')!r}, "
                f"is_recovery_attempt={request_data.get('is_recovery_attempt')}, "
                f"recovery_attempt_number={request_data.get('recovery_attempt_number')}")

    # Get result from analyze_recovery (returns dict)
    # Pass app config for LLM configuration
    from src.main import config as app_config
    
    # Inject global metrics (BR-HAPI-011, BR-HAPI-301)
    metrics = get_global_metrics()
    result_dict = await analyze_recovery(request_data, app_config, metrics=metrics)

    # Convert dict to Pydantic model for type safety and validation
    # This ensures all fields are validated per BR-HAPI-002 schema
    if isinstance(result_dict, dict):
        result = RecoveryResponse(**result_dict)
    else:
        result = result_dict  # Already a model (defensive programming)

    # DEBUG: Log response (now can use attribute access)
    logger.info(f"🔍 DEBUG: Response - needs_human_review={result.needs_human_review}, "
                f"human_review_reason={result.human_review_reason!r}, "
                f"can_recover={result.can_recover}, "
                f"has_selected_workflow={result.selected_workflow is not None}")

    return result


