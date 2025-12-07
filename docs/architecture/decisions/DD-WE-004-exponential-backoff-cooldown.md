# DD-WE-004: Exponential Backoff Cooldown

**Version**: 1.1
**Created**: 2025-12-06
**Last Updated**: 2025-12-06
**Status**: ✅ APPROVED
**Related BR**: BR-WE-012

---

## Context

The WorkflowExecution Controller implements resource locking (DD-WE-001) with a fixed cooldown period (default 5 minutes) to prevent redundant sequential executions. However, when a workflow experiences **pre-execution failures** (infrastructure issues) repeatedly on the same target resource, this fixed cooldown can lead to:

1. **Remediation storms**: Rapid retry cycles that waste resources
2. **Alert fatigue**: Repeated failure notifications without meaningful progress
3. **Resource exhaustion**: Continuous PipelineRun creation and cleanup cycles

This design decision introduces **exponential backoff** for the cooldown period after consecutive **pre-execution failures only**, similar to patterns used in:
- Kubernetes pod restart policy (10s → 5min cap)
- gRPC retry policy (exponential with jitter)
- AWS SDK retry strategies

---

## Critical Distinction: Two Types of Failures

**IMPORTANT**: This design decision applies ONLY to pre-execution failures. Execution failures follow a different path per existing cross-team agreements.

| Failure Type | `wasExecutionFailure` | Retry Safe? | Action |
|--------------|----------------------|-------------|--------|
| **Pre-execution** | `false` | ✅ Yes | Exponential backoff + retry |
| **During-execution** | `true` | ❌ No | Fail immediately, notify, manual review |

**Rationale for distinction**:
- **Pre-execution failures** (validation, image pull, quota, Tekton unavailable): Workflow never started, no state changes, transient issues may resolve
- **During-execution failures** (task failed, timeout during run): Workflow ran (partially), state may have changed, non-idempotent actions may have occurred, retrying is dangerous

**Reference**: This distinction is established in:
- `controller-implementation.md` (lines 654-655)
- `QUESTIONS_FROM_WORKFLOW_ENGINE_TEAM.md` (WE→RO-003)
- `DECISIONS_HAPI_EXECUTION_RESPONSIBILITIES.md`

---

## Decision

Implement **exponential backoff cooldown** that increases the waiting period between retries after consecutive **pre-execution failures** (`wasExecutionFailure: false`), with automatic reset on success.

**Execution failures** (`wasExecutionFailure: true`) are **NOT retried** - they fail immediately and require manual review per existing cross-team agreements.

### Parameters

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| `BaseCooldownPeriod` | 1 minute | Reasonable initial wait, allows quick recovery |
| `MaxCooldownPeriod` | 10 minutes | Prevents RemediationRequest timeout (60m) |
| `MaxBackoffExponent` | 4 | 2^4 = 16x max multiplier (capped at 10 min) |
| `MaxConsecutiveFailures` | 5 | After 5 failures at max cooldown → auto-fail WFE |

### Formula

```
Cooldown = min(BaseCooldown × 2^(consecutiveFailures-1), MaxCooldown)
```

### Cooldown Progression (Pre-Execution Failures Only)

| Pre-Exec Failure # | Formula | Calculated | Applied | Cumulative Wait |
|--------------------|---------|------------|---------|-----------------|
| 1 | 1 × 2^0 | 1 min | 1 min | 1 min |
| 2 | 1 × 2^1 | 2 min | 2 min | 3 min |
| 3 | 1 × 2^2 | 4 min | 4 min | 7 min |
| 4 | 1 × 2^3 | 8 min | 8 min | 15 min |
| 5 | 1 × 2^4 | 16 min | **10 min** (capped) | 25 min |
| 6+ | - | - | **→ Skipped (ExhaustedRetries)** | - |

**Note**: This progression ONLY applies to pre-execution failures. Execution failures (`wasExecutionFailure: true`) immediately block future retries with `PreviousExecutionFailed`.

### Counter Scope

**Per Target Resource** (Option A):
- Failure counter is associated with the `targetResource`, not individual WFE instances
- Prevents bypass via creating new WFE with different names
- Aligns with DD-WE-003 (deterministic naming per target)
- Single source of truth for failure history

### Reset Behavior

**Immediate Reset on Success** (Industry Standard):
- When a workflow succeeds, `ConsecutiveFailures` resets to 0
- `NextAllowedExecution` is cleared
- Target resource gets a "fresh start"

---

## CRD Schema Changes

### WorkflowExecutionStatus (additions)

```go
type WorkflowExecutionStatus struct {
    // ... existing fields ...

    // ConsecutiveFailures tracks consecutive failures for this target resource
    // Resets to 0 on successful completion
    // Used for exponential backoff calculation
    // +optional
    ConsecutiveFailures int32 `json:"consecutiveFailures,omitempty"`

    // NextAllowedExecution is the earliest timestamp when execution is allowed
    // Calculated using exponential backoff: Base × 2^(failures-1)
    // +optional
    NextAllowedExecution *metav1.Time `json:"nextAllowedExecution,omitempty"`
}
```

### New Skip Reasons

```go
const (
    SkipReasonResourceBusy              = "ResourceBusy"
    SkipReasonRecentlyRemediated        = "RecentlyRemediated"
    SkipReasonExhaustedRetries          = "ExhaustedRetries"          // NEW: After MaxConsecutiveFailures (pre-execution only)
    SkipReasonPreviousExecutionFailed   = "PreviousExecutionFailed"   // NEW: Previous execution ran and failed (wasExecutionFailure: true)
)
```

**Skip Reason Decision Matrix**:

| Scenario | Skip Reason | Retry Possible? |
|----------|-------------|-----------------|
| Another WFE running on target | `ResourceBusy` | ✅ Wait for completion |
| Same workflow recently ran (success) | `RecentlyRemediated` | ✅ Wait for cooldown |
| Pre-execution failed, backoff active | `RecentlyRemediated` | ✅ Wait for backoff |
| Pre-execution failed 5+ times | `ExhaustedRetries` | ❌ Manual intervention |
| **Previous execution ran and failed** | `PreviousExecutionFailed` | ❌ **Manual intervention required** |

---

## Controller Implementation Changes

### Reconciler Configuration

```go
type WorkflowExecutionReconciler struct {
    client.Client
    // ... existing fields ...

    // Exponential backoff configuration (DD-WE-004)
    // ONLY applies to pre-execution failures (wasExecutionFailure: false)
    BaseCooldownPeriod     time.Duration  // Default: 1 minute
    MaxCooldownPeriod      time.Duration  // Default: 10 minutes
    MaxBackoffExponent     int            // Default: 4
    MaxConsecutiveFailures int            // Default: 5
}
```

### CheckCooldown Enhancement

```go
func (r *WorkflowExecutionReconciler) CheckCooldown(
    ctx context.Context,
    wfe *workflowexecutionv1alpha1.WorkflowExecution,
) (blocked bool, skipDetails *workflowexecutionv1alpha1.SkipDetails, recentWFE *workflowexecutionv1alpha1.WorkflowExecution) {
    // Find most recent terminal WFE for same target + workflow
    recentWFE = r.findMostRecentTerminalWFE(ctx, wfe)
    if recentWFE == nil {
        return false, nil, nil
    }

    // CRITICAL: Check if previous execution was a DURING-EXECUTION failure
    // Per cross-team agreement: wasExecutionFailure: true → NO RETRY
    if recentWFE.Status.Phase == PhaseFailed &&
       recentWFE.Status.FailureDetails != nil &&
       recentWFE.Status.FailureDetails.WasExecutionFailure {
        return true, &workflowexecutionv1alpha1.SkipDetails{
            Reason:  SkipReasonPreviousExecutionFailed,
            Message: fmt.Sprintf("Previous execution failed during workflow run on target %s. Manual intervention required. Non-idempotent actions may have occurred.",
                wfe.Spec.TargetResource),
            SkippedAt: metav1.Now(),
            RecentRemediation: &workflowexecutionv1alpha1.RecentRemediationRef{
                Name:           recentWFE.Name,
                WorkflowID:     recentWFE.Spec.WorkflowRef.WorkflowID,
                CompletedAt:    *recentWFE.Status.CompletionTime,
                Outcome:        string(recentWFE.Status.Phase),
                TargetResource: recentWFE.Spec.TargetResource,
            },
        }, recentWFE
    }

    // From here: Only pre-execution failures (wasExecutionFailure: false)
    // These are safe to retry with exponential backoff

    // Check if max consecutive (pre-execution) failures reached → auto-fail
    if recentWFE.Status.ConsecutiveFailures >= int32(r.MaxConsecutiveFailures) {
        return true, &workflowexecutionv1alpha1.SkipDetails{
            Reason:  SkipReasonExhaustedRetries,
            Message: fmt.Sprintf("Exhausted %d consecutive pre-execution retries for target resource %s",
                r.MaxConsecutiveFailures, wfe.Spec.TargetResource),
            SkippedAt: metav1.Now(),
        }, recentWFE
    }

    // Check exponential backoff using NextAllowedExecution
    if recentWFE.Status.NextAllowedExecution != nil {
        if time.Now().Before(recentWFE.Status.NextAllowedExecution.Time) {
            remaining := time.Until(recentWFE.Status.NextAllowedExecution.Time)
            return true, &workflowexecutionv1alpha1.SkipDetails{
                Reason:  SkipReasonRecentlyRemediated,
                Message: fmt.Sprintf("Backoff active, next execution allowed in %s", remaining.Round(time.Second)),
                SkippedAt: metav1.Now(),
                RecentRemediation: &workflowexecutionv1alpha1.RecentRemediationRef{
                    Name:              recentWFE.Name,
                    WorkflowID:        recentWFE.Spec.WorkflowRef.WorkflowID,
                    CompletedAt:       *recentWFE.Status.CompletionTime,
                    Outcome:           string(recentWFE.Status.Phase),
                    TargetResource:    recentWFE.Spec.TargetResource,
                    CooldownRemaining: remaining.Round(time.Second).String(),
                },
            }, recentWFE
        }
    }

    return false, nil, recentWFE
}
```

### MarkFailed Enhancement

```go
func (r *WorkflowExecutionReconciler) MarkFailed(
    ctx context.Context,
    wfe *workflowexecutionv1alpha1.WorkflowExecution,
    reason, message string,
    wasExecutionFailure bool,  // CRITICAL: Determines retry behavior
) (ctrl.Result, error) {
    now := metav1.Now()
    wfe.Status.Phase = workflowexecutionv1alpha1.PhaseFailed
    wfe.Status.CompletionTime = &now

    // Populate failure details
    wfe.Status.FailureDetails = &workflowexecutionv1alpha1.FailureDetails{
        Reason:              reason,
        Message:             message,
        FailedAt:            now,
        WasExecutionFailure: wasExecutionFailure,
        RequiresManualReview: wasExecutionFailure,  // Execution failures need manual review
    }

    // ONLY apply exponential backoff for PRE-EXECUTION failures
    // Execution failures (wasExecutionFailure: true) → no retry, no backoff
    if !wasExecutionFailure {
        // Increment consecutive failures (DD-WE-004)
        wfe.Status.ConsecutiveFailures++

        // Calculate next allowed execution with exponential backoff
        exponent := int(wfe.Status.ConsecutiveFailures) - 1
        if exponent > r.MaxBackoffExponent {
            exponent = r.MaxBackoffExponent
        }
        backoffDuration := r.BaseCooldownPeriod * time.Duration(1<<exponent)
        if backoffDuration > r.MaxCooldownPeriod {
            backoffDuration = r.MaxCooldownPeriod
        }
        nextAllowed := metav1.NewTime(now.Add(backoffDuration))
        wfe.Status.NextAllowedExecution = &nextAllowed

        // Record backoff metrics
        metrics.RecordConsecutiveFailures(wfe.Spec.TargetResource, wfe.Status.ConsecutiveFailures)
    }
    // else: Execution failure - no backoff tracking, will be blocked by wasExecutionFailure check

    metrics.RecordWorkflowFailure(/* duration */)

    return ctrl.Result{}, r.Status().Update(ctx, wfe)
}
```

### MarkCompleted Enhancement

```go
func (r *WorkflowExecutionReconciler) MarkCompleted(
    ctx context.Context,
    wfe *workflowexecutionv1alpha1.WorkflowExecution,
    pr *tektonv1.PipelineRun,
) (ctrl.Result, error) {
    now := metav1.Now()
    wfe.Status.Phase = workflowexecutionv1alpha1.PhaseCompleted
    wfe.Status.CompletionTime = &now

    // Reset consecutive failures on success (DD-WE-004)
    wfe.Status.ConsecutiveFailures = 0
    wfe.Status.NextAllowedExecution = nil

    // Record metric
    metrics.RecordWorkflowCompletion(/* duration */)
    metrics.RecordConsecutiveFailuresReset(wfe.Spec.TargetResource)

    return ctrl.Result{}, r.Status().Update(ctx, wfe)
}
```

---

## Metrics

### New Metrics (BR-WE-008 Extension)

```go
var (
    // WorkflowBackoffSkipTotal counts executions skipped due to exponential backoff
    WorkflowBackoffSkipTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "workflowexecution_backoff_skip_total",
        Help: "Total workflow executions skipped due to exponential backoff.",
    }, []string{"reason"}) // reason: RecentlyRemediated, ExhaustedRetries

    // WorkflowConsecutiveFailures tracks current consecutive failure count per target
    WorkflowConsecutiveFailures = promauto.NewGaugeVec(prometheus.GaugeOpts{
        Name: "workflowexecution_consecutive_failures",
        Help: "Current consecutive failure count per target resource.",
    }, []string{"target_resource"})
)
```

---

## State Machine

```
                    ┌──────────────────────────────────────────────────────────────┐
                    │                                                              │
                    ▼                                                              │
┌─────────┐    ┌─────────┐    ┌─────────┐                                         │
│ Pending │───▶│ Running │───▶│Completed│                                         │
└─────────┘    └─────────┘    └─────────┘                                         │
     │              │              │                                               │
     │              │              ▼                                               │
     │              │      Reset to 0                                              │
     │              │      NextAllowedExecution = nil                              │
     │              │              │                                               │
     │              ▼              │                                               │
     │    ┌────────────────────┐  │                                               │
     │    │ Failed             │  │                                               │
     │    │ (wasExecFail?)     │  │                                               │
     │    └────────────────────┘  │                                               │
     │         │         │        │                                               │
     │         │         │        │                                               │
     │    ┌────┴────┐    │        │                                               │
     │    │         │    │        │                                               │
     │    ▼         ▼    │        │                                               │
     │  FALSE     TRUE   │        │                                               │
     │    │         │    │        │                                               │
     │    ▼         ▼    │        │                                               │
     │ ┌──────────────────────┐   │                                               │
     │ │ Pre-Execution Fail   │   │      ┌─────────────────────────────────────┐  │
     │ │ (wasExecFail=false)  │   │      │ Execution Fail (wasExecFail=true)   │  │
     │ │                      │   │      │                                     │  │
     │ │ ConsecutiveFailures++│   │      │ NO retry, NO backoff               │  │
     │ │ NextAllowedExecution │   │      │ requiresManualReview = true        │  │
     │ │    calculated        │   │      │                                     │  │
     │ └──────────────────────┘   │      │ Next WFE → Skipped                 │  │
     │         │                  │      │ (PreviousExecutionFailed)           │  │
     │         │                  │      └─────────────────────────────────────┘  │
     │         │                  │                                               │
     │         ▼                  │                                               │
     │  ┌─────────────────┐      │                                               │
     │  │ failures >= 5?  │      │                                               │
     │  └─────────────────┘      │                                               │
     │    │           │          │                                               │
     │   YES          NO         │                                               │
     │    │           │          │                                               │
     │    ▼           │          │                                               │
     │ ┌───────────┐  │          │                                               │
     │ │ Skipped   │  │          │                                               │
     │ │(Exhausted)│  │          │                                               │
     │ └───────────┘  │          │                                               │
     │                │          │                                               │
     └────────────────┴──────────┴───────────────────────────────────────────────┘
           (new WFE creation respects backoff/block from previous WFE)
```

**Key Distinction**:
- **Pre-execution failure** (`wasExecutionFailure: false`): Exponential backoff applies, retry allowed
- **Execution failure** (`wasExecutionFailure: true`): No backoff, no retry, manual intervention required

---

## Edge Cases

### 1. First Failure
- `ConsecutiveFailures` = 1
- `NextAllowedExecution` = now + 1 minute

### 2. Success After Multiple Failures
- `ConsecutiveFailures` = 0 (reset)
- `NextAllowedExecution` = nil (cleared)
- Fresh start for next failure sequence

### 3. Multiple WFEs Targeting Same Resource
- Only the most recent terminal WFE is checked
- New WFE inherits failure count context

### 4. WFE Deletion
- When WFE is deleted, its failure count is lost
- This is acceptable: deletion implies intentional reset
- Prevents "stuck" resources from accumulating forever

### 5. Controller Restart
- State persisted in CRD status fields
- No in-memory state required
- Full recovery on restart

### 6. Max Failures Reached
- WFE transitions to `Skipped` with `ExhaustedRetries` reason
- Notification triggered for operator intervention
- Manual reset required (delete old WFEs)

---

## Configuration

### Default Values

```go
const (
    DefaultBaseCooldownPeriod     = 1 * time.Minute
    DefaultMaxCooldownPeriod      = 10 * time.Minute
    DefaultMaxBackoffExponent     = 4
    DefaultMaxConsecutiveFailures = 5
)
```

### ConfigMap Override (ADR-030)

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: workflowexecution-config
  namespace: kubernaut-system
data:
  base-cooldown-period: "1m"
  max-cooldown-period: "10m"
  max-backoff-exponent: "4"
  max-consecutive-failures: "5"
```

---

## Alternatives Considered

### 1. Fixed Cooldown (Current)
- **Pro**: Simple implementation
- **Con**: Doesn't prevent remediation storms
- **Rejected**: Doesn't solve the problem

### 2. Linear Backoff
- **Pro**: Simpler formula
- **Con**: Grows too slowly for repeated failures
- **Rejected**: Exponential is industry standard

### 3. Circuit Breaker
- **Pro**: Complete halt after threshold
- **Con**: Requires manual intervention to resume
- **Deferred**: Can add in v1.1 if needed

### 4. Counter Per WFE Instance
- **Pro**: Isolated per execution
- **Con**: Easily bypassed by creating new WFE
- **Rejected**: Doesn't provide target protection

---

## Industry Alignment

| System | Base | Factor | Max | Reset |
|--------|------|--------|-----|-------|
| **Kubernetes Pod Restart** | 10s | 2x | 5 min | On success |
| **gRPC Retry** | 1s | 1.6x + jitter | 120s | On success |
| **AWS SDK** | 100ms | 2x | varies | On success |
| **Kubernaut (this decision)** | 1 min | 2x | 10 min | On success |

---

## Testing Requirements

### Unit Tests
- [ ] `CalculateBackoff()` with various failure counts
- [ ] `CheckCooldown()` with `NextAllowedExecution` set
- [ ] `MarkFailed()` increments counter and sets next allowed
- [ ] `MarkCompleted()` resets counter and clears next allowed
- [ ] Max consecutive failures triggers `ExhaustedRetries`
- [ ] Counter capped at `MaxBackoffExponent`

### Integration Tests
- [ ] Backoff progression across multiple WFE failures
- [ ] Reset on success scenario
- [ ] `ExhaustedRetries` skip scenario

---

## Confidence Assessment

```
Validation Confidence: 95%
Industry Alignment:    ✅ Follows K8s, gRPC, AWS patterns
User Alignment:        ✅ Parameters user-approved
Technical Feasibility: ✅ Uses existing CRD status pattern
Risk Assessment:       Low - all state in CRD, survives restart
```

---

## References

- [DD-WE-001: Resource Locking Safety](./DD-WE-001-resource-locking-safety.md)
- [DD-WE-003: Resource Lock Persistence](./DD-WE-003-resource-lock-persistence.md)
- [BR-WE-012: Exponential Backoff Cooldown](../../services/crd-controllers/03-workflowexecution/BUSINESS_REQUIREMENTS.md)
- [Kubernetes Pod Restart Policy](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#restart-policy)
- [gRPC Retry Design](https://github.com/grpc/proposal/blob/master/A6-client-retries.md)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.1 | 2025-12-06 | **Critical refinement**: Exponential backoff now ONLY applies to pre-execution failures (`wasExecutionFailure: false`). Execution failures (`wasExecutionFailure: true`) are blocked immediately with `PreviousExecutionFailed` reason - no retry allowed. Added explicit distinction and references to cross-team agreements. |
| 1.0 | 2025-12-06 | Initial design - exponential backoff cooldown |

