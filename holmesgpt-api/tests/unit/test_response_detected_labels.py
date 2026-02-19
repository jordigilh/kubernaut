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
Cycle 2.5: HAPI response includes detected_labels for PostRCAContext

Verifies that IncidentResponse and RecoveryResponse models include a
detected_labels field, and that the integration code populates it from
session_state before returning the response.

Authority: ADR-056, DD-HAPI-018
Business Requirement: BR-HAPI-102

Test IDs:
  UT-HAPI-056-063: IncidentResponse model includes detected_labels field
  UT-HAPI-056-064: RecoveryResponse model includes detected_labels field
  UT-HAPI-056-065: detected_labels injected into incident result from session_state
  UT-HAPI-056-066: detected_labels injected into recovery result from session_state
  UT-HAPI-056-067: detected_labels defaults to None when not in session_state
"""

import pytest
from typing import Dict, Any, Optional


class TestResponseDetectedLabelsModel:
    """
    UT-HAPI-056-063, UT-HAPI-056-064: Verify response models include detected_labels.
    """

    def test_ut_hapi_056_063_incident_response_has_detected_labels_field(self):
        """UT-HAPI-056-063: IncidentResponse model has detected_labels optional field."""
        from src.models.incident_models import IncidentResponse

        response = IncidentResponse(
            incident_id="inc-001",
            analysis="test analysis",
            root_cause_analysis={"summary": "test"},
            confidence=0.9,
            timestamp="2025-01-01T00:00:00Z",
            detected_labels={"gitOpsManaged": True, "pdbProtected": False},
        )
        assert response.detected_labels == {"gitOpsManaged": True, "pdbProtected": False}

    def test_ut_hapi_056_064_recovery_response_has_detected_labels_field(self):
        """UT-HAPI-056-064: RecoveryResponse model has detected_labels optional field."""
        from src.models.recovery_models import RecoveryResponse

        response = RecoveryResponse(
            incident_id="inc-001",
            can_recover=True,
            analysis_confidence=0.8,
            detected_labels={"stateful": True, "helmManaged": False},
        )
        assert response.detected_labels == {"stateful": True, "helmManaged": False}


class TestResponseDetectedLabelsInjection:
    """
    UT-HAPI-056-065 through UT-HAPI-056-067: Verify detected_labels injection from session_state.
    """

    def test_ut_hapi_056_065_incident_result_includes_detected_labels(self):
        """UT-HAPI-056-065: inject_detected_labels adds session_state labels to result dict."""
        from src.extensions.llm_config import inject_detected_labels

        result = {"incident_id": "inc-001", "analysis": "test"}
        session_state = {"detected_labels": {"gitOpsManaged": True}}

        inject_detected_labels(result, session_state)

        assert result["detected_labels"] == {"gitOpsManaged": True}

    def test_ut_hapi_056_066_recovery_result_includes_detected_labels(self):
        """UT-HAPI-056-066: inject_detected_labels works for recovery results too."""
        from src.extensions.llm_config import inject_detected_labels

        result = {"incident_id": "inc-001", "can_recover": True}
        session_state = {"detected_labels": {"stateful": True, "pdbProtected": True}}

        inject_detected_labels(result, session_state)

        assert result["detected_labels"] == {"stateful": True, "pdbProtected": True}

    def test_ut_hapi_056_067_detected_labels_none_when_not_in_session(self):
        """UT-HAPI-056-067: detected_labels not added when session_state has no labels."""
        from src.extensions.llm_config import inject_detected_labels

        result = {"incident_id": "inc-001"}

        inject_detected_labels(result, None)
        assert "detected_labels" not in result

        inject_detected_labels(result, {})
        assert "detected_labels" not in result

        inject_detected_labels(result, {"some_other_key": "value"})
        assert "detected_labels" not in result
