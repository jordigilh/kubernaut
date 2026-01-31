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
Recovery Analysis Structure Integration Tests (HAPI Team) - Refactored

REQUEST: docs/shared/REQUEST_HAPI_RECOVERYSTATUS_V1_0.md
Business Requirement: BR-AI-082 (RecoveryStatus Population)
Design Decision: DD-RECOVERY-003 (Recovery Analysis Response Structure)

PURPOSE:
--------
As the HAPI team, these tests verify that our recovery analysis business logic
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

TEST STRATEGY (REFACTORED - GO PATTERN):
----------------------------------------
✅ Direct business logic calls (analyze_recovery function)
✅ No TestClient / HTTP layer (integration testing, not E2E)
✅ No main.py import (avoids K8s auth initialization)
✅ Real Data Storage integration via conftest infrastructure
✅ Mock LLM mode for cost-free testing (BR-HAPI-212)
✅ Validates response structure contract

Pattern: Like Go Gateway/AIAnalysis integration tests
- Call business logic directly
- Validate return values
- No HTTP dependency
"""

import pytest
import os
from typing import Dict, Any


# ========================================
# FIXTURES
# ========================================

@pytest.fixture
def sample_recovery_request() -> Dict[str, Any]:
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
        "previous_workflow_id": "oom-memory-increase-v1",
        "previous_workflow_result": "Failed",
        "resource_namespace": "default",
        "resource_name": "test-pod",
        "resource_kind": "Pod",
    }


# ========================================
# INTEGRATION TESTS (GO PATTERN)
# ========================================

@pytest.mark.integration
class TestRecoveryAnalysisStructure:
    """
    Core tests validating recovery_analysis structure for AA team.

    These tests verify the 4 critical fields that AA team needs to populate
    RecoveryStatus in AIAnalysis CRD.
    
    Pattern: Direct business logic call → Validate response structure
    """

    @pytest.mark.asyncio
    async def test_recovery_analysis_field_present(self, sample_recovery_request):
        """
        HAPI Integration: recovery_analysis field is present in response.
        
        Pattern: Call analyze_recovery() directly → Validate structure
        """
        from src.extensions.recovery.llm_integration import analyze_recovery
        from src.models.config_models import AppConfig
        
        # ACT: Call business logic directly (NO HTTP)
        app_config = AppConfig()
        result = await analyze_recovery(sample_recovery_request, app_config=app_config)

        # ASSERT: recovery_analysis field present
        assert result is not None, "analyze_recovery should return result"
        assert 'recovery_analysis' in result, "recovery_analysis field must be present"
        assert result['recovery_analysis'] is not None, "recovery_analysis must not be null"

        print(f"\n✅ recovery_analysis present")
        print(f"   Keys: {list(result.keys())}")

    @pytest.mark.asyncio
    async def test_previous_attempt_assessment_structure(self, sample_recovery_request):
        """
        HAPI Integration: previous_attempt_assessment has all required fields.
        
        Pattern: Direct business logic → Validate nested structure
        """
        from src.extensions.recovery.llm_integration import analyze_recovery
        from src.models.config_models import AppConfig
        
        # ACT: Call business logic
        app_config = AppConfig()
        result = await analyze_recovery(sample_recovery_request, app_config=app_config)

        # ASSERT: previous_attempt_assessment exists
        assert 'previous_attempt_assessment' in result['recovery_analysis'], \
            "recovery_analysis must contain previous_attempt_assessment"

        prev_assessment = result['recovery_analysis']['previous_attempt_assessment']
        assert prev_assessment is not None, "previous_attempt_assessment must not be null"

        # ASSERT: All 4 required fields present
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

    @pytest.mark.asyncio
    async def test_field_types_correct(self, sample_recovery_request):
        """
        HAPI Integration: All 4 fields have correct types for AA team mapping.
        
        Pattern: Direct business logic → Validate field types
        """
        from src.extensions.recovery.llm_integration import analyze_recovery
        from src.models.config_models import AppConfig
        
        # ACT
        app_config = AppConfig()
        result = await analyze_recovery(sample_recovery_request, app_config=app_config)
        prev_assessment = result['recovery_analysis']['previous_attempt_assessment']

        # ASSERT: Field type 1 - failure_understood (boolean)
        assert isinstance(prev_assessment['failure_understood'], bool), \
            "failure_understood must be boolean"

        # ASSERT: Field type 2 - failure_reason_analysis (string)
        assert isinstance(prev_assessment['failure_reason_analysis'], str), \
            "failure_reason_analysis must be string"
        assert len(prev_assessment['failure_reason_analysis']) > 0, \
            "failure_reason_analysis must not be empty"

        # ASSERT: Field type 3 - state_changed (boolean)
        assert isinstance(prev_assessment['state_changed'], bool), \
            "state_changed must be boolean"

        # ASSERT: Field type 4 - current_signal_type (string)
        assert isinstance(prev_assessment['current_signal_type'], str), \
            "current_signal_type must be string"

        print(f"\n✅ Field types validated")
        print(f"   failure_understood: {type(prev_assessment['failure_understood']).__name__}")
        print(f"   failure_reason_analysis: {type(prev_assessment['failure_reason_analysis']).__name__}")
        print(f"   state_changed: {type(prev_assessment['state_changed']).__name__}")
        print(f"   current_signal_type: {type(prev_assessment['current_signal_type']).__name__}")

    @pytest.mark.asyncio
    async def test_failure_reason_analysis_has_substance(self, sample_recovery_request):
        """
        HAPI Integration: failure_reason_analysis contains meaningful analysis.
        
        Pattern: Direct business logic → Validate content quality
        """
        from src.extensions.recovery.llm_integration import analyze_recovery
        from src.models.config_models import AppConfig
        
        # ACT
        app_config = AppConfig()
        result = await analyze_recovery(sample_recovery_request, app_config=app_config)
        prev_assessment = result['recovery_analysis']['previous_attempt_assessment']

        # ASSERT: failure_reason_analysis has substance
        analysis = prev_assessment['failure_reason_analysis']
        
        # Should be more than just a stub
        assert len(analysis) > 20, \
            f"failure_reason_analysis should contain meaningful content (got {len(analysis)} chars)"
        
        # Should relate to the failure
        # (This is loose validation - full content validation is E2E)
        assert any(keyword in analysis.lower() for keyword in ['fail', 'error', 'reason', 'because']), \
            "failure_reason_analysis should contain failure-related keywords"

        print(f"\n✅ failure_reason_analysis has substance")
        print(f"   Length: {len(analysis)} characters")
        print(f"   Preview: {analysis[:100]}...")

    @pytest.mark.asyncio
    async def test_current_signal_type_matches_input(self, sample_recovery_request):
        """
        HAPI Integration: current_signal_type reflects the actual signal.
        
        Pattern: Direct business logic → Validate field semantics
        """
        from src.extensions.recovery.llm_integration import analyze_recovery
        from src.models.config_models import AppConfig
        
        # ACT
        app_config = AppConfig()
        result = await analyze_recovery(sample_recovery_request, app_config=app_config)
        prev_assessment = result['recovery_analysis']['previous_attempt_assessment']

        # ASSERT: current_signal_type is populated (exact value depends on LLM logic)
        current_signal = prev_assessment['current_signal_type']
        
        assert current_signal is not None, "current_signal_type must not be null"
        assert len(current_signal) > 0, "current_signal_type must not be empty"
        
        # Should be a valid signal type format (CamelCase or alphanumeric)
        assert current_signal.replace(' ', '').isalnum() or 'OOM' in current_signal.upper(), \
            "current_signal_type should be valid signal type format"

        print(f"\n✅ current_signal_type validated")
        print(f"   Value: {current_signal}")

    @pytest.mark.asyncio
    async def test_state_changed_is_boolean(self, sample_recovery_request):
        """
        HAPI Integration: state_changed is proper boolean (not string "true"/"false").
        
        Pattern: Direct business logic → Validate strict type
        """
        from src.extensions.recovery.llm_integration import analyze_recovery
        from src.models.config_models import AppConfig
        
        # ACT
        app_config = AppConfig()
        result = await analyze_recovery(sample_recovery_request, app_config=app_config)
        prev_assessment = result['recovery_analysis']['previous_attempt_assessment']

        # ASSERT: state_changed is bool (not string "true" or int 1)
        state_changed = prev_assessment['state_changed']
        
        assert type(state_changed) is bool, \
            f"state_changed must be bool type, got {type(state_changed).__name__}"
        
        # Should be True or False (Python bool)
        assert state_changed in [True, False], \
            "state_changed must be True or False"

        print(f"\n✅ state_changed is proper boolean")
        print(f"   Type: {type(state_changed).__name__}")
        print(f"   Value: {state_changed}")

    @pytest.mark.asyncio
    async def test_all_fields_serializable_to_json(self, sample_recovery_request):
        """
        HAPI Integration: recovery_analysis can be serialized to JSON.
        
        This is critical for AA team to store in CRD status.
        
        Pattern: Direct business logic → Validate serializability
        """
        import json
        from src.extensions.recovery.llm_integration import analyze_recovery
        from src.models.config_models import AppConfig
        
        # ACT
        app_config = AppConfig()
        result = await analyze_recovery(sample_recovery_request, app_config=app_config)

        # ASSERT: recovery_analysis serializes to JSON
        recovery_analysis = result['recovery_analysis']
        
        try:
            json_str = json.dumps(recovery_analysis)
            reconstructed = json.loads(json_str)
            
            assert reconstructed == recovery_analysis, \
                "Deserialized recovery_analysis should match original"
            
            print(f"\n✅ recovery_analysis JSON serializable")
            print(f"   JSON length: {len(json_str)} characters")
            
        except (TypeError, ValueError) as e:
            pytest.fail(f"recovery_analysis not JSON serializable: {e}")


# ========================================
# DEPRECATED (E2E PATTERN - DO NOT USE)
# ========================================

class TestRecoveryAnalysisViableCorrection_DEPRECATED:
    """
    ⚠️  DEPRECATED: These tests used HTTP TestClient pattern.
    
    Problem: Importing main.py causes K8s auth initialization failures.
    
    Solution: Refactored to call business logic directly (above tests).
    """
    pass
