# Test Plan: Provider-Neutral Mock LLM Conversation Planner (#2442)

> **Template Version**: 2.0 - Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2442-v1.0
**Feature**: Replace duplicated OpenAI/Gemini discovery routing with a provider-neutral Mock LLM planner
**Version**: 1.0
**Created**: 2026-09-21
**Author**: AI Assistant + Jordi Gil
**Status**: Fullpipeline fixture follow-up implemented; E2E verification pending CI
**Branch**: `fix/2442-workflow-discovery-membership`

---

## 1. Introduction

### 1.1 Purpose

This plan validates the Mock LLM redesign proposed by DD-TEST-018. It proves that
OpenAI and Gemini requests use one semantic DD-KA-017 discovery contract while retaining
their provider-specific wire formats. The plan specifically prevents a workflow from
being requested or submitted unless it was returned by `list_workflows` in the current
selection context.

### 1.2 Objectives

1. Prove the planner enforces discovery membership across pagination and self-correction.
2. Prove OpenAI and Gemini adapters produce equivalent semantic actions and correct wire responses.
3. Prove no unrelated tool result, prior request, selector, or scenario leaks into discovery state.
4. Prove global `force_text`, explicit per-scenario overrides, legacy flows, and custom chains retain their contracts.
5. Prove E2E configuration binds the intended workflow/environment UUID deterministically.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|---|---|---|
| P0 unit pass rate | 100% | `go test ./test/services/mock-llm` |
| P0 integration pass rate | 100% | `go test ./test/integration/mockllm/...` |
| Discovery business logic coverage | 100% of planner branches | Go coverage plus explicit scenario matrix |
| Wiring coverage | 100% of manifest rows | Section 14 integration tests |
| Existing Mock LLM regressions | 0 | Existing service and integration suites |
| Invalid membership actions | 0 | Planner and HTTP assertions that no invalid call is emitted |

---

## 2. References

### 2.1 Authority

- [DD-TEST-018: Provider-Neutral Mock LLM Conversation Planner](../../architecture/decisions/DD-TEST-018-provider-neutral-mock-llm-conversation-planner.md)
- [DD-KA-017: Three-Step Workflow Discovery Integration](../../architecture/decisions/DD-KA-017-three-step-workflow-discovery-integration.md)
- [DD-TEST-016: Explicit Transcript Scenarios](../../architecture/decisions/DD-TEST-016-a2a-transcript-scenario-harness.md)
- [DD-TEST-017: Structured Scenario Selectors](../../architecture/decisions/DD-TEST-017-structured-mock-llm-scenario-selectors.md)
- [BR-MOCK-010: Conversation Engine](../../services/test-infrastructure/mock-llm/BUSINESS_REQUIREMENTS.md)
- [BR-MOCK-012: Three-Step Discovery](../../services/test-infrastructure/mock-llm/BUSINESS_REQUIREMENTS.md)
- [BR-MOCK-014: Conversation Context Tracking](../../services/test-infrastructure/mock-llm/BUSINESS_REQUIREMENTS.md)
- [BR-KA-OBSERVABILITY-001: Kubernaut Agent observability](../../requirements/BR-KA-OBSERVABILITY-001-agent-prometheus-metrics.md)
- Issue #2442

### 2.2 Cross-References

- [Test Plan Template](../TEST_PLAN_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Mock LLM README](../../services/test-infrastructure/mock-llm/README.md)

---

## 3. Risks and Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|---|---|---|---|---|---|
| R1 | Planner emits `get_workflow` without membership | Invalid workflow selection and CI false success/failure | High | UT-MOCK-2442-001..006, IT-MOCK-2442-011..014 | Test target absent, exhausted pages, and membership-only transitions for both providers |
| R2 | OpenAI and Gemini adapters diverge semantically | Provider-dependent production behavior | High | UT-MOCK-2442-007..010, IT-MOCK-2442-013..014 | Run a shared table of canonical transcripts through both adapters |
| R3 | Parallel non-discovery results corrupt discovery state | Premature completion or wrong workflow call | High | UT-MOCK-2442-005, IT-MOCK-2442-008, IT-MOCK-2442-015 | Associate results by tool identity and ignore unrelated tools |
| R4 | Prior request state leaks into a later request | Cross-test and cross-tenant contamination | High | UT-MOCK-2442-009, IT-MOCK-2442-017 | Fresh transcript-derived context and concurrent request tests |
| R5 | Global `force_text` suppresses required discovery | Workflow membership is never established | High | UT-MOCK-2442-011, IT-MOCK-2442-015, IT-MOCK-2442-016 | Test global default, explicit true override, and advertised DD-KA-017 tools |
| R6 | Workflow/environment UUID binding selects the wrong candidate | `get_workflow` or submission uses a catalog-invalid ID | Medium | UT-MOCK-2442-018, IT-MOCK-2442-016, E2E-MOCK-2442-001 | Exact binding, deterministic fallback, and ambiguity rejection |
| R7 | Legacy scenarios regress during migration | Broad Mock LLM and E2E failures | Medium | IT-MOCK-2442-019, existing suite | Retain legacy DAG paths and run the full affected suite |
| R8 | Future provider is coupled to OpenAI types | Costly Anthropic or other adapter addition | Low | UT-MOCK-2442-007, design review | Canonical planner package has no provider response imports |
| R9 | Mock-selected workflow is excluded by the real signal-context catalog filters | AIAnalysis becomes unresolved/Failed and downstream workflow tests time out | High | E2E-FP-118-001, E2E-FP-1542-001, E2E-MOCK-2442-001 | Assert emitted severity/component/environment/priority against seeded workflow labels before selection |
| R10 | A2A RR-creation fixtures lack correlating Prometheus evidence | Severity triage fails before RR creation; chained calls receive no RR ID | High | E2E-FP-1853-001/002 and affected A2A FP cases | Seed and await a matching Prometheus alert/rule before RR-creating tools; retain negative fail-closed coverage |

### 3.1 Risk-to-Test Traceability

All High risks have P0 unit and integration coverage. E2E-MOCK-2442-001 is the release
confidence test for R1, R5, and R6. Any unmitigated High risk blocks implementation
completion.

### 3.2 Security and Observability Control Mapping

| Business behavior | Control objective | Test evidence | Boundary |
|---|---|---|---|
| A discovery adapter must not request a tool the caller did not advertise, and must return an unresolved response instead of bypassing the tool contract. | FedRAMP AC-4 (information-flow enforcement), AC-6 (least privilege); OWASP ASVS 4.0.3 V4.1.3 (least privilege) and V4.1.5 (fail securely); BR-MOCK-012 | UT-MOCK-2442-027, IT-MOCK-2442-027, IT-MOCK-2442-028 | Proves the mock provider boundary. Production authorization and audit persistence remain covered by their owning service suites. |
| Streamed multi-tool and chained responses preserve provider-reported token usage, so downstream monitoring does not undercount LLM consumption. | FedRAMP AU-3 (content of records), SOC2 CC7.2 (monitoring/investigation evidence); BR-KA-OBSERVABILITY-001, issue #2387 | UT-MOCK-2387-005, UT-MOCK-2387-006 | Proves mock wire-level data fidelity. Durable KA audit storage and end-to-end reconstruction are not replaced by these tests. |

---

## 4. Scope

### 4.1 Features to Be Tested

- `test/services/mock-llm/conversation/`: canonical transcript and discovery planner.
- `test/services/mock-llm/handlers/openai.go`: OpenAI adapter wiring and renderer.
- `test/services/mock-llm/handlers/gemini.go`: Gemini adapter wiring and renderer.
- `test/services/mock-llm/scenarios/`: workflow target and selector configuration behavior.
- `test/services/mock-llm/config/`: per-scenario mode and override behavior.
- `test/infrastructure/shared_e2e.go`: E2E YAML generation and deterministic workflow bindings.
- `test/integration/aianalysis/test_workflows.go`: integration config generation compatibility.
- OpenAI and Gemini HTTP routes through the real Mock LLM router.

### 4.2 Features Not to Be Tested

- Anthropic `/v1/messages`: production support exists, but no Mock LLM endpoint is added by #2442.
- Ollama discovery: Ollama remains text-only and has no three-step tool protocol.
- Production KA workflow-catalog implementation: covered by DD-KA-017 tests and existing KA suites.
- A complete rewrite of legacy DAGs, replay scenarios, or all custom tool chains.
- Independent PostgreSQL, Redis, and API audit failures observed in CI run 35669113733.

### 4.3 Design Decisions

| Decision | Rationale |
|---|---|
| One semantic planner with thin provider adapters | Prevents OpenAI/Gemini discovery divergence while preserving wire contracts |
| Transcript-derived state only | Avoids unreliable server-side session identity and state leakage |
| Membership is established only by `list_workflows` | Matches DD-KA-017 v2.1 and prevents catalog-valid but undiscovered selections |
| Explicit `force_text: true` overrides discovery | Preserves intentional scenario behavior while allowing the global default to support advertised discovery tools |
| Anthropic deferred | Avoids scope expansion without an active test path; planner remains provider-neutral |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: 100% of discovery planner and adapter normalization branches.
- **Integration**: 100% of wiring manifest rows and all provider endpoints.
- **E2E**: At least one real Kind journey proving discovery membership and workflow UUID fidelity.

### 5.2 Two-Tier Minimum

Every in-scope business requirement receives unit coverage for semantic behavior and
integration coverage through the actual HTTP handler/router path. E2E validates the
container and seeded catalog boundary.

### 5.3 Business Outcome Quality Bar

Tests assert observable actions and outcomes, not only internal state:

- The expected tool name and arguments are emitted.
- Invalid `get_workflow` and workflow submissions are not emitted.
- The selected workflow UUID matches the discovered catalog UUID.
- OpenAI and Gemini clients receive valid provider-specific responses.

### 5.4 Pass/Fail Criteria

**PASS** requires:

1. All P0 tests pass.
2. All wiring manifest rows pass through real handler/router entry points.
3. No invalid membership action is emitted in any provider or pagination case.
4. Existing Mock LLM and affected integration tests have no regressions.
5. E2E-MOCK-2442-001 passes with a seeded workflow UUID.

**FAIL** occurs on any P0 failure, any regression, any missing wiring proof, or any
observed `get_workflow`/workflow submission without list membership.

### 5.5 Suspension and Resumption

Suspend when the Mock LLM package does not compile, the required test container cannot
start, or more than three failures share one unresolved infrastructure cause. Resume
after the cause is identified and the affected test can run deterministically.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File or package | Functions/behavior |
|---|---|
| `test/services/mock-llm/conversation/` | Canonical transcript extraction, discovery state, planner transitions |
| `test/services/mock-llm/response/` | Provider-neutral action rendering inputs and tool result parsing helpers |
| `test/services/mock-llm/scenarios/` | Workflow target resolution and ambiguity behavior |
| `test/services/mock-llm/config/` | Override validation and effective mode policy inputs |

### 6.2 Integration-Testable Code

| File or package | Entry point |
|---|---|
| `test/services/mock-llm/handlers/openai.go` | OpenAI chat completion handler |
| `test/services/mock-llm/handlers/gemini.go` | Gemini `generateContent` handler |
| `test/services/mock-llm/handlers/router.go` | Provider route registration |
| `test/infrastructure/shared_e2e.go` | Kind deployment configuration generation |
| `test/integration/aianalysis/test_workflows.go` | Config file generation |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|---|---|---|
| Code under test | `946d117d9` plus implementation commits | Branch `fix/2442-workflow-discovery-membership` |
| CI evidence | Run `35669113733` | Failure evidence for Issue #2442 |
| Design | DD-TEST-018 v1.0 | Proposed on 2026-09-21 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|---|---|---|---|---|---|
| BR-MOCK-010 | Typed discovery state replaces count-only discovery routing | P0 | Unit | UT-MOCK-2442-001..005, UT-MOCK-2442-007 | Implemented |
| BR-MOCK-012 | Three-step discovery enforces workflow membership | P0 | Unit | UT-MOCK-2442-001..005 | Implemented |
| BR-MOCK-012 | OpenAI path is wired to the planner | P0 | Integration | IT-MOCK-2442-011, IT-MOCK-2442-013, IT-MOCK-2442-021..023 | Implemented |
| BR-MOCK-012 | Gemini path is wired to the planner | P0 | Integration | IT-MOCK-2442-012, IT-MOCK-2442-014 | Implemented |
| BR-MOCK-012 | OpenAI/Gemini reject discovery actions outside the advertised tool set | P0 | Unit + Integration | UT-MOCK-2442-027, IT-MOCK-2442-027..028 | Implemented |
| BR-MOCK-012 | Seeded E2E workflow is discovered before selection | P0 | E2E | E2E-MOCK-2442-001 | Pending: environment not run |
| BR-MOCK-014 | Request state is transcript-derived and isolated | P0 | Unit | UT-MOCK-2442-005, UT-MOCK-2442-007 | Implemented |
| BR-MOCK-014 | Concurrent provider requests do not leak state | P0 | Integration | Existing integration coverage | Implemented |
| BR-TESTING-001 | Scenario and environment overrides are deterministic | P0 | Unit | UT-MOCK-2442-018 | Implemented |
| BR-TESTING-001 | E2E configuration wires deterministic overrides | P0 | Integration | Existing config-generator coverage | Implemented |
| BR-TESTING-001 | A2A grounding rule fixtures are deterministic and target-scoped | P0 | Unit | UT-FP-2443-001/002 | Implemented; passed locally |
| BR-KA-017-003 / BR-WORKFLOW-004 | Filtered discovery returns the expected workflow only when its labels match signal context | P0 | E2E | E2E-FP-118-001, E2E-FP-1542-001 | Implemented; E2E pending |
| BR-SEVERITY-001 / BR-INTERACTIVE-010 | A2A RR creation proceeds with correlated alert/rule evidence and remains fail-closed without it | P0 | E2E | E2E-FP-1853-001/002, E2E-FP-1918-001 | Implemented; E2E pending |
| BR-MOCK-010 | Legacy and custom non-discovery flows remain compatible | P0 | Integration | Existing Mock LLM suite | Implemented |
| BR-KA-OBSERVABILITY-001 | Streamed multi-tool and chained responses preserve exact scripted usage | P1 | Unit | UT-MOCK-2387-005..006 | Implemented |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business outcome | Phase |
|---|---|---|
| UT-MOCK-2442-001 | Initial DD-KA-017 request calls the first discovery tool | Implemented |
| UT-MOCK-2442-002 | Listed target permits `get_workflow` | Implemented |
| UT-MOCK-2442-003 | Unlisted target with a next cursor requests another page | Implemented |
| UT-MOCK-2442-004 | Unlisted target without a cursor becomes unresolved | Implemented |
| UT-MOCK-2442-005 | `get_workflow` never grants membership | Implemented |
| UT-MOCK-2442-006 | Membership accumulates across pages and retries | Covered by UT-MOCK-2442-003 |
| UT-MOCK-2442-007 | OpenAI and Gemini equivalent transcripts yield equal semantic plans | Implemented |
| UT-MOCK-2442-008 | Parallel non-discovery results are ignored by discovery state | Covered by UT-MOCK-2442-005 and IT-MOCK-2442-021 |
| UT-MOCK-2442-009 | Prior-turn results do not satisfy a new selection context | Implemented |
| UT-MOCK-2442-010 | Malformed discovery results fail closed with diagnostics | Covered by UT-MOCK-2442-004 |
| UT-MOCK-2442-015 | Global and per-scenario `force_text` precedence is correct | Implemented |
| UT-MOCK-2442-018 | Ambiguous workflow/environment overrides are rejected | Implemented |
| UT-MOCK-2442-020 | Canonical transcript retains provider message content | Implemented |
| UT-MOCK-2442-027 | An undeclared discovery tool is rejected instead of being requested | Implemented |
| UT-FP-2443-001/002 | A2A grounding rules are deterministic, resource-correlated, and isolated from Gateway ingestion | Implemented; passed locally |

### Tier 2: Integration Tests

| ID | Business outcome | Phase |
|---|---|---|
| IT-MOCK-2442-011 | OpenAI HTTP flow completes valid three-step discovery | Implemented |
| IT-MOCK-2442-012 | Gemini HTTP flow completes valid three-step discovery | Implemented |
| IT-MOCK-2442-013 | OpenAI HTTP flow refuses an undiscovered workflow | Implemented |
| IT-MOCK-2442-014 | Gemini HTTP flow refuses an undiscovered workflow | Implemented |
| IT-MOCK-2442-027 | OpenAI HTTP flow fails closed when `list_workflows` is not advertised | Implemented |
| IT-MOCK-2442-028 | Gemini HTTP flow fails closed when `list_workflows` is not advertised | Implemented |
| IT-MOCK-2442-015 | Global force-text default does not suppress advertised discovery | Implemented |
| IT-MOCK-2442-016 | E2E and integration config generators emit deterministic bindings | Covered by existing tests |
| IT-MOCK-2442-017 | Concurrent OpenAI and Gemini requests remain isolated | Covered by existing tests |
| IT-MOCK-2442-019 | Legacy, replay, custom-chain, and text-only paths remain compatible | Covered by existing suite |
| IT-MOCK-2442-021 | OpenAI completion ignores parallel non-discovery results | Implemented |
| IT-MOCK-2442-022 | OpenAI discovery overrides cannot bypass membership planning | Implemented |
| IT-MOCK-2442-023 | Explicit scenario force-text suppresses unresolved submission | Implemented |
| IT-MOCK-2442-024 | Gemini discovery overrides cannot bypass membership planning | Implemented |
| IT-MOCK-2442-025 | Multi-tool and chained discovery overrides cannot bypass membership planning | Implemented |
| IT-MOCK-2442-026 | Workflow submission overrides cannot bypass membership planning | Implemented |

### Tier 3: E2E Tests

| ID | Business outcome | Phase |
|---|---|---|
| E2E-MOCK-2442-001 | Kind-based AIA/KA journey discovers the seeded workflow before selection | Pending: environment not run |
| E2E-FP-118-001 | Signal-context filters return the OOM workflow expected by the seeded Mock LLM scenario | Implemented; E2E pending |
| E2E-FP-1542-001 | BackOff/CrashLoop context discovers the real ConfigMap-fix workflow and completes the fix | Implemented; E2E pending |
| E2E-FP-1853-001/002 | A2A-created RR uses grounded severity evidence and passes a valid RR ID through the tool chain | Implemented; E2E pending |
| E2E-FP-1918-001, E2E-FP-1899-001/002 | A2A actionability/consent contracts remain covered after RR-creation fixtures are grounded | Implemented; E2E pending |

### Fullpipeline RCA Follow-Up — Run 35823531457

The approved follow-up keeps production discovery-membership and severity gates
fail-closed. Fullpipeline test namespaces and test-only workflow labels must make the
intended workflow eligible under the actual `SignalProcessing` context. A2A specs that
exercise RR creation must provide active, resource-correlated Prometheus evidence; a
Kubernetes Warning Event alone is not severity-triage evidence for these tools.

For each workflow-selection E2E, capture/assert the actual signal filters
(`severity`, `component`, `environment`, `priority`) and prove the expected seeded
workflow UUID appears in `list_workflows` before the mock requests `get_workflow` or
returns a selection. Existing negative membership tests must remain green so the fix
does not bypass the catalog contract.

### Tier Skip Rationale

No tier is skipped. Anthropic is excluded from this plan because the Mock LLM does not
currently expose that provider protocol and no active #2442 path requires it.

---

## 9. P0 Test Case Specifications

### UT-MOCK-2442-004: Exhausted discovery is unresolved

**BR**: BR-MOCK-012
**Priority**: P0
**Type**: Unit
**File**: `test/services/mock-llm/discovery_planner_test.go`

**Test Steps**:

1. Given a transcript containing `list_available_actions` and `list_workflows`.
2. When the workflow list contains no configured target and `hasNext=false`.
3. Then the planner returns `Unresolved`.
4. Then it does not return `get_workflow` or a workflow submission action.

**Acceptance Criteria**:

- The configured target is never treated as discovered merely because a result count was reached.
- The unresolved reason identifies exhausted workflow discovery.

### UT-MOCK-2442-006: Membership accumulates across pages

**BR**: BR-MOCK-012
**Priority**: P0
**Type**: Unit
**File**: `test/services/mock-llm/discovery_planner_test.go`

**Test Steps**:

1. Given page one contains workflow A and a next cursor.
2. When page two contains the configured target workflow B.
3. Then the planner requests page two and then `get_workflow` for B.
4. Then the membership set contains A and B for the current selection context.

**Acceptance Criteria**:

- Pagination does not reset earlier membership.
- The target is selected only after the page containing B is observed.

### UT-MOCK-2442-007: Provider semantic parity

**BR**: BR-MOCK-012
**Priority**: P0
**Type**: Unit
**File**: `test/services/mock-llm/provider_adapter_test.go`

**Test Steps**:

1. Given equivalent OpenAI and Gemini transcripts for the same discovery journey.
2. When each adapter normalizes its request.
3. Then the planner receives equivalent semantic events and returns the same plan.

**Acceptance Criteria**:

- Only the response serialization differs between providers.
- Tool names, membership decisions, pagination, and unresolved behavior are identical.

### UT-MOCK-2442-009: Selection context isolation

**BR**: BR-MOCK-014
**Priority**: P0
**Type**: Unit
**File**: `test/services/mock-llm/discovery_planner_test.go`

**Test Steps**:

1. Given one transcript that completes discovery for `workflow-first`.
2. When the planner is called again with a fresh transcript for `workflow-second`.
3. Then the second selection starts at `list_available_actions`.
4. Then no membership or completion state from the first selection is reused.

**Acceptance Criteria**:

- A completed selection cannot satisfy a later selection with a fresh transcript.
- Discovery state is derived only from the transcript supplied to the current planner call.

### IT-MOCK-2442-013: OpenAI refuses undiscovered workflow

**BR**: BR-MOCK-012
**Priority**: P0
**Type**: Integration
**File**: `test/integration/mockllm/openai_test.go`

**Test Steps**:

1. Send a real OpenAI-format request advertising DD-KA-017 tools.
2. Return a `list_workflows` result without the configured target and without a next page.
3. Send the next request through the real handler/router.
4. Assert no `get_workflow` or `submit_result_with_workflow` call contains the target.

**Acceptance Criteria**:

- The handler returns a deterministic unresolved/no-workflow response.
- The verification API records no invalid discovery action.

### IT-MOCK-2442-014: Gemini refuses undiscovered workflow

**BR**: BR-MOCK-012
**Priority**: P0
**Type**: Integration
**File**: `test/integration/mockllm/gemini_test.go`

**Test Steps**:

1. Send a real Gemini-format request advertising DD-KA-017 functions.
2. Return a `list_workflows` function response without the configured target and without a next page.
3. Send the next request through the real handler/router.
4. Assert no `get_workflow` or workflow submission call contains the target.

**Acceptance Criteria**:

- Gemini has the same semantic result as OpenAI.
- The response remains valid Gemini `generateContent` JSON.

### E2E-MOCK-2442-001: Seeded workflow discovery journey

**BR**: BR-MOCK-012, BR-TESTING-001
**Priority**: P0
**Type**: E2E
**File**: `test/e2e/kubernautagent/three_step_discovery_test.go`

**Test Steps**:

1. Seed a workflow with a deterministic environment-specific UUID.
2. Deploy the real Mock LLM image and generated configuration.
3. Execute the KA/AIA discovery journey in Kind.
4. Verify `list_workflows` returns the seeded UUID before `get_workflow` or selection.
5. Verify the selected UUID equals the seeded UUID.

**Acceptance Criteria**:

- The journey completes without `llm_parsing_error` caused by missing discovery membership.
- Mock LLM verification data shows the required sequence.

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD.
- **Mocks**: No external dependency mocks; use real transcript and planner logic.
- **Location**: `test/services/mock-llm/`.

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD.
- **Mocks**: No mocks of the Mock LLM handler; use `httptest.Server` and real router/handlers.
- **Infrastructure**: In-process Mock LLM server and provider-shaped JSON requests.
- **Location**: `test/integration/mockllm/`.

### 10.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD.
- **Infrastructure**: Kind, real Mock LLM container, seeded workflow catalog, KA/AIA dependencies.
- **Location**: `test/e2e/kubernautagent/`.

---

## 11. Dependencies and Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact | Workaround |
|---|---|---|---|---|
| DD-TEST-018 approval | Design | Approved | No implementation blocker | Shared planner design approved and implemented |
| DD-KA-017 v2.1 | Contract | Available | Defines membership behavior | Use current Go contract |
| CI run 35669113733 artifacts | Evidence | Available locally | Provides regression evidence | Use extracted artifacts; rerun after implementation |
| User worktree changes | Coordination | Present | Files must not be reverted | Rebase implementation around them |

### 11.2 Execution Order

1. **RED**: Add planner, adapter, policy, and configuration tests with real provider-shaped transcripts.
2. **GREEN**: Implement the smallest typed planner and thin OpenAI/Gemini adapters; wire both handlers.
3. **REFACTOR**: Remove discovery-specific count fallback and duplicate provider logic; retain legacy paths.
4. **WIRING VERIFICATION**: Run every manifest integration test through the actual router and configuration entry point.
5. **E2E**: Run the seeded Kind discovery journey and affected suites.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|---|---|---|
| Design decision | `docs/architecture/decisions/DD-TEST-018-provider-neutral-mock-llm-conversation-planner.md` | Alternatives, chosen boundary, constraints |
| This test plan | `docs/testing/2442/TEST_PLAN.md` | Test strategy and traceability |
| Planner unit tests | `test/services/mock-llm/` | Semantic state and adapter tests |
| Provider integration tests | `test/integration/mockllm/` | OpenAI/Gemini HTTP wiring proof |
| E2E validation | `test/e2e/kubernautagent/` | Seeded workflow membership journey |
| Coverage report | CI artifact | Per-tier coverage and result evidence |

---

## 13. Execution Commands

```bash
# Mock LLM unit tests
go test ./test/services/mock-llm

# Mock LLM provider integration tests
go test ./test/integration/mockllm/...

# Affected E2E package after container prerequisites are available
go test ./test/e2e/kubernautagent/...

# Full required validation before completion
go build ./...
golangci-lint run --timeout=5m
make test
```

---

## 14. Wiring Verification

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|---|---|---|---|---|
| Discovery planner | OpenAI `/v1/chat/completions` | OpenAI tool/text response | IT-MOCK-2442-011, IT-MOCK-2442-013, IT-MOCK-2442-021..023, IT-MOCK-2442-025..026 | Implemented |
| Discovery planner | Gemini `/v1beta/models/*:generateContent` | Gemini function/text response | IT-MOCK-2442-012, IT-MOCK-2442-014, IT-MOCK-2442-024 | Implemented |
| OpenAI adapter | OpenAI route registration | Canonical planner action | IT-MOCK-2442-011, IT-MOCK-2442-013 | Implemented |
| Gemini adapter | Gemini route registration | Canonical planner action | IT-MOCK-2442-012, IT-MOCK-2442-014, IT-MOCK-2442-024 | Implemented |
| Mode policy | Handler dispatch | Tool or text response | IT-MOCK-2442-015, IT-MOCK-2442-023 | Implemented |
| E2E config generation | `DeployMockLLMInNamespace` | Running container behavior | IT-MOCK-2442-016, E2E-MOCK-2442-001 | Pending: E2E not run |
| Integration config generation | `WriteMockLLMConfigFile` | Running Mock LLM behavior | IT-MOCK-2442-016 | Covered by existing tests |
| Fullpipeline signal-to-workflow path | Gateway → SignalProcessing → AIAnalysis/KA discovery | WorkflowExecution reaches expected terminal result | E2E-FP-118-001, E2E-FP-1542-001 | Implemented; E2E pending |
| Fullpipeline A2A RR-creation path | API Frontend A2A → severity triage → RemediationRequest | RR is created only after matching Prometheus evidence | E2E-FP-1853-001/002 and affected consent/actionability specs | Implemented; E2E pending |

Unit tests do not count as wiring proof.

---

## 15. Existing Tests Requiring Updates

| Location | Current assertion | Required change | Reason |
|---|---|---|---|
| `test/services/mock-llm/dag_builders_test.go` | Bare tool-result counts imply `get_workflow` | Build provider-shaped discovery transcripts and assert membership | Prevents count-only false confidence |
| `test/services/mock-llm/gemini_response_test.go` | Gemini helpers are tested independently | Add planner parity and unresolved discovery cases | Aligns Gemini with OpenAI semantic contract |
| `test/integration/mockllm/openai_test.go` | Tool sequence assertions | Assert target appears in `list_workflows` before `get_workflow` | Proves DD-KA-017 membership |
| `test/integration/mockllm/gemini_test.go` | Function-call sequence assertions | Assert same membership invariant | Prevents provider divergence |
| `test/e2e/kubernautagent/three_step_discovery_test.go` | Final selection/result only | Assert discovery order and seeded UUID membership | Detects CI failure earlier |
| `test/infrastructure/shared_e2e.go` | Generated scenario bindings | Assert deterministic workflow/environment mapping | Prevents wrong UUID configuration |

---

## 16. Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-09-21 | Initial design-first test plan for DD-TEST-018 |
