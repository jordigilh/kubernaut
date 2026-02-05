# Checkpoint Validation Protocol

## Overview

**Critical**: Supervisor must VERIFY, not trust self-reports. Use tools to gather evidence.

---

## Checkpoint 1: RED Phase (Test Quality)

### Worker Reports
```
"RED phase complete. Tests in test/unit/datastorage/validator_test.go"
```

### Supervisor Verification Steps

**1. Request Evidence**
```
"Provide the complete test file path."
```

**2. Read Test File**
```
Read tool: [test_file_path_from_worker]
```

**3. Verify Checklist**

- [ ] **Tests validate behavior** (NOT implementation):
  ```
  ✅ Good: "should reject workflow when label missing"
  ❌ Bad: "should call Validate() method 3 times"
  ```

- [ ] **Ginkgo/Gomega BDD** framework used:
  ```
  Look for: Describe(), Context(), It(), Expect()
  NOT: func Test...(t *testing.T)
  ```

- [ ] **BR references present**:
  ```
  Look for: BR-XXX-XXX in test descriptions
  ```

- [ ] **Tests currently failing**:
  ```
  Expected in RED phase - implementation doesn't exist yet
  ```

- [ ] **Edge cases covered**:
  ```
  Match edge cases from task brief
  ```

**4. Check for Anti-Patterns**
```bash
# Search for Skip() calls (forbidden)
grep "Skip()" [test_file_path]
# Should return: 0 results
```

### Decision

**✅ PASS**:
```
Tests validated. Quality confirmed:
- Tests validate behavior at lines [X-Y]
- Ginkgo/Gomega framework used
- BR-WORKFLOW-XXX referenced
- No Skip() calls found
- Edge cases covered

Proceed to GREEN phase: minimal implementation + integration in cmd/
```

**❌ FAIL**:
```
Issues found that must be fixed:

1. Lines 45-52: Tests validate implementation logic
   "should call helper 3 times" ← Implementation detail
   Fix: Test behavior instead (e.g., "should cache results")

2. Line 67: Using Skip()
   Fix: Remove Skip() or delete test

3. Missing edge case: [specific case from brief]
   Fix: Add test for this scenario

Do NOT proceed to GREEN until these are fixed.
Re-report when corrected.
```

---

## Checkpoint 2: GREEN Phase (Integration)

### Worker Reports
```
"GREEN phase complete. Implementation in pkg/datastorage/validator.go,
integrated in cmd/datastorage/main.go"
```

### Supervisor Verification Steps

**1. Request Evidence**
```
"Provide:
1. Implementation file path
2. cmd/ file where component is integrated"
```

**2. Read Implementation File**
```
Read tool: [implementation_file_from_worker]
```

Check for:
- Minimal implementation (simple logic)
- Error handling present
- No premature optimization

**3. Verify Integration in cmd/** (CRITICAL)

```bash
# Step A: Identify component constructor from implementation
# Example: If implementation has "func NewValidator()", search for "NewValidator"

# Step B: Search cmd/ for that constructor
grep -r "NewValidator" cmd/ --include="*.go"

# MUST find at least 1 result in cmd/[service]/main.go
```

**4. Read cmd/ Integration File**
```
Read tool: [cmd_file_from_grep_results]
```

Verify:
- Component initialized (e.g., `validator := NewValidator(...)`)
- Component wired into application (e.g., passed to handler)
- Integration is complete (not just imported)

**5. Verify Checklist**

- [ ] Implementation minimal (no complex logic)
- [ ] Integration EXISTS in cmd/ (grep found results)
- [ ] Integration properly wired (initialized + used)
- [ ] No orphaned components
- [ ] Tests passing (ask worker to run: `make test-unit-[service]`)

### Decision

**✅ PASS**:
```
Integration verified:
- grep found: cmd/datastorage/main.go:45
- Read file confirmed initialization at line 45
- Component wired to handler at line 52
- Tests passing

Proceed to REFACTOR phase: enhance code quality
```

**❌ MISSING INTEGRATION**:
```
CRITICAL: Integration not found in cmd/

grep -r "NewValidator" cmd/ returned 0 results

Integration is MANDATORY in GREEN phase - cannot defer to REFACTOR.

Add integration now:
1. Initialize validator in cmd/datastorage/main.go
2. Wire into workflow creation handler
3. Provide updated file location

Do NOT proceed to REFACTOR until integration verified.
```

**⚠️ TOO COMPLEX**:
```
Implementation has sophisticated logic:
- Lines 67-120: Complex caching algorithm (153 lines)
- Lines 200-245: Performance optimization

GREEN should be MINIMAL. Move these enhancements to REFACTOR:
1. Simplify implementation to pass tests
2. Move optimizations to REFACTOR phase
3. Re-report when simplified
```

---

## Checkpoint 3: REFACTOR Phase (Final Review)

### Worker Reports
```
"REFACTOR complete. Code enhanced, ready for final review."
```

### Supervisor Action

**Delegate to review-agent-work skill for comprehensive validation**:

```
"Final review needed for completed work.

Use review-agent-work skill to validate:
- Plan alignment (all acceptance criteria)
- Business requirements (BR mapping)
- TDD compliance (full RED-GREEN-REFACTOR)
- Code quality (no lint errors, proper types)
- Integration (verified in cmd/)
- Build & tests (all passing)

Worker implemented: [summary of work]
Original BR: [BR-XXX-NAME]
"
```

### Based on Review Results

**✅ APPROVED**:
```
Review assessment: All standards met

Work approved. Ready for commit.

[Include completion report]
```

**⚠️ APPROVED WITH WARNINGS**:
```
Review assessment: Functional with warnings

Warnings identified:
- [Non-critical concern]
- [Non-critical concern]

Work approved for commit. Consider addressing warnings in follow-up.
```

**❌ REQUIRES CHANGES**:
```
Review assessment: Blocking issues found

BLOCKING:
1. [Issue from review]
2. [Issue from review]

Worker must fix before approval.
```

---

## Verification Examples

### Example 1: Verifying Behavior vs Implementation Tests

**Read test file and look for**:

✅ **Behavior tests** (Good):
```go
It("should reject workflow when mandatory label missing [BR-WORKFLOW-197]", func() {
    result := validator.Validate(workflowWithoutLabel)
    Expect(result.Valid).To(BeFalse())
    Expect(result.Error).To(ContainSubstring("missing mandatory label"))
})
```

❌ **Implementation tests** (Bad):
```go
It("should call checkMandatoryLabels helper 3 times", func() {
    validator.Validate(workflow)
    Expect(mockHelper.CallCount).To(Equal(3))
})
```

### Example 2: Verifying Integration

**grep results showing integration**:
```bash
$ grep -r "NewValidator" cmd/datastorage/ --include="*.go"
cmd/datastorage/main.go:45:    validator := validator.NewValidator(config)
cmd/datastorage/main.go:52:    handler := workflow.NewHandler(repo, validator)
```

✅ Found 2 references - integration verified

**Read cmd/datastorage/main.go to confirm**:
```go
// Line 45
validator := validator.NewValidator(config)

// Line 52  
handler := workflow.NewHandler(repo, validator)
```

✅ Component initialized and used - integration complete

---

## Checkpoint Rejection Examples

### RED Phase Rejection Example

```
❌ RED phase has issues:

Read: test/unit/datastorage/validator_test.go

Issues found:
1. Lines 45-52: Test validates implementation
   Current: "should call Redis client 3 times"
   Fix: Test behavior - "should cache successful searches"

2. Line 67: Skip() call found
   Fix: Remove Skip() or delete test

3. Missing edge case: No test for empty label value
   Brief required: "validate label format (key=value)"
   Fix: Add test - "should reject workflow when label value empty"

Do NOT proceed to GREEN until fixed.
```

### GREEN Phase Rejection Example

```
❌ GREEN phase missing integration:

grep -r "NewValidator" cmd/ --include="*.go"
Result: No matches found

CRITICAL VIOLATION: Component not integrated in cmd/

You created:
- pkg/datastorage/validator.go (implementation)

But missing:
- Integration in cmd/datastorage/main.go

Add integration NOW (GREEN phase requirement):
1. Initialize: validator := validator.NewValidator(config)
2. Wire to handler: handler := workflow.NewHandler(repo, validator)
3. Report integration location

Do NOT proceed to REFACTOR without integration.
```

---

## Success Indicators

Verification working when:
- ✅ Supervisor reads actual files (not trusting reports)
- ✅ Supervisor runs grep commands (evidence-based)
- ✅ Decisions based on tool results (not assumptions)
- ✅ Specific line numbers cited in feedback
- ✅ Integration explicitly confirmed via grep + file read
