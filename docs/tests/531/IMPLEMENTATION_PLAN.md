# Implementation Plan: Mock LLM Go Sub-Issues (#531)

**Version**: 1.0
**Created**: 2026-03-04
**Status**: Ready for Execution
**Branch**: `development/v1.3`
**Methodology**: TDD (RED → GREEN → REFACTOR) per APDC framework

---

## Overview

This plan covers the 10 sub-issues of #531 (Mock LLM Go rewrite) in dependency order. The core Mock LLM implementation (23 Go files, ~2179 lines) is already landed with 25 test files (2389 lines). These sub-issues complete the architecture, eliminate dead code, and add extensibility features.

### Current State Summary

| Issue | Status | Existing Code | Existing Tests |
|-------|--------|---------------|----------------|
| #562 | **DONE** | `pkg/shared/types/openai/` (4 files) | UT-MOCK-060/061 (shared_types_test.go) |
| #548 | **DONE** (dependency) | `pkg/shared/uuid/` | UT-MOCK-030/031 (uuid_test.go) |
| #561 | **PARTIAL** | Deterministic UUIDs in scenarios; `LoadYAMLOverrides` not wired | UT-MOCK-033 (yaml_override_test.go) |
| #560 | **PARTIAL** | DAG types + Execute(); not wired into handlers | dag_engine_test.go, conversation_flow_test.go |
| #564 | **PARTIAL** | Registry + Detect + Match; monolithic default.go | registry_test.go, detection_test.go, catalog_test.go |
| #563 | **PARTIAL** | Verification endpoints exist; DAG path empty; testutil empty | verification_test.go |
| #570 | **PARTIAL** | HeaderRecorder; no per-request history; no Go helpers | auth_headers_test.go (unit + integration) |
| #565 | **PARTIAL** | Fault injector; delay unused; Ollama ignores faults | fault_test.go |
| #566 | **MINIMAL** | `LoadYAMLOverrides` for overrides only | yaml_override_test.go |
| #567 | **NOT STARTED** | Nothing | None |
| #568 | **NOT STARTED** | Nothing | None |

---

## Execution Phases

### Phase 1: Fix Foundations (#561, #560)

**Goal**: Establish single routing authority (DAG) and deprecate ConfigMap sync code.
**Estimated effort**: 4-5 days
**Test Plans**: [#561 Test Plan](../561/TEST_PLAN.md), [#560 Test Plan](../560/TEST_PLAN.md)

#### 1A: #561 — Wire YAML Overrides, Deprecate ConfigMap Sync (1.5-2 days)

**BRs**: BR-MOCK-030, BR-MOCK-031, BR-MOCK-032, BR-MOCK-033

**IMPORTANT — ConfigMap sync cannot be deleted yet**: `UpdateMockLLMConfigMap` is called in
`test/e2e/fullpipeline/suite_test.go`. `WriteMockLLMConfigFile` is called in
`test/integration/holmesgptapi/suite_test.go` and `test/integration/aianalysis/suite_test.go`.
`SortedWorkflowUUIDKeys` has 4 callers. These suites belong to other services (HAPI, AA, E2E)
and cannot be broken. Phase 1A **deprecates** these functions with doc comments + log warnings;
actual removal is deferred until those suites are migrated.

**TDD Sequence**:

1. **RED**: Write integration tests (IT-MOCK-561-001 through 004)
   - `test/integration/mockllm/override_wiring_test.go`
   - Test: server starts with override file → serves overridden UUID
   - Test: server starts without override → serves deterministic UUID
   - Test: deterministic UUID matches `uuid.DeterministicUUID()` via HTTP

2. **GREEN**: Wire `LoadYAMLOverrides` into `main.go`
   - `cmd/mock-llm/main.go`: Load overrides from `cfg.ConfigPath`, apply to registry
   - `scenarios/default.go`: `DefaultRegistry()` accepts optional `*config.Overrides` parameter
   - Override mechanism: `configScenario` stores `*MockScenarioConfig` (pointer); override replaces `WorkflowID`/`Confidence` in-place during registry construction
   - NOTE: `ApplyOverrides` is NOT a separate method — overrides are applied during `DefaultRegistry(overrides)` to keep construction atomic

3. **REFACTOR**: Cleanup + deprecation
   - Add `// Deprecated:` doc comments + `log.Printf("DEPRECATED: ...")` to `UpdateMockLLMConfigMap`, `WriteMockLLMConfigFile`, `SortedWorkflowUUIDKeys`
   - Add `POST /chat/completions` route alias in `router.go` (BR-MOCK-001, gap G5)
   - Remove empty `handlers/headers.go` (I5 inconsistency)
   - Update BUSINESS_REQUIREMENTS.md status for implemented BRs (G4)
   - Verify `go build ./...` (including E2E and integration suites — they must still compile)

**Commits**:
- `feat(#561): wire YAML overrides into Mock LLM startup`
- `chore(#561): deprecate ConfigMap sync helpers, add /chat/completions route alias`

#### 1B: #560 — Wire DAG Conversation Engine (2-3 days)

**BRs**: BR-MOCK-010 through BR-MOCK-015

**ARCHITECTURE DECISION — DAG as routing oracle**:
The DAG determines *which response to build* (terminal node identity + response type),
NOT the response itself. `HandlerResult` is extended with `ResponseType` (StepToolCall/StepFinalAnalysis)
and `ToolName` so the handler can build the correct response using existing `response.Build*` functions.
This avoids duplicating response-building logic inside DAG node handlers and ensures behavioral equivalence
with the existing `SelectMode` + tool-count path.

**IMPORTANT — Three-Phase RCA (BR-MOCK-013) is NEW behavior**:
Neither the Python Mock LLM nor the current Go handler implements Phase 3 routing.
`HasPhase3Markers()` exists on `Context` but is never called from handlers.
Phase 3 support is added as an additive feature AFTER behavioral equivalence is proven
for legacy + three-step modes.

**TDD Sequence**:

1. **RED**: Write unit tests for transition conditions
   - `test/unit/mockllm/dag_conditions_test.go` (new file)
   - UT-MOCK-010-005: ToolResultCount condition
   - UT-MOCK-010-006: HasPhase3Markers condition
   - UT-MOCK-010-007: IsForceText condition
   - UT-MOCK-010-008: HasTools condition

2. **GREEN**: Implement `TransitionCondition` types
   - `conversation/conditions.go`: Add `ToolResultCountCondition`, `HasPhase3MarkersCondition`, `IsForceTextCondition`, `HasToolsCondition`
   - Extend `HandlerResult` with `ResponseType StepType` and `ToolName string`

3. **RED**: Write unit tests for DAG builders (legacy + three-step ONLY)
   - `test/unit/mockllm/dag_modes_test.go` (new file)
   - UT-MOCK-011-001: Legacy DAG construction + execution
   - UT-MOCK-012-001: Three-step DAG
   - UT-MOCK-012-002: Four-step DAG (with resource context)

4. **GREEN**: Implement DAG builders
   - `conversation/modes.go`: Add `BuildLegacyDAG()`, `BuildThreeStepDAG(hasRC bool)`
   - `conversation/modes.go`: Add `SelectDAG(tools []openai.Tool) *DAG` (replaces `SelectMode`)
   - Node handlers return `HandlerResult` with response metadata (type + tool name)
   - `ConversationMode` type retained but deprecated — `SelectDAG` is the new entry point

5. **RED**: Write integration tests for DAG-driven routing
   - `test/integration/mockllm/dag_routing_test.go` (new file)
   - IT-MOCK-011-001: Legacy mode HTTP behavioral equivalence
   - IT-MOCK-012-001: Three-step mode HTTP behavioral equivalence
   - IT-MOCK-015-001: DAG path queryable via verification API

6. **GREEN**: Wire DAG into handler
   - `handlers/openai.go`: Replace `SelectMode` + manual tool counting with `SelectDAG` + `DAG.Execute()`
   - Handler reads `result.ResponseType` and `result.ToolName` to call `response.BuildToolCallResponse` or `response.BuildTextResponse` — same functions as before
   - Wire `tracker.RecordDAGPath(result.Path)` after each execution
   - If `scenario.DAG()` returns non-nil, use that instead of `SelectDAG`

7. **REFACTOR**: Clean up + add Phase 3
   - Remove `ConversationMode` step-counting code path
   - Ensure ALL existing integration tests pass unchanged (behavioral equivalence gate)
   - `go vet ./...` on affected packages
   - **Then** add Phase 3 as additive feature:
     - RED: UT-MOCK-013-001 (Phase 3 DAG with markers condition)
     - GREEN: `BuildThreePhaseDAG()` + wire into `SelectDAG` when Phase 3 markers detected
     - RED: IT-MOCK-013-001 (Phase 3 via HTTP)
     - GREEN: Handler supports Phase 3 flow

**Commits**:
- `feat(#560): add transition condition types for DAG engine`
- `feat(#560): add DAG builders for legacy and three-step modes`
- `feat(#560): wire DAG execution into OpenAI handler, enable path recording`
- `feat(#560): add three-phase RCA DAG (BR-MOCK-013, new behavior)`

---

### Phase 2: Scenario Registry (#564)

**Goal**: Split monolithic `default.go` into one-file-per-scenario with self-registration.
**Estimated effort**: 2 days
**Test Plan**: `docs/tests/564/TEST_PLAN.md` (to be created before execution)

**BRs**: BR-MOCK-020 through BR-MOCK-026

**TDD Sequence**:

1. **RED**: Write tests for registry ordering, priority resolution, and self-registration
2. **GREEN**: Refactor `default.go` into individual scenario files:
   - `scenarios/oomkilled.go`, `scenarios/crashloop.go`, `scenarios/node_not_ready.go`, etc.
   - Each file has an `init()` function that registers with `DefaultRegistry`
   - `DefaultRegistry()` becomes the global instance populated by `init()`
3. **REFACTOR**: Remove `DefaultRegistry()` function body (now just returns the global); verify all existing tests pass

**Acceptance gate**: All 15 scenarios self-register. `DefaultRegistry.List()` returns all metadata. Zero changes to HTTP behavior.

---

### Phase 3: Verification & Headers (#563, #570)

**Goal**: Complete the verification API and auth header recording to enable behavioral test assertions.
**Estimated effort**: 2-3 days
**Test Plans**: `docs/tests/563/TEST_PLAN.md`, `docs/tests/570/TEST_PLAN.md` (to be created before execution)

**BRs**: BR-MOCK-040 through BR-MOCK-044, BR-MOCK-006, BR-MOCK-007

**TDD Sequence for #563**:

1. **RED**: Tests for Go test helper functions (`AssertToolCalled`, `AssertToolSequence`, `AssertScenarioMatched`, `AssertDAGPath`, `Reset`)
2. **GREEN**: Implement `test/testutil/mockllm/client.go` with HTTP-based assertion helpers
3. **REFACTOR**: Enhance tracker to store per-request history (sequence numbers, conversation IDs)

**TDD Sequence for #570**:

1. **RED**: Tests for per-request header history with sequence/conversation_id, `?name=` filter, default header set
2. **GREEN**: Refactor `HeaderRecorder` to store `[]HeaderRecord` instead of flat map; add filter support; add default auth header patterns
3. **REFACTOR**: Wire `AssertHeaderReceived` / `AssertNoHeaderReceived` into test helpers; update issue #570 title (KAPI → KA)

---

### Phase 4: Fault Injection (#565)

**Goal**: Complete fault injection with typed faults, delay support, and Ollama coverage.
**Estimated effort**: 1-2 days
**Test Plan**: `docs/tests/565/TEST_PLAN.md` (to be created before execution)

**BRs**: BR-MOCK-050 through BR-MOCK-054

**TDD Sequence**:

1. **RED**: Tests for typed faults (timeout, rate_limit, intermittent, server_error), Ollama fault application
2. **GREEN**: Refactor `fault.Injector` to support typed faults with count and delay; apply faults in Ollama handler; wire `time.Sleep` for delay_ms
3. **REFACTOR**: Ensure fault metrics integration points exist for #568

---

### Phase 5: Extensibility (#566, #567) — DEFERRED (out of scope for v1.3)

**Status**: Deferred. Milestone removed from both issues. See GitHub comments for full justification.

**#566 (YAML Scenarios)**: Deferred until a non-Go consumer needs scenario authoring. Current Go-file-per-scenario pattern (Phase 2) is sufficient.

**#567 (Pillar Composition)**: Deferred until a second AIOps pillar (#554 Threat Remediation or #555 Cost Optimization) enters active development. Building a multi-pillar framework with a single consumer risks premature abstraction.

---

### Phase 6: Observability (#568) — COMPLETED

**Goal**: Prometheus metrics endpoint.
**Test Plan**: `docs/tests/568/TEST_PLAN.md`

**BRs**: BR-MOCK-080 through BR-MOCK-083

**Implementation Summary**:

- Created `test/services/mock-llm/metrics/metrics.go` with 4 Prometheus collectors:
  - `mock_llm_requests_total` (counter: endpoint, status_code, scenario)
  - `mock_llm_response_duration_seconds` (histogram: endpoint, scenario)
  - `mock_llm_scenario_detection_total` (counter: scenario, method)
  - `mock_llm_dag_phase_transitions_total` (counter: from_node, to_node)
- Wired `/metrics` endpoint via `promhttp.HandlerFor` in router
- Metrics reset on `POST /api/test/reset` via `Metrics.Reset()`
- Tests: 4 unit (UT-MOCK-568-001..004) + 7 integration (IT-MOCK-568-001..007)
- Zero regressions: 98 unit + 59 integration tests pass

---

## Risk Mitigations (from Triage + Plan Triage)

| Risk ID | Mitigation | Phase |
|---------|------------|-------|
| R1 (DAG disconnected) | Wire DAG into handler, verify behavioral equivalence via integration tests | Phase 1B |
| R2 (ConfigMap sync still active) | **Deprecate** (not delete) — active callers in E2E/HAPI/AA suites prevent removal. Add `// Deprecated:` + log warnings | Phase 1A |
| R3 (Ollama second-class) | Apply tracker/fault/header middleware to Ollama | Phase 4 |
| R4 (delay_ms dead code) | Wire `time.Sleep` in fault handler | Phase 4 |
| R5 (testutil empty) | Implement assertion helpers | Phase 3 |
| R6 (#570 says KAPI) | Update issue title/body | Phase 3 |
| R7 (HandlerResult too thin) | Extend with `ResponseType` + `ToolName` so handler can build response from DAG result | Phase 1B |
| R8 (Phase 3 RCA is NEW, not existing) | Implement after behavioral equivalence proven for legacy + three-step | Phase 1B (additive) |
| R9 (ApplyOverrides design) | Overrides applied during `DefaultRegistry(overrides)` construction, not as post-hoc method | Phase 1A |

## Gap Closures

| Gap ID | Resolution | Phase |
|--------|-----------|-------|
| G1 (No TransitionCondition types) | Create concrete condition implementations | Phase 1B |
| G2 (Last-only tracking) | Refactor to slice-of-records | Phase 3 |
| G3 (LoadYAMLOverrides not wired) | Wire in main.go | Phase 1A |
| G4 (Stale docs) | Update BUSINESS_REQUIREMENTS.md status | Phase 1A |
| G5 (Missing POST /chat/completions) | Add route alias | Phase 1A |

## Inconsistency Fixes

| ID | Fix | Phase |
|----|-----|-------|
| I1 (Dual routing) | Refactor `SelectMode` into `SelectDAG` returning `*DAG`. Deprecate `ConversationMode` step-counting. DAG is sole executor | Phase 1B |
| I2 (String literal tool names) | Replace with constants in tests | Phase 3 |
| I3 (MOCK_LLM_CONFIG_PATH unused) | Wire in main.go via `DefaultRegistry(overrides)` | Phase 1A |
| I4 (#570 KAPI refs) | Update issue | Phase 3 |
| I5 (Empty headers.go) | Delete | Phase 1A |

---

## Validation Gates

After each phase:

```bash
# Build validation
go build ./test/services/mock-llm/...
go vet ./test/services/mock-llm/...

# Unit tests (all mock-llm)
go test ./test/unit/mockllm/... -v -count=1

# Integration tests (all mock-llm)
go test ./test/integration/mockllm/... -v -count=1

# Full build check (no breakage in other packages)
go build ./...
```

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial implementation plan covering all 10 sub-issues |
