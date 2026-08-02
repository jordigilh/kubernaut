# Test Plan: apifrontend Vertex AI clients honor per-profile credentials

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1731-v1.0
**Feature**: AF's `vertex_ai` severity-triage LLM clients (Anthropic-on-Vertex and Gemini-on-Vertex) use each resolved profile's own credentials instead of relying solely on shared, process-wide ambient ADC.
**Version**: 1.0
**Created**: 2026-08-02
**Author**: Cursor Agent (session-driven, user-directed)
**Status**: Complete
**Branch**: `fix/1731-vertex-ai-per-profile-credentials` (based on `release/v1.5`)

---

## 1. Introduction

### 1.1 Purpose

API Frontend's `vertex_ai` client construction for severity-triage (`newAnthropicTriagerForVertex`,
`newGenAITriagerForVertex` in `cmd/apifrontend/main.go`) never reads a resolved LLM profile's own
`APIKey`/`APIKeyFile` — both paths rely solely on ambient Application Default Credentials (ADC),
shared process-wide. Two `vertex_ai` profiles with different `credentialsSecretName` (e.g. AF's
main `agent.llm` vs. an independent `severityTriage.llm` override) silently authenticate with
whichever credentials happen to be ambient, not their own. This test plan proves the fix threads
each profile's own resolved credential bytes into both Vertex construction paths, with zero
behavior change when no explicit credentials are resolved (ADC fallback preserved).

### 1.2 Objectives

1. **Explicit-bytes Anthropic-on-Vertex**: `severity.NewAnthropicVertexClient` constructs
   successfully from a profile's own `credentials.json` bytes alone, with ambient ADC absent —
   proving the bytes are actually used, not silently discarded.
2. **Explicit-bytes Gemini-on-Vertex**: `newGenAITriagerForVertex` resolves `genai.ClientConfig.Credentials`
   from a profile's own bytes alone, with ambient ADC absent.
3. **Backward compatibility**: both paths preserve today's ADC-fallback behavior unchanged when
   the profile has no resolved `APIKey` (current v1.5 default, until `kubernaut-operator` renders
   `apiKeyFile` for `vertex_ai` profiles — tracked as a companion, out-of-repo follow-up).
4. **No regression**: existing `TestAnthropicTriager_*` / `TestIsAnthropicModel` (plain `testing.T`,
   pre-dating this fix) continue to pass unmodified.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/severity/...` |
| Integration test pass rate | 100% | `go test ./cmd/apifrontend/...` |
| Backward compatibility | 0 regressions | Existing `anthropic_test.go` / `main_wiring_test.go` pass without modification |
| Live GCP/network calls in tests | 0 | All tests use locally-generated fake service-account JSON (no live credential validation at construction time) |

---

## 2. References

### 2.1 Authority

- BR-PLATFORM-008: Helm chart LLM configuration parity (explicitly tracks #1731 as an open constraint)
- Issue #1731: apifrontend: Vertex AI LLM clients ignore per-profile credentials, relying solely on ambient ADC
- Issue #1728 (KA precedent): per-phase-override credentials-bytes fix, mirrored here
- Issue #1801 (AF main-agent precedent): `newVertexGeminiModel`'s explicit-bytes pattern, mirrored here for severityTriage

### 2.2 Cross-References

- `pkg/kubernautagent/llm/anthropicfamily/client.go` (`resolveVertexAuth`) — proven reference implementation for explicit-bytes Anthropic-on-Vertex
- `pkg/apifrontend/launcher/model.go` (`newVertexGeminiModel`, `model_1801_test.go`) — proven reference implementation and test technique for explicit-bytes Gemini-on-Vertex

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `anthropic-sdk-go/vertex.WithCredentials` behaves differently than assumed from source reading alone | Fix silently no-ops, bug remains | Low | UT-AF-1731-001..003 | Test constructs a client with ambient ADC unset and only explicit bytes present — construction failure would surface immediately |
| R2 | Change breaks existing ADC-only (no `APIKey`) behavior on this maintenance branch | Regression for the majority of today's deployments (operator doesn't render `apiKeyFile` for `vertex_ai` yet) | Medium | UT-AF-1731-004, UT-AF-1731-006 | Explicit test proving empty `APIKey`/`credentialsJSON` still falls back to `vertex.WithGoogleAuth`/bare `genai.NewClient` unchanged |
| R3 | Full end-user benefit blocked without a `kubernaut-operator` companion change (per-profile `apiKeyFile` rendering + guard relaxation) | Fix lands but has no observable effect until operator catches up | High (known, accepted) | N/A — out of this repo's scope | Flagged explicitly to user; companion operator issue recommended separately |
| R4 | Release-branch (`release/v1.5`) test file (`anthropic_test.go`) uses legacy `testing.T`, diverging from `main`'s Ginkgo-migrated version | Inconsistent test style within the file | Low | N/A | New tests added in a new Ginkgo file per current mandatory convention; existing `testing.T` tests left untouched (out of scope for this fix) |

### 3.1 Risk-to-Test Traceability

R1 and R2 are the highest-value risks and are directly covered by UT-AF-1731-001..006 below. R3 has no test (it's a cross-repo dependency, not a code defect in this repo) and is called out to the user as a deferred follow-up rather than silently dropped.

---

## 4. Scope

### 4.1 Features to be Tested

- **`severity.NewAnthropicVertexClient`** (`pkg/apifrontend/severity/anthropic.go`): extended with an explicit `credentialsJSON` parameter; constructs via explicit bytes when present, ADC when absent.
- **`newGenAITriagerForVertex`** (`cmd/apifrontend/main.go`): resolves `genai.ClientConfig.Credentials` from the profile's `APIKey` bytes when present, preserving today's bare-ADC construction when absent.
- **`newAnthropicTriagerForVertex`** (`cmd/apifrontend/main.go`): threads `llmCfg.APIKey` into the extended `NewAnthropicVertexClient` call.

### 4.2 Features Not to be Tested

- **`main` branch (`cmd/apifrontend/backend_deps.go`, issue #1861)**: separate, non-identical patch per the issue's own branch-divergence note — out of scope for this plan.
- **`kubernaut-operator` credential rendering / validation guard**: separate repository; a companion issue will be proposed, not implemented here.
- **Live Vertex AI API calls** (actual model inference): credential *construction* is the unit under test, not live inference — no network calls in any test tier here.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Duplicate the ~15-line credential-type-detection helper locally in `pkg/apifrontend/severity` rather than importing `pkg/kubernautagent/llm/anthropicfamily` | Preserves the existing architectural boundary — AF and KA each own independent LLM client packages (mirrors how `newVertexGeminiModel` didn't reuse KA's `geminifamily` either); avoids a new cross-service dependency for ~15 lines |
| Always resolve credentials via `credentials.DetectDefault`-equivalent (explicit bytes when present, ADC when absent) rather than branching in the caller | Matches `newVertexGeminiModel`'s established pattern; the underlying SDK/library already implements the fallback correctly |
| No changes to `kubernaut-operator` in this task | Cross-repo change requires separate review/ownership; flagged as a companion follow-up per the #1069/#277 precedent from this session |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: 100% of the new/modified lines in `severity/anthropic.go` and the two `main.go` triager functions.
- **Integration**: the existing `newLLMTriagerFromConfig` wiring test suite (`main_wiring_test.go`) is extended to prove `APIKey` reaches both vertex_ai constructors through the real factory dispatch.
- **E2E**: not applicable — no live GCP credentials available in CI; covered instead by construction-level UT/IT using locally-generated fake service-account JSON (proven technique from `model_1801_test.go`).

### 5.2 Two-Tier Minimum

Both UT (construction logic) and IT (factory wiring/dispatch) are provided for every changed code path.

### 5.4 Pass/Fail Criteria

**PASS**: all new UT/IT pass; all pre-existing tests in `pkg/apifrontend/severity/` and `cmd/apifrontend/` pass unmodified; `go build ./...` and `golangci-lint run` clean.

**FAIL**: any new test fails, or any pre-existing test regresses.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/apifrontend/severity/anthropic.go` | `NewAnthropicVertexClient` | ~20 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `cmd/apifrontend/main.go` | `newAnthropicTriagerForVertex`, `newGenAITriagerForVertex` | ~40 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-PLATFORM-008 | AF severityTriage Anthropic-on-Vertex honors its own resolved credentials | P0 | Unit | UT-AF-1731-001..003 | Pass |
| BR-PLATFORM-008 | AF severityTriage Anthropic-on-Vertex preserves ADC fallback when no explicit credentials | P0 | Unit | UT-AF-1731-004 | Pass |
| BR-PLATFORM-008 | AF severityTriage Gemini-on-Vertex honors its own resolved credentials | P1 | Unit/Integration | UT-AF-1731-005, IT-AF-1731-002 | Pass |
| BR-PLATFORM-008 | AF severityTriage Gemini-on-Vertex preserves ADC fallback when no explicit credentials | P1 | Unit | UT-AF-1731-006 | Pass |
| BR-PLATFORM-008 | Factory (`newLLMTriagerFromConfig`) threads `APIKey` through for both vertex_ai model families | P0 | Integration | IT-AF-1731-001, IT-AF-1731-002 | Pass |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: `pkg/apifrontend/severity/anthropic.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AF-1731-001` | `NewAnthropicVertexClient` constructs successfully from explicit `credentials.json` bytes alone, ambient ADC absent | Pass |
| `UT-AF-1731-002` | `NewAnthropicVertexClient` rejects malformed/empty-type credential JSON with a clear error rather than silently falling back to ADC | Pass |
| `UT-AF-1731-003` | `NewAnthropicVertexClient` still requires `project` before touching any credential resolution (existing behavior preserved) | Pass |
| `UT-AF-1731-004` | `NewAnthropicVertexClient` falls back to ambient ADC unchanged when `credentialsJSON` is empty (backward compatibility) | Pass |
| `UT-AF-1731-005` | (via `cmd/apifrontend` package test) `newGenAITriagerForVertex` resolves `genai.ClientConfig.Credentials` from explicit bytes alone, ambient ADC absent | Pass |
| `UT-AF-1731-006` | `newGenAITriagerForVertex` preserves today's bare-ADC construction when `APIKey` is empty | Pass |

### Tier 2: Integration Tests

**Testable code scope**: `cmd/apifrontend/main.go` (`newLLMTriagerFromConfig` dispatch)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-AF-1731-001` | `newLLMTriagerFromConfig` routes a `vertex_ai` + `claude-*` profile's `APIKey` through to a successfully-constructed `AnthropicTriager`, with no ambient ADC | Pass |
| `IT-AF-1731-002` | `newLLMTriagerFromConfig` routes a `vertex_ai` + `gemini-*` profile's `APIKey` through to a successfully-constructed `GenAITriager`, with no ambient ADC | Pass |

### Tier Skip Rationale

- **E2E**: no live GCP credentials in CI; construction-level UT/IT with locally-generated fake service-account JSON (RSA-keyed, parses/validates locally with zero network calls) provides equivalent proof without a live dependency, mirroring `model_1801_test.go`'s established technique.

---

## 9. Test Cases (P0 detail)

### UT-AF-1731-001: Explicit-bytes Anthropic-on-Vertex construction

**BR**: BR-PLATFORM-008
**Priority**: P0
**Type**: Unit
**File**: `pkg/apifrontend/severity/anthropic_vertex_credentials_test.go`

**Preconditions**: `GOOGLE_APPLICATION_CREDENTIALS` unset for the duration of the test.

**Test Steps**:
1. **Given**: a locally-generated fake RSA-keyed `service_account` JSON blob and a non-empty `project`.
2. **When**: `NewAnthropicVertexClient(ctx, project, location, credentialsJSON)` is called.
3. **Then**: construction succeeds and returns a non-nil client, without reading any ambient env var.

**Expected Results**: `err` is nil; `client` is non-nil.

**Acceptance Criteria**: explicit bytes alone are sufficient for construction — proves the parameter isn't silently discarded.

---

### IT-AF-1731-001: Factory wiring proof for Anthropic-on-Vertex

**BR**: BR-PLATFORM-008
**Priority**: P0
**Type**: Integration
**File**: `cmd/apifrontend/main_wiring_test.go`

**Test Steps**:
1. **Given**: `config.LLMConfig{Provider: vertex_ai, Model: "claude-sonnet-4-6", APIKey: <fake JSON>}`, ambient ADC unset.
2. **When**: `newLLMTriagerFromConfig` is called (the real production dispatch path).
3. **Then**: an `AnthropicTriager` is returned with no error.

**Acceptance Criteria**: proves the fix is reachable from the actual production wiring point, not just the leaf constructor.

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory for new tests; pre-existing `testing.T` tests in `anthropic_test.go` are left untouched, out of scope)
- **Mocks**: none — real SDK types, local fake credential JSON (no external dependency)
- **Location**: `pkg/apifrontend/severity/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: zero — real factory dispatch, local fake credential JSON
- **Location**: `cmd/apifrontend/`

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| `kubernaut-operator` per-profile `apiKeyFile` rendering for `vertex_ai` | Cross-repo code | Not started | Fix lands but has no observable production effect until operator renders per-profile credentials | Companion issue proposed separately; this repo's fix is forward-compatible and non-breaking either way |

### 11.2 Execution Order

1. **Phase 1 (RED)**: UT-AF-1731-001..006 written against the not-yet-extended signatures — fail to compile/fail assertions.
2. **Phase 2 (GREEN)**: extend `NewAnthropicVertexClient` signature + `newGenAITriagerForVertex`/`newAnthropicTriagerForVertex` in `main.go`; wire `llmCfg.APIKey` through both call sites.
3. **Phase 3 (REFACTOR)**: N/A expected (small, targeted addition, no pre-existing duplication in these two functions) — confirmed after GREEN.
4. **Phase 4 (WIRING VERIFICATION)**: IT-AF-1731-001/002 prove the fix is reachable from `newLLMTriagerFromConfig`, the real production dispatch point.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|--------------|
| This test plan | `docs/testing/1731/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `pkg/apifrontend/severity/anthropic_vertex_credentials_test.go` | Ginkgo BDD |
| Integration test suite | `cmd/apifrontend/main_wiring_test.go` (extended) | Ginkgo BDD |

---

## 13. Execution

```bash
go test ./pkg/apifrontend/severity/... -run TestSeveritySuite -ginkgo.focus="1731"
go test ./cmd/apifrontend/... -run TestMain -ginkgo.focus="1731"
```

---

## 14. Wiring Verification (TDD Phase 4)

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|-------------|------------|-----------|--------|
| `newAnthropicTriagerForVertex` → `severity.NewAnthropicVertexClient` | `newLLMTriagerFromConfig` (severityTriage LLM factory, called from AF startup wiring) | constructed `*anthropic.Client` | `IT-AF-1731-001` | Pass |
| `newGenAITriagerForVertex` | `newLLMTriagerFromConfig` | constructed `*genai.Client` | `IT-AF-1731-002` | Pass |

---

## 15. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None | `NewAnthropicVertexClient(ctx, project, location)` call sites | Signature gains a 4th `credentialsJSON` parameter | All existing callers (`main.go`, `anthropic_test.go`) updated to pass `llmCfg.APIKey` or `""` |

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-08-02 | Initial test plan |
