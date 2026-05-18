# ADR-011: Deployment Model — Operator for Production, Helm for Development

**Status:** Accepted
**Date:** 2026-05-02
**Deciders:** AF team, Operator team
**Source:** OPS-2 amendment

## Context

kubernaut deploys on OpenShift Container Platform (OCP) in production via `kubernaut-operator`. The operator manages hardening (SCCs, NetworkPolicies, Routes), secret injection, and lifecycle management. Helm charts exist for development and testing on vanilla K8s (kind).

## Decision

- **Production (OCP):** kubernaut-operator deploys and manages AF
- **Development/Test:** Helm chart in `charts/kubernaut-apifrontend/`
- **Local dev:** `make run-local` with mock dependencies (no cluster needed)

The Helm chart and operator share the same logical configuration schema (OPS-2 values structure).

## Consequences

- Production deployments have enterprise-grade hardening (operator-managed SCCs, Routes, NetworkPolicies)
- Developers can iterate locally without operator dependency
- Helm chart serves as the "specification" for what operator must deploy
- CI uses Helm for integration tests (envtest + kind)
- Operator team has clear contract: AF issues #42-#45 define requirements
- No Helm in production — operator is the single source of truth for production state

## Alternatives Considered

| Alternative | Why rejected |
|-------------|-------------|
| Helm-only (no operator) | No OCP-specific hardening; manual Route/SCC management; inconsistent with kubernaut deployment model |
| Operator-only (no Helm) | Developers need operator running locally for any testing; too heavy for dev iteration |
| Kustomize overlays | Less feature-rich than Helm for parameterization; unfamiliar to kubernaut team |
| Raw manifests | No parameterization; unmaintainable for multi-environment |
