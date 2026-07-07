# DD-018: Helm Chart Single-Install-Per-Cluster Guard

## Status
**✅ APPROVED** (2026-07-06)
**Last Reviewed**: 2026-07-06
**Confidence**: 85%

## Context & Problem

**Problem**: The Kubernaut Helm chart (`charts/kubernaut/`) provisions ~30 cluster-scoped
resources (ClusterRole, ClusterRoleBinding, MutatingWebhookConfiguration,
ValidatingWebhookConfiguration) across 14 template files using static names with no
release/namespace prefix (e.g. `gateway-role`, `authwebhook-mutating`,
`kubernaut-agent-investigator`). Kubernaut's supported operational model is **one
installation per cluster** — it is technically possible to `helm install` a second
release into a different namespace, but that topology is explicitly unsupported.

Today, a second install attempt is only caught indirectly: Helm's built-in ownership
tracking (`meta.helm.sh/release-name` annotations) causes the second install to fail
when it hits the first colliding cluster-scoped resource, with a generic message like
`rendered manifests contain a resource that already exists... unable to continue with
install`. This is late-firing (only surfaces after some resources may have already been
applied), inconsistent across resource types/ordering, and does not explain *why* — an
operator sees a Helm ownership error, not "Kubernaut only supports one installation per
cluster."

**Key Requirements**:
- Fail fast, before any resources are applied, when a second installation is attempted.
- Produce a clear, Kubernaut-specific error message that explains the supported topology
  and the remediation step (`helm uninstall` the existing release first).
- No regression for the single, supported install-per-cluster case.
- Must work identically across `helm install` and `helm upgrade` (an upgrade of the
  *same* release must never trip the guard).
- Must degrade safely under `helm template`/`helm lint --strict` (no live cluster;
  `lookup` returns empty), consistent with the chart's existing offline-rendering
  contract.

## Alternatives Considered

### Alternative 1: Rename all cluster-scoped resources with a release prefix
**Approach**: Prefix every cluster-scoped resource name with
`{{ include "kubernaut.fullname" . }}-`, mirroring the Kubernaut Operator's own
`<namespace>-<name>` collision-avoidance pattern, so that multiple releases *can*
coexist without colliding.

**Pros**:
- ✅ Removes the collision entirely — multiple releases would genuinely work.
- ✅ Matches the Operator's own naming convention.

**Cons**:
- ❌ Solves a problem we don't want solved — multi-install-per-cluster is not a
  supported topology, so making it *work* is unnecessary surface area.
- ❌ Breaking change for any existing single-cluster install (cluster-scoped resources
  are recreated under new names; old ones orphaned) with no corresponding benefit.
- ❌ Does not, by itself, stop an operator from actually installing two competing
  copies of Kubernaut in one cluster, which is exactly the outcome we want to prevent.

**Confidence**: 30% (rejected — solves the wrong problem)

---

### Alternative 2: Document the constraint only, rely on Helm's existing error
**Approach**: Leave the static names and Helm's ownership-conflict behavior as-is;
add a README note stating "only one installation per cluster is supported."

**Pros**:
- ✅ Zero implementation cost.
- ✅ No new template logic to maintain.

**Cons**:
- ❌ The failure an operator actually sees is Helm's generic ownership-conflict error,
  not a Kubernaut-specific explanation — documentation alone doesn't change runtime
  behavior or the error surfaced at the point of failure.
- ❌ Failure timing depends on manifest ordering (whichever colliding resource Helm
  processes first) — inconsistent and unhelpful for diagnosis.

**Confidence**: 45% (rejected — passive documentation doesn't match the fail-fast,
clear-error requirement)

---

### Alternative 3: Explicit `lookup`-based pre-install guard (APPROVED)
**Approach**: Add a named template, called early during chart rendering (mirroring the
existing `kubernaut.hasClusterAccess` `lookup`-canary pattern introduced for API-server
and cert-manager auto-discovery), that:
1. Uses `lookup` to fetch one canonical, always-present cluster-scoped resource (the
   `gateway-role` ClusterRole).
2. If found, compares its `meta.helm.sh/release-name` / `meta.helm.sh/release-namespace`
   annotations against the current `.Release.Name` / `.Release.Namespace`.
3. If they don't match, `fail` immediately with a message naming the conflicting
   release/namespace and the exact `helm uninstall` remediation command.
4. If `lookup` returns empty (no live cluster access, e.g. under `helm template`/
   `helm lint --strict`, or a genuinely fresh cluster), the guard is a no-op — consistent
   with the chart's existing offline-rendering contract.
5. An upgrade of the *same* release passes trivially, since the annotations match.

**Pros**:
- ✅ Fails before any resources are applied (runs during template rendering, at the top
  of the render — before any resource templates emit YAML).
- ✅ Kubernaut-specific, actionable error message instead of Helm's generic one.
- ✅ Reuses an established, already-reviewed pattern in this chart (`lookup` +
  `hasClusterAccess`-style canary check) rather than introducing new mechanics.
- ✅ No rename, no breaking change, no orphaned resources for existing installs.
- ✅ Correctly permits `helm upgrade` of the same release.

**Cons**:
- ⚠️ Depends on a single canonical resource (`gateway-role`) as the existence canary —
  **Mitigation**: `gateway-role` is unconditionally rendered whenever the chart is
  installed (Gateway is not an optional/gated service), so it is a safe existence proxy;
  document this dependency inline so a future refactor that conditionally disables
  Gateway does not silently break the guard.
- ⚠️ `lookup` only works with a live API server (`helm install`/`upgrade`), not
  `helm template`/`helm lint` — **Mitigation**: this matches the chart's existing
  `kubernaut.hasClusterAccess` contract; the guard is validated live via
  `scripts/helm-smoke-test.sh`, not via template-only tests.
- ⚠️ Does not prevent two *concurrent* `helm install` races (TOCTOU) —
  **Mitigation**: acceptable residual risk; Helm's own ownership-conflict check still
  acts as a backstop for the race case, and concurrent installs of the same chart are
  an operational anti-pattern independent of this guard.

**Confidence**: 85% (approved)

## Decision

**APPROVED: Alternative 3** — Explicit `lookup`-based pre-install guard.

**Rationale**:
1. **Fail-fast, clear error**: Matches the actual requirement — stop a second install
   before it does anything, with a message that explains the supported topology.
2. **No unnecessary surface area**: Does not attempt to make multi-install *work*
   (Alternative 1), since that is explicitly not a goal.
3. **Consistent with existing chart patterns**: Reuses the `lookup`-canary mechanism
   already established and validated for API-server/cert-manager auto-discovery,
   keeping the chart's template logic uniform.
4. **No migration/breaking-change concern**: Static cluster-scoped names are unchanged.

## Implementation

- Implemented as a standalone template, `templates/infrastructure/singleinstallguard.yaml`,
  rather than a `_helpers.tpl` named define — this matches the codebase's existing convention
  for `lookup`-based pre-flight validation (the `$hasCluster` canary + secret-existence checks in
  `templates/infrastructure/secrets.yaml` use the same inline-`lookup`-plus-`fail` idiom, not a
  helper function), so the guard reads consistently with its nearest precedent.
- Canonical canary resource: the `gateway-role` ClusterRole (`templates/gateway/gateway.yaml`).
- Validated live via `scripts/helm-smoke-test.sh` (a second `helm install` in a different
  namespace against the same cluster must fail with the new guard's message); not
  reachable via `helm template`/`helm lint --strict` since those never see a live cluster.
- Confirmed live via a pre-implementation spike (standalone test chart, real Kind cluster):
  offline `helm template` is a no-op; first install passes; `helm upgrade` of the same release
  passes; installing a second, different release fails immediately with
  `GUARD TRIGGERED: existing release "<a>" in namespace "<ns-a>", this install is "<b>" in "<ns-b>"`
  with zero side effects (no partial resource creation for the failed install).

## Consequences

### Positive
- Fast, clear failure for the unsupported second-install case.
- Zero behavior change for the single supported installation.
- Reuses proven chart infrastructure — no new class of template logic.

### Negative
- Adds one more `lookup` call to chart rendering (negligible latency; `lookup` calls are
  already used for API-server/cert-manager discovery).

### Neutral
- Does not address concurrent-install races; accepted as a residual, low-probability risk.

## Related Decisions
- **Builds On**: the `kubernaut.hasClusterAccess` `lookup`-canary pattern (PR #1571,
  API-server endpoint / cert-manager issuer auto-discovery).
- **Tracked in**: Issue #1589 (Helm chart Operator-parity + OCP removal), item A4.

## Review & Evolution

**When to Revisit**:
- If Gateway ever becomes an optional/gated service (the canary resource would need to
  change to a different, unconditionally-rendered resource).
- If Kubernaut's operational model changes to explicitly support multiple installs per
  cluster (would require revisiting Alternative 1 instead).

**Success Metrics**:
- A second `helm install` attempt in the same cluster fails with the new
  Kubernaut-specific message, not Helm's generic ownership-conflict error.
- Zero false positives against `helm upgrade` of the same release.

---

**Priority**: MEDIUM — operational safety guard, not a functional blocker for the
primary Operator-parity/OCP-removal work in Issue #1589.
