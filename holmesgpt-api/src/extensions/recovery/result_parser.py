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
Recovery Analysis Result Parser

Business Requirements: BR-HAPI-001 to 050 (Recovery Analysis)
Design Decision: DD-RECOVERY-003 (Recovery Result Parsing)

This module contains all result parsing functions for recovery analysis,
including extraction of strategies, warnings, and recovery-specific fields.
"""

import re
import json
import logging
from typing import Dict, Any, List, Optional
from holmes.core.models import InvestigationResult
from src.models.recovery_models import RecoveryStrategy, RecoveryResponse

logger = logging.getLogger(__name__)

def _parse_investigation_result(investigation: InvestigationResult, request_data: Dict[str, Any]) -> Dict[str, Any]:
    """
    Parse HolmesGPT InvestigationResult into RecoveryResponse format

    Extracts recovery strategies from LLM analysis.
    Handles both incident and recovery analysis modes.

    Design Decision: DD-RECOVERY-003 - Recovery-specific parsing
    """
    incident_id = request_data.get("incident_id")
    is_recovery = request_data.get("is_recovery_attempt", False)

    # Parse LLM analysis for recovery strategies
    analysis_text = investigation.analysis or ""

    # If this is a recovery attempt, try to parse recovery-specific JSON first
    if is_recovery:
        recovery_result = _parse_recovery_specific_result(analysis_text, request_data)
        if recovery_result:
            return recovery_result

    # Fall back to standard parsing
    # Extract strategies from analysis
    strategies = _extract_strategies_from_analysis(analysis_text)

    # Determine if recovery is possible
    can_recover = len(strategies) > 0
    primary_recommendation = strategies[0].action_type if strategies else None

    # Calculate overall confidence
    analysis_confidence = max([s.confidence for s in strategies]) if strategies else 0.0

    # Extract warnings from analysis
    warnings = _extract_warnings_from_analysis(analysis_text)

    result = RecoveryResponse(
        incident_id=incident_id,
        can_recover=can_recover,
        strategies=strategies,
        primary_recommendation=primary_recommendation,
        analysis_confidence=analysis_confidence,
        warnings=warnings,
        metadata={
            "analysis_time_ms": 2000,  # GREEN phase: static value
            "tool_calls": len(investigation.tool_calls) if hasattr(investigation, 'tool_calls') and investigation.tool_calls else 0,
            "sdk_version": "holmesgpt-0.1.0",
            "is_recovery_attempt": is_recovery
        }
    )

    return result.model_dump() if hasattr(result, 'model_dump') else result.dict()


def _parse_recovery_specific_result(analysis_text: str, request_data: Dict[str, Any]) -> Optional[Dict[str, Any]]:
    """
    Parse HolmesGPT InvestigationResult into recovery response format.

    Handles recovery-specific fields: recovery_analysis, recovery_strategy

    Design Decision: DD-RECOVERY-003
    """
    incident_id = request_data.get("incident_id")
    recovery_attempt_number = request_data.get("recovery_attempt_number", 1)

    # Try to extract structured JSON from response
    json_match = re.search(r'```json\s*(.*?)\s*```', analysis_text, re.DOTALL)
    if not json_match:
        # Try to find JSON object directly
        json_match = re.search(r'\{.*"recovery_analysis".*\}', analysis_text, re.DOTALL)
        if not json_match:
            return None

    try:
        json_text = json_match.group(1) if hasattr(json_match, 'group') and json_match.lastindex else json_match.group(0)
        structured = json.loads(json_text)

        # Extract recovery-specific fields if present
        recovery_analysis = structured.get("recovery_analysis", {})
        recovery_strategy = structured.get("recovery_strategy", {})
        selected_workflow = structured.get("selected_workflow")

        # Build recovery response
        can_recover = selected_workflow is not None
        confidence = selected_workflow.get("confidence", 0.0) if selected_workflow else 0.0

        # Convert to standard RecoveryResponse format
        strategies = []
        if selected_workflow:
            strategies.append(RecoveryStrategy(
                action_type=selected_workflow.get("workflow_id", "unknown_workflow"),
                confidence=float(confidence),
                rationale=selected_workflow.get("rationale", "Recovery workflow selected based on failure analysis"),
                estimated_risk="medium",  # Default for recovery
                prerequisites=[]
            ))

        result = {
            "incident_id": incident_id,
            "is_recovery_attempt": True,
            "recovery_attempt_number": recovery_attempt_number,
            "can_recover": can_recover,
            "strategies": [s.model_dump() if hasattr(s, 'model_dump') else s.dict() for s in strategies],
            "primary_recommendation": strategies[0].action_type if strategies else None,
            "analysis_confidence": confidence,
            "warnings": [],
            "metadata": {
                "analysis_time_ms": 2000,
                "sdk_version": "holmesgpt-0.1.0",
                "is_recovery_attempt": True,
                "recovery_attempt_number": recovery_attempt_number
            },
            # Recovery-specific fields
            "recovery_analysis": recovery_analysis,
            "recovery_strategy": recovery_strategy,
            "selected_workflow": selected_workflow,
            "raw_analysis": analysis_text,
        }

        logger.info(f"Successfully parsed recovery-specific response for incident {incident_id}")
        return result

    except (json.JSONDecodeError, AttributeError, KeyError, ValueError) as e:
        logger.warning(f"Failed to parse recovery-specific JSON: {e}")
        return None


def _extract_strategies_from_analysis(analysis_text: str) -> List[RecoveryStrategy]:
    """
    Extract recovery strategies from LLM analysis text

    REFACTOR phase: Attempts to parse structured JSON output, falls back to keyword extraction
    """
    strategies = []

    # REFACTOR Phase: Try to parse structured JSON output
    try:
        # LLM may wrap JSON in markdown code blocks
        json_match = re.search(r'```(?:json)?\s*(\{.*?\})\s*```', analysis_text, re.DOTALL)
        if json_match:
            json_text = json_match.group(1)
        else:
            # Try to find JSON object directly
            json_match = re.search(r'\{.*"strategies".*\}', analysis_text, re.DOTALL)
            json_text = json_match.group(0) if json_match else None

        if json_text:
            parsed = json.loads(json_text)

            # Extract strategies from structured output
            for strategy_data in parsed.get("strategies", []):
                strategies.append(RecoveryStrategy(
                    action_type=strategy_data.get("action_type", "unknown_action"),
                    confidence=float(strategy_data.get("confidence", 0.5)),
                    rationale=strategy_data.get("rationale", "LLM analysis"),
                    estimated_risk=strategy_data.get("estimated_risk", "medium"),
                    prerequisites=strategy_data.get("prerequisites", [])
                ))

            if strategies:
                logger.info(f"Successfully parsed {len(strategies)} strategies from structured JSON")
                return strategies
    except (json.JSONDecodeError, AttributeError, KeyError, ValueError) as e:
        logger.warning(f"Failed to parse structured JSON from LLM: {e}, falling back to keyword extraction")

    # Fallback: Keyword-based extraction (backward compatible with GREEN phase)
    logger.info("Using keyword-based strategy extraction (fallback)")

    if "rollback" in analysis_text.lower():
        strategies.append(RecoveryStrategy(
            action_type="rollback_to_previous_state",
            confidence=0.8,
            rationale="LLM recommends rollback based on analysis",
            estimated_risk="low",
            prerequisites=[]
        ))

    if "scale" in analysis_text.lower() or "retry" in analysis_text.lower():
        strategies.append(RecoveryStrategy(
            action_type="retry_with_modifications",
            confidence=0.7,
            rationale="LLM suggests retry with adjustments",
            estimated_risk="medium",
            prerequisites=[]
        ))

    # If no strategies extracted, provide default
    if not strategies:
        strategies.append(RecoveryStrategy(
            action_type="manual_intervention_required",
            confidence=0.5,
            rationale="Automated recovery not recommended",
            estimated_risk="low",
            prerequisites=["human_review"]
        ))

    return strategies


def _extract_warnings_from_analysis(analysis_text: str) -> List[str]:
    """
    Extract warnings from LLM analysis

    REFACTOR phase: Attempts to parse structured JSON output, falls back to keyword extraction
    """
    warnings = []

    # REFACTOR Phase: Try to parse warnings from structured JSON
    try:
        json_match = re.search(r'```(?:json)?\s*(\{.*?\})\s*```', analysis_text, re.DOTALL)
        if json_match:
            json_text = json_match.group(1)
        else:
            json_match = re.search(r'\{.*"warnings".*\}', analysis_text, re.DOTALL)
            json_text = json_match.group(0) if json_match else None

        if json_text:
            parsed = json.loads(json_text)
            extracted_warnings = parsed.get("warnings", [])
            if extracted_warnings:
                logger.info(f"Successfully parsed {len(extracted_warnings)} warnings from structured JSON")
                return extracted_warnings
    except (json.JSONDecodeError, AttributeError, KeyError) as e:
        logger.debug(f"Failed to parse warnings from JSON: {e}, using keyword extraction")

    # Fallback: Keyword-based extraction
    if "risk" in analysis_text.lower() or "caution" in analysis_text.lower():
        warnings.append("LLM identified potential risks - review carefully")

    if "high load" in analysis_text.lower() or "resource" in analysis_text.lower():
        warnings.append("Resource constraints may affect recovery")

    return warnings


# NOTE: _get_workflow_recommendations() function REMOVED per DD-WORKFLOW-002 v2.4
# Workflow search is now handled by WorkflowCatalogToolset registered via
# register_workflow_catalog_toolset() - the LLM calls the tool during investigation.
# The old mcp_client-based workflow fetch was dead code (results stored but never used).


# REMOVED: _add_safety_validation_to_strategies()
# Reason: Dead code - never called in production API responses
# Architecture: HolmesGPT ServiceAccount is read-only, dangerous kubectl commands
#              fail at Kubernetes RBAC layer, making safety validator redundant
# Removed: December 24, 2025


