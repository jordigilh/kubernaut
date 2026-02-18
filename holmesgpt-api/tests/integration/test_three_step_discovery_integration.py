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
Integration Tests: Three-Step Workflow Discovery (DD-HAPI-017)

Business Requirements:
- BR-HAPI-017-001: Three-step tool implementation

Design Decisions:
- DD-WORKFLOW-016: Action-Type Workflow Catalog Indexing
- DD-HAPI-017: Three-Step Workflow Discovery Integration

Test IDs: IT-HAPI-017-001-001 through IT-HAPI-017-001-004

Prerequisites:
    Real Data Storage with PostgreSQL (started by Go infrastructure).
    Migration 025 applied (action_type_taxonomy seeded).

Run:
    python -m pytest tests/integration/test_three_step_discovery_integration.py -v
"""

import json
import pytest

from holmes.core.tools import StructuredToolResultStatus

from src.toolsets.workflow_discovery import (
    ListAvailableActionsTool,
    ListWorkflowsTool,
    GetWorkflowTool,
)

from tests.fixtures.workflow_fixtures import (
    TEST_WORKFLOWS,
    PAGINATION_WORKFLOWS,
    ALL_TEST_WORKFLOWS,
    bootstrap_workflows,
    bootstrap_action_type_taxonomy,
    ACTION_TYPE_SCALE_REPLICAS,
    get_workflows_by_action_type,
)

from tests.integration.conftest import (
    DATA_STORAGE_URL,
    create_authenticated_http_session,
    is_integration_infra_available,
)


# ========================================
# FIXTURES
# ========================================

@pytest.fixture(scope="module")
def auth_session():
    """Authenticated requests.Session for discovery tool HTTP calls (DD-AUTH-014)."""
    session = create_authenticated_http_session()
    yield session
    if session:
        session.close()


@pytest.fixture(scope="module")
def seeded_workflows(data_storage_url):
    """
    Bootstrap test workflows into Data Storage for discovery tests.

    Seeds both base TEST_WORKFLOWS and PAGINATION_WORKFLOWS to enable
    all four integration test scenarios including pagination.

    Returns:
        Dict with workflow_id_map and counts
    """
    # Verify taxonomy is available first
    taxonomy = bootstrap_action_type_taxonomy(data_storage_url)
    assert taxonomy["available"], (
        "Action type taxonomy not available in DS. "
        "Ensure Go infrastructure is running with migration 025 applied."
    )

    # Seed all test workflows (base + pagination = 31 total)
    results = bootstrap_workflows(data_storage_url, workflows=ALL_TEST_WORKFLOWS)

    total_ok = len(results["created"]) + len(results["existing"])
    assert total_ok > 0, f"No workflows seeded: {results}"
    assert len(results["failed"]) == 0, f"Workflow seeding failures: {results['failed']}"

    return results


# ========================================
# IT-HAPI-017-001-001: Action type discovery against real DS
# ========================================

@pytest.mark.requires_data_storage
class TestActionTypeDiscovery:
    """
    BR-HAPI-017-001: ListAvailableActionsTool returns action types from real DS.

    IT-HAPI-017-001-001: Action type discovery against real DS.
    """

    def test_list_available_actions_returns_action_types(self, data_storage_url, seeded_workflows, auth_session):
        """
        IT-HAPI-017-001-001: ListAvailableActionsTool returns action types
        from real DS with bootstrapped taxonomy.
        """
        # ARRANGE
        tool = ListAvailableActionsTool(
            data_storage_url=data_storage_url,
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            http_session=auth_session,
        )

        # ACT
        result = tool._invoke(params={"limit": 100})

        # ASSERT
        assert result.status == StructuredToolResultStatus.SUCCESS, (
            f"Expected SUCCESS, got {result.status}: {result.error}"
        )

        data = json.loads(result.data)
        action_types = data.get("actionTypes", [])

        assert len(action_types) >= 1, "Expected at least 1 action type from taxonomy"

        # Each entry must have required fields
        for entry in action_types:
            assert "actionType" in entry, f"Missing actionType in entry: {entry}"
            assert "description" in entry, f"Missing description in entry: {entry}"
            assert "workflowCount" in entry, f"Missing workflowCount in entry: {entry}"
            assert entry["workflowCount"] >= 0


# ========================================
# IT-HAPI-017-001-002: Workflow listing for action type against real DS
# ========================================

@pytest.mark.requires_data_storage
class TestWorkflowListingByActionType:
    """
    BR-HAPI-017-001: ListWorkflowsTool returns workflows for a given action type.

    IT-HAPI-017-001-002: Workflow listing for action type against real DS.
    """

    def test_list_workflows_returns_workflows_for_action_type(self, data_storage_url, seeded_workflows, auth_session):
        """
        IT-HAPI-017-001-002: ListWorkflowsTool returns workflows from real DS
        for a given action type.
        """
        # ARRANGE — ScaleReplicas has at least 1 base + 6 pagination = 7 workflows
        tool = ListWorkflowsTool(
            data_storage_url=data_storage_url,
            severity="high",
            component="deployment",
            environment="production",
            priority="P1",
            http_session=auth_session,
        )

        # ACT
        result = tool._invoke(params={"action_type": ACTION_TYPE_SCALE_REPLICAS, "limit": 100})

        # ASSERT
        assert result.status == StructuredToolResultStatus.SUCCESS, (
            f"Expected SUCCESS, got {result.status}: {result.error}"
        )

        data = json.loads(result.data)
        workflows = data.get("workflows", [])

        assert len(workflows) >= 1, (
            f"Expected at least 1 workflow for {ACTION_TYPE_SCALE_REPLICAS}, got {len(workflows)}"
        )

        # Each workflow must have required discovery fields
        for wf in workflows:
            assert "workflowId" in wf, f"Missing workflowId in workflow: {wf}"
            assert "workflowName" in wf, f"Missing workflowName in workflow: {wf}"
            assert "name" in wf, f"Missing name in workflow: {wf}"
            assert "description" in wf, f"Missing description in workflow: {wf}"


# ========================================
# IT-HAPI-017-001-003: Single workflow retrieval against real DS
# ========================================

@pytest.mark.requires_data_storage
class TestSingleWorkflowRetrieval:
    """
    BR-HAPI-017-001: GetWorkflowTool returns full workflow detail from real DS.

    IT-HAPI-017-001-003: Single workflow retrieval against real DS.
    """

    def test_get_workflow_returns_full_detail(self, data_storage_url, seeded_workflows, auth_session):
        """
        IT-HAPI-017-001-003: GetWorkflowTool returns full workflow detail
        including parameter schema, container image, and action_type.
        """
        # ARRANGE — first discover a workflow_id via Step 2
        # DD-WORKFLOW-016: Use ScaleReplicas (high/deployment/production/P1)
        # which matches pagination-scale-* workflows seeded by Python fixtures.
        list_tool = ListWorkflowsTool(
            data_storage_url=data_storage_url,
            severity="high",
            component="deployment",
            environment="production",
            priority="P1",
            http_session=auth_session,
        )
        list_result = list_tool._invoke(params={"action_type": ACTION_TYPE_SCALE_REPLICAS, "limit": 10})
        assert list_result.status == StructuredToolResultStatus.SUCCESS, (
            f"Step 2 failed: {list_result.error}"
        )
        list_data = json.loads(list_result.data)
        workflows = list_data.get("workflows", [])
        assert len(workflows) >= 1, "No workflows found for ScaleReplicas"

        known_workflow_id = workflows[0]["workflowId"]

        # ACT — Step 3: get full workflow detail
        get_tool = GetWorkflowTool(
            data_storage_url=data_storage_url,
            severity="high",
            component="deployment",
            environment="production",
            priority="P1",
            http_session=auth_session,
        )
        result = get_tool._invoke(params={"workflow_id": known_workflow_id})

        # ASSERT
        assert result.status == StructuredToolResultStatus.SUCCESS, (
            f"Expected SUCCESS, got {result.status}: {result.error}"
        )

        data = json.loads(result.data)
        assert "actionType" in data, "Missing actionType in workflow detail"
        assert "containerImage" in data, "Missing containerImage in workflow detail"
        assert "content" in data, "Missing content (YAML) in workflow detail"
        assert data["workflowId"] == known_workflow_id


# ========================================
# IT-HAPI-017-001-004: Pagination with real DS
# ========================================

@pytest.mark.requires_data_storage
class TestPagination:
    """
    BR-HAPI-017-001: Pagination works with enough workflows to span multiple pages.

    IT-HAPI-017-001-004: Pagination with real DS.
    Requires 25+ bootstrapped workflows (provided by PAGINATION_WORKFLOWS).
    """

    def test_pagination_returns_multiple_pages(self, data_storage_url, seeded_workflows, auth_session):
        """
        IT-HAPI-017-001-004: Multi-page navigation works end-to-end.
        """
        # ARRANGE
        tool = ListAvailableActionsTool(
            data_storage_url=data_storage_url,
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            http_session=auth_session,
        )

        # ACT — Page 1 with small limit
        page1_result = tool._invoke(params={"offset": 0, "limit": 2})

        # ASSERT — Page 1
        assert page1_result.status == StructuredToolResultStatus.SUCCESS, (
            f"Page 1 failed: {page1_result.error}"
        )
        page1_data = json.loads(page1_result.data)
        page1_types = page1_data.get("actionTypes", [])
        pagination = page1_data.get("pagination", {})

        assert len(page1_types) <= 2, f"Expected at most 2 items, got {len(page1_types)}"
        total_count = pagination.get("totalCount", 0)

        # Only test multi-page if there are enough action types
        if total_count > 2:
            assert pagination.get("hasMore") is True, (
                f"Expected hasMore=true with totalCount={total_count} and limit=2"
            )

            # ACT — Page 2
            page2_result = tool._invoke(params={"offset": 2, "limit": 2})

            # ASSERT — Page 2
            assert page2_result.status == StructuredToolResultStatus.SUCCESS, (
                f"Page 2 failed: {page2_result.error}"
            )
            page2_data = json.loads(page2_result.data)
            page2_types = page2_data.get("actionTypes", [])

            assert len(page2_types) >= 1, "Expected at least 1 action type on page 2"

            # Verify no overlap between pages
            page1_names = {at["actionType"] for at in page1_types}
            page2_names = {at["actionType"] for at in page2_types}
            assert page1_names.isdisjoint(page2_names), (
                f"Pages overlap: {page1_names & page2_names}"
            )
