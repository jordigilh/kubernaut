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
Incident Analysis Endpoint

Business Requirements: BR-HAPI-002 (Incident Analysis)
Design Decision: DD-RECOVERY-003 (DetectedLabels for workflow filtering)

Provides AI-powered Root Cause Analysis (RCA) and workflow selection for initial incidents.
Separate from recovery.py which handles failed remediation retry scenarios.
"""

import logging
import os
from typing import Dict, Any, Optional, List
from fastapi import APIRouter, HTTPException, status
from datetime import datetime

from src.models.incident_models import IncidentRequest, IncidentResponse, DetectedLabels, EnrichmentResults
from src.toolsets.workflow_catalog import WorkflowCatalogToolset

# HolmesGPT SDK imports
from holmes.config import Config
from holmes.core.models import InvestigateRequest, InvestigationResult
from holmes.core.investigation import investigate_issues

# Audit imports (BR-AUDIT-005, ADR-038, DD-AUDIT-002)
from src.audit import (
    BufferedAuditStore,
    AuditConfig,
    create_llm_request_event,
    create_llm_response_event,
    create_tool_call_event,
    create_validation_attempt_event,
)

logger = logging.getLogger(__name__)

# ========================================
# AUDIT STORE INITIALIZATION (BR-AUDIT-005, ADR-038)
# ========================================
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

router = APIRouter()

# ========================================
# LLM SELF-CORRECTION CONSTANTS (DD-HAPI-002 v1.2)
# ========================================
MAX_VALIDATION_ATTEMPTS = 3  # BR-HAPI-197: Max attempts before human review


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


# ========================================
# LLM SELF-CORRECTION FEEDBACK (DD-HAPI-002 v1.2)
# ========================================

def _build_validation_error_feedback(errors: List[str], attempt: int) -> str:
    """
    Build validation error feedback to append to LLM prompt for self-correction.

    Design Decision: DD-HAPI-002 v1.2 (Workflow Response Validation)
    Business Requirement: BR-HAPI-197 (needs_human_review field)

    This feedback is appended to the conversation when the LLM's workflow response
    fails validation. The LLM uses this to self-correct while context is preserved.

    Args:
        errors: List of validation error messages
        attempt: Current attempt number (0-indexed)

    Returns:
        Formatted error feedback section for the prompt
    """
    attempt_display = attempt + 1  # Convert to 1-indexed for display
    errors_list = "\n".join(f"- {error}" for error in errors)

    return f"""

## ⚠️ VALIDATION ERROR - CORRECTION REQUIRED (Attempt {attempt_display}/{MAX_VALIDATION_ATTEMPTS})

Your previous workflow response had validation errors:

{errors_list}

**Please correct your response:**
1. Re-check the workflow ID exists in the catalog (use MCP search_workflow_catalog)
2. Ensure container_image matches the catalog exactly (or omit to use catalog default)
3. Verify all required parameters are provided with correct types and values

**Re-submit your JSON response with the corrected workflow selection.**
"""


# Minimal DAL for HolmesGPT SDK integration (no Robusta Platform)
class MinimalDAL:
    """
    Minimal DAL for HolmesGPT SDK integration (no Robusta Platform)

    Kubernaut does NOT integrate with Robusta Platform.
    This MinimalDAL satisfies HolmesGPT SDK's DAL interface requirements
    without connecting to any Robusta Platform database.

    All methods return None/empty to indicate no Robusta Platform data available.
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


def _create_incident_investigation_prompt(request_data: Dict[str, Any]) -> str:
    """
    Create investigation prompt for initial incident analysis (ADR-041 v3.3).

    Used by: /incident/analyze endpoint
    Input: IncidentRequest model data
    Reference: ADR-041 v3.3 - LLM Prompt and Response Contract
    """
    # Extract fields from IncidentRequest
    signal_type = request_data.get("signal_type", "Unknown")
    severity = request_data.get("severity", "unknown")
    namespace = request_data.get("resource_namespace", "unknown")
    resource_kind = request_data.get("resource_kind", "unknown")
    resource_name = request_data.get("resource_name", "unknown")
    environment = request_data.get("environment", "unknown")
    priority = request_data.get("priority", "P2")
    risk_tolerance = request_data.get("risk_tolerance", "medium")
    business_category = request_data.get("business_category", "standard")

    # Error details (top-level in IncidentRequest, not nested)
    error_message = request_data.get("error_message", "Unknown error")
    description = request_data.get("description", "")

    # Timing information
    firing_time = request_data.get('firing_time', 'Unknown')
    received_time = request_data.get('received_time', 'Unknown')

    # Deduplication and storm
    is_duplicate = request_data.get('is_duplicate', False)
    occurrence_count = request_data.get('occurrence_count', 0)
    first_seen = request_data.get('first_seen', 'Unknown')
    last_seen = request_data.get('last_seen', 'Unknown')
    is_storm = request_data.get('is_storm', False)
    storm_signal_count = request_data.get('storm_signal_count', 0)
    storm_type = request_data.get('storm_type', 'Unknown')
    storm_window_minutes = request_data.get('storm_window_minutes', 5)
    affected_resources = request_data.get('affected_resources', [])

    # Cluster context
    cluster_name = request_data.get('cluster_name', 'unknown')
    signal_source = request_data.get('signal_source', 'unknown')
    signal_labels = request_data.get('signal_labels', {})

    # DetectedLabels from enrichment_results (DD-RECOVERY-003)
    enrichment_results = request_data.get('enrichment_results', {})
    detected_labels = {}
    if enrichment_results:
        # Handle both dict and EnrichmentResults model
        if hasattr(enrichment_results, 'detectedLabels'):
            dl = enrichment_results.detectedLabels
            if dl:
                detected_labels = dl.model_dump() if hasattr(dl, 'model_dump') else dl.dict() if hasattr(dl, 'dict') else dl
        elif isinstance(enrichment_results, dict):
            dl = enrichment_results.get('detectedLabels', {})
            if dl:
                detected_labels = dl.model_dump() if hasattr(dl, 'model_dump') else dl.dict() if hasattr(dl, 'dict') else dl

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
        incident_summary += f" **This signal has been received {occurrence_count} times within a {request_data.get('deduplication_window_minutes', 5)}-minute window**."

    # Add storm fact if storm detected
    if is_storm:
        resource_count = len(affected_resources) if affected_resources else "multiple"
        incident_summary += f" **Alert storm detected**: {storm_signal_count} similar signals within {storm_window_minutes} minutes affecting {resource_count} resources."

    incident_summary += f"\n{error_message}"

    # Build complete ADR-041 v3.3 hybrid prompt
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
- Signal Source: {signal_source}
- Cluster: {cluster_name}

## Error Details (FOR RCA INVESTIGATION)
- Error Message: {error_message}
- Description: {description if description else 'N/A'}
- Firing Time: {firing_time}
- Received Time: {received_time}
"""

    # Add Deduplication Context if applicable
    if is_duplicate and occurrence_count > 0:
        dedup_window = request_data.get('deduplication_window_minutes', 5)
        prompt += f"""
## Signal Deduplication Context

**Observable Fact**: This signal has been received {occurrence_count} times within a {dedup_window}-minute window.

**Deduplication Details**:
- First Seen: {first_seen}
- Last Seen: {last_seen}
- Deduplication Window: {dedup_window} minutes
- Occurrence Count: {occurrence_count}

**Note**: This indicates the same signal fingerprint was detected multiple times, suggesting a persistent or recurring issue.
"""

    # Add Storm Detection Context if applicable
    if is_storm:
        prompt += f"""
## Alert Storm Detection

**Observable Fact**: Alert storm detected with {storm_signal_count} similar signals within {storm_window_minutes} minutes.

**Storm Details**:
- Storm Type: {storm_type}
- Signal Count: {storm_signal_count}
- Time Window: {storm_window_minutes} minutes
- Affected Resources: {len(affected_resources) if affected_resources else 'Unknown'}
"""
        if affected_resources:
            prompt += f"- Resource List: {', '.join(affected_resources[:5])}"
            if len(affected_resources) > 5:
                prompt += f" (and {len(affected_resources) - 5} more)"
            prompt += "\n"

    # Add Cluster Environment Characteristics if DetectedLabels are available (DD-RECOVERY-003)
    if detected_labels:
        prompt += f"""
## Cluster Environment Characteristics (AUTO-DETECTED)

The following characteristics were automatically detected for the target resource.
**YOU MUST include these as filters in your MCP workflow search request.**

{_build_cluster_context_section(detected_labels)}

{_build_mcp_filter_instructions(detected_labels)}

"""

    # Add Business Context section
    prompt += f"""
## Business Context (FOR MCP WORKFLOW SEARCH)

**IMPORTANT**: These fields are for MCP workflow search label filtering, NOT for your RCA investigation.

- Environment: {environment}

- Priority: {priority}

- Business Category: {business_category}

- Risk Tolerance: {risk_tolerance}

**Note**: When you call MCP workflow search tools (e.g., `search_workflow_catalog`), you must
pass these business context fields as parameters.

## Your Investigation Workflow

**CRITICAL**: Follow this sequence in order. Do NOT search for workflows before investigating.

### Phase 1: Investigate the Incident
Use available tools to investigate the incident:
- Check pod status, events, and logs (kubectl)
- Review resource usage and limits
- Examine node conditions
- Analyze metrics from signal source (if prometheus)

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
  }},
  "alternative_workflows": [
    {{
      "workflow_id": "other-workflow-id",
      "container_image": "image:tag",
      "confidence": 0.75,
      "rationale": "Why this was considered but not selected"
    }}
  ]
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
  }},
  "alternative_workflows": [
    {{
      "workflow_id": "alternative-workflow-id",
      "container_image": "image:tag",
      "confidence": 0.75,
      "rationale": "Why this alternative was considered but not selected"
    }}
  ]
}}
```

**IMPORTANT**:
- Select ONE workflow per incident as `selected_workflow`
- Include up to 2-3 alternative workflows considered but not selected
- `alternative_workflows` are for AUDIT/CONTEXT only - they help operators understand what options were considered
- Populate ALL required parameters from the workflow schema
- Use your RCA findings to determine parameter values
- Pass-through business context fields (environment, priority, risk_tolerance, business_category) to MCP search
"""

    return prompt


async def analyze_incident(request_data: Dict[str, Any], mcp_config: Optional[Dict[str, Any]] = None, app_config: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
    """
    Core incident analysis logic with LLM self-correction loop.

    Business Requirements:
    - BR-HAPI-002 (Incident analysis)
    - BR-HAPI-197 (needs_human_review field)

    Design Decision: DD-HAPI-002 v1.2 (Workflow Response Validation)

    Self-Correction Loop:
    1. Call HolmesGPT SDK for RCA and workflow selection
    2. Validate workflow response (existence, image, parameters)
    3. If invalid, feed errors back to LLM for self-correction
    4. Retry up to MAX_VALIDATION_ATTEMPTS times
    5. If all attempts fail, set needs_human_review=True
    """
    incident_id = request_data.get("incident_id", "unknown")

    logger.info({
        "event": "incident_analysis_started",
        "incident_id": incident_id,
        "signal_type": request_data.get("signal_type")
    })

    # Use HolmesGPT SDK for AI-powered analysis
    try:
        # Create base investigation prompt
        base_prompt = _create_incident_investigation_prompt(request_data)

        # Create minimal DAL
        dal = MinimalDAL(cluster_name=request_data.get("cluster_name"))

        # Create HolmesGPT config with workflow catalog toolset (BR-HAPI-250)
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

        # Create HolmesGPT SDK Config
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

        # DD-WORKFLOW-001 v1.7: Extract detected_labels for workflow matching (100% safe)
        detected_labels_for_toolset = {}
        if enrichment_results:
            if hasattr(enrichment_results, 'detectedLabels') and enrichment_results.detectedLabels:
                dl = enrichment_results.detectedLabels
                detected_labels_for_toolset = dl.model_dump() if hasattr(dl, 'model_dump') else dl.dict() if hasattr(dl, 'dict') else dl
            elif isinstance(enrichment_results, dict):
                dl = enrichment_results.get('detectedLabels', {})
                if dl:
                    detected_labels_for_toolset = dl.model_dump() if hasattr(dl, 'model_dump') else dl.dict() if hasattr(dl, 'dict') else dl

        # DD-WORKFLOW-001 v1.7: Extract source_resource for DetectedLabels validation
        # This is the original signal's resource - compared against LLM's rca_resource
        source_resource = {
            "namespace": request_data.get("resource_namespace", ""),
            "kind": request_data.get("resource_kind", ""),
            "name": request_data.get("resource_name", "")
        }

        # DD-WORKFLOW-001 v1.7: Extract owner_chain from enrichment_results
        # This is the K8s ownership chain from SignalProcessing (via ownerReferences)
        # Format: [{"namespace": "prod", "kind": "ReplicaSet", "name": "..."}, {"kind": "Deployment", ...}]
        # Used for PROVEN relationship validation (100% safe)
        owner_chain = None
        if enrichment_results:
            if hasattr(enrichment_results, 'ownerChain'):
                owner_chain = enrichment_results.ownerChain
            elif isinstance(enrichment_results, dict):
                owner_chain = enrichment_results.get('ownerChain')

        if detected_labels_for_toolset:
            logger.info({
                "event": "detected_labels_extracted",
                "incident_id": incident_id,
                "fields": list(detected_labels_for_toolset.keys()),
                "source_resource": f"{source_resource.get('kind')}/{source_resource.get('namespace') or 'cluster'}",
                "owner_chain_length": len(owner_chain) if owner_chain else 0,
                "message": f"DD-WORKFLOW-001 v1.7: {len(detected_labels_for_toolset)} detected labels (100% safe validation)"
            })

        config = register_workflow_catalog_toolset(
            config,
            app_config,
            remediation_id=remediation_id,
            custom_labels=custom_labels,
            detected_labels=detected_labels_for_toolset,
            source_resource=source_resource,
            owner_chain=owner_chain
        )

        # DD-HAPI-002 v1.2: Create Data Storage client for workflow validation
        data_storage_client = _create_data_storage_client(app_config)

        # BR-AUDIT-005: Get audit store for LLM interaction tracking
        audit_store = get_audit_store()
        remediation_id = request_data.get("remediation_id", "")

        # ========================================
        # LLM SELF-CORRECTION LOOP (DD-HAPI-002 v1.2)
        # With full audit trail (BR-AUDIT-005)
        # ========================================
        validation_errors_history: List[List[str]] = []
        validation_attempts_history: List[Dict[str, Any]] = []  # For response
        result = None

        for attempt in range(MAX_VALIDATION_ATTEMPTS):
            attempt_timestamp = datetime.utcnow().isoformat() + "Z"

            # Build prompt with error feedback for retries
            if validation_errors_history:
                investigation_prompt = base_prompt + _build_validation_error_feedback(
                    validation_errors_history[-1],
                    attempt
                )
            else:
                investigation_prompt = base_prompt

            # Log the prompt
            print("\n" + "="*80)
            print(f"🔍 INCIDENT ANALYSIS PROMPT TO LLM (Attempt {attempt + 1}/{MAX_VALIDATION_ATTEMPTS})")
            print("="*80)
            print(investigation_prompt)
            print("="*80 + "\n")

            # Create investigation request
            investigation_request = InvestigateRequest(
                source="kubernaut",
                title=f"Incident analysis for {request_data.get('signal_type')}",
                description=investigation_prompt,
                subject={
                    "type": "incident",
                    "incident_id": incident_id,
                    "signal_type": request_data.get("signal_type")
                },
                context={
                    "incident_id": incident_id,
                    "issue_type": "incident",
                    "attempt": attempt + 1
                },
                source_instance_id="holmesgpt-api"
            )

            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            # AUDIT: LLM REQUEST (BR-AUDIT-005)
            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            if audit_store:
                audit_store.store_audit(create_llm_request_event(
                    incident_id=incident_id,
                    remediation_id=remediation_id,
                    model=config.model if config else "unknown",
                    prompt=investigation_prompt,
                    toolsets_enabled=list(config.toolsets.keys()) if config and config.toolsets else [],
                    mcp_servers=list(config.mcp_servers.keys()) if config and config.mcp_servers else []
                ))

            # Call HolmesGPT SDK
            logger.info({
                "event": "calling_holmesgpt_sdk",
                "incident_id": incident_id,
                "attempt": attempt + 1,
                "max_attempts": MAX_VALIDATION_ATTEMPTS
            })
            investigation_result = investigate_issues(
                investigate_request=investigation_request,
                dal=dal,
                config=config
            )

            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            # AUDIT: LLM RESPONSE (BR-AUDIT-005)
            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            has_analysis = investigation_result and investigation_result.analysis
            analysis_length = len(investigation_result.analysis) if has_analysis else 0
            analysis_preview = investigation_result.analysis[:500] + "..." if analysis_length > 500 else (investigation_result.analysis if has_analysis else "")
            tool_call_count = len(investigation_result.tool_calls) if investigation_result and hasattr(investigation_result, 'tool_calls') and investigation_result.tool_calls else 0

            if audit_store:
                audit_store.store_audit(create_llm_response_event(
                    incident_id=incident_id,
                    remediation_id=remediation_id,
                    has_analysis=bool(has_analysis),
                    analysis_length=analysis_length,
                    analysis_preview=analysis_preview,
                    tool_call_count=tool_call_count
                ))

                # Audit tool calls if any
                if investigation_result and hasattr(investigation_result, 'tool_calls') and investigation_result.tool_calls:
                    for idx, tool_call in enumerate(investigation_result.tool_calls):
                        tool_name = getattr(tool_call, 'name', 'unknown')
                        tool_arguments = getattr(tool_call, 'arguments', {})
                        tool_result = getattr(tool_call, 'result', None)
                        audit_store.store_audit(create_tool_call_event(
                            incident_id=incident_id,
                            remediation_id=remediation_id,
                            tool_call_index=idx,
                            tool_name=tool_name,
                            tool_arguments=tool_arguments,
                            tool_result=tool_result
                        ))

            # Parse and validate investigation result
            result, validation_result = _parse_and_validate_investigation_result(
                investigation_result,
                request_data,
                owner_chain=owner_chain,
                data_storage_client=data_storage_client
            )

            # Get workflow_id for audit
            workflow_id = result.get("selected_workflow", {}).get("workflow_id") if result.get("selected_workflow") else None

            # Check if validation passed or no workflow to validate
            is_valid = validation_result is None or validation_result.is_valid
            validation_errors = validation_result.errors if validation_result and not validation_result.is_valid else []

            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            # AUDIT: VALIDATION ATTEMPT (BR-AUDIT-005, DD-HAPI-002 v1.2)
            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            if audit_store:
                audit_store.store_audit(create_validation_attempt_event(
                    incident_id=incident_id,
                    remediation_id=remediation_id,
                    attempt=attempt + 1,
                    max_attempts=MAX_VALIDATION_ATTEMPTS,
                    is_valid=is_valid,
                    errors=validation_errors,
                    workflow_id=workflow_id
                ))

            # Build validation attempt record for response (BR-HAPI-197)
            validation_attempts_history.append({
                "attempt": attempt + 1,
                "workflow_id": workflow_id,
                "is_valid": is_valid,
                "errors": validation_errors,
                "timestamp": attempt_timestamp
            })

            if is_valid:
                logger.info({
                    "event": "workflow_validation_passed",
                    "incident_id": incident_id,
                    "attempt": attempt + 1,
                    "has_workflow": result.get("selected_workflow") is not None
                })
                break
            else:
                # Validation failed - log and prepare for retry
                validation_errors_history.append(validation_errors)
                logger.warning({
                    "event": "workflow_validation_retry",
                    "incident_id": incident_id,
                    "attempt": attempt + 1,
                    "max_attempts": MAX_VALIDATION_ATTEMPTS,
                    "errors": validation_errors,
                    "message": "DD-HAPI-002 v1.2: Workflow validation failed, retrying with error feedback"
                })

        # After loop: Check if we exhausted all attempts
        if validation_errors_history and len(validation_errors_history) >= MAX_VALIDATION_ATTEMPTS:
            # All attempts failed - set needs_human_review
            last_errors = validation_errors_history[-1]
            human_review_reason = _determine_human_review_reason(last_errors)
            result["needs_human_review"] = True
            result["human_review_reason"] = human_review_reason

            # Build detailed error summary from ALL attempts for operator notification
            all_errors_summary = []
            for i, errors in enumerate(validation_errors_history):
                all_errors_summary.append(f"Attempt {i+1}: {'; '.join(errors)}")
            result["warnings"].append(
                f"Workflow validation failed after {MAX_VALIDATION_ATTEMPTS} attempts. " +
                " | ".join(all_errors_summary)
            )

            # Final audit with human_review_reason
            if audit_store:
                audit_store.store_audit(create_validation_attempt_event(
                    incident_id=incident_id,
                    remediation_id=remediation_id,
                    attempt=MAX_VALIDATION_ATTEMPTS,
                    max_attempts=MAX_VALIDATION_ATTEMPTS,
                    is_valid=False,
                    errors=last_errors,
                    workflow_id=workflow_id,
                    human_review_reason=human_review_reason
                ))

            logger.warning({
                "event": "workflow_validation_exhausted",
                "incident_id": incident_id,
                "total_attempts": MAX_VALIDATION_ATTEMPTS,
                "all_errors": validation_errors_history,
                "final_errors": last_errors,
                "human_review_reason": human_review_reason,
                "message": "BR-HAPI-197: Max validation attempts exhausted, needs_human_review=True"
            })

        # Add validation history to response (BR-HAPI-197)
        result["validation_attempts_history"] = validation_attempts_history

        logger.info({
            "event": "incident_analysis_completed",
            "incident_id": incident_id,
            "has_workflow": result.get("selected_workflow") is not None,
            "target_in_owner_chain": result.get("target_in_owner_chain", True),
            "warnings_count": len(result.get("warnings", [])),
            "needs_human_review": result.get("needs_human_review", False),
            "validation_attempts": len(validation_errors_history) + 1 if validation_errors_history else 1
        })

        return result

    except Exception as e:
        logger.error({
            "event": "incident_analysis_failed",
            "incident_id": incident_id,
            "error": str(e)
        }, exc_info=True)
        raise


def _create_data_storage_client(app_config: Optional[Dict[str, Any]]):
    """
    Create Data Storage client for workflow validation.

    DD-HAPI-002 v1.2: Client used to validate workflow existence, container image, and parameters.

    Returns:
        DataStorageClient or None if configuration is missing
    """
    try:
        from src.clients.datastorage.client import DataStorageClient

        # Get Data Storage URL from config or environment
        ds_url = None
        if app_config:
            ds_url = app_config.get("data_storage_url") or app_config.get("DATA_STORAGE_URL")
        if not ds_url:
            ds_url = os.getenv("DATA_STORAGE_URL", "http://data-storage:8080")

        return DataStorageClient(base_url=ds_url)
    except Exception as e:
        logger.warning({
            "event": "data_storage_client_creation_failed",
            "error": str(e),
            "message": "Workflow validation will be skipped"
        })
        return None


def _determine_human_review_reason(errors: List[str]) -> str:
    """
    Determine the human_review_reason based on validation errors.

    BR-HAPI-197: Map validation errors to structured reason enum.

    Args:
        errors: List of validation error messages

    Returns:
        HumanReviewReason enum value as string
    """
    error_text = " ".join(errors).lower()

    if "not found" in error_text and "catalog" in error_text:
        return "workflow_not_found"
    elif "mismatch" in error_text or "image" in error_text:
        return "image_mismatch"
    elif "parameter" in error_text or "required" in error_text or "type" in error_text:
        return "parameter_validation_failed"
    else:
        return "parameter_validation_failed"  # Default for validation errors


def _parse_and_validate_investigation_result(
    investigation: InvestigationResult,
    request_data: Dict[str, Any],
    owner_chain: Optional[List[Dict[str, Any]]] = None,
    data_storage_client=None
):
    """
    Parse and validate HolmesGPT investigation result.

    DD-HAPI-002 v1.2: Returns both the parsed result AND the validation result
    so the caller can decide whether to retry with error feedback.

    Args:
        investigation: HolmesGPT investigation result
        request_data: Original request data
        owner_chain: OwnerChain from enrichment results for target validation
        data_storage_client: Data Storage client for workflow validation

    Returns:
        Tuple of (result_dict, validation_result) where validation_result is None
        if no workflow to validate or validation passed.
    """
    from src.validation.workflow_response_validator import WorkflowResponseValidator, ValidationResult

    incident_id = request_data.get("incident_id", "unknown")

    # Extract analysis text
    analysis = investigation.analysis if investigation and investigation.analysis else "No analysis available"

    # Try to parse JSON from analysis
    import json
    import re

    json_match = re.search(r'```json\s*(\{.*?\})\s*```', analysis, re.DOTALL)
    alternative_workflows = []
    selected_workflow = None
    rca = {"summary": "No structured RCA found", "severity": "unknown", "contributing_factors": []}
    confidence = 0.0
    validation_result = None

    if json_match:
        try:
            json_data = json.loads(json_match.group(1))
            rca = json_data.get("root_cause_analysis", {})
            selected_workflow = json_data.get("selected_workflow")
            confidence = selected_workflow.get("confidence", 0.0) if selected_workflow else 0.0

            # Extract alternative workflows (ADR-045 v1.2 - for audit/context only)
            raw_alternatives = json_data.get("alternative_workflows", [])
            for alt in raw_alternatives:
                if isinstance(alt, dict) and alt.get("workflow_id"):
                    alternative_workflows.append({
                        "workflow_id": alt.get("workflow_id", ""),
                        "container_image": alt.get("container_image"),
                        "confidence": float(alt.get("confidence", 0.0)),
                        "rationale": alt.get("rationale", "")
                    })

            # DD-HAPI-002 v1.2: Workflow Response Validation
            # Validates: workflow existence, container image consistency, parameter schema
            if selected_workflow and data_storage_client:
                validator = WorkflowResponseValidator(data_storage_client)
                validation_result = validator.validate(
                    workflow_id=selected_workflow.get("workflow_id", ""),
                    container_image=selected_workflow.get("container_image"),
                    parameters=selected_workflow.get("parameters", {})
                )
                if validation_result.is_valid:
                    # Use validated container image from catalog
                    if validation_result.validated_container_image:
                        selected_workflow["container_image"] = validation_result.validated_container_image
                    validation_result = None  # Clear to indicate success

        except json.JSONDecodeError:
            rca = {"summary": "Failed to parse RCA", "severity": "unknown", "contributing_factors": []}

    # OwnerChain validation (DD-WORKFLOW-001 v1.7, AIAnalysis request Dec 2025)
    target_in_owner_chain = True
    warnings: List[str] = []

    # Check if RCA-identified target is in OwnerChain
    rca_target = rca.get("affectedResource") or rca.get("affected_resource")
    if rca_target and owner_chain:
        target_in_owner_chain = _is_target_in_owner_chain(rca_target, owner_chain, request_data)
        if not target_in_owner_chain:
            warnings.append(
                "Target resource not found in OwnerChain - DetectedLabels may not apply to affected resource"
            )

    # Generate warnings for other conditions (BR-HAPI-197)
    needs_human_review = False
    human_review_reason = None

    if selected_workflow is None:
        warnings.append("No workflows matched the search criteria")
        needs_human_review = True
        human_review_reason = "no_matching_workflows"
    elif confidence < 0.7:
        warnings.append(f"Low confidence selection ({confidence:.0%}) - manual review recommended")
        needs_human_review = True
        human_review_reason = "low_confidence"

    from datetime import datetime
    result = {
        "incident_id": incident_id,
        "analysis": analysis,
        "root_cause_analysis": rca,
        "selected_workflow": selected_workflow,
        "confidence": confidence,
        "timestamp": datetime.utcnow().isoformat() + "Z",
        "target_in_owner_chain": target_in_owner_chain,
        "warnings": warnings,
        "needs_human_review": needs_human_review,
        "human_review_reason": human_review_reason,
        "alternative_workflows": alternative_workflows
    }

    return result, validation_result


def _parse_investigation_result(
    investigation: InvestigationResult,
    request_data: Dict[str, Any],
    owner_chain: Optional[List[Dict[str, Any]]] = None,
    data_storage_client=None
) -> Dict[str, Any]:
    """
    Parse HolmesGPT investigation result into IncidentResponse format.

    DEPRECATED: Use _parse_and_validate_investigation_result for self-correction loop.

    Business Requirement: BR-HAPI-002 (Incident analysis response schema)
    Design Decision: DD-WORKFLOW-001 v1.7 (OwnerChain validation)
    Design Decision: DD-HAPI-002 v1.2 (Workflow Response Validation)

    Args:
        investigation: HolmesGPT investigation result
        request_data: Original request data
        owner_chain: OwnerChain from enrichment results for target validation
        data_storage_client: Optional Data Storage client for workflow validation (DD-HAPI-002 v1.2)
    """
    incident_id = request_data.get("incident_id", "unknown")

    # Extract analysis text
    analysis = investigation.analysis if investigation and investigation.analysis else "No analysis available"

    # Try to parse JSON from analysis
    import json
    import re

    json_match = re.search(r'```json\s*(\{.*?\})\s*```', analysis, re.DOTALL)
    alternative_workflows = []
    if json_match:
        try:
            json_data = json.loads(json_match.group(1))
            rca = json_data.get("root_cause_analysis", {})
            selected_workflow = json_data.get("selected_workflow")
            confidence = selected_workflow.get("confidence", 0.0) if selected_workflow else 0.0

            # Extract alternative workflows (ADR-045 v1.2 - for audit/context only)
            raw_alternatives = json_data.get("alternative_workflows", [])
            for alt in raw_alternatives:
                if isinstance(alt, dict) and alt.get("workflow_id"):
                    alternative_workflows.append({
                        "workflow_id": alt.get("workflow_id", ""),
                        "container_image": alt.get("container_image"),
                        "confidence": float(alt.get("confidence", 0.0)),
                        "rationale": alt.get("rationale", "")
                    })

            # DD-HAPI-002 v1.2: Workflow Response Validation
            # Validates: workflow existence, container image consistency, parameter schema
            if selected_workflow and data_storage_client:
                from src.validation.workflow_response_validator import WorkflowResponseValidator
                validator = WorkflowResponseValidator(data_storage_client)
                validation_result = validator.validate(
                    workflow_id=selected_workflow.get("workflow_id", ""),
                    container_image=selected_workflow.get("container_image"),
                    parameters=selected_workflow.get("parameters", {})
                )
                if not validation_result.is_valid:
                    # Add validation errors as warnings (LLM self-correction would have happened in-session)
                    # If we reach here, it means the LLM failed to provide valid workflow after max attempts
                    workflow_validation_failed = True
                    workflow_validation_errors = validation_result.errors
                    logger.warning({
                        "event": "workflow_validation_failed",
                        "incident_id": request_data.get("incident_id", "unknown"),
                        "workflow_id": selected_workflow.get("workflow_id"),
                        "errors": validation_result.errors,
                        "message": "DD-HAPI-002 v1.2: Workflow response validation failed"
                    })
                    # Add validation errors to workflow for transparency
                    selected_workflow["validation_errors"] = validation_result.errors
                else:
                    # Use validated container image from catalog
                    if validation_result.validated_container_image:
                        selected_workflow["container_image"] = validation_result.validated_container_image
                        logger.debug({
                            "event": "workflow_validation_passed",
                            "incident_id": request_data.get("incident_id", "unknown"),
                            "workflow_id": selected_workflow.get("workflow_id"),
                            "container_image": validation_result.validated_container_image
                        })
        except json.JSONDecodeError:
            rca = {"summary": "Failed to parse RCA", "severity": "unknown", "contributing_factors": []}
            selected_workflow = None
            confidence = 0.0
    else:
        rca = {"summary": "No structured RCA found", "severity": "unknown", "contributing_factors": []}
        selected_workflow = None
        confidence = 0.0

    # OwnerChain validation (DD-WORKFLOW-001 v1.7, AIAnalysis request Dec 2025)
    target_in_owner_chain = True
    warnings: List[str] = []

    # DD-HAPI-002 v1.2: Workflow validation tracking
    workflow_validation_failed = False
    workflow_validation_errors: List[str] = []

    # Check if RCA-identified target is in OwnerChain
    rca_target = rca.get("affectedResource") or rca.get("affected_resource")
    if rca_target and owner_chain:
        target_in_owner_chain = _is_target_in_owner_chain(rca_target, owner_chain, request_data)
        if not target_in_owner_chain:
            warnings.append(
                "Target resource not found in OwnerChain - DetectedLabels may not apply to affected resource"
            )
            logger.warning({
                "event": "target_not_in_owner_chain",
                "incident_id": incident_id,
                "rca_target": rca_target,
                "owner_chain_length": len(owner_chain),
                "message": "DD-WORKFLOW-001 v1.7: RCA target not in OwnerChain, DetectedLabels may be from different scope"
            })

    # Generate warnings for other conditions (BR-HAPI-197)
    needs_human_review = False
    human_review_reason = None

    if selected_workflow is None:
        warnings.append("No workflows matched the search criteria")
        needs_human_review = True
        human_review_reason = "no_matching_workflows"
    elif confidence < 0.7:
        warnings.append(f"Low confidence selection ({confidence:.0%}) - manual review recommended")
        needs_human_review = True
        human_review_reason = "low_confidence"

    # DD-HAPI-002 v1.2: Set needs_human_review if workflow validation failed
    if workflow_validation_failed:
        warnings.append(f"Workflow validation failed: {'; '.join(workflow_validation_errors)}")
        needs_human_review = True
        # Determine specific reason from validation errors
        error_text = " ".join(workflow_validation_errors).lower()
        if "not found in catalog" in error_text:
            human_review_reason = "workflow_not_found"
        elif "mismatch" in error_text:
            human_review_reason = "image_mismatch"
        else:
            human_review_reason = "parameter_validation_failed"

    return {
        "incident_id": incident_id,
        "analysis": analysis,
        "root_cause_analysis": rca,
        "selected_workflow": selected_workflow,
        "confidence": confidence,
        "timestamp": datetime.utcnow().isoformat() + "Z",
        "target_in_owner_chain": target_in_owner_chain,
        "warnings": warnings,
        # DD-HAPI-002 v1.2, BR-HAPI-197: Human review flag and structured reason
        "needs_human_review": needs_human_review,
        "human_review_reason": human_review_reason,
        # Alternative workflows for audit/context (ADR-045 v1.2)
        # IMPORTANT: These are for INFORMATIONAL purposes only - NOT for automatic execution
        "alternative_workflows": alternative_workflows
    }


def _is_target_in_owner_chain(
    rca_target: Dict[str, Any],
    owner_chain: List[Dict[str, Any]],
    request_data: Dict[str, Any]
) -> bool:
    """
    Check if RCA-identified target resource is in the OwnerChain.

    DD-WORKFLOW-001 v1.7: OwnerChain validation ensures DetectedLabels
    are applicable to the actual affected resource.

    Args:
        rca_target: The resource identified by RCA (kind, name, namespace)
        owner_chain: List of owner resources from enrichment
        request_data: Original request for source resource comparison

    Returns:
        True if target is in OwnerChain or is the source resource, False otherwise
    """
    # Extract target identifiers
    target_kind = rca_target.get("kind", "").lower()
    target_name = rca_target.get("name", "").lower()
    target_namespace = rca_target.get("namespace", "").lower()

    # Check if target matches the source resource (always valid)
    source_kind = request_data.get("resource_kind", "").lower()
    source_name = request_data.get("resource_name", "").lower()
    source_namespace = request_data.get("resource_namespace", "").lower()

    if target_kind == source_kind and target_name == source_name:
        if not target_namespace or target_namespace == source_namespace:
            return True

    # Check if target is in OwnerChain
    for owner in owner_chain:
        owner_kind = owner.get("kind", "").lower()
        owner_name = owner.get("name", "").lower()
        owner_namespace = owner.get("namespace", "").lower()

        if target_kind == owner_kind and target_name == owner_name:
            if not target_namespace or target_namespace == owner_namespace:
                return True

    return False


@router.post("/incident/analyze", status_code=status.HTTP_200_OK, response_model=IncidentResponse)
async def incident_analyze_endpoint(request: IncidentRequest) -> IncidentResponse:
    """
    Analyze initial incident and provide RCA + workflow selection

    Business Requirement: BR-HAPI-002 (Incident analysis endpoint)
    Business Requirement: BR-WORKFLOW-001 (MCP Workflow Integration)

    Called by: AIAnalysis Controller (for initial incident RCA and workflow selection)
    """
    request_data = request.model_dump() if hasattr(request, 'model_dump') else request.dict()
    result = await analyze_incident(request_data)
    return result
