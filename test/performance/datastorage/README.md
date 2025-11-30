# Data Storage Performance Tests

## Overview

Performance tests for the Data Storage Service, specifically validating the hybrid weighted scoring SQL query performance at realistic scale.

## Infrastructure

**Local Testing Approach**: These tests reuse the integration test Podman infrastructure (PostgreSQL with pgvector). No production platform is required.

**Why Local Testing**:
- No production platform available yet
- Sufficient for detecting performance regressions
- Can run in CI/CD
- Production-scale testing deferred to V1.1+

## Performance Targets

**Local Testing Targets** (2x slower than production due to local hardware):

| Metric | Target | Rationale |
|--------|--------|-----------|
| P50 Latency | <100ms | Acceptable for local testing |
| P95 Latency | <200ms | 2x slower than production target (100ms) |
| P99 Latency | <500ms | Conservative for local testing |
| Concurrent Queries | 10 QPS | Limited by local resources |

**Test Scales**:
- **1K workflows**: Typical production catalog size
- **5K workflows**: Large production catalog (future)
- **10K workflows**: Stress test (future)

## Running Performance Tests

### Prerequisites

1. **Start Integration Test Infrastructure**:
   ```bash
   # Start Podman PostgreSQL with pgvector
   cd test/integration/datastorage
   make start-infra  # Or equivalent command to start Podman containers
   ```

2. **Verify PostgreSQL is Running**:
   ```bash
   psql postgresql://slm_user:test_password@localhost:5433/action_history -c "SELECT 1"
   ```

3. **⚠️ IMPORTANT: Avoid Port Conflicts**:
   - If integration tests are running, they use port `5433`
   - To avoid data corruption, use a different PostgreSQL instance for performance tests
   - Set `PERF_TEST_PG_PORT` environment variable to specify a different port
   - Example:
     ```bash
     # Use a different PostgreSQL instance on port 5434
     PERF_TEST_PG_PORT=5434 ginkgo test/performance/datastorage/
     ```

### Run Performance Tests

```bash
# Run all performance tests (uses default port 5433)
ginkgo test/performance/datastorage/

# Run with custom PostgreSQL port (recommended if integration tests are running)
PERF_TEST_PG_PORT=5434 ginkgo test/performance/datastorage/

# Run with verbose output
ginkgo -v test/performance/datastorage/

# Run specific test
ginkgo --focus="should achieve P50 latency" test/performance/datastorage/
```

### Expected Output

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Latency Distribution Summary (1K workflows, 100 queries)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Latency Metrics:
  Min: 15ms
  Avg: 45ms
  P50: 42ms
  P95: 78ms
  P99: 120ms
  Max: 150ms
Performance Targets:
  P50_target: <100ms ✅
  P95_target: <200ms ✅
  P99_target: <500ms ✅
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

## Test Coverage

### Performance Tests (1K Workflows)

1. **P50 Latency Test**
   - Runs 100 searches
   - Measures P50 latency
   - Target: <100ms

2. **P95 Latency Test**
   - Runs 100 searches
   - Measures P95 latency
   - Target: <200ms

3. **P99 Latency Test**
   - Runs 100 searches
   - Measures P99 latency
   - Target: <500ms

4. **Concurrent Query Test**
   - Runs 10 concurrent searches
   - Measures QPS
   - Target: ≥10 QPS

5. **Latency Distribution Summary**
   - Comprehensive latency analysis
   - Reports Min, Avg, P50, P95, P99, Max
   - Validates all targets

## CI/CD Integration

### GitHub Actions Workflow

```yaml
name: Performance Tests

on:
  pull_request:
    paths:
      - 'pkg/datastorage/repository/**'
      - 'pkg/datastorage/models/**'
      - 'migrations/**'

jobs:
  performance-test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: pgvector/pgvector:pg16
        env:
          POSTGRES_PASSWORD: postgres
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5

    steps:
      - uses: actions/checkout@v3

      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Run performance tests
        run: |
          ginkgo test/performance/datastorage/

      - name: Check performance regression
        run: |
          # Parse test output and fail if P95 > 200ms
          # Implementation depends on test output format
```

## Troubleshooting

### Test Failures

**Issue**: Tests fail with "PostgreSQL not available"
```
Solution: Start integration test infrastructure first
```

**Issue**: Tests fail with "pgvector extension not available"
```
Solution: Ensure PostgreSQL image has pgvector extension
```

**Issue**: Performance targets not met
```
Solution:
1. Check local system load
2. Verify no other heavy processes running
3. Check PostgreSQL configuration
4. Review SQL query execution plan
```

### Performance Debugging

**Analyze SQL Query Performance**:
```sql
-- Run EXPLAIN ANALYZE on hybrid scoring query
EXPLAIN (ANALYZE, BUFFERS, VERBOSE, FORMAT JSON)
SELECT
    *,
    (1 - (embedding <=> $1)) AS base_similarity,
    -- ... (boost/penalty calculations) ...
FROM remediation_workflow_catalog
WHERE status = 'active'
  AND labels->>'signal_type' = 'OOMKilled'
  AND labels->>'severity' = 'critical'
ORDER BY final_score DESC
LIMIT 10;
```

**Check Index Usage**:
```sql
-- Verify GIN index on labels
SELECT indexname, indexdef
FROM pg_indexes
WHERE tablename = 'remediation_workflow_catalog'
  AND indexname LIKE '%labels%';

-- Verify HNSW index on embedding
SELECT indexname, indexdef
FROM pg_indexes
WHERE tablename = 'remediation_workflow_catalog'
  AND indexname LIKE '%embedding%';
```

## Future Enhancements

### V1.1+ (Production-Scale Testing)

1. **Large-Scale Tests**
   - 10K workflows
   - 100K workflows
   - 1M workflows

2. **High Concurrency Tests**
   - 100 QPS
   - 1000 QPS
   - Load testing

3. **Production Infrastructure**
   - Multi-node PostgreSQL cluster
   - Network latency simulation
   - Production-like hardware

4. **Advanced Metrics**
   - Query planning time analysis
   - Index scan efficiency
   - Memory usage profiling
   - Connection pool performance

## References

- **Implementation Plan**: `docs/services/stateless/data-storage/SEMANTIC_SEARCH_HYBRID_SCORING_IMPLEMENTATION.md`
- **Design Decision**: `docs/architecture/decisions/DD-WORKFLOW-004-hybrid-weighted-label-scoring.md`
- **Business Requirement**: BR-STORAGE-013 (Semantic search with hybrid weighted scoring)

