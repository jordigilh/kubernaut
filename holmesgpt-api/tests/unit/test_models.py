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
Pydantic Model Validation Tests

Business Requirements: BR-HAPI-001, BR-HAPI-051 (Model validation)
"""

import pytest
from pydantic import ValidationError


class TestRecoveryModels:
    """Tests for recovery analysis models"""

    def test_recovery_request_model_validates_required_fields(self):
        """
        Business Requirement: Required field validation
        Expected: Pydantic requires specified fields
        """
        from src.models.recovery_models import RecoveryRequest

        # Missing required field
        with pytest.raises(ValidationError) as exc_info:
            RecoveryRequest(
                # Missing incident_id (required)
                failed_action={},
                failure_context={}
            )
        assert "incident_id" in str(exc_info.value).lower()

    def test_recovery_request_accepts_valid_data(self):
        """
        Business Requirement: Valid data acceptance
        Expected: Model accepts valid request data

        Updated: DD-WORKFLOW-002 v2.2 - remediation_id is now mandatory
        """
        from src.models.recovery_models import RecoveryRequest

        request = RecoveryRequest(
            incident_id="inc-001",
            remediation_id="req-2025-11-27-abc123",  # DD-WORKFLOW-002 v2.2: mandatory
            failed_action={"type": "scale"},
            failure_context={"error": "timeout"}
        )
        assert request.incident_id == "inc-001"
        assert request.remediation_id == "req-2025-11-27-abc123"

    def test_recovery_strategy_validates_confidence_range(self):
        """
        Business Requirement: Confidence must be 0.0 to 1.0
        Expected: ValidationError for out-of-range confidence
        """
        from src.models.recovery_models import RecoveryStrategy

        # Valid confidence
        strategy = RecoveryStrategy(
            action_type="rollback",
            confidence=0.85,
            rationale="Test",
            estimated_risk="low"
        )
        assert strategy.confidence == 0.85

        # Invalid confidence (too high)
        with pytest.raises(ValidationError):
            RecoveryStrategy(
                action_type="rollback",
                confidence=1.5,
                rationale="Test",
                estimated_risk="low"
            )


class TestPostExecModels:
    """Tests for post-execution analysis models"""

    def test_postexec_request_model_validates_required_fields(self):
        """
        Business Requirement: Required field validation
        Expected: Pydantic requires specified fields
        """
        from src.models.postexec_models import PostExecRequest

        # Missing required fields
        with pytest.raises(ValidationError):
            PostExecRequest(
                # Missing multiple required fields
                execution_id="exec-001"
            )

    def test_postexec_request_accepts_valid_data(self):
        """
        Business Requirement: Valid data acceptance
        Expected: Model accepts valid request data
        """
        from src.models.postexec_models import PostExecRequest

        request = PostExecRequest(
            execution_id="exec-001",
            action_id="action-001",
            action_type="scale",
            action_details={"replicas": 3},
            execution_success=True,
            execution_result={"status": "success"}
        )
        assert request.execution_id == "exec-001"
        assert request.execution_success is True

    def test_effectiveness_assessment_validates_confidence(self):
        """
        Business Requirement: Confidence must be 0.0 to 1.0
        Expected: ValidationError for invalid confidence
        """
        from src.models.postexec_models import EffectivenessAssessment

        # Valid confidence
        assessment = EffectivenessAssessment(
            success=True,
            confidence=0.9,
            reasoning="Test"
        )
        assert assessment.confidence == 0.9

        # Invalid confidence (negative)
        with pytest.raises(ValidationError):
            EffectivenessAssessment(
                success=True,
                confidence=-0.5,
                reasoning="Test"
            )


class TestModelSerialization:
    """Tests for model serialization/deserialization"""

    def test_recovery_request_serializes_to_dict(self):
        """
        Business Requirement: Model serialization
        Expected: Models can be converted to dictionaries

        Updated: DD-WORKFLOW-002 v2.2 - remediation_id is now mandatory
        """
        from src.models.recovery_models import RecoveryRequest

        request = RecoveryRequest(
            incident_id="inc-001",
            remediation_id="req-2025-11-27-abc123",  # DD-WORKFLOW-002 v2.2: mandatory
            failed_action={"type": "scale"},
            failure_context={"error": "timeout"}
        )

        # Use model_dump() or dict() depending on Pydantic version
        data = request.model_dump() if hasattr(request, 'model_dump') else request.dict()
        assert isinstance(data, dict)
        assert data["incident_id"] == "inc-001"
        assert data["remediation_id"] == "req-2025-11-27-abc123"

    def test_postexec_response_serializes_to_dict(self):
        """
        Business Requirement: Model serialization
        Expected: Response models can be converted to dictionaries
        """
        from src.models.postexec_models import PostExecResponse, EffectivenessAssessment

        response = PostExecResponse(
            execution_id="exec-001",
            effectiveness=EffectivenessAssessment(
                success=True,
                confidence=0.9,
                reasoning="Test"
            ),
            objectives_met=True
        )

        # Use model_dump() or dict() depending on Pydantic version
        data = response.model_dump() if hasattr(response, 'model_dump') else response.dict()
        assert isinstance(data, dict)
        assert data["execution_id"] == "exec-001"
        assert data["objectives_met"] is True
