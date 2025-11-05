# BR-INTEGRATION-009: Expose Playbook Success Rate API

**Business Requirement ID**: BR-INTEGRATION-009  
**Category**: Context API (Integration Service)  
**Priority**: P1  
**Target Version**: V1  
**Status**: ✅ Approved  
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 requires AI Service and Operations Dashboard to query playbook-specific success rates for version comparison and effectiveness analysis. The Context API must expose Data Storage's playbook success rate endpoint (BR-STORAGE-031-02) to upstream clients.

**Current Limitations**:
- ❌ Context API does not expose playbook success rate endpoint
- ❌ AI Service cannot compare playbook version effectiveness
- ❌ Operations Dashboard cannot display playbook version charts
- ❌ Violates ADR-032: AI should not call Data Storage directly

**Impact**:
- BR-AI-057 blocked: AI cannot compare playbook versions for optimal selection
- Teams cannot validate playbook version improvements
- Missing centralized API for playbook effectiveness queries
- Architectural violation if clients bypass Context API

---

## 🎯 **Business Objective**

**Expose Data Storage's playbook success rate API through Context API to enable AI Service and Operations Dashboard to query playbook version effectiveness.**

### **Success Criteria**
1. ✅ Context API exposes `GET /incidents/aggregate/success-rate/by-playbook` endpoint
2. ✅ Context API proxies requests to Data Storage with caching
3. ✅ Response includes playbook success rate + incident-type breakdown + action-level breakdown
4. ✅ AI Service uses Context API endpoint (not Data Storage directly)
5. ✅ Operations Dashboard uses Context API for playbook version comparison charts
6. ✅ Cache TTL: 5 minutes (balance freshness vs load)
7. ✅ Response time <300ms (including Data Storage query + caching)

---

## 📊 **Use Cases**

### **Use Case 1: AI Compares Playbook Versions**

**Scenario**: AI needs to compare `pod-oom-recovery v1.2` vs `v1.1` effectiveness.

**Current Flow** (Without BR-INTEGRATION-009):
```
1. AI needs to compare playbook versions
2. No Context API endpoint available
3. ❌ AI either:
   - Option A: Calls Data Storage directly (violates ADR-032)
   - Option B: Cannot compare versions (blocks version-based selection)
```

**Desired Flow with BR-INTEGRATION-009**:
```
1. AI queries Context API for both versions:
   - GET /api/v1/incidents/aggregate/success-rate/by-playbook?playbook_id=pod-oom-recovery&playbook_version=v1.2
   - GET /api/v1/incidents/aggregate/success-rate/by-playbook?playbook_id=pod-oom-recovery&playbook_version=v1.1
2. Context API checks cache for both queries
3. Context API returns:
   - v1.2: success_rate=0.89, total_executions=90
   - v1.1: success_rate=0.40, total_executions=10
4. ✅ AI selects v1.2 (39% higher success rate)
5. ✅ Respects ADR-032 integration layer
6. ✅ Cached responses for subsequent queries
```

---

### **Use Case 2: Operations Dashboard - Playbook Version Chart**

**Scenario**: Dashboard displays success rate comparison for all versions of `pod-oom-recovery`.

**Current Flow**:
```
1. Dashboard needs success rates for 3 versions (v1.0, v1.1, v1.2)
2. No centralized API available
3. ❌ Dashboard calls Data Storage directly (3 queries, no caching)
4. ❌ High load on Data Storage
```

**Desired Flow with BR-INTEGRATION-009**:
```
1. Dashboard queries Context API for each version:
   - GET /by-playbook?playbook_id=pod-oom-recovery&playbook_version=v1.0
   - GET /by-playbook?playbook_id=pod-oom-recovery&playbook_version=v1.1
   - GET /by-playbook?playbook_id=pod-oom-recovery&playbook_version=v1.2
2. Context API serves 2/3 from cache (hit rate: 67%)
3. Context API queries Data Storage for 1 cache miss
4. Dashboard renders chart:
   v1.0: 50% success (deprecated)
   v1.1: 40% success (deprecated)
   v1.2: 89% success (active)
5. ✅ Visual validation of playbook improvement
6. ✅ Reduced Data Storage load (1 query vs 3)
```

---

### **Use Case 3: Identify Playbook Step Failures**

**Scenario**: `database-recovery v2.0` has 60% success rate. Team uses action-level breakdown to identify failure hotspot.

**Current Flow**:
```
1. Team observes low success rate
2. No action-level breakdown available in Context API
3. ❌ Cannot identify which step fails
4. ❌ Time-consuming manual log analysis required
```

**Desired Flow with BR-INTEGRATION-009**:
```
1. Team queries Context API:
   GET /by-playbook?playbook_id=database-recovery&playbook_version=v2.0
2. Response includes breakdown_by_action:
   [
     {"action_type": "stop_pod", "step_number": 1, "success_rate": 0.95},
     {"action_type": "backup_database", "step_number": 2, "success_rate": 0.90},
     {"action_type": "restore_database", "step_number": 3, "success_rate": 0.40},  ← FAILURE HOTSPOT
     {"action_type": "start_pod", "step_number": 4, "success_rate": 0.98}
   ]
3. ✅ Team identifies: Step 3 (restore_database) causes 60% of failures
4. ✅ Team targets fix at restore_database step
5. ✅ Faster troubleshooting with data-driven insights
```

---

## 🔧 **Functional Requirements**

### **FR-INTEGRATION-009-01: Context API Endpoint**

**Requirement**: Context API SHALL expose playbook success rate endpoint that proxies Data Storage.

**Endpoint Specification**:
```http
GET /api/v1/incidents/aggregate/success-rate/by-playbook

Query Parameters:
- playbook_id (string, required): Playbook identifier (e.g., "pod-oom-recovery")
- playbook_version (string, required): Playbook version (e.g., "v1.2")
- time_range (string, optional, default: "7d"): Time window (7d, 30d, 90d)
- min_samples (int, optional, default: 5): Minimum executions required

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

**Implementation Example**:
```go
package contextapi

// handleGetSuccessRateByPlaybook proxies to Data Storage
func (s *Server) handleGetSuccessRateByPlaybook(w http.ResponseWriter, r *http.Request) {
    playbookID := r.URL.Query().Get("playbook_id")
    playbookVersion := r.URL.Query().Get("playbook_version")
    
    if playbookID == "" || playbookVersion == "" {
        s.respondError(w, http.StatusBadRequest, "playbook_id and playbook_version are required")
        return
    }

    timeRange := r.URL.Query().Get("time_range")
    if timeRange == "" {
        timeRange = "7d"
    }

    // Check cache
    cacheKey := fmt.Sprintf("success-rate:playbook:%s:%s:%s", playbookID, playbookVersion, timeRange)
    if cached, found := s.cache.Get(cacheKey); found {
        s.logger.Debug("cache hit", zap.String("cache_key", cacheKey))
        s.respondJSON(w, http.StatusOK, cached)
        return
    }

    // Cache miss: query Data Storage
    response, err := s.dataStorageClient.GetSuccessRateByPlaybook(r.Context(), playbookID, playbookVersion, timeRange)
    if err != nil {
        s.logger.Error("failed to query Data Storage",
            zap.String("playbook_id", playbookID),
            zap.String("playbook_version", playbookVersion),
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
- ✅ Returns 200 OK for valid requests
- ✅ Returns 400 Bad Request for missing playbook_id or playbook_version
- ✅ Response format matches Data Storage exactly (transparent proxy)
- ✅ Caching layer with 5-minute TTL
- ✅ Cache key includes playbook_id + playbook_version + time_range

---

### **FR-INTEGRATION-009-02: Data Storage Client Integration**

**Requirement**: Context API SHALL use Data Storage REST API client to query playbook success rates.

**Client Implementation**:
```go
// GetSuccessRateByPlaybook queries playbook-specific success rate
func (c *DataStorageClient) GetSuccessRateByPlaybook(ctx context.Context, playbookID, playbookVersion, timeRange string) (*PlaybookSuccessRateResponse, error) {
    url := fmt.Sprintf("%s/api/v1/incidents/aggregate/success-rate/by-playbook?playbook_id=%s&playbook_version=%s&time_range=%s",
        c.baseURL, url.QueryEscape(playbookID), url.QueryEscape(playbookVersion), timeRange)

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

    var successRate PlaybookSuccessRateResponse
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
- ✅ Client timeout: 2 seconds
- ✅ Client retries transient failures (503)

---

### **FR-INTEGRATION-009-03: Caching Strategy**

**Requirement**: Context API SHALL implement Redis-based caching with 5-minute TTL for playbook queries.

**Caching Details**:
- **Cache Key Format**: `success-rate:playbook:{playbook_id}:{playbook_version}:{time_range}`
- **TTL**: 5 minutes
- **Eviction Policy**: LRU
- **Cache Size**: 1000 entries

**Acceptance Criteria**:
- ✅ Cache hit rate >70% for typical usage
- ✅ Cache expires after 5 minutes
- ✅ Prometheus metrics: `contextapi_cache_hits_total{endpoint="playbook"}`, `contextapi_cache_misses_total{endpoint="playbook"}`

---

## 📈 **Non-Functional Requirements**

### **NFR-INTEGRATION-009-01: Performance**

- ✅ Response time <300ms for 95th percentile (cached: <50ms, uncached: <250ms)
- ✅ Cache hit rate >70%
- ✅ Support 200 concurrent requests

### **NFR-INTEGRATION-009-02: Reliability**

- ✅ Graceful degradation if Data Storage unavailable (return cached data even if expired)
- ✅ Circuit breaker for Data Storage calls (open after 5 failures)
- ✅ Timeout: 2 seconds for Data Storage queries

### **NFR-INTEGRATION-009-03: Observability**

- ✅ Log all Data Storage queries with latency
- ✅ Prometheus metrics:
  - `contextapi_datastorage_requests_total{endpoint="playbook_success_rate", status="success|error"}`
  - `contextapi_datastorage_request_duration_seconds{endpoint="playbook_success_rate"}`
  - `contextapi_cache_hits_total{endpoint="playbook"}`

---

## 🔗 **Dependencies**

### **Upstream Dependencies**
- ✅ ADR-032: Data Access Layer Isolation (Context API as integration layer)
- ✅ BR-STORAGE-031-02: Data Storage playbook success rate endpoint exists
- ✅ BR-INTEGRATION-008: Context API caching infrastructure established
- ✅ Redis: For caching layer

### **Downstream Impacts**
- ✅ BR-AI-057: AI Service uses Context API for playbook version comparison
- ✅ Operations Dashboard: Uses Context API for playbook version charts

---

## 🚀 **Implementation Phases**

### **Phase 1: Data Storage Client Extension** (Day 6 - 2 hours)
- Extend `DataStorageClient` with `GetSuccessRateByPlaybook()` method
- Add timeout and retry logic
- Unit tests

### **Phase 2: Context API Endpoint** (Day 6 - 3 hours)
- Implement `handleGetSuccessRateByPlaybook` handler
- Integrate Data Storage client + caching
- Add error handling and logging

### **Phase 3: Testing** (Day 7 - 3 hours)
- Unit tests: Handler logic, caching behavior
- Integration tests: Full endpoint with real Data Storage
- Test breakdown response structures (incident-type + action-level)

### **Phase 4: OpenAPI Spec** (Day 7 - 1 hour)
- Update OpenAPI spec with new endpoint
- Add request/response examples with breakdowns

**Total Estimated Effort**: 9 hours (1.125 days)

---

## 📊 **Success Metrics**

### **Cache Hit Rate**
- **Target**: 70%+ cache hit rate
- **Measure**: `contextapi_cache_hits_total{endpoint="playbook"}` / (hits + misses)

### **API Usage**
- **Target**: 500+ queries per day from AI Service and dashboards
- **Measure**: `contextapi_datastorage_requests_total{endpoint="playbook_success_rate"}`

### **Response Time**
- **Target**: P95 response time <300ms
- **Measure**: `contextapi_datastorage_request_duration_seconds` histogram

---

## 🔄 **Alternatives Considered**

### **Alternative 1: AI Calls Data Storage Directly**

**Approach**: AI Service queries Data Storage without Context API layer

**Rejected Because**:
- ❌ Violates ADR-032 (Data Access Layer Isolation)
- ❌ No centralized caching
- ❌ Tight coupling

---

### **Alternative 2: Combine with Incident-Type Endpoint**

**Approach**: Single endpoint for both incident-type and playbook queries

**Rejected Because**:
- ❌ Overly complex API (too many query parameters)
- ❌ Harder to cache (combinatorial explosion of cache keys)
- ❌ Less clear API semantics

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**  
**Date**: November 5, 2025  
**Decision**: Implement as P1 priority (enables playbook version comparison)  
**Rationale**: Required for AI version-based selection and playbook improvement validation  
**Approved By**: Architecture Team  
**Related ADR**: [ADR-032: Data Access Layer Isolation](../architecture/decisions/ADR-032-data-access-layer-isolation.md)

---

## 📚 **References**

### **Related Business Requirements**
- BR-STORAGE-031-02: Data Storage playbook success rate API (upstream dependency)
- BR-INTEGRATION-008: Context API incident-type success rate endpoint
- BR-AI-057: AI uses success rates for playbook selection
- BR-PLAYBOOK-002: Playbook versioning uses effectiveness data

### **Related Documents**
- [ADR-032: Data Access Layer Isolation](../architecture/decisions/ADR-032-data-access-layer-isolation.md)
- [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-033: Cross-Service BRs](../architecture/decisions/ADR-033-CROSS-SERVICE-BRS.md)
- [ADR-034: BR Template Standard](../architecture/decisions/ADR-034-business-requirement-template-standard.md)

---

**Document Version**: 1.0  
**Last Updated**: November 5, 2025  
**Status**: ✅ Approved for V1 Implementation

