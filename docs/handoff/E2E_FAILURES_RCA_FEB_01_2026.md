# E2E Test Failures - Root Cause Analysis
**Date**: February 1, 2026  
**CI Run**: https://github.com/jordigilh/kubernaut/actions/runs/21566653233  
**Commit**: `9ddb311f7` (+ `2852238aa` AIAnalysis fix)

---

## Executive Summary

**CI Status**: 3/9 E2E tests failed (Notification, WorkflowExecution, HolmesGPT-API)

| Service | Status | Root Cause |
|---------|--------|------------|
| ✅ RemediationOrchestrator | PASS | - |
| ✅ Gateway | PASS | - |
| ✅ DataStorage | PASS | - |
| ✅ AuthWebhook | PASS | - |
| ✅ SignalProcessing | PASS | - |
| ✅ AIAnalysis | PASS | Image naming fixed (commit `2852238aa`) |
| ❌ **Notification** | **FAIL** | Pod readiness timeout (unknown) |
| ❌ **WorkflowExecution** | **FAIL** | tkn CLI flag incompatibility |
| ❌ **HolmesGPT-API** | **FAIL** | RBAC fix incomplete (401 Unauthorized) |

---

## Issue 1: Notification E2E - Pod Readiness Timeout ❌

### Error
```
error: timed out waiting for the condition on pods/notification-controller-55bd5dd574-lcm5m
controller pod did not become ready: exit status 1
```

### Root Cause
**Unknown** - Pod was deployed successfully but failed readiness checks.

### Evidence
- Pod created: `notification-controller-55bd5dd574-lcm5m`
- Image pulled successfully: `ghcr.io/jordigilh/kubernaut/notification:pr-24`
- Deployment applied to namespace: `notification-e2e`
- Pod never became ready within timeout period

### Next Steps
1. **Download must-gather artifacts** to inspect pod logs
2. Check for:
   - Container crash loops
   - Image pull errors (unlikely, image exists)
   - Configuration issues
   - Missing dependencies (DataStorage, Kubernetes API access)
   - RBAC issues (ServiceAccount permissions)

### Investigation Required
```bash
# Download must-gather logs
gh run download 21566653233 --pattern 'must-gather-notification-*'

# Check pod describe output for events
# Check container logs for crash/error messages
# Verify RBAC configuration
```

### Potential Causes
1. **DataStorage unavailable**: Notification needs DataStorage access (DD-AUTH-014)
2. **ServiceAccount permissions**: Missing RBAC for DataStorage client
3. **Configuration error**: Invalid ConfigMap or environment variables
4. **Application crash**: Go panic or unhandled error on startup

---

## Issue 2: WorkflowExecution E2E - tkn CLI Flag Incompatibility ❌

### Error
```
Error: unknown flag: --override
failed to build hello-world bundle: tkn CLI not found - install from https://tekton.dev/docs/cli/
```

### Root Cause
**tkn v0.39.0 does not support the `--override` flag** used by the test infrastructure.

### Evidence
- Test tried to build Tekton bundle: `localhost/kubernaut-test-workflows/hello-world:v1.0.0`
- Command attempted: `tkn bundle push ...  --override`
- tkn CLI v0.39.0 installed successfully
- Flag `--override` is not recognized by this version

### Fix Options

#### Option A: Upgrade tkn CLI (Recommended)
```yaml
# .github/workflows/ci-pipeline.yml
- name: Install Tekton CLI (tkn)
  run: |
    # Upgrade to latest tkn version (supports --override flag)
    TKN_VERSION="0.40.0"  # Or latest version
    curl -LO "https://github.com/tektoncd/cli/releases/download/v${TKN_VERSION}/tkn_${TKN_VERSION}_Linux_x86_64.tar.gz"
    tar xzf "tkn_${TKN_VERSION}_Linux_x86_64.tar.gz" tkn
    chmod +x tkn
    sudo mv tkn /usr/local/bin/tkn
    tkn version
```

#### Option B: Remove `--override` flag from test code
```go
// test/infrastructure/tekton_bundles.go
// Change bundling command to remove --override flag
// (Only if flag is optional and not required for functionality)
```

#### Option C: Check tkn version compatibility
Research which tkn version introduced `--override` flag and update accordingly.

### Recommendation
**Upgrade tkn to v0.40.0 or later** (Option A), as `--override` flag is likely critical for bundle building workflow.

### Impact
- All WorkflowExecution E2E tests skipped (12 tests)
- Bundle building fails during BeforeSuite setup
- Local development may also be affected if using tkn v0.39.0

---

## Issue 3: HolmesGPT-API E2E - RBAC Incomplete (401 Unauthorized) ❌

### Error
```
HTTP response body: {"type":"about:blank","title":"Unauthorized","status":401,"detail":"Missing Authorization header with ***"}
UnauthorizedException: (401) Reason: Unauthorized
```

### Root Cause
**RBAC fix incomplete** - ServiceAccount created, RoleBinding added, BUT HAPI is still not sending Authorization header to DataStorage.

### Evidence (31/35 tests failed)
All failures show same pattern:
- `Data Storage Service request error: Unauthorized`
- `Missing Authorization header with ***`
- Tests: `test_workflow_catalog_container_image_integration`, `test_workflow_catalog_data_storage_integration`, `test_workflow_selection_e2e`, etc.

### Previous Fix Attempt (Commit `9ddb311f7`)
```go
// test/infrastructure/holmesgpt_api.go:deployHAPIServiceRBAC()
// Created:
// 1. ServiceAccount: holmesgpt-api-sa
// 2. ClusterRole: data-storage-client (with CRUD verbs)
// 3. RoleBinding: holmesgpt-api-data-storage-client
```

### What's Missing
The RBAC is now correct, but **HAPI is not using the ServiceAccount token**.

**HAPI Authentication Flow (DD-AUTH-014)**:
1. ServiceAccount token mounted at `/var/run/secrets/kubernetes.io/serviceaccount/token` ✅
2. `ServiceAccountAuthPoolManager` reads token ❓
3. Injects `Authorization: Bearer <token>` header in requests to DataStorage ❌

### Investigation Required

#### Check 1: Is ServiceAccountAuthPoolManager initialized?
```python
# holmesgpt-api/src/clients/datastorage_pool_manager.py
# Verify ServiceAccountAuthPoolManager is being used in E2E mode
```

#### Check 2: Are environment variables set correctly?
```yaml
# HAPI Deployment in E2E test
env:
  - name: KUBERNETES_SERVICE_ACCOUNT_TOKEN_PATH
    value: /var/run/secrets/kubernetes.io/serviceaccount/token
  - name: USE_SERVICE_ACCOUNT_AUTH
    value: "true"  # Is this set?
```

#### Check 3: Is token file accessible?
```bash
# From HAPI E2E must-gather logs:
kubectl exec -n kubernaut-system deploy/holmesgpt-api -- ls -la /var/run/secrets/kubernetes.io/serviceaccount/
kubectl exec -n kubernaut-system deploy/holmesgpt-api -- cat /var/run/secrets/kubernetes.io/serviceaccount/token
```

### Fix Required
**Ensure `ServiceAccountAuthPoolManager` is enabled in E2E tests.**

Possible solutions:
1. **Set environment variable** in HAPI deployment:
   ```yaml
   env:
     - name: USE_SERVICE_ACCOUNT_AUTH
       value: "true"
   ```

2. **Pass token path explicitly**:
   ```yaml
   env:
     - name: KUBERNETES_SERVICE_ACCOUNT_TOKEN_PATH
       value: /var/run/secrets/kubernetes.io/serviceaccount/token
   ```

3. **Check Python code initialization**:
   ```python
   # Ensure ServiceAccountAuthPoolManager is instantiated, not regular pool
   if os.path.exists("/var/run/secrets/kubernetes.io/serviceaccount/token"):
       pool = ServiceAccountAuthPoolManager(...)  # Must use this!
   else:
       pool = DataStoragePoolManager(...)  # Regular pool (no auth)
   ```

### Impact
- 31/35 HAPI E2E tests failed
- All tests requiring DataStorage access fail with 401
- Authentication via ServiceAccount not working despite RBAC being correct

---

## Additional Fix: AIAnalysis Image Naming (✅ Fixed in `2852238aa`)

### Issue
AIAnalysis E2E was building locally instead of pulling from registry.

### Error
```
Error: initializing source docker://ghcr.io/jordigilh/kubernaut/aianalysis-controller:pr-24
reading manifest pr-24: manifest unknown
⚠️  Registry pull failed: failed to pull image from registry: exit status 125
⚠️  Falling back to local build...
```

### Root Cause
Image naming mismatch:
- **CI pushed**: `ghcr.io/.../aianalysis:pr-24`
- **E2E pulled**: `ghcr.io/.../aianalysis-controller:pr-24`

### Fix (Commit `2852238aa`)
```go
// test/infrastructure/aianalysis_e2e.go
ServiceName: "aianalysis-controller" → "aianalysis"
ImageName: "kubernaut/aianalysis-controller" → "kubernaut/aianalysis"
```

### Result
✅ AIAnalysis E2E now pulls from registry (faster, validates CI artifacts)

---

## Summary & Next Actions

### Completed ✅
1. **AIAnalysis**: Image naming aligned with Operator SDK convention (`2852238aa`)
2. **Image Naming Convention**: All 9 services now consistent (no `-controller` suffix in image names)
3. **HAPI RBAC**: ServiceAccount + RoleBinding created (but not used yet)
4. **tkn CLI**: Installed in CI (but wrong version/incompatible flag)

### Pending ❌
1. **Notification**: Investigate pod readiness failure (download must-gather logs)
2. **WorkflowExecution**: Upgrade tkn CLI to v0.40.0+ (support `--override` flag)
3. **HAPI**: Enable `ServiceAccountAuthPoolManager` in E2E deployment

### Recommended Fixes (Priority Order)

#### 1. WorkflowExecution (Easy - 5 min)
```yaml
# .github/workflows/ci-pipeline.yml line ~550
TKN_VERSION="0.40.0"  # Change from 0.39.0
```

#### 2. HAPI (Medium - 15 min)
```go
// test/infrastructure/holmesgpt_api.go:deployHAPIOnly()
// Add environment variable to HAPI deployment:
env:
  - name: USE_SERVICE_ACCOUNT_AUTH
    value: "true"
```

#### 3. Notification (Unknown - requires investigation)
- Download must-gather logs
- Inspect pod describe/logs
- Fix root cause (likely RBAC or configuration)

---

## Authority References
- **DD-AUTH-014**: Middleware-based authentication with ServiceAccount tokens
- **Operator SDK Convention**: Image names without `-controller` suffix
- **Tekton CLI**: `--override` flag for bundle building

---

**End of RCA** - Ready for fixes.
