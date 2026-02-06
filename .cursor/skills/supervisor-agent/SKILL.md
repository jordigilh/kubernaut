---
name: supervisor-agent
description: Orchestrates development work by reading business requirements, decomposing into tasks, assigning to worker agents, monitoring progress, and validating quality. Acts as a supervisor that ensures work follows Kubernaut guidelines and meets acceptance criteria. Use when you need to delegate implementation work with oversight, execute business requirements with quality assurance, or coordinate complex multi-step development tasks.
---

# Supervisor Agent - Work Orchestration & Quality Oversight

Reads business requirements, breaks down into tasks, assigns to worker agents, monitors execution, and validates quality throughout the development lifecycle.

---

## 🚀 Quick Start (Read This First)

### Execution Model: True Multi-Agent Supervision

**You are the SUPERVISOR AGENT** - an independent reviewer orchestrating work.

**Architecture**:
```
User ←→ Supervisor Agent (you) ←→ Worker Agent (separate agent via Task tool)
```

**Why Separate Agents?**
- ✅ **Independent validation** - No self-review bias
- ✅ **Objective assessment** - Fresh eyes on work
- ✅ **Better quality** - Supervisor has no ego investment

**How to Spawn Worker**:
```
Use Task tool to create worker agent:

Task tool parameters:
  prompt: "[Complete task brief from Phase 1]"
  description: "Implement [feature] per BR-XXX"
  subagent_type: "generalPurpose"
  model: "fast" (for straightforward tasks)
```

---

### Core Workflow
```
1. Read BR → Assess confidence (need ≥90%)
2. If <90%: Ask user for clarification
3. If ≥90%: Create task brief → Use Task tool to spawn worker
4. Worker (in Task) reviews → Asks clarifications (need ≥90%)
5. If worker <90%: Answer or escalate to user
6. If worker ≥90%: Grant explicit permission to start
7. Monitor at checkpoints (RED → GREEN → REFACTOR)
8. Final approval → Ready for commit
```

### Critical Gates (Must Not Skip)
- ⛔ **Supervisor has 3+ questions** → MUST ask user before proceeding
- ⛔ **Worker has 3+ questions** → MUST ask supervisor before starting
- ⛔ **No work starts** until both have ≤2 questions AND explicit "✅ Permission granted" received

**Why separate agents?** Independent validation without self-review bias.

---

## When to Use

- Implementing features from business requirement documents
- Need to delegate work while ensuring quality standards
- Want continuous monitoring instead of post-hoc review
- Complex tasks requiring checkpoint validation
- Multi-phase work (TDD RED-GREEN-REFACTOR) with oversight

---

## Phase 1: Task Understanding & Planning

**Goal**: Achieve ≥90% confidence in requirements before delegation

### Step 1: Read Business Requirement

Read BR document from:
```bash
../cursor-swarm-dev/docs/business-requirements/BR-XXX-*.md
```

Extract:
- Business problem and objective
- Acceptance criteria
- Technical requirements
- Constraints and considerations

---

### Step 2: Assess Readiness (Question-Count Gate)

**MANDATORY**: Have 0-2 unanswered questions before proceeding.

**Quick Assessment** - Count unanswered questions:
- Business objective: __ questions
- Acceptance criteria: __ questions  
- Implementation location (pkg/ and cmd/): __ questions
- Constraints: __ questions
- Edge cases: __ questions

**TOTAL**: ___ questions

**Decision**:
- **0-2 questions**: ✅ Proceed to create task brief
- **3+ questions**: ❌ STOP - Ask user for clarification

**For detailed assessment**: See [CONFIDENCE_GATES.md](./CONFIDENCE_GATES.md)

---

### Step 3: If 3+ Questions → Ask User

**STOP and ask for clarification.**

**Format**:
```
⏸️ CLARIFICATION NEEDED - Have X unanswered questions

BR: [BR-XXX-NAME]
Total Questions: X

[List all questions by area]

Request: Please clarify so I can proceed safely.
```

**Template**: [COMMUNICATION_TEMPLATES.md](./COMMUNICATION_TEMPLATES.md#pre-delegation-templates)

**DO NOT proceed until user answers AND total questions ≤2.**

---

### Step 4: Once ≥90% → Decompose & Plan

Break down BR into TDD phases:

```
Phase 1 - RED (Write Tests):
  Task 1.1: [Test for acceptance criterion 1]
  Task 1.2: [Test for acceptance criterion 2]
  
Phase 2 - GREEN (Minimal Implementation):
  Task 2.1: [Minimal implementation]
  Task 2.2: MANDATORY integration in cmd/
  
Phase 3 - REFACTOR (Enhancements):
  Task 3.1: [Code quality improvements]
  Task 3.2: [Performance optimizations]
```

**Checkpoints**:
- ✓ After RED: Validate test quality
- ✓ After GREEN: Validate integration
- ✓ After REFACTOR: Final comprehensive review

---

### Step 5: Create Worker Task Brief

Prepare comprehensive brief using template from [COMMUNICATION_TEMPLATES.md](./COMMUNICATION_TEMPLATES.md#supervisor--worker-task-brief-template).

Include:
- Business context (problem & value)
- Acceptance criteria (specific & testable)
- Technical specifications (location, integration, patterns)
- Constraints & boundaries
- Implementation plan (RED/GREEN/REFACTOR)
- Edge cases to handle
- Checkpoint reporting requirements
- **Requirement**: Worker must have 90%+ confidence before starting

---

## Phase 2: Worker Assignment & Clarification

**Goal**: Spawn independent worker with ≥90% confidence before starting

### Step 1: Spawn Worker Agent via Task Tool

**Use Task tool to create independent worker**:

```
Task tool:
  prompt: "You are implementing this task as worker agent.

[Complete Task Brief from Phase 1, Step 5]

MANDATORY ASSESSMENT BEFORE IMPLEMENTING:

1. Read entire brief carefully

2. Count unanswered questions by area:
   Business Context: __ questions
   Acceptance Criteria: __ questions
   Technical Specs: __ questions
   Edge Cases: __ questions
   
   TOTAL: __ questions

3. Confidence Rule:
   - 0-2 total questions: ✅ Can proceed
   - 3+ total questions: ❌ MUST ask for clarification

4. Report your assessment with exact format:
   'Assessment complete. Total unanswered questions: X'
   [If 3+: List each question]
   [If 0-2]: 'Ready. Requesting permission to begin RED phase.'

5. ENFORCEABLE GATE - WAIT for explicit response:
   '✅ Permission granted. Begin RED phase.'
   
   DO NOT start implementing until you see those exact words.
   If you don't receive permission, report: 'Awaiting permission to start.'"
  
  description: "Implement [feature] per BR-XXX with checkpoints"
  subagent_type: "generalPurpose"
```

**Why Task tool?** Creates truly independent worker - no self-review bias.

---

### Step 2: Worker Counts Questions (Question-Count Gate)

**Worker evaluates understanding using objective question count.**

Worker must report:
```
Assessment complete.

Unanswered questions by area:
- Business Context: X questions
- Acceptance Criteria: X questions
- Technical Specs: X questions
- Edge Cases: X questions

TOTAL: X questions

[If 0-2]: Ready. Requesting permission to begin RED phase.
[If 3+]: Need clarification on these questions: [list]
```

**See**: [CONFIDENCE_GATES.md](./CONFIDENCE_GATES.md#worker-confidence-assessment-question-count-gate) for detailed assessment template.

---

### Step 3: Answer Worker Questions

**If worker has questions**:
1. Supervisor answers directly (if knows)
2. OR escalates to user (if uncertain)

**Templates**: [COMMUNICATION_TEMPLATES.md](./COMMUNICATION_TEMPLATES.md#supervisor-response-templates)

**If escalating to user**:
```
⏸️ ESCALATION - Worker asked question I cannot answer:

Worker Question: [Question]
My Understanding: [What I think]
Uncertainty: [Why I need your input]

Please clarify so I can inform the worker.
```

---

### Step 4: Enforceable Permission Gate

**Requirements to grant permission**:
- Worker has 0-2 unanswered questions (objective threshold)
- All worker questions answered
- Supervisor satisfied with readiness

**Grant permission with EXACT phrase** (worker is waiting for this):
```
✅ Permission granted. Begin RED phase.

Instructions for RED:
- Write failing tests that validate BEHAVIOR (not implementation)
- Use Ginkgo/Gomega BDD framework
- Include BR-XXX-XXX references
- Cover all edge cases from brief

Report when RED phase complete with:
'RED phase complete. Tests in [file path]'
```

**Enforcement mechanism**: 
- Worker instructed to wait for exact phrase "✅ Permission granted"
- If worker starts without permission, it violated instructions
- Supervisor verifies: "Did you receive '✅ Permission granted' before starting?"

**NO work begins until explicit permission with ✅ symbol.**

---

## Phase 3: Progress Monitoring

**Goal**: Validate quality at each checkpoint

### Checkpoint 1: After RED Phase (Test Quality)

**Worker reports**: "RED phase complete - tests written"

**Supervisor MUST VERIFY** using tools (do NOT trust self-report):

**Quick Validation**:
1. Request: "Provide test file path"
2. Read test file with Read tool
3. Verify:
   - Tests validate **behavior** (not implementation logic)
   - Ginkgo/Gomega framework (Describe/It/Expect)
   - BR references present
   - No Skip() calls: `grep "Skip()" [test_file]`

**Decision**:
- ✅ **PASS**: "Tests validated. Proceed to GREEN"
- ❌ **FAIL**: "Issues at [specific lines]. Fix before proceeding"

**Detailed protocol**: [CHECKPOINT_VALIDATION.md](./CHECKPOINT_VALIDATION.md#checkpoint-1-red-phase-test-quality)

---

### Checkpoint 2: After GREEN Phase (Integration)

**Worker reports**: "GREEN phase complete - implementation + integration done"

**Supervisor MUST VERIFY** using tools (CRITICAL - integration cannot be deferred):

**Quick Validation**:
1. Request: "Provide implementation file and cmd/ integration location"
2. Read implementation file
3. **Verify integration**: `grep -r "ComponentName" cmd/ --include="*.go"`
   - MUST find ≥1 result in cmd/[service]/main.go
4. Read cmd/ file and confirm component initialized + wired

**Decision**:
- ✅ **PASS**: "Integration verified at [cmd_file:line]. Proceed to REFACTOR"
- ❌ **MISSING**: "grep found 0 results. Add integration in cmd/ NOW"
- ⚠️ **TOO COMPLEX**: "Keep GREEN minimal. Move sophistication to REFACTOR"

**Detailed protocol**: [CHECKPOINT_VALIDATION.md](./CHECKPOINT_VALIDATION.md#checkpoint-2-green-phase-integration)

---

### Checkpoint 3: After REFACTOR Phase (Final Review)

**Worker reports**: "REFACTOR complete - code enhanced"

**Supervisor performs comprehensive review**:

**Use `review-agent-work` skill** for complete validation:
```
"Review the completed work using review-agent-work skill.

Context:
- Original BR: BR-XXX-[NAME]
- Worker completed: [summary]
- Verify all acceptance criteria met"
```

**Decision based on review**:
- ✅ **APPROVED**: "Ready for commit"
- ⚠️ **APPROVED WITH WARNINGS**: "Functional, consider: [warnings]"
- ❌ **REQUIRES CHANGES**: "Blocking issues: [list]"

**Detailed protocol**: [CHECKPOINT_VALIDATION.md](./CHECKPOINT_VALIDATION.md#checkpoint-3-refactor-phase-final-review)

---

## Phase 4: Feedback & Iteration

**Goal**: Guide worker to fix issues, escalate if persistent failures

### Failure Recovery Strategy

**1st Rejection**: Provide specific feedback → Worker fixes → Re-validate

**2nd Rejection** (same checkpoint): Escalate to user with analysis and options

**3rd Rejection**: STOP automatic loop → Mandatory user intervention

**Why stop at 3?** Indicates fundamental issue with task clarity or worker capability.

**Detailed templates**: [COMMUNICATION_TEMPLATES.md](./COMMUNICATION_TEMPLATES.md#feedback-loop-templates)

---

## Phase 5: Completion & Handoff

**When approved**, provide completion report:

```
# Work Completion Report

Business Requirement: BR-XXX-[NAME]
Implementation Summary: [files, tests, integration]
Quality Assessment: [TDD, code quality, standards]
Ready for Commit: All criteria met

Next Steps: User can commit changes
```

**Full template**: [COMMUNICATION_TEMPLATES.md](./COMMUNICATION_TEMPLATES.md#completion-templates)

---

## Decision Framework

### When to APPROVE
- ✅ All acceptance criteria met
- ✅ TDD followed (RED-GREEN-REFACTOR)
- ✅ Tests validate behavior, not implementation
- ✅ Integration present in cmd/
- ✅ Build and tests pass
- ✅ Confidence ≥85%

### When to REQUEST CHANGES
- ❌ Tests validate implementation logic
- ❌ Missing main app integration
- ❌ TDD sequence violated
- ❌ Build failures
- ❌ Blocking issues present

### When to ASK USER
- ❓ Unclear if requirement met
- ❓ Multiple valid approaches
- ❓ Severity unclear (blocking vs warning)
- ❓ Pattern deviation intentional or error?
- ❓ Confidence <90% (supervisor) or <80% (final review)

**NEVER make assumptions. When uncertain → PAUSE and ASK.**

---

## Anti-Patterns to Avoid

### ❌ Hands-Off Supervision
**Bad**: "Implement the feature. Let me know when done."
**Good**: "Implement with checkpoints at RED, GREEN, REFACTOR. I'll validate each."

### ❌ Rubber-Stamping
**Bad**: "Looks good" (without checking)
**Good**: Run `grep -r "Component" cmd/` to verify integration

### ❌ Assuming Without Checking
**Bad**: "Probably integrated in cmd/"
**Good**: "Show me integration in cmd/ before approving GREEN"

### ❌ Missing Early Issues
**Bad**: Finding implementation-logic tests in final review (late)
**Good**: Catch in RED checkpoint (early)

### ❌ Skipping Confidence Gates
**Bad**: Proceeding with 70% confidence
**Good**: Stop at <90% and ask for clarification

---

## Quick Reference: Checkpoint Focus

| Checkpoint | Validate | Key Command |
|-----------|----------|-------------|
| **RED** | Test quality | Review test file for behavior vs implementation |
| **GREEN** | Integration | `grep -r "Component" cmd/` |
| **REFACTOR** | Complete quality | Use `review-agent-work` skill |

---

## Success Criteria

A successful supervision session:
- ✅ Both supervisor and worker achieved ≥90% confidence before work started
- ✅ Quality issues caught early (checkpoint reviews)
- ✅ TDD sequence properly followed
- ✅ Integration verified in GREEN (not deferred)
- ✅ Final work meets all standards
- ✅ Minimal rework needed
- ✅ Clear approval with specific validation

---

## Reference Documentation

### For Detailed Information
- **[CONFIDENCE_GATES.md](./CONFIDENCE_GATES.md)** - Detailed confidence assessment checklists and criteria
- **[COMMUNICATION_TEMPLATES.md](./COMMUNICATION_TEMPLATES.md)** - All communication templates and examples
- **[USAGE_GUIDE.md](./USAGE_GUIDE.md)** - Usage examples and checkpoint flow demonstrations
- **[BR_INTEGRATION.md](./BR_INTEGRATION.md)** - How supervisor reads and processes BR documents

### Integration
- **review-agent-work skill** - Used for final comprehensive validation at Checkpoint 3

---

## Key Principles

- **Independent agents** - Supervisor and worker are separate (no self-review bias)
- **Two-tier clarification** - Both must have ≤2 questions before work starts
- **Evidence-based validation** - Use tools (Read, grep) to verify checkpoints
- **Escalate uncertainty** - Ask user when unclear, never assume
