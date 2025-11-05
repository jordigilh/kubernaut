# BR-STORAGE-031-04: AI Execution Mode in Aggregation Responses

**Business Requirement ID**: BR-STORAGE-031-04  
**Category**: Data Storage Service  
**Priority**: P1  
**Target Version**: V1  
**Status**: ✅ Approved  
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 tracks AI execution mode (catalog selection, chained playbooks, manual escalation) via BR-REMEDIATION-017. The Data Storage Service must include AI execution mode breakdown in aggregation API responses to enable analysis of ADR-033 Hybrid Model compliance (90-9-1 distribution).

**Current Limitations**:
- ❌ Aggregation responses do not include AI execution mode breakdown
- ❌ Cannot validate ADR-033 Hybrid Model distribution from aggregation APIs
- ❌ Dashboard cannot display execution mode breakdown charts

**Impact**:
- Cannot measure 90-9-1 distribution target
- Missing visibility into AI decision patterns
- Cannot validate ADR-033 compliance

---

## 🎯 **Business Objective**

**Include AI execution mode statistics in incident-type and playbook success rate aggregation responses.**

### **Success Criteria**
1. ✅ Incident-type aggregation includes `ai_execution_mode` breakdown
2. ✅ Playbook aggregation includes `ai_execution_mode` breakdown
3. ✅ Breakdown shows catalog_selected, chained, manual_escalation counts
4. ✅ Dashboard displays execution mode distribution (90-9-1 gauge)

---

## 🔧 **Functional Requirements**

### **FR-STORAGE-031-04-01: AI Execution Mode Breakdown**

**Enhanced Response**:
```http
GET /api/v1/incidents/aggregate/success-rate/by-incident-type?incident_type=pod-oom-killer

Response (200 OK):
{
  "incident_type": "pod-oom-killer",
  "success_rate": 0.85,
  "total_executions": 100,
  "ai_execution_mode": {
    "catalog_selected": 90,   // 90% (ADR-033 target)
    "chained": 9,             // 9% (ADR-033 target)
    "manual_escalation": 1    // 1% (ADR-033 target)
  },
  ...
}
```

**SQL Implementation**:
```sql
SELECT
    COUNT(CASE WHEN ai_selected_playbook = TRUE THEN 1 END) AS catalog_selected,
    COUNT(CASE WHEN ai_chained_playbooks = TRUE THEN 1 END) AS chained,
    COUNT(CASE WHEN ai_manual_escalation = TRUE THEN 1 END) AS manual_escalation
FROM resource_action_traces
WHERE incident_type = $1
  AND action_timestamp >= NOW() - INTERVAL '7 days';
```

**Acceptance Criteria**:
- ✅ Includes `ai_execution_mode` object in response
- ✅ Counts sum to total_executions
- ✅ Returns NULL if no AI execution data available

---

## 🚀 **Implementation Phases**

### **Phase 1: SQL Query Enhancement** (Day 14 - 2 hours)
- Add AI execution mode aggregation to SQL queries
- Update response models
- Unit tests

### **Phase 2: API Response Update** (Day 14 - 1 hour)
- Include ai_execution_mode in responses
- Integration tests

**Total Estimated Effort**: 3 hours (0.375 days)

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**  
**Date**: November 5, 2025  
**Decision**: Implement as P1 priority  
**Rationale**: Enables ADR-033 Hybrid Model compliance validation  
**Approved By**: Architecture Team

---

**Document Version**: 1.0  
**Last Updated**: November 5, 2025  
**Status**: ✅ Approved for V1 Implementation

