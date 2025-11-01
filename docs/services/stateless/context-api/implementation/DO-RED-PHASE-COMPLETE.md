# Context API Migration - DO-RED Phase Complete

**Date**: November 1, 2025  
**Phase**: DO-RED ✅ COMPLETE  
**Component**: `pkg/contextapi/query/executor.go`  
**Test File**: `test/unit/contextapi/executor_datastorage_migration_test.go`  
**Commits**: ba61d264

---

## ✅ **DO-RED Phase Summary**

### **What Was Accomplished**

Created **11 comprehensive failing unit tests** (406 lines) that define the contract for migrating Context API from direct PostgreSQL queries to Data Storage Service REST API.

---

## 📋 **Test Coverage Matrix**

### **BR-CONTEXT-007: HTTP Client Integration** (4 tests)

| Test | Purpose | Expected Behavior |
|---|---|---|
| `should use Data Storage REST API` | Verify API integration | Replace PostgreSQL query with HTTP GET to `/api/v1/incidents` |
| `should pass namespace filters` | Parameter mapping | Map `namespace` parameter to API query string |
| `should pass severity filters` | Parameter mapping | Map `severity` parameter to API query string |
| `should get total count from pagination` | Pagination metadata | Extract `total` from API response, not SQL COUNT |

**Key Implementation Requirement**:
- Replace `e.db.SelectContext()` with `dsClient.ListIncidents()`
- Map `models.ListIncidentsParams` → Data Storage API query parameters
- Extract `total` from `pagination.total` in API response

---

### **BR-CONTEXT-008: Circuit Breaker** (2 tests)

| Test | Purpose | Threshold |
|---|---|---|
| `should open circuit breaker after 3 failures` | Prevent cascade failures | 3 consecutive failures → open for 60s |
| `should close after timeout` | Auto-recovery | Skipped (timing test) |

**Key Implementation Requirement**:
- Track consecutive failures per ADR-030 configuration (`circuit_breaker.threshold: 3`)
- Open circuit after threshold reached
- Block requests with "circuit breaker open" error

---

### **BR-CONTEXT-009: Exponential Backoff Retry** (2 tests)

| Test | Purpose | Retry Pattern |
|---|---|---|
| `should retry with exponential backoff` | Transient error recovery | 100ms → 200ms → 400ms |
| `should give up after 3 attempts` | Prevent infinite retries | Max 3 attempts per ADR-030 |

**Key Implementation Requirement**:
- Implement retry logic per ADR-030 configuration:
  - `retry.max_attempts: 3`
  - `retry.base_delay: 100ms`
  - `retry.max_delay: 400ms`
- Exponential backoff: delay = base_delay * 2^attempt

---

### **BR-CONTEXT-010: Graceful Degradation** (2 tests)

| Test | Purpose | Fallback Strategy |
|---|---|---|
| `should return cached data when service down` | Availability > freshness | Return Redis cache when Data Storage unavailable |
| `should return error when cache empty` | Fail fast when no data | Return connection error if cache miss |

**Key Implementation Requirement**:
- On Data Storage failure → check Redis cache first
- Return cached data with warning log
- Only error if both Data Storage AND cache unavailable

---

### **RFC 7807 Error Handling** (1 test)

| Test | Purpose | Expected Behavior |
|---|---|---|
| `should parse RFC 7807 error details` | Structured error propagation | Parse `title`, `detail` from RFC 7807 response |

**Key Implementation Requirement**:
- Detect `Content-Type: application/problem+json`
- Parse RFC 7807 error structure
- Propagate `title` and `detail` in error message

---

## 🎯 **Implementation Target**

### **File**: `pkg/contextapi/query/executor.go`

#### **Methods to Replace** (RED Phase Scope)

1. **`queryDatabase()`** (lines 144-193)
   ```go
   // CURRENT: Direct PostgreSQL query
   err = e.db.SelectContext(ctx, &rows, query, args...)
   
   // FUTURE: Data Storage REST API
   incidents, err := e.dsClient.ListIncidents(ctx, params)
   ```

2. **`getTotalCount()`** (lines 195-227)
   ```go
   // CURRENT: SQL COUNT(*) query
   err = e.db.GetContext(ctx, &total, countQuery, args...)
   
   // FUTURE: Pagination metadata from API
   total = response.Pagination.Total
   ```

#### **New Constructor Required**

```go
// REQUIRED for GREEN phase
func NewCachedExecutorWithDataStorage(dsClient *dsclient.DataStorageClient) *CachedExecutor {
    // Initialize with Data Storage client
    // Configure circuit breaker, retry, cache
}
```

---

## 📊 **Test Execution Results**

### **Current Status** (RED Phase Complete)

```bash
$ go test -v ./test/unit/contextapi/executor_datastorage_migration_test.go

# Expected failure (RED phase):
undefined: query.NewCachedExecutorWithDataStorage
```

**Status**: ✅ **Correctly Failing**  
**Reason**: Constructor `NewCachedExecutorWithDataStorage` not yet implemented (GREEN phase work)

---

## 🔧 **Configuration Requirements** (from ADR-030)

The tests validate behavior defined in Context API configuration:

```yaml
# config/context-api.yaml (relevant section)
datastorage:
  url: "http://data-storage-service.kubernaut-system.svc.cluster.local:8080"
  timeout: "5s"
  max_connections: 100
  
  circuit_breaker:
    threshold: 3                     # Test: open after 3 failures
    timeout: "60s"                   # Time before half-open test
  
  retry:
    max_attempts: 3                  # Test: give up after 3 attempts
    base_delay: "100ms"              # Test: first retry delay
    max_delay: "400ms"               # Test: exponential cap (100→200→400)
```

---

## 🚀 **Next Steps** (DO-GREEN Phase)

### **1. Implement Constructor** (Priority: P0)

```go
// pkg/contextapi/query/executor.go

func NewCachedExecutorWithDataStorage(dsClient *dsclient.DataStorageClient) *CachedExecutor {
    return &CachedExecutor{
        dsClient: dsClient,
        // Initialize circuit breaker, retry, cache
    }
}
```

### **2. Replace PostgreSQL Methods** (Priority: P0)

- Modify `ListIncidents()` to use `e.dsClient.ListIncidents()`
- Remove `queryDatabase()` PostgreSQL logic
- Remove `getTotalCount()` SQL COUNT logic
- Add circuit breaker wrapper
- Add retry wrapper with exponential backoff
- Add cache fallback on Data Storage failure

### **3. Update Struct** (Priority: P0)

```go
type CachedExecutor struct {
    // Remove PostgreSQL dependency
    // db *sqlx.DB  // REMOVE

    // Add Data Storage client
    dsClient *dsclient.DataStorageClient  // ADD
    
    // Existing fields
    cache   cache.Cache
    metrics *metrics.ExecutorMetrics
    logger  *zap.Logger
}
```

---

## 🎯 **Success Criteria for GREEN Phase**

1. ✅ All 11 tests pass
2. ✅ No direct PostgreSQL queries in Context API executor
3. ✅ Circuit breaker opens after 3 failures
4. ✅ Exponential backoff retry (100ms, 200ms, 400ms)
5. ✅ Graceful degradation to cache when Data Storage unavailable
6. ✅ RFC 7807 error parsing functional

---

## 📈 **Confidence Assessment**

**Phase**: DO-RED ✅ COMPLETE  
**Confidence**: **100%**

**Justification**:
- ✅ Tests correctly failing on missing constructor (expected)
- ✅ Test coverage maps directly to business requirements
- ✅ Test scenarios validate configuration-driven behavior
- ✅ Clear implementation path defined
- ✅ No ambiguity in expected behavior

**Risks**: None (RED phase designed for failure)

---

## 🔗 **Related Documents**

- **ANALYSIS Phase**: [ANALYSIS-PHASE-CONTEXT-API-MIGRATION.md](./ANALYSIS-PHASE-CONTEXT-API-MIGRATION.md)
- **PLAN Phase**: [PLAN-PHASE-CONTEXT-API-MIGRATION.md](./PLAN-PHASE-CONTEXT-API-MIGRATION.md)
- **ADR-030**: Configuration Management Standard
- **Data Storage OpenAPI**: `docs/services/stateless/data-storage/openapi/v1.yaml`

---

**Document Status**: ✅ DO-RED Complete  
**Next Document**: DO-GREEN-PHASE-COMPLETE.md (after implementation)  
**Estimated GREEN Duration**: 3-4 hours  
**Confidence**: 100% (clear implementation path)

