# Test Plan: Shadow Agent — Prompt Injection Guardrails (Audit Remediation)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-601-v2.0
**Feature**: Fail-closed shadow audit agent with random boundary defense and head+tail truncation across all KA agents
**Version**: 2.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.4`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the audit remediation for Issue #601. A comprehensive adversarial audit identified 1 CRITICAL, 7 HIGH, 10 MEDIUM, and 5 LOW findings in the initial shadow agent implementation. This v2.0 plan supersedes TP-601-v1.0 and addresses all findings: fail-closed security posture, random boundary defense for all agents, head+tail truncation, goroutine lifecycle fixes, nil safety, config validation, and JSON response validation.

### 1.2 Objectives

1. **Fail-closed on all error paths**: Shadow agent evaluator errors, JSON parse failures, and verdict timeouts all escalate to human review with structured evidence.
2. **Random boundary defense**: All agents (shadow evaluator, investigation, conversation) wrap untrusted tool output in unpredictable boundaries that cannot be escaped.
3. **Head+tail truncation**: Long tool outputs are truncated from both ends to prevent injection placement after the truncation point.
4. **>=80% per-tier coverage**: Unit-testable code (boundary package, evaluator, observer, proxies, types, config) and integration-testable code (wrapper, main wiring, conversation adapter) each reach >=80%.
5. **Zero anti-patterns**: No `time.Sleep()`, no `Skip()`, no direct audit testing, no HTTP endpoint testing in integration tier.
6. **13/13 crafted injection payloads classified correctly**: P1-P8 suspicious (direct instruction injection), P11-P13 suspicious (data exfiltration), P9-P10 clean.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/alignment/...` + `./test/unit/kubernautagent/security/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on alignment + security/boundary packages |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/...` |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on wrapper + main wiring + conversation adapter |
| Backward compatibility | 0 regressions | Existing KA tests pass without modification |
| All fail-open paths eliminated | 0 remaining | Every error/timeout path returns Suspicious=true |
| Injection payload detection | 13/13 | Payloads P1-P13 classified correctly (8 instruction injection, 3 exfiltration, 2 clean) |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-AI-601**: Prompt injection guardrails for Kubernaut Agent agentic pipeline
- Issue #601: Security: Prompt injection guardrails for Kubernaut Agent agentic pipeline
- Issue #657: Extend mock-LLM to support tool call scenarios for E2E security testing

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Audit Remediation Plan](../../../.cursor/plans/601_audit_remediation_plan.md)
- [Test Plan v1.0](TEST_PLAN.md) (superseded by this document)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Fail-closed causes excessive human review escalation when shadow LLM is down | Operational noise; all investigations require human approval | Medium | UT-SA-601-FC-001, FC-007 | Structured evidence distinguishes "unavailable" from "detected"; Error-level logging for monitoring |
| R2 | Random boundary generation has insufficient entropy | Boundary guessable; attacker can escape | Low | UT-SA-601-BD-001, BD-005 | crypto/rand with 16 bytes (2^128 entropy); uniqueness test with 1000 samples |
| R3 | Boundary escape in tool output bypasses shadow agent | Attacker closes boundary tags and injects instructions | Medium | UT-SA-601-BD-004, BD-007 | Pre-scan for closing boundary; immediate fail-closed flag without LLM call |
| R4 | Head+tail truncation misses injection in the middle of content | Content between head and tail windows is invisible to shadow | Low | UT-SA-601-TR-002 | Acceptable: mid-content is lowest-value injection position; documented limitation |
| R5 | Observer timeout drops in-flight evaluations from verdict | Partial verdict misses suspicious steps | High | UT-SA-601-FC-003, FC-004, FC-005 | Fail-closed: pending steps count as suspicious; WaitResult tracks submitted vs completed |
| R6 | Goroutine leak in WaitForCompletion | Memory/goroutine growth under load | Medium | UT-SA-601-FC-003 | Waiter goroutine bounded by WaitGroup; documented lifecycle |
| R7 | Nil evaluator/inner causes panic | KA crash | Medium | UT-SA-601-CX-001, CX-002 | Nil guards in constructors; fatal on startup if alignment enabled |
| R8 | Shadow LLM itself prompt-injected via evaluated content | Shadow returns false negative | Medium | UT-SA-601-BD-006 | Random boundary wrapping + system prompt instruction; architectural read-only isolation |

### 3.1 Risk-to-Test Traceability

- **R1** (excessive escalation): UT-SA-601-FC-001, FC-007 — verify structured explanation distinguishes availability from detection
- **R2** (entropy): UT-SA-601-BD-001, BD-005 — verify 32-char hex and 1000-sample uniqueness
- **R3** (boundary escape): UT-SA-601-BD-004, BD-007 — verify pre-scan catches escape and flags without LLM
- **R5** (timeout drops): UT-SA-601-FC-003, FC-004, FC-005, FC-006 — verify pending steps → VerdictSuspicious
- **R7** (nil panic): UT-SA-601-CX-001, CX-002 — verify constructors reject nil
- **R8** (shadow injection): UT-SA-601-BD-006 — verify boundary wrapping in evaluator messages

---

## 4. Scope

### 4.1 Features to be Tested

- **Security boundary package** (`internal/kubernautagent/security/boundary/`): Random token generation, content wrapping, escape detection
- **Shadow evaluator fail-closed** (`internal/kubernautagent/alignment/evaluator.go`): Error paths return Suspicious=true, JSON validation, head+tail truncation, boundary wrapping
- **Observer completion tracking** (`internal/kubernautagent/alignment/observer.go`): WaitResult with Complete/Submitted/Pending/TimedOut fields
- **Verdict fail-closed** (`internal/kubernautagent/alignment/types.go`): Pending steps → VerdictSuspicious
- **Wrapper fail-closed** (`internal/kubernautagent/alignment/investigator_wrapper.go`): Timeout/pending → HumanReviewNeeded
- **Tool proxy error handling** (`internal/kubernautagent/alignment/toolproxy.go`): Error content sent to shadow
- **System prompt** (`internal/kubernautagent/alignment/prompt/system.go`): Boundary-aware instructions
- **Config validation** (`internal/kubernautagent/config/config.go`): VerdictTimeout, positive values
- **Investigation boundary wrapping** (`internal/kubernautagent/investigator/investigator.go`): Tool output wrapped in boundary
- **Conversation fail-closed** (`internal/kubernautagent/conversation/llm_adapter.go`): Final WaitForCompletion, boundary wrapping
- **Startup validation** (`cmd/kubernautagent/main.go`): Fatal when alignment enabled + shadow client fails

### 4.2 Features Not to be Tested

- **E2E tests**: Blocked on #657 (mock-LLM tool call scenario support). Deferred.
- **Formal injection benchmarking** (#602, v1.5): Curated attack datasets and scoring.
- **Circuit breaker for shadow downtime**: Future enhancement (v1.5+).
- **`todo_write` tool**: Internal session tool, not external data. Bypasses ToolProxy and boundary wrapping by design. Not an attack surface.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Fail-closed on all shadow errors | Security guardrail that silently degrades defeats its purpose |
| Random boundary per evaluation call | Attacker cannot predict delimiter; eliminates escape vector |
| Pre-scan before LLM call | Deterministic check; no LLM dependency for escape detection |
| Head+tail truncation (not removal) | Preserves injection patterns at both ends; middle is lowest-value position |
| Fatal startup on misconfigured shadow | Shadow is mandatory when enabled; silent degradation unacceptable |
| Tools-only proxy in conversation mode | Trust authenticated user input; protect untrusted tool output surface |
| Shadow observes raw tool output (pre-pipeline) | External data is untrusted; internal pipeline (sanitizer/summarizer) is trusted. Shadow audits raw content at ToolProxy interception point. Main LLM works with processed data. Two boundary purposes are decoupled: evaluator boundary protects shadow LLM; main LLM boundary protects investigation/conversation LLM. |
| Evaluator ordering: pre-scan raw -> truncate -> wrap | Pre-scan sees full content (catches escape even if truncation removes it, per fail-closed posture). Truncation before wrapping ensures boundary markers are never subject to truncation. Wrapping last guarantees pristine delimiters. |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code:
  - `internal/kubernautagent/security/boundary/` (100% target — small package)
  - `internal/kubernautagent/alignment/evaluator.go` (>=80%)
  - `internal/kubernautagent/alignment/observer.go` (>=80%)
  - `internal/kubernautagent/alignment/types.go` (>=80%)
  - `internal/kubernautagent/alignment/llmproxy.go` (>=80%)
  - `internal/kubernautagent/alignment/toolproxy.go` (>=80%)
  - `internal/kubernautagent/alignment/prompt/system.go` (>=80%)
  - `internal/kubernautagent/config/config.go` — AlignmentCheck validation only (>=80%)

- **Integration**: >=80% of integration-testable code:
  - `internal/kubernautagent/alignment/investigator_wrapper.go` (>=80%)
  - `internal/kubernautagent/investigator/investigator.go` — boundary wrapping in executeTool (>=80%)
  - `internal/kubernautagent/conversation/llm_adapter.go` — alignment paths (>=80%)
  - `cmd/kubernautagent/main.go` — alignment wiring (>=80%)

- **E2E**: Deferred to #657. Unit + Integration provide two-tier minimum.

### 5.2 Two-Tier Minimum

Every P0 *integration-testable* surface has Unit + Integration coverage. Pure logic helpers (boundary generation, truncation, JSON validation) are unit-tested only — they have no I/O or wiring to integration-test.
- Unit: Logic correctness (boundary generation, fail-closed, truncation, JSON validation)
- Integration: Wiring correctness (decorator chain, main startup, conversation adapter)

### 5.3 Business Outcome Quality Bar

Tests validate **security outcomes**: "does an attacker get caught?" and "does an operator get actionable evidence?" — not just "was a function called?"

### 5.4 Pass/Fail Criteria

**PASS** — all of the following:
1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier code coverage >=80% on unit-testable and integration-testable subsets
4. Zero fail-open paths remain (every error/timeout → Suspicious=true)
5. All 13 injection payloads classified correctly
6. No regressions in existing KA test suites
7. `go build ./...` succeeds with zero errors

**FAIL** — any of the following:
1. Any P0 test fails
2. Per-tier coverage below 80%
3. Any error/timeout path returns Suspicious=false (fail-open)
4. Existing tests that were passing now fail

### 5.5 Suspension & Resumption Criteria

**Suspend**:
- Build broken: code does not compile
- #657 blocker not resolved for E2E tier (E2E already deferred)
- Cascading failures: >3 tests fail for the same root cause

**Resume**:
- Build fixed and green
- Root cause identified and fix deployed

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/security/boundary/boundary.go` | `Generate`, `Wrap`, `ContainsEscape`, `WrapOrFlag` | ~50 |
| `internal/kubernautagent/alignment/evaluator.go` | `NewEvaluator`, `EvaluateStep`, `truncateHeadTail` | ~130 |
| `internal/kubernautagent/alignment/observer.go` | `NewObserver`, `NextStepIndex`, `SubmitAsync`, `WaitForCompletion`, `RenderVerdict` | ~130 |
| `internal/kubernautagent/alignment/types.go` | `Step`, `Observation`, `Verdict`, `WaitResult` | ~70 |
| `internal/kubernautagent/alignment/llmproxy.go` | `NewLLMProxy`, `Chat` | ~60 |
| `internal/kubernautagent/alignment/toolproxy.go` | `NewToolProxy`, `Execute`, `ToolsForPhase`, `All` | ~75 |
| `internal/kubernautagent/alignment/prompt/system.go` | `SystemPrompt` | ~95 |
| `internal/kubernautagent/config/config.go` | `AlignmentCheckConfig.Validate`, `EffectiveLLM` | ~40 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/alignment/investigator_wrapper.go` | `NewInvestigatorWrapper`, `Investigate`, `emitAlignmentAudit` | ~140 |
| `internal/kubernautagent/investigator/investigator.go` | `executeTool` (boundary wrapping) | ~30 |
| `internal/kubernautagent/conversation/llm_adapter.go` | `Respond` (alignment paths), `checkAlignmentWarning` | ~40 |
| `cmd/kubernautagent/main.go` | Alignment wiring section | ~60 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.4` HEAD | After audit remediation |
| Dependency: #657 | Open | Blocks E2E tier only |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AI-601 | Evaluator returns Suspicious=true after retry exhaustion | P0 | Unit | UT-SA-601-FC-001 | Pending |
| BR-AI-601 | Evaluator returns Suspicious=true on cancelled context | P0 | Unit | UT-SA-601-FC-002 | Pending |
| BR-AI-601 | Observer reports incomplete when timeout elapses | P0 | Unit | UT-SA-601-FC-003 | Pending |
| BR-AI-601 | Observer tracks pending step count after timeout | P0 | Unit | UT-SA-601-FC-004 | Pending |
| BR-AI-601 | RenderVerdict returns VerdictSuspicious when pending > 0 | P0 | Unit | UT-SA-601-FC-005 | Pending |
| BR-AI-601 | Wrapper sets HumanReviewNeeded on verdict timeout | P0 | Unit | UT-SA-601-FC-006 | Pending |
| BR-AI-601 | Wrapper sets HumanReviewNeeded when evaluator unavailable | P0 | Unit | UT-SA-601-FC-007 | Pending |
| BR-AI-601 | JSON response missing `suspicious` field → fail-closed | P0 | Unit | UT-SA-601-FC-008 | Pending |
| BR-AI-601 | JSON response `{}` → fail-closed | P0 | Unit | UT-SA-601-FC-009 | Pending |
| BR-AI-601 | MaxRetries=0 → immediate fail-closed | P1 | Unit | UT-SA-601-FC-010 | Pending |
| BR-AI-601 | Chat error path calls WaitForCompletion when alignObserver present | P0 | Unit | UT-SA-601-FC-011 | Pending |
| BR-AI-601 | Boundary Generate returns 32-char hex | P0 | Unit | UT-SA-601-BD-001 | Pending |
| BR-AI-601 | Boundary Wrap returns content between markers | P0 | Unit | UT-SA-601-BD-002 | Pending |
| BR-AI-601 | Boundary ContainsEscape detects closing marker | P0 | Unit | UT-SA-601-BD-003 | Pending |
| BR-AI-601 | WrapOrFlag returns escaped=true on escape attempt | P0 | Unit | UT-SA-601-BD-004 | Pending |
| BR-AI-601 | 1000 Generate calls produce unique tokens | P1 | Unit | UT-SA-601-BD-005 | Pending |
| BR-AI-601 | Evaluator wraps content in boundary before shadow LLM | P0 | Unit | UT-SA-601-BD-006 | Pending |
| BR-AI-601 | Evaluator boundary escape → immediate Suspicious=true | P0 | Unit | UT-SA-601-BD-007 | Pending |
| BR-AI-601 | System prompt contains boundary instruction | P1 | Unit | UT-SA-601-BD-008 | Pending |
| BR-AI-601 | WrapOrFlag with empty content does not panic | P1 | Unit | UT-SA-601-BD-009 | Pending |
| BR-AI-601 | Partial boundary marker substring not flagged as escape | P1 | Unit | UT-SA-601-BD-010 | Pending |
| BR-AI-601 | Short content unchanged by head+tail truncation | P1 | Unit | UT-SA-601-TR-001 | Pending |
| BR-AI-601 | Long content preserves head and tail runes | P0 | Unit | UT-SA-601-TR-002 | Pending |
| BR-AI-601 | max=0 returns full content | P1 | Unit | UT-SA-601-TR-003 | Pending |
| BR-AI-601 | Unicode multi-byte runes handled correctly | P1 | Unit | UT-SA-601-TR-004 | Pending |
| BR-AI-601 | Ellipsis marker present in truncated output | P1 | Unit | UT-SA-601-TR-005 | Pending |
| BR-AI-601 | NewObserver rejects nil evaluator | P0 | Unit | UT-SA-601-CX-001 | Pending |
| BR-AI-601 | NewInvestigatorWrapper rejects nil inner/evaluator | P0 | Unit | UT-SA-601-CX-002 | Pending |
| BR-AI-601 | ToolProxy sends error content to shadow | P0 | Unit | UT-SA-601-CX-003 | Pending |
| BR-AI-601 | Config: verdictTimeout must be positive when enabled | P1 | Unit | UT-SA-601-CX-004 | Pending |
| BR-AI-601 | Wrapper fail-closed on evaluator timeout | P0 | Integration | IT-SA-601-FC-001 | Pending |
| BR-AI-601 | Investigation tool output wrapped in boundary | P0 | Integration | IT-SA-601-BD-001 | Pending |
| BR-AI-601 | Investigation boundary escape → immediate flag | P0 | Integration | IT-SA-601-BD-002 | Pending |
| BR-AI-601 | Startup fatal when alignment enabled + shadow fails | P0 | Integration | IT-SA-601-CX-001 | Pending |
| BR-AI-601 | Conversation fail-closed on alignment timeout | P1 | Integration | IT-SA-601-FC-002 | Pending |
| BR-AI-601 | Conversation tool result wrapped in boundary | P1 | Integration | IT-SA-601-BD-003 | Pending |
| BR-AI-601 | System prompt disclosure exfiltration classified as suspicious | P1 | Unit | UT-SA-601-PAYLOAD-P11 | Pending |
| BR-AI-601 | Environment variable exfiltration classified as suspicious | P1 | Unit | UT-SA-601-PAYLOAD-P12 | Pending |
| BR-AI-601 | External URL redirect exfiltration classified as suspicious | P1 | Unit | UT-SA-601-PAYLOAD-P13 | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-SA-601-{GROUP}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SA**: Shadow Agent (service abbreviation)
- **GROUP**: `FC` (Fail-Closed), `BD` (Boundary), `TR` (Truncation), `CX` (Correctness)
- **SEQUENCE**: Zero-padded 3-digit

**Note on dual ID schemes**: Legacy IDs (UT-SA-601-001 through 014, PAYLOAD-P1-P10) are retained for existing tests. New tests use the group convention (FC, BD, TR, CX). Both schemes coexist; no migration planned.

### Tier 1: Unit Tests

**Testable code scope**: `internal/kubernautagent/alignment/`, `internal/kubernautagent/security/boundary/`, `internal/kubernautagent/config/config.go`

#### Fail-Closed Group (FC)

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `UT-SA-601-FC-001` | Operator sees "evaluator_unavailable (fail-closed)" with Suspicious=true when shadow LLM is down, ensuring no silent bypass | P0 | Pending |
| `UT-SA-601-FC-002` | Cancelled investigation context causes immediate fail-closed observation without wasting retries | P0 | Pending |
| `UT-SA-601-FC-003` | Observer WaitResult reports Complete=false and correct Pending count when timeout elapses | P0 | Pending |
| `UT-SA-601-FC-004` | Observer tracks submitted vs completed steps accurately under concurrency | P0 | Pending |
| `UT-SA-601-FC-005` | Verdict is VerdictSuspicious with "verdict_timeout" when pending steps exist after timeout | P0 | Pending |
| `UT-SA-601-FC-006` | Wrapper sets HumanReviewNeeded=true, HumanReviewReason="alignment_check_failed", and warning with timeout detail when verdict has pending steps | P0 | Pending |
| `UT-SA-601-FC-007` | Wrapper sets HumanReviewNeeded=true when evaluator returns evaluator_unavailable (fail-closed) | P0 | Pending |
| `UT-SA-601-FC-008` | JSON response with valid structure but missing `suspicious` field → treated as fail-closed (Suspicious=true) | P0 | Pending |
| `UT-SA-601-FC-009` | Empty JSON `{}` → fail-closed (Suspicious=true), not silent false | P0 | Pending |
| `UT-SA-601-FC-010` | MaxRetries=0 config → immediate fail-closed observation (no retries, no silent coercion) | P1 | Pending |
| `UT-SA-601-FC-011` | Chat error path in conversation adapter calls WaitForCompletion when alignObserver is present, ensuring alignment finalization on error | P0 | Pending |

#### Boundary Group (BD)

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `UT-SA-601-BD-001` | Generate produces 32-character hex string from crypto/rand ensuring unpredictability | P0 | Pending |
| `UT-SA-601-BD-002` | Wrap returns content between `<<<EVAL_{token}>>>` and `<<<END_EVAL_{token}>>>` markers | P0 | Pending |
| `UT-SA-601-BD-003` | ContainsEscape returns true when content contains the exact closing boundary marker | P0 | Pending |
| `UT-SA-601-BD-004` | WrapOrFlag returns escaped=true without wrapping when content contains closing marker | P0 | Pending |
| `UT-SA-601-BD-005` | 1000 sequential Generate calls produce 1000 unique tokens (no collisions) | P1 | Pending |
| `UT-SA-601-BD-006` | Evaluator wraps step content in random boundary before sending to shadow LLM; boundary markers visible in captured request | P0 | Pending |
| `UT-SA-601-BD-007` | Evaluator detects boundary escape in step content → returns Suspicious=true without calling shadow LLM | P0 | Pending |
| `UT-SA-601-BD-008` | System prompt contains instruction about boundary markers and untrusted data framing | P1 | Pending |
| `UT-SA-601-BD-009` | WrapOrFlag with empty content returns wrapped empty string without panic | P1 | Pending |
| `UT-SA-601-BD-010` | Content containing partial boundary marker (`<<<END_EVAL_` without full token+`>>>`) is NOT flagged as escape | P1 | Pending |

#### Truncation Group (TR)

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `UT-SA-601-TR-001` | Content shorter than MaxStepTokens is returned unchanged (no truncation) | P1 | Pending |
| `UT-SA-601-TR-002` | Content longer than MaxStepTokens preserves first N/2 runes and last N/2 runes with ellipsis | P0 | Pending |
| `UT-SA-601-TR-003` | MaxStepTokens=0 returns full content (no truncation) | P1 | Pending |
| `UT-SA-601-TR-004` | Unicode multi-byte runes (Chinese, emoji) are counted as single runes, not bytes | P1 | Pending |
| `UT-SA-601-TR-005` | Truncated output contains `…[truncated]…` marker between head and tail | P1 | Pending |

#### Correctness Group (CX)

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `UT-SA-601-CX-001` | NewObserver panics or returns error when passed nil evaluator, preventing nil deref in SubmitAsync | P0 | Pending |
| `UT-SA-601-CX-002` | NewInvestigatorWrapper panics or returns error when Inner or Evaluator is nil | P0 | Pending |
| `UT-SA-601-CX-003` | ToolProxy submits error content (err.Error()) to shadow when Execute fails, catching injection in error paths | P0 | Pending |
| `UT-SA-601-CX-004` | Config Validate rejects AlignmentCheck with VerdictTimeout <= 0 when enabled | P1 | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `internal/kubernautagent/alignment/investigator_wrapper.go`, `internal/kubernautagent/investigator/investigator.go`, `internal/kubernautagent/conversation/llm_adapter.go`, `cmd/kubernautagent/main.go`

| ID | Business Outcome Under Test | Priority | Phase |
|----|----------------------------|----------|-------|
| `IT-SA-601-FC-001` | Full investigation pipeline: slow shadow evaluator causes verdict timeout → HumanReviewNeeded=true with "verdict_timeout" in warnings | P0 | Pending |
| `IT-SA-601-FC-002` | Conversation adapter: final WaitForCompletion runs before successful return; timeout → alignment_warning SSE event | P1 | Pending |
| `IT-SA-601-BD-001` | Investigation executeTool wraps tool result in random boundary; boundary markers present in tool message content | P0 | Pending |
| `IT-SA-601-BD-002` | Investigation tool output containing closing boundary → executeTool returns error JSON and triggers alignment flag | P0 | Pending |
| `IT-SA-601-BD-003` | Conversation tool result wrapped in random boundary before appending to messages | P1 | Pending |
| `IT-SA-601-CX-001` | `setupAlignmentWiring` returns error when alignment enabled but shadow LLM creation fails (extracted from main.go; main calls os.Exit on error) | P0 | Pending |

### Tier 3: E2E Tests

**Deferred to #657**. Requires mock-LLM refactoring to support tool call scenarios for realistic injection testing via tool output path (Option C).

### Tier Skip Rationale

- **E2E**: Blocked on #657 (Extend mock-LLM to support tool call scenarios). Unit + Integration provide two-tier minimum with >=80% per-tier coverage. E2E will be implemented when #657 lands.

---

## 9. Test Cases

### UT-SA-601-FC-001: Evaluator fail-closed on retry exhaustion

**BR**: BR-AI-601
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/alignment/alignment_test.go`

**Preconditions**:
- Mock LLM client configured to return errors on all attempts

**Test Steps**:
1. **Given**: Evaluator with MaxRetries=3 and mock client that returns error on every call
2. **When**: `EvaluateStep(ctx, step)` is called
3. **Then**: Observation has `Suspicious=true` and Explanation contains "evaluator_unavailable" and "(fail-closed)"

**Acceptance Criteria**:
- **Behavior**: Evaluator escalates to suspicious on exhaustion (not silent false)
- **Correctness**: Explanation contains the last error for operator diagnosis
- **Accuracy**: `Suspicious` field is explicitly `true`, not zero-value `false`

---

### UT-SA-601-FC-005: Verdict suspicious when pending steps after timeout

**BR**: BR-AI-601
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/alignment/alignment_test.go`

**Preconditions**:
- Slow mock evaluator (500ms per step); short verdict timeout (50ms)

**Test Steps**:
1. **Given**: Observer with 3 submitted steps and a slow evaluator
2. **When**: `WaitForCompletion(50ms)` returns, then `RenderVerdict()` is called
3. **Then**: Verdict is `VerdictSuspicious` with `Pending > 0`, `TimedOut=true`, and Summary contains "verdict_timeout"

**Acceptance Criteria**:
- **Behavior**: Pending steps treated as suspicious (fail-closed)
- **Correctness**: `Pending = Submitted - len(Observations)`
- **Accuracy**: Summary message includes pending count for operator triage

---

### UT-SA-601-BD-007: Boundary escape → immediate suspicious without LLM

**BR**: BR-AI-601
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/alignment/alignment_test.go`

**Preconditions**:
- Mock LLM client with zero responses (should not be called)

**Test Steps**:
1. **Given**: Evaluator with boundary wrapping enabled
2. **When**: `EvaluateStep(ctx, step)` is called with step.Content containing `<<<END_EVAL_` + some hex + `>>>`
3. **Then**: Observation has `Suspicious=true`, Explanation contains "boundary escape", and mock LLM `chatCalls()==0`

**Acceptance Criteria**:
- **Behavior**: Escape detected by pre-scan; LLM never called
- **Correctness**: Zero LLM invocations
- **Accuracy**: Explanation clearly identifies the escape attempt

---

### IT-SA-601-FC-001: Full pipeline fail-closed on verdict timeout

**BR**: BR-AI-601
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/alignment_test.go`

**Preconditions**:
- InvestigatorWrapper with slow mock evaluator and short VerdictTimeout (50ms)
- Mock inner investigator that returns a clean result
- Observer context injected; LLMProxy and ToolProxy submit steps

**Test Steps**:
1. **Given**: Full decorator chain with wrapper, slow evaluator, and short timeout
2. **When**: `wrapper.Investigate(ctx, signal)` completes
3. **Then**: `result.HumanReviewNeeded=true`, `result.HumanReviewReason="alignment_check_failed"`, warning contains "verdict_timeout"

**Acceptance Criteria**:
- **Behavior**: Human review triggered by timeout, not by detection
- **Correctness**: Warning message distinguishes timeout from detection
- **Accuracy**: Inner investigation result preserved (RCASummary, Confidence intact)

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Mock LLM client (shadow), mock tool registry, mock investigation runner — external dependencies only
- **Location**: `test/unit/kubernautagent/alignment/`, `test/unit/kubernautagent/security/boundary/`
- **Anti-patterns avoided**: No `time.Sleep()` (use `Eventually()`), no `Skip()`, no direct audit store calls
- **Mock delay pattern**: Mock delays must use channel/select patterns (as in existing `slowMockLLMClient`), never `time.Sleep`. `time.After` in a `select` is acceptable.

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Mock LLM clients for investigation + shadow (external dependencies only); real business logic
- **Location**: `test/integration/kubernautagent/`
- **Anti-patterns avoided**: No HTTP endpoint testing, no direct audit infrastructure testing, no `time.Sleep()`

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact | Workaround |
|------------|------|--------|--------|------------|
| #657 | Infrastructure | Open | E2E tier blocked | Unit + Integration provide two-tier minimum |

### 11.2 Execution Order — TDD Phases

Each implementation phase is broken into individual TDD sub-phases (RED, GREEN, REFACTOR) with checkpoints.

---

#### PHASE 1: Foundation — Security Boundary Package + Head+Tail Truncation

**Phase 1A — TDD RED**: Write failing tests

| Test ID | File | What Fails |
|---------|------|------------|
| UT-SA-601-BD-001 | `test/unit/kubernautagent/security/boundary/boundary_test.go` | `boundary.Generate()` does not exist |
| UT-SA-601-BD-002 | same | `boundary.Wrap()` does not exist |
| UT-SA-601-BD-003 | same | `boundary.ContainsEscape()` does not exist |
| UT-SA-601-BD-004 | same | `boundary.WrapOrFlag()` does not exist |
| UT-SA-601-BD-005 | same | Uniqueness test fails (function does not exist) |
| UT-SA-601-BD-009 | same | `WrapOrFlag("")` does not exist |
| UT-SA-601-BD-010 | same | `ContainsEscape` for partial markers does not exist |
| UT-SA-601-TR-001 | `test/unit/kubernautagent/alignment/alignment_test.go` | `truncateHeadTail` does not exist or old behavior |
| UT-SA-601-TR-002 | same | Head+tail split not implemented |
| UT-SA-601-TR-003 | same | max=0 behavior |
| UT-SA-601-TR-004 | same | Unicode handling |
| UT-SA-601-TR-005 | same | Ellipsis marker |

**Deliverable**: All tests compile but fail (RED). `go test` exit code 1. Zero implementation code.

**Phase 1B — TDD GREEN**: Minimal implementation to pass

| File | What Changes |
|------|-------------|
| `internal/kubernautagent/security/boundary/boundary.go` | NEW: Implement `Generate`, `Wrap`, `ContainsEscape`, `WrapOrFlag` |
| `internal/kubernautagent/alignment/evaluator.go` | Replace `truncateRunes` with `truncateHeadTail` |

**Deliverable**: All Phase 1A tests pass (GREEN). Minimal code, no optimization.

**Phase 1C — TDD REFACTOR**: Polish without changing behavior

- Extract constants for boundary prefix/suffix markers
- Add doc comments
- Ensure `Generate` uses `crypto/rand` (not `math/rand`)
- Extract ellipsis constant

**Deliverable**: All tests still pass. Code is clean and well-documented.

---

**CHECKPOINT 1**: Rigorous due diligence

```
Audit Checklist:
- [ ] go build ./... succeeds
- [ ] All Phase 1 tests pass
- [ ] No existing tests broken (run full KA unit suite)
- [ ] Boundary package uses crypto/rand (security requirement)
- [ ] truncateHeadTail preserves exactly head+tail runes (verify with Unicode)
- [ ] No time.Sleep() in tests
- [ ] No Skip() in tests
- [ ] BR IDs consistent (BR-AI-601)
```

---

#### PHASE 2: Fail-Closed Core

**Phase 2A — TDD RED**: Write failing tests

| Test ID | File | What Fails |
|---------|------|------------|
| UT-SA-601-FC-001 | `test/unit/kubernautagent/alignment/alignment_test.go` | Evaluator returns Suspicious=false on exhaustion (current fail-open) |
| UT-SA-601-FC-002 | same | Evaluator does not short-circuit on cancelled context |
| UT-SA-601-FC-003 | same | Observer WaitForCompletion does not return WaitResult type |
| UT-SA-601-FC-004 | same | Observer has no Pending tracking |
| UT-SA-601-FC-005 | same | RenderVerdict returns VerdictClean when pending > 0 |
| UT-SA-601-FC-006 | same | Wrapper does not handle timeout/pending |
| UT-SA-601-FC-007 | same | Wrapper does not flag unavailable evaluations |
| UT-SA-601-FC-008 | same | Missing `suspicious` JSON field → Suspicious=false |
| UT-SA-601-FC-009 | same | Empty JSON `{}` → Suspicious=false |
| UT-SA-601-FC-010 | same | MaxRetries=0 coerced to 1 silently |
| UT-SA-601-FC-011 | same | Chat error path does not finalize alignment |

**Deliverable**: All new tests compile but fail (RED). Existing tests may also fail if signatures change — document expected breakage.

**Phase 2B — TDD GREEN**: Minimal implementation to pass

| File | What Changes |
|------|-------------|
| `internal/kubernautagent/alignment/evaluator.go` | Line 110-114: `Suspicious: true`; add JSON field validation; add `ctx.Err()` short-circuit |
| `internal/kubernautagent/alignment/observer.go` | New `WaitResult` return type; `submitted` counter; `WaitForCompletion` returns `WaitResult` |
| `internal/kubernautagent/alignment/types.go` | Add `Pending int`, `TimedOut bool` to `Verdict`; `WaitResult` struct |
| `internal/kubernautagent/alignment/investigator_wrapper.go` | Handle `WaitResult.Complete==false`; set HumanReviewNeeded on pending/timeout |
| `internal/kubernautagent/conversation/llm_adapter.go` | Final `WaitForCompletion` before return; increase timeout; fail-closed on timeout |
| `internal/kubernautagent/config/config.go` | Add VerdictTimeout validation |
| `cmd/kubernautagent/main.go` | `os.Exit(1)` when alignment enabled + shadow client fails |

**Deliverable**: All Phase 2A tests pass (GREEN). Existing tests updated where signatures changed.

**Phase 2C — TDD REFACTOR**: Polish without changing behavior

- Deduplicate fail-closed observation construction (extract helper)
- Improve structured log messages for fail-closed paths
- Update `emitAlignmentAudit` to emit events for unavailable/timeout observations
- Remove "Fail-open" comment, replace with "Fail-closed" documentation

**Deliverable**: All tests still pass. Structured logging verified. Audit events complete.

---

**CHECKPOINT 2**: Rigorous due diligence

```
Audit Checklist:
- [ ] go build ./... succeeds
- [ ] All Phase 1 + Phase 2 tests pass
- [ ] Full KA unit suite passes (no regressions)
- [ ] Every evaluator error path returns Suspicious=true (grep for "Suspicious:.*false")
- [ ] Every observer timeout path reports Complete=false
- [ ] Every wrapper path with pending/timeout sets HumanReviewNeeded
- [ ] Conversation adapter calls final WaitForCompletion before all return paths
- [ ] main.go exits fatally on shadow client failure when enabled
- [ ] Config validation rejects invalid verdictTimeout
- [ ] No time.Sleep() in tests
- [ ] No fail-open paths remain (adversarial review of all return statements)
```

---

#### PHASE 3: Random Boundaries — All Agents

**Phase 3A — TDD RED**: Write failing tests

| Test ID | File | What Fails |
|---------|------|------------|
| UT-SA-601-BD-006 | `test/unit/kubernautagent/alignment/alignment_test.go` | Evaluator does not wrap content in boundary |
| UT-SA-601-BD-007 | same | Evaluator does not detect boundary escape |
| UT-SA-601-BD-008 | same | System prompt does not mention boundary markers |
| IT-SA-601-BD-001 | `test/integration/kubernautagent/alignment_test.go` | Investigation executeTool does not wrap result |
| IT-SA-601-BD-002 | same | Investigation does not detect boundary escape in tool output |
| IT-SA-601-BD-003 | same | Conversation tool result not wrapped |

**Deliverable**: All new tests compile but fail (RED).

**Phase 3B — TDD GREEN**: Minimal implementation to pass

| File | What Changes |
|------|-------------|
| `internal/kubernautagent/alignment/evaluator.go` | Import boundary package; call `WrapOrFlag` before building userMsg; return escape observation if escaped |
| `internal/kubernautagent/alignment/prompt/system.go` | Add boundary-aware instruction + data exfiltration classification rule #7 to system prompt |
| `internal/kubernautagent/investigator/investigator.go` | In `executeTool`: wrap result with `boundary.WrapOrFlag`; return error JSON on escape |
| `internal/kubernautagent/conversation/llm_adapter.go` | After `toolRegistry.Execute`: wrap result with `boundary.WrapOrFlag`; emit alignment_warning on escape |

**Deliverable**: All Phase 3A tests pass (GREEN).

**Phase 3C — TDD REFACTOR**: Polish without changing behavior

- Extract shared boundary wrapping helper used by investigator and conversation
- Update few-shot examples in system prompt to show boundary-wrapped content
- Ensure boundary instruction is clear and concise

**Deliverable**: All tests still pass. Shared helper eliminates duplication.

---

**CHECKPOINT 3**: Rigorous due diligence

```
Audit Checklist:
- [ ] go build ./... succeeds
- [ ] All Phase 1 + 2 + 3 tests pass
- [ ] Full KA unit suite passes
- [ ] Boundary wrapping verified in evaluator captured requests (contains <<<EVAL_ markers)
- [ ] Boundary escape pre-scan verified (LLM not called when escape detected)
- [ ] Investigation executeTool wraps all tool output (grep for boundary.WrapOrFlag in investigator.go)
- [ ] Conversation adapter wraps all tool output
- [ ] System prompt contains boundary instruction
- [ ] Few-shot examples updated with boundary markers
- [ ] No time.Sleep() in tests
```

---

#### PHASE 4: Correctness Fixes

**Phase 4A — TDD RED**: Write failing tests

| Test ID | File | What Fails |
|---------|------|------------|
| UT-SA-601-CX-001 | `test/unit/kubernautagent/alignment/alignment_test.go` | NewObserver does not panic/error on nil evaluator |
| UT-SA-601-CX-002 | same | NewInvestigatorWrapper does not panic/error on nil |
| UT-SA-601-CX-003 | same | ToolProxy does not submit error content to shadow |
| UT-SA-601-CX-004 | same | Config does not validate verdictTimeout |
| IT-SA-601-CX-001 | `test/integration/kubernautagent/alignment_test.go` | main.go does not exit fatally |

**Deliverable**: All new tests compile but fail (RED).

**Phase 4B — TDD GREEN**: Minimal implementation to pass

| File | What Changes |
|------|-------------|
| `internal/kubernautagent/alignment/observer.go` | `NewObserver`: panic if evaluator nil |
| `internal/kubernautagent/alignment/investigator_wrapper.go` | `NewInvestigatorWrapper`: panic if Inner or Evaluator nil |
| `internal/kubernautagent/alignment/toolproxy.go` | Remove early return on error; submit error content to shadow |
| `internal/kubernautagent/alignment/evaluator.go` | Make prompt immutable (move to constructor parameter) |
| `internal/kubernautagent/config/config.go` | Add VerdictTimeout validation |
| `cmd/kubernautagent/main.go` | Fatal on shadow client failure |

**Deliverable**: All Phase 4A tests pass (GREEN).

**Phase 4C — TDD REFACTOR**: Polish without changing behavior

- Standardize all BR references to BR-AI-601 (replace BR-SEC-601)
- Update brittle `HaveLen(11)` in audit emitter test to use `ContainElements`
- Add comments documenting nil guard rationale
- Clean up WithSystemPrompt removal (update all callers)

**Deliverable**: All tests still pass. BR IDs consistent. Audit test not brittle.

---

**CHECKPOINT 4**: Rigorous due diligence

```
Audit Checklist:
- [ ] go build ./... succeeds
- [ ] All Phase 1 + 2 + 3 + 4 tests pass
- [ ] Full KA unit suite passes (including updated audit emitter test)
- [ ] NewObserver(nil) panics (verified in test)
- [ ] NewInvestigatorWrapper with nil Inner panics (verified in test)
- [ ] ToolProxy submits error content to shadow (verified in test)
- [ ] All BR references are BR-AI-601 (grep for BR-SEC-601 returns 0 results)
- [ ] WithSystemPrompt method removed; prompt set in constructor
- [ ] main.go alignment wiring updated for new constructor signature
- [ ] No time.Sleep() in tests
- [ ] No Skip() in tests
```

---

#### PHASE 5: Existing Test Updates + Gap Coverage

**Phase 5A — TDD RED**: Update existing tests for new behavior + add exfiltration payloads P11-P13

| Existing Test | What Breaks | Required Update |
|---------------|-------------|-----------------|
| UT-SA-601-003 (truncation) | `truncateRunes` replaced by `truncateHeadTail` | Update assertions for head+tail behavior |
| UT-SA-601-006 (partial timeout) | `WaitForCompletion` returns `WaitResult` not `[]Observation` | Update to use `WaitResult.Observations` |
| UT-SA-601-009 (config MaxStepTokens) | Same truncation behavior change | Update assertions |
| UT-SA-601-014 (wrapper clean) | Constructor signature changed (prompt in constructor) | Update evaluator construction |
| UT-SA-601-PAYLOAD-* | `WithSystemPrompt` removed | Update to new constructor |
| All existing tests | BR-SEC-601 → BR-AI-601 | Update Describe strings |

**Deliverable**: Document all existing test breakage. Tests updated to compile with new APIs.

**Phase 5B — TDD GREEN**: Fix tests to pass with new behavior

- Update all mock constructions for new constructor signatures
- Update assertions for WaitResult type
- Update truncation assertions for head+tail
- Verify all 14 existing tests + 10 payload tests pass

**Deliverable**: All existing + new tests pass (GREEN).

**Phase 5C — TDD REFACTOR**: Consolidate test helpers

- Deduplicate mock construction across test files
- Extract common evaluator/observer setup helpers
- Ensure consistent test naming pattern
- Review test descriptions for clarity

**Deliverable**: All tests still pass. Test code is clean and maintainable.

---

**CHECKPOINT 5 (FINAL)**: Comprehensive audit

```
Audit Checklist — FINAL:
- [ ] go build ./... succeeds with zero errors
- [ ] golangci-lint run --timeout=5m passes (no new lint errors)
- [ ] Full KA unit suite: 100% pass rate
- [ ] Full KA integration suite: 100% pass rate (if applicable tests exist)
- [ ] Per-tier coverage assessment:
      - [ ] Unit-testable alignment code: >=80%
      - [ ] Unit-testable boundary code: >=80%
      - [ ] Integration-testable wrapper/wiring code: >=80%
- [ ] ADVERSARIAL REVIEW:
      - [ ] grep "Suspicious:.*false" in evaluator.go → only in success path (valid JSON with explicit field)
      - [ ] grep "fail-open\|fail.open" in alignment/ → 0 results
      - [ ] Every WaitForCompletion caller checks Complete field
      - [ ] Every boundary wrap point calls WrapOrFlag (not just Wrap)
      - [ ] Every return path in Respond() (conversation) has alignment finalization
- [ ] ANTI-PATTERN CHECK:
      - [ ] Zero time.Sleep() in test files
      - [ ] Zero Skip() in test files
      - [ ] Zero direct audit store calls in tests (test business logic only)
      - [ ] All tests use Ginkgo/Gomega BDD framework
- [ ] TRACEABILITY:
      - [ ] Every test has BR-AI-601 reference
      - [ ] Every test has unique test ID
      - [ ] All P0 tests implemented
      - [ ] Coverage matrix in this plan updated with final status
```

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/601/TEST_PLAN_v2.md` | Strategy, test design, TDD phases |
| Boundary unit tests | `test/unit/kubernautagent/security/boundary/boundary_test.go` | NEW |
| Alignment unit tests | `test/unit/kubernautagent/alignment/alignment_test.go` | UPDATED with FC, BD, TR, CX tests |
| Payload unit tests | `test/unit/kubernautagent/alignment/payload_test.go` | UPDATED BR IDs + constructor |
| Integration tests | `test/integration/kubernautagent/alignment_test.go` | NEW or UPDATED |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Phase 1: Boundary + Truncation
go test ./test/unit/kubernautagent/security/boundary/... -ginkgo.v
go test ./test/unit/kubernautagent/alignment/... -ginkgo.v -ginkgo.focus="TR-"

# Phase 2: Fail-Closed
go test ./test/unit/kubernautagent/alignment/... -ginkgo.v -ginkgo.focus="FC-"

# Phase 3: Boundaries Integration
go test ./test/unit/kubernautagent/alignment/... -ginkgo.v -ginkgo.focus="BD-"
go test ./test/integration/kubernautagent/... -ginkgo.v -ginkgo.focus="BD-"

# Phase 4: Correctness
go test ./test/unit/kubernautagent/alignment/... -ginkgo.v -ginkgo.focus="CX-"

# Full suite
go test ./test/unit/kubernautagent/... -ginkgo.v
go test ./test/integration/kubernautagent/... -ginkgo.v

# Coverage
go test ./internal/kubernautagent/alignment/... -coverprofile=alignment_coverage.out
go test ./internal/kubernautagent/security/... -coverprofile=boundary_coverage.out
go tool cover -func=alignment_coverage.out
go tool cover -func=boundary_coverage.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| UT-SA-601-003 (alignment_test.go) | `truncateRunes` head-only behavior | Update for head+tail split + ellipsis | Truncation strategy changed to head+tail |
| UT-SA-601-006 (alignment_test.go) | `WaitForCompletion` returns `[]Observation` | Update to use `WaitResult.Observations` | Observer API change for completion tracking |
| UT-SA-601-009 (alignment_test.go) | `MaxStepTokens=10` truncation check | Update for head+tail behavior | Same truncation change |
| UT-SA-601-014 (alignment_test.go) | `alignment.NewEvaluator(...).WithSystemPrompt(...)` | Change to `alignment.NewEvaluator(client, cfg, prompt)` | WithSystemPrompt removed; prompt in constructor |
| UT-SA-601-PAYLOAD-* (payload_test.go) | `...WithSystemPrompt(alignprompt.SystemPrompt())` | Change to `NewEvaluator(client, cfg, alignprompt.SystemPrompt())` | Same constructor change |
| All (alignment_test.go) | `BR-SEC-601` in Describe strings | Replace with `BR-AI-601` | BR ID standardization |
| All (payload_test.go) | `BR-SEC-601` in Describe strings | Replace with `BR-AI-601` | BR ID standardization |
| Emitter test (emitter_test.go) | `HaveLen(11)` | Replace with `ContainElements(EventTypeAlignmentStep, EventTypeAlignmentVerdict)` | Brittle assertion |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
| 2.0 | 2026-03-04 | Audit remediation: fail-closed, random boundaries, head+tail truncation, correctness fixes. TDD phases broken into RED/GREEN/REFACTOR. Checkpoints added. Anti-pattern compliance. |
| 2.1 | 2026-04-09 | F1-F19 findings: BD-009/BD-010, FC-011, P11-P13 exfiltration payloads, two-tier claim fix, template path fix, todo_write exclusion, mock delay pattern, dual ID note, IT-CX-001 approach (extract wiring), shadow-raw + evaluator ordering design decisions, finding-to-test traceability. |

---

## 16. Finding-to-Test Traceability

| Finding | Severity | Test ID(s) | Notes |
|---------|----------|------------|-------|
| F1: os.Exit testing | CRITICAL | IT-SA-601-CX-001 | Extract `setupAlignmentWiring` from main.go |
| F2: Shadow observes raw | RESOLVED | (design decision) | Documented in Section 4.3 |
| F3: stepIdx counter | HIGH | UT-SA-601-FC-003, FC-004 | Derive Submitted from stepIdx.Load() |
| F4: Two-tier claim | HIGH | (Section 5.2 update) | Narrowed to integration-testable surfaces |
| F5: Template path | HIGH | (Phase 3B fix) | Corrected to incident_investigation.tmpl |
| F6: Chat error path | HIGH | UT-SA-601-FC-011 | Alignment finalization on Chat error |
| F7: todo_write bypass | HIGH | (Section 4.2 exclusion) | Documented as design decision |
| F8: Stale IMPLEMENTATION_PLAN | MEDIUM | (deprecation header) | Superseded by v2 plan |
| F9: `*bool` JSON validation | HIGH | UT-SA-601-FC-008, FC-009 | `Suspicious *bool` in evalResponse |
| F10: Mock delay pattern | MEDIUM | (Section 10.1 note) | Channel/select, no Sleep |
| F11: WrapOrFlag empty | MEDIUM | UT-SA-601-BD-009 | Empty content edge case |
| F12: Partial marker | MEDIUM | UT-SA-601-BD-010 | Substring not flagged |
| F13: Finding traceability | MEDIUM | (Section 16) | This appendix |
| F14: Dual ID schemes | LOW | (Section 8 note) | Legacy + group IDs coexist |
| F15: Single snapshot | HIGH | UT-SA-601-FC-003, FC-005 | RenderVerdict(WaitResult) |
| F16/F18: Dead code | LOW | (Phase 4C refactor) | llm_adapter.go:285 |
| F19: Exfiltration payloads | MEDIUM | UT-SA-601-PAYLOAD-P11, P12, P13 | System prompt rule #7 |
