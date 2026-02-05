# DD-AUTH-014: Header vs Context Analysis - Test Coverage

**Date**: 2026-01-27
**Authority**: DD-AUTH-014 (Middleware-Based SAR Authentication)

---

## 🎯 **User Questions**

1. **Who's populating the `X-Auth-Request-User` header in E2E tests?**
2. **Who's populating it in integration tests?**
3. **Are these scenarios covered in tests?**

---

## 📊 **Complete Answer**

### **1. E2E Tests (DataStorage)** ✅

**Who populates**: **Auth middleware** (after TokenReview + SAR)

**How it works**:
```go
// test/e2e/datastorage/datastorage_e2e_suite_test.go
// 1. Create ServiceAccount with Bearer token
token := getServiceAccountToken("datastorage-e2e-client")

// 2. Configure HTTP client with ServiceAccountTransport
transport := NewServiceAccountTransport(token)
HTTPClient = &http.Client{
    Transport: transport,
    Timeout:   20 * time.Second,
}

// 3. Tests make requests with Bearer token
req.Header.Set("Authorization", "Bearer " + token)
resp, err := HTTPClient.Do(req)

// 4. Auth middleware:
//    - Validates token via TokenReview
//    - Checks permissions via SAR
//    - Injects X-Auth-Request-User header ← HERE
//    - Passes to handler

// 5. Handler reads header
placedBy := r.Header.Get("X-Auth-Request-User")
```

**Code Reference**:
```go
// pkg/datastorage/server/middleware/auth.go:243
// Step 5: Inject X-Auth-Request-User header
r.Header.Set("X-Auth-Request-User", user)
```

**Test Evidence**:
```go
// test/e2e/datastorage/20_legal_hold_api_test.go:182
// DD-AUTH-014: X-Auth-Request-User is now injected by auth middleware
// (no manual header setting needed - middleware extracts from authenticated ServiceAccount)
```

**Tests passing**: 189/189 (100%)

---

### **2. DataStorage Integration Tests** ✅

**Who populates**: **Nobody** - auth is disabled

**How it works**:
```go
// test/integration/datastorage/graceful_shutdown_integration_test.go:1030
// DD-AUTH-014: Pass nil, nil, "" to skip auth middleware (test environment)
srv, err := server.NewServer(
    dbConnStr, redisAddr, redisPassword, 
    logger, appCfg, serverCfg, 
    1000,
    nil,  // authenticator = nil → auth disabled
    nil,  // authorizer = nil → auth disabled
    "",   // authNamespace = "" → auth disabled
)
```

**Why it works**:
- Integration tests test **repository layer** directly (no HTTP handlers)
- Example: `audit_export_integration_test.go` calls `auditRepo.CreateEvent()` directly
- Tests that need HTTP handlers (e.g., `graceful_shutdown_integration_test.go`) disable auth

**Code Reference**:
```go
// pkg/datastorage/server/server.go
if authenticator == nil || authorizer == nil {
    // Auth middleware is not applied
    return router
}
// Apply auth middleware only if authenticator/authorizer provided
router.Use(authMiddleware.Handler)
```

**Tests passing**: All DataStorage integration tests pass with auth disabled

---

### **3. Other Services' Integration Tests** ⚠️

**Who populates**: **MockUserTransport** (test helper)

**How it works**:
```go
// test/integration/gateway/suite_test.go:145
mockTransport := testauth.NewMockUserTransport("test-gateway@integration.test")
httpClient := &http.Client{Transport: mockTransport}
dsClient := datastorage.NewClientWithResponses(url, datastorage.WithHTTPClient(httpClient))

// MockUserTransport injects header
// test/shared/auth/mock_transport.go:102
reqClone.Header.Set("X-Auth-Request-User", t.mockUserID)
```

**Why needed**:
- Services like Gateway, SignalProcessing, etc. call DataStorage's REST API
- Their integration tests run against real DataStorage HTTP server
- DataStorage handlers check for `X-Auth-Request-User` header
- Without it, handlers return 401 Unauthorized

**Services using MockUserTransport**:
- ✅ Gateway
- ✅ AIAnalysis
- ✅ SignalProcessing
- ✅ WorkflowExecution
- ✅ RemediationOrchestrator
- ✅ Notification
- ✅ AuthWebhook

---

## 🧪 **Test Coverage Summary**

| Test Tier | Who Populates Header | How | Coverage |
|-----------|---------------------|-----|----------|
| **E2E (DataStorage)** | Auth middleware | TokenReview + SAR → inject header | ✅ 189/189 tests |
| **Integration (DataStorage)** | Nobody (auth disabled) | Pass nil authenticator/authorizer | ✅ All tests pass |
| **Integration (Other Services)** | MockUserTransport | Simulates middleware behavior | ✅ All tests pass |
| **Unit (Transport)** | MockUserTransport (tested) | Direct header injection | ✅ Tests validate injection |

---

## ⚠️ **Impact of Refactoring to Context**

### **Proposed Change**

**Current** (Header-based):
```go
// Handler
placedBy := r.Header.Get("X-Auth-Request-User")
```

**Proposed** (Context-based):
```go
// Handler
placedBy := middleware.GetUserFromContext(r.Context())
```

### **Impact Analysis**

| Test Tier | Current Status | After Refactor | Action Required |
|-----------|----------------|----------------|-----------------|
| **E2E (DataStorage)** | ✅ Works (middleware injects header) | ✅ Works (middleware sets context) | None |
| **Integration (DataStorage)** | ✅ Works (auth disabled) | ✅ Works (auth disabled) | None |
| **Integration (Other Services)** | ✅ Works (MockUserTransport) | ❌ **BREAKS** | **Needs redesign** |

### **Problem: MockUserTransport Cannot Set Context**

**Why it breaks**:
```go
// http.RoundTripper can only modify request headers
func (t *MockUserTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    reqClone := req.Clone(req.Context())
    reqClone.Header.Set("X-Auth-Request-User", t.mockUserID) // ✅ Can do
    // reqClone.Context().WithValue(...) // ❌ Cannot do - context is read-only
    return t.base.RoundTrip(reqClone)
}
```

**Why headers work but context doesn't**:
- Headers are **mutable** during request transport
- Context is **immutable** once request is created
- Context can only be set by **server-side middleware**

---

## ✅ **Solutions for Other Services' Integration Tests**

### **Option A: Keep Headers for Cross-Service Calls** (Recommended)

**Approach**: DataStorage handlers accept **both** header and context

```go
// Handler extracts user from either source
func getUserFromRequest(r *http.Request) string {
    // Priority 1: Context (set by auth middleware in production/E2E)
    if user := middleware.GetUserFromContext(r.Context()); user != "" {
        return user
    }
    
    // Priority 2: Header (set by MockUserTransport in integration tests)
    if user := r.Header.Get("X-Auth-Request-User"); user != "" {
        return user
    }
    
    return "" // Should not happen if auth is enabled
}
```

**Pros**:
- ✅ E2E tests work (context from middleware)
- ✅ Integration tests work (header from MockUserTransport)
- ✅ Minimal code changes
- ✅ Backward compatible

**Cons**:
- ⚠️ Two ways to pass user identity (slightly complex)

---

### **Option B: Mock Authenticator in Other Services' Integration Tests**

**Approach**: Other services run DataStorage with mock auth middleware

```go
// test/integration/gateway/suite_test.go
mockAuth := testauth.NewMockAuthenticator(map[string]string{
    "test-token": "test-gateway@integration.test",
})
mockAuthz := testauth.NewMockAuthorizer(true) // Allow all

dsServer, err := server.NewServer(
    dbConnStr, redisAddr, redisPassword,
    logger, appCfg, serverCfg,
    1000,
    mockAuth,  // ← Mock authenticator
    mockAuthz, // ← Mock authorizer
    "test",
)
```

**Pros**:
- ✅ Clean separation: handlers always use context
- ✅ Tests exercise real auth middleware code path
- ✅ More realistic integration test

**Cons**:
- ⚠️ Requires more test setup
- ⚠️ Other services need to manage DataStorage server lifecycle

---

### **Option C: Disable Auth for Cross-Service Integration Tests**

**Approach**: Other services run DataStorage with auth disabled

```go
// test/integration/gateway/suite_test.go
dsServer, err := server.NewServer(
    dbConnStr, redisAddr, redisPassword,
    logger, appCfg, serverCfg,
    1000,
    nil,  // ← Disable auth
    nil,  // ← Disable auth
    "",
)
```

**Pros**:
- ✅ Simple test setup
- ✅ Handlers don't check for user (context or header)

**Cons**:
- ⚠️ Handlers must handle empty user gracefully
- ⚠️ Less realistic (production always has auth)

---

## 📝 **Recommendation**

**Short-term** (for HAPI): ✅ **Already done** - use `request.state.user` (no header)

**Long-term** (for DataStorage): **Option A** (Accept both header and context)
- Minimal disruption to existing tests
- Maintains compatibility with cross-service integration tests
- Clear migration path: context is primary, header is fallback

**Rationale**:
- DataStorage handlers are consumed by **7 other services** in integration tests
- Refactoring all 7 services' integration tests is high-risk
- Option A provides clean migration path with backward compatibility

---

## 🔗 **Related Files**

### **E2E Test Setup**:
- `test/e2e/datastorage/datastorage_e2e_suite_test.go` - ServiceAccount provisioning
- `test/shared/auth/serviceaccount_transport.go` - Bearer token injection

### **Integration Test Setup**:
- `test/integration/datastorage/graceful_shutdown_integration_test.go` - Auth disabled
- `test/shared/auth/mock_transport.go` - Header injection for other services

### **Middleware**:
- `pkg/datastorage/server/middleware/auth.go` - Header injection logic
- `pkg/datastorage/server/server.go` - Auth middleware application

### **Handlers**:
- `pkg/datastorage/server/legal_hold_handler.go` - Reads header
- `pkg/datastorage/server/audit_export_handler.go` - Reads header

---

**Conclusion**: The header is populated by **auth middleware in E2E** and **MockUserTransport in integration tests**. Refactoring to context-only would break cross-service integration tests, requiring a migration strategy.
