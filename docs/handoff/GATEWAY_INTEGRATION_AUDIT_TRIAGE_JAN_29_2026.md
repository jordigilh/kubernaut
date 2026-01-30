# Gateway Integration Test Audit Failures - Triage Report

**Date:** January 29, 2026  
**Author:** AI Assistant  
**Status:** 🚨 ROOT CAUSE IDENTIFIED - Ready for Fix

---

## 📊 **Executive Summary**

Gateway integration tests: **73/90 passed (16 audit failures)**  
All 16 failures are due to **authentication mismatch** between Gateway and DataStorage.

**Root Cause:** Gateway uses `MockUserTransport` (X-Auth-Request-User headers) but DataStorage now requires **Bearer token authentication** (DD-AUTH-014 SAR middleware).

**Impact:** All audit events are buffered correctly but **dropped silently** due to HTTP 401 errors from DataStorage.

---

## 🔍 **Detailed Analysis**

### **Test Failures Pattern**

All 16 failures follow the same pattern:
1. ✅ Gateway buffers audit events successfully
2. ✅ Background writer flushes batches on timer (1s interval)
3. ❌ DataStorage returns HTTP 401 Unauthorized
4. ❌ Events are dropped (non-retryable 4xx error)
5. ❌ Test queries return 0 audit events

**Failed Test Scenarios:**
- `GW-INT-AUD-006`: CRD created audit events
- `GW-INT-AUD-007`: Target resource metadata in audit
- `GW-INT-AUD-008`: Fingerprint in audit event
- `GW-INT-AUD-009`: occurrence_count in new signal
- `GW-INT-AUD-011`: Signal deduplicated audit event
- `GW-INT-AUD-012`: Existing RR reference in dedup
- `GW-INT-AUD-013`: Incremented occurrence_count
- `GW-INT-AUD-014`: Multiple distinct fingerprints
- `GW-INT-AUD-015`: Terminal phase deduplication
- `GW-INT-AUD-016`: CRD creation failed audit
- `GW-INT-AUD-017`: Error type classification
- `GW-INT-AUD-019`: Circuit breaker error details
- `GW-INT-AUD-020`: Globally unique audit IDs
- `GW-INT-CFG-002`: Production-ready defaults
- `GW-INT-CFG-003`: Invalid config rejection

---

## 🚨 **Root Cause**

### **Evidence from Logs**

**50 occurrences** of this error pattern:

```
2026-01-29T17:13:03-05:00	ERROR	audit-store	Failed to write audit batch	
{"attempt": 1, "batch_size": 2, "error": "Data Storage Service returned status 401: HTTP 401 error: decode response: unexpected status code: 401"}

2026-01-29T17:13:03-05:00	ERROR	audit-store	Dropping audit batch due to non-retryable error (invalid data)	
{"batch_size": 2, "is_4xx_error": true}
```

### **Authentication Mismatch**

**Current Setup (INCORRECT):**

```go
// test/integration/gateway/suite_test.go:168-176
mockTransport := testauth.NewMockUserTransport(
    fmt.Sprintf("test-gateway@integration.test-p%d", processNum),
)

dsClient, err = audit.NewOpenAPIClientAdapterWithTransport(
    fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort),
    5*time.Second,
    mockTransport,  // ❌ Injects X-Auth-Request-User header (oauth-proxy model)
)
```

**What MockUserTransport Does:**
```go
// test/shared/auth/mock_transport.go:100-102
reqClone.Header.Set("X-Auth-Request-User", t.mockUserID)
// Simulates oauth-proxy behavior (OLD model)
```

**What DataStorage Expects (DD-AUTH-014):**
```go
// DataStorage SAR middleware validates:
1. Authorization: Bearer <token>  (TokenReview API)
2. SubjectAccessReview API (authorization)
```

**Conflict:**
- Gateway sends: `X-Auth-Request-User: test-gateway@integration.test-p1`
- DataStorage expects: `Authorization: Bearer <k8s-serviceaccount-token>`
- Result: **HTTP 401 Unauthorized**

---

## ✅ **Solution**

### **Pattern to Follow**

Use the **same pattern as AIAnalysis integration tests** (which successfully authenticate with DataStorage):

**AIAnalysis (CORRECT):**

```go
// test/integration/aianalysis/suite_test.go:384-389
Transport: testauth.NewServiceAccountTransport(authConfig.Token),
```

Where `authConfig.Token` comes from:
```go
// Created ServiceAccount in envtest
// Retrieved token using infrastructure.GetServiceAccountToken()
```

### **Required Changes for Gateway**

1. **Create ServiceAccount in envtest** (Phase 1 setup)
   - Namespace: `kubernaut-system` (or test namespace)
   - ServiceAccount: `gateway-integration-sa`
   - RBAC: Grant `audit:write` permission

2. **Get ServiceAccount Token**
   - Use `infrastructure.GetServiceAccountToken(ctx, namespace, saName, k8sConfig)`

3. **Replace MockUserTransport with ServiceAccountTransport**
   ```go
   // BEFORE (line 168-176)
   mockTransport := testauth.NewMockUserTransport(...)
   
   // AFTER
   token, err := infrastructure.GetServiceAccountToken(
       ctx, 
       "kubernaut-system", 
       "gateway-integration-sa", 
       k8sConfig,
   )
   Expect(err).ToNot(HaveOccurred())
   
   authTransport := testauth.NewServiceAccountTransport(token)
   
   dsClient, err = audit.NewOpenAPIClientAdapterWithTransport(
       fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort),
       5*time.Second,
       authTransport,  // ✅ Injects Bearer token
   )
   ```

---

## 📋 **Implementation Checklist**

**Phase 1: ServiceAccount Setup (suite_test.go BeforeSuite)**
- [ ] Create `gateway-integration-sa` ServiceAccount
- [ ] Create RBAC Role with `audit:write` permission
- [ ] Create RoleBinding
- [ ] Get ServiceAccount token

**Phase 2: Client Creation (suite_test.go AllProcesses)**
- [ ] Replace `NewMockUserTransport` with `NewServiceAccountTransport`
- [ ] Pass token from Phase 1
- [ ] Verify token is injected as Bearer header

**Phase 3: Validation**
- [ ] Re-run integration tests
- [ ] Verify all 16 audit tests pass
- [ ] Check DataStorage logs for successful audit writes (HTTP 201)

---

## 🔗 **Reference Examples**

### **AIAnalysis Integration (Successful Pattern)**

**ServiceAccount Setup:**
```go
// test/integration/aianalysis/suite_test.go:241-264
authConfig := infrastructure.CreateDataStorageAuthConfig(
    ctx,
    testEnv.Config,
    "kubernaut-system",
    "aianalysis-integration-sa",
)
```

**Client Creation:**
```go
// test/integration/aianalysis/suite_test.go:384-389
Transport: testauth.NewServiceAccountTransport(authConfig.Token),
```

### **Helper Functions Available**

```go
// test/infrastructure/serviceaccount.go
func CreateDataStorageAuthConfig(
    ctx context.Context,
    cfg *rest.Config,
    namespace string,
    saName string,
) (*AuthConfig, error)

func GetServiceAccountToken(
    ctx context.Context,
    namespace string,
    saName string,
    cfg *rest.Config,
) (string, error)
```

---

## 🎯 **Expected Outcome**

After implementing this fix:
- ✅ Gateway sends: `Authorization: Bearer <k8s-token>`
- ✅ DataStorage validates token via TokenReview API
- ✅ DataStorage authorizes request via SAR
- ✅ Audit batches written successfully (HTTP 201)
- ✅ Test queries return expected audit events
- ✅ All 16 audit tests pass

**Test Suite Status:**
- **Before:** 73/90 passed (16 audit failures)
- **After:** 90/90 passed ✅

---

## 📚 **Related Documentation**

- **DD-AUTH-014:** Middleware-Based SAR Authentication
- **DD-TEST-012:** envtest Real Authentication Pattern
- **DD-AUTH-005:** DataStorage Client Authentication Pattern
- **AIAnalysis Integration:** `test/integration/aianalysis/suite_test.go` (reference implementation)

---

## ⚠️ **Why This Wasn't Caught Earlier**

1. **DataStorage SAR Auth is NEW:** We just enabled it via envtest kubeconfig (earlier in this session)
2. **E2E Tests Don't Use This Pattern:** E2E tests use Kind cluster with real ServiceAccounts (different setup)
3. **MockUserTransport Worked Before:** Previously, DataStorage had no auth middleware

---

## 🚀 **Next Steps**

1. **Implement Fix:** Follow implementation checklist above
2. **Run Tests:** `make test-integration-gateway`
3. **Validate:** All 90 specs should pass
4. **Document:** Update any relevant test documentation if needed

**Estimated Effort:** 30-45 minutes (mostly copy-paste from AIAnalysis pattern)

---

**Key Insight:** This is a **clean architectural fix**, not a workaround. Gateway integration tests should use real authentication (ServiceAccount tokens) now that DataStorage has SAR middleware enabled.
