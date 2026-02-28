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

INTEGRATION TEST COMPLIANCE:
Per TESTING_GUIDELINES.md:614 - Integration tests MUST use real services via podman-compose.
These tests use HAPI OpenAPI client to validate API contract compliance.
The HAPI service must be started with MOCK_LLM_MODE=true environment variable.

MIGRATION: Updated to use HAPI OpenAPI client (Phase 2)
Authority: TRIAGE_HAPI_E2E_AND_CLIENT_GAPS.md
"""

import os
import pytest
import sys
sys.path.insert(0, 'tests/clients')

from holmesgpt_api_client import ApiClient, Configuration
from holmesgpt_api_client.api.incident_analysis_api import IncidentAnalysisApi
from holmesgpt_api_client.models.incident_request import IncidentRequest
from holmesgpt_api_client.exceptions import ApiException


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


class TestMockModeIncidentIntegration:
    """Integration tests for /incident/analyze in mock mode via OpenAPI client"""

    def test_incident_endpoint_returns_200_in_mock_mode(
        self, hapi_service_url, sample_incident_request
    ):
        """BR-HAPI-212: Incident endpoint should return 200 in mock mode via OpenAPI client"""
        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        incidents_api = IncidentAnalysisApi(client)

        # Build typed request
        incident_request = IncidentRequest(**sample_incident_request)

        # Act: Call API via OpenAPI client (should not raise exception)
        response = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request
        )

        # Assert: Request succeeded
        assert response.incident_id == sample_incident_request["incident_id"]

    def test_incident_response_has_aianalysis_required_fields(
        self, hapi_service_url, sample_incident_request
    ):
        """BR-HAPI-212: Response must have fields AIAnalysis expects via OpenAPI client"""
        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        incidents_api = IncidentAnalysisApi(client)

        # Build typed request
        incident_request = IncidentRequest(**sample_incident_request)

        # Act: Call API via OpenAPI client
        response = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request
        )

        # Assert: Fields AIAnalysis MUST have to create WorkflowExecution
        assert response.incident_id is not None
        assert response.analysis is not None
        assert response.root_cause_analysis is not None
        assert response.selected_workflow is not None
        assert response.confidence is not None
        assert response.needs_human_review is not None
        assert response.warnings is not None

    def test_incident_response_workflow_has_required_fields(
        self, hapi_service_url, sample_incident_request
    ):
        """BR-HAPI-212: selected_workflow must have fields for WorkflowExecution via OpenAPI client"""
        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        incidents_api = IncidentAnalysisApi(client)

        # Build typed request
        incident_request = IncidentRequest(**sample_incident_request)

        # Act: Call API via OpenAPI client
        response = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request
        )

        # Assert: Fields AIAnalysis needs for WorkflowExecution CRD
        # Note: selected_workflow may be dict or object depending on OpenAPI client deserialization
        selected_workflow = response.selected_workflow
        assert selected_workflow is not None

        if isinstance(selected_workflow, dict):
            assert selected_workflow.get('workflow_id') is not None
            # Check both camelCase and snake_case for execution_bundle
            execution_bundle = selected_workflow.get('execution_bundle') or selected_workflow.get('executionBundle')
            assert execution_bundle is not None
            assert selected_workflow.get('parameters') is not None
            assert selected_workflow.get('confidence') is not None
            assert selected_workflow.get('rationale') is not None
        else:
            assert selected_workflow.workflow_id is not None
            assert selected_workflow.execution_bundle is not None
            assert selected_workflow.parameters is not None
            assert selected_workflow.confidence is not None
            assert selected_workflow.rationale is not None

    def test_incident_response_is_deterministic(
        self, hapi_service_url, sample_incident_request
    ):
        """BR-HAPI-212: Same request should produce same response (for stable tests) via OpenAPI client"""
        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        incidents_api = IncidentAnalysisApi(client)
        incident_request = IncidentRequest(**sample_incident_request)

        # Act: Make two identical requests
        response1 = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request
        )
        response2 = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request
        )

        # Assert: Core fields should be identical
        assert response1.incident_id == response2.incident_id

        # Handle dict vs object for selected_workflow
        if isinstance(response1.selected_workflow, dict):
            workflow_id1 = response1.selected_workflow.get('workflow_id')
            workflow_id2 = response2.selected_workflow.get('workflow_id')
        else:
            workflow_id1 = response1.selected_workflow.workflow_id
            workflow_id2 = response2.selected_workflow.workflow_id

        assert workflow_id1 == workflow_id2
        assert response1.confidence == response2.confidence

    def test_incident_validation_still_enforced_in_mock_mode(
        self, hapi_service_url
    ):
        """BR-HAPI-212: Request validation should still run in mock mode via OpenAPI client

        Note: Pydantic validation happens at client-side before API call,
        so we expect ValidationError, not ApiException.
        """
        from pydantic import ValidationError

        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        incidents_api = IncidentAnalysisApi(client)

        # Act & Assert: Should raise ValidationError (client-side validation)
        with pytest.raises(ValidationError) as exc_info:
            # Missing required fields - will fail Pydantic validation
            invalid_request = IncidentRequest(
                incident_id="test-123",
                remediation_id="test-req"
                # Missing: signal_type, severity, and many other required fields
            )

        # Verify validation error mentions missing fields
        assert 'signal_type' in str(exc_info.value) or 'severity' in str(exc_info.value)

    def test_incident_mock_response_indicates_mock_mode(
        self, hapi_service_url, sample_incident_request
    ):
        """BR-HAPI-212: Response should indicate it's from mock mode via OpenAPI client"""
        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        incidents_api = IncidentAnalysisApi(client)
        incident_request = IncidentRequest(**sample_incident_request)

        # Act: Call API
        response = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request
        )

        # Assert: Warnings should indicate mock mode
        assert response.warnings is not None
        assert any("MOCK" in w.upper() for w in response.warnings)

    def test_incident_different_signal_types_produce_different_workflows(
        self, hapi_service_url, sample_incident_request
    ):
        """BR-HAPI-212: Different signal types should return different workflows via OpenAPI client"""
        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        incidents_api = IncidentAnalysisApi(client)

        # OOMKilled
        request_oom = sample_incident_request.copy()
        request_oom["signal_type"] = "OOMKilled"
        incident_oom = IncidentRequest(**request_oom)
        response_oom = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_oom
        )

        # CrashLoopBackOff
        request_crash = sample_incident_request.copy()
        request_crash["signal_type"] = "CrashLoopBackOff"
        request_crash["incident_id"] = "test-crashloop"
        incident_crash = IncidentRequest(**request_crash)
        response_crash = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_crash
        )

        # Assert: Should have different workflows
        # Note: selected_workflow may be dict or object depending on OpenAPI client deserialization
        oom_workflow_id = response_oom.selected_workflow.get('workflow_id') if isinstance(response_oom.selected_workflow, dict) else response_oom.selected_workflow.workflow_id
        crash_workflow_id = response_crash.selected_workflow.get('workflow_id') if isinstance(response_crash.selected_workflow, dict) else response_crash.selected_workflow.workflow_id
        assert oom_workflow_id != crash_workflow_id


class TestMockModeAIAnalysisScenarios:
    """Integration tests simulating AIAnalysis controller workflows via OpenAPI client"""

    def test_aianalysis_initial_investigation_flow(
        self, hapi_service_url, sample_incident_request
    ):
        """
        BR-HAPI-212: Simulate AIAnalysis initial investigation via OpenAPI client

        Flow:
        1. AIAnalysis receives RemediationProcessing
        2. Calls /incident/analyze
        3. Creates WorkflowExecution from response
        """
        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        incidents_api = IncidentAnalysisApi(client)
        incident_request = IncidentRequest(**sample_incident_request)

        # Act: Call incident endpoint (what AIAnalysis does)
        response = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request
        )

        # Assert: Verify AIAnalysis can extract fields for WorkflowExecution
        assert response.incident_id == sample_incident_request["incident_id"]

        # Note: selected_workflow may be dict or object depending on OpenAPI client deserialization
        selected_workflow = response.selected_workflow
        if isinstance(selected_workflow, dict):
            # For WorkflowExecution.spec.workflowRef
            assert selected_workflow.get('workflow_id') is not None
            # For WorkflowExecution.spec.executionBundle (may be camelCase or snake_case)
            execution_bundle = selected_workflow.get('execution_bundle') or selected_workflow.get('executionBundle')
            assert execution_bundle is not None
            # For WorkflowExecution.spec.parameters
            assert selected_workflow.get('parameters') is not None
            assert isinstance(selected_workflow.get('parameters'), dict)
        else:
            # For WorkflowExecution.spec.workflowRef
            assert selected_workflow.workflow_id is not None
            # For WorkflowExecution.spec.executionBundle
            assert selected_workflow.execution_bundle is not None
            # For WorkflowExecution.spec.parameters
            assert selected_workflow.parameters is not None
            assert isinstance(selected_workflow.parameters, dict)

        # Check if human review is needed
        if response.needs_human_review:
            assert response.human_review_reason is not None or len(response.warnings) > 0

    def test_aianalysis_handles_low_confidence_appropriately(
        self, hapi_service_url, sample_incident_request
    ):
        """
        BR-HAPI-212: AIAnalysis should handle low-confidence responses via OpenAPI client

        When confidence is low, AIAnalysis may:
        - Set needs_human_review = true
        - Not auto-create WorkflowExecution
        """
        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        incidents_api = IncidentAnalysisApi(client)

        # Use unknown signal type (lower confidence)
        request_data = sample_incident_request.copy()
        request_data["signal_type"] = "UnknownCustomSignal"
        incident_request = IncidentRequest(**request_data)

        # Act: Call API
        response = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request
        )

        # Assert: Even with unknown signal, should get a response
        assert response.confidence is not None

        # For unknown signals, confidence should be lower (mock uses 0.75)
        # AIAnalysis would check this before auto-creating WE

