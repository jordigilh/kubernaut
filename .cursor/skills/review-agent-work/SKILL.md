---
name: review-agent-work
description: Reviews and validates work completed by another agent against the original plan and Kubernaut project guidelines. Checks TDD compliance, business requirement mapping, integration patterns, and code quality. Use when verifying agent work, checking plan adherence, or validating implementation completeness.
---

# Agent Work Review

Validates completed work against the execution plan and Kubernaut core development standards to ensure alignment and quality.

## When to Use

- After another agent completes implementation work
- Before marking tasks as complete
- When verifying plan adherence
- Before committing or submitting changes

## Review Process

### Step 1: Understand the Context

**MANDATORY**: Achieve 100% confidence in understanding what needs to be reviewed.

1. **Read the plan/task description**: Understand the original goals, acceptance criteria, and approach
2. **Identify changed files**: Review what was actually modified
3. **Check for TODO items**: Look for any task tracking that indicates scope

**CRITICAL: Ask for user input when ANYTHING is unknown, unclear, or uncertain**:
- The plan is unclear or missing key details
- Changed files don't match expected scope
- There are conflicting requirements
- Business requirements are ambiguous
- Technical approach is questionable
- Expected behavior is not documented
- You're unsure about ANY aspect of what to validate

**DO NOT proceed to Step 2 until you have 100% confidence in what needs to be checked.**

**DO NOT make assumptions. When in doubt, ASK.**

---

### Step 2: Execute Review Checklist

**IMPORTANT: Stop and ask for input if you encounter ANY uncertainty during the review**:
- You cannot determine if a requirement is met
- Expected behavior is unclear
- Test coverage adequacy is ambiguous
- Integration points are uncertain
- You're making assumptions to complete the checklist

**When uncertain: PAUSE the review and ask the user for clarification.**

Copy this checklist and verify each item:

```
## Review Checklist

### Plan Alignment
- [ ] All planned tasks/features implemented
- [ ] No unplanned changes or scope creep
- [ ] Original acceptance criteria met
- [ ] Edge cases from plan addressed

### Business Requirements (Kubernaut Core)
- [ ] Business requirement(s) identified (BR-[CATEGORY]-[NUMBER])
- [ ] Code changes map to documented business needs
- [ ] No speculative or "nice to have" code without BR backing

### TDD Compliance
- [ ] Tests written first (RED phase evidence)
- [ ] Tests validate **business outcomes/behavior**, NOT implementation details
- [ ] Minimal implementation passes tests (GREEN phase)
- [ ] Code enhanced/refactored (REFACTOR phase)
- [ ] Ginkgo/Gomega BDD framework used (not standard Go testing)
- [ ] Test descriptions include BR or test scenario IDs
- [ ] NO Skip() or pending tests present

### Code Quality
- [ ] All errors handled and logged
- [ ] No new lint or compilation errors
- [ ] Structured types used (avoid `any`/`interface{}`)
- [ ] Error wrapping includes context
- [ ] No unused/dead code introduced

### Integration (Kubernaut Critical)
- [ ] Business code integrated with main applications (cmd/)
- [ ] Type definitions validated before field access
- [ ] No orphaned components or unused code
- [ ] Existing patterns enhanced (not reinvented)

### Build & Test Validation
- [ ] Code builds without errors (`go build ./...`)
- [ ] Tests pass (`go test ./...`)
- [ ] No refactoring artifacts left behind (old field names, types)

### Confidence Assessment Provided
- [ ] Confidence percentage provided (60-100%)
- [ ] Justification includes risks and assumptions
- [ ] Validation approach documented
```

---

### Step 3: Generate Review Report

Provide output in this format:

```markdown
# Agent Work Review Results

## Context
- **Task/Plan**: [Brief description]
- **Files Changed**: [Count and key files]
- **Reviewer Confidence**: [Percentage]%

## Checklist Results
[Pass/Fail status for each section from checklist above]

### Plan Alignment: ✅ PASS / ❌ FAIL
- [Specific findings]

### Business Requirements: ✅ PASS / ❌ FAIL
- [Specific findings]

### TDD Compliance: ✅ PASS / ❌ FAIL
- [Specific findings]

### Code Quality: ✅ PASS / ⚠️ WARNINGS / ❌ FAIL
- [Specific findings]

### Integration: ✅ PASS / ❌ FAIL
- [Specific findings]

### Build & Test Validation: ✅ PASS / ❌ FAIL
- [Specific findings]

## Overall Assessment

**Status**: ✅ APPROVED / ⚠️ APPROVED WITH WARNINGS / ❌ REQUIRES CHANGES

**Blocking Issues** (must fix):
- [List any critical issues]

**Warnings** (should address):
- [List any non-critical concerns]

**Confidence Score**: [60-100]%
**Rationale**: [Explain confidence level and any uncertainties]

## Next Steps
- [What needs to happen next - fix issues, proceed to commit, etc.]
```

---

## Quick Reference: Kubernaut Core Rules

### TDD RED-GREEN-REFACTOR
- **RED**: Write failing tests defining business contract
- **GREEN**: Minimal implementation + mandatory main app integration
- **REFACTOR**: Enhance existing code with sophisticated logic

### Business Integration
Business code MUST be integrated in main applications (`cmd/`):
```bash
# Verify integration
grep -r "[NewComponent]" cmd/ --include="*.go"
```

### Common Violations to Check
- ❌ Tests using Skip() to avoid failures
- ❌ Implementation without tests (RED phase skipped)
- ❌ Tests validating implementation logic instead of business behavior/outcomes
- ❌ Sophisticated logic in GREEN phase (should be in REFACTOR)
- ❌ New types not integrated with main apps
- ❌ Struct field references without type validation
- ❌ Refactoring without build validation

---

## Tool Usage Examples

### Check for business requirement mapping
```bash
rg "BR-[A-Z]+-[0-9]+" [test_file] [implementation_file]
```

### Verify main application integration
```bash
grep -r "ComponentName\|NewType" cmd/ --include="*.go"
```

### Validate build after refactoring
```bash
go build ./...
go test ./... -run=^$ -timeout=30s  # Quick compile check
```

### Check for Skip() usage
```bash
rg "Skip\(\)" test/ --include="*_test.go"
```

---

## Escalation Criteria

Stop the review and escalate to the user if:

1. **Plan is unclear or incomplete** - Cannot determine acceptance criteria
2. **Major architectural changes** - Scope significantly exceeds plan
3. **Critical safety issues** - Security vulnerabilities, data loss risks
4. **Complete TDD violation** - No tests written, all tests skipped
5. **Orphaned business code** - New components not integrated anywhere
6. **Confidence below 80%** - Uncertainty about correctness or completeness
7. **ANY uncertainty or ambiguity** - When you need input to complete the review

---

## When to Ask for User Input

**MANDATORY: Ask for user input when:**

### During Initial Assessment (Step 1)
- Plan/task description is vague or incomplete
- Acceptance criteria are not clearly defined
- Business requirements are ambiguous or missing
- Scope of work is unclear

### During Review Execution (Step 2)
- You cannot verify a checklist item without making assumptions
- Expected behavior is not documented
- Test coverage adequacy is subjective/unclear
- Integration patterns are uncertain
- You're unsure if something meets the standard

### During Report Generation (Step 3)
- Confidence is below 80% and you need clarification
- You're uncertain about severity (blocking vs. warning)
- Recommendations depend on project context you don't have

### General Rule
**If you find yourself thinking "I assume..." or "Probably..." → STOP and ASK**

**Examples of when to ask**:
- "I assume this component will be used in X service, but I don't see the integration. Should I flag this?"
- "The test coverage is 65%. Is this acceptable given the component type?"
- "This pattern differs from existing code. Is this intentional or should I flag it?"
- "I'm not sure if this qualifies as a critical security issue. Can you confirm?"

**DO NOT**:
- Make assumptions and proceed
- Guess at user intent
- Apply arbitrary thresholds without context
- Complete review with significant uncertainties

**ALWAYS prefer asking over guessing.**

---

## Anti-Patterns in Reviews

### ❌ Rubber-Stamping
Don't just check boxes without validating. Actually verify each item.

### ❌ Assuming Without Checking
Don't assume main app integration exists. Use `grep` to verify.

### ❌ Ignoring Failed Checks
Don't mark APPROVED if critical items fail. Be honest about blocking issues.

### ❌ Vague Feedback
Don't say "looks good" or "needs work". Provide specific, actionable findings.

### ❌ Making Assumptions
Don't proceed with assumptions when uncertain. Ask for clarification instead.

---

## Success Criteria

A successful review:
- ✅ Provides clear pass/fail status for each area
- ✅ Identifies specific blocking issues vs. warnings
- ✅ Includes confidence assessment with rationale
- ✅ Gives actionable next steps
- ✅ Catches critical Kubernaut guideline violations
- ✅ Verifies plan adherence completely
