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

Business Requirement: BR-HAPI-250 - Workflow Catalog Search Tool, BR-AI-075 - Workflow Selection Contract
Design Decisions:
  - DD-WORKFLOW-002 v2.4 - MCP Workflow Catalog Architecture (container_image in response)
  - DD-CONTRACT-001 v1.2 - AIAnalysis passes container_image to WorkflowExecution
  - DD-WORKFLOW-014 v2.1 - Audit trail (Data Storage generates)
  - DD-LLM-001 - MCP Workflow Search Parameter Taxonomy
  - DD-STORAGE-008 - Workflow Catalog Schema
  - DD-WORKFLOW-004 - Hybrid Weighted Label Scoring

Query Format (per DD-LLM-001):
  - Structured format: '<signal_type> <severity> [optional_keywords]'
  - Example: "OOMKilled critical", "CrashLoopBackOff high configuration"
  - Uses canonical Kubernetes event reasons from LLM's RCA findings

This toolset integrates with the Data Storage Service REST API for semantic workflow search:
  - Two-phase semantic search (exact label matching + pgvector similarity)
  - Hybrid weighted scoring (confidence = base_similarity + label_boost - label_penalty)
  - Real-time embedding generation via Data Storage Service
  - Confidence scores: 90-95% for exact label matches

Audit Trail (DD-WORKFLOW-014 v2.1):
  - HolmesGPT API passes remediation_id in JSON body (not HTTP header)
  - Data Storage Service generates audit events (has richer workflow context)
  - remediation_id is for CORRELATION ONLY (not used in search logic)

Configuration:
  - DATA_STORAGE_URL: Data Storage Service endpoint (default: http://data-storage:8080)
  - DATA_STORAGE_TIMEOUT: HTTP timeout in seconds (default: 10)
"""

import logging
import json
import os
import requests
from typing import Dict, Any, List, Optional
from holmes.core.tools import Tool, Toolset, StructuredToolResult, StructuredToolResultStatus, ToolParameter, ToolsetStatusEnum

logger = logging.getLogger(__name__)


# ========================================
# DATA STORAGE SERVICE INTEGRATION
# ========================================
# Business Requirement: BR-STORAGE-013 - Semantic Search for Remediation Workflows
# Design Decisions:
#   - DD-WORKFLOW-002 v2.0 - MCP Workflow Catalog Architecture
#   - DD-STORAGE-008 - Workflow Catalog Schema
#   - DD-WORKFLOW-004 - Hybrid Weighted Label Scoring
#
# Integration: This toolset calls Data Storage Service REST API
#   - Endpoint: POST /api/v1/workflows/search
#   - Two-phase semantic search (exact labels + pgvector similarity)
#   - Hybrid scoring (base_similarity + label_boost - label_penalty)
#   - Real-time embedding generation


# ========================================
# WORKFLOW CATALOG TOOL
# ========================================

class SearchWorkflowCatalogTool(Tool):
    """
    Tool for searching the workflow catalog based on incident characteristics.

    Business Requirement: BR-HAPI-250
    Design Decisions:
      - DD-WORKFLOW-002 v2.0 - MCP Workflow Catalog Architecture
      - DD-LLM-001 - MCP Workflow Search Parameter Taxonomy
      - DD-STORAGE-008 - Workflow Catalog Schema
      - DD-WORKFLOW-004 - Hybrid Weighted Label Scoring

    Query Format (per DD-LLM-001):
    - Structured format: '<signal_type> <severity> [optional_keywords]'
    - signal_type: Canonical Kubernetes event reason from LLM's RCA findings
    - severity: LLM's RCA severity assessment (critical/high/medium/low)
    - Example: "OOMKilled critical", "NodeNotReady critical infrastructure"

    Input (per DD-WORKFLOW-002 v2.0):
    - query: Structured query string (required)
    - label.*: Optional exact match filters (signal-type, severity, environment, etc.)
    - top_k: Number of top results to return (default: 3)

    Output (per DD-WORKFLOW-002 v2.0):
    - workflows: List of ranked workflows with confidence scores (90-95% for exact matches)

    🔄 PRODUCTION: Integrated with Data Storage Service REST API
    - Two-phase semantic search (exact labels + pgvector similarity)
    - Hybrid weighted scoring (base similarity + label boost - label penalty)
    - Real-time embedding generation
    """

    def __init__(self, data_storage_url: Optional[str] = None, remediation_id: Optional[str] = None):
        super().__init__(
            name="search_workflow_catalog",
            description=(
                "Search for remediation workflows based on incident characteristics and business context. "
                "Returns ranked workflows with confidence scores and descriptions. "
                "Use this tool AFTER completing your investigation to find appropriate remediation workflows."
            ),
            parameters={
                "query": ToolParameter(
                    type="string",
                    description=(
                        "Structured query in format '<signal_type> <severity> [optional_keywords]' per DD-LLM-001. "
                        "Use canonical Kubernetes event reason from your RCA findings (not initial signal). "
                        "Examples: 'OOMKilled critical', 'CrashLoopBackOff high', 'NodeNotReady critical infrastructure'"
                    ),
                    required=True
                ),
                "filters": ToolParameter(
                    type="object",
                    description="Optional filters for workflow search (environment, priority, etc.)",
                    required=False
                ),
                "top_k": ToolParameter(
                    type="integer",
                    description="Number of top results to return (default: 3, max: 10)",
                    required=False
                ),
            },
            additional_instructions=(
                "IMPORTANT: Use structured query format '<signal_type> <severity>' per DD-LLM-001. "
                "The signal_type must be a canonical Kubernetes event reason from your RCA findings. "
                "The 'confidence' score (0.0-1.0, typically 0.90-0.95 for exact matches) indicates how well "
                "the workflow matches your query. Higher confidence means better match. "
                "Select workflows with highest confidence scores."
            )
        )

        # Data Storage Service configuration
        # Default: Kubernetes service name (production)
        # Override: Environment variable or constructor parameter (testing)
        # Use object.__setattr__ to bypass Pydantic validation
        object.__setattr__(self, '_data_storage_url', data_storage_url or os.getenv(
            "DATA_STORAGE_URL",
            "http://data-storage:8080"
        ))
        object.__setattr__(self, '_http_timeout', int(os.getenv("DATA_STORAGE_TIMEOUT", "10")))

        # Store remediation_id for audit correlation (BR-AUDIT-005, DD-WORKFLOW-014)
        # This is the correlation_id that groups all audit events for a remediation
        # Value: remediation_id from AIAnalysis controller (e.g., "req-2025-10-06-abc123")
        object.__setattr__(self, '_remediation_id', remediation_id or "")

        logger.info(
            f"🔄 BR-STORAGE-013: Workflow catalog configured - "
            f"data_storage_url={self._data_storage_url}, timeout={self._http_timeout}s, "
            f"remediation_id={self._remediation_id or 'not-set'}"
        )

    @property
    def data_storage_url(self) -> str:
        """Get Data Storage Service URL"""
        return object.__getattribute__(self, '_data_storage_url')

    @data_storage_url.setter
    def data_storage_url(self, value: str):
        """Set Data Storage Service URL (for testing)"""
        object.__setattr__(self, '_data_storage_url', value)

    @property
    def http_timeout(self) -> int:
        """Get HTTP timeout"""
        return object.__getattribute__(self, '_http_timeout')

    def _invoke(self, params: Dict, user_approved: bool = False) -> StructuredToolResult:
        """
        Execute workflow catalog search

        Business Requirement: BR-HAPI-250
        Design Decisions:
          - DD-WORKFLOW-002 v2.3 - MCP Workflow Catalog Architecture
          - DD-WORKFLOW-014 v2.1 - Audit trail (Data Storage generates)
          - DD-LLM-001 - MCP Workflow Search Parameter Taxonomy

        Args:
            params: Tool parameters (query, filters, top_k)
            user_approved: Whether user approved this tool call

        Returns:
            StructuredToolResult with workflows array or error

        Implementation:
            - Calls Data Storage Service REST API
            - Two-phase semantic search (exact labels + pgvector similarity)
            - Hybrid weighted scoring (confidence = base + boost - penalty)
            - remediation_id passed in JSON body for audit correlation
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

            # Search workflows
            # DD-WORKFLOW-014 v2.0: remediation_id passed in JSON body
            # Data Storage Service generates audit events (not HolmesGPT API)
            workflows = self._search_workflows(query, filters, top_k)

            logger.info(
                f"📤 BR-HAPI-250: Workflow catalog search completed - "
                f"{len(workflows)} workflows found"
            )

            # NOTE: Audit event generation moved to Data Storage Service
            # per DD-WORKFLOW-014 v2.0. Data Storage has richer context
            # (all workflow metadata, success metrics, version history).
            # HolmesGPT API only passes remediation_id in JSON body.

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

    def _build_filters_from_query(self, query: str, additional_filters: Dict) -> Dict[str, Any]:
        """
        Transform LLM query into WorkflowSearchFilters format

        Business Requirement: BR-STORAGE-013
        Design Decision: DD-LLM-001 - MCP Workflow Search Parameter Taxonomy

        Args:
            query: Structured query '<signal_type> <severity> [keywords]' (per DD-LLM-001)
            additional_filters: Additional filters from LLM (optional labels)

        Returns:
            WorkflowSearchFilters dict with signal-type and severity (mandatory)

        Example:
            Input: "OOMKilled critical"
            Output: {"signal-type": "OOMKilled", "severity": "critical"}
        """
        # Parse structured query per DD-LLM-001
        # Extract signal_type and severity from query string
        parts = query.split()
        signal_type = parts[0] if len(parts) > 0 else ""

        # Known signal types (DD-WORKFLOW-002)
        known_signal_types = {
            "OOMKilled", "CrashLoopBackOff", "NodeNotReady",
            "ImagePullBackOff", "PodEvicted", "HighCPU", "HighMemory"
        }

        # Known severity levels (DD-LLM-001)
        known_severities = {"critical", "high", "medium", "low"}

        # Extract severity from query (look for known severity keywords)
        severity = "high"  # Default severity
        query_lower = query.lower()
        for sev in known_severities:
            if sev in query_lower:
                severity = sev
                break

        # Build filters (DD-STORAGE-008 format)
        filters = {
            "signal-type": signal_type,
            "severity": severity
        }

        # Merge additional filters (optional labels per DD-WORKFLOW-004)
        if additional_filters:
            # Map from LLM filter names to API field names
            filter_mapping = {
                "resource_management": "resource-management",
                "gitops_tool": "gitops-tool",
                "environment": "environment",
                "business_category": "business-category",
                "priority": "priority",
                "risk_tolerance": "risk-tolerance"
            }

            for llm_key, api_key in filter_mapping.items():
                if llm_key in additional_filters:
                    filters[api_key] = additional_filters[llm_key]

        return filters

    def _search_workflows(
        self, query: str, filters: Dict, top_k: int
    ) -> List[Dict[str, Any]]:
        """
        Search workflows using Data Storage Service REST API

        Business Requirement: BR-STORAGE-013
        Design Decisions:
          - DD-WORKFLOW-002 v2.3 - Two-phase semantic search
          - DD-WORKFLOW-014 v2.1 - remediation_id in JSON body
          - DD-LLM-001 - Structured query format
          - DD-STORAGE-008 - Workflow Catalog Schema
          - DD-WORKFLOW-004 - Hybrid Weighted Label Scoring

        Args:
            query: Structured query string '<signal_type> <severity> [keywords]' (per DD-LLM-001)
            filters: Optional filter criteria (additional labels)
            top_k: Maximum number of results to return

        Returns:
            List of transformed workflow dictionaries (for LLM)

        🔄 PRODUCTION: Calls Data Storage Service /api/v1/workflows/search
          Phase 1: Exact label matching (SQL WHERE clause)
          Phase 2: Semantic ranking (pgvector similarity)
          Hybrid scoring: base_similarity + label_boost - label_penalty
          Expected confidence: 90-95% for exact matches

        DD-WORKFLOW-014 v2.1: remediation_id passed in JSON body
          - Data Storage Service generates audit events
          - HolmesGPT API does NOT generate audit events
          - remediation_id is for CORRELATION ONLY (not used in search logic)
        """
        try:
            # Build request per DD-STORAGE-008
            search_filters = self._build_filters_from_query(query, filters)

            # DD-WORKFLOW-014 v2.1: Include remediation_id in JSON body
            # This enables Data Storage Service to generate audit events
            # with proper correlation to the remediation request
            request_data = {
                "query": query,
                "filters": search_filters,
                "top_k": top_k,
                "min_similarity": 0.3,  # 30% minimum similarity threshold (DD-WORKFLOW-002)
                "remediation_id": self._remediation_id  # For audit correlation
            }

            logger.info(
                f"🔍 BR-STORAGE-013: Calling Data Storage Service - "
                f"url={self._data_storage_url}/api/v1/workflows/search, "
                f"query='{query}', filters={search_filters}, top_k={top_k}, "
                f"remediation_id={self._remediation_id or 'not-set'}"
            )

            # Call Data Storage Service
            # DD-WORKFLOW-014 v2.1: remediation_id in JSON body for audit correlation
            response = requests.post(
                f"{self._data_storage_url}/api/v1/workflows/search",
                json=request_data,
                timeout=self._http_timeout
            )
            response.raise_for_status()

            # Parse response per DD-STORAGE-008
            search_response = response.json()
            api_workflows = search_response.get("workflows", [])

            logger.info(
                f"✅ BR-STORAGE-013: Data Storage Service responded - "
                f"total_results={search_response.get('total_results', 0)}, "
                f"returned={len(api_workflows)}, "
                f"duration_ms={int(response.elapsed.total_seconds() * 1000)}"
            )

            # Transform response to tool format
            return self._transform_api_response(api_workflows)

        except requests.exceptions.Timeout as e:
            logger.error(
                f"⏱️  BR-STORAGE-013: Data Storage Service timeout - {e}"
            )
            raise Exception(f"Data Storage Service timeout after {self._http_timeout}s")
        except requests.exceptions.ConnectionError as e:
            logger.error(
                f"🔌 BR-STORAGE-013: Data Storage Service connection failed - {e}"
            )
            raise Exception(f"Cannot connect to Data Storage Service at {self._data_storage_url}")
        except requests.exceptions.HTTPError as e:
            logger.error(
                f"❌ BR-STORAGE-013: Data Storage Service HTTP error - {e}"
            )
            raise Exception(f"Data Storage Service error: {e}")
        except Exception as e:
            logger.error(
                f"💥 BR-STORAGE-013: Unexpected error calling Data Storage Service - {e}"
            )
            raise

    def _transform_api_response(self, api_workflows: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """
        Transform Data Storage API response to tool result format

        Business Requirement: BR-STORAGE-013, BR-AI-075
        Design Decisions:
            - DD-WORKFLOW-004 - Hybrid Weighted Label Scoring
            - DD-WORKFLOW-002 v2.4 - container_image and container_digest in response
            - DD-CONTRACT-001 v1.2 - AIAnalysis passes container_image to WorkflowExecution
            - DD-WORKFLOW-012 - Workflow fields are immutable

        Args:
            api_workflows: List of WorkflowSearchResult from API

        Returns:
            List of workflow dicts in tool format (LLM-friendly)

        DD-WORKFLOW-002 v3.0 Response Format (FLAT - no nested 'workflow' object):
            - workflow_id: UUID (auto-generated primary key)
            - title: string (display name)
            - description: string
            - signal_type: string (singular, not array)
            - container_image: string
            - container_digest: string
            - confidence: float (0.0-1.0 similarity score)

        V1.0: No boost/penalty - just base similarity (deferred to V2.0+)
        """
        results = []

        for api_wf in api_workflows:
            # DD-WORKFLOW-002 v3.0: FLAT structure (no nested 'workflow' object)
            # v2.x had: {"workflow": {...}, "final_score": 0.59}
            # v3.0 has: {"workflow_id": "uuid", "title": "...", "confidence": 0.59, ...}

            # Build tool result format (per DD-WORKFLOW-002 v3.0)
            tool_workflow = {
                # DD-WORKFLOW-002 v3.0: workflow_id is UUID
                "workflow_id": api_wf.get("workflow_id", ""),
                # DD-WORKFLOW-002 v3.0: title is display name (version removed from response)
                "title": api_wf.get("title", ""),
                "description": api_wf.get("description", ""),
                # DD-WORKFLOW-002 v3.0: signal_type is singular string (not array)
                "signal_type": api_wf.get("signal_type", ""),
                # DD-WORKFLOW-002 v3.0: confidence (v2.x used "final_score")
                "confidence": api_wf.get("confidence", 0.0),
                # Container execution fields (DD-WORKFLOW-002 v2.4, DD-CONTRACT-001 v1.2)
                "container_image": api_wf.get("container_image"),
                "container_digest": api_wf.get("container_digest"),
            }

            results.append(tool_workflow)

        return results

    # NOTE: Audit event generation methods removed per DD-WORKFLOW-014 v2.0
    # Audit is now generated by Data Storage Service, which has richer context:
    # - All workflow metadata (owner, maintainer, version history)
    # - Success metrics (execution history)
    # - Full scoring breakdown (from database query)
    # HolmesGPT API only passes remediation_id in JSON body for correlation.

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
    Design Decisions:
      - DD-WORKFLOW-002 v2.3 - MCP Workflow Catalog Architecture
      - DD-WORKFLOW-014 v2.1 - Audit trail (Data Storage generates)
      - DD-LLM-001 - MCP Workflow Search Parameter Taxonomy

    Query Format (per DD-LLM-001):
    - Structured format: '<signal_type> <severity> [optional_keywords]'
    - signal_type: Canonical Kubernetes event reason from LLM's RCA findings
    - severity: LLM's RCA severity assessment (critical/high/medium/low)
    - Example: "OOMKilled critical", "NodeNotReady critical infrastructure"

    Architecture:
    - Embedded toolset in holmesgpt-api
    - Calls Data Storage Service REST API for semantic search
    - Two-phase search: exact label matching + pgvector similarity
    - Hybrid weighted scoring: confidence = base + boost - penalty
    - Expected confidence: 90-95% for exact matches

    Audit Trail (DD-WORKFLOW-014 v2.1):
    - HolmesGPT API passes remediation_id in JSON body
    - Data Storage Service generates audit events (richer context)
    - remediation_id is for CORRELATION ONLY (not used in search logic)
    """

    def __init__(self, enabled: bool = True, remediation_id: Optional[str] = None):
        """
        Initialize workflow catalog toolset

        Args:
            enabled: Whether the toolset is enabled
            remediation_id: Remediation request ID for audit correlation (DD-WORKFLOW-002 v2.2)
                           MANDATORY for audit trail - passed through to SearchWorkflowCatalogTool
                           This is for CORRELATION ONLY - not used in workflow search logic
        """
        super().__init__(
            name="workflow/catalog",
            description="Search and retrieve remediation workflows based on incident characteristics",
            enabled=enabled,
            status=ToolsetStatusEnum.ENABLED,  # CRITICAL: Must be ENABLED for SDK to include in function calling
            tools=[SearchWorkflowCatalogTool(remediation_id=remediation_id)],
            docs_url=(
                "https://github.com/jordigilh/kubernaut/blob/main/docs/architecture/decisions/"
                "DD-WORKFLOW-002-MCP-WORKFLOW-CATALOG-ARCHITECTURE.md"
            ),
            llm_instructions=(
                "Use this toolset to find appropriate remediation workflows for incidents. "
                "IMPORTANT: Only search for workflows AFTER you have completed your investigation "
                "and identified the root cause. Provide a structured query in format '<signal_type> <severity>' "
                "per DD-LLM-001, where signal_type is the canonical Kubernetes event reason from your RCA findings "
                "(not the initial signal). The tool returns ranked workflows with confidence scores (90-95% for exact matches) "
                "to help you select the most appropriate workflow."
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
