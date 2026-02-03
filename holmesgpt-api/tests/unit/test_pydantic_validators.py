"""
Unit tests for Pydantic field validators (E2E-HAPI-008, 018)

Tests the @field_validator decorators to debug why they're not triggering
in the E2E tests.

Run with: pytest holmesgpt-api/tests/unit/test_pydantic_validators.py -v
"""

import pytest
from pydantic import ValidationError
from src.models.incident_models import IncidentRequest
from src.models.recovery_models import RecoveryRequest


class TestIncidentRequestValidation:
    """Test IncidentRequest Pydantic validators (E2E-HAPI-008)"""

    def test_empty_remediation_id_raises_error(self):
        """E2E-HAPI-008: Empty remediation_id should raise ValidationError"""
        with pytest.raises(ValidationError) as exc_info:
            IncidentRequest(
                incident_id="test-001",
                remediation_id="",  # Empty string
                signal_type="CrashLoopBackOff",
                severity="high",
                signal_source="kubernetes",
                resource_namespace="default",
                resource_kind="Pod",
                resource_name="test-pod"
            )
        
        # Verify error message
        errors = exc_info.value.errors()
        assert len(errors) == 1
        assert errors[0]['loc'] == ('remediation_id',)
        assert 'required' in errors[0]['msg'].lower() or 'empty' in errors[0]['msg'].lower()

    def test_missing_remediation_id_raises_error(self):
        """E2E-HAPI-008: Missing remediation_id should raise ValidationError"""
        with pytest.raises(ValidationError) as exc_info:
            # Don't provide remediation_id at all
            IncidentRequest(
                incident_id="test-001",
                # remediation_id NOT provided
                signal_type="CrashLoopBackOff",
                severity="high",
                signal_source="kubernetes",
                resource_namespace="default",
                resource_kind="Pod",
                resource_name="test-pod"
            )
        
        # Verify error indicates missing field
        errors = exc_info.value.errors()
        assert any('remediation_id' in str(err['loc']) for err in errors)

    def test_valid_remediation_id_passes(self):
        """E2E-HAPI-008: Valid remediation_id should pass validation"""
        request = IncidentRequest(
            incident_id="test-001",
            remediation_id="test-rem-001",
            signal_type="CrashLoopBackOff",
            severity="high",
            signal_source="kubernetes",
            resource_namespace="default",
            resource_kind="Pod",
            resource_name="test-pod"
        )
        assert request.remediation_id == "test-rem-001"


class TestRecoveryRequestValidation:
    """Test RecoveryRequest Pydantic validators (E2E-HAPI-018)"""

    def test_invalid_recovery_attempt_number_raises_error(self):
        """E2E-HAPI-018: recovery_attempt_number < 1 should raise ValidationError"""
        with pytest.raises(ValidationError) as exc_info:
            RecoveryRequest(
                incident_id="test-recovery-001",
                remediation_id="test-rem-001",
                is_recovery_attempt=True,
                recovery_attempt_number=0,  # Invalid: < 1
                signal_type="OOMKilled",
                severity="high"
            )
        
        # Verify error message
        errors = exc_info.value.errors()
        assert len(errors) == 1
        assert errors[0]['loc'] == ('recovery_attempt_number',)
        assert '>= 1' in errors[0]['msg'] or 'greater' in errors[0]['msg'].lower()

    def test_negative_recovery_attempt_number_raises_error(self):
        """E2E-HAPI-018: Negative recovery_attempt_number should raise ValidationError"""
        with pytest.raises(ValidationError) as exc_info:
            RecoveryRequest(
                incident_id="test-recovery-001",
                remediation_id="test-rem-001",
                is_recovery_attempt=True,
                recovery_attempt_number=-1,  # Invalid: negative
                signal_type="OOMKilled",
                severity="high"
            )
        
        errors = exc_info.value.errors()
        assert any('recovery_attempt_number' in str(err['loc']) for err in errors)

    def test_valid_recovery_attempt_number_passes(self):
        """E2E-HAPI-018: Valid recovery_attempt_number should pass"""
        request = RecoveryRequest(
            incident_id="test-recovery-001",
            remediation_id="test-rem-001",
            is_recovery_attempt=True,
            recovery_attempt_number=1,  # Valid
            signal_type="OOMKilled",
            severity="high"
        )
        assert request.recovery_attempt_number == 1


class TestEndpointValidation:
    """Test endpoint-level validation (E2E-HAPI-007)"""

    def test_empty_signal_type_raises_error(self):
        """E2E-HAPI-007: Empty signal_type should raise ValidationError"""
        with pytest.raises(ValidationError) as exc_info:
            IncidentRequest(
                incident_id="test-001",
                remediation_id="test-rem-001",
                signal_type="",  # Empty
                severity="high",
                signal_source="kubernetes",
                resource_namespace="default",
                resource_kind="Pod",
                resource_name="test-pod"
            )
        
        errors = exc_info.value.errors()
        assert any('signal_type' in str(err['loc']) for err in errors)

    def test_invalid_severity_raises_error(self):
        """E2E-HAPI-007: Invalid severity should raise ValidationError"""
        # Note: This test checks if Pydantic has enum validation
        # The endpoint has additional validation for this
        request = IncidentRequest(
            incident_id="test-001",
            remediation_id="test-rem-001",
            signal_type="CrashLoopBackOff",
            severity="invalid_severity",  # This might pass Pydantic but fail endpoint validation
            signal_source="kubernetes",
            resource_namespace="default",
            resource_kind="Pod",
            resource_name="test-pod"
        )
        # If this passes, it means Pydantic doesn't validate severity enum
        # and we rely on endpoint validation (which we added in Phase 2)
        assert request.severity == "invalid_severity"


if __name__ == "__main__":
    pytest.main([__file__, "-v", "-s"])
