# Context API - Next Tasks

**Service**: Context API (Phase 2 - Intelligence Layer)
**Status**: ⏸️ **BLOCKED - Waiting for Data Storage Service**
**Timeline**: 12 days (96 hours)
**Implementation Plan**: [IMPLEMENTATION_PLAN_V1.0.md](IMPLEMENTATION_PLAN_V1.0.md)

---

## 🚨 DEPENDENCY BLOCK

**Context API implementation is BLOCKED until Data Storage Service is complete.**

**Reason**: Context API queries the `incident_events` table, which is owned by Data Storage Service.

**Required Before Starting**:
- ✅ Data Storage Service implemented (Phase 1)
- ✅ `incident_events` table schema finalized
- ✅ pgvector extension configured
- ✅ Database migrations complete
- ✅ Test data available for integration tests

**Estimated Unblock Date**: After Data Storage Service completion (Phase 1)

---

---

## 🎯 Pre-Implementation Tasks (Complete First)

### ✅ COMPLETED
- [x] Implementation plan created (5,685 lines, 99% confidence)
- [x] Design decision DD-CONTEXT-001 documented (REST vs RAG)
- [x] All 5 risk mitigations approved
- [x] Business requirements defined (12 BRs, 100% coverage)

### ⏸️ PENDING (Do Before Day 1)

#### 1. Infrastructure Setup
- [ ] **PODMAN Environment Validation** (Pre-Day 1, 2h)
  - Create validation script: `scripts/context-api/validate-podman-env.sh`
  - Test PostgreSQL container (port 5434)
  - Test Redis container (port 6380)
  - Test pgvector extension availability
  - Verify network connectivity between containers
  - **File**: `scripts/context-api/validate-podman-env.sh`

#### 2. Database Schema Preparation
- [ ] **Review Data Storage Service Schema** (Pre-Day 1, 1h)
  - Read `docs/services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V4.1.md`
  - Understand `incident_events` table structure
  - Verify pgvector column exists (`embedding_vector`)
  - Confirm query requirements match schema
  - **Dependencies**: Data Storage Service must be complete

#### 3. Business Requirements Validation
- [ ] **Create Business Requirements Document** (Pre-Day 1, 2h)
  - Document all 12 Context API business requirements
  - Map to specific features
  - Define acceptance criteria
  - **File**: `docs/services/stateless/context-api/BUSINESS_REQUIREMENTS.md`

#### 4. Tool Definition for Dynamic Toolset
- [ ] **Define Context API Tools** (Pre-Day 1, 1h)
  - Create tool definition YAML for Dynamic Toolset Service
  - Define `query_historical_incidents` tool
  - Define `pattern_match_incidents` tool
  - Define `aggregate_incidents` tool
  - **File**: `config.app/holmesgpt-tools/context-api-tools.yaml`

---

## 📅 Implementation Tasks (12 Days)

### Phase 1: Foundation (Days 1-4)

#### Day 1: Package Structure + Database Client (8h)
- [ ] **Morning: APDC Analysis Phase** (2h)
  - Execute Pre-Day 1 PODMAN validation
  - Review existing Data Storage patterns
  - Identify integration points
  - Document findings in `phase0/00-day1-analysis.md`

- [ ] **Afternoon: Package Structure** (3h)
  - Create `pkg/contextapi/` directory structure
  - Initialize Go modules
  - Define base interfaces
  - Setup logging infrastructure
  - **Deliverable**: Package skeleton

- [ ] **Evening: Database Client** (3h)
  - Implement PostgreSQL connection pooling
  - Add health check methods
  - Create basic query executor
  - Write unit tests (5+ tests)
  - **Deliverable**: Working database client

- [ ] **EOD Documentation**
  - Complete Day 1 EOD report
  - Update confidence assessment
  - Document blockers (if any)
  - **File**: `phase0/00-day1-complete.md`

#### Day 2: Query Builder (8h)
- [ ] **TDD RED Phase** (3h)
  - Write failing tests for query builder
  - Test parameterized SQL generation
  - Test multi-parameter filtering
  - Test pagination
  - **Deliverable**: 10+ failing tests

- [ ] **TDD GREEN Phase** (3h)
  - Implement minimal query builder
  - Parameterized SQL (SQL injection prevention)
  - Support: alert_name, namespace, workflow_status filters
  - Pagination support (limit, offset)
  - **Deliverable**: Tests passing

- [ ] **TDD REFACTOR Phase** (2h)
  - Extract query builder interface
  - Add query validation
  - Optimize query generation
  - **Deliverable**: Clean, reusable code

#### Day 3: Redis Cache Manager (8h)
- [ ] **TDD RED Phase** (3h)
  - Write cache manager tests
  - Test cache hit/miss scenarios
  - Test TTL management
  - Test error handling
  - **Deliverable**: 12+ failing tests

- [ ] **TDD GREEN Phase** (3h)
  - Implement Redis cache manager
  - Cache key generation
  - TTL configuration (5 minutes default)
  - Error handling for Redis failures
  - **Deliverable**: Tests passing

- [ ] **TDD REFACTOR Phase** (2h)
  - Add L2 in-memory cache (fallback)
  - Implement graceful degradation
  - Add cache metrics
  - **Deliverable**: Multi-tier caching

#### Day 4: Cached Query Executor (8h)
- [ ] **TDD RED Phase** (3h)
  - Write cached executor tests
  - Test cache-first strategy
  - Test fallback to database
  - Test cache population
  - **Deliverable**: 10+ failing tests

- [ ] **TDD GREEN Phase** (3h)
  - Implement cached executor
  - Cache → Database fallback
  - Cache population on miss
  - **Deliverable**: Tests passing

- [ ] **TDD REFACTOR Phase** (2h)
  - Extract caching logic
  - Add circuit breaker pattern
  - Optimize cache key generation
  - **Deliverable**: Production-ready caching

- [ ] **EOD: Error Handling Philosophy**
  - Create error handling philosophy document
  - Define error classifications
  - Document retry strategies
  - **File**: `design/ERROR_HANDLING_PHILOSOPHY.md`

---

### Phase 2: Advanced Features (Days 5-7)

#### Day 5: Vector DB Pattern Matching (8h)
- [ ] **TDD RED Phase** (2h)
  - Write pgvector search tests
  - Table-driven similarity threshold tests (5 scenarios)
  - Test namespace filtering
  - **Deliverable**: 8+ failing tests

- [ ] **TDD GREEN Phase** (4h)
  - Implement vector search with pgvector
  - Similarity threshold filtering
  - Score ordering
  - **Deliverable**: Tests passing

- [ ] **TDD REFACTOR Phase** (2h)
  - Add embedding service interface
  - Create mock embedding service
  - **Deliverable**: Reusable vector search

#### Day 6: Query Router + Aggregation (8h)
- [ ] **TDD RED Phase** (2h)
  - Write query router tests
  - Write aggregation tests
  - Table-driven route selection tests
  - **Deliverable**: 8+ failing tests

- [ ] **TDD GREEN Phase** (4h)
  - Implement query router
  - Implement aggregation queries
  - Success rate calculation
  - **Deliverable**: Tests passing

- [ ] **TDD REFACTOR Phase** (2h)
  - Extract aggregation service
  - Optimize SQL queries
  - **Deliverable**: Clean aggregation layer

#### Day 7: HTTP API + Metrics (8h)
- [ ] **Morning: HTTP API** (3h)
  - Implement 5 REST endpoints
  - Add request validation
  - Add middleware (logging, recovery)
  - **Deliverable**: Working HTTP API

- [ ] **Morning: Prometheus Metrics** (2h)
  - Define 10+ metrics
  - Implement metrics recording
  - **Deliverable**: Metrics exposed

- [ ] **Afternoon: Health Checks** (3h)
  - Implement liveness probe
  - Implement readiness probe
  - Add component health checks
  - **Deliverable**: Health endpoints

- [ ] **EOD Documentation**
  - Complete Day 7 EOD report
  - Document all core features complete
  - **File**: `phase0/03-day7-complete.md`

---

### Phase 3: Testing (Days 8-10)

#### Day 8: Integration Tests (8h)
- [ ] **Morning: Test Infrastructure** (1h)
  - Setup PODMAN test suite
  - Configure PostgreSQL + Redis
  - **Deliverable**: Test infrastructure ready

- [ ] **Integration Test 1: Query Lifecycle** (1.5h)
  - Test: API → Cache → Database flow
  - Validate cache population
  - **File**: `test/integration/contextapi/query_lifecycle_test.go`

- [ ] **Integration Test 2: Cache Fallback** (1h)
  - Test: Redis failure → Database
  - Validate graceful degradation
  - **File**: `test/integration/contextapi/cache_fallback_test.go`

- [ ] **Integration Test 3: Pattern Matching** (1.5h)
  - Test: pgvector semantic search
  - Validate similarity thresholds
  - **File**: `test/integration/contextapi/pattern_match_test.go`

- [ ] **Integration Test 4: Aggregation** (1h)
  - Test: Multi-table joins
  - Validate statistics
  - **File**: `test/integration/contextapi/aggregation_test.go`

- [ ] **Integration Test 5: Performance** (1h)
  - Test: Latency <200ms
  - Validate throughput
  - **File**: `test/integration/contextapi/performance_test.go`

- [ ] **Integration Test 6: Cache Consistency** (1h)
  - Test: Cache invalidation
  - Validate TTL expiration
  - **File**: `test/integration/contextapi/cache_consistency_test.go`

#### Day 9: Unit Tests + BR Coverage Matrix (8h)
- [ ] **Morning: Remaining Unit Tests** (4h)
  - API validation tests (SQL injection)
  - Cache eviction tests
  - Error handling tests
  - **Deliverable**: 55+ total unit tests

- [ ] **Afternoon: BR Coverage Matrix** (4h)
  - Document 100% BR coverage (12/12 BRs)
  - Map all tests to BRs
  - Create coverage gap analysis
  - **File**: `testing/BR-COVERAGE-MATRIX.md`

#### Day 10: E2E Testing + Performance (8h)
- [ ] **Morning: E2E Workflow Tests** (3h)
  - Test: Complete recovery workflow
  - Test: Multi-tool LLM orchestration
  - **File**: `test/e2e/contextapi/full_workflow_test.go`

- [ ] **Afternoon: Performance Validation** (3h)
  - Test: p95 latency <200ms
  - Test: Throughput >1000 req/s
  - Test: Cache hit rate >80%
  - **File**: `test/e2e/contextapi/performance_test.go`

- [ ] **Validation** (2h)
  - All tests passing
  - Performance targets met
  - BR coverage 100%

---

### Phase 4: Documentation & Production (Days 11-12)

#### Day 11: Documentation (8h)
- [ ] **Morning: Service README** (3h)
  - Complete service overview
  - API reference documentation
  - Configuration guide
  - Troubleshooting tips
  - **File**: `docs/services/stateless/context-api/README.md`

- [ ] **Afternoon: Design Decisions** (3h)
  - DD-CONTEXT-002: Multi-tier caching strategy
  - DD-CONTEXT-003: Hybrid storage (PostgreSQL + pgvector)
  - DD-CONTEXT-004: Monthly table partitioning
  - **Files**: `design/DD-CONTEXT-00*.md`

- [ ] **Evening: Testing Documentation** (2h)
  - Testing strategy document
  - Test coverage report
  - Known limitations
  - **File**: `testing/TESTING_STRATEGY.md`

#### Day 12: Production Readiness (8h)
- [ ] **Morning: Production Readiness Assessment** (4h)
  - Complete 109-point checklist
  - Target: 95+/109 points (87%+)
  - Document gaps and mitigations
  - **File**: `PRODUCTION_READINESS_REPORT.md`

- [ ] **Afternoon: Deployment Manifests** (2h)
  - Create Kubernetes Deployment
  - Create Service, ConfigMap, Secrets
  - Create RBAC (ServiceAccount, Role)
  - Create HorizontalPodAutoscaler
  - **Directory**: `deploy/context-api/`

- [ ] **Afternoon: Handoff Summary** (2h)
  - Complete handoff document
  - Document lessons learned
  - Provide troubleshooting guide
  - Final confidence assessment
  - **File**: `00-HANDOFF-SUMMARY.md`

---

## 🚀 Post-Implementation Tasks

### Integration with Other Services

#### 1. Dynamic Toolset Service Integration
- [ ] **Register Context API Tools** (1h)
  - Add Context API to service discovery
  - Register tool definitions in ConfigMap
  - Validate tool accessibility
  - **File**: ConfigMap update in Dynamic Toolset

#### 2. HolmesGPT API Integration (Phase 2)
- [ ] **Test Tool Invocation** (2h)
  - HolmesGPT API calls Context API
  - Validate tool response format
  - Test error handling
  - **Files**: Integration tests in HolmesGPT API

#### 3. AIAnalysis Service Integration (Phase 4)
- [ ] **End-to-End Validation** (3h)
  - AIAnalysis → HolmesGPT API → LLM → Context API
  - Validate CRD-based flow
  - Test multi-tool orchestration
  - **Files**: E2E tests in AIAnalysis Service

---

## 📋 Validation Checklist

### Pre-Implementation (Before Day 1)
- [ ] PODMAN environment validated
- [ ] Data Storage Service schema reviewed
- [ ] Business requirements documented
- [ ] Tool definitions created

### Implementation Complete (After Day 12)
- [ ] All 5 REST endpoints functional
- [ ] 55+ unit tests passing (>70% coverage)
- [ ] 6 integration tests passing (>60% coverage)
- [ ] 2+ E2E tests passing
- [ ] BR coverage matrix: 100% (12/12 BRs)
- [ ] Production readiness: 95+/109 points
- [ ] All documentation complete
- [ ] Deployment manifests created

### Integration Complete (Phase 4)
- [ ] Dynamic Toolset integration verified
- [ ] HolmesGPT API integration working
- [ ] AIAnalysis Service integration working
- [ ] Multi-tool LLM orchestration validated

---

## 🔗 Related Documents

- [IMPLEMENTATION_PLAN_V1.0.md](IMPLEMENTATION_PLAN_V1.0.md) - Detailed 12-day plan (5,685 lines)
- [DD-CONTEXT-001-REST-API-vs-RAG.md](design/DD-CONTEXT-001-REST-API-vs-RAG.md) - Architecture decision
- [APPROVED_MICROSERVICES_ARCHITECTURE.md](../../../architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md) - V1 architecture
- [SERVICE_DEVELOPMENT_ORDER_STRATEGY.md](../../../planning/SERVICE_DEVELOPMENT_ORDER_STRATEGY.md) - Phase 2 timeline

---

## 📞 Key Contacts

**Service Owner**: TBD (assign after implementation)
**Implementation Team**: Kubernaut Core Team
**Dependencies**: Data Storage Service (Phase 1)

---

**Status**: ✅ Ready to Begin
**Next Action**: Complete Pre-Implementation Tasks → Start Day 1
**Timeline**: 12 days (96 hours)
**Confidence**: 98%

