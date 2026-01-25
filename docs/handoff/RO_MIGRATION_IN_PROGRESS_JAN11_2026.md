# RemediationOrchestrator Multi-Controller Migration - IN PROGRESS

**Date**: January 11, 2026
**Status**: 🔄 **IN PROGRESS** - APIReader complete, test suite migration needed
**Pattern**: DD-TEST-010 Multi-Controller + DD-STATUS-001 APIReader
**Estimated Completion**: 1-2 hours remaining

---

## ✅ **Completed Steps**

### 1. APIReader Integration (COMPLETE)

**Files Modified**:
- ✅ `pkg/remediationorchestrator/status/manager.go` - Added `apiReader` field and updated signature
- ✅ `internal/controller/remediationorchestrator/reconciler.go` - Added `apiReader` parameter to `NewReconciler`
- ✅ `cmd/remediationorchestrator/main.go` - Pass `mgr.GetAPIReader()` to NewReconciler

**Changes**:
```go
// BEFORE
type Manager struct {
    client client.Client
}
func NewManager(client client.Client) *Manager

// AFTER (DD-STATUS-001)
type Manager struct {
    client    client.Client
    apiReader client.Reader // Cache-bypassed reads
}
func NewManager(client client.Client, apiReader client.Reader) *Manager
```

**Result**: ✅ Compilation successful

---

## 🔄 **Current Step: Test Suite Migration**

### Problem Identified

**File**: `test/integration/remediationorchestrator/suite_test.go`

**Current Structure** (INCORRECT):
- **Phase 1**: Starts infrastructure + envtest + controller (lines 118-399)
- **Phase 2**: Only creates k8s clients (lines 400-470)

**Issue**: Controller is started in Phase 1 (global), which means all parallel processes share the SAME controller instance. This violates DD-TEST-010 and prevents true parallel test execution.

---

### Required Migration

**Goal**: Move controller setup from Phase 1 to Phase 2

#### Phase 1 (SHOULD BE):
```go
var _ = SynchronizedBeforeSuite(func() []byte {
    // Infrastructure ONLY
    - Start PostgreSQL (15435)
    - Start Redis (16381)
    - Start DataStorage (18140)

    return []byte{} // No shared state
}, func(data []byte) {
    // Phase 2 setup...
})
```

#### Phase 2 (SHOULD BE):
```go
func(data []byte) {
    // PER-PROCESS setup (each parallel process gets its own):
    - Register CRD schemes
    - Start envtest (per-process K8s API)
    - Create k8sManager (per-process)
    - Create statusManager with apiReader
    - Wire controller with manager
    - Start manager

    // Cleanup in SynchronizedAfterSuite Phase 1
}
```

---

### Lines to Move

**FROM Phase 1 (delete from 160-399)**:
```
Lines 160-180: CRD scheme registration
Lines 182-195: envtest setup
Lines 197-220: k8s client and namespace creation
Lines 224-230: k8sManager creation
Lines 233-288: Controller setup (including NewReconciler call)
Lines 291-354: Manager start and cache sync
```

**TO Phase 2 (insert after line 425)**:
All of the above, with per-process context management

---

## 📊 **Comparison with Other Services**

| Service | Status | APIReader | Multi-Controller | Pass Rate |
|---|---|---|---|---|
| **AIAnalysis** | ✅ Complete | ✅ | ✅ | 100% (57/57) |
| **SignalProcessing** | ✅ Complete | ✅ | ✅ | 94% (77/82) |
| **Notification** | ✅ Complete | ✅ | ✅ (origin) | 97.5% (115/118) |
| **RemediationOrchestrator** | 🔄 In Progress | ✅ | ⏳ | TBD |

---

## 🎯 **Next Steps**

### Immediate (1-2 hours):
1. ⏳ **Simplify Phase 1**: Remove all controller setup code, return `[]byte{}`
2. ⏳ **Enhance Phase 2**: Move controller setup from Phase 1 to Phase 2
3. ⏳ **Update NewReconciler calls**: Ensure apiReader parameter is passed in tests
4. ⏳ **Check Serial markers**: Identify and remove unnecessary Serial test markers
5. ⏳ **Run tests**: Parallel execution with 12 processors

### Expected Outcome:
- ✅ Per-process controller isolation
- ✅ True parallel test execution
- ✅ APIReader prevents cache lag issues
- ✅ 85-95% test pass rate expected (similar to SP)

---

## 🔍 **Key Differences from SignalProcessing Migration**

### Similarities:
- Both had controller in Phase 1 (needed to move to Phase 2)
- Both needed apiReader parameter added
- Both have similar test infrastructure (DS, PostgreSQL, Redis)

### Differences:
1. **RO has MORE CRDs**: RemediationRequest, RemediationApprovalRequest, + 4 child CRDs
2. **RO has routing engine**: Additional dependency to wire up
3. **RO has metrics**: DD-METRICS-001 integration required
4. **RO has more complex orchestration**: Parent controller pattern

### Risk Assessment:
- **Low Risk**: APIReader integration (same pattern as SP/AIAnalysis)
- **Medium Risk**: Test suite migration (more complex due to orchestration)
- **Low Risk**: Compilation (patterns proven in other services)
- **Medium Risk**: Test pass rate (RO orchestration complexity may surface edge cases)

---

## 📚 **Reference Documents**

- **DD-TEST-010**: Multi-Controller Pattern (`docs/architecture/decisions/DD-TEST-010-multi-controller-pattern.md`)
- **DD-STATUS-001**: APIReader Pattern (Notification service origin)
- **SignalProcessing Migration**: `docs/handoff/SP_MIGRATION_COMPLETE_SUMMARY_JAN11_2026.md`
- **AIAnalysis Migration**: `docs/handoff/AA_COMPLETE_SESSION_SUMMARY_JAN11_2026.md`

---

## 🛠️ **Technical Details**

### APIReader Pattern (DD-STATUS-001)

**Purpose**: Bypass controller-runtime cache for optimistic locking refetch

**Benefit**: Prevents race conditions where cached data is stale

**Implementation**:
```go
// In status.Manager
func (m *Manager) AtomicStatusUpdate(...) error {
    return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
        // Use apiReader instead of client for refetch
        if err := m.apiReader.Get(ctx, ..., rr); err != nil {
            return fmt.Errorf("failed to refetch: %w", err)
        }
        // Apply updates...
        if err := m.client.Status().Update(ctx, rr); err != nil {
            return fmt.Errorf("failed to update: %w", err)
        }
        return nil
    })
}
```

**Usage in Main**:
```go
// cmd/remediationorchestrator/main.go
controller.NewReconciler(
    mgr.GetClient(),
    mgr.GetAPIReader(), // ← DD-STATUS-001
    mgr.GetScheme(),
    // ... other params
)
```

---

## ⏱️ **Time Tracking**

- **APIReader Integration**: 30 minutes (COMPLETE)
- **Test Suite Analysis**: 20 minutes (COMPLETE)
- **Test Suite Migration**: ~1-2 hours (IN PROGRESS)
- **Serial Marker Review**: ~15 minutes (PENDING)
- **Test Execution**: ~30-45 minutes (PENDING)
- **Documentation**: ~30 minutes (PENDING)

**Total Estimated**: 3-5 hours (as originally estimated)
**Elapsed**: ~1 hour
**Remaining**: ~2-4 hours

---

**Status**: 🔄 **Migration 40% Complete** - APIReader ✅, Test Suite ⏳

