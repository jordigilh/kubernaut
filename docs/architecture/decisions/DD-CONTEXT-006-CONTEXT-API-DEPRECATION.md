# DD-CONTEXT-006: Context API Deprecation Decision

**Date**: November 13, 2025
**Status**: ✅ **APPROVED**
**Decision Maker**: Kubernaut Architecture Team
**Authority**: ADR-034 (Unified Audit Table), DD-CONTEXT-005 (Minimal LLM Response Schema)
**Affects**: Context API, Data Storage Service, All dependent services
**Version**: 1.0

---

## 🎯 **Context**

**Problem**: Context API was created as a prototype service for providing historical context and semantic search to the LLM. However, its functionality overlaps significantly with the Data Storage Service, and its implementation is incomplete.

**Current State**:
- Context API has **80% migrated** to use Data Storage REST API client
- Semantic search was **never implemented** (stub only)
- Aggregation APIs exist but duplicate Data Storage functionality
- Maintenance burden of two overlapping services
- Incomplete migration creates technical debt

**Authoritative Sources**:
- **ADR-034**: Unified Audit Table Design (defines audit data access patterns)
- **DD-CONTEXT-005**: Minimal LLM Response Schema (defines query/response requirements)
- **PKG-CONTEXTAPI-QUERY-TRIAGE-CORRECTED.md**: Migration status analysis (80% complete)

---

## ✅ **Decision**

**APPROVED**: Deprecate Context API and consolidate all functionality into Data Storage Service.

**Confidence**: 98%

**Rationale**:
1. **Functional Overlap**: Context API's planned features (semantic search, aggregation) are better implemented in Data Storage Service
2. **Incomplete Implementation**: Semantic search was never implemented (stub only)
3. **Migration Already In Progress**: 80% of Context API already uses Data Storage REST API
4. **Simplified Architecture**: One service for data access reduces complexity and maintenance burden
5. **ADR-034 Compliance**: Unified audit table design centralizes data access in Data Storage Service
6. **No Production Usage**: Pre-release product allows clean deprecation without backward compatibility concerns

---

## 📊 **Analysis**

### **Context API Current State**

**Implemented Features**:
- ✅ Historical query API (delegates to Data Storage Service)
- ✅ Aggregation APIs (delegates to Data Storage Service)
- ✅ Cache fallback patterns (reusable in Data Storage Service)
- ✅ Graceful shutdown (reusable pattern)
- ✅ RFC 7807 error handling (reusable pattern)

**Unimplemented Features**:
- ❌ Semantic search (stub only, returns "not implemented")
- ❌ Playbook catalog integration (never started)
- ❌ Direct database access (deprecated in favor of Data Storage REST API)

**Migration Status** (from PKG-CONTEXTAPI-QUERY-TRIAGE-CORRECTED.md):
- ✅ 80% complete: Data Storage REST API client infrastructure ready
- ✅ `NewCachedExecutorWithDataStorage` function exists
- ✅ Circuit breaker, retry, graceful degradation implemented
- ❌ 20% remaining: `server.go` still uses deprecated direct DB constructor

---

### **Functional Overlap Analysis**

| Feature | Context API | Data Storage Service | Decision |
|---------|-------------|----------------------|----------|
| **Historical Query** | Delegates to DS | ✅ Native | **Consolidate in DS** |
| **Semantic Search** | Stub only | ✅ Planned (BR-STORAGE-012) | **Implement in DS** |
| **Aggregation** | Delegates to DS | ✅ Native | **Consolidate in DS** |
| **Audit Access** | Delegates to DS | ✅ Native (ADR-034) | **Consolidate in DS** |
| **Playbook Catalog** | Not started | ✅ Planned (DD-STORAGE-008) | **Implement in DS** |
| **Cache Patterns** | ✅ Implemented | ⚠️ Partial | **Migrate patterns to DS** |
| **Error Handling** | ✅ RFC 7807 | ⚠️ Partial | **Migrate patterns to DS** |

**Conclusion**: All Context API functionality is either already in Data Storage Service or better suited there.

---

### **Salvageable Components**

**High-Value Patterns to Migrate**:
1. **Cache Fallback Logic** (`pkg/contextapi/query/executor.go:239-280`)
   - Multi-tier cache with graceful degradation
   - Circuit breaker for external service calls
   - Retry with exponential backoff
   - **Target**: `pkg/datastorage/cache/`

2. **RFC 7807 Error Handling** (`pkg/contextapi/server/error_handlers.go`)
   - Structured error responses
   - HTTP status code mapping
   - **Target**: `pkg/datastorage/server/error_handlers.go`

3. **Graceful Shutdown** (`pkg/contextapi/server/server.go:Shutdown()`)
   - HTTP server graceful close
   - In-flight request draining
   - Timeout enforcement
   - **Target**: `pkg/datastorage/server/server.go`

4. **Integration Test Patterns** (`test/integration/contextapi/`)
   - Cache stampede prevention tests
   - Aggregation edge case tests
   - Graceful shutdown tests
   - **Target**: `test/integration/datastorage/`

**Low-Value Components (Discard)**:
- Direct database access code (deprecated)
- Stub implementations (semantic search)
- Service-specific configuration (no longer needed)

---

### **Business Requirements Impact**

**Context API BRs** (from `docs/services/stateless/context-api/BUSINESS_REQUIREMENTS.md`):

| BR ID | Description | Status | Migration Target |
|-------|-------------|--------|------------------|
| **BR-CONTEXT-001** | Historical Query | ✅ Implemented | Already uses DS (no change) |
| **BR-CONTEXT-002** | Input Validation | ✅ Implemented | Migrate patterns to DS |
| **BR-CONTEXT-003** | Vector Search | ❌ Stub only | **BR-STORAGE-012** (Playbook Embedding) |
| **BR-CONTEXT-004** | Aggregation | ✅ Implemented | Already uses DS (no change) |
| **BR-CONTEXT-005** | Cache Fallback | ✅ Implemented | Migrate to DS cache layer |
| **BR-CONTEXT-006** | Error Handling | ✅ Implemented | Migrate RFC 7807 to DS |
| **BR-CONTEXT-007** | DS Integration | ✅ Implemented | No longer needed (native DS) |
| **BR-CONTEXT-008** | Circuit Breaker | ✅ Implemented | Migrate to DS client |
| **BR-CONTEXT-009** | Retry Logic | ✅ Implemented | Migrate to DS client |
| **BR-CONTEXT-010** | Graceful Shutdown | ✅ Implemented | Migrate to DS server |
| **BR-CONTEXT-011** | Observability | ✅ Implemented | Migrate metrics to DS |
| **BR-CONTEXT-012** | Graceful Shutdown | ✅ Implemented | Migrate to DS server |

**Data Storage BRs** (from `docs/services/stateless/data-storage/BUSINESS_REQUIREMENTS.md`):

| BR ID | Description | Status | Context API Overlap |
|-------|-------------|--------|---------------------|
| **BR-STORAGE-001** | Audit Write API | ✅ Implemented | None (new functionality) |
| **BR-STORAGE-002** | Audit Query API | ✅ Implemented | None (new functionality) |
| **BR-STORAGE-012** | Playbook Embedding | 🚧 Planned | **BR-CONTEXT-003** (Vector Search) |
| **BR-STORAGE-013** | Semantic Search | 🚧 Planned | **BR-CONTEXT-003** (Vector Search) |

**Conclusion**: No BR conflicts. Context API BRs either already use DS or will be migrated.

---

## 📋 **Deprecation Strategy**

### **Phase 1: Complete Migration (0.5 days, 4 hours)**

**Objective**: Finish the 20% remaining Context API migration to Data Storage REST API

**Tasks**:
1. Update `pkg/contextapi/server/server.go` to use `NewCachedExecutorWithDataStorage`
2. Remove deprecated `NewCachedExecutor` constructor
3. Verify all Context API tests pass with Data Storage backend
4. Update Context API documentation to reflect DS dependency

**Deliverable**: Context API 100% uses Data Storage Service (no direct DB access)

**Confidence**: 99% (infrastructure already exists, just needs wiring)

---

### **Phase 2: Salvage Patterns (1 day, 8 hours)**

**Objective**: Migrate high-value patterns from Context API to Data Storage Service

**Tasks**:
1. **Cache Fallback Logic** (2 hours)
   - Create `pkg/datastorage/cache/fallback.go`
   - Migrate multi-tier cache with circuit breaker
   - Add integration tests for cache fallback

2. **RFC 7807 Error Handling** (2 hours)
   - Create `pkg/datastorage/server/error_handlers.go`
   - Migrate structured error response patterns
   - Update all Data Storage endpoints to use RFC 7807

3. **Graceful Shutdown** (2 hours)
   - Update `pkg/datastorage/server/server.go:Shutdown()`
   - Migrate HTTP server graceful close logic
   - Add integration tests for graceful shutdown

4. **Integration Test Patterns** (2 hours)
   - Migrate cache stampede prevention tests
   - Migrate aggregation edge case tests
   - Migrate graceful shutdown tests

**Deliverable**: Data Storage Service has all Context API patterns

**Confidence**: 95% (patterns are well-documented and tested)

---

### **Phase 3: Update Documentation (0.5 days, 4 hours)**

**Objective**: Mark Context API as deprecated in all documentation

**Tasks**:
1. **Architecture Documentation** (1 hour)
   - Update `docs/architecture/SERVICE_DEPENDENCY_MAP.md`
   - Update `docs/architecture/MICROSERVICES_COMMUNICATION_ARCHITECTURE.md`
   - Add deprecation notice to all ADRs/DDs referencing Context API

2. **Service Documentation** (1 hour)
   - Update `docs/services/README.md` (move to deprecated section)
   - Update `README.md` (remove from active services)
   - Add deprecation notice to `docs/services/stateless/context-api/README.md`

3. **Business Requirements** (1 hour)
   - Mark all Context API BRs as "MIGRATED TO DATA STORAGE"
   - Update BR mapping to point to Data Storage BRs
   - Document BR migration path

4. **Implementation Plans** (1 hour)
   - Mark all Context API implementation plans as "COMPLETED (DEPRECATED)"
   - Add references to Data Storage implementation plans
   - Archive Context API implementation plans

**Deliverable**: All documentation reflects Context API deprecation

**Confidence**: 100% (documentation update only)

---

### **Phase 4: Remove Code (0.5 days, 4 hours)**

**Objective**: Remove Context API codebase after pattern migration complete

**Tasks**:
1. **Remove Business Logic** (1 hour)
   - Delete `pkg/contextapi/` (after salvage complete)
   - Delete `cmd/contextapi/` (main application)
   - Verify no other services depend on Context API packages

2. **Remove Tests** (1 hour)
   - Delete `test/integration/contextapi/` (after pattern migration)
   - Delete `test/unit/contextapi/`
   - Delete `test/e2e/contextapi/`

3. **Remove Configuration** (1 hour)
   - Delete Context API Kubernetes manifests
   - Delete Context API Helm charts
   - Update deployment scripts to remove Context API

4. **Final Verification** (1 hour)
   - Run full test suite (ensure no Context API dependencies)
   - Build all services (ensure no import errors)
   - Update CI/CD pipelines to remove Context API

**Deliverable**: Context API code completely removed

**Confidence**: 95% (5% risk of undiscovered dependencies)

---

## 📊 **Timeline Summary**

| Phase | Duration | Effort | Dependencies |
|-------|----------|--------|--------------|
| **Phase 1: Complete Migration** | 0.5 days | 4 hours | None |
| **Phase 2: Salvage Patterns** | 1 day | 8 hours | Phase 1 complete |
| **Phase 3: Update Documentation** | 0.5 days | 4 hours | Phase 2 complete |
| **Phase 4: Remove Code** | 0.5 days | 4 hours | Phase 3 complete |
| **TOTAL** | **2.5 days** | **20 hours** | Sequential |

**Critical Path**: Sequential execution (each phase depends on previous)

**Confidence**: 97% (well-understood scope, clear dependencies)

---

## ✅ **Benefits**

### **Architectural Benefits**
- ✅ **Simplified Architecture**: One service for data access (not two)
- ✅ **Reduced Maintenance**: Single codebase for data operations
- ✅ **Clear Responsibility**: Data Storage Service owns all data access
- ✅ **ADR-034 Compliance**: Unified audit table design fully implemented

### **Development Benefits**
- ✅ **Faster Development**: No context switching between services
- ✅ **Easier Testing**: Single service to test and validate
- ✅ **Better Patterns**: Salvaged patterns improve Data Storage quality
- ✅ **No Technical Debt**: Clean deprecation without backward compatibility burden

### **Operational Benefits**
- ✅ **Fewer Deployments**: One less service to deploy and monitor
- ✅ **Simpler Troubleshooting**: Single service for data access issues
- ✅ **Lower Resource Usage**: Consolidate infrastructure
- ✅ **Clearer Observability**: Single service metrics and logs

---

## 🚨 **Risks and Mitigations**

### **Risk 1: Undiscovered Dependencies**
**Probability**: 5%
**Impact**: MEDIUM (requires additional refactoring)
**Mitigation**:
- Comprehensive grep for Context API imports before removal
- Run full test suite after each phase
- Gradual removal (Phase 2 → Phase 3 → Phase 4)

### **Risk 2: Pattern Migration Complexity**
**Probability**: 10%
**Impact**: LOW (delays Phase 2 by 1-2 days)
**Mitigation**:
- Patterns are well-documented and tested
- Integration tests verify correct behavior
- Incremental migration (one pattern at a time)

### **Risk 3: Documentation Gaps**
**Probability**: 15%
**Impact**: LOW (confusion during transition)
**Mitigation**:
- Comprehensive deprecation notices in all docs
- Clear migration path documented
- BR mapping updated to point to Data Storage

---

## 📊 **Confidence Assessment**

**Overall Confidence**: **98%**

**Breakdown**:
- **Deprecation Decision**: 100% (clear functional overlap, incomplete implementation)
- **Migration Feasibility**: 99% (80% already complete, infrastructure ready)
- **Pattern Salvage**: 95% (patterns well-documented, tested)
- **Timeline Accuracy**: 97% (well-understood scope, clear dependencies)
- **Risk Assessment**: 95% (low-probability risks with clear mitigations)

**Why 98% (not 100%)**:
- 2% uncertainty: Potential undiscovered dependencies on Context API (mitigated by comprehensive testing)

---

## 🔗 **Related Decisions**

- **ADR-034**: Unified Audit Table Design (centralizes data access in Data Storage)
- **DD-CONTEXT-005**: Minimal LLM Response Schema (defines query/response requirements)
- **DD-STORAGE-006**: V1.0 No-Cache Decision (playbook embedding caching strategy)
- **DD-STORAGE-008**: Playbook Catalog Schema (replaces Context API semantic search)
- **DD-STORAGE-009**: Unified Audit Migration Plan (implements ADR-034)
- **PKG-CONTEXTAPI-QUERY-TRIAGE-CORRECTED.md**: Migration status analysis (80% complete)

---

## 📋 **Next Steps**

1. ✅ **DD-CONTEXT-006 Approved** (this document)
2. 🚧 **Create DD-CONTEXT-006-MIGRATION**: Detailed migration plan (Phase 1-4 implementation details)
3. 🚧 **Create DD-STORAGE-010**: Data Storage V1.0 Implementation Plan (includes salvaged patterns)
4. 🚧 **Execute Phase 1**: Complete Context API migration (0.5 days)
5. 🚧 **Execute Phase 2**: Salvage patterns (1 day)
6. 🚧 **Execute Phase 3**: Update documentation (0.5 days)
7. 🚧 **Execute Phase 4**: Remove code (0.5 days)

---

**Document Version**: 1.0
**Last Updated**: November 13, 2025
**Status**: ✅ **APPROVED** (98% confidence, ready for migration planning)
**Next Review**: After Phase 1 complete (Context API 100% migrated to DS)

