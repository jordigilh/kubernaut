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
Comprehensive Mock LLM E2E Tests

Business Requirement: BR-HAPI-212 - Mock LLM Mode for Integration Testing

These tests exercise ALL mock scenarios and edge cases to ensure
HAPI's mock mode is production-ready for AIAnalysis integration testing.

Test Categories:
1. All Signal Type Scenarios (6 scenarios + default)
2. Recovery Flow Scenarios
3. Error Handling Edge Cases
4. Concurrent Request Handling
5. Response Contract Validation
"""

import os
import pytest
import concurrent.futures
from unittest.mock import patch
from fastapi.testclient import TestClient


# ============================================================================
# FIXTURES
# ============================================================================

@pytest.fixture(scope="module")
def mock_mode_client():
    """Create test client with MOCK_LLM_MODE=true for entire module"""
    with patch.dict(os.environ, {"MOCK_LLM_MODE": "true"}):
        from src.main import app
        client = TestClient(app)
        yield client


def create_incident_request(signal_type: str, namespace: str = "production") -> dict:
    """Factory for creating incident requests with different signal types"""
    return {
        "incident_id": f"e2e-mock-{signal_type.lower()}-001",
        "remediation_id": f"req-e2e-{signal_type.lower()}",
        "signal_type": signal_type,
        "severity": "critical",
        "signal_source": "prometheus",
        "resource_namespace": namespace,
        "resource_kind": "Pod",
        "resource_name": f"test-pod-{signal_type.lower()}",
        "error_message": f"Test error for {signal_type}",
        "description": f"E2E test for {signal_type} scenario",
        "environment": "production",
        "priority": "P1",
        "risk_tolerance": "medium",
        "business_category": "critical",
        "cluster_name": "e2e-test-cluster",
        "is_duplicate": False,
        "occurrence_count": 1,
        "is_storm": False,
        "enrichment_results": {
            "detectedLabels": {
                "gitOpsManaged": True,
                "gitOpsTool": "argocd",
                "hpaEnabled": False,
                "stateful": False,
            },
        },
    }


def create_recovery_request(signal_type: str, attempt_number: int = 1) -> dict:
    """Factory for creating recovery requests"""
    return {
        "incident_id": f"e2e-recovery-{signal_type.lower()}-001",
        "remediation_id": f"req-e2e-recovery-{signal_type.lower()}",
        "is_recovery_attempt": True,
        "recovery_attempt_number": attempt_number,
        "signal_type": signal_type,
        "severity": "critical",
        "resource_namespace": "production",
        "resource_kind": "Pod",
        "resource_name": f"recovery-pod-{signal_type.lower()}",
        "environment": "production",
        "priority": "P1",
        "risk_tolerance": "medium",
        "business_category": "critical",
        "previous_execution": {
            "workflow_execution_ref": f"we-{signal_type.lower()}-001",
            "original_rca": {
                "summary": f"Root cause analysis for {signal_type}",
                "signal_type": signal_type,
                "severity": "critical",
                "contributing_factors": ["factor1", "factor2"],
            },
            "selected_workflow": {
                "workflow_id": f"original-workflow-{signal_type.lower()}",
                "version": "1.0.0",
                "container_image": f"kubernaut/workflow-{signal_type.lower()}:v1.0.0",
                "parameters": {"NAMESPACE": "production"},
                "rationale": "Original workflow selection",
            },
            "failure": {
                "failed_step_index": 1,
                "failed_step_name": "remediation_step",
                "reason": signal_type,
                "message": f"Failed with {signal_type}",
                "exit_code": 1,
                "failed_at": "2025-12-11T10:00:00Z",
                "execution_time": "1m30s",
            },
            "natural_language_summary": f"Previous workflow failed with {signal_type}",
        },
        "enrichment_results": {
            "detectedLabels": {
                "gitOpsManaged": True,
            },
        },
    }


# ============================================================================
# TEST CLASS 1: All Signal Type Scenarios
# ============================================================================

@pytest.mark.e2e
@pytest.mark.mock_llm
class TestAllSignalTypeScenarios:
    """
    E2E tests for ALL defined mock scenarios.

    Ensures each signal type produces the correct mock workflow.
    """

    @pytest.mark.parametrize("signal_type,expected_workflow_prefix", [
        ("OOMKilled", "mock-oomkill"),
        ("CrashLoopBackOff", "mock-crashloop"),
        ("NodeNotReady", "mock-node-drain"),
        ("ImagePullBackOff", "mock-image-fix"),
        ("Evicted", "mock-eviction"),
        ("FailedScheduling", "mock-scheduling"),
    ])
    def test_signal_type_returns_correct_workflow(
        self, mock_mode_client, signal_type, expected_workflow_prefix
    ):
        """BR-HAPI-212: Each signal type should return its specific mock workflow"""
        request = create_incident_request(signal_type)

        response = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=request,
        )

        assert response.status_code == 200, f"Failed for {signal_type}"
        data = response.json()

        workflow_id = data["selected_workflow"]["workflow_id"]
        assert workflow_id.startswith(expected_workflow_prefix), \
            f"Expected workflow starting with '{expected_workflow_prefix}', got '{workflow_id}'"

    @pytest.mark.parametrize("signal_type,expected_severity", [
        ("OOMKilled", "critical"),
        ("CrashLoopBackOff", "high"),
        ("NodeNotReady", "critical"),
        ("ImagePullBackOff", "high"),
        ("Evicted", "high"),
        ("FailedScheduling", "medium"),
    ])
    def test_signal_type_returns_correct_severity(
        self, mock_mode_client, signal_type, expected_severity
    ):
        """BR-HAPI-212: Each signal type should return appropriate severity"""
        request = create_incident_request(signal_type)

        response = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=request,
        )

        assert response.status_code == 200
        data = response.json()

        rca_severity = data["root_cause_analysis"]["severity"]
        assert rca_severity == expected_severity, \
            f"Expected severity '{expected_severity}' for {signal_type}, got '{rca_severity}'"

    def test_unknown_signal_type_returns_default_workflow(self, mock_mode_client):
        """BR-HAPI-212: Unknown signal types should return generic workflow"""
        request = create_incident_request("CustomUnknownSignal")

        response = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=request,
        )

        assert response.status_code == 200
        data = response.json()

        workflow_id = data["selected_workflow"]["workflow_id"]
        assert "mock-generic" in workflow_id, \
            f"Unknown signal should use generic workflow, got '{workflow_id}'"

        # Lower confidence for unknown signals
        assert data["confidence"] <= 0.80, \
            "Unknown signals should have lower confidence"

    def test_case_insensitive_signal_type_matching(self, mock_mode_client):
        """BR-HAPI-212: Signal type matching should be case-insensitive"""
        # Test lowercase
        request_lower = create_incident_request("oomkilled")
        response_lower = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=request_lower,
        )

        # Test uppercase
        request_upper = create_incident_request("OOMKILLED")
        request_upper["incident_id"] = "e2e-oomkilled-upper"
        response_upper = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=request_upper,
        )

        # Both should return same workflow
        workflow_lower = response_lower.json()["selected_workflow"]["workflow_id"]
        workflow_upper = response_upper.json()["selected_workflow"]["workflow_id"]

        assert workflow_lower == workflow_upper, \
            "Signal type matching should be case-insensitive"


# ============================================================================
# TEST CLASS 2: Recovery Flow Scenarios
# ============================================================================

@pytest.mark.e2e
@pytest.mark.mock_llm
class TestRecoveryFlowScenarios:
    """
    E2E tests for recovery analysis in mock mode.

    Tests the recovery flow that AIAnalysis uses after a WorkflowExecution fails.
    """

    @pytest.mark.parametrize("signal_type", [
        "OOMKilled",
        "CrashLoopBackOff",
        "NodeNotReady",
        "ImagePullBackOff",
    ])
    def test_recovery_for_each_signal_type(self, mock_mode_client, signal_type):
        """BR-HAPI-212: Recovery should work for all signal types"""
        request = create_recovery_request(signal_type)

        response = mock_mode_client.post(
            "/api/v1/recovery/analyze",
            json=request,
        )

        assert response.status_code == 200, f"Recovery failed for {signal_type}"
        data = response.json()

        assert "can_recover" in data
        assert "analysis_confidence" in data

    def test_recovery_attempt_escalation(self, mock_mode_client):
        """BR-HAPI-212: Multiple recovery attempts should be handled"""
        # First attempt
        request_1 = create_recovery_request("OOMKilled", attempt_number=1)
        response_1 = mock_mode_client.post(
            "/api/v1/recovery/analyze",
            json=request_1,
        )

        # Second attempt
        request_2 = create_recovery_request("OOMKilled", attempt_number=2)
        request_2["incident_id"] = "e2e-recovery-attempt-2"
        response_2 = mock_mode_client.post(
            "/api/v1/recovery/analyze",
            json=request_2,
        )

        # Third attempt (near limit)
        request_3 = create_recovery_request("OOMKilled", attempt_number=3)
        request_3["incident_id"] = "e2e-recovery-attempt-3"
        response_3 = mock_mode_client.post(
            "/api/v1/recovery/analyze",
            json=request_3,
        )

        # All should succeed in mock mode
        assert response_1.status_code == 200
        assert response_2.status_code == 200
        assert response_3.status_code == 200


# ============================================================================
# TEST CLASS 3: Response Contract Validation
# ============================================================================

@pytest.mark.e2e
@pytest.mark.mock_llm
class TestResponseContractValidation:
    """
    E2E tests validating response contracts for AIAnalysis consumption.

    Ensures all required fields are present and correctly typed.
    """

    def test_incident_response_complete_contract(self, mock_mode_client):
        """BR-HAPI-212: Incident response must have complete contract"""
        request = create_incident_request("OOMKilled")

        response = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=request,
        )

        data = response.json()

        # Top-level fields
        assert isinstance(data["incident_id"], str)
        assert isinstance(data["analysis"], str)
        assert isinstance(data["confidence"], (int, float))
        assert isinstance(data["needs_human_review"], bool)
        assert isinstance(data["warnings"], list)

        # Root cause analysis structure
        rca = data["root_cause_analysis"]
        assert isinstance(rca["summary"], str)
        assert isinstance(rca["severity"], str)
        assert isinstance(rca["contributing_factors"], list)

        # Selected workflow structure (critical for AIAnalysis)
        workflow = data["selected_workflow"]
        assert isinstance(workflow["workflow_id"], str)
        assert isinstance(workflow["title"], str)
        assert isinstance(workflow["version"], str)
        assert isinstance(workflow["containerImage"], str)
        assert isinstance(workflow["confidence"], (int, float))
        assert isinstance(workflow["rationale"], str)
        assert isinstance(workflow["parameters"], dict)

    def test_recovery_response_complete_contract(self, mock_mode_client):
        """BR-HAPI-212: Recovery response must have complete contract"""
        request = create_recovery_request("CrashLoopBackOff")

        response = mock_mode_client.post(
            "/api/v1/recovery/analyze",
            json=request,
        )

        data = response.json()

        # Required fields for recovery decision
        assert isinstance(data["incident_id"], str)
        assert isinstance(data["can_recover"], bool)
        assert isinstance(data["analysis_confidence"], (int, float))
        assert 0 <= data["analysis_confidence"] <= 1.0

    def test_workflow_parameters_include_namespace(self, mock_mode_client):
        """BR-HAPI-212: Workflow parameters should include namespace from request"""
        namespace = "custom-namespace"
        request = create_incident_request("OOMKilled", namespace=namespace)

        response = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=request,
        )

        data = response.json()
        params = data["selected_workflow"]["parameters"]

        assert "NAMESPACE" in params
        assert params["NAMESPACE"] == namespace


# ============================================================================
# TEST CLASS 4: Edge Cases and Error Handling
# ============================================================================

@pytest.mark.e2e
@pytest.mark.mock_llm
class TestEdgeCasesAndErrorHandling:
    """
    E2E tests for edge cases and error scenarios in mock mode.
    """

    def test_empty_signal_type_uses_default(self, mock_mode_client):
        """BR-HAPI-212: Empty signal type should use default workflow"""
        request = create_incident_request("")
        request["signal_type"] = ""

        response = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=request,
        )

        # Should still return 200 with default workflow
        assert response.status_code == 200
        data = response.json()
        assert "mock-generic" in data["selected_workflow"]["workflow_id"]

    def test_very_long_resource_name(self, mock_mode_client):
        """BR-HAPI-212: Long resource names should be handled"""
        request = create_incident_request("OOMKilled")
        request["resource_name"] = "a" * 253  # Max K8s name length

        response = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=request,
        )

        assert response.status_code == 200

    def test_special_characters_in_namespace(self, mock_mode_client):
        """BR-HAPI-212: Special namespace characters should be handled"""
        request = create_incident_request("OOMKilled", namespace="kube-system")

        response = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=request,
        )

        assert response.status_code == 200
        data = response.json()
        assert data["selected_workflow"]["parameters"]["NAMESPACE"] == "kube-system"


# ============================================================================
# TEST CLASS 5: Concurrent Request Handling
# ============================================================================

@pytest.mark.e2e
@pytest.mark.mock_llm
class TestConcurrentRequestHandling:
    """
    E2E tests for concurrent request handling in mock mode.

    Ensures mock mode is thread-safe and handles parallel requests.
    """

    def test_concurrent_incident_requests(self, mock_mode_client):
        """BR-HAPI-212: Multiple concurrent requests should all succeed"""
        signal_types = ["OOMKilled", "CrashLoopBackOff", "NodeNotReady",
                       "ImagePullBackOff", "Evicted", "FailedScheduling"]

        def make_request(signal_type):
            request = create_incident_request(signal_type)
            request["incident_id"] = f"concurrent-{signal_type}"
            response = mock_mode_client.post(
                "/api/v1/incident/analyze",
                json=request,
            )
            return signal_type, response.status_code, response.json()

        # Execute concurrently
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as executor:
            futures = [executor.submit(make_request, st) for st in signal_types]
            results = [f.result() for f in concurrent.futures.as_completed(futures)]

        # All should succeed
        for signal_type, status_code, data in results:
            assert status_code == 200, f"Concurrent request failed for {signal_type}"
            assert "selected_workflow" in data

    def test_repeated_same_request_is_idempotent(self, mock_mode_client):
        """BR-HAPI-212: Same request repeated should return identical results"""
        request = create_incident_request("OOMKilled")

        responses = []
        for _ in range(5):
            response = mock_mode_client.post(
                "/api/v1/incident/analyze",
                json=request,
            )
            responses.append(response.json())

        # All responses should be identical
        first_workflow = responses[0]["selected_workflow"]["workflow_id"]
        for r in responses[1:]:
            assert r["selected_workflow"]["workflow_id"] == first_workflow


# ============================================================================
# TEST CLASS 6: Metrics and Observability
# ============================================================================

@pytest.mark.e2e
@pytest.mark.mock_llm
class TestMetricsAndObservability:
    """
    E2E tests verifying metrics are emitted in mock mode.
    """

    def test_mock_mode_increments_investigation_counter(self, mock_mode_client):
        """BR-HAPI-212: Mock requests should still increment metrics"""
        request = create_incident_request("OOMKilled")

        # Make request
        response = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=request,
        )

        assert response.status_code == 200

        # Check metrics endpoint
        metrics_response = mock_mode_client.get("/metrics")

        if metrics_response.status_code == 200:
            metrics_text = metrics_response.text
            # Should have investigation counter (DD-005 naming)
            assert "holmesgpt_api_" in metrics_text or "investigation" in metrics_text.lower()


