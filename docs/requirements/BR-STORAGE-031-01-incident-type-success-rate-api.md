# BR-STORAGE-031-01: Incident-Type Success Rate API

**Business Requirement ID**: BR-STORAGE-031-01
**Category**: Data Storage Service
**Priority**: P0
**Target Version**: V1
**Status**: ✅ Approved
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 introduces **Multi-Dimensional Success Tracking** for remediation playbooks. The Data Storage Service must expose a REST API to calculate success rates aggregated by **incident type** (the PRIMARY dimension for AI learning).

**Current Limitations**:
- ❌ No API endpoint to query success rates by incident type
- ❌ AI Service cannot learn from historical effectiveness by incident type
- ❌ No visibility into which incident types have proven remediation patterns
- ❌ Legacy `workflow_id` endpoint is architecturally invalid (each AI-generated workflow is unique)

**Impact**:
- AI cannot select playbooks based on historical success rates for specific incident types
- Operators lack data-driven insights for incident remediation effectiveness
- System cannot track continuous improvement of remediation strategies
- Missing foundation for ADR-033 Hybrid Model (90% catalog selection + 9% chaining + 1% manual)

---

## 🎯 **Business Objective**

**Enable Context API and AI Service to query incident-type-specific success rates for data-driven playbook selection.**

### **Success Criteria**
1. ✅ REST API endpoint exposes incident-type success rate aggregation
2. ✅ Response includes total executions, success/failure counts, and success rate (0.0-1.0)
3. ✅ Supports time range filtering (7d, 30d, 90d)
4. ✅ Includes playbook breakdown (which playbooks were used for this incident type)
5. ✅ Includes AI execution mode statistics (catalog/chained/manual)
6. ✅ Returns confidence level based on minimum samples threshold
7. ✅ Response time <200ms for typical queries (7d window, <10K records)

---

## 📊 **Use Cases**

### **Use Case 1: AI Playbook Selection Based on Incident Type**

**Scenario**: AI receives `pod-oom-killer` incident and needs to select the best playbook based on historical success rates.

**Current Flow** (Without BR-STORAGE-031-01):
```
1. AI receives pod-oom-killer alert
2. AI lacks historical success rate data by incident type
3. AI uses generic heuristics or random selection
4. ❌ No data-driven playbook selection
5. ❌ Suboptimal remediation success rate
```

**Desired Flow with BR-STORAGE-031-01**:
```
1. AI receives pod-oom-killer alert
2. AI queries Data Storage: GET /api/v1/incidents/aggregate/success-rate/by-incident-type?incident_type=pod-oom-killer&time_range=7d
3. Response: {
     "incident_type": "pod-oom-killer",
     "success_rate": 0.85,
     "total_executions": 100,
     "breakdown_by_playbook": [
       {"playbook_id": "pod-oom-recovery", "playbook_version": "v1.2", "success_rate": 0.89}
     ]
   }
4. AI selects pod-oom-recovery v1.2 (highest success rate)
5. ✅ Data-driven playbook selection
6. ✅ Improved remediation success rate
```

---

### **Use Case 2: Operator Dashboard - Incident Type Effectiveness**

**Scenario**: SRE wants to see which incident types have proven remediation patterns vs which need manual investigation.

**Current Flow**:
```
1. Operator checks dashboard
2. No incident-type success rate data available
3. ❌ Cannot identify high-success vs low-success incident types
4. ❌ Cannot prioritize playbook development efforts
```

**Desired Flow with BR-STORAGE-031-01**:
```
1. Operator checks dashboard
2. Dashboard queries Data Storage for all incident types
3. Dashboard shows:
   - pod-oom-killer: 85% success (proven playbook)
   - high-cpu-utilization: 40% success (needs improvement)
   - database-connection-timeout: 10% success (requires manual investigation)
4. ✅ Clear prioritization for playbook development
5. ✅ Data-driven remediation strategy improvements
```

---

### **Use Case 3: Continuous Improvement - Trend Analysis**

**Scenario**: Team wants to track if new playbook versions are improving incident-type success rates over time.

**Current Flow**:
```
1. Team deploys new playbook version
2. No historical comparison available
3. ❌ Cannot measure improvement
4. ❌ Cannot validate new playbook effectiveness
```

**Desired Flow with BR-STORAGE-031-01**:
```
1. Team deploys pod-oom-recovery v1.2
2. Query success rate for last 7d (v1.2): 0.89
3. Query success rate for last 30d (mixed v1.1/v1.2): 0.75
4. ✅ Measurable improvement: +14% success rate
5. ✅ Data-driven validation of new playbook version
```

---

## 🔧 **Functional Requirements**

### **FR-STORAGE-031-01-01: REST API Endpoint**

**Requirement**: Data Storage Service SHALL expose a REST API endpoint to calculate success rate by incident type.

**Endpoint Specification**:
```http
GET /api/v1/incidents/aggregate/success-rate/by-incident-type

Query Parameters:
- incident_type (string, required): Incident type (e.g., "pod-oom-killer")
- time_range (string, optional, default: "7d"): Time window (7d, 30d, 90d)
- min_samples (int, optional, default: 5): Minimum executions required for high confidence

Response (200 OK):
{
  "incident_type": "pod-oom-killer",
  "time_range": "7d",
  "total_executions": 100,
  "successful_executions": 85,
  "failed_executions": 15,
  "success_rate": 0.85,
  "confidence": "high",  // "high" | "medium" | "low" | "insufficient_data"
  "min_samples_met": true,
  "breakdown_by_playbook": [
    {
      "playbook_id": "pod-oom-recovery",
      "playbook_version": "v1.2",
      "executions": 90,
      "success_rate": 0.89
    },
    {
      "playbook_id": "pod-oom-recovery",
      "playbook_version": "v1.1",
      "executions": 10,
      "success_rate": 0.50
    }
  ],
  "ai_execution_mode": {
    "catalog_selected": 90,
    "chained": 9,
    "manual_escalation": 1
  }
}
```

**Acceptance Criteria**:
- ✅ Endpoint returns 200 OK for valid incident types
- ✅ Endpoint returns 400 Bad Request for missing incident_type parameter
- ✅ Endpoint returns 400 Bad Request for invalid time_range values
- ✅ Success rate is exactly `successful_executions / total_executions`
- ✅ Playbook breakdown sums to total executions
- ✅ AI execution mode stats sum to total executions
- ✅ Confidence level calculated based on min_samples threshold

---

### **FR-STORAGE-031-01-02: SQL Aggregation Query**

**Requirement**: Data Storage Service SHALL execute efficient SQL aggregation queries using new ADR-033 schema columns.

**SQL Implementation**:
```sql
-- Primary query: Incident-type success rate
SELECT
    incident_type,
    COUNT(*) AS total_executions,
    SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS successful_executions,
    SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) AS failed_executions,
    CAST(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS FLOAT) / NULLIF(COUNT(*), 0) AS success_rate
FROM resource_action_traces
WHERE incident_type = $1
  AND action_timestamp >= NOW() - INTERVAL '7 days'
GROUP BY incident_type;

-- Playbook breakdown subquery
SELECT
    playbook_id,
    playbook_version,
    COUNT(*) AS executions,
    CAST(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS FLOAT) / NULLIF(COUNT(*), 0) AS success_rate
FROM resource_action_traces
WHERE incident_type = $1
  AND action_timestamp >= NOW() - INTERVAL '7 days'
GROUP BY playbook_id, playbook_version
ORDER BY success_rate DESC;

-- AI execution mode stats
SELECT
    SUM(CASE WHEN ai_selected_playbook THEN 1 ELSE 0 END) AS catalog_selected,
    SUM(CASE WHEN ai_chained_playbooks THEN 1 ELSE 0 END) AS chained,
    SUM(CASE WHEN ai_manual_escalation THEN 1 ELSE 0 END) AS manual_escalation
FROM resource_action_traces
WHERE incident_type = $1
  AND action_timestamp >= NOW() - INTERVAL '7 days';
```

**Acceptance Criteria**:
- ✅ Queries use indexes on `incident_type` and `action_timestamp`
- ✅ Query execution time <50ms for typical datasets (<10K records in window)
- ✅ Handle zero executions gracefully (return 0.0 success rate, not division by zero error)
- ✅ Handle null incident_type values (exclude from aggregation)

---

### **FR-STORAGE-031-01-03: Confidence Level Calculation**

**Requirement**: API response SHALL include confidence level based on sample size.

**Confidence Level Logic**:
```
- insufficient_data: total_executions < min_samples
- low: min_samples <= total_executions < 20
- medium: 20 <= total_executions < 50
- high: total_executions >= 50
```

**Acceptance Criteria**:
- ✅ `min_samples_met` flag is `false` when `total_executions < min_samples`
- ✅ Confidence level accurately reflects thresholds
- ✅ Configurable min_samples threshold (default: 5, max: 100)

---

## 📈 **Non-Functional Requirements**

### **NFR-STORAGE-031-01-01: Performance**

- ✅ Response time <200ms for 95th percentile queries
- ✅ Support 100 concurrent requests per second
- ✅ Query optimization via indexes on `incident_type`, `action_timestamp`

### **NFR-STORAGE-031-01-02: Scalability**

- ✅ Handle datasets with 1M+ historical execution records
- ✅ Time range filtering prevents full table scans
- ✅ Pagination not required (aggregation results are always compact)

### **NFR-STORAGE-031-01-03: Data Accuracy**

- ✅ Success rate calculations are mathematically precise (no rounding errors >0.001)
- ✅ Aggregation results are consistent with raw data
- ✅ No data loss or corruption during aggregation

---

## 🔗 **Dependencies**

### **Upstream Dependencies**
- ✅ ADR-033: Remediation Playbook Catalog (architectural decision)
- ✅ BR-STORAGE-031-03: Schema migration (7 new columns must exist)
- ✅ Migration script `002_adr033_multidimensional_tracking.sql` applied

### **Downstream Impacts**
- ✅ BR-INTEGRATION-008: Context API must consume this endpoint
- ✅ BR-AI-057: AI Service must use success rates for playbook selection
- ✅ BR-EFFECTIVENESS-001: Effectiveness Monitor must poll this endpoint

---

## 🚀 **Implementation Phases**

### **Phase 1: Handler Implementation** (Day 13 - 4 hours)
- Implement `handleGetSuccessRateByIncidentType` HTTP handler
- Add parameter parsing and validation
- Add RFC 7807 error responses

### **Phase 2: Repository Layer** (Day 13 - 4 hours)
- Implement `GetSuccessRateByIncidentType` repository method
- Add SQL aggregation queries
- Add confidence level calculation logic

### **Phase 3: Unit Tests** (Day 14 - 2 hours)
- Test success rate calculation accuracy
- Test edge cases (zero executions, 100% success, 0% success)
- Test confidence level logic

### **Phase 4: Integration Tests** (Day 14 - 3 hours)
- Test full API endpoint with real PostgreSQL
- Test playbook breakdown correctness
- Test AI execution mode statistics

### **Phase 5: OpenAPI Spec** (Day 16 - 1 hour)
- Update `openapi.yaml` with new endpoint
- Add request/response schemas
- Add example responses

**Total Estimated Effort**: 14 hours (1.75 days)

---

## 📊 **Success Metrics**

### **API Usage Metrics**
- **Target**: 1000+ queries per day from Context API
- **Measure**: Track endpoint request count via Prometheus metrics

### **AI Decision Quality**
- **Target**: 10% improvement in AI playbook selection success rate
- **Measure**: Compare AI success rate before/after API availability

### **Response Time**
- **Target**: P95 response time <200ms
- **Measure**: Prometheus histogram metrics

---

## 🔄 **Alternatives Considered**

### **Alternative 1: No Incident-Type Aggregation (V1 Current State)**

**Approach**: Continue using workflow_id endpoint

**Rejected Because**:
- ❌ Workflow_id is meaningless for AI-generated unique workflows
- ❌ No data-driven playbook selection possible
- ❌ Cannot track continuous improvement by incident type

---

### **Alternative 2: Client-Side Aggregation**

**Approach**: Context API queries raw data and aggregates client-side

**Rejected Because**:
- ❌ Inefficient: Transfers large datasets over network
- ❌ Performance: Client-side aggregation is slower
- ❌ Inconsistent: Different clients might calculate differently

---

### **Alternative 3: Materialized View**

**Approach**: Pre-compute aggregations in PostgreSQL materialized views

**Rejected Because**:
- ❌ Complexity: Requires refresh logic and scheduling
- ❌ Staleness: Data may be outdated between refreshes
- ❌ Flexibility: Difficult to support dynamic time ranges

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**
**Date**: November 5, 2025
**Decision**: Implement as P0 priority for ADR-033 Phase 2
**Rationale**: Foundation for AI data-driven playbook selection and continuous improvement
**Approved By**: Architecture Team
**Related ADR**: [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)

---

## 📚 **References**

### **Related Business Requirements**
- BR-STORAGE-031-02: Playbook Success Rate API
- BR-STORAGE-031-03: Schema Migration (7 new columns)
- BR-INTEGRATION-008: Context API exposes incident-type endpoint
- BR-AI-057: AI uses success rates for playbook selection

### **Related Documents**
- [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-033: Cross-Service BRs](../architecture/decisions/ADR-033-CROSS-SERVICE-BRS.md)
- [ADR-037: BR Template Standard](../architecture/decisions/ADR-037-business-requirement-template-standard.md)
- [Data Storage Implementation Plan V5.0](../services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V5.3.md)

---

**Document Version**: 1.0
**Last Updated**: November 5, 2025
**Status**: ✅ Approved for V1 Implementation

