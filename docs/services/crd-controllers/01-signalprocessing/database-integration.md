## Database Integration for Audit & Tracking

> **📋 Changelog**
> | Version | Date | Changes | Reference |
> |---------|------|---------|-----------|
> | v1.2 | 2025-11-28 | Added async buffered audit (ADR-038), API group fix | [ADR-038](../../../architecture/decisions/ADR-038-async-buffered-audit-ingestion.md) |
> | v1.1 | 2025-11-27 | Service rename: RemediationProcessing → SignalProcessing | [DD-SIGNAL-PROCESSING-001](../../../architecture/decisions/DD-SIGNAL-PROCESSING-001-service-rename.md) |
> | v1.1 | 2025-11-27 | Data access via Data Storage Service REST API only | [ADR-032](../../../architecture/decisions/ADR-032-data-access-layer-isolation.md) |
> | v1.0 | 2025-01-15 | Initial database integration | - |

### Dual Audit System

Following the [KUBERNAUT_CRD_ARCHITECTURE.md](../../../architecture/KUBERNAUT_CRD_ARCHITECTURE.md#crd-lifecycle-management-and-cleanup) dual audit approach:

1. **CRDs**: Real-time execution state + 24-hour review window
2. **Database**: Long-term compliance + post-mortem analysis (via Data Storage Service)

### Data Access Layer Isolation (ADR-032)

> **📋 ADR-032: Data Access Layer Isolation**
> Signal Processing does NOT connect directly to PostgreSQL.
> All audit persistence uses Data Storage Service REST API.
> **See**: [ADR-032](../../../architecture/decisions/ADR-032-data-access-layer-isolation.md)

### Audit Data Persistence

**Service**: Data Storage Service (Port 8085)
**Purpose**: Persist signal processing audit trail before CRD cleanup

```go
package controller

import (
    kubernautv1alpha1 "github.com/jordigilh/kubernaut/api/kubernaut.io/v1alpha1"
)

type SignalProcessingReconciler struct {
    client.Client
    Scheme            *runtime.Scheme
    EnrichmentService EnrichmentService
    Classifier        *EnvironmentClassifier
    Categorizer       *PriorityCategorizer
    DataStorageClient DataStorageClient  // REST API client for audit trail (ADR-032)
}

// After processing completion, persist audit data via Data Storage Service
// Per ADR-038: Use asynchronous buffered audit writes (fire-and-forget pattern)
func (r *SignalProcessingReconciler) persistAuditTrail(ctx context.Context, sp *kubernautv1alpha1.SignalProcessing) error {
    auditRecord := &SignalProcessingAudit{
        RemediationID:    sp.Spec.RemediationRequestRef.Name,
        SignalFingerprint: sp.Spec.Signal.Fingerprint,
        ProcessingPhases: []ProcessingPhase{
            {
                Phase:     "enriching",
                StartTime: sp.CreationTimestamp.Time,
                EndTime:   sp.Status.EnrichmentResults.CompletedAt,
                Duration:  sp.Status.EnrichmentResults.Duration,
                // ✅ TYPE SAFE - Uses structured phase results
                EnrichmentPhaseResults: &EnrichmentPhaseResults{
                    ContextQuality:   sp.Status.EnrichmentResults.EnrichmentQuality,
                    ContextSizeBytes: calculateContextSize(sp.Status.EnrichmentResults),
                    ContextSources:   len(sp.Spec.EnrichmentConfig.ContextSources),
                    DegradedMode:     sp.Status.EnrichmentResults.EnrichmentQuality < 0.8,
                },
            },
            {
                Phase:     "classifying",
                StartTime: sp.Status.ClassificationStartTime,
                EndTime:   sp.Status.ClassificationEndTime,
                Duration:  sp.Status.ClassificationDuration,
                // ✅ TYPE SAFE - Uses structured phase results
                ClassificationPhaseResults: &ClassificationPhaseResults{
                    Environment:          sp.Status.EnvironmentClassification.Environment,
                    Confidence:           sp.Status.EnvironmentClassification.Confidence,
                    BusinessCriticality:  sp.Status.EnvironmentClassification.BusinessCriticality,
                    ClassificationMethod: "namespace-label",
                },
            },
            {
                Phase:     "categorizing",
                StartTime: sp.Status.CategorizationStartTime,
                EndTime:   sp.Status.CategorizationEndTime,
                Duration:  sp.Status.CategorizationDuration,
                // ✅ TYPE SAFE - Uses structured phase results (DD-CATEGORIZATION-001)
                CategorizationPhaseResults: &CategorizationPhaseResults{
                    Priority:       sp.Status.Categorization.Priority,
                    PriorityScore:  sp.Status.Categorization.PriorityScore,
                    Source:         sp.Status.Categorization.CategorizationSource,
                },
            },
        },
        CompletedAt: time.Now(),
        Status:      "completed",
    }

    // Send to Data Storage Service via REST API (ADR-032)
    return r.DataStorageClient.CreateAuditRecord(ctx, auditRecord)
}
```

### Audit Data Schema

```go
package storage

type SignalProcessingAudit struct {
    ID                string            `json:"id"`
    RemediationID     string            `json:"remediation_id"`
    SignalFingerprint string            `json:"signal_fingerprint"`
    ProcessingPhases  []ProcessingPhase `json:"processing_phases"`

    // Enrichment results
    EnrichmentQuality float64  `json:"enrichment_quality"`
    EnrichmentSources []string `json:"enrichment_sources"`
    ContextSize       int      `json:"context_size_bytes"`

    // Classification results
    Environment         string  `json:"environment"`
    Confidence          float64 `json:"confidence"`
    BusinessCriticality string  `json:"business_criticality"`
    SLARequirement      string  `json:"sla_requirement"`

    // Categorization results (DD-CATEGORIZATION-001)
    Priority      string `json:"priority"`
    PriorityScore int    `json:"priority_score"`

    // Routing decision
    RoutedToService string `json:"routed_to_service"`
    RoutingPriority int    `json:"routing_priority"`

    // Metadata
    CompletedAt  time.Time `json:"completed_at"`
    Status       string    `json:"status"`
    ErrorMessage string    `json:"error_message,omitempty"`
}

type ProcessingPhase struct {
    Phase     string        `json:"phase"`
    StartTime time.Time     `json:"start_time"`
    EndTime   time.Time     `json:"end_time"`
    Duration  time.Duration `json:"duration"`

    // Phase-specific results (only one will be populated based on phase)
    // ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
    EnrichmentPhaseResults     *EnrichmentPhaseResults     `json:"enrichmentResults,omitempty"`
    ClassificationPhaseResults *ClassificationPhaseResults `json:"classificationResults,omitempty"`
    CategorizationPhaseResults *CategorizationPhaseResults `json:"categorizationResults,omitempty"`
}

type EnrichmentPhaseResults struct {
    ContextQuality   float64 `json:"contextQuality"`
    ContextSizeBytes int     `json:"contextSizeBytes"`
    ContextSources   int     `json:"contextSources"`
    DegradedMode     bool    `json:"degradedMode"`
}

type ClassificationPhaseResults struct {
    Environment          string  `json:"environment"`
    Confidence           float64 `json:"confidence"`
    BusinessCriticality  string  `json:"businessCriticality"`
    ClassificationMethod string  `json:"classificationMethod"` // "namespace-label", "pattern", "fallback"
}

type CategorizationPhaseResults struct {
    Priority      string `json:"priority"`
    PriorityScore int    `json:"priorityScore"`
    Source        string `json:"source"` // "enriched_context", "fallback_labels", "default"
}
```

### Data Storage Service Integration

**Endpoint**: `POST /api/v1/audit/signal-processing`

**HTTP Client**:
```go
// DataStorageClient interface for audit trail persistence (ADR-032)
type DataStorageClient interface {
    CreateAuditRecord(ctx context.Context, audit *SignalProcessingAudit) error
}

// Implementation using HTTP client
type dataStorageClientImpl struct {
    httpClient *http.Client
    baseURL    string
}

func (c *dataStorageClientImpl) CreateAuditRecord(ctx context.Context, audit *SignalProcessingAudit) error {
    url := fmt.Sprintf("%s/api/v1/audit/signal-processing", c.baseURL)

    body, err := json.Marshal(audit)
    if err != nil {
        return fmt.Errorf("failed to marshal audit: %w", err)
    }

    req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
    if err != nil {
        return err
    }
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("failed to store audit: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusCreated {
        return fmt.Errorf("audit storage failed with status: %d", resp.StatusCode)
    }

    return nil
}
```

### Audit Use Cases

**Post-Mortem Analysis** (via Data Storage Service query API):
```sql
-- Find all signal processing records for a specific signal
SELECT * FROM signal_processing_audit
WHERE signal_fingerprint = 'mem-pressure-prod-123'
ORDER BY completed_at DESC;

-- Classification accuracy analysis
SELECT environment,
       AVG(confidence) as avg_confidence,
       COUNT(*) as total_signals
FROM signal_processing_audit
WHERE completed_at > NOW() - INTERVAL '7 days'
GROUP BY environment;

-- Priority distribution analysis (DD-CATEGORIZATION-001)
SELECT priority,
       COUNT(*) as count,
       AVG(priority_score) as avg_score
FROM signal_processing_audit
WHERE completed_at > NOW() - INTERVAL '7 days'
GROUP BY priority;
```

**Performance Tracking**:
```sql
-- Find slow enrichment operations
SELECT remediation_id, signal_fingerprint,
       (processing_phases->0->>'duration')::interval as enrichment_duration
FROM signal_processing_audit
WHERE (processing_phases->0->>'duration')::interval > INTERVAL '2 seconds'
ORDER BY enrichment_duration DESC;
```

**Compliance Reporting**:
- Retain all signal processing decisions for regulatory compliance
- Track classification and categorization accuracy over time
- Audit trail for all routing decisions

### Audit Metrics

Add metrics to track audit storage operations:

```go
var (
    // Counter: Audit storage attempts
    AuditStorageAttemptsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_signal_processing_audit_storage_attempts_total",
        Help: "Total audit storage attempts",
    }, []string{"status"}) // success, error

    // Histogram: Audit storage duration
    AuditStorageDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "kubernaut_signal_processing_audit_storage_duration_seconds",
        Help:    "Duration of audit storage operations",
        Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0},
    })
)
```
