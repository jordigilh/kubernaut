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


def _parse_remediation_target(rca_data: Dict[str, Any]) -> Optional[Dict[str, str]]:
    """Extract and validate remediationTarget from the RCA data.

    BR-HAPI-261 / #542: The LLM provides remediationTarget in its Phase 1 RCA
    response — the resource the workflow will operate on.
    Two valid structures:
      - Namespaced: {"kind": "...", "name": "...", "namespace": "..."}
      - Cluster:    {"kind": "...", "name": "..."}

    Returns the validated resource dict or None if absent/invalid.
    """
    raw = rca_data.get("remediationTarget")
    if raw is None:
        return None
    if not isinstance(raw, dict):
        logger.warning({"event": "invalid_remediation_target_type", "type": type(raw).__name__})
        return None
    kind = str(raw.get("kind", "")).strip()
    name = str(raw.get("name", "")).strip()
    if not kind or not name:
        logger.warning({"event": "missing_remediation_target_fields", "kind": kind, "name": name})
        return None
    result: Dict[str, str] = {"kind": kind, "name": name}
    namespace = str(raw.get("namespace", "")).strip()
    if namespace:
        result["namespace"] = namespace
    return result


def ensure_incident_response_shape(
    result: Dict[str, Any],
    incident_id: str = "unknown",
    analysis: str = "No analysis available",
) -> Dict[str, Any]:
    """Backfill missing required IncidentResponseData fields with safe defaults.

    Issue #624: Early-exit result dicts from llm_integration.py lack required
    fields (incident_id, analysis, confidence, timestamp). This normalizer
    ensures all required fields exist before audit event creation.

    The normalized dict uses snake_case field names which Pydantic v2 accepts
    via ``populate_by_name: True`` on the IncidentResponseData model.
    """
    normalized = dict(result)
    normalized.setdefault("incident_id", incident_id)
    normalized.setdefault("analysis", analysis)
    normalized.setdefault("confidence", 0.0)
    normalized.setdefault("timestamp", datetime.now(timezone.utc).isoformat())
    normalized.setdefault("needs_human_review", False)
    normalized.setdefault("root_cause_analysis", {
        "summary": "No root cause analysis available",
        "severity": "unknown",
        "contributing_factors": [],
    })
    return normalized


def _try_parse_json_value(text: str) -> Any:
    """Parse a JSON or Python-dict/list string into a Python object.

    Tries json.loads first (handles standard JSON with null, true, false),
    falls back to ast.literal_eval (handles Python syntax with None, True, False).
    Returns None on failure.
    """
    import ast as _ast
    try:
        return json.loads(text)
    except (json.JSONDecodeError, TypeError):
        pass
    try:
        return _ast.literal_eval(text)
    except (ValueError, SyntaxError):
        pass
    return None


def _parse_section_headers(analysis: str) -> Optional[Dict[str, Any]]:
    """Parse Pattern 2B section-header format directly into a Python dict.

    HolmesGPT SDK sometimes returns analysis in section-header format:
        # root_cause_analysis
        {"summary": "...", ...}
        # selected_workflow
        null
        # needs_human_review
        true

    This function extracts each section into native Python objects, avoiding
    the fragile string-concatenation + re-parsing approach that mixed JSON
    and Python literal syntax.

    Returns a populated dict if any sections were found, or None if the
    analysis text doesn't contain section headers.
    """
    from .json_utils import extract_balanced_json

    if '# selected_workflow' not in analysis and '# root_cause_analysis' not in analysis:
        return None

    result: Dict[str, Any] = {}

    # root_cause_analysis (JSON dict)
    rca_header = re.search(r'# root_cause_analysis\s*\n\s*', analysis)
    if rca_header:
        brace_pos = analysis.find('{', rca_header.end())
        if brace_pos != -1:
            balanced = extract_balanced_json(analysis, brace_pos)
            raw = balanced
            if not raw:
                m = re.search(r'# root_cause_analysis\s*\n\s*(\{.*?\})\s*(?:\n#|$)', analysis, re.DOTALL)
                raw = m.group(1) if m else None
            if raw:
                parsed = _try_parse_json_value(raw)
                if isinstance(parsed, dict):
                    result['root_cause_analysis'] = parsed
                    logger.debug(f"Pattern 2B: Extracted RCA ({len(raw)} chars)")

    # selected_workflow (JSON dict or null/None)
    wf_header = re.search(r'# selected_workflow\s*\n\s*', analysis)
    if wf_header:
        remainder = analysis[wf_header.end():].strip()
        if remainder.startswith("None") or remainder.startswith("null"):
            logger.debug("Pattern 2B: selected_workflow is None/null")
        else:
            brace_pos = analysis.find('{', wf_header.end())
            if brace_pos != -1:
                balanced = extract_balanced_json(analysis, brace_pos)
                raw = balanced
                if not raw:
                    m = re.search(r'# selected_workflow\s*\n\s*(\{.*?\})\s*(?:\n#|$|\n\n)', analysis, re.DOTALL)
                    raw = m.group(1) if m else None
                if raw:
                    parsed = _try_parse_json_value(raw)
                    if isinstance(parsed, dict):
                        result['selected_workflow'] = parsed
                        logger.debug(f"Pattern 2B: Extracted workflow ({len(raw)} chars)")

    # investigation_outcome (plain string)
    m = re.search(r'# investigation_outcome\s*\n\s*["\']?(.*?)["\']?\s*(?:\n#|$|\n\n)', analysis, re.DOTALL)
    if m:
        val = m.group(1).strip()
        if val and val not in ("None", "null"):
            result['investigation_outcome'] = val
            logger.debug(f"Pattern 2B: investigation_outcome = {val}")

    # confidence (float)
    m = re.search(r'# confidence\s*\n\s*([\d.]+)\s*(?:\n#|$|\n\n)', analysis, re.DOTALL)
    if m:
        try:
            result['confidence'] = float(m.group(1))
            logger.debug(f"Pattern 2B: confidence = {result['confidence']}")
        except ValueError:
            pass

    # alternative_workflows (JSON list)
    m = re.search(r'# alternative_workflows\s*\n\s*(\[.*?\])\s*(?:\n#|$|\n\n)', analysis, re.DOTALL)
    if m:
        parsed = _try_parse_json_value(m.group(1))
        if isinstance(parsed, list):
            result['alternative_workflows'] = parsed
            logger.debug(f"Pattern 2B: alternative_workflows ({len(parsed)} items)")

    # needs_human_review (bool)
    m = re.search(r'# needs_human_review\s*\n\s*(True|False|true|false)\s*(?:\n#|$|\n\n)', analysis, re.IGNORECASE)
    if m:
        result['needs_human_review'] = m.group(1).lower() == 'true'
        logger.debug(f"Pattern 2B: needs_human_review = {result['needs_human_review']}")

    # human_review_reason (plain string)
    m = re.search(r'# human_review_reason\s*\n\s*["\']?([^"\'\n]+)["\']?\s*(?:\n#|$|\n\n)', analysis)
    if m:
        val = m.group(1).strip()
        if val and val not in ("None", "null"):
            result['human_review_reason'] = val
            logger.debug(f"Pattern 2B: human_review_reason = {val}")

    # validation_attempts_history (JSON list)
    m = re.search(r'# validation_attempts_history\s*\n\s*(\[.*?\])\s*(?:\n#|$|\n\n)', analysis, re.DOTALL)
    if m:
        parsed = _try_parse_json_value(m.group(1))
        if isinstance(parsed, list):
            result['validation_attempts_history'] = parsed
            logger.debug(f"Pattern 2B: validation_attempts_history ({len(parsed)} entries)")

    # actionable (bool)
    m = re.search(r'# actionable\s*\n\s*(True|False|true|false)\s*(?:\n#|$|\n\n)', analysis, re.IGNORECASE)
    if m:
        result['actionable'] = m.group(1).lower() == 'true'
        logger.debug(f"Pattern 2B: actionable = {result['actionable']}")

    if result:
        logger.info({"event": "pattern_2b_parsed", "keys": list(result.keys())})
        return result
    return None


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
        if no workflow to validate or no data_storage_client provided.  When
        validation ran, the result is always returned (is_valid=True/False) so
        callers can access parameter_schema for conditional injection (#524).
    """
    from src.validation.workflow_response_validator import WorkflowResponseValidator, ValidationResult

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

    # Pattern 2B: Section-header format (HolmesGPT SDK format)
    # Parses directly into Python objects — no string serialization round-trip.
    json_data = None
    if not json_match:
        json_data = _parse_section_headers(analysis)

    alternative_workflows = []
    selected_workflow = None
    rca = {"summary": "No structured RCA found", "severity": "unknown", "contributing_factors": []}
    confidence = 0.0
    validation_result = None

    # Parse Pattern 1/2A match into json_data (only if Pattern 2B didn't produce it)
    if json_data is None and json_match:
        try:
            json_text = json_match.group(1) if hasattr(json_match, 'lastindex') and json_match.lastindex else json_match.group(0)
            json_data = _try_parse_json_value(json_text)
            if json_data is None:
                raise ValueError(f"Could not parse JSON from match ({len(json_text)} chars)")
        except (ValueError, SyntaxError) as e:
            logger.warning({
                "event": "parse_error",
                "error": str(e),
                "error_type": type(e).__name__,
                "incident_id": incident_id
            })
            rca = {"summary": "Failed to parse RCA", "severity": "unknown", "contributing_factors": []}

    # Process json_data (unified for all patterns)
    if json_data is not None:
        rca = json_data.get("root_cause_analysis", {})
        selected_workflow = json_data.get("selected_workflow")
        confidence = json_data.get("confidence", selected_workflow.get("confidence", 0.0) if selected_workflow else 0.0)

        raw_alternatives = json_data.get("alternative_workflows", [])
        logger.info({
            "event": "alternative_workflows_extraction",
            "incident_id": incident_id,
            "raw_alternatives_count": len(raw_alternatives),
            "raw_alternatives": raw_alternatives
        })
        for alt in raw_alternatives:
            if isinstance(alt, dict) and alt.get("workflow_id"):
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

        # DD-HAPI-002 v1.3, DD-WORKFLOW-017: Workflow Response Validation
        if selected_workflow and data_storage_client:
            validator = WorkflowResponseValidator(data_storage_client)
            validation_result = validator.validate(
                workflow_id=selected_workflow.get("workflow_id", ""),
                execution_bundle=selected_workflow.get("execution_bundle"),
                parameters=selected_workflow.get("parameters", {})
            )
            if validation_result.validated_service_account_name:
                selected_workflow["service_account_name"] = validation_result.validated_service_account_name

            if validation_result.is_valid:
                if validation_result.validated_execution_bundle:
                    selected_workflow["execution_bundle"] = validation_result.validated_execution_bundle

    # ADR-055: owner_chain validation removed. target_in_owner_chain is superseded
    # by affected_resource in Rego policy input (BR-AI-085, FR-AI-085-005).
    warnings: List[str] = []

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
    # #388: Extract actionable field (orthogonal to investigation_outcome)
    actionable_from_llm = json_data.get("actionable") if json_data else None
    is_actionable = True  # Default: alerts are actionable unless LLM says otherwise

    # #388 Outcome D: Alert not actionable — check BEFORE other outcome routing.
    # actionable=false is authoritative (same pattern as resolved in #301).
    if actionable_from_llm is False:
        warnings.append("Alert not actionable \u2014 no remediation warranted")
        is_actionable = False
        if needs_human_review_from_llm is not None and needs_human_review_from_llm:
            logger.warning({
                "event": "not_actionable_contradiction_override",
                "incident_id": incident_id,
                "needs_human_review_from_llm": needs_human_review_from_llm,
                "message": "#388: Overriding contradictory needs_human_review=true because actionable=false"
            })
        needs_human_review = False
        human_review_reason = None
        logger.info({
            "event": "alert_not_actionable",
            "incident_id": incident_id,
            "confidence": confidence,
            "message": "#388: LLM determined alert is benign — no remediation warranted"
        })
    # BR-HAPI-200: Outcome A - Problem self-resolved (high confidence, no workflow needed)
    elif investigation_outcome == "resolved":
        warnings.append("Problem self-resolved - no remediation required")
        # #301: resolved outcome is authoritative — override any contradictory LLM values.
        # needs_human_review=true + investigation_outcome=resolved is a contradiction;
        # the structured outcome takes precedence over the boolean flag.
        if needs_human_review_from_llm is not None and needs_human_review_from_llm:
            logger.warning({
                "event": "resolved_contradiction_override",
                "incident_id": incident_id,
                "needs_human_review_from_llm": needs_human_review_from_llm,
                "message": "#301: Overriding contradictory needs_human_review=true because investigation_outcome=resolved"
            })
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
        # #301 Layer 2: Infer resolved from RCA summary when LLM omits investigation_outcome.
        # The prompt contract (Outcome A) requires investigation_outcome=resolved, but LLMs
        # occasionally describe resolution in the RCA without setting the field.
        rca_summary_lower = (rca.get("summary", "") or "").lower() if rca else ""
        resolution_indicators = [
            "self-resolved", "self-healed", "auto-recovered",
            "recovered on its own", "no longer occurring",
        ]
        if any(ind in rca_summary_lower for ind in resolution_indicators):
            warnings.append("Problem self-resolved - no remediation required")
            needs_human_review = False
            human_review_reason = None
            logger.info({
                "event": "inferred_resolved_from_rca",
                "incident_id": incident_id,
                "rca_summary": rca.get("summary", ""),
                "message": "#301: Inferred resolution from RCA summary (LLM omitted investigation_outcome)"
            })
        else:
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
    # BR-496 v2: remediationTarget validation removed from parser — HAPI injects it
    # from K8s-verified root_owner post-loop via _inject_target_resource.

    # E2E-HAPI-003: Extract validation_attempts_history from LLM if provided
    validation_attempts_from_llm = json_data.get("validation_attempts_history") if json_data else None
    logger.info({
        "event": "validation_attempts_extraction",
        "incident_id": incident_id,
        "from_llm": validation_attempts_from_llm is not None,
        "count": len(validation_attempts_from_llm) if validation_attempts_from_llm else 0,
        "type": type(validation_attempts_from_llm).__name__ if validation_attempts_from_llm else "None"
    })

    # #607: Defense-in-depth confidence floor for actionable=false.
    # When the LLM explicitly determines an alert is not actionable,
    # a low/missing confidence is contradictory. Floor to 0.8 so
    # downstream consumers always have a well-formed value.
    if not is_actionable and confidence < 0.8:
        logger.info({
            "event": "confidence_floor_applied",
            "incident_id": incident_id,
            "confidence_original": confidence,
            "confidence_floored": 0.8,
            "message": "#607: Applying confidence floor for actionable=false determination"
        })
        confidence = 0.8

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
    # #388: Include is_actionable when LLM explicitly set actionable=false
    if not is_actionable:
        result["is_actionable"] = False
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

    # #372: When no structured output was parsed at all, signal format failure
    # so the retry loop can prompt the LLM to resubmit with correct format.
    # Legitimate no-workflow outcomes (A/B/C) always produce json_data != None.
    if json_data is None and selected_workflow is None and validation_result is None:
        validation_result = ValidationResult(
            is_valid=False,
            errors=[
                "LLM did not produce structured JSON output. "
                "Expected ```json``` code block or # section_header format."
            ],
        )
        logger.warning({
            "event": "structured_output_missing",
            "incident_id": incident_id,
            "message": "#372: LLM response contained no parseable structured output — triggering retry"
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

    # Pattern 2B: Section-header format — direct dict, no string round-trip
    json_data = None
    if not json_match:
        json_data = _parse_section_headers(analysis)

    alternative_workflows = []
    rca = {"summary": "No structured RCA found", "severity": "unknown", "contributing_factors": []}
    selected_workflow = None
    confidence = 0.0
    workflow_validation_failed = False
    workflow_validation_errors: List[str] = []

    # Parse Pattern 1/2A match into json_data (only if Pattern 2B didn't produce it)
    if json_data is None and json_match:
        try:
            json_text = json_match.group(1) if hasattr(json_match, 'lastindex') and json_match.lastindex else json_match.group(0)
            json_data = _try_parse_json_value(json_text)
            if json_data is None:
                raise ValueError(f"Could not parse JSON from match ({len(json_text)} chars)")
        except (ValueError, SyntaxError) as e:
            logger.warning({
                "event": "parse_error",
                "error": str(e),
                "incident_id": incident_id,
            })
            rca = {"summary": "Failed to parse RCA", "severity": "unknown", "contributing_factors": []}

    # Process json_data (unified for all patterns)
    if json_data is not None:
        rca = json_data.get("root_cause_analysis", {})
        selected_workflow = json_data.get("selected_workflow")
        confidence = json_data.get("confidence", selected_workflow.get("confidence", 0.0) if selected_workflow else 0.0)

        raw_alternatives = json_data.get("alternative_workflows", [])
        for alt in raw_alternatives:
            if isinstance(alt, dict) and alt.get("workflow_id"):
                rationale = alt.get("rationale") or "Alternative workflow option"
                alternative_workflows.append({
                    "workflow_id": alt.get("workflow_id", ""),
                    "execution_bundle": alt.get("execution_bundle"),
                    "confidence": float(alt.get("confidence", 0.0)),
                    "rationale": rationale
                })

        if selected_workflow and data_storage_client:
            from src.validation.workflow_response_validator import WorkflowResponseValidator
            validator = WorkflowResponseValidator(data_storage_client)
            validation_result = validator.validate(
                workflow_id=selected_workflow.get("workflow_id", ""),
                execution_bundle=selected_workflow.get("execution_bundle"),
                parameters=selected_workflow.get("parameters", {})
            )
            if validation_result.validated_service_account_name:
                selected_workflow["service_account_name"] = validation_result.validated_service_account_name

            if not validation_result.is_valid:
                workflow_validation_failed = True
                workflow_validation_errors = validation_result.errors
                selected_workflow["validation_errors"] = validation_result.errors
            else:
                if validation_result.validated_execution_bundle:
                    selected_workflow["execution_bundle"] = validation_result.validated_execution_bundle

    warnings: List[str] = []

    # Extract LLM-provided human review fields from json_data directly
    needs_human_review_from_llm = json_data.get("needs_human_review") if json_data else None
    human_review_reason_from_llm = json_data.get("human_review_reason") if json_data else None
    investigation_outcome = json_data.get("investigation_outcome") if json_data else None
    actionable_from_llm = json_data.get("actionable") if json_data else None
    
    # Initialize with LLM values if provided, otherwise use defaults
    if needs_human_review_from_llm is not None:
        needs_human_review = bool(needs_human_review_from_llm)
        human_review_reason = human_review_reason_from_llm if needs_human_review else None
    else:
        needs_human_review = False
        human_review_reason = None

    is_actionable = True  # Default: alerts are actionable unless LLM says otherwise

    # BR-HAPI-200: Handle special investigation outcomes

    # #388 Outcome D: Alert not actionable — check BEFORE other outcome routing.
    if actionable_from_llm is False:
        warnings.append("Alert not actionable \u2014 no remediation warranted")
        is_actionable = False
        needs_human_review = False
        human_review_reason = None
        logger.info({
            "event": "alert_not_actionable",
            "incident_id": incident_id,
            "confidence": confidence,
            "message": "#388: LLM determined alert is benign — no remediation warranted"
        })
    # BR-HAPI-200: Outcome A - Problem self-resolved (high confidence, no workflow needed)
    elif investigation_outcome == "resolved":
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

    # #607: Defense-in-depth confidence floor for actionable=false (deprecated parser).
    if not is_actionable and confidence < 0.8:
        logger.info({
            "event": "confidence_floor_applied",
            "incident_id": incident_id,
            "confidence_original": confidence,
            "confidence_floored": 0.8,
            "message": "#607: Applying confidence floor for actionable=false determination"
        })
        confidence = 0.8

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
    # #388: Include is_actionable when LLM explicitly set actionable=false
    if not is_actionable:
        result["is_actionable"] = False
    # BR-AUDIT-005 Gap #4: Always include alternative_workflows for audit trail (even if empty)
    # ADR-045 v1.2: Required for SOC2 compliance and RR reconstruction
    result["alternative_workflows"] = alternative_workflows
    
    return result

