# DataStorage Test Refactoring - Integration vs E2E Separation

**Date**: January 10, 2026
**Authority**: User request for proper test tier separation
**Goal**: Move HTTP API tests to E2E, keep integration tests focused on business logic

## Problem Statement

Current `test/integration/datastorage/` contains mixed test types:
- ✅ **True Integration**: Repository + PostgreSQL (direct business logic)
- ❌ **Misplaced E2E**: HTTP API + OpenAPI validation + Repository + PostgreSQL

This causes:
1. Unnecessary HTTP server infrastructure in integration tests
2. Per-process server complexity to avoid connection pool exhaustion
3. Confusion about test boundaries

## Solution

**Integration Tests**: Business logic + external services (PostgreSQL, Redis) - **NO HTTP**
**E2E Tests**: Complete deployment with HTTP API validation

## Migration Plan

### Phase 1: Move HTTP API Tests to E2E ✅ IN PROGRESS

| Source (Integration) | Destination (E2E) | Status | Notes |
|---------------------|-------------------|--------|-------|
| `audit_events_write_api_test.go` | `12_audit_write_api_test.go` | 🔄 TODO | OpenAPI write validation |
| `audit_events_query_api_test.go` | `13_audit_query_api_test.go` | 🔄 TODO | OpenAPI query validation |
| `audit_events_batch_write_api_test.go` | `14_audit_batch_write_api_test.go` | 🔄 TODO | OpenAPI batch validation |
| `legal_hold_integration_test.go` | Merge into `05_soc2_compliance_test.go` | 🔄 TODO | Already has legal hold tests |
| `http_api_test.go` | `15_http_api_test.go` | 🔄 TODO | HTTP endpoint validation |
| `aggregation_api_adr033_test.go` | `16_aggregation_api_test.go` | 🔄 TODO | HTTP aggregation endpoints |
| `audit_export_integration_test.go` | Merge into `05_soc2_compliance_test.go` | 🔄 TODO | SOC2 exports |
| `metrics_integration_test.go` | `17_metrics_api_test.go` | 🔄 TODO | HTTP metrics endpoint |
| `workflow_duplicate_api_test.go` | Merge into `04_workflow_search_test.go` | 🔄 TODO | Already has workflow tests |
| `graceful_shutdown_test.go` | `18_graceful_shutdown_test.go` | 🔄 TODO | Server lifecycle |

### Phase 2: Refactor audit_client_timing_integration_test.go

**Current**: Tests `audit.BufferedStore` via HTTP (hybrid approach)

**Refactor to**:
1. **Integration**: Test `audit.BufferedStore` → Repository → PostgreSQL (no HTTP)
   - File: `audit_client_timing_integration_test.go` (keep, refactor)
   - Focus: Buffer flush timing, batch behavior, retry logic

2. **E2E**: Test complete audit path including HTTP
   - File: `19_audit_client_happy_path_test.go` (new)
   - Focus: End-to-end audit trace delivery

### Phase 3: Remove HTTP Server from Integration Suite

**File**: `test/integration/datastorage/suite_test.go`

**Remove**:
```go
// Phase 1: Create in-process HTTP server (REMOVE)
dsServer, err = server.NewServer(...)
testServer = httptest.NewServer(dsServer.Handler())
```

**Keep**:
```go
// Phase 1: PostgreSQL + Redis containers (KEEP)
startPostgreSQL()
startRedis()
connectPostgreSQL()
connectRedis()
```

### Phase 4: Update Integration Test Helpers

**File**: `test/integration/datastorage/openapi_helpers.go`

**Decision**: DELETE this file (HTTP helpers not needed in integration tests)

**Alternative**: Move to `test/e2e/datastorage/helpers.go` if E2E needs additional helpers

### Phase 5: Move Helper Tests

| Source | Destination | Status |
|--------|-------------|--------|
| `audit_validation_helper_test.go` | `test/unit/testutil/` | 🔄 TODO |

## Test Inventory After Refactoring

### ✅ Integration Tests (Business Logic + External Services)

**Repository Tests** (Direct DB):
- `audit_events_repository_integration_test.go` - Audit repository + PostgreSQL
- `workflow_repository_integration_test.go` - Workflow repository + PostgreSQL
- `repository_adr033_integration_test.go` - ADR-033 repository + PostgreSQL
- `repository_test.go` - Base repository tests

**Schema & DB Tests**:
- `audit_events_schema_test.go` - Direct schema validation

**Business Logic Integration**:
- `audit_client_timing_integration_test.go` - BufferedStore timing (REFACTORED)
- `workflow_label_scoring_integration_test.go` - Label scoring + DB
- `dlq_test.go` - DLQ + Redis + PostgreSQL
- `dlq_near_capacity_warning_test.go` - DLQ warnings

**Infrastructure**:
- `config_integration_test.go` - Config loading + validation
- `cold_start_performance_test.go` - Startup performance
- `workflow_bulk_import_performance_test.go` - Bulk import performance

**Total**: ~12 integration test files (down from 24)

### ✅ E2E Tests (HTTP API + Full Deployment)

**Existing**:
1. `01_happy_path_test.go` - Basic audit write/read
2. `02_dlq_fallback_test.go` - DLQ failure handling
3. `03_query_api_timeline_test.go` - Timeline queries
4. `04_workflow_search_test.go` - Workflow search
5. `05_soc2_compliance_test.go` - Legal hold + exports
6. `06_workflow_search_audit_test.go` - Workflow audit traces
7. `07_workflow_version_management_test.go` - Workflow versions
8. `08_workflow_search_edge_cases_test.go` - Edge cases
9. `09_event_type_jsonb_comprehensive_test.go` - JSONB queries
10. `10_malformed_event_rejection_test.go` - Validation
11. `11_connection_pool_exhaustion_test.go` - Load testing

**New/Merged**:
12. `12_audit_write_api_test.go` - OpenAPI write validation (moved)
13. `13_audit_query_api_test.go` - OpenAPI query validation (moved)
14. `14_audit_batch_write_api_test.go` - OpenAPI batch (moved)
15. `15_http_api_test.go` - HTTP endpoints (moved)
16. `16_aggregation_api_test.go` - Aggregation endpoints (moved)
17. `17_metrics_api_test.go` - Metrics endpoint (moved)
18. `18_graceful_shutdown_test.go` - Server lifecycle (moved)
19. `19_audit_client_happy_path_test.go` - Audit client E2E (new)

**Total**: ~19 E2E test files

## Benefits

1. **✅ No HTTP server in integration tests** → Simpler setup, faster execution
2. **✅ Clear test boundaries** → Integration = business logic, E2E = HTTP API
3. **✅ Eliminates per-process server complexity** → No connection pool issues
4. **✅ Better test performance** → Integration tests ~2x faster without HTTP overhead
5. **✅ Proper ogen validation placement** → OpenAPI validation in E2E where it belongs

## Verification

After refactoring:

```bash
# Integration tests should NOT have HTTP clients
grep -r "httptest.NewServer\|CreateAuditEvent.*HTTP" test/integration/datastorage/
# Should return: (nothing)

# E2E tests should use external service
grep -r "os.Getenv.*DATASTORAGE_URL" test/e2e/datastorage/suite_test.go
# Should return: external service URL usage

# All integration tests pass without HTTP
make test-integration-datastorage
# Expected: All pass, no HTTP server creation logs
```

## Timeline

- **Phase 1**: Move HTTP API tests → 1-2 hours (10 files)
- **Phase 2**: Refactor audit_client_timing → 30 mins
- **Phase 3**: Remove HTTP from suite → 15 mins
- **Phase 4**: Update/delete helpers → 15 mins
- **Phase 5**: Move helper tests → 15 mins

**Total**: ~3 hours of focused refactoring

## Decision Log

- ✅ **User approved** moving HTTP API tests to E2E
- ✅ **User approved** refactoring audit_client_timing_integration_test.go to not use HTTP
- ✅ **User approved** creating happy path E2E version of audit client test
- ✅ **Proceed immediately** per user request

## Status

**Current**: Planning complete, starting Phase 1 execution
**Next**: Move first batch of HTTP API tests to E2E
