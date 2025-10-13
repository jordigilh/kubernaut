# Day 9 KNOWN_ISSUE_001 DO-RED Complete - Context Propagation Tests

**Date**: October 12, 2025
**Phase**: Day 9 DO-RED (Write Failing Tests)
**Status**: ✅ COMPLETE
**Time**: 20 minutes (as estimated)

---

## 🎯 Objective

Write failing unit tests to expose the context propagation bug in `Coordinator.Write()` and `writePostgreSQLOnly()`.

**Bug**: Methods accept `ctx context.Context` but call `Begin()` instead of `BeginTx(ctx, nil)`, causing context cancellation/timeout to be ignored.

---

## 📁 Files Created

### Test File
- **`test/unit/datastorage/dualwrite_context_test.go`** (313 lines)
  - 6 comprehensive context propagation tests
  - Context-aware mock (`MockDBWithContext`)
  - Tracks `Begin()` vs `BeginTx()` calls

---

## 🧪 Test Results (DO-RED)

### Expected Outcome: ❌ TESTS FAIL (Bug Exposed)

```bash
Summarizing 6 Failures:
  [FAIL] BR-STORAGE-016.1: cancelled context should fail fast
  [FAIL] BR-STORAGE-016.2: expired deadline should fail fast
  [FAIL] BR-STORAGE-016.3: zero timeout should fail fast
  [FAIL] BR-STORAGE-016.4: should respect cancelled context in fallback path
  [FAIL] BR-STORAGE-016.5: should timeout if transaction takes too long
  [FAIL] BR-STORAGE-016.6: should call BeginTx with context (not Begin)

Ran 20 of 20 Specs in 0.003 seconds
FAIL! -- 14 Passed | 6 Failed | 0 Pending | 0 Skipped
```

**Result**: ✅ **All 6 context tests FAIL as expected** (bug is exposed)

---

## 📋 Test Coverage

### BR-STORAGE-016: Context Propagation (NEW)

| Test ID | Scenario | Expected Behavior | Current Status |
|---|---|---|---|
| **016.1** | Cancelled context | Fail fast with `context.Canceled` | ❌ FAIL (ignored) |
| **016.2** | Expired deadline | Fail fast with `context.DeadlineExceeded` | ❌ FAIL (ignored) |
| **016.3** | Zero timeout | Fail fast with `context.DeadlineExceeded` | ❌ FAIL (ignored) |
| **016.4** | Cancelled context in fallback | Respect cancellation | ❌ FAIL (ignored) |
| **016.5** | Timeout during transaction | Respect deadline | ❌ FAIL (ignored) |
| **016.6** | API usage verification | Call `BeginTx()` not `Begin()` | ❌ FAIL (calls `Begin()`) |

---

## 🔍 Key Implementation Details

### MockDBWithContext

**Purpose**: Tracks which API is called (`Begin()` vs `BeginTx()`)

```go
type MockDBWithContext struct {
    beginCalled   bool // Legacy Begin() called (BAD)
    beginTxCalled bool // Modern BeginTx() called (GOOD)
    beginTxContext context.Context
    slowMode      bool
    delay         time.Duration
    // ... other fields
}

// BeginTx - Context-aware (CORRECT API)
func (m *MockDBWithContext) BeginTx(ctx context.Context, opts *sql.TxOptions) (dualwrite.Tx, error) {
    m.beginTxCalled = true
    m.beginTxContext = ctx

    // Check if context is already cancelled
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    // Simulate slow database if enabled
    if m.slowMode && m.delay > 0 {
        select {
        case <-time.After(m.delay):
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }

    return &MockTxContext{dbWithContext: m}, nil
}

// Begin - Legacy API (INCORRECT)
func (m *MockDBWithContext) Begin() (dualwrite.Tx, error) {
    m.beginCalled = true // Track legacy API usage
    return &MockTxContext{dbWithContext: m}, nil
}
```

### Table-Driven Tests

**3 entries for immediate context failures**:
```go
DescribeTable("should respect context signals",
    func(ctxSetup func() context.Context, expectedErr error, description string) {
        ctx := ctxSetup()
        embedding := make([]float32, 384)

        _, err := coordinator.Write(ctx, testAudit, embedding)

        Expect(err).To(HaveOccurred(), description)
        Expect(errors.Is(err, expectedErr)).To(BeTrue())
    },
    Entry("BR-STORAGE-016.1: cancelled context...", ...),
    Entry("BR-STORAGE-016.2: expired deadline...", ...),
    Entry("BR-STORAGE-016.3: zero timeout...", ...),
)
```

### Slow Database Simulation

**Test 016.5: Timeout during transaction**:
```go
mockDB.slowMode = true
mockDB.delay = 100 * time.Millisecond

ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
defer cancel()

_, err := coordinator.Write(ctx, testAudit, embedding)

Expect(err).To(HaveOccurred())
Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
```

---

## ✅ Success Criteria Met

**DO-RED Objective**: Expose the bug through failing tests
**Status**: ✅ COMPLETE

**Evidence**:
1. ✅ 6 tests created covering all context scenarios
2. ✅ All 6 tests FAIL as expected
3. ✅ Mock tracks API usage (`Begin()` vs `BeginTx()`)
4. ✅ Tests compile successfully
5. ✅ Failure messages clearly show bug (context ignored)

---

## 🔬 Bug Validation

### Test 016.6 Result (API Usage Verification)

**Current Behavior**:
```go
Expect(mockDB.beginCalled).To(BeFalse(), "Begin() should NOT be called (legacy API)")
// Result: FAILS - Begin() IS being called
```

**Proof**: The production code calls `c.db.Begin()` instead of `c.db.BeginTx(ctx, nil)`, so `mockDB.beginCalled` is `true`.

---

## ⏭️ Next Steps

### Phase 2: DO-GREEN (Fix Implementation)

**Estimated Time**: 15 minutes

**Files to Modify**:
1. **`pkg/datastorage/dualwrite/interfaces.go`**
   - Add `BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error)` to `DB` interface

2. **`pkg/datastorage/dualwrite/coordinator.go`**
   - Line 71: Change `c.db.Begin()` → `c.db.BeginTx(ctx, nil)`
   - Line 234: Change `c.db.Begin()` → `c.db.BeginTx(ctx, nil)`

**Expected Outcome**: ✅ **All 6 context tests PASS**

---

## 💯 Confidence Assessment

**100% Confidence** that DO-RED phase is complete and correct.

**Evidence**:
1. ✅ All 6 tests fail for the right reasons (context ignored)
2. ✅ Mock correctly tracks API usage
3. ✅ Tests cover all context scenarios (cancellation, timeout, deadline, fallback)
4. ✅ Test structure follows TDD best practices
5. ✅ Clear failure messages guide the fix

**Key Insight**: The failing tests **prove the bug exists** and **specify the exact fix needed** (use `BeginTx(ctx, nil)`).

---

## 🔗 Related Documentation

- [KNOWN_ISSUE_001_CONTEXT_PROPAGATION.md](../KNOWN_ISSUE_001_CONTEXT_PROPAGATION.md) - Full issue documentation
- [Day 5 Complete](./05-day5-complete.md) - When bug was introduced
- [Day 7 Complete](./09-day7-complete.md) - When bug was suspected (3 integration tests skipped)
- [IMPLEMENTATION_PLAN_V4.1.md](../IMPLEMENTATION_PLAN_V4.1.md) - Day 9 plan

---

## 📝 Summary

**Objective**: Write failing tests to expose context propagation bug
**Status**: ✅ COMPLETE
**Time**: 20 minutes
**Result**: 6/6 context tests FAIL as expected, bug is exposed

**Achievement**: DO-RED phase validates the bug exists and provides clear specification for the fix. Ready to proceed to DO-GREEN (implementation fix).


