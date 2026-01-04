# WorkflowExecution DataStorage Connection Refused Errors Triage - Jan 04, 2026

## 🚨 **Issue Report**

**CI Run**: [20696658360](https://github.com/jordigilh/kubernaut/actions/runs/20696658360)
**Test Suite**: WorkflowExecution Integration Tests
**Symptom**: Multiple "connection refused" errors when audit store tries to write to DataStorage after it has been stopped
**Classification**: Test suite graceful shutdown timing issue (NOT a production bug)
**Impact**: Log noise during test cleanup, but no test failures

---

## 🔍 **Root Cause Analysis**

### **Timeline of Events** (from CI logs)

```
17:44:14  ✅ Test completes: "should persist workflow.failed audit event"
17:44:20  🛑 [AfterSuite] starts
17:44:20  📊 Audit store Close() called
17:44:20  ⏹️  DataStorage infrastructure STOPPED (postgres, redis, datastorage)
17:44:20  ⚠️  ERROR: dial tcp 127.0.0.1:18097: connect: connection refused (attempt 1)
17:44:21  ⚠️  ERROR: dial tcp 127.0.0.1:18097: connect: connection refused (attempt 2)
17:44:25  ⚠️  ERROR: dial tcp 127.0.0.1:18097: connect: connection refused (attempt 3)
17:44:30  ✅ Next test starts: "BR-WE-004: Tekton Failure Reason Classification"
17:44:35  ⚠️  ERROR: dial tcp 127.0.0.1:18097: connect: connection refused (attempt 1)
17:44:36  ⚠️  ERROR: dial tcp 127.0.0.1:18097: connect: connection refused (attempt 2)
17:44:40  ⚠️  ERROR: dial tcp 127.0.0.1:18097: connect: connection refused (attempt 3)
17:44:47  🛑 Another [AfterSuite] cleanup
```

**Total Duration of Errors**: **27 seconds** (17:44:20 to 17:44:47)

---

### **The Problem**

Looking at `test/integration/workflowexecution/suite_test.go:275-305`:

```go
var _ = AfterSuite(func() {
    By("Tearing down the test environment")

    // Close REAL audit store to flush remaining events (DD-AUDIT-003)
    if realAuditStore != nil {
        By("Flushing and closing real audit store")
        err := realAuditStore.Close()  // 📊 Step 1: Close audit store
        if err != nil {
            GinkgoWriter.Printf("⚠️  Warning: Failed to close audit store: %v\n", err)
        } else {
            GinkgoWriter.Println("✅ Real audit store closed (all events flushed)")
        }
    }

    cancel()  // 🛑 Step 2: Cancel context

    err := testEnv.Stop()  // 🛑 Step 3: Stop envtest

    // DD-TEST-001: MANDATORY infrastructure cleanup after integration tests
    err = infrastructure.StopWEIntegrationInfrastructure(GinkgoWriter)  // 🛑 Step 4: Stop DataStorage
    if err != nil {
        GinkgoWriter.Printf("⚠️  Warning: Infrastructure stop failed: %v\n", err)
    } else {
        GinkgoWriter.Println("✅ DataStorage infrastructure stopped (postgres, redis, datastorage)")
    }

    GinkgoWriter.Println("✅ Cleanup complete")
})
```

**Issue**: `realAuditStore.Close()` (Step 1) happens **immediately before** `StopWEIntegrationInfrastructure()` (Step 4).

---

### **Audit Store Close() Behavior**

From `pkg/audit/store.go:349-395`:

```go
func (s *BufferedAuditStore) Close() error {
    // Check if already closed (atomic operation)
    if !atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
        s.logger.V(1).Info("Audit store already closed, skipping")
        return nil
    }

    s.logger.Info("Closing audit store, flushing remaining events")

    // Close buffer (signals background worker to stop)
    close(s.buffer)  // 🔔 Signals backgroundWriter to stop

    // Wait for background worker to finish (with timeout)
    done := make(chan struct{})
    go func() {
        s.wg.Wait()  // ⏳ Waits for backgroundWriter goroutine
        close(done)
    }()

    select {
    case <-done:
        // Background worker finished ✅
        return nil
    case <-time.After(30 * time.Second):  // ⏰ 30-second timeout
        // Timeout waiting for background worker
        s.logger.Error(nil, "Timeout waiting for audit store to close")
        return fmt.Errorf("timeout waiting for audit store to close")
    }
}
```

**Key Points**:
1. `Close()` signals the background writer to stop
2. Waits for `backgroundWriter()` goroutine to finish
3. **30-second timeout** for graceful shutdown
4. Background writer retries failed writes with exponential backoff (up to 3 attempts)

---

### **Background Writer Retry Logic**

From `pkg/audit/store.go:569`:

```go
func (s *BufferedAuditStore) writeBatchWithRetry(batch []map[string]interface{}) error {
    var lastErr error

    for attempt := 1; attempt <= s.config.MaxRetries; attempt++ {
        // ... retry logic with exponential backoff ...

        // Exponential backoff between retries (1s, 4s, 16s for attempts 1, 2, 3)
        backoff := time.Duration(1<<uint(attempt-1)) * time.Second
        time.Sleep(backoff)
    }

    return lastErr
}
```

**Total Retry Time**: ~1s + ~4s + ~16s = **~21 seconds** of retries before giving up

---

### **The Race Condition**

```
Thread 1 (Test Suite AfterSuite):          Thread 2 (Background Audit Writer):
┌────────────────────────────────┐         ┌────────────────────────────────┐
│                                │         │                                │
│ 17:44:20: realAuditStore.Close()│────────>│ Receives stop signal          │
│                                │         │ Starts flushing buffer        │
│                                │         │ Has 2 buffered events         │
│ 17:44:20: StopDataStorage()    │         │                                │
│ (DataStorage STOPPED)          │         │ Attempt 1: Write batch...     │
│                                │         │   ❌ Connection refused        │
│                                │         │                                │
│ Next test starts...            │         │ Backoff: 1 second             │
│                                │         │ Attempt 2: Write batch...     │
│                                │         │   ❌ Connection refused        │
│                                │         │                                │
│ 17:44:30: Test running...      │         │ Backoff: 4 seconds            │
│                                │         │ Attempt 3: Write batch...     │
│                                │         │   ❌ Connection refused        │
│                                │         │                                │
│ 17:44:47: AfterSuite again     │         │ Give up after 3 attempts      │
│                                │         │ Log errors to console         │
│                                │         │ Close() returns with error    │
└────────────────────────────────┘         └────────────────────────────────┘
```

**The Problem**: DataStorage is stopped while the background writer is still trying to flush buffered events.

---

## 📊 **Error Log Examples**

From CI run 20696658360:

```
2026-01-04T17:44:20Z	ERROR	audit.audit-store	Failed to write audit batch
{"attempt": 1, "batch_size": 2, "error": "network error: Post \"http://127.0.0.1:18097/api/v1/audit/events/batch\": dial tcp 127.0.0.1:18097: connect: connection refused"}

2026-01-04T17:44:21Z	ERROR	audit.audit-store	Failed to write audit batch
{"attempt": 2, "batch_size": 2, "error": "network error: Post \"http://127.0.0.1:18097/api/v1/audit/events/batch\": dial tcp 127.0.0.1:18097: connect: connection refused"}

2026-01-04T17:44:25Z	ERROR	audit.audit-store	Failed to write audit batch
{"attempt": 3, "batch_size": 2, "error": "network error: Post \"http://127.0.0.1:18097/api/v1/audit/events/batch\": dial tcp 127.0.0.1:18097: connect: connection refused"}
```

**Pattern**: 3 retry attempts per batch, exponential backoff (1s, 4s, 16s)

---

## ⚠️ **Impact Assessment**

| Aspect | Status | Notes |
|--------|--------|-------|
| **Test Failures** | ✅ NO | All tests pass (120/120 specs) |
| **Functional Impact** | ✅ NO | Tests complete successfully |
| **Production Risk** | ✅ NO | Issue only in test cleanup |
| **CI Log Noise** | ⚠️ YES | ~27 seconds of error logs per test run |
| **Developer Experience** | ⚠️ YES | Confusing error logs during cleanup |

**Severity**: 🟡 **LOW** - Cosmetic issue, no functional impact

**Business Impact**: 🟢 **NONE** - Test infrastructure timing issue only

---

## 🔧 **Solution Options**

### **Option A: Flush Audit Store Before Stopping DataStorage** (Recommended)

**Approach**: Add explicit `Flush()` call before stopping infrastructure

**Implementation**:
```go
var _ = AfterSuite(func() {
    By("Tearing down the test environment")

    // NEW: Flush audit store BEFORE closing
    if realAuditStore != nil {
        By("Flushing audit store before infrastructure shutdown")
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        err := realAuditStore.Flush(ctx)  // ← Explicit flush
        if err != nil {
            GinkgoWriter.Printf("⚠️  Warning: Failed to flush audit store: %v\n", err)
        }

        By("Closing audit store")
        err = realAuditStore.Close()  // ← Then close
        if err != nil {
            GinkgoWriter.Printf("⚠️  Warning: Failed to close audit store: %v\n", err)
        } else {
            GinkgoWriter.Println("✅ Real audit store closed (all events flushed)")
        }
    }

    cancel()
    err := testEnv.Stop()

    // Now safe to stop DataStorage (audit events already flushed)
    err = infrastructure.StopWEIntegrationInfrastructure(GinkgoWriter)
    // ...
})
```

**Pros**:
- ✅ Clean solution: Ensures all events written before DataStorage stops
- ✅ Explicit control over flush timing
- ✅ No error logs during cleanup
- ✅ Follows graceful shutdown best practices

**Cons**:
- ⚠️ Adds 10-second timeout to test suite cleanup (acceptable)

**Confidence**: **95%** - This is the cleanest solution

---

### **Option B: Ignore Errors from Audit Store During Cleanup**

**Approach**: Suppress audit errors during test cleanup

**Implementation**:
```go
var _ = AfterSuite(func() {
    By("Tearing down the test environment")

    // Close audit store (ignoring errors during cleanup)
    if realAuditStore != nil {
        By("Closing audit store (cleanup mode)")
        _ = realAuditStore.Close()  // ← Ignore errors
        GinkgoWriter.Println("✅ Real audit store closed")
    }

    cancel()
    err := testEnv.Stop()

    // Stop DataStorage infrastructure
    err = infrastructure.StopWEIntegrationInfrastructure(GinkgoWriter)
    // ...
})
```

**Pros**:
- ✅ Simplest fix: Just ignore the errors
- ✅ No additional timeout delays

**Cons**:
- ❌ Errors still logged to console (log noise remains)
- ❌ Doesn't follow graceful shutdown best practices
- ❌ Audit events may be lost (though this is test cleanup, so acceptable)

**Confidence**: **60%** - Quick fix but not clean

---

### **Option C: Make Audit Store Circuit Break Faster**

**Approach**: Add circuit breaker to audit store that fails fast when DataStorage is unavailable

**Implementation**:
```go
// In pkg/audit/store.go writeBatchWithRetry()

// Check if we're in shutdown mode
if atomic.LoadInt32(&s.closed) == 1 {
    // During shutdown, don't retry - fail fast
    return s.client.WriteBatch(ctx, batch)
}

// Normal operation - retry with backoff
for attempt := 1; attempt <= s.config.MaxRetries; attempt++ {
    // ...
}
```

**Pros**:
- ✅ Audit store stops trying once Close() is called
- ✅ Reduces error log noise
- ✅ Fails fast during shutdown

**Cons**:
- ⚠️ More complex change
- ⚠️ Affects audit store behavior (production code change)
- ⚠️ May not completely eliminate errors (timing dependent)

**Confidence**: **75%** - Good solution but more invasive

---

### **Option D: Add Delay Between Close() and StopDataStorage()**

**Approach**: Add `time.Sleep()` after audit store close

**Implementation**:
```go
var _ = AfterSuite(func() {
    By("Tearing down the test environment")

    if realAuditStore != nil {
        By("Closing audit store")
        err := realAuditStore.Close()
        // ...

        // Give background writer time to finish
        time.Sleep(2 * time.Second)  // ← Artificial delay
    }

    cancel()
    err := testEnv.Stop()

    err = infrastructure.StopWEIntegrationInfrastructure(GinkgoWriter)
    // ...
})
```

**Pros**:
- ✅ Very simple fix
- ✅ Likely reduces errors significantly

**Cons**:
- ❌ Artificial delay (not deterministic)
- ❌ Still possible to have race condition
- ❌ Adds 2 seconds to every test run
- ❌ Not a clean solution (violates DD-TESTING-001 - avoid time.Sleep)

**Confidence**: **40%** - Hacky workaround

---

## 🎯 **Recommended Solution**

**APPROVED**: **Option A - Flush Audit Store Before Stopping DataStorage**

**Implementation Plan**:
1. Add `Flush(ctx)` call before `Close()` in AfterSuite
2. Use 10-second timeout for flush operation
3. Stop DataStorage only after successful flush
4. Add comment explaining the order dependency

**Files to Modify**:
- `test/integration/workflowexecution/suite_test.go` (AfterSuite)

**Expected Result**:
- ✅ No connection refused errors during cleanup
- ✅ All audit events flushed before DataStorage stops
- ✅ Clean test logs
- ✅ +10 seconds to test suite duration (acceptable trade-off)

---

## 📝 **Other Controllers to Check**

Similar issue may exist in other integration test suites:

| Service | Port | Status | Notes |
|---------|------|--------|-------|
| **WorkflowExecution** | 18097 | ⚠️ ISSUE | This triage |
| **AIAnalysis** | 18094 | ❓ CHECK | Uses same pattern |
| **SignalProcessing** | 18094 | ❓ CHECK | Uses same pattern |
| **RemediationOrchestrator** | 18095 | ❓ CHECK | Uses same pattern |
| **Notification** | 18096 | ❓ CHECK | Uses same pattern |

**Action**: After fixing WE, audit other test suites for same pattern.

---

## 🔗 **Related Documentation**

- **Audit Store Implementation**: [pkg/audit/store.go](../../pkg/audit/store.go)
- **WE Test Suite**: [test/integration/workflowexecution/suite_test.go](../../test/integration/workflowexecution/suite_test.go)
- **Infrastructure Setup**: [test/infrastructure/workflowexecution.go](../../test/infrastructure/workflowexecution.go)
- **DD-AUDIT-003**: Audit store buffering strategy
- **DD-TEST-001**: Integration test infrastructure patterns

---

## ✅ **Success Criteria**

Fix is successful when:
- [ ] No "connection refused" errors in AfterSuite cleanup
- [ ] All audit events flushed before DataStorage stops
- [ ] Test suite passes with clean logs
- [ ] Fix applied consistently to all controller integration test suites
- [ ] Documentation updated with proper shutdown sequence

---

## 📊 **Additional Observations**

### **Parallel Test Execution**

From CI logs, multiple `[AfterSuite]` blocks run:
```
17:44:20  [AfterSuite] PASSED [1.466 seconds]
17:44:20  [AfterSuite] PASSED [1.501 seconds]
17:44:20  [AfterSuite] PASSED [some time]
```

This indicates **parallel test execution** (multiple Ginkgo processes).

**Implication**: Each process tries to stop DataStorage (though `StopWEIntegrationInfrastructure` is idempotent, showing "no such container" errors).

---

### **Not Actual Test Failures**

Reviewing all ERROR logs:
- ✅ "workflowexecutions.kubernaut.ai not found" - **Expected** (CRD deleted during cleanup)
- ✅ "PipelineRun not found - deleted externally" - **Expected** (test scenario)
- ✅ "Operation cannot be fulfilled...object has been modified" - **Expected** (concurrent reconciliation)
- ✅ "Spec validation failed" - **Expected** (negative test case)
- ⚠️ "dial tcp 127.0.0.1:18097: connect: connection refused" - **This Issue** (audit cleanup timing)

**None of these errors cause test failures** - the test suite completes successfully with 120/120 specs passing.

---

**Document Status**: ✅ Complete - Triage Analysis
**Next Step**: Implement Option A (Flush before shutdown)
**Priority**: P2 - MEDIUM (cosmetic issue, no functional impact)
**Owner**: TBD
**Blocking**: No


