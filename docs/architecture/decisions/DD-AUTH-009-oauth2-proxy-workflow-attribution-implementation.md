# DD-AUTH-009: OpenShift OAuth-Proxy & Workflow Catalog User Attribution

**Date**: January 26, 2026
**Status**: ✅ **IMPLEMENTED** - Using Custom Multi-Arch Build
**Version**: 2.0 (Updated January 26, 2026)
**Authority**: AUTHORITATIVE Implementation Plan
**Related**: DD-AUTH-007 (OAuth2-Proxy Migration), DD-AUTH-008 (Secret Management), DD-AUTH-004 (DataStorage OAuth), DD-AUTH-010 (E2E Real Auth), DD-AUTH-011 (Granular RBAC)

---

## 📋 **EXECUTIVE SUMMARY**

**Objective**: Use OpenShift oauth-proxy with multi-architecture support for Kubernetes SubjectAccessReview (SAR) enforcement and workflow catalog user attribution.

**Critical Decision**: Reverted from CNCF `oauth2-proxy` to OpenShift `origin-oauth-proxy` due to SAR requirements.

**Scope**:
1. Use `quay.io/jordigilh/ose-oauth-proxy:latest` (custom arm64+amd64 build) for dev/E2E
2. Use `quay.io/openshift/origin-oauth-proxy:latest` (official amd64) for production Helm charts
3. Enable Kubernetes SubjectAccessReview (SAR) with `--openshift-sar` flag
4. Implement Kustomize-based secret management (DD-AUTH-008)
5. Add workflow catalog user attribution (`created_by`, `updated_by`)
6. Update NetworkPolicy for cross-namespace DataStorage access

**Estimated Effort**: 10-12 hours (with DD-AUTH-010 real authentication + DD-AUTH-011 granular RBAC)

---

## 📝 **CHANGELOG**

### **Version 2.0 - January 26, 2026**

**CRITICAL CHANGE**: Reverted from CNCF `oauth2-proxy` to OpenShift `origin-oauth-proxy`

**Reason**:
- ❌ CNCF `oauth2-proxy:v7.5.1` does **NOT** support `--openshift-sar` flag
- ❌ SAR (SubjectAccessReview) enforcement is **required** for Kubernetes RBAC authorization
- ✅ OpenShift `origin-oauth-proxy` natively supports `--openshift-sar` for SAR validation

**Solution - Multi-Architecture Strategy**:
| Environment | Image | Architecture | SAR Support |
|-------------|-------|--------------|-------------|
| **Development** | `quay.io/jordigilh/ose-oauth-proxy:latest` | arm64 + amd64 | ✅ Yes |
| **E2E Tests** | `quay.io/jordigilh/ose-oauth-proxy:latest` | arm64 + amd64 | ✅ Yes |
| **Production** | `quay.io/openshift/origin-oauth-proxy:latest` | amd64 only | ✅ Yes |

**Custom Build**: `quay.io/jordigilh/ose-oauth-proxy:latest`
- Built from OpenShift `origin-oauth-proxy` source
- Adds arm64 architecture support for local development on Apple Silicon
- Maintains full OpenShift oauth-proxy feature parity (SAR, OIDC, etc.)
- Used for E2E testing to ensure production behavior

**Impact**:
- ✅ Unblocks SAR validation in E2E tests
- ✅ Enables real Kubernetes RBAC enforcement
- ✅ Maintains multi-arch support for development
- ✅ No code changes required (headers remain `X-Auth-Request-User`)

**Related Documents**:
- DD-AUTH-010: E2E Real Authentication Mandate
- DD-AUTH-011: Granular RBAC and SAR Verb Mapping

---

## 🚨 **CRITICAL UPDATE**

**DD-AUTH-010 Mandate**: E2E tests MUST use real ServiceAccount tokens and SAR enforcement (no pass-through mode).

**Impact**: Implementation effort increased from 4-6 hours to 10-12 hours due to:
- Real ServiceAccount token handling
- RBAC creation for each E2E suite
- Removal of pass-through mode from oauth2-proxy
- E2E client authentication updates

---

## 🎯 **BUSINESS REQUIREMENTS**

| Requirement | Description | Priority |
|-------------|-------------|----------|
| **BR-SECURITY-015** | Multi-architecture support (ARM64 + AMD64) | P0 |
| **BR-SOC2-CC8.1** | User attribution for audit trail | P0 |
| **BR-STORAGE-014** | Workflow catalog management with user tracking | P1 |
| **BR-SECURITY-016** | Secure secret management (not in Git) | P0 |

---

## 📊 **CURRENT STATE**

### **Services Using origin-oauth-proxy**

| Service | Image | Namespace | OIDC Provider | User Header | SAR Support |
|---------|-------|-----------|---------------|-------------|-------------|
| **DataStorage** | `quay.io/openshift/origin-oauth-proxy:latest` | `kubernaut-system` | K8s/OpenShift OAuth | `X-Auth-Request-User` | ✅ Yes |
| **HolmesGPT API** | `quay.io/openshift/origin-oauth-proxy:latest` | `kubernaut-system` | K8s/OpenShift OAuth | `X-Auth-Request-User` | ✅ Yes |

**Architecture**: amd64 only (no arm64 support for local development)

### **Secrets Management**

```yaml
# Current: Hardcoded placeholder in Git
apiVersion: v1
kind: Secret
metadata:
  name: data-storage-oauth-proxy-secret
stringData:
  cookie-secret: "REPLACE_ME_WITH_RANDOM_32_BYTE_BASE64_STRING=="
```

**Problems**:
- ❌ Placeholder value in Git (must be manually replaced)
- ❌ **No ARM64 support** (blocks local development on Apple Silicon)
- ❌ Workflow catalog missing user attribution
- ❌ E2E tests cannot validate SAR on arm64

---

## 🎯 **TARGET STATE**

### **Multi-Architecture OpenShift OAuth-Proxy Strategy**

| Environment | Service | Image | Arch | OIDC Provider | User Header | SAR Support |
|-------------|---------|-------|------|---------------|-------------|-------------|
| **Development** | DataStorage | `quay.io/jordigilh/ose-oauth-proxy:latest` | arm64+amd64 | K8s OAuth | `X-Auth-Request-User` | ✅ Yes |
| **Development** | HAPI | `quay.io/jordigilh/ose-oauth-proxy:latest` | arm64+amd64 | K8s OAuth | `X-Auth-Request-User` | ✅ Yes |
| **E2E Tests** | DataStorage | `quay.io/jordigilh/ose-oauth-proxy:latest` | arm64+amd64 | K8s OAuth | `X-Auth-Request-User` | ✅ Yes |
| **E2E Tests** | HAPI | `quay.io/jordigilh/ose-oauth-proxy:latest` | arm64+amd64 | K8s OAuth | `X-Auth-Request-User` | ✅ Yes |
| **Production** | DataStorage | `quay.io/openshift/origin-oauth-proxy:latest` | amd64 | OpenShift OAuth | `X-Auth-Request-User` | ✅ Yes |
| **Production** | HAPI | `quay.io/openshift/origin-oauth-proxy:latest` | amd64 | OpenShift OAuth | `X-Auth-Request-User` | ✅ Yes |

**Benefits**:
- ✅ **SAR Enforcement**: `--openshift-sar` flag enables Kubernetes RBAC validation
- ✅ **Multi-Arch Dev**: Custom build supports arm64 for Apple Silicon Macs
- ✅ **Production Ready**: Official OpenShift image for production Helm charts
- ✅ **E2E Validation**: Tests use same proxy as production (validates SAR logic)

### **Secrets Management**

```bash
# Kustomize generates secrets dynamically
kubectl apply -k deploy/secrets/
# Creates: Secrets with random 32-byte values (NOT in Git)
```

### **Workflow Catalog User Attribution**

```go
// Handlers extract user from oauth2-proxy header
createdBy := r.Header.Get("X-Auth-Request-User")
workflow.CreatedBy = &createdBy
```

---

## 📋 **IMPLEMENTATION PLAN**

### **Phase 0: RBAC Update** (15 minutes) ⚠️ **CRITICAL - FIRST**

**Authority**: DD-AUTH-010 (E2E Real Authentication Mandate)

#### **Task 0.1: Update data-storage-client ClusterRole**

**File**: `deploy/data-storage/client-rbac.yaml:33-38`

**Change**:
```yaml
# OLD (Will cause 403 Forbidden with SAR verb:"*")
rules:
  - apiGroups: [""]
    resources: ["services"]
    resourceNames: ["data-storage-service"]
    verbs: ["get"]

# NEW (Required for workflow catalog CRUD + audit operations)
rules:
  - apiGroups: [""]
    resources: ["services"]
    resourceNames: ["data-storage-service"]
    verbs: ["*"]
```

**Rationale**: 
- OAuth2-proxy SAR checks: `verb:"*"` (all operations)
- RBAC must allow all verbs for SAR to pass
- Without this, ALL E2E tests will get HTTP 403 Forbidden

**Success Criteria**:
- ✅ RBAC updated to allow all verbs
- ✅ No breaking changes to existing services

---

### **Phase 1: Secret Management Setup** (30 minutes)

**Authority**: DD-AUTH-008

#### **Task 1.1: Deploy Kustomize Secrets**

```bash
# Test secret generation
kubectl apply -k deploy/secrets/ --dry-run=client -o yaml

# Verify no hardcoded values
grep -r "REPLACE_ME" deploy/secrets/
# Expected: No matches

# Deploy secrets
kubectl apply -k deploy/secrets/

# Verify secrets created
kubectl get secrets -n kubernaut-system | grep oauth-proxy
```

**Success Criteria**:
- ✅ Secrets exist in cluster
- ✅ Secret values are 32 bytes (random)
- ✅ No hardcoded values in Git

---

### **Phase 2: DataStorage OAuth2-Proxy Migration** (1 hour)

**Authority**: DD-AUTH-007

#### **Task 2.1: Update Deployment Manifest**

**File**: `deploy/data-storage/deployment.yaml`

**Changes**:

```yaml
# OLD (OpenShift origin-oauth-proxy - amd64 only)
- name: oauth-proxy
  image: quay.io/openshift/origin-oauth-proxy:latest
  args:
    - --provider=openshift
    - --openshift-service-account=data-storage-sa
    - --cookie-secret-file=/etc/oauth-proxy/cookie-secret
    - --openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"get"}

# NEW (Custom multi-arch build for dev/E2E + official for production)
- name: oauth-proxy
  # Development/E2E: quay.io/jordigilh/ose-oauth-proxy:latest (arm64+amd64)
  # Production: quay.io/openshift/origin-oauth-proxy:latest (amd64)
  image: quay.io/jordigilh/ose-oauth-proxy:latest
  imagePullPolicy: IfNotPresent
  args:
    - --http-address=0.0.0.0:8080
    - --upstream=http://localhost:8081
    - --provider=oidc
    - --oidc-issuer-url=https://kubernetes.default.svc/.well-known/openid-configuration
    - --skip-oidc-discovery=true
    - --cookie-secret-file=/etc/oauth-proxy/cookie-secret
    - --cookie-name=_oauth_proxy_ds
    - --cookie-expire=24h0m0s
    - --email-domain=*
    - --skip-provider-button=true
    - --skip-auth-route=^/health$
    - --set-xauthrequest=true  # Injects X-Auth-Request-User header
    - --set-authorization-header=true
    - --pass-user-headers=true
    - --pass-access-token=false
    # DD-AUTH-011: SAR for DataStorage with verb:"create"
    - --openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"create"}
```

**Key Changes**:
1. ✅ Image: `quay.io/jordigilh/ose-oauth-proxy:latest` (custom multi-arch build)
2. ✅ Provider: `oidc` (Kubernetes/OpenShift OAuth)
3. ✅ SAR flag: `--openshift-sar` (**REQUIRES** OpenShift oauth-proxy, not supported by CNCF oauth2-proxy)
4. ✅ SAR verb: `create` (DD-AUTH-011 strategy - all services need audit write access)
5. ✅ Header injection: `--set-xauthrequest=true` → `X-Auth-Request-User` (unchanged)

#### **Task 2.2: Test DataStorage**

```bash
# Apply updated deployment
kubectl apply -f deploy/data-storage/deployment.yaml

# Watch pod startup
kubectl get pods -n kubernaut-system -l app=data-storage-service -w

# Verify oauth-proxy container running
kubectl get pods -n kubernaut-system -l app=data-storage-service \
  -o jsonpath='{.items[0].status.containerStatuses[?(@.name=="oauth-proxy")].ready}'
# Expected: true

# Check oauth-proxy logs
kubectl logs -n kubernaut-system deployment/data-storage-service -c oauth-proxy
# Expected: "OAuthProxy configured..." (no errors)
```

**Success Criteria**:
- ✅ Pod runs successfully
- ✅ oauth2-proxy container ready
- ✅ No error logs

---

### **Phase 3: HolmesGPT API OAuth2-Proxy Migration** (1 hour)

**Authority**: DD-AUTH-007

#### **Task 3.1: Update Deployment Manifest**

**File**: `deploy/holmesgpt-api/06-deployment.yaml`

**Changes**: Same as DataStorage (adjusted for HAPI-specific configuration)

```yaml
- name: oauth-proxy
  image: quay.io/oauth2-proxy/oauth2-proxy:v7.5.1
  args:
    - --http-address=0.0.0.0:8080
    - --upstream=http://localhost:8081
    - --provider=oidc
    - --oidc-issuer-url=https://kubernetes.default.svc/.well-known/openid-configuration
    - --skip-oidc-discovery=true
    - --cookie-secret-file=/etc/oauth-proxy/cookie-secret
    - --cookie-name=_oauth_proxy_hapi
    - --cookie-expire=24h0m0s
    - --email-domain=*
    - --skip-auth-route=^/health$
    - --set-xauthrequest=true
    - --set-authorization-header=true
    - --pass-user-headers=true
    # SAR for HAPI: LLM cost attribution
    - --openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"holmesgpt-api","verb":"get"}
```

#### **Task 3.2: Test HolmesGPT API**

```bash
kubectl apply -f deploy/holmesgpt-api/06-deployment.yaml
kubectl get pods -n kubernaut-system -l app.kubernetes.io/name=holmesgpt-api -w
```

---

### **Phase 4: Workflow Catalog User Attribution** (1.5 hours)

**Authority**: BR-SOC2-CC8.1, BR-STORAGE-014

#### **Task 4.1: Update Workflow Handlers**

**File**: `pkg/datastorage/server/workflow_handlers.go`

```go
// HandleCreateWorkflow - Add user extraction
func (h *Handler) HandleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
    // 1. Extract K8s user from oauth2-proxy header
    // DD-AUTH-004: OAuth2-proxy injects this header after JWT validation + SAR
    createdBy := r.Header.Get("X-Auth-Request-User")
    if createdBy == "" {
        h.logger.Info("Workflow creation rejected: missing X-Auth-Request-User header")
        response.WriteRFC7807Error(w, http.StatusUnauthorized, "unauthorized", "Unauthorized",
            "X-Auth-Request-User header required for workflow operations (missing authentication)", h.logger)
        return
    }

    // 2. Parse request body
    var workflow models.RemediationWorkflow
    if err := json.NewDecoder(r.Body).Decode(&workflow); err != nil {
        h.logger.Error(err, "Failed to decode workflow create request")
        response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
            fmt.Sprintf("Invalid request body: %v", err), h.logger)
        return
    }

    // 3. Validate required fields
    if err := h.validateCreateWorkflowRequest(&workflow); err != nil {
        h.logger.Error(err, "Invalid workflow create request",
            "workflow_name", workflow.WorkflowName,
        )
        response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request", err.Error(), h.logger)
        return
    }

    // 4. Set created_by from authenticated user
    workflow.CreatedBy = &createdBy

    // 5. Set default status if not provided
    if workflow.Status == "" {
        workflow.Status = "active"
    }

    // 6. Create workflow in repository
    if err := h.workflowRepo.Create(r.Context(), &workflow); err != nil {
        // Handle errors (duplicate, etc.)
        var pgErr *pgconn.PgError
        if errors.As(err, &pgErr) && pgErr.Code == "23505" {
            detail := fmt.Sprintf("Workflow '%s' version '%s' already exists", workflow.WorkflowName, workflow.Version)
            response.WriteRFC7807Error(w, http.StatusConflict, "conflict",
                "Workflow Already Exists", detail, h.logger)
            return
        }

        h.logger.Error(err, "Failed to create workflow",
            "workflow_name", workflow.WorkflowName,
            "version", workflow.Version,
            "created_by", createdBy,
        )
        response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error",
            "Internal Server Error", "Failed to create workflow", h.logger)
        return
    }

    // 7. Return success response
    w.Header().Set("Content-Type", "application/json")
    w.WriteStatus(http.StatusCreated)
    json.NewEncoder(w).Encode(workflow)
}
```

**Similar updates for**:
- `HandleUpdateWorkflow()` - Set `updated_by`
- `HandleDeleteWorkflow()` - Log `deleted_by` for audit trail

#### **Task 4.2: Update Integration Tests**

**File**: `test/integration/datastorage/workflow_integration_test.go`

```go
It("should set created_by from X-Auth-Request-User header", func() {
    req := httptest.NewRequest("POST", "/api/v1/workflows", body)
    req.Header.Set("X-Auth-Request-User", "test-operator@kubernaut.ai")
    
    resp, err := client.Do(req)
    Expect(err).ToNot(HaveOccurred())
    Expect(resp.StatusCode).To(Equal(http.StatusCreated))
    
    var workflow models.RemediationWorkflow
    json.NewDecoder(resp.Body).Decode(&workflow)
    Expect(workflow.CreatedBy).ToNot(BeNil())
    Expect(*workflow.CreatedBy).To(Equal("test-operator@kubernaut.ai"))
})

It("should return 401 if X-Auth-Request-User header missing", func() {
    req := httptest.NewRequest("POST", "/api/v1/workflows", body)
    // NO X-Auth-Request-User header
    
    resp, err := client.Do(req)
    Expect(err).ToNot(HaveOccurred())
    Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
})
```

---

### **Phase 5: E2E Real Authentication** (3 hours) ⚠️ **NEW - DD-AUTH-010**

**Authority**: DD-AUTH-010 (E2E Real Authentication Mandate)

#### **Task 5.1: Create ServiceAccount Transport**

**File**: `test/shared/auth/serviceaccount_transport.go` (NEW)

```go
package auth

import (
    "fmt"
    "net/http"
)

// ServiceAccountTransport implements http.RoundTripper for E2E tests.
// Uses real Kubernetes ServiceAccount tokens (not mock headers).
//
// Authority: DD-AUTH-010 (E2E Real Authentication Mandate)
type ServiceAccountTransport struct {
    base  http.RoundTripper
    token string
}

// NewServiceAccountTransport creates a transport that uses ServiceAccount Bearer tokens.
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

#### **Task 5.2: Create E2E ServiceAccount Helper**

**File**: `test/infrastructure/serviceaccount.go` (NEW)

```go
// CreateE2EServiceAccountWithDataStorageAccess creates SA + RBAC for E2E tests
func CreateE2EServiceAccountWithDataStorageAccess(ctx context.Context, namespace, kubeconfigPath, saName string, writer io.Writer) error {
    clientset, err := getKubernetesClient(kubeconfigPath)
    if err != nil {
        return err
    }

    // 1. Create ServiceAccount
    sa := &corev1.ServiceAccount{
        ObjectMeta: metav1.ObjectMeta{
            Name:      saName,
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
            Name:      fmt.Sprintf("%s-datastorage-access", saName),
            Namespace: namespace,
        },
        RoleRef: rbacv1.RoleRef{
            APIGroup: "rbac.authorization.k8s.io",
            Kind:     "ClusterRole",
            Name:     "data-storage-client",
        },
        Subjects: []rbacv1.Subject{
            {
                Kind:      "ServiceAccount",
                Name:      saName,
                Namespace: namespace,
            },
        },
    }
    _, err = clientset.RbacV1().RoleBindings(namespace).Create(ctx, rb, metav1.CreateOptions{})
    if err != nil && !apierrors.IsAlreadyExists(err) {
        return fmt.Errorf("failed to create E2E RoleBinding: %w", err)
    }

    fmt.Fprintf(writer, "✅ E2E ServiceAccount + RBAC created: %s\n", saName)
    return nil
}

// GetServiceAccountToken retrieves the token for a ServiceAccount
func GetServiceAccountToken(ctx context.Context, namespace, saName, kubeconfigPath string) (string, error) {
    clientset, err := getKubernetesClient(kubeconfigPath)
    if err != nil {
        return "", err
    }

    // Get ServiceAccount
    sa, err := clientset.CoreV1().ServiceAccounts(namespace).Get(ctx, saName, metav1.GetOptions{})
    if err != nil {
        return "", fmt.Errorf("failed to get ServiceAccount: %w", err)
    }

    // Wait for token secret to be created by K8s
    var tokenSecret string
    for i := 0; i < 10; i++ {
        secrets, err := clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
        if err != nil {
            return "", err
        }
        for _, secret := range secrets.Items {
            if secret.Type == corev1.SecretTypeServiceAccountToken {
                if ownerRefs := secret.OwnerReferences; len(ownerRefs) > 0 {
                    if ownerRefs[0].UID == sa.UID {
                        tokenSecret = secret.Name
                        break
                    }
                }
            }
        }
        if tokenSecret != "" {
            break
        }
        time.Sleep(1 * time.Second)
    }

    if tokenSecret == "" {
        return "", fmt.Errorf("ServiceAccount token secret not found")
    }

    // Get token from secret
    secret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, tokenSecret, metav1.GetOptions{})
    if err != nil {
        return "", fmt.Errorf("failed to get token secret: %w", err)
    }

    token, ok := secret.Data["token"]
    if !ok {
        return "", fmt.Errorf("token key not found in secret")
    }

    return string(token), nil
}
```

#### **Task 5.3: Remove Pass-Through Mode from E2E**

**File**: `test/infrastructure/datastorage.go:1079-1086`

```go
// OLD (Pass-through mode)
Args: []string{
    "--skip-auth-regex=.*",  // ❌ REMOVE
    "--provider=google",     // ❌ REMOVE
    "--client-id=e2e-test-client",  // ❌ REMOVE
},

// NEW (Real authentication - DD-AUTH-010)
Args: []string{
    "--http-address=0.0.0.0:8080",
    "--upstream=http://localhost:8081",
    "--provider=oidc",
    "--oidc-issuer-url=https://kubernetes.default.svc/.well-known/openid-configuration",
    "--skip-oidc-discovery=true",
    "--cookie-secret=0123456789ABCDEF0123456789ABCDEF",  // E2E static secret (OK for tests)
    "--cookie-name=_oauth_proxy_ds_e2e",
    "--cookie-expire=24h0m0s",
    "--email-domain=*",
    "--skip-provider-button=true",
    "--skip-auth-route=^/health$",  // ✅ Only skip health checks
    // ENABLE SAR for E2E
    "--openshift-sar={\"namespace\":\"" + namespace + "\",\"resource\":\"services\",\"resourceName\":\"data-storage-service\",\"verb\":\"*\"}",
    "--set-xauthrequest=true",
    "--set-authorization-header=true",
    "--pass-user-headers=true",
    "--pass-access-token=false",
},
```

#### **Task 5.4: Update DataStorage E2E Suite**

**File**: `test/e2e/datastorage/datastorage_e2e_suite_test.go`

**Changes**:

```go
var _ = SynchronizedBeforeSuite(
    func() []byte {
        // ... existing setup ...

        // DD-AUTH-010: Create E2E ServiceAccount + RBAC
        logger.Info("📋 DD-AUTH-010: Creating E2E ServiceAccount with DataStorage access...")
        err = infrastructure.CreateE2EServiceAccountWithDataStorageAccess(
            ctx, 
            sharedNamespace, 
            kubeconfigPath,
            "datastorage-e2e-sa",
            writer,
        )
        Expect(err).ToNot(HaveOccurred(), "Failed to create E2E ServiceAccount")
        logger.Info("✅ E2E ServiceAccount + RBAC created")

        // ... continue with rest of setup ...
    },
    func(kubeconfigBytes []byte) {
        // ... existing setup ...

        // DD-AUTH-010: Use real ServiceAccount token (not mock transport)
        logger.Info("📋 DD-AUTH-010: Authenticating with real ServiceAccount token...")
        tokenBytes, err := infrastructure.GetServiceAccountToken(
            ctx,
            sharedNamespace,
            "datastorage-e2e-sa",
            kubeconfigPath,
        )
        Expect(err).ToNot(HaveOccurred(), "Failed to get ServiceAccount token")
        logger.Info("✅ ServiceAccount token retrieved")

        // Create authenticated transport (real tokens, not mock)
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
        logger.Info("✅ DataStorage client authenticated with ServiceAccount token")
    },
)
```

---

### **Phase 6: NetworkPolicy Update** (30 minutes)

**Authority**: Cross-namespace access requirement (Notification controller in kubernaut-notifications)

#### **Task 5.1: Update DataStorage NetworkPolicy**

**File**: `deploy/data-storage/networkpolicy.yaml`

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: data-storage-network-policy
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: data-storage-service
  policyTypes:
    - Ingress
    - Egress
  ingress:
    # Allow from all kubernaut services in kubernaut-system namespace
    - from:
        - podSelector: {}  # All pods in kubernaut-system
      ports:
        - protocol: TCP
          port: 8080  # oauth-proxy port
    
    # Allow from Notification controller (kubernaut-notifications namespace)
    - from:
        - namespaceSelector:
            matchLabels:
              app.kubernetes.io/name: kubernaut
              app.kubernetes.io/component: notification-controller
      ports:
        - protocol: TCP
          port: 8080
    
    # Allow Prometheus scraping from monitoring namespace
    - from:
        - namespaceSelector:
            matchLabels:
              name: monitoring
      ports:
        - protocol: TCP
          port: 9090  # metrics port (direct, no auth)
  
  egress:
    # Allow egress to PostgreSQL
    - to:
        - podSelector:
            matchLabels:
              app: postgres
      ports:
        - protocol: TCP
          port: 5432
    
    # Allow egress to Redis (DLQ)
    - to:
        - podSelector:
            matchLabels:
              app: redis
      ports:
        - protocol: TCP
          port: 6379
    
    # Allow DNS
    - to:
        - namespaceSelector:
            matchLabels:
              name: kube-system
      ports:
        - protocol: UDP
          port: 53
```

**Key Changes**:
1. ✅ Allow ALL pods in `kubernaut-system` namespace
2. ✅ Allow Notification controller from `kubernaut-notifications` namespace
3. ✅ Maintain separation: oauth-proxy port (8080) vs metrics port (9090)

---

## ✅ **VALIDATION CHECKLIST**

### **RBAC (Phase 0)**
- [ ] `data-storage-client` ClusterRole updated to `verbs: ["*"]`
- [ ] All existing ServiceAccount RoleBindings still work
- [ ] No breaking changes to deployed services

### **Secret Management (Phase 1)**
- [ ] Secrets generated dynamically by Kustomize
- [ ] Secret values are 32 bytes (random)
- [ ] No hardcoded secrets in Git
- [ ] `helm template` does NOT show secret values

### **OAuth2-Proxy Migration (Phase 2-3)**
- [ ] DataStorage using `oauth2-proxy:v7.5.1`
- [ ] HolmesGPT API using `oauth2-proxy:v7.5.1`
- [ ] Pods running successfully (all containers ready)
- [ ] No error logs from oauth2-proxy containers
- [ ] User headers injected correctly (`X-Auth-Request-User`)
- [ ] SAR enforcement enabled (`verb:"*"`)

### **Workflow Catalog User Attribution (Phase 4)**
- [ ] `created_by` field populated from header
- [ ] `updated_by` field populated from header
- [ ] Returns 401 if header missing
- [ ] Integration tests pass

### **E2E Real Authentication (Phase 5)** ⚠️ **NEW - DD-AUTH-010**
- [ ] `ServiceAccountTransport` implemented (real tokens)
- [ ] E2E ServiceAccount + RBAC created per suite
- [ ] OAuth2-proxy SAR enabled (no pass-through mode)
- [ ] E2E clients use real ServiceAccount tokens
- [ ] No `MockUserTransport` in E2E tests
- [ ] E2E tests validate 401 (unauthenticated)
- [ ] E2E tests validate 403 (insufficient RBAC)
- [ ] E2E tests pass with real authentication

### **NetworkPolicy (Phase 6)**
- [ ] Notification controller can access DataStorage
- [ ] All kubernaut-system services can access DataStorage
- [ ] Metrics port accessible from monitoring namespace

---

## 🧪 **TESTING STRATEGY**

### **Unit Tests**
```bash
go test ./pkg/datastorage/server/... -v -run TestWorkflow
```

### **Integration Tests**
```bash
make test-integration-datastorage
```

### **E2E Tests**
```bash
make test-e2e-datastorage
```

### **Manual Verification**
```bash
# Test workflow creation with user attribution
curl -X POST http://localhost:8080/api/v1/workflows \
  -H "X-Auth-Request-User: admin@kubernaut.ai" \
  -H "Content-Type: application/json" \
  -d '{"workflow_name":"test-workflow","version":"1.0.0"}'
```

---

## 📊 **ROLLBACK PLAN**

If issues arise during deployment:

```bash
# Rollback DataStorage
kubectl rollout undo deployment/data-storage-service -n kubernaut-system

# Rollback HolmesGPT API
kubectl rollout undo deployment/holmesgpt-api -n kubernaut-system

# Revert to old secrets
kubectl apply -f deploy/data-storage/oauth-proxy-secret.yaml
kubectl apply -f deploy/holmesgpt-api/13-oauth-proxy-secret.yaml
```

---

## 🎯 **SUCCESS CRITERIA**

### **Production Deployment**
1. ✅ Both services using `oauth2-proxy:v7.5.1` (multi-arch)
2. ✅ Secrets generated dynamically (not in Git)
3. ✅ RBAC `verbs: ["*"]` allows all workflow operations
4. ✅ Workflow catalog tracks `created_by` and `updated_by`
5. ✅ Cross-namespace NetworkPolicy allows Notification access

### **E2E Tests** ⚠️ **CRITICAL - DD-AUTH-010**
6. ✅ OAuth2-proxy enforces real SAR (no pass-through mode)
7. ✅ E2E clients use real ServiceAccount tokens
8. ✅ E2E tests validate authentication (401/403 scenarios)
9. ✅ No `MockUserTransport` in E2E tests (only integration)

### **Testing**
10. ✅ All unit tests pass
11. ✅ All integration tests pass
12. ✅ All E2E tests pass with real authentication
13. ✅ No regressions in existing functionality

---

## 📚 **REFERENCES**

- **[DD-AUTH-008: Secret Management Strategy](./DD-AUTH-008-secret-management-kustomize-helm.md)**
- **[DD-AUTH-007: OAuth2-Proxy Migration](../../development/SOC2/DD-AUTH-007_OAUTH_PROXY_MIGRATION.md)**
- **[DD-AUTH-004: DataStorage OAuth-Proxy](./DD-AUTH-004-openshift-oauth-proxy-legal-hold.md)**
- **[BR-SOC2-CC8.1: User Attribution](../../requirements/11_SECURITY_ACCESS_CONTROL.md)**
- **[OAuth2-Proxy Documentation](https://oauth2-proxy.github.io/oauth2-proxy/docs/)**

---

**Document Version**: 1.0  
**Last Updated**: January 26, 2026  
**Status**: ✅ **APPROVED** - Ready for Implementation  
**Estimated Effort**: 4-6 hours  
**Next Step**: Begin Phase 1 (Secret Management Setup)
