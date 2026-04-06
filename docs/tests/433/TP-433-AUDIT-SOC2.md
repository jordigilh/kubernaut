# Test Plan: KA Audit Parity and SOC2 Compliance

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-433-AUDIT-SOC2-v1.0
**Feature**: Kubernaut Agent (KA) audit event parity with HAPI and SOC2 CC8.1 compliance — fully populated audit payloads enabling complete LLM conversation reconstruction from DataStorage audit trails.
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.3`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates that the Kubernaut Agent (KA) emits fully populated audit events for all 8 event types, achieving parity with the Python HAPI implementation and satisfying SOC2 CC8.1 requirements for complete LLM interaction reconstruction from audit traces.

KA currently emits all 8 event types but populates almost no data beyond correlation IDs and basic token counts. This creates a compliance gap: DataStorage stores structurally empty payloads that cannot reconstruct the agent-LLM conversation. This test plan covers closing 8 audit gaps (GAP-A1 through GAP-A8) identified in the KA-HAPI audit parity analysis, plus OpenAPI schema extensions for full data fidelity.

### 1.2 Objectives

1. **Payload completeness**: All 6 investigator event types (`llm.request`, `llm.response`, `llm.tool_call`, `validation_attempt`, `response.complete`, `response.failed`) carry fully populated OpenAPI-typed payloads
2. **SOC2 CC8.1 reconstruction**: Every LLM turn (prompt, response, tool calls, validation) is reconstructable from audit events queried by `correlation_id`
3. **ADR-034 compliance**: All events carry `event_id` (UUID), `EventAction`, `EventOutcome`, `ActorType`, `ActorID`
4. **Schema fidelity**: OpenAPI `IncidentResponseData` extended with `remediationTarget`, `executionBundle`, `confidence` — zero structural data loss
5. **No regressions**: Existing enrichment audit tests (`UT-KA-433W-010..011`, `IT-KA-433-ENR-*`) and adversarial tests (`UT-KA-433-AUD-*`) continue to pass
6. **Per-tier coverage**: >=80% on unit-testable audit code, >=80% on integration-testable audit code

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/audit/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/investigator/...` |
| E2E test pass rate | 100% | `go test ./test/e2e/kubernautagent/... -ginkgo.focus=AP` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `internal/kubernautagent/audit/` |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on `internal/kubernautagent/investigator/` |
| Existing test regressions | 0 | `UT-KA-433W-*`, `UT-KA-433-AUD-*`, `IT-KA-433-ENR-*`, `E2E-HAPI-045..048` |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-AUDIT-005**: Audit event persistence and queryability by `remediation_id`
- **DD-AUDIT-005**: Hybrid provider data capture — KA owns complete `IncidentResponse` in audit traces
- **SOC2 CC8.1**: Complete remediation request reconstruction from audit traces
- **ADR-034**: Unified audit table design with event-sourcing pattern
- **DD-AUDIT-003**: Per-service audit trace requirements (KA replaces HAPI for `aiagent.*` events)
- **BR-AUDIT-021-030**: Workflow selection audit trail
- Issue #433: Kubernaut Agent Go rewrite

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [TP-433-v1.0](TEST_PLAN.md) — Parent test plan for KA Go rewrite
- [TP-433-ADV](TP-433-ADV.md) — Adversarial parity test plan
- [TP-433-WIR-v1.0](TP-433-WIR-v1.0.md) — Wiring test plan (enrichment audit)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | OpenAPI schema extension breaks existing DS consumers | DataStorage API contract violation | Low | All E2E | New fields are optional/additive; no required field changes on existing schemas |
| R2 | `jx.Raw` encoding produces malformed JSON for tool results | DataStorage rejects audit events | Medium | UT-KA-433-AP-009, IT-KA-433-AP-003 | Unit test validates `jx.Raw` round-trip; IT verifies end-to-end |
| R3 | `IncidentResponseData` mapper fails on edge cases (nil severity, empty parameters) | Runtime panic in production | Medium | UT-KA-433-AP-015..019 | Dedicated edge-case unit tests for nil/empty handling |
| R4 | Per-tool-call emission increases audit volume, overwhelming DS | Performance degradation | Low | E2E-KA-433-AP-001 | Fire-and-forget pattern (`StoreBestEffort`) prevents blocking; DS handles batching |
| R5 | Stale unit test UT-KA-433W-012 conflicts with new `buildEventData` behavior | False test failures | High | UT-KA-433W-012 | Update test in Phase 1-RED to expect populated `LLMRequestPayload` |

### 3.1 Risk-to-Test Traceability

- **R1**: Mitigated by E2E-KA-433-AP-001 (full pipeline test proves DS accepts new payloads)
- **R2**: Mitigated by UT-KA-433-AP-009 (unit-level `jx.Raw` validation) + IT-KA-433-AP-003 (integration-level)
- **R3**: Mitigated by UT-KA-433-AP-015..019 (comprehensive mapper edge cases)
- **R5**: Mitigated by updating UT-KA-433W-012 in Phase 1-RED

---

## 4. Scope

### 4.1 Features to be Tested

- **Audit emitter** (`internal/kubernautagent/audit/emitter.go`): UUID `event_id` generation, `EventAction`/`EventOutcome` constants
- **DS audit store** (`internal/kubernautagent/audit/ds_store.go`): `ActorType`/`ActorID` on requests, `buildEventData` for all 8 event types, `toIncidentResponseData` mapper
- **Investigator audit emission** (`internal/kubernautagent/investigator/investigator.go`): Data population at each audit emission point — model name threading, prompt/analysis preview, per-tool-call granularity, per-attempt validation, error details, cumulative tokens, full `IncidentResponseData`
- **OpenAPI schema** (`api/openapi/data-storage-v1.yaml`): Extended `IncidentResponseData` fields

### 4.2 Features Not to be Tested

- **Enrichment audit events** (`aiagent.enrichment.*`): Already covered by TP-433-WIR Phase 7 (UT-KA-433W-010..011, IT-KA-433-ENR-002..008)
- **Audit shared library** (`pkg/audit/`): DataStorage's responsibility, covered by DS test plans
- **DataStorage persistence logic**: DS service responsibility
- **LLM provider integration**: Covered by KA E2E parity tests (TP-433-ADV)

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Unit tests directly test `buildEventData` and `toIncidentResponseData` as pure functions | Correct pattern per TESTING_GUIDELINES.md: these are stateless mapping functions with no I/O |
| Integration tests use `recordingAuditStore` to capture events from `Investigator.Investigate()` | Correct pattern: tests business logic that emits audits as side effect, not audit infrastructure |
| E2E tests query DataStorage API after HTTP investigation | Correct pattern: validates full pipeline including DS persistence |
| Extend OpenAPI schema rather than accept data loss | User decision: SOC2 requires full fidelity; no structural data loss acceptable |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code in `internal/kubernautagent/audit/` (pure logic: `NewEvent`, `buildEventData`, `toIncidentResponseData`, `dataString`/`dataInt`/`dataBool`)
- **Integration**: >=80% of integration-testable code in `internal/kubernautagent/investigator/` audit emission paths (I/O: LLM client interaction, audit store calls, tool execution)
- **E2E**: Container contract — validates audit events persist to DataStorage and are queryable

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers:
- **Unit tests**: Validate mapping correctness and edge cases (fast feedback)
- **Integration tests**: Validate investigator emits correct data at correct points (cross-component)
- **E2E tests**: Validate full persistence pipeline (system-level)

### 5.3 Business Outcome Quality Bar

Tests validate observable audit outcomes:
- "Can an operator reconstruct the LLM prompt from audit events?" (GAP-A1)
- "Can an operator see which tools the LLM called and what they returned?" (GAP-A3)
- "Can an operator see the complete incident response in the audit trail?" (GAP-A5)
- "Can an operator identify the service that emitted each event?" (GAP-A8)

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier code coverage meets >=80% threshold
4. No regressions in existing audit tests (`UT-KA-433W-*`, `UT-KA-433-AUD-*`, `IT-KA-433-ENR-*`, `E2E-HAPI-045..048`)
5. All 6 investigator event types produce non-empty `EventData` through `buildEventData`
6. `response.complete` event contains full `IncidentResponseData` with extended schema fields

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80% on any tier
3. Existing audit tests regress
4. Any event type produces empty `EventData` for a populated `AuditEvent`

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:

- OpenAPI schema extension breaks ogen code generation (`make generate-datastorage-client` fails)
- `go build ./...` fails in the v1.3 worktree
- DataStorage API rejects new payload schemas in E2E

**Resume testing when**:

- Schema fix applied and ogen regeneration succeeds
- Build restored
- DS schema alignment confirmed

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/audit/emitter.go` | `NewEvent` (+ UUID generation), constants | ~83 |
| `internal/kubernautagent/audit/ds_store.go` | `buildEventData` (8 cases), `toIncidentResponseData` (new), `dataString`, `dataInt`, `dataBool`, `StoreAudit` (ActorType/ActorID) | ~202 (+~80 for mapper) |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/investigator/investigator.go` | `Investigate`, `runLLMLoop`, `runWorkflowSelection` — audit emission points | ~545 |
| `cmd/kubernautagent/main.go` | Model name threading into Investigator config | ~5 (wiring) |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.3` HEAD | v1.3 worktree |
| OpenAPI spec | `data-storage-v1.yaml` (extended) | After Phase 0 schema changes |
| ogen-go | v1.18.0 | Per `gen.go` |
| github.com/google/uuid | v1.6.0 | Per `go.mod` |
| github.com/go-faster/jx | Per `go.mod` | Required for `jx.Raw` |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AUDIT-005 | Audit event persistence by remediation_id | P0 | Unit | UT-KA-433-AP-001 | Pending |
| BR-AUDIT-005 | Audit event persistence by remediation_id | P0 | E2E | E2E-KA-433-AP-001 | Pending |
| DD-AUDIT-005 | Complete IncidentResponse in response.complete | P0 | Unit | UT-KA-433-AP-014..019 | Pending |
| DD-AUDIT-005 | Complete IncidentResponse in response.complete | P0 | Integration | IT-KA-433-AP-005 | Pending |
| DD-AUDIT-005 | Complete IncidentResponse in response.complete | P0 | E2E | E2E-KA-433-AP-002 | Pending |
| SOC2 CC8.1 | Full conversation reconstruction | P0 | Unit | UT-KA-433-AP-004..010 | Pending |
| SOC2 CC8.1 | Full conversation reconstruction | P0 | Integration | IT-KA-433-AP-001..003 | Pending |
| ADR-034 | event_id UUID on every event | P0 | Unit | UT-KA-433-AP-001 | Pending |
| ADR-034 | event_id UUID on every event | P0 | Integration | IT-KA-433-AP-007 | Pending |
| ADR-034 | EventAction/EventOutcome on every event | P0 | Unit | UT-KA-433-AP-002 | Pending |
| ADR-034 | EventAction/EventOutcome on every event | P0 | Integration | IT-KA-433-AP-008 | Pending |
| ADR-034 | ActorType/ActorID on every event | P0 | Unit | UT-KA-433-AP-003 | Pending |
| ADR-034 | ActorType/ActorID on every event | P0 | E2E | E2E-KA-433-AP-003 | Pending |
| DD-AUDIT-005 | Error details in response.failed | P1 | Unit | UT-KA-433-AP-011 | Pending |
| DD-AUDIT-005 | Error details in response.failed | P1 | Integration | IT-KA-433-AP-006 | Pending |
| BR-AUDIT-021-030 | Per-attempt validation audit | P1 | Unit | UT-KA-433-AP-012..013 | Pending |
| BR-AUDIT-021-030 | Per-attempt validation audit | P1 | Integration | IT-KA-433-AP-004 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-KA-433-AP-{NNN}` (AP = Audit Parity)

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: `KA` (Kubernaut Agent)
- **AP**: Audit Parity (distinguishes from existing AUD/W test series)

### Tier 1: Unit Tests

**Testable code scope**: `internal/kubernautagent/audit/emitter.go`, `internal/kubernautagent/audit/ds_store.go` — >=80% coverage target

**File**: `test/unit/kubernautagent/audit/ds_store_audit_parity_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-433-AP-001` | `NewEvent` auto-generates UUID `event_id` in `Data["event_id"]` — enables per-event traceability (ADR-034) | Pending |
| `UT-KA-433-AP-002` | `EventAction`/`EventOutcome` constants defined for all 6 investigator event types — enables event classification (ADR-034) | Pending |
| `UT-KA-433-AP-003` | `StoreAudit` sets `ActorType="Service"`, `ActorID="kubernaut-agent"` — enables source attribution (ADR-034) | Pending |
| `UT-KA-433-AP-004` | `buildEventData` maps `LLMRequestPayload` with all required fields — enables prompt reconstruction (SOC2 CC8.1) | Pending |
| `UT-KA-433-AP-005` | `prompt_preview` truncates at 500 chars — prevents payload bloat while preserving preview (DD-AUDIT-005) | Pending |
| `UT-KA-433-AP-006` | `prompt_preview` selects last `role=user` message — ensures correct content for preview (DD-AUDIT-005) | Pending |
| `UT-KA-433-AP-007` | `buildEventData` maps `LLMResponsePayload` with analysis fields — enables response reconstruction (SOC2 CC8.1) | Pending |
| `UT-KA-433-AP-008` | `analysis_preview` truncates at 500 chars — prevents payload bloat (DD-AUDIT-005) | Pending |
| `UT-KA-433-AP-009` | `buildEventData` maps `LLMToolCallPayload` with `jx.Raw` — enables tool interaction reconstruction (SOC2 CC8.1) | Pending |
| `UT-KA-433-AP-010` | `tool_result_preview` truncates at 500 chars — prevents payload bloat (DD-AUDIT-005) | Pending |
| `UT-KA-433-AP-011` | `buildEventData` maps `AIAgentResponseFailedPayload` with error details — enables failure analysis (DD-AUDIT-005) | Pending |
| `UT-KA-433-AP-012` | `buildEventData` maps `WorkflowValidationPayload` with attempt details — enables validation audit (BR-AUDIT-021-030) | Pending |
| `UT-KA-433-AP-013` | Validation failure event has `EventOutcome="failure"` — enables outcome filtering (ADR-034) | Pending |
| `UT-KA-433-AP-014` | `buildEventData` maps `AIAgentResponsePayload` with full `IncidentResponseData` — enables complete response reconstruction (DD-AUDIT-005) | Pending |
| `UT-KA-433-AP-015` | `toIncidentResponseData` maps severity to ogen enum — ensures schema compliance | Pending |
| `UT-KA-433-AP-016` | `toIncidentResponseData` maps parameters to `jx.Raw` — ensures type-safe serialization | Pending |
| `UT-KA-433-AP-017` | `toIncidentResponseData` maps alternatives with extended schema fields — ensures no data loss | Pending |
| `UT-KA-433-AP-018` | `toIncidentResponseData` maps cumulative token totals — enables cost tracking | Pending |
| `UT-KA-433-AP-019` | `toIncidentResponseData` handles nil/empty optionals without panic — ensures robustness | Pending |

**Stale test update**: `UT-KA-433W-012` in `ds_store_test.go` must be updated to expect populated `LLMRequestPayload` EventData (currently asserts empty, contradicting `buildEventData` implementation).

### Tier 2: Integration Tests

**Testable code scope**: `internal/kubernautagent/investigator/` audit emission paths — >=80% coverage target

**File**: `test/integration/kubernautagent/investigator/investigator_audit_parity_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-433-AP-001` | Investigation emits `llm.request` with model name and `prompt_preview` — operator can see what was sent to LLM | Pending |
| `IT-KA-433-AP-002` | Investigation emits `llm.response` with `has_analysis` and `analysis_preview` — operator can see LLM response | Pending |
| `IT-KA-433-AP-003` | Investigation emits per-tool-call events with `tool_name` and `tool_result` — operator can reconstruct tool interactions | Pending |
| `IT-KA-433-AP-004` | Investigation emits `validation_attempt` per self-correction attempt — operator can see validation history | Pending |
| `IT-KA-433-AP-005` | Investigation emits `response.complete` with cumulative tokens and response data — operator can see final result | Pending |
| `IT-KA-433-AP-006` | Investigation emits `response.failed` with `error_message` and `phase` — operator can diagnose failures | Pending |
| `IT-KA-433-AP-007` | All investigator events have UUID `event_id` in Data — enables per-event traceability | Pending |
| `IT-KA-433-AP-008` | All investigator events have `EventAction` and `EventOutcome` set — enables event classification | Pending |

### Tier 3: E2E Tests

**Testable code scope**: Full stack — KA HTTP API -> Investigator -> DSAuditStore -> DataStorage PostgreSQL

**File**: `test/e2e/kubernautagent/audit_pipeline_test.go` (extend existing)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-KA-433-AP-001` | Full investigation audit trail in DataStorage contains all 6 investigator event types with populated payloads | Pending |
| `E2E-KA-433-AP-002` | `response.complete` audit event in DataStorage contains full `IncidentResponseData` | Pending |
| `E2E-KA-433-AP-003` | All audit events in DataStorage have `ActorType="Service"`, `ActorID="kubernaut-agent"` | Pending |

### Tier Skip Rationale

No tier is skipped. All three tiers are applicable and required.

---

## 9. Test Cases

### UT-KA-433-AP-001: NewEvent generates UUID event_id

**BR**: ADR-034, BR-AUDIT-005
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/audit/ds_store_audit_parity_test.go`

**Preconditions**:
- `github.com/google/uuid` v1.6.0 available in `go.mod`

**Test Steps**:
1. **Given**: No prior state
2. **When**: `audit.NewEvent(audit.EventTypeLLMRequest, "rem-123")` is called
3. **Then**: `event.Data["event_id"]` is a non-empty string that parses as a valid UUID v4

**Expected Results**:
1. `event.Data["event_id"]` is set
2. `uuid.Parse(event.Data["event_id"].(string))` succeeds
3. Two consecutive calls produce different UUIDs

**Acceptance Criteria**:
- **Behavior**: Every `NewEvent` call auto-generates a unique event_id
- **Correctness**: event_id is valid UUID v4 format
- **Accuracy**: No duplicate event_ids

### UT-KA-433-AP-003: StoreAudit sets ActorType and ActorID

**BR**: ADR-034, DD-AUDIT-005
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/audit/ds_store_audit_parity_test.go`

**Preconditions**:
- `fakeOgenClient` from `helpers_test.go`

**Test Steps**:
1. **Given**: A `DSAuditStore` backed by a fake ogen client
2. **When**: `StoreAudit` is called with any event
3. **Then**: The ogen `AuditEventRequest` has `ActorType` set to `"Service"` and `ActorID` set to `"kubernaut-agent"`

**Expected Results**:
1. `req.ActorType.Value` equals `"Service"`
2. `req.ActorID.Value` equals `"kubernaut-agent"`

### UT-KA-433-AP-014: buildEventData maps AIAgentResponsePayload with full IncidentResponseData

**BR**: DD-AUDIT-005, SOC2 CC8.1
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/audit/ds_store_audit_parity_test.go`

**Preconditions**:
- OpenAPI schema extended with `remediationTarget`, `executionBundle`, `confidence` on alternatives

**Test Steps**:
1. **Given**: An `AuditEvent` of type `EventTypeResponseComplete` with `Data` containing serialized `InvestigationResult` with RCA, workflow, alternatives, parameters, and cumulative tokens
2. **When**: `StoreAudit` is called
3. **Then**: The ogen request contains `AIAgentResponsePayload` with fully populated `IncidentResponseData`

**Expected Results**:
1. `response_data.rootCauseAnalysis.summary` matches input RCA summary
2. `response_data.rootCauseAnalysis.severity` is a valid enum value
3. `response_data.selectedWorkflow.workflowId` matches input
4. `response_data.alternativeWorkflows` length matches input
5. `response_data.confidence` matches input (within float32 precision)
6. `total_prompt_tokens` and `total_completion_tokens` are set

### IT-KA-433-AP-003: Investigation emits per-tool-call events

**BR**: SOC2 CC8.1, DD-AUDIT-005
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/investigator/investigator_audit_parity_test.go`

**Preconditions**:
- `recordingAuditStore` captures events
- `mockLLMClient` returns response with 2 tool calls

**Test Steps**:
1. **Given**: An investigator configured with a mock LLM that returns 2 tool calls in a single turn
2. **When**: `Investigate()` is called
3. **Then**: 2 separate `aiagent.llm.tool_call` events are emitted, each with distinct `tool_call_index`, `tool_name`, and `tool_result`

**Expected Results**:
1. `recordingAuditStore` contains exactly 2 events with `EventType == EventTypeLLMToolCall`
2. Event 0 has `tool_call_index=0`, event 1 has `tool_call_index=1`
3. Each event has non-empty `tool_name` and `tool_result`

### E2E-KA-433-AP-001: Full audit trail in DataStorage with populated payloads

**BR**: BR-AUDIT-005, DD-AUDIT-005, SOC2 CC8.1
**Priority**: P0
**Type**: E2E
**File**: `test/e2e/kubernautagent/audit_pipeline_test.go`

**Preconditions**:
- Kind cluster with KA, mock-llm, DataStorage deployed
- SA token for audit query authentication

**Test Steps**:
1. **Given**: A running KA with DataStorage audit store
2. **When**: An investigation is triggered via HTTP API
3. **Then**: DataStorage returns audit events for the `correlation_id` containing all 6 investigator event types with non-empty `event_data`

**Expected Results**:
1. `QueryAuditEvents` returns events for `correlation_id = remediation_id`
2. Event types include: `llm.request`, `llm.response`, `llm.tool_call`, `validation_attempt`, `response.complete`
3. Each event's `event_data` is non-empty (has typed payload)
4. `llm.request` event contains `model` and `prompt_preview`
5. `response.complete` event contains `response_data` with `rootCauseAnalysis`

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `fakeOgenClient` (existing in `helpers_test.go`) — records ogen requests
- **Location**: `test/unit/kubernautagent/audit/`
- **Resources**: Minimal (pure logic, no I/O)

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `mockLLMClient` (existing in `investigator_test.go`), `recordingAuditStore` (existing)
- **Infrastructure**: No external services (mock LLM client, in-memory audit store)
- **Location**: `test/integration/kubernautagent/investigator/`

### 10.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Kind cluster with KA, mock-llm, DataStorage, PostgreSQL
- **Location**: `test/e2e/kubernautagent/`
- **Resources**: Kind cluster (~4GB RAM), Docker/Podman

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| ogen-go | v1.18.0 | OpenAPI client generation |
| Kind | v0.20+ | E2E cluster |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Phase 0: OpenAPI schema extension | Code | Pending | UT-KA-433-AP-014..019, E2E-KA-433-AP-002 blocked | Use existing schema with data loss (not acceptable per user decision) |
| `github.com/google/uuid` | Library | Available in go.mod | UT-KA-433-AP-001 blocked | None needed |
| `github.com/go-faster/jx` | Library | Available in go.mod | UT-KA-433-AP-009 blocked | None needed |

### 11.2 Execution Order

1. **Phase 0**: OpenAPI schema extension + ogen regeneration (infrastructure prerequisite)
2. **Phase 1**: Foundation tests (UUID, constants, ActorType/ActorID)
3. **Phase 2**: LLM request tests
4. **Phase 3**: LLM response tests
5. **Phase 4**: Tool call tests
6. **Phase 5**: Response failed tests
7. **Phase 6**: Validation tests
8. **Phase 7**: Response complete tests (depends on Phase 0 for extended schema)
9. **Phase 8**: E2E tests (depends on all prior phases)
10. **Phase 9**: Documentation updates

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/433/TP-433-AUDIT-SOC2.md` | Strategy and test design |
| Unit test suite | `test/unit/kubernautagent/audit/ds_store_audit_parity_test.go` | 19 Ginkgo BDD tests |
| Integration test suite | `test/integration/kubernautagent/investigator/investigator_audit_parity_test.go` | 8 Ginkgo BDD tests |
| E2E test extensions | `test/e2e/kubernautagent/audit_pipeline_test.go` | 3 Ginkgo BDD tests |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/kubernautagent/audit/... -ginkgo.v

# Integration tests
go test ./test/integration/kubernautagent/investigator/... -ginkgo.v

# Specific test by ID
go test ./test/unit/kubernautagent/audit/... -ginkgo.focus="UT-KA-433-AP"

# E2E tests (requires Kind cluster)
go test ./test/e2e/kubernautagent/... -ginkgo.focus="AP" -ginkgo.v

# Coverage
go test ./test/unit/kubernautagent/audit/... -coverprofile=audit_ut_coverage.out
go tool cover -func=audit_ut_coverage.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `UT-KA-433W-012` in `ds_store_test.go:134` | Expects `EventData.Type` to be empty for LLM request events | Must expect `LLMRequestPayloadAuditEventRequestEventData` type | `buildEventData` now returns populated `LLMRequestPayload` for all event types including LLM request |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan covering GAP-A1..A8 and SOC2 CC8.1 |
