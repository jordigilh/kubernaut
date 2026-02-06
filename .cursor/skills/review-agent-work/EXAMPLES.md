# Review Examples

## Example 1: Complete Review with PASS Status

### Context
An agent implemented a new workflow validation feature with proper TDD methodology.

### Review Output

```markdown
# Agent Work Review Results

## Context
- **Task/Plan**: Implement workflow label validation per BR-WORKFLOW-197
- **Files Changed**: 8 files (4 implementation, 4 tests)
- **Reviewer Confidence**: 92%

## Checklist Results

### Plan Alignment: ✅ PASS
- All planned validation rules implemented
- Edge cases for missing labels handled
- Acceptance criteria met: validation rejects invalid workflows

### Business Requirements: ✅ PASS
- Mapped to BR-WORKFLOW-197 (workflow label validation)
- Tests reference BR-WORKFLOW-197 in descriptions
- No speculative features added

### TDD Compliance: ✅ PASS
- Evidence of RED phase: commit shows failing tests first
- GREEN phase: minimal validator implementation passes tests
- REFACTOR phase: enhanced with scoring algorithm
- Ginkgo/Gomega BDD framework used correctly
- Test descriptions include BR-WORKFLOW-197
- No Skip() usage found

### Code Quality: ✅ PASS
- All errors wrapped with context
- Structured ValidatorConfig type (no `any` usage)
- Consistent error logging throughout
- No dead code or unused imports

### Integration: ✅ PASS
- Validator integrated in cmd/workflowexecution/main.go
- Used in workflow handler initialization
- Follows existing validator patterns in pkg/datastorage/

### Build & Test Validation: ✅ PASS
- `go build ./...` succeeds
- All tests pass (92% coverage)
- No refactoring artifacts found

## Overall Assessment

**Status**: ✅ APPROVED

**Blocking Issues**: None

**Warnings**: None

**Confidence Score**: 92%
**Rationale**: Implementation follows established Kubernaut patterns with proper TDD methodology. Main app integration verified. Minor uncertainty around performance with 1000+ workflows, but this is acceptable for MVP.

## Next Steps
- Proceed to commit changes
- Consider adding performance test for large workflow sets in future iteration
```

---

## Example 2: Review with WARNINGS

### Context
An agent refactored database query logic but introduced some concerns.

### Review Output

```markdown
# Agent Work Review Results

## Context
- **Task/Plan**: Optimize workflow search queries per BR-PERFORMANCE-045
- **Files Changed**: 6 files (3 implementation, 3 tests)
- **Reviewer Confidence**: 78%

## Checklist Results

### Plan Alignment: ⚠️ PARTIAL
- Query optimization implemented
- Cache layer added (not in original plan)
- Performance improvement verified
- **WARNING**: Caching logic exceeds planned scope

### Business Requirements: ✅ PASS
- Mapped to BR-PERFORMANCE-045
- Tests include BR reference
- Performance metrics documented

### TDD Compliance: ✅ PASS
- Tests written first
- Ginkgo/Gomega framework used
- No Skip() usage
- Good test coverage (85%)

### Code Quality: ⚠️ WARNINGS
- Errors handled appropriately
- **WARNING**: Cache invalidation logic is complex
- **WARNING**: Using `interface{}` for cache values (should be structured type)
- Logging present but could be more detailed

### Integration: ✅ PASS
- Integrated in cmd/datastorage/main.go
- Cache initialized during startup
- Follows existing patterns

### Build & Test Validation: ✅ PASS
- Build succeeds
- All tests pass
- No compilation errors

## Overall Assessment

**Status**: ⚠️ APPROVED WITH WARNINGS

**Blocking Issues**: None

**Warnings** (should address):
- Cache layer added beyond plan scope (validate this is acceptable)
- Cache uses `interface{}` instead of structured types (violates type system guidelines)
- Cache invalidation complexity may cause maintenance burden

**Confidence Score**: 78%
**Rationale**: Core optimization goal achieved with proper testing. Cache addition is functional but introduces technical debt with `interface{}` usage and complex invalidation logic. Performance benefit is documented, but long-term maintainability concern exists.

## Next Steps
- Discuss cache addition with user (was this intended?)
- Consider refactoring cache to use structured types
- Add documentation for cache invalidation rules
- If user approves warnings, proceed to commit
```

---

## Example 3: Review with BLOCKING ISSUES

### Context
An agent implemented a new analysis engine but violated critical guidelines.

### Review Output

```markdown
# Agent Work Review Results

## Context
- **Task/Plan**: Add root cause analysis engine per BR-AI-089
- **Files Changed**: 12 files (8 implementation, 4 tests)
- **Reviewer Confidence**: 55%

## Checklist Results

### Plan Alignment: ✅ PASS
- Analysis engine implemented as planned
- All specified algorithms included
- Acceptance criteria met

### Business Requirements: ✅ PASS
- Mapped to BR-AI-089
- Tests reference business requirement

### TDD Compliance: ❌ FAIL
- **CRITICAL**: Tests use Skip() for complex scenarios
- **CRITICAL**: No evidence of RED phase (implementation committed before tests)
- Ginkgo/Gomega framework used
- Test coverage only 45%

### Code Quality: ⚠️ WARNINGS
- Some errors ignored without logging
- Mixed use of structured types and `interface{}`
- **WARNING**: Large functions (>150 lines) should be refactored

### Integration: ❌ FAIL
- **CRITICAL**: New AnalysisEngine type not integrated in cmd/aianalysis/main.go
- Engine code exists in pkg/aianalysis/engine.go but never instantiated
- **CRITICAL**: Orphaned component - no main app usage found

### Build & Test Validation: ❌ FAIL
- **CRITICAL**: Build fails with undefined symbol errors
- Tests skipped so failures hidden
- Refactoring artifacts found (old field names still referenced)

## Overall Assessment

**Status**: ❌ REQUIRES CHANGES

**Blocking Issues** (must fix):
1. **TDD Violation**: Remove all Skip() calls and fix failing tests
2. **Integration Failure**: Integrate AnalysisEngine in cmd/aianalysis/main.go
3. **Build Errors**: Fix undefined symbols and compilation errors
4. **Test Coverage**: Increase to 70%+ and follow proper RED-GREEN-REFACTOR

**Warnings** (should address):
- Error handling needs improvement (log all errors)
- Large functions should be broken down
- Type system violations (`interface{}` usage)

**Confidence Score**: 55%
**Rationale**: While the algorithm implementation appears sound, critical Kubernaut guidelines are violated. TDD process was not followed properly (tests with Skip(), implementation first). Most critically, the engine is completely orphaned - not integrated with any main application. Code doesn't build successfully. These are foundational violations that must be addressed.

## Next Steps
1. **STOP**: Do not commit this code
2. Fix blocking issues in order:
   a. Integrate engine in cmd/aianalysis/main.go first (GREEN phase requirement)
   b. Remove Skip() and fix failing tests
   c. Resolve build errors
   d. Improve test coverage to 70%+
3. Re-run review after fixes
4. Only proceed to commit after all blocking issues resolved
```

---

## Example 4: Escalation Scenario

### Context
An agent attempted to implement authentication but the plan is unclear.

### Review Output

```markdown
# Agent Work Review Results

## Context
- **Task/Plan**: "Add authentication" (vague, no acceptance criteria)
- **Files Changed**: 23 files (major architectural changes)
- **Reviewer Confidence**: 40%

## Escalation Required

**Reason**: Cannot complete review with confidence

**Issues**:
1. **Plan Unclear**: Original task lacks specific acceptance criteria or approach
2. **Scope Exceeds Plan**: 23 files changed suggests major architectural change
3. **Multiple Approaches Possible**: Could be JWT, session-based, or OAuth
4. **Business Requirement Missing**: No BR reference found
5. **Confidence Too Low**: 40% confidence is below threshold

## Partial Assessment

**What Was Implemented**:
- JWT-based authentication middleware
- Token validation in gateway
- User identity extraction
- Role-based access control (RBAC) partial implementation

**Concerns**:
- No business requirement mapped
- RBAC not in original task
- Security implications not documented
- Integration approach differs from existing patterns
- Tests minimal (30% coverage)

## Recommendation

**STOP REVIEW** - Escalate to user for clarification:

1. What specific authentication approach was intended?
2. Is JWT the approved method?
3. Was RBAC part of the scope?
4. What is the business requirement this serves?
5. What are the security requirements and constraints?

**Cannot proceed** with approval or rejection until plan clarity achieved.
```

---

## Common Patterns

### Pattern 1: Quick Approval

For simple, well-executed changes:
- Checklist results clear (mostly PASS)
- Confidence ≥85%
- No blocking issues
- Brief, focused review

### Pattern 2: Approval with Technical Debt

For functional but imperfect implementations:
- Core requirements met (PASS)
- Quality warnings present
- Technical debt documented
- Confidence 75-85%
- Suggest improvements for future

### Pattern 3: Conditional Approval

For out-of-scope but good implementations:
- Implementation quality high
- Scope exceeds plan
- Need user confirmation on added features
- Confidence varies based on risk

### Pattern 4: Rejection with Clear Fix Path

For fixable violations:
- Specific blocking issues listed
- Clear remediation steps provided
- TDD or integration failures
- Confidence often 60-75%
- Actionable next steps

### Pattern 5: Escalation

For unclear or high-risk situations:
- Plan ambiguity
- Major architectural changes
- Security concerns
- Confidence <60%
- Need user input before proceeding

### Pattern 6: Pause for Input

For mid-review uncertainties:
- Cannot verify checklist item without assumptions
- Standards application is ambiguous
- Multiple valid interpretations
- Severity assessment unclear (blocking vs. warning)
- Pattern deviations without clear rationale
- **Key**: Pause review, ask specific questions, resume after clarification

---

## Example 5: Asking for Input During Review (CRITICAL)

### Context
Reviewer encounters uncertainties while executing the review checklist for a caching feature.

### ✅ CORRECT: Pausing to Ask for Clarification

```markdown
# Agent Work Review - PAUSED FOR INPUT

## Context
- **Task/Plan**: Add Redis caching to workflow search per BR-PERFORMANCE-045
- **Files Changed**: 8 files (cache implementation + integration)
- **Review Status**: ⏸️ PAUSED - Need clarification

## Questions Requiring User Input

### Question 1: Integration Pattern Validation
**Finding**: Cache layer integrated in `pkg/datastorage/cache/` but NOT in `cmd/datastorage/main.go`

**Uncertainty**: I cannot determine if this is:
- ❓ Option A: Intentional (cache will be wired up in a follow-up task)
- ❓ Option B: Missing integration (violates GREEN phase requirement)

**Current Assessment**: Without knowing intent, I cannot mark Integration checklist as PASS or FAIL.

**Request**: Please clarify - is the main app integration planned for this PR or a follow-up?

---

### Question 2: Test Coverage Threshold
**Finding**: Cache implementation has 62% unit test coverage

**Uncertainty**: Standard is 70%+ for unit tests, but:
- ❓ Is 62% acceptable for infrastructure/wrapper code?
- ❓ Should this be flagged as blocking or just a warning?
- ❓ Are there untestable portions that justify lower coverage?

**Request**: Please confirm if 62% coverage is acceptable for this cache wrapper, or if 70%+ is required before approval.

---

### Question 3: Existing Pattern Deviation
**Finding**: New cache uses Redis client directly. Existing caching in `pkg/cache/` uses a `CacheProvider` interface.

**Uncertainty**: 
- ❓ Is this intentional architectural decision (Redis-specific features needed)?
- ❓ Should this be flagged as violating "enhance existing patterns" guideline?

**Request**: Please confirm if direct Redis usage is approved, or if this should use the existing `CacheProvider` interface pattern.

---

## Partial Checklist Results (Items Requiring Clarification)

### Integration: ❓ UNCERTAIN
- Cannot verify without knowing if main app integration is in-scope

### TDD Compliance: ❓ UNCERTAIN  
- Cannot determine if 62% coverage is acceptable

### Code Quality: ❓ UNCERTAIN
- Cannot assess if pattern deviation is intentional

## Next Steps
**Awaiting user input on the 3 questions above before completing review.**
```

### ❌ WRONG: Making Assumptions and Proceeding

```markdown
# Agent Work Review Results

### Integration: ❌ FAIL
- Cache not integrated in main app (ASSUMPTION: should be integrated now)

### TDD Compliance: ⚠️ WARNING
- Only 62% coverage (ASSUMPTION: this is just a warning, not blocking)

### Code Quality: ⚠️ WARNING
- Pattern deviates from existing code (ASSUMPTION: probably intentional)

**Status**: ⚠️ APPROVED WITH WARNINGS
```

**Why This is Wrong**:
- Made 3 critical assumptions without asking
- Integration "failure" may be incorrect if follow-up task planned
- Coverage assessment wrong if 62% is actually acceptable for this code type
- Pattern deviation might be a critical issue, not just a warning

### Key Principle

**When uncertain → PAUSE and ASK**

Valid reasons to pause:
- Cannot verify checklist item without assumptions
- Multiple interpretations possible
- Standards application is ambiguous
- Severity assessment unclear
- Pattern deviations without documented rationale

**DO NOT**:
- Assume and mark as FAIL/PASS
- Guess at severity (blocking vs. warning)
- Apply one-size-fits-all thresholds
- Complete review with "probably" thinking

---

## Example 6: Testing Implementation Logic vs Behavior (CRITICAL)

### Context
An agent added tests for a retry mechanism but focused on implementation details.

### ❌ WRONG: Testing Implementation Logic

```go
It("should call exponentialBackoff helper 3 times", func() {
    // Verifying internal helper calls
    Expect(mockBackoffHelper.CallCount()).To(Equal(3))
})

It("should use exponential calculation with base 2", func() {
    // Testing internal calculation logic
    Expect(backoffCalculator.Base).To(Equal(2))
})
```

**Problems**:
- Tests implementation details (helper call counts)
- Tests internal calculation mechanisms
- Tests break if implementation changes (even if behavior is correct)
- Doesn't validate actual business outcome

### ✅ CORRECT: Testing Business Behavior/Outcomes

```go
It("should retry failed requests with increasing delays [BR-NOT-052]", func() {
    // Test BEHAVIOR: Does retry happen with proper delays?
    startTime := time.Now()
    
    result := notifier.SendNotification(failingEndpoint)
    
    // Validate business outcome: retries occurred
    Expect(result.Attempts).To(Equal(4)) // Initial + 3 retries
    
    // Validate correctness: delays increased exponentially
    elapsed := time.Since(startTime)
    Expect(elapsed).To(BeNumerically(">=", 7*time.Second)) // 1s + 2s + 4s minimum
    Expect(elapsed).To(BeNumerically("<", 15*time.Second)) // Reasonable upper bound
})

It("should eventually succeed after transient failures [BR-NOT-052]", func() {
    // Test OUTCOME: Does the retry mechanism achieve its purpose?
    server := mockServerWithTransientFailures(failTimes: 2)
    
    result := notifier.SendNotification(server.URL)
    
    // Validate business outcome: notification delivered despite failures
    Expect(result.Success).To(BeTrue())
    Expect(result.Attempts).To(Equal(3)) // Failed twice, succeeded third time
})
```

**Why This is Correct**:
- Tests observable behavior (retry attempts, delays, success/failure)
- Tests business outcomes (notification delivered despite failures)
- Tests remain valid if implementation changes (e.g., switching from exponential to linear)
- Validates correctness (delays are appropriate, retries work)

### Review Assessment

**TDD Compliance**: ❌ FAIL (if implementation-focused tests) / ✅ PASS (if behavior-focused tests)

**Blocking Issue**:
Tests that verify "helper X called 3 times" or "internal field equals Y" are IMPLEMENTATION LOGIC tests and violate Kubernaut testing principles.

**Required Action**:
Rewrite tests to validate:
1. **Business outcomes**: Did the feature achieve its purpose?
2. **Correctness**: Does it behave correctly under various conditions?
3. **Observable behavior**: What external effects/results occur?

**Key Principle**: Tests must validate **business outcomes with correctness and behavior**, NEVER implementation details.
