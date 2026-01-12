# SignalProcessing Multi-Controller Migration

**Date**: January 11, 2026
**Pattern**: DD-TEST-010 Multi-Controller Architecture
**Reference**: AIAnalysis migration (AA_COMPLETE_SUCCESS_FINAL_JAN11_2026.md)
**Status**: In Progress

---

## 🎯 **Migration Objectives**

1. ✅ **APIReader Integration** - Cache-bypassed status refetch (SP-CACHE-001)
2. ⏳ **Multi-Controller Pattern** - Move controller from Phase 1 → Phase 2
3. ⏳ **Serial Marker Removal** - Enable full parallel execution
4. ⏳ **Test Validation** - Verify 100% pass rate in parallel

---

## ✅ **Phase 1: APIReader Integration** - COMPLETE

### Changes Made

**1. Status Manager** (`pkg/signalprocessing/status/manager.go`):
```go
type Manager struct {
    client    client.Client
    apiReader client.Reader  // ✅ ADDED
}

func NewManager(client client.Client, apiReader client.Reader) *Manager {  // ✅ UPDATED
    return &Manager{
        client:    client,
        apiReader: apiReader,  // ✅ ADDED
    }
}

// In AtomicStatusUpdate and UpdatePhase:
m.apiReader.Get()  // ✅ CHANGED from m.client.Get()
```

**2. Main Application** (`cmd/signalprocessing/main.go:311`):
```go
statusManager := spstatus.NewManager(mgr.GetClient(), mgr.GetAPIReader())  // ✅ UPDATED
```

**Benefits**:
- ✅ Prevents stale cached reads
- ✅ Ensures idempotency checks work correctly
- ✅ Eliminates duplicate API calls due to cache lag

---

## ⏳ **Phase 2: Multi-Controller Migration** - IN PROGRESS

### Current Architecture (Single-Controller)

**Phase 1** (Process 1 only):
1. Start infrastructure (PostgreSQL, Redis, DataStorage) ✅ **KEEP**
2. Create envtest ❌ **MOVE TO PHASE 2**
3. Create k8sManager ❌ **MOVE TO PHASE 2**
4. Setup controller ❌ **MOVE TO PHASE 2**
5. Start controller ❌ **MOVE TO PHASE 2**
6. Serialize REST config ❌ **DELETE (not needed)**

**Phase 2** (All processes):
1. Deserialize REST config ❌ **DELETE (not needed)**
2. Create k8sClient only ❌ **REPLACE WITH FULL SETUP**

### Target Architecture (Multi-Controller)

**Phase 1** (Process 1 only):
1. Start infrastructure (PostgreSQL, Redis, DataStorage) ✅ **KEEP**
2. **(THAT'S IT - no controller setup)**

**Phase 2** (All processes):
1. Start envtest (per process) ✅ **NEW**
2. Create k8sManager (per process) ✅ **NEW**
3. Setup audit store (per process) ✅ **NEW**
4. Setup controller (per process) ✅ **NEW**
5. Start controller (per process) ✅ **NEW**

---

## 📋 **Implementation Checklist**

### Phase 1 Refactoring (Keep Infrastructure Only)

- [ ] Remove `testEnv` creation (~line 248)
- [ ] Remove `testEnv.Start()` (~line 255)
- [ ] Remove `k8sClient` creation (~line 262)
- [ ] Remove audit store creation (~line 304-310)
- [ ] Remove `k8sManager` creation (~line 312-319)
- [ ] Remove controller setup (~line 321-558)
- [ ] Remove `k8sManager.Start()` (~line 580-585)
- [ ] Remove REST config serialization (~line 605-621)
- [ ] Keep only infrastructure startup and cleanup
- [ ] Return `[]byte{}` (no data to serialize)

### Phase 2 Implementation (Add Full Controller Setup)

- [ ] Remove REST config deserialization
- [ ] Add `testEnv` creation and start
- [ ] Add `k8sClient` creation
- [ ] Add audit store creation (per process, using shared infrastructure)
- [ ] Add `k8sManager` creation with `BindAddress: "0"`
- [ ] Add controller setup with all dependencies
- [ ] Add `k8sManager.Start()` in goroutine
- [ ] Pass `mgr.GetAPIReader()` to status manager

### Key Components to Move

**Components to replicate per process**:
1. `testEnv` - In-memory Kubernetes API server
2. `k8sManager` - Controller-runtime manager
3. `auditStore` - Buffered audit store (uses shared DataStorage)
4. `dsClient` - DataStorage HTTP client (uses shared infrastructure)
5. `auditClient` - Audit client wrapper
6. `statusManager` - Status update manager
7. Rego policy files - Environment/Priority classifiers
8. All business logic components:
   - `envClassifier` - Environment classification
   - `priorityEngine` - Priority assignment
   - `businessClassifier` - Business impact classification
   - `labelDetector` - Label detection
   - `regoEngine` - Rego evaluation engine
   - `k8sEnricher` - Kubernetes context enrichment
9. Controller reconciler setup
10. Manager start

**Shared infrastructure (stays in Phase 1)**:
- PostgreSQL (port 15436)
- Redis (port 16382)
- DataStorage (port 18094)
- Container health verification

---

## 🔧 **Technical Considerations**

### Rego Policy Files

**Challenge**: Temp files created in Phase 1, currently shared
**Solution**: Create temp files in Phase 2 (per process)
**Reason**: Each process needs its own file handles

### Audit Store

**Challenge**: Audit store uses shared DataStorage HTTP endpoint
**Solution**: Each process creates its own `auditStore` instance
**Reason**: Buffering and flushing is per-process, backend is shared

### Metrics

**Challenge**: Metrics tests might need reconciler access
**Solution**: Add global `var reconciler *signalprocessing.SignalProcessingReconciler` if needed
**Reason**: Metrics are per-process (each controller has own registry)

---

## 🚫 **Serial Markers to Address**

### Metrics Test (metrics_integration_test.go:60)

**Current**:
```go
var _ = Describe("Metrics Integration via Business Flows", Serial, Label("integration", "metrics"), func() {
```

**Action**: Remove `Serial`
**Reason**: Multi-controller pattern isolates metrics per process
**Confidence**: High (proven in AIAnalysis)

### Hot-Reload Test (hot_reloader_test.go:73)

**Current**:
```go
var _ = Describe("SignalProcessing Hot-Reload Integration", Serial, func() {
```

**Action**: **KEEP `Serial`**
**Reason**: Test manipulates shared policy files on disk
**Justification**: Legitimate shared resource that cannot be parallelized

**Comment to add**:
```go
// Serial: Hot-reload tests manipulate shared policy files and cannot run in parallel
// This is a legitimate shared resource constraint (not a metrics issue)
```

---

## 📈 **Expected Outcomes**

### Performance

- **Baseline** (Serial, 1 process): ~X minutes
- **Target** (Parallel, 4 processes): ~X * 0.75-0.85 minutes (15-25% improvement)

### Quality

- ✅ All tests pass in serial execution
- ✅ All tests pass in parallel execution (4 procs)
- ✅ No race conditions or duplicate API calls
- ✅ Proper controller isolation per process

### Maintainability

- ✅ Consistent with AIAnalysis pattern (DD-TEST-010)
- ✅ Clear separation: Phase 1 (infrastructure) / Phase 2 (controller)
- ✅ Easy to debug (isolated processes)

---

## 🔗 **Related Documentation**

**Design Decisions**:
- DD-TEST-010: Multi-Controller Pattern
- DD-STATUS-001: APIReader Pattern
- DD-CONTROLLER-001 v3.0: Pattern C Idempotency

**AIAnalysis Migration**:
- AA_COMPLETE_SUCCESS_FINAL_JAN11_2026.md
- AA_HAPI_001_API_READER_FIX_JAN11_2026.md
- AA_PARALLEL_EXECUTION_VALIDATION_JAN11_2026.md

**Triage**:
- MULTI_CONTROLLER_MIGRATION_TRIAGE_JAN11_2026.md

---

## ⚠️ **Risks & Mitigation**

| Risk | Probability | Impact | Mitigation |
|---|---|---|---|
| Rego policy file contention | Low | Medium | Create per-process temp files |
| Audit store race conditions | Low | High | Each process has own buffer |
| Metrics test failures | Low | Medium | Follow AIAnalysis pattern |
| Test timing changes | Medium | Low | Adjust timeouts if needed |

---

## 📝 **Progress Log**

### 2026-01-11T16:00 - Phase 1 Complete ✅
- ✅ APIReader added to status manager
- ✅ Main app updated to pass APIReader
- ✅ Backup created: `suite_test.go.bak-sp-multicontroller`

### 2026-01-11T16:15 - Phase 2 Complete ✅
- ✅ Test suite migrated to multi-controller pattern
- ✅ Phase 1: Now only starts infrastructure (PostgreSQL, Redis, DataStorage)
- ✅ Phase 2: Each process creates envtest + k8sManager + controller
- ✅ Serial marker removed from metrics test
- ✅ Serial marker kept for hot-reload test (legitimate shared resource)
- ✅ Per-process cleanup added (envtest + audit store flush)

### 2026-01-11T16:45 - Running Tests ⏳
- ⏳ Serial validation (TEST_PROCS=1)
- ⏳ Parallel validation (TEST_PROCS=4)

---

## 🎯 **Next Steps**

1. **Refactor Phase 1** - Keep only infrastructure startup
2. **Implement Phase 2** - Add full controller setup per process
3. **Remove Serial markers** - Except hot-reload test
4. **Run tests** - Validate serial + parallel execution
5. **Document learnings** - Update DD-TEST-010 if needed

---

**Status**: APIReader integration complete, test suite migration in progress
