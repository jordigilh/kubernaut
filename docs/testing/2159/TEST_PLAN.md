# Test Plan: Live-Cluster Verification of Helm RBAC Pruning

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2159-v1.0
**Feature**: Prove, against a real `helm upgrade` on a live Kind cluster, that Helm's
native manifest-diff mechanism actually prunes cluster-scoped RBAC (`ClusterRole`/
`ClusterRoleBinding`) when the feature toggle that produced it is disabled.
**Version**: 1.0
**Created**: 2026-08-15
**Author**: AI agent (Cursor) + jgil
**Status**: Complete
**Branch**: `fix/2159-helm-rbac-prune-verification` (working branch; see §13 for exact name)

---

## 1. Introduction

### 1.1 Purpose

Issue #2159 asked whether the Helm chart's cluster-scoped RBAC objects are actually
removed when the feature toggle that created them is disabled via `helm upgrade`, or
whether they are left behind as orphaned, over-privileged grants (a FedRAMP AC-6
least-privilege concern). The chart has extensive **single-render** coverage via
`helm-unittest` (proving an object renders/doesn't-render for a given values input),
but `helm-unittest` has no release-state/diff concept — it cannot prove an object that
existed in a *previous* release is actually *removed* by an *upgrade*. Only a real
`helm upgrade` against a live cluster (Helm's manifest-diff-and-prune codepath) can
prove that. `scripts/helm-smoke-test.sh` — the only place a real `helm upgrade` runs
against a live Kind cluster in this repo's test suite — had **zero** test cases that
toggle a feature off and assert the corresponding cluster-scoped object is actually
gone via `kubectl get`. This was a confirmed, complete gap.

Preflight (three parallel investigations: chart templates, smoke-test infra, and a
Helm-vs-Operator architecture comparison against `kubernaut-operator`'s unrelated #341
bug) found this to be a **test-coverage gap, not a code bug**: Helm 3's native
`helm upgrade` diffs the previous release's stored manifest against the newly rendered
one (keyed by GVK+name, which naturally covers cluster-scoped kinds) and deletes
anything present-before/absent-now, unless annotated `helm.sh/resource-policy: keep`
(confirmed via `Grep`: zero RBAC templates in this chart carry that annotation). All
three toggle cases named in #2159 were re-verified this way:

| Toggle | Finding |
|---|---|
| `fleetMetadataCache.enabled` | Real, gates 7 cluster-scoped RBAC objects across two templates |
| `gateway.enabled` | **Does not exist** — no `enabled` field for Gateway in `values.schema.json`; its RBAC renders unconditionally. Nothing to test. |
| `apifrontend.enabled` | Real, correctly wired via the `kubernaut.mergedValues` `$v.enabled` pattern (an initial grep-based preflight pass incorrectly flagged this as dead; corrected via Serena pattern search and confirmed by existing passing `helm-unittest` cases and a `helm template` spike showing zero rendered documents when disabled) |
| `additionalClusterRoleBindings` (global + per-service) | Real, list-membership-based rather than boolean — removing a name from the list is the "toggle off" |

No chart template code changes were made anywhere as part of this work — this is a
pure test-only addition.

### 1.2 Objectives

1. **Prove the actual live-cluster prune behavior**, not just the rendered manifest, for
   all three real toggle cases (FMC, APIFrontend, `additionalClusterRoleBindings`), each
   via a positive control (object exists while enabled) followed by the toggle-off
   assertion (object is gone).
2. **Close the #2159 ask precisely**: confirm `gateway.enabled` doesn't exist (nothing to
   test) and that `apifrontend.enabled` needed no code fix, just the missing live-prune
   test — both to be communicated back on the issue (§ "Test Deliverables").
3. **No new CI plumbing**: reuse the already-running Kind cluster / already-installed
   release from earlier in `flow_a_production()` — no new workflow steps, no new Kind
   cluster creation, minimal added CI time.
4. **FedRAMP AC-6 (least privilege) regression check**: a cluster-scoped RBAC grant that
   outlives its feature toggle is a privilege-creep violation; these tests are the
   concrete evidence that does not happen.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| New smoke-test pass rate | 100% | `ST-CHART-RBAC-PRUNE-001/002/003` all `ok` in `scripts/helm-smoke-test.sh` TAP output |
| Full smoke-suite regression | 0 new failures | Full `scripts/helm-smoke-test.sh` run (Flow A + Flow B), same pass count as pre-change plus 3 |
| Added CI time | Sub-minute | 3 `helm upgrade --reuse-values` calls (2m timeout each, real duration ~10-20s) + a handful of `kubectl get` calls |
| Chart/production code changes | 0 | `git diff --stat -- charts/` empty for this change |

---

## 1.4 FedRAMP/SOC2 Control Objective Assessment

**Finding**: this is directly FedRAMP-mapped, unlike a typical bug-fix test plan. AC-6
(least privilege) requires that access grants not outlive their justification. A
cluster-scoped `ClusterRole`/`ClusterRoleBinding` left behind after its owning feature
is disabled is exactly the kind of unbounded, unreviewed privilege-creep AC-6 exists to
prevent — the grant is real (usable by anything bound to the same `ServiceAccount`
name, or discoverable by cluster-wide RBAC audit tooling) even though the feature that
justified it is off. These three tests are the control-objective evidence: they prove,
against a real upgrade, that disabling the feature actually revokes the grant, not just
that the chart *can* render without it.

**Conclusion**: no #2141-style audit-schema gap exists here (this doesn't touch the
audit event pipeline at all) — this is a RBAC-lifecycle/least-privilege verification,
correctly scoped as a live-cluster smoke test rather than a unit/integration test
because the behavior under test (Helm's release-state diff-and-prune) only exists at
the `helm upgrade` layer, not in any Go or template code this repo controls.

---

## 2. References

### 2.1 Authority (governing documents)

- [BR-PLATFORM-005](../../requirements/BR-PLATFORM-005-helm-chart-operator-security-parity.md)
  (extended with new **FR-6**: RBAC lifecycle parity across upgrades — same domain and
  originating triage methodology, Issue #1589, as its existing 5 FRs)
- Issue #2159: verify Helm chart RBAC pruning on toggle-disable
- FedRAMP AC-6 (least privilege)

### 2.2 Cross-References

- `charts/kubernaut/templates/fleetmetadatacache/fleetmetadatacache.yaml`,
  `charts/kubernaut/templates/rbac/fmc-scope-check-client-rbac.yaml` — FMC RBAC
- `charts/kubernaut/templates/apifrontend/apifrontend.yaml` — APIFrontend RBAC,
  `{{- if $v.enabled }}` gate
- `charts/kubernaut/templates/_helpers.tpl` —
  `kubernaut.additionalClusterRoleBindings`, `kubernaut.fleetmetadatacache.effectiveEnabled`
- `charts/kubernaut/tests/networkpolicies_unconditional_test.yaml`,
  `console_apifrontend_dependency_test.yaml`,
  `kubernautagent_apifrontend_dependency_test.yaml` — pre-existing single-render
  `helm-unittest` coverage that `apifrontend.enabled=false` disables dependent behavior
  (referenced as corroborating evidence that no code fix was needed)
- `scripts/helm-smoke-test.sh` — `run_mon_003` (the nearest pre-existing analog: toggles
  autoscaling on/off, but never asserts the HPA is actually deleted — the gap this plan
  closes for RBAC)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|-----------------|------------|
| R1 | The live test reveals a real pruning gap, contradicting the architecture-based preflight confidence | A real fix would be needed, out of the original "test-only" scope | Low | All three | Per plan §"Risks and Mitigations": stop, do not attempt an ad-hoc fix — this would be a CHECKPOINT DD trigger requiring evidence-based escalation before deciding a fix approach. Not triggered: all three passed on the first live run. |
| R2 | Enabling `global.fleet.oauth2.enabled=true` (required by FMC's `values.schema.json` for `run_rbac_prune_001`'s positive control) cascades to every service's shared fleet-preamble helper, not just FMC, causing `kubernautagent`/`signalprocessing-controller`/`workflowexecution-controller` pods to mount a projected Secret volume for `credentialsSecretRef` that doesn't exist, hanging those pods in `ContainerCreating` for the rest of the flow | Collateral test failures unrelated to RBAC pruning itself | Confirmed in practice (found during local GREEN-phase run) | `run_rbac_prune_001` | `kubectl create secret generic fleet-oauth2-creds`/`we-oauth2-creds` provisioned as dummy OAuth2 credentials before the `helm upgrade` that enables FMC; idempotent-safe (`\|\| true`), no explicit cleanup needed since `run_uninst_001` removes the whole namespace shortly after in `flow_a_production` |
| R3 | `apifrontend.enabled=false` + `console.enabled=true` (left on, never reverted, by the earlier `run_console_live_001`) is a schema-rejected combination — console's nginx sidecar hardcodes a reverse-proxy to APIFrontend with no other backend | `run_rbac_prune_003`'s `helm upgrade` would fail outright | Confirmed via `helm template` spike before implementation | `run_rbac_prune_003` | Disable both together in the same `helm upgrade` (`--set apifrontend.enabled=false --set console.enabled=false`); confirmed via spike to render cleanly |
| R4 | CI time budget — the `Helm Smoke Tests (Kind, tls=hook)` job has already had its timeout bumped once (to 35 min) due to suite growth | Job could time out | Low | N/A (measured) | New tests add 3 `helm upgrade` calls (2m timeout each, real duration ~10-20s) + a handful of `kubectl get` calls — well under a minute added total; flagged in the plan as a known-tight budget, not newly introduced by this change |
| R5 | Binding to the built-in `view` ClusterRole a second time via `additionalClusterRoleBindings` (in `run_rbac_prune_002`) is slightly redundant with the existing unconditional `gateway-view` binding | None — cosmetic only | N/A | `run_rbac_prune_002` | Accepted: proves the CRB create/prune lifecycle mechanics without needing a throwaway test-only ClusterRole; grants no new permissions since `gateway-view` already binds the same ClusterRole unconditionally |

### 3.1 Risk-to-Test Traceability

R1 is the primary "did the architecture-based confidence hold" risk — resolved
by the live run itself (§8/§9). R2/R3 were preflight-spike findings, not surprises
found during the live run, and are mitigated directly in the test bodies. R4/R5 are
accepted, non-blocking observations.

---

## 4. Scope

### 4.1 Features to be Tested

- **FMC cluster-scoped RBAC lifecycle** (`fleetmetadatacache.enabled` true→false):
  7 objects across `fleetmetadatacache.yaml` and `fmc-scope-check-client-rbac.yaml`
  (5 checked directly by the test — the other 2 render only under a second,
  independent toggle (`fleetmetadatacache.namespace=""`) not exercised here).
- **`additionalClusterRoleBindings` lifecycle** (list-membership add/remove): the
  generated `ClusterRoleBinding` for a name added to, then removed from, the list.
- **APIFrontend cluster-scoped RBAC lifecycle** (`apifrontend.enabled` true→false): a
  representative subset (5 of 15) covering all 3 object shapes — main ClusterRole/CRB,
  a per-persona ClusterRole, and the console-access gate ClusterRole/CRB.

### 4.2 Features Not to be Tested

- **`gateway.enabled`**: does not exist as a field anywhere in `values.schema.json` —
  Gateway's RBAC renders unconditionally. Nothing to test; clarified on the issue
  (§12).
- **Any `kubernaut-operator` code**: architecturally unrelated (that repo's #341/#344 is
  a *different* bug — recomputing desired state with no stored-prior-state diff at all,
  the opposite of Helm's native behavior this plan verifies).
- **New `helm-unittest` cases**: existing single-render coverage
  (`networkpolicies_unconditional_test.yaml` et al.) is already sufficient for the
  render-side of the behavior; this plan closes only the upgrade-diff-and-prune gap.
- **Any chart template changes**: no bug was found anywhere in preflight; this is a
  pure test-only addition (`git diff --stat -- charts/` is empty for this change).

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Live-cluster smoke test (`scripts/helm-smoke-test.sh`), not `helm-unittest` or a Go integration test | The behavior under test — Helm's release-state diff-and-prune on `helm upgrade` — only exists at the real `helm upgrade` layer against a live cluster's stored release history. `helm-unittest` has no release-state/diff concept (single-render only); no Go code in this repo implements or wraps this behavior for a Go-level integration test to exercise. |
| Reuse the already-running Kind cluster/release from `flow_a_production()`, no new Kind cluster or workflow step | Minimizes added CI time and blast radius; all three tests are pure `helm upgrade --reuse-values` + `kubectl get` operations on the existing release. |
| Check a representative subset (5 of 15) of APIFrontend's RBAC objects, not all 15 | Keeps the test fast and readable while covering all 3 distinct object "shapes" (main, per-persona, console-access-gated) — sufficient to prove the `{{- if $v.enabled }}` gate prunes correctly across the whole template, since all 15 objects share the same single gate. |
| Bind to the built-in `view` ClusterRole in `run_rbac_prune_002` rather than a throwaway test-only ClusterRole | Avoids adding chart-unrelated test fixtures; `view` is already bound unconditionally via `gateway-view`, so this grants no new permissions — it isolates the CRB create/prune mechanics themselves, not the underlying role's permissions. |
| Provision dummy OAuth2 credential Secrets in `run_rbac_prune_001` rather than disabling `global.fleet.oauth2` | `global.fleet.oauth2.enabled=true` is required by `values.schema.json` to enable `fleetmetadatacache` at all (the positive control needs FMC actually enabled to prove the "was there" half of prune-verification) — the collateral secret-mount requirement on other services is a real, pre-existing cross-service coupling in the chart's fleet-preamble helper, not a test artifact to work around by disabling the very thing under test. |
| Place `run_rbac_prune_003` after `run_console_live_001`, not alongside `run_rbac_prune_001`/`002` | Console depends on `apifrontend.enabled=true`; disabling APIFrontend before the console-liveness test runs would break that unrelated test's precondition. |

---

## 5. Approach

### 5.1 Coverage Policy

- **Live-cluster smoke test only.** No unit or integration test tier applies — see
  §4.3 for why (the behavior lives entirely at the `helm upgrade` layer, outside any
  Go code this repo controls).
- Each test follows the same shape: **positive control** (assert the object(s) exist
  while the toggle is on) → **toggle off** (`helm upgrade --reuse-values`) →
  **prune assertion** (assert the object(s) are gone).

### 5.2 Two-Tier Minimum

Not applicable in the usual UT+IT sense — this is a live-cluster (E2E-equivalent for
Helm chart behavior) verification with no unit-testable logic and no Go wiring point;
the "wiring" being proven is Helm's own internal upgrade-diff codepath, invoked through
its real CLI, which is the correct and only way to observe it.

### 5.3 Business Outcome Quality Bar

The business outcome under test is: "when an operator disables a feature via
`helm upgrade`, the cluster-scoped RBAC grants that feature required are actually gone
from the cluster afterward" — not merely "the rendered template omits them for a given
values input" (which `helm-unittest` already proves).

### 5.4 Pass/Fail Criteria

**PASS**:
1. `ST-CHART-RBAC-PRUNE-001/002/003` all report `ok` in the smoke-test TAP output.
2. Zero regressions in the rest of `scripts/helm-smoke-test.sh`'s Flow A/Flow B suite.
3. `git diff --stat -- charts/` is empty (test-only change, no chart code touched).

**FAIL**: any of the above not met.

---

## 6. Test Items

### 6.1 Unit-Testable Code

None — no new Go or template logic was added.

### 6.2 Integration-Testable Code

None — see §4.3/§5.2 for why this behavior has no Go-level wiring point.

### 6.3 Smoke-Testable Code (this plan's actual tier)

| File | Functions | Lines (approx) |
|------|-----------|-----------------|
| `scripts/helm-smoke-test.sh` | `run_rbac_prune_001` | ~65 (incl. secret provisioning + comments) |
| `scripts/helm-smoke-test.sh` | `run_rbac_prune_002` | ~30 |
| `scripts/helm-smoke-test.sh` | `run_rbac_prune_003` | ~40 |
| `scripts/helm-smoke-test.sh` | `flow_a_production` (wiring call sites) | 3 lines added |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-PLATFORM-005 FR-6 | `fleetmetadatacache.enabled` transitioning to `false` prunes its 5 directly-checked cluster-scoped RBAC objects via a real `helm upgrade` | P2 | Smoke (live cluster) | ST-CHART-RBAC-PRUNE-001 | Pass |
| BR-PLATFORM-005 FR-6 | A `gateway.additionalClusterRoleBindings` entry's generated `ClusterRoleBinding` is pruned when removed from the list via a real `helm upgrade` | P2 | Smoke (live cluster) | ST-CHART-RBAC-PRUNE-002 | Pass |
| BR-PLATFORM-005 FR-6 | `apifrontend.enabled` transitioning to `false` prunes its cluster-scoped RBAC (representative 5-of-15 subset) via a real `helm upgrade` | P2 | Smoke (live cluster) | ST-CHART-RBAC-PRUNE-003 | Pass |
| FedRAMP AC-6 (least privilege) | All three tests above are the control-objective evidence that a disabled feature's cluster-scoped RBAC grant does not outlive it | — | Smoke (live cluster) | ST-CHART-RBAC-PRUNE-001/002/003 | Pass |

---

## 8. Test Scenarios

### Smoke Tier: Live-Cluster `helm upgrade` Verification

**File**: `scripts/helm-smoke-test.sh`

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `ST-CHART-RBAC-PRUNE-001` | FMC's cluster-scoped RBAC is gone from the cluster after `fleetmetadatacache.enabled=false`, not just absent from a fresh render | Pass |
| `ST-CHART-RBAC-PRUNE-002` | An `additionalClusterRoleBindings` entry's `ClusterRoleBinding` is gone after removing the entry from the list | Pass |
| `ST-CHART-RBAC-PRUNE-003` | APIFrontend's cluster-scoped RBAC is gone after `apifrontend.enabled=false` | Pass |

### Tier Skip Rationale

- **Unit/Integration**: not applicable — see §4.3/§5.2. There is no Go code or
  standalone template-rendering logic to isolate; the behavior under test is Helm's
  own internal upgrade-diff codepath, only observable through a real `helm upgrade`.
- **`helm-unittest`**: not added — existing single-render coverage
  (`networkpolicies_unconditional_test.yaml`, `console_apifrontend_dependency_test.yaml`,
  `kubernautagent_apifrontend_dependency_test.yaml`) already proves the render-side of
  `apifrontend.enabled`; this plan's gap was specifically the upgrade-diff-and-prune
  side, which `helm-unittest` cannot express.

---

## 9. Test Cases

### ST-CHART-RBAC-PRUNE-001: FMC cluster-scoped RBAC prune-on-disable

**BR**: BR-PLATFORM-005 FR-6
**Priority**: P2
**Type**: Smoke (live Kind cluster)
**File**: `scripts/helm-smoke-test.sh`, `run_rbac_prune_001` (called from `flow_a_production`, right after `run_mon_003`)

**Test Steps**:
1. **Given**: dummy OAuth2 credential Secrets (`fleet-oauth2-creds`, `we-oauth2-creds`)
   are provisioned in the release namespace (idempotent; required so the schema-mandated
   `global.fleet.oauth2.enabled=true` doesn't cascade into unrelated pod failures — see
   R2).
2. **When**: `helm upgrade --reuse-values --set fleetmetadatacache.enabled=true` (plus
   the required `global.fleet.*`/`workflowexecution.fleet.*` schema fields) is applied.
3. **Then** (precondition): all 5 checked FMC RBAC objects exist via `kubectl get`.
4. **When**: `helm upgrade --reuse-values --set fleetmetadatacache.enabled=false` is
   applied.
5. **Then**: none of the 5 objects exist via `kubectl get`.

**Live run result**: `ok` — confirmed 2026-08-16 (`/tmp/helm-smoke-2159-run2.log`,
full-suite run, test #111 of 145).

**Dependencies**: Kind cluster with the chart already installed by earlier steps in
`flow_a_production` (real, not envtest/mocked).

---

### ST-CHART-RBAC-PRUNE-002: `additionalClusterRoleBindings` prune-on-removal

**BR**: BR-PLATFORM-005 FR-6
**Priority**: P2
**Type**: Smoke (live Kind cluster)
**File**: `scripts/helm-smoke-test.sh`, `run_rbac_prune_002` (called right after `run_rbac_prune_001`)

**Test Steps**:
1. **When**: `helm upgrade --reuse-values --set 'gateway.additionalClusterRoleBindings[0]=view'`
   is applied.
2. **Then** (precondition): `clusterrolebinding/gateway-ext-view` exists.
3. **When**: `helm upgrade --reuse-values --set-json 'gateway.additionalClusterRoleBindings=[]'`
   is applied.
4. **Then**: `clusterrolebinding/gateway-ext-view` no longer exists.

**Live run result**: `ok` — confirmed 2026-08-16 (`/tmp/helm-smoke-2159-run2.log`,
test #112 of 145).

**Dependencies**: same as ST-CHART-RBAC-PRUNE-001.

---

### ST-CHART-RBAC-PRUNE-003: APIFrontend cluster-scoped RBAC prune-on-disable

**BR**: BR-PLATFORM-005 FR-6
**Priority**: P2
**Type**: Smoke (live Kind cluster)
**File**: `scripts/helm-smoke-test.sh`, `run_rbac_prune_003` (called after `run_console_live_001`, before `run_uninst_001`, per R3/§4.3)

**Test Steps**:
1. **Then** (precondition): 5 representative APIFrontend RBAC objects (main
   ClusterRole/CRB, one per-persona ClusterRole, console-access-gate ClusterRole/CRB)
   exist via `kubectl get` (APIFrontend is enabled by default).
2. **When**: `helm upgrade --reuse-values --set apifrontend.enabled=false --set console.enabled=false`
   is applied (both disabled together per R3 — console hard-depends on APIFrontend).
3. **Then**: none of the 5 objects exist via `kubectl get`.

**Live run result**: `ok` — confirmed 2026-08-16 (`/tmp/helm-smoke-2159-run2.log`,
test #115 of 145).

**Dependencies**: same as ST-CHART-RBAC-PRUNE-001, plus must run after
`run_console_live_001` (R3/§4.3).

---

## 10. Environmental Needs

### 10.1 Smoke Tests

- **Framework**: bash + TAP (`tap_ok`/`tap_not_ok` helpers already in
  `scripts/helm-smoke-test.sh`)
- **Infrastructure**: live Kind cluster (dedicated `kubernaut-2159-smoke` cluster used
  for local verification; CI's `helm-smoke-test` job's own Kind cluster in the pipeline)
  with all 13 service images (including `db-migrate`) built and loaded
- **Location**: `scripts/helm-smoke-test.sh`, `flow_a_production()`

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None — self-contained addition to `scripts/helm-smoke-test.sh`; reuses the existing
Kind cluster/release from earlier in `flow_a_production()`.

### 11.2 Execution Order

1. **Preflight + spike** (read-only `helm template` dry-runs, no cluster/file
   mutation): confirmed all three toggle cases' real behavior, corrected an initial
   wrong finding about `apifrontend.enabled`, confirmed exact `--set`/`--set-json`
   syntax and exact RBAC object names for all three cases.
2. **RED/GREEN (combined, since this is test-only)**: wrote `run_rbac_prune_001/002/003`
   and wired them into `flow_a_production()`; ran the full smoke suite once against a
   dedicated local Kind cluster (`kubernaut-2159-smoke`) with all 13 service images
   built and loaded. Found and fixed one collateral issue during this run (R2 — dummy
   OAuth2 secrets needed) before the full suite passed clean.
3. **Verification**: full-suite re-run (145 specs) confirmed 0 failures, including the
   3 new tests and no regressions to the other 142 pre-existing specs.
4. **REFACTOR**: N/A — test-only bash addition, no code to refactor.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|--------------|
| This test plan | `docs/testing/2159/TEST_PLAN.md` | Strategy and test design |
| BR extension | `docs/requirements/BR-PLATFORM-005-helm-chart-operator-security-parity.md` | New FR-6 (RBAC lifecycle parity across upgrades) |
| Smoke tests | `scripts/helm-smoke-test.sh` | `run_rbac_prune_001/002/003`, wired into `flow_a_production()` |
| Issue comment | GitHub issue #2159 | Clarifies `gateway.enabled` doesn't exist (nothing to test) and `apifrontend.enabled` is confirmed already-working (no fix needed, test-only) |

---

## 13. Execution

```bash
# Full local smoke-test run (requires a Kind cluster with all 13 service images
# — including db-migrate — built and loaded; see scripts/helm-smoke-test.sh --help)
export KIND_CLUSTER_NAME=<your-kind-cluster-name>
./scripts/helm-smoke-test.sh \
  --platform kind \
  --image-tag <tag> \
  --registry localhost \
  --chart-path charts/kubernaut/

# Confirm no chart/production code was touched
git diff --stat -- charts/
```

---

## 14. Wiring Verification

Not applicable in the usual CHECKPOINT W sense (no new `pkg/`/`cmd/` component was
introduced). The equivalent verification here is that all three new functions are
actually **called** from `flow_a_production()`, not just defined:

| Component | Called From | Status |
|-----------|--------------|--------|
| `run_rbac_prune_001` | `flow_a_production()`, line ~1653 (after `run_mon_003`) | Confirmed via live run (#111/145) |
| `run_rbac_prune_002` | `flow_a_production()`, line ~1654 (after `run_rbac_prune_001`) | Confirmed via live run (#112/145) |
| `run_rbac_prune_003` | `flow_a_production()`, line ~1660 (after `run_console_live_001`) | Confirmed via live run (#115/145) |

---

## 15. Existing Tests Requiring Updates

None. No pre-existing test's behavior or preconditions were changed. `run_mon_003`
(the nearest pre-existing analog) is untouched — it still reverts its own toggle
without asserting deletion, which is fine, since it is out of this plan's scope
(autoscaling, not RBAC).

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-08-15/16 | Initial and final test plan. Preflight (3 parallel investigations) + spike (`helm template` dry-runs) confirmed all three named toggle cases needed no code fix — this is a pure test-coverage addition. `run_rbac_prune_001/002/003` added to `scripts/helm-smoke-test.sh`, wired into `flow_a_production()`, and confirmed passing on a full local run (145/145 specs, 0 failures) against a dedicated Kind cluster. |
