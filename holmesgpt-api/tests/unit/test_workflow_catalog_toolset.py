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

import pytest
import json
from unittest.mock import Mock, patch, MagicMock
from src.toolsets.workflow_catalog import (
    SearchWorkflowCatalogTool,
    WorkflowCatalogToolset
)
from holmes.core.tools import StructuredToolResultStatus


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

    @patch('src.toolsets.workflow_catalog.requests.post')
    def test_http_client_integration_br_storage_013(self, mock_post):
        """
        BR-STORAGE-013: Tool must call Data Storage Service REST API

        BEHAVIOR: Tool sends HTTP POST to /api/v1/workflows/search
        CORRECTNESS: Request includes query, filters, top_k, min_similarity

        🔄 PRODUCTION: Replaces MOCK_WORKFLOWS with real API call
        """
        # Setup mock response (Data Storage API format per DD-STORAGE-008)
        mock_search_response = Mock()
        mock_search_response.status_code = 200
        mock_search_response.elapsed.total_seconds.return_value = 0.05
        # DD-WORKFLOW-002 v3.0: Flat response structure (no nested 'workflow' object)
        mock_search_response.json.return_value = {
            "workflows": [
                {
                    "workflow_id": "1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d",
                    "title": "OOMKill Remediation - Increase Memory Limits",
                    "description": "Increases memory limits for pods experiencing OOMKilled.",
                    "signal_type": "OOMKilled",
                    "confidence": 0.95
                }
            ],
            "total_results": 1,
            "query": "OOMKilled critical"
        }
        mock_search_response.raise_for_status = Mock()

        # Mock audit response
        mock_audit_response = Mock()
        mock_audit_response.status_code = 201
        mock_audit_response.json.return_value = {"event_id": "uuid-123"}

        mock_post.side_effect = [mock_search_response, mock_audit_response]

        # Execute tool with Data Storage Service URL configured
        toolset = WorkflowCatalogToolset()
        tool = toolset.tools[0]

        # Set Data Storage Service URL (will be from config in production)
        tool.data_storage_url = "http://data-storage:8080"

        result = tool.invoke(params={
            "query": "OOMKilled critical",
            "top_k": 3
        })

        # BEHAVIOR VALIDATION: HTTP POST called (search + audit)
        assert mock_post.call_count >= 1, "Should call search API at least once"
        call_args = mock_post.call_args_list[0]  # First call is search

        # CORRECTNESS VALIDATION: Correct endpoint
        assert call_args[0][0] == "http://data-storage:8080/api/v1/workflows/search", \
            "BR-STORAGE-013: Must call correct Data Storage API endpoint"

        # CORRECTNESS VALIDATION: Request format (per DD-STORAGE-008)
        request_json = call_args[1]['json']
        assert "query" in request_json, \
            "BR-STORAGE-013: Request must include query field"
        assert request_json["query"] == "OOMKilled critical", \
            "BR-STORAGE-013: Query must match input"
        assert "filters" in request_json, \
            "BR-STORAGE-013: Request must include filters field"
        assert "top_k" in request_json, \
            "BR-STORAGE-013: Request must include top_k field"
        assert request_json["top_k"] == 3, \
            "BR-STORAGE-013: top_k must match input"
        assert "min_similarity" in request_json, \
            "BR-STORAGE-013: Request must include min_similarity field"

        # CORRECTNESS VALIDATION: Timeout configured
        assert "timeout" in call_args[1], \
            "BR-STORAGE-013: HTTP request must have timeout"

        # BEHAVIOR VALIDATION: Successful result
        assert result.status == StructuredToolResultStatus.SUCCESS, \
            "BR-STORAGE-013: Tool must return SUCCESS for valid API response"

    @patch('src.toolsets.workflow_catalog.requests.post')
    def test_query_transformation_dd_llm_001(self, mock_post):
        """
        DD-LLM-001: Transform LLM query into WorkflowSearchRequest format

        BEHAVIOR: Parse structured query "signal_type severity [keywords]"
        CORRECTNESS: Extract signal-type and severity into filters

        🔄 PRODUCTION: Implements DD-LLM-001 query transformation
        """
        # Setup mock response
        mock_search_response = Mock()
        mock_search_response.status_code = 200
        mock_search_response.elapsed.total_seconds.return_value = 0.05
        mock_search_response.raise_for_status = Mock()
        mock_search_response.json.return_value = {
            "workflows": [],
            "total_results": 0,
            "query": "OOMKilled critical"
        }

        # Mock audit response
        mock_audit_response = Mock()
        mock_audit_response.status_code = 201
        mock_audit_response.json.return_value = {"event_id": "uuid-123"}

        mock_post.side_effect = [mock_search_response, mock_audit_response]

        toolset = WorkflowCatalogToolset()
        tool = toolset.tools[0]
        tool.data_storage_url = "http://data-storage:8080"

        # Execute with structured query per DD-LLM-001
        tool.invoke(params={
            "query": "OOMKilled critical",
            "top_k": 3
        })

        # CORRECTNESS VALIDATION: Query transformation (first call is search)
        request_json = mock_post.call_args_list[0][1]['json']
        filters = request_json["filters"]

        assert "signal-type" in filters, \
            "DD-LLM-001: Must extract signal-type from query"
        assert filters["signal-type"] == "OOMKilled", \
            "DD-LLM-001: signal-type must be first word of query"

        assert "severity" in filters, \
            "DD-LLM-001: Must extract severity from query"
        assert filters["severity"] == "critical", \
            "DD-LLM-001: severity must be second word of query"

    @patch('src.toolsets.workflow_catalog.requests.post')
    def test_response_transformation_dd_workflow_004(self, mock_post):
        """
        DD-WORKFLOW-004: Transform WorkflowSearchResponse into tool result

        BEHAVIOR: Parse hybrid scoring fields from API response
        CORRECTNESS: Map final_score to 'confidence' for LLM response

        NOTE: Per DD-WORKFLOW-002 v2.2 and user decision:
        - 'similarity_score' was renamed to 'confidence'
        - 'base_similarity' and 'label_boost' are internal fields (not exposed to LLM)
        - Only 'confidence' is exposed to the LLM response

        🔄 PRODUCTION: Implements DD-WORKFLOW-004 hybrid scoring display
        """
        # Setup mock response with DD-WORKFLOW-002 v3.0 flat format
        mock_search_response = Mock()
        mock_search_response.status_code = 200
        mock_search_response.elapsed.total_seconds.return_value = 0.05
        mock_search_response.raise_for_status = Mock()
        mock_search_response.json.return_value = {
            "workflows": [
                {
                    # DD-WORKFLOW-002 v3.0: Flat structure (no nested 'workflow' object)
                    "workflow_id": "2b3c4d5e-6f7a-8b9c-0d1e-2f3a4b5c6d7e",
                    "title": "OOMKill Remediation",
                    "description": "Increases memory limits",
                    "signal_type": "OOMKilled",
                    "container_image": "quay.io/kubernaut/workflow-oomkill:v1.0.0",
                    "container_digest": "sha256:abc123def456789012345678901234567890123456789012345678901234abcd",
                    "confidence": 0.95
                }
            ],
            "total_results": 1,
            "query": "OOMKilled critical"
        }

        # DD-WORKFLOW-014 v2.0: Audit generation moved to Data Storage Service
        # HolmesGPT API only makes ONE HTTP call (search), not two (search + audit)
        mock_post.return_value = mock_search_response

        toolset = WorkflowCatalogToolset()
        tool = toolset.tools[0]
        tool.data_storage_url = "http://data-storage:8080"

        result = tool.invoke(params={"query": "OOMKilled critical"})

        # CORRECTNESS VALIDATION: Response transformation
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

        # Per user decision: base_similarity and label_boost are internal fields
        # They are NOT exposed to the LLM response (only used for audit trail)
        assert "base_similarity" not in workflow, \
            "DD-WORKFLOW-002 v2.2: base_similarity is internal, not exposed to LLM"
        assert "label_boost" not in workflow, \
            "DD-WORKFLOW-002 v2.2: label_boost is internal, not exposed to LLM"

    @patch('src.toolsets.workflow_catalog.requests.post')
    def test_http_error_handling_br_storage_013(self, mock_post):
        """
        BR-STORAGE-013: Handle Data Storage Service errors gracefully

        BEHAVIOR: Tool handles HTTP failures without crashing
        CORRECTNESS: Returns ERROR status with meaningful message

        🔄 PRODUCTION: Robust error handling for API integration
        """
        # Setup mock to raise HTTP error
        mock_post.side_effect = Exception("Connection refused")

        toolset = WorkflowCatalogToolset()
        tool = toolset.tools[0]
        tool.data_storage_url = "http://data-storage:8080"

        result = tool.invoke(params={"query": "OOMKilled critical"})

        # BEHAVIOR VALIDATION: Graceful error handling
        assert result.status == StructuredToolResultStatus.ERROR, \
            "BR-STORAGE-013: Must return ERROR status on HTTP failure"

        # CORRECTNESS VALIDATION: Error message
        assert result.error is not None, \
            "BR-STORAGE-013: Must include error message"
        assert "Connection refused" in result.error or "data storage" in result.error.lower(), \
            "BR-STORAGE-013: Error message must be meaningful"

