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
Three-Step Workflow Discovery Toolset for HolmesGPT SDK

Authority: DD-WORKFLOW-016 (Action-Type Workflow Catalog Indexing)
Authority: DD-HAPI-017 (Three-Step Workflow Discovery Integration)
Business Requirement: BR-HAPI-017-001 (Three-Step Tool Implementation)
Business Requirement: BR-HAPI-017-005 (remediationId Propagation)

Three-Step Discovery Protocol:
  Step 1: list_available_actions  → GET /api/v1/workflows/actions
  Step 2: list_workflows          → GET /api/v1/workflows/actions/{action_type}
  Step 3: get_workflow            → GET /api/v1/workflows/{workflow_id}

Replaces: SearchWorkflowCatalogTool / WorkflowCatalogToolset (DD-WORKFLOW-002)

Signal Context Filters (propagated as query params on all three steps):
  - severity: From RCA findings (critical/high/medium/low)
  - component: K8s resource kind (pod/deployment/node/etc.)
  - environment: Namespace-derived (production/staging/development)
  - priority: Severity-mapped (P0/P1/P2/P3)
  - remediation_id: Audit correlation ID
  - custom_labels: Custom labels (DD-HAPI-001)
  - detected_labels: Infrastructure characteristics (DD-HAPI-017, DD-WORKFLOW-001 v2.1)

Audit Trail:
  - DS generates audit events per DD-WORKFLOW-014 v3.0
  - HAPI does NOT generate audit events
  - remediation_id passed via query param for correlation

Configuration:
  - DATA_STORAGE_URL: Data Storage Service endpoint (default: http://data-storage:8080)
  - DATA_STORAGE_TIMEOUT: HTTP timeout in seconds (default: 60)
"""

import logging
import json
import os
import requests
from typing import Dict, Any, List, Optional

from src.models.incident_models import DetectedLabels

from holmes.core.tools import (
    Tool,
    Toolset,
    StructuredToolResult,
    StructuredToolResultStatus,
    ToolParameter,
    ToolsetStatusEnum,
)

logger = logging.getLogger(__name__)

# ========================================
# SHARED UTILITIES (moved from workflow_catalog.py — Gap 5, DD-WORKFLOW-016)
# ========================================

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
    | PDB exists  | true         | []                   | Has PDB - use for filter   |
    | No PDB      | false        | []                   | No PDB - use for filter    |
    | RBAC denied | false        | ["pdbProtected"]     | Unknown - skip filter      |

    "Resource doesn't exist" != detection failure - it's a successful detection with result false.

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
            logger.debug(f"Stripping failed detection: {field} (detection failed)")
            clean_dict.pop(field, None)

    if failed_fields:
        logger.info(
            f"DD-WORKFLOW-001 v2.1: Stripped {len(failed_fields)} failed detections: {sorted(failed_fields)}"
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
        r1.get("kind", "") == r2.get("kind", "")
        and r1.get("namespace", "") == r2.get("namespace", "")
        and r1.get("name", "") == r2.get("name", "")
    )


def should_include_detected_labels(
    source_resource: Optional[Dict[str, str]],
    rca_resource: Optional[Dict[str, Any]],
    owner_chain: Optional[List[Dict[str, str]]] = None,
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
        - Pod(prod/api-xyz) -> Pod(prod/api-xyz): True (exact match)
        - Pod(prod/api-xyz) -> Deployment(prod/api) with owner_chain: True (proven)
        - Pod(prod/api-xyz) -> Node(worker-3): False (different scope)
        - Pod(prod/api-xyz) -> Deployment(prod/api) no owner_chain: False (can't prove)
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
            f"DetectedLabels INCLUDED - exact resource match: "
            f"{source_kind}/{source_ns}/{source_resource.get('name')}"
        )
        return True

    # GATE 3: Owner chain match (PROVEN relationship via K8s ownerReferences)
    if owner_chain:
        for owner in owner_chain:
            if _resources_match(owner, rca_resource):
                logger.info(
                    f"DetectedLabels INCLUDED - owner chain match: "
                    f"{source_kind} -> {owner.get('kind')}/{owner.get('name')} (proven)"
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
                    f"DetectedLabels INCLUDED - same cluster-scoped kind: {source_kind}"
                )
                return True
        elif not source_is_cluster and not rca_is_cluster:
            # Both namespaced - check namespace + kind
            if source_ns == rca_ns and source_kind == rca_kind:
                logger.info(
                    f"DetectedLabels INCLUDED - same namespace/kind: "
                    f"{source_kind}/{source_ns}"
                )
                return True

    # DEFAULT: Cannot prove relationship -> EXCLUDE (100% safe)
    logger.info(
        f"DetectedLabels EXCLUDED - no proven relationship: "
        f"source={source_kind}/{source_ns or 'cluster'}, "
        f"rca={rca_kind}/{rca_ns or 'cluster'}, "
        f"owner_chain={'provided' if owner_chain is not None else 'not provided'}"
    )
    return False


# ========================================
# SHARED CONTEXT FOR ALL DISCOVERY TOOLS
# ========================================
# Signal context filters are set once at toolset creation time
# (from the incident/recovery signal) and propagated to all three
# discovery steps as query parameters.
# The LLM does NOT provide these — they come from the signal context.


class _DiscoveryToolBase(Tool):
    """
    Base class for all three discovery tools.

    Holds shared signal context filters and Data Storage connection info.
    Subclasses implement _invoke() with step-specific logic.
    """

    def __init__(
        self,
        name: str,
        description: str,
        parameters: Dict[str, ToolParameter],
        additional_instructions: str,
        data_storage_url: Optional[str] = None,
        remediation_id: Optional[str] = None,
        severity: str = "",
        component: str = "",
        environment: str = "",
        priority: str = "",
        custom_labels: Optional[Dict[str, List[str]]] = None,
        detected_labels: Optional[DetectedLabels] = None,
        http_session: Optional[Any] = None,
    ):
        super().__init__(
            name=name,
            description=description,
            parameters=parameters,
            additional_instructions=additional_instructions,
        )

        # Data Storage connection
        object.__setattr__(
            self,
            "_data_storage_url",
            data_storage_url
            or os.getenv("DATA_STORAGE_URL", "http://data-storage:8080"),
        )
        object.__setattr__(
            self,
            "_http_timeout",
            int(os.getenv("DATA_STORAGE_TIMEOUT", "60")),
        )

        # Optional requests.Session for authentication (DD-AUTH-014).
        # In production, HAPI runs inside K8s where DS auth is handled via
        # ServiceAccount token injection. For integration tests, an
        # authenticated Session can be provided.
        object.__setattr__(self, "_http_session", http_session)

        # Signal context filters (set once, propagated to all steps)
        object.__setattr__(self, "_remediation_id", remediation_id or "")
        object.__setattr__(self, "_severity", severity)
        object.__setattr__(self, "_component", component)
        object.__setattr__(self, "_environment", environment)
        object.__setattr__(self, "_priority", priority)
        object.__setattr__(self, "_custom_labels", custom_labels or {})
        object.__setattr__(self, "_detected_labels", detected_labels)

    def _build_context_params(self) -> Dict[str, Any]:
        """
        Build query parameters dict with signal context filters.

        These are appended to every discovery GET request.
        BR-HAPI-017-005: remediation_id propagated for audit correlation.
        DD-HAPI-017: detected_labels propagated when present (DD-WORKFLOW-001 v2.1:
        strip_failed_detections applied before sending).
        """
        params: Dict[str, Any] = {}
        if self._severity:
            params["severity"] = self._severity
        if self._component:
            params["component"] = self._component
        if self._environment:
            params["environment"] = self._environment
        if self._priority:
            params["priority"] = self._priority
        if self._remediation_id:
            params["remediation_id"] = self._remediation_id
        if self._custom_labels:
            params["custom_labels"] = json.dumps(self._custom_labels)
        if self._detected_labels:
            clean_labels = strip_failed_detections(self._detected_labels)
            clean_dict = clean_labels.model_dump(exclude_defaults=True, exclude_none=True)
            if clean_dict:
                params["detected_labels"] = json.dumps(clean_dict)
        return params

    def _do_get(self, url: str, extra_params: Optional[Dict] = None) -> Dict:
        """
        Execute GET request to Data Storage with context filters.

        Uses self._http_session if provided (DD-AUTH-014 integration tests),
        otherwise falls back to requests.get().

        Raises on HTTP errors (non-2xx).
        Returns parsed JSON response dict.
        """
        params = self._build_context_params()
        if extra_params:
            params.update(extra_params)

        http_get = self._http_session.get if self._http_session else requests.get
        response = http_get(
            url,
            params=params,
            timeout=self._http_timeout,
        )

        # Handle non-2xx
        if response.status_code == 404:
            body = {}
            try:
                body = response.json()
            except Exception:
                pass
            detail = body.get("detail", "Resource not found")
            raise ResourceNotFoundError(detail)
        response.raise_for_status()
        return response.json()


class ResourceNotFoundError(Exception):
    """Raised when DS returns 404 (security gate or missing resource)."""

    pass


# ========================================
# STEP 1: ListAvailableActionsTool
# ========================================


class ListAvailableActionsTool(_DiscoveryToolBase):
    """
    Step 1 of the three-step workflow discovery protocol.

    Discovers available remediation action types from the taxonomy,
    filtered by the current signal context.

    Authority: DD-WORKFLOW-016, DD-HAPI-017
    Business Requirement: BR-HAPI-017-001

    Endpoint: GET /api/v1/workflows/actions
    """

    def __init__(
        self,
        data_storage_url: Optional[str] = None,
        remediation_id: Optional[str] = None,
        severity: str = "",
        component: str = "",
        environment: str = "",
        priority: str = "",
        custom_labels: Optional[Dict[str, List[str]]] = None,
        detected_labels: Optional[DetectedLabels] = None,
        http_session: Optional[Any] = None,
    ):
        super().__init__(
            name="list_available_actions",
            description=(
                "List available remediation action types. Returns action types with descriptions "
                "explaining WHAT each action does, WHEN to use it, and WHEN NOT to use it. "
                "Use this as the FIRST step after RCA to discover what kinds of remediation "
                "actions are available for the current incident context."
            ),
            parameters={
                "offset": ToolParameter(
                    type="integer",
                    description="Pagination offset (default: 0)",
                    required=False,
                ),
                "limit": ToolParameter(
                    type="integer",
                    description="Pagination limit (default: 10, max: 100)",
                    required=False,
                ),
            },
            additional_instructions=(
                "IMPORTANT: Call this tool AFTER completing your Root Cause Analysis (RCA). "
                "Review ALL returned action types before selecting one. "
                "Each action type includes 'when_to_use' and 'when_not_to_use' guidance — "
                "use these to decide which action type best matches the incident. "
                "If hasMore is true, call again with increased offset to see all action types."
            ),
            data_storage_url=data_storage_url,
            remediation_id=remediation_id,
            severity=severity,
            component=component,
            environment=environment,
            priority=priority,
            custom_labels=custom_labels,
            detected_labels=detected_labels,
            http_session=http_session,
        )

    def _invoke(
        self, params: Dict, user_approved: bool = False
    ) -> StructuredToolResult:
        try:
            offset = params.get("offset", 0)
            limit = params.get("limit", 10)

            url = f"{self._data_storage_url}/api/v1/workflows/actions"
            extra_params = {"offset": offset, "limit": limit}

            logger.info(
                f"🔍 BR-HAPI-017-001 Step 1: Listing available actions — "
                f"severity={self._severity}, component={self._component}, "
                f"environment={self._environment}, priority={self._priority}, "
                f"offset={offset}, limit={limit}"
            )

            data = self._do_get(url, extra_params)

            logger.info(
                f"✅ Step 1 complete: {len(data.get('actionTypes', []))} action types, "
                f"total={data.get('pagination', {}).get('totalCount', '?')}"
            )

            return StructuredToolResult(
                status=StructuredToolResultStatus.SUCCESS,
                data=json.dumps(data, indent=2),
                params=params,
            )
        except Exception as e:
            logger.error(f"❌ Step 1 failed: {e}")
            return StructuredToolResult(
                status=StructuredToolResultStatus.ERROR,
                error=str(e),
                params=params,
            )

    def get_parameterized_one_liner(self, params: Dict) -> str:
        return (
            f"List available actions (severity={self._severity}, "
            f"component={self._component}, env={self._environment})"
        )


# ========================================
# STEP 2: ListWorkflowsTool
# ========================================


class ListWorkflowsTool(_DiscoveryToolBase):
    """
    Step 2 of the three-step workflow discovery protocol.

    Lists workflows available for a specific action type, filtered
    by the current signal context. Returns summary info (no full schema).

    Authority: DD-WORKFLOW-016, DD-HAPI-017
    Business Requirement: BR-HAPI-017-001

    Endpoint: GET /api/v1/workflows/actions/{action_type}
    """

    def __init__(
        self,
        data_storage_url: Optional[str] = None,
        remediation_id: Optional[str] = None,
        severity: str = "",
        component: str = "",
        environment: str = "",
        priority: str = "",
        custom_labels: Optional[Dict[str, List[str]]] = None,
        detected_labels: Optional[DetectedLabels] = None,
        http_session: Optional[Any] = None,
    ):
        super().__init__(
            name="list_workflows",
            description=(
                "List workflows available for a specific action type. "
                "Returns workflow summaries including name, description, version, "
                "success rate, and execution count. "
                "Use this AFTER selecting an action type from list_available_actions."
            ),
            parameters={
                "action_type": ToolParameter(
                    type="string",
                    description=(
                        "The action type to list workflows for (e.g., 'ScaleReplicas', 'RestartPod'). "
                        "Must be one of the action types returned by list_available_actions."
                    ),
                    required=True,
                ),
                "offset": ToolParameter(
                    type="integer",
                    description="Pagination offset (default: 0)",
                    required=False,
                ),
                "limit": ToolParameter(
                    type="integer",
                    description="Pagination limit (default: 10, max: 100)",
                    required=False,
                ),
            },
            additional_instructions=(
                "IMPORTANT: You MUST review ALL pages of workflows before making a selection. "
                "If hasMore is true, call this tool again with increased offset until all workflows "
                "are reviewed. Compare workflows by success rate, execution count, and description "
                "to select the most appropriate one. Do NOT pick the first workflow without "
                "reviewing all available options."
            ),
            data_storage_url=data_storage_url,
            remediation_id=remediation_id,
            severity=severity,
            component=component,
            environment=environment,
            priority=priority,
            custom_labels=custom_labels,
            detected_labels=detected_labels,
            http_session=http_session,
        )

    def _invoke(
        self, params: Dict, user_approved: bool = False
    ) -> StructuredToolResult:
        try:
            action_type = params.get("action_type", "")
            if not action_type:
                return StructuredToolResult(
                    status=StructuredToolResultStatus.ERROR,
                    error="action_type is required. Use list_available_actions first to discover available action types.",
                    params=params,
                )

            offset = params.get("offset", 0)
            limit = params.get("limit", 10)

            url = f"{self._data_storage_url}/api/v1/workflows/actions/{action_type}"
            extra_params = {"offset": offset, "limit": limit}

            logger.info(
                f"🔍 BR-HAPI-017-001 Step 2: Listing workflows — "
                f"action_type={action_type}, offset={offset}, limit={limit}"
            )

            data = self._do_get(url, extra_params)

            logger.info(
                f"✅ Step 2 complete: {len(data.get('workflows', []))} workflows for {action_type}, "
                f"total={data.get('pagination', {}).get('totalCount', '?')}"
            )

            return StructuredToolResult(
                status=StructuredToolResultStatus.SUCCESS,
                data=json.dumps(data, indent=2),
                params=params,
            )
        except ResourceNotFoundError as e:
            logger.warning(f"⚠️ Step 2: action type not found — {e}")
            return StructuredToolResult(
                status=StructuredToolResultStatus.ERROR,
                error=f"Action type '{action_type}' not found or has no matching workflows: {e}",
                params=params,
            )
        except Exception as e:
            logger.error(f"❌ Step 2 failed: {e}")
            return StructuredToolResult(
                status=StructuredToolResultStatus.ERROR,
                error=str(e),
                params=params,
            )

    def get_parameterized_one_liner(self, params: Dict) -> str:
        action_type = params.get("action_type", "?")
        return f"List workflows for action type '{action_type}'"


# ========================================
# STEP 3: GetWorkflowTool
# ========================================


class GetWorkflowTool(_DiscoveryToolBase):
    """
    Step 3 of the three-step workflow discovery protocol.

    Retrieves full workflow details including parameter schema for a
    specific workflow ID. Includes a security gate via context filters.

    Authority: DD-WORKFLOW-016, DD-HAPI-017
    Business Requirement: BR-HAPI-017-001

    Endpoint: GET /api/v1/workflows/{workflow_id}
    """

    def __init__(
        self,
        data_storage_url: Optional[str] = None,
        remediation_id: Optional[str] = None,
        severity: str = "",
        component: str = "",
        environment: str = "",
        priority: str = "",
        custom_labels: Optional[Dict[str, List[str]]] = None,
        detected_labels: Optional[DetectedLabels] = None,
        http_session: Optional[Any] = None,
    ):
        super().__init__(
            name="get_workflow",
            description=(
                "Get full details of a specific workflow by ID, including its parameter schema. "
                "Returns the complete workflow definition needed for execution. "
                "Use this AFTER selecting a workflow from list_workflows to get its parameters."
            ),
            parameters={
                "workflow_id": ToolParameter(
                    type="string",
                    description=(
                        "The UUID of the workflow to retrieve. "
                        "Must be a workflow_id from the list_workflows results."
                    ),
                    required=True,
                ),
            },
            additional_instructions=(
                "IMPORTANT: The workflow_id MUST come from the list_workflows results. "
                "If this tool returns a 'not found' error, the workflow may not match your "
                "current signal context — go back to list_workflows and select a different workflow. "
                "Use the returned parameter schema to fill in the required workflow parameters."
            ),
            data_storage_url=data_storage_url,
            remediation_id=remediation_id,
            severity=severity,
            component=component,
            environment=environment,
            priority=priority,
            custom_labels=custom_labels,
            detected_labels=detected_labels,
            http_session=http_session,
        )

    def _invoke(
        self, params: Dict, user_approved: bool = False
    ) -> StructuredToolResult:
        try:
            workflow_id = params.get("workflow_id", "")
            if not workflow_id:
                return StructuredToolResult(
                    status=StructuredToolResultStatus.ERROR,
                    error="workflow_id is required. Use list_workflows first to discover available workflows.",
                    params=params,
                )

            url = f"{self._data_storage_url}/api/v1/workflows/{workflow_id}"

            logger.info(
                f"🔍 BR-HAPI-017-001 Step 3: Getting workflow — "
                f"workflow_id={workflow_id}"
            )

            data = self._do_get(url)

            logger.info(
                f"✅ Step 3 complete: Retrieved workflow {data.get('workflow_name', workflow_id)}"
            )

            return StructuredToolResult(
                status=StructuredToolResultStatus.SUCCESS,
                data=json.dumps(data, indent=2),
                params=params,
            )
        except ResourceNotFoundError as e:
            logger.warning(
                f"⚠️ Step 3: Workflow not found (security gate) — {e}"
            )
            return StructuredToolResult(
                status=StructuredToolResultStatus.ERROR,
                error=f"Workflow '{workflow_id}' not found or does not match signal context: {e}",
                params=params,
            )
        except Exception as e:
            logger.error(f"❌ Step 3 failed: {e}")
            return StructuredToolResult(
                status=StructuredToolResultStatus.ERROR,
                error=str(e),
                params=params,
            )

    def get_parameterized_one_liner(self, params: Dict) -> str:
        workflow_id = params.get("workflow_id", "?")
        return f"Get workflow details for '{workflow_id}'"


# ========================================
# WORKFLOW DISCOVERY TOOLSET
# ========================================


class WorkflowDiscoveryToolset(Toolset):
    """
    Toolset providing the three-step workflow discovery protocol.

    Authority: DD-WORKFLOW-016, DD-HAPI-017
    Business Requirement: BR-HAPI-017-001

    Replaces: WorkflowCatalogToolset (search_workflow_catalog)

    Tools:
      1. list_available_actions — Discover action types
      2. list_workflows — List workflows for an action type
      3. get_workflow — Get full workflow with parameter schema

    Signal context filters (severity, component, environment, priority)
    are set once at toolset creation time and propagated to all tools.
    The LLM does NOT provide these — they come from the signal context.
    """

    def __init__(
        self,
        enabled: bool = True,
        remediation_id: Optional[str] = None,
        severity: str = "",
        component: str = "",
        environment: str = "",
        priority: str = "",
        custom_labels: Optional[Dict[str, List[str]]] = None,
        detected_labels: Optional[DetectedLabels] = None,
    ):
        """
        Initialize the three-step discovery toolset.

        Args:
            enabled: Whether the toolset is enabled
            remediation_id: Audit correlation ID (BR-HAPI-017-005)
            severity: Signal severity (critical/high/medium/low)
            component: K8s resource kind (pod/deployment/node/etc.)
            environment: Namespace-derived environment (production/staging/etc.)
            priority: Severity-mapped priority (P0/P1/P2/P3)
            custom_labels: Custom labels for filtering (DD-HAPI-001)
            detected_labels: Auto-detected infrastructure labels (DD-HAPI-017, DD-WORKFLOW-001 v2.1)
        """
        # Shared constructor kwargs for all three tools
        shared_kwargs = dict(
            remediation_id=remediation_id,
            severity=severity,
            component=component,
            environment=environment,
            priority=priority,
            custom_labels=custom_labels,
            detected_labels=detected_labels,
        )

        super().__init__(
            name="workflow/discovery",
            description=(
                "Three-step workflow discovery protocol for finding and selecting "
                "remediation workflows (DD-WORKFLOW-016, DD-HAPI-017)"
            ),
            enabled=enabled,
            status=ToolsetStatusEnum.ENABLED,
            tools=[
                ListAvailableActionsTool(**shared_kwargs),
                ListWorkflowsTool(**shared_kwargs),
                GetWorkflowTool(**shared_kwargs),
            ],
            docs_url=(
                "https://github.com/jordigilh/kubernaut/blob/main/docs/architecture/decisions/"
                "DD-WORKFLOW-016-action-type-workflow-indexing.md"
            ),
            llm_instructions=(
                "Use this toolset to find remediation workflows. Follow the THREE-STEP protocol:\n"
                "1. Call list_available_actions to see what action types are available\n"
                "2. Call list_workflows with your chosen action_type to see specific workflows\n"
                "3. Call get_workflow with the selected workflow_id to get its parameter schema\n\n"
                "IMPORTANT: Review ALL results at each step before proceeding. "
                "If list_workflows returns hasMore=true, call it again with increased offset "
                "to review ALL workflows before selecting one."
            ),
            experimental=False,
            is_default=True,
        )

    def get_example_config(self) -> Dict[str, Any]:
        return {
            "workflow/discovery": {
                "enabled": True,
                "description": "Three-step workflow discovery protocol (DD-WORKFLOW-016)",
            }
        }
