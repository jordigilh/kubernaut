# Context API - ADR-033 Impact Analysis & Implementation Plan Update

**Date**: 2025-11-05  
**Scope**: Context API implementation plan changes after ADR-033 approval  
**Status**: 🔍 **ANALYSIS COMPLETE** - Awaiting user input for implementation approach  

---

## 🎯 **EXECUTIVE SUMMARY**

### **Key Finding**: ADR-033 FUNDAMENTALLY CHANGES Context API's Days 10-12

**Current Plan (v2.8.0)**: Days 10-12 focus on unit tests, E2E tests, and documentation  
**ADR-033 Reality**: Days 10-12 must implement **AggregationService** with Data Storage Service integration

**Impact Level**: **HIGH** - Requires complete rewrite of Days 10-12  
**Effort Estimate**: **24-32 hours** (3-4 days) for ADR-033 aggregation features  
**Confidence**: **90%** - Clear requirements, proven Data Storage endpoints  

---

## 📋 **CURRENT STATE ANALYSIS**

### **Context API v2.8.0 Status**

| Metric | Status | Details |
|--------|--------|---------|
| **Days Complete** | 9/12 | Days 1-9 implementation complete (75%) |
| **Production Code** | ✅ BUILDS | PostgreSQL, Redis, HTTP API, Observability |
| **Passing Tests** | ✅ 91/91 | All non-aggregation tests passing |
| **Disabled Tests** | ⏸️ 2 FILES | `aggregation_service_test.go.v1x`, `vector_test.go.v1x` |
| **ADR-033 Features** | ❌ NOT STARTED | AggregationService is empty stub |

### **Current Days 10-12 Plan** (OBSOLETE)

**Day 10**: Unit Test Completion (8h)
- Write remaining unit tests
- Achieve 90% coverage
- **ISSUE**: No aggregation features to test!

**Day 11**: E2E Testing (8h)
- RemediationProcessing → Context API
- HolmesGPT → Context API
- **ISSUE**: Aggregation endpoints don't exist!

**Day 12**: Documentation (8h)
- Service README
- Design decisions
- **ISSUE**: Can't document unimplemented features!

---

## 🚨 **ADR-033 REQUIREMENTS FOR CONTEXT API**

### **What ADR-033 Adds to Context API**

**New Service Role**: **Aggregation Layer** between AI/LLM Service and Data Storage Service

```
┌─────────────────────────────────────────────────────────────┐
│ ADR-033 ARCHITECTURE                                        │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  AI/LLM Service (Playbook Selection)                        │
│       │                                                     │
│       │ "Which playbook has highest success rate?"         │
│       ↓                                                     │
│  Context API ← ← ← YOU ARE HERE (AGGREGATION LAYER)        │
│       │                                                     │
│       │ Aggregates + Caches success rate data              │
│       ↓                                                     │
│  Data Storage Service (Raw Success Rate Endpoints)          │
│       │                                                     │
│       │ GET /api/v1/success-rate/incident-type             │
│       │ GET /api/v1/success-rate/playbook                  │
│       │ GET /api/v1/success-rate/multi-dimensional         │
│       ↓                                                     │
│  PostgreSQL (resource_action_traces table)                  │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### **New Business Requirements**

| BR ID | Description | Data Storage Endpoint | Context API Responsibility |
|-------|-------------|----------------------|---------------------------|
| **BR-CONTEXT-011** | Incident-Type Success Rate Aggregation | `/api/v1/success-rate/incident-type` | Call endpoint, cache results, expose to AI |
| **BR-CONTEXT-012** | Playbook Success Rate Aggregation | `/api/v1/success-rate/playbook` | Call endpoint, cache results, expose to AI |
| **BR-CONTEXT-013** | Multi-Dimensional Success Rate Aggregation | `/api/v1/success-rate/multi-dimensional` | Call endpoint, cache results, expose to AI |
| **BR-CONTEXT-014** | AI Execution Mode Tracking | Data Storage Service | Query AI execution mode stats |
| **BR-CONTEXT-015** | Playbook Chaining Analytics | Data Storage Service | Track chained playbook success rates |

**Total New BRs**: 5 (BR-CONTEXT-011 through BR-CONTEXT-015)

---

## 🔧 **REQUIRED IMPLEMENTATION CHANGES**

### **1. AggregationService Implementation** (NEW)

**Current State** (`pkg/contextapi/query/router.go`):
```go
// AggregationService is a stub for future aggregation functionality
// ADR-032: Aggregation requires Data Storage Service API support
type AggregationService struct{}
```

**Required State** (ADR-033):
```go
type AggregationService struct {
    dataStorageClient *dsclient.Client  // NEW: HTTP client for Data Storage Service
    cache             *cache.Manager     // EXISTING: Redis + LRU caching
    logger            *zap.Logger        // EXISTING: Structured logging
}

func NewAggregationService(
    dataStorageClient *dsclient.Client,
    cache *cache.Manager,
    logger *zap.Logger,
) *AggregationService {
    return &AggregationService{
        dataStorageClient: dataStorageClient,
        cache:             cache,
        logger:            logger,
    }
}

// BR-CONTEXT-011: Incident-Type Success Rate Aggregation
func (a *AggregationService) GetSuccessRateByIncidentType(
    ctx context.Context,
    params *IncidentTypeAggregationParams,
) (*IncidentTypeSuccessRate, error) {
    // 1. Check cache
    // 2. If miss, call Data Storage Service endpoint
    // 3. Cache result
    // 4. Return to caller
}

// BR-CONTEXT-012: Playbook Success Rate Aggregation
func (a *AggregationService) GetSuccessRateByPlaybook(
    ctx context.Context,
    params *PlaybookAggregationParams,
) (*PlaybookSuccessRate, error) {
    // Similar pattern
}

// BR-CONTEXT-013: Multi-Dimensional Success Rate Aggregation
func (a *AggregationService) GetSuccessRateMultiDimensional(
    ctx context.Context,
    params *MultiDimensionalAggregationParams,
) (*MultiDimensionalSuccessRate, error) {
    // Similar pattern
}
```

**Effort Estimate**: 8-10 hours (Day 10)

---

### **2. Data Storage Service HTTP Client** (NEW)

**Required**: HTTP client to call Data Storage Service REST API

**File**: `pkg/contextapi/datastorage/client.go` (NEW)

```go
package datastorage

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "time"

    "go.uber.org/zap"
)

// Client is an HTTP client for Data Storage Service
type Client struct {
    baseURL    string
    httpClient *http.Client
    logger     *zap.Logger
}

func NewClient(baseURL string, logger *zap.Logger) *Client {
    return &Client{
        baseURL: baseURL,
        httpClient: &http.Client{
            Timeout: 10 * time.Second,
        },
        logger: logger,
    }
}

// GetSuccessRateByIncidentType calls Data Storage Service endpoint
// GET /api/v1/success-rate/incident-type
func (c *Client) GetSuccessRateByIncidentType(
    ctx context.Context,
    incidentType string,
    timeRange string,
    minSamples int,
) (*IncidentTypeSuccessRateResponse, error) {
    // Build URL with query parameters
    u, _ := url.Parse(fmt.Sprintf("%s/api/v1/success-rate/incident-type", c.baseURL))
    q := u.Query()
    q.Set("incident_type", incidentType)
    q.Set("time_range", timeRange)
    q.Set("min_samples", fmt.Sprintf("%d", minSamples))
    u.RawQuery = q.Encode()

    // Make HTTP request
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("failed to call Data Storage Service: %w", err)
    }
    defer resp.Body.Close()

    // Parse response
    var result IncidentTypeSuccessRateResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("failed to parse response: %w", err)
    }

    return &result, nil
}

// Similar methods for playbook and multi-dimensional endpoints
```

**Effort Estimate**: 4-6 hours (Day 10)

---

### **3. HTTP API Endpoints** (NEW)

**Required**: Expose aggregation data to AI/LLM Service

**File**: `pkg/contextapi/server/aggregation_handlers.go` (NEW)

```go
package server

import (
    "encoding/json"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/jordigilh/kubernaut/pkg/contextapi/query"
    "go.uber.org/zap"
)

// HandleGetSuccessRateByIncidentType handles GET /api/v1/aggregation/success-rate/incident-type
// BR-CONTEXT-011: Incident-Type Success Rate Aggregation
func (s *Server) HandleGetSuccessRateByIncidentType(w http.ResponseWriter, r *http.Request) {
    // Parse query parameters
    incidentType := r.URL.Query().Get("incident_type")
    timeRange := r.URL.Query().Get("time_range")
    minSamples := r.URL.Query().Get("min_samples")

    // Validate parameters
    if incidentType == "" {
        s.respondError(w, http.StatusBadRequest, "incident_type is required")
        return
    }

    // Call AggregationService
    result, err := s.aggregationService.GetSuccessRateByIncidentType(r.Context(), &query.IncidentTypeAggregationParams{
        IncidentType: incidentType,
        TimeRange:    timeRange,
        MinSamples:   minSamples,
    })
    if err != nil {
        s.logger.Error("failed to get success rate by incident type",
            zap.String("incident_type", incidentType),
            zap.Error(err))
        s.respondError(w, http.StatusInternalServerError, "failed to retrieve success rate data")
        return
    }

    // Return JSON response
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}

// Similar handlers for playbook and multi-dimensional endpoints
```

**Effort Estimate**: 4-6 hours (Day 11)

---

### **4. Integration Tests** (NEW)

**Required**: Integration tests with real Data Storage Service

**File**: `test/integration/contextapi/11_aggregation_api_test.go` (NEW)

```go
var _ = Describe("Aggregation API - ADR-033", func() {
    Context("when querying incident-type success rate", func() {
        It("should return success rate data from Data Storage Service", func() {
            // ARRANGE: Insert test data via Data Storage Service
            // ACT: Call Context API aggregation endpoint
            resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom-killer&time_range=7d", contextAPIURL))
            Expect(err).ToNot(HaveOccurred())
            defer resp.Body.Close()

            // ASSERT: Verify response
            Expect(resp.StatusCode).To(Equal(http.StatusOK))

            var result models.IncidentTypeSuccessRate
            err = json.NewDecoder(resp.Body).Decode(&result)
            Expect(err).ToNot(HaveOccurred())

            // BEHAVIOR: Returns success rate data
            Expect(result.IncidentType).To(Equal("pod-oom-killer"))
            Expect(result.TotalExecutions).To(BeNumerically(">", 0))

            // CORRECTNESS: Success rate calculation is accurate
            expectedSuccessRate := float64(result.SuccessfulExecutions) / float64(result.TotalExecutions) * 100
            Expect(result.SuccessRate).To(BeNumerically("~", expectedSuccessRate, 0.01))
        })
    })

    Context("when Data Storage Service is unavailable", func() {
        It("should return cached data if available", func() {
            // Test cache fallback
        })

        It("should return 503 Service Unavailable if no cache", func() {
            // Test error handling
        })
    })
})
```

**Effort Estimate**: 6-8 hours (Day 11)

---

### **5. Unit Tests** (RE-ENABLE + ENHANCE)

**Required**: Re-enable disabled tests + add new tests

**Files**:
- `test/unit/contextapi/aggregation_service_test.go.v1x` → `.go` (RE-ENABLE)
- `test/unit/contextapi/datastorage_client_test.go` (NEW)
- `test/unit/contextapi/aggregation_handlers_test.go` (NEW)

**Effort Estimate**: 4-6 hours (Day 10)

---

### **6. Documentation Updates** (REQUIRED)

**Files to Update**:
1. `docs/services/stateless/context-api/README.md` - Add aggregation features
2. `docs/services/stateless/context-api/api-specification.md` - Document new endpoints
3. `docs/services/stateless/context-api/integration-points.md` - Add Data Storage Service dependency
4. `docs/architecture/decisions/DD-CONTEXT-003-aggregation-layer.md` (NEW)

**Effort Estimate**: 4-6 hours (Day 12)

---

## 📊 **REVISED DAYS 10-12 PLAN**

### **NEW Day 10: AggregationService Implementation** (8-10 hours)

**Objective**: Implement AggregationService with Data Storage Service integration

**TDD Phases**:
1. **RED** (2h): Write unit tests for `AggregationService` methods (15 tests)
2. **GREEN** (4h): Implement `AggregationService` + Data Storage HTTP client
3. **REFACTOR** (2h): Add caching, error handling, structured logging

**Deliverables**:
- ✅ `pkg/contextapi/query/aggregation.go` (AggregationService implementation)
- ✅ `pkg/contextapi/datastorage/client.go` (Data Storage HTTP client)
- ✅ `test/unit/contextapi/aggregation_service_test.go` (15 unit tests)
- ✅ `test/unit/contextapi/datastorage_client_test.go` (10 unit tests)

**BRs Covered**: BR-CONTEXT-011, BR-CONTEXT-012, BR-CONTEXT-013

**Confidence**: 90% (Data Storage endpoints exist, proven patterns)

---

### **NEW Day 11: HTTP API + Integration Tests** (8-10 hours)

**Objective**: Expose aggregation endpoints and validate with Data Storage Service

**TDD Phases**:
1. **RED** (2h): Write integration tests for aggregation endpoints (10 tests)
2. **GREEN** (4h): Implement HTTP handlers + route registration
3. **REFACTOR** (2h): Add RFC 7807 error handling, observability metrics

**Deliverables**:
- ✅ `pkg/contextapi/server/aggregation_handlers.go` (3 HTTP handlers)
- ✅ `test/integration/contextapi/11_aggregation_api_test.go` (10 integration tests)
- ✅ OpenAPI spec updates (3 new endpoints)

**BRs Covered**: BR-CONTEXT-011, BR-CONTEXT-012, BR-CONTEXT-013

**Confidence**: 85% (HTTP patterns established, Data Storage integration new)

---

### **NEW Day 12: E2E Tests + Documentation** (8-10 hours)

**Objective**: Validate AI/LLM → Context API → Data Storage flow + complete documentation

**Tasks**:
1. **E2E Tests** (4h): AI/LLM Service → Context API aggregation → Data Storage Service
2. **Documentation** (4h): Update README, API spec, integration points, DD-CONTEXT-003

**Deliverables**:
- ✅ `test/e2e/contextapi/01_ai_playbook_selection_test.go` (5 E2E tests)
- ✅ `docs/architecture/decisions/DD-CONTEXT-003-aggregation-layer.md` (NEW)
- ✅ Updated service documentation (README, API spec, integration points)

**BRs Covered**: BR-CONTEXT-011, BR-CONTEXT-012, BR-CONTEXT-013

**Confidence**: 80% (E2E tests require AI/LLM Service integration)

---

## ❓ **QUESTIONS FOR USER**

### **Q1: Implementation Approach**

**Option A: Rewrite Days 10-12 Now** (RECOMMENDED)
- **Pros**: ✅ Complete ADR-033 implementation, ✅ Context API ready for AI/LLM Service
- **Cons**: ⚠️ 24-32 hours of work, ⚠️ Delays Context API completion
- **Confidence**: 90%

**Option B: Defer ADR-033 to Phase 2**
- **Pros**: ✅ Complete current Days 10-12 quickly (unit tests, E2E, docs)
- **Cons**: ❌ Context API won't support AI playbook selection, ❌ Rework needed later
- **Confidence**: 60%

**Option C: Minimal ADR-033 Implementation**
- **Pros**: ✅ Basic aggregation support, ✅ Faster than Option A
- **Cons**: ⚠️ Incomplete features, ⚠️ May need rework
- **Confidence**: 70%

**Which option do you prefer?**

---

### **Q2: Business Requirements Numbering**

**Current Context API BRs**: BR-CONTEXT-001 through BR-CONTEXT-010 (Days 1-9)

**ADR-033 New BRs**: Should they be:
- **Option A**: BR-CONTEXT-011 through BR-CONTEXT-015 (sequential)
- **Option B**: BR-AGGREGATION-001 through BR-AGGREGATION-005 (new category)
- **Option C**: BR-AI-059 through BR-AI-063 (AI category, since it's for AI playbook selection)

**Which BR category makes most sense?**

---

### **Q3: Data Storage Service Dependency**

**Current Context API Dependencies**:
- PostgreSQL (direct connection)
- Redis (direct connection)

**ADR-033 Adds**:
- Data Storage Service (HTTP REST API)

**Question**: Should Context API:
- **Option A**: Keep direct PostgreSQL access + add Data Storage HTTP client (hybrid)
- **Option B**: Migrate ALL queries to Data Storage Service (API Gateway pattern per DD-ARCH-001)
- **Option C**: Use Data Storage ONLY for aggregation, keep direct PostgreSQL for other queries

**Which dependency model do you prefer?**

---

### **Q4: Caching Strategy for Aggregation Data**

**Aggregation data characteristics**:
- Changes infrequently (new action executions)
- Expensive to compute (multi-dimensional queries)
- Critical for AI playbook selection performance

**Question**: What cache TTL should we use?
- **Option A**: 5 minutes (same as current Context API cache TTL)
- **Option B**: 15 minutes (aggregation data changes slowly)
- **Option C**: 30 minutes (maximize cache hits, AI can tolerate slightly stale data)
- **Option D**: Configurable per endpoint (incident-type: 15min, playbook: 30min, multi-dimensional: 5min)

**Which caching strategy do you prefer?**

---

### **Q5: Integration Test Infrastructure**

**Current Context API Integration Tests**:
- Use Podman containers (PostgreSQL + Redis)
- No external service dependencies

**ADR-033 Integration Tests Need**:
- Data Storage Service running (HTTP server)
- PostgreSQL (for Data Storage Service)
- Redis (for Context API caching)

**Question**: How should we run Data Storage Service in integration tests?
- **Option A**: Start Data Storage Service in Podman container (like PostgreSQL/Redis)
- **Option B**: Start Data Storage Service as Go process in test setup
- **Option C**: Mock Data Storage Service HTTP responses (httptest)

**Which integration test approach do you prefer?**

---

## 📄 **NEXT STEPS**

**After User Input**:
1. ✅ Update `IMPLEMENTATION_PLAN_V2.8.md` to V2.9 with revised Days 10-12
2. ✅ Create formal BR documents (BR-CONTEXT-011 through BR-CONTEXT-015 or alternative)
3. ✅ Update `DESIGN_DECISIONS.md` index with DD-CONTEXT-003
4. ✅ Begin Day 10 implementation (AggregationService + Data Storage HTTP client)

---

## 🔗 **REFERENCES**

- **ADR-033**: `docs/architecture/decisions/ADR-033-remediation-playbook-catalog.md`
- **Context API Plan**: `docs/services/stateless/context-api/implementation/IMPLEMENTATION_PLAN_V2.8.md`
- **Data Storage Plan**: `docs/services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V5.3.md`
- **Data Storage OpenAPI**: `docs/services/stateless/data-storage/openapi/v2.yaml`
- **Unit Test Triage**: `CONTEXT_API_UNIT_TEST_TRIAGE.md`
- **Failures Analysis**: `CONTEXT_API_UNIT_TEST_FAILURES_ANALYSIS.md`

---

**Analysis Completed By**: AI Assistant  
**Analysis Date**: 2025-11-05  
**Status**: ⏳ **AWAITING USER INPUT** - 5 questions to answer before proceeding

