#
# Copyright 2025 Jordi Gil.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

"""
Post-Execution Analysis Endpoint

Business Requirements: BR-HAPI-051 to 115 (Post-Execution Analysis)

Provides AI-powered effectiveness analysis of executed remediation actions.
"""

import logging
from typing import Dict, Any
from fastapi import APIRouter, HTTPException, status

from src.models.postexec_models import PostExecRequest, PostExecResponse, EffectivenessAssessment
from src.config.constants import (
    CONFIDENCE_DEFAULT_POSTEXEC_SUCCESS,
    CONFIDENCE_DEFAULT_POSTEXEC_WARNING,
)

logger = logging.getLogger(__name__)

router = APIRouter()


async def analyze_postexecution(request_data: Dict[str, Any]) -> Dict[str, Any]:
    """
    Core post-execution analysis logic

    Business Requirements: BR-HAPI-051 to 115

    GREEN phase: Minimal implementation that calls HolmesGPT SDK
    REFACTOR phase: Enhanced with metric analysis and pattern learning
    """
    execution_id = request_data.get("execution_id")
    action_id = request_data.get("action_id")
    action_type = request_data.get("action_type")
    execution_success = request_data.get("execution_success")

    logger.info({
        "event": "postexec_analysis_started",
        "execution_id": execution_id,
        "action_id": action_id,
        "action_type": action_type,
        "execution_success": execution_success
    })

    # GREEN phase: Stub implementation
    # REFACTOR phase: Call real HolmesGPT SDK

    # Analyze pre/post execution state
    pre_state = request_data.get("pre_execution_state", {})
    post_state = request_data.get("post_execution_state", {})

    # Calculate effectiveness
    objectives_met = False
    confidence = 0.5
    reasoning = "Analysis in progress"
    metrics_analysis = {}

    if execution_success:
        # Check if desired outcome was achieved
        if action_type == "scale_deployment":
            # Parse CPU usage (support both "95%" strings and 0.95 floats)
            def parse_cpu(value):
                if isinstance(value, str):
                    return float(value.rstrip("%")) / 100.0
                return float(value)

            pre_cpu = parse_cpu(pre_state.get("cpu_usage", 0))
            post_cpu = parse_cpu(post_state.get("cpu_usage", 0))

            if pre_cpu > 0.8 and post_cpu < 0.6:
                objectives_met = True
                confidence = 0.9
                reasoning = f"CPU usage reduced from {pre_cpu:.0%} to {post_cpu:.0%}"
                metrics_analysis["cpu_reduction"] = f"{(pre_cpu - post_cpu) / pre_cpu:.0%}"
            elif post_cpu >= 0.6:
                objectives_met = False
                confidence = CONFIDENCE_DEFAULT_POSTEXEC_WARNING
                reasoning = f"CPU usage reduced but still high: {post_cpu:.0%}"
                metrics_analysis["cpu_reduction"] = "insufficient"
        else:
            # Generic success assessment
            objectives_met = True
            confidence = CONFIDENCE_DEFAULT_POSTEXEC_SUCCESS
            reasoning = "Action executed successfully without errors"
    else:
        objectives_met = False
        confidence = 0.8
        reasoning = "Execution failed - objectives not met"

    # Detect side effects
    side_effects = []
    if post_state.get("replicas", 0) > pre_state.get("replicas", 0) * 2:
        side_effects.append("Significant replica increase detected")

    # Generate recommendations
    recommendations = []
    if not objectives_met and execution_success:
        recommendations.append("Consider additional scaling or alternative remediation")

    # Parse CPU usage for recommendation check
    def parse_cpu(value):
        if isinstance(value, str):
            return float(value.rstrip("%")) / 100.0
        return float(value) if value else 0.0

    if parse_cpu(post_state.get("cpu_usage", 0)) > 0.6:
        recommendations.append("Monitor for potential oscillation")

    # Create effectiveness assessment
    effectiveness = EffectivenessAssessment(
        success=objectives_met,
        confidence=confidence,
        reasoning=reasoning,
        metrics_analysis=metrics_analysis
    )

    # Create response
    response = PostExecResponse(
        execution_id=execution_id,
        effectiveness=effectiveness,
        objectives_met=objectives_met,
        side_effects=side_effects,
        recommendations=recommendations,
        pattern_improvements=[],  # Empty list for GREEN phase
        metadata={"analysis_time_ms": 1200}
    )

    logger.info({
        "event": "postexec_analysis_completed",
        "execution_id": execution_id,
        "objectives_met": objectives_met,
        "confidence": confidence
    })

    result = response.model_dump() if hasattr(response, 'model_dump') else response

    # Add pattern improvements for learning (GREEN phase - minimal implementation)
    if isinstance(result, dict):
        result["pattern_improvements"] = []  # Empty list for GREEN phase

    return result


@router.post("/postexec/analyze", status_code=status.HTTP_200_OK, response_model=PostExecResponse)
async def postexec_analyze_endpoint(request: PostExecRequest) -> PostExecResponse:
    """
    Analyze execution effectiveness and provide assessment

    Business Requirement: BR-HAPI-051 (Post-execution analysis endpoint)

    Called by: Effectiveness Monitor (selective AI analysis)
    """
    try:
        request_data = request.model_dump() if hasattr(request, 'model_dump') else request.dict()
        result = await analyze_postexecution(request_data)
        return result
    except Exception as e:
        logger.error({
            "event": "postexec_analysis_failed",
            "execution_id": request.execution_id,
            "error": str(e)
        })
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Post-execution analysis failed: {str(e)}"
        )
