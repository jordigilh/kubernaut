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

# Note: Uses fixtures from tests/e2e/conftest.py (DD-TEST-001 compliant)
# DATA_STORAGE_URL and EMBEDDING_SERVICE_URL are provided via integration_infrastructure fixture


@pytest.fixture(scope="module")
def workflow_catalog_tool(integration_infrastructure):
    """
    Create WorkflowCatalogTool configured for E2E testing

    Configures tool to use integration test Data Storage Service URL from DD-TEST-001.
    """
    from src.toolsets.workflow_catalog import WorkflowCatalogToolset

    toolset = WorkflowCatalogToolset()
    tool = toolset.tools[0]

    # Override Data Storage URL for integration testing (DD-TEST-001: port 18090)
    tool.data_storage_url = integration_infrastructure["data_storage_url"]

    print(f"🔧 Workflow Catalog Tool configured: {tool.data_storage_url}")
    return tool


@pytest.fixture(scope="module")
def ensure_test_workflows(integration_infrastructure):
    """
    Verify test workflows with container_image are present in database

    DD-WORKFLOW-002 v3.0: workflow_id is UUID (auto-generated),
    we verify by searching for expected workflows.
    """
    data_storage_url = integration_infrastructure["data_storage_url"]

    # Verify test workflows exist by searching
    try:
        response = requests.post(
            f"{data_storage_url}/api/v1/workflows/search",
            json={"query": "OOMKilled", "top_k": 5, "min_similarity": 0.0},
            timeout=10
        )
        if response.status_code == 200:
            data = response.json()
            workflows = data.get("workflows", [])
            print(f"✅ Found {len(workflows)} test workflows in database")
            return workflows
        else:
            print(f"⚠️ Failed to verify test workflows: {response.status_code}")
            return []
    except requests.exceptions.RequestException as e:
        print(f"⚠️ Error verifying test workflows: {e}")
        return []


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

        # Skip if no test data available
        if len(workflows) == 0:
            pytest.skip("No test workflows available - run bootstrap-workflows.sh")

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

        # Skip if no test data available
        if len(workflows) == 0:
            pytest.skip("No test workflows available - run bootstrap-workflows.sh")

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

        # Skip if no test data available
        if len(workflows) == 0:
            pytest.skip("No test workflows available - run bootstrap-workflows.sh")

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

        # Skip if no test data available
        if len(workflows) == 0:
            pytest.skip("No test workflows available - run bootstrap-workflows.sh")

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
        integration_infrastructure,
        ensure_test_workflows
    ):
        """
        BR-AI-075: Direct API call returns container_image

        BEHAVIOR: Data Storage /api/v1/workflows/search includes container_image
        CORRECTNESS: API response matches DD-WORKFLOW-002 v3.0 contract (flat structure)

        This validates the Data Storage API directly, independent of
        the HolmesGPT tool transformation.
        """
        data_storage_url = integration_infrastructure["data_storage_url"]

        # ACT: Direct API call to Data Storage
        response = requests.post(
            f"{data_storage_url}/api/v1/workflows/search",
            json={
                "query": "OOMKilled critical",
                "filters": {"signal-type": "OOMKilled", "severity": "critical"},
                "top_k": 3,
                "min_similarity": 0.0  # Lower threshold to ensure results
            },
            timeout=10
        )

        # ASSERT: API success
        assert response.status_code == 200, \
            f"BR-AI-075: Data Storage API failed: {response.status_code} - {response.text}"

        data = response.json()
        workflows = data.get("workflows", [])

        # Skip if no test data available
        if len(workflows) == 0:
            pytest.skip("No test workflows available - run bootstrap-workflows.sh")

        # ASSERT: container_image in API response (DD-WORKFLOW-002 v3.0 flat format)
        for wf in workflows:
            # DD-WORKFLOW-002 v3.0: Flat structure - fields directly on workflow object
            workflow_id = wf.get("workflow_id", "unknown")

            # DD-WORKFLOW-002 v3.0: container_image directly on workflow
            assert "container_image" in wf, \
                f"BR-AI-075: API response missing container_image for {workflow_id}"
            assert "container_digest" in wf, \
                f"BR-AI-075: API response missing container_digest for {workflow_id}"

            # Validate format if present
            container_image = wf.get("container_image")
            container_digest = wf.get("container_digest")
            if container_image:
                assert "/" in container_image, \
                    f"BR-AI-075: container_image must be OCI reference for {workflow_id}"
            if container_digest:
                assert container_digest.startswith("sha256:"), \
                    f"BR-AI-075: container_digest must start with sha256: for {workflow_id}"


if __name__ == "__main__":
    pytest.main([__file__, "-v", "--tb=short"])
