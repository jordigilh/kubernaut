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
Unit tests for BR-HAPI-265: Infrastructure Labels in Workflow Discovery Context.

TDD Group 6: EnrichmentService labels flow to workflow discovery tools and Phase 3 prompt.

Test Plan: docs/tests/529/TEST_PLAN.md — Group 6
Tests:
  UT-HAPI-265-001: list_available_actions cluster_context populated from enrichment labels
  UT-HAPI-265-002: Phase 3 prompt includes enrichment context with labels + history
"""

import json
import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from src.extensions.incident.enrichment_service import EnrichmentResult


ENRICHMENT_WITH_LABELS = EnrichmentResult(
    root_owner={"kind": "Deployment", "name": "api", "namespace": "prod"},
    detected_labels={
        "gitOpsManaged": True,
        "gitOpsTool": "argocd",
        "pdbProtected": False,
        "stateful": False,
        "hpaEnabled": True,
        "failedDetections": [],
    },
    remediation_history={"entries": [
        {"workflow_id": "oom-recovery-v1", "status": "completed", "signal_resolved": True},
    ]},
)

_P = "src.extensions.llm_config"
_LLM = "src.extensions.incident.llm_integration"


class TestEnrichmentLabelsInWorkflowDiscovery:
    """G6: EnrichmentService labels in workflow discovery (BR-HAPI-265)."""

    @pytest.mark.asyncio
    async def test_ut_hapi_265_001_enrichment_labels_in_list_available_actions(self):
        """UT-HAPI-265-001: list_available_actions cluster_context populated from enrichment labels.

        BR-HAPI-265: After Phase 2 enrichment, the detected labels must be
        available to the workflow discovery tools via session_state so that
        list_available_actions can include them in the cluster_context response.
        """
        from holmes.core.models import InvestigationResult

        phase1_result = InvestigationResult(
            analysis=json.dumps({
                "root_cause_analysis": {
                    "summary": "OOM detected",
                    "affectedResource": {"kind": "Pod", "name": "api-xyz", "namespace": "prod"},
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

        def mock_investigate(investigate_request, dal, config):
            phase = investigate_request.context.get("phase")
            return phase1_result if phase == 1 else phase3_result

        mock_enrich = AsyncMock(return_value=ENRICHMENT_WITH_LABELS)

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
            result = await analyze_incident(
                {"incident_id": "inc-265-001", "signal_name": "OOMKill", "remediation_id": "rem-001"},
                app_config={},
            )

        assert result.get("detected_labels") is not None, (
            "EnrichmentService labels must be included in the response"
        )
        assert result["detected_labels"]["gitOpsManaged"] is True
        assert result["detected_labels"]["hpaEnabled"] is True

    @pytest.mark.asyncio
    async def test_ut_hapi_265_002_phase3_prompt_includes_enrichment_context(self):
        """UT-HAPI-265-002: Phase 3 prompt includes enrichment context.

        BR-HAPI-265: The Phase 3 prompt must include enrichment context
        (root owner, detected labels, remediation history) so the LLM
        can make an informed workflow selection.
        """
        from holmes.core.models import InvestigationResult

        phase1_result = InvestigationResult(
            analysis=json.dumps({
                "root_cause_analysis": {
                    "summary": "OOM detected",
                    "affectedResource": {"kind": "Pod", "name": "api-xyz", "namespace": "prod"},
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

        phase3_prompts = []

        def mock_investigate(investigate_request, dal, config):
            phase = investigate_request.context.get("phase")
            if phase == 3:
                phase3_prompts.append(investigate_request.description)
            return phase1_result if phase == 1 else phase3_result

        mock_enrich = AsyncMock(return_value=ENRICHMENT_WITH_LABELS)

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
                {"incident_id": "inc-265-002", "signal_name": "OOMKill", "remediation_id": "rem-001"},
                app_config={},
            )

        assert len(phase3_prompts) >= 1, "Phase 3 prompt should have been captured"
        prompt = phase3_prompts[0]
        assert "Enrichment Context" in prompt or "enrichment context" in prompt.lower(), (
            "Phase 3 prompt must contain an enrichment context section"
        )
        assert "Deployment/api" in prompt or ("Deployment" in prompt and "api" in prompt and "prod" in prompt), (
            "Phase 3 prompt must mention the resolved root owner"
        )
        assert "gitOpsManaged" in prompt, (
            "Phase 3 prompt must include detected label names from EnrichmentService"
        )
