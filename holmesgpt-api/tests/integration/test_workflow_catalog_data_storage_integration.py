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
Integration Tests for Workflow Catalog with Real Data Storage Service

Business Requirement: BR-STORAGE-013 - Semantic Search for Remediation Workflows
Design Decisions:
  - DD-WORKFLOW-002 v3.0 - MCP Workflow Catalog Architecture (FLAT response format)
  - DD-STORAGE-008 - Workflow Catalog Schema
  - DD-TEST-001 - Port Allocation Strategy

🔄 PRODUCTION INTEGRATION TESTS

These tests validate the complete workflow search flow with real services:
  - Data Storage Service (Go + PostgreSQL + pgvector)
  - Embedding Service (Python + sentence-transformers)
  - HolmesGPT API Workflow Catalog Toolset (Python)

DD-WORKFLOW-002 v3.0 Response Format:
  - FLAT structure (no nested 'workflow' object)
  - workflow_id is UUID (auto-generated)
  - signal_type is singular string (not array)
  - confidence (not final_score, similarity_score, etc.)
  - No version/estimated_duration/success_rate in search results

Test Coverage (6 tests):
1. End-to-end workflow search with semantic similarity
2. Hybrid scoring validation (confidence score)
3. Empty results handling (no matching workflows)
4. Filter validation (mandatory and optional labels)
5. Top-k limiting (result count validation)
6. Error handling (service unavailable, timeout)

Prerequisites:
  - Docker/Podman running
  - Data Storage Service container (port 18090 per DD-TEST-001)
  - Embedding Service container (port 18000 per DD-TEST-001)
  - PostgreSQL with pgvector extension (port 15433 per DD-TEST-001)
  - Redis (port 16380 per DD-TEST-001)
  - Test data bootstrapped in database

Setup:
  Run: ./tests/integration/setup_workflow_catalog_integration.sh

Teardown:
  Run: ./tests/integration/teardown_workflow_catalog_integration.sh
"""

import pytest
import json
import time
import requests
from typing import Dict, Any, List
from src.toolsets.workflow_catalog import (
    SearchWorkflowCatalogTool,
    WorkflowCatalogToolset
)
from holmes.core.tools import StructuredToolResultStatus

# Import infrastructure helpers from conftest (DD-TEST-001 compliant)
from tests.integration.conftest import (
    DATA_STORAGE_URL,
    # REMOVED: EMBEDDING_SERVICE_URL (V1.0 label-only architecture, no embeddings)
    is_integration_infra_available,
)


# Test timeouts
SERVICE_STARTUP_TIMEOUT = 30  # seconds
HTTP_REQUEST_TIMEOUT = 10  # seconds


# ========================================
# FIXTURES
# ========================================

@pytest.fixture(scope="module")
def wait_for_services(integration_infrastructure):
    """
    Wait for Data Storage Service and Embedding Service to be ready

    This fixture depends on integration_infrastructure from conftest.py,
    which will skip tests if infrastructure is not available.
    """
    # infrastructure_infrastructure fixture already verified services are up
    data_storage_url = integration_infrastructure["data_storage_url"]
    # REMOVED: embedding_service_url (V1.0 label-only architecture, no embeddings)
    # embedding_url = integration_infrastructure.get("embedding_service_url")

    print(f"\n✅ Services ready:")
    print(f"   Data Storage: {data_storage_url}")
    # REMOVED: embedding service print (V1.0 label-only architecture)

    yield integration_infrastructure
    print(f"\n🧹 Integration tests complete")


@pytest.fixture(scope="module")
def workflow_catalog_tool(wait_for_services):
    """
    Create WorkflowCatalogTool configured for integration testing

    Configures tool to use integration test Data Storage Service URL from DD-TEST-001.
    """
    toolset = WorkflowCatalogToolset()
    tool = toolset.tools[0]

    # Override Data Storage URL for integration testing (DD-TEST-001: port 18090)
    data_storage_url = wait_for_services["data_storage_url"]
    tool.data_storage_url = data_storage_url

    print(f"🔧 Workflow Catalog Tool configured: {data_storage_url}")
    return tool


@pytest.fixture(scope="module")
def test_workflows(integration_infrastructure):
    """
    Test workflow data that should be bootstrapped in database.

    DD-API-001 COMPLIANCE: Bootstraps workflows using OpenAPI client instead of shell scripts.
    DD-WORKFLOW-002 v3.0: workflow_id is UUID (auto-generated),
    we only track expected titles for validation.

    This fixture automatically populates Data Storage with test workflows if not present,
    then returns expected workflow titles for test validation.
    """
    from tests.fixtures import bootstrap_workflows

    data_storage_url = integration_infrastructure["data_storage_url"]

    # Bootstrap workflows if not already present
    try:
        results = bootstrap_workflows(data_storage_url)
        print(f"\n✅ Workflows ready: {len(results['created'])} created, {len(results['existing'])} existing")
    except Exception as e:
        pytest.fail(f"❌ Failed to bootstrap workflows: {e}")

    # Return expected workflow titles for test validation
    return [
        "Increase Memory Limits",           # OOMKilled remediation
        "CrashLoopBackOff Restart Pod",     # CrashLoop remediation
    ]


# ========================================
# INTEGRATION TESTS
# ========================================

class TestWorkflowCatalogEndToEnd:
    """
    BR-STORAGE-013: End-to-End Workflow Search Integration Tests

    BEHAVIOR: Complete workflow search flow with real services
    CORRECTNESS: Results match expected format and business rules

    🔄 PRODUCTION: Tests with real Data Storage Service + PostgreSQL + pgvector
    """

    def test_semantic_search_with_exact_match_br_storage_013(
        self,
        workflow_catalog_tool,
        test_workflows
    ):
        """
        BR-STORAGE-013: Semantic search returns workflows with high similarity

        BEHAVIOR: Tool finds workflows matching query semantically
        CORRECTNESS: Results ranked by confidence (DD-WORKFLOW-002 v3.0)

        Test Flow:
        1. Search for "OOMKilled critical"
        2. Expect OOMKilled workflows returned
        3. Validate v3.0 response format
        4. Confirm confidence is valid
        """
        # Execute search
        result = workflow_catalog_tool.invoke(params={
            "query": "OOMKilled critical",
            "top_k": 5
        })

        # BEHAVIOR VALIDATION: Successful search
        assert result.status == StructuredToolResultStatus.SUCCESS, \
            f"BR-STORAGE-013: Search must succeed, got error: {result.error}"

        # CORRECTNESS VALIDATION: Result format
        data = json.loads(result.data)
        assert "workflows" in data, \
            "DD-WORKFLOW-002 v3.0: Response must contain 'workflows' array"

        workflows = data["workflows"]

        # Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip
        if len(workflows) == 0:
            pytest.fail(
                "REQUIRED: No test workflows available.\n"
                "  Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip\n"
                "  Run: ./scripts/bootstrap-workflows.sh"
            )

        # CORRECTNESS VALIDATION: DD-WORKFLOW-002 v3.0 workflow structure (FLAT)
        first_workflow = workflows[0]
        required_fields = [
            "workflow_id",      # UUID
            "title",            # Display name
            "description",      # Description text
            "signal_type",      # Singular string (not array)
            "confidence",       # 0.0-1.0 similarity score
        ]

        for field in required_fields:
            assert field in first_workflow, \
                f"DD-WORKFLOW-002 v3.0: Workflow must have '{field}' field"

        # CORRECTNESS VALIDATION: confidence score
        assert 0.0 <= first_workflow["confidence"] <= 1.0, \
            "DD-WORKFLOW-002 v3.0: confidence must be between 0.0 and 1.0"

        # CORRECTNESS VALIDATION: Signal type matching
        assert first_workflow["signal_type"] == "OOMKilled", \
            f"BR-STORAGE-013: Top result must match signal type, got {first_workflow['signal_type']}"

        print(f"✅ Found {len(workflows)} workflows, top confidence: {first_workflow['confidence']:.3f}")

    def test_hybrid_scoring_with_label_boost_dd_workflow_004(
        self,
        workflow_catalog_tool,
        test_workflows
    ):
        """
        DD-WORKFLOW-004: Confidence score reflects search relevance

        BEHAVIOR: Workflows with matching filters have positive confidence
        CORRECTNESS: confidence > 0 for matching workflows

        DD-WORKFLOW-002 v3.0: Hybrid scoring internals (base_similarity, label_boost)
        are not exposed in search response - only final confidence is returned.

        Test Flow:
        1. Search for "OOMKilled critical" with resource_management filter
        2. Expect workflows with positive confidence
        3. Confirm confidence is valid range
        """
        # Execute search with optional filter
        result = workflow_catalog_tool.invoke(params={
            "query": "OOMKilled critical",
            "filters": {
                "resource_management": "gitops"
            },
            "top_k": 5
        })

        assert result.status == StructuredToolResultStatus.SUCCESS

        data = json.loads(result.data)
        workflows = data["workflows"]

        # Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip
        if len(workflows) == 0:
            pytest.fail(
                "REQUIRED: No test workflows available.\n"
                "  Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip\n"
                "  Run: ./scripts/bootstrap-workflows.sh"
            )

        # DD-WORKFLOW-002 v3.0: confidence is the only scoring field exposed
        # (base_similarity, label_boost are internal Data Storage details)
        for wf in workflows:
            assert "confidence" in wf, \
                "DD-WORKFLOW-002 v3.0: Workflow must have confidence field"
            assert 0.0 <= wf["confidence"] <= 1.0, \
                f"DD-WORKFLOW-002 v3.0: confidence must be 0.0-1.0, got {wf['confidence']}"

        # Validate that results are sorted by confidence (highest first)
        for i in range(len(workflows) - 1):
            assert workflows[i]["confidence"] >= workflows[i + 1]["confidence"], \
                "DD-WORKFLOW-002 v3.0: Results must be sorted by confidence descending"

        print(f"✅ Confidence scoring validated: {len(workflows)} workflows, " +
              f"top confidence: {workflows[0]['confidence']:.3f}")

    def test_empty_results_handling_br_hapi_250(
        self,
        workflow_catalog_tool
    ):
        """
        BR-HAPI-250: Tool handles queries with no matching workflows gracefully

        BEHAVIOR: Returns empty array (not error) for no matches
        CORRECTNESS: Response format valid with empty workflows array

        Test Flow:
        1. Search for non-existent signal type
        2. Expect SUCCESS status (not ERROR)
        3. Confirm empty workflows array returned
        """
        # Execute search with non-existent signal type
        result = workflow_catalog_tool.invoke(params={
            "query": "NonExistentSignalType12345 critical",
            "top_k": 5
        })

        # BEHAVIOR VALIDATION: Success (not error)
        assert result.status == StructuredToolResultStatus.SUCCESS, \
            "BR-HAPI-250: No matches is not an error condition"

        # CORRECTNESS VALIDATION: Empty array
        data = json.loads(result.data)
        assert data["workflows"] == [], \
            "BR-HAPI-250: Empty results must return empty array, not null"

        print(f"✅ Empty results handled gracefully")

    def test_filter_validation_dd_llm_001(
        self,
        workflow_catalog_tool,
        test_workflows
    ):
        """
        DD-LLM-001: Mandatory label filters applied correctly

        BEHAVIOR: Only workflows matching mandatory labels returned
        CORRECTNESS: All results have matching signal-type (singular string in v3.0)

        Test Flow:
        1. Search for "CrashLoopBackOff high"
        2. Expect CrashLoopBackOff workflows
        3. Validate signal_type is singular string
        """
        # Execute search for CrashLoopBackOff
        result = workflow_catalog_tool.invoke(params={
            "query": "CrashLoopBackOff high",
            "top_k": 5
        })

        assert result.status == StructuredToolResultStatus.SUCCESS

        data = json.loads(result.data)
        workflows = data["workflows"]

        # Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip
        if len(workflows) == 0:
            pytest.fail(
                "REQUIRED: No CrashLoopBackOff test workflows available.\n"
                "  Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip\n"
                "  Run: ./scripts/bootstrap-workflows.sh"
            )

        # DD-WORKFLOW-002 v3.0: signal_type is singular string (not array)
        for workflow in workflows:
            assert "signal_type" in workflow, \
                "DD-WORKFLOW-002 v3.0: Workflow must have signal_type field"
            assert isinstance(workflow["signal_type"], str), \
                f"DD-WORKFLOW-002 v3.0: signal_type must be string, got {type(workflow['signal_type'])}"
            assert workflow["signal_type"] == "CrashLoopBackOff", \
                f"DD-LLM-001: All results must match signal-type filter, got {workflow['signal_type']}"

        print(f"✅ Filter validation passed: {len(workflows)} CrashLoopBackOff workflows found")

    def test_top_k_limiting_br_hapi_250(
        self,
        workflow_catalog_tool
    ):
        """
        BR-HAPI-250: Tool respects top_k parameter

        BEHAVIOR: Result count <= top_k
        CORRECTNESS: Most relevant workflows returned first

        Test Flow:
        1. Search with top_k=1
        2. Expect at most 1 result
        3. Confirm result has positive confidence (DD-WORKFLOW-002 v3.0)
        """
        # Execute search with top_k=1
        result = workflow_catalog_tool.invoke(params={
            "query": "OOMKilled critical",
            "top_k": 1
        })

        assert result.status == StructuredToolResultStatus.SUCCESS

        data = json.loads(result.data)
        workflows = data["workflows"]

        # CORRECTNESS VALIDATION: Result count limit
        assert len(workflows) <= 1, \
            f"BR-HAPI-250: top_k=1 must return at most 1 result, got {len(workflows)}"

        if len(workflows) == 1:
            # DD-WORKFLOW-002 v3.0: confidence instead of similarity_score
            first_confidence = workflows[0]["confidence"]
            assert first_confidence >= 0, \
                "BR-HAPI-250: Returned workflow must have non-negative confidence"

        print(f"✅ Top-k limiting validated: {len(workflows)} result(s) returned")

    def test_error_handling_service_unavailable_br_storage_013(
        self,
        workflow_catalog_tool
    ):
        """
        BR-STORAGE-013: Tool handles Data Storage Service errors gracefully

        BEHAVIOR: Returns ERROR status with meaningful message
        CORRECTNESS: Error message indicates service unavailable

        Test Flow:
        1. Configure tool with invalid Data Storage URL
        2. Execute search
        3. Expect ERROR status (not exception)
        4. Validate error message is meaningful
        """
        # Create tool with invalid URL (non-resolvable hostname)
        toolset = WorkflowCatalogToolset()
        tool = toolset.tools[0]
        tool.data_storage_url = "http://invalid-host-that-does-not-exist:99999"

        # Execute search
        result = tool.invoke(params={
            "query": "OOMKilled critical",
            "top_k": 5
        })

        # BEHAVIOR VALIDATION: Error status (not exception)
        assert result.status == StructuredToolResultStatus.ERROR, \
            "BR-STORAGE-013: Must return ERROR status when service unavailable"

        # CORRECTNESS VALIDATION: Meaningful error message
        assert result.error is not None, \
            "BR-STORAGE-013: Error message must be provided"

        # Check for various error message patterns (different systems report differently)
        error_lower = result.error.lower()
        valid_error = (
            "data storage" in error_lower or
            "connect" in error_lower or
            "connection refused" in error_lower or
            "name resolution" in error_lower or
            "resolve" in error_lower or
            "failed to parse" in error_lower or
            "invalid" in error_lower
        )
        assert valid_error, \
            f"BR-STORAGE-013: Error message must indicate service issue, got: {result.error}"

        print(f"✅ Error handling validated: {result.error}")


# ========================================
# TEST UTILITIES
# ========================================

def verify_test_data_exists():
    """
    Verify test data is bootstrapped in Data Storage Service

    This function can be called manually to check if test data exists.
    """
    try:
        response = requests.post(
            f"{DATA_STORAGE_URL}/api/v1/workflows/search",
            json={
                "query": "OOMKilled critical",
                "filters": {
                    "signal-type": "OOMKilled",
                    "severity": "critical"
                },
                "top_k": 10,
                "min_score": 0.0
            },
            timeout=HTTP_REQUEST_TIMEOUT
        )

        if response.status_code == 200:
            data = response.json()
            workflow_count = len(data.get("workflows", []))
            print(f"✅ Test data verified: {workflow_count} workflows found")
            return True
        else:
            print(f"❌ Test data verification failed: HTTP {response.status_code}")
            return False
    except Exception as e:
        print(f"❌ Test data verification failed: {e}")
        return False


if __name__ == "__main__":
    # Manual test data verification
    print("🔍 Verifying test data...")
    verify_test_data_exists()

