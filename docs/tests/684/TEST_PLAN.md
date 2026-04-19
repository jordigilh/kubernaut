# Test Plan: Vertex AI + Claude Regression Fix

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-684-v1
**Feature**: Fix 3-bug regression preventing Claude-on-Vertex-AI provider in KA v1.3
**Version**: 1.0
**Created**: 2026-04-06
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/684-vertex-ai-claude`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the fix for Issue #684, a 3-bug regression that prevents
the Kubernaut Agent (KA) v1.3 from using Claude models hosted on Google Vertex AI
(`provider: vertex_ai`). This was a supported configuration in v1.2 (via litellm).
The test plan provides confidence that all three bugs are fixed without introducing
regressions to existing LLM providers.

### 1.2 Objectives

1. **Config Merge Completeness**: `MergeSDKConfig()` correctly propagates all LLM fields (`endpoint`, `vertex_project`, `vertex_location`, `api_key`, `temperature`, `max_retries`, `timeout_seconds`, `bedrock_region`, `azure_api_version`) from SDK config to main config using gap-fill semantics.
2. **Provider Alias Recognition**: `newModel()` recognizes `vertex_ai` as a valid provider name and routes it to the Claude-on-Vertex-AI code path. `Validate()` exempts `vertex`/`vertex_ai` from requiring an endpoint. `resolveCredentialsFile` maps `vertex`/`vertex_ai` to `GOOGLE_APPLICATION_CREDENTIALS`.
3. **Claude-on-Vertex-AI Round-trip**: A `vertex_ai` adapter successfully sends an Anthropic Messages API request to a Vertex AI Model Garden endpoint (via httptest mock), correctly formats the request body, uses GCP ADC-style auth, and parses the Anthropic response including text and tool calls.
4. **Zero Regressions**: All existing unit and integration tests continue to pass. No behavioral changes to `openai`, `ollama`, `azure`, `vertex` (Gemini), `anthropic`, `bedrock`, `huggingface`, or `mistral` providers.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/config/... ./test/unit/kubernautagent/llm/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/llm/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `config.go`, `adapter.go`, `vertex_anthropic.go` |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on adapter httptest files |
| Backward compatibility | 0 regressions | Existing tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-AI-684: Kubernaut Agent must support Claude models on Vertex AI (regression from v1.2)
- Issue #684: Kubernaut Agent: Vertex AI + Claude regression — 3 bugs blocking SDK config and provider wiring

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | SDK config merge changes break existing provider field inheritance | Config regression for all providers | Medium | UT-KA-684-001 through 006 | Gap-fill semantics: main config always takes precedence; only empty fields are filled from SDK |
| R2 | `vertex_ai` alias leaks into unrelated switch cases | Wrong code path for `vertex` (Gemini) | Low | UT-KA-684-101, 102 | Separate switch cases for `vertex` (Gemini) and `vertex_ai` (Claude); no fall-through |
| R3 | Anthropic SDK dependency introduces supply-chain risk | Security vulnerability via transitive deps | Low | All Bug 3 tests | Use official `github.com/anthropics/anthropic-sdk-go` with pinned version; audit transitive deps |
| R4 | GCP ADC auth cannot be tested in CI (no real GCP credentials) | False confidence in auth path | High | UT-KA-684-201, IT-KA-684-201 | Unit tests use mock credentials fixture; integration tests use httptest to intercept HTTP; manual validation with demo-scenarios team on Kind cluster |
| R5 | Vertex AI rawPredict endpoint URL format changes | Runtime 404 errors | Low | UT-KA-684-203, IT-KA-684-201 | URL construction tested with assertions on exact path format; documented in code |
| R6 | Temperature/MaxRetries/TimeoutSeconds SDK fields parsed but never wired to LLM calls | Silent config loss for operational tuning | Medium | UT-KA-684-005, 006 | Test that merged config fields are accessible; document that LangChainGo call options pass temperature through |

### 3.1 Risk-to-Test Traceability

- **R1**: UT-KA-684-001 through 006 validate every merged field and gap-fill precedence
- **R2**: UT-KA-684-101, 102 validate both `vertex` (Gemini) and `vertex_ai` (Claude) remain separate paths
- **R3**: Checkpoint audit after Bug 3 GREEN phase reviews dependency tree
- **R4**: IT-KA-684-201 uses httptest to validate full request/response cycle without real GCP; manual validation planned
- **R5**: UT-KA-684-203 asserts exact URL path construction
- **R6**: UT-KA-684-005, 006 validate temperature and operational fields survive merge

---

## 4. Scope

### 4.1 Features to be Tested

- **Config merge** (`internal/kubernautagent/config/config.go`): `SDKConfig` struct extension and `MergeSDKConfig()` gap-fill for all missing LLM fields
- **Provider validation** (`internal/kubernautagent/config/config.go`): `Validate()` endpoint exemption for `vertex`/`vertex_ai`
- **Provider wiring** (`pkg/kubernautagent/llm/langchaingo/adapter.go`): `newModel()` `vertex_ai` case routing to Claude-on-Vertex shim
- **Vertex Anthropic shim** (`pkg/kubernautagent/llm/langchaingo/vertex_anthropic.go`): New `llms.Model` implementation wrapping Anthropic SDK vertex subpackage
- **Credential resolution** (`cmd/kubernautagent/main.go`): `resolveCredentialsFile` mapping for `vertex`/`vertex_ai`
- **Main wiring** (`cmd/kubernautagent/main.go`): `buildLLMProviderOptions` passes Vertex fields through

### 4.2 Features Not to be Tested

- **Real GCP Vertex AI API calls**: Deferred to manual validation with demo-scenarios team (R4)
- **Other providers' Chat() behavior**: Covered by existing tests (UT-KA-433-* and IT-KA-433-*)
- **Helm chart changes**: Separate issue if needed; SDK config YAML format is backward-compatible
- **E2E tests**: Deferred — requires Kind cluster with GCP credentials; covered by manual validation

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Use `anthropic-sdk-go` official SDK for Vertex AI Claude | Well-maintained, handles GCP ADC auth, URL construction, and Anthropic Messages API natively via `vertex` subpackage |
| Implement `vertexAnthropicModel` as `llms.Model` shim | Allows reuse of existing `Adapter.Chat()` message conversion and call options; clean integration with LangChainGo interface |
| Separate `vertex_ai` from `vertex` in switch (not alias) | Different API surfaces (Gemini vs Anthropic Messages); different auth (ADC for both but different endpoints); avoids accidental routing |
| Gap-fill semantics for all SDK config fields | Maintains backward compatibility: main config always wins; SDK config only fills gaps |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code: `config.go` (MergeSDKConfig, Validate), `adapter.go` (newModel), `vertex_anthropic.go` (shim)
- **Integration**: >=80% of integration-testable code: adapter httptest round-trips for `vertex_ai`
- **E2E**: Deferred — requires GCP credentials in Kind cluster; manual validation with demo-scenarios team

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers (UT + IT):
- **Unit tests**: Validate config merge logic, provider selection, validation rules, shim message conversion
- **Integration tests**: Validate full adapter round-trip via httptest, request/response wire format

### 5.3 Business Outcome Quality Bar

Tests validate **business outcomes**:
- "Operator provides `vertex_ai` SDK config and KA starts successfully" (not just "function returns nil")
- "Claude on Vertex AI returns investigation results with tool calls" (not just "HTTP 200")
- "Existing providers continue to work after changes" (regression guard)

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier code coverage meets >=80% threshold
4. No regressions in existing test suites
5. `vertex_ai` provider with Claude model produces correct Chat() responses via httptest

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80%
3. Existing tests that were passing before the change now fail
4. Config merge silently drops any LLM field

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:

- `anthropic-sdk-go` dependency cannot be resolved (module proxy issue)
- Build broken due to LangChainGo API changes
- Cascading failures in >3 tests from same root cause

**Resume testing when**:

- Dependency issue resolved
- Build fixed and green locally
- Root cause identified and fix deployed

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/config/config.go` | `MergeSDKConfig`, `Validate`, `SDKConfig` struct | ~80 |
| `pkg/kubernautagent/llm/langchaingo/adapter.go` | `newModel` (switch cases) | ~65 |
| `pkg/kubernautagent/llm/langchaingo/vertex_anthropic.go` | `vertexAnthropicModel` shim, message conversion | ~120 (new) |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/kubernautagent/llm/langchaingo/adapter.go` | `New`, `Chat` full round-trip | ~30 |
| `cmd/kubernautagent/main.go` | `resolveCredentialsFile`, `buildLLMProviderOptions` | ~40 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/684-vertex-ai-claude` HEAD | Branch off `main` |
| Dependency: anthropic-sdk-go | latest | To be added via `go get` |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AI-684 | SDK config merges endpoint from SDK YAML | P0 | Unit | UT-KA-684-001 | Pending |
| BR-AI-684 | SDK config merges vertex_project from SDK YAML | P0 | Unit | UT-KA-684-002 | Pending |
| BR-AI-684 | SDK config merges vertex_location from SDK YAML | P0 | Unit | UT-KA-684-003 | Pending |
| BR-AI-684 | SDK config merges api_key from SDK YAML | P0 | Unit | UT-KA-684-004 | Pending |
| BR-AI-684 | SDK config merges temperature from SDK YAML | P1 | Unit | UT-KA-684-005 | Pending |
| BR-AI-684 | SDK config merges max_retries and timeout_seconds | P1 | Unit | UT-KA-684-006 | Pending |
| BR-AI-684 | Main config takes precedence over SDK (gap-fill) | P0 | Unit | UT-KA-684-007 | Pending |
| BR-AI-684 | SDK config merges bedrock_region | P1 | Unit | UT-KA-684-008 | Pending |
| BR-AI-684 | SDK config merges azure_api_version | P1 | Unit | UT-KA-684-009 | Pending |
| BR-AI-684 | Existing provider/model merge unaffected (regression) | P0 | Unit | UT-KA-684-010 | Pending |
| BR-AI-684 | `vertex_ai` recognized as valid provider | P0 | Unit | UT-KA-684-101 | Pending |
| BR-AI-684 | `vertex` (Gemini) still works after changes | P0 | Unit | UT-KA-684-102 | Pending |
| BR-AI-684 | `Validate()` exempts vertex/vertex_ai from endpoint | P0 | Unit | UT-KA-684-103 | Pending |
| BR-AI-684 | `Validate()` still requires endpoint for non-exempt | P0 | Unit | UT-KA-684-104 | Pending |
| BR-AI-684 | `vertex_ai` without project returns error | P0 | Unit | UT-KA-684-105 | Pending |
| BR-AI-684 | `resolveCredentialsFile` maps vertex to GOOGLE_APPLICATION_CREDENTIALS | P1 | Unit | UT-KA-684-106 | Pending |
| BR-AI-684 | Vertex Anthropic shim: text response | P0 | Unit | UT-KA-684-201 | Pending |
| BR-AI-684 | Vertex Anthropic shim: tool call response | P0 | Unit | UT-KA-684-202 | Pending |
| BR-AI-684 | Vertex Anthropic shim: correct URL construction | P0 | Unit | UT-KA-684-203 | Pending |
| BR-AI-684 | Vertex Anthropic shim: error handling (HTTP 500) | P1 | Unit | UT-KA-684-204 | Pending |
| BR-AI-684 | Vertex Anthropic shim: empty response handling | P1 | Unit | UT-KA-684-205 | Pending |
| BR-AI-684 | vertex_ai full round-trip via httptest | P0 | Integration | IT-KA-684-201 | Pending |
| BR-AI-684 | vertex_ai with tool calls via httptest | P0 | Integration | IT-KA-684-202 | Pending |
| BR-AI-684 | Existing Azure round-trip unaffected | P0 | Integration | IT-KA-684-203 | Pending |
| BR-AI-684 | Existing Anthropic round-trip unaffected | P0 | Integration | IT-KA-684-204 | Pending |

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

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `KA` (Kubernaut Agent)
- **BR_NUMBER**: 684
- **SEQUENCE**: Bug1=001-010, Bug2=101-106, Bug3=201-205, IT=201-204

### Tier 1: Unit Tests

**Testable code scope**: `config.go` (MergeSDKConfig, Validate, SDKConfig), `adapter.go` (newModel), `vertex_anthropic.go` (shim). >=80% coverage target.

#### Bug 1: Config Merge

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-684-001` | Operator provides endpoint in SDK config and KA uses it for LLM calls | Pending |
| `UT-KA-684-002` | Operator provides vertex_project in SDK config and KA passes it to Vertex provider | Pending |
| `UT-KA-684-003` | Operator provides vertex_location in SDK config and KA passes it to Vertex provider | Pending |
| `UT-KA-684-004` | Operator provides api_key in SDK config and KA uses it for authentication | Pending |
| `UT-KA-684-005` | Operator provides temperature in SDK config and KA applies it to LLM calls | Pending |
| `UT-KA-684-006` | Operator provides max_retries/timeout_seconds in SDK config and values survive merge | Pending |
| `UT-KA-684-007` | Main config endpoint takes precedence when both main and SDK provide it | Pending |
| `UT-KA-684-008` | Operator provides bedrock_region in SDK config and KA passes it to Bedrock provider | Pending |
| `UT-KA-684-009` | Operator provides azure_api_version in SDK config and KA passes it to Azure provider | Pending |
| `UT-KA-684-010` | Existing provider/model gap-fill semantics remain unchanged after SDK struct extension | Pending |

#### Bug 2: Provider Alias + Validation

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-684-101` | Adapter accepts `vertex_ai` as a valid provider and creates a non-nil adapter | Pending |
| `UT-KA-684-102` | Existing `vertex` (Gemini) provider still creates adapter successfully | Pending |
| `UT-KA-684-103` | `Validate()` accepts config with `vertex_ai` provider and no endpoint | Pending |
| `UT-KA-684-104` | `Validate()` still rejects config with `mistral` provider and no endpoint | Pending |
| `UT-KA-684-105` | `vertex_ai` provider without project returns descriptive error | Pending |
| `UT-KA-684-106` | `resolveCredentialsFile` returns GOOGLE_APPLICATION_CREDENTIALS path for vertex provider | Pending |

#### Bug 3: Vertex Anthropic Shim

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-684-201` | Claude on Vertex AI returns text investigation results to the investigator | Pending |
| `UT-KA-684-202` | Claude on Vertex AI returns tool calls (kubectl_describe, list_workflows) to the investigator | Pending |
| `UT-KA-684-203` | Vertex Anthropic shim constructs correct rawPredict URL from project/location/model | Pending |
| `UT-KA-684-204` | Vertex Anthropic shim surfaces Vertex AI HTTP errors to the investigator | Pending |
| `UT-KA-684-205` | Vertex Anthropic shim handles empty/malformed responses gracefully | Pending |

### Tier 2: Integration Tests

**Testable code scope**: Full adapter round-trip via httptest. >=80% coverage target.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-684-201` | vertex_ai adapter sends Anthropic Messages API request to correct endpoint and returns parsed response | Pending |
| `IT-KA-684-202` | vertex_ai adapter handles tool call responses end-to-end via httptest | Pending |
| `IT-KA-684-203` | Azure adapter round-trip unaffected by #684 changes (regression guard) | Pending |
| `IT-KA-684-204` | Anthropic adapter round-trip unaffected by #684 changes (regression guard) | Pending |

### Tier Skip Rationale

- **E2E**: Deferred — requires Kind cluster with GCP Application Default Credentials configured. Manual validation planned with demo-scenarios team using custom KA image deployed to their Kind cluster. E2E tests will be added as a follow-up when GCP credentials are available in CI.

---

## 9. Test Cases

### UT-KA-684-001: SDK config merges endpoint

**BR**: BR-AI-684
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/config/config_684_test.go`

**Test Steps**:
1. **Given**: Main config has no endpoint; SDK YAML has `endpoint: "https://europe-west1-aiplatform.googleapis.com"` <!-- pre-commit:allow-sensitive (test fixture) -->
2. **When**: `MergeSDKConfig()` is called
3. **Then**: `cfg.LLM.Endpoint` equals the SDK endpoint value

**Acceptance Criteria**:
- Endpoint from SDK config is accessible on merged config
- No error returned from MergeSDKConfig

---

### UT-KA-684-002: SDK config merges vertex_project

**BR**: BR-AI-684
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/config/config_684_test.go`

**Test Steps**:
1. **Given**: Main config has no vertex_project; SDK YAML has `vertex_project: "my-gcp-project"`
2. **When**: `MergeSDKConfig()` is called
3. **Then**: `cfg.LLM.VertexProject` equals `"my-gcp-project"`

---

### UT-KA-684-003: SDK config merges vertex_location

**BR**: BR-AI-684
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/config/config_684_test.go`

**Test Steps**:
1. **Given**: Main config has no vertex_location; SDK YAML has `vertex_location: "europe-west1"`
2. **When**: `MergeSDKConfig()` is called
3. **Then**: `cfg.LLM.VertexLocation` equals `"europe-west1"`

---

### UT-KA-684-004: SDK config merges api_key

**BR**: BR-AI-684
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/config/config_684_test.go`

**Test Steps**:
1. **Given**: Main config has no api_key; SDK YAML has `api_key: "sk-test-from-sdk"`
2. **When**: `MergeSDKConfig()` is called
3. **Then**: `cfg.LLM.APIKey` equals `"sk-test-from-sdk"`

---

### UT-KA-684-007: Main config takes precedence (gap-fill)

**BR**: BR-AI-684
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/config/config_684_test.go`

**Test Steps**:
1. **Given**: Main config has `endpoint: "http://main-endpoint"`; SDK YAML has `endpoint: "http://sdk-endpoint"`
2. **When**: `MergeSDKConfig()` is called
3. **Then**: `cfg.LLM.Endpoint` remains `"http://main-endpoint"` (not overwritten)

---

### UT-KA-684-101: vertex_ai recognized as valid provider

**BR**: BR-AI-684
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/llm/langchaingo_adapter_684_test.go`

**Test Steps**:
1. **Given**: GCP mock credentials fixture available
2. **When**: `langchaingo.New("vertex_ai", "", "claude-sonnet-4-6", "", WithVertexProject("p"), WithVertexLocation("us-central1"))` is called
3. **Then**: Returns non-nil adapter with no error

---

### UT-KA-684-103: Validate exempts vertex/vertex_ai from endpoint requirement

**BR**: BR-AI-684
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/config/config_684_test.go`

**Test Steps**:
1. **Given**: Config with `provider: "vertex_ai"`, `model: "claude-sonnet-4-6"`, empty endpoint
2. **When**: `cfg.Validate()` is called
3. **Then**: Returns nil (no validation error)

---

### UT-KA-684-201: Vertex Anthropic shim text response

**BR**: BR-AI-684
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/llm/langchaingo_adapter_684_test.go`

**Test Steps**:
1. **Given**: httptest server returning Anthropic Messages API response with text content
2. **When**: `adapter.Chat()` called with vertex_ai adapter pointed at httptest server
3. **Then**: Response contains assistant role and expected text content

---

### IT-KA-684-201: vertex_ai full round-trip via httptest

**BR**: BR-AI-684
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/llm/adapter_httptest_684_test.go`

**Test Steps**:
1. **Given**: httptest server mocking Vertex AI rawPredict endpoint with Anthropic response format
2. **When**: Full `langchaingo.New("vertex_ai", ...) + adapter.Chat()` cycle executed
3. **Then**: Request path contains correct rawPredict format; response parsed correctly with content and tool calls

**Acceptance Criteria**:
- Request URL path matches `v1/projects/{project}/locations/{location}/publishers/anthropic/models/{model}:rawPredict`
- Request body contains Anthropic Messages API format
- Response content is correctly extracted

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: GCP mock credentials fixture (`test/fixtures/gcp-mock-credentials.json`); httptest for shim tests
- **Location**: `test/unit/kubernautagent/config/`, `test/unit/kubernautagent/llm/`
- **Resources**: Standard CI runner

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO external mocks; httptest servers simulate Vertex AI endpoint
- **Infrastructure**: httptest.Server
- **Location**: `test/integration/kubernautagent/llm/`
- **Resources**: Standard CI runner

### 10.3 E2E Tests (deferred)

- **Status**: Deferred — requires GCP ADC credentials in Kind cluster
- **Validation**: Manual testing with demo-scenarios team using custom KA image

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| anthropic-sdk-go | latest | Vertex Anthropic shim dependency |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| anthropic-sdk-go | Go module | Available | Bug 3 tests cannot be implemented | Use raw HTTP client (higher risk) |
| GCP mock credentials | Test fixture | Available | Vertex adapter tests fail | Already exists at `test/fixtures/gcp-mock-credentials.json` |

### 11.2 Execution Order

1. **Phase 1 (Bug 1)**: Config merge — RED / GREEN / REFACTOR + Checkpoint 1
2. **Phase 2 (Bug 2)**: Provider alias + validation — RED / GREEN / REFACTOR + Checkpoint 2
3. **Phase 3 (Bug 3)**: Vertex Anthropic shim — RED / GREEN / REFACTOR + Final Checkpoint
4. **Phase 4**: PR creation, CI validation

**Rationale**: Bug 1 (config merge) unblocks all providers. Bug 2 (provider alias) depends on Bug 1 for field availability. Bug 3 (shim) depends on Bug 2 for routing.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/684/TEST_PLAN.md` | Strategy and test design |
| Bug 1 unit tests | `test/unit/kubernautagent/config/config_684_test.go` | SDK config merge tests |
| Bug 2 unit tests | `test/unit/kubernautagent/llm/langchaingo_adapter_684_test.go` | Provider alias + validation tests |
| Bug 3 unit tests | `test/unit/kubernautagent/llm/langchaingo_adapter_684_test.go` | Vertex Anthropic shim tests |
| Integration tests | `test/integration/kubernautagent/llm/adapter_httptest_684_test.go` | httptest round-trip tests |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Bug 1 unit tests
go test ./test/unit/kubernautagent/config/... -ginkgo.v -ginkgo.focus="684"

# Bug 2 + Bug 3 unit tests
go test ./test/unit/kubernautagent/llm/... -ginkgo.v -ginkgo.focus="684"

# Integration tests
go test ./test/integration/kubernautagent/llm/... -ginkgo.v -ginkgo.focus="684"

# All 684 tests
go test ./test/unit/kubernautagent/config/... ./test/unit/kubernautagent/llm/... ./test/integration/kubernautagent/llm/... -ginkgo.v -ginkgo.focus="684"

# Coverage
go test ./test/unit/kubernautagent/config/... -coverprofile=coverage-config.out -coverpkg=github.com/jordigilh/kubernaut/internal/kubernautagent/config
go test ./test/unit/kubernautagent/llm/... -coverprofile=coverage-llm.out -coverpkg=github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo
```

---

## 14. Existing Tests Requiring Updates (if applicable)

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None expected | N/A | N/A | All changes are additive (new fields, new switch case, new file); existing tests should not need modification |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-06 | Initial test plan |
