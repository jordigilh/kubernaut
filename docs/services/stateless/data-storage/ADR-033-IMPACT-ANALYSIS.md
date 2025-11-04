# ADR-033 Impact Analysis: Data Storage Service

**Date**: November 4, 2025
**Purpose**: Assess impact of ADR-033 (Remediation Playbook Catalog & Multi-Dimensional Success Tracking) on Data Storage Service
**Scope**: Schema changes, REST API updates, test modifications, Context API integration

---

## 🎯 **EXECUTIVE SUMMARY**

**Impact Level**: **HIGH** - Major schema changes and new REST API endpoints required

**Key Changes Required**:
1. **Schema Migration**: Add 7 new columns for multi-dimensional tracking
2. **REST API**: 3 new aggregation endpoints for playbook success rates
3. **Integration Tests**: Update existing tests + add multi-dimensional tracking tests
4. **Context API**: No breaking changes (additive only)

**Timeline Estimate**: 3-5 days (phased implementation)

**Confidence**: **95%** - Schema design is industry-validated (PagerDuty, BigPanda patterns)

---

## 📊 **CURRENT STATE ANALYSIS**

### **Existing Schema** (BR-STORAGE-001: Signal Audit Trail)
```sql
CREATE TABLE resource_action_traces (
    -- EXISTING COLUMNS (KEEP ALL)
    action_id VARCHAR(64) NOT NULL PRIMARY KEY,
    action_type VARCHAR(50) NOT NULL,
    action_timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
    action_parameters JSONB,

    -- Execution tracking
    status VARCHAR(20) NOT NULL,
    error_message TEXT,

    -- Resource context
    resource_type VARCHAR(50),
    resource_name VARCHAR(255),
    resource_namespace VARCHAR(255),

    -- AI/ML metadata
    model_used VARCHAR(100) NOT NULL,
    model_confidence DECIMAL(4,3) NOT NULL,
    model_reasoning TEXT,

    -- Effectiveness tracking
    effectiveness_score DECIMAL(4,3),
    side_effects_detected BOOLEAN DEFAULT false,

    -- Embeddings for similarity search
    embedding vector(768),

    -- Audit trail
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
```

### **Existing REST API Endpoints**
```
POST   /api/v1/incidents/actions                    # Create action trace
GET    /api/v1/incidents/actions/:id                # Get action trace
GET    /api/v1/incidents/actions                    # List action traces
GET    /api/v1/incidents/aggregate/success-rate     # SUCCESS RATE (workflow_id - DEPRECATED)
```

### **Current Integration Tests** (18 passing)
- ✅ Notification audit CRUD operations
- ✅ Prometheus metrics emission
- ✅ Graceful shutdown
- ✅ Success rate aggregation ⚠️ **(NEEDS UPDATE: workflow_id → incident_type + playbook_id)**

---

## 🔄 **REQUIRED CHANGES - ADR-033 MULTI-DIMENSIONAL TRACKING**

### **1. Schema Migration** (BREAKING: New columns required)

#### **New Columns for `resource_action_traces`**

```sql
-- ========================================
-- ADR-033: MULTI-DIMENSIONAL SUCCESS TRACKING
-- ========================================

ALTER TABLE resource_action_traces ADD COLUMN IF NOT EXISTS
    -- ========================================
    -- DIMENSION 1: INCIDENT TYPE (PRIMARY)
    -- ========================================
    incident_type VARCHAR(100),                      -- NEW: "pod-oom-killer", "high-cpu"
    alert_name VARCHAR(255),                         -- NEW: Prometheus alert name (proxy for incident_type)
    incident_severity VARCHAR(20),                   -- NEW: "critical", "warning", "info"

    -- ========================================
    -- DIMENSION 2: PLAYBOOK (SECONDARY)
    -- ========================================
    playbook_id VARCHAR(64),                         -- NEW: "pod-oom-recovery"
    playbook_version VARCHAR(20),                    -- NEW: "v1.2"
    playbook_step_number INT,                        -- NEW: Step position in playbook (1, 2, 3...)
    playbook_execution_id VARCHAR(64),               -- NEW: Groups all actions in single playbook run

    -- ========================================
    -- AI EXECUTION MODE (HYBRID MODEL)
    -- ========================================
    ai_selected_playbook BOOLEAN DEFAULT false,      -- NEW: Did AI select from playbook catalog?
    ai_chained_playbooks BOOLEAN DEFAULT false,      -- NEW: Did AI chain multiple playbooks?
    ai_manual_escalation BOOLEAN DEFAULT false,      -- NEW: Did AI escalate to human operator?
    ai_playbook_customization JSONB;                 -- NEW: Parameters customized by AI

-- ========================================
-- INDEXES FOR MULTI-DIMENSIONAL QUERIES
-- ========================================

-- Incident-Type Success Rate (PRIMARY dimension)
CREATE INDEX IF NOT EXISTS idx_incident_type_success
ON resource_action_traces(incident_type, status, action_timestamp);

-- Playbook Success Rate (SECONDARY dimension)
CREATE INDEX IF NOT EXISTS idx_playbook_success
ON resource_action_traces(playbook_id, playbook_version, status, action_timestamp);

-- Action-Type Success Rate (TERTIARY dimension)
CREATE INDEX IF NOT EXISTS idx_action_type_success
ON resource_action_traces(action_type, status, action_timestamp);

-- Multi-dimensional composite index
CREATE INDEX IF NOT EXISTS idx_multidimensional_success
ON resource_action_traces(incident_type, playbook_id, action_type, status, action_timestamp);

-- Playbook execution grouping
CREATE INDEX IF NOT EXISTS idx_playbook_execution
ON resource_action_traces(playbook_execution_id, playbook_step_number, action_timestamp);
```

#### **Migration Script**
```bash
# migrations/002_adr033_multidimensional_tracking.sql
# Run via: goose up

-- Safe migration (no data loss)
-- All new columns are nullable for backward compatibility
-- Existing data remains valid (NULL values for new columns)
```

---

### **2. REST API Changes** (NON-BREAKING: Additive only)

#### **New Aggregation Endpoints**

##### **A. Incident-Type Success Rate** (PRIMARY dimension)
```
GET /api/v1/incidents/aggregate/success-rate/by-incident-type
    ?incident_type=pod-oom-killer
    &time_range=7d
    &min_samples=5
```

**Response**:
```json
{
  "incident_type": "pod-oom-killer",
  "time_range": "7d",
  "total_executions": 42,
  "successful_executions": 38,
  "failed_executions": 4,
  "success_rate": 0.905,
  "confidence": "high",
  "min_samples_met": true,
  "breakdown_by_playbook": [
    {
      "playbook_id": "pod-oom-recovery",
      "playbook_version": "v1.2",
      "executions": 35,
      "success_rate": 0.943
    },
    {
      "playbook_id": "pod-oom-recovery",
      "playbook_version": "v1.1",
      "executions": 7,
      "success_rate": 0.714
    }
  ]
}
```

##### **B. Playbook Success Rate** (SECONDARY dimension)
```
GET /api/v1/incidents/aggregate/success-rate/by-playbook
    ?playbook_id=pod-oom-recovery
    &playbook_version=v1.2
    &time_range=30d
    &min_samples=10
```

**Response**:
```json
{
  "playbook_id": "pod-oom-recovery",
  "playbook_version": "v1.2",
  "time_range": "30d",
  "total_executions": 127,
  "successful_executions": 119,
  "failed_executions": 8,
  "success_rate": 0.937,
  "confidence": "high",
  "min_samples_met": true,
  "breakdown_by_incident_type": [
    {
      "incident_type": "pod-oom-killer",
      "executions": 85,
      "success_rate": 0.953
    },
    {
      "incident_type": "container-memory-pressure",
      "executions": 42,
      "success_rate": 0.905
    }
  ],
  "breakdown_by_action": [
    {
      "action_type": "increase_memory",
      "step_number": 1,
      "executions": 127,
      "success_rate": 0.984
    },
    {
      "action_type": "restart_pod",
      "step_number": 2,
      "executions": 127,
      "success_rate": 0.992
    }
  ]
}
```

##### **C. Multi-Dimensional Dashboard Query**
```
GET /api/v1/incidents/aggregate/success-rate/multi-dimensional
    ?incident_type=pod-oom-killer
    &playbook_id=pod-oom-recovery
    &action_type=increase_memory
    &time_range=7d
```

**Response**:
```json
{
  "dimensions": {
    "incident_type": "pod-oom-killer",
    "playbook_id": "pod-oom-recovery",
    "action_type": "increase_memory"
  },
  "time_range": "7d",
  "total_executions": 35,
  "successful_executions": 33,
  "failed_executions": 2,
  "success_rate": 0.943,
  "confidence": "high",
  "ai_execution_mode": {
    "catalog_selected": 32,
    "chained": 3,
    "manual_escalation": 0
  },
  "trend": {
    "direction": "improving",
    "previous_week_success_rate": 0.912,
    "change_percent": 3.4
  }
}
```

#### **Deprecated Endpoint** (KEEP for backward compatibility)
```
GET /api/v1/incidents/aggregate/success-rate?workflow_id=xyz
    # DEPRECATED: workflow_id is meaningless for AI-generated workflows
    # Returns: HTTP 200 with deprecation warning in response header
    # Header: Warning: 299 - "workflow_id query parameter is deprecated. Use /by-incident-type or /by-playbook endpoints"
```

---

### **3. Go Model Changes**

#### **Update `models/notification_audit.go`**
```go
// models/notification_audit.go
type NotificationAudit struct {
    // EXISTING FIELDS (KEEP ALL)
    ActionID            string                 `json:"action_id" db:"action_id"`
    ActionType          string                 `json:"action_type" db:"action_type"`
    ActionTimestamp     time.Time              `json:"action_timestamp" db:"action_timestamp"`
    ActionParameters    map[string]interface{} `json:"action_parameters" db:"action_parameters"`

    Status              string                 `json:"status" db:"status"`
    ErrorMessage        *string                `json:"error_message,omitempty" db:"error_message"`

    // Resource context
    ResourceType        string                 `json:"resource_type" db:"resource_type"`
    ResourceName        string                 `json:"resource_name" db:"resource_name"`
    ResourceNamespace   string                 `json:"resource_namespace" db:"resource_namespace"`

    // AI/ML metadata
    ModelUsed           string                 `json:"model_used" db:"model_used"`
    ModelConfidence     float64                `json:"model_confidence" db:"model_confidence"`
    ModelReasoning      string                 `json:"model_reasoning" db:"model_reasoning"`

    // Effectiveness tracking
    EffectivenessScore  *float64               `json:"effectiveness_score,omitempty" db:"effectiveness_score"`
    SideEffectsDetected bool                   `json:"side_effects_detected" db:"side_effects_detected"`

    // Embeddings
    Embedding           []float32              `json:"embedding,omitempty" db:"embedding"`

    // Audit trail
    CreatedAt           time.Time              `json:"created_at" db:"created_at"`
    UpdatedAt           time.Time              `json:"updated_at" db:"updated_at"`

    // ========================================
    // ADR-033: NEW FIELDS
    // ========================================

    // DIMENSION 1: INCIDENT TYPE (PRIMARY)
    IncidentType        *string                `json:"incident_type,omitempty" db:"incident_type"`
    AlertName           *string                `json:"alert_name,omitempty" db:"alert_name"`
    IncidentSeverity    *string                `json:"incident_severity,omitempty" db:"incident_severity"`

    // DIMENSION 2: PLAYBOOK (SECONDARY)
    PlaybookID          *string                `json:"playbook_id,omitempty" db:"playbook_id"`
    PlaybookVersion     *string                `json:"playbook_version,omitempty" db:"playbook_version"`
    PlaybookStepNumber  *int                   `json:"playbook_step_number,omitempty" db:"playbook_step_number"`
    PlaybookExecutionID *string                `json:"playbook_execution_id,omitempty" db:"playbook_execution_id"`

    // AI EXECUTION MODE (HYBRID MODEL)
    AISelectedPlaybook    bool                 `json:"ai_selected_playbook" db:"ai_selected_playbook"`
    AIChainedPlaybooks    bool                 `json:"ai_chained_playbooks" db:"ai_chained_playbooks"`
    AIManualEscalation    bool                 `json:"ai_manual_escalation" db:"ai_manual_escalation"`
    AIPlaybookCustomization map[string]interface{} `json:"ai_playbook_customization,omitempty" db:"ai_playbook_customization"`
}
```

#### **New Aggregation Response Models**
```go
// models/aggregation_responses.go

// IncidentTypeSuccessRate represents success rate by incident type
type IncidentTypeSuccessRate struct {
    IncidentType         string                     `json:"incident_type"`
    TimeRange            string                     `json:"time_range"`
    TotalExecutions      int                        `json:"total_executions"`
    SuccessfulExecutions int                        `json:"successful_executions"`
    FailedExecutions     int                        `json:"failed_executions"`
    SuccessRate          float64                    `json:"success_rate"`
    Confidence           string                     `json:"confidence"`
    MinSamplesMet        bool                       `json:"min_samples_met"`
    BreakdownByPlaybook  []PlaybookBreakdown        `json:"breakdown_by_playbook"`
}

// PlaybookSuccessRate represents success rate by playbook
type PlaybookSuccessRate struct {
    PlaybookID             string                   `json:"playbook_id"`
    PlaybookVersion        string                   `json:"playbook_version"`
    TimeRange              string                   `json:"time_range"`
    TotalExecutions        int                      `json:"total_executions"`
    SuccessfulExecutions   int                      `json:"successful_executions"`
    FailedExecutions       int                      `json:"failed_executions"`
    SuccessRate            float64                  `json:"success_rate"`
    Confidence             string                   `json:"confidence"`
    MinSamplesMet          bool                     `json:"min_samples_met"`
    BreakdownByIncidentType []IncidentTypeBreakdown `json:"breakdown_by_incident_type"`
    BreakdownByAction       []ActionBreakdown       `json:"breakdown_by_action"`
}

// MultiDimensionalSuccessRate represents success rate across all dimensions
type MultiDimensionalSuccessRate struct {
    Dimensions           Dimensions               `json:"dimensions"`
    TimeRange            string                   `json:"time_range"`
    TotalExecutions      int                      `json:"total_executions"`
    SuccessfulExecutions int                      `json:"successful_executions"`
    FailedExecutions     int                      `json:"failed_executions"`
    SuccessRate          float64                  `json:"success_rate"`
    Confidence           string                   `json:"confidence"`
    AIExecutionMode      AIExecutionModeStats     `json:"ai_execution_mode"`
    Trend                TrendAnalysis            `json:"trend"`
}

// Dimensions represents the three dimensions of success tracking
type Dimensions struct {
    IncidentType string `json:"incident_type"`
    PlaybookID   string `json:"playbook_id"`
    ActionType   string `json:"action_type"`
}

// AIExecutionModeStats tracks AI execution mode distribution
type AIExecutionModeStats struct {
    CatalogSelected  int `json:"catalog_selected"`
    Chained          int `json:"chained"`
    ManualEscalation int `json:"manual_escalation"`
}

// TrendAnalysis shows success rate trends
type TrendAnalysis struct {
    Direction             string  `json:"direction"` // "improving", "stable", "declining"
    PreviousWeekSuccessRate float64 `json:"previous_week_success_rate"`
    ChangePercent         float64 `json:"change_percent"`
}
```

---

### **4. Integration Test Updates**

#### **Existing Tests to Update**

##### **A. Success Rate Aggregation Test** ⚠️ **(HIGH PRIORITY: CURRENTLY FAILING)**
```go
// test/integration/datastorage/aggregation_api_test.go

// ❌ DEPRECATED: workflow_id-based test
It("should calculate success rate correctly with exact counts", func() {
    resp, err := client.Get(fmt.Sprintf("%s/api/v1/incidents/aggregate/success-rate?workflow_id=workflow-agg-1", datastorageURL))
    // This test is architecturally flawed (ADR-033)
})

// ✅ NEW: incident_type-based test
It("should calculate incident-type success rate correctly", func() {
    // Setup: Create 10 pod-oom-killer incidents with 8 successes, 2 failures
    for i := 0; i < 8; i++ {
        createNotificationAudit(models.NotificationAudit{
            IncidentType: stringPtr("pod-oom-killer"),
            Status:       "completed",
            // ... other fields
        })
    }
    for i := 0; i < 2; i++ {
        createNotificationAudit(models.NotificationAudit{
            IncidentType: stringPtr("pod-oom-killer"),
            Status:       "failed",
            // ... other fields
        })
    }

    resp, err := client.Get(fmt.Sprintf("%s/api/v1/incidents/aggregate/success-rate/by-incident-type?incident_type=pod-oom-killer", datastorageURL))
    Expect(err).ToNot(HaveOccurred())
    Expect(resp.StatusCode).To(Equal(200))

    var result models.IncidentTypeSuccessRate
    json.NewDecoder(resp.Body).Decode(&result)

    // BEHAVIOR: Calculates success rate by incident type
    Expect(result.IncidentType).To(Equal("pod-oom-killer"))
    Expect(result.TotalExecutions).To(Equal(10))
    Expect(result.SuccessfulExecutions).To(Equal(8))
    Expect(result.FailedExecutions).To(Equal(2))

    // CORRECTNESS: Success rate is exactly 0.80 (80%)
    Expect(result.SuccessRate).To(BeNumerically("~", 0.80, 0.001))
})
```

#### **New Integration Tests to Add**

##### **B. Playbook Success Rate Test**
```go
It("should calculate playbook success rate across incident types", func() {
    playbookID := "pod-oom-recovery"
    playbookVersion := "v1.2"

    // Setup: pod-oom-recovery v1.2 used for 2 incident types
    // Incident type 1: pod-oom-killer (5 successes)
    for i := 0; i < 5; i++ {
        createNotificationAudit(models.NotificationAudit{
            IncidentType:    stringPtr("pod-oom-killer"),
            PlaybookID:      stringPtr(playbookID),
            PlaybookVersion: stringPtr(playbookVersion),
            Status:          "completed",
        })
    }

    // Incident type 2: container-memory-pressure (3 successes, 2 failures)
    for i := 0; i < 3; i++ {
        createNotificationAudit(models.NotificationAudit{
            IncidentType:    stringPtr("container-memory-pressure"),
            PlaybookID:      stringPtr(playbookID),
            PlaybookVersion: stringPtr(playbookVersion),
            Status:          "completed",
        })
    }
    for i := 0; i < 2; i++ {
        createNotificationAudit(models.NotificationAudit{
            IncidentType:    stringPtr("container-memory-pressure"),
            PlaybookID:      stringPtr(playbookID),
            PlaybookVersion: stringPtr(playbookVersion),
            Status:          "failed",
        })
    }

    resp, err := client.Get(fmt.Sprintf("%s/api/v1/incidents/aggregate/success-rate/by-playbook?playbook_id=%s&playbook_version=%s",
        datastorageURL, playbookID, playbookVersion))
    Expect(err).ToNot(HaveOccurred())
    Expect(resp.StatusCode).To(Equal(200))

    var result models.PlaybookSuccessRate
    json.NewDecoder(resp.Body).Decode(&result)

    // BEHAVIOR: Aggregates across all incident types
    Expect(result.PlaybookID).To(Equal(playbookID))
    Expect(result.PlaybookVersion).To(Equal(playbookVersion))
    Expect(result.TotalExecutions).To(Equal(10))
    Expect(result.SuccessfulExecutions).To(Equal(8))
    Expect(result.FailedExecutions).To(Equal(2))

    // CORRECTNESS: Overall success rate is 0.80
    Expect(result.SuccessRate).To(BeNumerically("~", 0.80, 0.001))

    // CORRECTNESS: Breakdown by incident type
    Expect(result.BreakdownByIncidentType).To(HaveLen(2))

    // Find pod-oom-killer breakdown
    var podOOMBreakdown *models.IncidentTypeBreakdown
    for _, b := range result.BreakdownByIncidentType {
        if b.IncidentType == "pod-oom-killer" {
            podOOMBreakdown = &b
            break
        }
    }
    Expect(podOOMBreakdown).ToNot(BeNil())
    Expect(podOOMBreakdown.Executions).To(Equal(5))
    Expect(podOOMBreakdown.SuccessRate).To(BeNumerically("~", 1.00, 0.001))
})
```

##### **C. AI Execution Mode Tracking Test**
```go
It("should track AI execution mode distribution", func() {
    incidentType := "complex-cascading-failure"

    // Setup: 10 catalog selections, 3 chained, 1 manual escalation
    for i := 0; i < 10; i++ {
        createNotificationAudit(models.NotificationAudit{
            IncidentType:       stringPtr(incidentType),
            AISelectedPlaybook: true,
            Status:             "completed",
        })
    }
    for i := 0; i < 3; i++ {
        createNotificationAudit(models.NotificationAudit{
            IncidentType:       stringPtr(incidentType),
            AIChainedPlaybooks: true,
            Status:             "completed",
        })
    }
    createNotificationAudit(models.NotificationAudit{
        IncidentType:       stringPtr(incidentType),
        AIManualEscalation: true,
        Status:             "completed",
    })

    resp, err := client.Get(fmt.Sprintf("%s/api/v1/incidents/aggregate/success-rate/by-incident-type?incident_type=%s",
        datastorageURL, incidentType))
    Expect(err).ToNot(HaveOccurred())

    var result models.IncidentTypeSuccessRate
    json.NewDecoder(resp.Body).Decode(&result)

    // BEHAVIOR: Tracks AI execution mode distribution
    Expect(result.AIExecutionMode.CatalogSelected).To(Equal(10))
    Expect(result.AIExecutionMode.Chained).To(Equal(3))
    Expect(result.AIExecutionMode.ManualEscalation).To(Equal(1))
})
```

---

### **5. Context API Impact** ✅ **NO BREAKING CHANGES**

#### **Current Context API Usage**
```go
// Context API reads notification audit data via Data Storage REST API
// Current queries: List actions, filter by status, time range

// Example: Get recent actions for incident correlation
GET /api/v1/incidents/actions?time_range=1h&status=completed
```

#### **ADR-033 Impact on Context API** ✅ **ADDITIVE ONLY**

**NO CHANGES REQUIRED** for existing Context API functionality:
- ✅ All new columns are **nullable** (backward compatible)
- ✅ Existing REST API endpoints remain unchanged
- ✅ Context API can continue using current queries

**OPTIONAL ENHANCEMENTS** (future Context API features):
- Context API can optionally query by `incident_type` for better incident correlation
- Context API can optionally query by `playbook_id` for playbook-specific context
- Context API can optionally use multi-dimensional queries for richer context

**Example (Future Enhancement)**:
```go
// Context API can now provide playbook-specific context
func (c *ContextAPI) GetPlaybookContext(playbookID, playbookVersion string) (*PlaybookContext, error) {
    // Query Data Storage for historical success rate
    resp, err := c.dataStorageClient.Get(fmt.Sprintf(
        "/api/v1/incidents/aggregate/success-rate/by-playbook?playbook_id=%s&playbook_version=%s",
        playbookID, playbookVersion))

    // Provide AI with historical context for better decision-making
    return &PlaybookContext{
        PlaybookID:         playbookID,
        HistoricalSuccessRate: result.SuccessRate,
        RecommendedForIncidents: result.BreakdownByIncidentType,
    }, nil
}
```

---

## 📋 **PHASED IMPLEMENTATION PLAN**

### **Phase 1: Schema Migration** (Day 1)
**Goal**: Add new columns without breaking existing functionality

- [ ] Create migration script `002_adr033_multidimensional_tracking.sql`
- [ ] Run migration on development database
- [ ] Verify all existing integration tests still pass (18 tests)
- [ ] Update Go models with new fields (nullable, backward compatible)

**Success Criteria**: All 18 existing integration tests pass without modification

---

### **Phase 2: New REST API Endpoints** (Day 2-3)
**Goal**: Implement 3 new aggregation endpoints

- [ ] Implement `/api/v1/incidents/aggregate/success-rate/by-incident-type`
- [ ] Implement `/api/v1/incidents/aggregate/success-rate/by-playbook`
- [ ] Implement `/api/v1/incidents/aggregate/success-rate/multi-dimensional`
- [ ] Add deprecation warning to legacy `/success-rate?workflow_id=` endpoint
- [ ] Add 3 new integration tests for each endpoint

**Success Criteria**: 21 integration tests pass (18 existing + 3 new)

---

### **Phase 3: Update Existing Tests** (Day 4)
**Goal**: Migrate existing success-rate test to incident-type-based approach

- [ ] Refactor `aggregation_api_test.go` to use `incident_type` instead of `workflow_id`
- [ ] Add AI execution mode tracking test
- [ ] Add playbook chaining test scenario
- [ ] Update test data setup to include `incident_type`, `playbook_id` fields

**Success Criteria**: All 24 integration tests pass with new multi-dimensional approach

---

### **Phase 4: Documentation & OpenAPI Spec** (Day 5)
**Goal**: Update all documentation

- [ ] Update OpenAPI 3.0 spec with 3 new endpoints
- [ ] Update Data Storage implementation plan with ADR-033 references
- [ ] Update Context API documentation (optional enhancements)
- [ ] Add ADR-033 impact summary to Data Storage README

**Success Criteria**: Documentation reflects new multi-dimensional architecture

---

## 🎯 **VALIDATION CHECKLIST**

### **Schema Validation**
- [ ] All new columns are nullable (backward compatible)
- [ ] Indexes created for multi-dimensional queries
- [ ] Migration script tested on development database
- [ ] No data loss during migration

### **API Validation**
- [ ] All 3 new endpoints return correct HTTP status codes
- [ ] Response formats match OpenAPI spec
- [ ] Query parameters validated (time_range, min_samples, etc.)
- [ ] Deprecation warnings added to legacy endpoint

### **Test Validation**
- [ ] All existing tests pass without modification (Phase 1)
- [ ] 3 new integration tests added and passing (Phase 2)
- [ ] Refactored tests use incident-type approach (Phase 3)
- [ ] Test coverage remains >90% for aggregation logic

### **Context API Validation**
- [ ] Existing Context API queries still work
- [ ] No breaking changes to Context API REST client
- [ ] Optional enhancements documented for future use

---

## 🔍 **RISK ASSESSMENT**

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| **Schema migration breaks existing queries** | Low | High | All new columns nullable, existing queries unchanged |
| **Integration tests fail after migration** | Medium | Medium | Run full test suite in Phase 1, rollback if failures |
| **Context API breaks due to new fields** | Very Low | High | New fields are additive, Context API ignores unknown fields |
| **Performance degradation on aggregation queries** | Medium | Medium | Add indexes for multi-dimensional queries, test with 10K+ rows |
| **Legacy workflow_id queries fail** | Low | Low | Keep legacy endpoint with deprecation warning, no hard break |

---

## 📊 **CONFIDENCE ASSESSMENT**

**Overall Confidence**: **95%**

**Justification**:
- ✅ **Schema Design**: Industry-validated (PagerDuty, BigPanda patterns)
- ✅ **Backward Compatibility**: All new columns nullable, no breaking changes
- ✅ **Test Coverage**: Phased approach with validation at each step
- ✅ **Context API Impact**: No breaking changes, additive only
- ✅ **Performance**: Indexes designed for multi-dimensional queries

**Risks (5%)**:
- ⚠️ Migration may take longer than estimated (complex database)
- ⚠️ Performance testing needed with large datasets (10K+ rows)

---

## 🎯 **NEXT STEPS**

1. **Approve Phase 1** (Schema Migration) - Start implementation?
2. **Review Go model changes** - Ensure backward compatibility?
3. **Validate REST API design** - Any changes to endpoint naming?
4. **Context API enhancements** - Should we implement optional features now or defer?

**Recommendation**: Proceed with **Phase 1** (Schema Migration) after approval

