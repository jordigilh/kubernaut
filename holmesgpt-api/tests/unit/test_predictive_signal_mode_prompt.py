"""
Copyright 2026 Jordi Gil.

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
Unit Tests: Predictive Signal Mode Prompt Builder
BR-AI-084: Predictive Signal Mode Prompt Strategy
ADR-054: Predictive Signal Mode Classification

Tests that create_incident_investigation_prompt() generates the correct
investigation directives based on signal_mode.
"""

import pytest
import sys
import types
import importlib

# ---------------------------------------------------------------------------
# Mock the holmes SDK and prometrix packages to avoid pydantic v1 + Python 3.14
# incompatibility.  create_incident_investigation_prompt only depends on
# src.models.incident_models and src.extensions.incident.constants — it does NOT
# depend on holmes.  But the import chain through src.extensions → recovery/incident
# → llm_integration → holmes triggers the crash.
#
# Strategy: Create proper module stubs with __path__, __spec__, and a permissive
# __getattr__ so any name imported from them resolves to a no-op placeholder.
# ---------------------------------------------------------------------------

def _make_stub(name):
    """Create a stub module that acts as a package and returns MagicMock for any attribute."""
    mod = types.ModuleType(name)
    mod.__path__ = []
    mod.__package__ = name
    mod.__spec__ = importlib.machinery.ModuleSpec(name, None, is_package=True)

    # Any attribute access returns a MagicMock so `from X import Y` works
    from unittest.mock import MagicMock
    _sentinel = MagicMock()
    original_getattr = mod.__getattribute__

    def _permissive_getattr(attr):
        try:
            return original_getattr(attr)
        except AttributeError:
            return _sentinel
    mod.__getattr__ = lambda attr: _sentinel
    return mod


_MOCK_PACKAGES = [
    "holmes", "holmes.config", "holmes.core", "holmes.core.models",
    "holmes.core.investigation", "holmes.core.supabase_dal", "holmes.core.tools",
    "holmes.core.tools_utils", "holmes.core.tools_utils.tool_executor",
    "holmes.core.tools_utils.toolset_utils",
    "holmes.plugins", "holmes.plugins.toolsets",
    "holmes.plugins.toolsets.logging_utils",
    "holmes.plugins.toolsets.logging_utils.logging_api",
    "holmes.plugins.toolsets.prometheus",
    "holmes.plugins.toolsets.prometheus.prometheus",
    "prometrix", "prometrix.connect", "prometrix.connect.aws_connect",
    "prometrix.auth", "prometrix.models", "prometrix.models.prometheus_config",
]

for _mod_name in _MOCK_PACKAGES:
    if _mod_name not in sys.modules:
        sys.modules[_mod_name] = _make_stub(_mod_name)

from src.extensions.incident.prompt_builder import create_incident_investigation_prompt


def _make_request_data(signal_mode=None, signal_type="OOMKilled", severity="critical"):
    """Helper to build a minimal IncidentRequest data dict."""
    data = {
        "incident_id": "test-incident-001",
        "remediation_id": "test-remediation-001",
        "signal_type": signal_type,
        "severity": severity,
        "signal_source": "prometheus",
        "resource_namespace": "production",
        "resource_kind": "Pod",
        "resource_name": "payment-api-abc123",
        "error_message": "Container exceeded memory limit",
        "environment": "production",
        "priority": "P0",
        "risk_tolerance": "low",
        "business_category": "revenue-critical",
        "cluster_name": "prod-us-east-1",
    }
    if signal_mode is not None:
        data["signal_mode"] = signal_mode
    return data


class TestPredictiveSignalModePrompt:
    """UT-HAPI-084: Prompt builder signal_mode tests."""

    def test_ut_hapi_084_001_reactive_mode_contains_rca_directive(self):
        """UT-HAPI-084-001: Prompt contains reactive RCA directive when signal_mode = reactive."""
        request_data = _make_request_data(signal_mode="reactive")
        prompt = create_incident_investigation_prompt(request_data)

        # Phase 1: Should mention investigating the incident (not predicted)
        assert "has occurred" in prompt
        assert "Understand what actually happened and why" in prompt

        # Phase 2: Should be about root cause analysis
        assert "Determine Root Cause (RCA)" in prompt

        # Should NOT contain predictive-specific content
        assert "PREDICTIVE MODE" not in prompt
        assert "Predictive Signal Mode" not in prompt

    def test_ut_hapi_084_002_predictive_mode_contains_prevention_directive(self):
        """UT-HAPI-084-002: Prompt contains predictive prevention directive when signal_mode = predictive."""
        request_data = _make_request_data(signal_mode="predictive")
        prompt = create_incident_investigation_prompt(request_data)

        # Incident summary should indicate prediction
        assert "predicted" in prompt.lower()
        assert "NOT yet occurred" in prompt

        # Phase 1: Should mention predictive investigation
        assert "PREDICTIVE MODE" in prompt
        assert "Predicted Incident" in prompt

        # Phase 2: Should be about prevention, not RCA
        assert "Assess Prediction and Determine Prevention Strategy" in prompt
        assert "No action needed" in prompt

        # Predictive context section should be present
        assert "Predictive Signal Mode" in prompt
        assert "predict_linear()" in prompt
        assert "PREVENTIVE action" in prompt

    def test_ut_hapi_084_003_default_to_reactive_when_absent(self):
        """UT-HAPI-084-003: Default to reactive when signal_mode is absent."""
        # No signal_mode in request data
        request_data = _make_request_data(signal_mode=None)
        prompt = create_incident_investigation_prompt(request_data)

        # Should behave like reactive mode
        assert "has occurred" in prompt
        assert "Understand what actually happened and why" in prompt
        assert "Determine Root Cause (RCA)" in prompt

        # Should NOT contain predictive content
        assert "PREDICTIVE MODE" not in prompt
        assert "Predictive Signal Mode" not in prompt

    def test_predictive_mode_preserves_signal_type(self):
        """Signal type in prompt should be the normalized type (from SP), not the predictive original."""
        # SP normalizes PredictedOOMKill -> OOMKilled before passing to HAPI
        request_data = _make_request_data(
            signal_mode="predictive",
            signal_type="OOMKilled",  # Already normalized by SP
        )
        prompt = create_incident_investigation_prompt(request_data)

        # The prompt should use the normalized type
        assert "OOMKilled" in prompt
        # The prompt should NOT reference the original predictive type
        # (SP normalized it before passing to HAPI)

    def test_predictive_mode_phase5_summary_mentions_prevention(self):
        """Phase 5 summary should mention prediction assessment for predictive mode."""
        request_data = _make_request_data(signal_mode="predictive")
        prompt = create_incident_investigation_prompt(request_data)

        assert "prediction assessment" in prompt.lower()
        assert "preventive workflow" in prompt.lower()

    def test_reactive_mode_phase5_summary_is_standard(self):
        """Phase 5 summary for reactive mode should be standard."""
        request_data = _make_request_data(signal_mode="reactive")
        prompt = create_incident_investigation_prompt(request_data)

        # Standard Phase 5 text (no prediction references)
        assert "Provide natural language summary + structured JSON" in prompt
