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
E2E Tests for Mock LLM Edge Cases

Business Requirement: BR-HAPI-212 - Mock LLM Mode for Integration Testing
Design Decision: DD-HAPI-002 - Workflow Response Validation

These tests validate that HAPI correctly handles non-happy-path scenarios
using deterministic mock responses. The edge cases tested here ensure that
downstream consumers (AIAnalysis) can handle:

1. No workflow found → needs_human_review=true
2. Low confidence → needs_human_review=true
3. Signal not reproducible → can_recover=false
4. Max retries exhausted → needs_human_review=true with validation history

Edge cases are triggered by special signal_type values:
- MOCK_NO_WORKFLOW_FOUND
- MOCK_LOW_CONFIDENCE
- MOCK_NOT_REPRODUCIBLE
- MOCK_MAX_RETRIES_EXHAUSTED
"""

import os
import pytest

# HAPI OpenAPI Client imports (DD-API-001)
from holmesgpt_api_client import ApiClient as HAPIApiClient, Configuration as HAPIConfiguration
from holmesgpt_api_client.api.incident_analysis_api import IncidentAnalysisApi
from holmesgpt_api_client.api.recovery_analysis_api import RecoveryAnalysisApi
from holmesgpt_api_client.models.incident_request import IncidentRequest
from holmesgpt_api_client.models.recovery_request import RecoveryRequest

# Skip entire module if not in mock mode
pytestmark = [
    pytest.mark.e2e,
    pytest.mark.mock_llm,
    # Mock LLM tests run by default in E2E (Mock LLM service is part of E2E infrastructure)
    # Real LLM tests require explicit opt-in via RUN_REAL_LLM=true
]


@pytest.fixture(scope="module")
def hapi_incident_api(hapi_service_url, hapi_auth_token):
    """HAPI Incident Analysis API client for E2E tests (DD-API-001)."""
    config = HAPIConfiguration(host=hapi_service_url)
    config.timeout = 60  # CRITICAL: Prevent "read timeout=0" errors
    # WORKAROUND: OpenAPI spec doesn't apply security to endpoints
    # Manually inject Authorization header via default_headers
    api_client = HAPIApiClient(configuration=config)
    # DD-AUTH-014: Auth ALWAYS enabled in E2E/INT for ALL services
    api_client.default_headers['Authorization'] = f'Bearer {hapi_auth_token}'
    return IncidentAnalysisApi(api_client=api_client)


@pytest.fixture(scope="module")
def hapi_recovery_api(hapi_service_url, hapi_auth_token):
    """HAPI Recovery Analysis API client for E2E tests (DD-API-001)."""
    config = HAPIConfiguration(host=hapi_service_url)
    config.timeout = 60  # CRITICAL: Prevent "read timeout=0" errors
    # WORKAROUND: OpenAPI spec doesn't apply security to endpoints
    # Manually inject Authorization header via default_headers
    api_client = HAPIApiClient(configuration=config)
    # DD-AUTH-014: Auth ALWAYS enabled in E2E/INT for ALL services
    api_client.default_headers['Authorization'] = f'Bearer {hapi_auth_token}'
    return RecoveryAnalysisApi(api_client=api_client)


def make_incident_request(signal_type: str) -> dict:
    """Create a valid IncidentRequest with the specified signal_type."""
    return {
        "incident_id": f"test-edge-case-{signal_type.lower()}",
        "remediation_id": f"test-remediation-{signal_type.lower()}",  # REQUIRED per DD-WORKFLOW-002
        "signal_type": signal_type,
        "severity": "high",  # REQUIRED
        "signal_source": "prometheus",  # REQUIRED
        "resource_kind": "Pod",
        "resource_name": "test-pod",
        "resource_namespace": "default",
        "cluster_name": "e2e-test",  # REQUIRED
        "environment": "testing",  # REQUIRED
        "priority": "P2",  # REQUIRED
        "risk_tolerance": "medium",  # REQUIRED
        "business_category": "test",  # REQUIRED
        "error_message": f"Test edge case for {signal_type}",  # REQUIRED
    }


def make_recovery_request(signal_type: str) -> dict:
    """Create a valid RecoveryRequest with the specified signal_type."""
    return {
        "incident_id": f"test-recovery-{signal_type.lower()}",
        "remediation_id": f"test-remediation-{signal_type.lower()}",
        "signal_type": signal_type,
        "previous_workflow_id": "mock-previous-workflow-v1",
        "previous_workflow_result": "Failed",
        "resource_namespace": "default",
        "resource_name": "test-pod",
        "resource_kind": "Pod"
    }


class TestIncidentEdgeCases:
    """E2E tests for incident analysis edge cases."""

    def test_no_workflow_found_returns_needs_human_review(self, hapi_incident_api):
        """
        BR-HAPI-197: When no matching workflow is found, HAPI should:
        - Set needs_human_review=true
        - Set human_review_reason="no_matching_workflows"
        - Set selected_workflow=null

        This tests the AIAnalysis consumer's ability to handle the
        "no automation possible" scenario.
        """
        request_data = make_incident_request("MOCK_NO_WORKFLOW_FOUND")
        incident_request = IncidentRequest(**request_data)

        response = hapi_incident_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request,
            _request_timeout=30
        )

        # Convert response model to dict for assertions
        data = response.model_dump()

        # Verify edge case response
        assert data["needs_human_review"] is True, "needs_human_review should be True"
        assert data["human_review_reason"] == "no_matching_workflows", \
            f"Expected 'no_matching_workflows', got '{data.get('human_review_reason')}'"
        assert data["selected_workflow"] is None, "selected_workflow should be None"
        assert data["confidence"] == 0.0, "Confidence should be 0 when no workflow"

        # Verify warnings indicate mock mode
        assert any("MOCK_MODE" in w for w in data.get("warnings", [])), \
            "Response should include MOCK_MODE warning"

    def test_low_confidence_returns_needs_human_review(self, hapi_incident_api):
        """
        BR-HAPI-197: When analysis confidence is below threshold, HAPI should:
        - Set needs_human_review=true
        - Set human_review_reason="low_confidence"
        - Still provide a tentative selected_workflow
        - Include alternative_workflows for human selection

        This tests the AIAnalysis consumer's ability to handle
        "uncertain" recommendations.
        """
        request_data = make_incident_request("MOCK_LOW_CONFIDENCE")
        incident_request = IncidentRequest(**request_data)

        response = hapi_incident_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request,
            _request_timeout=30
        )

        # Convert response model to dict for assertions
        data = response.model_dump()

        # Verify edge case response
        assert data["needs_human_review"] is True, "needs_human_review should be True"
        assert data["human_review_reason"] == "low_confidence", \
            f"Expected 'low_confidence', got '{data.get('human_review_reason')}'"

        # Tentative workflow provided
        assert data["selected_workflow"] is not None, "Should have tentative workflow"
        assert data["selected_workflow"]["confidence"] < 0.5, \
            f"Confidence should be below threshold, got {data['selected_workflow']['confidence']}"

        # Alternatives for human decision
        assert len(data.get("alternative_workflows", [])) > 0, \
            "Should include alternatives for human selection"

    def test_max_retries_exhausted_returns_validation_history(self, hapi_incident_api):
        """
        BR-HAPI-197: When LLM self-correction exhausts max retries, HAPI should:
        - Set needs_human_review=true
        - Set human_review_reason="llm_parsing_error"
        - Include validation_attempts_history with all failed attempts
        - Set selected_workflow=null (no valid workflow found)

        This tests the AIAnalysis consumer's ability to handle
        "AI gave up" scenarios with audit trail.
        """
        request_data = make_incident_request("MOCK_MAX_RETRIES_EXHAUSTED")
        incident_request = IncidentRequest(**request_data)

        response = hapi_incident_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request,
            _request_timeout=30
        )

        # Convert response model to dict for assertions
        data = response.model_dump()

        # Verify edge case response
        assert data["needs_human_review"] is True, "needs_human_review should be True"
        assert data["human_review_reason"] == "llm_parsing_error", \
            f"Expected 'llm_parsing_error', got '{data.get('human_review_reason')}'"
        assert data["selected_workflow"] is None, "selected_workflow should be None after max retries"

        # Verify validation history is provided for debugging
        history = data.get("validation_attempts_history", [])
        assert len(history) >= 3, f"Should have at least 3 attempts, got {len(history)}"

        # Each attempt should have expected fields (per ValidationAttempt model)
        for attempt in history:
            assert "attempt" in attempt, "Missing attempt (was: attempt_number)"
            assert "workflow_id" in attempt, "Missing workflow_id"
            assert "is_valid" in attempt, "Missing is_valid (was: validation_passed)"
            assert "errors" in attempt, "Missing errors field"
            assert "timestamp" in attempt, "Missing timestamp field"
            assert attempt["is_valid"] is False, "All attempts should have failed"


class TestRecoveryEdgeCases:
    """E2E tests for recovery analysis edge cases."""

    def test_signal_not_reproducible_returns_no_recovery(self, hapi_recovery_api):
        """
        BR-HAPI-212: When signal is not reproducible (issue self-resolved):
        - Set can_recover=false (no action needed)
        - Set needs_human_review=false (no decision needed)
        - Set selected_workflow=null (no workflow to run)
        - High confidence that issue resolved

        This tests the AIAnalysis consumer's ability to handle
        "nothing to do" scenarios gracefully.
        """
        request_data = make_recovery_request("MOCK_NOT_REPRODUCIBLE")
        recovery_request = RecoveryRequest(**request_data)

        response = hapi_recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
            recovery_request=recovery_request,
            _request_timeout=30
        )

        # Convert response model to dict for assertions
        data = response.model_dump()

        # Key assertion: can_recover=false means no action needed
        assert data["can_recover"] is False, "can_recover should be False for self-resolved issues"
        assert data["needs_human_review"] is False, "No review needed when issue resolved"
        assert data["selected_workflow"] is None, "No workflow needed"

        # High confidence in the assessment
        assert data["analysis_confidence"] > 0.8, \
            f"Should have high confidence issue resolved, got {data['analysis_confidence']}"

        # Verify recovery_analysis indicates state changed
        recovery_analysis = data.get("recovery_analysis", {})
        prev_assessment = recovery_analysis.get("previous_attempt_assessment", {})
        assert prev_assessment.get("state_changed") is True, \
            "state_changed should be True (resource is now healthy)"

    def test_no_recovery_workflow_returns_human_review(self, hapi_recovery_api):
        """
        BR-HAPI-197: When no recovery workflow is available:
        - Set can_recover=true (recovery might be possible)
        - Set needs_human_review=true (human must find solution)
        - Set human_review_reason="no_matching_workflows"
        - Set selected_workflow=null

        This tests handling of "we can't help automatically" scenarios.
        """
        request_data = make_recovery_request("MOCK_NO_WORKFLOW_FOUND")
        recovery_request = RecoveryRequest(**request_data)

        response = hapi_recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
            recovery_request=recovery_request,
            _request_timeout=30
        )

        # Convert response model to dict for assertions
        data = response.model_dump()

        # Recovery might be possible but we don't have a workflow
        assert data["can_recover"] is True, "can_recover=true (manual intervention possible)"
        assert data["needs_human_review"] is True, "needs_human_review should be True"
        assert data["human_review_reason"] == "no_matching_workflows"
        assert data["selected_workflow"] is None

    def test_low_confidence_recovery_returns_human_review(self, hapi_recovery_api):
        """
        BR-HAPI-197: When recovery confidence is low:
        - Set can_recover=true
        - Set needs_human_review=true
        - Set human_review_reason="low_confidence"
        - Provide tentative workflow with low confidence

        This tests handling of uncertain recovery scenarios.
        """
        request_data = make_recovery_request("MOCK_LOW_CONFIDENCE")
        recovery_request = RecoveryRequest(**request_data)

        response = hapi_recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
            recovery_request=recovery_request,
            _request_timeout=30
        )

        # Convert response model to dict for assertions
        data = response.model_dump()

        assert data["can_recover"] is True
        assert data["needs_human_review"] is True
        assert data["human_review_reason"] == "low_confidence"
        assert data["analysis_confidence"] < 0.5

        # Tentative workflow provided
        assert data["selected_workflow"] is not None
        assert data["selected_workflow"]["confidence"] < 0.5


class TestHappyPathComparison:
    """Verify happy path still works with mock mode."""

    def test_normal_incident_analysis_succeeds(self, hapi_incident_api):
        """
        Verify that normal signal types still produce happy-path responses
        even when edge case support is enabled.
        """
        request_data = make_incident_request("OOMKilled")
        incident_request = IncidentRequest(**request_data)

        response = hapi_incident_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request,
            _request_timeout=30
        )

        # Convert response model to dict for assertions
        data = response.model_dump()

        # Happy path assertions
        assert data["needs_human_review"] is False
        assert data["selected_workflow"] is not None
        assert data["confidence"] > 0.8
        assert "mock-oomkill" in data["selected_workflow"]["workflow_id"]

    def test_normal_recovery_analysis_succeeds(self, hapi_recovery_api):
        """
        Verify that normal signal types still produce happy-path recovery responses.
        """
        request_data = make_recovery_request("CrashLoopBackOff")
        recovery_request = RecoveryRequest(**request_data)

        response = hapi_recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
            recovery_request=recovery_request,
            _request_timeout=30
        )

        # Convert response model to dict for assertions
        data = response.model_dump()

        # Happy path assertions
        assert data["can_recover"] is True
        assert data["needs_human_review"] is False
        assert data["selected_workflow"] is not None
        assert data["analysis_confidence"] > 0.7

