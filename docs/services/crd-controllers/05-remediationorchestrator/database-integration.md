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
    kubernetes_execution_name VARCHAR(255),

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

