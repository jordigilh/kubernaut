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
Issue #542: Cluster-scoped remediationTarget regression.

Regression of #524 — the LLM sets remediationTarget to the symptom resource
(e.g., Deployment) instead of the remediation target (e.g., Node) for
scenarios like pending-taint and NodeNotReady.  Root cause: field was named
affectedResource (ambiguous), and examples only showed namespaced resources.
Renamed to remediationTarget with principle-based guidance.

Business Requirement: BR-HAPI-261 (LLM-provided remediationTarget)
Issue: #542

Test IDs:
  UT-HAPI-542-001: Phase 1 prompt includes cluster-scoped remediationTarget example
  UT-HAPI-542-002: Phase 1 prompt JSON success block shows cluster-scoped variant
  UT-HAPI-542-003: Phase 1 prompt JSON failure block shows cluster-scoped variant
  UT-HAPI-542-004: Phase 3 prompt includes cluster-scoped remediationTarget example
  UT-HAPI-542-005: PHASE3_SECTIONS description mentions scope-aware format
  UT-HAPI-542-006: Mock LLM picks get_cluster_resource_context for node_not_ready
  UT-HAPI-542-007: Mock LLM omits namespace from tool_args for cluster-scoped
  UT-HAPI-542-008: Mock LLM Phase 1 RCA omits namespace for cluster-scoped resource
  UT-HAPI-542-009: Phase 1 prompt uses remediationTarget, not affectedResource
  UT-HAPI-542-010: Phase 3 prompt uses remediationTarget, not affectedResource
  UT-HAPI-542-011: Mock LLM Phase 1 RCA uses remediationTarget field name
  UT-HAPI-542-012: Phase 1 prompt includes principle-based guidance for remediationTarget
"""

import json
import pytest


# ────────────────────────────────────────────────────────────────
# Helpers
# ────────────────────────────────────────────────────────────────

def _make_request_data(**overrides):
    """Build a minimal IncidentRequest-like dict."""
    data = {
        "incident_id": "inc-542-001",
        "signal_name": "NodeNotReady",
        "severity": "critical",
        "signal_source": "prometheus",
        "resource_namespace": "",
        "resource_kind": "Node",
        "resource_name": "worker-node-1",
        "error_message": "Node not ready due to taint",
        "environment": "production",
        "priority": "P1",
        "risk_tolerance": "medium",
        "business_category": "critical",
        "cluster_name": "prod-us-west-2",
    }
    data.update(overrides)
    return data


def _make_namespaced_request_data(**overrides):
    """Build a minimal IncidentRequest-like dict for a namespaced resource."""
    data = {
        "incident_id": "inc-542-ns",
        "signal_name": "OOMKilled",
        "severity": "critical",
        "signal_source": "prometheus",
        "resource_namespace": "production",
        "resource_kind": "Pod",
        "resource_name": "api-server-abc123",
        "error_message": "OOMKilled",
        "environment": "production",
        "priority": "P1",
        "risk_tolerance": "medium",
        "business_category": "critical",
        "cluster_name": "prod-us-west-2",
    }
    data.update(overrides)
    return data


# ════════════════════════════════════════════════════════════════
# Part A: Prompt Builder — cluster-scoped remediationTarget examples
# ════════════════════════════════════════════════════════════════

class TestPhase1PromptClusterScopeExamples:
    """UT-HAPI-542-001 through 003: Phase 1 prompt must teach the LLM about
    cluster-scoped remediationTarget (no namespace)."""

    def test_ut_hapi_542_001_phase1_prompt_includes_cluster_scoped_example(self):
        """UT-HAPI-542-001: Phase 1 prompt contains a cluster-scoped remediationTarget
        example showing {kind: Node, name: ...} WITHOUT namespace."""
        from src.extensions.incident import create_incident_investigation_prompt

        prompt = create_incident_investigation_prompt(
            _make_namespaced_request_data()
        )

        assert '"kind": "Node"' in prompt or "'kind': 'Node'" in prompt, (
            "Phase 1 prompt must include at least one cluster-scoped "
            "remediationTarget example (e.g. Node) to prevent the LLM from "
            "always defaulting to Deployment-style namespaced resources"
        )

    def test_ut_hapi_542_002_phase1_json_success_has_cluster_example(self):
        """UT-HAPI-542-002: Phase 1 prompt's JSON success response block shows
        a cluster-scoped remediationTarget variant (no 'namespace' key)."""
        from src.extensions.incident import create_incident_investigation_prompt

        prompt = create_incident_investigation_prompt(
            _make_namespaced_request_data()
        )

        assert "cluster-scoped" in prompt.lower() or (
            '"kind": "Node"' in prompt
        ), (
            "Phase 1 JSON success block must mention cluster-scoped resources "
            "or include a Node example"
        )

    def test_ut_hapi_542_003_phase1_section_header_has_cluster_example(self):
        """UT-HAPI-542-003: Phase 1 prompt's section-header response format
        (the '# root_cause_analysis' line in Part 2) documents the cluster-scoped variant."""
        from src.extensions.incident import create_incident_investigation_prompt

        prompt = create_incident_investigation_prompt(
            _make_namespaced_request_data()
        )

        part2_idx = prompt.find("REQUIRED FORMAT")
        assert part2_idx >= 0, "Prompt must contain 'REQUIRED FORMAT' section"

        section_text = prompt[part2_idx:part2_idx + 800]
        assert "node" in section_text.lower() or "cluster-scoped" in section_text.lower(), (
            "The 'Part 2: Structured Data' section's '# root_cause_analysis' "
            "example must document the cluster-scoped remediationTarget format (no namespace)"
        )


class TestPhase3PromptClusterScopeExamples:
    """UT-HAPI-542-004 through 005: Phase 3 prompt and sections must be
    scope-aware for remediationTarget."""

    def test_ut_hapi_542_004_phase3_prompt_includes_cluster_scoped_example(self):
        """UT-HAPI-542-004: Phase 3 prompt includes a cluster-scoped
        remediationTarget example alongside the namespaced one."""
        from src.extensions.incident.prompt_builder import create_phase3_workflow_prompt

        prompt = create_phase3_workflow_prompt(_make_request_data())

        rca_section_idx = prompt.find("# root_cause_analysis")
        assert rca_section_idx >= 0

        section_text = prompt[rca_section_idx:rca_section_idx + 600]
        assert "node" in section_text.lower() or "cluster-scoped" in section_text.lower(), (
            "Phase 3 '# root_cause_analysis' example must document "
            "the cluster-scoped remediationTarget format"
        )

    def test_ut_hapi_542_005_phase3_sections_description_is_scope_aware(self):
        """UT-HAPI-542-005: PHASE3_SECTIONS root_cause_analysis description
        mentions that remediationTarget can be cluster-scoped (no namespace)."""
        from src.extensions.incident.prompt_builder import PHASE3_SECTIONS

        rca_desc = PHASE3_SECTIONS["root_cause_analysis"]
        assert "cluster" in rca_desc.lower() or "namespace" in rca_desc.lower(), (
            "PHASE3_SECTIONS['root_cause_analysis'] must mention that "
            "remediationTarget omits namespace for cluster-scoped resources"
        )


# ════════════════════════════════════════════════════════════════
# Part B: Mock LLM — cluster-scoped resource context tool selection
# ════════════════════════════════════════════════════════════════

class TestMockLLMClusterScopeToolSelection:
    """UT-HAPI-542-006 through 008: Mock LLM must pick the correct resource
    context tool based on resource scope."""

    def _make_mock_handler(self):
        """Instantiate MockLLMRequestHandler for unit-level testing."""
        import importlib.util
        import os

        server_path = os.path.normpath(os.path.join(
            os.path.dirname(__file__), "..", "..", "..",
            "test", "services", "mock-llm", "src", "server.py",
        ))
        spec = importlib.util.spec_from_file_location("mock_llm_server", server_path)
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)

        handler = object.__new__(mod.MockLLMRequestHandler)
        return handler, mod.MOCK_SCENARIOS

    def test_ut_hapi_542_006_mock_picks_cluster_tool_for_node_scenario(self):
        """UT-HAPI-542-006: For node_not_ready scenario (cluster-scoped),
        the mock must emit get_cluster_resource_context, not
        get_namespaced_resource_context."""
        handler, scenarios = self._make_mock_handler()
        scenario = scenarios["node_not_ready"]

        tools = [
            {"function": {"name": "get_namespaced_resource_context"}},
            {"function": {"name": "get_cluster_resource_context"}},
            {"function": {"name": "list_available_actions"}},
            {"function": {"name": "list_workflows"}},
            {"function": {"name": "get_workflow"}},
        ]
        messages = [
            {"role": "user", "content": "Investigate NodeNotReady on worker-node-1"}
        ]
        request_data = {"messages": messages, "tools": tools}

        response = handler._discovery_tool_call_response(
            scenario, request_data, step=0,
            has_resource_context=True, tools=tools,
        )

        tool_call = response["choices"][0]["message"]["tool_calls"][0]
        tool_name = tool_call["function"]["name"]
        assert tool_name == "get_cluster_resource_context", (
            f"Expected get_cluster_resource_context for cluster-scoped "
            f"node_not_ready, got {tool_name}"
        )

    def test_ut_hapi_542_007_mock_omits_namespace_for_cluster_scope(self):
        """UT-HAPI-542-007: For cluster-scoped scenarios, tool_args must NOT
        include a 'namespace' key."""
        handler, scenarios = self._make_mock_handler()
        scenario = scenarios["node_not_ready"]

        tools = [
            {"function": {"name": "get_namespaced_resource_context"}},
            {"function": {"name": "get_cluster_resource_context"}},
            {"function": {"name": "list_available_actions"}},
            {"function": {"name": "list_workflows"}},
            {"function": {"name": "get_workflow"}},
        ]
        messages = [
            {"role": "user", "content": "Investigate NodeNotReady on worker-node-1"}
        ]
        request_data = {"messages": messages, "tools": tools}

        response = handler._discovery_tool_call_response(
            scenario, request_data, step=0,
            has_resource_context=True, tools=tools,
        )

        tool_call = response["choices"][0]["message"]["tool_calls"][0]
        tool_args = json.loads(tool_call["function"]["arguments"])
        assert "namespace" not in tool_args, (
            f"Cluster-scoped tool_args must NOT include 'namespace', "
            f"got {tool_args}"
        )

    def test_ut_hapi_542_008_mock_phase1_rca_omits_namespace_for_cluster(self):
        """UT-HAPI-542-008: Mock LLM Phase 1 RCA response for node_not_ready
        must produce a remediationTarget without namespace."""
        handler, scenarios = self._make_mock_handler()
        scenario = scenarios["node_not_ready"]

        messages = [
            {"role": "user", "content": "Investigate NodeNotReady on worker-node-1"},
            {"role": "tool", "content": json.dumps({
                "root_owner": {"kind": "Node", "name": "worker-node-1"},
                "remediation_history": [],
            })},
        ]
        request_data = {
            "messages": messages,
            "model": "mock-model",
        }

        response = handler._phase1_rca_response(scenario, request_data)
        content = response["choices"][0]["message"]["content"]

        rca_header_idx = content.find("# root_cause_analysis")
        assert rca_header_idx >= 0

        rca_line = content[rca_header_idx:]
        rca_json_start = rca_line.find("{")
        rca_json_str = rca_line[rca_json_start:]
        end_idx = rca_json_str.find("\n\n")
        if end_idx > 0:
            rca_json_str = rca_json_str[:end_idx]

        rca_data = json.loads(rca_json_str)
        target = rca_data.get("remediationTarget", {})

        assert target["kind"] == "Node"
        assert target["name"] == "worker-node-1"
        assert "namespace" not in target, (
            f"Cluster-scoped remediationTarget must NOT have 'namespace', "
            f"got {target}"
        )


# ════════════════════════════════════════════════════════════════
# Part C: Rename — affectedResource → remediationTarget (#542)
# ════════════════════════════════════════════════════════════════

class TestRemediationTargetRename:
    """UT-HAPI-542-009 through 012: Verify the field is exclusively named
    remediationTarget and that affectedResource no longer appears in
    prompt text or LLM response schemas."""

    def test_ut_hapi_542_009_phase1_prompt_uses_remediation_target(self):
        """UT-HAPI-542-009: Phase 1 prompt must use 'remediationTarget',
        not 'affectedResource'."""
        from src.extensions.incident import create_incident_investigation_prompt

        prompt = create_incident_investigation_prompt(
            _make_namespaced_request_data()
        )

        assert "remediationTarget" in prompt, (
            "Phase 1 prompt must contain the field name 'remediationTarget'"
        )
        assert "affectedResource" not in prompt, (
            "Phase 1 prompt must NOT contain the old field name 'affectedResource'"
        )

    def test_ut_hapi_542_010_phase3_prompt_uses_remediation_target(self):
        """UT-HAPI-542-010: Phase 3 prompt must use 'remediationTarget',
        not 'affectedResource'."""
        from src.extensions.incident.prompt_builder import (
            create_phase3_workflow_prompt,
            PHASE3_SECTIONS,
        )

        prompt = create_phase3_workflow_prompt(_make_request_data())

        assert "remediationTarget" in prompt, (
            "Phase 3 prompt must contain the field name 'remediationTarget'"
        )
        assert "affectedResource" not in prompt, (
            "Phase 3 prompt must NOT contain the old field name 'affectedResource'"
        )

        rca_desc = PHASE3_SECTIONS["root_cause_analysis"]
        assert "remediationTarget" in rca_desc, (
            "PHASE3_SECTIONS description must reference 'remediationTarget'"
        )
        assert "affectedResource" not in rca_desc, (
            "PHASE3_SECTIONS description must NOT reference 'affectedResource'"
        )

    def test_ut_hapi_542_011_mock_llm_rca_uses_remediation_target(self):
        """UT-HAPI-542-011: Mock LLM Phase 1 RCA uses 'remediationTarget'
        field name in the JSON response."""
        import importlib.util
        import os

        server_path = os.path.normpath(os.path.join(
            os.path.dirname(__file__), "..", "..", "..",
            "test", "services", "mock-llm", "src", "server.py",
        ))
        spec = importlib.util.spec_from_file_location("mock_llm_server", server_path)
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)

        handler = object.__new__(mod.MockLLMRequestHandler)
        scenario = mod.MOCK_SCENARIOS["oomkilled"]

        messages = [
            {"role": "user", "content": "Investigate OOMKilled on api-server-abc123 in production"},
            {"role": "tool", "content": json.dumps({
                "root_owner": {"kind": "Deployment", "name": "api-server", "namespace": "production"},
                "remediation_history": [],
            })},
        ]
        request_data = {"messages": messages, "model": "mock-model"}

        response = handler._phase1_rca_response(scenario, request_data)
        content = response["choices"][0]["message"]["content"]

        rca_header_idx = content.find("# root_cause_analysis")
        assert rca_header_idx >= 0

        rca_line = content[rca_header_idx:]
        rca_json_start = rca_line.find("{")
        rca_json_str = rca_line[rca_json_start:]
        end_idx = rca_json_str.find("\n\n")
        if end_idx > 0:
            rca_json_str = rca_json_str[:end_idx]

        rca_data = json.loads(rca_json_str)

        assert "remediationTarget" in rca_data, (
            f"Mock LLM RCA must use 'remediationTarget', got keys: {list(rca_data.keys())}"
        )
        assert "affectedResource" not in rca_data, (
            "Mock LLM RCA must NOT use the old 'affectedResource' field name"
        )

    def test_ut_hapi_542_012_phase1_prompt_includes_principle_based_guidance(self):
        """UT-HAPI-542-012: Phase 1 prompt includes principle-based guidance
        explaining that remediationTarget is the resource the workflow will
        operate on, not just the resource exhibiting symptoms."""
        from src.extensions.incident import create_incident_investigation_prompt

        prompt = create_incident_investigation_prompt(
            _make_namespaced_request_data()
        )

        prompt_lower = prompt.lower()
        has_guidance = (
            "workflow" in prompt_lower
            and "remediation" in prompt_lower
            and ("operate" in prompt_lower or "act on" in prompt_lower or "target" in prompt_lower)
        )
        assert has_guidance, (
            "Phase 1 prompt must include principle-based guidance explaining "
            "that remediationTarget is the resource the workflow will operate on"
        )
