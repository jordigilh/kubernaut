# Data Storage Performance Smoke Tests

## Overview

**Smoke tests** for Data Storage Service operational resilience. These are **NOT** true load tests - they detect obvious performance regressions without requiring production-scale infrastructure.

## Purpose: Smoke Testing, Not Load Testing

**What These Tests Do** ✅:
- Detect **order-of-magnitude regressions** (e.g., query takes 10s instead of 100ms)
- Validate **operational resilience** (cold start, burst handling)
- Run on **local infrastructure** (Podman PostgreSQL)

**What These Tests DON'T Do** ❌:
- Validate production performance under realistic load
- Simulate multi-node clusters or network latency
- Test with production-scale data (10K+ workflows, 1M+ events)
- Measure precise latency percentiles under load

**Rationale**: True load testing requires production-like infrastructure that isn't available yet. These smoke tests catch obvious problems during development.

## Infrastructure

**Local Testing Approach**: Reuses integration test Podman infrastructure (PostgreSQL with pgvector). No production platform required.

## Smoke Test Targets (Order-of-Magnitude Checks)

**Local Smoke Test Targets**:

| Metric | Target | Purpose |
|--------|--------|---------|
| Cold Start First Request | <5s | Detect startup issues |
| Burst Writes (150 events) | No crashes | Detect buffer overflow |
| P95 Latency | <1s | Order-of-magnitude check |

**Note**: These targets are **NOT** production SLAs. They're "sanity checks" to catch obvious regressions.

## Running Smoke Tests

### Prerequisites

1. **Start Integration Test Infrastructure**:
   ```bash
   # Start Podman PostgreSQL with pgvector + Data Storage service
   cd test/integration/datastorage
   # Start infrastructure (assumes you have a startup script)
   # Data Storage should be running on http://localhost:18090
   ```

2. **Verify Data Storage is Running**:
   ```bash
   curl http://localhost:18090/health
   ```

### Run Smoke Tests

```bash
# Run smoke tests (default Data Storage URL: http://localhost:18090)
ginkgo test/performance/datastorage/

# Run with custom Data Storage URL
DATASTORAGE_URL=http://localhost:8080 ginkgo test/performance/datastorage/

# Run with verbose output
ginkgo -v test/performance/datastorage/

# Run specific smoke test
ginkgo --focus="Cold Start" test/performance/datastorage/
```

### Expected Output

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
GAP 5.3: Testing cold start performance (service restart)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Service became healthy in 850ms
First request completed in 1.2s
Second request completed in 45ms

✅ Cold start performance validated:
   Startup:       850ms (target: <5s)
   First request: 1.2s (target: <5s)
   Second request: 45ms (target: <1s)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

## Smoke Test Coverage

### Operational Resilience Smoke Tests

1. **Cold Start Performance** (BR-STORAGE-031)
   - **Purpose**: Detect slow service restarts
   - **Test**: Service health + first request latency
   - **Target**: First request <5s (order-of-magnitude check)
   - **Why**: Rolling updates must not cause extended downtime

2. **Write Burst Handling** (BR-STORAGE-028)
   - **Purpose**: Detect buffer overflow on incident storms
   - **Test**: 150 audit events in 1 second
   - **Target**: No crashes, all events accepted
   - **Why**: Real incidents create write storms (50 pods × 3 events)

## CI/CD Integration

### GitHub Actions Workflow (Optional)

**Note**: These smoke tests can run in CI/CD, but they're primarily for local development regression detection.

```yaml
name: Performance Smoke Tests

on:
  pull_request:
    paths:
      - 'pkg/datastorage/**'
      - 'test/performance/datastorage/**'

jobs:
  smoke-test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: pgvector/pgvector:pg16
        env:
          POSTGRES_PASSWORD: test_password
          POSTGRES_USER: slm_user
          POSTGRES_DB: action_history
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

      - name: Start Data Storage service
        run: |
          # Start Data Storage in background
          make run-datastorage &
          sleep 5  # Wait for startup

      - name: Run smoke tests
        run: |
          ginkgo test/performance/datastorage/

      - name: Check for obvious regressions
        run: |
          # Fail if tests detect order-of-magnitude regressions
          # (Tests will fail on their own if targets not met)
```

## Troubleshooting

### Smoke Test Failures

**Issue**: "Data Storage not available"
```
Solution: Ensure Data Storage service is running on http://localhost:18090
Check: curl http://localhost:18090/health
```

**Issue**: "Cold start test fails - first request >5s"
```
Interpretation: Likely a real problem - investigate service startup
NOT a flaky test - cold start should be fast even locally
```

**Issue**: "Burst write test fails - events dropped"
```
Interpretation: Buffer overflow issue - investigate BufferedAuditStore
NOT a resource constraint - 150 events should succeed locally
```

**Issue**: Tests pass but seem slow
```
Interpretation: This is expected for local testing
Smoke tests have generous targets to avoid false positives
Only fail if order-of-magnitude regression (e.g., 10x slower)
```

### When to Ignore Smoke Test Failures

**Local Environment Issues** (can ignore):
- System under heavy load
- Disk I/O contention
- Network issues (if running remote DB)

**Real Problems** (DO NOT ignore):
- Service crashes during test
- Connection pool exhaustion
- Buffer overflow errors
- Order-of-magnitude latency increases (e.g., 100ms → 10s)

## When to Do REAL Load Testing (V1.1+)

**Current Status**: These smoke tests are sufficient for **development regression detection**.

**True Load Testing Required When**:
- Deploying to production
- Significant architectural changes
- Performance-critical features
- Production incidents suggest performance issues

### V1.1+ Production Load Testing Requirements

1. **Infrastructure Prerequisites**
   - Multi-node PostgreSQL cluster (3+ nodes)
   - Distributed Data Storage instances (3+ replicas)
   - Production-like network topology
   - Realistic data scale (100K+ workflows, 10M+ audit events)

2. **Test Scenarios**
   - Sustained high concurrency (100+ QPS)
   - Multi-tenant workload simulation
   - Realistic AI query patterns from HolmesGPT
   - Multiple simultaneous incident storms
   - Chaos engineering (node failures, network partitions)

3. **Metrics to Measure**
   - True P50/P95/P99 latencies under load
   - Query planning time at scale
   - Index scan efficiency with large datasets
   - Memory usage patterns
   - Connection pool behavior under contention

**Defer Until**: Production infrastructure available and performance becomes critical path

## References

- **Smoke Test Design**: This README
- **Business Requirements**: BR-STORAGE-028 (burst handling), BR-STORAGE-031 (cold start)

