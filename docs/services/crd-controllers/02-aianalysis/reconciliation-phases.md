# AI Analysis Service - Reconciliation Phases

**Version**: v2.2
**Last Updated**: 2025-12-09
**Status**: ✅ V1.0 Scope Defined

---

## Changelog

| Version | Date | Changes | Reference |
|---------|------|---------|-----------|
| **v2.2** | 2025-12-09 | **V1.0 COMPLIANCE AUDIT**: (1) Timeout should be `spec.TimeoutConfig` not annotation (pending RO clarification); (2) Recovery attempts must use `/api/v1/recovery/analyze` endpoint (pending HAPI confirmation); (3) Recovery fields must be passed to HAPI | [NOTICE_AIANALYSIS_V1_COMPLIANCE_GAPS.md](../../../handoff/NOTICE_AIANALYSIS_V1_COMPLIANCE_GAPS.md), [REQUEST_RO_TIMEOUT_PASSTHROUGH_CLARIFICATION.md](../../../handoff/REQUEST_RO_TIMEOUT_PASSTHROUGH_CLARIFICATION.md) |
| v2.1 | 2025-12-06 | **BR-HAPI-197**: Added `SubReason` field for granular failure tracking; Removed `Recommending` from Phase enum; Added failure taxonomy | BR-HAPI-197, DD-HAPI-002 v1.2 |
| v2.0 | 2025-11-30 | **REGENERATED**: Removed "Approving" phase (V1.0); Removed BR-AI-051-053 (dependency validation); Simplified to 4-phase flow; Added DetectedLabels/CustomLabels handling | DD-RECOVERY-002, BR_MAPPING v1.2 |
| v1.1 | 2025-10-20 | Added approval context population | ADR-018 |
| v1.0 | 2025-10-15 | Initial specification | - |

---

## Phase Overview (V1.0)

### Phase Transitions

```
Pending → Investigating → Analyzing → Completed
   ↓           ↓              ↓           ↓
(<1s)       (≤60s)         (≤5s)     (terminal)
```

**Note**: The "Approving" phase is **deferred to V1.1**. In V1.0, approval decisions are made during the Analyzing phase, and the Remediation Orchestrator (RO) handles notification.

### Phase Summary

| Phase | Purpose | Timeout | Key Actions |
|-------|---------|---------|-------------|
| **Pending** | Validation | <1s | Validate spec, add finalizer |
| **Investigating** | AI Analysis | 60s | Call HolmesGPT-API, receive workflow recommendation |
| **Analyzing** | Policy Evaluation | 5s | Evaluate Rego policies, validate workflow exists in catalog |
| **Completed** | Terminal | N/A | Output ready for RO consumption |

---

## Phase 1: Pending

**Purpose**: Initial validation and setup

**Timeout**: Immediate (<1s)

### Actions

1. **Validate Spec**
   - Ensure `enrichmentResults` is present (required)
   - Validate `signalContext` structure
   - Check parent references

2. **Add Finalizer**
   - Add `aianalysis.kubernaut.ai/cleanup` finalizer
   - Enables cleanup on deletion

3. **Initialize Status**
   - Set `status.phase = "Pending"`
   - Record `status.startTime`

### Transition Criteria

```go
if specValid && finalizerAdded {
    status.Phase = "Investigating"
    status.PhaseTransitions["Investigating"] = metav1.Now()
}
```

### Example Status After Pending

```yaml
status:
  phase: "Investigating"
  startTime: "2025-11-30T10:00:00Z"
  phaseTransitions:
    Pending: "2025-11-30T10:00:00Z"
    Investigating: "2025-11-30T10:00:01Z"
```

---

## Phase 2: Investigating

**Purpose**: AI-powered investigation via HolmesGPT-API

**Timeout**: 60 seconds (configurable via annotation)

### Actions

1. **Build Investigation Request**
   - Include `signalContext` from spec
   - Include `enrichmentResults.kubernetesContext`
   - Include `enrichmentResults.detectedLabels`
   - Include `enrichmentResults.customLabels`
   - If recovery: include `previousExecutions` array

2. **Call HolmesGPT-API**
   - Endpoint: `POST /api/v1/investigate`
   - HolmesGPT-API uses MCP tool to search workflow catalog
   - Labels are used for workflow pre-filtering in Data Storage

3. **Receive Workflow Recommendation**
   - `workflowId`: UUID from catalog
   - `containerImage`: OCI reference (resolved by HolmesGPT-API)
   - `parameters`: Workflow parameters
   - `confidence`: 0.0-1.0 score
   - `reasoning`: Human-readable explanation

### Investigation Request Example

```go
type InvestigationRequest struct {
    SignalContext      SignalContextInput     `json:"signalContext"`
    KubernetesContext  *KubernetesContext     `json:"kubernetesContext,omitempty"`
    DetectedLabels     *DetectedLabels        `json:"detectedLabels,omitempty"`
    CustomLabels       map[string][]string    `json:"customLabels,omitempty"`
    OwnerChain         []OwnerChainEntry      `json:"ownerChain,omitempty"`

    // Recovery context (if isRecoveryAttempt)
    IsRecoveryAttempt   bool                  `json:"isRecoveryAttempt,omitempty"`
    PreviousExecutions  []PreviousExecution   `json:"previousExecutions,omitempty"`
}
```

### Transition Criteria

```go
if investigationResponse != nil && investigationResponse.WorkflowRecommendation != nil {
    status.InvestigationResult = investigationResponse
    status.Phase = "Analyzing"
    status.PhaseTransitions["Analyzing"] = metav1.Now()
} else if timeoutExceeded {
    status.Phase = "Failed"
    status.FailureReason = "Investigation timeout exceeded (60s)"
}
```

### Timeout Configuration

```yaml
metadata:
  annotations:
    aianalysis.kubernaut.ai/investigating-timeout: "90s"  # Override default 60s
```

### Error Handling

| Error | Action | Retry |
|-------|--------|-------|
| HolmesGPT-API unavailable | Retry with exponential backoff | 3 attempts |
| Timeout | Mark as Failed | No |
| Invalid response | Mark as Failed | No |

### BR-HAPI-197: Human Review Required Handling

When HolmesGPT-API returns `needs_human_review=true`, the controller MUST:

1. **Fail immediately** - Do not proceed to Analyzing phase
2. **Set structured failure** - Use `Reason` + `SubReason` fields
3. **Emit metrics** - Track failure reason for observability

```go
if response.NeedsHumanReview {
    status.Phase = "Failed"
    status.Reason = "WorkflowResolutionFailed"  // Umbrella category
    status.SubReason = mapWarningsToSubReason(response.Warnings)  // Specific cause
    status.Message = strings.Join(response.Warnings, "; ")
    // Store partial response for operator context
}
```

### SubReason Mapping

| HolmesGPT-API Trigger | SubReason |
|-----------------------|-----------|
| Workflow Not Found | `WorkflowNotFound` |
| Container Image Mismatch | `ImageMismatch` |
| Parameter Validation Failed | `ParameterValidationFailed` |
| No Workflows Matched | `NoMatchingWorkflows` |
| Low Confidence (<70%) | `LowConfidence` |
| LLM Parsing Error | `LLMParsingError` |

---

## Phase 3: Analyzing

**Purpose**: Rego policy evaluation and workflow validation

**Timeout**: 5 seconds

### Actions

1. **Load Rego Approval Policies**
   - ConfigMap: `ai-approval-policies` in `kubernaut-system`
   - Evaluate with investigation result and context

2. **Build Policy Input**
   ```go
   type ApprovalPolicyInput struct {
       Confidence       float64           `json:"confidence"`
       Environment      string            `json:"environment"`
       Severity         string            `json:"severity"`
       ActionType       string            `json:"action_type"`
       DetectedLabels   *DetectedLabels   `json:"detected_labels,omitempty"`
       CustomLabels     map[string][]string `json:"custom_labels,omitempty"`
       IsRecoveryAttempt bool             `json:"is_recovery_attempt"`
   }
   ```

3. **Evaluate Approval Decision**
   - `AUTO_APPROVE`: Confidence ≥80%, low-risk environment
   - `MANUAL_APPROVAL_REQUIRED`: Confidence <80%, production, high-risk action

4. **Validate Workflow Response** (Defense-in-Depth)

   > ⚠️ **Note**: Per DD-HAPI-002 v1.1, primary validation happens in **HolmesGPT-API**
   > where the LLM can self-correct. AIAnalysis validation is defense-in-depth only.

   | Validation | Primary | AIAnalysis (Defense) |
   |------------|---------|---------------------|
   | `workflowId` exists in catalog | ✅ **HAPI** (LLM self-corrects) | 🟡 Optional |
   | `containerImage` valid OCI format | ✅ **Data Storage** (registration) | 🟡 Optional |
   | Parameters conform to schema | ✅ **HAPI** (`validate_workflow_parameters`) | ❌ Not recommended |

   **Rationale** (DD-HAPI-002):
   - If validation fails at HAPI → LLM can self-correct in same session (good UX)
   - If validation fails at AIAnalysis → Must restart entire RCA (poor UX)

### Transition Criteria

```go
if policyEvaluated && workflowValidated {
    status.SelectedWorkflow = investigationResult.WorkflowRecommendation
    status.ApprovalRequired = (regoDecision == "MANUAL_APPROVAL_REQUIRED")
    status.ApprovalReason = regoDecision.Reason
    status.Phase = "Completed"
    status.CompletionTime = metav1.Now()
}
```

### Rego Policy Example

```rego
package aianalysis.approval

default decision = "MANUAL_APPROVAL_REQUIRED"

# Auto-approve if high confidence in non-production
decision = "AUTO_APPROVE" {
    input.confidence >= 0.8
    input.environment != "production"
}

# Auto-approve GitOps-managed + low-risk
decision = "AUTO_APPROVE" {
    input.confidence >= 0.85
    input.detected_labels.git_ops_managed == true
    input.action_type != "drain_node"
}
```

---

## Phase 4: Completed

**Purpose**: Terminal state - output ready for RO consumption

**Timeout**: None (terminal)

### Status After Completion

```yaml
status:
  phase: "Completed"
  completionTime: "2025-11-30T10:00:45Z"

  # Workflow recommendation (from HolmesGPT-API)
  selectedWorkflow:
    workflowId: "wf-memory-increase-v2"
    containerImage: "ghcr.io/kubernaut/workflows/memory-increase:v2.1.0"
    parameters:
      targetDeployment: "payment-api"
      memoryIncrease: "512Mi"
      namespace: "production"
    confidence: 0.87
    reasoning: "Historical success rate 92% for similar OOM scenarios"

  # Approval decision (from Rego policy)
  approvalRequired: true
  approvalReason: "Production environment requires manual approval"

  # Investigation summary (for operator context)
  investigationSummary: "OOMKilled due to memory leak in payment processing coroutine"

  # Phase timing
  phaseTransitions:
    Pending: "2025-11-30T10:00:00Z"
    Investigating: "2025-11-30T10:00:01Z"
    Analyzing: "2025-11-30T10:00:40Z"
    Completed: "2025-11-30T10:00:45Z"
```

### What Happens Next (RO Responsibility)

1. **RO watches** `AIAnalysis.status.phase == "Completed"`
2. **RO checks** `status.approvalRequired`:
   - **If false**: Create WorkflowExecution CRD immediately
   - **If true**: Create notification (Slack/Console), wait for operator approval
3. **V1.1**: RO will create `RemediationApprovalRequest` CRD for explicit approval workflow

---

## Phase 5: Failed

**Purpose**: Terminal failure state with structured reason

**Timeout**: None (terminal)

### Failure Taxonomy (BR-HAPI-197)

AIAnalysis uses a structured failure taxonomy with `reason` (umbrella category) and `subReason` (specific cause):

| Reason (Umbrella) | SubReason | Description |
|-------------------|-----------|-------------|
| `WorkflowResolutionFailed` | `WorkflowNotFound` | LLM hallucinated a workflow that doesn't exist |
| `WorkflowResolutionFailed` | `ImageMismatch` | LLM provided wrong container image |
| `WorkflowResolutionFailed` | `ParameterValidationFailed` | Parameters don't conform to schema |
| `WorkflowResolutionFailed` | `NoMatchingWorkflows` | Catalog has no matching workflows |
| `WorkflowResolutionFailed` | `LowConfidence` | AI confidence below 70% threshold |
| `WorkflowResolutionFailed` | `LLMParsingError` | Cannot parse LLM response |
| `TransientError` | Various | Temporary failure, retry recommended |
| `PermanentError` | Various | Unrecoverable failure |

### Failed Status Example

```yaml
status:
  phase: "Failed"
  reason: "WorkflowResolutionFailed"
  subReason: "WorkflowNotFound"
  message: "Workflow validation failed: workflow 'restart-pod-v1' not found in catalog"

  # Partial response preserved for operator context
  selectedWorkflow:
    workflowId: "restart-pod-v1"  # Invalid - not in catalog
    confidence: 0.85
    reasoning: "Historical success with similar OOM scenarios"

  phaseTransitions:
    Pending: "2025-12-06T10:00:00Z"
    Investigating: "2025-12-06T10:00:01Z"
    Failed: "2025-12-06T10:00:05Z"
```

### What Happens Next (RO Responsibility on Failure)

1. **RO watches** `AIAnalysis.status.phase == "Failed"`
2. **RO checks** `status.reason`:
   - **If `WorkflowResolutionFailed`**: Notify operator, require manual intervention
   - **If `TransientError`**: May trigger recovery attempt (up to max retries)
3. **No WorkflowExecution** is created for failed AIAnalysis

---

## Recovery Attempts

### Handling Previous Failures

When `spec.isRecoveryAttempt = true`, the controller:

1. **Includes all previous executions** in investigation request
2. **HolmesGPT-API uses** failure context to avoid repeating mistakes
3. **Tracks attempt number** for max retry enforcement

### Example Recovery Input

```yaml
spec:
  isRecoveryAttempt: true
  recoveryAttemptNumber: 2
  previousExecutions:
    - workflowId: "wf-oom-restart-v1"
      containerImage: "ghcr.io/kubernaut/workflows/oom-restart:v1.2.0"
      failureReason: "Pod evicted during restart - node pressure"
      failurePhase: "execution"
      kubernetesReason: "Evicted"
      attemptNumber: 1
    - workflowId: "wf-node-drain-v1"
      containerImage: "ghcr.io/kubernaut/workflows/node-drain:v1.0.0"
      failureReason: "PDB violation - insufficient replicas"
      failurePhase: "validation"
      kubernetesReason: "PodDisruptionBudgetViolation"
      attemptNumber: 2
```

### Recovery Decision

HolmesGPT-API analyzes previous failures and:
- Avoids selecting the same workflow if it failed with non-transient error
- Considers alternative approaches based on failure patterns
- May escalate to `notify_only` if all reasonable options exhausted

---

## CRD-Based Coordination

### Watch Patterns

```
RemediationOrchestrator
    ↓ (watches SignalProcessing completion)
SignalProcessing.status.phase == "Completed"
    ↓ (creates AIAnalysis with enrichmentResults)
AIAnalysis CRD created
    ↓ (AIAnalysis controller watches)
AIAnalysis.status.phase == "Completed"
    ↓ (RO watches for completion)
RemediationOrchestrator creates WorkflowExecution (if approved)
```

### No Direct HTTP Calls Between Controllers

**Correct Pattern**: CRD status updates + Kubernetes watches

**Benefits**:
- ✅ **Reliability**: Status persists in etcd
- ✅ **Observability**: `kubectl get aianalysis` shows state
- ✅ **Decoupling**: Controllers don't know about each other's endpoints

---

## Phase Timeout Configuration

### Default Timeouts

| Phase | Default | Configurable |
|-------|---------|--------------|
| Pending | Immediate | No |
| Investigating | 60s | Yes (spec field) |
| Analyzing | 5s | Yes (spec field) |
| Completed | N/A | No |

### ⚠️ Timeout Configuration (v2.2 Update)

> **DEPRECATION NOTICE**: The annotation-based timeout approach is **deprecated**.
> Per V1.0 compliance audit, timeout should be a **spec field** to:
> 1. Ensure immutability after creation (security)
> 2. Align with other CRDs (RO uses `spec.TimeoutConfig`)
> 3. Enable kubebuilder validation
>
> **Pending**: [REQUEST_RO_TIMEOUT_PASSTHROUGH_CLARIFICATION.md](../../../handoff/REQUEST_RO_TIMEOUT_PASSTHROUGH_CLARIFICATION.md)

### Target Timeout Configuration (Spec Field)

```go
// AIAnalysisSpec (target implementation)
type AIAnalysisSpec struct {
    // ... existing fields ...
    
    // Optional timeout configuration for this analysis
    // +optional
    TimeoutConfig *AIAnalysisTimeoutConfig `json:"timeoutConfig,omitempty"`
}

type AIAnalysisTimeoutConfig struct {
    // Timeout for Investigating phase (default: 60s)
    // +optional
    InvestigatingTimeout metav1.Duration `json:"investigatingTimeout,omitempty"`
    
    // Timeout for Analyzing phase (default: 5s)
    // +optional
    AnalyzingTimeout metav1.Duration `json:"analyzingTimeout,omitempty"`
}
```

### Legacy Timeout Annotation (Deprecated)

```yaml
# DEPRECATED - Do not use in new code
metadata:
  annotations:
    aianalysis.kubernaut.ai/investigating-timeout: "120s"
```

---

## Metrics

### Phase Duration Metrics

```go
var (
    aiAnalysisPhaseDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "kubernaut_aianalysis_phase_duration_seconds",
            Help:    "Duration of each AIAnalysis phase",
            Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120},
        },
        []string{"phase", "environment"},
    )

    aiAnalysisPhaseTransitions = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubernaut_aianalysis_phase_transitions_total",
            Help: "Total phase transitions",
        },
        []string{"from_phase", "to_phase"},
    )
)
```

### Key Metrics

| Metric | Purpose |
|--------|---------|
| `aianalysis_phase_duration_seconds{phase="Investigating"}` | HolmesGPT-API latency |
| `aianalysis_phase_duration_seconds{phase="Analyzing"}` | Rego evaluation time |
| `aianalysis_phase_transitions_total{to_phase="Failed"}` | Failure rate |
| `aianalysis_approval_required_total` | Manual approval rate |

---

## Related Documents

| Document | Purpose |
|----------|---------|
| [Overview](./overview.md) | Service architecture |
| [Controller Implementation](./controller-implementation.md) | Reconciler logic |
| [Rego Policy Examples](./REGO_POLICY_EXAMPLES.md) | Approval policy patterns |
| [DD-RECOVERY-002](../../../architecture/decisions/DD-RECOVERY-002-direct-aianalysis-recovery-flow.md) | Recovery flow design |
