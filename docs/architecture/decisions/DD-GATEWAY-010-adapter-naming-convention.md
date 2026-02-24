# DD-GATEWAY-010: Adapter Naming Convention (SignalSource vs SignalType)

## Status
**✅ APPROVED** (2025-11-21)
**Last Reviewed**: 2025-11-21
**Confidence**: 100%

---

## Revision: Issue #166 SignalType Rename (2026-02)

**SignalType values are now normalized to `"alert"`** (generic). The source-specific values `"prometheus-alert"` and `"kubernetes-event"` documented below were superseded by Issue #166. RR.Spec.SignalType now uses the generic value `"alert"` for all adapters.

**Adapter identity for audit/metrics** now uses `signal.Source` (e.g., `"prometheus"`, `"kubernetes-events"`) rather than `signal.SourceType` or source-specific SignalType values. This provides adapter identity for observability while keeping RR.Spec.SignalType generic.

The historical content below documents the original design rationale; implementation follows the Issue #166 conventions.

---

## Context & Problem

The Gateway service uses adapters to convert signals from various monitoring systems (Prometheus, Kubernetes Events, Grafana, etc.) into a unified `NormalizedSignal` format. Each adapter must provide two key identifiers:

1. **SignalSource**: Identifies the **monitoring system** that generated the signal
2. **SignalType**: Identifies the **type of signal** received

**Key Requirements**:
- **LLM Tool Selection**: SignalSource must map clearly to investigation tools (kubectl, promql, etc.)
- **Metrics/Logging**: SignalType must enable clear signal classification
- **Consistency**: All adapters must follow the same naming pattern
- **Extensibility**: Pattern must work for future adapters (Grafana, Datadog, etc.)

**Problem Statement**: Should we use singular or plural forms for these identifiers? What naming convention ensures consistency across all adapters?

**Trigger**: During integration testing, a test expected `"kubernetes-event"` (singular) for SignalSource but the adapter correctly returned `"kubernetes-events"` (plural). This revealed the need to document the naming convention.

---

## Alternatives Considered

### Alternative 1: All Singular (Rejected)
**Approach**: Use singular form for both SignalSource and SignalType

**Example**:
```go
// Prometheus Adapter
SignalSource: "prometheus"      // ✅ Correct (system name)
SignalType:   "prometheus-alert" // ✅ Correct

// Kubernetes Event Adapter
SignalSource: "kubernetes-event"  // ❌ Wrong (should be "kubernetes-events")
SignalType:   "kubernetes-event"  // ✅ Correct
```

**Pros**:
- ✅ Simple rule (always singular)
- ✅ Easy to remember

**Cons**:
- ❌ Doesn't match Kubernetes API conventions (`events.v1.core` is plural)
- ❌ Breaks LLM tool selection (`kubectl get events` not `kubectl get event`)
- ❌ Inconsistent with monitoring system names (K8s Events API is plural)

**Confidence**: 40% (rejected - doesn't match K8s conventions)

---

### Alternative 2: All Plural (Rejected)
**Approach**: Use plural form for both SignalSource and SignalType

**Example**:
```go
// Prometheus Adapter
SignalSource: "prometheus"        // ✅ Correct (system name has no plural)
SignalType:   "prometheus-alerts" // ❌ Wrong (one alert, not multiple)

// Kubernetes Event Adapter
SignalSource: "kubernetes-events" // ✅ Correct (K8s Events API)
SignalType:   "kubernetes-events" // ❌ Wrong (one event, not multiple)
```

**Pros**:
- ✅ Matches K8s API naming for Events

**Cons**:
- ❌ Doesn't work for systems without plural forms (Prometheus, Grafana)
- ❌ Incorrect for SignalType (each signal is ONE event/alert)
- ❌ Confusing metrics (`signals_received_total{signal_type="prometheus-alerts"}` implies multiple)

**Confidence**: 30% (rejected - doesn't work universally)

---

### Alternative 3: Use Monitoring System Name As-Is for SignalSource, Singular for SignalType (APPROVED)
**Approach**:
- **SignalSource**: Use the **actual monitoring system name** (as documented by the system)
- **SignalType**: Use **singular form** (one event/alert per signal)

**Example**:
```go
// Prometheus Adapter
SignalSource: "prometheus"       // ✅ System name (Prometheus)
SignalType:   "prometheus-alert" // ✅ One alert (singular)

// Kubernetes Event Adapter
SignalSource: "kubernetes-events" // ✅ System name (K8s Events API - plural)
SignalType:   "kubernetes-event"  // ✅ One event (singular)

// Future: Grafana Adapter
SignalSource: "grafana"          // ✅ System name (Grafana)
SignalType:   "grafana-alert"    // ✅ One alert (singular)
```

**Pros**:
- ✅ **Matches official system names** (K8s Events API is officially plural)
- ✅ **Clear LLM tool selection** (`kubernetes-events` → `kubectl get events`)
- ✅ **Correct event-driven semantics** (SignalType is singular - one event)
- ✅ **Works for all systems** (respects each system's naming convention)
- ✅ **Consistent metrics** (`signal_type="kubernetes-event"` counts individual events)

**Cons**:
- ⚠️ Requires developers to check official system documentation (minimal overhead)

**Confidence**: 100% (approved)

---

## Decision

**APPROVED: Alternative 3** - Use Monitoring System Name As-Is for SignalSource, Singular for SignalType

**Rationale**:
1. **System Name Accuracy**: SignalSource must match the official monitoring system name for LLM tool selection
2. **K8s API Alignment**: "Kubernetes Events" is the official name (plural) - matches `events.v1.core` API
3. **Event-Driven Semantics**: SignalType represents one signal (singular) - consistent with event architecture
4. **LLM Tool Selection**: `signal_source="kubernetes-events"` → LLM knows to use `kubectl get events`
5. **Metrics Clarity**: `signal_type="kubernetes-event"` counts individual events (singular)

**Key Insight**: SignalSource and SignalType serve different purposes:
- **SignalSource**: External system identification (use official name)
- **SignalType**: Internal signal classification (use singular form)

---

## Implementation

### Primary Implementation Files

**Prometheus Adapter**: `pkg/gateway/adapters/prometheus_adapter.go`

```go
// GetSourceService returns the monitoring system name (BR-GATEWAY-027)
//
// Returns "prometheus" (the monitoring system) instead of "prometheus-adapter" (the adapter name).
// The LLM uses this to select appropriate investigation tools:
// - signal_source="prometheus" → LLM uses Prometheus queries for investigation
//
// This is the SOURCE MONITORING SYSTEM, not the adapter implementation name.
func (a *PrometheusAdapter) GetSourceService() string {
	return "prometheus"  // ✅ System name (no plural form)
}

// GetSourceType returns the signal type identifier
//
// Returns "prometheus-alert" to distinguish Prometheus alerts from other signal types.
// Used for metrics, logging, and signal classification.
func (a *PrometheusAdapter) GetSourceType() string {
	return "prometheus-alert"  // ✅ One alert (singular)
}
```

**Kubernetes Event Adapter**: `pkg/gateway/adapters/kubernetes_event_adapter.go`

```go
// GetSourceService returns the monitoring system name (BR-GATEWAY-027)
//
// Returns "kubernetes-events" (the monitoring system) instead of "k8s-event-adapter" (the adapter name).
// The LLM uses this to select appropriate investigation tools:
// - signal_source="kubernetes-events" → LLM uses kubectl for investigation
//
// This is the SOURCE MONITORING SYSTEM, not the adapter implementation name.
func (a *KubernetesEventAdapter) GetSourceService() string {
	return "kubernetes-events"  // ✅ System name (K8s Events API - plural)
}

// GetSourceType returns the signal type identifier
//
// Returns "kubernetes-event" to distinguish Kubernetes events from other signal types.
// Used for metrics, logging, and signal classification.
func (a *KubernetesEventAdapter) GetSourceType() string {
	return "kubernetes-event"  // ✅ One event (singular)
}
```

### Data Flow

**Prometheus Alert Processing**:
```
1. AlertManager sends webhook → Prometheus Adapter
2. Adapter.GetSourceService() → "prometheus"
3. Adapter.GetSourceType() → "prometheus-alert"
4. NormalizedSignal created:
   - SignalSource: "prometheus"
   - SignalType: "prometheus-alert"
5. LLM receives signal → uses Prometheus queries for investigation
6. Metrics recorded: signals_received_total{signal_type="prometheus-alert"} 1
```

**Kubernetes Event Processing**:
```
1. K8s Event webhook → Kubernetes Event Adapter
2. Adapter.GetSourceService() → "kubernetes-events"
3. Adapter.GetSourceType() → "kubernetes-event"
4. NormalizedSignal created:
   - SignalSource: "kubernetes-events"
   - SignalType: "kubernetes-event"
5. LLM receives signal → uses kubectl commands for investigation
6. Metrics recorded: signals_received_total{signal_type="kubernetes-event"} 1
```

---

## Consequences

### Positive
- ✅ **LLM Tool Selection**: Clear mapping from SignalSource to investigation tools
- ✅ **K8s API Consistency**: Matches official Kubernetes Events API naming
- ✅ **Metrics Clarity**: SignalType counts individual signals (singular)
- ✅ **Extensibility**: Pattern works for all future adapters
- ✅ **Documentation**: Clear guidelines for adapter developers

### Negative
- ⚠️ **Developer Learning Curve**: Must check official system names - **Mitigation**: Documented in adapter development guide
- ⚠️ **Test Expectations**: Tests must use correct values - **Mitigation**: Reference documentation in test comments

### Neutral
- 🔄 **Existing Adapters**: Both Prometheus and K8s Event adapters already follow this pattern (no changes needed)
- 🔄 **Test Fix**: One test expectation corrected (`kubernetes-event` → `kubernetes-events` for SignalSource)

---

## Validation Results

### Consistency Matrix

| Adapter | SignalSource | SignalType | LLM Tools | Consistent? |
|---------|--------------|------------|-----------|-------------|
| **Prometheus** | `prometheus` | `prometheus-alert` | Prometheus queries | ✅ YES |
| **Kubernetes Event** | `kubernetes-events` | `kubernetes-event` | kubectl events | ✅ YES |
| **Future: Grafana** | `grafana` | `grafana-alert` | Grafana queries | ✅ YES |
| **Future: Datadog** | `datadog` | `datadog-alert` | Datadog queries | ✅ YES |

### Code Evidence

**Test Fix Applied**: `test/integration/gateway/adapter_interaction_test.go`

```go
// Before (INCORRECT)
Expect(crd.Spec.SignalSource).To(Equal("kubernetes-event"))  // ❌ Wrong

// After (CORRECT)
Expect(crd.Spec.SignalSource).To(Equal("kubernetes-events")) // ✅ Correct
```

### Confidence Assessment Progression
- Initial assessment: 85% confidence (needed validation)
- After code review: 95% confidence (implementation correct)
- After test fix: 100% confidence (validated in production code and tests)

---

## Guidelines for Future Adapters

### Naming Checklist

When creating a new adapter, follow these steps:

1. **SignalSource (Monitoring System)**:
   - [ ] Check official system documentation for the system name
   - [ ] Use the name exactly as documented (respect singular/plural)
   - [ ] Examples: "prometheus", "kubernetes-events", "grafana", "datadog"

2. **SignalType (Event Type)**:
   - [ ] Use singular form: `{system}-{type}`
   - [ ] Examples: "prometheus-alert", "kubernetes-event", "grafana-alert"

3. **LLM Tool Mapping**:
   - [ ] Document which tools LLM should use for this SignalSource
   - [ ] Examples:
     - `"prometheus"` → Prometheus queries, PromQL
     - `"kubernetes-events"` → kubectl get events, kubectl describe event
     - `"grafana"` → Grafana API, dashboard queries

4. **Adapter Name**:
   - [ ] Use simple identifier (lowercase, no special chars)
   - [ ] Examples: "prometheus", "kubernetes-event", "grafana"

5. **HTTP Route**:
   - [ ] Use `/api/v1/signals/{adapter-name}`
   - [ ] Examples: `/api/v1/signals/prometheus`, `/api/v1/signals/kubernetes-event`

### Example: Grafana Adapter

```go
func (a *GrafanaAdapter) GetSourceService() string {
	return "grafana"  // ✅ System name (check Grafana docs)
}

func (a *GrafanaAdapter) GetSourceType() string {
	return "grafana-alert"  // ✅ Singular (one alert)
}

func (a *GrafanaAdapter) Name() string {
	return "grafana"  // ✅ Adapter identifier
}

func (a *GrafanaAdapter) GetRoute() string {
	return "/api/v1/signals/grafana"  // ✅ HTTP route
}
```

---

## Related Decisions

- **Builds On**: BR-GATEWAY-027 (Signal source for LLM tool selection)
- **References**:
  - `pkg/gateway/adapters/adapter.go` - Adapter interface definition
  - `pkg/gateway/adapters/prometheus_adapter.go` - Reference implementation
  - `pkg/gateway/adapters/kubernetes_event_adapter.go` - K8s Events implementation
- **Related Documentation**: `docs/architecture/ADAPTER_NAMING_CONSISTENCY.md` - Detailed triage analysis

---

## Review & Evolution

### When to Revisit
- If adding adapter for system with unconventional naming (e.g., all caps, special chars)
- If LLM tool selection becomes more sophisticated (multiple tools per source)
- If metrics requirements change (need different granularity)

### Success Metrics
- **Adapter Consistency**: 100% of adapters follow naming convention
- **LLM Tool Selection**: 100% accuracy in tool mapping
- **Metrics Clarity**: 0 confusion about signal_type values
- **Developer Onboarding**: <5 minutes to understand convention

---

**Last Updated**: 2025-11-21
**Maintained By**: Gateway Service Team
**Related Files**:
- `pkg/gateway/adapters/prometheus_adapter.go`
- `pkg/gateway/adapters/kubernetes_event_adapter.go`
- `test/integration/gateway/adapter_interaction_test.go`

