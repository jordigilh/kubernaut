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
Incident Analysis Prompt Builder

Business Requirements: BR-HAPI-002 (Incident Analysis)
Design Decisions:
- DD-RECOVERY-003 (DetectedLabels for workflow filtering)
- DD-WORKFLOW-001 v2.1 (Honor failedDetections)
- DD-HAPI-002 v1.2 (LLM Self-Correction feedback)

This module handles all prompt construction for LLM incident analysis,
including cluster context, MCP filter instructions, and validation error feedback.
"""

import json
from typing import Dict, Any, List

from src.models.incident_models import DetectedLabels
from .constants import (
    MAX_VALIDATION_ATTEMPTS,
    PRIORITY_DESCRIPTIONS,
    RISK_GUIDANCE
)


def build_cluster_context_section(detected_labels: DetectedLabels) -> str:
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
    failed_fields = set(detected_labels.failedDetections)

    sections = []

    # GitOps - skip if gitOpsManaged detection failed
    if 'gitOpsManaged' not in failed_fields and detected_labels.gitOpsManaged:
        tool = detected_labels.gitOpsTool or "unknown"
        sections.append(f"This namespace is managed by GitOps ({tool}). "
                       "DO NOT make direct changes - recommend GitOps-aware workflows.")

    # Protection - skip if respective detection failed
    if 'pdbProtected' not in failed_fields and detected_labels.pdbProtected:
        sections.append("A PodDisruptionBudget protects this workload. "
                       "Workflows must respect PDB constraints.")

    if 'hpaEnabled' not in failed_fields and detected_labels.hpaEnabled:
        sections.append("HorizontalPodAutoscaler is active. "
                       "Manual scaling may conflict with HPA - prefer HPA-aware workflows.")

    # Workload type - skip if respective detection failed
    if 'stateful' not in failed_fields and detected_labels.stateful:
        sections.append("This is a STATEFUL workload (StatefulSet or has PVCs). "
                       "Use stateful-aware remediation workflows.")

    if 'helmManaged' not in failed_fields and detected_labels.helmManaged:
        sections.append("This resource is managed by Helm. "
                       "Consider Helm-compatible workflows.")

    # Security - skip if respective detection failed
    if 'networkIsolated' not in failed_fields and detected_labels.networkIsolated:
        sections.append("NetworkPolicy restricts traffic in this namespace. "
                       "Workflows may need network exceptions.")

    # DD-WORKFLOW-001 v2.2: podSecurityLevel REMOVED (PSP deprecated, PSS is namespace-level)

    if 'serviceMesh' not in failed_fields:
        mesh = detected_labels.serviceMesh
        if mesh:
            sections.append(f"Service mesh ({mesh}) is present. "
                           "Consider service mesh-aware workflows.")

    return "\n".join(sections) if sections else "No special cluster characteristics detected."


def build_mcp_filter_instructions(detected_labels: DetectedLabels) -> str:
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

    # DD-WORKFLOW-001 v2.1: Get fields where detection failed
    failed_fields = set(detected_labels.failedDetections)

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

        value = getattr(detected_labels, label_field, None)
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


def build_validation_error_feedback(errors: List[str], attempt: int) -> str:
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


def create_incident_investigation_prompt(request_data: Dict[str, Any]) -> str:
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

    # DetectedLabels from enrichment_results (DD-RECOVERY-003)
    enrichment_results = request_data.get('enrichment_results', {})
    detected_labels = None
    if enrichment_results:
        # Handle both dict and EnrichmentResults model
        if hasattr(enrichment_results, 'detectedLabels'):
            dl = enrichment_results.detectedLabels
            if dl:
                # If it's already a DetectedLabels model, use it directly
                if isinstance(dl, DetectedLabels):
                    detected_labels = dl
                # Otherwise convert dict to DetectedLabels model
                elif isinstance(dl, dict):
                    detected_labels = DetectedLabels(**dl)
        elif isinstance(enrichment_results, dict):
            dl = enrichment_results.get('detectedLabels', {})
            if dl:
                # Convert dict to DetectedLabels model
                if isinstance(dl, dict):
                    detected_labels = DetectedLabels(**dl)
                # If it's already a DetectedLabels model, use it directly
                elif isinstance(dl, DetectedLabels):
                    detected_labels = dl

    # Generate contextual descriptions
    priority_desc = PRIORITY_DESCRIPTIONS.get(priority, f"{priority} - Standard priority").format(business_category=business_category)
    risk_desc = RISK_GUIDANCE.get(risk_tolerance, f"{risk_tolerance} risk tolerance")

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

{build_cluster_context_section(detected_labels)}

{build_mcp_filter_instructions(detected_labels)}

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

## Special Investigation Outcomes (BR-HAPI-200)

**IMPORTANT**: Not all investigations result in a workflow recommendation. Handle these cases explicitly:

### Outcome A: Problem Self-Resolved (No Remediation Needed)

If your investigation confirms the problem has **already resolved** (e.g., pod recovered, resource pressure normalized):

# root_cause_analysis
{{"summary": "Problem self-resolved. [Describe what you found]", "severity": "low", "contributing_factors": ["Transient condition", "Auto-recovery"]}}

# confidence
0.85

# selected_workflow
None

# investigation_outcome
resolved

**When to use**: High confidence (≥0.7) that the problem is resolved:
- Pod status shows Running/Ready after previous OOMKilled
- Resource metrics normalized (CPU/memory within limits)
- Error rate returned to baseline
- Node conditions cleared

### Outcome B: Investigation Inconclusive (Human Review Required)

If your investigation **cannot determine** the root cause or current state:

# root_cause_analysis
{{"summary": "Unable to determine root cause. [Describe ambiguity]", "severity": "unknown", "contributing_factors": ["Insufficient data", "Conflicting signals"]}}

# confidence
0.3

# selected_workflow
None

# investigation_outcome
inconclusive

**When to use**: Low confidence (<0.5) due to:
- Metrics/events unavailable or stale
- Conflicting information from different sources
- Resource state ambiguous (neither clearly healthy nor clearly failing)
- Cannot reproduce or verify the reported condition
- Insufficient access to relevant data

**DO NOT** guess or hallucinate when uncertain. Return `investigation_outcome: "inconclusive"` instead.

## RCA Severity Assessment

After your investigation, assess the severity of the root cause using these levels.

**IMPORTANT**: Your RCA severity may differ from the input signal severity. Use your analysis to determine the actual severity based on business impact.

### Severity Levels (DD-SEVERITY-001):

**critical** - Immediate remediation required
- Production service completely unavailable
- Data loss or corruption occurring
- Security breach actively exploited
- SLA violation in progress
- Revenue-impacting outage
- Affects >50% of users
- High error rate (>10% of requests failing)

**warning** - Remediation needed
- Significant service degradation (>50% performance loss)
- Moderate error rate (1-10% of requests failing)
- Production issue that needs attention
- Affects 10-50% of users
- SLA at risk
- Non-production critical issues

**info** - Remediation recommended or informational
- Minor service degradation (<50% performance loss)
- Low error rate (<1% of requests failing)
- Development/staging environment issues
- Affects <10% of users
- Optimization opportunities
- Capacity planning alerts
- No immediate user impact

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

**CRITICAL**: Use section header format (NOT a single JSON block) to ensure all fields are preserved:

### Part 1: Natural Language Analysis

Explain your investigation findings, root cause analysis, and reasoning for workflow selection.

### Part 2: Structured Data (Section Header Format)

**REQUIRED FORMAT** - Each field must be on its own line with section header:

# root_cause_analysis
{{"summary": "Brief summary of root cause", "severity": "critical|high|medium|low", "contributing_factors": ["factor1", "factor2"]}}

# confidence
0.95

# selected_workflow
{{"workflow_id": "workflow-id-from-mcp-search-results", "version": "1.0.0", "confidence": 0.95, "rationale": "Why this workflow was selected", "parameters": {{"PARAM_NAME": "value"}}}}

# alternative_workflows
[{{"workflow_id": "alt-workflow-id", "container_image": "image:tag", "confidence": 0.75, "rationale": "Why this was considered but not selected"}}]

**IMPORTANT**:
- **DO NOT** use a single ```json block - use section headers as shown above
- Select ONE workflow per incident as `selected_workflow`
- Include up to 2-3 `alternative_workflows` (for AUDIT/CONTEXT only)
- Each field must have its own `# field_name` header
- If a field is not applicable, use `None` or `[]` as the value
- Alternative workflows help operators understand what options were considered
- Populate ALL required parameters from the workflow schema
- Use your RCA findings to determine parameter values
"""

    return prompt

