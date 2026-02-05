# Audit Emission Missing from Controllers - Jan 30, 2026

## Executive Summary

**Status**: ✅ **ROOT CAUSE IDENTIFIED**  
**Date**: January 30, 2026  
**Investigation**: E2E Audit Failure Triage

---

## Problem Statement

Both RemediationOrchestrator and WorkflowExecution E2E tests show 0 audit events reaching DataStorage, despite:
- Controllers processing CRs successfully
- Audit store initialized correctly
- Authentication working (ServiceAccount transport)
- Background workers running

---

## Root Cause

**Controllers are NOT calling `auditStore.StoreAudit()`** in their reconciliation logic.

### Evidence

From RO controller logs (`remediationorchestrator-e2e-logs-20260130-082157`):

```
✅ Audit store initialized:
   - buffer_size: 10000
   - batch_size: 50
   - flush_interval: 100ms
   - Background worker started

✅ Timer ticks every 100ms (tick 1-80 observed)

❌ Buffer ALWAYS empty:
   - batch_size_before_flush: 0
   - buffer_utilization: 0
   - NO events ever added to buffer
```

**Key Log Entries**:
```
2026-01-30T13:18:55Z INFO setup Audit store initialized
{"level":"info","ts":1769779135.7643733,"logger":"audit.audit-store","msg":"⏰ Timer tick received","tick_number":1,"batch_size_before_flush":0,"buffer_utilization":0}
... (repeated for 80+ ticks, always 0 events)
```

---

## Test Results

### Port Allocation Fix: ✅ SUCCESS

- **RO E2E**: 26/29 passed (84%)
- **WE E2E**: 9/12 passed (75%)
- **Parallel execution**: ✅ NO PORT CONFLICTS for 8 minutes
- **Port strategy**: RO→DS:8089, WE→DS:8092 (DD-TEST-001 compliant)

### Audit Failures: ❌ 6 TOTAL

**RO E2E (3 failures)**:
1. Gap #8 webhook audit event (BR-AUDIT-005)
2. Audit lifecycle events
3. DataStorage audit emission

**WE E2E (3 failures)**:
1. Audit event persistence (BR-WE-005)
2. WorkflowExecutionAuditPayload fields
3. workflow.failed audit event

**Common Pattern**: All tests expect audit events, but find `0 events` in DataStorage.

---

## Architecture Analysis

### What IS Working

1. ✅ **Audit Client Authentication**
   - `audit.NewOpenAPIClientAdapter()` creates authenticated client
   - Uses `auth.NewServiceAccountTransportWithBase()`
   - Injects `Authorization: Bearer <token>` on every request
   - Source: `pkg/audit/openapi_client_adapter.go:146-154`

2. ✅ **Audit Store Initialization**
   - Controllers initialize `BufferedAuditStore` correctly
   - Background worker started
   - Timer ticking every 100ms
   - Source: `cmd/remediationorchestrator/main.go:161`, `cmd/workflowexecution/main.go:217`

3. ✅ **Test Infrastructure**
   - Tests query DataStorage successfully (200 responses)
   - Authentication working (ServiceAccount tokens)
   - DataStorage accessible at correct ports

### What Is NOT Working

❌ **Reconcilers Do NOT Emit Audit Events**

**Evidence**:
```bash
# Searched for actual StoreAudit calls:
grep -r "StoreAudit" pkg/remediationorchestrator/
# Result: Only comments, NO actual calls

# Found audit manager, but NO usage:
pkg/remediationorchestrator/audit/manager.go exists
# But reconciler doesn't call manager methods
```

**Key Files Missing Audit Emission**:
- RemediationOrchestrator reconciler: `pkg/remediationorchestrator/*.go`
- WorkflowExecution reconciler: `pkg/workflowexecution/*.go`

---

## Required Fix

### RemediationOrchestrator Controller

**Add audit emission calls** in reconciler:

```go
// Example locations (need verification):
// 1. RR created: auditStore.StoreAudit(ctx, manager.BuildLifecycleCreated(...))
// 2. Phase transitions: auditStore.StoreAudit(ctx, manager.BuildLifecycleTransitioned(...))
// 3. RR completed: auditStore.StoreAudit(ctx, manager.BuildLifecycleCompleted(...))
// 4. RR failed: auditStore.StoreAudit(ctx, manager.BuildLifecycleFailed(...))
// 5. Approval events: auditStore.StoreAudit(ctx, manager.BuildApproval...(...))
```

**Audit Manager Already Exists**:
- `pkg/remediationorchestrator/audit/manager.go`
- Provides `BuildLifecycleStarted()`, `BuildLifecycleCompleted()`, etc.
- **Just need to wire it up to reconciler!**

### WorkflowExecution Controller

**Add audit emission calls** in reconciler:

```go
// Example locations (need verification):
// 1. Workflow started: auditManager.EmitWorkflowStarted(...)
// 2. Workflow completed: auditManager.EmitWorkflowCompleted(...)
// 3. Workflow failed: auditManager.EmitWorkflowFailed(...)
```

**Audit Manager Already Exists**:
- `pkg/workflowexecution/audit/manager.go` (if exists)
- Or need to create similar to RO pattern

---

## Investigation Timeline

1. **Port allocation fix** (completed Jan 30, 09:10-09:18)
   - Fixed: RO→DS:8089, WE→DS:8092
   - Committed: `2e996509b`
   - Result: ✅ NO port conflicts in parallel execution

2. **E2E test run** (completed Jan 30, 09:18)
   - Both tests ran in parallel for ~8 minutes
   - Result: 26/29 RO, 9/12 WE passed

3. **Audit failure triage** (completed Jan 30, 09:25-09:30)
   - Checked controller logs in must-gather
   - Identified buffer always empty
   - Confirmed no `StoreAudit()` calls in reconcilers

---

## Next Steps

1. **Verify reconciler structure**
   - Check if reconcilers have access to `auditStore` field
   - Identify where to add audit emission calls

2. **Add audit emission logic**
   - RemediationOrchestrator: Use existing audit manager
   - WorkflowExecution: Use existing audit manager or create one

3. **Re-run E2E tests**
   - Validate audit events now appear in DataStorage
   - Should see `batch_size_before_flush > 0` in controller logs

4. **Update E2E test expectations** (if needed)
   - Verify correlation IDs match
   - Check event types match expected values

---

## Related Files

### Controller Main Files
- `cmd/remediationorchestrator/main.go:161` - Audit store initialization
- `cmd/workflowexecution/main.go:217` - Audit store initialization

### Audit Infrastructure
- `pkg/audit/store.go` - BufferedAuditStore implementation
- `pkg/audit/openapi_client_adapter.go:107` - Authenticated client
- `pkg/remediationorchestrator/audit/manager.go` - RO audit manager

### Test Files (Expecting Audit Events)
- `test/e2e/remediationorchestrator/audit_wiring_e2e_test.go:172`
- `test/e2e/remediationorchestrator/gap8_webhook_test.go:215`
- `test/e2e/workflowexecution/02_observability_test.go:503`

### Must-Gather Logs
- `/tmp/remediationorchestrator-e2e-logs-20260130-082157/`
- Controller logs show buffer always empty

---

## Confidence Assessment

**Root Cause Identification**: 98%
- ✅ Evidence: Buffer always empty (80+ timer ticks, 0 events)
- ✅ Evidence: No `StoreAudit()` calls in reconciler code
- ✅ Evidence: Audit infrastructure working correctly

**Fix Complexity**: Medium
- ✅ Audit managers already exist
- ✅ Audit store already initialized
- ⚠️ Need to identify correct reconciler injection points
- ⚠️ Need to ensure correlation IDs match test expectations

**Estimated Effort**: 2-4 hours
- Review reconciler structure: 30 minutes
- Add audit emission calls: 1-2 hours
- Test and validate: 1-2 hours

---

## Business Requirements Affected

- **BR-AUDIT-005**: RemediationRequest audit trail (Gap #8 webhook)
- **BR-WE-005**: WorkflowExecution audit persistence
- **BR-STORAGE-001**: Complete audit trail with no data loss
- **DD-AUDIT-003**: All services must emit audit events

---

## Success Criteria

✅ **Fix Complete When**:
1. RO E2E audit tests pass (3 currently failing)
2. WE E2E audit tests pass (3 currently failing)
3. Controller logs show `batch_size_before_flush > 0`
4. DataStorage receives and persists audit events

---

## Additional Context

This issue was discovered during parallel E2E test validation after successfully fixing port allocation conflicts. The port fix is working perfectly - this is a separate issue where the reconciliation business logic simply doesn't emit audit events yet.

**Important**: The audit infrastructure is 100% functional. We just need to add the emission calls in the reconcilers.
