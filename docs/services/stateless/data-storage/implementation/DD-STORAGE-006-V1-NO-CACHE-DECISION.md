# DD-STORAGE-006: V1.0 No-Cache Decision for Playbook Embeddings

**Date**: November 13, 2025
**Status**: ✅ **RECOMMENDED**
**Decision Maker**: Kubernaut Data Storage Team
**Affects**: BR-STORAGE-009, BR-STORAGE-012

---

## Context

V1.0 playbooks are managed via direct SQL (no REST API, no CRD controller). This creates a cache invalidation problem:
- **Problem**: If playbooks are updated via SQL, cached embeddings become stale
- **Impact**: 24-hour TTL could serve stale embeddings for a full day after SQL update
- **V1.1 Solution**: CRD controller will provide REST API for cache invalidation

**Question**: Should V1.0 implement playbook embedding caching despite no invalidation mechanism?

---

## Decision

**V1.0: NO CACHING** - Defer playbook embedding caching to V1.1

**Confidence**: 92%

---

## Analysis

### Scale Assessment

**Playbook Catalog Size** (from BR-PLAYBOOK-001):
- **Target**: 20+ playbooks in catalog
- **Versions**: ~2-3 versions per playbook
- **Total**: ~50-60 playbook records

**Query Volume** (from BR-PLAYBOOK-001):
- **Target**: 1,000+ AI queries per day
- **Peak**: ~1-2 queries/second
- **Per-incident**: 1 semantic search query (incident description → playbook matching)

**Embedding Generation Cost**:
- **Model**: sentence-transformers/all-MiniLM-L6-v2 (384 dimensions)
- **Latency**: ~50ms per playbook (local inference)
- **Total per query**: 50ms × 50 playbooks = 2,500ms = **2.5 seconds**

---

## Option 1: No Caching (V1.0) ⭐ **RECOMMENDED**

**Confidence**: 92%

### Architecture

```
Incident arrives → HolmesGPT API
    ↓
Data Storage Service: GET /api/v1/playbooks/search?query=pod+crash
    ↓
For EACH playbook in catalog (50 playbooks):
    1. Generate embedding (50ms) - NO CACHE
    2. Calculate cosine similarity with query embedding
    3. Filter by labels (environment, priority)
    ↓
Return top 10 playbooks sorted by confidence
```

**Total Latency**: 2.5 seconds per query (50 playbooks × 50ms)

### Pros

1. ✅ **No Stale Data** (95% confidence)
   - Every query gets fresh embeddings
   - SQL updates immediately reflected
   - No cache invalidation complexity

2. ✅ **Simpler V1.0** (90% confidence)
   - No Redis dependency for caching
   - No cache invalidation logic
   - No cache monitoring/debugging

3. ✅ **Acceptable Performance** (85% confidence)
   - 2.5s latency is acceptable for AI decision-making
   - AI analysis already takes 5-10s (LLM calls)
   - Semantic search is small fraction of total time

4. ✅ **Low Query Volume** (90% confidence)
   - 1,000 queries/day = 1-2 queries/second
   - Not a high-throughput scenario
   - No performance crisis without caching

### Cons

1. ⚠️ **Higher Latency** (85% concern)
   - 2.5s vs ~50ms with cache
   - 50× slower than cached approach
   - **Mitigation**: Still acceptable for AI workflow

2. ⚠️ **Higher CPU Usage** (70% concern)
   - Regenerate embeddings on every query
   - 50 embeddings × 1,000 queries/day = 50,000 embeddings/day
   - **Mitigation**: sentence-transformers is lightweight, CPU cost is low

3. ⚠️ **No Cache Hit Rate Metrics** (60% concern)
   - Can't measure cache efficiency
   - **Mitigation**: Not needed in V1.0, add in V1.1

### Performance Calculation

**Per-Query Cost**:
```
50 playbooks × 50ms/playbook = 2,500ms = 2.5 seconds
```

**Daily Cost**:
```
1,000 queries/day × 2.5s/query = 2,500 seconds/day = 42 minutes/day of CPU
```

**CPU Usage**:
```
42 minutes/day ÷ 1,440 minutes/day = 2.9% CPU utilization
```

**Conclusion**: CPU cost is negligible (< 3% utilization)

---

## Option 2: TTL-Only Caching (No Invalidation)

**Confidence**: 45% (NOT RECOMMENDED)

### Architecture

```
Incident arrives → HolmesGPT API
    ↓
Data Storage Service: GET /api/v1/playbooks/search?query=pod+crash
    ↓
For EACH playbook in catalog:
    1. Check Redis cache (key: embedding:playbook:{id}:{version})
    2. If HIT: Use cached embedding (1ms)
    3. If MISS: Generate embedding (50ms) + cache with 24h TTL
    4. Calculate cosine similarity
    ↓
Return top 10 playbooks
```

**Latency**:
- **Cold cache**: 2.5 seconds (same as no-cache)
- **Warm cache**: 50ms (50× faster)

### Pros

1. ✅ **50× Faster (Warm Cache)** (95% confidence)
   - 50ms vs 2.5s
   - Excellent user experience
   - Reduces AI decision time

2. ✅ **High Cache Hit Rate** (90% confidence)
   - Playbooks change infrequently (weekly/monthly)
   - Same playbooks queried repeatedly
   - Expected hit rate: 90-95%

### Cons

1. ❌ **Stale Embeddings** (95% concern) ⭐ **CRITICAL**
   - SQL update → stale cache for 24 hours
   - No way to invalidate cache in V1.0
   - Users see outdated playbook recommendations
   - **Impact**: UNACCEPTABLE for production

2. ❌ **No Control Over TTL** (90% concern)
   - 24h TTL is arbitrary
   - Too short: low hit rate
   - Too long: stale data
   - **Mitigation**: None in V1.0

3. ❌ **Operational Complexity** (70% concern)
   - Redis deployment required
   - Cache monitoring required
   - Debugging cache issues
   - **Mitigation**: Redis is already deployed for other features

### Stale Data Impact Analysis

**Scenario**: Playbook updated via SQL at 10:00 AM

```
10:00 AM - Playbook updated in PostgreSQL (new version, new content)
10:01 AM - Query arrives → Cache HIT → Returns OLD embedding (stale)
10:05 AM - Query arrives → Cache HIT → Returns OLD embedding (stale)
...
10:00 AM next day - Cache expires → Fresh embedding generated
```

**Impact**: 24 hours of stale recommendations

**Business Risk**:
- ❌ AI recommends outdated playbook
- ❌ Remediation uses old steps
- ❌ Potential incident escalation
- ❌ Loss of trust in AI recommendations

**Conclusion**: Stale data risk is UNACCEPTABLE

---

## Option 3: Short TTL Caching (5-minute TTL)

**Confidence**: 65%

### Architecture

Same as Option 2, but with 5-minute TTL instead of 24-hour TTL.

### Pros

1. ✅ **Reduced Stale Data Window** (85% confidence)
   - 5 minutes vs 24 hours
   - Acceptable staleness for most use cases

2. ✅ **Still Fast (Warm Cache)** (90% confidence)
   - 50ms vs 2.5s
   - Good user experience

### Cons

1. ⚠️ **Lower Cache Hit Rate** (80% concern)
   - 5-minute TTL = cache expires quickly
   - Expected hit rate: 40-50% (vs 90-95% with 24h TTL)
   - **Calculation**: 1,000 queries/day ÷ 1,440 minutes/day = 0.7 queries/minute
   - With 5-minute TTL, cache expires between queries

2. ⚠️ **Still No Invalidation Control** (90% concern)
   - 5 minutes is still arbitrary
   - SQL update → 5 minutes of stale data
   - **Mitigation**: 5 minutes is more acceptable than 24 hours

3. ⚠️ **Operational Complexity** (70% concern)
   - Same as Option 2 (Redis deployment, monitoring)

### Cache Hit Rate Calculation

**Assumptions**:
- 1,000 queries/day = 0.7 queries/minute
- 5-minute TTL
- 50 playbooks in catalog

**Hit Rate**:
```
If query arrives every 1.4 minutes (0.7 queries/min):
- Cache expires after 5 minutes
- ~3-4 queries hit cache before expiration
- Hit rate: 3.5 queries / 5 minutes = 70% hit rate
```

**Revised Estimate**: 70% hit rate (better than expected)

**Latency**:
```
70% cache hits: 0.7 × 50ms = 35ms
30% cache misses: 0.3 × 2,500ms = 750ms
Average: 35ms + 750ms = 785ms
```

**Conclusion**: 785ms average latency (vs 2,500ms no-cache, 50ms full-cache)

---

## Option 4: PostgreSQL-Only Caching (Materialized View)

**Confidence**: 75%

### Architecture

```sql
-- Materialized view stores pre-computed embeddings
CREATE MATERIALIZED VIEW playbook_embeddings_cache AS
SELECT
    playbook_id,
    version,
    embedding,
    updated_at
FROM playbook_catalog;

-- Refresh on playbook update (manual trigger)
REFRESH MATERIALIZED VIEW playbook_embeddings_cache;
```

**Query Flow**:
```
Incident arrives → Context API
    ↓
Data Storage Service: GET /api/v1/playbooks/search
    ↓
Query materialized view (fast):
    SELECT * FROM playbook_embeddings_cache
    WHERE embedding <=> query_embedding
    ORDER BY embedding <=> query_embedding
    LIMIT 10
```

### Pros

1. ✅ **Fast Queries** (90% confidence)
   - Materialized view is pre-computed
   - No Redis dependency
   - PostgreSQL-native solution

2. ✅ **Manual Refresh Control** (85% confidence)
   - `REFRESH MATERIALIZED VIEW` after SQL updates
   - Explicit invalidation (not automatic, but controllable)

3. ✅ **Simpler Architecture** (80% confidence)
   - No Redis
   - No cache invalidation logic
   - PostgreSQL handles everything

### Cons

1. ❌ **Manual Refresh Required** (90% concern)
   - SQL update → must manually run `REFRESH MATERIALIZED VIEW`
   - Easy to forget
   - **Mitigation**: Document refresh procedure

2. ⚠️ **Refresh Locks Table** (70% concern)
   - `REFRESH MATERIALIZED VIEW` locks the view
   - Queries blocked during refresh
   - **Mitigation**: Use `REFRESH MATERIALIZED VIEW CONCURRENTLY` (PostgreSQL 9.4+)

3. ⚠️ **Not Real-Time** (60% concern)
   - Embeddings only updated on manual refresh
   - **Mitigation**: Acceptable for V1.0 (playbooks change infrequently)

### Refresh Strategy

**Option A: Manual Refresh** (Recommended for V1.0)
```sql
-- After SQL update
UPDATE playbook_catalog SET content = '...' WHERE playbook_id = 'pod-oom-recovery';
REFRESH MATERIALIZED VIEW CONCURRENTLY playbook_embeddings_cache;
```

**Option B: Scheduled Refresh** (Alternative)
```sql
-- Cron job: refresh every 5 minutes
*/5 * * * * psql -c "REFRESH MATERIALIZED VIEW CONCURRENTLY playbook_embeddings_cache;"
```

**Option C: Trigger-Based Refresh** (V1.1 with CRD controller)
```sql
-- Trigger on playbook_catalog updates
CREATE TRIGGER refresh_embeddings_cache
AFTER INSERT OR UPDATE OR DELETE ON playbook_catalog
FOR EACH STATEMENT
EXECUTE FUNCTION refresh_embeddings_cache_function();
```

---

## Comparison Matrix

| Aspect | Option 1: No Cache | Option 2: 24h TTL | Option 3: 5min TTL | Option 4: Materialized View |
|--------|-------------------|-------------------|--------------------|-----------------------------|
| **Latency (Warm)** | 2,500ms | 50ms | 50ms | 100ms |
| **Latency (Cold)** | 2,500ms | 2,500ms | 2,500ms | 100ms |
| **Latency (Avg)** | 2,500ms | 50ms | 785ms | 100ms |
| **Stale Data Risk** | ✅ None | ❌ 24h | ⚠️ 5min | ⚠️ Manual refresh |
| **Cache Hit Rate** | N/A | 95% | 70% | 100% |
| **Complexity** | ✅ Low | ❌ High | ❌ High | ⚠️ Medium |
| **Redis Required** | ✅ No | ❌ Yes | ❌ Yes | ✅ No |
| **Invalidation Control** | ✅ N/A | ❌ None | ❌ None | ✅ Manual |
| **CPU Usage** | ⚠️ 3% | ✅ <1% | ⚠️ 1.5% | ✅ <1% |
| **V1.0 Readiness** | ✅ Ready | ❌ Not Ready | ⚠️ Acceptable | ✅ Ready |
| **Confidence** | **92%** ⭐ | 45% | 65% | 75% |

---

## Recommended Decision

### **V1.0: Option 1 (No Caching)** ⭐

**Confidence**: 92%

**Rationale**:
1. ✅ **No stale data risk** (critical for production)
2. ✅ **Simplest architecture** (no Redis, no cache logic)
3. ✅ **Acceptable performance** (2.5s is fine for AI workflow)
4. ✅ **Low query volume** (1,000 queries/day = 1-2 queries/second)
5. ✅ **Low CPU cost** (< 3% utilization)

**Trade-off Accepted**:
- ⚠️ 2.5s latency (vs 50ms with cache)
- ✅ **Worth it**: Eliminates stale data risk and complexity

---

### **V1.1: Option 4 (Materialized View) or Redis with CRD Invalidation**

**Confidence**: 85%

**Rationale**:
1. ✅ **CRD controller provides invalidation** (REST API or trigger)
2. ✅ **Faster queries** (50-100ms vs 2.5s)
3. ✅ **Controlled staleness** (explicit refresh)

**V1.1 Options**:

**Option A: Materialized View** (Simpler)
- ✅ No Redis dependency
- ✅ PostgreSQL-native
- ⚠️ Manual refresh (CRD controller calls `REFRESH MATERIALIZED VIEW`)

**Option B: Redis + CRD Invalidation** (More Flexible)
- ✅ Automatic invalidation (CRD controller calls Data Storage invalidation endpoint)
- ✅ Fine-grained control (per-playbook invalidation)
- ❌ Redis dependency

**Recommendation**: Start with Option A (Materialized View) in V1.1, evaluate Option B if performance issues arise.

---

## Implementation Plan

### V1.0 (Current)

**No caching implementation**:
1. ✅ Generate embeddings on every query
2. ✅ No Redis dependency
3. ✅ No cache invalidation logic
4. ✅ Document 2.5s latency as expected behavior

**Code Changes**: None (remove caching from implementation plan)

### V1.1 (Future)

**Materialized View implementation**:
1. Create materialized view for playbook embeddings
2. CRD controller calls `REFRESH MATERIALIZED VIEW` on playbook updates
3. Update Data Storage Service to query materialized view
4. Add metrics for refresh duration

**Estimated Effort**: 4-6 hours

---

## Risks and Mitigations

### Risk 1: 2.5s Latency Unacceptable

**Likelihood**: 20%
**Impact**: High

**Mitigation**:
- Monitor p95 latency in production
- If > 5s, implement Option 4 (Materialized View) immediately
- Fallback: Option 3 (5-minute TTL) as emergency fix

### Risk 2: Query Volume Higher Than Expected

**Likelihood**: 30%
**Impact**: Medium

**Mitigation**:
- Monitor query volume in production
- If > 5,000 queries/day, implement caching in V1.0.1
- Use Option 4 (Materialized View) for quick fix

### Risk 3: CPU Usage Higher Than Expected

**Likelihood**: 10%
**Impact**: Low

**Mitigation**:
- Monitor CPU usage in production
- If > 10%, implement caching immediately
- Use Option 4 (Materialized View) for quick fix

---

## Success Metrics

### V1.0 (No Cache)

| Metric | Target | Measurement |
|--------|--------|-------------|
| **p95 Latency** | < 5s | Prometheus histogram |
| **CPU Usage** | < 10% | Prometheus gauge |
| **Query Volume** | < 5,000/day | Prometheus counter |
| **Error Rate** | < 1% | Prometheus counter |

### V1.1 (With Cache)

| Metric | Target | Measurement |
|--------|--------|-------------|
| **p95 Latency** | < 200ms | Prometheus histogram |
| **Cache Hit Rate** | > 90% | Prometheus gauge |
| **Refresh Duration** | < 1s | Prometheus histogram |
| **Stale Data Incidents** | 0 | Manual tracking |

---

## Confidence Breakdown

| Factor | Confidence | Reasoning |
|--------|-----------|-----------|
| **Performance Acceptable** | 85% | 2.5s is fine for AI workflow (5-10s total) |
| **No Stale Data Risk** | 95% | No cache = no staleness |
| **Simplicity** | 90% | No Redis, no cache logic |
| **Low Query Volume** | 90% | 1,000 queries/day is low |
| **Low CPU Cost** | 95% | < 3% utilization is negligible |
| **V1.1 Migration Path** | 85% | Materialized view is straightforward |
| **Overall** | **92%** | Strong recommendation for V1.0 |

---

## Conclusion

**V1.0: No caching** is the right decision with **92% confidence**.

**Key Reasons**:
1. ✅ Eliminates stale data risk (critical)
2. ✅ Simplifies V1.0 architecture
3. ✅ Acceptable performance for low query volume
4. ✅ Clear migration path to V1.1 caching

**V1.1: Implement caching** with materialized view or CRD-triggered invalidation.

**Next Steps**:
1. Update BR-STORAGE-009 to defer caching to V1.1
2. Update implementation plan to remove caching
3. Document 2.5s latency as expected V1.0 behavior
4. Monitor production metrics to validate decision

---

**Document Version**: 1.0
**Last Updated**: November 13, 2025
**Status**: ✅ **APPROVED**

