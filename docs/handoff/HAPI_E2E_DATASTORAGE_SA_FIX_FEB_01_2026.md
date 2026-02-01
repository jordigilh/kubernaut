# HAPI E2E DataStorage ServiceAccount Fix
## DataStorage Pod Timeout Resolved - February 1, 2026

---

## 🎯 Issue Summary

**Problem**: HAPI E2E tests fail in BeforeSuite with "DataStorage pod should become ready" timeout (2 minutes).

**Root Cause**: DataStorage deployment references ServiceAccount `data-storage-sa` that was never created.

**Impact**: HAPI E2E tests couldn't run (0/1 tests executed).

---

## 🔍 Root Cause Analysis

### Timeline

1. **Test Start**: HAPI E2E begins infrastructure setup
2. **ServiceAccount Missing**: `data-storage-sa` never created
3. **Pod Creation Rejected**: Kubernetes event: "serviceaccount 'data-storage-sa' not found"
4. **Deployment Stuck**: 0/1 pods ready
5. **Test Timeout**: BeforeSuite times out at 2 minutes waiting for pod

### Investigation Process

**Phase 1: Preserved Cluster Analysis**
```bash
kubectl get pods -n holmesgpt-api-e2e
# NAME                           READY   STATUS    RESTARTS   AGE
# datastorage-54878f546c-...     0/1     ...       0          4m44s
# holmesgpt-api-595984b4c4-...   1/1     Running   0          4m27s
# postgresql-c4469d6cd-...       1/1     Running   0          4m27s
# redis-fd7cd4847-...            1/1     Running   0          4m27s
```

**Phase 2: Deployment Analysis**
```bash
kubectl describe deployment datastorage -n holmesgpt-api-e2e
# Error: serviceaccount "data-storage-sa" not found
```

**Phase 3: Code Analysis**
- `holmesgpt_api.go:184`: Calls `deployDataStorageServiceInNamespaceWithNodePort()` directly
- `datastorage.go:999`: Function does NOT create ServiceAccount
- ServiceAccount creation requires separate call to `deployDataStorageServiceRBAC()`

---

## ✅ Solution

### Fix Applied

**File**: `test/infrastructure/holmesgpt_api.go`  
**Commit**: `cf391813b`  
**Pattern**: Same as Gateway/RO/Notification E2E (commit `81823ef8d`)

**Change**:
```go
// BEFORE:
go func() {
    defer GinkgoRecover()
    err := deployDataStorageServiceInNamespaceWithNodePort(ctx, namespace, kubeconfigPath, dataStorageImage, 30098, writer)
    deployResults <- deployResult{"DataStorage", err}
}()

// AFTER:
go func() {
    defer GinkgoRecover()
    
    // DD-AUTH-014: Create ServiceAccount BEFORE deployment (required for pod creation)
    // Fix for: "serviceaccount 'data-storage-sa' not found" error
    // Same fix as Gateway/RO/Notification E2E (commit 81823ef8d)
    _, _ = fmt.Fprintf(writer, "  🔐 Creating DataStorage ServiceAccount + RBAC...\n")
    if err := deployDataStorageServiceRBAC(ctx, namespace, kubeconfigPath, writer); err != nil {
        deployResults <- deployResult{"DataStorage", fmt.Errorf("failed to create ServiceAccount: %w", err)}
        return
    }
    
    err := deployDataStorageServiceInNamespaceWithNodePort(ctx, namespace, kubeconfigPath, dataStorageImage, 30098, writer)
    deployResults <- deployResult{"DataStorage", err}
}()
```

### What `deployDataStorageServiceRBAC()` Creates

1. **ServiceAccount**: `data-storage-sa`
   - Required by DataStorage deployment spec
   - Used for TokenReview and SubjectAccessReview API calls

2. **ClusterRole**: `data-storage-auth-middleware`
   - Permissions: `authentication.k8s.io/tokenreviews:create`
   - Permissions: `authorization.k8s.io/subjectaccessreviews:create`

3. **ClusterRoleBinding**: Binds SA to ClusterRole

---

## 📊 Validation Results

### Before Fix
```
Ran 0 of 1 Specs in 434.808 seconds
FAIL! -- A BeforeSuite node failed so all tests were skipped.

Error: DataStorage pod should become ready
Expected <bool>: false to be true
```

### After Fix
```
✅ Creating DataStorage ServiceAccount + RBAC...
serviceaccount/data-storage-sa serverside-applied
✅ DD-AUTH-014: Using in-cluster config with ServiceAccount, POD_NAMESPACE=holmesgpt-api-e2e
⏳ Waiting for DataStorage pod to be ready...
✅ DataStorage ready

Ran 1 of 1 Specs in 217.355 seconds
```

**Infrastructure Success**: DataStorage pod starts successfully!

---

## 🔴 Remaining Issue (Unrelated)

The test now progresses past infrastructure setup but fails with a **Python dependency error**:

```
ModuleNotFoundError: No module named 'fastapi'
```

**Root Cause**: Missing Python dependencies in test environment.

**Impact**: HAPI Python E2E tests don't execute (18 tests expected).

**Note**: This is a **test environment issue**, not an infrastructure/Kubernetes issue.

---

## 📚 Related Work

### Similar Fixes Applied
- **Commit `81823ef8d`**: Gateway, Notification, RO E2E (Jan 29, 2026)
- **Commit `71abaf835`**: AIAnalysis E2E via `DeployDataStorageTestServices()` (Jan 30, 2026)
- **Commit `cf391813b`**: HAPI E2E (Feb 1, 2026) ← THIS FIX

### Pattern
All E2E tests that deploy DataStorage with NodePort need to:
1. Call `deployDataStorageServiceRBAC()` FIRST
2. Then call `deployDataStorageServiceInNamespaceWithNodePort()`

### Why This Pattern Exists
- DD-AUTH-014 introduced middleware-based authentication
- DataStorage needs ServiceAccount for TokenReview/SAR API calls
- Deployment spec references `ServiceAccountName: "data-storage-sa"`
- Kubernetes rejects pod creation if ServiceAccount doesn't exist

---

## 🔧 Next Steps

1. **Fix Python Dependencies** (HAPI E2E)
   - Install `fastapi` module in test environment
   - Verify all Python dependencies from `holmesgpt-api/requirements.txt`

2. **Validate Full HAPI E2E Test Suite**
   - Expected: 18 Python E2E tests
   - Currently: 0 tests run (Python environment issue)

3. **Consider**: Add pre-flight check for Python dependencies in BeforeSuite

---

## 📖 References

### Business Requirements
- **BR-HAPI-197**: HolmesGPT-API E2E test requirements

### Technical Documentation
- **DD-AUTH-014**: Middleware-based authentication
- **DD-TEST-001**: E2E port allocation strategy
- **TD-E2E-001**: E2E test infrastructure phases

### Related Commits
```bash
cf391813b fix(test): Add missing DataStorage ServiceAccount creation for HAPI E2E
71abaf835 fix(test): Add missing ServiceAccount creation for AIAnalysis/HAPI E2E
81823ef8d fix: add missing ServiceAccount creation for DataStorage E2E deployments
```

---

## ✅ Success Metrics

- **Infrastructure Setup**: ✅ Complete (DataStorage pod starts)
- **ServiceAccount Creation**: ✅ Working
- **Pod Readiness**: ✅ Confirmed
- **Test Execution**: ❌ Blocked by Python dependency issue

**Infrastructure Fix**: **100% Complete**  
**Test Suite Fix**: Pending Python dependency resolution

---

**Generated**: February 1, 2026  
**Status**: Infrastructure fix complete, test environment fix pending  
**Confidence**: 100% (infrastructure), Unknown (test suite)
