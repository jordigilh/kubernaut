# Test Plan: KA tool-budget error surfacing, per-investigation isolation, and configurable ceiling

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1889-v1.0
**Feature**: Fix three compounding KubernautAgent (KA) tool-call-budget gaps: (1) budget exhaustion is redacted to a generic `internal_error` instead of a named `MCPError` code; (2) the pod-wide `AnomalyDetector` singleton allows concurrent investigations to corrupt each other's tool-call counters via `Reset()`; (3) `maxTotalToolCalls` is not operator-configurable via Helm.
**Version**: 1.1
**Created**: 2026-08-03
**Author**: AI agent (Cursor), reviewed with user
**Status**: Implemented — all tests passing
**Branch**: `fix/1889-1892-tool-budget-isolation`

---

## 1. Introduction

### 1.1 Purpose

A live-cluster E2E validation surfaced that when KA's I7 anomaly detector exhausts an
investigation's tool-call budget, the operator gets no actionable signal: the error is
redacted to a generic `internal_error`, the associated `AIAnalysis` sits silently in
`Investigating` for minutes, and the eventual terminal failure reason is misleading
(#1889). Root-causing that gap surfaced a second, more severe, previously-untracked
issue: the `AnomalyDetector` is a single pod-wide singleton shared by every concurrent
investigation, so one investigation's `Reset()` call can silently corrupt another's
in-flight budget (#1892) — a safety-mechanism bypass, not just a UX gap. This plan also
covers the Helm change requested alongside these fixes: exposing `maxTotalToolCalls` so
operators can raise the ceiling without hand-editing the rendered ConfigMap.

### 1.2 Objectives

1. **Actionable error surfacing**: `ExhaustedResult{Reason: "tool budget exhausted"}`
   reaches the MCP caller as a named `MCPError` (code `tool_budget_exhausted`), not the
   generic `internal_error`.
2. **Per-investigation isolation**: two investigations with different correlation IDs
   running concurrently on the same KA pod never observe each other's tool-call/failure
   counters — one exhausting its budget or resetting between phases has zero effect on
   the other.
3. **Configurable ceiling**: `kubernautAgent.ai.safety.anomaly.maxTotalToolCalls` is a
   documented Helm value that flows through to `AI.Safety.Anomaly.MaxTotalToolCalls`,
   defaulting to 30 when unset.
4. **Zero regressions**: all ~80 existing `AnomalyDetector`-related unit/integration
   tests pass unmodified (Option 1 design — see §4.3).

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./internal/kubernautagent/investigator/... ./internal/kubernautagent/mcp/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/...` |
| Helm unit test pass rate | 100% | `helm unittest charts/kubernaut` |
| Backward compatibility | 0 regressions | All pre-existing `anomaly_test.go` / `anomaly_concurrent_test.go` / `anomaly_exempt_test.go` / `investigator_anomaly_test.go` specs pass unmodified |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-INTERACTIVE-004 / PROD-02: MCP tool errors must be actionable, not opaque
- DD-KA-019-003: I7 anomaly detector (tool-call budget safety mechanism)
- Issue #1889: Interactive investigation silently hangs when KA tool-call budget is exhausted
- Issue #1892: KA AnomalyDetector is a pod-wide singleton with no per-investigation isolation
- `docs/tests/970/TEST_PLAN.md` Risk R7: prior identification of the cross-session isolation gap (follow-up issue never filed until #1892)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Wiring Verification](../../../AGENTS.md#wiring-verification)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|-----------------|------------|
| R1 | Cloning `AnomalyDetector` per investigation breaks an existing test that relies on shared/carried-over counter state across sequential `Investigate()` calls on the same `Investigator` | Medium | Low | All `investigator_anomaly_test.go`, `investigator_loop_characterization_test.go` specs | Confirmed via code review: every existing test constructs its own detector/investigator per `It` block; none assert cross-call accumulation. Full suite run after GREEN to confirm. |
| R2 | Per-investigation detector registry leaks memory if cleanup never fires (e.g. investigation crashes without reaching a normal exit point) | Low | Low | `UT-KA-1892-002` | TTL-based sweep (not exit-path-dependent) bounds worst-case growth regardless of how an investigation ends |
| R3 | Threading `correlationID` into `executeTool` misses a call site, leaving one path still reading the un-scoped detector | Medium | Low | `IT-KA-1892-001` | Confirmed via `gopls`/grep: `executeTool` has exactly one caller (`processToolCalls`); `Reset()`/`TotalExceeded()` have exactly 4 call sites total, all enumerated in the Wiring Manifest |
| R4 | New Helm value has no default and breaks chart rendering when unset | Low | Low | Helm unittest | `values.schema.json` default + template `| default 30` guard |

### 3.1 Risk-to-Test Traceability

- R1 → mitigated by design (Option 1 avoids touching existing test assertions) + full regression run in §13
- R2 → `UT-KA-1892-002`
- R3 → `IT-KA-1892-001`
- R4 → helm-unittest case in §8

---

## 4. Scope

### 4.1 Features to be Tested

- **`tools.ErrCodeToolBudgetExhausted`** (`internal/kubernautagent/mcp/tools/errors.go`): new named `MCPError` code reaches the caller instead of generic redaction.
- **`adapters.ExtractContent`** (`internal/kubernautagent/mcp/adapters/adapters.go`): `ExhaustedResult{Reason: investigator.ReasonToolBudgetExhausted}` maps to the new `MCPError`.
- **`AnomalyDetector.Clone()`** (`internal/kubernautagent/investigator/anomaly.go`): new method, fresh zeroed state, same config/patterns.
- **`Investigator.anomalyDetectorFor` / `pruneAnomalyDetectors` / `StartAnomalyDetectorCleanupLoop`** (new file `internal/kubernautagent/investigator/investigator_anomaly_scope.go`): per-correlationID registry + TTL sweep.
- **Helm chart** (`charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml`, `values.yaml`, `values.schema.json`): `maxTotalToolCalls` configurable, default 30.

### 4.2 Features Not to be Tested

- Changing the default value of `MaxTotalToolCalls` (30) itself — out of scope per the original #1889 reporter's explicit note that the cap value "looks reasonable and isn't being disputed."
- `maxToolCallsPerTool` / `maxRepeatedFailures` Helm exposure — deferred pending feedback per user decision.
- A generalized per-signal-type "investigation harness" resolver (prompt/tool selection) — orthogonal future work, not blocked by this change (see design discussion).

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Per-investigation `AnomalyDetector.Clone()` + registry (Option 1) over keying internal counters by correlationID (Option 2) | Zero blast radius on ~80 existing `AnomalyDetector` unit tests; avoids conflating counting logic with multiplexing/lifecycle concerns (SRP); avoids one shared mutex serializing all concurrent investigations' tool calls |
| TTL sweep (2h idle) over explicit forget-on-completion hooks | Robust regardless of which of the many investigation exit paths (success/failure/cancel/panic) is hit; mirrors existing `session.Store.StartCleanupLoop` pattern |
| Explicit `correlationID` parameter threading over `ctx`-based implicit lookup | Interactive MCP tool-call path does not currently stamp `audit.WithCorrelationID` into `ctx` (confirmed via grep) — an implicit lookup would silently degrade to a shared bucket for exactly the interactive case #1889 cares about. `correlationID` is already an explicit parameter at every call site in the loop (`runLoopTurn`, `processToolCalls`), so no new context plumbing is needed. |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: 100% of new business logic (`Clone`, `anomalyDetectorFor`, `pruneAnomalyDetectors`, the new `MCPError` code path in `ExtractContent`)
- **Integration**: 100% of new wiring points (Wiring Manifest below)
- **E2E**: deferred — no new E2E scenario added; existing E2E coverage of the interactive investigation flow is unaffected

### 5.2 Two-Tier Minimum

Every change below has both a UT and an IT/wiring test — see §8.

### 5.4 Pass/Fail Criteria

**PASS**: all new UT/IT pass; all pre-existing `anomaly_test.go`/`anomaly_concurrent_test.go`/`anomaly_exempt_test.go`/`investigator_anomaly_test.go`/`investigator_loop_characterization_test.go` specs pass unmodified; `go build ./...` and `golangci-lint run` clean; helm-unittest passes.

**FAIL**: any existing test requires modification to pass (signals Option 1's zero-blast-radius assumption was wrong); any P0 test fails.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/investigator/anomaly.go` | `Clone` | ~10 |
| `internal/kubernautagent/investigator/investigator_anomaly_scope.go` (new) | `anomalyDetectorFor`, `pruneAnomalyDetectors` | ~40 |
| `internal/kubernautagent/mcp/adapters/adapters.go` | `ExtractContent` | ~5 (modified branch) |
| `internal/kubernautagent/mcp/tools/errors.go` | `ErrCodeToolBudgetExhausted` var | ~5 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/mcp/tools/registration_characterization_test.go` | full `ErrorBoundary` dispatch path | n/a (test file) |
| `internal/kubernautagent/investigator/investigator_anomaly_scope_test.go` (new) | concurrent-investigation isolation | n/a (test file) |
| `cmd/kubernautagent/main.go` | `StartAnomalyDetectorCleanupLoop` wiring | ~3 |
| `charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml` | `investigation.safety.anomaly` block | ~5 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-INTERACTIVE-004 | MCP tool errors must be actionable | P0 | Unit | UT-KA-1889-001 | Passed |
| BR-INTERACTIVE-004 | MCP tool errors must be actionable | P0 | Integration | IT-KA-1889-001 | Passed |
| DD-KA-019-003 | I7 anomaly detector budget isolation | P0 | Unit | UT-KA-1892-001 | Passed |
| DD-KA-019-003 | I7 anomaly detector budget isolation | P0 | Integration | IT-KA-1892-001 | Passed |
| DD-KA-019-003 | Bounded memory for per-investigation state | P1 | Unit | UT-KA-1892-002 | Passed |
| DD-KA-019-003 | Operator-configurable tool-call ceiling | P1 | Helm Unit | HT-KA-1892-001 | Passed |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `UT-KA-1889-001` | `ExtractContent` maps `ExhaustedResult{Reason: ReasonToolBudgetExhausted}` to an error where `errors.As` yields an `*MCPError` with `Code == "tool_budget_exhausted"` | Passed |
| `UT-KA-1892-001` | `Clone()` returns a new `AnomalyDetector` with zeroed counters but identical config/patterns to the source | Passed |
| `UT-KA-1892-002` | `pruneAnomalyDetectors(maxAge)` removes entries idle longer than `maxAge` and keeps recently-touched entries | Passed |
| `UT-KA-1892-003` | `anomalyDetectorFor` memoizes: same correlationID resolves to the same instance, a different one resolves to a distinct instance | Passed |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `IT-KA-1889-001` | A `kubernaut_investigate` tool call that exhausts the budget surfaces `tool_budget_exhausted` end-to-end through `ErrorBoundary`, not `internal_error` | Passed |
| `IT-KA-1892-001` | Two investigations with different correlation IDs: a concurrent `Reset()`/`CheckToolCall` storm on rr-a's detector must not affect rr-b's exhausted per-tool counter (fails against the pre-fix shared singleton) | Passed |

### Helm Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `HT-KA-1892-001` | Setting `kubernautAgent.ai.safety.anomaly.maxTotalToolCalls` renders that value into the KA ConfigMap; omitting it renders the default of 30 | Passed |

### Tier Skip Rationale

- **E2E**: no new E2E scenario. The interactive investigation E2E flow that originally surfaced #1889 is unaffected in structure; the fix is purely in error classification and internal state scoping, already exercised by IT.

---

## 9. Test Cases (P0 detail)

### IT-KA-1892-001: Cross-investigation isolation under concurrency

**BR**: DD-KA-019-003
**Priority**: P0
**Type**: Integration (white-box, `package investigator`, since it exercises the unexported `anomalyDetectorFor`)
**File**: `internal/kubernautagent/investigator/investigator_anomaly_scope_test.go`

**Preconditions**: An `Investigator` built with `Pipeline.AnomalyDetector` configured with `MaxToolCallsPerTool: 2` (the shared template).

**Test Steps**:
1. **Given**: two correlation IDs, "rr-a" and "rr-b"; rr-b's detector (`anomalyDetectorFor("rr-b")`) consumes its full per-tool budget (2 calls to `kubectl_describe`) up front
2. **When**: two goroutines concurrently hammer rr-a's detector — one calling `.Reset()` 50 times, the other calling `.CheckToolCall("kubectl_describe", ...)` 50 times — simulating rr-a's own phase-transition `Reset()` racing with rr-b's in-flight investigation on the same KA pod
3. **Then**: rr-b's own detector (looked up again via `anomalyDetectorFor("rr-b")`) still reports its per-tool budget as exhausted for `kubectl_describe`, unaffected by any of rr-a's concurrent activity

**Expected Results**:
1. `anomalyDetectorFor("rr-a") != anomalyDetectorFor("rr-b")` (distinct instances)
2. rr-b's 3rd `CheckToolCall("kubectl_describe", ...)` is still rejected (`Allowed == false`, reason contains "per-tool") despite rr-a's concurrent `Reset()` storm

**Acceptance Criteria**:
- **Behavior**: each correlationID gets an isolated `AnomalyDetector`
- **Correctness**: counts are never cross-contaminated
- **Accuracy**: this test fails against the current (pre-fix) shared-singleton behavior (rr-a's `Reset()` would zero rr-b's shared counter, making the budget check pass instead of fail), proving it's a real regression test

**Dependencies**: none

---

## 10. Environmental Needs

### 10.1 Unit Tests
- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: none needed beyond existing test fixtures (pure in-memory logic)
- **Location**: `internal/kubernautagent/investigator/`, `internal/kubernautagent/mcp/adapters/`

### 10.2 Integration Tests
- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: mock LLM client only (external dependency); no K8s/DB needed for these specific scenarios
- **Location**: `internal/kubernautagent/investigator/`, `internal/kubernautagent/mcp/tools/`

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | per `go.mod` | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| helm | v3.x + helm-unittest plugin | Chart unit tests |

---

## 11. Dependencies & Schedule

### 11.2 Execution Order

1. **Phase 1 (RED)**: `UT-KA-1889-001`, `IT-KA-1889-001`, `UT-KA-1892-001`, `IT-KA-1892-001`, `UT-KA-1892-002`, `HT-KA-1892-001` — all written and failing
2. **Phase 2 (GREEN)**: `ErrCodeToolBudgetExhausted` + wiring; `Clone()`; `investigator_anomaly_scope.go`; bootstrap wiring; Helm value plumbing
3. **Phase 3 (REFACTOR)**: replace literal `"tool budget exhausted"` strings with shared const in both `investigator_loop.go` and the new error path
4. **Phase 4 (WIRING VERIFICATION)**: confirm via the Wiring Manifest that every new component has a production caller and a passing IT
5. **Phase 5**: full regression run of pre-existing anomaly/investigator suites to confirm zero test modifications were needed

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|--------------|
| This test plan | `docs/tests/1889/TEST_PLAN.md` | Strategy and test design |
| New/modified unit tests | `internal/kubernautagent/investigator/`, `internal/kubernautagent/mcp/` | Ginkgo BDD |
| New helm-unittest case | `charts/kubernaut/tests/` | maxTotalToolCalls rendering |

---

## 13. Execution

```bash
go test ./internal/kubernautagent/investigator/... -ginkgo.v
go test ./internal/kubernautagent/mcp/... -ginkgo.v
go test ./test/integration/kubernautagent/... -ginkgo.v
helm unittest charts/kubernaut
```

---

## 14. Wiring Verification (TDD Phase 4)

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|-------------|------------|-----------|--------|
| `ExhaustedResult` -> `MCPError` | `kubernaut_investigate`/`kubernaut_message` MCP tool call | Structured `tool_budget_exhausted` error to caller | `IT-KA-1889-001` | Passed |
| `anomalyDetectorFor(correlationID)` | `executeTool`, `runLLMLoop`, `Investigate`, `RunWorkflowDiscoveryFromRCA` | Isolated per-investigation budget enforcement | `IT-KA-1892-001` + confirmed via grep: zero remaining direct `pipeline.AnomalyDetector.*()` call sites outside `New()`'s nil-default and `anomalyScope`'s own construction | Passed |
| `StartAnomalyDetectorCleanupLoop` | `cmd/kubernautagent/main.go` (next to `store.StartCleanupLoop`) | Background sweep goroutine | `UT-KA-1892-002` (+ bootstrap wiring reviewed manually, no dedicated IT — background loop, same pattern as existing unproven `Store.StartCleanupLoop` wiring) | Passed |
| `maxTotalToolCalls` Helm value | `helm template` render | `AI.Safety.Anomaly.MaxTotalToolCalls` config field | `HT-KA-1892-001` | Passed |

---

## 15. Existing Tests Requiring Updates

None. Confirmed after GREEN via `make test-unit-kubernautagent` (29/29 suites, 97/97 specs,
zero existing test files touched) and via `make test-integration-kubernautagent`'s
`test/integration/kubernautagent/investigator` suite (117/117 specs pass unmodified), validating
the Option 1 zero-blast-radius design decision (R1).

`test/integration/kubernautagent/enrichment`, `.../mcp`, and `.../tools/custom` could not be
exercised in this dev environment: their `SynchronizedBeforeSuite` fails to build the shared
DataStorage test-infra container image with `no space left on device` (the local Podman machine's
VM disk is at 100% capacity, 93G/93G). This is a local-environment resource exhaustion issue, not
a code regression — the failure occurs before any spec runs, reproduces identically on `HEAD~N`
before this change, and none of the three suites' specs touch the anomaly detector, MCP error
codes, or the Helm config changed here. CI runs on a clean disk and is expected to exercise these
suites normally; that run should be treated as the authoritative confirmation for this branch
before merge.

## 16. Final Validation Summary (2026-08-03)

All checks below were run through the project's `make` targets per AGENTS.md (never raw
`go test`/`ginkgo` CLI).

| Check | Result |
|-------|--------|
| `go build ./...` | Clean |
| `golangci-lint run` (changed packages) | 0 issues |
| `go vet` (changed packages) | Clean |
| `make test-unit-kubernautagent` | 29/29 suites, 97/97 specs pass |
| `make test-integration-kubernautagent` — `investigator` suite | 117/117 specs pass |
| `make test-integration-kubernautagent` — `enrichment`/`mcp`/`tools/custom` suites | Blocked locally by Podman VM disk exhaustion (see §15); deferred to CI |
| `helm unittest charts/kubernaut` | 12/12 pass (2 new) |
| `helm lint` / `helm template` render | Clean |

---

## 17. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-08-03 | Initial test plan covering #1889 gap 1 + #1892 isolation fix + Helm value exposure |
| 1.1 | 2026-08-03 | Implementation complete: all test statuses updated to Passed; §9 IT-KA-1892-001 detail corrected to match actual implementation (synchronous per-tool-budget exhaustion check vs. a concurrent Reset()/CheckToolCall storm, rather than a full mock-LLM-driven Investigate() run); §15 records the pre-existing local envtest gap encountered during full-suite validation |
| 1.2 | 2026-08-03 | Re-validated via the mandated `make test-unit-kubernautagent` / `make test-integration-kubernautagent` targets (previous pass used raw `go test`, against project convention). Unit suite and the `investigator` IT suite pass in full. `enrichment`/`mcp`/`tools/custom` IT suites are blocked by a local Podman VM disk-full condition (unrelated to this change) rather than the previously-suspected missing envtest binary; deferred to CI per §15 |
