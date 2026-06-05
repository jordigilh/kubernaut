# Spike 1: OCP MCP Server Tool Coverage Gap Analysis

**Date**: 2026-06-04
**Status**: Complete
**Objective**: Determine if the OCP MCP server's toolsets cover KA's investigation needs for multi-cluster ServiceNow signal investigation.

## Environment

- **kubernetes-mcp-server** v0.0.62 (generic, `containers/kubernetes-mcp-server`)
  - Available toolsets: `config`, `core`, `helm`, `kcp`, `kiali`, `kubevirt`, `tekton`
- **openshift-mcp-server** (OCP variant, `openshift/openshift-mcp-server`)
  - Additional toolsets: `metrics` (Prometheus/Alertmanager), `traces` (Tempo)
  - Go-native K8s API implementation (not kubectl wrapper)
  - Built-in multi-cluster via kubeconfig contexts (all tools accept optional `context` parameter)

## Coverage Matrix

### K8s Resource Query Tools (7 KA tools)

| KA Tool | OCP MCP Tool | Coverage | Notes |
|---------|-------------|----------|-------|
| `kubectl_describe` | `resources_get` | **FULL** | Both return structured resource data via dynamic client |
| `kubectl_get_by_name` | `resources_get` | **FULL** | Direct equivalent; OCP uses `apiVersion` + `kind` + `name` + `namespace` |
| `kubectl_get_by_name_in_cluster` | `resources_get` (omit namespace) | **FULL** | OCP `resources_get` supports omitting namespace for cluster-scoped lookup |
| `kubectl_get_by_kind_in_namespace` | `resources_list` | **FULL** | Direct equivalent with namespace param |
| `kubectl_get_by_kind_in_cluster` | `resources_list` (omit namespace) | **FULL** | Direct equivalent; omitting namespace lists all namespaces |
| `kubectl_find_resource` | `resources_list` + selectors | **PARTIAL** | OCP uses `fieldSelector`/`labelSelector`; KA uses substring keyword match across JSON. LLM can compose list + filter. |
| `kubectl_get_yaml` | `resources_get` | **PARTIAL** | OCP returns JSON/table; no native YAML output. JSON is functionally equivalent for LLM consumption. |

### Memory Request Tools (2 KA tools)

| KA Tool | OCP MCP Tool | Coverage | Notes |
|---------|-------------|----------|-------|
| `kubectl_memory_requests_all_namespaces` | `pods_top` + `resources_list` | **PARTIAL** | KA reads `spec.containers[].resources.requests` (configured), OCP `pods_top` returns actual usage from metrics-server. LLM can use `resources_list` for Pod specs to extract requests. |
| `kubectl_memory_requests_namespace` | `pods_top` + `resources_list` | **PARTIAL** | Same as above, scoped to namespace. |

### Events Tool (1 KA tool)

| KA Tool | OCP MCP Tool | Coverage | Notes |
|---------|-------------|----------|-------|
| `kubectl_events` | `events_list` | **FULL** | OCP has broader field selector support. KA filters by `involvedObject.name` only; OCP supports all standard field selectors. OCP is arguably better. |

### Log Tools (8 KA tools)

| KA Tool | OCP MCP Tool | Coverage | Notes |
|---------|-------------|----------|-------|
| `kubectl_logs` | `pods_log` | **FULL** | Direct equivalent; OCP supports `tail`, `previous`, `container` params |
| `kubectl_previous_logs` | `pods_log` (`previous=true`) | **FULL** | Direct equivalent |
| `kubectl_logs_all_containers` | `pods_log` (per container) | **PARTIAL** | Would need one `pods_log` call per container. LLM can discover containers via `pods_get` then call `pods_log` per container. |
| `kubectl_container_logs` | `pods_log` (`container` param) | **FULL** | Direct equivalent |
| `kubectl_container_previous_logs` | `pods_log` (`previous` + `container`) | **FULL** | Direct equivalent |
| `kubectl_previous_logs_all_containers` | `pods_log` (per container, `previous`) | **PARTIAL** | Same multi-call pattern as above |
| `kubectl_logs_grep` | No direct equivalent | **GAP** | OCP has no grep/filter parameter. LLM must fetch full logs and scan in context. Mitigated by `tail` param limiting output. |
| `kubectl_logs_all_containers_grep` | No direct equivalent | **GAP** | Same as above, compounded by multi-container. |

### JQ/Aggregation Tools (2 KA tools)

| KA Tool | OCP MCP Tool | Coverage | Notes |
|---------|-------------|----------|-------|
| `kubernetes_jq_query` | No direct equivalent | **GAP** | OCP has no JQ expression support. LLM can use `resources_list` and reason over the JSON directly. For investigation (not data mining), this is acceptable. |
| `kubernetes_count` | No direct equivalent | **GAP** | OCP has no count aggregation. LLM can use `resources_list` and count items. |

### Metrics Tools (2 KA tools)

| KA Tool | OCP MCP Tool | Coverage | Notes |
|---------|-------------|----------|-------|
| `kubectl_top_pods` | `pods_top` | **FULL** | Direct equivalent with richer filtering (label selector, specific pod) |
| `kubectl_top_nodes` | `nodes_top` | **FULL** | Direct equivalent with label selector support |

### Advanced Log Tool (1 KA tool)

| KA Tool | OCP MCP Tool | Coverage | Notes |
|---------|-------------|----------|-------|
| `fetch_pod_logs` | `pods_log` | **PARTIAL** | KA version has time-range filtering, regex include/exclude, multi-container merge, line limit. OCP `pods_log` has `tail` and `previous` only. Time-range and regex are KA-specific features. |

### Prometheus Tools (8 KA tools)

| KA Tool | OCP MCP Tool (metrics toolset) | Coverage | Notes |
|---------|-------------------------------|----------|-------|
| `execute_prometheus_instant_query` | `prometheus_query` | **FULL** | Direct equivalent |
| `execute_prometheus_range_query` | `prometheus_query_range` | **FULL** | Direct equivalent |
| `get_metric_names` | No direct equivalent | **GAP** | OCP `metrics` toolset may have this; not confirmed in current docs |
| `get_label_values` | No direct equivalent | **GAP** | Same as above |
| `get_all_labels` | No direct equivalent | **GAP** | Same as above |
| `get_metric_metadata` | No direct equivalent | **GAP** | Same as above |
| `list_prometheus_rules` | No direct equivalent | **GAP** | Same as above |
| `get_series` | No direct equivalent | **GAP** | Same as above |

**Note**: OCP variant adds `alertmanager_alerts` (query active/pending alerts) which KA does not have -- a bonus.

## OCP MCP Server Bonus Tools (not in KA)

These tools are available in OCP MCP server but KA does NOT currently have:

| OCP MCP Tool | Description | Investigation Value |
|-------------|-------------|---------------------|
| `nodes_log` | Get node-level logs (kubelet, kube-proxy) | **HIGH** -- critical for node-level ServiceNow investigations |
| `nodes_stats_summary` | Detailed node resource stats via kubelet Summary API | **HIGH** -- CPU, memory, filesystem, network, PSI metrics |
| `pods_exec` | Execute commands inside pod containers | **MEDIUM** -- useful for advanced debugging (security gated by `--read-only`) |
| `pods_run` | Run a pod from an image | **LOW** -- not needed for investigation |
| `alertmanager_alerts` | Query Alertmanager for active/pending alerts | **HIGH** -- cross-reference ServiceNow tickets with active alerts |
| `namespaces_list` | List all namespaces | **MEDIUM** -- useful for cluster overview |
| `resources_create_or_update` | Create/update resources | **LOW** -- blocked by `--read-only` mode |
| `resources_scale` | Scale workloads | **LOW** -- blocked by `--read-only` mode |

## Coverage Summary

### K8s Investigation Tools (22 KA tools)

| Coverage | Count | Percentage | Tools |
|----------|-------|------------|-------|
| **FULL** | 12 | 55% | describe, get_by_name (x2), get_by_kind (x2), events, logs (x4), top_pods, top_nodes |
| **PARTIAL** | 6 | 27% | find_resource, get_yaml, memory_requests (x2), logs_all_containers (x2) |
| **GAP** | 4 | 18% | kubernetes_jq_query, kubernetes_count, kubectl_logs_grep, kubectl_logs_all_containers_grep |

### Prometheus Tools (8 KA tools)

| Coverage | Count | Percentage |
|----------|-------|------------|
| **FULL** | 2 | 25% |
| **GAP** | 6 | 75% |

### Overall (30 KA tools)

| Coverage | Count | Percentage |
|----------|-------|------------|
| **FULL** | 14 | 47% |
| **PARTIAL** | 6 | 20% |
| **GAP** | 10 | 33% |

## Gap Impact Assessment for ServiceNow Investigation

### Critical for Investigation?

| Gap Tool | Impact on ServiceNow Investigation | Mitigation |
|----------|------------------------------------|------------|
| `kubernetes_jq_query` | **LOW** -- complex JQ queries are a power-user feature; LLM can reason over raw JSON from `resources_get`/`resources_list` | LLM processes JSON directly |
| `kubernetes_count` | **LOW** -- counting is easily done by LLM from list results | LLM counts from `resources_list` output |
| `kubectl_logs_grep` | **MEDIUM** -- log grep is useful for finding specific patterns | LLM scans logs after `pods_log` fetch; `tail` param limits volume |
| `kubectl_logs_all_containers_grep` | **LOW** -- edge case (grep across all containers) | Multi-call `pods_log` + LLM scanning |
| `fetch_pod_logs` (partial) | **MEDIUM** -- time-range filtering is useful but not critical for investigation | `pods_log` with `tail` param covers most cases |
| Prometheus metadata tools (6) | **LOW for POC** -- metadata discovery tools help the LLM find the right metrics. Core PromQL queries (`instant` + `range`) ARE covered. | LLM can query known metrics directly. KA prompt can include common metric names. |

### Net Assessment

**For K8s investigation tools: 82% coverage (FULL + PARTIAL) is sufficient for POC.**

The 4 K8s gap tools (`jq_query`, `count`, `logs_grep`, `logs_all_containers_grep`) are convenience tools that the LLM can work around. The OCP MCP server's `nodes_log`, `nodes_stats_summary`, and `alertmanager_alerts` bonus tools actually ADD investigation capability that KA currently lacks.

**For Prometheus: 25% FULL coverage is low but acceptable for POC.** The 2 core PromQL query tools are covered. Metadata discovery tools help the LLM explore unfamiliar Prometheus instances but are not required when the LLM is prompted with known metric names.

## Decision

**GO**: Use OCP MCP server `core` + `metrics` toolsets for remote cluster investigation via MCP Gateway.

**Architecture**:
- **Local cluster** (K8s-originated signals): Continue using KA's native client-go tools (no change)
- **Remote clusters** (ServiceNow signals): Use OCP MCP server tools via MCP Gateway
- **Prometheus** for remote clusters: Use OCP `metrics` toolset for core queries; defer metadata discovery

**Rationale**:
1. 82% K8s tool coverage exceeds the 80% threshold
2. Bonus tools (nodes_log, nodes_stats_summary, alertmanager_alerts) add value for ServiceNow investigation
3. Gap tools are non-critical and have LLM-based workarounds
4. OCP MCP server is Go-native, read-only mode supported, multi-cluster built-in
5. Same SDK (`modelcontextprotocol/go-sdk`) already in Kubernaut's go.mod
