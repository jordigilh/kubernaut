# Integration Test Refactoring Required

**Date**: October 28, 2025
**Status**: 🚨 **BLOCKING** - Integration tests incompatible with current server implementation
**Severity**: HIGH - 60+ integration tests need refactoring

---

## 🔍 **Root Cause**

Integration tests were written for an old `NewServer` API that took 12+ individual component parameters:

```go
// OLD API (what integration tests expect)
func NewServer(
    adapterRegistry *adapters.AdapterRegistry,
    classifier *processing.EnvironmentClassifier,
    priorityEngine *processing.PriorityEngine,
    pathDecider *processing.RemediationPathDecider,
    crdCreator *processing.CRDCreator,
    dedupService *processing.DeduplicationService,
    stormDetector *processing.StormDetector,
    stormAggregator *processing.StormAggregator,
    redisClient *goredis.Client,
    logger *zap.Logger,
    config *Config,
    metricsRegistry *prometheus.Registry,
) (*Server, error)
```

Current implementation uses a simplified API:

```go
// CURRENT API (pkg/gateway/server.go:168)
func NewServer(cfg *ServerConfig, logger *zap.Logger) (*Server, error)
```

**Impact**: All integration test helpers in `test/integration/gateway/helpers.go` are broken.

---

## 📊 **Files Affected**

### Corrupted Files (Fixed)
- ✅ `test/integration/gateway/security_suite_setup.go` - Fixed (250 lines, was 515 lines with 9x duplication)
- ✅ `test/integration/gateway/deduplication_ttl_test.go` - Fixed (292 lines, was 332 lines with duplication)

### Files Needing Refactoring
- ❌ `test/integration/gateway/helpers.go` - **BLOCKING** - Uses old NewServer API
- ❌ `test/integration/gateway/webhook_integration_test.go` - Uses helpers.go
- ❌ `test/integration/gateway/k8s_api_failure_test.go` - Uses helpers.go
- ❌ `test/integration/gateway/storm_aggregation_test.go` - Uses helpers.go
- ❌ `test/integration/gateway/deduplication_ttl_test.go` - Uses helpers.go
- ❌ All other integration tests using `StartTestGateway()` helper

---

## 🔧 **Required Changes**

### Option A: Refactor helpers.go to use new API (RECOMMENDED)

**Effort**: 2-3 hours
**Risk**: Medium - need to understand new ServerConfig structure

**Changes**:
1. Update `StartTestGateway()` to create `ServerConfig` instead of individual components
2. Remove manual component creation (NewAdapterRegistry, NewEnvironmentClassifier, etc.)
3. Update all test files to use new helper signature
4. Verify all 60+ integration tests still pass

**Example**:
```go
func StartTestGateway(redisClient *RedisClient, k8sClient *K8sClient) (*server.Server, error) {
    logger, _ := zap.NewProduction()

    cfg := &gateway.ServerConfig{
        ListenAddr:                 ":8080",
        ReadTimeout:                5 * time.Second,
        WriteTimeout:               10 * time.Second,
        RateLimitRequestsPerMinute: 20, // Lower for tests
        RateLimitBurst:             5,
        Redis:                      redisClient.Options(),
        DeduplicationTTL:           5 * time.Second, // Fast for tests
    }

    return gateway.NewServer(cfg, logger)
}
```

### Option B: Create test-specific constructor (WORKAROUND)

**Effort**: 1 hour
**Risk**: Low - but creates technical debt

**Changes**:
1. Add `NewServerForTesting()` function in `pkg/gateway/server.go`
2. Keep old signature for backward compatibility with tests
3. Mark as deprecated for future removal

---

## 📋 **Compilation Errors**

```
test/integration/gateway/helpers.go:216:21: not enough arguments in call to adapters.NewAdapterRegistry
        have ()
        want (*zap.Logger)
test/integration/gateway/helpers.go:217:16: not enough arguments in call to processing.NewEnvironmentClassifier
        have ()
        want ("sigs.k8s.io/controller-runtime/pkg/client".Client, *zap.Logger)
test/integration/gateway/helpers.go:231:21: undefined: gatewayMetrics
test/integration/gateway/helpers.go:231:59: undefined: metricsRegistry
test/integration/gateway/helpers.go:256:3: unknown field RateLimit in struct literal of type server.Config
test/integration/gateway/helpers.go:257:3: unknown field RateLimitWindow in struct literal of type server.Config
test/integration/gateway/helpers.go:276:3: too many arguments in call to server.NewServer
```

---

## 🎯 **Recommendation**

**Proceed with Option A** - Refactor helpers.go to use new API

**Rationale**:
1. Aligns tests with current implementation
2. Removes technical debt
3. Ensures tests validate actual production code path
4. No deprecated code to maintain

**Timeline**:
- Refactor helpers.go: 1 hour
- Update test files: 1 hour
- Verify all tests pass: 30 min
- **Total**: 2.5 hours

---

## ✅ **Next Steps**

1. Refactor `test/integration/gateway/helpers.go` to use `gateway.NewServer(cfg, logger)`
2. Update all integration tests to use new helper signature
3. Run full integration test suite to verify
4. Commit fixed integration tests

**Status**: Ready to proceed with refactoring

