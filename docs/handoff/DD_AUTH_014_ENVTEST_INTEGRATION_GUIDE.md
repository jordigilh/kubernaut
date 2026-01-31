# DD-AUTH-014: envtest Integration Guide for Real K8s Auth in Integration Tests

> ⚠️ **DEPRECATION NOTICE**: ENV_MODE pattern removed as of Jan 31, 2026 (commit `5dce72c5d`)
>
> **What Changed**: HAPI production code no longer uses ENV_MODE conditional logic.
> - Production & Integration: Both use `K8sAuthenticator` + `K8sAuthorizer`
> - KUBECONFIG environment variable determines K8s API endpoint (in-cluster vs envtest)
> - Mock auth classes available for unit tests only (not in main.py)
>
> **See**: `holmesgpt-api/AUTH_RESPONSES.md` for current architecture


**Status**: Implementation Complete  
**Date**: 2026-01-27  
**Authority**: DD-AUTH-014 (Middleware-based Authentication)

---

## 🎯 **Problem Solved**

Previously, integration tests for services that depend on DataStorage had **three bad options**:

1. ❌ **MockUserTransport** - Manually inject `X-Auth-Request-User` header (bypasses auth)
2. ❌ **ENV_MODE** - Conditional auth logic in production binary (security risk)
3. ❌ **Disabled Auth** - Pass `nil` authenticator/authorizer (no test coverage)

**Now**: Integration tests use **real Kubernetes TokenReview/SAR APIs via envtest** ✅

---

## ✅ **Benefits**

| Benefit | Description |
|---------|-------------|
| **Real Auth Code Path** | Tests execute actual middleware auth logic (TokenReview + SAR) |
| **Zero Security Risk** | Production binary unchanged - no ENV_MODE, no conditional logic |
| **Clean Handler Logic** | DataStorage handlers use `context` only (no header fallback) |
| **Accurate Test Coverage** | Integration tests now validate real auth behavior |
| **Minimal Code Changes** | One-liner addition to each service's test suite |

---

## 🔧 **Implementation (3 Steps per Service)**

### **Step 1: Create ServiceAccount + Token in envtest**

Add to `SynchronizedBeforeSuite` **Phase 1** (after envtest starts):

```go
var _ = SynchronizedBeforeSuite(func() []byte {
    // ... Phase 1: Start infrastructure (PostgreSQL, Redis) ...
    
    // NEW: Create envtest ServiceAccount with DataStorage RBAC
    authConfig, err := infrastructure.CreateIntegrationServiceAccountWithDataStorageAccess(
        cfg,                                    // ← from testEnv.Start()
        "remediationorchestrator-ds-client",   // ServiceAccount name
        "default",                              // Namespace
        GinkgoWriter,                           // Log writer
    )
    Expect(err).ToNot(HaveOccurred())
    
    // Start DataStorage with envtest kubeconfig
    dsInfra, err := infrastructure.StartDSBootstrap(infrastructure.DSBootstrapConfig{
        ServiceName:       "remediationorchestrator",
        PostgresPort:      15435,
        RedisPort:         16381,
        DataStoragePort:   ROIntegrationDataStoragePort,
        MetricsPort:       19140,
        ConfigDir:         "test/integration/remediationorchestrator/config",
        EnvtestKubeconfig: authConfig.KubeconfigPath, // ← NEW: Pass envtest kubeconfig
    }, GinkgoWriter)
    Expect(err).ToNot(HaveOccurred())
    
    // Serialize token for Phase 2
    return []byte(authConfig.Token)
}, func(data []byte) {
    // Phase 2: Use token for client requests
    token := string(data)
    // ... rest of Phase 2 setup ...
})
```

---

### **Step 2: Replace MockUserTransport with ServiceAccountTransport**

**Before (DD-AUTH-005 - MockUserTransport)**:
```go
mockTransport := testauth.NewMockUserTransport("test-remediationorchestrator@integration.test")
dataStorageClient, err := audit.NewOpenAPIClientAdapterWithTransport(
    dataStorageBaseURL,
    5*time.Second,
    mockTransport, // ← Manually injects X-Auth-Request-User header
)
```

**After (DD-AUTH-014 - ServiceAccountTransport)**:
```go
saTransport := testauth.NewServiceAccountTransport(token) // ← Uses real K8s token
dataStorageClient, err := audit.NewOpenAPIClientAdapterWithTransport(
    dataStorageBaseURL,
    5*time.Second,
    saTransport, // ← Injects Bearer token (middleware validates via envtest)
)
```

---

### **Step 3: Update Main Application (if needed)**

If your service's `main.go` was previously passing `nil` for DataStorage client authenticator:

**Before**:
```go
// DD-AUTH-005: Integration tests use mock user transport (no auth middleware)
dataStorageClient, err := audit.NewOpenAPIClientAdapter(dataStorageURL, 5*time.Second)
```

**After**:
```go
// DD-AUTH-014: Production uses real auth (in-cluster kubeconfig)
// Integration tests use envtest auth (envtest kubeconfig)
dataStorageClient, err := audit.NewOpenAPIClientAdapter(dataStorageURL, 5*time.Second)
// No changes needed - client automatically uses in-cluster config in production
// and envtest config in integration tests (via KUBECONFIG environment variable)
```

---

## 📊 **Affected Services**

All services that use DataStorage in integration tests:

| Service | Integration Test Suite | Lines Changed |
|---------|------------------------|---------------|
| **RemediationOrchestrator** | `test/integration/remediationorchestrator/suite_test.go` | ~10 lines |
| **Gateway** | `test/integration/gateway/suite_test.go` | ~10 lines |
| **AIAnalysis** | `test/integration/aianalysis/suite_test.go` | ~10 lines |
| **SignalProcessing** | `test/integration/signalprocessing/suite_test.go` | ~10 lines |
| **WorkflowExecution** | `test/integration/workflowexecution/suite_test.go` | ~10 lines |
| **Notification** | `test/integration/notification/suite_test.go` | ~10 lines |
| **AuthWebhook** | `test/integration/authwebhook/suite_test.go` | ~10 lines |

**Total**: ~70 lines of code changes across 7 services

---

## 🔍 **Technical Details**

### **What `CreateIntegrationServiceAccountWithDataStorageAccess` Does**

```go
func CreateIntegrationServiceAccountWithDataStorageAccess(
    cfg *rest.Config,     // envtest config from testEnv.Start()
    saName string,        // ServiceAccount name (e.g., "gateway-ds-client")
    namespace string,     // Namespace (e.g., "default")
    writer io.Writer,     // Log writer (e.g., GinkgoWriter)
) (*IntegrationAuthConfig, error)
```

**Steps**:
1. ✅ Create `Namespace` (if it doesn't exist)
2. ✅ Create `ServiceAccount` with labels (`rbac:dd-auth-014`)
3. ✅ Create `ClusterRole` (`data-storage-client`) with DataStorage RBAC:
   - API: `""`
   - Resource: `services`
   - ResourceName: `data-storage-service`
   - Verbs: `create, get, list, update, delete`
4. ✅ Create `ClusterRoleBinding` (links ServiceAccount → ClusterRole)
5. ✅ Call **TokenRequest API** to get JWT token (expires in 1 hour)
6. ✅ Write **kubeconfig file** to `/tmp/envtest-kubeconfig-{saName}.yaml`
7. ✅ Return `IntegrationAuthConfig` with token + kubeconfig path

### **What Happens Inside DataStorage Container**

```yaml
# DataStorage container receives:
ENV KUBECONFIG=/tmp/kubeconfig  # ← Mounted from host

# DataStorage middleware (pkg/datastorage/server/middleware/auth.go):
1. Extracts Bearer token from Authorization header
2. Calls TokenReview API (envtest) → validates token
3. Calls SAR API (envtest) → checks RBAC
4. Sets user in context: middleware.UserContextKey
5. Handler reads user: middleware.GetUserFromContext(r.Context())
```

---

## 🧪 **Testing Strategy**

### **Unit Tests**
- Mock `Authenticator`/`Authorizer` interfaces
- Test business logic in isolation
- No envtest required

### **Integration Tests (NEW)**
- Real `K8sAuthenticator`/`K8sAuthorizer` implementations
- envtest provides real TokenReview/SAR APIs
- Tests validate actual middleware code path

### **E2E Tests**
- Real Kind cluster
- Real Kubernetes API server
- Tests validate production deployment

---

## 📝 **Example: RemediationOrchestrator Integration Test**

```go
var _ = SynchronizedBeforeSuite(func() []byte {
    // ════════════════════════════════════════════════════════════════════════
    // PHASE 1: INFRASTRUCTURE + envtest SETUP
    // ════════════════════════════════════════════════════════════════════════
    
    // Start envtest (runs ONCE globally)
    testEnv = &envtest.Environment{
        CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
        ErrorIfCRDPathMissing: true,
    }
    cfg, err := testEnv.Start()
    Expect(err).NotTo(HaveOccurred())
    
    // Create ServiceAccount + RBAC in envtest (NEW)
    authConfig, err := infrastructure.CreateIntegrationServiceAccountWithDataStorageAccess(
        cfg,
        "remediationorchestrator-ds-client",
        "default",
        GinkgoWriter,
    )
    Expect(err).ToNot(HaveOccurred())
    
    // Start DataStorage with envtest kubeconfig (UPDATED)
    dsInfra, err := infrastructure.StartDSBootstrap(infrastructure.DSBootstrapConfig{
        ServiceName:       "remediationorchestrator",
        PostgresPort:      15435,
        RedisPort:         16381,
        DataStoragePort:   ROIntegrationDataStoragePort,
        MetricsPort:       19140,
        ConfigDir:         "test/integration/remediationorchestrator/config",
        EnvtestKubeconfig: authConfig.KubeconfigPath, // ← NEW
    }, GinkgoWriter)
    Expect(err).ToNot(HaveOccurred())
    
    DeferCleanup(func() {
        _ = infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
    })
    
    // Serialize token for Phase 2
    return []byte(authConfig.Token)
    
}, func(data []byte) {
    // ════════════════════════════════════════════════════════════════════════
    // PHASE 2: PER-PROCESS SETUP
    // ════════════════════════════════════════════════════════════════════════
    
    token := string(data) // Deserialize token
    
    // Create DataStorage client with ServiceAccount transport (UPDATED)
    saTransport := testauth.NewServiceAccountTransport(token) // ← NEW
    dataStorageClient, err := audit.NewOpenAPIClientAdapterWithTransport(
        dataStorageBaseURL,
        5*time.Second,
        saTransport, // ← Real K8s token (not MockUserTransport)
    )
    Expect(err).ToNot(HaveOccurred())
    
    // ... rest of controller setup ...
})
```

---

## 🚀 **Rollout Plan**

### **Phase 1: DataStorage Handler Refactoring**
- ✅ Remove `X-Auth-Request-User` header fallback from handlers
- ✅ Update handlers to use `middleware.GetUserFromContext(r.Context())` exclusively
- ✅ Update DataStorage integration tests to verify context-only user attribution

### **Phase 2: Infrastructure Functions**
- ✅ Add `CreateIntegrationServiceAccountWithDataStorageAccess` to `test/infrastructure/serviceaccount.go`
- ✅ Update `DSBootstrapConfig` to accept `EnvtestKubeconfig`
- ✅ Update `startDSBootstrapService` to mount kubeconfig and set `KUBECONFIG` env var

### **Phase 3: Service-by-Service Migration** (Current)
- 🔄 Update RemediationOrchestrator integration tests
- ⏳ Update Gateway integration tests
- ⏳ Update AIAnalysis integration tests
- ⏳ Update SignalProcessing integration tests
- ⏳ Update WorkflowExecution integration tests
- ⏳ Update Notification integration tests
- ⏳ Update AuthWebhook integration tests

### **Phase 4: Cleanup**
- ⏳ Delete `test/shared/auth/mock_transport.go` (no longer needed)
- ⏳ Update documentation to reflect new testing strategy
- ⏳ Verify all integration tests pass with real auth

---

## 📚 **References**

- **[DD-AUTH-014](DD_AUTH_014_HAPI_IMPLEMENTATION_COMPLETE.md)**: Middleware-based authentication with dependency injection
- **[DD-AUTH-010](../architecture/decisions/DD-AUTH-010-e2e-real-authentication-mandate.md)**: E2E Real Authentication Mandate
- **[DD-TEST-010](../architecture/decisions/DD-TEST-010-multi-controller-pattern.md)**: Multi-Controller Pattern for Parallel Test Execution
- **[DD-AUTH-005](../architecture/decisions/DD-AUTH-005-integration-testing-strategy.md)**: Integration Testing Strategy (DEPRECATED - replaced by DD-AUTH-014)

---

## ✅ **Validation Checklist**

Before marking this complete, verify:

- [ ] All 7 services' integration tests updated
- [ ] DataStorage handlers use context-only user attribution
- [ ] MockUserTransport deleted
- [ ] All integration tests pass with real auth
- [ ] Documentation updated

---

**Summary**: This guide provides a **zero-security-risk, minimal-code-change solution** for integrating real Kubernetes auth into integration tests. Each service requires ~10 lines of code changes to gain accurate auth test coverage.
