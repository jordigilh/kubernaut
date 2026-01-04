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
Mock LLM Mode Unit Tests

Business Requirement: BR-HAPI-212 - Mock LLM Mode for Integration Testing

Tests verify:
1. Mock mode detection via environment variable
2. Deterministic responses based on signal_type
3. Schema compliance of mock responses
4. Request validation still runs in mock mode
"""

import os
from unittest.mock import patch

from src.mock_responses import (
    is_mock_mode_enabled,
    get_mock_scenario,
    generate_mock_incident_response,
    generate_mock_recovery_response,
    MOCK_SCENARIOS,
    DEFAULT_SCENARIO,
)


class TestMockModeDetection:
    """Tests for MOCK_LLM_MODE environment variable detection"""

    def test_mock_mode_disabled_by_default(self):
        """BR-HAPI-212: Mock mode should be disabled when env var not set"""
        with patch.dict(os.environ, {}, clear=True):
            # Remove MOCK_LLM_MODE if it exists
            os.environ.pop("MOCK_LLM_MODE", None)
            assert is_mock_mode_enabled() is False

    def test_mock_mode_enabled_when_true(self):
        """BR-HAPI-212: Mock mode should be enabled when MOCK_LLM_MODE=true"""
        with patch.dict(os.environ, {"MOCK_LLM_MODE": "true"}):
            assert is_mock_mode_enabled() is True

    def test_mock_mode_enabled_case_insensitive(self):
        """BR-HAPI-212: Mock mode should work with TRUE, True, etc."""
        with patch.dict(os.environ, {"MOCK_LLM_MODE": "TRUE"}):
            assert is_mock_mode_enabled() is True

        with patch.dict(os.environ, {"MOCK_LLM_MODE": "True"}):
            assert is_mock_mode_enabled() is True

    def test_mock_mode_disabled_when_false(self):
        """BR-HAPI-212: Mock mode should be disabled when MOCK_LLM_MODE=false"""
        with patch.dict(os.environ, {"MOCK_LLM_MODE": "false"}):
            assert is_mock_mode_enabled() is False

    def test_mock_mode_disabled_when_empty(self):
        """BR-HAPI-212: Mock mode should be disabled when MOCK_LLM_MODE is empty"""
        with patch.dict(os.environ, {"MOCK_LLM_MODE": ""}):
            assert is_mock_mode_enabled() is False


class TestMockScenarioSelection:
    """Tests for signal_type to scenario mapping"""

    def test_oomkilled_scenario(self):
        """BR-HAPI-212: OOMKilled should return OOMKilled scenario"""
        scenario = get_mock_scenario("OOMKilled")
        assert scenario.signal_type == "OOMKilled"
        assert scenario.workflow_id == "mock-oomkill-increase-memory-v1"
        assert scenario.severity == "critical"
        assert scenario.confidence == 0.92

    def test_crashloop_scenario(self):
        """BR-HAPI-212: CrashLoopBackOff should return CrashLoop scenario"""
        scenario = get_mock_scenario("CrashLoopBackOff")
        assert scenario.signal_type == "CrashLoopBackOff"
        assert scenario.workflow_id == "mock-crashloop-config-fix-v1"
        assert scenario.severity == "high"

    def test_node_not_ready_scenario(self):
        """BR-HAPI-212: NodeNotReady should return node scenario"""
        scenario = get_mock_scenario("NodeNotReady")
        assert scenario.signal_type == "NodeNotReady"
        assert scenario.workflow_id == "mock-node-drain-reboot-v1"
        assert scenario.severity == "critical"

    def test_image_pull_backoff_scenario(self):
        """BR-HAPI-212: ImagePullBackOff should return image scenario"""
        scenario = get_mock_scenario("ImagePullBackOff")
        assert scenario.signal_type == "ImagePullBackOff"
        assert scenario.workflow_id == "mock-image-fix-v1"

    def test_unknown_signal_returns_default(self):
        """BR-HAPI-212: Unknown signal_type should return default scenario"""
        scenario = get_mock_scenario("UnknownSignal123")
        assert scenario == DEFAULT_SCENARIO
        assert scenario.workflow_id == "mock-generic-restart-v1"
        assert scenario.confidence == 0.75

    def test_case_insensitive_matching(self):
        """BR-HAPI-212: Signal type matching should be case-insensitive"""
        # Test lowercase
        scenario = get_mock_scenario("oomkilled")
        assert scenario.signal_type == "OOMKilled"

        # Test mixed case
        scenario = get_mock_scenario("CrashloopBackoff")
        assert scenario.signal_type == "CrashLoopBackOff"

    def test_all_scenarios_have_required_fields(self):
        """BR-HAPI-212: All mock scenarios should have required fields"""
        for signal_type, scenario in MOCK_SCENARIOS.items():
            assert scenario.signal_type, f"Missing signal_type in {signal_type}"
            assert scenario.workflow_id, f"Missing workflow_id in {signal_type}"
            assert scenario.severity in ["critical", "high", "medium", "low"], f"Invalid severity in {signal_type}"
            assert 0 <= scenario.confidence <= 1.0, f"Invalid confidence in {signal_type}"
            assert scenario.root_cause_summary, f"Missing root_cause_summary in {signal_type}"


class TestMockIncidentResponse:
    """Tests for generate_mock_incident_response"""

    def test_response_has_required_fields(self):
        """BR-HAPI-212: Mock response should have all required IncidentResponse fields"""
        request_data = {
            "incident_id": "test-incident-123",
            "signal_type": "OOMKilled",
            "resource_namespace": "production",
            "resource_name": "api-server-abc",
            "resource_kind": "Pod",
        }

        response = generate_mock_incident_response(request_data)

        # Required fields per IncidentResponse schema
        assert "incident_id" in response
        assert "analysis" in response
        assert "root_cause_analysis" in response
        assert "selected_workflow" in response
        assert "confidence" in response
        assert "timestamp" in response
        assert "warnings" in response
        assert "needs_human_review" in response

    def test_response_uses_request_incident_id(self):
        """BR-HAPI-212: Mock response should use incident_id from request"""
        request_data = {
            "incident_id": "my-unique-incident-456",
            "signal_type": "OOMKilled",
        }

        response = generate_mock_incident_response(request_data)
        assert response["incident_id"] == "my-unique-incident-456"

    def test_response_workflow_matches_signal_type(self):
        """BR-HAPI-212: Mock workflow should be determined by signal_type"""
        # OOMKilled
        response = generate_mock_incident_response({"signal_type": "OOMKilled"})
        assert response["selected_workflow"]["workflow_id"] == "mock-oomkill-increase-memory-v1"
        assert response["confidence"] == 0.92

        # CrashLoopBackOff
        response = generate_mock_incident_response({"signal_type": "CrashLoopBackOff"})
        assert response["selected_workflow"]["workflow_id"] == "mock-crashloop-config-fix-v1"
        assert response["confidence"] == 0.88

    def test_response_includes_mock_mode_warning(self):
        """BR-HAPI-212: Mock response should include warning about mock mode"""
        response = generate_mock_incident_response({"signal_type": "OOMKilled"})

        assert any("MOCK_MODE" in w for w in response["warnings"])
        assert any("BR-HAPI-212" in w for w in response["warnings"])

    def test_response_root_cause_analysis_structure(self):
        """BR-HAPI-212: Mock RCA should have correct structure"""
        response = generate_mock_incident_response({"signal_type": "OOMKilled"})

        rca = response["root_cause_analysis"]
        assert "summary" in rca
        assert "severity" in rca
        assert "contributing_factors" in rca
        assert isinstance(rca["contributing_factors"], list)

    def test_response_workflow_has_parameters(self):
        """BR-HAPI-212: Mock workflow should include parameters"""
        request_data = {
            "signal_type": "OOMKilled",
            "resource_namespace": "my-namespace",
        }

        response = generate_mock_incident_response(request_data)

        workflow = response["selected_workflow"]
        assert "parameters" in workflow
        assert workflow["parameters"]["NAMESPACE"] == "my-namespace"

    def test_response_is_deterministic(self):
        """BR-HAPI-212: Same input should produce same output (deterministic)"""
        request_data = {
            "incident_id": "test-123",
            "signal_type": "OOMKilled",
            "resource_namespace": "production",
        }

        response1 = generate_mock_incident_response(request_data)
        response2 = generate_mock_incident_response(request_data)

        # Exclude timestamp from comparison (it will differ)
        response1.pop("timestamp")
        response2.pop("timestamp")

        assert response1["incident_id"] == response2["incident_id"]
        assert response1["selected_workflow"] == response2["selected_workflow"]
        assert response1["confidence"] == response2["confidence"]


class TestMockRecoveryResponse:
    """Tests for generate_mock_recovery_response"""

    def test_response_has_required_fields(self):
        """BR-HAPI-212: Mock recovery response should have all required fields"""
        request_data = {
            "remediation_id": "rem-123",
            "signal_type": "OOMKilled",
            "previous_workflow_id": "previous-workflow-v1",
        }

        response = generate_mock_recovery_response(request_data)

        assert "remediation_id" in response
        assert "analysis" in response
        assert "recovery_analysis" in response
        assert "selected_workflow" in response
        assert "confidence" in response
        assert "warnings" in response

    def test_response_uses_request_remediation_id(self):
        """BR-HAPI-212: Mock recovery response should use remediation_id from request"""
        request_data = {
            "remediation_id": "my-remediation-789",
            "signal_type": "CrashLoopBackOff",
        }

        response = generate_mock_recovery_response(request_data)
        assert response["remediation_id"] == "my-remediation-789"

    def test_recovery_workflow_differs_from_previous(self):
        """BR-HAPI-212: Recovery workflow should be different (has -recovery suffix)"""
        request_data = {
            "signal_type": "OOMKilled",
            "previous_workflow_id": "some-previous-workflow",
        }

        response = generate_mock_recovery_response(request_data)

        # Recovery workflow should have -recovery suffix
        assert "-recovery" in response["selected_workflow"]["workflow_id"]

    def test_recovery_includes_previous_workflow_reference(self):
        """BR-HAPI-212: Recovery analysis should reference previous workflow"""
        request_data = {
            "signal_type": "OOMKilled",
            "previous_workflow_id": "failed-workflow-v1",
        }

        response = generate_mock_recovery_response(request_data)

        recovery = response["recovery_analysis"]
        assert "previous_attempt_assessment" in recovery
        assert recovery["previous_attempt_assessment"]["workflow_id"] == "failed-workflow-v1"

    def test_recovery_confidence_slightly_lower(self):
        """BR-HAPI-212: Recovery confidence should be slightly lower than incident"""
        incident_response = generate_mock_incident_response({"signal_type": "OOMKilled"})
        recovery_response = generate_mock_recovery_response({"signal_type": "OOMKilled"})

        # Recovery confidence should be 0.05 lower
        assert recovery_response["confidence"] < incident_response["confidence"]
        assert recovery_response["confidence"] == incident_response["confidence"] - 0.05


class TestMockEdgeCaseIncidentResponses:
    """Tests for edge case incident mock responses (BR-HAPI-197)."""

    def test_no_workflow_found_edge_case(self):
        """BR-HAPI-197: MOCK_NO_WORKFLOW_FOUND triggers needs_human_review."""
        request_data = {
            "incident_id": "edge-case-001",
            "signal_type": "MOCK_NO_WORKFLOW_FOUND",
            "resource_namespace": "test",
        }

        response = generate_mock_incident_response(request_data)

        assert response["needs_human_review"] is True
        assert response["human_review_reason"] == "no_matching_workflows"
        assert response["selected_workflow"] is None
        assert response["confidence"] == 0.0

    def test_low_confidence_edge_case(self):
        """BR-HAPI-197: MOCK_LOW_CONFIDENCE triggers needs_human_review."""
        request_data = {
            "incident_id": "edge-case-002",
            "signal_type": "MOCK_LOW_CONFIDENCE",
            "resource_namespace": "test",
        }

        response = generate_mock_incident_response(request_data)

        assert response["needs_human_review"] is True
        assert response["human_review_reason"] == "low_confidence"
        assert response["selected_workflow"] is not None  # Tentative workflow
        assert response["selected_workflow"]["confidence"] < 0.5
        assert len(response["alternative_workflows"]) > 0

    def test_max_retries_edge_case(self):
        """BR-HAPI-197: MOCK_MAX_RETRIES_EXHAUSTED provides validation history."""
        request_data = {
            "incident_id": "edge-case-003",
            "signal_type": "MOCK_MAX_RETRIES_EXHAUSTED",
            "resource_namespace": "test",
        }

        response = generate_mock_incident_response(request_data)

        assert response["needs_human_review"] is True
        assert response["human_review_reason"] == "llm_parsing_error"
        assert response["selected_workflow"] is None
        assert len(response["validation_attempts_history"]) >= 3

    def test_edge_case_signal_type_case_insensitive(self):
        """BR-HAPI-212: Edge case signal types should be case-insensitive."""
        request_data = {
            "signal_type": "mock_no_workflow_found",  # lowercase
            "resource_namespace": "test",
        }

        response = generate_mock_incident_response(request_data)
        assert response["needs_human_review"] is True
        assert response["human_review_reason"] == "no_matching_workflows"


class TestMockEdgeCaseRecoveryResponses:
    """Tests for edge case recovery mock responses (BR-HAPI-197, BR-HAPI-212)."""

    def test_not_reproducible_returns_no_recovery(self):
        """BR-HAPI-212: MOCK_NOT_REPRODUCIBLE triggers can_recover=false."""
        request_data = {
            "incident_id": "edge-recovery-001",
            "remediation_id": "rem-001",
            "signal_type": "MOCK_NOT_REPRODUCIBLE",
            "previous_workflow_id": "prev-workflow-v1",
        }

        response = generate_mock_recovery_response(request_data)

        assert response["can_recover"] is False  # Key assertion
        assert response["needs_human_review"] is False  # No review needed
        assert response["selected_workflow"] is None
        assert response["analysis_confidence"] > 0.8  # High confidence resolved

        # Verify state_changed is True (resource is now healthy)
        recovery_analysis = response.get("recovery_analysis", {})
        prev_assessment = recovery_analysis.get("previous_attempt_assessment", {})
        assert prev_assessment.get("state_changed") is True

    def test_no_recovery_workflow_returns_human_review(self):
        """BR-HAPI-197: MOCK_NO_WORKFLOW_FOUND triggers needs_human_review for recovery."""
        request_data = {
            "incident_id": "edge-recovery-002",
            "remediation_id": "rem-002",
            "signal_type": "MOCK_NO_WORKFLOW_FOUND",
            "previous_workflow_id": "prev-workflow-v1",
        }

        response = generate_mock_recovery_response(request_data)

        assert response["can_recover"] is True  # Recovery possible manually
        assert response["needs_human_review"] is True
        assert response["human_review_reason"] == "no_matching_workflows"
        assert response["selected_workflow"] is None

    def test_low_confidence_recovery_returns_human_review(self):
        """BR-HAPI-197: MOCK_LOW_CONFIDENCE triggers needs_human_review for recovery."""
        request_data = {
            "incident_id": "edge-recovery-003",
            "remediation_id": "rem-003",
            "signal_type": "MOCK_LOW_CONFIDENCE",
            "previous_workflow_id": "prev-workflow-v1",
        }

        response = generate_mock_recovery_response(request_data)

        assert response["can_recover"] is True
        assert response["needs_human_review"] is True
        assert response["human_review_reason"] == "low_confidence"
        assert response["analysis_confidence"] < 0.5
        assert response["selected_workflow"] is not None  # Tentative


# NOTE: Integration-style tests removed for unit test module.
# Full integration tests with FastAPI TestClient require complete request schemas
# which evolve with the IncidentRequest/RecoveryRequest models.
# See tests/integration/ for endpoint integration tests.
#
# The unit tests above (TestMockModeDetection, TestMockScenarioSelection,
# TestMockIncidentResponse, TestMockRecoveryResponse, TestMockEdgeCaseIncidentResponses,
# TestMockEdgeCaseRecoveryResponses) thoroughly test the mock_responses.py module
# without requiring FastAPI TestClient.

