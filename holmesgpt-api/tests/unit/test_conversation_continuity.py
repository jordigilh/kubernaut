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
Unit tests for BR-HAPI-263: Conversation Continuity.

TDD Group 1: Prompt-based conversation continuity.
Tests that Phase 1 RCA analysis text is carried forward into the Phase 3
prompt so the LLM has full context for workflow selection.

Implementation: Phase 1 analysis is injected as a "Phase 1 Root Cause
Analysis" section in the Phase 3 prompt (no SDK changes required).
"""

import json
import pytest
from unittest.mock import MagicMock, patch, AsyncMock

from holmes.core.models import InvestigationResult
from src.extensions.incident.enrichment_service import EnrichmentResult

_P = "src.extensions.llm_config"
_LLM = "src.extensions.incident.llm_integration"

VALID_AFFECTED_RESOURCE = {"kind": "Pod", "name": "api-xyz", "namespace": "prod"}

ENRICHMENT_RESULT = EnrichmentResult(
    root_owner={"kind": "Deployment", "name": "api", "namespace": "prod"},
    detected_labels={"component": "backend"},
    remediation_history={"entries": []},
)

PHASE1_ANALYSIS = json.dumps({
    "root_cause_analysis": {
        "summary": "OOM detected in pod api-xyz due to memory leak in request handler",
        "affectedResource": VALID_AFFECTED_RESOURCE,
    },
})


class TestPromptBasedConversationContinuity:
    """G1: Prompt-based conversation continuity (BR-HAPI-263)."""

    @pytest.mark.asyncio
    async def test_ut_hapi_263_001_phase3_prompt_includes_phase1_analysis(self):
        """UT-HAPI-263-001: Phase 3 prompt includes Phase 1 analysis text.

        BR-HAPI-263: The Phase 3 prompt must contain the Phase 1 RCA analysis
        so the LLM has context about the root cause when selecting a workflow.
        """
        phase1_result = InvestigationResult(analysis=PHASE1_ANALYSIS)
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
                {"incident_id": "inc-263-001", "signal_name": "OOMKill", "remediation_id": "rem-001"},
                app_config={},
            )

        assert len(phase3_prompts) >= 1, "Phase 3 prompt should have been captured"
        prompt = phase3_prompts[0]
        assert "Phase 1 Root Cause Analysis" in prompt, (
            "Phase 3 prompt must contain the Phase 1 RCA section header"
        )
        assert "OOM detected in pod api-xyz" in prompt, (
            "Phase 3 prompt must contain Phase 1 analysis content"
        )

    @pytest.mark.asyncio
    async def test_ut_hapi_263_002_phase1_analysis_survives_retry(self):
        """UT-HAPI-263-002: Phase 1 analysis text persists across Phase 3 validation retries.

        BR-HAPI-263: When Phase 3 validation fails and retries, the Phase 1
        analysis must still be present in the retried Phase 3 prompt.
        """
        phase1_result = InvestigationResult(analysis=PHASE1_ANALYSIS)

        invalid_phase3 = InvestigationResult(
            analysis=json.dumps({
                "root_cause_analysis": {"summary": "OOM detected"},
                "selected_workflow": {"workflow_id": "bad-workflow"},
            }),
        )
        valid_phase3 = InvestigationResult(
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
        call_count = 0

        def mock_investigate(investigate_request, dal, config):
            nonlocal call_count
            call_count += 1
            phase = investigate_request.context.get("phase")
            if phase == 3:
                phase3_prompts.append(investigate_request.description)
            if phase == 1:
                return phase1_result
            return invalid_phase3 if len(phase3_prompts) == 1 else valid_phase3

        parse_call_count = 0

        def mock_parse(investigation_result, request_data, data_storage_client=None):
            nonlocal parse_call_count
            parse_call_count += 1
            parsed = json.loads(investigation_result.analysis)
            if parse_call_count == 1:
                return (
                    parsed,
                    MagicMock(is_valid=False, errors=["invalid workflow"], parameter_schema=None, schema_hint=None),
                )
            return (
                parsed,
                MagicMock(is_valid=True, errors=[], parameter_schema=None, schema_hint=None),
            )

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
             patch(f"{_LLM}.parse_and_validate_investigation_result", side_effect=mock_parse):

            mock_audit.return_value = MagicMock()
            MockES.return_value.enrich = mock_enrich

            from src.extensions.incident.llm_integration import analyze_incident
            await analyze_incident(
                {"incident_id": "inc-263-002", "signal_name": "OOMKill", "remediation_id": "rem-001"},
                app_config={},
            )

        assert len(phase3_prompts) >= 2, (
            f"Expected at least 2 Phase 3 prompts (initial + retry), got {len(phase3_prompts)}"
        )
        for i, prompt in enumerate(phase3_prompts):
            assert "Phase 1 Root Cause Analysis" in prompt, (
                f"Phase 3 prompt attempt {i+1} must contain Phase 1 RCA section"
            )
            assert "OOM detected in pod api-xyz" in prompt, (
                f"Phase 3 prompt attempt {i+1} must contain Phase 1 analysis content"
            )
