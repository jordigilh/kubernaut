# DD-AUTH-016: Service Authentication Migration Plan

**Status**: ✅ Approved  
**Date**: 2026-01-27  
**Author**: AI Assistant (via user collaboration)  
**Related**: DD-AUTH-014, DD-TEST-012  

---

## Executive Summary

This document provides a **systematic migration plan** for implementing Kubernetes-native authentication (TokenReview + SAR) across all Kubernaut services.

### Current Status

| Service | Status | Integration Tests | Notes |
|---------|--------|-------------------|-------|
| **DataStorage** | ✅ Complete | 59/59 passing | Production-ready |
| **HolmesGPT API** | ✅ Complete | Python implementation | Production-ready |
| **RemediationOrchestrator** | ✅ Complete | 59/59 passing | Production-ready |
| **NotificationController** | ⏳ Ready to migrate | Exists | Follows same pattern |
| **WorkflowExecution** | ⏳ Ready to migrate | Exists | Follows same pattern |
| **AIAnalysis** | ⏳ Ready to migrate | Exists | Follows same pattern |
| **SignalProcessing** | ⏳ Ready to migrate | Exists | Follows same pattern |

---

## Migration Pattern (Proven)

The migration pattern has been validated with **2 services (DataStorage + RemediationOrchestrator)** and is ready for replication:

### 1. Production Code

**Time**: 2-3 hours per service

#### A. Implement Auth Interfaces

```go
// Use shared interfaces from pkg/shared/auth/interfaces.go
type Authenticator interface {
    ValidateToken(ctx context.Context, token string) (*UserInfo, error)
}

type Authorizer interface {
    CheckAccess(ctx context.Context, user *UserInfo, verb, resource string) (bool, error)
}
```

#### B. Add Authentication Middleware

```go
// pkg/<service>/server/middleware/auth.go
type AuthMiddleware struct {
    authenticator auth.Authenticator
    authorizer    auth.Authorizer
}

func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 1. Extract Bearer token from Authorization header
        token := extractBearerToken(r)
        if token == "" {
            http.Error(w, "Authentication failed - missing Authorization header", http.StatusUnauthorized)
            return
        }

        // 2. Validate token via K8s TokenReview API
        userInfo, err := m.authenticator.ValidateToken(r.Context(), token)
        if err != nil {
            http.Error(w, fmt.Sprintf("Authentication failed: %v", err), http.StatusUnauthorized)
            return
        }

        // 3. Check permissions via K8s SAR API
        hasAccess, err := m.authorizer.CheckAccess(r.Context(), userInfo, "create", "events")
        if err != nil || !hasAccess {
            http.Error(w, "Insufficient permissions", http.StatusForbidden)
            return
        }

        // 4. Store user info in context for audit logging
        ctx := context.WithValue(r.Context(), "user", userInfo)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

#### C. Wire Auth in Main Application

```go
// cmd/<service>/main.go
func main() {
    // 1. Load kubeconfig
    config, err := rest.InClusterConfig()
    if err != nil {
        config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
    }

    // 2. Create Kubernetes client
    k8sClient, err := kubernetes.NewForConfig(config)

    // 3. Create auth components
    authenticator := k8sauth.NewKubernetesAuthenticator(k8sClient)
    authorizer := k8sauth.NewKubernetesAuthorizer(k8sClient)

    // 4. Create server with auth middleware
    server := server.New(authenticator, authorizer, /* other deps */)

    // 5. Start server
    if err := server.Run(ctx); err != nil {
        log.Fatal(err)
    }
}
```

### 2. Test Infrastructure

**Time**: 1-2 hours per service

#### A. Update Suite Setup (suite_test.go)

```go
var _ = SynchronizedBeforeSuite(func() []byte {
    // 1. Force IPv4 binding
    sharedTestEnv := &envtest.Environment{
        ControlPlane: envtest.ControlPlane{
            APIServer: &envtest.APIServer{
                SecureServing: envtest.SecureServing{
                    ListenAddr: envtest.ListenAddr{
                        Address: "127.0.0.1", // Force IPv4!
                    },
                },
            },
        },
    }

    // 2. Create ServiceAccounts + RBAC
    serviceToken, _ := infrastructure.CreateServiceAccountWithToken(
        ctx, k8sClient, cfg, "default", "<service>-service",
        "<service>-tokenreview",
        []rbacv1.PolicyRule{
            {APIGroups: ["authentication.k8s.io"], Resources: ["tokenreviews"], Verbs: ["create"]},
            {APIGroups: ["authorization.k8s.io"], Resources: ["subjectaccessreviews"], Verbs: ["create"]},
        },
    )

    clientToken, _ := infrastructure.CreateServiceAccountWithToken(
        ctx, k8sClient, cfg, "default", "<service>-client",
        "<service>-client",
        []rbacv1.PolicyRule{
            // Service-specific API permissions
            {APIGroups: [""], Resources: ["events"], Verbs: ["create", "get", "list"]},
            // envtest workaround
            {APIGroups: ["authentication.k8s.io"], Resources: ["tokenreviews"], Verbs: ["create"]},
        },
    )

    // 3. Bootstrap service with kubeconfig
    kubeconfigPath, _ := infrastructure.GenerateKubeconfigForPodman(cfg, serviceToken, "<service>-service")
    infrastructure.Bootstrap<Service>(ctx, kubeconfigPath)

    // 4. Serialize for Phase 2
    return []byte(fmt.Sprintf("%s:%s", cfg.Host, clientToken))
}, func(data []byte) {
    // Phase 2: Create authenticated clients
    parts := strings.Split(string(data), ":")
    clientToken := parts[1]
    
    serviceClients = integration.NewAuthenticated<Service>Clients(baseURL, clientToken, 5*time.Second)
})
```

#### B. Create Authenticated Client Helper

```go
// test/shared/integration/<service>_auth.go
type Authenticated<Service>Clients struct {
    AuditClient   *audit.BackgroundWriteHTTPClient
    OpenAPIClient *ogenclient.Client
}

func NewAuthenticated<Service>Clients(baseURL, token string, timeout time.Duration) *Authenticated<Service>Clients {
    transport := &serviceaccounttransport.ServiceAccountTransport{
        Transport: http.DefaultTransport,
        Token:     token,
    }

    httpClient := &http.Client{Transport: transport, Timeout: timeout}

    return &Authenticated<Service>Clients{
        AuditClient:   audit.NewBackgroundWriteHTTPClient(baseURL, audit.WithBackgroundWriteHTTPClient(httpClient)),
        OpenAPIClient: ogenclient.NewClient(baseURL, ogenclient.WithClient(httpClient)),
    }
}
```

#### C. Update Test Files

```go
// test/integration/<service>/*_test.go
var _ = Describe("Integration Tests", func() {
    var client *ogenclient.Client

    BeforeEach(func() {
        // ✅ Use authenticated client from suite setup
        client = serviceClients.OpenAPIClient
    })

    It("should successfully query with valid token", func() {
        resp, err := client.Query(ctx, params)
        Expect(err).NotTo(HaveOccurred())
    })
})
```

### 3. Deployment Manifests

**Time**: 30 minutes per service

#### A. Service RBAC

```yaml
# deploy/<service>/service-rbac.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: <service>-service
  namespace: kubernaut
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: <service>-tokenreview
rules:
- apiGroups: ["authentication.k8s.io"]
  resources: ["tokenreviews"]
  verbs: ["create"]
- apiGroups: ["authorization.k8s.io"]
  resources: ["subjectaccessreviews"]
  verbs: ["create"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: <service>-tokenreview
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: <service>-tokenreview
subjects:
- kind: ServiceAccount
  name: <service>-service
  namespace: kubernaut
```

#### B. Client RBAC

```yaml
# deploy/<service>/client-rbac.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: <service>-client
  namespace: kubernaut
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: <service>-client
rules:
# Service-specific API permissions
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: <service>-client
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: <service>-client
subjects:
- kind: ServiceAccount
  name: <service>-client
  namespace: kubernaut
```

---

## Migration Order (Recommended)

### Phase 1: Stateless Services (Higher Priority)

1. **NotificationController** (3-4 hours)
   - Already has integration tests
   - Uses audit client (similar to RemediationOrchestrator)
   - Critical for SOC2 compliance (notification delivery attribution)

2. **WorkflowExecution** (3-4 hours)
   - Already has integration tests
   - Uses audit client
   - Critical for workflow execution attribution

3. **AIAnalysis** (3-4 hours)
   - Already has integration tests
   - Uses audit client
   - Critical for AI decision attribution

4. **SignalProcessing** (3-4 hours)
   - Already has integration tests
   - Uses audit client
   - Critical for signal classification attribution

### Phase 2: Stateful Services (Lower Priority)

5. **Gateway** (Already has auth via ose-oauth-proxy)
   - Defer until Phase 1 complete
   - More complex (entrypoint for all requests)
   - May require additional coordination

---

## Per-Service Migration Checklist

### Pre-Migration (1 hour)

- [ ] Review service architecture (HTTP server, dependencies)
- [ ] Identify all HTTP endpoints that need auth
- [ ] Review existing integration test suite
- [ ] Verify service can read kubeconfig from environment

### Implementation (3-4 hours)

#### Production Code (2-3 hours)

- [ ] Add auth middleware (pkg/<service>/server/middleware/auth.go)
- [ ] Wire auth in main application (cmd/<service>/main.go)
- [ ] Update OpenAPI spec with 401/403 responses
- [ ] Update deployment manifests (service-rbac.yaml, client-rbac.yaml)

#### Test Infrastructure (1-2 hours)

- [ ] Update suite_test.go with envtest IPv4 binding
- [ ] Create service and client ServiceAccounts + RBAC
- [ ] Generate kubeconfig for Podman container
- [ ] Create authenticated client helper (test/shared/integration/<service>_auth.go)
- [ ] Update test files to use authenticated clients

#### Validation (30 minutes)

- [ ] Run integration tests: `make test-integration-<service>`
- [ ] Verify 0 authentication failures (except explicit auth tests)
- [ ] Check logs for "Token validated successfully" messages
- [ ] Verify tests complete in <3 minutes

### Post-Migration (30 minutes)

- [ ] Update service README with auth architecture
- [ ] Add auth test scenarios to test plan
- [ ] Document any service-specific auth requirements
- [ ] Update this migration plan with lessons learned

---

## Estimated Timeline

| Service | Implementation | Testing | Total | Start Date | Target Date |
|---------|---------------|---------|-------|------------|-------------|
| NotificationController | 3 hours | 1 hour | 4 hours | TBD | TBD |
| WorkflowExecution | 3 hours | 1 hour | 4 hours | TBD | TBD |
| AIAnalysis | 3 hours | 1 hour | 4 hours | TBD | TBD |
| SignalProcessing | 3 hours | 1 hour | 4 hours | TBD | TBD |

**Total Effort**: ~16 hours (~2 days for all Phase 1 services)

---

## Risk Mitigation

### Risk 1: Integration Test Failures

**Probability**: Medium  
**Impact**: High (blocks deployment)

**Mitigation**:
- Follow DD-TEST-012 pattern exactly (proven with 2 services)
- Test locally before pushing to CI/CD
- Use DataStorage/RemediationOrchestrator as reference implementation

### Risk 2: RBAC Configuration Errors

**Probability**: Low  
**Impact**: High (403 Forbidden in production)

**Mitigation**:
- Integration tests catch RBAC errors before E2E
- Copy RBAC from working services (DataStorage/RemediationOrchestrator)
- Verify RBAC in integration tests (5-10x faster feedback than E2E)

### Risk 3: Performance Regression

**Probability**: Low  
**Impact**: Medium (slower integration tests)

**Mitigation**:
- Acceptable 25-33% overhead (validated with DataStorage/RemediationOrchestrator)
- Benefits outweigh costs (5-10x faster auth bug detection)
- Optimize if tests exceed 3 minutes (use parallel execution, minimal CRDs)

---

## Success Criteria

Per service:
- ✅ Integration tests pass with real K8s auth (59/59 or similar)
- ✅ Zero authentication failures (except explicit auth tests)
- ✅ Tests complete in <3 minutes
- ✅ Logs show "Token validated successfully" messages
- ✅ RBAC correctly configured (caught by integration tests)

Overall:
- ✅ All Phase 1 services migrated within 2 days
- ✅ E2E test pass rate improves (fewer auth failures)
- ✅ CI/CD pipeline faster (less time debugging auth issues)

---

## References

- **DD-AUTH-014**: Middleware-based SAR authentication (production implementation)
- **DD-TEST-012**: envtest real authentication pattern (test infrastructure)
- **Reference Implementation**: DataStorage + RemediationOrchestrator
  - `test/integration/remediationorchestrator/suite_test.go` (lines 120-380)
  - `test/shared/integration/datastorage_auth.go`
  - `test/infrastructure/serviceaccount.go`
- **Troubleshooting**: `docs/handoff/DD_AUTH_014_ENVTEST_IPV6_BLOCKER.md`

---

## Next Steps

1. **Review this plan** with the team
2. **Select first service** from Phase 1 (recommend: NotificationController)
3. **Allocate 4 hours** for implementation + testing
4. **Follow checklist** step-by-step (don't skip steps!)
5. **Document lessons learned** and update this plan
6. **Repeat** for remaining services

---

## Questions & Decisions

| Question | Decision | Date |
|----------|----------|------|
| Which service to migrate first? | NotificationController (most similar to RemediationOrchestrator) | TBD |
| Can we migrate services in parallel? | Yes, but recommend sequential to catch common issues early | TBD |
| What if a service needs custom RBAC? | Document in service-specific section, follow same pattern | TBD |

---

**Status**: Ready for Phase 1 migration 🚀
