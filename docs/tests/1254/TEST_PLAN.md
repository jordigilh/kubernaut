# Test Plan: AF OpenAI-Compatible LLM Backend Support

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1254-v1.0
**Feature**: Enable API Frontend to use OpenAI-compatible LLM endpoints (LlamaStack, vLLM, Ollama)
**Version**: 1.0
**Created**: 2026-06-24
**Author**: AI Agent
**Status**: Draft
**Branch**: `feat/af-openai-llm-1254`

---

## 1. Introduction

### 1.1 Purpose

Validate that the API Frontend (AF) can be configured to use OpenAI-compatible
LLM endpoints — such as LlamaStack, vLLM, and Ollama — for A2A agent
conversations and severity triage. This enables users running self-hosted or
on-premises models to use Kubernaut without a cloud LLM subscription.

### 1.2 Objectives

1. **Provider acceptance**: AF config validation accepts `openai` and
   `openai_compatible` as LLM providers with correct required-field rules
2. **Adapter correctness**: The in-house OpenAI adapter correctly translates
   ADK `model.LLMRequest` to OpenAI chat completions and back
3. **Transport chain integration**: The adapter accepts a custom `*http.Client`,
   enabling the full transport chain (TLS CA, OAuth2, custom headers, circuit
   breaker) — the first AF provider to support this
4. **API key optionality**: The adapter works with and without an API key,
   supporting keyless local deployments (LlamaStack)
5. **Factory wiring**: `NewModelFromConfig` dispatches to the new adapter and
   the resulting `model.LLM` is used by `buildA2AHandler` in production

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/...` |
| Integration test pass rate | 100% | `go test ./test/integration/apifrontend/...` |
| E2E test pass rate | 100% | `go test ./test/e2e/apifrontend/... -ginkgo.focus="1254"` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on adapter + config |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on factory wiring |
| Backward compatibility | 0 regressions | Existing provider tests pass unchanged |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-INTEGRATION-1254**: AF supports OpenAI-compatible LLM backends for
  on-premises and self-hosted model deployments
- **Issue #1254**: AF OpenAI-compatible LLM backend support
- **Issue #1342**: Transport chain not injectable into all providers

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Wiring Verification](../../.cursor/rules/10-wiring-verification.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- [Integration/E2E No-Mocks Policy](../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `openai/openai-go` SDK breaks backward compat on minor release | Adapter compilation failures | Low | All adapter UTs | Pin SDK version in `go.mod` |
| R2 | OpenAI streaming SSE format differs between providers (LlamaStack vs vLLM) | Streaming responses malformed | Medium | UT-AF-1254-024, -025 | Test streaming with representative chunk shapes |
| R3 | Tool call accumulation logic incorrect for partial chunks | Agent tool calls fail silently | Medium | UT-AF-1254-023 | Dedicated test with multi-chunk tool call fragments |
| R4 | API key empty + no env var = silent auth failure at runtime | 401 from provider with no clear error | Low | UT-AF-1254-005 | Config validation warns; adapter passes empty key gracefully |

### 3.1 Risk-to-Test Traceability

- **R1**: Mitigated by pinning SDK version and adapter compilation in CI
- **R2**: Mitigated by UT-AF-1254-024 (streaming text) and UT-AF-1254-025 (streaming tool calls)
- **R3**: Mitigated by UT-AF-1254-023 (multi-chunk tool call accumulation)
- **R4**: Mitigated by UT-AF-1254-005 (keyless config accepted) and IT-AF-1254-002 (keyless wiring)

---

## 4. Scope

### 4.1 Features to be Tested

- **Config validation** (`pkg/apifrontend/config/config.go`): `openai` and
  `openai_compatible` provider constants accepted; `endpoint` + `model` required;
  `apiKeyFile` optional
- **OpenAI adapter** (`pkg/apifrontend/launcher/openai/adapter.go`): Implements
  `model.LLM` interface; converts requests/responses; supports streaming with
  tool call accumulation; accepts custom `*http.Client`
- **Factory wiring** (`pkg/apifrontend/launcher/model.go`): `NewModelFromConfig`
  dispatches to `newOpenAICompatibleModel` for new providers; transport chain
  injected
- **Production integration** (`cmd/apifrontend/main.go`): `buildA2AHandler` and
  `newLLMTriagerFromConfig` create working models with OpenAI config

### 4.2 Features Not to be Tested

- **Existing providers** (vertex_ai, gemini, anthropic): Covered by existing
  test plans; regression verified by unchanged test pass rate
- **KA OpenAI support**: KA already uses langchaingo for OpenAI; this plan
  covers AF only
- **E2E with real LLM endpoint**: E2E uses the existing mock-LLM OpenAI
  handler (`/v1/chat/completions`) in the Kind cluster, not a real LLM

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| In-house adapter over community library | Avoids single-maintainer dependency risk; enables `*http.Client` injection for transport chain (Issue #1342) |
| Uses `openai/openai-go` official SDK | Stable, actively maintained by OpenAI; supports `option.WithHTTPClient` |
| Two provider constants (`openai`, `openai_compatible`) | `openai` for OpenAI API directly; `openai_compatible` for LlamaStack/vLLM/Ollama |
| API key optional for `openai_compatible` | LlamaStack and local Ollama don't require auth |
| `endpoint` required for both | No default base URL; operator must specify their endpoint |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code (adapter conversion logic, config
  validation branches, factory dispatch)
- **Integration**: >=80% of integration-testable code (factory -> adapter ->
  httptest.NewServer round-trip, transport chain injection)
- **E2E**: >=80% of full service code — happy-path A2A conversation through
  AF configured with `openai_compatible` provider using mock-LLM's existing
  OpenAI handler in Kind cluster

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least UT + IT:
- **Unit tests**: Adapter logic (message conversion, streaming, tool calls),
  config validation rules
- **Integration tests**: Factory wiring through production code path, transport
  chain injection, `httptest.NewServer` round-trip

### 5.3 Business Outcome Quality Bar

Tests validate business outcomes — "can the operator configure AF to use their
LlamaStack endpoint and get working AI analysis?" — not just code path coverage.

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier code coverage meets >=80% threshold
4. No regressions in existing LLM provider tests
5. Adapter round-trips a complete chat completion through `httptest.NewServer`

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80%
3. Existing provider tests regress
4. Adapter fails to handle streaming with tool calls

### 5.5 Suspension & Resumption Criteria

**Suspend when**:
- `openai/openai-go` SDK has breaking changes incompatible with `google.golang.org/adk v1.4.0`
- Build broken — code does not compile

**Resume when**:
- SDK version pinned or compatibility resolved
- Build fixed

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/apifrontend/config/config.go` | `validateLLMConfig` (new branches) | ~15 |
| `pkg/apifrontend/launcher/openai/adapter.go` (new) | `NewModel`, `GenerateContent`, `convertRequest`, `convertResponse`, message/tool/schema conversion | ~460 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/apifrontend/launcher/model.go` | `NewModelFromConfig` (new case), `newOpenAICompatibleModel` | ~30 |
| `cmd/apifrontend/main.go` | `buildA2AHandler`, `newLLMTriagerFromConfig` | ~10 (switch addition) |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `feat/af-openai-llm-1254` HEAD | Feature branch |
| `google.golang.org/adk` | v1.4.0 | Existing dependency; `model.LLM` interface |
| `github.com/openai/openai-go` | TBD (latest stable) | New dependency for adapter |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-INTEGRATION-1254 | AF accepts `openai` provider config | P0 | Unit | UT-AF-1254-001 | Pending |
| BR-INTEGRATION-1254 | AF accepts `openai_compatible` provider config | P0 | Unit | UT-AF-1254-002 | Pending |
| BR-INTEGRATION-1254 | AF rejects unknown provider | P0 | Unit | UT-AF-1254-003 | Pending |
| BR-INTEGRATION-1254 | `endpoint` required for OpenAI providers | P0 | Unit | UT-AF-1254-004 | Pending |
| BR-INTEGRATION-1254 | `apiKeyFile` optional for `openai_compatible` | P0 | Unit | UT-AF-1254-005 | Pending |
| BR-INTEGRATION-1254 | `apiKeyFile` required for `openai` | P0 | Unit | UT-AF-1254-006 | Pending |
| BR-INTEGRATION-1254 | Adapter converts user message to OpenAI format | P0 | Unit | UT-AF-1254-020 | Pending |
| BR-INTEGRATION-1254 | Adapter converts system instruction | P0 | Unit | UT-AF-1254-021 | Pending |
| BR-INTEGRATION-1254 | Adapter converts tool declarations | P0 | Unit | UT-AF-1254-022 | Pending |
| BR-INTEGRATION-1254 | Adapter accumulates streaming tool call chunks | P0 | Unit | UT-AF-1254-023 | Pending |
| BR-INTEGRATION-1254 | Adapter streams text responses | P0 | Unit | UT-AF-1254-024 | Pending |
| BR-INTEGRATION-1254 | Adapter handles streaming finish reason | P1 | Unit | UT-AF-1254-025 | Pending |
| BR-INTEGRATION-1254 | Adapter maps OpenAI response to LLMResponse | P0 | Unit | UT-AF-1254-026 | Pending |
| BR-INTEGRATION-1254 | Adapter handles generation config params | P1 | Unit | UT-AF-1254-027 | Pending |
| BR-INTEGRATION-1254 | Adapter handles structured output (response schema) | P1 | Unit | UT-AF-1254-028 | Pending |
| BR-INTEGRATION-1254 | Adapter accepts custom HTTP client | P0 | Unit | UT-AF-1254-029 | Pending |
| BR-INTEGRATION-1254 | Factory dispatches to OpenAI adapter | P0 | Integration | IT-AF-1254-001 | Pending |
| BR-INTEGRATION-1254 | Factory wiring works without API key (keyless) | P0 | Integration | IT-AF-1254-002 | Pending |
| BR-INTEGRATION-1254 | Transport chain injected into adapter | P0 | Integration | IT-AF-1254-003 | Pending |
| BR-INTEGRATION-1254 | Adapter round-trips chat completion via httptest | P0 | Integration | IT-AF-1254-004 | Pending |
| BR-INTEGRATION-1254 | Full A2A journey with openai_compatible in Kind | P0 | E2E | E2E-AF-1254-001 | Pending |

---

## 8. FedRAMP / SOC2 Control Verification

Tests verify **business-level behavior** tied to each control objective.

### SC-8 — Transmission Confidentiality and Integrity

**Business behavior**: When an operator configures TLS for the OpenAI-compatible
endpoint, all LLM traffic is encrypted in transit. The transport chain (TLS CA,
client certs) is injected into the adapter's HTTP client.

**Tests**: UT-AF-1254-029, IT-AF-1254-003

### IA-5 — Authenticator Management

**Business behavior**: API keys for LLM authentication are resolved from mounted
Kubernetes Secrets (file-based), never hardcoded. For `openai_compatible`
providers (LlamaStack), the API key is optional — the system operates without
credentials when the endpoint doesn't require them.

**Tests**: UT-AF-1254-005, UT-AF-1254-006, IT-AF-1254-002

### SI-10 — Information Input Validation

**Business behavior**: AF rejects misconfigured LLM provider settings at startup,
preventing malformed requests from reaching external endpoints. Unknown providers
are rejected. Required fields (`model`, `endpoint`) are enforced.

**Tests**: UT-AF-1254-001 through UT-AF-1254-006

### CM-6 — Configuration Settings

**Business behavior**: The system enforces organization-defined configuration
settings for LLM providers. Only validated provider values are accepted.
Endpoint URLs must be well-formed. The configuration schema is consistent
with existing providers (vertex_ai, gemini, anthropic).

**Tests**: UT-AF-1254-001 through UT-AF-1254-006, IT-AF-1254-001

### AC-4 — Information Flow Enforcement

**Business behavior**: Data flows correctly from the AF A2A handler through the
adapter to the configured OpenAI-compatible endpoint. The adapter preserves
message content, tool declarations, and generation parameters without data loss
or corruption.

**Tests**: UT-AF-1254-020 through UT-AF-1254-028, IT-AF-1254-004

---

## 9. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-AF-1254-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **AF**: API Frontend service
- **1254**: Issue number
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `pkg/apifrontend/config/config.go` (validation
branches), `pkg/apifrontend/launcher/openai/adapter.go` (all conversion logic)

#### Config Validation (SI-10, CM-6)

| ID | Business Outcome Under Test | FedRAMP | Phase |
|----|----------------------------|---------|-------|
| UT-AF-1254-001 [SI-10, CM-6] | Operator configuring `provider: openai` with valid model+endpoint passes validation — AF starts successfully | SI-10 | Pending |
| UT-AF-1254-002 [SI-10, CM-6] | Operator configuring `provider: openai_compatible` with valid model+endpoint passes validation — AF starts successfully | SI-10 | Pending |
| UT-AF-1254-003 [SI-10] | Operator configuring unknown provider is rejected at startup — prevents misconfigured LLM from reaching production | SI-10 | Pending |
| UT-AF-1254-004 [SI-10, CM-6] | Operator omitting `endpoint` for OpenAI provider is rejected — prevents requests to undefined URL | SI-10 | Pending |
| UT-AF-1254-005 [IA-5, CM-6] | Operator omitting `apiKeyFile` for `openai_compatible` passes validation — supports keyless LlamaStack deployments | IA-5 | Pending |
| UT-AF-1254-006 [IA-5, SI-10] | Operator omitting `apiKeyFile` for `openai` (not `openai_compatible`) is rejected — OpenAI API requires authentication | IA-5 | Pending |

#### Adapter Message Conversion (AC-4)

| ID | Business Outcome Under Test | FedRAMP | Phase |
|----|----------------------------|---------|-------|
| UT-AF-1254-020 [AC-4] | User message content is faithfully transmitted to the LLM — no data loss in the request path | AC-4 | Pending |
| UT-AF-1254-021 [AC-4] | System instruction is transmitted as OpenAI system message — agent personality/guardrails are enforced | AC-4 | Pending |
| UT-AF-1254-022 [AC-4] | Tool declarations are transmitted as OpenAI function tools — agent can invoke Kubernaut tools | AC-4 | Pending |
| UT-AF-1254-023 [AC-4] | Multi-chunk streaming tool call fragments are accumulated into a complete tool call — agent tool invocations are not corrupted | AC-4 | Pending |
| UT-AF-1254-024 [AC-4] | Streaming text responses are yielded as partial LLMResponse events — user sees real-time token output | AC-4 | Pending |
| UT-AF-1254-025 [AC-4] | Streaming finish reason is mapped to genai.FinishReason — runner correctly detects turn completion | AC-4 | Pending |
| UT-AF-1254-026 [AC-4] | Non-streaming response content and usage metadata are mapped to LLMResponse — runner processes complete turn | AC-4 | Pending |
| UT-AF-1254-027 [AC-4] | Generation config (temperature, topP, maxTokens, stop sequences) is forwarded to OpenAI params — operator tuning takes effect | AC-4 | Pending |
| UT-AF-1254-028 [AC-4] | Response schema is forwarded as OpenAI JSON schema response format — structured output works | AC-4 | Pending |

#### Adapter Transport (SC-8)

| ID | Business Outcome Under Test | FedRAMP | Phase |
|----|----------------------------|---------|-------|
| UT-AF-1254-029 [SC-8] | Custom HTTP client is used for all requests — TLS CA, OAuth2, custom headers, circuit breaker are applied | SC-8 | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `pkg/apifrontend/launcher/model.go` (factory dispatch),
transport chain injection, `httptest.NewServer` round-trip

| ID | Business Outcome Under Test | FedRAMP | Phase |
|----|----------------------------|---------|-------|
| IT-AF-1254-001 [CM-6] | `NewModelFromConfig` with `provider: openai_compatible` returns a working `model.LLM` — factory dispatch is wired | CM-6 | Pending |
| IT-AF-1254-002 [IA-5] | `NewModelFromConfig` with `openai_compatible` and no API key returns a working model — keyless LlamaStack works end-to-end | IA-5 | Pending |
| IT-AF-1254-003 [SC-8] | `NewModelFromConfig` with TLS config injects transport chain into adapter HTTP client — encrypted LLM traffic | SC-8 | Pending |
| IT-AF-1254-004 [AC-4] | Adapter round-trips a chat completion request through `httptest.NewServer` — OpenAI API contract is honored end-to-end | AC-4 | Pending |

### Tier 3: E2E Tests

**Testable code scope**: Full A2A journey through AF configured with
`openai_compatible` provider, using mock-LLM's existing OpenAI handler
(`/v1/chat/completions`) in Kind cluster

| ID | Business Outcome Under Test | FedRAMP | Phase |
|----|----------------------------|---------|-------|
| E2E-AF-1254-001 [AC-4, CM-6] | Operator configures AF with `provider: openai_compatible` pointing at mock-LLM; user sends A2A message; AF routes through in-house adapter to mock-LLM OpenAI endpoint; mock-LLM returns response with tool calls; A2A conversation completes successfully end-to-end | AC-4, CM-6 | Pending |

---

## 10. Test Cases

### UT-AF-1254-001: openai provider accepted

**BR**: BR-INTEGRATION-1254
**Priority**: P0
**Type**: Unit
**File**: `pkg/apifrontend/config/config_test.go`
**FedRAMP**: SI-10, CM-6

**Test Steps**:
1. **Given**: LLMConfig with `provider: "openai"`, `model: "gpt-4o"`,
   `endpoint: "https://api.openai.com/v1"`, `apiKeyFile` set to a valid path
2. **When**: `validateLLMConfig` is called
3. **Then**: No error returned

**Acceptance Criteria**:
- **Behavior**: Validation passes for the `openai` provider
- **Correctness**: No error; config is usable by the factory

### UT-AF-1254-005: apiKeyFile optional for openai_compatible

**BR**: BR-INTEGRATION-1254
**Priority**: P0
**Type**: Unit
**File**: `pkg/apifrontend/config/config_test.go`
**FedRAMP**: IA-5, CM-6

**Test Steps**:
1. **Given**: LLMConfig with `provider: "openai_compatible"`,
   `model: "llama3.1"`, `endpoint: "http://llamastack:8080/v1"`,
   `apiKeyFile: ""` (empty)
2. **When**: `validateLLMConfig` is called
3. **Then**: No error returned — keyless deployment is valid

**Acceptance Criteria**:
- **Behavior**: Validation passes without API key for `openai_compatible`
- **Correctness**: LlamaStack users are not blocked by a mandatory API key check

### IT-AF-1254-004: adapter round-trip via httptest

**BR**: BR-INTEGRATION-1254
**Priority**: P0
**Type**: Integration
**File**: `test/integration/apifrontend/openai_adapter_test.go`
**FedRAMP**: AC-4

**Test Steps**:
1. **Given**: `httptest.NewServer` serving a valid OpenAI chat completion
   response with tool calls
2. **When**: `NewModelFromConfig` creates adapter with the httptest URL as
   endpoint; `GenerateContent` is called with a user message + tool declaration
3. **Then**: Adapter sends correct OpenAI request format; response is correctly
   mapped to `model.LLMResponse` with tool call parts

**Acceptance Criteria**:
- **Behavior**: Full round-trip through production code path
- **Correctness**: Request body matches OpenAI API contract; response content
  and tool calls are correctly mapped to genai types
- **Accuracy**: Usage metadata (token counts) preserved in response

### E2E-AF-1254-001: A2A conversation with openai_compatible provider

**BR**: BR-INTEGRATION-1254
**Priority**: P0
**Type**: E2E
**File**: `test/e2e/apifrontend/openai_provider_test.go`
**FedRAMP**: AC-4, CM-6

**Test Steps**:
1. **Given**: AF deployed in Kind cluster with `provider: openai_compatible`,
   `endpoint: http://mock-llm:18080/v1`, `model: gpt-4o`; mock-LLM already
   deployed with OpenAI handler
2. **When**: User sends A2A `message/send` with text prompt triggering a known
   mock-LLM keyword scenario
3. **Then**: AF routes through in-house adapter to mock-LLM's OpenAI endpoint;
   response contains expected investigation analysis text

**Acceptance Criteria**:
- **Behavior**: A2A conversation completes (HTTP 200, valid JSON-RPC response)
- **Correctness**: Response contains expected text from mock-LLM scenario
- **Accuracy**: No 501 ("unsupported provider"), no 500, no connection errors

**Dependencies**: mock-LLM deployed in Kind cluster (existing E2E infrastructure)

---

## 11. Environmental Needs

### 11.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `httptest.NewServer` for OpenAI chat completions endpoint
- **Location**: `pkg/apifrontend/config/config_test.go`,
  `pkg/apifrontend/launcher/openai/adapter_test.go`

### 11.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: `httptest.NewServer` as mock OpenAI endpoint; real
  `NewModelFromConfig` factory; real transport chain
- **Location**: `test/integration/apifrontend/openai_adapter_test.go`

### 11.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Kind cluster with AF + mock-LLM deployed; AF ConfigMap
  updated with `openai_compatible` provider pointing to mock-LLM's OpenAI
  endpoint
- **Location**: `test/e2e/apifrontend/openai_provider_test.go`

### 11.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.23 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| `openai/openai-go` | latest stable | OpenAI SDK dependency |

---

## 12. Dependencies & Schedule

### 12.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| `openai/openai-go` SDK | External | Available | Adapter cannot compile | N/A |
| `google.golang.org/adk` v1.4.0 | Existing | In `go.mod` | `model.LLM` interface | N/A |

### 12.2 Execution Order

1. **Phase 1 (RED — Config)**: Unit tests for config validation (UT-AF-1254-001
   through -006)
2. **Phase 2 (RED — Adapter)**: Unit tests for adapter conversion logic
   (UT-AF-1254-020 through -029)
3. **Phase 3 (RED — Wiring)**: Integration tests for factory dispatch and
   round-trip (IT-AF-1254-001 through -004)
4. **Phase 4 (RED — E2E)**: E2E test for full A2A journey
   (E2E-AF-1254-001)
5. **Phase 5 (GREEN — Config)**: Add provider constants and update
   `validateLLMConfig`
6. **Phase 6 (GREEN — Adapter)**: Implement in-house adapter in
   `pkg/apifrontend/launcher/openai/`
7. **Phase 7 (GREEN — Factory)**: Wire `newOpenAICompatibleModel` into
   `NewModelFromConfig`; `go get openai/openai-go`
8. **Phase 8 (CHECKPOINT W)**: Verify all wiring manifest rows
9. **Phase 9 (REFACTOR)**: Clean up, deduplicate conversion helpers

---

## 13. Wiring Manifest (Pyramid Invariant)

> UT proves logic. IT proves wiring. E2E proves the journey.

| Component | Production Entry Point | Wiring Location | UT (logic) | IT (wiring) | E2E (journey) |
|-----------|----------------------|-----------------|------------|-------------|---------------|
| `openai.NewModel()` | `newOpenAICompatibleModel()` | `pkg/apifrontend/launcher/model.go` | UT-AF-1254-029 | IT-AF-1254-001 | E2E-AF-1254-001 |
| `newOpenAICompatibleModel()` | `NewModelFromConfig()` switch | `pkg/apifrontend/launcher/model.go` | UT-AF-1254-020..028 | IT-AF-1254-004 | E2E-AF-1254-001 |
| `LLMProviderOpenAI` constant | `validateLLMConfig()` switch | `pkg/apifrontend/config/config.go` | UT-AF-1254-001..006 | IT-AF-1254-001 | E2E-AF-1254-001 |
| Transport chain injection | `buildLLMHTTPClient()` -> adapter | `pkg/apifrontend/launcher/model.go` | UT-AF-1254-029 | IT-AF-1254-003 | — |

---

## 14. Wiring Verification (TDD Phase 7)

| Code Path | Entry Point | Exit Point | Wiring Test | Status |
|-----------|-------------|------------|-------------|--------|
| Config -> Factory -> Adapter -> LLM | YAML config load | `model.LLM.GenerateContent()` | IT-AF-1254-001 | Pending |
| Keyless config -> Factory -> Adapter | YAML without apiKeyFile | `model.LLM.GenerateContent()` | IT-AF-1254-002 | Pending |
| TLS config -> Transport chain -> Adapter | `buildLLMHTTPClient()` | HTTPS request via custom transport | IT-AF-1254-003 | Pending |
| Full round-trip | `GenerateContent()` | `*model.LLMResponse` with content + tool calls | IT-AF-1254-004 | Pending |
| Full A2A journey (E2E) | A2A `message/send` | JSON-RPC response with analysis | E2E-AF-1254-001 | Pending |

**Unit tests do NOT count as wiring proof.** Only integration tests that
traverse the real factory/transport/adapter stack qualify.

---

## 15. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/1254/TEST_PLAN.md` | Strategy and test design |
| Config unit tests | `pkg/apifrontend/config/config_test.go` | New test cases in existing file |
| Adapter unit tests | `pkg/apifrontend/launcher/openai/adapter_test.go` | New file |
| Integration tests | `test/integration/apifrontend/openai_adapter_test.go` | New file |
| E2E tests | `test/e2e/apifrontend/openai_provider_test.go` | New file |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 16. Execution

```bash
# Unit tests (config + adapter)
go test ./pkg/apifrontend/config/... -ginkgo.v -ginkgo.focus="1254"
go test ./pkg/apifrontend/launcher/openai/... -ginkgo.v

# Integration tests
go test ./test/integration/apifrontend/... -ginkgo.v -ginkgo.focus="1254"

# E2E tests (requires Kind cluster with mock-LLM)
go test ./test/e2e/apifrontend/... -ginkgo.v -ginkgo.focus="1254"

# Coverage
go test ./pkg/apifrontend/launcher/openai/... -coverprofile=adapter_coverage.out
go tool cover -func=adapter_coverage.out
```

---

## 17. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| UT-AF-1252-001 (`model_test.go`) | Error message: "unsupported LLM provider" for unknown providers | No change needed | New providers are added to the switch; "unsupported" still returned for truly unknown values |
| `validateLLMConfig` error message | Lists vertex_ai, gemini, anthropic | Must include openai, openai_compatible | New providers added to the allowed list |

---

## 18. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-06-24 | Initial test plan |
