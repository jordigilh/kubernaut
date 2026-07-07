# Test Plan: Per-Phase Reasoning/Effort Overrides for `phaseModels`

**Test Plan Identifier**: TP-1616-v1.0
**Feature**: `LLMOverrideConfig.Reasoning` wired through `EffectivePhaseConfig` (KA `phaseModels`) and `AlignmentCheckConfig.EffectiveLLM` (shadow LLM); removal of the dead static `types.LLMConfig.PhaseModels`/`types.LLMOverride` duplicate
**Version**: 1.0
**Created**: 2026-07-07
**Author**: KubernautAgent Team
**Status**: Draft
**Branch**: `fix/1616-phase-reasoning-overrides`

---

## 1. Introduction

### 1.1 Purpose

Validates BR-AI-086 (extended): an operator who configures `phaseModels` to run a phase on a
different provider/model can also tune that phase's `reasoning`/`effort` settings independently
of the base `ai.llm.reasoning` config. Today `LLMOverrideConfig` — the struct actually wired into
KA's per-phase LLM client construction — has no `Reasoning` field, so every phase silently
inherits base reasoning regardless of `phaseModels`. This plan also covers removing a confirmed,
fully dead, parallel duplicate (`types.LLMConfig.PhaseModels`/`types.LLMOverride`) discovered
during preflight, so a second reasoning field isn't added to a struct with zero production callers.

### 1.2 Objectives

1. **Per-phase override correctness**: `EffectivePhaseConfig` applies a phase's `Reasoning`
   override when set, and falls back to base `Reasoning` unchanged when not set (no regression).
2. **Shadow/alignment override correctness**: `AlignmentCheckConfig.EffectiveLLM` applies the
   same merge semantics for `ai.alignmentCheck.llm.reasoning`, so this issue's fix does not
   introduce a *second* dead field.
3. **Validation correctness (fail-closed, SI-10)**: a phase or alignment override's `reasoning`
   is validated against the same effort vocabulary and Anthropic-family contradiction rule as
   the base config, using the override's *effective* provider.
4. **Bug fix**: a phase override that sets *only* `reasoning` is accepted, not rejected as an
   "empty override" (confirmed pre-existing bug in `isEmptyPhaseOverride`).
5. **Wiring proof (Pyramid Invariant)**: the `Reasoning` value set via a phase or alignment
   override actually reaches the real outgoing LLM request, not just an in-memory struct field.
6. **Identity-lock non-interference**: a hot-reload that changes only a phase's `reasoning`/
   `effort` (no provider/model change) is accepted, not rejected by the #1599/DD-LLM-008
   restart-required identity lock.
7. **Dead-code removal**: `types.LLMConfig.PhaseModels`/`types.LLMOverride` (zero production
   callers, confirmed via grep across `cmd/` and `pkg/apifrontend/`) is removed without breaking
   any real functionality.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|---|---|---|
| Unit test pass rate | 100% | `go test ./internal/kubernautagent/config/... ./pkg/shared/types/...` |
| Integration test pass rate | 100% | `go test ./cmd/kubernautagent/...` |
| Backward compatibility | 0 regressions | All existing `UT-AI-1470-*`, `UT-GAP1-*`, `IT-KA-1599-*`, `UT-KA-1329-*` tests pass unmodified |
| Dead-code removal completeness | 0 references | `grep -rn "types.LLMOverride" --include=*.go` returns nothing outside this removal's own diff |
| Build/lint | Clean | `go build ./...`, `golangci-lint run --timeout=5m` |

---

## 2. References

### 2.1 Authority

- [BR-AI-086: Model-Aware LLM Reasoning/Thinking Token Support](../../requirements/BR-AI-086-llm-reasoning-token-support.md)
- [DD-LLM-008: Restart-Required LLM Identity Lock](../../architecture/decisions/DD-LLM-008-restart-required-llm-identity-lock.md)
- [DD-LLM-005: Model-Aware Reasoning/Thinking Token Support](../../architecture/decisions/DD-LLM-005-model-aware-reasoning-support.md)
- Issue #1616 (this plan), related: #1599, #1604, #1601, #1578, #1470

### 2.2 Cross-References

- [Configuration Reference §6.1/§7.1](../../services/kubernaut-agent/configuration-reference.md)
- Existing sibling test files: `internal/kubernautagent/config/config_1470_test.go`, `cmd/kubernautagent/llm_builder_effort_1604_test.go`, `cmd/kubernautagent/reload_callback_1470_test.go`

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|---|---|---|---|---|---|
| R1 | `isEmptyPhaseOverride` isn't updated for the new `Reasoning` field, so a reasoning-only phase override is wrongly rejected as an empty override | Feature is unusable for the exact use case the issue asks for | Certain if not fixed (confirmed via code read, not speculation) | UT-AI-1616-003 | Explicit test asserting a reasoning-only override passes `Validate` |
| R2 | `LLMOverrideConfig` is shared by `phaseModels` and `ai.alignmentCheck.llm`; wiring only one consumer recreates the exact dead-field anti-pattern this issue fixes, on the other | Silent no-op on the alignment path | High if only one path is wired (this is a two-consumer shared type) | UT-AI-1616-006/007, IT-KA-1616-003 | Both `EffectivePhaseConfig` and `EffectiveLLM` wired; wire-level IT proof for both |
| R3 | Effective-provider resolution for override validation (override's own `Provider`, falling back to base) is implemented incorrectly, causing false accepts or false rejects of the Anthropic "none+enabled" contradiction | Medium — either lets an invalid config through, or blocks a valid one | Medium (easy off-by-one: forgetting the fallback) | UT-AI-1616-005 | Test specifically covers override with empty `Provider` inheriting an Anthropic-family base |
| R4 | Effort-vocabulary/contradiction validation logic is duplicated between `pkg/shared/types` (base) and `internal/kubernautagent/config` (override) instead of shared, causing future drift | Low immediate risk, maintenance/drift risk over time | Medium if not refactored | N/A (structural, not test-observable) | REFACTOR extracts `types.ValidateReasoningConfig`, reused by both call sites |
| R5 | Removing `types.LLMOverride`/`PhaseModels` breaks a caller that wasn't found by grep (deepcopy generation, AF, or a downstream tool) | Build break | Low — grep-confirmed zero production callers in both KA and AF | Verification step | `go build ./...` across the full module + `make generate` diff review + repo-wide grep re-check |
| R6 | A hot-reload changing only `reasoning`/`effort` on a phase is incorrectly caught by `validatePhaseIdentity` (over-broad identity comparison) | Feature appears broken for the hot-reload path specifically | Low — `validatePhaseIdentity` already only compares `Provider`/`Model`, not `Reasoning`, by construction | IT-KA-1616-001 | Regression test proves current identity-lock code is unaffected, not just assumed |

### 3.1 Risk-to-Test Traceability

All risks R1-R3 and R5-R6 have at least one dedicated test. R4 is a structural/maintainability
risk with no direct test — mitigated by the REFACTOR step itself, verified by `go build` +
existing base-level reasoning tests (`pkg/shared/types/llm_test.go`) continuing to pass unmodified
against the extracted shared validator.

---

## 4. Scope

### 4.1 Features to be Tested

- **`EffectivePhaseConfig`** (`internal/kubernautagent/config/config_types.go`): phase-level
  `Reasoning` merge precedence.
- **`AlignmentCheckConfig.EffectiveLLM`** (same file): shadow-LLM `Reasoning` merge precedence.
- **`LLMRuntimeConfig.Validate` / `isEmptyPhaseOverride`** (`internal/kubernautagent/config/config.go`):
  reasoning-only override acceptance, effort vocabulary, Anthropic contradiction check.
- **`buildLLMClients` / `reloadSinglePhaseClient`** (`cmd/kubernautagent/bootstrap.go` /
  `llm_builder.go`): end-to-end wiring proof that a phase override's `Reasoning` reaches the real
  outgoing LLM request.
- **`buildAlignmentStack`** (`cmd/kubernautagent/bootstrap.go`): same wiring proof for the shadow
  client.
- **`llmRuntimeReloadCallback` / `validatePhaseIdentity`** (`cmd/kubernautagent/llm_builder.go`):
  non-interference with the #1599 identity lock.
- **`types.LLMConfig`/`types.LLMOverride`** (`pkg/shared/types/llm.go`): removal of the dead
  `PhaseModels` field and `LLMOverride` type.

### 4.2 Features Not to be Tested

- **Operator CRD exposure of `reasoning` for `phaseModels`/`ai.alignmentCheck.llm`**: `phaseModels`
  *itself* is already exposed via the Kubernaut CRD (`kubernaut-operator` [#178](https://github.com/jordigilh/kubernaut-operator/issues/178),
  merged) and propagated into KA's ConfigMap today. What is **not** yet exposed is the new
  `reasoning` sub-field this issue adds — that requires a CRD schema change
  (`LLMSpec.Reasoning`/`LLMPhaseOverrideSpec.Reasoning`) tracked separately as
  `kubernaut-operator` [#211](https://github.com/jordigilh/kubernaut-operator/issues/211) (open).
  Operators using ConfigMap-based config (bypassing the CRD, e.g. `RuntimeConfigMapName`) can use
  this issue's fix today; CRD users need #211 to land first. Cross-linked on both issues.
- **`pkg/apifrontend`'s own `phaseModels` concept**: confirmed (grep) AF has no per-phase concept
  in production code; nothing to change or test there. (AF's *base-level* `Reasoning` already
  works today via the shared `types.LLMConfig.Reasoning` field, wired in #1604 — unaffected by
  this issue either way.)
- **New E2E control objective**: see Section 5.2 — no new E2E test is added.

### 4.3 Design Decisions

| Decision | Rationale |
|---|---|
| Remove `types.LLMConfig.PhaseModels`/`types.LLMOverride` entirely rather than wire or leave it | Confirmed zero production callers in both KA and AF; leaving it would let a second, always-broken duplicate exist alongside the real fix (user-approved decision) |
| Wire `Reasoning` through both `EffectivePhaseConfig` and `EffectiveLLM` | `LLMOverrideConfig` is shared by both consumers; wiring only one recreates this issue's exact complaint on the other (user-approved decision) |
| Extract a shared `types.ValidateReasoningConfig` helper rather than duplicate validation logic | Avoids the Go Anti-Pattern Checklist's duplication concern; both `LLMConfig.Validate` (base) and the new override validation need identical effort-vocabulary/contradiction rules |
| No new E2E test | This issue changes which config produces a `Reasoning` value, not the capture/audit mechanism itself, which is already E2E-proven generically (`E2E-KA-AUDIT-001`) |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: 100% of new/changed unit-testable code (`EffectivePhaseConfig`, `EffectiveLLM`,
  `isEmptyPhaseOverride`, new validation logic).
- **Integration**: 100% of the two wiring points (phase override -> real LLM request, alignment
  override -> real LLM request) plus the identity-lock non-interference regression.
- **E2E**: N/A for this issue — see Section 5.2.

### 5.2 E2E Tier Assessment: N/A (justified)

This issue does not introduce a new SOC2/FedRAMP control objective requiring its own proving
journey. `E2E-KA-AUDIT-001` (`test/e2e/kubernautagent/reasoning_audit_e2e_test.go`) already
proves reasoning-content capture and SOC2 CC8.1 audit reconstruction end-to-end at the
provider-client level, independent of which config path (base vs. per-phase vs. per-alignment
override) produced the `Reasoning` value. This issue only changes *which configuration produces*
that value for a given phase/shadow client — a wiring concern fully covered by IT-KA-1616-002/003
(Section 8), not a new user/control-facing journey.

### 5.3 Two-Tier Minimum

Every objective in Section 1.2 is covered by at least Unit + Integration (see BR Coverage Matrix,
Section 7).

### 5.4 Pass/Fail Criteria

**PASS** — all of the following:
1. All P0 tests pass (UT-AI-1616-003, IT-KA-1616-001, IT-KA-1616-002).
2. All P1/P2 tests pass.
3. `go build ./...` and `golangci-lint run --timeout=5m` are clean.
4. No regressions in `UT-AI-1470-*`, `UT-GAP1-*`, `IT-KA-1599-*`, `UT-KA-1329-*`, or the base-level
   `pkg/shared/types/llm_test.go` reasoning tests.
5. `grep -rn "types.LLMOverride" --include=*.go` returns zero matches after removal.

**FAIL** — any P0 test fails, or any pre-existing test regresses.

### 5.5 Suspension & Resumption Criteria

**Suspend** if `make generate` (controller-gen) produces an unexpected diff beyond the removed
type's deepcopy methods — investigate before proceeding rather than force-committing a stale or
over-broad regeneration.

**Resume** once the generate diff is understood and scoped to exactly the removed type.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|---|---|---|
| `internal/kubernautagent/config/config_types.go` | `EffectivePhaseConfig`, `EffectiveLLM` | ~15 (added) |
| `internal/kubernautagent/config/config.go` | `isEmptyPhaseOverride`, `LLMRuntimeConfig.Validate` | ~15 (added) |
| `pkg/shared/types/llm.go` | `ValidateReasoningConfig` (new, extracted), `LLMConfig.validateReasoning` (simplified); `PhaseModels`/`LLMOverride`/`IsValidPhaseName`/`validatePhaseModels` (removed) | ~40 net |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|---|---|---|
| `cmd/kubernautagent/llm_builder.go` | `buildLLMClientFromConfig` (via phase/alignment merge paths), `parseAndAuthorizeReload`, `validatePhaseIdentity` | 0 (no change; proven via new tests) |
| `cmd/kubernautagent/bootstrap.go` | `buildLLMClients`, `buildAlignmentStack` | 0 (no change; proven via new tests) |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|---|---|---|
| Code under test | HEAD of `fix/1616-phase-reasoning-overrides` | Branch cut from `main` |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|---|---|---|---|---|---|
| BR-AI-086 | Phase override's `Reasoning` wins over base when set | P1 | Unit | UT-AI-1616-001 | Pending |
| BR-AI-086 | Falls back to base `Reasoning` when phase override unset | P1 | Unit | UT-AI-1616-002 | Pending |
| BR-AI-086 | Reasoning-only phase override not rejected as empty | P0 | Unit | UT-AI-1616-003 | Pending |
| BR-AI-086 (SI-10) | Invalid `effort` value rejected on override | P1 | Unit | UT-AI-1616-004 | Pending |
| BR-AI-086 (SI-10) | Anthropic `none`+`enabled` contradiction rejected on override's effective provider | P1 | Unit | UT-AI-1616-005 | Pending |
| BR-AI-086 | Alignment override's `Reasoning` wins over base when set | P2 | Unit | UT-AI-1616-006 | Pending |
| BR-AI-086 | Alignment falls back to base `Reasoning` when unset | P2 | Unit | UT-AI-1616-007 | Pending |
| BR-AI-086 / DD-LLM-008 | Reasoning-only reload not rejected by #1599 identity lock | P0 | Integration | IT-KA-1616-001 | Pending |
| BR-AI-086 | Phase override `Reasoning` reaches the real outgoing LLM request | P0 | Integration | IT-KA-1616-002 | Pending |
| BR-AI-086 | Alignment override `Reasoning` reaches the real outgoing LLM request | P1 | Integration | IT-KA-1616-003 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}` — `AI` for config-package unit logic (matches
`UT-AI-1470-*`/`UT-AI-1604-*` precedent), `KA` for `cmd/kubernautagent` integration tests
(matches `IT-KA-1599-*` precedent).

### Tier 1: Unit Tests

**Testable code scope**: `internal/kubernautagent/config` (config_types.go, config.go),
`pkg/shared/types/llm.go`

| ID | Business Outcome Under Test | Phase |
|---|---|---|
| UT-AI-1616-001 | Operator's per-phase reasoning tuning takes effect | Pending |
| UT-AI-1616-002 | Phases without a reasoning override keep base behavior (no regression) | Pending |
| UT-AI-1616-003 | Operator can set a reasoning-only phase override without a spurious validation error | Pending |
| UT-AI-1616-004 | Operator gets an immediate, clear error for a malformed `effort` value on an override | Pending |
| UT-AI-1616-005 | Operator gets an immediate, clear error for a contradictory Anthropic reasoning config on an override | Pending |
| UT-AI-1616-006 | Operator can independently tune the shadow/alignment-checker's reasoning | Pending |
| UT-AI-1616-007 | Shadow/alignment-checker without a reasoning override keeps base behavior | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `cmd/kubernautagent` (llm_builder.go, bootstrap.go)

| ID | Business Outcome Under Test | Phase |
|---|---|---|
| IT-KA-1616-001 | Operator can hot-reload a phase's reasoning tuning without triggering an unnecessary restart-required rejection | Pending |
| IT-KA-1616-002 | A phase's reasoning override actually changes what KA sends to that phase's real LLM provider | Pending |
| IT-KA-1616-003 | The alignment-checker's reasoning override actually changes what KA sends to the shadow LLM provider | Pending |

### Tier 3: E2E Tests

Not applicable — see Section 5.2.

### Tier Skip Rationale

- **E2E**: no new SOC2/FedRAMP control objective introduced; existing `E2E-KA-AUDIT-001` already
  proves the reasoning-capture-to-audit journey generically, independent of which config path
  produced the value.

---

## 9. Test Cases (P0 detail)

### UT-AI-1616-003: Reasoning-only phase override is accepted

**BR**: BR-AI-086
**Priority**: P0
**Type**: Unit
**File**: `internal/kubernautagent/config/config_1616_test.go`

**Test Steps**:
1. **Given**: an `LLMRuntimeConfig` with a `phaseModels.rca` override that sets only `Reasoning`
   (no `provider`/`model`/`endpoint`/etc.).
2. **When**: `Validate(provider)` is called.
3. **Then**: no error is returned.

**Acceptance Criteria**: `isEmptyPhaseOverride` returns `false` for this override.

---

### IT-KA-1616-001: Reasoning-only reload accepted by the real reload path

**BR**: BR-AI-086 / DD-LLM-008
**Priority**: P0
**Type**: Integration
**File**: `cmd/kubernautagent/reload_callback_1616_test.go`

**Test Steps**:
1. **Given**: a running phase resolver booted with a phase override that has no `Reasoning` set.
2. **When**: `llmRuntimeReloadCallback` is invoked (the real production reload path) with new
   content that adds/changes only that phase's `reasoning.effort`, provider and model unchanged.
3. **Then**: the reload succeeds (no error), and the phase's resolved client reflects the new
   reasoning setting.

**Acceptance Criteria**: `parseAndAuthorizeReload`/`validatePhaseIdentity` do not reject the reload.

---

### IT-KA-1616-002: Phase override reasoning reaches the real wire request

**BR**: BR-AI-086
**Priority**: P0
**Type**: Integration
**File**: `cmd/kubernautagent/llm_builder_1616_test.go`

**Test Steps**:
1. **Given**: a base config and a `phaseModels` override for one phase with `Reasoning.Effort`
   set to a value different from the base.
2. **When**: the phase client is built through `buildLLMClients`/`reloadSinglePhaseClient` and a
   chat request is sent to an `httptest.Server`.
3. **Then**: the request body received by the test server carries the phase-specific effort
   value, not the base's.

**Acceptance Criteria**: wire-level assertion, not just a struct-field assertion — proves
CHECKPOINT W (production dispatch path exercised end to end).

---

*(P1/P2 test cases summarized in Section 8; full Given/When/Then specs omitted for this issue's
size — each follows the same shape as the P0 cases above, substituting the specific field/path
under test.)*

---

## 10. Environmental Needs

### 10.1 Unit Tests
- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: none (pure config/merge logic, no external dependencies)
- **Location**: `internal/kubernautagent/config/`, `pkg/shared/types/`

### 10.2 Integration Tests
- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: ZERO mocks for internal logic; `httptest.Server` stands in for the external LLM
  provider (matches existing `llm_builder_effort_1604_test.go` pattern)
- **Location**: `cmd/kubernautagent/`

### 10.3 Tools & Versions
| Tool | Minimum Version | Purpose |
|---|---|---|
| Go | per `go.mod` | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| controller-gen | per `Makefile` `CONTROLLER_TOOLS_VERSION` | Regenerate deepcopy after type removal |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None — self-contained within `internal/kubernautagent/config`, `pkg/shared/types`, and
`cmd/kubernautagent`.

### 11.2 Execution Order (with estimated durations)

1. **Phase 1 (RED)** — ~1.5-2h: write UT-AI-1616-001..007, IT-KA-1616-001..003 against the
   current code; confirm all fail for the expected reason (missing field / missing wiring / bug).
2. **Phase 2 (GREEN)** — ~1-1.5h: add `Reasoning` field, wire `EffectivePhaseConfig` and
   `EffectiveLLM`, fix `isEmptyPhaseOverride`, add validation, remove dead type, regenerate
   deepcopy. All RED tests pass.
3. **Phase 3 (REFACTOR)** — ~0.5-1h: extract `types.ValidateReasoningConfig`; update doc comments
   referencing #1616/DD-LLM-008. Re-run full test suite to confirm behavior-preserving.
4. **Phase 4 (WIRING VERIFICATION)** — included in Phase 1/2 by construction (IT tests are
   written against the real production entry points, not added after the fact).
5. **Phase 5 (Docs)** — ~0.5h: configuration-reference.md §6.1/§7.1, DD-LLM-008 addendum,
   BR-AI-086 new AC.

**Total estimate**: ~4-5 hours.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|---|---|---|
| This test plan | `docs/tests/1616/TEST_PLAN.md` | Strategy and test design |
| Implementation plan | `docs/tests/1616/IMPLEMENTATION_PLAN.md` | Phase-by-phase execution detail |
| Unit test suite | `internal/kubernautagent/config/config_1616_test.go`, additions to `config_test.go` | Ginkgo BDD |
| Integration test suite | `cmd/kubernautagent/reload_callback_1616_test.go`, `cmd/kubernautagent/llm_builder_1616_test.go` | Ginkgo BDD |

---

## 13. Execution

```bash
# Unit tests
go test ./internal/kubernautagent/config/... ./pkg/shared/types/... -ginkgo.v

# Integration tests
go test ./cmd/kubernautagent/... -ginkgo.v

# Specific test by ID
go test ./internal/kubernautagent/config/... -ginkgo.focus="UT-AI-1616"
go test ./cmd/kubernautagent/... -ginkgo.focus="IT-KA-1616"
```

---

## 14. Wiring Verification (TDD Phase 4)

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|---|---|---|---|---|
| Phase override `Reasoning` | `buildLLMClients`/`reloadSinglePhaseClient` (boot + hot-reload) | Outgoing LLM HTTP request | IT-KA-1616-002 | Pending |
| Alignment override `Reasoning` | `buildAlignmentStack` | Outgoing shadow LLM HTTP request | IT-KA-1616-003 | Pending |
| Identity-lock non-interference | `llmRuntimeReloadCallback` -> `parseAndAuthorizeReload` -> `validatePhaseIdentity` | Reload accepted/rejected | IT-KA-1616-001 | Pending |

**Unit tests do NOT count as wiring proof.** IT-KA-1616-002/003 traverse the real client-construction
path and assert on the actual outgoing HTTP request body.

---

## 15. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|---|---|---|---|
| `pkg/shared/types/llm_test.go` ~L265-281 | `LLMConfig.PhaseModels` round-trips a `types.LLMOverride` including `provider`/`model` | Remove this test block | `PhaseModels`/`LLMOverride` are being removed (dead code) |
| `pkg/shared/types/llm_test.go` ~L333-347 | Similar `PhaseModels` round-trip variant | Remove | Same |
| `pkg/shared/types/llm_test.go` ~L409-428 ("should deserialize reasoning fields on a per-phase LLMOverride from YAML") | Asserts `cfg.PhaseModels["rca"].Reasoning` round-trips | Remove | Same — this was testing the dead field this issue is retiring |

---

## 16. Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-07-07 | Initial test plan |
