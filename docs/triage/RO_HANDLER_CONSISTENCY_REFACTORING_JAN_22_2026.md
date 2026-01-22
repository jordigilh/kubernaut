# RemediationOrchestrator Handler Consistency Refactoring

**Date**: 2026-01-22
**Status**: 🚧 **IN PROGRESS**
**Business Requirement**: BR-AUDIT-005 (Audit Event Emission)
**Root Cause**: 4 handlers bypass `transitionToFailed()`, missing lifecycle.failed audit events
**Architectural Goal**: Handler consistency across all child CRD services

---

## 🎯 **OBJECTIVES**

### **Primary Goal**: Fix Missing Audit Events
4 handlers directly update `rr.Status.OverallPhase = Failed` without calling reconciler's `transitionToFailed()`, causing missing `orchestrator.lifecycle.failed` audit events:

1. **AIAnalysisHandler** (2 methods):
   - `handleHumanReviewRequired()` - line ~182
   - `propagateFailure()` - lines 338, 392

2. **Skip Handlers** (2 handlers):
   - `ExhaustedRetriesHandler.Handle()` - line 79
   - `PreviousExecutionFailedHandler.Handle()` - line 80

### **Secondary Goal**: Architectural Consistency
Extract all child CRD status handling into dedicated handlers for maintainability:

| Service | Current State | Target State |
|---------|---------------|--------------|
| **SignalProcessing** | ❌ Inline (~5 lines) | ✅ SignalProcessingHandler |
| **AIAnalysis** | ✅ AIAnalysisHandler | ✅ Keep + add callback |
| **WorkflowExecution** | ❌ Inline (~50 lines) | ✅ WorkflowExecutionHandler |
| **NotificationRequest** | ✅ NotificationHandler | ✅ Keep (no changes) |

**Rationale**: One handler per service = easier to read, test, and maintain

---

## 📊 **CURRENT STATE ANALYSIS**

### **Handler Usage Patterns**

#### **Pattern 1: Delegated Handlers** (AIAnalysis, NotificationRequest)
```go
// Reconciler delegates complex logic to handlers
return r.aiAnalysisHandler.HandleAIAnalysisStatus(ctx, rr, ai)
return r.notificationHandler.HandleNotificationStatus(ctx, rr, notification)
```

**Characteristics**:
- ✅ Complex conditional logic (multiple failure paths)
- ✅ Testable in isolation
- ✅ Centralized business logic
- ❌ **BUG**: Some methods bypass `transitionToFailed()`

#### **Pattern 2: Inline Handling** (SignalProcessing, WorkflowExecution)
```go
// Reconciler handles status checks inline
switch agg.SignalProcessingPhase {
case "Completed": return r.transitionPhase(ctx, rr, phase.Analyzing)
}

switch agg.WorkflowExecutionPhase {
case "Completed": return r.transitionToCompleted(ctx, rr, "Remediated")
case "Failed": return r.transitionToFailed(ctx, rr, "workflow_execution", "WorkflowExecution failed")
}
```

**Characteristics**:
- ✅ Simple state-based logic
- ✅ **CORRECT**: Directly calls `transitionToFailed()` (no audit bug)
- ❌ Inconsistent with Pattern 1
- ❌ Harder to unit test (coupled to reconciler)

---

## 🔄 **REFACTORING SCOPE**

### **Phase 1: Extract Handlers (Consistency)**

#### **1.1 Create SignalProcessingHandler**
**File**: `pkg/remediationorchestrator/handler/signalprocessing.go` (NEW)

```go
type SignalProcessingHandler struct {
    client              client.Client
    scheme              *runtime.Scheme
    metrics             *metrics.Metrics
    transitionToPhase   func(context.Context, *remediationv1.RemediationRequest, remediationv1.RemediationPhase) (ctrl.Result, error)
}

func NewSignalProcessingHandler(
    c client.Client,
    s *runtime.Scheme,
    m *metrics.Metrics,
    ttp func(context.Context, *remediationv1.RemediationRequest, remediationv1.RemediationPhase) (ctrl.Result, error),
) *SignalProcessingHandler {
    return &SignalProcessingHandler{
        client:            c,
        scheme:            s,
        metrics:           m,
        transitionToPhase: ttp,
    }
}

func (h *SignalProcessingHandler) HandleStatus(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
    sp *signalprocessingv1.SignalProcessing,
) (ctrl.Result, error) {
    logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name, "signalProcessing", sp.Name)

    switch sp.Status.Phase {
    case signalprocessingv1.PhaseCompleted:
        logger.Info("SignalProcessing completed, transitioning to Analyzing")
        return h.transitionToPhase(ctx, rr, phase.Analyzing)
    case signalprocessingv1.PhaseFailed:
        logger.Info("SignalProcessing failed")
        // Future: Add failure handling logic
        return ctrl.Result{}, fmt.Errorf("SignalProcessing failed")
    default:
        // Still in progress
        return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
    }
}
```

**Changes to Reconciler**:
```go
// internal/controller/remediationorchestrator/reconciler.go
type Reconciler struct {
    // ... existing fields ...
    spHandler *handler.SignalProcessingHandler // NEW
}

func NewReconciler(...) *Reconciler {
    r := &Reconciler{
        // ... existing fields ...
    }

    // Initialize SP handler with callback
    r.spHandler = handler.NewSignalProcessingHandler(c, s, m, r.transitionPhase)

    return r
}

func (r *Reconciler) handleProcessingPhase(ctx, rr, agg) (ctrl.Result, error) {
    // OLD (inline):
    // switch agg.SignalProcessingPhase {
    // case "Completed": return r.transitionPhase(ctx, rr, phase.Analyzing)
    // }

    // NEW (delegated):
    sp := &signalprocessingv1.SignalProcessing{}
    if err := r.client.Get(ctx, client.ObjectKey{...}, sp); err != nil {
        return ctrl.Result{}, err
    }
    return r.spHandler.HandleStatus(ctx, rr, sp)
}
```

---

#### **1.2 Refactor WorkflowExecutionHandler**

**Delete Dead Code** (never called by reconciler):
- `HandleSkipped()` - stubbed, never implemented
- `HandleFailed()` - legacy, 3 direct `rr.Status.OverallPhase = Failed` assignments
- All helper methods only used by dead code

**Keep/Refactor for Real Usage**:
```go
// pkg/remediationorchestrator/handler/workflowexecution.go

type WorkflowExecutionHandler struct {
    client                client.Client
    scheme                *runtime.Scheme
    metrics               *metrics.Metrics
    transitionToFailed    func(context.Context, *remediationv1.RemediationRequest, string, string) (ctrl.Result, error)
    transitionToCompleted func(context.Context, *remediationv1.RemediationRequest, string) (ctrl.Result, error)
}

func NewWorkflowExecutionHandler(
    c client.Client,
    s *runtime.Scheme,
    m *metrics.Metrics,
    ttf func(context.Context, *remediationv1.RemediationRequest, string, string) (ctrl.Result, error),
    ttc func(context.Context, *remediationv1.RemediationRequest, string) (ctrl.Result, error),
) *WorkflowExecutionHandler {
    return &WorkflowExecutionHandler{
        client:                c,
        scheme:                s,
        metrics:               m,
        transitionToFailed:    ttf,
        transitionToCompleted: ttc,
    }
}

func (h *WorkflowExecutionHandler) HandleStatus(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
    we *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
    logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name, "workflowExecution", we.Name)

    switch we.Status.Phase {
    case workflowexecutionv1.PhaseCompleted:
        logger.Info("WorkflowExecution completed, transitioning to Completed")
        // Set WorkflowExecutionComplete condition (DD-PERF-001)
        // ... condition logic moved from reconciler ...
        return h.transitionToCompleted(ctx, rr, "Remediated")

    case workflowexecutionv1.PhaseFailed:
        logger.Info("WorkflowExecution failed, transitioning to Failed")
        // Set WorkflowExecutionComplete condition (DD-PERF-001)
        // ... condition logic moved from reconciler ...
        return h.transitionToFailed(ctx, rr, "workflow_execution", "WorkflowExecution failed")

    case "":
        // Empty phase - check if child is missing
        logger.Error(nil, "WorkflowExecution phase is empty")
        return h.transitionToFailed(ctx, rr, "workflow_execution", "WorkflowExecution not found")

    default:
        // Still in progress
        return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
    }
}
```

**Changes to Reconciler**:
```go
// internal/controller/remediationorchestrator/reconciler.go
type Reconciler struct {
    // ... existing fields ...
    weHandler *handler.WorkflowExecutionHandler // NEW (replaces inline logic)
}

func NewReconciler(...) *Reconciler {
    r := &Reconciler{
        // ... existing fields ...
    }

    // Initialize WE handler with callbacks
    r.weHandler = handler.NewWorkflowExecutionHandler(c, s, m, r.transitionToFailed, r.transitionToCompleted)

    return r
}

func (r *Reconciler) handleExecutingPhase(ctx, rr, agg) (ctrl.Result, error) {
    logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name, "wePhase", agg.WorkflowExecutionPhase)

    // Check for corrupted state (no WE ref)
    if rr.Status.WorkflowExecutionRef == nil {
        logger.Error(nil, "Executing phase but no WorkflowExecutionRef - corrupted state")
        return r.transitionToFailed(ctx, rr, "workflow_execution", "WorkflowExecution not found")
    }

    // Fetch WE CRD
    we := &workflowexecutionv1.WorkflowExecution{}
    if err := r.client.Get(ctx, client.ObjectKey{
        Name:      rr.Status.WorkflowExecutionRef.Name,
        Namespace: rr.Status.WorkflowExecutionRef.Namespace,
    }, we); err != nil {
        logger.Error(err, "Failed to fetch WorkflowExecution CRD")
        return r.transitionToFailed(ctx, rr, "workflow_execution", "WorkflowExecution not found")
    }

    // Delegate to handler
    return r.weHandler.HandleStatus(ctx, rr, we)
}
```

---

### **Phase 2: Fix Audit Emission Bug**

#### **2.1 Fix AIAnalysisHandler**

**Add callback field**:
```go
// pkg/remediationorchestrator/handler/aianalysis.go
type AIAnalysisHandler struct {
    client              client.Client
    scheme              *runtime.Scheme
    notificationCreator *creator.NotificationCreator
    Metrics             *metrics.Metrics
    transitionToFailed  func(context.Context, *remediationv1.RemediationRequest, string, string) (ctrl.Result, error) // NEW
}

func NewAIAnalysisHandler(
    c client.Client,
    s *runtime.Scheme,
    nc *creator.NotificationCreator,
    m *metrics.Metrics,
    ttf func(context.Context, *remediationv1.RemediationRequest, string, string) (ctrl.Result, error), // NEW
) *AIAnalysisHandler {
    return &AIAnalysisHandler{
        client:              c,
        scheme:              s,
        notificationCreator: nc,
        Metrics:             m,
        transitionToFailed:  ttf, // NEW
    }
}
```

**Fix Method 1: handleHumanReviewRequired()**
```go
// BEFORE (line ~182):
rr.Status.OverallPhase = remediationv1.PhaseFailed // ❌ Direct update - no audit

// AFTER:
return h.transitionToFailed(ctx, rr, "ai_analysis",
    fmt.Sprintf("Human review required: %s", ai.Status.HumanReviewReason)) // ✅ Audit emitted
```

**Fix Method 2: propagateFailure()**
```go
// BEFORE (lines 338, 392):
rr.Status.OverallPhase = remediationv1.PhaseFailed // ❌ Direct update - no audit

// AFTER:
return h.transitionToFailed(ctx, rr, "ai_analysis",
    fmt.Sprintf("AIAnalysis failed: %s: %s", ai.Status.Reason, ai.Status.Message)) // ✅ Audit emitted
```

---

#### **2.2 Fix Skip Handlers**

**Add callback to Context**:
```go
// pkg/remediationorchestrator/handler/skip/types.go
type Context struct {
    Client              client.Client
    Metrics             *metrics.Metrics
    NotificationCreator interface { ... }
    TransitionToFailedFunc func(context.Context, *remediationv1.RemediationRequest, string, string) (ctrl.Result, error) // NEW
}
```

**Fix ExhaustedRetriesHandler**:
```go
// pkg/remediationorchestrator/handler/skip/exhausted_retries.go

// BEFORE (line 79):
rr.Status.OverallPhase = remediationv1.PhaseFailed // ❌ Direct update - no audit

// AFTER:
return h.ctx.TransitionToFailedFunc(ctx, rr, "workflow_execution",
    "Exhausted retries - manual intervention required") // ✅ Audit emitted
```

**Fix PreviousExecutionFailedHandler**:
```go
// pkg/remediationorchestrator/handler/skip/previous_execution_failed.go

// BEFORE (line 80):
rr.Status.OverallPhase = remediationv1.PhaseFailed // ❌ Direct update - no audit

// AFTER:
return h.ctx.TransitionToFailedFunc(ctx, rr, "workflow_execution",
    "Previous execution failed - manual review required") // ✅ Audit emitted
```

**Update Reconciler to pass callback**:
```go
// internal/controller/remediationorchestrator/reconciler.go

func NewReconciler(...) *Reconciler {
    r := &Reconciler{ ... }

    // Create skip handler context with callback
    skipCtx := &skip.Context{
        Client:                 c,
        Metrics:                m,
        NotificationCreator:    nc,
        TransitionToFailedFunc: r.transitionToFailed, // NEW
    }

    // Initialize handlers with context
    r.weHandler = handler.NewWorkflowExecutionHandler(c, s, m, skipCtx, r.transitionToFailed, r.transitionToCompleted)

    return r
}
```

---

## 🧪 **TESTING STRATEGY**

### **Phase 1: Unit Tests (NEW/REFACTORED)**

#### **1.1 SignalProcessingHandler Unit Tests**
**File**: `test/unit/remediationorchestrator/signalprocessing_handler_test.go` (NEW)

```go
var _ = Describe("SignalProcessingHandler", func() {
    var (
        handler             *handler.SignalProcessingHandler
        mockTransitionPhase func(context.Context, *remediationv1.RemediationRequest, remediationv1.RemediationPhase) (ctrl.Result, error)
        transitionCalled    bool
        transitionToPhase   remediationv1.RemediationPhase
    )

    BeforeEach(func() {
        transitionCalled = false
        mockTransitionPhase = func(ctx context.Context, rr *remediationv1.RemediationRequest, phase remediationv1.RemediationPhase) (ctrl.Result, error) {
            transitionCalled = true
            transitionToPhase = phase
            return ctrl.Result{}, nil
        }
        handler = handler.NewSignalProcessingHandler(k8sClient, scheme, metrics, mockTransitionPhase)
    })

    Context("HandleStatus", func() {
        It("should transition to Analyzing when SP completes", func() {
            sp := &signalprocessingv1.SignalProcessing{
                Status: signalprocessingv1.SignalProcessingStatus{
                    Phase: signalprocessingv1.PhaseCompleted,
                },
            }
            result, err := handler.HandleStatus(ctx, rr, sp)
            Expect(err).ToNot(HaveOccurred())
            Expect(transitionCalled).To(BeTrue())
            Expect(transitionToPhase).To(Equal(phase.Analyzing))
        })
    })
})
```

#### **1.2 WorkflowExecutionHandler Unit Tests**
**File**: `test/unit/remediationorchestrator/workflowexecution_handler_test.go` (REFACTOR)

**DELETE**: All tests for dead methods (`HandleSkipped`, legacy `HandleFailed`)

**ADD**: Tests for new `HandleStatus()` method

```go
var _ = Describe("WorkflowExecutionHandler", func() {
    var (
        handler                  *handler.WorkflowExecutionHandler
        mockTransitionFailed     func(context.Context, *remediationv1.RemediationRequest, string, string) (ctrl.Result, error)
        mockTransitionCompleted  func(context.Context, *remediationv1.RemediationRequest, string) (ctrl.Result, error)
        failedCalled             bool
        completedCalled          bool
    )

    BeforeEach(func() {
        failedCalled = false
        completedCalled = false
        mockTransitionFailed = func(ctx context.Context, rr *remediationv1.RemediationRequest, phase, reason string) (ctrl.Result, error) {
            failedCalled = true
            return ctrl.Result{}, nil
        }
        mockTransitionCompleted = func(ctx context.Context, rr *remediationv1.RemediationRequest, outcome string) (ctrl.Result, error) {
            completedCalled = true
            return ctrl.Result{}, nil
        }
        handler = handler.NewWorkflowExecutionHandler(k8sClient, scheme, metrics, mockTransitionFailed, mockTransitionCompleted)
    })

    Context("HandleStatus", func() {
        It("should call transitionToCompleted when WE completes", func() {
            we := &workflowexecutionv1.WorkflowExecution{
                Status: workflowexecutionv1.WorkflowExecutionStatus{
                    Phase: workflowexecutionv1.PhaseCompleted,
                },
            }
            result, err := handler.HandleStatus(ctx, rr, we)
            Expect(err).ToNot(HaveOccurred())
            Expect(completedCalled).To(BeTrue())
            Expect(failedCalled).To(BeFalse())
        })

        It("should call transitionToFailed when WE fails", func() {
            we := &workflowexecutionv1.WorkflowExecution{
                Status: workflowexecutionv1.WorkflowExecutionStatus{
                    Phase: workflowexecutionv1.PhaseFailed,
                },
            }
            result, err := handler.HandleStatus(ctx, rr, we)
            Expect(err).ToNot(HaveOccurred())
            Expect(failedCalled).To(BeTrue())
            Expect(completedCalled).To(BeFalse())
        })
    })
})
```

#### **1.3 AIAnalysisHandler Unit Tests**
**File**: `test/unit/remediationorchestrator/aianalysis_handler_test.go` (UPDATE)

**ADD**: Mock `transitionToFailed` callback and verify it's called

```go
var _ = Describe("AIAnalysisHandler", func() {
    var (
        handler              *handler.AIAnalysisHandler
        mockTransitionFailed func(context.Context, *remediationv1.RemediationRequest, string, string) (ctrl.Result, error)
        failedCalled         bool
        failurePhase         string
        failureReason        string
    )

    BeforeEach(func() {
        failedCalled = false
        mockTransitionFailed = func(ctx context.Context, rr *remediationv1.RemediationRequest, phase, reason string) (ctrl.Result, error) {
            failedCalled = true
            failurePhase = phase
            failureReason = reason
            return ctrl.Result{}, nil
        }
        handler = handler.NewAIAnalysisHandler(k8sClient, scheme, notificationCreator, metrics, mockTransitionFailed)
    })

    Context("HandleAIAnalysisStatus - NeedsHumanReview", func() {
        It("should call transitionToFailed with correct parameters (UT-RO-197-001)", func() {
            ai := &aianalysisv1.AIAnalysis{
                Status: aianalysisv1.AIAnalysisStatus{
                    Phase:              aianalysisv1.PhaseCompleted,
                    NeedsHumanReview:   true,
                    HumanReviewReason:  "INSUFFICIENT_CONFIDENCE",
                },
            }
            result, err := handler.HandleAIAnalysisStatus(ctx, rr, ai)
            Expect(err).ToNot(HaveOccurred())
            Expect(failedCalled).To(BeTrue())
            Expect(failurePhase).To(Equal("ai_analysis"))
            Expect(failureReason).To(ContainSubstring("INSUFFICIENT_CONFIDENCE"))
        })
    })
})
```

#### **1.4 Skip Handler Unit Tests**
**File**: `test/unit/remediationorchestrator/skip_handlers_test.go` (UPDATE)

**ADD**: Mock `TransitionToFailedFunc` callback and verify it's called

---

### **Phase 2: Integration Tests**

**File**: `test/integration/remediationorchestrator/audit_phase_lifecycle_integration_test.go`

**Expected Result**: All 4 integration tests should now PASS
- `IT-AUDIT-PHASE-001` ✅ (already passing)
- `IT-AUDIT-PHASE-002` ✅ (already passing)
- `IT-AUDIT-COMPLETION-001` ✅ (already passing)
- `IT-AUDIT-COMPLETION-002` ✅ **WILL NOW PASS** (audit event emitted)

---

### **Phase 3: E2E Tests**

**No changes required** - E2E tests already programmatically update CRD statuses, bypassing handlers

---

## 📋 **IMPLEMENTATION CHECKLIST**

### **Phase 1: Handler Extraction**
- [ ] Create `pkg/remediationorchestrator/handler/signalprocessing.go`
- [ ] Refactor `pkg/remediationorchestrator/handler/workflowexecution.go`
  - [ ] Delete dead methods (`HandleSkipped`, legacy `HandleFailed`)
  - [ ] Add new `HandleStatus()` method
  - [ ] Add `transitionToFailed` and `transitionToCompleted` callbacks
- [ ] Update reconciler to use `spHandler` and `weHandler`

### **Phase 2: Audit Fix**
- [ ] Update `AIAnalysisHandler` constructor to accept `transitionToFailed` callback
- [ ] Fix `handleHumanReviewRequired()` to call `transitionToFailed`
- [ ] Fix `propagateFailure()` to call `transitionToFailed`
- [ ] Update `skip.Context` to include `TransitionToFailedFunc`
- [ ] Fix `ExhaustedRetriesHandler.Handle()` to call `TransitionToFailedFunc`
- [ ] Fix `PreviousExecutionFailedHandler.Handle()` to call `TransitionToFailedFunc`
- [ ] Update reconciler's `NewReconciler()` to pass callbacks

### **Phase 3: Testing**
- [ ] Create `test/unit/remediationorchestrator/signalprocessing_handler_test.go`
- [ ] Refactor `test/unit/remediationorchestrator/workflowexecution_handler_test.go`
- [ ] Update `test/unit/remediationorchestrator/aianalysis_handler_test.go`
- [ ] Update `test/unit/remediationorchestrator/skip_handlers_test.go`
- [ ] Run unit tests: `make test-unit-remediationorchestrator`
- [ ] Run integration tests: `make test-integration-remediationorchestrator`
- [ ] Run E2E tests: `make test-e2e-remediationorchestrator`

### **Phase 4: Validation**
- [ ] Verify `IT-AUDIT-COMPLETION-002` passes
- [ ] Check no lint errors: `golangci-lint run`
- [ ] Check no compilation errors: `go build ./...`
- [ ] Verify no regressions in other tests

---

## 🎯 **SUCCESS CRITERIA**

✅ **Audit Bug Fixed**:
- `IT-AUDIT-COMPLETION-002` passes
- All 4 handlers emit `orchestrator.lifecycle.failed` audit events

✅ **Handler Consistency Achieved**:
- Every service has a dedicated handler (SP, AA, WE, NT)
- All handlers follow same pattern (callbacks for transitions)

✅ **Dead Code Removed**:
- Legacy `WorkflowExecutionHandler` methods deleted
- No unused imports or functions

✅ **Tests Passing**:
- All unit tests pass
- All integration tests pass (including 4 audit tests)
- All E2E tests pass (no regressions)

✅ **Code Quality**:
- No lint errors
- No compilation errors
- Handlers are testable in isolation

---

## ⏱️ **ESTIMATED TIMELINE**

- **Phase 1**: Handler Extraction (~30-45 min)
- **Phase 2**: Audit Fix (~15-20 min)
- **Phase 3**: Testing (~30-45 min)
- **Phase 4**: Validation (~10-15 min)

**Total**: ~1.5-2 hours

---

## 🔗 **RELATED DOCUMENTATION**

- **Business Requirement**: BR-AUDIT-005 (Audit Event Emission)
- **Root Cause**: `IT-AUDIT-COMPLETION-002` failure analysis
- **Testing Guidelines**: `docs/development/business-requirements/TESTING_GUIDELINES.md`
- **Handler Patterns**: `pkg/remediationorchestrator/handler/`

---

**Status**: 🚧 Ready to implement - awaiting user approval
