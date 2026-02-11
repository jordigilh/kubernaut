"""
Unit tests for LLM Audit Integration

Business Requirement: BR-AUDIT-005 - Workflow Selection Audit Trail
Design Decisions:
  - ADR-034: Unified Audit Table Design (AUTHORITATIVE schema)
  - ADR-038: Asynchronous Buffered Audit Trace Ingestion
  - DD-AUDIT-002: Audit Shared Library Design

Test Strategy:
1. Verify audit store is initialized with correct config
2. Verify LLM request audit events have ADR-034 compliant structure
3. Verify LLM response audit events have ADR-034 compliant structure
4. Verify tool call audit events have ADR-034 compliant structure
5. Verify non-blocking behavior (fire-and-forget)
"""

# Standard library imports
# Third-party imports
import pytest  # noqa: E402

# Local imports
from src.audit import BufferedAuditStore, AuditConfig  # noqa: E402
from src.audit.events import (  # noqa: E402
    create_llm_request_event as create_llm_request_audit_event,
    create_llm_response_event as create_llm_response_audit_event,
    create_tool_call_event as create_tool_call_audit_event,
)


class TestLLMAuditIntegration:
    """
    Unit tests for LLM audit integration

    Business Requirement: BR-AUDIT-005
    Design Decisions: ADR-034, ADR-038, DD-AUDIT-002
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
            "version": "1.0",
            "event_category": "test-service",
            "event_type": "aiagent.llm.request",
            "event_timestamp": "2025-01-01T00:00:00Z",
            "correlation_id": "test-123",
            "event_action": "test_op",
            "event_outcome": "success",
            "event_data": {"model": "test-model"}
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
        BR-AUDIT-005 + ADR-034: LLM request audit events have correct structure

        BEHAVIOR: Audit event contains ADR-034 envelope + event_data
        CORRECTNESS: Fields match ADR-034 unified audit schema
        """
        event = create_llm_request_audit_event(
            incident_id="inc-12345",
            remediation_id="rem-67890",
            model="claude-3-5-sonnet",
            prompt="Test prompt",
            toolsets_enabled=["kubernetes/core", "workflow/catalog"]
        )

        # ADR-034 envelope fields (top-level)
        assert "version" in event
        assert event["version"] == "1.0"
        assert "event_category" in event
        assert event["event_category"] == "aiagent"  # ADR-034 v1.2: HolmesGPT API is "aiagent" service (AI Agent Provider)
        assert "event_type" in event
        assert event["event_type"] == "aiagent.llm.request"
        assert "event_timestamp" in event
        assert "correlation_id" in event
        assert event["correlation_id"] == "rem-67890"
        assert "event_action" in event
        assert "event_outcome" in event
        assert "event_data" in event

        # Service-specific fields (in event_data)
        event_data = event["event_data"]
        assert "event_id" in event_data
        assert "incident_id" in event_data
        assert event_data["incident_id"] == "inc-12345"
        assert "model" in event_data
        assert event_data["model"] == "claude-3-5-sonnet"
        assert "prompt_length" in event_data
        assert "toolsets_enabled" in event_data

    def test_llm_response_audit_event_structure(self):
        """
        BR-AUDIT-005 + ADR-034: LLM response audit events have correct structure

        BEHAVIOR: Audit event contains ADR-034 envelope + event_data
        CORRECTNESS: Fields match ADR-034 unified audit schema
        """
        event = create_llm_response_audit_event(
            incident_id="inc-12345",
            remediation_id="rem-67890",
            has_analysis=True,
            analysis_length=1500,
            analysis_preview="Test analysis preview",
            tool_call_count=3
        )

        # ADR-034 envelope fields (top-level)
        assert "version" in event
        assert "event_category" in event
        assert "event_type" in event
        assert event["event_type"] == "aiagent.llm.response"
        assert "event_timestamp" in event
        assert "correlation_id" in event
        assert event["correlation_id"] == "rem-67890"
        assert "event_action" in event
        assert "event_outcome" in event
        assert "event_data" in event

        # Service-specific fields (in event_data)
        event_data = event["event_data"]
        assert "event_id" in event_data
        assert "incident_id" in event_data
        assert "has_analysis" in event_data
        assert "analysis_length" in event_data
        assert "tool_call_count" in event_data

    def test_tool_call_audit_event_structure(self):
        """
        BR-AUDIT-005 + ADR-034: Tool call audit events have correct structure

        BEHAVIOR: Audit event contains ADR-034 envelope + event_data
        CORRECTNESS: Fields match ADR-034 unified audit schema
        """
        event = create_tool_call_audit_event(
            incident_id="inc-12345",
            remediation_id="rem-67890",
            tool_call_index=0,
            tool_name="search_workflow_catalog",
            tool_arguments={"query": "OOMKilled critical"},
            tool_result={"workflows": []}
        )

        # ADR-034 envelope fields (top-level)
        assert "version" in event
        assert "event_category" in event
        assert "event_type" in event
        assert event["event_type"] == "aiagent.llm.tool_call"
        assert "event_timestamp" in event
        assert "correlation_id" in event
        assert event["correlation_id"] == "rem-67890"
        assert "event_action" in event
        assert "event_outcome" in event
        assert "event_data" in event

        # Service-specific fields (in event_data)
        event_data = event["event_data"]
        assert "event_id" in event_data
        assert "incident_id" in event_data
        assert "tool_name" in event_data
        assert event_data["tool_name"] == "search_workflow_catalog"
        assert "tool_arguments" in event_data
        assert "tool_result" in event_data


if __name__ == "__main__":
    pytest.main([__file__, "-v", "--tb=short"])
