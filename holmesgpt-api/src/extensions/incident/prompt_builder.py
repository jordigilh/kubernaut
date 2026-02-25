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
Incident Analysis Prompt Builder

Business Requirements: BR-HAPI-002 (Incident Analysis)
Design Decisions:
- DD-HAPI-001 (DetectedLabels for workflow filtering)
- DD-WORKFLOW-001 v2.1 (Honor failedDetections)
- DD-HAPI-002 v1.2 (LLM Self-Correction feedback)

This module handles all prompt construction for LLM incident analysis,
including cluster context, MCP filter instructions, and validation error feedback.
"""

import json
from typing import Dict, Any, List, Optional

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

    Design Decision: DD-HAPI-001, DD-WORKFLOW-001 v2.1

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
    Build workflow discovery filter instructions based on DetectedLabels.

    Design Decision: DD-HAPI-001, DD-WORKFLOW-001 v2.1

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

    # Build conditional guidance based on which labels are actually present in filters
    # DD-WORKFLOW-001 v2.1: Only mention labels that weren't excluded by failedDetections
    guidance_lines = []
    if "gitops_managed" in filters:
        guidance_lines.append("If `gitOpsManaged=true`, prioritize workflows with `gitops_aware=true` tag.")
    if "stateful" in filters:
        guidance_lines.append("If `stateful=true`, prefer stateful-aware workflows.")
    if "pdb_protected" in filters:
        guidance_lines.append("If `pdb_protected=true`, ensure the selected workflow respects PodDisruptionBudget constraints.")

    guidance_text = "\n".join(guidance_lines) if guidance_lines else "Use the detected characteristics to guide workflow selection."

    return f"""
### Workflow Discovery Context (DetectedLabels)

The following detected labels describe the target cluster environment.
Use these to inform your workflow selection reasoning — prefer workflows
that are compatible with the detected environment characteristics.

Detected environment characteristics:
```json
{json.dumps(filters, indent=4)}
```

**IMPORTANT**: {guidance_text}
"""


def build_validation_error_feedback(
    errors: List[str], attempt: int, schema_hint: Optional[str] = None
) -> str:
    """
    Build validation error feedback to append to LLM prompt for self-correction.

    Design Decision: DD-HAPI-002 v1.2 (Workflow Response Validation)
    Business Requirement: BR-HAPI-197 (needs_human_review field), BR-HAPI-191 (Parameter Schema)

    This feedback is appended to the conversation when the LLM's workflow response
    fails validation. The LLM uses this to self-correct while context is preserved.

    Args:
        errors: List of validation error messages
        attempt: Current attempt number (0-indexed)
        schema_hint: Optional parameter schema hint for LLM self-correction (BR-HAPI-191).
                     When present, includes the expected parameter names and types so the
                     LLM can correct parameter mismatches.

    Returns:
        Formatted error feedback section for the prompt
    """
    attempt_display = attempt + 1  # Convert to 1-indexed for display
    errors_list = "\n".join(f"- {error}" for error in errors)

    # BR-HAPI-191: Include parameter schema hint when available
    schema_section = ""
    if schema_hint:
        schema_section = f"""

**Expected Parameter Schema:**
```
{schema_hint}
```
Use the parameter names, types, and constraints shown above when correcting your response.
"""

    return f"""

## ⚠️ VALIDATION ERROR - CORRECTION REQUIRED (Attempt {attempt_display}/{MAX_VALIDATION_ATTEMPTS})

Your previous workflow response had validation errors:

{errors_list}
{schema_section}
**Please correct your response:**
1. Re-check the workflow ID exists (use get_workflow with the workflow_id)
2. Ensure execution_bundle matches the catalog exactly (or omit to use catalog default)
3. Verify all required parameters are provided with correct types and values

**Re-submit your JSON response with the corrected workflow selection.**
"""


def create_incident_investigation_prompt(
    request_data: Dict[str, Any],
    remediation_history_context: Optional[Dict[str, Any]] = None,
) -> str:
    """
    Create investigation prompt for initial incident analysis (ADR-041 v3.3).

    Used by: /incident/analyze endpoint
    Input: IncidentRequest model data
    Reference: ADR-041 v3.3 - LLM Prompt and Response Contract

    Args:
        request_data: IncidentRequest model data.
        remediation_history_context: Optional remediation history context from
            DataStorage (BR-HAPI-016, DD-HAPI-016 v1.1). When provided, a
            remediation history section is appended to the prompt to inform
            the LLM about past remediations for the affected resource.
    """
    # Extract fields from IncidentRequest
    signal_name = request_data.get("signal_name", "Unknown")
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

    # BR-AI-084: Signal mode determines investigation strategy (ADR-054)
    # "reactive" (default): RCA - incident has occurred
    # "predictive": Predict & prevent - incident predicted but not yet occurred
    signal_mode = request_data.get('signal_mode') or 'reactive'

    # ADR-056: DetectedLabels are no longer extracted from enrichment_results
    # for prompt construction. They are computed at runtime by HAPI's
    # get_resource_context tool and used via session_state.

    # Generate contextual descriptions
    priority_desc = PRIORITY_DESCRIPTIONS.get(priority, f"{priority} - Standard priority").format(business_category=business_category)
    risk_desc = RISK_GUIDANCE.get(risk_tolerance, f"{risk_tolerance} risk tolerance")

    # Build incident summary with natural language
    # BR-AI-084: Adapt summary based on signal mode
    if signal_mode == "predictive":
        incident_summary = f"A **{severity} {signal_name} event** from **{signal_source}** has been **predicted** for **{namespace}/{resource_kind}/{resource_name}**. This incident has NOT yet occurred — it is based on resource trend analysis (e.g., Prometheus predict_linear())."
    else:
        incident_summary = f"A **{severity} {signal_name} event** from **{signal_source}** has occurred in the **{namespace}/{resource_kind}/{resource_name}**."

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
- Signal Name: {signal_name}
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

    # BR-AI-084: Add predictive signal mode context if applicable
    if signal_mode == "predictive":
        prompt += f"""
## Predictive Signal Mode (ADR-054)

**IMPORTANT**: This is a PREDICTIVE signal. The incident has NOT yet occurred.

This signal was generated by resource trend analysis (e.g., Prometheus `predict_linear()`) predicting
that a **{signal_name}** event will occur for **{namespace}/{resource_kind}/{resource_name}**.

**Your task is DIFFERENT from a reactive investigation**:
1. Assess current resource utilization trends
2. Evaluate recent deployments and configuration changes
3. Determine if the prediction is likely to materialize
4. Recommend PREVENTIVE action if warranted
5. **"No action needed" is a valid outcome** if the prediction is unlikely to materialize

**Investigation focus**: Current state assessment and prevention, NOT root cause analysis of a past event.
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

    # ADR-056: Cluster environment characteristics are now computed at runtime
    # by the get_resource_context tool (LabelDetector) and injected into
    # DataStorage queries via session_state. No longer included in the prompt.

    # Issue #198: PDB-specific guidance for KubePodDisruptionBudgetAtLimit signals
    pdb_signal_guidance = ""
    if signal_name and "PodDisruptionBudget" in signal_name:
        pdb_signal_guidance = """
## PDB-Specific Investigation Guidance (Issue #198)

**IMPORTANT**: This signal indicates a PodDisruptionBudget is blocking voluntary
disruptions (drain/eviction). Before concluding this is a taint or scheduling issue:

1. **Inspect the PDB spec**: Call `get_resource_context` for the PDB itself
   (kind=PodDisruptionBudget) to understand its selector and minAvailable/maxUnavailable.
2. **Check matched pods**: The PDB's selector identifies which pods it protects.
   Verify the replica count vs minAvailable constraint.
3. **Prefer RelaxPDB over RemoveTaint**: A drained node with SchedulingDisabled is a
   SYMPTOM of the PDB blocking eviction, not the root cause. The root cause is the
   PDB constraint preventing the eviction needed for the drain to complete.
4. **RCA target**: The affected resource should be the PDB, not the Node.
"""

    if pdb_signal_guidance:
        prompt += pdb_signal_guidance

    # BR-HAPI-016: Add remediation history section if context is available
    if remediation_history_context:
        from extensions.remediation_history_prompt import build_remediation_history_section

        history_section = build_remediation_history_section(remediation_history_context)
        if history_section:
            prompt += f"""
## Remediation History Context (AUTO-DETECTED)

{history_section}

"""

    # Add Business Context section
    prompt += f"""
## Business Context (AUTO-INJECTED INTO WORKFLOW DISCOVERY)

**IMPORTANT**: These fields are automatically injected into all workflow discovery tool calls. They are for context only — you do NOT need to pass them.

- Environment: {environment}

- Priority: {priority}

- Business Category: {business_category}

- Risk Tolerance: {risk_tolerance}

**Note**: These business context fields are automatically included as signal context filters
in all workflow discovery tool calls. You do NOT need to pass them manually.

## Your Investigation Workflow

**CRITICAL**: Follow this sequence in order. Do NOT search for workflows before investigating.

### Phase 1: Investigate the {'Predicted Incident' if signal_mode == 'predictive' else 'Incident'}
{'**PREDICTIVE MODE**: This incident is predicted based on resource trend analysis but has NOT yet occurred.' if signal_mode == 'predictive' else ''}
Use available tools to investigate the {'predicted incident' if signal_mode == 'predictive' else 'incident'}:
- Check pod status, events, and logs (kubectl)
- Review resource usage and limits
- Examine node conditions
- Analyze metrics from signal source (if prometheus)
{'- Assess resource utilization trends and recent deployments' if signal_mode == 'predictive' else ''}
{'- Determine if the prediction is likely to materialize based on current state' if signal_mode == 'predictive' else ''}

**Goal**: {'Evaluate current environment. Assess resource utilization trends, recent deployments, and current state to determine if preemptive action is warranted and how to prevent this incident.' if signal_mode == 'predictive' else 'Understand what actually happened and why.'}

**Input Signal Provided**: {signal_name} (starting point for investigation)

### Phase 2: {'Assess Prediction and Determine Prevention Strategy' if signal_mode == 'predictive' else 'Determine Root Cause (RCA)'}
{'Based on your investigation findings, determine if the predicted incident is likely to occur and identify preventive actions. "No action needed" is a valid outcome if the prediction is unlikely to materialize.' if signal_mode == 'predictive' else 'Based on your investigation findings, identify the root cause.\nIs the input signal the root cause, or just a symptom?'}

### Phase 3: Identify Signal Name That Describes the Effect
Based on your RCA, determine the signal_name that best describes the effect:

**If investigation confirms input signal is the root cause**:
- Input: OOMKilled → Investigation confirms memory limit exceeded → Use "OOMKilled"

**If investigation reveals different root cause**:
- Input: OOMKilled → Investigation shows node memory pressure → Use "NodePressure" or "Evicted"

**Important**: The signal_name for workflow search comes from YOUR investigation findings, not the input signal.

### Phase 3b: Identify the Affected Resource (MANDATORY for remediation)

Determine the resource that the remediation should target:
- Call `get_resource_context` with the resource you identified during RCA (kind, name, namespace).
- The tool resolves the **root managing resource** (e.g., for a Pod it finds the managing Deployment) and returns:
  - `root_owner`: The root managing resource (`kind`, `name`, `namespace`). Use this as your `affectedResource`.
  - `remediation_history`: Past remediations for that resource. Use this to avoid repeating recently failed workflows.
- Set `affectedResource` in `root_cause_analysis` to the `root_owner` from the tool response.
- **Example**: You call the tool for Pod "api-xyz-abc". The tool returns `root_owner: {{kind: Deployment, name: api, namespace: prod}}`. Your `affectedResource` is the Deployment, not the Pod.

### Phase 4: Discover and Select Workflow (MANDATORY - Three-Step Protocol)
**YOU MUST** follow this three-step workflow discovery protocol:

**Step 1**: Call `list_available_actions` to discover available remediation action types.
Review all returned action types and their descriptions. Choose the action type that
best matches your RCA findings.

**Step 2**: Call `list_workflows` with `action_type` set to your chosen action type.
**CRITICAL**: If `pagination.hasMore` is true, call again with increased `offset` to
review ALL workflows. Compare workflow descriptions, version notes, and suitability
for your RCA findings across ALL workflows before selecting.

**Step 3**: Call `get_workflow` with the `workflow_id` of your selected workflow to
retrieve its full parameter schema. If you get "not found", go back to Step 2
and choose a different workflow.

**This step is REQUIRED** - you cannot skip workflow discovery. If the tools are available, you must invoke them.

### Phase 5: Return Summary + JSON Payload
{'Provide natural language summary of your prediction assessment + structured JSON with preventive workflow and parameters. Include whether preemptive action is recommended or if the prediction is unlikely to materialize.' if signal_mode == 'predictive' else 'Provide natural language summary + structured JSON with workflow and parameters.'}

**If workflow discovery succeeds**:
```json
{{
  "root_cause_analysis": {{
    "summary": "Brief summary of root cause from investigation",
    "severity": "critical|high|medium|low|unknown",
    "contributing_factors": ["factor1", "factor2"],
    "affectedResource": {{
      "kind": "Deployment",
      "name": "resource-name",
      "namespace": "resource-namespace"
    }}
  }},
  "selected_workflow": {{
    "workflow_id": "workflow-id-from-mcp-search",
    "action_type": "ScaleReplicas",
    "version": "1.0.0",
    "confidence": 0.95,
    "rationale": "Why your RCA findings led to this workflow selection",
    "execution_engine": "tekton|job",
    "parameters": {{
      "PARAM_NAME": "value-from-investigation"
    }}
  }},
  "alternative_workflows": [
    {{
      "workflow_id": "other-workflow-id",
      "execution_bundle": "image@sha256:digest",
      "confidence": 0.75,
      "rationale": "Why this was considered but not selected"
    }}
  ]
}}
```

**CRITICAL**: The `affectedResource` field in `root_cause_analysis` is **REQUIRED**.
It identifies the Kubernetes resource that the remediation workflow will act upon.
- **kind**: The root owner resource type (e.g., "Deployment", "StatefulSet"). **NEVER use "Pod"** — always trace up the OwnerReferences chain to the root owner.
- **name**: The root owner resource name (e.g., if the Pod is `memory-eater-abc-123`, the Deployment is `memory-eater`)
- **namespace**: The namespace where the resource lives

**Example**: For an OOMKilled Pod named `memory-eater-7f86bb8877-4hv68` in namespace `production`:
```json
"affectedResource": {{"kind": "Deployment", "name": "memory-eater", "namespace": "production"}}
```

**If workflow discovery fails or returns no workflows**:
```json
{{
  "root_cause_analysis": {{
    "summary": "Root cause from investigation",
    "severity": "critical|high|medium|low|unknown",
    "contributing_factors": ["factor1", "factor2"],
    "affectedResource": {{
      "kind": "Deployment",
      "name": "resource-name",
      "namespace": "resource-namespace"
    }}
  }},
  "selected_workflow": null,
  "rationale": "Workflow discovery failed: [error details]. RCA completed but workflow selection unavailable."
}}
```

## Special Investigation Outcomes (BR-HAPI-200)

**IMPORTANT**: Not all investigations result in a workflow recommendation. Handle these cases explicitly.

**CRITICAL RULE**: `investigation_outcome: resolved` means the problem is **no longer occurring** and the resource is **currently healthy RIGHT NOW**. If the problem is still happening (e.g., pod is still in CrashLoopBackOff, OOMKilled, or Error state), you MUST NOT use `resolved`. Understanding the root cause is NOT the same as the problem being resolved.

### Outcome A: Problem Self-Resolved (No Remediation Needed)

If your investigation confirms the problem has **already resolved on its own** and the resource is **currently healthy**:

# root_cause_analysis
{{"summary": "Problem self-resolved. [Describe what you found]", "severity": "low", "contributing_factors": ["Transient condition", "Auto-recovery"]}}

# confidence
0.85

# selected_workflow
None

# investigation_outcome
resolved

**When to use**: High confidence (>=0.7) that the problem is **no longer occurring**:
- Pod status is **currently** Running/Ready (not CrashLoopBackOff, not OOMKilled)
- Resource metrics are **currently** within normal limits
- Error rate has **already** returned to baseline
- The issue was transient and self-corrected without intervention

**When NOT to use**: Do NOT use `resolved` if:
- The pod is still crashing, OOMKilling, or in an error state
- You identified the root cause but the problem persists
- The issue requires manual intervention or a configuration change to fix

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

### Outcome C: Problem Identified, No Automated Remediation Available

If your investigation **successfully identifies** the root cause, the problem is **still occurring**, but no workflow matched in the catalog search:

# root_cause_analysis
{{"summary": "[Describe the identified root cause]", "severity": "[appropriate severity]", "contributing_factors": ["[specific factors]"], "affectedResource": {{"kind": "[root owner kind, e.g. Deployment]", "name": "[root owner name]", "namespace": "[namespace]"}}}}

# confidence
[your confidence in the RCA, typically >=0.7]

# selected_workflow
None

Do NOT include `investigation_outcome` in this case. The system will automatically flag this for human review.

**When to use**:
- You clearly identified the root cause (high confidence)
- The problem is still active (pod still crashing, resource still unhealthy)
- The workflow discovery returned no matching workflows
- Human intervention is needed to resolve the issue

## RCA Severity Assessment

After your investigation, assess the severity of the root cause using these levels.

**IMPORTANT**: Your RCA severity may differ from the input signal severity. Use your analysis to determine the actual severity based on business impact.

### Severity Levels (BR-SEVERITY-001, DD-SEVERITY-001 v1.1):

**critical** - Immediate remediation required
- Production service completely unavailable
- Data loss or corruption occurring
- Security breach actively exploited
- SLA violation in progress
- Revenue-impacting outage
- Affects >50% of users
- Example: All replicas of a Deployment are in CrashLoopBackOff — zero available endpoints, requests fail with 503

**high** - Urgent remediation needed
- Significant service degradation (>50% performance loss)
- High error rate (>10% of requests failing)
- Production issue escalating toward critical
- Affects 10-50% of users
- SLA at risk
- Example: 1 of 3 replicas OOMKilled and restarting — service degraded, another failure would cause outage

**medium** - Remediation recommended
- Minor service degradation (<50% performance loss)
- Moderate error rate (1-10% of requests failing)
- Non-production critical issues
- Affects <10% of users
- Staging/development critical issues
- Example: 1 of 5 replicas in CrashLoopBackOff — 4 healthy replicas handle load, but headroom is reduced

**low** - Remediation optional
- Informational issues
- Optimization opportunities
- Development environment issues
- No user impact
- Capacity planning alerts
- Example: Pod is over-provisioned (using 40% of CPU request with 10x limit) — no impact, wasted capacity

**unknown** - Human triage required
- Root cause could not be determined
- Conflicting signals prevent confident assessment
- Insufficient monitoring data or logs to evaluate impact
- Novel condition with no precedent in the system
- Example: Pod in CrashLoopBackOff but container logs are empty and no events provide context

## Workflow Discovery Guidance (Three-Step Protocol)

You have three workflow discovery tools available. Use them in order:

1. **`list_available_actions`** — Discover what remediation action types are available.
   Each action type includes guidance on when to use it and when not to.

2. **`list_workflows`** with `action_type` — List specific workflows for your chosen action.
   Review ALL pages (if `hasMore=true`, call again with increased `offset`).
   Compare workflow descriptions and suitability for your RCA findings.

3. **`get_workflow`** with `workflow_id` — Get the full workflow with parameter schema.
   If "not found", the workflow doesn't match your signal context — choose another.

**Signal context filters** (severity, component, environment, priority, custom_labels,
detected_labels) are automatically included in all discovery calls. You do NOT need to
provide them manually — they are injected by the system.

**Canonical Signal Types** (for reference during RCA):
- `OOMKilled`, `CrashLoopBackOff`, `ImagePullBackOff`, `Evicted`
- `NodeNotReady`, `PodPending`, `FailedScheduling`
- `BackoffLimitExceeded`, `DeadlineExceeded`, `FailedMount`

Use any canonical Kubernetes event reason that matches your RCA findings.
For complete list, see: https://kubernetes.io/docs/reference/kubernetes-api/cluster-resources/event-v1/#Event

## Expected Response Format

**CRITICAL**: Use section header format (NOT a single JSON block) to ensure all fields are preserved:

### Part 1: Natural Language Analysis

Explain your investigation findings, root cause analysis, and reasoning for workflow selection.

### Part 2: Structured Data (Section Header Format)

**REQUIRED FORMAT** - Each field must be on its own line with section header:

# root_cause_analysis
{{"summary": "Brief summary of root cause", "severity": "critical|high|medium|low|unknown", "contributing_factors": ["factor1", "factor2"], "affectedResource": {{"kind": "Deployment", "name": "the-deployment-name", "namespace": "the-namespace"}}}}

# confidence
0.95

# selected_workflow
{{"workflow_id": "workflow-id-from-mcp-search-results", "action_type": "ScaleReplicas", "version": "1.0.0", "confidence": 0.95, "rationale": "Why this workflow was selected", "execution_engine": "tekton", "parameters": {{"PARAM_NAME": "value"}}}}

# alternative_workflows
[{{"workflow_id": "alt-workflow-id", "execution_bundle": "image@sha256:digest", "confidence": 0.75, "rationale": "Why this was considered but not selected"}}]

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

