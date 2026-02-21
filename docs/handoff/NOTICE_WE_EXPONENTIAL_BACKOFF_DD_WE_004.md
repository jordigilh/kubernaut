# NOTICE: WorkflowExecution Exponential Backoff (DD-WE-004)

> **Note (Issue #91):** This document references `kubernaut.ai/*` CRD labels that have since been migrated to immutable spec fields. See [DD-CRD-003](../architecture/DD-CRD-003-field-selectors-operational-queries.md) for the current field-selector-based approach.

**From**: WorkflowExecution Team
**To**: RemediationOrchestrator Team, Notification Team
**Date**: 2025-12-06
**Priority**: 🔴 HIGH (Contract Change)
**Status**: ALL TEAMS ACKNOWLEDGED + SKIP-REASON LABEL IMPLEMENTED
**Version**: 1.4

---

## Summary

WorkflowExecution now implements **exponential backoff** for cooldown periods (DD-WE-004 v1.1), with a **critical distinction** between pre-execution and execution failures.

**Key Change**: Execution failures (`wasExecutionFailure: true`) now **immediately block ALL future retries** with a new skip reason `PreviousExecutionFailed`. This enforces the existing WE→RO-003 agreement at the WE level.

---

## Background

Per the cross-team agreement documented in `QUESTIONS_FROM_WORKFLOW_ENGINE_TEAM.md` (WE→RO-003):

> **Response**: ✅ **CONFIRMED - RO does NOT auto-retry execution failures** (December 2, 2025 - RO Team)
>
> **Details**:
> - When `wasExecutionFailure: true`, cluster state is unknown/potentially modified
> - Auto-retry is dangerous - could cause cascading failures
> - RO creates NotificationRequest for manual review

DD-WE-004 now **enforces this at the WE level** by blocking retries when `wasExecutionFailure: true`.

---

## Changes

### 1. New Skip Reason: `PreviousExecutionFailed`

When a previous WFE **ran and failed** (`wasExecutionFailure: true`), subsequent WFEs for the same target+workflow will be **immediately Skipped** with:

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: workflow-payment-oom-003
spec:
  targetResource: "payment/deployment/payment-api"
  workflowRef:
    workflowId: "oomkill-increase-memory"
status:
  phase: Skipped
  skipDetails:
    reason: PreviousExecutionFailed
    message: "Previous execution failed during workflow run on target payment/deployment/payment-api. Manual intervention required. Non-idempotent actions may have occurred."
    skippedAt: "2025-12-06T10:30:00Z"
    recentRemediation:
      name: "workflow-payment-oom-002"
      workflowId: "oomkill-increase-memory"
      completedAt: "2025-12-06T10:15:00Z"
      outcome: "Failed"
      targetResource: "payment/deployment/payment-api"
```

**Important**: This is NOT a temporary block. Manual intervention is required before retries can proceed.

---

### 2. New Status Fields

```go
type WorkflowExecutionStatus struct {
    // ... existing fields ...

    // ConsecutiveFailures tracks PRE-EXECUTION failures only
    // NOT incremented for execution failures (wasExecutionFailure: true)
    // Resets to 0 on successful completion
    ConsecutiveFailures int32 `json:"consecutiveFailures,omitempty"`

    // NextAllowedExecution for exponential backoff (pre-execution failures only)
    // Calculated as: BaseCooldown × 2^(failures-1), capped at MaxCooldown
    NextAllowedExecution *metav1.Time `json:"nextAllowedExecution,omitempty"`
}
```

---

### 3. Skip Reason Summary (Updated)

| Skip Reason | Meaning | Retry Possible? | Manual Intervention? |
|-------------|---------|-----------------|---------------------|
| `ResourceBusy` | Another WFE running on target | ✅ Wait for completion | No |
| `RecentlyRemediated` | Cooldown/backoff active | ✅ Wait for expiry | No |
| `ExhaustedRetries` | 5+ pre-execution failures | ❌ No | Yes |
| **`PreviousExecutionFailed`** | **Workflow ran and failed** | ❌ **No** | **Yes** |

---

### 4. Behavior Matrix

| Failure Type | `wasExecutionFailure` | Backoff Applied? | Next WFE Blocked? |
|--------------|----------------------|------------------|-------------------|
| Pre-execution (validation, image pull, quota) | `false` | ✅ Yes (exponential) | Only during backoff |
| Execution (task failed, timeout during run) | `true` | ❌ No | ✅ **Yes - permanently until manual intervention** |

---

## Rationale

### Why Block Execution Failure Retries?

Non-idempotent workflows can cause cascading damage if retried after partial execution:

```
Workflow: "increase-replicas"
  Step 1: kubectl patch deployment --replicas +1  ← EXECUTED
  Step 2: kubectl apply memory limits             ← FAILED

Result: Replicas = original + 1

If retried:
  Step 1: kubectl patch deployment --replicas +1  ← EXECUTED AGAIN
  Step 2: kubectl apply memory limits             ← May fail again

Result: Replicas = original + 2  ← WRONG!
```

By blocking retries for execution failures, we prevent this cascading damage.

---

## Impact on RO Team

### Required Actions

1. **Handle `PreviousExecutionFailed` Skip Reason**
   - When WFE returns `Phase=Skipped` with `Reason=PreviousExecutionFailed`:
     - Mark RemediationRequest appropriately
     - Create high-priority notification for operator
     - Do NOT attempt automatic retry or recovery

2. **Existing Behavior Unchanged for Other Skip Reasons**
   - `ResourceBusy`: Wait and retry (existing behavior)
   - `RecentlyRemediated`: Wait for cooldown/backoff expiry (existing behavior)
   - `ExhaustedRetries`: Manual intervention required (new, similar to `PreviousExecutionFailed`)

### Suggested RO Code Update

```go
func (r *Reconciler) handleWorkflowExecutionSkipped(ctx context.Context, rr *RemediationRequest, we *WorkflowExecution) error {
    switch we.Status.SkipDetails.Reason {
    case "ResourceBusy":
        // Requeue - another workflow is running
        return ctrl.Result{RequeueAfter: 30 * time.Second}, nil

    case "RecentlyRemediated":
        // Requeue - wait for cooldown/backoff
        return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil

    case "ExhaustedRetries", "PreviousExecutionFailed":
        // CRITICAL: Manual intervention required
        rr.Status.Phase = "Failed"
        rr.Status.RequiresManualReview = true
        rr.Status.Message = we.Status.SkipDetails.Message
        return r.createManualReviewNotification(ctx, rr, we)
    }
    return nil
}
```

---

## Impact on Notification Team

### Suggested Actions

1. **New Notification Template for `PreviousExecutionFailed`**

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest
spec:
  type: escalation
  priority: high
  subject: "⚠️ Workflow Retry Blocked: Previous Execution Failed"
  body: |
    A workflow retry has been **blocked** because a previous execution ran and failed.

    **This is NOT a temporary block.** Manual intervention is required.

    ## Details
    - **Target Resource:** {{ .TargetResource }}
    - **Workflow ID:** {{ .WorkflowID }}
    - **Previous Failure:** {{ .PreviousExecutionName }}
    - **Failed At:** {{ .FailedAt }}

    ## Why is retry blocked?
    The previous execution may have partially modified cluster state.
    Automatic retry could cause further damage (e.g., non-idempotent actions
    like scaling replicas could be applied multiple times).

    ## Required Action
    1. Investigate the previous failure
    2. Verify cluster state is correct
    3. Delete the failed WorkflowExecution CRD to clear the block
    4. Retry manually if appropriate

  channels:
    - console
    - slack
    - email
```

2. **Differentiate from `ExhaustedRetries`**

| Skip Reason | Notification Type | Message Focus |
|-------------|-------------------|---------------|
| `ExhaustedRetries` | Warning | "Infrastructure issues persisting, retries exhausted" |
| `PreviousExecutionFailed` | Critical | "Workflow ran and failed, cluster state unknown" |

---

## Questions for RO Team

1. Do you need to update your reconciler to handle `PreviousExecutionFailed` skip reason differently from existing skip reasons?

2. Should the RemediationRequest status include additional fields to expose the exponential backoff state (e.g., `ConsecutiveFailures`, `NextAllowedExecution`)?

3. How do you want to handle the scenario where an operator "clears" the block? (Delete the failed WFE? Annotation?)

---

## Questions for Notification Team

1. Do you need a new notification template for `PreviousExecutionFailed`?

2. Should the notification message be different from `ExhaustedRetries`?

3. Should `PreviousExecutionFailed` notifications be higher priority than `ExhaustedRetries`?

---

## References

| Document | Description |
|----------|-------------|
| [DD-WE-004 v1.1](../architecture/decisions/DD-WE-004-exponential-backoff-cooldown.md) | Exponential Backoff Cooldown Design Decision |
| [BR-WE-012](../services/crd-controllers/03-workflowexecution/BUSINESS_REQUIREMENTS.md) | Business Requirement |
| [WE→RO-003](./QUESTIONS_FROM_WORKFLOW_ENGINE_TEAM.md) | Original Cross-Team Agreement |
| [crd-schema.md](../services/crd-controllers/03-workflowexecution/crd-schema.md) | Updated CRD Schema |

---

## Acknowledgment Required

| Team | Representative | Status | Date | Notes |
|------|---------------|--------|------|-------|
| RemediationOrchestrator | RO Team | ✅ ACKNOWLEDGED | 2025-12-06 | See response below |
| Notification | Notification Team | ✅ ACKNOWLEDGED | 2025-12-06 | See response below |

---

## 📬 **RO Team Response** (2025-12-06)

### Acknowledgment

| Item | Status |
|------|--------|
| We understand `PreviousExecutionFailed` skip reason | ✅ |
| We understand `ExhaustedRetries` skip reason | ✅ |
| We understand exponential backoff for pre-execution failures | ✅ |
| We understand execution failures block permanently | ✅ |
| We will update BR-ORCH-032 to include both new skip reasons | ✅ In Progress |

---

### Answers to WE Team Questions

#### **Q1: Do you need to update your reconciler to handle `PreviousExecutionFailed` differently?**

**Answer**: ✅ **Yes, we will handle it differently.**

Per our updated BR-ORCH-032, we now have **four categories** of skip handling:

| Skip Reason | RO Handling | Is Duplicate? | Notification |
|-------------|-------------|---------------|--------------|
| `ResourceBusy` | Requeue (30s) | ✅ Yes | Bulk (BR-ORCH-034) |
| `RecentlyRemediated` | Requeue (1m) | ✅ Yes | Bulk (BR-ORCH-034) |
| `ExhaustedRetries` | Mark Failed, manual review | ❌ No | Individual (escalation) |
| `PreviousExecutionFailed` | Mark Failed, manual review | ❌ No | Individual (escalation) |

**Code update** (aligned with your suggestion):

```go
func (r *Reconciler) handleWorkflowExecutionSkipped(ctx context.Context, rr *RemediationRequest, we *WorkflowExecution) error {
    reason := we.Status.SkipDetails.Reason

    switch reason {
    case "ResourceBusy":
        // Requeue - another workflow is running, track as duplicate
        r.trackDuplicate(ctx, rr, we)
        return ctrl.Result{RequeueAfter: 30 * time.Second}, nil

    case "RecentlyRemediated":
        // Requeue - wait for cooldown/backoff, track as duplicate
        r.trackDuplicate(ctx, rr, we)
        requeueAfter := r.calculateRequeueTime(we.Status.NextAllowedExecution)
        return ctrl.Result{RequeueAfter: requeueAfter}, nil

    case "ExhaustedRetries", "PreviousExecutionFailed":
        // CRITICAL: Manual intervention required - NOT a duplicate
        rr.Status.Phase = "Failed"
        rr.Status.RequiresManualReview = true
        rr.Status.SkipReason = reason
        rr.Status.Message = we.Status.SkipDetails.Message

        // Create high-priority notification
        return r.createManualReviewNotification(ctx, rr, we)
    }
    return nil
}
```

---

#### **Q2: Should RemediationRequest status include `ConsecutiveFailures` and `NextAllowedExecution`?**

**Answer**: ⚠️ **Recommendation: No, but expose via labels/annotations.**

**Rationale**:
- RO's `RemediationRequest` is the **business-level** view of a remediation
- WE's `WorkflowExecution` holds the **technical** retry state
- Duplicating fields creates sync risk and schema bloat

**Recommended approach**:
1. RO reads `ConsecutiveFailures` and `NextAllowedExecution` from `WorkflowExecution.Status`
2. RO includes these in notifications for operator context
3. RO does NOT duplicate in `RemediationRequest.Status`

**If you strongly prefer exposure**, we could add:
```go
// In RemediationRequestStatus (optional, for convenience)
WorkflowRetryState *WorkflowRetryState `json:"workflowRetryState,omitempty"`

type WorkflowRetryState struct {
    ConsecutiveFailures  int32       `json:"consecutiveFailures"`
    NextAllowedExecution *metav1.Time `json:"nextAllowedExecution,omitempty"`
}
```

**🔴 Question for you (WE Team)**: Do you have a preference? Is there a use case where operators need this in RR rather than WFE?

> ### ✅ **WE Team Response (Q2)** (2025-12-06)
>
> **Answer**: ✅ **Agree - No duplication.**
>
> RO should read `ConsecutiveFailures` and `NextAllowedExecution` directly from `WorkflowExecution.Status`.
> Duplicating in `RemediationRequest.Status` creates sync risk and schema bloat.
>
> There is no use case where operators need this in RR rather than WFE.

---

#### **Q3: How should operators "clear" the block?**

**Answer**: ✅ **Prefer annotation-based clearing.**

**Recommended approach**:

| Option | Mechanism | Pros | Cons |
|--------|-----------|------|------|
| A | Delete failed WFE | Simple | Loses audit trail |
| **B** | Annotation on WFE | Preserves audit, explicit intent | Slightly more complex |
| C | Annotation on RR | RO-controlled | WE must watch RR (coupling) |

**We recommend Option B** (annotation on WFE):

```yaml
# Operator adds annotation to clear block
kubectl annotate workflowexecution workflow-payment-oom-002 \
  kubernaut.ai/clear-execution-block="acknowledged-by-operator"
```

WE watches for this annotation and:
1. Clears the `PreviousExecutionFailed` block
2. Allows next WFE for same target+workflow
3. Preserves failed WFE for audit trail

**🔴 Question for you (WE Team)**: Will you implement annotation-based clearing? If so, what annotation key/value?

> ### ✅ **WE Team Response (Q3)** (2025-12-06)
>
> **Answer**: ⏳ **Defer to v1.1.**
>
> We will NOT implement annotation-based clearing in v1.0.
>
> **Rationale**: Annotations are not ideal for this use case because:
> 1. **Audit trail gap**: We cannot trace WHO made the annotation change in the audit
> 2. **Accountability**: Clearing a safety block should have clear operator attribution
>
> **v1.0 Approach**: Operators delete the failed WFE to clear the block.
>
> **v1.1 Enhancement**: Tracked as **BR-WE-013** (Audit-Tracked Execution Block Clearing). Will design a proper clearing mechanism that:
> - Tracks who cleared the block (user identity)
> - Records the clearing action in audit trail
> - Possibly via a dedicated API endpoint or CRD field with admission webhook validation
>
> **Reference**: [BR-WE-013](../services/crd-controllers/03-workflowexecution/BUSINESS_REQUIREMENTS.md)

---

### RO Follow-up Questions for WE Team

#### **Q4: What about `ExhaustedRetries` + `PreviousExecutionFailed` interaction?**

**Scenario**:
1. WFE-1 fails (pre-execution) - `ConsecutiveFailures = 1`
2. WFE-2 fails (pre-execution) - `ConsecutiveFailures = 2`
3. WFE-3 **runs and fails** (`wasExecutionFailure: true`)
4. WFE-4 is created

**Question**: What skip reason does WFE-4 get?
- `PreviousExecutionFailed` (takes precedence)?
- `ExhaustedRetries` (if count >= 5)?
- Both checks applied in order?

**Our assumption**: `PreviousExecutionFailed` takes precedence (execution failure is more severe).

> ### ✅ **WE Team Response (Q4)** (2025-12-06)
>
> **Answer**: ✅ **Correct - `PreviousExecutionFailed` takes precedence.**
>
> Per DD-WE-004 implementation, `CheckCooldown()` evaluates in this order:
> 1. **First**: Check if previous WFE has `wasExecutionFailure: true` → Skip with `PreviousExecutionFailed`
> 2. **Second**: Check if `ConsecutiveFailures >= MaxConsecutiveFailures` → Skip with `ExhaustedRetries`
> 3. **Third**: Check if `time.Now() < NextAllowedExecution` → Skip with `RecentlyRemediated`
>
> Execution failures are more severe because they indicate potential cluster state modification.
> Pre-execution failures (tracked by `ConsecutiveFailures`) are infrastructure issues that may self-resolve.

---

#### **Q5: Backoff state per target+workflow or per target only?**

**Question**: Is `ConsecutiveFailures` tracked per:
- **A)** Target only (e.g., `payment/deployment/api`)
- **B)** Target + Workflow (e.g., `payment/deployment/api` + `oomkill-increase-memory`)

**Impact**: If (A), different workflows for same target share failure count. If (B), they're independent.

> ### ✅ **WE Team Response (Q5)** (2025-12-06)
>
> **Answer**: **(A) Per target resource only.**
>
> Per DD-WE-004-4, `ConsecutiveFailures` is scoped **per target resource**, not per target+workflow.
>
> **Rationale**:
> 1. **Prevents Bypass**: A user cannot bypass cooldown by trying a different workflow on the same target
> 2. **Single Source of Truth**: Consistent failure history for a given resource
> 3. **Alignment**: Aligns with DD-WE-003 (deterministic naming per target) and DD-WE-001 (resource locking)
>
> **Example**: If `payment/deployment/api` has `ConsecutiveFailures = 3` from workflow A failing,
> workflow B targeting the same resource will also see the backoff. This is intentional - the target
> resource itself may have issues preventing any workflow from succeeding.

---

#### **Q6: How does RO know when backoff expires?**

When WFE is skipped with `RecentlyRemediated`, should RO:
- **A)** Use `NextAllowedExecution` from skipped WFE status (if available)
- **B)** Calculate based on `completedAt` + cooldown config
- **C)** Simply requeue with fixed interval (1 minute) and let WE re-evaluate

**Our assumption**: (A) if `NextAllowedExecution` is populated, else (C).

> ### ✅ **WE Team Response (Q6)** (2025-12-06)
>
> **Answer**: **(C) Requeue with fixed interval, let WE re-evaluate.**
>
> **Rationale**:
> 1. **Separation of Concerns**: Cooldown logic is WE's scope, not RO's
> 2. **Single Source of Truth**: WE owns the backoff calculation; RO should not duplicate it
> 3. **Simplicity**: RO doesn't need to understand backoff parameters or formulas
> 4. **Future-Proof**: If WE changes backoff logic, RO doesn't need to update
>
> **RO Implementation**:
> ```go
> case "RecentlyRemediated":
>     // Don't calculate - just requeue and let WE re-evaluate
>     return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
> ```
>
> The `NextAllowedExecution` field in WFE status is for **observability** (operators can see when next attempt is allowed),
> not for RO to use directly in its logic.

---

### Impact on RO Documentation

We will update the following documents:

| Document | Update Required |
|----------|-----------------|
| BR-ORCH-032 | Add `ExhaustedRetries` skip reason (already added `PreviousExecutionFailed`) |
| reconciliation-phases.md | Add WE failure handling section |
| controller-implementation.md | Add exponential backoff awareness |
| DAYS_02_07_PHASE_HANDLERS.md | Add Day 5 content for WE status handling |

---

### Timeline

| Milestone | Target | Status |
|-----------|--------|--------|
| Acknowledge notice | 2025-12-06 | ✅ Done |
| Update BR-ORCH-032 | 2025-12-06 | ⏳ In Progress |
| Update spec docs | 2025-12-06 | ⏳ Pending |
| Update implementation plan | 2025-12-06 | ⏳ Pending |
| Implement in Day 5 | TBD | ⏳ Pending |

---

**Issued By**: RemediationOrchestrator Team
**Date**: 2025-12-06

---

## 📬 **Notification Team Response** (2025-12-06)

### Acknowledgment

| Item | Status |
|------|--------|
| We understand `PreviousExecutionFailed` skip reason | ✅ |
| We understand `ExhaustedRetries` skip reason | ✅ |
| We understand exponential backoff for pre-execution failures | ✅ |
| We understand execution failures block permanently | ✅ |
| Template suggestion reviewed and approved | ✅ |

---

### Answers to WE Team Questions

#### **Q1: Do you need a new notification template for `PreviousExecutionFailed`?**

**Answer**: ⚠️ **Clarification: Templates are NOT managed by Notification Service.**

Per our architecture (ADR-017), the **RemediationOrchestrator creates NotificationRequest CRDs** with fully-formed content. The Notification Service:
- **Does NOT** manage templates
- **Does NOT** interpret skip reasons
- **DOES** deliver whatever is in `spec.subject`, `spec.body`, `spec.channels`

**What we DO provide**:
- Channel routing via labels (BR-NOT-065, now implemented)
- Delivery to configured channels (slack, email, pagerduty, console, webhook)
- Retry logic with exponential backoff for delivery failures

**Recommended approach**:
1. **RO Team** creates the NotificationRequest with appropriate content (your suggested template is excellent)
2. **RO Team** sets labels for routing:
   ```yaml
   labels:
     kubernaut.ai/notification-type: workflow_blocked  # or escalation
     kubernaut.ai/severity: critical
     kubernaut.ai/skip-reason: PreviousExecutionFailed
   ```
3. **Notification Service** routes based on labels → delivers to appropriate channels

---

#### **Q2: Should the notification message be different from `ExhaustedRetries`?**

**Answer**: ✅ **Yes, and this is RO's responsibility.**

Your differentiation table is exactly right:

| Skip Reason | Notification Type | Message Focus | Suggested Priority |
|-------------|-------------------|---------------|-------------------|
| `ExhaustedRetries` | `escalation` | "Infrastructure issues persisting, retries exhausted" | `high` |
| `PreviousExecutionFailed` | `escalation` | "Workflow ran and failed, cluster state unknown" | `critical` |

**Key differences RO should include in the NotificationRequest**:

| Field | `ExhaustedRetries` | `PreviousExecutionFailed` |
|-------|-------------------|---------------------------|
| `spec.type` | `escalation` | `escalation` |
| `spec.priority` | `high` | `critical` |
| `spec.subject` | "⚠️ Workflow Retries Exhausted" | "🔴 Workflow Blocked: Execution Failed" |
| `spec.body` | Focus on infrastructure/quota issues | Focus on cluster state uncertainty |

---

#### **Q3: Should `PreviousExecutionFailed` notifications be higher priority than `ExhaustedRetries`?**

**Answer**: ✅ **Yes, we recommend `critical` vs `high`.**

**Rationale**:

| Skip Reason | Severity | Why |
|-------------|----------|-----|
| `ExhaustedRetries` | `high` | Infrastructure problem, but cluster state is **known** (no modifications made) |
| `PreviousExecutionFailed` | `critical` | Cluster state is **unknown/partially modified** - potential for cascading damage |

**Routing implications** (via BR-NOT-065):
- `critical` notifications can be routed to PagerDuty for immediate alerting
- `high` notifications can be routed to Slack for team awareness

**Example routing config**:
```yaml
route:
  routes:
    - match:
        kubernaut.ai/skip-reason: PreviousExecutionFailed
        kubernaut.ai/severity: critical
      receiver: pagerduty-oncall  # Immediate escalation
    - match:
        kubernaut.ai/skip-reason: ExhaustedRetries
        kubernaut.ai/severity: high
      receiver: slack-ops  # Team notification
```

---

### Notification Team Follow-up Questions

#### **Q7 (for WE Team): Should we add a dedicated routing label for skip reasons?**

We currently support these labels for routing (BR-NOT-065):
- `kubernaut.ai/notification-type`
- `kubernaut.ai/severity`
- `kubernaut.ai/environment`
- `kubernaut.ai/priority`

**Question**: Should we add `kubernaut.ai/skip-reason` as a routing label?

**Use case**: Allows operators to configure different notification channels specifically for `PreviousExecutionFailed` vs `ExhaustedRetries`.

**Our recommendation**: ✅ Yes, add it. This provides fine-grained routing control.

> ### ✅ **WE Team Response (Q7)** (2025-12-06)
>
> **Response**: ⏭️ **WE Team has no input on this decision.**
>
> **Rationale**: This question concerns:
> 1. **NotificationRequest CRD schema** - Owned by Notification Team
> 2. **What labels RO sets on NotificationRequest** - Owned by RO Team
>
> **WE's responsibility** is limited to correctly setting `WorkflowExecution.Status.SkipDetails.Reason`, which we already do per DD-WE-004 v1.1.
>
> **Data flow**:
> ```
> WE sets: wfe.Status.SkipDetails.Reason = "PreviousExecutionFailed"
>        │
>        ▼
> RO reads WFE status
>        │
>        ▼
> RO creates NotificationRequest (RO decides labels)
>        │
>        ▼
> Notification Service routes (Notification team defines supported labels)
> ```
>
> **Recommendation**: Notification Team should:
> 1. Decide if `kubernaut.ai/skip-reason` should be a supported routing label (API decision)
> 2. Notify RO Team of the decision so they can implement accordingly
>
> WE will continue to provide accurate skip reasons in the WFE status.

---

#### **Q8 (for RO Team): Will you set routing labels when creating NotificationRequests?**

For the label-based routing (BR-NOT-065) to work correctly, RO needs to set appropriate labels on the NotificationRequest:

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: nr-execution-blocked-xyz
  labels:
    kubernaut.ai/notification-type: escalation
    kubernaut.ai/severity: critical
    kubernaut.ai/skip-reason: PreviousExecutionFailed  # NEW - if approved
    kubernaut.ai/environment: production
    kubernaut.ai/remediation-request: rr-12345
spec:
  type: escalation
  priority: critical
  # ... rest of spec
```

**Question**: Will RO set these labels, or should Notification Service infer them from `spec` fields?

> ### ✅ **RO Team Answer (Q8)** (2025-12-06)
>
> **Answer**: ✅ **Yes, RO will set all routing labels.**
>
> Per our Day 5 implementation plan (DAYS_02_07_PHASE_HANDLERS.md v1.3), RO already sets these labels when creating NotificationRequests for manual review scenarios:
>
> ```go
> // From handleManualReviewRequired() in handler/workflowexecution.go
> Labels: map[string]string{
>     "kubernaut.ai/remediation-request": rr.Name,
>     "kubernaut.ai/notification-type":   "manual-review",
>     "kubernaut.ai/skip-reason":         skipReason,  // ← Already included
> },
> ```
>
> **Full label set RO will provide**:
>
> | Label | Source | Example |
> |-------|--------|---------|
> | `kubernaut.ai/remediation-request` | `rr.Name` | `rr-payment-oom-001` |
> | `kubernaut.ai/notification-type` | Hardcoded per scenario | `manual-review`, `approval`, `escalation` |
> | `kubernaut.ai/skip-reason` | `we.Status.SkipDetails.Reason` | `PreviousExecutionFailed`, `ExhaustedRetries` |
> | `kubernaut.ai/severity` | Mapped from priority | `critical`, `high`, `medium`, `low` |
> | `kubernaut.ai/environment` | `rr.Spec.Environment` | `production`, `staging` |
> | `kubernaut.ai/component` | Hardcoded | `remediation-orchestrator` |
>
> **Preference**: RO sets labels explicitly. This is cleaner than inferring from spec fields because:
> 1. Labels are the Kubernetes-native way to express routing metadata
> 2. Spec fields may have different semantics (e.g., `priority` affects delivery, not just routing)
> 3. Explicit labels make routing rules more readable
>
> **Action**: We will update all NotificationRequest creators to include the full label set.

---

> ### ✅ **RO Team Input on Q7** (2025-12-06)
>
> **Question**: Should Notification Service add `kubernaut.ai/skip-reason` as a routing label?
>
> **RO Answer**: ✅ **Yes, strongly support this.**
>
> **Rationale**:
> 1. RO is already setting this label (per Q8 answer)
> 2. Operators need fine-grained routing control per skip reason
> 3. `PreviousExecutionFailed` (cluster state unknown) requires different escalation than `ExhaustedRetries` (infra issue)
>
> **Use case example**:
> ```yaml
> # Operator wants PagerDuty only for execution failures, Slack for others
> route:
>   routes:
>     - match:
>         kubernaut.ai/skip-reason: PreviousExecutionFailed
>       receiver: pagerduty-oncall
>     - match:
>         kubernaut.ai/skip-reason: ExhaustedRetries
>       receiver: slack-ops
> ```
>
> **Recommendation**: Add `kubernaut.ai/skip-reason` to BR-NOT-065 supported labels.

---

### Impact on Notification Documentation

Documentation has been updated to support skip-reason label routing:

| Document | Status |
|----------|--------|
| BR-NOT-065 (Label Routing) | ✅ Updated with skip-reason label |
| BR-NOT-066 (Alertmanager Config) | ✅ Already implemented |
| `pkg/notification/routing/labels.go` | ✅ Added `LabelSkipReason` constant |
| CRD Schema | ✅ Already supports `escalation` type and `critical` priority |
| Enhancement Plan | ✅ Created `ENHANCEMENT_BR-NOT-065-SKIP-REASON-LABEL.md` |

---

### Timeline

| Milestone | Target | Status |
|-----------|--------|--------|
| Acknowledge notice | 2025-12-06 | ✅ Done |
| Confirm template approach | 2025-12-06 | ✅ Done (RO owns templates) |
| Label routing ready | 2025-12-06 | ✅ Already implemented |
| Add `skip-reason` label support | 2025-12-06 | ✅ **IMPLEMENTED** |
| Unit tests for skip-reason routing | Day 13 | ✅ **COMPLETE** (9 tests) |
| Controller integration | Day 13 | ✅ **COMPLETE** (resolveChannelsFromRouting) |
| Example routing config | Day 13 | ✅ **COMPLETE** (`notification_routing_config.yaml`) |
| Skip-reason runbook | Day 13 | ✅ **COMPLETE** (`SKIP_REASON_ROUTING.md`) |
| Mandatory labels in BR-NOT-065 | Day 13 | ✅ **COMPLETE** |

### Skip-Reason Label Implementation (2025-12-06)

**Files Updated**:
1. `pkg/notification/routing/labels.go` - Added constants:
   ```go
   LabelSkipReason = "kubernaut.ai/skip-reason"

   SkipReasonPreviousExecutionFailed = "PreviousExecutionFailed"  // CRITICAL
   SkipReasonExhaustedRetries        = "ExhaustedRetries"         // HIGH
   SkipReasonResourceBusy            = "ResourceBusy"             // LOW
   SkipReasonRecentlyRemediated      = "RecentlyRemediated"       // LOW
   ```

2. `docs/services/crd-controllers/06-notification/BUSINESS_REQUIREMENTS.md` - Updated BR-NOT-065

3. `docs/services/crd-controllers/06-notification/implementation/ENHANCEMENT_BR-NOT-065-SKIP-REASON-LABEL.md` - Created enhancement plan for Day 13

**Day 13 Complete** (2025-12-06):
- ✅ Unit tests for skip-reason routing (9 tests) - `test/unit/notification/routing_config_test.go`
- ✅ Controller integration (resolveChannelsFromRouting) - `internal/controller/notification/notificationrequest_controller.go`
- ✅ Example routing config - `config/samples/notification_routing_config.yaml`
- ✅ Skip-reason runbook - `docs/services/crd-controllers/06-notification/runbooks/SKIP_REASON_ROUTING.md`
- ✅ Mandatory labels in BR-NOT-065 - `BUSINESS_REQUIREMENTS.md`

---

**Issued By**: Notification Team
**Date**: 2025-12-06

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.5 | 2025-12-06 | **Day 13 Complete**: Unit tests (9), controller integration (resolveChannelsFromRouting), example config, runbook, mandatory labels in BR-NOT-065 |
| 1.4 | 2025-12-06 | Notification Team: Implemented `skip-reason` label support in `pkg/notification/routing/labels.go`, updated BR-NOT-065, created enhancement plan for Day 13 |
| 1.3 | 2025-12-06 | WE Team responses to RO questions Q2-Q6: No duplication (Q2), defer clearing to v1.1 as BR-WE-013 (Q3), precedence confirmed (Q4), per-target scope (Q5), RO requeue approach (Q6) |
| 1.2 | 2025-12-06 | WE Team response to Q7: Clarified ownership - WE has no input on NotificationRequest labels |
| 1.1 | 2025-12-06 | Notification Team acknowledgment and responses |
| 1.0 | 2025-12-06 | Initial notice for DD-WE-004 v1.1 |

