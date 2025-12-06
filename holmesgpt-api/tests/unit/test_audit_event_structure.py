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

Moved from E2E per TESTING_GUIDELINES.md:
- Unit tests: "Focus on implementation correctness", "Execute quickly", "Have minimal external dependencies"
"""

import pytest


class TestAuditEventStructure:
    """
    Unit tests for audit event structure compliance with ADR-034.

    These test the event factory functions in isolation.
    """

    def test_llm_request_event_structure(self):
        """ADR-034: LLM request event has required fields."""
        from src.audit.events import create_llm_request_event

        event = create_llm_request_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            model="gpt-4",
            prompt="Test prompt",
            toolsets_enabled=["kubernetes/core"]
        )

        required_fields = [
            "event_id", "event_type", "timestamp",
            "incident_id", "remediation_id", "model",
            "prompt_length", "toolsets_enabled"
        ]

        for field in required_fields:
            assert field in event, f"Missing required field: {field}"

        assert event["event_type"] == "llm_request"

    def test_llm_response_event_structure(self):
        """ADR-034: LLM response event has required fields."""
        from src.audit.events import create_llm_response_event

        event = create_llm_response_event(
            incident_id="inc-123",
            remediation_id="rem-456",
            has_analysis=True,
            analysis_length=1500,
            analysis_preview="Test analysis...",
            tool_call_count=2
        )

        required_fields = [
            "event_id", "event_type", "timestamp",
            "incident_id", "remediation_id",
            "has_analysis", "analysis_length", "tool_call_count"
        ]

        for field in required_fields:
            assert field in event, f"Missing required field: {field}"

        assert event["event_type"] == "llm_response"

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

        required_fields = [
            "event_id", "event_type", "timestamp",
            "incident_id", "remediation_id",
            "attempt", "max_attempts", "is_valid",
            "errors", "workflow_id", "is_final_attempt"
        ]

        for field in required_fields:
            assert field in event, f"Missing required field: {field}"

        assert event["event_type"] == "workflow_validation_attempt"
        assert event["is_final_attempt"] is False  # attempt 1 of 3

    def test_validation_attempt_final_attempt_flag(self):
        """DD-HAPI-002: is_final_attempt set correctly."""
        from src.audit.events import create_validation_attempt_event

        # Not final
        event1 = create_validation_attempt_event(
            incident_id="inc", remediation_id="rem",
            attempt=2, max_attempts=3,
            is_valid=False, errors=["Error"]
        )
        assert event1["is_final_attempt"] is False

        # Final attempt
        event2 = create_validation_attempt_event(
            incident_id="inc", remediation_id="rem",
            attempt=3, max_attempts=3,
            is_valid=False, errors=["Error"]
        )
        assert event2["is_final_attempt"] is True

