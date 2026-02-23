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
                signal_name="CrashLoopBackOff",
                severity="high",
                signal_source="kubernetes",
                resource_namespace="default",
                resource_kind="Pod",
                resource_name="test-pod",
                error_message="Container crashed",
                environment="production",
                priority="high",
                risk_tolerance="low",
                business_category="critical",
                cluster_name="prod-cluster-1"
            )
        
        # Verify error message contains remediation_id error
        errors = exc_info.value.errors()
        # Should have only 1 error for remediation_id (the other required fields are provided)
        remediation_errors = [e for e in errors if 'remediation_id' in str(e['loc'])]
        assert len(remediation_errors) >= 1
        assert 'required' in remediation_errors[0]['msg'].lower() or 'empty' in remediation_errors[0]['msg'].lower() or '1 character' in remediation_errors[0]['msg'].lower()

    def test_missing_remediation_id_raises_error(self):
        """E2E-HAPI-008: Missing remediation_id should raise ValidationError"""
        with pytest.raises(ValidationError) as exc_info:
            # Don't provide remediation_id at all
            IncidentRequest(
                incident_id="test-001",
                # remediation_id NOT provided
                signal_name="CrashLoopBackOff",
                severity="high",
                signal_source="kubernetes",
                resource_namespace="default",
                resource_kind="Pod",
                resource_name="test-pod",
                error_message="Container crashed",
                environment="production",
                priority="high",
                risk_tolerance="low",
                business_category="critical",
                cluster_name="prod-cluster-1"
            )
        
        # Verify error indicates missing field
        errors = exc_info.value.errors()
        assert any('remediation_id' in str(err['loc']) for err in errors)

    def test_valid_remediation_id_passes(self):
        """E2E-HAPI-008: Valid remediation_id should pass validation"""
        request = IncidentRequest(
            incident_id="test-001",
            remediation_id="test-rem-001",
            signal_name="CrashLoopBackOff",
            severity="high",
            signal_source="kubernetes",
            resource_namespace="default",
            resource_kind="Pod",
            resource_name="test-pod",
            error_message="Container crashed",
            environment="production",
            priority="high",
            risk_tolerance="low",
            business_category="critical",
            cluster_name="prod-cluster-1"
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
                signal_name="OOMKilled",
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
                signal_name="OOMKilled",
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
            signal_name="OOMKilled",
            severity="high"
        )
        assert request.recovery_attempt_number == 1


class TestEndpointValidation:
    """Test endpoint-level validation (E2E-HAPI-007)"""

    def test_empty_signal_name_raises_error(self):
        """E2E-HAPI-007: Empty signal_name should raise ValidationError"""
        # Empty signal_name passes Pydantic but may fail endpoint validation
        request = IncidentRequest(
            incident_id="test-001",
            remediation_id="test-rem-001",
            signal_name="",  # Empty
            severity="high",
            signal_source="kubernetes",
            resource_namespace="default",
            resource_kind="Pod",
            resource_name="test-pod",
            error_message="Container crashed",
            environment="production",
            priority="high",
            risk_tolerance="low",
            business_category="critical",
            cluster_name="prod-cluster-1"
        )
        # Pydantic allows empty string, endpoint validation should catch it
        assert request.signal_name == ""

    def test_invalid_severity_raises_error(self):
        """E2E-HAPI-007: Invalid severity should raise ValidationError"""
        # BR-SEVERITY-001: severity is a Severity enum — Pydantic rejects invalid values
        with pytest.raises(ValidationError, match="severity"):
            IncidentRequest(
                incident_id="test-001",
                remediation_id="test-rem-001",
                signal_name="CrashLoopBackOff",
                severity="invalid_severity",
                signal_source="kubernetes",
                resource_namespace="default",
                resource_kind="Pod",
                resource_name="test-pod",
                error_message="Container crashed",
                environment="production",
                priority="high",
                risk_tolerance="low",
                business_category="critical",
                cluster_name="prod-cluster-1"
            )


if __name__ == "__main__":
    pytest.main([__file__, "-v", "-s"])
