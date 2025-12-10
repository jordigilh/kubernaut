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
Mock LLM Mode Integration Tests

Business Requirement: BR-HAPI-212 - Mock LLM Mode for Integration Testing

These tests verify that AIAnalysis (and other consumers) can successfully
call HAPI endpoints when MOCK_LLM_MODE=true, receiving deterministic
responses without requiring an actual LLM provider.

Test Scenarios:
1. Full endpoint integration with mock mode
2. Request validation still enforced in mock mode
3. Response schema compliance for AIAnalysis consumption
4. Deterministic responses for integration test stability
"""

import os
import pytest
from unittest.mock import patch
from fastapi.testclient import TestClient


@pytest.fixture
def mock_mode_client():
    """Create test client with MOCK_LLM_MODE=true"""
    with patch.dict(os.environ, {"MOCK_LLM_MODE": "true"}):
        # Import app after setting env var
        from src.main import app
        client = TestClient(app)
        yield client


@pytest.fixture
def sample_incident_request():
    """Complete IncidentRequest payload as AIAnalysis would send"""
    return {
        "incident_id": "integration-test-incident-001",
        "remediation_id": "req-2025-12-10-integration-test",
        "signal_type": "OOMKilled",
        "severity": "critical",
        "signal_source": "prometheus",
        "resource_namespace": "production",
        "resource_kind": "Pod",
        "resource_name": "api-server-abc123",
        "error_message": "Container exceeded memory limit",
        "description": "Pod terminated due to OOM",
        "environment": "production",
        "priority": "P1",
        "risk_tolerance": "medium",
        "business_category": "critical",
        "cluster_name": "prod-cluster-1",
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
            "kubernetesContext": {
                "podPhase": "Failed",
                "containerState": "Terminated",
            },
        },
    }


@pytest.fixture
def sample_recovery_request():
    """Complete RecoveryRequest payload as AIAnalysis would send"""
    return {
        "incident_id": "integration-test-incident-001",
        "remediation_id": "req-2025-12-10-integration-recovery",
        "is_recovery_attempt": True,
        "recovery_attempt_number": 1,
        "signal_type": "OOMKilled",
        "severity": "critical",
        "resource_namespace": "production",
        "resource_kind": "Pod",
        "resource_name": "api-server-abc123",
        "environment": "production",
        "priority": "P1",
        "risk_tolerance": "medium",
        "business_category": "critical",
        "previous_execution": {
            "workflow_execution_ref": "req-2025-12-10-we-1",
            "original_rca": {
                "summary": "Memory exhaustion causing OOMKilled",
                "signal_type": "OOMKilled",
                "severity": "critical",
                "contributing_factors": ["memory_leak", "insufficient_limits"],
            },
            "selected_workflow": {
                "workflow_id": "scale-horizontal-v1",
                "version": "1.0.0",
                "container_image": "kubernaut/workflow-scale:v1.0.0",
                "parameters": {"TARGET_REPLICAS": "5"},
                "rationale": "Scale out to distribute load",
            },
            "failure": {
                "failed_step_index": 2,
                "failed_step_name": "scale_deployment",
                "reason": "OOMKilled",
                "message": "Container exceeded memory limit during scale",
                "exit_code": 137,
                "failed_at": "2025-12-10T10:30:00Z",
                "execution_time": "2m34s",
            },
            "natural_language_summary": "Previous workflow failed at step 2 (scale_deployment) with OOMKilled",
        },
        "enrichment_results": {
            "detectedLabels": {
                "gitOpsManaged": True,
                "gitOpsTool": "argocd",
            },
        },
    }


class TestMockModeIncidentIntegration:
    """Integration tests for /incident/analyze in mock mode"""

    def test_incident_endpoint_returns_200_in_mock_mode(
        self, mock_mode_client, sample_incident_request
    ):
        """BR-HAPI-212: Incident endpoint should return 200 in mock mode"""
        response = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=sample_incident_request,
        )
        assert response.status_code == 200

    def test_incident_response_has_aianalysis_required_fields(
        self, mock_mode_client, sample_incident_request
    ):
        """BR-HAPI-212: Response must have fields AIAnalysis expects"""
        response = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=sample_incident_request,
        )
        data = response.json()

        # Fields AIAnalysis MUST have to create WorkflowExecution
        assert "incident_id" in data
        assert "analysis" in data
        assert "root_cause_analysis" in data
        assert "selected_workflow" in data
        assert "confidence" in data
        assert "needs_human_review" in data
        assert "warnings" in data

    def test_incident_response_workflow_has_required_fields(
        self, mock_mode_client, sample_incident_request
    ):
        """BR-HAPI-212: selected_workflow must have fields for WorkflowExecution"""
        response = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=sample_incident_request,
        )
        data = response.json()
        workflow = data["selected_workflow"]

        # Fields AIAnalysis needs for WorkflowExecution CRD
        assert "workflow_id" in workflow
        assert "containerImage" in workflow
        assert "parameters" in workflow
        assert "confidence" in workflow
        assert "rationale" in workflow

    def test_incident_response_is_deterministic(
        self, mock_mode_client, sample_incident_request
    ):
        """BR-HAPI-212: Same request should produce same response (for stable tests)"""
        response1 = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=sample_incident_request,
        )
        response2 = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=sample_incident_request,
        )

        data1 = response1.json()
        data2 = response2.json()

        # Core fields should be identical
        assert data1["incident_id"] == data2["incident_id"]
        assert data1["selected_workflow"]["workflow_id"] == data2["selected_workflow"]["workflow_id"]
        assert data1["confidence"] == data2["confidence"]

    def test_incident_validation_still_enforced_in_mock_mode(
        self, mock_mode_client
    ):
        """BR-HAPI-212: Request validation should still run in mock mode"""
        # Missing required fields
        invalid_request = {
            "incident_id": "test-123",
            # Missing: remediation_id, signal_type, severity, etc.
        }

        response = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=invalid_request,
        )

        # Should return 4xx error (400 Bad Request per RFC 7807, or 422 Validation Error)
        assert response.status_code in [400, 422]

    def test_incident_mock_response_indicates_mock_mode(
        self, mock_mode_client, sample_incident_request
    ):
        """BR-HAPI-212: Response should indicate it's from mock mode"""
        response = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=sample_incident_request,
        )
        data = response.json()

        # Warnings should indicate mock mode
        assert any("MOCK" in w.upper() for w in data.get("warnings", []))

    def test_incident_different_signal_types_produce_different_workflows(
        self, mock_mode_client, sample_incident_request
    ):
        """BR-HAPI-212: Different signal types should return different workflows"""
        # OOMKilled
        sample_incident_request["signal_type"] = "OOMKilled"
        response_oom = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=sample_incident_request,
        )

        # CrashLoopBackOff
        sample_incident_request["signal_type"] = "CrashLoopBackOff"
        sample_incident_request["incident_id"] = "test-crashloop"
        response_crash = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=sample_incident_request,
        )

        # Should have different workflows
        workflow_oom = response_oom.json()["selected_workflow"]["workflow_id"]
        workflow_crash = response_crash.json()["selected_workflow"]["workflow_id"]

        assert workflow_oom != workflow_crash


class TestMockModeRecoveryIntegration:
    """Integration tests for /recovery/analyze in mock mode"""

    def test_recovery_endpoint_returns_200_in_mock_mode(
        self, mock_mode_client, sample_recovery_request
    ):
        """BR-HAPI-212: Recovery endpoint should return 200 in mock mode"""
        response = mock_mode_client.post(
            "/api/v1/recovery/analyze",
            json=sample_recovery_request,
        )
        assert response.status_code == 200

    def test_recovery_response_has_aianalysis_required_fields(
        self, mock_mode_client, sample_recovery_request
    ):
        """BR-HAPI-212: Recovery response must have fields AIAnalysis expects"""
        response = mock_mode_client.post(
            "/api/v1/recovery/analyze",
            json=sample_recovery_request,
        )
        data = response.json()

        # Fields AIAnalysis needs for recovery WorkflowExecution
        assert "incident_id" in data
        assert "can_recover" in data
        assert "analysis_confidence" in data

    def test_recovery_response_is_deterministic(
        self, mock_mode_client, sample_recovery_request
    ):
        """BR-HAPI-212: Same request should produce same response"""
        response1 = mock_mode_client.post(
            "/api/v1/recovery/analyze",
            json=sample_recovery_request,
        )
        response2 = mock_mode_client.post(
            "/api/v1/recovery/analyze",
            json=sample_recovery_request,
        )

        data1 = response1.json()
        data2 = response2.json()

        assert data1["incident_id"] == data2["incident_id"]
        assert data1["can_recover"] == data2["can_recover"]


class TestMockModeAIAnalysisScenarios:
    """Integration tests simulating AIAnalysis controller workflows"""

    def test_aianalysis_initial_investigation_flow(
        self, mock_mode_client, sample_incident_request
    ):
        """
        BR-HAPI-212: Simulate AIAnalysis initial investigation

        Flow:
        1. AIAnalysis receives RemediationProcessing
        2. Calls /incident/analyze
        3. Creates WorkflowExecution from response
        """
        # Step 1: Call incident endpoint (what AIAnalysis does)
        response = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=sample_incident_request,
        )

        assert response.status_code == 200
        data = response.json()

        # Step 2: Verify AIAnalysis can extract fields for WorkflowExecution
        assert data["incident_id"] == sample_incident_request["incident_id"]

        # For WorkflowExecution.spec.workflowRef
        assert data["selected_workflow"]["workflow_id"]

        # For WorkflowExecution.spec.containerImage
        assert data["selected_workflow"]["containerImage"]

        # For WorkflowExecution.spec.parameters
        assert isinstance(data["selected_workflow"]["parameters"], dict)

        # Check if human review is needed
        if data["needs_human_review"]:
            assert "human_review_reason" in data or len(data["warnings"]) > 0

    def test_aianalysis_recovery_flow_after_failure(
        self, mock_mode_client, sample_recovery_request
    ):
        """
        BR-HAPI-212: Simulate AIAnalysis recovery attempt

        Flow:
        1. WorkflowExecution failed
        2. AIAnalysis calls /recovery/analyze with failure context
        3. Gets recovery workflow recommendation
        """
        # Call recovery endpoint (what AIAnalysis does after WE failure)
        response = mock_mode_client.post(
            "/api/v1/recovery/analyze",
            json=sample_recovery_request,
        )

        assert response.status_code == 200
        data = response.json()

        # Verify AIAnalysis can determine if recovery is possible
        assert "can_recover" in data
        assert isinstance(data["can_recover"], bool)

        # Verify confidence for decision making
        assert "analysis_confidence" in data
        assert 0 <= data["analysis_confidence"] <= 1.0

    def test_aianalysis_handles_low_confidence_appropriately(
        self, mock_mode_client, sample_incident_request
    ):
        """
        BR-HAPI-212: AIAnalysis should handle low-confidence responses

        When confidence is low, AIAnalysis may:
        - Set needs_human_review = true
        - Not auto-create WorkflowExecution
        """
        # Use unknown signal type (lower confidence)
        sample_incident_request["signal_type"] = "UnknownCustomSignal"

        response = mock_mode_client.post(
            "/api/v1/incident/analyze",
            json=sample_incident_request,
        )

        data = response.json()

        # Even with unknown signal, should get a response
        assert response.status_code == 200
        assert "confidence" in data

        # For unknown signals, confidence should be lower (mock uses 0.75)
        # AIAnalysis would check this before auto-creating WE

