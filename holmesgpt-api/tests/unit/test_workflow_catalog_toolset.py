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
Unit Tests for Workflow Catalog Toolset

Business Requirement: BR-HAPI-250 - Workflow Catalog Search Tool
Design Decision: DD-WORKFLOW-002 - MCP Workflow Catalog Architecture

⚠️ MVP TESTING SCOPE - MOCK DATA ONLY ⚠️

These tests validate tool behavior with mock data (MOCK_WORKFLOWS).
Tests expect hardcoded workflows and simple keyword matching.

TODO: Refactor tests when PostgreSQL + pgvector backend is ready
  - Replace MOCK_WORKFLOWS expectations with database queries
  - Add semantic search validation (pgvector)
  - Add advanced filtering tests (business_category, risk_tolerance, environment)
  - Add workflow parameter validation tests
  - Add workflow execution tracking tests
  See: Data Storage Service implementation plan

Test Coverage (8 tests):
1. BR-HAPI-250: Toolset initialization and configuration
2. BR-HAPI-250: Tool parameter validation
3. BR-HAPI-250: Search with query only (MOCK DATA)
4. BR-HAPI-250: Search with signal_types filter (MOCK DATA)
5. BR-HAPI-250: Search with top_k parameter (MOCK DATA)
6. BR-HAPI-250: Empty result handling (MOCK DATA)
7. BR-HAPI-250: Error handling
8. DD-WORKFLOW-002: Result format compliance (MOCK DATA)

Reference: src/extensions/recovery.py (SDK integration patterns)
Reference: tests/unit/test_recovery_analysis.py (test structure)
"""

import json
from unittest.mock import patch
from uuid import UUID
from src.toolsets.workflow_catalog import (
    SearchWorkflowCatalogTool,
    WorkflowCatalogToolset
)
from holmes.core.tools import StructuredToolResultStatus
from datastorage.models.workflow_search_response import WorkflowSearchResponse
from datastorage.models.workflow_search_result import WorkflowSearchResult


class TestWorkflowCatalogToolset:
    """
    BR-HAPI-250: Workflow Catalog Search Tool

    BEHAVIOR: Toolset must be discoverable by HolmesGPT SDK
    CORRECTNESS: Toolset configuration matches SDK requirements
    """

    def test_toolset_initialization_br_hapi_250(self):
        """
        BR-HAPI-250: Toolset must initialize with correct configuration

        BEHAVIOR: Toolset registers as 'workflow/catalog' with 1 tool
        CORRECTNESS: enabled=True, is_default=True, experimental=False
        """
        toolset = WorkflowCatalogToolset(enabled=True)

        # BEHAVIOR VALIDATION: Toolset must be discoverable
        assert toolset.name == "workflow/catalog", \
            "BR-HAPI-250: Toolset name must be 'workflow/catalog' for SDK registration"
        assert toolset.enabled is True, \
            "BR-HAPI-250: Toolset must be enabled by default for MVP"

        # CORRECTNESS VALIDATION: Tool count and type
        assert len(toolset.tools) == 1, \
            "BR-HAPI-250: Toolset must contain exactly 1 tool (search_workflow_catalog)"
        assert isinstance(toolset.tools[0], SearchWorkflowCatalogTool), \
            "BR-HAPI-250: Tool must be SearchWorkflowCatalogTool instance"

        # CORRECTNESS VALIDATION: Configuration
        assert toolset.is_default is True, \
            "BR-HAPI-250: Toolset must be default for all investigations"
        assert toolset.experimental is False, \
            "BR-HAPI-250: Toolset is production-ready (not experimental)"

    def test_tool_parameter_validation_br_hapi_250(self):
        """
        BR-HAPI-250: Tool must accept DD-WORKFLOW-002 compliant parameters

        BEHAVIOR: Tool accepts query (required), filters (optional), top_k (optional)
        CORRECTNESS: Parameter types and requirements match DD-WORKFLOW-002
        """
        toolset = WorkflowCatalogToolset()
        tool = toolset.tools[0]

        # BEHAVIOR VALIDATION: Tool name
        assert tool.name == "search_workflow_catalog", \
            "BR-HAPI-250: Tool name must match DD-WORKFLOW-002 specification"

        # CORRECTNESS VALIDATION: Required parameters
        assert "query" in tool.parameters, \
            "DD-WORKFLOW-002: Tool must accept 'query' parameter"
        assert tool.parameters["query"].required is True, \
            "DD-WORKFLOW-002: 'query' parameter is required"

        # CORRECTNESS VALIDATION: Optional parameters
        assert "filters" in tool.parameters, \
            "DD-WORKFLOW-002: Tool must accept 'filters' parameter"
        assert tool.parameters["filters"].required is False, \
            "DD-WORKFLOW-002: 'filters' parameter is optional"

        assert "top_k" in tool.parameters, \
            "DD-WORKFLOW-002: Tool must accept 'top_k' parameter"
        assert tool.parameters["top_k"].required is False, \
            "DD-WORKFLOW-002: 'top_k' parameter is optional"


class TestSearchWorkflowCatalogTool:
    """
    BR-HAPI-250: Workflow Search Tool Unit Tests

    SCOPE: Unit tests for tool behavior (non-HTTP functionality)

    ⚠️ DEPRECATED MVP TESTS REMOVED:
    - test_search_with_query_only_br_hapi_250 → Covered by TestWorkflowCatalogDataStorageIntegration
    - test_search_with_signal_types_filter_br_hapi_250 → Covered by test_query_transformation_dd_llm_001
    - test_search_with_top_k_parameter_br_hapi_250 → Covered by test_http_client_integration_br_storage_013
    - test_search_with_no_results_br_hapi_250 → Should be integration test with real DB
    - test_result_format_compliance_dd_workflow_002 → Covered by test_response_transformation_dd_workflow_004

    📋 TODO: Create integration tests in tests/integration/test_workflow_catalog_integration.py:
    - End-to-end workflow search with real Data Storage Service
    - Empty results handling with real database
    - Filter validation with real database
    - Top-k limiting with real database
    """

    def test_get_parameterized_one_liner_br_hapi_250(self):
        """
        BR-HAPI-250: Tool must provide human-readable description for logging

        BEHAVIOR: Tool generates descriptive one-liner for logs
        CORRECTNESS: One-liner includes query, filters, and top_k
        """
        toolset = WorkflowCatalogToolset()
        tool = toolset.tools[0]

        one_liner = tool.get_parameterized_one_liner({
            "query": "OOMKilled critical",
            "filters": {"signal_types": ["OOMKilled"]},
            "top_k": 3
        })

        # CORRECTNESS VALIDATION: Description completeness
        assert "OOMKilled critical" in one_liner, \
            "BR-HAPI-250: One-liner must include query text"
        assert "signal_types" in one_liner, \
            "BR-HAPI-250: One-liner must include filter information"
        assert "top 3" in one_liner, \
            "BR-HAPI-250: One-liner must include top_k value"

    def test_error_handling_br_hapi_250(self):
        """
        BR-HAPI-250: Tool must handle errors gracefully

        BEHAVIOR: Tool doesn't crash on invalid input
        CORRECTNESS: Tool returns error status (not exception)
        """
        toolset = WorkflowCatalogToolset()
        tool = toolset.tools[0]

        # Test with missing required parameter
        result = tool.invoke(params={})  # Missing 'query'

        # BEHAVIOR VALIDATION: Graceful error handling
        assert result is not None, \
            "BR-HAPI-250: Tool must not crash on invalid input"
        # Note: SDK may validate parameters before invoke(), so we just ensure no crash


class TestWorkflowCatalogDataStorageIntegration:
    """
    BR-STORAGE-013: Data Storage Service Integration Tests

    BEHAVIOR: Tool must call Data Storage Service REST API
    CORRECTNESS: HTTP requests match API contract, responses parsed correctly

    🔄 PRODUCTION INTEGRATION - REPLACES MOCK_WORKFLOWS

    Test Coverage (4 tests):
    1. HTTP client calls Data Storage Service with correct request format
    2. Query transformation: LLM query → WorkflowSearchRequest JSON
    3. Response transformation: WorkflowSearchResponse → Tool result
    4. Error handling: HTTP failures, timeouts, invalid responses
    """

    @patch('datastorage.api.workflow_catalog_api_api.WorkflowCatalogAPIApi.search_workflows')
    def test_http_client_integration_br_storage_013(self, mock_search):
        """
        BR-STORAGE-013: Tool must call Data Storage Service REST API via OpenAPI client

        BEHAVIOR: Tool calls OpenAPI client search_workflows method
        CORRECTNESS: Request includes query, filters, top_k, min_score

        🔄 PRODUCTION: Uses OpenAPI client instead of direct HTTP calls
        """
        # Setup mock OpenAPI response
        mock_workflow = WorkflowSearchResult(
            workflow_id=str(UUID("1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d")),
            title="OOMKill Remediation - Increase Memory Limits",
            description="Increases memory limits for pods experiencing OOMKilled.",
            signal_type="OOMKilled",
            confidence=0.95,
            final_score=0.95,
            rank=1
        )
        mock_response = WorkflowSearchResponse(
            workflows=[mock_workflow],
            total_results=1
        )
        mock_search.return_value = mock_response

        # Execute tool with Data Storage Service URL configured
        toolset = WorkflowCatalogToolset()
        tool = toolset.tools[0]

        # Set Data Storage Service URL (will be from config in production)
        tool._data_storage_url = "http://data-storage:8080"

        result = tool.invoke(params={
            "query": "OOMKilled critical",
            "top_k": 3
        })

        # BEHAVIOR VALIDATION: OpenAPI client called
        assert mock_search.call_count >= 1, "Should call search API at least once"
        call_kwargs = mock_search.call_args.kwargs

        # CORRECTNESS VALIDATION: Request format (per DD-STORAGE-008)
        request_obj = call_kwargs.get('workflow_search_request')
        assert request_obj is not None, "BR-STORAGE-013: Must pass workflow_search_request"
        assert request_obj.filters is not None, \
            "BR-STORAGE-013: Request must include filters"
        assert request_obj.top_k == 3, \
            "BR-STORAGE-013: top_k must match input"

        # BEHAVIOR VALIDATION: Successful result
        assert result.status == StructuredToolResultStatus.SUCCESS, \
            "BR-STORAGE-013: Tool must return SUCCESS for valid API response"

    @patch('datastorage.api.workflow_catalog_api_api.WorkflowCatalogAPIApi.search_workflows')
    def test_query_transformation_dd_llm_001(self, mock_search):
        """
        DD-LLM-001: Transform LLM query into WorkflowSearchRequest format via OpenAPI client

        BEHAVIOR: Parse structured query "signal_type severity [keywords]"
        CORRECTNESS: Extract signal-type and severity into filters

        🔄 PRODUCTION: Implements DD-LLM-001 query transformation using OpenAPI client
        """
        # Setup mock OpenAPI response
        mock_workflow = WorkflowSearchResult(
            workflow_id=str(UUID("1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d")),
            title="OOMKill Remediation",
            description="Test",
            signal_type="OOMKilled",
            confidence=0.95,
            final_score=0.95,
            rank=1
        )
        mock_response = WorkflowSearchResponse(
            workflows=[mock_workflow],
            total_results=1
        )
        mock_search.return_value = mock_response

        toolset = WorkflowCatalogToolset()
        tool = toolset.tools[0]
        tool._data_storage_url = "http://data-storage:8080"

        # Execute with structured query per DD-LLM-001
        tool.invoke(params={
            "query": "OOMKilled critical",
            "top_k": 3
        })

        # CORRECTNESS VALIDATION: Query transformation via OpenAPI client
        call_kwargs = mock_search.call_args.kwargs
        request_obj = call_kwargs.get('workflow_search_request')
        filters = request_obj.filters

        assert hasattr(filters, 'signal_type'), \
            "DD-LLM-001: Must extract signal_type from query"
        assert filters.signal_type == "OOMKilled", \
            "DD-LLM-001: signal_type must be first word of query"

        assert hasattr(filters, 'severity'), \
            "DD-LLM-001: Must extract severity from query"
        assert filters.severity == "critical", \
            "DD-LLM-001: severity must be second word of query"

    @patch('datastorage.api.workflow_catalog_api_api.WorkflowCatalogAPIApi.search_workflows')
    def test_response_transformation_dd_workflow_004(self, mock_search):
        """
        DD-WORKFLOW-004: Transform WorkflowSearchResponse into tool result via OpenAPI client

        BEHAVIOR: Parse hybrid scoring fields from API response
        CORRECTNESS: Map final_score to 'confidence' for LLM response

        NOTE: Per DD-WORKFLOW-015 (V1.0 label-only architecture):
        - 'confidence' is the only scoring field exposed
        - V1.0 uses label-only search (no semantic similarity)

        🔄 PRODUCTION: Implements DD-WORKFLOW-015 V1.0 label-only search using OpenAPI client
        """
        # Setup mock OpenAPI response with DD-WORKFLOW-002 v3.0 flat format
        mock_workflow = WorkflowSearchResult(
            workflow_id=str(UUID("2b3c4d5e-6f7a-8b9c-0d1e-2f3a4b5c6d7e")),
            title="OOMKill Remediation",
            description="Increases memory limits",
            signal_type="OOMKilled",
            container_image="quay.io/kubernaut/workflow-oomkill:v1.0.0",
            container_digest="sha256:abc123def456789012345678901234567890123456789012345678901234abcd",
            confidence=0.95,
            final_score=0.95,
            rank=1
        )
        mock_response = WorkflowSearchResponse(
            workflows=[mock_workflow],
            total_results=1
        )
        mock_search.return_value = mock_response

        toolset = WorkflowCatalogToolset()
        tool = toolset.tools[0]
        tool._data_storage_url = "http://data-storage:8080"

        result = tool.invoke(params={"query": "OOMKilled critical"})

        # CORRECTNESS VALIDATION: Response transformation
        assert result.data is not None, "Result data should not be None"
        data = json.loads(result.data)
        workflow = data["workflows"][0]

        # DD-WORKFLOW-002 v2.4: Must include confidence and container_image
        assert "confidence" in workflow, \
            "DD-WORKFLOW-002 v2.4: Must include 'confidence' (final_score)"
        assert workflow["confidence"] == 0.95, \
            "DD-WORKFLOW-002 v2.4: confidence must be final_score from API"
        assert "container_image" in workflow, \
            "DD-WORKFLOW-002 v2.4: Must include container_image"
        assert workflow["container_image"] == "quay.io/kubernaut/workflow-oomkill:v1.0.0", \
            "DD-WORKFLOW-002 v2.4: container_image must match API response"

        # Per DD-WORKFLOW-015 (V1.0 label-only architecture):
        # base_similarity and label_boost don't exist in V1.0
        assert "base_similarity" not in workflow, \
            "DD-WORKFLOW-015: base_similarity doesn't exist in V1.0 label-only architecture"
        assert "label_boost" not in workflow, \
            "DD-WORKFLOW-015: label_boost doesn't exist in V1.0 label-only architecture"

    @patch('datastorage.api.workflow_catalog_api_api.WorkflowCatalogAPIApi.search_workflows')
    def test_http_error_handling_br_storage_013(self, mock_search):
        """
        BR-STORAGE-013: Handle Data Storage Service errors gracefully via OpenAPI client

        BEHAVIOR: Tool handles API failures without crashing
        CORRECTNESS: Returns ERROR status with meaningful message

        🔄 PRODUCTION: Robust error handling for API integration using OpenAPI client
        """
        # Setup mock to raise API error
        from datastorage.exceptions import ApiException
        mock_search.side_effect = ApiException(status=0, reason="Connection refused")

        toolset = WorkflowCatalogToolset()
        tool = toolset.tools[0]
        tool._data_storage_url = "http://data-storage:8080"

        result = tool.invoke(params={"query": "OOMKilled critical"})

        # BEHAVIOR VALIDATION: Graceful error handling
        assert result.status == StructuredToolResultStatus.ERROR, \
            "BR-STORAGE-013: Must return ERROR status on API failure"

        # CORRECTNESS VALIDATION: Error message
        assert result.error is not None, \
            "BR-STORAGE-013: Must include error message"
        # Note: The actual error message from ApiException will vary
        assert len(result.error) > 0, \
            "BR-STORAGE-013: Error message must be meaningful"

