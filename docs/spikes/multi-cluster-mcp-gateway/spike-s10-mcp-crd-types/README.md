# Spike S10 — K8s MCP Server with CRD Types (Non-Core Resources)

## Objective

Validate that the real `kubernetes-mcp-server` binary can list, get, and filter
Custom Resource Definition instances (non-core types) via envtest — specifically
`RemediationRequest` (`kubernaut.ai/v1alpha1`).

## Findings

### PASS — All 6 test scenarios validated

| Test | Description | Result |
|------|-------------|--------|
| S10-001 | `resources_list` returns CRD instances | PASS |
| S10-002 | `resources_list` with `labelSelector` filters CRD instances | PASS |
| S10-003 | `resources_get` retrieves specific CRD with full spec (incl. `clusterID`) | PASS |
| S10-004 | `resources_get` returns `isError=true` for non-existent CRD | PASS |
| S10-005 | Cluster-scoped resources (ClusterRole) list correctly | PASS |
| S10-006 | Full scope check pattern works on CRD instances | PASS |

### Key Findings

1. **CRDs work identically to core resources**: `resources_list` and `resources_get`
   handle `kubernaut.ai/v1alpha1/RemediationRequest` with the same arguments as
   built-in types (`apps/v1/Deployment`).

2. **`labelSelector` works on CRDs**: Filtering by `kubernaut.ai/managed=true` correctly
   returns only labeled CRD instances, excluding unlabeled ones.

3. **Full spec is returned**: `resources_get` includes all CRD spec fields (e.g.,
   `clusterID: prod-east`, `signalName: HighCPU`, `targetResource`).

4. **Error handling**: Non-existent CRD instances return `isError=true` with a clear
   "not found" message — same behavior as core resources.

5. **`apiVersion` format**: CRDs use the full `group/version` format (e.g.,
   `kubernaut.ai/v1alpha1`), which the K8s MCP Server resolves correctly.

6. **Cluster-scoped resources**: `ClusterRole` (`rbac.authorization.k8s.io/v1`)
   also works, confirming the pattern for `MCPServerRegistration` (`mcp.kuadrant.io/v1alpha1`).

### Performance

- Total test time (6 tests + envtest + CRD installation): **6.5 seconds**
- CRD installation adds ~2s vs S8's core-resource-only tests (3.4s)

### Risk Removed

The concern "K8s MCP Server behavior on CRD types" is now fully de-risked:
- FMC Writer can list `MCPServerRegistration` CRDs to discover clusters
- FMC Writer can list any CRD type with `labelSelector` for scope cache population
- Scope checks via MCP work for both namespace labels and CRD instance labels

### apiVersion Format Reference

| Resource | apiVersion for MCP calls |
|----------|-------------------------|
| Pod, Namespace, Service | `v1` |
| Deployment, StatefulSet | `apps/v1` |
| RemediationRequest | `kubernaut.ai/v1alpha1` |
| MCPServerRegistration | `mcp.kuadrant.io/v1alpha1` |
| ClusterRole | `rbac.authorization.k8s.io/v1` |
