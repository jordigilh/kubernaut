# Integration Test Refactoring Plan

**Date**: October 28, 2025
**Status**: 🚀 **IN PROGRESS**
**Effort**: 2-3 hours
**Priority**: HIGH (blocks integration test execution)

---

## 🎯 **Objective**

Refactor `test/integration/gateway/helpers.go` to use the new `gateway.NewServer(cfg *ServerConfig, logger *zap.Logger)` API instead of the old 12-parameter constructor.

---

## 🔍 **Root Cause Analysis**

### **Current Problem**
The `StartTestGateway()` function in `helpers.go` is:
1. Importing wrong package: `pkg/contextapi/server` instead of `pkg/gateway`
2. Manually creating all components (adapters, classifier, priority, etc.)
3. Calling old `server.NewServer()` with 12 parameters
4. This API doesn't exist in current `pkg/gateway/server.go`

### **Current Implementation** (lines 208-296)
```go
// WRONG: Imports contextapi server
import "github.com/jordigilh/kubernaut/pkg/contextapi/server"

func StartTestGateway(ctx context.Context, redisClient *RedisTestClient, k8sClient *K8sTestClient) string {
    // Manually creates all components
    adapterRegistry := adapters.NewAdapterRegistry()
    classifier := processing.NewEnvironmentClassifier()
    priorityEngine, _ := processing.NewPriorityEngineWithRego(policyPath, logger)
    pathDecider := processing.NewRemediationPathDecider(logger)
    crdCreator := processing.NewCRDCreator(k8sClient.Client, logger, metricsInstance)
    dedupService := processing.NewDeduplicationServiceWithTTL(redisClient.Client, 5*time.Second, logger, metricsInstance)
    stormDetector := processing.NewStormDetector(redisClient.Client, 0, 0, metricsInstance)
    stormAggregator := processing.NewStormAggregator(redisClient.Client)

    // Calls old API (doesn't exist)
    gatewayServer, err := server.NewServer(
        adapterRegistry,
        classifier,
        priorityEngine,
        pathDecider,
        crdCreator,
        dedupService,
        stormDetector,
        stormAggregator,
        redisClient.Client,
        logger,
        serverConfig,
        metricsRegistry,
    )
}
```

### **Target Implementation** (pkg/gateway/server.go:169)
```go
// CORRECT: Uses ServerConfig
func NewServer(cfg *ServerConfig, logger *zap.Logger) (*Server, error) {
    // Internally creates all components
    // Returns configured server
}
```

---

## 📋 **Refactoring Steps**

### **Step 1: Update Imports** ✅
- Remove: `"github.com/jordigilh/kubernaut/pkg/contextapi/server"`
- Add: `gateway "github.com/jordigilh/kubernaut/pkg/gateway"`

### **Step 2: Refactor StartTestGateway()** ✅
Replace manual component creation with `ServerConfig`:

```go
func StartTestGateway(ctx context.Context, redisClient *RedisTestClient, k8sClient *K8sTestClient) (*gateway.Server, error) {
    logger, _ := zap.NewProduction()

    // Create ServerConfig for tests
    cfg := &gateway.ServerConfig{
        ListenAddr:   ":8080",
        ReadTimeout:  5 * time.Second,
        WriteTimeout: 10 * time.Second,
        IdleTimeout:  120 * time.Second,

        // Rate limiting (lower for tests)
        RateLimitRequestsPerMinute: 20,  // Production: 100
        RateLimitBurst:             5,   // Production: 10

        // Redis configuration
        Redis: redisClient.Client.Options(),

        // Fast TTLs for tests
        DeduplicationTTL:       5 * time.Second,  // Production: 5 minutes
        StormRateThreshold:     2,                // Production: 10
        StormPatternThreshold:  2,                // Production: 5
        StormAggregationWindow: 5 * time.Second,  // Production: 1 minute
        EnvironmentCacheTTL:    5 * time.Second,  // Production: 30 seconds

        // Environment classification
        EnvConfigMapNamespace: "kubernaut-system",
        EnvConfigMapName:      "kubernaut-environment-overrides",
    }

    return gateway.NewServer(cfg, logger)
}
```

### **Step 3: Update Return Type** ✅
- Old: `string` (returns URL)
- New: `(*gateway.Server, error)` (returns server instance)

### **Step 4: Update Test Files** ✅
Update all test files that use `StartTestGateway()`:
- `webhook_integration_test.go`
- `k8s_api_failure_test.go`
- `storm_aggregation_test.go`
- `deduplication_ttl_test.go`
- All other integration tests

**Pattern**:
```go
// OLD
gatewayURL := StartTestGateway(ctx, redisClient, k8sClient)
resp, _ := http.Post(gatewayURL+"/webhook/prometheus", ...)

// NEW
gatewayServer, err := StartTestGateway(ctx, redisClient, k8sClient)
Expect(err).ToNot(HaveOccurred())

// Create test HTTP server
testServer := httptest.NewServer(gatewayServer.Handler())
defer testServer.Close()

resp, _ := http.Post(testServer.URL+"/webhook/prometheus", ...)
```

### **Step 5: Remove StopTestGateway()** ✅
No longer needed - tests will use `defer testServer.Close()`

---

## 📁 **Files to Modify**

### **Primary File**
1. **test/integration/gateway/helpers.go** (CRITICAL)
   - Update imports
   - Refactor `StartTestGateway()`
   - Remove `StopTestGateway()`
   - Update return type

### **Test Files** (60+ tests affected)
2. **test/integration/gateway/webhook_integration_test.go**
3. **test/integration/gateway/k8s_api_failure_test.go**
4. **test/integration/gateway/storm_aggregation_test.go**
5. **test/integration/gateway/deduplication_ttl_test.go**
6. All other `*_test.go` files in `test/integration/gateway/`

---

## 🎯 **Success Criteria**

### **Code Compiles**
- ✅ `helpers.go` compiles without errors
- ✅ All test files compile without errors
- ✅ No undefined references to old API

### **Tests Pass**
- ✅ All integration tests pass
- ✅ No panics or crashes
- ✅ Redis connectivity works
- ✅ K8s API calls work

### **Functionality Preserved**
- ✅ Same test behavior as before
- ✅ Same Redis configuration
- ✅ Same K8s client configuration
- ✅ Same rate limiting behavior

---

## ⚠️ **Risks & Mitigation**

### **Risk 1: Breaking All Integration Tests**
- **Probability**: HIGH (60+ tests affected)
- **Impact**: HIGH (blocks integration test execution)
- **Mitigation**:
  - Refactor `helpers.go` first
  - Test with one simple test file
  - Fix issues before updating all tests

### **Risk 2: Configuration Mismatch**
- **Probability**: MEDIUM (ServerConfig may have different defaults)
- **Impact**: MEDIUM (tests may behave differently)
- **Mitigation**:
  - Document all configuration values
  - Use same values as old implementation
  - Verify behavior matches

### **Risk 3: Missing Components**
- **Probability**: LOW (NewServer creates all components)
- **Impact**: MEDIUM (tests may fail)
- **Mitigation**:
  - Verify NewServer creates all required components
  - Check that adapters are registered
  - Validate metrics are initialized

---

## 📊 **Progress Tracking**

### **Phase 1: Refactor helpers.go** (30 min)
- [ ] Update imports
- [ ] Refactor `StartTestGateway()`
- [ ] Remove `StopTestGateway()`
- [ ] Verify compilation

### **Phase 2: Update One Test File** (15 min)
- [ ] Choose simple test file (e.g., webhook_integration_test.go)
- [ ] Update to use new API
- [ ] Run test and verify it passes
- [ ] Document any issues

### **Phase 3: Update Remaining Test Files** (1-1.5h)
- [ ] Update all `*_test.go` files
- [ ] Run all integration tests
- [ ] Fix any failures
- [ ] Verify all tests pass

### **Phase 4: Cleanup & Documentation** (15 min)
- [ ] Remove old code/comments
- [ ] Update INTEGRATION_TEST_REFACTORING_NEEDED.md
- [ ] Create completion report
- [ ] Commit changes

---

## 🚀 **Implementation Order**

1. **Start**: Refactor `helpers.go` (30 min)
2. **Validate**: Update one test file (15 min)
3. **Scale**: Update all test files (1-1.5h)
4. **Complete**: Cleanup & documentation (15 min)

**Total Estimated Time**: 2-3 hours

---

## 📝 **Notes**

### **Key Differences**
- Old API: Manual component creation
- New API: ServerConfig-based creation
- Old API: Returns URL string
- New API: Returns Server instance

### **Test-Specific Configuration**
- Deduplication TTL: 5s (vs 5min production)
- Storm thresholds: 2 (vs 10 production)
- Rate limit: 20 req/min (vs 100 production)
- Environment cache: 5s (vs 30s production)

### **Preserved Behavior**
- Redis connectivity
- K8s API access
- Adapter registration
- Metrics collection
- Rate limiting

---

**Status**: 🚀 **READY TO IMPLEMENT**
**Next Step**: Start Phase 1 - Refactor helpers.go

