# Gateway Standardization Design Triage
**Date:** January 30, 2026  
**Purpose:** Review standardization implementation vs. design document requirements

---

## 🎯 **Standardization Document Requirements**

**Reference:** `docs/handoff/INT_AUDIT_STORE_STANDARDIZED_PATTERN.md`

### **5 Required Fixes:**

1. ✅ **Fix 1:** Add `GatewayIntegrationDataStoragePort` constant
2. ❌ **Fix 2:** DELETE `getDataStorageURL()` function entirely
3. ✅ **Fix 3:** Use `integration.NewAuthenticatedDataStorageClients()`
4. ⚠️ **Fix 4:** Update 21 test files to use `createGatewayServer(..., sharedAuditStore)`
5. ❌ **Fix 5:** Set `DataStorageURL: ""` (empty) in `createGatewayConfig("")`

---

## 📊 **Implementation Status**

### **✅ Fix 1: Infrastructure Constant - COMPLETE**
**Status:** ✅ Already exists in `test/infrastructure/gateway_e2e.go`

```go
const (
    GatewayIntegrationDataStoragePort = 18091
)
```

**No changes needed.**

---

### **❌ Fix 2: Delete getDataStorageURL() - INCOMPLETE**

**Current Implementation (WRONG):**
```go
// File: test/integration/gateway/helpers.go line 199-202
func getDataStorageURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", infrastructure.GatewayIntegrationDataStoragePort)
}
```

**Required:**
```diff
- func getDataStorageURL() string {
-     return fmt.Sprintf("http://127.0.0.1:%d", infrastructure.GatewayIntegrationDataStoragePort)
- }
```

**Rationale:**
- Non-standard pattern (other services don't have this helper)
- Encourages indirect URL construction
- Should be removed entirely

---

### **✅ Fix 3: Standard Client Creation - COMPLETE**

**Implementation:**
```go
// File: test/integration/gateway/suite_test.go Phase 2
dataStorageURL := fmt.Sprintf("http://127.0.0.1:%d", infrastructure.GatewayIntegrationDataStoragePort)
dsClients := integration.NewAuthenticatedDataStorageClients(
    dataStorageURL,
    saToken,
    5*time.Second,
)
dsClient = dsClients.AuditClient // ✅ For backward compatibility
```

**Status:** ✅ Correctly uses standard helper

---

### **⚠️ Fix 4: Update Test Files - PARTIALLY CORRECT**

**Analysis:**

**Tests USING shared audit store (46+ tests):** ✅ **CORRECT**
```go
// Pattern: Explicit DataStorage URL + createGatewayServer with sharedAuditStore
gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient, sharedAuditStore)
```

**Files:**
- `audit_emission_integration_test.go` (16 occurrences)
- `metrics_emission_integration_test.go` (15 occurrences)
- `error_handling_integration_test.go` (3 occurrences)
- `custom_severity_integration_test.go` (5 occurrences)
- `adapters_integration_test.go` (3 occurrences)
- `34_status_deduplication_integration_test.go` (1 occurrence)

**Tests NOT using shared audit store (6 tests):** ⚠️ **DESIGN QUESTION**
```go
// Pattern: Empty DataStorage URL + gateway.NewServerWithK8sClient (NO audit store)
cfg := createGatewayConfig("") // Comment: "No DataStorage for this test"
gwServer, err = gateway.NewServerWithK8sClient(cfg, testLogger, nil, k8sClient)
```

**Files:**
1. `10_crd_creation_lifecycle_integration_test.go` - line 112, 115
2. `21_crd_lifecycle_integration_test.go` - line 89 (comment only)
3. `11_fingerprint_stability_integration_test.go` - line 92 (comment only)
4. `06_concurrent_alerts_integration_test.go` - line 95 (comment only)
5. `05_multi_namespace_isolation_integration_test.go` - line 107 (comment only)
6. `02_state_based_deduplication_integration_test.go` - line 92 (comment only)

---

### **❌ Fix 5: Empty DataStorageURL - INCOMPLETE**

**Current Implementation (WRONG):**
```go
// File: test/integration/gateway/helpers.go line 1308-1311
func createGatewayConfig(dataStorageURL string) *config.ServerConfig {
    // Integration tests use real DataStorage from standardized infrastructure constant
    if dataStorageURL == "" {
        dataStorageURL = getDataStorageURL()  // ← FILLS IN URL EVEN WHEN EMPTY
    }
    return &config.ServerConfig{
        Infrastructure: config.InfrastructureSettings{
            DataStorageURL: dataStorageURL,
        },
        // ...
    }
}
```

**Required:**
```go
func createGatewayConfig(dataStorageURL string) *config.ServerConfig {
    // DON'T fill in empty URLs - respect caller's intent
    return &config.ServerConfig{
        Infrastructure: config.InfrastructureSettings{
            DataStorageURL: dataStorageURL,  // ← Can be "" which is valid
        },
        // ...
    }
}
```

**Impact:**
- 6 tests pass `createGatewayConfig("")` expecting NO DataStorage
- Current code fills in URL anyway → creates audit store → tries to connect
- Should respect empty string → skip audit store creation

---

## 🔍 **Design Question: The 6 "No Audit" Tests**

### **Current State:**
These 6 tests explicitly say "No DataStorage for this test":
- CRD lifecycle tests
- Fingerprint stability tests
- Deduplication tests
- Concurrent alerts tests
- Multi-namespace isolation tests

### **Why No Audit?**
These tests focus on:
- **Core business logic:** CRD creation, fingerprinting, deduplication
- **NOT audit emission:** They don't verify audit events
- **Faster execution:** Skip DataStorage dependency

### **Current Bug:**
```go
createGatewayConfig("")  // Intent: No DataStorage
  ↓
getDataStorageURL() fills in "http://127.0.0.1:18091"
  ↓
gateway.NewServerWithK8sClient() creates audit store
  ↓
Audit store tries to connect to DataStorage
  ↓
If connection works: Unnecessary overhead
If connection fails: Test pollution/errors
```

### **Correct Behavior:**
```go
createGatewayConfig("")  // Intent: No DataStorage
  ↓
DataStorageURL remains ""
  ↓
gateway.NewServerWithK8sClient() skips audit store creation
  ↓
createServerWithClients() checks: if cfg.Infrastructure.DataStorageURL != "" { ... }
  ↓
auditStore = nil (valid for tests that don't emit audit events)
```

---

## 🛠️ **Required Design Fixes**

### **Fix 2 (Final): Delete getDataStorageURL()**
```diff
// File: test/integration/gateway/helpers.go
- // getDataStorageURL returns the standardized DataStorage URL for Gateway integration tests
- // STANDARDIZED PATTERN: All services use direct URL construction from infrastructure constants
- func getDataStorageURL() string {
-     return fmt.Sprintf("http://127.0.0.1:%d", infrastructure.GatewayIntegrationDataStoragePort)
- }
```

**Callers:** NONE (only called from `createGatewayConfig` which we're fixing)

---

### **Fix 5 (Final): Respect Empty DataStorageURL**
```diff
// File: test/integration/gateway/helpers.go
  func createGatewayConfig(dataStorageURL string) *config.ServerConfig {
-     // Integration tests use real DataStorage from standardized infrastructure constant
-     if dataStorageURL == "" {
-         dataStorageURL = getDataStorageURL()
-     }
  
      return &config.ServerConfig{
          Server: config.ServerSettings{
              ListenAddr: ":0", // Random port (we don't use HTTP in integration tests)
          },
          Infrastructure: config.InfrastructureSettings{
              DataStorageURL: dataStorageURL,  // ← Respects "" (empty) for tests without audit
          },
          Processing: config.ProcessingSettings{
              Retry: config.DefaultRetrySettings(), // Enable K8s API retry (3 attempts)
          },
          // Middleware uses defaults
      }
  }
```

---

### **Fix 4 (Clarification): Test Files Are Mostly Correct**

**Status:** ⚠️ **MIXED - Depends on test intent**

**46+ Tests WITH Audit:** ✅ **CORRECT PATTERN**
```go
// Tests that EMIT audit events (audit_emission, metrics, error_handling, etc.)
gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient, sharedAuditStore)
```

**6 Tests WITHOUT Audit:** ✅ **INTENTIONAL DESIGN**
```go
// Tests that DON'T emit audit events (CRD lifecycle, fingerprinting, etc.)
cfg := createGatewayConfig("") // No DataStorage for this test
gwServer, err = gateway.NewServerWithK8sClient(cfg, testLogger, nil, k8sClient)
```

**Rationale:**
- `createGatewayServer()` requires `sharedAuditStore` parameter
- If test doesn't need audit, passing `nil` audit store makes sense
- But `gateway.NewServerWithK8sClient()` doesn't accept audit store parameter
- So it's a different constructor for "no audit" tests

**Action:** ✅ **NO CHANGE NEEDED** (after Fixes 2 & 5 are complete)

---

## 📋 **Verification Checklist**

After implementing Fixes 2 & 5:

### **Tests WITH Audit (46+ tests):**
- [x] Use explicit DataStorage URL: `fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort)`
- [x] Call `createGatewayServer(..., sharedAuditStore)`
- [x] Audit events emitted to shared store
- [x] Background flusher runs continuously

### **Tests WITHOUT Audit (6 tests):**
- [ ] Pass empty URL: `createGatewayConfig("")`
- [ ] `DataStorageURL` remains empty (NOT filled in)
- [ ] Call `gateway.NewServerWithK8sClient()` (no audit store parameter)
- [ ] `createServerWithClients()` skips audit store creation (`auditStore = nil`)
- [ ] No connection attempts to DataStorage
- [ ] Tests run faster (no audit overhead)

---

## 🎯 **Design Validation**

### **Q: Is it OK for some tests to skip audit?**
**A:** ✅ YES

**Rationale:**
- `createServerWithClients()` already handles `cfg.Infrastructure.DataStorageURL == ""` case
- Sets `auditStore = nil` which is valid
- Audit emission methods check `if s.auditStore != nil` before emitting
- Tests that focus on CRD logic don't need audit overhead

### **Q: Should ALL tests use `createGatewayServer(..., sharedAuditStore)`?**
**A:** ❌ NO (current mix is intentional)

**Rationale:**
- **Tests WITH audit:** Must use `createGatewayServer(..., sharedAuditStore)` for shared flusher
- **Tests WITHOUT audit:** Can use `gateway.NewServerWithK8sClient(..., nil)` for no audit overhead

---

## 🚀 **Implementation Plan**

### **Step 1: Delete getDataStorageURL() (Fix 2)**
```bash
# File: test/integration/gateway/helpers.go
# Delete lines 199-202
```

### **Step 2: Fix createGatewayConfig() (Fix 5)**
```bash
# File: test/integration/gateway/helpers.go
# Remove lines 1310-1312 (the if statement that fills empty URLs)
```

### **Step 3: Verify Compilation**
```bash
go build ./test/integration/gateway
```

### **Step 4: Run Tests**
```bash
make test-integration-gateway
```

---

## 📊 **Expected Impact**

### **46+ Tests WITH Audit:**
- ✅ No change (already correct)
- ✅ Use shared audit store
- ✅ Audit events reliably flushed

### **6 Tests WITHOUT Audit:**
- ✅ NOW CORRECT: `DataStorageURL` remains empty
- ✅ No audit store created
- ✅ No connection attempts to DataStorage
- ✅ Tests run faster

---

## ✅ **Conclusion**

**Current Standardization:** 3/5 fixes complete (60%)

**Remaining Work:** 2 small fixes (Delete 1 function, Remove 3 lines)

**Design Validation:** ✅ Test file pattern is CORRECT (mixed use is intentional)

**Auth Failures:** Likely unrelated to these fixes (need separate investigation)

---

**Next Steps:**
1. Implement Fixes 2 & 5 (10 minutes)
2. Verify compilation (2 minutes)
3. Rerun Gateway integration tests
4. Investigate 401 auth errors (if they persist)

---

**Author:** AI Assistant (via Cursor)  
**Branch:** `feature/k8s-sar-user-id-stateless-services`
