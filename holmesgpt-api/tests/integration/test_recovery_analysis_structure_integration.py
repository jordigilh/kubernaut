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
Recovery Analysis Structure Integration Tests (HAPI Team)

REQUEST: docs/shared/REQUEST_HAPI_RECOVERYSTATUS_V1_0.md
Business Requirement: BR-AI-082 (RecoveryStatus Population)
Design Decision: DD-RECOVERY-003 (Recovery Analysis Response Structure)

PURPOSE:
--------
As the HAPI team, these tests verify that our /api/v1/recovery/analyze endpoint
returns the recovery_analysis structure that the AIAnalysis team needs to populate
the RecoveryStatus field in the AIAnalysis CRD status.

MAPPING TO AIANALYSIS CRD:
-------------------------
HAPI Field → AIAnalysis CRD Field:
  recovery_analysis.previous_attempt_assessment.failure_understood
    → status.recoveryStatus.previousAttemptAssessment.failureUnderstood

  recovery_analysis.previous_attempt_assessment.failure_reason_analysis
    → status.recoveryStatus.previousAttemptAssessment.failureReasonAnalysis

  recovery_analysis.previous_attempt_assessment.state_changed
    → status.recoveryStatus.stateChanged

  recovery_analysis.previous_attempt_assessment.current_signal_type
    → status.recoveryStatus.currentSignalType

TEST STRATEGY:
-------------
- Uses FastAPI TestClient for direct app testing (no external service needed)
- Real Data Storage integration via conftest infrastructure fixture
- Mock LLM mode for cost-free testing (BR-HAPI-212)
- Validates contract compliance with OpenAPI spec
"""

import pytest
import sys
import json
import os

# Import HAPI app directly for testing
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '../..'))
from fastapi.testclient import TestClient


# ========================================
# FIXTURES
# ========================================

@pytest.fixture(scope="session", autouse=True)
def setup_test_environment():
    """Setup test environment for HAPI integration tests."""
    # DD-TEST-001 v2.5: Use global LLM configuration from conftest.py
    # The conftest.py pytest_configure() already sets:
    #   LLM_MODEL="gpt-4-turbo"
    #   LLM_ENDPOINT="http://127.0.0.1:18140"
    # DO NOT override these - recovery tests need to call actual Mock LLM server

    # Only set config file path (must be before app import)
    test_dir = os.path.dirname(__file__)
    config_path = os.path.abspath(os.path.join(test_dir, "../../config.yaml"))
    os.environ["CONFIG_FILE"] = config_path

    # DATA_STORAGE_URL is also set globally, but allow override if needed
    if "DATA_STORAGE_URL" not in os.environ:
        os.environ["DATA_STORAGE_URL"] = "http://localhost:18098"

    print(f"\n🔧 Recovery Test Environment:")
    print(f"   CONFIG_FILE: {config_path}")
    print(f"   LLM_MODEL: {os.environ.get('LLM_MODEL', 'NOT SET')}")
    print(f"   LLM_ENDPOINT: {os.environ.get('LLM_ENDPOINT', 'NOT SET')}")
    print(f"   DATA_STORAGE_URL: {os.environ['DATA_STORAGE_URL']}")

    yield


@pytest.fixture
def hapi_client(integration_infrastructure):
    """Create FastAPI TestClient for HAPI testing."""
    # Import here AFTER environment is set up
    from src.main import app
    return TestClient(app)


@pytest.fixture
def sample_recovery_request():
    """Sample recovery request with previous execution context."""
    return {
        "incident_id": "test-recovery-001",
        "remediation_id": "req-test-001",
        "signal_type": "OOMKilled",
        "severity": "critical",
        "environment": "production",
        "priority": "P0",
        "is_recovery_attempt": True,
        "recovery_attempt_number": 1,
        "previous_execution": {
            "workflow_execution_ref": "we-test-001",
            "original_rca": {
                "summary": "Container exceeded memory limits",
                "signal_type": "OOMKilled",
                "severity": "critical"
            },
            "selected_workflow": {
                "workflow_id": "oom-memory-increase-v1",
                "version": "v1.0.0",
                "container_image": "quay.io/kubernaut/oom-fix:v1.0.0",
                "rationale": "Increase memory allocation to resolve OOM"
            },
            "failure": {
                "failed_step_index": 0,
                "failed_step_name": "validate-quota",
                "reason": "InsufficientMemory",
                "message": "Cannot increase memory beyond quota",
                "exit_code": 1,
                "failed_at": "2025-12-29T10:00:00Z",
                "execution_time": "5s"
            }
        }
    }


# ========================================
# TESTS
# ========================================

@pytest.mark.integration
class TestRecoveryAnalysisStructure:
    """
    Core tests validating recovery_analysis structure for AA team.

    These tests verify the 4 critical fields that AA team needs to populate
    RecoveryStatus in AIAnalysis CRD.
    """

    def test_recovery_analysis_field_present(self, hapi_client, sample_recovery_request):
        """HAPI Integration: recovery_analysis field is present in response."""
        # Act
        response = hapi_client.post("/api/v1/recovery/analyze", json=sample_recovery_request)

        # Assert
        assert response.status_code == 200, f"Expected 200, got {response.status_code}: {response.text}"
        data = response.json()

        assert 'recovery_analysis' in data, "recovery_analysis field must be present"
        assert data['recovery_analysis'] is not None, "recovery_analysis must not be null"

        print(f"\n✅ recovery_analysis present")

    def test_previous_attempt_assessment_structure(self, hapi_client, sample_recovery_request):
        """HAPI Integration: previous_attempt_assessment has all required fields."""
        # Act
        response = hapi_client.post("/api/v1/recovery/analyze", json=sample_recovery_request)
        data = response.json()

        # Assert: previous_attempt_assessment exists
        assert 'previous_attempt_assessment' in data['recovery_analysis'], \
            "recovery_analysis must contain previous_attempt_assessment"

        prev_assessment = data['recovery_analysis']['previous_attempt_assessment']
        assert prev_assessment is not None, "previous_attempt_assessment must not be null"

        # Assert: All 4 required fields present
        required_fields = [
            'failure_understood',
            'failure_reason_analysis',
            'state_changed',
            'current_signal_type'
        ]
        for field in required_fields:
            assert field in prev_assessment, f"previous_attempt_assessment must contain '{field}'"

        print(f"\n✅ previous_attempt_assessment structure validated")
        print(f"   Fields: {list(prev_assessment.keys())}")

    def test_field_types_correct(self, hapi_client, sample_recovery_request):
        """HAPI Integration: All 4 fields have correct types for AA team mapping."""
        # Act
        response = hapi_client.post("/api/v1/recovery/analyze", json=sample_recovery_request)
        data = response.json()
        prev_assessment = data['recovery_analysis']['previous_attempt_assessment']

        # Assert: Field type 1 - failure_understood (boolean)
        assert isinstance(prev_assessment['failure_understood'], bool), \
            f"failure_understood must be boolean, got {type(prev_assessment['failure_understood'])}"

        # Assert: Field type 2 - failure_reason_analysis (string)
        assert isinstance(prev_assessment['failure_reason_analysis'], str), \
            f"failure_reason_analysis must be string, got {type(prev_assessment['failure_reason_analysis'])}"
        assert len(prev_assessment['failure_reason_analysis']) > 0, \
            "failure_reason_analysis must not be empty"

        # Assert: Field type 3 - state_changed (boolean)
        assert isinstance(prev_assessment['state_changed'], bool), \
            f"state_changed must be boolean, got {type(prev_assessment['state_changed'])}"

        # Assert: Field type 4 - current_signal_type (string)
        assert isinstance(prev_assessment['current_signal_type'], str), \
            f"current_signal_type must be string, got {type(prev_assessment['current_signal_type'])}"
        assert len(prev_assessment['current_signal_type']) > 0, \
            "current_signal_type must not be empty"

        print(f"\n✅ All 4 field types validated")
        print(f"   failure_understood: {type(prev_assessment['failure_understood']).__name__} = {prev_assessment['failure_understood']}")
        print(f"   failure_reason_analysis: {type(prev_assessment['failure_reason_analysis']).__name__} ({len(prev_assessment['failure_reason_analysis'])} chars)")
        print(f"   state_changed: {type(prev_assessment['state_changed']).__name__} = {prev_assessment['state_changed']}")
        print(f"   current_signal_type: {type(prev_assessment['current_signal_type']).__name__} = {prev_assessment['current_signal_type']}")

    def test_mock_mode_returns_valid_structure(self, hapi_client, sample_recovery_request):
        """HAPI Integration: Mock mode (BR-HAPI-212) returns valid recovery_analysis."""
        # Act
        response = hapi_client.post("/api/v1/recovery/analyze", json=sample_recovery_request)
        data = response.json()

        # Assert: Mock mode still provides complete structure
        assert 'recovery_analysis' in data
        assert 'previous_attempt_assessment' in data['recovery_analysis']
        prev_assessment = data['recovery_analysis']['previous_attempt_assessment']

        # All 4 fields must be present and correctly typed
        assert isinstance(prev_assessment['failure_understood'], bool)
        assert isinstance(prev_assessment['failure_reason_analysis'], str)
        assert len(prev_assessment['failure_reason_analysis']) > 0
        assert isinstance(prev_assessment['state_changed'], bool)
        assert isinstance(prev_assessment['current_signal_type'], str)
        assert len(prev_assessment['current_signal_type']) > 0

        print(f"\n✅ Mock mode recovery_analysis validated (BR-HAPI-212)")

    def test_aa_team_integration_mapping(self, hapi_client, sample_recovery_request):
        """
        HAPI Integration: Response provides all fields for AA team's RecoveryStatus mapping.

        This validates the EXACT field mapping documented in REQUEST_HAPI_RECOVERYSTATUS_V1_0.md.
        The AA team performs this mapping in pkg/aianalysis/handlers/response_processor.go.
        """
        # Act
        response = hapi_client.post("/api/v1/recovery/analyze", json=sample_recovery_request)
        data = response.json()
        prev_assessment = data['recovery_analysis']['previous_attempt_assessment']

        # Validate EXACT mapping per REQUEST document
        # MAPPING 1: failure_understood → previousAttemptAssessment.failureUnderstood
        failure_understood = prev_assessment['failure_understood']
        assert isinstance(failure_understood, bool)

        # MAPPING 2: failure_reason_analysis → previousAttemptAssessment.failureReasonAnalysis
        failure_reason = prev_assessment['failure_reason_analysis']
        assert isinstance(failure_reason, str) and len(failure_reason) > 0

        # MAPPING 3: state_changed → stateChanged
        state_changed = prev_assessment['state_changed']
        assert isinstance(state_changed, bool)

        # MAPPING 4: current_signal_type → currentSignalType
        current_signal = prev_assessment['current_signal_type']
        assert isinstance(current_signal, str) and len(current_signal) > 0

        print(f"\n🎯 AA TEAM INTEGRATION READY:")
        print(f"   ✅ MAPPING 1: failure_understood (bool)")
        print(f"   ✅ MAPPING 2: failure_reason_analysis (string)")
        print(f"   ✅ MAPPING 3: state_changed (bool)")
        print(f"   ✅ MAPPING 4: current_signal_type (string)")
        print(f"   All 4 RecoveryStatus mappings validated")

    def test_multiple_recovery_attempts(self, hapi_client, sample_recovery_request):
        """HAPI Integration: Multiple recovery attempts all return valid structure."""
        for attempt_num in [1, 2, 3]:
            # Arrange
            request = sample_recovery_request.copy()
            request['recovery_attempt_number'] = attempt_num
            request['incident_id'] = f"test-attempt-{attempt_num}"

            # Act
            response = hapi_client.post("/api/v1/recovery/analyze", json=request)
            data = response.json()

            # Assert: Each attempt gets valid recovery_analysis
            assert 'recovery_analysis' in data, f"Attempt {attempt_num} must have recovery_analysis"
            assert 'previous_attempt_assessment' in data['recovery_analysis'], \
                f"Attempt {attempt_num} must have previous_attempt_assessment"

            print(f"✅ Attempt {attempt_num} validated")
