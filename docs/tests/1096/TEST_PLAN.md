# Test Plan: Full-Context Grounding Review (#1096)

**Version**: 1.0
**Created**: 2026-05-11
**Status**: Active
**Service**: Kubernaut Agent (KA) — Shadow Agent Alignment Subsystem
**Service Type**: [x] Stateless HTTP API
**Issue**: #1096
**Business Requirements**: BR-AI-601.3 (shadow agent alignment — grounding review extension), BR-HAPI-433-004/I7 (behavioral anomaly detection)
**Compliance**: FedRAMP AU-2/AU-3 (audit events for grounding review), SI-4 (continuous monitoring of investigation reasoning), SI-10 (input validation of LLM conversation context)
**ADR**: ADR-KA-001 Section 8 (future enhancements — full-context evaluation)

---

## 1. Scope

This test plan covers the full-context grounding review feature — a second evaluation layer that runs the entire RCA conversation through the shadow LLM in a single call, triggered when the RCA phase completes. It runs in parallel with workflow discovery to detect distributed prompt injection that per-step isolation cannot catch.

### 1.1 Problem Statement

The shadow agent evaluates each step in isolation (one LLM call per step, no conversation history). An attacker who can influence multiple tool outputs (K8s labels, annotations, ConfigMap values, log messages) can spread injection across steps that individually appear clean but collectively nudge the primary LLM's reasoning (distributed/boiling frog attack). The shadow agent has no mechanism to detect this because it never sees the evidence chain.

### 1.2 Solution

Add `EvaluateGrounding` to the Evaluator and `StartGroundingReview` to the Observer. At RCA sentinel detection, the investigator calls `NotifyRCAComplete` which triggers a parallel grounding review. The review completes during workflow discovery (free parallelism, zero added latency). `WaitForCompletion` and `RenderVerdict` incorporate the grounding result into the final verdict.

### 1.3 Components Under Test

| Component | File | Change Type |
|-----------|------|-------------|
| GroundingObservation type | `internal/kubernautagent/alignment/types.go` | Extend |
| Grounding metrics | `internal/kubernautagent/alignment/metrics.go` | Extend |
| Grounding audit events | `internal/kubernautagent/audit/emitter.go` | Extend |
| Grounding system prompt | `internal/kubernautagent/alignment/prompt/grounding.go` | New |
| EvaluateGrounding method | `internal/kubernautagent/alignment/grounding.go` | New |
| Observer grounding integration | `internal/kubernautagent/alignment/observer.go` | Extend |
| GroundingReview config | `internal/kubernautagent/config/config.go` | Extend |
| RCA completion trigger | `internal/kubernautagent/investigator/investigator.go` | Extend (1 line) |
| Main wiring | `cmd/kubernautagent/main.go` | Extend |

---

## 2. Test Scenario Naming Convention

**Format**: `UT-SA-1096-{GROUP}-{SEQUENCE}`

- `UT-SA-1096-OBS-*` — Observer grounding integration
- `UT-SA-1096-EVAL-*` — Evaluator EvaluateGrounding method
- `UT-SA-1096-CFG-*` — Config gating and validation
- `UT-SA-1096-METRIC-*` — Metrics observability wiring
- `UT-SA-1096-PAR-*` — Parallel execution and concurrency

---

## 3. Features Not to be Tested

- **E2E tests for grounding review**: The mock-LLM service does not yet support multi-message conversation injection scenarios. E2E tests are deferred to a follow-up when mock-LLM supports grounding review scenarios.
- **Formal distributed injection benchmarking**: (#602, v1.5) Curated multi-step attack datasets and scoring.
- **Grafana dashboard updates**: Deferred; dashboards will be updated in a separate operational PR.
- **Integration tests with real LLM**: Grounding review uses the same `llm.Client` interface as `EvaluateStep`; integration coverage is inherited from existing shadow LLM integration tests.

---

## 4. Test Scenarios

### 4.1 Phase 1: Observer Grounding Integration (8 tests)

| ID | Description | Type | Expected Result | BR | Checkpoint Categories |
|----|-------------|------|-----------------|----|-----------------------|
| UT-SA-1096-OBS-001 | `StartGroundingReview` stores grounding result when LLM returns grounded | Unit | `GroundingObservation.Grounded == true`, explanation populated | BR-AI-601.3 | Core happy path |
| UT-SA-1096-OBS-002 | `StartGroundingReview` stores ungrounded result when LLM flags reasoning drift | Unit | `GroundingObservation.Grounded == false`, explanation describes drift | BR-AI-601.3 | Core negative path |
| UT-SA-1096-OBS-003 | `WaitForCompletion` includes grounding result when review finishes before timeout | Unit | `WaitResult` contains non-nil `GroundingObservation` | BR-AI-601.3 | Cross-phase integration (7) |
| UT-SA-1096-OBS-004 | `WaitForCompletion` timeout fires before grounding completes — fail-closed (ungrounded) | Unit | `GroundingObservation.Grounded == false`, explanation mentions timeout | BR-AI-601.3 | Nil/zero edge cases (5), Resource bounds (3) |
| UT-SA-1096-OBS-005 | `RenderVerdict` with ungrounded grounding -> verdict is Suspicious with grounding summary | Unit | `Verdict.Result == VerdictSuspicious`, summary includes "grounding" | BR-AI-601.3 | Cross-phase integration (7) |
| UT-SA-1096-OBS-006 | `RenderVerdict` with grounded grounding + clean steps -> verdict is Clean | Unit | `Verdict.Result == VerdictClean` | BR-AI-601.3 | Cross-phase integration (7) |
| UT-SA-1096-OBS-007 | `StartGroundingReview` with nil messages slice -> fail-closed, no panic | Unit | `GroundingObservation.Grounded == false`, no panic | BR-HAPI-433-004/I7 | Nil/zero edge cases (5) |
| UT-SA-1096-OBS-008 | `StartGroundingReview` with empty messages slice -> fail-closed, no panic | Unit | `GroundingObservation.Grounded == false`, no panic | BR-HAPI-433-004/I7 | Nil/zero edge cases (5) |

### 4.2 Phase 2: Evaluator Grounding Method (8 tests)

| ID | Description | Type | Expected Result | BR | Checkpoint Categories |
|----|-------------|------|-----------------|----|-----------------------|
| UT-SA-1096-EVAL-001 | `EvaluateGrounding` with clean conversation returns `Grounded=true` | Unit | `Grounded == true`, explanation describes factual grounding | BR-AI-601.3 | Core happy path |
| UT-SA-1096-EVAL-002 | `EvaluateGrounding` with injected conversation returns `Grounded=false` | Unit | `Grounded == false`, explanation identifies reasoning drift | BR-AI-601.3 | Core negative path |
| UT-SA-1096-EVAL-003 | `EvaluateGrounding` timeout -> fail-closed `Grounded=false` | Unit | `Grounded == false`, explanation mentions timeout | BR-AI-601.3 | Error-path observability (6) |
| UT-SA-1096-EVAL-004 | `EvaluateGrounding` LLM error -> fail-closed `Grounded=false` with log context | Unit | `Grounded == false`, explanation includes error details (resource, phase) | BR-AI-601.3 | Error-path observability (6) |
| UT-SA-1096-EVAL-005 | `EvaluateGrounding` with conversation exceeding `MaxConversationTokens` -> truncation applied, no OOM | Unit | Truncated content passed to LLM, `Grounded` has a value, no OOM | BR-HAPI-433-004/I7 | Adversarial inputs (2) |
| UT-SA-1096-EVAL-006 | `EvaluateGrounding` with messages containing path traversal (`../../etc/passwd`) -> processed safely | Unit | No file access, normal evaluation | BR-HAPI-433-004/I7 | Adversarial inputs (2) |
| UT-SA-1096-EVAL-007 | `EvaluateGrounding` with Unicode edge cases (RTL override U+202E, zero-width joiners U+200D, BOM U+FEFF) -> handled without crash | Unit | Evaluation completes, no panic | BR-HAPI-433-004/I7 | Adversarial inputs (2) |
| UT-SA-1096-EVAL-008 | `EvaluateGrounding` emits audit events (grounding.request + grounding.response) with structured context | Unit | `mockAuditStore` captures 2 events with correct event types and correlation ID | BR-AI-601.3 | Observability wiring (1) |

### 4.3 Phase 3: Config Gating + Audit/Metrics (8 tests)

| ID | Description | Type | Expected Result | BR | Checkpoint Categories |
|----|-------------|------|-----------------|----|-----------------------|
| UT-SA-1096-CFG-001 | `GroundingReview.Enabled=false` -> `StartGroundingReview` is a no-op, `disabled` metric incremented | Unit | No LLM call, `grounding_total{result="disabled"}` incremented | BR-AI-601.3 | Config gating, Observability wiring (1) |
| UT-SA-1096-CFG-002 | `GroundingReview.Enabled=true, Timeout=0` -> validation error | Unit | `Validate()` returns error mentioning timeout | BR-AI-601.3 | Adversarial inputs (2) |
| UT-SA-1096-CFG-003 | `GroundingReview.MaxConversationTokens=0` -> validation error | Unit | `Validate()` returns error mentioning maxConversationTokens | BR-AI-601.3 | Adversarial inputs (2) |
| UT-SA-1096-CFG-004 | Default config values applied when GroundingReview section omitted from YAML | Unit | `Enabled=false`, `Timeout=30s`, `MaxConversationTokens=32000` | BR-AI-601.3 | Nil/zero edge cases (5) |
| UT-SA-1096-METRIC-001 | Grounded result increments `kubernaut_alignment_grounding_total{result="grounded"}` | Unit | Counter value increases by 1 | BR-AI-601.3 | Observability wiring (1) |
| UT-SA-1096-METRIC-002 | Ungrounded result increments `kubernaut_alignment_grounding_total{result="ungrounded"}` | Unit | Counter value increases by 1 | BR-AI-601.3 | Observability wiring (1) |
| UT-SA-1096-METRIC-003 | Error result increments `kubernaut_alignment_grounding_total{result="error"}` | Unit | Counter value increases by 1 | BR-AI-601.3 | Observability wiring (1) |
| UT-SA-1096-METRIC-004 | `kubernaut_alignment_grounding_duration_seconds` records positive value after grounding review | Unit | Histogram sample count > 0, sample sum > 0 | BR-AI-601.3 | Observability wiring (1) |

### 4.4 Phase 4: Parallel Execution + Concurrency (5 tests)

| ID | Description | Type | Expected Result | BR | Checkpoint Categories |
|----|-------------|------|-----------------|----|-----------------------|
| UT-SA-1096-PAR-001 | Grounding review runs concurrently with `SubmitAsync` steps — no data race under `-race` | Unit | No race detector violations, both complete | BR-AI-601.3 | Concurrency (4) |
| UT-SA-1096-PAR-002 | 10 goroutines calling `SubmitAsync` + 1 calling `StartGroundingReview` concurrently — no mutex deadlock | Unit | All goroutines complete within timeout | BR-AI-601.3 | Concurrency (4) |
| UT-SA-1096-PAR-003 | Circuit breaker fires during grounding review — grounding context cancelled, fail-closed ungrounded | Unit | `GroundingObservation.Grounded == false`, circuit breaker respected | BR-SAFETY-1076 | Concurrency — competing transitions (4) |
| UT-SA-1096-PAR-004 | `WaitForCompletion` timeout with both pending steps and pending grounding -> both reported in WaitResult | Unit | `WaitResult.Pending > 0`, grounding observation is fail-closed | BR-AI-601.3 | Resource bounds (3) |
| UT-SA-1096-PAR-005 | 50 create/start/wait lifecycle cycles -> Observer grounding fields do not leak across cycles | Unit | Each cycle produces independent results, no cross-contamination | BR-AI-601.3 | Resource bounds (3) |

---

## 5. Checkpoint Audit Mapping (9 Categories)

Each checkpoint verifies all 9 categories before advancing to the next TDD phase.

### Category 1: Observability Wiring
Every metric and audit event defined must have a test proving it is incremented/emitted via the production code path.

| Metric/Event | Test(s) |
|--------------|---------|
| `kubernaut_alignment_grounding_total{result="grounded"}` | UT-SA-1096-METRIC-001 |
| `kubernaut_alignment_grounding_total{result="ungrounded"}` | UT-SA-1096-METRIC-002 |
| `kubernaut_alignment_grounding_total{result="error"}` | UT-SA-1096-METRIC-003 |
| `kubernaut_alignment_grounding_total{result="disabled"}` | UT-SA-1096-CFG-001 |
| `kubernaut_alignment_grounding_duration_seconds` | UT-SA-1096-METRIC-004 |
| `aiagent.alignment.grounding.request` audit event | UT-SA-1096-EVAL-008 |
| `aiagent.alignment.grounding.response` audit event | UT-SA-1096-EVAL-008 |

### Category 2: Adversarial Inputs
For every string parameter accepted from outside the package.

| Parameter | Test(s) |
|-----------|---------|
| `[]llm.Message` conversation (empty) | UT-SA-1096-OBS-008 |
| `[]llm.Message` conversation (nil) | UT-SA-1096-OBS-007 |
| `[]llm.Message` conversation (max-length+1) | UT-SA-1096-EVAL-005 |
| `[]llm.Message` with path traversal content | UT-SA-1096-EVAL-006 |
| `[]llm.Message` with Unicode edge cases | UT-SA-1096-EVAL-007 |
| `GroundingReview.Timeout = 0` | UT-SA-1096-CFG-002 |
| `GroundingReview.MaxConversationTokens = 0` | UT-SA-1096-CFG-003 |

### Category 3: Resource Bounds
For every map, slice, or cache that grows with usage.

| Structure | Test(s) |
|-----------|---------|
| Observer `groundingObs` field lifecycle | UT-SA-1096-PAR-005 (50 cycles) |
| `WaitResult` with combined pending steps + grounding | UT-SA-1096-PAR-004 |

### Category 4: Concurrency
For every method protected by a mutex or accessed from multiple goroutines.

| Method/Field | Test(s) |
|--------------|---------|
| `StartGroundingReview` + `SubmitAsync` concurrent access | UT-SA-1096-PAR-001 |
| Observer `mu` under 10+ goroutines | UT-SA-1096-PAR-002 |
| Circuit breaker vs grounding review race | UT-SA-1096-PAR-003 |

### Category 5: Nil/Zero Edge Cases
For every struct field that can be nil or zero-valued.

| Field | Test(s) |
|-------|---------|
| `[]llm.Message` = nil | UT-SA-1096-OBS-007 |
| `[]llm.Message` = empty | UT-SA-1096-OBS-008 |
| `GroundingReview` config omitted (zero struct) | UT-SA-1096-CFG-004 |
| `WaitResult.GroundingObservation` = nil (timeout) | UT-SA-1096-OBS-004 |

### Category 6: Error-Path Observability
For every error return, verify log line includes SRE-diagnosable context.

| Error Path | Test(s) |
|------------|---------|
| LLM timeout in `EvaluateGrounding` | UT-SA-1096-EVAL-003 |
| LLM error in `EvaluateGrounding` | UT-SA-1096-EVAL-004 |

### Category 7: Cross-Phase Integration
Components defined in one phase wired to consumers in another.

| Producer | Consumer | Test(s) |
|----------|----------|---------|
| `EvaluateGrounding` (Phase 2) | `StartGroundingReview` (Phase 1) | UT-SA-1096-OBS-001, OBS-002 |
| `StartGroundingReview` (Phase 1) | `WaitForCompletion` (Phase 1) | UT-SA-1096-OBS-003 |
| `GroundingObservation` (Phase 1) | `RenderVerdict` (Phase 1) | UT-SA-1096-OBS-005, OBS-006 |
| Grounding metrics (Phase 3) | Observer code paths (Phase 1) | UT-SA-1096-METRIC-001..004 |
| Config gating (Phase 3) | `StartGroundingReview` (Phase 1) | UT-SA-1096-CFG-001 |

### Category 8: Spec Compliance
Protocol and naming compliance.

| Spec | Requirement | Verification |
|------|-------------|--------------|
| LLM JSON mode | `ChatOptions.JSONMode = true` in grounding request | UT-SA-1096-EVAL-001 (mock captures request) |
| DD-005 metric naming | `kubernaut_alignment_grounding_{total,duration_seconds}` | UT-SA-1096-METRIC-001..004 (metric names match) |
| Audit event naming | `aiagent.alignment.grounding.{request,response}` follows `aiagent.` prefix | UT-SA-1096-EVAL-008 |

### Category 9: API Surface Hygiene
No test helpers, internal constants, or debug functions exported from production packages.

| Check | Status |
|-------|--------|
| `GroundingObservation` — needed by `InvestigatorWrapper` and tests | Exported (justified) |
| `StartGroundingReview` — called from investigator via context | Exported (justified) |
| `EvaluateGrounding` — called from Observer | Exported (justified) |
| `TruncateHeadTail` — existing known exception (test-only export) | Flagged, pre-existing |
| No new test-only exports in `alignment` package | Verified at checkpoint |

---

## 6. TDD Phase Breakdown

### 6.1 TDD RED Phase (Write Failing Tests)

**File**: `test/unit/kubernautagent/alignment/grounding_test.go`

Write all 29 test scenarios using Ginkgo/Gomega BDD framework. Tests reference types and methods that do not yet exist (`GroundingObservation`, `StartGroundingReview`, `EvaluateGrounding`, grounding metrics). All tests must fail with compile errors or assertion failures.

**Shared test infrastructure**:
- Reuse existing `mockLLMClient`, `slowMockLLMClient`, `mockAuditStore` from `helpers_test.go`
- Add `groundedResponse()` and `ungroundedResponse()` helpers following `cleanResponse()`/`suspiciousResponse()` pattern

**Checkpoint after RED**: All 9 categories have mapped tests. No category has zero coverage. Escalate if gaps found.

### 6.2 TDD GREEN Phase (Implement to Pass)

Implementation order (dependency-driven):

1. **Types**: Add `GroundingObservation` struct to `types.go`. Add `GroundingObservation *GroundingObservation` field to `WaitResult`.
2. **Metrics**: Add `alignmentGroundingTotal` (CounterVec) and `alignmentGroundingDuration` (Histogram) to `metrics.go`.
3. **Audit**: Add `EventTypeGroundingRequest`, `EventTypeGroundingResponse`, `ActionGroundingRequest`, `ActionGroundingResponse` to `emitter.go`. Add to `AllEventTypes`.
4. **Prompt**: Create `prompt/grounding.go` with grounding review system prompt.
5. **Evaluator**: Create `grounding.go` with `EvaluateGrounding(ctx, []llm.Message) GroundingObservation` method.
6. **Observer**: Add `StartGroundingReview(messages []llm.Message)`, integrate into `WaitForCompletion` and `RenderVerdict`. Add `groundingEnabled bool` field and `WithGroundingEnabled` option.
7. **Config**: Add `GroundingReview` sub-struct to `AlignmentCheckConfig`, add validation rules, add defaults.
8. **Investigator**: Add `NotifyRCAComplete` one-line call at sentinel detection point in `runLLMLoop`.
9. **Main**: Wire `GroundingReview` config into Observer construction in `cmd/kubernautagent/main.go`.

**Checkpoint after GREEN**: All 29 tests pass. All 9 audit categories verified with passing tests.

### 6.3 TDD REFACTOR Phase

- Validate against 100 Go Mistakes checklist (focus areas: #1 unintended variable shadowing, #9 unused errors, #26 slice init, #28 nil/empty confusion, #56 goroutine leak, #65 using defer in loops, #77 JSON handling edge cases, #83 race conditions)
- DRY assessment: extract shared JSON parsing between `EvaluateStep` and `EvaluateGrounding` if duplication exceeds 10 lines
- Lint check: `golangci-lint run ./internal/kubernautagent/alignment/...`
- Full test suite with race detector: `go test ./internal/kubernautagent/alignment/... -race -count=1`

**Checkpoint after REFACTOR**: Final 9-category audit. API surface hygiene verified. Spec compliance confirmed. All tests green with `-race`.

---

## 7. Prometheus Metrics Changes

| Metric | Change | Breaking |
|--------|--------|----------|
| `kubernaut_alignment_grounding_total` | New counter with `{result}` label: grounded, ungrounded, error, timeout, disabled | No |
| `kubernaut_alignment_grounding_duration_seconds` | New histogram (DefBuckets) | No |

---

## 8. Audit Event Changes

| Event Type | Action | Trigger |
|------------|--------|---------|
| `aiagent.alignment.grounding.request` | `grounding_request` | Before `EvaluateGrounding` LLM call |
| `aiagent.alignment.grounding.response` | `grounding_response` | After `EvaluateGrounding` LLM call |

Required audit fields per event:
- `correlation_id`: investigation correlation ID
- `conversation_length`: number of messages in the conversation
- `conversation_tokens`: estimated token count
- `grounded` (response only): boolean result
- `duration_ms` (response only): evaluation duration in milliseconds

---

## 9. Concurrency Tests (all run with `-race`)

| ID | Description | Scope |
|----|-------------|-------|
| UT-SA-1096-PAR-001 | Grounding review concurrent with SubmitAsync steps | Observer |
| UT-SA-1096-PAR-002 | 10+ goroutines SubmitAsync + 1 StartGroundingReview | Observer mutex |
| UT-SA-1096-PAR-003 | Circuit breaker fires during grounding review | Competing state transitions |
| Existing observer_perf_test.go | 10+ goroutines calling SubmitAsync (regression) | Observer |

---

## 10. Coverage Estimate

| Tier | Target | Testable Code Subset | Expected Coverage |
|------|--------|---------------------|-------------------|
| Unit | >= 80% | `grounding.go`, `observer.go` grounding paths, `types.go` grounding type, `metrics.go` grounding metrics, `config.go` grounding validation | ~90% (29 tests covering all code paths including error, timeout, nil, concurrent) |
| Integration | N/A | Inherited from existing shadow LLM integration tests | N/A |
| E2E | Deferred | Requires mock-LLM grounding scenario support | 0% (deferred) |

---

## 11. Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-11 | Initial test plan for #1096 — full-context grounding review |
