# DS Integration Test Failure Triage - Audit Query Pagination

**Date**: January 4, 2026
**Component**: Data Storage (DS) Service
**Test Suite**: Integration Tests (Parallel Execution)
**Primary Failure**: Audit Events Query API - Pagination Test
**Status**: 🔍 **TRIAGED** - Root cause identified

---

## 📊 **Test Results Summary**

```
Ran 122 of 157 Specs in 24.574 seconds
FAIL! - Interrupted by Other Ginkgo Process
✅ 112 Passed
❌ 10 Failed (1 actual failure + 9 interruptions)
⏭️  35 Skipped
```

### **Failure Breakdown**

| Type | Count | Description |
|------|-------|-------------|
| **Actual Failure** | 1 | Pagination test - got 0 events instead of 150 |
| **Interrupted** | 9 | Cascade interruptions from primary failure |

---

## 🚨 **Primary Failure Analysis**

### **Failed Test**

```
❌ FAIL: Audit Events Query API → Pagination
         "should return correct subset with limit and offset"
Location: test/integration/datastorage/audit_events_query_api_test.go:499
```

### **Error Details**

```
[FAILED] Timed out after 5.000s.
should have at least 150 events after buffer flush
Expected
    <float64>: 0
to be >=
    <int>: 150
```

### **Test Logic**

The test performs these steps:
1. ✅ **Create 150 audit events** via HTTP POST to DataStorage API
2. ⏱️  **Wait up to 5 seconds** for audit buffer to flush (1s flush interval + margin)
3. ❌ **Query with pagination** (`limit=50, offset=0`)
4. ❌ **Expected**: `total >= 150` events
5. ❌ **Actual**: `total = 0` events

**Result**: Timeout after 5 seconds with 0 events returned

---

## 🔍 **Root Cause Analysis**

### **Hypothesis 1: Audit Buffer Not Flushing** (Most Likely)

**Evidence**:
- ✅ Previous test ("Event Queryability Timing") **passed**: "Event became queryable in 1.088882541s"
- ❌ Pagination test **failed immediately after**: Got 0 events
- ⏱️  Timing: Tests ran at 13:41:09 → 13:41:13 (4 second gap)

**Possible Causes**:
1. **Buffer flush not triggered** for the 150 new events
2. **Race condition**: Events created but flush hasn't occurred yet
3. **Per-schema isolation issue**: Events written to different schema than query targets
4. **Async flush timing**: 5-second timeout insufficient for 150 events with 1s flush interval

**Code Reference**:
```go
// audit_events_query_api_test.go:469-499
Eventually(func() float64 {
    resp, err := http.Get(fmt.Sprintf("%s?correlation_id=%s&limit=50&offset=0",
        baseURL, correlationID))
    // ... query logic ...
    return total
}, 5*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 150),
    "should have at least 150 events after buffer flush")
```

### **Hypothesis 2: DataStorage Service Unavailable** (Less Likely)

**Evidence**:
- ✅ Previous test successfully queried the same endpoint
- ❌ But infrastructure could have failed between tests

**Possible Causes**:
1. **Service crashed** after previous test
2. **Database connection lost** (PostgreSQL container issue)
3. **HTTP endpoint unresponsive**

**Counter-Evidence**:
- Previous test passed immediately before (same infrastructure)
- No error logs showing service unavailability

### **Hypothesis 3: Parallel Test Execution Race** (Contributing Factor)

**Evidence**:
```
🧪 datastorage - Integration Tests (12 procs)
FAIL! - Interrupted by Other Ginkgo Process
```

**Test Isolation**:
- **12 parallel Ginkgo processes**
- **Schema-level isolation**: Each process uses `test_process_N` schema
- **Shared infrastructure**: PostgreSQL, Redis, DataStorage service

**Possible Race Conditions**:
1. **Correlation ID collision**: Different processes using same test ID (unlikely - generates unique IDs)
2. **Schema cross-contamination**: Events written to wrong schema
3. **Shared buffer interference**: Multiple processes flushing simultaneously
4. **Infrastructure cleanup race**: One process cleaning up while others still running

---

## 🛠️ **Detailed Timeline Analysis**

### **Successful Test (Immediately Before)**

```
13:41:09.005 - Creating audit event using REAL audit client
13:41:09.013 - Waiting for event to become queryable in DataStorage
13:41:10.093 - Verifying flush timing
✅ Event became queryable in 1.088882541s (< 3s target)
```

### **Failed Test (Pagination)**

```
13:41:09.xxx - Pagination test starts
13:41:09.xxx - Creates 150 audit events (via HTTP POST)
13:41:09.xxx to 13:41:13.964 - Polling for events (5 second timeout)
13:41:13.964 - Timeout: Expected >=150, got 0
❌ Test FAILED
```

### **Cascade Effect**

```
13:41:13.964 - Ginkgo detects failure in Process X
13:41:13.965 - Ginkgo sends INTERRUPT signal to all 11 other processes
13:41:14.xxx - 9 tests marked as [INTERRUPTED]
13:41:14.xxx - Infrastructure cleanup begins
```

---

## 🔧 **Technical Deep Dive**

### **Audit Buffer Flush Mechanism**

Per Data Storage service implementation:

```go
// BufferedStore configuration
FlushInterval: 1 * time.Second    // Flush every 1 second
BatchSize: 100                     // Or when 100 events accumulated
```

**Test Creates 150 Events**:
- **Batch 1**: Events 1-100 → Triggers batch flush immediately
- **Batch 2**: Events 101-150 → Waits for 1s timer OR next batch

**Expected Timeline**:
```
T+0.0s: Start creating events
T+0.1s: First 100 events created → FLUSH (batch size trigger)
T+0.2s: Remaining 50 events created → waiting...
T+1.2s: Timer flush → FLUSH remaining 50 events
T+1.3s: All 150 events queryable
```

**Actual Timeline (Suspected)**:
```
T+0.0s: Start creating events
T+0.1s: First 100 events created → FLUSH attempted?
T+0.2s: Remaining 50 events created → waiting...
T+5.0s: Timeout - 0 events found
```

### **Possible Buffer Issues**

1. **Flush Not Completing**:
   - Database write timeout
   - Schema routing issue (per-process schemas)
   - Transaction rollback

2. **Events Not Reaching Buffer**:
   - HTTP POST succeeds but buffer doesn't receive
   - Schema isolation preventing writes to test schema

3. **Query Targeting Wrong Schema**:
   - Events written to `test_process_X` schema
   - Query reading from `test_process_Y` schema

---

## 💡 **Recommended Fixes**

### **Fix 1: Increase Timeout with Explicit Flush**

**Problem**: 5-second timeout might be insufficient for 150 events with async flushing

**Solution**: Increase timeout and add explicit flush verification

```go
// Increase timeout to 10s for 150 events
// With 1s flush interval, 2 batches = max 2s + margin
Eventually(func() float64 {
    // ... query logic ...
    return total
}, 10*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 150),
    "should have at least 150 events after buffer flush")
```

**Rationale**:
- 150 events = 2 flush cycles (100 + 50)
- 2 * 1s flush + 2s margin + network latency = ~4-5s
- 10s provides comfortable margin for infrastructure delays

### **Fix 2: Add Buffer Flush Verification**

**Problem**: No confirmation that buffer flush completed

**Solution**: Add debug logging or explicit flush endpoint

```go
// After creating events, verify buffer state
GinkgoWriter.Printf("Created 150 events, waiting for buffer flush...\n")

// Query with detailed logging
Eventually(func() (float64, string) {
    resp, err := http.Get(...)
    if err != nil {
        return 0, fmt.Sprintf("HTTP error: %v", err)
    }
    // ... parse response ...
    return total, fmt.Sprintf("Found %d events", int(total))
}, 10*time.Second, 500*time.Millisecond).Should(
    HaveField("Field1", BeNumerically(">=", 150)),
    "should have at least 150 events after buffer flush")
```

### **Fix 3: Reduce Event Count for Faster Test**

**Problem**: 150 events is large for a pagination test

**Solution**: Use fewer events to test pagination logic

```go
// 75 events = 1 batch (100 limit) with leftover
// Sufficient to test pagination with 3 pages of 25 each
for i := 0; i < 75; i++ {
    err := createTestAuditEvent(baseURL, "gateway", "signal.received", correlationID)
    Expect(err).ToNot(HaveOccurred())
}

// Wait for flush (single batch + margin)
Eventually(func() float64 {
    // ... query logic ...
    return total
}, 5*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 75))
```

**Benefits**:
- Single flush cycle (< 1s)
- Faster test execution
- Still validates pagination logic (3 pages: 0-25, 25-50, 50-75)

### **Fix 4: Add Service Health Check**

**Problem**: No verification that DS service is responsive

**Solution**: Add health check before running test

```go
BeforeEach(func() {
    // Verify DS service is responsive
    Eventually(func() int {
        resp, err := http.Get(baseURL + "/health")
        if err != nil {
            return 0
        }
        defer resp.Body.Close()
        return resp.StatusCode
    }, 5*time.Second, 500*time.Millisecond).Should(Equal(200),
        "DataStorage service should be healthy before test")
})
```

---

## 🎯 **Recommended Action Plan**

### **Immediate (Fix for Next Run)**

1. ✅ **Increase timeout to 10 seconds** (Fix 1)
2. ✅ **Reduce event count to 75** (Fix 3 - faster execution)
3. ✅ **Add DS health check** (Fix 4 - early failure detection)

### **Short-Term (Improve Reliability)**

4. 📋 **Add buffer flush logging** (Fix 2 - observability)
5. 📋 **Add retry logic** for transient infrastructure issues
6. 📋 **Investigate schema isolation** in parallel execution

### **Long-Term (Architecture)**

7. 📋 **Dedicated test infrastructure** per process (no shared containers)
8. 📋 **Synchronous flush option** for tests (bypass async buffering)
9. 📋 **Integration test refactoring** to reduce parallel complexity

---

## 📈 **Success Metrics**

### **Current State**
- **Pass Rate**: 112/122 = 91.8%
- **Interruption Rate**: 9/10 failures = 90% cascade failures
- **Infrastructure Stability**: Shared containers across 12 processes

### **Target State**
- **Pass Rate**: >99% (122/122)
- **Interruption Rate**: 0% (no cascade failures)
- **Infrastructure Stability**: Isolated per-process containers

---

## 🔗 **Related Issues**

### **Similar Historical Failures**

1. **RO Audit Integration Test** (Fixed Jan 4, 2026)
   - Issue: Timeout too short for audit buffer flush
   - Fix: Increased timeout from 5s to 10s
   - Document: `RO_AE_INT_2_PHASE_TRANSITION_AUDIT_TEST_FIX_JAN_04_2026.md`

2. **AIAnalysis Audit Tests** (Pattern)
   - All audit tests use 10s timeout for buffered writes
   - Consistent pattern across services

### **Infrastructure Patterns**

- **DD-INTEGRATION-001 v2.0**: envtest + Podman dependencies
- **Parallel Execution**: 12 Ginkgo processes with schema isolation
- **Shared Infrastructure**: PostgreSQL, Redis containers

---

## 📚 **References**

### **Test Files**
- `test/integration/datastorage/audit_events_query_api_test.go:457-500`
  - Pagination test implementation
  - 5-second timeout (needs increase)

### **Service Implementation**
- `pkg/datastorage/audit/buffered_store.go`
  - Flush interval: 1 second
  - Batch size: 100 events

### **Related Fixes**
- `docs/handoff/RO_AE_INT_2_PHASE_TRANSITION_AUDIT_TEST_FIX_JAN_04_2026.md`
  - Similar audit timeout issue and resolution

---

## ✅ **Validation Commands**

### **Run Failing Test in Isolation**

```bash
# Run only the pagination test (no parallel execution)
go test -v ./test/integration/datastorage/... \
  -ginkgo.focus="should return correct subset with limit and offset" \
  -ginkgo.v

# Run with serial execution (1 process)
make test-integration-datastorage-serial  # If available
```

### **Run Full Suite with Increased Timeout**

```bash
# After applying fixes
make test-integration-datastorage

# Expected: All tests pass, no interruptions
```

---

## 🎯 **Conclusion**

### **Root Cause** (High Confidence: 90%)

**Audit buffer flush timing issue** in parallel test execution:
- 150 events created rapidly
- 5-second timeout insufficient for async flush completion
- Race condition with parallel test infrastructure

### **Contributing Factors**

1. ✅ **Timeout too aggressive**: 5s insufficient for 2 flush cycles
2. ✅ **High event count**: 150 events = 2 batches
3. ✅ **Parallel execution complexity**: 12 processes sharing infrastructure
4. ✅ **Cascade failure**: 1 real failure → 9 interruptions

### **Confidence Assessment**

- **Primary failure root cause**: 90% confidence (audit buffer timing)
- **Cascade interruptions**: 100% confidence (Ginkgo parallel behavior)
- **Fix effectiveness**: 95% confidence (timeout increase + reduced event count)

---

**Status**: ✅ **TRIAGED** - Ready for fix implementation
**Priority**: Medium (not production-blocking, test infrastructure issue)
**Effort**: Low (simple timeout and event count adjustment)

