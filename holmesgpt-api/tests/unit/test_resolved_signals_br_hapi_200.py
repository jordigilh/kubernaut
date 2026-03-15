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

    def test_outcome_d_not_actionable(self):
        """
        BR-HAPI-200 Outcome D: Alert not actionable.

        When LLM determines the alert is benign (no remediation warranted):
        - actionable = False
        - needs_human_review = False
        - is_actionable = False
        - human_review_reason = None
        - selected_workflow = None
        - confidence >= 0.7

        This is a SUCCESSFUL outcome - alert is benign.
        Distinct from Outcome A (problem resolved) — here the condition
        may still be present but is not harmful.
        """
        expected_response = {
            "actionable": False,
            "needs_human_review": False,
            "is_actionable": False,
            "human_review_reason": None,
            "selected_workflow": None,
            "confidence": 0.85,
            "warnings": ["Alert not actionable — no remediation warranted"]
        }

        assert expected_response["actionable"] is False
        assert expected_response["needs_human_review"] is False
        assert expected_response["is_actionable"] is False
        assert expected_response["human_review_reason"] is None
        assert expected_response["confidence"] >= 0.7


class TestActionableFieldPromptContent:
    """UT-HAPI-388-001: Prompt includes `actionable` field and Outcome D."""

    def test_prompt_contains_actionable_field_definition(self):
        """
        UT-HAPI-388-001: The prompt template must include `# actionable` field
        in the output format so the LLM knows to provide it.
        """
        from src.extensions.incident.prompt_builder import create_incident_investigation_prompt

        prompt = create_incident_investigation_prompt({
            "incident_id": "test-001",
            "signal_name": "KubePersistentVolumeClaimOrphaned",
            "severity": "low",
            "resource_namespace": "demo-orphaned-pvc",
            "resource_kind": "PersistentVolumeClaim",
            "resource_name": "orphaned-pvc-1",
            "environment": "production",
            "error_message": "PVC is not bound to any pod",
        })

        assert "# actionable" in prompt, \
            "Prompt must define '# actionable' as an output field for the LLM"

    def test_prompt_contains_outcome_d_section(self):
        """
        UT-HAPI-388-001: The prompt must include Outcome D section instructing
        the LLM to signal `actionable: false` for benign alerts.
        """
        from src.extensions.incident.prompt_builder import create_incident_investigation_prompt

        prompt = create_incident_investigation_prompt({
            "incident_id": "test-001",
            "signal_name": "KubePersistentVolumeClaimOrphaned",
            "severity": "low",
            "resource_namespace": "demo-orphaned-pvc",
            "resource_kind": "PersistentVolumeClaim",
            "resource_name": "orphaned-pvc-1",
            "environment": "production",
            "error_message": "PVC is not bound to any pod",
        })

        assert "Outcome D" in prompt, \
            "Prompt must include Outcome D heading for benign alerts"
        assert "actionable" in prompt.lower(), \
            "Outcome D must reference the 'actionable' field"
        assert "false" in prompt.lower(), \
            "Outcome D must instruct setting actionable to false"

    def test_prompt_distinguishes_not_actionable_from_resolved(self):
        """
        UT-HAPI-388-001: Prompt must clearly distinguish Outcome D (not actionable)
        from Outcome A (problem resolved).
        """
        from src.extensions.incident.prompt_builder import create_incident_investigation_prompt

        prompt = create_incident_investigation_prompt({
            "incident_id": "test-001",
            "signal_name": "KubePersistentVolumeClaimOrphaned",
            "severity": "low",
            "resource_namespace": "demo-orphaned-pvc",
            "resource_kind": "PersistentVolumeClaim",
            "resource_name": "orphaned-pvc-1",
            "environment": "production",
            "error_message": "PVC is not bound to any pod",
        })

        assert "Outcome A" in prompt, "Prompt must have Outcome A (resolved)"
        assert "Outcome D" in prompt, "Prompt must have Outcome D (not actionable)"


class TestActionableFieldParser:
    """UT-HAPI-388-002 to 005: Result parser handles `actionable` field."""

    def _build_analysis_text(self, actionable=None, needs_human_review=None,
                             human_review_reason=None, confidence=0.85):
        """Build a mock LLM analysis text with section headers."""
        lines = []
        lines.append("Investigation of orphaned PVC alert.")
        lines.append("")
        lines.append("```json")
        json_obj = {
            "root_cause_analysis": {
                "summary": "Orphaned PVCs from completed batch jobs, not impacting any workload",
                "severity": "low",
                "contributing_factors": ["Completed batch job artifacts"]
            },
            "selected_workflow": None,
            "confidence": confidence,
        }
        if actionable is not None:
            json_obj["actionable"] = actionable
        if needs_human_review is not None:
            json_obj["needs_human_review"] = needs_human_review
        if human_review_reason is not None:
            json_obj["human_review_reason"] = human_review_reason
        lines.append(json.dumps(json_obj, indent=2))
        lines.append("```")
        return "\n".join(lines)

    def _parse(self, analysis_text, incident_id="test-incident-001"):
        """Parse analysis text using the result parser."""
        from unittest.mock import MagicMock
        from src.extensions.incident.result_parser import parse_and_validate_investigation_result

        investigation = MagicMock()
        investigation.analysis = analysis_text

        request_data = {
            "incident_id": incident_id,
            "signal_name": "KubePersistentVolumeClaimOrphaned",
            "severity": "low",
        }

        result, _validation = parse_and_validate_investigation_result(
            investigation, request_data
        )
        return result

    def test_actionable_false_sets_no_human_review(self):
        """
        UT-HAPI-388-002: When LLM returns `actionable: false`, the parser must
        set `needs_human_review: False` and `is_actionable: False`.
        """
        analysis = self._build_analysis_text(actionable=False)
        result = self._parse(analysis)

        assert result["needs_human_review"] is False, \
            "actionable: false must set needs_human_review to False"
        assert result.get("is_actionable") is False, \
            "actionable: false must set is_actionable to False"
        assert result.get("human_review_reason") is None, \
            "actionable: false must not set a human_review_reason"

    def test_actionable_false_overrides_contradictory_human_review(self):
        """
        UT-HAPI-388-003: `actionable: false` overrides contradictory
        `needs_human_review: true` from the LLM.
        """
        analysis = self._build_analysis_text(
            actionable=False,
            needs_human_review=True,
        )
        result = self._parse(analysis)

        assert result["needs_human_review"] is False, \
            "actionable: false must override contradictory needs_human_review: true"
        assert result.get("is_actionable") is False, \
            "actionable: false must set is_actionable regardless of contradiction"

    def test_actionable_false_skips_no_matching_workflows_escalation(self):
        """
        UT-HAPI-388-004: `actionable: false` must skip the `no_matching_workflows`
        escalation that normally triggers when selected_workflow is None.
        """
        analysis = self._build_analysis_text(actionable=False)
        result = self._parse(analysis)

        assert result["needs_human_review"] is False, \
            "no_matching_workflows escalation must not override actionable: false"
        for warning in result.get("warnings", []):
            assert "no_matching_workflows" not in warning.lower(), \
                f"no_matching_workflows warning should not appear: {warning}"

    def test_actionable_false_emits_audit_warning(self):
        """
        UT-HAPI-388-005: `actionable: false` must emit a distinct warning
        for audit trail.
        """
        analysis = self._build_analysis_text(actionable=False)
        result = self._parse(analysis)

        warnings = result.get("warnings", [])
        has_not_actionable_warning = any(
            "not actionable" in w.lower() for w in warnings
        )
        assert has_not_actionable_warning, \
            f"Must emit 'not actionable' audit warning, got: {warnings}"


class TestActionableFieldParserPattern2B:
    """UT-HAPI-388-006: Result parser handles `# actionable` in section-header (Pattern 2B) format.

    The LLM prompt instructs section-header output. Pattern 2B in the active
    parser must extract `# actionable` — regression test for GAP 1 fix.
    """

    def _build_section_header_text(self, actionable=None, confidence=0.85):
        """Build LLM output in section-header format (Pattern 2B)."""
        lines = [
            "Investigation of orphaned PVC alert.",
            "",
            "# root_cause_analysis",
            '{"summary": "Orphaned PVCs from completed batch jobs", "severity": "low", "contributing_factors": ["Completed batch job artifacts"]}',
            "",
            "# confidence",
            str(confidence),
            "",
            "# selected_workflow",
            "None",
        ]
        if actionable is not None:
            lines.extend(["", "# actionable", str(actionable)])
        return "\n".join(lines)

    def _parse(self, analysis_text, incident_id="test-incident-2b"):
        from unittest.mock import MagicMock
        from src.extensions.incident.result_parser import parse_and_validate_investigation_result

        investigation = MagicMock()
        investigation.analysis = analysis_text
        request_data = {
            "incident_id": incident_id,
            "signal_name": "KubePersistentVolumeClaimOrphaned",
            "severity": "low",
        }
        result, _validation = parse_and_validate_investigation_result(
            investigation, request_data
        )
        return result

    def test_pattern_2b_actionable_false_detected(self):
        """
        UT-HAPI-388-006: `# actionable\\nfalse` in section-header format must
        be extracted and produce is_actionable=False, needs_human_review=False.
        """
        analysis = self._build_section_header_text(actionable=False)
        result = self._parse(analysis)

        assert result["needs_human_review"] is False, \
            "Pattern 2B: actionable=false must set needs_human_review to False"
        assert result.get("is_actionable") is False, \
            "Pattern 2B: actionable=false must set is_actionable to False"

    def test_pattern_2b_actionable_false_emits_warning(self):
        """
        UT-HAPI-388-006: Section-header `actionable=false` must also emit the
        audit trail warning.
        """
        analysis = self._build_section_header_text(actionable=False)
        result = self._parse(analysis)

        warnings = result.get("warnings", [])
        has_not_actionable_warning = any(
            "not actionable" in w.lower() for w in warnings
        )
        assert has_not_actionable_warning, \
            f"Pattern 2B: Must emit 'not actionable' warning, got: {warnings}"

    def test_pattern_2b_actionable_true_is_default(self):
        """
        UT-HAPI-388-006: When `# actionable` is absent from section-header
        output, the result must NOT contain is_actionable=False.
        """
        analysis = self._build_section_header_text(actionable=None)
        result = self._parse(analysis)

        assert result.get("is_actionable") is not False, \
            "Pattern 2B: Missing actionable field must not produce is_actionable=False"
