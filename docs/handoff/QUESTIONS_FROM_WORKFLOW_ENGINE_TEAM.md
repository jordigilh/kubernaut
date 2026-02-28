# Questions from WorkflowExecution Team

> **Note (Issue #91):** This document references `kubernaut.ai/*` CRD labels that have since been migrated to immutable spec fields. See [DD-CRD-003](../architecture/DD-CRD-003-field-selectors-operational-queries.md) for the current field-selector-based approach.

**Date**: December 1, 2025
**From**: WorkflowExecution Team
**Context**: WE v3.1 Release - Resource Locking & Safety Features

---

## Overview

The WorkflowExecution service has updated to v3.1 with new resource locking and safety mechanisms (DD-WE-001, BR-WE-009/010/011). These questions clarify integration points with other teams.

**Related Documents**:
- WE CRD Schema v3.1: `docs/services/crd-controllers/03-workflowexecution/crd-schema.md`
- DD-WE-001: Resource Locking Safety
- DD-CONTRACT-001 v1.4: AIAnalysis-WorkflowExecution Alignment

---

## Questions by Team

| To Team | Questions | Priority | Status |
|---------|-----------|----------|--------|
| **HolmesGPT-API** | 3 questions | High | ✅ **RESOLVED** |
| **RemediationOrchestrator** | 5 questions | High | ✅ **RESOLVED** |
| **Gateway** | 3 questions | Medium | ✅ **RESOLVED** |
| **AIAnalysis** | 4 questions | Medium | ✅ **RESOLVED** |
| **Notification** | 5 questions | Medium | ✅ **RESOLVED** |
| **DataStorage** | ~~5 questions~~ | ~~High~~ | ✅ **CANCELLED** |

---

## Questions for HolmesGPT-API Team

### WE→HAPI-001: naturalLanguageSummary Usage 🔴 HIGH

**Context**: WE v3.1 provides `failureDetails.naturalLanguageSummary` for LLM-friendly error context.

**Question**: Will HolmesGPT-API use `naturalLanguageSummary` from `failureDetails` when generating recovery prompts?

**Example output from WE**:
```
Workflow 'scale-deployment' failed during step 'apply-hpa' after 2m15s.
Exit code: 1. Error: unable to apply HPA - resource quota exceeded.
2 of 5 tasks completed successfully before failure.
The HPA may have been partially applied. Manual review recommended.
```

**Response**: ✅ **YES - Will consume for recovery prompts** (December 1, 2025 - HolmesGPT-API Team)

HolmesGPT-API will consume `failureDetails.naturalLanguageSummary` from `WorkflowExecution.Status` when generating recovery prompts.

**Responsibility clarification**:
- **WE**: Generates `naturalLanguageSummary` from PipelineRun/TaskRun details
- **RO**: Passes to AIAnalysis via `PreviousExecutions[].NaturalLanguageSummary`
- **HAPI**: Includes in LLM recovery prompt for context

---

### WE→HAPI-002: wasExecutionFailure Handling 🔴 HIGH

**Context**: WE distinguishes pre-execution (safe to retry) vs during-execution (NOT safe) failures.

**Question**: When WE returns `wasExecutionFailure: true`, will HolmesGPT-API avoid automatic retry and instead inform the user that manual review is required?

| Failure Type | `wasExecutionFailure` | Safe to Retry? |
|--------------|----------------------|----------------|
| ValidationFailed | `false` | ✅ Yes |
| ImagePullBackOff | `false` | ✅ Yes |
| TaskFailed | `true` | ❌ No |
| Timeout (during exec) | `true` | ❌ No |

**Response**: ✅ **HAPI does NOT retry - RO decides** (December 1, 2025 - HolmesGPT-API Team)

**Decision**: HolmesGPT-API does NOT implement retry logic for workflow executions.

**Responsibility split**:
| Service | Responsibility |
|---------|----------------|
| **HolmesGPT-API** | RCA + Workflow Selection + Report results |
| **RO** | Orchestration + Retry/Recovery decisions |
| **WE** | Execution + Status reporting |

**Flow on failure**:
1. WE reports failure with `wasExecutionFailure`, `requiresManualReview`, `naturalLanguageSummary`
2. RO receives failure, evaluates `wasExecutionFailure`:
   - `false`: May trigger recovery AIAnalysis (HAPI analyzes and selects alternative)
   - `true`: May create notification for manual review
3. HAPI provides analysis/recommendations but does NOT decide retry

**Rationale**: Keeps concerns separated. HAPI focuses on intelligence (RCA, workflow selection), RO handles orchestration policy (retry, escalation, approval).

---

### WE→HAPI-003: Parameter Casing Enforcement 🟡 MEDIUM

**Context**: Tekton expects UPPER_SNAKE_CASE parameters.

**Question**: Can you confirm HolmesGPT-API enforces UPPER_SNAKE_CASE for generated parameters (per DD-WORKFLOW-003)?

**Response**: ✅ **No hardcoded format - workflow defines parameters** (December 1, 2025 - HolmesGPT-API Team)

**Decision**: HolmesGPT-API does NOT enforce a specific parameter casing format.

**Design rationale**:
- Parameters are defined by the **workflow schema** (in Data Storage)
- LLM populates values based on RCA and workflow's parameter definitions
- HAPI passes through as-is to AIAnalysis → RO → WE
- WE passes to runtime engine (Tekton, Ansible, etc.)

**Why no hardcoded format**:
- Supports multiple runtime engines with different conventions:
  - Tekton: `UPPER_SNAKE_CASE`
  - Ansible: `lower_snake_case` (typical)
  - Future engines: TBD
- Workflow author defines parameter schema, runtime engine interprets

**Contract**:
```
Workflow Schema (Data Storage) → defines parameter names/types/casing
LLM → populates values based on RCA
HolmesGPT-API → passes through as-is
WE → passes to Tekton/Ansible/etc (no transformation)
```

**Current state**: V1.0 workflows use `UPPER_SNAKE_CASE` because all V1.0 workflows are Tekton-based. Future runtime support (Ansible, etc.) will use workflow-defined conventions.

---

## Questions for RemediationOrchestrator Team

### WE→RO-001: targetResource Population 🔴 HIGH

**Context**: WE resource locking requires `targetResource` string.

**Question**: Confirm RO will populate `WorkflowExecution.Spec.TargetResource` from `RemediationRequest.Spec.TargetResource` using this format:

```go
func buildTargetResource(rr *RemediationRequest) string {
    tr := rr.Spec.TargetResource
    if tr.Namespace != "" {
        return fmt.Sprintf("%s/%s/%s", tr.Namespace, tr.Kind, tr.Name)
    }
    return fmt.Sprintf("%s/%s", tr.Kind, tr.Name)
}
```

**Response**: ✅ **CONFIRMED** (December 2, 2025 - RO Team)

**Details**:
- RO builds `targetResource` string from `RemediationRequest.Spec.TargetResource`
- Format: `namespace/kind/name` for namespaced resources, `kind/name` for cluster-scoped
- Casing preserved (Kubernetes conventions: `Deployment`, `Pod`, `Node`)

**Examples**:
- `"payment/Deployment/payment-api"` (namespaced)
- `"Node/worker-node-1"` (cluster-scoped)

**Related BR**: BR-ORCH-032 (Handle WE Skipped Phase)

---

### WE→RO-002: Skipped Phase Handling 🟡 MEDIUM

**Context**: WE v3.1 introduces `Skipped` phase.

**Question**: How will RO handle `phase: Skipped` with these reasons?

| Skip Reason | Description | RO Action? |
|-------------|-------------|------------|
| `ResourceBusy` | Another WE is running on same target | ❓ |
| `RecentlyRemediated` | Same workflow succeeded recently | ❓ |
| `PreviousExecutionFailed` | Previous execution failed during execution | ❓ |

**Options**:
- A) Treat as terminal - no retry
- B) Queue for retry after TTL
- C) Escalate to notification
- D) Per-reason handling

**Response**: ✅ **Option D - Per-reason handling** (December 2, 2025 - RO Team)

**Per DD-RO-001 (Resource Lock Deduplication Handling)**:

| Skip Reason | RO Action |
|-------------|-----------|
| **ResourceBusy** | Mark RR as `Skipped`, track on parent RR, include in bulk notification |
| **RecentlyRemediated** | Mark RR as `Skipped`, track on parent RR, include in bulk notification |

**Implementation**:
1. **Mark as Skipped** (not Failed):
   ```go
   rr.Status.Phase = "Skipped"
   rr.Status.SkipReason = we.Status.SkipDetails.Reason // "ResourceBusy" | "RecentlyRemediated"
   rr.Status.DuplicateOf = parentRRName
   ```

2. **Track on Parent RR**:
   ```go
   parentRR.Status.DuplicateCount++
   parentRR.Status.DuplicateRefs = append(parentRR.Status.DuplicateRefs, rr.Name)
   ```

3. **Bulk Notification** (when parent completes):
   - ONE notification sent with duplicate count and breakdown
   - Avoids notification spam (10 skipped = 1 notification)

**Related BRs**: BR-ORCH-032, BR-ORCH-033, BR-ORCH-034
**Related DD**: DD-RO-001 (Resource Lock Deduplication Handling)

---

### WE→RO-003: Recovery Flow After Execution Failure [Deprecated - Issue #180] 🔴 HIGH

**Context**: Execution failures leave cluster state unknown.

**Question**: When WE returns `wasExecutionFailure: true`, confirm RO will NOT auto-retry but instead:
1. Mark RemediationRequest as requiring manual review
2. Create NotificationRequest with failure details
3. Wait for human intervention

**WE Output**:
```yaml
status:
  phase: Failed
  failureDetails:
    reason: "TaskFailed"
    wasExecutionFailure: true      # ← KEY FIELD
    requiresManualReview: true
    naturalLanguageSummary: "Workflow failed during step..."
```

**Response**: ✅ **CONFIRMED - RO does NOT auto-retry execution failures** (December 2, 2025 - RO Team)

**Details**:
- When `wasExecutionFailure: true`, cluster state is unknown/potentially modified
- Auto-retry is dangerous - could cause cascading failures
- RO creates NotificationRequest for manual review

**Implementation**:
```go
func (r *Reconciler) handleWorkflowExecutionFailed(ctx context.Context, rr *RemediationRequest, we *WorkflowExecution) error {
    if we.Status.FailureDetails.WasExecutionFailure {
        // During-execution failure - DO NOT retry
        rr.Status.Phase = "Failed"
        rr.Status.RequiresManualReview = true
        rr.Status.Message = fmt.Sprintf("Execution failed during step '%s' - manual review required",
            we.Status.FailureDetails.FailedStep)

        // Create high-priority notification
        return r.createManualReviewNotification(ctx, rr, we)
    }

    // Pre-execution failure (validation, image pull) - MAY consider recovery
    return r.evaluateRecoveryOptions(ctx, rr, we)
}
```

**RO Actions for `wasExecutionFailure: true`**:
1. ✅ Mark RemediationRequest as `Failed`
2. ✅ Set `requiresManualReview: true`
3. ✅ Create NotificationRequest with failure details
4. ❌ NO automatic retry
5. ❌ NO automatic recovery AIAnalysis

---

### WE→RO-004: WorkflowRef Pass-Through 🟢 LOW

**Context**: RO creates WE from AIAnalysis output.

**Question**: Confirm RO passes all fields from `AIAnalysis.Status.SelectedWorkflow` to `WorkflowExecution.Spec.WorkflowRef`:

| Field | Source |
|-------|--------|
| `workflowId` | `AIAnalysis.Status.SelectedWorkflow.WorkflowID` |
| `containerImage` | `AIAnalysis.Status.SelectedWorkflow.ContainerImage` |
| `containerDigest` | `AIAnalysis.Status.SelectedWorkflow.ContainerDigest` |
| `parameters` | `AIAnalysis.Status.SelectedWorkflow.Parameters` |

**Response**: ✅ **CONFIRMED - Complete pass-through per DD-CONTRACT-002 v1.2** (December 2, 2025 - RO Team)

**Field Mapping**:
| WE Field | Source |
|----------|--------|
| `spec.workflowRef.workflowId` | `AIAnalysis.Status.SelectedWorkflow.WorkflowID` |
| `spec.workflowRef.version` | `AIAnalysis.Status.SelectedWorkflow.Version` |
| `spec.workflowRef.containerImage` | `AIAnalysis.Status.SelectedWorkflow.ContainerImage` |
| `spec.workflowRef.containerDigest` | `AIAnalysis.Status.SelectedWorkflow.ContainerDigest` |
| `spec.parameters` | `AIAnalysis.Status.SelectedWorkflow.Parameters` |
| `spec.confidence` | `AIAnalysis.Status.SelectedWorkflow.Confidence` |
| `spec.rationale` | `AIAnalysis.Status.SelectedWorkflow.Rationale` |

**Key Point**: RO does NOT call Data Storage API for catalog lookup. HolmesGPT-API already resolved `workflow_id → containerImage` during MCP search.

**Related BR**: BR-ORCH-025 (Workflow Data Pass-Through)
**Related DD**: DD-CONTRACT-001 v1.2, DD-CONTRACT-002 v1.2

---

### WE→RO-005: Concurrent WE Limit 🟢 LOW

**Context**: Resource locking prevents parallel execution.

**Question**: Does RO limit how many WE CRs it creates per RemediationRequest? If RO creates multiple WEs for different workflows on same target, all but one will be skipped with `ResourceBusy`.

**Response**: ✅ **YES - One WE per RR by design** (December 2, 2025 - RO Team)

**Details**:
- RO creates exactly ONE WorkflowExecution per RemediationRequest
- WE is created only when AIAnalysis completes (and approval is granted if needed)
- If WE fails, RO may create a NEW AIAnalysis for recovery (not a new WE directly)
- WE's resource locking (DD-WE-001) provides additional safety

**Flow**:
```
RR → SP → AIAnalysis → (Approval?) → ONE WE → (Fail?) → NEW AIAnalysis → NEW WE
                                                  ↓
                                            (Success?) → Complete
```

**Why not multiple WEs**:
- Single workflow per remediation follows "one action per incident" principle
- Multiple WEs would require tracking parallel execution (complex)
- Recovery involves RE-ANALYSIS, not just retrying same workflow

---

### 📝 **RO Team Addendum** (December 6, 2025)

#### **Update 1: Missing `PreviousExecutionFailed` Handling**

The WE→RO-002 response table is incomplete. Adding clarification:

| Skip Reason | RO Action | Notification |
|-------------|-----------|--------------|
| **ResourceBusy** | Mark RR as `Skipped`, track on parent RR | Bulk notification (BR-ORCH-034) |
| **RecentlyRemediated** | Mark RR as `Skipped`, track on parent RR | Bulk notification (BR-ORCH-034) |
| **PreviousExecutionFailed** | Mark RR as `Skipped`, **DO NOT track as duplicate** | Individual notification (requires investigation) |

**Rationale**: `PreviousExecutionFailed` is different from `ResourceBusy`/`RecentlyRemediated`:
- It indicates the target resource has an unresolved failure state
- NOT a duplicate - it's a new remediation blocked by prior failure
- Operator needs to resolve prior failure before retry

---

#### **Update 2: New NotificationType for Manual Review (BR-ORCH-036)**

Per recent discussions with AIAnalysis team ([NOTICE_AIANALYSIS_WORKFLOW_RESOLUTION_FAILURE.md](./NOTICE_AIANALYSIS_WORKFLOW_RESOLUTION_FAILURE.md)), a new notification type `manual-review` has been added to the API:

```go
// Updated NotificationType enum (December 6, 2025)
const (
    NotificationTypeEscalation   NotificationType = "escalation"
    NotificationTypeSimple       NotificationType = "simple"
    NotificationTypeStatusUpdate NotificationType = "status-update"
    NotificationTypeApproval     NotificationType = "approval"      // NEW - BR-ORCH-001
    NotificationTypeManualReview NotificationType = "manual-review" // NEW - BR-ORCH-036
)
```

**Notification Type Selection Matrix** (updated):

| Scenario | Source | NotificationType | Priority |
|----------|--------|------------------|----------|
| AIAnalysis `WorkflowResolutionFailed` | AIAnalysis | `manual-review` | high |
| WE `wasExecutionFailure: true` | WE | `escalation` | high |
| WE `Skipped` (ResourceBusy) | WE | `status-update` | low |
| WE `Skipped` (PreviousExecutionFailed) | WE | `escalation` | medium |
| Approval Required (confidence 60-79%) | AIAnalysis | `approval` | high |

**Updated WE→RO-003 Code Example**:
```go
func (r *Reconciler) handleWorkflowExecutionFailed(ctx context.Context, rr *RemediationRequest, we *WorkflowExecution) error {
    if we.Status.FailureDetails.WasExecutionFailure {
        // During-execution failure - DO NOT retry
        rr.Status.Phase = "Failed"
        rr.Status.RequiresManualReview = true

        // Use escalation type (not manual-review - that's for AIAnalysis failures)
        nr := &notificationv1.NotificationRequest{
            Spec: notificationv1.NotificationRequestSpec{
                Type:     notificationv1.NotificationTypeEscalation, // execution failure
                Priority: notificationv1.NotificationPriorityHigh,
                Subject:  fmt.Sprintf("⚠️ Workflow Execution Failed: %s", we.Spec.WorkflowRef.WorkflowID),
                Body:     we.Status.FailureDetails.NaturalLanguageSummary,
                Channels: []notificationv1.Channel{
                    notificationv1.ChannelConsole,
                    notificationv1.ChannelSlack,
                    notificationv1.ChannelEmail,
                },
            },
        }
        return r.client.Create(ctx, nr)
    }

    // Pre-execution failure - MAY consider recovery
    return r.evaluateRecoveryOptions(ctx, rr, we)
}
```

---

#### **Update 3: New BRs for RO**

| BR | Description | Status |
|----|-------------|--------|
| **BR-ORCH-035** | NotificationRequest reference tracking in RR.Status | ✅ Implemented |
| **BR-ORCH-036** | Handle AIAnalysis `WorkflowResolutionFailed` | ✅ **READY** - HAPI Q18/Q19 resolved |

---

#### **Update 4: Clarification on `manual-review` vs `escalation`**

| NotificationType | Use Case | Description |
|------------------|----------|-------------|
| `manual-review` | AIAnalysis failure | AI couldn't recommend a workflow (catalog issues, low confidence, LLM errors) |
| `escalation` | WE failure | Workflow execution failed (cluster state may be modified, dangerous to retry) |

**Key Distinction**:
- `manual-review`: "AI needs help selecting a workflow" → investigate catalog/configuration
- `escalation`: "Workflow ran but failed" → investigate cluster state, potential rollback

---

## Questions for Gateway Team ✅ ALL RESOLVED

### WE→GW-001: Namespace for Cluster-Scoped Resources ✅

**Context**: WE builds lock keys from TargetResource.

**Question**: For cluster-scoped resources (Node, ClusterRole, PV), does Gateway leave `Namespace` as empty string or omit entirely?

**Response**: ✅ **Empty string** (December 1, 2025)
- Confirmed by code review of adapters
- Kubernetes Event Adapter: Uses `event.InvolvedObject.Namespace` (empty for cluster-scoped per K8s API)
- Prometheus Adapter: Uses `extractNamespace(alert.Labels)` (empty if no label)

---

### WE→GW-002: ResourceIdentifier Source ✅

**Question**: Where does Gateway extract `Kind`, `Name`, `Namespace` from?

**Response**: ✅ **C - NormalizedSignal.Resource** (December 1, 2025)
- Populated from adapters (K8s Event, Prometheus)
- CRD Creator does pass-through (empty preserved)

---

### WE→GW-003: Unknown Resource Handling ✅

**Context**: Gateway mentions `Kind: "Unknown"` default.

**Question**: What triggers `Kind: Unknown`? Should WE skip resource locking for these signals?

**Response**: ✅ **Never happens in V1** (December 1, 2025)
- All supported signals have resource info from adapters
- WE does not need to handle this edge case

---

## Questions for AIAnalysis Team ✅ ALL RESOLVED

### WE→AI-001: SelectedWorkflow Schema Alignment 🟡 MEDIUM

**Context**: WE depends on stable SelectedWorkflow schema.

**Question**: Does `AIAnalysis.Status.SelectedWorkflow` match DD-CONTRACT-001 v1.4 schema?

```go
type SelectedWorkflow struct {
    WorkflowID      string            `json:"workflowId"`
    ContainerImage  string            `json:"containerImage"`
    ContainerDigest string            `json:"containerDigest,omitempty"`
    Parameters      map[string]string `json:"parameters"`
    Confidence      float64           `json:"confidence"`
    Rationale       string            `json:"rationale"`
}
```

**Response**: ✅ **YES - Schema aligns with DD-CONTRACT-001 v1.4** (December 2, 2025 - AIAnalysis Team)

**AIAnalysis Schema** (from `aianalysis_types.go:397-419`):

| Field | WE Expected | AIAnalysis Actual | Match |
|-------|-------------|-------------------|-------|
| `workflowId` | `string` | `string` | ✅ |
| `containerImage` | `string` | `string` | ✅ |
| `containerDigest` | `string (optional)` | `string (omitempty)` | ✅ |
| `parameters` | `map[string]string` | `map[string]string` | ✅ |
| `confidence` | `float64` | `float64` | ✅ |
| `rationale` | `string` | `string` | ✅ |

**Note**: AIAnalysis also includes `Version` field which WE can use for audit purposes.

**Authoritative Source**: `api/aianalysis/v1alpha1/aianalysis_types.go`

---

### WE→AI-002: Parameter Key Format 🟢 LOW

**Question**: Are parameter keys guaranteed UPPER_SNAKE_CASE?

**Response**: ✅ **YES - UPPER_SNAKE_CASE guaranteed per DD-WORKFLOW-003** (December 2, 2025 - AIAnalysis Team)

**Evidence**:
- `aianalysis_types.go:416`: `// Workflow parameters (UPPER_SNAKE_CASE keys per DD-WORKFLOW-003)`
- ADR-045: `description: "Keys in UPPER_SNAKE_CASE per DD-WORKFLOW-003"`

**Guarantee**: HolmesGPT-API produces UPPER_SNAKE_CASE parameters based on workflow schema definitions. AIAnalysis passes through unchanged.

---

### WE→AI-003: ContainerImage Always Populated 🟢 LOW

**Question**: Is `ContainerImage` always populated when `SelectedWorkflow` is not nil?

**Edge cases**:
- Low confidence selection?
- Approval required?

**Response**: ✅ **YES - Always populated when SelectedWorkflow exists** (December 2, 2025 - AIAnalysis Team)

**Evidence**:
- `aianalysis_types.go:407-408`: `// +kubebuilder:validation:Required` on `ContainerImage`
- ADR-045: `containerImage` is a **required** field in `SelectedWorkflow` response schema

**Edge Case Handling**:
| Scenario | ContainerImage | Rationale |
|----------|----------------|-----------|
| Low confidence (< 0.8) | ✅ Populated | HolmesGPT-API always selects best match |
| Approval required | ✅ Populated | Workflow selected, just needs approval |
| No workflow match | ❌ `SelectedWorkflow = nil` | Entire struct is nil |

---

### WE→AI-004: AlternativeWorkflows Schema 🟢 LOW

**Question**: Do alternatives include `ContainerImage`, or only the primary selection?

**Response**: ✅ **YES - Alternatives include ContainerImage** (December 2, 2025 - AIAnalysis Team)

**Evidence** (from ADR-045):
```yaml
AlternativeWorkflow:
  type: object
  properties:
    workflowId: string
    containerImage: string    # ← Included
    confidence: number
    rationale: string
```

**Implementation Note**: `AlternativeWorkflow` type is defined in ADR-045 but not yet implemented in `aianalysis_types.go`. Will be added during implementation phase.

**Rationale**: Alternatives are fully resolved by HolmesGPT-API during the same catalog search. Including `ContainerImage` allows RO to use alternatives if primary fails without re-querying HolmesGPT-API.

---

## Questions for DataStorage Team ✅ CANCELLED

> **CANCELLED**: December 1, 2025
>
> **Reason**: BR-WE-001 (defense-in-depth parameter validation) has been cancelled.
>
> **Decision**: HolmesGPT-API is now the **sole** parameter validator (BR-HAPI-191).
> WE does not need to access Data Storage for workflow schemas.
>
> **Rationale**:
> - If validation fails at WE → must restart entire RCA flow (expensive)
> - If validation fails at HAPI → LLM can self-correct in same session (cheap)
> - Edge cases should be fixed at source (HAPI), not duplicated
> - Simplifies WE architecture - no DS dependency for validation
>
> **Updated Documents**:
> - BR-WE-001: Cancelled
> - DD-HAPI-002 v1.1: WE validation layer removed
>
> ~~**Original questions (archived)**:~~
> - ~~WE→DS-001: WorkflowSchema API for validation~~
> - ~~WE→DS-002: Schema in search response~~
> - ~~WE→DS-003: Schema versioning guarantees~~
> - ~~WE→DS-004: Workflow deprecation handling~~
> - ~~WE→DS-005: Validation responsibility split~~

---

## Questions for Notification Team

### WE→NOT-001: Skipped Workflow Notifications 🟡 MEDIUM

**Context**: WE v3.1 introduces `Skipped` phase.

**Question**: Should users be notified when workflows are skipped?

| Skip Reason | Notify? |
|-------------|---------|
| `ResourceBusy` | ❓ |
| `RecentlyRemediated` | ❓ |
| `PreviousExecutionFailed` | ❓ |

**Options**:
- A) Always notify
- B) Selective (only some reasons)
- C) Never notify
- D) Configurable per channel

**Response**: ✅ **Selective Notification (Option B)** (December 2, 2025 - Notification Team)

**Decision**: Implement selective notification based on skip reason with configurable rules.

#### **Recommendation by Skip Reason**

| Skip Reason | Notify? | Priority | Channel | Rationale |
|-------------|---------|----------|---------|-----------|
| **ResourceBusy** | ✅ YES | Low | Slack only | Informational - helps operators understand workflow queue status |
| **RecentlyRemediated** | ❌ NO | N/A | None | Expected behavior - no action needed, reduces noise |
| **PreviousExecutionFailed** | ✅ YES | Medium | Slack + Console | Operators need awareness that workflows are being blocked by prior failures |

#### **Implementation Approach**

**Phase 1 (V1.0 - Immediate)**:
- Create `NotificationRequest` CRDs for **ResourceBusy** and **PreviousExecutionFailed** only
- Skip notification for `RecentlyRemediated` (treated as success)
- Use existing `type: status-update` for low-priority skipped notifications

**Notification Format**:
```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: workflow-skipped-{workflowExecution-name}
spec:
  type: status-update  # Status change notification
  priority: low
  subject: "Workflow Skipped: {workflowId} ({skipReason})"
  body: |
    Workflow execution was skipped:
    - Workflow: {workflowId}
    - Target Resource: {targetResource}
    - Skip Reason: {skipReason}
    - Conflicting Workflow: {conflictingWorkflow.Name} (if ResourceBusy)
  channels:
    - console
    - slack  # Only if configured
```

**Phase 2 (V1.1 - Configurable Routing)**:
- Implement BR-NOT-065 (Channel Routing Based on Labels)
- Enable per-namespace configuration: `skipReason: ResourceBusy → slack`, `skipReason: PreviousExecutionFailed → email`

#### **Integration with Remediation Orchestrator**

**RO Responsibility**: Create `NotificationRequest` CRD when WE returns `phase: Skipped`

**Example RO Logic**:
```go
// In RO reconciler when receiving WE status update
if workflowExecution.Status.Phase == "Skipped" {
    skipReason := workflowExecution.Status.SkipDetails.Reason

    // Selective notification
    if skipReason == "ResourceBusy" || skipReason == "PreviousExecutionFailed" {
        notification := &notificationv1alpha1.NotificationRequest{
            Spec: notificationv1alpha1.NotificationRequestSpec{
                Type:     notificationv1alpha1.NotificationTypeStatusUpdate,
                Priority: notificationv1alpha1.NotificationPriorityLow,
                Subject:  fmt.Sprintf("Workflow Skipped: %s (%s)",
                    workflowExecution.Spec.WorkflowRef.WorkflowID, skipReason),
                Body:     buildSkippedBody(workflowExecution),
                Channels: []notificationv1alpha1.Channel{
                    notificationv1alpha1.ChannelConsole,
                    notificationv1alpha1.ChannelSlack, // If configured
                },
            },
        }
        r.Create(ctx, notification)
    }
    // RecentlyRemediated: no notification (reduces noise)
}
```

**Confidence**: 95% (based on production-ready notification infrastructure)

---

### WE→NOT-002: Manual Review Notifications 🔴 HIGH

**Context**: Execution failures require human review.

**Question**: When `requiresManualReview: true`, what notification priority/channel should be used?

**Options**:
- A) High-priority alert to ops team
- B) Standard failure notification
- C) Separate channel for manual review items

**Response**: ✅ **Option A: High-Priority Alert to Ops Team** (December 2, 2025 - Notification Team)

**Decision**: Treat manual review requirements as **high-priority alerts** requiring immediate operator attention.

#### **Recommended Notification Configuration**

| Attribute | Value | Rationale |
|-----------|-------|-----------|
| **Type** | `escalation` | Indicates critical event requiring immediate response |
| **Priority** | `high` | Requires timely operator attention (within 15 minutes) |
| **Channels** | Console + Slack + Email | Multi-channel for redundancy (BR-NOT-055: Graceful Degradation) |
| **Subject Format** | `⚠️ Manual Review Required: {workflowId} - {targetResource}` | Clear urgency indicator |
| **Body Content** | Includes: naturalLanguageSummary, failure reason, target resource, next steps | Actionable information (BR-NOT-032) |

#### **Notification Template**

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: manual-review-{workflowExecution-name}
spec:
  type: escalation
  priority: high
  subject: "⚠️ Manual Review Required: {workflowId} - {targetResource}"
  body: |
    A workflow execution has failed during execution and requires manual review.

    **Workflow Details:**
    - Workflow: {workflowId}
    - Target Resource: {targetResource}
    - Execution Name: {workflowExecution.Name}

    **Failure Context:**
    {failureDetails.naturalLanguageSummary}

    **Technical Details:**
    - Failure Reason: {failureDetails.reason}
    - Failed At: {failureDetails.failureTime}
    - Was Execution Failure: true (DO NOT AUTO-RETRY)

    **Next Steps:**
    1. Review workflow execution logs: `kubectl logs {pipelineRun}`
    2. Inspect target resource state: `kubectl get {targetResource.Kind} {targetResource.Name} -n {targetResource.Namespace}`
    3. Determine if manual intervention is needed or if alternative workflow should be attempted
    4. Update RemediationRequest status after resolution

    **Remediation Request:** {remediationRequest.Name}

  channels:
    - console
    - slack
    - email
  recipients:
    - name: ops-team
      slack: "#ops-alerts"
      email: ops@company.com
```

#### **Distinction from Standard Failure Notifications**

| Failure Type | `requiresManualReview` | Priority | Channels | Auto-Retry? |
|--------------|------------------------|----------|----------|-------------|
| **Pre-execution failure** (validation, ImagePullBackOff) | `false` | medium | Slack + Console | ✅ Yes (RO may retry) |
| **Execution failure** (TaskFailed, Timeout) | `true` | **high** | **Slack + Console + Email** | ❌ No (manual review) |

#### **Integration Example**

**Remediation Orchestrator**:
```go
// When WE returns execution failure
if workflowExecution.Status.FailureDetails.RequiresManualReview {
    notification := &notificationv1alpha1.NotificationRequest{
        Spec: notificationv1alpha1.NotificationRequestSpec{
            Type:     notificationv1alpha1.NotificationTypeEscalation,
            Priority: notificationv1alpha1.NotificationPriorityHigh,
            Subject:  fmt.Sprintf("⚠️ Manual Review Required: %s - %s",
                workflowExecution.Spec.WorkflowRef.WorkflowID,
                workflowExecution.Spec.TargetResource),
            Body:     buildManualReviewBody(workflowExecution),
            Channels: []notificationv1alpha1.Channel{
                notificationv1alpha1.ChannelConsole,
                notificationv1alpha1.ChannelSlack,
                notificationv1alpha1.ChannelEmail,
            },
            Recipients: []notificationv1alpha1.Recipient{
                {
                    Name:  "ops-team",
                    Slack: "#ops-alerts",
                    Email: "ops@company.com",
                },
            },
        },
    }
    r.Create(ctx, notification)

    // Mark RemediationRequest as requiring manual review
    remediationRequest.Status.Phase = "AwaitingManualReview"
    r.Status().Update(ctx, remediationRequest)
}
```

**Confidence**: 98% (based on production-ready multi-channel delivery with BR-NOT-055 graceful degradation)

---

### WE→NOT-003: NaturalLanguageSummary in Notifications 🟢 LOW

**Question**: Will notifications include `failureDetails.naturalLanguageSummary`?

**Options**:
- A) Full summary in notification body
- B) Truncated (first N characters)
- C) Link to full details
- D) Own formatting

**Response**: ✅ **Option A: Full summary in notification body** (December 2, 2025 - Notification Team)

**Decision**: Include complete `naturalLanguageSummary` in notification body for maximum operator context.

#### **Implementation Details**

| Notification Type | NaturalLanguageSummary Usage |
|-------------------|------------------------------|
| **Manual Review Alerts** | Full summary in body (primary context) |
| **Failure Notifications** | Full summary in body (helps diagnose) |
| **Success Notifications** | Omitted (not needed for success) |

#### **Size Limit Handling**

**Per BR-NOT-058**: Notification body max size is 10KB (10,240 bytes)

| NaturalLanguageSummary Size | Handling |
|-----------------------------|----------|
| **< 8KB** | Include in full |
| **8KB - 10KB** | Include with warning: "Large failure summary included" |
| **> 10KB** | Truncate to 8KB + append: "[TRUNCATED - Full details in WorkflowExecution status]" |

**Sanitization**: Applied before inclusion (BR-NOT-054 - 22 secret patterns redacted)

#### **Example Notification with NaturalLanguageSummary**

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest
spec:
  type: escalation
  priority: high
  subject: "Workflow Execution Failed: scale-deployment"
  body: |
    Workflow execution failed during execution.

    **Failure Summary:**
    {failureDetails.naturalLanguageSummary}

    **Example Output:**
    "Workflow 'scale-deployment' failed during step 'apply-hpa' after 2m15s.
    Exit code: 1. Error: unable to apply HPA - resource quota exceeded.
    2 of 5 tasks completed successfully before failure.
    The HPA may have been partially applied. Manual review recommended."

    **Technical Details:**
    - Failure Reason: {failureDetails.reason}
    - Failed At: {failureDetails.failureTime}

    **Next Steps:**
    1. Review full execution details: `kubectl get workflowexecution {name} -o yaml`
    2. Check target resource state
    3. Determine if rollback is needed
```

#### **Slack Formatting Enhancement**

For Slack channel (V1.0 - already implemented):
- `naturalLanguageSummary` rendered as **block quote** for readability
- Automatically linkified references (e.g., `kubectl` commands, resource names)
- Max 40KB Slack Block Kit limit enforced (BR-NOT-036)

**Slack Block Kit Example**:
```json
{
  "blocks": [
    {
      "type": "header",
      "text": {
        "type": "plain_text",
        "text": "⚠️ Workflow Execution Failed: scale-deployment"
      }
    },
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "*Failure Summary:*\n> Workflow 'scale-deployment' failed during step 'apply-hpa' after 2m15s..."
      }
    }
  ]
}
```

**Confidence**: 100% (already implemented in production-ready v1.2.0)

---

### WE→NOT-004: Failure Subtyping 🟢 LOW

**Question**: Should notifications differentiate pre-execution vs during-execution failures?

| Type | `wasExecutionFailure` | Different notification? |
|------|----------------------|------------------------|
| Pre-execution | `false` | ✅ YES (info, medium priority) |
| During-execution | `true` | ✅ YES (alert, high priority) |

**Response**: ✅ **YES - Different notification types** (December 2, 2025 - Notification Team)

**Decision**: Differentiate failure notifications based on `wasExecutionFailure` flag to provide appropriate operator guidance.

#### **Failure Type Notification Matrix**

| Failure Type | `wasExecutionFailure` | Notification Type | Priority | Subject Prefix | Retry Guidance |
|--------------|-----------------------|-------------------|----------|----------------|----------------|
| **Pre-execution** | `false` | `status-update` | medium | `ℹ️ Workflow Validation Failed` | "May be retried automatically" |
| **During-execution** | `true` | `escalation` | high | `⚠️ Workflow Execution Failed` | "DO NOT AUTO-RETRY - Manual review required" |

#### **Pre-Execution Failure Notification**

**Characteristics**:
- Lower priority (`medium`) - retryable failure
- Console + Slack (no email needed)
- Guidance: "Validation failed, will be retried"
- No immediate action required

**Example**:
```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest
spec:
  type: status-update
  priority: medium
  subject: "ℹ️ Workflow Validation Failed: {workflowId}"
  body: |
    Workflow execution failed during validation (pre-execution).

    **Failure Type:** Pre-execution (safe to retry)
    **Failure Reason:** {failureDetails.reason}

    **Context:**
    This failure occurred before any changes were applied to the cluster.
    The Remediation Orchestrator may automatically retry with corrected parameters.

    **Next Steps:**
    - No immediate action required
    - If failure persists, review workflow parameters
  channels:
    - console
    - slack
```

#### **During-Execution Failure Notification**

**Characteristics**:
- Higher priority (`high`) - requires manual review
- Console + Slack + Email (multi-channel for redundancy)
- Guidance: "DO NOT AUTO-RETRY - Manual review required"
- Immediate operator attention needed

**Example**:
```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest
spec:
  type: escalation
  priority: high
  subject: "⚠️ Workflow Execution Failed: {workflowId} - MANUAL REVIEW REQUIRED"
  body: |
    Workflow execution failed during execution (cluster state may be inconsistent).

    **Failure Type:** During-execution (NOT safe to retry)
    **Was Execution Failure:** true
    **Requires Manual Review:** true

    **⚠️ CRITICAL:** Do NOT automatically retry this workflow.
    Cluster state may be partially modified and requires validation.

    {failureDetails.naturalLanguageSummary}

    **Next Steps (REQUIRED):**
    1. Inspect target resource state
    2. Determine if rollback is needed
    3. Review execution logs for partial changes
    4. Manually approve retry ONLY if safe
  channels:
    - console
    - slack
    - email
```

#### **Integration with RO**

**Remediation Orchestrator Logic**:
```go
// In RO reconciler when WE reports failure
failureType := "pre-execution"
notificationPriority := notificationv1alpha1.NotificationPriorityMedium
notificationType := notificationv1alpha1.NotificationTypeStatusUpdate
channels := []notificationv1alpha1.Channel{
    notificationv1alpha1.ChannelConsole,
    notificationv1alpha1.ChannelSlack,
}

if workflowExecution.Status.FailureDetails.WasExecutionFailure {
    failureType = "during-execution"
    notificationPriority = notificationv1alpha1.NotificationPriorityHigh
    notificationType = notificationv1alpha1.NotificationTypeEscalation
    channels = append(channels, notificationv1alpha1.ChannelEmail) // Add email for high priority
}

notification := &notificationv1alpha1.NotificationRequest{
    Spec: notificationv1alpha1.NotificationRequestSpec{
        Type:     notificationType,
        Priority: notificationPriority,
        Subject:  buildFailureSubject(workflowExecution, failureType),
        Body:     buildFailureBody(workflowExecution, failureType),
        Channels: channels,
    },
}
r.Create(ctx, notification)
```

**Confidence**: 98% (requires RO integration, Notification Service ready)

---

### WE→NOT-005: ConflictingWorkflow Details 🟢 LOW

**Question**: For `ResourceBusy` skips, include conflicting workflow details in notification?

**Available data**:
```go
type ConflictingWorkflow struct {
    Name       string      `json:"name"`
    WorkflowID string      `json:"workflowId"`
    StartedAt  metav1.Time `json:"startedAt"`
}
```

**Response**: ✅ **YES - Include conflicting workflow details** (December 2, 2025 - Notification Team)

**Decision**: Include conflicting workflow information in `ResourceBusy` skip notifications to help operators understand workflow queue status.

#### **Notification Format with Conflicting Workflow**

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest
spec:
  type: status-update
  priority: low
  subject: "Workflow Skipped: {workflowId} (Resource Busy)"
  body: |
    Workflow execution was skipped because the target resource is currently locked by another workflow.

    **Skipped Workflow:**
    - Workflow: {workflowId}
    - Target Resource: {targetResource}
    - Requested At: {workflowExecution.CreationTimestamp}

    **Conflicting Workflow (Currently Running):**
    - Name: {conflictingWorkflow.Name}
    - Workflow ID: {conflictingWorkflow.WorkflowID}
    - Started At: {conflictingWorkflow.StartedAt}
    - Duration: {time.Since(conflictingWorkflow.StartedAt)} (still running)

    **Next Steps:**
    - Wait for conflicting workflow to complete
    - Workflow will be retried automatically after lock is released
    - If conflicting workflow is stuck (>15 minutes), investigate: `kubectl get workflowexecution {conflictingWorkflow.Name}`

    **Resource Locking Details:**
    Resource locking prevents concurrent modifications to the same resource,
    ensuring safety and preventing race conditions.
  channels:
    - console
    - slack
```

#### **Enhanced Slack Formatting**

For Slack channel, use **structured blocks** for better readability:

```json
{
  "blocks": [
    {
      "type": "header",
      "text": {
        "type": "plain_text",
        "text": "ℹ️ Workflow Skipped: Resource Busy"
      }
    },
    {
      "type": "section",
      "fields": [
        {
          "type": "mrkdwn",
          "text": "*Skipped Workflow:*\nscale-deployment-v2"
        },
        {
          "type": "mrkdwn",
          "text": "*Target Resource:*\ndeployment/api-server"
        }
      ]
    },
    {
      "type": "divider"
    },
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "*Conflicting Workflow (Currently Running):*"
      },
      "fields": [
        {
          "type": "mrkdwn",
          "text": "*Name:*\nwe-abc123"
        },
        {
          "type": "mrkdwn",
          "text": "*Workflow ID:*\nrestart-deployment-v1"
        },
        {
          "type": "mrkdwn",
          "text": "*Started:*\n2 minutes ago"
        },
        {
          "type": "mrkdwn",
          "text": "*Status:*\nStill running..."
        }
      ]
    },
    {
      "type": "context",
      "elements": [
        {
          "type": "mrkdwn",
          "text": "💡 Workflow will retry automatically after lock is released"
        }
      ]
    }
  ]
}
```

#### **Data Sanitization**

**Per BR-NOT-054**: Apply sanitization to conflicting workflow details
- Workflow names: No sanitization needed (non-sensitive)
- Workflow IDs: No sanitization needed (non-sensitive)
- **Target resource names**: Sanitize if contain sensitive patterns (e.g., `secret-manager-xyz`)

#### **Integration with RO**

**Remediation Orchestrator**:
```go
// When WE returns Skipped with ResourceBusy
if workflowExecution.Status.Phase == "Skipped" &&
   workflowExecution.Status.SkipDetails.Reason == "ResourceBusy" {

    conflictingWorkflow := workflowExecution.Status.SkipDetails.ConflictingWorkflow

    notification := &notificationv1alpha1.NotificationRequest{
        Spec: notificationv1alpha1.NotificationRequestSpec{
            Type:     notificationv1alpha1.NotificationTypeStatusUpdate,
            Priority: notificationv1alpha1.NotificationPriorityLow,
            Subject:  fmt.Sprintf("Workflow Skipped: %s (Resource Busy)",
                workflowExecution.Spec.WorkflowRef.WorkflowID),
            Body:     buildResourceBusyBody(workflowExecution, conflictingWorkflow),
            Channels: []notificationv1alpha1.Channel{
                notificationv1alpha1.ChannelConsole,
                notificationv1alpha1.ChannelSlack,
            },
        },
    }
    r.Create(ctx, notification)
}
```

**Confidence**: 95% (requires RO integration, Notification Service ready for structured content)

---

## Response Template

When answering, update the Response field:

```markdown
**Response**: ✅ **[BRIEF ANSWER]** (Date, Responder)
- Detail 1
- Detail 2
```

Then update the status table at the top.

---

## 📝 **Addendum: Notification Type Corrections** (December 2, 2025)

### **Update Summary**

The Notification team responses (WE→NOT-001 through WE→NOT-005) have been **corrected** to use CRD-compliant notification types per the authoritative schema in `api/notification/v1alpha1/notificationrequest_types.go`.

### **Corrections Made**

| Original (Incorrect) | Corrected | Affected Questions |
|---------------------|-----------|-------------------|
| `type: alert` | `type: escalation` | WE→NOT-002, WE→NOT-003, WE→NOT-004 |
| `type: info` | `type: status-update` | WE→NOT-001, WE→NOT-004, WE→NOT-005 |
| `NotificationTypeAlert` | `NotificationTypeEscalation` | All Go code examples |
| `NotificationTypeInfo` | `NotificationTypeStatusUpdate` | All Go code examples |

### **Valid NotificationRequest Types** (CRD Schema)

Per `api/notification/v1alpha1/notificationrequest_types.go`:

```go
// +kubebuilder:validation:Enum=escalation;simple;status-update
type NotificationType string

const (
    NotificationTypeEscalation   NotificationType = "escalation"   // Critical events requiring immediate response
    NotificationTypeSimple       NotificationType = "simple"        // Basic notifications
    NotificationTypeStatusUpdate NotificationType = "status-update" // Status change notifications
)
```

### **Updated Usage Guide**

| Use Case | Recommended Type | Priority | Example |
|----------|------------------|----------|---------|
| **Manual review required** | `escalation` | `high` | WE→NOT-002 (wasExecutionFailure: true) |
| **Execution failures** | `escalation` | `high` | WE→NOT-004 (during-execution) |
| **Pre-execution failures** | `status-update` | `medium` | WE→NOT-004 (validation failed) |
| **Workflow skipped** | `status-update` | `low` | WE→NOT-001, WE→NOT-005 |
| **General status updates** | `status-update` or `simple` | `low`-`medium` | Informational notifications |

### **Impact**

- ✅ **All YAML examples updated** with correct types
- ✅ **All Go code examples updated** with correct constants
- ✅ **All responses now CRD-compliant** - no validation errors when creating NotificationRequest CRDs
- ✅ **Integration examples ready** for Remediation Orchestrator implementation

### **Action Required**

**None** - The responses are now correct and ready for use. When implementing NotificationRequest CRD creation in Remediation Orchestrator, use the types as shown in the updated responses.

### **Questions?**

If you have questions about notification types or need clarification on which type to use for specific scenarios, contact the Notification team.

---

**Document Version**: 1.2 (Notification types corrected)
**Last Updated**: December 2, 2025
**Changelog**:
- v1.2: **Notification Type Corrections**: Fixed all invalid types (alert→escalation, info→status-update) per CRD schema
- v1.1: **AIAnalysis Team Response**: All 4 questions resolved (WE→AI-001/002/003/004)
- v1.0: Initial creation

