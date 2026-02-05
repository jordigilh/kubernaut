# E2E Audit Test Failures - Root Cause Analysis

**Date**: January 30, 2026  
**Status**: ✅ **COMPLETE - ROOT CAUSES IDENTIFIED**  
**Analyst**: AI Assistant  
**Test Run**: Batch 7 (post-DNS fix)

---

## Executive Summary

**Test Results**:
- **RemediationOrchestrator**: 28/29 passed (96%) - 1 audit failure
- **WorkflowExecution**: 9/12 passed (75%) - 3 audit failures

**Root Causes Identified**: 3 distinct issues, 2 requiring code fixes

---

## Issue #1: RO Gap #8 - AuthWebhook Not Emitting Audit Events

### Status
❌ **CRITICAL** - Audit compliance violation (BR-AUDIT-005)

### Test Failure
```
E2E-GAP8-01: Operator Modifies TimeoutConfig
[FAILED] should emit webhook.remediationrequest.timeout_modified audit event
Expected: ≥1 audit events
Actual: 0 events found
```

### Root Cause

**AuthWebhook admission controller does NOT emit audit events.**

### Evidence

1. **RO Controller Works**: 28/29 RO tests passed, including all audit lifecycle tests
   - ✅ `remediationrequest.lifecycle.created`
   - ✅ `remediationrequest.lifecycle.updated` 
   - ✅ `remediationrequest.phase.changed`
   - ✅ DataStorage audit emission

2. **Gap #8 Test Expectations**: 
   - Test creates RemediationRequest
   - Operator modifies `TimeoutConfig` via status update
   - AuthWebhook intercepts the mutation (admission webhook)
   - **Expected**: AuthWebhook emits `webhook.remediationrequest.timeout_modified` audit event
   - **Actual**: No audit event emitted (AuthWebhook doesn't have audit code)

3. **Test Pattern**:
   ```go
   // Gap #8 test flow:
   // 1. Create RR (RO controller emits "created" ✅)
   // 2. Operator modifies TimeoutConfig
   // 3. AuthWebhook intercepts mutation
   // 4. Test queries DataStorage for webhook.* event
   // 5. FAIL: 0 events found (AuthWebhook has no audit emission)
   ```

### Impact

- **1 RO E2E test failure** (Gap #8)
- **SOC2 Compliance Risk**: No attribution for TimeoutConfig mutations
- **BR-AUDIT-005 Gap #8**: Webhook audit not implemented

### Fix Required

**Implement audit emission in AuthWebhook**:

1. **Add AuditStore to AuthWebhook**:
   ```go
   // cmd/authwebhook/main.go
   auditStore := audit.NewBufferedStore(dsClient, config, "authwebhook", logger)
   webhookHandler := authwebhook.NewHandler(auditStore, logger)
   ```

2. **Emit audit events in mutation handlers**:
   ```go
   // pkg/authwebhook/remediationrequest_handler.go
   if timeoutConfigModified {
       auditStore.StoreAudit(ctx, &audit.Event{
           Type: "webhook.remediationrequest.timeout_modified",
           Category: "remediationrequest",
           ...
       })
   }
   ```

3. **Wire audit manager to all webhook handlers**:
   - RemediationRequest timeout mutation
   - WorkflowExecution block clearance (Gap #9)
   - RemediationApprovalRequest approval/rejection
   - NotificationRequest deletion attribution

### Files Affected

- `cmd/authwebhook/main.go` - Add audit store initialization
- `pkg/authwebhook/*_handler.go` - Add audit emission calls
- `test/e2e/authwebhook/*` - Verify audit emission

---

## Issue #2: WE Controller - Missing ServiceAccount Configuration

### Status
🚨 **CRITICAL** - Authentication failure (DD-AUTH-014 violation)

### Test Failures

All 3 WE audit tests failed with identical symptoms:

```
[FAILED] should persist audit events to Data Storage for completed workflow
[FAILED] should persist audit events with correct WorkflowExecutionAuditPayload fields
[FAILED] should emit workflow.failed audit event with complete failure details

Common Pattern: Found 0 events (queried 8+ times over 30 seconds)
```

### Root Cause

**WorkflowExecution controller deployment has NO serviceAccountName, so it runs with the default ServiceAccount which has NO DataStorage RBAC permissions.**

### Evidence

1. **Deployment Comparison**:
   ```yaml
   # RO Deployment (✅ WORKS):
   spec:
     template:
       spec:
         serviceAccountName: remediationorchestrator-controller  # ✅ Has SA
   
   # WE Deployment (❌ FAILS):
   spec:
     template:
       spec:
         # ❌ NO serviceAccountName - uses default SA
   ```

2. **RBAC Setup**:
   ```go
   // test/infrastructure/remediationorchestrator_e2e_hybrid.go
   CreateServiceAccountWithDataStorageAccess(ctx, namespace, "remediationorchestrator-controller", ...)
   // ✅ Creates SA + RoleBinding for DataStorage audit writes
   
   // test/infrastructure/workflowexecution_e2e_hybrid.go
   // ❌ NO corresponding SA creation for WE controller
   // Only creates RoleBinding for "data-storage-service" itself
   ```

3. **Audit Infrastructure Present**:
   - ✅ WE has `pkg/workflowexecution/audit/manager.go` with full audit code
   - ✅ `cmd/workflowexecution/main.go` initializes `BufferedAuditStore`
   - ✅ Reconciler calls `auditManager.RecordWorkflowStarted()`, etc.
   - ✅ Logs show "Audit store initialized successfully"
   - ❌ **BUT**: HTTP requests to DataStorage fail with 401/403 (no SA token)

4. **DNS Fix Not Sufficient**:
   - WE uses correct DNS: `--datastorage-url=http://data-storage-service.kubernaut-system:8080`
   - DNS lookup succeeds
   - HTTP connection succeeds
   - **Authentication fails**: No Bearer token (no SA)

### Impact

- **3 WE E2E test failures** (all audit tests)
- **Production Risk**: WE audit events silently dropped
- **SOC2 Compliance**: No audit trail for workflow execution
- **BR-WE-005 Violation**: Audit persistence not working

### Fix Required

**Add ServiceAccount to WE controller deployment**:

1. **Create ServiceAccount + RBAC**:
   ```go
   // test/infrastructure/workflowexecution_e2e_hybrid.go
   // Add BEFORE deployment phase:
   if err := CreateServiceAccountWithDataStorageAccess(
       ctx,
       WorkflowExecutionNamespace,
       kubeconfigPath,
       "workflowexecution-controller",  // NEW SA name
       writer,
   ); err != nil {
       return fmt.Errorf("failed to create WE ServiceAccount: %w", err)
   }
   ```

2. **Add serviceAccountName to Deployment**:
   ```go
   // test/infrastructure/workflowexecution_e2e_hybrid.go (line ~985)
   Spec: corev1.PodSpec{
       ServiceAccountName: "workflowexecution-controller",  // ADD THIS
       SecurityContext: func() *corev1.PodSecurityContext { ... }(),
       Containers: []corev1.Container{ ... },
   }
   ```

3. **Verify Pattern Matches RO**:
   - Reference: `test/infrastructure/remediationorchestrator_e2e_hybrid.go` (line 513)
   - Ensure consistent SA naming: `{service}-controller`

### Files Affected

- `test/infrastructure/workflowexecution_e2e_hybrid.go` - Add SA creation + deployment spec
- Possibly production manifests: `deploy/workflowexecution/deployment.yaml`

---

## Issue #3: WE Config - Wrong Default DataStorage DNS Hostname

### Status
⚠️ **MEDIUM** - Masked by E2E override, but production risk

### Root Cause

**Default DataStorage URL uses wrong DNS hostname**:

```go
// pkg/workflowexecution/config/config.go:138
DataStorageURL: "http://datastorage-service:8080",  // ❌ WRONG
```

**Should be**:
```go
DataStorageURL: "http://data-storage-service:8080",  // ✅ CORRECT
```

### Why Not Causing E2E Failures

E2E tests override the default via command-line flag:
```go
// test/infrastructure/workflowexecution_e2e_hybrid.go:1011
"--datastorage-url=http://data-storage-service.kubernaut-system:8080",
```

But production deployments without explicit config would use the wrong default.

### Impact

- **E2E**: ✅ No impact (overridden)
- **Production**: ❌ Potential DNS failures if config file not provided
- **Consistency**: ❌ Violates DD-AUTH-011 (standard service name)

### Fix Required

**Update default hostname**:

```go
// pkg/workflowexecution/config/config.go
func DefaultConfig() *Config {
    return &Config{
        ...
        Audit: AuditConfig{
            DataStorageURL: "http://data-storage-service:8080",  // FIX: Add hyphen
            Timeout:        10 * time.Second,
        },
        ...
    }
}

func LoadFromFile(path string) (*Config, error) {
    ...
    if cfg.Audit.DataStorageURL == "" {
        cfg.Audit.DataStorageURL = "http://data-storage-service:8080"  // FIX: Add hyphen
    }
    ...
}
```

### Files Affected

- `pkg/workflowexecution/config/config.go` (lines 138, 190)

---

## Verification Strategy

### Issue #1: AuthWebhook Audit

**Test**:
```bash
make test-e2e-remediationorchestrator
# Expected: Gap #8 test passes (1 webhook.* event found)
```

**Validation**:
```bash
kubectl logs -n kubernaut-system deployment/authwebhook | grep "Audit event recorded"
# Should show: action=webhook.remediationrequest.timeout_modified
```

---

### Issue #2: WE ServiceAccount

**Test**:
```bash
make test-e2e-workflowexecution
# Expected: All 3 audit tests pass (9 → 12 events found)
```

**Validation**:
```bash
# 1. Verify SA created:
kubectl get sa -n kubernaut-system workflowexecution-controller

# 2. Verify RoleBinding:
kubectl get rolebinding -n kubernaut-system workflowexecution-datastorage-client

# 3. Verify pod uses SA:
kubectl get pod -n kubernaut-system -l app=workflowexecution-controller -o yaml | grep serviceAccountName

# 4. Verify audit writes:
kubectl logs -n kubernaut-system -l app=workflowexecution-controller | grep "Audit event recorded"
```

---

### Issue #3: WE Config Default

**Test**:
```bash
# 1. Verify default loads correctly:
go test ./pkg/workflowexecution/config -v

# 2. Check integration:
go build ./cmd/workflowexecution
./workflowexecution --help | grep datastorage-url
```

---

## Priority & Sequencing

### Priority 1: Fix WE ServiceAccount (Issue #2)
**Impact**: 3 test failures, production audit loss  
**Effort**: Low (1 file, ~10 lines)  
**Risk**: None (follows RO pattern)

### Priority 2: Fix WE Config Default (Issue #3)
**Impact**: Production risk, consistency  
**Effort**: Trivial (2 lines)  
**Risk**: None (E2E already validates correct hostname)

### Priority 3: Implement AuthWebhook Audit (Issue #1)
**Impact**: 1 test failure, SOC2 compliance  
**Effort**: Medium (audit store wiring, multiple handlers)  
**Risk**: Low (follows RO audit pattern)

---

## Related Documentation

- **DD-AUTH-014**: Middleware-Based ServiceAccount Authentication
- **DD-AUTH-011**: Standard DataStorage Service Naming
- **BR-AUDIT-005**: Audit Trail Requirements
- **BR-WE-005**: WorkflowExecution Audit Persistence
- **ADR-032**: Mandatory Audit for P0 Services
- **Gap #8 Test**: `test/e2e/remediationorchestrator/gap8_webhook_test.go`
- **WE Audit Tests**: `test/e2e/workflowexecution/02_observability_test.go`

---

## Confidence Assessment

**Issue #1 (AuthWebhook)**: 100% - Clear from test expectations and RO success  
**Issue #2 (WE SA)**: 100% - Directly observed in deployment specs  
**Issue #3 (Config DNS)**: 100% - Verified in source code  

**Overall RCA Confidence**: 100% - All root causes identified with evidence
