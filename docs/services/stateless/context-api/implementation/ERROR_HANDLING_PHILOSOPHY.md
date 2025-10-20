# Context API Error Handling Philosophy

## 📋 Purpose

This document defines the error handling strategy for Context API, ensuring:
- **Graceful Degradation**: Cache failures never block queries
- **Clear Error Messages**: Structured errors with context
- **Retry Logic**: Smart retry strategies for transient errors
- **Production Debugging**: Runbooks for common error scenarios

**Business Requirements**:
- BR-CONTEXT-003: Multi-tier caching with graceful degradation
- BR-CONTEXT-005: Error handling and recovery
- BR-CONTEXT-006: Health checks & metrics

---

## 🎯 Core Principles

### Principle 1: Never Block on Cache Failures
**Rule**: Cache failures MUST NOT prevent successful database queries

```go
// ✅ CORRECT: Cache error doesn't block query
incidents, total, err := cache.GetIncidents(ctx, key)
if err != nil {
    // Log cache error but continue to database
    logger.Warn("cache miss", "error", err, "key", key)
}

// Always fall back to database
return database.ListIncidents(ctx, params)
```

```go
// ❌ WRONG: Cache error blocks query
incidents, total, err := cache.GetIncidents(ctx, key)
if err != nil {
    return nil, 0, err // DON'T DO THIS
}
```

### Principle 2: Async Cache Operations Are Non-Blocking
**Rule**: Cache repopulation happens in background goroutines

```go
// ✅ CORRECT: Non-blocking cache repopulation
incidents, total, err := database.ListIncidents(ctx, params)
if err != nil {
    return nil, 0, err
}

// Async repopulation (fire and forget)
go func() {
    ctx := context.Background() // New context, not caller's
    _ = cache.SetIncidents(ctx, key, incidents, total, ttl)
}()

return incidents, total, nil
```

### Principle 3: Database Errors Are Always Propagated
**Rule**: Database query failures return errors to caller

```go
// ✅ CORRECT: Database error propagated
incidents, total, err := database.ListIncidents(ctx, params)
if err != nil {
    return nil, 0, fmt.Errorf("database query failed: %w", err)
}
```

### Principle 4: Context Cancellation Is Respected
**Rule**: All operations respect context cancellation/timeout

```go
// ✅ CORRECT: Context-aware operation
select {
case <-ctx.Done():
    return nil, 0, ctx.Err()
case result := <-resultChan:
    return result.incidents, result.total, result.err
}
```

---

## 📊 Error Categories

### Category 1: Cache Miss (Not an Error)
**Nature**: Expected behavior, not a failure condition
**Action**: Fall back to database
**Logging**: Debug level only
**Retry**: Never (not an error)

**Example**:
```go
incidents, total, err := cache.GetIncidents(ctx, key)
if err == cache.ErrCacheMiss {
    // Expected - query database
    logger.Debug("cache miss", "key", key)
    return queryDatabase(ctx, params)
}
```

**Production Impact**: None - normal operation

---

### Category 2: Transient Cache Errors
**Nature**: Temporary Redis/network issues
**Action**: Log warning, fall back to database
**Logging**: Warn level
**Retry**: No retry for cache (use L2 fallback)

**Examples**:
- Redis connection timeout
- Redis connection refused
- Network timeout

**Code Pattern**:
```go
incidents, total, err := cache.GetIncidents(ctx, key)
if err != nil && err != cache.ErrCacheMiss {
    // Transient cache error - log and continue
    logger.Warn("cache error, falling back to database",
        "error", err,
        "key", key,
        "cache_tier", "L1_redis")

    // Try L2 cache or database
    return queryDatabase(ctx, params)
}
```

**Production Impact**: Slight latency increase (database query), increased database load

**Mitigation**:
- L2 (LRU) cache absorbs Redis failures
- Database connection pooling handles increased load
- Monitor cache hit rate metrics

---

### Category 3: Transient Database Errors
**Nature**: Temporary database connectivity issues
**Action**: Retry with exponential backoff (max 3 attempts)
**Logging**: Warn on retry, Error on final failure
**Retry**: Yes (3 retries with backoff)

**Examples**:
- Connection refused (database restarting)
- Connection timeout
- Connection pool exhausted (temporary)
- Statement timeout

**Code Pattern**:
```go
var incidents []*models.IncidentEvent
var total int
var err error

backoff := []time.Duration{50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond}

for attempt := 0; attempt <= 3; attempt++ {
    incidents, total, err = database.ListIncidents(ctx, params)

    if err == nil {
        // Success
        return incidents, total, nil
    }

    if !isTransientError(err) {
        // Permanent error - don't retry
        return nil, 0, fmt.Errorf("database query failed: %w", err)
    }

    if attempt < 3 {
        logger.Warn("transient database error, retrying",
            "error", err,
            "attempt", attempt+1,
            "backoff_ms", backoff[attempt].Milliseconds())

        time.Sleep(backoff[attempt])
    }
}

// All retries exhausted
logger.Error("database query failed after retries",
    "error", err,
    "attempts", 4)
return nil, 0, fmt.Errorf("database query failed after 4 attempts: %w", err)
```

**Helper Function**:
```go
func isTransientError(err error) bool {
    if err == nil {
        return false
    }

    errStr := err.Error()

    // Transient error patterns
    transientPatterns := []string{
        "connection refused",
        "connection timeout",
        "connection reset",
        "connection pool exhausted",
        "statement timeout",
        "context deadline exceeded",
    }

    for _, pattern := range transientPatterns {
        if strings.Contains(strings.ToLower(errStr), pattern) {
            return true
        }
    }

    return false
}
```

**Production Impact**: Query latency increase (50-350ms for retries)

**Monitoring**:
- Track retry rate metric: `context_api_database_retries_total`
- Alert if retry rate > 5% of queries

---

### Category 4: Permanent Database Errors
**Nature**: Non-recoverable errors (bad SQL, constraint violations)
**Action**: Return error immediately, no retries
**Logging**: Error level
**Retry**: Never

**Examples**:
- SQL syntax error
- Table/column doesn't exist
- Data type mismatch
- Foreign key constraint violation

**Code Pattern**:
```go
incidents, total, err := database.ListIncidents(ctx, params)
if err != nil {
    if isPermanentError(err) {
        logger.Error("permanent database error",
            "error", err,
            "query_params", params)
        return nil, 0, fmt.Errorf("database query failed: %w", err)
    }

    // Transient error - retry logic
    return retryQuery(ctx, params)
}
```

**Helper Function**:
```go
func isPermanentError(err error) bool {
    if err == nil {
        return false
    }

    errStr := err.Error()

    // Permanent error patterns
    permanentPatterns := []string{
        "syntax error",
        "does not exist",
        "type mismatch",
        "constraint violation",
        "invalid input",
        "permission denied",
    }

    for _, pattern := range permanentPatterns {
        if strings.Contains(strings.ToLower(errStr), pattern) {
            return true
        }
    }

    return false
}
```

**Production Impact**: Query fails immediately, no retry overhead

**Debugging**:
- Check SQL query syntax
- Verify schema matches expectations
- Review database migrations

---

### Category 5: Context Cancellation/Timeout
**Nature**: Caller cancelled request or timeout exceeded
**Action**: Return context error immediately
**Logging**: Debug level (expected behavior)
**Retry**: Never

**Examples**:
- `context.Canceled` - caller cancelled request
- `context.DeadlineExceeded` - timeout exceeded

**Code Pattern**:
```go
// Check context before expensive operations
if ctx.Err() != nil {
    logger.Debug("context cancelled before query",
        "error", ctx.Err())
    return nil, 0, ctx.Err()
}

incidents, total, err := database.ListIncidents(ctx, params)
if err != nil {
    if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
        logger.Debug("context cancelled during query",
            "error", err)
        return nil, 0, err
    }

    // Other error types
    return handleDatabaseError(err)
}
```

**Production Impact**: Query cancelled, resources cleaned up

**Monitoring**:
- Track context cancellation rate: `context_api_context_cancelled_total`
- Alert if cancellation rate > 10% (indicates timeout issues)

---

### Category 6: Data Corruption/Deserialization Errors
**Nature**: Cache contains corrupted/invalid data
**Action**: Invalidate cache entry, fall back to database
**Logging**: Warn level
**Retry**: No retry, invalidate and requery

**Examples**:
- Invalid JSON in cache
- Missing required fields
- Type mismatches

**Code Pattern**:
```go
// Try cache
cachedData, err := cache.GetIncidents(ctx, key)
if err == nil {
    // Validate cached data
    if !isValidIncidentData(cachedData) {
        logger.Warn("corrupted cache data detected, invalidating",
            "key", key,
            "error", "validation failed")

        // Invalidate cache entry
        _ = cache.Delete(ctx, key)

        // Fall back to database
        return queryDatabase(ctx, params)
    }

    return cachedData, total, nil
}
```

**Helper Function**:
```go
func isValidIncidentData(incidents []*models.IncidentEvent) bool {
    if incidents == nil {
        return true // Empty result is valid
    }

    for _, incident := range incidents {
        // Check required fields
        if incident.ID == 0 {
            return false
        }
        if incident.Name == "" {
            return false
        }
        if incident.CreatedAt.IsZero() {
            return false
        }
    }

    return true
}
```

**Production Impact**: Cache invalidation, single database query

**Monitoring**:
- Track corruption rate: `context_api_cache_corruption_total`
- Alert if corruption rate > 0.1% (indicates systemic issue)

---

## 🔧 Structured Error Types

### Custom Error Definitions
```go
// Package errors provides structured errors for Context API
package errors

import "fmt"

// ErrorCategory represents error classification
type ErrorCategory string

const (
    CategoryCacheMiss         ErrorCategory = "cache_miss"
    CategoryCacheError        ErrorCategory = "cache_error"
    CategoryDatabaseTransient ErrorCategory = "database_transient"
    CategoryDatabasePermanent ErrorCategory = "database_permanent"
    CategoryContextCancelled  ErrorCategory = "context_cancelled"
    CategoryDataCorruption    ErrorCategory = "data_corruption"
)

// ContextAPIError wraps errors with additional context
type ContextAPIError struct {
    Category   ErrorCategory
    Message    string
    Underlying error
    Retryable  bool
    Context    map[string]interface{}
}

// Error implements error interface
func (e *ContextAPIError) Error() string {
    if e.Underlying != nil {
        return fmt.Sprintf("%s: %s: %v", e.Category, e.Message, e.Underlying)
    }
    return fmt.Sprintf("%s: %s", e.Category, e.Message)
}

// Unwrap allows errors.Is and errors.As to work
func (e *ContextAPIError) Unwrap() error {
    return e.Underlying
}

// NewCacheError creates a cache error
func NewCacheError(msg string, err error) *ContextAPIError {
    return &ContextAPIError{
        Category:   CategoryCacheError,
        Message:    msg,
        Underlying: err,
        Retryable:  false, // Cache errors don't retry, fall back to database
        Context:    make(map[string]interface{}),
    }
}

// NewDatabaseError creates a database error
func NewDatabaseError(msg string, err error, retryable bool) *ContextAPIError {
    category := CategoryDatabasePermanent
    if retryable {
        category = CategoryDatabaseTransient
    }

    return &ContextAPIError{
        Category:   category,
        Message:    msg,
        Underlying: err,
        Retryable:  retryable,
        Context:    make(map[string]interface{}),
    }
}
```

---

## 📚 Production Runbooks

### Runbook 1: High Cache Miss Rate
**Symptom**: Cache hit rate drops below 80%
**Metric**: `context_api_cache_hit_rate < 0.8`

**Diagnosis Steps**:
1. Check Redis connectivity:
   ```bash
   kubectl exec -it context-api-pod -- redis-cli -h redis-service ping
   ```

2. Check Redis memory usage:
   ```bash
   kubectl exec -it redis-pod -- redis-cli INFO memory
   ```

3. Check TTL configuration:
   ```bash
   kubectl get configmap context-api-config -o yaml | grep cache_ttl
   ```

**Resolution**:
- **If Redis is down**: Restart Redis, L2 cache will handle load
- **If Redis memory full**: Increase Redis memory or reduce TTL
- **If TTL too short**: Increase `CACHE_TTL` environment variable

**Impact**: Increased database load, slower query response times

---

### Runbook 2: Database Connection Pool Exhausted
**Symptom**: Errors containing "connection pool exhausted"
**Metric**: `context_api_database_connection_errors_total` increasing

**Diagnosis Steps**:
1. Check active database connections:
   ```sql
   SELECT count(*) FROM pg_stat_activity WHERE datname = 'action_history';
   ```

2. Check max connections configured:
   ```sql
   SHOW max_connections;
   ```

3. Check Context API pool settings:
   ```bash
   kubectl get deployment context-api -o yaml | grep -A5 DB_MAX_OPEN_CONNS
   ```

**Resolution**:
- **If connections leaked**: Restart Context API pods (ensures connections closed)
- **If legitimate load spike**: Increase `DB_MAX_OPEN_CONNS` (default 25)
- **If database under-provisioned**: Scale database (add replicas or increase resources)

**Impact**: Queries queue and timeout

---

### Runbook 3: High Query Latency (p95 > 500ms)
**Symptom**: Query latency exceeds 500ms at p95
**Metric**: `context_api_query_duration_seconds{quantile="0.95"} > 0.5`

**Diagnosis Steps**:
1. Check cache hit rate (should be >80%):
   ```promql
   rate(context_api_cache_hits_total[5m]) / rate(context_api_cache_requests_total[5m])
   ```

2. Check database query performance:
   ```sql
   SELECT query, mean_exec_time, calls
   FROM pg_stat_statements
   WHERE query LIKE '%remediation_audit%'
   ORDER BY mean_exec_time DESC
   LIMIT 10;
   ```

3. Check for missing indexes:
   ```sql
   SELECT schemaname, tablename, attname, n_distinct
   FROM pg_stats
   WHERE tablename = 'remediation_audit'
   AND correlation < 0.1;
   ```

**Resolution**:
- **If cache hit rate low**: See Runbook 1
- **If slow queries**: Add indexes on frequently filtered columns
- **If database CPU high**: Scale database vertically or add read replicas

**Impact**: User-facing query timeouts, increased error rate

---

### Runbook 4: Context Cancellation Rate High (>10%)
**Symptom**: High rate of context cancelled errors
**Metric**: `context_api_context_cancelled_total / context_api_requests_total > 0.1`

**Diagnosis Steps**:
1. Check caller timeout configuration:
   ```bash
   # Check client timeout settings
   kubectl get configmap holmesgpt-config -o yaml | grep context_api_timeout
   ```

2. Check query latency distribution:
   ```promql
   histogram_quantile(0.95, rate(context_api_query_duration_seconds_bucket[5m]))
   ```

3. Check for slow queries:
   ```sql
   SELECT * FROM pg_stat_activity WHERE state = 'active' AND now() - query_start > interval '30 seconds';
   ```

**Resolution**:
- **If queries legitimately slow**: Optimize queries or increase caller timeout
- **If caller timeout too aggressive**: Increase timeout to 5-10 seconds
- **If database overloaded**: Scale database or reduce query complexity

**Impact**: Partial request failures, degraded user experience

---

## 🎯 Error Handling Decision Matrix

| Error Type | Log Level | Retry? | Fallback | Propagate to Caller? |
|------------|-----------|--------|----------|---------------------|
| **Cache Miss** | Debug | No | Database | No (handled internally) |
| **Cache Error (Redis)** | Warn | No | L2 Cache → Database | No (graceful degradation) |
| **Cache Error (L2)** | Warn | No | Database | No (graceful degradation) |
| **Database Transient** | Warn | Yes (3x) | None | Yes (after retries fail) |
| **Database Permanent** | Error | No | None | Yes (immediately) |
| **Context Cancelled** | Debug | No | None | Yes (immediately) |
| **Context Timeout** | Debug | No | None | Yes (immediately) |
| **Data Corruption** | Warn | No | Invalidate + Database | No (handled internally) |
| **Validation Error** | Info | No | None | Yes (immediately) |
| **Connection Pool Full** | Warn | Yes (queue) | None | Yes (if timeout) |

---

## 📊 Error Metrics

### Required Prometheus Metrics
```go
// Cache metrics
context_api_cache_requests_total{tier="L1|L2",result="hit|miss|error"}
context_api_cache_hit_rate{tier="L1|L2"}
context_api_cache_corruption_total

// Database metrics
context_api_database_queries_total{result="success|error"}
context_api_database_retries_total
context_api_database_connection_errors_total

// Query metrics
context_api_query_duration_seconds{endpoint="/incidents|/search"}
context_api_query_errors_total{category="cache|database|validation"}

// Context metrics
context_api_context_cancelled_total
context_api_context_timeout_total
```

### Alert Rules
```yaml
# High cache miss rate
- alert: ContextAPICacheMissRateHigh
  expr: |
    rate(context_api_cache_requests_total{result="miss"}[5m]) /
    rate(context_api_cache_requests_total[5m]) > 0.2
  for: 5m
  severity: warning

# Database connection errors
- alert: ContextAPIConnectionPoolExhausted
  expr: rate(context_api_database_connection_errors_total[1m]) > 1
  for: 2m
  severity: critical

# High query latency
- alert: ContextAPIHighLatency
  expr: |
    histogram_quantile(0.95,
      rate(context_api_query_duration_seconds_bucket[5m])
    ) > 0.5
  for: 5m
  severity: warning
```

---

## ✅ Testing Error Scenarios

### Unit Test Coverage
Each error category MUST have dedicated unit tests:

```go
var _ = Describe("Error Handling", func() {
    Context("Category 1: Cache Miss", func() {
        It("should fall back to database on cache miss")
    })

    Context("Category 2: Transient Cache Errors", func() {
        It("should fall back to L2 cache on Redis error")
        It("should fall back to database on L2 error")
    })

    Context("Category 3: Transient Database Errors", func() {
        It("should retry on connection refused")
        It("should retry on timeout")
        It("should give up after 3 retries")
    })

    Context("Category 4: Permanent Database Errors", func() {
        It("should not retry on syntax error")
        It("should not retry on constraint violation")
    })

    Context("Category 5: Context Cancellation", func() {
        It("should respect context cancellation")
        It("should respect context timeout")
    })

    Context("Category 6: Data Corruption", func() {
        It("should invalidate corrupted cache entry")
        It("should fall back to database after invalidation")
    })
})
```

---

## 📖 References

- [03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc) - Testing framework
- [02-technical-implementation.mdc](mdc:.cursor/rules/02-technical-implementation.mdc) - Error handling patterns
- [IMPLEMENTATION_PLAN_V1.0.md](../IMPLEMENTATION_PLAN_V1.0.md) - Implementation timeline
- [BR Coverage Matrix](../testing/BR-COVERAGE-MATRIX.md) - Business requirement mapping

---

**Status**: ✅ ERROR HANDLING PHILOSOPHY DOCUMENTED

**Confidence**: 98%

**Rationale**: Comprehensive error handling strategy defined with:
- 6 error categories with clear handling logic
- Structured error types for better debugging
- Production runbooks for common scenarios
- Decision matrix for error propagation
- Prometheus metrics and alert rules
- Complete test coverage requirements

**Next Steps**: Implement error handling in CachedQueryExecutor (Day 4 continuation)




