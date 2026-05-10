# RB-AF-009: MCP Bridge Troubleshooting

## Overview

The MCP bridge dispatches tool calls from AI agents to backend services (K8s CRDs,
KA REST/MCP, DataStorage). This runbook covers diagnosing failures in the bridge.

## Alerts

| Alert | Meaning |
|-------|---------|
| `ApifrontendToolLatencyHigh` | Tool P99 > 5s — slow downstream |
| `ApifrontendMCPRBACDenialSpike` | Sustained RBAC denials > 1/s — misconfigured roles or attack |
| `ApifrontendToolErrorRate` | >5% of tool calls failing — service degradation |
| `ApifrontendToolThrottling` | Concurrency limit saturated — capacity issue |

## Key Metrics

```promql
# Tool call breakdown by result
sum by (tool, result) (rate(af_tool_calls_total[5m]))

# RBAC denials by tool
sum by (tool) (rate(af_mcp_rbac_denied_total[5m]))

# P99 latency per tool
histogram_quantile(0.99, sum by (le, tool) (rate(af_tool_call_duration_seconds_bucket[5m])))

# Throttle rate
sum(rate(af_tool_calls_total{result="throttled"}[5m]))
```

## Configuration Reference

| Parameter | Default | Description |
|-----------|---------|-------------|
| `ToolTimeout` | 30s | Per-tool context deadline |
| `MaxConcurrentTools` | 10 | Per-session semaphore limit |
| `RBACRoles` | nil (open) | Map of group → allowed tools; nil disables app-layer RBAC |

## Diagnosis Steps

### 1. RBAC Denials

1. Check `af_mcp_rbac_denied_total` by tool — which tool is being denied?
2. Verify `rbac_roles.yaml` maps the user's OIDC groups correctly
3. Check audit trail (DataStorage) for `EventMCPToolDenied` events with user/tool
4. If `RBACRoles` is nil, all calls are allowed at app layer; check K8s RBAC for CRD ops

### 2. Tool Timeouts

1. Query `af_tool_calls_total{result="timeout"}` — which tools are timing out?
2. Check downstream health:
   - K8s API server: `kubectl get --raw /readyz`
   - KA: `curl $KA_URL/healthz`
   - DataStorage: check DS liveness probe
3. If isolated to CRD tools, check K8s API server latency and etcd health
4. If isolated to KA tools, check KA circuit breaker state

### 3. Panics

1. Look for `result="panic"` in `af_tool_calls_total`
2. Check audit trail for `EventMCPToolFailed` with `error: "internal error"`
3. Check pod logs for panic stack traces (bridge logs with `"tool handler panicked"`)
4. Likely causes: nil pointer in handler, invalid type assertion, concurrent map write

### 4. Throttling

1. If `af_tool_calls_total{result="throttled"}` is elevated:
   - Check if many concurrent sessions are active
   - Increase `MaxConcurrentTools` if downstream can handle more load
   - Investigate slow handlers causing semaphore starvation

### 5. Session Issues

- MCP sessions require `Mcp-Session-Id` header after initialization
- Missing header → 400 Bad Request (not a bridge error)
- Expired session → re-initialize required

## Escalation

If the bridge itself is healthy but downstream fails:
- K8s issues → escalate to platform team
- KA issues → check `../kubernaut` service health
- DataStorage issues → check DS deployment and network policies
