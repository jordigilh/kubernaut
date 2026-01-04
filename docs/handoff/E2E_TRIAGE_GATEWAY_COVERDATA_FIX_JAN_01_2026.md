# Gateway E2E Test Triage - Missing Coverdata Directory

**Date**: January 1, 2026
**Status**: ✅ **FIXED** - Gateway E2E tests now starting
**Service**: Gateway (GW)
**Test Tier**: E2E Tests

---

## 🚨 **Issue Summary**

Gateway E2E tests were failing immediately in `SynchronizedBeforeSuite` with:
```
ERROR: failed to create cluster: command "podman run..." failed with error: exit status 125
Error: statfs /Users/jgil/go/src/github.com/jordigilh/kubernaut/coverdata: no such file or directory
```

---

## 🔍 **Root Cause Analysis**

### Discovery Process

1. **Initial Error**: E2E tests failed with generic "kind create cluster failed: exit status 1"
2. **Manual Test**: Tried creating Kind cluster manually to get detailed error
3. **First Attempt**: Failed with "kind-config.yaml: no such file or directory" (wrong path used)
4. **Correct Path**: Found config at `test/infrastructure/kind-gateway-config.yaml`
5. **Second Attempt**: Revealed actual error: missing `coverdata` directory

### Root Cause

**File**: `test/infrastructure/kind-gateway-config.yaml`

The Kind config includes a volume mount for coverage collection (DD-TEST-007):
```yaml
extraMounts:
  - hostPath: ./coverdata  # Relative to project root
    containerPath: /coverdata
```

**Problem**: The `coverdata` directory didn't exist in the project root, causing Kind cluster creation to fail.

**Why This Matters**:
- Coverage collection is part of DD-TEST-007 (E2E Coverage Capture Standard)
- Kind validates host paths exist before creating the cluster
- Without this directory, no E2E tests can run

---

## ✅ **Fix Applied**

### Solution

```bash
mkdir -p /Users/jgil/go/src/github.com/jordigilh/kubernaut/coverdata
chmod 777 coverdata  # Allow Kind containers to write coverage data
```

### Verification

Manually tested Kind cluster creation:
```bash
KIND_EXPERIMENTAL_PROVIDER=podman kind create cluster \
  --name gateway-e2e-test \
  --config test/infrastructure/kind-gateway-config.yaml \
  --wait 2m
```

**Result**: ✅ Cluster created successfully in 9 seconds

---

## 📋 **Impact Analysis**

### Services Affected

This issue likely affects ALL services using Kind for E2E tests:
- ✅ Gateway (GW) - **FIXED**
- ⚠️ AIAnalysis (AA) - **NEEDS CHECK**
- ⚠️  Data Storage (DS) - **NEEDS CHECK**
- ⚠️  RemediationOrchestrator (RO) - **NEEDS CHECK**
- ⚠️  WorkflowExecution (WE) - **NEEDS CHECK**
- ⚠️  SignalProcessing (SP) - **NEEDS CHECK**
- ⚠️  Notification (notif) - **NEEDS CHECK**

**Why All Services**: Every service using coverage collection (DD-TEST-007) will have the same Kind config pattern.

### Kind Config Files Using coverdata Mount

```bash
$ grep -l "coverdata" test/infrastructure/kind-*-config.yaml

test/infrastructure/kind-aianalysis-config.yaml
test/infrastructure/kind-datastorage-config.yaml
test/infrastructure/kind-gateway-config.yaml
test/infrastructure/kind-notification-config.yaml
test/infrastructure/kind-remediationorchestrator-config.yaml
test/infrastructure/kind-signalprocessing-config.yaml
test/infrastructure/kind-workflowexecution-config.yaml
```

**Conclusion**: All 7 services would have failed E2E tests before the fix.

---

## 🎯 **Action Items**

### ✅ Completed
- [x] Created `coverdata` directory in project root
- [x] Set appropriate permissions (777)
- [x] Verified Gateway E2E test starts successfully
- [x] Documented root cause and fix

### 🔄 In Progress
- [ ] Gateway E2E tests running (monitoring for other issues)

### 📋 Pending
- [ ] Verify other services' E2E tests work with same fix
- [ ] Consider adding `coverdata/` to `.gitignore` if not already present
- [ ] Consider adding directory creation to CI/CD setup scripts
- [ ] Update E2E test documentation to mention `coverdata` prerequisite

---

## 💡 **Recommendations**

### Short-Term (Immediate)
1. Run E2E tests for all remaining services to validate fix applies universally
2. Add to `.gitignore`:
   ```gitignore
   # E2E Coverage Data (DD-TEST-007)
   coverdata/
   ```

### Medium-Term (Next Sprint)
3. Add to CI/CD workflow preparation:
   ```yaml
   - name: Create E2E coverage directory
     run: mkdir -p coverdata && chmod 777 coverdata
   ```

4. Add to developer setup documentation:
   ```markdown
   ## E2E Test Setup
   Before running E2E tests, create the coverage directory:
   ```bash
   mkdir -p coverdata
   chmod 777 coverdata
   ```
   ```

### Long-Term (Future Enhancement)
5. Consider using `tmpfs` mount in Kind config for coverage (ephemeral, no cleanup needed)
6. Evaluate if coverage collection should be optional (environment variable flag)

---

## 📊 **Test Execution Status**

### Gateway E2E Tests
**Status**: 🔄 Running
**Started**: 2026-01-01 10:24:00
**Expected Duration**: ~8-10 minutes
**Log**: `/tmp/gateway_e2e_retry_20260101_102400.log`

**Progress Monitoring**:
```bash
# Monitor progress
tail -f /tmp/gateway_e2e_retry_*.log | grep -E "PASS|FAIL|Running|specs"
```

---

## 🔗 **Related Documentation**

- [DD-TEST-007: E2E Coverage Capture Standard](../architecture/decisions/DD-TEST-007-e2e-coverage-capture.md)
- [Kind Configuration Pattern](../../test/infrastructure/kind-gateway-config.yaml)
- [Gateway E2E Test Suite](../../test/e2e/gateway/)

---

## ✅ **Success Criteria**

- [x] Kind cluster creates successfully
- [ ] Gateway E2E test suite runs to completion
- [ ] All E2E tests pass or have documented failures
- [ ] Other services' E2E tests validated with same fix

---

**Next Steps**: Monitor Gateway E2E test completion, then proceed to triage other services' E2E tests.


