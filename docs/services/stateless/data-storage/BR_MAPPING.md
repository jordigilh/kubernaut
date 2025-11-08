# Data Storage Service - BR Mapping

**Version**: v1.0
**Date**: November 8, 2025
**Purpose**: Map high-level Business Requirements to sub-BRs and test files

---

## 📊 **Summary**

- **Total Umbrella BRs**: 9
- **Total Sub-BRs**: 30
- **Test File Coverage**: 18 test files

---

## 🗂️ **High-Level BR → Sub-BR Mapping**

### **1. Audit Persistence**

**Umbrella BR**: Persist audit records to PostgreSQL with complete metadata

**Sub-BRs**:
- **BR-STORAGE-001**: Persist notification audit records
- **BR-STORAGE-020**: Audit write API endpoints

**Test Coverage**:
- Integration: `test/integration/datastorage/repository_test.go`
- Integration: `test/integration/datastorage/http_api_test.go`
- Integration: `test/integration/datastorage/suite_test.go`

**Implementation**:
- `pkg/datastorage/repository/notification_audit_repository.go`
- `pkg/datastorage/server/audit_handlers.go`

---

### **2. Dual-Write Coordination**

**Umbrella BR**: Atomic writes to PostgreSQL + Redis Vector DB

**Sub-BRs**:
- **BR-STORAGE-002**: Typed error handling for dual-write operations
- **BR-STORAGE-014**: Atomic dual-write operations with rollback
- **BR-STORAGE-015**: Graceful degradation (PostgreSQL-only fallback)
- **BR-STORAGE-016**: Context propagation for cancellation and timeouts

**Test Coverage**:
- Unit: `test/unit/datastorage/dualwrite_test.go`
- Unit: `test/unit/datastorage/dualwrite_context_test.go`
- Unit: `test/unit/datastorage/errors_dualwrite_test.go`

**Implementation**:
- `pkg/datastorage/dualwrite/coordinator.go`
- `pkg/datastorage/dualwrite/errors.go`

---

### **3. Query API**

**Umbrella BR**: REST API for querying audit records with filtering and pagination

**Sub-BRs**:
- **BR-STORAGE-005**: Query API with filtering (namespace, status, phase)
- **BR-STORAGE-006**: Pagination support (limit, offset)
- **BR-STORAGE-021**: REST API read endpoints
- **BR-STORAGE-022**: Query filtering via URL parameters
- **BR-STORAGE-023**: Pagination validation (limit: 1-1000)

**Test Coverage**:
- Unit: `test/unit/datastorage/query_test.go`
- Unit: `test/unit/datastorage/query_builder_test.go`
- Unit: `test/unit/datastorage/handlers_test.go`

**Implementation**:
- `pkg/datastorage/query/service.go`
- `pkg/datastorage/query/builder.go`
- `pkg/datastorage/server/handler.go`

---

### **4. Input Validation & Sanitization**

**Umbrella BR**: Validate and sanitize all incoming data

**Sub-BRs**:
- **BR-STORAGE-010**: Input validation (required fields, lengths, types)
- **BR-STORAGE-011**: Input sanitization (XSS prevention)
- **BR-STORAGE-025**: SQL injection prevention
- **BR-STORAGE-026**: Unicode support

**Test Coverage**:
- Unit: `test/unit/datastorage/validation_test.go`
- Unit: `test/unit/datastorage/sanitization_test.go`
- Unit: `test/unit/datastorage/query_builder_test.go`
- Unit: `test/unit/datastorage/handlers_test.go`
- Integration: `test/integration/datastorage/repository_test.go`

**Implementation**:
- `pkg/datastorage/validation/validator.go`
- `pkg/datastorage/validation/rules.go`
- `pkg/datastorage/query/builder.go`

---

### **5. Embedding & Vector Operations**

**Umbrella BR**: Generate embeddings and support semantic search

**Sub-BRs**:
- **BR-STORAGE-012**: Embedding generation (384-dimensional vectors)
- **BR-STORAGE-013**: Query performance metrics for vector operations

**Test Coverage**:
- Unit: `test/unit/datastorage/embedding_test.go`
- Unit: `test/unit/datastorage/query_test.go` (semantic search reference)
- Unit: `test/unit/datastorage/metrics_test.go`

**Implementation**:
- `pkg/datastorage/embedding/pipeline.go`
- `pkg/datastorage/embedding/redis_cache.go`

---

### **6. Observability & Metrics**

**Umbrella BR**: Comprehensive Prometheus metrics for all operations

**Sub-BRs**:
- **BR-STORAGE-007**: Query performance tracking
- **BR-STORAGE-009**: Cache hit/miss tracking
- **BR-STORAGE-019**: Prometheus metrics (write total, duration, dual-write, cache, validation)

**Test Coverage**:
- Unit: `test/unit/datastorage/metrics_test.go`

**Implementation**:
- `pkg/datastorage/metrics/metrics.go`
- `pkg/datastorage/metrics/helpers.go`

---

### **7. Error Handling**

**Umbrella BR**: RFC 7807 compliant error responses

**Sub-BRs**:
- **BR-STORAGE-017**: RFC 7807 error responses for all API failures
- **BR-STORAGE-024**: RFC 7807 for 404 Not Found

**Test Coverage**:
- Integration: `test/integration/datastorage/repository_test.go`
- Unit: `test/unit/datastorage/handlers_test.go`

**Implementation**:
- `pkg/datastorage/server/handler.go`

---

### **8. Production Readiness**

**Umbrella BR**: Production-ready features (graceful shutdown, database validation)

**Sub-BRs**:
- **BR-STORAGE-003**: Database version validation (PostgreSQL ≥14, pgvector ≥0.5.0)
- **BR-STORAGE-027**: Large result set handling
- **BR-STORAGE-028**: Graceful shutdown (DD-007 pattern)

**Test Coverage**:
- Unit: `test/unit/datastorage/validator_schema_test.go`
- Unit: `test/unit/datastorage/handlers_test.go`
- Integration: `test/integration/datastorage/graceful_shutdown_test.go`

**Implementation**:
- `pkg/datastorage/schema/validator.go`
- `pkg/datastorage/server/server.go`

---

### **9. Aggregation API**

**Umbrella BR**: Analytics and reporting aggregations

**Sub-BRs**:
- **BR-STORAGE-030**: Aggregation API endpoints
- **BR-STORAGE-031**: Success rate aggregation (incident-type, playbook)
- **BR-STORAGE-032**: Namespace grouping aggregation
- **BR-STORAGE-033**: Severity distribution aggregation
- **BR-STORAGE-034**: Incident trend aggregation (time-series)

**Test Coverage**:
- Integration: `test/integration/datastorage/aggregation_api_test.go`
- Unit: `test/unit/datastorage/aggregation_handlers_test.go`

**Implementation**:
- `pkg/datastorage/server/aggregation_handlers.go`
- `pkg/datastorage/models/aggregation_responses.go`

---

## 📋 **Test File Coverage**

### **Unit Tests** (12 files)

1. `test/unit/datastorage/aggregation_handlers_test.go` - BR-STORAGE-031
2. `test/unit/datastorage/dualwrite_context_test.go` - BR-STORAGE-016
3. `test/unit/datastorage/dualwrite_test.go` - BR-STORAGE-002, 014, 015
4. `test/unit/datastorage/embedding_test.go` - BR-STORAGE-012
5. `test/unit/datastorage/errors_dualwrite_test.go` - BR-STORAGE-002
6. `test/unit/datastorage/handlers_test.go` - BR-STORAGE-021, 024, 025, 027, 028
7. `test/unit/datastorage/metrics_test.go` - BR-STORAGE-007, 009, 010, 013, 014, 015, 019
8. `test/unit/datastorage/query_builder_test.go` - BR-STORAGE-021, 022, 023, 025, 026
9. `test/unit/datastorage/query_test.go` - BR-STORAGE-005, 006, 012
10. `test/unit/datastorage/sanitization_test.go` - BR-STORAGE-011
11. `test/unit/datastorage/validation_test.go` - BR-STORAGE-010
12. `test/unit/datastorage/validator_schema_test.go` - BR-STORAGE-003

### **Integration Tests** (6 files)

1. `test/integration/datastorage/aggregation_api_test.go` - BR-STORAGE-030, 032, 033, 034
2. `test/integration/datastorage/graceful_shutdown_test.go` - BR-STORAGE-028
3. `test/integration/datastorage/http_api_test.go` - BR-STORAGE-001, 020
4. `test/integration/datastorage/repository_test.go` - BR-STORAGE-001, 010, 017
5. `test/integration/datastorage/suite_test.go` - BR-STORAGE-001, 020
6. `test/integration/datastorage/aggregation_api_adr033_test.go` - BR-STORAGE-031 (ADR-033 compliance)

---

## 🔍 **BR Coverage by Test Tier**

### **Unit Test Coverage** (24 BRs - 80%)

BR-STORAGE-002, 003, 005, 006, 007, 009, 010, 011, 012, 013, 014, 015, 016, 019, 021, 022, 023, 024, 025, 026, 027, 028, 031

### **Integration Test Coverage** (10 BRs - 33%)

BR-STORAGE-001, 017, 020, 028, 030, 031, 032, 033, 034

### **2x Coverage (Unit + Integration)** (2 BRs)

- BR-STORAGE-028 (graceful shutdown)
- BR-STORAGE-031 (success rate aggregation)

---

## 📊 **BR Distribution by Category**

| Category | BR Count | Test Files |
|----------|----------|------------|
| **Audit Persistence** | 2 | 3 integration |
| **Dual-Write Coordination** | 4 | 3 unit |
| **Query API** | 5 | 3 unit |
| **Input Validation & Sanitization** | 4 | 4 unit, 1 integration |
| **Embedding & Vector Operations** | 2 | 3 unit |
| **Observability & Metrics** | 3 | 1 unit |
| **Error Handling** | 2 | 1 integration, 1 unit |
| **Production Readiness** | 3 | 2 unit, 1 integration |
| **Aggregation API** | 5 | 1 integration, 1 unit |

---

## 🎯 **Missing BR Numbers**

**Identified Gaps**: BR-STORAGE-004, 008, 018, 029

**Status**: Not found in test files or implementation

**Recommendation**: Triage to determine if:
1. BRs were deprecated and removed
2. BR numbering is intentionally non-sequential
3. BRs exist but are not yet tested

---

## 📚 **References**

- **Business Requirements**: `docs/services/stateless/data-storage/BUSINESS_REQUIREMENTS.md`
- **Test Files**: `test/unit/datastorage/*.go`, `test/integration/datastorage/*.go`
- **Implementation**: `pkg/datastorage/**/*.go`
- **ADR-032**: Data Access Layer Isolation
- **ADR-033**: Incident-Type Aggregation
- **DD-007**: Kubernetes-Aware Graceful Shutdown

---

**Last Updated**: November 8, 2025
**Next Review**: After Phase 1 completion and user approval

