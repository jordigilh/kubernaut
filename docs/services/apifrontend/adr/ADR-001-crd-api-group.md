# ADR-001: CRD API Group Selection

**Status:** Superseded (by migration to `kubernaut.ai` — see #1219)
**Date:** 2026-05-01
**Superseded:** 2026-05-21
**Deciders:** AF team
**Source:** #56, OPS-1 amendment

## Supersession Note

This ADR is superseded. The InvestigationSession CRD has been migrated from
`apifrontend.kubernaut.ai/v1alpha1` to `kubernaut.ai/v1alpha1`.

**Rationale:** AF is now a first-class service in the kubernaut monorepo, shares
the same release cadence, and its CRD is installed by the kubernaut operator
alongside all other `kubernaut.ai` CRDs. The sub-domain added complexity
(double-nested api directory, separate RBAC rules, confusing `sync-embed` glob
history) with no remaining benefit. Since InvestigationSession was introduced in
v1.5 RC and has no production data, the migration carries no data-migration risk.

## Context (original)

The API Frontend owns an InvestigationSession CRD for session persistence. We need to choose an API group that avoids collision with kubernaut core CRDs while maintaining organizational clarity.

kubernaut core CRDs use `kubernaut.ai/v1alpha1` (RemediationRequest, AIAnalysis, SignalProcessing). AF's CRD is owned and managed exclusively by AF — it is never read or written by kubernaut controllers.

## Decision (original)

Use `apifrontend.kubernaut.ai/v1alpha1` as the API group for AF-owned CRDs.

## Consequences (original)

- Clear ownership: `apifrontend.kubernaut.ai` resources are AF-exclusive
- No collision with kubernaut core CRDs (`kubernaut.ai`)
- Follows the Cluster API sub-domain pattern (e.g., `infrastructure.cluster.x-k8s.io`)
- Operator installs AF CRDs separately from kubernaut CRDs
- Labels use `apifrontend.kubernaut.ai/` prefix (no collision with `kubernaut.ai/` labels)

## Alternatives Considered

| Alternative | Why rejected |
|-------------|-------------|
| `kubernaut.ai/v1alpha1` (same as core) | Couples AF lifecycle to kubernaut CRD versioning; confusing ownership |
| `apifrontend.io/v1alpha1` | No organizational tie to kubernaut; confusing in multi-product clusters |
| `sessions.kubernaut.ai/v1alpha1` | Too narrow if AF adds more CRDs later |
