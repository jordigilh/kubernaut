# SignalProcessing Integration Audit Flush Root Cause Analysis - January 15, 2026

## Executive Summary

**Status**: 🔴 **CRITICAL - 1 Real Failure Blocking 100% Pass Rate**
**Root Cause**: **Audit events are buffered and flushed, but NOT visible in DataStorage queries**
**Affected Test**: "should emit audit event with policy-defined fallback severity"
**Pass Rate Impact**: 94% (84/89) → Blocking path to 100%

---

## 🔍 Deep Dive Analysis Using Must-Gather Logs

### Test Environment Configuration
```
Parallel Processes: 12 (HIGH contention environment)
Flush Interval: 100ms (VERY aggressive for 12 parallel processes)
Total Specs: 92
Specs Run: 89
Duration: 160.2 seconds (2m 40s)
```

---

## 🎯 Failing Test Details

### Test: "should emit audit event with policy-defined fallback severity"
**Location**: `test/integration/signalprocessing/severity_integration_test.go:352`
**Correlation ID**: `test-policy-fallback-audit-rr-1768527745230346000`

### Timeline from Must-Gather Logs
```
20:42:25.XXX - Controller processes SignalProcessing CRD "test-policy-fallback-audit"
20:42:25.XXX - Events buffered: phase.transition (total: 57)
20:42:25.XXX - Events buffered: enrichment.completed (total: 58)
20:42:25.XXX - Events buffered: phase.transition (total: 59)
20:42:25.XXX - Events buffered: classification.decision (total: 60) ⬅️ TARGET EVENT
20:42:25.XXX - Events buffered: phase.transition (total: 61)
20:42:25.XXX - Events buffered: signal.processed (total: 63)
20:42:25.XXX - Events buffered: business.classified (total: 64)

20:42:26.238 - Test queries DataStorage: 0 events found ❌
20:42:28.241 - Test queries DataStorage: 0 events found ❌
... (continuous polling for 60 seconds) ...
20:43:26.234 - Test times out after 60 seconds ❌
```

---

## 🚨 Critical Findings from Log Analysis

### Finding 1: Buffer Behavior Anomaly
**Evidence from logs**:
```json
// FAILING TEST (test-policy-fallback-audit)
{
  "msg": "✅ Event buffered successfully",
  "event_type": "signalprocessing.classification.decision",
  "correlation_id": "test-policy-fallback-audit-rr-1768527745230346000",
  "buffer_size_after": 0,  ⬅️ ANOMALY: Buffer is EMPTY after adding event
  "total_buffered": 60
}

// SUCCESSFUL TEST (test-policy-hash) - runs at same time
{
  "msg": "✅ Event buffered successfully",
  "event_type": "signalprocessing.classification.decision",
  "correlation_id": "test-policy-hash-rr-1768527745331072000",
  "buffer_size_after": 1,  ⬅️ NORMAL: Buffer has 1 event after adding
  "total_buffered": 70
}
```

**Analysis**:
- `buffer_size_after: 0` indicates the event was immediately removed from buffer
- `total_buffered: 60` shows the event WAS added (cumulative counter)
- This suggests **immediate flush** or **buffer reset** occurred

### Finding 2: Flush Interval is Extremely Aggressive
**Configuration**: `FlushInterval = 100 * time.Millisecond` (line 295, `suite_test.go`)

**Timer Tick Analysis** (from 3,948 timer ticks):
```
3,825 ticks - batch_size_before_flush: 0  (97% of ticks)
   43 ticks - batch_size_before_flush: 8
   33 ticks - batch_size_before_flush: 1
   23 ticks - batch_size_before_flush: 7
... (various small batches)
    1 tick  - batch_size_before_flush: 62 (largest batch)
```

**Interpretation**:
- 97% of timer ticks find an empty batch (already flushed)
- Events are being flushed VERY rapidly (every ~100ms)
- With 12 parallel processes, this creates **HIGH write contention** on DataStorage

### Finding 3: No Flush Completion Logs
**Expected Logs** (should exist): `"Flushed batch"`, `"POST /api/v1/audit/events/batch"`, `"Successfully wrote N events"`
**Actual Logs**: ❌ **NONE FOUND**

**But**:
- `flushAuditStoreAndWait()` reports: ✅ "Audit store flushed successfully" (12 times)
- This means `auditStore.Flush(ctx)` returns `nil` error
- **Conclusion**: Flush is called and returns success, but events don't reach DataStorage **OR** they reach DataStorage but aren't visible to queries

### Finding 4: Successful vs Failing Test Comparison
**Successful Test** (`test-policy-hash-rr-1768527745331072000`):
- Events buffered at 20:42:25
- Query at 20:42:25.334: 0 events found
- Query at 20:42:27.343: **1 event found** ✅

**Failing Test** (`test-policy-fallback-audit-rr-1768527745230346000`):
- Events buffered at 20:42:25
- Query at 20:42:26.238: 0 events found
- Query at 20:42:28.241: 0 events found
- ... continuous polling for 60 seconds: **0 events found** ❌

**Time Delta**: Both tests run within ~2 seconds of each other, but one succeeds and one fails

---

## 🧩 Root Cause Hypotheses (Ranked by Likelihood)

### Hypothesis A: PostgreSQL Transaction Isolation Issue (HIGH LIKELIHOOD)
**Theory**: DataStorage uses PostgreSQL Read Committed isolation level. With 12 parallel processes writing rapidly:
1. Audit event is written to DataStorage in Transaction A
2. Test queries DataStorage in Transaction B (different process)
3. Transaction B cannot see Transaction A's writes until committed
4. Transaction A takes longer than 60 seconds to commit (unlikely) OR never commits (bug)

**Evidence**:
- ✅ 12 parallel processes (high contention)
- ✅ Extremely fast flush interval (100ms)
- ✅ Events show `buffer_size_after: 0` (flushed immediately)
- ✅ No visibility in queries for 60+ seconds
- ✅ Successful test finds events after ~2 seconds (normal commit time)

**PostgreSQL Default Behavior**:
```sql
-- Read Committed (PostgreSQL default)
BEGIN;
INSERT INTO audit_events VALUES (...);  -- Transaction A (audit flush)
-- Query in Transaction B (test) cannot see this row until COMMIT
COMMIT;  -- Now visible to other transactions
```

**Why one test succeeds and one fails**:
- Random timing: Successful test's transaction commits before query
- Failing test's transaction is still open OR conflicts with other writes

### Hypothesis B: Multiple Audit Store Instances (MEDIUM LIKELIHOOD)
**Theory**: Each of the 12 parallel processes has its own audit store instance (confirmed in code), but they may not all be flushing to the same DataStorage endpoint or connection.

**Evidence**:
- ✅ Code confirms per-process audit store: `auditStore, err = audit.NewBufferedStore(...)` (line 298, `suite_test.go`)
- ✅ Comment: "SP-CACHE-001: Create audit store per-process"
- ❌ All processes use same DataStorage port: `infrastructure.SignalProcessingIntegrationDataStoragePort`

**Why this might cause issues**:
- Different database connections
- Different transaction contexts
- Connection pooling limits (if DataStorage connection pool is exhausted)

### Hypothesis C: Audit Store Flush Implementation Bug (LOW LIKELIHOOD)
**Theory**: The `auditStore.Flush(ctx)` method returns success but doesn't actually persist events to DataStorage.

**Evidence**:
- ❌ Other tests succeed with same flush mechanism
- ❌ AIAnalysis integration tests use same pattern and work
- ✅ No DataStorage write logs found (but may be in separate log file)

---

## 📊 Buffer Behavior Deep Dive

### Normal Buffer Flow (Expected)
```
1. Event arrives → Buffer size: 0 → 1
2. Timer tick (100ms) → Flush triggered
3. Batch sent to DataStorage → HTTP POST → Success
4. Buffer cleared → Buffer size: 1 → 0
```

### Observed Failing Test Behavior
```
1. Event arrives → Buffer size remains: 0 (immediate flush?)
2. No flush logs observed
3. Event not visible in DataStorage queries
```

**Possible Explanations**:
1. **Immediate Flush**: Event is flushed before timer tick (why?)
2. **Buffer Reset**: Buffer is cleared without flushing (bug?)
3. **Different Buffer**: Event goes to a different buffer instance (hypothesis B)

---

## 🎯 Recommended Fixes (Priority Order)

### Fix 1: Increase Flush Interval (IMMEDIATE - Low Risk)
**Rationale**: 100ms flush interval with 12 parallel processes creates extreme write contention

**Implementation**:
```go
// test/integration/signalprocessing/suite_test.go (line 295)
// BEFORE:
auditConfig.FlushInterval = 100 * time.Millisecond // Faster flush for tests

// AFTER:
auditConfig.FlushInterval = 1 * time.Second // Standard flush interval
```

**Expected Impact**: Reduces write contention, allows PostgreSQL transactions to complete before queries

**Risk**: Low - 1 second is still fast enough for tests (AIAnalysis uses 1s successfully)

---

### Fix 2: Add Explicit Commit/Wait After Flush (SHORT-TERM - Medium Risk)
**Rationale**: Ensure PostgreSQL transaction is committed before test queries DataStorage

**Implementation**:
```go
// test/integration/signalprocessing/audit_integration_test.go
func flushAuditStoreAndWait() {
	flushCtx, flushCancel := context.WithTimeout(ctx, 10*time.Second)
	defer flushCancel()

	err := auditStore.Flush(flushCtx)
	if err != nil {
		GinkgoWriter.Printf("⚠️  Flush warning (non-fatal, Eventually will retry): %v\n", err)
	} else {
		GinkgoWriter.Printf("✅ Audit store flushed successfully\n")
	}

	// NEW: Wait for PostgreSQL transaction commit
	// In high-contention scenarios (12 parallel processes), transactions may take time to commit
	time.Sleep(500 * time.Millisecond)  // Allow transaction commit to complete
}
```

**Expected Impact**: Ensures DataStorage has committed audit events before test queries

**Risk**: Medium - Increases test duration by 0.5s per flush call

---

### Fix 3: Use Explicit DataStorage Synchronization API (LONG-TERM - High Risk)
**Rationale**: Add a "barrier" endpoint to DataStorage that blocks until all pending writes are committed

**Implementation**:
```go
// NEW API endpoint in DataStorage
POST /api/v1/audit/sync
// Blocks until all pending writes are committed and visible to queries
// Returns: 200 OK when safe to query

// Usage in tests
func flushAuditStoreAndWait() {
	// ... existing flush ...

	// Wait for DataStorage to commit all pending writes
	_, err := dsClient.SyncAuditEvents(ctx)
	if err != nil {
		GinkgoWriter.Printf("⚠️  Sync warning: %v\n", err)
	}
}
```

**Expected Impact**: Guarantees transaction visibility without sleep-based waits

**Risk**: High - Requires DataStorage API changes

---

### Fix 4: Reduce Parallel Process Count for Tests (ALTERNATIVE - Low Risk)
**Rationale**: Fewer processes = less contention = fewer transaction isolation issues

**Implementation**:
```bash
# Run with 4 processes instead of 12
make test-integration-signalprocessing GINKGO_FLAGS="-p -procs=4"
```

**Expected Impact**: Reduces write contention, may resolve issue without code changes

**Risk**: Low - Increases test duration but improves stability

---

## 🧪 Diagnostic Commands for Further Investigation

### 1. Check DataStorage Connection Pool Configuration
```bash
grep -E "MaxOpenConns|MaxIdleConns" test/integration/signalprocessing/config/config.yaml
```

### 2. Check for DataStorage Errors During Test Window
```bash
# If DataStorage logs exist in a separate file
grep "20:42:2[5-9]\|20:42:3[0-9]" /path/to/datastorage.log | grep -i "error\|fail\|timeout"
```

### 3. Verify PostgreSQL Transaction Isolation Level
```sql
-- Connect to DataStorage PostgreSQL instance
SELECT current_setting('transaction_isolation');
-- Expected: read committed (PostgreSQL default)
```

### 4. Monitor Buffer Size Distribution
```bash
# Already done - showed 97% of ticks have empty batch
grep "batch_size_before_flush" /tmp/sp-integration-FINAL-RESULT.log | \
  grep -oE '"batch_size_before_flush":[0-9]+' | cut -d':' -f2 | \
  sort | uniq -c | sort -rn
```

---

## 📈 Expected Outcomes After Fixes

### If Fix 1 is Applied (Increase Flush Interval to 1s)
- ✅ Reduce write contention by 10x (10 flushes/sec → 1 flush/sec per process)
- ✅ Allow PostgreSQL transactions more time to commit
- ✅ Align with AIAnalysis integration test configuration (proven stable)
- 🎯 **Expected Pass Rate**: 98-100% (eliminate timing-based failures)

### If Fix 2 is Applied (Add 500ms Wait After Flush)
- ✅ Ensure transaction commit before query
- ⚠️ Increase test duration by ~6 seconds (12 flush calls × 0.5s)
- 🎯 **Expected Pass Rate**: 95-98% (reduce but not eliminate failures)

### If Fix 4 is Applied (Reduce to 4 Processes)
- ✅ Reduce write contention by 66% (12 → 4 processes)
- ⚠️  Increase test duration by ~3x (fewer parallel processes)
- 🎯 **Expected Pass Rate**: 98-100% (fewer contention points)

---

## 🚀 Recommended Action Plan

### Phase 1: Immediate (Today)
1. ✅ Apply Fix 1: Increase flush interval to 1 second
2. ✅ Rerun integration tests
3. ✅ Verify pass rate improves to 98-100%

### Phase 2: Validation (If Phase 1 Doesn't Resolve)
1. Apply Fix 2: Add 500ms wait after flush
2. Rerun tests with detailed DataStorage logging
3. Capture PostgreSQL transaction logs

### Phase 3: Long-Term (Next Sprint)
1. Implement Fix 3: DataStorage synchronization API
2. Add integration test for high-contention scenarios
3. Document audit flush behavior in ADR

---

## 📁 Artifacts

### Log Files
- `/tmp/sp-integration-FINAL-RESULT.log` - Must-gather logs with full test output (2.8MB)

### Key Log Sections
- Lines showing `test-policy-fallback-audit-rr-1768527745230346000` - Failing test events
- Lines showing `test-policy-hash-rr-1768527745331072000` - Successful test for comparison
- Timer tick analysis - 3,948 ticks showing buffer behavior

---

## 🏆 Success Criteria

- ✅ 100% pass rate for SignalProcessing integration tests
- ✅ No timing-based failures in parallel execution (12 processes)
- ✅ Audit events visible in DataStorage queries within 2 seconds of flush
- ✅ No "Interrupted by Other Ginkgo Process" failures

---

## 📚 References

- **DD-TESTING-001**: Audit event validation standards
- **BR-SP-090**: Audit event emission requirements
- **SP-CACHE-001**: Per-process audit store design
- **PostgreSQL Docs**: Transaction Isolation (https://www.postgresql.org/docs/current/transaction-iso.html)

---

**Analysis Completed By**: AI Assistant
**Date**: January 15, 2026
**Status**: Ready for Fix 1 implementation (increase flush interval)
**Confidence**: 85% that Fix 1 will resolve the issue
