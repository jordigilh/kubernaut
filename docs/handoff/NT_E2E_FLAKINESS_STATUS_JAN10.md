# Notification E2E Flakiness Status

**Status**: 🔴 **PERSISTENT FLAKINESS** (2026-01-10)
**Last Updated**: 2026-01-10
**Confidence**: 95%

## Current Situation

### Test Results Over Multiple Runs

| Run | Timeout | Pass Rate | Status |
|---|---|---|---|
| Run 1 | 15s | 19/19 (100%) | ✅ PASS (initial success) |
| Run 2 | 15s | 16/19 (84%) | ❌ FLAKY |
| Run 3 | 20s | 16/19 (84%) | ❌ FLAKY |
| Run 4 | 20s | 15/19 (79%) | ❌ FLAKY (worse!) |

**Conclusion**: Pass rate is **DECREASING** despite increased timeouts, indicating a fundamental issue beyond simple filesystem sync latency.

### Consistently Failing Tests

1. **Test 03: Priority Field Validation** - File not found or data corruption
2. **Test 07-1: Critical Priority File Audit** - "Notification type must be preserved" error
3. **Test 07-2: Multiple Priorities Ordering** - File not found or data corruption
4. **Test 06-1: Multi-Channel Fanout** (intermittent) - Timeout

## Root Cause Hypothesis

### ❌ NOT a Simple Timeout Issue

**Evidence**:
- Increasing timeout from 15s → 20s made pass rate **worse** (84% → 79%)
- Tests that previously passed at 15s are now failing at 20s
- Different tests fail on different runs (non-deterministic)

### ✅ Likely Causes

1. **Race Condition in File Writes**: Multiple parallel tests writing to same `/tmp/notifications/` directory may be causing file collisions or corruption
2. **Notification Type Serialization Issue**: "Notification type must be preserved" suggests JSON marshaling/unmarshaling corruption
3. **Podman VM Instability**: Multiple parallel `kubectl exec` operations may be overwhelming the Podman VM layer
4. **Test Cleanup Issues**: Files from previous tests may not be cleaned up, causing assertions on wrong files

## Attempted Solutions

| Solution | Result | Notes |
|---|---|---|
| Remove fixed delay, use adaptive polling | ❌ Didn't help | Still flaky |
| Increase timeout 5s → 15s | 🟡 Partial | 84% pass rate |
| Increase timeout 15s → 20s | ❌ Made worse | 79% pass rate |
| Serial execution (--procs=1) | ✅ 100% pass | User rejected (not realistic) |

## Recommendation

Given the persistent flakiness and decreasing reliability, I recommend:

### Option A: Run Specific File Tests Serially (Surgical Approach)

**Approach**: Use Ginkgo's `Serial` decorator for only the problematic file validation tests

```go
var _ = Describe("File-Based Notification Delivery E2E Tests", Serial, func() {
    // These tests run serially due to shared /tmp/notifications/ directory
    // BR-NOT-001: File delivery with virtiofs sync under Podman
})
```

**Pros**:
- ✅ Maintains parallel execution for most tests (16/19 tests)
- ✅ Only slows down the 3 problematic file tests
- ✅ Realistic for tests that don't share file resources

**Cons**:
- ⚠️ File tests run slower (~3x)
- ⚠️ Doesn't solve root cause (masks the problem)

### Option B: Isolate File Output Per Test (Best Practice)

**Approach**: Each test writes to a unique subdirectory: `/tmp/notifications/{test-name}/`

```go
testOutputDir := filepath.Join("/tmp/notifications", GinkgoT().Name())
// Configure controller to write to testOutputDir for this test's notifications
```

**Pros**:
- ✅ Eliminates file collisions between parallel tests
- ✅ Maintains full parallelism
- ✅ Solves root cause (proper test isolation)

**Cons**:
- ⚠️ Requires controller configuration changes (per-test output dir)
- ⚠️ More complex test setup

### Option C: Move File Validation to Integration Tests (Pragmatic)

**Approach**: E2E tests verify controller behavior, integration tests verify file content

**Pros**:
- ✅ Integration tests can use mocks (more reliable)
- ✅ E2E focuses on Kubernetes interactions only
- ✅ Faster test execution

**Cons**:
- ⚠️ Less end-to-end coverage for file delivery
- ⚠️ Doesn't test actual file writes to hostPath volumes

## Immediate Next Steps

1. **Document current flakiness** ✅ (this document)
2. **Move to Integration Tests** (90.7% pass rate, 12 failures to investigate)
3. **Return to E2E after integration tests are stable**
4. **Implement Option B (test isolation)** if E2E file tests remain critical

## Integration Test Priority

The NT integration tests have:
- ✅ **90.7% pass rate** (117/129 passing)
- ✅ **Build issues already fixed**
- ✅ **Only 12 runtime failures** to investigate
- ✅ **More deterministic** (no virtiofs/Podman issues)

**Recommendation**: Focus on integration tests first, return to E2E flakiness with fresh perspective.

## Related Documents

- [NT_E2E_PARALLEL_SUCCESS_FINAL_JAN10.md](./NT_E2E_PARALLEL_SUCCESS_FINAL_JAN10.md) - Initial "success" (actually flaky)
- [NT_E2E_PARALLEL_EXECUTION_ISSUE_JAN10.md](./NT_E2E_PARALLEL_EXECUTION_ISSUE_JAN10.md) - Root cause analysis

**Authority**: DD-NOT-006 v5, BR-NOTIFICATION-001
**Status**: IN PROGRESS - Recommend shifting to integration tests
