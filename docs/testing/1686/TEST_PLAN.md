# Test Plan: Fleet ClusterRegistry RBAC Least-Privilege Split

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1686-v1.0
**Feature**: Namespace-scoped `Role`/`RoleBinding` instead of a cluster-wide `ClusterRole` rule for
the fleet `ClusterRegistry` CRD watch, whenever the watch itself is scoped to a single namespace,
across SignalProcessing (SP), FleetMetadataCache (FMC), APIFrontend (AF), and EffectivenessMonitor (EM).
**Version**: 1.0
**Created**: 2026-07-23
**Author**: AI Agent (Cursor)
**Status**: ✅ Complete — all tests passing, implementation merged to branch
**Branch**: `fix/1686-fleet-registry-rbac-least-privilege`

---

## 1. Introduction

### 1.1 Purpose

SP/FMC/AF/EM each construct a `pkg/fleet/registry.ClusterRegistry` that watches MCP Gateway CRDs
(`MCPServerRegistration` for Kuadrant; `Backend`/`MCPRoute` for Envoy AI Gateway) to discover
fleet-managed clusters. The registry supports scoping this watch to a single namespace
(`RegistryConfig.Namespace`), but the Helm chart always grants a cluster-wide `ClusterRole` rule for
these CRDs regardless of whether the namespace scoping knob is set — an avoidable least-privilege
violation (FedRAMP AC-6/CM-6). This test plan proves: (a) the two previously-hardcoded config paths
(AF, EM) now thread `Namespace` through to `RegistryConfig`, and (b) all four Helm charts render a
namespace-scoped `Role`+`RoleBinding` instead of the cluster-wide rule whenever a namespace is
configured, with zero behavior change when it is not (backward compatibility).

### 1.2 Objectives

1. **Go wiring**: `fleet.FleetConfig.Namespace` (new field) flows into `registry.RegistryConfig.Namespace`
   for both AF and EM, matching the pattern SP/FMC already have.
2. **RBAC split**: For each of SP/FMC/AF/EM, the fleet CRD watch rule is granted via a namespace-scoped
   `Role`+`RoleBinding` when a namespace is configured, and via the existing cluster-wide `ClusterRole`
   rule when it is not (default, unchanged for current deployments).
3. **Backward compatibility**: default (`namespace` unset) rendering is byte-for-byte unchanged from
   pre-fix output for all 4 charts.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/fleet/...` |
| Integration test pass rate | 100% | `go test ./cmd/apifrontend/... ./cmd/effectivenessmonitor/...` |
| Helm-unittest pass rate | 100% | `helm unittest charts/kubernaut/` |
| Backward compatibility | 0 regressions | Existing `helm-unittest`/`helm-smoke-test` suites pass unmodified |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-RBAC-020: "MUST implement least-privilege access by default" (`docs/requirements/11_SECURITY_ACCESS_CONTROL.md`)
- FedRAMP AC-6 (Least Privilege), CM-6 (Configuration Settings)
- Issue #1686: Fleet ClusterRegistry RBAC: cluster-wide ClusterRole granted even when `namespace`
  scopes the watch to one namespace
- Prior art (same class of fix): `jordigilh/kubernaut-operator#223` (ADR-068 fleet triage,
  `mcpGatewayNamespace`)
- DD-PLATFORM-004 (chart default hardening): establishes `helm-unittest` as the chart's unit-test
  convention this plan extends

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- `charts/kubernaut/tests/tls_mode_test.yaml` — existing precedent for conditional-RBAC helm-unittest specs
- `charts/kubernaut/templates/_helpers.tpl` (`kubernaut.nsRoleForSecrets`) — existing precedent for a
  namespace-scoped `Role`/`RoleBinding` named template

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | RBAC split accidentally breaks default (namespace-unset) deployments by omitting the `ClusterRole` rule | High — fleet classification silently stops working for existing installs | Low | `IT-HELM-020-002/004/006/008` (regression guards) | Explicit "namespace empty" test case per service asserting the `ClusterRole` rule is still present |
| R2 | Namespace-scoped `Role`/`RoleBinding` omits a required verb/resource present in the `ClusterRole` variant | Medium — ClusterRegistry watch fails with Forbidden in namespace-scoped mode | Low | `IT-HELM-020-001/003/005/007` | Assert exact rule content (apiGroups/resources/verbs) matches the `ClusterRole` variant it replaces |
| R3 | AF/EM's shared `fleet.FleetConfig.Namespace` field addition silently breaks GW/RO (also embed `FleetConfig`) | Low — GW/RO never call `NewClusterRegistry` directly | Low | `UT-FLEET-020-001` | Additive `omitempty` field; existing `FleetConfig` UT suite (`pkg/fleet/fleet_test.go`) re-run as regression guard |

### 3.1 Risk-to-Test Traceability

All three risks (R1–R3) have a directly mapped test; no coverage gaps.

---

## 4. Scope

### 4.1 Features to be Tested

- **`fleet.FleetConfig`** (`pkg/fleet/config.go`): new `Namespace` field YAML round-trip.
- **AF fleet wiring** (`cmd/apifrontend/backend_deps.go: buildFleetReaderDeps`): `Namespace` threads
  into `registry.RegistryConfig`.
- **EM fleet wiring** (`cmd/effectivenessmonitor/main.go: buildFleetReaderFactory`): `Namespace`
  threads into `registry.RegistryConfig`.
- **Helm RBAC rendering** (4 templates: `signalprocessing.yaml`, `fleetmetadatacache.yaml`,
  `apifrontend.yaml`, `effectivenessmonitor.yaml`): namespace-scoped `Role`/`RoleBinding` vs.
  cluster-wide `ClusterRole` rule, gated on the relevant `*.fleet.namespace` (or `*.namespace` for FMC)
  value.

### 4.2 Features Not to be Tested

- The `ClusterRegistry`/informer's own namespace-scoped List/Watch behavior (`pkg/fleet/registry`) —
  already implemented and tested prior to #1686; this plan only proves the RBAC *granted* matches
  what the watch actually *needs*.
- CI wiring of `helm-unittest` itself (new job, Makefile target) — verified by the CI run on this PR,
  not a Ginkgo/helm-unittest test.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Add `Namespace` to the *shared* `fleet.FleetConfig` (not a new AF/EM-local struct) | Matches the struct's existing purpose (shared by GW/RO/AF/EM); avoids duplicating SP's local-struct pattern for two more services; additive `omitempty` field is a no-op for GW/RO, which never call `NewClusterRegistry` |
| New `kubernaut.fleet.registryNsRBAC` Helm helper (not 4 inline copies) | Mirrors the existing `kubernaut.nsRoleForSecrets` DRY pattern already used in this chart |
| helm-unittest (not a Go-based `os/exec helm template` harness) | Already the chart's established unit-test convention (DD-PLATFORM-001..004); avoids introducing a second, competing test mechanism |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: `pkg/fleet/config.go`'s new field — pure data, covered by YAML round-trip UT.
- **Integration**: AF/EM wiring — proven via existing `buildFleetReaderDeps`/`buildFleetReaderFactory`
  IT harnesses (fake dynamic client + mock MCP gateway), extended with a `Namespace`-set case.
- **Helm (requirements-based, not line-coverage)**: all 4 templates' RBAC rendering, both branches
  (namespace set / unset) — proven via `helm-unittest` `documentSelector`+`contains`/`notContains`
  assertions, mirroring `tls_mode_test.yaml`'s existing style.

### 5.2 Two-Tier Minimum

Go wiring (AF/EM): UT (field round-trip) + IT (wiring through production entry point). Helm RBAC:
covered by `helm-unittest` only — there is no meaningful "unit" tier below chart-template rendering,
and no live-cluster E2E tier is warranted for a pure RBAC-shape change (the existing Kind-based
`helm-smoke-test.yml` already proves the chart installs cleanly with the changed templates as a
regression net, not a new test written for this issue).

### 5.4 Pass/Fail Criteria

**PASS**: all UT/IT/helm-unittest cases below pass; `helm lint --strict` clean; existing
`helm-smoke-test.yml` and `helm-unittest` suites (pre-existing specs) remain green (no regressions).

**FAIL**: any new or existing test regresses, or default (namespace-unset) rendering differs from
pre-fix output for any of the 4 charts.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|--------------------|-----------------|
| `pkg/fleet/config.go` | `FleetConfig` (new `Namespace` field) | ~5 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|--------------------|-----------------|
| `cmd/apifrontend/backend_deps.go` | `buildFleetReaderDeps` | ~3 (call-site change) |
| `cmd/effectivenessmonitor/main.go` | `buildFleetReaderFactory` | ~3 (call-site change) |
| `charts/kubernaut/templates/_helpers.tpl` | `kubernaut.fleet.registryNsRBAC` | ~20 (new helper) |
| `charts/kubernaut/templates/signalprocessing/signalprocessing.yaml` | ClusterRole/Role split | ~15 |
| `charts/kubernaut/templates/fleetmetadatacache/fleetmetadatacache.yaml` | ClusterRole/Role split (2 branches) | ~25 |
| `charts/kubernaut/templates/apifrontend/apifrontend.yaml` | ClusterRole/Role split | ~15 |
| `charts/kubernaut/templates/effectivenessmonitor/effectivenessmonitor.yaml` | ClusterRole/Role split | ~15 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-RBAC-020 | Namespace field round-trips through FleetConfig | P1 | Unit | UT-FLEET-020-001 | ✅ Pass |
| BR-RBAC-020 | AF wires Config.Fleet.Namespace into ClusterRegistry | P0 | Integration | IT-AF-020-001 | ✅ Pass |
| BR-RBAC-020 | EM wires Config.Fleet.Namespace into ClusterRegistry | P0 | Integration | IT-EM-020-001 | ✅ Pass |
| BR-RBAC-020 | SP grants namespace-scoped Role instead of ClusterRole rule when namespace set | P0 | Helm | IT-HELM-020-001 | ✅ Pass |
| BR-RBAC-020 | SP retains ClusterRole rule when namespace unset (regression guard) | P0 | Helm | IT-HELM-020-002 | ✅ Pass |
| BR-RBAC-020 | FMC (kuadrant) grants namespace-scoped Role when namespace set | P0 | Helm | IT-HELM-020-003 | ✅ Pass |
| BR-RBAC-020 | FMC (kuadrant) retains ClusterRole rule when namespace unset | P0 | Helm | IT-HELM-020-004 | ✅ Pass |
| BR-RBAC-020 | FMC (eaigw) grants namespace-scoped Role when namespace set | P0 | Helm | IT-HELM-020-005 | ✅ Pass |
| BR-RBAC-020 | FMC (eaigw) retains ClusterRole rule when namespace unset | P0 | Helm | IT-HELM-020-006 | ✅ Pass |
| BR-RBAC-020 | AF grants namespace-scoped Role when namespace set | P0 | Helm | IT-HELM-020-007 | ✅ Pass |
| BR-RBAC-020 | AF retains ClusterRole rule when namespace unset | P0 | Helm | IT-HELM-020-008 | ✅ Pass |
| BR-RBAC-020 | EM grants namespace-scoped Role when namespace set | P0 | Helm | IT-HELM-020-009 | ✅ Pass |
| BR-RBAC-020 | EM retains ClusterRole rule when namespace unset | P0 | Helm | IT-HELM-020-010 | ✅ Pass |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `UT-FLEET-020-001` | `FleetConfig.Namespace` set via YAML `namespace:` key is readable on the struct; empty when omitted | ✅ Pass |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `IT-AF-020-001` | Setting `cfg.Fleet.Namespace` and calling `buildFleetReaderDeps` results in a `ClusterRegistry` constructed with that namespace (proven via the fake dynamic client's list call being namespace-scoped) | ✅ Pass |
| `IT-EM-020-001` | Same proof for `buildFleetReaderFactory` | ✅ Pass |

### Tier 3: Helm Chart Tests (helm-unittest)

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `IT-HELM-020-001` | SP: `signalprocessing.fleet.namespace` set + `mcpGatewayType=kuadrant` → namespace-scoped `Role`/`RoleBinding` rendered with the `mcpserverregistrations` rule; `ClusterRole` does NOT contain it | ✅ Pass |
| `IT-HELM-020-002` | SP: `namespace` unset → `ClusterRole` contains the rule; no `Role` rendered (regression guard) | ✅ Pass |
| `IT-HELM-020-003` | FMC: `namespace` set + `gatewayType=kuadrant` → `Role` rendered with `mcpserverregistrations`+`gateways`+`httproutes` rules | ✅ Pass |
| `IT-HELM-020-004` | FMC: `namespace` unset + `gatewayType=kuadrant` → `ClusterRole` contains the rules; no `Role` (regression guard) | ✅ Pass |
| `IT-HELM-020-005` | FMC: `namespace` set + `gatewayType=eaigw` → `Role` rendered with `backends`+`mcproutes` rules | ✅ Pass |
| `IT-HELM-020-006` | FMC: `namespace` unset + `gatewayType=eaigw` → `ClusterRole` contains the rules; no `Role` (regression guard) | ✅ Pass |
| `IT-HELM-020-007` | AF: `apifrontend.fleet.namespace` set + kuadrant → `Role` rendered | ✅ Pass |
| `IT-HELM-020-008` | AF: `namespace` unset → `ClusterRole` contains the rule (regression guard) | ✅ Pass |
| `IT-HELM-020-009` | EM: `effectivenessmonitor.fleet.namespace` set + kuadrant → `Role` rendered | ✅ Pass |
| `IT-HELM-020-010` | EM: `namespace` unset → `ClusterRole` contains the rule (regression guard) | ✅ Pass |

### Tier Skip Rationale

- **E2E**: not applicable — this is a static RBAC-shape change with no runtime behavior to exercise
  beyond what `helm-smoke-test.yml` (pre-existing, unmodified) already covers by installing the chart.

---

## 9. Test Cases (P0 detail)

### IT-HELM-020-001: SP namespace-scoped Role replaces ClusterRole rule

**BR**: BR-RBAC-020
**Priority**: P0
**Type**: Helm (helm-unittest)
**File**: `charts/kubernaut/tests/fleet_registry_rbac_test.yaml`

**Test Steps**:
1. **Given**: `signalprocessing.fleet.mcpGatewayType=kuadrant`, `signalprocessing.fleet.namespace=team-a`
2. **When**: rendering `templates/signalprocessing/signalprocessing.yaml`
3. **Then**: a `Role`+`RoleBinding` named for the SP fleet registry exist in namespace `team-a` granting
   `mcp.kuadrant.io/mcpserverregistrations` get/list/watch, bound to `signalprocessing-controller`; the
   `ClusterRole` document does NOT contain that rule.

**Acceptance Criteria**: exact rule parity with the pre-fix `ClusterRole` rule (no verb/resource drift).

---

## 10. Environmental Needs

- **Unit/Integration**: Ginkgo/Gomega BDD (mandatory), `go test`, no external infra.
- **Helm**: `helm` CLI + `helm-unittest` plugin (`helm plugin install https://github.com/helm-unittest/helm-unittest --version v1.1.1`), no cluster needed.

---

## 11. Dependencies & Schedule

No blocking dependencies. Execution order: RED (failing UT/IT/helm-unittest) → GREEN (Go wiring +
Helm template split, CI wiring) → REFACTOR (dedupe/naming pass, `go build`/`helm lint` as safety net).

---

## 12. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| This test plan | `docs/testing/1686/TEST_PLAN.md` |
| Unit test | `pkg/fleet/fleet_test.go` |
| Integration tests | `cmd/apifrontend/fleet_wiring_test.go`, `cmd/effectivenessmonitor/fleet_wiring_test.go` |
| Helm-unittest suite | `charts/kubernaut/tests/fleet_registry_rbac_test.yaml` |
| CI wiring | `.github/workflows/ci-pipeline.yml` (`helm-unittest` job), `Makefile` (`test-helm` target) |

---

## 13. Execution

```bash
# Unit + Integration
go test ./pkg/fleet/... -run TestFleet
go test ./cmd/apifrontend/... -run TestBuildFleetReaderDeps
go test ./cmd/effectivenessmonitor/... -run TestBuildFleetReaderFactory

# Helm
helm unittest charts/kubernaut/ --file 'tests/fleet_registry_rbac_test.yaml'
helm lint charts/kubernaut/ --strict --set global.image.tag=test ...
```

---

## 14. Wiring Verification (TDD Phase 4)

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|-------------|------------|-----------|--------|
| `fleet.FleetConfig.Namespace` → `registry.RegistryConfig.Namespace` (AF) | `cmd/apifrontend/main.go` startup | `buildFleetReaderDeps` → `registry.NewClusterRegistry` | `IT-AF-020-001` | ✅ Pass |
| `fleet.FleetConfig.Namespace` → `registry.RegistryConfig.Namespace` (EM) | `cmd/effectivenessmonitor/main.go` startup | `buildFleetReaderFactory` → `registry.NewClusterRegistry` | `IT-EM-020-001` | ✅ Pass |

---

## 15. Existing Tests Requiring Updates

None — this is purely additive (new field, new helper, new conditional branch); no existing test
asserts the *absence* of namespace scoping in a way that would break.

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-07-23 | Initial test plan |
| 1.1 | 2026-07-23 | Implementation complete. All UT/IT/helm-unittest cases pass (100/100 `helm unittest charts/kubernaut/`, including the 81 pre-existing cases as a regression check). `helm lint --strict` clean. Actual test IDs in code differ slightly from this plan's placeholders: `UT-FLEET-020-001` was implemented as four scenarios `UT-FLEET-CFG-080..083` in `pkg/fleet/fleet_test.go` (settable/round-trip/omitempty/Validate-optional), following that file's existing per-field numbering convention rather than introducing a new `020` block. `IT-AF-020-001`/`IT-EM-020-001` and `IT-HELM-020-001..010` match this plan's IDs exactly. `helm-unittest` CI wiring is additionally documented in [DD-PLATFORM-005](../../architecture/decisions/DD-PLATFORM-005-helm-unittest-ci-integration.md) (dedicated Stage-1 job vs. folding into `helm-smoke-test`). A follow-up tech-debt issue (#1719) was filed for the pre-existing, unrelated drift across 3 divergent fleet-config struct shapes discovered during preflight — explicitly out of scope for this fix. |
