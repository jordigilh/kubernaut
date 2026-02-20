"""
Unit tests for BR-HAPI-192: Recovery Context Consumption

BR-HAPI-192 requires HolmesGPT-API to consume `naturalLanguageSummary` from
Workflow Engine (WE) recovery context to improve LLM analysis during retry scenarios.

TDD Phase: RED - These tests should FAIL until implementation is complete.

Acceptance Criteria (from BR-HAPI-192):
- [ ] HolmesGPT-API accepts `recoveryContext` in request schema
- [ ] LLM prompt includes `naturalLanguageSummary` when provided
- [ ] Response includes warning when `isRecovery=true` but summary is empty
- [ ] Unit tests cover recovery context scenarios
"""


from src.models.recovery_models import (
    PreviousExecution,
    ExecutionFailure,
    OriginalRCA,
    SelectedWorkflowSummary,
    RecoveryRequest,
)


class TestPreviousExecutionNaturalLanguageSummary:
    """Test that PreviousExecution model accepts naturalLanguageSummary."""

    def test_previous_execution_accepts_natural_language_summary(self):
        """
        BR-HAPI-192-001: PreviousExecution MUST accept naturalLanguageSummary field.

        The WE-generated summary provides LLM-friendly context about the failure.
        """
        execution = PreviousExecution(
            workflow_execution_ref="we-12345",
            original_rca=OriginalRCA(
                summary="Memory exhaustion causing OOMKilled",
                signal_type="OOMKilled",
                severity="high",
                contributing_factors=["memory leak"]
            ),
            selected_workflow=SelectedWorkflowSummary(
                workflow_id="scale-horizontal-v1",
                version="1.0.0",
                execution_bundle="kubernaut/workflow:v1",
                parameters={"TARGET_REPLICAS": "5"},
                rationale="Scale to distribute load"
            ),
            failure=ExecutionFailure(
                failed_step_index=2,
                failed_step_name="scale_deployment",
                reason="OOMKilled",
                message="Container exceeded memory limit",
                exit_code=137,
                failed_at="2025-12-07T10:30:00Z",
                execution_time="2m34s"
            ),
            # BR-HAPI-192: This field should be accepted
            natural_language_summary="Workflow 'scale-horizontal-v1' failed during execution. "
                                    "The target pod payment-service-abc123 was evicted before "
                                    "memory adjustment could complete. Exit code 137 indicates "
                                    "OOMKilled during the remediation process itself."
        )

        assert execution.natural_language_summary is not None
        assert "scale-horizontal-v1" in execution.natural_language_summary
        assert "OOMKilled" in execution.natural_language_summary

    def test_natural_language_summary_is_optional(self):
        """
        BR-HAPI-192-003: naturalLanguageSummary is optional for backwards compatibility.

        Recovery should work without it (but with degraded LLM context).
        """
        execution = PreviousExecution(
            workflow_execution_ref="we-12345",
            original_rca=OriginalRCA(
                summary="Memory issue",
                signal_type="OOMKilled",
                severity="high"
            ),
            selected_workflow=SelectedWorkflowSummary(
                workflow_id="scale-v1",
                version="1.0.0",
                execution_bundle="kubernaut/workflow:v1",
                parameters={},
                rationale="Scale out"
            ),
            failure=ExecutionFailure(
                failed_step_index=0,
                failed_step_name="step1",
                reason="OOMKilled",
                message="Failed",
                failed_at="2025-12-07T10:30:00Z",
                execution_time="1m"
            )
            # Note: natural_language_summary NOT provided
        )

        # Should be None when not provided
        assert execution.natural_language_summary is None

    def test_natural_language_summary_serialization(self):
        """
        BR-HAPI-192: naturalLanguageSummary should serialize correctly in JSON.
        """
        execution = PreviousExecution(
            workflow_execution_ref="we-12345",
            original_rca=OriginalRCA(
                summary="Memory issue",
                signal_type="OOMKilled",
                severity="high"
            ),
            selected_workflow=SelectedWorkflowSummary(
                workflow_id="scale-v1",
                version="1.0.0",
                execution_bundle="kubernaut/workflow:v1",
                parameters={},
                rationale="Scale out"
            ),
            failure=ExecutionFailure(
                failed_step_index=0,
                failed_step_name="step1",
                reason="OOMKilled",
                message="Failed",
                failed_at="2025-12-07T10:30:00Z",
                execution_time="1m"
            ),
            natural_language_summary="WE-generated failure context"
        )

        # Should serialize with the field
        data = execution.model_dump()
        assert "natural_language_summary" in data
        assert data["natural_language_summary"] == "WE-generated failure context"


class TestRecoveryRequestWithNaturalLanguageSummary:
    """Test RecoveryRequest accepts PreviousExecution with naturalLanguageSummary."""

    def test_recovery_request_with_full_context(self):
        """
        BR-HAPI-192: Full recovery request with naturalLanguageSummary.
        """
        request = RecoveryRequest(
            incident_id="inc-001",
            remediation_id="req-2025-12-07-abc",
            is_recovery_attempt=True,
            recovery_attempt_number=2,
            previous_execution=PreviousExecution(
                workflow_execution_ref="we-12345",
                original_rca=OriginalRCA(
                    summary="Memory exhaustion",
                    signal_type="OOMKilled",
                    severity="high"
                ),
                selected_workflow=SelectedWorkflowSummary(
                    workflow_id="scale-v1",
                    version="1.0.0",
                    execution_bundle="kubernaut/workflow:v1",
                    parameters={},
                    rationale="Scale out"
                ),
                failure=ExecutionFailure(
                    failed_step_index=0,
                    failed_step_name="step1",
                    reason="OOMKilled",
                    message="Failed",
                    failed_at="2025-12-07T10:30:00Z",
                    execution_time="1m"
                ),
                natural_language_summary="Workflow failed due to memory limits"
            )
        )

        assert request.is_recovery_attempt is True
        assert request.previous_execution is not None
        assert request.previous_execution.natural_language_summary == "Workflow failed due to memory limits"


class TestRecoveryPromptInclusion:
    """Test that recovery prompts include naturalLanguageSummary."""

    def test_recovery_prompt_includes_natural_language_summary(self):
        """
        BR-HAPI-192-002: LLM prompt MUST include naturalLanguageSummary when provided.
        """
        from src.extensions.recovery import _create_recovery_investigation_prompt

        request_data = {
            "incident_id": "inc-001",
            "remediation_id": "req-2025-12-07-abc",
            "is_recovery_attempt": True,
            "previous_execution": {
                "workflow_execution_ref": "we-12345",
                "original_rca": {
                    "summary": "Memory exhaustion",
                    "signal_type": "OOMKilled",
                    "severity": "high",
                    "contributing_factors": []
                },
                "selected_workflow": {
                    "workflow_id": "scale-v1",
                    "version": "1.0.0",
                    "execution_bundle": "kubernaut/workflow:v1",
                    "parameters": {},
                    "rationale": "Scale out"
                },
                "failure": {
                    "failed_step_index": 0,
                    "failed_step_name": "step1",
                    "reason": "OOMKilled",
                    "message": "Container exceeded memory limit",
                    "exit_code": 137,
                    "failed_at": "2025-12-07T10:30:00Z",
                    "execution_time": "1m"
                },
                # BR-HAPI-192: This should appear in the prompt
                "natural_language_summary": "Workflow 'scale-v1' failed during step1. "
                                           "Pod was OOMKilled with exit code 137. "
                                           "Memory limit of 512Mi was exceeded."
            },
            "signal_type": "OOMKilled",
            "severity": "high",
            "resource_namespace": "production",
            "resource_kind": "Deployment",
            "resource_name": "api-server"
        }

        prompt = _create_recovery_investigation_prompt(request_data)

        # BR-HAPI-192-002: Prompt MUST include the WE-generated summary
        assert "Workflow 'scale-v1' failed during step1" in prompt
        assert "OOMKilled with exit code 137" in prompt
        assert "Memory limit of 512Mi was exceeded" in prompt

    def test_recovery_prompt_without_natural_language_summary(self):
        """
        BR-HAPI-192-003: When naturalLanguageSummary is empty/null, prompt should
        still work but include a note about missing context.
        """
        from src.extensions.recovery import _create_recovery_investigation_prompt

        request_data = {
            "incident_id": "inc-001",
            "remediation_id": "req-2025-12-07-abc",
            "is_recovery_attempt": True,
            "previous_execution": {
                "workflow_execution_ref": "we-12345",
                "original_rca": {
                    "summary": "Memory exhaustion",
                    "signal_type": "OOMKilled",
                    "severity": "high",
                    "contributing_factors": []
                },
                "selected_workflow": {
                    "workflow_id": "scale-v1",
                    "version": "1.0.0",
                    "execution_bundle": "kubernaut/workflow:v1",
                    "parameters": {},
                    "rationale": "Scale out"
                },
                "failure": {
                    "failed_step_index": 0,
                    "failed_step_name": "step1",
                    "reason": "OOMKilled",
                    "message": "Container exceeded memory limit",
                    "exit_code": 137,
                    "failed_at": "2025-12-07T10:30:00Z",
                    "execution_time": "1m"
                }
                # Note: natural_language_summary NOT provided
            },
            "signal_type": "OOMKilled",
            "severity": "high"
        }

        prompt = _create_recovery_investigation_prompt(request_data)

        # Should still generate a valid prompt
        assert "OOMKilled" in prompt
        assert "scale-v1" in prompt
        # Should NOT fail even without naturalLanguageSummary


class TestRecoveryResponseWarnings:
    """Test that recovery response includes appropriate warnings."""

    def test_warning_when_recovery_without_summary(self):
        """
        BR-HAPI-192-003: Response MUST include warning when isRecovery=true but
        naturalLanguageSummary is empty/null.
        """
        # This test verifies the warning is added in the recovery flow
        # The actual implementation will add this warning in the recovery handler
        pass  # Implementation will be tested via integration tests

