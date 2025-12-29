# RO Routing Requirements for WE Integration

**Date**: 2025-12-17
**From**: WorkflowExecution Team
**To**: RemediationOrchestrator Team
**Purpose**: Define RO's routing responsibilities before creating WorkflowExecution
**Status**: ✅ **READY FOR RO IMPLEMENTATION**

---

## 🎯 **Executive Summary**

**WE Controller State**: ✅ **"Pure Executor"** - Zero routing logic

**RO Responsibility**: **ALL routing decisions** BEFORE creating WorkflowExecution

**Handoff Point**: If `WorkflowExecution` CRD exists → WE executes without checks

**Design Decision**: DD-RO-002 (Centralized Routing Responsibility)

**Timeline**: RO Days 2-5 (Dec 17-20) → Integration Tests Days 8-9 (Dec 21-22)

---

## 📋 **RO's Mandatory Routing Checks**

### **Decision Flow** (BEFORE Creating WorkflowExecution)

```
RO Controller (Executing Phase)
│
├─ **Check 1: Resource Lock**
│   Query: Does PipelineRun exist for targetResource?
│   If YES: Skip workflow (resource busy)
│   Field: WorkflowExecution.spec.targetResource
│
├─ **Check 2: Cooldown**
│   Query: Find recent terminal WFE for same target+workflow
│   Check: CompletionTime + cooldown > now?
│   If YES: Skip workflow (cooldown active)
│
├─ **Check 3: Exponential Backoff**
│   Check: Previous WFE failed? Count ConsecutiveFailures
│   Calculate: NextAllowedExecution
│   If NOW < NextAllowedExecution: Skip workflow (backoff active)
│
├─ **Check 4: Exhausted Retries**
│   Check: ConsecutiveFailures >= max threshold?
│   If YES: Skip workflow, create manual review notification
│
├─ **Check 5: Previous Execution Failure**
│   Check: Most recent WFE has WasExecutionFailure=true?
│   If YES: Skip workflow, create manual review notification
│
└─ **DECISION**:
    ├─ If ANY check fails → DO NOT create WFE
    │   → Populate RR.Status.skipMessage
    │   → Populate RR.Status.blockingWorkflowExecution
    │
    └─ If ALL checks pass → CREATE WorkflowExecution
        → RR transitions to Executing phase
        → WE controller picks up WFE and executes
```

---

## 🔍 **Detailed Check Specifications**

### **Check 1: Resource Lock** (Critical - Prevents Concurrent Execution)

**Purpose**: Ensure only ONE workflow runs per target resource at a time

**Query**:
```go
// Check if PipelineRun exists for target resource
prName := fmt.Sprintf("wfe-%s", hash(wfe.Spec.TargetResource)[:16])
pr := &tektonv1.PipelineRun{}
err := r.Get(ctx, types.NamespacedName{
    Namespace: "kubernaut-workflows",  // DD-WE-002: Execution namespace
    Name:      prName,
}, pr)

if err == nil {
    // PipelineRun exists - resource is locked
    return RoutingDecision{
        Action:  SkipWorkflow,
        Reason:  "ResourceBusy",
        Message: fmt.Sprintf("Resource %s is locked by PipelineRun %s", targetResource, prName),
    }
}

if !apierrors.IsNotFound(err) {
    // Error querying K8s API
    return RoutingDecision{
        Action: RetryLater,
        Reason: "APIError",
    }
}

// PipelineRun not found - resource is free
return RoutingDecision{Action: ContinueChecks}
```

**Field Index Required**: ❌ **NOT NEEDED** (deterministic name lookup)

**RR.Status Population**:
```go
rr.Status.BlockReason = "ResourceBusy"
rr.Status.BlockMessage = "Resource <target> is locked by PipelineRun <prName>"
rr.Status.BlockingWorkflowExecution = "<wfe-name-if-found>"  // Optional
```

**Lock Properties**:
- **Name**: `wfe-<sha256(targetResource)[:16]>`
- **Namespace**: `kubernaut-workflows`
- **Lifecycle**: Created by WE, checked by RO
- **Collision Prevention**: Deterministic name ensures atomicity (DD-WE-003)

---

### **Check 2: Cooldown** (Prevents Frequent Re-execution)

**Purpose**: Enforce cooldown period after workflow completion

**Query**:
```go
// Find most recent terminal WFE for same target + workflow
wfeList := &workflowexecutionv1alpha1.WorkflowExecutionList{}
err := r.List(ctx, wfeList,
    client.MatchingFields{
        "spec.targetResource": targetResource,   // FIELD INDEX REQUIRED
        "spec.workflowName":   workflowName,     // FIELD INDEX REQUIRED
    },
)

// Filter for terminal phases (Completed or Failed)
var mostRecent *workflowexecutionv1alpha1.WorkflowExecution
for i := range wfeList.Items {
    wfe := &wfeList.Items[i]
    if wfe.Status.Phase != "Completed" && wfe.Status.Phase != "Failed" {
        continue  // Skip non-terminal WFEs
    }
    if wfe.Status.CompletionTime == nil {
        continue  // Skip WFEs with nil CompletionTime (data inconsistency)
    }
    if mostRecent == nil || wfe.Status.CompletionTime.After(mostRecent.Status.CompletionTime.Time) {
        mostRecent = wfe
    }
}

if mostRecent != nil {
    elapsed := time.Since(mostRecent.Status.CompletionTime.Time)
    cooldown := getCooldownPeriod(workflowName)  // From workflow configuration

    if elapsed < cooldown {
        remaining := cooldown - elapsed
        return RoutingDecision{
            Action:  SkipWorkflow,
            Reason:  "CooldownActive",
            Message: fmt.Sprintf("Cooldown active: %s remaining", remaining),
        }
    }
}

return RoutingDecision{Action: ContinueChecks}
```

**Field Index Required**: ✅ **YES** - `WorkflowExecution.spec.targetResource`

**Field Index Setup**:
```go
// In SetupWithManager()
if err := mgr.GetFieldIndexer().IndexField(
    ctx,
    &workflowexecutionv1alpha1.WorkflowExecution{},
    "spec.targetResource",
    func(obj client.Object) []string {
        wfe := obj.(*workflowexecutionv1alpha1.WorkflowExecution)
        return []string{wfe.Spec.TargetResource}
    },
); err != nil {
    return err
}
```

**RR.Status Population**:
```go
rr.Status.BlockReason = "CooldownActive"
rr.Status.BlockMessage = fmt.Sprintf("Cooldown active: %s remaining", remaining)
rr.Status.BlockedUntil = &metav1.Time{Time: mostRecent.Status.CompletionTime.Add(cooldown)}
rr.Status.BlockingWorkflowExecution = mostRecent.Name
```

**Cooldown Source**: Per-workflow configuration (DD-WE-001, now superseded by DD-RO-002)

---

### **Check 3: Exponential Backoff** (Rate-limits Failed Workflows)

**Purpose**: Progressively delay retries after consecutive failures

**Query**:
```go
// Reuse mostRecent WFE from cooldown check
if mostRecent != nil && mostRecent.Status.Phase == "Failed" {
    consecutiveFailures := mostRecent.Status.ConsecutiveFailures

    if mostRecent.Status.NextAllowedExecution != nil {
        if time.Now().Before(mostRecent.Status.NextAllowedExecution.Time) {
            remaining := time.Until(mostRecent.Status.NextAllowedExecution.Time)
            return RoutingDecision{
                Action:  SkipWorkflow,
                Reason:  "ExponentialBackoff",
                Message: fmt.Sprintf("Backoff active: %s remaining (failure #%d)", remaining, consecutiveFailures),
            }
        }
    }
}

return RoutingDecision{Action: ContinueChecks}
```

**Field Index Required**: ❌ **REUSES COOLDOWN QUERY** (same index)

**RR.Status Population**:
```go
rr.Status.BlockReason = "ExponentialBackoff"
rr.Status.BlockMessage = fmt.Sprintf("Backoff active: %s remaining (failure #%d)", remaining, consecutiveFailures)
rr.Status.BlockedUntil = mostRecent.Status.NextAllowedExecution
rr.Status.BlockingWorkflowExecution = mostRecent.Name
```

**Backoff Calculation**: Use shared backoff library (`pkg/shared/backoff`)

**Configuration**:
```go
import "github.com/jordigilh/kubernaut/pkg/shared/backoff"

backoffConfig := backoff.Config{
    BasePeriod:    5 * time.Minute,   // Start with 5min delay
    MaxPeriod:     4 * time.Hour,     // Cap at 4 hours
    Multiplier:    2.0,               // Power-of-2 exponential
    JitterPercent: 10,                // ±10% jitter (anti-thundering herd)
}

backoffDuration := backoffConfig.Calculate(consecutiveFailures)
nextAllowedExecution := mostRecent.Status.CompletionTime.Add(backoffDuration)
```

---

### **Check 4: Exhausted Retries** (Requires Manual Intervention)

**Purpose**: Stop automated retries after max consecutive failures

**Query**:
```go
// Reuse mostRecent WFE from cooldown check
if mostRecent != nil && mostRecent.Status.Phase == "Failed" {
    maxRetries := getMaxRetries(workflowName)  // From workflow configuration

    if mostRecent.Status.ConsecutiveFailures >= maxRetries {
        return RoutingDecision{
            Action:  SkipWorkflow,
            Reason:  "ExhaustedRetries",
            Message: fmt.Sprintf("Max retries (%d) exhausted - manual review required", maxRetries),
            RequiresManualReview: true,
        }
    }
}

return RoutingDecision{Action: ContinueChecks}
```

**Field Index Required**: ❌ **REUSES COOLDOWN QUERY** (same index)

**RR.Status Population**:
```go
rr.Status.BlockReason = "ExhaustedRetries"
rr.Status.BlockMessage = fmt.Sprintf("Max retries (%d) exhausted - manual review required", maxRetries)
rr.Status.BlockingWorkflowExecution = mostRecent.Name

// Create manual review notification
notification := &notificationv1alpha1.Notification{
    Spec: notificationv1alpha1.NotificationSpec{
        Type:     "ManualReviewRequired",
        Severity: "High",
        Message:  fmt.Sprintf("WorkflowExecution %s failed %d times - manual review required", workflowName, maxRetries),
        TargetResource: targetResource,
    },
}
_ = r.Create(ctx, notification)
```

**Default Max Retries**: 5 (configurable per workflow)

---

### **Check 5: Previous Execution Failure** (Blocks Failed Workflows)

**Purpose**: Prevent re-execution if previous attempt had execution-time failure

**Query**:
```go
// Reuse mostRecent WFE from cooldown check
if mostRecent != nil && mostRecent.Status.Phase == "Failed" {
    if mostRecent.Status.FailureDetails != nil && mostRecent.Status.FailureDetails.WasExecutionFailure {
        return RoutingDecision{
            Action:  SkipWorkflow,
            Reason:  "PreviousExecutionFailed",
            Message: fmt.Sprintf("Previous execution failed: %s", mostRecent.Status.FailureDetails.Reason),
            RequiresManualReview: true,
        }
    }
}

return RoutingDecision{Action: ContinueChecks}
```

**Field Index Required**: ❌ **REUSES COOLDOWN QUERY** (same index)

**RR.Status Population**:
```go
rr.Status.BlockReason = "PreviousExecutionFailed"
rr.Status.BlockMessage = fmt.Sprintf("Previous execution failed: %s", mostRecent.Status.FailureDetails.Reason)
rr.Status.BlockingWorkflowExecution = mostRecent.Name

// Create manual review notification
notification := &notificationv1alpha1.Notification{
    Spec: notificationv1alpha1.NotificationSpec{
        Type:     "ManualReviewRequired",
        Severity: "Critical",
        Message:  fmt.Sprintf("WorkflowExecution %s had execution failure - manual review required", workflowName),
        Details:  mostRecent.Status.FailureDetails.Message,
        TargetResource: targetResource,
    },
}
_ = r.Create(ctx, notification)
```

**Execution Failure**: PipelineRun ran but failed (vs pre-execution failure like validation error)

---

## 📊 **Field Index Requirements**

### **Required Field Index**

**Single Index Required**:
```go
// Field: WorkflowExecution.spec.targetResource
// Used by: Cooldown check, exponential backoff, exhausted retries, previous execution failure
```

**Setup Code** (in `remediationorchestrator_controller.go`):
```go
func (r *RemediationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Field index for WorkflowExecution queries
    if err := mgr.GetFieldIndexer().IndexField(
        context.Background(),
        &workflowexecutionv1alpha1.WorkflowExecution{},
        "spec.targetResource",
        func(obj client.Object) []string {
            wfe := obj.(*workflowexecutionv1alpha1.WorkflowExecution)
            return []string{wfe.Spec.TargetResource}
        },
    ); err != nil {
        return fmt.Errorf("failed to setup field index on WorkflowExecution.spec.targetResource: %w", err)
    }

    return ctrl.NewControllerManagedBy(mgr).
        For(&remediationrequestv1alpha1.RemediationRequest{}).
        Owns(&workflowexecutionv1alpha1.WorkflowExecution{}).
        Complete(r)
}
```

**Performance**: Index enables efficient queries (O(log n) vs O(n) linear scan)

---

## 🚫 **What RO Must NOT Do**

### **DO NOT Execute Workflows Directly** ❌

```go
// ❌ BAD: RO should NEVER create PipelineRuns directly
pr := &tektonv1.PipelineRun{...}
r.Create(ctx, pr)  // WRONG - This is WE's responsibility
```

**Correct Pattern**:
```go
// ✅ GOOD: RO creates WorkflowExecution, WE creates PipelineRun
wfe := &workflowexecutionv1alpha1.WorkflowExecution{
    Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
        TargetResource: targetResource,
        WorkflowName:   workflowName,
        Parameters:     parameters,
    },
}
r.Create(ctx, wfe)  // CORRECT - WE will handle execution
```

---

### **DO NOT Manage PipelineRun Lifecycle** ❌

```go
// ❌ BAD: RO should NEVER delete PipelineRuns
pr := &tektonv1.PipelineRun{...}
r.Delete(ctx, pr)  // WRONG - This is WE's responsibility
```

**Correct Pattern**:
```go
// ✅ GOOD: WE manages PipelineRun lifecycle (create, watch, delete)
// RO only checks if PipelineRun EXISTS (for resource lock check)
```

---

### **DO NOT Update WorkflowExecution Status** ❌

```go
// ❌ BAD: RO should NEVER update WFE status
wfe.Status.Phase = "Running"
r.Status().Update(ctx, wfe)  // WRONG - This is WE's responsibility
```

**Correct Pattern**:
```go
// ✅ GOOD: RO creates WFE, WE manages status
// RO can READ WFE status for routing decisions, but never writes
```

---

## ✅ **RO Implementation Checklist**

### **Phase 1: Setup** (1 hour)

- [ ] Create field index on `WorkflowExecution.spec.targetResource`
- [ ] Import shared backoff library (`pkg/shared/backoff`)
- [ ] Configure backoff parameters (base: 5min, max: 4h, multiplier: 2.0, jitter: 10%)
- [ ] Define max retries per workflow type (default: 5)

### **Phase 2: Resource Lock Check** (2 hours)

- [ ] Implement deterministic PipelineRun name calculation
- [ ] Query PipelineRun existence in `kubernaut-workflows` namespace
- [ ] Populate `RR.Status.BlockReason` = "ResourceBusy" on lock detected
- [ ] Add unit tests for lock detection

### **Phase 3: Cooldown Check** (3 hours)

- [ ] Query terminal WFEs using field index
- [ ] Filter by `targetResource` and `workflowName`
- [ ] Find most recent terminal WFE (CompletionTime sorting)
- [ ] Calculate elapsed time since completion
- [ ] Populate `RR.Status.BlockReason` = "CooldownActive" if in cooldown
- [ ] Add unit tests for cooldown scenarios

### **Phase 4: Exponential Backoff** (2 hours)

- [ ] Read `ConsecutiveFailures` and `NextAllowedExecution` from most recent WFE
- [ ] Compare current time with `NextAllowedExecution`
- [ ] Populate `RR.Status.BlockReason` = "ExponentialBackoff" if backoff active
- [ ] Add unit tests for backoff scenarios

### **Phase 5: Exhausted Retries** (2 hours)

- [ ] Check `ConsecutiveFailures` >= max threshold
- [ ] Create manual review notification if exhausted
- [ ] Populate `RR.Status.BlockReason` = "ExhaustedRetries"
- [ ] Add unit tests for exhausted retry scenarios

### **Phase 6: Previous Execution Failure** (2 hours)

- [ ] Check `FailureDetails.WasExecutionFailure` flag
- [ ] Create manual review notification if execution failed
- [ ] Populate `RR.Status.BlockReason` = "PreviousExecutionFailed"
- [ ] Add unit tests for execution failure scenarios

### **Phase 7: Integration** (4 hours)

- [ ] Integrate routing engine into RO reconciler
- [ ] Wire routing decision to WFE creation logic
- [ ] Ensure all 5 checks run in sequence
- [ ] Add integration tests for complete routing flow

**Total Estimated Effort**: **16 hours** (Days 2-5)

---

## 🧪 **Integration Test Scenarios**

### **Scenario 1: Happy Path** ✅

**Setup**:
- No existing PipelineRun for target
- No recent terminal WFE
- All routing checks pass

**Expected**:
- ✅ RO creates WorkflowExecution
- ✅ WE picks up WFE and executes
- ✅ PipelineRun created
- ✅ Workflow completes successfully

---

### **Scenario 2: Resource Busy** ❌

**Setup**:
- PipelineRun exists for target (created by another WFE)

**Expected**:
- ❌ RO does NOT create WorkflowExecution
- ✅ RR.Status.BlockReason = "ResourceBusy"
- ✅ RR.Status.BlockingWorkflowExecution = existing WFE name

---

### **Scenario 3: Cooldown Active** ❌

**Setup**:
- Recent terminal WFE completed 2 minutes ago
- Cooldown period is 5 minutes

**Expected**:
- ❌ RO does NOT create WorkflowExecution
- ✅ RR.Status.BlockReason = "CooldownActive"
- ✅ RR.Status.BlockedUntil = CompletionTime + 5min

---

### **Scenario 4: Exponential Backoff** ❌

**Setup**:
- Recent WFE failed (ConsecutiveFailures = 3)
- NextAllowedExecution = now + 20 minutes

**Expected**:
- ❌ RO does NOT create WorkflowExecution
- ✅ RR.Status.BlockReason = "ExponentialBackoff"
- ✅ RR.Status.BlockedUntil = NextAllowedExecution

---

### **Scenario 5: Exhausted Retries** ❌

**Setup**:
- Recent WFE failed (ConsecutiveFailures = 5)
- Max retries = 5

**Expected**:
- ❌ RO does NOT create WorkflowExecution
- ✅ RR.Status.BlockReason = "ExhaustedRetries"
- ✅ Manual review notification created

---

### **Scenario 6: Previous Execution Failure** ❌

**Setup**:
- Recent WFE failed with WasExecutionFailure = true

**Expected**:
- ❌ RO does NOT create WorkflowExecution
- ✅ RR.Status.BlockReason = "PreviousExecutionFailed"
- ✅ Manual review notification created

---

### **Scenario 7: Execution-Time Race** ⚠️

**Setup**:
- RO routing missed concurrent WFE creation
- Another WFE already created PipelineRun

**Expected**:
- ✅ RO creates WorkflowExecution (routing missed race)
- ⚠️ WE detects AlreadyExists during PipelineRun creation
- ✅ WE marks WFE as Failed with "ExecutionRaceCondition"
- ✅ Layer 2 safety activated (DD-WE-003)

**Note**: This is RARE but handled gracefully by WE

---

## 📞 **WE Team Support**

### **Questions Welcome**

**For routing behavior clarification**:
- What does WE expect in WFE.Spec?
- How does WE handle status fields?
- What happens if WE encounters execution-time collision?

**For integration testing**:
- How to validate RO-WE handoff?
- What integration test fixtures exist?
- How to test edge cases?

**Contact**: WorkflowExecution Team (@jgil)

---

## 🔗 **Reference Documents**

1. ✅ `DD-RO-002` - Centralized Routing Responsibility (design decision)
2. ✅ `DD-WE-001` (superseded) - Workflow-Specific Cooldown (historical context)
3. ✅ `DD-WE-003` - Lock Persistence via Deterministic Naming (Layer 2 safety)
4. ✅ `WE_PURE_EXECUTOR_VERIFICATION.md` - Evidence that WE has zero routing logic
5. ✅ `WE_PURE_EXECUTOR_STATUS_DEC_17_2025.md` - Current WE implementation status
6. ✅ `pkg/shared/backoff` - Shared exponential backoff library

---

## ✅ **Final Checklist for RO Team**

### **Before Starting Implementation**

- [ ] Read DD-RO-002 (Centralized Routing Responsibility)
- [ ] Review WE_PURE_EXECUTOR_VERIFICATION.md (understand WE state)
- [ ] Review this document (understand routing requirements)
- [ ] Confirm max retries configuration
- [ ] Confirm backoff configuration

### **During Implementation**

- [ ] Follow implementation checklist (Phases 1-7)
- [ ] Write unit tests for each routing check
- [ ] Test against real WorkflowExecution CRDs
- [ ] Validate RR.Status population patterns

### **Before Integration Tests**

- [ ] All 5 routing checks implemented
- [ ] Field index created and validated
- [ ] All unit tests passing
- [ ] RO routing engine integrated into reconciler
- [ ] Documentation updated

### **Integration Test Readiness**

- [ ] 7 integration test scenarios prepared
- [ ] Test fixtures created
- [ ] WE team notified of completion
- [ ] Ready for Days 8-9 joint testing

---

**Document Owner**: WorkflowExecution Team
**Date**: December 17, 2025
**Status**: ✅ **READY FOR RO IMPLEMENTATION**
**RO Timeline**: Days 2-5 (Dec 17-20)
**Integration Tests**: Days 8-9 (Dec 21-22)





