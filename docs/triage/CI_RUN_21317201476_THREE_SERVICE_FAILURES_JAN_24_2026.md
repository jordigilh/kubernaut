# CI Run 21317201476 - Three Service Failures Analysis

**Date**: 2026-01-24
**CI Run**: 21317201476
**Branch**: feature/soc2-compliance
**Commit**: 26d7cc29 (fix: resolve DS hash chain and RO namespace isolation failures)
**Status**: 🔴 3 FAILURES (NEW REGRESSION)

---

## Executive Summary

**CRITICAL**: Our fixes from commit 26d7cc29 **DID NOT RESOLVE** the original failures, and we now have 3 failing test suites instead of 2.

| Service | Previous Run (21316766113) | Current Run (21317201476) | Delta |
|---------|----------------------------|---------------------------|-------|
| DataStorage | ❌ 1 failure | ❌ 2 failures | +1 NEW |
| RemediationOrchestrator | ❌ 1 failure | ❌ 1 failure | STILL FAILING |
| SignalProcessing | ✅ PASSED | ❌ 1 failure | +1 NEW |
| Notification | ✅ PASSED | ✅ PASSED | - |
| AIAnalysis | ✅ PASSED | ✅ PASSED | - |
| WorkflowExecution | ✅ PASSED | ✅ PASSED | - |
| Gateway | ✅ PASSED | ✅ PASSED | - |

**Test Results**: 108/110 DS, 58/59 RO, 91/92 SP passing

---

## Failure 1: DataStorage Hash Chain (STILL FAILING)

### Test Details
- **Test**: `Audit Export Integration Tests - SOC2 > Hash Chain Verification > should verify hash chain integrity correctly`
- **File**: `test/integration/datastorage/audit_export_integration_test.go:221`
- **Status**: ❌ STILL FAILING (0/5 events valid)

### Error Message
```
[FAILED] All events should have valid hash chain
Expected
    <int>: 0
to equal
    <int>: 5
```

### Root Cause Analysis

**Our Fix Was Incomplete**

We fixed the `json.Marshal()` call (by value vs pointer), but missed a **CRITICAL** detail:

**During Event Creation** (`audit_events_repository.go` line ~270-290):
```go
// CRITICAL: Normalize event_data through JSON round-trip
eventDataJSON, err := json.Marshal(event.EventData)
// ...
var normalizedEventData map[string]interface{}
if err := json.Unmarshal(eventDataJSON, &normalizedEventData); err != nil {
    // error
}
event.EventData = normalizedEventData // ← NORMALIZED before hash calculation
```

**During Event Verification** (`audit_export.go` line ~348-380):
```go
// Just copies the event and clears fields
eventCopy := *event
eventCopy.EventHash = ""
// ...
eventJSON, err := json.Marshal(eventCopy) // ← EventData NOT NORMALIZED
```

**The Problem**: EventData contains numbers that PostgreSQL JSONB normalizes to float64. During creation, we normalize EventData before hashing. During verification, we DON'T normalize, so the JSON is different.

**Example**:
- Creation: `{"retry_count": 1}` → normalized to `{"retry_count": 1.0}` → hash
- Verification: `{"retry_count": 1}` → hash (different JSON, different hash!)

### Correct Fix

**Extract a shared `prepareEventForHashing()` function that includes EventData normalization**:

```go
// pkg/datastorage/repository/audit_events_repository.go

func prepareEventForHashing(event *AuditEvent) (*AuditEvent, error) {
    // Create a copy to avoid modifying the original
    eventForHashing := *event

    // Clear hash fields
    eventForHashing.EventHash = ""
    eventForHashing.PreviousEventHash = ""
    eventForHashing.EventDate = DateOnly{}

    // Clear legal hold fields (SOC2 Gap #8)
    eventForHashing.LegalHold = false
    eventForHashing.LegalHoldReason = ""
    eventForHashing.LegalHoldPlacedBy = ""
    eventForHashing.LegalHoldPlacedAt = nil

    // CRITICAL: Normalize EventData through JSON round-trip
    // PostgreSQL JSONB stores numbers as float64, so we must normalize
    eventDataJSON, err := json.Marshal(eventForHashing.EventData)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal event_data: %w", err)
    }

    var normalizedEventData map[string]interface{}
    if len(eventDataJSON) > 0 && string(eventDataJSON) != "null" {
        if err := json.Unmarshal(eventDataJSON, &normalizedEventData); err != nil {
            return nil, fmt.Errorf("failed to normalize event_data: %w", err)
        }
        eventForHashing.EventData = normalizedEventData
    }

    return &eventForHashing, nil
}

// Use in both calculateEventHash() and calculateEventHashForVerification()
```

---

## Failure 2: DataStorage Unique Constraint (NEW)

### Test Details
- **Test**: `Workflow Catalog Repository Integration Tests > Create > should return unique constraint violation error`
- **File**: `test/integration/datastorage/workflow_repository_integration_test.go:207`
- **Status**: ❌ NEW FAILURE

### Error Message
```
[FAILED] Expected an error to have occurred. Got:
    <nil>: nil
```

### Log Evidence
```
workflow created	{"workflow_id": "049a427d-f03e-4330-b7aa-284c947f51a8", "workflow_name": "...-duplicate", "version": "v1.0.0"}
workflow created	{"workflow_id": "62979aa2-e1ce-4988-8b64-1ef7de74f3ce", "workflow_name": "...-duplicate", "version": "v1.0.0"}
```

### Root Cause

**Database Unique Constraint MISSING or Not Enforced**

The test attempts to create two workflows with the SAME `(workflow_name, version)` composite key:
- 1st insert: SUCCEEDED (correct)
- 2nd insert: SUCCEEDED (WRONG! should have failed with unique constraint violation)

**Expected Behavior**: Second insert should fail with `ERROR: duplicate key value violates unique constraint "workflows_workflow_name_version_key"`

**Actual Behavior**: Both inserts succeeded, each with different `workflow_id` (UUID primary key)

### Possible Causes

1. **Schema Migration Issue**: Unique constraint not created during migration
2. **Test Database State**: Constraint dropped/missing in test environment
3. **Parallel Test Interference**: Another test dropped the constraint
4. **Schema Version Mismatch**: CI using older migration version

### Database Schema Check Required

```sql
-- Check if unique constraint exists
SELECT constraint_name, constraint_type
FROM information_schema.table_constraints
WHERE table_name = 'workflows'
AND constraint_type = 'UNIQUE';

-- Expected output:
-- workflows_workflow_name_version_key | UNIQUE
```

### Fix Strategy

1. **Verify Migration**: Check `pkg/datastorage/repository/migrations/` for workflow table unique constraint
2. **Add Schema Validation**: Add test to verify constraints exist before running tests
3. **Add Migration Idempotency**: Ensure migrations can be re-run safely

**Severity**: HIGH (data integrity issue - could allow duplicate workflows in production)

---

## Failure 3: RemediationOrchestrator Namespace Isolation (STILL FAILING)

### Test Details
- **Test**: `BR-ORCH-042: Consecutive Failure Blocking > should isolate blocking by namespace (multi-tenant)`
- **File**: `test/integration/remediationorchestrator/blocking_integration_test.go:260`
- **Status**: ❌ STILL FAILING (timeout after 120s)

### Root Cause Analysis

**Our Fix Was Correct But Insufficient**

The fix we applied:
```go
// Added namespace parameter
func (r *RoutingEngine) FindActiveRRForFingerprint(
    ctx context.Context,
    namespace string,  // ← Added
    fingerprint string,
    excludeName string,
)

// Updated caller
originalRR, err := r.FindActiveRRForFingerprint(ctx, rr.Namespace, ...)  // ← Updated
```

**But the test is STILL failing**, which suggests:

1. **There may be ANOTHER place** where we query by fingerprint without namespace filtering
2. **The fix didn't get deployed properly** (though git shows it's there)
3. **The test setup is incorrect** (creating wrong fingerprints or namespaces)
4. **CI timing issue** (120s timeout suggests it's waiting for something that never happens)

**Action Required**:
1. Check must-gather logs for actual error details
2. Search for ALL occurrences of fingerprint queries in RO code
3. Verify the fix is actually being used in CI (check build artifacts)

---

## Failure 4: SignalProcessing Phase Transition Auditing (NEW)

### Test Details
- **Test**: `BR-SP-090: SignalProcessing → Data Storage Audit Integration > should create 'phase.transition' audit events`
- **File**: `test/integration/signalprocessing/...`
- **Status**: ❌ NEW FAILURE (was passing in previous run)

### Root Cause

**UNKNOWN** - This is completely new. Possible causes:
1. **Related to DS hash chain failure**: If audit events are failing to be verified, SP tests might fail
2. **Test flakiness**: Audit event timing/buffering issue
3. **Regression**: Our changes somehow affected SP audit trail

**Action Required**: Get detailed error message and logs.

---

## Pattern Analysis

### Pattern 1: Our Fixes Were Incomplete
- DS hash chain: Fixed marshal call, missed EventData normalization
- RO namespace: Fixed function signature, but test still fails (unknown why)

### Pattern 2: New Failures Appeared
- DS unique constraint violation
- SP phase transition auditing

This suggests our changes may have had **UNINTENDED SIDE EFFECTS**.

### Pattern 3: CI vs Local Discrepancy
- **Local**: Both DS and RO tests passed 100%
- **CI**: Both still failing

This indicates:
1. CI environment difference (4 cores vs 12 cores)
2. Timing-dependent issues
3. Database state differences
4. OR our local tests didn't actually exercise the same code paths

---

## Recommended Action Plan

### Immediate (P0)

1. **Download Must-Gather Artifacts**:
   ```bash
   gh run download 21317201476 --name must-gather-logs
   ```

2. **Analyze Detailed Logs**:
   - DS: Check PostgreSQL logs for unique constraint violations
   - RO: Check controller logs for namespace filtering behavior
   - SP: Check audit store logs for phase transition events

3. **Verify Code Deployment**:
   - Confirm 26d7cc29 was actually built and deployed in CI
   - Check if there's a caching issue

### Short-Term (P1)

4. **Fix DS Hash Chain PROPERLY**:
   - Extract `prepareEventForHashing()` helper
   - Include EventData normalization
   - Use in both creation and verification

5. **Fix RO Namespace Isolation**:
   - Search for ALL fingerprint queries
   - Add comprehensive logging to trace why test fails
   - Consider if field index needs namespace compound key

6. **Investigate New Failures**:
   - DS unique constraint: Check test data cleanup
   - SP phase transitions: Check audit buffer flushing

### Long-Term (P2)

7. **Improve Test Reliability**:
   - Add more logging to integration tests
   - Improve must-gather artifact collection
   - Add test retry logic for timing-sensitive tests

8. **CI/Local Parity**:
   - Document differences between CI and local environments
   - Add CI simulation mode for local testing

---

## Critical Questions

1. **Why did DS hash chain pass locally but fail in CI?**
   - Answer: We likely didn't run the EXACT same test, or timing differences exposed the issue

2. **Why did RO namespace isolation pass locally but fail in CI?**
   - Answer: UNKNOWN - needs must-gather analysis

3. **Are the new failures related to our changes?**
   - Answer: UNKNOWN - need to compare with previous runs before our commit

4. **Should we revert 26d7cc29?**
   - Answer: NO - it's a step in the right direction, we just need to complete the fix

---

## Must-Gather Analysis Summary

### DataStorage PostgreSQL Logs

**Key Errors Found**:

1. **Notification Audit Duplicate** (line 15:23:55.114):
   ```
   ERROR: duplicate key value violates unique constraint "notification_audit_notification_id_key"
   DETAIL: Key (notification_id)=(notif-test-4-f0db850d-8f83-48c5-a434-977c21c9961c) already exists.
   ```
   - **Impact**: Parallel test collision in notification_audit table
   - **Related**: May indicate test isolation issue

2. **Foreign Key Constraint Violation** (line 15:23:55.144):
   ```
   ERROR: update or delete on table "audit_events_2026_01" violates foreign key constraint
         "audit_events_parent_event_id_parent_event_date_fkey" on table "audit_events"
   DETAIL: Key (event_id, event_date)=(9b006cfa-8af5-4ed3-81f4-447cf8a15b67, 2026-01-24)
           is still referenced from table "audit_events".
   STATEMENT: DELETE FROM audit_events WHERE event_id = $1
   ```
   - **Impact**: Cannot delete parent events during test cleanup
   - **Related**: Hash chain test cleanup or event hierarchy issue

### RemediationOrchestrator Logs

**Status**: Logs show timeout waiting for RemediationRequest to unblock (120s)

### SignalProcessing Logs

**Status**: Audit phase transition events not being created or verified

---

## Next Steps

1. ✅ Download must-gather artifacts from run 21317201476
2. ✅ Analyze PostgreSQL, Redis, and controller logs
3. ⏳ Fix DS hash chain with proper EventData normalization
4. ⏳ Fix DS workflow catalog unique constraint (schema verification)
5. ⏳ Fix RO namespace isolation (add more logging first)
6. ⏳ Investigate SP audit failure (audit buffer/timing)
7. ⏳ Run ALL integration tests locally (not just DS and RO)
8. ⏳ Push and verify CI passes

---

**Confidence**: 70% on RCA (need must-gather logs to confirm)
**Priority**: P0 (blocking PR merge)
**Impact**: SOC2 compliance, multi-tenant isolation, audit trail integrity

---

## Related Documents
- [Previous CI Failure Analysis](CI_INTEGRATION_TEST_FAILURES_JAN_24_2026.md)
- [DS Hash Chain Fix (Jan 23)](DS_HASH_CHAIN_VERIFICATION_FIX_JAN_23_2026.md)
