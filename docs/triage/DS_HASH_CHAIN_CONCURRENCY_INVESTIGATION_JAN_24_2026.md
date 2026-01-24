# DataStorage Hash Chain Concurrency Investigation

**Date**: 2026-01-24  
**Status**: 🔍 INVESTIGATION IN PROGRESS  
**Hypothesis**: Concurrency/ordering issues causing hash chain breaks  

---

## Findings from Integration Test Analysis

### Test Structure

**File**: `test/integration/datastorage/audit_export_integration_test.go:176-231`

```go
It("should verify hash chain integrity correctly", func() {
    correlationID := fmt.Sprintf("soc2-export-%s-verify-valid", testID)
    
    // Create 5 events SEQUENTIALLY (not parallel)
    for i := 0; i < 5; i++ {
        event := &repository.AuditEvent{
            EventID: uuid.New(),
            // ... fields ...
            EventData: map[string]interface{}{"step": i, "status": "completed"},
        }
        _, err := auditRepo.Create(ctx, event)
        Expect(err).ToNot(HaveOccurred())
    }
    
    // 100ms sleep to ensure transactions commit
    time.Sleep(100 * time.Millisecond)
    
    // Export and verify
    result, err := auditRepo.Export(ctx, filters)
    
    // EXPECTATION: 5/5 events valid
    // ACTUAL IN CI: 0/5 events valid ❌
})
```

---

## Key Observations

### 1. Sequential Event Creation ✅
- Events created in a `for` loop (NOT goroutines)
- Each `Create()` call waits for completion before next iteration
- **NOT a parallel creation issue**

### 2. Schema Isolation ✅
- Each Ginkgo parallel process has its own schema (`test_process_N`)
- `audit_events` table copied to each process schema
- No cross-process interference

### 3. 100ms Sleep ⚠️
- Added as "RACE FIX" (line 201-207)
- Comment says: "ensure all transactions are committed before querying"
- **BUT**: If Create() is synchronous, why is this needed?
- **HYPOTHESIS**: `Create()` might return before transaction fully commits?

### 4. Advisory Lock in Place ✅
- `getPreviousEventHash()` uses `pg_advisory_xact_lock()` (line 256 in audit_events_repository.go)
- Should prevent concurrent hash calculation race conditions

---

## Verification Logic Analysis

**File**: `pkg/datastorage/repository/audit_export.go:235-344`

### Hash Chain Verification Process

```go
// 1. Read events ordered by: event_timestamp ASC, event_id ASC
// 2. Group by correlation_id
// 3. For each correlation_id group:
for i, event := range corrEvents {
    // Check 1: Previous hash match
    if event.PreviousEventHash != previousHash {
        // CHAIN BROKEN - mark as invalid
        exportEvent.HashChainValid = false
    } else {
        // Check 2: Calculate expected hash
        expectedHash, _ := calculateEventHashForVerification(previousHash, event)
        
        if event.EventHash != expectedHash {
            // TAMPERING DETECTED - mark as invalid
            exportEvent.HashChainValid = false
        } else {
            // VALID
            result.ValidChainEvents++
        }
    }
    
    // Update for next iteration
    previousHash = event.EventHash
}
```

---

## Critical Failure Pattern

**CI Result**: 0/5 events valid

This means **ALL 5 events fail hash verification**, which suggests:

### Hypothesis A: Systematic Hash Mismatch
- ❌ NOT likely a concurrency issue (would affect some events, not all)
- ✅ MORE likely a **consistent difference** in hash calculation

**Evidence**:
- Our unit test proved JSON is identical for zero values vs sql.NullString
- EventData normalization is correct
- Timezone conversion is correct

### Hypothesis B: Event Ordering Issue
- Events read from DB in wrong order?
- But query explicitly orders: `ORDER BY event_timestamp ASC, event_id ASC`

### Hypothesis C: First Event Previous Hash
- Line 311-320 in audit_export.go checks if first event (i==0) has empty previous_hash
- But this check happens AFTER the main verification
- If first event has non-empty previous_hash, chain is marked broken

---

## New Critical Discovery: Logging Shows What's Actually Happening

From `audit_export.go:284-306`, when hash chain is broken, it logs:

```go
r.logger.Info("Hash chain broken: previous_event_hash mismatch",
    "event_id", event.EventID,
    "correlation_id", correlationID,
    "expected_previous_hash", previousHash,
    "actual_previous_hash", event.PreviousEventHash)

// OR

r.logger.Info("Hash chain broken: event_hash mismatch (tampering detected)",
    "event_id", event.EventID,
    "correlation_id", correlationID,
    "expected_hash", expectedHash,
    "actual_hash", event.EventHash)
```

**ACTION REQUIRED**: Check must-gather logs for these specific log entries!

---

## Must-Gather Log Analysis

**File**: `/tmp/ci-must-gather-21317201476/kubernaut-must-gather/datastorage-integration-20260124-152403/datastorage_datastorage_test.log`

Let me search for:
1. "Hash chain broken" log entries
2. "expected_hash" vs "actual_hash" values
3. "expected_previous_hash" vs "actual_previous_hash" values

This will tell us **EXACTLY** why the verification is failing.

---

## Potential Root Causes (Ranked by Likelihood)

### 1. **EventTimestamp Precision** (HIGH) ⭐⭐⭐
- **Issue**: Go's `time.Time` has nanosecond precision
- **PostgreSQL**: `timestamptz` has microsecond precision (loses last 3 digits)
- **Impact**: When event is read back, timestamp is slightly different
- **JSON**: `"event_timestamp":"2026-01-24T10:00:00.123456789Z"` (creation)
  vs. `"event_timestamp":"2026-01-24T10:00:00.123456Z"` (verification)
- **Result**: Different JSON → Different Hash

**TEST THIS**: Our unit test used EXACT timestamp match. Need to test with DB round-trip.

### 2. **ParentEventDate Null Handling** (MEDIUM) ⭐⭐
- **Issue**: `ParentEventDate *time.Time` is nullable
- **Creation**: `nil` → marshals as `null`
- **Verification**: SQL NULL → `*time.Time` might be different representation
- **Impact**: JSON difference in parent_event_date field

### 3. **Event Ordering in Export** (LOW) ⭐
- **Issue**: Query orders by timestamp+event_id, but events created with same timestamp
- **Impact**: If timestamps are identical (millisecond precision in test), ordering is non-deterministic
- **Result**: Events read in wrong order, breaking chain

### 4. **Advisory Lock Scope** (LOW) ⭐
- **Issue**: Lock released too early, allowing concurrent reads
- **Impact**: Export reads events before all are committed
- **Likelihood**: Low (100ms sleep should mitigate)

---

## Action Plan

### **Phase 1: Get Actual Hash Values from CI** ✅

```bash
cd /tmp/ci-must-gather-21317201476
grep -r "Hash chain broken\|expected_hash\|actual_hash" . --include="*.log"
```

This will show us EXACTLY what's different.

### **Phase 2: Reproduce Locally with DB Round-Trip** ⏳

Create a new unit test that:
1. Creates an event in PostgreSQL
2. Reads it back
3. Compares the JSON marshaling before and after

```go
It("should produce identical JSON after DB round-trip", func() {
    // Create event
    event := &repository.AuditEvent{...}
    
    // Calculate hash BEFORE DB insert
    jsonBefore, _ := json.Marshal(event)
    hashBefore, _ := calculateEventHash("", event)
    
    // Insert to DB
    _, err := auditRepo.Create(ctx, event)
    
    // Read from DB
    retrieved := auditRepo.GetByID(ctx, event.EventID)
    
    // Calculate hash AFTER DB round-trip
    jsonAfter, _ := json.Marshal(retrieved)
    hashAfter, _ := calculateEventHashForVerification("", retrieved)
    
    // COMPARE
    Expect(string(jsonBefore)).To(Equal(string(jsonAfter)))
    Expect(hashBefore).To(Equal(hashAfter))
})
```

### **Phase 3: Fix Based on Evidence** ⏳

Once we know the exact field causing the difference, apply targeted fix.

---

## Hypothesis Testing Order

1. ✅ Check must-gather logs for actual hash mismatch details
2. ⏳ Test EventTimestamp precision with DB round-trip
3. ⏳ Test ParentEventDate null handling
4. ⏳ Test EventData with DB JSONB round-trip (even though unit test passed)

---

## Next Command

```bash
cd /tmp/ci-must-gather-21317201476 && \
find . -name "*.log" -type f | \
xargs grep -B 5 -A 5 "Hash chain broken\|expected_hash\|actual_hash" | \
head -100
```

---

**Status**: Waiting for must-gather log analysis to reveal exact hash mismatch cause.

**Confidence**: 80% that we'll find the root cause in the logs.
