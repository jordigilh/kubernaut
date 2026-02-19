# Database Integration for Audit & Tracking

**Version**: v2.0
**Status**: ✅ Complete - V1.0 Aligned
**Last Updated**: 2025-11-30

---

## 📋 Changelog

| Version | Date | Changes |
|---------|------|---------|
| v2.0 | 2025-11-30 | **V1.0 ALIGNMENT**: Updated schema to match AIAnalysis CRD types; Added DetectedLabels (ADR-056: removed from EnrichmentResults)/CustomLabels columns; Updated for workflow selection (not recommendation generation); Removed hallucination detection column (redefined for catalog validation) |
| v1.0 | 2025-10-15 | Initial specification |

---

## Dual Audit System

### CRD System (Temporary - 24 hours)

- Real-time execution tracking
- Status transitions and phase history
- 24-hour retention for review window
- Automatic cleanup after RemediationRequest deletion

### Database System (Permanent)

- Complete audit trail for compliance
- Historical pattern analysis
- Workflow selection tracking
- Long-term trending and analytics

---

## Audit Data Persistence

### Trigger Points

1. **Investigation Start**: Record analysis initiation
2. **Investigation Complete**: Store RCA and workflow selection
3. **Approval Signaling**: Store approval decision (V1.0: signaling, not CRD)
4. **Analysis Complete**: Store final status and outcome
5. **Analysis Failed**: Store failure reason and context

---

## Audit Data Schema

### PostgreSQL Table: `ai_analysis_audit`

```sql
CREATE TABLE ai_analysis_audit (
    id                      UUID PRIMARY KEY,
    crd_name               VARCHAR(255) NOT NULL,
    crd_namespace          VARCHAR(255) NOT NULL,
    remediation_id         VARCHAR(64) NOT NULL,
    alert_fingerprint      VARCHAR(64) NOT NULL,
    environment            VARCHAR(50) NOT NULL,
    severity               VARCHAR(20) NOT NULL,

    -- Signal context
    signal_type            VARCHAR(50) NOT NULL,
    business_priority      VARCHAR(20),
    target_resource_kind   VARCHAR(50),
    target_resource_name   VARCHAR(255),
    target_resource_ns     VARCHAR(255),

    -- DetectedLabels (V1.0) - stored as JSON for flexibility
    detected_labels        JSONB,  -- {"gitOpsManaged": true, "gitOpsTool": "argocd", ...}

    -- CustomLabels (V1.0) - Rego-extracted labels
    custom_labels          JSONB,  -- {"constraint": ["cost-constrained"], "team": ["name=payments"]}

    -- Investigation phase
    investigation_start    TIMESTAMP,
    investigation_end      TIMESTAMP,
    investigation_duration_ms INTEGER,
    root_cause_summary     TEXT,
    root_cause_confidence  DECIMAL(3,2),

    -- Workflow selection (V1.0)
    selected_workflow_id   VARCHAR(255),
    workflow_confidence    DECIMAL(3,2),
    workflow_parameters    JSONB,

    -- Approval (V1.0: signaling, not CRD)
    approval_required      BOOLEAN DEFAULT FALSE,
    approval_reason        TEXT,  -- Rego policy reason if approval required

    -- Catalog validation (V1.0: replaces hallucination detection)
    catalog_validation_passed BOOLEAN DEFAULT TRUE,
    catalog_validation_errors JSONB,  -- ["workflow_not_found", "invalid_parameters", ...]

    -- Recovery context (if isRecoveryAttempt = true)
    is_recovery_attempt    BOOLEAN DEFAULT FALSE,
    previous_attempts      JSONB,  -- Array of previous execution summaries
    recovery_attempt_number INTEGER,

    -- Metadata
    completion_status      VARCHAR(50),  -- Ready | Failed | Pending
    failure_reason         TEXT,
    created_at             TIMESTAMP DEFAULT NOW(),
    completed_at           TIMESTAMP,

    -- Indexes
    INDEX idx_fingerprint (alert_fingerprint),
    INDEX idx_environment (environment),
    INDEX idx_remediation_id (remediation_id),
    INDEX idx_created_at (created_at),
    INDEX idx_workflow_id (selected_workflow_id)
);
```

### PostgreSQL Table: `ai_investigation_embeddings`

```sql
-- For future semantic search of past investigations (V1.1+)
CREATE TABLE ai_investigation_embeddings (
    id                  UUID PRIMARY KEY,
    alert_fingerprint   VARCHAR(64) NOT NULL,
    investigation_text  TEXT NOT NULL,
    embedding           vector(1536),  -- pgvector extension
    root_cause_summary  TEXT,
    workflow_id         VARCHAR(255),
    confidence_score    DECIMAL(3,2),
    environment         VARCHAR(50),
    created_at          TIMESTAMP DEFAULT NOW(),

    INDEX idx_embedding USING ivfflat (embedding vector_cosine_ops)
);
```

---

## Audit Client Integration

```go
// pkg/aianalysis/integration/audit.go
package integration

import (
    "context"
    "encoding/json"
    "time"

    "github.com/google/uuid"

    aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/storage/database"
)

type AuditClient struct {
    db *database.PostgresClient
}

func NewAuditClient(db *database.PostgresClient) *AuditClient {
    return &AuditClient{db: db}
}

// RecordInvestigationStart records when analysis begins
func (a *AuditClient) RecordInvestigationStart(
    ctx context.Context,
    aiAnalysis *aianalysisv1alpha1.AIAnalysis,
) error {
    // Marshal DetectedLabels and CustomLabels to JSON
    detectedLabelsJSON, _ := json.Marshal(aiAnalysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults.DetectedLabels)  // ADR-056: removed from EnrichmentResults
    customLabelsJSON, _ := json.Marshal(aiAnalysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults.CustomLabels)

    audit := &AIAnalysisAudit{
        ID:                 uuid.New(),
        CRDName:            aiAnalysis.Name,
        CRDNamespace:       aiAnalysis.Namespace,
        RemediationID:      aiAnalysis.Spec.RemediationID,
        AlertFingerprint:   aiAnalysis.Spec.AnalysisRequest.SignalContext.Fingerprint,
        Environment:        aiAnalysis.Spec.AnalysisRequest.SignalContext.Environment,
        Severity:           aiAnalysis.Spec.AnalysisRequest.SignalContext.Severity,
        SignalType:         aiAnalysis.Spec.AnalysisRequest.SignalContext.SignalType,
        BusinessPriority:   aiAnalysis.Spec.AnalysisRequest.SignalContext.BusinessPriority,
        TargetResourceKind: aiAnalysis.Spec.AnalysisRequest.SignalContext.TargetResource.Kind,
        TargetResourceName: aiAnalysis.Spec.AnalysisRequest.SignalContext.TargetResource.Name,
        TargetResourceNS:   aiAnalysis.Spec.AnalysisRequest.SignalContext.TargetResource.Namespace,
        DetectedLabels:     detectedLabelsJSON,
        CustomLabels:       customLabelsJSON,
        InvestigationStart: time.Now(),
        IsRecoveryAttempt:  aiAnalysis.Spec.IsRecoveryAttempt,
        RecoveryAttemptNum: len(aiAnalysis.Spec.PreviousExecutions) + 1,
        CompletionStatus:   "Pending",
    }

    return a.db.Insert(ctx, "ai_analysis_audit", audit)
}

// RecordCompletion records the final analysis outcome
func (a *AuditClient) RecordCompletion(
    ctx context.Context,
    aiAnalysis *aianalysisv1alpha1.AIAnalysis,
) error {
    // Marshal workflow parameters
    workflowParams, _ := json.Marshal(aiAnalysis.Status.SelectedWorkflow.Parameters)
    catalogErrors, _ := json.Marshal(aiAnalysis.Status.ValidationErrors)
    previousAttempts, _ := json.Marshal(aiAnalysis.Spec.PreviousExecutions)

    update := map[string]interface{}{
        "investigation_end":         time.Now(),
        "investigation_duration_ms": calculateDuration(aiAnalysis),
        "root_cause_summary":        aiAnalysis.Status.RootCauseAnalysis.Summary,
        "root_cause_confidence":     aiAnalysis.Status.RootCauseAnalysis.Confidence,
        "selected_workflow_id":      aiAnalysis.Status.SelectedWorkflow.WorkflowID,
        "workflow_confidence":       aiAnalysis.Status.SelectedWorkflow.Confidence,
        "workflow_parameters":       workflowParams,
        "approval_required":         aiAnalysis.Status.ApprovalRequired,
        "approval_reason":           aiAnalysis.Status.ApprovalReason,
        "catalog_validation_passed": len(aiAnalysis.Status.ValidationErrors) == 0,
        "catalog_validation_errors": catalogErrors,
        "previous_attempts":         previousAttempts,
        "completion_status":         string(aiAnalysis.Status.Phase),
        "completed_at":              time.Now(),
    }

    if aiAnalysis.Status.Phase == aianalysisv1alpha1.PhaseFailed {
        update["failure_reason"] = aiAnalysis.Status.ErrorMessage
    }

    return a.db.Update(ctx, "ai_analysis_audit", update,
        "crd_name = ? AND crd_namespace = ?",
        aiAnalysis.Name, aiAnalysis.Namespace)
}

func calculateDuration(aiAnalysis *aianalysisv1alpha1.AIAnalysis) int {
    if aiAnalysis.Status.CompletionTime == nil {
        return 0
    }
    return int(aiAnalysis.Status.CompletionTime.Sub(aiAnalysis.CreationTimestamp.Time).Milliseconds())
}
```

---

## Analytics Queries

### Historical Success Rate for Workflows

```sql
-- Workflow success rate by workflow ID
SELECT
    selected_workflow_id,
    AVG(workflow_confidence) as avg_confidence,
    COUNT(*) as total_selections,
    SUM(CASE WHEN completion_status = 'Ready' THEN 1 ELSE 0 END) as successful,
    SUM(CASE WHEN approval_required THEN 1 ELSE 0 END) as required_approval
FROM ai_analysis_audit
WHERE completion_status IN ('Ready', 'Failed')
  AND selected_workflow_id IS NOT NULL
GROUP BY selected_workflow_id
ORDER BY total_selections DESC;
```

### Investigation Performance by Environment

```sql
SELECT
    environment,
    AVG(investigation_duration_ms) as avg_duration_ms,
    AVG(root_cause_confidence) as avg_confidence,
    COUNT(*) as total_investigations,
    SUM(CASE WHEN is_recovery_attempt THEN 1 ELSE 0 END) as recovery_attempts
FROM ai_analysis_audit
WHERE completion_status IS NOT NULL
GROUP BY environment
ORDER BY total_investigations DESC;
```

### Catalog Validation Trends (V1.0)

```sql
-- Catalog validation failures (replaces hallucination detection)
SELECT
    DATE(created_at) as date,
    SUM(CASE WHEN NOT catalog_validation_passed THEN 1 ELSE 0 END) as validation_failures,
    COUNT(*) as total_analyses,
    SUM(CASE WHEN NOT catalog_validation_passed THEN 1 ELSE 0 END)::float / COUNT(*) as failure_rate
FROM ai_analysis_audit
WHERE created_at > NOW() - INTERVAL '30 days'
GROUP BY DATE(created_at)
ORDER BY date DESC;
```

### Approval Rate by Signal Type

```sql
SELECT
    signal_type,
    COUNT(*) as total,
    SUM(CASE WHEN approval_required THEN 1 ELSE 0 END) as approvals_required,
    SUM(CASE WHEN approval_required THEN 1 ELSE 0 END)::float / COUNT(*) as approval_rate
FROM ai_analysis_audit
WHERE completion_status = 'Ready'
GROUP BY signal_type
ORDER BY total DESC;
```

### DetectedLabels Usage Analysis

```sql
-- Most common DetectedLabels combinations
SELECT
    detected_labels->>'gitOpsManaged' as gitops_managed,
    detected_labels->>'pdbProtected' as pdb_protected,
    detected_labels->>'stateful' as stateful,
    COUNT(*) as occurrence_count,
    AVG(workflow_confidence) as avg_confidence
FROM ai_analysis_audit
WHERE detected_labels IS NOT NULL
GROUP BY
    detected_labels->>'gitOpsManaged',
    detected_labels->>'pdbProtected',
    detected_labels->>'stateful'
ORDER BY occurrence_count DESC
LIMIT 20;
```

### Recovery Attempt Success Rate

```sql
SELECT
    recovery_attempt_number,
    COUNT(*) as total_attempts,
    SUM(CASE WHEN completion_status = 'Ready' THEN 1 ELSE 0 END) as successful,
    SUM(CASE WHEN completion_status = 'Ready' THEN 1 ELSE 0 END)::float / COUNT(*) as success_rate
FROM ai_analysis_audit
WHERE is_recovery_attempt = TRUE
GROUP BY recovery_attempt_number
ORDER BY recovery_attempt_number;
```

---

## References

- [CRD Schema](./crd-schema.md) - AIAnalysis types definition
- [DD-WORKFLOW-001 v1.8](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) - DetectedLabels/CustomLabels schema
- [BR_MAPPING.md](./BR_MAPPING.md) - Business requirements (BR-AI-023 catalog validation)
