# SignalProcessing Test Failures - FINAL Root Cause: Multiple Audit Event Emissions

**Date**: 2026-01-14
**Status**: 🔴 ROOT CAUSE CONFIRMED
**Priority**: P1 - Test Expectations vs Controller Behavior Mismatch
**Related**: `docs/handoff/SP_TEST_ROOT_CAUSE_STALE_AUDIT_EVENTS_JAN14_2026.md`, `docs/handoff/SP_UNIQUE_CORRELATION_ID_FIX_JAN14_2026.md`

---

## 📋 Executive Summary

**USER INSIGHT (CORRECT)**: "Controller should be queried directly without the cache, and the audit should flush and then query by correlationID in an Eventually() loop."

**ANALYSIS RESULT**: Tests ARE doing this correctly! The problem is **the controller emits 3 `classification.decision` events** for unmapped severity tests, but **the test expects exactly 1 event**.

**ACTUAL ROOT CAUSE**: Test expectation mismatch - not stale data, not caching, not timing issues.

---

## 🔍 Final Evidence

### Unique Correlation ID Confirmed

```log
correlation_id="test-policy-fallback-audit-rr-1768440104645983000"
```

✅ **Unique per test run** (includes Unix nanosecond timestamp)
✅ **No stale data from previous runs**
✅ **Correlation ID fix worked as intended**

### Multiple Event Emissions Discovered

```log
# Same correlation ID, THREE classification.decision events:
{"logger":"audit-store","msg":"✅ Event buffered successfully",
 "event_type":"signalprocessing.classification.decision",
 "correlation_id":"test-policy-fallback-audit-rr-1768440104645983000",
 "total_buffered":15}  ← Event #1

{"logger":"audit-store","msg":"✅ Event buffered successfully",
 "event_type":"signalprocessing.classification.decision",
 "correlation_id":"test-policy-fallback-audit-rr-1768440104645983000",
 "total_buffered":17}  ← Event #2

{"logger":"audit-store","msg":"✅ Event buffered successfully",
 "event_type":"signalprocessing.classification.decision",
 "correlation_id":"test-policy-fallback-audit-rr-1768440104645983000",
 "total_buffered":21}  ← Event #3
```

### Test Expectation

```go
// Line 336: severity_integration_test.go
g.Expect(count).To(Equal(1), "Should have exactly 1 classification.decision event")
```

### Actual Result

```
Expected <int>: 3
to equal <int>: 1
```

---

## 💡 Root Cause Analysis

### Why Does Controller Emit 3 Events?

**Test Case**: `"should emit audit event with policy-defined fallback severity"`

**Signal Severity**: `"UNMAPPED_VALUE_999"` (unmapped/invalid value)

**Controller Behavior** (observed from logs):
1. **First reconciliation**: Initial classification attempt → Emit event #1
2. **Second reconciliation**: Retry or re-categorization → Emit event #2
3. **Third reconciliation**: Final classification or error handling → Emit event #3

**Possible Causes**:
1. **Multiple reconciliations triggered** by status updates during classification
2. **Controller retries** classification for unmapped severity values
3. **Business logic** emits classification event at multiple decision points
4. **Audit client** emits events during initialization, processing, AND completion

### Is This a Bug or Expected Behavior?

**Need to determine**:
- ✅ Does the controller SHOULD emit only 1 event per classification?
- ❌ OR should the test expect 3 events for complex classification scenarios?

---

## 🎯 Next Steps

### Option A: Fix Controller (if multiple emissions are bugs)

**Investigation needed**:
1. Check controller reconciliation logic for duplicate emission points
2. Review audit client calls in `signalprocessing_controller.go`
3. Ensure classification events are emitted only once per final decision
4. Add controller-level deduplication if needed

**Expected fix location**: `internal/controller/signalprocessing/signalprocessing_controller.go`

---

### Option B: Fix Test Expectation (if multiple emissions are correct)

**If controller behavior is correct**, update test to expect multiple events:

```go
// Option B1: Expect exact count based on business logic
g.Expect(count).To(BeNumerically(">=", 1),
    "Should have at least 1 classification.decision event")

// Option B2: Test only the LATEST event (final decision)
event, err := getLatestAuditEvent("signalprocessing.classification.decision", correlationID)
Expect(err).ToNot(HaveOccurred())
// Assert on event content, ignore count
```

**Rationale**: If controller emits interim classification events during processing, tests should verify the **final decision**, not the count.

---

### Option C: Investigate Why Unmapped Severity Causes Multiple Emissions

**Hypothesis**: Unmapped severity (`"UNMAPPED_VALUE_999"`) triggers:
1. Initial classification attempt → Event #1
2. Policy fallback evaluation → Event #2
3. Final severity determination → Event #3

**Compare with passing tests**:
- Tests with valid severity values (e.g., `"critical"`) might emit only 1 event
- Tests with invalid severity values might emit multiple events during error handling

**Verification**:
```bash
# Check if other tests with valid severity emit fewer events
grep "correlation_id.*test-audit-event-rr" /tmp/sp-integration-unique-correlation-id-fix.log | \
  grep "classification.decision" | wc -l
```

---

## 🧪 Validation Plan

### Step 1: Understand Controller Emission Points

**Action**: Review controller code to find ALL places where `classification.decision` events are emitted.

**Expected locations**:
- Initial classification start
- Mid-classification decision points (categorization loops?)
- Final classification completion

### Step 2: Verify Business Logic

**Question**: Should there be 3 distinct classification.decision events, or is this unexpected?

**Consult**:
- Controller developer
- Audit event specification (DD-AUDIT-*)
- SignalProcessing business requirements (BR-SP-105)

### Step 3: Fix Based on Findings

**If 1 event is correct**:
- Fix controller to emit only once
- Keep test expectation at `Equal(1)`

**If 3 events are correct**:
- Fix test to expect 3 OR use `BeNumerically(">=", 1)`
- Document why multiple events are valid

---

## 📊 Impact Assessment

### Current Test Results (After Unique Correlation ID Fix)

```
Ran 87 of 92 Specs in 132.341 seconds
FAIL! -- 84 Passed | 3 Failed | 2 Pending | 3 Skipped
Pass Rate: 96.6% (84/87)
```

### Affected Tests (All Expecting count == 1)

1. **Line 337**: `should emit audit event with policy-defined fallback severity`
   - Expected: 1, Found: 3 (FAIL after 61s)

2. **Line 213**: `should emit 'classification.decision' audit event with both external and normalized severity`
   - Expected: 1, Found: ? (INTERRUPTED)

3. **audit_integration_test.go:274**: `should create 'classification.decision' audit event with all categorization results`
   - Expected: 1, Found: ? (INTERRUPTED)

**Common Pattern**: All 3 tests query for `classification.decision` events and expect exactly 1.

---

## 🔧 Recommended Approach

### Immediate Action: Investigate Controller Behavior

**Priority**: Understand why 3 events are emitted before deciding on fix.

**Commands**:
```bash
# Find all classification.decision emission points in controller
grep -r "classification.decision" internal/controller/signalprocessing/ -A5 -B5

# Check if audit client has deduplication logic
grep -r "classification.decision" pkg/signalprocessing/audit/ -A10

# Compare event counts for different test scenarios
grep "classification.decision.*correlation_id" /tmp/sp-integration-unique-correlation-id-fix.log | \
  cut -d'"' -f12 | sort | uniq -c
```

---

## 📝 Key Learnings

### What User Was Right About

1. ✅ **Controller query should be direct** (non-cached) - tests already do this
2. ✅ **Audit should flush and query by correlation ID** - tests already do this
3. ✅ **Eventually() loop is correct pattern** - tests already use this

**The user's insight forced us to look beyond timing/caching and discover the actual issue: emission count mismatch.**

### What We Initially Misdiagnosed

1. ❌ **Stale audit events from previous runs** - correlation IDs ARE unique now
2. ❌ **Slow DataStorage** - queries complete in <2s
3. ❌ **Cache staleness** - k8sClient is non-cached
4. ❌ **Insufficient timeouts** - 60s is more than enough

**Actual issue**: Test expectations don't match controller behavior.

---

## 🔗 Related Documents

1. **Initial Triage**: `docs/handoff/TEST_FAILURE_TRIAGE_JAN14_2026.md`
2. **Stale Data Analysis**: `docs/handoff/SP_TEST_ROOT_CAUSE_STALE_AUDIT_EVENTS_JAN14_2026.md`
3. **Unique ID Implementation**: `docs/handoff/SP_UNIQUE_CORRELATION_ID_FIX_JAN14_2026.md`
4. **This Document**: Final root cause after correlation ID fix

---

## ⏭️ Next Steps

1. [ ] Review controller code for `classification.decision` emission points
2. [ ] Consult with SignalProcessing team on expected behavior
3. [ ] Decide: Fix controller OR fix test expectations
4. [ ] Implement fix based on business requirements
5. [ ] Re-run tests to verify 100% pass rate

---

**Last Updated**: 2026-01-14T20:30:00
**Status**: 🔄 Awaiting controller behavior clarification before implementing fix
