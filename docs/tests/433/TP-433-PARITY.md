# Test Plan: Kubernaut Agent HAPI-KA Integration Test Parity (#433)

> **Template**: IEEE 829-2008 + Kubernaut Hybrid v2.0

**Test Plan Identifier**: TP-433-PARITY-v1.0
**Feature**: Close the integration-test parity gap between the deprecated Python HolmesGPT-API and the Go Kubernaut Agent, ensuring no business-scenario regression during the HAPI-to-KA migration.
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.3`

---

## 1. Introduction

### 1.1 Purpose

TP-433 (119 scenarios) covers the Go Kubernaut Agent rewrite core: engine, investigation loop, tools, security, and E2E parity. A triage of 49 Python HAPI integration tests against 89 Go KA/AA integration tests reveals **6 gap areas** where business scenarios tested in Python have no Go equivalent. This supplementary plan closes those gaps so that the HAPI-to-KA transition is provably complete at the integration test tier.

### 1.2 Objectives

1. **Detected labels parity**: Go KA detects all 10 cluster characteristics (GitOps, HPA, PDB, etc.) and returns them in the `detected_labels` response field (already declared in the OpenAPI spec but not populated)
2. **Signal mode parity**: Go KA reads `signal_mode` from `IncidentRequest` and switches prompt strategy between reactive and proactive (BR-AI-084 R4)
3. **Token audit parity**: Go KA accumulates LLM token usage and includes it in audit events (`aiagent.llm.response`, `aiagent.response.complete`)
4. **LLM metrics parity**: Go KA emits Prometheus metrics for LLM call count, duration, and token usage (BR-HAPI-011, BR-HAPI-301)
5. **Three-phase RCA parity**: Go KA extracts `RemediationTarget` from nested parser output, forwards it from Phase 1 to final result, and surfaces detected labels in Phase 3 workflow selection context
6. **Prompt content parity**: Go KA prompt builder renders cluster context sections and detected labels when present

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| New UT pass rate | 100% | `go test ./test/unit/kubernautagent/... -ginkgo.focus="433-(DL\|SM\|TK\|LM\|RCA\|PB)"` |
| New IT pass rate | 100% | `go test ./test/integration/kubernautagent/... -ginkgo.focus="433-(DL\|SM\|TK\|LM\|RCA)"` |
| Unit-testable code coverage | >=80% | Coverage on enrichment, prompt, parser, audit, llm packages |
| Integration-testable code coverage | >=80% | Coverage on investigator, enrichment, llm packages |
| Existing test regressions | 0 | Full TP-433 suite passes without modification |
| No duplicate test IDs | 0 collisions | `grep -r "KA-433-" test/ \| sort \| uniq -d` |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-HAPI-433**: Go Language Migration (parent BR -- "no feature regression")
- **BR-SP-101**: DetectedLabels Auto-Detection (8 cluster characteristics)
- **ADR-056 v1.7**: Post-RCA Label Computation Relocation (detection in EnrichmentService Phase 2)
- **BR-AI-084 R4**: Proactive Signal Mode Prompt Strategy (HAPI MUST switch prompt)
- **BR-HAPI-011**: Investigation Metrics (Prometheus observability)
- **BR-HAPI-301**: LLM Observability Metrics (token usage, duration, provider labels)
- **BR-HAPI-016**: Remediation History Context Enrichment
- **BR-HAPI-261**: LLM-Provided Affected Resource with Owner Resolution
- **BR-HAPI-264**: Post-RCA Infrastructure Label Detection via EnrichmentService
- **BR-HAPI-265**: Infrastructure Labels in Workflow Discovery Context
- **Issue #435**: Wire LLM Token Usage into Audit Traces
- **Issue #529**: Three-Phase RCA Flow

### 2.2 Cross-References

- [TP-433-v1.0](./TEST_PLAN.md) -- Parent test plan (119 scenarios)
- [TP-433-WIR-v1.0](./TP-433-WIR-v1.0.md) -- Wiring supplement (33 scenarios)
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [docs/tests/435/TEST_PLAN.md](../435/TEST_PLAN.md) -- Token audit test plan (scope: tokens are audit-only, NOT API response)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R-PAR-1 | **Label detection K8s queries slow in large clusters** -- EnrichmentService must query HPA, PDB, annotations per ADR-056 v1.7 | Enrichment timeout, incomplete labels | Medium | IT-KA-433-DL-001..003 | Enricher already has timeout handling; detection queries are O(1) per namespace. IT uses fake K8s client. |
| R-PAR-2 | **InstrumentedClient adds latency to every LLM call** -- Prometheus observe/increment on every Chat() | Investigation latency increase | Low | IT-KA-433-LM-001..002 | Prometheus operations are O(1). P4-REFACTOR validates overhead < 1ms. |
| R-PAR-3 | **Token counts zero for some LLM providers** -- Ollama and some open-source models may not report token usage | Audit events emit 0 instead of real counts | Medium | UT-KA-433-TK-002 | `TokenAccumulator` handles zero gracefully; emits `0` rather than omitting fields. |
| R-PAR-4 | **OpenAPI contract drift** -- `detected_labels` field already in spec but ogen-generated types may not match enrichment struct | Type mismatch at compile time | Low | IT-KA-433-DL-003 | CHECKPOINT P-1 validates ogen type compatibility before proceeding. |
| R-PAR-5 | **Signal mode regression** -- Changing hardcoded "reactive" may break existing tests that assume reactive-only prompts | Existing UT-KA-433-017..020 fail | Medium | UT-KA-433-SM-003 | SM-003 explicitly tests empty/missing signal_mode defaults to reactive. GREEN phase runs full existing suite. |

### 3.1 Risk-to-Test Traceability

| Risk | Primary Tests | Secondary Tests |
|------|--------------|-----------------|
| R-PAR-1 | IT-KA-433-DL-001..003 | CHECKPOINT P-1 |
| R-PAR-2 | IT-KA-433-LM-001..002 | CHECKPOINT P-3 |
| R-PAR-3 | UT-KA-433-TK-002 | IT-KA-433-TK-001 |
| R-PAR-4 | IT-KA-433-DL-003 | CHECKPOINT P-1 |
| R-PAR-5 | UT-KA-433-SM-003 | CHECKPOINT P-2 |

---

## 4. Scope

### 4.1 Features to be Tested

- **Detected Labels Detection** (`internal/kubernautagent/enrichment/enricher.go`): 10 cluster characteristics detected from K8s resources, wired to `InvestigationResult` and API response
- **Signal Mode Prompt Strategy** (`internal/kubernautagent/prompt/builder.go`, `internal/kubernautagent/types/types.go`): `signal_mode` wired from request to prompt, activating proactive template branches
- **Token Usage Audit** (`internal/kubernautagent/investigator/investigator.go`, `internal/kubernautagent/audit/emitter.go`): Token accumulation from `ChatResponse.Usage`, injected into audit events
- **LLM Prometheus Metrics** (`pkg/kubernautagent/llm/`): `InstrumentedClient` wrapper emitting call count, duration, token counters
- **Three-Phase RCA Parity** (`internal/kubernautagent/parser/parser.go`, `internal/kubernautagent/server/handler.go`): Nested `RemediationTarget` extraction, Phase 1 forwarding, `root_cause_analysis` map enrichment
- **Prompt Content Assertions** (`internal/kubernautagent/prompt/builder.go`): Cluster context rendering, detected labels section rendering

### 4.2 Features Not to be Tested

- **Session HTTP semantics** (BR-AA-HAPI-064): E2E-tier per TESTING_GUIDELINES; existing E2E-KA-433-003..005 cover 202/404/409
- **Workflow security gate** (BR-HAPI-017-003): Tested in Data Storage ITs (`IT-DS-017-*`); KA delegates to DS
- **Three-step discovery pagination** (BR-HAPI-017-001): Tested in DS ITs; KA tools forward to DS
- **Conversation continuity** (BR-HAPI-263): Two-invocation architecture is intentional -- Phase 3 starts fresh with injected RCA summary per `phase3_workflow_selection.tmpl` header
- **Remediation history/spec drift prompt content**: Already covered by `UT-KA-433-HP-001..011` and `UT-KA-433-017..020`

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Sub-namespaced test IDs (`433-DL-*`, `433-SM-*`, etc.) | Avoids collision with existing TP-433 IDs (highest UT: 621, highest IT: 212). Existing suite has duplicate numeric IDs. |
| `remediation_target` nested in `root_cause_analysis` map, not top-level | OpenAPI spec declares `root_cause_analysis` as `additionalProperties: true`; no top-level `remediation_target` field exists. Adding one would break the API contract. |
| Token usage in audit events only, not API response | Per docs/tests/435/TEST_PLAN.md: "tokens are HAPI-internal", "not AA CRD status", "not HAPI HTTP API response" |
| LLM metrics via business logic wrapper, not HTTP middleware | BR-HAPI-011: "Metrics incremented in business logic (not middleware)"; BR-HAPI-301: "Metrics recorded in business logic (LLM client wrapper)" |
| Signal mode defaults to "reactive" when empty | Backward compatibility with existing callers that do not send `signal_mode` |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code in affected packages (enrichment detection logic, prompt rendering, parser, token accumulator, instrumented client)
- **Integration**: >=80% of integration-testable code in affected packages (investigator flow, enrichment I/O, metrics emission)
- **E2E**: Deferred to existing E2E-KA-433-001..009 (parity tests already validate end-to-end)

### 5.2 Two-Tier Minimum

Every business requirement covered by at least UT + IT:
- **Unit tests**: Catch logic errors in detection, parsing, accumulation, prompt rendering
- **Integration tests**: Catch wiring errors across enricher/investigator/handler boundaries

### 5.3 Business Outcome Quality Bar

Tests validate **business outcomes**: "operator gets correct detected labels in incident response", "audit trail includes token usage for cost analysis", "proactive signal produces proactive-style investigation prompt". Not "function X is called".

### 5.4 Pass/Fail Criteria

**PASS** -- all of the following:
1. All P0 tests pass (0 failures)
2. Per-tier code coverage meets >=80% on affected packages
3. No regressions in existing TP-433 test suite
4. `detected_labels` field populated in ogen `IncidentResponse` matching OpenAPI schema

**FAIL** -- any of the following:
1. Any P0 test fails
2. Coverage below 80% on any affected tier
3. Existing TP-433 tests that were passing now fail
4. OpenAPI contract violation (ogen validation error on response)

### 5.5 Suspension & Resumption Criteria

**Suspend when**:
- Build broken -- code does not compile
- More than 3 tests fail for the same root cause -- stop and investigate
- ogen type mismatch blocks `detected_labels` wiring (R-PAR-4)

**Resume when**:
- Build fixed and green on CI
- Root cause identified and fix deployed
- ogen types aligned with enrichment struct

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O) -- target >=80%

| Package | Key Files | Approx Lines |
|---------|-----------|-------------|
| `internal/kubernautagent/enrichment/` | `enricher.go` (detection logic) | ~50 new |
| `internal/kubernautagent/prompt/` | `builder.go` (signal mode wiring) | ~10 changed |
| `internal/kubernautagent/parser/` | `parser.go` (`parseLLMFormat` nested target) | ~15 changed |
| `internal/kubernautagent/audit/` | `emitter.go` (token fields) | ~20 new |
| `pkg/kubernautagent/llm/` | `instrumented.go` (new file, metrics wrapper) | ~80 new |
| `internal/kubernautagent/investigator/` | `investigator.go` (token accumulator) | ~30 new |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component) -- target >=80%

| Package | Key Files | Approx Lines |
|---------|-----------|-------------|
| `internal/kubernautagent/investigator/` | `investigator.go` (enrichment -> prompt -> response flow) | ~300 existing |
| `internal/kubernautagent/enrichment/` | `enricher.go` (K8s client calls for label detection) | ~150 existing + ~50 new |
| `internal/kubernautagent/server/` | `handler.go` (`mapInvestigationResultToResponse` wiring) | ~100 existing |
| `pkg/kubernautagent/llm/` | `instrumented.go` (wrapper around real client) | ~80 new |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.3` HEAD | Post-rebase on `origin/main` (v1.2.0-rc3) |
| TP-433 (parent) | v1.3 | 119 scenarios, 69 UT + 42 IT + 8 E2E |
| TP-433-WIR (wiring) | v1.0 | 33 scenarios (all passing) |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-SP-101 | Detected labels auto-detection (10 characteristics) | P0 | Unit | UT-KA-433-DL-001..006 | ✅ Pass |
| BR-SP-101 | Detected labels auto-detection | P0 | Integration | IT-KA-433-DL-001..003 | ✅ Pass |
| BR-AI-084 | Signal mode prompt strategy (R4: HAPI switches prompt) | P0 | Unit | UT-KA-433-SM-001..003 | ✅ Pass |
| BR-AI-084 | Signal mode prompt strategy | P0 | Integration | IT-KA-433-SM-001..002 | ✅ Pass |
| #435 | Token usage in audit events | P1 | Unit | UT-KA-433-TK-001..003 | ✅ Pass |
| #435 | Token usage in audit events | P1 | Integration | IT-KA-433-TK-001 | ✅ Pass |
| BR-HAPI-011 | Investigation metrics (Prometheus) | P1 | Unit | UT-KA-433-LM-001..006 | ✅ Pass |
| BR-HAPI-301 | LLM observability metrics | P1 | Integration | IT-KA-433-LM-001..002 | ✅ Pass |
| BR-HAPI-261 | LLM-provided affected resource | P0 | Unit | UT-KA-433-RCA-001..004 | ✅ Pass |
| BR-HAPI-265 | Labels in workflow discovery context | P0 | Integration | IT-KA-433-RCA-001..002 | ✅ Pass |
| BR-AI-001 | Prompt cluster context rendering | P1 | Unit | UT-KA-433-PB-001..003 | ✅ Pass |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-KA-433-{SUB}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SUB**: Gap-specific namespace:
  - `DL` -- Detected Labels
  - `SM` -- Signal Mode
  - `TK` -- Token Usage
  - `LM` -- LLM Metrics
  - `RCA` -- Three-Phase RCA Parity
  - `PB` -- Prompt Business Logic

### Tier 1: Unit Tests (17 scenarios)

**Phase P6 -- Prompt Business Logic (3 UT)**

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-433-PB-001` | Operator sees cluster context (namespace, resource annotations) in investigation prompt when signal data is provided | ✅ Pass |
| `UT-KA-433-PB-002` | Operator sees detected_labels section in prompt when `EnrichmentData.DetectedLabels` is populated (canary for P1) | ✅ Pass |
| `UT-KA-433-PB-003` | Prompt omits detected_labels section when `EnrichmentData.DetectedLabels` is nil (guards backward compatibility) | ✅ Pass |

**Phase P1 -- Detected Labels (6 UT)**

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-433-DL-001` | Enricher detects GitOps management (Flux/ArgoCD annotations on root owner) | ✅ Pass |
| `UT-KA-433-DL-002` | Enricher detects HPA presence for the target workload | ✅ Pass |
| `UT-KA-433-DL-003` | Enricher detects PDB protection for the target workload | ✅ Pass |
| `UT-KA-433-DL-004` | Enricher detects Helm management (helm.sh labels/annotations) | ✅ Pass |
| `UT-KA-433-DL-005` | Enricher detects all 10 label fields from a mixed K8s resource set (full parity with Python `LabelDetector`) | ✅ Pass |
| `UT-KA-433-DL-006` | Enricher returns empty `DetectedLabels` when no characteristics are found (zero-value safety) | ✅ Pass |

**Phase P2 -- Signal Mode (3 UT)**

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-433-SM-001` | Prompt builder activates proactive template sections when `SignalMode="proactive"` | ✅ Pass |
| `UT-KA-433-SM-002` | Prompt builder activates reactive template sections when `SignalMode="reactive"` | ✅ Pass |
| `UT-KA-433-SM-003` | Prompt builder defaults to reactive when `SignalMode` is empty or missing (backward compat) | ✅ Pass |

**Phase P3 -- Token Usage (4 UT)**

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-433-TK-001` | Token accumulator correctly sums `PromptTokens` + `CompletionTokens` across multiple LLM calls | ✅ Pass |
| `UT-KA-433-TK-002` | Token accumulator AuditData produces correct map for audit events | ✅ Pass |
| `UT-KA-433-TK-003` | Zero-value accumulator returns zeroes | ✅ Pass |

**Phase P5 -- Three-Phase RCA (3 UT)**

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-433-RCA-001` | Parser extracts `RemediationTarget` from nested `root_cause_analysis` JSON block | ✅ Pass |
| `UT-KA-433-RCA-002` | Parser handles missing remediation_target gracefully | ✅ Pass |
| `UT-KA-433-RCA-003` | Hybrid JSON — flat rca_summary + nested remediation_target | ✅ Pass |
| `UT-KA-433-RCA-004` | camelCase remediationTarget accepted | ✅ Pass |

### Tier 2: Integration Tests (9 scenarios)

**Phase P1 -- Detected Labels (3 IT)**

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-433-DL-001` | Enricher populates `DetectedLabels` via fake K8s client with GitOps+HPA fixtures | ✅ Pass |
| `IT-KA-433-DL-002` | `InvestigationResult` carries `DetectedLabels` through `toPromptEnrichment` to prompt | ✅ Pass |
| `IT-KA-433-DL-003` | Handler populates `detected_labels` field in ogen `IncidentResponse` (OpenAPI contract test) | ✅ Pass |

**Phase P2 -- Signal Mode (2 IT)**

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-433-SM-001` | Investigation with `signal_mode=proactive` in request produces proactive-style prompt content | ✅ Pass |
| `IT-KA-433-SM-002` | Investigation with missing `signal_mode` defaults to reactive behavior (no regression) | ✅ Pass |

**Phase P3 -- Token Usage (1 IT)**

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-433-TK-001` | Investigation flow emits `aiagent.response.complete` audit event with non-zero `total_tokens_used` | ✅ Pass |

**Phase P4 -- LLM Metrics (2 IT)**

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-433-LM-001` | Investigation flow increments `aiagent_api_llm_requests_total` Prometheus counter (business logic side effect) | ✅ Pass |
| `IT-KA-433-LM-002` | Investigation flow updates `aiagent_api_llm_request_duration_seconds` Prometheus histogram (business logic side effect) | ✅ Pass |

**Phase P5 -- Three-Phase RCA (2 IT) -- Note: Also add 3 UT for LLM Metrics**

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-433-RCA-001` | End-to-end investigation produces `root_cause_analysis` map with both `summary` and `remediationTarget` keys | ✅ Pass |
| `IT-KA-433-RCA-002` | Post-RCA detected labels (from P1) appear in Phase 3 workflow selection prompt context (BR-HAPI-265) | ✅ Pass |

### Tier 2 Supplement: LLM Metrics Unit Tests (Phase P4)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-433-LM-001` | `InstrumentedClient` delegates to inner client | ✅ Pass |
| `UT-KA-433-LM-002` | `InstrumentedClient` propagates errors | ✅ Pass |
| `UT-KA-433-LM-003` | `InstrumentedClient` satisfies llm.Client interface | ✅ Pass |
| `UT-KA-433-LM-004` | Prometheus metrics are recorded on success | ✅ Pass |
| `UT-KA-433-LM-005` | Prometheus error metric incremented on failure | ✅ Pass |
| `UT-KA-433-LM-006` | Duration histogram records observations | ✅ Pass |

### Tier Skip Rationale

- **E2E**: Not applicable for this supplement. Existing E2E-KA-433-001..009 cover full-stack parity. New features (detected labels, signal mode) will be validated end-to-end when E2E tests run against the updated KA image.

---

## 9. Test Cases

### UT-KA-433-DL-001: Detect GitOps Management

**BR**: BR-SP-101, ADR-056 v1.7
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/enrichment/detected_labels_test.go`

**Preconditions**:
- Fake K8s client loaded with a Deployment owned by a ReplicaSet with `app.kubernetes.io/managed-by: argocd` annotation

**Test Steps**:
1. **Given**: K8s resource with ArgoCD management annotation on root owner
2. **When**: `detectLabels(ctx, k8sClient, kind, name, namespace)` is called
3. **Then**: `DetectedLabels.GitOpsManaged == true` and `DetectedLabels.GitOpsTool == "argocd"`

**Acceptance Criteria**:
- **Behavior**: Detection returns correct GitOps flags
- **Correctness**: Both `GitOpsManaged` and `GitOpsTool` populated
- **Accuracy**: Tool name matches annotation value

### UT-KA-433-SM-001: Proactive Prompt Rendering

**BR**: BR-AI-084 R4
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/prompt/signal_mode_test.go`

**Preconditions**:
- Prompt builder initialized with embedded templates

**Test Steps**:
1. **Given**: `SignalData` with `SignalMode = "proactive"` and valid signal fields
2. **When**: `builder.RenderInvestigation(signalData, enrichData)` is called
3. **Then**: Rendered prompt contains proactive-specific text (e.g., "proactive monitoring" or "no active incident")

**Acceptance Criteria**:
- **Behavior**: Template `{{ if eq .SignalMode "proactive" }}` branch is exercised
- **Correctness**: Proactive sections present, reactive-only sections absent
- **Accuracy**: Prompt content matches expected proactive template output

### UT-KA-433-TK-001: Token Accumulation

**BR**: #435
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/token_usage_test.go`

**Preconditions**:
- `TokenAccumulator` initialized

**Test Steps**:
1. **Given**: Fresh `TokenAccumulator`
2. **When**: `Add(TokenUsage{PromptTokens: 100, CompletionTokens: 50})` called twice
3. **Then**: `Total()` returns `TokenUsage{PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300}`

**Acceptance Criteria**:
- **Behavior**: Accumulator sums correctly across calls
- **Correctness**: `TotalTokens = PromptTokens + CompletionTokens`

### UT-KA-433-RCA-001: Nested RemediationTarget Parsing

**BR**: BR-HAPI-261
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/parser/remediation_target_test.go`

**Preconditions**:
- Parser initialized

**Test Steps**:
1. **Given**: LLM JSON response with `root_cause_analysis: { summary: "...", remediationTarget: { kind: "Deployment", name: "web", namespace: "prod" } }`
2. **When**: `parser.Parse(response)` is called
3. **Then**: `result.RemediationTarget` has `Kind="Deployment"`, `Name="web"`, `Namespace="prod"`

**Acceptance Criteria**:
- **Behavior**: Nested `remediationTarget` extracted from `root_cause_analysis` block
- **Correctness**: All 3 fields (kind, name, namespace) populated
- **Accuracy**: Values match LLM output exactly

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `fake.NewClientBuilder()` for K8s (detected labels), no other mocks needed (pure logic)
- **Location**: `test/unit/kubernautagent/{enrichment,prompt,investigator,parser,audit,llm}/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks per no-mocks policy. Mock `llm.Client` interface for LLM provider (external dependency). `fake.NewClientBuilder()` for K8s.
- **Infrastructure**: None (no containers, no envtest -- investigator ITs use in-process wiring)
- **Location**: `test/integration/kubernautagent/{enrichment,investigator}/`
- **Prometheus**: `prometheus.NewPedanticRegistry()` for metrics isolation in P4 tests

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| golangci-lint | 1.55+ | Lint validation |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| TP-433 base (119 scenarios) | Code | Merged | Cannot validate no-regression | None -- must be on branch |
| ogen-generated types for `detected_labels` | Code | Present in OpenAPI | IT-KA-433-DL-003 cannot validate | Use raw map assertion |

### 11.2 Execution Order

Strict sequential with checkpoints:

1. **Phase P6**: Prompt business logic assertions (test-only, validates existing behavior)
2. **Phase P1**: Detected labels detection (largest service change, P5 depends on it)
3. **CHECKPOINT P-1**: Build + vet + detected labels end-to-end
4. **Phase P2**: Signal mode / proactive prompts
5. **CHECKPOINT P-2**: Build + proactive/reactive branches verified
6. **Phase P3**: Token usage audit
7. **Phase P4**: LLM Prometheus metrics
8. **CHECKPOINT P-3**: Build + observability complete
9. **Phase P5**: Three-phase RCA parity (depends on P1 for detected labels)
10. **CHECKPOINT P-4 (Final)**: Full build + lint + race + coverage + regression check

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/433/TP-433-PARITY.md` | Strategy, scenarios, audit findings |
| Unit test suite (DL) | `test/unit/kubernautagent/enrichment/detected_labels_test.go` | Label detection UT |
| Unit test suite (SM) | `test/unit/kubernautagent/prompt/signal_mode_test.go` | Signal mode UT |
| Unit test suite (TK) | `test/unit/kubernautagent/investigator/token_usage_test.go` | Token accumulator UT |
| Unit test suite (LM) | `test/unit/kubernautagent/llm/instrumented_client_test.go` | Instrumented client UT |
| Unit test suite (RCA) | `test/unit/kubernautagent/parser/remediation_target_test.go` | Nested target parsing UT |
| Unit test suite (PB) | `test/unit/kubernautagent/prompt/prompt_content_parity_test.go` | Prompt content parity UT |
| Integration test suite (DL) | `test/integration/kubernautagent/enrichment/detected_labels_it_test.go` | Label detection IT |
| Integration test suite (SM) | `test/integration/kubernautagent/investigator/signal_mode_it_test.go` | Signal mode IT |
| Integration test suite (TK) | `test/integration/kubernautagent/investigator/token_audit_it_test.go` | Token audit IT |
| Integration test suite (LM) | `test/integration/kubernautagent/investigator/llm_metrics_it_test.go` | LLM metrics IT |
| Integration test suite (RCA) | `test/integration/kubernautagent/investigator/rca_parity_it_test.go` | RCA parity IT |

---

## 13. Execution

```bash
# All parity unit tests
go test -v -race ./test/unit/kubernautagent/... -ginkgo.focus="433-(DL|SM|TK|LM|RCA|PB)"

# All parity integration tests
go test -v -race ./test/integration/kubernautagent/... -ginkgo.focus="433-(DL|SM|TK|LM|RCA)"

# Specific phase by sub-namespace
go test -v ./test/unit/kubernautagent/... -ginkgo.focus="433-DL"
go test -v ./test/integration/kubernautagent/... -ginkgo.focus="433-DL"

# Full regression (existing + new)
go test -v -race ./test/unit/kubernautagent/...
go test -v -race ./test/integration/kubernautagent/...

# Coverage
go test -coverprofile=coverage-parity-ut.out ./test/unit/kubernautagent/...
go test -coverprofile=coverage-parity-it.out ./test/integration/kubernautagent/...
go tool cover -func=coverage-parity-ut.out
go tool cover -func=coverage-parity-it.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `UT-KA-433-017` (`builder_test.go`) | Asserts rendered prompt with hardcoded reactive mode | May need update if `SignalData` struct changes | P2 adds `SignalMode` field to `SignalData` |
| `UT-KA-433-018` (`builder_test.go`) | Asserts enrichment rendering without detected labels | No change needed -- `DetectedLabels` is optional (`map[string]string`) | P1 adds labels but nil case preserved |
| `IT-KA-433-009` (`investigator_test.go`) | Asserts 8 audit event types as side effects | May need update to verify token fields in audit events | P3 adds token data to `aiagent.llm.response` and `aiagent.response.complete` |

---

## 15. Quality Checkpoints

### CHECKPOINT P-1 (After Phase P1 REFACTOR)

| Action | Pass Criteria |
|--------|---------------|
| `go build ./...` | Zero errors |
| `go vet ./...` | Zero warnings |
| UT-KA-433-DL-001..006 all pass | 6/6 green |
| IT-KA-433-DL-001..003 all pass | 3/3 green |
| UT-KA-433-PB-002 now passes (canary) | Was RED in P6, now GREEN |
| Existing UT-KA-433-017..020 still pass | No regression |
| `detected_labels` field in ogen response type-checks | OpenAPI contract satisfied |

### CHECKPOINT P-2 (After Phase P2 REFACTOR)

| Action | Pass Criteria |
|--------|---------------|
| `go build ./...` | Zero errors |
| UT-KA-433-SM-001..003 all pass | 3/3 green |
| IT-KA-433-SM-001..002 all pass | 2/2 green |
| Template `{{ if eq .SignalMode "proactive" }}` exercised | Proactive prompt content verified |
| Existing UT-KA-433-017..020 still pass | Reactive default preserved |

### CHECKPOINT P-3 (After Phase P4 REFACTOR)

| Action | Pass Criteria |
|--------|---------------|
| `go build ./...` | Zero errors |
| UT-KA-433-TK-001..004 all pass | 4/4 green |
| IT-KA-433-TK-001 passes | 1/1 green |
| UT-KA-433-LM-001..003 all pass | 3/3 green |
| IT-KA-433-LM-001..002 all pass | 2/2 green |
| `go test -race ./test/integration/kubernautagent/...` | Zero races |
| Existing IT-KA-433-009 still passes | Audit event structure backward compatible |

### CHECKPOINT P-4 -- Final (After Phase P5 REFACTOR)

| Action | Pass Criteria |
|--------|---------------|
| `go build ./...` | Zero errors |
| `golangci-lint run --timeout=5m` | Zero new lint errors |
| `go test -race ./test/unit/kubernautagent/...` | All pass, zero races |
| `go test -race ./test/integration/kubernautagent/...` | All pass, zero races |
| Per-tier coverage on affected packages | >=80% UT, >=80% IT |
| `grep -r "KA-433-" test/ \| sort \| uniq -d` | Zero duplicate IDs |
| All 26 new scenarios mapped in BR Coverage Matrix (Section 7) | Complete traceability |
| TP-433 TEST_PLAN.md cross-reference updated | Includes TP-433-PARITY |

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial parity test plan. 6 phases, 26 scenarios (17 UT + 9 IT). 12 audit findings incorporated. Sub-namespaced test IDs to avoid collision with TP-433 (F1). |
