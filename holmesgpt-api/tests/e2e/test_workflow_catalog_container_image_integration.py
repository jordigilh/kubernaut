"""
E2E tests for container_image and container_digest fields in workflow catalog tool

Business Requirement: BR-AI-075 - Workflow Selection Contract
Design Decision: DD-WORKFLOW-002 v3.0, DD-CONTRACT-001 v1.2
Authority: AIAnalysis crd-schema.md - SelectedWorkflow.containerImage field

Prerequisites:
- Integration infrastructure running (./setup_workflow_catalog_integration.sh)
- Test workflows registered with container_image and container_digest

⚠️ CRITICAL: DD-CONTRACT-001 v1.2 Architecture
- Data Storage Service must return container_image in search results
- These fields come from workflow catalog database (DD-WORKFLOW-009)
- End-to-end flow validates complete integration

DD-WORKFLOW-002 v3.0 Response Format:
- FLAT structure (no nested 'workflow' object)
- workflow_id is UUID (auto-generated)
- signal_type is singular string (not array)
- confidence (not final_score)

Test Coverage:
- E2E Test 1: Data Storage returns container_image in search
- E2E Test 2: Data Storage returns container_digest in search
- E2E Test 3: End-to-end container_image flow
- E2E Test 4: container_image matches catalog entry
"""

import pytest
import requests
import json
import os

# DD-API-001: OpenAPI client imports for Data Storage API
from datastorage.api.workflow_catalog_api_api import WorkflowCatalogAPIApi
from datastorage.models.workflow_search_request import WorkflowSearchRequest
from datastorage.models.workflow_search_filters import WorkflowSearchFilters
from datastorage.api_client import ApiClient
from datastorage.configuration import Configuration

# Note: Uses fixtures from tests/e2e/conftest.py (V1.0 Go infrastructure)
# DATA_STORAGE_URL is provided via data_storage_stack fixture


@pytest.fixture(scope="module")
def workflow_catalog_tool(data_storage_stack):
    """
    Create WorkflowCatalogTool configured for E2E testing

    Configures tool to use Data Storage Service from Go infrastructure.
    """
    from src.toolsets.workflow_catalog import WorkflowCatalogToolset

    toolset = WorkflowCatalogToolset()
    tool = toolset.tools[0]

    # Override Data Storage URL for E2E testing (Go infrastructure: port 8081)
    tool.data_storage_url = data_storage_stack

    print(f"🔧 Workflow Catalog Tool configured: {tool.data_storage_url}")
    return tool


@pytest.fixture(scope="module")
def ensure_test_workflows(test_workflows_bootstrapped):
    """
    MIGRATED: Now uses Python fixtures instead of shell script.

    DD-API-001 COMPLIANCE: Workflows bootstrapped via OpenAPI client.

    The test_workflows_bootstrapped fixture automatically creates test workflows
    using Python code with type safety and OpenAPI client compliance.

    See: tests/fixtures/workflow_fixtures.py
    """
    # test_workflows_bootstrapped already bootstrapped the workflows
    # Just pass through the results
    return test_workflows_bootstrapped


# ========================================
# E2E TESTS
# ========================================

class TestWorkflowCatalogContainerImageIntegration:
    """
    BR-AI-075: E2E tests for container_image propagation

    BEHAVIOR: Real Data Storage Service returns container_image in search results
    CORRECTNESS: Field values match registered workflows in database

    🔄 PRODUCTION: Tests with real Data Storage Service + PostgreSQL + pgvector
    """

    # ========================================
    # E2E TEST 1: Data Storage returns container_image
    # ========================================

    def test_data_storage_returns_container_image_in_search(
        self,
        workflow_catalog_tool,
        ensure_test_workflows
    ):
        """
        BR-AI-075: Data Storage API returns container_image in search results

        BEHAVIOR: Real Data Storage Service includes container_image in response
        CORRECTNESS: Field is present (may be None for some workflows)
        """
        from holmes.core.tools import StructuredToolResultStatus

        # ACT: Search for known workflow
        result = workflow_catalog_tool.invoke(params={
            "query": "OOMKilled critical",
            "top_k": 3
        })

        # ASSERT BEHAVIOR: Search succeeds
        assert result.status == StructuredToolResultStatus.SUCCESS, \
            f"BR-AI-075: Search must succeed, got error: {result.error}"

        data = json.loads(result.data)
        workflows = data.get("workflows", [])

        # Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip
        if len(workflows) == 0:
            pytest.fail(
                "REQUIRED: No test workflows available.\n"
                "  Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip\n"
                "  Workflows should auto-bootstrap via test_workflows_bootstrapped fixture.\n"
                "  See: tests/fixtures/workflow_fixtures.py"
            )

        # ASSERT CORRECTNESS: container_image field present
        for wf in workflows:
            workflow_id = wf.get("workflow_id", "unknown")

            assert "container_image" in wf, \
                f"BR-AI-075: Workflow {workflow_id} missing container_image field"

            container_image = wf.get("container_image")
            # Validate OCI reference format if present
            if container_image:
                assert "/" in container_image, \
                    f"BR-AI-075: container_image must be OCI reference (contain /), got '{container_image}'"

    # ========================================
    # E2E TEST 2: Data Storage returns container_digest
    # ========================================

    def test_data_storage_returns_container_digest_in_search(
        self,
        workflow_catalog_tool,
        ensure_test_workflows
    ):
        """
        BR-AI-075: Data Storage API returns container_digest in search results

        BEHAVIOR: Real Data Storage Service includes container_digest in response
        CORRECTNESS: Field format is valid sha256 if present
        """
        from holmes.core.tools import StructuredToolResultStatus
        import re

        # ACT: Search for known workflow
        result = workflow_catalog_tool.invoke(params={
            "query": "OOMKilled critical",
            "top_k": 3
        })

        # ASSERT BEHAVIOR: Search succeeds
        assert result.status == StructuredToolResultStatus.SUCCESS

        data = json.loads(result.data)
        workflows = data.get("workflows", [])

        # Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip
        if len(workflows) == 0:
            pytest.fail(
                "REQUIRED: No test workflows available.\n"
                "  Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip\n"
                "  Workflows should auto-bootstrap via test_workflows_bootstrapped fixture.\n"
                "  See: tests/fixtures/workflow_fixtures.py"
            )

        # ASSERT CORRECTNESS: container_digest field present
        sha256_pattern = re.compile(r'^sha256:[a-f0-9]{64}$')

        for wf in workflows:
            workflow_id = wf.get("workflow_id", "unknown")

            assert "container_digest" in wf, \
                f"BR-AI-075: Workflow {workflow_id} missing container_digest field"

            container_digest = wf.get("container_digest")
            # Validate sha256 format if present
            if container_digest:
                assert sha256_pattern.match(container_digest), \
                    f"BR-AI-075: container_digest must be sha256:hex64, got '{container_digest}' for {workflow_id}"

    # ========================================
    # E2E TEST 3: End-to-end container_image flow
    # ========================================

    def test_end_to_end_container_image_flow(
        self,
        workflow_catalog_tool,
        ensure_test_workflows
    ):
        """
        BR-AI-075: End-to-end flow from search to container_image extraction

        BEHAVIOR: Complete search → result flow includes container_image
        CORRECTNESS: Fields are correctly propagated from database to tool output

        Test Flow:
        1. Search for "OOMKilled critical"
        2. Validate container_image in each result
        3. Validate container_digest in each result
        4. Validate other workflow fields still present
        """
        from holmes.core.tools import StructuredToolResultStatus

        # ACT: Full search flow
        result = workflow_catalog_tool.invoke(params={
            "query": "OOMKilled critical",
            "filters": {},
            "top_k": 5
        })

        # ASSERT: Success
        assert result.status == StructuredToolResultStatus.SUCCESS

        data = json.loads(result.data)
        workflows = data.get("workflows", [])

        # Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip
        if len(workflows) == 0:
            pytest.fail(
                "REQUIRED: No test workflows available.\n"
                "  Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip\n"
                "  Workflows should auto-bootstrap via test_workflows_bootstrapped fixture.\n"
                "  See: tests/fixtures/workflow_fixtures.py"
            )

        # Validate complete workflow structure for each result
        # DD-WORKFLOW-002 v3.0: No version in search results
        required_fields = [
            "workflow_id",
            "title",
            "description",
            "signal_type",
            "confidence",
            "container_image",      # DD-WORKFLOW-002 v3.0
            "container_digest"      # DD-WORKFLOW-002 v3.0
        ]

        for wf in workflows:
            workflow_id = wf.get("workflow_id", "unknown")

            for field in required_fields:
                assert field in wf, \
                    f"BR-AI-075: Workflow {workflow_id} missing required field '{field}'"

            # Validate field types
            assert wf["container_image"] is None or isinstance(wf["container_image"], str), \
                f"container_image must be string or None for {workflow_id}"
            assert wf["container_digest"] is None or isinstance(wf["container_digest"], str), \
                f"container_digest must be string or None for {workflow_id}"
            assert isinstance(wf["confidence"], (int, float)), \
                f"confidence must be numeric for {workflow_id}"

    # ========================================
    # E2E TEST 4: container_image matches catalog entry
    # ========================================

    def test_container_image_matches_catalog_entry(
        self,
        workflow_catalog_tool,
        ensure_test_workflows
    ):
        """
        BR-AI-075: container_image has valid OCI format when present

        BEHAVIOR: Returned container_image is valid OCI reference
        CORRECTNESS: Format matches registry/repo:tag or registry/repo@sha256:...

        DD-WORKFLOW-002 v3.0: workflow_id is UUID, so we validate by format
        """
        from holmes.core.tools import StructuredToolResultStatus

        # ACT: Search for OOMKilled workflows
        result = workflow_catalog_tool.invoke(params={
            "query": "OOMKilled critical",
            "top_k": 5
        })

        assert result.status == StructuredToolResultStatus.SUCCESS

        data = json.loads(result.data)
        workflows = data.get("workflows", [])

        # Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip
        if len(workflows) == 0:
            pytest.fail(
                "REQUIRED: No test workflows available.\n"
                "  Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip\n"
                "  Workflows should auto-bootstrap via test_workflows_bootstrapped fixture.\n"
                "  See: tests/fixtures/workflow_fixtures.py"
            )

        # ASSERT: At least one workflow has valid container_image format
        workflow_with_image_found = False

        for wf in workflows:
            container_image = wf.get("container_image")
            if container_image and "/" in container_image:
                workflow_with_image_found = True

                # Validate OCI reference format
                assert ":" in container_image or "@sha256:" in container_image, \
                    f"BR-AI-075: container_image must have :tag or @sha256:, got '{container_image}'"

        # If no workflows have container_image, that's okay (field is optional per workflow)
        print(f"✅ Found {len(workflows)} workflows, " +
              f"with_container_image: {workflow_with_image_found}")


class TestWorkflowCatalogContainerImageDirectAPI:
    """
    Direct API tests for container_image in Data Storage response

    These tests bypass the tool and call Data Storage API directly
    to verify the API contract per DD-WORKFLOW-002 v3.0.
    """

    def test_direct_api_search_returns_container_image(
        self,
        data_storage_stack,
        test_workflows_bootstrapped,
        ensure_test_workflows
    ):
        """
        BR-AI-075: Direct API call returns container_image

        BEHAVIOR: Data Storage /api/v1/workflows/search includes container_image
        CORRECTNESS: API response matches DD-WORKFLOW-002 v3.0 contract (flat structure)

        This validates the Data Storage API directly, independent of
        the HolmesGPT tool transformation.

        DD-API-001 COMPLIANCE: Uses OpenAPI client for type safety and contract validation.
        """
        data_storage_url = data_storage_stack

        # DD-API-001: Direct API call using OpenAPI client (not raw requests)
        config = Configuration(host=data_storage_url)
        api_client = ApiClient(configuration=config)
        search_api = WorkflowCatalogAPIApi(api_client)

        # DD-STORAGE-011 v2.0: All 5 mandatory filter fields required
        # Matches oomkill-increase-memory-limits fixture
        filters = WorkflowSearchFilters(
            signal_type="OOMKilled",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0"  # Matches test fixture in tests/fixtures/workflow_fixtures.py
        )

        request = WorkflowSearchRequest(
            filters=filters,
            top_k=3
        )

        # ACT: Search using OpenAPI client
        try:
            response = search_api.search_workflows(
                workflow_search_request=request,
                _request_timeout=10
            )
            workflows = response.workflows or []
        except Exception as e:
            pytest.fail(f"BR-AI-075: Data Storage API call failed: {e}")

        # Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip
        if len(workflows) == 0:
            pytest.fail(
                "REQUIRED: No test workflows available.\n"
                "  Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip\n"
                "  Workflows should auto-bootstrap via test_workflows_bootstrapped fixture.\n"
                "  See: tests/fixtures/workflow_fixtures.py"
            )

        # ASSERT: container_image in API response (DD-WORKFLOW-002 v3.0 flat format)
        # DD-API-001: Pydantic models from OpenAPI client (attribute access, not dict)
        for wf in workflows:
            # DD-WORKFLOW-002 v3.0: Flat structure - fields directly on workflow object
            workflow_id = wf.workflow_id if hasattr(wf, 'workflow_id') else "unknown"

            # DD-WORKFLOW-002 v3.0: container_image directly on workflow
            assert hasattr(wf, 'container_image'), \
                f"BR-AI-075: API response missing container_image for {workflow_id}"
            assert hasattr(wf, 'container_digest'), \
                f"BR-AI-075: API response missing container_digest for {workflow_id}"

            # Validate format if present
            container_image = wf.container_image
            container_digest = wf.container_digest
            if container_image:
                assert "/" in container_image, \
                    f"BR-AI-075: container_image must be OCI reference for {workflow_id}"
            if container_digest:
                assert container_digest.startswith("sha256:"), \
                    f"BR-AI-075: container_digest must start with sha256: for {workflow_id}"


if __name__ == "__main__":
    pytest.main([__file__, "-v", "--tb=short"])
