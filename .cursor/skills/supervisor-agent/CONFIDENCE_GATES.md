# Confidence Assessment Gates

## Objective Question-Count Method

**Why not percentages?** Percentages are subjective and unreliable. Question counting is **objective and measurable**.

**The Rule**: Count unanswered questions. If 3+ questions, MUST ask user before proceeding.

**Rationale**:
- 0 questions: Full clarity, 100% confident
- 1-2 questions: Minor gaps, can proceed (details emerge during work)
- 3+ questions: Significant ambiguity, too risky to proceed

This is **objective** - anyone can count questions consistently.

---

## Supervisor Confidence Assessment (Question-Count Gate)

**MANDATORY: Supervisor must have 0-2 unanswered questions BEFORE delegation.**

### Question-Count Assessment

```
Unanswered Questions Count:

Business Objective:
[ ] What problem does this solve? → __ questions
[ ] Why does this matter to stakeholders? → __ questions
[ ] What's the measurable value? → __ questions
Subtotal: __ questions

Acceptance Criteria:
[ ] Which criteria are vague/unmeasurable? → __ questions
[ ] How do I validate each criterion? → __ questions
[ ] What's "good enough" vs "perfect"? → __ questions
Subtotal: __ questions

Technical Approach:
[ ] Which pkg/ for implementation? → __ questions
[ ] Which cmd/ for integration? → __ questions
[ ] Which existing pattern to follow? → __ questions
Subtotal: __ questions

Constraints:
[ ] What performance requirements? → __ questions
[ ] What can/cannot break? → __ questions
[ ] What compatibility needed? → __ questions
Subtotal: __ questions

Edge Cases:
[ ] What error scenarios exist? → __ questions
[ ] What are boundary conditions? → __ questions
[ ] What failure modes? → __ questions
Subtotal: __ questions

TOTAL UNANSWERED QUESTIONS: ___

Decision:
- 0-2 questions: ✅ Can proceed to delegation
- 3+ questions: ❌ MUST ask user for clarification
```

### If 3+ Questions

**STOP**: List all unanswered questions and ask user for clarification.

**Examples of valid questions**:

```
❓ "BR says 'efficient caching' but doesn't specify:
   - Cache TTL (5 min? 1 hour? configurable?)
   - Eviction policy (LRU? FIFO? TTL-based?)
   - Memory limits (100MB? 1GB? unlimited?)
   
   Current confidence: 60% (cannot delegate until clarified)"

❓ "Acceptance criteria says 'clear error messages' but:
   - What level of detail? (minimal? detailed? with examples?)
   - Should include remediation steps?
   - Localization needed?
   
   Current confidence: 70% (need clarity before delegation)"

❓ "Technical requirements say 'integrate with workflow handler' but:
   - Which handler? (creation? update? both?)
   - Sync or async validation?
   - Where in the request flow? (before persistence? after?)
   
   Current confidence: 65% (blocking delegation)"
```

---

## Worker Confidence Assessment (Question-Count Gate)

**MANDATORY: Worker must have 0-2 unanswered questions BEFORE starting work.**

### Worker Question-Count Assessment

```
I've reviewed the task brief. Counting unanswered questions:

Business Context:
[ ] What problem am I solving? → __ questions
[ ] Why does this matter? → __ questions
Subtotal: __ questions

Acceptance Criteria:
[ ] Unclear criteria? → __ questions
[ ] How to validate each? → __ questions
Subtotal: __ questions

Technical Specifications:
[ ] Implementation location unclear? → __ questions
[ ] Integration point ambiguous? → __ questions
[ ] Which pattern to follow? → __ questions
Subtotal: __ questions

Edge Cases:
[ ] Unclear edge case behaviors? → __ questions
[ ] Missing expected outcomes? → __ questions
Subtotal: __ questions

TOTAL UNANSWERED QUESTIONS: ___

Decision:
- 0-2 questions: ✅ Ready to start. Requesting permission.
- 3+ questions: ❌ Need clarification before starting.

[If 0-2 questions]:
Assessment complete. Total unanswered questions: X
Ready. Requesting permission to begin RED phase.

[If 3+ questions]:
Assessment complete. Total unanswered questions: X
Questions for supervisor:
1. [Specific question]
2. [Specific question]
...
```

### If Worker Has 3+ Questions

**Worker must ask supervisor specific questions.**

Supervisor either:
- Answers directly (if knows)
- Escalates to user (if uncertain)

**NO work starts until worker has 0-2 questions AND receives explicit '✅ Permission granted'.**

---

## Escalation Criteria

### Supervisor → User Escalation

Ask user when:
- ❓ Unclear if requirement is met
- ❓ Multiple valid approaches
- ❓ Severity unclear (blocking vs warning)
- ❓ Test coverage threshold ambiguous
- ❓ Pattern deviation intentionality uncertain
- ❓ Confidence <90%

### Worker → Supervisor Escalation

Worker asks supervisor when:
- ❓ Task brief ambiguous
- ❓ Technical approach unclear
- ❓ Edge case behavior unspecified
- ❓ Acceptance criteria validation method unknown
- ❓ Confidence <90%

---

## Permission Gates

### Gate 1: Supervisor Delegation

**Requirement**: Supervisor ≥90% confidence

**If passed**: Create task brief and assign to worker

**If failed**: Ask user for clarification

---

### Gate 2: Worker Start

**Requirements**: 
- Worker ≥90% confidence
- Supervisor explicit permission

**If passed**: Worker begins RED phase

**If failed**: Worker asks questions, supervisor answers or escalates

---

## Question-Count Threshold Guide

**Objective measurement** - Count actual unanswered questions:

**0 questions**: 
- 100% clarity on all aspects
- No ambiguity, no unknowns
- Can proceed with full confidence

**1-2 questions**: 
- Minor gaps that won't derail work
- Details will emerge during implementation
- ✅ **Can proceed** (acceptable uncertainty)
- Example: "Should error messages include stack traces?" - can be decided during implementation

**3-4 questions**: 
- Multiple ambiguities accumulating
- Risk of wrong direction increases
- ❌ **Should ask** for clarification
- Example: "Which handler?", "What format?", "Which pattern?" - too many unknowns

**5+ questions**: 
- Fundamental understanding gaps
- Very high risk of rework
- ❌ **MUST ask** - cannot proceed safely
- Indicates BR or task brief needs significant clarification

**Why 3 is the threshold?**
- 1-2 questions: Normal for complex work
- 3+ questions: Pattern of insufficient clarity
- Better to spend 10 minutes clarifying than waste hours on wrong approach
