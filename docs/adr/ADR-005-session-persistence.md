# ADR-005: Session Persistence via K8s CRD

**Status:** Accepted
**Date:** 2026-05-01
**Deciders:** AF team
**Source:** #56 design

## Context

AF manages investigation sessions that link A2A tasks to kubernaut pipeline CRDs. Sessions must survive AF pod restarts and enable user reconnection. We need a persistence mechanism.

## Decision

Persist session state as a Kubernetes Custom Resource (InvestigationSession CRD) at `apifrontend.kubernaut.ai/v1alpha1`. AF is fully stateless between requests — the CRD is the only persistent state.

## Consequences

- Sessions survive pod restarts (K8s etcd is the store)
- User reconnection works by querying CRD via label selectors (O(1) with field indexes)
- No external database dependency (Redis, PostgreSQL)
- Standard K8s tooling works (`kubectl get investigationsessions`)
- Built-in RBAC (only AF SA can write sessions)
- TTL cleanup via standard controller-runtime reconciler
- etcd size consideration: expected <100 active sessions (human-driven, not automated)
- Field indexes enable efficient lookups without full list/filter

## Alternatives Considered

| Alternative | Why rejected |
|-------------|-------------|
| In-memory map | Lost on pod restart; no HA across replicas; no `kubectl` visibility |
| Redis | External dependency; requires operator support; overkill for <100 sessions |
| PostgreSQL/DataStorage | External dependency; schema management overhead; not K8s-native |
| ConfigMap per session | No schema validation; no status sub-resource; size limits (1MB) |
