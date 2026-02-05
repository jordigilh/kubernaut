# Communication Templates

## Pre-Delegation Templates

### Supervisor → User: Clarification Request

```markdown
⏸️ CLARIFICATION NEEDED - Cannot proceed with <90% confidence

## BR: [BR-XXX-NAME]

### Area of Uncertainty: [Business Objective | Acceptance Criteria | Technical Approach | Constraints | Edge Cases]

**What I Understand**:
- [Current understanding]

**What Is Unclear**:
1. [Specific question/ambiguity]
2. [Specific question/ambiguity]

**Options I'm Considering**:
A) [Interpretation 1 with implications]
B) [Interpretation 2 with implications]
C) [Interpretation 3 with implications]

**Impact of Ambiguity**:
- [What could go wrong if I guess wrong]

**Request**: Please clarify [specific aspect] so I can proceed with ≥90% confidence.

**Current Confidence**: [X]% (need 90%+ to delegate to worker)
```

---

### Supervisor → Worker: Task Brief Template

```markdown
# Task: [Feature Name] - BR-XXX-[NAME]

## Business Context
**Problem**: [What problem this solves]
**Value**: [Why this matters]

## Acceptance Criteria (Must All Pass)
1. [Specific, testable criterion with validation method]
2. [Specific, testable criterion with validation method]
3. [Specific, testable criterion with validation method]

## Technical Specifications
**Location**: [pkg path]
**Integration**: [cmd path + specific location]
**Patterns**: [Follow pattern from file X]
**Test Coverage**: [X%+ required]

## Constraints & Boundaries
**Performance**: [Specific requirements]
**Compatibility**: [What must not break]
**Breaking Changes**: [Allowed/Not allowed]

## Implementation Plan
Phase 1 - RED (Write Tests):
  [Specific tasks]

Phase 2 - GREEN (Minimal Implementation):
  [Specific tasks + MANDATORY integration in cmd/]

Phase 3 - REFACTOR (Enhancement):
  [Specific tasks]

## Edge Cases to Handle
- [Specific edge case with expected behavior]
- [Specific edge case with expected behavior]

## Checkpoints
Report to supervisor after:
1. RED phase complete (tests written)
2. GREEN phase complete (implementation + integration)  
3. REFACTOR phase complete (enhancements)

## Questions?
Before starting, review this brief and ask supervisor for clarification on ANYTHING unclear.
You must have 90%+ confidence you understand the task before implementing.
```

---

### Supervisor → Worker: Task Assignment

```
Worker: Review this task brief for BR-XXX-[NAME]

[Complete Task Brief from above]

MANDATORY BEFORE STARTING:
1. Read the entire brief carefully
2. Assess your understanding (aim for 90%+ confidence)
3. Ask me clarifying questions about ANYTHING unclear
4. Wait for my permission before starting

Do NOT implement until you confirm 90%+ confidence and I grant permission.
```

---

## Worker Clarification Templates

### Worker → Supervisor: Confidence Assessment

```
I've reviewed the task brief. Here's my understanding assessment:

Confidence Assessment:
- Business Context: [X]%
- Acceptance Criteria: [X]%  
- Technical Specs: [X]%
- Edge Cases: [X]%

Overall: [X]%

Questions:
1. [Specific question]
2. [Specific question]

[If ≥90%]: Ready to start. Requesting permission to begin RED phase.
[If <90%]: Need clarification on above questions before starting.
```

---

## Supervisor Response Templates

### Supervisor → Worker: Answering Questions

```
Answering your questions:

Q1: [Worker's question]
A1: [Clear answer with specifics and examples]

Q2: [Worker's question]  
A2: [Clear answer with specifics and examples]

Does this clarify? What's your confidence now?
```

---

### Supervisor → User: Escalate Worker Question

```
⏸️ ESCALATION - Worker asked question I cannot answer:

Worker Question: [Question]
Context: [Why worker needs this information]
My Understanding: [What I think but not sure]
Uncertainty: [Why I need your input]

Please clarify so I can inform the worker.
```

---

### Supervisor → Worker: Grant Permission

```
✅ Permission granted. Begin RED phase:

Write failing tests that:
- Validate behavior/outcomes (NOT implementation)
- Use Ginkgo/Gomega BDD framework
- Include BR-XXX-XXX references
- Cover all edge cases from brief

Report when RED phase complete.
```

---

## Checkpoint Communication Templates

### Checkpoint 1: RED Phase Feedback

**Approval**:
```
✅ RED phase approved. Tests look good:
- Tests validate behavior not implementation
- Ginkgo/Gomega framework used correctly
- BR references present
- Edge cases covered

Proceed to GREEN phase:
- Implement minimal logic to pass tests
- MUST integrate in cmd/ during GREEN (not deferred)
- Keep implementation simple (enhance in REFACTOR)

Report when GREEN phase complete.
```

**Rejection**:
```
❌ RED phase has issues that must be fixed:

1. Tests validate implementation logic (lines 45-52):
   - "should call helper 3 times" ← Implementation detail
   Fix: Test behavior instead (e.g., "should cache results")

2. Missing edge case tests:
   - No test for empty input handling
   - No test for concurrent access

Fix these issues before proceeding to GREEN phase.
Report when corrected.
```

---

### Checkpoint 2: GREEN Phase Feedback

**Approval**:
```
✅ GREEN phase approved:
- Integration verified in cmd/[service]/main.go
- All tests passing
- Implementation minimal (no premature optimization)

Proceed to REFACTOR phase:
- Extract configuration to separate file
- Add detailed error messages
- Optimize performance if needed

Report when REFACTOR complete.
```

**Missing Integration**:
```
❌ CRITICAL: Integration missing in cmd/

grep -r "NewValidator" cmd/ --include="*.go"
Result: No matches found

You must integrate the component in cmd/[service]/main.go during GREEN phase.
This is MANDATORY - cannot defer to REFACTOR.

Add integration:
1. Initialize component in main.go
2. Wire into request handler
3. Show me the integration code

Do NOT proceed to REFACTOR until integration verified.
```

---

### Checkpoint 3: REFACTOR Phase Feedback

**Approval**:
```
✅ REFACTOR complete and APPROVED:
- All acceptance criteria met
- TDD sequence followed (RED → GREEN → REFACTOR)
- Tests validate behavior not implementation
- Integration verified in cmd/
- Build and tests pass
- Code quality excellent

Work is ready for commit.

Implementation Summary:
- Files changed: [count]
- Tests added: [count] (behavior-focused)
- Integration: Verified in cmd/[service]/main.go
- Build: ✅ Success
- Tests: ✅ All Pass (coverage: [X]%)

Recommendations for future work:
- [Any suggestions]
```

**Requires Changes**:
```
❌ REFACTOR has blocking issues:

BLOCKING (must fix):
1. Build fails with undefined symbol error (file.go:123)
2. Tests validate implementation logic (test_file.go:45-67)

WARNINGS (should address):
- Test coverage 68% (below 70% target)
- Large function in validator.go (150+ lines)

Fix blocking issues before approval.
Report when ready for re-review.
```

---

## Feedback Loop Templates

### Request Corrections

```
Changes needed before approval:

BLOCKING Issues (must fix):
1. [Specific issue with file:line]
   Fix: [How to address it]
   
2. [Specific issue with file:line]
   Fix: [How to address it]

WARNINGS (should address):
- [Non-critical concern]
- [Non-critical concern]

Next Steps:
1. Fix blocking issues
2. Report when ready for re-review
3. I'll re-validate the fixes
```

---

## Completion Templates

### Final Approval Report

```
# Work Completion Report

## Business Requirement
BR-XXX-[NAME] - [Description]

## Implementation Summary
- Files Changed: [count] files
  - [file1.go] (new/modified)
  - [file2.go] (new/modified)
- Tests Added: [count] tests (all behavior-focused)
- Integration: Verified in cmd/[service]/main.go
- Build Status: ✅ Success
- Test Status: ✅ All Pass ([X]% coverage)

## Quality Assessment
- TDD Compliance: ✅ Full RED-GREEN-REFACTOR sequence
- Test Quality: ✅ All tests validate behavior, not implementation
- Integration: ✅ Verified in main application
- Code Quality: ✅ [X]% confidence
- Standards: ✅ All Kubernaut guidelines followed

## Acceptance Criteria Validation
✅ AC-1: [Criterion] - Validated
✅ AC-2: [Criterion] - Validated
✅ AC-3: [Criterion] - Validated

## Ready for Commit
All blocking issues resolved. Work meets acceptance criteria.

Next Steps:
- User can commit changes
- [Any recommendations for future work]
```

---

## Quick Reference: Message Types

| From → To | When | Purpose | Template Section |
|-----------|------|---------|-----------------|
| Supervisor → User | Confidence <90% | Get clarification | Pre-Delegation |
| Supervisor → Worker | After clarification | Assign task | Task Assignment |
| Worker → Supervisor | After task review | Assess confidence | Worker Clarification |
| Supervisor → Worker | Worker ≥90% | Grant permission | Grant Permission |
| Worker → Supervisor | Checkpoint reached | Report progress | (Worker reports) |
| Supervisor → Worker | After checkpoint | Approve/reject | Checkpoint Feedback |
| Supervisor → User | Uncertain validation | Escalate question | Escalate Worker Question |
| Supervisor → User | Work complete | Final report | Completion |
