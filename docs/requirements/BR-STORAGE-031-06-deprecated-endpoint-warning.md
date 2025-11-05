# BR-STORAGE-031-06: Deprecated Endpoint Warning Headers

**Business Requirement ID**: BR-STORAGE-031-06  
**Category**: Data Storage Service  
**Priority**: P2  
**Target Version**: V1  
**Status**: ✅ Approved  
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 deprecates the legacy `workflow_id` success rate endpoint in favor of `incident_type` and `playbook_id` endpoints. The Data Storage Service must maintain backward compatibility while guiding clients to migrate to new endpoints.

**Current Limitations**:
- ❌ No deprecation warnings for legacy endpoint
- ❌ Clients unaware they're using deprecated API
- ❌ No migration guidance in API responses
- ❌ Difficult to track legacy endpoint usage

**Impact**:
- Clients continue using deprecated endpoints indefinitely
- Cannot plan legacy endpoint removal
- No visibility into migration progress

---

## 🎯 **Business Objective**

**Add deprecation warning headers to legacy endpoint responses to guide clients toward new APIs and enable migration tracking.**

### **Success Criteria**
1. ✅ Legacy endpoint returns `Deprecation` and `Sunset` headers
2. ✅ Response includes migration guidance in `Link` header
3. ✅ Prometheus metrics track deprecated endpoint usage
4. ✅ Alert if deprecated endpoint usage >5% of total

---

## 🔧 **Functional Requirements**

### **FR-STORAGE-031-06-01: Deprecation Headers**

**HTTP Headers**:
```http
GET /api/v1/incidents/aggregate/success-rate?workflow_id=xyz

Response (200 OK):
Deprecation: true
Sunset: Wed, 01 Apr 2026 00:00:00 GMT
Link: </api/v1/incidents/aggregate/success-rate/by-incident-type>; rel="alternate"; title="Use incident-type endpoint"
Warning: 299 - "Deprecated endpoint, use /by-incident-type or /by-playbook instead"

{
  "workflow_id": "xyz",
  "success_rate": 0.85,
  ...
}
```

**Acceptance Criteria**:
- ✅ `Deprecation: true` header present
- ✅ `Sunset` header indicates removal date (6 months)
- ✅ `Link` header points to alternative endpoint
- ✅ `Warning` header provides migration guidance

---

### **FR-STORAGE-031-06-02: Usage Tracking**

**Prometheus Metrics**:
```prometheus
datastorage_deprecated_endpoint_requests_total{endpoint="workflow_id"} 150
datastorage_total_requests_total{endpoint="all"} 10000
```

**Acceptance Criteria**:
- ✅ Tracks deprecated endpoint usage
- ✅ Alert if usage >5% of total requests
- ✅ Dashboard shows migration progress

---

## 🚀 **Implementation Phases**

### **Phase 1: Add Headers** (Day 13 - 2 hours)
- Add deprecation headers to response
- Add migration guidance
- Unit tests

### **Phase 2: Tracking** (Day 13 - 1 hour)
- Add Prometheus metrics
- Add alerting threshold

**Total Estimated Effort**: 3 hours (0.375 days)

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**  
**Date**: November 5, 2025  
**Decision**: Implement as P2 priority  
**Rationale**: Enables smooth migration from legacy endpoints  
**Approved By**: Architecture Team

---

**Document Version**: 1.0  
**Last Updated**: November 5, 2025  
**Status**: ✅ Approved for V1 Implementation

