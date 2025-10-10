## Database Integration for Audit & Tracking

### Dual Audit System

Following the [MULTI_CRD_RECONCILIATION_ARCHITECTURE.md](../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md#crd-lifecycle-management-and-cleanup) dual audit approach:

1. **CRDs**: Real-time execution state + 24-hour review window
2. **Database**: Long-term compliance + workflow learning

### Audit Data Schema

```go
package storage

type WorkflowExecutionAudit struct {
    ID               string                    `json:"id" db:"id"`
    RemediationID    string                    `json:"remediation_id" db:"remediation_id"`
    WorkflowName     string                    `json:"workflow_name" db:"workflow_name"`
    WorkflowVersion  string                    `json:"workflow_version" db:"workflow_version"`

    // Execution metrics
    TotalSteps       int                       `json:"total_steps" db:"total_steps"`
    StepsCompleted   int                       `json:"steps_completed" db:"steps_completed"`
    StepsFailed      int                       `json:"steps_failed" db:"steps_failed"`
    TotalDuration    time.Duration             `json:"total_duration" db:"total_duration"`

    // Outcome
    Outcome          string                    `json:"outcome" db:"outcome"` // success, failed, partial
    EffectivenessScore float64                 `json:"effectiveness_score" db:"effectiveness_score"`
    RollbacksPerformed int                     `json:"rollbacks_performed" db:"rollbacks_performed"`

    // Learning data
    StepExecutions   []StepExecutionAudit      `json:"step_executions" db:"step_executions"`
    AdaptiveAdjustments []AdaptiveAdjustment   `json:"adaptive_adjustments" db:"adaptive_adjustments"`

    // Metadata
    CompletedAt      time.Time                 `json:"completed_at" db:"completed_at"`
    Status           string                    `json:"status" db:"status"`
    ErrorMessage     string                    `json:"error_message,omitempty" db:"error_message"`
}

type StepExecutionAudit struct {
    StepNumber       int                       `json:"step_number"`
    Action           string                    `json:"action"`
    Duration         time.Duration             `json:"duration"`
    Status           string                    `json:"status"`
    RetriesAttempted int                       `json:"retries_attempted"`
    ErrorMessage     string                    `json:"error_message,omitempty"`
}

type AdaptiveAdjustment struct {
    StepNumber       int                       `json:"step_number"`
    AdjustmentType   string                    `json:"adjustment_type"` // timeout, retry, skip
    Reason           string                    `json:"reason"`
    Timestamp        time.Time                 `json:"timestamp"`
}
```

---

## PostgreSQL Schema

```sql
CREATE TABLE workflow_execution_audit (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    remediation_id VARCHAR(255) NOT NULL,
    workflow_name VARCHAR(255) NOT NULL,
    workflow_version VARCHAR(50) NOT NULL,

    -- Execution metrics
    total_steps INTEGER NOT NULL,
    steps_completed INTEGER DEFAULT 0,
    steps_failed INTEGER DEFAULT 0,
    total_duration INTERVAL,

    -- Outcome
    outcome VARCHAR(50) NOT NULL, -- success, failed, partial
    effectiveness_score FLOAT,
    rollbacks_performed INTEGER DEFAULT 0,

    -- Learning data (JSONB for flexibility)
    step_executions JSONB,
    adaptive_adjustments JSONB,

    -- Metadata
    completed_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(50),
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Indexes
    INDEX idx_remediation_id (remediation_id),
    INDEX idx_workflow_name (workflow_name),
    INDEX idx_outcome (outcome),
    INDEX idx_created_at (created_at)
);

-- Partition by created_at for performance
CREATE TABLE workflow_execution_audit_2025_01 PARTITION OF workflow_execution_audit
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');
```

---

## Controller Integration

### Audit Publishing in Reconcile Loop

```go
// In WorkflowExecutionReconciler
type WorkflowExecutionReconciler struct {
    client.Client
    Scheme       *runtime.Scheme
    AuditStorage storage.AuditStorageClient
}

// Publish audit on workflow completion
func (r *WorkflowExecutionReconciler) persistWorkflowAudit(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) error {
    // Collect step execution data
    stepExecutions := make([]storage.StepExecutionAudit, 0, len(workflow.Status.Steps))
    for _, step := range workflow.Status.Steps {
        stepExec := storage.StepExecutionAudit{
            StepNumber:       step.Index,
            Action:           step.Action,
            Duration:         step.CompletedAt.Time.Sub(step.StartedAt.Time),
            Status:           string(step.Phase),
            RetriesAttempted: step.RetryCount,
        }
        if step.Error != "" {
            stepExec.ErrorMessage = step.Error
        }
        stepExecutions = append(stepExecutions, stepExec)
    }

    // Calculate effectiveness score
    effectivenessScore := calculateEffectivenessScore(workflow)

    // Collect adaptive adjustments
    adaptiveAdjustments := collectAdaptiveAdjustments(workflow)

    audit := &storage.WorkflowExecutionAudit{
        RemediationID:       workflow.Spec.RemediationRequestRef.Name,
        WorkflowName:        workflow.Name,
        WorkflowVersion:     workflow.Spec.WorkflowVersion,
        TotalSteps:          len(workflow.Spec.Steps),
        StepsCompleted:      countCompletedSteps(workflow),
        StepsFailed:         countFailedSteps(workflow),
        TotalDuration:       workflow.Status.CompletedAt.Time.Sub(workflow.CreationTimestamp.Time),
        Outcome:             determineOutcome(workflow),
        EffectivenessScore:  effectivenessScore,
        RollbacksPerformed:  countRollbacks(workflow),
        StepExecutions:      stepExecutions,
        AdaptiveAdjustments: adaptiveAdjustments,
        CompletedAt:         workflow.Status.CompletedAt.Time,
        Status:              string(workflow.Status.Phase),
    }

    if workflow.Status.Error != "" {
        audit.ErrorMessage = workflow.Status.Error
    }

    return r.AuditStorage.StoreWorkflowExecutionAudit(ctx, audit)
}

// Calculate workflow effectiveness (0.0-1.0)
func calculateEffectivenessScore(workflow *workflowv1.WorkflowExecution) float64 {
    if len(workflow.Status.Steps) == 0 {
        return 0.0
    }

    completedSteps := countCompletedSteps(workflow)
    totalSteps := len(workflow.Spec.Steps)

    successRate := float64(completedSteps) / float64(totalSteps)

    // Penalize for retries and rollbacks
    totalRetries := 0
    for _, step := range workflow.Status.Steps {
        totalRetries += step.RetryCount
    }
    retryPenalty := float64(totalRetries) * 0.05 // -5% per retry
    rollbackPenalty := float64(countRollbacks(workflow)) * 0.10 // -10% per rollback

    score := successRate - retryPenalty - rollbackPenalty
    if score < 0.0 {
        return 0.0
    }
    if score > 1.0 {
        return 1.0
    }
    return score
}

func countCompletedSteps(workflow *workflowv1.WorkflowExecution) int {
    count := 0
    for _, step := range workflow.Status.Steps {
        if step.Phase == "completed" {
            count++
        }
    }
    return count
}

func countFailedSteps(workflow *workflowv1.WorkflowExecution) int {
    count := 0
    for _, step := range workflow.Status.Steps {
        if step.Phase == "failed" {
            count++
        }
    }
    return count
}

func countRollbacks(workflow *workflowv1.WorkflowExecution) int {
    // Count steps that were rolled back
    rollbacks := 0
    for _, step := range workflow.Status.Steps {
        if step.RolledBack {
            rollbacks++
        }
    }
    return rollbacks
}

func determineOutcome(workflow *workflowv1.WorkflowExecution) string {
    if workflow.Status.Phase == "completed" {
        return "success"
    } else if workflow.Status.Phase == "failed" {
        return "failed"
    } else {
        return "partial"
    }
}

func collectAdaptiveAdjustments(workflow *workflowv1.WorkflowExecution) []storage.AdaptiveAdjustment {
    adjustments := make([]storage.AdaptiveAdjustment, 0)

    // Extract adjustments from workflow annotations or status
    if workflow.Annotations != nil {
        if adjustmentsJSON, ok := workflow.Annotations["adaptive-adjustments"]; ok {
            json.Unmarshal([]byte(adjustmentsJSON), &adjustments)
        }
    }

    return adjustments
}
```

---

## HTTP Client Implementation

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

func (c *AuditStorageClient) StoreWorkflowExecutionAudit(
    ctx context.Context,
    audit *WorkflowExecutionAudit,
) error {
    url := fmt.Sprintf("%s/api/v1/audit/workflow-execution", c.storageServiceURL)

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

### Workflow Learning & Optimization

**Find most effective workflows:**
```sql
SELECT
    workflow_name,
    COUNT(*) as execution_count,
    AVG(effectiveness_score) as avg_effectiveness,
    AVG(total_steps) as avg_steps,
    AVG(EXTRACT(EPOCH FROM total_duration)) as avg_duration_seconds
FROM workflow_execution_audit
WHERE created_at > NOW() - INTERVAL '30 days'
  AND outcome = 'success'
GROUP BY workflow_name
ORDER BY avg_effectiveness DESC
LIMIT 10;
```

**Identify workflows needing optimization:**
```sql
SELECT
    workflow_name,
    COUNT(*) as failure_count,
    AVG(steps_failed) as avg_failed_steps,
    ARRAY_AGG(DISTINCT error_message) as error_patterns
FROM workflow_execution_audit
WHERE outcome = 'failed'
  AND created_at > NOW() - INTERVAL '7 days'
GROUP BY workflow_name
ORDER BY failure_count DESC;
```

### Step Performance Analysis

**Slowest workflow steps:**
```sql
SELECT
    workflow_name,
    step_exec->>'action' as action_type,
    COUNT(*) as execution_count,
    AVG((step_exec->>'duration')::numeric) as avg_duration_ms,
    PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY (step_exec->>'duration')::numeric) as p95_duration_ms
FROM workflow_execution_audit,
     jsonb_array_elements(step_executions) as step_exec
WHERE created_at > NOW() - INTERVAL '7 days'
GROUP BY workflow_name, step_exec->>'action'
ORDER BY avg_duration_ms DESC
LIMIT 20;
```

**Step retry patterns:**
```sql
SELECT
    step_exec->>'action' as action_type,
    AVG((step_exec->>'retries_attempted')::int) as avg_retries,
    COUNT(*) as total_executions,
    COUNT(CASE WHEN (step_exec->>'status')::text = 'failed' THEN 1 END) as failures
FROM workflow_execution_audit,
     jsonb_array_elements(step_executions) as step_exec
WHERE created_at > NOW() - INTERVAL '7 days'
  AND (step_exec->>'retries_attempted')::int > 0
GROUP BY step_exec->>'action'
ORDER BY avg_retries DESC;
```

### Adaptive Learning Insights

**Most common adaptive adjustments:**
```sql
SELECT
    adj->>'adjustment_type' as adjustment_type,
    adj->>'reason' as reason,
    COUNT(*) as frequency
FROM workflow_execution_audit,
     jsonb_array_elements(adaptive_adjustments) as adj
WHERE created_at > NOW() - INTERVAL '30 days'
GROUP BY adj->>'adjustment_type', adj->>'reason'
ORDER BY frequency DESC;
```

---

## Prometheus Metrics

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Histogram: Workflow execution duration
    WorkflowExecutionDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "kubernaut_workflow_execution_duration_seconds",
        Help:    "Duration of workflow executions",
        Buckets: []float64{30, 60, 120, 300, 600, 1200, 1800}, // 30s to 30min
    }, []string{"workflow_name", "outcome"})

    // Counter: Step execution results
    StepExecutionResults = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_workflow_step_executions_total",
        Help: "Total step executions by action and status",
    }, []string{"workflow_name", "action", "status"})

    // Gauge: Workflow effectiveness score
    WorkflowEffectivenessScore = promauto.NewGaugeVec(prometheus.GaugeOpts{
        Name: "kubernaut_workflow_effectiveness_score",
        Help: "Workflow effectiveness score (0.0-1.0)",
    }, []string{"workflow_name"})

    // Counter: Audit storage operations
    AuditStorageOperations = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_workflow_audit_storage_operations_total",
        Help: "Total workflow audit storage operations",
    }, []string{"status"}) // success, error
)
```

---

## Data Retention Policy

### Automated Cleanup

```sql
-- Delete audit records older than 90 days
DELETE FROM workflow_execution_audit
WHERE created_at < NOW() - INTERVAL '90 days';

-- Archive old records before deletion
INSERT INTO workflow_execution_audit_archive
SELECT * FROM workflow_execution_audit
WHERE created_at < NOW() - INTERVAL '90 days';
```

---

## Testing

### Unit Tests

```go
func TestPersistWorkflowAudit(t *testing.T) {
    mockStorage := &MockAuditStorage{}
    reconciler := &WorkflowExecutionReconciler{
        AuditStorage: mockStorage,
    }

    workflow := createTestWorkflow()
    workflow.Status.Phase = "completed"
    workflow.Status.CompletedAt = &metav1.Time{Time: time.Now()}

    err := reconciler.persistWorkflowAudit(context.TODO(), workflow)
    assert.NoError(t, err)

    assert.Equal(t, 1, len(mockStorage.stored))
    assert.Equal(t, "success", mockStorage.stored[0].Outcome)
}
```

### Integration Tests

```go
func TestIntegration_WorkflowAuditStorage(t *testing.T) {
    db := startTestDatabase(t)
    defer db.Close()

    client := storage.NewAuditStorageClient("http://localhost:8085")

    audit := &storage.WorkflowExecutionAudit{
        WorkflowName:    "test-workflow",
        WorkflowVersion: "v1",
        TotalSteps:      3,
        StepsCompleted:  3,
        Outcome:         "success",
        EffectivenessScore: 0.95,
    }

    err := client.StoreWorkflowExecutionAudit(context.TODO(), audit)
    assert.NoError(t, err)

    // Verify in database
    var stored storage.WorkflowExecutionAudit
    err = db.QueryRow("SELECT * FROM workflow_execution_audit WHERE workflow_name = $1",
        "test-workflow").Scan(&stored)
    assert.NoError(t, err)
    assert.Equal(t, "success", stored.Outcome)
}
```

---

## References

- **Architecture**: [Multi-CRD Reconciliation](../../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)
- **Storage Service**: [Storage Service Spec](../../services/storage-service/)
- **Metrics**: [Metrics & SLOs](./metrics-slos.md)
- **Testing**: [Testing Strategy](./testing-strategy.md)
