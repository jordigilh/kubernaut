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
Incident Analysis FastAPI Endpoint

Business Requirement: BR-HAPI-002 (Incident Analysis Endpoint)
Business Requirement: BR-AA-HAPI-064 (Session-Based Async Pattern)

This module defines the FastAPI router and endpoints for incident analysis.
All analysis is performed asynchronously via the session-based submit/poll/result pattern.
"""

from fastapi import APIRouter, BackgroundTasks, HTTPException, Request, status
import logging

from src.models.incident_models import IncidentRequest, IncidentResponse
from .llm_integration import analyze_incident
from src.audit import get_audit_store, create_aiagent_response_complete_event  # DD-AUDIT-005
from src.middleware.user_context import get_authenticated_user  # DD-AUTH-006
from src.metrics import get_global_metrics  # BR-HAPI-011, BR-HAPI-301
from src.errors import PROBLEM_JSON_ERROR_RESPONSES  # BR-HAPI-200: Shared RFC 7807 error responses
from src.session import SessionManager, session_status_response, session_result_response
from src.session.session_manager import get_session_manager

router = APIRouter()
logger = logging.getLogger(__name__)


async def _run_incident_investigation(session_manager: SessionManager, session_id: str, request_data: dict) -> None:
    """
    Background task that runs the incident investigation and updates the session.

    BR-AA-HAPI-064.1: Executes the LLM investigation asynchronously.
    DD-AUDIT-005: Emits audit event after completion.
    """
    async def _investigate(data: dict) -> dict:
        from src.main import config as app_config
        metrics = get_global_metrics()
        result = await analyze_incident(data, mcp_config=None, app_config=app_config, metrics=metrics)

        # DD-AUDIT-005: Emit audit event for the completed investigation
        try:
            audit_store = get_audit_store()
            if audit_store:
                if isinstance(result, dict):
                    response_dict = result
                elif hasattr(result, 'model_dump'):
                    response_dict = result.model_dump()
                else:
                    response_dict = result.dict()

                audit_event = create_aiagent_response_complete_event(
                    incident_id=data.get("incident_id", ""),
                    remediation_id=data.get("remediation_id", ""),
                    response_data=response_dict
                )
                audit_store.store_audit(audit_event)
        except Exception as e:
            logger.error(f"Failed to emit audit event: {e}", exc_info=True)

        return result

    await session_manager.run_investigation(session_id, _investigate, request_data)


@router.post(
    "/incident/analyze",
    status_code=status.HTTP_202_ACCEPTED,
    responses=PROBLEM_JSON_ERROR_RESPONSES
)
async def incident_analyze_endpoint(
    incident_req: IncidentRequest,
    background_tasks: BackgroundTasks,
    request: Request,
):
    """
    Submit incident analysis request (async session-based pattern).

    Business Requirement: BR-HAPI-002 (Incident analysis endpoint)
    Business Requirement: BR-AA-HAPI-064.1 (Async submit returns session ID)
    Design Decision: DD-AUTH-006 (User attribution for LLM cost tracking)

    Called by: AIAnalysis Controller via SubmitInvestigation()

    Returns HTTP 202 Accepted with {"session_id": "<uuid>"}.
    The investigation runs as a background task. Poll via GET /incident/session/{id}.
    """
    # DD-AUTH-006: Extract authenticated user for logging/audit
    user = get_authenticated_user(request)
    logger.info({
        "event": "incident_analysis_requested",
        "user": user,
        "endpoint": "/incident/analyze",
        "purpose": "LLM cost tracking and audit trail"
    })

    # BR-HAPI-200: Input validation (E2E-HAPI-008)
    if not incident_req.remediation_id or not incident_req.remediation_id.strip():
        raise HTTPException(status_code=400, detail="remediation_id is required")

    # E2E-HAPI-007: Validate signal_type is not empty or obviously invalid
    if not incident_req.signal_type or not incident_req.signal_type.strip():
        raise HTTPException(status_code=400, detail="signal_type is required and cannot be empty")
    if "INVALID_SIGNAL_TYPE" in incident_req.signal_type.upper():
        raise HTTPException(status_code=400, detail=f"signal_type '{incident_req.signal_type}' is not valid")

    # E2E-HAPI-007: Validate severity
    valid_severities = ["critical", "high", "medium", "low", "unknown"]
    if incident_req.severity and incident_req.severity.lower() not in valid_severities:
        raise HTTPException(
            status_code=400,
            detail=f"severity must be one of: {', '.join(valid_severities)}. Got: '{incident_req.severity}'"
        )

    # mode="json" ensures all values are JSON-serializable plain types (str, not Enum)
    request_data = incident_req.model_dump(mode="json")

    # BR-AA-HAPI-064.1: Create session and return immediately
    sm = get_session_manager()
    session_id = sm.create_session("incident", request_data)

    # Schedule investigation as background task
    background_tasks.add_task(_run_incident_investigation, sm, session_id, request_data)

    return {"session_id": session_id}


@router.get(
    "/incident/session/{session_id}",
    status_code=status.HTTP_200_OK,
    responses={404: {"description": "Session not found (HAPI may have restarted)"}},
)
async def incident_session_status_endpoint(session_id: str):
    """Poll session status. BR-AA-HAPI-064.2."""
    return session_status_response(session_id)


@router.get(
    "/incident/session/{session_id}/result",
    status_code=status.HTTP_200_OK,
    response_model=IncidentResponse,
    responses={404: {"description": "Session not found"}, 409: {"description": "Session not yet completed"}},
)
async def incident_session_result_endpoint(session_id: str):
    """Retrieve completed investigation result. BR-AA-HAPI-064.3."""
    return session_result_response(session_id)
