# BR-PLATFORM-004: Helm Chart Single-Install-Per-Cluster Guard

**Business Requirement ID**: BR-PLATFORM-004
**Category**: Platform
**Priority**: P2
**Target Version**: V1.5
**Status**: Approved
**Date**: 2026-07-06

---

## Business Need

### Problem Statement

Kubernaut's supported operational model is **one Helm installation per cluster**. The chart
provisions ~30 cluster-scoped resources (ClusterRole, ClusterRoleBinding,
MutatingWebhookConfiguration, ValidatingWebhookConfiguration) with static, unprefixed names.
A second `helm install` into a different namespace on the same cluster is technically possible to
attempt but is not a supported topology — today it is only caught indirectly, when Helm's
ownership-tracking hits the first colliding cluster-scoped resource, producing a generic
"rendered manifests contain a resource that already exists" error that gives the operator no
indication of *why*, and can fire after some resources have already been applied.

**Impact**:
- Operators attempting a second install get a confusing, generic Helm error instead of an
  actionable, Kubernaut-specific explanation of the supported topology.
- Failure timing is inconsistent (depends on manifest processing order), and provides no
  remediation guidance.

---

## Business Objective

Fail fast and clearly, before any resources are applied, when a second Kubernaut installation is
attempted on a cluster that already has one — without preventing `helm upgrade` of the existing
release, and without requiring any live cluster access from `helm template`/`helm lint --strict`.

### Success Criteria

1. A second `helm install` attempt on a cluster with an existing Kubernaut release fails
   immediately with a Kubernaut-specific message identifying the existing release/namespace and
   the `helm uninstall` remediation step — not Helm's generic ownership-conflict error.
2. `helm upgrade` of the *same* release continues to work with zero behavior change.
3. `helm template` / `helm lint --strict` (no live cluster) render successfully with the guard as
   a no-op, consistent with the chart's existing offline-rendering contract.
4. Zero regression for the single, supported installation-per-cluster case.

---

## Functional Requirements

- **FR-1**: New `kubernaut.singleInstallGuard` helper in `templates/_helpers.tpl` that `lookup`s
  the `gateway-role` ClusterRole (an unconditionally-rendered, always-present canary resource) and
  compares its `meta.helm.sh/release-name`/`meta.helm.sh/release-namespace` annotations against
  `.Release.Name`/`.Release.Namespace`.
- **FR-2**: If the annotations exist and differ, `fail` with a message naming the conflicting
  release/namespace and the exact `helm uninstall` command to run.
- **FR-3**: If `lookup` returns empty (no live cluster access, or first install), the guard is a
  no-op.
- **FR-4**: The guard must be validated live (not via `helm template`, since `lookup` requires a
  live API server) via `scripts/helm-smoke-test.sh`.

---

## Non-Goals

- Does not attempt to make multi-install-per-cluster *work* (e.g. via resource renaming) — that
  topology remains explicitly unsupported.
- Does not address concurrent (racing) `helm install` attempts — accepted residual risk; Helm's
  own ownership-conflict check remains a backstop for that case.

---

## Related Decisions

- **Design rationale**: [DD-018](../architecture/decisions/DD-018-helm-chart-single-install-per-cluster-guard.md)
  (alternatives considered: rename cluster-scoped resources, docs-only, explicit `lookup`-based
  guard — the last was approved).
- **Tracked in**: [Issue #1589](https://github.com/jordigilh/kubernaut/issues/1589).

---

**Document Status**: ✅ Approved
**Priority**: P2 — operational safety guard, not a functional blocker
