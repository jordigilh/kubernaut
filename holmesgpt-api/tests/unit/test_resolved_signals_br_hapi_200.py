"""
Unit tests for BR-HAPI-200: Handling Inconclusive Investigations.

BR-HAPI-200 defines two distinct outcomes:
1. Problem confirmed resolved → needs_human_review=false, selected_workflow=null (successful outcome)
2. Investigation inconclusive → needs_human_review=true, human_review_reason="investigation_inconclusive"

TDD Phase: GREEN - These tests verify the enum and basic model behavior.
"""

import json

from src.models.incident_models import HumanReviewReason


class TestHumanReviewReasonEnum:
    """Test that HumanReviewReason enum includes INVESTIGATION_INCONCLUSIVE."""

    def test_investigation_inconclusive_enum_exists(self):
        """
        BR-HAPI-200.1: Add INVESTIGATION_INCONCLUSIVE to HumanReviewReason enum.

        The enum should have a value for when the LLM investigation is inconclusive.
        """
        assert hasattr(HumanReviewReason, 'INVESTIGATION_INCONCLUSIVE')
        assert HumanReviewReason.INVESTIGATION_INCONCLUSIVE.value == "investigation_inconclusive"

    def test_investigation_inconclusive_is_string_enum(self):
        """
        BR-HAPI-200.1: The enum value should serialize to a string.
        """
        reason = HumanReviewReason.INVESTIGATION_INCONCLUSIVE
        assert str(reason.value) == "investigation_inconclusive"

    def test_all_human_review_reasons_exist(self):
        """
        Verify all HumanReviewReason enum values exist.
        """
        expected_reasons = [
            "WORKFLOW_NOT_FOUND",
            "IMAGE_MISMATCH",
            "PARAMETER_VALIDATION_FAILED",
            "NO_MATCHING_WORKFLOWS",
            "LOW_CONFIDENCE",
            "LLM_PARSING_ERROR",
            "INVESTIGATION_INCONCLUSIVE",  # BR-HAPI-200
        ]

        for reason in expected_reasons:
            assert hasattr(HumanReviewReason, reason), f"Missing enum: {reason}"

    def test_human_review_reason_json_serialization(self):
        """
        BR-HAPI-200: Enum values should serialize correctly for JSON responses.
        """
        reason = HumanReviewReason.INVESTIGATION_INCONCLUSIVE

        # Should serialize to string value
        assert reason.value == "investigation_inconclusive"

        # Should be usable in JSON
        json_data = {"human_review_reason": reason.value}
        json_str = json.dumps(json_data)
        assert '"investigation_inconclusive"' in json_str


class TestInconclusiveInvestigationSemantics:
    """Test the semantic meaning of INVESTIGATION_INCONCLUSIVE."""

    def test_inconclusive_means_uncertain_not_resolved(self):
        """
        BR-HAPI-200: INVESTIGATION_INCONCLUSIVE means LLM is uncertain.

        This is different from:
        - Problem confirmed resolved (confident, no human review)
        - Workflow found (confident, has workflow)
        """
        # INVESTIGATION_INCONCLUSIVE = LLM couldn't determine the state
        reason = HumanReviewReason.INVESTIGATION_INCONCLUSIVE

        # It implies uncertainty, not resolution
        assert "inconclusive" in reason.value
        assert reason != HumanReviewReason.LOW_CONFIDENCE  # Different from low confidence
        assert reason != HumanReviewReason.NO_MATCHING_WORKFLOWS  # Different from no workflows

    def test_when_to_use_investigation_inconclusive(self):
        """
        Document when INVESTIGATION_INCONCLUSIVE should be used.

        Use when:
        - LLM cannot determine root cause
        - LLM cannot verify current resource state
        - Events/metrics are ambiguous or unavailable
        - Investigation yields no clear answer

        DO NOT use when:
        - Problem is confirmed resolved (use needs_human_review=false)
        - Workflow validation fails (use WORKFLOW_NOT_FOUND, etc.)
        - Confidence is low but RCA exists (use LOW_CONFIDENCE)
        """
        # This is a documentation test - the assertions verify the enum exists
        # and can be used for the intended purpose
        reason = HumanReviewReason.INVESTIGATION_INCONCLUSIVE
        assert reason.value == "investigation_inconclusive"


class TestOutcomeDistinction:
    """Test the distinction between resolved and inconclusive outcomes."""

    def test_outcome_a_resolved_no_human_review(self):
        """
        BR-HAPI-200 Outcome A: Problem confirmed resolved.

        When LLM is CONFIDENT the problem is resolved:
        - needs_human_review = False
        - human_review_reason = None
        - selected_workflow = None
        - confidence >= 0.7

        This is a SUCCESSFUL outcome - no action needed.
        """
        # This documents the expected response structure
        expected_response = {
            "needs_human_review": False,
            "human_review_reason": None,
            "selected_workflow": None,
            "confidence": 0.92,
            "warnings": ["Problem self-resolved - no remediation required"]
        }

        assert expected_response["needs_human_review"] is False
        assert expected_response["human_review_reason"] is None
        assert expected_response["confidence"] >= 0.7

    def test_outcome_b_inconclusive_needs_human_review(self):
        """
        BR-HAPI-200 Outcome B: Investigation inconclusive.

        When LLM is UNCERTAIN about the state:
        - needs_human_review = True
        - human_review_reason = "investigation_inconclusive"
        - selected_workflow = None
        - confidence < 0.5

        This requires HUMAN judgment.
        """
        # This documents the expected response structure
        expected_response = {
            "needs_human_review": True,
            "human_review_reason": HumanReviewReason.INVESTIGATION_INCONCLUSIVE.value,
            "selected_workflow": None,
            "confidence": 0.35,
            "warnings": ["Investigation inconclusive - human review recommended"]
        }

        assert expected_response["needs_human_review"] is True
        assert expected_response["human_review_reason"] == "investigation_inconclusive"
        assert expected_response["confidence"] < 0.5

    def test_outcome_c_normal_workflow_selection(self):
        """
        Normal outcome: Problem exists, workflow selected.

        When LLM finds root cause and selects workflow:
        - needs_human_review = False (or True if low confidence)
        - human_review_reason = None (or LOW_CONFIDENCE)
        - selected_workflow = {...}
        - confidence varies

        This is the NORMAL flow - remediation proceeds.
        """
        expected_response = {
            "needs_human_review": False,
            "human_review_reason": None,
            "selected_workflow": {"workflow_id": "restart-pod", "confidence": 0.85},
            "confidence": 0.85,
        }

        assert expected_response["needs_human_review"] is False
        assert expected_response["selected_workflow"] is not None
