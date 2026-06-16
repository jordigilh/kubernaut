# Test Plan: Structured Decision Payload + EventBridge Truncation Fix

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1395-1396-v1
**Feature**: End-to-end structured RCA delivery in present_decision payload with EventBridge truncation fix
**Version**: 1.0
**Created**: 2026-06-11
**Author**: AI Agent
**Status**: Draft
**Branch**: `feat/structured-decision-payload`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the end-to-end delivery of structured RCA data from the KA investigator through the AF agent to the Console, ensuring that decision payloads are machine-parseable, grounded in server-provided data (not LLM hallucination), and not corrupted by the EventBridge sanitization pipeline.

### 1.2 Objectives

1. **KA metrics delivery**: InvestigationMetrics accumulates LLM turns and tool calls across all investigation phases and populates InvestigationResult
2. **Structured complete event**: emitCompleteEvent attaches RCA subset payload to MCP event Data field
3. **AF RCA parsing**: bridgeEventsCollectSummary extracts structured RCA from EventTypeComplete into InvestigateMCPResult
4. **Truncation fix**: EmitStructuredMeta passes structured JSON through without 512-rune corruption while enforcing 8KB safety cap
5. **Type schema enforcement**: PresentDecisionArgs.RCA is required (not optional), triggering ADK self-correction on omission
6. **Console wire contract**: Decision event JSON structure matches Console expected schema

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./internal/kubernautagent/... ./pkg/apifrontend/...` |
| Integration test pass rate | 100% | `go test ./test/integration/...` |
| E2E test pass rate | 100% | `make test-e2e-apifrontend` (structured decision focus) |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on logic files |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on wiring files |
| E2E code coverage | >=80% | `E2E_COVERAGE=true` binary coverage collection |
| Backward compatibility | 0 regressions | All existing tests pass without modification |
| JSON integrity | 0 truncated payloads | No `...` suffix on structured JSON events |

---

## 2. References

### 2.1 Authority (governing documents)

- Issue #1395: EventBridge sanitizeBridgeText truncates structured JSON payloads at 512 runes
- Issue #1396: AF: Emit structured RCA and extended workflow options in present_decision payload
- BR-SESSION-001: Interactive investigation lifecycle
- DD-HAPI-847: Adversarial Due Diligence (causal_chain schema)

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Wiring Verification](../../.cursor/rules/10-wiring-verification.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- [100 Go Mistakes](https://github.com/teivah/100-go-mistakes) — refactoring validation

### 2.3 FedRAMP Control Objectives

| Control | NIST Intent | Application to This Feature |
|---------|-------------|----------------------------|
| **AU-3** | Audit records contain sufficient detail | Decision JSON includes all options, RCA, recommendation flags |
| **SI-4** | Real-time monitoring | metadata.type=decision classification; bridge failure metrics |
| **SI-10** | Input validation/sanitization | Control-char strip + secret redaction on structured payloads |
| **SI-17** | Fail-safe on error | Nil/malformed responses produce no corrupt events; oversized payloads rejected cleanly |
| **SC-7** | Boundary protection | Secrets redacted before crossing AF→client boundary |
| **AC-3** | Enforce authorized information flow | Structured output gated to appropriate emit paths |
| **AC-6** | Least privilege / human-in-the-loop | All options surfaced; recommendation explicit; no hidden automated choices |

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Structured JSON truncated at 512 runes | Console cannot parse decision cards | High (current bug) | UT-AF-1395-001..003 | EmitStructuredMeta with separate sanitization path |
| R2 | LLM omits required `rca` field | Empty RCA card in Console | Medium | UT-AF-1396-010..012 | Required value type triggers ADK schema self-correction |
| R3 | emitCompleteEvent drops RCA due to full channel buffer | AF receives no structured RCA | Low | IT-KA-1396-001 | Non-blocking send is pre-existing; RCA also available via investigate summary |
| R4 | Metrics under-count LLM turns (missed call sites) | Inaccurate tool_calls_count/llm_turns | Medium | UT-KA-1396-001..004 | Comprehensive grep of all ChatWithParams sites |
| R5 | security.RedactText corrupts JSON structure | Broken JSON in decision event | Low | UT-AF-1395-004 | Test with JWT-containing field values |
| R6 | 8KB cap exceeded by legitimate payload | Decision not rendered | Low | UT-AF-1395-003 | Reject-log with metric; realistic payloads are 1.5-6KB |

### 3.1 Risk-to-Test Traceability

- **R1** (Critical): UT-AF-1395-001, UT-AF-1395-002, IT-AF-1395-001
- **R2** (High): UT-AF-1396-010, UT-AF-1396-011, UT-AF-1396-012
- **R4** (Medium): UT-KA-1396-001, UT-KA-1396-002, UT-KA-1396-003, UT-KA-1396-004

---

## 4. Scope

### 4.1 Features to be Tested

- **InvestigationMetrics accumulator** (`internal/kubernautagent/investigator/`): Counts LLM turns and tool calls across investigation lifecycle
- **emitCompleteEvent RCA attachment** (`internal/kubernautagent/session/manager.go`): Marshals RCA subset into MCP complete event Data
- **AF RCA parsing** (`pkg/apifrontend/tools/ka_investigate_mcp.go`): Deserializes RCA from complete event into InvestigateMCPResult
- **EmitStructuredMeta** (`pkg/apifrontend/launcher/event_bridge.go`): 8KB-capped structured JSON emission without 512-rune truncation
- **RCAData + WorkflowOption extensions** (`pkg/apifrontend/tools/ka_tools.go`): Type schema for Console wire contract
- **Prompt pass-through** (`pkg/apifrontend/agent/prompt.txt`): LLM instructed to relay grounded RCA

### 4.2 Features Not to be Tested

- **Console rendering**: Separate repo (`kubernaut-demo-console`); covered by Console integration tests
- **Kagenti compatibility**: External project; does not inspect metadata.type=decision today

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| 8KB cap (not unlimited) for structured payloads | Defense-in-depth; no upstream per-event byte limit in a2a-go; realistic max is ~6KB |
| Reject-log on oversized (not truncate with `...`) | Preserves JSON integrity; broken JSON is worse than missing event |
| RCA as required value type (not pointer) | Forces ADK schema validation → LLM self-correction; investigation always produces minimum fields |
| rcaEventPayload subset (not full InvestigationResult) | Bounded size (~500B-2KB); avoids leaking internal workflow/validation state |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code (metrics accumulator, sanitization, type serialization, payload marshaling)
- **Integration**: >=80% of integration-testable code (MCP event bridge wiring, AF EventBridge → SSE delivery, complete event pipeline)
- **E2E**: >=80% of full service code exercised through Kind cluster (decision event journey through real AF+KA stack)

### 5.2 Two-Tier Minimum

Every business requirement covered by UT + IT minimum.

### 5.3 Business Outcome Quality Bar

Tests validate that the **Console receives machine-parseable structured decision data grounded in KA investigation findings** — not just that functions are called.

### 5.4 Pass/Fail Criteria

**PASS**:
1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier code coverage >=80%
4. No regressions in existing test suites
5. Decision payload JSON validates against Console schema for all tested scenarios
6. Existing `UT-AF-1189-*`, `UT-AF-WATCH-OUTPUT-*`, `IT-AF-WATCH-OUTPUT-*` tests continue to pass

**FAIL**:
1. Any P0 test fails
2. Per-tier coverage below 80%
3. Existing tests regress
4. Structured JSON payload contains `...` suffix (truncation corruption)

### 5.5 Suspension & Resumption Criteria

**Suspend**: Build broken; KA investigator changes introduce cascading test failures
**Resume**: Build green; root cause identified

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/investigator/investigation_metrics.go` (NEW) | `NewMetrics`, `IncLLMTurns`, `IncToolCalls`, `LLMTurns`, `ToolCalls` | ~30 |
| `internal/kubernautagent/session/manager.go` | `marshalRCASubset` (NEW) | ~25 |
| `pkg/apifrontend/launcher/event_bridge.go` | `EmitStructuredMeta`, `sanitizeStructuredText` (NEW) | ~30 |
| `pkg/apifrontend/tools/ka_tools.go` | `RCAData`, `PresentDecisionArgs`, `WorkflowOption` (type extensions) | ~25 |
| `pkg/apifrontend/launcher/part_converter.go` | `emitDecisionEvent`, `emitStructuredOutput` (wiring change) | ~15 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/investigator/investigator.go` | `Investigate`, `runLLMLoop`, `executeTool` (metrics wiring) | ~20 |
| `internal/kubernautagent/session/manager.go` | `emitCompleteEvent` (event emission) | ~15 |
| `pkg/apifrontend/tools/ka_investigate_mcp.go` | `bridgeEventsCollectSummary` (RCA parsing) | ~20 |
| `pkg/apifrontend/launcher/part_converter.go` | `emitPartViaBridge` (structured emit wiring) | ~10 |

---

## 7. BR Coverage Matrix

| BR/Issue | Description | Priority | Tier | Test ID | Status |
|----------|-------------|----------|------|---------|--------|
| #1395 | Structured JSON not truncated at 512 | P0 | Unit | UT-AF-1395-001 | Pending |
| #1395 | 8KB cap rejects oversized payload | P0 | Unit | UT-AF-1395-002 | Pending |
| #1395 | Reject-log emits metric + fallback status | P0 | Unit | UT-AF-1395-003 | Pending |
| #1395 | Sanitization (ctrl-char, redaction) still applied | P0 | Unit | UT-AF-1395-004 | Pending |
| #1395 | Free-text events still bounded at 512 | P0 | Unit | UT-AF-1395-005 | Pending |
| #1395 | Decision event > 512 chars reaches client intact | P0 | Integration | IT-AF-1395-001 | Pending |
| #1396 | RCAData serialization roundtrip | P0 | Unit | UT-AF-1396-001 | Pending |
| #1396 | WorkflowOption.Parameters serialization | P0 | Unit | UT-AF-1396-002 | Pending |
| #1396 | WorkflowOption.RuledOutReason serialization | P0 | Unit | UT-AF-1396-003 | Pending |
| #1396 | PresentDecisionArgs.RCA required (not optional) | P0 | Unit | UT-AF-1396-010 | Pending |
| #1396 | HandlePresentDecision with full RCA | P1 | Unit | UT-AF-1396-011 | Pending |
| #1396 | Decision event contains structured RCA in JSON | P0 | Integration | IT-AF-1396-001 | Pending |
| #1396 | KA InvestigationMetrics counts LLM turns | P0 | Unit | UT-KA-1396-001 | Pending |
| #1396 | KA InvestigationMetrics counts tool calls | P0 | Unit | UT-KA-1396-002 | Pending |
| #1396 | KA metrics not reset between phases | P0 | Unit | UT-KA-1396-003 | Pending |
| #1396 | KA emitCompleteEvent attaches RCA payload | P0 | Unit | UT-KA-1396-004 | Pending |
| #1396 | AF bridgeEventsCollectSummary parses RCA | P0 | Unit | UT-AF-1396-020 | Pending |
| #1396 | AF bridgeEventsCollectSummary fallback on empty Data | P1 | Unit | UT-AF-1396-021 | Pending |
| #1396 | Full pipeline: KA complete → AF RCA → decision event | P0 | Integration | IT-AF-1396-002 | Pending |
| #1396 | Structured decision event delivered end-to-end over SSE | P0 | E2E | E2E-AF-1396-001 | Pending |
| #1396 | Decision metadata.type classification in SSE stream | P1 | E2E | E2E-AF-1396-002 | Pending |
| #1395 | No truncation corruption through full AF stack | P0 | E2E | E2E-AF-1395-001 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}`
- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `AF` (ApiFrontend), `KA` (KubernautAgent)
- **ISSUE**: `1395` or `1396`
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: EventBridge structured emit, type serialization, metrics accumulator, RCA marshaling

| ID | Business Outcome Under Test | FedRAMP | Phase |
|----|----------------------------|---------|-------|
| `UT-AF-1395-001` | SI-10: Structured JSON payload (600 chars) passes through EmitStructuredMeta without truncation | SI-10 | Pending |
| `UT-AF-1395-002` | SI-10: Structured JSON payload (9000 chars) is rejected entirely (not truncated with `...`) | SI-10, SI-17 | Pending |
| `UT-AF-1395-003` | SI-17: Oversized payload rejection increments bridge write failure metric and emits fallback status | SI-17, SI-4 | Pending |
| `UT-AF-1395-004` | SC-7: JWT token in structured JSON field value is redacted by sanitizeStructuredText | SC-7 | Pending |
| `UT-AF-1395-005` | SI-10: Free-text EmitStatus still truncates at 512 runes (regression guard) | SI-10 | Pending |
| `UT-AF-1396-001` | AU-3: RCAData struct serializes severity, confidence, causal_chain, target, tool_calls_count, llm_turns | AU-3 | Pending |
| `UT-AF-1396-002` | AU-3: WorkflowOption.Parameters map[string]string serializes as JSON object | AU-3 | Pending |
| `UT-AF-1396-003` | AC-6: WorkflowOption.RuledOutReason explains why option is not viable | AC-6 | Pending |
| `UT-AF-1396-010` | AC-6: PresentDecisionArgs with RCA value type — zero-value RCA still serializes (schema enforcement) | AC-6 | Pending |
| `UT-AF-1396-011` | AU-3: HandlePresentDecision includes RCA summary in formatted message | AU-3 | Pending |
| `UT-AF-1396-020` | AU-3: bridgeEventsCollectSummary populates InvestigateMCPResult.RCA from complete event Data | AU-3 | Pending |
| `UT-AF-1396-021` | SI-17: bridgeEventsCollectSummary with empty Data on complete event — fallback to text summary | SI-17 | Pending |
| `UT-KA-1396-001` | AU-3: InvestigationMetrics.IncLLMTurns increments on each tokens.Add call | AU-3 | Pending |
| `UT-KA-1396-002` | AU-3: InvestigationMetrics.IncToolCalls increments on each executeTool dispatch | AU-3 | Pending |
| `UT-KA-1396-003` | AU-3: InvestigationMetrics not reset between RCA and workflow phases | AU-3 | Pending |
| `UT-KA-1396-004` | AU-3: marshalRCASubset produces valid JSON with severity, confidence, causal_chain, target, metrics | AU-3 | Pending |

### Tier 2: Integration Tests

**Testable code scope**: MCP event bridge pipeline, AF EventBridge → SSE delivery, complete event wiring

| ID | Business Outcome Under Test | FedRAMP | Phase |
|----|----------------------------|---------|-------|
| `IT-AF-1395-001` | SI-10: Decision event with 3 workflow options (>512 chars) reaches A2A event queue intact as valid JSON | SI-10, AU-3 | Pending |
| `IT-AF-1396-001` | AU-3: Full decision event contains rca.severity, rca.confidence, rca.causal_chain, options[].parameters | AU-3, AC-6 | Pending |
| `IT-AF-1396-002` | AU-3: KA complete event with RCA → AF parses → LLM present_decision → Console receives structured JSON | AU-3 | Pending |

### Tier 3: E2E Tests

**Testable code scope**: Full AF+KA stack in Kind — investigation lifecycle → structured decision event delivery over SSE

| ID | Business Outcome Under Test | FedRAMP | Phase |
|----|----------------------------|---------|-------|
| `E2E-AF-1396-001` | AU-3, AC-6: A2A message/stream delivers decision SSE frame with structured RCA (severity, confidence, causal_chain) and extended options — payload >512 chars, valid JSON, no truncation | AU-3, AC-6, SI-10 | Pending |
| `E2E-AF-1396-002` | SI-4: Decision SSE frame carries metadata.type=decision for monitoring/audit tooling separation | SI-4 | Pending |
| `E2E-AF-1395-001` | SI-10, SI-17: Decision payload with 3+ options (realistic ~1.5KB) reaches client intact through full AF stack without 512-rune corruption | SI-10, SI-17 | Pending |

**Infrastructure**: Existing `test/e2e/apifrontend/` cluster (KA+DS+mock-LLM+DEX), reusing `a2aMessageStream` + `scanSSEFrames` from `streaming_test.go`

---

## 9. Test Cases

### UT-AF-1395-001: Structured JSON passes through without truncation

**Priority**: P0
**Type**: Unit
**File**: `pkg/apifrontend/launcher/event_bridge_test.go`

**Test Steps**:
1. **Given**: A 600-char valid JSON string (realistic decision payload)
2. **When**: EmitStructuredMeta is called with metadata.type=decision
3. **Then**: The full JSON string is written to the event queue without modification (no `...` suffix)

**Expected Results**:
1. Event queue receives TaskStatusUpdateEvent with full 600-char text
2. Text does not end with `...`
3. JSON.parse succeeds on the text content

**Acceptance Criteria**:
- **Behavior**: Structured payloads bypass 512-rune truncation
- **Correctness**: Full JSON preserved byte-for-byte (after sanitization)
- **Accuracy**: No data loss

---

### UT-AF-1395-002: Oversized payload rejected entirely

**Priority**: P0
**Type**: Unit
**File**: `pkg/apifrontend/launcher/event_bridge_test.go`

**Test Steps**:
1. **Given**: A 9000-char JSON string (exceeds 8KB cap)
2. **When**: EmitStructuredMeta is called
3. **Then**: The structured payload is NOT written; a fallback status text IS emitted

**Expected Results**:
1. Event queue receives a status event with "too large to render" text
2. No event with the 9000-char payload exists in the queue
3. Bridge write failure metric incremented

---

### UT-AF-1395-004: Secret redaction in structured JSON

**Priority**: P0
**Type**: Unit
**File**: `pkg/apifrontend/launcher/event_bridge_test.go`

**Test Steps**:
1. **Given**: A JSON string containing `"description": "token=Bearer eyJhbGciOiJSUzI1NiJ9.payload.sig"`
2. **When**: EmitStructuredMeta is called
3. **Then**: The JWT is redacted in the output

**Expected Results**:
1. Output contains `[BEARER_REDACTED]` or `[JWT_REDACTED]`
2. Output does NOT contain `eyJhbGci`
3. Remaining JSON structure is valid

---

### UT-KA-1396-004: marshalRCASubset produces valid bounded JSON

**Priority**: P0
**Type**: Unit
**File**: `internal/kubernautagent/session/manager_test.go`

**Test Steps**:
1. **Given**: An InvestigationResult with severity="critical", confidence=0.92, causal_chain=["A","B","C"], target="ConfigMap/app in demo", TotalLLMTurns=17, TotalToolCalls=19
2. **When**: marshalRCASubset is called
3. **Then**: Valid JSON containing all fields; no workflow/validation internal state leaked

**Expected Results**:
1. JSON contains `"severity":"critical"`, `"confidence":0.92`, `"causal_chain":["A","B","C"]`
2. JSON contains `"total_llm_turns":17`, `"total_tool_calls":19`
3. JSON does NOT contain `workflow_id`, `validation_attempts_history`, `due_diligence`
4. JSON size < 2KB for typical payloads

---

### IT-AF-1395-001: Decision event with realistic payload reaches queue intact

**Priority**: P0
**Type**: Integration
**File**: `pkg/apifrontend/launcher/part_converter_test.go`

**Test Steps**:
1. **Given**: A genai.Part with FunctionCall name=kubernaut_present_decision, Args containing session_id, summary, rca (with all fields), and 3 workflow options with parameters
2. **When**: The streaming part converter processes this part with an active EventBridge
3. **Then**: The TaskStatusUpdateEvent in the queue contains valid JSON matching the full Console schema

**Expected Results**:
1. Event metadata.type == "decision"
2. JSON.parse succeeds
3. Payload contains `rca.severity`, `rca.confidence`, `rca.causal_chain`
4. Payload contains `options[0].parameters`, `options[1].ruled_out_reason`
5. Payload length > 512 chars (proves truncation bypass works)
6. No `...` suffix

---

### IT-AF-1396-002: Full pipeline KA→AF→Console

**Priority**: P0
**Type**: Integration
**File**: `pkg/apifrontend/tools/ka_investigate_mcp_test.go`

**Test Steps**:
1. **Given**: A simulated MCP event stream ending with EventTypeComplete carrying rcaEventPayload JSON in Data
2. **When**: bridgeEventsCollectSummary processes the stream
3. **Then**: InvestigateMCPResult.RCA is populated with all fields from the complete event

**Expected Results**:
1. RCA.Severity matches source
2. RCA.Confidence matches source
3. RCA.CausalChain matches source array
4. RCA.TotalLLMTurns and RCA.TotalToolCalls match source
5. Summary fallback: if streaming text was empty, RCA.RCASummary populates Summary

---

### E2E-AF-1396-001: Structured decision event delivered over A2A SSE

**Priority**: P0
**Type**: E2E
**File**: `test/e2e/apifrontend/structured_decision_e2e_test.go`

**Preconditions**:
- AF E2E cluster running (KA+DS+mock-LLM+DEX deployed)
- Mock-LLM `af_structured_decision` keyword scenario loaded in ConfigMap
- Valid DEX token for `analyst` persona

**Test Steps**:
1. **Given**: A RemediationRequest CRD created with signal triggering investigation
2. **When**: A2A `message/stream` is invoked with keyword triggering `af_structured_decision` scenario (investigate → present_decision with RCA)
3. **Then**: SSE stream contains a TaskStatusUpdateEvent with:
   - `metadata.type == "decision"`
   - Text is valid JSON
   - JSON contains `rca.severity`, `rca.confidence`, `rca.causal_chain` (non-empty array)
   - JSON contains `options` array with >=2 entries, one with `recommended: true`
   - Payload length > 512 chars (proves #1395 fix)
   - No trailing `...`

**Expected Results**:
1. Structured decision card delivered intact over SSE to A2A client
2. All RCA fields grounded (not hallucinated) — match mock-LLM scenario data
3. Extended options include `parameters` and `risk` fields

**Infrastructure**: Reuses `a2aMessageStream` + `scanSSEFrames` from existing `streaming_test.go`

---

### E2E-AF-1396-002: Decision metadata type classification

**Priority**: P1
**Type**: E2E
**File**: `test/e2e/apifrontend/structured_decision_e2e_test.go`

**Test Steps**:
1. **Given**: Same setup as E2E-AF-1396-001
2. **When**: SSE frames are parsed from the stream
3. **Then**: Decision frame is distinguishable from free-text status frames via `metadata.type`

**Expected Results**:
1. Exactly one frame has `metadata.type == "decision"` (not duplicated)
2. Other status frames (if any) have `metadata.type == "status"` or no type

---

### E2E-AF-1395-001: No truncation corruption through full stack

**Priority**: P0
**Type**: E2E
**File**: `test/e2e/apifrontend/structured_decision_e2e_test.go`

**Test Steps**:
1. **Given**: Mock-LLM scenario produces `present_decision` with 3 workflow options (each with parameters map and description), yielding ~1.5KB payload
2. **When**: SSE decision frame is received
3. **Then**: `json.Unmarshal` succeeds on the frame text AND payload matches expected option count

**Expected Results**:
1. Frame text is valid JSON (not truncated mid-key/value)
2. `options` array has exactly 3 elements
3. Each option has non-empty `workflow_id`, `name`, `description`
4. Total payload length > 512 chars and < 8192 chars

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: fakeQueue (event queue mock — already exists in test helpers)
- **Location**: Co-located `_test.go` files (existing pattern)

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks for AF wiring; simulated MCP event channel for KA→AF pipeline
- **Infrastructure**: In-process (no external services)
- **Location**: Co-located `_test.go` files

### 10.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Kind cluster (`apifrontend-e2e`) with KA+DS+PostgreSQL+Redis+mock-LLM+DEX
- **Mock LLM**: New `af_structured_decision` keyword scenario (extends existing `shared_e2e.go` keyword_scenarios)
- **Helpers**: Reuse `a2aMessageStream`, `scanSSEFrames`, `unwrapSSEDataLine` from `test/e2e/apifrontend/streaming_test.go`
- **Location**: `test/e2e/apifrontend/structured_decision_e2e_test.go`
- **Run**: `make test-e2e-apifrontend` or `ginkgo -v --focus="structured decision" ./test/e2e/apifrontend/`

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22 | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available |
|-----------|------|--------|------------------------|
| Existing `fakeQueue` test helper | Test infra | Available | Tests cannot emit bridge events |
| `EventTypeComplete` constant in `ka` package | Code | Available | AF cannot detect completion |
| ADK `functiontool` schema generation | External lib | Available (v1.4.0) | Cannot validate required RCA enforcement |

### 11.2 TDD Execution Order (Phased)

```
Phase A: KA Metrics + Complete Event
  ├─ TDD-RED:   UT-KA-1396-001..004 (metrics + marshalRCASubset)
  ├─ TDD-GREEN: Implement InvestigationMetrics + emitCompleteEvent extension
  ├─ TDD-REFACTOR: 100-go-mistakes validation
  └─ CHECKPOINT A: GA readiness audit

Phase B: AF RCA Parsing
  ├─ TDD-RED:   UT-AF-1396-020..021 (bridgeEventsCollectSummary)
  ├─ TDD-GREEN: Parse evt.Data on EventTypeComplete
  ├─ TDD-REFACTOR: 100-go-mistakes validation
  └─ CHECKPOINT B: GA readiness audit

Phase C: Truncation Fix (#1395)
  ├─ TDD-RED:   UT-AF-1395-001..005 (EmitStructuredMeta + sanitizeStructuredText)
  ├─ TDD-GREEN: Implement EmitStructuredMeta + wire callers
  ├─ TDD-REFACTOR: 100-go-mistakes validation
  └─ CHECKPOINT C: GA readiness audit

Phase D: AF Types + Schema Enforcement (#1396)
  ├─ TDD-RED:   UT-AF-1396-001..003, 010..011 (types + required RCA)
  ├─ TDD-GREEN: Add types to ka_tools.go
  ├─ TDD-REFACTOR: 100-go-mistakes validation + prompt update
  └─ CHECKPOINT D: GA readiness audit

Phase E: Integration Tests + Wiring Verification
  ├─ TDD-RED:   IT-AF-1395-001, IT-AF-1396-001..002
  ├─ TDD-GREEN: Wire all layers together
  ├─ TDD-REFACTOR: Final cleanup
  └─ CHECKPOINT E: GA readiness audit (CHECKPOINT W)

Phase F: E2E Tests (Journey Proof)
  ├─ TDD-RED:   E2E-AF-1396-001..002, E2E-AF-1395-001
  ├─ TDD-GREEN: Add mock-LLM af_structured_decision scenario + E2E test file
  ├─ TDD-REFACTOR: Cleanup, ensure deterministic scenario
  └─ CHECKPOINT F: Final GA readiness audit (Pyramid Invariant complete)
```

---

## 12. Test Deliverables

| Deliverable | Location | Format |
|-------------|----------|--------|
| Test Plan | `docs/tests/1395-1396/TEST_PLAN.md` | This document |
| Unit tests (KA) | `internal/kubernautagent/investigator/*_test.go`, `internal/kubernautagent/session/*_test.go` | Ginkgo BDD |
| Unit tests (AF) | `pkg/apifrontend/launcher/*_test.go`, `pkg/apifrontend/tools/*_test.go` | Ginkgo BDD |
| Integration tests | `pkg/apifrontend/launcher/*_test.go`, `pkg/apifrontend/tools/*_test.go` | Ginkgo BDD |
| E2E tests | `test/e2e/apifrontend/structured_decision_e2e_test.go` | Ginkgo BDD |
| Mock LLM scenario | `test/infrastructure/shared_e2e.go` (keyword addition) | YAML in ConfigMap |
| Coverage report | CI artifact | `go test -coverprofile` + `E2E_COVERAGE=true` |

---

## 13. Execution

```bash
# Unit tests (KA metrics + complete event)
go test ./internal/kubernautagent/investigator/... ./internal/kubernautagent/session/... -count=1

# Unit tests (AF truncation + types)
go test ./pkg/apifrontend/launcher/... ./pkg/apifrontend/tools/... -count=1

# Integration tests (full pipeline)
go test ./pkg/apifrontend/... -count=1 -run "IT-AF-139[56]"

# E2E tests (structured decision journey)
make test-e2e-apifrontend
# Or focused:
ginkgo -v --focus="structured decision" ./test/e2e/apifrontend/

# Coverage
go test ./pkg/apifrontend/launcher/... -coverprofile=cover.out -covermode=atomic
go tool cover -func=cover.out | grep -E "event_bridge|part_converter|ka_tools"
```

---

## 14. Go Anti-Pattern Validation (TDD Refactor Phase)

During each TDD Refactor phase, validate against [100 Go Mistakes](https://github.com/teivah/100-go-mistakes):

| Category | Checks Applied |
|----------|---------------|
| **#1 Unintended variable shadowing** | No `:=` in inner scope shadowing outer `err` or `result` |
| **#5 Interface pollution** | No unnecessary interfaces; BridgeMetrics is minimal |
| **#9 Being confused about when to use generics** | No premature generics; concrete types for RCAData |
| **#26 Slices and memory leaks** | `marshalRCASubset` does not retain references to large InvestigationResult |
| **#28 Maps and memory leaks** | No unbounded map growth in metrics |
| **#48 Forgetting about sync.Mutex copy** | EmitStructuredMeta uses pointer receiver (existing pattern) |
| **#53 Not handling defer errors** | No deferred Close() without error check in new code |
| **#54 Not handling panics in goroutines** | emitCompleteEvent caller already in defer-recover goroutine |
| **#77 JSON handling mistakes** | RCAData uses concrete types (no `interface{}`); `omitempty` semantics correct |
| **#78 Common SQL mistakes** | N/A (no SQL) |
| **#89 Writing inaccurate benchmarks** | N/A unless perf test added |
| **#100 Not understanding Go memory model** | Metrics accumulator uses value copy at read point; no concurrent access without mutex |

---

## 15. Checkpoint Protocol

At each checkpoint (A through E), perform:

1. **Build validation**: `go build ./...` passes
2. **Lint compliance**: `golangci-lint run --timeout=5m` (no new errors)
3. **Test regression**: All pre-existing tests pass
4. **Coverage check**: New code >=80% per tier
5. **Wiring verification** (Checkpoint E only): CHECKPOINT W per `10-wiring-verification.mdc`
6. **100-go-mistakes scan**: Refactored code free of listed anti-patterns
7. **Confidence assessment**: Must be >=95% to proceed; escalate if below
