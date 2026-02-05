# DataStorage E2E SAR Testing - Triage Report

**Date**: January 26, 2026  
**Status**: 🚧 **IN PROGRESS** - Infrastructure fixes applied, token retrieval needs fixing  
**Activity**: DataStorage E2E test execution with SAR access control validation

---

## 📊 **TEST EXECUTION SUMMARY**

**Last Run**: January 26, 2026 10:34 AM - 10:39 AM  
**Duration**: ~5 minutes  
**Specs Executed**: 56 of 190  
**Results**: 3 Passed | 53 Failed | 1 Pending | 133 Skipped

---

## ✅ **FIXES APPLIED THIS SESSION**

### **1. Test Compilation Errors** ✅
Fixed workflow type usage in `test/e2e/datastorage/23_sar_access_control_test.go`:
- `WorkflowCreateRequest` → `RemediationWorkflow`
- `ListAuditEvents` → `QueryAuditEvents`
- `MandatoryLabels` field types corrected
- **Result**: Test compiles successfully

### **2. ClusterRole Deployment** ✅
Fixed `deployDataStorageClientClusterRole` in `test/infrastructure/datastorage.go`:
- **Problem**: `client-rbac-v2.yaml` contains RoleBindings for `kubernaut-system` namespace (doesn't exist in E2E)
- **Solution**: Use `yq` to extract only ClusterRole, skip RoleBindings
- **Result**: ClusterRole deploys successfully

### **3. Health Check Port Mismatch** ✅
Fixed liveness/readiness probes:
- **Problem**: Probes checked port 8080, DataStorage listens on 8081
- **Solution**: Changed probes to port 8081 (direct to DataStorage, bypass oauth-proxy)
- **Location**: Both `deploy/data-storage/deployment.yaml` and `test/infrastructure/datastorage.go`
- **Result**: Probes now work correctly

### **4. OAuth-Proxy Configuration** ✅
Fixed multiple oauth-proxy issues:
- **Provider**: Changed from `--provider=oidc` to `--provider=openshift` (required for `--openshift-sar`)
- **Bypass flag**: Changed from `--skip-auth-route` (doesn't exist) to `--bypass-auth-for` (ose-oauth-proxy flag)
- **Required flags**: Added `--client-id`, `--client-secret`, `--cookie-secret` (required for config validation)
- **Result**: OAuth-proxy starts and runs correctly

### **5. Health Check Path** ✅
Fixed health check in test setup:
- **Problem**: Checked `/health/ready` but DataStorage only has `/health`
- **Solution**: Changed to `/health`
- **Result**: Health check responds successfully (200 OK)

### **6. ConfigMap Port** ✅
Fixed DataStorage configuration:
- **Problem**: `deploy/data-storage/configmap.yaml` had `port: 8080`
- **Solution**: Changed to `port: 8081` (DD-AUTH-004: internal port behind oauth-proxy)
- **Result**: DataStorage listens on correct port

---

## 🚧 **CURRENT BLOCKER: ServiceAccount Token Retrieval**

### **Problem**
SAR tests fail in `BeforeEach` waiting for ServiceAccount token Secret:
```
⏳ Waiting for token Secret creation...
(repeats until timeout after 30s)
```

### **Root Cause**
**Kubernetes 1.34** (used in Kind) **does NOT auto-create token Secrets** for ServiceAccounts.

- **Changed in K8s 1.24**: Token Secrets are no longer automatically created
- **Current code**: Waits for Secret that will never be created
- **Solution**: Use **TokenRequest API** to generate ephemeral tokens

### **Fix Applied** ✅
Modified `GetServiceAccountToken()` in `test/infrastructure/serviceaccount.go`:
```go
// OLD (broken in K8s 1.24+):
// Wait for auto-created token Secret

// NEW (K8s 1.24+ compliant):
treq := &authenticationv1.TokenRequest{
    Spec: authenticationv1.TokenRequestSpec{
        ExpirationSeconds: func() *int64 { exp := int64(3600); return &exp }(),
    },
}
tokenResp, err := clientset.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, saName, treq, metav1.CreateOptions{})
return tokenResp.Status.Token, nil
```

---

## 🎯 **INFRASTRUCTURE STATUS**

### **Cluster** ✅ RUNNING
```
datastorage-e2e (Kind v1.34.0)
- control-plane node: Ready
- All system pods: Running
```

### **DataStorage Pods** ✅ RUNNING (2/2)
```
datastorage-585764d489-tch5v   2/2  Running
- oauth2-proxy container: Running (listening on 8080)
- datastorage container: Running (listening on 8081)
```

### **Supporting Services** ✅ RUNNING
```
postgresql-c4469d6cd-rkztd     1/1  Running
redis-fd7cd4847-qnmhp          1/1  Running
```

### **OAuth-Proxy** ✅ WORKING
```
2026/01/26 15:25:22 oauthproxy.go:210: mapping path "/" => upstream "http://localhost:8081/"
2026/01/26 15:25:22 oauthproxy.go:247: Cookie settings: name:_oauth_proxy
2026/01/26 15:25:22 http.go:64: HTTP: listening on 0.0.0.0:8080
2026/01/26 15:25:22 oauthproxy.go:231: compiled skip-auth-regex => "^/health$"
2026/01/26 15:25:22 oauthproxy.go:231: compiled skip-auth-regex => "^/ready$"
```

**Bypass auth working**: `/health` returns 200 from inside pod

---

## 📋 **FILES MODIFIED**

### **Test Infrastructure**
```
test/infrastructure/datastorage.go (3 fixes):
  - deployDataStorageClientClusterRole: yq filter for ClusterRole only
  - oauth2-proxy args: provider=openshift + bypass-auth-for flags
  - Health probes: port 8081

test/infrastructure/serviceaccount.go (1 fix):
  - GetServiceAccountToken: TokenRequest API instead of waiting for Secret

test/e2e/datastorage/datastorage_e2e_suite_test.go (1 fix):
  - Health check path: /health/ready → /health

test/e2e/datastorage/23_sar_access_control_test.go (1 fix):
  - Workflow types: WorkflowCreateRequest → RemediationWorkflow
```

### **Production Deployment**
```
deploy/data-storage/deployment.yaml (3 fixes):
  - oauth2-proxy args: add bypass-auth-for flags
  - oauth2-proxy args: add client-id, client-secret, cookie-secret
  - Health probes: uncommented and set to port 8081

deploy/data-storage/configmap.yaml (1 fix):
  - server.port: 8080 → 8081
```

---

## 🚀 **NEXT STEPS**

### **Immediate** (Session In Progress)
1. Run E2E tests with TokenRequest API fix
2. Validate all 6 SAR test scenarios pass:
   - Test 1: Authorized SA can write audit events (201 Created)
   - Test 2: Unauthorized SA gets 403 Forbidden
   - Test 3: Read-only SA (verb:"get") gets 403 Forbidden
   - Test 4: Workflow creation captures user attribution
   - Test 5: Unauthorized SA cannot create workflows (403)
   - Test 6: RBAC verification with `kubectl auth can-i`

### **Pending** (After Token Fix)
1. Triage other test failures (cert-manager timeout, workflow catalog, etc.)
2. Add 401 Unauthorized test scenarios (invalid/expired tokens)
3. Create HAPI E2E auth validation tests
4. Run Notification E2E tests (validates cross-namespace RBAC)

---

## 📊 **CONFIGURATION ALIGNMENT**

### **OAuth-Proxy Production vs E2E**

| Configuration | Production | E2E (Fixed) | Status |
|---------------|------------|-------------|--------|
| **Image** | `quay.io/openshift/oauth-proxy:latest` | `quay.io/jordigilh/ose-oauth-proxy:latest` | ✅ Aligned (multi-arch for dev) |
| **Provider** | `--provider=openshift` | `--provider=openshift` | ✅ Aligned |
| **SAR Verb** | `verb:"create"` | `verb:"create"` | ✅ Aligned |
| **Bypass Auth** | (not set) | `--bypass-auth-for=^/health$` | ✅ Added for both |
| **Cookie Secret** | `--cookie-secret-file` | `--cookie-secret` (inline) | ✅ Both present |
| **ServiceAccount** | `data-storage-sa` | `default` (E2E) | ✅ Appropriate |
| **Health Probes** | Port 8081 | Port 8081 | ✅ Aligned |

---

## 📚 **AUTHORITY DOCUMENTS**

- **DD-AUTH-011**: Granular RBAC & SAR Verb Mapping
- **DD-AUTH-012**: ose-oauth-proxy for SAR-Based REST API Authorization
- **DD-AUTH-013**: HTTP Status Codes for OAuth-Proxy
- **DD-AUTH-010**: E2E Real Authentication Mandate
- **DD-TEST-001**: E2E Port Allocation Strategy

---

**Document Version**: 1.0  
**Last Updated**: January 26, 2026  
**Status**: 🚧 IN PROGRESS  
**Next Action**: Run tests with TokenRequest API fix
