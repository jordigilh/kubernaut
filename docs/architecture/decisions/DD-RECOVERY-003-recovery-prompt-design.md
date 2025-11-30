# DD-RECOVERY-003: Recovery Prompt Design

## Status
**✅ APPROVED** (2025-11-29)
**Related**: DD-RECOVERY-002 (Direct AIAnalysis Recovery Flow)
**Confidence**: 92%

## Context & Problem

When a workflow execution fails, the AI must analyze the failure and recommend an alternative approach. The recovery prompt must:

1. **Reuse the incident prompt structure** - Consistency for the LLM
2. **Add previous attempt context** - What was tried, what RCA concluded, what failed
3. **Expect signal type may have changed** - Workflow execution may have altered cluster state
4. **Use Kubernetes reason codes** - Structured contract for failure classification
5. **Guide investigation from failure point** - Don't re-investigate the original problem

## Decision

### Recovery Prompt Structure

The recovery prompt extends the incident prompt with a **"Previous Remediation Attempt"** section that appears BEFORE the investigation instructions.

```
# Recovery Analysis Request (Attempt {N})

## Previous Remediation Attempt - CRITICAL CONTEXT

⚠️ **This is a RECOVERY attempt**. A previous remediation was tried and FAILED.

### What Was Tried

**Original Root Cause Analysis**:
- Summary: {original_rca_summary}
- Signal Type (from RCA): {original_signal_type}
- Severity: {original_severity}
- Contributing Factors: {contributing_factors}

**Selected Workflow**:
- Workflow ID: {workflow_id}
- Version: {version}
- Rationale: {selection_rationale}
- Parameters Used: {parameters}

### What Failed

**Failure Details** (Kubernetes Reason Code):
- Failed Step: Step {step_index} - {step_name}
- Reason: **{kubernetes_reason}**
- Message: {error_message}
- Exit Code: {exit_code}
- Execution Time: {duration} before failure
- Failed At: {timestamp}

### Your Recovery Task

1. **DO NOT** repeat the same workflow with the same parameters
2. **INVESTIGATE** the current cluster state starting from the failure point
3. **DETERMINE** if the signal type has changed due to the failed execution
4. **RECOMMEND** an alternative approach based on:
   - Why the previous attempt failed (Kubernetes reason)
   - Current cluster state (may have changed)
   - What alternative workflows exist

---

[... rest of incident prompt structure ...]
```

## Detailed Prompt Template

### Complete Recovery Prompt

```python
def _create_recovery_investigation_prompt(request_data: Dict[str, Any]) -> str:
    """
    Create investigation prompt for recovery analysis.

    Extends incident prompt with previous attempt context.
    Reference: DD-RECOVERY-003
    """
    # Previous attempt details
    previous = request_data.get("previous_execution", {})
    original_rca = previous.get("original_rca", {})
    selected_workflow = previous.get("selected_workflow", {})
    failure = previous.get("failure", {})
    attempt_number = request_data.get("recovery_attempt_number", 1)

    # Build recovery context section
    recovery_context = f"""# Recovery Analysis Request (Attempt {attempt_number})

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
        recovery_context += "\n**Parameters Used**:\n"
        for key, value in params.items():
            recovery_context += f"- `{key}`: `{value}`\n"

    # Add failure details with Kubernetes reason code
    failure_reason = failure.get('reason', 'Unknown')
    recovery_context += f"""
---

### What Failed During Execution

**Execution Failure Details**:
- **Failed Step**: Step {failure.get('failed_step_index', '?')} - `{failure.get('failed_step_name', 'Unknown')}`
- **Kubernetes Reason**: **`{failure_reason}`**
- **Error Message**: {failure.get('message', 'No message')}
- **Exit Code**: {failure.get('exit_code', 'N/A')}
- **Execution Duration**: {failure.get('execution_time', 'Unknown')} before failure
- **Failed At**: {failure.get('failed_at', 'Unknown')}

**Failure Reason Interpretation** (`{failure_reason}`):
"""

    # Add reason-specific guidance
    reason_guidance = _get_failure_reason_guidance(failure_reason)
    recovery_context += reason_guidance

    recovery_context += """
---

### Your Recovery Investigation Task

**CRITICAL INSTRUCTIONS**:

1. **DO NOT** select the same workflow (`{workflow_id}`) with the same parameters
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

""".format(workflow_id=selected_workflow.get('workflow_id', 'Unknown'),
           failure_reason=failure_reason)

    # Now append the standard incident prompt structure
    # (reuse the incident prompt sections for consistency)
    incident_sections = _create_incident_sections(request_data)

    return recovery_context + incident_sections


def _get_failure_reason_guidance(reason: str) -> str:
    """
    Provide reason-specific recovery guidance based on Kubernetes reason codes.

    These are the canonical Kubernetes reason codes used as API contract.
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
```

### Recovery vs Incident: Key Differences

| Aspect | Incident Prompt | Recovery Prompt |
|--------|-----------------|-----------------|
| **Opening Section** | "Incident Analysis Request" | "Recovery Analysis Request (Attempt N)" |
| **Context** | Signal details only | Signal + Previous attempt + Failure |
| **Investigation Start** | Original signal | Failure point |
| **Signal Type** | From input | May have CHANGED |
| **Workflow Search** | Based on RCA | Based on failure reason + alternatives |
| **Constraint** | None | Cannot repeat same workflow |

### HolmesGPT-API Model Updates

```python
# src/models/recovery_models.py

class PreviousExecution(BaseModel):
    """Previous execution context for recovery analysis"""
    workflow_execution_ref: str = Field(..., description="Name of failed WorkflowExecution CRD")

    original_rca: OriginalRCA = Field(..., description="RCA from initial AIAnalysis")
    selected_workflow: SelectedWorkflowSummary = Field(..., description="Workflow that was executed")
    failure: ExecutionFailure = Field(..., description="Structured failure details")


class OriginalRCA(BaseModel):
    """Summary of original root cause analysis"""
    summary: str = Field(..., description="Brief RCA summary")
    signal_type: str = Field(..., description="Signal type determined by RCA")
    severity: str = Field(..., description="Severity determined by RCA")
    contributing_factors: List[str] = Field(default_factory=list)


class SelectedWorkflowSummary(BaseModel):
    """Summary of workflow that was executed"""
    workflow_id: str = Field(..., description="Workflow identifier")
    version: str = Field(..., description="Workflow version")
    container_image: str = Field(..., description="Container image used")
    parameters: Dict[str, str] = Field(default_factory=dict)
    rationale: str = Field(..., description="Why this workflow was selected")


class ExecutionFailure(BaseModel):
    """Structured failure information using Kubernetes reason codes"""
    failed_step_index: int = Field(..., description="0-indexed step that failed")
    failed_step_name: str = Field(..., description="Name of failed step")
    reason: str = Field(
        ...,
        description="Kubernetes reason code (e.g., OOMKilled, DeadlineExceeded)"
    )
    message: str = Field(..., description="Human-readable error message")
    exit_code: Optional[int] = Field(None, description="Exit code if applicable")
    failed_at: str = Field(..., description="ISO timestamp of failure")
    execution_time: str = Field(..., description="Duration before failure (e.g., '2m34s')")


class RecoveryRequest(BaseModel):
    """Extended recovery request with previous execution context"""
    incident_id: str
    remediation_id: str

    # Recovery-specific fields
    is_recovery_attempt: bool = Field(default=True)
    recovery_attempt_number: int = Field(..., ge=1)

    # COMPLETE history of ALL previous attempts (allows LLM to see full context)
    # Ordered chronologically: index 0 = first attempt, last index = most recent
    # LLM can: avoid repeating failures, learn from patterns, retry earlier approaches
    previous_executions: List[PreviousExecution] = Field(
        ...,
        description="Complete history of all previous attempts"
    )

    # Original context (reused from SignalProcessing)
    enrichment_results: Dict[str, Any] = Field(..., description="Original enriched context")

    # Standard fields
    signal_type: str
    severity: str
    resource_namespace: str
    resource_kind: str
    resource_name: str
    # ... other standard fields
```

## Expected LLM Response Format

### Recovery Response Structure

```json
{
  "recovery_analysis": {
    "previous_attempt_assessment": {
      "failure_understood": true,
      "failure_reason_analysis": "The OOMKilled suggests the workflow container exceeded memory limits while attempting to scale the deployment. The aggressive scaling approach required more memory than available.",
      "state_changed": true,
      "current_signal_type": "InsufficientMemory"
    },
    "current_rca": {
      "summary": "Cluster now in memory pressure state after failed remediation attempt",
      "severity": "high",
      "signal_type": "InsufficientMemory",
      "contributing_factors": [
        "Previous remediation consumed available memory",
        "Node memory pressure triggered",
        "Other pods may be affected"
      ]
    }
  },
  "selected_workflow": {
    "workflow_id": "memory-pressure-relief-v1",
    "version": "1.0.0",
    "confidence": 0.82,
    "rationale": "Selected memory-pressure-relief workflow instead of scaling. This workflow identifies and evicts lower-priority pods to free memory before attempting any scaling operations. Avoids the OOMKilled issue from previous attempt.",
    "parameters": {
      "TARGET_NAMESPACE": "production",
      "MIN_PRIORITY_TO_EVICT": "100",
      "MEMORY_FREE_TARGET": "2Gi"
    }
  },
  "recovery_strategy": {
    "approach": "resource_relief_first",
    "differs_from_previous": true,
    "why_different": "Previous attempt tried direct scaling which required more memory. This approach frees memory first before any scaling operations."
  }
}
```

## Canonical Kubernetes Reason Codes

The following reason codes form the **structured API contract** between WorkflowExecution status and AIAnalysis recovery:

### Resource Reasons
- `OOMKilled` - Container exceeded memory limit
- `InsufficientCPU` - Not enough CPU available
- `InsufficientMemory` - Not enough memory available
- `Evicted` - Pod evicted due to resource pressure

### Scheduling Reasons
- `FailedScheduling` - Scheduler couldn't place pod
- `Unschedulable` - Pod marked unschedulable

### Image Reasons
- `ImagePullBackOff` - Repeated image pull failures
- `ErrImagePull` - Single image pull failure
- `InvalidImageName` - Image name format invalid

### Execution Reasons
- `DeadlineExceeded` - Execution exceeded timeout
- `BackoffLimitExceeded` - Exceeded retry limit
- `Error` - Generic execution error
- `Completed` - (Not a failure, but included for completeness)

### Permission Reasons
- `Unauthorized` - Authentication failed
- `Forbidden` - Authorization denied

### Volume Reasons
- `FailedMount` - Volume mount failed
- `FailedAttachVolume` - Volume attach failed

### Node Reasons
- `NodeNotReady` - Node unavailable
- `NodeUnreachable` - Node network unreachable

### Network Reasons
- `NetworkNotReady` - Network not available

## Consequences

### Positive
- ✅ LLM has complete context about what was tried and failed
- ✅ Structured failure reasons enable deterministic recovery guidance
- ✅ Prompt consistency with incident flow (same structure)
- ✅ Clear instruction not to repeat failed approach

### Negative
- ⚠️ Longer prompt for recovery scenarios
  - **Mitigation**: Token cost acceptable for better recovery quality
- ⚠️ LLM must understand Kubernetes reason codes
  - **Mitigation**: Guidance provided in prompt for each reason

### Neutral
- 🔄 Recovery-specific response fields (`recovery_analysis`, `recovery_strategy`)
- 🔄 Reason code mapping maintained in HolmesGPT-API

## Validation Checklist

- [ ] Recovery prompt template implemented in `recovery.py`
- [ ] RecoveryRequest model updated with `PreviousExecution`
- [ ] Reason code guidance map complete
- [ ] Response parsing handles recovery-specific fields
- [ ] Unit tests for recovery prompt generation
- [ ] Integration tests for recovery analysis endpoint

## Related Decisions

| Decision | Relationship |
|----------|-------------|
| DD-RECOVERY-002 | Parent - defines the flow |
| ADR-041 | Extended - same prompt contract principles |
| DD-CONTRACT-002 | Aligned - uses same field definitions |

