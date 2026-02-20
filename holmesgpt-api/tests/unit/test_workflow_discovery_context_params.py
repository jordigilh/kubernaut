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
Cycle 2.2: _build_context_params reads detected_labels from session_state

Tests that _build_context_params() reads detected_labels from session_state
(populated at runtime by get_resource_context / LabelDetector) instead of
relying solely on constructor-time detected_labels.

Authority: ADR-056, DD-HAPI-017 Section 8
Business Requirement: BR-HAPI-102

Test IDs:
  UT-HAPI-056-051: Reads detected_labels from session_state
  UT-HAPI-056-052: session_state detected_labels take precedence over constructor
  UT-HAPI-056-053: strip_failed_detections applied to session_state labels
  UT-HAPI-056-054: Empty sentinel {} in session_state produces no detected_labels param
  UT-HAPI-056-055: session_state present but detected_labels key missing (no param)
"""

import json
import pytest

from src.models.incident_models import DetectedLabels
from src.toolsets.workflow_discovery import ListAvailableActionsTool


COMMON_KWARGS = dict(
    severity="critical",
    component="pod",
    environment="production",
    priority="P0",
)


class TestBuildContextParamsSessionState:
    """
    UT-HAPI-056-051 through UT-HAPI-056-055: _build_context_params reads detected_labels
    from session_state when available, with proper fallback, precedence, and
    strip_failed_detections behavior.
    """

    def test_ut_hapi_056_051_reads_detected_labels_from_session_state(self):
        """UT-HAPI-056-051: detected_labels are read from session_state and included in params."""
        session_state = {
            "detected_labels": {
                "gitOpsManaged": True,
                "gitOpsTool": "argocd",
                "pdbProtected": False,
                "hpaEnabled": True,
                "stateful": False,
                "helmManaged": False,
                "networkIsolated": False,
                "serviceMesh": "",
            }
        }
        tool = ListAvailableActionsTool(
            **COMMON_KWARGS,
            session_state=session_state,
        )
        params = tool._build_context_params()

        assert "detected_labels" in params
        parsed = json.loads(params["detected_labels"])
        assert parsed["gitOpsManaged"] is True
        assert parsed["gitOpsTool"] == "argocd"
        assert parsed["hpaEnabled"] is True

    def test_ut_hapi_056_052_session_state_takes_precedence_over_constructor(self):
        """UT-HAPI-056-052: session_state detected_labels override constructor detected_labels."""
        constructor_labels = DetectedLabels(gitOpsManaged=False, gitOpsTool="flux")
        session_state = {
            "detected_labels": {
                "gitOpsManaged": True,
                "gitOpsTool": "argocd",
                "pdbProtected": True,
            }
        }
        tool = ListAvailableActionsTool(
            **COMMON_KWARGS,
            detected_labels=constructor_labels,
            session_state=session_state,
        )
        params = tool._build_context_params()

        assert "detected_labels" in params
        parsed = json.loads(params["detected_labels"])
        assert parsed["gitOpsManaged"] is True
        assert parsed["gitOpsTool"] == "argocd"
        assert parsed["pdbProtected"] is True

    def test_ut_hapi_056_053_strip_failed_detections_applied_to_session_state(self):
        """UT-HAPI-056-053: strip_failed_detections is applied to session_state labels."""
        session_state = {
            "detected_labels": {
                "gitOpsManaged": True,
                "pdbProtected": False,
                "failedDetections": ["pdbProtected"],
            }
        }
        tool = ListAvailableActionsTool(
            **COMMON_KWARGS,
            session_state=session_state,
        )
        params = tool._build_context_params()

        assert "detected_labels" in params
        parsed = json.loads(params["detected_labels"])
        assert "pdbProtected" not in parsed
        assert parsed["gitOpsManaged"] is True

    def test_ut_hapi_056_054_empty_sentinel_produces_no_detected_labels_param(self):
        """UT-HAPI-056-054: Empty {} sentinel in session_state -> no detected_labels in params."""
        session_state = {"detected_labels": {}}
        tool = ListAvailableActionsTool(
            **COMMON_KWARGS,
            session_state=session_state,
        )
        params = tool._build_context_params()

        assert "detected_labels" not in params

    def test_ut_hapi_056_055_session_state_present_but_no_key(self):
        """UT-HAPI-056-055: session_state exists but detected_labels key missing -> no param."""
        session_state = {"some_other_key": "value"}
        tool = ListAvailableActionsTool(
            **COMMON_KWARGS,
            session_state=session_state,
        )
        params = tool._build_context_params()

        assert "detected_labels" not in params
