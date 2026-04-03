# Test Plan: AA Controller Treats "Not Actionable" LLM Response as Failure

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-607-v2
**Feature**: Remove confidence gate from `actionable=false` path + Go agent confidence floor for defense-in-depth
**Version**: 2.0
**Created**: 2026-03-04
**Updated**: 2026-03-04
**Author**: AI Assistant
**Status**: Passed
**Branch**: `development/v1.3`

---

## 1. Introduction

### 1.1 Purpose

When the LLM determines that an alert is **not actionable** (e.g., orphaned PVCs from completed batch jobs), the AA response processor falls through to `handleNoWorkflowTerminalFailure` instead of `handleNotActionableFromIncident` because the LLM omits the `confidence` field in the response, causing it to default to 0.0 — which fails the `>= 0.7` gate on line 121 of `response_processor.go`. This causes the RR to escalate to `ManualReviewRequired` instead of completing as `NoActionRequired`.

The fix has two parts (both in Go, targeting v1.3):
- **AA processor** (`response_processor.go`): Make the `actionable=false` check independent of the confidence threshold, matching the same pattern used for `needs_human_review=true` (line 94) — explicit LLM signals are trusted without confidence gating.
- **Go Kubernaut Agent parser** (`parser.go`): Parse the `actionable` field from LLM JSON, synthesize the standard warning string, set `IsActionable=false`, and apply a confidence floor of 0.8 when `actionable=false` (defense-in-depth, replacing the Python HAPI's equivalent logic).

### 1.2 Scope Change from v1.0

**v1.0** (branch `fix/v1.2.0-rc3`): Fix split between Go (AA processor) and Python (HAPI result_parser).
**v2.0** (branch `development/v1.3`): Entire fix in Go. The Python HAPI confidence floor logic is replaced by equivalent Go logic in the Kubernaut Agent parser. Python tests (UT-HAPI-607-001..004) are superseded by Go KA parser tests (UT-KA-607-001..005).

### 1.3 Objectives

1. **Not-actionable routing correctness**: When the LLM signals `actionable=false` with any confidence value (including 0.0/omitted), the AA phase must be `Completed` with `Reason=WorkflowNotNeeded`, `SubReason=NotActionable`.
2. **No regression for other paths**: Human review, resolved, low-confidence, and normal workflow paths remain unaffected.
3. **KA parser confidence floor**: When `actionable=false`, the Go agent parser always emits `confidence >= 0.8` regardless of what the LLM provided.
4. **KA parser signal synthesis**: When `actionable=false`, the Go agent parser synthesizes the "Alert not actionable" warning and sets `IsActionable=false` on `InvestigationResult`.
5. **Response mapping completeness**: `mapInvestigationResultToResponse` maps `IsActionable` and `Warnings` to the `IncidentResponse` OpenAPI contract.

### 1.4 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/aianalysis/...` + `go test ./test/unit/kubernautagent/parser/...` |
| Regression pass rate | 100% | Existing UT-AA-388-001/002 tests still pass |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on response_processor.go and parser.go branching logic |
| Backward compatibility | 0 regressions | All 316 AA specs + all 16 parser specs pass |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-HAPI-200**: Investigation outcome routing (Outcome D — not actionable)
- **Issue #388**: Original implementation of `actionable=false` path
- **Issue #607**: Bug — AA controller treats not-actionable LLM response as failure
- **Issue #433**: Go language migration (Kubernaut Agent)
- **DD-CONTROLLER-001**: ObservedGeneration contract

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- Existing tests: `test/unit/aianalysis/investigating_handler_test.go` (UT-AA-388-001, UT-AA-388-002)
- KA parser tests: `test/unit/kubernautagent/parser/parser_test.go`

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Removing confidence gate lets bogus `actionable=false` through | RR incorrectly completes as NoActionRequired for a real problem | Low | UT-AA-607-003 | `actionable=false` requires BOTH the warning signal AND `is_actionable=false` — this is a dual-field check, not just one flag. The agent only sets both when LLM explicitly says `actionable: false`. |
| R2 | KA confidence floor masks real parser bugs | Confidence is always overwritten, hiding cases where the LLM genuinely returned low confidence | Low | UT-KA-607-004 | Floor only applies when `actionable=false`. All other outcomes preserve the LLM's original confidence. |
| R3 | Removing confidence gate inadvertently affects resolved/human-review paths | Existing Outcome A (resolved) or human-review path regresses | Low | UT-AA-607-004, UT-AA-607-005 | No branch reordering — only the `actionable=false` condition is relaxed. Dedicated regression tests verify resolved and terminal-failure paths are unaffected. Existing UT-AA-388-001/002 remain unmodified. |
| R4 | Operator notification behavior change | Before fix: benign alerts escalate to `ManualReviewRequired`, generating a Slack/webhook notification. After fix: benign alerts complete silently as `NoActionRequired` with no notification. | Low | N/A (intentional) | This is the correct behavior per BR-ORCH-037. |

### 3.1 Risk-to-Test Traceability

- **R1** → UT-AA-607-003 (warning-only without `is_actionable=false` must NOT route to NotActionable)
- **R2** → UT-KA-607-004 (floor does NOT apply for `actionable=true` or absent)
- **R3** → UT-AA-607-004, UT-AA-607-005 (resolved and terminal-failure paths unaffected)

---

## 4. Scope

### 4.1 Features to be Tested

- **AA Response Processor** (`pkg/aianalysis/handlers/response_processor.go`): `ProcessIncidentResponse` branching — the `actionable=false` path must work independently of confidence threshold.
- **KA Parser** (`internal/kubernautagent/parser/parser.go`): Parse `actionable` field, synthesize warning, set `IsActionable`, apply confidence floor of 0.8.
- **KA Types** (`internal/kubernautagent/types/types.go`): `InvestigationResult` must carry `IsActionable *bool` and `Warnings []string`.
- **KA Handler** (`internal/kubernautagent/server/handler.go`): `mapInvestigationResultToResponse` must map `IsActionable` and `Warnings` to the `IncidentResponse` OpenAPI contract.

### 4.2 Features Not to be Tested

- **RO handleWorkflowNotNeeded**: Already works correctly. No change needed.
- **Python HAPI result_parser**: Superseded by Go KA parser in v1.3.
- **E2E orphaned-pvc-no-action scenario**: Deferred to demo-scenarios team validation.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Remove `resp.Confidence >= 0.7` from the `actionable=false` condition | Mirrors `needs_human_review=true` — explicit LLM determinations are authoritative signals not gated by quality thresholds. |
| Keep confidence gate for `ProblemResolved` | Resolved is inferred from `investigation_outcome`, which is less authoritative than `actionable: false`. |
| KA confidence floor = 0.8 (same as Python HAPI) | Provides headroom above 0.7. Represents "parser is confident enough in its determination." |
| `applyActionableSignals` handles both flat and nested LLM JSON formats | Both code paths (`Parse` direct and `parseLLMFormat`) share the same signal logic. |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of branching logic in `ProcessIncidentResponse` and `applyActionableSignals`.
- **Integration**: Not applicable — no I/O changes.
- **E2E**: Deferred.

### 5.2 Two-Tier Minimum

- **Go Unit tests (KA)**: 7 specs covering parser confidence floor, signal synthesis, and response mapping
- **Go Unit tests (AA)**: 5 specs covering bug fix and regression guards
- **Integration**: Not applicable (pure logic change)

### 5.3 Business Outcome Quality Bar

Each test validates **operator-visible outcomes**: phase, reason, actionability (AA side), and correctness of signal synthesis/confidence floor (KA side).

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:
1. All P0 tests pass (0 failures)
2. All P1 tests pass
3. Existing UT-AA-388-001 and UT-AA-388-002 pass without modification
4. `go build ./...` succeeds
5. All 316 AA specs pass, all 16 KA parser specs pass

**FAIL** — any of:
1. Any P0 test fails
2. Any existing #388 test regresses
3. Code does not compile

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/aianalysis/handlers/response_processor.go` | `ProcessIncidentResponse` (branching at lines 94-130), `handleNotActionableFromIncident` | ~80 |
| `internal/kubernautagent/parser/parser.go` | `Parse`, `parseLLMFormat`, `applyActionableSignals` | ~50 |
| `internal/kubernautagent/server/handler.go` | `mapInvestigationResultToResponse` (IsActionable/Warnings mapping) | ~15 |
| `internal/kubernautagent/types/types.go` | `InvestigationResult` struct definition | ~10 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-200 (Outcome D) | KA parser applies confidence floor when actionable=false without confidence | P0 | Unit (KA) | UT-KA-607-001 | Passed |
| BR-HAPI-200 (Outcome D) | KA parser applies confidence floor when actionable=false with low confidence | P0 | Unit (KA) | UT-KA-607-002 | Passed |
| BR-HAPI-200 (Outcome D) | KA parser synthesizes warning and sets IsActionable=false | P0 | Unit (KA) | UT-KA-607-003 | Passed |
| BR-HAPI-200 | KA parser does NOT apply floor for actionable=true or absent | P1 | Unit (KA) | UT-KA-607-004 | Passed |
| BR-HAPI-200 (Outcome D) | InvestigationResult carries IsActionable and Warnings for response mapping | P1 | Unit (KA) | UT-KA-607-005 | Passed |
| BR-HAPI-200 (Outcome D) | Not-actionable with confidence < 0.7 routes to Completed/NotActionable | P0 | Unit (AA) | UT-AA-607-001 | Passed |
| BR-HAPI-200 (Outcome D) | Not-actionable with confidence = 0.0 (omitted) routes correctly | P0 | Unit (AA) | UT-AA-607-002 | Passed |
| BR-HAPI-200 (Outcome D) | Partial signal (warning only, no is_actionable) must NOT route to NotActionable | P0 | Unit (AA) | UT-AA-607-003 | Passed |
| BR-HAPI-200 (Outcome A) | Resolved path still requires confidence >= 0.7 (regression guard) | P1 | Unit (AA) | UT-AA-607-004 | Passed |
| BR-HAPI-200 | Terminal failure path unaffected (regression guard) | P1 | Unit (AA) | UT-AA-607-005 | Passed |

---

## 8. Test Scenarios

### Test ID Naming Convention

- **KA parser tests**: `UT-KA-607-{NNN}` — Go Kubernaut Agent parser unit tests
- **AA processor tests**: `UT-AA-607-{NNN}` — AA response processor unit tests

### Tier 1: Unit Tests (Go — KA Parser + Response Mapping)

**Testable code scope**: `applyActionableSignals` in `parser.go`, `InvestigationResult` in `types.go`, `mapInvestigationResultToResponse` in `handler.go`

| ID | Business Outcome Under Test | Status |
|----|----------------------------|--------|
| `UT-KA-607-001` | Parser applies confidence floor (0.8) when LLM returns `actionable: false` without confidence field | Passed |
| `UT-KA-607-002` | Parser applies confidence floor (0.8) when LLM returns `actionable: false` with low confidence (0.3) | Passed |
| `UT-KA-607-003` | Parser synthesizes "Alert not actionable" warning and sets `IsActionable=false` | Passed |
| `UT-KA-607-004` | Parser does NOT apply floor for `actionable: true` or absent — original confidence preserved (2 sub-specs) | Passed |
| `UT-KA-607-005` | `InvestigationResult` carries `IsActionable` and `Warnings` when actionable=false, nil/empty when absent (2 sub-specs) | Passed |

### Tier 1: Unit Tests (Go — AA Response Processor)

**Testable code scope**: `ProcessIncidentResponse` branching logic in `response_processor.go`

| ID | Business Outcome Under Test | Status |
|----|----------------------------|--------|
| `UT-AA-607-001` | Operator sees `NoActionRequired` when LLM says not-actionable with low confidence (0.4) | Passed |
| `UT-AA-607-002` | Operator sees `NoActionRequired` when LLM omits confidence entirely (0.0) | Passed |
| `UT-AA-607-003` | System escalates to ManualReview when only the warning is present but `is_actionable` is missing | Passed |
| `UT-AA-607-004` | Resolved path still requires confidence >= 0.7 (no regression) | Passed |
| `UT-AA-607-005` | Terminal failure path fires when no workflow, no signals, low confidence (no regression) | Passed |

### Tier Skip Rationale

- **Integration**: Not applicable. The fix changes pure branching logic. No I/O, HTTP, database, or Kubernetes API calls are affected.
- **E2E**: Deferred. Requires mock LLM scenario configuration out of scope for this fix.

---

## 9. Test Cases

### UT-KA-607-001: Parser applies confidence floor when actionable=false without confidence

**BR**: BR-HAPI-200 (Outcome D)
**Priority**: P0
**File**: `test/unit/kubernautagent/parser/parser_test.go`

**Preconditions**: LLM JSON with `"actionable": false` and no `"confidence"` key.

**Test Steps**:
1. **Given**: `{"rca_summary": "Orphaned PVCs from completed batch jobs", "actionable": false}`
2. **When**: `Parse()` is called
3. **Then**: `result.Confidence >= 0.8`

**Acceptance**: Defense-in-depth floor of 0.8 applied when LLM omits confidence for not-actionable.

### UT-KA-607-002: Parser applies confidence floor when actionable=false with low confidence

**BR**: BR-HAPI-200 (Outcome D)
**Priority**: P0
**File**: `test/unit/kubernautagent/parser/parser_test.go`

**Preconditions**: LLM JSON with `"actionable": false` and `"confidence": 0.3`.

**Test Steps**:
1. **Given**: `{"rca_summary": "...", "actionable": false, "confidence": 0.3}`
2. **When**: `Parse()` is called
3. **Then**: `result.Confidence >= 0.8`

**Acceptance**: Floor overrides low LLM confidence for actionable=false.

### UT-KA-607-003: Parser synthesizes warning and sets IsActionable=false

**BR**: BR-HAPI-200 (Outcome D)
**Priority**: P0
**File**: `test/unit/kubernautagent/parser/parser_test.go`

**Test Steps**:
1. **Given**: LLM JSON with `"actionable": false`
2. **When**: `Parse()` is called
3. **Then**: `result.IsActionable != nil && *result.IsActionable == false`, `result.Warnings` contains "Alert not actionable"

**Acceptance**: Standard warning synthesized, IsActionable pointer set.

### UT-KA-607-004: Parser does NOT apply floor for actionable=true or absent

**BR**: BR-HAPI-200
**Priority**: P1
**File**: `test/unit/kubernautagent/parser/parser_test.go`

**Test Steps** (2 sub-specs):
1. `actionable: true, confidence: 0.3` → `result.Confidence == 0.3`
2. `actionable absent, confidence: 0.4` → `result.Confidence == 0.4`

**Acceptance**: Floor only applies for actionable=false.

### UT-KA-607-005: InvestigationResult carries IsActionable and Warnings for response mapping

**BR**: BR-HAPI-200 (Outcome D)
**Priority**: P1
**File**: `test/unit/kubernautagent/parser/parser_test.go`

**Test Steps** (2 sub-specs):
1. `actionable: false` → `IsActionable != nil, *IsActionable == false, Warnings non-empty`
2. `actionable absent` → `IsActionable == nil, Warnings empty`

**Acceptance**: Fields populated correctly for downstream `mapInvestigationResultToResponse`.

### UT-AA-607-001: Not-actionable with low confidence routes to Completed/NotActionable

**BR**: BR-HAPI-200 (Outcome D)
**Priority**: P0
**File**: `test/unit/aianalysis/investigating_handler_test.go`

**Preconditions**: Mock client returns `Confidence=0.4`, `Warnings=["Alert not actionable — no remediation warranted"]`, `IsActionable=false`.

**Expected**: `Phase=Completed`, `Reason=WorkflowNotNeeded`, `SubReason=NotActionable`, `NeedsHumanReview=false`.

### UT-AA-607-002: Not-actionable with zero confidence routes correctly

**BR**: BR-HAPI-200 (Outcome D)
**Priority**: P0
**File**: `test/unit/aianalysis/investigating_handler_test.go`

**Preconditions**: Mock client returns `Confidence=0.0`, `Warnings=["Alert not actionable — no remediation warranted"]`, `IsActionable=false`.

**Expected**: `Phase=Completed`, `Reason=WorkflowNotNeeded`, `NeedsHumanReview=false`.

### UT-AA-607-003: Partial signal (warning only) does NOT route to NotActionable

**BR**: BR-HAPI-200 (Outcome D)
**Priority**: P0
**File**: `test/unit/aianalysis/investigating_handler_test.go`

**Preconditions**: Mock client returns warning text but `IsActionable` is NOT SET.

**Expected**: `Phase=Failed`, `Reason=WorkflowResolutionFailed`, `NeedsHumanReview=true`.

### UT-AA-607-004: Resolved path still requires confidence >= 0.7

**BR**: BR-HAPI-200 (Outcome A)
**Priority**: P1
**File**: `test/unit/aianalysis/investigating_handler_test.go`

**Preconditions**: "Problem self-resolved" warning, `Confidence=0.4`.

**Expected**: `Phase=Failed`, `Reason != WorkflowNotNeeded`.

### UT-AA-607-005: Terminal failure path fires normally

**BR**: BR-HAPI-200
**Priority**: P1
**File**: `test/unit/aianalysis/investigating_handler_test.go`

**Preconditions**: No workflow, no signals, `Confidence=0.3`.

**Expected**: `Phase=Failed`, `Reason=WorkflowResolutionFailed`, `NeedsHumanReview=true`.

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Go**: 1.23+
- **Ginkgo CLI**: v2.x
- **Worktree**: `kubernaut-v1.3` on `development/v1.3` branch

---

## 11. Execution

```bash
# KA parser tests
go test -v ./test/unit/kubernautagent/parser/...

# AA processor tests (all)
go test -v ./test/unit/aianalysis/...

# Build validation
go build ./...
```

---

## 12. Test Results Summary

| Suite | Total Specs | Passed | Failed | Status |
|-------|-------------|--------|--------|--------|
| KA Parser | 16 | 16 | 0 | PASSED |
| AA Processor | 316 | 316 | 0 | PASSED |
| KA Server/Handler | 3 | 3 | 0 | PASSED |
| Full Build | — | — | — | PASSED |

---

## 13. Superseded Tests

The following tests from TP-607-v1 (branch `fix/v1.2.0-rc3`) are **superseded** by the Go-only implementation:

| Original ID | Superseded By | Reason |
|-------------|---------------|--------|
| UT-HAPI-607-001 | UT-KA-607-001 | Python HAPI confidence floor replaced by Go parser |
| UT-HAPI-607-002 | UT-KA-607-002 | Python HAPI confidence floor replaced by Go parser |
| UT-HAPI-607-003 | UT-KA-607-003 | Pattern 2B covered by Go parser (both flat + nested formats) |
| UT-HAPI-607-004 | UT-KA-607-004 | Non-actionable regression guard in Go parser |

---

## 14. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 2.0 | 2026-03-04 | v1.3 Go-only rewrite: Replaced Python HAPI tests with Go KA parser tests (UT-KA-607-001..005). Added `InvestigationResult.IsActionable` and `Warnings` fields. Updated scope, approach, and results. Superseded UT-HAPI-607-001..004. Branch changed from `fix/v1.2.0-rc3` to `development/v1.3`. |
| 1.1 | 2026-03-04 | Due diligence review: Corrected fix description, added R4 risk, documented Python test helpers, clarified deprecated parser exclusion. |
| 1.0 | 2026-03-04 | Initial test plan |
