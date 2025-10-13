# Day 10: Observability + Advanced Tests - Implementation Plan

**Date**: October 13, 2025
**Duration**: 8 hours
**Status**: 🚀 **IN PROGRESS**
**Previous**: Day 9 Complete (Context Propagation + BR Coverage)

---

## 🎯 Objectives

### Morning Session (4h): Prometheus Metrics
1. Create `pkg/datastorage/metrics/metrics.go` with 10+ Prometheus metrics
2. Instrument existing code with metrics
3. Add tests for metrics collection

### Afternoon Session (4h): Structured Logging + Advanced Integration Tests
1. Enhance logging with structured context
2. Add performance benchmarks
3. Create advanced integration tests
4. Document observability patterns

---

## 📊 Prometheus Metrics to Implement

### Business Requirement: BR-STORAGE-019 (Logging + Observability)

| Metric | Type | Labels | Purpose |
|---|---|---|---|
| `datastorage_write_total` | Counter | table, status | Total write operations |
| `datastorage_write_duration_seconds` | Histogram | table | Write operation duration |
| `datastorage_dualwrite_success_total` | Counter | - | Successful dual-writes |
| `datastorage_dualwrite_failure_total` | Counter | reason | Failed dual-writes |
| `datastorage_cache_hits_total` | Counter | - | Embedding cache hits |
| `datastorage_cache_misses_total` | Counter | - | Embedding cache misses |
| `datastorage_validation_failures_total` | Counter | field, reason | Validation failures |
| `datastorage_query_duration_seconds` | Histogram | operation | Query operation duration |
| `datastorage_embedding_generation_duration_seconds` | Histogram | - | Embedding generation time |
| `datastorage_fallback_mode_total` | Counter | - | PostgreSQL-only fallback count |

**Total**: 10 metrics (exceeding target)

---

## 📝 APDC Analysis Phase (30 min)

### Business Context

**Problem**: Need production-ready observability for debugging, performance monitoring, and alerting

**Key Requirements**:
- **BR-STORAGE-019**: Structured logging and metrics for all operations
- **Performance Monitoring**: Track query latency, write throughput, cache efficiency
- **Error Tracking**: Detailed failure reasons for debugging
- **Production Readiness**: Enable Prometheus/Grafana dashboards

### Technical Context

**Existing Observability**:
- ✅ Basic zap logging in place
- ✅ Error handling throughout codebase
- ❌ No Prometheus metrics yet
- ❌ No structured logging with context
- ❌ No performance benchmarks

**Integration Points**:
- Instrument `pkg/datastorage/client.go` (CRUD operations)
- Instrument `pkg/datastorage/dualwrite/coordinator.go` (dual-write logic)
- Instrument `pkg/datastorage/embedding/pipeline.go` (embedding generation)
- Instrument `pkg/datastorage/query/service.go` (query operations)
- Instrument `pkg/datastorage/validator.go` (validation failures)

### Complexity Assessment

**Complexity**: **MEDIUM**

**Rationale**:
- Metrics package is straightforward (simple counter/histogram definitions)
- Instrumentation requires touching multiple files but changes are localized
- No new business logic, only observability wrappers
- Testing is simple (increment metric, verify value)

---

## 📋 Implementation Plan

### Phase 1: Create Metrics Package (1h)

**File**: `pkg/datastorage/metrics/metrics.go`

1. Define 10 Prometheus metrics with appropriate types
2. Use `promauto` for automatic registration
3. Add documentation for each metric
4. Export metrics for use in instrumented code

### Phase 2: Instrument Dual-Write Coordinator (1h)

**File**: `pkg/datastorage/dualwrite/coordinator.go`

1. Add metrics imports
2. Wrap `Write()` method with duration tracking
3. Increment success/failure counters
4. Track fallback mode usage
5. Add structured logging with operation context

### Phase 3: Instrument Client Operations (1h)

**Files**: `pkg/datastorage/client.go`, `pkg/datastorage/embedding/pipeline.go`, `pkg/datastorage/query/service.go`

1. Track write operations per table
2. Track embedding generation duration
3. Track cache hits/misses
4. Track query operation duration

### Phase 4: Instrument Validation (30min)

**File**: `pkg/datastorage/validator.go`

1. Track validation failures by field and reason
2. Add structured logging for validation errors

### Phase 5: Testing + Benchmarks (1.5h)

**Files**: `test/unit/datastorage/metrics_test.go`, `test/unit/datastorage/benchmark_test.go`

1. Unit tests for metric collection
2. Benchmark tests for write operations
3. Benchmark tests for query operations
4. Performance regression detection

### Phase 6: Advanced Integration Tests (2h)

**File**: `test/integration/datastorage/observability_integration_test.go`

1. Metrics collection validation
2. High-throughput stress test
3. Cache efficiency test
4. Concurrent operations with metrics

### Phase 7: Documentation (1h)

1. Create `docs/services/stateless/data-storage/observability.md`
2. Document all metrics with Prometheus query examples
3. Create Grafana dashboard JSON
4. Document performance targets and SLOs

---

## ✅ Success Criteria

### Metrics Coverage
- ✅ 10+ Prometheus metrics defined
- ✅ All critical paths instrumented (write, read, query, embedding)
- ✅ Validation failures tracked by field/reason
- ✅ Dual-write success/failure tracked

### Testing
- ✅ Metrics unit tests passing
- ✅ Benchmarks establish performance baselines
- ✅ Integration tests validate metrics collection
- ✅ No performance regression (< 5% overhead)

### Documentation
- ✅ Observability guide created
- ✅ Prometheus queries documented
- ✅ Grafana dashboard template provided
- ✅ SLO targets documented

---

## 🎯 Performance Targets

| Operation | Target | Measured With |
|---|---|---|
| Write Operation | < 50ms p95 | `datastorage_write_duration_seconds` |
| Query Operation | < 100ms p95 | `datastorage_query_duration_seconds` |
| Embedding Generation | < 500ms p95 | `datastorage_embedding_generation_duration_seconds` |
| Cache Hit Rate | > 80% | `cache_hits / (cache_hits + cache_misses)` |
| Dual-Write Success Rate | > 99.9% | `dualwrite_success / (dualwrite_success + dualwrite_failure)` |

---

## 📚 Related Business Requirements

- **BR-STORAGE-019**: Logging and metrics for all operations
- **BR-STORAGE-002**: Dual-write transaction monitoring
- **BR-STORAGE-008**: Embedding generation performance
- **BR-STORAGE-012**: Query operation performance

---

## 🔄 Next Steps After Day 10

**Day 11**: Main App + HTTP Server Implementation
- HTTP server with 4 POST endpoints
- Health check endpoints
- Metrics endpoint (`/metrics`)
- Main application wiring

---

## 📝 Notes

- Metrics overhead should be < 5% (validated via benchmarks)
- Use `promauto` for automatic registration (no manual registration needed)
- Follow Prometheus naming conventions (suffix: `_total`, `_seconds`, `_bytes`)
- Histograms use appropriate buckets for expected latencies

---

**Status**: Ready to implement
**Confidence**: 95% (straightforward instrumentation, well-established patterns)
**Risk**: Low (additive changes only, no business logic modifications)

