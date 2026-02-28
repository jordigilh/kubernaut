# Database Integration for Audit & Tracking

> **DEPRECATED**: KubernetesExecution CRD and KubernetesExecutor service were eliminated by ADR-025 and replaced by Tekton TaskRun via WorkflowExecution. This documentation is retained for historical reference only. API types and CRD manifests have been removed from the codebase.

## Overview

The Kubernetes Executor service requires robust audit trail persistence for:
1. **Compliance**: Regulatory requirements for action execution tracking
2. **Post-Mortem Analysis**: Investigation of failed actions or incidents
3. **Performance Monitoring**: Track execution duration and resource usage
4. **Security Auditing**: Monitor all Kubernetes API interactions

---

## 1. Dual Audit System

Following the [MULTI_CRD_RECONCILIATION_ARCHITECTURE.md](../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md#crd-lifecycle-management-and-cleanup) dual audit approach:

1. **CRDs**: Real-time execution state + 24-hour review window
2. **Database**: Long-term compliance + historical analysis

### Why Dual System?

- **CRDs**: Fast query, Kubernetes-native, automatic cleanup
- **Database**: Long-term retention, complex queries, compliance reporting

---

## 2. Audit Data Persistence

**Database Service**: Data Storage Service (Port 8085)
**Purpose**: Persist action execution audit trail before CRD cleanup

### Controller Integration

```go
package controller

import (
    "github.com/jordigilh/kubernaut/pkg/storage"
)

type KubernetesExecutionReconciler struct {
    client.Client
    Scheme       *runtime.Scheme
    AuditStorage storage.AuditStorageClient  // Database client for audit trail
}

// After execution completion, persist audit data
func (r *KubernetesExecutionReconciler) persistExecutionAudit(
    ctx context.Context,
    execution *executionv1.KubernetesExecution,
    job *batchv1.Job,
) error {
    auditRecord := &storage.KubernetesExecutionAudit{
        ExecutionID:       execution.Name,
        WorkflowRef:       execution.Spec.WorkflowExecutionRef.Name,
        StepIndex:         execution.Spec.StepIndex,
        ActionType:        execution.Spec.Action.Type,
        ActionParameters:  execution.Spec.Action.Parameters,

        // Target information
        TargetNamespace:   execution.Spec.Action.Target.Namespace,
        TargetResourceKind: execution.Spec.Action.Target.ResourceKind,
        TargetResourceName: execution.Spec.Action.Target.ResourceName,

        // Execution results
        Success:           execution.Status.Result.Success,
        Output:            execution.Status.Result.Output,
        Error:             execution.Status.Result.Error,
        StartTime:         execution.Status.Result.StartTime.Time,
        EndTime:           execution.Status.Result.EndTime.Time,
        Duration:          execution.Status.Result.EndTime.Time.Sub(execution.Status.Result.StartTime.Time),

        // Job information
        JobName:           job.Name,
        JobPodName:        getPodName(job),
        JobExitCode:       getJobExitCode(job),
        JobResourceUsage:  getResourceUsage(job),

        // Retry information
        RetryCount:        execution.Status.RetryCount,
        MaxRetries:        execution.Spec.MaxRetries,

        // Metadata
        CreatedAt:         execution.CreationTimestamp.Time,
        CompletedAt:       time.Now(),
        Phase:             string(execution.Status.Phase),
    }

    if err := r.AuditStorage.StoreKubernetesExecutionAudit(ctx, auditRecord); err != nil {
        r.Log.Error(err, "Failed to store execution audit",
            "executionID", execution.Name,
            "actionType", execution.Spec.Action.Type)
        // Don't fail reconciliation, but track metric
        AuditStorageFailuresTotal.WithLabelValues("kubernetes_execution").Inc()
    }

    return nil
}
```

---

## 3. Audit Data Schema

### PostgreSQL Schema

```sql
CREATE TABLE kubernetes_execution_audit (
    -- Primary keys
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id        VARCHAR(255) NOT NULL,

    -- Parent references
    workflow_ref        VARCHAR(255) NOT NULL,
    step_index          INTEGER NOT NULL,

    -- Action details
    action_type         VARCHAR(100) NOT NULL,
    action_parameters   JSONB,

    -- Target resource
    target_namespace    VARCHAR(253),
    target_resource_kind VARCHAR(100),
    target_resource_name VARCHAR(253),

    -- Execution results
    success             BOOLEAN NOT NULL,
    output              TEXT,
    error               TEXT,
    start_time          TIMESTAMP WITH TIME ZONE,
    end_time            TIMESTAMP WITH TIME ZONE,
    duration            INTERVAL,

    -- Job information
    job_name            VARCHAR(255),
    job_pod_name        VARCHAR(255),
    job_exit_code       INTEGER,
    job_resource_usage  JSONB,  -- CPU, memory usage

    -- Retry information
    retry_count         INTEGER DEFAULT 0,
    max_retries         INTEGER DEFAULT 3,

    -- Metadata
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL,
    completed_at        TIMESTAMP WITH TIME ZONE,
    phase               VARCHAR(50),

    -- Indexes
    INDEX idx_execution_id (execution_id),
    INDEX idx_workflow_ref (workflow_ref),
    INDEX idx_action_type (action_type),
    INDEX idx_created_at (created_at),
    INDEX idx_success (success),
    INDEX idx_target_resource (target_namespace, target_resource_kind, target_resource_name)
);

-- Partition by created_at for performance
CREATE TABLE kubernetes_execution_audit_2025_01 PARTITION OF kubernetes_execution_audit
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');
```

### Go Types

```go
package storage

type KubernetesExecutionAudit struct {
    ID            string                 `json:"id" db:"id"`
    ExecutionID   string                 `json:"execution_id" db:"execution_id"`

    // Parent references
    WorkflowRef   string                 `json:"workflow_ref" db:"workflow_ref"`
    StepIndex     int                    `json:"step_index" db:"step_index"`

    // Action details
    ActionType       string            `json:"action_type" db:"action_type"`
    ActionParameters json.RawMessage   `json:"action_parameters" db:"action_parameters"`

    // Target resource
    TargetNamespace    string `json:"target_namespace" db:"target_namespace"`
    TargetResourceKind string `json:"target_resource_kind" db:"target_resource_kind"`
    TargetResourceName string `json:"target_resource_name" db:"target_resource_name"`

    // Execution results
    Success   bool          `json:"success" db:"success"`
    Output    string        `json:"output" db:"output"`
    Error     string        `json:"error,omitempty" db:"error"`
    StartTime time.Time     `json:"start_time" db:"start_time"`
    EndTime   time.Time     `json:"end_time" db:"end_time"`
    Duration  time.Duration `json:"duration" db:"duration"`

    // Job information
    JobName          string              `json:"job_name" db:"job_name"`
    JobPodName       string              `json:"job_pod_name" db:"job_pod_name"`
    JobExitCode      int                 `json:"job_exit_code" db:"job_exit_code"`
    JobResourceUsage *ResourceUsage      `json:"job_resource_usage" db:"job_resource_usage"`

    // Retry information
    RetryCount   int `json:"retry_count" db:"retry_count"`
    MaxRetries   int `json:"max_retries" db:"max_retries"`

    // Metadata
    CreatedAt   time.Time `json:"created_at" db:"created_at"`
    CompletedAt time.Time `json:"completed_at" db:"completed_at"`
    Phase       string    `json:"phase" db:"phase"`
}

type ResourceUsage struct {
    CPUMillicores int64 `json:"cpu_millicores" db:"cpu_millicores"`
    MemoryBytes   int64 `json:"memory_bytes" db:"memory_bytes"`
}
```

---

## 4. Audit Use Cases

### Post-Mortem Analysis

**Find all executions for a specific workflow:**
```sql
SELECT
    execution_id,
    action_type,
    target_resource_name,
    success,
    duration,
    retry_count
FROM kubernetes_execution_audit
WHERE workflow_ref = 'alert-12345-workflow'
ORDER BY step_index;
```

**Find failed executions with errors:**
```sql
SELECT
    execution_id,
    action_type,
    target_namespace,
    target_resource_name,
    error,
    retry_count,
    completed_at
FROM kubernetes_execution_audit
WHERE success = FALSE
  AND completed_at > NOW() - INTERVAL '24 hours'
ORDER BY completed_at DESC;
```

### Performance Analysis

**Average execution duration by action type:**
```sql
SELECT
    action_type,
    COUNT(*) as total_executions,
    AVG(duration) as avg_duration,
    PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration) as p50_duration,
    PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration) as p95_duration,
    PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY duration) as p99_duration
FROM kubernetes_execution_audit
WHERE created_at > NOW() - INTERVAL '7 days'
  AND success = TRUE
GROUP BY action_type
ORDER BY avg_duration DESC;
```

**Find slow executions:**
```sql
SELECT
    execution_id,
    action_type,
    target_resource_name,
    duration,
    job_resource_usage
FROM kubernetes_execution_audit
WHERE duration > INTERVAL '30 seconds'
  AND created_at > NOW() - INTERVAL '24 hours'
ORDER BY duration DESC
LIMIT 100;
```

### Compliance & Security Auditing

**Track all actions on production namespace:**
```sql
SELECT
    execution_id,
    action_type,
    target_resource_kind,
    target_resource_name,
    success,
    created_at,
    job_pod_name
FROM kubernetes_execution_audit
WHERE target_namespace LIKE 'prod-%'
  AND created_at > NOW() - INTERVAL '30 days'
ORDER BY created_at DESC;
```

**Find retry patterns (actions requiring multiple attempts):**
```sql
SELECT
    action_type,
    target_namespace,
    COUNT(*) as total_with_retries,
    AVG(retry_count) as avg_retries
FROM kubernetes_execution_audit
WHERE retry_count > 0
  AND created_at > NOW() - INTERVAL '7 days'
GROUP BY action_type, target_namespace
ORDER BY total_with_retries DESC;
```

**Resource usage analysis:**
```sql
SELECT
    action_type,
    AVG((job_resource_usage->>'cpu_millicores')::bigint) as avg_cpu_millicores,
    AVG((job_resource_usage->>'memory_bytes')::bigint) as avg_memory_bytes,
    COUNT(*) as total_executions
FROM kubernetes_execution_audit
WHERE created_at > NOW() - INTERVAL '7 days'
  AND job_resource_usage IS NOT NULL
GROUP BY action_type
ORDER BY avg_cpu_millicores DESC;
```

---

## 5. Storage Service Integration

### HTTP Client Implementation

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

func (c *AuditStorageClient) StoreKubernetesExecutionAudit(
    ctx context.Context,
    audit *KubernetesExecutionAudit,
) error {
    url := fmt.Sprintf("%s/api/v1/audit/kubernetes-execution", c.storageServiceURL)

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

### Configuration

```yaml
# config/default/manager_config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernetes-executor-config
data:
  storage_service_url: "http://storage-service.kubernaut.svc.cluster.local:8085"
  audit_enabled: "true"
  audit_batch_size: "100"
  audit_flush_interval: "30s"
```

---

## 6. Audit Metrics

### Prometheus Metrics

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Counter: Audit storage attempts
    AuditStorageAttemptsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_kubernetesexecutor_audit_storage_attempts_total",
        Help: "Total audit storage attempts",
    }, []string{"status"}) // success, error

    // Counter: Audit storage failures
    AuditStorageFailuresTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_kubernetesexecutor_audit_storage_failures_total",
        Help: "Total audit storage failures",
    }, []string{"error_type"}) // timeout, marshal_error, http_error

    // Histogram: Audit storage duration
    AuditStorageDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "kubernaut_kubernetesexecutor_audit_storage_duration_seconds",
        Help:    "Duration of audit storage operations",
        Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.0},
    })

    // Gauge: Pending audit records (if using batching)
    PendingAuditRecords = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "kubernaut_kubernetesexecutor_pending_audit_records",
        Help: "Number of audit records waiting to be persisted",
    })
)

// Usage in controller
func (r *KubernetesExecutionReconciler) persistExecutionAudit(...) error {
    start := time.Now()
    defer func() {
        AuditStorageDuration.Observe(time.Since(start).Seconds())
    }()

    err := r.AuditStorage.StoreKubernetesExecutionAudit(ctx, auditRecord)
    if err != nil {
        AuditStorageAttemptsTotal.WithLabelValues("error").Inc()
        AuditStorageFailuresTotal.WithLabelValues(classifyError(err)).Inc()
        return err
    }

    AuditStorageAttemptsTotal.WithLabelValues("success").Inc()
    return nil
}
```

---

## 7. Batch Processing (Optional Optimization)

For high-volume environments, consider batch processing audit records:

```go
type BatchedAuditStorage struct {
    client      *AuditStorageClient
    batchSize   int
    flushInterval time.Duration
    pending     []*KubernetesExecutionAudit
    mu          sync.Mutex
}

func (b *BatchedAuditStorage) Add(audit *KubernetesExecutionAudit) error {
    b.mu.Lock()
    defer b.mu.Unlock()

    b.pending = append(b.pending, audit)
    PendingAuditRecords.Set(float64(len(b.pending)))

    if len(b.pending) >= b.batchSize {
        return b.flushLocked()
    }

    return nil
}

func (b *BatchedAuditStorage) flushLocked() error {
    if len(b.pending) == 0 {
        return nil
    }

    // Send batch to storage service
    err := b.client.StoreBatch(context.Background(), b.pending)
    if err == nil {
        b.pending = b.pending[:0] // Clear batch
        PendingAuditRecords.Set(0)
    }

    return err
}
```

---

## 8. Data Retention Policy

### Automated Cleanup

```sql
-- Delete audit records older than 90 days (compliance requirement)
DELETE FROM kubernetes_execution_audit
WHERE created_at < NOW() - INTERVAL '90 days';

-- Archive old records to cold storage before deletion
INSERT INTO kubernetes_execution_audit_archive
SELECT * FROM kubernetes_execution_audit
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

## 9. Testing

### Unit Tests (Mock Storage)

```go
func TestPersistExecutionAudit(t *testing.T) {
    mockStorage := &MockAuditStorage{
        stored: make([]*storage.KubernetesExecutionAudit, 0),
    }

    reconciler := &KubernetesExecutionReconciler{
        AuditStorage: mockStorage,
    }

    execution := createTestExecution()
    job := createTestJob()

    err := reconciler.persistExecutionAudit(context.TODO(), execution, job)
    assert.NoError(t, err)

    assert.Equal(t, 1, len(mockStorage.stored))
    assert.Equal(t, execution.Name, mockStorage.stored[0].ExecutionID)
}
```

### Integration Tests (Real Database)

```go
func TestIntegration_AuditStorage(t *testing.T) {
    // Start test PostgreSQL
    db := startTestDatabase(t)
    defer db.Close()

    client := storage.NewAuditStorageClient("http://localhost:8085")

    audit := &storage.KubernetesExecutionAudit{
        ExecutionID: "test-exec-1",
        ActionType:  "scale-deployment",
        Success:     true,
    }

    err := client.StoreKubernetesExecutionAudit(context.TODO(), audit)
    assert.NoError(t, err)

    // Verify in database
    var stored storage.KubernetesExecutionAudit
    err = db.QueryRow("SELECT * FROM kubernetes_execution_audit WHERE execution_id = $1",
        "test-exec-1").Scan(&stored)
    assert.NoError(t, err)
    assert.Equal(t, "scale-deployment", stored.ActionType)
}
```

---

## 10. Common Pitfalls & Best Practices

### Don't ❌

- **Don't block reconciliation on audit failures**: Audit storage should be best-effort
- **Don't store sensitive data**: Sanitize action parameters before storage
- **Don't use synchronous storage**: Consider batching for high volume
- **Don't ignore storage failures**: Track metrics and alert on failures

### Do ✅

- **Use async storage**: Don't block controller on database writes
- **Sanitize sensitive data**: Redact secrets, tokens from audit records
- **Implement retry logic**: Retry transient failures with exponential backoff
- **Monitor audit metrics**: Track success rate, duration, failures
- **Set retention policies**: Comply with regulatory requirements
- **Use partitioning**: Partition tables by date for better performance

---

## 11. References

- **Architecture**: [Multi-CRD Reconciliation](../../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)
- **Storage Service**: [Storage Service Spec](../../services/storage-service/)
- **Metrics**: [Metrics & SLOs](./metrics-slos.md)
- **Testing**: [Testing Strategy](./testing-strategy.md)
- **Security**: [Security Configuration](./security-configuration.md)
