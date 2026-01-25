"""
Unit tests for Mock LLM rca_incomplete scenario (BR-HAPI-212).

This test validates the new "rca_incomplete" scenario added to support
integration testing of BR-HAPI-212 (Missing affectedResource in RCA path).
"""

import pytest
from src.server import MOCK_SCENARIOS


class TestRCAIncompleteScenario:
    """Tests for the rca_incomplete scenario."""

    def test_rca_incomplete_scenario_exists(self):
        """Verify rca_incomplete scenario is registered in MOCK_SCENARIOS."""
        assert "rca_incomplete" in MOCK_SCENARIOS, "rca_incomplete scenario should exist"

    def test_rca_incomplete_scenario_properties(self):
        """Verify rca_incomplete scenario has correct properties for BR-HAPI-212."""
        scenario = MOCK_SCENARIOS["rca_incomplete"]

        # Verify scenario identity
        assert scenario.name == "rca_incomplete"
        assert scenario.signal_type == "MOCK_RCA_INCOMPLETE"

        # Verify workflow IS selected (BR-HAPI-212: Workflow selected but affectedResource missing)
        assert scenario.workflow_id != "", "rca_incomplete should have a workflow_id (not empty)"
        assert scenario.workflow_name == "generic-restart-v1", "rca_incomplete should use generic-restart-v1"
        assert scenario.workflow_title != "", "rca_incomplete should have a workflow_title"

        # Verify high confidence (>= 0.7 threshold, but incomplete RCA)
        assert scenario.confidence >= 0.7, "rca_incomplete confidence should be >= 0.7 for BR-HAPI-212"
        assert scenario.confidence == 0.88, "rca_incomplete confidence should be 0.88"

        # Verify root cause explains incomplete RCA
        assert "could not be determined" in scenario.root_cause.lower() or \
               "incomplete" in scenario.root_cause.lower(), \
            "Root cause should mention incomplete RCA or inability to determine affected resource"

        # Verify severity is critical (problem exists but affected resource unknown)
        assert scenario.severity == "critical", "rca_incomplete severity should be 'critical'"

        # BR-HAPI-212: Verify include_affected_resource flag is False
        assert scenario.include_affected_resource == False, \
            "rca_incomplete should have include_affected_resource=False to trigger BR-HAPI-212"

    def test_rca_incomplete_vs_no_workflow_found_distinction(self):
        """Verify rca_incomplete is distinct from no_workflow_found scenario."""
        rca_incomplete = MOCK_SCENARIOS["rca_incomplete"]
        no_workflow_found = MOCK_SCENARIOS["no_workflow_found"]

        # rca_incomplete HAS workflow, no_workflow_found does NOT
        assert rca_incomplete.workflow_id != "", \
            "rca_incomplete should have a workflow_id (workflow IS selected)"
        assert no_workflow_found.workflow_id == "", \
            "no_workflow_found should have empty workflow_id (workflow NOT selected)"

        # Both should trigger human review, but for different reasons
        # rca_incomplete: High confidence in workflow, but can't determine affected resource
        # no_workflow_found: No workflow matches the signal
        assert rca_incomplete.confidence >= 0.7, \
            "rca_incomplete should have high confidence (workflow is good)"
        assert no_workflow_found.confidence == 0.0, \
            "no_workflow_found should have zero confidence (no workflow match)"

        # Different signal types
        assert rca_incomplete.signal_type != no_workflow_found.signal_type, \
            "rca_incomplete and no_workflow_found should have distinct signal types"

    def test_rca_incomplete_parameters(self):
        """Verify rca_incomplete has parameters (workflow is selected, needs parameters)."""
        scenario = MOCK_SCENARIOS["rca_incomplete"]
        assert scenario.parameters != {}, "rca_incomplete should have parameters (workflow selected)"
        assert "ACTION" in scenario.parameters, "rca_incomplete should have ACTION parameter"

    def test_rca_incomplete_rca_context(self):
        """Verify rca_incomplete includes RCA context (even though affectedResource won't be in response)."""
        scenario = MOCK_SCENARIOS["rca_incomplete"]

        # RCA context should be present in scenario definition
        # (used for internal test setup, but NOT included in Mock LLM response due to include_affected_resource=False)
        assert scenario.rca_resource_kind != "", "RCA resource kind should be set in scenario"
        assert scenario.rca_resource_name != "", "RCA resource name should be set in scenario"
        assert scenario.rca_resource_api_version != "", "RCA resource API version should be set for BR-HAPI-212"

        # Verify reasonable values
        assert scenario.rca_resource_kind == "Pod"
        assert scenario.rca_resource_namespace == "production"
        assert scenario.rca_resource_name == "ambiguous-pod"
        assert scenario.rca_resource_api_version == "v1"


class TestRCAIncompleteIntegrationPattern:
    """Tests for how rca_incomplete scenario integrates with test suite."""

    def test_rca_incomplete_scenario_count(self):
        """Verify we have the expected number of scenarios (9 total including rca_incomplete)."""
        expected_scenarios = {
            "oomkilled",
            "crashloop",
            "node_not_ready",
            "recovery",
            "test_signal",
            "no_workflow_found",
            "low_confidence",
            "problem_resolved",
            "rca_incomplete"  # NEW: BR-HAPI-212
        }

        actual_scenarios = set(MOCK_SCENARIOS.keys())
        assert actual_scenarios == expected_scenarios, \
            f"Expected {expected_scenarios}, got {actual_scenarios}"

    def test_rca_incomplete_unique_signal_type(self):
        """Verify rca_incomplete has a unique signal type (no collisions)."""
        signal_types = [scenario.signal_type for scenario in MOCK_SCENARIOS.values()]

        # Count occurrences of MOCK_RCA_INCOMPLETE
        rca_incomplete_count = signal_types.count("MOCK_RCA_INCOMPLETE")
        assert rca_incomplete_count == 1, \
            "MOCK_RCA_INCOMPLETE should appear exactly once in scenarios"

    def test_include_affected_resource_flag_usage(self):
        """Verify include_affected_resource flag is used correctly across scenarios."""
        # Most scenarios should include affectedResource (default behavior)
        scenarios_with_affected_resource = [
            scenario for scenario in MOCK_SCENARIOS.values()
            if scenario.include_affected_resource
        ]

        # Only rca_incomplete should have include_affected_resource=False
        scenarios_without_affected_resource = [
            scenario for scenario in MOCK_SCENARIOS.values()
            if not scenario.include_affected_resource
        ]

        assert len(scenarios_without_affected_resource) == 1, \
            "Only one scenario (rca_incomplete) should have include_affected_resource=False"
        assert scenarios_without_affected_resource[0].name == "rca_incomplete", \
            "rca_incomplete should be the only scenario without affectedResource"

        # Verify most scenarios do include affectedResource
        assert len(scenarios_with_affected_resource) >= 7, \
            "Most scenarios should include affectedResource by default"

    def test_rca_incomplete_workflow_validation_trigger(self):
        """Verify rca_incomplete scenario will trigger HAPI validation (BR-HAPI-212)."""
        scenario = MOCK_SCENARIOS["rca_incomplete"]

        # BR-HAPI-212 validation should trigger when:
        # 1. selected_workflow is not None (workflow IS selected)
        # 2. affectedResource is missing from root_cause_analysis
        # 3. investigation_outcome is not "resolved"

        # Condition 1: Workflow selected
        assert scenario.workflow_id != "", \
            "Workflow must be selected for BR-HAPI-212 validation to trigger"

        # Condition 2: affectedResource will be missing in response
        assert scenario.include_affected_resource == False, \
            "affectedResource must be missing for BR-HAPI-212 validation to trigger"

        # Condition 3: investigation_outcome is not "resolved" (implied, no investigation_outcome set)
        # This is implicit - rca_incomplete scenario doesn't set investigation_outcome="resolved"
        # HAPI validation will see: workflow selected + missing affectedResource → needs_human_review=True
