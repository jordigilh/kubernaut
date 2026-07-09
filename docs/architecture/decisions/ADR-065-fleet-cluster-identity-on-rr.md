# ADR-065: Fleet Cluster Identity on RemediationRequest CRD

**Status**: Partially superseded (see amendment below)
**Date**: 2026-06-12 (Proposed) | 2026-06-13 (Accepted) | 2026-06-19 (Implemented) | 2026-07-08 (Amended)
**Deciders**: Architecture Team
**Context**: Fleet remediation requires per-RR cluster identity (#54, #1409)
**Related**: ADR-064 (Multi-Cluster MCP Gateway), ADR-068 (Fleet Federation Architecture), Issue #54, Issue #1409, Issue #1651

> **Amendment (2026-07-08, Issue #1651)**: `ClusterName` was removed from `RemediationRequestSpec`
> (and the corresponding `ClusterInfo.Name` / `kubernaut.ai/cluster-name` annotation from the fleet
> registry). It never shipped in a release, was not guaranteed unique across clusters, and was
> therefore unsafe for disambiguation. `ClusterID` remains the sole supported cluster identifier;
> callers that need a display name must resolve it themselves from `ClusterID`. All schema and
> code samples below referencing `ClusterName` are historical and no longer reflect the current
> implementation.

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
| GW cluster extraction | PrometheusAdapter.Parse() | pkg/gateway/adapters/prometheus_adapter.go | IT-GW-FLEET-001 |
| GW CRD population | CRDCreator.Create() | pkg/gateway/processing/crd_creator.go | IT-GW-FLEET-001 |
| GW cluster-aware fingerprint | CalculateClusterAwareFingerprint | pkg/gateway/types/fingerprint.go | IT-GW-FLEET-002 |
| KA SignalContext population | ResolveSignalContext() | internal/kubernautagent/mcp/adapters/signal_resolver.go | IT-KA-FLEET-001 |
| AF RR creation (cluster_id LLM arg) | HandleCreateRR / HandleInvestigateAlert / HandleRemediate / HandleInvestigationMCP | pkg/apifrontend/tools/{af_create_rr,af_investigate_alert,ka_remediate,ka_investigate_mcp}.go | IT-AF-1409-001..005 |
| AF cluster-aware dedup | HandleCheckExistingRR (rrFingerprintWithCluster) | pkg/apifrontend/tools/af_check_existing_rr.go | UT-AF-1409-005/005b/005c, IT-AF-1409-006 |
| AF takeover context reconstruction | resolveInvestigationRR / setTakeoverRRContext | pkg/apifrontend/tools/ka_investigate_mcp.go | UT-AF-1409-011/012/013 |
| AF EventBridge RRContext.ClusterID | RRContext / mergeRRContext / RRContextSafe | pkg/apifrontend/launcher/event_bridge.go | UT-AF-1409-001/001b/001c/001d/002 |
| AF investigation_summary artifact | emitDecisionEvent | pkg/apifrontend/launcher/part_converter.go | UT-AF-1409-006/006b/006c, IT-AF-1409-009 |
| AF execution_progress artifact | BuildProgressSnapshot / crd_tools_watch.go | pkg/apifrontend/tools/execution_progress.go | UT-AF-1409-007/008, IT-AF-1409-007/007b |
| AF Console context banner (full journey) | A2A `message/send` -> investigation_summary DataPart | test/e2e/fullpipeline/10_af_fleet_cluster_id_test.go | E2E-AF-1409-001 |

---

## Business Requirement Linkage

- **BR-FLEET-001**: Fleet remediation requires cluster identity on every RR
- **BR-INTEGRATION-065**: Multi-cluster signal routing and scope gating
- **Issue #54**: Multi-cluster remediation tracking
- **Issue #1409**: Console cluster context banner

---

## Test Plan Reference

| Test ID | Description | Status |
|---------|-------------|--------|
| IT-GW-FLEET-001 | Cluster label propagation to RR spec.clusterID | PASS |
| IT-GW-FLEET-002 | Cluster-aware deduplication (different fingerprints) | PASS |
| IT-GW-FLEET-003 | Backward compat (empty clusterID when no cluster label) | PASS |
| IT-KA-FLEET-001 | Fleet tools visible in RCA phase after AppendFleetToolsToRCA | PASS |
| IT-KA-FLEET-002 | Empty fleet tools do not corrupt phase map | PASS |
| UT/IT/E2E-AF-1409-* | AF cluster_id threading: RRContext, dedup, takeover reconstruction, investigation_summary/execution_progress artifacts, Console banner journey — see [docs/tests/1409/TEST_PLAN.md](../../tests/1409/TEST_PLAN.md) | PASS (full pyramid: 13 UT, 10 IT, 1 E2E — E2E verified against a live Kind cluster) |

---

## FedRAMP Implications

| Control | Impact |
|---------|--------|
| AU-3 (Audit content) | Cluster provenance now recorded per-RR, enabling cluster-scoped audit queries |
| SI-4 (Monitoring) | Cross-cluster signal correlation enabled by `ClusterID` field |
| AC-6 (Least privilege) | Future: RBAC scoped per-cluster identity for multi-tenant fleet |
