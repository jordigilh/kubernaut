# Data Storage Plan V4.5 - Test Package Naming Correction

**Date**: November 2, 2025  
**Issue**: GAP-07 documentation contained incorrect examples  
**Status**: ✅ **FIXED**  
**Confidence**: 100%

---

## 🚨 **Issue Identified**

**User Feedback**: "this package name is wrong" - referring to `package datastorage_test` for integration tests

**Problem**: Implementation plan V4.5 GAP-07 documentation contained **contradictory** and **incorrect** examples for test package naming.

### **Incorrect Statement (V4.5 Initial)**

```
❌ WRONG:
"When to use `_test` suffix: ONLY for integration/E2E tests in separate directories:
test/integration/datastorage/suite_test.go → package datastorage_test  ✅"
```

**Error**: This suggested using `package datastorage_test` for integration tests, which is WRONG.

---

## ✅ **Corrected Rule**

### **Kubernaut Project Convention**

**Rule**: **ALL tests use the SAME package name as production code** (white-box testing)

**No Exceptions**: This applies to:
- ✅ Unit tests (same directory as production code)
- ✅ Integration tests (separate directory: `test/integration/`)
- ✅ E2E tests (separate directory: `test/e2e/`)

### **Correct Examples**

#### **Unit Tests (Same Directory)**
```go
// File: pkg/datastorage/validator.go
package datastorage

// File: pkg/datastorage/validator_test.go
package datastorage  // ✅ CORRECT: Same package as production code
```

#### **Integration Tests (Separate Directory)**
```go
// File: pkg/datastorage/validator.go
package datastorage

// File: test/integration/datastorage/suite_test.go
package datastorage  // ✅ CORRECT: Same package, even though different directory
```

#### **E2E Tests (Separate Directory)**
```go
// File: pkg/datastorage/validator.go
package datastorage

// File: test/e2e/datastorage/workflow_test.go
package datastorage  // ✅ CORRECT: Same package, even though different directory
```

### **NEVER Use `_test` Suffix**

```go
❌ WRONG:
package datastorage_test  // NEVER use this in Kubernaut project

✅ CORRECT:
package datastorage  // ALWAYS use this, regardless of test location
```

---

## 🔧 **Corrections Applied**

### **Files Updated**

**File**: `docs/services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V4.5.md`

**Changes**: 7 corrections made

| Line | Original (Wrong) | Corrected (Right) |
|------|-----------------|-------------------|
| 83 | `Integration/E2E tests use _test suffix (black-box)` | `Even integration/E2E tests use same package name` |
| 474 | `test/.../suite_test.go → package datastorage_test ✅` | `test/.../suite_test.go → package datastorage_test ❌ WRONG` |
| 476 | Added new example showing correct: `→ package datastorage ✅` | |
| 841 | `package datastorage_test // ← Black-box testing...` | `package datastorage // ← Same package as production code...` |
| 1069 | `package datastorage_test` | `package datastorage // ← Same package...` |
| 1856 | `Use wrong test package names - Violates convention (white-box vs black-box)` | `Use _test suffix for test packages - Violates convention (always same package)` |
| 1879 | `Follow test naming convention - Same package for unit tests (white-box)` | `Use same package name for ALL tests - package datastorage for unit, integration, E2E` |

---

## 📝 **Updated GAP-07 Documentation**

### **Before (Incorrect)**

```markdown
**Rule**: Same directory = same package name (white-box testing)
**Exception**: Integration/E2E tests use `_test` suffix (black-box)

test/integration/datastorage/suite_test.go → package datastorage_test  ✅
```

### **After (Correct)**

```markdown
**Rule**: ALL tests use the SAME package name as production code (white-box testing)
**No Exceptions**: Even integration/E2E tests in separate directories use same package name

**✅ Correct (Integration Tests - Separate Directory)**:
pkg/datastorage/validator.go                → package datastorage
test/integration/datastorage/suite_test.go  → package datastorage  ✅ (same package, different directory)

**❌ WRONG - Never Use `_test` Suffix**:
test/integration/datastorage/suite_test.go → package datastorage_test  ❌ WRONG
```

---

## 🎓 **Why This Convention?**

### **White-Box Testing Benefits**

1. **Access Internal Functions**: Tests can access unexported (internal) functions, types, and variables
2. **Simplified Imports**: No need to import your own package in tests
3. **Consistency**: Same pattern across all test types (unit, integration, E2E)
4. **Tooling**: Better IDE support and test coverage reporting

### **Example: Why Same Package Matters**

```go
// pkg/datastorage/validator.go
package datastorage

// unexported (internal) function
func sanitizeInput(s string) string {
    return strings.TrimSpace(s)
}

// exported function
func Validate(audit *Audit) error {
    audit.Name = sanitizeInput(audit.Name)  // Uses internal function
    // ... validation logic ...
}
```

```go
// test/integration/datastorage/validator_test.go

// ❌ WRONG: Using _test suffix prevents testing internal functions
package datastorage_test

func TestValidate(t *testing.T) {
    // Can't test sanitizeInput() directly - it's unexported!
    // Can only test Validate() (exported)
}

// ✅ CORRECT: Same package allows testing internal functions
package datastorage

func TestSanitizeInput(t *testing.T) {
    // Can test sanitizeInput() directly - same package!
    result := sanitizeInput("  test  ")
    assert.Equal(t, "test", result)
}

func TestValidate(t *testing.T) {
    // Can also test Validate() (exported)
    err := Validate(&Audit{Name: "  test  "})
    // ...
}
```

---

## 📊 **Context API Precedent**

**Historical Context**: During Context API migration, the agent initially used `package contextapi_test` for integration tests.

**User Correction**: "this is not following the project's naming convention"

**Resolution**: Changed to `package contextapi` for all tests (unit, integration, E2E).

**Lesson Applied**: This correction ensures Data Storage implementation follows the established project pattern from day 1.

---

## ✅ **Verification Checklist**

Verified that **all** test examples in V4.5 plan now follow correct convention:

- [x] Line 258: Unit test example uses `package datastorage` ✅
- [x] Line 469: Integration test example uses `package datastorage` ✅
- [x] Line 476: Wrong example explicitly marked as ❌ WRONG
- [x] Line 841: Infrastructure setup uses `package datastorage` ✅
- [x] Line 1069: Behavior + Correctness test uses `package datastorage` ✅
- [x] Line 1856: Common pitfall correctly describes issue ✅
- [x] Line 1879: "Do This Instead" shows correct pattern ✅

**Status**: ✅ **ALL INSTANCES CORRECTED**

---

## 🎯 **Summary**

**Issue**: GAP-07 documentation initially suggested using `package datastorage_test` for integration tests  
**Root Cause**: Misunderstanding of project convention (no black-box testing pattern in Kubernaut)  
**Correction**: All 7 instances updated to show `package datastorage` for ALL tests  
**Verification**: Context API pattern confirmed, all examples now consistent  
**Impact**: Prevents developers from using wrong package naming during implementation  

**Confidence**: 100% (based on user feedback and Context API precedent)  
**Status**: ✅ **RESOLVED** - Implementation Plan V4.5 now accurate

