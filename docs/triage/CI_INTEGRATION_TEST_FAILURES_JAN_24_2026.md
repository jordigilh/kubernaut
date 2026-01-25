# CI Integration Test Failures - January 24, 2026

**Date**: 2026-01-24
**CI Run**: 21316766113
**Branch**: feature/soc2-compliance
**Commit**: e6654c39 (fix(notification): update audit test fixtures for DD-AUDIT-CORRELATION-002)
**Status**: 🔴 2 FAILURES

---

## Executive Summary

**Failed Tests**: 2/9 integration test suites
**Passed Tests**: 7/9 integration test suites

| Service | Status | Failure Count | Duration |
|---------|--------|---------------|----------|
| DataStorage | ❌ FAILED | 1/110 | 1m 7s |
| RemediationOrchestrator | ❌ FAILED | 1/59 | 4m 11s |
| AIAnalysis | ✅ PASSED | 0/59 | - |
| Notification | ✅ PASSED | 0/117 | - |
| SignalProcessing | ✅ PASSED | 0/66 | - |
| WorkflowExecution | ✅ PASSED | 0/92 | - |
| Gateway | ✅ PASSED | 0/132 | - |

---

## Failure 1: DataStorage Hash Chain Verification

### Test Details
- **Test**: `Audit Export Integration Tests - SOC2 > Hash Chain Verification > when exporting audit events with valid hash chain > should verify hash chain integrity correctly`
- **File**: `test/integration/datastorage/audit_export_integration_test.go:221`
- **Status**: ❌ FAILED (1/110 tests)

### Error Message
```
[FAILED] All events should have valid hash chain
Expected
    <int>: 0
to equal
    <int>: 5
```

### Root Cause Analysis

**Expected**: 5 events with valid hash chains
**Actual**: 0 events with valid hash chains (all hash verification failures)

This is a **REGRESSION** from commit `f7114cef` (fix(datastorage): exclude legal hold fields from hash calculation (SOC2 Gap #8)).

The fix we applied to exclude legal hold fields from hash calculation is **NOT WORKING** as expected. The hash chain verification is still failing to find any valid events.

### Investigation Questions

1. **Did the fix actually get deployed?**
   - ✅ Commit `f7114cef` was applied
   - ✅ Code changes are in `pkg/datastorage/repository/audit_events_repository.go`
   - ❓ Need to verify if `audit_export.go` was also updated consistently

2. **Is there a mismatch between creation and verification?**
   - The fix only updated `audit_events_repository.go` (event creation)
   - `audit_export.go` (verification) might still include legal hold fields
   - **HYPOTHESIS**: Creation excludes legal hold, verification includes them → mismatch

3. **Are legal hold fields being set during the test?**
   - The test creates events and then verifies them
   - If legal hold fields are NULL/false, they shouldn't affect the hash
   - But JSON marshaling of nil vs false might differ

### Recommended Fix

**Option A: Complete the Refactor (RECOMMENDED)**
```go
// pkg/datastorage/repository/audit_events_repository.go
// Extract shared helper function

func prepareEventForHashing(event *AuditEvent) AuditEvent {
    eventForHashing := *event
    eventForHashing.EventHash = ""
    eventForHashing.PreviousEventHash = ""
    eventForHashing.EventDate = DateOnly{}
    // SOC2 Gap #8: Legal hold fields excluded
    eventForHashing.LegalHold = false
    eventForHashing.LegalHoldReason = ""
    eventForHashing.LegalHoldPlacedBy = ""
    eventForHashing.LegalHoldPlacedAt = nil
    return eventForHashing
}

// Use in both calculateEventHash() and calculateEventHashForVerification()
```

**Option B: Revert and Redesign**
- Revert commit `f7114cef`
- Redesign hash calculation to be truly immutable
- Consider hash chain versioning for backward compatibility

### Must-Gather Artifacts
- ✅ PostgreSQL logs: `/tmp/kubernaut-must-gather/datastorage-integration-20260124-145020/datastorage_datastorage-postgres-test.log`
- ✅ Redis logs: `/tmp/kubernaut-must-gather/datastorage-integration-20260124-145020/datastorage_datastorage-redis-test.log`

### Impact
- **Severity**: HIGH
- **SOC2 Compliance**: ⚠️ BLOCKED (hash chain integrity verification failing)
- **Business Risk**: Cannot demonstrate tamper-evidence for audit trail

---

## Failure 2: RemediationOrchestrator Namespace Isolation

### Test Details
- **Test**: `BR-ORCH-042: Consecutive Failure Blocking > Blocking Logic Fingerprint Edge Cases > should isolate blocking by namespace (multi-tenant)`
- **File**: `test/integration/remediationorchestrator/blocking_integration_test.go:260`
- **Status**: ❌ FAILED (1/59 tests)
- **Duration**: 120.038 seconds (TIMEOUT)

### Error Message
```
[FAILED] Test timed out after 120 seconds
```

### Root Cause Analysis

**Expected Behavior**:
- Namespace A: Creates 3 RRs with same fingerprint, triggers consecutive failure blocking (threshold = 3)
- Namespace B: Creates 1 RR with **same fingerprint**, should NOT be blocked (different namespace)

**Actual Behavior**:
```
Found active RR with fingerprint: ns-b-fail-1 (namespace: ro-fingerprint-b-9b80d81c-3872)
Routing blocked - will not create SignalProcessing
Reason: DuplicateInProgress
Message: Duplicate of active remediation ns-b-fail-1. Will inherit outcome when original completes.
```

**Problem**: The RO controller's duplicate detection (`CheckDuplicateInProgress`) is finding `ns-b-fail-1` from namespace B and blocking `ns-a-fail-0` from namespace A, even though they are in **different namespaces**.

### Code Location

**File**: `internal/controller/remediationorchestrator/remediationrequest_controller.go`

**Suspected Logic**:
```go
// Somewhere in CheckDuplicateInProgress or similar function
activeRRs := r.listActiveRemediationRequests(fingerprint)
if len(activeRRs) > 0 {
    return true, activeRRs[0]  // ❌ BUG: Not checking namespace!
}
```

### Investigation Questions

1. **Does `CheckDuplicateInProgress` filter by namespace?**
   - ❌ Evidence suggests NO
   - The log shows it found `ns-b-fail-1` from a different namespace

2. **Should duplicate detection be namespace-scoped?**
   - ✅ YES (per BR-ORCH-042 multi-tenant isolation requirement)
   - Each namespace should have independent blocking logic

3. **Is this a test issue or production bug?**
   - Production bug: Multi-tenant deployments would have cross-namespace interference
   - Severity: HIGH (violates namespace isolation)

### Recommended Fix

**Add namespace filtering to duplicate detection**:
```go
// internal/controller/remediationorchestrator/remediationrequest_controller.go

func (r *RemediationRequestReconciler) CheckDuplicateInProgress(
    ctx context.Context,
    fingerprint, namespace string,  // ✅ Add namespace parameter
) (bool, string, error) {
    activeRRs := &remediationv1.RemediationRequestList{}
    if err := r.List(ctx, activeRRs, client.MatchingFields{
        "spec.signalFingerprint": fingerprint,
        "metadata.namespace":      namespace,  // ✅ Filter by namespace
    }); err != nil {
        return false, "", err
    }

    // Filter to only active (non-terminal) phases
    for _, rr := range activeRRs.Items {
        if rr.Status.Phase != remediationv1.PhaseFailed &&
           rr.Status.Phase != remediationv1.PhaseSucceeded {
            return true, rr.Name, nil
        }
    }

    return false, "", nil
}
```

### Must-Gather Artifacts
- ✅ DataStorage logs: `/tmp/kubernaut-must-gather/remediationorchestrator-integration-20260124-145344/remediationorchestrator_remediationorchestrator_datastorage_test.log`
- ✅ PostgreSQL logs: `/tmp/kubernaut-must-gather/remediationorchestrator-integration-20260124-145344/remediationorchestrator_remediationorchestrator_postgres_test.log`
- ✅ Redis logs: `/tmp/kubernaut-must-gather/remediationorchestrator-integration-20260124-145344/remediationorchestrator_remediationorchestrator_redis_test.log`

### Impact
- **Severity**: HIGH
- **Multi-Tenant Isolation**: ⚠️ VIOLATED
- **Business Risk**: Cross-namespace interference in production deployments

---

## Common Patterns

### Pattern 1: Recent Code Changes
Both failures are related to recent code changes:
- DS failure: Commit `f7114cef` (hash chain fix incomplete)
- RO failure: Pre-existing bug exposed by improved test coverage

### Pattern 2: Integration Test Coverage
Integration tests are catching real bugs that unit tests missed:
- DS: Hash calculation consistency
- RO: Namespace isolation logic

### Pattern 3: CI Resource Constraints
- RO test timeout (120s) suggests CI may be slower than local (4 cores vs 12 cores)
- Not a root cause but exacerbates timing issues

---

## Recommended Action Plan

### Immediate (P0 - Block PR Merge)
1. **DataStorage Hash Chain**:
   - [ ] Verify if `audit_export.go` was updated with legal hold exclusion
   - [ ] If not, refactor both files to use shared `prepareEventForHashing()` helper
   - [ ] Run local DS integration tests to verify fix
   - [ ] Document hash exclusion fields in code comments

2. **RemediationOrchestrator Namespace Isolation**:
   - [ ] Add namespace parameter to `CheckDuplicateInProgress()`
   - [ ] Update all callers to pass namespace
   - [ ] Add field index for `metadata.namespace` if not already present
   - [ ] Run local RO integration tests to verify fix

### Short-Term (P1 - Next Sprint)
3. **Test Stability**:
   - [ ] Review timeout values across all integration tests
   - [ ] Consider dynamic timeouts based on CI core count
   - [ ] Add retry logic for flaky timing-dependent tests

4. **Documentation**:
   - [ ] Update SOC2 Gap #8 documentation with hash field exclusions
   - [ ] Document multi-tenant namespace isolation requirements (BR-ORCH-042)
   - [ ] Add architecture decision record for fingerprint scoping

### Long-Term (P2 - Technical Debt)
5. **Hash Chain Versioning**:
   - [ ] Design hash chain version field for future-proofing
   - [ ] Plan migration strategy for existing events
   - [ ] Consider cryptographic signature instead of hash chain

6. **Integration Test Infrastructure**:
   - [ ] Investigate CI performance optimization
   - [ ] Consider dedicated integration test cluster
   - [ ] Improve must-gather artifact collection and analysis

---

## Next Steps

1. **Fix DataStorage Hash Chain**: Extract `prepareEventForHashing()` helper
2. **Fix RO Namespace Isolation**: Add namespace filtering
3. **Local Verification**: Run both integration test suites locally
4. **Push and Monitor**: Push fixes and verify CI passes
5. **Post-Mortem**: Schedule retrospective on how these bugs were introduced

---

## Related Documents

- [DS Hash Chain Verification Fix (Jan 23)](DS_HASH_CHAIN_VERIFICATION_FIX_JAN_23_2026.md)
- [SOC2 Gap #8](../architecture/soc2/gap-8-legal-hold.md)
- [BR-ORCH-042](../business-requirements/BR-ORCH-042-consecutive-failure-blocking.md)
- [Multi-Tenant Architecture](../architecture/multi-tenant-isolation.md)

---

**Confidence**: 90% (root causes identified, fixes proposed, needs verification)
