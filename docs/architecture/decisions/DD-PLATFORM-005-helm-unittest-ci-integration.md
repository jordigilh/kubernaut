# DD-PLATFORM-005: helm-unittest as a Dedicated Fast-Fail CI Gate

**Date**: July 23, 2026
**Status**: ✅ **APPROVED**
**Confidence**: 92%
**Last Reviewed**: July 23, 2026
**Related**: Issue #1686 (Fleet ClusterRegistry RBAC least-privilege split, BR-RBAC-020),
`charts/kubernaut/tests/` (pre-existing `helm-unittest` specs, previously never run in CI)

---

## 🎯 **DECISION**

**`helm-unittest` is wired into `ci-pipeline.yml` as its own job (`helm-unittest`),
scheduled in Stage 1 alongside `lint-go`/`license-scan`/`unit-tests` (depends only
on `detect-changes`), rather than folded into the existing `helm-smoke-test` job
or left unintegrated.** A `make test-helm` target is added for local parity.

---

## 📊 **Context & Problem**

`charts/kubernaut/tests/*.yaml` already contained 8 `helm-unittest` suites (81
test cases) covering RBAC conditionals, TLS mode branching, sync-wave ordering,
and default-hardening behavior — an established, working pattern in this repo.
None of it ran in CI: `ci-pipeline.yml` had no `helm unittest` invocation
anywhere. The only chart validation gate was `helm-smoke-test`, which installs
the full chart into a live Kind cluster and asserts pods come up healthy —
useful for catching install-time failures, but unable to assert *specific*
rendered output (e.g. "does this ClusterRole contain rule X when value Y is
set") the way `helm-unittest`'s `contains`/`notContains`/`documentSelector`
assertions can.

Implementing #1686 (BR-RBAC-020: namespace-scoped RBAC instead of unconditional
cluster-wide grants) needed exactly this kind of assertion — "the ClusterRole
does NOT contain rule X, and a namespace-scoped Role DOES" — which motivated
finally wiring the existing `helm-unittest` specs into CI rather than relying
solely on manual `helm template | grep` verification.

---

## 🔍 **Alternatives Considered**

### **Option A: Dedicated `helm-unittest` job in Stage 1** ✅ **CHOSEN**

**Approach**: New job depending only on `detect-changes`, running in parallel
with `lint-go`/`unit-tests` (~1 minute: checkout + `helm plugin install` +
`helm unittest`). Wired into `summary` and `merge-gate` `needs:` lists.

**Pros**:
- ✅ Fails fast (~1 min) on a broken template, long before the ~30-45 min
  `helm-smoke-test` (which needs `build-images` + a live Kind cluster) would
  ever catch the same defect
- ✅ No new infra dependency (no cluster, no built images) — pure offline
  chart rendering + assertion
- ✅ Assertion granularity `helm-smoke-test` cannot match (specific rule
  presence/absence, conditional resource counts) without turning the smoke
  test into an unmaintainable pile of `kubectl get -o yaml | grep`

**Cons**:
- ❌ One more job in an already-long pipeline (mitigated: ~1 min, runs in
  parallel with existing Stage 1 jobs, adds ~0 min to total wall-clock time)

**Confidence**: 92% (approved)

---

### **Option B: Fold into `helm-smoke-test`**

**Approach**: Add a `helm unittest charts/kubernaut/` step inside the existing
`helm-smoke-test` job, before the `helm install` steps.

**Pros**:
- ✅ No new job in the pipeline's job graph

**Cons**:
- ❌ `helm-smoke-test` needs `build-images`/`build-infra-images` first (Stage
  2+) — a pure-template bug would only surface after waiting on image builds,
  losing the fast-fail benefit that's the whole point of a unit-test tier
- ❌ `helm-smoke-test` already runs a `tls_mode` matrix (`hook`,
  `cert-manager`) at 30-45 min each; conflating "assert rendered YAML shape"
  with "assert the chart installs and pods become healthy" mixes two
  different test tiers (unit vs. integration) into one job, working against
  the same Pyramid Invariant principle this project applies to Go tests

**Confidence**: N/A (rejected)

---

### **Option C: Leave unintegrated (status quo)**

**Approach**: Keep `helm-unittest` specs as a local-only, `make`-less,
CI-less convenience never enforced on PRs.

**Pros**:
- ✅ Zero pipeline changes

**Cons**:
- ❌ 81 pre-existing test cases (now 100 with #1686's additions) provide no
  actual regression protection — anyone can merge a chart change that
  silently breaks them, since nothing runs them
- ❌ Directly contradicts the reason `helm-unittest` specs were written in
  the first place (RBAC/TLS-mode/sync-wave regression guards)

**Confidence**: N/A (rejected — this was literally the pre-#1686 status quo
being fixed)

---

## Decision

**APPROVED: Option A** — dedicated Stage-1 `helm-unittest` job.

**Rationale**:
1. **Fast-fail economics**: a chart template regression should fail in ~1
   minute, not ~30-45 minutes behind image builds and a live cluster
2. **Tier separation**: `helm-unittest` (offline, rendered-YAML assertions) and
   `helm-smoke-test` (live cluster, install/health) are different test tiers
   with different failure modes worth keeping distinguishable in CI output
3. **Zero net wall-clock cost**: runs in parallel with existing Stage 1 jobs,
   which are already the pipeline's critical-path floor

---

## Implementation

**Primary Implementation Files**:
- `.github/workflows/ci-pipeline.yml` — new `helm-unittest` job (Stage 1,
  `needs: [detect-changes]`); added to `summary` and `merge-gate` `needs:`
  lists and status-reporting steps
- `Makefile` — new `test-helm` target (local parity, guards on the plugin
  being installed)
- `charts/kubernaut/tests/fleet_registry_rbac_test.yaml` — first suite
  written expecting to run in CI (19 cases, #1686/BR-RBAC-020)

**Pinned versions**: `helm-unittest` plugin `v1.1.1` (latest tag at time of
writing); `azure/setup-helm` reuses the same pinned SHA already used by
`helm-smoke-test` for consistency.

### Consequences

**Positive**:
- ✅ 100 `helm-unittest` cases now enforced on every PR touching
  `charts/**`/`Makefile`/`.github/workflows/**` (per existing
  `detect-changes` filters — no filter changes needed)
- ✅ Local dev parity via `make test-helm`

**Negative**:
- ⚠️ One more required check in `merge-gate`'s `needs:` list — **Mitigation**:
  negligible wall-clock cost (parallel, ~1 min); `merge-gate`'s existing
  generic `contains(needs.*.result, 'failure')` check needed no new logic

**Neutral**:
- 🔄 Establishes the pattern for future chart-behavior assertions to land as
  `helm-unittest` specs (enforced) rather than only `helm-smoke-test`
  installs (which can't assert specific rendered content) or ad hoc manual
  `helm template` checks (which aren't enforced at all)

### Related Decisions
- **Builds On**: pre-existing `charts/kubernaut/tests/*.yaml` suites (never
  previously CI-enforced)
- **Supports**: BR-RBAC-020 (#1686)
