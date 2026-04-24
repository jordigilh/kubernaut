#
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
#

"""
Unit tests for HAPI audit trace correctness (#600 + blast radius).

Test Plan: docs/tests/600/TEST_PLAN.md
Issue: https://github.com/jordigilh/kubernaut/issues/600

Authority:
  - BR-AUDIT-005: Complete audit trail
  - ADR-032 §1: Audit is MANDATORY
  - ADR-034: Unified audit table design

Group A (UT-HAPI-600-001–004): Proves and fixes the #600 bug where
getattr reads non-existent attributes on ToolCallResult.

Group B (UT-HAPI-600-005–011): Blast radius — validates all 7 remaining
HAPI audit event types produce correct payload VALUES (not just structure),
ensuring no similar attribute-mismatch bugs exist elsewhere.
"""

from holmes.core.models import InvestigationResult, ToolCallResult
from holmes.core.tools import StructuredToolResult, StructuredToolResultStatus

from src.audit.events import (
    create_tool_call_event,
    create_llm_request_event,
    create_llm_response_event,
    create_validation_attempt_event,
    create_aiagent_response_complete_event,
    create_aiagent_response_failed_event,
    create_enrichment_completed_event,
    create_enrichment_failed_event,
)


def _make_tool_call(name, params, data="ok"):
    return ToolCallResult(
        tool_call_id=f"call_{name}",
        tool_name=name,
        description=f"Invoke {name}",
        result=StructuredToolResult(
            status=StructuredToolResultStatus.SUCCESS,
            data=data,
            params=params,
        ),
    )


class TestToolCallAuditSerialization:
    """UT-HAPI-600-001 through UT-HAPI-600-004: tool_call audit field mapping."""

    def test_ut_hapi_600_001_tool_name_is_captured(self):
        """UT-HAPI-600-001 (BR-AUDIT-005): tool_name must be read from
        ToolCallResult.tool_name, not the non-existent .name attribute.

        Proves the bug: getattr(tc, 'name', 'unknown') returns 'unknown'
        because ToolCallResult has .tool_name, not .name."""
        tc = _make_tool_call("kubectl_describe", {"resource": "pod/api"})

        # Current buggy code (line 161 of investigation_helpers.py):
        buggy_name = getattr(tc, 'name', 'unknown')
        assert buggy_name == 'unknown', "Bug precondition: .name doesn't exist on ToolCallResult"

        # Fixed code:
        correct_name = getattr(tc, 'tool_name', 'unknown')
        assert correct_name == "kubectl_describe"

        event = create_tool_call_event(
            incident_id="inc-001", remediation_id="rr-001",
            tool_call_index=0, tool_name=correct_name,
            tool_arguments=tc.result.params or {},
            tool_result=tc.result,
        )
        payload = event.event_data.actual_instance
        assert payload.tool_name == "kubectl_describe"

    def test_ut_hapi_600_002_tool_arguments_from_result_params(self):
        """UT-HAPI-600-002 (BR-AUDIT-005): tool_arguments must be read from
        ToolCallResult.result.params, not the non-existent .arguments attribute.

        Proves the bug: getattr(tc, 'arguments', {}) returns {} because
        .arguments doesn't exist. The params live at .result.params."""
        params = {"namespace": "default", "name": "api-gateway"}
        tc = _make_tool_call("list_available_actions", params)

        # Current buggy code (line 162):
        buggy_args = getattr(tc, 'arguments', {})
        assert buggy_args == {}, "Bug precondition: .arguments doesn't exist on ToolCallResult"

        # Fixed code:
        correct_args = getattr(tc.result, 'params', {}) if tc.result else {}
        assert correct_args == params

        event = create_tool_call_event(
            incident_id="inc-002", remediation_id="rr-002",
            tool_call_index=0, tool_name=tc.tool_name,
            tool_arguments=correct_args,
            tool_result=tc.result,
        )
        payload = event.event_data.actual_instance
        assert payload.tool_arguments == params

    def test_ut_hapi_600_003_multiple_tool_calls_all_captured(self):
        """UT-HAPI-600-003 (BR-AUDIT-005): When multiple tool calls are present,
        each one must produce the correct tool_name and arguments."""
        tools = [
            _make_tool_call("kubectl_describe", {"resource": "pod/web"}),
            _make_tool_call("kubectl_logs", {"pod": "web", "tail": "100"}),
            _make_tool_call("list_available_actions", {"severity": "critical"}),
        ]

        events = []
        for idx, tc in enumerate(tools):
            name = getattr(tc, 'tool_name', 'unknown')
            args = getattr(tc.result, 'params', {}) if tc.result else {}
            event = create_tool_call_event(
                incident_id="inc-003", remediation_id="rr-003",
                tool_call_index=idx, tool_name=name,
                tool_arguments=args, tool_result=tc.result,
            )
            events.append(event)

        payloads = [e.event_data.actual_instance for e in events]
        assert payloads[0].tool_name == "kubectl_describe"
        assert payloads[1].tool_name == "kubectl_logs"
        assert payloads[2].tool_name == "list_available_actions"

        assert payloads[0].tool_arguments == {"resource": "pod/web"}
        assert payloads[1].tool_arguments == {"pod": "web", "tail": "100"}
        assert payloads[2].tool_arguments == {"severity": "critical"}

    def test_ut_hapi_600_004_null_result_params_defaults_to_empty(self):
        """UT-HAPI-600-004 (BR-AUDIT-005): When result.params is None,
        tool_arguments should default to {} rather than None."""
        tc = ToolCallResult(
            tool_call_id="call_no_params",
            tool_name="get_cluster_info",
            description="Get cluster info",
            result=StructuredToolResult(
                status=StructuredToolResultStatus.SUCCESS,
                data="k8s 1.28",
                params=None,
            ),
        )

        args = getattr(tc.result, 'params', {}) if tc.result else {}
        # params is None, so we need the `or {}` fallback
        args = args or {}
        assert args == {}

        event = create_tool_call_event(
            incident_id="inc-004", remediation_id="rr-004",
            tool_call_index=0, tool_name=tc.tool_name,
            tool_arguments=args, tool_result=tc.result,
        )
        payload = event.event_data.actual_instance
        assert payload.tool_arguments == {}


class TestAuditTraceBlastRadius:
    """UT-HAPI-600-005 through UT-HAPI-600-011: blast radius validation
    for all remaining HAPI audit event types."""

    def test_ut_hapi_600_005_llm_request_values(self):
        """UT-HAPI-600-005 (BR-AUDIT-005): LLM request audit captures
        model name, toolset list, and MCP server list."""
        event = create_llm_request_event(
            incident_id="inc-005",
            remediation_id="rr-005",
            model="claude-3-5-sonnet",
            prompt="Investigate OOMKilled pod in namespace prod",
            toolsets_enabled=["kubernetes/core", "prometheus"],
            mcp_servers=["kubectl"],
        )
        p = event.event_data.actual_instance
        assert p.incident_id == "inc-005"
        assert p.model == "claude-3-5-sonnet"
        assert p.toolsets_enabled == ["kubernetes/core", "prometheus"]
        assert p.mcp_servers == ["kubectl"]
        assert p.prompt_length == len("Investigate OOMKilled pod in namespace prod")
        assert p.prompt_preview == "Investigate OOMKilled pod in namespace prod"

    def test_ut_hapi_600_006_llm_response_values(self):
        """UT-HAPI-600-006 (BR-AUDIT-005): LLM response audit captures
        analysis metrics and token usage."""
        long_analysis = "A" * 600
        event = create_llm_response_event(
            incident_id="inc-006",
            remediation_id="rr-006",
            has_analysis=True,
            analysis_length=600,
            analysis_preview=long_analysis[:500] + "...",
            tool_call_count=5,
            tokens_used=3200,
        )
        p = event.event_data.actual_instance
        assert p.incident_id == "inc-006"
        assert p.has_analysis is True
        assert p.analysis_length == 600
        assert p.analysis_preview.endswith("...")
        assert len(p.analysis_preview) == 503  # 500 chars + "..."
        assert p.tool_call_count == 5
        assert p.tokens_used == 3200
        assert event.event_outcome == "success"

    def test_ut_hapi_600_007_validation_attempt_values(self):
        """UT-HAPI-600-007 (BR-AUDIT-005): Validation attempt audit captures
        workflow_id, errors, and human_review_reason."""
        event = create_validation_attempt_event(
            incident_id="inc-007",
            remediation_id="rr-007",
            attempt=2,
            max_attempts=3,
            is_valid=False,
            errors=["workflow_not_found", "schema_mismatch"],
            workflow_id="rollback-v1",
            human_review_reason="no_matching_workflow",
        )
        p = event.event_data.actual_instance
        assert p.incident_id == "inc-007"
        assert p.attempt == 2
        assert p.max_attempts == 3
        assert p.is_valid is False
        assert p.errors == ["workflow_not_found", "schema_mismatch"]
        assert p.validation_errors == "workflow_not_found; schema_mismatch"
        assert p.workflow_id == "rollback-v1"
        assert p.human_review_reason == "no_matching_workflow"
        assert p.is_final_attempt is False
        assert event.event_outcome == "pending"

    def test_ut_hapi_600_008_response_complete_values(self):
        """UT-HAPI-600-008 (BR-AUDIT-005): Response complete audit captures
        full response_data dict and token breakdown."""
        response_data = {
            "incidentId": "inc-008",
            "analysis": "Root cause: OOMKilled",
            "rootCauseAnalysis": {
                "summary": "OOMKilled due to memory leak",
                "severity": "critical",
                "contributingFactors": ["memory leak", "no HPA"],
            },
            "confidence": 0.85,
            "timestamp": "2026-03-04T12:00:00Z",
            "needsHumanReview": False,
        }
        event = create_aiagent_response_complete_event(
            incident_id="inc-008",
            remediation_id="rr-008",
            response_data=response_data,
            total_prompt_tokens=2000,
            total_completion_tokens=1200,
        )
        p = event.event_data.actual_instance
        assert p.incident_id == "inc-008"
        assert p.response_data.incident_id == "inc-008"
        assert p.response_data.analysis == "Root cause: OOMKilled"
        assert p.response_data.root_cause_analysis.summary == "OOMKilled due to memory leak"
        assert p.response_data.root_cause_analysis.severity == "critical"
        assert p.response_data.confidence == 0.85
        assert p.response_data.needs_human_review is False
        assert p.total_prompt_tokens == 2000
        assert p.total_completion_tokens == 1200
        assert event.event_outcome == "success"
        assert event.correlation_id == "rr-008"

    def test_ut_hapi_600_009_response_failed_values(self):
        """UT-HAPI-600-009 (BR-AUDIT-005): Response failed audit captures
        error_message, phase, and duration."""
        event = create_aiagent_response_failed_event(
            incident_id="inc-009",
            remediation_id="rr-009",
            error_message="LLM request timed out after 120 seconds",
            phase="llm_analysis",
            duration_seconds=120.5,
        )
        p = event.event_data.actual_instance
        assert p.incident_id == "inc-009"
        assert p.error_message == "LLM request timed out after 120 seconds"
        assert p.phase == "llm_analysis"
        assert p.duration_seconds == 120.5
        assert event.event_outcome == "failure"

    def test_ut_hapi_600_010_enrichment_completed_values(self):
        """UT-HAPI-600-010 (BR-AUDIT-005): Enrichment completed audit captures
        root_owner fields, chain length, labels, and history flag."""
        event = create_enrichment_completed_event(
            incident_id="inc-010",
            remediation_id="rr-010",
            root_owner={"kind": "Deployment", "name": "api-gw", "namespace": "prod"},
            owner_chain_length=3,
            detected_labels={"gitOpsManaged": True, "stateful": False},
            failed_detections=["hpaEnabled"],
            remediation_history_fetched=True,
        )
        p = event.event_data.actual_instance
        assert p.incident_id == "inc-010"
        assert p.root_owner_kind == "Deployment"
        assert p.root_owner_name == "api-gw"
        assert p.root_owner_namespace == "prod"
        assert p.owner_chain_length == 3
        assert p.detected_labels_summary == {"gitOpsManaged": True, "stateful": False}
        assert p.failed_detections == ["hpaEnabled"]
        assert p.remediation_history_fetched is True
        assert event.event_outcome == "success"

    def test_ut_hapi_600_011_enrichment_failed_values(self):
        """UT-HAPI-600-011 (BR-AUDIT-005): Enrichment failed audit captures
        reason, detail, and affected_resource fields."""
        event = create_enrichment_failed_event(
            incident_id="inc-011",
            remediation_id="rr-011",
            reason="rca_incomplete",
            detail="Retry 3/3 exhausted — LLM returned empty analysis",
            affected_resource={"kind": "Pod", "name": "web-abc", "namespace": "demo"},
        )
        p = event.event_data.actual_instance
        assert p.incident_id == "inc-011"
        assert p.reason == "rca_incomplete"
        assert p.detail == "Retry 3/3 exhausted — LLM returned empty analysis"
        assert p.affected_resource_kind == "Pod"
        assert p.affected_resource_name == "web-abc"
        assert p.affected_resource_namespace == "demo"
        assert event.event_outcome == "failure"
