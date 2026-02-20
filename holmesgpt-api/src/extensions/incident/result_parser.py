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
Incident Analysis Result Parser

Business Requirements:
- BR-HAPI-002 (Incident Analysis)
- BR-HAPI-197 (needs_human_review field)
- BR-HAPI-200 (Investigation inconclusive outcome)

Design Decisions:
- DD-HAPI-002 v1.2 (Workflow Response Validation with self-correction)
- DD-WORKFLOW-001 v1.7 (OwnerChain validation)

This module handles parsing and validation of HolmesGPT investigation results,
including JSON extraction, workflow validation, and human review determination.
"""

import json
import re
import logging
from typing import Dict, Any, List, Optional
from datetime import datetime, timezone

# HolmesGPT SDK imports
from holmes.core.models import InvestigationResult

logger = logging.getLogger(__name__)


def parse_and_validate_investigation_result(
    investigation: InvestigationResult,
    request_data: Dict[str, Any],
    data_storage_client=None
):
    """
    Parse and validate HolmesGPT investigation result.

    DD-HAPI-002 v1.2: Returns both the parsed result AND the validation result
    so the caller can decide whether to retry with error feedback.

    Args:
        investigation: HolmesGPT investigation result
        request_data: Original request data
        data_storage_client: Data Storage client for workflow validation

    Returns:
        Tuple of (result_dict, validation_result) where validation_result is None
        if no workflow to validate or validation passed.
    """
    from src.validation.workflow_response_validator import WorkflowResponseValidator

    incident_id = request_data.get("incident_id", "unknown")

    # Extract analysis text
    analysis = investigation.analysis if investigation and investigation.analysis else "No analysis available"
    
    # DEBUG: Log what we received from SDK
    logger.info({
        "event": "sdk_analysis_received",
        "incident_id": incident_id,
        "analysis_length": len(analysis),
        "has_json_marker": "```json" in analysis,
        "analysis_preview": analysis[:500]
    })

    # Try to parse JSON from analysis
    # Pattern 1: JSON code block (standard format)
    # BR-HAPI-200: Use greedy match to capture FULL JSON dict, not just first {...} object
    json_match = re.search(r'```json\s*(\{.*\})\s*```', analysis, re.DOTALL)
    if json_match:
        logger.info(f"Pattern 1: Matched ```json``` block ({len(json_match.group(1))} chars)")
    else:
        logger.info("Pattern 1: Did NOT match ```json``` block")

    # BR-HAPI-200: Pattern 2A - Extract complete JSON dict (Mock LLM format with confidence, investigation_outcome)
    # Mock LLM includes the full JSON dict in the response, not just section headers
    if not json_match:
        # Look for complete JSON dict (handles Mock LLM format)
        json_dict_match = re.search(r'\{[\s\S]*?"root_cause_analysis"[\s\S]*?\}(?=\n```|\n\n|\Z)', analysis, re.DOTALL)
        if json_dict_match:
            logger.debug(f"Pattern 2A: Found complete JSON dict: {json_dict_match.group(0)[:200]}...")
            class FakeMatch:
                def __init__(self, text):
                    self._text = text
                    self.lastindex = None
                def group(self, n):
                    return self._text
            json_match = FakeMatch(json_dict_match.group(0))
            logger.info("Pattern 2A: Successfully extracted complete JSON dict")

    # Pattern 2B: Legacy - Python dict format with section headers (HolmesGPT SDK format)
    # Format: "# root_cause_analysis\n{'summary': '...', ...}\n\n# selected_workflow\n{'workflow_id': '...', ...}"
    if not json_match and ('# selected_workflow' in analysis or '# root_cause_analysis' in analysis):
        import ast
        parts = {}

        # Extract root_cause_analysis
        rca_match = re.search(r'# root_cause_analysis\s*\n\s*(\{.*?\})\s*(?:\n#|$)', analysis, re.DOTALL)
        if rca_match:
            parts['root_cause_analysis'] = rca_match.group(1)
            logger.debug(f"Pattern 2B: Extracted RCA: {parts['root_cause_analysis'][:100]}...")

        # Extract selected_workflow
        wf_match = re.search(r'# selected_workflow\s*\n\s*(\{.*?\}|None)\s*(?:\n#|$|\n\n)', analysis, re.DOTALL)
        if wf_match:
            parts['selected_workflow'] = wf_match.group(1)
            logger.debug(f"Pattern 2B: Extracted workflow: {parts['selected_workflow'][:100]}...")

        # BR-HAPI-200: Extract investigation_outcome (for problem_resolved case)
        outcome_match = re.search(r'# investigation_outcome\s*\n\s*["\']?(.*?)["\']?\s*(?:\n#|$|\n\n)', analysis, re.DOTALL)
        if outcome_match:
            parts['investigation_outcome'] = f'"{outcome_match.group(1).strip()}"'
            logger.debug(f"Pattern 2B: Extracted investigation_outcome: {parts['investigation_outcome']}")

        # BR-HAPI-200: Extract confidence (for problem_resolved case)
        conf_match = re.search(r'# confidence\s*\n\s*([\d.]+)\s*(?:\n#|$|\n\n)', analysis, re.DOTALL)
        if conf_match:
            parts['confidence'] = conf_match.group(1)
            logger.debug(f"Pattern 2B: Extracted confidence: {parts['confidence']}")
        
        # E2E-HAPI-002: Extract alternative_workflows
        alt_wf_match = re.search(r'# alternative_workflows\s*\n\s*(\[.*?\])\s*(?:\n#|$|\n\n)', analysis, re.DOTALL)
        if alt_wf_match:
            parts['alternative_workflows'] = alt_wf_match.group(1)
            logger.debug(f"Pattern 2B: Extracted alternative_workflows: {parts['alternative_workflows'][:100]}...")
        
        # E2E-HAPI-003: Extract needs_human_review
        nhr_match = re.search(r'# needs_human_review\s*\n\s*(True|False|true|false)\s*(?:\n#|$|\n\n)', analysis, re.IGNORECASE)
        if nhr_match:
            parts['needs_human_review'] = nhr_match.group(1)
            logger.debug(f"Pattern 2B: Extracted needs_human_review: {parts['needs_human_review']}")
        
        # E2E-HAPI-003: Extract human_review_reason
        hrr_match = re.search(r'# human_review_reason\s*\n\s*["\']?([^"\'\n]+)["\']?\s*(?:\n#|$|\n\n)', analysis)
        if hrr_match:
            parts['human_review_reason'] = f'"{hrr_match.group(1)}"'
            logger.debug(f"Pattern 2B: Extracted human_review_reason: {parts['human_review_reason']}")
        
        # E2E-HAPI-003: Extract validation_attempts_history
        vah_match = re.search(r'# validation_attempts_history\s*\n\s*(\[.*?\])\s*(?:\n#|$|\n\n)', analysis, re.DOTALL)
        if vah_match:
            parts['validation_attempts_history'] = vah_match.group(1)
            logger.debug(f"Pattern 2B: Extracted validation_attempts_history")

        if parts:
            # Combine into a single dict string
            combined_dict = '{'
            for key, value in parts.items():
                combined_dict += f'"{key}": {value}, '
            combined_dict = combined_dict.rstrip(', ') + '}'
            logger.debug(f"Pattern 2B: Combined dict: {combined_dict[:200]}...")

            # Create a fake match object
            class FakeMatch:
                def __init__(self, text):
                    self._text = text
                    self.lastindex = None
                def group(self, n):
                    return self._text

            json_match = FakeMatch(combined_dict)
            logger.info("Pattern 2B: Successfully created FakeMatch for SDK format (legacy)")

    alternative_workflows = []
    selected_workflow = None
    rca = {"summary": "No structured RCA found", "severity": "unknown", "contributing_factors": []}
    confidence = 0.0
    validation_result = None
    json_data = None  # BR-HAPI-200: Initialize for investigation_outcome check

    if json_match:
        try:
            # Handle both regular match objects and FakeMatch
            json_text = json_match.group(1) if hasattr(json_match, 'lastindex') and json_match.lastindex else json_match.group(0)

            # Try parsing as JSON first
            try:
                json_data = json.loads(json_text)
            except json.JSONDecodeError:
                # Fallback: Try ast.literal_eval for Python dict strings
                import ast
                json_data = ast.literal_eval(json_text)
                logger.debug("Successfully parsed Python dict using ast.literal_eval")

            rca = json_data.get("root_cause_analysis", {})
            selected_workflow = json_data.get("selected_workflow")
            # BR-HAPI-200: Extract confidence from top-level JSON first (for problem_resolved case)
            # Fall back to selected_workflow.confidence for backward compatibility
            confidence = json_data.get("confidence", selected_workflow.get("confidence", 0.0) if selected_workflow else 0.0)

            # Extract alternative workflows (ADR-045 v1.2 - for audit/context only)
            raw_alternatives = json_data.get("alternative_workflows", [])
            logger.info({
                "event": "alternative_workflows_extraction",
                "incident_id": incident_id,
                "raw_alternatives_count": len(raw_alternatives),
                "raw_alternatives": raw_alternatives
            })
            for alt in raw_alternatives:
                if isinstance(alt, dict) and alt.get("workflow_id"):
                    # E2E-HAPI-002: rationale is required by Pydantic, provide non-empty default
                    rationale = alt.get("rationale") or "Alternative workflow option"
                    alternative_workflows.append({
                        "workflow_id": alt.get("workflow_id", ""),
                        "execution_bundle": alt.get("execution_bundle"),
                        "confidence": float(alt.get("confidence", 0.0)),
                        "rationale": rationale
                    })
            logger.info({
                "event": "alternative_workflows_parsed",
                "incident_id": incident_id,
                "parsed_count": len(alternative_workflows)
            })

            # DD-HAPI-002 v1.2, DD-WORKFLOW-017: Workflow Response Validation
            if selected_workflow and data_storage_client:
                validator = WorkflowResponseValidator(data_storage_client)
                validation_result = validator.validate(
                    workflow_id=selected_workflow.get("workflow_id", ""),
                    execution_bundle=selected_workflow.get("execution_bundle"),
                    parameters=selected_workflow.get("parameters", {})
                )
                if validation_result.is_valid:
                    if validation_result.validated_execution_bundle:
                        selected_workflow["execution_bundle"] = validation_result.validated_execution_bundle
                    validation_result = None  # Clear to indicate success

        except (json.JSONDecodeError, ValueError, SyntaxError) as e:
            logger.warning({
                "event": "parse_error",
                "error": str(e),
                "error_type": type(e).__name__,
                "incident_id": incident_id
            })
            rca = {"summary": "Failed to parse RCA", "severity": "unknown", "contributing_factors": []}

    # ADR-055: owner_chain validation removed. target_in_owner_chain is superseded
    # by affected_resource in Rego policy input (BR-AI-085, FR-AI-085-005).
    warnings: List[str] = []

    # Extract RCA target for affectedResource validation
    rca_target = rca.get("affectedResource") or rca.get("affected_resource")

    # E2E-HAPI-003: Check if LLM explicitly set human review fields
    needs_human_review_from_llm = json_data.get("needs_human_review") if json_data else None
    human_review_reason_from_llm = json_data.get("human_review_reason") if json_data else None
    
    # DEBUG: Log what we extracted
    logger.info({
        "event": "llm_value_extraction",
        "incident_id": incident_id,
        "needs_human_review_from_llm": needs_human_review_from_llm,
        "human_review_reason_from_llm": human_review_reason_from_llm,
        "json_data_keys": list(json_data.keys()) if json_data else None
    })

    # E2E-HAPI-003: Track if LLM explicitly provided these values to prevent override later
    llm_provided_human_review = (needs_human_review_from_llm is not None or 
                                   human_review_reason_from_llm is not None)
    
    # Initialize with LLM values if provided, otherwise use defaults
    if needs_human_review_from_llm is not None:
        needs_human_review = bool(needs_human_review_from_llm)
        human_review_reason = human_review_reason_from_llm if needs_human_review else None
        logger.info({
            "event": "human_review_from_llm",
            "incident_id": incident_id,
            "needs_human_review": needs_human_review,
            "human_review_reason": human_review_reason,
            "llm_provided": True
        })
    elif human_review_reason_from_llm is not None:
        # E2E-HAPI-003: LLM provided reason without needs flag
        needs_human_review = True
        human_review_reason = human_review_reason_from_llm
        logger.info({
            "event": "human_review_reason_from_llm_only",
            "incident_id": incident_id,
            "human_review_reason": human_review_reason,
            "llm_provided": True
        })
    else:
        # BR-HAPI-197: Default - HAPI doesn't set human review based on confidence
        # Will be overridden by investigation outcome checks below
        needs_human_review = False
        human_review_reason = None

    # BR-HAPI-200: Handle special investigation outcomes
    investigation_outcome = json_data.get("investigation_outcome") if json_data else None

    # BR-HAPI-200: Outcome A - Problem self-resolved (high confidence, no workflow needed)
    if investigation_outcome == "resolved":
        warnings.append("Problem self-resolved - no remediation required")
        # Don't override if LLM already set these
        if needs_human_review_from_llm is None:
            needs_human_review = False
            human_review_reason = None
        logger.info({
            "event": "problem_self_resolved",
            "incident_id": incident_id,
            "confidence": confidence,
            "message": "BR-HAPI-200: Investigation confirmed problem has resolved"
        })
    # BR-HAPI-200: Outcome B - Investigation inconclusive (human review required)
    elif investigation_outcome == "inconclusive":
        warnings.append("Investigation inconclusive - human review recommended")
        # Don't override if LLM already set these
        if needs_human_review_from_llm is None:
            needs_human_review = True
            human_review_reason = "investigation_inconclusive"
        logger.warning({
            "event": "investigation_inconclusive",
            "incident_id": incident_id,
            "confidence": confidence,
            "message": "BR-HAPI-200: Investigation could not determine root cause"
        })
    elif selected_workflow is None:
        warnings.append("No workflows matched the search criteria")
        # E2E-HAPI-003: Only override if LLM didn't explicitly provide human review values
        if not llm_provided_human_review:
            needs_human_review = True
            human_review_reason = "no_matching_workflows"
            logger.info("E2E-HAPI-003: Using default no_matching_workflows (LLM didn't provide)")
        else:
            logger.info(f"E2E-HAPI-003: Preserving LLM-provided values - needs_human_review={needs_human_review}, reason={human_review_reason}")
    # BR-HAPI-197: Confidence threshold enforcement is AIAnalysis's responsibility, not HAPI's
    # HAPI only sets needs_human_review for validation failures, not confidence thresholds
    # AIAnalysis will apply the 70% threshold (V1.0) or configurable rules (V1.1, BR-HAPI-198)
    # BR-HAPI-212: Validate affectedResource is present when workflow selected
    # This check must happen AFTER problem_resolved check (workflow not needed if resolved)
    elif selected_workflow is not None and not rca_target:
        warnings.append("RCA is missing affectedResource field - cannot determine target for remediation")
        # E2E-HAPI-003: Only override if LLM didn't provide values
        if not llm_provided_human_review:
            needs_human_review = True
            human_review_reason = "rca_incomplete"
        logger.warning({
            "event": "rca_incomplete_missing_affected_resource",
            "incident_id": incident_id,
            "selected_workflow_id": selected_workflow.get("workflow_id") if selected_workflow else None,
            "message": "BR-HAPI-212: Workflow selected but affectedResource missing from RCA"
        })

    # E2E-HAPI-003: Extract validation_attempts_history from LLM if provided
    validation_attempts_from_llm = json_data.get("validation_attempts_history") if json_data else None
    logger.info({
        "event": "validation_attempts_extraction",
        "incident_id": incident_id,
        "from_llm": validation_attempts_from_llm is not None,
        "count": len(validation_attempts_from_llm) if validation_attempts_from_llm else 0,
        "type": type(validation_attempts_from_llm).__name__ if validation_attempts_from_llm else "None"
    })
    
    result = {
        "incident_id": incident_id,
        "analysis": analysis,
        "root_cause_analysis": rca,
        "confidence": confidence,
        "timestamp": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "warnings": warnings,
        "needs_human_review": needs_human_review,
    }
    
    # E2E-HAPI-002/003: Only include optional fields if they have values
    # This ensures Pydantic Optional fields have Set=false when not provided
    if selected_workflow is not None:
        result["selected_workflow"] = selected_workflow
    if human_review_reason is not None:
        result["human_review_reason"] = human_review_reason
    # BR-AUDIT-005 Gap #4: Always include alternative_workflows for audit trail (even if empty)
    # ADR-045 v1.2: Required for SOC2 compliance and RR reconstruction
    result["alternative_workflows"] = alternative_workflows
    # E2E-HAPI-003: Include LLM-provided validation history (for max_retries_exhausted simulation)
    if validation_attempts_from_llm:
        result["validation_attempts_history"] = validation_attempts_from_llm
        logger.info({
            "event": "validation_attempts_added_to_result",
            "incident_id": incident_id,
            "count": len(validation_attempts_from_llm)
        })

    return result, validation_result


def determine_human_review_reason(errors: List[str]) -> str:
    """
    Determine the human_review_reason based on validation errors.

    BR-HAPI-197: Map validation errors to structured reason enum.

    Args:
        errors: List of validation error messages

    Returns:
        HumanReviewReason enum value as string
    """
    error_text = " ".join(errors).lower()

    if "not found" in error_text and "catalog" in error_text:
        return "workflow_not_found"
    elif "mismatch" in error_text or "image" in error_text:
        return "image_mismatch"
    elif "parameter" in error_text or "required" in error_text or "type" in error_text:
        return "parameter_validation_failed"
    else:
        return "parameter_validation_failed"  # Default for validation errors


def parse_investigation_result(
    investigation: InvestigationResult,
    request_data: Dict[str, Any],
    data_storage_client=None
) -> Dict[str, Any]:
    """
    Parse HolmesGPT investigation result into IncidentResponse format.

    DEPRECATED: Use parse_and_validate_investigation_result for self-correction loop.

    Business Requirement: BR-HAPI-002 (Incident analysis response schema)
    Design Decision: DD-HAPI-002 v1.2 (Workflow Response Validation)

    Args:
        investigation: HolmesGPT investigation result
        request_data: Original request data
        data_storage_client: Optional Data Storage client for workflow validation (DD-HAPI-002 v1.2)
    """
    incident_id = request_data.get("incident_id", "unknown")

    # Extract analysis text
    analysis = investigation.analysis if investigation and investigation.analysis else "No analysis available"

    # Try to parse JSON from analysis
    # Pattern 1: JSON code block (standard format)
    # BR-HAPI-200: Use greedy match to capture FULL JSON dict, not just first {...} object
    json_match = re.search(r'```json\s*(\{.*\})\s*```', analysis, re.DOTALL)
    if json_match:
        logger.info(f"Pattern 1: Matched ```json``` block ({len(json_match.group(1))} chars)")
    else:
        logger.info("Pattern 1: Did NOT match ```json``` block")

    # BR-HAPI-200: Pattern 2A - Extract complete JSON dict (Mock LLM format with confidence, investigation_outcome)
    # Mock LLM includes the full JSON dict in the response, not just section headers
    if not json_match:
        # Look for complete JSON dict (handles Mock LLM format)
        json_dict_match = re.search(r'\{[\s\S]*?"root_cause_analysis"[\s\S]*?\}(?=\n```|\n\n|\Z)', analysis, re.DOTALL)
        if json_dict_match:
            logger.debug(f"Pattern 2A: Found complete JSON dict: {json_dict_match.group(0)[:200]}...")
            class FakeMatch:
                def __init__(self, text):
                    self._text = text
                    self.lastindex = None
                def group(self, n):
                    return self._text
            json_match = FakeMatch(json_dict_match.group(0))
            logger.info("Pattern 2A: Successfully extracted complete JSON dict")

    # Pattern 2B: Legacy - Python dict format with section headers (HolmesGPT SDK format)
    # Format: "# root_cause_analysis\n{'summary': '...', ...}\n\n# selected_workflow\n{'workflow_id': '...', ...}"
    if not json_match and ('# selected_workflow' in analysis or '# root_cause_analysis' in analysis):
        import ast
        parts = {}

        # Extract root_cause_analysis
        rca_match = re.search(r'# root_cause_analysis\s*\n\s*(\{.*?\})\s*(?:\n#|$)', analysis, re.DOTALL)
        if rca_match:
            parts['root_cause_analysis'] = rca_match.group(1)
            logger.debug(f"Pattern 2B: Extracted RCA: {parts['root_cause_analysis'][:100]}...")

        # Extract selected_workflow
        wf_match = re.search(r'# selected_workflow\s*\n\s*(\{.*?\}|None)\s*(?:\n#|$|\n\n)', analysis, re.DOTALL)
        if wf_match:
            parts['selected_workflow'] = wf_match.group(1)
            logger.debug(f"Pattern 2B: Extracted workflow: {parts['selected_workflow'][:100]}...")

        # BR-HAPI-200: Extract investigation_outcome (for problem_resolved case)
        outcome_match = re.search(r'# investigation_outcome\s*\n\s*["\']?(.*?)["\']?\s*(?:\n#|$|\n\n)', analysis, re.DOTALL)
        if outcome_match:
            parts['investigation_outcome'] = f'"{outcome_match.group(1).strip()}"'
            logger.debug(f"Pattern 2B: Extracted investigation_outcome: {parts['investigation_outcome']}")

        # BR-HAPI-200: Extract confidence (for problem_resolved case)
        conf_match = re.search(r'# confidence\s*\n\s*([\d.]+)\s*(?:\n#|$|\n\n)', analysis, re.DOTALL)
        if conf_match:
            parts['confidence'] = conf_match.group(1)
            logger.debug(f"Pattern 2B: Extracted confidence: {parts['confidence']}")

        if parts:
            # Combine into a single dict string
            combined_dict = '{'
            for key, value in parts.items():
                combined_dict += f'"{key}": {value}, '
            combined_dict = combined_dict.rstrip(', ') + '}'
            logger.debug(f"Pattern 2B: Combined dict: {combined_dict[:200]}...")

            # Create a fake match object
            class FakeMatch:
                def __init__(self, text):
                    self._text = text
                    self.lastindex = None
                def group(self, n):
                    return self._text

            json_match = FakeMatch(combined_dict)
            logger.info("Pattern 2B: Successfully created FakeMatch for SDK format (legacy)")

    alternative_workflows = []
    if json_match:
        try:
            # Handle both regular match objects and FakeMatch
            json_text = json_match.group(1) if hasattr(json_match, 'lastindex') and json_match.lastindex else json_match.group(0)

            # Try parsing as JSON first
            try:
                json_data = json.loads(json_text)
            except json.JSONDecodeError:
                # Fallback: Try ast.literal_eval for Python dict strings
                import ast
                try:
                    json_data = ast.literal_eval(json_text)
                except (ValueError, SyntaxError) as e:
                    logger.error({
                        "event": "parse_error",
                        "error": str(e),
                        "json_text_preview": json_text[:200] if json_text else ""
                    })
                    raise  # Re-raise to be caught by outer exception handler
            rca = json_data.get("root_cause_analysis", {})
            selected_workflow = json_data.get("selected_workflow")
            # BR-HAPI-200: Extract confidence from top-level JSON first (for problem_resolved case)
            # Fall back to selected_workflow.confidence for backward compatibility
            confidence = json_data.get("confidence", selected_workflow.get("confidence", 0.0) if selected_workflow else 0.0)

            # Extract alternative workflows (ADR-045 v1.2 - for audit/context only)
            raw_alternatives = json_data.get("alternative_workflows", [])
            logger.info({
                "event": "alternative_workflows_extraction",
                "incident_id": incident_id,
                "raw_alternatives_count": len(raw_alternatives),
                "raw_alternatives": raw_alternatives
            })
            for alt in raw_alternatives:
                if isinstance(alt, dict) and alt.get("workflow_id"):
                    # E2E-HAPI-002: rationale is required by Pydantic, provide non-empty default
                    rationale = alt.get("rationale") or "Alternative workflow option"
                    alternative_workflows.append({
                        "workflow_id": alt.get("workflow_id", ""),
                        "execution_bundle": alt.get("execution_bundle"),
                        "confidence": float(alt.get("confidence", 0.0)),
                        "rationale": rationale
                    })
            logger.info({
                "event": "alternative_workflows_parsed",
                "incident_id": incident_id,
                "parsed_count": len(alternative_workflows)
            })

            # DD-HAPI-002 v1.2, DD-WORKFLOW-017: Workflow Response Validation
            if selected_workflow and data_storage_client:
                from src.validation.workflow_response_validator import WorkflowResponseValidator
                validator = WorkflowResponseValidator(data_storage_client)
                validation_result = validator.validate(
                    workflow_id=selected_workflow.get("workflow_id", ""),
                    execution_bundle=selected_workflow.get("execution_bundle"),
                    parameters=selected_workflow.get("parameters", {})
                )
                if not validation_result.is_valid:
                    workflow_validation_failed = True
                    workflow_validation_errors = validation_result.errors
                    logger.warning({
                        "event": "workflow_validation_failed",
                        "incident_id": request_data.get("incident_id", "unknown"),
                        "workflow_id": selected_workflow.get("workflow_id"),
                        "errors": validation_result.errors,
                        "message": "DD-HAPI-002 v1.2: Workflow response validation failed"
                    })
                    selected_workflow["validation_errors"] = validation_result.errors
                else:
                    if validation_result.validated_execution_bundle:
                        selected_workflow["execution_bundle"] = validation_result.validated_execution_bundle
                        logger.debug({
                            "event": "workflow_validation_passed",
                            "incident_id": request_data.get("incident_id", "unknown"),
                            "workflow_id": selected_workflow.get("workflow_id"),
                            "execution_bundle": validation_result.validated_execution_bundle
                        })
        except json.JSONDecodeError:
            rca = {"summary": "Failed to parse RCA", "severity": "unknown", "contributing_factors": []}
            selected_workflow = None
            confidence = 0.0
    else:
        rca = {"summary": "No structured RCA found", "severity": "unknown", "contributing_factors": []}
        selected_workflow = None
        confidence = 0.0

    # ADR-055: owner_chain validation removed. target_in_owner_chain is superseded
    # by affected_resource in Rego policy input (BR-AI-085, FR-AI-085-005).
    warnings: List[str] = []

    # DD-HAPI-002 v1.2: Workflow validation tracking
    workflow_validation_failed = False
    workflow_validation_errors: List[str] = []

    # Generate warnings for other conditions (BR-HAPI-197, BR-HAPI-200)
    # E2E-HAPI-003: Extract LLM-provided human review fields first
    needs_human_review_from_llm = None
    human_review_reason_from_llm = None
    investigation_outcome = None
    if json_match:
        try:
            json_data_outcome = json.loads(json_match.group(1))
            investigation_outcome = json_data_outcome.get("investigation_outcome")
            needs_human_review_from_llm = json_data_outcome.get("needs_human_review")
            human_review_reason_from_llm = json_data_outcome.get("human_review_reason")
        except json.JSONDecodeError:
            pass
    
    # Initialize with LLM values if provided, otherwise use defaults
    if needs_human_review_from_llm is not None:
        needs_human_review = bool(needs_human_review_from_llm)
        human_review_reason = human_review_reason_from_llm if needs_human_review else None
    else:
        needs_human_review = False
        human_review_reason = None

    # BR-HAPI-200: Handle special investigation outcomes

    # BR-HAPI-200: Outcome A - Problem self-resolved (high confidence, no workflow needed)
    if investigation_outcome == "resolved":
        warnings.append("Problem self-resolved - no remediation required")
        # Don't override if LLM already set these
        if needs_human_review_from_llm is None:
            needs_human_review = False
            human_review_reason = None
        logger.info({
            "event": "problem_self_resolved",
            "incident_id": incident_id,
            "confidence": confidence,
            "message": "BR-HAPI-200: Investigation confirmed problem has resolved"
        })
    # BR-HAPI-200: Outcome B - Investigation inconclusive (human review required)
    elif investigation_outcome == "inconclusive":
        warnings.append("Investigation inconclusive - human review recommended")
        # Don't override if LLM already set these
        if needs_human_review_from_llm is None:
            needs_human_review = True
            human_review_reason = "investigation_inconclusive"
        logger.warning({
            "event": "investigation_inconclusive",
            "incident_id": incident_id,
            "confidence": confidence,
            "message": "BR-HAPI-200: Investigation could not determine root cause"
        })
    elif selected_workflow is None:
        warnings.append("No workflows matched the search criteria")
        # E2E-HAPI-003: Only override if LLM didn't provide values
        # Check BOTH needs_human_review AND human_review_reason (LLM may provide reason without needs flag)
        if needs_human_review_from_llm is None and human_review_reason_from_llm is None:
            needs_human_review = True
            human_review_reason = "no_matching_workflows"
        elif human_review_reason_from_llm is not None:
            # LLM provided reason, use it
            needs_human_review = True
            human_review_reason = human_review_reason_from_llm
    # BR-HAPI-197: Confidence threshold enforcement is AIAnalysis's responsibility, not HAPI's
    # (Duplicate check removed here as well)

    # DD-HAPI-002 v1.2: Set needs_human_review if workflow validation failed
    if workflow_validation_failed:
        warnings.append(f"Workflow validation failed: {'; '.join(workflow_validation_errors)}")
        needs_human_review = True
        # Determine specific reason from validation errors
        error_text = " ".join(workflow_validation_errors).lower()
        if "not found in catalog" in error_text:
            human_review_reason = "workflow_not_found"
        elif "mismatch" in error_text:
            human_review_reason = "image_mismatch"
        else:
            human_review_reason = "parameter_validation_failed"

    result = {
        "incident_id": incident_id,
        "analysis": analysis,
        "root_cause_analysis": rca,
        "confidence": confidence,
        "timestamp": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "warnings": warnings,
        # DD-HAPI-002 v1.2, BR-HAPI-197: Human review flag and structured reason
        "needs_human_review": needs_human_review,
    }
    
    # E2E-HAPI-002/003: Only include optional fields if they have values
    # This ensures Pydantic Optional fields have Set=false when not provided
    if selected_workflow is not None:
        result["selected_workflow"] = selected_workflow
    if human_review_reason is not None:
        result["human_review_reason"] = human_review_reason
    # BR-AUDIT-005 Gap #4: Always include alternative_workflows for audit trail (even if empty)
    # ADR-045 v1.2: Required for SOC2 compliance and RR reconstruction
    result["alternative_workflows"] = alternative_workflows
    
    return result

