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

INTEGRATION TEST COMPLIANCE:
Per TESTING_GUIDELINES.md:614 - Integration tests MUST use real services via podman-compose.
These tests use OpenAPI-generated client to validate API contract compliance.

MIGRATION: Updated to use HAPI OpenAPI client (Phase 2)
Authority: TRIAGE_HAPI_E2E_AND_CLIENT_GAPS.md
"""

import pytest
import sys
sys.path.insert(0, 'tests/clients')

from holmesgpt_api_client import ApiClient, Configuration
from holmesgpt_api_client.api.recovery_analysis_api import RecoveryAnalysisApi
from holmesgpt_api_client.api.incident_analysis_api import IncidentAnalysisApi
from holmesgpt_api_client.models.recovery_request import RecoveryRequest
from holmesgpt_api_client.models.incident_request import IncidentRequest
from holmesgpt_api_client.models.previous_execution import PreviousExecution
from holmesgpt_api_client.models.detected_labels import DetectedLabels
from holmesgpt_api_client.exceptions import ApiException


@pytest.mark.integration
class TestRecoveryEndpointWithPreviousExecution:
    """
    Integration tests for recovery endpoint with PreviousExecution context

    Business Outcome: Recovery flow receives and processes failure context
    """

    def test_recovery_endpoint_accepts_previous_execution(
        self, hapi_service_url, sample_recovery_request_with_previous_execution
    ):
        """
        Business Outcome: Recovery request with failure context is accepted via OpenAPI client

        Validates: OpenAPI contract compliance for recovery endpoint
        """
        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        recovery_api = RecoveryAnalysisApi(client)

        # Build typed request from sample data
        request_data = sample_recovery_request_with_previous_execution
        recovery_request = RecoveryRequest(
            remediation_id=request_data["remediation_id"],
            incident_id=request_data["incident_id"],
            signal_type=request_data.get("signal_type"),
            namespace=request_data.get("namespace"),
            previous_workflow_id=request_data.get("previous_workflow_id"),
            previous_execution=PreviousExecution(**request_data["previous_execution"]) if "previous_execution" in request_data else None
        )

        # Act: Call API via OpenAPI client
        response = recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
            recovery_request=recovery_request
        )

        # Assert: Validate typed response
        assert response.incident_id == request_data["incident_id"]

    def test_recovery_endpoint_returns_metadata_for_recovery_attempt(
        self, hapi_service_url, sample_recovery_request_with_previous_execution
    ):
        """
        Business Outcome: Response metadata indicates recovery attempt via OpenAPI client
        """
        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        recovery_api = RecoveryAnalysisApi(client)

        # Build typed request from sample data
        request_data = sample_recovery_request_with_previous_execution
        recovery_request = RecoveryRequest(
            remediation_id=request_data["remediation_id"],
            incident_id=request_data["incident_id"],
            signal_type=request_data.get("signal_type"),
            namespace=request_data.get("namespace"),
            previous_workflow_id=request_data.get("previous_workflow_id"),
            previous_execution=PreviousExecution(**request_data["previous_execution"]) if "previous_execution" in request_data else None
        )

        # Act: Call API via OpenAPI client
        response = recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
            recovery_request=recovery_request
        )

        # Assert: Validate typed response has metadata
        assert response.metadata is not None

    def test_recovery_endpoint_returns_strategies(
        self, hapi_service_url, sample_recovery_request_with_previous_execution
    ):
        """
        Business Outcome: Recovery returns actionable strategies via OpenAPI client
        """
        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        recovery_api = RecoveryAnalysisApi(client)

        # Build typed request from sample data
        request_data = sample_recovery_request_with_previous_execution
        recovery_request = RecoveryRequest(
            remediation_id=request_data["remediation_id"],
            incident_id=request_data["incident_id"],
            signal_type=request_data.get("signal_type"),
            namespace=request_data.get("namespace"),
            previous_workflow_id=request_data.get("previous_workflow_id"),
            previous_execution=PreviousExecution(**request_data["previous_execution"]) if "previous_execution" in request_data else None
        )

        # Act: Call API via OpenAPI client
        response = recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
            recovery_request=recovery_request
        )

        # Assert: Validate typed response has strategies
        assert response.strategies is not None
        assert len(response.strategies) > 0

@pytest.mark.integration
class TestRecoveryEndpointWithDetectedLabels:
    """
    Integration tests for recovery endpoint with DetectedLabels

    Business Outcome: Cluster context is available for workflow filtering
    """

    def test_recovery_with_detected_labels_succeeds(
        self, hapi_service_url, sample_recovery_request_with_previous_execution
    ):
        """
        Business Outcome: DetectedLabels don't break the request via OpenAPI client
        """
        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        recovery_api = RecoveryAnalysisApi(client)

        # Build typed request from sample data
        request_data = sample_recovery_request_with_previous_execution
        recovery_request = RecoveryRequest(
            remediation_id=request_data["remediation_id"],
            incident_id=request_data["incident_id"],
            signal_type=request_data.get("signal_type"),
            namespace=request_data.get("namespace"),
            previous_workflow_id=request_data.get("previous_workflow_id"),
            previous_execution=PreviousExecution(**request_data["previous_execution"]) if "previous_execution" in request_data else None
        )

        # Act: Call API via OpenAPI client (should not raise exception)
        response = recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
            recovery_request=recovery_request
        )

        # Assert: Request succeeded
        assert response.incident_id == request_data["incident_id"]

    def test_recovery_without_detected_labels_succeeds(
        self, hapi_service_url
    ):
        """
        Business Outcome: Missing DetectedLabels don't break request via OpenAPI client
        """
        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        recovery_api = RecoveryAnalysisApi(client)

        # Build request without detected labels
        recovery_request = RecoveryRequest(
            incident_id="test-inc-no-labels",
            remediation_id="req-test-2025-11-29-no-labels",
            is_recovery_attempt=True,
            recovery_attempt_number=1,
            previous_execution=PreviousExecution(
                workflow_execution_ref="test-we-1",
                original_rca={
                    "summary": "Test issue",
                    "signal_type": "Error",
                    "severity": "medium"
                },
                selected_workflow={
                    "workflow_id": "test-v1",
                    "version": "1.0.0",
                    "execution_bundle": "test:latest",
                    "rationale": "Test"
                },
                failure={
                    "failed_step_index": 0,
                    "failed_step_name": "test",
                    "reason": "Error",
                    "message": "Test failure",
                    "failed_at": "2025-11-29T10:30:00Z",
                    "execution_time": "1m"
                }
            )
            # No enrichment_results
        )

        # Act: Call API via OpenAPI client
        response = recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
            recovery_request=recovery_request
        )

        # Assert: Request succeeded
        assert response.incident_id == "test-inc-no-labels"


@pytest.mark.integration
class TestIncidentEndpointWithDetectedLabels:
    """
    Integration tests for incident endpoint with DetectedLabels

    Business Outcome: Incident analysis receives cluster context
    """

    def test_incident_with_detected_labels_succeeds(
        self, hapi_service_url, sample_incident_request_with_detected_labels
    ):
        """
        Business Outcome: DetectedLabels are processed in incident flow via OpenAPI client
        """
        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        incidents_api = IncidentAnalysisApi(client)

        # Build typed request from sample data
        request_data = sample_incident_request_with_detected_labels
        incident_request = IncidentRequest(
            incident_id=request_data["incident_id"],
            remediation_id=request_data["remediation_id"],
            signal_type=request_data["signal_type"],
            severity=request_data["severity"],
            signal_source=request_data.get("signal_source"),
            resource_namespace=request_data.get("resource_namespace"),
            resource_kind=request_data.get("resource_kind"),
            resource_name=request_data.get("resource_name"),
            error_message=request_data.get("error_message"),
            environment=request_data.get("environment"),
            priority=request_data.get("priority"),
            risk_tolerance=request_data.get("risk_tolerance"),
            business_category=request_data.get("business_category"),
            cluster_name=request_data.get("cluster_name"),
            enrichment_results=request_data.get("enrichment_results")
        )

        # Act: Call API via OpenAPI client
        response = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request
        )

        # Assert: Validate typed response
        assert response.incident_id == request_data["incident_id"]

    def test_incident_without_detected_labels_succeeds(
        self, hapi_service_url
    ):
        """
        Business Outcome: Missing DetectedLabels don't break incident request via OpenAPI client
        """
        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        incidents_api = IncidentAnalysisApi(client)

        # Build request without detected labels
        incident_request = IncidentRequest(
            incident_id="test-inc-no-labels",
            remediation_id="req-test-2025-11-29-no-labels",
            signal_type="OOMKilled",
            severity="high",
            signal_source="prometheus",
            resource_namespace="test",
            resource_kind="Deployment",
            resource_name="test-app",
            error_message="Container exceeded memory limit",
            environment="test",
            priority="P2",
            risk_tolerance="medium",
            business_category="standard",
            cluster_name="test-cluster"
            # No enrichment_results
        )

        # Act: Call API via OpenAPI client
        response = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request
        )

        # Assert: Request succeeded
        assert response.incident_id == "test-inc-no-labels"


@pytest.mark.integration
class TestRecoveryRequestValidation:
    """
    Integration tests for recovery request validation

    Business Outcome: Invalid requests are rejected with clear errors
    """

    def test_recovery_rejects_invalid_recovery_attempt_number(self, hapi_service_url):
        """
        Business Outcome: Invalid attempt numbers are rejected via OpenAPI client

        Note: Pydantic validation happens at client-side before API call,
        so we expect ValidationError, not ApiException.
        """
        from pydantic import ValidationError

        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        recovery_api = RecoveryAnalysisApi(client)

        # Act & Assert: Should raise ValidationError (client-side validation)
        with pytest.raises(ValidationError) as exc_info:
            recovery_request = RecoveryRequest(
                incident_id="test-inc-invalid",
                remediation_id="req-test-invalid",
                is_recovery_attempt=True,
                recovery_attempt_number=0  # Invalid - must be >= 1
            )

        # Verify it's the recovery_attempt_number field that failed
        assert 'recovery_attempt_number' in str(exc_info.value)

    def test_recovery_rejects_missing_remediation_id(self, hapi_service_url):
        """
        Business Outcome: Missing remediation_id is rejected (DD-WORKFLOW-002 v2.2) via OpenAPI client

        Note: Pydantic validation happens at client-side before API call,
        so we expect ValidationError, not ApiException.
        """
        from pydantic import ValidationError

        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        recovery_api = RecoveryAnalysisApi(client)

        # Act & Assert: Should raise ValidationError (client-side validation)
        with pytest.raises(ValidationError) as exc_info:
            recovery_request = RecoveryRequest(
                incident_id="test-inc-no-remediation-id"
                # Missing remediation_id - MANDATORY per DD-WORKFLOW-002 v2.2
            )

        # Verify it's the remediation_id field that failed
        assert 'remediation_id' in str(exc_info.value)

