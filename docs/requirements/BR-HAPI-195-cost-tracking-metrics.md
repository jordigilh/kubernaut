# BR-HAPI-195: LLM Cost Tracking Metrics

**Business Requirement ID**: BR-HAPI-195
**Category**: HolmesGPT-API
**Priority**: P2
**Target Version**: V2.0
**Status**: Pending
**Date**: 2025-12-03

---

## 📋 **Business Need**

### **Problem Statement**

HolmesGPT-API currently tracks LLM token usage (`holmesgpt_llm_token_usage_total`) but does not provide direct cost tracking. Operations teams must manually calculate costs by:
1. Exporting token metrics
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

## 🎯 **Business Objective**

Implement direct cost tracking metric (`holmesgpt_investigation_cost_dollars_total`) to enable real-time cost monitoring, alerting, and provider comparison.

### **Success Criteria**
1. ✅ Cost metric exposed at `/metrics` endpoint
2. ✅ Cost calculated per investigation request
3. ✅ Cost breakdown by provider and model
4. ✅ Grafana dashboard shows real-time cost trends
5. ✅ Alert rules trigger on cost anomalies

---

## 📊 **Use Cases**

### **Use Case 1: Real-Time Cost Dashboard**

**Scenario**: Operations team wants to monitor daily AI spending

**Current Flow**:
```
1. Export token metrics from Prometheus
2. Download LLM provider pricing CSV
3. Join data in spreadsheet
4. ❌ Manual calculation, delayed visibility
```

**Desired Flow with BR-HAPI-195**:
```
1. Open Grafana dashboard
2. View "Daily Cost" panel
3. ✅ Real-time cost visibility
```

### **Use Case 2: Cost Spike Alerting**

**Scenario**: Unusual investigation volume causes cost spike

**Current Flow**:
```
1. Token spike detected
2. ❌ No cost alert (only volume alert)
3. Cost overrun discovered at month-end
```

**Desired Flow with BR-HAPI-195**:
```
1. Cost exceeds hourly budget
2. ✅ Alert fires: "HolmesGPT hourly cost > $50"
3. Immediate investigation
```

### **Use Case 3: Provider Cost Comparison**

**Scenario**: Evaluate switching from OpenAI to Claude

**Current Flow**:
```
1. Run A/B test with both providers
2. Export token metrics
3. ❌ Manually calculate cost per investigation
4. Compare in spreadsheet
```

**Desired Flow with BR-HAPI-195**:
```
1. Run A/B test with both providers
2. Query: avg cost by provider
3. ✅ Direct comparison in Prometheus
```

---

## 🔧 **Functional Requirements**

### **FR-HAPI-195-01: Cost Counter Metric**

**Requirement**: HolmesGPT-API SHALL expose a Prometheus Counter metric tracking cumulative cost in US dollars.

**Metric Definition**:
```python
from prometheus_client import Counter

investigation_cost_dollars = Counter(
    'holmesgpt_investigation_cost_dollars_total',
    'Total cost of LLM API calls in US dollars',
    ['provider', 'model', 'endpoint']
)
```

**Labels**:
- `provider`: LLM provider (e.g., "openai", "anthropic", "ollama")
- `model`: Model name (e.g., "gpt-4", "claude-3-sonnet")
- `endpoint`: API endpoint (e.g., "/api/v1/incident/analyze")

**Acceptance Criteria**:
- ✅ Metric registered at service startup
- ✅ Metric visible at `/metrics` endpoint
- ✅ Metric increments after each LLM call

---

### **FR-HAPI-195-02: Cost Calculation Logic**

**Requirement**: Cost SHALL be calculated based on provider-specific pricing per 1K tokens.

**Implementation**:
```python
# Provider pricing configuration (per 1K tokens)
LLM_PRICING = {
    "openai": {
        "gpt-4": {"prompt": 0.03, "completion": 0.06},
        "gpt-4-turbo": {"prompt": 0.01, "completion": 0.03},
        "gpt-3.5-turbo": {"prompt": 0.0005, "completion": 0.0015},
    },
    "anthropic": {
        "claude-3-opus": {"prompt": 0.015, "completion": 0.075},
        "claude-3-sonnet": {"prompt": 0.003, "completion": 0.015},
        "claude-3-haiku": {"prompt": 0.00025, "completion": 0.00125},
    },
    "ollama": {
        # Local models - $0 cost
        "*": {"prompt": 0.0, "completion": 0.0},
    }
}

def calculate_cost(provider: str, model: str, prompt_tokens: int, completion_tokens: int) -> float:
    pricing = LLM_PRICING.get(provider, {}).get(model, {"prompt": 0, "completion": 0})
    cost = (prompt_tokens / 1000 * pricing["prompt"]) + (completion_tokens / 1000 * pricing["completion"])
    return cost
```

**Acceptance Criteria**:
- ✅ Pricing configurable via ConfigMap
- ✅ Unknown provider/model defaults to $0 (safe fallback)
- ✅ Cost calculated to 6 decimal places (micro-dollar precision)

---

### **FR-HAPI-195-03: Integration with LLM Call Recording**

**Requirement**: Cost SHALL be recorded alongside existing `record_llm_call()` function.

**Implementation**:
```python
def record_llm_call(
    provider: str,
    model: str,
    status: str,
    duration: float,
    prompt_tokens: int = 0,
    completion_tokens: int = 0,
    endpoint: str = ""
):
    # Existing metrics
    llm_calls_total.labels(provider=provider, model=model, status=status).inc()
    llm_call_duration_seconds.labels(provider=provider, model=model).observe(duration)

    # Token metrics
    if prompt_tokens > 0:
        llm_token_usage.labels(provider=provider, model=model, type="prompt").inc(prompt_tokens)
    if completion_tokens > 0:
        llm_token_usage.labels(provider=provider, model=model, type="completion").inc(completion_tokens)

    # NEW: Cost metric
    if status == "success":
        cost = calculate_cost(provider, model, prompt_tokens, completion_tokens)
        investigation_cost_dollars.labels(
            provider=provider,
            model=model,
            endpoint=endpoint
        ).inc(cost)
```

**Acceptance Criteria**:
- ✅ Cost recorded only for successful calls
- ✅ Failed calls do not increment cost (no charge)
- ✅ Endpoint label enables per-endpoint cost analysis

---

## 📈 **Non-Functional Requirements**

### **NFR-HAPI-195-01: Performance**

- Cost calculation adds <1ms latency per request
- No additional external API calls required

### **NFR-HAPI-195-02: Accuracy**

- Cost matches provider invoice within 5% margin
- Token counts from LLM response used (not estimated)

### **NFR-HAPI-195-03: Configurability**

- Pricing table configurable via ConfigMap
- No code changes required to update pricing

---

## 🔗 **Dependencies**

### **Upstream Dependencies**
- None (uses existing token metrics infrastructure)

### **Downstream Dependencies**
- Grafana dashboards (new cost panels)
- AlertManager rules (cost-based alerts)

---

## 📊 **Metrics & Observability**

### **New Prometheus Queries**

```promql
# Cost per hour
rate(holmesgpt_investigation_cost_dollars_total[1h]) * 3600

# Daily cost
sum(increase(holmesgpt_investigation_cost_dollars_total[24h]))

# Cost by provider
sum by (provider) (rate(holmesgpt_investigation_cost_dollars_total[1h]) * 3600)

# Average cost per investigation
rate(holmesgpt_investigation_cost_dollars_total[5m])
/
rate(holmesgpt_investigations_total{status=~"2.."}[5m])
```

### **Alert Rules**

```yaml
- alert: HolmesGPTHighCost
  expr: |
    rate(holmesgpt_investigation_cost_dollars_total[1h]) * 3600 > 50
  for: 15m
  labels:
    severity: warning
  annotations:
    summary: "HolmesGPT hourly cost exceeds $50"
```

---

## 🧪 **Testing Requirements**

### **Unit Tests**
- `test_calculate_cost_openai_gpt4` - Verify GPT-4 pricing
- `test_calculate_cost_claude_sonnet` - Verify Claude pricing
- `test_calculate_cost_ollama_local` - Verify $0 for local models
- `test_calculate_cost_unknown_provider` - Verify fallback to $0
- `test_record_llm_call_increments_cost` - Integration with record_llm_call

### **Integration Tests**
- `test_cost_metric_exposed` - Verify metric at /metrics
- `test_cost_increments_on_successful_call` - E2E verification

---

## 📚 **References**

- [metrics-slos.md](../services/stateless/holmesgpt-api/metrics-slos.md) - Metrics specification
- [src/middleware/metrics.py](../../holmesgpt-api/src/middleware/metrics.py) - Current implementation
- [DD-HOLMESGPT-013](../architecture/decisions/DD-HOLMESGPT-013-Observability-Strategy.md) - Observability strategy

---

**Document Status**: ✅ Complete
**Author**: Kubernaut Development Team
**Review Required**: Yes (architecture review for pricing configuration)


