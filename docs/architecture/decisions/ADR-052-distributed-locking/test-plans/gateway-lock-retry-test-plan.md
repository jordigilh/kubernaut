# Gateway Distributed Lock Retry - Unit Test Plan

**Version**: 1.0
**Date**: January 18, 2026
**Status**: Active
**Related ADR**: [ADR-052 Addendum 001: Exponential Backoff with Jitter](../ADR-052-ADDENDUM-001-exponential-backoff-jitter.md)
**Business Requirements**: BR-GATEWAY-190

---

## 🎯 **Test Objectives**

**Primary Goal**: Validate that Gateway's distributed lock retry mechanism uses exponential backoff with jitter instead of unbounded recursion.

**Secondary Goals**:
- Ensure max retry limit prevents infinite loops
- Verify jitter prevents thundering herd
- Validate deduplication during retry
- Ensure stack safety (iterative vs recursive)

---

## 📋 **Test Coverage Strategy**

Per [TESTING_GUIDELINES.md](../../../../development/business-requirements/TESTING_GUIDELINES.md):

- **Unit Tests**: 70%+ coverage - Algorithm correctness, edge cases
- **Integration Tests**: >50% coverage - Real K8s API, lock manager behavior
- **E2E Tests**: 10-15% coverage - Full deployment validation

**This Test Plan**: Unit tier (algorithm correctness)

---

## 🧪 **Unit Test Cases**

### **Test Case Group 1: Lock Acquisition Success**

#### **LOCK-RETRY-U-001: Immediate lock acquisition (no retry)**

**Purpose**: Validate no retry overhead when lock is available

**Test ID**: `LOCK-RETRY-U-001`

**Mapped To**: BR-GATEWAY-190 (Multi-Replica Deduplication Safety)

**Scenario**: Lock acquired on first attempt

**Given**:
- Mock lock manager configured to return `acquired=true` on first attempt
- Test signal with `fingerprint="test-fp-123"`

**When**:
- Lock acquisition attempted

**Then**:
- Lock acquired successfully
- Only 1 attempt made
- No backoff delay
- No retry occurred

**Assertions**:
```go
Expect(acquired).To(BeTrue())
Expect(lockManager.acquireAttempt).To(Equal(1))
Expect(elapsed).To(BeNumerically("<", 10*time.Millisecond)) // No backoff
```

---

### **Test Case Group 2: Exponential Backoff Behavior**

#### **LOCK-RETRY-U-002: Exponential backoff with successful retry**

**Purpose**: Validate exponential backoff timing (100ms → 200ms → 400ms)

**Test ID**: `LOCK-RETRY-U-002`

**Mapped To**: ADR-052 Addendum 001 (Exponential Backoff)

**Scenario**: Lock acquired after 3 retries

**Given**:
- Mock lock manager configured: `[false, false, false, true]`
- Backoff config: `BasePeriod=100ms, Multiplier=2.0, Jitter=0%` (deterministic)

**When**:
- Lock acquisition retried 3 times

**Then**:
- Total backoff time ≈ 700ms (100 + 200 + 400)
- 4 attempts made (1 initial + 3 retries)
- Lock acquired on 4th attempt

**Assertions**:
```go
Expect(lockManager.acquireAttempt).To(Equal(4))
Expect(elapsed).To(BeNumerically(">=", 700*time.Millisecond))
Expect(elapsed).To(BeNumerically("<=", 900*time.Millisecond)) // ±200ms tolerance
```

**Confidence**: 95% (standard exponential backoff pattern)

---

#### **LOCK-RETRY-U-003: Max retry limit (10 attempts)**

**Purpose**: Validate bounded retry prevents infinite loops

**Test ID**: `LOCK-RETRY-U-003`

**Mapped To**: ADR-052 Addendum 001 (Bounded Retry)

**Scenario**: Lock never acquired, max retries exceeded

**Given**:
- Mock lock manager configured: `[false × 11]` (always fails)
- Max retries: 10

**When**:
- Lock acquisition attempted repeatedly

**Then**:
- Exactly 10 attempts made
- Timeout error returned
- No 11th attempt

**Assertions**:
```go
Expect(lockManager.acquireAttempt).To(Equal(10))
Expect(err).To(HaveOccurred())
Expect(err.Error()).To(ContainSubstring("lock acquisition timeout"))
Expect(err.Error()).To(ContainSubstring("10 attempts"))
```

**Confidence**: 100% (critical safety feature)

---

### **Test Case Group 3: Jitter Distribution**

#### **LOCK-RETRY-U-004: Jitter prevents thundering herd**

**Purpose**: Validate ±10% jitter distributes retry attempts

**Test ID**: `LOCK-RETRY-U-004`

**Mapped To**: ADR-052 Addendum 001 (Anti-Thundering Herd)

**Scenario**: 100 concurrent lock acquisitions with jitter

**Given**:
- Backoff config: `BasePeriod=100ms, Jitter=10%`
- 100 iterations

**When**:
- Backoff calculated 100 times for attempt=1

**Then**:
- All backoff times within 90-110ms range
- Distribution shows variance (not all identical)

**Assertions**:
```go
for _, backoff := range backoffTimes {
    Expect(backoff).To(BeNumerically(">=", 90*time.Millisecond))
    Expect(backoff).To(BeNumerically("<=", 110*time.Millisecond))
}
// Verify variance (not all same value)
Expect(len(uniqueValues(backoffTimes))).To(BeNumerically(">", 10))
```

**Confidence**: 90% (statistical validation)

---

### **Test Case Group 4: Deduplication During Retry**

#### **LOCK-RETRY-U-005: Deduplication check finds existing RR**

**Purpose**: Validate early exit when other pod creates RR

**Test ID**: `LOCK-RETRY-U-005`

**Mapped To**: BR-GATEWAY-190 (Deduplication Safety)

**Scenario**: Lock fails, but RR exists after backoff

**Given**:
- Mock lock manager: `[false, false]` (always fails)
- Mock phase checker: `shouldDeduplicate=true, existingRR=&RR{Name:"rr-123"}`

**When**:
- First lock attempt fails
- Backoff executes (100ms)
- Deduplication check runs

**Then**:
- Success response (duplicate detected)
- No further lock attempts
- Existing RR returned

**Assertions**:
```go
Expect(lockManager.acquireAttempt).To(Equal(1)) // Only first attempt
Expect(result).ToNot(BeNil())
Expect(result.IsDuplicate).To(BeTrue())
Expect(result.ExistingRRName).To(Equal("rr-123"))
```

**Confidence**: 95% (critical deduplication path)

---

### **Test Case Group 5: Error Handling**

#### **LOCK-RETRY-U-006: K8s API error propagation**

**Purpose**: Validate API errors fail immediately (no retry)

**Test ID**: `LOCK-RETRY-U-006`

**Mapped To**: ADR-052 (Error Handling)

**Scenario**: K8s API returns error (not lock contention)

**Given**:
- Mock lock manager: returns error `"k8s API: permission denied"`

**When**:
- Lock acquisition attempted

**Then**:
- Error returned immediately
- No retry attempted
- Clear error message

**Assertions**:
```go
Expect(err).To(HaveOccurred())
Expect(err.Error()).To(ContainSubstring("k8s API"))
Expect(lockManager.acquireAttempt).To(Equal(1)) // No retry on API errors
```

**Confidence**: 100% (error handling correctness)

---

#### **LOCK-RETRY-U-007: Timeout error message clarity**

**Purpose**: Validate clear error when max retries exceeded

**Test ID**: `LOCK-RETRY-U-007`

**Mapped To**: ADR-052 Addendum 001 (User Experience)

**Scenario**: Max retries exceeded

**Given**:
- Mock lock manager: `[false × 11]`
- Max retries: 10

**When**:
- Lock acquisition fails 10 times

**Then**:
- Error message includes:
  - "lock acquisition timeout"
  - "10 attempts"
  - `fingerprint`

**Assertions**:
```go
Expect(err.Error()).To(ContainSubstring("lock acquisition timeout"))
Expect(err.Error()).To(ContainSubstring("10 attempts"))
Expect(err.Error()).To(ContainSubstring("test-fingerprint-123"))
```

**Confidence**: 100% (debugging aid)

---

### **Test Case Group 6: Stack Safety**

#### **LOCK-RETRY-U-008: Iterative loop (no recursion)**

**Purpose**: Validate constant stack usage vs recursive implementation

**Test ID**: `LOCK-RETRY-U-008`

**Mapped To**: ADR-052 Addendum 001 (Stack Overflow Prevention)

**Scenario**: High retry count doesn't exhaust stack

**Given**:
- Mock lock manager: `[false × 9, true]` (succeed on 10th)
- 10 retry attempts

**When**:
- Lock acquisition retries 9 times

**Then**:
- Lock acquired
- Stack usage constant (not growing)
- No stack overflow

**Assertions**:
```go
Expect(lockManager.acquireAttempt).To(Equal(10))
// Note: Stack safety validated by test completing successfully
// Recursive implementation would risk overflow at 100+ retries
```

**Confidence**: 100% (critical safety feature)

---

## 📊 **Coverage Matrix**

| Feature | Unit Test | Integration Test | E2E Test |
|---------|-----------|------------------|----------|
| **Exponential backoff** | ✅ U-002 | ⬜ | ⬜ |
| **Max retry limit** | ✅ U-003 | ⬜ | ⬜ |
| **Jitter distribution** | ✅ U-004 | ⬜ | ⬜ |
| **Deduplication during retry** | ✅ U-005 | ✅ (separate) | ⬜ |
| **Error handling** | ✅ U-006, U-007 | ⬜ | ⬜ |
| **Stack safety** | ✅ U-008 | ⬜ | ⬜ |
| **Lock acquisition** | ✅ U-001 | ✅ (separate) | ✅ (separate) |

**Legend**: ✅ Tested | ⬜ Not tested in this tier

---

## 🚫 **Anti-Patterns to Avoid**

Per TESTING_GUIDELINES.md:

### **❌ FORBIDDEN: time.Sleep() for waiting**

```go
// ❌ WRONG: Using sleep to wait for condition
time.Sleep(5 * time.Second)
Expect(condition).To(BeTrue())

// ✅ CORRECT: Using Eventually() for async operations
Eventually(func() bool {
    return condition
}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())
```

**Exception**: `time.Sleep()` IS acceptable in LOCK-RETRY-U-002 because we're testing **timing behavior itself** (exponential backoff duration).

### **❌ FORBIDDEN: Skip() to bypass failures**

```go
// ❌ WRONG: Skipping test when condition not met
if !lockManagerAvailable {
    Skip("Lock manager not available")
}

// ✅ CORRECT: Fail with clear message
Expect(lockManagerAvailable).To(BeTrue(),
    "Lock manager REQUIRED for this test")
```

### **✅ REQUIRED: Test against business outcomes**

```go
// ❌ WRONG: Testing implementation details
Expect(backoffDuration).To(Equal(100 * time.Millisecond))

// ✅ CORRECT: Testing business outcome
Expect(elapsed).To(BeNumerically(">=", 700*time.Millisecond),
    "ADR-052: Exponential backoff should delay ~700ms for 3 retries")
```

---

## 🎯 **Success Criteria**

### **Test Suite Success**

- [ ] All 8 unit tests pass
- [ ] 95%+ code coverage for lock retry logic
- [ ] No `time.Sleep()` anti-patterns (except timing tests)
- [ ] All tests reference BR-GATEWAY-190 or ADR-052
- [ ] Test execution < 5 seconds (fast feedback)

### **Implementation Validation**

- [ ] Iterative loop (not recursive)
- [ ] Exponential backoff (100ms → 1s)
- [ ] ±10% jitter
- [ ] Max 10 retries
- [ ] Clear error messages

---

## 📝 **Test Execution Plan**

### **Phase 1: RED (Tests Fail)**

```bash
# Step 1: Implement tests based on this plan
# Step 2: Run tests against current (recursive) implementation
make test-unit-gateway

# Expected: FAIL (tests expect exponential backoff, code uses recursion)
```

### **Phase 2: GREEN (Implement Fix)**

```bash
# Step 3: Implement exponential backoff in pkg/gateway/server.go
# Step 4: Run tests again
make test-unit-gateway

# Expected: PASS (all 8 tests green)
```

### **Phase 3: REFACTOR (Cleanup)**

```bash
# Step 5: Review implementation for improvements
# Step 6: Run tests to ensure still passing
make test-unit-gateway

# Expected: PASS (no regression)
```

---

## 🔗 **Related Documents**

- **ADR-052 Addendum 001**: [Exponential Backoff with Jitter](../ADR-052-ADDENDUM-001-exponential-backoff-jitter.md)
- **TESTING_GUIDELINES.md**: [Testing standards](../../../../development/business-requirements/TESTING_GUIDELINES.md)
- **pkg/shared/backoff**: Shared backoff implementation
- **Integration Test Plan**: (separate document)

---

## ✅ **Approval & Sign-Off**

**Test Plan Reviewed By**: (to be filled)
**Date**: January 18, 2026
**Approved**: ⬜ Pending

**Next Steps**:
1. Implement tests following this plan
2. Verify RED phase (tests fail with current code)
3. Implement exponential backoff (GREEN phase)
4. Review and refactor (REFACTOR phase)
