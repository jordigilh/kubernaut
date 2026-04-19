# DD-AUTH-014: Middleware-Based SAR Authentication (Interface-Driven)

**Status**: Approved  
**Version**: 3.0  
**Date**: January 31, 2026  
**Decision Makers**: Architecture Team  
**Affected Services**: 
- **Phase 2 (POC)**: ✅ DataStorage (Complete)
- **Phase 3**: ✅ HolmesGPT API (Complete)
- **Phase 4**: ✅ Gateway (Complete - January 2026)
- **Phase 5**: ✅ AIAnalysis Controller (Complete - January 2026)
- **Future**: Notification, other REST API services (TBD)

---

## 📋 **Changelog**

### Version 3.0 (January 31, 2026)
- **CRITICAL FIX**: Corrected RBAC for HolmesGPT API access
  - **FOUND**: Gateway was granted `kubernaut-agent-client` RBAC but has ZERO HAPI code references
  - **FIXED**: AIAnalysis controller granted `kubernaut-agent-client` RBAC (actual HAPI caller)
  - **EVIDENCE**: `cmd/aianalysis/main.go` has 9 HAPI references, `cmd/gateway/main.go` has 0
  - **ARCHITECTURE**: Gateway creates AIAnalysis CRDs → Controller calls HAPI (correct flow)
- **ADDED**: AIAnalysis controller ServiceAccount token mount (`automountServiceAccountToken: true`)
- **IMPACT**: Fixes all 21 AIAnalysis E2E test failures (HTTP 401 auth errors eliminated)
- **PRODUCTION**: `deploy/kubernaut-agent/14-client-rbac.yaml` corrected (gateway-sa → aianalysis-controller)
- **E2E**: `test/infrastructure/aianalysis_e2e.go` updated with proper RBAC
- **RELATED**: Commits `ccbc818f3`, `a786c11a5` (AIAnalysis auth fix)

### Version 2.0 (January 29, 2026)
- **APPROVED**: Gateway service added to Phase 4 scope
- **RATIONALE**: Gateway is external-facing entry point, requires authentication for:
  - Defense-in-depth security (zero-trust architecture)
  - SOC2 compliance (operator attribution for signal injection)
  - Webhook compatibility (Prometheus AlertManager + K8s Events support Bearer tokens)
- **DECISION**: No caching for Gateway (low throughput <100 signals/min, NetworkPolicy reduces risk)
- **SUPERSEDES**: DD-GATEWAY-006 (Network Policies only) - now obsolete
- **UPDATES**: ADR-036 exception - Gateway now requires SAR auth despite original decision
- **NEW BRs**: BR-GATEWAY-182 (Authentication), BR-GATEWAY-183 (Authorization)

### Version 1.0 (January 26, 2026)
- Initial POC design for DataStorage
- Interface-driven architecture with dependency injection
- Real K8s auth for E2E, mocks for integration tests (later revised)

---

## 📋 **Context**

### **Problem Statement**

Current implementation uses `ose-oauth-proxy` sidecar for authentication and Subject Access Review (SAR) authorization. This approach has several limitations:

1. **OpenShift Dependency**: `ose-oauth-proxy` requires OpenShift-specific resources (`openshift-config-managed` namespace, OAuth server)
2. **Testing Limitations**: Cannot test SAR in E2E (Kind) or Integration (envtest) environments
3. **Complexity**: Sidecar containers, port mapping, configuration overhead
4. **Limited Control**: Authorization logic external to application
5. **Debugging Difficulty**: Logs split between proxy and application

### **Requirements**

- **REQ-1**: Authenticate ServiceAccount tokens using Kubernetes TokenReview API
- **REQ-2**: Authorize requests using Kubernetes SubjectAccessReview (SAR) API
- **REQ-3**: Extract user identity for audit logging (SOC2 CC8.1 compliance)
- **REQ-4**: Work in all environments: Production (OpenShift), E2E (Kind), Integration (envtest)
- **REQ-5**: Testable without mocking infrastructure in integration tests (use dependency injection)
- **REQ-6**: No runtime disable flags (security requirement - user mandate)
- **REQ-7**: Reusable across all REST API services
- **REQ-8**: **Completely remove oauth-proxy dependency** (no sidecars)
- **REQ-9**: **Minimal API server impact** (caching, connection pooling, monitoring)

---

## 🎯 **Decision**

**Implement authentication and authorization as application middleware using dependency injection with Go interfaces.**

### **Key Design Principles**

1. **Interface-Based Design**: Define `Authenticator` and `Authorizer` interfaces
2. **Dependency Injection**: Inject implementations at runtime
3. **No Runtime Flags**: Authentication always enforced (no `AuthEnabled` flags)
4. **Test Doubles**: Use mocks for integration tests, real implementations for E2E/production
5. **Standard Kubernetes APIs**: TokenReview (authentication) + SAR (authorization)

---

## 🏗️ **Architecture**

### **Component Diagram**

```
┌─────────────────────────────────────────────────────────────┐
│                     HTTP Request                             │
│              (Authorization: Bearer <token>)                 │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
        ┌──────────────────────────────────┐
        │    Auth Middleware (Always On)   │
        │                                   │
        │  1. Extract Bearer token          │
        │  2. Authenticate (interface)      │
        │  3. Authorize (interface)         │
        │  4. Inject user into context      │
        └──────────────┬───────────────────┘
                       │
        ┌──────────────┴─────────────────┐
        │                                 │
        ▼                                 ▼
┌────────────────┐            ┌──────────────────┐
│ Authenticator  │            │   Authorizer     │
│  (Interface)   │            │   (Interface)    │
└────────┬───────┘            └────────┬─────────┘
         │                              │
    ┌────┴──────┐              ┌────────┴─────────┐
    │           │              │                  │
    ▼           ▼              ▼                  ▼
┌────────┐  ┌────────┐   ┌────────┐      ┌──────────┐
│  Real  │  │  Mock  │   │  Real  │      │   Mock   │
│  K8s   │  │ (Test) │   │  K8s   │      │  (Test)  │
│TokenRev│  │        │   │  SAR   │      │          │
└────────┘  └────────┘   └────────┘      └──────────┘
```

### **Interface Definitions**

```go
// pkg/shared/auth/interfaces.go

// Authenticator validates tokens and returns user identity
type Authenticator interface {
    // ValidateToken checks if the token is valid and returns the user identity
    // Returns username (e.g., "system:serviceaccount:ns:sa-name") or error
    ValidateToken(ctx context.Context, token string) (string, error)
}

// Authorizer checks if a user has permission to perform an action
type Authorizer interface {
    // CheckAccess verifies if the user has the required permissions
    // Returns true if allowed, false if denied, error on API failure
    CheckAccess(ctx context.Context, user, namespace, resource, resourceName, verb string) (bool, error)
}
```

---

## 🔧 **Implementation Details**

### **1. Production Implementation** (Real Kubernetes)

```go
// pkg/shared/auth/k8s_auth.go

type K8sAuthenticator struct {
    client kubernetes.Interface
}

func NewK8sAuthenticator(client kubernetes.Interface) *K8sAuthenticator {
    return &K8sAuthenticator{client: client}
}

func (a *K8sAuthenticator) ValidateToken(ctx context.Context, token string) (string, error) {
    review := &authenticationv1.TokenReview{
        Spec: authenticationv1.TokenReviewSpec{
            Token: token,
        },
    }
    
    result, err := a.client.AuthenticationV1().TokenReviews().Create(
        ctx, review, metav1.CreateOptions{},
    )
    if err != nil {
        return "", fmt.Errorf("token validation failed: %w", err)
    }
    
    if !result.Status.Authenticated {
        return "", errors.New("token not authenticated")
    }
    
    return result.Status.User.Username, nil
}

type K8sAuthorizer struct {
    client kubernetes.Interface
}

func NewK8sAuthorizer(client kubernetes.Interface) *K8sAuthorizer {
    return &K8sAuthorizer{client: client}
}

func (a *K8sAuthorizer) CheckAccess(ctx context.Context, user, namespace, resource, resourceName, verb string) (bool, error) {
    sar := &authorizationv1.SubjectAccessReview{
        Spec: authorizationv1.SubjectAccessReviewSpec{
            User: user,
            ResourceAttributes: &authorizationv1.ResourceAttributes{
                Namespace:    namespace,
                Resource:     resource,
                ResourceName: resourceName,
                Verb:         verb,
            },
        },
    }
    
    result, err := a.client.AuthorizationV1().SubjectAccessReviews().Create(
        ctx, sar, metav1.CreateOptions{},
    )
    if err != nil {
        return false, fmt.Errorf("authorization check failed: %w", err)
    }
    
    return result.Status.Allowed, nil
}
```

### **2. Test Implementation** (Integration Tests)

```go
// pkg/shared/auth/mock_auth_test.go

// MockAuthenticator is a test double for integration tests
type MockAuthenticator struct {
    // Map of token -> username
    ValidUsers map[string]string
    // Optional: simulate errors
    ErrorToReturn error
}

func (a *MockAuthenticator) ValidateToken(ctx context.Context, token string) (string, error) {
    if a.ErrorToReturn != nil {
        return "", a.ErrorToReturn
    }
    
    user, ok := a.ValidUsers[token]
    if !ok {
        return "", errors.New("invalid token")
    }
    
    return user, nil
}

// MockAuthorizer is a test double for integration tests
type MockAuthorizer struct {
    // Map of username -> allowed
    AllowedUsers map[string]bool
    // Optional: simulate errors
    ErrorToReturn error
}

func (a *MockAuthorizer) CheckAccess(ctx context.Context, user, namespace, resource, resourceName, verb string) (bool, error) {
    if a.ErrorToReturn != nil {
        return false, a.ErrorToReturn
    }
    
    allowed, exists := a.AllowedUsers[user]
    if !exists {
        return false, nil  // Default deny
    }
    
    return allowed, nil
}
```

### **3. Auth Middleware** (Service-Specific)

```go
// pkg/datastorage/middleware/auth.go

type AuthMiddleware struct {
    authenticator auth.Authenticator
    authorizer    auth.Authorizer
    config        AuthConfig
}

type AuthConfig struct {
    Namespace    string
    Resource     string
    ResourceName string
    Verb         string
}

func NewAuthMiddleware(authenticator auth.Authenticator, authorizer auth.Authorizer, config AuthConfig) *AuthMiddleware {
    return &AuthMiddleware{
        authenticator: authenticator,
        authorizer:    authorizer,
        config:        config,
    }
}

func (m *AuthMiddleware) Handler(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 1. Extract Bearer token
        authHeader := r.Header.Get("Authorization")
        if !strings.HasPrefix(authHeader, "Bearer ") {
            http.Error(w, "Unauthorized: missing or invalid Authorization header", http.StatusUnauthorized)
            return
        }
        token := strings.TrimPrefix(authHeader, "Bearer ")
        
        // 2. Authenticate (TokenReview)
        user, err := m.authenticator.ValidateToken(r.Context(), token)
        if err != nil {
            http.Error(w, "Unauthorized: token validation failed", http.StatusUnauthorized)
            return
        }
        
        // 3. Authorize (SAR)
        allowed, err := m.authorizer.CheckAccess(
            r.Context(),
            user,
            m.config.Namespace,
            m.config.Resource,
            m.config.ResourceName,
            m.config.Verb,
        )
        if err != nil {
            http.Error(w, "Internal Server Error: authorization check failed", http.StatusInternalServerError)
            return
        }
        if !allowed {
            http.Error(w, "Forbidden: insufficient permissions", http.StatusForbidden)
            return
        }
        
        // 4. Inject user identity into request context (for audit logging)
        ctx := context.WithValue(r.Context(), "user", user)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

---

## 🧪 **Testing Strategy**

### **Unit Tests** (pkg/shared/auth/, pkg/datastorage/middleware/)

**Test middleware behavior with mocks**:

```go
Describe("AuthMiddleware", func() {
    var (
        authenticator *MockAuthenticator
        authorizer    *MockAuthorizer
        middleware    *AuthMiddleware
    )
    
    BeforeEach(func() {
        authenticator = &MockAuthenticator{
            ValidUsers: map[string]string{
                "valid-token": "system:serviceaccount:test:authorized-sa",
            },
        }
        authorizer = &MockAuthorizer{
            AllowedUsers: map[string]bool{
                "system:serviceaccount:test:authorized-sa": true,
            },
        }
        middleware = NewAuthMiddleware(authenticator, authorizer, config)
    })
    
    It("should reject request without token", func() {
        req := httptest.NewRequest("POST", "/api/v1/workflows", nil)
        resp := httptest.NewRecorder()
        
        middleware.Handler(nextHandler).ServeHTTP(resp, req)
        
        Expect(resp.Code).To(Equal(401))
    })
    
    It("should reject invalid token", func() {
        req := httptest.NewRequest("POST", "/api/v1/workflows", nil)
        req.Header.Set("Authorization", "Bearer invalid-token")
        resp := httptest.NewRecorder()
        
        middleware.Handler(nextHandler).ServeHTTP(resp, req)
        
        Expect(resp.Code).To(Equal(401))
    })
    
    It("should reject unauthorized user", func() {
        authenticator.ValidUsers["unauthorized-token"] = "system:serviceaccount:test:unauthorized-sa"
        authorizer.AllowedUsers["system:serviceaccount:test:unauthorized-sa"] = false
        
        req := httptest.NewRequest("POST", "/api/v1/workflows", nil)
        req.Header.Set("Authorization", "Bearer unauthorized-token")
        resp := httptest.NewRecorder()
        
        middleware.Handler(nextHandler).ServeHTTP(resp, req)
        
        Expect(resp.Code).To(Equal(403))
    })
    
    It("should allow authorized user", func() {
        req := httptest.NewRequest("POST", "/api/v1/workflows", nil)
        req.Header.Set("Authorization", "Bearer valid-token")
        resp := httptest.NewRecorder()
        
        middleware.Handler(nextHandler).ServeHTTP(resp, req)
        
        Expect(resp.Code).To(Equal(200))
    })
})
```

### **Integration Tests** (envtest)

**Inject mocks - auth still enforced**:

```go
// test/integration/datastorage/suite_test.go

BeforeSuite(func() {
    // Start envtest
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{"../../../config/crd/bases"},
    }
    k8sConfig, err := testEnv.Start()
    Expect(err).ToNot(HaveOccurred())
    
    // Integration tests: Use mocks (auth still enforced!)
    authenticator := &auth.MockAuthenticator{
        ValidUsers: map[string]string{
            "test-token-authorized": "system:serviceaccount:test:authorized-sa",
            "test-token-readonly":   "system:serviceaccount:test:readonly-sa",
        },
    }
    
    authorizer := &auth.MockAuthorizer{
        AllowedUsers: map[string]bool{
            "system:serviceaccount:test:authorized-sa": true,
            "system:serviceaccount:test:readonly-sa":   false,
        },
    }
    
    // Start DataStorage with injected mocks
    dsServer := datastorage.NewServer(datastorage.Config{
        Authenticator: authenticator,
        Authorizer:    authorizer,
        K8sConfig:     k8sConfig,
    })
})

// Tests provide tokens - auth is validated
It("should create workflow with authorized token", func() {
    req := dsgen.NewCreateWorkflowRequest(workflow)
    req.Header.Set("Authorization", "Bearer test-token-authorized")
    
    resp, err := client.CreateWorkflow(ctx, req)
    Expect(err).ToNot(HaveOccurred())
    Expect(resp.StatusCode).To(Equal(201))
})

It("should reject workflow with readonly token", func() {
    req := dsgen.NewCreateWorkflowRequest(workflow)
    req.Header.Set("Authorization", "Bearer test-token-readonly")
    
    resp, err := client.CreateWorkflow(ctx, req)
    Expect(resp.StatusCode).To(Equal(403))
})
```

### **E2E Tests** (Kind)

**Use real Kubernetes auth - full validation**:

```go
// test/e2e/datastorage/23_sar_access_control_test.go

BeforeSuite(func() {
    // Create Kind cluster
    // Deploy DataStorage with REAL K8s authenticator/authorizer
    
    // DataStorage automatically uses:
    // authenticator := auth.NewK8sAuthenticator(k8sClient)
    // authorizer := auth.NewK8sAuthorizer(k8sClient)
})

It("should allow authorized ServiceAccount to write audit events", func() {
    // Get real token from Kind cluster
    token, err := infrastructure.GetServiceAccountToken(
        ctx,
        "datastorage-e2e",
        "authorized-sa",
        kubeconfigPath,
    )
    Expect(err).ToNot(HaveOccurred())
    
    // Create workflow with real token
    req := dsgen.NewCreateWorkflowRequest(workflow)
    req.Header.Set("Authorization", "Bearer "+token)
    
    // DataStorage validates with real TokenReview + SAR
    resp, err := client.CreateWorkflow(ctx, req)
    Expect(err).ToNot(HaveOccurred())
    Expect(resp.StatusCode).To(Equal(201))
})

It("should reject unauthorized ServiceAccount", func() {
    token, err := infrastructure.GetServiceAccountToken(
        ctx,
        "datastorage-e2e",
        "unauthorized-sa",  // No RBAC permissions
        kubeconfigPath,
    )
    Expect(err).ToNot(HaveOccurred())
    
    req := dsgen.NewCreateWorkflowRequest(workflow)
    req.Header.Set("Authorization", "Bearer "+token)
    
    resp, err := client.CreateWorkflow(ctx, req)
    Expect(resp.StatusCode).To(Equal(403))
})
```

---

## 🔐 **Security Considerations**

### **1. No Runtime Disable Flags** ✅

**Problem**: Having `AuthEnabled: false` is a security vulnerability:
```go
// ❌ DANGEROUS - could be accidentally enabled in production
if !m.authEnabled {
    next.ServeHTTP(w, r)  // Skip auth
    return
}
```

**Solution**: Auth is **always enforced** via dependency injection:
```go
// ✅ SECURE - auth always runs, only implementation varies
user, err := m.authenticator.ValidateToken(ctx, token)
// No bypass possible
```

### **2. Interface-Based Testing** ✅

- **Production**: Real Kubernetes TokenReview + SAR
- **Integration**: Mock implementations (still validates flow)
- **E2E**: Real Kubernetes TokenReview + SAR
- **Same middleware code** in all environments

### **3. Defense in Depth** ✅

Multiple layers of validation:
1. HTTP Authorization header presence
2. Token format validation (Bearer prefix)
3. TokenReview API call (authentication)
4. SAR API call (authorization)
5. RBAC policy evaluation (Kubernetes)

### **4. Audit Trail** ✅

User identity injected into request context:
```go
ctx := context.WithValue(r.Context(), "user", user)
// Available for audit event logging (SOC2 CC8.1)
```

### **5. Error Handling** ✅

- 401 Unauthorized: Token validation fails
- 403 Forbidden: SAR denies access
- 500 Internal Server Error: TokenReview/SAR API errors

---

## 📊 **Comparison: Proxy vs Middleware**

| Feature | ose-oauth-proxy (Current) | Middleware (DD-AUTH-014) |
|---------|---------------------------|--------------------------|
| **OpenShift Dependency** | ✅ Required | ❌ Not required |
| **Works in Kind (E2E)** | ❌ No (requires OpenShift) | ✅ Yes (vanilla K8s) |
| **Works in envtest (Integration)** | ❌ Can't test | ✅ Yes (with mocks) |
| **Complexity** | High (sidecar, ports, config) | Low (application code) |
| **Debugging** | Hard (2 containers) | Easy (single codebase) |
| **Control** | External (proxy) | Internal (application) |
| **Security** | ✅ Good | ✅ Good |
| **Performance** | Extra network hop | Direct (no proxy) |
| **Portability** | OpenShift only | Any Kubernetes |
| **Testability** | Limited | Full coverage |

---

## 🚀 **Implementation Plan**

### **Phased Rollout Strategy** 🎯

**Rationale**: 
- Validate approach in DataStorage first (proof-of-concept)
- Measure API server impact before expanding
- Evaluate Gateway E2E tests for high-throughput scenarios
- Then decide: expand to HAPI only, or all services

### **Phase 1: Core Infrastructure** (1 day)

**Goal**: Build reusable auth framework

1. Create `pkg/shared/auth/interfaces.go`
   - Define `Authenticator` interface
   - Define `Authorizer` interface
   - Document interface contracts

2. Create `pkg/shared/auth/k8s_auth.go`
   - Implement `K8sAuthenticator` (TokenReview)
   - Implement `K8sAuthorizer` (SAR)
   - Add connection pooling
   - Add basic retry logic

3. Create `pkg/shared/auth/mock_auth_test.go`
   - Implement `MockAuthenticator`
   - Implement `MockAuthorizer`
   - Unit tests for mocks

4. Create `pkg/shared/auth/cached_auth.go` (optional optimization)
   - Implement `CachedAuthenticator` (wraps K8sAuthenticator)
   - Token cache with 5-minute TTL
   - Performance metrics

### **Phase 2: DataStorage POC** (2-3 days) ⭐ **START HERE**

**Goal**: Prove the approach works, measure API server impact, completely remove oauth-proxy

**Implementation**:

1. Create `pkg/datastorage/middleware/auth.go`
   - Implement `AuthMiddleware` with dependency injection
   - No disable flags (security requirement)
   - Apply to all routes

2. Update `cmd/datastorage/main.go`
   - Inject `K8sAuthenticator` + `K8sAuthorizer` (production)
   - Apply middleware to HTTP router
   - **Remove all oauth-proxy references**

3. Update integration tests
   - Inject `MockAuthenticator` + `MockAuthorizer`
   - Update test assertions (expect 401/403)
   - Verify auth is enforced (no bypass)

4. Update E2E tests
   - **Remove `ose-oauth-proxy` from `test/infrastructure/datastorage.go`**
   - Remove sidecar container definition
   - Update service ports (direct to 8081)
   - Use direct DataStorage access with real tokens
   - Validate TokenReview + SAR in Kind

5. Update deployment manifests
   - **Remove `ose-oauth-proxy` sidecar from `deploy/data-storage/deployment.yaml`**
   - Remove oauth-proxy container definition
   - Remove oauth-proxy secrets/volumes
   - Update health check paths (direct to service)
   - Update service ports (8081 direct, no proxy)

**Performance Validation**:
- Measure TokenReview/SAR API call rates
- Monitor API server load during E2E tests
- Collect latency metrics (p50, p95, p99)
- Document findings in DD-AUTH-014 addendum

### **Phase 3: Decision Point** 🔀 (1 day)

**Goal**: Evaluate DataStorage POC results and decide next steps

**Evaluation Criteria**:
- ✅ All DataStorage tests pass (unit, integration, E2E)
- ✅ API server load acceptable (< 100 req/s during E2E)
- ✅ Latency acceptable (p95 < 500ms)
- ✅ No rate limiting issues
- ✅ oauth-proxy completely removed

**Decision Options**:

**Option A**: Expand to HAPI only (targeted rollout)
- Apply to HolmesGPT API (similar traffic patterns to DataStorage)
- Keep Gateway/Notification with existing auth (if needed)
- **Recommended if API server shows stress**

**Option B**: Expand to all REST API services (full rollout)
- Apply to HAPI, Notification, Gateway
- Standardize auth across all services
- **Recommended if POC shows no issues**

**Option C**: Rollback and re-evaluate
- Keep oauth-proxy for now
- Investigate alternative approaches (e.g., service mesh)
- **Only if POC fails validation**

### **Phase 3: HolmesGPT API** ✅ **COMPLETE** (2 days)

**Goal**: Apply proven pattern to HAPI

**Status**: ✅ Implementation complete (January 2026)

1. ✅ Created `pkg/kubernaut-agent/middleware/auth.go`
   - Reused `pkg/shared/auth` interfaces
   - Service-specific SAR configuration

2. ✅ Updated `kubernaut-agent/main.py`
   - Added auth middleware to FastAPI app
   - Configured SAR parameters
   - **Removed oauth-proxy from Python app**

3. ✅ Updated tests (same pattern as DataStorage)
   - Integration: Real envtest with K8s auth
   - E2E: Real Kind cluster with K8s auth

4. ✅ Updated deployment
   - **Removed oauth-proxy sidecar**
   - Updated manifests

### **Phase 4: Gateway Service** 🚧 **IN PROGRESS** (3-4 days)

**Goal**: Secure external-facing entry point with SAR authentication

**Status**: 🚧 Implementation in progress (January 29, 2026)

**Rationale for Gateway SAR Auth**:
1. **Security**: Gateway is external-facing (Prometheus AlertManager, K8s Event forwarders)
2. **Zero-Trust**: Network Policies alone insufficient (DD-GATEWAY-006 superseded)
3. **SOC2 Compliance**: Need operator attribution for signal injection (REQ-3)
4. **Webhook Support**: AlertManager + K8s Events already support Bearer tokens

**Performance Considerations**:
- ✅ Low throughput: <100 signals/min in most deployments
- ✅ No caching needed: NetworkPolicy reduces unauthorized traffic
- ✅ Proven pattern: Same as DataStorage/HAPI (validated)

**Implementation Tasks**:
1. 🚧 Create `pkg/gateway/middleware/auth.go`
   - Reuse `pkg/shared/auth` interfaces (K8sAuthenticator, K8sAuthorizer)
   - Apply to `/webhook/*` routes only
   - Extract user identity for audit events

2. 🚧 Update `pkg/gateway/server.go`
   - Add k8sClient parameter to NewServer()
   - Instantiate AuthMiddleware
   - Inject authenticated user into audit events (ActorID)

3. 🚧 Update Business Requirements
   - **BR-GATEWAY-182**: ServiceAccount Authentication (TokenReview)
   - **BR-GATEWAY-183**: SubjectAccessReview Authorization

4. 🚧 Update tests
   - Integration: Real envtest with ServiceAccounts + RBAC
   - E2E: Real Kind cluster with ServiceAccount tokens

5. 🚧 Update deployment docs
   - Webhook configuration examples (Bearer tokens)
   - RBAC requirements
   - ServiceAccount setup guide

**Decision**: ✅ **APPROVED** - Gateway auth/authz required for production security

### **Phase 6: Documentation & Rollout** (1 day)

1. Update documentation
   - Migration guide for other services
   - Performance tuning guide
   - Troubleshooting runbook

2. Mark superseded documents
   - DD-AUTH-011 (RBAC with proxy) → Superseded by DD-AUTH-014
   - DD-AUTH-012 (ose-oauth-proxy) → Superseded by DD-AUTH-014

3. Production rollout checklist
   - Staging validation
   - Gradual rollout (canary)
   - Monitoring dashboards
   - Rollback plan

---

## 📈 **Success Metrics**

### **Functional**
- ✅ **DataStorage**: Authenticates using TokenReview API (Complete)
- ✅ **HAPI**: Authenticates using TokenReview API (Complete)
- 🚧 **Gateway**: Authentication in progress (January 2026)
- ✅ All services authorize using SAR API
- ✅ User identity captured for audit logging (SOC2 CC8.1 compliance)

### **Testing**
- ✅ **DataStorage**: 100% auth middleware coverage (Unit + Integration + E2E)
- ✅ **HAPI**: Full auth flow validated in integration + E2E
- 🚧 **Gateway**: Tests pending (envtest + Kind)
- ✅ Integration tests: Real K8s auth with envtest (not mocks)
- ✅ E2E tests: Real K8s auth in Kind cluster

### **Operational**
- ✅ **DataStorage**: oauth-proxy removed (Single container deployment)
- ✅ **HAPI**: oauth-proxy removed (Simplified debugging)
- 🚧 **Gateway**: Network Policies replaced with SAR auth (In progress)
- ✅ Portable across Kubernetes distributions (OpenShift, vanilla K8s)
- ✅ Reduced K8s API load: No issues observed with real auth in DS/HAPI

---

## 🔗 **Related Documents**

### **Superseded Documents**
- **DD-AUTH-011**: Granular RBAC & SAR Verb Mapping → Superseded by DD-AUTH-014
- **DD-AUTH-012**: ose-oauth-proxy for SAR → Superseded by DD-AUTH-014
- **DD-GATEWAY-006**: Gateway Network Policies Only → Superseded by DD-AUTH-014 V2.0 (Gateway now requires SAR)
- **ADR-036**: Authentication Strategy → Exception: Gateway now requires SAR despite original decision

### **Complementary Documents**
- **DD-AUTH-013**: HTTP Status Codes for Auth Errors (401/403 handling)
- **DD-TEST-012**: Envtest Real Authentication Pattern (integration test strategy)

### **Business Requirements**
- **BR-SECURITY-016**: Kubernetes RBAC enforcement for REST API endpoints
- **BR-SECURITY-017**: ServiceAccount token authentication
- **BR-GATEWAY-182**: Gateway ServiceAccount Authentication (NEW - January 2026)
- **BR-GATEWAY-183**: Gateway SubjectAccessReview Authorization (NEW - January 2026)

---

## 📝 **Decision Rationale**

### **Why Middleware Over Proxy?**

1. **Portability**: Works on any Kubernetes (not just OpenShift)
2. **Testability**: Can test auth in all tiers (unit, integration, E2E)
3. **Simplicity**: No sidecar containers, simpler deployment
4. **Control**: Application owns auth logic
5. **Debugging**: Single codebase, unified logs

### **Why Dependency Injection?**

1. **Security**: No runtime disable flags (auth always enforced)
2. **Testability**: Use mocks without modifying production code
3. **Type Safety**: Interfaces enforce correct signatures
4. **Flexibility**: Swap implementations without code changes

### **Why Not Skip Auth in Tests?**

- ❌ Security risk (accidental production bypass)
- ❌ Inconsistent code paths (test vs production)
- ✅ Dependency injection provides safe testing

---

## ⚠️ **Risks & Mitigations**

### **Critical Concern: API Server Load** 🚨

**Problem**: Every authenticated request hits the Kubernetes API server twice:
1. TokenReview API call (authentication)
2. SubjectAccessReview API call (authorization)

**Impact**:
- High-throughput services (e.g., Gateway processing alerts) could overload API server
- API server rate limiting may throttle legitimate requests
- Increased latency (200-500ms per request)

**Mitigations**:

| Risk | Mitigation Strategy | Implementation |
|------|---------------------|----------------|
| **API server overload** | Token caching (5-minute TTL) | Cache TokenReview results by token hash |
| **Rate limiting** | Connection pooling + backoff | Use K8s client-go with rate limiter |
| **High latency** | Async validation + circuit breaker | Fall back on repeated failures |
| **SAR storm** | SAR result caching (user+resource) | Cache allowed/denied decisions |
| **Migration risk** | Phased rollout (DataStorage POC first) | Validate performance before expanding |

### **Performance Optimization Strategy**

```go
// Example: Token cache (5-minute TTL)
type CachedAuthenticator struct {
    delegate Authenticator
    cache    *ttlcache.Cache  // token -> (user, exp)
}

func (a *CachedAuthenticator) ValidateToken(ctx context.Context, token string) (string, error) {
    // Check cache first (avoid API call)
    if cached, ok := a.cache.Get(tokenHash(token)); ok {
        return cached.User, nil
    }
    
    // Cache miss - call real API
    user, err := a.delegate.ValidateToken(ctx, token)
    if err != nil {
        return "", err
    }
    
    // Cache for 5 minutes
    a.cache.Set(tokenHash(token), user, 5*time.Minute)
    return user, nil
}
```

### **Gateway E2E Evaluation** ⚠️

**Concern**: Gateway E2E tests process high volumes of alerts and may stress API server.

**Action Required**:
- Evaluate Gateway E2E tests after DataStorage POC
- Measure TokenReview/SAR call rates
- Consider rate limiting or test-specific optimizations
- May need different caching strategy for high-throughput services

### **Other Risks**

| Risk | Mitigation |
|------|------------|
| **Auth logic in app** (more responsibility) | Well-tested middleware, reusable across services |
| **TokenReview/SAR API failures** | Retry logic, circuit breakers, fail-safe defaults |
| **Migration complexity** | Phased rollout, run proxy + middleware in parallel initially |

---

## ✅ **Acceptance Criteria**

- [ ] Interfaces defined in `pkg/shared/auth/`
- [ ] Real implementations use Kubernetes APIs (TokenReview, SAR)
- [ ] Mock implementations for integration tests
- [ ] Auth middleware applies to all service routes
- [ ] No runtime disable flags in production code
- [ ] Unit tests: 100% middleware coverage
- [ ] Integration tests: Auth validated with mocks
- [ ] E2E tests: Full auth flow in Kind
- [ ] Documentation updated (deployment, ADRs, runbooks)
- [ ] Migration guide for other services

---

## 🎯 **Next Steps**

### **Immediate Actions** (This Week)

1. ✅ **Review & Approve**: Architecture team review this DD
2. ⭐ **Phase 1**: Implement core infrastructure (`pkg/shared/auth/`)
3. ⭐ **Phase 2**: DataStorage POC (complete oauth-proxy removal)
4. 📊 **Validate**: Measure API server impact during E2E tests
5. 🔀 **Phase 3**: Decision point based on POC results

### **DataStorage POC Success Criteria**

- [ ] All tests pass (unit, integration, E2E)
- [ ] oauth-proxy completely removed from DataStorage
- [ ] API server load measured and acceptable
- [ ] Latency metrics within tolerance (p95 < 500ms)
- [ ] No rate limiting issues observed
- [ ] Auth validated in all environments (Kind, envtest, production)

### **Follow-Up Actions** (After POC)

- **If successful**: Proceed to Phase 4 (HAPI implementation)
- **If API server stress**: Implement caching optimizations, re-test
- **If issues**: Evaluate Gateway E2E impact before expanding
- **Final decision**: Expand to all services OR targeted rollout only

---

**Document Version**: 1.0  
**Last Updated**: January 26, 2026  
**Status**: Proposed (Awaiting Architecture Review)  
**Author**: AI Assistant + Engineering Team
