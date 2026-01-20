"""
Unit tests for Mock LLM problem_resolved scenario (BR-HAPI-200).

This test validates the new "problem_resolved" scenario added to support
integration testing of BR-HAPI-200 (Problem Self-Resolved path).
"""

import pytest
from src.server import MOCK_SCENARIOS


class TestProblemResolvedScenario:
    """Tests for the problem_resolved scenario."""

    def test_problem_resolved_scenario_exists(self):
        """Verify problem_resolved scenario is registered in MOCK_SCENARIOS."""
        assert "problem_resolved" in MOCK_SCENARIOS, "problem_resolved scenario should exist"

    def test_problem_resolved_scenario_properties(self):
        """Verify problem_resolved scenario has correct properties for BR-HAPI-200."""
        scenario = MOCK_SCENARIOS["problem_resolved"]

        # Verify scenario identity
        assert scenario.name == "problem_resolved"
        assert scenario.signal_type == "MOCK_PROBLEM_RESOLVED"

        # Verify no workflow (problem self-resolved)
        assert scenario.workflow_id == "", "problem_resolved should have empty workflow_id"
        assert scenario.workflow_name == "", "problem_resolved should have empty workflow_name"
        assert scenario.workflow_title == "", "problem_resolved should have empty workflow_title"

        # Verify high confidence (>= 0.7 threshold for BR-HAPI-200 Outcome A)
        assert scenario.confidence >= 0.7, "problem_resolved confidence should be >= 0.7 for BR-HAPI-200"
        assert scenario.confidence == 0.85, "problem_resolved confidence should be 0.85"

        # Verify root cause explains self-resolution
        assert "self-resolved" in scenario.root_cause.lower(), \
            "Root cause should mention self-resolution"

        # Verify severity is low (DD-SEVERITY-001 v1.1: problem no longer critical/high)
        assert scenario.severity == "low", "problem_resolved severity should be 'low'"

    def test_problem_resolved_vs_no_workflow_found_distinction(self):
        """Verify problem_resolved is distinct from no_workflow_found scenario."""
        problem_resolved = MOCK_SCENARIOS["problem_resolved"]
        no_workflow_found = MOCK_SCENARIOS["no_workflow_found"]

        # Both have empty workflow_id, but different meanings
        assert problem_resolved.workflow_id == ""
        assert no_workflow_found.workflow_id == ""

        # Different confidence levels
        assert problem_resolved.confidence >= 0.7, \
            "problem_resolved should have high confidence (problem truly resolved)"
        assert no_workflow_found.confidence < 0.7, \
            "no_workflow_found should have low confidence (needs human review)"

        # Different signal types
        assert problem_resolved.signal_type != no_workflow_found.signal_type, \
            "problem_resolved and no_workflow_found should have distinct signal types"

        # Different severities
        assert problem_resolved.severity == "low", \
            "problem_resolved should be 'low' (DD-SEVERITY-001 v1.1: problem no longer critical)"
        assert no_workflow_found.severity == "critical", \
            "no_workflow_found should be 'critical' (problem still exists, needs attention)"

    def test_problem_resolved_parameters(self):
        """Verify problem_resolved has no parameters (no workflow to parameterize)."""
        scenario = MOCK_SCENARIOS["problem_resolved"]
        assert scenario.parameters == {}, "problem_resolved should have empty parameters dict"

    def test_problem_resolved_rca_context(self):
        """Verify problem_resolved includes RCA context for audit trail."""
        scenario = MOCK_SCENARIOS["problem_resolved"]

        # RCA context should be present for audit trail completeness
        assert scenario.rca_resource_kind != "", "RCA resource kind should be set"
        assert scenario.rca_resource_namespace != "", "RCA resource namespace should be set"
        assert scenario.rca_resource_name != "", "RCA resource name should be set"

        # Verify reasonable values
        assert scenario.rca_resource_kind == "Pod"
        assert scenario.rca_resource_namespace == "production"
        assert scenario.rca_resource_name == "recovered-pod"


class TestProblemResolvedIntegrationPattern:
    """Tests for how problem_resolved scenario integrates with test suite."""

    def test_problem_resolved_scenario_count(self):
        """Verify we have the expected number of scenarios (9 total including problem_resolved and rca_incomplete)."""
        expected_scenarios = {
            "oomkilled",
            "crashloop",
            "node_not_ready",
            "recovery",
            "test_signal",
            "no_workflow_found",
            "low_confidence",
            "problem_resolved",  # BR-HAPI-200
            "rca_incomplete"  # BR-HAPI-212
        }

        actual_scenarios = set(MOCK_SCENARIOS.keys())
        assert actual_scenarios == expected_scenarios, \
            f"Expected {expected_scenarios}, got {actual_scenarios}"

    def test_problem_resolved_unique_signal_type(self):
        """Verify problem_resolved has a unique signal type (no collisions)."""
        signal_types = [scenario.signal_type for scenario in MOCK_SCENARIOS.values()]

        # Count occurrences of MOCK_PROBLEM_RESOLVED
        problem_resolved_count = signal_types.count("MOCK_PROBLEM_RESOLVED")
        assert problem_resolved_count == 1, \
            "MOCK_PROBLEM_RESOLVED should appear exactly once in scenarios"
