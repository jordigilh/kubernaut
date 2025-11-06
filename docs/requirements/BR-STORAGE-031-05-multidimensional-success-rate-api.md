# BR-STORAGE-031-05: Multi-Dimensional Success Rate API

**Business Requirement ID**: BR-STORAGE-031-05
**Category**: Data Storage Service
**Priority**: P1
**Target Version**: V1
**Status**: ✅ Approved
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 tracks success across THREE dimensions: incident_type (PRIMARY), playbook (SECONDARY), action_type (TERTIARY). The Data Storage Service must provide a comprehensive multi-dimensional aggregation API to analyze success rates across all dimensions simultaneously.

**Current Limitations**:
- ❌ No single API to query across all 3 dimensions
- ❌ Requires multiple API calls (incident-type + playbook + action)
- ❌ Cannot answer: "What's the success rate for pod-oom-recovery v1.2 handling pod-oom-killer incidents with increase_memory action?"

**Impact**:
- Complex queries require multiple API calls
- Cannot perform comprehensive effectiveness analysis
- Missing deep-dive analysis capabilities

---

## 🎯 **Business Objective**

**Provide multi-dimensional success rate API to query effectiveness across incident_type, playbook, and action_type simultaneously.**

### **Success Criteria**
1. ✅ API accepts all 3 dimensions as query parameters
2. ✅ Returns success rate for specific dimension combination
3. ✅ Supports partial queries (any combination of dimensions)
4. ✅ Includes breakdown by missing dimensions

---

## 🔧 **Functional Requirements**

### **FR-STORAGE-031-05-01: Multi-Dimensional Query API**

**API Specification**:
```http
GET /api/v1/incidents/aggregate/success-rate/multi-dimensional

Query Parameters:
- incident_type (string, optional): Incident type filter
- playbook_id (string, optional): Playbook filter
- playbook_version (string, optional): Playbook version filter (requires playbook_id)
- action_type (string, optional): Action type filter
- time_range (string, optional, default: "7d")

Response (200 OK):
{
  "dimensions": {
    "incident_type": "pod-oom-killer",
    "playbook_id": "pod-oom-recovery",
    "playbook_version": "v1.2",
    "action_type": "increase_memory"
  },
  "time_range": "7d",
  "total_executions": 85,
  "successful_executions": 80,
  "failed_executions": 5,
  "success_rate": 0.94,
  "confidence": "high",
  "ai_execution_mode": {
    "catalog_selected": 76,
    "chained": 8,
    "manual_escalation": 1
  }
}
```

**SQL Implementation**:
```sql
SELECT
    COUNT(*) AS total_executions,
    SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS successful_executions,
    CAST(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS FLOAT) / NULLIF(COUNT(*), 0) AS success_rate
FROM resource_action_traces
WHERE incident_type = $1            -- Optional filter
  AND playbook_id = $2              -- Optional filter
  AND playbook_version = $3         -- Optional filter
  AND action_type = $4              -- Optional filter
  AND action_timestamp >= NOW() - INTERVAL '7 days';
```

**Acceptance Criteria**:
- ✅ Accepts any combination of dimension filters
- ✅ Returns 200 OK for valid queries
- ✅ Returns 400 Bad Request if playbook_version provided without playbook_id
- ✅ Handles queries with 0 results gracefully

---

## 🚀 **Implementation Phases**

### **Phase 1: Multi-Dimensional Handler** (Day 15 - 4 hours)
- Implement multi-dimensional query logic
- Add parameter validation
- SQL query construction

### **Phase 2: Testing** (Day 15 - 2 hours)
- Unit tests (dimension combinations)
- Integration tests

**Total Estimated Effort**: 6 hours (0.75 days)

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**
**Date**: November 5, 2025
**Decision**: Implement as P1 priority
**Rationale**: Enables comprehensive effectiveness analysis
**Approved By**: Architecture Team

---

**Document Version**: 1.0
**Last Updated**: November 5, 2025
**Status**: ✅ Approved for V1 Implementation

