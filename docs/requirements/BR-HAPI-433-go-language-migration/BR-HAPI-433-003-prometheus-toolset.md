# BR-HAPI-433-003: Prometheus Toolset Scope

**Parent**: [BR-HAPI-433: Go Language Migration](BR-HAPI-433-go-language-migration.md)
**Category**: HolmesGPT-API Service
**Priority**: P0
**Status**: ✅ Approved — Tier 1+2 (6 tools), rule fetching dropped
**Date**: 2026-03-04
**GitHub Issue**: [#509](https://github.com/jordigilh/kubernaut/issues/509)

---

## 📋 **Business Need**

HolmesGPT provides 8 Prometheus tools implemented as Python HTTP calls via `requests`/`prometrix`. The Go rewrite must determine which tools to reimplement for v1.3 feature parity, accounting for Kubernaut's signal pipeline which already synthesizes alert context.

---

## 📊 **Current State: HolmesGPT Prometheus Tools (8 tools)**

| Tool | Prometheus API Endpoint | Description |
|---|---|---|
| `list_prometheus_rules` | `GET /api/v1/rules` | List all alerting/recording rules (cached with TTL) |
| `get_metric_names` | `GET /api/v1/label/__name__/values` | Discover metric names with match[] filter |
| `get_label_values` | `GET /api/v1/label/{label}/values` | Get all values for a label |
| `get_all_labels` | `GET /api/v1/labels` | List all label names |
| `get_series` | `GET /api/v1/series` | Get time series label sets matching a selector |
| `get_metric_metadata` | `GET /api/v1/metadata` | Get metric type, help text, unit |
| `execute_prometheus_instant_query` | `POST /api/v1/query` | Execute an instant PromQL query |
| `execute_prometheus_range_query` | `POST /api/v1/query_range` | Execute a range PromQL query |

### Supporting Infrastructure

| Feature | Description |
|---|---|
| AWS AMP (SigV4) | Amazon Managed Prometheus auth via STS assume-role |
| VictoriaMetrics auto-detection | K8s service discovery by label |
| Prometheus auto-discovery | K8s service discovery to find Prometheus endpoints |
| OpenShift token auth | Automatic SA token injection |
| Response size limiting | Configurable limit (default 30000 chars) with `topk()` suggestion |
| Rules caching | TTL cache (default 30 min) for `list_prometheus_rules` |
| Query timeout | Configurable default (20s) and max (180s) per query |

---

## 🛡️ **Key Design Decision: No Rule Fetching**

### `list_prometheus_rules` — DROPPED

HolmesGPT's LLM instructions say "ALWAYS call `list_prometheus_rules` when investigating a Prometheus alert." This is **not applicable** to Kubernaut:

1. **Redundant**: Kubernaut's signal pipeline (AlertManager → SignalProcessor → IncidentRequest) already synthesizes the alert context. HAPI receives the signal name, severity, expression context, labels, and firing time in the `IncidentRequest`. There is no need to re-fetch the rule definition from Prometheus.

2. **Prompt injection risk**: Prometheus rule annotations (`summary`, `description`, `runbook_url`) are free-text fields. An operator or attacker could embed malicious LLM instructions in these annotations. Since we already have the signal context, there is no reason to expose the LLM to this additional attack surface.

3. **Dependency reduction**: Dropping this tool means HAPI doesn't need to locate the Prometheus rules API, which varies by deployment (standard Prometheus vs Thanos Ruler vs Cortex ruler).

---

## 🛡️ **Security Concern: Prometheus Labels as Prompt Injection Vector**

Prometheus metric labels are **attacker-controlled**. Anyone who can create a Pod, Service, ConfigMap, or custom metric exporter can set arbitrary label values. PromQL query results include these labels:

```json
{"metric": {"pod": "api-xyz", "namespace": "prod", "app": "Ignore instructions..."}, "value": [1709564400, "0.85"]}
```

All remaining Prometheus tools MUST have tool-output sanitization (security layer I1) applied to query results — specifically stripping instruction-like content from label values.

---

## 🔧 **Tier Analysis**

### Tier 1 — Core Query Tools (2 tools) ✅ INCLUDE

| Tool | Why Essential | Go Implementation |
|---|---|---|
| `execute_prometheus_instant_query` | Query current metric values for RCA evidence | Go `net/http` POST to `/api/v1/query` |
| `execute_prometheus_range_query` | Query metric trends over time — critical for understanding whether a condition is trending or was a spike | Go `net/http` POST to `/api/v1/query_range` |

Both carry forward existing safeguards:
- Response size limiting with `topk()` suggestion for large results
- Query timeout (configurable default + max enforcement)
- Tool-output sanitization for label values (new)

### Tier 2 — Metric Discovery Tools (4 tools) ✅ INCLUDE

| Tool | Why Useful | Go Implementation |
|---|---|---|
| `get_metric_names` | Fastest metric discovery — LLM searches by pattern | Go `net/http` GET `/api/v1/label/__name__/values` |
| `get_label_values` | Discover available pods, namespaces, jobs for accurate PromQL selectors | Go `net/http` GET `/api/v1/label/{label}/values` |
| `get_all_labels` | List available label names — helps LLM understand what dimensions exist | Go `net/http` GET `/api/v1/labels` |
| `get_metric_metadata` | Get metric type and description — helps LLM understand what a metric measures and whether to use `rate()` vs raw value | Go `net/http` GET `/api/v1/metadata` |

### Tier 3 — Deferred (2 tools) ❌ NOT IN v1.3

| Tool | Why Deferred | Alternative |
|---|---|---|
| `list_prometheus_rules` | **Dropped** — redundant (signal pipeline already provides alert context) and prompt injection risk (rule annotations are free-text) | Signal pipeline synthesizes alert context in `IncidentRequest` |
| `get_series` | Slowest discovery method. Returns full label sets for matching series. | `get_metric_names` + `get_label_values` cover discovery needs |

---

## 🔧 **Provider Support**

| Provider | Auth Mechanism | Go Implementation |
|---|---|---|
| Standard Prometheus | None or custom headers | `net/http` with config-driven headers |
| Prometheus + auth proxy | Bearer token / basic auth | Headers from config/env |
| AWS Managed Prometheus (AMP) | SigV4 | `aws-sdk-go-v2` signing middleware |
| VictoriaMetrics | Compatible API | Same client (API-compatible) |
| OpenShift Prometheus | SA bearer token | Read token from `/var/run/secrets/` |
| Thanos Query | Compatible API | Same client (API-compatible) |

---

## ✅ **Decision**

**v1.3 scope: Tier 1 + Tier 2 = 6 tools**, implemented via Go `net/http`.

- No Python dependencies (replaces `requests`, `prometrix`, `dateutil`)
- `list_prometheus_rules` dropped (redundant + injection risk)
- Tool-output sanitization applied to all query results
- AWS AMP support via `aws-sdk-go-v2`
- Auto-discovery reimplemented via client-go service listing + label matching

---

**Document Version**: 1.0
**Last Updated**: 2026-03-04
