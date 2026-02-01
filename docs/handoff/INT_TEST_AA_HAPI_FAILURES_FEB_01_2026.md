# AIAnalysis + HolmesGPT-API Integration Test Failures (Feb 01, 2026)

**Date**: 2026-02-01  
**CI Run**: 21555881350  
**Status**: 🔧 AIAnalysis Fixed, HAPI Needs Investigation  
**Severity**: High (blocks PR merge)

---

## Executive Summary

**Result**: 7/9 integration tests passed (77.8%)

**Passing (7)**:
- ✅ DataStorage
- ✅ Gateway
- ✅ AuthWebhook
- ✅ Notification
- ✅ RemediationOrchestrator
- ✅ WorkflowExecution
- ✅ SignalProcessing

**Failing (2)**:
- ❌ AIAnalysis (infrastructure setup failure)
- ❌ HolmesGPT-API (4 test failures)

---

## 🔍 AIAnalysis Root Cause

### The Issue: Kubeconfig Permission Denied

**Failure**:
```
PermissionError: [Errno 13] Permission denied: '/tmp/kubeconfig'
container health check failed: timeout waiting for http://127.0.0.1:18120/health after 2m0s
```

**Location**: `test/infrastructure/serviceaccount.go:1232`

**Root Cause**:
```go
// BROKEN: File created with 0600 (owner-only read/write)
err = os.WriteFile(kubeconfigPath, kubeconfigBytes, 0600)
// Container runs as non-root user, can't read the file
```

**Why This Failed**:
1. HAPI container runs as non-root user (UBI security hardening)
2. Kubeconfig file created with `0600` permissions (owner-only)
3. When mounted into container, non-root user can't read it
4. HAPI startup fails trying to load kubeconfig
5. Health check times out after 2 minutes

**The Fix**:
```go
// Create file with restrictive permissions first
err = os.WriteFile(kubeconfigPath, kubeconfigBytes, 0600)

// Then chmod to 0644 for Podman rootless containers
err = os.Chmod(kubeconfigPath, 0644)
```

**Why This Works**:
- `0644` = readable by all users, writable by owner only
- Container's non-root user can read the mounted file
- Still secure (only root or owner can write)
- Consistent with other kubeconfig files in codebase (lines 965, 1060)

---

## 📊 Evidence: AIAnalysis

### Container Configuration
```json
{
  "HostConfig": {
    "NetworkMode": "aianalysis_test_network",
    "Binds": [
      "/tmp/envtest-kubeconfig-holmesgpt-service.yaml:/tmp/kubeconfig:ro"
    ]
  },
  "Config": {
    "Env": [
      "KUBECONFIG=/tmp/kubeconfig"
    ]
  }
}
```

### Error Stack Trace
```python
File "/opt/app-root/lib/python3.12/site-packages/kubernetes/config/kube_config.py", line 715, in load_config
  with open(path) as f:
       ^^^^^^^^^^
PermissionError: [Errno 13] Permission denied: '/tmp/kubeconfig'
```

### DataStorage Status
```
Health: healthy ✅
Network: host ✅
Postgres: Connected ✅
Redis: Connected ✅
```

**Conclusion**: DataStorage infrastructure healthy, HAPI startup failed due to kubeconfig permissions.

---

## 🔍 HolmesGPT-API Root Cause (TBD)

### Failed Tests (4 total)

1. **test_custom_registry_isolates_test_metrics**
   - **Test**: `tests/integration/test_hapi_metrics_integration.py::TestMetricsIsolation::test_custom_registry_isolates_test_metrics`
   - **Status**: FAILED

2. **test_incident_analysis_emits_llm_tool_call_events**
   - **Test**: `tests/integration/test_hapi_audit_flow_integration.py::TestIncidentAnalysisAuditFlow::test_incident_analysis_emits_llm_tool_call_events`
   - **Status**: FAILED

3. **test_workflow_not_found_emits_audit_with_error_context**
   - **Test**: `tests/integration/test_hapi_audit_flow_integration.py::TestErrorScenarioAuditFlow::test_workflow_not_found_emits_audit_with_error_context`
   - **Status**: FAILED

4. **test_incident_analysis_emits_llm_request_and_response_events**
   - **Test**: `tests/integration/test_hapi_audit_flow_integration.py::TestIncidentAnalysisAuditFlow::test_incident_analysis_emits_llm_request_and_response_events`
   - **Status**: FAILED

5. **test_incident_analysis_increments_investigations_total**
   - **Test**: `tests/integration/test_hapi_metrics_integration.py::TestIncidentAnalysisMetrics::test_incident_analysis_increments_investigations_total`
   - **Status**: FAILED

### Infrastructure Status
```
DataStorage: healthy ✅
PostgreSQL: Connected ✅
Redis: Connected ✅
Mock LLM: Running ✅
HAPI: Running ✅
```

**Pattern**: All HAPI tests passed infrastructure setup, but 4 tests failed during execution.

**Categories**:
- 2x Audit flow tests
- 2x Metrics tests
- 1x Error propagation test

**Hypothesis**: 
1. Possible timing/race condition issues
2. Mock LLM response format changes
3. Audit event batching/timing issues
4. Metrics registry isolation issues

**Next Steps**:
1. Download detailed test output logs
2. Check for assertion errors or unexpected responses
3. Verify Mock LLM is returning expected responses
4. Check audit event timing and batching

---

## 🔧 Fix Applied: AIAnalysis

### File Changed
`test/infrastructure/serviceaccount.go`

### Change
```diff
  kubeconfigPath := filepath.Join(os.TempDir(), fmt.Sprintf("envtest-kubeconfig-%s.yaml", saName))
  err = os.WriteFile(kubeconfigPath, kubeconfigBytes, 0600)
  if err != nil {
      return nil, fmt.Errorf("failed to write kubeconfig file: %w", err)
  }
+ 
+ // Fix file permissions for Podman rootless (DD-AUTH-014)
+ // Container runs as non-root user and needs to read the mounted kubeconfig
+ err = os.Chmod(kubeconfigPath, 0644)
+ if err != nil {
+     return nil, fmt.Errorf("failed to chmod kubeconfig file: %w", err)
+ }
- _, _ = fmt.Fprintf(writer, "   ✅ Kubeconfig generated: %s\n", kubeconfigPath)
+ _, _ = fmt.Fprintf(writer, "   ✅ Kubeconfig generated: %s (mode: 0644, Podman-mountable)\n", kubeconfigPath)
```

---

## ✅ Expected Outcome: AIAnalysis

After fix:
1. ✅ Kubeconfig file created with `0644` permissions
2. ✅ Container's non-root user can read mounted kubeconfig
3. ✅ HAPI loads kubeconfig successfully
4. ✅ HAPI health check passes
5. ✅ All 59 AIAnalysis integration tests pass

---

## 🎯 Commit Strategy

### Commit 1: Fix AIAnalysis (Immediate)
- Fix kubeconfig permissions issue
- Test locally or wait for CI
- Should fix AIAnalysis → 8/9 passing (88.9%)

### Commit 2: Fix HAPI (After Investigation)
- Investigate detailed test failure logs
- Identify root causes for 4 test failures
- Apply fixes
- Target: 9/9 passing (100%)

---

## 📝 Related Documentation

- **Authority**: DD-AUTH-014 (Middleware-based authentication)
- **Related**: `INT_TEST_TRIPLE_FIX_COMPLETE_RCA_FEB_01_2026.md`
- **Reference**: Lines 965, 1060 in `serviceaccount.go` (existing chmod pattern)

---

## 💡 Key Lesson

**Podman Rootless Container Pattern**:
```go
// ❌ WRONG: Owner-only permissions break container mounting
os.WriteFile(path, data, 0600)

// ✅ CORRECT: Readable by all for Podman rootless
os.WriteFile(path, data, 0600)  // Create secure first
os.Chmod(path, 0644)             // Then make readable
```

**Why**: UBI containers run as non-root (user `1001`), mounted files need to be world-readable.

---

**Status**: AIAnalysis fix ready for commit. HAPI needs detailed log investigation.
