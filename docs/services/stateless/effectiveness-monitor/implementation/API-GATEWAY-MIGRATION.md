# Effectiveness Monitor - API Gateway Migration

**Related Decision**: [DD-ARCH-001: Alternative 2 (API Gateway Pattern)](../../../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md)
**Date**: November 2, 2025
**Status**: ✅ **APPROVED FOR IMPLEMENTATION**
**Service**: Effectiveness Monitor
**Timeline**: **2-3 Days** (Phase 3 of overall migration)
**Depends On**: [Data Storage Service Phase 1](../../data-storage/implementation/API-GATEWAY-MIGRATION.md) ✅ Must complete first

---

## 🎯 **WHAT THIS SERVICE NEEDS TO DO**

**Current State**: Effectiveness Monitor reads audit trail data directly from PostgreSQL

**New State**: Effectiveness Monitor queries Data Storage Service REST API for audit data (continues to write assessments directly)

**Changes Needed**:
1. ✅ Replace direct SQL queries for **audit trail reads** with HTTP client calls
2. ✅ **Keep direct writes** for effectiveness assessments (unchanged)
3. ✅ Update service specification
4. ✅ Update integration test infrastructure (start Data Storage Service in tests)

---

## 📋 **SPECIFICATION CHANGES**

### **1. Service Overview Update**

**File**: `overview.md`

**Current**:
> Effectiveness Monitor reads audit trail data from PostgreSQL.

**New**:
> Effectiveness Monitor queries audit trail data via **Data Storage Service REST API**.
>
> **Data Access Pattern**:
> - **Reads**: Audit trail data → **Data Storage Service REST API** (NEW)
> - **Writes**: Effectiveness assessments → **Direct PostgreSQL** (unchanged)
>
> **Design Decision**: [DD-ARCH-001 Alternative 2](../../../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md)

---

### **2. Integration Points Update**

**File**: `integration-points.md`

**Current**:
> **Downstream**:
> - PostgreSQL (Read audit data, Write assessments)

**New (Updated for ADR-032 v1.1)**:
> **Downstream**:
> - **Data Storage Service REST API** (Read audit trail data AND Write audit traces)
>   - **Read**: `GET /api/v1/incidents?...` (Context API client pattern)
>   - **Write**: `POST /api/v1/audit/effectiveness` (Audit trail persistence)
>   - Client: `pkg/datastorage/client/http_client.go`
> - PostgreSQL (Write effectiveness operational assessments - unchanged)
>   - Tables: `effectiveness_results`, `action_assessments`, `action_outcomes` (migrations/006)
>
> **CRITICAL DISTINCTION** (ADR-032 v1.1):
> - **Audit Trail** (`effectiveness_audit` via Data Storage) → V2.0 RAR generation, 7+ year compliance
> - **Operational Assessments** (`effectiveness_results` direct PostgreSQL) → Real-time learning, 90 day retention
>
> **Design Decision**: [ADR-032 v1.1 Data Access Layer Isolation](../../../../architecture/decisions/ADR-032-data-access-layer-isolation.md)

---

## 📊 **EFFECTIVENESS MONITOR AUDIT SCHEMA** (Phase 0 Day 0.2 - GAP #8)

### **Complete Audit Data Structure**

**Authority**: `migrations/010_audit_write_api.sql` (Table 6: `effectiveness_audit`)

```go
// pkg/effectivenessmonitor/audit/types.go
package audit

import "time"

// EffectivenessAudit represents the complete audit trail for effectiveness assessments
// Persisted via Data Storage Service for V2.0 RAR generation and 7+ year compliance
// Schema Authority: migrations/010_audit_write_api.sql (lines 277-318)
type EffectivenessAudit struct {
    // Identity
    ID            string `json:"id"`              // Unique audit record ID
    AssessmentID  string `json:"assessment_id"`   // Links to effectiveness_results.id (operational table)
    RemediationID string `json:"remediation_id"`  // Links to remediation request
    ActionType    string `json:"action_type"`     // restart-pod, scale-deployment, etc.
    
    // Assessment Results
    TraditionalScore    float64 `json:"traditional_score"`     // 0.0-1.0 (success/failure rate)
    EnvironmentalImpact float64 `json:"environmental_impact"`  // -1.0 to 1.0 (negative = adverse effect)
    Confidence          float64 `json:"confidence"`            // 0.0-1.0 (data quality indicator)
    
    // Trend Analysis
    TrendDirection        string  `json:"trend_direction"`         // improving, declining, stable, insufficient_data
    RecentSuccessRate     float64 `json:"recent_success_rate,omitempty"`     // Last 30 days success rate
    HistoricalSuccessRate float64 `json:"historical_success_rate,omitempty"` // Last 90 days success rate
    
    // Data Quality
    DataQuality string `json:"data_quality"` // sufficient, limited, insufficient
    SampleSize  int    `json:"sample_size"`  // Number of samples used for assessment
    DataAgeDays int    `json:"data_age_days"` // Age of oldest sample in days
    
    // Pattern Recognition (V2.0 RAR Feature)
    PatternDetected     bool   `json:"pattern_detected"`      // true if temporal/environmental pattern found
    PatternDescription  string `json:"pattern_description,omitempty"`  // Human-readable pattern description
    TemporalPattern     string `json:"temporal_pattern,omitempty"`     // time_of_day, day_of_week, monthly
    
    // Side Effects
    SideEffectsDetected    bool   `json:"side_effects_detected"`              // true if adverse effects found
    SideEffectsDescription string `json:"side_effects_description,omitempty"` // Description of side effects
    
    // Metadata
    CompletedAt time.Time `json:"completed_at"` // Assessment completion timestamp
    CreatedAt   time.Time `json:"created_at"`   // Record creation timestamp
    UpdatedAt   time.Time `json:"updated_at"`   // Record last update timestamp
}

// EffectivenessTrendDirection represents trend analysis results
type EffectivenessTrendDirection string

const (
    TrendImproving        EffectivenessTrendDirection = "improving"
    TrendDeclining        EffectivenessTrendDirection = "declining"
    TrendStable           EffectivenessTrendDirection = "stable"
    TrendInsufficientData EffectivenessTrendDirection = "insufficient_data"
)

// DataQualityLevel represents assessment data quality
type DataQualityLevel string

const (
    DataQualitySufficient   DataQualityLevel = "sufficient"   // ≥10 samples, <30 days old
    DataQualityLimited      DataQualityLevel = "limited"      // 5-9 samples or 30-60 days old
    DataQualityInsufficient DataQualityLevel = "insufficient" // <5 samples or >60 days old
)
```

### **PostgreSQL Table Schema** (Migration 010)

```sql
-- Table 6: effectiveness_audit
-- Service: Effectiveness Monitor
-- Endpoint: POST /api/v1/audit/effectiveness
-- Purpose: Audit trail for effectiveness assessments (V2.0 RAR + 7+ year compliance)
CREATE TABLE IF NOT EXISTS effectiveness_audit (
    id BIGSERIAL PRIMARY KEY,
    
    -- Identity
    assessment_id VARCHAR(255) NOT NULL UNIQUE,
    remediation_id VARCHAR(255) NOT NULL,
    action_type VARCHAR(100) NOT NULL,
    
    -- Assessment results
    traditional_score FLOAT NOT NULL CHECK (traditional_score BETWEEN 0 AND 1),
    environmental_impact FLOAT CHECK (environmental_impact BETWEEN -1 AND 1),
    confidence FLOAT NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    
    -- Trend analysis
    trend_direction VARCHAR(20) CHECK (trend_direction IN ('improving', 'declining', 'stable', 'insufficient_data')),
    recent_success_rate FLOAT CHECK (recent_success_rate BETWEEN 0 AND 1),
    historical_success_rate FLOAT CHECK (historical_success_rate BETWEEN 0 AND 1),
    
    -- Data quality
    data_quality VARCHAR(20) CHECK (data_quality IN ('sufficient', 'limited', 'insufficient')),
    sample_size INTEGER,
    data_age_days INTEGER,
    
    -- Pattern recognition
    pattern_detected BOOLEAN DEFAULT FALSE,
    pattern_description TEXT,
    temporal_pattern VARCHAR(50),
    
    -- Side effects
    side_effects_detected BOOLEAN DEFAULT FALSE,
    side_effects_description TEXT,
    
    -- Metadata
    completed_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for effectiveness_audit
CREATE INDEX IF NOT EXISTS idx_effectiveness_audit_assessment_id ON effectiveness_audit(assessment_id);
CREATE INDEX IF NOT EXISTS idx_effectiveness_audit_remediation_id ON effectiveness_audit(remediation_id);
CREATE INDEX IF NOT EXISTS idx_effectiveness_audit_action_type ON effectiveness_audit(action_type);
CREATE INDEX IF NOT EXISTS idx_effectiveness_audit_completed_at ON effectiveness_audit(completed_at DESC);
CREATE INDEX IF NOT EXISTS idx_effectiveness_audit_trend_direction ON effectiveness_audit(trend_direction);
```

### **Audit Write Trigger Points**

**Trigger**: After effectiveness assessment completes (whether successful or insufficient data)

```go
// pkg/effectivenessmonitor/assessor.go
package effectivenessmonitor

import (
    "context"
    "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
)

type EffectivenessAssessor struct {
    auditClient *audit.Client  // Data Storage Service client
}

func (a *EffectivenessAssessor) AssessRemediation(ctx context.Context, remediationID string) (*Assessment, error) {
    // Step 1: Perform effectiveness assessment (operational logic)
    assessment, err := a.performAssessment(ctx, remediationID)
    if err != nil {
        return nil, err
    }
    
    // Step 2: Write operational assessment to PostgreSQL (direct write - unchanged)
    if err := a.writeOperationalAssessment(ctx, assessment); err != nil {
        return nil, err
    }
    
    // Step 3: Write audit trail to Data Storage Service (NEW - ADR-032 v1.1)
    auditData := &audit.EffectivenessAudit{
        AssessmentID:           assessment.ID,
        RemediationID:          remediationID,
        ActionType:             assessment.ActionType,
        TraditionalScore:       assessment.TraditionalScore,
        EnvironmentalImpact:    calculateEnvironmentalImpact(assessment.Metrics),
        Confidence:             assessment.Confidence,
        TrendDirection:         assessment.TrendDirection,
        RecentSuccessRate:      assessment.RecentSuccessRate,
        HistoricalSuccessRate:  assessment.HistoricalSuccessRate,
        DataQuality:            assessment.DataQuality,
        SampleSize:             assessment.SampleSize,
        DataAgeDays:            assessment.DataAgeDays,
        PatternDetected:        assessment.PatternDetected,
        PatternDescription:     assessment.PatternDescription,
        TemporalPattern:        assessment.TemporalPattern,
        SideEffectsDetected:    assessment.SideEffectsDetected,
        SideEffectsDescription: assessment.SideEffectsDescription,
        CompletedAt:            time.Now(),
    }
    
    // Non-blocking audit write with DLQ fallback (DD-009)
    if err := a.auditClient.WriteEffectivenessAudit(ctx, auditData); err != nil {
        a.logger.Error("Failed to write effectiveness audit", zap.Error(err), zap.String("assessmentID", assessment.ID))
        // DO NOT FAIL assessment - audit is best-effort
    }
    
    return assessment, nil
}
```

### **HTTP Client Integration** (Data Storage Service)

**Endpoint**: `POST /api/v1/audit/effectiveness`

```go
// pkg/datastorage/audit/client.go
package audit

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
)

// WriteEffectivenessAudit writes effectiveness audit to Data Storage Service
func (c *Client) WriteEffectivenessAudit(ctx context.Context, audit *EffectivenessAudit) error {
    url := fmt.Sprintf("%s/api/v1/audit/effectiveness", c.storageServiceURL)
    
    body, err := json.Marshal(audit)
    if err != nil {
        return fmt.Errorf("failed to marshal effectiveness audit: %w", err)
    }
    
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
    if err != nil {
        return fmt.Errorf("failed to create HTTP request: %w", err)
    }
    
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := c.httpClient.Do(req)
    if err != nil {
        // Network error - fallback to DLQ (DD-009)
        return c.fallbackToDLQ(ctx, "effectiveness", audit, err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
        return c.fallbackToDLQ(ctx, "effectiveness", audit, fmt.Errorf("HTTP %d", resp.StatusCode))
    }
    
    return nil
}
```

### **V2.0 RAR Use Case: Effectiveness Trend Analysis**

**Query**: "Find effectiveness trends for similar action types"

```sql
-- V2.0 RAR: Effectiveness analysis over time
SELECT 
    action_type,
    DATE_TRUNC('month', completed_at) AS month,
    AVG(traditional_score) AS avg_traditional_score,
    AVG(environmental_impact) AS avg_environmental_impact,
    AVG(confidence) AS avg_confidence,
    COUNT(*) AS assessment_count,
    COUNT(*) FILTER (WHERE trend_direction = 'improving') AS improving_count,
    COUNT(*) FILTER (WHERE trend_direction = 'declining') AS declining_count,
    COUNT(*) FILTER (WHERE side_effects_detected = true) AS side_effects_count
FROM effectiveness_audit
WHERE action_type = $1  -- e.g., 'restart-pod'
AND completed_at > NOW() - INTERVAL '6 months'
GROUP BY action_type, month
ORDER BY month DESC;
```

**Business Value**: RAR can show "restart-pod effectiveness declined from 0.85 to 0.68 over last 6 months, with increasing side effects"

---

## ✅ **Phase 0 Day 0.2 - Task 3 Complete**

**Deliverable**: ✅ Effectiveness Monitor audit schema completed  
**Validation**: Schema aligns with `effectiveness_audit` table in `migrations/010_audit_write_api.sql`  
**Distinction Clarified**: Audit trail (Data Storage) vs. Operational assessments (direct PostgreSQL)  
**Confidence**: 100%

---

## 🚀 **IMPLEMENTATION PLAN**

### **Phase 0: Documentation Updates** (1-1.5 hours)

**Status**: ✅ **Defined above**

**Tasks**:
1. Update `overview.md` (data access pattern)
2. Update `integration-points.md` (Data Storage Service client)

**Deliverables**:
- ✅ Service specification reflects new architecture

---

### **Day 1: HTTP Client Integration** (3-4 hours)

**Objective**: Integrate Data Storage Service HTTP client for audit reads

**Tasks**:
1. Reuse `pkg/datastorage/client/` package (already created for Context API)
2. Update Effectiveness Monitor's query logic to use HTTP client
3. Keep direct database writes unchanged

**Changes in `pkg/effectivenessmonitor/analyzer.go`** (example):

**Before**:
```go
type EffectivenessAnalyzer struct {
    db *sqlx.DB  // Direct SQL
}

func (a *EffectivenessAnalyzer) AnalyzeRemediation(ctx context.Context, remediationID int64) (*Assessment, error) {
    // Read audit trail directly from PostgreSQL
    var auditData []AuditEvent
    query := "SELECT * FROM resource_action_traces WHERE remediation_id = ?"
    a.db.SelectContext(ctx, &auditData, query, remediationID)

    // Analyze effectiveness
    assessment := a.calculateEffectiveness(auditData)

    // Write assessment directly to PostgreSQL (UNCHANGED)
    a.db.ExecContext(ctx, "INSERT INTO effectiveness_assessments (...) VALUES (...)", ...)

    return assessment, nil
}
```

**After**:
```go
type EffectivenessAnalyzer struct {
    storageClient datastorage.Client  // HTTP client
    db            *sqlx.DB             // For writes only
}

func (a *EffectivenessAnalyzer) AnalyzeRemediation(ctx context.Context, remediationID int64) (*Assessment, error) {
    // Read audit trail via Data Storage Service REST API
    response, err := a.storageClient.ListIncidents(ctx, &datastorage.ListParams{
        RemediationID: &remediationID,
    })
    if err != nil {
        return nil, fmt.Errorf("data storage query failed: %w", err)
    }

    // Analyze effectiveness (UNCHANGED)
    assessment := a.calculateEffectiveness(response.Incidents)

    // Write assessment directly to PostgreSQL (UNCHANGED)
    a.db.ExecContext(ctx, "INSERT INTO effectiveness_assessments (...) VALUES (...)", ...)

    return assessment, nil
}
```

**Code Changes**:
- Remove direct SQL queries for audit reads (~30 lines)
- Add HTTP client calls (~20 lines)
- Keep direct database writes unchanged (~0 lines)

**Deliverables**:
- ✅ Effectiveness Monitor uses Data Storage Service REST API for reads
- ✅ Direct database writes unchanged
- ✅ Unit tests updated

---

### **Day 2: Update Integration Test Infrastructure** (2-3 hours)

**Objective**: Integration tests now start Data Storage Service

**Tasks**:
1. Reuse test helper from Context API migration
2. Update `BeforeSuite()` to start Data Storage Service
3. Verify all integration tests pass

**Changes in `test/integration/effectivenessmonitor/effectiveness_monitor_suite_test.go`**:

**Before**:
```go
var _ = BeforeSuite(func() {
    // Start PostgreSQL
    db = testutil.StartPostgreSQL()
})
```

**After**:
```go
var (
    db            *sqlx.DB
    storageServer *datastorage.Server  // NEW
    storageClient datastorage.Client   // NEW
)

var _ = BeforeSuite(func() {
    // Start PostgreSQL
    db = testutil.StartPostgreSQL()

    // Start Data Storage Service (NEW - reuse from Context API)
    storageServer = datastorage.NewServer(&datastorage.Config{
        DBConnection: db,
        Port:        8082,  // Different port from Context API tests
    })
    go storageServer.Start()
    testutil.WaitForHTTP("http://localhost:8082/health")

    // Create client for Effectiveness Monitor to use
    storageClient = datastorage.NewHTTPClient("http://localhost:8082")
})

var _ = AfterSuite(func() {
    storageServer.Shutdown()  // NEW
    db.Close()
})
```

**Deliverables**:
- ✅ Integration tests updated
- ✅ All tests passing

---

### **Day 3: Validation & Documentation** (1-2 hours)

**Objective**: Final validation and documentation update

**Tasks**:
1. Run full test suite (unit + integration)
2. Manual testing with real Data Storage Service
3. Update service documentation

**Deliverables**:
- ✅ All tests passing
- ✅ Service specification updated
- ✅ **Effectiveness Monitor successfully migrated**

---

## ✅ **SUCCESS CRITERIA**

- ✅ HTTP client for Data Storage Service integrated
- ✅ Effectiveness Monitor reads audit data via REST API
- ✅ Direct database writes unchanged (effectiveness assessments)
- ✅ Integration tests updated (start Data Storage Service)
- ✅ All unit + integration tests passing
- ✅ Service specification updated
- ✅ **Effectiveness Monitor successfully migrated to API Gateway pattern**

---

## 📊 **CODE IMPACT SUMMARY**

| Component | Change | Lines |
|-----------|--------|-------|
| Direct SQL reads (audit data) | **REMOVED** | -30 |
| HTTP client (audit data) | **ADDED** | +20 |
| Direct SQL writes (assessments) | **UNCHANGED** | 0 |
| Integration test infra | **UPDATED** | +40 |
| **Net Change** | | **+30 lines** |

**Confidence**: 95% - Similar to Context API migration, straightforward HTTP client replacement

---

## 🔗 **RELATED DOCUMENTATION**

- [DD-ARCH-001 Final Decision](../../../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md) - Architecture decision
- [Data Storage Service Migration](../../data-storage/implementation/API-GATEWAY-MIGRATION.md) - Phase 1 (dependency)
- [Context API Migration](../../context-api/implementation/API-GATEWAY-MIGRATION.md) - Phase 2 (parallel)

---

**Status**: ✅ **APPROVED - Ready for implementation after Data Storage Service Phase 1**
**Dependencies**: Data Storage Service REST API must be implemented first
**Parallel Work**: Can be done in parallel with Context API migration

