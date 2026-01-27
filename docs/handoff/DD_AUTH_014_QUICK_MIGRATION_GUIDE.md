# DD-AUTH-014: Quick Migration Guide - envtest Integration

**Target**: All 7 services using DataStorage in integration tests  
**Effort**: ~10 lines per service  
**Time**: ~5 minutes per service

---

## 🎯 **What You're Doing**

**Replacing**: `MockUserTransport` (manual header injection)  
**With**: `ServiceAccountTransport` (real K8s token + envtest validation)

**Result**: Integration tests now use **real middleware auth code path** ✅

---

## 📝 **Copy-Paste Template**

### **Step 1: Import the new helper** (top of `suite_test.go`)

```go
import (
    // ... existing imports ...
    testauth "github.com/jordigilh/kubernaut/test/shared/auth"
    "github.com/jordigilh/kubernaut/test/infrastructure"
)
```

---

### **Step 2: Add auth setup to Phase 1** (`SynchronizedBeforeSuite`)

**Find this**:
```go
var _ = SynchronizedBeforeSuite(func() []byte {
    // Phase 1: Infrastructure setup
    
    // ... envtest start code ...
    cfg, err := testEnv.Start()
    Expect(err).NotTo(HaveOccurred())
    
    // ... infrastructure start code ...
```

**Add after `testEnv.Start()`**:
```go
    // DD-AUTH-014: Create ServiceAccount + RBAC in envtest for real auth
    authConfig, err := infrastructure.CreateIntegrationServiceAccountWithDataStorageAccess(
        cfg,
        "YOUR_SERVICE_NAME-ds-client",  // ← Change to: gateway, aianalysis, etc.
        "default",
        GinkgoWriter,
    )
    Expect(err).ToNot(HaveOccurred())
```

**Then update `StartDSBootstrap` call**:
```go
    dsInfra, err := infrastructure.StartDSBootstrap(infrastructure.DSBootstrapConfig{
        ServiceName:       "YOUR_SERVICE_NAME",
        PostgresPort:      YOUR_POSTGRES_PORT,
        RedisPort:         YOUR_REDIS_PORT,
        DataStoragePort:   YOUR_DS_PORT,
        MetricsPort:       YOUR_METRICS_PORT,
        ConfigDir:         "test/integration/YOUR_SERVICE/config",
        EnvtestKubeconfig: authConfig.KubeconfigPath, // ← ADD THIS LINE
    }, GinkgoWriter)
```

**Then serialize token for Phase 2**:
```go
    // Serialize token for Phase 2 processes
    return []byte(authConfig.Token)  // ← CHANGE from: return []byte{}
```

---

### **Step 3: Update Phase 2** (`SynchronizedBeforeSuite` second function)

**Find this**:
```go
}, func(data []byte) {
    // Phase 2: Per-process setup
```

**Add token deserialization at the top**:
```go
}, func(data []byte) {
    // Phase 2: Per-process setup
    
    token := string(data) // ← ADD THIS LINE (deserialize token from Phase 1)
```

**Then find DataStorage client creation**:
```go
    // OLD CODE (remove this):
    mockTransport := testauth.NewMockUserTransport("test-yourservice@integration.test")
    dataStorageClient, err := audit.NewOpenAPIClientAdapterWithTransport(
        dataStorageBaseURL,
        5*time.Second,
        mockTransport,
    )
```

**Replace with**:
```go
    // DD-AUTH-014: Use real ServiceAccount token for auth
    saTransport := testauth.NewServiceAccountTransport(token) // ← NEW
    dataStorageClient, err := audit.NewOpenAPIClientAdapterWithTransport(
        dataStorageBaseURL,
        5*time.Second,
        saTransport, // ← CHANGED
    )
```

---

## 🔧 **Service-Specific Values**

| Service | `ServiceName` | `PostgresPort` | `RedisPort` | `DataStoragePort` | `MetricsPort` |
|---------|---------------|----------------|-------------|-------------------|---------------|
| **Gateway** | `gateway` | 15437 | 16383 | 18091 | 19091 |
| **RemediationOrchestrator** | `remediationorchestrator` | 15435 | 16381 | 18140 | 19140 |
| **AIAnalysis** | `aianalysis` | 15432 | 16378 | 18092 | 19092 |
| **SignalProcessing** | `signalprocessing` | 15433 | 16379 | 18093 | 19093 |
| **WorkflowExecution** | `workflowexecution` | 15434 | 16380 | 18094 | 19094 |
| **Notification** | `notification` | 15436 | 16382 | 18095 | 19095 |
| **AuthWebhook** | `authwebhook` | 15438 | 16384 | 18096 | 19096 |

---

## ✅ **Validation**

After making changes:

```bash
# Run integration tests
make test-integration-YOUR_SERVICE

# Should see in logs:
# ✅ Creating Integration ServiceAccount with DataStorage Access (envtest)
# ✅ ServiceAccount created
# ✅ ClusterRole created: data-storage-client
# ✅ ClusterRoleBinding created
# ✅ Token retrieved (expires: ...)
# ✅ Kubeconfig written: /tmp/envtest-kubeconfig-YOUR_SERVICE-ds-client.yaml
# ✅ Integration auth configured with real K8s TokenReview/SAR
# 🔐 Mounting envtest kubeconfig for real K8s auth
```

---

## 🚨 **Common Issues**

### **Issue**: Tests fail with `401 Unauthorized`
**Cause**: Token not being passed to DataStorage client  
**Fix**: Verify `token := string(data)` in Phase 2 and `saTransport := testauth.NewServiceAccountTransport(token)`

### **Issue**: Tests fail with `KUBECONFIG not found`
**Cause**: `EnvtestKubeconfig` not set in `DSBootstrapConfig`  
**Fix**: Verify `EnvtestKubeconfig: authConfig.KubeconfigPath` in `StartDSBootstrap`

### **Issue**: Tests fail with `connection refused`
**Cause**: envtest API server not started  
**Fix**: Verify `testEnv.Start()` completes before calling `CreateIntegrationServiceAccountWithDataStorageAccess`

---

## 📊 **Migration Progress Tracking**

- [x] **RemediationOrchestrator** - `test/integration/remediationorchestrator/suite_test.go` ✅ [COMPLETE](DD_AUTH_014_RO_MIGRATION_COMPLETE.md)
- [ ] Gateway - `test/integration/gateway/suite_test.go`
- [ ] AIAnalysis - `test/integration/aianalysis/suite_test.go`
- [ ] SignalProcessing - `test/integration/signalprocessing/suite_test.go`
- [ ] WorkflowExecution - `test/integration/workflowexecution/suite_test.go`
- [ ] Notification - `test/integration/notification/suite_test.go`
- [ ] AuthWebhook - `test/integration/authwebhook/suite_test.go`

---

## 🎉 **Benefits After Migration**

- ✅ **Real Auth Testing**: Middleware code path fully exercised
- ✅ **Zero Security Risk**: No conditional logic in production binary
- ✅ **Accurate Coverage**: Tests validate actual TokenReview/SAR behavior
- ✅ **Clean Handlers**: DataStorage handlers use context only (no header fallback)
- ✅ **Maintainability**: Single pattern across all services

---

**Total Migration Time**: ~35 minutes for all 7 services 🚀
