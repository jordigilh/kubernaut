# BR-SP-072 Hot-Reload Implementation - FINAL TRIAGE

**Date**: 2025-12-13 15:00 PST
**Status**: 🔍 **COMPLETE ASSESSMENT**

---

## 📊 COMPREHENSIVE TRIAGE RESULTS

### Summary Table

| Component | Has policyPath? | Has Mutex? | Has LoadPolicy? | Has FileWatcher? | Has StartHotReload? | **Status** |
|-----------|----------------|------------|-----------------|------------------|---------------------|------------|
| **Priority Engine** | ✅ YES | ✅ YES | ✅ YES | ✅ YES | ✅ YES | **✅ COMPLETE** (just wired up) |
| **Environment Classifier** | ❌ NO | ⚠️ PARTIAL | ❌ NO | ❌ NO | ❌ NO | **❌ NEEDS FULL IMPLEMENTATION** |
| **CustomLabels Engine** | ✅ YES | ✅ YES | ✅ YES | ❌ NO | ❌ NO | **⚠️ 60% COMPLETE** (needs FileWatcher) |

---

## 🔍 DETAILED FINDINGS

### 1. Priority Engine ✅ **COMPLETE**

**File**: `pkg/signalprocessing/classifier/priority.go`

**What Exists**:
```go
type PriorityEngine struct {
    regoQuery   *rego.PreparedEvalQuery
    fileWatcher *hotreload.FileWatcher  // Line 68 ✅
    policyPath  string                  // Line 69 ✅
    logger      logr.Logger
    mu          sync.RWMutex            // Line 71 ✅
}

// Lines 235-273: Full hot-reload implementation ✅
func (p *PriorityEngine) StartHotReload(ctx context.Context) error
func (p *PriorityEngine) Stop()
func (p *PriorityEngine) GetPolicyHash() string
```

**Status**: ✅ **DONE** - Just wired up in `main.go` (completed 30min ago)

---

### 2. Environment Classifier ❌ **NEEDS FULL IMPLEMENTATION**

**File**: `pkg/signalprocessing/classifier/environment.go`

**What Exists**:
```go
type EnvironmentClassifier struct {
    regoQuery *rego.PreparedEvalQuery  // Line 67
    k8sClient client.Client            // Line 68
    logger    logr.Logger              // Line 69

    // ConfigMap cache (NOT for Rego policy)
    configMapMu      sync.RWMutex       // Line 72 - ONLY for ConfigMap mapping
    configMapMapping map[string]string  // Line 73
}

// Line 389: ONLY reloads ConfigMap mapping, NOT Rego policy
func (c *EnvironmentClassifier) ReloadConfigMap(ctx context.Context) error
```

**What's Missing**:
- ❌ NO `policyPath` field stored
- ❌ NO `mu sync.RWMutex` for Rego policy (existing mutex is for ConfigMap only)
- ❌ NO `fileWatcher` field
- ❌ NO `StartHotReload` method
- ❌ NO `Stop` method
- ❌ NO `GetPolicyHash` method

**Complexity**: **HIGH** - Needs complete hot-reload infrastructure

**Estimated Time**: **1.5-2h**

---

### 3. CustomLabels Engine ⚠️ **60% COMPLETE**

**File**: `pkg/signalprocessing/rego/engine.go`

**What Exists**:
```go
type Engine struct {
    logger       logr.Logger
    policyPath   string                 // Line 73 ✅ ALREADY STORED
    policyModule string                 // Line 74 - stores policy content
    mu           sync.RWMutex           // Line 75 ✅ ALREADY EXISTS
}

// Lines 102-111: LoadPolicy with mutex ✅ ALREADY EXISTS
func (e *Engine) LoadPolicy(policyContent string) error {
    e.mu.Lock()
    defer e.mu.Unlock()
    e.policyModule = policyContent
    return nil
}
```

**What's Missing**:
- ❌ NO `fileWatcher` field
- ❌ NO `StartHotReload` method
- ❌ NO `Stop` method
- ❌ NO `GetPolicyHash` method
- ⚠️ `LoadPolicy` exists but has NO validation (accepts any string)

**Complexity**: **MEDIUM** - Infrastructure 60% done, just needs FileWatcher wiring

**Estimated Time**: **1-1.5h**

---

## 🎯 REVISED IMPLEMENTATION PLAN

### Phase 1: CustomLabels Engine (EASIER) - 1-1.5h

**Why Start Here**: 60% complete, easier win

**Tasks**:
1. Add `fileWatcher *hotreload.FileWatcher` field
2. Add `StartHotReload(ctx)` method (copy from Priority Engine pattern)
3. Add `Stop()` method
4. Add `GetPolicyHash()` method
5. Add validation to `LoadPolicy` (compile Rego to check syntax)
6. Wire up in `main.go`

**Pattern to Copy**: `priority.go` lines 235-273

---

### Phase 2: Environment Classifier (HARDER) - 1.5-2h

**Why Second**: More complex, needs more infrastructure

**Tasks**:
1. Add `policyPath string` field
2. Add second mutex `policyMu sync.RWMutex` (keep `configMapMu` separate)
3. Add `fileWatcher *hotreload.FileWatcher` field
4. Refactor `Classify` to use `policyMu` for reading `regoQuery`
5. Add `StartHotReload(ctx)` method
6. Add `Stop()` method
7. Add `GetPolicyHash()` method
8. Wire up in `main.go`

**Pattern to Copy**: `priority.go` lines 235-273

---

## ⏱️ FINAL TIME ESTIMATES

| Task | Original Estimate | Revised Estimate | Reason |
|------|------------------|------------------|--------|
| Priority Engine | 2h | **✅ 0.5h (DONE)** | Already implemented |
| CustomLabels Engine | 1h | **1-1.5h** | 60% complete |
| Environment Classifier | 1h | **1.5-2h** | Needs full implementation |
| Policy Validation | 1.5h | **INCLUDED** | Priority has pattern |
| Graceful Degradation | 1.5h | **INCLUDED** | Priority has pattern |
| Component APIs | 2h | **1-2h** | TBD - check if exported |
| Test Fixes | 2h | **2-3h** | Same as before |
| **TOTAL** | **11h** | **5.5-9h** | **50% less work** |

---

## 🚀 RECOMMENDED APPROACH

### Option A: Complete All 3 Engines (5.5-9h)
**What**: Implement hot-reload for Environment + CustomLabels
**Time**: 2.5-3.5h implementation + 3-5.5h testing
**Result**: All tests passing, full BR-SP-072 implementation

### Option B: Priority Engine Only (CURRENT)
**What**: Stop here, mark other engines as V1.1
**Time**: 0h additional
**Result**: Partial hot-reload, 12 test failures remain

---

## 💡 RECOMMENDATION

**Proceed with Option A** - Complete all 3 engines

**Rationale**:
1. **Priority Engine pattern proven** - just copy it twice
2. **CustomLabels 60% done** - low-hanging fruit (1-1.5h)
3. **Total time manageable** - 5.5-9h vs original 8-12h
4. **Tests expect all 3** - 12 failures need all engines implemented
5. **Consistent with HolmesGPT** - they have full hot-reload

**Confidence**: **90%** - We can complete this today

---

## 📋 NEXT STEPS

**Immediate**:
1. Start with CustomLabels Engine (1-1.5h) - easier win
2. Then Environment Classifier (1.5-2h) - harder one
3. Then Component APIs (1-2h)
4. Finally test fixes (2-3h)

**Total Remaining**: **5.5-8.5h**

---

**Last Updated**: 2025-12-13 15:00 PST
**Status**: Awaiting decision - proceed with full implementation?


