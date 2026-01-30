# Integration Test Audit Store Standardized Pattern
**Date:** January 30, 2026  
**Purpose:** Document standardized pattern for audit store setup across all INT test services

---

## 🎯 Standardized Pattern (5 Services)

### **Services Using Standard Pattern:**
- WorkflowExecution (WE)
- Notification (NT)
- RemediationOrchestrator (RO)
- SignalProcessing (SP)
- AIAnalysis (AA)

### **Pattern Components:**

#### 1. **Port Constants** (in `test/infrastructure/<service>_integration.go`)
```go
const (
    <Service>IntegrationDataStoragePort = 180XX  // Unique port per service
)
```

**Examples:**
- `WEIntegrationDataStoragePort = 18097` (WorkflowExecution)
- `NTIntegrationDataStoragePort = 18096` (Notification)
- `SignalProcessingIntegrationDataStoragePort = 18094`
- AIAnalysis: Hardcoded `18095` (no constant)

#### 2. **DataStorage URL Construction** (in `suite_test.go` Phase 2)
```go
// PATTERN: Direct URL construction using infrastructure constant
dataStorageURL := fmt.Sprintf("http://127.0.0.1:%d", infrastructure.<Service>IntegrationDataStoragePort)

// OR with env var fallback (Notification only):
dataStorageURL := os.Getenv("DATA_STORAGE_URL")
if dataStorageURL == "" {
    dataStorageURL = fmt.Sprintf("http://127.0.0.1:%d", infrastructure.NTIntegrationDataStoragePort)
}
```

**Key Insight:** ✅ **NO services use `TEST_DATA_STORAGE_URL` environment variable**

#### 3. **Authenticated Client Creation** (in `suite_test.go` Phase 2)
```go
// PATTERN: Use centralized helper function
dsClients = integration.NewAuthenticatedDataStorageClients(
    dataStorageURL,
    token,                     // From Phase 1
    5*time.Second,
)
GinkgoWriter.Println("✅ Authenticated DataStorage clients created")
```

#### 4. **Shared Audit Store Creation** (in `suite_test.go` Phase 2)
```go
// PATTERN: ONE audit store per process, shared across all tests
auditStore, err = audit.NewBufferedStore(
    dsClients.AuditClient,     // Authenticated client
    audit.DefaultConfig(),     // Or audit.RecommendedConfig("<service>")
    "<service>-test",
    logger,
)
Expect(err).ToNot(HaveOccurred(), "Audit store creation must succeed")
GinkgoWriter.Println("✅ Shared audit store created")
```

#### 5. **Controller/Service Creation** (in `suite_test.go` Phase 2 or `BeforeEach`)
```go
// PATTERN: Pass sharedAuditStore to controller/service
reconciler := &<service>.Reconciler{
    Client:      k8sClient,
    AuditStore:  auditStore,    // ← Shared across all tests
    // ... other dependencies
}
```

---

## ❌ Gateway Deviation from Standard Pattern

### **Current Gateway Pattern (BROKEN):**

#### 1. **Port Constant** (in `test/integration/gateway/suite_test.go`)
```go
const (
    gatewayDataStoragePort = 18091  // ❌ LOCAL constant instead of infrastructure constant
)
```

#### 2. **DataStorage URL** (in `test/integration/gateway/helpers.go`)
```go
// ❌ DEVIATION: Uses env var with WRONG fallback port
func getDataStorageURL() string {
    envURL := os.Getenv("TEST_DATA_STORAGE_URL")  // ❌ Different env var name
    if envURL == "" {
        return "http://localhost:18090"  // ❌ WRONG PORT (should be 18091)
    }
    return envURL
}
```

#### 3. **Authenticated Client Creation** (in `suite_test.go` Phase 2)
```go
// ❌ DEVIATION: Manual client creation instead of helper
dsClient, err = audit.NewOpenAPIClientAdapterWithTransport(
    fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort),
    5*time.Second,
    authTransport,
)
```

**Issue:** Creates only ONE client (audit), missing OpenAPI client pattern

#### 4. **Shared Audit Store Creation** (in `suite_test.go` Phase 2)
```go
// ✅ CORRECT: Creates shared audit store (recently added)
sharedAuditStore, err = audit.NewBufferedStore(dsClient, auditConfig, "gateway-test", logger)
```

#### 5. **Test-Level Server Creation** (in individual test files)
```go
// ❌ DEVIATION: 21 tests use OLD constructor that creates NEW audit store
gwServer, err := gateway.NewServerWithK8sClient(cfg, testLogger, nil, k8sClient)
//                                                                  ↑
//                                               nil = NO audit store injection
//                                               → Server creates its OWN audit store
//                                               → Uses wrong DataStorage URL (getDataStorageURL())
```

**Impact:** Each test creates a NEW audit store pointing to WRONG URL (`http://localhost:18090`)

---

## 🔧 Required Fixes for Gateway Standardization

### **Fix 1: Move Port Constant to Infrastructure**
```go
// File: test/infrastructure/gateway_integration.go (NEW or UPDATE existing gateway_e2e.go)
const (
    GatewayIntegrationDataStoragePort = 18091
)
```

### **Fix 2: Remove `TEST_DATA_STORAGE_URL` Pattern**
```go
// File: test/integration/gateway/helpers.go
// DELETE getDataStorageURL() function entirely
```

### **Fix 3: Use Standard Client Creation**
```go
// File: test/integration/gateway/suite_test.go Phase 2
// REPLACE manual client creation with:
dsClients := integration.NewAuthenticatedDataStorageClients(
    fmt.Sprintf("http://127.0.0.1:%d", infrastructure.GatewayIntegrationDataStoragePort),
    saToken,
    5*time.Second,
)
GinkgoWriter.Println("✅ Authenticated DataStorage clients created")

// Update global variables:
// var dsClient audit.DataStorageClient  // OLD
var dsClients *integration.AuthenticatedDataStorageClients  // NEW

// Use dsClients.AuditClient for audit store
sharedAuditStore, err = audit.NewBufferedStore(dsClients.AuditClient, auditConfig, "gateway-test", logger)
```

### **Fix 4: Update All Test Files**
```bash
# Replace 21 occurrences of:
gateway.NewServerWithK8sClient(cfg, testLogger, nil, k8sClient)

# With:
createGatewayServer(cfg, testLogger, k8sClient, sharedAuditStore)
```

**Files Affected:**
- `test/integration/gateway/10_crd_creation_lifecycle_integration_test.go`
- `test/integration/gateway/21_crd_lifecycle_integration_test.go`
- Plus 19 other test files (identified via grep)

### **Fix 5: Update `createGatewayConfig()`**
```go
// File: test/integration/gateway/helpers.go
func createGatewayConfig(dataStorageURL string) *config.ServerConfig {
    // REMOVE getDataStorageURL() call
    // Infrastructure URL is ONLY passed via sharedAuditStore
    // Gateway server MUST NOT create its own audit store in tests
    return &config.ServerConfig{
        Server: config.ServerSettings{
            ListenAddr: ":0",
        },
        Infrastructure: config.InfrastructureSettings{
            DataStorageURL: "", // ← Empty - audit store injected via NewServerForTesting
        },
        Processing: config.ProcessingSettings{
            Retry: config.DefaultRetrySettings(),
        },
    }
}
```

---

## 📊 Standardization Benefits

### **Before (Gateway Deviation):**
❌ 21 test files use `NewServerWithK8sClient()` → creates NEW audit stores  
❌ Each audit store uses `getDataStorageURL()` → wrong port (`18090`)  
❌ Background flusher points to non-existent DataStorage  
❌ 14 audit tests fail with "connection refused"  

### **After (Standardized Pattern):**
✅ All tests use `createGatewayServer(..., sharedAuditStore)`  
✅ ONE shared audit store per process → continuous background flusher  
✅ Correct DataStorage URL (`http://127.0.0.1:18091`)  
✅ Matches pattern used by WE, NT, RO, SP, AA services  
✅ Zero audit test failures  

---

## 📚 Reference Implementation

**Best Reference:** `test/integration/workflowexecution/suite_test.go`

**Why?**
- Clean infrastructure constant usage
- Standard `integration.NewAuthenticatedDataStorageClients()` helper
- Single shared audit store for entire suite
- No environment variable overrides

**Pattern to Copy:**
```go
// Phase 2
dataStorageBaseURL := fmt.Sprintf("http://127.0.0.1:%d", infrastructure.WEIntegrationDataStoragePort)
dsClients = integration.NewAuthenticatedDataStorageClients(dataStorageBaseURL, token, 5*time.Second)
auditStore, err = audit.NewBufferedStore(dsClients.AuditClient, audit.DefaultConfig(), "workflowexecution", logger)
```

---

## 🚨 Key Insight: Why Gateway Failed

**Root Cause:** Gateway tests created a **hybrid anti-pattern**:
1. ✅ `suite_test.go` creates `sharedAuditStore` (correct)
2. ❌ 21 tests ignore `sharedAuditStore` and use `NewServerWithK8sClient()` (incorrect)
3. ❌ `NewServerWithK8sClient()` → `createServerWithClients()` → creates **NEW audit store**
4. ❌ Config uses `getDataStorageURL()` → wrong port → connection refused

**Solution:** Delete `NewServerWithK8sClient()` usage from tests, use `createGatewayServer(..., sharedAuditStore)` exclusively

---

**Author:** AI Assistant (via Cursor)  
**Branch:** `feature/k8s-sar-user-id-stateless-services`
