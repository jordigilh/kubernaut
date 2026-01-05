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
Audit Event Data Models

Business Requirement: BR-AUDIT-001 (Unified audit trail)
Design Decision: ADR-034 (Unified Audit Table Design)
Purpose: Type-safe event_data structures for audit events

These models ensure ADR-034 compliance at compile-time by providing
structured validation for audit event payloads before sending to
Data Storage Service.
"""

from pydantic import BaseModel, Field
from typing import Dict, Any, Optional, List


class LLMRequestEventData(BaseModel):
    """
    event_data structure for llm_request audit events (ADR-034).

    Emitted when: HAPI makes an LLM API call to external provider
    Audit Event Type: llm_request
    Service: HolmesGPT API (HAPI)
    """
    event_id: str = Field(..., description="Unique event identifier")
    incident_id: str = Field(..., description="Incident correlation ID (remediation_id)")
    model: str = Field(..., description="LLM model identifier (e.g., 'gpt-4')")
    prompt_length: int = Field(..., ge=0, description="Length of prompt sent to LLM")
    prompt_preview: str = Field(..., description="First 500 characters of prompt for audit")
    max_tokens: Optional[int] = Field(None, ge=0, description="Maximum tokens requested")
    toolsets_enabled: List[str] = Field(default_factory=list, description="List of enabled toolsets")
    mcp_servers: List[str] = Field(default_factory=list, description="List of MCP servers")


class LLMResponseEventData(BaseModel):
    """
    event_data structure for llm_response audit events (ADR-034).

    Emitted when: HAPI receives response from LLM API call
    Audit Event Type: llm_response
    Service: HolmesGPT API (HAPI)
    """
    event_id: str = Field(..., description="Unique event identifier")
    incident_id: str = Field(..., description="Incident correlation ID (remediation_id)")
    has_analysis: bool = Field(..., description="Whether LLM provided analysis")
    analysis_length: int = Field(..., ge=0, description="Length of LLM response")
    analysis_preview: str = Field(..., description="First 500 characters of response for audit")
    tokens_used: Optional[int] = Field(None, ge=0, description="Tokens consumed by LLM")
    tool_call_count: int = Field(0, ge=0, description="Number of tool calls made by LLM")


class LLMToolCallEventData(BaseModel):
    """
    event_data structure for llm_tool_call audit events (ADR-034).

    Emitted when: LLM invokes a tool during analysis (e.g., workflow search)
    Audit Event Type: llm_tool_call
    Service: HolmesGPT API (HAPI)
    """
    event_id: str = Field(..., description="Unique event identifier")
    incident_id: str = Field(..., description="Incident correlation ID (remediation_id)")
    tool_call_index: int = Field(..., ge=0, description="Sequential index of tool call in conversation")
    tool_name: str = Field(..., description="Name of tool invoked (e.g., 'search_workflow_catalog')")
    tool_arguments: Dict[str, Any] = Field(default_factory=dict, description="Arguments passed to tool (flexible for different tools)")
    tool_result: Any = Field(..., description="Full result returned by tool")
    tool_result_preview: Optional[str] = Field(None, description="First 500 characters of tool result")


class WorkflowValidationEventData(BaseModel):
    """
    event_data structure for workflow_validation_attempt audit events (ADR-034).

    Emitted when: HAPI validates LLM-recommended workflows
    Audit Event Type: workflow_validation_attempt
    Service: HolmesGPT API (HAPI)
    """
    event_id: str = Field(..., description="Unique event identifier")
    incident_id: str = Field(..., description="Incident correlation ID (remediation_id)")
    attempt: int = Field(..., ge=1, description="Current validation attempt number")
    max_attempts: int = Field(..., ge=1, description="Maximum validation attempts allowed")
    is_valid: bool = Field(..., description="Whether validation succeeded")
    errors: List[str] = Field(default_factory=list, description="List of validation error messages")
    validation_errors: Optional[str] = Field(None, description="Combined validation error messages (for backward compatibility)")
    workflow_id: Optional[str] = Field(None, description="Workflow ID being validated")
    workflow_name: Optional[str] = Field(None, description="Name of workflow being validated")
    human_review_reason: Optional[str] = Field(None, description="Reason code if needs_human_review (final attempt)")
    is_final_attempt: bool = Field(False, description="Whether this is the final validation attempt")


class HAPIResponseEventData(BaseModel):
    """
    event_data structure for holmesgpt.response.complete audit events (DD-AUDIT-005).

    Emitted when: HAPI completes incident analysis and returns IncidentResponse
    Audit Event Type: holmesgpt.response.complete
    Service: HolmesGPT API (HAPI)
    Business Requirement: BR-AUDIT-005 v2.0 (Gap #4 - AI Provider Data)
    Design Decision: DD-AUDIT-005 (Hybrid Provider Data Capture)

    Purpose: Captures complete HAPI API response (provider perspective) for:
    - SOC2 Type II compliance (RemediationRequest reconstruction)
    - Defense-in-depth auditing (provider + consumer perspectives)
    - Complete data integrity validation

    Note: This is the AUTHORITATIVE audit event for HAPI API responses.
    AI Analysis service emits complementary aianalysis.analysis.completed event
    with provider_response_summary (consumer perspective + business context).
    """
    event_id: str = Field(..., description="Unique event identifier")
    incident_id: str = Field(..., description="Incident correlation ID from request")
    response_data: Dict[str, Any] = Field(..., description="Complete IncidentResponse structure (all fields)")


