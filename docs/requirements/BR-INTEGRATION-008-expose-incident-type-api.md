# BR-INTEGRATION-008: Expose Incident-Type Success Rate API

**Business Requirement ID**: BR-INTEGRATION-008  
**Category**: Context API (Integration Service)  
**Priority**: P0  
**Target Version**: V1  
**Status**: ✅ Approved  
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 requires AI Service and Operations Dashboard to query incident-type success rates for data-driven remediation decisions. The Context API must act as the integration layer, consuming Data Storage's aggregation endpoint and exposing it to upstream clients (AI Service, dashboards, etc.).

**Current Limitations**:
- ❌ Context API does not expose incident-type success rate endpoint
- ❌ AI Service has no way to query historical success rates
- ❌ Operations Dashboard cannot display incident-type effectiveness
- ❌ Violates ADR-032 (Data Access Layer Isolation): AI should not call Data Storage directly

**Impact**:
- BR-AI-057 (AI uses success rates) blocked without Context API exposure
- AI cannot implement ADR-033 data-driven playbook selection
- Architectural violation: AI Service directly calling Data Storage Service bypasses integration layer
- No centralized API for success rate queries (multiple clients calling Data Storage directly)

---

## 🎯 **Business Objective**

**Expose Data Storage's incident-type success rate API through Context API to enable AI Service and Operations Dashboard to query remediation effectiveness data.**

### **Success Criteria**
1. ✅ Context API exposes `GET /incidents/aggregate/success-rate/by-incident-type` endpoint
2. ✅ Context API proxies requests to Data Storage aggregation endpoint
3. ✅ Response format matches Data Storage's response (transparent proxy)
4. ✅ AI Service uses Context API endpoint (not Data Storage directly)
5. ✅ Operations Dashboard uses Context API endpoint
6. ✅ Context API adds caching layer (5-minute TTL) to reduce Data Storage load
7. ✅ Response time <300ms (including Data Storage query + caching)

---

## 📊 **Use Cases**

### **Use Case 1: AI Queries Incident-Type Success Rate**

**Scenario**: AI Service needs to query success rate for `pod-oom-killer` incident type.

**Current Flow** (Without BR-INTEGRATION-008):
```
1. AI Service needs incident-type success rate
2. No Context API endpoint available
3. ❌ AI either:
   - Option A: Calls Data Storage directly (violates ADR-032 architecture)
   - Option B: Cannot get success rate data (blocks ADR-033)
4. ❌ Architectural violation or feature blocked
```

**Desired Flow with BR-INTEGRATION-008**:
```
1. AI Service needs success rate for pod-oom-killer
2. AI calls Context API:
   GET /api/v1/incidents/aggregate/success-rate/by-incident-type?incident_type=pod-oom-killer&time_range=7d
3. Context API checks cache (5-minute TTL)
   - If cached: Return cached response (fast path, <50ms)
   - If not cached: Query Data Storage endpoint
4. Context API queries Data Storage:
   GET /api/v1/incidents/aggregate/success-rate/by-incident-type?incident_type=pod-oom-killer&time_range=7d
5. Data Storage returns response:
   {
     "incident_type": "pod-oom-killer",
     "success_rate": 0.85,
     "total_executions": 100,
     ...
   }
6. Context API caches response (5-minute TTL)
7. Context API returns response to AI Service
8. ✅ AI receives data through proper integration layer
9. ✅ Respects ADR-032 data access pattern
10. ✅ Caching reduces Data Storage load
```

---

### **Use Case 2: Operations Dashboard Displays Effectiveness**

**Scenario**: Operations Dashboard shows incident-type remediation effectiveness chart.

**Current Flow**:
```
1. Dashboard needs success rates for top 10 incident types
2. No centralized API available
3. ❌ Dashboard calls Data Storage directly (10 requests)
4. ❌ No caching (repeated queries every page load)
5. ❌ High load on Data Storage
```

**Desired Flow with BR-INTEGRATION-008**:
```
1. Dashboard queries Context API for top 10 incident types:
   - GET /incidents/aggregate/success-rate/by-incident-type?incident_type=pod-oom-killer
   - GET /incidents/aggregate/success-rate/by-incident-type?incident_type=high-cpu
   - ... (10 requests)
2. Context API serves 8/10 from cache (hit rate: 80%)
3. Context API queries Data Storage for 2 cache misses
4. ✅ Reduced Data Storage load (2 queries vs 10)
5. ✅ Faster response time for dashboard (8 cached responses)
6. ✅ Proper architectural layering (dashboard → Context API → Data Storage)
```

---

### **Use Case 3: Cache Invalidation for Real-Time Updates**

**Scenario**: New remediation execution completes, Context API cache should reflect updated success rate within 5 minutes.

**Current Flow**:
```
1. New execution completes at 10:00 AM
2. Context API cache has stale data (from 9:57 AM query)
3. ❌ Cache TTL is 60 minutes (too stale)
4. ❌ Success rate not updated until 10:57 AM
5. ❌ AI uses outdated success rates for decisions
```

**Desired Flow with BR-INTEGRATION-008**:
```
1. New execution completes at 10:00 AM
2. Context API cache has data from 9:57 AM (TTL: 5 minutes)
3. Cache expires at 10:02 AM (5 minutes after last query)
4. Next query at 10:03 AM:
   - Cache miss (expired)
   - Query Data Storage (fresh data including 10:00 AM execution)
   - Cache new response (TTL: 5 minutes)
5. ✅ Success rate updated within 5 minutes
6. ✅ Balance between freshness and Data Storage load
```

---

## 🔧 **Functional Requirements**

### **FR-INTEGRATION-008-01: Context API Endpoint**

**Requirement**: Context API SHALL expose incident-type success rate endpoint that proxies Data Storage.

**Endpoint Specification**:
```http
GET /api/v1/incidents/aggregate/success-rate/by-incident-type

Query Parameters:
- incident_type (string, required): Incident type (e.g., "pod-oom-killer")
- time_range (string, optional, default: "7d"): Time window (7d, 30d, 90d)
- min_samples (int, optional, default: 5): Minimum executions required

Response (200 OK):
{
  "incident_type": "pod-oom-killer",
  "time_range": "7d",
  "total_executions": 100,
  "successful_executions": 85,
  "failed_executions": 15,
  "success_rate": 0.85,
  "confidence": "high",
  "min_samples_met": true,
  "breakdown_by_playbook": [...]
  "ai_execution_mode": {...}
}
```

**Implementation Example**:
```go
package contextapi

// handleGetSuccessRateByIncidentType proxies to Data Storage
func (s *Server) handleGetSuccessRateByIncidentType(w http.ResponseWriter, r *http.Request) {
    incidentType := r.URL.Query().Get("incident_type")
    if incidentType == "" {
        s.respondError(w, http.StatusBadRequest, "incident_type is required")
        return
    }

    timeRange := r.URL.Query().Get("time_range")
    if timeRange == "" {
        timeRange = "7d"
    }

    // Check cache first
    cacheKey := fmt.Sprintf("success-rate:incident:%s:%s", incidentType, timeRange)
    if cached, found := s.cache.Get(cacheKey); found {
        s.logger.Debug("cache hit", zap.String("cache_key", cacheKey))
        s.respondJSON(w, http.StatusOK, cached)
        return
    }

    // Cache miss: query Data Storage
    response, err := s.dataStorageClient.GetSuccessRateByIncidentType(r.Context(), incidentType, timeRange)
    if err != nil {
        s.logger.Error("failed to query Data Storage",
            zap.String("incident_type", incidentType),
            zap.Error(err))
        s.respondError(w, http.StatusInternalServerError, "failed to query success rate")
        return
    }

    // Cache response (5-minute TTL)
    s.cache.Set(cacheKey, response, 5*time.Minute)

    s.respondJSON(w, http.StatusOK, response)
}
```

**Acceptance Criteria**:
- ✅ Endpoint returns 200 OK for valid requests
- ✅ Endpoint returns 400 Bad Request for missing incident_type
- ✅ Response format matches Data Storage response exactly (transparent proxy)
- ✅ Caching layer implemented with 5-minute TTL
- ✅ Cache key includes incident_type + time_range
- ✅ Logs cache hits/misses for monitoring

---

### **FR-INTEGRATION-008-02: Data Storage Client Integration**

**Requirement**: Context API SHALL use Data Storage REST API client to query success rates.

**Client Implementation**:
```go
package contextapi

// DataStorageClient queries Data Storage Service
type DataStorageClient struct {
    baseURL    string
    httpClient *http.Client
    logger     *zap.Logger
}

// GetSuccessRateByIncidentType queries incident-type success rate
func (c *DataStorageClient) GetSuccessRateByIncidentType(ctx context.Context, incidentType, timeRange string) (*SuccessRateResponse, error) {
    url := fmt.Sprintf("%s/api/v1/incidents/aggregate/success-rate/by-incident-type?incident_type=%s&time_range=%s",
        c.baseURL, url.QueryEscape(incidentType), timeRange)

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("failed to query Data Storage: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("Data Storage returned %d: %s", resp.StatusCode, string(body))
    }

    var successRate SuccessRateResponse
    if err := json.NewDecoder(resp.Body).Decode(&successRate); err != nil {
        return nil, fmt.Errorf("failed to decode response: %w", err)
    }

    return &successRate, nil
}
```

**Acceptance Criteria**:
- ✅ Client uses context for cancellation/timeout
- ✅ Client handles HTTP errors (4xx, 5xx)
- ✅ Client logs requests and responses
- ✅ Client timeout: 2 seconds (prevent slow queries from blocking Context API)
- ✅ Client retries transient failures (503 Service Unavailable)

---

### **FR-INTEGRATION-008-03: Caching Layer**

**Requirement**: Context API SHALL implement Redis-based caching with 5-minute TTL.

**Caching Strategy**:
- **Cache Key Format**: `success-rate:incident:{incident_type}:{time_range}`
- **TTL**: 5 minutes
- **Eviction Policy**: LRU (Least Recently Used)
- **Cache Size**: 1000 entries (approximately 100KB)

**Acceptance Criteria**:
- ✅ Cache hit rate >70% for typical usage (AI + dashboard queries)
- ✅ Cache miss triggers Data Storage query
- ✅ Cache entries expire after 5 minutes
- ✅ Cache handles concurrent requests (thread-safe)
- ✅ Prometheus metrics: `contextapi_cache_hits_total`, `contextapi_cache_misses_total`

---

## 📈 **Non-Functional Requirements**

### **NFR-INTEGRATION-008-01: Performance**

- ✅ Response time <300ms for 95th percentile (cached: <50ms, uncached: <250ms)
- ✅ Cache hit rate >70%
- ✅ Support 200 concurrent requests

### **NFR-INTEGRATION-008-02: Reliability**

- ✅ Graceful degradation if Data Storage unavailable (return cached data even if expired)
- ✅ Circuit breaker pattern for Data Storage calls (open after 5 consecutive failures)
- ✅ Timeout: 2 seconds for Data Storage queries

### **NFR-INTEGRATION-008-03: Observability**

- ✅ Log all Data Storage queries with latency
- ✅ Prometheus metrics:
  - `contextapi_datastorage_requests_total{endpoint="...", status="success|error"}`
  - `contextapi_datastorage_request_duration_seconds{endpoint="..."}`
  - `contextapi_cache_hits_total{endpoint="..."}`
  - `contextapi_cache_misses_total{endpoint="..."}`

---

## 🔗 **Dependencies**

### **Upstream Dependencies**
- ✅ ADR-032: Data Access Layer Isolation (Context API as integration layer)
- ✅ BR-STORAGE-031-01: Data Storage incident-type success rate endpoint exists
- ✅ Redis: For caching layer

### **Downstream Impacts**
- ✅ BR-AI-057: AI Service uses Context API endpoint (not Data Storage directly)
- ✅ Operations Dashboard: Uses Context API for success rate charts

---

## 🚀 **Implementation Phases**

### **Phase 1: Data Storage Client** (Day 5 - 3 hours)
- Implement `DataStorageClient` with success rate query method
- Add timeout and retry logic
- Unit tests for client

### **Phase 2: Caching Layer** (Day 5 - 3 hours)
- Integrate Redis client
- Implement cache key generation
- Add cache hit/miss metrics

### **Phase 3: Context API Endpoint** (Day 6 - 4 hours)
- Implement `handleGetSuccessRateByIncidentType` handler
- Integrate Data Storage client + caching
- Add error handling and logging

### **Phase 4: Testing** (Day 7 - 4 hours)
- Unit tests: Handler logic, caching behavior
- Integration tests: Full endpoint with real Data Storage
- Test cache expiration and invalidation

**Total Estimated Effort**: 14 hours (1.75 days)

---

## 📊 **Success Metrics**

### **Cache Hit Rate**
- **Target**: 70%+ cache hit rate
- **Measure**: `contextapi_cache_hits_total / (contextapi_cache_hits_total + contextapi_cache_misses_total)`

### **API Usage**
- **Target**: 1000+ queries per day from AI Service and dashboards
- **Measure**: `contextapi_datastorage_requests_total{endpoint="success-rate-by-incident-type"}`

### **Response Time**
- **Target**: P95 response time <300ms
- **Measure**: `contextapi_datastorage_request_duration_seconds` histogram

---

## 🔄 **Alternatives Considered**

### **Alternative 1: AI Calls Data Storage Directly**

**Approach**: AI Service queries Data Storage without Context API layer

**Rejected Because**:
- ❌ Violates ADR-032 (Data Access Layer Isolation)
- ❌ Tight coupling between AI and Data Storage
- ❌ No centralized caching (each client implements own cache)
- ❌ Harder to migrate Data Storage API changes

---

### **Alternative 2: No Caching Layer**

**Approach**: Context API proxies requests without caching

**Rejected Because**:
- ❌ High load on Data Storage (every query hits database)
- ❌ Slower response time (no fast path)
- ❌ Cannot handle Data Storage outages gracefully

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**  
**Date**: November 5, 2025  
**Decision**: Implement as P0 priority (required for ADR-032 architecture compliance)  
**Rationale**: AI Service and dashboards need proper integration layer to query success rates  
**Approved By**: Architecture Team  
**Related ADR**: [ADR-032: Data Access Layer Isolation](../architecture/decisions/ADR-032-data-access-layer-isolation.md)

---

## 📚 **References**

### **Related Business Requirements**
- BR-STORAGE-031-01: Data Storage incident-type success rate API (upstream dependency)
- BR-AI-057: AI uses success rates for playbook selection (consumes this endpoint)
- BR-INTEGRATION-009: Context API exposes playbook success rate API
- BR-INTEGRATION-010: Context API exposes multi-dimensional success rate API

### **Related Documents**
- [ADR-032: Data Access Layer Isolation](../architecture/decisions/ADR-032-data-access-layer-isolation.md)
- [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-033: Cross-Service BRs](../architecture/decisions/ADR-033-CROSS-SERVICE-BRS.md)
- [ADR-034: BR Template Standard](../architecture/decisions/ADR-034-business-requirement-template-standard.md)

---

**Document Version**: 1.0  
**Last Updated**: November 5, 2025  
**Status**: ✅ Approved for V1 Implementation

