# Audit Trace Master Specification - All Controllers

**Version**: 1.0
**Date**: 2025-11-03
**Status**: ✅ **ROADMAP COMPLETE** - Comprehensive audit strategy for all 6 controllers
**Purpose**: Define audit trace requirements for all controllers to guide TDD-aligned implementation
**Authority**: ADR-032 v1.3 - Data Access Layer Isolation (Phased Audit Table Development)

---

## 📋 **Executive Summary**

This master specification defines the audit trace requirements for **all 6 controllers/services** in the Kubernaut platform, serving as the comprehensive roadmap for implementing audit persistence via the Data Storage Service. Each controller will follow the **Notification Controller pilot pattern** established in Phase 1.

**Phased Implementation Strategy**:
- ✅ **Phase 1 (Pilot)**: Notification Controller - Validates audit architecture (4.5 days)
- ⏸️ **Phase 2 (TDD-Aligned)**: Remaining 5 controllers - Implemented during controller TDD

**Key Principles**:
1. **Real-Time Audit Writes**: Audit data written immediately after CRD status updates
2. **Non-Blocking**: Audit write failures do NOT block business logic
3. **DLQ Fallback**: Failed audit writes go to Dead Letter Queue (DD-009)
4. **Single Source of Truth**: Audit data matches CRD status fields exactly
5. **Append-Only**: Audit data is immutable once written

---

## 🎯 **Controller Implementation Status**

| Controller | CRD Status | Controller Status | Audit Table | Phase | Timeline |
|-----------|-----------|-------------------|-------------|-------|----------|
| **Notification** | ✅ Implemented | ✅ Operational | `notification_audit` | **Phase 1 (Pilot)** | Week 1 |
| **RemediationProcessor** | ⚠️ Placeholder | ❌ Not Implemented | `signal_processing_audit` | Phase 2 | Week 3-4 |
| **RemediationOrchestrator** | ⚠️ Placeholder | ❌ Not Implemented | `orchestration_audit` | Phase 2 | Week 5-6 |
| **AIAnalysis** | ⚠️ Placeholder | ❌ Not Implemented | `ai_analysis_audit` | Phase 2 | Week 7-8 |
| **WorkflowExecution** | ⚠️ Placeholder | ❌ Not Implemented | `workflow_execution_audit` | Phase 2 | Week 9-10 |
| **EffectivenessMonitor** | ❌ No Service | ❌ Not Implemented | `effectiveness_audit` | Phase 2 | Week 11-12 |

---

## 📊 **1. Notification Controller** ✅ **PHASE 1 PILOT**

### **Status**: ✅ Fully Implemented Controller

**Detailed Specification**: [notification/audit-trace-specification.md](../services/crd-controllers/06-notification/audit-trace-specification.md)

### **Audit Triggers** (4 Status Transitions)

| Trigger | CRD Status Transition | Audit Status | Business Value |
|---------|----------------------|--------------|----------------|
| **Sent** | `pending` → `sent` | `sent` | Confirms successful delivery |
| **Failed** | `pending/sending` → `failed` | `failed` | Enables debugging & retry |
| **Acknowledged** | `sent` → `acknowledged` | `acknowledged` | Tracks SLA compliance |
| **Escalated** | `sent/failed` → `escalated` | `escalated` | Tracks escalation events |

### **Key CRD Status Fields**

```go
type NotificationRequestStatus struct {
    Status          NotificationStatus  // sent, failed, acknowledged, escalated
    DeliveryStatus  string             // Provider response
    ErrorMessage    string             // Failure details
    SentAt          *metav1.Time       // Send timestamp
    AcknowledgedAt  *metav1.Time       // Acknowledgment timestamp
    EscalationLevel int                // Escalation count
}
```

### **Audit Table Schema**

```sql
CREATE TABLE notification_audit (
    id BIGSERIAL PRIMARY KEY,
    remediation_id VARCHAR(255) NOT NULL,
    notification_id VARCHAR(255) NOT NULL UNIQUE,
    recipient VARCHAR(255) NOT NULL,
    channel VARCHAR(50) NOT NULL,
    message_summary TEXT NOT NULL,
    status VARCHAR(50) NOT NULL CHECK (status IN ('sent', 'failed', 'acknowledged', 'escalated')),
    sent_at TIMESTAMP WITH TIME ZONE NOT NULL,
    delivery_status TEXT,
    error_message TEXT,
    escalation_level INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

**Endpoint**: `POST /api/v1/audit/notifications`

---

## 📊 **2. RemediationProcessor Controller** ⏸️ **PHASE 2**

### **Status**: ⚠️ CRD Placeholder, Controller Not Implemented

**CRD Authority**: `api/remediationprocessing/v1alpha1/remediationprocessing_types.go`

### **Expected Audit Triggers** (Based on CRD Placeholder)

| Trigger | Expected CRD Status Transition | Audit Status | Business Value |
|---------|-------------------------------|--------------|----------------|
| **Enrichment Started** | `pending` → `enriching` | `enriching` | Tracks context gathering start |
| **Enrichment Complete** | `enriching` → `enriched` | `enriched` | Tracks enrichment quality |
| **Classification Complete** | `enriched` → `classified` | `classified` | Tracks environment classification |
| **CRD Created** | `classified` → `completed` | `completed` | Tracks child CRD creation |
| **Failed** | `*` → `failed` | `failed` | Tracks processing failures |

### **Expected CRD Status Fields** (To Be Finalized During TDD)

```go
// PLACEHOLDER - Will be finalized during controller TDD
type RemediationProcessingStatus struct {
    Phase                   string                // Current phase
    EnrichmentResult        *EnrichmentResult     // Historical context data
    ClassificationDecision  *ClassificationDecision // Classification output
    CreatedCRDName          string                // Child CRD name (AIAnalysis or WorkflowExecution)
    CreatedCRDType          string                // "AIAnalysis" or "WorkflowExecution"
    ErrorMessage            string                // Failure details
    StartTime               *metav1.Time          // Processing start
    CompletedAt             *metav1.Time          // Processing completion
}
```

### **Expected Audit Table Schema** (To Be Created in Migration 011)

```sql
-- Migration 011: signal_processing_audit (Created during RemediationProcessor TDD)
CREATE TABLE signal_processing_audit (
    id BIGSERIAL PRIMARY KEY,
    remediation_id VARCHAR(255) NOT NULL,
    signal_id VARCHAR(255) NOT NULL UNIQUE,  -- RemediationProcessing CRD name
    alert_fingerprint VARCHAR(255) NOT NULL,
    environment VARCHAR(50) NOT NULL,  -- prod, staging, dev
    severity VARCHAR(50) NOT NULL,  -- critical, warning, info
    status VARCHAR(50) NOT NULL,  -- received, enriched, classified, completed, failed
    processing_start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    processing_end_time TIMESTAMP WITH TIME ZONE,
    processing_duration_ms INTEGER,
    enrichment_quality FLOAT,  -- 0.0-1.0
    classification_score FLOAT,  -- 0.0-1.0
    context_data JSONB,  -- Enriched context data
    created_crd_name VARCHAR(255),  -- Child CRD name
    created_crd_type VARCHAR(50),  -- AIAnalysis or WorkflowExecution
    error_message TEXT,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

**Endpoint**: `POST /api/v1/audit/signal-processing`

**TDD Integration Point**: Audit table schema will be finalized during controller TDD GREEN phase based on actual CRD status fields.

---

## 📊 **3. RemediationOrchestrator Controller** ⏸️ **PHASE 2**

### **Status**: ⚠️ CRD Placeholder (RemediationRequest), Controller Partially Implemented

**CRD Authority**: `api/remediation/v1alpha1/remediationrequest_types.go`

### **Expected Audit Triggers** (Based on Orchestration Workflow)

| Trigger | Expected CRD Status Transition | Audit Status | Business Value |
|---------|-------------------------------|--------------|----------------|
| **Processing Started** | `pending` → `processing` | `processing` | Tracks RemediationProcessing creation |
| **AI Analysis Started** | `processing` → `analyzing` | `analyzing` | Tracks AIAnalysis creation |
| **Workflow Execution Started** | `analyzing` → `executing` | `executing` | Tracks WorkflowExecution creation |
| **Remediation Complete** | `executing` → `completed` | `completed` | Tracks successful remediation |
| **Timeout** | `*` → `timeout` | `timeout` | Tracks phase timeouts |
| **Failed** | `*` → `failed` | `failed` | Tracks orchestration failures |

### **Expected CRD Status Fields** (To Be Finalized During TDD)

```go
// PLACEHOLDER - Will be finalized during controller TDD
type RemediationRequestStatus struct {
    OverallPhase string  // pending, processing, analyzing, executing, completed, failed, timeout

    // Child CRD references
    RemediationProcessingRef *CRDReference
    AIAnalysisRef            *CRDReference
    WorkflowExecutionRef     *CRDReference

    // Service CRD statuses (JSONB for flexibility)
    ServiceCRDStatuses map[string]interface{}

    // Timestamps
    StartTime      metav1.Time
    CompletionTime *metav1.Time

    // Timeout/Failure tracking
    TimeoutPhase  string
    FailurePhase  string
    FailureReason string
}
```

### **Expected Audit Table Schema** (To Be Created in Migration 012)

```sql
-- Migration 012: orchestration_audit (Created during RemediationOrchestrator TDD)
CREATE TABLE orchestration_audit (
    id BIGSERIAL PRIMARY KEY,
    remediation_id VARCHAR(255) NOT NULL UNIQUE,  -- RemediationRequest CRD name
    alert_fingerprint VARCHAR(255) NOT NULL,
    remediation_name VARCHAR(255) NOT NULL,
    overall_phase VARCHAR(50) NOT NULL,  -- pending, processing, analyzing, executing, completed, failed, timeout
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    completion_time TIMESTAMP WITH TIME ZONE,
    remediation_processing_name VARCHAR(255),  -- Link to RemediationProcessing CRD
    ai_analysis_name VARCHAR(255),  -- Link to AIAnalysis CRD
    workflow_execution_name VARCHAR(255),  -- Link to WorkflowExecution CRD
    service_crd_statuses JSONB,  -- Stores status of related CRDs
    timeout_phase VARCHAR(50),  -- Phase where timeout occurred
    failure_phase VARCHAR(50),  -- Phase where failure occurred
    failure_reason TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

**Endpoint**: `POST /api/v1/audit/orchestration`

**TDD Integration Point**: Audit table schema will be finalized during controller TDD GREEN phase based on actual orchestration workflow and CRD status fields.

---

## 📊 **4. AIAnalysis Controller** ⏸️ **PHASE 2**

### **Status**: ⚠️ CRD Placeholder, Controller Not Implemented

**CRD Authority**: `api/aianalysis/v1alpha1/aianalysis_types.go`

### **Expected Audit Triggers** (Based on AI Analysis Workflow)

| Trigger | Expected CRD Status Transition | Audit Status | Business Value |
|---------|-------------------------------|--------------|----------------|
| **Investigation Started** | `pending` → `investigating` | `investigating` | Tracks AI investigation start |
| **Investigation Complete** | `investigating` → `investigated` | `investigated` | Tracks root cause findings |
| **Analysis Started** | `investigated` → `analyzing` | `analyzing` | Tracks AI analysis start |
| **Analysis Complete** | `analyzing` → `analyzed` | `analyzed` | Tracks confidence score |
| **Recommendation Generated** | `analyzed` → `recommending` | `recommending` | Tracks workflow recommendation |
| **Recommendation Complete** | `recommending` → `completed` | `completed` | Tracks successful AI analysis |
| **Hallucination Detected** | `*` → `hallucination_detected` | `hallucination_detected` | Tracks AI hallucinations |
| **Failed** | `*` → `failed` | `failed` | Tracks AI analysis failures |

### **Expected CRD Status Fields** (To Be Finalized During TDD)

```go
// PLACEHOLDER - Will be finalized during controller TDD
type AIAnalysisStatus struct {
    Phase string  // pending, investigating, investigated, analyzing, analyzed, recommending, completed, failed

    // Investigation results
    InvestigationStartTime *metav1.Time
    InvestigationEndTime   *metav1.Time
    RootCauseCount         int
    InvestigationReport    string  // Summary of AI's findings

    // Analysis results
    AnalysisStartTime *metav1.Time
    AnalysisEndTime   *metav1.Time
    ConfidenceScore   float64  // 0.0-1.0
    HallucinationDetected bool

    // Recommendation results
    RecommendationStartTime *metav1.Time
    RecommendationEndTime   *metav1.Time
    Recommendations         []Recommendation
    TopRecommendation       string
    EffectivenessProbability float64
    HistoricalSuccessRate   float64

    // Workflow CRD reference (if AI triggered workflow)
    WorkflowCRDName      string
    WorkflowCRDNamespace string

    // Completion
    CompletionStatus string  // completed, failed, pending_human_review
    FailureReason    string
}
```

### **Expected Audit Table Schema** (To Be Created in Migration 013)

```sql
-- Migration 013: ai_analysis_audit (Created during AIAnalysis TDD)
CREATE TABLE ai_analysis_audit (
    id BIGSERIAL PRIMARY KEY,
    remediation_id VARCHAR(255) NOT NULL,
    ai_analysis_id VARCHAR(255) NOT NULL UNIQUE,  -- AIAnalysis CRD name
    alert_fingerprint VARCHAR(255) NOT NULL,
    environment VARCHAR(50) NOT NULL,
    severity VARCHAR(50) NOT NULL,

    -- Investigation phase
    investigation_start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    investigation_end_time TIMESTAMP WITH TIME ZONE,
    investigation_duration_ms INTEGER,
    root_cause_count INTEGER,
    investigation_report TEXT,  -- Summary of AI's findings

    -- Analysis phase
    analysis_start_time TIMESTAMP WITH TIME ZONE,
    analysis_end_time TIMESTAMP WITH TIME ZONE,
    analysis_duration_ms INTEGER,
    confidence_score FLOAT NOT NULL CHECK (confidence_score BETWEEN 0 AND 1),
    hallucination_detected BOOLEAN DEFAULT FALSE,

    -- Recommendation phase
    recommendation_start_time TIMESTAMP WITH TIME ZONE,
    recommendation_end_time TIMESTAMP WITH TIME ZONE,
    recommendations JSONB,  -- Array of recommended actions
    top_recommendation TEXT,
    effectiveness_probability FLOAT,
    historical_success_rate FLOAT,

    -- Workflow CRD reference
    workflow_crd_name VARCHAR(255),
    workflow_crd_namespace VARCHAR(255),

    -- Completion
    completion_status VARCHAR(50) NOT NULL,  -- completed, failed, pending_human_review
    failure_reason TEXT,

    -- pgvector embedding for semantic search (Decision 1a: AIAnalysis only)
    embedding vector(1536),

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- pgvector index for semantic search
CREATE INDEX IF NOT EXISTS idx_ai_analysis_audit_embedding ON ai_analysis_audit USING hnsw (embedding vector_cosine_ops);
```

**Endpoint**: `POST /api/v1/audit/ai-decisions`

**Special Feature**: Includes `embedding vector(1536)` column for V2.0 Remediation Analysis Report (RAR) semantic search.

**TDD Integration Point**: Audit table schema will be finalized during controller TDD GREEN phase based on actual AI analysis workflow and CRD status fields.

---

## 📊 **5. WorkflowExecution Controller** ⏸️ **PHASE 2**

### **Status**: ⚠️ CRD Placeholder, Controller Not Implemented

**CRD Authority**: `api/workflowexecution/v1alpha1/workflowexecution_types.go`

### **Expected Audit Triggers** (Based on Workflow Execution)

| Trigger | Expected CRD Status Transition | Audit Status | Business Value |
|---------|-------------------------------|--------------|----------------|
| **Execution Started** | `pending` → `running` | `running` | Tracks workflow start |
| **Step Completed** | `running` → `running` | `running` | Tracks step progress |
| **Execution Complete** | `running` → `completed` | `completed` | Tracks successful execution |
| **Rollback Started** | `running` → `rolling_back` | `rolling_back` | Tracks rollback initiation |
| **Rollback Complete** | `rolling_back` → `rolled_back` | `rolled_back` | Tracks rollback completion |
| **Partial Success** | `running` → `partial_success` | `partial_success` | Tracks partial completions |
| **Failed** | `*` → `failed` | `failed` | Tracks execution failures |

### **Expected CRD Status Fields** (To Be Finalized During TDD)

```go
// PLACEHOLDER - Will be finalized during controller TDD
type WorkflowExecutionStatus struct {
    Phase string  // pending, running, completed, failed, rolling_back, rolled_back, partial_success

    // Execution tracking
    WorkflowName    string
    WorkflowVersion string
    TotalSteps      int
    StepsCompleted  int
    StepsFailed     int
    TotalDurationMs int

    // Outcome
    Outcome            string  // success, failure, rollback, partial_success
    EffectivenessScore float64  // 0.0-1.0
    RollbacksPerformed int

    // Step execution details
    StepExecutions []StepExecution  // Details of each step's execution

    // Adaptive adjustments
    AdaptiveAdjustments []AdaptiveAdjustment  // Details of any adaptive changes made

    // Completion
    CompletedAt   *metav1.Time
    Status        string  // running, completed, failed, paused
    ErrorMessage  string
}
```

### **Expected Audit Table Schema** (To Be Created in Migration 014)

```sql
-- Migration 014: workflow_execution_audit (Created during WorkflowExecution TDD)
CREATE TABLE workflow_execution_audit (
    id BIGSERIAL PRIMARY KEY,
    remediation_id VARCHAR(255) NOT NULL,
    workflow_execution_id VARCHAR(255) NOT NULL UNIQUE,  -- WorkflowExecution CRD name
    workflow_name VARCHAR(255) NOT NULL,
    workflow_version VARCHAR(50) NOT NULL,
    total_steps INTEGER NOT NULL,
    steps_completed INTEGER NOT NULL,
    steps_failed INTEGER NOT NULL,
    total_duration_ms INTEGER,
    outcome VARCHAR(50) NOT NULL,  -- success, failure, rollback, partial_success
    effectiveness_score FLOAT,  -- 0.0-1.0
    rollbacks_performed INTEGER DEFAULT 0,
    step_executions JSONB,  -- Details of each step's execution
    adaptive_adjustments JSONB,  -- Details of any adaptive changes made
    completed_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(50) NOT NULL,  -- running, completed, failed, paused
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

**Endpoint**: `POST /api/v1/audit/executions`

**TDD Integration Point**: Audit table schema will be finalized during controller TDD GREEN phase based on actual workflow execution logic and CRD status fields.

---

## 📊 **6. EffectivenessMonitor Service** ⏸️ **PHASE 2**

### **Status**: ❌ No Service (Business logic exists in `pkg/ai/insights/`, no HTTP wrapper)

**Business Logic Authority**: `pkg/ai/insights/service.go`, `pkg/ai/insights/assessor.go`

### **Expected Audit Triggers** (Based on Effectiveness Assessment)

| Trigger | Expected Assessment Event | Audit Status | Business Value |
|---------|--------------------------|--------------|----------------|
| **Assessment Complete** | Assessment finished | `completed` | Tracks effectiveness score |
| **Trend Detected** | Trend analysis complete | `completed` | Tracks improving/declining trends |
| **Pattern Detected** | Pattern recognition complete | `completed` | Tracks temporal/environmental patterns |
| **Side Effects Detected** | Side effect analysis complete | `completed` | Tracks adverse effects |
| **Assessment Failed** | Assessment error | `failed` | Tracks assessment failures |

### **Expected Service Data Structure** (To Be Finalized During Service TDD)

```go
// PLACEHOLDER - Will be finalized during service TDD
type EffectivenessAssessment struct {
    // Identity
    AssessmentID   string
    RemediationID  string
    ActionType     string

    // Assessment results
    TraditionalScore      float64  // 0.0-1.0
    EnvironmentalImpact   float64  // -1.0 to 1.0
    Confidence            float64  // 0.0-1.0

    // Trend analysis
    TrendDirection        string  // improving, declining, stable, insufficient_data
    RecentSuccessRate     float64
    HistoricalSuccessRate float64

    // Data quality
    DataQuality string  // sufficient, limited, insufficient
    SampleSize  int
    DataAgeDays int

    // Pattern recognition
    PatternDetected     bool
    PatternDescription  string
    TemporalPattern     string

    // Side effects
    SideEffectsDetected     bool
    SideEffectsDescription  string

    // Metadata
    CompletedAt time.Time
}
```

### **Expected Audit Table Schema** (To Be Created in Migration 015)

```sql
-- Migration 015: effectiveness_audit (Created during EffectivenessMonitor service TDD)
CREATE TABLE effectiveness_audit (
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
```

**Endpoint**: `POST /api/v1/audit/effectiveness`

**TDD Integration Point**: Audit table schema will be finalized during service TDD GREEN phase based on actual effectiveness assessment logic and data structures.

---

## 🔄 **Common Audit Integration Pattern**

All controllers follow this standard pattern (established by Notification Controller pilot):

### **1. Audit Client Initialization**

```go
type ControllerReconciler struct {
    client.Client
    Scheme      *runtime.Scheme
    Log         logr.Logger
    auditClient *datastorage.AuditClient  // Data Storage audit client
}

func (r *ControllerReconciler) SetupWithManager(mgr ctrl.Manager) error {
    auditServiceURL := os.Getenv("DATA_STORAGE_SERVICE_URL")
    if auditServiceURL == "" {
        auditServiceURL = "http://data-storage-service.kubernaut-system.svc.cluster.local:8080"
    }

    r.auditClient = datastorage.NewAuditClient(auditServiceURL, r.Log)

    return ctrl.NewControllerManagedBy(mgr).
        For(&v1.CRD{}).
        Named("controller-name").
        Complete(r)
}
```

### **2. Audit Write in Reconcile Loop**

```go
func (r *ControllerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Store old status for comparison
    oldStatus := crd.Status.Phase

    // ... (business logic) ...

    // Update CRD status
    if err := r.Status().Update(ctx, crd); err != nil {
        return ctrl.Result{}, err
    }

    // Write audit trace AFTER CRD status update (non-blocking)
    if crd.Status.Phase != oldStatus {
        auditData := r.buildAuditData(crd, oldStatus)

        go func() {
            if err := r.auditClient.WriteAudit(ctx, auditData); err != nil {
                r.Log.Error(err, "Failed to write audit (DLQ fallback triggered)",
                    "crdName", crd.Name,
                    "phase", crd.Status.Phase)
            }
        }()
    }

    return ctrl.Result{}, nil
}
```

### **3. Audit Data Transformation**

```go
func (r *ControllerReconciler) buildAuditData(crd *v1.CRD, oldStatus string) *datastorage.Audit {
    return &datastorage.Audit{
        // Map CRD fields to audit table columns
        // Single source of truth: CRD status → audit data
    }
}
```

---

## 📊 **Audit Write API Endpoints Summary**

| Endpoint | Controller | Audit Table | Phase | Status |
|----------|-----------|-------------|-------|--------|
| `POST /api/v1/audit/notifications` | Notification | `notification_audit` | **Phase 1** | ✅ Ready |
| `POST /api/v1/audit/signal-processing` | RemediationProcessor | `signal_processing_audit` | Phase 2 | ⏸️ TDD-Aligned |
| `POST /api/v1/audit/orchestration` | RemediationOrchestrator | `orchestration_audit` | Phase 2 | ⏸️ TDD-Aligned |
| `POST /api/v1/audit/ai-decisions` | AIAnalysis | `ai_analysis_audit` | Phase 2 | ⏸️ TDD-Aligned |
| `POST /api/v1/audit/executions` | WorkflowExecution | `workflow_execution_audit` | Phase 2 | ⏸️ TDD-Aligned |
| `POST /api/v1/audit/effectiveness` | EffectivenessMonitor | `effectiveness_audit` | Phase 2 | ⏸️ TDD-Aligned |

---

## 🎯 **Implementation Timeline**

| Week | Controller | Activity | Deliverables |
|------|-----------|----------|--------------|
| **Week 1** | Notification | Phase 1 Pilot | Data Storage API + Notification enhancement |
| **Week 3-4** | RemediationProcessor | Controller TDD | Controller + Migration 011 + Enhancement |
| **Week 5-6** | RemediationOrchestrator | Controller TDD | Controller + Migration 012 + Enhancement |
| **Week 7-8** | AIAnalysis | Controller TDD | Controller + Migration 013 + Enhancement |
| **Week 9-10** | WorkflowExecution | Controller TDD | Controller + Migration 014 + Enhancement |
| **Week 11-12** | EffectivenessMonitor | Service TDD | Service + Migration 015 + Enhancement |

---

## ✅ **Success Criteria**

| Criterion | Target | Validation Method |
|-----------|--------|-------------------|
| **Audit Write Success Rate** | >99% per controller | Prometheus metrics |
| **DLQ Fallback Rate** | <1% per controller | Prometheus metrics |
| **Audit Data Accuracy** | 100% (CRD-audit consistency) | Integration tests |
| **Non-Blocking Guarantee** | 100% (business logic not impacted) | E2E tests |
| **Schema Accuracy** | 100% (matches actual CRD fields) | TDD validation |

---

## 📚 **Related Documentation**

- [ADR-032 v1.3: Data Access Layer Isolation](./decisions/ADR-032-data-access-layer-isolation.md)
- [DD-009: Audit Write Error Recovery (DLQ)](./decisions/DD-009-audit-write-error-recovery.md)
- [Data Storage Implementation Plan V4.8](../services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V4.8.md)
- [Notification Controller Audit Trace Specification](../services/crd-controllers/06-notification/audit-trace-specification.md) ← **PILOT**
- [Notification Controller Enhancement Plan V0.1](../services/crd-controllers/06-notification/implementation/ENHANCEMENT_PLAN_V0.1.md) ← **TO BE CREATED**

---

## 🚀 **Next Steps**

1. ✅ **This Document**: Master audit specification complete
2. ⏸️ **Notification Enhancement Plan**: Create TDD plan for Notification Controller audit integration
3. ⏸️ **Data Storage Write API**: Implement Phase 1 (notification_audit endpoint)
4. ⏸️ **Notification Controller Enhancement**: Add audit writes to Notification Controller
5. ⏸️ **Phase 2 Controllers**: Implement remaining 5 controllers with TDD-aligned audit tables

---

**Confidence**: 100% for Phase 1 (Notification), 80% for Phase 2 (will reach 100% during controller TDD)
**Status**: ✅ Roadmap complete, ready for implementation

