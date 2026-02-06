# Supervisor Agent - Usage Guide

## 🎯 What is the Supervisor Agent?

The supervisor agent **orchestrates work** instead of just reviewing completed work. It:
1. Reads your business requirements
2. Breaks them into tasks
3. Assigns work to a worker agent
4. Monitors progress at checkpoints
5. Validates quality throughout
6. Provides feedback and corrections
7. Approves when standards are met

Think of it as a **project manager + quality assurance** combined.

---

## 🆚 Supervisor vs Review Agent

| Aspect | Review Agent | Supervisor Agent |
|--------|--------------|------------------|
| **Timing** | Post-completion | Throughout execution |
| **Role** | Quality validator | Work orchestrator |
| **Interaction** | One-time check | Continuous oversight |
| **Worker** | Any agent | Specifically assigned sub-agent |
| **Feedback** | Final report | Checkpoint reviews + final |
| **Task Definition** | Assumes task understood | Reads BR, creates task plan |

**Use Review Agent When**: Work is done, need validation

**Use Supervisor Agent When**: Starting new work, need oversight

---

## 🚀 Basic Usage

### Scenario 1: Implement from Business Requirement Document

```
You: Supervisor, implement BR-WORKFLOW-197 from 
     ../cursor-swarm-dev/docs/business-requirements/BR-WORKFLOW-197.md
     
     Assign to a worker agent and monitor with checkpoints.
```

**What Happens**:
```
Supervisor: Reading BR-WORKFLOW-197...
[Supervisor reads and analyzes the BR]

Supervisor: BR understood. Breaking down into tasks:

Phase 1 - RED (Tests):
  - Task 1.1: Write tests for mandatory label validation
  - Task 1.2: Write tests for label format validation
  
Phase 2 - GREEN (Implementation):  
  - Task 2.1: Implement minimal validator
  - Task 2.2: Integrate in cmd/workflowexecution/main.go
  
Phase 3 - REFACTOR (Enhancement):
  - Task 3.1: Extract validation rules
  - Task 3.2: Add detailed error messages

Checkpoints:
✓ After RED: Validate test quality
✓ After GREEN: Validate integration
✓ After REFACTOR: Final comprehensive review

Assigning to worker agent...

---

Worker: Starting RED phase - writing tests for label validation...

[Worker writes tests]

Worker: RED phase complete. Tests written in test/unit/workflowexecution/validator_test.go

Supervisor: Reviewing RED phase checkpoint...
[Supervisor validates test quality]

Supervisor: ✅ Tests look good! They validate behavior not implementation.
           Proceed to GREEN phase: minimal implementation + integration.

Worker: Starting GREEN phase...

[Worker implements + integrates]

Worker: GREEN phase complete. Validator implemented and integrated in cmd/workflowexecution/main.go

Supervisor: Reviewing GREEN phase checkpoint...
[Supervisor validates integration]

Supervisor: ✅ Integration verified. Tests passing. Proceed to REFACTOR phase.

[Continues through REFACTOR]

Supervisor: Final review complete. ✅ APPROVED. Ready for commit.
```

---

### Scenario 2: Inline BR Specification

If you don't have a BR document:

```
You: Supervisor, oversee implementation of this requirement:

"Add Redis caching to workflow search"

Acceptance Criteria:
- Cache layer in pkg/datastorage/cache/
- Integration in cmd/datastorage/main.go
- 70%+ test coverage
- Use existing CacheProvider interface

Assign to worker with checkpoint monitoring.
```

**Supervisor**:
1. Parses the inline requirement
2. Creates task breakdown
3. Assigns to worker
4. Monitors at checkpoints
5. Validates against acceptance criteria

---

### Scenario 3: Resume Supervision After Interruption

```
You: Worker completed RED phase (tests written). 
     Supervisor, validate test quality and approve or request changes.
```

**Supervisor validates checkpoint and provides feedback**

---

## 📊 Checkpoint Flow Examples

### Example 1: RED Phase - Test Quality Issue Found

```
Worker: RED phase complete. Tests in test/unit/datastorage/cache_test.go

Supervisor: Reviewing RED phase...

Supervisor: ❌ RED phase has issues:

1. Tests validate implementation logic:
   - "should call Redis client 3 times" ← Implementation detail
   - "should use exponential backoff helper" ← Implementation detail
   
2. Missing behavior tests:
   - No test for "should cache successful searches"
   - No test for "should skip caching on errors"

Fix these issues:
- Replace implementation tests with behavior tests
- Add missing behavior validation tests

DO NOT proceed to GREEN until RED phase approved.

---

Worker: Fixed. Updated tests to validate behavior:
  - "should return cached results for repeated searches"
  - "should fetch from database on cache miss"
  - "should skip caching when errors occur"

Supervisor: Reviewing updated tests...

Supervisor: ✅ Tests now validate behavior correctly! 
           Proceed to GREEN phase: minimal implementation + integration.
```

**Key**: Issue caught in RED phase (early) instead of final review (late) → Less rework

---

### Example 2: GREEN Phase - Missing Integration Caught

```
Worker: GREEN phase complete. Implementation in pkg/datastorage/cache/redis.go

Supervisor: Reviewing GREEN phase...

Supervisor: ❌ CRITICAL ISSUE: Missing main app integration

grep -r "RedisCache" cmd/ --include="*.go"
Result: No matches found

The cache layer exists in pkg/ but is NOT integrated in cmd/datastorage/main.go

This violates GREEN phase requirement: "Integration must happen in GREEN, not deferred"

Action Required:
1. Add cache initialization in cmd/datastorage/main.go
2. Wire cache into the workflow search handler
3. Show me the integration code

DO NOT proceed to REFACTOR until integration verified.

---

Worker: Integration added in cmd/datastorage/main.go:

```go
// Initialize cache
redisCache, err := cache.NewRedisCache(redisClient)
if err != nil {
    log.Fatal(err)
}

// Wire into search handler
searchHandler := workflow.NewSearchHandler(repo, redisCache)
```

Supervisor: Verifying integration...
grep -r "NewRedisCache" cmd/datastorage/ --include="*.go"
✓ Found in cmd/datastorage/main.go

Supervisor: ✅ Integration verified! Tests passing? 

Worker: Yes, all tests pass.

Supervisor: ✅ GREEN phase approved. Proceed to REFACTOR: enhance code quality.
```

**Key**: Integration verified in GREEN (correct) instead of deferred to later (incorrect)

---

## 🎯 Advanced Usage Patterns

### Pattern 1: Multiple BR Implementation

```
You: Supervisor, implement these requirements in sequence:
     1. BR-WORKFLOW-197 (validation)
     2. BR-WORKFLOW-198 (search optimization)
     
     Assign to worker, monitor each with checkpoints, 
     ensure BR-197 approved before starting BR-198.
```

### Pattern 2: High-Risk Work with Extra Validation

```
You: Supervisor, this is a critical security feature.
     
     Use these checkpoints:
     1. Test design review (before any implementation)
     2. Security validation review (during GREEN)
     3. Penetration test scenarios (during REFACTOR)
     4. Final comprehensive security review
     
     Pause for my approval at each checkpoint before worker proceeds.
```

### Pattern 3: Supervisor with Specific Focus Area

```
You: Supervisor, focus specifically on ensuring:
     - Tests validate behavior, not implementation
     - Integration happens in GREEN, not deferred
     - No orphaned components
     
     Standard validation for other aspects.
```

---

## 💬 Communication Flow

### Supervisor → Worker (Task Assignment)
```
"Worker: Implement workflow validation per BR-WORKFLOW-197

Tasks:
[Breakdown]

Guidelines:
- TDD: RED → GREEN → REFACTOR
- Tests validate behavior, not implementation
- Integrate in cmd/ during GREEN

Report at each checkpoint."
```

### Worker → Supervisor (Checkpoint Report)
```
"RED phase complete. Tests written in test/unit/workflowexecution/validator_test.go"
```

### Supervisor → Worker (Checkpoint Feedback)
```
✅ "Tests approved. Proceed to GREEN: minimal implementation + integration"

OR

❌ "Tests have issues: [specific feedback]. Fix before GREEN."
```

### Supervisor → User (Clarification Needed)
```
"⏸️ PAUSED - Need clarification:

Question: BR says 'efficient caching' but doesn't specify TTL. 
         Should I use:
         A) 5 minute TTL (standard)
         B) Configurable TTL
         C) User will specify

Cannot proceed without this clarification."
```

### User → Supervisor (Resume)
```
"Use configurable TTL with 5 minute default."
```

---

## 🔧 Troubleshooting

### Issue: Supervisor Not Taking Initiative

**Problem**: Supervisor waits for instructions instead of managing worker

**Solution**: Be explicit about supervision mode:
```
"Act as supervisor: Read BR, create plan, assign worker, monitor checkpoints"
```

### Issue: Worker Bypassing Checkpoints

**Problem**: Worker completes all phases without stopping for validation

**Solution**: Supervisor should be more directive:
```
Supervisor: "Worker, STOP after RED phase. Report test completion. 
            Do NOT proceed to GREEN until I validate."
```

### Issue: Supervisor Rubber-Stamping

**Problem**: Supervisor approves without actually checking

**Solution**: User can request detailed validation:
```
You: "Supervisor, show me your validation. Run the grep commands. 
     Show evidence that integration exists."
```

---

## ✅ Success Indicators

You know supervision is working when:

- ✅ Worker reports at each checkpoint (not skipping ahead)
- ✅ Supervisor validates with evidence (grep results, file reads)
- ✅ Issues caught early (RED/GREEN) not late (final review)
- ✅ Clear feedback provided ("Fix X before Y")
- ✅ Integration verified in GREEN phase (not deferred)
- ✅ Supervisor pauses to ask when uncertain
- ✅ Final work meets all standards (minimal rework)

---

## 🎓 Learning from Supervision

Track patterns over time:

**Common Issues Caught Early**:
- Implementation-logic tests (caught in RED checkpoint)
- Missing integration (caught in GREEN checkpoint)
- Sophisticated logic in GREEN (should be REFACTOR)

**Supervisor Learning**:
- Which checkpoints catch most issues
- Which acceptance criteria are often unclear
- Which worker patterns need improvement

---

## 🔗 Integration with Other Skills

### With Review Agent Skill

Supervisor uses review agent for final comprehensive validation:

```
Supervisor: "Worker completed REFACTOR. 
            Using review-agent-work skill for final validation..."
            
[Review agent performs full checklist]

Supervisor: "Review complete. ✅ APPROVED based on review assessment."
```

### With Project Documentation

Supervisor reads from your docs:

```
Supervisor: Reading business requirement from:
            ../cursor-swarm-dev/docs/business-requirements/BR-XXX.md
            
[Supervisor extracts acceptance criteria, constraints, etc.]
```

---

## 🎯 Best Practices

### DO:
- ✅ Read BR documents thoroughly
- ✅ Create clear task breakdowns
- ✅ Set explicit checkpoints
- ✅ Validate with evidence (grep, file reads)
- ✅ Catch issues early (checkpoint reviews)
- ✅ Pause to ask when uncertain
- ✅ Provide specific, actionable feedback

### DON'T:
- ❌ Skip checkpoint validations
- ❌ Assume integration without checking
- ❌ Approve without evidence
- ❌ Let worker skip ahead without approval
- ❌ Make assumptions about unclear requirements

---

## Quick Start

**Ready to try it?**

```
You: Supervisor, implement [BR-NAME] from [path or inline spec].
     Assign to worker with checkpoint monitoring.
     
     Checkpoints:
     - After RED: Validate test quality
     - After GREEN: Validate integration
     - After REFACTOR: Final comprehensive review
     
     Start now.
```

The supervisor will:
1. Read/understand the requirement
2. Create task breakdown
3. Assign to worker
4. Monitor and validate
5. Approve when ready

---

**Want to try with one of your BRs?** Just say:

```
"Supervisor, implement BR-XXX from ../cursor-swarm-dev/docs/business-requirements/BR-XXX.md"
```
