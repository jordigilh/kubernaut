"""
Unit tests for LLM Audit Integration

Business Requirement: BR-AUDIT-005 - Workflow Selection Audit Trail
Design Decisions:
  - ADR-038: Asynchronous Buffered Audit Trace Ingestion
  - DD-AUDIT-002: Audit Shared Library Design

TDD Phase: RED (failing tests - audit integration needed in recovery.py)

Test Strategy:
1. Verify audit store is initialized with correct config
2. Verify LLM request audit events are stored
3. Verify LLM response audit events are stored
4. Verify tool call audit events are stored
5. Verify non-blocking behavior (fire-and-forget)
"""

import pytest
import uuid
from unittest.mock import Mock, patch, MagicMock
from typing import Dict, Any

from src.audit import BufferedAuditStore, AuditConfig


class TestLLMAuditIntegration:
    """
    Unit tests for LLM audit integration

    Business Requirement: BR-AUDIT-005
    Design Decisions: ADR-038, DD-AUDIT-002
    """

    def test_buffered_audit_store_initialization(self):
        """
        BR-AUDIT-005: BufferedAuditStore can be initialized with config

        BEHAVIOR: Store initializes with data_storage_url and config
        CORRECTNESS: Store has correct configuration

        TDD Phase: GREEN - BufferedAuditStore already exists
        """
        config = AuditConfig(
            buffer_size=1000,
            batch_size=10,
            flush_interval_seconds=1.0
        )

        store = BufferedAuditStore(
            data_storage_url="http://localhost:8080",
            config=config
        )

        assert store is not None
        assert store._config.buffer_size == 1000
        assert store._config.batch_size == 10

        # Cleanup
        store.close()

    def test_store_audit_event_non_blocking(self):
        """
        ADR-038: store_audit() must be non-blocking (fire-and-forget)

        BEHAVIOR: store_audit() returns immediately
        CORRECTNESS: Event is queued, not written synchronously

        TDD Phase: GREEN - BufferedAuditStore already implements this
        """
        config = AuditConfig(buffer_size=100, batch_size=10)
        store = BufferedAuditStore(
            data_storage_url="http://localhost:8080",
            config=config
        )

        # Store event (should not block)
        event = {
            "event_id": str(uuid.uuid4()),
            "event_type": "llm_request",
            "incident_id": "test-123",
            "model": "test-model"
        }

        result = store.store_audit(event)

        # Should return True (event queued)
        assert result is True

        # Event should be in queue, not written yet
        assert store._queue.qsize() == 1

        # Cleanup
        store.close()

    def test_llm_request_audit_event_structure(self):
        """
        BR-AUDIT-005: LLM request audit events have correct structure

        BEHAVIOR: Audit event contains all required fields
        CORRECTNESS: Fields match ADR-034 unified audit schema

        TDD Phase: RED - Need to define LLM audit event structure
        """
        # Define expected LLM request audit event
        event = create_llm_request_audit_event(
            incident_id="inc-12345",
            remediation_id="rem-67890",
            model="claude-3-5-sonnet",
            prompt="Test prompt",
            toolsets_enabled=["kubernetes/core", "workflow/catalog"]
        )

        # Required fields per ADR-034
        assert "event_id" in event
        assert "event_type" in event
        assert event["event_type"] == "llm_request"
        assert "timestamp" in event
        assert "incident_id" in event
        assert "remediation_id" in event
        assert "model" in event
        assert "prompt_length" in event
        assert "toolsets_enabled" in event

    def test_llm_response_audit_event_structure(self):
        """
        BR-AUDIT-005: LLM response audit events have correct structure

        BEHAVIOR: Audit event contains all required fields
        CORRECTNESS: Fields match ADR-034 unified audit schema

        TDD Phase: GREEN - Now using src/audit/events.py
        """
        # Define expected LLM response audit event
        event = create_llm_response_audit_event(
            incident_id="inc-12345",
            remediation_id="rem-67890",
            has_analysis=True,
            analysis_length=1500,
            analysis_preview="Test analysis preview",
            tool_call_count=3
        )

        # Required fields per ADR-034
        assert "event_id" in event
        assert "event_type" in event
        assert event["event_type"] == "llm_response"
        assert "timestamp" in event
        assert "incident_id" in event
        assert "remediation_id" in event
        assert "has_analysis" in event
        assert "analysis_length" in event
        assert "tool_call_count" in event

    def test_tool_call_audit_event_structure(self):
        """
        BR-AUDIT-005: Tool call audit events have correct structure

        BEHAVIOR: Audit event contains all required fields
        CORRECTNESS: Fields match ADR-034 unified audit schema

        TDD Phase: GREEN - Now using src/audit/events.py
        """
        # Define expected tool call audit event
        event = create_tool_call_audit_event(
            incident_id="inc-12345",
            remediation_id="rem-67890",
            tool_call_index=0,
            tool_name="search_workflow_catalog",
            tool_arguments={"query": "OOMKilled critical"},
            tool_result={"workflows": []}
        )

        # Required fields per ADR-034
        assert "event_id" in event
        assert "event_type" in event
        assert event["event_type"] == "llm_tool_call"
        assert "timestamp" in event
        assert "incident_id" in event
        assert "remediation_id" in event
        assert "tool_name" in event
        assert "tool_arguments" in event
        assert "tool_result" in event


# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Import audit event factory functions from src/audit/events.py
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
from src.audit.events import (
    create_llm_request_event as create_llm_request_audit_event,
    create_llm_response_event as create_llm_response_audit_event,
    create_tool_call_event as create_tool_call_audit_event,
)


if __name__ == "__main__":
    pytest.main([__file__, "-v", "--tb=short"])

