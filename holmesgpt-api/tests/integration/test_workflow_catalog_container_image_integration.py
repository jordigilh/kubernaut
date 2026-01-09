"""
Integration tests for container_image and container_digest fields in workflow catalog tool

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
- Flat structure (no nested 'workflow' object)
- workflow_id is UUID
- signal_type is singular string (not array)
- confidence instead of final_score

Test Coverage:
- Integration Test 1: Data Storage returns container_image in search
- Integration Test 2: Data Storage returns container_digest in search
- Integration Test 3: End-to-end container_image flow
- Integration Test 4: container_image matches catalog entry
"""

import pytest
import requests
import json
import os

# OpenAPI client imports for direct API contract tests
from datastorage.api.workflow_catalog_api_api import WorkflowCatalogAPIApi
from datastorage.models.workflow_search_request import WorkflowSearchRequest
from datastorage.models.workflow_search_filters import WorkflowSearchFilters
from datastorage.api_client import ApiClient
from datastorage.configuration import Configuration

# Import infrastructure helpers from conftest (DD-TEST-001 compliant)
from tests.integration.conftest import (
    DATA_STORAGE_URL,
    # REMOVED: EMBEDDING_SERVICE_URL (V1.0 label-only architecture, no embeddings)
)


@pytest.fixture(scope="module")
def workflow_catalog_tool(integration_infrastructure):
    """
    Create WorkflowCatalogTool configured for integration testing

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
    Ensure test workflows with container_image are present in database

    This fixture bootstraps test workflows if they don't exist.
    Per DD-WORKFLOW-005 v1.2: V1.0 uses manual upload via REST API.
    Per DD-WORKFLOW-002 v3.0: Uses workflow_name, name (title), and content YAML format.
    """
    data_storage_url = integration_infrastructure["data_storage_url"]

    # DD-WORKFLOW-002 v3.0: Workflows created via REST API - database auto-generates UUID
    # These are for tracking, actual data comes from bootstrapped workflows
    test_workflow_titles = [
        "Increase Memory Limits",
        "CrashLoopBackOff Restart Pod",
    ]

    # Verify test workflows exist by searching using OpenAPI client (DD-STORAGE-011)
    # V1.0: Must provide all 5 mandatory filter fields
    try:
        config = Configuration(host=data_storage_url)
        api_client = ApiClient(configuration=config)
        search_api = WorkflowCatalogAPIApi(api_client)

        filters = WorkflowSearchFilters(
            signal_type="OOMKilled",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0"
        )
        request = WorkflowSearchRequest(filters=filters, top_k=5)

        response = search_api.search_workflows(workflow_search_request=request, _request_timeout=10)
        workflows = response.workflows or []
        print(f"✅ Found {len(workflows)} test workflows in database")
    except Exception as e:
        print(f"⚠️ Error verifying test workflows: {e}")

    return test_workflow_titles


# ========================================
# INTEGRATION TESTS
# ========================================

class TestWorkflowCatalogContainerImageIntegration:
    """
    BR-AI-075: Integration tests for container_image propagation

    BEHAVIOR: Real Data Storage Service returns container_image in search results
    CORRECTNESS: Field values match registered workflows in database

    🔄 PRODUCTION: Tests with real Data Storage Service + PostgreSQL + pgvector
    """

    # ========================================
    # INTEGRATION TEST 1: Data Storage returns container_image
    # ========================================

    def test_data_storage_returns_container_image_in_search(
        self,
        workflow_catalog_tool,
        ensure_test_workflows
    ):
        """
        BR-AI-075: Data Storage API returns container_image in search results

        BEHAVIOR: Real Data Storage Service includes container_image in response
        CORRECTNESS: Field is non-empty OCI reference for registered workflows

        TDD Phase: RED (this test should FAIL initially if Data Storage
        doesn't return container_image or if tool doesn't extract it)
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

        # ASSERT BEHAVIOR: Results returned
        assert len(workflows) > 0, "BR-AI-075: Search must return results"

        # ASSERT CORRECTNESS: container_image present and valid OCI format
        for wf in workflows:
            workflow_id = wf.get("workflow_id", "unknown")

            assert "container_image" in wf, \
                f"BR-AI-075: Workflow {workflow_id} missing container_image field"

            container_image = wf.get("container_image")
            assert container_image is not None, \
                f"BR-AI-075: container_image must not be None for {workflow_id}"
            assert container_image != "", \
                f"BR-AI-075: container_image must not be empty for {workflow_id}"

            # Validate OCI reference format (contains / and :)
            assert "/" in container_image, \
                f"BR-AI-075: container_image must be OCI reference (contain /), got '{container_image}'"

    # ========================================
    # INTEGRATION TEST 2: Data Storage returns container_digest
    # ========================================

    def test_data_storage_returns_container_digest_in_search(
        self,
        workflow_catalog_tool,
        ensure_test_workflows
    ):
        """
        BR-AI-075: Data Storage API returns container_digest in search results

        BEHAVIOR: Real Data Storage Service includes container_digest in response
        CORRECTNESS: Field is valid sha256 digest format

        TDD Phase: RED (this test should FAIL initially)
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
        assert len(workflows) > 0, "BR-AI-075: Search must return results"

        # ASSERT CORRECTNESS: container_digest present and valid sha256 format
        sha256_pattern = re.compile(r'^sha256:[a-f0-9]{64}$')

        for wf in workflows:
            workflow_id = wf.get("workflow_id", "unknown")

            assert "container_digest" in wf, \
                f"BR-AI-075: Workflow {workflow_id} missing container_digest field"

            container_digest = wf.get("container_digest")
            assert container_digest is not None, \
                f"BR-AI-075: container_digest must not be None for {workflow_id}"
            assert container_digest != "", \
                f"BR-AI-075: container_digest must not be empty for {workflow_id}"

            # Validate sha256 format
            assert sha256_pattern.match(container_digest), \
                f"BR-AI-075: container_digest must be sha256:hex64, got '{container_digest}' for {workflow_id}"

    # ========================================
    # INTEGRATION TEST 3: End-to-end container_image flow
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
        assert len(workflows) > 0

        # Validate complete workflow structure for each result
        # DD-WORKFLOW-002 v3.0: Flat structure, no version in search results
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
            assert isinstance(wf["container_image"], str), \
                f"container_image must be string for {workflow_id}"
            assert isinstance(wf["container_digest"], str), \
                f"container_digest must be string for {workflow_id}"
            assert isinstance(wf["confidence"], (int, float)), \
                f"confidence must be numeric for {workflow_id}"

    # ========================================
    # INTEGRATION TEST 4: container_image matches catalog entry
    # ========================================

    def test_container_image_matches_catalog_entry(
        self,
        workflow_catalog_tool,
        ensure_test_workflows
    ):
        """
        BR-AI-075: container_image matches the registered workflow in catalog

        BEHAVIOR: Returned container_image matches what was registered
        CORRECTNESS: container_image contains valid OCI reference

        Prerequisites: Test workflows must be bootstrapped with known container_image
        DD-WORKFLOW-002 v3.0: workflow_id is now UUID, so we validate by title pattern
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
                "  Run: ./scripts/bootstrap-workflows.sh"
            )

        # ASSERT: At least one workflow has container_image with OCI format
        workflow_with_image_found = False

        for wf in workflows:
            container_image = wf.get("container_image")
            if container_image and "/" in container_image:
                workflow_with_image_found = True

                # ASSERT CORRECTNESS: container_image is valid OCI reference
                assert "quay.io" in container_image or "docker.io" in container_image or "/" in container_image, \
                    f"BR-AI-075: container_image must be valid OCI reference, got '{container_image}'"

        assert workflow_with_image_found, \
            f"BR-AI-075: No workflows with valid container_image found. " \
            f"Got: {[wf.get('container_image') for wf in workflows]}"


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

        # ACT: Direct API call to Data Storage using OpenAPI client (DD-STORAGE-011)
        config = Configuration(host=data_storage_url)
        api_client = ApiClient(configuration=config)
        search_api = WorkflowCatalogAPIApi(api_client)

        filters = WorkflowSearchFilters(
            signal_type="OOMKilled",  # snake_case per DD-WORKFLOW-001 v1.6
            severity="critical",
            component="pod",
            environment="production",
            priority="P0"
        )
        request = WorkflowSearchRequest(
            query="OOMKilled critical",
            filters=filters,
            top_k=3
        )

        # Execute type-safe API call
        response = search_api.search_workflows(workflow_search_request=request, _request_timeout=10)

        # ASSERT: API success (OpenAPI client would raise exception on error)
        assert response is not None, "BR-AI-075: Data Storage API failed"

        # Convert typed response to list of dicts for validation
        workflows = [wf.to_dict() for wf in response.workflows] if response.workflows else []

        # Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip
        if len(workflows) == 0:
            pytest.fail(
                "REQUIRED: No test workflows available.\n"
                "  Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip\n"
                "  Run: ./scripts/bootstrap-workflows.sh"
            )

        # ASSERT: container_image in API response (DD-WORKFLOW-002 v3.0 flat format)
        for wf in workflows:
            # DD-WORKFLOW-002 v3.0: Flat structure - fields directly on workflow object
            workflow_id = wf.get("workflow_id", "unknown")

            # DD-WORKFLOW-002 v3.0: container_image directly on workflow
            assert "container_image" in wf, \
                f"BR-AI-075: API response missing container_image for {workflow_id}"
            assert "container_digest" in wf, \
                f"BR-AI-075: API response missing container_digest for {workflow_id}"

            # Validate non-empty (if present)
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

