# BR-GATEWAY-SIGNAL-TERMINOLOGY: Signal Terminology Standard

**Business Requirement ID**: BR-GATEWAY-SIGNAL-TERMINOLOGY
**Status**: ✅ **MANDATORY** (Effective 2025-10-30)
**Priority**: P0 - CRITICAL
**Category**: Architecture / Business Domain
**Related ADR**: [ADR-015: Alert to Signal Naming Migration](../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md)

---

## 🎯 **Business Requirement**

**Kubernaut MUST use "Signal" terminology (NOT "Alert" terminology) in all code, metrics, documentation, and interfaces to accurately reflect its multi-source signal processing architecture.**

---

## 📋 **Business Justification**

### **Why This Matters**

Kubernaut is a **multi-signal intelligent remediation platform**, not just a Prometheus alert handler.

**Supported Signal Types**:
1. ✅ **Prometheus Alerts** (AlertManager webhooks)
2. ✅ **Kubernetes Events** (Warning/Error events)
3. ✅ **AWS CloudWatch Alarms** (future)
4. ✅ **Custom Webhooks** (extensible)
5. ✅ **Future Signal Sources** (Azure Monitor, Datadog, etc.)

**Using "Alert" terminology**:
- ❌ **Misleads operators**: Implies only Prometheus alerts are supported
- ❌ **Blocks adoption**: Kubernetes-only users think it's not for them
- ❌ **Confuses developers**: Creates parallel services per signal type
- ❌ **Limits business growth**: Harder to sell multi-cloud monitoring story

**Using "Signal" terminology**:
- ✅ **Accurate representation**: Reflects true multi-source capabilities
- ✅ **Broader market appeal**: Attracts Kubernetes, AWS, Azure customers
- ✅ **Developer clarity**: Single processing pipeline for all signals
- ✅ **Business scalability**: Easy to add new signal sources

---

## 🚨 **MANDATORY IMPLEMENTATION RULES**

### **Rule 1: Metric Names MUST Use "signal" (NOT "alert")**

**CORRECT** ✅:
```prometheus
gateway_signals_received_total{source="prometheus",severity="critical"} 150
gateway_signals_received_total{source="kubernetes-event",severity="warning"} 300
gateway_signals_deduplicated_total{signal_name="HighMemoryUsage"} 50
gateway_signal_storms_detected_total{storm_type="rate"} 5
```

**INCORRECT** ❌:
```prometheus
gateway_alerts_received_total{...}  # WRONG: Implies only Prometheus alerts
gateway_alerts_deduplicated_total{...}  # WRONG: Excludes Kubernetes events
gateway_alert_storms_detected_total{...}  # WRONG: Not just alert storms
```

**Business Impact**:
- ✅ Operators can monitor ALL signal types in Grafana
- ✅ SLOs can be defined per signal type
- ✅ Dashboards show complete system health (not just alerts)

---

### **Rule 2: Code MUST Use "signal" Variables and Functions**

**CORRECT** ✅:
```go
func ProcessSignal(ctx context.Context, signal types.Signal) error {
    signalCount++
    metrics.SignalsReceivedTotal.Inc()
    return processSignalPipeline(signal)
}
```

**INCORRECT** ❌:
```go
func ProcessAlert(ctx context.Context, alert types.Alert) error {  // WRONG
    alertCount++  // WRONG: What about Kubernetes events?
    metrics.AlertsReceivedTotal.Inc()  // WRONG: Misleading metric name
    return processAlertPipeline(alert)  // WRONG: Implies alerts only
}
```

**Business Impact**:
- ✅ Code is self-documenting about multi-signal support
- ✅ New developers immediately understand architecture
- ✅ Reduces onboarding time and confusion

---

### **Rule 3: Documentation MUST Use "signal" Terminology**

**CORRECT** ✅:
```markdown
## Signal Processing Pipeline

The Gateway processes signals from multiple sources:
- Prometheus alerts via AlertManager webhook
- Kubernetes events via Event API
- AWS CloudWatch alarms via SNS
```

**INCORRECT** ❌:
```markdown
## Alert Processing Pipeline  # WRONG: Implies only alerts

The Gateway processes alerts from Prometheus.  # WRONG: Incomplete picture
```

**Business Impact**:
- ✅ Documentation accurately represents product capabilities
- ✅ Marketing can confidently claim multi-source support
- ✅ Sales can demonstrate Kubernetes event handling

---

### **Rule 4: Comments MUST Use "signal" Terminology**

**CORRECT** ✅:
```go
// ProcessSignal handles signals from any source (Prometheus, Kubernetes, AWS)
// Business outcome: Unified signal processing reduces operational complexity
func ProcessSignal(ctx context.Context, signal types.Signal) error {
    // Deduplicate signal across all sources
    // Storm detection works for any signal type
}
```

**INCORRECT** ❌:
```go
// ProcessAlert handles Prometheus alerts  # WRONG: Too narrow
func ProcessAlert(ctx context.Context, alert types.Alert) error {  // WRONG
    // Deduplicate alerts  # WRONG: What about events?
}
```

**Business Impact**:
- ✅ Code reviews catch terminology violations
- ✅ AI code assistants learn correct terminology
- ✅ Future maintainers understand multi-signal design

---

## 📊 **Business Outcomes**

### **Operator Benefits**
- ✅ **Unified Monitoring**: Single Grafana dashboard for all signal types
- ✅ **Accurate SLOs**: Per-signal-type SLOs (alert SLO vs event SLO)
- ✅ **Complete Visibility**: No blind spots from terminology confusion

### **Developer Benefits**
- ✅ **Clear Architecture**: Immediately understand multi-signal design
- ✅ **Single Pipeline**: Reuse processing logic for all signal types
- ✅ **Easy Extension**: Add new signal types without architectural changes

### **Business Benefits**
- ✅ **Broader Market**: Appeal to Kubernetes-only and multi-cloud customers
- ✅ **Competitive Advantage**: Multi-signal support is a differentiator
- ✅ **Faster Sales Cycles**: Clear messaging about capabilities

---

## ✅ **Validation Criteria**

**This requirement is MET when**:
- ✅ ALL metrics use `gateway_signals_*` naming (NOT `gateway_alerts_*`)
- ✅ ALL code variables use `signal` (NOT `alert`)
- ✅ ALL function names use `Signal` (NOT `Alert`)
- ✅ ALL documentation uses "signal" terminology
- ✅ ALL comments reference "signals" (NOT "alerts")
- ✅ Code reviews reject PRs using "alert" terminology
- ✅ CI/CD linters flag "alert" terminology violations

---

## 🔗 **Related Documents**

- **Architectural Decision**: [ADR-015: Alert to Signal Naming Migration](../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md)
- **Signal Type Definitions**: [SIGNAL_TYPE_DEFINITIONS_DESIGN.md](../development/SIGNAL_TYPE_DEFINITIONS_DESIGN.md)
- **Gateway Service Overview**: [gateway-service/overview.md](../services/stateless/gateway-service/overview.md)
- **CRD Schemas**: [CRD_SCHEMAS.md](../architecture/CRD_SCHEMAS.md) (uses `SignalType` field)

---

## 📝 **Implementation Examples**

### **Example 1: Metric Registration**

**CORRECT** ✅:
```go
SignalsReceivedTotal: factory.NewCounterVec(
    prometheus.CounterOpts{
        Name: "gateway_signals_received_total",
        Help: "Total signals received by source, severity, and environment (Prometheus alerts, K8s events, etc.)",
    },
    []string{"source", "severity", "environment"},
)
```

**INCORRECT** ❌:
```go
AlertsReceivedTotal: factory.NewCounterVec(
    prometheus.CounterOpts{
        Name: "gateway_alerts_received_total",  // WRONG
        Help: "Total alerts received...",  // WRONG: Excludes events
    },
    []string{"source", "severity", "environment"},
)
```

### **Example 2: Processing Function**

**CORRECT** ✅:
```go
func (s *Server) ProcessSignal(ctx context.Context, signal types.Signal) error {
    // Increment signal counter (works for all sources)
    s.metrics.SignalsReceivedTotal.WithLabelValues(
        signal.Source,      // "prometheus" or "kubernetes-event"
        signal.Severity,
        signal.Environment,
    ).Inc()
    
    // Process signal through unified pipeline
    return s.signalPipeline.Process(ctx, signal)
}
```

**INCORRECT** ❌:
```go
func (s *Server) ProcessAlert(ctx context.Context, alert types.Alert) error {  // WRONG
    s.metrics.AlertsReceivedTotal.Inc()  // WRONG: What about K8s events?
    return s.alertPipeline.Process(ctx, alert)  // WRONG: Implies alerts only
}
```

---

## 🎯 **Success Metrics**

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Metric Naming Compliance** | 100% use `signal` | Grep codebase for `gateway_alerts_*` (should be 0) |
| **Code Variable Compliance** | 100% use `signal` | Grep codebase for `alert` variables (should be 0 in new code) |
| **Documentation Compliance** | 100% use "signal" | Grep docs for "alert processing" (should be 0) |
| **Developer Understanding** | 95%+ understand multi-signal | Survey: "Does Kubernaut handle only Prometheus alerts?" (Answer: NO) |
| **Operator Clarity** | 100% | Grafana dashboards show all signal types |

---

## 🚨 **Enforcement**

### **Code Review Checklist**
- [ ] Metrics use `gateway_signals_*` naming
- [ ] Variables use `signal` (NOT `alert`)
- [ ] Functions use `Signal` prefix (NOT `Alert`)
- [ ] Comments reference "signals" (NOT "alerts")
- [ ] Documentation uses "signal" terminology

### **CI/CD Validation**
```bash
# Fail build if new "alert" terminology is introduced
if git diff main...HEAD | grep -i "gateway_alerts_"; then
    echo "❌ VIOLATION: Use 'gateway_signals_*' metrics, not 'gateway_alerts_*'"
    exit 1
fi
```

### **Linter Rules**
```yaml
# .golangci.yml
linters-settings:
  gocritic:
    enabled-checks:
      - commentFormatting
    settings:
      commentFormatting:
        # Reject comments using "alert" instead of "signal"
        bannedWords: ["alert processing", "handle alerts", "process alerts"]
```

---

**Approved By**: Architecture Review Board
**Effective Date**: 2025-10-30
**Review Date**: Quarterly (or when adding new signal types)
**Owner**: Gateway Service Team

---

## 📚 **Appendix: Migration Checklist**

For existing code using "alert" terminology:

- [ ] Rename metrics: `gateway_alerts_*` → `gateway_signals_*`
- [ ] Update metric labels: `alertname` → `signal_name`
- [ ] Rename variables: `alert` → `signal`, `alertCount` → `signalCount`
- [ ] Update functions: `ProcessAlert()` → `ProcessSignal()`
- [ ] Fix comments: "handles alerts" → "handles signals"
- [ ] Update docs: "alert processing" → "signal processing"
- [ ] Update tests: Verify multi-signal scenarios
- [ ] Update Grafana dashboards: Show all signal types
- [ ] Update runbooks: Reference "signals" not "alerts"
- [ ] Update API specs: Use "signal" terminology

---

**Version**: 1.0
**Last Updated**: 2025-10-30
**Status**: ✅ **ACTIVE AND MANDATORY**

