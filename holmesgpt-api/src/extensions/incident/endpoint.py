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

    Called by: AIAnalysis Controller (for initial incident RCA and workflow selection)

    Flow:
    1. Receive IncidentRequest from AIAnalysis
    2. Sanitize input for LLM (BR-HAPI-211)
    3. Call HolmesGPT SDK for investigation (BR-HAPI-002)
    4. Search workflow catalog via MCP (BR-HAPI-250)
    5. Validate workflow response (DD-HAPI-002 v1.2)
    6. Self-correct if validation fails (up to 3 attempts)
    7. Return IncidentResponse with RCA and workflow selection
    """
    request_data = request.model_dump() if hasattr(request, 'model_dump') else request.dict()
    result = await analyze_incident(request_data)
    return result





