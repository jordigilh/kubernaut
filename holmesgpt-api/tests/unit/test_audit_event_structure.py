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
Unit Tests for Audit Event Structure

These are UNIT tests that validate audit event structure compliance with ADR-034.
They do NOT require any external infrastructure.

ADR-034 Required Fields (top-level):
  - version: Schema version ("1.0")
  - service: Service name ("holmesgpt-api")
  - event_type: Event type (e.g., "llm_request")
  - event_timestamp: ISO 8601 timestamp
  - correlation_id: Remediation ID for correlation
  - operation: Action performed
  - outcome: Result status ("success", "failure", "pending")
  - event_data: Service-specific payload (JSONB)

Moved from E2E per TESTING_GUIDELINES.md:
- Unit tests: "Focus on implementation correctness", "Execute quickly", "Have minimal external dependencies"
"""



class TestAuditEventStructure:
    """
    Unit tests for audit event structure compliance with ADR-034.

    These test the event factory functions in isolation.
    """

    def test_llm_request_event_structure(self):
        """ADR-034: LLM request event has required envelope and data fields."""
        from src.audit.events import create_llm_request_event

        event = create_llm_request_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            model="gpt-4",
            prompt="Test prompt",
            toolsets_enabled=["kubernetes/core"]
        )

        # ADR-034 envelope fields (top-level)
        envelope_fields = [
            "version", "event_category", "event_type", "event_timestamp",
            "correlation_id", "event_action", "event_outcome", "event_data"
        ]

        for field in envelope_fields:
            assert hasattr(event, field), f"Missing ADR-034 envelope field: {field}"

        # Verify envelope values
        assert event.version == "1.0"
        assert event.event_category == "analysis"  # ADR-034 v1.2: HolmesGPT API is "analysis" service
        assert event.event_type == "llm_request"
        assert event.correlation_id == "rem-456"
        assert event.event_action == "llm_request_sent"
        assert event.event_outcome == "success"

        # Service-specific fields (in event_data)
        event_data = event.event_data.actual_instance
        data_fields = [
            "event_id", "incident_id", "model",
            "prompt_length", "toolsets_enabled"
        ]

        for field in data_fields:
            assert hasattr(event_data, field), f"Missing event_data field: {field}"

        assert event_data.incident_id == "inc-123"
        assert event_data.model == "gpt-4"

    def test_llm_response_event_structure(self):
        """ADR-034: LLM response event has required envelope and data fields."""
        from src.audit.events import create_llm_response_event

        event = create_llm_response_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            has_analysis=True,
            analysis_length=1500,
            analysis_preview="Test analysis...",
            tool_call_count=2
        )

        # ADR-034 envelope fields (top-level)
        envelope_fields = [
            "version", "event_category", "event_type", "event_timestamp",
            "correlation_id", "event_action", "event_outcome", "event_data"
        ]

        for field in envelope_fields:
            assert hasattr(event, field), f"Missing ADR-034 envelope field: {field}"

        # Verify envelope values
        assert event.event_type == "llm_response"
        assert event.event_action == "llm_response_received"
        assert event.event_outcome == "success"  # has_analysis=True

        # Service-specific fields (in event_data)
        event_data = event.event_data.actual_instance
        data_fields = [
            "event_id", "incident_id",
            "has_analysis", "analysis_length", "tool_call_count"
        ]

        for field in data_fields:
            assert hasattr(event_data, field), f"Missing event_data field: {field}"

    def test_llm_response_failure_outcome(self):
        """ADR-034: LLM response with no analysis has failure outcome."""
        from src.audit.events import create_llm_response_event

        event = create_llm_response_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            has_analysis=False,
            analysis_length=0,
            analysis_preview="",
            tool_call_count=0
        )

        assert event.event_outcome == "failure"

    def test_validation_attempt_event_structure(self):
        """DD-HAPI-002 v1.2: Validation attempt event has required fields."""
        from src.audit.events import create_validation_attempt_event

        event = create_validation_attempt_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            attempt=1,
            max_attempts=3,
            is_valid=False,
            errors=["Workflow not found"],
            workflow_id="bad-workflow",
            human_review_reason="workflow_not_found"
        )

        # ADR-034 envelope fields (top-level)
        envelope_fields = [
            "version", "event_category", "event_type", "event_timestamp",
            "correlation_id", "event_action", "event_outcome", "event_data"
        ]

        for field in envelope_fields:
            assert hasattr(event, field), f"Missing ADR-034 envelope field: {field}"

        # Verify envelope values
        assert event.event_type == "workflow_validation_attempt"
        assert event.event_action == "validation_executed"
        assert event.event_outcome == "pending"  # Will retry (attempt 1 of 3)

        # Service-specific fields (in event_data)
        event_data = event.event_data.actual_instance
        data_fields = [
            "event_id", "incident_id",
            "attempt", "max_attempts", "is_valid",
            "errors", "workflow_id", "is_final_attempt"
        ]

        for field in data_fields:
            assert hasattr(event_data, field), f"Missing event_data field: {field}"

        assert event_data.is_final_attempt is False  # attempt 1 of 3

    def test_validation_attempt_final_attempt_flag(self):
        """DD-HAPI-002: is_final_attempt and outcome set correctly."""
        from src.audit.events import create_validation_attempt_event

        # Not final - outcome should be "pending"
        event1 = create_validation_attempt_event(
            incident_id="inc", remediation_id="rem",
            attempt=2, max_attempts=3,
            is_valid=False, errors=["Error"]
        )
        assert event1.event_data.actual_instance.is_final_attempt is False
        assert event1.event_outcome == "pending"

        # Final attempt - outcome should be "failure"
        event2 = create_validation_attempt_event(
            incident_id="inc", remediation_id="rem",
            attempt=3, max_attempts=3,
            is_valid=False, errors=["Error"]
        )
        assert event2.event_data.actual_instance.is_final_attempt is True
        assert event2.event_outcome == "failure"

        # Valid - outcome should be "success"
        event3 = create_validation_attempt_event(
            incident_id="inc", remediation_id="rem",
            attempt=1, max_attempts=3,
            is_valid=True, errors=[]
        )
        assert event3.event_outcome == "success"

    def test_tool_call_event_structure(self):
        """ADR-034: Tool call event has required envelope and data fields."""
        from src.audit.events import create_tool_call_event

        event = create_tool_call_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            tool_call_index=0,
            tool_name="search_workflow_catalog",
            tool_arguments={"query": "OOMKilled critical"},
            tool_result={"workflows": []}
        )

        # ADR-034 envelope fields (top-level)
        envelope_fields = [
            "version", "event_category", "event_type", "event_timestamp",
            "correlation_id", "event_action", "event_outcome", "event_data"
        ]

        for field in envelope_fields:
            assert hasattr(event, field), f"Missing ADR-034 envelope field: {field}"

        # Verify envelope values
        assert event.event_type == "llm_tool_call"
        assert event.event_action == "tool_invoked"
        assert event.event_outcome == "success"

        # Service-specific fields (in event_data)
        event_data = event.event_data.actual_instance
        data_fields = [
            "event_id", "incident_id",
            "tool_call_index", "tool_name",
            "tool_arguments", "tool_result"
        ]

        for field in data_fields:
            assert hasattr(event_data, field), f"Missing event_data field: {field}"

    def test_correlation_id_uses_remediation_id(self):
        """ADR-034: correlation_id maps to remediation_id for audit trail."""
        from src.audit.events import create_llm_request_event

        event = create_llm_request_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            model="gpt-4",
            prompt="Test",
            toolsets_enabled=[]
        )

        assert event.correlation_id == "rem-456"

    def test_empty_remediation_id_handled(self):
        """ADR-034: Empty remediation_id uses 'unknown' as correlation_id (minLength: 1)."""
        from src.audit.events import create_llm_request_event

        event = create_llm_request_event(
            incident_id="inc-123",
            remediation_id=None,
            model="gpt-4",
            prompt="Test",
            toolsets_enabled=[]
        )

        assert event.correlation_id == "unknown"
