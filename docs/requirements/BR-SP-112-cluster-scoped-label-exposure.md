# BR-SP-112: Cluster-Scoped Resource Label Exposure

**Document Version**: 1.0
**Date**: May 13, 2026
**Status**: DRAFT
**Category**: Enrichment
**Priority**: P1 (High)
**Service**: SignalProcessing
**GitHub Issue**: [#1110](https://github.com/jordigilh/kubernaut/issues/1110)
**Related**: BR-SP-001, BR-SP-080, BR-SP-081, BR-SP-102, DD-017

---

## Business Context

### Problem Statement

The Signal Processing enrichment pipeline assumes all target resources are namespace-scoped. When processing cluster-scoped resources (Node, PersistentVolume, ClusterRole), several code paths silently skip label extraction because they guard on `Namespace != nil`:

1. **`classifyBusiness`** only reads labels from `KubernetesContext.Namespace`; workload labels on Node/PV resources are ignored, producing empty `BusinessUnit` and `ServiceOwner`.
2. **CustomLabels fallback** (BR-SP-102) only extracts `kubernaut.ai/*` labels from namespace; workload labels are never consulted.
3. **`#437 guard`** in `reconcileClassifying` treats `Namespace == nil` as "enrichment incomplete" and requeues, even though cluster-scoped enrichment legitimately produces nil Namespace.
4. **`enrichNodeSignal`** returns a hard error on Node NotFound instead of entering degraded mode (unlike Pod/Deployment/StatefulSet/DaemonSet/ReplicaSet paths which all support graceful degradation per DD-017 Principle 3).
5. **`enrichNamespaceOnly`** (unknown kind fallback) calls `getNamespace("")` for cluster-scoped kinds with empty namespace, producing an error instead of a meaningful context.
6. **`BuildDegradedContext`** always creates a `Namespace` struct even for cluster-scoped resources, and never populates `Workload`.

### Business Value

1. **Node signal support**: Kubernaut must process Node-targeted alerts (e.g., `NodeNotReady`, `DiskPressure`) end-to-end without classification gaps.
2. **Correct business classification**: Node labels (e.g., `kubernaut.ai/business-unit`, `kubernaut.ai/tier`) must flow into business classification and SLA assignment.
3. **Graceful degradation parity**: All resource kinds must support degraded mode on NotFound, per DD-017 Principle 3.
4. **Rego policy completeness**: Rego policies receive both namespace and workload context via `PolicyInput`; missing workload labels produce incomplete classification decisions.

---

## Requirements

### R1: Workload Label Fallback in Business Classification

`classifyBusiness` MUST check `KubernetesContext.Workload.Labels` when `Namespace` is nil or when namespace labels do not contain the required `kubernaut.ai/*` keys.

**Priority order**:
1. Namespace labels (existing behavior for namespace-scoped resources)
2. Workload labels (fallback for cluster-scoped resources or when namespace labels are absent)

### R2: Workload Label Fallback in CustomLabels

The CustomLabels fallback path (BR-SP-102) MUST also consult `KubernetesContext.Workload.Labels` when `Namespace` is nil.

### R3: Cluster-Scoped Enrichment Guard

The `#437 guard` in `reconcileClassifying` MUST NOT treat nil Namespace as "enrichment incomplete" when the target resource is cluster-scoped (e.g., Kind == "Node"). Cluster-scoped resources legitimately have nil Namespace after enrichment.

### R4: Node NotFound Degraded Mode

`enrichNodeSignal` MUST enter degraded mode on `apierrors.IsNotFound` instead of returning a hard error, matching the behavior of all other resource kind enrichers.

### R5: Unknown Cluster-Scoped Kind Handling

`enrichNamespaceOnly` MUST handle cluster-scoped kinds (empty namespace) gracefully. When `signal.TargetResource.Namespace` is empty, enrichment should return a context with nil Namespace and DegradedMode=true, rather than attempting to fetch an empty namespace name.

### R6: Degraded Context Workload Population

`BuildDegradedContext` SHOULD populate `Workload` from the signal's target resource metadata (Kind, Name) when available. For cluster-scoped resources, it MUST NOT create a `Namespace` with an empty name.

---

## Acceptance Criteria

- [ ] `classifyBusiness` extracts `kubernaut.ai/business-unit`, `kubernaut.ai/team`, `kubernaut.ai/service-owner` from workload labels when namespace labels are absent
- [ ] CustomLabels fallback checks workload labels when namespace is nil
- [ ] `#437 guard` allows classification to proceed for cluster-scoped resources with nil Namespace
- [ ] `enrichNodeSignal` returns `DegradedMode=true` on Node NotFound instead of error
- [ ] `enrichNamespaceOnly` returns degraded context for empty namespace (cluster-scoped kinds)
- [ ] `BuildDegradedContext` populates `Workload` with target Kind/Name; skips Namespace for cluster-scoped resources
- [ ] Existing Pod/Deployment/StatefulSet enrichment behavior is unchanged (regression guard)
- [ ] All tests pass with `-race` flag

---

## Implementation Points

| Component | File(s) | Change |
|---|---|---|
| Business classifier | `internal/controller/signalprocessing/signalprocessing_controller.go` | `classifyBusiness`: add workload label fallback |
| CustomLabels fallback | `internal/controller/signalprocessing/signalprocessing_controller.go` | Add workload label extraction when namespace nil |
| Classification guard | `internal/controller/signalprocessing/signalprocessing_controller.go` | `#437 guard`: check target kind before requeue |
| Node enricher | `pkg/signalprocessing/enricher/k8s_enricher.go` | `enrichNodeSignal`: add NotFound degraded branch |
| Unknown kind enricher | `pkg/signalprocessing/enricher/k8s_enricher.go` | `enrichNamespaceOnly`: handle empty namespace |
| Degraded context | `pkg/signalprocessing/enricher/degraded.go` | `BuildDegradedContext`: populate Workload, conditional Namespace |

---

## Test Plan

### Unit Tests (Phase 2 RED)
- Table-driven tests for `classifyBusiness` with nil Namespace but populated Workload labels
- CustomLabels fallback extraction from workload labels
- `#437 guard` bypass for Node target kind
- `enrichNodeSignal` NotFound → DegradedMode=true
- `enrichNamespaceOnly` with empty namespace → degraded context
- `BuildDegradedContext` cluster-scoped output (no Namespace, Workload populated)
- D4: `NewK8sEnricher` panic specification test on nil metrics

### Regression Guards
- Existing Pod/Deployment enrichment paths unchanged
- Existing namespace-scoped `classifyBusiness` behavior unchanged

---

## References

- [BR-SP-001: K8s Context Enrichment](../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md)
- [BR-SP-080/081: Business Classification](../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md)
- [BR-SP-102: Custom Labels](../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md)
- [DD-017: Graceful Degradation Principles](../architecture/decisions/)
- [Issue #437: Stale cache guard](https://github.com/jordigilh/kubernaut/issues/437)
- [Issue #1110: SP Multi-Dimension Readiness Audit](https://github.com/jordigilh/kubernaut/issues/1110)
