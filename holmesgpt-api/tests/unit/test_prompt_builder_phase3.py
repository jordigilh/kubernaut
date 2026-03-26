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
Issue #537: Phase 3 (Workflow Selection) prompt and sections

Tests that Phase 3 uses a dedicated workflow selection prompt and custom
sections that align with what the HAPI result parser expects, rather than
re-sending the full multi-phase investigation prompt with the SDK's
DEFAULT_SECTIONS.

Authority: Issue #537
Business Requirement: BR-HAPI-263

Test IDs:
  UT-HAPI-537-001: Phase 3 prompt is focused on workflow selection
  UT-HAPI-537-002: Phase 3 prompt includes structured response format
  UT-HAPI-537-003: Phase 3 prompt does NOT include Phase 1 investigation instructions
  UT-HAPI-537-004: Phase 3 prompt includes three-step discovery protocol
  UT-HAPI-537-005: Phase 3 prompt includes enrichment context placeholder
  UT-HAPI-537-006: PHASE3_SECTIONS contains keys matching HAPI parser expectations
  UT-HAPI-537-007: Phase 3 prompt includes special investigation outcomes (BR-HAPI-200)
"""

import pytest


def _make_request_data(**overrides):
    """Build a minimal IncidentRequest-like dict."""
    data = {
        "incident_id": "inc-537-001",
        "signal_name": "OOMKilled",
        "severity": "high",
        "signal_source": "prometheus",
        "resource_namespace": "production",
        "resource_kind": "Pod",
        "resource_name": "api-server-abc123",
        "error_message": "Container exceeded memory limit",
        "environment": "production",
        "priority": "P1",
        "risk_tolerance": "medium",
        "business_category": "critical",
        "cluster_name": "prod-us-west-2",
    }
    data.update(overrides)
    return data


class TestPhase3WorkflowPrompt:
    """
    UT-HAPI-537-001 through UT-HAPI-537-005: Phase 3 prompt construction.
    """

    def test_ut_hapi_537_001_phase3_prompt_is_focused_on_workflow_selection(self):
        """UT-HAPI-537-001: Phase 3 prompt focuses on workflow selection, not investigation."""
        from src.extensions.incident.prompt_builder import create_phase3_workflow_prompt

        prompt = create_phase3_workflow_prompt(_make_request_data())

        assert "workflow selection" in prompt.lower() or "select.*workflow" in prompt.lower()
        assert "workflow_id" in prompt

    def test_ut_hapi_537_002_phase3_prompt_includes_structured_response_format(self):
        """UT-HAPI-537-002: Phase 3 prompt includes the expected section-header format."""
        from src.extensions.incident.prompt_builder import create_phase3_workflow_prompt

        prompt = create_phase3_workflow_prompt(_make_request_data())

        assert "# root_cause_analysis" in prompt
        assert "# selected_workflow" in prompt
        assert "# confidence" in prompt

    def test_ut_hapi_537_003_phase3_prompt_excludes_investigation_instructions(self):
        """UT-HAPI-537-003: Phase 3 prompt does NOT include Phase 1 investigation instructions."""
        from src.extensions.incident.prompt_builder import create_phase3_workflow_prompt

        prompt = create_phase3_workflow_prompt(_make_request_data())

        assert "Check pod status, events, and logs" not in prompt
        assert "Review resource usage and limits" not in prompt
        assert "Examine node conditions" not in prompt
        assert "Phase 1: Investigate the Incident" not in prompt

    def test_ut_hapi_537_004_phase3_prompt_includes_three_step_discovery(self):
        """UT-HAPI-537-004: Phase 3 prompt includes the three-step workflow discovery protocol."""
        from src.extensions.incident.prompt_builder import create_phase3_workflow_prompt

        prompt = create_phase3_workflow_prompt(_make_request_data())

        assert "list_available_actions" in prompt
        assert "list_workflows" in prompt
        assert "get_workflow" in prompt

    def test_ut_hapi_537_005_phase3_prompt_includes_investigation_outcomes(self):
        """UT-HAPI-537-005: Phase 3 prompt includes special investigation outcomes (BR-HAPI-200)."""
        from src.extensions.incident.prompt_builder import create_phase3_workflow_prompt

        prompt = create_phase3_workflow_prompt(_make_request_data())

        assert "investigation_outcome" in prompt
        assert "resolved" in prompt
        assert "inconclusive" in prompt


class TestPhase3Sections:
    """
    UT-HAPI-537-006: PHASE3_SECTIONS matches HAPI parser expectations.
    """

    def test_ut_hapi_537_006_phase3_sections_contains_expected_keys(self):
        """UT-HAPI-537-006: PHASE3_SECTIONS contains keys the HAPI result parser extracts."""
        from src.extensions.incident.prompt_builder import PHASE3_SECTIONS

        assert "root_cause_analysis" in PHASE3_SECTIONS
        assert "selected_workflow" in PHASE3_SECTIONS
        assert "confidence" in PHASE3_SECTIONS
        assert "alternative_workflows" in PHASE3_SECTIONS
        assert "investigation_outcome" in PHASE3_SECTIONS

    def test_ut_hapi_537_007_phase3_sections_does_not_contain_default_sdk_keys(self):
        """UT-HAPI-537-007: PHASE3_SECTIONS does NOT contain the SDK's DEFAULT_SECTIONS keys."""
        from src.extensions.incident.prompt_builder import PHASE3_SECTIONS

        assert "Alert Explanation" not in PHASE3_SECTIONS
        assert "Key Findings" not in PHASE3_SECTIONS
        assert "Next Steps" not in PHASE3_SECTIONS
        assert "App or Infra?" not in PHASE3_SECTIONS
