# Notification E2E Parallel Execution Issue

**Status**: 🟡 **IN PROGRESS** (2026-01-10)
**Last Updated**: 2026-01-10
**Confidence**: 95%

## Context & Problem

Notification E2E tests exhibit different behavior when run individually vs. in parallel:

| Execution Mode | Result | Evidence |
|---|---|---|
| **Focused run (3 tests)** | ✅ **3/3 PASSING** | All file validation tests pass |
| **Full suite (19 tests, 12 parallel processes)** | ❌ **15/19 PASSING** | 4 file validation tests fail (same ones that passed in focused run) |

**Key Requirements**:
- File validation tests must check for files written by the Notification controller
- Tests must work reliably in both CI and local development environments
- Tests should complete in reasonable time (<10 minutes for full suite)

## Root Cause Analysis

**Primary Issue**: `virtiofs` filesystem sync latency on macOS + Podman under concurrent I/O load

**Evidence**:
1. **Controller logs show successful writes**: All files are written successfully to `/tmp/notifications/` inside the pod
   ```
   2026-01-10T19:18:14Z INFO Notification delivered successfully to file {"filePath": "/tmp/notifications/notification-e2e-multi-channel-fanout-20260110-191814.485730.json", "filesize": 1965}
   ```

2. **Tests see files in focused run**: When running 3 tests (low concurrency), `kubectl exec ls /tmp/notifications/` finds files immediately

3. **Tests don't see files in parallel run**: When running 19 tests (12 parallel processes), `kubectl exec ls /tmp/notifications/` frequently finds 0 files, even after waiting

4. **virtiofs sync latency increases under load**:
   - Light load (3 tests): 1-2s sync time
   - High load (12 parallel processes): 3-4s+ sync time
   - Concurrent I/O contention in Podman VM layer exacerbates the issue

5. **Infrastructure flakiness**: Frequent transient failures during BeforeSuite:
   - AuthWebhook image load failures (`failed to load image to Kind: exit status 1`)
   - DataStorage image build failures (`failed to build E2E image: exit status 125`)
   - These are unrelated to the virtiofs issue but add noise to debugging

## Attempted Solutions

| Solution | Status | Outcome |
|---|---|---|
| **1. Remove `virtiofsSyncDelay` entirely** | ❌ **FAILED** | Tests failed even faster (no time for filesystem sync) |
| **2. Add 1s delay per poll** | ❌ **FAILED** | Limited Eventually retries to ~2-3 attempts in 5s timeout |
| **3. Add one-time 2s initial delay** | 🟡 **PARTIAL** | Focused run passes, full suite fails (insufficient for high concurrency) |
| **4. Increase initial delay to 4s** | 🟡 **TESTING** | Not yet verified due to infrastructure failures |
| **5. Run tests serially (--procs=1)** | 🟢 **PROPOSED** | Eliminates concurrent I/O contention, increases runtime by ~2-3x |

## Decision Options

### Option A: Serial Execution (Recommended)
**Approach**: Run Notification E2E tests with `ginkgo --procs=1` to eliminate parallelism

**Pros**:
- ✅ Eliminates virtiofs concurrent I/O contention
- ✅ Deterministic behavior (same as focused run)
- ✅ Simpler infrastructure (less stress on Podman)
- ✅ Easier debugging (sequential test order)

**Cons**:
- ⚠️ Increased runtime: ~5min (focused) → ~15min (full suite serial)
- ⚠️ Less realistic CI simulation (CI often runs tests in parallel)

**Implementation**:
```bash
# In Makefile or test script
test-e2e-notification:
	ginkgo --procs=1 -v test/e2e/notification/
```

### Option B: Continue Tuning Delay (Not Recommended)
**Approach**: Keep increasing initial delay until parallel tests pass

**Pros**:
- ✅ Preserves parallel execution benefits
- ✅ Closer to CI behavior

**Cons**:
- ❌ Unpredictable delay requirements (depends on system load)
- ❌ Tests become slow anyway (4s+ delay per file check)
- ❌ Doesn't solve root cause (virtiofs sync under load)
- ❌ Infrastructure flakiness remains a blocker

### Option C: Hybrid Approach
**Approach**: Run most tests in parallel, but file-heavy tests serially

**Pros**:
- ✅ Balances speed and reliability
- ✅ Only slow tests run serially

**Cons**:
- ❌ Complex Makefile/Ginkgo setup
- ❌ Requires manual test categorization
- ❌ Doesn't fully eliminate virtiofs issues

## Recommendation: Option A (Serial Execution)

**Rationale**:
1. **Simplicity**: One change to fix all file validation tests reliably
2. **Confidence**: Focused runs already prove serial execution works
3. **Maintainability**: No complex delay tuning or test categorization needed
4. **Determinism**: Predictable test behavior across environments
5. **Acceptable Trade-off**: 15min for full E2E suite is reasonable for pre-merge validation

**Implementation Steps**:
1. Update `Makefile` or test script to use `ginkgo --procs=1`
2. Run full suite to verify 19/19 passing
3. Document runtime expectations in test README
4. Monitor for infrastructure flakiness (AuthWebhook/DataStorage image issues)

## Current Test Results (Before Fix)

| Test Scenario | Focused Run | Parallel Run |
|---|---|---|
| 01: Notification Lifecycle Audit | ✅ PASS | ✅ PASS |
| 02: Audit Correlation | ✅ PASS | ✅ PASS |
| 03: File Delivery Validation | ✅ PASS | ✅ PASS |
| 04: Failed Delivery Audit | ✅ PASS | ✅ PASS |
| 06: Multi-Channel Fanout Scenario 1 | ✅ PASS | ❌ FAIL (file not found) |
| 06: Multi-Channel Fanout Scenario 2 | ✅ PASS | ✅ PASS |
| 07: Priority Routing Scenario 1 | ✅ PASS | ❌ FAIL (file not found) |
| 07: Priority Routing Scenario 2 | ✅ PASS | ❌ FAIL (file not found) |
| 07: Priority Routing Scenario 3 | N/A | ❌ FAIL (file not found) |
| (13 other tests) | ✅ PASS | ✅ PASS |

**Total**: Focused (3/3) vs. Parallel (15/19)

## Next Steps

1. **Immediate**: User approval for Option A (serial execution)
2. **If approved**: Update Makefile, run full suite, verify 19/19 passing
3. **If not approved**: Continue with Option B (increase delay to 5s, 6s, ...)
4. **Future**: Investigate Podman virtiofs alternatives (e.g., `bindfs`, NFS mounts)

## Related Decisions
- **Supports**: BR-NOT-001 (File delivery), DD-NOT-006 (E2E infrastructure)
- **Conflicts**: None (serial execution is a valid E2E strategy)

## Review & Evolution

**When to Revisit**:
- If runtime becomes unacceptable (>20 minutes)
- If CI environment shows different behavior
- If Podman/virtiofs performance improves significantly

**Success Metrics**:
- 19/19 E2E tests passing consistently
- Runtime <15 minutes for full suite
- No infrastructure flakiness (image load/build failures)
