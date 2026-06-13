# ADR-065: Fleet Cluster Identity on RemediationRequest CRD

**Status**: Accepted
**Date**: 2026-06-12 (Proposed) | 2026-06-13 (Accepted)
**Deciders**: Architecture Team
**Context**: Fleet remediation requires per-RR cluster identity (#54, #1409)
**Related**: ADR-064 (Multi-Cluster MCP Gateway), Issue #54, Issue #1409

## Context

Today, cluster identity is only known to RO via boot-time discovery (`pkg/shared/cluster/identity.go` → `kube-system` namespace UID). It's included in notification bodies but not on the RR CRD itself.

For fleet-wide remediation (#54), a central control plane manages RemediationRequests from multiple managed clusters. Services consuming RRs (AF, RO, Console) need to know which cluster a signal originated from without inferring it from their own environment.

Currently:
- **Gateway** ingests signals and creates RRs — it knows which cluster the signal came from (each adapter instance is bound to a specific cluster)
- **RO** discovers its local cluster UUID at boot — works for single-cluster, breaks for fleet
- **AF** has no cluster context at all — Console cannot display cluster identity (#1409)
- **RR CRD** (`RemediationRequestSpec`) has no cluster identity field

## Decision

Add `ClusterID` as a first-class field on `RemediationRequestSpec`, populated at creation time by Gateway.

### Schema Change

```go
// RemediationRequestSpec — Universal Fields section
type RemediationRequestSpec struct {
    // ...existing fields...

    // ClusterID identifies the managed cluster where this signal originated.
    // Populated by Gateway at RR creation from the adapter's cluster context.
    // Format: kube-system namespace UID (consistent with clusterid.DiscoverIdentity).
    // +kubebuilder:validation:MaxLength=253
    // +optional
    ClusterID string `json:"clusterID,omitempty"`

    // ClusterName is the human-readable cluster name (OCP infrastructure name,
    // Kind cluster name, or operator-configured label).
    // +kubebuilder:validation:MaxLength=253
    // +optional
    ClusterName string `json:"clusterName,omitempty"`
}
```

### Population Strategy

| Deployment Model | Who Populates | Source |
|---|---|---|
| Single-cluster (current) | Gateway | `clusterid.DiscoverIdentity()` at adapter boot, injected into every RR |
| Fleet (future, #54) | Gateway per cluster | Each adapter instance discovers its own cluster identity at boot |
| Cross-cluster MCP (ADR-064) | MCP Gateway | Cluster prefix → identity mapping from `ClusterResolver` ConfigMap |

### Consumers

| Service | Usage |
|---|---|
| **AF** | Include in `investigation_summary` and `execution_progress` SSE payloads for Console context banner (#1409) |
| **RO** | Replace boot-time `SetClusterIdentity` with RR-sourced identity in notifications (cleaner, per-RR) |
| **Console** | Display cluster in persistent context banner during remediation flows |
| **Fleet scheduler** (future) | Route RRs to appropriate cluster-scoped workflow executors |
| **Audit/DS** | Cluster-scoped audit queries and compliance reporting |

## Alternatives Considered

### A. Boot-time discovery only (current for RO)
- Works for single-cluster
- Fails for fleet: cannot distinguish RRs from different clusters
- AF would need its own discovery call but still couldn't differentiate multi-cluster RRs

### B. Environment variable / ConfigMap per service
- Adds operational burden (configure each service with cluster ID)
- Still per-service, not per-RR — same fleet limitation

### C. Label on RR instead of spec field
- Labels are mutable, not validated, not versioned
- CRD spec fields are the correct mechanism for immutable identity

## Consequences

### Positive
- Every RR is self-describing — carries its own cluster identity
- Fleet-ready from day one — no architectural change needed when #54 lands
- Console gets cluster context immediately (#1409)
- Audit trail includes cluster provenance per-event

### Negative
- CRD schema change (backwards-compat via `omitempty`)
- Gateway must be updated to populate the fields
- Existing RRs will have empty `clusterID` until backfilled or recreated

### Migration

1. Add fields to CRD spec (`omitempty` — no breaking change)
2. Update Gateway adapters to populate `clusterID`/`clusterName` at RR creation
3. AF reads from RR spec when session starts (already fetches RR)
4. RO migrates from `SetClusterIdentity` to RR field (optional, can keep both during transition)

## Implementation Plan

1. CRD schema update (`api/remediation/v1alpha1/remediationrequest_types.go`)
2. Gateway adapter wiring (call `clusterid.DiscoverIdentity` at boot, inject into RR creation)
3. AF consumption (read from RR, thread to EventBridge, include in payloads)
4. RO migration (read from RR spec in notifications, fall back to boot-time if empty)

---

## Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|-----------|----------------------|---------------------|---------|
| ClusterID field | RemediationRequestSpec | api/remediation/v1alpha1/remediationrequest_types.go | CRD validation |
| ClusterName field | RemediationRequestSpec | api/remediation/v1alpha1/remediationrequest_types.go | CRD validation |
| Gateway population | adapter.CreateRR() | pkg/gateway/adapter/ (future) | — |
| AF consumption | session start | pkg/apifrontend/agent/ (future) | — |

---

## Business Requirement Linkage

- **BR-FLEET-001**: Fleet remediation requires cluster identity on every RR
- **Issue #54**: Multi-cluster remediation tracking
- **Issue #1409**: Console cluster context banner

---

## Test Plan Reference

Implementation test plan will be created when the CRD schema change lands. Current branch adds the `ClusterID`/`ClusterName` fields to the spec with `+optional` and `omitempty` for backward compatibility.

---

## FedRAMP Implications

| Control | Impact |
|---------|--------|
| AU-3 (Audit content) | Cluster provenance now recorded per-RR, enabling cluster-scoped audit queries |
| SI-4 (Monitoring) | Cross-cluster signal correlation enabled by `ClusterID` field |
| AC-6 (Least privilege) | Future: RBAC scoped per-cluster identity for multi-tenant fleet |
