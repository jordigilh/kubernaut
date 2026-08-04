# DD-AUTH-014: Middleware-Based SAR Authentication (Interface-Driven)

**Status**: Approved  
**Version**: 4.2  
**Date**: August 4, 2026  
**Decision Makers**: Architecture Team  
**Affected Services**: 
- **Phase 2 (POC)**: ✅ DataStorage (Complete)
- **Phase 3**: ✅ Kubernaut Agent (KA) (Complete)
- **Phase 4**: ✅ Gateway (Complete - January 2026)
- **Phase 5**: ✅ AIAnalysis Controller (Complete - January 2026)
- **Phase 7**: ❌ Audience-bound `TokenReview` for AF + KA — implemented and fully reverted (August 2026), deferred to [#1900](https://github.com/jordigilh/kubernaut/issues/1900) for v1.6. Only the KA SOC2 audit-reason enrichment side-effect shipped (#1909).
- **Future**: Notification, other REST API services (TBD)

---

## 📋 **Changelog**

### Version 4.2 (August 4, 2026) — AF audience-binding also reverted; fully deferred to v1.6 (#1900)
- **REMOVED**: API Frontend's `tokenReviewAudiences` audience-bound `TokenReview` option (`apifrontend.auth.tokenReviewAudiences`, `pkg/apifrontend/auth/tokenreview.go`'s `WithExpectedAudiences`/`TokenReviewerOption`/`audiencesIntersect`, and the `buildTokenReviewFallback` wiring in `cmd/apifrontend/auth_wiring.go`) — full symmetry with the v4.1 KA revert
- **WHY**: AF's audience check had no structural blocker like KA's (proven working against a real `kube-apiserver` via envtest, `IT-AF-1900-001/002`), but on further review its incremental value was unclear: AF already gates all tool/action use via `SubjectAccessReview` (`pkg/apifrontend/auth/sar.go`), so an audience-mismatched-but-authenticated caller is already denied at the authorization layer regardless of this check — its value is authentication-layer defense-in-depth only, narrower than "closes a replay hole" as originally framed. Additionally, shipping AF's half alone would not have unblocked this issue's original motivation ([kubernaut-operator#139](https://github.com/jordigilh/kubernaut-operator/issues/139)), since that plan depends on KA's half, which the v4.1 finding showed is structurally unbuildable
- **RESULT**: Neither AF nor KA implements audience-bound `TokenReview` today. [#1900](https://github.com/jordigilh/kubernaut/issues/1900) remains open (not closed by #1909), targeting v1.6, carrying the full investigation
- **UNAFFECTED**: The SOC2/FedRAMP audit-reason enrichment (kept in v4.1) ships as-is — it is generic to any classified auth-failure reason, not audience-specific
- **ALSO FILED**: [#1919](https://github.com/jordigilh/kubernaut/issues/1919) / [kubernaut-console#48](https://github.com/jordigilh/kubernaut-console/issues/48) — a related but distinct authorization gap found during this review: the console UI renders unconditionally once OIDC auth succeeds, with no pre-render authorization check (per-tool SAR only runs once the user attempts an action). Proposed fix is a dedicated coarse-grained role/synthetic resource (`kubernaut.ai/console`, verb `use`), not an audience check — tracked separately for v1.6, out of scope here
- **SEE**: [BR-SECURITY-1900](../../requirements/BR-SECURITY-1900-audience-bound-tokenreview.md) § "Fully Deferred to v1.6" for the full architectural analysis of both services

### Version 4.1 (August 3, 2026) — KA audience-binding descoped (#1900)
- **REMOVED**: Kubernaut Agent's `expectedAFAudience` audience-bound `TokenReview` option (`auth.expectedAFAudience`, `k8sAuthenticatorOptions`/`newAuthMiddleware` wiring in `cmd/kubernautagent/routes.go`, and the `WithExpectedAudiences` functional option on the shared `pkg/shared/auth.K8sAuthenticator`)
- **WHY**: KA's real AF-originated traffic flows exclusively through the `/api/v1/mcp` endpoint, which DD-AUTH-MCP-001 requires to permanently serve both AF-delegated calls *and* arbitrary direct in-cluster K8s clients on the same authenticator — the latter can never be forced to carry a `kubernaut-agent`-audience token, since they are independent K8s principals KA does not mint tokens for. The one KA code path that *could* safely be audience-bound (`pkg/apifrontend/ka.Client`'s `Analyze/Status/Result/Cancel` REST methods) has zero production callers (`deps.KAClient` in `cmd/apifrontend/backend_deps.go` is only ever used for `.Healthy()`, a local circuit-breaker check with no network call) — so binding it protects nothing live. Attempting to ship the option anyway (even Helm-unset/opt-in) either breaks every interactive MCP session the moment an operator enables it, or sits unwired as orphaned `pkg/` code with no real caller, violating CHECKPOINT C (Business Integration Validation)
- **KEPT (at the time — later also reverted in v4.2, see above)**: AF's own audience-bound `TokenReview` check (`apifrontend.auth.tokenReviewAudiences`, `pkg/apifrontend/auth/tokenreview.go`'s `TokenReviewer.expectedAudiences`) was unaffected by this KA-only finding — AF's `TokenReview` fallback authenticates callers presenting tokens directly *to AF*; it has no equivalent "one endpoint serves two structurally different populations" constraint, so requiring `Spec.Audiences: ["kubernaut-apifrontend"]` there didn't collide with anything else AF does
- **KEPT**: The SOC2/FedRAMP audit-reason enrichment (`pkg/shared/auth.WithFailureReasonCapture`, KA's `AuditAuthMiddleware` persisting `Data["reason"]` into `AIAgentAuthFailurePayload`/`AIAgentAuthDeniedPayload`) — this is generic to *any* auth-failure reason (`missing_auth_header`, `invalid_auth_format`, `empty_bearer_token`, `invalid_token`, `authorization_denied`), so it stands on its own merit independent of whether KA ever gets an audience check
- **SEE**: [BR-SECURITY-1900](../../requirements/BR-SECURITY-1900-audience-bound-tokenreview.md) § "KA Audience-Binding: Descoped" for the full architectural analysis

### Version 4.0 (August 3, 2026) — Audience-Bound TokenReview (#1900)
- **ADDED**: Optional audience-bound `TokenReview` validation for API Frontend and Kubernaut Agent (BR-SECURITY-1900), see [§ Audience-Bound TokenReview](#-audience-bound-tokenreview-br-security-1900) below
- **PROBLEM**: `K8sAuthenticator`/`TokenReviewer` requested no `Spec.Audiences`, so a ServiceAccount token minted for one service (e.g. AF) was accepted just as readily by a different service (e.g. KA) — a cross-service token-replay path
- **SOLUTION**: `WithExpectedAudiences(...)` functional option on both `K8sAuthenticator` (`pkg/shared/auth`) and `TokenReviewer` (`pkg/apifrontend/auth`); sets `Spec.Audiences` on the outbound request (server-side enforcement) AND independently verifies `Status.Audiences` intersects the expected set (defense-in-depth against an audience-unaware API server)
- **BACKWARD COMPATIBLE**: No audience configured → behavior unchanged (opt-in hardening, not a breaking change); confirmed zero behavior change for DataStorage/Gateway, which call `NewK8sAuthenticator` with no options
- **CONFIG**: `apifrontend.auth.tokenReviewAudiences` (AF), `kubernautagent.auth.expectedAFAudience` (KA) — **both fields were removed, KA's in v4.1 and AF's in v4.2 above, see rationale there**
- **AUDIT**: `pkg/shared/auth/middleware.go`'s `security_event` log now distinguishes `reason=invalid_token_audience` from the generic `invalid_token` (SOC2 AU-3/CC7.2); AF's existing `classifyAuthError` already mapped `ErrInvalidAudience` to `failure_reason=invalid_audience`
- **TESTED**: Unit (fake clientset) + Integration (envtest, real API server) + E2E (real Kind cluster) for both AF and KA — see BR-SECURITY-1900 for the full test matrix
- **NEW BR**: [BR-SECURITY-1900](../../requirements/BR-SECURITY-1900-audience-bound-tokenreview.md)
- **OUT OF SCOPE**: Extending audience binding to DataStorage/Gateway; a fail-closed enforcement mode that treats an audience-unaware API server itself as a hard failure (deferred, tracked separately per the same "don't scope-creep the fix" reasoning used for DD-AUTH-016's fail-closed alternative)

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

## 🔒 **Audience-Bound TokenReview (BR-SECURITY-1900) — Reverted, Deferred to v1.6**

**Added in v4.0, KA scope removed in v4.1, AF scope removed in v4.2 (#1900, deferred to [#1900](https://github.com/jordigilh/kubernaut/issues/1900) for v1.6).** The base implementation above requests `TokenReview` with no `Spec.Audiences`, so any Kubernaut service trusting the cluster's `TokenReview` API accepts any token that authenticates — including one minted for a *different* Kubernaut service. This section documents an opt-in mechanism, implemented for both AF and KA and then fully reverted for both, that would scope a `TokenReview`-validating service to only accept tokens minted for it. **Neither AF nor KA implements this today** — see [§ Why It Was Fully Reverted](#why-it-was-fully-reverted) below. The code samples below are retained as historical reference for whoever picks up [#1900](https://github.com/jordigilh/kubernaut/issues/1900) for v1.6.

### Mechanism (as implemented, then reverted)

```go
// pkg/apifrontend/auth/tokenreview.go (AF's dedicated TokenReviewer) -- REVERTED, no longer in the codebase
reviewer := auth.NewTokenReviewer(clientset, auth.WithExpectedAudiences("apifrontend"))
```

When configured:
1. `Spec.Audiences` is set on the outbound `TokenReview` request, so an audience-aware API server enforces the match **server-side** (the primary defense — the K8s API server itself is authoritative on which audience a presented token was minted for).
2. The response's `Status.Audiences` is independently checked (`audiencesIntersect`) against the expected set. This is defense-in-depth: an audience-*unaware* API server (one that ignores `Spec.Audiences` and returns `Authenticated: true` with empty `Status.Audiences`) must not be mistaken for one that enforced the requested scoping.

### Token Minting Side (TokenRequest)

The caller minting the token would request the same audience via the `TokenRequest` API:

```go
tokenReq := &authenticationv1.TokenRequest{
    Spec: authenticationv1.TokenRequestSpec{
        Audiences:         []string{"apifrontend"},
        ExpirationSeconds: ptr(int64(3600)),
    },
}
```

A token minted with a different audience and then replayed against a `TokenReviewer` configured with `WithExpectedAudiences("apifrontend")` would be rejected — the cross-service replay this BR was meant to defend against.

### Threat Model Boundary

Audience binding defends against a validly-minted token being **replayed across a trust boundary** it wasn't scoped for. It does **not** defend against a compromised token-issuing authority: a service with legitimate `TokenRequest` permissions that has itself been compromised can simply mint a token scoped to any audience it likes, including the victim's. This is the same boundary documented in [DD-AUTH-016](./DD-AUTH-016-signed-user-identity-delegation.md)'s threat model for the analogous AF-minted identity JWT — audience/signature binding raises the bar for credential misuse, it does not substitute for protecting the minting authority itself.

### Audit Trail

KA's shared `pkg/shared/auth/middleware.go` retains generic `security_event` reason classification (`invalidTokenReason`, `WithFailureReasonCapture`) that would recognize an `ErrTokenInvalid` wrapping an "audience mismatch" substring as `reason=invalid_token_audience` — this mechanism ships regardless as forward-compatible plumbing (it also correctly classifies `missing_auth_header`, `empty_bearer_token`, `authorization_denied`, etc., all of which KA does produce today) — but since **neither** KA nor AF has an audience-checking authenticator anymore, it has no live producer and will only ever be exercised by tests constructing a synthetic error string.

### Configuration

Not configurable today — both `apifrontend.auth.tokenReviewAudiences` and `kubernautagent.auth.expectedAFAudience` were removed along with the reverted mechanism. If re-implemented for v1.6, the same field names/shapes remain a reasonable starting point.

### Why It Was Fully Reverted

Kubernaut Agent originally got the identical mechanism (`expectedAFAudience` → the shared `K8sAuthenticator`'s `WithExpectedAudiences`), but it was removed after tracing where AF's *real* traffic to KA actually flows in production wiring (`cmd/apifrontend/backend_deps.go`):

- **100% of AF's live traffic to KA goes through `/api/v1/mcp`** (`ka.NewSDKMCPClient`/`ka.NewKASessionPool`, the trusted-intermediary pattern). [DD-AUTH-MCP-001](./DD-AUTH-MCP-001-mcp-endpoint-security.md) documents this endpoint as *permanently* serving two populations on the same authenticator: AF acting as a delegate, and arbitrary direct in-cluster K8s clients. The direct-client half can never be forced to carry a `kubernaut-agent`-audience token — those are independent K8s principals, not something KA or AF mints tokens for — so binding this endpoint's authenticator would either break every interactive MCP session the moment it's enabled, or require abandoning the documented dual-client requirement.
- **The one path that *could* safely be audience-bound has no real traffic.** `deps.KAClient` (`pkg/apifrontend/ka.Client`, exposing `Analyze/Status/Result/Cancel` over KA's plain REST API) is constructed in production wiring, but the only production call site (`cmd/apifrontend/mcp_a2a_handlers.go`) only ever calls `.Healthy()` — a local circuit-breaker check with no network call. `Analyze/Status/Result/Cancel` have zero callers outside test files; it's dead code from a pre-MCP design.

Splitting KA's authenticator into a REST-only audience-checked path and an MCP audience-unaware path (an initially-proposed middle ground) does not change this conclusion — the REST path it would protect isn't used for anything real, and the MCP path it can't touch is the only one that is. Shipping the option regardless would leave `pkg/shared/auth.K8sAuthenticator`'s `WithExpectedAudiences` option with no genuine production caller anywhere (DS/GW don't use it either), which fails CHECKPOINT C (Business Integration Validation: no orphaned `pkg/` code without a real caller).

AF's own audience check had no such structural blocker — it guards tokens presented directly *to AF*, with no "one endpoint serves two structurally different populations" constraint — and was proven working against a real `kube-apiserver` via envtest. It was reverted anyway (v4.2) on further review: AF already gates all tool/action use via `SubjectAccessReview` (`pkg/apifrontend/auth/sar.go`), so an audience-mismatched-but-authenticated caller is already denied at the authorization layer independent of this check. Its remaining value is authentication-layer defense-in-depth only — real, but narrower than "closes a replay hole" as originally framed, since SAR already closes the practical exploitation path. Shipping AF's half alone also wouldn't unblock this issue's original motivation ([kubernaut-operator#139](https://github.com/jordigilh/kubernaut-operator/issues/139)'s custom-audience projected-token plan), since that depends on KA's half, which remains structurally unbuildable per the findings above. Both are deferred together to [#1900](https://github.com/jordigilh/kubernaut/issues/1900) for v1.6, to be revisited as one coherent decision.

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
- Then decide: expand to KA only, or all services

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

**Option A**: Expand to KA only (targeted rollout)
- Apply to Kubernaut Agent (similar traffic patterns to DataStorage)
- Keep Gateway/Notification with existing auth (if needed)
- **Recommended if API server shows stress**

**Option B**: Expand to all REST API services (full rollout)
- Apply to KA, Notification, Gateway
- Standardize auth across all services
- **Recommended if POC shows no issues**

**Option C**: Rollback and re-evaluate
- Keep oauth-proxy for now
- Investigate alternative approaches (e.g., service mesh)
- **Only if POC fails validation**

### **Phase 3: Kubernaut Agent** ✅ **COMPLETE** (2 days)

**Goal**: Apply proven pattern to KA

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
- ✅ Proven pattern: Same as DataStorage/KA (validated)

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
- ✅ **KA**: Authenticates using TokenReview API (Complete)
- 🚧 **Gateway**: Authentication in progress (January 2026)
- ✅ All services authorize using SAR API
- ✅ User identity captured for audit logging (SOC2 CC8.1 compliance)

### **Testing**
- ✅ **DataStorage**: 100% auth middleware coverage (Unit + Integration + E2E)
- ✅ **KA**: Full auth flow validated in integration + E2E
- 🚧 **Gateway**: Tests pending (envtest + Kind)
- ✅ Integration tests: Real K8s auth with envtest (not mocks)
- ✅ E2E tests: Real K8s auth in Kind cluster

### **Operational**
- ✅ **DataStorage**: oauth-proxy removed (Single container deployment)
- ✅ **KA**: oauth-proxy removed (Simplified debugging)
- 🚧 **Gateway**: Network Policies replaced with SAR auth (In progress)
- ✅ Portable across Kubernetes distributions (OpenShift, vanilla K8s)
- ✅ Reduced K8s API load: No issues observed with real auth in DS/KA

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
- **BR-SECURITY-1900**: Audience-bound TokenReview validation (NEW - August 2026, #1900)
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

- **If successful**: Proceed to Phase 4 (KA implementation)
- **If API server stress**: Implement caching optimizations, re-test
- **If issues**: Evaluate Gateway E2E impact before expanding
- **Final decision**: Expand to all services OR targeted rollout only

---

**Document Version**: 4.0  
**Last Updated**: August 3, 2026  
**Status**: Approved  
**Author**: AI Assistant + Engineering Team
