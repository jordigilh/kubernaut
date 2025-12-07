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
Recovery Analysis Endpoint

Business Requirements: BR-HAPI-001 to 050 (Recovery Analysis)

Provides AI-powered recovery strategy recommendations for failed remediation actions.
"""

import logging
import os
import json
import re
from typing import Dict, Any, List, Optional
from fastapi import APIRouter, HTTPException, status

from src.models.recovery_models import (
    RecoveryRequest, RecoveryResponse, RecoveryStrategy,
    PreviousExecution, OriginalRCA, SelectedWorkflowSummary, ExecutionFailure
)
from src.models.incident_models import DetectedLabels, EnrichmentResults
# NOTE: WorkflowCatalogToolset is registered via register_workflow_catalog_toolset()
# and used by the LLM during investigation - no direct import needed here

# Audit imports (BR-AUDIT-005, ADR-038, DD-AUDIT-002)
from src.audit import (
    BufferedAuditStore,
    AuditConfig,
    create_llm_request_event,
    create_llm_response_event,
    create_tool_call_event,
)

logger = logging.getLogger(__name__)

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# AUDIT STORE INITIALIZATION (BR-AUDIT-005, ADR-038)
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
_audit_store: Optional[BufferedAuditStore] = None


def get_audit_store() -> Optional[BufferedAuditStore]:
    """Get or initialize the audit store singleton (ADR-038)"""
    global _audit_store
    if _audit_store is None:
        data_storage_url = os.getenv("DATA_STORAGE_URL", "http://data-storage:8080")
        try:
            _audit_store = BufferedAuditStore(
                data_storage_url=data_storage_url,
                config=AuditConfig(buffer_size=10000, batch_size=50, flush_interval_seconds=5.0)
            )
            logger.info(f"BR-AUDIT-005: Initialized audit store - url={data_storage_url}")
        except Exception as e:
            logger.warning(f"BR-AUDIT-005: Failed to initialize audit store: {e}")
    return _audit_store

# HolmesGPT SDK imports
from holmes.config import Config
from holmes.core.models import InvestigateRequest, InvestigationResult
from holmes.core.investigation import investigate_issues

# Minimal DAL for HolmesGPT SDK integration (no Robusta Platform)
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
    def __init__(self, cluster_name=None):
        self.cluster = cluster_name
        self.cluster_name = cluster_name  # Backwards compatibility
        self.enabled = False  # Disable Robusta platform features
        logger.info(f"Using MinimalDAL (no Robusta Platform) for cluster={cluster_name}")

    def get_issue_data(self, issue_id):
        """
        Historical issue data (NOT USED)

        Kubernaut: Context API provides historical data via separate service
        """
        return None

    def get_resource_instructions(self, resource_type, issue_type):
        """
        Custom investigation runbooks (NOT USED)

        Kubernaut: Rego policies in RemediationExecution Controller provide custom logic

        Returns None to signal no custom runbooks (SDK will use defaults)
        """
        return None

    def get_global_instructions_for_account(self):
        """
        Account-level investigation guidelines (NOT USED)

        Kubernaut: RemediationExecution Controller manages investigation flow

        Returns None to signal no global instructions (SDK will use defaults)
        """
        return None


# ========================================
# FAILURE REASON GUIDANCE (DD-RECOVERY-003)
# ========================================

def _get_failure_reason_guidance(reason: str) -> str:
    """
    Provide reason-specific recovery guidance based on Kubernetes reason codes.

    These are canonical Kubernetes reason codes - the API contract between
    WorkflowExecution status and AIAnalysis recovery.

    Design Decision: DD-RECOVERY-003
    """
    guidance_map = {
        # Resource-related failures
        "OOMKilled": """
  - Container exceeded memory limits during remediation
  - Consider: Workflow with lower memory footprint, or scale resources first
  - Alternative: Gradual remediation instead of aggressive action
""",
        "InsufficientCPU": """
  - Not enough CPU available to execute remediation
  - Consider: Wait for resources, or request resource increase first
  - Alternative: Lower-priority workflow that uses less CPU
""",
        "InsufficientMemory": """
  - Not enough memory available in cluster
  - Consider: Evict lower-priority workloads first, or use smaller workflow
  - Alternative: Remediation that doesn't require additional memory
""",

        # Scheduling failures
        "FailedScheduling": """
  - Kubernetes scheduler couldn't place the remediation pod
  - Consider: Node affinity issues, resource constraints, or taints
  - Alternative: Workflow that can run on different nodes
""",
        "Unschedulable": """
  - Pod marked as unschedulable
  - Consider: Check node conditions, tolerations, and affinity rules
  - Alternative: Workflow that removes scheduling constraints
""",

        # Image-related failures
        "ImagePullBackOff": """
  - Could not pull workflow container image
  - Consider: Image doesn't exist, registry auth, network issues
  - Alternative: Workflow with different container image
""",
        "ErrImagePull": """
  - Failed to pull container image
  - Consider: Check image name, tag, and registry access
  - Alternative: Use cached image or different workflow
""",

        # Execution failures
        "DeadlineExceeded": """
  - Workflow execution exceeded time limit
  - Consider: Task taking longer than expected, or stuck
  - Alternative: Workflow with longer timeout, or faster approach
""",
        "BackoffLimitExceeded": """
  - Workflow exceeded retry attempts
  - Consider: Persistent failure, requires different approach
  - Alternative: Completely different remediation strategy
""",
        "Error": """
  - Generic execution error
  - Consider: Check logs for specific error details
  - Alternative: Based on error message analysis
""",

        # Permission failures
        "Unauthorized": """
  - Workflow lacks required permissions
  - Consider: RBAC configuration, service account permissions
  - Alternative: Workflow that doesn't require elevated permissions
""",
        "Forbidden": """
  - Action forbidden by security policy
  - Consider: PodSecurityPolicy, NetworkPolicy, or admission controller
  - Alternative: Workflow that complies with security policies
""",

        # Volume/Storage failures
        "FailedMount": """
  - Could not mount required volume
  - Consider: PVC issues, storage class problems, or capacity
  - Alternative: Workflow that doesn't require persistent storage
""",
        "FailedAttachVolume": """
  - Could not attach volume to node
  - Consider: Volume already attached elsewhere, or node issues
  - Alternative: Workflow that uses different storage approach
""",

        # Network failures
        "NetworkNotReady": """
  - Network not available for pod
  - Consider: CNI issues, network policy blocking
  - Alternative: Workflow that can work with limited network
""",

        # Node failures
        "NodeNotReady": """
  - Node became unavailable during execution
  - Consider: Node health issues, draining, or cordoning
  - Alternative: Workflow that can run on different nodes
""",
        "Evicted": """
  - Pod was evicted during execution
  - Consider: Resource pressure on node
  - Alternative: Workflow with resource requests/limits, or different node
""",
    }

    return guidance_map.get(reason, f"""
  - Kubernetes reason: `{reason}`
  - Investigate the specific failure mode
  - Search for workflows that handle this condition
""")


# ========================================
# CLUSTER CONTEXT SECTION (DD-RECOVERY-003)
# ========================================

def _build_cluster_context_section(detected_labels: Dict[str, Any]) -> str:
    """
    Convert DetectedLabels to natural language for LLM context.

    This helps the LLM understand the cluster environment and make
    appropriate workflow recommendations.

    Design Decision: DD-RECOVERY-003, DD-WORKFLOW-001 v2.1

    DD-WORKFLOW-001 v2.1: Honor failedDetections
    - Fields in failedDetections are EXCLUDED from cluster context
    - Prevents LLM from receiving misleading information about unknown cluster state

    Key Distinction (per SignalProcessing team):
    - "Resource doesn't exist" (pdbProtected=false, failedDetections=[]) = valid, use it
    - "Detection failed" (pdbProtected=false, failedDetections=["pdbProtected"]) = unknown, skip it
    """
    if not detected_labels:
        return "No special cluster characteristics detected."

    # DD-WORKFLOW-001 v2.1: Get fields where detection failed
    failed_fields = set(detected_labels.get('failedDetections', []))

    sections = []

    # GitOps - skip if gitOpsManaged detection failed
    if 'gitOpsManaged' not in failed_fields and detected_labels.get("gitOpsManaged"):
        tool = detected_labels.get("gitOpsTool", "unknown")
        sections.append(f"This namespace is managed by GitOps ({tool}). "
                       "DO NOT make direct changes - recommend GitOps-aware workflows.")

    # Protection - skip if respective detection failed
    if 'pdbProtected' not in failed_fields and detected_labels.get("pdbProtected"):
        sections.append("A PodDisruptionBudget protects this workload. "
                       "Workflows must respect PDB constraints.")

    if 'hpaEnabled' not in failed_fields and detected_labels.get("hpaEnabled"):
        sections.append("HorizontalPodAutoscaler is active. "
                       "Manual scaling may conflict with HPA - prefer HPA-aware workflows.")

    # Workload type - skip if respective detection failed
    if 'stateful' not in failed_fields and detected_labels.get("stateful"):
        sections.append("This is a STATEFUL workload (StatefulSet or has PVCs). "
                       "Use stateful-aware remediation workflows.")

    if 'helmManaged' not in failed_fields and detected_labels.get("helmManaged"):
        sections.append("This resource is managed by Helm. "
                       "Consider Helm-compatible workflows.")

    # Security - skip if respective detection failed
    if 'networkIsolated' not in failed_fields and detected_labels.get("networkIsolated"):
        sections.append("NetworkPolicy restricts traffic in this namespace. "
                       "Workflows may need network exceptions.")

    # DD-WORKFLOW-001 v2.2: podSecurityLevel REMOVED (PSP deprecated, PSS is namespace-level)

    if 'serviceMesh' not in failed_fields:
        mesh = detected_labels.get("serviceMesh", "")
        if mesh:
            sections.append(f"Service mesh ({mesh}) is present. "
                           "Consider service mesh-aware workflows.")

    return "\n".join(sections) if sections else "No special cluster characteristics detected."


def _build_mcp_filter_instructions(detected_labels: Dict[str, Any]) -> str:
    """
    Build MCP workflow search filter instructions based on DetectedLabels.

    Design Decision: DD-RECOVERY-003, DD-WORKFLOW-001 v2.1

    DD-WORKFLOW-001 v2.1: Honor failedDetections
    - Fields in failedDetections are EXCLUDED from filter instructions
    - Prevents LLM from filtering on unknown values (e.g., RBAC denied)

    Key Distinction (per SignalProcessing team):
    | Scenario    | pdbProtected | failedDetections     | Meaning                    |
    |-------------|--------------|----------------------|----------------------------|
    | PDB exists  | true         | []                   | ✅ Has PDB - use for filter |
    | No PDB      | false        | []                   | ✅ No PDB - use for filter  |
    | RBAC denied | false        | ["pdbProtected"]     | ⚠️ Unknown - skip filter    |
    """
    if not detected_labels:
        return ""

    import json

    # DD-WORKFLOW-001 v2.1: Get fields where detection failed
    failed_fields = set(detected_labels.get('failedDetections', []))

    # Build filters, excluding failed detections
    # Map from DetectedLabels field names to filter names
    field_mapping = {
        'gitOpsManaged': 'gitops_managed',
        'pdbProtected': 'pdb_protected',
        'stateful': 'stateful',
        'helmManaged': 'helm_managed',
        'gitOpsTool': 'gitops_tool',
        'serviceMesh': 'service_mesh',
        # DD-WORKFLOW-001 v2.2: podSecurityLevel REMOVED
    }

    filters = {}
    for label_field, filter_name in field_mapping.items():
        # Skip fields where detection failed
        if label_field in failed_fields:
            continue
        # Also skip gitOpsTool if gitOpsManaged detection failed
        if label_field == 'gitOpsTool' and 'gitOpsManaged' in failed_fields:
            continue

        value = detected_labels.get(label_field)
        if value is None:
            continue

        # Convert booleans to lowercase strings
        if isinstance(value, bool):
            filters[filter_name] = str(value).lower()
        elif value:  # Only include non-empty string values
            filters[filter_name] = value

    # Remove empty string values
    filters = {k: v for k, v in filters.items() if v}

    return f"""
### MCP Workflow Search Instructions

When calling the `search_workflow_catalog` MCP tool, include detected labels as filters:

```json
{{
  "query": "<signal_type> <severity>",
  "filters": {json.dumps(filters, indent=4)}
}}
```

The Data Storage service will use these filters to return only workflows that are compatible
with the detected cluster environment.

**IMPORTANT**: If `gitOpsManaged=true`, prioritize workflows with `gitops_aware=true` tag.
"""


router = APIRouter()


# ========================================
# RECOVERY INVESTIGATION PROMPT (DD-RECOVERY-003)
# ========================================

def _create_recovery_investigation_prompt(request_data: Dict[str, Any]) -> str:
    """
    Create investigation prompt for recovery analysis.

    Design Decision: DD-RECOVERY-003

    Key Differences from Incident Prompt:
    1. Adds "Previous Remediation Attempt" section at TOP
    2. Includes Kubernetes reason code with specific guidance
    3. Instructs LLM NOT to repeat failed workflow
    4. Expects signal type may have CHANGED
    """
    # Extract previous execution context
    previous = request_data.get("previous_execution", {})
    original_rca = previous.get("original_rca", {})
    selected_workflow = previous.get("selected_workflow", {})
    failure = previous.get("failure", {})
    attempt_number = request_data.get("recovery_attempt_number", 1)

    # Get Kubernetes reason code
    failure_reason = failure.get("reason", "Unknown")

    # Extract standard fields
    signal_type = request_data.get("signal_type", "Unknown")
    severity = request_data.get("severity", "unknown")
    namespace = request_data.get("resource_namespace", "unknown")
    resource_kind = request_data.get("resource_kind", "unknown")
    resource_name = request_data.get("resource_name", "unknown")
    environment = request_data.get("environment", "unknown")
    priority = request_data.get("priority", "P2")
    risk_tolerance = request_data.get("risk_tolerance", "medium")
    business_category = request_data.get("business_category", "standard")
    error_message = request_data.get("error_message", "Unknown error")
    cluster_name = request_data.get("cluster_name", "unknown")
    signal_source = request_data.get("signal_source", "unknown")

    # Get detected labels for cluster context
    enrichment_results = request_data.get("enrichment_results", {})
    detected_labels = enrichment_results.get("detectedLabels", {}) if enrichment_results else {}

    # Build recovery context section (appears BEFORE standard sections)
    prompt = f"""# Recovery Analysis Request (Attempt {attempt_number})

## ⚠️ Previous Remediation Attempt - CRITICAL CONTEXT

**This is a RECOVERY attempt**. A previous remediation was tried and FAILED.
You must understand what was attempted and why it failed before recommending alternatives.

---

### What Was Originally Determined

**Original Root Cause Analysis (from initial investigation)**:
- **Summary**: {original_rca.get('summary', 'Unknown')}
- **Signal Type** (RCA determination): `{original_rca.get('signal_type', 'Unknown')}`
- **Severity**: {original_rca.get('severity', 'unknown')}
- **Contributing Factors**: {', '.join(original_rca.get('contributing_factors', ['None recorded']))}

**Workflow Selected Based on RCA**:
- **Workflow ID**: `{selected_workflow.get('workflow_id', 'Unknown')}`
- **Version**: {selected_workflow.get('version', 'Unknown')}
- **Container Image**: `{selected_workflow.get('container_image', 'Unknown')}`
- **Selection Rationale**: {selected_workflow.get('rationale', 'Not recorded')}
"""

    # Add parameters if present
    params = selected_workflow.get('parameters', {})
    if params:
        prompt += "\n**Parameters Used**:\n"
        for key, value in params.items():
            prompt += f"- `{key}`: `{value}`\n"

    # BR-HAPI-192: Extract WE-generated natural language summary
    natural_language_summary = previous.get('natural_language_summary', '')

    # Add failure details with Kubernetes reason code
    prompt += f"""
---

### What Failed During Execution

**Execution Failure Details**:
- **Failed Step**: Step {failure.get('failed_step_index', '?')} - `{failure.get('failed_step_name', 'Unknown')}`
- **Kubernetes Reason**: **`{failure_reason}`**
- **Error Message**: {failure.get('message', 'No message')}
- **Exit Code**: {failure.get('exit_code', 'N/A')}
- **Execution Duration**: {failure.get('execution_time', 'Unknown')} before failure
- **Failed At**: {failure.get('failed_at', 'Unknown')}
"""

    # BR-HAPI-192: Include WE-generated natural language summary if available
    if natural_language_summary:
        prompt += f"""
**Workflow Engine Summary** (LLM-friendly context from WE):
> {natural_language_summary}
"""

    prompt += f"""
**Failure Reason Interpretation** (`{failure_reason}`):
{_get_failure_reason_guidance(failure_reason)}

---

### Your Recovery Investigation Task

**CRITICAL INSTRUCTIONS**:

1. **DO NOT** select the same workflow (`{selected_workflow.get('workflow_id', 'Unknown')}`) with the same parameters
   - The previous attempt already failed with this approach
   - You must find an ALTERNATIVE solution

2. **INVESTIGATE** the CURRENT cluster state:
   - Start from the failure point, not the original signal
   - Check if the failed step partially executed and changed state
   - Determine if the resource is now in a different condition

3. **DETERMINE** if the signal type has CHANGED:
   - The workflow execution may have altered the cluster state
   - Example: OOMKilled → workflow tried to scale → now "InsufficientCPU"
   - Your workflow search should use the CURRENT signal type, not the original

4. **CONSIDER** alternative approaches based on failure reason:
   - `{failure_reason}` suggests specific recovery strategies (see guidance above)
   - Search for workflows that handle this specific failure mode
   - Consider less aggressive remediation if original was too aggressive

5. **SEARCH** for alternative workflows using:
   - Query: `"<CURRENT_SIGNAL_TYPE> <CURRENT_SEVERITY> recovery"`
   - Include the failure reason in your search rationale

---

"""

    # Add cluster context section if DetectedLabels are available
    if detected_labels:
        prompt += f"""## Cluster Environment Characteristics (AUTO-DETECTED)

The following characteristics were automatically detected for the target resource.
**YOU MUST include these as filters in your MCP workflow search request.**

{_build_cluster_context_section(detected_labels)}

{_build_mcp_filter_instructions(detected_labels)}

---

"""

    # Add current signal context
    prompt += f"""
## Current Signal Context

**Technical Details**:
- Signal Type: {signal_type}
- Severity: {severity}
- Resource: {namespace}/{resource_kind}/{resource_name}
- Error: {error_message}

## Business Context (FOR MCP WORKFLOW SEARCH)

- Environment: {environment}
- Priority: {priority}
- Business Category: {business_category}
- Risk Tolerance: {risk_tolerance}

## Your Investigation Workflow (Recovery Mode)

### Phase 1: Assess Current State
- Check the CURRENT state of the resource (may have changed due to failed workflow)
- Determine if the failure left the system in a degraded state
- Look for side effects from the partial execution

### Phase 2: Re-evaluate Root Cause
- The original RCA was: `{original_rca.get('signal_type', 'Unknown')}`
- Determine if the signal type has CHANGED after the failed workflow
- If changed, use the NEW signal type for workflow search

### Phase 3: Search for Alternative Workflow (MANDATORY)
**YOU MUST** call MCP `search_workflow_catalog` tool with:
- **Query**: `"<CURRENT_SIGNAL_TYPE> <CURRENT_SEVERITY> recovery"`
- **Constraint**: Do NOT select the previously failed workflow

### Phase 4: Return Recovery Recommendation
Provide structured JSON with alternative workflow and updated parameters.

**If MCP search succeeds**:
```json
{{
  "recovery_analysis": {{
    "previous_attempt_assessment": {{
      "failure_understood": true,
      "failure_reason_analysis": "Explanation of why previous attempt failed",
      "state_changed": true,
      "current_signal_type": "Current signal type after failure"
    }},
    "current_rca": {{
      "summary": "Updated RCA based on current state",
      "severity": "current severity",
      "signal_type": "current signal type",
      "contributing_factors": ["factor1", "factor2"]
    }}
  }},
  "selected_workflow": {{
    "workflow_id": "alternative-workflow-id",
    "version": "1.0.0",
    "confidence": 0.85,
    "rationale": "Why this alternative was selected and how it differs from failed attempt",
    "parameters": {{
      "PARAM_NAME": "value"
    }}
  }},
  "recovery_strategy": {{
    "approach": "description of recovery approach",
    "differs_from_previous": true,
    "why_different": "Explanation of why this approach is different"
  }}
}}
```

**If MCP search fails or returns no workflows**:
```json
{{
  "recovery_analysis": {{
    "previous_attempt_assessment": {{
      "failure_understood": true,
      "failure_reason_analysis": "Explanation of why previous attempt failed"
    }},
    "current_rca": {{
      "summary": "Root cause from investigation",
      "severity": "critical|high|medium|low",
      "signal_type": "current signal type",
      "contributing_factors": ["factor1", "factor2"]
    }}
  }},
  "selected_workflow": null,
  "rationale": "MCP search failed: [error details]. RCA completed but workflow selection unavailable."
}}
```
"""

    return prompt


def _get_holmes_config(
    app_config: Dict[str, Any] = None,
    remediation_id: Optional[str] = None,
    custom_labels: Optional[Dict[str, List[str]]] = None,
    detected_labels: Optional[Dict[str, Any]] = None,
    source_resource: Optional[Dict[str, str]] = None,
    owner_chain: Optional[List[Dict[str, str]]] = None
) -> Config:
    """
    Initialize HolmesGPT SDK Config from environment variables and app config

    Args:
        app_config: Application configuration dictionary
        remediation_id: Remediation request ID for audit correlation (DD-WORKFLOW-002 v2.2)
                       MANDATORY per DD-WORKFLOW-002 v2.2. This ID is for CORRELATION/AUDIT ONLY -
                       do NOT use for RCA analysis or workflow matching.
        custom_labels: Custom labels for auto-append to workflow search (DD-HAPI-001)
                      Format: map[string][]string (subdomain → list of values)
                      Example: {"constraint": ["cost-constrained"], "team": ["name=payments"]}
                      Auto-appended to all MCP workflow search calls - invisible to LLM.
        detected_labels: Auto-detected labels for workflow matching (DD-WORKFLOW-001 v1.7)
                        Format: {"gitOpsManaged": true, "gitOpsTool": "argocd", ...}
                        Only included when relationship to RCA resource is PROVEN.
        source_resource: Original signal's resource for DetectedLabels validation
                        Format: {"namespace": "production", "kind": "Pod", "name": "api-xyz"}
                        Compared against LLM's rca_resource.
        owner_chain: K8s ownership chain from SignalProcessing enrichment
                    Format: [{"namespace": "prod", "kind": "ReplicaSet", "name": "..."}, ...]
                    Used for PROVEN relationship validation (100% safe).

    Required environment variables:
    - LLM_MODEL: Full litellm-compatible model identifier (e.g., "provider/model-name")
    - LLM_ENDPOINT: Optional LLM API endpoint

    Note: LLM_MODEL should include the litellm provider prefix if needed
    Examples:
    - "gpt-4" (OpenAI - no prefix needed)
    - "provider_name/model-name" (other providers)
    """
    # Get formatted model name for litellm (supports Ollama, OpenAI, Claude, Vertex AI)
    from src.extensions.llm_config import (
        get_model_config_for_sdk,
        prepare_toolsets_config_for_sdk,
        register_workflow_catalog_toolset
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

    # Get MCP servers configuration from app_config
    # MCP servers are registered as toolsets by the SDK's ToolsetManager
    mcp_servers_config = None
    if app_config and "mcp_servers" in app_config:
        mcp_servers_config = app_config["mcp_servers"]
        logger.info(f"Registering MCP servers: {list(mcp_servers_config.keys())}")

    # Create HolmesGPT SDK Config
    config_data = {
        "model": model_name,
        "api_base": os.getenv("LLM_ENDPOINT"),
        "toolsets": toolsets_config,
        "mcp_servers": mcp_servers_config,
    }

    try:
        config = Config(**config_data)

        # BR-HAPI-250: Register workflow catalog toolset programmatically
        # BR-AUDIT-001: Pass remediation_id for audit trail correlation (DD-WORKFLOW-002 v2.2)
        # DD-HAPI-001: Pass custom_labels for auto-append to workflow search
        # DD-WORKFLOW-001 v1.7: Pass detected_labels with source_resource and owner_chain (100% safe)
        config = register_workflow_catalog_toolset(
            config,
            app_config,
            remediation_id=remediation_id,
            custom_labels=custom_labels,
            detected_labels=detected_labels,
            source_resource=source_resource,
            owner_chain=owner_chain
        )

        # Log labels count for debugging
        custom_labels_info = f", custom_labels={len(custom_labels)} subdomains" if custom_labels else ""
        detected_labels_info = f", detected_labels={len(detected_labels)} fields" if detected_labels else ""
        source_info = f", source={source_resource.get('kind')}/{source_resource.get('namespace', 'cluster')}" if source_resource else ""
        owner_info = f", owner_chain={len(owner_chain)} owners" if owner_chain else ""
        logger.info(f"Initialized HolmesGPT SDK config: model={model_name}, toolsets={list(config.toolset_manager.toolsets.keys())}{custom_labels_info}{detected_labels_info}{source_info}{owner_info}")
        return config
    except Exception as e:
        logger.error(f"Failed to initialize HolmesGPT config: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"LLM configuration error: {str(e)}"
        )


def _create_investigation_prompt(request_data: Dict[str, Any]) -> str:
    """
    Create investigation prompt with complete ADR-041 v3.3 hybrid format.

    Reference: ADR-041 v3.3 - LLM Prompt and Response Contract
    """
    # Extract fields
    signal_type = request_data.get("signal_type", "Unknown")
    severity = request_data.get("severity", "unknown")
    namespace = request_data.get("resource_namespace", "unknown")
    resource_kind = request_data.get("resource_kind", "unknown")
    resource_name = request_data.get("resource_name", "unknown")
    environment = request_data.get("environment", "unknown")
    priority = request_data.get("priority", "P2")
    risk_tolerance = request_data.get("risk_tolerance", "medium")
    business_category = request_data.get("business_category", "standard")

    # Support both legacy and new format (DD-RECOVERY-003)
    failed_action = request_data.get("failed_action", {}) or {}
    failure_context = request_data.get("failure_context", {}) or {}
    error_message = request_data.get("error_message") or failure_context.get("error_message", "Unknown error")
    description = failure_context.get("description", "")

    # Timing information
    firing_time = request_data.get('firing_time', 'Unknown')
    received_time = request_data.get('received_time', 'Unknown')

    # Deduplication and storm
    is_duplicate = request_data.get('is_duplicate', False)
    occurrence_count = request_data.get('occurrence_count', 0)
    first_seen = request_data.get('first_seen', 'Unknown')
    last_seen = request_data.get('last_seen', 'Unknown')
    is_storm = request_data.get('is_storm', False)
    storm_alert_count = request_data.get('storm_alert_count', 0)
    storm_type = request_data.get('storm_type', 'Unknown')
    storm_window = request_data.get('storm_window', '5m')
    affected_resources = request_data.get('affected_resources', [])

    # Cluster context
    cluster_name = request_data.get('cluster_name', 'unknown')
    signal_source = request_data.get('signal_source', 'unknown')
    signal_labels = request_data.get('signal_labels', {})

    # Generate contextual descriptions
    priority_descriptions = {
        "P0": f"P0 (highest priority) - This is a {business_category} service requiring immediate attention",
        "P1": "P1 (high priority) - This service requires prompt attention",
        "P2": "P2 (medium priority) - This service requires timely resolution",
        "P3": "P3 (low priority) - This service can be addressed during normal operations"
    }

    risk_guidance = {
        "low": "low (conservative remediation required - avoid aggressive restarts or scaling)",
        "medium": "medium (balanced approach - standard remediation actions permitted)",
        "high": "high (aggressive remediation permitted - prioritize recovery speed)"
    }

    priority_desc = priority_descriptions.get(priority, f"{priority} - Standard priority")
    risk_desc = risk_guidance.get(risk_tolerance, f"{risk_tolerance} risk tolerance")

    # Build incident summary with natural language
    incident_summary = f"A **{severity} {signal_type} event** from **{signal_source}** has occurred in the **{namespace}/{resource_kind}/{resource_name}**."

    # Add deduplication fact if duplicate
    if is_duplicate and occurrence_count > 0:
        incident_summary += f" **Alert fired {occurrence_count} times**."

    # Add storm fact if storm detected
    if is_storm:
        resource_count = len(affected_resources) if affected_resources else "multiple"
        incident_summary += f" **Storm detected**: {storm_type} type, {storm_alert_count} alerts, {resource_count} resources."

    incident_summary += f"\n{error_message}"

    # Build complete ADR-041 v3.1 hybrid prompt
    prompt = f"""# Incident Analysis Request

## Incident Summary

{incident_summary}

**Business Impact Assessment**:
- **Priority**: {priority_desc}
- **Environment**: {environment}
- **Risk Tolerance**: {risk_desc}

**Technical Details**:
- Signal Type: {signal_type}
- Severity: {severity}
- Resource: {namespace}/{resource_kind}/{resource_name}
- Error: {error_message}
- Failed Action: {failed_action.get('type', 'N/A')} (target: {failed_action.get('target', 'N/A')})

## Error Details (FOR RCA INVESTIGATION)
- Error Message: {error_message}
- Description: {description if description else 'N/A'}
- Firing Time: {firing_time}
- Received Time: {received_time}
"""

    # Add Deduplication Context if applicable
    if is_duplicate and occurrence_count > 0:
        prompt += f"""
## Deduplication Context (FOR RCA INVESTIGATION)
- Is Duplicate: {is_duplicate}
- First Seen: {first_seen}
- Last Seen: {last_seen}
- Occurrence Count: {occurrence_count}

**What Deduplication Means**:
Deduplication tracks duplicate alerts from the monitoring system (Prometheus/Kubernetes). When the same
condition persists, Prometheus fires the same alert every evaluation interval (30-60 seconds). The Gateway
deduplicates these within a 5-minute window to avoid creating multiple RemediationRequest CRDs for the
same ongoing issue.

**RCA Implications**:
- `occurrence_count > 1` means the condition has been **continuously present** since `first_seen`
- This indicates a **persistent, ongoing issue** - not that remediation was attempted and failed
- Focus on understanding why the condition persists, not why remediation failed
- Higher occurrence counts suggest the condition is stable/consistent, not intermittent
"""

    # Add Storm Detection if applicable
    if is_storm:
        prompt += f"""
## Storm Detection (FOR RCA INVESTIGATION)
- Is Storm: {is_storm}
- Storm Type: {storm_type}
- Storm Window: {storm_window}
- Storm Alert Count: {storm_alert_count}
- Affected Resources: {len(affected_resources) if affected_resources else 'Unknown'}
"""
        if affected_resources and len(affected_resources) <= 10:
            prompt += "\n**Affected Resources List**:\n"
            for resource in affected_resources:
                prompt += f"- {resource}\n"
        elif affected_resources:
            prompt += f"\n**Affected Resources** (showing first 10 of {len(affected_resources)}):\n"
            for resource in affected_resources[:10]:
                prompt += f"- {resource}\n"

    # Add Cluster Context
    prompt += f"""
## Cluster Context (FOR RCA INVESTIGATION)
- Cluster: {cluster_name}
- Signal Source: {signal_source}
- Signal Labels: {signal_labels if signal_labels else 'N/A'}

## Business Context (FOR WORKFLOW SEARCH - NOT FOR RCA)
These fields are used by MCP workflow search tools to match workflows.
You do NOT need to consider these in your RCA analysis.

- Environment: {environment}
- Priority: {priority}
- Business Category: {business_category}
- Risk Tolerance: {risk_tolerance}

**Note**: When you call MCP workflow search tools (e.g., `search_workflow_catalog`), you must
pass these business context fields as parameters.

## Required Analysis

**INVESTIGATION APPROACH**:
Perform independent Root Cause Analysis (RCA) using available tools based on the signal type and incident context.

**Available Tools**:
- Kubernetes investigation tools (kubectl, API queries)
- Prometheus/metrics tools (if applicable to signal source)
- Log analysis tools
- Other tools as appropriate for the signal source

**Analysis Steps** (adapt based on signal source and incident):
1. Investigate the signal using appropriate tools for the signal source
2. Gather relevant context and evidence
3. Perform Root Cause Analysis based on your investigation
4. Formulate remediation strategies based on your RCA findings

**Guidance**:
- Use tools appropriate for the signal source (e.g., Kubernetes for pod failures, Prometheus for metric alerts)
- Base your analysis on actual investigation findings, not assumptions
- Consider cluster state and resource availability
- Focus on technical remediation based on RCA findings


## Your Investigation Workflow

**CRITICAL**: Follow this sequence in order. Do NOT search for workflows before investigating.

### Phase 1: Investigate the Incident
Use available tools to investigate the incident:
- Check pod status, events, and logs (kubectl)
- Review resource usage and limits
- Examine node conditions
- Analyze metrics from signal source (if prometheus-adapter)

**Goal**: Understand what actually happened and why.

**Input Signal Provided**: {signal_type} (starting point for investigation)

### Phase 2: Determine Root Cause (RCA)
Based on your investigation findings, identify the root cause.
Is the input signal the root cause, or just a symptom?

### Phase 3: Identify Signal Type That Describes the Effect
Based on your RCA, determine the signal_type that best describes the effect:

**If investigation confirms input signal is the root cause**:
- Input: OOMKilled → Investigation confirms memory limit exceeded → Use "OOMKilled"

**If investigation reveals different root cause**:
- Input: OOMKilled → Investigation shows node memory pressure → Use "NodePressure" or "Evicted"

**Important**: The signal_type for workflow search comes from YOUR investigation findings, not the input signal.

### Phase 4: Search for Workflow (MANDATORY)
**YOU MUST** call MCP `search_workflow_catalog` tool with:
- **Query**: `"<YOUR_RCA_SIGNAL_TYPE> <YOUR_RCA_SEVERITY>"`
- **Label Filters**: Business context values

**This step is REQUIRED** - you cannot skip workflow search. If the tool is available, you must invoke it.

### Phase 5: Return Summary + JSON Payload
Provide natural language summary + structured JSON with workflow and parameters.

**If MCP search succeeds**:
```json
{{
  "root_cause_analysis": {{
    "summary": "Brief summary of root cause from investigation",
    "severity": "critical|high|medium|low",
    "contributing_factors": ["factor1", "factor2"]
  }},
  "selected_workflow": {{
    "workflow_id": "workflow-id-from-mcp-search",
    "version": "1.0.0",
    "confidence": 0.95,
    "rationale": "Why your RCA findings led to this workflow selection",
    "parameters": {{
      "PARAM_NAME": "value-from-investigation"
    }}
  }}
}}
```

**If MCP search fails or returns no workflows**:
```json
{{
  "root_cause_analysis": {{
    "summary": "Root cause from investigation",
    "severity": "critical|high|medium|low",
    "contributing_factors": ["factor1", "factor2"]
  }},
  "selected_workflow": null,
  "rationale": "MCP search failed: [error details]. RCA completed but workflow selection unavailable."
}}
```

## RCA Severity Assessment

After your investigation, assess the severity of the root cause using these levels.

**IMPORTANT**: Your RCA severity may differ from the input signal severity. Use your analysis to determine the actual severity based on business impact.

### Severity Levels:

**critical** - Immediate remediation required
- Production service completely unavailable
- Data loss or corruption occurring
- Security breach actively exploited
- SLA violation in progress
- Revenue-impacting outage
- Affects >50% of users

**high** - Urgent remediation needed
- Significant service degradation (>50% performance loss)
- High error rate (>10% of requests failing)
- Production issue escalating toward critical
- Affects 10-50% of users
- SLA at risk

**medium** - Remediation recommended
- Minor service degradation (<50% performance loss)
- Moderate error rate (1-10% of requests failing)
- Non-production critical issues
- Affects <10% of users
- Staging/development critical issues

**low** - Remediation optional
- Informational issues
- Optimization opportunities
- Development environment issues
- No user impact
- Capacity planning alerts

## MCP Workflow Search Guidance

When searching for remediation workflows, use this taxonomy:

**Query Format**: `<signal_type> <severity> [optional_keywords]`
- Example: `"OOMKilled critical"` or `"CrashLoopBackOff high"`
- Use canonical Kubernetes event reasons for signal_type (from your RCA assessment)
- Use your RCA severity assessment (may differ from input signal)

**Canonical Signal Types** (examples - use any canonical Kubernetes event reason):
- `OOMKilled`: Container exceeded memory limit and was killed
- `CrashLoopBackOff`: Container repeatedly crashing
- `ImagePullBackOff`: Cannot pull container image
- `Evicted`: Pod evicted due to resource pressure
- `NodeNotReady`: Node is not ready
- `PodPending`: Pod stuck in pending state
- `FailedScheduling`: Scheduler cannot place pod
- `BackoffLimitExceeded`: Job exceeded retry limit
- `DeadlineExceeded`: Job exceeded active deadline
- `FailedMount`: Volume mount failed

**Note**: These are common examples. Use any canonical Kubernetes event reason that matches your RCA findings.
For complete list, see: https://kubernetes.io/docs/reference/kubernetes-api/cluster-resources/event-v1/#Event

**Label Parameters** (for MCP workflow search):
1. **signal_type** (Technical - from your RCA assessment)
2. **severity** (Technical - from your RCA assessment)
3. **environment** (Business - pass-through: `{environment}`)
4. **priority** (Business - pass-through: `{priority}`)
5. **risk_tolerance** (Business - pass-through: `{risk_tolerance}`)
6. **business_category** (Business - pass-through: `{business_category}`)

**Search Optimization**:
- Exact label matching increases confidence score
- Workflow descriptions should start with `"<signal_type> <severity>:"`
- Use all 6 label parameters for filtering

## Expected Response Format

Provide your analysis in two parts:

### Part 1: Natural Language Analysis

Explain your investigation findings, root cause analysis, and reasoning for workflow selection.

### Part 2: Structured JSON

```json
{{
  "root_cause_analysis": {{
    "summary": "Brief summary of root cause",
    "severity": "critical|high|medium|low",
    "contributing_factors": ["factor1", "factor2"]
  }},
  "selected_workflow": {{
    "workflow_id": "workflow-id-from-mcp-search-results",
    "version": "1.0.0",
    "confidence": 0.95,
    "rationale": "Why your search parameters led to this workflow selection (based on RCA findings)",
    "parameters": {{
      "PARAM_NAME": "value",
      "ANOTHER_PARAM": "value"
    }}
  }}
}}
```

**IMPORTANT**:
- Select ONE workflow per incident
- Populate ALL required parameters from the workflow schema
- Use your RCA findings to determine parameter values
- Pass-through business context fields (environment, priority, risk_tolerance, business_category) to MCP search
"""

    return prompt


def _parse_investigation_result(investigation: InvestigationResult, request_data: Dict[str, Any]) -> Dict[str, Any]:
    """
    Parse HolmesGPT InvestigationResult into RecoveryResponse format

    Extracts recovery strategies from LLM analysis.
    Handles both incident and recovery analysis modes.

    Design Decision: DD-RECOVERY-003 - Recovery-specific parsing
    """
    incident_id = request_data.get("incident_id")
    is_recovery = request_data.get("is_recovery_attempt", False)

    # Parse LLM analysis for recovery strategies
    analysis_text = investigation.analysis or ""

    # If this is a recovery attempt, try to parse recovery-specific JSON first
    if is_recovery:
        recovery_result = _parse_recovery_specific_result(analysis_text, request_data)
        if recovery_result:
            return recovery_result

    # Fall back to standard parsing
    # Extract strategies from analysis
    strategies = _extract_strategies_from_analysis(analysis_text)

    # Determine if recovery is possible
    can_recover = len(strategies) > 0
    primary_recommendation = strategies[0].action_type if strategies else None

    # Calculate overall confidence
    analysis_confidence = max([s.confidence for s in strategies]) if strategies else 0.0

    # Extract warnings from analysis
    warnings = _extract_warnings_from_analysis(analysis_text)

    result = RecoveryResponse(
        incident_id=incident_id,
        can_recover=can_recover,
        strategies=strategies,
        primary_recommendation=primary_recommendation,
        analysis_confidence=analysis_confidence,
        warnings=warnings,
        metadata={
            "analysis_time_ms": 2000,  # GREEN phase: static value
            "tool_calls": len(investigation.tool_calls) if hasattr(investigation, 'tool_calls') and investigation.tool_calls else 0,
            "sdk_version": "holmesgpt-0.1.0",
            "is_recovery_attempt": is_recovery
        }
    )

    return result.model_dump() if hasattr(result, 'model_dump') else result.dict()


def _parse_recovery_specific_result(analysis_text: str, request_data: Dict[str, Any]) -> Optional[Dict[str, Any]]:
    """
    Parse HolmesGPT InvestigationResult into recovery response format.

    Handles recovery-specific fields: recovery_analysis, recovery_strategy

    Design Decision: DD-RECOVERY-003
    """
    incident_id = request_data.get("incident_id")
    recovery_attempt_number = request_data.get("recovery_attempt_number", 1)

    # Try to extract structured JSON from response
    json_match = re.search(r'```json\s*(.*?)\s*```', analysis_text, re.DOTALL)
    if not json_match:
        # Try to find JSON object directly
        json_match = re.search(r'\{.*"recovery_analysis".*\}', analysis_text, re.DOTALL)
        if not json_match:
            return None

    try:
        json_text = json_match.group(1) if hasattr(json_match, 'group') and json_match.lastindex else json_match.group(0)
        structured = json.loads(json_text)

        # Extract recovery-specific fields if present
        recovery_analysis = structured.get("recovery_analysis", {})
        recovery_strategy = structured.get("recovery_strategy", {})
        selected_workflow = structured.get("selected_workflow")

        # Build recovery response
        can_recover = selected_workflow is not None
        confidence = selected_workflow.get("confidence", 0.0) if selected_workflow else 0.0

        # Convert to standard RecoveryResponse format
        strategies = []
        if selected_workflow:
            strategies.append(RecoveryStrategy(
                action_type=selected_workflow.get("workflow_id", "unknown_workflow"),
                confidence=float(confidence),
                rationale=selected_workflow.get("rationale", "Recovery workflow selected based on failure analysis"),
                estimated_risk="medium",  # Default for recovery
                prerequisites=[]
            ))

        result = {
            "incident_id": incident_id,
            "is_recovery_attempt": True,
            "recovery_attempt_number": recovery_attempt_number,
            "can_recover": can_recover,
            "strategies": [s.model_dump() if hasattr(s, 'model_dump') else s.dict() for s in strategies],
            "primary_recommendation": strategies[0].action_type if strategies else None,
            "analysis_confidence": confidence,
            "warnings": [],
            "metadata": {
                "analysis_time_ms": 2000,
                "sdk_version": "holmesgpt-0.1.0",
                "is_recovery_attempt": True,
                "recovery_attempt_number": recovery_attempt_number
            },
            # Recovery-specific fields
            "recovery_analysis": recovery_analysis,
            "recovery_strategy": recovery_strategy,
            "selected_workflow": selected_workflow,
            "raw_analysis": analysis_text,
        }

        logger.info(f"Successfully parsed recovery-specific response for incident {incident_id}")
        return result

    except (json.JSONDecodeError, AttributeError, KeyError, ValueError) as e:
        logger.warning(f"Failed to parse recovery-specific JSON: {e}")
        return None


def _extract_strategies_from_analysis(analysis_text: str) -> List[RecoveryStrategy]:
    """
    Extract recovery strategies from LLM analysis text

    REFACTOR phase: Attempts to parse structured JSON output, falls back to keyword extraction
    """
    strategies = []

    # REFACTOR Phase: Try to parse structured JSON output
    try:
        # LLM may wrap JSON in markdown code blocks
        json_match = re.search(r'```(?:json)?\s*(\{.*?\})\s*```', analysis_text, re.DOTALL)
        if json_match:
            json_text = json_match.group(1)
        else:
            # Try to find JSON object directly
            json_match = re.search(r'\{.*"strategies".*\}', analysis_text, re.DOTALL)
            json_text = json_match.group(0) if json_match else None

        if json_text:
            parsed = json.loads(json_text)

            # Extract strategies from structured output
            for strategy_data in parsed.get("strategies", []):
                strategies.append(RecoveryStrategy(
                    action_type=strategy_data.get("action_type", "unknown_action"),
                    confidence=float(strategy_data.get("confidence", 0.5)),
                    rationale=strategy_data.get("rationale", "LLM analysis"),
                    estimated_risk=strategy_data.get("estimated_risk", "medium"),
                    prerequisites=strategy_data.get("prerequisites", [])
                ))

            if strategies:
                logger.info(f"Successfully parsed {len(strategies)} strategies from structured JSON")
                return strategies
    except (json.JSONDecodeError, AttributeError, KeyError, ValueError) as e:
        logger.warning(f"Failed to parse structured JSON from LLM: {e}, falling back to keyword extraction")

    # Fallback: Keyword-based extraction (backward compatible with GREEN phase)
    logger.info("Using keyword-based strategy extraction (fallback)")

    if "rollback" in analysis_text.lower():
        strategies.append(RecoveryStrategy(
            action_type="rollback_to_previous_state",
            confidence=0.8,
            rationale="LLM recommends rollback based on analysis",
            estimated_risk="low",
            prerequisites=[]
        ))

    if "scale" in analysis_text.lower() or "retry" in analysis_text.lower():
        strategies.append(RecoveryStrategy(
            action_type="retry_with_modifications",
            confidence=0.7,
            rationale="LLM suggests retry with adjustments",
            estimated_risk="medium",
            prerequisites=[]
        ))

    # If no strategies extracted, provide default
    if not strategies:
        strategies.append(RecoveryStrategy(
            action_type="manual_intervention_required",
            confidence=0.5,
            rationale="Automated recovery not recommended",
            estimated_risk="low",
            prerequisites=["human_review"]
        ))

    return strategies


def _extract_warnings_from_analysis(analysis_text: str) -> List[str]:
    """
    Extract warnings from LLM analysis

    REFACTOR phase: Attempts to parse structured JSON output, falls back to keyword extraction
    """
    warnings = []

    # REFACTOR Phase: Try to parse warnings from structured JSON
    try:
        json_match = re.search(r'```(?:json)?\s*(\{.*?\})\s*```', analysis_text, re.DOTALL)
        if json_match:
            json_text = json_match.group(1)
        else:
            json_match = re.search(r'\{.*"warnings".*\}', analysis_text, re.DOTALL)
            json_text = json_match.group(0) if json_match else None

        if json_text:
            parsed = json.loads(json_text)
            extracted_warnings = parsed.get("warnings", [])
            if extracted_warnings:
                logger.info(f"Successfully parsed {len(extracted_warnings)} warnings from structured JSON")
                return extracted_warnings
    except (json.JSONDecodeError, AttributeError, KeyError) as e:
        logger.debug(f"Failed to parse warnings from JSON: {e}, using keyword extraction")

    # Fallback: Keyword-based extraction
    if "risk" in analysis_text.lower() or "caution" in analysis_text.lower():
        warnings.append("LLM identified potential risks - review carefully")

    if "high load" in analysis_text.lower() or "resource" in analysis_text.lower():
        warnings.append("Resource constraints may affect recovery")

    return warnings


# NOTE: _get_workflow_recommendations() function REMOVED per DD-WORKFLOW-002 v2.4
# Workflow search is now handled by WorkflowCatalogToolset registered via
# register_workflow_catalog_toolset() - the LLM calls the tool during investigation.
# The old mcp_client-based workflow fetch was dead code (results stored but never used).


async def analyze_recovery(request_data: Dict[str, Any], app_config: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
    """
    Core recovery analysis logic

    Business Requirements: BR-HAPI-001 to 050
    Design Decision: DD-WORKFLOW-002 v2.4 - WorkflowCatalogToolset via SDK

    Uses HolmesGPT SDK for AI-powered recovery analysis.
    Workflow search is handled by WorkflowCatalogToolset registered with the SDK.
    LLM endpoint is configured via environment variables (LLM_ENDPOINT, LLM_MODEL, LLM_PROVIDER).
    """
    incident_id = request_data.get("incident_id")
    is_recovery = request_data.get("is_recovery_attempt", False)

    # Support both legacy and new format (DD-RECOVERY-003)
    failed_action = request_data.get("failed_action", {}) or {}
    failure_context = request_data.get("failure_context", {}) or {}
    previous_execution = request_data.get("previous_execution", {}) or {}

    # Determine action type for logging
    if is_recovery and previous_execution:
        failure = previous_execution.get("failure", {}) or {}
        action_type = f"recovery_from_{failure.get('reason', 'unknown')}"
    else:
        action_type = failed_action.get("type", "unknown")

    logger.info({
        "event": "recovery_analysis_started",
        "incident_id": incident_id,
        "action_type": action_type,
        "is_recovery_attempt": is_recovery
    })

    # BR-AUDIT-001: Extract remediation_id for audit trail correlation (DD-WORKFLOW-002 v2.2)
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
    custom_labels = enrichment_results.get("customLabels")  # camelCase from K8s

    if custom_labels:
        logger.info({
            "event": "custom_labels_extracted",
            "incident_id": incident_id,
            "subdomains": list(custom_labels.keys()),
            "message": f"DD-HAPI-001: {len(custom_labels)} custom label subdomains will be auto-appended to workflow search"
        })

    # DD-WORKFLOW-001 v1.7: Extract detected_labels for workflow matching (100% safe)
    detected_labels = enrichment_results.get("detectedLabels", {}) or {}

    # DD-WORKFLOW-001 v1.7: Extract source_resource for DetectedLabels validation
    # This is the original signal's resource - compared against LLM's rca_resource
    source_resource = {
        "namespace": request_data.get("resource_namespace", ""),
        "kind": request_data.get("resource_kind", ""),
        "name": request_data.get("resource_name", "")
    }

    # DD-WORKFLOW-001 v1.7: Extract owner_chain from enrichment_results
    # K8s ownership chain from SignalProcessing (via ownerReferences)
    # Used for PROVEN relationship validation (100% safe)
    owner_chain = enrichment_results.get("ownerChain")

    if detected_labels:
        logger.info({
            "event": "detected_labels_extracted",
            "incident_id": incident_id,
            "fields": list(detected_labels.keys()),
            "source_resource": f"{source_resource.get('kind')}/{source_resource.get('namespace') or 'cluster'}",
            "owner_chain_length": len(owner_chain) if owner_chain else 0,
            "message": f"DD-WORKFLOW-001 v1.7: {len(detected_labels)} detected labels (100% safe validation)"
        })

    config = _get_holmes_config(
        app_config,
        remediation_id=remediation_id,
        custom_labels=custom_labels,
        detected_labels=detected_labels,
        source_resource=source_resource,
        owner_chain=owner_chain
    )

    # Use HolmesGPT SDK with enhanced error handling
    # NOTE: Workflow search is handled by WorkflowCatalogToolset registered via
    # register_workflow_catalog_toolset() - the LLM calls the tool during investigation
    # per DD-WORKFLOW-002 v2.4
    try:
        # Create investigation prompt
        # DD-RECOVERY-003: Use recovery-specific prompt for recovery attempts
        is_recovery = request_data.get("is_recovery_attempt", False)
        if is_recovery and request_data.get("previous_execution"):
            investigation_prompt = _create_recovery_investigation_prompt(request_data)
            logger.info({
                "event": "using_recovery_prompt",
                "incident_id": incident_id,
                "recovery_attempt_number": request_data.get("recovery_attempt_number", 1)
            })
        else:
            investigation_prompt = _create_investigation_prompt(request_data)

        # Log the prompt being sent to LLM
        print("\n" + "="*80)
        print("🔍 PROMPT TO LLM PROVIDER (via HolmesGPT SDK)")
        print("="*80)
        print(investigation_prompt)
        print("="*80 + "\n")

        # Create investigation request
        investigation_request = InvestigateRequest(
            source="kubernaut",
            title=f"Recovery analysis for {failed_action.get('type')} failure",
            description=investigation_prompt,
            subject={
                "type": "remediation_failure",
                "incident_id": incident_id,
                "failed_action": failed_action
            },
            context={
                "incident_id": incident_id,
                "issue_type": "remediation_failure"
            },
            source_instance_id="holmesgpt-api"
        )

        # Create minimal DAL (no Robusta Platform database needed)
        dal = MinimalDAL(cluster_name=request_data.get("context", {}).get("cluster"))

        # Debug: Log investigation details before SDK call
        logger.debug({
            "event": "calling_holmesgpt_sdk",
            "incident_id": incident_id,
            "prompt_length": len(investigation_prompt),
            "toolsets_enabled": config.toolsets if config else None,
            "prompt_preview": investigation_prompt[:300] + "..." if len(investigation_prompt) > 300 else investigation_prompt
        })

        # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
        # LLM INTERACTION AUDIT (BR-AUDIT-005, ADR-038, DD-AUDIT-002)
        # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
        # Store structured audit events via BufferedAuditStore (fire-and-forget)

        # Get remediation_id from request context (for audit correlation)
        remediation_id = request_data.get("context", {}).get("remediation_request_id", "")

        # Audit: LLM request (prompt sent to model)
        audit_store = get_audit_store()
        if audit_store:
            audit_store.store_audit(create_llm_request_event(
                incident_id=incident_id,
                remediation_id=remediation_id,
                model=config.model if config else "unknown",
                prompt=investigation_prompt,
                toolsets_enabled=list(config.toolsets.keys()) if config and config.toolsets else [],
                mcp_servers=list(config.mcp_servers.keys()) if config and hasattr(config, 'mcp_servers') and config.mcp_servers else []
            ))

        # Debug log (kept for backwards compatibility)
        logger.info({
            "event": "llm_request",
            "incident_id": incident_id,
            "model": config.model if config else "unknown",
            "prompt_length": len(investigation_prompt),
            "prompt_preview": investigation_prompt[:500] + "..." if len(investigation_prompt) > 500 else investigation_prompt,
            "toolsets_enabled": config.toolsets if config else [],
            "mcp_servers": list(config.mcp_servers.keys()) if config and hasattr(config, 'mcp_servers') and config.mcp_servers else [],
        })

        # Call HolmesGPT SDK
        logger.info("Calling HolmesGPT SDK for recovery analysis")
        investigation_result = investigate_issues(
            investigate_request=investigation_request,
            dal=dal,
            config=config
        )

        # Audit: LLM response
        has_analysis = bool(investigation_result and investigation_result.analysis)
        analysis_length = len(investigation_result.analysis) if investigation_result and investigation_result.analysis else 0
        analysis_preview = investigation_result.analysis[:500] + "..." if investigation_result and investigation_result.analysis and len(investigation_result.analysis) > 500 else (investigation_result.analysis if investigation_result and investigation_result.analysis else "")
        tool_call_count = len(investigation_result.tool_calls) if investigation_result and hasattr(investigation_result, 'tool_calls') and investigation_result.tool_calls else 0

        if audit_store:
            audit_store.store_audit(create_llm_response_event(
                incident_id=incident_id,
                remediation_id=remediation_id,
                has_analysis=has_analysis,
                analysis_length=analysis_length,
                analysis_preview=analysis_preview,
                tool_call_count=tool_call_count
            ))

        # Debug log (kept for backwards compatibility)
        logger.info({
            "event": "llm_response",
            "incident_id": incident_id,
            "has_analysis": has_analysis,
            "analysis_length": analysis_length,
            "analysis_preview": analysis_preview,
            "has_tool_calls": hasattr(investigation_result, 'tool_calls') and bool(investigation_result.tool_calls) if investigation_result else False,
            "tool_call_count": tool_call_count,
        })

        # Audit: Tool call details (SDK-dependent)
        if investigation_result and hasattr(investigation_result, 'tool_calls') and investigation_result.tool_calls:
            for idx, tool_call in enumerate(investigation_result.tool_calls):
                tool_name = getattr(tool_call, 'name', 'unknown')
                tool_arguments = getattr(tool_call, 'arguments', {})
                tool_result = getattr(tool_call, 'result', None)

                if audit_store:
                    audit_store.store_audit(create_tool_call_event(
                        incident_id=incident_id,
                        remediation_id=remediation_id,
                        tool_call_index=idx,
                        tool_name=tool_name,
                        tool_arguments=tool_arguments,
                        tool_result=tool_result
                    ))

                # Debug log (kept for backwards compatibility)
                logger.info({
                    "event": "llm_tool_call",
                    "incident_id": incident_id,
                    "tool_call_index": idx,
                    "tool_name": tool_name,
                    "tool_arguments": tool_arguments,
                    "tool_result": tool_result,
                })

        # Log the raw LLM response (for debugging)
        print("\n" + "="*80)
        print("🤖 RAW LLM RESPONSE (from HolmesGPT SDK)")
        print("="*80)
        if investigation_result:
            if investigation_result.analysis:
                print(f"Analysis (full):\n{investigation_result.analysis}")
            else:
                print("No analysis returned")
            # Check if tool_calls attribute exists
            if hasattr(investigation_result, 'tool_calls') and investigation_result.tool_calls:
                print(f"\nTool Calls: {len(investigation_result.tool_calls)}")
                for idx, tool_call in enumerate(investigation_result.tool_calls):
                    print(f"  Tool {idx+1}: {getattr(tool_call, 'name', 'unknown')}")
        else:
            print("No result returned from SDK")
        print("="*80 + "\n")
        # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

        # Validate investigation result
        if not investigation_result or not investigation_result.analysis:
            logger.error({
                "event": "sdk_empty_response",
                "incident_id": incident_id,
                "message": "SDK returned empty analysis"
            })
            raise HTTPException(
                status_code=status.HTTP_502_BAD_GATEWAY,
                detail="LLM provider returned empty response"
            )

        # Parse result into recovery response
        result = _parse_investigation_result(investigation_result, request_data)

        logger.info({
            "event": "recovery_analysis_completed",
            "incident_id": incident_id,
            "strategy_count": len(result.get("strategies", [])),
            "confidence": result.get("analysis_confidence"),
            "analysis_length": len(investigation_result.analysis) if investigation_result.analysis else 0
        })

        return result

    except ValueError as e:
        # Configuration or validation errors
        logger.error({
            "event": "sdk_validation_error",
            "incident_id": incident_id,
            "error_type": "ValueError",
            "error": str(e),
            "failed_action": failed_action.get("type")
        })
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Configuration error: {str(e)}"
        )

    except (ConnectionError, TimeoutError) as e:
        # Network/LLM provider errors
        logger.error({
            "event": "sdk_connection_error",
            "incident_id": incident_id,
            "error_type": type(e).__name__,
            "error": str(e),
            "provider": os.getenv("LLM_MODEL", "unknown")
        })
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail=f"LLM provider unavailable: {str(e)}"
        )

    except Exception as e:
        # Catch-all for unexpected errors
        logger.error({
            "event": "sdk_analysis_failed",
            "incident_id": incident_id,
            "error_type": type(e).__name__,
            "error": str(e),
            "error_details": {
                "failed_action_type": failed_action.get("type"),
                "cluster": request_data.get("context", {}).get("cluster"),
                "namespace": failed_action.get("namespace")
            }
        }, exc_info=True)
        raise


@router.post("/recovery/analyze", status_code=status.HTTP_200_OK, response_model=RecoveryResponse)
async def recovery_analyze_endpoint(request: RecoveryRequest) -> RecoveryResponse:
    """
    Analyze failed action and provide recovery strategies

    Business Requirement: BR-HAPI-001 (Recovery analysis endpoint)
    Design Decision: DD-WORKFLOW-002 v2.4 - WorkflowCatalogToolset via SDK

    Called by: AIAnalysis Controller (for recovery attempts after workflow failure)
    """
    request_data = request.model_dump() if hasattr(request, 'model_dump') else request.dict()
    result = await analyze_recovery(request_data)
    return result


