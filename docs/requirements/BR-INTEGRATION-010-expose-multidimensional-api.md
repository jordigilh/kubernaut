# BR-INTEGRATION-010: Expose Multi-Dimensional Success Rate API

**Business Requirement ID**: BR-INTEGRATION-010
**Category**: Context API (Integration Service)
**Priority**: P1
**Target Version**: V1
**Status**: ✅ Approved
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 requires comprehensive multi-dimensional success rate analysis (incident_type + playbook + action_type). The Context API must expose Data Storage's multi-dimensional endpoint (BR-STORAGE-031-05) with caching to enable advanced analytics.

**Current Limitations**:
- ❌ Context API does not expose multi-dimensional endpoint
- ❌ Advanced analytics require multiple API calls or direct Data Storage access
- ❌ Violates ADR-032: clients should not call Data Storage directly

**Impact**:
- Complex queries require multiple API calls
- No centralized caching for multi-dimensional queries
- Architectural violation if clients bypass Context API

---

## 🎯 **Business Objective**

**Expose Data Storage's multi-dimensional success rate API through Context API with caching.**

### **Success Criteria**
1. ✅ Context API exposes multi-dimensional endpoint
2. ✅ Proxies requests to Data Storage with caching (5-minute TTL)
3. ✅ Supports all dimension combinations
4. ✅ Cache hit rate >70%
5. ✅ Response time <300ms

---

## 🔧 **Functional Requirements**

### **FR-INTEGRATION-010-01: Multi-Dimensional Endpoint**

**API Specification**:
```http
GET /api/v1/incidents/aggregate/success-rate/multi-dimensional

Query Parameters:
- incident_type (string, optional)
- playbook_id (string, optional)
- playbook_version (string, optional)
- action_type (string, optional)
- time_range (string, optional, default: "7d")

Response (200 OK):
{
  "dimensions": {...},
  "success_rate": 0.94,
  "total_executions": 85,
  ...
}
```

**Implementation**:
```go
// handleGetSuccessRateMultiDimensional proxies to Data Storage
func (s *Server) handleGetSuccessRateMultiDimensional(w http.ResponseWriter, r *http.Request) {
    // Parse query parameters
    incidentType := r.URL.Query().Get("incident_type")
    playbookID := r.URL.Query().Get("playbook_id")
    playbookVersion := r.URL.Query().Get("playbook_version")
    actionType := r.URL.Query().Get("action_type")
    timeRange := r.URL.Query().Get("time_range")
    if timeRange == "" {
        timeRange = "7d"
    }

    // Check cache
    cacheKey := fmt.Sprintf("success-rate:multi:%s:%s:%s:%s:%s",
        incidentType, playbookID, playbookVersion, actionType, timeRange)
    if cached, found := s.cache.Get(cacheKey); found {
        s.respondJSON(w, http.StatusOK, cached)
        return
    }

    // Query Data Storage
    response, err := s.dataStorageClient.GetSuccessRateMultiDimensional(
        r.Context(), incidentType, playbookID, playbookVersion, actionType, timeRange)
    if err != nil {
        s.respondError(w, http.StatusInternalServerError, "failed to query success rate")
        return
    }

    // Cache response (5-minute TTL)
    s.cache.Set(cacheKey, response, 5*time.Minute)
    s.respondJSON(w, http.StatusOK, response)
}
```

**Acceptance Criteria**:
- ✅ Returns 200 OK for valid queries
- ✅ Caching layer with 5-minute TTL
- ✅ Supports all dimension combinations

---

## 🚀 **Implementation Phases**

### **Phase 1: Context API Endpoint** (Day 8 - 3 hours)
- Implement handler
- Add caching
- Integration tests

**Total Estimated Effort**: 3 hours (0.375 days)

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**
**Date**: November 5, 2025
**Decision**: Implement as P1 priority
**Rationale**: Enables advanced multi-dimensional analytics
**Approved By**: Architecture Team

---

**Document Version**: 1.0
**Last Updated**: November 5, 2025
**Status**: ✅ Approved for V1 Implementation

