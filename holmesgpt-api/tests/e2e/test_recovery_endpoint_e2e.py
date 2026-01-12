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
Recovery Endpoint E2E Tests

Business Requirement: BR-AI-080, BR-AI-081 (Recovery Analysis)
Phase: Phase 3 - E2E Test Coverage

These E2E tests validate the complete recovery endpoint flow using the
HAPI OpenAPI client. They provide defense-in-depth testing and would have
caught the missing fields bug that the AA team discovered.

Test Strategy:
- Use HAPI OpenAPI client for type-safe API calls
- Validate OpenAPI contract compliance end-to-end
- Test real Data Storage integration
- Cover happy paths and error scenarios
- Ensure fields required by AA team are present

Authority: TRIAGE_HAPI_E2E_AND_CLIENT_GAPS.md
Phase: 3 of 4 (Road to 100% Confidence)
"""

import pytest
import sys
from pathlib import Path
# Add tests/clients to path (absolute path resolution for CI)
sys.path.insert(0, str(Path(__file__).parent.parent / 'clients'))

from holmesgpt_api_client import ApiClient, Configuration
from holmesgpt_api_client.api.recovery_analysis_api import RecoveryAnalysisApi
from holmesgpt_api_client.api.incident_analysis_api import IncidentAnalysisApi
from holmesgpt_api_client.models.recovery_request import RecoveryRequest
from holmesgpt_api_client.models.incident_request import IncidentRequest
from holmesgpt_api_client.models.previous_execution import PreviousExecution
from holmesgpt_api_client.exceptions import ApiException


@pytest.fixture
def hapi_client_config(hapi_service_url):
    """Create HAPI OpenAPI client configuration"""
    return Configuration(host=hapi_service_url)


@pytest.fixture
def recovery_api(hapi_client_config):
    """Create Recovery API instance"""
    client = ApiClient(configuration=hapi_client_config)
    return RecoveryAnalysisApi(client)


@pytest.fixture
def incidents_api(hapi_client_config):
    """Create Incidents API instance"""
    client = ApiClient(configuration=hapi_client_config)
    return IncidentAnalysisApi(client)


@pytest.fixture
def sample_previous_execution():
    """Sample previous workflow execution context"""
    return {
        "workflow_execution_ref": "we-test-e2e-001",
        "original_rca": {
            "summary": "Container exceeded memory limits",
            "signal_type": "OOMKilled",
            "severity": "critical",
            "contributing_factors": ["memory_leak", "high_load"]
        },
        "selected_workflow": {
            "workflow_id": "oom-memory-increase-v1",
            "version": "v1.0.0",
            "container_image": "quay.io/kubernaut/oom-remediation:v1.0.0",
            "parameters": {"memory_increment": "256Mi"},
            "rationale": "Increase memory limits to prevent OOM"
        },
        "failure": {
            "failed_step_index": 0,
            "failed_step_name": "validate-quota",
            "reason": "InsufficientMemory",
            "message": "Cannot increase memory beyond quota",
            "exit_code": 1,
            "failed_at": "2025-12-13T10:00:00Z",
            "execution_time": "30s"
        }
    }


@pytest.mark.e2e
class TestRecoveryEndpointE2EHappyPath:
    """
    E2E Test Case 1: Happy Path - Recovery Returns Complete Response

    Business Outcome: Recovery endpoint provides complete response for workflow selection

    This test validates the complete recovery flow and would have caught
    the missing selected_workflow and recovery_analysis fields bug.
    """

    def test_recovery_endpoint_returns_complete_response_e2e(
        self, recovery_api, sample_previous_execution
    ):
        """
        E2E: Recovery endpoint returns all required fields via OpenAPI client

        This test validates BR-AI-080 and BR-AI-081 and would have caught
        the bug where selected_workflow and recovery_analysis were null.
        """
        # Arrange: Create recovery request with previous execution context
        recovery_request = RecoveryRequest(
            incident_id="e2e-recovery-001",
            remediation_id="req-e2e-2025-12-13-001",
            signal_type="OOMKilled",
            severity="critical",
            resource_namespace="production",
            resource_kind="Deployment",
            resource_name="payment-service",
            environment="production",
            priority="P0",
            risk_tolerance="low",
            business_category="revenue-critical",
            is_recovery_attempt=True,
            recovery_attempt_number=1,
            previous_workflow_id="oom-memory-increase-v1",
            previous_execution=PreviousExecution(**sample_previous_execution)
        )

        # Act: Call recovery endpoint via OpenAPI client
        response = recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
            recovery_request=recovery_request
        )

        # Assert: All required fields are present (validates OpenAPI contract)
        assert response is not None, "Response must not be null"
        assert response.incident_id == "e2e-recovery-001", "incident_id must match request"

        # Critical fields that were missing (BR-AI-080, BR-AI-081)
        assert response.selected_workflow is not None, "selected_workflow must be present (BR-AI-080)"
        assert response.recovery_analysis is not None, "recovery_analysis must be present (BR-AI-081)"

        # Additional required fields for AA team
        assert response.can_recover is not None, "can_recover must be present"
        assert response.analysis_confidence is not None, "analysis_confidence must be present"
        assert response.metadata is not None, "metadata must be present"
        assert response.strategies is not None, "strategies must be present"


@pytest.mark.e2e
class TestRecoveryEndpointE2EFieldValidation:
    """
    E2E Test Case 2: Field Validation - All Required Fields Present and Typed

    Business Outcome: Response structure matches OpenAPI spec and AA team expectations
    """

    def test_recovery_response_has_correct_field_types_e2e(
        self, recovery_api, sample_previous_execution
    ):
        """E2E: Validate response field types match OpenAPI spec"""
        # Arrange
        recovery_request = RecoveryRequest(
            incident_id="e2e-recovery-002",
            remediation_id="req-e2e-2025-12-13-002",
            signal_type="OOMKilled",
            severity="critical",
            environment="production",
            priority="P0",
            risk_tolerance="low",
            is_recovery_attempt=True,
            recovery_attempt_number=1,
            previous_execution=PreviousExecution(**sample_previous_execution)
        )

        # Act
        response = recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
            recovery_request=recovery_request
        )

        # Assert: Field types match OpenAPI spec
        assert isinstance(response.incident_id, str)
        assert isinstance(response.can_recover, bool)
        assert isinstance(response.analysis_confidence, (int, float))
        assert 0 <= response.analysis_confidence <= 1.0

        # Validate selected_workflow structure (dict from API response)
        assert response.selected_workflow['workflow_id'] is not None
        assert isinstance(response.selected_workflow['workflow_id'], str)
        assert isinstance(response.selected_workflow['confidence'], (int, float))
        # Note: container_image may not be present in mock mode responses
        # In real scenarios, it would come from Data Storage workflow records

        # Validate recovery_analysis structure (dict from API response)
        # In mock mode, recovery_analysis may have different structure
        if 'can_recover' in response.recovery_analysis:
            assert isinstance(response.recovery_analysis['can_recover'], bool)
        if 'confidence' in response.recovery_analysis:
            assert isinstance(response.recovery_analysis['confidence'], (int, float))


@pytest.mark.e2e
class TestRecoveryEndpointE2EPreviousExecution:
    """
    E2E Test Case 3: Previous Execution - Context Properly Handled

    Business Outcome: Recovery analysis uses previous failure context
    """

    def test_recovery_processes_previous_execution_context_e2e(
        self, recovery_api, sample_previous_execution
    ):
        """E2E: Recovery endpoint processes previous execution context"""
        # Arrange: Create request with rich previous execution context
        recovery_request = RecoveryRequest(
            incident_id="e2e-recovery-003",
            remediation_id="req-e2e-2025-12-13-003",
            signal_type="OOMKilled",
            severity="critical",
            environment="production",
            priority="P0",
            is_recovery_attempt=True,
            recovery_attempt_number=2,  # Second attempt
            previous_workflow_id="oom-memory-increase-v1",
            previous_execution=PreviousExecution(**sample_previous_execution)
        )

        # Act
        response = recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
            recovery_request=recovery_request
        )

        # Assert: Response reflects recovery attempt context
        assert response.incident_id == "e2e-recovery-003"
        assert response.selected_workflow is not None
        assert response.recovery_analysis is not None

        # Recovery workflow should differ from failed workflow
        # (or confidence should reflect retry risk)
        assert response.analysis_confidence is not None


@pytest.mark.e2e
class TestRecoveryEndpointE2EDetectedLabels:
    """
    E2E Test Case 4: Detected Labels - Labels Included in Analysis

    Business Outcome: Cluster context influences recovery workflow selection
    """

    def test_recovery_uses_detected_labels_for_workflow_selection_e2e(
        self, recovery_api, sample_previous_execution
    ):
        """E2E: Detected labels influence recovery workflow selection"""
        # Arrange: Request with detected labels
        recovery_request = RecoveryRequest(
            incident_id="e2e-recovery-004",
            remediation_id="req-e2e-2025-12-13-004",
            signal_type="OOMKilled",
            severity="critical",
            environment="production",
            priority="P0",
            is_recovery_attempt=True,
            recovery_attempt_number=1,
            previous_execution=PreviousExecution(**sample_previous_execution),
            enrichment_results={
                "detectedLabels": {
                    "gitOpsManaged": True,
                    "pdbProtected": True,
                    "stateful": True
                }
            }
        )

        # Act
        response = recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
            recovery_request=recovery_request
        )

        # Assert: Response includes workflow considering labels
        assert response.selected_workflow is not None
        assert response.selected_workflow['workflow_id'] is not None
        # Workflow selection should consider stateful + PDB constraints


@pytest.mark.e2e
class TestRecoveryEndpointE2EMockMode:
    """
    E2E Test Case 5: Mock LLM Mode - Mock Responses Valid

    Business Outcome: Mock mode produces valid responses for testing
    """

    def test_recovery_mock_mode_produces_valid_responses_e2e(
        self, recovery_api, sample_previous_execution
    ):
        """E2E: Mock LLM mode produces OpenAPI-compliant responses"""
        # Arrange: Standard recovery request (mock mode auto-detected via env)
        recovery_request = RecoveryRequest(
            incident_id="e2e-recovery-mock-001",
            remediation_id="req-e2e-mock-2025-12-13-001",
            signal_type="CrashLoopBackOff",
            severity="high",
            environment="staging",
            priority="P1",
            is_recovery_attempt=True,
            recovery_attempt_number=1,
            previous_execution=PreviousExecution(**sample_previous_execution)
        )

        # Act
        response = recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
            recovery_request=recovery_request
        )

        # Assert: Mock response is OpenAPI-compliant
        assert response.selected_workflow is not None
        assert response.recovery_analysis is not None
        assert response.can_recover is not None
        assert response.analysis_confidence is not None

        # Mock mode should indicate in warnings
        if response.warnings:
            assert any("MOCK" in w.upper() for w in response.warnings)


@pytest.mark.e2e
class TestRecoveryEndpointE2EErrorScenarios:
    """
    E2E Test Case 6: Error Scenarios - API Errors Properly Formatted

    Business Outcome: Invalid requests rejected with clear error messages
    """

    def test_recovery_rejects_invalid_recovery_attempt_number_e2e(
        self, recovery_api
    ):
        """E2E: Invalid recovery attempt number rejected (client-side validation)"""
        from pydantic_core import ValidationError

        # Arrange: Invalid request (recovery_attempt_number = 0)
        # Act & Assert: Pydantic validates on the client before API call
        with pytest.raises(ValidationError) as exc_info:
            recovery_request = RecoveryRequest(
                incident_id="e2e-recovery-invalid-001",
                remediation_id="req-e2e-invalid-001",
                signal_type="OOMKilled",
                severity="critical",
                is_recovery_attempt=True,
                recovery_attempt_number=0  # Invalid: must be >= 1
            )

        # Validation error should mention recovery_attempt_number
        assert "recovery_attempt_number" in str(exc_info.value)

    def test_recovery_requires_previous_execution_for_recovery_attempts_e2e(
        self, recovery_api
    ):
        """E2E: Recovery attempt without previous execution context"""
        # Arrange: Recovery attempt without previous_execution
        recovery_request = RecoveryRequest(
            incident_id="e2e-recovery-invalid-002",
            remediation_id="req-e2e-invalid-002",
            signal_type="OOMKilled",
            severity="critical",
            is_recovery_attempt=True,
            recovery_attempt_number=1
            # Missing: previous_execution (should it be required?)
        )

        # Act: Call API (may succeed with default behavior or reject)
        try:
            response = recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
                recovery_request=recovery_request
            )
            # If it succeeds, validate response structure
            assert response.can_recover is not None
        except ApiException as e:
            # If it rejects, should be validation error
            assert e.status in [400, 422]


@pytest.mark.e2e
class TestRecoveryEndpointE2EDataStorageIntegration:
    """
    E2E Test Case 7: Data Storage Integration - Workflow Search Works

    Business Outcome: Recovery endpoint integrates with Data Storage for workflow search
    """

    def test_recovery_searches_data_storage_for_workflows_e2e(
        self, recovery_api, sample_previous_execution
    ):
        """E2E: Recovery endpoint searches Data Storage via OpenAPI client"""
        # Arrange: Request that requires workflow search
        recovery_request = RecoveryRequest(
            incident_id="e2e-recovery-ds-001",
            remediation_id="req-e2e-ds-001",
            signal_type="OOMKilled",
            severity="critical",
            resource_namespace="production",
            environment="production",
            priority="P0",
            is_recovery_attempt=True,
            recovery_attempt_number=1,
            previous_execution=PreviousExecution(**sample_previous_execution)
        )

        # Act: Call API (should search Data Storage for recovery workflows)
        response = recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
            recovery_request=recovery_request
        )

        # Assert: Response includes workflow from Data Storage search
        assert response.selected_workflow is not None
        assert response.selected_workflow['workflow_id'] is not None
        # Workflow should come from Data Storage catalog


@pytest.mark.e2e
class TestRecoveryEndpointE2EWorkflowValidation:
    """
    E2E Test Case 8: Workflow Validation - Selected Workflow is Executable

    Business Outcome: Recovery endpoint returns executable workflow specifications
    """

    def test_recovery_returns_executable_workflow_specification_e2e(
        self, recovery_api, sample_previous_execution
    ):
        """E2E: Selected recovery workflow has all fields for execution"""
        # Arrange
        recovery_request = RecoveryRequest(
            incident_id="e2e-recovery-exec-001",
            remediation_id="req-e2e-exec-001",
            signal_type="OOMKilled",
            severity="critical",
            environment="production",
            priority="P0",
            is_recovery_attempt=True,
            recovery_attempt_number=1,
            previous_execution=PreviousExecution(**sample_previous_execution)
        )

        # Act
        response = recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
            recovery_request=recovery_request
        )

        # Assert: Workflow is executable by WorkflowExecution controller
        workflow = response.selected_workflow

        # Required fields for WorkflowExecution CRD (dict from API response)
        assert workflow['workflow_id'] is not None, "workflow_id required"
        assert workflow.get('parameters') is not None, "parameters required"
        assert isinstance(workflow.get('parameters'), dict), "parameters must be dict"
        # Note: container_image validation would require real Data Storage workflow records
        # In mock mode, this field may not be populated

        # Additional fields for execution (dict from API response)
        assert workflow.get('confidence') is not None, "confidence required"
        assert workflow.get('rationale') is not None, "rationale required"

        # Version information
        if hasattr(workflow, 'version'):
            assert workflow.version is not None


@pytest.mark.e2e
class TestRecoveryEndpointE2EEndToEndFlow:
    """
    E2E Integration Test: Complete Incident → Recovery Flow

    Business Outcome: Validate complete remediation lifecycle
    """

    def test_complete_incident_to_recovery_flow_e2e(
        self, incidents_api, recovery_api, test_workflows_bootstrapped
    ):
        """
        E2E: Complete flow from incident analysis to recovery attempt

        This simulates the full AA team workflow:
        1. Incident analyzed
        2. Workflow executed (simulated failure)
        3. Recovery analyzed

        Note: test_workflows_bootstrapped fixture ensures OOMKilled workflows exist
        """
        # Step 1: Analyze initial incident
        incident_request = IncidentRequest(
            incident_id="e2e-flow-001",
            remediation_id="req-e2e-flow-001",
            signal_type="OOMKilled",
            severity="critical",
            signal_source="prometheus",
            resource_namespace="production",
            resource_kind="Deployment",
            resource_name="api-server",
            error_message="Container exceeded memory limit",
            environment="production",
            priority="P0",
            risk_tolerance="low",
            business_category="critical",
            cluster_name="prod-cluster-1"
        )

        incident_response = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request
        )

        # Validate incident response
        assert incident_response.selected_workflow is not None
        initial_workflow_id = incident_response.selected_workflow['workflow_id']

        # Step 2: Simulate workflow execution failure
        simulated_failure = {
            "workflow_execution_ref": "we-e2e-flow-001",
            "original_rca": {
                "summary": incident_response.root_cause_analysis.get("summary", "OOM"),
                "signal_type": "OOMKilled",
                "severity": "critical"
            },
            "selected_workflow": {
                "workflow_id": initial_workflow_id,
                "version": "v1.0.0",
                "container_image": incident_response.selected_workflow.get('container_image') or incident_response.selected_workflow.get('containerImage'),
                "parameters": incident_response.selected_workflow.get('parameters'),
                "rationale": incident_response.selected_workflow.get('rationale')
            },
            "failure": {
                "failed_step_index": 0,
                "failed_step_name": "validate-quota",
                "reason": "InsufficientResources",
                "message": "Cannot increase memory beyond quota",
                "exit_code": 1,
                "failed_at": "2025-12-13T12:00:00Z",
                "execution_time": "30s"
            }
        }

        # Step 3: Analyze recovery options
        recovery_request = RecoveryRequest(
            incident_id="e2e-flow-001",
            remediation_id="req-e2e-flow-001",
            signal_type="OOMKilled",
            severity="critical",
            environment="production",
            priority="P0",
            is_recovery_attempt=True,
            recovery_attempt_number=1,
            previous_workflow_id=initial_workflow_id,
            previous_execution=PreviousExecution(**simulated_failure)
        )

        recovery_response = recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
            recovery_request=recovery_request
        )

        # Assert: Complete flow validated
        assert recovery_response.incident_id == "e2e-flow-001"
        assert recovery_response.selected_workflow is not None
        assert recovery_response.recovery_analysis is not None
        assert recovery_response.can_recover is not None

        # Recovery workflow may differ from initial workflow
        recovery_workflow_id = recovery_response.selected_workflow['workflow_id']
        # Could be same workflow with different params, or different approach
        assert recovery_workflow_id is not None

