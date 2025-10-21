# DD-CONTEXT-001: Cache Stampede Prevention - Implementation Level

## Status
**✅ APPROVED** (2025-10-20)
**Last Reviewed**: 2025-10-20
**Confidence**: 90%
**User Approval**: Explicit approval received

---

## Context & Problem

### Problem Statement
During high-concurrency scenarios, when multiple concurrent requests hit the cache for the same expired/missing key, all requests cascade to the database simultaneously, causing a **cache stampede** (also known as "thundering herd").

**Production Reality**: ✅ **Very Common**
- Observed in every multi-tier cache service
- Happens during cache expiration at high traffic
- Can cause database overload and service degradation

### Current Architecture
```
User Request (×10 concurrent)
    ↓
CachedExecutor.ListIncidents()
    ↓
Cache.Get(key) → MISS (×10)
    ↓
Database.Query() → ⚠️ STAMPEDE! (×10 queries)
```

**Without Single-Flight**:
- 10 concurrent requests for same key
- All 10 hit database simultaneously
- Database sees 10× load spike
- 9 queries are wasted (same result)

**With Single-Flight**:
- 10 concurrent requests for same key
- Only 1 query executes, others wait
- Database sees normal load
- All 10 requests share 1 result

### Key Requirements
1. **Prevent database overload** during cache misses
2. **Minimize wasted queries** (don't query same data multiple times)
3. **Maintain consistency** (all concurrent requests get same result)
4. **Graceful degradation** (don't break existing functionality)
5. **Testability** (unit tests must validate single-flight behavior)

---

## Alternatives Considered

### Alternative 1: CachedExecutor Level (Database Layer)

**Approach**: Implement single-flight pattern in `CachedExecutor.ListIncidents()` to prevent multiple concurrent database queries for the same cache key.

**Implementation Location**: `pkg/contextapi/query/executor.go`

**How It Works**:
```go
// CachedExecutor orchestrates cache + database
func (e *CachedExecutor) ListIncidents(ctx, params) {
    cacheKey := generateKey(params)

    // Check cache first
    if cachedResult := e.cache.Get(cacheKey); cachedResult != nil {
        return cachedResult  // Cache hit, no DB query
    }

    // ✅ SINGLE-FLIGHT: Only 1 goroutine queries DB for this key
    // All others wait and share the result
    result, err := e.singleflight.Do(cacheKey, func() (interface{}, error) {
        return e.queryDatabase(ctx, params)
    })

    // Populate cache for next request
    e.cache.Set(cacheKey, result)

    return result, err
}
```

**Pros**:
- ✅ **Solves actual problem**: Prevents database stampede (not Redis stampede)
- ✅ **Architecturally correct**: Executor orchestrates, doesn't add logic to cache
- ✅ **High production value**: Database queries are slow (50-200ms), stampede is costly
- ✅ **Simpler implementation**: Single call site to protect (`queryDatabase`)
- ✅ **Easier to test**: Mock DB client, verify only 1 query despite N concurrent requests
- ✅ **Clear separation of concerns**: Cache = simple get/set, Executor = orchestration
- ✅ **Future-proof**: Works for all executor methods (ListIncidents, SemanticSearch, etc.)

**Cons**:
- ⚠️ **Requires refactoring**: Need to add singleflight to CachedExecutor
- ⚠️ **Test migration**: Move test from cache manager suite to executor suite
- ⚠️ **New dependency**: `golang.org/x/sync/singleflight` (or implement custom)

**Confidence**: **90%** ✅

**Effort**: 1-1.5 hours
- 15 min: Move test to `cached_executor_test.go`
- 30 min: Add singleflight to `CachedExecutor`
- 15 min: Import `golang.org/x/sync/singleflight`
- 30 min: Verify integration tests

---

### Alternative 2: Cache Manager Level (Redis Layer)

**Approach**: Implement single-flight pattern in `CacheManager.Get()` to prevent multiple concurrent Redis GET operations.

**Implementation Location**: `pkg/contextapi/cache/manager.go`

**How It Works**:
```go
// CacheManager wraps Redis + LRU
func (c *multiTierCache) Get(ctx, key) {
    // ⚠️ SINGLE-FLIGHT: Prevent duplicate Redis GETs
    result, err := c.singleflight.Do(key, func() (interface{}, error) {
        // Try L1 (Redis)
        if val := c.redis.Get(key); val != nil {
            return val, nil
        }
        // Try L2 (LRU)
        if val := c.memory[key]; val != nil {
            return val, nil
        }
        return nil, nil  // Cache miss
    })

    return result, err
}
```

**Pros**:
- ✅ **Prevents duplicate Redis GETs**: Minor optimization
- ✅ **No test migration**: Test stays in cache manager suite

**Cons**:
- ❌ **Doesn't solve real problem**: Redis GET is fast (~1ms), stampede not critical
- ❌ **Wrong abstraction**: Cache should be simple storage, not orchestration layer
- ❌ **Over-engineering**: Redis can easily handle 10 concurrent GETs
- ❌ **More complex**: Must coordinate Redis L1 + LRU L2 within single-flight
- ❌ **Harder to test**: Need to mock Redis timing, not just result
- ❌ **Database stampede still happens**: CachedExecutor still calls DB 10 times
- ❌ **Lower production value**: Optimizes fast operation, ignores slow operation

**Confidence**: **60%** ⚠️

**Effort**: 2-3 hours
- 30 min: Add singleflight to cache manager
- 60 min: Handle two-tier coordination (Redis + LRU)
- 30 min: Fix test logic
- 30 min: Test edge cases (LRU-only, Redis down)
- 30 min: Verify integration with executor

---

### Alternative 3: Skip Implementation

**Approach**: Defer cache stampede prevention to integration tests or production monitoring.

**Pros**:
- ✅ **Zero effort**: No implementation needed
- ✅ **Integration tests**: May already validate concurrent queries

**Cons**:
- ❌ **Production risk**: Database stampede can occur
- ❌ **Incomplete coverage**: Unit tests don't validate stampede prevention
- ❌ **Reactive approach**: Wait for problem instead of preventing

**Confidence**: **0%** (Rejected - user chose comprehensive approach)

---

## Decision

**APPROVED: Alternative 1** - **CachedExecutor Level (Database Layer)**

### Rationale

1. **Solves Real Production Problem**
   - Cache stampede occurs at the **database**, not the cache layer
   - Database queries are slow (50-200ms) and expensive
   - Redis GETs are fast (~1ms) and cheap
   - **Impact**: Prevents database overload, not Redis optimization

2. **Architecturally Correct**
   - `CachedExecutor` orchestrates cache + database interaction
   - `CacheManager` should remain simple (get/set operations)
   - Single-flight belongs in orchestration layer, not storage layer
   - **Principle**: Separation of concerns

3. **Faster Implementation**
   - 1-1.5 hours vs 2-3 hours (Option B)
   - Single call site to protect (`queryDatabase`)
   - Simpler testing (mock DB, verify single query)

4. **Higher Production Value**
   - Prevents actual database overload (critical)
   - Not just Redis GET optimization (minor)
   - Scales to all executor methods (ListIncidents, SemanticSearch, Aggregations)

5. **User Approval**
   - User explicitly approved Option A after confidence assessment
   - User requested comprehensive edge case coverage (all 8 scenarios)
   - User chose "Validate + Implement" approach (production-harden)

**Key Insight**: Cache stampede is a **database problem**, not a **cache problem**. The solution belongs in the orchestration layer (`CachedExecutor`), not the storage layer (`CacheManager`).

---

## Implementation

### Primary Implementation Files

1. **`pkg/contextapi/query/executor.go`** - Add single-flight to CachedExecutor
   ```go
   import "golang.org/x/sync/singleflight"

   type CachedExecutor struct {
       db             *sqlx.DB
       cache          cache.CacheManager
       singleflight   singleflight.Group  // NEW: Prevent stampede
       // ... existing fields
   }

   func (e *CachedExecutor) ListIncidents(ctx, params) {
       cacheKey := e.generateCacheKey(params)

       // Check cache first
       if cached := e.getFromCache(ctx, cacheKey); cached != nil {
           return cached
       }

       // Single-flight: deduplicate concurrent DB queries
       result, err, _ := e.singleflight.Do(cacheKey, func() (interface{}, error) {
           return e.queryDatabase(ctx, params)
       })

       if err != nil {
           return nil, 0, err
       }

       // Populate cache
       e.populateCache(ctx, cacheKey, result)

       return result.([]*models.IncidentEvent), result.(int), nil
   }
   ```

2. **`test/unit/contextapi/cached_executor_test.go`** - Move stampede test here
   ```go
   Context("Edge Case 1.1: Cache Stampede Prevention (P1)", func() {
       It("should prevent database stampede with single-flight", func() {
           // Mock DB to track query count
           dbCalls := atomic.Int64{}
           mockDB := &MockDB{
               OnQuery: func() {
                   dbCalls.Add(1)
                   time.Sleep(50 * time.Millisecond)  // Simulate slow query
               },
           }

           executor := NewCachedExecutor(mockDB, cache, logger)

           // 10 concurrent requests for same params
           var wg sync.WaitGroup
           for i := 0; i < 10; i++ {
               wg.Add(1)
               go func() {
                   defer wg.Done()
                   _, _, err := executor.ListIncidents(ctx, params)
                   Expect(err).ToNot(HaveOccurred())
               }()
           }

           wg.Wait()

           // ✅ Single-flight: Only 1 DB query despite 10 concurrent requests
           Expect(dbCalls.Load()).To(Equal(int64(1)))
       })
   })
   ```

3. **`go.mod`** - Add singleflight dependency
   ```
   require (
       golang.org/x/sync v0.5.0  // For singleflight.Group
       // ... existing dependencies
   )
   ```

### Data Flow (With Single-Flight)

```
Request 1 (t=0ms)  ┐
Request 2 (t=1ms)  ├─→ CachedExecutor.ListIncidents()
...                │       ↓
Request 10 (t=9ms) ┘   Cache.Get(key) → MISS
                           ↓
                   singleflight.Do("namespace=X&severity=Y")
                           ↓
                   [Request 1 executes DB query]  ← Only 1 query!
                   [Requests 2-10 wait and share result]
                           ↓
                   Database.Query() (50ms) ← Single query
                           ↓
                   [All 10 requests get shared result]
                           ↓
                   Cache.Set(key, result)
                           ↓
                   Return to all 10 requests
```

### Graceful Degradation

**Scenario: Single-flight fails or panics**
```go
// singleflight.Do handles panics internally
// If primary goroutine panics:
//   - Error is returned to all waiting goroutines
//   - Next request tries again (no permanent failure)
//   - Cache remains functional (doesn't break existing behavior)
```

**Scenario: Database timeout during single-flight**
```go
// Context timeout applies to all waiting goroutines
// If DB query times out:
//   - All waiting requests get context.DeadlineExceeded
//   - Next request (after timeout) can retry
//   - No goroutine is permanently blocked
```

---

## Consequences

### Positive

- ✅ **Prevents database overload** during high-concurrency cache misses
- ✅ **Reduces wasted queries** (N concurrent requests = 1 DB query)
- ✅ **Improves database performance** (lower query load)
- ✅ **Better resource utilization** (fewer DB connections needed)
- ✅ **Consistent results** (all concurrent requests get identical data)
- ✅ **Testable** (unit tests validate single-flight behavior)
- ✅ **Scalable** (works for all executor methods)

### Negative

- ⚠️ **Slightly increased latency for waiting requests**
  - **Mitigation**: First request already pays DB query cost (50-200ms)
  - Waiting requests get result faster than if they queried independently
  - Net effect: Improved p50/p95 latency (fewer slow queries)

- ⚠️ **New dependency**: `golang.org/x/sync/singleflight`
  - **Mitigation**: Well-maintained Google package, part of Go extended library
  - Widely used in production (part of standard patterns)
  - Minimal complexity, well-tested

- ⚠️ **Potential for cascading failures if primary request fails**
  - **Mitigation**: Each request has independent context timeout
  - Failed request doesn't block subsequent requests
  - Cache remains functional even if single-flight fails

### Neutral

- 🔄 **Test moved from cache to executor suite** (better architectural fit)
- 🔄 **Requires understanding of single-flight pattern** (standard Go pattern)

---

## Validation Results

### Confidence Assessment Progression

- **Initial assessment**: 95% (Option A solves real problem)
- **After detailed analysis**: 90% (accounts for refactoring risk)
- **After user approval**: 90% (confirmed correct approach)

### Key Validation Points

- ✅ **Architectural correctness**: Executor is orchestration layer (correct placement)
- ✅ **Production value**: Prevents database overload (high impact)
- ✅ **Implementation feasibility**: Well-known pattern, standard library support
- ✅ **Test coverage**: Unit tests validate single-flight, integration tests validate end-to-end
- ✅ **User approval**: Explicit approval after confidence assessment

---

## Related Decisions

- **Builds On**: BR-CONTEXT-005 (Multi-tier caching with graceful degradation)
- **Supports**: BR-CONTEXT-006 (Performance - Query latency targets)
- **Related**: Day 11 Edge Case Implementation (Comprehensive production edge cases)

---

## Review & Evolution

### When to Revisit

- If database stampede is observed despite single-flight (investigate cache key generation)
- If single-flight causes unexpected latency (measure p95, adjust if needed)
- If new executor methods are added (apply single-flight consistently)
- If `golang.org/x/sync/singleflight` is deprecated (migrate to alternative)

### Success Metrics

- **Database query count**: N concurrent requests = 1 DB query (target: 100% deduplication)
- **Cache hit rate**: Remains ≥70% (single-flight doesn't degrade caching)
- **Query latency (p95)**: ≤200ms (single-flight doesn't increase latency)
- **Database connection pool**: No exhaustion during high concurrency

---

## Implementation Timeline

**Phase 1: Unit Test (RED Phase)** - 15 minutes ✅
- Move test from `cache_manager_test.go` to `cached_executor_test.go`
- Update test to mock database instead of cache
- Verify test fails (RED)

**Phase 2: Implementation (GREEN Phase)** - 30 minutes
- Add `singleflight.Group` to `CachedExecutor` struct
- Wrap `queryDatabase` call in `singleflight.Do`
- Add `go.mod` dependency

**Phase 3: Verification (REFACTOR Phase)** - 30 minutes
- Run unit test, verify passes (GREEN)
- Run integration tests, verify no regressions
- Update documentation

**Total Effort**: 1-1.5 hours

---

**Decision Status**: ✅ **APPROVED AND DOCUMENTED**
**Next Step**: Implement Alternative 1 using Pure TDD (RED → GREEN → REFACTOR)
**Confidence**: 90% ✅

---

**Sign-off**: AI Assistant (Cursor)
**Date**: October 20, 2025
**User Approval**: Explicit approval received
**Design Decision**: DD-CONTEXT-001



