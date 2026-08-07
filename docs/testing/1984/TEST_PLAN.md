# Test Plan: Helm Chart Schema-Level Input Validation (+ ADR-068 OAuth2 Gap Closure)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1984-v1.0
**Feature**: Move Helm chart mandatory-ness/cross-field enforcement from template `fail()` guards
to `values.schema.json` (draft-07 `anyOf`/`if`-`then`), and close the ADR-068 gap where
`global.fleet.oauth2.enabled=false` permits unauthenticated MCP Gateway access for 7 of 8
fleet-capable services.
**Version**: 1.0
**Created**: 2026-08-06
**Author**: AI Agent (Cursor)
**Status**: 🚧 In Progress
**Branch**: `fix/1984-schema-required-fields`

---

## 1. Introduction

### 1.1 Purpose

`charts/kubernaut/templates/**/*.yaml` enforces 19 mandatory/cross-field constraints via
`{{- fail "..." }}` guards across 12 files. `fail()` only triggers during template rendering —
after `values.schema.json` validation has already passed — so schema-only tooling (`helm lint
--strict` without an install, IDE validators) cannot see these constraints, and operators find out
about a bad config later than necessary. This plan moves every guard that validates **raw input**
(not a `default`/lookup-derived value) into `values.schema.json`, and additionally closes a
newly-discovered gap: `global.fleet.oauth2.enabled` defaults `false` for 7 of 8 fleet-capable
services, permitting unauthenticated MCP Gateway access in contradiction of ADR-068's "OAuth2
mandatory, no fallback" mandate. FleetMetadataCache (FMC) already implements the correct pattern
unconditionally; this plan generalizes it.

### 1.2 Objectives

1. **Schema hardening (Phases A–C)**: policy-content `anyOf`, Fleet MCP Gateway endpoint
   requirement, cross-component dependency (`console`↔`apifrontend`,
   `kubernautAgent.interactive`↔`apifrontend`) and console leaf-field requirements, and the
   `fleetmetadatacache.enabled=false` conflict guard all move from `fail()` to
   `values.schema.json`, with the corresponding template guards removed.
2. **OAuth2 mandatory-when-MCP-Gateway-used (Phase D)**: `global.fleet.oauth2.{enabled: true,
   tokenURL, credentialsSecretRef}` becomes schema-required whenever
   `global.fleet.mcpGatewayEndpoint` is set (shared block, covers GW/RO/AF/EM/SP/KA/FMC), plus a
   dedicated no-fallback block for WorkflowExecution's own `credentialsSecretRef`. Mirrored in Go
   `Validate()` for GW/RO/AF/EM (`pkg/fleet/config.go`), SP, WE, and KA as defense in depth.
3. **Zero regression**: every legitimate existing configuration (E2E harness, example values,
   CI schema-consistency job) continues to render/validate successfully.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/fleet/... ./pkg/signalprocessing/... ./pkg/workflowexecution/... ./internal/kubernautagent/...` |
| Helm-unittest pass rate | 100% | `helm unittest charts/kubernaut/` |
| `helm lint --strict` | Clean | `helm lint charts/kubernaut/ --strict` |
| Zero-override `helm template` | Renders successfully | `helm template charts/kubernaut/` |
| Backward compatibility | 0 regressions in unrelated pre-existing cases | Full `helm unittest` + `go test` suites green |

---

## 2. References

### 2.1 Authority (governing documents)

- [BR-PLATFORM-010](../../requirements/BR-PLATFORM-010-helm-chart-schema-level-input-validation.md):
  Helm Chart Schema-Level Input Validation (+ ADR-068 OAuth2 Enforcement)
- [ADR-068](../../architecture/decisions/ADR-068-fleet-federation-architecture.md): Fleet
  Federation Architecture — Goal (line 22), Boundary 1 auth table (line 614+), line 792
- Issue #1984 (schema hardening), Issue #1991 (ADR-068 `oauth2.enabled=false` gap)
- FedRAMP/NIST 800-53: SI-10, CM-6, IA-2, AC-3, AC-17
- Prior art: `docs/testing/1686/TEST_PLAN.md` (helm-unittest convention this plan extends),
  DD-PLATFORM-004 (chart default hardening), DD-PLATFORM-006 (configuration surface reduction —
  origin of most of the cross-component `fail()` guards touched here)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- `pkg/fleet/fmc/config/config.go` — reference pattern: OAuth2 required unconditionally, no
  `enabled` toggle
- `charts/kubernaut/templates/_helpers.tpl` (`kubernaut.fleet.oauth2`, `kubernaut.fleet.config`,
  `kubernaut.fleet.preamble`) — existing Go-template helpers whose *raw-input* preconditions this
  plan pushes into schema
- Technical spikes (this session, ephemeral — not committed to `docs/architecture/spikes/` since
  their sole purpose was mechanism validation, not a design decision requiring a durable record):
  confirmed draft-07 `anyOf`/`if`-`then` behavior against real `helm lint`/`helm template`/`helm
  unittest`, including nested `const` several levels deep and multiple independent `if`/`then`
  blocks in one `allOf`. Confirmed Helm's schema validator does not support the custom
  `errorMessage` keyword (generic `failedTemplate: {}` only) and silently ignores
  `dependentRequired` on draft-07.

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Phase D schema breaks pre-existing happy-path `helm-unittest` cases that set `mcpGatewayEndpoint` without a complete `oauth2` block | High — CI regression across `fleet_config_consolidation_test.yaml`, `fleetmetadatacache_effective_enabled_test.yaml`, `workflowexecution_fleet_wiring_test.yaml` | High (confirmed during preflight) | All 3 files' full suites re-run after Phase D GREEN | Full `helm unittest charts/kubernaut/` run after every schema change (not just the new cases) catches every regressed case; each is fixed by adding the now-mandatory `oauth2` fields to its `set:` block, or (where the case's entire premise was "renders successfully with oauth2 disabled") converting it to a `failedTemplate: {}` rejection case |
| R2 | Moving a `fail()` guard whose condition is post-`default`/lookup-derived (not raw input) to schema silently breaks it, since schema validates before `default`/lookup logic runs | Medium — guard becomes permanently unreachable or incorrectly rejects valid input | Low (each candidate guard individually inspected before moving) | Phase C's `fleetmetadatacache.enabled` guard specifically | Guard's condition (`global.fleet.backend` defaulting to `"fleetmetadatacache"` when unset) is expressible in schema via `if`/`then` with `then: false` plus a `not`/`enum` check on the raw (pre-default) `backend` field — validated against the `helm template` output, not assumed |
| R3 | The Go `Validate()` flip (Phase D Go) breaks a legitimate existing deployment that sets an MCP-Gateway-endpoint field without OAuth2 | High — startup failure for real deployments | Low (preflight found the only live-cluster E2E harness that exercises this path already sets OAuth2 unconditionally; no example/CI config relies on the old behavior) | `UT-FLEET-CFG-061` and SP/WE/KA equivalents (flipped), full `go build ./...` | Preflight audit of every real usage site (E2E harness, `values-airgap.yaml`, `examples/`, CI `helm lint --strict` job) confirmed none rely on `mcpGatewayEndpoint`-set + `oauth2.enabled=false` |
| R4 | Schema-vs-Go gate condition divergence: schema gates on `global.fleet.mcpGatewayEndpoint`, Go gates on each service's own endpoint field — these must be the same value by construction | Medium — a future per-service endpoint override could silently bypass one side | Low (`mcpGatewayEndpoint` confirmed schema-global-only, no per-service key exists) | Phase D schema + Go test suites both pass against the same fixture values | `git grep "mcpGatewayEndpoint" values.schema.json` confirms it is a schema property exactly once (`global.fleet.mcpGatewayEndpoint`); no per-service override exists for the gates to diverge on |
| R5 | `credentialsSecretRef`'s per-service-override-with-global-fallback pattern cannot be exactly replicated in schema (schema sees raw input, not the post-merge resolved value) | Low — schema is stricter than the Go/template runtime behavior for this one field | Certain (accepted design simplification) | Phase D schema tests assert the stricter (superset) requirement | Explicitly documented design simplification (see plan): schema requires `global.fleet.oauth2.credentialsSecretRef` unconditionally when the endpoint is set, treating per-service overrides as additive. Strictly safer than status quo; flagged for reviewer sign-off, not a functional gap |

### 3.1 Risk-to-Test Traceability

All five risks (R1–R5) have a directly mapped test or full-suite regression gate; no coverage
gaps. R1 in particular is mitigated by treating "run the full pre-existing `helm-unittest` suite,
not just new cases" as a mandatory step of every phase's GREEN, not just Phase D's.

---

## 4. Scope

### 4.1 Features to be Tested

- **`values.schema.json`**: new `anyOf` (Phase A), `if`/`then` (Phases B–D) blocks.
- **12 Helm template files**: 19 `fail()` guard call sites removed (see enumeration in the plan;
  full list re-verified via `git grep "{{- fail" -- charts/kubernaut/templates` before each
  removal).
- **Go `Validate()`**: `pkg/fleet/config.go` (`FleetConfig.Validate()`, covers GW/RO/AF/EM),
  `pkg/signalprocessing/config/config.go`, `pkg/workflowexecution/config/config.go`,
  `internal/kubernautagent/config/config.go` (`validateFleetIntegration()`).

### 4.2 Features Not to be Tested

- Guards validating post-`default`/lookup-derived values, which remain as `fail()` (see BR-PLATFORM-010
  Non-Goals): TLS `certManager.issuerRef.name` auto-discovery, monitoring URL auto-discovery,
  `networkPolicies.apiServerCIDR` auto-discovery, `llmProfileRef` referential integrity,
  KubernautAgent phase-name enum, per-`llmProfile` OAuth2, any live-cluster lookup guard.
- The Kubernaut Operator's equivalent validation (out of scope; separate codebase).
- FMC's own `Validate()` — already correct (reference pattern), not modified.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| One shared `global.fleet.oauth2` `if`/`then` block (not 7 near-identical per-service blocks) | Spike-validated (`/tmp/schema-spike2`) that a single `if`/`then` keyed on `global.fleet.mcpGatewayEndpoint` correctly gates the shared `oauth2` fields for every consumer; mirrors the chart's existing `kubernaut.fleet.oauth2` DRY helper philosophy |
| WorkflowExecution keeps a second, independent `if`/`then` block (not folded into the shared one) | WE's `credentialsSecretRef` is deliberately non-fallback-able (least privilege, AC-6) — a structurally different requirement, not a variation of the shared one; spike-validated that two independent `if`/`then` blocks in the same `allOf` fire correctly together |
| Schema requires `global.fleet.oauth2.credentialsSecretRef` unconditionally (not "either global or per-service") when the endpoint is set | JSON Schema validates raw input before Helm's template-side fallback merge runs, so the post-merge resolved value is invisible to schema; requiring the global field unconditionally is a strictly safer superset of the actual runtime requirement |
| Go-side gate condition is each service's own MCP-Gateway-endpoint field, not `global.fleet.enabled` | A GW/RO deployment can enable fleet for scope-checking only (Backend+Endpoint against ACM/FMC) without ever calling the MCP Gateway; OAuth2 is irrelevant in that case, so gating on `enabled` alone would over-reject |
| `helm-unittest` (not a new Go-based harness) | Already the chart's established convention (DD-PLATFORM-001..004); this plan only adds cases to it |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit (Go)**: `Validate()` methods in `pkg/fleet`, `pkg/signalprocessing/config`,
  `pkg/workflowexecution/config`, `internal/kubernautagent/config` — pure logic, 100% of the new
  branches covered by Ginkgo `It()` cases.
- **Helm (requirements-based, not line-coverage)**: every new schema constraint proven both
  positively (valid config renders) and negatively (`failedTemplate: {}` on the invalid shape) via
  `helm-unittest`.

### 5.2 Two-Tier Minimum

Go wiring: UT only — these are pure validation functions with no I/O; the "wiring" proof for the
Go side is `Validate()` already being called from each service's `cmd/*/main.go` startup path
(pre-existing wiring, not new — this plan only changes the function bodies). Helm: covered by
`helm-unittest` only, per the same rationale as `docs/testing/1686/TEST_PLAN.md` §5.2 (no
meaningful unit tier below chart-template rendering; existing `helm-smoke-test.yml` Kind install
is the regression net for the E2E tier, not a new test written for this issue).

### 5.4 Pass/Fail Criteria

**PASS**: all new UT/`helm-unittest` cases pass; `helm lint --strict` clean; zero-override `helm
template` renders; the **entire** pre-existing `helm-unittest` and `go test` suites remain green
(any pre-existing case that legitimately needs updating per R1 is updated, not skipped/deleted
without justification).

**FAIL**: any new or existing test regresses without an explicit, documented reason; any
`fail()` guard removed without an equivalent schema constraint proven to reject the same invalid
shape.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|--------------------|-----------------|
| `pkg/fleet/config.go` | `FleetConfig.Validate()` (new OAuth2-mandatory-when-endpoint-set branch) | ~8 |
| `pkg/signalprocessing/config/config.go` | `Config.Validate()` (same) | ~6 |
| `pkg/workflowexecution/config/config.go` | `Config.Validate()` (same) | ~6 |
| `internal/kubernautagent/config/config.go` | `validateFleetIntegration()` (same) | ~6 |

### 6.2 Integration-Testable Code (Helm chart, tested via helm-unittest)

| File | Constraint | Lines (approx) |
|------|------------|-----------------|
| `charts/kubernaut/values.schema.json` | Phase A `anyOf` (policies) | ~20 |
| `charts/kubernaut/values.schema.json` | Phase B `if`/`then` (mcpGatewayEndpoint required) | ~25 |
| `charts/kubernaut/values.schema.json` | Phase C `if`/`then` × 3 (console deps, KA interactive dep, FMC-enabled conflict) | ~60 |
| `charts/kubernaut/values.schema.json` | Phase D `if`/`then` × 2 (shared oauth2, WE dedicated) | ~40 |
| 12 template files | 19 `fail()` guard removals | ~60 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID(s) | Status |
|-------|-------------|----------|------|------------|--------|
| BR-PLATFORM-010 FR-1 | Policy content `anyOf` schema-enforced (aianalysis, signalprocessing) | P1 | Helm | IT-HELM-1984-001..004 | ⏳ Pending |
| BR-PLATFORM-010 FR-2 | Fleet MCP Gateway endpoint schema-required for GW/RO/FMC | P1 | Helm | IT-HELM-1984-010..015 | ⏳ Pending |
| BR-PLATFORM-010 FR-3 | Cross-component deps + console leaf fields + FMC-enabled conflict | P1 | Helm | IT-HELM-1984-020..032 | ⏳ Pending |
| BR-PLATFORM-010 FR-4 | OAuth2 mandatory-when-MCP-Gateway-used (schema, shared + WE dedicated) | P0 | Helm | IT-HELM-1984-040..052 | ⏳ Pending |
| BR-PLATFORM-010 FR-5 | OAuth2 mandatory-when-MCP-Gateway-used (Go, GW/RO/AF/EM/SP/WE/KA) | P0 | Unit | UT-FLEET-CFG-061 (flipped), UT-FLEET-CFG-090+, UT-SP-CFG-0xx, UT-WE-CFG-0xx, UT-KA-CFG-0xx | ⏳ Pending |
| BR-PLATFORM-010 FR-6 | Existing test suite updated for the new schema surface (no false regressions) | P0 | Helm | Full `helm unittest charts/kubernaut/` re-run | ⏳ Pending |

Exact test IDs are assigned during each phase's RED step, following the file's existing per-area
numbering convention (see `docs/testing/1686/TEST_PLAN.md`'s changelog precedent for why
placeholder IDs above may shift slightly once written).

---

## 8. Test Scenarios

### Tier 1: Unit Tests (Go `Validate()`)

| ID (placeholder) | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `UT-FLEET-CFG-061` (flip) | `FleetConfig.Validate()` now **rejects** `mcpGatewayEndpoint` set + `oauth2.enabled=false` (previously accepted) | Phase D Go |
| `UT-FLEET-CFG-09x` (new) | `FleetConfig.Validate()` accepts `oauth2.enabled=false` when `mcpGatewayEndpoint` is empty (Backend/Endpoint-only scope-check deployment, OAuth2 irrelevant) | Phase D Go |
| `UT-SP-CFG-0xx` (flip+new) | SP `Config.Validate()` mirrors the above for `Fleet.Endpoint` | Phase D Go |
| `UT-WE-CFG-0xx` (flip+new) | WE `Config.Validate()` mirrors the above for `Fleet.Endpoint` | Phase D Go |
| `UT-KA-CFG-0xx` (flip+new) | KA `validateFleetIntegration()` mirrors the above for `Integrations.Fleet.Endpoint` | Phase D Go |

### Tier 2: Helm Chart Tests (helm-unittest)

| ID (placeholder) | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `IT-HELM-1984-001/002` | aianalysis/signalprocessing: neither `policies.content` nor `existingConfigMap` set → schema rejects (`failedTemplate: {}`) | Phase A |
| `IT-HELM-1984-003/004` | aianalysis/signalprocessing: exactly one of `policies.content`/`existingConfigMap` set → renders successfully | Phase A |
| `IT-HELM-1984-010..012` | GW/RO/FMC: fleet-enabled (or FMC effectively-enabled) without `global.fleet.mcpGatewayEndpoint` → schema rejects | Phase B |
| `IT-HELM-1984-013..015` | Same services, `mcpGatewayEndpoint` set → renders successfully | Phase B |
| `IT-HELM-1984-020/021` | `console.enabled=true` + `apifrontend.enabled=false` → schema rejects; both enabled → renders | Phase C |
| `IT-HELM-1984-022..025` | `console.enabled=true` missing `auth.secretName` / OIDC issuer / `ingress.host` → schema rejects each independently | Phase C |
| `IT-HELM-1984-026/027` | `kubernautAgent.interactive.enabled=true` + `apifrontend.enabled=false` → schema rejects; both enabled → renders | Phase C |
| `IT-HELM-1984-028..032` | `fleetmetadatacache.enabled=false` explicit + `global.fleet.enabled=true` + backend resolving to `fleetmetadatacache` → schema rejects; non-contradictory combinations render | Phase C |
| `IT-HELM-1984-040..044` | `global.fleet.mcpGatewayEndpoint` set + `oauth2.enabled=false`/missing `tokenURL`/missing `credentialsSecretRef` → schema rejects (shared block, GW as representative caller) | Phase D schema |
| `IT-HELM-1984-045` | `mcpGatewayEndpoint` set + complete `oauth2` block → renders successfully | Phase D schema |
| `IT-HELM-1984-046..050` | WE: shared block satisfied but `workflowexecution.fleet.oauth2.credentialsSecretRef` unset → schema rejects (dedicated no-fallback block); set → renders | Phase D schema |
| `IT-HELM-1984-051/052` | `mcpGatewayEndpoint` unset → `oauth2` remains fully optional (regression guard, all 8 services) | Phase D schema |

### Tier Skip Rationale

- **E2E**: not applicable as new tests for this issue — the existing `fullpipeline_e2e_helm.go`
  Fleet+MCP-Gateway harness (which already sets OAuth2 unconditionally, per preflight) and
  `helm-smoke-test.yml` serve as the regression net proving the changed chart still installs and
  the Go binaries still start.

---

## 9. Test Cases (P0 detail)

### IT-HELM-1984-040: shared OAuth2 block rejects endpoint-set + oauth2-disabled

**BR**: BR-PLATFORM-010 FR-4
**Priority**: P0
**Type**: Helm (helm-unittest)
**File**: `charts/kubernaut/tests/fleet_oauth2_mandatory_test.yaml` (new)

**Test Steps**:
1. **Given**: `global.fleet.mcpGatewayEndpoint: "http://mcp.example.com"`, `global.fleet.oauth2.enabled` omitted (defaults `false`)
2. **When**: rendering `templates/gateway/gateway.yaml`
3. **Then**: Helm's schema validator rejects the render (`failedTemplate: {}`) before any template `fail()` logic executes.

**Acceptance Criteria**: rejection happens at the schema layer (proven by the absence of the
removed `fail()` guard in the template at the time this test runs — Phase D schema GREEN removes
it).

### UT-FLEET-CFG-061 (flipped): Go rejects endpoint-set + oauth2-disabled

**BR**: BR-PLATFORM-010 FR-5
**Priority**: P0
**Type**: Unit (Ginkgo)
**File**: `pkg/fleet/fleet_test.go`

**Test Steps**:
1. **Given**: `FleetConfig{Enabled: true, MCPGatewayEndpoint: "http://mcp.example.com", OAuth2: FleetOAuth2Config{Enabled: false}}`
2. **When**: calling `Validate()`
3. **Then**: returns a non-nil error identifying that OAuth2 is required.

**Acceptance Criteria**: mirrors FMC's unconditional requirement; error message follows the file's
existing `fmt.Errorf` convention (lowercase-start, `fleet:` prefix).

---

## 10. Environmental Needs

- **Unit**: Ginkgo/Gomega BDD (mandatory), `go test`, no external infra.
- **Helm**: `helm` CLI + `helm-unittest` plugin (`--verify=false` on Helm v4+), no cluster needed.

---

## 11. Dependencies & Schedule

No blocking dependencies. Execution order per phase: RED (failing `helm-unittest`/UT cases,
committed to fail against the current schema/`Validate()`) → GREEN (schema/template/Go change,
full suite re-run to green, including every pre-existing case per R1) → next phase. REFACTOR
(schema `definitions`/`$ref` dedupe) after all 4 phases are GREEN, then final validation gate.

---

## 12. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| This test plan | `docs/testing/1984/TEST_PLAN.md` |
| Business requirement | `docs/requirements/BR-PLATFORM-010-helm-chart-schema-level-input-validation.md` |
| New/updated helm-unittest suites | `charts/kubernaut/tests/policies_schema_validation_test.yaml` (new, Phase A), `charts/kubernaut/tests/fleet_mcp_gateway_endpoint_required_test.yaml` (new, Phase B), `charts/kubernaut/tests/fleet_oauth2_mandatory_test.yaml` (new, Phase D), plus updates to `console_apifrontend_dependency_test.yaml`, `kubernautagent_apifrontend_dependency_test.yaml`, `fleetmetadatacache_effective_enabled_test.yaml`, `workflowexecution_fleet_wiring_test.yaml`, `kubernaut_agent_fleet_wiring_test.yaml`, `fleet_config_consolidation_test.yaml` |
| Updated Go unit tests | `pkg/fleet/fleet_test.go`, `pkg/signalprocessing/config/config_test.go`, `pkg/workflowexecution/config/config_test.go`, `internal/kubernautagent/config/config_test.go` |
| Updated Go source | `pkg/fleet/config.go`, `pkg/signalprocessing/config/config.go`, `pkg/workflowexecution/config/config.go`, `internal/kubernautagent/config/config.go` |
| Updated schema | `charts/kubernaut/values.schema.json` |

---

## 13. Execution

```bash
# Unit
go test ./pkg/fleet/... ./pkg/signalprocessing/... ./pkg/workflowexecution/... ./internal/kubernautagent/...

# Helm
helm unittest charts/kubernaut/
helm lint charts/kubernaut/ --strict
helm template charts/kubernaut/

# Full build
go build ./...
```

---

## 14. Wiring Verification (TDD Phase 4)

This plan tightens existing enforcement rather than introducing new production entry points, so
the Wiring Manifest proves the validation is reachable from the same production paths it already
runs on:

| Validation | Production Entry Point | Enforcement Location | Test ID(s) |
|-----------|-------------|------------|-----------|
| Policy content `anyOf` | `helm template`/`helm install` schema gate | `values.schema.json` | IT-HELM-1984-001..004 |
| Fleet MCP Gateway endpoint required | schema gate | `values.schema.json` | IT-HELM-1984-010..015 |
| Cross-component deps + console leaf fields | schema gate | `values.schema.json` | IT-HELM-1984-020..032 |
| OAuth2 mandatory (schema) | schema gate | `values.schema.json` | IT-HELM-1984-040..052 |
| OAuth2 mandatory (Go, GW/RO/AF/EM) | `cmd/{gateway,remediationorchestrator,apifrontend,effectivenessmonitor}/main.go` startup | `pkg/fleet.FleetConfig.Validate()` | UT-FLEET-CFG-061 (flipped) |
| OAuth2 mandatory (Go, SP) | `cmd/signalprocessing/main.go` startup | `pkg/signalprocessing/config.Config.Validate()` | UT-SP-CFG-0xx |
| OAuth2 mandatory (Go, WE) | `cmd/workflowexecution/main.go` startup | `pkg/workflowexecution/config.Config.Validate()` | UT-WE-CFG-0xx |
| OAuth2 mandatory (Go, KA) | `cmd/kubernautagent/main.go` startup | `internal/kubernautagent/config.validateFleetIntegration()` | UT-KA-CFG-0xx |

---

## 15. Existing Tests Requiring Updates

Confirmed during preflight (full enumeration finalized during each phase's RED step, since the
exact set of now-invalid cases is proven by actually running `helm unittest`, not estimated):

- `workflowexecution_fleet_wiring_test.yaml` — 2 `errorMessage:` assertions → `failedTemplate: {}`;
  1 happy-path case gains a complete `oauth2` block; 1 case ("no oauth2 block when disabled")
  inverts to a rejection case (the exact combination it tested is now invalid by design).
- `kubernaut_agent_fleet_wiring_test.yaml` — 2 `errorMessage:` assertions → `failedTemplate: {}`.
- `console_apifrontend_dependency_test.yaml` — 1 `errorMessage:` assertion → `failedTemplate: {}`.
- `kubernautagent_apifrontend_dependency_test.yaml` — 1 `errorMessage:` assertion →
  `failedTemplate: {}`.
- `fleetmetadatacache_effective_enabled_test.yaml` — 1 `errorMessage:` assertion →
  `failedTemplate: {}`; several happy-path cases gain `oauth2.enabled: true`.
- `fleet_config_consolidation_test.yaml` — 7 happy-path cases (GW/RO backend rendering ×4, SP
  endpoint rendering ×1, FMC gatewayType/valkey-default ×2) gain the now-mandatory `oauth2` fields.

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-08-06 | Initial test plan, written before Phase A implementation begins. |
