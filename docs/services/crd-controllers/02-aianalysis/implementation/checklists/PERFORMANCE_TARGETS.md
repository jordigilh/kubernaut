# AI Analysis Service - Performance Targets

**Parent Document**: [IMPLEMENTATION_PLAN_V1.0.md](../IMPLEMENTATION_PLAN_V1.0.md)
**Version**: 1.0

---

## 🎯 **Performance Objectives**

### **Latency Targets (SLOs)**

| Metric | P50 | P95 | P99 | SLO |
|--------|-----|-----|-----|-----|
| Reconciliation Duration | <1s | <5s | <10s | 99% <10s |
| Phase Transition | <100ms | <500ms | <1s | 99% <1s |
| HolmesGPT-API Call | <5s | <30s | <60s | 95% <30s |
| Rego Policy Evaluation | <10ms | <50ms | <100ms | 99% <100ms |
| Status Update | <100ms | <300ms | <500ms | 99% <500ms |

### **Throughput Targets**

| Metric | Target | Notes |
|--------|--------|-------|
| Concurrent AIAnalysis | 50 | Per controller instance |
| Reconciliations/min | 100 | Steady state |
| HolmesGPT calls/min | 20 | Rate limited |
| Rego evaluations/min | 100 | Cached policy |

### **Resource Targets**

| Resource | Request | Limit | Notes |
|----------|---------|-------|-------|
| CPU | 100m | 500m | Per controller pod |
| Memory | 128Mi | 512Mi | Per controller pod |
| Max Goroutines | 100 | — | Concurrent workers |

---

## 📊 **Performance Metrics**

### **Key Metrics to Monitor**

```bash
# Reconciliation latency
aianalysis_reconciliation_duration_seconds_bucket
aianalysis_reconciliation_duration_seconds_count
aianalysis_reconciliation_duration_seconds_sum

# Phase duration
aianalysis_phase_duration_seconds_bucket{phase="investigating"}
aianalysis_phase_duration_seconds_bucket{phase="analyzing"}

# HolmesGPT-API latency
aianalysis_holmesgpt_latency_seconds_bucket
aianalysis_holmesgpt_latency_seconds_count

# Rego policy latency
aianalysis_rego_policy_latency_seconds_bucket
```

### **Prometheus Queries**

```promql
# P95 reconciliation latency
histogram_quantile(0.95,
  sum(rate(aianalysis_reconciliation_duration_seconds_bucket[5m])) by (le)
)

# P95 HolmesGPT latency
histogram_quantile(0.95,
  sum(rate(aianalysis_holmesgpt_latency_seconds_bucket[5m])) by (le)
)

# Reconciliation rate
sum(rate(aianalysis_reconciliation_total[5m]))

# Error rate
sum(rate(aianalysis_reconciliation_total{result="error"}[5m])) /
sum(rate(aianalysis_reconciliation_total[5m]))
```

---

## 🧪 **Performance Testing**

### **Benchmark Tests**

**File**: `test/benchmark/aianalysis/benchmark_test.go`

```go
package benchmark

import (
	"context"
	"testing"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

func BenchmarkRegoEvaluation(b *testing.B) {
	engine := rego.NewEngine(nil)
	engine.LoadPolicy(testPolicy, "1.0.0")

	input := &rego.PolicyInput{
		Environment:      "production",
		BusinessPriority: "P0",
		Confidence:       0.95,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.Evaluate(context.Background(), input)
	}
}

func BenchmarkValidation(b *testing.B) {
	handler := phases.NewValidatingHandler(nil, nil)
	analysis := newTestAIAnalysis("bench")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = handler.validateRequiredFields(analysis)
	}
}
```

### **Load Testing**

```bash
# Generate load with k6
k6 run --vus 10 --duration 30s test/load/aianalysis-load.js

# Or with custom script
for i in $(seq 1 50); do
  kubectl apply -f test/fixtures/aianalysis-${i}.yaml &
done
wait
```

---

## 🔍 **Performance Optimization**

### **Optimization Checklist**

#### **Reconciliation**
- [ ] Use client-go caching
- [ ] Minimize API calls per reconciliation
- [ ] Use status subresource for updates
- [ ] Implement exponential backoff

#### **Rego Policy**
- [ ] Cache prepared query
- [ ] Use efficient data structures in policy
- [ ] Minimize policy complexity
- [ ] Profile policy evaluation

#### **HolmesGPT-API**
- [ ] Connection pooling
- [ ] Request timeout configuration
- [ ] Circuit breaker for failures
- [ ] Response caching (where appropriate)

#### **Memory**
- [ ] Avoid large allocations in hot paths
- [ ] Use sync.Pool for frequent allocations
- [ ] Profile memory usage
- [ ] Set appropriate resource limits

---

## 📈 **Baseline Measurements**

### **Pre-Implementation Baseline**

| Metric | Baseline | Target | Actual |
|--------|----------|--------|--------|
| Reconciliation P95 | — | <5s | — |
| HolmesGPT P95 | — | <30s | — |
| Rego P95 | — | <50ms | — |
| Memory (idle) | — | <100Mi | — |
| Memory (peak) | — | <400Mi | — |

### **Post-Implementation Measurement**

Run after Day 8 completion:

```bash
# Start controller with profiling
go run ./cmd/aianalysis/... -v=2 2>&1 | tee controller.log &

# Generate load
kubectl apply -f test/fixtures/aianalysis-load/

# Collect metrics
curl localhost:9090/metrics > metrics-$(date +%s).txt

# Profile memory
curl localhost:9090/debug/pprof/heap > heap-$(date +%s).prof

# Profile CPU
curl localhost:9090/debug/pprof/profile?seconds=30 > cpu-$(date +%s).prof
```

---

## 🚨 **Performance Alerts**

### **Recommended Alerts**

```yaml
# High reconciliation latency
- alert: AIAnalysisHighLatency
  expr: histogram_quantile(0.95, sum(rate(aianalysis_reconciliation_duration_seconds_bucket[5m])) by (le)) > 10
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: AIAnalysis reconciliation latency is high

# High HolmesGPT latency
- alert: AIAnalysisHolmesGPTSlow
  expr: histogram_quantile(0.95, sum(rate(aianalysis_holmesgpt_latency_seconds_bucket[5m])) by (le)) > 30
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: HolmesGPT-API calls are slow

# High error rate
- alert: AIAnalysisHighErrorRate
  expr: sum(rate(aianalysis_reconciliation_total{result="error"}[5m])) / sum(rate(aianalysis_reconciliation_total[5m])) > 0.05
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: AIAnalysis error rate is above 5%
```

---

## 📚 **References**

| Document | Purpose |
|----------|---------|
| [metrics-slos.md](../../metrics-slos.md) | SLO definitions |
| [DD-005: Observability](../../../../architecture/decisions/DD-005-observability-standards.md) | Metrics standards |

