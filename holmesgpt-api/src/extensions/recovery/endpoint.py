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
Business Requirement: BR-AA-HAPI-064.9 (Recovery Session-Based Async Pattern)
Design Decision: DD-RECOVERY-003 (Recovery API Endpoint)

This module contains the FastAPI router and endpoints for recovery analysis.
All analysis is performed asynchronously via the session-based submit/poll/result pattern.
"""

import logging
from fastapi import APIRouter, BackgroundTasks, HTTPException, Request, status
from src.models.recovery_models import RecoveryRequest, RecoveryResponse
from .llm_integration import analyze_recovery
from src.middleware.user_context import get_authenticated_user  # DD-AUTH-006
from src.metrics import get_global_metrics  # BR-HAPI-011, BR-HAPI-301
from src.errors import PROBLEM_JSON_ERROR_RESPONSES  # BR-HAPI-200: Shared RFC 7807 error responses
from src.session import SessionManager, session_status_response, session_result_response
from src.session.session_manager import get_session_manager

logger = logging.getLogger(__name__)

router = APIRouter()


async def _run_recovery_investigation(session_manager: SessionManager, session_id: str, request_data: dict) -> None:
    """
    Background task that runs the recovery investigation and updates the session.

    BR-AA-HAPI-064.9: Executes recovery analysis asynchronously.
    """
    async def _investigate(data: dict) -> dict:
        from src.main import config as app_config
        metrics = get_global_metrics()
        result_dict = await analyze_recovery(data, app_config, metrics=metrics)

        # Validate via Pydantic model, then serialize excluding None fields.
        # exclude_none=True ensures optional fields like selected_workflow are ABSENT
        # from JSON (not "null"), which maps correctly to Go ogen OptNil (Set=false).
        if isinstance(result_dict, dict):
            result = RecoveryResponse(**result_dict)
            return result.model_dump(exclude_none=True)
        elif hasattr(result_dict, 'model_dump'):
            return result_dict.model_dump(exclude_none=True)
        else:
            return result_dict.dict(exclude_none=True) if hasattr(result_dict, 'dict') else result_dict

    await session_manager.run_investigation(session_id, _investigate, request_data)


@router.post(
    "/recovery/analyze",
    status_code=status.HTTP_202_ACCEPTED,
    responses=PROBLEM_JSON_ERROR_RESPONSES
)
async def recovery_analyze_endpoint(
    recovery_req: RecoveryRequest,
    background_tasks: BackgroundTasks,
    request: Request,
):
    """
    Submit recovery analysis request (async session-based pattern).

    Business Requirement: BR-HAPI-001 (Recovery analysis endpoint)
    Business Requirement: BR-AA-HAPI-064.9 (Recovery async submit)
    Design Decision: DD-AUTH-006 (User attribution for LLM cost tracking)

    Called by: AIAnalysis Controller via SubmitRecoveryInvestigation()

    Returns HTTP 202 Accepted with {"session_id": "<uuid>"}.
    The investigation runs as a background task. Poll via GET /recovery/session/{id}.
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
    if recovery_req.is_recovery_attempt and recovery_req.recovery_attempt_number is not None:
        if recovery_req.recovery_attempt_number < 1:
            raise HTTPException(
                status_code=400,
                detail=f"recovery_attempt_number must be >= 1, got {recovery_req.recovery_attempt_number}"
            )

    # mode="json" ensures all values are JSON-serializable plain types (str, not Enum)
    request_data = recovery_req.model_dump(mode="json")

    # BR-AA-HAPI-064.9: Create recovery session and return immediately
    sm = get_session_manager()
    session_id = sm.create_session("recovery", request_data)

    # Schedule recovery investigation as background task
    background_tasks.add_task(_run_recovery_investigation, sm, session_id, request_data)

    return {"session_id": session_id}


@router.get(
    "/recovery/session/{session_id}",
    status_code=status.HTTP_200_OK,
    responses={404: {"description": "Session not found (HAPI may have restarted)"}},
)
async def recovery_session_status_endpoint(session_id: str):
    """Poll recovery session status. BR-AA-HAPI-064.9."""
    return session_status_response(session_id)


@router.get(
    "/recovery/session/{session_id}/result",
    status_code=status.HTTP_200_OK,
    response_model=RecoveryResponse,
    responses={404: {"description": "Session not found"}, 409: {"description": "Session not yet completed"}},
)
async def recovery_session_result_endpoint(session_id: str):
    """Retrieve completed recovery result. BR-AA-HAPI-064.9."""
    return session_result_response(session_id)
