# Implementation Plan: Per-Phase Reasoning/Effort Overrides for `phaseModels`

**Issue**: #1616
**Business Requirement**: [BR-AI-086](../../requirements/BR-AI-086-llm-reasoning-token-support.md) (extended)
**Test Plan**: [TP-1616-v1.0](TEST_PLAN.md)
**Related**: #1599, #1604, #1601, #1578, #1470; [DD-LLM-008](../../architecture/decisions/DD-LLM-008-restart-required-llm-identity-lock.md)
**Branch**: `fix/1616-phase-reasoning-overrides`
**Created**: 2026-07-07

---

## Overview

Two mechanisms, both scoped to `internal/kubernautagent/config` and `cmd/kubernautagent`:

1. Add `Reasoning *types.LLMReasoningConfig` to `LLMOverrideConfig` and wire it through both of
   that type's consumers — `EffectivePhaseConfig` (KA's real, hot-reloadable `phaseModels`) and
   `AlignmentCheckConfig.EffectiveLLM` (shadow/alignment-checker LLM) — so an operator can tune
   reasoning independently per phase and for the shadow LLM.
2. Remove `types.LLMConfig.PhaseModels`/`types.LLMOverride` (`pkg/shared/types/llm.go`) entirely:
   a confirmed, fully dead, parallel duplicate of mechanism 1 with zero production callers in
   either KA or AF.

### Due Diligence Findings (incorporated)

| ID | Finding | Resolution |
|---|---|---|
| F1 | `mergeLLMConfig`/`EffectivePhaseConfig` never touch `Reasoning` at all — `LLMOverrideConfig` doesn't even have the field — so every phase silently inherits base `ai.llm.reasoning` regardless of `phaseModels` | Add `Reasoning` field, wire it in `EffectivePhaseConfig` |
| F2 | `types.LLMConfig.PhaseModels`/`types.LLMOverride` has **zero** non-test callers anywhere — confirmed via grep across `cmd/kubernautagent`, `cmd/apifrontend`, `pkg/apifrontend` — this is broader than the issue's literal text, which only called out the `Reasoning` sub-field as dead | Remove the entire field/type rather than patch only its `Reasoning` sub-field (user-approved) |
| F3 | `LLMOverrideConfig` is shared by two consumers: `phaseModels` (`EffectivePhaseConfig`) and `ai.alignmentCheck.llm` (`EffectiveLLM`). Wiring only the first recreates this issue's exact complaint on the second the moment the field is added | Wire both consumers (user-approved) |
| F4 | `isEmptyPhaseOverride` (`config.go`) does not know about the new `Reasoning` field — a reasoning-only phase override would be rejected as "at least one override field must be set", defeating the whole point of this issue | Update `isEmptyPhaseOverride` to treat `Reasoning != nil` as non-empty |
| F5 | Effort-vocabulary and Anthropic-contradiction validation already exists for the *base* `LLMConfig.Reasoning` (`pkg/shared/types/llm.go` `validateReasoning`), but nothing validates an *override's* `Reasoning` the same way; naive duplication would violate the Go Anti-Pattern Checklist | Extract a shared `types.ValidateReasoningConfig(prefix, r, effectiveProvider)` helper; both base and override validation call it |
| F6 | `types.LLMOverride`'s deepcopy (`pkg/shared/types/zz_generated.deepcopy.go`) is `controller-gen`-generated (`make generate`), not hand-written | Regenerate rather than hand-edit; verify the diff is scoped to exactly the removed type |
| F7 | `validatePhaseIdentity` (#1599/DD-LLM-008) already only compares `Provider`/`Model` when deciding whether a phase's effective identity changed — `Reasoning` was never part of that comparison | No code change needed for the identity lock itself; add a regression test (IT-KA-1616-001) proving this holds, rather than assuming it |
| F8 | No new SOC2/FedRAMP control objective is introduced — `E2E-KA-AUDIT-001` already E2E-proves reasoning capture/audit reconstruction generically, independent of which config path produced the value | No new E2E test; documented explicitly in TEST_PLAN.md Section 5.2 rather than silently omitted |

### Key Design Decisions

- **Remove, don't wire, the dead static duplicate** — `types.LLMOverride`/`PhaseModels` has no
  production callers in either service; wiring it would duplicate a concept that already works
  correctly through `LLMOverrideConfig`.
- **Wire both `LLMOverrideConfig` consumers** — `EffectivePhaseConfig` and `EffectiveLLM` — in the
  same change, since they share the struct being modified.
- **Extract, don't duplicate, reasoning validation** — one `types.ValidateReasoningConfig` helper,
  called from both the base `LLMConfig.Validate` path and the new override validation path.
- **Wiring proof via wire-level assertions, not reload-acceptance alone** — IT-KA-1616-002/003
  assert on the actual outgoing HTTP request body (mirroring `llm_builder_effort_1604_test.go`),
  not just that a hot-reload was accepted.

---

## Phase 1: TDD RED

Per AGENTS.md's Wiring-First TDD Sequence: write the IT tests through the production entry point
first, then the UT tests for the logic behind them. All MUST fail at this point for the right
reason (missing field, missing wiring, or the confirmed pre-existing bug).

### Phase 1.1: Integration tests (written first)

**Files**: `cmd/kubernautagent/reload_callback_1616_test.go` (new, follows
`reload_callback_1470_test.go` pattern), `cmd/kubernautagent/llm_builder_1616_test.go` (new,
follows `llm_builder_effort_1604_test.go`'s `httptest.Server` pattern)

| Test ID | What it asserts | Why it fails today |
|---|---|---|
| IT-KA-1616-001 | A hot-reload changing only a phase's `reasoning`/`effort` (provider/model unchanged) is accepted by `llmRuntimeReloadCallback` | Compiles today (no field yet, so this scenario can't even be expressed until `LLMOverrideConfig.Reasoning` exists — added as RED scaffolding, not GREEN, since nothing consumes it yet) |
| IT-KA-1616-002 | A phase override's `Reasoning.Effort` reaches the real outgoing LLM request via `buildLLMClients`/`reloadSinglePhaseClient` | `EffectivePhaseConfig` doesn't merge `Reasoning`; the override has no such field to set |
| IT-KA-1616-003 | `ai.alignmentCheck.llm.reasoning` reaches the real outgoing shadow-LLM request via `buildAlignmentStack` | `EffectiveLLM` doesn't merge `Reasoning`; the override has no such field to set |

To let these compile (not pass), add the plain `Reasoning *types.LLMReasoningConfig` field to
`LLMOverrideConfig` with zero wiring — the narrow scaffolding carve-out AGENTS.md's REFACTOR
section acknowledges ("a milestone that only adds plain data-type fields with no logic"). This is
not GREEN: nothing yet reads this field in `EffectivePhaseConfig`/`EffectiveLLM`.

### Phase 1.2: Unit tests for the logic behind these wiring points

**Files**: `internal/kubernautagent/config/config_1616_test.go` (new), additions to the existing
`AlignmentCheck EffectiveLLM merge — BR-AI-601` describe block in `config_test.go`

| Test ID | What it asserts | Why it fails today |
|---|---|---|
| UT-AI-1616-001 | `EffectivePhaseConfig` returns the phase override's `Reasoning` when set | Field not merged (scaffolding-only at this point) |
| UT-AI-1616-002 | `EffectivePhaseConfig` falls back to base `Reasoning` when no override | Trivially "passes" today only because the field doesn't exist; once scaffolded, must explicitly assert the fallback branch |
| UT-AI-1616-003 | A reasoning-only phase override passes `Validate` | Fails today: `isEmptyPhaseOverride` doesn't know about `Reasoning`, rejects as empty |
| UT-AI-1616-004 | `Validate` rejects an invalid `effort` value on an override | No validation exists for override `Reasoning` yet |
| UT-AI-1616-005 | `Validate` rejects `effort: none` + `enabled: true` for an Anthropic-family effective provider on an override | Same |
| UT-AI-1616-006 | `EffectiveLLM` applies the alignment override's `Reasoning` when set | Field not merged |
| UT-AI-1616-007 | `EffectiveLLM` falls back to base `Reasoning` when no alignment override | Same as UT-AI-1616-002 for the alignment path |

---

## Phase 2: TDD GREEN (minimal implementation)

1. [`internal/kubernautagent/config/config_types.go`](../../../internal/kubernautagent/config/config_types.go):
   - Add `Reasoning *types.LLMReasoningConfig` field to `LLMOverrideConfig`.
   - `EffectivePhaseConfig`: `if override.Reasoning != nil { staticOut.Reasoning = override.Reasoning }`.
   - `EffectiveLLM`: `if c.LLM.Reasoning != nil { staticOut.Reasoning = c.LLM.Reasoning }`.
2. [`internal/kubernautagent/config/config.go`](../../../internal/kubernautagent/config/config.go):
   - `isEmptyPhaseOverride`: add `&& override.Reasoning == nil` to the empty-check conjunction.
   - `LLMRuntimeConfig.Validate`: for each phase override, compute effective provider
     (`override.Provider`, falling back to the `provider` parameter) and call
     `types.ValidateReasoningConfig` when `override.Reasoning != nil`.
3. [`pkg/shared/types/llm.go`](../../../pkg/shared/types/llm.go):
   - Remove `PhaseModels` field from `LLMConfig`, the `LLMOverride` type, its `IsEmpty` method,
     `IsValidPhaseName`/`validPhaseNames`, `validatePhaseModels`, and its call in `Validate`.
4. Regenerate `pkg/shared/types/zz_generated.deepcopy.go` (`make generate`).

All Phase 1 RED tests pass after this phase. This is the CHECKPOINT W gate: IT-KA-1616-002/003
must pass through the real production entry points before GREEN is declared complete — UT passing
alone is not sufficient (AGENTS.md "UT-Only GREEN" anti-pattern).

---

## Phase 3: TDD REFACTOR

- Extract the effort-vocabulary + Anthropic-contradiction checks currently inline in
  `LLMConfig.validateReasoning` (`pkg/shared/types/llm.go`) into an exported
  `types.ValidateReasoningConfig(prefix string, r *LLMReasoningConfig, effectiveProvider string) error`.
  `LLMConfig.validateReasoning` becomes a thin wrapper calling it with `c.Provider`; the new
  override validation (Phase 2, step 2) calls it with the override's effective provider.
- Update doc comments on `LLMOverrideConfig`, `EffectivePhaseConfig`, and `EffectiveLLM`
  referencing #1616 and clarifying `Reasoning` is a tuning field, not identity (per DD-LLM-008).

**Post-refactor validation** (mandatory, not itself a refactor bullet):
```bash
go build ./...
go test ./internal/kubernautagent/config/... ./pkg/shared/types/... ./cmd/kubernautagent/...
grep -rn "OldFieldName\|types.LLMOverride" --include="*.go" .
```

---

## Phase 4: Documentation

- [`docs/services/kubernaut-agent/configuration-reference.md`](../../services/kubernaut-agent/configuration-reference.md)
  §6.1: add `reasoning` row to the `phaseModels.<phase>` table, noting it is hot-reloadable and
  NOT subject to the identity lock.
- Same doc §7.1: add `reasoning` row to `ai.alignmentCheck.llm` overrides.
- [`DD-LLM-008`](../../architecture/decisions/DD-LLM-008-restart-required-llm-identity-lock.md)
  "What is intentionally unaffected" section: name `reasoning`/`effort` explicitly.
- [`BR-AI-086`](../../requirements/BR-AI-086-llm-reasoning-token-support.md): add an acceptance
  criterion covering per-phase/per-shadow reasoning overrides.

---

## Verification (Definition of Done)

- [x] `go build ./...`
- [x] `go test ./internal/kubernautagent/config/... ./pkg/shared/types/... ./cmd/kubernautagent/...`
- [x] `golangci-lint run --timeout=5m`
- [x] `grep -rn "types.LLMOverride" --include=*.go` returns zero matches
- [x] `controller-gen` deepcopy regeneration diff reviewed and scoped to exactly the removed type's deepcopy methods (`make generate` also runs `ogen`, unrelated to this change; ran the `controller-gen` step directly instead)
- [x] All TEST_PLAN.md Section 7 (BR Coverage Matrix) rows moved from Pending to Pass
- [x] Documentation updates (Phase 4) merged alongside code

## Out of Scope

- **Operator CRD exposure of `reasoning`**: `phaseModels` itself is already exposed via the
  Kubernaut CRD (`kubernaut-operator` [#178](https://github.com/jordigilh/kubernaut-operator/issues/178),
  merged); the new `reasoning` sub-field this issue adds is not yet exposable through the CRD —
  that's tracked separately as `kubernaut-operator` [#211](https://github.com/jordigilh/kubernaut-operator/issues/211)
  (open, cross-linked to kubernaut#1616 and vice versa). #211's own scope also has a gap (AF
  pass-through in `afAgentLLMConfig()`, and base-level `reasoning` not propagated by the operator
  at all) — flagged there, not folded into this PR.
- `pkg/apifrontend` (confirmed no `phaseModels`/per-phase concept in production code; AF's
  base-level `Reasoning` already works via the shared `types.LLMConfig`, unaffected here).
- New E2E test (see TEST_PLAN.md Section 5.2 for justification).
