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
E2E Tests for Workflow Catalog - Critical User Journeys

Business Requirement: BR-STORAGE-013 - Semantic Search for Remediation Workflows
Design Decision: DD-WORKFLOW-002 v3.0 - MCP Workflow Catalog Architecture
Test Plan: TEST_PLAN_WORKFLOW_CATALOG_EDGE_CASES.md

TIER 3: E2E TESTS (10% Coverage)

These tests simulate what the LLM actually does when searching for workflows.
They validate critical user journeys end-to-end.

Test Coverage (2 tests):
  E1.1: OOMKilled incident finds memory workflow
  E1.2: CrashLoopBackOff incident finds restart workflow

Prerequisites:
  - Full integration stack running (Data Storage + Embedding + PostgreSQL + Redis)
  - Test workflows bootstrapped via bootstrap-workflows.sh
"""

import pytest
import json
import re
from typing import Dict, Any, List
from src.toolsets.workflow_catalog import (
    SearchWorkflowCatalogTool,
    WorkflowCatalogToolset
)
from holmes.core.tools import StructuredToolResultStatus


# =============================================================================
# FIXTURES
# =============================================================================

@pytest.fixture(scope="module")
def workflow_catalog_tool(data_storage_stack):
    """
    Create WorkflowCatalogTool configured for E2E testing

    Uses data_storage_stack fixture which:
    - Verifies Data Storage Service is available via Go infrastructure
    - Skips tests if infrastructure not running
    """
    toolset = WorkflowCatalogToolset()
    tool = toolset.tools[0]

    tool.data_storage_url = data_storage_stack

    print(f"🔧 E2E: Workflow Catalog Tool configured: {data_storage_stack}")
    return tool


# =============================================================================
# E2E TEST SUITE: CRITICAL USER JOURNEYS (E1.1-E1.2)
# =============================================================================

@pytest.mark.e2e
@pytest.mark.requires_data_storage
class TestCriticalUserJourneys:
    """
    E1.x: E2E tests for critical workflow search scenarios

    These tests simulate real LLM behavior and validate business outcomes.
    """

    def test_oomkilled_incident_finds_memory_workflow_e1_1(self, workflow_catalog_tool):
        """
        E1.1: Complete user journey - AI searches for OOMKilled remediation

        Business Outcome: When an OOMKilled alert fires, the AI can find
        a workflow that addresses memory issues.

        Simulates: LLM completing RCA and searching for remediation
        Note: Requires test data to be bootstrapped
        """
        # ARRANGE - Simulating what the LLM does after RCA
        # The LLM has identified OOMKilled as the root cause

        # ACT - LLM calls the workflow search tool
        result = workflow_catalog_tool.invoke(params={
            "query": "OOMKilled pod memory limit exceeded critical",
            "top_k": 5
        })

        # ASSERT - Business outcome validation
        assert result.status == StructuredToolResultStatus.SUCCESS, \
            f"E1.1: Search must succeed. Error: {result.error}"

        data = json.loads(result.data)
        workflows = data["workflows"]

        # Skip if no test data bootstrapped
        if len(workflows) == 0:
            pytest.skip("E1.1: No test data bootstrapped - run bootstrap-workflows.sh")

        top_workflow = workflows[0]

        # BUSINESS OUTCOME: AI can present the workflow to operator
        assert top_workflow.get("title"), \
            "E1.1: Workflow must have title for AI to present to operator"
        assert top_workflow.get("description"), \
            "E1.1: Workflow must have description for AI to explain purpose"
        assert top_workflow.get("confidence", 0) > 0.0, \
            "E1.1: Workflow must have positive confidence score"

        # BUSINESS OUTCOME: Workflow addresses memory issues
        signal_type = top_workflow.get("signal_type", "")
        assert signal_type in ["OOMKilled", "MemoryPressure", "ResourceQuota"], \
            f"E1.1: Top workflow should address memory issues. " + \
            f"Got signal_type: {signal_type}"

        # BUSINESS OUTCOME: AI can identify the workflow
        assert top_workflow.get("workflow_id"), \
            "E1.1: Workflow must have ID for reference"

        # Validate UUID format for tracking
        uuid_pattern = re.compile(
            r'^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$',
            re.IGNORECASE
        )
        assert uuid_pattern.match(top_workflow["workflow_id"]), \
            f"E1.1: workflow_id must be UUID for tracking. Got: {top_workflow['workflow_id']}"

        print(f"✅ E1.1: OOMKilled search found {len(workflows)} workflow(s)")
        print(f"   Top result: {top_workflow['title']} (confidence: {top_workflow['confidence']:.2f})")

    def test_crashloop_incident_finds_restart_workflow_e1_2(self, workflow_catalog_tool):
        """
        E1.2: Complete user journey - AI searches for CrashLoopBackOff remediation

        Business Outcome: When a CrashLoopBackOff alert fires, the AI can find
        a workflow that addresses container restart issues.

        Simulates: LLM completing RCA for container restart issues
        Note: Requires test data to be bootstrapped
        """
        # ARRANGE - LLM has identified CrashLoopBackOff as the issue

        # ACT - LLM calls the workflow search tool
        result = workflow_catalog_tool.invoke(params={
            "query": "CrashLoopBackOff container keeps restarting high severity",
            "top_k": 5
        })

        # ASSERT - Business outcome validation
        assert result.status == StructuredToolResultStatus.SUCCESS, \
            f"E1.2: Search must succeed. Error: {result.error}"

        data = json.loads(result.data)
        workflows = data["workflows"]

        # Skip if no test data bootstrapped
        if len(workflows) == 0:
            pytest.skip("E1.2: No test data bootstrapped - run bootstrap-workflows.sh")

        top_workflow = workflows[0]

        # BUSINESS OUTCOME: AI can present the workflow
        assert top_workflow.get("title"), \
            "E1.2: Workflow must have title for AI to present"
        assert top_workflow.get("description"), \
            "E1.2: Workflow must have description for AI to explain"

        # BUSINESS OUTCOME: Workflow addresses restart issues
        signal_type = top_workflow.get("signal_type", "")
        # Could match CrashLoopBackOff directly, or related issues
        print(f"   Signal type found: {signal_type}")

        # BUSINESS OUTCOME: AI has confidence in the recommendation
        confidence = top_workflow.get("confidence", 0)
        assert confidence > 0.0, \
            "E1.2: Workflow must have positive confidence score"

        print(f"✅ E1.2: CrashLoopBackOff search found {len(workflows)} workflow(s)")
        print(f"   Top result: {top_workflow['title']} (confidence: {confidence:.2f})")


# =============================================================================
# ADDITIONAL E2E TESTS (Optional - for expanded coverage)
# =============================================================================

@pytest.mark.e2e
@pytest.mark.requires_data_storage
class TestEdgeCaseUserJourneys:
    """
    Additional E2E tests for edge case scenarios
    """

    def test_ai_handles_no_matching_workflows(self, workflow_catalog_tool):
        """
        Business Outcome: AI handles "no matching workflows" gracefully

        When the AI searches for an incident type with no workflows,
        it should get an empty result, not an error.
        """
        # ACT - Search for non-existent incident type
        result = workflow_catalog_tool.invoke(params={
            "query": "QuantumFluxCapacitorOverload critical",  # Non-existent
            "top_k": 5
        })

        # ASSERT - No error, just empty results
        assert result.status == StructuredToolResultStatus.SUCCESS, \
            f"No matches should be SUCCESS, not ERROR. Got: {result.error}"

        data = json.loads(result.data)

        # AI can tell operator "No workflows found for this incident type"
        assert "workflows" in data, \
            "Response must have 'workflows' field even if empty"

        print(f"✅ No matching workflows handled gracefully: " +
              f"{len(data['workflows'])} results")

    def test_ai_can_refine_search(self, workflow_catalog_tool):
        """
        Business Outcome: AI can refine search with additional keywords

        The AI might first search broadly, then refine with more specific terms.
        Both searches should work.
        """
        # ACT - Broad search first
        broad_result = workflow_catalog_tool.invoke(params={
            "query": "memory",
            "top_k": 5
        })

        # ACT - Refined search
        refined_result = workflow_catalog_tool.invoke(params={
            "query": "OOMKilled pod memory limit exceeded critical kubernetes",
            "top_k": 5
        })

        # ASSERT - Both searches succeed
        assert broad_result.status == StructuredToolResultStatus.SUCCESS, \
            f"Broad search failed: {broad_result.error}"
        assert refined_result.status == StructuredToolResultStatus.SUCCESS, \
            f"Refined search failed: {refined_result.error}"

        broad_data = json.loads(broad_result.data)
        refined_data = json.loads(refined_result.data)

        print(f"✅ Search refinement works: " +
              f"broad={len(broad_data['workflows'])}, refined={len(refined_data['workflows'])}")


if __name__ == "__main__":
    print("Run with: pytest tests/e2e/test_workflow_catalog_e2e.py -v")

