# Metrics Testing with Dynamic Ports: Registry Inspection Pattern

**Date**: December 25, 2025
**Status**: ✅ **COMPLETE** - Pattern documented and implemented
**Key Insight**: HTTP endpoints NOT needed for integration-tier metrics testing

---

## 🎯 **The Problem We Solved**

### **Initial Misconception** (❌ WRONG)
> "Dynamic port allocation (`:0`) prevents metrics testing in integration tests because we can't discover the HTTP endpoint."

### **Correct Understanding** (✅ RIGHT)
> "Dynamic port allocation does NOT prevent metrics testing. Integration tests use **registry inspection**, not HTTP scraping."

---

## ✅ **The Solution: Registry Inspection Pattern**

### **Key API**: `ctrlmetrics.Registry.Gather()`

**Direct in-process access** to Prometheus registry - no HTTP needed!

```go
import (
    dto "github.com/prometheus/client_model/go"
    ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Gather all metrics from controller-runtime's global registry
gatherMetrics := func() (map[string]*dto.MetricFamily, error) {
    families, err := ctrlmetrics.Registry.Gather()  // ← Magic happens here!
    if err != nil {
        return nil, err
    }
    result := make(map[string]*dto.MetricFamily)
    for _, family := range families {
        result[family.GetName()] = family
    }
    return result, nil
}

// Check if metric exists
metricExists := func(name string) bool {
    families, err := gatherMetrics()
    if err != nil {
        return false
    }
    _, exists := families[name]
    return exists
}

// Get counter value with labels
getCounterValue := func(name string, labels map[string]string) float64 {
    families, err := gatherMetrics()
    if err != nil {
        return -1
    }
    family, exists := families[name]
    if !exists {
        return -1
    }
    for _, m := range family.GetMetric() {
        if matchLabels(m.GetLabel(), labels) {
            return m.GetCounter().GetValue()
        }
    }
    return -1
}
```

---

## 📚 **Services Using This Pattern**

### **1. AIAnalysis** (Original Pattern)
- **File**: `test/integration/aianalysis/metrics_integration_test.go`
- **Status**: ✅ Original implementation
- **Tests**: 8 business-value metrics
- **Pattern**: Direct registry access via `ctrlmetrics.Registry.Gather()`

### **2. RemediationOrchestrator** (Newly Updated)
- **File**: `test/integration/remediationorchestrator/operational_metrics_integration_test.go`
- **Status**: ✅ Rewritten to use registry inspection
- **Tests**: 3 operational metrics (reconcile_total, reconcile_duration, phase_transitions_total)
- **Pattern**: Follows AIAnalysis approach

### **3. SignalProcessing** (Already Correct)
- **File**: `test/integration/signalprocessing/metrics_integration_test.go`
- **Status**: ✅ Uses registry inspection with custom test registry
- **Tests**: Multiple metrics with controller integration
- **Pattern**: Test registry + gathering

---

## 🔍 **Why This Works**

### **Prometheus Registry Architecture**

```
┌─────────────────────────────────────────┐
│ controller-runtime Manager              │
│                                         │
│  ┌─────────────────┐                   │
│  │ Metrics Server  │──HTTP→ :9090 (or :0)
│  │  (HTTP Handler) │                   │
│  └─────────────────┘                   │
│         ↓                               │
│  ┌─────────────────┐                   │
│  │ Registry        │←───In-Process     │
│  │ (Global State)  │    Access! ✅     │
│  └─────────────────┘                   │
│         ↑                               │
│    Metrics.Inc()                        │
│                                         │
└─────────────────────────────────────────┘
```

**Key Insight**: The Prometheus registry is **global in-process state**. We can access it directly without HTTP!

---

## 🆚 **Comparison: HTTP vs Registry Inspection**

| Aspect | HTTP Scraping | Registry Inspection |
|--------|--------------|-------------------|
| **Port Discovery** | ❌ Requires known port | ✅ Not needed |
| **Dynamic Ports** | ❌ Blocked by `:0` | ✅ Works with `:0` |
| **Test Complexity** | ⚠️ HTTP client, parsing | ✅ Native Go types |
| **Performance** | ⚠️ HTTP overhead | ✅ Direct memory access |
| **Parallel Tests** | ❌ Port conflicts | ✅ No conflicts |
| **Type Safety** | ❌ Parse strings | ✅ Typed protobuf DTOs |

**Winner**: Registry Inspection for integration tests! 🏆

---

## 📋 **Implementation Checklist**

### **For New Services**:
- [ ] Import `dto "github.com/prometheus/client_model/go"`
- [ ] Import `ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"`
- [ ] Create `gatherMetrics()` helper
- [ ] Create `metricExists()` helper
- [ ] Create type-specific helpers (`getCounterValue`, `histogramHasSamples`, etc.)
- [ ] Write tests using registry inspection
- [ ] Use `BindAddress: ":0"` for dynamic port allocation

### **For Existing Services**:
- [ ] Review current metrics tests
- [ ] If using HTTP scraping with hardcoded ports → convert to registry inspection
- [ ] Follow AIAnalysis or RemediationOrchestrator pattern
- [ ] Update `BindAddress` to `":0"` if not already

---

## 🧪 **Testing Strategy by Tier**

| Tier | What to Test | How | Why |
|------|-------------|-----|-----|
| **Unit** | Metrics increment logic | Mock registries, test metrics objects | Isolated logic testing |
| **Integration** | Metrics registration & recording | `ctrlmetrics.Registry.Gather()` | Real registry, real controller |
| **E2E** | HTTP /metrics endpoint | `curl http://localhost:30090/metrics` | External observability |

**All tiers can test metrics!** No tier is blocked by dynamic port allocation. ✅

---

## 💡 **Key Takeaways**

### **1. Dynamic Ports Do NOT Block Metrics Testing**
- Registry inspection works regardless of HTTP port
- `:0` is safe for integration tests

### **2. Registry Inspection is BETTER for Integration Tests**
- No HTTP overhead
- Type-safe access
- No parsing needed
- No port discovery needed

### **3. HTTP Testing Belongs in E2E Tier**
- E2E tests use Kind clusters with known NodePorts
- External observability validation (Prometheus scraping)
- End-user perspective

### **4. Follow Established Patterns**
- AIAnalysis: Original pattern (comprehensive)
- RemediationOrchestrator: Simplified pattern (3 metrics)
- SignalProcessing: Custom registry pattern

---

## 📖 **Example: Complete Test**

```go
var _ = Describe("Metrics Integration", func() {
    var testNamespace string

    BeforeEach(func() {
        testNamespace = createTestNamespace("metrics")
    })

    AfterEach(func() {
        deleteTestNamespace(testNamespace)
    })

    // Helper: Gather metrics from registry
    gatherMetrics := func() (map[string]*dto.MetricFamily, error) {
        families, err := ctrlmetrics.Registry.Gather()
        if err != nil {
            return nil, err
        }
        result := make(map[string]*dto.MetricFamily)
        for _, family := range families {
            result[family.GetName()] = family
        }
        return result, nil
    }

    // Helper: Check metric exists
    metricExists := func(name string) bool {
        families, err := gatherMetrics()
        if err != nil {
            return false
        }
        _, exists := families[name]
        return exists
    }

    // Helper: Get counter value
    getCounterValue := func(name string, labels map[string]string) float64 {
        families, err := gatherMetrics()
        if err != nil {
            return -1
        }
        family, exists := families[name]
        if !exists {
            return -1
        }
        for _, m := range family.GetMetric() {
            labelMatch := true
            for wantKey, wantValue := range labels {
                found := false
                for _, l := range m.GetLabel() {
                    if l.GetName() == wantKey && l.GetValue() == wantValue {
                        found = true
                        break
                    }
                }
                if !found {
                    labelMatch = false
                    break
                }
            }
            if labelMatch {
                return m.GetCounter().GetValue()
            }
        }
        return -1
    }

    It("should register and increment metrics", func() {
        // Verify metric is registered
        Expect(metricExists("my_counter_total")).To(BeTrue())

        // Trigger business logic that increments metric
        // ... create CRD, wait for reconciliation ...

        // Verify metric was incremented
        Eventually(func() float64 {
            return getCounterValue("my_counter_total", map[string]string{
                "namespace": testNamespace,
            })
        }, timeout, interval).Should(BeNumerically(">", 0))
    })
})
```

---

## 🎯 **Benefits Achieved**

1. ✅ **No port conflicts** - `:0` works perfectly
2. ✅ **Full metrics testing** - all tiers can validate metrics
3. ✅ **Type-safe access** - native Go protobuf DTOs
4. ✅ **Better performance** - no HTTP overhead
5. ✅ **Simpler tests** - no HTTP client, no parsing
6. ✅ **Parallel execution** - no resource contention

---

## 📞 **Support & Questions**

### **For Implementation Help**
- **Reference**: `test/integration/aianalysis/metrics_integration_test.go` (original)
- **Reference**: `test/integration/remediationorchestrator/operational_metrics_integration_test.go` (simplified)
- **Pattern**: Copy helper functions, adapt for your metrics

### **For Architecture Questions**
- **Contact**: Platform Team
- **Documentation**: This document + DD-METRICS-001

---

**Document Status**: ✅ **Complete**
**Pattern**: ✅ **Documented and Implemented**
**Services Updated**: 3/3 (AIAnalysis, RO, SP)
**Created**: 2025-12-25
**Last Updated**: 2025-12-25
**Owner**: Platform Team

