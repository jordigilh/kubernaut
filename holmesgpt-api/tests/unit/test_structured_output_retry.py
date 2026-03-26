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
Unit Tests for Structured Output Detection in HAPI Self-Correction (#372)

Business Requirement: BR-HAPI-002 (Incident Analysis)
Business Requirement: BR-HAPI-197 (needs_human_review field)
Design Decision: DD-HAPI-002 v1.2 (Workflow Response Validation with self-correction)
GitHub Issue: #372

When the LLM fails to produce structured JSON output, the parser must return
a failed ValidationResult so the retry loop can prompt the LLM to resubmit.
Previously, format failures returned validation_result=None, which the retry
loop treated as "valid" (same as a deliberate no-workflow decision).
"""

from unittest.mock import Mock
from tests.unit.conftest import CANONICAL_TARGET_PARAMS


class TestFormatFailureDetection:
    """Tests that LLM format failures are detected and signal retry."""

    def test_ut_hapi_372_001_plain_text_returns_failed_validation(self):
        """UT-HAPI-372-001: Plain-text narrative without JSON returns failed ValidationResult.

        BR-HAPI-002: The system must detect when the LLM fails to produce
        structured output and trigger retry via the self-correction loop.
        """
        from src.extensions.incident import _parse_and_validate_investigation_result

        investigation = Mock()
        investigation.analysis = (
            "The node stress-worker-3 is experiencing DiskPressure. "
            "The kubelet has started evicting pods due to nodefs threshold being exceeded. "
            "The primary contributor is the postgres-emptydir deployment which is writing "
            "large amounts of data to its emptyDir volume at /var/lib/postgresql/data. "
            "I recommend migrating the data to a PersistentVolumeClaim."
        )

        request_data = {"incident_id": "test-372-001"}

        result, validation_result = _parse_and_validate_investigation_result(
            investigation, request_data, data_storage_client=None
        )

        # BUG (pre-fix): validation_result is None, so retry loop treats it as valid
        # EXPECTED (post-fix): validation_result signals format failure
        assert validation_result is not None, (
            "Format failure must return a ValidationResult, not None"
        )
        assert validation_result.is_valid is False
        assert len(validation_result.errors) == 1
        assert "structured JSON output" in validation_result.errors[0]

        # Existing parser behavior for result dict should be preserved
        assert result["needs_human_review"] is True
        assert any("No workflows matched" in w for w in result["warnings"])

    def test_ut_hapi_372_002_rich_markdown_returns_failed_validation(self):
        """UT-HAPI-372-002: Rich markdown analysis without structured JSON returns failed ValidationResult.

        BR-HAPI-002: Even complex markdown with headers, bullets, and code
        blocks (but no JSON selection block) must be detected as format failure.
        """
        from src.extensions.incident import _parse_and_validate_investigation_result

        investigation = Mock()
        investigation.analysis = """## Root Cause Analysis

### Summary
The node `stress-worker-3` is under **DiskPressure** due to excessive emptyDir usage.

### Contributing Factors
- The `postgres-emptydir` deployment writes ~500MB every 30 seconds
- Several noise pods (`log-collector`, `cache-warmer`) also consume emptyDir
- The node only has 15GB disk, with kubelet threshold at 15%

### Recommended Action
Migrate the `postgres-emptydir` volume from `emptyDir` to a `PersistentVolumeClaim`:

```yaml
volumes:
  - name: data
    persistentVolumeClaim:
      claimName: postgres-data
```

### Severity
Critical — pod evictions will continue until disk pressure is relieved.
"""

        request_data = {"incident_id": "test-372-002"}

        result, validation_result = _parse_and_validate_investigation_result(
            investigation, request_data, data_storage_client=None
        )

        assert validation_result is not None, (
            "Rich markdown without structured JSON must return a ValidationResult"
        )
        assert validation_result.is_valid is False
        assert len(validation_result.errors) >= 1
        assert "structured JSON output" in validation_result.errors[0]


class TestLegitimateNoWorkflowOutcomes:
    """Regression guards: legitimate no-workflow decisions must NOT trigger retry."""

    def test_ut_hapi_372_003_outcome_a_resolved_returns_none(self):
        """UT-HAPI-372-003: Outcome A (problem self-resolved) returns validation_result=None.

        BR-HAPI-002: When the LLM deliberately reports the problem resolved via
        structured output, the retry loop must NOT retry.
        """
        from src.extensions.incident import _parse_and_validate_investigation_result

        investigation = Mock()
        investigation.analysis = '''
```json
{
  "root_cause_analysis": {
    "summary": "Problem self-resolved. The DiskPressure condition cleared after pod eviction freed space.",
    "severity": "low",
    "contributing_factors": ["Transient condition", "Auto-recovery via pod eviction"]
  },
  "confidence": 0.85,
  "selected_workflow": null,
  "investigation_outcome": "resolved"
}
```
'''
        request_data = {"incident_id": "test-372-003"}

        result, validation_result = _parse_and_validate_investigation_result(
            investigation, request_data, data_storage_client=None
        )

        assert validation_result is None, (
            "Outcome A (resolved) must return validation_result=None — no retry"
        )
        assert any("Problem self-resolved" in w for w in result["warnings"])
        assert result["needs_human_review"] is False

    def test_ut_hapi_372_004_outcome_b_inconclusive_returns_none(self):
        """UT-HAPI-372-004: Outcome B (investigation inconclusive) returns validation_result=None.

        BR-HAPI-002: When the LLM reports inconclusive findings via structured
        output, the retry loop must NOT retry (human review is already flagged).
        """
        from src.extensions.incident import _parse_and_validate_investigation_result

        investigation = Mock()
        investigation.analysis = '''
```json
{
  "root_cause_analysis": {
    "summary": "Unable to determine root cause due to insufficient metrics data.",
    "severity": "unknown",
    "contributing_factors": ["Metrics unavailable", "Stale event data"]
  },
  "confidence": 0.3,
  "selected_workflow": null,
  "investigation_outcome": "inconclusive"
}
```
'''
        request_data = {"incident_id": "test-372-004"}

        result, validation_result = _parse_and_validate_investigation_result(
            investigation, request_data, data_storage_client=None
        )

        assert validation_result is None, (
            "Outcome B (inconclusive) must return validation_result=None — no retry"
        )
        assert any("Investigation inconclusive" in w for w in result["warnings"])
        assert result["needs_human_review"] is True
        assert result.get("human_review_reason") == "investigation_inconclusive"

    def test_ut_hapi_372_005_outcome_c_no_automated_fix_returns_none(self):
        """UT-HAPI-372-005: Outcome C (structured JSON, no workflow, no investigation_outcome).

        BR-HAPI-002: When the LLM identifies the problem but finds no matching
        workflow via structured output, the retry loop must NOT retry.
        This is the closest case to the bug — ensures the fix is precise.
        """
        from src.extensions.incident import _parse_and_validate_investigation_result

        investigation = Mock()
        investigation.analysis = '''
```json
{
  "root_cause_analysis": {
    "summary": "DiskPressure caused by excessive emptyDir usage on the node.",
    "severity": "critical",
    "contributing_factors": ["emptyDir volume growth", "No PVC migration available"]
  },
  "confidence": 0.75,
  "selected_workflow": null
}
```
'''
        request_data = {"incident_id": "test-372-005"}

        result, validation_result = _parse_and_validate_investigation_result(
            investigation, request_data, data_storage_client=None
        )

        assert validation_result is None, (
            "Outcome C (no automated fix, structured output) must return None — no retry"
        )
        assert any("No workflows matched" in w for w in result["warnings"])
        assert result["needs_human_review"] is True
        assert result.get("human_review_reason") == "no_matching_workflows"

    def test_ut_hapi_372_006_valid_workflow_passes_unchanged(self):
        """UT-HAPI-372-006: Valid workflow passes validation, returns is_valid=True.

        BR-HAPI-197: When the LLM selects a valid workflow that passes catalog
        validation, the ValidationResult is returned with is_valid=True (not
        cleared to None) so callers can access parameter_schema for #524
        conditional injection.
        """
        from src.extensions.incident import _parse_and_validate_investigation_result
        from src.validation.workflow_response_validator import ValidationResult

        mock_ds = Mock()
        mock_workflow = Mock()
        mock_workflow.workflow_id = "emptydir-to-pvc-migration"
        mock_workflow.execution_bundle = "quay.io/kubernaut-ai/emptydir-migration:v1.0"
        mock_workflow.parameters = {"schema": {"parameters": list(CANONICAL_TARGET_PARAMS)}}
        mock_ds.get_workflow_by_id.return_value = mock_workflow

        investigation = Mock()
        investigation.analysis = '''
```json
{
  "root_cause_analysis": {
    "summary": "DiskPressure caused by emptyDir growth on postgres-emptydir.",
    "severity": "critical",
    "contributing_factors": ["emptyDir volume growth"],
    "remediationTarget": {"kind": "Deployment", "name": "postgres-emptydir", "namespace": "disk-pressure-demo"}
  },
  "confidence": 0.9,
  "selected_workflow": {
    "workflow_id": "emptydir-to-pvc-migration",
    "execution_bundle": "quay.io/kubernaut-ai/emptydir-migration:v1.0",
    "parameters": {}
  }
}
```
'''
        request_data = {"incident_id": "test-372-006"}

        result, validation_result = _parse_and_validate_investigation_result(
            investigation, request_data, data_storage_client=mock_ds
        )

        assert validation_result is not None, (
            "Valid workflow must return ValidationResult (not None) for #524 schema propagation"
        )
        assert validation_result.is_valid is True
        assert result.get("selected_workflow") is not None
        assert result["needs_human_review"] is False

    def test_ut_hapi_372_007_format_failure_error_is_actionable(self):
        """UT-HAPI-372-007: Format failure ValidationResult contains actionable error.

        DD-HAPI-002: The error message must be suitable for inclusion in the
        retry feedback prompt via build_validation_error_feedback().
        """
        from src.extensions.incident import _parse_and_validate_investigation_result

        investigation = Mock()
        investigation.analysis = "This is plain text with no structured output at all."

        request_data = {"incident_id": "test-372-007"}

        _, validation_result = _parse_and_validate_investigation_result(
            investigation, request_data, data_storage_client=None
        )

        assert validation_result is not None
        assert validation_result.is_valid is False
        error_msg = validation_result.errors[0]
        assert "structured JSON output" in error_msg
        assert "```json```" in error_msg or "section_header" in error_msg
        assert len(error_msg) > 20, "Error must be descriptive enough for LLM feedback"


class TestFormatSpecificFeedback:
    """REFACTOR phase: format-specific retry feedback prompt."""

    def test_ut_hapi_372_008_format_specific_feedback_prompt(self):
        """UT-HAPI-372-008: build_validation_error_feedback produces format-specific prompt.

        DD-HAPI-002: When the error is a structured output format failure,
        the feedback must instruct the LLM to use the correct JSON format,
        not give generic "check workflow ID" instructions.
        """
        from src.extensions.incident import _build_validation_error_feedback

        format_error = [
            "LLM did not produce structured JSON output. "
            "Expected ```json``` code block or # section_header format."
        ]

        feedback = _build_validation_error_feedback(format_error, attempt=0)

        # Must contain format-specific instructions
        assert "OUTPUT FORMAT ERROR" in feedback, (
            "Format failure must use format-specific heading, not generic 'VALIDATION ERROR'"
        )
        assert "```json```" in feedback
        assert "root_cause_analysis" in feedback
        assert "selected_workflow" in feedback

        # Must NOT contain generic workflow validation instructions
        assert "Re-check the workflow ID exists" not in feedback, (
            "Format failure feedback must not include workflow-specific instructions"
        )

        # Must include attempt counter
        assert "Attempt 1/" in feedback

    def test_ut_hapi_372_008b_workflow_errors_use_generic_feedback(self):
        """UT-HAPI-372-008b: Non-format errors still use the generic workflow feedback.

        DD-HAPI-002: Ensures the format-specific branch does not affect
        existing workflow validation error feedback.
        """
        from src.extensions.incident import _build_validation_error_feedback

        workflow_error = ["Workflow 'bad-id' not found in catalog"]

        feedback = _build_validation_error_feedback(workflow_error, attempt=1)

        assert "VALIDATION ERROR" in feedback
        assert "Re-check the workflow ID exists" in feedback
        assert "bad-id" in feedback
        assert "OUTPUT FORMAT ERROR" not in feedback
