# Context API - API Gateway Migration

**Related Decision**: [DD-ARCH-001: Alternative 2 (API Gateway Pattern)](../../../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md)
**Date**: November 2, 2025
**Status**: ✅ **APPROVED FOR IMPLEMENTATION**
**Service**: Context API
**Timeline**: **2-3 Days** (Phase 2 of overall migration)
**Depends On**: [Data Storage Service Phase 1](../../data-storage/implementation/API-GATEWAY-MIGRATION.md) ✅ Must complete first

---

## 🎯 **WHAT THIS SERVICE NEEDS TO DO**

**Current State**: Context API queries PostgreSQL directly using SQL builder

**New State**: Context API queries Data Storage Service REST API (still caches results)

**Changes Needed**:
1. ✅ Replace direct SQL queries with HTTP client calls to Data Storage Service
2. ✅ Keep Redis (L1) + LRU (L2) caching (no changes to cache logic)
3. ✅ Update service specification
4. ✅ Update integration test infrastructure (start Data Storage Service in tests)

---

## 📋 **SPECIFICATION CHANGES**

### **1. Service Overview Update**

**File**: `overview.md`

**Current**:
> Context API queries PostgreSQL directly for historical incident data.

**New**:
> Context API queries historical incidents via **Data Storage Service REST API**.
>
> **Query Flow** (NEW):
> ```
> AI Request
>   → Context API (Check L1/L2 cache)
>     → [Cache MISS] → Data Storage Service REST API
>       → PostgreSQL
>     ← Incident Data
>   ← Cached Result
> ```
>
> **Design Decision**: [DD-ARCH-001 Alternative 2](../../../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md)

---

### **2. Integration Points Update**

**File**: `integration-points.md`

**Current**:
> **Downstream**:
> - PostgreSQL (Direct SQL queries)

**New**:
> **Downstream**:
> - **Data Storage Service REST API** (Historical queries)
>   - Endpoint: `GET /api/v1/incidents?namespace={ns}&severity={sev}&...`
>   - Client: `pkg/datastorage/client/http_client.go`
> - Redis (L1 cache - unchanged)
>
> **Design Decision**: [DD-ARCH-001 Alternative 2](../../../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md)

---

## 🚀 **IMPLEMENTATION PLAN**

### **Phase 0: Documentation Updates** (1-1.5 hours)

**Status**: ✅ **Defined above**

**Tasks**:
1. Update `overview.md` (query flow diagram)
2. Update `integration-points.md` (Data Storage Service client)

**Deliverables**:
- ✅ Service specification reflects new architecture

---

### **Day 1: HTTP Client for Data Storage Service** (4-6 hours)

**Objective**: Create HTTP client to replace direct SQL queries

**Tasks**:
1. Create `pkg/datastorage/client/` package
2. Implement `Client` interface with `ListIncidents()` method
3. Create HTTP client implementation
4. Add retries, timeouts, circuit breaker (if needed)

**New Files**:
- `pkg/datastorage/client/client.go` - Interface definition
- `pkg/datastorage/client/http_client.go` - HTTP implementation
- `pkg/datastorage/client/models.go` - Request/response models

**Example Client**:
```go
// pkg/datastorage/client/client.go
type Client interface {
    ListIncidents(ctx context.Context, params *ListParams) (*ListResponse, error)
}

// pkg/datastorage/client/http_client.go
type HTTPClient struct {
    baseURL string
    client  *http.Client
}

func (c *HTTPClient) ListIncidents(ctx context.Context, params *ListParams) (*ListResponse, error) {
    url := fmt.Sprintf("%s/api/v1/incidents?%s", c.baseURL, params.ToQueryString())

    resp, err := c.client.Get(url)
    // ... handle response

    var result ListResponse
    json.NewDecoder(resp.Body).Decode(&result)
    return &result, nil
}
```

**Deliverables**:
- ✅ HTTP client created
- ✅ Unit tests for client
- ✅ Ready to replace SQL queries

---

### **Day 2: Replace Direct SQL with HTTP Client** (3-4 hours)

**Objective**: Update `query/executor.go` to use Data Storage Service client

**Tasks**:
1. Update `CachedQueryExecutor` to inject `datastorage.Client`
2. Replace SQL builder calls with HTTP client calls
3. Keep caching logic unchanged
4. Remove direct database connection (read-only)

**Changes in `pkg/contextapi/query/executor.go`**:

**Before**:
```go
type CachedQueryExecutor struct {
    db          *sqlx.DB        // Direct SQL
    sqlBuilder  *sqlbuilder.Builder
    cacheManager *cache.CacheManager
}

func (e *CachedQueryExecutor) QueryIncidents(ctx context.Context, params *models.ListIncidentsParams) ([]*models.IncidentEvent, error) {
    // Check cache first
    if cached := e.cacheManager.Get(cacheKey); cached != nil {
        return cached, nil
    }

    // Build SQL query
    query, args, _ := e.sqlBuilder.WithNamespace(params.Namespace).Build()

    // Execute direct SQL
    var incidents []*models.IncidentEvent
    e.db.SelectContext(ctx, &incidents, query, args...)

    // Cache and return
    e.cacheManager.Set(cacheKey, incidents)
    return incidents, nil
}
```

**After**:
```go
type CachedQueryExecutor struct {
    storageClient datastorage.Client  // HTTP client
    cacheManager  *cache.CacheManager
}

func (e *CachedQueryExecutor) QueryIncidents(ctx context.Context, params *models.ListIncidentsParams) ([]*models.IncidentEvent, error) {
    // Check cache first (UNCHANGED)
    if cached := e.cacheManager.Get(cacheKey); cached != nil {
        return cached, nil
    }

    // Query via Data Storage Service REST API
    response, err := e.storageClient.ListIncidents(ctx, &datastorage.ListParams{
        Namespace: params.Namespace,
        Severity:  params.Severity,
        Limit:     params.Limit,
        Offset:    params.Offset,
    })
    if err != nil {
        return nil, fmt.Errorf("data storage query failed: %w", err)
    }

    // Cache and return (UNCHANGED)
    e.cacheManager.Set(cacheKey, response.Incidents)
    return response.Incidents, nil
}
```

**Code Removed**:
- Direct `sqlx.DB` dependency (~20 lines)
- SQL builder usage (~50 lines)

**Code Added**:
- HTTP client dependency (~10 lines)
- HTTP client calls (~20 lines)

**Net Change**: ~40 lines removed, ~30 lines added

**Deliverables**:
- ✅ Context API uses Data Storage Service REST API
- ✅ Caching logic unchanged
- ✅ Direct database connection removed (for reads)

---

### **Day 3: Update Integration Test Infrastructure** (2-4 hours)

**Objective**: Integration tests now start Data Storage Service

**Tasks**:
1. Create test helper to start Data Storage Service HTTP server
2. Update `BeforeSuite()` to start both PostgreSQL AND Data Storage Service
3. Update `AfterSuite()` to stop both
4. Verify all integration tests pass

**Changes in `test/integration/contextapi/context_api_suite_test.go`**:

**Before**:
```go
var _ = BeforeSuite(func() {
    // Start PostgreSQL
    db = testutil.StartPostgreSQL()
})
```

**After**:
```go
var (
    db            *sqlx.DB
    storageServer *datastorage.Server  // NEW
    storageClient datastorage.Client   // NEW
)

var _ = BeforeSuite(func() {
    // Start PostgreSQL
    db = testutil.StartPostgreSQL()

    // Start Data Storage Service (NEW)
    storageServer = datastorage.NewServer(&datastorage.Config{
        DBConnection: db,
        Port:        8081,
    })
    go storageServer.Start()
    testutil.WaitForHTTP("http://localhost:8081/health")

    // Create client for Context API to use
    storageClient = datastorage.NewHTTPClient("http://localhost:8081")
})

var _ = AfterSuite(func() {
    storageServer.Shutdown()  // NEW
    db.Close()
})
```

**Deliverables**:
- ✅ Integration tests updated
- ✅ All tests passing (Context API → Data Storage Service → PostgreSQL)

---

## ✅ **SUCCESS CRITERIA**

- ✅ HTTP client for Data Storage Service implemented
- ✅ Context API queries via REST API (not direct SQL)
- ✅ Caching logic unchanged (Redis L1 + LRU L2)
- ✅ Integration tests updated (start Data Storage Service)
- ✅ All unit + integration tests passing
- ✅ Service specification updated
- ✅ **Context API successfully migrated to API Gateway pattern**

---

## 📊 **CODE IMPACT SUMMARY**

| Component | Change | Lines |
|-----------|--------|-------|
| Direct SQL queries | **REMOVED** | -70 |
| HTTP client | **ADDED** | +150 |
| Caching logic | **UNCHANGED** | 0 |
| Integration test infra | **UPDATED** | +50 |
| **Net Change** | | **+130 lines** |

**Confidence**: 95% - Straightforward HTTP client replacement, caching logic untouched

---

## 🔗 **RELATED DOCUMENTATION**

- [DD-ARCH-001 Final Decision](../../../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md) - Architecture decision
- [Data Storage Service Migration](../../data-storage/implementation/API-GATEWAY-MIGRATION.md) - Phase 1 (dependency)
- [Effectiveness Monitor Migration](../../effectiveness-monitor/implementation/API-GATEWAY-MIGRATION.md) - Phase 3 (parallel)

---

**Status**: ✅ **APPROVED - Ready for implementation after Data Storage Service Phase 1**
**Dependencies**: Data Storage Service REST API must be implemented first
**Parallel Work**: Can be done in parallel with Effectiveness Monitor migration

