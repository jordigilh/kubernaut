# DataStorage E2E SAR - Middleware Architecture Decision

**Date**: January 26, 2026  
**Session**: SAR Authentication Architecture  
**Status**: ✅ **DECISION MADE** - Middleware with Dependency Injection  
**Design Decision**: DD-AUTH-014

---

## 📋 **Session Summary**

### **Problem Discovered**

E2E tests with `ose-oauth-proxy` failed because:
1. `ose-oauth-proxy` **only works with OpenShift** (requires `openshift-config-managed` namespace)
2. Kind clusters are **vanilla Kubernetes** (no OpenShift resources)
3. Cannot test SAR in E2E or integration environments with proxy approach

### **Root Cause Analysis**

```bash
# ose-oauth-proxy source code check
podman run --rm quay.io/jordigilh/ose-oauth-proxy:latest --help
# Result: --provider string: OAuth provider (default "openshift")

# main.go analysis shows ONLY OpenShift provider supported:
switch opts.Provider {
case "openshift":
    p = providerOpenShift
default:
    os.Exit(1)  // ANY other provider EXITS!
}
```

**Conclusion**: `ose-oauth-proxy` is **hardcoded for OpenShift only** ❌

---

## 🎯 **Architectural Decision**

**Use middleware-based authentication with dependency injection** instead of proxy-based authentication.

### **Key Benefits**

| Aspect | Proxy (ose-oauth-proxy) | Middleware (DD-AUTH-014) |
|--------|-------------------------|--------------------------|
| **OpenShift Dependency** | ✅ Required | ❌ Not required |
| **Works in Kind (E2E)** | ❌ No | ✅ Yes |
| **Works in envtest (Integration)** | ❌ No | ✅ Yes (with mocks) |
| **Testing** | Limited (can't test auth) | Full (unit, integration, E2E) |
| **Complexity** | High (sidecar) | Low (application code) |
| **Security** | Good | Good (no disable flags) |
| **Portability** | OpenShift only | Any Kubernetes |

---

## 🏗️ **Architecture Overview**

### **Design Pattern: Interface-Based Dependency Injection**

```go
// 1. Define interfaces (pkg/shared/auth/interfaces.go)
type Authenticator interface {
    ValidateToken(ctx context.Context, token string) (string, error)
}

type Authorizer interface {
    CheckAccess(ctx context.Context, user, namespace, resource, resourceName, verb string) (bool, error)
}

// 2. Production: Real Kubernetes implementations
type K8sAuthenticator struct {
    client kubernetes.Interface
}
// Uses TokenReview API

type K8sAuthorizer struct {
    client kubernetes.Interface
}
// Uses SubjectAccessReview API

// 3. Tests: Mock implementations
type MockAuthenticator struct {
    ValidUsers map[string]string
}

type MockAuthorizer struct {
    AllowedUsers map[string]bool
}

// 4. Middleware (always enforced - no disable flags)
type AuthMiddleware struct {
    authenticator Authenticator  // Injected at runtime
    authorizer    Authorizer     // Injected at runtime
}
```

---

## 🧪 **Testing Strategy**

### **Unit Tests** (Middleware)
```go
// Test with mocks - validate middleware logic
authenticator := &MockAuthenticator{ValidUsers: testUsers}
authorizer := &MockAuthorizer{AllowedUsers: allowedUsers}
middleware := NewAuthMiddleware(authenticator, authorizer)

It("should reject request without token", func() {
    // Test returns 401
})

It("should reject unauthorized user", func() {
    // Test returns 403
})
```

### **Integration Tests** (envtest)
```go
// Inject mocks - auth still enforced!
authenticator := &MockAuthenticator{
    ValidUsers: map[string]string{
        "test-token": "system:serviceaccount:test:authorized-sa",
    },
}
authorizer := &MockAuthorizer{
    AllowedUsers: map[string]bool{
        "system:serviceaccount:test:authorized-sa": true,
    },
}

dsServer := datastorage.NewServer(datastorage.Config{
    Authenticator: authenticator,  // Mock injected
    Authorizer:    authorizer,      // Mock injected
})

// Tests provide tokens - auth is validated
req.Header.Set("Authorization", "Bearer test-token")
```

### **E2E Tests** (Kind)
```go
// Real Kubernetes auth - full validation
// DataStorage uses K8sAuthenticator + K8sAuthorizer

// Get real token from Kind cluster
token, err := infrastructure.GetServiceAccountToken(
    ctx, "datastorage-e2e", "authorized-sa", kubeconfigPath,
)

// Make authenticated request
req.Header.Set("Authorization", "Bearer "+token)

// DataStorage validates with real TokenReview + SAR ✅
resp, err := client.CreateWorkflow(ctx, req)
Expect(resp.StatusCode).To(Equal(201))
```

---

## 🔐 **Security Considerations**

### **❌ NO Runtime Disable Flags** (Critical Security Requirement)

**User's Requirement**:
> "I don't want to have any logic in the code to disable auth or authz. It's a security risk"

**Solution**: Dependency injection ensures auth is **always enforced**

```go
// ❌ DANGEROUS (rejected approach)
if m.authEnabled {  // Runtime flag
    validateAuth()
}

// ✅ SECURE (approved approach)
func (m *AuthMiddleware) Handler(next http.Handler) http.Handler {
    // Auth ALWAYS runs - no bypass possible
    user, err := m.authenticator.ValidateToken(ctx, token)
    // Only the implementation varies (real vs mock)
}
```

### **Defense in Depth**

1. HTTP Authorization header validation
2. Token format validation (Bearer prefix)
3. TokenReview API call (authentication)
4. SubjectAccessReview API call (authorization)
5. RBAC policy evaluation (Kubernetes)

---

## 📊 **Environment Support**

| Environment | Authenticator | Authorizer | Status |
|------------|---------------|------------|--------|
| **Production (OpenShift)** | K8sAuthenticator | K8sAuthorizer | ✅ Real K8s APIs |
| **E2E (Kind)** | K8sAuthenticator | K8sAuthorizer | ✅ Real K8s APIs |
| **Integration (envtest)** | MockAuthenticator | MockAuthorizer | ✅ Test doubles |
| **Unit Tests** | MockAuthenticator | MockAuthorizer | ✅ Test doubles |

---

## 🚀 **Implementation Plan** (Phased Approach)

### **Critical Concern: API Server Load** 🚨

**User's Concern**: 
> "My main concern was hitting the API server including the middleware as part of the business logic"

**Impact**:
- Every authenticated request hits Kubernetes API server twice (TokenReview + SAR)
- High-throughput services (Gateway) could overload API server
- Need to measure and optimize

**Mitigation**:
- Token caching (5-minute TTL)
- Connection pooling
- Performance metrics
- Phased rollout (validate impact before expanding)

---

### **Rollout Strategy** 🎯

**User's Decision**:
> "Let's document this approach first, implement it in the DS as test bed then we can decide to extend to the HAPI or to all other services."

**Plan**: DataStorage POC → Measure impact → Decide expansion

---

### **Phase 1: Core Infrastructure** (1 day)
- [ ] Create `pkg/shared/auth/interfaces.go`
- [ ] Implement `K8sAuthenticator` (TokenReview)
- [ ] Implement `K8sAuthorizer` (SAR)
- [ ] Implement `MockAuthenticator` (tests)
- [ ] Implement `MockAuthorizer` (tests)
- [ ] Add connection pooling + retry logic
- [ ] (Optional) Implement `CachedAuthenticator` for optimization

### **Phase 2: DataStorage POC** ⭐ (2-3 days)

**Goal**: Prove approach works + measure API server impact + **completely remove oauth-proxy**

**Implementation**:
- [ ] Create `pkg/datastorage/middleware/auth.go`
- [ ] Update `cmd/datastorage/main.go` (inject real implementations)
- [ ] **Remove ALL oauth-proxy references from code**
- [ ] Update integration tests (inject mocks)
- [ ] Update E2E tests (remove proxy, use direct auth)
- [ ] **Remove `ose-oauth-proxy` from `test/infrastructure/datastorage.go`**
- [ ] **Remove oauth-proxy sidecar from `deploy/data-storage/deployment.yaml`**
- [ ] Update service ports (8081 direct, no proxy)

**Performance Validation**:
- [ ] Measure TokenReview/SAR API call rates during E2E
- [ ] Monitor API server load metrics
- [ ] Collect latency metrics (p50, p95, p99)
- [ ] Document findings

### **Phase 3: Decision Point** 🔀 (1 day)

**Evaluation Criteria**:
- ✅ All DataStorage tests pass
- ✅ oauth-proxy completely removed
- ✅ API server load acceptable
- ✅ Latency acceptable (p95 < 500ms)
- ✅ No rate limiting issues

**Decision Options**:
- **Option A**: Expand to HAPI only (if API server shows stress)
- **Option B**: Expand to all services (if no issues)
- **Option C**: Rollback and re-evaluate (if POC fails)

### **Phase 4: HolmesGPT API** (If Approved) (2 days)
- [ ] Apply same pattern as DataStorage
- [ ] **Remove oauth-proxy from HAPI**
- [ ] Measure API server impact

### **Phase 5: Gateway E2E Evaluation** ⚠️ (1 day)

**User's Note**:
> "I don't know if the gateway e2e tests will need to be refactored since it might hit the api server, but we need to evaluate it later on."

**Concern**: Gateway processes high alert volumes - may stress API server

**Actions**:
- [ ] Run Gateway E2E with current auth
- [ ] Measure TokenReview/SAR call patterns
- [ ] Estimate load if middleware applied
- [ ] Consider optimizations (longer cache TTL, batch SAR)
- [ ] **Decide**: Apply middleware only if load acceptable

### **Phase 6: Documentation** (1 day)
- [ ] Update DD-AUTH-011, DD-AUTH-012 (mark as superseded)
- [ ] Create migration guide
- [ ] Performance tuning guide
- [ ] Troubleshooting runbook

---

## 📚 **Key Documents**

### **Created in This Session**
- **DD-AUTH-014**: Middleware-Based SAR Authentication (AUTHORITATIVE)
- **This Document**: Session handoff and decision summary

### **Related Documents**
- **DD-AUTH-011**: Granular RBAC & SAR Verb Mapping (superseded)
- **DD-AUTH-012**: ose-oauth-proxy for SAR (superseded)
- **DD-AUTH-013**: HTTP Status Codes for Auth Errors
- **OSE_OAUTH_PROXY_E2E_FINDING_JAN08.md**: Original proxy investigation

---

## 🎓 **Key Learnings**

### **1. ose-oauth-proxy is OpenShift-Only**
- Hardcoded provider: `"openshift"` (only option)
- Requires OpenShift-specific resources
- Cannot be used in vanilla Kubernetes (Kind, EKS, GKE, AKS)

### **2. envtest Does NOT Support RBAC Evaluation**
- envtest starts Kubernetes API server
- SAR API endpoint exists, but authorization is disabled by default
- Cannot test real RBAC policies in envtest
- **Solution**: Use mocks for integration tests

### **3. Kind DOES Support Full RBAC**
- Kind is a real Kubernetes cluster
- TokenReview API works ✅
- SubjectAccessReview API works ✅
- RBAC policies are evaluated ✅
- **Can test full auth flow in E2E** ✅

### **4. Dependency Injection Enables Secure Testing**
- Production: Real Kubernetes APIs
- Tests: Mock implementations
- Same middleware code (no disable flags)
- Auth always enforced

---

## ✅ **Decision Approval**

**Proposed By**: Engineering Team  
**Decision**: Use middleware-based authentication with dependency injection  
**Scope**: DataStorage POC first, then evaluate expansion

**Rationale**: 
- Portable (works on any Kubernetes)
- Testable (full coverage in all tiers using dependency injection)
- Secure (no runtime disable flags - user mandate)
- **Completely removes oauth-proxy** (no sidecar containers)
- Standard pattern (Go interfaces)

**Critical Concerns**:
- ⚠️ API server load (TokenReview + SAR calls)
- ⚠️ Gateway E2E impact (high-throughput)
- ✅ Mitigated by: Caching, connection pooling, phased rollout

**Phased Rollout**:
1. ⭐ DataStorage POC (prove approach, measure impact)
2. 🔀 Decision point (evaluate results)
3. 📈 Expand to HAPI or all services (based on POC)

**Next Action**: Implement Phase 1 (Core Infrastructure) + Phase 2 (DataStorage POC)

---

## 📞 **Key Concerns to Address**

### **1. API Server Load** 🚨
**Concern**: Every authenticated request hits API server twice (TokenReview + SAR)

**Mitigation**:
- Token caching (5-minute TTL)
- Connection pooling
- Performance metrics during POC
- Phased rollout to validate impact

### **2. Gateway E2E Tests** ⚠️
**Concern**: Gateway processes high alert volumes - may overload API server

**Plan**:
- Evaluate Gateway E2E after DataStorage POC
- Measure call patterns and API server load
- Consider optimizations (longer cache, batch SAR)
- **Decide later**: Apply middleware only if acceptable

### **3. Complete oauth-proxy Removal** ✅
**Goal**: Remove ALL oauth-proxy dependencies from DataStorage and HAPI

**Actions**:
- Remove sidecar containers
- Remove proxy configuration
- Update service ports (direct access)
- Update health checks
- Update E2E infrastructure

### **4. Dependency Injection for Tests** ✅
**Requirement**: No runtime disable flags in production code

**Solution**:
- Use interfaces (`Authenticator`, `Authorizer`)
- Inject mocks in integration tests
- Inject real implementations in E2E/production
- Auth always enforced (secure)

---

**Document Version**: 1.0  
**Last Updated**: January 26, 2026  
**Status**: ✅ Decision approved  
**Next Step**: Begin implementation (Phase 1)
