# Webhook Unit Test Triage - Test Plan Mapping

**Date**: January 6, 2026
**Status**: 🔴 **CRITICAL GAPS FOUND**
**Purpose**: Map existing unit tests to WEBHOOK_TEST_PLAN.md test scenarios
**Issue**: Tests lack test case IDs, duplicates found, gaps identified

---

## 🚨 **Problem Statement**

Per `TESTING_GUIDELINES.md` lines 353-369:
> When a test plan document exists, tests SHOULD use test case ID from the plan instead of BR IDs

**Current State**:
- ❌ Tests use business outcome descriptions, not test case IDs
- ❌ Tests don't reference WEBHOOK_TEST_PLAN.md test numbers
- ❌ Some test plan scenarios are missing
- ❌ Some tests have no corresponding test plan entry

---

## 📊 **Test Plan vs Implementation Mapping**

### **Authenticator Tests (`test/unit/authwebhook/authenticator_test.go`)**

| Test Plan ID | Test Plan Description | Implementation Status | Current Test Description |
|--------------|----------------------|---------------------|------------------------|
| **Test 1** | Extract Valid User Info | ✅ IMPLEMENTED (DUPLICATE) | 1. "capture operator identity" <br> 2. "accepts complete operator auth" |
| **Test 2** | Reject Missing Username | ✅ IMPLEMENTED | "rejects request missing username" |
| **Test 3** | Reject Empty UID | ✅ IMPLEMENTED | "rejects request missing UID" |
| **Test 4** | Extract Multiple Groups | ❌ **MISSING** | - |
| **Test 9** | Extract User with No Groups | ❌ **MISSING** | - |
| **Test 10** | Extract Service Account User | ❌ **MISSING** | - |
| **N/A** | Format operator identity | ⚠️ NOT IN PLAN | "format operator identity for audit trail" |
| **N/A** | Reject both username & UID missing | ⚠️ NOT IN PLAN | "rejects request missing both" |
| **N/A** | Reject malformed requests | ⚠️ NOT IN PLAN | "reject malformed webhook requests" |

### **Validator Tests (`test/unit/authwebhook/validator_test.go`)**

| Test Plan ID | Test Plan Description | Implementation Status | Current Test Description |
|--------------|----------------------|---------------------|------------------------|
| **Test 5** | ValidateReason - Accept Valid Input | ✅ IMPLEMENTED | "accepts justification meeting minimum" <br> "accepts detailed operational justification" |
| **Test 6** | ValidateReason - Reject Empty Reason | ✅ IMPLEMENTED | "rejects empty justification" <br> "rejects whitespace-only justification" |
| **Test 7** | ValidateReason - Reject Overly Long Reason | ❌ **MISSING** | - |
| **Test 8** | ValidateReason - Accept Reason at Max Length | ❌ **MISSING** | - |
| **N/A** | Reject vague justification | ⚠️ NOT IN PLAN | "rejects vague justification lacking context" |
| **N/A** | Reject single-word justification | ⚠️ NOT IN PLAN | "rejects single-word non-descriptive" |
| **N/A** | Reject negative minimum words | ⚠️ NOT IN PLAN | "rejects negative minimum" |
| **N/A** | Reject zero minimum words | ⚠️ NOT IN PLAN | "rejects zero minimum" |
| **N/A** | ValidateTimestamp - Accept recent | ⚠️ NOT IN PLAN | "accepts recent legitimate clearance request" |
| **N/A** | ValidateTimestamp - Accept at boundary | ⚠️ NOT IN PLAN | "accepts request at maximum age boundary" |
| **N/A** | ValidateTimestamp - Reject future | ⚠️ NOT IN PLAN | "rejects future timestamp" |
| **N/A** | ValidateTimestamp - Reject slightly future | ⚠️ NOT IN PLAN | "rejects slightly future timestamp" |
| **N/A** | ValidateTimestamp - Reject stale | ⚠️ NOT IN PLAN | "rejects stale request" |
| **N/A** | ValidateTimestamp - Reject very old | ⚠️ NOT IN PLAN | "rejects very old request" |
| **N/A** | ValidateTimestamp - Reject zero | ⚠️ NOT IN PLAN | "rejects zero timestamp" |

---

## 📊 **Coverage Summary**

| Category | Test Plan Tests | Implemented | Missing | Not in Plan |
|----------|----------------|-------------|---------|-------------|
| **Authenticator** | 6 | 3 | 3 | 3 |
| **Validator** | 4 | 2 | 2 | 13 |
| **TOTAL** | **10** | **5** | **5** | **16** |

---

## 🚨 **Critical Issues**

### **Issue 1: Missing Test Case IDs**
**Problem**: Tests don't reference test plan IDs
**Impact**: Can't trace tests to test plan, unclear coverage
**Example**:
```go
// ❌ CURRENT: No test case ID
It("should capture operator identity for audit attribution", func() {

// ✅ SHOULD BE: Test case ID from plan
It("AUTH-001: Extract Valid User Info", func() {  // Maps to Test Plan Test 1
```

### **Issue 2: Tests Not in Test Plan**
**Problem**: 16 tests have no corresponding test plan entry
**Impact**: Tests may be redundant or test plan is incomplete

**Questions**:
1. Should these tests be added to the test plan?
2. Are these tests duplicates of plan tests with different descriptions?
3. Are these edge cases that should be in the plan?

### **Issue 3: Missing Test Plan Scenarios**
**Problem**: 5 test plan scenarios are not implemented
**Impact**: Incomplete test coverage per plan

**Missing Tests**:
1. **Test 4**: Extract Multiple Groups
2. **Test 7**: ValidateReason - Reject Overly Long Reason
3. **Test 8**: ValidateReason - Accept Reason at Max Length
4. **Test 9**: Extract User with No Groups
5. **Test 10**: Extract Service Account User

### **Issue 4: Duplicate Coverage**
**Problem**: Test 1 (Extract Valid User Info) tested twice
**Impact**: Redundant test execution

**Duplicates**:
- Test: "should capture operator identity for audit attribution"
- Table Entry: "accepts complete operator authentication"

---

## 🎯 **Recommended Actions**

### **Option A: Update Tests to Match Test Plan (RECOMMENDED)**

**Rationale**: Test plan is authoritative specification

**Steps**:
1. ✅ Add test case IDs to all existing tests
2. ✅ Remove duplicate tests
3. ✅ Implement 5 missing test plan scenarios
4. ✅ Document why non-plan tests exist (or remove them)

**Example Refactor**:
```go
// Current (business outcome description)
It("should capture operator identity for audit attribution", func() {

// Refactored (test case ID + description)
It("AUTH-001: Extract Valid User Info - captures username, UID, groups", func() {
    // Test plan: Test 1 - Extract Valid User Info
    // BR: BR-AUTH-001 (Operator Attribution)
```

### **Option B: Update Test Plan to Match Tests**

**Rationale**: Tests are more comprehensive than plan

**Steps**:
1. Add 16 new test scenarios to WEBHOOK_TEST_PLAN.md
2. Assign formal test case IDs (AUTH-001 through AUTH-026)
3. Document business rationale for each test
4. Update test code to reference new IDs

**Trade-off**: Significant test plan rewrite required

### **Option C: Create Mapping Document**

**Rationale**: Preserve both, just clarify relationship

**Steps**:
1. Create `WEBHOOK_TEST_MAPPING.md` showing test-to-plan mapping
2. Add comments in test code referencing plan tests
3. Accept that some tests exceed plan scope

**Trade-off**: Doesn't solve traceability problem

---

## 📋 **Proposed Test Case ID Format**

Per `TESTING_GUIDELINES.md` convention:

```
AUTH-XXX: Description
  ├─ AUTH-001: Extract Valid User Info
  ├─ AUTH-002: Reject Missing Username
  ├─ AUTH-003: Reject Empty UID
  ├─ AUTH-004: Extract Multiple Groups
  ├─ AUTH-005: ValidateReason - Accept Valid Input
  └─ ...
```

**Benefits**:
- ✅ Consistent with project convention (e.g., ENRICH-DS-01)
- ✅ Easy to trace test to test plan
- ✅ Centralizes BR mapping in test plan document

---

## 🚀 **Implementation Plan**

### **Phase 1: Add Test Case IDs** (2 hours)
1. Assign AUTH-XXX IDs to test plan tests (Test 1-10)
2. Update test descriptions to include IDs
3. Add comments mapping tests to plan

### **Phase 2: Implement Missing Tests** (3 hours)
1. AUTH-004: Extract Multiple Groups
2. AUTH-007: ValidateReason - Reject Overly Long Reason
3. AUTH-008: ValidateReason - Accept Reason at Max Length
4. AUTH-009: Extract User with No Groups
5. AUTH-010: Extract Service Account User

### **Phase 3: Triage Non-Plan Tests** (2 hours)
1. Evaluate if 16 non-plan tests should be added to plan
2. Remove duplicates
3. Document rationale for keeping extras

### **Phase 4: Update Documentation** (1 hour)
1. Update WEBHOOK_TEST_PLAN.md with formal AUTH-XXX IDs
2. Add mapping section to test plan
3. Update TESTING_GUIDELINES.md example if needed

**Total Estimated Time**: 8 hours

---

## 🎯 **Success Criteria**

- [ ] Every test has a test case ID (AUTH-XXX)
- [ ] All test plan scenarios (Test 1-10) are implemented
- [ ] No duplicate tests
- [ ] Clear mapping between tests and test plan
- [ ] Test plan updated with formal test case IDs
- [ ] All tests passing after refactor

---

## 📚 **References**

- **Authoritative Test Plan**: `docs/development/SOC2/WEBHOOK_TEST_PLAN.md`
- **Testing Guidelines**: `docs/development/business-requirements/TESTING_GUIDELINES.md` lines 353-369
- **Example Pattern**: SignalProcessing uses ENRICH-DS-01, ENRICH-DS-02, etc.

---

## ✅ **Decision Required**

**Question**: Which option should we proceed with?

- **Option A**: Update tests to match plan ✅ **RECOMMENDED**
- **Option B**: Update plan to match tests
- **Option C**: Create mapping document

**Recommendation**: **Option A** - Test plan should be authoritative specification



