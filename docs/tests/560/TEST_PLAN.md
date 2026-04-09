# Test Plan: Mock LLM DAG-Based Conversation Engine (#560)

**Feature**: Wire the existing DAG engine into the HTTP request path, replacing the hardcoded `SelectMode` + tool-count routing with DAG-driven conversation flow
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `development/v1.3`

**Authority**:
- [BR-MOCK-010]: DAG-Based Conversation State Machine
- [BR-MOCK-011]: Legacy Conversation Mode
- [BR-MOCK-012]: Three-Step Discovery Mode (DD-HAPI-017)
- [BR-MOCK-013]: Three-Phase RCA Mode (#529)
- [BR-MOCK-014]: Conversation Context Tracking
- [BR-MOCK-015]: DAG Path Recording

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../development/business-requirements/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [#560]: Parent issue
- [DD-HAPI-017]: Three-step workflow discovery
- [#529]: Three-phase RCA architecture

---

## 1. Scope

### In Scope

- **conversation/conditions.go**: New `TransitionCondition` implementations wrapping `Context` methods
- **conversation/dag.go**: DAG engine execution (already implemented, now being wired)
- **conversation/modes.go**: Refactor to build DAGs instead of step-based modes
- **conversation/context.go**: Context methods used by conditions (already implemented)
- **handlers/openai.go**: Replace `SelectMode` + manual routing with `DAG.Execute()`
- **scenarios/default.go**: Scenarios return real DAGs (not nil) via `DAG()` method
- **tracker/tracker.go**: `RecordDAGPath` wired into request path
- **handlers/verification.go**: `/api/test/dag-path` returns real data

### Out of Scope

- Ollama handler changes — deferred to #565 (fault injection consistency)
- YAML-defined DAGs — deferred to #566 (declarative scenarios)
- Scenario registry restructuring — deferred to #564
- Pillar composition DAG fragments — deferred to #567

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Create concrete `TransitionCondition` types | DAG engine requires them; current logic in `Context` methods must be wrapped as conditions |
| Build two default DAGs first (legacy, three-step), add three-phase as additive feature | Behavioral equivalence with existing code is proven for legacy + three-step before adding new behavior |
| DAG as routing oracle, not full response builder | `DAG.Execute` determines which response to build (terminal node + step type); handler builds response from node metadata. Less invasive, easier to prove equivalence |
| `HandlerResult` extended with response metadata | `HandlerResult` carries `ResponseType` (tool call vs text) and `ToolName` so handler can build the response without post-execution switching |
| Refactor `SelectMode` into `SelectDAG` | `SelectDAG(tools)` returns a `*DAG` instead of a `*ConversationMode`. `ConversationMode` step-counting path is deprecated. `SelectDAG` is the selector; `DAG.Execute` is the executor |
| Scenarios default to mode-selected DAG | `Scenario.DAG()` returns nil → handler falls back to `SelectDAG`-chosen default DAG |
| Three-phase RCA (BR-MOCK-013) is new behavior | Neither Python nor current Go handler implements Phase 3 routing. `HasPhase3Markers()` exists on Context but is unused. Implemented as additive feature after equivalence is proven |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (conditions, DAG construction, mode-to-DAG builders)
- **Integration**: >=80% of integration-testable code (handler routing via DAG, path recording via HTTP)

### 2-Tier Minimum

Every BR gap is covered by Unit + Integration tiers.

### Business Outcome Quality Bar

Tests validate that DAG-driven conversation routing produces **identical HTTP responses** to the previous `SelectMode` + tool-count routing for all three conversation modes, and that the verification API now returns meaningful DAG paths.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `conversation/conditions.go` | `HasPhase3MarkersCondition.Evaluate`, `ToolResultCountCondition.Evaluate`, `IsForceTextCondition.Evaluate`, `HasToolsCondition.Evaluate`, `HasThreeStepToolsCondition.Evaluate` | ~80 (new) |
| `conversation/dag.go` | `NewDAG`, `AddNode`, `AddTransition`, `Execute` | ~106 |
| `conversation/context.go` | `CountToolResults`, `HasPhase3Markers`, `ExtractResource`, `ExtractRootOwner` | ~133 |
| `conversation/modes.go` | `BuildLegacyDAG`, `BuildThreeStepDAG`, `BuildThreePhaseDAG`, `SelectDAG` | ~80 (refactored) |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `handlers/openai.go` | `handleOpenAI` (DAG-driven routing) | ~140 |
| `handlers/verification.go` | `handleGetDAGPath` (functional path data) | ~75 |
| `tracker/tracker.go` | `RecordDAGPath`, `GetDAGPath` | ~117 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-MOCK-010 | DAG engine executes conversation flow | P1 | Unit | UT-MOCK-010-001 | Pending |
| BR-MOCK-010 | DAG transitions evaluate conditions correctly | P1 | Unit | UT-MOCK-010-002 | Pending |
| BR-MOCK-010 | DAG returns error when no transition matches | P1 | Unit | UT-MOCK-010-003 | Pass |
| BR-MOCK-010 | Concurrent DAG executions do not leak state | P1 | Unit | UT-MOCK-010-004 | Pass |
| BR-MOCK-011 | Legacy mode DAG: tool call → final analysis | P0 | Unit | UT-MOCK-011-001 | Pending |
| BR-MOCK-011 | Legacy mode via HTTP produces same response | P0 | Integration | IT-MOCK-011-001 | Pending |
| BR-MOCK-012 | Three-step mode DAG: 3 tool calls → final analysis | P0 | Unit | UT-MOCK-012-001 | Pending |
| BR-MOCK-012 | Three-step mode via HTTP produces same response | P0 | Integration | IT-MOCK-012-001 | Pending |
| BR-MOCK-012 | Four-step mode (resource context) DAG | P0 | Unit | UT-MOCK-012-002 | Pending |
| BR-MOCK-013 | Three-phase RCA DAG: phase markers trigger transition (NEW — not in Python or current Go) | P1 | Unit | UT-MOCK-013-001 | Pending |
| BR-MOCK-013 | Three-phase RCA via HTTP returns workflow selection content (NEW) | P1 | Integration | IT-MOCK-013-001 | Pending |
| BR-MOCK-014 | Conversation context tracks tool results | P0 | Unit | UT-MOCK-014-001 | Pass |
| BR-MOCK-014 | Context detects Phase 3 markers | P0 | Unit | UT-MOCK-014-002 | Pass |
| BR-MOCK-015 | DAG path recorded during execution | P1 | Unit | UT-MOCK-015-001 | Pending |
| BR-MOCK-015 | DAG path queryable via /api/test/dag-path | P1 | Integration | IT-MOCK-015-001 | Pending |
| BR-MOCK-010 | TransitionCondition: ToolResultCount evaluates correctly | P1 | Unit | UT-MOCK-010-005 | Pending |
| BR-MOCK-010 | TransitionCondition: HasPhase3Markers evaluates correctly | P1 | Unit | UT-MOCK-010-006 | Pending |
| BR-MOCK-010 | TransitionCondition: IsForceText evaluates correctly | P1 | Unit | UT-MOCK-010-007 | Pending |
| BR-MOCK-010 | TransitionCondition: HasTools evaluates correctly | P1 | Unit | UT-MOCK-010-008 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- Pass: Implemented and passing (from existing test suite)

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-MOCK-{BR_NUMBER}-{SEQUENCE}`

### Tier 1: Unit Tests

**Testable code scope**: `conversation/conditions.go` (new), `conversation/dag.go`, `conversation/modes.go` (refactored) — target >=80% coverage

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-MOCK-010-001` | DAG engine traverses nodes following true conditions and records path | Pending |
| `UT-MOCK-010-002` | DAG transitions are evaluated in priority order | Pending |
| `UT-MOCK-010-005` | ToolResultCount condition returns true when tool results >= threshold | Pending |
| `UT-MOCK-010-006` | HasPhase3Markers condition returns true when all 3 markers present | Pending |
| `UT-MOCK-010-007` | IsForceText condition returns true when ForceText metadata is set | Pending |
| `UT-MOCK-010-008` | HasTools condition returns true when request includes tools | Pending |
| `UT-MOCK-011-001` | Legacy DAG: no tool results → discovery node → tool call response; with tool results → analysis node → text response | Pending |
| `UT-MOCK-012-001` | Three-step DAG: steps through list_available_actions → list_workflows → get_workflow → analysis | Pending |
| `UT-MOCK-012-002` | Four-step DAG: get_resource_context → list_available_actions → list_workflows → get_workflow → analysis | Pending |
| `UT-MOCK-013-001` | Three-phase DAG: phase 3 markers present → workflow selection node | Pending |
| `UT-MOCK-015-001` | DAG execution path contains all visited node names in order | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `handlers/openai.go`, `handlers/verification.go` — target >=80% coverage

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-MOCK-011-001` | Legacy mode: first request gets tool call, second (with tool result) gets text response — identical to pre-DAG behavior | Pending |
| `IT-MOCK-012-001` | Three-step mode: 3 sequential requests stepping through tool calls produce correct tool names in order | Pending |
| `IT-MOCK-013-001` | Three-phase RCA: request with Phase 3 markers produces workflow selection content | Pending |
| `IT-MOCK-015-001` | After a conversation, `GET /api/test/dag-path` returns the correct node traversal sequence | Pending |

### Tier Skip Rationale

- **E2E**: Deferred — DAG wiring is an internal refactor. Behavioral equivalence proven by existing E2E tests passing without modification.

---

## 6. Test Cases (Detail)

### UT-MOCK-010-005: ToolResultCount Condition

**BR**: BR-MOCK-010
**Type**: Unit
**File**: `test/unit/mockllm/dag_conditions_test.go`

**Given**: A conversation context with N tool result messages (role="tool")
**When**: `ToolResultCountCondition{Threshold: T}.Evaluate(ctx)` is called
**Then**: Returns true when N >= T, false when N < T

**Acceptance Criteria**:
- Threshold 0: always true
- Threshold 1: true with 1+ tool results
- Threshold 3: true with 3, false with 2

---

### UT-MOCK-010-006: HasPhase3Markers Condition

**BR**: BR-MOCK-010
**Type**: Unit
**File**: `test/unit/mockllm/dag_conditions_test.go`

**Given**: A conversation context with messages containing various Phase 3 markers
**When**: `HasPhase3MarkersCondition{}.Evaluate(ctx)` is called
**Then**: Returns true only when ALL three markers are present

**Acceptance Criteria**:
- All 3 markers → true
- 2 of 3 markers → false
- 0 markers → false

---

### UT-MOCK-011-001: Legacy Mode DAG Construction and Execution

**BR**: BR-MOCK-011
**Type**: Unit
**File**: `test/unit/mockllm/dag_modes_test.go`

**Given**: A `BuildLegacyDAG()` constructed DAG
**When**: Executed with a context containing 0 tool results
**Then**: Path is `[discovery, analysis]` with terminal node `analysis`
**When**: Executed with a context containing 1+ tool results
**Then**: Path is `[discovery, analysis]` with terminal node `analysis` (skips tool call)

**Acceptance Criteria**:
- Tool call node handler invoked when no tool results
- Analysis node handler invoked when tool results present
- Path recorded correctly

---

### UT-MOCK-012-001: Three-Step Mode DAG

**BR**: BR-MOCK-012
**Type**: Unit
**File**: `test/unit/mockllm/dag_modes_test.go`

**Given**: A `BuildThreeStepDAG(false)` constructed DAG
**When**: Executed with contexts containing 0, 1, 2, and 3 tool results respectively
**Then**: Each execution traverses one node further: `discovery` → `list_actions` → `list_workflows` → `get_workflow` → `analysis`

**Acceptance Criteria**:
- Step 0: tool call `list_available_actions`
- Step 1: tool call `list_workflows`
- Step 2: tool call `get_workflow`
- Step 3+: final text analysis

---

### IT-MOCK-011-001: Legacy Mode HTTP Behavioral Equivalence

**BR**: BR-MOCK-011
**Type**: Integration
**File**: `test/integration/mockllm/dag_routing_test.go`

**Given**: Mock LLM server started with default config
**When**: POST /v1/chat/completions with `tools=[search_workflow_catalog]` and oomkilled keyword, no tool results
**Then**: Response contains `tool_calls` with `search_workflow_catalog`
**When**: Follow-up request with tool result in messages
**Then**: Response contains `content` with RCA text (no tool calls)

**Acceptance Criteria**:
- Response JSON structure identical to pre-DAG implementation
- Scenario detection still works
- Workflow UUID is deterministic

---

### IT-MOCK-015-001: DAG Path Queryable via Verification API

**BR**: BR-MOCK-015
**Type**: Integration
**File**: `test/integration/mockllm/dag_routing_test.go`

**Given**: Mock LLM server started, state reset via `POST /api/test/reset`
**When**: A legacy-mode conversation completes (2 requests)
**Then**: `GET /api/test/dag-path` returns `["discovery", "analysis"]` (or equivalent node names)

**Acceptance Criteria**:
- Path is non-empty after conversation
- Path reflects actual nodes traversed
- Reset clears the path

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Mock `ResponseHandler` implementations for DAG node testing (internal test doubles, not external mocks)
- **Location**: `test/unit/mockllm/`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks — real HTTP server via `httptest.NewServer`
- **Infrastructure**: None beyond the in-process server
- **Location**: `test/integration/mockllm/`

---

## 8. Execution

```bash
# Unit tests
go test ./test/unit/mockllm/... -v -count=1

# Integration tests
go test ./test/integration/mockllm/... -v -count=1

# Specific test by ID
go test ./test/unit/mockllm/... -ginkgo.focus="UT-MOCK-010-005"
go test ./test/integration/mockllm/... -ginkgo.focus="IT-MOCK-015-001"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
