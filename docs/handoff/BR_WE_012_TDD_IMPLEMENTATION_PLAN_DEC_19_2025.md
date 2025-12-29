# BR-WE-012 TDD Implementation Plan: RO Exponential Backoff Routing

**Date**: December 19, 2025
**Status**: 🔴 **GAP CONFIRMED - READY FOR IMPLEMENTATION**
**Phase**: DD-RO-002 Phase 2 - RO Routing Logic
**Business Requirement**: BR-WE-012 (Exponential Backoff Cooldown)
**Confidence**: 95% that implementation approach is correct

---

## 📋 **Executive Summary**

**Gap Confirmed**: ✅ **VERIFIED** through code inspection and authoritative documentation

**What's Working**:
- ✅ WE tracks `ConsecutiveFailures`, `NextAllowedExecution`, `WasExecutionFailure`
- ✅ WE calculates exponential backoff using `pkg/shared/backoff`
- ✅ WE resets counter on success
- ✅ WE unit tests passing (215/216)

**What's Missing**:
- ❌ RO does NOT check `NextAllowedExecution` before creating WFE
- ❌ RO does NOT enforce `MaxConsecutiveFailures` limit
- ❌ RO does NOT query previous WFEs for backoff state
- ❌ RO routing integration tests missing

**Evidence**:
```bash
$ grep -r "NextAllowedExecution\|ExponentialBackoff" pkg/remediationorchestrator/creator/
# No matches found ← CONFIRMS GAP

$ grep -r "ConsecutiveFailures\|NextAllowedExecution" internal/controller/workflowexecution/
# 27 matches found ← WE IMPLEMENTATION EXISTS
```

**Authoritative Source**: [DD-RO-002](../architecture/decisions/DD-RO-002-centralized-routing-responsibility.md)
- Line 330: "Phase 2: RO Routing Logic (Days 2-5) - ⏳ NOT STARTED"
- Line 114-117: Expected RO code for Check 3 (Exponential Backoff)
- Line 257: "BR-WE-012 (Exponential Backoff) | RO routing logic (Check 3)"

---

## 🎯 **TDD Implementation Strategy**

### **APDC-Enhanced TDD Workflow**

**Phase**: DO-RED → DO-GREEN → DO-REFACTOR (with APDC Analysis + Plan phases)

### **Analysis Phase** (5-10 minutes)

**Business Context**:
- **BR-WE-012**: Exponential Backoff Cooldown prevents rapid retry loops
- **Gap**: RO creates WFE without checking if backoff window is active
- **Impact**: Unnecessary WFE creation when previous execution failed

**Technical Context**:
- WE already tracks state in `WorkflowExecutionStatus.NextAllowedExecution`
- RO has field index on `spec.targetResource` (Phase 1 complete)
- Query pattern validated: 2-20ms with 1000 WFEs (DD-RO-002 line 307-308)

**Integration Context**:
- RO reconciles `Analyzing` phase (after AIAnalysis complete)
- RO calls `workflowexecution.Creator.Create()` to create WFE
- Need to inject routing check BEFORE Create() call

**Complexity Assessment**: MEDIUM
- Need to query previous WFEs for same target
- Need to check `NextAllowedExecution` timestamp
- Need to check `ConsecutiveFailures >= MaxConsecutiveFailures`
- Need to set RR skip status if blocked

### **Plan Phase** (10-15 minutes)

**TDD Strategy**:
1. **RED** (10-15 min): Write integration test for backoff routing check
2. **GREEN** (15-20 min): Implement routing check in RO Creator
3. **REFACTOR** (20-30 min): Extract helper methods, add unit tests

**Integration Plan**:
- Modify: `pkg/remediationorchestrator/creator/workflowexecution.go`
- Add: Query method to find previous WFE with backoff state
- Integration: Called from `reconciler.go` before WFE creation

**Success Definition**:
- Integration test: RO skips WFE creation when backoff active
- Unit test: Routing check logic validates backoff calculation
- RR Status: `SkipReason="ExponentialBackoff"`, `BlockedUntil` set

**Risk Mitigation**:
- Use existing field index (already validated for performance)
- Follow DD-RO-002 specification exactly (lines 114-117)
- Reuse WE's backoff calculation (don't duplicate logic)

---

## 🧪 **TDD Phase: DO-RED (Write Failing Tests)**

### **Step 1: Integration Test - Backoff Routing Check**

**File**: `test/integration/remediationorchestrator/exponential_backoff_routing_test.go` (NEW)

**Test Scenario**: "RO should skip WFE creation when backoff window is active"

**Test Pattern** (Phase 1 style):
```go
var _ = Describe("BR-WE-012: Exponential Backoff Routing", func() {
    It("should skip WFE creation when NextAllowedExecution blocks execution", func() {
        ns := createTestNamespace("backoff-routing")
        defer deleteTestNamespace(ns)

        // ========================================
        // SETUP: Create RR with completed AI analysis
        // ========================================
        rr := createRemediationRequest(ns, "rr-backoff-test")

        // Phase 1: Manually create SP and AI (no child controllers)
        sp := createSignalProcessingCRD(ns, rr)
        Expect(k8sClient.Create(ctx, sp)).To(Succeed())
        updateSPStatus(ns, sp.Name, signalprocessingv1.PhaseCompleted)

        ai := createAIAnalysisCRD(ns, rr)
        Expect(k8sClient.Create(ctx, ai)).To(Succeed())

        // Simulate AI completion with workflow selection
        ai.Status.Phase = "Completed"
        ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
            WorkflowID:     "restart-pod-v1",
            Version:        "v1",
            ContainerImage: "test/restart-pod:v1",
            Confidence:     0.9,
        }
        Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

        // ========================================
        // SETUP: Create previous WFE with active backoff
        // ========================================
        prevWFE := createWorkflowExecutionCRD(ns, rr, "restart-pod-v1")
        Expect(k8sClient.Create(ctx, prevWFE)).To(Succeed())

        // Simulate pre-execution failure with active backoff
        now := metav1.Now()
        nextAllowed := metav1.NewTime(now.Add(5 * time.Minute)) // 5 min backoff

        prevWFE.Status.Phase = "Failed"
        prevWFE.Status.ConsecutiveFailures = 1
        prevWFE.Status.NextAllowedExecution = &nextAllowed
        prevWFE.Status.FailureDetails = &workflowexecutionv1.FailureDetails{
            WasExecutionFailure: false, // Pre-execution failure
            FailureReason:       "ImagePullBackOff",
        }
        Expect(k8sClient.Status().Update(ctx, prevWFE)).To(Succeed())

        // ========================================
        // TEST: RO should NOT create new WFE (backoff active)
        // ========================================

        // Wait for RO to process (should skip WFE creation)
        Eventually(func() string {
            rr := &remediationv1.RemediationRequest{}
            err := k8sClient.Get(ctx, types.NamespacedName{
                Name: rr.Name, Namespace: ns,
            }, rr)
            if err != nil {
                return ""
            }
            return rr.Status.SkipReason
        }, timeout, interval).Should(Equal("ExponentialBackoff"),
            "RO should skip WFE creation with ExponentialBackoff reason")

        // Verify RR status has backoff details
        rr = &remediationv1.RemediationRequest{}
        Expect(k8sClient.Get(ctx, types.NamespacedName{
            Name: rr.Name, Namespace: ns,
        }, rr)).To(Succeed())

        Expect(rr.Status.SkipReason).To(Equal("ExponentialBackoff"))
        Expect(rr.Status.BlockedUntil).ToNot(BeNil())
        Expect(rr.Status.BlockedUntil.Time).To(BeTemporally("~", nextAllowed.Time, 5*time.Second))
        Expect(rr.Status.OverallPhase).To(Equal(remediationv1.PhaseSkipped))
        Expect(rr.Status.SkipMessage).To(ContainSubstring("Backoff active"))

        // Verify NO new WFE was created
        wfeList := &workflowexecutionv1.WorkflowExecutionList{}
        Expect(k8sClient.List(ctx, wfeList, client.InNamespace(ns))).To(Succeed())
        Expect(wfeList.Items).To(HaveLen(1), "Should only have original WFE, no new one created")
    })

    It("should create WFE when backoff window has expired", func() {
        ns := createTestNamespace("backoff-expired")
        defer deleteTestNamespace(ns)

        // Similar setup, but NextAllowedExecution is in the past
        // Verify RO DOES create new WFE
    })

    It("should skip WFE creation when MaxConsecutiveFailures reached", func() {
        ns := createTestNamespace("max-failures")
        defer deleteTestNamespace(ns)

        // Create previous WFE with ConsecutiveFailures >= 3
        // Verify RO skips with SkipReason="ExhaustedRetries"
    })
})
```

**Expected Result**: ❌ TEST FAILS (RO doesn't check backoff, creates WFE anyway)

### **Step 2: Unit Test - Backoff Calculation Logic**

**File**: `test/unit/remediationorchestrator/routing/exponential_backoff_test.go` (NEW)

**Test Scenarios**:
1. "calculateExponentialBackoff returns nil when no previous WFE"
2. "calculateExponentialBackoff returns nil when NextAllowedExecution expired"
3. "calculateExponentialBackoff returns BlockedUntil when backoff active"
4. "hasExhaustedRetries returns true when ConsecutiveFailures >= 3"
5. "hasExhaustedRetries returns false when ConsecutiveFailures < 3"

**Expected Result**: ❌ TESTS FAIL (functions don't exist yet)

---

## 🟢 **TDD Phase: DO-GREEN (Minimal Implementation)**

### **Step 1: Add Routing Check to WFE Creator**

**File**: `pkg/remediationorchestrator/creator/workflowexecution.go`

**Changes**:
```go
// Create creates a new WorkflowExecution CRD for a RemediationRequest.
// BR-WE-012: Check exponential backoff before creating WFE (DD-RO-002 Check 3)
func (c *WorkflowExecutionCreator) Create(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
    ai *aianalysisv1.AIAnalysis,
) (string, error) {
    logger := log.FromContext(ctx)

    // ========================================
    // DD-RO-002 CHECK 3: Exponential Backoff
    // ========================================
    // Query previous WFE for same target to check backoff state
    targetResource := BuildTargetResourceString(rr)
    prevWFE, err := c.findPreviousWFE(ctx, targetResource)
    if err != nil {
        logger.Error(err, "Failed to query previous WFE for backoff check")
        // Continue - backoff check failure shouldn't block new execution
    }

    if prevWFE != nil {
        // Check 3a: Exponential backoff window active
        if prevWFE.Status.NextAllowedExecution != nil {
            if time.Now().Before(prevWFE.Status.NextAllowedExecution.Time) {
                // Backoff window is active - do NOT create WFE
                return "", &BackoffActiveError{
                    BlockedUntil: prevWFE.Status.NextAllowedExecution,
                    PreviousWFE:  prevWFE.Name,
                    Message:      fmt.Sprintf("Backoff active. Next allowed: %s", prevWFE.Status.NextAllowedExecution.Format(time.RFC3339)),
                }
            }
        }

        // Check 3b: Max consecutive failures reached
        if prevWFE.Status.ConsecutiveFailures >= c.maxConsecutiveFailures {
            return "", &ExhaustedRetriesError{
                Failures:    prevWFE.Status.ConsecutiveFailures,
                PreviousWFE: prevWFE.Name,
                Message:     fmt.Sprintf("Maximum retry attempts reached (%d consecutive failures)", c.maxConsecutiveFailures),
            }
        }
    }

    // Backoff window passed or no previous failure → Create WFE
    return c.createWorkflowExecution(ctx, rr, ai)
}

// findPreviousWFE queries the most recent WFE for the same target resource.
// Uses field index on spec.targetResource for O(1) lookup performance.
func (c *WorkflowExecutionCreator) findPreviousWFE(
    ctx context.Context,
    targetResource string,
) (*workflowexecutionv1.WorkflowExecution, error) {
    wfeList := &workflowexecutionv1.WorkflowExecutionList{}

    // Query using field index (validated: 2-20ms with 1000 WFEs per DD-RO-002)
    err := c.client.List(ctx, wfeList, client.MatchingFields{
        "spec.targetResource": targetResource,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to query WFEs for target %s: %w", targetResource, err)
    }

    if len(wfeList.Items) == 0 {
        return nil, nil // No previous WFE
    }

    // Find most recent terminal WFE (Completed or Failed)
    var mostRecent *workflowexecutionv1.WorkflowExecution
    var mostRecentTime time.Time

    for i := range wfeList.Items {
        wfe := &wfeList.Items[i]

        // Only consider terminal phases (Completed or Failed)
        if wfe.Status.Phase != "Completed" && wfe.Status.Phase != "Failed" {
            continue
        }

        if wfe.Status.CompletionTime != nil {
            if mostRecent == nil || wfe.Status.CompletionTime.After(mostRecentTime) {
                mostRecent = wfe
                mostRecentTime = wfe.Status.CompletionTime.Time
            }
        }
    }

    return mostRecent, nil
}

// BackoffActiveError indicates WFE creation was blocked by active backoff window.
type BackoffActiveError struct {
    BlockedUntil *metav1.Time
    PreviousWFE  string
    Message      string
}

func (e *BackoffActiveError) Error() string {
    return e.Message
}

// ExhaustedRetriesError indicates WFE creation was blocked by max consecutive failures.
type ExhaustedRetriesError struct {
    Failures    int32
    PreviousWFE string
    Message     string
}

func (e *ExhaustedRetriesError) Error() string {
    return e.Message
}
```

### **Step 2: Handle Backoff Errors in RO Reconciler**

**File**: `pkg/remediationorchestrator/controller/reconciler.go`

**Changes in `reconcileAnalyzing()` method**:
```go
// Create WorkflowExecution CRD (BR-ORCH-025, BR-ORCH-031)
// DD-RO-002 Check 3: Exponential backoff routing check
weName, err := r.weCreator.Create(ctx, rr, ai)
if err != nil {
    // Check if error is backoff-related (DD-RO-002 routing decision)
    var backoffErr *creator.BackoffActiveError
    var exhaustedErr *creator.ExhaustedRetriesError

    if errors.As(err, &backoffErr) {
        // Temporary skip: backoff window active
        logger.Info("Skipping WFE creation - backoff window active",
            "blockedUntil", backoffErr.BlockedUntil,
            "previousWFE", backoffErr.PreviousWFE)

        return r.markTemporarySkip(ctx, rr,
            "ExponentialBackoff",
            backoffErr.PreviousWFE,
            backoffErr.BlockedUntil,
            backoffErr.Message)
    }

    if errors.As(err, &exhaustedErr) {
        // Permanent skip: max retries exhausted
        logger.Info("Skipping WFE creation - max retries exhausted",
            "consecutiveFailures", exhaustedErr.Failures,
            "previousWFE", exhaustedErr.PreviousWFE)

        return r.markPermanentSkip(ctx, rr,
            "ExhaustedRetries",
            exhaustedErr.PreviousWFE,
            exhaustedErr.Message)
    }

    // Other error - log and requeue
    logger.Error(err, "Failed to create WorkflowExecution CRD")
    return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil
}
```

### **Step 3: Add Skip Helper Methods**

**File**: `pkg/remediationorchestrator/controller/reconciler.go`

**New Methods**:
```go
// markTemporarySkip sets RR to Skipped phase with temporary skip details.
// Used for: ExponentialBackoff, RecentlyRemediated, ResourceBusy
func (r *Reconciler) markTemporarySkip(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
    reason, blockingResource string,
    blockedUntil *metav1.Time,
    message string,
) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
        rr.Status.OverallPhase = remediationv1.PhaseSkipped
        rr.Status.SkipReason = reason
        rr.Status.SkipMessage = message
        rr.Status.BlockedUntil = blockedUntil

        if reason == "ExponentialBackoff" {
            rr.Status.BlockingWorkflowExecution = blockingResource
        } else if reason == "RecentlyRemediated" {
            rr.Status.DuplicateOf = blockingResource
        } else if reason == "ResourceBusy" {
            rr.Status.BlockingWorkflowExecution = blockingResource
        }

        return nil
    })

    if err != nil {
        logger.Error(err, "Failed to mark RR as temporarily skipped", "reason", reason)
        return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil
    }

    // Requeue when blockedUntil expires (for time-based blocks)
    if blockedUntil != nil {
        requeueAfter := time.Until(blockedUntil.Time)
        if requeueAfter > 0 {
            logger.Info("RR temporarily skipped, requeuing when block expires",
                "reason", reason,
                "blockedUntil", blockedUntil.Format(time.RFC3339),
                "requeueAfter", requeueAfter)
            return ctrl.Result{RequeueAfter: requeueAfter}, nil
        }
    }

    return ctrl.Result{}, nil
}

// markPermanentSkip sets RR to Failed phase with permanent skip details.
// Used for: PreviousExecutionFailed, ExhaustedRetries
func (r *Reconciler) markPermanentSkip(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
    reason, blockingResource, message string,
) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
        rr.Status.OverallPhase = remediationv1.PhaseFailed
        rr.Status.SkipReason = reason
        rr.Status.SkipMessage = message

        if reason == "PreviousExecutionFailed" || reason == "ExhaustedRetries" {
            rr.Status.BlockingWorkflowExecution = blockingResource
        }

        return nil
    })

    if err != nil {
        logger.Error(err, "Failed to mark RR as permanently skipped", "reason", reason)
        return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil
    }

    logger.Info("RR permanently skipped - manual intervention required", "reason", reason)
    return ctrl.Result{}, nil
}
```

### **Step 4: Add Configuration**

**File**: `pkg/remediationorchestrator/creator/workflowexecution.go`

**Add field to WorkflowExecutionCreator**:
```go
type WorkflowExecutionCreator struct {
    client                  client.Client
    maxConsecutiveFailures  int32  // NEW: BR-WE-012 max retry limit
}

// NewWorkflowExecutionCreator creates a new WorkflowExecution creator.
func NewWorkflowExecutionCreator(client client.Client, maxConsecutiveFailures int32) *WorkflowExecutionCreator {
    if maxConsecutiveFailures <= 0 {
        maxConsecutiveFailures = 3 // Default per BR-WE-012
    }
    return &WorkflowExecutionCreator{
        client:                 client,
        maxConsecutiveFailures: maxConsecutiveFailures,
    }
}
```

**File**: `cmd/remediationorchestrator/main.go`

**Add flag**:
```go
maxConsecutiveFailures := flag.Int("max-consecutive-failures", 3,
    "Maximum consecutive failures before permanent skip (BR-WE-012)")

// ...

weCreator := creator.NewWorkflowExecutionCreator(mgr.GetClient(), int32(*maxConsecutiveFailures))
```

**Expected Result**: ✅ TESTS PASS (minimal implementation)

---

## 🔄 **TDD Phase: DO-REFACTOR (Enhance & Optimize)**

### **Step 1: Extract Routing Package**

**File**: `pkg/remediationorchestrator/routing/backoff.go` (NEW)

**Extract backoff logic into reusable routing package**:
```go
package routing

// BackoffChecker checks if exponential backoff window is active.
type BackoffChecker struct {
    client                 client.Client
    maxConsecutiveFailures int32
}

// CheckBackoff returns backoff details if execution should be blocked.
func (c *BackoffChecker) CheckBackoff(
    ctx context.Context,
    targetResource string,
) (*BackoffBlock, error) {
    prevWFE, err := c.findPreviousWFE(ctx, targetResource)
    if err != nil || prevWFE == nil {
        return nil, err
    }

    // Check backoff window
    if prevWFE.Status.NextAllowedExecution != nil {
        if time.Now().Before(prevWFE.Status.NextAllowedExecution.Time) {
            return &BackoffBlock{
                Type:         "ExponentialBackoff",
                BlockedUntil: prevWFE.Status.NextAllowedExecution,
                PreviousWFE:  prevWFE.Name,
                Message:      fmt.Sprintf("Backoff active. Next allowed: %s", prevWFE.Status.NextAllowedExecution.Format(time.RFC3339)),
            }, nil
        }
    }

    // Check max consecutive failures
    if prevWFE.Status.ConsecutiveFailures >= c.maxConsecutiveFailures {
        return &BackoffBlock{
            Type:        "ExhaustedRetries",
            PreviousWFE: prevWFE.Name,
            Message:     fmt.Sprintf("Maximum retry attempts reached (%d consecutive failures)", c.maxConsecutiveFailures),
        }, nil
    }

    return nil, nil // No block
}
```

### **Step 2: Add Comprehensive Unit Tests**

**File**: `test/unit/remediationorchestrator/routing/backoff_test.go` (NEW)

**Test Coverage**:
1. CheckBackoff with no previous WFE
2. CheckBackoff with expired NextAllowedExecution
3. CheckBackoff with active backoff window
4. CheckBackoff with max consecutive failures
5. findPreviousWFE with multiple WFEs (returns most recent)
6. findPreviousWFE with only Running WFEs (returns nil)

### **Step 3: Add Edge Case Tests**

**Integration Tests**:
1. Multiple RRs with same target (sequential backoff)
2. Backoff expires during reconciliation
3. WFE created while backoff query in progress
4. Previous WFE status update race condition

---

## 📊 **Success Criteria**

### **Code Quality**
- [ ] Integration tests pass (3 scenarios minimum)
- [ ] Unit tests pass (6 scenarios minimum)
- [ ] Code coverage >70% for routing package
- [ ] golangci-lint passes with no errors

### **Functional Requirements**
- [ ] RO skips WFE creation when `NextAllowedExecution` blocks
- [ ] RO skips WFE creation when `ConsecutiveFailures >= MaxConsecutiveFailures`
- [ ] RR status correctly populated with skip details
- [ ] RR requeued when backoff window expires

### **Performance Requirements**
- [ ] Field index query < 50ms (per DD-RO-002)
- [ ] No N+1 query problems
- [ ] Minimal memory allocation for queries

### **Documentation**
- [ ] Code comments reference BR-WE-012 and DD-RO-002
- [ ] Integration test comments explain Phase 1 pattern
- [ ] Routing decision logic clearly documented

---

## ⏱️ **Time Estimates**

| Phase | Task | Estimated Time | Confidence |
|-------|------|----------------|------------|
| **RED** | Integration test (backoff routing) | 30-45 min | 90% |
| **RED** | Unit tests (backoff logic) | 20-30 min | 85% |
| **GREEN** | WFE Creator routing check | 45-60 min | 85% |
| **GREEN** | RO reconciler error handling | 30-45 min | 90% |
| **GREEN** | Skip helper methods | 20-30 min | 95% |
| **GREEN** | Configuration flags | 15-20 min | 95% |
| **REFACTOR** | Extract routing package | 30-45 min | 80% |
| **REFACTOR** | Comprehensive unit tests | 45-60 min | 85% |
| **REFACTOR** | Edge case integration tests | 30-45 min | 80% |
| **TOTAL** | | **4-6 hours** | **87%** |

**Confidence Factors**:
- ✅ Clear specification (DD-RO-002)
- ✅ WE implementation reference available
- ✅ Field index already setup (Phase 1 complete)
- ⚠️ Integration test complexity (Phase 1 pattern)
- ⚠️ Edge case handling (race conditions)

---

## 🔗 **Related Documentation**

### **Authoritative Sources**
- [DD-RO-002](../architecture/decisions/DD-RO-002-centralized-routing-responsibility.md) - Routing responsibility design
- [BR-WE-012 Responsibility Assessment](BR_WE_012_RESPONSIBILITY_CONFIDENCE_ASSESSMENT_DEC_19_2025.md) - Gap analysis

### **Reference Implementation**
- `internal/controller/workflowexecution/workflowexecution_controller.go` (lines 903-925) - WE backoff calculation
- `internal/controller/workflowexecution/failure_analysis.go` - WE failure categorization
- `pkg/shared/backoff/backoff.go` - Shared backoff utility

### **Test Patterns**
- `test/integration/remediationorchestrator/routing_integration_test.go` - Existing routing tests (Phase 1 pattern)
- `test/integration/remediationorchestrator/suite_test.go` - Helper functions (`createWorkflowExecutionCRD`)

---

## 🎯 **Next Steps**

**Immediate (Start TDD Implementation)**:
1. Create integration test file: `exponential_backoff_routing_test.go`
2. Write failing test: "should skip WFE creation when backoff active"
3. Run test → verify FAILURE
4. Implement routing check in WFE Creator
5. Run test → verify SUCCESS

**Post-Implementation**:
1. Update DD-RO-002 Phase 2 status (Check 3 complete)
2. Document routing decision in RO README
3. Add BR-WE-012 reference to test coverage matrix

---

**Status**: 🔴 **READY FOR TDD IMPLEMENTATION**
**Confidence**: 95% that approach will work
**Estimated Effort**: 4-6 hours
**Priority**: HIGH (DD-RO-002 Phase 2 blocker)

