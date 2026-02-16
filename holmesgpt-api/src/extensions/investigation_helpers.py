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
Shared Investigation Helpers

Business Requirements:
  - BR-AUDIT-005 (Audit Trail)
  - BR-HAPI-017-004 (Max Validation Attempts)
  - BR-HAPI-197 (needs_human_review)

Design Decisions:
  - DD-HAPI-002 v1.2 (LLM Self-Correction Loop)
  - ADR-032 §1 (Audit is MANDATORY)
  - ADR-038 (Async Buffered Audit Ingestion)

DRY helpers for LLM investigation audit and post-loop exhaustion handling,
shared by both incident and recovery analysis flows.
"""

import logging
from typing import Any, Dict, List, Optional

from src.audit import (
    create_llm_request_event,
    create_llm_response_event,
    create_tool_call_event,
    create_validation_attempt_event,
)
from src.extensions.incident.result_parser import determine_human_review_reason

logger = logging.getLogger(__name__)


def require_audit_store(audit_store, incident_id: str, remediation_id: str) -> None:
    """
    Validate audit_store is available per ADR-032 §1.

    Args:
        audit_store: Audit store instance
        incident_id: Incident ID for error context
        remediation_id: Remediation ID for error context

    Raises:
        RuntimeError: If audit_store is None
    """
    if audit_store is None:
        logger.error(
            "CRITICAL: audit_store is None - audit is MANDATORY per ADR-032 §1",
            extra={
                "incident_id": incident_id,
                "remediation_id": remediation_id,
                "adr": "ADR-032 §1",
            }
        )
        raise RuntimeError("audit_store is None - audit is MANDATORY per ADR-032 §1")


def audit_llm_request(
    audit_store,
    incident_id: str,
    remediation_id: str,
    config,
    prompt: str,
) -> None:
    """
    Audit the LLM request (prompt being sent to the model).

    Business Requirement: BR-AUDIT-005
    Design Decision: ADR-032 §1 (audit is MANDATORY)

    Must be called BEFORE the SDK investigate_issues() call so the request
    is audited even if the call fails.

    Args:
        audit_store: Audit store instance (must not be None)
        incident_id: Incident ID for correlation
        remediation_id: Remediation ID for correlation
        config: HolmesGPT SDK Config (for model, toolsets, mcp_servers)
        prompt: The investigation prompt being sent to the LLM
    """
    require_audit_store(audit_store, incident_id, remediation_id)
    audit_store.store_audit(create_llm_request_event(
        incident_id=incident_id,
        remediation_id=remediation_id,
        model=config.model if config else "unknown",
        prompt=prompt,
        toolsets_enabled=list(config.toolsets.keys()) if config and config.toolsets else [],
        mcp_servers=list(config.mcp_servers.keys()) if config and hasattr(config, 'mcp_servers') and config.mcp_servers else [],
    ))


def audit_llm_response_and_tools(
    audit_store,
    incident_id: str,
    remediation_id: str,
    investigation_result,
) -> None:
    """
    Audit the LLM response and any tool calls made during investigation.

    Business Requirement: BR-AUDIT-005
    Design Decision: ADR-032 §1 (audit is MANDATORY)

    Must be called AFTER the SDK investigate_issues() call returns.

    Args:
        audit_store: Audit store instance (must not be None)
        incident_id: Incident ID for correlation
        remediation_id: Remediation ID for correlation
        investigation_result: The InvestigationResult from the HolmesGPT SDK
    """
    require_audit_store(audit_store, incident_id, remediation_id)

    has_analysis = bool(investigation_result and investigation_result.analysis)
    analysis_length = len(investigation_result.analysis) if has_analysis else 0
    analysis_preview = (
        investigation_result.analysis[:500] + "..."
        if analysis_length > 500
        else (investigation_result.analysis if has_analysis else "")
    )
    tool_call_count = (
        len(investigation_result.tool_calls)
        if investigation_result and hasattr(investigation_result, 'tool_calls') and investigation_result.tool_calls
        else 0
    )

    audit_store.store_audit(create_llm_response_event(
        incident_id=incident_id,
        remediation_id=remediation_id,
        has_analysis=has_analysis,
        analysis_length=analysis_length,
        analysis_preview=analysis_preview,
        tool_call_count=tool_call_count,
    ))

    # Audit individual tool calls if any
    if investigation_result and hasattr(investigation_result, 'tool_calls') and investigation_result.tool_calls:
        for idx, tool_call in enumerate(investigation_result.tool_calls):
            audit_store.store_audit(create_tool_call_event(
                incident_id=incident_id,
                remediation_id=remediation_id,
                tool_call_index=idx,
                tool_name=getattr(tool_call, 'name', 'unknown'),
                tool_arguments=getattr(tool_call, 'arguments', {}),
                tool_result=getattr(tool_call, 'result', None),
            ))


def handle_validation_exhaustion(
    result: Dict[str, Any],
    validation_errors_history: List[List[str]],
    max_attempts: int,
    audit_store,
    incident_id: str,
    remediation_id: str,
    workflow_id: Optional[str],
) -> None:
    """
    Handle the case where all LLM validation attempts are exhausted.

    Sets needs_human_review=True, builds a detailed error summary from all
    attempts, writes a final audit event, and logs a warning.

    No-op if validation_errors_history is empty or fewer than max_attempts.

    Business Requirements:
      - BR-HAPI-017-004 (Max Validation Attempts)
      - BR-HAPI-197 (needs_human_review field)

    Args:
        result: The parsed result dict (will be mutated in-place)
        validation_errors_history: History of validation errors per attempt
        max_attempts: Maximum validation attempts allowed
        audit_store: Audit store instance
        incident_id: Incident ID for correlation
        remediation_id: Remediation ID for correlation
        workflow_id: The workflow ID that failed validation
    """
    if not validation_errors_history or len(validation_errors_history) < max_attempts:
        return

    require_audit_store(audit_store, incident_id, remediation_id)

    last_errors = validation_errors_history[-1]
    human_review_reason = determine_human_review_reason(last_errors)

    # BR-HAPI-197.2 (MUST): needs_human_review SHALL be true when parameter
    # validation fails. This is unconditional — the LLM's self-assessment
    # cannot override a safety gate. If the LLM cannot produce a valid
    # workflow after max_attempts retries, human review is mandatory.
    result["needs_human_review"] = True
    result["human_review_reason"] = human_review_reason

    # Build detailed error summary from ALL attempts
    all_errors_summary = []
    for i, errors in enumerate(validation_errors_history):
        all_errors_summary.append(f"Attempt {i+1}: {'; '.join(errors)}")
    result["warnings"].append(
        f"Workflow validation failed after {max_attempts} attempts. " +
        " | ".join(all_errors_summary)
    )

    # Final audit with human_review_reason
    audit_store.store_audit(create_validation_attempt_event(
        incident_id=incident_id,
        remediation_id=remediation_id,
        attempt=max_attempts,
        max_attempts=max_attempts,
        is_valid=False,
        errors=last_errors,
        workflow_id=workflow_id,
        human_review_reason=human_review_reason,
    ))

    logger.warning({
        "event": "workflow_validation_exhausted",
        "incident_id": incident_id,
        "total_attempts": max_attempts,
        "all_errors": validation_errors_history,
        "final_errors": last_errors,
        "human_review_reason": human_review_reason,
        "message": "Max validation attempts exhausted, needs_human_review=True",
    })
