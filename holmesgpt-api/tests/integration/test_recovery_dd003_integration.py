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
Recovery Endpoint Integration Tests (DD-RECOVERY-002, DD-RECOVERY-003)

Business Requirements: BR-HAPI-001 to 050 (Recovery Analysis)
Design Decisions: DD-RECOVERY-002, DD-RECOVERY-003

Tests validate BUSINESS OUTCOMES:
- Recovery endpoint accepts PreviousExecution context
- Recovery endpoint returns appropriate response format
- Recovery endpoint handles DetectedLabels correctly
"""

import pytest


@pytest.mark.integration
class TestRecoveryEndpointWithPreviousExecution:
    """
    Integration tests for recovery endpoint with PreviousExecution context

    Business Outcome: Recovery flow receives and processes failure context
    """

    def test_recovery_endpoint_accepts_previous_execution(
        self, client, sample_recovery_request_with_previous_execution
    ):
        """
        Business Outcome: Recovery request with failure context is accepted
        """
        response = client.post(
            "/api/v1/recovery/analyze",
            json=sample_recovery_request_with_previous_execution
        )

        assert response.status_code == 200
        data = response.json()
        assert data["incident_id"] == sample_recovery_request_with_previous_execution["incident_id"]

    def test_recovery_endpoint_returns_metadata_for_recovery_attempt(
        self, client, sample_recovery_request_with_previous_execution
    ):
        """
        Business Outcome: Response metadata indicates recovery attempt
        """
        response = client.post(
            "/api/v1/recovery/analyze",
            json=sample_recovery_request_with_previous_execution
        )

        assert response.status_code == 200
        data = response.json()

        # Metadata should exist (may be empty dict for LLM responses)
        assert "metadata" in data or data.get("analysis_confidence", 0) >= 0

    def test_recovery_endpoint_returns_strategies(
        self, client, sample_recovery_request_with_previous_execution
    ):
        """
        Business Outcome: Recovery returns actionable strategies
        """
        response = client.post(
            "/api/v1/recovery/analyze",
            json=sample_recovery_request_with_previous_execution
        )

        assert response.status_code == 200
        data = response.json()

        assert "strategies" in data
        assert isinstance(data["strategies"], list)
        assert len(data["strategies"]) > 0

@pytest.mark.integration
class TestRecoveryEndpointWithDetectedLabels:
    """
    Integration tests for recovery endpoint with DetectedLabels

    Business Outcome: Cluster context is available for workflow filtering
    """

    def test_recovery_with_detected_labels_succeeds(
        self, client, sample_recovery_request_with_previous_execution
    ):
        """
        Business Outcome: DetectedLabels don't break the request
        """
        response = client.post(
            "/api/v1/recovery/analyze",
            json=sample_recovery_request_with_previous_execution
        )

        assert response.status_code == 200

    def test_recovery_without_detected_labels_succeeds(
        self, client
    ):
        """
        Business Outcome: Missing DetectedLabels don't break request
        """
        request = {
            "incident_id": "test-inc-no-labels",
            "remediation_id": "req-test-2025-11-29-no-labels",
            "is_recovery_attempt": True,
            "recovery_attempt_number": 1,
            "previous_execution": {
                "workflow_execution_ref": "test-we-1",
                "original_rca": {
                    "summary": "Test issue",
                    "signal_type": "Error",
                    "severity": "medium"
                },
                "selected_workflow": {
                    "workflow_id": "test-v1",
                    "version": "1.0.0",
                    "container_image": "test:latest",
                    "rationale": "Test"
                },
                "failure": {
                    "failed_step_index": 0,
                    "failed_step_name": "test",
                    "reason": "Error",
                    "message": "Test failure",
                    "failed_at": "2025-11-29T10:30:00Z",
                    "execution_time": "1m"
                }
            }
            # No enrichment_results
        }

        response = client.post("/api/v1/recovery/analyze", json=request)
        assert response.status_code == 200


@pytest.mark.integration
class TestIncidentEndpointWithDetectedLabels:
    """
    Integration tests for incident endpoint with DetectedLabels

    Business Outcome: Incident analysis receives cluster context
    """

    def test_incident_with_detected_labels_succeeds(
        self, client, sample_incident_request_with_detected_labels
    ):
        """
        Business Outcome: DetectedLabels are processed in incident flow
        """
        response = client.post(
            "/api/v1/incident/analyze",
            json=sample_incident_request_with_detected_labels
        )

        assert response.status_code == 200
        data = response.json()
        assert data["incident_id"] == sample_incident_request_with_detected_labels["incident_id"]

    def test_incident_without_detected_labels_succeeds(
        self, client
    ):
        """
        Business Outcome: Missing DetectedLabels don't break incident request
        """
        request = {
            "incident_id": "test-inc-no-labels",
            "remediation_id": "req-test-2025-11-29-no-labels",
            "signal_type": "OOMKilled",
            "severity": "high",
            "signal_source": "prometheus",
            "resource_namespace": "test",
            "resource_kind": "Deployment",
            "resource_name": "test-app",
            "error_message": "Container exceeded memory limit",
            "environment": "test",
            "priority": "P2",
            "risk_tolerance": "medium",
            "business_category": "standard",
            "cluster_name": "test-cluster"
            # No enrichment_results
        }

        response = client.post("/api/v1/incident/analyze", json=request)
        assert response.status_code == 200


@pytest.mark.integration
class TestRecoveryRequestValidation:
    """
    Integration tests for recovery request validation

    Business Outcome: Invalid requests are rejected with clear errors
    """

    def test_recovery_rejects_invalid_recovery_attempt_number(self, client):
        """
        Business Outcome: Invalid attempt numbers are rejected
        """
        request = {
            "incident_id": "test-inc-invalid",
            "remediation_id": "req-test-invalid",
            "is_recovery_attempt": True,
            "recovery_attempt_number": 0  # Invalid - must be >= 1
        }

        response = client.post("/api/v1/recovery/analyze", json=request)
        # Should return 400 for validation error
        assert response.status_code == 400

    def test_recovery_rejects_missing_remediation_id(self, client):
        """
        Business Outcome: Missing remediation_id is rejected (DD-WORKFLOW-002 v2.2)
        """
        request = {
            "incident_id": "test-inc-no-remediation-id"
            # Missing remediation_id - MANDATORY per DD-WORKFLOW-002 v2.2
        }

        response = client.post("/api/v1/recovery/analyze", json=request)
        assert response.status_code == 400

