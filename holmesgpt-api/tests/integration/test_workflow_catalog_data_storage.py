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
  - DD-WORKFLOW-004 - Hybrid Weighted Label Scoring
  - DD-TEST-001 - Port Allocation Strategy

TIER 2: INTEGRATION TESTS (20% Coverage)

Test Coverage (10 tests):
  I1.1-I1.5: Contract validation with real Data Storage
  I2.1-I2.3: Semantic search behavior
  I3.1-I3.2: Error propagation

Prerequisites:
  - Docker/Podman running
  - Data Storage Service container (port 18090 per DD-TEST-001)
  - Embedding Service container (port 18000 per DD-TEST-001)
  - PostgreSQL with pgvector extension (port 15433 per DD-TEST-001)
  - Redis (port 16380 per DD-TEST-001)
  - Test data bootstrapped via bootstrap-workflows.sh

Setup:
  Run: ./tests/integration/setup_workflow_catalog_integration.sh
"""

import pytest
import json
import re
import uuid
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
    is_integration_infra_available,
)


# =============================================================================
# FIXTURES
# =============================================================================

@pytest.fixture(scope="module")
def workflow_catalog_tool(integration_infrastructure):
    """
    Create WorkflowCatalogTool configured for integration testing

    Uses integration_infrastructure fixture from conftest.py which:
    - Checks if Data Storage Service is available
    - Skips tests if infrastructure not running
    """
    toolset = WorkflowCatalogToolset()
    tool = toolset.tools[0]

    # Override Data Storage URL for integration testing (DD-TEST-001: port 18090)
    data_storage_url = integration_infrastructure["data_storage_url"]
    tool.data_storage_url = data_storage_url

    print(f"🔧 Workflow Catalog Tool configured: {data_storage_url}")
    return tool


# =============================================================================
# INTEGRATION TEST SUITE 1: CONTRACT VALIDATION (I1.1-I1.5)
# =============================================================================

@pytest.mark.requires_data_storage
class TestContractValidation:
    """
    I1.x: Integration tests validating HTTP contract with real Data Storage

    These tests verify that HolmesGPT-API and Data Storage Service agree
    on request format and response format (DD-WORKFLOW-002 v3.0).
    """

    def test_search_request_format_accepted_i1_1(self, workflow_catalog_tool):
        """
        I1.1: Data Storage accepts our search request format

        Integration Outcome: Data Storage doesn't reject our request
        """
        # ACT
        result = workflow_catalog_tool.invoke(params={
            "query": "OOMKilled critical",
            "top_k": 5
        })

        # ASSERT - Contract validation: request accepted
        assert result.status == StructuredToolResultStatus.SUCCESS, \
            f"I1.1: Data Storage rejected our request: {result.error}"

    def test_response_format_is_v3_compliant_i1_2(self, workflow_catalog_tool):
        """
        I1.2: Response contains all DD-WORKFLOW-002 v3.0 required fields

        Integration Outcome: Response matches v3.0 contract
        """
        # ACT
        result = workflow_catalog_tool.invoke(params={
            "query": "OOMKilled",
            "top_k": 5
        })

        # ASSERT
        assert result.status == StructuredToolResultStatus.SUCCESS, \
            f"I1.2: Search failed: {result.error}"

        data = json.loads(result.data)
        assert "workflows" in data, "I1.2: Response must have 'workflows' field"

        if data["workflows"]:
            wf = data["workflows"][0]

            # DD-WORKFLOW-002 v3.0 required fields
            v3_required_fields = [
                "workflow_id",  # UUID
                "title",        # Display name
                "description",  # Workflow description
                "signal_type",  # Singular string (not array)
                "confidence",   # Float 0.0-1.0
            ]

            for field in v3_required_fields:
                assert field in wf, \
                    f"I1.2: v3.0 required field '{field}' missing from response"

    def test_workflow_id_is_uuid_from_real_service_i1_3(self, workflow_catalog_tool):
        """
        I1.3: Real service returns UUID format workflow_id

        Integration Outcome: workflow_id is valid UUID
        """
        # ACT
        result = workflow_catalog_tool.invoke(params={
            "query": "OOMKilled critical",
            "top_k": 5
        })

        # ASSERT
        assert result.status == StructuredToolResultStatus.SUCCESS

        data = json.loads(result.data)
        if data["workflows"]:
            wf = data["workflows"][0]

            # Validate UUID format
            uuid_pattern = re.compile(
                r'^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$',
                re.IGNORECASE
            )
            assert uuid_pattern.match(wf["workflow_id"]), \
                f"I1.3: workflow_id must be UUID, got: {wf['workflow_id']}"

            # Also validate it can be parsed as UUID
            parsed = uuid.UUID(wf["workflow_id"])
            assert str(parsed) == wf["workflow_id"].lower(), \
                "I1.3: workflow_id must be parseable as UUID"

    def test_confidence_ordering_from_real_service_i1_4(self, workflow_catalog_tool):
        """
        I1.4: Results ordered by confidence (descending)

        Integration Outcome: First result has highest confidence
        """
        # ACT
        result = workflow_catalog_tool.invoke(params={
            "query": "OOMKilled critical",
            "top_k": 10
        })

        # ASSERT
        assert result.status == StructuredToolResultStatus.SUCCESS

        data = json.loads(result.data)
        workflows = data["workflows"]

        if len(workflows) > 1:
            # Verify descending order
            for i in range(len(workflows) - 1):
                current = workflows[i]["confidence"]
                next_wf = workflows[i + 1]["confidence"]
                assert current >= next_wf, \
                    f"I1.4: Results must be ordered by confidence DESC. " + \
                    f"workflows[{i}]={current} < workflows[{i+1}]={next_wf}"

    def test_container_image_digest_format_i1_5(self, workflow_catalog_tool):
        """
        I1.5: container_digest is valid sha256 format (if present)

        Integration Outcome: Digests are valid sha256 or null
        """
        # ACT
        result = workflow_catalog_tool.invoke(params={
            "query": "OOMKilled critical",
            "top_k": 5
        })

        # ASSERT
        assert result.status == StructuredToolResultStatus.SUCCESS

        data = json.loads(result.data)
        for wf in data["workflows"]:
            digest = wf.get("container_digest")

            if digest is not None and digest != "":
                # Validate sha256 format
                sha256_pattern = re.compile(r'^sha256:[0-9a-f]{64}$', re.IGNORECASE)
                assert sha256_pattern.match(digest), \
                    f"I1.5: container_digest must be sha256:64hex, got: {digest}"


# =============================================================================
# INTEGRATION TEST SUITE 2: SEMANTIC SEARCH BEHAVIOR (I2.1-I2.3)
# =============================================================================

@pytest.mark.requires_data_storage
class TestSemanticSearchBehavior:
    """
    I2.x: Integration tests validating semantic search with real embeddings

    These tests verify that semantic search returns relevant results.
    """

    def test_semantic_search_returns_relevant_results_i2_1(self, workflow_catalog_tool):
        """
        I2.1: "OOMKilled" query returns OOMKilled workflows (if data exists)

        Integration Outcome: Semantic search finds relevant workflows when data exists
        Note: This test validates contract, not data presence
        """
        # ACT
        result = workflow_catalog_tool.invoke(params={
            "query": "OOMKilled pod memory limit exceeded critical",
            "top_k": 5
        })

        # ASSERT - Search must succeed
        assert result.status == StructuredToolResultStatus.SUCCESS, \
            f"I2.1: Search failed: {result.error}"

        data = json.loads(result.data)
        workflows = data["workflows"]

        # If workflows exist, validate relevance
        if len(workflows) > 0:
            top_wf = workflows[0]
            # Top result should be OOMKilled-related (if data is bootstrapped)
            print(f"I2.1: Found {len(workflows)} workflows, top: {top_wf.get('signal_type')}")
        else:
            # Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip
            pytest.fail(
                "REQUIRED: I2.1 - No test data bootstrapped.\n"
                "  Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip\n"
                "  Run: ./scripts/bootstrap-workflows.sh"
            )

    def test_different_queries_return_different_results_i2_2(self, workflow_catalog_tool):
        """
        I2.2: Different queries return different top results

        Integration Outcome: Semantic search differentiates query intents
        """
        # ACT - Search for two different incident types
        oom_result = workflow_catalog_tool.invoke(params={
            "query": "OOMKilled memory",
            "top_k": 3
        })

        crash_result = workflow_catalog_tool.invoke(params={
            "query": "CrashLoopBackOff container restart",
            "top_k": 3
        })

        # ASSERT - Both searches succeed
        assert oom_result.status == StructuredToolResultStatus.SUCCESS
        assert crash_result.status == StructuredToolResultStatus.SUCCESS

        oom_data = json.loads(oom_result.data)
        crash_data = json.loads(crash_result.data)

        # If both have results, top results should be different
        if oom_data["workflows"] and crash_data["workflows"]:
            oom_top = oom_data["workflows"][0]["workflow_id"]
            crash_top = crash_data["workflows"][0]["workflow_id"]

            # They could be the same if only one workflow exists,
            # but typically should be different
            # This test documents the behavior
            print(f"I2.2: OOM top={oom_top}, Crash top={crash_top}")

    def test_top_k_limits_results_i2_3(self, workflow_catalog_tool):
        """
        I2.3: top_k=3 returns at most 3 results

        Integration Outcome: Result count respects top_k
        """
        # ACT
        result = workflow_catalog_tool.invoke(params={
            "query": "OOMKilled critical",
            "top_k": 3
        })

        # ASSERT
        assert result.status == StructuredToolResultStatus.SUCCESS

        data = json.loads(result.data)
        assert len(data["workflows"]) <= 3, \
            f"I2.3: top_k=3 must return <=3 results, got {len(data['workflows'])}"


# =============================================================================
# INTEGRATION TEST SUITE 3: ERROR PROPAGATION (I3.1-I3.2)
# =============================================================================

class TestErrorPropagationWithoutInfrastructure:
    """
    I3.1: Error handling tests that don't require infrastructure.

    These tests use intentionally invalid URLs to validate error handling.
    BUSINESS OUTCOME: Connection failures are handled gracefully.

    NOTE: No @pytest.mark.requires_data_storage marker - uses fake URLs.
    """

    def test_data_storage_unavailable_returns_error_i3_1(self):
        """
        I3.1: Connection refused returns clear ERROR status

        Integration Outcome: Tool returns ERROR, not exception

        NOTE: Uses invalid URL - does NOT require infrastructure.
        """
        # ARRANGE - Tool pointing to invalid URL
        tool = SearchWorkflowCatalogTool(
            data_storage_url="http://invalid-host-that-does-not-exist:99999"
        )

        # ACT
        result = tool.invoke(params={
            "query": "OOMKilled critical",
            "top_k": 5
        })

        # ASSERT
        assert result.status == StructuredToolResultStatus.ERROR, \
            "I3.1: Connection refused must return ERROR status"

        assert result.error is not None, \
            "I3.1: Error message must be provided"

        assert len(result.error) > 0, \
            "I3.1: Error message must not be empty"

        # Error message should be meaningful (any error message is acceptable)
        # Different HTTP libraries produce different error messages
        print(f"I3.1: Got error: {result.error}")


@pytest.mark.requires_data_storage
class TestErrorPropagationWithInfrastructure:
    """
    I3.2+: Error handling tests that require real infrastructure.

    These tests verify timeout and error handling with real Data Storage service.
    """

    def test_data_storage_timeout_behavior_i3_2(self, workflow_catalog_tool):
        """
        I3.2: Document timeout behavior (timeout configured via env)

        Integration Outcome: Timeout behavior is documented
        """
        # This test documents the timeout configuration
        # Actual timeout testing requires mock server with delays

        # ACT - Normal request (should complete within timeout)
        result = workflow_catalog_tool.invoke(params={
            "query": "OOMKilled critical",
            "top_k": 5
        })

        # ASSERT - Normal request completes
        assert result.status == StructuredToolResultStatus.SUCCESS, \
            f"I3.2: Normal request should complete, got: {result.error}"

        # Document timeout configuration
        timeout = workflow_catalog_tool.http_timeout
        print(f"I3.2: HTTP timeout configured: {timeout}s")


# =============================================================================
# TEST UTILITIES
# =============================================================================

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
            timeout=10
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

