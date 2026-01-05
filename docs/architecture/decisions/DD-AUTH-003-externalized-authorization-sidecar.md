# DD-AUTH-003: Externalized Authorization via Sidecar Pattern

**Date**: January 6, 2026
**Status**: ✅ **APPROVED** - Authoritative V1.0 Pattern
**Supersedes**: DD-AUTH-002 (HTTP Authentication Middleware)
**Confidence**: 95%
**Last Reviewed**: January 6, 2026

---

## 🎯 **DECISION**

**All HTTP services requiring user authentication SHALL externalize authorization logic to a sidecar proxy (Envoy/Istio + OAuth2-Proxy) that validates tokens and injects user identity headers. Business services SHALL trust these headers without performing token validation.**

**Scope**:
- Gateway service (webhook endpoints)
- Data Storage service (REST APIs)
- Future HTTP services requiring authentication
- SOC2 CC8.1 attribution (operator identity capture)

**Pattern**: Zero-trust network architecture with sidecar authentication

---

## 📊 **Context & Problem**

### **Business Requirements**

1. **SOC2 CC8.1 Attribution**: Must capture WHO performed actions
2. **Audit Trail**: Every API call needs `actor_id` (user identity)
3. **OAuth/OIDC Support**: External users (not just K8s ServiceAccounts)
4. **Clean Business Logic**: Services should focus on business logic, not auth
5. **Testing Complexity**: Auth logic makes unit/integration testing difficult

### **Problem Statement**

**Original Approach (DD-AUTH-002)**:
- Application code validates JWT tokens
- Uses K8s TokenReview API for validation
- Requires auth middleware in every service
- Hard to test (need K8s JWT tokens in tests)
- Pollutes business layer with auth concerns

**Why This Doesn't Work**:
- ❌ **Testing Complexity**: Unit tests need K8s JWT mocking
- ❌ **Code Pollution**: Auth logic in business services
- ❌ **Limited Auth Methods**: Only K8s ServiceAccounts (no OAuth/OIDC)
- ❌ **Duplicate Logic**: Every service reimplements auth
- ❌ **Hard to Change**: Auth changes require service redeployment

---

## 🔍 **Alternatives Considered**

### **Alternative 1: Application-Level JWT Validation** ❌ REJECTED

**Approach**: Services decode and validate JWT tokens using middleware

```go
// In every service (pkg/datastorage/middleware/auth.go)
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := extractToken(r)
        userInfo, err := validateJWT(token)  // Complex validation logic
        if err != nil {
            http.Error(w, "Unauthorized", 401)
            return
        }
        ctx := context.WithValue(r.Context(), "user", userInfo)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

**Pros**:
- ✅ Full control over validation logic
- ✅ No additional infrastructure

**Cons**:
- ❌ **Auth logic in business code** (separation of concerns violation)
- ❌ **Testing complexity** (need JWT mocking in every test)
- ❌ **Duplicate code** (every service reimplements auth)
- ❌ **Hard to test** (K8s JWT tokens required)
- ❌ **Limited to K8s auth** (no OAuth/OIDC support)
- ❌ **Deployment coupling** (auth changes require service redeploy)

**Why Rejected**: Violates separation of concerns, hard to test, limited auth methods

**Confidence**: 95% rejection (clear architectural smell)

---

### **Alternative 2: API Gateway with Auth** ⚠️ PARTIAL SOLUTION

**Approach**: Single API Gateway validates tokens, forwards to services

```
External → API Gateway (auth) → Internal Services (no auth)
```

**Pros**:
- ✅ Centralized auth logic
- ✅ Services have no auth code
- ✅ Single point of auth enforcement

**Cons**:
- ❌ **Single point of failure** (gateway down = no auth)
- ❌ **Doesn't work for internal service-to-service** calls
- ❌ **East-west traffic unprotected** (services can bypass gateway)
- ❌ **Not zero-trust** (assumes internal network is safe)
- ❌ **Scaling bottleneck** (all traffic through one gateway)

**Why Rejected**: Not zero-trust, doesn't protect internal traffic

**Confidence**: 90% rejection (security gaps)

---

### **Alternative 3: Externalized Sidecar Authorization** ✅ APPROVED

**Approach**: Each service pod has sidecar proxy that validates tokens and injects headers

```
External Traffic
       ↓
┌──────────────────────────────────────────┐
│ Pod: gateway-deployment                   │
│                                           │
│  ┌─────────────┐      ┌────────────────┐ │
│  │ OAuth2-Proxy│─────>│ Gateway Service│ │
│  │ Sidecar     │      │ (port 8080)    │ │
│  │ (port 4180) │      │                │ │
│  │             │      │ • Read headers │ │
│  │ • Validate  │      │ • No auth code │ │
│  │   OAuth     │      │ • Clean logic  │ │
│  │ • Inject    │      │                │ │
│  │   X-User-ID │      │                │ │
│  └─────────────┘      └────────────────┘ │
│         ↑                                 │
│  Port 4180 (external)                    │
│  Network Policy: ONLY route to sidecar   │
└──────────────────────────────────────────┘
```

**Pros**:
- ✅ **Zero auth code in services** (clean separation of concerns)
- ✅ **Easy testing** (unit tests just set headers, no JWT mocking)
- ✅ **Zero-trust architecture** (every pod protected)
- ✅ **OAuth/OIDC support** (not limited to K8s auth)
- ✅ **Network policy enforced** (can't bypass sidecar)
- ✅ **Standard pattern** (service mesh best practice)
- ✅ **Independent deployment** (auth changes don't require service redeploy)
- ✅ **Consistent across services** (same pattern everywhere)

**Cons**:
- ⚠️ **Additional sidecar** (~50MB memory per pod)
- ⚠️ **Configuration overhead** (sidecar config per service)
- ⚠️ **Network policy required** (must enforce proxy path)

**Mitigations**:
- Memory cost is negligible (50MB is 1% of typical service memory)
- Configuration is one-time, reusable template
- Network policies are mandatory for zero-trust anyway

**Why Approved**: Clean separation, easy testing, zero-trust, industry standard

**Confidence**: 95% approval (proven pattern with clear benefits)

---

## 🏗️ **Implementation Architecture**

### **Component 1: OAuth2-Proxy Sidecar**

```yaml
# deploy/gateway/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
spec:
  template:
    spec:
      containers:
      # Main application container
      - name: gateway
        image: kubernaut/gateway:v1.0.0
        ports:
        - containerPort: 8080  # Internal port (unauthenticated)
          name: http
        env:
        - name: TRUSTED_PROXY_ENABLED
          value: "true"  # Trust X-User-ID headers from sidecar
        - name: TRUSTED_PROXY_CIDRS
          value: "127.0.0.1/32"  # Only trust localhost (sidecar)

      # OAuth2-Proxy sidecar
      - name: oauth2-proxy
        image: quay.io/oauth2-proxy/oauth2-proxy:v7.6.0
        ports:
        - containerPort: 4180  # External port (authenticated)
          name: proxy
        args:
        # OAuth provider configuration
        - --provider=oidc
        - --oidc-issuer-url=https://auth.kubernaut.ai
        - --client-id=$(OAUTH2_CLIENT_ID)
        - --client-secret=$(OAUTH2_CLIENT_SECRET)

        # Upstream configuration (forward to main app)
        - --upstream=http://127.0.0.1:8080
        - --http-address=0.0.0.0:4180

        # Header injection (user identity)
        - --set-xauthrequest=true
        - --pass-user-headers=true
        - --pass-authorization-header=false  # Don't pass token to app

        # Security
        - --cookie-secure=true
        - --cookie-samesite=lax
        - --cookie-httponly=true

        env:
        - name: OAUTH2_CLIENT_ID
          valueFrom:
            secretKeyRef:
              name: oauth2-credentials
              key: client-id
        - name: OAUTH2_CLIENT_SECRET
          valueFrom:
            secretKeyRef:
              name: oauth2-credentials
              key: client-secret

        resources:
          requests:
            memory: "50Mi"
            cpu: "50m"
          limits:
            memory: "100Mi"
            cpu: "200m"
```

**Headers Injected by OAuth2-Proxy**:
- `X-Forwarded-User`: User email/ID (e.g., `alice@kubernaut.ai`)
- `X-Forwarded-Email`: User email
- `X-Auth-Request-User`: User ID (alternative header name)
- `X-Auth-Request-Email`: User email (alternative)

---

### **Component 2: Network Policy (Critical Security)**

```yaml
# deploy/gateway/network-policy.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-auth-enforcement
  namespace: kubernaut
spec:
  podSelector:
    matchLabels:
      app: gateway
  policyTypes:
  - Ingress

  ingress:
  # RULE 1: External traffic MUST go to sidecar port only
  - from:
    - namespaceSelector: {}  # From any namespace
    - podSelector: {}        # From any pod
    ports:
    - protocol: TCP
      port: 4180  # OAuth2-Proxy port (authenticated)

  # RULE 2: Application port accessible ONLY from localhost (sidecar)
  # This is enforced by pod-local networking (127.0.0.1)
  # External traffic to port 8080 is blocked by not listing it in ingress rules
```

**Security Guarantee**:
- ✅ External traffic CANNOT reach port 8080 (not in network policy)
- ✅ Only sidecar (localhost) can reach port 8080
- ✅ Application MUST trust headers because network enforces proxy path
- ✅ Cannot bypass sidecar authentication

---

### **Component 3: Application Code (Minimal Changes)**

```go
// pkg/gateway/middleware/auth.go

package middleware

import (
    "context"
    "errors"
    "net"
    "net/http"
    "strings"
)

// UserContext holds authenticated user information
type UserContext struct {
    UserID string
    Email  string
}

// contextKey for user context
type contextKey string

const userContextKey = contextKey("user")

// ExtractUserFromProxyHeaders reads user identity from trusted sidecar headers
//
// SECURITY: This function MUST only be enabled when:
// 1. TRUSTED_PROXY_ENABLED=true environment variable is set
// 2. Request originates from trusted CIDR (127.0.0.1 = sidecar)
// 3. Network policy enforces sidecar path
//
// Headers Read (from OAuth2-Proxy sidecar):
// - X-Forwarded-User: Primary user identifier
// - X-Forwarded-Email: User email (fallback if user ID missing)
//
// DD-AUTH-003: Externalized Authorization Pattern
func ExtractUserFromProxyHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Check if proxy trust is enabled
        if !config.TrustedProxyEnabled {
            http.Error(w, "Proxy authentication not enabled", http.StatusInternalServerError)
            return
        }

        // Verify request came from trusted source (sidecar = localhost)
        if !isFromTrustedProxy(r.RemoteAddr) {
            http.Error(w, "Unauthorized: request not from trusted proxy", http.StatusUnauthorized)
            return
        }

        // Extract user identity from sidecar-injected headers
        userID := r.Header.Get("X-Forwarded-User")
        email := r.Header.Get("X-Forwarded-Email")

        if userID == "" && email == "" {
            http.Error(w, "Unauthorized: no user identity in headers", http.StatusUnauthorized)
            return
        }

        // Use email as fallback if userID not provided
        if userID == "" {
            userID = email
        }

        // Inject user context
        userCtx := &UserContext{
            UserID: userID,
            Email:  email,
        }

        ctx := context.WithValue(r.Context(), userContextKey, userCtx)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// isFromTrustedProxy validates request originated from sidecar
func isFromTrustedProxy(remoteAddr string) bool {
    // Extract IP from "IP:port" format
    ip, _, err := net.SplitHostPort(remoteAddr)
    if err != nil {
        return false
    }

    // Check against trusted CIDRs (configured via env var)
    for _, cidr := range config.TrustedProxyCIDRs {
        _, network, err := net.ParseCIDR(cidr)
        if err != nil {
            continue
        }
        if network.Contains(net.ParseIP(ip)) {
            return true
        }
    }

    return false
}

// GetUserFromContext extracts user context from request context
func GetUserFromContext(ctx context.Context) (*UserContext, error) {
    user, ok := ctx.Value(userContextKey).(*UserContext)
    if !ok || user == nil {
        return nil, errors.New("no user context found")
    }
    return user, nil
}
```

---

### **Component 4: Audit Event Integration**

```go
// pkg/gateway/audit/events.go

func (s *Server) recordAuditEvent(ctx context.Context, eventType string, data map[string]interface{}) error {
    // Extract user from context (injected by middleware)
    user, err := middleware.GetUserFromContext(ctx)
    if err != nil {
        return fmt.Errorf("missing user context: %w", err)
    }

    // Create audit event with user attribution (SOC2 CC8.1)
    event := audit.AuditEvent{
        EventType:     eventType,
        ActorId:       user.UserID,  // From sidecar header (no JWT decoding!)
        ActorEmail:    user.Email,   // Additional context
        CorrelationId: extractCorrelationID(ctx),
        Category:      "gateway",
        Outcome:       "success",
        Timestamp:     time.Now().UTC(),
        EventData:     data,
    }

    return s.auditStore.StoreAudit(ctx, event)
}
```

**Key Point**: Application code is SIMPLE - just reads headers. No JWT decoding, no token validation, no auth complexity.

---

## 🧪 **Testing Strategy**

### **Unit Tests (70%+ Coverage)** ✅ SIMPLE

```go
// pkg/gateway/middleware/auth_test.go

var _ = Describe("User Context Extraction", func() {
    It("should extract user from proxy headers", func() {
        req := httptest.NewRequest("GET", "/api/v1/workflows", nil)
        req.RemoteAddr = "127.0.0.1:12345"  // Simulates sidecar
        req.Header.Set("X-Forwarded-User", "alice@kubernaut.ai")
        req.Header.Set("X-Forwarded-Email", "alice@kubernaut.ai")

        rec := httptest.NewRecorder()

        handler := ExtractUserFromProxyHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            user, err := GetUserFromContext(r.Context())
            Expect(err).ToNot(HaveOccurred())
            Expect(user.UserID).To(Equal("alice@kubernaut.ai"))
            w.WriteHeader(http.StatusOK)
        }))

        handler.ServeHTTP(rec, req)
        Expect(rec.Code).To(Equal(http.StatusOK))
    })

    It("should reject requests without user headers", func() {
        req := httptest.NewRequest("GET", "/api/v1/workflows", nil)
        req.RemoteAddr = "127.0.0.1:12345"
        // No X-Forwarded-User header

        rec := httptest.NewRecorder()
        handler := ExtractUserFromProxyHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            t.Fatal("should not reach handler")
        }))

        handler.ServeHTTP(rec, req)
        Expect(rec.Code).To(Equal(http.StatusUnauthorized))
    })
})
```

**Benefits**:
- ✅ **No JWT mocking** (just set headers)
- ✅ **No K8s API mocking** (no TokenReview calls)
- ✅ **Fast tests** (no HTTP calls to auth services)
- ✅ **Simple test setup** (httptest.NewRequest)

---

### **Integration Tests (>50% Coverage)** ✅ SIMPLE

```go
// test/integration/gateway/auth_integration_test.go

var _ = Describe("Gateway Authentication Integration", func() {
    It("should accept requests with valid user headers", func() {
        // Create authenticated request (simulates sidecar injection)
        req := &http.Request{
            Method: "POST",
            URL:    parseURL("http://localhost:8080/api/v1/webhooks/kubernetes-events"),
            Header: http.Header{
                "X-Forwarded-User":  []string{"operator@example.com"},
                "X-Forwarded-Email": []string{"operator@example.com"},
                "Content-Type":      []string{"application/json"},
            },
            Body: io.NopCloser(strings.NewReader(`{"test": "data"}`)),
        }

        resp, err := http.DefaultClient.Do(req)
        Expect(err).ToNot(HaveOccurred())
        Expect(resp.StatusCode).To(Equal(http.StatusAccepted))

        // Verify audit event has correct actor_id
        events := queryAuditEvents("test-correlation-id")
        Expect(events[0].ActorId).To(Equal("operator@example.com"))
    })
})
```

**No OAuth2-Proxy in integration tests** - just set headers directly!

---

### **E2E Tests (10-15% Coverage)** ⚠️ WITH SIDECAR

```go
// test/e2e/gateway/auth_e2e_test.go

var _ = Describe("Gateway E2E with OAuth Sidecar", func() {
    It("should authenticate via OAuth2-Proxy sidecar", func() {
        // Connect to external port (OAuth2-Proxy: 4180)
        // This test requires OAuth2-Proxy actually running

        // Get OAuth token from test provider
        token := getTestOAuthToken()

        req := &http.Request{
            Method: "POST",
            URL:    parseURL("http://localhost:4180/api/v1/webhooks/kubernetes-events"),  // Sidecar port
            Header: http.Header{
                "Authorization": []string{"Bearer " + token},
                "Content-Type":  []string{"application/json"},
            },
            Body: io.NopCloser(strings.NewReader(`{"test": "data"}`)),
        }

        resp, err := http.DefaultClient.Do(req)
        Expect(err).ToNot(HaveOccurred())
        Expect(resp.StatusCode).To(Equal(http.StatusAccepted))
    })
})
```

**E2E tests ARE the only place OAuth2-Proxy runs** - validates complete flow.

---

## ✅ **Consequences**

### **Positive Impacts**

1. **Clean Business Logic** ✅
   - Services have ZERO auth code
   - Focus on business requirements
   - Easy to understand and maintain

2. **Easy Testing** ✅
   - Unit tests: Set headers, no JWT mocking
   - Integration tests: Set headers, no OAuth setup
   - E2E tests: Full OAuth flow (only where needed)
   - Test authz independently from business logic

3. **Zero-Trust Architecture** ✅
   - Every pod protected by sidecar
   - Network policy enforces proxy path
   - Cannot bypass authentication

4. **Flexible Auth Methods** ✅
   - OAuth/OIDC support
   - K8s ServiceAccounts (if needed)
   - External identity providers
   - Easy to add new auth methods (just reconfigure sidecar)

5. **SOC2 CC8.1 Compliance** ✅
   - User attribution on every API call
   - Audit events have `actor_id`
   - Cryptographically verified identity

6. **Independent Deployment** ✅
   - Auth changes don't require service redeploy
   - Update sidecar config independently
   - Rolling updates without downtime

7. **Consistent Pattern** ✅
   - Same auth pattern across all services
   - Easy to onboard new services
   - Standard service mesh practice

---

### **Negative Impacts** (Mitigated)

1. **Additional Memory** ⚠️
   - **Impact**: ~50MB per pod (OAuth2-Proxy sidecar)
   - **Mitigation**: Negligible (1% of typical service memory)
   - **Severity**: LOW

2. **Configuration Overhead** ⚠️
   - **Impact**: Each service needs sidecar configuration
   - **Mitigation**: Reusable Kustomize template, one-time setup
   - **Severity**: LOW

3. **Network Policy Required** ⚠️
   - **Impact**: Must enforce sidecar path via network policies
   - **Mitigation**: Network policies mandatory for zero-trust anyway
   - **Severity**: LOW

4. **Slightly More Complex Deployment** ⚠️
   - **Impact**: Two containers per pod instead of one
   - **Mitigation**: Standard K8s pattern, widely understood
   - **Severity**: LOW

---

### **Neutral Impacts**

1. **Service Mesh Compatibility** 🔄
   - Pattern works with or without full service mesh (Istio/Linkerd)
   - Can upgrade to full mesh later without changing pattern
   - OAuth2-Proxy is lightweight alternative to full mesh

2. **Observability** 🔄
   - Sidecar adds auth metrics (login success/failure rates)
   - Additional logs from OAuth2-Proxy
   - May need separate monitoring for sidecar

---

## 📊 **Supersedes DD-AUTH-002**

### **Why DD-AUTH-002 is Superseded**

DD-AUTH-002 described application-level JWT validation using K8s TokenReview API. This approach has been **SUPERSEDED** by DD-AUTH-003 because:

| Aspect | DD-AUTH-002 (Old) | DD-AUTH-003 (New) |
|--------|-------------------|-------------------|
| **Auth Location** | In application code | In sidecar (external) |
| **Token Validation** | Application validates JWT | Sidecar validates JWT |
| **Auth Methods** | K8s ServiceAccounts only | OAuth/OIDC + K8s |
| **Testing Complexity** | High (need K8s JWT mocking) | Low (just set headers) |
| **Code Pollution** | High (auth middleware in services) | None (zero auth code) |
| **Separation of Concerns** | Poor (auth + business mixed) | Excellent (complete separation) |
| **Zero-Trust** | No (application-level only) | Yes (network-enforced) |

### **Migration Path from DD-AUTH-002**

**If DD-AUTH-002 was implemented**:
1. Add OAuth2-Proxy sidecar to deployments
2. Add network policies to enforce sidecar path
3. Replace auth middleware with header extraction
4. Remove JWT validation code from services
5. Remove K8s TokenReview API client dependencies

**Timeline**: ~1 week per service (deployment changes + code simplification)

**Risk**: LOW (sidecar pattern is standard, well-tested)

---

## 🔗 **Related Decisions**

- **Supersedes**: [DD-AUTH-002](mdc:docs/architecture/decisions/DD-AUTH-002-http-authentication-middleware.md) (HTTP Authentication Middleware)
- **Supports**: [DD-AUTH-001](mdc:docs/architecture/decisions/DD-AUTH-001-crd-webhook-authentication.md) (CRD Webhook Authentication)
- **Enables**: SOC2 CC8.1 (Change Control - Attribution)
- **Aligns With**: Zero-Trust Architecture, Service Mesh Best Practices

---

## 📈 **Success Metrics**

| Metric | Target | Current | Status |
|--------|--------|---------|--------|
| Services with auth sidecar | 100% | 0% | ⬜ Not Started |
| Unit test auth complexity | Zero JWT mocking | N/A | ⬜ Not Started |
| Auth code in services | 0 lines | N/A | ⬜ Not Started |
| OAuth provider support | ≥ 2 providers | 0 | ⬜ Not Started |
| Network policy enforcement | 100% pods | 0% | ⬜ Not Started |

---

## 🔄 **Review & Evolution**

### **When to Revisit This Decision**

1. **Full Service Mesh Adoption**: If adopting Istio/Linkerd, may consolidate into mesh auth
2. **Performance Issues**: If sidecar latency becomes bottleneck (unlikely, but monitor)
3. **New Auth Requirements**: If new auth methods emerge that don't fit sidecar pattern

### **Next Steps**

1. **Week 1**: Implement OAuth2-Proxy sidecar for Gateway service
2. **Week 1**: Add network policies for Gateway
3. **Week 1**: Update Gateway code to read headers (remove DD-AUTH-002 middleware)
4. **Week 2**: Extend pattern to Data Storage service
5. **Week 2**: Document deployment templates and runbooks

---

## 📚 **References**

- [OAuth2-Proxy Documentation](https://oauth2-proxy.github.io/oauth2-proxy/)
- [Kubernetes NetworkPolicy Guide](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [Service Mesh Security Best Practices](https://www.cncf.io/blog/2021/03/18/service-mesh-security-best-practices/)
- [NIST Zero Trust Architecture](https://csrc.nist.gov/publications/detail/sp/800-207/final)

---

**Document Status**: ✅ APPROVED
**Implementation Status**: ⬜ NOT STARTED
**Target V1.0**: Yes (Gateway + Data Storage services)
**Confidence**: 95%

