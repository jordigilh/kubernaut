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

from fastapi import APIRouter, status

from src.models.incident_models import IncidentRequest, IncidentResponse
from .llm_integration import analyze_incident
from src.audit import get_audit_store, create_hapi_response_complete_event  # DD-AUDIT-005

router = APIRouter()


@router.post(
    "/incident/analyze",
    status_code=status.HTTP_200_OK,
    response_model=IncidentResponse,
    response_model_exclude_unset=False  # BR-HAPI-197: Include needs_human_review fields in OpenAPI spec
)
async def incident_analyze_endpoint(request: IncidentRequest) -> IncidentResponse:
    """
    Analyze initial incident and provide RCA + workflow selection

    Business Requirement: BR-HAPI-002 (Incident analysis endpoint)
    Business Requirement: BR-WORKFLOW-001 (MCP Workflow Integration)
    Business Requirement: BR-AUDIT-005 v2.0 (Gap #4 - AI Provider Data)
    Design Decision: DD-AUDIT-005 (Hybrid Provider Data Capture)

    Called by: AIAnalysis Controller (for initial incident RCA and workflow selection)

    Flow:
    1. Receive IncidentRequest from AIAnalysis
    2. Sanitize input for LLM (BR-HAPI-211)
    3. Call HolmesGPT SDK for investigation (BR-HAPI-002)
    4. Search workflow catalog via MCP (BR-HAPI-250)
    5. Validate workflow response (DD-HAPI-002 v1.2)
    6. Self-correct if validation fails (up to 3 attempts)
    7. Emit audit event with complete response (DD-AUDIT-005)
    8. Return IncidentResponse with RCA and workflow selection
    """
    request_data = request.model_dump() if hasattr(request, 'model_dump') else request.dict()
    result = await analyze_incident(request_data)

    # DD-AUDIT-005: Capture complete HAPI response for audit trail (provider perspective)
    # This is the AUTHORITATIVE audit event for HAPI API responses
    # AI Analysis service will emit complementary aianalysis.analysis.completed event
    # with provider_response_summary (consumer perspective + business context)
    try:
        audit_store = get_audit_store()
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
                incident_id=request.incident_id,
                remediation_id=request.remediation_id,
                response_data=response_dict
            )
            audit_store.store_audit(audit_event)
    except Exception as e:
        # BR-AUDIT-005: Audit writes are MANDATORY, but should not block business operation
        # Log the error but allow the business operation to complete
        import logging
        logger = logging.getLogger(__name__)
        logger.error(
            f"Failed to emit holmesgpt.response.complete audit event: {e}",
            extra={
                "incident_id": request.incident_id,
                "remediation_id": request.remediation_id,
                "event_type": "holmesgpt.response.complete",
                "adr": "ADR-032 §1",  # Audit writes are mandatory, but non-blocking
            },
            exc_info=True
        )

    return result





