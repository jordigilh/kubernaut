"""
Unit tests for remediation_id validation in incident analysis requests

Business Requirement: BR-AUDIT-001, BR-INTEGRATION-003
Design Decision: DD-WORKFLOW-002 v2.4, DD-WORKFLOW-014
Authority: REMEDIATION-ID-PROPAGATION-IMPLEMENTATION_PLAN_V1.4.md Day 1 AM

TDD Phase: RED (failing tests)

⚠️ CRITICAL: remediation_id Usage Constraint
- Field is MANDATORY but for CORRELATION/AUDIT ONLY
- Do NOT use for RCA analysis or workflow matching
- Simply propagate from request context to workflow search for traceability
"""

import pytest
from pydantic import ValidationError


class TestIncidentRequestRemediationId:
    """
    Unit tests for remediation_id field in IncidentRequest

    Business Requirement: BR-AUDIT-001 - Unified audit trail
    Design Decision: DD-WORKFLOW-002 v2.2 - remediation_id mandatory

    Test Strategy:
    - Test remediation_id is required (validation error if missing)
    - Test remediation_id cannot be empty string
    - Test remediation_id accepts valid UUID-style identifiers
    - Test remediation_id is pass-through (not used in business logic)
    """

    @pytest.fixture
    def valid_incident_data(self):
        """Valid incident request data WITH remediation_id"""
        return {
            "incident_id": "inc-2025-11-27-001",
            "remediation_id": "req-2025-11-27-abc123",  # MANDATORY per DD-WORKFLOW-002 v2.2
            "signal_name": "OOMKilled",
            "severity": "critical",
            "signal_source": "prometheus",
            "resource_namespace": "production",
            "resource_kind": "Pod",
            "resource_name": "nginx-deployment-abc123",
            "error_message": "Container exceeded memory limit",
            "environment": "production",
            "priority": "P0",
            "risk_tolerance": "low",
            "business_category": "revenue-critical",
            "cluster_name": "prod-us-west-2",
        }

    @pytest.fixture
    def incident_data_without_remediation_id(self, valid_incident_data):
        """Incident request data WITHOUT remediation_id (should fail validation)"""
        data = valid_incident_data.copy()
        del data["remediation_id"]
        return data

    def test_incident_request_requires_remediation_id(self, incident_data_without_remediation_id):
        """
        Test that IncidentRequest requires remediation_id field

        Business Requirement: BR-AUDIT-001 - Unified audit trail
        Design Decision: DD-WORKFLOW-002 v2.2 - remediation_id mandatory

        TDD Phase: RED (this test should FAIL initially)
        Expected Error: ValidationError for missing remediation_id field
        """
        from src.models.incident_models import IncidentRequest

        # ACT & ASSERT: Should raise ValidationError for missing remediation_id
        with pytest.raises(ValidationError) as exc_info:
            IncidentRequest(**incident_data_without_remediation_id)

        # Verify error mentions remediation_id
        error_str = str(exc_info.value).lower()
        assert "remediation_id" in error_str, \
            f"ValidationError should mention 'remediation_id', got: {exc_info.value}"

    def test_incident_request_rejects_empty_remediation_id(self, valid_incident_data):
        """
        Test that IncidentRequest rejects empty remediation_id

        Business Requirement: BR-AUDIT-001 - Unified audit trail
        Design Decision: DD-WORKFLOW-002 v2.2 - remediation_id cannot be empty

        TDD Phase: RED (this test should FAIL initially)
        Expected Error: ValidationError for empty remediation_id
        """
        from src.models.incident_models import IncidentRequest

        # ARRANGE: Set remediation_id to empty string
        data = valid_incident_data.copy()
        data["remediation_id"] = ""

        # ACT & ASSERT: Should raise ValidationError for empty remediation_id
        with pytest.raises(ValidationError) as exc_info:
            IncidentRequest(**data)

        # Verify error mentions remediation_id
        error_str = str(exc_info.value).lower()
        assert "remediation_id" in error_str, \
            f"ValidationError should mention 'remediation_id', got: {exc_info.value}"

    def test_incident_request_accepts_valid_remediation_id(self, valid_incident_data):
        """
        Test that IncidentRequest accepts valid remediation_id

        Business Requirement: BR-AUDIT-001 - Unified audit trail
        Design Decision: DD-WORKFLOW-002 v2.2 - remediation_id format

        TDD Phase: RED (this test should FAIL initially)
        Expected: IncidentRequest created successfully with remediation_id field
        """
        from src.models.incident_models import IncidentRequest

        # ACT: Create IncidentRequest with valid remediation_id
        request = IncidentRequest(**valid_incident_data)

        # ASSERT: remediation_id should be stored correctly
        assert request.remediation_id == "req-2025-11-27-abc123", \
            f"Expected remediation_id 'req-2025-11-27-abc123', got '{request.remediation_id}'"

    def test_remediation_id_is_passthrough_only(self, valid_incident_data):
        """
        Test that remediation_id is a pass-through field (not used in business logic)

        Business Requirement: BR-AUDIT-001 - Correlation only
        Design Decision: DD-WORKFLOW-002 v2.2 - Pass-through value

        ⚠️ CRITICAL: remediation_id must NOT be used for:
        - RCA analysis
        - Workflow matching
        - Search queries

        It is ONLY for audit trail correlation.

        TDD Phase: RED (this test should FAIL initially)
        """
        from src.models.incident_models import IncidentRequest

        # ARRANGE: Create two requests with different remediation_ids
        data1 = valid_incident_data.copy()
        data1["remediation_id"] = "req-001"

        data2 = valid_incident_data.copy()
        data2["remediation_id"] = "req-002"

        # ACT: Create both requests
        request1 = IncidentRequest(**data1)
        request2 = IncidentRequest(**data2)

        # ASSERT: Both requests should have correct remediation_ids
        assert request1.remediation_id == "req-001"
        assert request2.remediation_id == "req-002"

        # ASSERT: All OTHER fields should be identical (remediation_id doesn't affect them)
        assert request1.incident_id == request2.incident_id
        assert request1.signal_name == request2.signal_name
        assert request1.severity == request2.severity


class TestRecoveryRequestRemediationId:
    """
    Unit tests for remediation_id field in RecoveryRequest

    Business Requirement: BR-AUDIT-001 - Unified audit trail
    Design Decision: DD-WORKFLOW-002 v2.2 - remediation_id mandatory for all request types
    """

    @pytest.fixture
    def valid_recovery_data(self):
        """Valid recovery request data WITH remediation_id"""
        return {
            "incident_id": "inc-2025-11-27-001",
            "remediation_id": "req-2025-11-27-abc123",  # MANDATORY per DD-WORKFLOW-002 v2.2
            "failed_action": {
                "type": "scale_deployment",
                "target": "nginx",
                "desired_replicas": 5
            },
            "failure_context": {
                "error": "insufficient_resources",
                "cluster_state": "high_load"
            },
        }

    def test_recovery_request_requires_remediation_id(self, valid_recovery_data):
        """
        Test that RecoveryRequest requires remediation_id field

        Business Requirement: BR-AUDIT-001 - Unified audit trail
        Design Decision: DD-WORKFLOW-002 v2.2 - remediation_id mandatory

        TDD Phase: RED (this test should FAIL initially)
        """
        from src.models.recovery_models import RecoveryRequest

        # ARRANGE: Remove remediation_id
        data = valid_recovery_data.copy()
        del data["remediation_id"]

        # ACT & ASSERT: Should raise ValidationError
        with pytest.raises(ValidationError) as exc_info:
            RecoveryRequest(**data)

        error_str = str(exc_info.value).lower()
        assert "remediation_id" in error_str

    def test_recovery_request_accepts_valid_remediation_id(self, valid_recovery_data):
        """
        Test that RecoveryRequest accepts valid remediation_id

        TDD Phase: RED (this test should FAIL initially)
        """
        from src.models.recovery_models import RecoveryRequest

        # ACT: Create RecoveryRequest
        request = RecoveryRequest(**valid_recovery_data)

        # ASSERT: remediation_id stored correctly
        assert request.remediation_id == "req-2025-11-27-abc123"

