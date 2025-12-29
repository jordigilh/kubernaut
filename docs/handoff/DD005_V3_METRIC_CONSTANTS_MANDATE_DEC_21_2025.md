# DD-005 V3.0: Metric Name Constants Now MANDATORY

**Date**: December 21, 2025
**Author**: AI Assistant (WE Team)
**Status**: ✅ APPROVED FOR PRODUCTION
**Authoritative Document**: [DD-005-OBSERVABILITY-STANDARDS.md](../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md)

---

## 📬 **Handoff Routing**

| Field | Value |
|-------|-------|
| **From** | WorkflowExecution Team |
| **To** | **ALL SERVICE TEAMS** (Notification, SignalProcessing, RemediationOrchestrator, AIAnalysis, Gateway, DataStorage) |
| **CC** | Platform/Observability Team |
| **Action Required** | **MANDATORY**: Implement metric name constants in all services |
| **Response Deadline** | Before respective service's V1.0 release |
| **Priority** | 🔴 **P0 - CRITICAL** (Prevents E2E test failures) |

---

## 🎯 **Executive Summary**

**DD-005 V3.0 Update**: Metric name constants are now **MANDATORY** for all services.

**Root Cause**: WorkflowExecution E2E tests failed due to hardcoded metric name typos (`workflowexecution_total` vs. `workflowexecution_reconciler_total`).

**Solution**: All services MUST define exported constants for metric names and label values.

**Impact**: Prevents typos, ensures test/production parity, reduces maintenance burden.

---

## 🚨 **What Changed in DD-005 V3.0**

### **New Section: 1.1. Metric Name Constants (MANDATORY)**

**Location**: [DD-005 Section 1.1](../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md#11-metric-name-constants-mandatory)

**Key Requirements**:
1. ✅ Define `MetricName*` constants for all metric names
2. ✅ Define `Label*` constants for common label values
3. ✅ Use constants in production code (`prometheus.CounterOpts{Name: ...}`)
4. ✅ Use constants in test code (import from production package)
5. ✅ Export constants (capitalize) for test access
6. ✅ Document constants with Go doc comments

---

## 📊 **Service Compliance Status**

| Service | Constants Defined | Status | Action Required |
|---------|------------------|--------|-----------------|
| **WorkflowExecution** | ✅ Yes | ✅ Compliant | None (reference implementation) |
| **Notification** | ❌ No | 🔴 **Non-compliant** | Implement constants |
| **SignalProcessing** | ❌ No | 🔴 **Non-compliant** | Implement constants |
| **RemediationOrchestrator** | ❌ No | 🔴 **Non-compliant** | Implement constants |
| **AIAnalysis** | ❌ No | 🔴 **Non-compliant** | Implement constants |
| **Gateway** | ❌ No | 🔴 **Non-compliant** | **TRIAGE COMPLETE** - See [GW_METRICS_TRIAGE_DEC_21_2025.md](GW_METRICS_TRIAGE_DEC_21_2025.md) |
| **DataStorage** | ❓ Unknown | 🔄 **Triage required** | Assess compliance |

---

## 🔍 **Root Cause: WorkflowExecution E2E Failures**

### **The Bug**

E2E tests were using **incorrect metric names**:
```go
// ❌ Test code (WRONG)
expectedMetrics := []string{
	"workflowexecution_total",              // Missing "_reconciler"
	"workflowexecution_duration_seconds",   // Missing "_reconciler"
}
```

**Actual metric names** (production):
```go
// ✅ Production code (CORRECT)
Name: "workflowexecution_reconciler_total",
Name: "workflowexecution_reconciler_duration_seconds",
```

**Result**: 3 E2E tests failed because metrics were never found.

### **Why It Happened**

**27 duplication sites** with hardcoded strings:
- 6 in `NewMetrics()`
- 6 in `NewMetricsWithRegistry()`
- 4 in recording methods
- 11 in test files

**No constants** → Easy to make typos → Tests used wrong names → Runtime failures.

---

## ✅ **Solution: Metric Name Constants**

### **Reference Implementation: WorkflowExecution**

**File**: `pkg/workflowexecution/metrics/metrics.go`

```go
package metrics

// Metric name constants - DRY principle for tests and production
// These constants ensure tests use correct metric names and prevent typos.
const (
	// MetricNameExecutionTotal is the name of the execution counter metric
	MetricNameExecutionTotal = "workflowexecution_reconciler_total"

	// MetricNameExecutionDuration is the name of the execution duration histogram metric
	MetricNameExecutionDuration = "workflowexecution_reconciler_duration_seconds"

	// MetricNamePipelineRunCreations is the name of the PipelineRun creation counter metric
	MetricNamePipelineRunCreations = "workflowexecution_reconciler_pipelinerun_creations_total"

	// Label values for outcome dimension
	// LabelOutcomeCompleted indicates successful workflow completion
	LabelOutcomeCompleted = "Completed"

	// LabelOutcomeFailed indicates workflow failure
	LabelOutcomeFailed = "Failed"
)

// Production usage
func NewMetrics() *Metrics {
	m := &Metrics{
		ExecutionTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameExecutionTotal, // Use constant ✅
				Help: "Total number of workflow executions by outcome",
			},
			[]string{"outcome"},
		),
		// ... other metrics
	}
	return m
}

// Recording methods
func (m *Metrics) RecordWorkflowCompletion(durationSeconds float64) {
	m.ExecutionTotal.WithLabelValues(LabelOutcomeCompleted).Inc() // Use constant ✅
	m.ExecutionDuration.WithLabelValues(LabelOutcomeCompleted).Observe(durationSeconds)
}
```

**Test usage**:
```go
import wemetrics "github.com/jordigilh/kubernaut/pkg/workflowexecution/metrics"

func TestMetrics(t *testing.T) {
	// Type-safe: compiler catches typos ✅
	count := extractMetricValue(body,
		wemetrics.MetricNameExecutionTotal,      // Constant from production
		wemetrics.LabelOutcomeCompleted)         // Constant from production

	require.Greater(t, count, 0.0)
}
```

---

## 📋 **Implementation Checklist for Service Teams**

### **Step 1: Define Constants** (5 minutes)

Create constants in `pkg/{service}/metrics/metrics.go`:

```go
const (
	// Metric names
	MetricNameYourMetric = "service_component_metric_total"

	// Label values
	LabelValueSuccess = "success"
	LabelValueError   = "error"
)
```

### **Step 2: Update Production Code** (10 minutes)

Replace hardcoded strings with constants:

```go
// Before ❌
Name: "service_component_metric_total",

// After ✅
Name: MetricNameYourMetric,
```

### **Step 3: Update Test Code** (10 minutes)

Import constants from production package:

```go
// Before ❌
if !strings.Contains(body, "service_component_metric_total") { ... }

// After ✅
import svcmetrics "github.com/jordigilh/kubernaut/pkg/{service}/metrics"
if !strings.Contains(body, svcmetrics.MetricNameYourMetric) { ... }
```

### **Step 4: Verify** (5 minutes)

```bash
# Compilation catches errors
go build ./pkg/{service}/metrics/...
go test -c ./test/e2e/{service}/... -o /dev/null

# Run tests
make test-e2e-{service}
```

**Total Time**: ~30 minutes per service

---

## 🎯 **Benefits**

| Aspect | Without Constants | With Constants |
|--------|------------------|----------------|
| **Typo Prevention** | Runtime errors | Compile-time errors |
| **Maintenance** | Update 20+ locations | Update 1 location |
| **Test Safety** | Wrong names pass tests | Compiler enforces correctness |
| **Refactoring** | Manual find/replace | IDE "Find Usages" + Rename |
| **Documentation** | Implicit in strings | Explicit via Go doc comments |

---

## 📚 **Reference Documents**

### **Authoritative Standard**
- **DD-005 V3.0**: [Observability Standards](../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md)
  - Section 1.1: Metric Name Constants (MANDATORY)

### **Implementation Examples**
- **WorkflowExecution**: `pkg/workflowexecution/metrics/metrics.go`
- **E2E Tests**: `test/e2e/workflowexecution/02_observability_test.go`

### **Related Documents**
- **Root Cause Analysis**: [WE_METRICS_DRY_REFACTOR_DEC_21_2025.md](WE_METRICS_DRY_REFACTOR_DEC_21_2025.md)
- **DD-METRICS-001**: [Controller Metrics Wiring Pattern](../architecture/decisions/DD-METRICS-001-controller-metrics-wiring-pattern.md)
- **DD-TEST-005**: [Metrics Unit Testing Standard](../architecture/decisions/DD-TEST-005-metrics-unit-testing-standard.md)

---

## ⚠️ **Action Required by Service Teams**

### **Notification Team**
- [ ] Triage current metrics implementation
- [ ] Define metric name constants
- [ ] Update production code to use constants
- [ ] Update test code to import constants
- [ ] Verify E2E tests pass
- [ ] Document in handoff: `NT_METRICS_CONSTANTS_COMPLIANCE_DEC_XX_2025.md`

### **SignalProcessing Team**
- [ ] Triage current metrics implementation
- [ ] Define metric name constants
- [ ] Update production code to use constants
- [ ] Update test code to import constants
- [ ] Verify E2E tests pass
- [ ] Document in handoff: `SP_METRICS_CONSTANTS_COMPLIANCE_DEC_XX_2025.md`

### **RemediationOrchestrator Team**
- [ ] Triage current metrics implementation
- [ ] Define metric name constants
- [ ] Update production code to use constants
- [ ] Update test code to import constants
- [ ] Verify E2E tests pass
- [ ] Document in handoff: `RO_METRICS_CONSTANTS_COMPLIANCE_DEC_XX_2025.md`

### **AIAnalysis Team**
- [ ] Triage current metrics implementation
- [ ] Define metric name constants
- [ ] Update production code to use constants
- [ ] Update test code to import constants
- [ ] Verify E2E tests pass
- [ ] Document in handoff: `AI_METRICS_CONSTANTS_COMPLIANCE_DEC_XX_2025.md`

### **Gateway Team**
- [ ] Triage current metrics implementation for compliance
- [ ] If non-compliant: Follow implementation checklist
- [ ] Document in handoff: `GW_METRICS_CONSTANTS_COMPLIANCE_DEC_XX_2025.md`

### **DataStorage Team**
- [ ] Triage current metrics implementation for compliance
- [ ] If non-compliant: Follow implementation checklist
- [ ] Document in handoff: `DS_METRICS_CONSTANTS_COMPLIANCE_DEC_XX_2025.md`

---

## 🎓 **Key Takeaways**

1. **Metric name constants are MANDATORY** (DD-005 V3.0)
2. **WorkflowExecution is the reference implementation**
3. **All services must comply before V1.0 release**
4. **Implementation takes ~30 minutes per service**
5. **Prevents E2E test failures and maintenance burden**

---

## 📞 **Questions?**

- **DD-005 Clarifications**: Review [Section 1.1](../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md#11-metric-name-constants-mandatory)
- **Implementation Help**: Reference WorkflowExecution implementation
- **Technical Questions**: Consult with WorkflowExecution team (implemented first)

---

**End of Document**

