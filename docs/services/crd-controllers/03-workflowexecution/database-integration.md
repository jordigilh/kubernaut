## Database Integration for Audit & Tracking

**Version**: 4.0
**Last Updated**: 2025-12-02
**CRD API Group**: `workflowexecution.kubernaut.ai/v1alpha1`
**Status**: ✅ Updated for Tekton Architecture (ADR-044)

---

## Changelog

### Version 4.0 (2025-12-02)
- ✅ **Updated**: Audit schema for Tekton delegation model
- ✅ **Removed**: Step orchestration audit (Tekton provides this)
- ✅ **Added**: Resource locking audit fields (DD-WE-001)

---

### Dual Audit System

Following the [MULTI_CRD_RECONCILIATION_ARCHITECTURE.md](../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md#crd-lifecycle-management-and-cleanup) dual audit approach:

1. **CRDs**: Real-time execution state + 24-hour review window
2. **Database**: Long-term compliance + workflow learning

### Audit Data Schema

```go
package storage

import "time"

type WorkflowExecutionAudit struct {
    ID               string    `json:"id" db:"id"`
    RemediationID    string    `json:"remediation_id" db:"remediation_id"`

    // Workflow reference
    WorkflowID       string    `json:"workflow_id" db:"workflow_id"`
    WorkflowVersion  string    `json:"workflow_version" db:"workflow_version"`
    ContainerImage   string    `json:"container_image" db:"container_image"`

    // Execution context
    TargetResource   string    `json:"target_resource" db:"target_resource"`
    Parameters       map[string]string `json:"parameters" db:"parameters"`

    // Execution metrics (from Tekton PipelineRun)
    TotalDuration    time.Duration     `json:"total_duration" db:"total_duration"`
    PipelineRunName  string            `json:"pipelinerun_name" db:"pipelinerun_name"`

    // Outcome
    Phase            string    `json:"phase" db:"phase"`         // Completed, Failed, Skipped
    Outcome          string    `json:"outcome" db:"outcome"`     // Success, Failed, n/a
    FailureReason    string    `json:"failure_reason,omitempty" db:"failure_reason"`
    FailureMessage   string    `json:"failure_message,omitempty" db:"failure_message"`

    // Resource locking (DD-WE-001)
    SkipReason       string    `json:"skip_reason,omitempty" db:"skip_reason"` // ResourceBusy, RecentlyRemediated
    ConflictingWFE   string    `json:"conflicting_wfe,omitempty" db:"conflicting_wfe"`

    // Metadata
    StartedAt        time.Time `json:"started_at" db:"started_at"`
    CompletedAt      time.Time `json:"completed_at" db:"completed_at"`
    CorrelationID    string    `json:"correlation_id" db:"correlation_id"`
    CreatedAt        time.Time `json:"created_at" db:"created_at"`
}
```

---

## PostgreSQL Schema

```sql
CREATE TABLE workflow_execution_audit (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    remediation_id VARCHAR(255) NOT NULL,

    -- Workflow reference
    workflow_id VARCHAR(255) NOT NULL,
    workflow_version VARCHAR(50),
    container_image VARCHAR(512) NOT NULL,

    -- Execution context
    target_resource VARCHAR(255) NOT NULL,
    parameters JSONB,

    -- Execution metrics
    total_duration INTERVAL,
    pipelinerun_name VARCHAR(255),

    -- Outcome
    phase VARCHAR(50) NOT NULL,       -- Completed, Failed, Skipped
    outcome VARCHAR(50),              -- Success, Failed, n/a
    failure_reason VARCHAR(100),
    failure_message TEXT,

    -- Resource locking (DD-WE-001)
    skip_reason VARCHAR(50),          -- ResourceBusy, RecentlyRemediated
    conflicting_wfe VARCHAR(255),

    -- Metadata
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    correlation_id VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Indexes for common queries
    INDEX idx_remediation_id (remediation_id),
    INDEX idx_workflow_id (workflow_id),
    INDEX idx_target_resource (target_resource),
    INDEX idx_phase (phase),
    INDEX idx_correlation_id (correlation_id),
    INDEX idx_created_at (created_at)
);

-- Partition by created_at for performance
CREATE TABLE workflow_execution_audit_2025_01 PARTITION OF workflow_execution_audit
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');
```

---

## Client Implementation

```go
package storage

import (
    "context"
    "time"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

type WorkflowAuditClient interface {
    RecordExecution(ctx context.Context, wfe *workflowexecutionv1.WorkflowExecution) error
}

type workflowAuditClient struct {
    db *sql.DB
}

func NewWorkflowAuditClient(db *sql.DB) WorkflowAuditClient {
    return &workflowAuditClient{db: db}
}

func (c *workflowAuditClient) RecordExecution(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) error {
    audit := &WorkflowExecutionAudit{
        RemediationID:   wfe.Spec.RemediationRequestRef.Name,
        WorkflowID:      wfe.Spec.WorkflowRef.WorkflowID,
        WorkflowVersion: wfe.Spec.WorkflowRef.Version,
        ContainerImage:  wfe.Spec.WorkflowRef.ContainerImage,
        TargetResource:  wfe.Spec.TargetResource,
        Parameters:      wfe.Spec.Parameters,
        Phase:           string(wfe.Status.Phase),
        Outcome:         string(wfe.Status.Outcome),
        CorrelationID:   wfe.Labels["kubernaut.ai/correlation-id"],
        CreatedAt:       time.Now(),
    }

    // Add timing info
    if wfe.Status.StartTime != nil {
        audit.StartedAt = wfe.Status.StartTime.Time
    }
    if wfe.Status.CompletionTime != nil {
        audit.CompletedAt = wfe.Status.CompletionTime.Time
        if wfe.Status.StartTime != nil {
            audit.TotalDuration = audit.CompletedAt.Sub(audit.StartedAt)
        }
    }

    // Add PipelineRun reference
    if wfe.Status.PipelineRunRef != nil {
        audit.PipelineRunName = wfe.Status.PipelineRunRef.Name
    }

    // Add failure details
    if wfe.Status.FailureDetails != nil {
        audit.FailureReason = wfe.Status.FailureDetails.Reason
        audit.FailureMessage = wfe.Status.FailureDetails.Message
    }

    // Add skip details (DD-WE-001)
    if wfe.Status.SkipDetails != nil {
        audit.SkipReason = wfe.Status.SkipDetails.Reason
        if wfe.Status.SkipDetails.ConflictingWorkflow != nil {
            audit.ConflictingWFE = wfe.Status.SkipDetails.ConflictingWorkflow.Name
        }
    }

    return c.insert(ctx, audit)
}

func (c *workflowAuditClient) insert(ctx context.Context, audit *WorkflowExecutionAudit) error {
    query := `
        INSERT INTO workflow_execution_audit (
            remediation_id, workflow_id, workflow_version, container_image,
            target_resource, parameters, total_duration, pipelinerun_name,
            phase, outcome, failure_reason, failure_message,
            skip_reason, conflicting_wfe,
            started_at, completed_at, correlation_id, created_at
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
        )`

    _, err := c.db.ExecContext(ctx, query,
        audit.RemediationID, audit.WorkflowID, audit.WorkflowVersion, audit.ContainerImage,
        audit.TargetResource, audit.Parameters, audit.TotalDuration, audit.PipelineRunName,
        audit.Phase, audit.Outcome, audit.FailureReason, audit.FailureMessage,
        audit.SkipReason, audit.ConflictingWFE,
        audit.StartedAt, audit.CompletedAt, audit.CorrelationID, audit.CreatedAt,
    )
    return err
}
```

---

## Useful Queries

### Recent Failures by Target Resource

```sql
SELECT
    target_resource,
    workflow_id,
    failure_reason,
    failure_message,
    completed_at
FROM workflow_execution_audit
WHERE phase = 'Failed'
  AND completed_at > NOW() - INTERVAL '24 hours'
ORDER BY completed_at DESC;
```

### Resource Lock Effectiveness (DD-WE-001)

```sql
SELECT
    skip_reason,
    COUNT(*) as skip_count
FROM workflow_execution_audit
WHERE phase = 'Skipped'
  AND created_at > NOW() - INTERVAL '7 days'
GROUP BY skip_reason
ORDER BY skip_count DESC;
```

### Workflow Success Rate by Target

```sql
SELECT
    target_resource,
    workflow_id,
    COUNT(*) as total_executions,
    COUNT(*) FILTER (WHERE outcome = 'Success') as successful,
    ROUND(
        COUNT(*) FILTER (WHERE outcome = 'Success')::numeric /
        NULLIF(COUNT(*), 0) * 100, 2
    ) as success_rate_pct
FROM workflow_execution_audit
WHERE phase IN ('Completed', 'Failed')
  AND created_at > NOW() - INTERVAL '30 days'
GROUP BY target_resource, workflow_id
ORDER BY total_executions DESC;
```

---

## Prometheus Metrics

```go
package controller

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Audit recording metrics
    AuditRecordTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_workflowexecution_audit_records_total",
        Help: "Total audit records by phase",
    }, []string{"phase"})

    AuditRecordDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "kubernaut_workflowexecution_audit_record_duration_seconds",
        Help:    "Audit record insertion duration",
        Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to 1s
    })

    AuditRecordErrors = promauto.NewCounter(prometheus.CounterOpts{
        Name: "kubernaut_workflowexecution_audit_record_errors_total",
        Help: "Total audit record insertion errors",
    })
)
```

---
