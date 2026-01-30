# Gateway Per-Test Server Pattern - Regression Analysis
**Date:** January 30, 2026  
**Purpose:** Document why Gateway uses per-test servers and ensure audit store fix doesn't introduce regression

---

## 🎯 Original Design Intent

### **WHY Gateway Creates Per-Test Servers**

#### **Primary Reason: Prometheus Metrics Isolation**
```go
// From: test/integration/gateway/helpers.go (commit 29c8324ec, Oct 28 2025)
// Create isolated Prometheus registry for this test
// This prevents "duplicate metrics collector registration" panics when
// multiple Gateway servers are created in the same test suite
registry := prometheus.NewRegistry()
metricsInstance := metrics.NewMetricsWithRegistry(registry)
```

**Problem Solved:**
- Prometheus metrics collectors must be unique per registry
- Multiple Gateway servers in same test process → duplicate collector registration → **PANIC**
- Solution: Create isolated `prometheus.NewRegistry()` per server instance

**Git History:**
- Commit: `29c8324ec` - "fix(gateway): Add missing imports for Prometheus metrics isolation"
- Commit: `6e5c9e609` - "feat(test): Implement initial 6 Gateway metrics emission tests"

#### **Architecture: Stateless Service vs Controller**

**Gateway (Stateless HTTP Service):**
- Short-lived request/response cycles
- No persistent in-memory state
- Tests need to verify different configurations/timeouts/error conditions
- **Pattern:** Create server → test → destroy → create new server with different config

**Controllers (Stateful Services - WE, NT, RO):**
- Long-running reconciliation loops
- Persistent in-memory state (work queues, caches)
- Tests share ONE controller instance across all tests
- **Pattern:** Start controller once → run all tests → stop controller

---

## 🐛 The Audit Store Problem (Oct 2025 - Jan 2026)

### **Accidental Side Effect of Per-Test Servers**

#### **Timeline:**
1. **Oct 28, 2025:** Per-test servers created for Prometheus metrics isolation
2. **Side Effect:** Each server instance also created its own audit store
3. **Why:** `gateway.NewServerWithK8sClient()` internally calls `createServerWithClients()` which creates audit store
4. **Jan 30, 2026:** Discovered audit stores connect to WRONG DataStorage URL

#### **What Went Wrong:**

```go
// OLD PATTERN (BROKEN)
// File: test/integration/gateway/10_crd_creation_lifecycle_integration_test.go
gwServer, err := gateway.NewServerWithK8sClient(cfg, testLogger, nil, k8sClient)
//                          ↓
//    gateway.NewServerWithK8sClient() calls createServerWithClients()
//                          ↓
//    createServerWithClients() creates NEW audit.NewBufferedStore()
//                          ↓
//    Uses cfg.Infrastructure.DataStorageURL from createGatewayConfig()
//                          ↓
//    createGatewayConfig() calls getDataStorageURL()
//                          ↓
//    getDataStorageURL() checks TEST_DATA_STORAGE_URL env var
//                          ↓
//    Env var NOT SET → fallback to "http://localhost:18090" (WRONG PORT!)
//                          ↓
//    Background flusher connects to non-existent DataStorage
//                          ↓
//    14 audit tests fail: "connection refused"
```

#### **Root Cause:**
- ❌ Per-test **servers** were necessary (Prometheus isolation)
- ❌ Per-test **audit stores** were accidental side effect
- ❌ Each audit store had short lifecycle → background flusher cancelled → events lost
- ❌ Wrong DataStorage URL made failures visible

---

## ✅ The Fix: Separate Concerns

### **Keep:** Per-Test Server Creation (Required)
```go
// REASON: Prometheus metrics isolation
registry := prometheus.NewRegistry()           // ✅ Isolated per test
metricsInstance := metrics.NewMetricsWithRegistry(registry)
```

### **Change:** Use Shared Audit Store (Fix)
```go
// REASON: Continuous background flusher + correct URL
sharedAuditStore, err = audit.NewBufferedStore(
    dsClients.AuditClient,                     // ✅ From suite_test.go Phase 2
    audit.RecommendedConfig("gateway-test"),
    "gateway-test",
    logger,
)

// Pass to ALL servers
gwServer, err := createGatewayServer(cfg, testLogger, k8sClient, sharedAuditStore)
```

---

## 🔬 Regression Analysis

### **Question:** Will shared audit store cause regressions?

#### **Analysis Dimensions:**

### 1. **Prometheus Metrics Isolation** ✅ PRESERVED
```go
// BEFORE FIX:
registry := prometheus.NewRegistry()  // Per test
metricsInstance := metrics.NewMetricsWithRegistry(registry)
gwServer := gateway.NewServerWithK8sClient(cfg, logger, metricsInstance, k8sClient)

// AFTER FIX:
registry := prometheus.NewRegistry()  // ✅ Still per test
metricsInstance := metrics.NewMetricsWithRegistry(registry)
gwServer := createGatewayServer(cfg, logger, k8sClient, sharedAuditStore)
//                                                         ↑ Only audit store is shared
```

**Result:** ✅ **NO REGRESSION** - Metrics still isolated per test

---

### 2. **Test Isolation** ✅ PRESERVED
```go
// Each test creates NEW server with:
// - NEW Prometheus registry (isolated metrics)
// - NEW Gateway instance (isolated processing state)
// - NEW K8s client connection (isolated API calls)
// - SHARED audit store (only audit events shared)
```

**Audit Event Isolation:**
- Each event has unique `correlation_id` (per RemediationRequest)
- Tests query by `correlation_id` → isolated results
- Shared audit store is just a **transport mechanism** (like shared network)

**Result:** ✅ **NO REGRESSION** - Tests still isolated via correlation_id

---

### 3. **Configuration Testing** ✅ PRESERVED
```go
// Config tests DON'T create servers - just test config loading:
cfg, err := config.LoadFromFile(configPath)
err = cfg.Validate()
```

**Result:** ✅ **NO REGRESSION** - Config tests unaffected

---

### 4. **Timeout/Error Testing** ✅ PRESERVED
```go
// Each test creates server with DIFFERENT config:
// Test A: cfg.Processing.Retry.MaxAttempts = 3
// Test B: cfg.Processing.Retry.MaxAttempts = 1
```

**Audit Store Impact:**
- Audit store doesn't affect retry behavior
- Audit store only records events AFTER processing
- Server configuration still isolated per test

**Result:** ✅ **NO REGRESSION** - Server config still per-test

---

### 5. **Parallel Test Execution** ✅ IMPROVED
```go
// BEFORE: Each test → NEW audit store → NEW background flusher → resource contention
// AFTER:  All tests → SHARED audit store → ONE background flusher → better resource usage
```

**Benefits:**
- Fewer goroutines (1 flusher vs N flushers)
- Better connection pooling to DataStorage
- Reduced memory footprint

**Result:** ✅ **IMPROVEMENT** - Better resource utilization

---

### 6. **Audit Event Reliability** ✅ IMPROVED
```go
// BEFORE:
// - Test creates server → audit store created
// - Audit events buffered
// - Test finishes → server destroyed → context cancelled → flusher stopped
// - Buffered events LOST

// AFTER:
// - suite_test.go creates ONE audit store
// - Background flusher runs CONTINUOUSLY
// - Test creates server → uses shared store
// - Test finishes → server destroyed → audit store CONTINUES RUNNING
// - Events reliably flushed
```

**Result:** ✅ **IMPROVEMENT** - Audit events no longer lost

---

## 📊 Comparison Matrix

| Aspect | Before Fix | After Fix | Regression Risk |
|--------|-----------|-----------|----------------|
| **Prometheus Metrics** | ✅ Isolated per test | ✅ Isolated per test | ✅ **NONE** |
| **Gateway Instance** | ✅ New per test | ✅ New per test | ✅ **NONE** |
| **Server Config** | ✅ Different per test | ✅ Different per test | ✅ **NONE** |
| **Test Isolation** | ✅ Via new server | ✅ Via correlation_id | ✅ **NONE** |
| **Audit Store** | ❌ New per test | ✅ Shared (1 per process) | ✅ **NONE** |
| **Background Flusher** | ❌ Cancelled per test | ✅ Continuous | ✅ **NONE** |
| **DataStorage URL** | ❌ Wrong (18090) | ✅ Correct (18091) | ✅ **NONE** |
| **Event Delivery** | ❌ Lost on test end | ✅ Reliably flushed | ✅ **NONE** |
| **Resource Usage** | ❌ N flushers | ✅ 1 flusher | ✅ **IMPROVED** |

---

## 🎯 Conclusion

### **Original Intent:** Per-test servers for Prometheus metrics isolation
### **Accidental Side Effect:** Per-test audit stores with short lifecycle
### **Fix:** Separate concerns - keep per-test servers, share audit store
### **Regression Risk:** ✅ **ZERO** - All isolation benefits preserved

---

## 🔍 Why This Pattern Differs from Controllers

### **Controllers (WE, NT, RO):**
```go
// ONE controller instance for entire suite
reconciler := &WorkflowExecutionReconciler{
    AuditStore: sharedAuditStore,  // ✅ Controller + audit store both shared
}
// ALL tests use SAME controller
```

**Why?** Controllers have persistent state (work queues, rate limiters, cooldowns)

### **Gateway (Stateless Service):**
```go
// NEW server per test (or per test group)
gwServer := createGatewayServer(cfg, logger, k8sClient, sharedAuditStore)
//                               ↑                         ↑
//                        Server isolated          Audit store shared
```

**Why?** Gateway is stateless - safe to create/destroy per test for config isolation

---

## 📚 Key Takeaway

**Gateway's per-test server pattern is CORRECT and NECESSARY.**

**The bug was NOT the pattern itself, but:**
1. ❌ Using wrong DataStorage URL (environment variable fallback issue)
2. ❌ Creating audit stores as side effect (should be explicit dependency injection)
3. ❌ Short-lived audit stores losing buffered events

**The fix preserves the pattern and fixes the bugs.**

---

**Author:** AI Assistant (via Cursor)  
**Branch:** `feature/k8s-sar-user-id-stateless-services`
