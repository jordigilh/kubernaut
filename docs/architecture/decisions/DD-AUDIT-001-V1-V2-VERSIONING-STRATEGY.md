# DD-AUDIT-001 Versioning Strategy: V1.0/V1.1 Foundation → V2.0 RAR Generation

**Status**: ✅ **APPROVED**
**Date**: November 2, 2025
**Related**: DD-AUDIT-001, ADR-032, BR-REMEDIATION-ANALYSIS-001 to BR-REMEDIATION-ANALYSIS-004

---

## 🎯 **Versioning Strategy Overview**

### **V1.0 & V1.1: Foundation (Data Capture Only)**
**Purpose**: Capture COMPLETE audit data for future RAR generation
**Scope**: Data capture and storage (NO report generation)
**Timeline**: Current implementation

### **V2.0: RAR Generation (Reports Only)**
**Purpose**: Generate Remediation Analysis Reports using V1.0/V1.1 data
**Scope**: LLM-powered analysis and report generation
**Timeline**: Future feature
**Critical Constraint**: ✅ **NO database schema changes required**

---

## 🚨 **CRITICAL V1.0/V1.1 REQUIREMENTS**

### 1. Real-Time Audit Writing (Not Just Before Deletion)

**Principle**: Write audit data to database **AS SOON AS POSSIBLE** during execution

**Architecture**:
```
Controller updates CRD status → IMMEDIATELY write to database (async, non-blocking)
                                ↓
                        CRD + Database contain SAME data

During execution (0-24h):
- CRD: Real-time operator visibility (kubectl get, K8s API)
- Database: Permanent record (identical data)

After 24h:
- CRD: DELETED (prevent cluster overload)
- Database: ONLY source (RAR generation)
```

**Why Real-Time Writing**:
- ✅ **Immediate persistence**: Data safe even if CRD force-deleted
- ✅ **Redundancy**: CRD + DB during 24h window (reliability)
- ✅ **Operator choice**: Query CRDs (fast) OR database (rich queries)
- ✅ **No data loss**: Audit persisted immediately, not buffered

**Writing Pattern** (Each Controller):
```go
// Controller reconciliation loop
func (r *AIAnalysisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Update CRD status (real-time operator visibility)
    aiAnalysis.Status.Phase = "Completed"
    aiAnalysis.Status.HolmesGPTResults = results
    aiAnalysis.Status.ApprovalDecision = "approved"
    if err := r.Status().Update(ctx, aiAnalysis); err != nil {
        return ctrl.Result{}, err
    }
    
    // 2. IMMEDIATELY write audit to database (async, non-blocking)
    go func() {
        audit := buildAuditFromCRDStatus(aiAnalysis)
        if err := r.DataStorageClient.WriteAuditAsync(ctx, audit); err != nil {
            r.Log.Error(err, "Failed to write audit", "remediation", aiAnalysis.Name)
            // Retry in next reconcile (finalizer ensures eventual persistence)
        }
    }()
    
    return ctrl.Result{}, nil
}
```

### 2. Forward-Compatible Data Capture

**Principle**: V1.0/V1.1 must capture **ALL** data needed for V2.0 RAR generation, even if not used yet.

### Why This Matters

If V1.0/V1.1 doesn't capture complete data:
- ❌ **Schema Migration Required**: V2.0 needs new fields → database migration nightmare
- ❌ **Data Loss**: Old remediations missing fields → incomplete RARs
- ❌ **Backward Compatibility**: V2.0 can't analyze V1.0 remediations
- ❌ **Production Impact**: Schema changes require downtime

If V1.0/V1.1 captures complete data:
- ✅ **Zero Migration**: V2.0 reads existing data as-is
- ✅ **Complete History**: All remediations analyzable (from day 1)
- ✅ **Smooth Upgrade**: V2.0 deployment = new service only
- ✅ **No Downtime**: Database untouched

---

## 📊 **V1.0/V1.1 Audit Data Requirements**

### What V1.0/V1.1 MUST Capture (Even If Not Used Yet)

#### 1. RemediationOrchestrator Audit
**Database Table**: `orchestration_audit` (or equivalent in Data Storage Service schema)

**Required Fields** (V1.0/V1.1):
```sql
CREATE TABLE orchestration_audit (
    id                      UUID PRIMARY KEY,
    remediation_id          VARCHAR(255) NOT NULL,
    signal_fingerprint      VARCHAR(255) NOT NULL,

    -- Timeline
    signal_received_at      TIMESTAMP NOT NULL,
    remediation_created_at  TIMESTAMP NOT NULL,
    ai_analysis_started_at  TIMESTAMP,
    workflow_started_at     TIMESTAMP,
    notification_sent_at    TIMESTAMP,
    completed_at            TIMESTAMP,

    -- Service coordination
    services_invoked        JSONB,  -- ["AIAnalysis", "WorkflowExecution", "Notification"]
    service_transitions     JSONB,  -- [{"from": "AIAnalysis", "to": "WorkflowExecution", "duration": "2m30s"}]

    -- V2.0 RAR fields (captured but not analyzed in V1.0/V1.1)
    total_duration_seconds  INTEGER,
    bottleneck_phase        VARCHAR(50),  -- "approval_wait", "ai_analysis", etc.
    bottleneck_duration_seconds INTEGER,

    created_at              TIMESTAMP DEFAULT NOW()
);
```

**Why These Fields**: V2.0 RAR needs complete timeline for bottleneck analysis

---

#### 2. AIAnalysis Audit
**Database Table**: `ai_analysis_audit`

**Required Fields** (V1.0/V1.1):
```sql
CREATE TABLE ai_analysis_audit (
    id                      UUID PRIMARY KEY,
    remediation_id          VARCHAR(255) NOT NULL,

    -- Investigation
    investigation_started_at TIMESTAMP NOT NULL,
    investigation_completed_at TIMESTAMP,
    holmesgpt_response_time_ms INTEGER,

    -- AI Decision
    root_cause              TEXT,
    root_cause_confidence   FLOAT,
    recommended_action      VARCHAR(255),
    action_confidence       FLOAT,
    action_rationale        TEXT,

    -- Alternatives (V2.0 RAR needs this for "why not X?" analysis)
    alternatives_considered JSONB,  -- [{"action": "increase-memory", "confidence": 65, "rejected_reason": "capacity"}]

    -- Approval Decision (V2.0 RAR needs this for compliance)
    approval_status         VARCHAR(50),  -- "approved", "rejected", "auto-approved"
    approval_time           TIMESTAMP,
    approval_duration_seconds INTEGER,
    approval_method         VARCHAR(50),  -- "console", "slack", "api"
    approval_justification  TEXT,
    approved_by             VARCHAR(255),
    rejected_by             VARCHAR(255),
    rejection_reason        TEXT,

    created_at              TIMESTAMP DEFAULT NOW()
);
```

**Why These Fields**: V2.0 RAR analyzes AI decision quality and approval patterns

---

#### 3. WorkflowExecution Audit
**Database Table**: `workflow_execution_audit`

**Required Fields** (V1.0/V1.1):
```sql
CREATE TABLE workflow_execution_audit (
    id                      UUID PRIMARY KEY,
    remediation_id          VARCHAR(255) NOT NULL,
    workflow_id             VARCHAR(255) NOT NULL,

    -- Execution timeline
    execution_started_at    TIMESTAMP NOT NULL,
    execution_completed_at  TIMESTAMP,
    total_duration_seconds  INTEGER,

    -- Step details (V2.0 RAR needs step-by-step analysis)
    steps_executed          JSONB,  -- [{"step": 1, "action": "restart-pod", "duration": 5, "status": "success", ...}]
    total_steps             INTEGER,
    steps_succeeded         INTEGER,
    steps_failed            INTEGER,
    retries_performed       INTEGER,

    -- Validation results (V2.0 RAR needs this for effectiveness)
    pre_conditions_passed   BOOLEAN,
    post_conditions_passed  BOOLEAN,
    validation_results      JSONB,

    -- Outcome (V2.0 RAR needs this for success rate analysis)
    outcome                 VARCHAR(50),  -- "success", "failure", "partial"
    effectiveness_score     FLOAT,
    rollbacks_performed     INTEGER,

    -- Adaptive adjustments (V2.0 RAR needs this for optimization)
    adaptive_adjustments    JSONB,

    created_at              TIMESTAMP DEFAULT NOW()
);
```

**Why These Fields**: V2.0 RAR analyzes remediation effectiveness and execution patterns

---

#### 4. Notification Audit
**Database Table**: `notification_audit`

**Required Fields** (V1.0/V1.1):
```sql
CREATE TABLE notification_audit (
    id                      UUID PRIMARY KEY,
    remediation_id          VARCHAR(255) NOT NULL,

    -- Notification timeline
    notification_requested_at TIMESTAMP NOT NULL,
    notification_sent_at    TIMESTAMP,
    delivery_confirmed_at   TIMESTAMP,

    -- Delivery details
    channels                JSONB,  -- ["slack", "console"]
    delivery_attempts       JSONB,  -- [{"channel": "slack", "attempt": 1, "status": "success", "duration": 250}]
    delivery_status         VARCHAR(50),  -- "success", "partial", "failed"

    created_at              TIMESTAMP DEFAULT NOW()
);
```

**Why These Fields**: V2.0 RAR includes notification delivery in end-to-end timeline

---

## ✅ **V1.0/V1.1 Implementation Checklist**

### Data Storage Service Schema
- [ ] Create tables: `orchestration_audit`, `ai_analysis_audit`, `workflow_execution_audit`, `notification_audit`
- [ ] Include ALL V2.0 RAR fields (even if not analyzed yet)
- [ ] Add indexes on `remediation_id` for RAR query performance
- [ ] Add indexes on timestamps for timeline queries

### REST API Endpoints (V1.0/V1.1)
- [ ] `POST /api/v1/audit/orchestration` - Write orchestration audit
- [ ] `POST /api/v1/audit/ai-decisions` - Write AI analysis audit
- [ ] `POST /api/v1/audit/executions` - Write workflow execution audit
- [ ] `POST /api/v1/audit/notifications` - Write notification audit
- [ ] **NO GET endpoints yet** (V2.0 feature)

### Controller Implementation
- [ ] RemediationOrchestrator: Write orchestration audit **AS SOON AS** status changes (real-time)
- [ ] AIAnalysis Controller: Write AI decision audit **AS SOON AS** analysis completes (real-time)
- [ ] WorkflowExecution Controller: Write execution audit **AS SOON AS** each step completes (real-time)
- [ ] Notification Controller: Write notification audit **AS SOON AS** delivery confirmed (real-time)
- [ ] All controllers use **async writes** (non-blocking, don't delay reconciliation)
- [ ] All controllers use **finalizers** to verify audit written before CRD deletion (eventual consistency guarantee)

### Real-Time Audit Pattern
```go
// Pattern: Update CRD → IMMEDIATELY write audit (async)
func (r *Controller) updateStatusAndWriteAudit(ctx context.Context, obj *CRD, status Status) error {
    // 1. Update CRD status (operator visibility)
    obj.Status = status
    if err := r.Status().Update(ctx, obj); err != nil {
        return err
    }
    
    // 2. Write audit IMMEDIATELY (async, non-blocking)
    go r.writeAuditAsync(ctx, obj)
    
    return nil
}
```

### Audit Persistence Guarantee
**Finalizer Pattern**: Ensures audit written before CRD deletion (eventual consistency)
```go
func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // During execution: async audit writes
    // Before deletion: finalizer ensures audit persisted
    
    if obj.DeletionTimestamp != nil {
        // Finalizer: Block deletion until audit verified in database
        if !r.auditPersistedInDB(ctx, obj) {
            return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
        }
        // Remove finalizer → allow deletion
        return r.removeFinalizer(ctx, obj)
    }
    
    return ctrl.Result{}, nil
}
```

---

## 🔄 **V2.0 Implementation (Future - NO Schema Changes)**

### New Service: RAR Generator
- [ ] New microservice: `rar-generator-service` (or add to existing service)
- [ ] Read audit data from Data Storage Service
- [ ] LLM integration for analysis
- [ ] Report generation and formatting

### REST API Endpoints (V2.0 - NEW)
- [ ] `GET /api/v2/rar/{remediation_id}` - Generate RAR for specific remediation
- [ ] `GET /api/v2/rar?start=X&end=Y` - Generate RARs for date range
- [ ] `POST /api/v2/rar/batch` - Batch RAR generation

### NO Database Changes
- ✅ **Read V1.0/V1.1 audit tables as-is**
- ✅ **No new tables**
- ✅ **No schema migrations**
- ✅ **No production downtime**

---

## 📊 **V1.0/V1.1 vs V2.0 Comparison**

| Capability | V1.0 & V1.1 | V2.0 |
|------------|-------------|------|
| **Capture orchestration audit** | ✅ YES | ✅ YES (same) |
| **Capture AI decision audit** | ✅ YES | ✅ YES (same) |
| **Capture execution audit** | ✅ YES | ✅ YES (same) |
| **Capture notification audit** | ✅ YES | ✅ YES (same) |
| **Generate RAR** | ❌ NO | ✅ YES (NEW) |
| **LLM analysis** | ❌ NO | ✅ YES (NEW) |
| **Timeline reconstruction** | ❌ NO | ✅ YES (NEW) |
| **Effectiveness analysis** | ❌ NO | ✅ YES (NEW) |
| **Database schema** | ✅ Forward-compatible | ✅ NO CHANGES |

---

## 🎯 **Success Criteria**

### V1.0/V1.1 Success
- ✅ **All audit data captured**: Every controller writes complete audit
- ✅ **Forward-compatible schema**: All V2.0 RAR fields present
- ✅ **No data loss**: Audit written before CRD deletion
- ✅ **Production stable**: No schema changes in V2.0

### V2.0 Success
- ✅ **Zero migration**: RAR service deploys without database changes
- ✅ **Complete history**: RARs generated for all V1.0/V1.1 remediations
- ✅ **No downtime**: Seamless upgrade from V1.0/V1.1 to V2.0

---

## 📋 **Summary**

**V1.0 & V1.1**: Capture complete audit data (foundation)
**V2.0**: Generate RARs using V1.0/V1.1 data (no schema changes)

**Key Principle**: **Capture everything in V1.0/V1.1, analyze in V2.0**

**Critical Requirement**: ✅ NO database schema changes between V1.0/V1.1 and V2.0

**This versioning strategy ensures smooth V2.0 deployment with zero migration risk.**

