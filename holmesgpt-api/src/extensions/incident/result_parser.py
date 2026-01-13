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

# Application constants
from src.config.constants import CONFIDENCE_THRESHOLD_HUMAN_REVIEW

logger = logging.getLogger(__name__)


def parse_and_validate_investigation_result(
    investigation: InvestigationResult,
    request_data: Dict[str, Any],
    owner_chain: Optional[List[Dict[str, Any]]] = None,
    data_storage_client=None
):
    """
    Parse and validate HolmesGPT investigation result.

    DD-HAPI-002 v1.2: Returns both the parsed result AND the validation result
    so the caller can decide whether to retry with error feedback.

    Args:
        investigation: HolmesGPT investigation result
        request_data: Original request data
        owner_chain: OwnerChain from enrichment results for target validation
        data_storage_client: Data Storage client for workflow validation

    Returns:
        Tuple of (result_dict, validation_result) where validation_result is None
        if no workflow to validate or validation passed.
    """
    from src.validation.workflow_response_validator import WorkflowResponseValidator

    incident_id = request_data.get("incident_id", "unknown")

    # Extract analysis text
    analysis = investigation.analysis if investigation and investigation.analysis else "No analysis available"

    # Try to parse JSON from analysis
    # Pattern 1: JSON code block (standard format)
    json_match = re.search(r'```json\s*(\{.*?\})\s*```', analysis, re.DOTALL)

    # Pattern 2: Python dict format with section headers (HolmesGPT SDK format)
    # Format: "# root_cause_analysis\n{'summary': '...', ...}\n\n# selected_workflow\n{'workflow_id': '...', ...}"
    if not json_match and ('# selected_workflow' in analysis or '# root_cause_analysis' in analysis):
        import ast
        parts = {}

        # Extract root_cause_analysis
        rca_match = re.search(r'# root_cause_analysis\s*\n\s*(\{.*?\})\s*(?:\n#|$)', analysis, re.DOTALL)
        if rca_match:
            parts['root_cause_analysis'] = rca_match.group(1)
            logger.debug(f"Pattern 2: Extracted RCA: {parts['root_cause_analysis'][:100]}...")

        # Extract selected_workflow
        wf_match = re.search(r'# selected_workflow\s*\n\s*(\{.*?\})\s*(?:\n#|$|\n\n)', analysis, re.DOTALL)
        if wf_match:
            parts['selected_workflow'] = wf_match.group(1)
            logger.debug(f"Pattern 2: Extracted workflow: {parts['selected_workflow'][:100]}...")

        if parts:
            # Combine into a single dict string
            combined_dict = '{'
            for key, value in parts.items():
                combined_dict += f'"{key}": {value}, '
            combined_dict = combined_dict.rstrip(', ') + '}'
            logger.debug(f"Pattern 2: Combined dict: {combined_dict[:200]}...")

            # Create a fake match object
            class FakeMatch:
                def __init__(self, text):
                    self._text = text
                    self.lastindex = None
                def group(self, n):
                    return self._text

            json_match = FakeMatch(combined_dict)
            logger.info("Pattern 2: Successfully created FakeMatch for SDK format")

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
            confidence = selected_workflow.get("confidence", 0.0) if selected_workflow else 0.0

            # Extract alternative workflows (ADR-045 v1.2 - for audit/context only)
            raw_alternatives = json_data.get("alternative_workflows", [])
            for alt in raw_alternatives:
                if isinstance(alt, dict) and alt.get("workflow_id"):
                    alternative_workflows.append({
                        "workflow_id": alt.get("workflow_id", ""),
                        "container_image": alt.get("container_image"),
                        "confidence": float(alt.get("confidence", 0.0)),
                        "rationale": alt.get("rationale", "")
                    })

            # DD-HAPI-002 v1.2: Workflow Response Validation
            # Validates: workflow existence, container image consistency, parameter schema
            if selected_workflow and data_storage_client:
                validator = WorkflowResponseValidator(data_storage_client)
                validation_result = validator.validate(
                    workflow_id=selected_workflow.get("workflow_id", ""),
                    container_image=selected_workflow.get("container_image"),
                    parameters=selected_workflow.get("parameters", {})
                )
                if validation_result.is_valid:
                    # Use validated container image from catalog
                    if validation_result.validated_container_image:
                        selected_workflow["container_image"] = validation_result.validated_container_image
                    validation_result = None  # Clear to indicate success

        except (json.JSONDecodeError, ValueError, SyntaxError) as e:
            logger.warning({
                "event": "parse_error",
                "error": str(e),
                "error_type": type(e).__name__,
                "incident_id": incident_id
            })
            rca = {"summary": "Failed to parse RCA", "severity": "unknown", "contributing_factors": []}

    # OwnerChain validation (DD-WORKFLOW-001 v1.7, AIAnalysis request Dec 2025)
    target_in_owner_chain = True
    warnings: List[str] = []

    # Check if RCA-identified target is in OwnerChain
    rca_target = rca.get("affectedResource") or rca.get("affected_resource")
    if rca_target and owner_chain:
        target_in_owner_chain = is_target_in_owner_chain(rca_target, owner_chain, request_data)
        if not target_in_owner_chain:
            warnings.append(
                "Target resource not found in OwnerChain - DetectedLabels may not apply to affected resource"
            )

    # Generate warnings for other conditions (BR-HAPI-197, BR-HAPI-200)
    needs_human_review = False
    human_review_reason = None

    # BR-HAPI-200: Handle special investigation outcomes
    investigation_outcome = json_data.get("investigation_outcome") if json_data else None

    # BR-HAPI-200: Outcome A - Problem self-resolved (high confidence, no workflow needed)
    if investigation_outcome == "resolved":
        warnings.append("Problem self-resolved - no remediation required")
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
        needs_human_review = True
        human_review_reason = "no_matching_workflows"
    elif confidence < CONFIDENCE_THRESHOLD_HUMAN_REVIEW:
        warnings.append(f"Low confidence selection ({confidence:.0%}) - manual review recommended")
        needs_human_review = True
        human_review_reason = "low_confidence"

    result = {
        "incident_id": incident_id,
        "analysis": analysis,
        "root_cause_analysis": rca,
        "selected_workflow": selected_workflow,
        "confidence": confidence,
        "timestamp": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "target_in_owner_chain": target_in_owner_chain,
        "warnings": warnings,
        "needs_human_review": needs_human_review,
        "human_review_reason": human_review_reason,
        "alternative_workflows": alternative_workflows
    }

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


def is_target_in_owner_chain(
    rca_target: Dict[str, Any],
    owner_chain: List[Dict[str, Any]],
    request_data: Dict[str, Any]
) -> bool:
    """
    Check if RCA-identified target resource is in the OwnerChain.

    DD-WORKFLOW-001 v1.7: OwnerChain validation ensures DetectedLabels
    are applicable to the actual affected resource.

    Args:
        rca_target: The resource identified by RCA (kind, name, namespace)
        owner_chain: List of owner resources from enrichment
        request_data: Original request for source resource comparison

    Returns:
        True if target is in OwnerChain or is the source resource, False otherwise
    """
    # Extract target identifiers
    target_kind = rca_target.get("kind", "").lower()
    target_name = rca_target.get("name", "").lower()
    target_namespace = rca_target.get("namespace", "").lower()

    # Check if target matches the source resource (always valid)
    source_kind = request_data.get("resource_kind", "").lower()
    source_name = request_data.get("resource_name", "").lower()
    source_namespace = request_data.get("resource_namespace", "").lower()

    if target_kind == source_kind and target_name == source_name:
        if not target_namespace or target_namespace == source_namespace:
            return True

    # Check if target is in OwnerChain
    for owner in owner_chain:
        owner_kind = owner.get("kind", "").lower()
        owner_name = owner.get("name", "").lower()
        owner_namespace = owner.get("namespace", "").lower()

        if target_kind == owner_kind and target_name == owner_name:
            if not target_namespace or target_namespace == owner_namespace:
                return True

    return False


def parse_investigation_result(
    investigation: InvestigationResult,
    request_data: Dict[str, Any],
    owner_chain: Optional[List[Dict[str, Any]]] = None,
    data_storage_client=None
) -> Dict[str, Any]:
    """
    Parse HolmesGPT investigation result into IncidentResponse format.

    DEPRECATED: Use parse_and_validate_investigation_result for self-correction loop.

    Business Requirement: BR-HAPI-002 (Incident analysis response schema)
    Design Decision: DD-WORKFLOW-001 v1.7 (OwnerChain validation)
    Design Decision: DD-HAPI-002 v1.2 (Workflow Response Validation)

    Args:
        investigation: HolmesGPT investigation result
        request_data: Original request data
        owner_chain: OwnerChain from enrichment results for target validation
        data_storage_client: Optional Data Storage client for workflow validation (DD-HAPI-002 v1.2)
    """
    incident_id = request_data.get("incident_id", "unknown")

    # Extract analysis text
    analysis = investigation.analysis if investigation and investigation.analysis else "No analysis available"

    # Try to parse JSON from analysis
    # Pattern 1: JSON code block (standard format)
    json_match = re.search(r'```json\s*(\{.*?\})\s*```', analysis, re.DOTALL)

    # Pattern 2: Python dict format with section headers (HolmesGPT SDK format)
    # Format: "# root_cause_analysis\n{'summary': '...', ...}\n\n# selected_workflow\n{'workflow_id': '...', ...}"
    if not json_match and ('# selected_workflow' in analysis or '# root_cause_analysis' in analysis):
        import ast
        parts = {}

        # Extract root_cause_analysis
        rca_match = re.search(r'# root_cause_analysis\s*\n\s*(\{.*?\})\s*(?:\n#|$)', analysis, re.DOTALL)
        if rca_match:
            parts['root_cause_analysis'] = rca_match.group(1)
            logger.debug(f"Pattern 2: Extracted RCA: {parts['root_cause_analysis'][:100]}...")

        # Extract selected_workflow
        wf_match = re.search(r'# selected_workflow\s*\n\s*(\{.*?\})\s*(?:\n#|$|\n\n)', analysis, re.DOTALL)
        if wf_match:
            parts['selected_workflow'] = wf_match.group(1)
            logger.debug(f"Pattern 2: Extracted workflow: {parts['selected_workflow'][:100]}...")

        if parts:
            # Combine into a single dict string
            combined_dict = '{'
            for key, value in parts.items():
                combined_dict += f'"{key}": {value}, '
            combined_dict = combined_dict.rstrip(', ') + '}'
            logger.debug(f"Pattern 2: Combined dict: {combined_dict[:200]}...")

            # Create a fake match object
            class FakeMatch:
                def __init__(self, text):
                    self._text = text
                    self.lastindex = None
                def group(self, n):
                    return self._text

            json_match = FakeMatch(combined_dict)
            logger.info("Pattern 2: Successfully created FakeMatch for SDK format")

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
            confidence = selected_workflow.get("confidence", 0.0) if selected_workflow else 0.0

            # Extract alternative workflows (ADR-045 v1.2 - for audit/context only)
            raw_alternatives = json_data.get("alternative_workflows", [])
            for alt in raw_alternatives:
                if isinstance(alt, dict) and alt.get("workflow_id"):
                    alternative_workflows.append({
                        "workflow_id": alt.get("workflow_id", ""),
                        "container_image": alt.get("container_image"),
                        "confidence": float(alt.get("confidence", 0.0)),
                        "rationale": alt.get("rationale", "")
                    })

            # DD-HAPI-002 v1.2: Workflow Response Validation
            # Validates: workflow existence, container image consistency, parameter schema
            if selected_workflow and data_storage_client:
                from src.validation.workflow_response_validator import WorkflowResponseValidator
                validator = WorkflowResponseValidator(data_storage_client)
                validation_result = validator.validate(
                    workflow_id=selected_workflow.get("workflow_id", ""),
                    container_image=selected_workflow.get("container_image"),
                    parameters=selected_workflow.get("parameters", {})
                )
                if not validation_result.is_valid:
                    # Add validation errors as warnings (LLM self-correction would have happened in-session)
                    # If we reach here, it means the LLM failed to provide valid workflow after max attempts
                    workflow_validation_failed = True
                    workflow_validation_errors = validation_result.errors
                    logger.warning({
                        "event": "workflow_validation_failed",
                        "incident_id": request_data.get("incident_id", "unknown"),
                        "workflow_id": selected_workflow.get("workflow_id"),
                        "errors": validation_result.errors,
                        "message": "DD-HAPI-002 v1.2: Workflow response validation failed"
                    })
                    # Add validation errors to workflow for transparency
                    selected_workflow["validation_errors"] = validation_result.errors
                else:
                    # Use validated container image from catalog
                    if validation_result.validated_container_image:
                        selected_workflow["container_image"] = validation_result.validated_container_image
                        logger.debug({
                            "event": "workflow_validation_passed",
                            "incident_id": request_data.get("incident_id", "unknown"),
                            "workflow_id": selected_workflow.get("workflow_id"),
                            "container_image": validation_result.validated_container_image
                        })
        except json.JSONDecodeError:
            rca = {"summary": "Failed to parse RCA", "severity": "unknown", "contributing_factors": []}
            selected_workflow = None
            confidence = 0.0
    else:
        rca = {"summary": "No structured RCA found", "severity": "unknown", "contributing_factors": []}
        selected_workflow = None
        confidence = 0.0

    # OwnerChain validation (DD-WORKFLOW-001 v1.7, AIAnalysis request Dec 2025)
    target_in_owner_chain = True
    warnings: List[str] = []

    # DD-HAPI-002 v1.2: Workflow validation tracking
    workflow_validation_failed = False
    workflow_validation_errors: List[str] = []

    # Check if RCA-identified target is in OwnerChain
    rca_target = rca.get("affectedResource") or rca.get("affected_resource")
    if rca_target and owner_chain:
        target_in_owner_chain = is_target_in_owner_chain(rca_target, owner_chain, request_data)
        if not target_in_owner_chain:
            warnings.append(
                "Target resource not found in OwnerChain - DetectedLabels may not apply to affected resource"
            )
            logger.warning({
                "event": "target_not_in_owner_chain",
                "incident_id": incident_id,
                "rca_target": rca_target,
                "owner_chain_length": len(owner_chain),
                "message": "DD-WORKFLOW-001 v1.7: RCA target not in OwnerChain, DetectedLabels may be from different scope"
            })

    # Generate warnings for other conditions (BR-HAPI-197, BR-HAPI-200)
    needs_human_review = False
    human_review_reason = None

    # BR-HAPI-200: Handle special investigation outcomes
    investigation_outcome = None
    if json_match:
        try:
            json_data_outcome = json.loads(json_match.group(1))
            investigation_outcome = json_data_outcome.get("investigation_outcome")
        except json.JSONDecodeError:
            pass

    # BR-HAPI-200: Outcome A - Problem self-resolved (high confidence, no workflow needed)
    if investigation_outcome == "resolved":
        warnings.append("Problem self-resolved - no remediation required")
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
        needs_human_review = True
        human_review_reason = "no_matching_workflows"
    elif confidence < CONFIDENCE_THRESHOLD_HUMAN_REVIEW:
        warnings.append(f"Low confidence selection ({confidence:.0%}) - manual review recommended")
        needs_human_review = True
        human_review_reason = "low_confidence"

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

    return {
        "incident_id": incident_id,
        "analysis": analysis,
        "root_cause_analysis": rca,
        "selected_workflow": selected_workflow,
        "confidence": confidence,
        "timestamp": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "target_in_owner_chain": target_in_owner_chain,
        "warnings": warnings,
        # DD-HAPI-002 v1.2, BR-HAPI-197: Human review flag and structured reason
        "needs_human_review": needs_human_review,
        "human_review_reason": human_review_reason,
        # Alternative workflows for audit/context (ADR-045 v1.2)
        # IMPORTANT: These are for INFORMATIONAL purposes only - NOT for automatic execution
        "alternative_workflows": alternative_workflows
    }

