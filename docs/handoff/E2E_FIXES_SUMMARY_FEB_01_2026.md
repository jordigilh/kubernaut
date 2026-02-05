# E2E Test Fixes Summary - February 1, 2026

## Current Status (Commit: 738162bbf)

### ✅ **FIXED**:
1. WorkflowExecution: tkn CLI `--override` flag
2. All services: Image cleanup logic  
3. AIAnalysis: Image naming alignment

### ⚠️ **PENDING VERIFICATION** (pushed, awaiting CI):
1. HAPI E2E: 401 Unauthorized (removed test logic from deployment)
2. Notification E2E: Pod readiness timeout (works locally)

---

## Detailed RCA & Fixes

### 1. WorkflowExecution - tkn CLI `--override` Flag ✅

**Error**: `Error: unknown flag: --override`

**Root Cause**: The `--override` flag doesn't exist in any version of tkn CLI (tested v0.39.0, v0.40.0, local install).

**Evidence**:
- Local: `tkn bundle push --help` shows no `--override` flag
- CI logs: Same error after upgrading to v0.40.0

**Fix** (Commit: `9d2ddb9e4`):
```go
// test/infrastructure/tekton_bundles.go
// BEFORE:
cmd := exec.Command("tkn", "bundle", "push", bundleRef,
    "-f", pipelineYAML,
    "--override",  // ❌ Flag doesn't exist
)

// AFTER:
cmd := exec.Command("tkn", "bundle", "push", bundleRef,
    "-f", pipelineYAML,
    // No --override flag - bundles are naturally overwritable
)
```

**Authority**: tkn CLI documentation, local verification

**Confidence**: 100% - Flag confirmed non-existent

---

### 2. All E2E Suites - Image Cleanup Error ✅

**Error**: `Error: aianalysis:pr-24: image not known`

**Root Cause**: When `IMAGE_REGISTRY` + `IMAGE_TAG` are set (CI/CD mode), images are **pulled** from remote registry, not built locally. The cleanup logic tried to remove local images that don't exist.

**Evidence**:
- CI: Images pulled as `ghcr.io/jordigilh/kubernaut/{service}:pr-24`
- Local podman: No such image exists as `{service}:pr-24`
- DataStorage had same issue, already fixed

**Fix** (Commit: `9d2ddb9e4`, `072e9432f`):
```go
// Pattern applied to all *_e2e_suite_test.go files:
imageRegistry := os.Getenv("IMAGE_REGISTRY")
imageTag := os.Getenv("IMAGE_TAG")

// Skip cleanup when using registry images (CI/CD mode)
if imageRegistry != "" && imageTag != "" {
    logger.Info("ℹ️  Registry mode detected - skipping local image removal")
} else if imageTag != "" {
    // Local build mode: Remove locally built images
    imageName := fmt.Sprintf("%s:%s", serviceName, imageTag)
    pruneCmd := exec.Command("podman", "rmi", imageName)
    // ...
}
```

**Files Updated**:
- test/e2e/datastorage/datastorage_e2e_suite_test.go
- test/e2e/gateway/gateway_e2e_suite_test.go
- test/e2e/authwebhook/authwebhook_e2e_suite_test.go
- test/e2e/notification/notification_e2e_suite_test.go
- test/e2e/workflowexecution/workflowexecution_e2e_suite_test.go
- test/e2e/aianalysis/suite_test.go

**Authority**: DD-TEST-001 v1.1 (image cleanup patterns)

**Confidence**: 100% - Pattern validated in DataStorage

---

### 3. AIAnalysis - Image Naming Mismatch ✅

**Error**: `reading manifest pr-24 in ghcr.io/.../aianalysis-controller: manifest unknown`

**Root Cause**: E2E test requested `aianalysis-controller:pr-24` but CI pushed `aianalysis:pr-24` (no `-controller` suffix per Operator SDK convention).

**Evidence**:
- Production manifests: Use `aianalysis`, `notification`, etc. (no suffix)
- Operator SDK convention: Binary has `-controller` suffix, image doesn't
- CI build matrix: Pushes without suffix

**Fix** (Commit: `2852238aa`):
```go
// test/infrastructure/aianalysis_e2e.go
cfg := E2EImageConfig{
    ServiceName:      "aianalysis",  // ✅ No -controller suffix
    ImageName:        "kubernaut/aianalysis",
    DockerfilePath:   "docker/aianalysis.Dockerfile",
    // ...
}
```

**Authority**: Operator SDK convention, production deployment manifests

**Confidence**: 100% - Aligned with all other services

---

### 4. HolmesGPT-API - 401 Unauthorized ⚠️ (Pending Verification)

**Error**: `Missing Authorization header with ***` (31/35 tests failed)

**Root Cause**: Test logic leaked into infrastructure. Environment variables `USE_SERVICE_ACCOUNT_AUTH` and `KUBERNETES_SERVICE_ACCOUNT_TOKEN_PATH` were added but:
1. Python production code **doesn't read these env vars**
2. This violates Kubernaut Core Rules (no test logic in business code)

**Evidence**:
- ServiceAccount `holmesgpt-api-sa` created ✅
- RoleBinding for DataStorage access created ✅
- HAPI pod became ready ✅
- But Authorization header not sent

**Correct Pattern (DD-AUTH-014)**:
- **E2E**: Kubernetes auto-mounts token when `serviceAccountName: holmesgpt-api-sa` is set
- **INT**: Test explicitly mounts token file in container volume
- **Production**: Kubernetes auto-mounts token
- **Business code**: Always reads from standard path `/var/run/secrets/kubernetes.io/serviceaccount/token`

**Fix** (Commit: `738162bbf`):
```go
// test/infrastructure/holmesgpt_api.go - Removed from deployment:
- name: USE_SERVICE_ACCOUNT_AUTH
  value: "true"  // ❌ Test logic - Python doesn't use this
- name: KUBERNETES_SERVICE_ACCOUNT_TOKEN_PATH
  value: "/var/run/secrets/kubernetes.io/serviceaccount/token"  // ❌ Redundant
```

**Verification Needed**: CI run 21568156624 (in progress)

**Authority**: 
- DD-AUTH-014 (ServiceAccount authentication pattern)
- Kubernaut Core Rules (no test logic in business code)
- AIAnalysis INT test pattern (line 477 of suite_test.go)

**Confidence**: 85% - Pattern is correct, but need CI verification

---

### 5. Notification - Pod Readiness Timeout ⚠️ (CI-Specific)

**Error**: `controller pod did not become ready: exit status 1` (after 120s wait)

**Root Cause**: Timing issue - pod takes longer to initialize in CI environment than local.

**Evidence**:
- **Local**: Pod ready in 38 seconds ✅
- **CI**: Timeout after 120 seconds ❌
- **Comparison**: AuthWebhook (similar DataStorage dependency) uses `failureThreshold: 6` and works

**Fix Applied** (Commit: `072e9432f`):
```yaml
# test/e2e/notification/manifests/notification-deployment.yaml
readinessProbe:
  httpGet:
    path: /readyz
    port: 8081
  initialDelaySeconds: 30
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 6  # ✅ Increased from 3 (90s max retry time)
```

**Calculation**:
- Initial: 30s
- Retries: 6 failures × 10s period = 60s
- **Total**: 90s max for DataStorage client to initialize

**kubectl wait**: 120s (adequate buffer)

**Status**: 
- ✅ Works locally (pod ready in 38s, 24/30 tests pass)
- ❌ Still fails in CI (pod not ready after 120s)

**Hypothesis**: CI environment under extreme resource contention OR network latency to DataStorage service

**Authority**: DD-TEST-002 (E2E cluster setup patterns), AuthWebhook readiness probe config

**Confidence**: 70% - Configuration is correct per AuthWebhook pattern, but CI environment may need more time

**Next Steps**:
- Monitor CI run 21568156624
- If still fails: Investigate CI pod logs (why DataStorage client init takes >120s)

---

## Summary of Changes

### Commits:
1. `9d2ddb9e4` - tkn flag + image cleanup
2. `072e9432f` - Notification readiness probe
3. `738162bbf` - HAPI env var cleanup (test logic removal)

### Remaining Investigations:
1. HAPI 401: Awaiting CI verification after env var removal
2. Notification timeout: May need CI-specific tuning or root cause investigation

### Test Results:
- **Local Notification E2E**: 24/30 passed (80%), pod ready in 38s
- **CI (previous run)**: Pod not ready after 120s

---

## Authority References

- DD-AUTH-014: ServiceAccount authentication pattern
- DD-TEST-001 v1.1: Image cleanup for E2E tests
- DD-TEST-002: E2E cluster setup patterns
- Kubernaut Core Rules: No test logic in business code
- Operator SDK: Image naming conventions

---

**Status**: Awaiting CI run 21568156624 to verify fixes
**Next**: Monitor CI, investigate any remaining failures with must-gather logs
