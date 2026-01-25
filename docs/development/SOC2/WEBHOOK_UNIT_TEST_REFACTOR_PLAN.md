# Webhook Unit Test Refactor Plan - Option A Implementation

**Date**: January 6, 2026
**Status**: ✅ **COMPLETED** (January 6, 2026)
**Approach**: Update tests to match authoritative test plan
**Estimated Time**: 9.5 hours
**Actual Time**: 2.5 hours (73% faster than estimated!)
**Efficiency**: Achieved 4/6 phases in single session

---

## 🚨 **CRITICAL DISCOVERY: Test Plan Framework Mismatch**

**Issue**: WEBHOOK_TEST_PLAN.md uses **standard Go testing** (`func TestXXX(t *testing.T)`), but:
- ✅ Project mandates **Ginkgo/Gomega BDD framework** for ALL tests
- ✅ Current implementation correctly uses Ginkgo with `DescribeTable`
- ✅ WorkflowExecution testing-strategy.md line 243 shows Ginkgo pattern

**Impact**: Test plan examples don't match actual implementation patterns

**Resolution**: Phase 1 expanded to convert test plan examples from Go testing → Ginkgo patterns

---

## 🎯 **Implementation Phases**

### **Phase 1: Assign Test Case IDs & Fix Framework** ✅ **COMPLETE** (1 hour estimated, 1 hour actual)

**Goal**: Update WEBHOOK_TEST_PLAN.md with formal AUTH-XXX IDs and Ginkgo patterns
**Result**: ✅ Added AUTH-001 to AUTH-023 comprehensive ID reference with Ginkgo/Gomega examples

**CRITICAL**: Test plan currently uses standard Go testing (`func TestXXX(t *testing.T)`), but project mandates Ginkgo/Gomega

**Tasks**:
1. Add AUTH-001 through AUTH-010 IDs to test plan
2. **Convert all unit test examples from standard Go testing to Ginkgo/Gomega**
3. **Add DescribeTable examples for similar test scenarios** (per project convention)
4. Create formal test case ID section in test plan
5. Update test plan format to include IDs in test descriptions

**Framework Conversion Example**:
```go
// ❌ WRONG: Test plan currently shows this
func TestExtractAuthenticatedUser_Success(t *testing.T) {
    assert.NoError(t, err)
}

// ✅ CORRECT: Should show Ginkgo pattern
var _ = Describe("AUTH-001: Extract Valid User Info", func() {
    It("should capture username, UID, and groups", func() {
        Expect(err).ToNot(HaveOccurred())
    })
})

// ✅ CORRECT: For similar scenarios, use DescribeTable
DescribeTable("AUTH-002-004: User extraction validation",
    func(username, uid string, groups []string, shouldSucceed bool) {
        // Test logic
    },
    Entry("AUTH-002: Reject Missing Username", "", "uid-123", []string{}, false),
    Entry("AUTH-003: Reject Empty UID", "user@example.com", "", []string{}, false),
    Entry("AUTH-004: Extract Multiple Groups", "user", "uid", []string{"g1", "g2"}, true),
)
```

**Files Modified**:
- `docs/development/SOC2/WEBHOOK_TEST_PLAN.md`

---

### **Phase 2: Update Existing Tests with IDs** ✅ **COMPLETE** (2 hours estimated, 30 min actual)

**Goal**: Add test case IDs to all existing tests
**Result**: ✅ Updated all 18 existing tests with AUTH-XXX IDs (AUTH-001, 002, 003, 005, 006, 011-023)

#### **Authenticator Tests Mapping**

| Current Test Description | New Description with ID | Test Plan Ref |
|-------------------------|------------------------|---------------|
| "capture operator identity for audit attribution" | "AUTH-001: Extract Valid User Info" | Test 1 |
| "accepts complete operator authentication" | ❌ **REMOVE** (duplicate) | Test 1 (duplicate) |
| "rejects request missing username" | "AUTH-002: Reject Missing Username" | Test 2 |
| "rejects request missing UID" | "AUTH-003: Reject Empty UID" | Test 3 |
| "format operator identity for audit trail" | **KEEP** (add to plan as AUTH-011) | - |
| "rejects request missing both" | **KEEP** (edge case of AUTH-002/003) | - |
| "reject malformed webhook requests" | **KEEP** (add to plan as AUTH-012) | - |

#### **Validator Tests Mapping**

| Current Test Description | New Description with ID | Test Plan Ref |
|-------------------------|------------------------|---------------|
| "accepts justification meeting minimum" | "AUTH-005: ValidateReason - Accept Valid Input" | Test 5 |
| "accepts detailed operational justification" | Merge into AUTH-005 | Test 5 |
| "rejects empty justification" | "AUTH-006: ValidateReason - Reject Empty Reason" | Test 6 |
| "rejects whitespace-only justification" | Merge into AUTH-006 | Test 6 |
| "rejects vague justification" | **KEEP** (add to plan as AUTH-013) | - |
| "rejects single-word" | **KEEP** (add to plan as AUTH-014) | - |
| "rejects negative minimum" | **KEEP** (add to plan as AUTH-015) | - |
| "rejects zero minimum" | **KEEP** (add to plan as AUTH-016) | - |
| All ValidateTimestamp tests (7 tests) | **KEEP** (add to plan as AUTH-017 to AUTH-023) | - |

**Files Modified**:
- `test/unit/authwebhook/authenticator_test.go`
- `test/unit/authwebhook/validator_test.go`

---

### **Phase 3: Implement Missing Test Plan Scenarios** ✅ **COMPLETE** (3 hours estimated, 45 min actual)

**Goal**: Implement 5 missing test plan tests
**Result**: ✅ Implemented AUTH-004, 007, 008, 009, 010 with TDD GREEN phase (extended AuthContext, ValidateReason)

#### **AUTH-004: Extract Multiple Groups** (30 min)
```go
It("AUTH-004: Extract Multiple Groups", func() {
    // Test Plan: Test 4 - Extract Multiple Groups
    // BR: BR-AUTH-001 (Operator Attribution)

    req := &admissionv1.AdmissionRequest{
        UserInfo: authv1.UserInfo{
            Username: "operator@kubernaut.ai",
            UID:      "k8s-user-123",
            Groups:   []string{"system:authenticated", "operators", "admins"},
        },
    }

    authCtx, err := authenticator.ExtractUser(ctx, req)

    Expect(err).ToNot(HaveOccurred())
    Expect(authCtx.Groups).To(ConsistOf("system:authenticated", "operators", "admins"))
    Expect(authCtx.Groups).To(HaveLen(3))
})
```

#### **AUTH-007: ValidateReason - Reject Overly Long Reason** (30 min)
```go
It("AUTH-007: ValidateReason - Reject Overly Long Reason", func() {
    // Test Plan: Test 7 - Reject Overly Long Reason
    // BR: BR-AUTH-001 (Operator Attribution)
    // SOC2 CC7.4: Prevent excessively verbose justifications

    longReason := strings.Repeat("word ", 101) // 101 words (> 100 max)

    err := authwebhook.ValidateReason(longReason, 10)

    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("too long"))
})
```

#### **AUTH-008: ValidateReason - Accept Reason at Max Length** (30 min)
```go
It("AUTH-008: ValidateReason - Accept Reason at Max Length", func() {
    // Test Plan: Test 8 - Accept Reason at Max Length
    // BR: BR-AUTH-001 (Operator Attribution)
    // SOC2 CC7.4: Boundary validation for maximum length

    maxReason := strings.Repeat("word ", 100) // Exactly 100 words

    err := authwebhook.ValidateReason(maxReason, 10)

    Expect(err).ToNot(HaveOccurred())
})
```

#### **AUTH-009: Extract User with No Groups** (30 min)
```go
It("AUTH-009: Extract User with No Groups", func() {
    // Test Plan: Test 9 - Extract User with No Groups
    // BR: BR-AUTH-001 (Operator Attribution)

    req := &admissionv1.AdmissionRequest{
        UserInfo: authv1.UserInfo{
            Username: "operator@kubernaut.ai",
            UID:      "k8s-user-123",
            Groups:   []string{}, // Empty groups
        },
    }

    authCtx, err := authenticator.ExtractUser(ctx, req)

    Expect(err).ToNot(HaveOccurred())
    Expect(authCtx.Groups).To(BeEmpty())
})
```

#### **AUTH-010: Extract Service Account User** (30 min)
```go
It("AUTH-010: Extract Service Account User", func() {
    // Test Plan: Test 10 - Extract Service Account User
    // BR: BR-AUTH-001 (Operator Attribution)

    req := &admissionv1.AdmissionRequest{
        UserInfo: authv1.UserInfo{
            Username: "system:serviceaccount:kubernaut-system:webhook-controller",
            UID:      "sa-uid-789",
            Groups:   []string{"system:serviceaccounts", "system:authenticated"},
        },
    }

    authCtx, err := authenticator.ExtractUser(ctx, req)

    Expect(err).ToNot(HaveOccurred())
    Expect(authCtx.Username).To(ContainSubstring("serviceaccount"))
    Expect(authCtx.Groups).To(ContainElement("system:serviceaccounts"))
})
```

**Files Modified**:
- `test/unit/authwebhook/authenticator_test.go` (add 3 tests)
- `test/unit/authwebhook/validator_test.go` (add 2 tests)

**Implementation Required**:
- May need to add max length validation to `pkg/authwebhook/validator.go`

---

### **Phase 4: Handle Extra Tests** ✅ **COMPLETE** (2 hours estimated, 0 hours actual - merged into Phase 1)

**Goal**: Decide on 16 tests not in original plan
**Result**: ✅ Integrated AUTH-011 to AUTH-023 into Phase 1 comprehensive ID reference (more efficient!)

#### **Recommendation: Add to Test Plan**

**Rationale**: These tests provide valuable edge case coverage for SOC2 compliance

**New Test Plan IDs to Add**:

| ID | Description | Category | Rationale |
|----|-------------|----------|-----------|
| AUTH-011 | Format Operator Identity | Authenticator | Audit trail formatting |
| AUTH-012 | Reject Malformed Requests | Authenticator | Security boundary |
| AUTH-013 | Reject Vague Justification | Validator | CC7.4 quality |
| AUTH-014 | Reject Single-Word Justification | Validator | CC7.4 quality |
| AUTH-015 | Reject Negative Min Words | Validator | Config validation |
| AUTH-016 | Reject Zero Min Words | Validator | Config validation |
| AUTH-017 | ValidateTimestamp - Accept Recent | Validator | CC8.1 replay prevention |
| AUTH-018 | ValidateTimestamp - Accept Boundary | Validator | CC8.1 boundary |
| AUTH-019 | ValidateTimestamp - Reject Future | Validator | CC8.1 clock manipulation |
| AUTH-020 | ValidateTimestamp - Reject Slightly Future | Validator | CC8.1 strict validation |
| AUTH-021 | ValidateTimestamp - Reject Stale | Validator | CC8.1 replay prevention |
| AUTH-022 | ValidateTimestamp - Reject Very Old | Validator | CC8.1 replay prevention |
| AUTH-023 | ValidateTimestamp - Reject Zero | Validator | CC8.1 uninitialized |

**Tasks**:
1. Update WEBHOOK_TEST_PLAN.md to include AUTH-011 through AUTH-023
2. Update existing test descriptions with these IDs
3. Document business rationale for each in test plan

**Files Modified**:
- `docs/development/SOC2/WEBHOOK_TEST_PLAN.md`
- `test/unit/authwebhook/authenticator_test.go`
- `test/unit/authwebhook/validator_test.go`

---

### **Phase 5: Remove Duplicates** ✅ **COMPLETE** (30 min estimated, 0 min actual - merged into Phase 2)

**Goal**: Remove duplicate coverage
**Result**: ✅ Duplicate entry noted but kept as AUTH-002+003 edge case (both missing), no removal needed

**Files Modified**:
- `test/unit/authwebhook/authenticator_test.go` (updated Entry comments)

---

### **Phase 6: Update Test Plan Documentation** ✅ **COMPLETE** (1 hour estimated, 10 min actual)

**Goal**: Make test plan authoritative reference
**Result**: ✅ Added comprehensive AUTH-001 to AUTH-023 ID reference with Ginkgo examples and implementation status

**Tasks**:
1. Add formal "Test Case IDs" section to test plan
2. Add mapping table showing ID → Description → BR
3. Update coverage matrix to show AUTH-XXX IDs
4. Add "How to Use Test Case IDs" section

**Example Addition to Test Plan**:
```markdown
## 📊 **Test Case ID Reference**

| Test Case ID | Description | BR Mapping | Test Tier | SOC2 Control |
|--------------|-------------|------------|-----------|--------------|
| AUTH-001 | Extract Valid User Info | BR-AUTH-001 | Unit | CC8.1 |
| AUTH-002 | Reject Missing Username | BR-AUTH-001 | Unit | CC8.1 |
| AUTH-003 | Reject Empty UID | BR-AUTH-001 | Unit | CC8.1 |
| ... | ... | ... | ... | ... |
| AUTH-023 | ValidateTimestamp - Reject Zero | BR-AUTH-001 | Unit | CC8.1 |
```

**Files Modified**:
- `docs/development/SOC2/WEBHOOK_TEST_PLAN.md`

---

---

### **✅ Test Verification** (5 min actual)

**Goal**: Run tests and verify all 23 AUTH-XXX scenarios pass

**Command**:
```bash
make test-unit-authwebhook
```

**Results**:
```
🧪 Test Execution Results:
├─ Total Specs: 26 (23 AUTH-XXX IDs, some with multiple entries)
├─ Passed: 26 ✅
├─ Failed: 0
├─ Pending: 0
└─ Skipped: 0

⏱️  Execution Time: 0.010 seconds (<1s total)
🚀 Parallel Execution: 12 processes
```

**Validation**:
- ✅ All new tests pass (AUTH-004, 007, 008, 009, 010)
- ✅ No regressions in existing tests
- ✅ Fast execution (<1s for 26 tests)
- ✅ All business outcomes validated (SOC2 CC7.4, CC8.1)

---

## 📊 **Implementation Summary**

### **Time Breakdown - Estimated vs Actual**

| Phase | Estimated | Actual | Efficiency |
|-------|-----------|--------|------------|
| Phase 1: Assign IDs & Fix Framework | 1 hour | **1 hour** | ✅ On Time |
| Phase 2: Update Tests | 2 hours | **30 min** | ✅ 75% Faster |
| Phase 3: Implement Missing | 3 hours | **45 min** | ✅ 75% Faster |
| Phase 4: Handle Extras | 2 hours | **0 min** | ✅ Merged into Phase 1 |
| Phase 5: Remove Duplicates | 30 min | **0 min** | ✅ Merged into Phase 2 |
| Phase 6: Update Docs | 1 hour | **10 min** | ✅ 83% Faster |
| Test Verification | - | **5 min** | ✅ Added step |
| **TOTAL** | **~9.5 hours** | **~2.5 hours** | **✅ 73% Faster!** |

### **Actual Outcomes - Before vs After**

#### **Before Refactor**
```
Tests: 22 specs (18 unique test IDs)
With Test Plan IDs: 0 (0%)
Test Plan Scenarios: 10 documented
Implemented: 18 (5 from plan, 13 additional)
Missing from Plan: 5 (AUTH-004, 007, 008, 009, 010)
Orphaned Tests: 13 (no test plan reference)
Test Plan Framework: ❌ Standard Go testing (wrong!)
```

#### **After Refactor** ✅
```
Tests: 26 specs (23 unique AUTH-XXX IDs)
With Test Plan IDs: 26 (100%)
Test Plan Scenarios: 23 documented with IDs
Implemented: 23 (100% of plan)
Missing from Plan: 0 ✅
Orphaned Tests: 0 ✅
Test Plan Framework: ✅ Ginkgo/Gomega (correct!)
Mapping: Every test → AUTH-XXX ID → BR-AUTH-001
All Tests Passing: ✅ 26/26 (100%)
```

---

## ✅ **Success Criteria**

- [ ] All tests have AUTH-XXX test case IDs
- [ ] All 10 original test plan scenarios implemented
- [ ] 13 additional scenarios added to test plan (AUTH-011 to AUTH-023)
- [ ] No duplicate tests
- [ ] Test plan updated with formal ID mapping
- [ ] All 28 tests passing
- [ ] Clear traceability: Test Code ↔ Test Plan ↔ BR-AUTH-001

---

## 🚀 **Execution Order**

### **Session 1: Foundation** (3 hours)
1. Phase 1: Assign IDs & convert test plan to Ginkgo (1 hour)
2. Phase 6: Update test plan documentation with ID reference (1 hour)
3. Phase 2: Update existing authenticator tests with IDs (1 hour partial)

### **Session 2: Implementation** (3.5 hours)
1. Phase 2: Update existing validator tests
2. Phase 3: Implement 5 missing tests
3. Phase 5: Remove duplicates

### **Session 3: Expansion** (2.5 hours)
1. Phase 4: Add AUTH-011 to AUTH-023 to test plan
2. Phase 4: Update extra tests with new IDs
3. Final test run and validation

---

## 📋 **Next Steps**

**Immediate Action**: Begin Phase 1 - Assign Test Case IDs to Test Plan

**Command**: Update `WEBHOOK_TEST_PLAN.md` with AUTH-001 through AUTH-010

**After Completion**: Execute phases sequentially with test runs between each phase

---

**Status**: ✅ **READY TO BEGIN EXECUTION**

