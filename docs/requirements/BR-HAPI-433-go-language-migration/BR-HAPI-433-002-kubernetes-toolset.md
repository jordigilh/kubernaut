# BR-HAPI-433-002: Kubernetes Toolset Scope

**Parent**: [BR-HAPI-433: Go Language Migration](BR-HAPI-433-go-language-migration.md)
**Category**: HolmesGPT-API Service
**Priority**: P0
**Status**: ✅ Approved — Tier 1+2 (11 tools)
**Date**: 2026-03-04
**GitHub Issue**: [#508](https://github.com/jordigilh/kubernaut/issues/508)

---

## 📋 **Business Need**

HolmesGPT provides 25 Kubernetes tools across 5 YAML-defined toolsets, all implemented as `kubectl` subprocess calls. The Go rewrite must determine which tools to reimplement for v1.3 feature parity and which to defer.

---

## 📊 **Current State: HolmesGPT Kubernetes Tools (25 tools)**

All 25 tools execute via `subprocess.run(cmd, shell=True, executable="/bin/bash")`.

### kubernetes/core (11 tools)

| Tool | Description |
|---|---|
| `kubectl_describe` | Describe a resource (has `llm_summarize` for outputs >1000 chars) |
| `kubectl_get_by_name` | Get a specific resource with labels |
| `kubectl_get_by_kind_in_namespace` | List resources by kind in namespace (has `llm_summarize`) |
| `kubectl_get_by_kind_in_cluster` | List resources by kind cluster-wide (has `llm_summarize`) |
| `kubectl_find_resource` | Search resources by keyword |
| `kubectl_get_yaml` | Full YAML dump of a resource |
| `kubectl_events` | Events for a specific resource |
| `kubectl_memory_requests_all_namespaces` | Memory requests for all pods (complex awk script) |
| `kubectl_memory_requests_namespace` | Memory requests for pods in a namespace |
| `kubernetes_jq_query` | Arbitrary jq filtering on resources (has `llm_summarize`) |
| `kubernetes_count` | Count resources matching a jq filter |

### kubernetes/logs (8 tools)

| Tool | Description |
|---|---|
| `kubectl_logs` | Current pod logs (has `llm_summarize`) |
| `kubectl_logs_all_containers` | Current logs, all containers (has `llm_summarize`) |
| `kubectl_container_logs` | Current logs, specific container |
| `kubectl_previous_logs` | Previous container logs (crashed pod) |
| `kubectl_previous_logs_all_containers` | Previous logs, all containers |
| `kubectl_container_previous_logs` | Previous logs, specific container |
| `kubectl_logs_grep` | Search pod logs |
| `kubectl_logs_all_containers_grep` | Search all containers logs |

### kubernetes/live-metrics (2 tools)

| Tool | Description |
|---|---|
| `kubectl_top_pods` | Real-time pod CPU/memory via metrics-server |
| `kubectl_top_nodes` | Real-time node CPU/memory via metrics-server |

### kubernetes/kube-prometheus-stack (1 tool)

| Tool | Description |
|---|---|
| `get_prometheus_target` | Fetch Prometheus scrape target definition |

### kubernetes/krew-extras & kube-lineage-extras (2 unique tools)

| Tool | Description |
|---|---|
| `kubectl_lineage_children` | Get children of a resource via ownerReferences |
| `kubectl_lineage_parents` | Get parents of a resource (reverse traversal) |

---

## 🔧 **Tier Analysis**

### Tier 1 — Core Investigation Tools (8 tools) ✅ INCLUDE

Used in every incident investigation flow. The investigation prompt explicitly guides the LLM to use these.

| Tool | Why Essential | Go Implementation |
|---|---|---|
| `kubectl_describe` | Primary tool for resource state, conditions, events | `client.Resource(gvr).Get()` → structured summary |
| `kubectl_get_by_name` | Check resource existence, status, labels | `client.Resource(gvr).Get()` → format with labels |
| `kubectl_get_by_kind_in_namespace` | List pods/resources to find related resources | `client.Resource(gvr).Namespace(ns).List()` |
| `kubectl_events` | K8s events (OOMKilled, FailedScheduling, BackOff) — critical for RCA | `client.CoreV1().Events().List()` with field selector |
| `kubectl_logs` | Container logs — primary evidence for RCA | `client.CoreV1().Pods(ns).GetLogs(pod, &PodLogOptions{})` |
| `kubectl_previous_logs` | Crashed container logs — essential for CrashLoopBackOff/OOMKilled | `GetLogs(pod, &PodLogOptions{Previous: true})` |
| `kubectl_logs_all_containers` | Multi-container pod investigation (sidecars, init containers) | `GetLogs()` per container in pod spec |
| `kubectl_container_logs` | Targeted container investigation in multi-container pods | `GetLogs(pod, &PodLogOptions{Container: c})` |

### Tier 2 — Useful Supporting Tools (3 tools) ✅ INCLUDE

Support common investigation patterns and improve token efficiency.

| Tool | Why Useful | Go Implementation |
|---|---|---|
| `kubectl_container_previous_logs` | Previous logs for a specific container (multi-container crash) | `GetLogs(pod, &PodLogOptions{Container: c, Previous: true})` |
| `kubectl_previous_logs_all_containers` | Previous logs for all containers (multi-container crash) | `GetLogs()` per container with `Previous: true` |
| `kubectl_logs_grep` | Search logs for specific errors — reduces token usage vs full log pull. Also benefits the tool-output sanitizer (less untrusted content in context). | `GetLogs()` + Go `strings.Contains` line filter |

### Tier 3 — Deferred (14 tools) ❌ NOT IN v1.3

| Tool | Why Deferred | Alternative |
|---|---|---|
| `kubectl_get_by_kind_in_cluster` | Cluster-wide listing rarely needed for targeted RCA | `kubectl_get_by_kind_in_namespace` covers most cases |
| `kubectl_find_resource` | Grep-by-keyword convenience | LLM can use `kubectl_get_by_kind_in_namespace` + filter |
| `kubectl_get_yaml` | Full YAML dump — `kubectl_describe` gives same info more concisely | `kubectl_describe` |
| `kubectl_memory_requests_all_namespaces` | Niche memory analysis | Prometheus `kube_pod_container_resource_requests` is better (historical) |
| `kubectl_memory_requests_namespace` | Same | Same |
| `kubernetes_jq_query` | Arbitrary jq filtering — too general, hard to sanitize | client-go `List()` + Go filtering for specific needs |
| `kubernetes_count` | Resource counting — niche | `List()` + `len()` if ever needed |
| `kubectl_top_pods` | Real-time snapshot only (last ~15s). Requires metrics-server. | Prometheus `container_memory_working_set_bytes` provides historical data |
| `kubectl_top_nodes` | Same limitation | Prometheus `node_memory_MemAvailable_bytes` |
| `get_prometheus_target` | Niche — fetching Prometheus scrape target config | Can be added later |
| `kubectl_lineage_children` | **Redundant** — `get_resource_context` already does ownerRef traversal | `get_resource_context` (HAPI-custom toolset) |
| `kubectl_lineage_parents` | **Redundant** — same as above, reverse direction | `get_resource_context` |
| `kubectl_logs_all_containers_grep` | Marginal improvement over `kubectl_logs_grep` | Can be added if needed |

---

## 🔧 **Cross-Cutting: llm_summarize Transformer**

Several HolmesGPT tools apply `llm_summarize` when output exceeds 1000 chars — a secondary LLM call to reduce context window usage. This is carried forward as a **Kubernaut-owned feature**: if tool output exceeds a configurable threshold, summarize via secondary LLM call before returning to the investigation loop.

---

## ✅ **Decision**

**v1.3 scope: Tier 1 + Tier 2 = 11 tools**, implemented via `k8s.io/client-go`.

- No kubectl binary in the image
- No shell execution
- Structured output (Go structs → JSON) rather than kubectl text table format
- Built-in size control (TailLines, LimitBytes) for log tools
- Tier 3 deferred to v1.4+ or MCP extensibility

---

**Document Version**: 1.0
**Last Updated**: 2026-03-04
