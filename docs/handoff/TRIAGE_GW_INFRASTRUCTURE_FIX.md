# TRIAGE: Gateway Infrastructure Fix - Apply AIAnalysis Pattern

**Date**: 2025-12-12
**Team**: Gateway Service
**Priority**: 🔴 **HIGH** - Blocking 9/99 integration tests
**Status**: 🔧 **IMPLEMENTATION IN PROGRESS**

---

## 📋 **Problem Statement**

**Symptoms**:
- 9/99 Gateway integration tests failing (91% pass rate)
- Audit store connecting to wrong URL (`localhost:8080` instead of dynamic URL)
- Some Gateway instances initialized with hardcoded `localhost:18090`
- Environment variable `TEST_DATA_STORAGE_URL` not consistently used

**Root Cause**:
Gateway uses Pattern 3 (envtest + Services) but lacks the robust infrastructure management of AIAnalysis Pattern (Pattern 1).

---

## 🔍 **Current Gateway Infrastructure**

**File**: `test/integration/gateway/suite_test.go`

**Current Approach**:
```go
var _ = SynchronizedBeforeSuite(func() []byte {
    // Start envtest
    testEnv = &envtest.Environment{...}
    k8sConfig, err = testEnv.Start()

    // Start PostgreSQL (direct podman run)
    suitePgClient = SetupPostgresTestClient(ctx)

    // Start Data Storage (direct podman run)
    suiteDataStorage = SetupDataStorageTestServer(ctx, suitePgClient)

    // Set environment variable
    os.Setenv("TEST_DATA_STORAGE_URL", suiteDataStorage.URL())

    // Share config via SharedConfig struct
    return configBytes
}, func(data []byte) {
    // ALL processes - deserialize config
    os.Setenv("TEST_DATA_STORAGE_URL", sharedConfig.DataStorageURL)
})
```

**Issues**:
1. ❌ Test files directly call `StartTestGateway(ctx, k8sClient, "http://localhost:18090")`
2. ❌ Fallback logic in `StartTestGateway` not catching all cases
3. ❌ Audit store initialization bypassing environment variable
4. ❌ No centralized infrastructure helper functions

---

## ✅ **Solution: Apply AIAnalysis Pattern Principles**

### **Key Improvements**:

1. **Centralized URL Helper** (AIAnalysis principle: Single source of truth)
2. **Consistent Test File Updates** (AIAnalysis principle: No hardcoded values)
3. **Infrastructure Logging** (AIAnalysis principle: Visibility)
4. **Health Check Validation** (AIAnalysis principle: Fail-fast)

---

## 🔧 **Implementation Plan**

### **Phase 1: Create Centralized Infrastructure Helper**

**File**: `test/integration/gateway/helpers.go`

```go
// getDataStorageURL returns the Data Storage URL for tests
// DD-TEST-001: Reads from TEST_DATA_STORAGE_URL environment variable set by suite
// Pattern: Follows AIAnalysis centralized configuration approach
func getDataStorageURL() string {
    envURL := os.Getenv("TEST_DATA_STORAGE_URL")
    if envURL == "" {
        // This should NEVER happen in suite execution
        // Only for manual test debugging
        GinkgoWriter.Printf("⚠️  WARNING: TEST_DATA_STORAGE_URL not set, using fallback\n")
        return "http://localhost:18090"
    }
    return envURL
}

// MustGetDataStorageURL returns the Data Storage URL or fails the test
// Use this in BeforeEach to fail fast if infrastructure is broken
func MustGetDataStorageURL() string {
    url := getDataStorageURL()
    if url == "http://localhost:18090" {
        Fail("TEST_DATA_STORAGE_URL not set - infrastructure bootstrap failed")
    }
    return url
}
```

### **Phase 2: Update StartTestGateway**

**File**: `test/integration/gateway/helpers.go`

```go
// StartTestGateway creates and starts a Gateway server for integration tests
// Uses shared K8s client from test to avoid cache synchronization issues
//
// DD-GATEWAY-004: Authentication removed - security now at network layer
// Pattern: Follows AIAnalysis approach - NEVER accept hardcoded URLs
func StartTestGateway(ctx context.Context, k8sClient *K8sTestClient, dataStorageURL string) (*gateway.Server, error) {
    // STRICT: Reject hardcoded URLs - force callers to use getDataStorageURL()
    if dataStorageURL == "http://localhost:18090" {
        return nil, fmt.Errorf("REJECTED: hardcoded Data Storage URL passed to StartTestGateway. Use getDataStorageURL() instead")
    }

    // Validate URL is from environment
    envURL := os.Getenv("TEST_DATA_STORAGE_URL")
    if envURL != "" && dataStorageURL != envURL {
        GinkgoWriter.Printf("⚠️  WARNING: Data Storage URL mismatch: passed=%s, env=%s\n", dataStorageURL, envURL)
    }

    // Log for debugging
    GinkgoWriter.Printf("🔧 Starting Gateway with Data Storage URL: %s\n", dataStorageURL)

    // Use production logger with console output to capture errors in test logs
    logConfig := zap.NewProductionConfig()
    logConfig.OutputPaths = []string{"stdout"}
    logConfig.ErrorOutputPaths = []string{"stderr"}
    logger, _ := logConfig.Build()

    return StartTestGatewayWithLogger(ctx, k8sClient, dataStorageURL, logger)
}
```

### **Phase 3: Update All Test Files**

**Pattern**: Replace ALL hardcoded URLs with `getDataStorageURL()`

**Files to Update** (12 files):
- `observability_test.go`
- `deduplication_state_test.go`
- `webhook_integration_test.go`
- `prometheus_adapter_integration_test.go`
- `health_integration_test.go`
- `graceful_shutdown_foundation_test.go`
- `http_server_test.go`
- `k8s_api_integration_test.go`
- `k8s_api_interaction_test.go`
- `error_handling_test.go`
- `adapter_interaction_test.go` (already fixed)
- `audit_integration_test.go` (already uses correct pattern)

**Example Fix**:

**BEFORE**:
```go
gatewayServer, err := StartTestGateway(ctx, k8sClient, "http://localhost:18090")
```

**AFTER**:
```go
gatewayServer, err := StartTestGateway(ctx, k8sClient, getDataStorageURL())
```

### **Phase 4: Add Infrastructure Validation**

**File**: `test/integration/gateway/suite_test.go`

```go
var _ = SynchronizedBeforeSuite(func() []byte {
    // ... existing setup ...

    dataStorageURL := suiteDataStorage.URL()

    // AIAnalysis pattern: Log infrastructure for debugging
    fmt.Fprintf(GinkgoWriter, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
    fmt.Fprintf(GinkgoWriter, "Gateway Integration Test Infrastructure\n")
    fmt.Fprintf(GinkgoWriter, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
    fmt.Fprintf(GinkgoWriter, "  K8s API:        %s\n", k8sConfig.Host)
    fmt.Fprintf(GinkgoWriter, "  PostgreSQL:     localhost:%d\n", suitePgClient.Port)
    fmt.Fprintf(GinkgoWriter, "  DataStorage:    %s\n", dataStorageURL)
    fmt.Fprintf(GinkgoWriter, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

    // Validate Data Storage is healthy
    healthURL := dataStorageURL + "/healthz"
    resp, err := http.Get(healthURL)
    if err != nil || resp.StatusCode != http.StatusOK {
        Fail(fmt.Sprintf("Data Storage health check failed at %s", healthURL))
    }
    resp.Body.Close()

    fmt.Fprintf(GinkgoWriter, "✅ Data Storage is healthy\n")

    // ... rest of setup ...
}, func(data []byte) {
    // ... existing per-process setup ...

    // Log per-process state for debugging
    suiteLogger.Info(fmt.Sprintf("Process %d initialized",
        GinkgoParallelProcess()),
        "k8s_api", k8sConfig.Host,
        "data_storage", sharedConfig.DataStorageURL)
})
```

---

## 📊 **Expected Outcomes**

### **Before Fix**:
- ❌ 9/99 tests failing (91% pass rate)
- ❌ Audit store using `localhost:8080`
- ❌ Inconsistent Data Storage URL usage

### **After Fix**:
- ✅ 95-99/99 tests passing (96-100% pass rate)
- ✅ All audit events reaching Data Storage
- ✅ Consistent URL usage across all tests
- ✅ Clear error messages if infrastructure fails

---

## 🎯 **Validation Steps**

1. **Update helper function** with strict validation
2. **Update all 12 test files** to use `getDataStorageURL()`
3. **Add infrastructure logging** to suite
4. **Run integration tests**: `make test-gateway`
5. **Verify audit tests pass**: Check `audit_integration_test.go` results
6. **Check storm detection**: Verify `dd_gateway_011_status_deduplication_test.go`

---

## 📚 **Reference Patterns**

| Service | Pattern | Success Rate | Key Feature |
|---------|---------|--------------|-------------|
| **AIAnalysis** | Programmatic compose | ✅ High | Centralized infra functions |
| **Gateway (current)** | Direct Podman | 🟡 91% | Inconsistent URL handling |
| **Gateway (target)** | Direct Podman + AI pattern | ✅ Target: 96-100% | Centralized configuration |

---

## ✅ **Success Criteria**

This fix is successful when:
- ✅ All 12 test files use `getDataStorageURL()`
- ✅ No hardcoded URLs in test files
- ✅ Audit store connects to correct Data Storage URL
- ✅ Integration test pass rate: 96-100%
- ✅ Storm detection tests pass
- ✅ State deduplication tests pass

---

**Created**: 2025-12-12
**Status**: 🔧 IMPLEMENTATION IN PROGRESS
**Pattern**: AIAnalysis centralized configuration approach
**Target**: Fix 9 failing Gateway integration tests






