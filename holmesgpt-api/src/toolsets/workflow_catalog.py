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
  - DD-HAPI-001 - Custom Labels Auto-Append Architecture

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

# OpenAPI client imports (datastorage package is in src/clients, added to sys.path)
from datastorage.api.workflow_catalog_api_api import WorkflowCatalogAPIApi
from datastorage.models.workflow_search_request import WorkflowSearchRequest
from datastorage.models.workflow_search_filters import WorkflowSearchFilters
from datastorage.api_client import ApiClient
from datastorage.configuration import Configuration
from datastorage.exceptions import ApiException

# Import DetectedLabels for type hints
from src.models.incident_models import DetectedLabels

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
# DETECTED LABELS VALIDATION (100% SAFE)
# ========================================
# DD-WORKFLOW-001 v1.7: DetectedLabels validation with owner chain support
#
# PRINCIPLE: Include ONLY when relationship is PROVEN. Default to EXCLUDE.
# This ensures 100% safety - we never include wrong labels that could cause
# query failures or return incorrect workflows.
#
# Two-tier validation:
# 1. Owner chain (preferred): Use actual K8s ownerReferences from SignalProcessing
# 2. Heuristic fallback: Namespace/kind matching when owner_chain unavailable

# Cluster-scoped resources (no namespace)
CLUSTER_SCOPED_KINDS = {"Node", "PersistentVolume", "ClusterRole", "ClusterRoleBinding", "Namespace", "StorageClass"}


def strip_failed_detections(detected_labels: DetectedLabels) -> DetectedLabels:
    """
    Strip fields where detection failed from DetectedLabels.

    Business Requirement: BR-HAPI-194 - Honor failedDetections in workflow filtering
    Design Decision: DD-WORKFLOW-001 v2.1 - DetectedLabels failedDetections field

    Key Distinction (per SignalProcessing team):
    | Scenario    | pdbProtected | failedDetections     | Meaning                    |
    |-------------|--------------|----------------------|----------------------------|
    | PDB exists  | true         | []                   | ✅ Has PDB - use for filter |
    | No PDB      | false        | []                   | ✅ No PDB - use for filter  |
    | RBAC denied | false        | ["pdbProtected"]     | ⚠️ Unknown - skip filter    |

    "Resource doesn't exist" ≠ detection failure - it's a successful detection with result `false`.

    Args:
        detected_labels: DetectedLabels Pydantic model potentially containing failedDetections

    Returns:
        New DetectedLabels model with:
        - failedDetections cleared (empty list)
        - Fields listed in failedDetections set to None
        - All other fields preserved
    """
    if not detected_labels:
        return DetectedLabels()

    # Handle both dict and Pydantic model
    if isinstance(detected_labels, dict):
        failed_fields = set(detected_labels.get("failed_detections", []))
        clean_dict = {k: v for k, v in detected_labels.items() if v is not None}
    else:
        # Get list of failed detection field names
        failed_fields = set(detected_labels.failedDetections)
        # Convert to dict, exclude failed fields, rebuild DetectedLabels
        clean_dict = detected_labels.model_dump(exclude_none=True)

    # Remove failedDetections meta field
    clean_dict.pop("failedDetections", None)

    # Remove fields where detection failed
    for field in failed_fields:
        if field in clean_dict:
            logger.debug(f"🔕 Stripping failed detection: {field} (detection failed)")
            clean_dict.pop(field, None)

    if failed_fields:
        logger.info(
            f"📋 DD-WORKFLOW-001 v2.1: Stripped {len(failed_fields)} failed detections: {sorted(failed_fields)}"
        )

    # Return new DetectedLabels with cleaned data
    return DetectedLabels(**clean_dict)


def _resources_match(r1: Dict[str, str], r2: Dict[str, Any]) -> bool:
    """
    Check if two resource references match (kind + namespace + name).

    Args:
        r1: First resource {kind, namespace, name}
        r2: Second resource {kind, namespace, name}

    Returns:
        True if all fields match
    """
    return (
        r1.get("kind", "") == r2.get("kind", "") and
        r1.get("namespace", "") == r2.get("namespace", "") and
        r1.get("name", "") == r2.get("name", "")
    )


def should_include_detected_labels(
    source_resource: Optional[Dict[str, str]],
    rca_resource: Optional[Dict[str, Any]],
    owner_chain: Optional[List[Dict[str, str]]] = None
) -> bool:
    """
    100% SAFE: Include DetectedLabels ONLY when relationship is PROVEN.

    Design Decision: DD-WORKFLOW-001 v1.7

    DetectedLabels describe the original signal's resource characteristics.
    Including wrong labels for a different resource can cause:
    - Query failures (no workflows match)
    - Wrong workflows returned

    SAFETY PRINCIPLE: Default to EXCLUDE. Only include when we can PROVE
    the RCA resource is related to the source resource.

    Validation order:
    1. Required data check (source_resource, rca_resource, rca_resource.kind)
    2. Exact match (same resource)
    3. Owner chain match (K8s ownerReferences from SignalProcessing)
    4. Same namespace + kind (fallback when owner_chain provided but empty)
    5. Default: EXCLUDE (safe - prevents query failures)

    Args:
        source_resource: Original signal's resource {namespace, kind, name}
        rca_resource: LLM's RCA resource {signal_type, namespace, kind, name}
        owner_chain: K8s ownership chain from SignalProcessing enrichment
                    [{kind, namespace, name}, ...] - ordered from direct parent to root
                    Example for Pod: [ReplicaSet, Deployment]

    Returns:
        True ONLY if relationship is PROVEN, False otherwise (safe default)

    Examples:
        - Pod(prod/api-xyz) → Pod(prod/api-xyz): True (exact match)
        - Pod(prod/api-xyz) → Deployment(prod/api) with owner_chain: True (proven)
        - Pod(prod/api-xyz) → Node(worker-3): False (different scope)
        - Pod(prod/api-xyz) → Deployment(prod/api) no owner_chain: False (can't prove)
    """
    # GATE 1: Required data must be present
    if not source_resource:
        logger.debug("DetectedLabels EXCLUDED - source_resource missing")
        return False

    if not rca_resource:
        logger.debug("DetectedLabels EXCLUDED - rca_resource not provided by LLM")
        return False

    if not rca_resource.get("kind"):
        logger.debug("DetectedLabels EXCLUDED - rca_resource.kind missing")
        return False

    source_kind = source_resource.get("kind", "")
    rca_kind = rca_resource.get("kind", "")
    source_ns = source_resource.get("namespace", "")
    rca_ns = rca_resource.get("namespace", "")

    # GATE 2: Exact match (same resource)
    if _resources_match(source_resource, rca_resource):
        logger.info(
            f"🔍 DetectedLabels INCLUDED - exact resource match: "
            f"{source_kind}/{source_ns}/{source_resource.get('name')}"
        )
        return True

    # GATE 3: Owner chain match (PROVEN relationship via K8s ownerReferences)
    if owner_chain:
        for owner in owner_chain:
            if _resources_match(owner, rca_resource):
                logger.info(
                    f"🔍 DetectedLabels INCLUDED - owner chain match: "
                    f"{source_kind} → {owner.get('kind')}/{owner.get('name')} (proven)"
                )
                return True

    # GATE 4: Same namespace + same kind (conservative fallback)
    # Only if owner_chain was explicitly provided (even if empty)
    # This handles sibling resources in same workload context
    if owner_chain is not None:
        # Check scope compatibility first
        source_is_cluster = source_kind in CLUSTER_SCOPED_KINDS
        rca_is_cluster = rca_kind in CLUSTER_SCOPED_KINDS

        if source_is_cluster and rca_is_cluster:
            # Both cluster-scoped - same kind is sufficient
            if source_kind == rca_kind:
                logger.info(
                    f"🔍 DetectedLabels INCLUDED - same cluster-scoped kind: {source_kind}"
                )
                return True
        elif not source_is_cluster and not rca_is_cluster:
            # Both namespaced - check namespace + kind
            if source_ns == rca_ns and source_kind == rca_kind:
                logger.info(
                    f"🔍 DetectedLabels INCLUDED - same namespace/kind: "
                    f"{source_kind}/{source_ns}"
                )
                return True

    # DEFAULT: Cannot prove relationship → EXCLUDE (100% safe)
    logger.info(
        f"⚠️  DetectedLabels EXCLUDED - no proven relationship: "
        f"source={source_kind}/{source_ns or 'cluster'}, "
        f"rca={rca_kind}/{rca_ns or 'cluster'}, "
        f"owner_chain={'provided' if owner_chain is not None else 'not provided'}"
    )
    return False


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

    def __init__(
        self,
        data_storage_url: Optional[str] = None,
        remediation_id: Optional[str] = None,
        custom_labels: Optional[Dict[str, List[str]]] = None,
        detected_labels: Optional[DetectedLabels] = None,
        source_resource: Optional[Dict[str, str]] = None,
        owner_chain: Optional[List[Dict[str, str]]] = None
    ):
        """
        Initialize SearchWorkflowCatalogTool.

        Args:
            data_storage_url: Data Storage Service URL (default from env or http://data-storage:8080)
            remediation_id: Remediation request ID for audit correlation (DD-WORKFLOW-014)
            custom_labels: Custom labels for auto-append to workflow search (DD-HAPI-001)
                          Format: map[string][]string (subdomain → values)
                          Example: {"constraint": ["cost-constrained"], "team": ["name=payments"]}
                          These are auto-appended to all MCP calls - invisible to LLM.
            detected_labels: Auto-detected labels for workflow matching (DD-WORKFLOW-001 v1.7)
                            Format: {"gitOpsManaged": true, "gitOpsTool": "argocd", ...}
                            Only booleans=true and non-empty strings are included.
                            ONLY included if RCA resource relationship is PROVEN.
            source_resource: Original signal's resource for DetectedLabels validation
                            Format: {"namespace": "production", "kind": "Pod", "name": "api-xyz"}
                            Used to compare against LLM's rca_resource.
            owner_chain: K8s ownership chain from SignalProcessing enrichment
                        Format: [{"namespace": "prod", "kind": "ReplicaSet", "name": "api-xyz"}, ...]
                        Ordered from direct parent to root (e.g., [ReplicaSet, Deployment])
                        Used for PROVEN relationship validation (100% safe).
        """
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
                "rca_resource": ToolParameter(
                    type="object",
                    description=(
                        "The Kubernetes resource you identified as root cause. REQUIRED for accurate workflow matching. "
                        "Include: signal_type (the issue found), kind (resource type), namespace (if namespaced), name (resource name). "
                        "Examples: "
                        "{signal_type: 'DiskPressure', kind: 'Node', name: 'worker-3'} for cluster-scoped resources, "
                        "{signal_type: 'OOMKilled', kind: 'Pod', namespace: 'production', name: 'api-server-xyz'} for namespaced resources."
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
                "You MUST provide rca_resource with the root cause resource details for accurate workflow matching. "
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

        # Initialize OpenAPI client for type-safe Data Storage API calls
        # DD-STORAGE-011: OpenAPI Client Generation
        config = Configuration(host=self._data_storage_url)
        api_client = ApiClient(configuration=config)
        object.__setattr__(self, '_search_api', WorkflowCatalogAPIApi(api_client))

        # Store remediation_id for audit correlation (BR-AUDIT-005, DD-WORKFLOW-014)
        # This is the correlation_id that groups all audit events for a remediation
        # Value: remediation_id from AIAnalysis controller (e.g., "req-2025-10-06-abc123")
        object.__setattr__(self, '_remediation_id', remediation_id or "")

        # Store custom_labels for auto-append (DD-HAPI-001)
        # These are automatically appended to all workflow search calls
        # LLM does NOT need to know about or provide these - they are invisible to the LLM
        # Format: map[string][]string (subdomain → list of values)
        object.__setattr__(self, '_custom_labels', custom_labels or {})

        # Store detected_labels for workflow matching (DD-WORKFLOW-001 v1.6)
        # Auto-detected from K8s resources by SignalProcessing
        # Used for workflow wildcard matching (e.g., workflow specifies gitOpsTool: "*")
        # Format: DetectedLabels Pydantic model
        # Only booleans=true and non-empty strings are included (Boolean Normalization Rule)
        # IMPORTANT: Only included if RCA resource matches source resource context
        object.__setattr__(self, '_detected_labels', detected_labels or DetectedLabels())

        # Store source_resource for DetectedLabels validation
        # When LLM provides rca_resource, we compare to source_resource
        # to decide if detected_labels should be included in the search
        # Format: {"namespace": "production", "kind": "Pod", "name": "api-xyz"}
        object.__setattr__(self, '_source_resource', source_resource or {})

        # Store owner_chain for PROVEN relationship validation (DD-WORKFLOW-001 v1.7)
        # From SignalProcessing's K8s ownerReferences traversal
        # Format: [{"kind": "ReplicaSet", ...}, {"kind": "Deployment", ...}]
        # None = not provided (use heuristics), [] = explicitly empty (orphan resource)
        object.__setattr__(self, '_owner_chain', owner_chain)

        # Log labels info for debugging (not the values for security)
        custom_labels_info = f"{len(self._custom_labels)} subdomains" if self._custom_labels else "none"
        # Count non-None fields in DetectedLabels model
        # Handle both dict and Pydantic model for detected_labels
        if self._detected_labels:
            if isinstance(self._detected_labels, dict):
                detected_labels_count = len([f for f in self._detected_labels.keys() if f != "failed_detections"])
            else:
                detected_labels_count = len([f for f in self._detected_labels.model_dump(exclude_none=True).keys() if f != "failedDetections"])
        else:
            detected_labels_count = 0
        detected_labels_info = f"{detected_labels_count} fields" if self._detected_labels else "none"
        source_resource_info = f"{source_resource.get('kind', 'unknown')}/{source_resource.get('namespace', 'cluster')}" if source_resource else "none"
        owner_chain_info = f"{len(owner_chain)} owners" if owner_chain else ("empty" if owner_chain == [] else "not provided")
        logger.info(
            f"🔄 BR-STORAGE-013: Workflow catalog configured - "
            f"data_storage_url={self._data_storage_url}, timeout={self._http_timeout}s, "
            f"remediation_id={self._remediation_id or 'not-set'}, "
            f"custom_labels={custom_labels_info}, detected_labels={detected_labels_info}, "
            f"source_resource={source_resource_info}, owner_chain={owner_chain_info}"
        )

    @property
    def data_storage_url(self) -> str:
        """Get Data Storage Service URL"""
        return object.__getattribute__(self, '_data_storage_url')

    @data_storage_url.setter
    def data_storage_url(self, value: str):
        """Set Data Storage Service URL (for testing)"""
        object.__setattr__(self, '_data_storage_url', value)
        # Reinitialize OpenAPI client with new URL
        config = Configuration(host=value)
        api_client = ApiClient(configuration=config)
        object.__setattr__(self, '_search_api', WorkflowCatalogAPIApi(api_client))

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
            rca_resource = params.get("rca_resource", {})
            filters = params.get("filters", {})
            top_k = params.get("top_k", 3)

            # Validate top_k (BR-HAPI-250: max 10 results)
            if top_k > 10:
                logger.warning(f"top_k={top_k} exceeds maximum (10), capping to 10")
                top_k = 10

            # Log RCA resource for debugging
            rca_info = f"{rca_resource.get('kind', 'unknown')}/{rca_resource.get('namespace', 'cluster')}" if rca_resource else "not-provided"
            logger.info(
                f"🔍 BR-HAPI-250: Workflow catalog search - "
                f"query='{query}', rca_resource={rca_info}, filters={filters}, top_k={top_k}"
            )

            # Search workflows
            # DD-WORKFLOW-014 v2.0: remediation_id passed in JSON body
            # Data Storage Service generates audit events (not HolmesGPT API)
            workflows = self._search_workflows(query, rca_resource, filters, top_k)

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

    def _extract_component_from_rca(self, rca_resource: Dict[str, Any]) -> Optional[str]:
        """
        Extract component (pod/deployment/node) from RCA resource.

        Business Requirement: BR-HAPI-250 (Workflow Catalog Search Tool)
        Design Decision: DD-WORKFLOW-001 v1.6 (component is mandatory)

        Args:
            rca_resource: RCA resource with kind field

        Returns:
            Component type from RCA resource kind, or None if not available
        """
        if not rca_resource:
            return None

        # Extract kind from RCA resource
        kind = rca_resource.get("kind", "").lower()
        if not kind:
            return None

        # Map Kubernetes kinds to component labels
        kind_mapping = {
            "pod": "pod",
            "deployment": "deployment",
            "replicaset": "deployment",  # ReplicaSets are managed by Deployments
            "statefulset": "statefulset",
            "daemonset": "daemonset",
            "node": "node",
            "service": "service",
            "persistentvolumeclaim": "pvc",
            "persistentvolume": "pv",
        }

        return kind_mapping.get(kind, kind)

    def _extract_environment_from_rca(self, rca_resource: Dict[str, Any]) -> Optional[str]:
        """
        Extract environment from RCA resource namespace.

        Business Requirement: BR-HAPI-250 (Workflow Catalog Search Tool)
        Design Decision: DD-WORKFLOW-001 v1.6 (environment is mandatory)

        Args:
            rca_resource: RCA resource with namespace field

        Returns:
            Environment inferred from namespace, or None if not available
        """
        if not rca_resource:
            return None

        namespace = rca_resource.get("namespace", "").lower()
        if not namespace:
            return None

        # Heuristic mapping (customers should use custom_labels for precise control)
        if "prod" in namespace or "production" in namespace:
            return "production"
        elif "stag" in namespace or "staging" in namespace:
            return "staging"
        elif "dev" in namespace or "development" in namespace:
            return "development"
        elif "test" in namespace:
            return "test"

        return None  # Will default to "*" wildcard in caller

    def _map_severity_to_priority(self, severity: str) -> str:
        """
        Map severity level to priority level.

        Business Requirement: BR-HAPI-250 (Workflow Catalog Search Tool)
        Design Decision: DD-WORKFLOW-001 v1.6 (priority is mandatory)

        Args:
            severity: critical/high/medium/low

        Returns:
            Priority level: P0/P1/P2/P3
        """
        severity_to_priority = {
            "critical": "P0",
            "high": "P1",
            "medium": "P2",
            "low": "P3",
        }
        return severity_to_priority.get(severity.lower(), "P2")  # Default to P2

    def _build_filters_from_query(
        self, query: str, rca_resource: Dict[str, Any], additional_filters: Dict
    ) -> Dict[str, Any]:
        """
        Transform LLM query into WorkflowSearchFilters format

        Business Requirement: BR-STORAGE-013
        Design Decisions:
            - DD-LLM-001 - MCP Workflow Search Parameter Taxonomy
            - DD-WORKFLOW-001 v1.6 - 5 Mandatory Labels

        Args:
            query: Structured query '<signal_type> <severity> [keywords]' (per DD-LLM-001)
            rca_resource: RCA resource for component/environment extraction
            additional_filters: Additional filters from LLM (optional labels)

        Returns:
            WorkflowSearchFilters dict with 5 mandatory fields:
            - signal_type (REQUIRED)
            - severity (REQUIRED)
            - component (REQUIRED)
            - environment (REQUIRED)
            - priority (REQUIRED)

        Example:
            Input: "OOMKilled critical", rca_resource={kind: "Pod", namespace: "production"}
            Output: {
                "signal_type": "OOMKilled",
                "severity": "critical",
                "component": "pod",
                "environment": "production",
                "priority": "P0"
            }
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

        # Build filters with 5 MANDATORY fields (DD-WORKFLOW-001 v2.5)
        filters = {
            "signal_type": signal_type,
            "severity": severity,
            # NEW: 3 additional mandatory fields with smart defaults
            # NOTE: Wildcards should be in workflow labels (DB), not search filters
            # Default to most common values when RCA resource unavailable
            "component": self._extract_component_from_rca(rca_resource) or "pod",  # Most common K8s resource
            # DD-WORKFLOW-001 v2.5: Single environment value from Signal Processing
            "environment": self._extract_environment_from_rca(rca_resource) or "production",  # Single environment
            "priority": self._map_severity_to_priority(severity),  # Map severity → priority
        }

        # Merge additional filters (optional labels per DD-WORKFLOW-004)
        # These can override the smart defaults above
        if additional_filters:
            # Map from LLM filter names to API field names
            filter_mapping = {
                "resource_management": "resource-management",
                "gitops_tool": "gitops-tool",
                "environment": "environment",
                "component": "component",  # Allow LLM to override default
                "business_category": "business-category",
                "priority": "priority",  # Allow LLM to override default
                "risk_tolerance": "risk-tolerance"
            }

            for llm_key, api_key in filter_mapping.items():
                if llm_key in additional_filters:
                    value = additional_filters[llm_key]
                    # DD-WORKFLOW-001 v2.5: Environment is single string (not array)
                    filters[api_key] = value

        return filters

    def _search_workflows(
        self, query: str, rca_resource: Dict[str, Any], filters: Dict, top_k: int
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
          - DD-WORKFLOW-001 v1.6 - DetectedLabels validation against RCA resource

        Args:
            query: Structured query string '<signal_type> <severity> [keywords]' (per DD-LLM-001)
            rca_resource: LLM's RCA resource {signal_type, kind, namespace, name}
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

        DD-WORKFLOW-001 v1.6: DetectedLabels validation
          - Compare source_resource to rca_resource
          - Only include detected_labels if same resource context
          - Handles Pod ↔ Deployment ownership relationships
        """
        try:
            # Build request per DD-STORAGE-008
            # DD-WORKFLOW-001 v1.6: Now includes 5 mandatory fields (signal_type, severity, component, environment, priority)
            search_filters = self._build_filters_from_query(query, rca_resource, filters)

            # DD-HAPI-001: Auto-append custom_labels to filters (invisible to LLM)
            # Custom labels are passed through from the original request context
            # They are NOT provided by the LLM - HolmesGPT-API auto-appends them
            if self._custom_labels:
                search_filters["custom_labels"] = self._custom_labels
                logger.debug(
                    f"🏷️  DD-HAPI-001: Auto-appending custom_labels - "
                    f"subdomains={list(self._custom_labels.keys())}"
                )

            # DD-WORKFLOW-001 v1.7: Conditionally append detected_labels (100% SAFE)
            # DetectedLabels are ONLY included when relationship is PROVEN
            # Uses owner_chain from SignalProcessing for deterministic validation
            # Default: EXCLUDE (prevents query failures from wrong labels)
            #
            # DD-WORKFLOW-001 v2.1: Strip fields where detection failed
            # Fields in failedDetections are removed before sending to Data Storage
            # This prevents filtering on unknown values (e.g., RBAC denied)
            if self._detected_labels:
                if should_include_detected_labels(
                    self._source_resource,
                    rca_resource,
                    self._owner_chain
                ):
                    # Strip failed detections before passing to Data Storage
                    clean_labels = strip_failed_detections(self._detected_labels)
                    # Check if clean_labels has meaningful data (not just defaults)
                    clean_labels_dict = clean_labels.model_dump(exclude_defaults=True, exclude_none=True)
                    if clean_labels_dict:  # Only include if there are non-default reliable labels
                        # Convert Pydantic model to dict for API call
                        search_filters["detected_labels"] = clean_labels.model_dump(exclude_defaults=True, exclude_none=True)
                # Logging is handled inside should_include_detected_labels() and strip_failed_detections()

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

            # Log custom_labels presence (not values for security)
            custom_labels_info = f", custom_labels={len(self._custom_labels)} subdomains" if self._custom_labels else ""
            logger.info(
                f"🔍 BR-STORAGE-013: Calling Data Storage Service - "
                f"url={self._data_storage_url}/api/v1/workflows/search, "
                f"query='{query}', filters={search_filters}, top_k={top_k}, "
                f"remediation_id={self._remediation_id or 'not-set'}{custom_labels_info}"
            )

            # Call Data Storage Service using OpenAPI client (type-safe)
            # DD-STORAGE-011: OpenAPI Client Generation
            # DD-WORKFLOW-014 v2.1: remediation_id in JSON body for audit correlation
            import time
            start_time = time.time()

            # Build type-safe request objects
            filters_obj = WorkflowSearchFilters(**search_filters)
            request_obj = WorkflowSearchRequest(
                query=query,
                filters=filters_obj,
                top_k=top_k,
                min_similarity=0.3,
                remediation_id=self._remediation_id
            )

            # Execute type-safe API call
            search_response = self._search_api.search_workflows(
                workflow_search_request=request_obj,
                _request_timeout=self._http_timeout
            )

            # Extract workflows from typed response
            # Use model_dump(mode='json') to properly serialize UUID to string
            api_workflows = [w.model_dump(mode='json') for w in search_response.workflows] if search_response.workflows else []
            duration_ms = int((time.time() - start_time) * 1000)

            logger.info(
                f"✅ BR-STORAGE-013: Data Storage Service responded - "
                f"total_results={search_response.total_results or 0}, "
                f"returned={len(api_workflows)}, "
                f"duration_ms={duration_ms}"
            )

            # Transform response to tool format
            return self._transform_api_response(api_workflows)

        except ApiException as e:
            # OpenAPI client HTTP errors (400, 404, 500, etc.)
            logger.error(
                f"❌ BR-STORAGE-013: Data Storage Service API error - "
                f"status={e.status}, reason={e.reason}, body={e.body}"
            )
            if e.status == 408 or "timeout" in str(e).lower():
                raise Exception(f"Data Storage Service timeout after {self._http_timeout}s")
            elif e.status >= 500:
                raise Exception(f"Data Storage Service internal error: {e.reason}")
            elif e.status >= 400:
                raise Exception(f"Data Storage Service request error: {e.reason}")
            else:
                raise Exception(f"Data Storage Service error: {e}")
        except requests.exceptions.Timeout as e:
            # HTTP timeout (from underlying urllib3)
            logger.error(
                f"⏱️  BR-STORAGE-013: Data Storage Service timeout - {e}"
            )
            raise Exception(f"Data Storage Service timeout after {self._http_timeout}s")
        except (requests.exceptions.ConnectionError, ConnectionRefusedError) as e:
            # Connection failures
            logger.error(
                f"🔌 BR-STORAGE-013: Data Storage Service connection failed - {e}"
            )
            raise Exception(f"Cannot connect to Data Storage Service at {self._data_storage_url}")
        except ValueError as e:
            # URL parsing errors (invalid hostname, malformed URL, etc.)
            # This catches urllib3 LocationValueError and similar parsing errors
            logger.error(
                f"🔗 BR-STORAGE-013: Invalid Data Storage Service URL - {e}"
            )
            raise Exception(f"Invalid Data Storage Service URL: {self._data_storage_url}")
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
      - DD-HAPI-001 - Custom Labels Auto-Append Architecture

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

    Custom Labels Auto-Append (DD-HAPI-001):
    - custom_labels are extracted from enrichment_results.customLabels
    - auto-appended to all MCP workflow search calls
    - invisible to LLM (LLM does NOT provide these)
    - ensures 100% reliable custom label filtering
    """

    def __init__(
        self,
        enabled: bool = True,
        remediation_id: Optional[str] = None,
        custom_labels: Optional[Dict[str, List[str]]] = None,
        detected_labels: Optional[DetectedLabels] = None,
        source_resource: Optional[Dict[str, str]] = None,
        owner_chain: Optional[List[Dict[str, str]]] = None
    ):
        """
        Initialize workflow catalog toolset

        Args:
            enabled: Whether the toolset is enabled
            remediation_id: Remediation request ID for audit correlation (DD-WORKFLOW-002 v2.2)
                           MANDATORY for audit trail - passed through to SearchWorkflowCatalogTool
                           This is for CORRELATION ONLY - not used in workflow search logic
            custom_labels: Custom labels for auto-append (DD-HAPI-001)
                          Format: map[string][]string (subdomain → list of values)
                          Example: {"constraint": ["cost-constrained"], "team": ["name=payments"]}
                          Auto-appended to all MCP workflow search calls - invisible to LLM.
            detected_labels: Auto-detected labels for workflow matching (DD-WORKFLOW-001 v1.7)
                            Format: {"gitOpsManaged": true, "gitOpsTool": "argocd", ...}
                            Only booleans=true and non-empty strings are included.
                            ONLY included if relationship is PROVEN (100% safe).
            source_resource: Original signal's resource for DetectedLabels validation
                            Format: {"namespace": "production", "kind": "Pod", "name": "api-xyz"}
                            Compared against LLM's rca_resource.
            owner_chain: K8s ownership chain from SignalProcessing enrichment
                        Format: [{"namespace": "prod", "kind": "ReplicaSet", "name": "api-xyz"}, ...]
                        Used for PROVEN relationship validation (100% safe).
        """
        super().__init__(
            name="workflow/catalog",
            description="Search and retrieve remediation workflows based on incident characteristics",
            enabled=enabled,
            status=ToolsetStatusEnum.ENABLED,  # CRITICAL: Must be ENABLED for SDK to include in function calling
            tools=[SearchWorkflowCatalogTool(
                remediation_id=remediation_id,
                custom_labels=custom_labels,  # DD-HAPI-001: Pass custom_labels for auto-append
                detected_labels=detected_labels,  # DD-WORKFLOW-001 v1.7: Pass detected_labels (100% safe)
                source_resource=source_resource,  # DD-WORKFLOW-001 v1.7: For RCA resource comparison
                owner_chain=owner_chain  # DD-WORKFLOW-001 v1.7: For PROVEN relationship validation
            )],
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
