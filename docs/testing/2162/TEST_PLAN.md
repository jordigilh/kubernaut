# Test Plan: `gateway.enabled` Helm Chart Toggle

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2162-v1.0
**Feature**: Add a `gateway.enabled` toggle to the Helm chart so the Gateway component
(Deployment/Service/RBAC/NetworkPolicy/Ingress) can be fully disabled, mirroring the existing
`apifrontend.enabled` pattern — Gateway and APIFrontend are independent, complementary ingress
points into the same `RemediationRequest` pipeline, not one superseding the other.
**Version**: 1.0
**Created**: 2026-08-16
**Author**: AI agent (Cursor) + jgil
**Status**: Complete
**Branch**: `feat/2162-gateway-enabled-toggle`

---

## 1. Introduction

### 1.1 Purpose

Prior to this change, the Helm chart had no way to disable Gateway on its own — unlike
`apifrontend.enabled`/`console.enabled`, which already support this. This test plan proves that
the new `gateway.enabled` toggle (a) correctly prunes every Gateway-owned and Gateway-referencing
resource when disabled, (b) is fully independent of `apifrontend.enabled`, and (c) does not
regress the chart's single-install guard (BR-PLATFORM-004), which previously relied on Gateway's
`ClusterRole` always existing as an existence canary.

### 1.2 Objectives

1. **Structural correctness**: `helm template` with `gateway.enabled=false` renders zero
   Gateway-owned documents (`gateway.yaml`, `ingress.yaml`, `networkpolicy.yaml`,
   `rbac/gateway-signal-source-rbac.yaml`) and prunes exactly the Gateway-specific
   cross-service RBAC bindings/PDB, leaving every sibling service's resources unaffected.
2. **Independence**: `gateway.enabled` and `apifrontend.enabled` can be toggled in any
   combination (both true, either alone, both false) with no cross-dependency; `console.enabled`
   continues to depend only on `apifrontend.enabled`.
3. **Live-cluster pruning proof**: `helm upgrade --set gateway.enabled=false` on a real
   cluster actually removes the cluster-scoped `ClusterRole`/`ClusterRoleBinding` objects
   (Helm's manifest-diff-and-prune, not just `helm template`'s static rendering).
4. **No regression to the single-install guard**: swapping its existence canary from
   `gateway-role` to `authwebhook` does not change its observable behavior (still blocks a
   second install; still a no-op under `helm template`).
5. **Zero regression**: all 61 pre-existing helm-unittest suites (532 tests) continue to pass
   unmodified, except the one (`networkpolicies_unconditional_test.yaml`) whose assertion was
   itself about the exact behavior this issue changes.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| helm-unittest pass rate | 100% | `helm unittest charts/kubernaut/` |
| New structural test coverage | 13 new tests | `charts/kubernaut/tests/gateway_enabled_test.yaml` |
| Live-cluster smoke test pass rate | 100% | `scripts/helm-smoke-test.sh` (`ST-CHART-RBAC-PRUNE-004`) |
| Backward compatibility | 0 regressions | 61 pre-existing suites unmodified except 1 intentional update |
| `helm lint --strict` | 0 errors/warnings | `helm lint charts/kubernaut/` |

---

## 2. References

### 2.1 Authority (governing documents)

- [BR-PLATFORM-005](../../requirements/BR-PLATFORM-005-helm-chart-operator-security-parity.md) FR-7 (added by this issue)
- Issue #2162: Helm chart: add gateway.enabled toggle for full Gateway component parity with apifrontend.enabled/console.enabled
- `charts/kubernaut/templates/apifrontend/apifrontend.yaml` (the `$v.enabled` gate pattern mirrored here)
- `charts/kubernaut/templates/infrastructure/singleinstallguard.yaml` (BR-PLATFORM-004, DD-018 — canary swap)

### 2.2 Cross-References

- Companion issue #2159 (verified pruning of *existing* toggles; this issue *adds* a new one) —
  its `ST-CHART-RBAC-PRUNE-00{1,2,3}` smoke tests had not yet merged to `main` at the time this
  branch was cut, so `ST-CHART-RBAC-PRUNE-004` here may need renumbering at merge time if
  ordering differs from expected.

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Single-install guard's `gateway-role` canary stops existing once Gateway is optional, silently defeating BR-PLATFORM-004 | High (security-relevant: a second install could go undetected) | High (certain, once anyone sets `gateway.enabled=false`) | `ST-CHART-GUARD-001` (existing, unmodified — proves behavior preserved after canary swap) | Re-pointed the guard's `lookup` at `ClusterRole/authwebhook` (unconditional, no toggle) instead |
| R2 | Cross-service RBAC bindings to Gateway's ServiceAccount (`fmc-scope-check-client-rbac.yaml`, `datastorage-rbac.yaml`) dangle-reference a nonexistent SA when Gateway is disabled | Medium (RBAC hygiene / least-privilege gap, not an active over-grant) | High (certain, without a fix) | `gateway_enabled_test.yaml` (2 new tests) | Gated both bindings on `gateway.enabled` |
| R3 | `pdb.yaml`'s opt-in skip-list uses a raw `$component.enabled` read, which is `nil`/falsy for any field not explicitly set — silently wrong for `gateway`/`apifrontend` (default `true`), unlike `console`/`fleetmetadatacache` (default `false`) where the same raw read coincidentally matches intent | High (would silently skip rendering the PDB by default) | High (certain, if implemented naively) | `gateway_enabled_test.yaml` (2 new tests: PDB dropped when disabled, PDB present by default) | Resolved via `kubernaut.mergedValues` (schema-default-aware) instead of a raw field read |
| R4 | `gateway.enabled=false` breaks Console (if a hidden dependency existed) | Low (would be a functional regression) | Low (preflight found none) | `gateway_enabled_test.yaml` independence tests | Preflight confirmed `console.enabled` depends only on `apifrontend.enabled`; no schema/template edit touches that dependency |

### 3.1 Risk-to-Test Traceability

All four risks (R1–R4) have direct test coverage, per the table above. R1 is the highest-impact
risk in this change and is the only one requiring live-cluster verification (`lookup` is a no-op
under `helm template`) — covered by the pre-existing, unmodified `ST-CHART-GUARD-001`.

---

## 4. Scope

### 4.1 Features to be Tested

- **`templates/gateway/gateway.yaml`** (whole-file gate): ServiceAccount, ClusterRole,
  2×ClusterRoleBinding, ext-CRBs, Role/RoleBinding, Deployment, Service — all pruned together.
- **`templates/gateway/ingress.yaml`, `templates/gateway/networkpolicy.yaml`**: each gated on
  `gateway.enabled`, mirroring `apifrontend`'s equivalent files exactly.
- **`templates/rbac/gateway-signal-source-rbac.yaml`**: gated on `gateway.enabled`.
- **`templates/rbac/fmc-scope-check-client-rbac.yaml`**, **`templates/rbac/datastorage-rbac.yaml`**:
  the specific Gateway-SA-binding documents within these multi-document files are gated; sibling
  bindings for other services are proven unaffected.
- **`templates/pdb.yaml`**: `gateway` (and, opportunistically, `apifrontend`) added to the
  opt-in skip-list, resolved via `kubernaut.mergedValues` rather than a raw field read.
- **`templates/infrastructure/singleinstallguard.yaml`**: existence canary swapped from
  `gateway-role` to `authwebhook`.
- **`values.schema.json`**: new `gateway.enabled` boolean property, default `true`.

### 4.2 Features Not to be Tested

- **`kubernaut-operator`'s `GatewaySpec.Enabled`**: a separate repo/team's responsibility;
  out of scope entirely (confirmed with the requester).
- **`templates/gateway/servicemonitor.yaml`**: deliberately left ungated on `gateway.enabled`,
  consistent with `apifrontend/servicemonitor.yaml`'s identical, pre-existing, accepted gap
  (gated only on `monitoring.serviceMonitor.enabled` + CRD presence).

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Swap the single-install guard's canary to `ClusterRole/authwebhook` (not `data-storage-auth-middleware`) | Smaller, single-purpose object; no toggle planned for AuthWebhook either; approved by requester over the DataStorage alternative |
| Add `apifrontend` to `pdb.yaml`'s skip-list opportunistically in this same PR, even though its gap pre-dates this issue | Same file, same fix shape, same root cause (raw-vs-merged `enabled` read); approved by requester rather than filing a separate follow-up |
| No new `gatewayNotDisabledRequirement` schema guard (unlike `apifrontendNotDisabledRequirement`) | Preflight confirmed nothing schema-depends on Gateway always existing — `console.enabled` depends only on `apifrontend.enabled` |

---

## 5. Approach

### 5.1 Coverage Policy

This is a Helm chart change, not Go application code — the "unit" and "integration" tiers below
map to helm-unittest (structural, offline rendering) and a live-cluster smoke test (the only way
to exercise Helm's actual manifest-diff-and-prune behavior, which `helm template` cannot do)
respectively, per the established pattern for this chart's RBAC-lifecycle testing (companion
issue #2159).

- **Structural (helm-unittest)**: 100% of new/modified gate conditions covered by at least one
  positive (renders) and one negative (pruned) assertion.
- **Live-cluster (smoke test)**: 100% of new cluster-scoped RBAC objects proven to actually prune
  via a real `helm upgrade`, not just statically absent from a `helm template` render.

### 5.2 Two-Tier Minimum

Every gate added in this change has both a structural test (proves the template logic is
correct) and, where the object is cluster-scoped, a live-cluster test (proves Helm's upgrade
path actually removes the previously-applied object — `helm template` alone cannot prove this,
since it has no live-cluster state to diff against).

### 5.4 Pass/Fail Criteria

**PASS**:

1. All new and pre-existing helm-unittest tests pass (0 failures).
2. `ST-CHART-RBAC-PRUNE-004` and `ST-CHART-GUARD-001` pass live.
3. `helm lint --strict` and `helm template` (both default and `gateway.enabled=false`) succeed
   with 0 errors.
4. No regressions in the 61 pre-existing helm-unittest suites, except the one intentional update.

**FAIL**: any of the above is not met.

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**: the live Kind cluster used for `scripts/helm-smoke-test.sh` cannot be
provisioned, or a pre-existing, unrelated flake (e.g. the `kubernaut-console` image not being
built by CI, a known issue independent of this change) blocks the full flow from completing.

**Resume when**: the blocking condition is resolved, or the specific new assertions
(`ST-CHART-RBAC-PRUNE-004`) are confirmed to run and pass in isolation despite an unrelated
flake elsewhere in the flow.

---

## 6. Test Items

### 6.1 Structural (helm-unittest, offline rendering)

| File | Scope | Lines (approx) |
|------|-------|-----------------|
| `charts/kubernaut/templates/gateway/gateway.yaml` | Whole-file `$v.enabled` gate | ~400 |
| `charts/kubernaut/templates/gateway/ingress.yaml` | `and $v.enabled .Values.gateway.ingress.enabled` | ~50 |
| `charts/kubernaut/templates/gateway/networkpolicy.yaml` | `$v.enabled` gate | ~57 |
| `charts/kubernaut/templates/rbac/gateway-signal-source-rbac.yaml` | `$v.enabled` gate | ~40 |
| `charts/kubernaut/templates/rbac/fmc-scope-check-client-rbac.yaml` | Gateway-specific CRB gate | ~110 |
| `charts/kubernaut/templates/rbac/datastorage-rbac.yaml` | Gateway-specific RoleBinding gate | ~250 |
| `charts/kubernaut/templates/pdb.yaml` | Skip-list + effective-enabled resolution | ~53 |
| `charts/kubernaut/values.schema.json` | New `gateway.enabled` property | 1 |

### 6.2 Live-Cluster-Testable (Helm's manifest-diff-and-prune, requires a real cluster)

| File | Scope |
|------|-------|
| `charts/kubernaut/templates/infrastructure/singleinstallguard.yaml` | Canary swap (`lookup` is a no-op under `helm template`) |
| Gateway's cluster-scoped RBAC (`gateway-role`, `gateway-rolebinding`, `gateway-view`) | Proven pruned via real `helm upgrade`, not just statically absent |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `feat/2162-gateway-enabled-toggle` HEAD | Branched from `origin/main` |
| Companion issue #2159 | Open PR #2161, unmerged at time of writing | `ST-CHART-RBAC-PRUNE-00{1,2,3}` not present in this branch's baseline |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-PLATFORM-005 FR-7 | `gateway.enabled` gates Gateway's full resource set | P0 | Structural | `gateway_enabled_test.yaml` (13 tests) | Pass |
| BR-PLATFORM-005 FR-7 | Independence from `apifrontend.enabled`/`console.enabled` | P0 | Structural | `gateway_enabled_test.yaml` (3 independence tests) | Pass |
| BR-PLATFORM-005 FR-7 | Gateway's NetworkPolicy owning-service gate | P1 | Structural | `networkpolicies_unconditional_test.yaml` (2 tests, moved Group A→B) | Pass |
| BR-PLATFORM-005 FR-7 / BR-PLATFORM-004 | Cluster-scoped RBAC pruning on live `helm upgrade` | P0 | Live smoke | `ST-CHART-RBAC-PRUNE-004` | Pass |
| BR-PLATFORM-004 | Single-install guard unaffected by canary swap | P0 | Live smoke | `ST-CHART-GUARD-001` (pre-existing, unmodified) | Pass |

### Status Legend

- **Pass**: Implemented and passing (all scenarios in this plan are at this status — this is a
  retrospective plan documenting a completed implementation, not a forward-looking one).

---

## 8. Test Scenarios

### Test ID Naming Convention

- Structural: no formal Test Scenario ID scheme exists for helm-unittest in this chart; tests
  are identified by their `it:` description string within `gateway_enabled_test.yaml`.
- Live smoke: `ST-CHART-{AREA}-{SEQUENCE}`, per `scripts/helm-smoke-test.sh`'s existing convention.

### Tier 1: Structural (helm-unittest)

| Test (in `gateway_enabled_test.yaml`) | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| "gateway.yaml renders ... by default" | Zero regression: Gateway still deploys out of the box | Pass |
| "gateway.enabled=false renders zero documents from gateway.yaml" | Disabling Gateway removes its entire resource set atomically | Pass |
| "gateway.enabled=false renders zero documents from ingress.yaml, even when gateway.ingress.enabled=true" | Ingress can't leak past the component-level gate | Pass |
| "gateway.enabled=false renders zero documents from networkpolicy.yaml" | NetworkPolicy owning-service gate works | Pass |
| "gateway.enabled=false renders zero documents from rbac/gateway-signal-source-rbac.yaml" | Signal-source RBAC doesn't dangle | Pass |
| "fmc-scope-check-client-rbac.yaml renders 5 documents when FMC enabled + gateway default" | Baseline document count established before the negative test | Pass |
| "gateway.enabled=false drops exactly the gateway-fmc-scope-check-client binding ... (5→4)" | Only Gateway's binding is pruned; RO's sibling binding is untouched | Pass |
| "gateway.enabled=false drops exactly the gateway-data-storage-client RoleBinding ... (14→13)" | Cross-service RBAC hygiene | Pass |
| "gateway.enabled=false drops exactly the gateway PodDisruptionBudget ... (11→10)" | PDB doesn't dangle for a nonexistent Deployment | Pass |
| "gateway PodDisruptionBudget renders by default" | Zero regression for the default case | Pass |
| "... disabling gateway does not affect apifrontend" | Independence (direction 1) | Pass |
| "... disabling apifrontend does not affect gateway" | Independence (direction 2) | Pass |
| "both gateway.enabled=false and apifrontend.enabled=false simultaneously renders zero documents for both" | Independence (both-off case; neither is a hard dependency of the other) | Pass |

### Tier 2: Live-Cluster Smoke Test

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `ST-CHART-RBAC-PRUNE-004` | `helm upgrade --set gateway.enabled=false` on a real cluster actually removes `clusterrole/gateway-role`, `clusterrolebinding/gateway-rolebinding`, `clusterrolebinding/gateway-view` — proves Helm's manifest-diff-and-prune, which `helm template` cannot exercise | Pass |
| `ST-CHART-GUARD-001` (pre-existing, unmodified) | Single-install guard still blocks a second install after its canary was swapped from `gateway-role` to `authwebhook` | Pass |

### Tier Skip Rationale

- **Go unit/integration/E2E tests**: not applicable — this change is entirely Helm chart
  templates/schema/tests, with zero Go code touched (confirmed in preflight: no controller
  code has a hard dependency on Gateway's runtime presence).

---

## 9. Test Cases (representative — full detail in Section 8)

### `gateway.enabled=false drops exactly the gateway PodDisruptionBudget out of pdb.yaml`

**BR**: BR-PLATFORM-005 FR-7
**Priority**: P0
**Type**: Structural (helm-unittest)
**File**: `charts/kubernaut/tests/gateway_enabled_test.yaml`

**Preconditions**: none (offline rendering).

**Test Steps**:
1. **Given**: the chart's default values plus `gateway.enabled=false`.
2. **When**: `templates/pdb.yaml` is rendered.
3. **Then**: exactly 10 documents render (down from 11 at default), and none is named `gateway`.

**Expected Results**:
1. Document count is exactly 10.
2. No dangling PDB for a nonexistent Deployment.

**Acceptance Criteria**:
- **Behavior**: the skip-list correctly resolves `gateway.enabled` via `kubernaut.mergedValues`,
  not a raw (and therefore always-`nil`/falsy-for-a-true-default) field read.
- **Correctness**: exactly one document is removed, no others.

**Dependencies**: none.

---

## 10. Environmental Needs

### 10.1 Structural Tests

- **Framework**: `helm-unittest` (Helm plugin), not Ginkgo — this is a Helm chart, not Go code;
  the project's Ginkgo/BDD mandate applies to business-logic Go tests, and this chart's own
  established convention (61 pre-existing suites) already uses helm-unittest exclusively.
- **Mocks**: none — offline template rendering only, no external dependencies.
- **Location**: `charts/kubernaut/tests/`

### 10.2 Live-Cluster Smoke Test

- **Infrastructure**: Kind cluster (or any real Kubernetes cluster with `kubectl`/`helm` access).
- **Location**: `scripts/helm-smoke-test.sh`
- **Resources**: standard Kind smoke-test resource footprint (unchanged by this PR).

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Helm | 3.x | `helm template`, `helm lint`, `helm upgrade` |
| `helm unittest` plugin | 1.1.1 (confirmed installed) | Structural test runner |
| `shellcheck` | any recent | Smoke test script lint |
| Kind | any recent | Live-cluster smoke test |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|--------------------------|------------|
| Issue #2159 / PR #2161 | Code | Open, unmerged | `ST-CHART-RBAC-PRUNE-004`'s numbering may need adjustment at merge time | Documented in the function's own comment; trivial rename, no logic change |

### 11.2 Execution Order

1. **Phase 1 (RED-equivalent)**: preflight blast-radius investigation (this is a Helm chart, so
   there is no failing-test-first step in the Go TDD sense — the "RED" here was proving, via
   read-only `helm template` dry runs, exactly which files needed to change).
2. **Phase 2 (GREEN)**: schema + template gating changes, made minimal and directly mirroring
   `apifrontend.enabled`'s proven pattern.
3. **Phase 3 (REFACTOR)**: N/A — no follow-up cleanup was needed; the GREEN implementation was
   already the final, minimal, correct form (single-purpose `$v.enabled` gates, no duplication
   introduced).
4. **Phase 4 (WIRING VERIFICATION)**: `gateway_enabled_test.yaml`'s 13 tests + regenerated
   `_generated_defaults.tpl`/`helm-values-reference.md` (CI-gated).
5. **Phase 5 (E2E-equivalent)**: `ST-CHART-RBAC-PRUNE-004` live-cluster smoke test.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|--------------|
| This test plan | `docs/testing/2162/TEST_PLAN.md` | Strategy and test design |
| Structural test suite | `charts/kubernaut/tests/gateway_enabled_test.yaml` | 13 new helm-unittest tests |
| Modified test suite | `charts/kubernaut/tests/networkpolicies_unconditional_test.yaml` | Gateway NetworkPolicy moved Group A→B, +1 net test |
| Live smoke test | `scripts/helm-smoke-test.sh` (`run_rbac_prune_004`) | Proves live-cluster RBAC pruning |

---

## 13. Execution

```bash
# Structural tests (all suites)
helm unittest charts/kubernaut/

# Structural tests (just this feature)
helm unittest charts/kubernaut/ -f "tests/gateway_enabled_test.yaml"

# Lint
helm lint charts/kubernaut/ --set postgresql.auth.existingSecret=dummy ...

# Live smoke test (requires a Kind cluster)
scripts/helm-smoke-test.sh --platform kind --chart-path charts/kubernaut/ ...
```

---

## 14. Wiring Verification (TDD Phase 4)

| Code Path | Entry Point | Exit Point | Wiring Test | Status |
|-----------|-------------|------------|-------------|--------|
| `gateway.enabled` schema field | `values.schema.json` | `kubernaut.mergedValues "service" "gateway"` → `$v.enabled` | `gateway_enabled_test.yaml` (13 tests) | Pass |
| Single-install guard canary | `templates/infrastructure/singleinstallguard.yaml` | `lookup "ClusterRole" "authwebhook"` | `ST-CHART-GUARD-001` (live) | Pass |
| Live RBAC pruning | `helm upgrade --set gateway.enabled=false` | `kubectl get clusterrole/gateway-role` (absent) | `ST-CHART-RBAC-PRUNE-004` (live) | Pass |

---

## 15. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|---------------------|--------------------|-------------------|--------|
| `networkpolicies_unconditional_test.yaml:63-67` ("gateway NetworkPolicy always renders (no enabled toggle exists anymore)") | Group A: unconditional render, `count: 1` | Moved to Group B: split into a default-renders test + a `gateway.enabled=false` → `count: 0` test | The premise ("no enabled toggle exists anymore") is now false; the test's own name would be misleading if left unchanged, even though it happened to still pass under default values |

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-08-16 | Initial test plan (written retrospectively alongside the completed implementation) |
