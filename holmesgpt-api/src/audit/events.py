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
LLM Audit Event Factories

Business Requirement: BR-AUDIT-005 - Workflow Selection Audit Trail
Design Decisions:
  - ADR-034: Unified Audit Table Design (AUTHORITATIVE schema)
  - ADR-038: Asynchronous Buffered Audit Trace Ingestion
  - DD-AUDIT-002: Audit Shared Library Design

This module provides factory functions for creating LLM audit events.
Events are structured per ADR-034 unified audit schema.

ADR-034 Required Fields:
  - version: Schema version (always "1.0")
  - service/event_category: Service category per ADR-034 v1.2 ("aiagent" for AI Agent Provider)
  - event_type: Event type (e.g., "llm_request", "llm_response", "llm_tool_call")
  - event_timestamp: ISO 8601 timestamp
  - correlation_id: Remediation ID for correlation
  - operation/event_action: Action performed
  - outcome/event_outcome: Result status
  - event_data: Service-specific payload (JSONB)

Event Types:
  - llm_request: LLM prompt sent to model
  - llm_response: LLM analysis response received
  - llm_tool_call: LLM tool invocation (e.g., search_workflow_catalog)
  - workflow_validation_attempt: Validation retry event

Usage:
    from src.audit.events import (
        create_llm_request_event,
        create_llm_response_event,
        create_tool_call_event
    )

    # Create event (ADR-034 compliant)
    event = create_llm_request_event(
        incident_id="inc-123",
        remediation_id="rem-456",
        model="claude-3-5-sonnet",
        prompt="Test prompt",
        toolsets_enabled=["kubernetes/core"]
    )

    # Store event (non-blocking)
    audit_store.store_audit(event)
"""

# Standard library imports
import uuid  # noqa: E402
from datetime import datetime, timezone  # noqa: E402
from typing import Any, Dict, List, Optional  # noqa: E402

# OpenAPI-generated DataStorage client types (DD-API-001)
from datastorage.models.audit_event_request import AuditEventRequest  # noqa: E402
from datastorage.models.audit_event_request_event_data import AuditEventRequestEventData  # noqa: E402

# Local imports - structured audit event data models (ADR-034 compliance)
from src.models.audit_models import (  # noqa: E402
    LLMRequestEventData,
    LLMResponseEventData,
    LLMToolCallEventData,
    WorkflowValidationEventData,
    HAPIResponseEventData  # DD-AUDIT-005: HAPI response complete event
)


# ADR-034 Constants
AUDIT_VERSION = "1.0"
# ADR-034 v1.2: event_category must be one of the allowed service categories
# HolmesGPT API is an AI Analysis service, so use "analysis"
# See Data Storage OpenAPI: event_category enum values
SERVICE_NAME = "aiagent"  # ADR-034 v1.2: AI Agent Provider (HolmesGPT autonomous tool-calling agent)


def _get_utc_timestamp() -> str:
    """Get current UTC timestamp in ISO 8601 format (ADR-034 compliant)."""
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def _create_adr034_event(
    event_type: str,
    operation: str,
    outcome: str,
    correlation_id: str,
    event_data: Any  # Pydantic model (LLMRequestEventData, etc.)
) -> AuditEventRequest:
    """
    Create an ADR-034 compliant audit event envelope using OpenAPI types.

    V3.0: OGEN MIGRATION - Returns OpenAPI-generated Pydantic model instead of dict.

    This is the canonical format for all audit events per ADR-034.
    Data Storage expects this exact structure.

    Args:
        event_type: Event type (e.g., "llm_request")
        operation: Action performed (e.g., "llm_request_sent")
        outcome: Result status ("success", "failure", "pending")
        correlation_id: Remediation ID for correlation
        event_data: Service-specific Pydantic payload (LLMRequestEventData, etc.)

    Returns:
        AuditEventRequest (OpenAPI-generated Pydantic model)
    """
    # Wrap event_data in discriminated union type
    # OpenAPI client expects AuditEventRequestEventData (discriminated union)
    event_data_union = AuditEventRequestEventData(actual_instance=event_data)

    return AuditEventRequest(
        # ADR-034 Required Fields (OpenAPI spec per Data Storage Service)
        version=AUDIT_VERSION,
        event_category=SERVICE_NAME,  # Service name (OpenAPI: event_category)
        event_type=event_type,
        event_timestamp=_get_utc_timestamp(),
        correlation_id=correlation_id,
        event_action=operation,  # Action performed (OpenAPI: event_action)
        event_outcome=outcome,  # Result status (OpenAPI: event_outcome)
        event_data=event_data_union,  # Discriminated union wrapper
        actor_type="Service",  # Actor type per ADR-034
        actor_id="holmesgpt-api",  # Service identifier per DD-AUDIT-005
    )


def create_llm_request_event(
    incident_id: str,
    remediation_id: Optional[str],
    model: str,
    prompt: str,
    toolsets_enabled: List[str],
    mcp_servers: Optional[List[str]] = None
) -> AuditEventRequest:
    """
    Create an LLM request audit event (ADR-034 compliant)

    Business Requirement: BR-AUDIT-005
    Design Decision: ADR-034 - Unified Audit Table Design

    Args:
        incident_id: Incident identifier for correlation
        remediation_id: Remediation request ID for audit correlation (DD-WORKFLOW-002 v2.2)
        model: LLM model name (e.g., "claude-3-5-sonnet")
        prompt: Full prompt sent to LLM
        toolsets_enabled: List of enabled toolsets
        mcp_servers: Optional list of MCP servers

    Returns:
        ADR-034 compliant audit event dictionary
    """
    # Create structured event_data using Pydantic model for validation
    prompt_preview = prompt[:500] + "..." if len(prompt) > 500 else prompt

    event_data_model = LLMRequestEventData(
        event_type="llm_request",  # ✅ FIX: Discriminator required for OpenAPI validation
        event_id=str(uuid.uuid4()),
        incident_id=incident_id,
        model=model,
        prompt_length=len(prompt),
        prompt_preview=prompt_preview,
        max_tokens=None,  # Can be extended if available from config
        toolsets_enabled=toolsets_enabled,
        mcp_servers=mcp_servers or []
    )

    # V3.0: OGEN MIGRATION - Pass Pydantic model directly, not dict
    # AuditEventRequestEventData expects actual_instance to be a Pydantic model
    return _create_adr034_event(
        event_type="llm_request",
        operation="llm_request_sent",
        outcome="success",
        correlation_id=remediation_id or "unknown",
        event_data=event_data_model  # ← Pass model, not model_dump()
    )


def create_llm_response_event(
    incident_id: str,
    remediation_id: Optional[str],
    has_analysis: bool,
    analysis_length: int,
    analysis_preview: str,
    tool_call_count: int
) -> AuditEventRequest:
    """
    Create an LLM response audit event (ADR-034 compliant)

    Business Requirement: BR-AUDIT-005
    Design Decision: ADR-034 - Unified Audit Table Design

    Args:
        incident_id: Incident identifier for correlation
        remediation_id: Remediation request ID for audit correlation
        has_analysis: Whether LLM returned analysis
        analysis_length: Length of analysis text
        analysis_preview: First 500 chars of analysis
        tool_call_count: Number of tool calls made by LLM

    Returns:
        ADR-034 compliant audit event dictionary
    """
    # Create structured event_data using Pydantic model for validation
    event_data_model = LLMResponseEventData(
        event_type="llm_response",  # ✅ FIX: Discriminator required for OpenAPI validation
        event_id=str(uuid.uuid4()),
        incident_id=incident_id,
        has_analysis=has_analysis,
        analysis_length=analysis_length,
        analysis_preview=analysis_preview,
        tokens_used=None,  # Can be extended if available from LLM response
        tool_call_count=tool_call_count
    )

    # V3.0: OGEN MIGRATION - Pass Pydantic model directly, not dict
    return _create_adr034_event(
        event_type="llm_response",
        operation="llm_response_received",
        outcome="success" if has_analysis else "failure",
        correlation_id=remediation_id or "unknown",
        event_data=event_data_model  # ← Pass model, not model_dump()
    )


def create_tool_call_event(
    incident_id: str,
    remediation_id: Optional[str],
    tool_call_index: int,
    tool_name: str,
    tool_arguments: Dict[str, Any],
    tool_result: Any
) -> AuditEventRequest:
    """
    Create a tool call audit event (ADR-034 compliant)

    Business Requirement: BR-AUDIT-005
    Design Decision: ADR-034 - Unified Audit Table Design

    Args:
        incident_id: Incident identifier for correlation
        remediation_id: Remediation request ID for audit correlation
        tool_call_index: Index of tool call in sequence (0-based)
        tool_name: Name of tool invoked (e.g., "search_workflow_catalog")
        tool_arguments: Arguments passed to tool
        tool_result: Result returned by tool

    Returns:
        ADR-034 compliant audit event dictionary
    """
    # Generate preview of tool result (first 500 chars)
    tool_result_str = str(tool_result)
    tool_result_preview = tool_result_str[:500] + "..." if len(tool_result_str) > 500 else tool_result_str

    # Create structured event_data using Pydantic model for validation
    event_data_model = LLMToolCallEventData(
        event_type="llm_tool_call",  # ✅ FIX: Discriminator required for OpenAPI validation
        event_id=str(uuid.uuid4()),
        incident_id=incident_id,
        tool_call_index=tool_call_index,
        tool_name=tool_name,
        tool_arguments=tool_arguments,
        tool_result=tool_result,
        tool_result_preview=tool_result_preview
    )

    # V3.0: OGEN MIGRATION - Pass Pydantic model directly, not dict
    return _create_adr034_event(
        event_type="llm_tool_call",
        operation="tool_invoked",
        outcome="success",
        correlation_id=remediation_id or "unknown",
        event_data=event_data_model  # ← Pass model, not model_dump()
    )


def create_validation_attempt_event(
    incident_id: str,
    remediation_id: Optional[str],
    attempt: int,
    max_attempts: int,
    is_valid: bool,
    errors: List[str],
    workflow_id: Optional[str] = None,
    human_review_reason: Optional[str] = None
) -> AuditEventRequest:
    """
    Create a workflow validation attempt audit event (ADR-034 compliant)

    Business Requirement: BR-AUDIT-005, BR-HAPI-197
    Design Decision: DD-HAPI-002 v1.2 - Workflow Response Validation

    Tracks each validation attempt during LLM self-correction loop.
    Critical for understanding LLM failures and operator notifications.

    Args:
        incident_id: Incident identifier for correlation
        remediation_id: Remediation request ID for audit correlation
        attempt: Current attempt number (1-indexed)
        max_attempts: Maximum allowed attempts
        is_valid: Whether validation passed
        errors: List of validation error messages
        workflow_id: Workflow ID being validated (if any)
        human_review_reason: Reason code if needs_human_review (final attempt)

    Returns:
        ADR-034 compliant audit event dictionary
    """
    is_final_attempt = attempt >= max_attempts

    # Create structured event_data using Pydantic model for validation
    # Combine error messages into a single string for validation_errors field
    validation_errors_str = "; ".join(errors) if errors else None

    event_data_model = WorkflowValidationEventData(
        event_type="workflow_validation_attempt",  # ✅ FIX: Discriminator required for OpenAPI validation
        event_id=str(uuid.uuid4()),
        incident_id=incident_id,
        attempt=attempt,
        max_attempts=max_attempts,
        is_valid=is_valid,
        errors=errors,
        validation_errors=validation_errors_str,
        workflow_id=workflow_id,
        workflow_name=workflow_id,  # Using workflow_id as workflow_name for now
        human_review_reason=human_review_reason,
        is_final_attempt=is_final_attempt
    )

    # Determine outcome based on validation result
    if is_valid:
        outcome = "success"
    elif is_final_attempt:
        outcome = "failure"
    else:
        outcome = "pending"  # Will retry

    # V3.0: OGEN MIGRATION - Pass Pydantic model directly, not dict
    return _create_adr034_event(
        event_type="workflow_validation_attempt",
        operation="validation_executed",
        outcome=outcome,
        correlation_id=remediation_id or "unknown",
        event_data=event_data_model  # ← Pass model, not model_dump()
    )


def create_hapi_response_complete_event(
    incident_id: str,
    remediation_id: str,
    response_data: Dict[str, Any]
) -> AuditEventRequest:
    """
    Create HAPI response completion audit event (ADR-034 compliant)

    Business Requirement: BR-AUDIT-005 v2.0 (Gap #4 - AI Provider Data)
    Design Decision: DD-AUDIT-005 (Hybrid Provider Data Capture)

    This event captures the COMPLETE HolmesGPT API response (provider perspective)
    for SOC2 Type II compliance and RemediationRequest reconstruction.

    Hybrid Audit Approach:
    - HAPI emits: holmesgpt.response.complete (full IncidentResponse - provider perspective)
    - AI Analysis emits: aianalysis.analysis.completed (summary + business context - consumer perspective)

    This provides defense-in-depth auditing with both provider and consumer perspectives.

    Args:
        incident_id: Incident identifier for correlation
        remediation_id: Remediation request ID for audit correlation
        response_data: Complete IncidentResponse structure (all fields)

    Returns:
        ADR-034 compliant audit event dictionary

    Example response_data:
        {
            "incident_id": "inc-123",
            "analysis": "Root cause analysis text...",
            "root_cause_analysis": {...},
            "selected_workflow": {...},
            "alternative_workflows": [...],
            "confidence": 0.85,
            "needs_human_review": false,
            "warnings": [...]
        }
    """
    # Create structured event_data using Pydantic model for validation
    event_data_model = HAPIResponseEventData(
        event_type="holmesgpt.response.complete",  # Discriminator for OpenAPI validation
        event_id=str(uuid.uuid4()),
        incident_id=incident_id,
        response_data=response_data  # Full IncidentResponse
    )

    # V3.0: OGEN MIGRATION - Pass Pydantic model directly, not dict
    return _create_adr034_event(
        event_type="holmesgpt.response.complete",
        operation="response_sent",
        outcome="success",
        correlation_id=remediation_id,
        event_data=event_data_model  # ← Pass model, not model_dump()
    )

