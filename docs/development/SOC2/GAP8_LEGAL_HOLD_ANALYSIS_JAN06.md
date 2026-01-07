# SOC2 Gap #8: Legal Hold & Retention Policies - ANALYSIS

**Date**: January 6, 2026
**Status**: 🔍 **ANALYSIS PHASE**
**Authority**: `AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md` - Day 8
**Compliance Gap**: ❌ **REQUIRED FOR Sarbanes-Oxley, HIPAA, Litigation Hold**

---

## 🎯 **Business Requirement**

**BR-AUDIT-006**: The system MUST provide legal hold capability to prevent deletion of audit events during litigation or regulatory investigation, meeting Sarbanes-Oxley and HIPAA requirements.

---

## 📊 **Current State Assessment**

### **What We Have** ✅
- ✅ PostgreSQL audit_events table with retention_days column (default: 2555 days = 7 years)
- ✅ Date-based partitioning for efficient lifecycle management
- ✅ Configurable retention per event (ADR-034)

### **What's Missing** ❌
- ❌ Legal hold flag to prevent event deletion
- ❌ Legal hold API endpoints (place/release/list holds)
- ❌ Database trigger to enforce legal hold (prevent DELETE)
- ❌ Retention policy table (event_category → retention_days mapping)
- ❌ Automated partition management honoring legal holds

---

## 🔍 **Compliance Requirements**

| Standard | Requirement | Current Status | Gap |
|----------|------------|----------------|-----|
| **Sarbanes-Oxley** | 7-year retention | ⚠️ Configurable (no enforcement) | Legal hold enforcement |
| **HIPAA** | Litigation hold capability | ❌ Not implemented | Full implementation |
| **SOC 2 Type II** | Audit log retention policies | ⚠️ Partial (no automation) | Automated enforcement |
| **ISO 27001** | Retention policy management | ⚠️ Manual only | Automated policies |

**Compliance Score**: **30%** → Target: **90%**

---

## 🏗️ **Technical Architecture**

### **Schema Changes Required**

#### **1. Add Legal Hold Column to audit_events**
```sql
ALTER TABLE audit_events ADD COLUMN legal_hold BOOLEAN DEFAULT FALSE;
CREATE INDEX idx_audit_events_legal_hold ON audit_events(legal_hold)
  WHERE legal_hold = TRUE;  -- Partial index (only TRUE values)
```

**Rationale**:
- Boolean flag prevents accidental deletion
- Partial index minimizes overhead (few events have holds)
- Default FALSE ensures backwards compatibility

---

#### **2. Create audit_retention_policies Table**
```sql
CREATE TABLE audit_retention_policies (
    policy_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_category TEXT NOT NULL UNIQUE,  -- e.g., 'gateway', 'workflow', 'remediation'
    retention_days INTEGER NOT NULL,      -- e.g., 2555 (7 years for SOX)
    legal_hold_override BOOLEAN DEFAULT FALSE,  -- If TRUE, never delete even after retention
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Default policies for SOX compliance
INSERT INTO audit_retention_policies (event_category, retention_days) VALUES
    ('gateway', 2555),        -- 7 years
    ('workflow', 2555),       -- 7 years
    ('remediation', 2555),    -- 7 years
    ('analysis', 2555),       -- 7 years
    ('notification', 2555);   -- 7 years
```

**Rationale**:
- Centralized retention policy per event category
- 2555 days = 7 years (Sarbanes-Oxley requirement)
- `legal_hold_override` for permanent retention (e.g., active litigation)

---

#### **3. Create Legal Hold Enforcement Trigger**
```sql
CREATE OR REPLACE FUNCTION prevent_legal_hold_deletion()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.legal_hold = TRUE THEN
        RAISE EXCEPTION 'Cannot delete audit event with legal hold: event_id=%', OLD.event_id
            USING HINT = 'Release legal hold before deletion',
                  ERRCODE = '23503';  -- foreign_key_violation (closest match)
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER enforce_legal_hold
    BEFORE DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION prevent_legal_hold_deletion();
```

**Rationale**:
- Database-level enforcement (cannot be bypassed by application)
- Explicit error message for compliance audits
- ERRCODE 23503 for consistent error handling

---

### **API Endpoints Required**

#### **1. POST /api/v1/audit/legal-hold** (Place Legal Hold)
```json
{
  "correlation_id": "rr-2026-001",
  "reason": "Litigation: Case #2026-ABC-123",
  "placed_by": "legal-team@company.com",
  "expiration_date": null  // null = indefinite
}
```

**Response**:
```json
{
  "hold_id": "uuid",
  "events_affected": 42,
  "status": "active"
}
```

---

#### **2. DELETE /api/v1/audit/legal-hold/{hold_id}** (Release Legal Hold)
```json
{
  "released_by": "legal-team@company.com",
  "release_reason": "Case settled"
}
```

**Response**:
```json
{
  "hold_id": "uuid",
  "events_released": 42,
  "status": "released"
}
```

---

#### **3. GET /api/v1/audit/legal-hold** (List Active Holds)
**Response**:
```json
{
  "holds": [
    {
      "hold_id": "uuid",
      "correlation_id": "rr-2026-001",
      "events_affected": 42,
      "placed_by": "legal-team@company.com",
      "placed_at": "2026-01-06T10:00:00Z",
      "reason": "Litigation: Case #2026-ABC-123"
    }
  ]
}
```

---

## 📝 **Implementation Tasks**

### **Phase 1: Database Migration** (1 hour)
- [ ] Create migration 024_add_legal_hold.sql
- [ ] Add `legal_hold` column to audit_events
- [ ] Create `audit_retention_policies` table
- [ ] Create `prevent_legal_hold_deletion()` trigger
- [ ] Insert default retention policies (7 years)

### **Phase 2: Legal Hold API** (3 hours)
- [ ] Create legal_hold_handler.go (place/release/list)
- [ ] Implement `PlaceLegalHold()` - UPDATE audit_events SET legal_hold=TRUE
- [ ] Implement `ReleaseLegalHold()` - UPDATE audit_events SET legal_hold=FALSE
- [ ] Implement `ListLegalHolds()` - Query events with legal_hold=TRUE
- [ ] Register endpoints in server router

### **Phase 3: Retention Policy Enforcement** (2 hours)
- [ ] Create retention policy repository
- [ ] Implement partition lifecycle management (honor legal holds)
- [ ] Add cron job / scheduled task for partition cleanup
- [ ] Meta-audit trail for legal hold actions

### **Phase 4: Integration Tests** (1 hour)
- [ ] Test legal hold prevents deletion
- [ ] Test legal hold release allows deletion
- [ ] Test retention policy enforcement
- [ ] Test API endpoints (place/release/list)

---

## 🚨 **Critical Decisions Needed**

### **Q1: Legal Hold Scope**
**Options**:
- **A**: correlation_id-based (all events for a remediation request) ⭐ **Recommended**
- **B**: event_id-based (individual events)
- **C**: date_range-based (all events in timeframe)

**Recommendation**: **A** (correlation_id-based)
- Most common use case: litigation involves entire remediation flow
- Easier to manage (single hold for entire incident)
- Aligns with audit chain verification (Gap #9)

---

### **Q2: Legal Hold Metadata Storage**
**Options**:
- **A**: Store in audit_events table (legal_hold + legal_hold_reason columns) ⭐ **Recommended**
- **B**: Separate audit_legal_holds table (audit trail of holds)

**Recommendation**: **A** (audit_events table) for simplicity
- Minimal schema changes
- Database trigger enforcement
- **Future enhancement**: Add audit_legal_holds table for meta-audit trail (who placed/released, when, why)

---

### **Q3: Retention Policy Enforcement**
**Options**:
- **A**: Manual partition management (DBA runs script monthly)
- **B**: Automated cron job in DataStorage service ⭐ **Recommended**
- **C**: External orchestrator (Kubernetes CronJob)

**Recommendation**: **B** (DataStorage service cron job)
- Honors legal holds automatically
- No external dependencies
- Simple to test and validate

---

### **Q4: Legal Hold Authorization**
**Options**:
- **A**: No authorization (any user can place hold) ❌ **Not Recommended**
- **B**: Authorization via X-User-ID header (requires webhook auth) ⭐ **Recommended**
- **C**: Dedicated legal-hold service with RBAC

**Recommendation**: **B** (X-User-ID header)
- Leverages existing auth webhook (DD-AUTH-003)
- Meta-audit trail captures who placed/released holds
- Simple to implement

---

## 📊 **Effort Estimate**

| Phase | Task | Estimate | Dependencies |
|-------|------|----------|--------------|
| **1** | Database Migration | 1 hour | None |
| **2** | Legal Hold API | 3 hours | Phase 1 |
| **3** | Retention Enforcement | 2 hours | Phase 1 |
| **4** | Integration Tests | 1 hour | Phases 1-3 |
| **Total** | | **7 hours** | ~1 day |

**Confidence**: 85%
- **+10%**: Simple schema changes, clear requirements
- **-15%**: Legal hold authorization integration with webhook

---

## 🎯 **Success Criteria**

### **Functional Requirements**
1. ✅ Legal hold prevents event deletion at database level
2. ✅ API endpoints functional (place/release/list)
3. ✅ Retention policies enforceable per event_category
4. ✅ Meta-audit trail for legal hold actions

### **Compliance Requirements**
1. ✅ Sarbanes-Oxley: 7-year retention enforced
2. ✅ HIPAA: Legal hold capability operational
3. ✅ SOC 2 Type II: Retention policy automation

### **Testing Requirements**
1. ✅ Unit tests: Trigger enforcement
2. ✅ Integration tests: API endpoints
3. ✅ E2E tests: Full legal hold workflow

---

## 🔗 **Dependencies**

### **Existing Components**
- ✅ audit_events table (ADR-034)
- ✅ DataStorage API server
- ✅ AuthWebhook (DD-AUTH-003) for X-User-ID

### **New Components**
- ❌ audit_retention_policies table (new)
- ❌ Legal hold API endpoints (new)
- ❌ Retention enforcement cron job (new)

---

## 📚 **Documentation Requirements**

1. **Migration Guide**: migration 024_add_legal_hold.sql
2. **API Documentation**: Legal hold endpoints in OpenAPI spec
3. **Compliance Guide**: How to use legal hold for SOX/HIPAA
4. **Runbook**: Retention policy management procedures

---

## ⏭️ **Next Steps**

**Immediate**: Get user approval on critical decisions (Q1-Q4)
**Then**: Proceed to PLAN phase with detailed implementation strategy
**Finally**: Implement in TDD sequence (RED → GREEN → REFACTOR)

---

**Document Status**: 🔍 ANALYSIS COMPLETE - Awaiting User Approval
**Created**: 2026-01-06
**Estimated Completion**: 1 day after approval

