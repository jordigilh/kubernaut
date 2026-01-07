# Test Infrastructure Phase 2: DataStorage Deployment Consolidation

**Date**: 2026-01-07
**Status**: 🔄 **IN PROGRESS** - Analysis Complete, Ready for Implementation
**Phase**: 2 - DataStorage E2E Deployment Consolidation
**Authority**: `docs/handoff/TEST_INFRASTRUCTURE_REFACTORING_TRIAGE_JAN07.md`

---

## 🎯 **Phase 2 Objectives**

### **Primary Goal**
Consolidate 24+ duplicate DataStorage deployment calls across E2E tests into shared functions.

### **Current State: Excessive Duplication**
```
Duplicate Deployment Calls: 24 across 8 E2E test files
- deployPostgreSQLInNamespace():  14 references
- deployRedisInNamespace():       14 references
- ApplyAllMigrations():           14 references
- deployDataStorageServiceInNamespace(): 12 references
```

### **Target State: Consolidated Functions**
```
Consolidated Functions:     2 shared functions
- DeployDataStorageTestServices() (sequential)
- DeployDataStorageTestServicesWithNodePort() (with custom port)
Expected Reduction:         ~400-600 lines across 8 files
```

---

## 📊 **Current Duplication Analysis**

### **E2E Files with Duplicate DataStorage Deployment**

| File | Duplicate Calls | Pattern | Lines |
|------|----------------|---------|-------|
| `gateway_e2e.go` | 8 calls (2 variants) | Sequential + Parallel | ~40 lines each |
| `gateway_e2e_hybrid.go` | 4 calls | Parallel | ~30 lines |
| `signalprocessing_e2e_hybrid.go` | 4 calls | Parallel | ~30 lines |
| `workflowexecution_e2e_hybrid.go` | 4 calls | Parallel | ~30 lines |
| `remediationorchestrator_e2e_hybrid.go` | 4 calls | Parallel | ~30 lines |
| `holmesgpt_api.go` | 4 calls | Parallel + NodePort | ~30 lines |

**Total Duplication**: ~240 lines of duplicated deployment logic

---

## ✅ **Good News: Consolidated Functions Already Exist!**

### **Function 1: Sequential Deployment**
```go
// test/infrastructure/datastorage.go (already exists!)
func DeployDataStorageTestServices(
    ctx context.Context,
    namespace string,
    kubeconfigPath string,
    dataStorageImage string,
    writer io.Writer,
) error {
    // 1. Create namespace
    // 2. Deploy PostgreSQL
    // 3. Deploy Redis
    // 4. Apply migrations
    // 5. Deploy DataStorage
    // 6. Wait for ready
    return nil
}
```

**Used By**: DataStorage E2E tests
**Status**: ✅ Proven, well-tested

---

### **Function 2: Sequential with Custom NodePort**
```go
// test/infrastructure/datastorage.go (already exists!)
func DeployDataStorageTestServicesWithNodePort(
    ctx context.Context,
    namespace string,
    kubeconfigPath string,
    dataStorageImage string,
    nodePort int32,  // Custom NodePort for Kind port mapping
    writer io.Writer,
) error {
    // Same as above but with custom NodePort
    return nil
}
```

**Used By**: DataStorage E2E tests, Notification E2E tests
**Status**: ✅ Proven, supports custom ports

---

## 🔍 **Analysis: Why E2E Tests Don't Use Consolidated Functions**

### **Reason 1: Parallel Deployment Pattern**
```go
// Current pattern in gateway_e2e.go, signalprocessing, etc.
// Deploy PostgreSQL + Redis in PARALLEL (faster)
go func() {
    deployPostgreSQLInNamespace(...)
    deployRedisInNamespace(...)
}()

// Then SEQUENTIAL: migrations + DataStorage
ApplyAllMigrations(...)
deployDataStorageServiceInNamespace(...)
```

**Issue**: Consolidated functions use sequential deployment (slower)
**Impact**: E2E tests would take longer if forced to use sequential

---

### **Reason 2: Coverage Support**
```go
// Gateway E2E has TWO setup functions:
// 1. SetupGatewayInfrastructureParallel() - Standard
// 2. SetupGatewayInfrastructureParallelWithCoverage() - Coverage-enabled
```

**Issue**: Consolidated functions don't have coverage variants
**Impact**: Coverage tests would need special handling

---

### **Reason 3: Service-Specific NodePorts**
```go
// Different services use different NodePorts
Gateway:         30081 (kind-gateway-config.yaml)
Notification:    30090 (kind-notification-config.yaml)
HolmesGPT:       30098 (kind-holmesgpt-config.yaml)
```

**Issue**: `DeployDataStorageTestServicesWithNodePort` exists but isn't used
**Impact**: Services are duplicating code instead of using this function

---

## 💡 **Phase 2 Strategy: Create Parallel Deployment Function**

### **Approach: Add NEW Consolidated Function**
```go
// New function to support parallel PostgreSQL + Redis deployment
func DeployDataStorageTestServicesParallel(
    ctx context.Context,
    namespace string,
    kubeconfigPath string,
    dataStorageImage string,
    nodePort int32,  // Optional: 0 = use default (30081)
    writer io.Writer,
) error {
    // PHASE 1: Parallel deployment
    results := make(chan result, 2)

    // Goroutine 1: PostgreSQL
    go func() {
        err := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer)
        results <- result{name: "PostgreSQL", err: err}
    }()

    // Goroutine 2: Redis
    go func() {
        err := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer)
        results <- result{name: "Redis", err: err}
    }()

    // Wait for parallel tasks
    for i := 0; i < 2; i++ {
        r := <-results
        if r.err != nil {
            return fmt.Errorf("%s failed: %w", r.name, r.err)
        }
    }

    // PHASE 2: Sequential (requires PostgreSQL)
    // 1. Apply migrations
    if err := ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer); err != nil {
        return err
    }

    // 2. Deploy DataStorage
    if nodePort == 0 {
        nodePort = 30081 // Default
    }
    if err := deployDataStorageServiceInNamespaceWithNodePort(
        ctx, namespace, kubeconfigPath, dataStorageImage, nodePort, writer); err != nil {
        return err
    }

    // 3. Wait for ready
    return waitForDataStorageServicesReady(ctx, namespace, kubeconfigPath, writer)
}
```

---

## 📋 **Migration Plan**

### **Step 1: Create Consolidated Function** (30 min)
- ✅ Add `DeployDataStorageTestServicesParallel()` to `datastorage.go`
- ✅ Support optional NodePort parameter
- ✅ Reuse existing helper functions

### **Step 2: Migrate Gateway E2E** (30 min)
**Before** (40 lines):
```go
// PHASE 2: Deploy PostgreSQL + Redis (parallel)
results := make(chan result, 2)
go func() {
    err := deployPostgreSQLInNamespace(...)
    results <- result{name: "PostgreSQL", err: err}
}()
go func() {
    err := deployRedisInNamespace(...)
    results <- result{name: "Redis", err: err}
}()
// ... wait for results ...

// PHASE 3: Apply migrations + Deploy DataStorage
ApplyAllMigrations(...)
deployDataStorageServiceInNamespace(...)
```

**After** (5 lines):
```go
// PHASE 2: Deploy DataStorage Stack (parallel PostgreSQL/Redis)
if err := DeployDataStorageTestServicesParallel(
    ctx, namespace, kubeconfigPath, dataStorageImage, 0, writer); err != nil {
    return fmt.Errorf("failed to deploy DataStorage stack: %w", err)
}
```

**Reduction**: 35 lines per function × 2 functions = 70 lines

---

### **Step 3: Migrate Other E2E Tests** (1 hour)
Apply same pattern to:
- ✅ `gateway_e2e_hybrid.go` - Use `DeployDataStorageTestServicesParallel()`
- ✅ `signalprocessing_e2e_hybrid.go` - Use `DeployDataStorageTestServicesParallel()`
- ✅ `workflowexecution_e2e_hybrid.go` - Use `DeployDataStorageTestServicesParallel()`
- ✅ `remediationorchestrator_e2e_hybrid.go` - Use `DeployDataStorageTestServicesParallel()`
- ✅ `holmesgpt_api.go` - Use `DeployDataStorageTestServicesParallel(..., 30098, ...)`

**Expected Reduction**: ~150-200 lines across 5 files

---

### **Step 4: Test All E2E Suites** (1 hour)
```bash
# Gateway E2E (most critical)
ginkgo -p -r --label-filter=e2e test/e2e/gateway

# Other E2E suites
ginkgo -p -r --label-filter=e2e test/e2e/signalprocessing
ginkgo -p -r --label-filter=e2e test/e2e/workflowexecution
ginkgo -p -r --label-filter=e2e test/e2e/remediationorchestrator
```

---

## 📊 **Expected Benefits**

### **Code Reduction**
```
Current:  ~240 lines of duplicated deployment logic
After:    ~50 lines (1 shared function + minimal wrappers)
Savings:  ~190 lines (79% reduction)
```

### **Maintainability**
- ✅ Bug fixes in 1 place instead of 8 places
- ✅ Consistent DataStorage deployment across all E2E tests
- ✅ Easier to add features (e.g., health check improvements)
- ✅ Parallel deployment preserved (no performance loss)

### **Consistency**
- ✅ All E2E tests use identical PostgreSQL deployment
- ✅ All E2E tests use identical Redis deployment
- ✅ All E2E tests use identical migration application
- ✅ All E2E tests use identical DataStorage deployment

---

## ⚠️ **Risks and Mitigation**

### **Risk 1: Breaking Parallel Deployment Performance**
**Mitigation**: New function preserves parallel PostgreSQL + Redis deployment

### **Risk 2: NodePort Conflicts**
**Mitigation**: Function accepts custom NodePort parameter

### **Risk 3: Coverage Test Compatibility**
**Mitigation**: Coverage is handled at image build time, not deployment time

### **Risk 4: Service-Specific Requirements**
**Mitigation**: Function is flexible enough to support all current use cases

---

## ✅ **Success Criteria**

### **Phase 2 Complete When**:
- ✅ `DeployDataStorageTestServicesParallel()` function created
- ✅ Gateway E2E tests use consolidated function
- ✅ All hybrid E2E tests use consolidated function
- ✅ HolmesGPT E2E tests use consolidated function with custom port
- ✅ ~190 lines of code eliminated
- ✅ All E2E tests pass without regressions
- ✅ No performance degradation (parallel deployment preserved)

---

## 🎯 **Implementation Checklist**

### **Step 1: Create Consolidated Function** ⏳
- [ ] Add `DeployDataStorageTestServicesParallel()` to `datastorage.go`
- [ ] Add comprehensive documentation
- [ ] Add example usage in comments
- [ ] Verify lint compliance

### **Step 2: Migrate Gateway E2E** ⏳
- [ ] Update `SetupGatewayInfrastructureParallel()`
- [ ] Update `SetupGatewayInfrastructureParallelWithCoverage()`
- [ ] Remove duplicate deployment code
- [ ] Test Gateway E2E suite

### **Step 3: Migrate Other E2E Tests** ⏳
- [ ] Migrate `gateway_e2e_hybrid.go`
- [ ] Migrate `signalprocessing_e2e_hybrid.go`
- [ ] Migrate `workflowexecution_e2e_hybrid.go`
- [ ] Migrate `remediationorchestrator_e2e_hybrid.go`
- [ ] Migrate `holmesgpt_api.go` (with custom NodePort)

### **Step 4: Verification** ⏳
- [ ] Run Gateway E2E tests
- [ ] Run SignalProcessing E2E tests
- [ ] Run WorkflowExecution E2E tests
- [ ] Run RemediationOrchestrator E2E tests
- [ ] Verify no lint errors
- [ ] Verify no performance regressions

---

## 📚 **Related Documentation**

- **Phase 1 Complete**: `TEST_INFRASTRUCTURE_PHASE1_COMPLETE_JAN07.md`
- **Overall Triage**: `TEST_INFRASTRUCTURE_REFACTORING_TRIAGE_JAN07.md`
- **DD-TEST-001**: Port Allocation Strategy
- **DD-TEST-002**: Sequential Startup Pattern

---

**Document Status**: ✅ Analysis Complete - Ready for Implementation
**Next Step**: Create `DeployDataStorageTestServicesParallel()` function
**Estimated Time**: 3-4 hours
**Priority**: P2 - High Impact, Medium Risk

