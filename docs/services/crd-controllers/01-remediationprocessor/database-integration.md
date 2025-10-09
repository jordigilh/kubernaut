## Database Integration for Audit & Tracking

### Dual Audit System

Following the [MULTI_CRD_RECONCILIATION_ARCHITECTURE.md](../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md#crd-lifecycle-management-and-cleanup) dual audit approach:

1. **CRDs**: Real-time execution state + 24-hour review window
2. **Database**: Long-term compliance + post-mortem analysis

### Audit Data Persistence

**Database Service**: Data Storage Service (Port 8085)
**Purpose**: Persist alert processing audit trail before CRD cleanup

```go
package controller

import (
    "github.com/jordigilh/kubernaut/pkg/storage"
)

type RemediationProcessingReconciler struct {
    client.Client
    Scheme         *runtime.Scheme
    ContextService ContextService
    Classifier     *EnvironmentClassifier
    AuditStorage   storage.AuditStorageClient  // Database client for audit trail
}

// After each phase completion, persist audit data
func (r *RemediationProcessingReconciler) reconcileRoutingWithAudit(ctx context.Context, ap *processingv1.RemediationProcessing) (ctrl.Result, error) {
    // ... routing logic ...

    // Persist complete alert processing audit trail
    auditRecord := &storage.RemediationProcessingAudit{
        RemediationID:    ap.Spec.RemediationRequestRef.Name,
        AlertFingerprint: ap.Spec.Signal.Fingerprint,
        ProcessingPhases: []storage.ProcessingPhase{
            {
                Phase:     "enriching",
                StartTime: ap.CreationTimestamp.Time,
                EndTime:   ap.Status.EnrichmentResults.CompletedAt,
                Duration:  ap.Status.EnrichmentResults.Duration,
                // ✅ TYPE SAFE - Uses structured phase results
                EnrichmentPhaseResults: &storage.EnrichmentPhaseResults{
                    ContextQuality:   ap.Status.EnrichmentResults.EnrichmentQuality,
                    ContextSizeBytes: calculateContextSize(ap.Status.EnrichmentResults),
                    ContextSources:   len(ap.Spec.EnrichmentConfig.ContextSources),
                    DegradedMode:     ap.Status.DegradedMode,
                },
            },
            {
                Phase:     "classifying",
                StartTime: ap.Status.ClassificationStartTime,
                EndTime:   ap.Status.ClassificationEndTime,
                Duration:  ap.Status.ClassificationDuration,
                // ✅ TYPE SAFE - Uses structured phase results
                ClassificationPhaseResults: &storage.ClassificationPhaseResults{
                    Environment:          ap.Status.EnvironmentClassification.Environment,
                    Confidence:           ap.Status.EnvironmentClassification.Confidence,
                    BusinessPriority:     ap.Status.EnvironmentClassification.BusinessPriority,
                    ClassificationMethod: "namespace-label", // or from status
                },
            },
        },
        CompletedAt: time.Now(),
        Status:      "completed",
    }

    if err := r.AuditStorage.StoreRemediationProcessingAudit(ctx, auditRecord); err != nil {
        r.Log.Error(err, "Failed to store alert processing audit", "fingerprint", ap.Spec.Signal.Fingerprint)
        ErrorsTotal.WithLabelValues("audit_storage_failed", "routing").Inc()
        // Don't fail reconciliation, but log for investigation
    }

    // ... continue with routing ...
}
```

### Audit Data Schema

```go
package storage

type RemediationProcessingAudit struct {
    ID               string            `json:"id" db:"id"`
    RemediationID    string            `json:"remediation_id" db:"remediation_id"`
    AlertFingerprint string            `json:"alert_fingerprint" db:"alert_fingerprint"`
    ProcessingPhases []ProcessingPhase `json:"processing_phases" db:"processing_phases"`

    // Enrichment results
    EnrichmentQuality float64                 `json:"enrichment_quality" db:"enrichment_quality"`
    EnrichmentSources []string                `json:"enrichment_sources" db:"enrichment_sources"`
    ContextSize       int                     `json:"context_size_bytes" db:"context_size_bytes"`

    // Classification results
    Environment      string  `json:"environment" db:"environment"`
    Confidence       float64 `json:"confidence" db:"confidence"`
    BusinessPriority string  `json:"business_priority" db:"business_priority"`
    SLARequirement   string  `json:"sla_requirement" db:"sla_requirement"`

    // Routing decision
    RoutedToService string    `json:"routed_to_service" db:"routed_to_service"`
    RoutingPriority int       `json:"routing_priority" db:"routing_priority"`

    // Metadata
    CompletedAt time.Time `json:"completed_at" db:"completed_at"`
    Status      string    `json:"status" db:"status"`
    ErrorMessage string   `json:"error_message,omitempty" db:"error_message"`
}

type ProcessingPhase struct {
    Phase     string        `json:"phase" db:"phase"`
    StartTime time.Time     `json:"start_time" db:"start_time"`
    EndTime   time.Time     `json:"end_time" db:"end_time"`
    Duration  time.Duration `json:"duration" db:"duration"`

    // Phase-specific results (only one will be populated based on phase)
    // ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
    EnrichmentPhaseResults    *EnrichmentPhaseResults    `json:"enrichmentResults,omitempty" db:"enrichment_results"`
    ClassificationPhaseResults *ClassificationPhaseResults `json:"classificationResults,omitempty" db:"classification_results"`
    RoutingPhaseResults       *RoutingPhaseResults       `json:"routingResults,omitempty" db:"routing_results"`
}

type EnrichmentPhaseResults struct {
    ContextQuality   float64 `json:"contextQuality" db:"context_quality"`
    ContextSizeBytes int     `json:"contextSizeBytes" db:"context_size_bytes"`
    ContextSources   int     `json:"contextSources" db:"context_sources"` // Number of sources used
    DegradedMode     bool    `json:"degradedMode" db:"degraded_mode"`
}

type ClassificationPhaseResults struct {
    Environment          string  `json:"environment" db:"environment"`
    Confidence           float64 `json:"confidence" db:"confidence"`
    BusinessPriority     string  `json:"businessPriority" db:"business_priority"`
    ClassificationMethod string  `json:"classificationMethod" db:"classification_method"` // "namespace-label", "pattern", "fallback"
}

type RoutingPhaseResults struct {
    RoutedToService string `json:"routedToService" db:"routed_to_service"`
    RoutingPriority int    `json:"routingPriority" db:"routing_priority"`
    RoutingKey      string `json:"routingKey" db:"routing_key"`
}
```

### Audit Use Cases

**Post-Mortem Analysis**:
```sql
-- Find all alert processing records for a specific alert
SELECT * FROM alert_processing_audit
WHERE alert_fingerprint = 'mem-pressure-prod-123'
ORDER BY completed_at DESC;

-- Classification accuracy analysis
SELECT environment,
       AVG(confidence) as avg_confidence,
       COUNT(*) as total_alerts
FROM alert_processing_audit
WHERE completed_at > NOW() - INTERVAL '7 days'
GROUP BY environment;
```

**Performance Tracking**:
```sql
-- Find slow enrichment operations
SELECT remediation_id, alert_fingerprint,
       (processing_phases->0->>'duration')::interval as enrichment_duration
FROM alert_processing_audit
WHERE (processing_phases->0->>'duration')::interval > INTERVAL '2 seconds'
ORDER BY enrichment_duration DESC;
```

**Compliance Reporting**:
- Retain all alert processing decisions for regulatory compliance
- Track classification accuracy over time
- Audit trail for all routing decisions

### Storage Service Integration

**Dependencies**:
- Data Storage Service (Port 8085)
- PostgreSQL database with `alert_processing_audit` table
- Optional: Vector DB for enrichment context storage

**HTTP Client**:
```go
// Simple HTTP POST to Data Storage Service
func (c *AuditStorageClient) StoreRemediationProcessingAudit(ctx context.Context, audit *RemediationProcessingAudit) error {
    url := fmt.Sprintf("%s/api/v1/audit/alert-processing", c.storageServiceURL)

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

### Audit Metrics

Add metrics to track audit storage operations:

```go
var (
    // Counter: Audit storage attempts
    AuditStorageAttemptsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_alertprocessor_audit_storage_attempts_total",
        Help: "Total audit storage attempts",
    }, []string{"status"}) // success, error

    // Histogram: Audit storage duration
    AuditStorageDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "kubernaut_alertprocessor_audit_storage_duration_seconds",
        Help:    "Duration of audit storage operations",
        Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0},
    })
)
```

