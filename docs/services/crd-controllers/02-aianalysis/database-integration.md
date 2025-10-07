## Database Integration for Audit & Tracking

### Dual Audit System

**CRD System** (Temporary - 24 hours):
- Real-time execution tracking
- Status transitions and phase history
- 24-hour retention for review window
- Automatic cleanup after RemediationRequest deletion

**Database System** (Permanent):
- Complete audit trail for compliance
- Historical pattern analysis
- Success rate tracking for recommendations
- Long-term trending and analytics

### Audit Data Persistence

**Trigger Points**:
1. **Investigation Complete**: Store investigation results
2. **Analysis Complete**: Store analysis results and confidence scores
3. **Recommendations Generated**: Store all recommendations with rankings
4. **Workflow Created**: Store recommendation-to-workflow mapping
5. **Analysis Failed**: Store failure reason and context

### Audit Data Schema

**PostgreSQL Table**: `ai_analysis_audit`

```sql
CREATE TABLE ai_analysis_audit (
    id                      UUID PRIMARY KEY,
    crd_name               VARCHAR(255) NOT NULL,
    crd_namespace          VARCHAR(255) NOT NULL,
    alert_fingerprint      VARCHAR(64) NOT NULL,
    environment            VARCHAR(50) NOT NULL,
    severity               VARCHAR(20) NOT NULL,

    -- Investigation phase
    investigation_start    TIMESTAMP,
    investigation_end      TIMESTAMP,
    investigation_duration_ms INTEGER,
    root_cause_count       INTEGER,
    investigation_report   TEXT,

    -- Analysis phase
    analysis_start         TIMESTAMP,
    analysis_end           TIMESTAMP,
    analysis_duration_ms   INTEGER,
    confidence_score       DECIMAL(3,2),
    hallucination_detected BOOLEAN,

    -- Recommendation phase
    recommendation_start   TIMESTAMP,
    recommendation_end     TIMESTAMP,
    recommendations        JSONB,  -- Array of recommendations
    top_recommendation     VARCHAR(255),
    effectiveness_probability DECIMAL(3,2),
    historical_success_rate   DECIMAL(3,2),

    -- Workflow tracking
    workflow_crd_name      VARCHAR(255),
    workflow_crd_namespace VARCHAR(255),

    -- Metadata
    completion_status      VARCHAR(50),  -- completed, failed, timeout
    failure_reason         TEXT,
    created_at             TIMESTAMP DEFAULT NOW(),

    INDEX idx_fingerprint (alert_fingerprint),
    INDEX idx_environment (environment),
    INDEX idx_created_at (created_at)
);
```

**Vector Database Table**: `ai_investigation_embeddings`

```sql
CREATE TABLE ai_investigation_embeddings (
    id                  UUID PRIMARY KEY,
    alert_fingerprint   VARCHAR(64) NOT NULL,
    investigation_text  TEXT NOT NULL,
    embedding           vector(1536),  -- pgvector extension
    root_cause         VARCHAR(255),
    confidence_score   DECIMAL(3,2),
    environment        VARCHAR(50),
    created_at         TIMESTAMP DEFAULT NOW(),

    INDEX idx_embedding USING ivfflat (embedding vector_cosine_ops)
);
```

### Audit Client Integration

```go
// pkg/ai/analysis/integration/audit.go
package integration

import (
    "context"
    "github.com/google/uuid"
    "github.com/jordigilh/kubernaut/pkg/storage/database"
)

type AuditClient struct {
    db *database.PostgresClient
}

func (a *AuditClient) RecordInvestigation(ctx context.Context, aiAnalysis *aianalysisv1.AIAnalysis) error {
    audit := &AIAnalysisAudit{
        ID:                uuid.New(),
        CRDName:          aiAnalysis.Name,
        CRDNamespace:     aiAnalysis.Namespace,
        AlertFingerprint: aiAnalysis.Spec.AnalysisRequest.AlertContext.Fingerprint,
        Environment:      aiAnalysis.Spec.AnalysisRequest.AlertContext.Environment,
        Severity:         aiAnalysis.Spec.AnalysisRequest.AlertContext.Severity,

        InvestigationStart: aiAnalysis.Status.PhaseTransitions["investigating"],
        InvestigationEnd:   time.Now(),
        RootCauseCount:     len(aiAnalysis.Status.InvestigationResult.RootCauseHypotheses),
        InvestigationReport: aiAnalysis.Status.InvestigationResult.InvestigationReport,
    }

    return a.db.Insert(ctx, "ai_analysis_audit", audit)
}

func (a *AuditClient) RecordCompletion(ctx context.Context, aiAnalysis *aianalysisv1.AIAnalysis) error {
    recommendations, _ := json.Marshal(aiAnalysis.Status.Recommendations)

    update := map[string]interface{}{
        "recommendations":             recommendations,
        "top_recommendation":          aiAnalysis.Status.Recommendations[0].Action,
        "effectiveness_probability":   aiAnalysis.Status.Recommendations[0].EffectivenessProbability,
        "historical_success_rate":     aiAnalysis.Status.Recommendations[0].HistoricalSuccessRate,
        "workflow_crd_name":           aiAnalysis.Status.WorkflowExecutionRef.Name,
        "workflow_crd_namespace":      aiAnalysis.Status.WorkflowExecutionRef.Namespace,
        "completion_status":           "completed",
    }

    return a.db.Update(ctx, "ai_analysis_audit", update, "crd_name = ? AND crd_namespace = ?",
        aiAnalysis.Name, aiAnalysis.Namespace)
}
```

### Metrics from Database

```sql
-- Historical success rate for action types
SELECT
    top_recommendation,
    AVG(effectiveness_probability) as avg_effectiveness,
    AVG(historical_success_rate) as avg_historical_success,
    COUNT(*) as total_recommendations
FROM ai_analysis_audit
WHERE completion_status = 'completed'
GROUP BY top_recommendation
ORDER BY avg_effectiveness DESC;

-- Investigation performance by environment
SELECT
    environment,
    AVG(investigation_duration_ms) as avg_duration,
    AVG(confidence_score) as avg_confidence,
    COUNT(*) as total_investigations
FROM ai_analysis_audit
GROUP BY environment;

-- Hallucination detection trends
SELECT
    DATE(created_at) as date,
    SUM(CASE WHEN hallucination_detected THEN 1 ELSE 0 END) as hallucinations,
    COUNT(*) as total_analyses,
    SUM(CASE WHEN hallucination_detected THEN 1 ELSE 0 END)::float / COUNT(*) as hallucination_rate
FROM ai_analysis_audit
GROUP BY DATE(created_at)
ORDER BY date DESC;
```

---

