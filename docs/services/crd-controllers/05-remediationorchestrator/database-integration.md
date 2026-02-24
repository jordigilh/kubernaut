## Database Integration

### Audit Table Schema

**PostgreSQL Table**: `remediation_audit`

```sql
CREATE TABLE remediation_audit (
    id SERIAL PRIMARY KEY,
    alert_fingerprint VARCHAR(64) NOT NULL,
    remediation_name VARCHAR(255) NOT NULL,
    overall_phase VARCHAR(50) NOT NULL,
    start_time TIMESTAMP NOT NULL,
    completion_time TIMESTAMP,

    -- Service CRD references
    alert_processing_name VARCHAR(255),
    ai_analysis_name VARCHAR(255),
    workflow_execution_name VARCHAR(255),
    kubernetes_execution_name VARCHAR(255),  -- DEPRECATED - ADR-025

    -- Service CRD statuses (JSONB for flexibility)
    service_crd_statuses JSONB,

    -- Timeout/Failure tracking
    timeout_phase VARCHAR(50),
    timeout_time TIMESTAMP,
    failure_phase VARCHAR(50),
    failure_reason TEXT,

    -- Duplicate tracking
    duplicate_count INT DEFAULT 0,
    last_duplicate_time TIMESTAMP,

    -- Retention tracking
    retention_expiry_time TIMESTAMP,
    deleted_at TIMESTAMP,

    -- Indexing
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    INDEX idx_alert_fingerprint (alert_fingerprint),
    INDEX idx_remediation_name (remediation_name),
    INDEX idx_overall_phase (overall_phase),
    INDEX idx_retention_expiry (retention_expiry_time)
);
```

---

## Go Types for Audit Records

### Remediation Orchestration Audit

```go
package storage

import (
    "encoding/json"
    "time"
)

type RemediationOrchestrationAudit struct {
    ID               int64     `json:"id" db:"id"`
    AlertFingerprint string    `json:"alert_fingerprint" db:"alert_fingerprint"`
    RemediationName  string    `json:"remediation_name" db:"remediation_name"`
    OverallPhase     string    `json:"overall_phase" db:"overall_phase"`
    StartTime        time.Time `json:"start_time" db:"start_time"`
    CompletionTime   *time.Time `json:"completion_time,omitempty" db:"completion_time"`

    // Downstream CRD references
    RemediationProcessingName string `json:"remediation_processing_name,omitempty" db:"remediation_processing_name"`
    AIAnalysisName            string `json:"ai_analysis_name,omitempty" db:"ai_analysis_name"`
    WorkflowExecutionName     string `json:"workflow_execution_name,omitempty" db:"workflow_execution_name"`

    // CRD statuses snapshot (JSONB)
    ServiceCRDStatuses json.RawMessage `json:"service_crd_statuses,omitempty" db:"service_crd_statuses"`

    // Timeout/Failure tracking
    TimeoutPhase   string     `json:"timeout_phase,omitempty" db:"timeout_phase"`
    TimeoutTime    *time.Time `json:"timeout_time,omitempty" db:"timeout_time"`
    FailurePhase   string     `json:"failure_phase,omitempty" db:"failure_phase"`
    FailureReason  string     `json:"failure_reason,omitempty" db:"failure_reason"`

    // Duplicate tracking
    DuplicateCount      int        `json:"duplicate_count" db:"duplicate_count"`
    LastDuplicateTime   *time.Time `json:"last_duplicate_time,omitempty" db:"last_duplicate_time"`

    // Retention
    RetentionExpiryTime *time.Time `json:"retention_expiry_time,omitempty" db:"retention_expiry_time"`
    DeletedAt           *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`

    // Timestamps
    CreatedAt time.Time `json:"created_at" db:"created_at"`
    UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// ServiceCRDStatuses is the JSONB structure
type ServiceCRDStatuses struct {
    RemediationProcessing *CRDStatusSnapshot `json:"remediationProcessing,omitempty"`
    AIAnalysis            *CRDStatusSnapshot `json:"aiAnalysis,omitempty"`
    WorkflowExecution     *CRDStatusSnapshot `json:"workflowExecution,omitempty"`
}

type CRDStatusSnapshot struct {
    Phase       string    `json:"phase"`
    StartTime   time.Time `json:"startTime"`
    CompletionTime *time.Time `json:"completionTime,omitempty"`
    Success     bool      `json:"success"`
    Error       string    `json:"error,omitempty"`
}
```

---

## Controller Integration

### Audit Publishing in Reconcile Loop

```go
// In RemediationRequestReconciler
type RemediationRequestReconciler struct {
    client.Client
    Scheme       *runtime.Scheme
    AuditStorage storage.AuditStorageClient
}

// Publish audit record on phase transitions
func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := logf.FromContext(ctx)

    var remediation remediationv1alpha1.RemediationRequest
    if err := r.Get(ctx, req.NamespacedName, &remediation); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Track phase transitions
    previousPhase := remediation.Status.Phase

    // ... orchestration logic (create downstream CRDs, watch status) ...

    // Publish audit on phase change
    if remediation.Status.Phase != previousPhase {
        if err := r.publishAuditRecord(ctx, &remediation); err != nil {
            log.Error(err, "Failed to publish audit record",
                "remediation", remediation.Name,
                "phase", remediation.Status.Phase)
            // Don't fail reconciliation on audit failure
            AuditPublishFailuresTotal.Inc()
        }
    }

    return ctrl.Result{}, nil
}

// Publish orchestration audit record
func (r *RemediationRequestReconciler) publishAuditRecord(
    ctx context.Context,
    remediation *remediationv1alpha1.RemediationRequest,
) error {
    // Collect downstream CRD statuses
    crdStatuses, err := r.collectCRDStatuses(ctx, remediation)
    if err != nil {
        return fmt.Errorf("failed to collect CRD statuses: %w", err)
    }

    crdStatusesJSON, err := json.Marshal(crdStatuses)
    if err != nil {
        return fmt.Errorf("failed to marshal CRD statuses: %w", err)
    }

    audit := &storage.RemediationOrchestrationAudit{
        AlertFingerprint:          remediation.Spec.SignalFingerprint,
        RemediationName:           remediation.Name,
        OverallPhase:              remediation.Status.Phase,
        StartTime:                 remediation.CreationTimestamp.Time,
        RemediationProcessingName: getCRDName(remediation.Status.RemediationProcessingRef),
        AIAnalysisName:            getCRDName(remediation.Status.AIAnalysisRef),
        WorkflowExecutionName:     getCRDName(remediation.Status.WorkflowExecutionRef),
        ServiceCRDStatuses:        crdStatusesJSON,
    }

    // Set completion time if completed
    if remediation.Status.Phase == "completed" || remediation.Status.Phase == "failed" {
        now := time.Now()
        audit.CompletionTime = &now
    }

    // Set failure info if failed
    if remediation.Status.Phase == "failed" {
        audit.FailurePhase = remediation.Status.FailedPhase
        audit.FailureReason = remediation.Status.ErrorMessage
    }

    return r.AuditStorage.StoreRemediationOrchestrationAudit(ctx, audit)
}

// Collect downstream CRD statuses
func (r *RemediationRequestReconciler) collectCRDStatuses(
    ctx context.Context,
    remediation *remediationv1alpha1.RemediationRequest,
) (*storage.ServiceCRDStatuses, error) {
    statuses := &storage.ServiceCRDStatuses{}

    // RemediationProcessing status
    if remediation.Status.RemediationProcessingRef != nil {
        var processing remediationprocessingv1alpha1.RemediationProcessing
        if err := r.Get(ctx, client.ObjectKey{
            Name:      remediation.Status.RemediationProcessingRef.Name,
            Namespace: remediation.Namespace,
        }, &processing); err == nil {
            statuses.RemediationProcessing = &storage.CRDStatusSnapshot{
                Phase:     string(processing.Status.Phase),
                StartTime: processing.CreationTimestamp.Time,
                Success:   processing.Status.Phase == "completed",
            }
            if processing.Status.CompletedAt != nil {
                statuses.RemediationProcessing.CompletionTime = &processing.Status.CompletedAt.Time
            }
        }
    }

    // AIAnalysis status
    if remediation.Status.AIAnalysisRef != nil {
        var analysis aianalysisv1alpha1.AIAnalysis
        if err := r.Get(ctx, client.ObjectKey{
            Name:      remediation.Status.AIAnalysisRef.Name,
            Namespace: remediation.Namespace,
        }, &analysis); err == nil {
            statuses.AIAnalysis = &storage.CRDStatusSnapshot{
                Phase:     string(analysis.Status.Phase),
                StartTime: analysis.CreationTimestamp.Time,
                Success:   analysis.Status.Phase == "completed",
            }
            if analysis.Status.CompletedAt != nil {
                statuses.AIAnalysis.CompletionTime = &analysis.Status.CompletedAt.Time
            }
        }
    }

    // WorkflowExecution status
    if remediation.Status.WorkflowExecutionRef != nil {
        var workflow workflowv1alpha1.WorkflowExecution
        if err := r.Get(ctx, client.ObjectKey{
            Name:      remediation.Status.WorkflowExecutionRef.Name,
            Namespace: remediation.Namespace,
        }, &workflow); err == nil {
            statuses.WorkflowExecution = &storage.CRDStatusSnapshot{
                Phase:     string(workflow.Status.Phase),
                StartTime: workflow.CreationTimestamp.Time,
                Success:   workflow.Status.Phase == "completed",
            }
            if workflow.Status.CompletedAt != nil {
                statuses.WorkflowExecution.CompletionTime = &workflow.Status.CompletedAt.Time
            }
        }
    }

    return statuses, nil
}

func getCRDName(ref *remediationv1alpha1.CRDReference) string {
    if ref == nil {
        return ""
    }
    return ref.Name
}
```

---

## HTTP Client Implementation

### Storage Service Client

```go
package storage

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type AuditStorageClient struct {
    storageServiceURL string
    httpClient        *http.Client
}

func NewAuditStorageClient(storageServiceURL string) *AuditStorageClient {
    return &AuditStorageClient{
        storageServiceURL: storageServiceURL,
        httpClient: &http.Client{
            Timeout: 5 * time.Second,
        },
    }
}

func (c *AuditStorageClient) StoreRemediationOrchestrationAudit(
    ctx context.Context,
    audit *RemediationOrchestrationAudit,
) error {
    url := fmt.Sprintf("%s/api/v1/audit/remediation-orchestration", c.storageServiceURL)

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

---

## Audit Use Cases

### Post-Mortem Analysis

**Find all remediation attempts for a specific alert:**
```sql
SELECT
    remediation_name,
    overall_phase,
    start_time,
    completion_time,
    EXTRACT(EPOCH FROM (completion_time - start_time)) as duration_seconds,
    failure_phase,
    failure_reason
FROM remediation_audit
WHERE alert_fingerprint = 'mem-pressure-prod-123'
ORDER BY start_time DESC
LIMIT 10;
```

**Orchestration success rate:**
```sql
SELECT
    overall_phase,
    COUNT(*) as total,
    COUNT(CASE WHEN overall_phase = 'completed' THEN 1 END) as successful,
    COUNT(CASE WHEN overall_phase = 'failed' THEN 1 END) as failed,
    ROUND(100.0 * COUNT(CASE WHEN overall_phase = 'completed' THEN 1 END) / COUNT(*), 2) as success_rate_pct
FROM remediation_audit
WHERE created_at > NOW() - INTERVAL '7 days'
GROUP BY overall_phase;
```

### Performance Analysis

**Average orchestration duration by phase:**
```sql
SELECT
    overall_phase,
    COUNT(*) as total_remediations,
    AVG(EXTRACT(EPOCH FROM (completion_time - start_time))) as avg_duration_seconds,
    PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (completion_time - start_time))) as p50_duration,
    PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (completion_time - start_time))) as p95_duration
FROM remediation_audit
WHERE completion_time IS NOT NULL
  AND created_at > NOW() - INTERVAL '7 days'
GROUP BY overall_phase;
```

**Slowest orchestrations:**
```sql
SELECT
    remediation_name,
    alert_fingerprint,
    overall_phase,
    start_time,
    completion_time,
    EXTRACT(EPOCH FROM (completion_time - start_time)) as duration_seconds,
    service_crd_statuses
FROM remediation_audit
WHERE completion_time IS NOT NULL
  AND created_at > NOW() - INTERVAL '24 hours'
ORDER BY duration_seconds DESC
LIMIT 20;
```

### Failure Analysis

**Most common failure phases:**
```sql
SELECT
    failure_phase,
    COUNT(*) as failure_count,
    COUNT(DISTINCT alert_fingerprint) as unique_alerts_affected,
    ARRAY_AGG(DISTINCT substring(failure_reason, 1, 100)) as sample_reasons
FROM remediation_audit
WHERE failure_phase IS NOT NULL
  AND created_at > NOW() - INTERVAL '7 days'
GROUP BY failure_phase
ORDER BY failure_count DESC;
```

**Remediation retry analysis:**
```sql
SELECT
    alert_fingerprint,
    COUNT(*) as retry_count,
    MAX(created_at) as last_attempt,
    ARRAY_AGG(overall_phase ORDER BY created_at) as phase_progression,
    BOOL_OR(overall_phase = 'completed') as eventually_successful
FROM remediation_audit
WHERE created_at > NOW() - INTERVAL '24 hours'
GROUP BY alert_fingerprint
HAVING COUNT(*) > 1
ORDER BY retry_count DESC;
```

---

## Prometheus Metrics

### Audit Metrics

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Counter: Audit publish attempts
    AuditPublishAttemptsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_orchestrator_audit_publish_attempts_total",
        Help: "Total audit publish attempts",
    }, []string{"status"}) // success, error

    // Counter: Audit publish failures
    AuditPublishFailuresTotal = promauto.NewCounter(prometheus.CounterOpts{
        Name: "kubernaut_orchestrator_audit_publish_failures_total",
        Help: "Total audit publish failures",
    })

    // Histogram: Audit publish duration
    AuditPublishDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "kubernaut_orchestrator_audit_publish_duration_seconds",
        Help:    "Duration of audit publish operations",
        Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.0},
    })

    // Gauge: Pending audit records (if using batching)
    PendingAuditRecords = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "kubernaut_orchestrator_pending_audit_records",
        Help: "Number of audit records waiting to be published",
    })
)
```

---

## Data Retention Policy

### Automated Cleanup

```sql
-- Delete audit records older than 90 days (compliance requirement)
DELETE FROM remediation_audit
WHERE created_at < NOW() - INTERVAL '90 days'
  AND overall_phase IN ('completed', 'failed');

-- Archive old records to cold storage before deletion
INSERT INTO remediation_audit_archive
SELECT * FROM remediation_audit
WHERE created_at < NOW() - INTERVAL '90 days';
```

### Retention Schedule

| Data Age | Storage Tier | Query Performance | Cost |
|----------|--------------|-------------------|------|
| 0-7 days | Hot (SSD) | Fast (<100ms) | High |
| 7-30 days | Warm (SSD) | Medium (<500ms) | Medium |
| 30-90 days | Cold (HDD) | Slow (<2s) | Low |
| >90 days | Archive (S3) | Very slow (>5s) | Very low |

---

## Testing

### Unit Tests (Mock Storage)

```go
func TestPublishAuditRecord(t *testing.T) {
    mockStorage := &MockAuditStorage{
        stored: make([]*storage.RemediationOrchestrationAudit, 0),
    }

    reconciler := &RemediationRequestReconciler{
        AuditStorage: mockStorage,
    }

    remediation := createTestRemediationRequest()
    err := reconciler.publishAuditRecord(context.TODO(), remediation)
    assert.NoError(t, err)

    assert.Equal(t, 1, len(mockStorage.stored))
    assert.Equal(t, remediation.Name, mockStorage.stored[0].RemediationName)
}
```

### Integration Tests (Real Database)

```go
func TestIntegration_AuditStorage(t *testing.T) {
    // Start test PostgreSQL
    db := startTestDatabase(t)
    defer db.Close()

    client := storage.NewAuditStorageClient("http://localhost:8085")

    audit := &storage.RemediationOrchestrationAudit{
        AlertFingerprint: "test-alert-123",
        RemediationName:  "test-remediation",
        OverallPhase:     "completed",
        StartTime:        time.Now().Add(-5 * time.Minute),
    }
    completionTime := time.Now()
    audit.CompletionTime = &completionTime

    err := client.StoreRemediationOrchestrationAudit(context.TODO(), audit)
    assert.NoError(t, err)

    // Verify in database
    var stored storage.RemediationOrchestrationAudit
    err = db.QueryRow("SELECT * FROM remediation_audit WHERE alert_fingerprint = $1",
        "test-alert-123").Scan(&stored)
    assert.NoError(t, err)
    assert.Equal(t, "completed", stored.OverallPhase)
}
```

---

## Common Pitfalls & Best Practices

### Don't ❌

- **Don't block reconciliation on audit failures**: Audit storage should be best-effort
- **Don't store business logic results**: Downstream controllers audit their own business logic
- **Don't use synchronous storage**: Consider batching for high volume
- **Don't skip CRD status collection**: Capture complete orchestration state

### Do ✅

- **Use async storage**: Don't block controller on database writes
- **Implement retry logic**: Retry transient failures with exponential backoff
- **Monitor audit metrics**: Track success rate, duration, failures
- **Set retention policies**: Comply with regulatory requirements
- **Capture phase transitions**: Audit on every phase change for debugging
- **Include CRD status snapshots**: Store downstream CRD states in JSONB

---

## References

- **Architecture**: [Multi-CRD Reconciliation](../../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)
- **Storage Service**: [Storage Service Spec](../../services/storage-service/)
- **Metrics**: [Metrics & SLOs](./metrics-slos.md)
- **Testing**: [Testing Strategy](./testing-strategy.md)
- **Integration**: [integration-points.md](./integration-points.md)
