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
Three-Step Workflow Discovery Prompt Builder Tests (DD-HAPI-017)

Business Requirements: BR-HAPI-017-002 (Prompt Template Updates)
Design Decisions:
- DD-HAPI-017: Three-Step Workflow Discovery Integration
- DD-WORKFLOW-016: Action-Type Workflow Catalog Indexing

Tests validate BUSINESS OUTCOMES:
- Both incident and recovery prompts reference three-step discovery tools
- Old search_workflow_catalog references are fully removed
- LLM is instructed to review ALL workflows before selecting (Step 2 mandate)

Test IDs: UT-HAPI-017-002-001 through UT-HAPI-017-002-005
"""


# ============================================================
# Shared test data helpers
# ============================================================

def _make_incident_request_data(**overrides):
    """Build a minimal IncidentRequest-like dict for prompt generation."""
    data = {
        "incident_id": "inc-test-001",
        "signal_name": "OOMKilled",
        "severity": "high",
        "signal_source": "prometheus",
        "resource_namespace": "production",
        "resource_kind": "Deployment",
        "resource_name": "api-server",
        "error_message": "Container exceeded memory limit",
        "environment": "production",
        "priority": "P1",
        "risk_tolerance": "medium",
        "business_category": "critical",
        "cluster_name": "prod-us-west-2",
    }
    data.update(overrides)
    return data


def _make_recovery_request_data(**overrides):
    """Build a minimal recovery request dict for prompt generation."""
    data = {
        "incident_id": "inc-test-001",
        "is_recovery_attempt": True,
        "recovery_attempt_number": 2,
        "signal_name": "OOMKilled",
        "severity": "high",
        "signal_source": "prometheus",
        "resource_namespace": "production",
        "resource_kind": "Deployment",
        "resource_name": "api-server",
        "error_message": "Container exceeded memory limit",
        "environment": "production",
        "priority": "P1",
        "risk_tolerance": "medium",
        "business_category": "critical",
        "cluster_name": "prod-us-west-2",
        "previous_execution": {
            "workflow_execution_ref": "req-001-we-1",
            "original_rca": {
                "summary": "Memory exhaustion due to leak",
                "signal_name": "OOMKilled",
                "severity": "high",
                "contributing_factors": ["memory leak"],
            },
            "selected_workflow": {
                "workflow_id": "scale-horizontal-v1",
                "version": "1.0.0",
                "execution_bundle": "kubernaut/workflow-scale:v1.0.0",
                "parameters": {"TARGET_REPLICAS": "5"},
                "rationale": "Scale to distribute load",
            },
            "failure": {
                "failed_step_index": 2,
                "failed_step_name": "scale_deployment",
                "reason": "OOMKilled",
                "message": "Container exceeded memory limit",
                "exit_code": 137,
                "failed_at": "2025-11-29T10:30:00Z",
                "execution_time": "2m34s",
            },
        },
    }
    data.update(overrides)
    return data


# ============================================================
# UT-HAPI-017-002-001: Incident prompt contains three-step instructions
# ============================================================

class TestIncidentPromptThreeStepInstructions:
    """
    UT-HAPI-017-002-001: Incident prompt contains three-step instructions

    Business Outcome: Incident LLM prompts reference all three workflow
    discovery tools so the LLM follows the three-step protocol.

    BR: BR-HAPI-017-002
    DD: DD-HAPI-017, DD-WORKFLOW-016
    """

    def test_incident_prompt_contains_list_available_actions(self):
        """Step 1 tool name present in incident prompt."""
        from src.extensions.incident import create_incident_investigation_prompt

        prompt = create_incident_investigation_prompt(_make_incident_request_data())
        assert "list_available_actions" in prompt

    def test_incident_prompt_contains_list_workflows(self):
        """Step 2 tool name present in incident prompt."""
        from src.extensions.incident import create_incident_investigation_prompt

        prompt = create_incident_investigation_prompt(_make_incident_request_data())
        assert "list_workflows" in prompt

    def test_incident_prompt_contains_get_workflow(self):
        """Step 3 tool name present in incident prompt."""
        from src.extensions.incident import create_incident_investigation_prompt

        prompt = create_incident_investigation_prompt(_make_incident_request_data())
        assert "get_workflow" in prompt

    def test_incident_prompt_contains_all_three_tools(self):
        """All three tool names appear together in incident prompt."""
        from src.extensions.incident import create_incident_investigation_prompt

        prompt = create_incident_investigation_prompt(_make_incident_request_data())

        assert "list_available_actions" in prompt, "Missing Step 1: list_available_actions"
        assert "list_workflows" in prompt, "Missing Step 2: list_workflows"
        assert "get_workflow" in prompt, "Missing Step 3: get_workflow"


# ============================================================
# UT-HAPI-017-002-002: Recovery prompt contains three-step instructions
# ============================================================

class TestRecoveryPromptThreeStepInstructions:
    """
    UT-HAPI-017-002-002: Recovery prompt contains three-step instructions

    Business Outcome: Recovery LLM prompts reference all three workflow
    discovery tools so the LLM follows the three-step protocol (parity
    with incident flow).

    BR: BR-HAPI-017-002
    DD: DD-HAPI-017, DD-WORKFLOW-016
    """

    def test_recovery_prompt_contains_list_available_actions(self):
        """Step 1 tool name present in recovery prompt."""
        from src.extensions.recovery import _create_recovery_investigation_prompt

        prompt = _create_recovery_investigation_prompt(_make_recovery_request_data())
        assert "list_available_actions" in prompt

    def test_recovery_prompt_contains_list_workflows(self):
        """Step 2 tool name present in recovery prompt."""
        from src.extensions.recovery import _create_recovery_investigation_prompt

        prompt = _create_recovery_investigation_prompt(_make_recovery_request_data())
        assert "list_workflows" in prompt

    def test_recovery_prompt_contains_get_workflow(self):
        """Step 3 tool name present in recovery prompt."""
        from src.extensions.recovery import _create_recovery_investigation_prompt

        prompt = _create_recovery_investigation_prompt(_make_recovery_request_data())
        assert "get_workflow" in prompt

    def test_recovery_prompt_contains_all_three_tools(self):
        """All three tool names appear together in recovery prompt."""
        from src.extensions.recovery import _create_recovery_investigation_prompt

        prompt = _create_recovery_investigation_prompt(_make_recovery_request_data())

        assert "list_available_actions" in prompt, "Missing Step 1: list_available_actions"
        assert "list_workflows" in prompt, "Missing Step 2: list_workflows"
        assert "get_workflow" in prompt, "Missing Step 3: get_workflow"

    def test_legacy_investigation_prompt_contains_three_step_tools(self):
        """Legacy _create_investigation_prompt also uses three-step protocol."""
        from src.extensions.recovery import _create_investigation_prompt

        prompt = _create_investigation_prompt(_make_recovery_request_data())

        assert "list_available_actions" in prompt, "Missing Step 1 in legacy prompt"
        assert "list_workflows" in prompt, "Missing Step 2 in legacy prompt"
        assert "get_workflow" in prompt, "Missing Step 3 in legacy prompt"


# ============================================================
# UT-HAPI-017-002-003: Incident prompt does NOT contain search_workflow_catalog
# ============================================================

class TestIncidentPromptNoOldToolName:
    """
    UT-HAPI-017-002-003: Incident prompt does NOT contain search_workflow_catalog

    Business Outcome: Old tool name is fully removed from incident prompts,
    preventing LLM from attempting to call the superseded tool.

    BR: BR-HAPI-017-002, BR-HAPI-017-006
    DD: DD-HAPI-017
    """

    def test_incident_prompt_no_search_workflow_catalog(self):
        """Incident prompt must not reference old tool name."""
        from src.extensions.incident import create_incident_investigation_prompt

        prompt = create_incident_investigation_prompt(_make_incident_request_data())
        assert "search_workflow_catalog" not in prompt

    def test_incident_mcp_filter_instructions_no_search_workflow_catalog(self):
        """MCP filter instructions must not reference old tool name."""
        from src.extensions.incident import build_mcp_filter_instructions
        from src.models.incident_models import DetectedLabels

        detected_labels = DetectedLabels(
            gitOpsManaged=True,
            gitOpsTool="argocd",
            pdbProtected=True,
        )

        instructions = build_mcp_filter_instructions(detected_labels)
        assert "search_workflow_catalog" not in instructions

    def test_incident_validation_feedback_no_search_workflow_catalog(self):
        """Validation error feedback must not reference old tool name."""
        from src.extensions.incident import build_validation_error_feedback

        feedback = build_validation_error_feedback(
            errors=["workflow_id not found"],
            attempt=0,
        )
        assert "search_workflow_catalog" not in feedback


# ============================================================
# UT-HAPI-017-002-004: Recovery prompt does NOT contain search_workflow_catalog
# ============================================================

class TestRecoveryPromptNoOldToolName:
    """
    UT-HAPI-017-002-004: Recovery prompt does NOT contain search_workflow_catalog

    Business Outcome: Old tool name is fully removed from recovery prompts,
    preventing LLM from attempting to call the superseded tool.

    BR: BR-HAPI-017-002, BR-HAPI-017-006
    DD: DD-HAPI-017
    """

    def test_recovery_prompt_no_search_workflow_catalog(self):
        """Recovery prompt must not reference old tool name."""
        from src.extensions.recovery import _create_recovery_investigation_prompt

        prompt = _create_recovery_investigation_prompt(_make_recovery_request_data())
        assert "search_workflow_catalog" not in prompt

    def test_recovery_mcp_filter_instructions_no_search_workflow_catalog(self):
        """Recovery MCP filter instructions must not reference old tool name."""
        from src.extensions.recovery import _build_mcp_filter_instructions
        from src.models.incident_models import DetectedLabels

        detected_labels = DetectedLabels(
            gitOpsManaged=True,
            gitOpsTool="argocd",
            stateful=True,
        )

        instructions = _build_mcp_filter_instructions(detected_labels)
        assert "search_workflow_catalog" not in instructions

    def test_legacy_investigation_prompt_no_search_workflow_catalog(self):
        """Legacy _create_investigation_prompt must not reference old tool name."""
        from src.extensions.recovery import _create_investigation_prompt

        prompt = _create_investigation_prompt(_make_recovery_request_data())
        assert "search_workflow_catalog" not in prompt


# ============================================================
# UT-HAPI-017-002-005: Step 2 includes "review ALL workflows" mandate
# ============================================================

class TestStepTwoReviewAllMandate:
    """
    UT-HAPI-017-002-005: Step 2 includes "review ALL workflows" mandate

    Business Outcome: LLM is explicitly instructed to review ALL pages
    of workflows before making a selection, preventing premature first-page
    selection bias.

    BR: BR-HAPI-017-002
    DD: DD-HAPI-017, DD-WORKFLOW-016
    """

    def test_incident_prompt_review_all_mandate(self):
        """Incident prompt instructs LLM to review ALL workflows."""
        from src.extensions.incident import create_incident_investigation_prompt

        prompt = create_incident_investigation_prompt(_make_incident_request_data())
        prompt_lower = prompt.lower()

        # Must instruct to review all workflows/pages
        assert "all" in prompt_lower and "workflow" in prompt_lower
        # Must mention pagination behavior (hasMore)
        assert "hasmore" in prompt_lower or "has_more" in prompt_lower

    def test_recovery_prompt_review_all_mandate(self):
        """Recovery prompt instructs LLM to review ALL workflows."""
        from src.extensions.recovery import _create_recovery_investigation_prompt

        prompt = _create_recovery_investigation_prompt(_make_recovery_request_data())
        prompt_lower = prompt.lower()

        # Must instruct to review all workflows/pages
        assert "all" in prompt_lower and "workflow" in prompt_lower
        # Must mention pagination behavior (hasMore)
        assert "hasmore" in prompt_lower or "has_more" in prompt_lower

    def test_legacy_investigation_prompt_review_all_mandate(self):
        """Legacy prompt instructs LLM to review ALL workflows."""
        from src.extensions.recovery import _create_investigation_prompt

        prompt = _create_investigation_prompt(_make_recovery_request_data())
        prompt_lower = prompt.lower()

        # Must instruct to review all workflows/pages
        assert "all" in prompt_lower and "workflow" in prompt_lower
        # Must mention pagination behavior (hasMore)
        assert "hasmore" in prompt_lower or "has_more" in prompt_lower
