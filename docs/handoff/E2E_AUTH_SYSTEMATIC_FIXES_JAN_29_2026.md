# E2E Auth/Authz Systematic Fixes - Progress Report
**Date**: January 29, 2026  
**Authority**: DD-AUTH-014 (Middleware-Based SAR Authentication)

---

## Executive Summary

Successfully identified and systematically fixed auth/authz issues across E2E test suites.

**Root Cause**: Services creating DataStorage API clients without ServiceAccount Bearer tokens.  
**Impact**: Tests failing with `401 Unauthorized` or `403 Forbidden`.  
**Solution**: Systematic application of authenticated OpenAPI client pattern from Gateway E2E.

---

## Status Overview

| Service | Infrastructure | Suite Setup | Test Files | Compiled | Status |
|---------|---------------|-------------|------------|----------|--------|
| **Gateway** | ✅ Already working | ✅ Already working | ✅ Already working | ✅ Yes | ✅ **COMPLETE** |
| **SignalProcessing** | ✅ Fixed (commit 452a9a6a1) | ✅ Fixed (commit 98aa375db) | ✅ Fixed | ✅ Yes | ✅ **COMPLETE** |
| **RemediationOrchestrator** | ✅ Fixed (commit a03f5cd03) | ✅ Fixed | ✅ Fixed | ✅ Yes | ✅ **COMPLETE** |
| **WorkflowExecution** | ✅ Fixed (uncommitted) | ⏳ Partial | ⏳ Pending | ❓ Not tested | ⏳ **IN PROGRESS** |
| **Notification** | ⏳ Pending | ⏳ Pending | ⏳ Pending | ❓ Not tested | ⏳ **PENDING** |
| **AIAnalysis** | ⏳ Pending | ⏳ Pending | ⏳ Pending | ❓ Not tested | ⏳ **PENDING** |

---

## Completed Work

### 1. **SignalProcessing E2E** ✅

**Files Modified**:
- `test/infrastructure/signalprocessing_e2e_hybrid.go`
  - Added `deployDataStorageClientClusterRole()` in Phase 3.5
- `test/e2e/signalprocessing/suite_test.go`
  - Added `e2eAuthToken` global variable
  - Added E2E ServiceAccount creation + token retrieval
  - Updated data passing: `kubeconfig|coverage|token`
- `test/e2e/signalprocessing/business_requirements_test.go`
  - Updated `queryAuditEvents()` to use `ServiceAccountTransport`
  - Added `testauth` import

**Validation**:
- ✅ Compiles successfully
- ✅ Authentication working (no more 401/403 errors)
- ⚠️  BR-SP-090 still fails (audit emission issue, not auth)

**Commits**:
- `98aa375db` - Add ServiceAccount authentication
- `452a9a6a1` - Deploy data-storage-client ClusterRole

---

### 2. **RemediationOrchestrator E2E** ✅

**Files Modified**:
- `test/infrastructure/remediationorchestrator_e2e_hybrid.go`
  - Added `deployDataStorageClientClusterRole()` in Phase 3.5
- `test/e2e/remediationorchestrator/suite_test.go`
  - Added `e2eAuthToken` global variable
  - Added E2E ServiceAccount creation + token retrieval
  - Updated data passing: `kubeconfig|token`
- `test/e2e/remediationorchestrator/audit_wiring_e2e_test.go`
  - Updated BeforeEach to use `ServiceAccountTransport`
  - Added `testauth` import

**Validation**:
- ✅ Compiles successfully
- ⏳ E2E tests not run yet (will validate after all fixes applied)

**Commit**:
- `a03f5cd03` - Systematic auth/authz for SP and RO

---

## Remaining Work

### 3. **WorkflowExecution E2E** ⏳

**Status**: Infrastructure fixed, suite partially updated

**Files to Modify**:
- ✅ `test/infrastructure/workflowexecution_e2e_hybrid.go` - ClusterRole added
- ⏳ `test/e2e/workflowexecution/workflowexecution_e2e_suite_test.go` - Need to add:
  - `e2eAuthToken` global variable
  - E2E ServiceAccount creation
  - Token retrieval and passing
- ⏳ `test/e2e/workflowexecution/02_observability_test.go` - Need to update:
  - 3x `ogenclient.NewClient()` calls to use authenticated transport

**Estimated Time**: 10-15 minutes

---

### 4. **Notification E2E** ⏳

**Status**: Not started

**Files to Modify**:
- ⏳ Infrastructure file (need to find which one)
- ⏳ Suite file
- ⏳ Test files:
  - `01_notification_lifecycle_audit_test.go`
  - `02_audit_correlation_test.go`
  - `04_failed_delivery_audit_test.go`

**Estimated Time**: 15-20 minutes

---

### 5. **AIAnalysis E2E** ⏳

**Status**: Not started

**Files to Modify**:
- ⏳ Infrastructure file
- ⏳ `test/e2e/aianalysis/suite_test.go`
- ⏳ Audit trail test files

**Estimated Time**: 10-15 minutes

---

## Standard Fix Pattern

All fixes follow this pattern (established from Gateway E2E):

### A. Infrastructure (`*_e2e_hybrid.go`)
```go
// Phase 3.5 - BEFORE DataStorage deployment
_, _ = fmt.Fprintf(writer, "\n🔐 Deploying data-storage-client ClusterRole (DD-AUTH-014)...\n")
if err := deployDataStorageClientClusterRole(ctx, kubeconfigPath, writer); err != nil {
    return fmt.Errorf("failed to deploy client ClusterRole: %w", err)
}
```

### B. Suite Setup (`*suite*test.go`)
```go
// 1. Add global variable
var (
    e2eAuthToken string // DD-AUTH-014
)

// 2. In SynchronizedBeforeSuite (process 1)
e2eSAName := "{service}-e2e-sa"
err = infrastructure.CreateE2EServiceAccountWithDataStorageAccess(ctx, namespace, kubeconfigPath, e2eSAName, GinkgoWriter)
token, err := infrastructure.GetServiceAccountToken(ctx, namespace, e2eSAName, kubeconfigPath)
return []byte(fmt.Sprintf("%s|%s", kubeconfigPath, token)) // Add token to return

// 3. In SynchronizedBeforeSuite (all processes)
parts := strings.Split(string(data), "|")
kubeconfigPath = parts[0]
if len(parts) > 1 {
    e2eAuthToken = parts[1]
}
```

### C. Test Files
```go
// 1. Add import
import testauth "github.com/jordigilh/kubernaut/test/shared/auth"

// 2. Replace unauthenticated client
// OLD:
dsClient, err := dsgen.NewClient(dataStorageURL)

// NEW:
saTransport := testauth.NewServiceAccountTransport(e2eAuthToken)
httpClient := &http.Client{Timeout: 20*time.Second, Transport: saTransport}
dsClient, err := dsgen.NewClient(dataStorageURL, dsgen.WithClient(httpClient))
```

---

## Next Steps

**Option A**: Complete remaining 3 services systematically (recommended)
- Finish WorkflowExecution (infrastructure done, suite needs completion)
- Apply pattern to Notification
- Apply pattern to AIAnalysis
- Validate all compile
- Commit as batch

**Option B**: Test each service individually
- Complete WE, test, commit
- Complete NT, test, commit  
- Complete AA, test, commit

**Recommendation**: Option A (batch) is faster and ensures consistency, but Option B provides incremental validation.

---

## Success Criteria

For each service:
- ✅ Code compiles without errors
- ✅ No `401 Unauthorized` errors
- ✅ No `403 Forbidden` errors
- ✅ DataStorage queries return `200 OK` (even if empty results)
- ⚠️  Tests may still fail for business logic reasons (separate from auth)

---

## Commits Made

1. `98aa375db` - fix(e2e): add ServiceAccount authentication for SignalProcessing DataStorage queries
2. `452a9a6a1` - fix(e2e): deploy data-storage-client ClusterRole for SignalProcessing E2E
3. `01383f112` - fix(e2e): add missing DataStorage ServiceAccount RBAC for SP and WE tests
4. `a03f5cd03` - fix(e2e): systematic auth/authz for SignalProcessing and RemediationOrchestrator

---

## Files Changed Summary

**Infrastructure** (4 files):
- ✅ `test/infrastructure/signalprocessing_e2e_hybrid.go`
- ✅ `test/infrastructure/remediationorchestrator_e2e_hybrid.go`
- ✅ `test/infrastructure/workflowexecution_e2e_hybrid.go`
- ⏳ Notification infrastructure (TBD)
- ⏳ AIAnalysis infrastructure (TBD)

**Suite Files** (5 files):
- ✅ `test/e2e/signalprocessing/suite_test.go`
- ✅ `test/e2e/remediationorchestrator/suite_test.go`
- ⏳ `test/e2e/workflowexecution/workflowexecution_e2e_suite_test.go`
- ⏳ Notification suite (TBD)
- ⏳ AIAnalysis suite (TBD)

**Test Files** (~8 files):
- ✅ `test/e2e/signalprocessing/business_requirements_test.go`
- ✅ `test/e2e/remediationorchestrator/audit_wiring_e2e_test.go`
- ⏳ WorkflowExecution observability tests (3x client creations)
- ⏳ Notification audit tests (3 files)
- ⏳ AIAnalysis audit tests (TBD)

---

## Authority & References

- **DD-AUTH-014**: Middleware-Based SAR Authentication
- **Pattern Source**: `test/e2e/gateway/` (working reference)
- **ClusterRole**: `deploy/data-storage/client-rbac-v2.yaml`
- **Helper**: `test/infrastructure/serviceaccount.go`
- **Transport**: `test/shared/auth/serviceaccount_transport.go`
