# Gateway Integration Test Architecture Audit - Jan 15, 2026

## 🚨 **CRITICAL DISCOVERY: Test Plan Assumptions vs Reality**

### **Executive Summary**

**Problem**: The `GW_INTEGRATION_TEST_PLAN_V1.0.md` assumes Gateway integration tests have access to a **real DataStorage client** running in a Podman container. **This assumption is INCORRECT**.

**Current Reality**: Gateway integration tests currently:
- ✅ Have `k8sClient` (envtest - in-memory K8s API)
- ✅ Have `logger` (Ginkgo writer)
- ❌ **DO NOT** have DataStorage client
- ❌ **DO NOT** have audit infrastructure
- ❌ **DO NOT** connect to Podman containers

### **Impact on Test Plan**

The test plan's `BeforeEach` blocks are **architecturally incorrect** for the current Gateway integration test suite:

```go
// ❌ ASSUMED (in test plan) - NOT IMPLEMENTED
BeforeEach(func() {
    dsClient = suite.GetDataStorageClient()  // ← DOES NOT EXIST
    gateway = gateway.NewService(dsClient, k8sClient, logger)
})
```

```go
// ✅ ACTUAL (current Gateway integration tests)
BeforeEach(func() {
    ctx = context.Background()
    zapLogger := zap.NewNop()
    logger = zapr.NewLogger(zapLogger)
    
    // Use mock K8s client for error injection
    failingK8sClient = &ErrorInjectableK8sClient{
        failCreate: true,
        errorMsg:   "connection refused",
    }
})
```

---

## 📊 **Architecture Comparison: Gateway vs Other Services**

### **AIAnalysis Integration Tests (HAS DataStorage)**

```go
// File: test/integration/aianalysis/suite_test.go
var (
    auditStore audit.AuditStore  // ← Has audit infrastructure
)

// Phase 1: Infrastructure startup (SynchronizedBeforeSuite)
// - PostgreSQL (port 15438)
// - Redis (port 16384)
// - Data Storage API (port 18095)  ← Shared DataStorage service
// - Mock LLM Service (port 18141)
// - HolmesGPT API (port 18120)

// Phase 2: Per-process setup
auditMockTransport := testauth.NewMockUserTransport(
    fmt.Sprintf("test-aianalysis@integration.test-p%d", processNum),
)
dsClient, err := audit.NewOpenAPIClientAdapterWithTransport(
    "http://127.0.0.1:18095",  // ← Connects to real DataStorage
    5*time.Second,
    auditMockTransport,
)

auditStore, err = audit.NewBufferedStore(dsClient, auditConfig, "aianalysis", auditLogger)
```

**Key Pattern**:
- ✅ **Phase 1**: Start shared infrastructure (PostgreSQL + DataStorage in Podman)
- ✅ **Phase 2**: Each process creates its own `dsClient` connecting to shared DataStorage
- ✅ **Parallel-safe**: Uses correlation IDs and authentication per process

### **Gateway Integration Tests (NO DataStorage)**

```go
// File: test/integration/gateway/suite_test.go
var (
    ctx       context.Context
    k8sClient client.Client  // ← Only envtest K8s client
    logger    logr.Logger
    testEnv   *envtest.Environment
)

// NO Phase 1 infrastructure startup
// NO DataStorage
// NO PostgreSQL
// NO audit infrastructure

var _ = BeforeSuite(func() {
    ctx, cancel = context.WithCancel(context.Background())
    
    // Only envtest (in-memory K8s API)
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{"../../../config/crd/bases"},
    }
    
    k8sConfig, err = testEnv.Start()
    k8sClient, err = client.New(k8sConfig, client.Options{Scheme: scheme})
})
```

**Key Pattern**:
- ✅ Only envtest (in-memory K8s API server)
- ❌ NO shared infrastructure phase
- ❌ NO DataStorage client
- ❌ NO audit store
- ❌ NO Podman containers

### **Current Gateway Integration Test Example**

```go
// File: test/integration/gateway/29_k8s_api_failure_integration_test.go

// Uses mock K8s client for error injection
type ErrorInjectableK8sClient struct {
    client.Client
    failCreate bool
    errorMsg   string
}

var _ = Describe("BR-GATEWAY-019: K8s API Failure Handling", func() {
    var (
        ctx              context.Context
        crdCreator       *processing.CRDCreator
        logger           logr.Logger
        failingK8sClient *ErrorInjectableK8sClient  // ← MOCK K8s client
        testSignal       *types.NormalizedSignal
    )

    BeforeEach(func() {
        ctx = context.Background()
        zapLogger := zap.NewNop()
        logger = zapr.NewLogger(zapLogger)

        // ✅ CURRENT PATTERN: Mock K8s client for error injection
        failingK8sClient = &ErrorInjectableK8sClient{
            failCreate: true,
            errorMsg:   "connection refused",
        }
    })
})
```

**Key Pattern**:
- ✅ Uses mock K8s clients (`ErrorInjectableK8sClient`)
- ✅ No external dependencies
- ✅ Self-contained tests
- ❌ NO audit event validation (no DataStorage)
- ❌ NO metrics emission validation (no real infrastructure)

---

## 🎯 **Decision Required: Three Options**

### **Option A: Add DataStorage Infrastructure to Gateway Integration Tests**

**What**: Upgrade Gateway integration test suite to match AIAnalysis pattern

**Changes Required**:
1. Add `SynchronizedBeforeSuite` with Phase 1 (infrastructure) and Phase 2 (per-process)
2. Start PostgreSQL + DataStorage in Podman (shared across processes)
3. Create `dsClient` per process connecting to shared DataStorage
4. Update all tests to use real audit emission and validation
5. Implement correlation ID filtering for parallel execution

**Pros**:
- ✅ Enables audit event validation (high business value)
- ✅ Enables metrics emission validation
- ✅ Tests real integration with DataStorage
- ✅ Aligns with user's explicit requirement ("integration tests run with DS service in a container with podman")

**Cons**:
- ⏱️ Requires infrastructure setup refactoring (~2-4 hours)
- ⏱️ Slower test execution (infrastructure startup overhead)
- 🔧 More complex test maintenance

**Effort**: Medium (2-4 hours for infrastructure setup + test updates)

---

### **Option B: Keep Current Architecture, Update Test Plan**

**What**: Accept that Gateway integration tests are "lightweight integration tests" without DataStorage

**Changes Required**:
1. Update `GW_INTEGRATION_TEST_PLAN_V1.0.md` to use mock audit stores
2. Remove all `dsClient = suite.GetDataStorageClient()` references
3. Focus tests on business logic without audit/metrics validation
4. Document this architectural decision

**Pros**:
- ⏱️ Fast execution (no infrastructure overhead)
- ✅ Simple test maintenance
- ✅ Aligns with current implementation

**Cons**:
- ❌ Cannot validate audit events (violates SOC2 compliance testing)
- ❌ Cannot validate metrics emission
- ❌ Contradicts user's explicit requirement ("integration tests run with DS service in a container with podman")
- ❌ Lower business value (no end-to-end validation)

**Effort**: Low (1-2 hours to update test plan)

---

### **Option C: Split Test Plan into Two Tiers**

**What**: Create separate test plans for "Integration-Light" (current) and "Integration-Full" (with DataStorage)

**Changes Required**:
1. Rename current 22 tests to "Integration-Light" (no DataStorage)
2. Keep current mock-based patterns for these tests
3. Create new "Integration-Full" suite with DataStorage infrastructure
4. Move audit/metrics tests to "Integration-Full"
5. Keep business logic tests in "Integration-Light"

**Pros**:
- ✅ Preserves current fast tests
- ✅ Adds high-value audit/metrics tests
- ✅ Clear separation of concerns
- ✅ Gradual migration path

**Cons**:
- 🔧 More complex test structure (two tiers)
- ⏱️ Requires both infrastructure setup AND documentation split
- 📊 Coverage calculation becomes more complex

**Effort**: High (4-6 hours for infrastructure + test plan split)

---

## 📋 **Current Gateway Integration Test Suite**

### **Existing Tests (22 tests)**
```bash
$ ls test/integration/gateway/
suite_test.go
29_k8s_api_failure_integration_test.go  # 1 test
```

**Test Architecture**:
- Uses `ErrorInjectableK8sClient` mock
- No DataStorage dependency
- No audit event validation
- No metrics emission validation
- Focuses on business logic error handling

---

## 💡 **Recommendation**

### **Recommended: Option A (Add DataStorage Infrastructure)**

**Rationale**:
1. **User's Explicit Requirement**: "integration tests run with DS service in a container with podman. No mocks allowed in GW integration tests."
2. **SOC2 Compliance**: Audit event validation is mandatory for compliance
3. **Business Value**: Audit and metrics scenarios identified in gap analysis are high-value
4. **Consistency**: Aligns with other services (AIAnalysis, SignalProcessing)
5. **Coverage Target**: Need >50% integration coverage - current 30.1% is critically low

**Implementation Plan**:
1. **Week 1, Day 1**: Add infrastructure setup to Gateway integration suite
   - Copy `SynchronizedBeforeSuite` pattern from AIAnalysis
   - Start PostgreSQL + DataStorage in Phase 1
   - Create per-process `dsClient` in Phase 2
   - Verify parallel execution works

2. **Week 1, Days 2-3**: Implement first 7 audit tests (Scenario 1.1-1.4)
   - Use real DataStorage client
   - Use correlation ID filtering
   - Use OpenAPI constants
   - Verify audit events in DataStorage

3. **Week 1, Days 4-5**: Implement 14 metrics tests (Scenario 2.1-2.3)
   - Use real Prometheus registry
   - Use real K8s client
   - Verify metrics emission

4. **Week 2-3**: Continue with remaining scenarios per test plan

---

## 🔍 **Critical Questions for User**

1. **Confirm Requirement**: Does Gateway integration test suite MUST have DataStorage infrastructure?
2. **Timeline**: Is 2-4 hour infrastructure setup acceptable before starting test implementation?
3. **Alternative**: If not, should we accept Gateway as an exception with "lightweight integration tests"?

---

## 📚 **References**

- **Authoritative**: User feedback Jan 15, 2026 - "integration tests run with DS service in a container with podman. No mocks allowed in GW integration tests."
- **Pattern Source**: `test/integration/aianalysis/suite_test.go` (lines 136-450)
- **Current Gateway**: `test/integration/gateway/suite_test.go` (lines 85-152)
- **Example Test**: `test/integration/gateway/29_k8s_api_failure_integration_test.go`
- **Testing Strategy**: `03-testing-strategy.mdc` (>50% integration coverage required)

---

**Document Status**: ✅ Active  
**Created**: 2026-01-15  
**Priority**: 🚨 CRITICAL (blocks test plan implementation)  
**Decision Required**: User must choose Option A, B, or C before proceeding
