# BR-KA-195: LLM Cost Tracking Metrics

**Business Requirement ID**: BR-KA-195
**Category**: KA (Kubernaut Agent)
**Priority**: P2
**Target Version**: V2.0 (V1 slice tracked separately — see Status below)
**Status**: Pending (V2.0 enhancements); V1 cost-counter slice approved under [BR-KA-OBSERVABILITY-001](BR-KA-OBSERVABILITY-001-agent-prometheus-metrics.md)
**Date**: 2025-12-03

**Last Updated**: 2026-08-01 — Renamed from `BR-HAPI-195`, terminology corrected to Kubernaut
Agent (KA, Go), and de-duplicated against [BR-KA-OBSERVABILITY-001](BR-KA-OBSERVABILITY-001-agent-prometheus-metrics.md)
([Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806)). Neither this BR's original
proposal nor `BR-KA-OBSERVABILITY-001.3`'s cost counter is implemented in code as of this update
(confirmed by code search — no `llm_cost_dollars`/`CostDollars`/pricing-map symbol exists in
`internal/kubernautagent/metrics/`). This document now covers only the V2.0 enhancements beyond
that approved V1 slice, to avoid two BRs specifying the same metric.

---

## Business Need

### Problem Statement

Kubernaut Agent (KA) tracks LLM token usage internally (`internal/kubernautagent/investigator/token_accumulator.go`,
audit-only per #435 — not currently exposed as a Prometheus metric) but does not provide direct
cost tracking. Operations teams must manually calculate costs by:
1. Exporting token counts from audit records
2. Looking up per-provider pricing
3. Multiplying tokens by cost-per-token

**Current Limitations**:
- ❌ No real-time cost visibility in Grafana dashboards
- ❌ No cost-based alerting (e.g., "daily cost exceeded $X")
- ❌ Manual cost calculation required for budgeting
- ❌ Cannot compare cost efficiency across LLM providers

**Impact**:
- Operations teams cannot monitor AI costs in real-time
- Budget overruns may go undetected until invoice arrives
- Provider cost comparison requires external tooling

---

## Relationship to BR-KA-OBSERVABILITY-001

[BR-KA-OBSERVABILITY-001.3](BR-KA-OBSERVABILITY-001-agent-prometheus-metrics.md#br-ka-observability-0013-llm-cost-tracking)
(Status: Approved, Target: V1.5) specifies the **V1 slice** of this business need: a single
Prometheus counter, `aiagent_llm_cost_dollars_total{model}`, recorded once per LLM call inside
`chatOrStream()`, using a **hardcoded** pricing map with unknown-model-defaults-to-$0 fallback.

This document (`BR-KA-195`) now covers only what BR-KA-OBSERVABILITY-001.3 explicitly **defers**:

| Capability | BR-KA-OBSERVABILITY-001.3 (V1, approved) | BR-KA-195 (this doc, V2.0) |
|------------|-------------------------------------------|----------------------------|
| Cost counter metric | ✅ `aiagent_llm_cost_dollars_total{model}` | — (reuses V1 metric) |
| Pricing source | Hardcoded Go map | ConfigMap-driven, no code changes to update pricing |
| Labels | `model` only | `provider` + `model` (+ possibly `endpoint`/phase) for finer-grained comparison |
| Dashboards | Not specified | Grafana "Daily Cost" panel |
| Alerting | Not specified | AlertManager cost-anomaly rules |

**Do not re-implement the base counter under this BR** — implement `aiagent_llm_cost_dollars_total`
per BR-KA-OBSERVABILITY-001.3's acceptance criteria first; this document's functional
requirements below apply only once that V1 slice exists.

---

## Business Objective

Extend the V1 cost counter (BR-KA-OBSERVABILITY-001.3) with ConfigMap-driven pricing and
provider-level breakdown to enable real-time cost monitoring, alerting, and provider comparison
without code changes for pricing updates.

### Success Criteria
1. ✅ Cost metric exposed at `/metrics` endpoint (satisfied by BR-KA-OBSERVABILITY-001.3)
2. ⏳ Pricing table configurable via ConfigMap (no code deploy required to update rates)
3. ⏳ Cost breakdown by provider and model (not just model)
4. ⏳ Grafana dashboard shows real-time cost trends
5. ⏳ Alert rules trigger on cost anomalies

---

## Use Cases

### Use Case 1: Real-Time Cost Dashboard

**Scenario**: Operations team wants to monitor daily AI spending.

**Current Flow** (as of this update): `aiagent_llm_cost_dollars_total` does not yet exist in code;
operators would need to derive cost manually from audit-logged token counts.

**Desired Flow with BR-KA-OBSERVABILITY-001.3 + BR-KA-195**:
```
1. Open Grafana dashboard
2. View "Daily Cost" panel
3. ✅ Real-time cost visibility, broken down by provider
```

### Use Case 2: Cost Spike Alerting

**Scenario**: Unusual investigation volume causes a cost spike.

**Desired Flow**:
```
1. Cost exceeds hourly budget
2. ✅ Alert fires: "KA hourly cost > $50"
3. Immediate investigation
```

### Use Case 3: Provider Cost Comparison

**Scenario**: Evaluate switching LLM providers (KA supports VertexAI, Anthropic, OpenAI, Ollama,
and others — see `pkg/kubernautagent/llm/`).

**Desired Flow**:
```
1. Run A/B test with both providers
2. Query: avg cost by provider (requires `provider` label — V2.0 gap in BR-KA-OBSERVABILITY-001.3's `model`-only label)
3. ✅ Direct comparison in Prometheus
```

---

## Functional Requirements

### FR-KA-195-01: Provider Label on Cost Metric

**Requirement**: The `aiagent_llm_cost_dollars_total` counter (introduced by
BR-KA-OBSERVABILITY-001.3) SHALL add a `provider` label alongside the existing `model` label.

**Acceptance Criteria**:
- ✅ Existing `model`-only cardinality is preserved as a label subset (no breaking dashboard queries)
- ✅ `provider` sourced from the same LLM client abstraction used by `chatOrStream()` (`pkg/kubernautagent/llm/`)

### FR-KA-195-02: ConfigMap-Driven Pricing

**Requirement**: Cost pricing SHALL be loadable from a ConfigMap, replacing
BR-KA-OBSERVABILITY-001.3's hardcoded pricing map, following KA's existing hot-reload pattern
(`internal/kubernautagent/config` file-watcher) rather than a bespoke mechanism.

**Acceptance Criteria**:
- ✅ Pricing configurable via ConfigMap without a code deploy
- ✅ Unknown provider/model defaults to $0 (safe fallback — unchanged from V1)
- ✅ Pricing reload does not require a KA pod restart (consistent with other hot-reloadable KA config)

### FR-KA-195-03: Cost Dashboards and Alerts

**Requirement**: Grafana dashboard panels and AlertManager rules SHALL be added once the metric
and labels above exist.

**Acceptance Criteria**:
- ✅ "Daily Cost" and "Cost by Provider" Grafana panels
- ✅ AlertManager rule for hourly-cost-exceeds-threshold

---

## Non-Functional Requirements

### NFR-KA-195-01: Performance
- Cost calculation adds <1ms latency per LLM call (unchanged expectation from the original proposal)
- No additional external API calls required

### NFR-KA-195-02: Accuracy
- Cost matches provider invoice within 5% margin
- Token counts sourced from the LLM response itself (not estimated) — consistent with existing
  `TokenAccumulator` behavior

### NFR-KA-195-03: Configurability
- Pricing table configurable via ConfigMap
- No code changes required to update pricing

---

## Dependencies

### Upstream Dependencies
- **[BR-KA-OBSERVABILITY-001.3](BR-KA-OBSERVABILITY-001-agent-prometheus-metrics.md#br-ka-observability-0013-llm-cost-tracking)**: base `aiagent_llm_cost_dollars_total{model}` counter — **must ship first**

### Downstream Dependencies
- Grafana dashboards (new cost panels)
- AlertManager rules (cost-based alerts)

---

## Metrics & Observability (Target State)

### Prometheus Queries (once FR-KA-195-01 ships)

```promql
# Cost per hour
rate(aiagent_llm_cost_dollars_total[1h]) * 3600

# Daily cost
sum(increase(aiagent_llm_cost_dollars_total[24h]))

# Cost by provider (requires provider label, FR-KA-195-01)
sum by (provider) (rate(aiagent_llm_cost_dollars_total[1h]) * 3600)
```

### Alert Rule (target state)

```yaml
- alert: KAHighCost
  expr: |
    rate(aiagent_llm_cost_dollars_total[1h]) * 3600 > 50
  for: 15m
  labels:
    severity: warning
  annotations:
    summary: "Kubernaut Agent hourly LLM cost exceeds $50"
```

---

## Testing Requirements

Per [AGENTS.md](../../AGENTS.md), KA business logic tests use Ginkgo/Gomega, not Go's standard
`testing.T` — the original document's `test_calculate_cost_*` function-name style (implying plain
`testing.T` tests) does not apply. Once implemented, coverage should include:

- **Unit** (Ginkgo): pricing lookup for known providers/models, `$0` fallback for unknown
  provider/model, ConfigMap reload updates pricing without restart
- **Integration**: cost metric exposed at `/metrics`, increments on successful LLM calls only
  (no charge recorded for failed calls, consistent with BR-KA-OBSERVABILITY-001.3 AC-001-3.1)

---

## References

- [BR-KA-OBSERVABILITY-001](BR-KA-OBSERVABILITY-001-agent-prometheus-metrics.md) — base cost counter (V1 slice) and full KA metrics catalog
- `internal/kubernautagent/investigator/token_accumulator.go` — existing (audit-only) token tracking
- `pkg/kubernautagent/llm/` — LLM client abstraction (provider/model source for the `provider` label)

---

**Document Status**: Pending (V2.0 scope; depends on BR-KA-OBSERVABILITY-001.3 shipping first)
**Author**: Kubernaut Development Team
**Review Required**: Yes (architecture review for pricing configuration)
