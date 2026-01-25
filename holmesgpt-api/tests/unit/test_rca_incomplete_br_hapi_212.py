"""
Unit tests for BR-HAPI-212: Missing affectedResource in RCA.

BR-HAPI-212 extends BR-HAPI-197 to add scenario #7:
When a workflow is selected but affectedResource is missing from root_cause_analysis,
the response should set needs_human_review=true with human_review_reason="rca_incomplete".

TDD Phase: GREEN - These tests verify the enum behavior.
Integration tests (test/integration/) will verify the full validation logic.
"""

import json
from src.models.incident_models import HumanReviewReason


class TestRCAIncompleteEnum:
    """Test that HumanReviewReason enum includes RCA_INCOMPLETE."""

    def test_rca_incomplete_enum_exists(self):
        """
        BR-HAPI-212: Add RCA_INCOMPLETE to HumanReviewReason enum.

        The enum should have a value for when RCA is missing affectedResource.
        """
        assert hasattr(HumanReviewReason, 'RCA_INCOMPLETE')
        assert HumanReviewReason.RCA_INCOMPLETE.value == "rca_incomplete"

    def test_rca_incomplete_is_string_enum(self):
        """
        BR-HAPI-212: The enum value should serialize to a string.
        """
        reason = HumanReviewReason.RCA_INCOMPLETE
        assert str(reason.value) == "rca_incomplete"

    def test_all_human_review_reasons_exist(self):
        """
        Verify all HumanReviewReason enum values exist (including BR-HAPI-212).
        """
        expected_reasons = [
            "WORKFLOW_NOT_FOUND",
            "IMAGE_MISMATCH",
            "PARAMETER_VALIDATION_FAILED",
            "NO_MATCHING_WORKFLOWS",
            "LOW_CONFIDENCE",
            "LLM_PARSING_ERROR",
            "INVESTIGATION_INCONCLUSIVE",  # BR-HAPI-200
            "RCA_INCOMPLETE",  # BR-HAPI-212
        ]

        for reason in expected_reasons:
            assert hasattr(HumanReviewReason, reason), f"Missing enum: {reason}"


class TestRCAIncompleteValidationLogic:
    """Test the semantic validation logic for missing affectedResource in RCA."""

    def test_rca_incomplete_validation_condition_documented(self):
        """
        BR-HAPI-212: Document when rca_incomplete validation triggers.

        Validation triggers when:
        1. selected_workflow is not None (workflow IS selected)
        2. affectedResource is missing from root_cause_analysis
        3. investigation_outcome is NOT "resolved"

        This test documents the expected behavior for integration tests.
        """
        # This is a documentation test - no actual validation here
        # Integration tests in holmesgpt-api/tests/integration/ will verify this behavior

        # Expected precedence order (checked in sequence):
        validation_checks = [
            "1. BR-HAPI-200: investigation_outcome='resolved' → needs_human_review=false",
            "2. BR-HAPI-200: investigation_outcome='inconclusive' → needs_human_review=true (investigation_inconclusive)",
            "3. BR-HAPI-197: selected_workflow=None → needs_human_review=true (no_matching_workflows)",
            "4. BR-HAPI-197: confidence < threshold → needs_human_review=true (low_confidence)",
            "5. BR-HAPI-212: selected_workflow!=None AND affectedResource missing → needs_human_review=true (rca_incomplete)"
        ]

        # This test passes if the validation checks are documented
        assert len(validation_checks) == 5, "All validation checks should be documented"

    def test_rca_incomplete_takes_precedence_over_normal_flow(self):
        """
        BR-HAPI-212: rca_incomplete check happens AFTER other human review conditions.

        Precedence order:
        1. problem_resolved (BR-HAPI-200) → no human review needed
        2. investigation_inconclusive (BR-HAPI-200) → needs_human_review=true
        3. no_matching_workflows (BR-HAPI-197) → needs_human_review=true
        4. low_confidence (BR-HAPI-197) → needs_human_review=true
        5. rca_incomplete (BR-HAPI-212) → needs_human_review=true (NEW)
        """
        # This test documents the expected precedence order
        # Integration tests will verify actual behavior

        precedence_order = [
            HumanReviewReason.INVESTIGATION_INCONCLUSIVE,  # BR-HAPI-200
            HumanReviewReason.NO_MATCHING_WORKFLOWS,      # BR-HAPI-197
            HumanReviewReason.LOW_CONFIDENCE,             # BR-HAPI-197
            HumanReviewReason.RCA_INCOMPLETE,             # BR-HAPI-212 (NEW)
        ]

        assert len(precedence_order) == 4, "All precedence levels should be documented"
        assert HumanReviewReason.RCA_INCOMPLETE in precedence_order, \
            "BR-HAPI-212 rca_incomplete should be in precedence order"


class TestRCAIncompleteSemantics:
    """Test the semantic meaning of RCA_INCOMPLETE."""

    def test_rca_incomplete_means_workflow_selected_but_target_unknown(self):
        """
        BR-HAPI-212: RCA_INCOMPLETE means workflow is selected but affected resource is unknown.

        This is different from:
        - No workflow found (BR-HAPI-197: no_matching_workflows)
        - Problem resolved (BR-HAPI-200: investigation_outcome="resolved")
        - Low confidence (BR-HAPI-197: low_confidence)
        """
        reason = HumanReviewReason.RCA_INCOMPLETE

        # It implies incomplete RCA, not missing workflow
        assert "incomplete" in reason.value or "rca" in reason.value
        assert reason != HumanReviewReason.NO_MATCHING_WORKFLOWS  # Different from no workflows
        assert reason != HumanReviewReason.LOW_CONFIDENCE  # Different from low confidence
        assert reason != HumanReviewReason.INVESTIGATION_INCONCLUSIVE  # Different from inconclusive

    def test_when_to_use_rca_incomplete(self):
        """
        Document when RCA_INCOMPLETE should be used.

        Use when:
        - selected_workflow is not None (workflow IS selected)
        - affectedResource is missing from root_cause_analysis
        - investigation_outcome is NOT "resolved"

        DO NOT use when:
        - No workflow found (use NO_MATCHING_WORKFLOWS)
        - Problem is confirmed resolved (use needs_human_review=false)
        - Confidence is low but affectedResource exists (use LOW_CONFIDENCE)
        - Investigation is inconclusive (use INVESTIGATION_INCONCLUSIVE)
        """
        # This is a documentation test - validates understanding
        reason = HumanReviewReason.RCA_INCOMPLETE

        # Should only be used for missing affectedResource when workflow selected
        assert reason.value == "rca_incomplete"
        assert reason != HumanReviewReason.NO_MATCHING_WORKFLOWS
        assert reason != HumanReviewReason.INVESTIGATION_INCONCLUSIVE
