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

This module defines the FastAPI router and endpoint for incident analysis.
It handles HTTP request/response and delegates business logic to llm_integration module.
"""

from fastapi import APIRouter, Request, status
import logging

from src.models.incident_models import IncidentRequest, IncidentResponse
from .llm_integration import analyze_incident
from src.audit import get_audit_store, create_hapi_response_complete_event  # DD-AUDIT-005
from src.middleware.user_context import get_authenticated_user  # DD-AUTH-006

router = APIRouter()
logger = logging.getLogger(__name__)


@router.post(
    "/incident/analyze",
    status_code=status.HTTP_200_OK,
    response_model=IncidentResponse,
    response_model_exclude_unset=False  # BR-HAPI-197: Include needs_human_review fields in OpenAPI spec
)
async def incident_analyze_endpoint(incident_req: IncidentRequest, request: Request) -> IncidentResponse:
    """
    Analyze initial incident and provide RCA + workflow selection

    Business Requirement: BR-HAPI-002 (Incident analysis endpoint)
    Business Requirement: BR-WORKFLOW-001 (MCP Workflow Integration)
    Business Requirement: BR-AUDIT-005 v2.0 (Gap #4 - AI Provider Data)
    Design Decision: DD-AUDIT-005 (Hybrid Provider Data Capture)
    Design Decision: DD-AUTH-006 (User attribution for LLM cost tracking)

    Called by: AIAnalysis Controller (for initial incident RCA and workflow selection)

    Flow:
    1. Receive IncidentRequest from AIAnalysis
    2. Extract authenticated user from oauth-proxy header (DD-AUTH-006)
    3. Sanitize input for LLM (BR-HAPI-211)
    4. Call HolmesGPT SDK for investigation (BR-HAPI-002)
    5. Search workflow catalog via MCP (BR-HAPI-250)
    6. Validate workflow response (DD-HAPI-002 v1.2)
    7. Self-correct if validation fails (up to 3 attempts)
    8. Emit audit event with complete response (DD-AUDIT-005)
    9. Return IncidentResponse with RCA and workflow selection
    """
    # DD-AUTH-006: Extract authenticated user for logging/audit
    # OAuth-proxy has already validated token and performed SAR
    # This is for cost tracking, security auditing, and future SOC2 readiness
    user = get_authenticated_user(request)
    logger.info({
        "event": "incident_analysis_requested",
        "user": user,
        "endpoint": "/incident/analyze",
        "purpose": "LLM cost tracking and audit trail"
    })

    request_data = incident_req.model_dump() if hasattr(incident_req, 'model_dump') else incident_req.dict()
    result = await analyze_incident(request_data)

    # DD-AUDIT-005: Capture complete HAPI response for audit trail (provider perspective)
    # This is the AUTHORITATIVE audit event for HAPI API responses
    # AI Analysis service will emit complementary aianalysis.analysis.completed event
    # with provider_response_summary (consumer perspective + business context)
    try:
        audit_store = get_audit_store()
        logger.info(f"DD-AUDIT-005: Creating holmesgpt.response.complete event (incident_id={incident_req.incident_id}, remediation_id={incident_req.remediation_id})")
        if audit_store:
            # Convert IncidentResponse to dict for audit storage
            # BR-HAPI-212: In mock mode, result is already a dict
            if isinstance(result, dict):
                response_dict = result
            elif hasattr(result, 'model_dump'):
                response_dict = result.model_dump()
            else:
                response_dict = result.dict()

            audit_event = create_hapi_response_complete_event(
                incident_id=incident_req.incident_id,
                remediation_id=incident_req.remediation_id,
                response_data=response_dict
            )
            logger.info(f"DD-AUDIT-005: Storing holmesgpt.response.complete event (correlation_id={incident_req.remediation_id})")
            
            # AGGRESSIVE LOGGING: Check store_audit() return value
            store_result = audit_store.store_audit(audit_event)
            if store_result:
                logger.info(f"✅ DD-AUDIT-005: Event stored successfully (buffered=True, correlation_id={incident_req.remediation_id})")
            else:
                logger.warning(f"⚠️ DD-AUDIT-005: Event NOT stored (buffered=False, buffer full or shutting down, correlation_id={incident_req.remediation_id})")
            
            # Print to stderr as backup
            import sys as _sys
            print(f"🔍 HAPI AUDIT STORE: result={store_result}, correlation_id={incident_req.remediation_id}", file=_sys.stderr, flush=True)
    except Exception as e:
        # BR-AUDIT-005: Audit writes are MANDATORY, but should not block business operation
        # Log the error but allow the business operation to complete
        # Note: logger already defined at module level (line 35)
        logger.error(
            f"Failed to emit holmesgpt.response.complete audit event: {e}",
            extra={
                "incident_id": incident_req.incident_id,
                "remediation_id": incident_req.remediation_id,
                "event_type": "holmesgpt.response.complete",
                "adr": "ADR-032 §1",  # Audit writes are mandatory, but non-blocking
            },
            exc_info=True
        )

    return result





