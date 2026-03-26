#
# Copyright 2025 Jordi Gil.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

"""
Incident Analysis LLM Integration

Business Requirements:
- BR-HAPI-002 (Incident Analysis)
- BR-HAPI-197 (needs_human_review field)
- BR-HAPI-211 (LLM Input Sanitization)
- BR-HAPI-212 (Mock LLM Mode)
- BR-HAPI-250 (Workflow Catalog Toolset)
- BR-AUDIT-005 (Audit Trail)

Design Decisions:
- DD-HAPI-002 v1.2 (LLM Self-Correction Loop)
- DD-HOLMESGPT-014 (MinimalDAL Stateless Architecture)
- DD-WORKFLOW-001 v1.7 (DetectedLabels validation)
- ADR-038 (Async Buffered Audit Ingestion)

This module contains the core LLM integration logic for incident analysis,
including HolmesGPT SDK integration, self-correction loop, and audit trail.
"""

import asyncio
import json
import os
import logging
from typing import Dict, Any, Optional, List

from src.models.config_models import AppConfig
from datetime import datetime, timezone
from fastapi import HTTPException, status

# HolmesGPT SDK imports
from holmes.config import Config
from holmes.core.models import InvestigateRequest
from holmes.core.investigation import investigate_issues

# Audit imports (BR-AUDIT-005, ADR-038, DD-AUDIT-002)
from src.audit import (
    get_audit_store,
    create_validation_attempt_event,
)

from .constants import MAX_VALIDATION_ATTEMPTS
from .prompt_builder import (
    create_incident_investigation_prompt,
    build_validation_error_feedback,
    build_resource_context_mismatch_feedback,
)
from .result_parser import parse_and_validate_investigation_result, _parse_affected_resource
from .enrichment_service import EnrichmentService, EnrichmentFailure, EnrichmentResult
from src.extensions.investigation_helpers import (
    audit_llm_request,
    audit_llm_response_and_tools,
    handle_validation_exhaustion,
)
# ADR-056: DetectedLabels import removed -- no longer extracted from request

logger = logging.getLogger(__name__)


def _build_enrichment_context(enrichment_result: Optional["EnrichmentResult"]) -> str:
    """Build an enrichment context section for the Phase 3 prompt.

    #529 BR-HAPI-265: Provides the LLM with Phase 2 enrichment results
    (root owner, detected labels, remediation history) so it can make
    informed workflow selections.
    """
    if enrichment_result is None:
        return ""

    parts = ["\n\n## Enrichment Context (Phase 2 — Auto-Detected)\n"]

    ro = enrichment_result.root_owner
    if ro:
        ns = ro.get("namespace", "")
        ro_desc = f"{ro.get('kind', '?')}/{ro.get('name', '?')}"
        if ns:
            ro_desc += f" in namespace '{ns}'"
        parts.append(f"**Root Owner**: {ro_desc}\n")

    labels = enrichment_result.detected_labels
    if labels:
        label_items = {k: v for k, v in labels.items() if k != "failedDetections"}
        if label_items:
            parts.append("**Detected Infrastructure Labels**:")
            parts.append(f"```json\n{json.dumps(label_items, indent=2)}\n```\n")

    history = enrichment_result.remediation_history
    if history:
        entries = history.get("entries", []) if isinstance(history, dict) else []
        if entries:
            parts.append(f"**Remediation History**: {len(entries)} past remediation(s) for this resource.\n")

    return "\n".join(parts) if len(parts) > 1 else ""


def _extract_balanced_json(text: str, start: int) -> Optional[str]:
    """Extract a balanced JSON object starting at position start.

    Uses brace counting with string-literal awareness to handle nested
    objects like ``{"affectedResource": {"kind": "Deployment"}}``.
    """
    if start >= len(text) or text[start] != '{':
        return None
    depth = 0
    in_string = False
    escape_next = False
    for i in range(start, len(text)):
        ch = text[i]
        if escape_next:
            escape_next = False
            continue
        if ch == '\\' and in_string:
            escape_next = True
            continue
        if ch == '"':
            in_string = not in_string
            continue
        if in_string:
            continue
        if ch == '{':
            depth += 1
        elif ch == '}':
            depth -= 1
            if depth == 0:
                return text[start:i + 1]
    return None


def _extract_phase1_json(analysis_text: str) -> Dict[str, Any]:
    """Extract JSON from Phase 1 analysis text to get affectedResource.

    Handles plain JSON, markdown code blocks, and section-header format
    (including nested JSON objects like affectedResource).
    Returns empty dict on failure.
    """
    import re as _re
    if not analysis_text:
        return {}
    try:
        return json.loads(analysis_text)
    except (json.JSONDecodeError, TypeError):
        pass
    match = _re.search(r'```json\s*(\{.*\})\s*```', analysis_text, _re.DOTALL)
    if match:
        try:
            return json.loads(match.group(1))
        except json.JSONDecodeError:
            pass
    rca_header = _re.search(r'# root_cause_analysis\s*\n\s*', analysis_text)
    if rca_header:
        brace_start = analysis_text.find('{', rca_header.end())
        if brace_start != -1:
            balanced = _extract_balanced_json(analysis_text, brace_start)
            if balanced:
                try:
                    return {"root_cause_analysis": json.loads(balanced)}
                except json.JSONDecodeError:
                    pass
    return {}


# ========================================
# BR-496 v2: HAPI-Owned Target Resource Identity
# ========================================

def _inject_target_resource(
    result: Dict[str, Any],
    session_state: Dict[str, Any],
    remediation_id: str,
    enrichment_result: Optional["EnrichmentResult"] = None,
) -> None:
    """Inject TARGET_RESOURCE_* and affectedResource from K8s-verified root_owner.

    #529: Prefer EnrichmentResult.root_owner (Phase 2) over session_state.
    Falls back to session_state["root_owner"] for backward compatibility with
    flows that haven't migrated to the 3-phase architecture yet.

    #524: Conditional injection — only inject TARGET_RESOURCE_* parameters
    that the workflow schema actually declares. Namespace is also skipped
    when resource_scope is 'cluster'.
    """
    if enrichment_result is not None and enrichment_result.root_owner is not None:
        root_owner = enrichment_result.root_owner
    else:
        root_owner = session_state.get("root_owner")

    if root_owner is None:
        logger.warning({
            "event": "target_resource_injection_failed",
            "reason": "root_owner_missing",
            "remediation_id": remediation_id,
        })
        result["needs_human_review"] = True
        result["human_review_reason"] = "rca_incomplete"
        return

    kind = root_owner.get("kind", "")
    name = root_owner.get("name", "")
    if not kind or not name:
        logger.warning({
            "event": "target_resource_injection_failed",
            "reason": "root_owner_malformed",
            "root_owner": root_owner,
            "remediation_id": remediation_id,
        })
        result["needs_human_review"] = True
        result["human_review_reason"] = "rca_incomplete"
        return

    ns = root_owner.get("namespace", "")
    resource_scope = session_state.get("resource_scope", "namespaced")

    affected_resource: Dict[str, str] = {"kind": kind, "name": name}
    if ns:
        affected_resource["namespace"] = ns

    rca = result.get("root_cause_analysis")
    if rca is None:
        rca = {}
        result["root_cause_analysis"] = rca
    rca["affectedResource"] = affected_resource

    logger.info({
        "event": "target_resource_injected",
        "root_owner": root_owner,
        "resource_scope": resource_scope,
        "remediation_id": remediation_id,
        "has_workflow": result.get("selected_workflow") is not None,
    })

    selected_wf = result.get("selected_workflow")
    if selected_wf is not None:
        params = selected_wf.get("parameters")
        if params is None:
            params = {}
            selected_wf["parameters"] = params

        # #524: Only inject parameters declared in the workflow schema.
        declared = _get_declared_param_names(session_state)

        if declared is None or "TARGET_RESOURCE_NAME" in declared:
            params["TARGET_RESOURCE_NAME"] = name
        if declared is None or "TARGET_RESOURCE_KIND" in declared:
            params["TARGET_RESOURCE_KIND"] = kind
        if (
            ns
            and resource_scope != "cluster"
            and (declared is None or "TARGET_RESOURCE_NAMESPACE" in declared)
        ):
            params["TARGET_RESOURCE_NAMESPACE"] = ns


def _get_declared_param_names(session_state: Dict[str, Any]) -> Optional[set]:
    """Extract declared parameter names from workflow_schema in session_state.

    Returns None when no schema key is present (backward compat: inject all).
    Returns empty set when schema is an empty list (workflow declares no params).
    """
    schema = session_state.get("workflow_schema")
    if schema is None:
        return None
    return {p.get("name") for p in schema if p.get("name")}


# #524: Action types that target cluster-scoped resources (Nodes, PVs, etc.)
_NODE_SCOPED_ACTION_TYPES = frozenset({
    "RemoveTaint",
    "DrainNode",
    "CordonNode",
    "UncordonNode",
    "RebootNode",
    "CleanupNode",
})


def _check_scope_mismatch(
    result: Dict[str, Any],
    session_state: Dict[str, Any],
) -> Optional[str]:
    """Detect mismatch between workflow target scope and resource context tool.

    #524: If the LLM selected a node-scoped workflow but used the namespaced
    tool (resource_scope='namespaced'), the root_owner is the workload's
    Deployment — not the Node. Returns a nudge message for the self-correction
    loop; None if no mismatch.
    """
    resource_scope = session_state.get("resource_scope")
    if resource_scope is None:
        return None

    selected_wf = result.get("selected_workflow")
    if selected_wf is None:
        return None

    action_type = selected_wf.get("action_type", "")
    if action_type in _NODE_SCOPED_ACTION_TYPES and resource_scope == "namespaced":
        return (
            f"You selected a node-scoped workflow (action_type='{action_type}') but "
            f"used the namespaced resource context tool. The root_owner may be a "
            f"Deployment instead of the Node. Please call "
            f"`get_cluster_resource_context` with the Node's kind and name, then "
            f"re-select the workflow."
        )

    return None


def _check_resource_context_mismatch(
    result: Dict[str, Any],
    session_state: Dict[str, Any],
    incident_id: str,
) -> Optional[str]:
    """Return correction feedback if the LLM's affectedResource doesn't match
    the last get_resource_context target, or None if they match.

    Issue #516: The LLM may call get_resource_context for one resource during
    early investigation but later identify a different resource as the RCA
    target.  Since detected_labels are computed by get_resource_context, stale
    labels from the wrong resource can lead to incorrect workflow selection.

    Skipped when no workflow was selected (no remediation to validate).
    """
    if result.get("selected_workflow") is None:
        return None

    rca = result.get("root_cause_analysis")
    if not isinstance(rca, dict):
        return None

    affected = rca.get("affectedResource")
    if not isinstance(affected, dict):
        return None

    last_target = session_state.get("last_resource_context_target")

    if last_target is None:
        logger.warning({
            "event": "resource_context_never_called",
            "incident_id": incident_id,
            "affected_resource": affected,
        })
        return build_resource_context_mismatch_feedback(affected, None)

    if (
        last_target.get("kind") != affected.get("kind")
        or last_target.get("name") != affected.get("name")
        or last_target.get("namespace", "") != affected.get("namespace", "")
    ):
        logger.warning({
            "event": "resource_context_target_mismatch",
            "incident_id": incident_id,
            "affected_resource": affected,
            "last_resource_context_target": last_target,
        })
        return build_resource_context_mismatch_feedback(affected, last_target)

    return None


# ========================================
# MINIMAL DAL (DD-HOLMESGPT-014)
# ========================================

class MinimalDAL:
    """
    Minimal DAL for HolmesGPT SDK integration (no Robusta Platform)

    Architecture Decision (DD-HOLMESGPT-014):
    Kubernaut does NOT integrate with Robusta Platform.

    Kubernaut Provides Equivalent Features Via:
    - Workflow catalog → PostgreSQL with Data Storage Service (not Robusta Platform)
    - Historical data → Context API (not Supabase)
    - Custom investigation logic → Rego policies in RemediationExecution Controller
    - LLM credentials → Kubernetes Secrets (not database)
    - Remediation state → CRDs (RemediationRequest, AIAnalysis, RemediationExecution)

    Result: No Robusta Platform database integration needed.

    This MinimalDAL satisfies HolmesGPT SDK's DAL interface requirements
    without connecting to any Robusta Platform database.

    Note: We still install supabase/postgrest dependencies (~50MB) because
    the SDK requires them, but this class ensures they're never used at runtime.

    See: docs/decisions/DD-HOLMESGPT-014-MinimalDAL-Stateless-Architecture.md
    """
    def __init__(self, cluster_name: str = "unknown"):
        self.cluster_name = cluster_name
        self.enabled = True  # Always enabled for Kubernaut (no Robusta Platform toggle)

    def get_issues(self, *args, **kwargs):
        """Return empty list - no historical issues from Robusta Platform"""
        return []

    def get_issue(self, *args, **kwargs):
        """Return None - no issue data from Robusta Platform"""
        return None

    def get_issue_data(self, *args, **kwargs):
        """Return None - no issue data from Robusta Platform"""
        return None

    def get_resource_instructions(self, *args, **kwargs):
        """Return None - no resource-specific instructions from Robusta Platform"""
        return None

    def get_global_instructions_for_account(self, *args, **kwargs):
        """Return None - no global account instructions from Robusta Platform"""
        return None

    def get_account_id(self, *args, **kwargs):
        """Return None - no Robusta Platform account"""
        return None

    def get_cluster_name(self, *args, **kwargs):
        """Return cluster name from initialization"""
        return self.cluster_name


def create_data_storage_client(app_config: Optional[AppConfig]):
    """
    Create Data Storage client for workflow validation.

    DD-HAPI-002 v1.2: Client used to validate workflow existence, container image, and parameters.

    Returns:
        WorkflowCatalogAPIApi or None if configuration is missing
    """
    try:
        from datastorage.api.workflow_catalog_api_api import WorkflowCatalogAPIApi
        from datastorage.api_client import ApiClient
        from datastorage.configuration import Configuration

        # Get Data Storage URL from config or environment
        ds_url = None
        if app_config:
            ds_url = app_config.get("data_storage_url") or app_config.get("DATA_STORAGE_URL")
        if not ds_url:
            ds_url = os.getenv("DATA_STORAGE_URL", "http://data-storage:8080")

        # DD-AUTH-014: Use ServiceAccount authentication
        # Performance Fix: Use singleton pool manager to reuse HTTP connections
        from datastorage_pool_manager import get_shared_datastorage_pool_manager
        auth_pool = get_shared_datastorage_pool_manager()
        configuration = Configuration(host=ds_url)
        api_client = ApiClient(configuration)
        api_client.rest_client.pool_manager = auth_pool  # Inject ServiceAccount token
        return WorkflowCatalogAPIApi(api_client)
    except Exception as e:
        logger.warning({
            "event": "data_storage_client_creation_failed",
            "error": str(e),
            "message": "Workflow validation will be skipped"
        })
        return None


async def analyze_incident(
    request_data: Dict[str, Any],
    mcp_config: Optional[Dict[str, Any]] = None,
    app_config: Optional[AppConfig] = None,
    metrics=None  # Injectable HAMetrics instance (Go pattern)
) -> Dict[str, Any]:
    """
    Core incident analysis logic with LLM self-correction loop.

    Business Requirements:
    - BR-HAPI-002 (Incident analysis)
    - BR-HAPI-011 (Investigation metrics)
    - BR-HAPI-197 (needs_human_review field)
    - BR-HAPI-211 (LLM Input Sanitization)
    - BR-HAPI-212 (Mock LLM Mode)
    - BR-HAPI-250 (Workflow Catalog Toolset)
    - BR-HAPI-301 (LLM observability metrics)

    Design Decision: DD-HAPI-002 v1.2 (Workflow Response Validation)

    Three-Phase Architecture (#529):
      Phase 1: LLM provides RCA + affectedResource
      Phase 2: EnrichmentService resolves K8s owner chain, detects labels, fetches history
      Phase 3: LLM selects workflow using enrichment context + Phase 1 analysis

    Self-Correction Loop (within Phase 3):
    1. Validate workflow response (existence, image, parameters)
    2. If invalid, feed errors back to LLM for self-correction
    3. Retry up to MAX_VALIDATION_ATTEMPTS times
    4. If all attempts fail, set needs_human_review=True
    
    Args:
        request_data: Incident request data dict
        mcp_config: Optional MCP configuration
        app_config: Optional application configuration
        metrics: Optional HAMetrics instance (injected by caller, uses global if None)
    """
    import time
    
    # Start timing for BR-HAPI-011 (Investigation metrics)
    start_time = time.time()
    
    incident_id = request_data.get("incident_id", "unknown")

    logger.info({
        "event": "incident_analysis_started",
        "incident_id": incident_id,
        "signal_name": request_data.get("signal_name")
    })

    # BR-AUDIT-005: Initialize audit store
    # Per ADR-032 §1: Audit is MANDATORY for ALL LLM interactions
    audit_store = get_audit_store()
    remediation_id = request_data.get("remediation_id", "")

    # Use HolmesGPT SDK for AI-powered analysis (calls standalone Mock LLM in E2E)
    # ADR-055: Context enrichment is post-RCA via get_namespaced_resource_context / get_cluster_resource_context.
    try:
        # BR-HAPI-211: Sanitize prompt BEFORE sending to LLM to prevent credential leakage
        from src.sanitization import sanitize_for_llm
        from src.extensions.incident.prompt_builder import create_phase3_workflow_prompt, PHASE3_SECTIONS
        base_prompt = sanitize_for_llm(create_incident_investigation_prompt(request_data))
        phase3_base_prompt = sanitize_for_llm(create_phase3_workflow_prompt(request_data))

        # Create minimal DAL
        dal = MinimalDAL(cluster_name=request_data.get("cluster_name"))

        # Create HolmesGPT config with workflow catalog toolset (BR-HAPI-250)
        # Get formatted model name for litellm (supports Ollama, OpenAI, Claude, Vertex AI)
        from src.extensions.llm_config import (
            get_model_config_for_sdk,
            prepare_toolsets_config_for_sdk,
            register_workflow_discovery_toolset,
            register_resource_context_toolset,
        )

        try:
            model_name, provider = get_model_config_for_sdk(app_config)
        except ValueError as e:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=str(e)
            )

        # Prepare toolsets configuration (BR-HAPI-002: Enable toolsets by default, BR-HAPI-250: Workflow catalog)
        toolsets_config = prepare_toolsets_config_for_sdk(app_config)

        # Create HolmesGPT SDK Config
        # NOTE: api_key is obtained from OPENAI_API_KEY environment variable via SDK's model registry
        # Do NOT pass api_key to Config() - it's not a valid field and will cause Pydantic validation error
        config = Config(
            model=model_name,
            api_base=os.getenv("LLM_ENDPOINT"),
            toolsets=toolsets_config,
            mcp_servers=app_config.get("mcp_servers", {}) if app_config else {},
        )

        # BR-HAPI-250: Register workflow catalog toolset programmatically
        # BR-AUDIT-001: Pass remediation_id for audit trail correlation (DD-WORKFLOW-002 v2.2)
        # remediation_id is MANDATORY per DD-WORKFLOW-002 v2.2 - used for CORRELATION ONLY
        remediation_id = request_data.get("remediation_id")
        if not remediation_id:
            logger.warning({
                "event": "missing_remediation_id",
                "incident_id": incident_id,
                "message": "remediation_id not provided - audit trail will be incomplete"
            })

        # DD-HAPI-001: Extract custom_labels from enrichment_results for auto-append
        # Custom labels are passed to WorkflowCatalogToolset and auto-appended to all MCP calls
        # The LLM does NOT see or provide these - they are operational metadata
        enrichment_results = request_data.get("enrichment_results", {}) or {}
        if hasattr(enrichment_results, 'customLabels'):
            # Pydantic model - access attribute directly
            custom_labels = enrichment_results.customLabels
        elif isinstance(enrichment_results, dict):
            # Dict - access via key (camelCase from K8s)
            custom_labels = enrichment_results.get("customLabels")
        else:
            custom_labels = None

        if custom_labels:
            logger.info({
                "event": "custom_labels_extracted",
                "incident_id": incident_id,
                "subdomains": list(custom_labels.keys()),
                "message": f"DD-HAPI-001: {len(custom_labels)} custom label subdomains will be auto-appended to workflow search"
            })

        # ADR-056 v1.4: Create shared session_state for inter-tool communication.
        # Labels are detected by get_namespaced_resource_context / get_cluster_resource_context and read by workflow discovery.
        session_state: Dict[str, Any] = {}

        # DD-HAPI-017: Register three-step workflow discovery toolset
        config = register_workflow_discovery_toolset(
            config,
            app_config,
            remediation_id=remediation_id,
            custom_labels=custom_labels,
            detected_labels=None,  # ADR-056 v1.4: populated via session_state by get_namespaced_resource_context / get_cluster_resource_context
            severity=request_data.get("severity", ""),
            component=request_data.get("resource_kind", ""),
            environment=request_data.get("environment", ""),
            priority=request_data.get("priority", ""),
            session_state=session_state,
        )

        # ADR-055/ADR-056 v1.4: Register resource context toolset for post-RCA enrichment + label detection
        config = register_resource_context_toolset(config, app_config, session_state=session_state)

        # DD-HAPI-002 v1.2: Create Data Storage client for workflow validation
        data_storage_client = create_data_storage_client(app_config)

        # BR-AUDIT-005: audit_store and remediation_id already initialized at function start

        # ========================================
        # THREE-PHASE SELF-CORRECTION LOOP (#529, DD-HAPI-002 v1.4)
        # Phase 1: RCA + affectedResource
        # Phase 2: HAPI-driven EnrichmentService
        # Phase 3: Workflow selection with enrichment context
        # ========================================
        validation_errors_history: List[List[str]] = []
        validation_attempts_history: List[Dict[str, Any]] = []
        last_schema_hint: Optional[str] = None
        pending_mismatch_feedback: Optional[str] = None
        result = None
        workflow_id = None
        enrichment_result_obj: Optional[EnrichmentResult] = None
        phase1_top_level: Dict[str, Any] = {}

        for attempt in range(MAX_VALIDATION_ATTEMPTS):
            attempt_timestamp = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

            # ─── PHASE 1: RCA + affectedResource ───
            if enrichment_result_obj is None:
                investigation_prompt = base_prompt

                logger.info({
                    "event": "phase1_rca_started",
                    "incident_id": incident_id,
                    "attempt": attempt + 1,
                })

                investigation_request = InvestigateRequest(
                    source="kubernaut",
                    title=f"Incident analysis for {request_data.get('signal_name')}",
                    description=investigation_prompt,
                    subject={
                        "type": "incident",
                        "incident_id": incident_id,
                        "signal_name": request_data.get("signal_name")
                    },
                    context={
                        "incident_id": incident_id,
                        "issue_type": "incident",
                        "attempt": attempt + 1,
                        "phase": 1,
                    },
                    source_instance_id="holmesgpt-api"
                )

                audit_llm_request(audit_store, incident_id, remediation_id, config, investigation_prompt)

                phase1_result = await asyncio.to_thread(
                    investigate_issues,
                    investigate_request=investigation_request,
                    dal=dal,
                    config=config,
                )

                audit_llm_response_and_tools(audit_store, incident_id, remediation_id, phase1_result)

                rca_data: Dict[str, Any] = {}
                phase1_raw = phase1_result.analysis if phase1_result and phase1_result.analysis else ""
                phase1_json = _extract_phase1_json(phase1_raw)
                if not isinstance(phase1_json, dict):
                    phase1_json = {}

                if phase1_result and getattr(phase1_result, "sections", None):
                    rca_section = phase1_result.sections.get("root_cause_analysis")
                    if rca_section:
                        try:
                            rca_data = json.loads(rca_section)
                        except (json.JSONDecodeError, TypeError):
                            pass
                if not rca_data:
                    rca_data = phase1_json.get("root_cause_analysis", {})
                affected_resource = _parse_affected_resource(rca_data)

                # BR-HAPI-200: Capture top-level Phase 1 fields that the
                # parser must propagate into the final response (e.g.,
                # investigation_outcome, can_recover, confidence).
                phase1_top_level = {}
                _phase1_propagate_keys = ("investigation_outcome", "can_recover", "confidence")
                for _k in _phase1_propagate_keys:
                    if _k in phase1_json:
                        phase1_top_level[_k] = phase1_json[_k]
                # Also parse from section headers (# investigation_outcome\n"resolved")
                if phase1_raw:
                    import re as _re
                    for _field in _phase1_propagate_keys:
                        if _field not in phase1_top_level:
                            _m = _re.search(
                                rf'#\s+{_field}\s*\n\s*(.+)',
                                phase1_raw,
                            )
                            if _m:
                                try:
                                    phase1_top_level[_field] = json.loads(_m.group(1).strip())
                                except (json.JSONDecodeError, ValueError):
                                    pass

                if affected_resource is None:
                    logger.warning({
                        "event": "phase1_no_affected_resource",
                        "incident_id": incident_id,
                        "attempt": attempt + 1,
                    })
                    continue

                phase1_analysis = phase1_result.analysis if phase1_result else None

                # ─── PHASE 2: Enrichment (HAPI-driven) ───
                logger.info({
                    "event": "phase2_enrichment_started",
                    "incident_id": incident_id,
                    "affected_resource": affected_resource,
                })

                from src.clients.k8s_client import get_k8s_client
                from src.detection.labels import LabelDetector

                try:
                    k8s = get_k8s_client()
                except Exception:
                    k8s = None

                detector_fn = None
                if k8s:
                    _detector = LabelDetector(k8s)

                    async def _detect(root_owner, owner_chain):
                        namespace = root_owner.get("namespace", "")
                        kind = root_owner.get("kind", "")
                        name = root_owner.get("name", "")
                        k8s_context: Dict[str, Any] = {"namespace": namespace}

                        if kind == "Pod":
                            pod = await k8s._get_resource_metadata("Pod", name, namespace)
                            if pod is not None:
                                k8s_context["pod_details"] = {
                                    "name": name,
                                    "labels": pod.metadata.labels or {},
                                    "annotations": pod.metadata.annotations or {},
                                }

                        for entry in (owner_chain or []):
                            if entry.get("kind") == "Deployment":
                                deploy = await k8s._get_resource_metadata(
                                    "Deployment", entry["name"], entry.get("namespace", ""),
                                )
                                if deploy is not None:
                                    k8s_context["deployment_details"] = {
                                        "name": entry["name"],
                                        "labels": deploy.metadata.labels or {},
                                        "annotations": deploy.metadata.annotations or {},
                                    }
                                    if "pod_details" not in k8s_context:
                                        template = getattr(getattr(deploy, "spec", None), "template", None)
                                        if template is not None:
                                            meta = getattr(template, "metadata", None)
                                            if meta is not None:
                                                pod_labels = getattr(meta, "labels", None) or {}
                                                if pod_labels:
                                                    k8s_context["pod_details"] = {
                                                        "name": entry["name"],
                                                        "labels": pod_labels,
                                                        "annotations": getattr(meta, "annotations", None) or {},
                                                    }
                                break

                        ns_meta = await k8s.get_namespace_metadata(namespace)
                        if ns_meta is not None:
                            k8s_context["namespace_labels"] = ns_meta.get("labels", {})
                            k8s_context["namespace_annotations"] = ns_meta.get("annotations", {})

                        return await _detector.detect_labels(k8s_context, owner_chain)

                    detector_fn = _detect

                enrichment_svc = EnrichmentService(
                    k8s_client=k8s,
                    ds_client=data_storage_client,
                    label_detector=detector_fn,
                )
                try:
                    enrichment_result_obj = await enrichment_svc.enrich(affected_resource)
                except EnrichmentFailure as ef:
                    logger.error({
                        "event": "phase2_enrichment_failed",
                        "incident_id": incident_id,
                        "reason": ef.reason,
                        "detail": ef.detail,
                    })
                    result = {
                        "root_cause_analysis": rca_data,
                        "needs_human_review": True,
                        "human_review_reason": "rca_incomplete",
                    }
                    break

                # #529 BR-HAPI-265: Populate session_state with enrichment labels
                # so Phase 3 workflow discovery tools can surface them in cluster_context.
                if enrichment_result_obj.detected_labels:
                    session_state["detected_labels"] = enrichment_result_obj.detected_labels

                # Issue #535 / BR-HAPI-261: Store resolved root_owner in session_state
                # so workflow discovery tools use the correct component filter.
                if enrichment_result_obj.root_owner:
                    session_state["root_owner"] = enrichment_result_obj.root_owner

            # ─── PHASE 3: Workflow Selection ───
            # Issue #537 / BR-HAPI-263: Use focused Phase 3 prompt + custom
            # sections so the LLM knows it is in workflow selection mode and
            # produces output structured with the keys the HAPI parser expects.
            phase1_context = ""
            if phase1_analysis:
                phase1_context = f"\n\n## Phase 1 Root Cause Analysis\n{phase1_analysis}\n"

            enrichment_context = _build_enrichment_context(enrichment_result_obj)
            if pending_mismatch_feedback is not None:
                investigation_prompt = phase3_base_prompt + phase1_context + enrichment_context + pending_mismatch_feedback
                pending_mismatch_feedback = None
            elif validation_errors_history:
                investigation_prompt = phase3_base_prompt + phase1_context + enrichment_context + build_validation_error_feedback(
                    validation_errors_history[-1],
                    attempt,
                    schema_hint=last_schema_hint,
                )
            else:
                investigation_prompt = phase3_base_prompt + phase1_context + enrichment_context

            logger.info({
                "event": "phase3_workflow_selection_started",
                "incident_id": incident_id,
                "attempt": attempt + 1,
            })

            phase3_request = InvestigateRequest(
                source="kubernaut",
                title=f"Workflow selection for {request_data.get('signal_name')}",
                description=investigation_prompt,
                subject={
                    "type": "incident",
                    "incident_id": incident_id,
                    "signal_name": request_data.get("signal_name")
                },
                context={
                    "incident_id": incident_id,
                    "issue_type": "incident",
                    "attempt": attempt + 1,
                    "phase": 3,
                },
                sections=PHASE3_SECTIONS,
                source_instance_id="holmesgpt-api"
            )

            audit_llm_request(audit_store, incident_id, remediation_id, config, investigation_prompt)

            phase3_investigation_result = await asyncio.to_thread(
                investigate_issues,
                investigate_request=phase3_request,
                dal=dal,
                config=config,
            )

            audit_llm_response_and_tools(audit_store, incident_id, remediation_id, phase3_investigation_result)

            result, validation_result = parse_and_validate_investigation_result(
                phase3_investigation_result,
                request_data,
                data_storage_client=data_storage_client
            )

            # #529: Phase 3 only returns workflow selection — the parser produces
            # an empty/minimal root_cause_analysis.  Merge the Phase 1 RCA so the
            # final response contains summary/severity/contributingFactors.
            if rca_data:
                phase3_rca = result.get("root_cause_analysis", {})
                merged_rca = dict(rca_data)
                merged_rca.update({k: v for k, v in phase3_rca.items() if v})
                result["root_cause_analysis"] = merged_rca

            # BR-HAPI-200: Propagate top-level Phase 1 fields (investigation_outcome,
            # can_recover) that Phase 3 may also return.  Phase 3 values take precedence;
            # Phase 1 values fill in anything the Phase 3 parser didn't produce.
            for _key, _val in phase1_top_level.items():
                result.setdefault(_key, _val)

            workflow_id = result.get("selected_workflow", {}).get("workflow_id") if result.get("selected_workflow") else None
            is_valid = validation_result is None or validation_result.is_valid
            validation_errors = validation_result.errors if validation_result and not validation_result.is_valid else []

            if validation_result and validation_result.parameter_schema is not None:
                session_state["workflow_schema"] = validation_result.parameter_schema
            elif validation_result and validation_result.parameter_schema is None:
                session_state.pop("workflow_schema", None)

            if is_valid:
                scope_nudge = _check_scope_mismatch(result, session_state)
                if scope_nudge is not None:
                    is_valid = False
                    validation_errors = [scope_nudge]
                    logger.warning({
                        "event": "scope_mismatch_detected",
                        "incident_id": incident_id,
                        "attempt": attempt + 1,
                        "nudge": scope_nudge,
                    })

            audit_store.store_audit(create_validation_attempt_event(
                incident_id=incident_id,
                remediation_id=remediation_id,
                attempt=attempt + 1,
                max_attempts=MAX_VALIDATION_ATTEMPTS,
                is_valid=is_valid,
                errors=validation_errors,
                workflow_id=workflow_id
            ))

            validation_attempts_history.append({
                "attempt": attempt + 1,
                "workflow_id": workflow_id,
                "is_valid": is_valid,
                "errors": validation_errors,
                "timestamp": attempt_timestamp
            })

            if is_valid:
                # #529: Skip the legacy resource-context mismatch check when the
                # three-phase enrichment flow was used — Phase 2 EnrichmentService
                # already resolved and validated the affected resource.
                if enrichment_result_obj is None:
                    mismatch_feedback = _check_resource_context_mismatch(
                        result, session_state, incident_id
                    )
                    if mismatch_feedback is not None:
                        session_state.pop("detected_labels", None)
                        validation_errors_history.append(["resource_context_mismatch"])
                        pending_mismatch_feedback = mismatch_feedback
                        logger.warning({
                            "event": "resource_context_mismatch",
                            "incident_id": incident_id,
                            "attempt": attempt + 1,
                        })
                        continue

                logger.info({
                    "event": "workflow_validation_passed",
                    "incident_id": incident_id,
                    "attempt": attempt + 1,
                    "has_workflow": result.get("selected_workflow") is not None
                })
                break
            else:
                validation_errors_history.append(validation_errors)
                if validation_result and validation_result.schema_hint:
                    last_schema_hint = validation_result.schema_hint
                logger.warning({
                    "event": "workflow_validation_retry",
                    "incident_id": incident_id,
                    "attempt": attempt + 1,
                    "max_attempts": MAX_VALIDATION_ATTEMPTS,
                    "errors": validation_errors,
                })

        if result is None:
            logger.warning({
                "event": "phase1_exhaustion_no_result",
                "incident_id": incident_id,
                "max_attempts": MAX_VALIDATION_ATTEMPTS,
            })
            result = {
                "root_cause_analysis": {"summary": "Phase 1 failed to identify affected resource after all attempts"},
                "needs_human_review": True,
                "human_review_reason": "rca_incomplete",
                "selected_workflow": None,
            }

        handle_validation_exhaustion(
            result, validation_errors_history, MAX_VALIDATION_ATTEMPTS,
            audit_store, incident_id, remediation_id, workflow_id
        )

        if "validation_attempts_history" not in result or not result["validation_attempts_history"]:
            result["validation_attempts_history"] = validation_attempts_history

        # #529: Inject from EnrichmentResult (Phase 2) instead of session_state
        _inject_target_resource(result, session_state, remediation_id, enrichment_result=enrichment_result_obj)

        # ADR-056: Inject runtime-computed detected_labels into response
        from src.extensions.llm_config import inject_detected_labels
        inject_detected_labels(result, session_state)

        logger.info({
            "event": "incident_analysis_completed",
            "incident_id": incident_id,
            "has_workflow": result.get("selected_workflow") is not None,
            "warnings_count": len(result.get("warnings", [])),
            "needs_human_review": result.get("needs_human_review", False),
            "validation_attempts": len(validation_errors_history) + 1 if validation_errors_history else 1,
            "has_detected_labels": "detected_labels" in result,
        })
        
        # Record metrics (BR-HAPI-011: Investigation metrics)
        if metrics:
            inv_status = "needs_review" if result.get("needs_human_review", False) else "success"
            metrics.record_investigation_complete(start_time, inv_status)

        return result

    except Exception as e:
        logger.error({
            "event": "incident_analysis_failed",
            "incident_id": incident_id,
            "error": str(e)
        }, exc_info=True)
        
        # Record error metrics (BR-HAPI-011)
        if metrics:
            metrics.record_investigation_complete(start_time, "error")
        
        raise

