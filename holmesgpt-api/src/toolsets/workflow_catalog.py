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
Workflow Catalog Toolset for HolmesGPT SDK

Business Requirement: BR-HAPI-250 - Workflow Catalog Search Tool
Design Decision: DD-WORKFLOW-002 - MCP Workflow Catalog Architecture

⚠️ MVP IMPLEMENTATION - MOCK DATA ONLY ⚠️

This implementation uses hardcoded MOCK_WORKFLOWS for MVP validation.
Simple keyword-based search validates LLM integration.

TODO: Replace with PostgreSQL + pgvector when Data Storage Service is ready
  - Replace MOCK_WORKFLOWS with database queries
  - Implement semantic search using pgvector
  - Add advanced filtering (business_category, risk_tolerance, environment)
  - Add workflow parameter validation
  - Add workflow execution tracking
  See: Data Storage Service implementation plan
"""

import logging
import json
from typing import Dict, Any, List
from holmes.core.tools import Tool, Toolset, StructuredToolResult, StructuredToolResultStatus, ToolParameter, ToolsetStatusEnum

logger = logging.getLogger(__name__)


# ========================================
# MOCK WORKFLOW DATABASE (MVP ONLY)
# ========================================
# TODO: Replace with PostgreSQL + pgvector semantic search
# See: Data Storage Service implementation plan
MOCK_WORKFLOWS = [
    {
        "workflow_id": "oomkill-increase-memory",
        "version": "1.0.0",
        "title": "OOMKill Remediation - Increase Memory Limits",
        "description": "Increases memory limits for pods experiencing OOMKilled.",
        "signal_types": ["OOMKilled"],
        "estimated_duration": "10 minutes",
        "success_rate": 0.85,
        "similarity_score": 0.92,
    },
    {
        "workflow_id": "oomkill-scale-down",
        "version": "1.0.0",
        "title": "OOMKill Remediation - Scale Down Replicas",
        "description": "Reduces replica count for deployments experiencing OOMKilled.",
        "signal_types": ["OOMKilled"],
        "estimated_duration": "5 minutes",
        "success_rate": 0.80,
        "similarity_score": 0.85,
    },
    {
        "workflow_id": "crashloopbackoff-fix-config",
        "version": "1.0.0",
        "title": "CrashLoopBackOff - Fix Configuration",
        "description": "Identifies and fixes configuration issues causing CrashLoopBackOff.",
        "signal_types": ["CrashLoopBackOff"],
        "estimated_duration": "15 minutes",
        "success_rate": 0.75,
        "similarity_score": 0.88,
    },
]


# ========================================
# WORKFLOW CATALOG TOOL
# ========================================

class SearchWorkflowCatalogTool(Tool):
    """
    Tool for searching the workflow catalog based on incident characteristics.

    Business Requirement: BR-HAPI-250
    Design Decision: DD-WORKFLOW-002

    Input (per DD-WORKFLOW-002):
    - query: Natural language query describing the incident
    - filters: Optional filters (signal_types, business_category, risk_tolerance, environment)
    - top_k: Number of top results to return (default: 3)

    Output (per DD-WORKFLOW-002):
    - workflows: List of ranked workflows with metadata

    ⚠️ MVP: Uses MOCK_WORKFLOWS with simple keyword matching
    TODO: Implement PostgreSQL + pgvector semantic search
    """

    def __init__(self):
        super().__init__(
            name="search_workflow_catalog",
            description=(
                "Search for remediation workflows based on incident characteristics and business context. "
                "Returns ranked workflows with similarity scores, success rates, estimated durations, and descriptions. "
                "Use this tool AFTER completing your investigation to find appropriate remediation workflows."
            ),
            parameters={
                "query": ToolParameter(
                    type="string",
                    description=(
                        "Natural language query describing the incident and desired remediation "
                        "(e.g., 'OOMKilled critical pod recovery', 'CrashLoopBackOff due to configuration error')"
                    ),
                    required=True
                ),
                "filters": ToolParameter(
                    type="object",
                    description="Optional filters for workflow search",
                    required=False
                ),
                "top_k": ToolParameter(
                    type="integer",
                    description="Number of top results to return (default: 3, max: 10)",
                    required=False
                ),
            },
            additional_instructions=(
                "The similarity_score indicates how well the workflow matches your query. "
                "The success_rate indicates historical effectiveness. "
                "Consider both metrics when selecting a workflow."
            )
        )

    def _invoke(self, params: Dict, user_approved: bool = False) -> StructuredToolResult:
        """
        Execute workflow catalog search

        Business Requirement: BR-HAPI-250
        Design Decision: DD-WORKFLOW-002

        Args:
            params: Tool parameters (query, filters, top_k)
            user_approved: Whether user approved this tool call

        Returns:
            StructuredToolResult with workflows array or error

        ⚠️ MVP Implementation:
            - Mock workflow database (MOCK_WORKFLOWS)
            - Simple keyword-based search
            - Basic signal_types filtering

        TODO: Production Migration (when Data Storage Service is ready)
            - Replace with PostgreSQL queries
            - Add pgvector semantic search
            - Add advanced filtering (business_category, risk_tolerance, environment)
            - Add workflow parameter validation
            - Add workflow execution tracking
            See: Data Storage Service implementation plan
        """
        try:
            # Extract and validate parameters
            query = params.get("query", "")
            filters = params.get("filters", {})
            top_k = params.get("top_k", 3)

            # Validate top_k (BR-HAPI-250: max 10 results)
            if top_k > 10:
                logger.warning(f"top_k={top_k} exceeds maximum (10), capping to 10")
                top_k = 10

            logger.info(
                f"🔍 BR-HAPI-250: Workflow catalog search - "
                f"query='{query}', filters={filters}, top_k={top_k}"
            )

            # Search workflows (mock implementation for MVP)
            workflows = self._search_workflows(query, filters, top_k)

            logger.info(
                f"📤 BR-HAPI-250: Workflow catalog search completed - "
                f"{len(workflows)} workflows found"
            )

            # Format results as JSON (DD-WORKFLOW-002 compliant)
            result_data = json.dumps({"workflows": workflows}, indent=2)

            return StructuredToolResult(
                status=StructuredToolResultStatus.SUCCESS,
                data=result_data,
                params=params
            )
        except Exception as e:
            logger.error(f"❌ BR-HAPI-250: Workflow catalog search failed - {e}")
            return StructuredToolResult(
                status=StructuredToolResultStatus.ERROR,
                error=str(e),
                params=params
            )

    def _search_workflows(self, query: str, filters: Dict, top_k: int) -> List[Dict[str, Any]]:
        """
        Mock workflow search for MVP

        Business Requirement: BR-HAPI-250

        Args:
            query: Natural language query string
            filters: Optional filter criteria (signal_types, etc.)
            top_k: Maximum number of results to return

        Returns:
            List of workflow dictionaries sorted by similarity_score

        ⚠️ MVP: Simple keyword matching with MOCK_WORKFLOWS
        TODO: Replace with PostgreSQL + pgvector semantic search
          - Embed query using text-embedding-ada-002 or similar
          - Perform vector similarity search in pgvector
          - Apply filters using SQL WHERE clauses
          - Return top_k results ordered by similarity
          See: Data Storage Service implementation plan
        """
        results = []

        for workflow in MOCK_WORKFLOWS:
            # Apply signal_types filter if provided
            if filters.get("signal_types"):
                if not any(st in workflow["signal_types"] for st in filters["signal_types"]):
                    continue

            # Simple keyword matching for MVP (query is optional for filtered searches)
            if query:
                query_lower = query.lower()
                # Match keywords in query against workflow signal_types, title, and description
                workflow_text = (
                    " ".join(workflow["signal_types"]) + " " +
                    workflow["title"] + " " +
                    workflow["description"]
                ).lower()

                # Check if any query keywords appear in workflow text
                query_keywords = query_lower.split()
                if not any(keyword in workflow_text for keyword in query_keywords):
                    continue

            # Add workflow to results
            results.append(workflow)

        # Sort by similarity_score (descending) and limit to top_k
        results.sort(key=lambda w: w["similarity_score"], reverse=True)
        return results[:top_k]

    def get_parameterized_one_liner(self, params: Dict) -> str:
        """
        Return human-readable description of tool call

        Business Requirement: BR-HAPI-250

        Used for logging and user-facing tool call descriptions.

        Args:
            params: Tool parameters (query, filters, top_k)

        Returns:
            Human-readable string describing the tool call
        """
        query = params.get("query", "")
        filters = params.get("filters", {})
        top_k = params.get("top_k", 3)

        filter_str = ""
        if filters:
            filter_parts = []
            if filters.get("signal_types"):
                filter_parts.append(f"signal_types={filters['signal_types']}")
            if filters.get("business_category"):
                filter_parts.append(f"business_category={filters['business_category']}")
            if filters.get("risk_tolerance"):
                filter_parts.append(f"risk_tolerance={filters['risk_tolerance']}")
            if filters.get("environment"):
                filter_parts.append(f"environment={filters['environment']}")
            if filter_parts:
                filter_str = f" (filters: {', '.join(filter_parts)})"

        return f"Search workflow catalog: '{query}'{filter_str} (top {top_k})"


# ========================================
# WORKFLOW CATALOG TOOLSET
# ========================================

class WorkflowCatalogToolset(Toolset):
    """
    Toolset for workflow catalog operations.

    Business Requirement: BR-HAPI-250 - Workflow Catalog Search
    Design Decision: DD-WORKFLOW-002 - MCP Workflow Catalog Architecture

    Architecture:
    - v1.x: Embedded toolset in holmesgpt-api (not external MCP server)
    - v2.0: May extract to standalone service (to be evaluated)

    MVP Implementation:
    - Mock workflow database (hardcoded data)
    - Simple keyword-based search
    - No semantic search (pgvector)

    Production Migration Path:
    - Replace MOCK_WORKFLOWS with PostgreSQL queries
    - Add pgvector semantic search
    - Add workflow parameter validation
    - Add workflow execution tracking

    ⚠️ MVP: Mock data implementation for LLM prompt validation
    TODO: Integrate with Data Storage Service when ready
    """

    def __init__(self, enabled: bool = True):
        super().__init__(
            name="workflow/catalog",
            description="Search and retrieve remediation workflows based on incident characteristics",
            enabled=enabled,
            status=ToolsetStatusEnum.ENABLED,  # CRITICAL: Must be ENABLED for SDK to include in function calling
            tools=[SearchWorkflowCatalogTool()],
            docs_url=(
                "https://github.com/jordigilh/kubernaut/blob/main/docs/architecture/decisions/"
                "DD-WORKFLOW-002-MCP-WORKFLOW-CATALOG-ARCHITECTURE.md"
            ),
            llm_instructions=(
                "Use this toolset to find appropriate remediation workflows for incidents. "
                "IMPORTANT: Only search for workflows AFTER you have completed your investigation "
                "and identified the root cause. Provide a natural language query describing the "
                "incident and any relevant filters. The tool returns ranked workflows with "
                "similarity scores and success rates to help you select the most appropriate workflow."
            ),
            experimental=False,
            is_default=True,  # Enable by default for all investigations
        )

    def get_example_config(self) -> Dict[str, Any]:
        """
        Return example configuration for this toolset

        Required by HolmesGPT SDK Toolset base class.

        Returns:
            Dict with example configuration showing how to enable this toolset
        """
        return {
            "workflow/catalog": {
                "enabled": True,
                "description": "Search remediation workflows based on incident characteristics"
            }
        }
