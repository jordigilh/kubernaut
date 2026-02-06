⚠️ **OBSOLETE** - February 6, 2026

**Reason**: Kubernaut adopted middleware-based authentication approach (DD-AUTH-014) using Kubernetes TokenReview and SubjectAccessReview APIs instead of `oauth2-proxy`. See `holmesgpt-api/src/middleware/auth.py` for implementation.

**Architectural Decision**: DD-AUTH-014 provides application-level authentication/authorization control without the need for sidecar proxies, simplifying deployment and improving performance.

**Current Authentication Stack**:
- **HAPI**: `holmesgpt-api/src/middleware/auth.py` (DD-AUTH-014)
- **Gateway**: Kubernetes-native TokenReview + SAR
- **DataStorage**: Direct service-to-service with ServiceAccount tokens

This document is retained for historical context but should not be implemented.

---

# TD-E2E-001: OAuth2-Proxy E2E Testing Gap

**Date Created**: January 21, 2026
**Date Started**: January 21, 2026
**Date Completed (Phase 1)**: January 21, 2026
**Status**: ❌ OBSOLETE (Replaced by DD-AUTH-014 middleware approach)
**Priority**: ~~CRITICAL~~ N/A (SOC2 CC8.1 compliance achieved via middleware)
**Estimated Effort**: Phase 1: ✅ COMPLETE (2.5 hours) | Phase 2: ~~8-10 hours~~ NOT REQUIRED
**Related**: DD-AUTH-007, DD-AUTH-004, BR-HAPI-197, BR-SCOPE-001, SOC2 CC8.1, **DD-AUTH-014 (replacement)**

---

## 📋 **IMPLEMENTATION PHASES**

### **Phase 1: Architecture Parity** (✅ COMPLETE - January 21, 2026)
**Goal**: Deploy oauth2-proxy sidecar in E2E with pass-through mode
**Duration**: ✅ 2.5 hours (actual)
**Scope**:
- ✅ OAuth2-proxy pulled from public registry (`quay.io/oauth2-proxy/oauth2-proxy:v7.5.1`)
  - **NO BUILD REQUIRED** - Kubernetes pulls automatically from public registry
- ✅ DataStorage deployment function with oauth2-proxy sidecar (pass-through mode: `--skip-auth-regex=.*`)
- ✅ Updated ALL 9 E2E services to deploy DataStorage with oauth2-proxy sidecar:
  - RemediationOrchestrator, WorkflowExecution, SignalProcessing
  - Gateway (3 variants: parallel, sequential, hybrid)
  - AIAnalysis, HAPI, Notification, AuthWebhook
- ✅ All E2E services compile successfully
- ✅ Architecture validated: Service:8080 → oauth2-proxy:8080 → DataStorage:8081

**What Phase 1 Validates**:
- ✅ Architecture: Service:8080 → oauth2-proxy:8080 → DataStorage:8081
- ✅ Sidecar deployment pattern (matches production)
- ✅ Port routing correctness
- ✅ OAuth2-proxy pulled from `quay.io` (no custom build needed)
- ✅ Pass-through mode (`--skip-auth-regex=.*`) - no auth validation yet

**What Phase 1 Doesn't Validate** (deferred to Phase 2 - Post-BR-HAPI-197):
- ⏳ Real ServiceAccount token validation (pass-through mode only)
- ⏳ RBAC/SAR enforcement (no token checks yet)
- ⏳ OAuth failure scenarios (401/403/503)
- ⏳ Selective endpoint protection (currently all endpoints pass through)

### **Phase 2: Real OAuth Testing** (⏳ DEFERRED - After BR-HAPI-197, Before BR-SCOPE-001)
**Goal**: Full OAuth2-proxy functionality with ServiceAccount tokens
**Duration**: 8-10 hours
**Scope**:
- ServiceAccount token validation (K8s TokenReview API)
- RBAC enforcement via SubjectAccessReview
- **CRITICAL**: Configure `--skip-auth-regex` to match authoritative documentation:
  - Review `api/openapi/data-storage-v1.yaml` for endpoints requiring auth
  - Review DD-AUTH-004, DD-AUTH-007 for OAuth configuration
  - Match production `deploy/data-storage/deployment.yaml` patterns
  - Workflow Catalog WRITE endpoints: POST /workflows, PATCH /workflows/{id}/disable (require auth)
  - Legal Hold endpoints: POST /legal-hold, DELETE /legal-hold/{id} (require auth)
  - All READ endpoints: GET /workflows, POST /workflows/search, GET /audit/events (NO auth)
- OAuth failure tests (6+ test cases: 401/403/503)
- Update E2E clients to use real SA tokens
- Documentation updates

---

## 📋 **PROBLEM STATEMENT**

**Current State**: ALL 8 E2E services deploy DataStorage **WITHOUT** oauth2-proxy sidecar, despite production relying on it for authentication/authorization.

**Impact**: OAuth authentication failures (401/403/503) would only be discovered in production, not in E2E tests.

---

## 🚨 **PRODUCTION RISKS NOT COVERED BY E2E**

### **Authentication Flow Risks**
1. **OAuth2-proxy crash/restart**: E2E never tests proxy unavailability (503)
2. **Invalid ServiceAccount tokens**: E2E doesn't test HTTP 401 responses
3. **Insufficient RBAC permissions**: E2E doesn't test HTTP 403 responses
4. **Port routing mismatch**: E2E uses DS:8080 direct, production uses Service:8080 → oauth2-proxy:8080 → DS:8081
5. **Header injection**: E2E doesn't validate `X-Forwarded-User` header propagation from proxy
6. **Token expiration**: E2E doesn't test ServiceAccount token lifecycle

### **SOC2 Compliance Risks**
- Legal hold operations REQUIRE `X-Forwarded-User` header for user attribution (SOC2 CC8.1)
- E2E tests pass without this header, bypassing SOC2 controls
- Production failures would violate audit trail requirements

---

## 🎯 **TECHNICAL DEBT SCOPE**

### **Phase 1: Infrastructure Setup** ⏱️ 4-6 hours

#### **1.1: Create OAuth2-Proxy Build Infrastructure**
**New File**: `test/infrastructure/oauth2_proxy.go`

**Functions**:
```go
// Build oauth2-proxy image for E2E
func BuildOAuth2ProxyImageForKind(ctx context.Context, consumer string, writer io.Writer) (imageName string, err error)

// Load oauth2-proxy image to Kind cluster
func LoadOAuth2ProxyImageToKind(imageName, clusterName string, writer io.Writer) error

// Deploy DataStorage WITH oauth2-proxy sidecar (DS:8081, proxy:8080)
func DeployDataStorageWithOAuth2Proxy(ctx context.Context, ns, kubeconfig, dsImage, oauthImage string, writer io.Writer) error
```

**Image Source**:
- Public: `quay.io/oauth2-proxy/oauth2-proxy:v7.5.1` (multi-arch: ARM64 + AMD64)
- OR custom build from `build/oauth2-proxy/Dockerfile`

**OAuth2-Proxy Configuration** (per DD-AUTH-004):
```yaml
args:
  # Skip auth for read-only endpoints
  - --skip-auth-route=^/(health|metrics)$
  - --skip-auth-route=^/api/v1/workflows$                    # GET list
  - --skip-auth-route=^/api/v1/workflows/[^/]+$              # GET by ID
  - --skip-auth-route=^/api/v1/workflows/search$             # POST search (read-only)
  - --skip-auth-regex=^/api/v1/audit/(?!legal-hold$|legal-hold/[^/]+$).*

  # Require auth for write endpoints
  # - POST /api/v1/workflows (create)
  # - PATCH /api/v1/workflows/{id}/disable
  # - POST /api/v1/audit/legal-hold (place hold)
  # - DELETE /api/v1/audit/legal-hold/{id} (release hold)

  # Header injection for SOC2 user attribution
  - --set-xauthrequest=true  # Injects X-Forwarded-User header
```

---

#### **1.2: Update ALL 8 E2E Services**

**Services to Update**:
| Service | File | Current | Target |
|---------|------|---------|--------|
| **RO** | `test/infrastructure/remediationorchestrator_e2e_hybrid.go` | 3 images (RO, DS, AW) | 4 images (+OAuth2-Proxy) |
| **WE** | `test/infrastructure/workflowexecution_e2e_hybrid.go` | 3 images (WE, DS, AW) | 4 images (+OAuth2-Proxy) |
| **NT** | `test/infrastructure/notification_e2e.go` | 2 images (NT, AW) | 3 images (+OAuth2-Proxy) |
| **Gateway** | `test/infrastructure/gateway_e2e.go` | 2 images (GW, DS) | 3 images (+OAuth2-Proxy) |
| **SignalProcessing** | `test/infrastructure/signalprocessing_e2e.go` | 2 images (SP, DS) | 3 images (+OAuth2-Proxy) |
| **AIAnalysis** | `test/infrastructure/aianalysis_e2e.go` | 4 images (AA, DS, HAPI, MockLLM) | 5 images (+OAuth2-Proxy) |
| **HAPI** | `test/infrastructure/holmesgpt_api.go` | 3 images (HAPI, DS, MockLLM) | 4 images (+OAuth2-Proxy) |
| **AuthWebhook** | `test/infrastructure/authwebhook_e2e.go` | 2 images (AW, DS) | 3 images (+OAuth2-Proxy) |

**Pattern** (same as AuthWebhook parallel build/load):
```go
// PHASE 1: Build OAuth2-Proxy in parallel
go func() {
    imageName, err := BuildOAuth2ProxyImageForKind(ctx, "remediationorchestrator-e2e", writer)
    buildResults <- buildResult{name: "OAuth2-Proxy", imageName: imageName, err: err}
}()

// PHASE 3: Load OAuth2-Proxy in parallel
go func() {
    err := LoadOAuth2ProxyImageToKind(oauth2ProxyImage, clusterName, writer)
    loadResults <- result{name: "OAuth2-Proxy load", err: err}
}()

// PHASE 4: Deploy DataStorage WITH oauth2-proxy sidecar
err := DeployDataStorageWithOAuth2Proxy(ctx, namespace, kubeconfigPath, dsImage, oauth2ProxyImage, writer)
```

---

#### **1.3: Create ServiceAccount + RBAC for Each E2E Service**

**Pattern** (add to each E2E suite's `BeforeSuite`):
```go
// Create ServiceAccount for E2E client
sa := &corev1.ServiceAccount{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "remediationorchestrator-e2e-sa",
        Namespace: namespace,
    },
}
clientset.CoreV1().ServiceAccounts(namespace).Create(ctx, sa, metav1.CreateOptions{})

// Create RoleBinding to allow DataStorage access
rb := &rbacv1.RoleBinding{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "remediationorchestrator-e2e-datastorage-access",
        Namespace: namespace,
    },
    RoleRef: rbacv1.RoleRef{
        APIGroup: "rbac.authorization.k8s.io",
        Kind:     "ClusterRole",
        Name:     "system:auth-delegator",  // Standard K8s role for SAR
    },
    Subjects: []rbacv1.Subject{
        {
            Kind:      "ServiceAccount",
            Name:      "remediationorchestrator-e2e-sa",
            Namespace: namespace,
        },
    },
}
clientset.RbacV1().RoleBindings(namespace).Create(ctx, rb, metav1.CreateOptions{})
```

---

#### **1.4: Update E2E Clients to Use ServiceAccount Tokens**

**Current** (mock headers - bypasses oauth2-proxy):
```go
mockTransport := testauth.NewMockUserTransport("datastorage-e2e@test.kubernaut.io")
httpClient = &http.Client{Transport: mockTransport}
```

**Target** (real ServiceAccount tokens):
```go
// Read ServiceAccount token from mounted secret
tokenBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
Expect(err).ToNot(HaveOccurred())
token := string(tokenBytes)

// Create authenticated transport
authenticatedTransport := &testauth.ServiceAccountTransport{
    BaseTransport: http.DefaultTransport,
    Token:         token,
}
httpClient = &http.Client{Transport: authenticatedTransport}
```

**Helper** (new file: `pkg/testutil/auth/serviceaccount_transport.go`):
```go
type ServiceAccountTransport struct {
    BaseTransport http.RoundTripper
    Token         string
}

func (t *ServiceAccountTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", t.Token))
    return t.BaseTransport.RoundTrip(req)
}
```

---

### **Phase 2: OAuth Failure Scenario Tests** ⏱️ 6-8 hours

#### **2.1: Add OAuth Failure Tests to DataStorage E2E Suite**

**New File**: `test/e2e/datastorage/23_oauth_authentication_test.go`

**Test Cases**:

```go
var _ = Describe("OAuth2-Proxy Authentication (TD-E2E-001)", Label("e2e", "oauth", "technical-debt"), func() {

    It("should reject requests without Authorization header (HTTP 401)", func() {
        // Test Case: TD-E2E-001-TC1
        // Validates: Unauthenticated requests to write endpoints are rejected

        unauthClient := &http.Client{Transport: http.DefaultTransport}

        workflow := createSampleWorkflow()
        resp, err := unauthClient.Post(
            fmt.Sprintf("%s/api/v1/workflows", dsBaseURL),
            "application/json",
            bytes.NewReader(workflowJSON),
        )
        Expect(err).ToNot(HaveOccurred())
        Expect(resp.StatusCode).To(Equal(401), "Should reject unauthenticated write request")
    })

    It("should reject requests with invalid token (HTTP 401)", func() {
        // Test Case: TD-E2E-001-TC2
        // Validates: Invalid tokens are rejected by oauth2-proxy

        invalidClient := &http.Client{
            Transport: &testauth.ServiceAccountTransport{
                BaseTransport: http.DefaultTransport,
                Token:         "invalid-token-12345",
            },
        }

        resp, err := invalidClient.Post(dsBaseURL+"/api/v1/workflows", ...)
        Expect(resp.StatusCode).To(Equal(401))
    })

    It("should reject requests with insufficient RBAC permissions (HTTP 403)", func() {
        // Test Case: TD-E2E-001-TC3
        // Validates: RBAC enforcement via Subject Access Review (SAR)

        limitedSA := createServiceAccountWithoutDataStorageAccess()
        limitedClient := createClientWithServiceAccount(limitedSA)

        resp, err := limitedClient.Post(dsBaseURL+"/api/v1/workflows", ...)
        Expect(resp.StatusCode).To(Equal(403))
    })

    It("should allow read-only endpoints without authentication", func() {
        // Test Case: TD-E2E-001-TC4
        // Validates: Selective endpoint protection (read-only bypass)

        unauthClient := &http.Client{Transport: http.DefaultTransport}

        // GET /workflows (list) should work
        resp, err := unauthClient.Get(dsBaseURL + "/api/v1/workflows")
        Expect(resp.StatusCode).To(Equal(200), "Read-only endpoints should not require auth")

        // POST /workflows/search should work (read-only search)
        resp, err = unauthClient.Post(dsBaseURL+"/api/v1/workflows/search", ...)
        Expect(resp.StatusCode).To(Equal(200))
    })

    It("should inject X-Forwarded-User header for authenticated requests", func() {
        // Test Case: TD-E2E-001-TC5
        // Validates: SOC2 CC8.1 user attribution via header injection

        workflow := createWorkflow(authenticatedClient, "test-workflow")

        // Query audit events for workflow creation
        events := queryAuditEvents("workflow.catalog.created", workflow.WorkflowID)

        // Verify user attribution
        Expect(events).To(HaveLen(1))
        Expect(events[0].User).To(Equal("system:serviceaccount:kubernaut-e2e:datastorage-e2e-sa"))
    })

    It("should handle oauth2-proxy sidecar restart gracefully (HTTP 503)", func() {
        // Test Case: TD-E2E-001-TC6
        // Validates: Resilience to oauth2-proxy failures

        // Kill oauth2-proxy pod
        killOAuth2ProxyPod(namespace)

        // Request should fail with 503 (service unavailable)
        _, err := authenticatedClient.Post(dsBaseURL+"/api/v1/workflows", ...)
        Expect(err).To(HaveOccurred())

        // Wait for pod to restart
        Eventually(func() bool {
            return isOAuth2ProxyReady(namespace)
        }, "30s", "1s").Should(BeTrue())

        // Retry request should succeed
        workflow := createWorkflow(authenticatedClient, "test-workflow-after-restart")
        Expect(workflow).ToNot(BeNil())
    })
})
```

**Success Criteria**:
- ✅ 6 new E2E tests for OAuth failure scenarios
- ✅ Tests validate 401/403/503 error handling
- ✅ Tests validate read-only endpoints bypass auth
- ✅ Tests validate `X-Forwarded-User` header injection (SOC2 compliance)
- ✅ Tests validate oauth2-proxy resilience

---

### **Phase 3: Documentation Updates** ⏱️ 1-2 hours

**Files to Update**:
1. `docs/development/SOC2/DD-AUTH-007_OAUTH_PROXY_MIGRATION.md` - Mark E2E migration as complete
2. `docs/testing/e2e/E2E_OAUTH_AUTHENTICATION.md` - New doc explaining E2E OAuth setup
3. `test/e2e/datastorage/README.md` - Update with OAuth test instructions
4. `docs/technical-debt/TD-E2E-001-OAUTH2-PROXY-TESTING.md` - Close this technical debt issue

---

## 📊 **ENDPOINTS REQUIRING AUTHENTICATION**

**Authority**: `api/openapi/data-storage-v1.yaml`, DD-AUTH-004, DD-AUTH-007

### **Workflow Catalog API** (BR-STORAGE-014)

| Endpoint | Method | Auth Required? | Reason |
|----------|--------|---------------|---------|
| `/api/v1/workflows` | POST | ✅ YES | Create workflow (write operation) |
| `/api/v1/workflows` | GET | ❌ NO | List workflows (read-only) |
| `/api/v1/workflows/{id}` | GET | ❌ NO | Get workflow (read-only) |
| `/api/v1/workflows/{id}/disable` | PATCH | ✅ YES | Disable workflow (write operation) |
| `/api/v1/workflows/search` | POST | ❌ NO | Search workflows (read-only, POST for JSON body) |

### **Audit API** (BR-STORAGE-001 to BR-STORAGE-020)

| Endpoint | Method | Auth Required? | Reason |
|----------|--------|---------------|---------|
| `/api/v1/audit/events` | POST | ❌ NO | System audit events (service-to-service) |
| `/api/v1/audit/export` | GET | ❌ NO | Export audit trail (read-only) |
| `/api/v1/audit/legal-hold` | POST | ✅ YES | Place legal hold (SOC2 CC8.1 user attribution) |
| `/api/v1/audit/legal-hold/{id}` | DELETE | ✅ YES | Release legal hold (SOC2 CC8.1 user attribution) |
| `/api/v1/audit/legal-hold` | GET | ❌ NO | List legal holds (read-only) |

---

## 🎯 **ACCEPTANCE CRITERIA**

**Phase 1 (✅ COMPLETE - January 21, 2026)**:
- [x] OAuth2-proxy pulled from `quay.io` (no build needed)
- [x] DataStorage deploys with oauth2-proxy sidecar in ALL 9 E2E services
- [x] Architecture: Service:8080 → oauth2-proxy:8080 → DataStorage:8081
- [x] Pass-through mode (`--skip-auth-regex=.*`) validated

**Phase 2 (⏳ DEFERRED - Post-BR-HAPI-197)**:
- [ ] ServiceAccounts + RBAC created for each E2E service
- [ ] E2E clients use real ServiceAccount tokens (not mock headers)

**Tests**:
- [ ] 6+ E2E tests for OAuth failure scenarios (401/403/503)
- [ ] Tests validate selective endpoint protection (read-only bypass)
- [ ] Tests validate `X-Forwarded-User` header injection
- [ ] Tests validate oauth2-proxy resilience (restart/crash)

**Documentation**:
- [ ] DD-AUTH-007 marked as complete
- [ ] E2E OAuth authentication guide created
- [ ] Technical debt closed

---

## 📋 **RELATED DOCUMENTATION**

- **DD-AUTH-007**: OAuth Proxy Migration (ose-oauth-proxy → oauth2-proxy)
- **DD-AUTH-004**: OpenShift OAuth-Proxy Legal Hold
- **DD-AUTH-005**: DataStorage Client Authentication Pattern
- **BR-STORAGE-014**: Workflow Catalog Management
- **BR-AUDIT-006**: Legal Hold Capability (SOC2 Gap #8)
- **SOC2 CC8.1**: User Attribution Requirements

---

## 💡 **IMPLEMENTATION NOTES**

### **Why Defer to Technical Debt?**
1. **Scope**: OAuth2-proxy infrastructure is substantial work (11-16 hours)
2. **Risk**: Low immediate risk - existing E2E tests use mock auth (validates handler logic)
3. **Priority**: BR-HAPI-197 E2E tests blocked on other work, not OAuth infrastructure
4. **Separation**: OAuth infrastructure deserves focused sprint, not rushed alongside feature work

### **Temporary Mitigation**
- E2E tests use `testauth.NewMockUserTransport()` to simulate `X-Forwarded-User` headers
- Handler logic is validated (just not the oauth2-proxy itself)
- OAuth2-proxy is industry-standard (CNCF project, well-tested)

### **Timeline Estimate**
- **Phase 1** (Infrastructure): 4-6 hours
- **Phase 2** (OAuth Tests): 6-8 hours
- **Phase 3** (Documentation): 1-2 hours
- **Total**: 11-16 hours (~2 days)

---

## ✅ **NEXT STEPS**

**Phase 1** (✅ COMPLETE - January 21, 2026):
1. ✅ File as technical debt (this document)
2. ✅ Create OAuth2-proxy infrastructure (public registry pull)
3. ✅ Update ALL 9 E2E services with oauth2-proxy sidecar
4. ✅ Verify compilation and architecture parity

**Phase 2** (⏳ DEFERRED - After BR-HAPI-197, Before BR-SCOPE-001):
1. ⏳ Implement real ServiceAccount token validation
2. ⏳ Add RBAC/SAR enforcement
3. ⏳ Implement 6+ OAuth failure tests (401/403/503)
4. ⏳ Update documentation (DD-AUTH-007 completion)

**Priority**: HIGH (production parity issue - Phase 1 complete, Phase 2 deferred)
**Risk**: LOW (Phase 1 complete = architecture validated, Phase 2 = auth logic)
**Remaining Effort**: Phase 2: ~8-10 hours (focused sprint)
