# DD-AUTH-011 Implementation Plan: Granular RBAC with Audit Tracking

**Date**: January 26, 2026
**Status**: ✅ **APPROVED** - Ready for Implementation
**Authority**: DD-AUTH-011 (User Approved)
**Estimated Effort**: 3-4 hours

---

## 🎯 **APPROVED APPROACH**

**Single OAuth2-Proxy + Single RBAC ClusterRole + Audit Tracking**

### **Key Decisions**

1. ✅ **Single ClusterRole**: `data-storage-client` with `verbs: ["create"]`
2. ✅ **All 8 services** get `create` permission (needed for audit writes)
3. ✅ **Audit logs track workflow access** (user mandate)
4. ✅ **No multi-proxy architecture** (too complex)
5. ✅ **No per-endpoint SAR** (oauth2-proxy limitation)

### **Rationale**

- **ALL services write audit events** → Need `create` permission
- **OAuth2-proxy SAR limitation** → Single global SAR check only
- **Workflow access visibility** → Audit logs capture all operations
- **User mandate**: "For now we can use the audit traces to track who interacts with these endpoints"

---

## 📋 **IMPLEMENTATION PHASES**

### **Phase 0: Backup Current RBAC** (5 min)

```bash
# Backup current RBAC configuration
kubectl get clusterrole data-storage-client -o yaml > backup/data-storage-client-backup-$(date +%Y%m%d).yaml
kubectl get rolebindings -n kubernaut-system -l app=data-storage-service -o yaml > backup/data-storage-rolebindings-backup-$(date +%Y%m%d).yaml
```

---

### **Phase 1: Update RBAC ClusterRole** (30 min)

#### **Task 1.1: Deploy New RBAC Configuration**

```bash
# Deploy updated RBAC (DD-AUTH-011)
kubectl apply -f deploy/data-storage/client-rbac-v2.yaml

# Verify ClusterRole
kubectl get clusterrole data-storage-client -o yaml
# Expected: verbs: ["create"]

# Verify RoleBindings (should be 14 total: 8 services + 6 E2E)
kubectl get rolebindings -n kubernaut-system -l component=rbac | grep data-storage-client | wc -l
# Expected: 14
```

#### **Task 1.2: Validate RBAC Permissions**

```bash
# Test Gateway ServiceAccount can "create"
kubectl auth can-i create services/data-storage-service \
  --as=system:serviceaccount:kubernaut-system:gateway-sa \
  -n kubernaut-system
# Expected: yes

# Test HolmesGPT API ServiceAccount can "create"
kubectl auth can-i create services/data-storage-service \
  --as=system:serviceaccount:kubernaut-system:holmesgpt-api-sa \
  -n kubernaut-system
# Expected: yes

# Test E2E ServiceAccount can "create"
kubectl auth can-i create services/data-storage-service \
  --as=system:serviceaccount:kubernaut-system:datastorage-e2e-sa \
  -n kubernaut-system
# Expected: yes
```

**Success Criteria**:
- [ ] ClusterRole has `verbs: ["create"]`
- [ ] All 8 services have RoleBinding
- [ ] All 6 E2E suites have RoleBinding
- [ ] `kubectl auth can-i` returns `yes` for all ServiceAccounts

---

### **Phase 2: Update OAuth2-Proxy SAR** (45 min)

#### **Task 2.1: Update DataStorage Deployment**

**File**: `deploy/data-storage/deployment.yaml`

**Change** (around line 61):
```yaml
# OLD
- --openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"get"}

# NEW
- --openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"create"}
```

```bash
# Apply updated deployment
kubectl apply -f deploy/data-storage/deployment.yaml

# Watch pod restart
kubectl rollout status deployment/data-storage-service -n kubernaut-system

# Verify oauth-proxy container args
kubectl get pods -n kubernaut-system -l app=data-storage-service -o jsonpath='{.items[0].spec.containers[?(@.name=="oauth-proxy")].args}' | jq .
# Expected: Contains --openshift-sar={"...","verb":"create"}
```

#### **Task 2.2: Update HolmesGPT API Deployment** (if needed)

**File**: `deploy/holmesgpt-api/06-deployment.yaml`

Check if HAPI also needs SAR update (currently `verb:"get"`):

```bash
# Check current HAPI oauth-proxy SAR
kubectl get pods -n kubernaut-system -l app=holmesgpt-api -o jsonpath='{.items[0].spec.containers[?(@.name=="oauth-proxy")].args}' | grep openshift-sar
```

If HAPI uses DataStorage, update similarly.

**Success Criteria**:
- [ ] DataStorage oauth-proxy uses `verb:"create"` in SAR
- [ ] Pod restarted successfully
- [ ] No CrashLoopBackOff
- [ ] oauth-proxy logs show no SAR errors

---

### **Phase 3: Update E2E Test Infrastructure** (1.5 hours)

#### **Task 3.1: Create ServiceAccount Transport**

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
//
// Used by: E2E tests to authenticate with oauth2-proxy
// Flow: E2E client → oauth2-proxy (validates token + SAR) → DataStorage
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

#### **Task 3.2: Create E2E ServiceAccount Helpers**

**File**: `test/infrastructure/serviceaccount.go` (NEW)

```go
package infrastructure

import (
    "context"
    "fmt"
    "io"
    "time"

    corev1 "k8s.io/api/core/v1"
    rbacv1 "k8s.io/api/rbac/authorization/k8s.io/v1"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateE2EServiceAccountWithDataStorageAccess creates SA + RBAC for E2E tests
func CreateE2EServiceAccountWithDataStorageAccess(
    ctx context.Context,
    namespace, kubeconfigPath, saName string,
    writer io.Writer,
) error {
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
            Name:     "data-storage-client",  // DD-AUTH-011: verb:"create"
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
func GetServiceAccountToken(
    ctx context.Context,
    namespace, saName, kubeconfigPath string,
) (string, error) {
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
        return "", fmt.Errorf("ServiceAccount token secret not found after 10s")
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

#### **Task 3.3: Remove Pass-Through Mode from E2E**

**File**: `test/infrastructure/datastorage.go:1079-1086`

**Change**:
```go
// OLD (Pass-through mode - TD-E2E-001 Phase 1)
Args: []string{
    "--skip-auth-regex=.*",  // ❌ REMOVE
    "--provider=google",     // ❌ REMOVE
    "--client-id=e2e-test-client",  // ❌ REMOVE
},

// NEW (Real authentication - DD-AUTH-010 + DD-AUTH-011)
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
    // DD-AUTH-011: ENABLE SAR with verb:"create"
    "--openshift-sar={\"namespace\":\"" + namespace + "\",\"resource\":\"services\",\"resourceName\":\"data-storage-service\",\"verb\":\"create\"}",
    "--set-xauthrequest=true",
    "--set-authorization-header=true",
    "--pass-user-headers=true",
    "--pass-access-token=false",
},
```

#### **Task 3.4: Update DataStorage E2E Suite**

**File**: `test/e2e/datastorage/datastorage_e2e_suite_test.go`

**Add to `SynchronizedBeforeSuite`** (first function):
```go
// DD-AUTH-010: Create E2E ServiceAccount + RBAC
logger.Info("📋 DD-AUTH-010/DD-AUTH-011: Creating E2E ServiceAccount with DataStorage access...")
err = infrastructure.CreateE2EServiceAccountWithDataStorageAccess(
    ctx,
    sharedNamespace,
    kubeconfigPath,
    "datastorage-e2e-sa",
    writer,
)
Expect(err).ToNot(HaveOccurred(), "Failed to create E2E ServiceAccount")
logger.Info("✅ E2E ServiceAccount + RBAC created (verb:create)")
```

**Update in `SynchronizedBeforeSuite`** (second function):
```go
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
```

**Success Criteria**:
- [ ] `ServiceAccountTransport` implemented
- [ ] E2E ServiceAccount helpers implemented
- [ ] OAuth2-proxy args updated (no pass-through)
- [ ] E2E suite creates ServiceAccount
- [ ] E2E suite uses real tokens

---

### **Phase 4: Testing & Validation** (1 hour)

#### **Task 4.1: Unit Tests (Existing - No Changes)**

```bash
# Run existing unit tests (should still pass)
make test
# Expected: All pass (no changes to business logic)
```

#### **Task 4.2: E2E Tests with Real Auth**

```bash
# Run DataStorage E2E tests
make test-e2e-datastorage

# Expected behavior:
# ✅ E2E ServiceAccount created
# ✅ RoleBinding created (verb:"create")
# ✅ ServiceAccount token retrieved
# ✅ OAuth2-proxy SAR passes (has "create" permission)
# ✅ All audit write operations succeed
# ✅ All workflow operations succeed (HAPI search, E2E CRUD)
```

#### **Task 4.3: Audit Log Validation**

```bash
# Query DataStorage for workflow audit events
kubectl port-forward -n kubernaut-system svc/data-storage-service 8080:8080

# Get audit events for workflow operations (example using cURL)
curl -H "Authorization: Bearer $(kubectl create token datastorage-e2e-sa -n kubernaut-system)" \
  http://localhost:8080/api/v1/audit/workflow-search?limit=10

# Verify audit events capture:
# - user: system:serviceaccount:kubernaut-system:holmesgpt-api-sa (for HAPI searches)
# - user: system:serviceaccount:kubernaut-system:datastorage-e2e-sa (for E2E CRUD)
# - operation: workflow.created, workflow.searched, workflow.updated
# - timestamp: ISO 8601
```

**Success Criteria**:
- [ ] All E2E tests pass
- [ ] OAuth2-proxy logs show successful SAR checks
- [ ] No 403 Forbidden errors
- [ ] Audit events captured for workflow operations
- [ ] Audit events include user attribution (`X-Auth-Request-User`)

---

## 🎯 **ROLLBACK PLAN**

If issues occur during implementation:

### **Rollback RBAC**
```bash
# Restore previous RBAC configuration
kubectl apply -f backup/data-storage-client-backup-YYYYMMDD.yaml
kubectl apply -f backup/data-storage-rolebindings-backup-YYYYMMDD.yaml
```

### **Rollback OAuth2-Proxy**
```bash
# Revert deployment to previous SAR configuration
git checkout HEAD~1 -- deploy/data-storage/deployment.yaml
kubectl apply -f deploy/data-storage/deployment.yaml
```

### **Rollback E2E Tests**
```bash
# Revert to pass-through mode
git checkout HEAD~1 -- test/infrastructure/datastorage.go
git checkout HEAD~1 -- test/e2e/datastorage/datastorage_e2e_suite_test.go
```

---

## ✅ **SUCCESS CRITERIA**

### **RBAC Validation**
- [ ] ClusterRole `data-storage-client` has `verbs: ["create"]`
- [ ] All 8 services have RoleBinding to `data-storage-client`
- [ ] All 6 E2E suites have RoleBinding
- [ ] `kubectl auth can-i` confirms `create` permission for all ServiceAccounts

### **OAuth2-Proxy SAR**
- [ ] DataStorage oauth-proxy uses `verb:"create"` in SAR
- [ ] Pods running successfully (no CrashLoopBackOff)
- [ ] OAuth-proxy logs show successful SAR checks

### **E2E Tests**
- [ ] E2E tests create ServiceAccounts programmatically
- [ ] E2E tests use real ServiceAccount tokens (no mocks)
- [ ] All E2E suites pass with real authentication
- [ ] No 401/403 errors in E2E test runs

### **Audit Tracking**
- [ ] Workflow operations logged in audit events
- [ ] Audit events include user attribution (`X-Auth-Request-User`)
- [ ] Security team can query audit logs for workflow access patterns

---

## 📋 **POST-IMPLEMENTATION TASKS**

1. **Update Documentation**:
   - Mark DD-AUTH-011 as IMPLEMENTED
   - Update DD-AUTH-009 status (OAuth2-proxy migration complete)
   - Update DD-AUTH-010 status (E2E real auth complete)

2. **Create README for RBAC**:
   - Document new RBAC configuration
   - Explain audit tracking for workflow access
   - Provide examples of audit log queries

3. **Security Review**:
   - Share audit log analysis approach with security team
   - Document workflow access patterns
   - Confirm SOC2 compliance

4. **Deprecate Old Files**:
   - Archive `deploy/data-storage/client-rbac.yaml` (old version)
   - Rename `client-rbac-v2.yaml` → `client-rbac.yaml`

---

## 📊 **EFFORT SUMMARY**

| Phase | Task | Estimated Time |
|-------|------|----------------|
| Phase 0 | Backup Current RBAC | 5 min |
| Phase 1 | Update RBAC ClusterRole | 30 min |
| Phase 2 | Update OAuth2-Proxy SAR | 45 min |
| Phase 3 | Update E2E Test Infrastructure | 1.5 hours |
| Phase 4 | Testing & Validation | 1 hour |
| **TOTAL** | | **~3.5 hours** |

**Confidence**: 90% (straightforward RBAC + OAuth2-proxy configuration changes)

---

**Status**: Ready for implementation
**Next Step**: Execute Phase 0 (Backup Current RBAC)
