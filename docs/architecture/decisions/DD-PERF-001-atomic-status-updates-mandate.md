# DD-PERF-001: Atomic Status Updates - Mandatory Standard

**Status**: ✅ **APPROVED**
**Date**: December 26, 2025
**Category**: Performance & Architecture
**Priority**: P1 - System-Wide Performance Optimization
**Scope**: All CRD controllers (Notification, AIAnalysis, WorkflowExecution, SignalProcessing, RemediationOrchestrator)

---

## 📋 **Decision**

**ALL Kubernetes controllers that update CRD status fields MUST use atomic status updates** to consolidate multiple field changes into a single API call.

### **Mandate Scope**

This applies to controllers that perform:
- Phase transitions (e.g., `Pending` → `Analyzing` → `Complete`)
- Counter updates (e.g., `totalAttempts`, `successfulSteps`, `failedAnalyses`)
- Array appends (e.g., `deliveryAttempts[]`, `analysisSteps[]`, `executionHistory[]`)
- Timestamp updates (e.g., `completionTime`, `lastRetryTime`)

**Implementation Standard**:
- ✅ **Status Manager Pattern**: Extract status update logic to `pkg/[service]/status/manager.go`
- ✅ **AtomicStatusUpdate Method**: Single method that batches all status changes
- ✅ **Optimistic Locking**: Use `RetryOnConflict` with refetch-before-update pattern
- ✅ **Phase Validation**: Validate state transitions before updating

---

## 🎯 **Context**

### **Problem: Inefficient Status Updates**

Current pattern in most controllers:
```go
// ❌ BAD: Multiple sequential API calls
for _, step := range steps {
    workflow.Status.ExecutedSteps = append(...)
    workflow.Status.StepCount++
    client.Status().Update(ctx, workflow) // API call #1, #2, #3...
}

workflow.Status.Phase = "Complete"
client.Status().Update(ctx, workflow) // API call #N+1

// Result: N+1 API calls to K8s API server
```

**Consequences**:
- 🔴 **High API Server Load**: 4-10x more API calls than necessary
- 🔴 **Race Conditions**: Multiple updates create conflict windows
- 🔴 **Inconsistent State**: Brief periods where phase ≠ actual state
- 🔴 **Performance**: Network latency multiplied by N+1
- 🔴 **etcd Load**: Each update triggers etcd write + watch notifications

### **Real-World Impact (Notification Service)**

**Before Atomic Updates**:
- 3 channels × 3 retries = 9 attempts + 1 phase = **10 API calls**
- 100 notifications/min = **400-1000 API calls/min**

**After Atomic Updates**:
- All attempts + phase in 1 call = **1 API call**
- 100 notifications/min = **100 API calls/min**
- **90% API call reduction** ✅

---

## 🏗️ **Solution: Atomic Status Update Pattern**

### **Implementation Components**

#### **1. Status Manager (pkg/[service]/status/manager.go)**

```go
package status

import (
    "context"
    k8sretry "k8s.io/client-go/util/retry"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

type Manager struct {
    client client.Client
}

func NewManager(client client.Client) *Manager {
    return &Manager{client: client}
}

// AtomicStatusUpdate: Batch all status changes into single API call
func (m *Manager) AtomicStatusUpdate(
    ctx context.Context,
    resource client.Object, // Generic: works for any CRD
    updateFunc func() error, // Callback: modify status fields
) error {
    return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
        // 1. Refetch latest resourceVersion (optimistic locking)
        if err := m.client.Get(ctx, client.ObjectKeyFromObject(resource), resource); err != nil {
            return err
        }

        // 2. Apply all status field changes in memory
        if err := updateFunc(); err != nil {
            return err
        }

        // 3. SINGLE ATOMIC UPDATE: Commit all changes together
        return m.client.Status().Update(ctx, resource)
    })
}
```

#### **2. Controller Integration**

```go
// ✅ GOOD: Atomic update pattern
result := r.processWorkflow(ctx, workflow)

// Batch all changes into single update
err := r.StatusManager.AtomicStatusUpdate(ctx, workflow, func() error {
    // Update phase
    workflow.Status.Phase = "Complete"
    workflow.Status.Reason = "AllStepsSucceeded"

    // Append all steps atomically
    for _, step := range result.steps {
        workflow.Status.ExecutedSteps = append(workflow.Status.ExecutedSteps, step)
        workflow.Status.StepCount++
    }

    // Update counters
    workflow.Status.SuccessfulSteps = result.successCount
    workflow.Status.FailedSteps = result.failureCount

    // Set completion time
    now := metav1.Now()
    workflow.Status.CompletionTime = &now

    return nil
})

// Result: 1 API call regardless of number of steps
```

---

## 📊 **Expected Benefits**

### **Performance Improvements**

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| API Calls per Reconcile | N+1 | 1 | 75-90% reduction |
| Network Roundtrips | N+1 | 1 | 75-90% reduction |
| etcd Writes | N+1 | 1 | 75-90% reduction |
| Watch Events Triggered | N+1 | 1 | 75-90% reduction |
| Conflict Window Duration | N+1 × latency | 1 × latency | 75-90% reduction |

### **System-Wide Impact (Estimated)**

**Current Load**:
- 5 services × 100 resources/min × 5 avg updates = **2,500 API calls/min**

**After Atomic Updates**:
- 5 services × 100 resources/min × 1 atomic update = **500 API calls/min**

**Total Reduction**: **80% fewer K8s API calls** 🎯

---

## 🔧 **Implementation Requirements**

### **Mandatory Components (Per Service)**

1. **Status Manager** (`pkg/[service]/status/manager.go`)
   - ✅ `AtomicStatusUpdate()` method
   - ✅ Optimistic locking with `RetryOnConflict`
   - ✅ Phase validation (if applicable)
   - ✅ Unit tests for atomic operations

2. **Controller Refactoring**
   - ✅ Extract inline `Status().Update()` calls
   - ✅ Batch status changes during reconciliation
   - ✅ Single atomic update at phase transition
   - ✅ Remove sequential update loops

3. **Testing**
   - ✅ E2E tests pass unchanged (observable behavior identical)
   - ✅ Unit tests for status manager
   - ✅ Verify conflict handling with concurrent updates

4. **Documentation**
   - ✅ Update controller documentation with atomic pattern
   - ✅ Add performance metrics to observability dashboard
   - ✅ Document migration from old pattern

---

## 🎓 **Design Patterns**

### **Pattern 1: Phase Transition with Arrays**

**Use Case**: Notification delivery, workflow execution, analysis steps

```go
// Collect operations during reconciliation
type reconcileResult struct {
    newPhase    string
    items       []Item
    counters    Counters
}

// Single atomic update at end
r.StatusManager.AtomicStatusUpdate(ctx, resource, func() error {
    resource.Status.Phase = result.newPhase
    resource.Status.Items = append(resource.Status.Items, result.items...)
    resource.Status.SuccessCount = result.counters.success
    resource.Status.FailureCount = result.counters.failure
    return nil
})
```

### **Pattern 2: Conditional Phase Transition**

**Use Case**: Failed with retry vs permanent failure

```go
r.StatusManager.AtomicStatusUpdate(ctx, resource, func() error {
    // Only change phase if transitioning
    if shouldTransition {
        resource.Status.Phase = newPhase
        if isTerminal {
            now := metav1.Now()
            resource.Status.CompletionTime = &now
        }
    }

    // Always record attempts
    resource.Status.Attempts = append(resource.Status.Attempts, attempts...)
    resource.Status.AttemptCount++

    return nil
})
```

### **Pattern 3: Generic Status Manager**

**Use Case**: Reusable across all services

```go
// Generic manager works for any CRD
type Manager struct {
    client client.Client
}

// Type-safe wrappers per service
func (m *Manager) UpdateWorkflowStatus(
    ctx context.Context,
    workflow *v1alpha1.WorkflowExecution,
    phase v1alpha1.WorkflowPhase,
    steps []v1alpha1.ExecutionStep,
) error {
    return m.AtomicStatusUpdate(ctx, workflow, func() error {
        workflow.Status.Phase = phase
        workflow.Status.Steps = append(workflow.Status.Steps, steps...)
        workflow.Status.StepCount = len(workflow.Status.Steps)
        return nil
    })
}
```

---

## 📋 **Impacted Services & Priority**

| Service | Current Pattern | Priority | Estimated Reduction | Complexity |
|---------|----------------|----------|---------------------|------------|
| **Notification** | ✅ **COMPLETE** (reference impl) | P0 | 75-90% | DONE |
| **WorkflowExecution** | Sequential updates | P1 | 80-90% | Medium |
| **AIAnalysis** | Sequential updates | P1 | 70-85% | Medium |
| **SignalProcessing** | Sequential updates | P2 | 60-75% | Low |
| **RemediationOrchestrator** | Sequential updates | P2 | 70-85% | Medium |

---

## 📐 **Per-Controller Status Update Patterns** (Issue #79, 2026-02-18)

Each controller uses a different status update pattern based on its reconciliation flow:

| Controller | Pattern | Notes |
|------------|---------|-------|
| **SignalProcessing** | Callback-based `AtomicStatusUpdate(ctx, obj, func() error)` | `pkg/signalprocessing/status` or inline; batches phase + conditions + counters |
| **AIAnalysis** | Callback-based `AtomicStatusUpdate(ctx, obj, func() error)` | `pkg/aianalysis` status manager; batches phase + conditions |
| **WorkflowExecution** | Callback-based `AtomicStatusUpdate(ctx, obj, func() error)` | `pkg/workflowexecution`; batches phase + conditions |
| **Notification** | Fixed-signature `AtomicStatusUpdate(ctx, obj, conditions []metav1.Condition)` | **Critical**: `conditions` parameter passed in prevents refetch wipe. Without it, `Get` inside `RetryOnConflict` would overwrite in-memory conditions before `Update`. See `pkg/notification/status/manager.go`. |
| **RemediationOrchestrator (RR)** | `helpers.UpdateRemediationRequestStatus` callback | Callback receives refetched RR; batches phase + outcome + conditions |
| **RemediationOrchestrator (RAR)** | Direct retry loop with `Status().Update()` | No StatusManager; in-memory build + direct update (RAR has simpler status) |
| **EffectivenessMonitor** | In-memory build + direct `Status().Update()` | No StatusManager; no `AtomicStatusUpdate` pattern |

---

## ✅ **Acceptance Criteria**

### **Per-Service Completion Checklist**

- [ ] Status Manager created (`pkg/[service]/status/manager.go`)
- [ ] `AtomicStatusUpdate()` method implemented
- [ ] Controller refactored to use atomic updates
- [ ] Sequential `Status().Update()` calls removed
- [ ] Unit tests for status manager pass
- [ ] E2E tests pass unchanged
- [ ] Performance metrics show API call reduction
- [ ] Documentation updated

### **System-Wide Validation**

- [ ] All 5 services using atomic updates
- [ ] K8s API server load reduced by 60%+ (measured)
- [ ] No increase in status update conflicts (monitored)
- [ ] E2E test suites pass for all services
- [ ] Observability dashboard shows metrics

---

## 🚫 **Non-Goals**

This decision does **NOT** require:
- ❌ CRD schema changes
- ❌ API version bumps
- ❌ Migration scripts
- ❌ Backward compatibility handling
- ❌ Changes to E2E test assertions

**Rationale**: Atomic updates are an **internal optimization** - observable behavior is identical.

---

## 📚 **References**

- **Reference Implementation**: `pkg/notification/status/manager.go`
- **Performance Analysis**: `docs/handoff/NT_ATOMIC_STATUS_UPDATES_DEC_26_2025.md`
- **K8s Best Practices**: [Optimistic Concurrency Control](https://kubernetes.io/docs/reference/using-api/api-concepts/#optimistic-concurrency)
- **Related Pattern**: `docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md` (Pattern 2: Status Manager)

---

## 🔄 **Rollout Plan**

### **Phase 1: Foundation (Complete)**
- ✅ Implement reference pattern in Notification service
- ✅ Measure performance improvement (75-90% reduction confirmed)
- ✅ Create DD-PERF-001 mandate
- ✅ Create shared implementation guide

### **Phase 2: High-Priority Services (P1)**
- ⏳ WorkflowExecution (highest reconciliation frequency)
- ⏳ AIAnalysis (complex status with multiple arrays)

### **Phase 3: Medium-Priority Services (P2)**
- ⏳ RemediationOrchestrator
- ⏳ SignalProcessing

### **Phase 4: Validation & Documentation**
- ⏳ Measure system-wide API call reduction
- ⏳ Update observability dashboard
- ⏳ Document pattern in architecture guide

---

## 🎯 **Decision Rationale**

### **Why Mandatory?**

1. **System Health**: K8s API server load directly impacts cluster stability
2. **Consistency**: Single pattern across all services reduces cognitive load
3. **Performance**: 75-90% reduction is too significant to leave optional
4. **Quality**: Eliminates race conditions and inconsistent state windows
5. **Scalability**: Required for supporting higher reconciliation rates

### **Why Now?**

- ✅ Reference implementation proven (Notification service)
- ✅ Zero CRD changes required (low risk)
- ✅ E2E tests validate behavior (automated verification)
- ✅ Clear performance wins (measured 75-90% reduction)
- ✅ Pattern is reusable (apply to all services systematically)

---

## 📊 **Success Metrics**

### **Per-Service Metrics**

| Metric | Target | Measurement |
|--------|--------|-------------|
| API Call Reduction | >60% | Prometheus `k8s_api_calls_total` |
| Status Update Conflicts | No increase | Controller logs + metrics |
| E2E Test Pass Rate | 100% | CI/CD pipeline |
| Implementation Time | <1 day per service | Sprint tracking |

### **System-Wide Metrics**

| Metric | Target | Measurement |
|--------|--------|-------------|
| Total K8s API Calls | -60% | Prometheus aggregation |
| etcd Write Latency | No increase | etcd metrics |
| Controller Latency | No increase | Controller metrics |
| Watch Event Volume | -60% | K8s metrics |

---

## 🔐 **Risk Assessment**

### **Risks & Mitigations**

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Optimistic lock conflicts increase | Low | Medium | `RetryOnConflict` handles automatically |
| E2E tests break | Very Low | High | Tests validate observable behavior |
| Performance regression | Very Low | High | Atomic updates always faster than N+1 |
| Implementation bugs | Low | Medium | Reference impl + unit tests + E2E tests |

### **Rollback Plan**

If issues arise:
1. Keep old methods intact during migration (`UpdatePhase()`, `RecordAttempt()`)
2. Add feature flag per service for atomic vs sequential
3. Revert to sequential if metrics show problems
4. Atomic update is opt-in until validated

**Risk Level**: **LOW** (proven pattern, backward compatible, automated testing)

---

## ✅ **Approval**

**Decision Maker**: Engineering Lead
**Status**: ✅ **APPROVED**
**Date**: December 26, 2025

**Mandate Effective**: Immediately for new controllers, phased rollout for existing services

---

## 📝 **Change Log**

| Date | Change | Author |
|------|--------|--------|
| 2025-12-26 | Initial DD created | AI Assistant |
| 2025-12-26 | Reference implementation (Notification) | AI Assistant |
| 2025-12-26 | Mandate approved for system-wide rollout | Engineering Lead |
| 2026-02-18 | Per-controller status update patterns documented (Issue #79); NT conditions parameter extension noted | AI Assistant |

