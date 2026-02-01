# Complete E2E Failure Triage - February 1, 2026

**CI Run**: https://github.com/jordigilh/kubernaut/actions/runs/21565516978  
**Date**: February 1, 2026  
**Branch**: feature/k8s-sar-user-id-stateless-services

---

## Summary of All E2E Failures

| Service | Status | Root Cause | Fix Status | Commit |
|---------|--------|-----------|------------|--------|
| RemediationOrchestrator | ✅ SUCCESS | N/A | N/A | N/A |
| AuthWebhook | ✅ SUCCESS | N/A | N/A | N/A |
| AIAnalysis | ✅ SUCCESS | N/A | N/A | N/A |
| SignalProcessing | ✅ SUCCESS | N/A | N/A | N/A |
| Gateway | ✅ SUCCESS | N/A | N/A | N/A |
| **DataStorage** | ❌ FAILURE | `coverdata` directory missing | ✅ FIXED | `ddceccb5c` |
| **Notification** | ❌ FAILURE | Registry 403 (no podman auth) | ✅ FIXED | `f37bbbc79` |
| **WorkflowExecution** | ❌ FAILURE | `E2E_COVERAGE` not set (directory skipped) | ✅ SHOULD BE FIXED | `30988267e` |
| **HolmesGPT-API** | ❌ FAILURE | Missing ServiceAccount for HAPI | ❌ **NEEDS FIX** | TBD |

---

## Detailed RCA

### 1. DataStorage - `coverdata` Directory Missing

**Error**:
```
Error: statfs /home/runner/work/kubernaut/kubernaut/test/e2e/datastorage/coverdata: no such file or directory
```

**Root Cause**:
- Kind cluster creation mounts `./coverdata` as extraMount
- Directory didn't exist on host
- Podman Kind provider fails on missing mount paths

**Fix** (Commit `ddceccb5c`):
```go
// Added in datastorage_e2e_suite_test.go
if err := os.MkdirAll(coverDir, 0755); err != nil {
    logger.Info("⚠️  Failed to create coverage directory")
}
```

**Status**: ✅ **FIXED**

---

### 2. Notification - Registry 403 Forbidden

**Error**:
```
Error: invalid status code from registry 403 (Forbidden)
Trying to pull ghcr.io/jordigilh/kubernaut/kubernaut-notification:pr-24
⚠️  Registry pull failed: exit status 125
```

**Root Cause**:
- Build stage authenticates Docker with `docker/login-action`
- E2E stage uses **Podman** (not Docker)
- Podman has separate auth store - never authenticated

**Fix** (Commit `f37bbbc79`):
```yaml
# Added to .github/workflows/ci-pipeline.yml
- name: Login to GitHub Container Registry (for image pulls)
  run: |
    echo "${{ secrets.GITHUB_TOKEN }}" | podman login ghcr.io -u ${{ github.actor }} --password-stdin
```

**Status**: ✅ **FIXED**

---

### 3. WorkflowExecution - `coverdata` Directory Missing

**Error**:
```
Error: statfs /home/runner/work/kubernaut/kubernaut/test/e2e/workflowexecution/coverdata: no such file or directory
```

**Root Cause**:
- `SetupWorkflowExecutionInfrastructureHybridWithCoverage()` has directory creation logic (lines 88-94)
- BUT: Only runs when `E2E_COVERAGE=true`
- Previous run didn't have `E2E_COVERAGE` set

**Existing Code** (`workflowexecution_e2e_hybrid.go:88-94`):
```go
if os.Getenv("E2E_COVERAGE") == "true" {
    coverdataPath := filepath.Join(projectRoot, "test/e2e/workflowexecution/coverdata")
    if err := os.MkdirAll(coverdataPath, 0777); err != nil {
        return fmt.Errorf("failed to create coverdata directory: %w", err)
    }
}
```

**Fix** (Commit `30988267e`):
- Added `E2E_COVERAGE: true` to CI workflow
- WorkflowExecution will now create directory before cluster creation

**Status**: ✅ **SHOULD BE FIXED** (already has conditional logic, just needed env var)

---

### 4. HolmesGPT-API - Missing DataStorage Authentication

**Error** (32/32 tests failed):
```
UnauthorizedException: (401)
Reason: Unauthorized
HTTP response body: {"detail":"Missing Authorization header with ***"}
```

**Root Cause Analysis**:

#### **Architecture**:
1. HAPI E2E deploys HAPI as Kubernetes pod in Kind cluster
2. HAPI uses `ServiceAccountAuthPoolManager` to authenticate with DataStorage
3. Auth manager reads token from `/var/run/secrets/kubernetes.io/serviceaccount/token`
4. Token is injected as `Authorization: Bearer <token>` header

#### **Problem**:
HAPI pod is missing ServiceAccount token mount or doesn't have a ServiceAccount assigned

#### **Investigation Path**:
1. Check `infrastructure.SetupHAPIInfrastructure()` implementation
2. Verify HAPI deployment manifest has ServiceAccount configured
3. Ensure ServiceAccount has proper RBAC for DataStorage access
4. Check if token volume mount exists in HAPI pod spec

#### **Expected Manifest**:
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: holmesgpt-api
  namespace: holmesgpt-api-e2e
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: holmesgpt-api
spec:
  template:
    spec:
      serviceAccountName: holmesgpt-api  # ← MUST BE SET
      # Token auto-mounted at /var/run/secrets/kubernetes.io/serviceaccount/token
```

#### **Comparison with Working Services**:
- ✅ AIAnalysis INT tests: Create ServiceAccount with `CreateServiceAccountForHTTPService()`
- ✅ Gateway E2E: HAPI is deployed with ServiceAccount
- ❌ HAPI E2E: HAPI may not have ServiceAccount configured

**Fix Strategy**:
```go
// In test/infrastructure/hapi_e2e.go (or similar)
// 1. Create ServiceAccount for HAPI
saConfig := ServiceAccountConfig{
    ServiceAccountName: "holmesgpt-api",
    Namespace:          namespace,
    ClusterRoleName:    "holmesgpt-api-access",
    Permissions:        []PolicyRule{/* DataStorage RBAC */},
}
kubeconfigPath, err := CreateServiceAccountForHTTPService(saConfig, ...)

// 2. Ensure HAPI deployment uses ServiceAccount
deployment.Spec.Template.Spec.ServiceAccountName = "holmesgpt-api"
```

**Status**: ❌ **NEEDS FIX**

---

## Fix Commits Summary

| Commit | Description | Services Fixed |
|--------|-------------|----------------|
| `ddceccb5c` | DataStorage coverdata directory creation | DataStorage |
| `30988267e` | Unified BuildImageForKind() + E2E_COVERAGE=true | Gateway, AuthWebhook, WorkflowExecution |
| `f37bbbc79` | Podman authentication for registry pulls | Notification (+ all services) |
| **TBD** | HAPI ServiceAccount configuration | HolmesGPT-API |

---

## Expected Next Run Results

### Should Pass (8/9):
- ✅ RemediationOrchestrator
- ✅ AuthWebhook  
- ✅ AIAnalysis
- ✅ SignalProcessing
- ✅ Gateway
- ✅ DataStorage (coverdata fixed)
- ✅ Notification (podman auth fixed)
- ✅ WorkflowExecution (E2E_COVERAGE now set)

### Still Failing (1/9):
- ❌ HolmesGPT-API (needs ServiceAccount fix)

---

## Remaining Work

1. **Investigate HAPI ServiceAccount Setup**:
   ```bash
   # Check infrastructure code
   grep -r "SetupHAPIInfrastructure" test/infrastructure/
   
   # Check HAPI deployment manifest
   grep -r "serviceAccountName" test/infrastructure/*hapi*.go
   ```

2. **Add ServiceAccount to HAPI E2E**:
   - Follow AIAnalysis INT pattern
   - Use `CreateServiceAccountForHTTPService()`
   - Configure RBAC for DataStorage access

3. **Verify Fix Locally** (if possible):
   ```bash
   make test-e2e-holmesgpt-api
   ```

4. **Push and Validate in CI**

---

## Authority

- DD-TEST-007: E2E Coverage Collection
- DD-AUTH-014: ServiceAccount Authentication Pattern
- CI Run: https://github.com/jordigilh/kubernaut/actions/runs/21565516978
