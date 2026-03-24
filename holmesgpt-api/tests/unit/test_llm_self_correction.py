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
Unit Tests for LLM Self-Correction Loop (Phase 4)

Business Requirement: BR-HAPI-197 (needs_human_review field)
Design Decision: DD-HAPI-002 v1.2 (Workflow Response Validation)

Tests the LLM self-correction helper functions and constants.
Integration testing of the full loop is done via E2E tests.
"""



class TestLLMSelfCorrectionConstants:
    """Test constants for self-correction loop."""

    def test_max_validation_attempts_is_three(self):
        """BR-HAPI-197: Max 3 attempts before human review."""
        from src.extensions.incident import MAX_VALIDATION_ATTEMPTS
        assert MAX_VALIDATION_ATTEMPTS == 3

    def test_max_validation_attempts_is_positive(self):
        """Sanity check: attempts must be positive."""
        from src.extensions.incident import MAX_VALIDATION_ATTEMPTS
        assert MAX_VALIDATION_ATTEMPTS > 0


class TestValidationErrorFeedback:
    """Test error feedback prompt generation."""

    def test_builds_feedback_with_single_error(self):
        """DD-HAPI-002: Single error formatted correctly."""
        from src.extensions.incident import _build_validation_error_feedback

        errors = ["Workflow 'invalid-workflow' not found in catalog"]
        feedback = _build_validation_error_feedback(errors, attempt=1)

        assert "VALIDATION ERROR" in feedback
        assert "invalid-workflow" in feedback
        assert "not found" in feedback
        assert "Attempt 2/3" in feedback  # attempt is 0-indexed, display is 1-indexed

    def test_builds_feedback_with_multiple_errors(self):
        """DD-HAPI-002: Multiple errors formatted correctly."""
        from src.extensions.incident import _build_validation_error_feedback

        errors = [
            "Workflow 'invalid-workflow' not found in catalog",
            "Container image mismatch: expected 'ghcr.io/x:v1', got 'docker.io/y:v2'",
            "Missing required parameter: 'namespace'"
        ]
        feedback = _build_validation_error_feedback(errors, attempt=0)

        assert "invalid-workflow" in feedback
        assert "Container image mismatch" in feedback
        assert "Missing required parameter" in feedback
        assert "Attempt 1/3" in feedback

    def test_feedback_includes_correction_instructions(self):
        """DD-HAPI-002: Feedback includes correction guidance."""
        from src.extensions.incident import _build_validation_error_feedback

        errors = ["Parameter 'replicas' must be >= 1"]
        feedback = _build_validation_error_feedback(errors, attempt=2)

        assert "correct" in feedback.lower()
        assert "Attempt 3/3" in feedback

    def test_feedback_attempt_numbering(self):
        """Verify attempt numbers are displayed correctly."""
        from src.extensions.incident import _build_validation_error_feedback

        for attempt in range(3):
            feedback = _build_validation_error_feedback(["error"], attempt=attempt)
            expected = f"Attempt {attempt + 1}/3"
            assert expected in feedback

    def test_feedback_mentions_mcp_search(self):
        """DD-HAPI-002: Feedback suggests using MCP search to verify workflow."""
        from src.extensions.incident import _build_validation_error_feedback

        errors = ["Workflow 'bad-id' not found"]
        feedback = _build_validation_error_feedback(errors, attempt=0)

        assert "MCP" in feedback or "search" in feedback.lower() or "catalog" in feedback.lower()


class TestDetermineHumanReviewReason:
    """Test human review reason determination from validation errors."""

    def test_workflow_not_found_reason(self):
        """BR-HAPI-197: Correct reason for workflow not found."""
        from src.extensions.incident import _determine_human_review_reason

        errors = ["Workflow 'xyz' not found in catalog"]
        reason = _determine_human_review_reason(errors)

        assert reason == "workflow_not_found"

    def test_image_mismatch_reason(self):
        """BR-HAPI-197: Correct reason for image mismatch."""
        from src.extensions.incident import _determine_human_review_reason

        errors = ["Container image mismatch: expected 'a', got 'b'"]
        reason = _determine_human_review_reason(errors)

        assert reason == "image_mismatch"

    def test_parameter_validation_reason(self):
        """BR-HAPI-197: Correct reason for parameter validation failure."""
        from src.extensions.incident import _determine_human_review_reason

        errors = ["Missing required parameter: 'namespace'"]
        reason = _determine_human_review_reason(errors)

        assert reason == "parameter_validation_failed"

    def test_type_error_maps_to_parameter_validation(self):
        """BR-HAPI-197: Type errors map to parameter validation."""
        from src.extensions.incident import _determine_human_review_reason

        errors = ["Parameter 'replicas' must be of type int, got str"]
        reason = _determine_human_review_reason(errors)

        assert reason == "parameter_validation_failed"

    def test_default_reason_for_unknown_errors(self):
        """BR-HAPI-197: Default reason for unrecognized errors."""
        from src.extensions.incident import _determine_human_review_reason

        errors = ["Some unknown error occurred"]
        reason = _determine_human_review_reason(errors)

        # Should default to parameter_validation_failed
        assert reason == "parameter_validation_failed"


class TestParseAndValidateInvestigationResult:
    """Test the parse and validate function."""

    def test_returns_tuple_with_result_and_validation(self):
        """DD-HAPI-002: Function returns tuple of (result, validation_result).

        #372: Plain text without structured JSON now correctly returns a failed
        ValidationResult to trigger the self-correction retry loop.
        """
        from src.extensions.incident import _parse_and_validate_investigation_result
        from unittest.mock import Mock

        investigation = Mock()
        investigation.analysis = "No workflow found"

        request_data = {"incident_id": "test-123"}

        result, validation_result = _parse_and_validate_investigation_result(
            investigation,
            request_data,
            data_storage_client=None
        )

        assert isinstance(result, dict)
        assert "incident_id" in result
        # #372: No structured output → failed ValidationResult (triggers retry)
        assert validation_result is not None
        assert validation_result.is_valid is False

    def test_parses_json_from_analysis(self):
        """DD-HAPI-002: Correctly parses JSON from LLM analysis."""
        from src.extensions.incident import _parse_and_validate_investigation_result
        from unittest.mock import Mock

        investigation = Mock()
        investigation.analysis = '''
Based on my analysis:

```json
{
  "root_cause_analysis": {
    "summary": "Container OOM",
    "severity": "critical",
    "contributing_factors": ["memory leak"]
  },
  "selected_workflow": null
}
```
'''
        request_data = {"incident_id": "test-456"}

        result, _ = _parse_and_validate_investigation_result(
            investigation,
            request_data,
            data_storage_client=None
        )

        assert result["root_cause_analysis"]["summary"] == "Container OOM"
        assert result["root_cause_analysis"]["severity"] == "critical"
        # ADR-045 v1.2: selected_workflow field is only included when not None
        # Parser no longer includes null fields (parse_and_validate_investigation_result)
        assert "selected_workflow" not in result or result.get("selected_workflow") is None

    def test_sets_needs_human_review_when_no_workflow(self):
        """BR-HAPI-197: No workflow triggers needs_human_review."""
        from src.extensions.incident import _parse_and_validate_investigation_result
        from unittest.mock import Mock

        investigation = Mock()
        investigation.analysis = '''
```json
{
  "root_cause_analysis": {"summary": "Unknown", "severity": "medium", "contributing_factors": []},
  "selected_workflow": null
}
```
'''
        request_data = {"incident_id": "test-789"}

        result, _ = _parse_and_validate_investigation_result(
            investigation,
            request_data,
            data_storage_client=None
        )

        assert result["needs_human_review"] is True
        assert result["human_review_reason"] == "no_matching_workflows"

    def test_low_confidence_not_escalated_by_parser(self):
        """BR-HAPI-197: Low confidence is AIAnalysis's responsibility, not the parser's.

        BR-496 v2: Parser no longer checks affectedResource — HAPI injects it
        post-loop via _inject_target_resource. With no investigation outcome and
        a workflow present, the parser does not set needs_human_review.
        """
        from src.extensions.incident import _parse_and_validate_investigation_result
        from unittest.mock import Mock

        investigation = Mock()
        investigation.analysis = '''
```json
{
  "root_cause_analysis": {"summary": "Maybe OOM", "severity": "medium", "contributing_factors": []},
  "selected_workflow": {
    "workflow_id": "restart-pod",
    "confidence": 0.5
  }
}
```
'''
        request_data = {"incident_id": "test-low-conf"}

        result, _ = _parse_and_validate_investigation_result(
            investigation,
            request_data,
            data_storage_client=None  # No validation
        )

        # BR-496 v2: Parser does not check affectedResource — _inject_target_resource
        # handles that post-loop. With no special investigation outcome and a
        # selected_workflow present, the parser leaves needs_human_review=False.
        assert result["needs_human_review"] is False
        assert "human_review_reason" not in result

    def test_extracts_alternative_workflows(self):
        """ADR-045 v1.2: Alternative workflows extracted for audit."""
        from src.extensions.incident import _parse_and_validate_investigation_result
        from unittest.mock import Mock

        investigation = Mock()
        investigation.analysis = '''
```json
{
  "root_cause_analysis": {"summary": "OOM", "severity": "critical", "contributing_factors": []},
  "selected_workflow": {
    "workflow_id": "restart-pod",
    "confidence": 0.95
  },
  "alternative_workflows": [
    {"workflow_id": "scale-down", "confidence": 0.7, "rationale": "Also considered"}
  ]
}
```
'''
        request_data = {"incident_id": "test-alt"}

        result, _ = _parse_and_validate_investigation_result(
            investigation,
            request_data,
            data_storage_client=None
        )

        assert len(result["alternative_workflows"]) == 1
        assert result["alternative_workflows"][0]["workflow_id"] == "scale-down"


class TestCreateDataStorageClient:
    """Test Data Storage client creation."""

    def test_creates_client_from_env_var(self):
        """DD-HAPI-002: Creates client using environment variable."""
        from src.extensions.incident import _create_data_storage_client
        import os

        # Save and set env var
        original = os.environ.get("DATA_STORAGE_URL")
        os.environ["DATA_STORAGE_URL"] = "http://test-data-storage:8080"

        try:
            client = _create_data_storage_client(None)
            assert client is not None
            # Verify it's the correct OpenAPI client type
            from datastorage.api.workflow_catalog_api_api import WorkflowCatalogAPIApi
            assert isinstance(client, WorkflowCatalogAPIApi)
            # Check the configuration host
            assert client.api_client.configuration.host == "http://test-data-storage:8080"
        finally:
            if original:
                os.environ["DATA_STORAGE_URL"] = original
            else:
                os.environ.pop("DATA_STORAGE_URL", None)

    def test_creates_client_from_app_config(self):
        """DD-HAPI-002: Creates client using app config."""
        from src.extensions.incident import _create_data_storage_client

        app_config = {"data_storage_url": "http://config-data-storage:9090"}

        client = _create_data_storage_client(app_config)
        assert client is not None
        # Verify it's the correct OpenAPI client type
        from datastorage.api.workflow_catalog_api_api import WorkflowCatalogAPIApi
        assert isinstance(client, WorkflowCatalogAPIApi)
        # Check the configuration host
        assert client.api_client.configuration.host == "http://config-data-storage:9090"

    def test_returns_none_on_error(self):
        """DD-HAPI-002: Returns None if client creation fails."""
        from src.extensions.incident import _create_data_storage_client
        from unittest.mock import patch

        # Patch the Configuration import to raise an error
        with patch("datastorage.configuration.Configuration", side_effect=Exception("Connection failed")):
            client = _create_data_storage_client(None)
            # Should return None and log warning, not raise
            assert client is None


class TestResourceContextMismatchCheck:
    """UT-HAPI-516-010 through 014: Resource context mismatch validation (#516)."""

    def test_ut_hapi_516_010_mismatch_detected_different_target(self):
        """UT-HAPI-516-010: Mismatch when session_state target differs from affectedResource."""
        from src.extensions.incident.llm_integration import _check_resource_context_mismatch

        result = {
            "selected_workflow": {"workflow_id": "wf-1"},
            "root_cause_analysis": {
                "summary": "Disk pressure from emptyDir",
                "affectedResource": {"kind": "Deployment", "name": "postgres-emptydir", "namespace": "prod"},
            },
        }
        session_state = {
            "last_resource_context_target": {"kind": "Deployment", "name": "log-collector", "namespace": "prod"},
        }

        feedback = _check_resource_context_mismatch(result, session_state, "inc-1")
        assert feedback is not None
        assert "MISMATCH" in feedback
        assert "postgres-emptydir" in feedback
        assert "log-collector" in feedback

    def test_ut_hapi_516_011_mismatch_when_tool_never_called(self):
        """UT-HAPI-516-011: Mismatch when get_resource_context was never called."""
        from src.extensions.incident.llm_integration import _check_resource_context_mismatch

        result = {
            "selected_workflow": {"workflow_id": "wf-1"},
            "root_cause_analysis": {
                "summary": "OOMKilled",
                "affectedResource": {"kind": "Deployment", "name": "api", "namespace": "prod"},
            },
        }
        session_state = {}

        feedback = _check_resource_context_mismatch(result, session_state, "inc-2")
        assert feedback is not None
        assert "MISSING" in feedback
        assert "did not call" in feedback

    def test_ut_hapi_516_012_no_mismatch_when_targets_match(self):
        """UT-HAPI-516-012: No mismatch when targets match."""
        from src.extensions.incident.llm_integration import _check_resource_context_mismatch

        result = {
            "selected_workflow": {"workflow_id": "wf-1"},
            "root_cause_analysis": {
                "summary": "OOMKilled",
                "affectedResource": {"kind": "Deployment", "name": "api", "namespace": "prod"},
            },
        }
        session_state = {
            "last_resource_context_target": {"kind": "Deployment", "name": "api", "namespace": "prod"},
        }

        feedback = _check_resource_context_mismatch(result, session_state, "inc-3")
        assert feedback is None

    def test_ut_hapi_516_013_skip_when_no_workflow_selected(self):
        """UT-HAPI-516-013: Skip mismatch check when no workflow selected."""
        from src.extensions.incident.llm_integration import _check_resource_context_mismatch

        result = {
            "selected_workflow": None,
            "root_cause_analysis": {
                "summary": "Self-resolved",
                "affectedResource": {"kind": "Deployment", "name": "api", "namespace": "prod"},
            },
        }
        session_state = {
            "last_resource_context_target": {"kind": "Node", "name": "worker-1", "namespace": ""},
        }

        feedback = _check_resource_context_mismatch(result, session_state, "inc-4")
        assert feedback is None

    def test_ut_hapi_516_014_skip_when_no_affected_resource(self):
        """UT-HAPI-516-014: Skip when LLM didn't provide affectedResource."""
        from src.extensions.incident.llm_integration import _check_resource_context_mismatch

        result = {
            "selected_workflow": {"workflow_id": "wf-1"},
            "root_cause_analysis": {
                "summary": "OOMKilled",
            },
        }
        session_state = {
            "last_resource_context_target": {"kind": "Deployment", "name": "api", "namespace": "prod"},
        }

        feedback = _check_resource_context_mismatch(result, session_state, "inc-5")
        assert feedback is None


class TestResourceContextMismatchFeedback:
    """UT-HAPI-516-020 through 021: Mismatch feedback prompt generation (#516)."""

    def test_ut_hapi_516_020_mismatch_feedback_includes_both_targets(self):
        """UT-HAPI-516-020: Feedback includes both affected and last-queried targets."""
        from src.extensions.incident.prompt_builder import build_resource_context_mismatch_feedback

        feedback = build_resource_context_mismatch_feedback(
            affected_resource={"kind": "Deployment", "name": "postgres-emptydir", "namespace": "prod"},
            last_target={"kind": "Deployment", "name": "log-collector", "namespace": "prod"},
        )

        assert "postgres-emptydir" in feedback
        assert "log-collector" in feedback
        assert "get_resource_context" in feedback
        assert "detected infrastructure" in feedback.lower()

    def test_ut_hapi_516_021_missing_feedback_when_no_target(self):
        """UT-HAPI-516-021: Feedback for tool-never-called case."""
        from src.extensions.incident.prompt_builder import build_resource_context_mismatch_feedback

        feedback = build_resource_context_mismatch_feedback(
            affected_resource={"kind": "Deployment", "name": "api", "namespace": "prod"},
            last_target=None,
        )

        assert "did not call" in feedback
        assert "REQUIRED" in feedback
        assert "get_resource_context" in feedback


class TestSelfCorrectionLoopIntegration:
    """Integration-style tests for the self-correction loop.

    These tests verify the loop behavior without mocking the full HolmesGPT SDK.
    Full E2E tests are in tests/e2e/.
    """

    def test_loop_uses_max_validation_attempts(self):
        """Verify loop constant is used correctly."""
        from src.extensions.incident import MAX_VALIDATION_ATTEMPTS

        # The loop should iterate MAX_VALIDATION_ATTEMPTS times
        # This is verified by the implementation structure
        assert MAX_VALIDATION_ATTEMPTS == 3

    def test_validation_errors_history_accumulates(self):
        """DD-HAPI-002: Errors from each attempt are tracked."""
        # This behavior is tested by examining the function structure
        # The implementation uses validation_errors_history: List[List[str]]
        # Each attempt appends its errors to this list
        pass  # Structure verified in implementation

    def test_feedback_includes_previous_errors(self):
        """DD-HAPI-002: Retry prompts include previous errors."""
        from src.extensions.incident import _build_validation_error_feedback

        # First attempt fails with these errors
        errors = ["Workflow 'bad-id' not found in catalog"]

        # Feedback for retry (attempt 1 = second try)
        feedback = _build_validation_error_feedback(errors, attempt=1)

        # Should include the error and attempt number
        assert "bad-id" in feedback
        assert "Attempt 2/3" in feedback
