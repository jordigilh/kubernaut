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
Recovery Validation Loop Unit Tests

Business Requirement: BR-HAPI-017-004
Design Decision: DD-HAPI-002 v1.2 (LLM Self-Correction Loop)

Tests the recovery validation loop that retries LLM investigation
on validation failure, injecting error feedback for self-correction
up to MAX_VALIDATION_ATTEMPTS (3).

Test Plan: docs/testing/DD-HAPI-017/TEST_PLAN.md - Phase 4
Test Scenarios: UT-HAPI-017-004-001 through UT-HAPI-017-004-004
"""

import pytest
from unittest.mock import Mock, patch
from src.validation.workflow_response_validator import ValidationResult


# Must match recovery/constants.py
MAX_VALIDATION_ATTEMPTS = 3

# Module path for patching
MODULE = "src.extensions.recovery.llm_integration"


def _make_investigation_result(analysis="mock analysis"):
    """Create a mock HolmesGPT InvestigationResult."""
    result = Mock()
    result.analysis = analysis
    result.tool_calls = []
    return result


def _make_parsed_result(workflow_id="wf-test-001", needs_human_review=False):
    """Create a mock parsed recovery result dict (fresh copy each call)."""
    result = {
        "incident_id": "inc-test",
        "is_recovery_attempt": True,
        "recovery_attempt_number": 1,
        "can_recover": True,
        "strategies": [],
        "primary_recommendation": None,
        "analysis_confidence": 0.85,
        "warnings": [],
        "metadata": {},
        "recovery_analysis": {},
        "raw_analysis": "mock",
        "needs_human_review": needs_human_review,
        "human_review_reason": None,
    }
    if workflow_id:
        result["selected_workflow"] = {
            "workflow_id": workflow_id,
            "container_image": "registry.io/wf:1.0",
            "parameters": {},
            "confidence": 0.85,
        }
    return result


def _make_request_data():
    """Minimal request data for analyze_recovery."""
    return {
        "incident_id": "inc-test",
        "is_recovery_attempt": True,
        "recovery_attempt_number": 1,
        "signal_type": "OOMKilled",
        "severity": "critical",
        "resource_namespace": "production",
        "resource_kind": "Deployment",
        "resource_name": "api-server",
        "failed_action": {"type": "restart"},
        "failure_context": {},
        "previous_execution": {
            "original_rca": {
                "summary": "Memory exhaustion",
                "signal_type": "OOMKilled",
                "severity": "critical",
            },
            "selected_workflow": {"workflow_id": "scale-v1", "version": "1.0.0"},
            "failure": {
                "reason": "OOMKilled",
                "message": "OOMKilled",
                "failed_step_index": 1,
                "failed_step_name": "scale",
            },
        },
        "context": {"cluster": "test"},
        "enrichment_results": {},
        "remediation_id": "rem-test-001",
    }


@pytest.fixture
def mock_recovery_env():
    """Shared mock environment for recovery validation loop tests.

    Provides patched HolmesGPT SDK components and audit infrastructure
    needed by all recovery validation tests. Eliminates 8-patch boilerplate
    duplication across test methods.
    """
    with (
        patch(f"{MODULE}._get_holmes_config") as mock_config,
        patch(f"{MODULE}.investigate_issues") as mock_investigate,
        patch(f"{MODULE}.get_audit_store") as mock_audit,
        patch(f"{MODULE}._parse_investigation_result") as mock_parse,
        patch(f"{MODULE}.create_data_storage_client") as mock_ds,
        patch(f"{MODULE}.WorkflowResponseValidator") as mock_validator_cls,
        patch(f"{MODULE}.build_validation_error_feedback") as mock_feedback,
        patch("src.sanitization.sanitize_for_llm", side_effect=lambda x: x),
        # DD-HAPI-017: Patch discovery toolset registration (no-op in unit tests)
        patch(
            "src.extensions.llm_config.register_workflow_discovery_toolset",
            side_effect=lambda config, *a, **kw: config,
        ),
    ):
        mock_config.return_value = Mock(model="test", toolsets={}, mcp_servers={})
        mock_audit.return_value = Mock(store_audit=Mock())
        mock_investigate.return_value = _make_investigation_result()
        mock_ds.return_value = Mock()

        yield {
            "config": mock_config,
            "investigate": mock_investigate,
            "audit": mock_audit,
            "parse": mock_parse,
            "ds": mock_ds,
            "validator_cls": mock_validator_cls,
            "feedback": mock_feedback,
        }


class TestRecoveryValidationLoop:
    """
    Test recovery validation loop (DD-HAPI-002 v1.2, BR-HAPI-017-004).

    The recovery flow should retry LLM investigation on validation failure,
    injecting error feedback for self-correction up to MAX_VALIDATION_ATTEMPTS.
    """

    @pytest.mark.asyncio
    async def test_executes_max_validation_attempts_ut_004_001(self, mock_recovery_env):
        """
        UT-HAPI-017-004-001: Recovery validation loop executes up to MAX_VALIDATION_ATTEMPTS.

        BR: BR-HAPI-017-004
        Type: Unit / Happy Path
        When validation fails on all attempts, investigate_issues is called
        exactly MAX_VALIDATION_ATTEMPTS (3) times, then flow terminates.
        """
        mocks = mock_recovery_env
        mocks["parse"].side_effect = lambda *args, **kwargs: _make_parsed_result(
            workflow_id="wf-bad"
        )
        mocks["feedback"].return_value = "\n## Error: unknown workflow_id\n"

        # Validator always fails
        mock_validator = Mock()
        mock_validator.validate.return_value = ValidationResult(
            is_valid=False, errors=["unknown workflow_id"]
        )
        mocks["validator_cls"].return_value = mock_validator

        from src.extensions.recovery.llm_integration import analyze_recovery

        result = await analyze_recovery(_make_request_data())

        # Assert: investigate_issues called exactly MAX_VALIDATION_ATTEMPTS times
        assert mocks["investigate"].call_count == MAX_VALIDATION_ATTEMPTS

    @pytest.mark.asyncio
    async def test_injects_feedback_on_retry_ut_004_002(self, mock_recovery_env):
        """
        UT-HAPI-017-004-002: Recovery validation loop injects feedback on retry.

        BR: BR-HAPI-017-004
        Type: Unit / Happy Path
        On retry, validation errors from previous attempt are injected
        into the prompt. Second call succeeds after self-correction.
        """
        mocks = mock_recovery_env
        mocks["parse"].side_effect = lambda *args, **kwargs: _make_parsed_result(
            workflow_id="wf-test"
        )

        # Feedback returns identifiable string
        mocks["feedback"].return_value = (
            "\n## VALIDATION_ERROR_FEEDBACK: unknown workflow_id\n"
        )

        # Validator: fail on first call, succeed on second
        mock_validator = Mock()
        mock_validator.validate.side_effect = [
            ValidationResult(
                is_valid=False, errors=["unknown workflow_id"]
            ),
            ValidationResult(is_valid=True, errors=[]),
        ]
        mocks["validator_cls"].return_value = mock_validator

        from src.extensions.recovery.llm_integration import analyze_recovery

        result = await analyze_recovery(_make_request_data())

        # Should have been called 2 times (fail then succeed)
        assert mocks["investigate"].call_count == 2

        # The second call's prompt should contain the error feedback
        second_call = mocks["investigate"].call_args_list[1]
        second_request = second_call.kwargs.get("investigate_request")
        assert "VALIDATION_ERROR_FEEDBACK" in second_request.description

    @pytest.mark.asyncio
    async def test_sets_needs_human_review_after_exhausting_ut_004_003(self, mock_recovery_env):
        """
        UT-HAPI-017-004-003: Recovery sets needs_human_review after exhausting attempts.

        BR: BR-HAPI-017-004
        Type: Unit / Error Handling
        After 3 failed validation attempts, recovery marks result as
        needs_human_review=True with human_review_reason containing
        validation failure details.
        """
        mocks = mock_recovery_env
        mocks["parse"].side_effect = lambda *args, **kwargs: _make_parsed_result(
            workflow_id="wf-bad", needs_human_review=False
        )
        mocks["feedback"].return_value = "\n## Error\n"

        # Validator always fails
        mock_validator = Mock()
        mock_validator.validate.return_value = ValidationResult(
            is_valid=False, errors=["unknown workflow_id"]
        )
        mocks["validator_cls"].return_value = mock_validator

        from src.extensions.recovery.llm_integration import analyze_recovery

        result = await analyze_recovery(_make_request_data())

        # After exhausting all attempts, needs_human_review must be True
        assert result["needs_human_review"] is True
        assert result.get("human_review_reason") is not None

    @pytest.mark.asyncio
    async def test_succeeds_on_retry_ut_004_004(self, mock_recovery_env):
        """
        UT-HAPI-017-004-004: Recovery validation loop succeeds on retry.

        BR: BR-HAPI-017-004
        Type: Unit / Happy Path
        Recovery flow succeeds when LLM self-corrects on second attempt.
        Loop exits early on success — no unnecessary retries.
        """
        mocks = mock_recovery_env
        mocks["parse"].side_effect = lambda *args, **kwargs: _make_parsed_result(
            workflow_id="wf-good"
        )
        mocks["feedback"].return_value = "\n## Error\n"

        # Validator: fail first, succeed second
        mock_validator = Mock()
        mock_validator.validate.side_effect = [
            ValidationResult(
                is_valid=False, errors=["wrong container image"]
            ),
            ValidationResult(is_valid=True, errors=[]),
        ]
        mocks["validator_cls"].return_value = mock_validator

        from src.extensions.recovery.llm_integration import analyze_recovery

        result = await analyze_recovery(_make_request_data())

        # Should have been called exactly 2 times (not 3)
        assert mocks["investigate"].call_count == 2
        # Should NOT set needs_human_review (validation succeeded)
        assert result.get("needs_human_review", False) is False
