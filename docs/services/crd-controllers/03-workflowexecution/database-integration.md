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
```

---

