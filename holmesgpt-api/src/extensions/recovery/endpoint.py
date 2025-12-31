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
from fastapi import APIRouter, HTTPException, status
from src.models.recovery_models import RecoveryRequest, RecoveryResponse
from .llm_integration import analyze_recovery

logger = logging.getLogger(__name__)

router = APIRouter()


@router.post(
    "/recovery/analyze",
    status_code=status.HTTP_200_OK,
    response_model=RecoveryResponse,
    response_model_exclude_unset=False  # BR-HAPI-197: Include needs_human_review fields in OpenAPI spec
)
async def recovery_analyze_endpoint(request: RecoveryRequest) -> RecoveryResponse:
    """
    Analyze failed action and provide recovery strategies

    Business Requirement: BR-HAPI-001 (Recovery analysis endpoint)
    Design Decision: DD-WORKFLOW-002 v2.4 - WorkflowCatalogToolset via SDK

    Called by: AIAnalysis Controller (for recovery attempts after workflow failure)
    """
    request_data = request.model_dump() if hasattr(request, 'model_dump') else request.dict()
    result = await analyze_recovery(request_data)
    return result


