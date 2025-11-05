# BR-STORAGE-031-02: Playbook Success Rate API

**Business Requirement ID**: BR-STORAGE-031-02
**Category**: Data Storage Service
**Priority**: P0
**Target Version**: V1
**Status**: ✅ Approved
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 introduces **Remediation Playbook Catalog** with version tracking. To enable continuous improvement and data-driven playbook versioning decisions, the Data Storage Service must expose a REST API to calculate success rates aggregated by **playbook** (the SECONDARY dimension for AI learning).

**Current Limitations**:
- ❌ No API endpoint to query success rates by playbook (ID + version)
- ❌ Cannot compare effectiveness of different playbook versions (e.g., v1.1 vs v1.2)
- ❌ No visibility into which playbooks work best for which incident types
- ❌ No data to support playbook deprecation decisions
- ❌ Cannot track playbook step-by-step success rates for troubleshooting

**Impact**:
- Teams cannot validate if new playbook versions improve remediation success rates
- No data-driven playbook deprecation strategy
- Cannot identify which steps in a multi-step playbook cause failures
- Missing foundation for continuous playbook improvement
- AI cannot optimize playbook selection across incident types

---

## 🎯 **Business Objective**

**Enable Context API, AI Service, and Operations teams to query playbook-specific success rates for data-driven playbook improvement and version management.**

### **Success Criteria**
1. ✅ REST API endpoint exposes playbook success rate aggregation (by playbook_id + playbook_version)
2. ✅ Response includes total executions, success/failure counts, and success rate (0.0-1.0)
3. ✅ Includes incident-type breakdown (which incident types used this playbook)
4. ✅ Includes action-level breakdown (step-by-step success rates for multi-step playbooks)
5. ✅ Supports time range filtering (7d, 30d, 90d)
6. ✅ Returns confidence level based on minimum samples threshold
7. ✅ Enables version comparison (e.g., compare v1.1 vs v1.2 effectiveness)

---

## 📊 **Use Cases**

### **Use Case 1: Validate New Playbook Version Effectiveness**

**Scenario**: Team deploys `pod-oom-recovery v1.2` and wants to validate it's more effective than `v1.1`.

**Current Flow** (Without BR-STORAGE-031-02):
```
1. Team deploys pod-oom-recovery v1.2
2. No playbook-specific success rate endpoint available
3. ❌ Cannot compare v1.2 vs v1.1 effectiveness
4. ❌ Must rely on anecdotal evidence or manual log analysis
5. ❌ No data-driven validation of playbook improvements
```

**Desired Flow with BR-STORAGE-031-02**:
```
1. Team deploys pod-oom-recovery v1.2
2. Query Data Storage:
   - GET /api/v1/incidents/aggregate/success-rate/by-playbook?playbook_id=pod-oom-recovery&playbook_version=v1.2&time_range=7d
   - GET /api/v1/incidents/aggregate/success-rate/by-playbook?playbook_id=pod-oom-recovery&playbook_version=v1.1&time_range=7d
3. Response:
   - v1.2: success_rate=0.89, total_executions=90
   - v1.1: success_rate=0.50, total_executions=10
4. ✅ Measurable improvement: +39% success rate
5. ✅ Data-driven decision to promote v1.2 as default
6. ✅ Deprecate v1.1 based on proven low effectiveness
```

---

### **Use Case 2: Identify Playbook Failure Hotspots**

**Scenario**: `database-recovery-playbook` has 60% success rate. Team wants to identify which step is causing failures.

**Current Flow**:
```
1. Team observes low success rate for playbook
2. No step-by-step breakdown available
3. ❌ Cannot identify which step in the playbook fails most often
4. ❌ Must manually analyze logs across multiple executions
5. ❌ Time-consuming troubleshooting process
```

**Desired Flow with BR-STORAGE-031-02**:
```
1. Team observes 60% success rate
2. Query playbook with action breakdown:
   GET /api/v1/incidents/aggregate/success-rate/by-playbook?playbook_id=database-recovery&playbook_version=v2.0
3. Response includes breakdown_by_action:
   [
     {"action_type": "stop_pod", "step_number": 1, "success_rate": 0.95},
     {"action_type": "backup_database", "step_number": 2, "success_rate": 0.90},
     {"action_type": "restore_database", "step_number": 3, "success_rate": 0.40},  ← FAILURE HOTSPOT
     {"action_type": "start_pod", "step_number": 4, "success_rate": 0.98}
   ]
4. ✅ Identified: Step 3 (restore_database) causes 60% of failures
5. ✅ Team focuses improvements on restore_database action
6. ✅ Faster troubleshooting and targeted fixes
```

---

### **Use Case 3: AI Cross-Incident-Type Playbook Selection**

**Scenario**: AI receives `container-memory-pressure` incident and wants to check if `pod-oom-recovery` playbook (designed for `pod-oom-killer`) also works for this incident type.

**Current Flow**:
```
1. AI receives container-memory-pressure alert
2. No cross-incident-type playbook effectiveness data
3. ❌ AI cannot determine if pod-oom-recovery playbook works for this incident type
4. ❌ AI either: (a) doesn't use playbook, or (b) uses it blindly without data
```

**Desired Flow with BR-STORAGE-031-02**:
```
1. AI receives container-memory-pressure alert
2. Query playbook effectiveness with incident breakdown:
   GET /api/v1/incidents/aggregate/success-rate/by-playbook?playbook_id=pod-oom-recovery&playbook_version=v1.2
3. Response includes breakdown_by_incident_type:
   [
     {"incident_type": "pod-oom-killer", "executions": 85, "success_rate": 0.90},
     {"incident_type": "container-memory-pressure", "executions": 5, "success_rate": 0.60}
   ]
4. ✅ AI learns: pod-oom-recovery works great for pod-oom-killer (90%)
5. ✅ AI learns: pod-oom-recovery works moderately for container-memory-pressure (60%)
6. ✅ AI makes informed decision: use playbook but with lower confidence
```

---

## 🔧 **Functional Requirements**

### **FR-STORAGE-031-02-01: REST API Endpoint**

**Requirement**: Data Storage Service SHALL expose a REST API endpoint to calculate success rate by playbook (ID + version).

**Endpoint Specification**:
```http
GET /api/v1/incidents/aggregate/success-rate/by-playbook

Query Parameters:
- playbook_id (string, required): Playbook identifier (e.g., "pod-oom-recovery")
- playbook_version (string, required): Playbook version (e.g., "v1.2")
- time_range (string, optional, default: "7d"): Time window (7d, 30d, 90d)
- min_samples (int, optional, default: 5): Minimum executions required for high confidence

Response (200 OK):
{
  "playbook_id": "pod-oom-recovery",
  "playbook_version": "v1.2",
  "time_range": "7d",
  "total_executions": 90,
  "successful_executions": 80,
  "failed_executions": 10,
  "success_rate": 0.89,
  "confidence": "high",
  "min_samples_met": true,
  "breakdown_by_incident_type": [
    {
      "incident_type": "pod-oom-killer",
      "executions": 85,
      "success_rate": 0.90
    },
    {
      "incident_type": "container-memory-pressure",
      "executions": 5,
      "success_rate": 0.60
    }
  ],
  "breakdown_by_action": [
    {
      "action_type": "increase_memory",
      "step_number": 1,
      "executions": 90,
      "success_rate": 0.95
    },
    {
      "action_type": "restart_pod",
      "step_number": 2,
      "executions": 90,
      "success_rate": 0.93
    }
  ]
}
```

**Acceptance Criteria**:
- ✅ Endpoint returns 200 OK for valid playbook_id + playbook_version
- ✅ Endpoint returns 400 Bad Request for missing required parameters
- ✅ Success rate is exactly `successful_executions / total_executions`
- ✅ Incident-type breakdown sums to total executions
- ✅ Action breakdown shows step-by-step success rates
- ✅ Confidence level calculated based on min_samples threshold

---

### **FR-STORAGE-031-02-02: SQL Aggregation Queries**

**Requirement**: Data Storage Service SHALL execute efficient SQL aggregation queries for playbook-specific data.

**SQL Implementation**:
```sql
-- Primary query: Playbook success rate
SELECT
    playbook_id,
    playbook_version,
    COUNT(*) AS total_executions,
    SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS successful_executions,
    SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) AS failed_executions,
    CAST(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS FLOAT) / NULLIF(COUNT(*), 0) AS success_rate
FROM resource_action_traces
WHERE playbook_id = $1
  AND playbook_version = $2
  AND action_timestamp >= NOW() - INTERVAL '7 days'
GROUP BY playbook_id, playbook_version;

-- Incident-type breakdown subquery
SELECT
    incident_type,
    COUNT(*) AS executions,
    CAST(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS FLOAT) / NULLIF(COUNT(*), 0) AS success_rate
FROM resource_action_traces
WHERE playbook_id = $1
  AND playbook_version = $2
  AND action_timestamp >= NOW() - INTERVAL '7 days'
GROUP BY incident_type
ORDER BY executions DESC;

-- Action-level breakdown subquery (step-by-step)
SELECT
    action_type,
    playbook_step_number AS step_number,
    COUNT(*) AS executions,
    CAST(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS FLOAT) / NULLIF(COUNT(*), 0) AS success_rate
FROM resource_action_traces
WHERE playbook_id = $1
  AND playbook_version = $2
  AND action_timestamp >= NOW() - INTERVAL '7 days'
GROUP BY action_type, playbook_step_number
ORDER BY playbook_step_number ASC;
```

**Acceptance Criteria**:
- ✅ Queries use indexes on `playbook_id`, `playbook_version`, `action_timestamp`
- ✅ Query execution time <50ms for typical datasets (<10K records in window)
- ✅ Handle zero executions gracefully (return 0.0 success rate)
- ✅ Handle null playbook fields (exclude from aggregation)

---

### **FR-STORAGE-031-02-03: Playbook Version Comparison Support**

**Requirement**: API response SHALL enable easy version comparison for the same playbook_id.

**Implementation**: Client can make parallel requests for different versions and compare results.

**Example Comparison Flow**:
```go
// Query v1.1
resp_v1_1, _ := client.Get("/api/v1/incidents/aggregate/success-rate/by-playbook?playbook_id=pod-oom-recovery&playbook_version=v1.1")
// Query v1.2
resp_v1_2, _ := client.Get("/api/v1/incidents/aggregate/success-rate/by-playbook?playbook_id=pod-oom-recovery&playbook_version=v1.2")

// Compare success rates
if resp_v1_2.SuccessRate > resp_v1_1.SuccessRate {
    fmt.Println("v1.2 is more effective than v1.1")
}
```

**Acceptance Criteria**:
- ✅ Response format is consistent across versions for easy comparison
- ✅ No special endpoint needed for version comparison (client-side logic)
- ✅ API documentation includes version comparison example

---

## 📈 **Non-Functional Requirements**

### **NFR-STORAGE-031-02-01: Performance**

- ✅ Response time <200ms for 95th percentile queries
- ✅ Support 100 concurrent requests per second
- ✅ Query optimization via indexes on `playbook_id`, `playbook_version`, `action_timestamp`

### **NFR-STORAGE-031-02-02: Scalability**

- ✅ Handle datasets with 1M+ historical execution records
- ✅ Time range filtering prevents full table scans
- ✅ Breakdown queries return compact results (<100 rows per breakdown)

### **NFR-STORAGE-031-02-03: Data Accuracy**

- ✅ Success rate calculations are mathematically precise (no rounding errors >0.001)
- ✅ Breakdown totals sum exactly to total_executions
- ✅ Step-by-step action breakdown reflects actual playbook execution order

---

## 🔗 **Dependencies**

### **Upstream Dependencies**
- ✅ ADR-033: Remediation Playbook Catalog (architectural decision)
- ✅ BR-STORAGE-031-03: Schema migration (playbook_id, playbook_version, playbook_step_number columns must exist)
- ✅ BR-PLAYBOOK-001: Playbook registry (validates playbook_id exists)

### **Downstream Impacts**
- ✅ BR-INTEGRATION-009: Context API must consume this endpoint
- ✅ BR-AI-057: AI Service uses playbook success rates for selection optimization
- ✅ BR-EFFECTIVENESS-002: Effectiveness Monitor calculates playbook trends
- ✅ Operations Dashboard: Displays playbook version comparison charts

---

## 🚀 **Implementation Phases**

### **Phase 1: Handler Implementation** (Day 13 - 4 hours)
- Implement `handleGetSuccessRateByPlaybook` HTTP handler
- Add parameter parsing and validation (playbook_id, playbook_version required)
- Add RFC 7807 error responses

### **Phase 2: Repository Layer** (Day 13 - 4 hours)
- Implement `GetSuccessRateByPlaybook` repository method
- Add SQL aggregation queries (primary + 2 breakdown queries)
- Add confidence level calculation logic

### **Phase 3: Unit Tests** (Day 14 - 3 hours)
- Test success rate calculation accuracy
- Test incident-type breakdown correctness
- Test action-level breakdown (step ordering)
- Test version comparison scenarios

### **Phase 4: Integration Tests** (Day 14 - 3 hours)
- Test full API endpoint with real PostgreSQL
- Test concurrent version queries (v1.1 + v1.2 simultaneously)
- Test breakdown query performance

### **Phase 5: OpenAPI Spec** (Day 16 - 1 hour)
- Update `openapi.yaml` with new endpoint
- Add request/response schemas with breakdown structures
- Add version comparison example

**Total Estimated Effort**: 15 hours (2 days)

---

## 📊 **Success Metrics**

### **API Usage Metrics**
- **Target**: 500+ queries per day from Context API and Operations Dashboard
- **Measure**: Track endpoint request count via Prometheus metrics

### **Playbook Improvement Validation**
- **Target**: 80% of new playbook versions show measurable improvement (>5% success rate increase)
- **Measure**: Compare new version vs old version success rates within 7 days of deployment

### **Deprecation Decision Support**
- **Target**: 100% of playbook deprecations are data-driven (based on low success rates)
- **Measure**: Track playbook deprecation decisions vs API query data

---

## 🔄 **Alternatives Considered**

### **Alternative 1: No Playbook Breakdown (Only Overall Success Rate)**

**Approach**: Return only overall success rate without incident-type or action-level breakdowns

**Rejected Because**:
- ❌ Cannot identify failure hotspots (which step fails)
- ❌ Cannot optimize playbook for specific incident types
- ❌ Limited troubleshooting value

---

### **Alternative 2: Separate Endpoints for Each Breakdown**

**Approach**: Create 3 separate endpoints:
- `/by-playbook/overall`
- `/by-playbook/incident-breakdown`
- `/by-playbook/action-breakdown`

**Rejected Because**:
- ❌ Multiple API calls required for complete picture
- ❌ Increased latency (3 round trips instead of 1)
- ❌ More complex client integration

---

### **Alternative 3: Include All Versions in Single Response**

**Approach**: Endpoint returns all versions for a playbook_id in single response

**Rejected Because**:
- ❌ Response size grows with number of versions
- ❌ Clients often only need specific version
- ❌ Less flexible for client-side version comparison logic

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**
**Date**: November 5, 2025
**Decision**: Implement as P0 priority for ADR-033 Phase 2
**Rationale**: Required for playbook version management, continuous improvement, and AI optimization
**Approved By**: Architecture Team
**Related ADR**: [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)

---

## 📚 **References**

### **Related Business Requirements**
- BR-STORAGE-031-01: Incident-Type Success Rate API
- BR-STORAGE-031-03: Schema Migration (7 new columns)
- BR-INTEGRATION-009: Context API exposes playbook endpoint
- BR-AI-057: AI uses success rates for playbook selection
- BR-PLAYBOOK-001: Playbook registry management

### **Related Documents**
- [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-033: Cross-Service BRs](../architecture/decisions/ADR-033-CROSS-SERVICE-BRS.md)
- [ADR-034: BR Template Standard](../architecture/decisions/ADR-034-business-requirement-template-standard.md)
- [Data Storage Implementation Plan V5.0](../services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V5.0.md)

---

**Document Version**: 1.0
**Last Updated**: November 5, 2025
**Status**: ✅ Approved for V1 Implementation

