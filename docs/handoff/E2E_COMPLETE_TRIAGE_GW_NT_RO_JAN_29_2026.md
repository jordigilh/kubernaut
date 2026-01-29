# E2E Complete Triage & Fix - Gateway, Notification, RO

**Date**: January 29, 2026  
**Status**: ✅ **INFRASTRUCTURE FIXED** | ⚠️ **GATEWAY AUDIT EMISSION ISSUE REMAINS**  
**Supervisor Triage**: Complete systematic root cause analysis

---

## 🎯 Summary

### ✅ **Infrastructure Issues: RESOLVED**
1. **ServiceAccount Creation**: DataStorage pod couldn't start (missing `data-storage-sa`)
2. **Client RBAC Missing**: Test clients getting 401 Unauthorized
3. **Test Client Authentication**: Tests using unauthenticated ogen clients

### ⚠️ **Remaining Issue: Gateway Not Emitting Audit Events**
Gateway service **isn't emitting audit events at all** (functional issue, not auth).

---

## 🔍 Systematic Triage Process

### Phase 1: Initial Test Execution

**Tests Run**:
1. Gateway E2E: 89/98 passed, 9 failed (audit-related)
2. Notification E2E: 0/30 ran (BeforeSuite failure)
3. Remediation Orchestrator E2E: 0/31 ran (BeforeSuite failure)

**Common Pattern**: All failures related to DataStorage infrastructure.

### Phase 2: Must-Gather Log Analysis

**Evidence Collected**:
- Gateway must-gather: `/tmp/gateway-e2e-logs-20260129-130424/`
- Notification must-gather: `/tmp/notification-e2e-logs-20260129-131549/`
- RO: No logs (cluster deleted before capture)

**Key Findings**:
```bash
# Gateway pods:
✅ gateway-XXXXX           Running
✅ postgresql-XXXXX        Running
✅ redis-XXXXX             Running
❌ NO datastorage pod found
```

### Phase 3: Live Cluster Investigation

**Commands Executed**:
```bash
$ kubectl get deployment datastorage -n kubernaut-system
NAME          READY   UP-TO-DATE   AVAILABLE   AGE
datastorage   0/1     0            0           47s  ❌

$ kubectl describe deployment datastorage
Conditions:
  ReplicaFailure   True    FailedCreate  ❌

$ kubectl describe replicaset datastorage-6999bb6568
Events:
  Warning  FailedCreate  "serviceaccount \"data-storage-sa\" not found"
```

### Phase 4: Code Analysis

**Found**:
- ServiceAccount referenced: `test/infrastructure/datastorage.go:1133`
- Creation function exists: `deployDataStorageServiceRBAC()`
- **Gap**: E2E setup never called RBAC creation function

---

## 🔧 Fixes Applied

### Fix 1: Add ServiceAccount Creation (3 files)

#### A. Gateway E2E (`test/infrastructure/gateway_e2e.go`)
```go
// Added before deployDataStorageServiceInNamespace():
if err := deployDataStorageClientClusterRole(ctx, kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy client ClusterRole: %w", err)
}
if err := deployDataStorageServiceRBAC(ctx, namespace, kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy service RBAC: %w", err)
}
```

#### B. Notification/General E2E (`test/infrastructure/datastorage.go`)
```go
// In DeployDataStorageTestServicesWithNodePort():
// Added ServiceAccount + client ClusterRole deployment
```

#### C. RO E2E (`test/infrastructure/remediationorchestrator_e2e_hybrid.go`)
```go
// Added in DataStorage deployment goroutine:
if clientRBACErr := deployDataStorageClientClusterRole(ctx, kubeconfigPath, writer); clientRBACErr != nil {
    return
}
if rbacErr := deployDataStorageServiceRBAC(ctx, namespace, kubeconfigPath, writer); rbacErr != nil {
    return
}
```

**Result**:
```
✅ ServiceAccount created:   data-storage-sa
✅ DataStorage deployment:   1/1 READY
✅ DataStorage pod:          Running
```

### Fix 2: Add Client RBAC for SAR Permissions

**Problem**: Test clients getting 401 Unauthorized from DataStorage middleware.

**Root Cause**: DataStorage middleware checks SAR:
```
Can <ServiceAccount> CREATE services/data-storage-service in kubernaut-system?
```

**Fix**: Deploy `client-rbac-v2.yaml` which creates:
- ClusterRole `data-storage-client` with CRUD permissions
- RoleBindings for all service ServiceAccounts

**Result**:
```
✅ ClusterRole 'data-storage-client' deployed
❌ Still getting 401 Unauthorized from tests
```

### Fix 3: Authenticate E2E Test Clients

**Problem**: E2E tests query DataStorage directly (to verify audit events) using plain ogen client:
```go
// BEFORE (no auth)
auditClient, _ = dsgen.NewClient(dataStorageURL)
```

**Root Cause**: Test code runs outside cluster, doesn't have ServiceAccount token.

**Fix**: Create E2E ServiceAccount and use authenticated client:
```go
// AFTER (with auth)
// 1. Create E2E ServiceAccount
err := infrastructure.CreateE2EServiceAccountWithDataStorageAccess(
    ctx, gatewayNamespace, kubeconfigPath, "gateway-e2e-audit-client", GinkgoWriter)

// 2. Get token
e2eToken, err := infrastructure.GetServiceAccountToken(
    ctx, gatewayNamespace, "gateway-e2e-audit-client", kubeconfigPath)

// 3. Create authenticated HTTP client
saTransport := testauth.NewServiceAccountTransport(e2eToken)
httpClient := &http.Client{
    Timeout:   20 * time.Second,
    Transport: saTransport,
}

// 4. Create ogen client with auth
auditClient, err = dsgen.NewClient(dataStorageURL, dsgen.WithClient(httpClient))
```

**Files Fixed**:
- `test/e2e/gateway/15_audit_trace_validation_test.go`
- `test/e2e/gateway/23_audit_emission_test.go`
- `test/e2e/gateway/24_audit_signal_data_test.go`

**Result**:
```
✅ NO MORE 401/403 errors!
✅ Auth fully working
⚠️  Still 9 failures (different reason)
```

---

## 📊 Test Results Evolution

### Run 1: Before Any Fixes
```
Status: INFRASTRUCTURE BLOCKED
- DataStorage pod: NOT CREATED (ServiceAccount missing)
- Error: "serviceaccount 'data-storage-sa' not found"
- Tests: 89/98 passed (audit tests failed - connection reset)
```

### Run 2: After ServiceAccount Fix
```
Status: AUTH BLOCKED
- DataStorage pod: ✅ RUNNING
- Error: "401 Unauthorized" (15+ occurrences)
- Tests: 89/98 passed (audit tests failed - auth error)
```

### Run 3: After Client RBAC Fix
```
Status: AUTH BLOCKED (TEST CLIENT)
- DataStorage pod: ✅ RUNNING
- ClusterRole: ✅ DEPLOYED
- Error: "401 Unauthorized" (test client not authenticated)
- Tests: 89/98 passed (audit tests failed - auth error)
```

### Run 4: After Test Client Auth Fix
```
Status: ✅ AUTH WORKING | ⚠️ GATEWAY NOT EMITTING AUDIT
- DataStorage pod: ✅ RUNNING
- ClusterRole: ✅ DEPLOYED
- Test client auth: ✅ WORKING
- Error: ZERO auth errors!
- Tests: 89/98 passed (audit tests failed - NO EVENTS EMITTED)
```

---

## ⚠️ Remaining Issue: Gateway Audit Emission

### Problem
Gateway service is **NOT emitting audit events** to DataStorage.

**Evidence**:
- ✅ Test client can query DataStorage successfully (no 401 errors)
- ✅ Test sends signal to Gateway
- ✅ Gateway processes signal (RemediationRequest CRD created)
- ❌ NO audit events appear in DataStorage (expected 2, got 0)

### Likely Causes

#### 1. Gateway Audit Client Not Initialized (Most Likely)
**Check**: Gateway startup logs
```bash
kubectl logs -l app=gateway -n kubernaut-system
# Look for: "Audit store initialized" or "Failed to create audit store"
```

#### 2. Gateway Audit Client Configuration Missing
**Check**: Gateway ConfigMap
```bash
kubectl get configmap gateway-config -n kubernaut-system -o yaml
# Verify: data_storage_url is set
```

#### 3. Gateway Audit Store Disabled/Failed
**Code Location**: `cmd/gateway/main.go:470-485`
```go
if cfg.Infrastructure.DataStorageURL != "" {
    dsClient, err := audit.NewOpenAPIClientAdapter(cfg.Infrastructure.DataStorageURL, 5*time.Second)
    if err != nil {
        logger.Error(err, "Failed to create Data Storage client")
        // Gateway continues WITHOUT audit capability!
    }
}
```

#### 4. Audit Events Not Being Sent
**Check**: Gateway code for audit emission calls
```bash
grep -r "auditStore.Write\|auditStore.StoreBatch" pkg/gateway/
```

---

## ✅ Commits Created

### Commit 1: ServiceAccount Creation
```
fix: add missing ServiceAccount creation for DataStorage E2E deployments

- Added deployDataStorageServiceRBAC() calls
- Fixes pod creation errors in Gateway, Notification, RO E2E
- Related: DD-AUTH-014
```

### Commit 2: Client RBAC + Test Authentication (Pending)
```
fix: add authenticated audit clients for Gateway E2E tests

- Deploy data-storage-client ClusterRole
- Create E2E ServiceAccounts with tokens
- Use authenticated ogen clients in tests
- Fixes 401 Unauthorized errors
- Related: DD-AUTH-014
```

---

## 🚀 Next Steps

### Immediate: Investigate Gateway Audit Emission (15-30 min)
1. Check Gateway startup logs
2. Verify Gateway ConfigMap has `data_storage_url`
3. Check if audit store initialized
4. Search Gateway code for audit emission calls

### Short-Term: Fix Gateway Audit Emission (30-60 min)
- Based on investigation findings
- Likely: Configuration or initialization issue

### Validation: Run All E2E Tests (60 min)
1. Gateway E2E: Should have 98/98 passing
2. Notification E2E: Should have 30/30 passing
3. RO E2E: Should have 31/31 passing

---

## 📚 Files Modified

| File | Change | Status |
|------|--------|--------|
| `test/infrastructure/gateway_e2e.go` | Add RBAC deployment | ✅ Committed |
| `test/infrastructure/datastorage.go` | Add RBAC deployment | ✅ Committed |
| `test/infrastructure/remediationorchestrator_e2e_hybrid.go` | Add RBAC deployment | ✅ Committed |
| `test/e2e/gateway/15_audit_trace_validation_test.go` | Add auth client | ⏳ Pending commit |
| `test/e2e/gateway/23_audit_emission_test.go` | Add auth client | ⏳ Pending commit |
| `test/e2e/gateway/24_audit_signal_data_test.go` | Add auth client | ⏳ Pending commit |

---

## 🎓 Lessons Learned

1. **DD-AUTH-014 Ripple Effects**: Authentication changes require updates across:
   - Service deployments (ServiceAccount references)
   - E2E infrastructure (RBAC creation)
   - Test code (authenticated clients)

2. **Layered Authentication**: Three layers must work:
   - Layer 1: Service pods (ServiceAccount for pod creation)
   - Layer 2: Service→Service calls (Bearer tokens + SAR)
   - Layer 3: Test→Service calls (E2E ServiceAccounts + tokens)

3. **Must-Gather Critical**: Live cluster investigation was essential for root cause.

4. **Progressive Debugging**: Each fix revealed the next layer:
   - Fix SA → Revealed RBAC issue
   - Fix RBAC → Revealed test client auth issue
   - Fix test auth → Revealed Gateway emission issue

---

**Status**: ✅ **AUTH INFRASTRUCTURE COMPLETE**  
**Next**: Investigate why Gateway isn't emitting audit events  
**Confidence**: 95% (Auth validated, functional issue remains)

---

**Triage Completed By**: AI Supervisor Agent  
**Duration**: ~3 hours (investigation + 3 iterative fixes)  
**Date**: January 29, 2026
