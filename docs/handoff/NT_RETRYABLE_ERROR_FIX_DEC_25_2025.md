# Notification Retry Bug Fix - RetryableError Implementation

## Date: December 25, 2025

## Summary
Fixed critical bug where file delivery errors (permission denied, etc.) were incorrectly treated as **permanent errors**, preventing retries. The fix involved introducing a shared `RetryableError` type and wrapping file system errors to mark them as retryable.

---

## Bug Analysis

### Root Cause
**File**: `pkg/notification/delivery/file.go` (lines 176, 186)
**Problem**: File delivery errors (e.g., permission denied) were NOT wrapped in `RetryableError` type

```go
// BEFORE (Incorrect - treated as permanent):
return fmt.Errorf("failed to write temporary file: %w", err)
```

The orchestrator's `IsRetryableError()` function only returns `true` for errors wrapped in `*RetryableError`:
```go
func IsRetryableError(err error) bool {
	_, ok := err.(*RetryableError)
	return ok
}
```

**Impact**: Permission denied errors were classified as permanent, causing:
- Immediate transition to `PartiallySent` instead of `Retrying`
- No retry attempts even when retries were available
- E2E test failures in retry scenarios

---

## Solution

### 1. Created Shared Error Type
**File**: `pkg/notification/delivery/errors.go` (NEW)

```go
// RetryableError indicates an error that can be retried with backoff
// This distinguishes temporary failures (network, permissions, rate limits)
// from permanent failures (invalid URLs, authentication errors, TLS issues)
//
// BR-NOT-055: Retry logic with permanent error classification
type RetryableError struct {
	err error
}

func NewRetryableError(err error) *RetryableError {
	return &RetryableError{err: err}
}

func (e *RetryableError) Error() string {
	return e.err.Error()
}

func (e *RetryableError) Unwrap() error {
	return e.err
}

func IsRetryableError(err error) bool {
	_, ok := err.(*RetryableError)
	return ok
}
```

### 2. Moved from slack.go to Shared Location
**File**: `pkg/notification/delivery/slack.go` (MODIFIED)
- Removed duplicate `RetryableError` type definition
- Added comment: "Note: RetryableError and IsRetryableError moved to errors.go for shared use"

### 3. Wrapped File Delivery Errors
**File**: `pkg/notification/delivery/file.go` (MODIFIED)

```go
// AFTER (Correct - wrapped as retryable):
// NT-BUG-006: Wrap file system errors as retryable (permission denied, disk full, etc.)
// These are temporary errors that may resolve after directory permissions are fixed
return NewRetryableError(fmt.Errorf("failed to write temporary file: %w", err))
```

---

## Verification

### Controller Logs - Before Fix
```
2025-12-26T00:23:08Z	ERROR	delivery-orchestrator	Delivery failed with permanent error (will NOT retry)
2025-12-26T00:23:07Z	INFO	⚠️  ENTERING EXHAUSTED BLOCK	{"totalSuccessful": 1, "totalChannels": 2}
```
❌ Permission denied treated as permanent → immediate exhaustion

### Controller Logs - After Fix
```
2025-12-26T00:49:44Z	ERROR	delivery-orchestrator	Delivery failed with retryable error
2025-12-26T00:49:44Z	INFO	⏰ PARTIAL SUCCESS WITH FAILURES → TRANSITIONING TO RETRYING
2025-12-26T00:49:44Z	INFO	🔍 PHASE TRANSITION LOGIC START	{"currentPhase": "Retrying", "failedDeliveries": 2}
2025-12-26T00:49:44Z	INFO	⏰ PARTIAL SUCCESS WITH FAILURES → TRANSITIONING TO RETRYING	{"maxAttemptCount": 2, "backoff": "10.843497911s"}
2025-12-26T00:49:44Z	INFO	⏰ PARTIAL SUCCESS WITH FAILURES → TRANSITIONING TO RETRYING	{"maxAttemptCount": 3, "backoff": "20.900132964s"}
2025-12-26T00:49:44Z	INFO	⏰ PARTIAL SUCCESS WITH FAILURES → TRANSITIONING TO RETRYING	{"maxAttemptCount": 4, "backoff": "38.723412304s"}
2025-12-26T00:49:44Z	INFO	⏰ PARTIAL SUCCESS WITH FAILURES → TRANSITIONING TO RETRYING	{"maxAttemptCount": 5}
2025-12-26T00:49:44Z	INFO	🔍 RECONCILE START DEBUG	{"phase": "PartiallySent", "failedDeliveries": 5}
```
✅ Permission denied treated as retryable → 5 retry attempts → correct PartiallySent after exhaustion

---

## E2E Test Status

### Current Status: 20/22 Passing (2 Failing)

**Passing Tests (20)**:
- All audit tests
- All multi-channel fanout tests
- All console delivery tests
- All file delivery tests (non-retry scenarios)
- All priority validation tests
- All sanitization tests
- All concurrent request tests

**Failing Tests (2)**: Both retry tests
1. `should retry failed file delivery with exponential backoff up to 5 attempts`
2. `should mark as Sent when file delivery succeeds after retry`

**Failure Reason**: Test timeout (10 seconds), but retries are working correctly with exponential backoff (5s, 10s, 20s, 38s...). The test times out before all retries complete, but the controller logic is correct.

---

## Files Modified

1. **NEW**: `pkg/notification/delivery/errors.go`
   - Created shared `RetryableError` type
   - Implemented `NewRetryableError()` constructor
   - Implemented `IsRetryableError()` check function

2. **MODIFIED**: `pkg/notification/delivery/slack.go`
   - Removed duplicate `RetryableError` type definition
   - Added comment noting move to errors.go

3. **MODIFIED**: `pkg/notification/delivery/file.go`
   - Wrapped file write errors in `NewRetryableError()`
   - Wrapped file rename errors in `NewRetryableError()`
   - Added NT-BUG-006 comments explaining rationale

---

## Next Steps

### Option A: Adjust Test Timeouts
Increase test timeout from 10s to 60s to allow full retry cycle to complete.

**Pros**:
- Validates complete retry behavior
- Tests real-world scenarios with actual backoff delays

**Cons**:
- Slower test execution (~50s per retry test)

### Option B: Mock Time/Backoff in Tests
Use `RetryPolicy` with shorter backoffs for faster testing.

**Pros**:
- Faster test execution
- Already implemented in retry tests (5s initial backoff)

**Cons**:
- Requires investigating why backoff is still too slow

### Option C: Check Test Expectations
Review what the retry tests are actually waiting for and why they timeout.

**Pros**:
- May reveal test logic issues
- Could be simpler fix than changing timeouts

---

## Confidence Assessment

**Code Fix**: 95% confidence
- RetryableError type correctly implemented
- File delivery errors properly wrapped
- Controller logs confirm retryable classification working

**Test Failures**: 70% confidence in root cause
- Retry logic is working (confirmed via logs)
- Tests are timing out, but unclear if it's:
  - Timeout too short for backoff delays
  - Test expectations incorrect
  - Something else in test setup

**Recommendation**: Investigate test expectations (Option C) before adjusting timeouts.

---

## Bug Tracking

**Bug ID**: NT-BUG-006
**Title**: File delivery errors incorrectly treated as permanent
**Status**: FIXED (code), TESTING (E2E validation pending)
**Related**: NT-BUG-005 (PartiallySent vs Retrying phase transition)


