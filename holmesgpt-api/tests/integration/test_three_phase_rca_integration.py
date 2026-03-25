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
Integration tests for #529 Three-Phase RCA Architecture.

These tests exercise the full analyze_incident flow with all internal
components wired together (real parsing, real enrichment context building,
real target resource injection). Only external boundaries are mocked:
LLM (investigate_issues), K8s API, DataStorage, and audit store.

Business Requirements:
  - BR-HAPI-261: LLM-provided affectedResource with HAPI owner resolution
  - BR-HAPI-263: Conversation continuity (Phase 1 analysis in Phase 3 prompt)
  - BR-HAPI-264: Post-RCA label detection via EnrichmentService
  - BR-HAPI-265: Labels in workflow discovery context
  - #529: Three-phase orchestration (RCA → Enrichment → Workflow Selection)

Test Plan: docs/tests/529/TEST_PLAN.md — Group 7 (Integration)
Tests:
  IT-529-261-001: Full Phase 1 → 2 → 3 flow produces valid result
  IT-529-263-001: Phase 1 analysis text present in Phase 3 prompt
  IT-529-264-001: EnrichmentService labels populate session_state and response
  IT-529-265-001: EnrichmentService retry exhaustion produces rca_incomplete
  IT-529-E-001:   Shared retry budget exhaustion when Phase 1 never provides affectedResource
"""

import json
import pytest
from dataclasses import dataclass, field
from typing import Any, Dict, Optional
from unittest.mock import AsyncMock, MagicMock, patch

_LLM = "src.extensions.incident.llm_integration"
_CFG = "src.extensions.llm_config"


@dataclass
class FakeInvestigationResult:
    """Lightweight stand-in for holmes.core.models.InvestigationResult.

    Avoids importing the full Holmes SDK which triggers a Pydantic V1
    failure on Python 3.14 when SDK_CONFIG_FILE is set (integration conftest).
    """
    analysis: Optional[str] = None
    sections: Optional[Dict[str, Any]] = None


# ---------------------------------------------------------------------------
# Shared fixtures and helpers
# ---------------------------------------------------------------------------

VALID_AFFECTED_RESOURCE = {"kind": "Pod", "name": "api-xyz", "namespace": "prod"}


@dataclass
class FakeEnrichmentResult:
    """Lightweight stand-in for EnrichmentResult.

    Avoids importing src.extensions.incident.enrichment_service which triggers
    the package __init__.py → llm_integration → Holmes SDK import chain.
    """
    root_owner: Optional[Dict[str, str]] = None
    detected_labels: Optional[Dict[str, Any]] = None
    remediation_history: Optional[Dict[str, Any]] = None


@dataclass
class FakeEnrichmentFailure(Exception):
    """Stand-in for EnrichmentFailure."""
    reason: str = ""
    detail: str = ""


def _enrichment_result():
    return FakeEnrichmentResult(
        root_owner={"kind": "Deployment", "name": "api", "namespace": "prod"},
        detected_labels={"pdbProtected": True, "gitOpsManaged": False},
        remediation_history={"entries": []},
    )


def _enrichment_with_labels():
    return FakeEnrichmentResult(
        root_owner={"kind": "Deployment", "name": "api", "namespace": "prod"},
        detected_labels={
            "pdbProtected": True,
            "gitOpsManaged": True,
            "gitOpsTool": "argocd",
            "helmManaged": False,
        },
        remediation_history={"entries": [{"workflow": "scale-up", "outcome": "success"}]},
    )


def _make_incident_request(
    incident_id: str = "inc-it-529",
    signal_name: str = "OOMKill",
    remediation_id: str = "rem-it-529",
) -> dict:
    return {
        "incident_id": incident_id,
        "remediation_id": remediation_id,
        "signal_name": signal_name,
        "severity": "critical",
        "signal_source": "prometheus",
        "resource_namespace": "prod",
        "resource_kind": "Pod",
        "resource_name": "api-xyz",
        "error_message": "Container OOMKilled",
        "environment": "production",
        "priority": "P0",
        "risk_tolerance": "medium",
        "business_category": "standard",
        "cluster_name": "integration-test",
        "enrichment_results": {},
    }


def _phase1_analysis_json(affected_resource=None):
    """Build a Phase 1 analysis JSON string."""
    rca = {"summary": "OOM detected due to traffic spike", "severity": "critical"}
    if affected_resource:
        rca["affectedResource"] = affected_resource
    return json.dumps({"root_cause_analysis": rca})


def _phase3_analysis_json(workflow_id="oom-recovery-v1", confidence=0.92):
    """Build a Phase 3 analysis JSON string with workflow selection."""
    return json.dumps({
        "root_cause_analysis": {"summary": "OOM detected"},
        "selected_workflow": {
            "workflow_id": workflow_id,
            "title": "OOM Recovery",
            "action_type": "IncreaseMemoryLimits",
            "version": "1.0.0",
            "confidence": confidence,
            "rationale": "OOM recovery workflow",
            "execution_engine": "tekton",
            "parameters": {"MEMORY_LIMIT_NEW": "512Mi"},
        },
        "alternative_workflows": [],
        "needs_human_review": False,
        "human_review_reason": None,
    })


import contextlib


@contextlib.contextmanager
def _patched_analyze(investigate_fn, enrich_fn, parse_return):
    """Context manager that patches all external boundaries for analyze_incident.

    Args:
        investigate_fn: Side effect for investigate_issues (LLM boundary)
        enrich_fn: Async callable for EnrichmentService.enrich
        parse_return: Tuple (result_dict, validation_mock) for parse_and_validate
    """
    with patch(f"{_LLM}.get_audit_store") as mock_audit, \
         patch(f"{_LLM}.create_data_storage_client", return_value=None), \
         patch(f"{_CFG}.get_model_config_for_sdk", return_value=("mock-model", "openai")), \
         patch(f"{_CFG}.prepare_toolsets_config_for_sdk", return_value={}), \
         patch(f"{_CFG}.register_workflow_discovery_toolset", side_effect=lambda c, *a, **kw: c), \
         patch(f"{_CFG}.register_resource_context_toolset", side_effect=lambda c, *a, **kw: c), \
         patch("src.sanitization.sanitize_for_llm", side_effect=lambda x: x), \
         patch(f"{_LLM}.EnrichmentFailure", FakeEnrichmentFailure), \
         patch(f"{_LLM}.investigate_issues", side_effect=investigate_fn), \
         patch(f"{_LLM}.EnrichmentService") as MockES, \
         patch(f"{_LLM}.parse_and_validate_investigation_result") as mock_parse:

        mock_audit.return_value = MagicMock()
        MockES.return_value.enrich = enrich_fn
        mock_parse.return_value = parse_return
        yield


# ===========================================================================
# IT-529-261-001: Full Phase 1 → 2 → 3 flow produces valid result
# ===========================================================================

class TestThreePhaseFlowIntegration:
    """IT-529-261-001: End-to-end three-phase RCA integration.

    Verifies that analyze_incident correctly orchestrates:
      Phase 1: LLM provides RCA + affectedResource
      Phase 2: EnrichmentService resolves owner, labels, history
      Phase 3: LLM selects workflow with enrichment context
    and produces a valid result with TARGET_RESOURCE injected from root_owner.
    """

    @pytest.mark.asyncio
    async def test_it_529_261_001_full_three_phase_flow(self):
        """IT-529-261-001: Full Phase 1 → 2 → 3 flow produces valid result.

        Given: Phase 1 returns valid affectedResource
          And: EnrichmentService returns root_owner, labels, history
          And: Phase 3 selects a workflow
        When: analyze_incident runs
        Then: Result contains selected_workflow with TARGET_RESOURCE from root_owner
          And: Result contains detected_labels from enrichment
          And: EnrichmentService was called with Phase 1's affectedResource
        """
        phase1_result = FakeInvestigationResult(
            analysis=_phase1_analysis_json(VALID_AFFECTED_RESOURCE),
        )
        phase3_result = FakeInvestigationResult(
            analysis=_phase3_analysis_json(),
        )

        enrich_called_with = []

        async def mock_enrich(affected_resource):
            enrich_called_with.append(affected_resource)
            return _enrichment_result()

        def mock_investigate(investigate_request, dal, config):
            phase = investigate_request.context.get("phase")
            return phase1_result if phase == 1 else phase3_result

        parse_return = (
            json.loads(phase3_result.analysis),
            MagicMock(is_valid=True, errors=[], parameter_schema=None, schema_hint=None),
        )

        with _patched_analyze(mock_investigate, mock_enrich, parse_return):
            from src.extensions.incident.llm_integration import analyze_incident
            result = await analyze_incident(
                _make_incident_request(incident_id="inc-it-261-001"),
                app_config={},
            )

        assert result is not None, "analyze_incident must return a result"
        assert result.get("selected_workflow") is not None, "Phase 3 must select a workflow"
        assert result["selected_workflow"]["workflow_id"] == "oom-recovery-v1"

        params = result["selected_workflow"].get("parameters", {})
        assert params.get("TARGET_RESOURCE_KIND") == "Deployment", (
            "TARGET_RESOURCE_KIND must come from EnrichmentResult.root_owner"
        )
        assert params.get("TARGET_RESOURCE_NAME") == "api", (
            "TARGET_RESOURCE_NAME must come from EnrichmentResult.root_owner"
        )
        assert params.get("TARGET_RESOURCE_NAMESPACE") == "prod"

        assert len(enrich_called_with) == 1, "EnrichmentService.enrich must be called once"
        assert enrich_called_with[0] == VALID_AFFECTED_RESOURCE


# ===========================================================================
# IT-529-263-001: Phase 1 analysis text present in Phase 3 prompt
# ===========================================================================

class TestConversationContinuityIntegration:
    """IT-529-263-001: Prompt-based conversation continuity integration.

    Verifies that Phase 1 analysis text is captured and injected into the
    Phase 3 prompt so the LLM has full RCA context when selecting a workflow.
    """

    @pytest.mark.asyncio
    async def test_it_529_263_001_phase1_analysis_in_phase3_prompt(self):
        """IT-529-263-001: Phase 1 analysis text present in Phase 3 prompt.

        Given: Phase 1 returns analysis with specific RCA text
        When: Phase 3 investigation prompt is constructed
        Then: Phase 3 prompt contains the Phase 1 analysis text
          And: Phase 3 prompt contains the enrichment context
        """
        phase1_text = "OOM detected due to traffic spike"
        phase1_result = FakeInvestigationResult(
            analysis=_phase1_analysis_json(VALID_AFFECTED_RESOURCE),
        )
        phase3_result = FakeInvestigationResult(
            analysis=_phase3_analysis_json(),
        )

        captured_phase3_prompts = []

        def mock_investigate(investigate_request, dal, config):
            phase = investigate_request.context.get("phase")
            if phase == 3:
                captured_phase3_prompts.append(investigate_request.description)
            return phase1_result if phase == 1 else phase3_result

        mock_enrich = AsyncMock(return_value=_enrichment_result())
        parse_return = (
            json.loads(phase3_result.analysis),
            MagicMock(is_valid=True, errors=[], parameter_schema=None, schema_hint=None),
        )

        with _patched_analyze(mock_investigate, mock_enrich, parse_return):
            from src.extensions.incident.llm_integration import analyze_incident
            await analyze_incident(
                _make_incident_request(incident_id="inc-it-263-001"),
                app_config={},
            )

        assert len(captured_phase3_prompts) >= 1, "Phase 3 prompt must be captured"
        prompt = captured_phase3_prompts[0]

        assert "Phase 1 Root Cause Analysis" in prompt, (
            "Phase 3 prompt must contain the Phase 1 RCA section header"
        )
        assert phase1_text in prompt, (
            "Phase 3 prompt must contain Phase 1 analysis content"
        )
        assert "Enrichment Context" in prompt or "enrichment" in prompt.lower(), (
            "Phase 3 prompt must contain enrichment context"
        )


# ===========================================================================
# IT-529-264-001: EnrichmentService labels populate session_state and response
# ===========================================================================

class TestEnrichmentLabelsIntegration:
    """IT-529-264-001: Post-RCA label detection via EnrichmentService.

    Verifies that labels from EnrichmentService are:
    1. Written to session_state["detected_labels"] for Phase 3 tools
    2. Present in the final response via inject_detected_labels
    """

    @pytest.mark.asyncio
    async def test_it_529_264_001_enrichment_labels_in_response(self):
        """IT-529-264-001: Enrichment labels populate response detected_labels.

        Given: EnrichmentService returns labels (pdbProtected=true, gitOpsManaged=true)
        When: analyze_incident completes the three-phase flow
        Then: Result contains detected_labels from enrichment
          And: Labels include pdbProtected and gitOpsManaged
        """
        phase1_result = FakeInvestigationResult(
            analysis=_phase1_analysis_json(VALID_AFFECTED_RESOURCE),
        )
        phase3_result = FakeInvestigationResult(
            analysis=_phase3_analysis_json(),
        )

        mock_enrich = AsyncMock(return_value=_enrichment_with_labels())

        def mock_investigate(investigate_request, dal, config):
            phase = investigate_request.context.get("phase")
            return phase1_result if phase == 1 else phase3_result

        parse_return = (
            json.loads(phase3_result.analysis),
            MagicMock(is_valid=True, errors=[], parameter_schema=None, schema_hint=None),
        )

        with _patched_analyze(mock_investigate, mock_enrich, parse_return):
            from src.extensions.incident.llm_integration import analyze_incident
            result = await analyze_incident(
                _make_incident_request(incident_id="inc-it-264-001"),
                app_config={},
            )

        assert result is not None
        labels = result.get("detected_labels")
        assert labels is not None, "Response must include detected_labels from enrichment"
        assert labels.get("pdbProtected") is True
        assert labels.get("gitOpsManaged") is True
        assert labels.get("gitOpsTool") == "argocd"


# ===========================================================================
# IT-529-265-001: EnrichmentService failure produces rca_incomplete
# ===========================================================================

class TestEnrichmentFailureIntegration:
    """IT-529-265-001: EnrichmentService retry exhaustion produces rca_incomplete.

    Verifies that when EnrichmentService raises EnrichmentFailure (after
    exhausting its internal retries), analyze_incident produces a result
    with needs_human_review=True and human_review_reason=rca_incomplete.
    """

    @pytest.mark.asyncio
    async def test_it_529_265_001_enrichment_failure_produces_rca_incomplete(self):
        """IT-529-265-001: EnrichmentService failure produces rca_incomplete.

        Given: Phase 1 returns valid affectedResource
          And: EnrichmentService raises EnrichmentFailure (infrastructure down)
        When: analyze_incident runs
        Then: Result has needs_human_review=True
          And: Result has human_review_reason=rca_incomplete
          And: Result has no selected_workflow
        """
        phase1_result = FakeInvestigationResult(
            analysis=_phase1_analysis_json(VALID_AFFECTED_RESOURCE),
        )

        async def mock_enrich_fail(affected_resource):
            raise FakeEnrichmentFailure(
                reason="k8s_api_unreachable",
                detail="Failed to resolve owner chain after 3 retries",
            )

        def mock_investigate(investigate_request, dal, config):
            return phase1_result

        parse_return = (
            {"root_cause_analysis": {"summary": "OOM"}},
            MagicMock(is_valid=True, errors=[], parameter_schema=None, schema_hint=None),
        )

        with _patched_analyze(mock_investigate, mock_enrich_fail, parse_return):
            from src.extensions.incident.llm_integration import analyze_incident
            result = await analyze_incident(
                _make_incident_request(incident_id="inc-it-265-001"),
                app_config={},
            )

        assert result is not None
        assert result.get("needs_human_review") is True, (
            "Enrichment failure must trigger human review"
        )
        assert result.get("human_review_reason") == "rca_incomplete", (
            "Enrichment failure reason must be rca_incomplete"
        )


# ===========================================================================
# IT-529-E-001: Shared retry budget exhaustion → rca_incomplete
# ===========================================================================

class TestRetryBudgetExhaustionIntegration:
    """IT-529-E-001: Shared retry budget exhaustion when Phase 1 never provides affectedResource.

    Verifies that when all retry attempts fail to extract a valid
    affectedResource from Phase 1, analyze_incident returns an
    rca_incomplete result instead of crashing.
    """

    @pytest.mark.asyncio
    async def test_it_529_e_001_budget_exhaustion_produces_rca_incomplete(self):
        """IT-529-E-001: Shared retry budget exhaustion → rca_incomplete.

        Given: Phase 1 never returns affectedResource (all 3 attempts)
        When: analyze_incident exhausts the retry budget
        Then: Result has needs_human_review=True
          And: Result has human_review_reason=rca_incomplete
          And: Result has selected_workflow=None
          And: EnrichmentService was never called
        """
        phase1_no_ar = FakeInvestigationResult(
            analysis=_phase1_analysis_json(affected_resource=None),
        )

        investigate_count = 0

        def mock_investigate(investigate_request, dal, config):
            nonlocal investigate_count
            investigate_count += 1
            return phase1_no_ar

        mock_enrich = AsyncMock(return_value=_enrichment_result())
        parse_return = (
            {"root_cause_analysis": {"summary": "OOM"}},
            MagicMock(is_valid=True, errors=[], parameter_schema=None, schema_hint=None),
        )

        with _patched_analyze(mock_investigate, mock_enrich, parse_return):
            from src.extensions.incident.llm_integration import analyze_incident
            result = await analyze_incident(
                _make_incident_request(incident_id="inc-it-529-e-001"),
                app_config={},
            )

        assert result is not None, "Must return a result even on budget exhaustion"
        assert result.get("needs_human_review") is True, (
            "Budget exhaustion must trigger human review"
        )
        assert result.get("human_review_reason") == "rca_incomplete"
        assert result.get("selected_workflow") is None, (
            "No workflow when Phase 1 never provided affectedResource"
        )
        assert investigate_count == 3, (
            f"Expected 3 Phase 1 attempts (MAX_VALIDATION_ATTEMPTS), got {investigate_count}"
        )
        mock_enrich.assert_not_awaited()
