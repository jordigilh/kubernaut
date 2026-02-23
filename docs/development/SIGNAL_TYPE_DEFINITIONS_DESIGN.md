# Signal Type Definitions Design Document

**Date**: 2025-10-07
**Status**: Design Proposal (not implemented)
**Related**: [ADR-015: Alert to Signal Naming Migration](../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md)
**Phase**: Phase 1 - Core Type Definitions
**Estimated Effort**: 8-12 hours

---

## Revision: Issue #166 Type vs Name Semantics (2026-02)

**Type vs Name distinction**: Issue #166 established that:
- **SignalName** = semantic classification (e.g., `OOMKilled`, `HighCPULoad`) — the human-meaningful signal identifier used for workflow catalog matching
- **SignalType** (in RR.Spec) = generic value `"alert"` — all adapters use this; source identity comes from **SignalSource** (e.g., `"prometheus"`, `"kubernetes-events"`)
- **SourceSignalName** = pre-normalization signal type (e.g., `PredictedOOMKill`) — preserved for audit trail

If this design is implemented, align with these semantics: use `SignalName` for semantic classification, `SignalSource` for adapter identity, and `SignalType` only for the generic `"alert"` value.

---

## 📋 Purpose

This document defines the **Signal-prefixed types** that will replace Alert-prefixed types in Kubernaut's multi-signal processing architecture. These type definitions enable Kubernaut to handle Prometheus alerts, Kubernetes events, AWS CloudWatch alarms, and custom webhooks through a unified signal processing pipeline.

---

## 🎯 Design Goals

1. **Signal-Type Agnostic**: Types must handle ANY signal source without modification
2. **Polymorphic**: Support signal-specific data through type discrimination
3. **Backward Compatible**: Type aliases prevent breaking existing code during migration
4. **Observable**: Metrics must distinguish between signal types
5. **Extensible**: Adding new signal types requires minimal code changes

---

## 📦 Package Structure

### **New Package: `pkg/signal/`**

```
pkg/signal/
├── types.go              # Core Signal type and enums
├── service.go            # SignalProcessorService interface
├── context.go            # SignalContext for AI analysis
├── metrics.go            # SignalProcessingMetrics
├── history.go            # SignalHistoryResult
├── converters.go         # Legacy Alert→Signal converters (temporary)
└── signal_test.go        # Unit tests
```

### **Backward Compatibility: `pkg/alert/` (Updated)**

```
pkg/alert/
├── service.go            # Type alias: AlertService = signal.SignalProcessorService
└── DEPRECATED.md         # Migration guide for developers
```

---

## 🔧 Core Type Definitions

### **1. Signal Type** (pkg/signal/types.go)

```go
// pkg/signal/types.go
package signal

import (
    "encoding/json"
    "time"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SignalType represents the source of a signal
type SignalType string

const (
    SignalTypePrometheusAlert   SignalType = "prometheus-alert"
    SignalTypeKubernetesEvent   SignalType = "kubernetes-event"
    SignalTypeCloudWatchAlarm   SignalType = "cloudwatch-alarm"
    SignalTypeCustomWebhook     SignalType = "custom-webhook"
    SignalTypeUnknown           SignalType = "unknown"
)

// Signal represents a unified event that triggers remediation
// Can be a Prometheus alert, Kubernetes event, CloudWatch alarm, etc.
type Signal struct {
    // Universal fields (present for ALL signal types)
    Type         SignalType  `json:"type"`                   // Signal source type
    Source       string      `json:"source"`                 // Adapter name (e.g., "prometheus-adapter")
    Fingerprint  string      `json:"fingerprint"`            // Unique identifier for deduplication
    Name         string      `json:"name"`                   // Human-readable signal name
    Severity     string      `json:"severity"`               // "critical", "warning", "info"
    Namespace    string      `json:"namespace,omitempty"`    // Kubernetes namespace (if applicable)
    Timestamp    time.Time   `json:"timestamp"`              // Signal occurrence time
    ReceivedTime time.Time   `json:"received_time"`          // When Gateway received signal

    // Polymorphic signal-specific data
    // Unmarshal this based on Type field
    Data         json.RawMessage `json:"data"`

    // Metadata (optional, signal-specific enrichment)
    Metadata     map[string]string `json:"metadata,omitempty"`

    // Storm detection (from Gateway Service)
    IsStorm      bool   `json:"is_storm,omitempty"`
    StormType    string `json:"storm_type,omitempty"`    // "rate" or "pattern"
    StormWindow  string `json:"storm_window,omitempty"`  // e.g., "5m"
}

// PrometheusAlertData contains Prometheus-specific alert data
type PrometheusAlertData struct {
    AlertName        string            `json:"alert_name"`
    Labels           map[string]string `json:"labels"`
    Annotations      map[string]string `json:"annotations"`
    StartsAt         time.Time         `json:"starts_at"`
    EndsAt           time.Time         `json:"ends_at,omitempty"`
    GeneratorURL     string            `json:"generator_url"`
    AlertmanagerURL  string            `json:"alertmanager_url,omitempty"`
    GrafanaURL       string            `json:"grafana_url,omitempty"`
}

// KubernetesEventData contains Kubernetes Event-specific data
type KubernetesEventData struct {
    EventType        string            `json:"event_type"`    // "Warning", "Normal"
    Reason           string            `json:"reason"`
    Message          string            `json:"message"`
    InvolvedObject   ObjectReference   `json:"involved_object"`
    FirstTimestamp   metav1.Time       `json:"first_timestamp"`
    LastTimestamp    metav1.Time       `json:"last_timestamp"`
    Count            int32             `json:"count"`
    SourceComponent  string            `json:"source_component"`
    SourceHost       string            `json:"source_host,omitempty"`
}

// ObjectReference identifies a Kubernetes resource
type ObjectReference struct {
    Kind       string `json:"kind"`
    Namespace  string `json:"namespace,omitempty"`
    Name       string `json:"name"`
    UID        string `json:"uid,omitempty"`
    APIVersion string `json:"api_version,omitempty"`
}

// CloudWatchAlarmData contains AWS CloudWatch-specific alarm data
type CloudWatchAlarmData struct {
    AlarmName        string            `json:"alarm_name"`
    AlarmDescription string            `json:"alarm_description,omitempty"`
    AWSAccountID     string            `json:"aws_account_id"`
    Region           string            `json:"region"`
    NewStateValue    string            `json:"new_state_value"`  // "ALARM", "OK", "INSUFFICIENT_DATA"
    NewStateReason   string            `json:"new_state_reason"`
    StateChangeTime  time.Time         `json:"state_change_time"`
    MetricName       string            `json:"metric_name"`
    Namespace        string            `json:"namespace"`  // AWS namespace, not K8s
    Dimensions       map[string]string `json:"dimensions,omitempty"`
}

// CustomWebhookData contains custom webhook-specific data
type CustomWebhookData struct {
    WebhookSource    string                 `json:"webhook_source"`  // Custom source identifier
    Payload          map[string]interface{} `json:"payload"`
}

// UnmarshalSignalData unmarshals signal.Data into appropriate typed struct
func (s *Signal) UnmarshalSignalData() (interface{}, error) {
    switch s.Type {
    case SignalTypePrometheusAlert:
        var data PrometheusAlertData
        if err := json.Unmarshal(s.Data, &data); err != nil {
            return nil, err
        }
        return &data, nil

    case SignalTypeKubernetesEvent:
        var data KubernetesEventData
        if err := json.Unmarshal(s.Data, &data); err != nil {
            return nil, err
        }
        return &data, nil

    case SignalTypeCloudWatchAlarm:
        var data CloudWatchAlarmData
        if err := json.Unmarshal(s.Data, &data); err != nil {
            return nil, err
        }
        return &data, nil

    case SignalTypeCustomWebhook:
        var data CustomWebhookData
        if err := json.Unmarshal(s.Data, &data); err != nil {
            return nil, err
        }
        return &data, nil

    default:
        return nil, fmt.Errorf("unsupported signal type: %s", s.Type)
    }
}
```

---

### **2. SignalProcessorService Interface** (pkg/signal/service.go)

```go
// pkg/signal/service.go
package signal

import (
    "context"
    "time"
)

// SignalProcessorService defines operations for processing ANY signal type
// Replaces: AlertService, AlertProcessorService
type SignalProcessorService interface {
    // Core processing
    ProcessSignal(ctx context.Context, signal *Signal) (*ProcessResult, error)

    // Validation
    ValidateSignal(signal *Signal) (*ValidationResult, error)

    // Routing
    RouteSignal(ctx context.Context, signal *Signal) (*RoutingResult, error)

    // Enrichment
    EnrichSignal(ctx context.Context, signal *Signal) (*EnrichmentResult, error)

    // Persistence
    PersistSignal(ctx context.Context, signal *Signal) (*PersistenceResult, error)
    GetSignalHistory(namespace string, duration time.Duration) (*SignalHistoryResult, error)

    // Metrics and Health
    GetProcessingMetrics() (*SignalProcessingMetrics, error)
    Health() (*HealthStatus, error)
}

// ProcessResult contains the outcome of signal processing
type ProcessResult struct {
    SignalFingerprint string    `json:"signal_fingerprint"`
    Processed         bool      `json:"processed"`
    Routed            bool      `json:"routed"`
    Enriched          bool      `json:"enriched"`
    Persisted         bool      `json:"persisted"`
    ProcessingTime    time.Duration `json:"processing_time"`
    Errors            []string  `json:"errors,omitempty"`
}

// ValidationResult contains signal validation outcome
type ValidationResult struct {
    Valid  bool     `json:"valid"`
    Errors []string `json:"errors,omitempty"`
    Warnings []string `json:"warnings,omitempty"`
}

// RoutingResult contains signal routing outcome
type RoutingResult struct {
    Routed      bool   `json:"routed"`
    Destination string `json:"destination,omitempty"` // CRD name or queue
    RouteType   string `json:"route_type"`            // "immediate", "deferred", "dropped"
    Priority    string `json:"priority,omitempty"`    // P0, P1, P2
    Reason      string `json:"reason,omitempty"`
}

// EnrichmentResult contains signal enrichment outcome
type EnrichmentResult struct {
    Status              string                 `json:"enrichment_status"` // "success", "partial", "failed"
    AIAnalysis          *AIAnalysisResult      `json:"ai_analysis,omitempty"`
    Metadata            map[string]string      `json:"metadata,omitempty"`
    EnrichmentTimestamp time.Time              `json:"enrichment_timestamp"`
}

// AIAnalysisResult contains AI enrichment data
type AIAnalysisResult struct {
    Confidence float64           `json:"confidence"`
    Analysis   string            `json:"analysis"`
    Metadata   map[string]string `json:"metadata,omitempty"`
}

// PersistenceResult contains signal persistence outcome
type PersistenceResult struct {
    Persisted bool      `json:"persisted"`
    SignalID  string    `json:"signal_id,omitempty"`
    Timestamp time.Time `json:"timestamp"`
}

// HealthStatus contains service health information
type HealthStatus struct {
    Status        string           `json:"status"` // "healthy", "degraded", "unhealthy"
    Service       string           `json:"service"`
    AIIntegration bool             `json:"ai_integration"`
    Components    *ComponentHealth `json:"components"`
}

// ComponentHealth contains health of individual components
type ComponentHealth struct {
    Processor    bool `json:"processor"`
    Enricher     bool `json:"enricher"`
    Router       bool `json:"router"`
    Validator    bool `json:"validator"`
    Persister    bool `json:"persister"`
}
```

---

### **3. SignalContext for AI Analysis** (pkg/signal/context.go)

```go
// pkg/signal/context.go
package signal

import "time"

// SignalContext provides context for AI analysis of signals
// Replaces: AlertContext
type SignalContext struct {
    // Signal identification
    SignalType       SignalType  `json:"signal_type"`
    SignalSource     string      `json:"signal_source"`
    SignalFingerprint string     `json:"signal_fingerprint"`

    // Kubernetes context
    Namespace        string      `json:"namespace,omitempty"`
    Cluster          string      `json:"cluster,omitempty"`

    // Temporal context
    Timestamp        time.Time   `json:"timestamp"`
    ReceivedTime     time.Time   `json:"received_time"`

    // Signal-specific metadata
    Metadata         map[string]string `json:"metadata,omitempty"`

    // Enrichment data
    CorrelatedSignals []string   `json:"correlated_signals,omitempty"` // Other signal fingerprints
    HistoricalContext *HistoricalContext `json:"historical_context,omitempty"`
}

// HistoricalContext provides historical signal information
type HistoricalContext struct {
    PreviousOccurrences int       `json:"previous_occurrences"`
    LastOccurrence      time.Time `json:"last_occurrence,omitempty"`
    AverageFrequency    float64   `json:"average_frequency"` // occurrences per hour
}
```

---

### **4. SignalProcessingMetrics** (pkg/signal/metrics.go)

```go
// pkg/signal/metrics.go
package signal

import "time"

// SignalProcessingMetrics provides signal-type-aware metrics
// Replaces: AlertMetrics
type SignalProcessingMetrics struct {
    // Per-signal-type metrics
    SignalsIngested  map[SignalType]int     `json:"signals_ingested"`  // {"prometheus-alert": 150, "kubernetes-event": 300}
    SignalsValidated map[SignalType]int     `json:"signals_validated"`
    SignalsRouted    map[SignalType]int     `json:"signals_routed"`
    SignalsEnriched  map[SignalType]int     `json:"signals_enriched"`
    SignalsPersisted map[SignalType]int     `json:"signals_persisted"`

    // Per-signal-type rates
    ProcessingRate   map[SignalType]float64 `json:"processing_rate"`   // Signals per minute per type
    SuccessRate      map[SignalType]float64 `json:"success_rate"`      // % success per type

    // Aggregate metrics
    TotalSignals     int                    `json:"total_signals"`
    TotalErrors      int                    `json:"total_errors"`

    // Timestamp
    LastUpdated      time.Time              `json:"last_updated"`
}

// NewSignalProcessingMetrics creates initialized metrics
func NewSignalProcessingMetrics() *SignalProcessingMetrics {
    return &SignalProcessingMetrics{
        SignalsIngested:  make(map[SignalType]int),
        SignalsValidated: make(map[SignalType]int),
        SignalsRouted:    make(map[SignalType]int),
        SignalsEnriched:  make(map[SignalType]int),
        SignalsPersisted: make(map[SignalType]int),
        ProcessingRate:   make(map[SignalType]float64),
        SuccessRate:      make(map[SignalType]float64),
        LastUpdated:      time.Now(),
    }
}
```

---

### **5. SignalHistoryResult** (pkg/signal/history.go)

```go
// pkg/signal/history.go
package signal

import "time"

// SignalHistoryResult contains historical signals query result
// Replaces: AlertHistoryResult
type SignalHistoryResult struct {
    Signals      []*Signal   `json:"signals"`       // Polymorphic signal types
    TotalCount   int         `json:"total_count"`
    Namespace    string      `json:"namespace,omitempty"`
    SignalTypes  []SignalType `json:"signal_types"` // Types included in this result
    Duration     time.Duration `json:"duration"`
    RetrievedAt  time.Time   `json:"retrieved_at"`
}

// FilterByType returns signals of a specific type
func (h *SignalHistoryResult) FilterByType(signalType SignalType) []*Signal {
    var filtered []*Signal
    for _, signal := range h.Signals {
        if signal.Type == signalType {
            filtered = append(filtered, signal)
        }
    }
    return filtered
}

// CountByType returns count of signals per type
func (h *SignalHistoryResult) CountByType() map[SignalType]int {
    counts := make(map[SignalType]int)
    for _, signal := range h.Signals {
        counts[signal.Type]++
    }
    return counts
}
```

---

## 🔄 Backward Compatibility Layer

### **Type Aliases** (pkg/alert/service.go - TEMPORARY)

```go
// pkg/alert/service.go
package alert

import (
    "github.com/jordigilh/kubernaut/pkg/signal"
)

// DEPRECATED: Use signal.SignalProcessorService instead
// This alias will be removed in V2
// See: docs/architecture/decisions/ADR-015-alert-to-signal-naming-migration.md
type AlertService = signal.SignalProcessorService

// DEPRECATED: Use signal.SignalProcessorService instead
type AlertProcessorService = signal.SignalProcessorService

// DEPRECATED: Use signal.ProcessResult instead
type ProcessResult = signal.ProcessResult

// DEPRECATED: Use signal.ValidationResult instead
type ValidationResult = signal.ValidationResult

// DEPRECATED: Use signal.RoutingResult instead
type RoutingResult = signal.RoutingResult

// DEPRECATED: Use signal.EnrichmentResult instead
type EnrichmentResult = signal.EnrichmentResult

// DEPRECATED: Use signal.PersistenceResult instead
type PersistenceResult = signal.PersistenceResult

// DEPRECATED: Use signal.SignalHistoryResult instead
type AlertHistoryResult = signal.SignalHistoryResult

// DEPRECATED: Use signal.SignalProcessingMetrics instead
type AlertMetrics = signal.SignalProcessingMetrics

// DEPRECATED: Use signal.HealthStatus instead
type HealthStatus = signal.HealthStatus
```

---

## 📋 Implementation Checklist

### **Phase 1A: Create Signal Package** (4 hours)
- [ ] Create `pkg/signal/` directory
- [ ] Implement `types.go` with Signal struct and enums
- [ ] Implement `service.go` with SignalProcessorService interface
- [ ] Implement `context.go` with SignalContext
- [ ] Implement `metrics.go` with SignalProcessingMetrics
- [ ] Implement `history.go` with SignalHistoryResult
- [ ] Add unit tests for all types

### **Phase 1B: Add Backward Compatibility** (2 hours)
- [ ] Update `pkg/alert/service.go` with type aliases
- [ ] Add deprecation comments to all aliases
- [ ] Create `pkg/alert/DEPRECATED.md` migration guide
- [ ] Validate builds pass with backward compatibility

### **Phase 1C: Add Converters (Temporary)** (2 hours)
- [ ] Implement `converters.go` with Alert→Signal conversion
- [ ] Add `signalFromAlert()` helper function
- [ ] Add `alertFromSignal()` helper function (for legacy callers)
- [ ] Unit tests for converters

### **Phase 1D: Documentation** (2 hours)
- [ ] Update API documentation
- [ ] Create examples in `docs/development/examples/`
- [ ] Update developer onboarding guide
- [ ] Add to CHANGELOG

---

## ✅ Validation Criteria

Phase 1 is **complete** when:
- ✅ All Signal-prefixed types compile without errors
- ✅ Unit tests pass for all new types (70%+ coverage)
- ✅ Backward-compatible type aliases work (existing code compiles)
- ✅ Documentation updated with examples
- ✅ No breaking changes in existing code

---

## 📊 Success Metrics

| Metric | Target |
|--------|--------|
| **Build Success** | 100% on all platforms |
| **Test Coverage** | 70%+ for new `pkg/signal/` package |
| **Breaking Changes** | 0 (backward compatibility maintained) |
| **Documentation** | 100% of public APIs documented |

---

## 🔗 Next Steps

After Phase 1 completion:
1. **Phase 2**: Migrate service implementations to use `SignalProcessorService`
2. **Phase 3**: Migrate all tests to use Signal types
3. **Phase 4**: Update documentation
4. **Phase 5**: Remove type aliases (V2)

---

**Author**: Development Team
**Date**: 2025-10-07
**Status**: Ready for Implementation
**Related ADR**: [ADR-015](../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md)
