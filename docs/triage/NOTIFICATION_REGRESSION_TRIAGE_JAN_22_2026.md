# Notification Integration Test Regression Triage - January 22, 2026

**Context**: After fixing the race condition in Notification service (DD-PERF-001 + DD-NOT-008), we have introduced a regression affecting 2-5 tests depending on test run.

**Session**: Post-race-condition-fix validation  
**Status**: 🚨 REGRESSION DETECTED

---

## 📊 **TEST RESULTS SUMMARY**

### **Inconsistent Results Across Runs**

| Run Method | Pass/Total | Failed Tests | Flaked Tests |
|-----------|-----------|--------------|--------------|
| **Direct go test** | 115/117 (98.3%) | 2 | 0 |
| **Make target (run 1)** | 116/117 (99.1%) | 1 | 1 |
| **Make target (run 2)** | 112/117 (95.7%) | 5 | 0 |

**Observation**: Test results are **non-deterministic**, suggesting a race condition or timing issue introduced by our fix.

---

## 🚨 **FAILING TESTS**

### **Test #1: Retry Logic - Stop After Success**

**File**: `test/integration/notification/controller_retry_logic_test.go:356`  
**Test**: `[BR-NOT-054] should stop retrying after first success`  
**Business Requirement**: BR-NOT-054 (Retry Logic Correctness)

#### **Error**
```
Expected <int>: 4 to equal <int>: 3
DD-STATUS-001: Wait for all 3 attempts to propagate to API server
```

#### **Expected Behavior**
- Delivery fails twice → succeeds on 3rd attempt
- **Total attempts**: 3 (2 failures + 1 success)
- Retry logic stops after first success

#### **Actual Behavior**
- Delivery fails twice → succeeds on 3rd attempt
- **Total attempts recorded**: 4 (not 3!)
- Extra attempt is being recorded

#### **Root Cause Hypothesis**
Our race condition fix (moving in-flight counter management) may be causing an extra attempt to be counted. The logs show:

```
deliveryAttemptsRecorded: 1
statusDeliveryAttempts: 2  <-- Already has 2 before new attempt added!
→ After update: totalAttempts: 3
→ Then another reconcile: deliveryAttemptCount: 4  <-- EXTRA ATTEMPT!
```

**Evidence**: The status update is adding attempts correctly (1 → 2 → 3), but then a **4th attempt** appears after the notification is marked as `Sent`.

---

### **Test #2: Partial Failure Handling**

**File**: `test/integration/notification/controller_partial_failure_test.go:178`  
**Test**: `[BR-NOT-053] should mark notification as PartiallySent`  
**Business Requirement**: BR-NOT-053 (Partial Failure Handling)

#### **Error**
```
Expected status.Phase to be PartiallySent
```

#### **Expected Behavior**
- File channel fails
- Console/log channels succeed
- Phase transitions to `PartiallySent`

#### **Actual Behavior**
- Appears to be affected by the same race condition as Test #1
- May be counting extra attempts which affects phase transition logic

---

### **Test #3-5: Audit Event Emission (Sporadic)**

**File**: `test/integration/notification/controller_audit_emission_test.go:230, 300, 364`  
**Tests**: Audit event correlation and emission  
**Business Requirements**: BR-NOT-062, DD-AUDIT-003

#### **Errors**
```
Timed out after 10.001s.
Audit event correlation_id should match remediationID
Expected <bool>: false to be true
```

#### **Pattern**
- These tests **sometimes pass, sometimes fail** (flaky)
- Suggests timing/race condition in audit event emission
- May be related to rapid reconciliations caused by our fix

---

## 🔍 **ROOT CAUSE ANALYSIS**

### **What Changed**
In our previous fix (NOTIFICATION_RACE_CONDITION_FIX.md), we:

1. **Added `apiReader` for cache bypass** (DD-PERF-001)
   - Uses `m.apiReader.Get()` to fetch fresh data from API server
   - Prevents stale cache reads ✅

2. **Moved in-flight counter management** (DD-NOT-008)
   - Moved from `doDelivery()` to `DeliverToChannels()` orchestrator loop
   - Ensured `attemptCount` is retrieved **after** in-flight increment ✅

### **Unintended Consequence**
Our fix appears to have introduced a **subtle timing window** where:

1. **Primary reconcile** completes successfully
   - Records attempt #1, #2, #3
   - Transitions to `Sent` phase
   - Sets `ObservedGeneration = Generation`

2. **Rapid follow-up reconcile** (triggered by status update)
   - Should be prevented by generation check
   - But logs show: `deliveryAttemptCount: 4` after phase = `Sent`
   - This suggests the duplicate prevention isn't working as expected

### **Key Log Evidence**

From the failing test:
```go
2026-01-22T16:16:41	phase: "Sent"
2026-01-22T16:16:41	deliveryAttemptCount: 3  // ✅ Correct
→ Status update triggers reconcile
2026-01-22T16:16:41	deliveryAttemptCount: 4  // ❌ EXTRA!
```

**Question**: Why is a reconcile happening **after** the notification is marked as `Sent` and generation is marked as processed?

---

## 💡 **HYPOTHESES**

### **Hypothesis A: Status Update Triggers Extra Reconcile**
- Status update (adding attempts) triggers watch event
- New reconcile sees `Generation = ObservedGeneration` but still processes
- Duplicate prevention check failing

**Evidence**: Logs show `✅ DUPLICATE RECONCILE PREVENTED` but **after** attempt was already recorded

### **Hypothesis B: In-Flight Counter Race**
- Multiple reconciles entering `DeliverToChannels()` simultaneously
- In-flight counter increments for both
- Both fetch attempt counts and add attempts

**Counter-Evidence**: We have in-flight counter management at orchestrator level now

### **Hypothesis C: API Reader Not Preventing Race**
- `apiReader.Get()` fetches fresh data
- But between fetch and update, another reconcile completes
- `RetryOnConflict` retries → fetches again → sees new attempt count → adds another

**Evidence**: Status manager logs show `deliveryAttemptsBeforeUpdate: 3, newAttemptsToAdd: 1` → should result in 4

---

## 🎯 **RECOMMENDED INVESTIGATION**

### **Step 1: Verify Duplicate Prevention Logic**
Check if duplicate reconcile prevention is working:

```bash
# Look for generation check logic
grep -A 10 "ObservedGeneration.*Generation" pkg/notification/controller/notificationrequest_controller.go
```

**Question**: Does the generation check happen **before** or **after** attempt counting?

### **Step 2: Check Status Update Timing**
Review when status updates trigger reconciles:

```bash
# Find where ObservedGeneration is set
grep -B 5 -A 5 "ObservedGeneration.*=" pkg/notification/
```

**Question**: Is `ObservedGeneration` set **before** or **after** the attempt is recorded in status?

### **Step 3: Review In-Flight Counter Scope**
Check if in-flight counter prevents concurrent attempt recording:

```bash
# Review orchestrator in-flight counter logic
grep -A 20 "IncrementInFlight" pkg/notification/delivery/orchestrator.go
```

**Question**: Does the in-flight counter prevent the race, or just prevent concurrent **deliveries**?

---

## 🛠️ **POSSIBLE FIXES**

### **Option A: Move Attempt Recording Earlier in Flow**
- Record attempt **before** calling `DeliverToChannels()`
- Update `ObservedGeneration` **before** delivery
- Ensures duplicate prevention happens before attempt counting

**Risk**: May break atomicity guarantees

### **Option B: Add Attempt-Level Deduplication**
- Track which attempts have been recorded (by attempt ID or timestamp)
- Skip recording if attempt already exists in status
- Similar pattern to audit event deduplication

**Risk**: Adds complexity

### **Option C: Stricter Generation Check**
- Check `Generation == ObservedGeneration` **AND** `Phase == terminal`
- Prevent **any** reconciliation after terminal phase reached
- More aggressive duplicate prevention

**Risk**: May prevent legitimate reconciliations (e.g., updates after completion)

### **Option D: Investigate apiReader Timing**
- The `apiReader` fetch might not be solving the race as expected
- Consider fetching **and locking** the resource before attempt recording
- Or use optimistic locking with version checks

**Risk**: Performance impact

---

## 📋 **NEXT STEPS**

### **Immediate**
1. ✅ Document regression (this file)
2. 🔍 Run focused test with verbose logging to see exact attempt flow
3. 📊 Add debug logging around:
   - Generation checks
   - Attempt recording
   - ObservedGeneration updates

### **Investigation**
1. Read `notificationrequest_controller.go` reconcile logic carefully
2. Trace exact flow from reconcile start → attempt count → status update → ObservedGeneration
3. Identify where the "extra" attempt is being added

### **Fix**
1. Present investigation findings to user
2. Propose specific fix with evidence
3. Implement fix following TDD (write test that reproduces issue, then fix)
4. Verify all 117 tests pass consistently

---

## 🎯 **USER DECISION REQUIRED**

### **Question 1**: Investigation Approach
Which hypothesis should we investigate first?
- **A**: Duplicate prevention logic
- **B**: Status update timing
- **C**: In-flight counter scope
- **D**: All of the above (comprehensive analysis)

### **Question 2**: Acceptable Risk Level
- **Low Risk**: Investigate thoroughly before any code changes (1-2 hours)
- **Medium Risk**: Implement most likely fix (Option B - attempt deduplication) and test
- **High Risk**: Revert race condition fix and re-approach

---

**Last Updated**: 2026-01-22 16:23:00 EST  
**Status**: Awaiting user decision on investigation approach  
**Impact**: 2-5 Notification integration tests failing (95.7%-99.1% pass rate)
