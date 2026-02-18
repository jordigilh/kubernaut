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
Recovery Analysis Prompt Builder

Business Requirements: BR-HAPI-001 to 050 (Recovery Analysis)
Design Decision: DD-RECOVERY-003 (Recovery Investigation Prompt Structure)

This module contains all prompt-building functions for recovery analysis,
including failure reason guidance, cluster context, and investigation prompts.
"""

import json
from typing import Any, Dict, Optional

from src.models.incident_models import DetectedLabels


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


def _build_cluster_context_section(detected_labels: DetectedLabels) -> str:
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


def _build_mcp_filter_instructions(detected_labels: DetectedLabels) -> str:
    """
    Build workflow discovery filter instructions based on DetectedLabels.

    Design Decision: DD-RECOVERY-003, DD-WORKFLOW-001 v2.1, DD-HAPI-017

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
### Workflow Discovery Context (DetectedLabels)

The following detected labels describe the target cluster environment.
These are automatically included in your workflow discovery tool calls as context filters.
You do NOT need to pass them manually — they are injected by the system.

Detected label filters:
```json
{json.dumps(filters, indent=4)}
```

The Data Storage service uses these labels to return only workflows compatible
with the detected cluster environment.

**IMPORTANT**: If `gitOpsManaged=true`, prioritize workflows with `gitops_aware=true` tag.
"""


def _build_business_context_section(
    environment: str, priority: str, business_category: str, risk_tolerance: str
) -> str:
    """
    Build the auto-injected business context section for LLM prompts.

    Shared by incident and recovery prompt builders to ensure consistency.
    These fields are automatically injected into workflow discovery tool calls.

    Design Decision: DD-HAPI-017
    """
    return f"""## Business Context (AUTO-INJECTED INTO WORKFLOW DISCOVERY)

**IMPORTANT**: These fields are automatically injected into all workflow discovery tool calls. They are for context only — you do NOT need to pass them.

- Environment: {environment}
- Priority: {priority}
- Business Category: {business_category}
- Risk Tolerance: {risk_tolerance}

**Note**: These business context fields are automatically included as signal context filters
in all workflow discovery tool calls. You do NOT need to pass them manually."""


def _build_three_step_protocol_instructions(
    rca_context: str = "your RCA findings",
    exclude_workflow_id: str = None,
) -> str:
    """
    Build the three-step workflow discovery protocol instructions.

    Shared by incident and recovery prompt builders to avoid duplication.
    Parameterized for recovery-specific constraints (workflow exclusion).

    Design Decision: DD-HAPI-017, DD-WORKFLOW-016

    Args:
        rca_context: What the LLM should match against in Step 1.
        exclude_workflow_id: If set, adds constraint to avoid re-selecting this workflow.
    """
    step2_constraint = ""
    closing = ""

    if exclude_workflow_id:
        step2_constraint = (
            f" **Do NOT select the previously failed workflow\n"
            f"(`{exclude_workflow_id}`).**"
        )
        closing = "\n\n**Constraint**: Do NOT select the previously failed workflow."
    else:
        closing = (
            "\n\n**This step is REQUIRED** - you cannot skip workflow discovery. "
            "If the tools are available, you must invoke them."
        )

    return f"""**Step 1**: Call `list_available_actions` to discover available remediation action types.
Review all returned action types and their descriptions. Choose the action type that
best matches {rca_context}.

**Step 2**: Call `list_workflows` with `action_type` set to your chosen action type.
**CRITICAL**: If `pagination.hasMore` is true, call again with increased `offset` to
review ALL workflows. Compare workflow descriptions, version notes, and suitability
for your RCA findings across ALL workflows before selecting.{step2_constraint}

**Step 3**: Call `get_workflow` with the `workflow_id` of your selected workflow to
retrieve its full parameter schema. If you get "not found", go back to Step 2
and choose a different workflow.{closing}"""


def _create_recovery_investigation_prompt(
    request_data: Dict[str, Any],
    remediation_history_context: Optional[Dict[str, Any]] = None,
) -> str:
    """
    Create investigation prompt for recovery analysis.

    Design Decision: DD-RECOVERY-003
    BR-HAPI-016: Remediation history context for LLM prompt enrichment.

    Key Differences from Incident Prompt:
    1. Adds "Previous Remediation Attempt" section at TOP
    2. Includes Kubernetes reason code with specific guidance
    3. Instructs LLM NOT to repeat failed workflow
    4. Expects signal type may have CHANGED

    Args:
        request_data: Recovery request data dict.
        remediation_history_context: Optional remediation history context from
            DataStorage (BR-HAPI-016, DD-HAPI-016 v1.1). When provided, a
            remediation history section is appended to the prompt.
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
- **Execution Bundle**: `{selected_workflow.get('execution_bundle', 'Unknown')}`
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

5. **DISCOVER** alternative workflows using the three-step protocol:
   - Call `list_available_actions` to see what action types are available
   - Call `list_workflows` to browse workflows for the best action type
   - Call `get_workflow` to retrieve the full parameter schema

---

"""

    # Add cluster context section if DetectedLabels are available
    if detected_labels:
        prompt += f"""## Cluster Environment Characteristics (AUTO-DETECTED)

The following characteristics were automatically detected for the target resource.
**These are automatically included as context filters in your workflow discovery tool calls.**

{_build_cluster_context_section(detected_labels)}

{_build_mcp_filter_instructions(detected_labels)}

---

"""

    # Pre-compute three-step protocol instructions for recovery (DD-HAPI-017)
    _failed_workflow_id = selected_workflow.get('workflow_id', 'Unknown')
    three_step_recovery = _build_three_step_protocol_instructions(
        rca_context="your updated RCA findings for the CURRENT state (not the original signal)",
        exclude_workflow_id=_failed_workflow_id,
    )

    # Add current signal context
    prompt += f"""
## Current Signal Context

**Technical Details**:
- Signal Type: {signal_type}
- Severity: {severity}
- Resource: {namespace}/{resource_kind}/{resource_name}
- Error: {error_message}

{_build_business_context_section(environment, priority, business_category, risk_tolerance)}

## Your Investigation Workflow (Recovery Mode)

### Phase 1: Assess Current State
- Check the CURRENT state of the resource (may have changed due to failed workflow)
- Determine if the failure left the system in a degraded state
- Look for side effects from the partial execution

### Phase 2: Re-evaluate Root Cause
- The original RCA was: `{original_rca.get('signal_type', 'Unknown')}`
- Determine if the signal type has CHANGED after the failed workflow
- If changed, use the NEW signal type for workflow search

### Phase 3: Discover and Select Alternative Workflow (MANDATORY - Three-Step Protocol)
**YOU MUST** follow this three-step workflow discovery protocol:

{three_step_recovery}

### Phase 4: Return Recovery Recommendation

**CRITICAL**: Use section header format (NOT a single JSON block) to ensure all fields are preserved.

**If workflow discovery succeeds**:

# recovery_analysis
{{"previous_attempt_assessment": {{"failure_understood": true, "failure_reason_analysis": "Explanation of why previous attempt failed", "state_changed": true, "current_signal_type": "Current signal type"}}, "current_rca": {{"summary": "Updated RCA", "severity": "current severity", "signal_type": "current signal type", "contributing_factors": ["factor1"]}}}}

# confidence
0.85

# selected_workflow
{{"workflow_id": "alternative-workflow-id", "version": "1.0.0", "confidence": 0.85, "rationale": "Why this alternative was selected", "parameters": {{"PARAM_NAME": "value"}}}}

# can_recover
True

**If workflow discovery fails or no workflows found**:

# recovery_analysis
{{"previous_attempt_assessment": {{"failure_understood": true, "failure_reason_analysis": "Explanation"}}, "current_rca": {{"summary": "Root cause", "severity": "high", "contributing_factors": ["factor1"]}}}}

# confidence
0.3

# selected_workflow
None

# can_recover
False

# needs_human_review
True

# human_review_reason
no_matching_workflows
"""

    # BR-HAPI-016: Add remediation history section if context is available
    if remediation_history_context:
        from extensions.remediation_history_prompt import build_remediation_history_section

        history_section = build_remediation_history_section(remediation_history_context)
        if history_section:
            prompt += f"""
## Remediation History Context (AUTO-DETECTED)

{history_section}

"""

    return prompt


def _create_investigation_prompt(
    request_data: Dict[str, Any],
    remediation_history_context: Optional[Dict[str, Any]] = None,
) -> str:
    """
    Create investigation prompt with complete ADR-041 v3.3 hybrid format.

    Reference: ADR-041 v3.3 - LLM Prompt and Response Contract
    BR-HAPI-016: Remediation history context for LLM prompt enrichment.

    Args:
        request_data: Request data dict.
        remediation_history_context: Optional remediation history context from
            DataStorage (BR-HAPI-016, DD-HAPI-016 v1.1). When provided, a
            remediation history section is appended to the prompt.
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

{_build_business_context_section(environment, priority, business_category, risk_tolerance)}

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

### Phase 4: Discover and Select Workflow (MANDATORY - Three-Step Protocol)
**YOU MUST** follow this three-step workflow discovery protocol:

{_build_three_step_protocol_instructions()}

### Phase 5: Return Summary + JSON Payload
Provide natural language summary + structured JSON with workflow and parameters.

**If workflow discovery succeeds**:
```json
{{
  "root_cause_analysis": {{
    "summary": "Brief summary of root cause from investigation",
    "severity": "critical|high|medium|low|unknown",
    "contributing_factors": ["factor1", "factor2"]
  }},
  "selected_workflow": {{
    "workflow_id": "workflow-id-from-discovery",
    "version": "1.0.0",
    "confidence": 0.95,
    "rationale": "Why your RCA findings led to this workflow selection",
    "parameters": {{
      "PARAM_NAME": "value-from-investigation"
    }}
  }}
}}
```

**If workflow discovery fails or returns no workflows**:
```json
{{
  "root_cause_analysis": {{
    "summary": "Root cause from investigation",
    "severity": "critical|high|medium|low|unknown",
    "contributing_factors": ["factor1", "factor2"]
  }},
  "selected_workflow": null,
  "rationale": "Workflow discovery failed: [error details]. RCA completed but workflow selection unavailable."
}}
```

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

**Signal context filters** (severity, component, environment, priority) are automatically
included in all discovery calls. You do NOT need to provide them manually.

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
{{"summary": "Brief summary of root cause", "severity": "critical|high|medium|low|unknown", "contributing_factors": ["factor1", "factor2"]}}

# confidence
0.95

# selected_workflow
{{"workflow_id": "workflow-id-from-discovery", "version": "1.0.0", "confidence": 0.95, "rationale": "Why this workflow was selected", "parameters": {{"PARAM_NAME": "value"}}}}

**IMPORTANT**:
- **DO NOT** use a single ```json block - use section headers as shown above
- Select ONE workflow per incident
- Each field must have its own `# field_name` header
- If a field is not applicable, use `None` or `[]` as the value
- Populate ALL required parameters from the workflow schema
- Use your RCA findings to determine parameter values
"""

    # BR-HAPI-016: Add remediation history section if context is available
    if remediation_history_context:
        from extensions.remediation_history_prompt import build_remediation_history_section

        history_section = build_remediation_history_section(remediation_history_context)
        if history_section:
            prompt += f"""
## Remediation History Context (AUTO-DETECTED)

{history_section}

"""
    return prompt
