# DataStorage E2E SAR Testing - Complete Session Summary

**Date**: January 26, 2026  
**Session Duration**: ~2.5 hours (documentation + E2E testing)  
**Status**: 🚧 **SIGNIFICANT PROGRESS** - All infrastructure issues identified and fixed  
**Remaining**: Service name alignment + final test validation

---

## 📊 **SESSION OVERVIEW**

### **Phase 1: Documentation Organization** ✅ COMPLETE
- Created authoritative documentation structure guide
- Organized DD-AUTH-011, DD-AUTH-012, DD-AUTH-013 into multi-file directories
- Updated README.md and Cursor rules
- **Duration**: ~1 hour
- **Result**: 27 files organized, 3 README indexes created

### **Phase 2: E2E Test Fixes** 🚧 IN PROGRESS
- Fixed 10+ infrastructure and configuration issues
- Identified root cause of SAR test failures
- **Duration**: ~1.5 hours
- **Result**: Tests run but SAR validation needs service name alignment

---

## ✅ **10 CRITICAL FIXES APPLIED**

### **1. Test Compilation Errors** ✅
**File**: `test/e2e/datastorage/23_sar_access_control_test.go`

**Problems**:
- `WorkflowCreateRequest` type doesn't exist (should be `RemediationWorkflow`)
- `ListAuditEvents` method doesn't exist (should be `QueryAuditEvents`)
- `MandatoryLabels` field types incorrect

**Fixes**:
```go
// BEFORE:
workflowReq := dsgen.WorkflowCreateRequest{...}
auditResp, err := client.ListAuditEvents(...)

// AFTER:
workflowReq := dsgen.RemediationWorkflow{
    WorkflowID: dsgen.NewOptUUID(workflowID),
    Labels: dsgen.MandatoryLabels{
        Severity: dsgen.MandatoryLabelsSeverityHigh,
        SignalType: "prometheus-alert",
        Priority: dsgen.MandatoryLabelsPriority_P2,
        // ...
    },
}
auditResp, err := client.QueryAuditEvents(...)
```

**Result**: Test compiles successfully ✅

---

### **2. ClusterRole Deployment Error** ✅
**File**: `test/infrastructure/datastorage.go` (`deployDataStorageClientClusterRole`)

**Problem**: `deploy/data-storage/client-rbac-v2.yaml` contains RoleBindings for `kubernaut-system` namespace which doesn't exist in E2E tests

**Fix**: Use `yq` to extract only ClusterRole:
```go
// BEFORE:
kubectl apply -f client-rbac-v2.yaml  // Fails on RoleBinding namespace

// AFTER:
yq eval 'select(.kind == "ClusterRole")' client-rbac-v2.yaml | kubectl apply -f -
```

**Result**: ClusterRole deploys successfully ✅

---

### **3. Health Check Port Mismatch** ✅
**Files**: 
- `deploy/data-storage/deployment.yaml`
- `test/infrastructure/datastorage.go`

**Problem**: Liveness/readiness probes check port 8080, but DataStorage listens on port 8081

**Fix**:
```yaml
# Probes now check DataStorage directly (bypass oauth-proxy)
livenessProbe:
  httpGet:
    port: 8081  # DataStorage internal port
readinessProbe:
  httpGet:
    port: 8081  # DataStorage internal port
```

**Rationale**: Kubernetes probes are trusted (internal), should check container directly, not through auth proxy

**Result**: Probes pass, pods reach Running status ✅

---

### **4. OAuth-Proxy Provider Configuration** ✅
**File**: `test/infrastructure/datastorage.go`

**Problem**: Used `--provider=oidc` but ose-oauth-proxy requires `--provider=openshift`

**Fix**:
```go
// BEFORE:
"--provider=oidc",
"--oidc-issuer-url=...",
"--skip-oidc-discovery=true",
// ... OIDC-specific flags

// AFTER:
"--provider=openshift",
// Note: autodiscovers ServiceAccount from pod mount
```

**Result**: OAuth-proxy starts successfully ✅

---

### **5. OAuth-Proxy Bypass Flag** ✅
**Files**:
- `deploy/data-storage/deployment.yaml`
- `test/infrastructure/datastorage.go`

**Problem**: Used `--skip-auth-route` (doesn't exist in ose-oauth-proxy)

**Error**: `flag provided but not defined: -skip-auth-route`

**Fix**:
```yaml
# BEFORE:
- --skip-auth-route=^/health$

# AFTER (ose-oauth-proxy flag):
- --bypass-auth-for=^/health$
- --bypass-auth-for=^/ready$
```

**Result**: Health endpoints bypass authentication ✅

---

### **6. OAuth-Proxy Required Flags** ✅
**Files**:
- `deploy/data-storage/deployment.yaml`  
- `test/infrastructure/datastorage.go`

**Problem**: Missing `--client-id`, `--client-secret`, `--cookie-secret` (required for config validation)

**Error**: `Invalid configuration: missing setting: cookie-secret`

**Fix**:
```yaml
# SME Confirmation: Not functionally required for ServiceAccount token auth,
# but required for binary config validation
- --client-id=kubernetes
- --client-secret=unused
- --cookie-secret-file=/etc/oauth-proxy/cookie-secret  # Production
- --cookie-secret=0123456789ABCDEF0123456789ABCDEF    # E2E (inline)
```

**Result**: OAuth-proxy starts and listens on 8080 ✅

---

### **7. Health Check Path** ✅
**File**: `test/e2e/datastorage/datastorage_e2e_suite_test.go`

**Problem**: Checked `/health/ready` but DataStorage only has `/health` endpoint

**Fix**:
```go
// BEFORE:
httpClient.Get("http://localhost:28090/health/ready")

// AFTER:
httpClient.Get("http://localhost:28090/health")
```

**Result**: Health check responds 200 OK ✅

---

### **8. ConfigMap Port Configuration** ✅
**File**: `deploy/data-storage/configmap.yaml`

**Problem**: `server.port: 8080` but DataStorage should listen on 8081 (behind oauth-proxy)

**Fix**:
```yaml
# BEFORE:
server:
  port: 8080  # Conflicts with oauth-proxy

# AFTER:
server:
  port: 8081  # DD-AUTH-004: Internal port (oauth-proxy listens on 8080, proxies to 8081)
```

**Result**: DataStorage listens on correct port ✅

---

### **9. ServiceAccount Token Retrieval** ✅
**File**: `test/infrastructure/serviceaccount.go`

**Problem**: Kubernetes 1.24+ does NOT auto-create token Secrets for ServiceAccounts

**Old Code** (Broken in K8s 1.24+):
```go
// Wait for auto-created token Secret (never happens in K8s 1.24+)
secrets, err := clientset.CoreV1().Secrets(namespace).List(...)
// Timeout after 30s
```

**New Code** (K8s 1.24+ Compliant):
```go
// Use TokenRequest API for ephemeral tokens
treq := &authenticationv1.TokenRequest{
    Spec: authenticationv1.TokenRequestSpec{
        ExpirationSeconds: func() *int64 { exp := int64(3600); return &exp }(),
    },
}
tokenResp, err := clientset.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, saName, treq, metav1.CreateOptions{})
return tokenResp.Status.Token, nil
```

**Removed 3 calls** to `waitForServiceAccountToken` (no longer needed)

**Result**: Tokens retrieved instantly via TokenRequest API ✅

---

### **10. E2E Service Name Alignment** ✅
**File**: `test/infrastructure/datastorage.go`

**Problem**: E2E service named `datastorage`, but SAR checks for `data-storage-service`

**RBAC Check**:
```bash
$ kubectl auth can-i create services/data-storage-service --as=system:serviceaccount:datastorage-e2e:datastorage-e2e-authorized-sa
yes  ✅ (RBAC configured correctly)

$ curl -H "Authorization: Bearer $TOKEN" http://localhost:28090/api/v1/audit/events
403  ❌ (SAR fails because service name doesn't match)
```

**Root Cause**: OAuth-proxy SAR checks for `data-storage-service` resource, but E2E creates service named `datastorage`

**Fix**:
```go
// BEFORE:
service := &corev1.Service{
    ObjectMeta: metav1.ObjectMeta{
        Name: "datastorage",  // Mismatch!
        
// AFTER:
service := &corev1.Service{
    ObjectMeta: metav1.ObjectMeta{
        Name: "data-storage-service",  // DD-AUTH-011: Match production
```

**Result**: Service name matches SAR resourceName ✅

---

## 🎯 **INFRASTRUCTURE VALIDATION**

### **Cluster Status** ✅
```
datastorage-e2e (Kind v1.34.0)
- Control-plane node: Ready
- System pods: All Running
```

### **DataStorage Pods** ✅ (Last Run)
```
datastorage-585764d489-tch5v   2/2  Running
- oauth2-proxy: Running, listening on 8080
- datastorage: Running, listening on 8081
```

### **OAuth-Proxy Logs** ✅
```
2026/01/26 15:25:22 oauthproxy.go:210: mapping path "/" => upstream "http://localhost:8081/"
2026/01/26 15:25:22 http.go:64: HTTP: listening on 0.0.0.0:8080
2026/01/26 15:25:22 oauthproxy.go:231: compiled skip-auth-regex => "^/health$"
2026/01/26 15:25:22 oauthproxy.go:231: compiled skip-auth-regex => "^/ready$"
```

### **Health Checks** ✅
```bash
# From inside pod (directly to DataStorage):
$ curl http://localhost:8081/health
{"status":"healthy","database":"connected"}  ✅

# Through oauth-proxy (bypassed):
$ curl http://localhost:8080/health
{"status":"healthy","database":"connected"}  ✅
```

### **Token Retrieval** ✅
```bash
$ kubectl create token datastorage-e2e-authorized-sa --duration=1h
eyJhbGciOiJSUzI1NiIsImtpZCI6InkzZ2Nna0hYcTRCY081UnQ1d01v...  ✅
(Token retrieved instantly via TokenRequest API)
```

### **RBAC Validation** ✅
```bash
$ kubectl auth can-i create services/data-storage-service \
    --as=system:serviceaccount:datastorage-e2e:datastorage-e2e-authorized-sa \
    -n datastorage-e2e
yes  ✅ (ClusterRole + RoleBinding configured correctly)
```

---

## 📋 **TEST EXECUTION SUMMARY**

### **Last Test Run** (Before Service Name Fix)
- **Specs Executed**: 56 of 190
- **Results**: 6 Passed | 50 Failed | 1 Pending | 133 Skipped
- **Duration**: ~4 minutes
- **Infrastructure**: ✅ All pods Running
- **SAR Tests**: ❌ All failed with 403 Forbidden
- **Root Cause**: Service name mismatch (`datastorage` vs `data-storage-service`)

### **Expected After Service Name Fix**
- SAR tests should pass (RBAC already validates correctly)
- 403 errors should become 201 Created for authorized ServiceAccount
- 403 errors should remain for unauthorized ServiceAccount (correct behavior)

---

## 🚀 **NEXT STEPS**

### **Immediate** (Next Session)
1. ✅ **Service name fix applied** - ready to test
2. Run E2E tests to validate SAR enforcement:
   ```bash
   make test-e2e-datastorage
   ```
3. Expected SAR test results:
   - Test 1: Authorized SA writes audit event → 201 Created ✅
   - Test 2: Unauthorized SA → 403 Forbidden ✅
   - Test 3: Read-only SA (verb:"get") → 403 Forbidden ✅
   - Test 4: Workflow creation with user attribution → 201 + audit log ✅
   - Test 5: Unauthorized workflow creation → 403 Forbidden ✅
   - Test 6: RBAC verification with `kubectl auth can-i` → All pass ✅

### **Follow-Up**
1. Add 401 Unauthorized test scenarios (invalid/expired tokens)
2. Create HAPI E2E auth validation tests (`test/e2e/holmesgpt-api/auth_validation_test.go`)
3. Run Notification E2E tests (validates cross-namespace RBAC)
4. Triage other test failures (cert-manager timeout, workflow catalog, etc.)

---

## 📚 **FILES MODIFIED (10 files)**

### **Test Infrastructure** (3 files)
```
test/infrastructure/datastorage.go:
  Line 587: ClusterRole deployment (yq filter)
  Line 1029: Service name alignment (data-storage-service)
  Line 1140: OAuth-proxy provider (openshift)
  Line 1147: OAuth-proxy bypass flags
  Line 1153: OAuth-proxy required flags
  Line 1250: Health probe port (8081)

test/infrastructure/serviceaccount.go:
  Line 28: Add authenticationv1 import
  Line 166: GetServiceAccountToken (TokenRequest API)
  Line 136: Remove waitForServiceAccountToken call (x3)

test/e2e/datastorage/datastorage_e2e_suite_test.go:
  Line 168: Health check path (/health)

test/e2e/datastorage/23_sar_access_control_test.go:
  Line 297: WorkflowCreateRequest → RemediationWorkflow
  Line 326: ListAuditEvents → QueryAuditEvents
```

### **Production Deployment** (2 files)
```
deploy/data-storage/deployment.yaml:
  Line 55: OAuth-proxy bypass flags
  Line 68: OAuth-proxy required flags
  Line 117: Health probes uncommented (port 8081)

deploy/data-storage/configmap.yaml:
  Line 9: server.port (8080 → 8081)
```

### **Documentation** (5 files)
```
docs/architecture/decisions/DD-AUTH-011/README.md (380 lines)
docs/architecture/decisions/DD-AUTH-012/README.md (320 lines)
docs/architecture/decisions/DD-AUTH-013/README.md (320 lines)
docs/handoff/DS_E2E_SAR_TRIAGE_JAN_26_2026.md
docs/handoff/DS_E2E_SAR_SESSION_COMPLETE_JAN_26_2026.md (this file)
```

---

## 🔧 **OAUTH-PROXY CONFIGURATION SUMMARY**

### **Production vs E2E Alignment**

| Configuration | Production | E2E | Status |
|---------------|------------|-----|--------|
| **Image** | `quay.io/openshift/oauth-proxy:latest` | `quay.io/jordigilh/ose-oauth-proxy:latest` | ✅ Aligned (multi-arch) |
| **Provider** | `--provider=openshift` | `--provider=openshift` | ✅ Aligned |
| **SAR Namespace** | `kubernaut-system` | `datastorage-e2e` | ✅ Appropriate |
| **SAR Resource** | `services/data-storage-service` | `services/data-storage-service` | ✅ Aligned |
| **SAR Verb** | `verb:"create"` | `verb:"create"` | ✅ Aligned |
| **Bypass Auth** | `--bypass-auth-for=^/health$` | `--bypass-auth-for=^/health$` | ✅ Aligned |
| **Cookie Secret** | `--cookie-secret-file` (volume) | `--cookie-secret` (inline) | ✅ Both present |
| **Service Account** | `data-storage-sa` | `default` | ✅ Appropriate |
| **Health Probes** | Port 8081 (direct) | Port 8081 (direct) | ✅ Aligned |

---

## 📊 **KEY LEARNINGS**

### **1. Kubernetes 1.24+ Token Changes**
**Breaking Change**: ServiceAccount token Secrets are no longer auto-created

**Migration**:
- **Old**: Wait for token Secret to be created automatically
- **New**: Use TokenRequest API (`clientset.CoreV1().ServiceAccounts().CreateToken()`)
- **Impact**: All E2E tests using ServiceAccount tokens must use TokenRequest API

**Authority**: https://kubernetes.io/docs/reference/kubernetes-api/authentication-resources/token-request-v1/

---

### **2. ose-oauth-proxy vs oauth2-proxy Flags**
**Key Differences**:

| Flag | oauth2-proxy (CNCF) | ose-oauth-proxy (OpenShift) |
|------|---------------------|----------------------------|
| **Provider** | `--provider=oidc` | `--provider=openshift` |
| **Skip Auth** | `--skip-auth-route` | `--bypass-auth-for` |
| **SAR Support** | ❌ No `--openshift-sar` | ✅ `--openshift-sar` supported |

**Authority**: DD-AUTH-012 (ose-oauth-proxy for SAR-Based REST API Authorization)

---

### **3. Health Probe Best Practice**
**Pattern**: Kubernetes probes should check containers directly, not through auth proxies

**Rationale**:
- Probes are from kubelet (trusted, cluster-internal)
- Faster response (no auth overhead)
- More reliable (no auth failures blocking health status)
- Standard Kubernetes pattern

**Implementation**:
- **External clients** → Service:8080 → oauth-proxy:8080 → DataStorage:8081 (auth enforced)
- **Kubernetes probes** → DataStorage:8081 directly (no auth)

---

### **4. SAR Resource Name Matching**
**Critical**: SAR `resourceName` must match actual K8s Service resource name

**OAuth-proxy SAR Check**:
```json
{"namespace":"datastorage-e2e","resource":"services","resourceName":"data-storage-service","verb":"create"}
```

**ClusterRole Grant**:
```yaml
rules:
- resources: ["services"]
  resourceNames: ["data-storage-service"]  # Must match SAR resourceName
  verbs: ["create"]
```

**E2E Service**:
```go
service := &corev1.Service{
    ObjectMeta: metav1.ObjectMeta{
        Name: "data-storage-service",  // Must match resourceName
```

**RBAC Logic**: Kubernetes checks if ServiceAccount can perform verb on that specific resource name in that namespace

---

## 🚧 **KNOWN REMAINING ISSUES**

### **1. SAR Tests Still Failing** (Service Name Fix Applied, Pending Validation)
**Status**: Fix applied, needs test run to confirm

**Expected**: All 6 SAR tests should pass after service name alignment

---

### **2. Cert-Manager Certificate Timeout**
**Test**: SOC2 Compliance Features (cert-manager)

**Error**: `Timed out after 30.001s. Certificate generation should complete within 30s`

**Root Cause**: cert-manager not installed or not ready in Kind cluster

**Impact**: 7-8 SOC2 compliance tests fail

**Priority**: Low (SOC2 tests are separate from SAR validation)

---

### **3. Workflow Catalog Tests**
**Status**: Multiple workflow-related tests failing

**Likely Cause**: Schema changes or test data issues (not related to auth)

**Priority**: Medium (validate after SAR tests pass)

---

## 📖 **AUTHORITY DOCUMENTS**

- **[DD-AUTH-011](docs/architecture/decisions/DD-AUTH-011/DD-AUTH-011-granular-rbac-sar-verb-mapping.md)**: Granular RBAC & SAR Verb Mapping
- **[DD-AUTH-012](docs/architecture/decisions/DD-AUTH-012/DD-AUTH-012-ose-oauth-proxy-sar-rest-api-endpoints.md)**: ose-oauth-proxy for SAR-Based REST API Authorization
- **[DD-AUTH-013](docs/architecture/decisions/DD-AUTH-013/DD-AUTH-013-http-status-codes-oauth-proxy.md)**: HTTP Status Codes for OAuth-Proxy
- **[DD-AUTH-010](docs/architecture/decisions/DD-AUTH-010-e2e-real-authentication-mandate.md)**: E2E Real Authentication Mandate
- **[DD-TEST-001](docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md)**: E2E Port Allocation Strategy

---

## 🎯 **SUCCESS CRITERIA**

### **Completed** ✅
- [x] Test code compiles without errors
- [x] Kind cluster creates successfully
- [x] All infrastructure pods reach Running status (2/2 DataStorage, 1/1 PostgreSQL, 1/1 Redis)
- [x] OAuth-proxy starts and listens on 8080
- [x] DataStorage starts and listens on 8081
- [x] Health checks pass (probes on port 8081)
- [x] ServiceAccount tokens retrieved via TokenRequest API
- [x] RBAC permissions validate correctly (`kubectl auth can-i` returns "yes")
- [x] Service name aligned with SAR resourceName

### **Pending** 🚧
- [ ] SAR tests pass (all 6 scenarios)
- [ ] 403 Forbidden for unauthorized ServiceAccounts
- [ ] 201 Created for authorized ServiceAccounts
- [ ] User attribution captured in audit logs

---

## 💡 **RECOMMENDATIONS**

### **For Production Deployment**
1. ✅ **Use health check bypass**: Add `--bypass-auth-for=^/health$` to avoid probe failures
2. ✅ **Use port 8081 for probes**: Check DataStorage directly, not through oauth-proxy
3. ✅ **Cookie-secret is required**: Even for ServiceAccount auth (config validation)

### **For E2E Testing**
1. ✅ **Use TokenRequest API**: No more waiting for token Secrets (K8s 1.24+)
2. ✅ **Match production service names**: Ensures SAR validation works correctly
3. ✅ **Direct health probes**: Port 8081, not 8080

### **For Other Services**
1. Apply same oauth-proxy configuration pattern to HolmesGPT API
2. Use TokenRequest API for all E2E tests creating ServiceAccounts
3. Ensure service names match SAR resourceName configurations

---

## 📊 **CONFIDENCE ASSESSMENT**

**Current Status**: 90% confidence SAR tests will pass after service name fix

**Rationale**:
- All infrastructure issues resolved
- RBAC validates correctly (`kubectl auth can-i` returns "yes")
- OAuth-proxy configuration aligned with production
- TokenRequest API working (ServiceAccounts created successfully)
- Service name is the last remaining mismatch

**Risk**: 10% - Potential edge cases in oauth-proxy SAR evaluation or RBAC propagation delay

---

## 🎉 **SESSION ACCOMPLISHMENTS**

### **Documentation** ✅
- Created `docs/DOCS_DIRECTORY_STRUCTURE_GUIDE.md` (389 lines)
- Organized 27 DD-AUTH files into 3 directories with README indexes (1,020 lines)
- Updated README.md and Cursor rules for future reference

### **Infrastructure Fixes** ✅
- Fixed 10 critical issues blocking E2E test execution
- Aligned E2E configuration with production deployment
- Modernized token retrieval for Kubernetes 1.24+

### **Code Quality** ✅
- Removed deprecated wait-for-Secret pattern
- Added proper comments explaining design decisions
- Aligned all configurations with authoritative documents (DD-AUTH-011/012/013)

---

**Document Version**: 1.0  
**Last Updated**: January 26, 2026 10:58 AM  
**Status**: 🚧 IN PROGRESS  
**Next Action**: Run E2E tests to validate service name fix
**Expected Result**: All 6 SAR tests pass ✅
