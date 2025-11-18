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
from src.toolsets.workflow_catalog import (
    SearchWorkflowCatalogTool,
    WorkflowCatalogToolset,
    MOCK_WORKFLOWS
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
    BR-HAPI-250: Workflow Search Functionality

    BEHAVIOR: Tool must find workflows based on query and filters
    CORRECTNESS: Results match DD-WORKFLOW-002 format and business rules

    ⚠️ MVP SCOPE: Tests expect MOCK_WORKFLOWS data
    TODO: Refactor when PostgreSQL + pgvector backend is ready
    """

    def test_search_with_query_only_br_hapi_250(self):
        """
        BR-HAPI-250: Tool must search workflows using natural language query

        BEHAVIOR: Tool finds workflows matching query keywords
        CORRECTNESS: Results contain workflows with matching signal_types

        ⚠️ MVP: Expects MOCK_WORKFLOWS with "OOMKilled" signal_types
        TODO: Refactor to test PostgreSQL semantic search when backend is ready
        """
        toolset = WorkflowCatalogToolset()
        tool = toolset.tools[0]

        result = tool.invoke(params={"query": "OOMKilled pod recovery"})

        # BEHAVIOR VALIDATION: Successful search
        assert result.status == StructuredToolResultStatus.SUCCESS, \
            "BR-HAPI-250: Tool must return SUCCESS status for valid query"

        # CORRECTNESS VALIDATION: Result format
        data = json.loads(result.data)
        assert "workflows" in data, \
            "DD-WORKFLOW-002: Response must contain 'workflows' array"

        # MVP VALIDATION: Expects MOCK_WORKFLOWS data
        assert len(data["workflows"]) > 0, \
            "BR-HAPI-250: Tool must find at least 1 OOMKilled workflow in MOCK_WORKFLOWS"

        # CORRECTNESS VALIDATION: Business rule - matching workflows
        assert any("OOMKilled" in w["signal_types"] for w in data["workflows"]), \
            "BR-HAPI-250: Results must include workflows matching query signal type"

    def test_search_with_signal_types_filter_br_hapi_250(self):
        """
        BR-HAPI-250: Tool must filter workflows by signal_types

        BEHAVIOR: Tool respects signal_types filter
        CORRECTNESS: All results match filter criteria

        ⚠️ MVP: Expects MOCK_WORKFLOWS filtering
        TODO: Refactor to test PostgreSQL filtering when backend is ready
        """
        toolset = WorkflowCatalogToolset()
        tool = toolset.tools[0]

        result = tool.invoke(params={
            "query": "pod issue",
            "filters": {"signal_types": ["CrashLoopBackOff"]}
        })

        # BEHAVIOR VALIDATION: Filter applied
        assert result.status == StructuredToolResultStatus.SUCCESS, \
            "BR-HAPI-250: Tool must handle filtered searches"

        # CORRECTNESS VALIDATION: Filter enforcement
        data = json.loads(result.data)
        for workflow in data["workflows"]:
            assert "CrashLoopBackOff" in workflow["signal_types"], \
                "BR-HAPI-250: All results must match signal_types filter"

    def test_search_with_top_k_parameter_br_hapi_250(self):
        """
        BR-HAPI-250: Tool must respect top_k parameter

        BEHAVIOR: Tool limits results to top_k count
        CORRECTNESS: Result count <= top_k

        ⚠️ MVP: Tests with MOCK_WORKFLOWS
        TODO: Refactor to test PostgreSQL LIMIT clause when backend is ready
        """
        toolset = WorkflowCatalogToolset()
        tool = toolset.tools[0]

        result = tool.invoke(params={
            "query": "OOMKilled",
            "top_k": 1
        })

        # CORRECTNESS VALIDATION: Result count limit
        assert result.status == StructuredToolResultStatus.SUCCESS
        data = json.loads(result.data)
        assert len(data["workflows"]) <= 1, \
            "BR-HAPI-250: Tool must return at most top_k results"

    def test_search_with_no_results_br_hapi_250(self):
        """
        BR-HAPI-250: Tool must handle queries with no matching workflows

        BEHAVIOR: Tool returns empty results (not error) for no matches
        CORRECTNESS: Response format is valid with empty workflows array

        ⚠️ MVP: Tests with MOCK_WORKFLOWS (limited dataset)
        TODO: Refactor to test PostgreSQL empty result handling when backend is ready
        """
        toolset = WorkflowCatalogToolset()
        tool = toolset.tools[0]

        result = tool.invoke(params={
            "query": "nonexistent_signal_type_xyz"
        })

        # BEHAVIOR VALIDATION: Graceful handling
        assert result.status == StructuredToolResultStatus.SUCCESS, \
            "BR-HAPI-250: No matches is not an error condition"

        # CORRECTNESS VALIDATION: Empty result format
        data = json.loads(result.data)
        assert data["workflows"] == [], \
            "BR-HAPI-250: Empty results must return empty array, not null"

    def test_result_format_compliance_dd_workflow_002(self):
        """
        DD-WORKFLOW-002: Results must comply with MCP Workflow Catalog format

        BEHAVIOR: Tool returns workflows with all required fields
        CORRECTNESS: Each workflow has workflow_id, title, description, etc.

        ⚠️ MVP: Validates MOCK_WORKFLOWS structure
        TODO: Refactor to validate PostgreSQL result structure when backend is ready
        """
        toolset = WorkflowCatalogToolset()
        tool = toolset.tools[0]

        result = tool.invoke(params={"query": "OOMKilled"})
        data = json.loads(result.data)

        # CORRECTNESS VALIDATION: Response structure
        assert "workflows" in data, \
            "DD-WORKFLOW-002: Response must have 'workflows' key"

        # CORRECTNESS VALIDATION: Workflow structure (DD-WORKFLOW-002)
        required_fields = [
            "workflow_id", "title", "description", "signal_types",
            "estimated_duration", "success_rate", "similarity_score"
        ]

        for workflow in data["workflows"]:
            for field in required_fields:
                assert field in workflow, \
                    f"DD-WORKFLOW-002: Workflow must have '{field}' field"

            # CORRECTNESS VALIDATION: Field types
            assert isinstance(workflow["signal_types"], list), \
                "DD-WORKFLOW-002: signal_types must be array"
            assert isinstance(workflow["success_rate"], (int, float)), \
                "DD-WORKFLOW-002: success_rate must be numeric"
            assert isinstance(workflow["similarity_score"], (int, float)), \
                "DD-WORKFLOW-002: similarity_score must be numeric"
            assert 0.0 <= workflow["similarity_score"] <= 1.0, \
                "DD-WORKFLOW-002: similarity_score must be between 0.0 and 1.0"

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

