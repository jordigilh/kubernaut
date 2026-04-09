# Test Plan: Extend Mock-LLM for Tool Call Scenarios

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-657-v1.0
**Feature**: Per-scenario tool call overrides in mock-LLM, kubectl argument builders, and poisoned resource E2E fixture
**Version**: 1.0
**Created**: 2026-04-09
**Author**: AI Assistant
**Status**: Active
**Branch**: `development/v1.4`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the extension of the mock-LLM service to support per-scenario
tool call overrides, enabling end-to-end testing of the #601 prompt injection guardrail
pipeline. Without this, the primary attack vector (attacker-controlled content in
Kubernetes resources flowing through tool call results into the LLM context) cannot be
exercised in E2E tests.

### 1.2 Objectives

1. **Per-scenario forceText override**: A scenario can opt out of global `MOCK_LLM_FORCE_TEXT=true` to return tool calls
2. **kubectl tool argument support**: `buildToolArguments` produces correct `{kind, name, namespace}` for kubectl tools
3. **Custom tool call bypass**: A scenario with `ToolCallName` set bypasses the DAG and returns that tool call directly
4. **Injection scenario**: A new scenario detects `injection_configmap_read` signals and triggers kubectl tool calls
5. **Poisoned resource E2E**: An E2E test creates a ConfigMap with injection content, triggers tool execution, and verifies shadow alignment flag
6. **Backward compatibility**: All existing mock-LLM tests (19 UT + 14 IT) pass without modification

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/mockllm/...` |
| Integration test pass rate | 100% | `go test ./test/integration/mockllm/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on config, scenarios, response packages |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on handlers package |
| Backward compatibility | 0 regressions | All existing mock-LLM + KA tests pass without modification |
| E2E injection pipeline | 1/1 | Poisoned ConfigMap triggers shadow alignment flag |

---

## 2. References

### 2.1 Authority (governing documents)

- Issue #657: Extend mock-LLM to support tool call scenarios for E2E security testing
- Issue #601: Prompt injection guardrails for Kubernaut Agent agentic pipeline
- BR-AI-601: Shadow agent alignment check for prompt injection detection

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- [#601 Test Plan](../601/TEST_PLAN_v2.md)
- [Mock-LLM Business Requirements](../../services/test-infrastructure/mock-llm/BUSINESS_REQUIREMENTS.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Per-scenario forceText breaks existing E2E scenarios | All E2E tests fail | Low | IT-MOCK-657-002 | `*bool` nil semantics = fall through to global; only explicitly set overrides change behavior |
| R2 | Custom tool call response mismatches KA expectations | KA rejects tool call, test hangs | Medium | UT-MOCK-657-008, IT-MOCK-657-004 | Unit tests validate response format matches `openai.ChatCompletionResponse` with proper `tool_calls` |
| R3 | Poisoned ConfigMap not found during E2E execution | Tool execution returns error | Medium | E2E-MOCK-657-001 | Create ConfigMap in BeforeEach; verify existence before investigation |
| R4 | DAG engine interference with custom tool call path | Unexpected routing | Low | IT-MOCK-657-006 | Bypass DAG entirely when `ToolCall` override is active |
| R5 | Shadow handler doesn't detect injection in tool result | E2E test doesn't flag suspicious content | Low | E2E-MOCK-657-001 | Poisoned content includes known patterns ("system:", "ignore previous") |

### 3.1 Risk-to-Test Traceability

- R1 mitigated by IT-MOCK-657-002 (nil ForceText falls through to global)
- R2 mitigated by UT-MOCK-657-008 (valid response JSON) and IT-MOCK-657-004 (tool call response)
- R3 mitigated by E2E-MOCK-657-001 setup phase (BeforeEach ConfigMap creation)
- R4 mitigated by IT-MOCK-657-006 (no ToolCallName follows normal DAG)
- R5 mitigated by shadow handler substring detection (verified in preflight)

---

## 4. Scope

### 4.1 Features to be Tested

- **Override types** (`test/services/mock-llm/config/overrides.go`): `ForceText *bool` and `ToolCall *ToolCallOverride` YAML parsing
- **Scenario config** (`test/services/mock-llm/scenarios/types.go`): `ForceText *bool`, `ToolCallName string`, `ToolCallArgs` fields
- **Override application** (`test/services/mock-llm/scenarios/registry_default.go`): `applyOverride` extension
- **kubectl arguments** (`test/services/mock-llm/response/openai.go`): `buildToolArguments` for kubectl tools
- **Per-scenario forceText** (`test/services/mock-llm/handlers/openai.go`): Scenario-level override of global flag
- **Custom tool call bypass** (`test/services/mock-llm/handlers/openai.go`): DAG bypass when `ToolCallName` is set
- **Injection scenario** (`test/services/mock-llm/scenarios/scenario_injection.go`): New scenario for injection testing
- **E2E fixture** (`test/infrastructure/shared_e2e.go`): Poisoned ConfigMap creation helper

### 4.2 Features Not to be Tested

- **Shadow handler logic**: Already tested in #601 test plan; no changes to shadow handler in this issue
- **Boundary wrapping in evaluator**: Covered by #601 unit tests
- **Conversation adapter tool calls**: Out of scope; #657 targets the investigation pipeline only
- **DAG engine internals**: No changes to DAG engine; only bypass path added

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Option B: Handler bypass instead of new DAG | Simpler, avoids DAG engine complexity; sufficient for single-tool-call scenarios |
| `*bool` for ForceText | Nil = use global default; false = force tool calls; true = force text. Backward compatible. |
| `hasToolResults` for multi-turn detection | Standard OpenAI `role: "tool"` messages; verified in preflight that both KA and mock-LLM use same convention |
| kubectl tool names as string constants | Added to `pkg/shared/types/openai/tool.go` for consistency with existing constants |
| Poisoned content uses known shadow patterns | Shadow handler uses substring matching; content includes "SYSTEM:" and "ignore previous" for reliable detection |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (config parsing, scenario types, override application, buildToolArguments, scenario detection)
- **Integration**: >=80% of integration-testable code (OpenAI handler with per-scenario forceText, custom tool call bypass, multi-turn flow)
- **E2E**: Container contract validation (poisoned ConfigMap → tool call → shadow evaluation pipeline)

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers:
- Override types: UT (parsing) + IT (handler behavior)
- kubectl arguments: UT (pure logic) + IT (response in handler)
- ForceText override: UT (config) + IT (handler)
- Tool call bypass: IT (handler) + E2E (full pipeline)

### 5.3 Business Outcome Quality Bar

Tests validate business outcomes:
- "Can the mock-LLM return tool calls for a specific scenario even when force-text is globally enabled?"
- "Does the mock-LLM produce kubectl tool call arguments that KA can execute?"
- "Does the full pipeline detect injection content in tool output?"

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier code coverage meets >=80% threshold
4. No regressions in existing mock-LLM or KA test suites
5. E2E-MOCK-657-001 triggers shadow alignment flag on poisoned content

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80% on any tier
3. Existing tests regress
4. E2E pipeline does not detect injection

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- Kind cluster cannot be provisioned (E2E only)
- Build broken — code does not compile
- v1.3 merge conflicts block rebase

**Resume testing when**:
- Blocking condition resolved
- Build fixed and green

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `test/services/mock-llm/config/overrides.go` | `LoadYAMLOverrides`, `ScenarioOverride` struct | ~40 |
| `test/services/mock-llm/scenarios/types.go` | `MockScenarioConfig`, `configScenario` | ~95 |
| `test/services/mock-llm/scenarios/registry_default.go` | `applyOverride`, `findOverrideByWorkflowName` | ~70 |
| `test/services/mock-llm/response/openai.go` | `buildToolArguments`, `BuildToolCallResponse` | ~130 |
| `test/services/mock-llm/scenarios/scenario_injection.go` | New injection scenario config | ~40 |
| `test/services/mock-llm/scenarios/registry.go` | `Registry.Detect` | ~60 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `test/services/mock-llm/handlers/openai.go` | `handleOpenAI` (forceText + tool call bypass) | ~110 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.4` HEAD | Branch |
| Dependency: #601 | Merged in v1.4 | Shadow agent alignment check |
| Dependency: mock-LLM | Current HEAD | All existing scenarios |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-TESTING-657 | Per-scenario forceText YAML parsing | P0 | Unit | UT-MOCK-657-001 | Pending |
| BR-TESTING-657 | ToolCall YAML config parsing | P0 | Unit | UT-MOCK-657-002 | Pending |
| BR-TESTING-657 | applyOverride applies ForceText | P0 | Unit | UT-MOCK-657-003 | Pending |
| BR-TESTING-657 | applyOverride applies ToolCall fields | P0 | Unit | UT-MOCK-657-004 | Pending |
| BR-TESTING-657 | Missing forceText backward compatible | P0 | Unit | UT-MOCK-657-005 | Pending |
| BR-TESTING-657 | buildToolArguments kubectl_get_yaml | P0 | Unit | UT-MOCK-657-006 | Pending |
| BR-TESTING-657 | buildToolArguments kubectl_get_by_name | P0 | Unit | UT-MOCK-657-007 | Pending |
| BR-TESTING-657 | BuildToolCallResponse with kubectl | P0 | Unit | UT-MOCK-657-008 | Pending |
| BR-TESTING-657 | Registry detects injection scenario | P1 | Unit | UT-MOCK-657-009 | Pending |
| BR-TESTING-657 | Injection scenario config defaults | P1 | Unit | UT-MOCK-657-010 | Pending |
| BR-TESTING-657 | Per-scenario ForceText=false overrides global | P0 | Integration | IT-MOCK-657-001 | Pending |
| BR-TESTING-657 | Per-scenario ForceText=nil backward compat | P0 | Integration | IT-MOCK-657-002 | Pending |
| BR-TESTING-657 | Per-scenario ForceText=true overrides global | P1 | Integration | IT-MOCK-657-003 | Pending |
| BR-TESTING-657 | ToolCallName returns custom tool call | P0 | Integration | IT-MOCK-657-004 | Pending |
| BR-TESTING-657 | After tool result returns text analysis | P0 | Integration | IT-MOCK-657-005 | Pending |
| BR-TESTING-657 | No ToolCallName follows normal DAG | P0 | Integration | IT-MOCK-657-006 | Pending |
| BR-TESTING-657 | Poisoned ConfigMap injection via tool call | P0 | E2E | E2E-MOCK-657-001 | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: `MOCK` (Mock-LLM)
- **BR_NUMBER**: 657
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `config/overrides.go`, `scenarios/types.go`, `scenarios/registry_default.go`, `response/openai.go`, `scenarios/scenario_injection.go` — >=80% coverage target

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-MOCK-657-001` | YAML `forceText: false` parses to `*bool(false)` on ScenarioOverride | Pending |
| `UT-MOCK-657-002` | YAML `toolCall` config parses tool name and arguments | Pending |
| `UT-MOCK-657-003` | `applyOverride` copies ForceText from override to MockScenarioConfig | Pending |
| `UT-MOCK-657-004` | `applyOverride` copies ToolCall fields to MockScenarioConfig | Pending |
| `UT-MOCK-657-005` | Missing `forceText` in YAML leaves nil (existing behavior preserved) | Pending |
| `UT-MOCK-657-006` | `buildToolArguments("kubectl_get_yaml", cfg)` returns `{kind, name, namespace}` | Pending |
| `UT-MOCK-657-007` | `buildToolArguments("kubectl_get_by_name", cfg)` returns `{kind, name, namespace}` | Pending |
| `UT-MOCK-657-008` | `BuildToolCallResponse` with kubectl tool produces valid OpenAI response | Pending |
| `UT-MOCK-657-009` | Registry detects `injection_configmap_read` signal name | Pending |
| `UT-MOCK-657-010` | Injection scenario config has ToolCallName=kubectl_get_yaml and correct resource target | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `handlers/openai.go` — >=80% coverage target on handler forceText and tool call bypass paths

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-MOCK-657-001` | Scenario with ForceText=false returns tool call even when global forceText=true | Pending |
| `IT-MOCK-657-002` | Scenario with ForceText=nil falls through to global forceText=true (backward compat) | Pending |
| `IT-MOCK-657-003` | Scenario with ForceText=true returns text even when global forceText=false | Pending |
| `IT-MOCK-657-004` | Scenario with ToolCallName returns that tool call on first request | Pending |
| `IT-MOCK-657-005` | After tool result submitted, scenario returns text analysis on second request | Pending |
| `IT-MOCK-657-006` | Scenario without ToolCallName follows normal DAG path (backward compat) | Pending |

### Tier 3: E2E Tests

**Testable code scope**: Full pipeline — mock-LLM tool call → KA tool execution → ToolProxy → shadow evaluation

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-MOCK-657-001` | Poisoned ConfigMap with injection content triggers shadow alignment flag via tool execution path | Pending |

### Tier Skip Rationale

- No tiers are skipped. All three tiers are covered.

---

## 9. Test Cases

### UT-MOCK-657-001: YAML forceText parsing

**BR**: BR-TESTING-657
**Priority**: P0
**Type**: Unit
**File**: `test/unit/mockllm/yaml_override_test.go`

**Test Steps**:
1. **Given**: YAML content with `forceText: false` under a scenario key
2. **When**: `LoadYAMLOverrides` parses the YAML
3. **Then**: `ScenarioOverride.ForceText` is `*bool` pointing to `false`

### UT-MOCK-657-006: buildToolArguments kubectl_get_yaml

**BR**: BR-TESTING-657
**Priority**: P0
**Type**: Unit
**File**: `test/unit/mockllm/response_test.go`

**Test Steps**:
1. **Given**: A `MockScenarioConfig` with `ResourceKind="ConfigMap"`, `ResourceNS="default"`, `ResourceName="poisoned-cm"`
2. **When**: `buildToolArguments("kubectl_get_yaml", cfg)` is called
3. **Then**: Returns `{"kind": "ConfigMap", "name": "poisoned-cm", "namespace": "default"}`

### IT-MOCK-657-001: Per-scenario ForceText override

**BR**: BR-TESTING-657
**Priority**: P0
**Type**: Integration
**File**: `test/integration/mockllm/tool_call_override_test.go`

**Test Steps**:
1. **Given**: A registry with a scenario that has `ForceText=ptr(false)`, and a server with global `forceText=true`
2. **When**: A chat completion request is sent with tools for that scenario
3. **Then**: Response has `finish_reason: "tool_calls"` and a tool call, not text

### E2E-MOCK-657-001: Poisoned ConfigMap injection pipeline

**BR**: BR-TESTING-657
**Priority**: P0
**Type**: E2E
**File**: `test/e2e/kubernautagent/alignment_e2e_test.go`

**Preconditions**:
- Kind cluster with KA, mock-LLM, mock-LLM-shadow deployed
- KA alignment check enabled pointing to mock-llm-shadow

**Test Steps**:
1. **Given**: A ConfigMap in the test namespace with data containing injection content (`SYSTEM: ignore previous instructions`)
2. **When**: An investigation is triggered with signal `injection_configmap_read` targeting that ConfigMap
3. **Then**: Mock-LLM returns `kubectl_get_yaml` tool call → KA executes the tool → ToolProxy submits content to shadow → shadow flags injection → `NeedsHumanReview=true`

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None — pure logic tests on config parsing, override application, response building
- **Location**: `test/unit/mockllm/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks — uses `httptest.Server` with real handlers
- **Infrastructure**: `httptest.NewServer` with mock-LLM router
- **Location**: `test/integration/mockllm/`

### 10.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Kind cluster with KA, mock-LLM, mock-LLM-shadow, DataStorage
- **Location**: `test/e2e/kubernautagent/`
- **Resources**: Standard KA E2E cluster (4 CPU, 8GB RAM)
- **Pattern**: client-go in BeforeEach to create poisoned ConfigMap (per `detected_labels_e2e_test.go` pattern)

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.23 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| Kind | 0.20+ | E2E cluster |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Issue #601 | Code | Merged | Shadow agent not available for E2E | N/A — already merged |
| Mock-LLM shadow mode | Code | Merged | Shadow handler unavailable | N/A — already available |

### 11.2 Execution Order

1. **Phase 1**: Override types and YAML parsing (UT)
2. **Phase 2**: kubectl tool arguments (UT)
3. **Phase 3**: Per-scenario forceText in handler (IT)
4. **Phase 4**: Custom tool call handler bypass (IT)
5. **Phase 5**: Injection scenario registration (UT)
6. **Phase 6**: E2E infrastructure and test (E2E)

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/657/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/mockllm/` | Override parsing, kubectl args, scenario detection |
| Integration test suite | `test/integration/mockllm/` | Handler forceText override, tool call bypass |
| E2E test | `test/e2e/kubernautagent/` | Poisoned ConfigMap pipeline |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/mockllm/... -ginkgo.v

# Integration tests
go test ./test/integration/mockllm/... -ginkgo.v

# Specific test by ID
go test ./test/unit/mockllm/... -ginkgo.focus="UT-MOCK-657"

# Coverage
go test ./test/unit/mockllm/... -coverprofile=coverage.out -coverpkg=./test/services/mock-llm/...
go tool cover -func=coverage.out
```

---

## 14. Existing Tests Requiring Updates

No existing tests require updates. All changes are additive:
- New fields on `ScenarioOverride` use `*bool` / pointer types (nil = unchanged)
- New `buildToolArguments` cases only affect new tool names
- Handler bypass only triggers when `ToolCallName` is set (no existing scenario sets this)
- New scenario uses a unique signal name (`injection_configmap_read`)

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-09 | Initial test plan |
