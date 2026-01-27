# DD-AUTH-010: E2E Real Authentication Mandate

**Date**: January 26, 2026
**Status**: ✅ **APPROVED** - AUTHORITATIVE
**Authority**: Supersedes TD-E2E-001 Phase 1 (pass-through mode)
**Priority**: CRITICAL (E2E test fidelity)
**Related**: DD-AUTH-009 (OAuth2-Proxy Migration), TD-E2E-001 (OAuth2-Proxy E2E Testing)

---

## 🎯 **DECISION**

**E2E tests MUST use production-equivalent authentication mechanisms. Mock authentication is ONLY permitted for external dependencies (LLM providers). Internal Kubernetes authentication MUST use real ServiceAccount tokens and RBAC enforcement.**

**Scope**:
- All E2E test suites (DataStorage, HolmesGPT API, AIAnalysis, etc.)
- OAuth2-proxy sidecar configuration
- ServiceAccount token handling
- RBAC enforcement via SubjectAccessReview (SAR)

**Rationale**: E2E tests exist to validate production behavior. Authentication is production behavior.

---

## 📊 **CONTEXT**

### **Problem Statement**

**TD-E2E-001 Phase 1 Approach** (INCORRECT):
```yaml
# test/infrastructure/datastorage.go:1079
args:
  - --skip-auth-regex=.*  # ❌ Pass-through mode (bypasses all auth)
  - --provider=google     # ❌ Dummy provider (not used)
```

```go
// test/e2e/datastorage/datastorage_e2e_suite_test.go:184
mockTransport := testauth.NewMockUserTransport("datastorage-e2e@test.kubernaut.io")
// ❌ Directly injects header, bypasses oauth2-proxy authentication
```

**Why This Is Wrong**:
- ❌ **E2E tests don't validate production authentication flow**
- ❌ OAuth2-proxy SAR checks are completely bypassed
- ❌ RBAC permissions are never tested
- ❌ ServiceAccount token validation is skipped
- ❌ 401/403 authentication failures would only be caught in production

---

## 🔍 **PRINCIPLE: E2E vs Integration Test Boundaries**

### **Integration Tests** ✅ Mocks Allowed
```go
// Integration tests: Mock external dependencies only
mockTransport := testauth.NewMockUserTransport("test-user@example.com")
// ✅ CORRECT: Integration tests focus on handler logic, not auth infrastructure
```

### **E2E Tests** ❌ Mocks Forbidden (Except External Services)
```go
// E2E tests: Use real K8s infrastructure
tokenBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
token := string(tokenBytes)
authenticatedTransport := &testauth.ServiceAccountTransport{Token: token}
// ✅ CORRECT: E2E tests validate full production flow
```

**Exception**: Mock LLM provider (cost consideration)

---

## ✅ **APPROVED APPROACH**

### **E2E OAuth2-Proxy Configuration** (Production-Like)

```yaml
# test/infrastructure/datastorage.go (oauth2-proxy args)
args:
  - --http-address=0.0.0.0:8080
  - --upstream=http://localhost:8081
  - --provider=oidc
  - --oidc-issuer-url=https://kubernetes.default.svc/.well-known/openid-configuration
  - --skip-oidc-discovery=true
  - --cookie-secret-file=/etc/oauth-proxy/cookie-secret
  - --cookie-name=_oauth_proxy_ds_e2e
  - --cookie-expire=24h0m0s
  - --email-domain=*
  - --skip-provider-button=true
  # CRITICAL: Only skip health checks, NOT all endpoints
  - --skip-auth-route=^/health$
  # ENABLE SAR for E2E tests
  - --openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"*"}
  # Header injection (SOC2 CC8.1)
  - --set-xauthrequest=true
  - --set-authorization-header=true
  - --pass-user-headers=true
  - --pass-access-token=false
```

**Key Changes from TD-E2E-001 Phase 1**:
1. ❌ Remove: `--skip-auth-regex=.*` (pass-through mode)
2. ✅ Add: Real SAR check with `verb:"*"`
3. ✅ Use: OIDC provider (K8s ServiceAccount tokens)

---

### **E2E ServiceAccount Creation** (Per E2E Suite)

```go
// test/infrastructure/datastorage.go (new function)
func CreateE2EServiceAccountWithDataStorageAccess(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
    clientset, err := getKubernetesClient(kubeconfigPath)
    if err != nil {
        return err
    }

    // 1. Create ServiceAccount
    sa := &corev1.ServiceAccount{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "datastorage-e2e-sa",
            Namespace: namespace,
        },
    }
    _, err = clientset.CoreV1().ServiceAccounts(namespace).Create(ctx, sa, metav1.CreateOptions{})
    if err != nil && !apierrors.IsAlreadyExists(err) {
        return fmt.Errorf("failed to create E2E ServiceAccount: %w", err)
    }

    // 2. Create RoleBinding to data-storage-client ClusterRole
    rb := &rbacv1.RoleBinding{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "datastorage-e2e-access",
            Namespace: namespace,
        },
        RoleRef: rbacv1.RoleRef{
            APIGroup: "rbac.authorization.k8s.io",
            Kind:     "ClusterRole",
            Name:     "data-storage-client",  // Uses existing ClusterRole
        },
        Subjects: []rbacv1.Subject{
            {
                Kind:      "ServiceAccount",
                Name:      "datastorage-e2e-sa",
                Namespace: namespace,
            },
        },
    }
    _, err = clientset.RbacV1().RoleBindings(namespace).Create(ctx, rb, metav1.CreateOptions{})
    if err != nil && !apierrors.IsAlreadyExists(err) {
        return fmt.Errorf("failed to create E2E RoleBinding: %w", err)
    }

    fmt.Fprintf(writer, "✅ E2E ServiceAccount + RBAC created: datastorage-e2e-sa\n")
    return nil
}
```

---

### **E2E Client Authentication** (Real ServiceAccount Tokens)

```go
// test/shared/auth/serviceaccount_transport.go (NEW FILE)
package auth

import (
    "fmt"
    "net/http"
)

// ServiceAccountTransport implements http.RoundTripper for E2E tests.
// Uses real Kubernetes ServiceAccount tokens (not mock headers).
//
// Used by: E2E tests (validates full oauth2-proxy flow)
// Authority: DD-AUTH-010 (E2E Real Authentication Mandate)
type ServiceAccountTransport struct {
    base  http.RoundTripper
    token string
}

// NewServiceAccountTransport creates a transport that uses ServiceAccount Bearer tokens.
//
// Used by: E2E tests to authenticate with oauth2-proxy
// Flow: E2E client → oauth2-proxy (validates token + SAR) → DataStorage
//
// Example:
//   tokenBytes, _ := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
//   transport := auth.NewServiceAccountTransport(string(tokenBytes))
//   httpClient := &http.Client{Transport: transport}
func NewServiceAccountTransport(token string) *ServiceAccountTransport {
    return &ServiceAccountTransport{
        base:  http.DefaultTransport,
        token: token,
    }
}

// RoundTrip implements http.RoundTripper.
// Injects Authorization: Bearer <token> header for oauth2-proxy validation.
func (t *ServiceAccountTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    reqClone := req.Clone(req.Context())
    reqClone.Header.Set("Authorization", fmt.Sprintf("Bearer %s", t.token))
    return t.base.RoundTrip(reqClone)
}
```

---

### **E2E Suite Updates** (Example: DataStorage E2E)

```go
// test/e2e/datastorage/datastorage_e2e_suite_test.go

var _ = SynchronizedBeforeSuite(
    func() []byte {
        // ... existing setup ...

        // DD-AUTH-010: Create E2E ServiceAccount + RBAC
        logger.Info("📋 DD-AUTH-010: Creating E2E ServiceAccount with DataStorage access...")
        err = infrastructure.CreateE2EServiceAccountWithDataStorageAccess(ctx, sharedNamespace, kubeconfigPath, writer)
        Expect(err).ToNot(HaveOccurred(), "Failed to create E2E ServiceAccount")
        logger.Info("✅ E2E ServiceAccount + RBAC created")

        // ... rest of setup ...
        return []byte(kubeconfigPath)
    },
    func(kubeconfigBytes []byte) {
        // ... existing parallel setup ...

        // DD-AUTH-010: Use real ServiceAccount token (not mock transport)
        logger.Info("📋 DD-AUTH-010: Creating authenticated client with ServiceAccount token...")
        
        // Read ServiceAccount token from Kind cluster
        tokenBytes, err := infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "datastorage-e2e-sa", kubeconfigPath)
        Expect(err).ToNot(HaveOccurred(), "Failed to get ServiceAccount token")
        
        // Create authenticated transport
        authenticatedTransport := testauth.NewServiceAccountTransport(string(tokenBytes))
        httpClient := &http.Client{
            Timeout:   10 * time.Second,
            Transport: authenticatedTransport,
        }
        
        dsClient, err = dsgen.NewClient(
            dataStorageURL,
            dsgen.WithClient(httpClient),
        )
        Expect(err).ToNot(HaveOccurred(), "Failed to create DataStorage OpenAPI client")
        logger.Info("✅ DataStorage client created with real ServiceAccount token")
    },
)
```

---

## 🔧 **REQUIRED INFRASTRUCTURE CHANGES**

### **Update 1: RBAC ClusterRole** (CRITICAL)

**File**: `deploy/data-storage/client-rbac.yaml:33-38`

```yaml
# OLD (Will cause 403 Forbidden)
rules:
  - apiGroups: [""]
    resources: ["services"]
    resourceNames: ["data-storage-service"]
    verbs: ["get"]

# NEW (Required for workflow catalog CRUD)
rules:
  - apiGroups: [""]
    resources: ["services"]
    resourceNames: ["data-storage-service"]
    verbs: ["*"]
```

---

### **Update 2: E2E OAuth2-Proxy Configuration**

**File**: `test/infrastructure/datastorage.go:1079-1086`

```yaml
# OLD (Pass-through mode)
args:
  - --skip-auth-regex=.*           # ❌ Bypasses ALL auth
  - --provider=google              # ❌ Dummy provider

# NEW (Real authentication)
args:
  - --skip-auth-route=^/health$    # ✅ Only skip health checks
  - --provider=oidc                # ✅ Real K8s OAuth
  - --oidc-issuer-url=https://kubernetes.default.svc/.well-known/openid-configuration
  - --openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"*"}
```

---

### **Update 3: E2E Test Suites**

**Files to Update** (9 E2E suites):
1. `test/e2e/datastorage/datastorage_e2e_suite_test.go`
2. `test/e2e/aianalysis/aianalysis_e2e_suite_test.go`
3. `test/e2e/gateway/gateway_e2e_suite_test.go`
4. (And 6 more E2E suites)

**Changes**:
1. Create E2E ServiceAccount + RBAC in `BeforeSuite`
2. Replace `MockUserTransport` with `ServiceAccountTransport`
3. Use real tokens from K8s API

---

## 📋 **UPDATED IMPLEMENTATION PLAN**

### **Phase 1: RBAC Update** (15 minutes)

**Task**: Update `data-storage-client` ClusterRole from `verbs: ["get"]` to `verbs: ["*"]`

**Files**:
- `deploy/data-storage/client-rbac.yaml`

---

### **Phase 2: E2E Infrastructure** (2 hours)

**Tasks**:
1. Create `test/shared/auth/serviceaccount_transport.go` (real token transport)
2. Create `CreateE2EServiceAccountWithDataStorageAccess()` helper
3. Create `GetServiceAccountToken()` helper
4. Update `test/infrastructure/datastorage.go` oauth2-proxy args (remove pass-through)

---

### **Phase 3: E2E Suite Updates** (2 hours)

**Tasks**:
1. Update DataStorage E2E suite to use real tokens
2. Update AIAnalysis E2E suite
3. Update Gateway E2E suite
4. (Repeat for remaining 6 E2E suites)

---

### **Phase 4: OAuth2-Proxy Deployment Migration** (2 hours)

**Tasks**:
1. Update `deploy/data-storage/deployment.yaml` (oauth2-proxy v7.5.1 + SAR)
2. Update `deploy/holmesgpt-api/06-deployment.yaml` (oauth2-proxy v7.5.1)
3. Create secrets with Kustomize
4. Test deployments

---

### **Phase 5: Workflow Catalog User Attribution** (1.5 hours)

**Tasks**:
1. Update workflow handlers to extract user from header
2. Add integration tests for user attribution
3. Add E2E tests for user attribution

---

## ✅ **SUCCESS CRITERIA**

### **RBAC**
- [ ] `data-storage-client` ClusterRole has `verbs: ["*"]`
- [ ] All E2E ServiceAccounts bound to `data-storage-client`

### **E2E Authentication**
- [ ] OAuth2-proxy uses real OIDC provider (not pass-through)
- [ ] OAuth2-proxy enforces SAR (`verb:"*"`)
- [ ] E2E clients use real ServiceAccount tokens
- [ ] No `MockUserTransport` in E2E tests (only integration)

### **Testing**
- [ ] E2E tests validate 401 (unauthenticated)
- [ ] E2E tests validate 403 (insufficient RBAC)
- [ ] E2E tests validate successful auth flow
- [ ] All E2E suites pass with real authentication

---

## 📊 **MOCK POLICY: WHERE MOCKS ARE ALLOWED**

### **✅ ALLOWED: External Dependencies**

| Dependency | Mock Used | Justification |
|------------|-----------|---------------|
| **LLM Providers** | ✅ Mock LLM | Cost consideration ($$$) |
| **External APIs** | ✅ Mock responses | Availability, cost |

### **❌ FORBIDDEN: Internal Kubernetes Infrastructure**

| Component | Mock Forbidden | Must Use Real |
|-----------|---------------|---------------|
| **ServiceAccount Tokens** | ❌ MockUserTransport in E2E | ✅ Real K8s SA tokens |
| **OAuth2-Proxy** | ❌ Pass-through mode | ✅ Real SAR enforcement |
| **RBAC** | ❌ Skipped checks | ✅ Real RoleBindings |
| **CRD Controllers** | ❌ Fake clients in E2E | ✅ Real controllers in Kind |

---

## 🎯 **REVISED EFFORT ESTIMATES**

| Phase | Task | Original Estimate | Revised Estimate |
|-------|------|-------------------|------------------|
| Phase 1 | RBAC Update | 15 min | 15 min |
| Phase 2 | E2E Infrastructure | 2 hours | 3 hours (real auth) |
| Phase 3 | E2E Suite Updates | 2 hours | 4 hours (9 suites) |
| Phase 4 | OAuth2-Proxy Migration | 2 hours | 2 hours |
| Phase 5 | Workflow Attribution | 1.5 hours | 1.5 hours |
| **TOTAL** | | **7.5 hours** | **10.5 hours** |

**Confidence**: 85% (real authentication adds complexity)

---

## 📚 **REFERENCES**

- **TD-E2E-001**: OAuth2-Proxy E2E Testing Gap (pass-through mode rejected)
- **DD-AUTH-009**: OAuth2-Proxy Migration & Workflow Attribution
- **DD-AUTH-008**: Secret Management Strategy
- **DD-AUTH-007**: OAuth2-Proxy Migration Guide
- **BR-SOC2-CC8.1**: User Attribution Requirements

---

## 🚨 **BREAKING CHANGE**

### **TD-E2E-001 Phase 1 Approach Rejected**

**Previous Approval**: Pass-through mode (`--skip-auth-regex=.*`) for Phase 1
**New Decision**: **REJECTED** - E2E tests MUST use real authentication from the start

**Rationale**:
- E2E tests exist to validate production behavior
- Pass-through mode defeats the purpose of E2E testing
- Mock authentication belongs in integration tests, not E2E

**Impact**:
- ✅ Phase 1 work (oauth2-proxy sidecar deployment) is still valuable
- ❌ Phase 1 configuration (pass-through mode) must be replaced
- ✅ Phase 2 work (real authentication) must happen NOW, not deferred

---

## ✅ **APPROVAL**

**Status**: ✅ **APPROVED** - AUTHORITATIVE

**Authority**: This decision supersedes TD-E2E-001 Phase 1 pass-through approach

**Next Action**: Implement real authentication in E2E tests (Phases 1-5 above)

---

**Document Version**: 1.0  
**Last Updated**: January 26, 2026  
**Estimated Total Effort**: 10.5 hours (revised from 7.5 hours)
