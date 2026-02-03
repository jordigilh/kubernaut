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
import ast
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

    # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
    # DEBUG: Parser Input Inspection (Option A: Understand SDK format)
    # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
    logger.info({
        "event": "parser_input_debug",
        "incident_id": incident_id,
        "analysis_text_length": len(analysis_text) if analysis_text else 0,
        "analysis_text_preview": analysis_text[:300] if analysis_text else "",
        "has_json_codeblock": "```json" in analysis_text if analysis_text else False,
        "has_section_headers": "# selected_workflow" in analysis_text if analysis_text else False,
        "has_root_cause_header": "# root_cause_analysis" in analysis_text if analysis_text else False,
        "text_type": type(analysis_text).__name__,
    })

    # Try to extract structured JSON from response
    # Pattern 1: JSON code block (standard format)
    json_match = re.search(r'```json\s*(.*?)\s*```', analysis_text, re.DOTALL)

    logger.info({
        "event": "parser_pattern1_result",
        "incident_id": incident_id,
        "pattern": "json_codeblock",
        "matched": json_match is not None,
    })

    # Pattern 2: Python dict format with section headers (HolmesGPT SDK format)
    # Format: "# field_name\n{'key': 'value', ...}" OR "# field_name\n'value'"
    # E2E-HAPI-023: Extract ALL section headers, not just root_cause_analysis and selected_workflow
    if not json_match and ('# selected_workflow' in analysis_text or '# root_cause_analysis' in analysis_text):
        # Extract the dict portions and combine them
        parts = {}

        # Extract root_cause_analysis (dict)
        rca_match = re.search(r'# root_cause_analysis\s*\n\s*(\{.*?\})\s*(?:\n#|$)', analysis_text, re.DOTALL)
        if rca_match:
            parts['root_cause_analysis'] = rca_match.group(1)

        # Extract selected_workflow (dict or None)
        wf_match = re.search(r'# selected_workflow\s*\n\s*(None|\{.*?\})\s*(?:\n#|$|\n\n)', analysis_text, re.DOTALL)
        if wf_match:
            parts['selected_workflow'] = wf_match.group(1)

        # E2E-HAPI-023: Extract investigation_outcome (string)
        outcome_match = re.search(r'# investigation_outcome\s*\n\s*["\']?([^"\'\n]+)["\']?\s*(?:\n#|$)', analysis_text)
        if outcome_match:
            parts['investigation_outcome'] = f'"{outcome_match.group(1)}"'

        # E2E-HAPI-023: Extract can_recover (boolean)
        can_recover_match = re.search(r'# can_recover\s*\n\s*(True|False|true|false)\s*(?:\n#|$)', analysis_text, re.IGNORECASE)
        if can_recover_match:
            parts['can_recover'] = can_recover_match.group(1).lower()

        # Extract confidence (float)
        confidence_match = re.search(r'# confidence\s*\n\s*([0-9.]+)\s*(?:\n#|$)', analysis_text)
        if confidence_match:
            parts['confidence'] = confidence_match.group(1)

        # Extract recovery_analysis (dict)
        recovery_analysis_match = re.search(r'# recovery_analysis\s*\n\s*(\{.*?\})\s*(?:\n#|$)', analysis_text, re.DOTALL)
        if recovery_analysis_match:
            parts['recovery_analysis'] = recovery_analysis_match.group(1)

        if parts:
            # Combine into a single dict string
            combined_dict = '{'
            for key, value in parts.items():
                combined_dict += f'"{key}": {value}, '
            combined_dict = combined_dict.rstrip(', ') + '}'

            # Create a fake match object
            class FakeMatch:
                def __init__(self, text):
                    self._text = text
                    self.lastindex = None  # Add lastindex attribute
                def group(self, n):
                    return self._text  # Always return the full text

            json_match = FakeMatch(combined_dict)

            logger.info({
                "event": "parser_pattern2_result",
                "incident_id": incident_id,
                "pattern": "section_headers",
                "matched": True,
                "parts_found": list(parts.keys()),
                "combined_dict_preview": combined_dict[:200] if combined_dict else "",
            })

    if not json_match:
        logger.info({
            "event": "parser_pattern2_result",
            "incident_id": incident_id,
            "pattern": "section_headers",
            "matched": False,
            "reason": "no_parts_extracted",
        })

    # Pattern 3: Direct JSON object (fallback)
    if not json_match:
        json_match = re.search(r'\{.*"recovery_analysis".*\}', analysis_text, re.DOTALL)

        logger.info({
            "event": "parser_pattern3_result",
            "incident_id": incident_id,
            "pattern": "direct_json",
            "matched": json_match is not None,
        })

        if not json_match:
            logger.warning({
                "event": "parser_no_match",
                "incident_id": incident_id,
                "message": "No pattern matched - returning None",
            })
            return None

    try:
        json_text = json_match.group(1) if hasattr(json_match, 'group') and json_match.lastindex else json_match.group(0)

        # Try parsing as JSON first
        try:
            structured = json.loads(json_text)
        except json.JSONDecodeError:
            # Fallback: Try parsing as Python dict literal (handles single quotes from SDK)
            # BR-HAPI-001: Handle HolmesGPT SDK Python repr() format
            try:
                structured = ast.literal_eval(json_text)
                logger.info("Successfully parsed recovery response using ast.literal_eval() fallback")
            except (ValueError, SyntaxError) as ast_err:
                logger.warning(f"Failed to parse as both JSON and Python literal: {ast_err}")
                raise

        # Extract recovery-specific fields if present
        recovery_analysis = structured.get("recovery_analysis", {})
        recovery_strategy = structured.get("recovery_strategy", {})
        selected_workflow = structured.get("selected_workflow")
        
        # BR-AI-081: If this is a recovery request but LLM didn't return recovery_analysis,
        # construct it from the RCA (LLM may not have been configured to return it)
        is_recovery = request_data.get("is_recovery_attempt", False)
        if is_recovery and not recovery_analysis:
            # Extract RCA fields to populate recovery_analysis
            rca = structured.get("root_cause_analysis", {})
            recovery_analysis = {
                "previous_attempt_assessment": {
                    "failure_understood": True,
                    "failure_reason_analysis": rca.get("summary", "Previous workflow execution failed"),
                    "state_changed": False,
                    "current_signal_type": rca.get("signal_type", request_data.get("signal_type", "Unknown"))
                }
            }
            logger.info({
                "event": "recovery_analysis_constructed",
                "incident_id": incident_id,
                "reason": "LLM response did not include recovery_analysis field",
            })

        # Extract investigation outcome to detect self-resolved issues
        investigation_outcome = structured.get("investigation_outcome")
        confidence = selected_workflow.get("confidence", 0.0) if selected_workflow else 0.0

        # BR-HAPI-197: Determine human review requirement
        # HAPI only sets needs_human_review for validation failures, not confidence thresholds
        needs_human_review = False
        human_review_reason = None

        # E2E-HAPI-023: Use LLM's can_recover value if present, otherwise calculate it
        can_recover_from_llm = structured.get("can_recover")
        
        if can_recover_from_llm is not None:
            # E2E-HAPI-023: LLM explicitly set can_recover - use that value
            can_recover = bool(can_recover_from_llm)
            logger.info({
                "event": "can_recover_from_llm",
                "incident_id": incident_id,
                "can_recover": can_recover,
                "investigation_outcome": investigation_outcome,
            })
        elif investigation_outcome == "resolved":
            # BR-HAPI-200: Problem resolved itself - no recovery needed
            needs_human_review = False
            human_review_reason = None
            can_recover = False  # No recovery action needed
        elif not selected_workflow:
            # No automated workflow available - manual recovery possible
            needs_human_review = True
            human_review_reason = "no_matching_workflows"
            can_recover = True  # Manual recovery is possible
        else:
            # Automated workflow available
            # BR-HAPI-197: Manual recovery is possible even with automated workflow if human review needed
            can_recover = True
        
        # BR-HAPI-200: Set human review flags based on investigation outcome
        if investigation_outcome == "resolved":
            needs_human_review = False
            human_review_reason = None

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
            # BR-HAPI-197: Human review fields for recovery
            "needs_human_review": needs_human_review,
            "human_review_reason": human_review_reason,
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
            # Try parsing as JSON first, fallback to Python dict literal
            try:
                parsed = json.loads(json_text)
            except json.JSONDecodeError:
                try:
                    parsed = ast.literal_eval(json_text)
                    logger.debug("Parsed strategies using ast.literal_eval() fallback")
                except (ValueError, SyntaxError):
                    parsed = {}

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
            # Try parsing as JSON first, fallback to Python dict literal
            try:
                parsed = json.loads(json_text)
            except json.JSONDecodeError:
                try:
                    parsed = ast.literal_eval(json_text)
                    logger.debug("Parsed warnings using ast.literal_eval() fallback")
                except (ValueError, SyntaxError):
                    parsed = {}

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


