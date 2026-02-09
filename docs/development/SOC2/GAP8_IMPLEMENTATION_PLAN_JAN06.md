# SOC2 Gap #8: Legal Hold & Retention - IMPLEMENTATION PLAN

**Date**: January 6, 2026
**Status**: 📋 **PLAN PHASE**
**Authority**: `AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md` - Day 8
**Approved Decisions**: A (correlation_id), A (audit_events table), B (service cron), B (X-User-ID auth)

---

## 🎯 **Implementation Strategy**

### **APDC Phases**
1. ✅ **Analysis**: Complete (GAP8_LEGAL_HOLD_ANALYSIS_JAN06.md)
2. 🔄 **Plan**: This document
3. ⏳ **Do**: TDD phases (RED → GREEN → REFACTOR)
4. ⏳ **Check**: Validation and compliance verification

---

## 📋 **Phase 1: Database Migration (1 hour)**

### **Task 1.1: Create Migration 024**
**File**: `migrations/024_add_legal_hold.sql`

**Schema Changes**:
```sql
-- Step 1: Add legal_hold column to audit_events
ALTER TABLE audit_events ADD COLUMN legal_hold BOOLEAN DEFAULT FALSE;

-- Step 2: Create partial index (only TRUE values for performance)
CREATE INDEX idx_audit_events_legal_hold ON audit_events(legal_hold)
  WHERE legal_hold = TRUE;

-- Step 3: Add columns for legal hold metadata
ALTER TABLE audit_events ADD COLUMN legal_hold_reason TEXT;
ALTER TABLE audit_events ADD COLUMN legal_hold_placed_by TEXT;
ALTER TABLE audit_events ADD COLUMN legal_hold_placed_at TIMESTAMP;

-- Step 4: Create audit_retention_policies table
CREATE TABLE audit_retention_policies (
    policy_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_category TEXT NOT NULL UNIQUE,
    retention_days INTEGER NOT NULL,
    legal_hold_override BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Step 5: Insert default SOX-compliant retention policies (7 years = 2555 days)
INSERT INTO audit_retention_policies (event_category, retention_days) VALUES
    ('gateway', 2555),
    ('workflow', 2555),
    ('remediation', 2555),
    ('analysis', 2555),
    ('notification', 2555);

-- Step 6: Create trigger to prevent deletion of events with legal hold
CREATE OR REPLACE FUNCTION prevent_legal_hold_deletion()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.legal_hold = TRUE THEN
        RAISE EXCEPTION 'Cannot delete audit event with legal hold: event_id=%, correlation_id=%',
            OLD.event_id, OLD.correlation_id
            USING HINT = 'Release legal hold before deletion',
                  ERRCODE = '23503';
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER enforce_legal_hold
    BEFORE DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION prevent_legal_hold_deletion();
```

**Validation**:
```bash
# Test migration up/down
goose -dir migrations postgres "..." up
goose -dir migrations postgres "..." down
```

---

## 📋 **Phase 2: Legal Hold API (3 hours)**

### **Task 2.1: Create Legal Hold Handler**
**File**: `pkg/datastorage/server/legal_hold_handler.go`

**Endpoints**:
1. `POST /api/v1/audit/legal-hold` - Place legal hold
2. `DELETE /api/v1/audit/legal-hold/{correlation_id}` - Release legal hold
3. `GET /api/v1/audit/legal-hold` - List active legal holds

**Request/Response Models**:
```go
// PlaceLegalHoldRequest represents a legal hold placement request
type PlaceLegalHoldRequest struct {
    CorrelationID string `json:"correlation_id" validate:"required"`
    Reason        string `json:"reason" validate:"required"`
    PlacedBy      string `json:"placed_by"`  // From X-User-ID header
}

// PlaceLegalHoldResponse represents the result of placing a legal hold
type PlaceLegalHoldResponse struct {
    CorrelationID   string    `json:"correlation_id"`
    EventsAffected  int       `json:"events_affected"`
    PlacedBy        string    `json:"placed_by"`
    PlacedAt        time.Time `json:"placed_at"`
    Reason          string    `json:"reason"`
}

// ReleaseLegalHoldRequest represents a legal hold release request
type ReleaseLegalHoldRequest struct {
    ReleaseReason string `json:"release_reason" validate:"required"`
    ReleasedBy    string `json:"released_by"`  // From X-User-ID header
}

// ReleaseLegalHoldResponse represents the result of releasing a legal hold
type ReleaseLegalHoldResponse struct {
    CorrelationID   string    `json:"correlation_id"`
    EventsReleased  int       `json:"events_released"`
    ReleasedBy      string    `json:"released_by"`
    ReleasedAt      time.Time `json:"released_at"`
}

// LegalHold represents an active legal hold
type LegalHold struct {
    CorrelationID   string    `json:"correlation_id"`
    EventsAffected  int       `json:"events_affected"`
    PlacedBy        string    `json:"placed_by"`
    PlacedAt        time.Time `json:"placed_at"`
    Reason          string    `json:"reason"`
}

// ListLegalHoldsResponse represents a list of active legal holds
type ListLegalHoldsResponse struct {
    Holds []LegalHold `json:"holds"`
    Total int         `json:"total"`
}
```

**Handler Implementation**:
```go
// HandlePlaceLegalHold places a legal hold on all events with a correlation_id
func (s *Server) HandlePlaceLegalHold(w http.ResponseWriter, r *http.Request) {
    // 1. Parse request body
    // 2. Extract X-User-ID header (placed_by)
    // 3. Validate correlation_id exists
    // 4. Update all events: SET legal_hold=TRUE, legal_hold_reason, placed_by, placed_at
    // 5. Return count of affected events
    // 6. Meta-audit log: "legal_hold.placed"
}

// HandleReleaseLegalHold releases a legal hold
func (s *Server) HandleReleaseLegalHold(w http.ResponseWriter, r *http.Request) {
    // 1. Extract correlation_id from URL path
    // 2. Parse request body (release_reason)
    // 3. Extract X-User-ID header (released_by)
    // 4. Update events: SET legal_hold=FALSE
    // 5. Return count of released events
    // 6. Meta-audit log: "legal_hold.released"
}

// HandleListLegalHolds lists all active legal holds
func (s *Server) HandleListLegalHolds(w http.ResponseWriter, r *http.Request) {
    // 1. Query: SELECT DISTINCT correlation_id, legal_hold_* FROM audit_events WHERE legal_hold=TRUE
    // 2. For each correlation_id, count events
    // 3. Return list with metadata
}
```

### **Task 2.2: Register Endpoints**
**File**: `pkg/datastorage/server/server.go`

```go
// SOC2 Gap #8: Legal Hold & Retention Policies
// BR-AUDIT-006: Legal hold capability for Sarbanes-Oxley and HIPAA compliance
s.logger.V(1).Info("Registering /api/v1/audit/legal-hold handlers (SOC2 Gap #8)")
r.Post("/api/v1/audit/legal-hold", s.HandlePlaceLegalHold)
r.Delete("/api/v1/audit/legal-hold/{correlation_id}", s.HandleReleaseLegalHold)
r.Get("/api/v1/audit/legal-hold", s.HandleListLegalHolds)
```

---

## 📋 **Phase 3: Retention Policy Repository (2 hours)**

### **Task 3.1: Create Retention Policy Repository**
**File**: `pkg/datastorage/repository/retention_policy_repository.go`

**Interface**:
```go
type RetentionPolicyRepository interface {
    GetByCategory(ctx context.Context, category string) (*RetentionPolicy, error)
    ListAll(ctx context.Context) ([]*RetentionPolicy, error)
    Update(ctx context.Context, policy *RetentionPolicy) error
}

type RetentionPolicy struct {
    PolicyID          uuid.UUID
    EventCategory     string
    RetentionDays     int
    LegalHoldOverride bool
    CreatedAt         time.Time
    UpdatedAt         time.Time
}
```

### **Task 3.2: Implement Retention Enforcement (Deferred)**
**Note**: Automated partition cleanup is **deferred to v1.1**
- Reason: Requires careful testing with production data
- Workaround: Manual DBA scripts for v1.0
- Legal hold enforcement is **immediate** (database trigger)

---

## 📋 **Phase 4: Integration Tests (1 hour)**

### **Task 4.1: Legal Hold Enforcement Tests**
**File**: `test/integration/datastorage/legal_hold_integration_test.go`

**Test Cases**:
```go
var _ = Describe("Legal Hold Integration Tests", func() {
    Describe("BR-AUDIT-006: Legal Hold Enforcement", func() {
        It("should prevent deletion of events with legal hold", func() {
            // 1. Create audit events
            // 2. Place legal hold via API
            // 3. Attempt DELETE FROM audit_events WHERE correlation_id=X
            // 4. Expect database error: "Cannot delete audit event with legal hold"
        })

        It("should allow deletion after legal hold release", func() {
            // 1. Create audit events with legal hold
            // 2. Release legal hold via API
            // 3. DELETE FROM audit_events WHERE correlation_id=X
            // 4. Expect success
        })
    })

    Describe("POST /api/v1/audit/legal-hold", func() {
        It("should place legal hold on all events with correlation_id", func() {
            // 1. Create 5 audit events with same correlation_id
            // 2. POST /api/v1/audit/legal-hold
            // 3. Verify response: events_affected=5
            // 4. Query DB: SELECT COUNT(*) WHERE correlation_id=X AND legal_hold=TRUE
            // 5. Expect count=5
        })

        It("should return 400 if correlation_id not found", func() {
            // 1. POST with non-existent correlation_id
            // 2. Expect 400 Bad Request
        })

        It("should capture X-User-ID in placed_by field", func() {
            // 1. POST with X-User-ID: legal-team@company.com
            // 2. Query DB: legal_hold_placed_by
            // 3. Expect "legal-team@company.com"
        })
    })

    Describe("DELETE /api/v1/audit/legal-hold/{correlation_id}", func() {
        It("should release legal hold on all events", func() {
            // 1. Create events with legal hold
            // 2. DELETE /api/v1/audit/legal-hold/{correlation_id}
            // 3. Verify response: events_released=5
            // 4. Query DB: legal_hold=FALSE
        })
    })

    Describe("GET /api/v1/audit/legal-hold", func() {
        It("should list all active legal holds", func() {
            // 1. Place holds on 3 different correlation_ids
            // 2. GET /api/v1/audit/legal-hold
            // 3. Expect 3 holds in response
        })
    })
})
```

---

## 📊 **Timeline & Dependencies**

| Phase | Tasks | Duration | Dependencies | Deliverables |
|-------|-------|----------|--------------|--------------|
| **1** | Database Migration | 1 hour | None | migration 024, trigger |
| **2** | Legal Hold API | 3 hours | Phase 1 | 3 endpoints, handler |
| **3** | Retention Repository | 2 hours | Phase 1 | Repository interface |
| **4** | Integration Tests | 1 hour | Phases 1-3 | Test coverage |
| **Total** | | **7 hours** | | **~1 day** |

---

## 🎯 **Success Criteria**

### **Functional Requirements**
- ✅ Legal hold prevents event deletion at database level (trigger)
- ✅ API endpoints functional (place/release/list)
- ✅ X-User-ID captured in legal_hold_placed_by
- ✅ Meta-audit trail for legal hold actions

### **Compliance Requirements**
- ✅ Sarbanes-Oxley: 7-year retention policy defined
- ✅ HIPAA: Legal hold capability operational
- ✅ SOC 2 Type II: Legal hold API documented

### **Testing Requirements**
- ✅ Integration tests: Legal hold enforcement
- ✅ Integration tests: API endpoints (place/release/list)
- ✅ Integration tests: Authorization (X-User-ID)

---

## 🔒 **Security Considerations**

### **Authorization**
- X-User-ID header required (from AuthWebhook)
- Legal hold actions logged in meta-audit trail
- Only authorized users (legal team) should have access

### **Audit Trail**
- Who placed the hold? → `legal_hold_placed_by`
- When was it placed? → `legal_hold_placed_at`
- Why was it placed? → `legal_hold_reason`
- Who released it? → Logged in meta-audit event

---

## 🚀 **Implementation Order (TDD)**

### **Phase DO-RED: Write Failing Tests** (1.5 hours)
1. Write integration test: "should prevent deletion with legal hold"
2. Write integration test: "POST /api/v1/audit/legal-hold"
3. Write integration test: "DELETE /api/v1/audit/legal-hold/{correlation_id}"
4. Write integration test: "GET /api/v1/audit/legal-hold"
5. Run tests → **FAIL** (expected - no implementation yet)

### **Phase DO-GREEN: Minimal Implementation** (3 hours)
1. Apply migration 024 (legal_hold column, trigger, policies table)
2. Create legal_hold_handler.go with minimal logic
3. Register endpoints in server.go
4. Run tests → **PASS**

### **Phase DO-REFACTOR: Enhance Implementation** (2.5 hours)
1. Add retention policy repository
2. Add meta-audit logging for legal hold actions
3. Add comprehensive error handling
4. Add OpenAPI documentation
5. Run tests → **PASS**

---

## 📚 **Documentation Updates**

### **Files to Create**
1. ✅ `migrations/024_add_legal_hold.sql` - Database schema
2. ✅ `pkg/datastorage/server/legal_hold_handler.go` - API handlers
3. ✅ `pkg/datastorage/repository/retention_policy_repository.go` - Repository
4. ✅ `test/integration/datastorage/legal_hold_integration_test.go` - Tests
5. ✅ `docs/development/SOC2/GAP8_LEGAL_HOLD_COMPLETE_JAN06.md` - Completion doc

### **Files to Update**
1. ✅ `pkg/datastorage/server/server.go` - Register endpoints
2. ✅ `pkg/datastorage/repository/audit_events_repository.go` - Add legal_hold field to AuditEvent struct

---

## ⚠️ **Risks & Mitigations**

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| **Trigger breaks existing deletes** | High | Low | Integration tests before deployment |
| **Performance impact of legal hold queries** | Medium | Medium | Partial index (WHERE legal_hold=TRUE) |
| **Authorization bypass** | High | Low | X-User-ID validation mandatory |
| **Retention automation complexity** | Medium | Medium | Defer to v1.1 (manual for v1.0) |

---

## ✅ **Approval Status**

- ✅ **Q1**: correlation_id-based holds (APPROVED)
- ✅ **Q2**: legal_hold column in audit_events (APPROVED)
- ✅ **Q3**: DataStorage service cron (APPROVED - deferred to v1.1)
- ✅ **Q4**: X-User-ID authorization (APPROVED)

---

## ⏭️ **Next Steps**

1. **Immediate**: Start Phase DO-RED (write failing integration tests)
2. **Then**: Phase DO-GREEN (minimal implementation)
3. **Finally**: Phase DO-REFACTOR (enhance + document)

**Ready to proceed!** 🚀

---

**Document Status**: 📋 PLAN COMPLETE - Ready for Implementation
**Created**: 2026-01-06
**Estimated Completion**: 7 hours (~1 day)
**Confidence**: 85%



