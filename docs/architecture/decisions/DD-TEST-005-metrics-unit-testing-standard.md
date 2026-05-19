# DD-TEST-005: Metrics Unit Testing Standard

**Status**: 📋 **DRAFT** (Awaiting Team Review)
**Date**: December 16, 2025
**Last Reviewed**: May 19, 2026
**Confidence**: 95%
**Based On**: Gateway/DataStorage Reference Implementations + AF Wiring Gap RCA (Issue #1176)

---

## 🎯 **Overview**

This design decision establishes **mandatory metrics unit testing standards** for all Kubernaut services, covering:
1. **Test File Requirements** - Mandatory `metrics_test.go` for all services with metrics
2. **Testing Pattern** - Authoritative `prometheus/client_model/go` (`dto`) pattern
3. **Coverage Requirements** - All metrics must have value verification tests
4. **Anti-Patterns** - NULL-TESTING patterns are FORBIDDEN

**Key Principle**: Metrics unit tests MUST verify actual metric VALUES, not just existence.

**Scope**: All Kubernaut services with Prometheus metrics.

---

## 📋 **Table of Contents**

1. [Context & Problem](#context--problem)
2. [Requirements](#requirements)
3. [Decision](#decision)
4. [Authoritative Pattern](#authoritative-pattern)
5. [Forbidden Anti-Patterns](#forbidden-anti-patterns)
6. [Implementation Guide](#implementation-guide)
7. [Service Compliance Status](#service-compliance-status)
8. [Migration Guide](#migration-guide)
9. [References](#references)

---

## 🎯 **Context & Problem**

### **Challenge**

Cross-team triage revealed inconsistent metrics unit testing across services:

| Issue | Impact |
|-------|--------|
| **Missing Tests** | Notification service has NO metrics tests despite full implementation |
| **NULL-TESTING** | AIAnalysis, RO, SP use `NotTo(BeNil())` which provides ZERO coverage |
| **No Standard** | Each team invented their own (or no) testing approach |
| **DD-005 Validation Gap** | Naming conventions not validated at unit test level |

### **Triage Results (December 16, 2025)**

| Service | Metrics Implementation | Unit Tests | Pattern | Status |
|---------|----------------------|------------|---------|--------|
| **Gateway** | ✅ Complete | ✅ Complete | `dto` + `.Write()` | ✅ Authoritative |
| **DataStorage** | ✅ Complete | ✅ Complete | `dto` + helpers | ✅ Authoritative |
| **SignalProcessing** | ✅ Complete | ⚠️ NULL-TESTING | `NotTo(BeNil())` | ❌ Non-Compliant |
| **AIAnalysis** | ✅ Complete | ⚠️ NULL-TESTING | `NotTo(Panic())` | ❌ Non-Compliant |
| **RemediationOrchestrator** | ✅ Complete | ⚠️ NULL-TESTING | `ToNot(Panic())` | ❌ Non-Compliant |
| **WorkflowExecution** | ✅ Complete | ⚠️ Missing | "Deferred" | ❌ Non-Compliant |
| **Notification** | ✅ Complete | ❌ **MISSING** | N/A | 🚨 Critical Gap |

### **Business Impact**

| Impact Area | Risk Level | Description |
|-------------|------------|-------------|
| **Production Reliability** | 🔴 HIGH | Metrics may silently break without test detection |
| **DD-005 Compliance** | 🔴 HIGH | Naming conventions not validated at unit level |
| **Observability** | 🔴 HIGH | SLO monitoring depends on correct metrics |
| **Regression Detection** | 🔴 HIGH | Future changes may break metrics without test failure |

---

## 📋 **Requirements**

### **Functional Requirements**

| ID | Requirement | Priority | Status |
|----|-------------|----------|--------|
| **FR-1** | All services with metrics MUST have `metrics_test.go` | P0 | 📋 Draft |
| **FR-2** | All tests MUST verify metric VALUES, not just existence | P0 | 📋 Draft |
| **FR-3** | Counter tests MUST verify increment behavior | P0 | 📋 Draft |
| **FR-4** | Histogram tests MUST verify observation recording | P0 | 📋 Draft |
| **FR-5** | Tests MUST use `prometheus/client_model/go` (`dto`) pattern | P0 | 📋 Draft |
| **FR-6** | NULL-TESTING patterns are FORBIDDEN | P0 | 📋 Draft |

### **Non-Functional Requirements**

| ID | Requirement | Target | Status |
|----|-------------|--------|--------|
| **NFR-1** | Metrics test execution time | < 5 seconds | 📋 Draft |
| **NFR-2** | All metrics from `pkg/{service}/metrics/` covered | 100% | 📋 Draft |

---

## ✅ **Decision**

**APPROVED**: Standardize metrics unit testing across all Kubernaut services

**Rationale**:
1. **Gateway/DataStorage** provide proven, working patterns
2. **NULL-TESTING** provides zero actual coverage
3. **DD-005 Compliance** requires unit-level validation
4. **Production Reliability** depends on correct metrics

---

## ⚠️ **Unit Test Limitation: Wiring Gaps**

### **Problem Statement**

Unit tests that exercise metrics in isolation (even with `dto` value verification) **cannot guarantee that production code actually wires and increments those metrics**. A metric can pass all UT assertions while remaining dead code in production because:

1. The registry field is nil-guarded at the call site (`if reg != nil { ... }`) and the registry is never injected
2. The metric is registered in `NewRegistry()` but the struct field is never referenced in the handler
3. The production wiring (`main.go` or config struct) omits the field entirely

**Real-world example (Issue #1176)**: `af_discover_workflows_*` metrics passed all unit tests but were never emitted in production because `MetricsRegistry` was not assigned in `MCPBridgeConfig` — the nil check silently skipped instrumentation.

### **Required: Integration-Tier Wiring Tests**

To guarantee metrics are wired end-to-end, services MUST include **integration-tier wiring tests** that:

1. Instantiate the real handler/bridge with a real `metrics.Registry`
2. Execute actual business logic through the production code path (e.g., make a tool call via the MCP bridge)
3. Inspect the registry using `dto` to verify the counter/histogram actually changed

This is the **only** way to catch "registered but never wired" bugs.

### **Wiring Test Pattern (Authoritative Reference: `pkg/apifrontend/handler/mcp_bridge_test.go`)**

```go
var _ = Describe("Metrics Wiring", Label("metrics", "wiring"), func() {
    var (
        h          http.Handler
        sessionID  string
        metricsReg *metrics.Registry
    )

    BeforeEach(func() {
        // Use real registry — same struct production code uses
        metricsReg = metrics.NewRegistry()

        cfg := handler.BridgeConfig{
            Metrics:         bridgeMetricsFrom(metricsReg),
            MetricsRegistry: metricsReg, // <-- the field that was missing
            // ... other real deps
        }
        h = handler.NewHandler(cfg)
        sessionID = initSession(h)
    })

    It("successful tool call increments tool_calls_total{result=success}", func() {
        before := getCounterValue(metricsReg.ToolCallsTotal,
            prometheus.Labels{"tool": "my_tool", "result": "success"})

        callTool(h, sessionID, "my_tool", validArgs)

        after := getCounterValue(metricsReg.ToolCallsTotal,
            prometheus.Labels{"tool": "my_tool", "result": "success"})
        Expect(after - before).To(Equal(float64(1)))
    })
})
```

### **Testing Tiers Summary**

| Tier | What It Validates | Catches |
|------|------------------|---------|
| **Unit (dto)** | Registry construction, label schemas, increment math | Wrong label cardinality, registration panics |
| **Integration (wiring)** | Production code path actually emits metrics | Nil-guarded fields, missing config wiring, dead code |
| **E2E (scrape)** | `/metrics` endpoint exposes expected families in a running cluster | Deployment misconfig, runtime env issues |

All three tiers are required for GA-ready metrics coverage.

---

## 📊 **Authoritative Pattern**

### **Reference Implementations**

| Service | File | Pattern |
|---------|------|---------|
| **Gateway** | `test/unit/gateway/metrics/metrics_test.go` | Full `dto` pattern |
| **DataStorage** | `test/unit/datastorage/metrics_test.go` | Helper function pattern |

### **Standard Test Structure**

```go
package {service}_test

import (
    "github.com/prometheus/client_golang/prometheus"
    dto "github.com/prometheus/client_model/go"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "{service}/metrics"
)

// ========================================
// Helper Functions (from DataStorage)
// ========================================

func getCounterValue(counter prometheus.Counter) float64 {
    metric := &dto.Metric{}
    if err := counter.Write(metric); err != nil {
        return 0
    }
    return metric.GetCounter().GetValue()
}

func getGaugeValue(gauge prometheus.Gauge) float64 {
    metric := &dto.Metric{}
    if err := gauge.Write(metric); err != nil {
        return 0
    }
    return metric.GetGauge().GetValue()
}

func getHistogramSampleCount(obs prometheus.Observer) uint64 {
    metric := &dto.Metric{}
    obs.(prometheus.Metric).Write(metric)
    return metric.GetHistogram().GetSampleCount()
}

func getHistogramSampleSum(obs prometheus.Observer) float64 {
    metric := &dto.Metric{}
    obs.(prometheus.Metric).Write(metric)
    return metric.GetHistogram().GetSampleSum()
}

// ========================================
// Test Suite
// ========================================

var _ = Describe("BR-{SERVICE}-XXX: Metrics", func() {
    Context("Counter Metrics", func() {
        It("should increment {MetricName} counter", func() {
            // Get baseline
            before := getCounterValue(
                metrics.{MetricName}.WithLabelValues("label1", "label2"))

            // Execute
            metrics.{MetricName}.WithLabelValues("label1", "label2").Inc()

            // Verify increment
            after := getCounterValue(
                metrics.{MetricName}.WithLabelValues("label1", "label2"))
            Expect(after - before).To(Equal(float64(1)))
        })
    })

    Context("Histogram Metrics", func() {
        It("should observe {MetricName} duration", func() {
            // Get baseline
            beforeCount := getHistogramSampleCount(
                metrics.{MetricName}.WithLabelValues("label1"))
            beforeSum := getHistogramSampleSum(
                metrics.{MetricName}.WithLabelValues("label1"))

            // Execute
            testDuration := 0.5
            metrics.{MetricName}.WithLabelValues("label1").Observe(testDuration)

            // Verify observation
            afterCount := getHistogramSampleCount(
                metrics.{MetricName}.WithLabelValues("label1"))
            afterSum := getHistogramSampleSum(
                metrics.{MetricName}.WithLabelValues("label1"))

            Expect(afterCount - beforeCount).To(Equal(uint64(1)))
            Expect(afterSum - beforeSum).To(BeNumerically("~", testDuration, 0.001))
        })
    })

    Context("Gauge Metrics", func() {
        It("should set {MetricName} gauge", func() {
            // Execute
            testValue := 42.0
            metrics.{MetricName}.WithLabelValues("label1").Set(testValue)

            // Verify
            value := getGaugeValue(
                metrics.{MetricName}.WithLabelValues("label1"))
            Expect(value).To(Equal(testValue))
        })
    })

    Context("DD-005 Naming Compliance", func() {
        It("should follow {service}_{component}_{metric}_{unit} naming", func() {
            // Verify metric name follows DD-005 convention
            desc := metrics.{MetricName}.WithLabelValues("label1", "label2").Desc()
            Expect(desc.String()).To(ContainSubstring("{service}_{component}_{metric}"))
        })
    })
})
```

---

## 🚫 **Forbidden Anti-Patterns**

### **NULL-TESTING (FORBIDDEN)**

```go
// ❌ FORBIDDEN: Only checks existence, provides ZERO coverage
It("should create metrics", func() {
    Expect(metrics.ProcessingTotal).NotTo(BeNil())
})

// ❌ FORBIDDEN: Only checks it doesn't panic, provides ZERO coverage
It("should register metrics", func() {
    Expect(func() {
        metrics.ProcessingTotal.WithLabelValues("phase", "status")
    }).ToNot(Panic())
})

// ❌ FORBIDDEN: Only checks metric object exists
It("should have processing counter", func() {
    counter := metrics.ProcessingTotal.WithLabelValues("enriching", "success")
    Expect(counter).NotTo(BeNil())
})
```

### **Why NULL-TESTING is Forbidden**

| Issue | Impact |
|-------|--------|
| **Zero Value Verification** | Doesn't verify metric actually records values |
| **No Increment Testing** | Counter could be broken but test still passes |
| **No Observation Testing** | Histogram could fail silently |
| **False Confidence** | Test passes but metrics don't work |

---

## 🔧 **Implementation Guide**

### **Step 1: Create Test File**

```bash
# Location: test/unit/{service}/metrics_test.go
touch test/unit/{service}/metrics_test.go
```

### **Step 2: Import Required Packages**

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    dto "github.com/prometheus/client_model/go"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/{service}/metrics"
)
```

### **Step 3: Add Helper Functions**

Copy the helper functions from the authoritative pattern section.

### **Step 4: Write Tests for Each Metric**

For each metric in `pkg/{service}/metrics/metrics.go`:
1. **Counter**: Test increment behavior
2. **Histogram**: Test observation recording
3. **Gauge**: Test set/add behavior

### **Step 5: Validate DD-005 Compliance**

Add tests to verify metric naming follows `{service}_{component}_{metric}_{unit}` format.

---

## 📊 **Service Compliance Status**

### **Compliance Matrix**

| Service | Test File | `dto` Pattern | Value Verification | DD-005 Validation | Status |
|---------|-----------|--------------|-------------------|-------------------|--------|
| **Gateway** | ✅ | ✅ | ✅ | ✅ | ✅ Compliant |
| **DataStorage** | ✅ | ✅ | ✅ | ✅ | ✅ Compliant |
| **SignalProcessing** | ✅ | ❌ | ❌ | ❌ | 🔄 Remediation |
| **AIAnalysis** | ✅ | ❌ | ❌ | ❌ | ⬜ Pending |
| **RemediationOrchestrator** | ✅ | ❌ | ❌ | ❌ | ⬜ Pending |
| **WorkflowExecution** | ❌ | ❌ | ❌ | ❌ | ⬜ Pending |
| **Notification** | ❌ | ❌ | ❌ | ❌ | 🚨 Critical |

### **Compliance Timeline**

| Service | Deadline | Effort | Assignee |
|---------|----------|--------|----------|
| **SignalProcessing** | Dec 17, 2025 | 3-4 hours | @jgil |
| **Notification** | Jan 3, 2026 | 3-4 hours | TBD |
| **AIAnalysis** | Jan 10, 2026 | 2-3 hours | TBD |
| **RemediationOrchestrator** | Jan 10, 2026 | 2-3 hours | TBD |
| **WorkflowExecution** | Jan 10, 2026 | 2-3 hours | TBD |

---

## 🔄 **Migration Guide**

### **For Services with NULL-TESTING**

```go
// BEFORE (❌ NULL-TESTING)
It("should create metrics", func() {
    Expect(metrics.ProcessingTotal).NotTo(BeNil())
})

// AFTER (✅ VALUE VERIFICATION)
It("should increment processing counter", func() {
    before := getCounterValue(metrics.ProcessingTotal.WithLabelValues("enriching", "success"))
    metrics.ProcessingTotal.WithLabelValues("enriching", "success").Inc()
    after := getCounterValue(metrics.ProcessingTotal.WithLabelValues("enriching", "success"))
    Expect(after - before).To(Equal(float64(1)))
})
```

### **For Services with Missing Tests**

1. Create `test/unit/{service}/metrics_test.go`
2. Copy helper functions from authoritative pattern
3. Add tests for each metric in `pkg/{service}/metrics/`
4. Run tests: `make test-unit-{service}`

---

## 📚 **References**

| Document | Purpose |
|----------|---------|
| [DD-005-OBSERVABILITY-STANDARDS.md](./DD-005-OBSERVABILITY-STANDARDS.md) | Metrics naming conventions |
| [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md) | Test structure and anti-patterns |
| `test/unit/gateway/metrics/metrics_test.go` | Authoritative reference implementation |
| `test/unit/datastorage/metrics_test.go` | Authoritative reference implementation |
| [TEAM_NOTIFICATION_METRICS_UNIT_TESTING_GAP.md](../../handoff/TEAM_NOTIFICATION_METRICS_UNIT_TESTING_GAP.md) | Team notification document |

---

## ✅ **Approval Status**

| Role | Name | Date | Status |
|------|------|------|--------|
| **Author** | SignalProcessing Team | 2025-12-16 | ✅ Draft Complete |
| **Gateway Team** | - | - | ⬜ Pending Review |
| **DataStorage Team** | - | - | ⬜ Pending Review |
| **AIAnalysis Team** | - | - | ⬜ Pending Review |
| **Notification Team** | - | - | ⬜ Pending Review |
| **RemediationOrchestrator Team** | - | - | ⬜ Pending Review |
| **WorkflowExecution Team** | - | - | ⬜ Pending Review |

---

## 📊 **Success Metrics**

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Service Compliance** | 100% | All 7 services have compliant `metrics_test.go` |
| **NULL-TESTING Elimination** | 0 instances | `grep -r "NotTo(BeNil())\|NotTo(Panic())" test/unit/*/metrics_test.go` |
| **Value Verification** | 100% | All metrics have increment/observation tests |

---

**Document Created By**: AI Assistant (SignalProcessing Team)
**Date**: December 16, 2025
**Status**: 📋 DRAFT - Awaiting Team Review



