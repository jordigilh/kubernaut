# DD-AF-005: Alert Prioritization Algorithm

**Status**: Accepted
**Date**: 2026-06-12
**Issue**: [#1412](https://github.com/jordigilh/kubernaut/issues/1412)
**Related**: ADR-066 (4-level severity model), BR-ALERT-001

## Context

When multiple Prometheus alerts fire simultaneously in a namespace, the `list_alerts` tool must deterministically identify the highest-priority alert for investigation. Without prioritization, the LLM or autonomous flow would select alerts non-deterministically, potentially wasting remediation time on low-severity events while critical alerts go unaddressed.

FedRAMP control SI-4(5) requires automated mechanisms to alert on security-relevant events with appropriate severity classification.

## Decision

### Ranking Algorithm (`PrioritizeAlerts`)

Alerts are ranked using a two-key stable sort:

1. **Primary key**: Severity (descending) — higher numeric rank wins
2. **Secondary key**: ActiveAt timestamp (ascending, FIFO) — longer-firing alerts take priority among same-severity peers

```
slices.SortStableFunc(sorted, func(a, b AlertSummary) int {
    sevCmp := severity.CompareSeverity(b.Labels["severity"], a.Labels["severity"])
    if sevCmp != 0 {
        return sevCmp
    }
    return cmp.Compare(a.ActiveAt.UnixNano(), b.ActiveAt.UnixNano())
})
```

### Severity Rank Table

| Level | Rank | Origin |
|-------|------|--------|
| critical | 5 | Prometheus, PagerDuty |
| high | 4 | PagerDuty, ServiceNow P2 |
| warning | 3 | Prometheus standard |
| medium | 3 | Legacy (forward-compat, same rank as warning) |
| low | 2 | Legacy (deprecated, see ADR-066 Phase 5) |
| info | 1 | Prometheus, all systems |
| (unknown/empty) | 0 | Implicit Go map zero-value |

Unknown or empty severity labels rank 0, ensuring they always sort below info.

### Response Contract

The `list_alerts` tool returns a `ListAlertsResult` with an optional `prioritized` field:

```json
{
  "alerts": [...],
  "count": 5,
  "truncated": false,
  "prioritized": {
    "selected": { "labels": {"severity": "critical", ...}, "state": "firing", "active_at": "..." },
    "tied": [],
    "also_active": [...]
  }
}
```

| Field | Description |
|-------|-------------|
| `selected` | Highest-severity, longest-firing alert. Investigate this one. |
| `tied` | Other alerts at the same severity as selected (same rank, different alert names). |
| `also_active` | Lower-severity alerts provided for context only. |

`prioritized` is `null`/omitted when no alerts match the query (empty result set).

### Pipeline Ordering

```
GetAlerts → Filter (namespace/severity/state) → TrimSliceToFit → PrioritizeAlerts
```

Truncation occurs BEFORE prioritization. The `truncated` flag indicates alerts were dropped for size limits; the ranking operates only on the retained subset.

### MCP Bridge Registration

The tool is registered as `kubernaut_list_alerts` in the MCP bridge, conditionally:

```go
if cfg.PromClient != nil {
    registerTool(srv, cfg, sem, "kubernaut_list_alerts", ...)
}
```

When `PromClient` is nil (Prometheus not configured), the tool is not available via MCP.

In the A2A agent (ADK path), the tool is registered as `list_alerts` (no prefix).

### RBAC

MCP access requires SAR authorization on `kubernaut.ai/tools` resource `kubernaut_list_alerts` with verb `use`.

## Alternatives Considered

### A: Random selection among highest-severity alerts

Rejected: Non-deterministic behavior makes testing unreliable and E2E assertions impossible.

### B: Let LLM choose from a presented list

Rejected: Adds a round-trip to the user/LLM; in autonomous mode there is no user to ask. The system should deterministically select the most urgent alert.

### C: Most-recent-first (newest alert wins)

Rejected: Oldest-first (FIFO) aligns with incident response best practice — alerts that have been firing longest represent sustained impact and should be investigated first.

## Consequences

### Positive

- Deterministic: same alerts always produce same selection (testable, auditable)
- FIFO tie-breaking aligns with incident response urgency
- Response contract enables Console to highlight the selected alert
- MCP conditional registration prevents errors when Prometheus is not configured

### Negative

- Truncation before ranking means very large alert sets may lose high-severity alerts to size limits (mitigated: truncation cap is generous at 100 alerts)
- `medium` and `warning` sharing rank 3 means they are interchangeable until ADR-066 Phase 5 cleanup

## Test Coverage

| Tier | IDs | Validates |
|------|-----|-----------|
| UT | UT-AF-1412-010..060 | Ranking logic, tie-breaking, edge cases |
| IT | IT-AF-1412-001, 002 | MCP wiring, RBAC |
| E2E | E2E-AF-1412-001, 002 | End-to-end via MCP tools/call |

## FedRAMP Controls

| Control | Application |
|---------|-------------|
| SI-4(5) | Automated severity-based alert prioritization |
| IR-4(1) | Deterministic selection enables automated incident response |
| AU-3 | Alert selection logged in MCP audit trail |
