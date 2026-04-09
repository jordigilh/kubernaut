# Test Plan: Mock LLM Deterministic UUIDs — Eliminate ConfigMap Sync (#561)

**Feature**: Remove ConfigMap-based UUID synchronization, wire optional YAML overrides, clean up dead infrastructure
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `development/v1.3`

**Authority**:
- [BR-MOCK-030]: Deterministic UUID Generation
- [BR-MOCK-031]: Shared UUID Function with DataStorage
- [BR-MOCK-032]: Environment Variable Configuration
- [BR-MOCK-033]: Optional YAML Scenario Overrides
- [DD-TEST-011]: File-based configuration pattern (partially superseded)

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../development/business-requirements/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [#548]: DS Deterministic UUIDs (prerequisite — landed)
- [#561]: Parent issue

---

## 1. Scope

### In Scope

- **config/overrides.go**: Wiring `LoadYAMLOverrides` into server startup when `MOCK_LLM_CONFIG_PATH` is set
- **cmd/mock-llm/main.go**: Startup integration of overrides with the scenario registry
- **scenarios/default.go**: Verifying scenarios use `uuid.DeterministicUUID` (already done) and accept override injection
- **Test infrastructure cleanup**: Removing `UpdateMockLLMConfigMap`, `WriteMockLLMConfigFile`, `SortedWorkflowUUIDKeys` from E2E/integration helpers

### Out of Scope

- `pkg/shared/uuid/uuid.go` — already implemented and tested under #548
- E2E full-pipeline tests — affected by infrastructure cleanup but not the subject of new test scenarios here
- Scenario detection logic — unchanged, covered by #564

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Keep `MOCK_LLM_CONFIG_PATH` for optional overrides | Some tests may need to override a UUID for specific scenarios; deterministic defaults handle the common case |
| Deprecate (not delete) ConfigMap sync | Active callers in E2E, HAPI integration, and AA integration suites prevent deletion. Functions get `// Deprecated:` doc comments + log warnings. Actual removal deferred to when those suites migrate |
| Overrides applied during registry construction | `DefaultRegistry(overrides)` accepts optional `*config.Overrides` and applies during scenario registration, keeping construction atomic |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (`config/overrides.go` pure parsing logic, scenario UUID generation)
- **Integration**: >=80% of integration-testable code (`main.go` startup wiring, server with/without overrides)

### 2-Tier Minimum

Every BR gap is covered by both Unit and Integration tiers.

### Business Outcome Quality Bar

Tests validate that the Mock LLM produces correct, deterministic workflow UUIDs without any external ConfigMap synchronization, and that optional overrides merge correctly on top of defaults.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `config/overrides.go` | `LoadYAMLOverrides` | ~48 |
| `scenarios/default.go` | `DefaultRegistry()`, scenario configs with `uuid.DeterministicUUID` | ~372 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `cmd/mock-llm/main.go` | `main()` startup wiring with override loading | ~60 |
| `handlers/router.go` | `NewFullRouter` — registry + override integration | ~97 |
| `test/infrastructure/holmesgpt_api.go` | `UpdateMockLLMConfigMap` (to be deprecated, not deleted — has active E2E callers) | ~50 |
| `test/infrastructure/workflow_seeding.go` | `WriteMockLLMConfigFile`, `SortedWorkflowUUIDKeys` (to be deprecated — has active HAPI/AA callers) | ~40 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-MOCK-030 | Deterministic UUID generation | P0 | Unit | UT-MOCK-030-001 | Pass |
| BR-MOCK-030 | Different workflows produce different UUIDs | P0 | Unit | UT-MOCK-030-002 | Pass |
| BR-MOCK-031 | Shared UUID consistent with DataStorage | P0 | Unit | UT-MOCK-031-001 | Pass |
| BR-MOCK-033 | YAML override merges on deterministic defaults | P1 | Unit | UT-MOCK-033-001 | Pass |
| BR-MOCK-033 | Missing YAML file falls back gracefully | P1 | Unit | UT-MOCK-033-002 | Pass |
| BR-MOCK-033 | Override applied to registry at startup | P1 | Integration | IT-MOCK-561-001 | Pending |
| BR-MOCK-033 | Server starts correctly without override file | P0 | Integration | IT-MOCK-561-002 | Pending |
| BR-MOCK-030 | Scenarios serve deterministic UUIDs via HTTP | P0 | Integration | IT-MOCK-561-003 | Pending |
| BR-MOCK-032 | MOCK_LLM_CONFIG_PATH env var wired at startup | P1 | Integration | IT-MOCK-561-004 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-MOCK-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: MOCK (Mock LLM)
- **BR_NUMBER**: Business requirement number or issue number for wiring tests

### Tier 1: Unit Tests

**Testable code scope**: `config/overrides.go`, `scenarios/default.go` — target >=80% coverage

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-MOCK-030-001` | Same workflow name always produces the same UUID | Pass |
| `UT-MOCK-030-002` | Different workflow names produce different UUIDs | Pass |
| `UT-MOCK-031-001` | Shared UUID is consistent from Mock LLM context | Pass |
| `UT-MOCK-033-001` | YAML override merges on top of deterministic defaults | Pass |
| `UT-MOCK-033-002` | Missing YAML file falls back gracefully | Pass |

### Tier 2: Integration Tests

**Testable code scope**: `main.go` startup, `handlers/router.go`, server HTTP behavior — target >=80% coverage

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-MOCK-561-001` | When YAML override file exists with a custom workflow_id, Mock LLM serves the overridden UUID in tool call responses | Pending |
| `IT-MOCK-561-002` | When no override file exists, Mock LLM starts cleanly and serves deterministic default UUIDs | Pending |
| `IT-MOCK-561-003` | Workflow UUID in tool call response matches `uuid.DeterministicUUID(workflowName)` | Pending |
| `IT-MOCK-561-004` | `MOCK_LLM_CONFIG_PATH` env var is consumed at startup and overrides are loaded from the specified path | Pending |

### Tier Skip Rationale

- **E2E**: Deferred — ConfigMap sync functions are deprecated (not deleted) because E2E and other integration suites still call them. When those suites are migrated to deterministic UUIDs, the deprecated functions will be removed. No new E2E scenarios needed.

---

## 6. Test Cases (Detail)

### IT-MOCK-561-001: YAML Override Applied via HTTP

**BR**: BR-MOCK-033
**Type**: Integration
**File**: `test/integration/mockllm/override_wiring_test.go`

**Given**: A YAML file at a temp path with `scenarios: { oomkilled: { workflow_id: "custom-uuid-override" } }`
**When**: Mock LLM server starts with `MOCK_LLM_CONFIG_PATH` pointing to that file, and a chat completion request triggers the oomkilled scenario
**Then**: The tool call response arguments include `"custom-uuid-override"` as the workflow ID

**Acceptance Criteria**:
- Overridden UUID appears in the `search_workflow_catalog` tool call arguments
- Non-overridden scenarios still use deterministic defaults

---

### IT-MOCK-561-002: Startup Without Override File

**BR**: BR-MOCK-030, BR-MOCK-033
**Type**: Integration
**File**: `test/integration/mockllm/override_wiring_test.go`

**Given**: No override file exists, `MOCK_LLM_CONFIG_PATH` is empty
**When**: Mock LLM server starts and a request triggers the oomkilled scenario
**Then**: Tool call response arguments contain the deterministic UUID from `uuid.DeterministicUUID("oom-recovery")`

**Acceptance Criteria**:
- Server starts without error
- `/health` returns 200
- UUID matches `uuid.DeterministicUUID` output exactly

---

### IT-MOCK-561-003: Deterministic UUID End-to-End via HTTP

**BR**: BR-MOCK-030
**Type**: Integration
**File**: `test/integration/mockllm/override_wiring_test.go`

**Given**: Mock LLM server running with no overrides
**When**: Sending requests for multiple different scenarios (oomkilled, crashloop, node_not_ready)
**Then**: Each scenario's workflow UUID in the response matches `uuid.DeterministicUUID(workflowName)` exactly

**Acceptance Criteria**:
- At least 3 different scenarios verified
- UUIDs are stable across multiple requests (determinism)

---

### IT-MOCK-561-004: Config Path Env Var Wired

**BR**: BR-MOCK-032
**Type**: Integration
**File**: `test/integration/mockllm/server_config_test.go` (extend existing)

**Given**: `MOCK_LLM_CONFIG_PATH` set to a valid YAML override file path
**When**: Server starts
**Then**: Overrides from the file are applied to the registry

**Acceptance Criteria**:
- Can be verified via HTTP request to an overridden scenario

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None — pure logic tests
- **Location**: `test/unit/mockllm/`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks — real HTTP server via `httptest.NewServer`
- **Infrastructure**: Temp files for YAML overrides
- **Location**: `test/integration/mockllm/`

---

## 8. Execution

```bash
# Unit tests
go test ./test/unit/mockllm/... -v -count=1

# Integration tests
go test ./test/integration/mockllm/... -v -count=1

# Specific test by ID
go test ./test/integration/mockllm/... -ginkgo.focus="IT-MOCK-561-001"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
