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
Unit Tests: Anti-Confirmation-Bias Investigation Guardrails
BR-HAPI-214: Anti-Confirmation-Bias Investigation Guardrails
Issue #462: Forward RR.spec.signalAnnotations to HAPI + anti-confirmation-bias guardrail

Tests that create_incident_investigation_prompt() includes generic guardrails
preventing premature LLM conclusions (resolved/not-actionable) without thorough
verification.
"""

import pytest
import sys
import types
import importlib
from pathlib import Path

# ---------------------------------------------------------------------------
# Import prompt_builder WITHOUT triggering __init__.py's heavy endpoint/auth
# import chain.  prompt_builder only depends on src.models.incident_models and
# .constants — we pre-register the package in sys.modules so Python skips
# __init__.py, then import the module directly.
# ---------------------------------------------------------------------------

def _make_stub(name):
    """Create a stub module that acts as a package and returns MagicMock for any attribute."""
    mod = types.ModuleType(name)
    mod.__path__ = []
    mod.__package__ = name
    mod.__spec__ = importlib.machinery.ModuleSpec(name, None, is_package=True)

    from unittest.mock import MagicMock
    _sentinel = MagicMock()
    mod.__getattr__ = lambda attr: _sentinel
    return mod


# Stub holmes/prometrix to avoid pydantic v1 + Python 3.14 crash
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

# Pre-register the incident package so __init__.py is NOT executed during our
# import. This avoids the endpoint -> middleware -> kubernetes -> litellm chain.
# After importing prompt_builder, remove the stub so other test files can import
# normally through __init__.py.
_pkg_name = "src.extensions.incident"
_had_pkg = _pkg_name in sys.modules
_orig_pkg = sys.modules.get(_pkg_name)

if not _had_pkg:
    _pkg = types.ModuleType(_pkg_name)
    _pkg.__path__ = [str(Path(__file__).resolve().parents[2] / "src" / "extensions" / "incident")]
    _pkg.__package__ = _pkg_name
    _pkg.__spec__ = importlib.machinery.ModuleSpec(_pkg_name, None, is_package=True)
    sys.modules[_pkg_name] = _pkg

from src.extensions.incident.prompt_builder import create_incident_investigation_prompt

# Restore original state so other test modules that import from
# src.extensions.incident (with the full __init__.py) are not broken.
if not _had_pkg:
    del sys.modules[_pkg_name]
elif _orig_pkg is not None:
    sys.modules[_pkg_name] = _orig_pkg


def _make_request_data(signal_mode=None):
    """Build a minimal IncidentRequest data dict for guardrail tests."""
    data = {
        "incident_id": "test-guardrail-001",
        "remediation_id": "test-remediation-001",
        "signal_name": "OOMKilled",
        "severity": "critical",
        "signal_source": "prometheus",
        "resource_namespace": "production",
        "resource_kind": "Pod",
        "resource_name": "storage-api-789",
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


class TestInvestigationGuardrails:
    """BR-HAPI-214: Anti-confirmation-bias investigation guardrails."""

    def test_ut_hapi_214_001_guardrail_section_present_in_reactive_prompt(self):
        """UT-HAPI-214-001: Reactive investigation prompt includes guardrails."""
        prompt = create_incident_investigation_prompt(_make_request_data())
        assert "## Investigation Guardrails" in prompt

    def test_ut_hapi_214_002_guardrail_section_present_in_proactive_prompt(self):
        """UT-HAPI-214-002: Proactive investigation prompt includes guardrails."""
        prompt = create_incident_investigation_prompt(
            _make_request_data(signal_mode="proactive")
        )
        assert "## Investigation Guardrails" in prompt

    def test_ut_hapi_214_003_pre_conclusion_gate_in_outcome_a(self):
        """UT-HAPI-214-003: Outcome A (resolved) includes pre-conclusion verification gate."""
        prompt = create_incident_investigation_prompt(_make_request_data())

        outcome_a_start = prompt.index("Outcome A")
        outcome_b_start = prompt.index("Outcome B")
        outcome_a_section = prompt[outcome_a_start:outcome_b_start]

        assert "GUARDRAIL CHECK" in outcome_a_section

    def test_ut_hapi_214_004_pre_conclusion_gate_in_outcome_d(self):
        """UT-HAPI-214-004: Outcome D (not actionable) includes pre-conclusion verification gate."""
        prompt = create_incident_investigation_prompt(_make_request_data())

        outcome_d_start = prompt.index("Outcome D")
        severity_start = prompt.index("## RCA Severity Assessment")
        outcome_d_section = prompt[outcome_d_start:severity_start]

        assert "GUARDRAIL CHECK" in outcome_d_section

    def test_ut_hapi_214_005_exhaustive_verification_mandate(self):
        """UT-HAPI-214-005: Guardrail mandates exhaustive resource verification."""
        prompt = create_incident_investigation_prompt(_make_request_data())

        guardrail_start = prompt.index("## Investigation Guardrails")
        # Guardrail section ends at next ## heading
        next_section = prompt.index("###", guardrail_start + 1)
        guardrail_section = prompt[guardrail_start:next_section]

        assert "Exhaustive Verification" in guardrail_section

    def test_ut_hapi_214_006_contradicting_evidence_search(self):
        """UT-HAPI-214-006: Guardrail requires contradicting evidence search."""
        prompt = create_incident_investigation_prompt(_make_request_data())

        guardrail_start = prompt.index("## Investigation Guardrails")
        next_section = prompt.index("###", guardrail_start + 1)
        guardrail_section = prompt[guardrail_start:next_section]

        assert "Contradicting Evidence" in guardrail_section
