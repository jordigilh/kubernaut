# Spike 4: Cluster Context Resolution

**Date**: 2026-06-04
**Status**: Complete
**Objective**: Validate that CMDB CI -> cluster context mapping works end-to-end and decide on the routing strategy.

## The Routing Problem

When KA investigates a ServiceNow signal, it needs to determine WHICH cluster to query. The signal's `ProviderData` contains the CMDB CI sys_id for a cluster or node. KA must map this CI to a specific cluster in the MCP Gateway.

```
ServiceNow Ticket (INC0067890)
  └── CMDB CI: sys_id="abc123", name="prod-cluster-east"
         └── KA needs to call: cluster_prod_east_pods_list (or similar)
```

## Option Analysis

### Option A: Per-Cluster MCP Servers with Tool Prefix

Each workload cluster runs its own OCP MCP server, registered with the MCP Gateway with a unique `prefix`. Tools appear as `cluster_a_pods_list`, `cluster_b_resources_get`.

**Mapping**: CMDB CI name -> tool prefix

```
CMDB CI name: "prod-cluster-east"
  -> Gateway prefix: "prod_east_"
  -> Tool: "prod_east_pods_list"
```

**Pros**:
- Clean isolation per cluster
- Security: each MCP server has its own ServiceAccount with least-privilege
- Failure isolation: one cluster's MCP server failing doesn't affect others
- Scale: add new clusters by deploying a new MCP server + registration

**Cons**:
- More resources: one MCP server pod per cluster
- Tool name explosion: N clusters x M tools = N*M tool names visible to the LLM
- Prefix mapping requires a lookup table (CMDB CI name -> prefix)

### Option B: Single Multi-Context MCP Server

One OCP MCP server with a kubeconfig containing all cluster contexts. All tools accept an optional `context` parameter.

**Mapping**: CMDB CI name -> kubeconfig context name

```
CMDB CI name: "prod-cluster-east"
  -> Context: "prod-cluster-east"
  -> Tool: "pods_list" with args: {"namespace": "default", "context": "prod-cluster-east"}
```

**Pros**:
- Simpler deployment: one server instance
- Tool names stay clean (no prefix explosion)
- LLM sees a single, familiar tool set

**Cons**:
- Single point of failure for all clusters
- Security: one ServiceAccount needs access to all clusters (violates least-privilege)
- Kubeconfig management: adding a cluster requires restarting the MCP server
- Not suitable for cross-network clusters (clusters must be reachable from the MCP server)

### Option C: Hybrid -- Per-Cluster Servers, KA Selects by Server

Each workload cluster has its own MCP server, but instead of tool prefixes, KA connects to each server directly (bypassing the gateway for tool routing). KA uses `ProviderData` to select which server to connect to.

**Pros**:
- No prefix pollution
- Direct connection to the right cluster

**Cons**:
- Loses gateway benefits (auth, rate limiting, aggregation)
- KA needs to manage multiple MCP client connections
- More complex session management

## Decision: Option A -- Per-Cluster Servers with Tool Prefix

**Rationale**:

1. **Security**: Each cluster has its own ServiceAccount with least-privilege RBAC. A compromised MCP server on cluster A cannot access cluster B.

2. **Failure isolation**: If cluster A's MCP server is down, cluster B's investigation still works. This is critical for customer demos.

3. **Scale**: Adding a new cluster is declarative -- deploy MCP server + create MCPServerRegistration. No restart of existing components.

4. **Gateway benefits preserved**: Auth, rate limiting, observability all work at the gateway level.

5. **Tool name explosion is manageable**: For a POC with 2-3 clusters, the LLM context has ~40-60 tools total (20 tools * 2-3 clusters). The LLM handles this well, especially with clear prefix naming. The prompt template will instruct the LLM which prefix to use based on `ProviderData`.

## CMDB CI -> Cluster Prefix Mapping

### Mapping Table

The mapping is stored as a ConfigMap in the management cluster, loaded at KA startup:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-cluster-map
  namespace: kubernaut-system
data:
  cluster-map.yaml: |
    clusters:
      - cmdb_ci_name: "prod-cluster-east"
        cmdb_ci_sys_id: "abc123def456"
        mcp_prefix: "prod_east_"
        display_name: "Production East"
      - cmdb_ci_name: "staging-cluster"
        cmdb_ci_sys_id: "789ghi012jkl"
        mcp_prefix: "staging_"
        display_name: "Staging"
```

### Resolution Logic

```go
// ClusterResolver maps ServiceNow CMDB CI identifiers to MCP tool prefixes.
type ClusterResolver struct {
    byName  map[string]ClusterMapping  // CMDB CI name -> mapping
    bySysID map[string]ClusterMapping  // CMDB CI sys_id -> mapping
}

type ClusterMapping struct {
    CMDBCIName   string // e.g., "prod-cluster-east"
    CMDBCISysID  string // e.g., "abc123def456"
    MCPPrefix    string // e.g., "prod_east_"
    DisplayName  string // e.g., "Production East"
}

// Resolve returns the MCP tool prefix for a given CMDB CI.
// Tries sys_id first (exact match), falls back to name (case-insensitive).
func (r *ClusterResolver) Resolve(cmdbCIName, cmdbCISysID string) (ClusterMapping, error) {
    if cmdbCISysID != "" {
        if m, ok := r.bySysID[cmdbCISysID]; ok {
            return m, nil
        }
    }
    if cmdbCIName != "" {
        if m, ok := r.byName[strings.ToLower(cmdbCIName)]; ok {
            return m, nil
        }
    }
    return ClusterMapping{}, fmt.Errorf("no cluster mapping for CI name=%q sys_id=%q", cmdbCIName, cmdbCISysID)
}
```

### Integration with KA Investigation

When KA receives a ServiceNow signal:

1. **Extract CMDB CI** from `ProviderData` (JSON):
   ```json
   {
     "cmdb_ci": {
       "sys_id": "abc123def456",
       "name": "prod-cluster-east",
       "sys_class_name": "cmdb_ci_kubernetes_cluster"
     }
   }
   ```

2. **Resolve cluster prefix** via `ClusterResolver`:
   ```go
   mapping, err := resolver.Resolve(ci.Name, ci.SysID)
   // mapping.MCPPrefix = "prod_east_"
   ```

3. **Filter tools for this cluster**: Only expose tools with matching prefix to the LLM:
   ```go
   func ServiceNowPhaseToolMap(prefix string, allMCPTools []string) PhaseToolMap {
       var clusterTools []string
       for _, t := range allMCPTools {
           if strings.HasPrefix(t, prefix) {
               clusterTools = append(clusterTools, t)
           }
       }
       return PhaseToolMap{
           PhaseRCA: clusterTools,
           // ... workflow tools remain the same
       }
   }
   ```

4. **LLM prompt includes cluster context**: The investigation prompt tells the LLM which cluster it's investigating and which tool prefix to use.

## Impact on KA's ServiceNowPhaseToolMap

The existing design for `ServiceNowPhaseToolMap()` must be parameterized by cluster prefix:

| Field | Source | Example |
|-------|--------|---------|
| `prefix` | Resolved from CMDB CI at investigation start | `"prod_east_"` |
| RCA tools | All MCP tools matching the prefix | `["prod_east_pods_list", "prod_east_resources_get", ...]` |
| ServiceNow tools | Registered locally (no prefix) | `["servicenow_get_ticket", "servicenow_query_maintenance"]` |
| Workflow tools | Same as K8s (no prefix) | `["list_available_actions", "list_workflows", "get_workflow"]` |

The RCA phase tool list for ServiceNow investigations becomes:
```
[prod_east_pods_list, prod_east_resources_get, prod_east_events_list, ...]  # MCP bridge tools
+ [servicenow_get_ticket, servicenow_query_maintenance]                      # ServiceNow API tools
+ [TodoWrite]                                                                # common tools
```

## Deliverables

- [x] Routing strategy documented (Option A: per-cluster servers with tool prefix)
- [x] CMDB CI -> cluster prefix mapping design (ConfigMap + ClusterResolver)
- [x] Impact on `ServiceNowPhaseToolMap()` documented
- [x] Decision: per-cluster servers (with prefix) vs single multi-context
