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
- Incident prompts reference three-step discovery tools
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
# UT-HAPI-017-002-004: Incident prompt does NOT contain search_workflow_catalog
# ============================================================

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

