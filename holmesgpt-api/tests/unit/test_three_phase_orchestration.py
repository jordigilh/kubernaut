# Copyright 2026 Jordi Gil.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""
Unit tests for #529 Three-Phase RCA Orchestration (G5).

Business Requirements:
- BR-HAPI-263: Conversation continuity — Phase 1 analysis injected into Phase 3 prompt
- BR-HAPI-261: _inject_target_resource uses EnrichmentResult.root_owner

Design Decisions:
- DD-HAPI-002 v1.4: Three-phase loop structure (RCA, enrichment, workflow selection)
- ADR-055 v1.5: EnrichmentService is sole authoritative source for enrichment

Test Plan: docs/tests/529/TEST_PLAN.md — Group 5
Tests:
  UT-HAPI-263-003: Phase 3 prompt contains Phase 1 analysis text
  UT-HAPI-261-006: _inject_target_resource uses EnrichmentResult.root_owner
  UT-529-ORCH-001: analyze_incident calls EnrichmentService between Phase 1 and Phase 3
  UT-529-ORCH-002: Invalid affectedResource from Phase 1 triggers Phase 1 retry
"""

import json
import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from src.extensions.incident.enrichment_service import EnrichmentResult

# Common patch targets: these symbols are imported locally inside analyze_incident.
_P = "src.extensions.llm_config"
_LLM = "src.extensions.incident.llm_integration"


# ---------------------------------------------------------------------------
# Fixtures / helpers
# ---------------------------------------------------------------------------

VALID_AFFECTED_RESOURCE = {"kind": "Pod", "name": "api-xyz", "namespace": "prod"}

ENRICHMENT_RESULT = EnrichmentResult(
    root_owner={"kind": "Deployment", "name": "api", "namespace": "prod"},
    detected_labels={"component": "backend", "severity": "critical"},
    remediation_history={"entries": []},
)


def _make_result(
    rca=None,
    selected_workflow=None,
    affected_resource=None,
):
    """Build a minimal HAPI result dict."""
    result = {
        "root_cause_analysis": rca or {"summary": "OOM detected"},
        "needs_human_review": False,
    }
    if affected_resource:
        result["root_cause_analysis"]["affectedResource"] = affected_resource
    if selected_workflow is not None:
        result["selected_workflow"] = selected_workflow
    else:
        result["selected_workflow"] = None
    return result


# ===========================================================================
# UT-HAPI-261-006: _inject_target_resource uses EnrichmentResult.root_owner
# ===========================================================================

class TestInjectFromEnrichmentResult:
    """G5: _inject_target_resource rewired to EnrichmentResult (BR-HAPI-261)."""

    def test_ut_hapi_261_006_inject_uses_enrichment_result_root_owner(self):
        """UT-HAPI-261-006: _inject_target_resource reads root_owner from EnrichmentResult.

        #529: _inject_target_resource is rewired to accept an enrichment_result
        parameter. When provided, root_owner is taken from enrichment_result
        instead of session_state. session_state no longer contains root_owner
        after the G4 resource context tool refactor.
        """
        from src.extensions.incident.llm_integration import _inject_target_resource

        result = _make_result(
            rca={"summary": "OOM detected"},
            selected_workflow={"workflow_id": "oom-recovery-v1", "parameters": {}},
        )
        session_state = {}  # no root_owner in session_state (#529 G4 stripped writes)

        _inject_target_resource(
            result,
            session_state,
            "rem-261-006",
            enrichment_result=ENRICHMENT_RESULT,
        )

        params = result["selected_workflow"]["parameters"]
        assert params["TARGET_RESOURCE_NAME"] == "api", "Should use EnrichmentResult root_owner name"
        assert params["TARGET_RESOURCE_KIND"] == "Deployment", "Should use EnrichmentResult root_owner kind"
        assert params["TARGET_RESOURCE_NAMESPACE"] == "prod", "Should use EnrichmentResult root_owner namespace"

        ar = result["root_cause_analysis"]["affectedResource"]
        assert ar["kind"] == "Deployment"
        assert ar["name"] == "api"
        assert ar["namespace"] == "prod"
        assert result.get("needs_human_review") is False, "Should not flag rca_incomplete"

    def test_ut_hapi_261_006b_inject_cluster_scoped_from_enrichment_result(self):
        """UT-HAPI-261-006b: Cluster-scoped EnrichmentResult root_owner omits namespace.

        #529: When EnrichmentResult.root_owner has no namespace (cluster-scoped),
        TARGET_RESOURCE_NAMESPACE is not injected and affectedResource has no namespace.
        """
        from src.extensions.incident.llm_integration import _inject_target_resource

        cluster_enrichment = EnrichmentResult(
            root_owner={"kind": "Node", "name": "worker-1"},
            detected_labels=None,
            remediation_history=None,
        )
        result = _make_result(
            selected_workflow={"workflow_id": "drain-node-v1", "parameters": {}},
        )
        session_state = {}

        _inject_target_resource(
            result,
            session_state,
            "rem-261-006b",
            enrichment_result=cluster_enrichment,
        )

        params = result["selected_workflow"]["parameters"]
        assert params["TARGET_RESOURCE_NAME"] == "worker-1"
        assert params["TARGET_RESOURCE_KIND"] == "Node"
        assert "TARGET_RESOURCE_NAMESPACE" not in params

    def test_ut_hapi_261_006c_inject_rca_incomplete_when_no_enrichment_result(self):
        """UT-HAPI-261-006c: rca_incomplete when enrichment_result is None and session_state empty.

        #529: With session_state writes stripped (G4), if no enrichment_result
        is provided and session_state has no root_owner, result is rca_incomplete.
        """
        from src.extensions.incident.llm_integration import _inject_target_resource

        result = _make_result(
            selected_workflow={"workflow_id": "oom-recovery-v1", "parameters": {}},
        )
        session_state = {}

        _inject_target_resource(result, session_state, "rem-261-006c", enrichment_result=None)

        assert result["needs_human_review"] is True
        assert result["human_review_reason"] == "rca_incomplete"


# ===========================================================================
# UT-HAPI-263-003: Phase 3 prompt contains Phase 1 analysis text
# ===========================================================================

class TestConversationContinuityOrchestration:
    """G5: Prompt-based conversation continuity at HAPI level (BR-HAPI-263)."""

    @pytest.mark.asyncio
    async def test_ut_hapi_263_003_phase3_prompt_contains_phase1_analysis(self):
        """UT-HAPI-263-003: Phase 3 prompt contains Phase 1 analysis text.

        BR-HAPI-263: analyze_incident must capture the Phase 1 analysis text
        and inject it into the Phase 3 prompt so the LLM has RCA context when
        selecting a workflow.
        """
        from holmes.core.models import InvestigationResult

        phase1_analysis_text = json.dumps({
            "root_cause_analysis": {
                "summary": "OOM detected",
                "affectedResource": VALID_AFFECTED_RESOURCE,
            },
        })
        phase1_result = InvestigationResult(analysis=phase1_analysis_text)
        phase3_result = InvestigationResult(
            analysis=json.dumps({
                "root_cause_analysis": {"summary": "OOM detected"},
                "selected_workflow": {
                    "workflow_id": "oom-recovery-v1",
                    "action_type": "IncreaseMemoryLimits",
                    "version": "1.0.0",
                    "confidence": 0.9,
                    "rationale": "OOM recovery",
                    "execution_engine": "tekton",
                    "parameters": {"MEMORY_LIMIT_NEW": "512Mi"},
                },
            }),
        )

        phase3_prompts = []

        def mock_investigate(investigate_request, dal, config):
            phase = investigate_request.context.get("phase")
            if phase == 3:
                phase3_prompts.append(investigate_request.description)
            return phase1_result if phase == 1 else phase3_result

        mock_enrich = AsyncMock(return_value=ENRICHMENT_RESULT)

        with patch(f"{_LLM}.investigate_issues", side_effect=mock_investigate), \
             patch(f"{_LLM}.EnrichmentService") as MockES, \
             patch(f"{_LLM}.get_audit_store") as mock_audit, \
             patch(f"{_LLM}.create_data_storage_client", return_value=None), \
             patch(f"{_P}.get_model_config_for_sdk", return_value=("mock-model", "openai")), \
             patch(f"{_P}.prepare_toolsets_config_for_sdk", return_value={}), \
             patch(f"{_P}.register_workflow_discovery_toolset", side_effect=lambda c, *a, **kw: c), \
             patch(f"{_P}.register_resource_context_toolset", side_effect=lambda c, *a, **kw: c), \
             patch("src.sanitization.sanitize_for_llm", side_effect=lambda x: x), \
             patch(f"{_LLM}.parse_and_validate_investigation_result") as mock_parse:

            mock_audit.return_value = MagicMock()
            MockES.return_value.enrich = mock_enrich
            mock_parse.return_value = (
                json.loads(phase3_result.analysis),
                MagicMock(is_valid=True, errors=[], parameter_schema=None, schema_hint=None),
            )

            from src.extensions.incident.llm_integration import analyze_incident
            await analyze_incident(
                {"incident_id": "inc-263-003", "signal_name": "OOMKill", "remediation_id": "rem-001"},
                app_config={},
            )

        assert len(phase3_prompts) >= 1, "Phase 3 prompt should have been captured"
        prompt = phase3_prompts[0]
        assert "Phase 1 Root Cause Analysis" in prompt, (
            "Phase 3 prompt must contain the Phase 1 RCA section header"
        )
        assert "OOM detected" in prompt, (
            "Phase 3 prompt must contain Phase 1 analysis content"
        )


# ===========================================================================
# UT-529-ORCH-001: analyze_incident calls EnrichmentService between phases
# ===========================================================================

class TestThreePhaseFlow:
    """G5: Three-phase orchestration flow (#529)."""

    @pytest.mark.asyncio
    async def test_ut_529_orch_001_enrichment_called_between_phases(self):
        """UT-529-ORCH-001: analyze_incident calls EnrichmentService between Phase 1 and Phase 3.

        #529: After Phase 1 returns RCA with affectedResource, HAPI must call
        EnrichmentService.enrich(affectedResource) before issuing Phase 3.
        """
        from holmes.core.models import InvestigationResult

        phase1_result = InvestigationResult(
            analysis=json.dumps({
                "root_cause_analysis": {
                    "summary": "OOM detected",
                    "affectedResource": VALID_AFFECTED_RESOURCE,
                },
            }),
        )
        phase3_result = InvestigationResult(
            analysis=json.dumps({
                "root_cause_analysis": {"summary": "OOM detected"},
                "selected_workflow": {
                    "workflow_id": "oom-recovery-v1",
                    "action_type": "IncreaseMemoryLimits",
                    "version": "1.0.0",
                    "confidence": 0.9,
                    "rationale": "OOM recovery",
                    "execution_engine": "tekton",
                    "parameters": {"MEMORY_LIMIT_NEW": "512Mi"},
                },
            }),
        )

        call_sequence = []

        def mock_investigate(investigate_request, dal, config):
            call_sequence.append("investigate")
            if len([c for c in call_sequence if c == "investigate"]) == 1:
                return phase1_result
            return phase3_result

        async def mock_enrich(affected_resource):
            call_sequence.append("enrich")
            assert affected_resource == VALID_AFFECTED_RESOURCE, (
                "EnrichmentService must receive the LLM-provided affectedResource"
            )
            return ENRICHMENT_RESULT

        with patch(f"{_LLM}.investigate_issues", side_effect=mock_investigate), \
             patch(f"{_LLM}.EnrichmentService") as MockES, \
             patch(f"{_LLM}.get_audit_store") as mock_audit, \
             patch(f"{_LLM}.create_data_storage_client", return_value=None), \
             patch(f"{_P}.get_model_config_for_sdk", return_value=("mock-model", "openai")), \
             patch(f"{_P}.prepare_toolsets_config_for_sdk", return_value={}), \
             patch(f"{_P}.register_workflow_discovery_toolset", side_effect=lambda c, *a, **kw: c), \
             patch(f"{_P}.register_resource_context_toolset", side_effect=lambda c, *a, **kw: c), \
             patch("src.sanitization.sanitize_for_llm", side_effect=lambda x: x), \
             patch(f"{_LLM}.parse_and_validate_investigation_result") as mock_parse:

            mock_audit.return_value = MagicMock()
            MockES.return_value.enrich = mock_enrich
            mock_parse.return_value = (
                json.loads(phase3_result.analysis),
                MagicMock(is_valid=True, errors=[], parameter_schema=None, schema_hint=None),
            )

            from src.extensions.incident.llm_integration import analyze_incident
            await analyze_incident(
                {"incident_id": "inc-orch-001", "signal_name": "OOMKill", "remediation_id": "rem-001"},
                app_config={},
            )

        assert call_sequence == ["investigate", "enrich", "investigate"], (
            f"Expected Phase 1 (investigate) -> Phase 2 (enrich) -> Phase 3 (investigate), "
            f"got {call_sequence}"
        )

    @pytest.mark.asyncio
    async def test_ut_529_orch_002_invalid_affected_resource_retries_phase1(self):
        """UT-529-ORCH-002: Invalid affectedResource from Phase 1 triggers Phase 1 retry.

        #529: If Phase 1 returns an invalid or missing affectedResource, HAPI
        must retry Phase 1 (within the shared retry budget) instead of proceeding
        to Phase 2.
        """
        from holmes.core.models import InvestigationResult

        phase1_no_ar = InvestigationResult(
            analysis=json.dumps({
                "root_cause_analysis": {"summary": "OOM detected"},
            }),
        )
        phase1_with_ar = InvestigationResult(
            analysis=json.dumps({
                "root_cause_analysis": {
                    "summary": "OOM detected",
                    "affectedResource": VALID_AFFECTED_RESOURCE,
                },
            }),
        )
        phase3_result = InvestigationResult(
            analysis=json.dumps({
                "root_cause_analysis": {"summary": "OOM detected"},
                "selected_workflow": {
                    "workflow_id": "oom-recovery-v1",
                    "action_type": "IncreaseMemoryLimits",
                    "version": "1.0.0",
                    "confidence": 0.9,
                    "rationale": "OOM recovery",
                    "execution_engine": "tekton",
                    "parameters": {"MEMORY_LIMIT_NEW": "512Mi"},
                },
            }),
        )

        investigate_call_count = 0

        def mock_investigate(investigate_request, dal, config):
            nonlocal investigate_call_count
            investigate_call_count += 1
            if investigate_call_count == 1:
                return phase1_no_ar  # First Phase 1: no affectedResource
            elif investigate_call_count == 2:
                return phase1_with_ar  # Second Phase 1: valid affectedResource
            return phase3_result  # Phase 3

        mock_enrich = AsyncMock(return_value=ENRICHMENT_RESULT)

        with patch(f"{_LLM}.investigate_issues", side_effect=mock_investigate), \
             patch(f"{_LLM}.EnrichmentService") as MockES, \
             patch(f"{_LLM}.get_audit_store") as mock_audit, \
             patch(f"{_LLM}.create_data_storage_client", return_value=None), \
             patch(f"{_P}.get_model_config_for_sdk", return_value=("mock-model", "openai")), \
             patch(f"{_P}.prepare_toolsets_config_for_sdk", return_value={}), \
             patch(f"{_P}.register_workflow_discovery_toolset", side_effect=lambda c, *a, **kw: c), \
             patch(f"{_P}.register_resource_context_toolset", side_effect=lambda c, *a, **kw: c), \
             patch("src.sanitization.sanitize_for_llm", side_effect=lambda x: x), \
             patch(f"{_LLM}.parse_and_validate_investigation_result") as mock_parse:

            mock_audit.return_value = MagicMock()
            MockES.return_value.enrich = mock_enrich
            mock_parse.return_value = (
                json.loads(phase3_result.analysis),
                MagicMock(is_valid=True, errors=[], parameter_schema=None, schema_hint=None),
            )

            from src.extensions.incident.llm_integration import analyze_incident
            await analyze_incident(
                {"incident_id": "inc-orch-002", "signal_name": "OOMKill", "remediation_id": "rem-001"},
                app_config={},
            )

        assert investigate_call_count == 3, (
            f"Expected 3 investigate_issues calls (retry Phase 1 + Phase 1 + Phase 3), "
            f"got {investigate_call_count}"
        )
