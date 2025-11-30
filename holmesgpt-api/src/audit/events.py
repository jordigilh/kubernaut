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
  - ADR-034: Unified Audit Table Design
  - ADR-038: Asynchronous Buffered Audit Trace Ingestion
  - DD-AUDIT-002: Audit Shared Library Design

This module provides factory functions for creating LLM audit events.
Events are structured per ADR-034 unified audit schema.

Event Types:
  - llm_request: LLM prompt sent to model
  - llm_response: LLM analysis response received
  - llm_tool_call: LLM tool invocation (e.g., search_workflow_catalog)

Usage:
    from src.audit.events import (
        create_llm_request_event,
        create_llm_response_event,
        create_tool_call_event
    )

    # Create event
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

import datetime
import uuid
from typing import Dict, Any, List, Optional


def create_llm_request_event(
    incident_id: str,
    remediation_id: Optional[str],
    model: str,
    prompt: str,
    toolsets_enabled: List[str],
    mcp_servers: Optional[List[str]] = None
) -> Dict[str, Any]:
    """
    Create an LLM request audit event

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
        Structured audit event dictionary
    """
    return {
        "event_id": str(uuid.uuid4()),
        "event_type": "llm_request",
        "timestamp": datetime.datetime.utcnow().isoformat() + "Z",
        "incident_id": incident_id,
        "remediation_id": remediation_id or "",
        "model": model,
        "prompt_length": len(prompt),
        "prompt_preview": prompt[:500] + "..." if len(prompt) > 500 else prompt,
        "toolsets_enabled": toolsets_enabled,
        "mcp_servers": mcp_servers or [],
    }


def create_llm_response_event(
    incident_id: str,
    remediation_id: Optional[str],
    has_analysis: bool,
    analysis_length: int,
    analysis_preview: str,
    tool_call_count: int
) -> Dict[str, Any]:
    """
    Create an LLM response audit event

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
        Structured audit event dictionary
    """
    return {
        "event_id": str(uuid.uuid4()),
        "event_type": "llm_response",
        "timestamp": datetime.datetime.utcnow().isoformat() + "Z",
        "incident_id": incident_id,
        "remediation_id": remediation_id or "",
        "has_analysis": has_analysis,
        "analysis_length": analysis_length,
        "analysis_preview": analysis_preview,
        "tool_call_count": tool_call_count,
    }


def create_tool_call_event(
    incident_id: str,
    remediation_id: Optional[str],
    tool_call_index: int,
    tool_name: str,
    tool_arguments: Dict[str, Any],
    tool_result: Any
) -> Dict[str, Any]:
    """
    Create a tool call audit event

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
        Structured audit event dictionary
    """
    return {
        "event_id": str(uuid.uuid4()),
        "event_type": "llm_tool_call",
        "timestamp": datetime.datetime.utcnow().isoformat() + "Z",
        "incident_id": incident_id,
        "remediation_id": remediation_id or "",
        "tool_call_index": tool_call_index,
        "tool_name": tool_name,
        "tool_arguments": tool_arguments,
        "tool_result": tool_result,
    }



