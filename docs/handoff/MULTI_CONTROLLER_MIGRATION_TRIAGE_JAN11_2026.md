# Multi-Controller Migration Triage

**Date**: January 11, 2026
**Services**: RemediationOrchestrator (RO), SignalProcessing (SP), Notification (NT)
**Pattern**: DD-TEST-010 Multi-Controller Architecture
**Purpose**: Assess readiness for multi-controller + APIReader migration

---

## 🎯 **Executive Summary**

| Service | Current Pattern | APIReader | Serial Markers | Effort | Priority |
|---|---|---|---|---|---|
| **Notification** | ✅ Multi-Controller | ✅ Has APIReader | 6 markers | **1-2 hours** | Low (already done) |
| **SignalProcessing** | ❌ Single-Controller | ❌ No APIReader | 4 markers | **2-4 hours** | Medium |
| **RemediationOrchestrator** | ❌ Single-Controller | ❌ No APIReader | 4 markers | **3-5 hours** | High (complexity) |

**Key Finding**: **Notification already uses multi-controller pattern!** Only needs Serial marker removal.

---

## 📊 **Detailed Service Analysis**

### 1. Notification (NT) - ✅ ALREADY MULTI-CONTROLLER

#### Current State

**Architecture**: ✅ **Already Multi-Controller**
- **Phase 1**: Starts shared infrastructure only (PostgreSQL, Redis, DataStorage)
- **Phase 2**: Each process creates envtest + k8sManager + controller
- **Pattern**: Same as AIAnalysis post-migration

**APIReader**: ✅ **Already Integrated (DD-STATUS-001)**
```go
// test/integration/notification/suite_test.go:337
statusManager := notificationstatus.NewManager(
    k8sManager.GetClient(),
    k8sManager.GetAPIReader()  // ✅ APIReader already passed
)
```

**Status Manager**: ✅ Has APIReader parameter
```go
// pkg/notification/status/manager.go
type Manager struct {
    client    client.Client
    apiReader client.Reader  // ✅ Already has APIReader
}
```

**Test Stats**:
- **Test Files**: 20
- **Test Specs**: 19
- **Serial Markers**: 6 (performance tests only)

#### Migration Needed

**✅ No Architecture Changes Required**

**Work Required**:
1. **Remove Serial markers** from performance tests (if appropriate)
2. **Validate** tests pass in parallel
3. **Document** multi-controller setup (already working)

**Estimated Effort**: **1-2 hours**
- Remove Serial markers: 30 min
- Run parallel tests: 30 min
- Documentation: 30 min

**Confidence**: 95% - Already working, just needs validation

---

### 2. SignalProcessing (SP) - ❌ SINGLE-CONTROLLER

#### Current State

**Architecture**: ❌ **Single-Controller Pattern**
- **Phase 1**: Starts infrastructure + **creates controller** (single-controller)
- **Phase 2**: Receives serialized config, creates k8sClient only
- **Issue**: Only process 1 has controller, others just have client

**Evidence**:
```go
// test/integration/signalprocessing/suite_test.go:313 (Phase 1)
k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{...})  // ❌ In Phase 1

// test/integration/signalprocessing/suite_test.go:537
statusManager := spstatus.NewManager(k8sManager.GetClient())  // ❌ No APIReader
```

**APIReader**: ❌ **Not Integrated**
```go
// pkg/signalprocessing/status/manager.go
type Manager struct {
    client client.Client  // ❌ No apiReader field
}

func NewManager(client client.Client) *Manager {  // ❌ No apiReader parameter
```

**Test Stats**:
- **Test Files**: 7
- **Test Specs**: 7
- **Serial Markers**: 4
  - `metrics_integration_test.go` - Serial (metrics)
  - `hot_reloader_test.go` - Serial (shared policy files)

#### Migration Required

**Phase 1: APIReader Integration**

1. **Update Status Manager** (`pkg/signalprocessing/status/manager.go`):
```go
type Manager struct {
    client    client.Client
    apiReader client.Reader  // ADD
}

func NewManager(client client.Client, apiReader client.Reader) *Manager {  // ADD apiReader param
    return &Manager{
        client:    client,
        apiReader: apiReader,  // ADD
    }
}

func (m *Manager) AtomicStatusUpdate(...) error {
    // Change refetch from:
    m.client.Get()  // ❌ Cached
    // To:
    m.apiReader.Get()  // ✅ Fresh
}
```

2. **Update Main Application** (`cmd/signalprocessing/main.go`):
```go
// Find line with:
statusManager := spstatus.NewManager(mgr.GetClient())

// Change to:
statusManager := spstatus.NewManager(mgr.GetClient(), mgr.GetAPIReader())
```

**Phase 2: Multi-Controller Migration**

3. **Update Test Suite** (`test/integration/signalprocessing/suite_test.go`):

**Current Phase 1** (lines ~313):
```go
// Phase 1: Start infrastructure + controller (WRONG)
k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{...})
statusManager := spstatus.NewManager(k8sManager.GetClient())
// ... controller setup ...
```

**New Phase 1**:
```go
// Phase 1: Start infrastructure ONLY (no controller)
// ... PostgreSQL, Redis, DataStorage startup ...
// NO k8sManager, NO controller
```

**New Phase 2**:
```go
// Phase 2: Per-process envtest + controller
envtest.start()  // Each process
k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
    BindAddress: "0",  // Random port per process
})
statusManager := spstatus.NewManager(k8sManager.GetClient(), k8sManager.GetAPIReader())
reconciler := &signalprocessing.SignalProcessingReconciler{...}
reconciler.SetupWithManager(k8sManager)
go k8sManager.Start()
```

4. **Remove Serial Markers**:
- `metrics_integration_test.go` - Remove Serial (metrics now isolated)
- `hot_reloader_test.go` - Keep Serial (shared policy files justification)

**Phase 3: Validation**

5. **Run Tests**:
```bash
make test-integration-signalprocessing TEST_PROCS=1  # Baseline
make test-integration-signalprocessing TEST_PROCS=4  # Parallel
```

#### Estimated Effort

**Total**: **2-4 hours**
- APIReader integration: 1 hour
- Multi-controller migration: 1-2 hours
- Serial marker removal: 30 min
- Testing and validation: 1 hour

**Confidence**: 90% - Straightforward migration following AIAnalysis pattern

**Risks**: Low
- Well-documented pattern (DD-TEST-010)
- Status manager already exists
- Hot-reloader might still need Serial (shared files)

---

### 3. RemediationOrchestrator (RO) - ❌ SINGLE-CONTROLLER

#### Current State

**Architecture**: ❌ **Single-Controller Pattern**
- **Phase 1**: Starts infrastructure + **creates controller** (single-controller)
- **Phase 2**: Receives serialized config, creates k8sClient only
- **Issue**: Only process 1 has controller

**Evidence**:
```go
// test/integration/remediationorchestrator/suite_test.go:222 (Phase 1)
k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{...})  // ❌ In Phase 1
```

**APIReader**: ❌ **Not Integrated**
```go
// pkg/remediationorchestrator/status/manager.go
type Manager struct {
    client client.Client  // ❌ No apiReader field
}

func NewManager(client client.Client) *Manager {  // ❌ No apiReader parameter
```

**Test Stats**:
- **Test Files**: 14
- **Test Specs**: 14
- **Serial Markers**: 4
  - `operational_metrics_integration_test.go` - Serial (metrics)
  - `operational_test.go` - Serial (high load, 300-400 audit events)

**Complexity Note**: RO has documented high-load issue:
> "⚠️ CRITICAL: This test MUST run in Serial mode. Reason: Generates ~300-400 audit events that can crash DataStorage under default 2GB memory limit"
> - See: `RO_DATASTORAGE_CRASH_ROOT_CAUSE_DEC_24_2025.md`

#### Migration Required

**Phase 1: APIReader Integration**

1. **Update Status Manager** (`pkg/remediationorchestrator/status/manager.go`):
```go
type Manager struct {
    client    client.Client
    apiReader client.Reader  // ADD
}

func NewManager(client client.Client, apiReader client.Reader) *Manager {
    return &Manager{
        client:    client,
        apiReader: apiReader,
    }
}

func (m *Manager) AtomicStatusUpdate(...) error {
    m.apiReader.Get()  // Change from m.client.Get()
}
```

2. **Update Main Application** (`cmd/remediationorchestrator/main.go`):
```go
statusManager := rostatus.NewManager(mgr.GetClient(), mgr.GetAPIReader())
```

**Phase 2: Multi-Controller Migration**

3. **Update Test Suite** (`test/integration/remediationorchestrator/suite_test.go`):

**Current Phase 1**:
```go
// Phase 1: Infrastructure + controller (WRONG)
k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{...})
// ... controller setup ...
```

**New Phase 1**:
```go
// Phase 1: Infrastructure ONLY
// PostgreSQL, Redis, DataStorage, HAPI
// NO k8sManager, NO controller
```

**New Phase 2**:
```go
// Phase 2: Per-process controller
envtest.start()
k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{BindAddress: "0"})
statusManager := rostatus.NewManager(k8sManager.GetClient(), k8sManager.GetAPIReader())
reconciler := &remediationorchestrator.RemediationOrchestratorReconciler{...}
reconciler.SetupWithManager(k8sManager)
go k8sManager.Start()
```

4. **Handle Serial Markers**:
- `operational_metrics_integration_test.go` - Remove Serial (metrics now isolated)
- `operational_test.go` - **Keep Serial** (high-load DataStorage limitation)
  - This is a known infrastructure limit, not a code issue
  - Document as exception in DD-TEST-010

**Phase 3: Validation**

5. **Run Tests**:
```bash
make test-integration-remediationorchestrator TEST_PROCS=1
make test-integration-remediationorchestrator TEST_PROCS=4
```

#### Estimated Effort

**Total**: **3-5 hours**
- APIReader integration: 1-1.5 hours
- Multi-controller migration: 1.5-2 hours
- Serial marker analysis: 30 min
- Testing and validation: 1-1.5 hours
- High-load test consideration: +30 min

**Confidence**: 85% - More complex due to high-load test considerations

**Risks**: Medium
- High-load test might expose infrastructure limits
- 300-400 audit events per test could cause issues in parallel
- May need to keep some Serial markers for resource-intensive tests

---

## 🔄 **Migration Checklist Template**

### Per Service

#### Phase 1: APIReader Integration (1 hour)
- [ ] Update `pkg/[service]/status/manager.go`:
  - [ ] Add `apiReader client.Reader` field to Manager struct
  - [ ] Update `NewManager()` signature to accept `apiReader`
  - [ ] Change `m.client.Get()` to `m.apiReader.Get()` in refetch operations
- [ ] Update `cmd/[service]/main.go`:
  - [ ] Pass `mgr.GetAPIReader()` to status manager

#### Phase 2: Multi-Controller Setup (1-2 hours)
- [ ] Update `test/integration/[service]/suite_test.go`:
  - [ ] **Phase 1**: Remove controller setup, keep infrastructure only
  - [ ] **Phase 2**: Add per-process envtest + k8sManager + controller
  - [ ] Pass `k8sManager.GetAPIReader()` to status manager in tests
  - [ ] Store reconciler instance in global variable (for metrics access)

#### Phase 3: Serial Marker Removal (30 min)
- [ ] Identify Serial markers related to:
  - [ ] Metrics tests → Remove (now isolated per process)
  - [ ] Shared file tests → Keep (legitimate reason)
  - [ ] High-load tests → Evaluate case-by-case

#### Phase 4: Validation (1 hour)
- [ ] Run tests in serial: `TEST_PROCS=1`
- [ ] Run tests in parallel: `TEST_PROCS=4`
- [ ] Verify no duplicate API calls in logs
- [ ] Verify metrics tests pass without Serial
- [ ] Document any Serial markers kept with justification

---

## 📊 **Migration Priority & Order**

### Recommended Order

1. **Notification (NT)** - 1-2 hours
   - **Reason**: Already done, just needs validation
   - **Value**: Confirms multi-controller pattern works
   - **Risk**: Minimal

2. **SignalProcessing (SP)** - 2-4 hours
   - **Reason**: Simpler than RO, fewer tests
   - **Value**: Practice run before RO
   - **Risk**: Low (straightforward migration)

3. **RemediationOrchestrator (RO)** - 3-5 hours
   - **Reason**: Most complex, high-load considerations
   - **Value**: Complete migration of all CRD services
   - **Risk**: Medium (high-load test needs careful handling)

**Total Effort**: **6-11 hours** for all three services

---

## 🎯 **Success Criteria**

### Per Service

1. ✅ **APIReader Integrated**:
   - Status manager has `apiReader` field
   - All refetch operations use `apiReader.Get()`
   - Main app and tests pass `mgr.GetAPIReader()`

2. ✅ **Multi-Controller Pattern**:
   - Phase 1: Infrastructure only
   - Phase 2: Per-process controller setup
   - Each process has isolated: envtest, manager, controller, metrics

3. ✅ **Serial Markers Justified**:
   - Metrics tests run in parallel (no Serial)
   - Only legitimate Serial markers remain (shared files, high-load)
   - Each Serial marker documented with reason

4. ✅ **Tests Pass**:
   - 100% pass rate in serial execution
   - 100% pass rate in parallel execution (4 procs)
   - No race conditions or duplicate API calls

---

## 📈 **Expected Benefits**

### Performance
- **Test Duration**: 15-25% faster with 4 parallel processes
- **CI/CD**: Reduced pipeline time
- **Developer Experience**: Faster feedback loop

### Quality
- **Race Conditions**: Exposed early (parallel execution)
- **Idempotency**: Reliable with APIReader
- **Flakiness**: Eliminated (proper isolation)

### Maintainability
- **Consistent Pattern**: All services follow DD-TEST-010
- **Reusable Knowledge**: Pattern documented once, used everywhere
- **Easy Debugging**: Isolated processes, no shared state

---

## 🔗 **Related Documentation**

**Authoritative Design Decisions**:
- **DD-TEST-010**: Multi-Controller Pattern (AIAnalysis implementation)
- **DD-STATUS-001**: APIReader Pattern (Notification implementation)
- **DD-CONTROLLER-001 v3.0**: Pattern C Idempotency

**Handoff Documents**:
- `AA_COMPLETE_SUCCESS_FINAL_JAN11_2026.md` - AIAnalysis migration success
- `AA_PARALLEL_EXECUTION_VALIDATION_JAN11_2026.md` - Parallel validation
- `AA_HAPI_001_API_READER_FIX_JAN11_2026.md` - APIReader implementation

---

## ⚠️ **Known Considerations**

### Notification (NT)
- **Already multi-controller** ✅
- Serial markers in performance tests may be intentional (stress testing)
- Validate that existing Serial markers are justified

### SignalProcessing (SP)
- Hot-reloader test manipulates shared policy files
  - **Keep Serial** for this test (legitimate shared resource)
- Other Serial markers (metrics) should be removable

### RemediationOrchestrator (RO)
- High-load test generates 300-400 audit events
  - **May need to keep Serial** due to DataStorage memory limit
  - Or: Increase DataStorage resources for parallel execution
  - Document exception in DD-TEST-010
- Complex orchestration logic may have edge cases

---

## 🎯 **Confidence Assessment**

| Service | Migration Confidence | Test Confidence | Notes |
|---|---|---|---|
| **Notification** | 95% | 95% | Already done, just validate |
| **SignalProcessing** | 90% | 85% | Straightforward, hot-reloader needs care |
| **RemediationOrchestrator** | 85% | 80% | High-load test needs evaluation |

**Overall Confidence**: 90% - Well-documented pattern, proven in AIAnalysis

---

## 📝 **Next Steps**

### Option A: Sequential Migration (Recommended)
1. Validate Notification (1-2 hours)
2. Migrate SignalProcessing (2-4 hours)
3. Migrate RemediationOrchestrator (3-5 hours)
4. Document learnings and update DD-TEST-010

**Total**: 6-11 hours

### Option B: Parallel Investigation
- Audit all three services' Serial markers first
- Categorize by reason (metrics, shared files, high-load)
- Create targeted migration plan per service

**Total**: 8-13 hours (more analysis upfront)

---

## ✅ **Triage Complete**

**Summary**:
- **Notification**: ✅ Already multi-controller, needs validation
- **SignalProcessing**: ❌ Needs full migration (APIReader + multi-controller)
- **RemediationOrchestrator**: ❌ Needs full migration (APIReader + multi-controller) + high-load consideration

**Recommended**: Start with Notification validation, then SP, then RO

**Total Effort**: 6-11 hours for all three services

