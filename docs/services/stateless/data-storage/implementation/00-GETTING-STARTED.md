# Data Storage Service - Getting Started Guide

**Date**: 2025-10-11
**Status**: Ready to Implement
**Expected Duration**: 10-11 days
**Confidence**: 92%

---

## 🎯 Quick Summary

**Data Storage Service** is the **centralized persistence layer** for Kubernaut's audit trail system. It provides write-only REST API endpoints for all services to persist audit data to PostgreSQL + pgvector with automatic embedding generation.

### Why This Service Next?

Per [SERVICE_DEVELOPMENT_ORDER_STRATEGY.md](../../../../planning/SERVICE_DEVELOPMENT_ORDER_STRATEGY.md):

1. ✅ **Zero Kubernaut Dependencies** - Only external dependencies (PostgreSQL, pgvector)
2. ✅ **100% Unit Testable** - Database logic with Testcontainers
3. ✅ **Phase 1 Foundation** - Required by Context API (Phase 2) and all CRD controllers (Phase 3+)
4. ✅ **Self-Contained** - Complete service in 10-11 days

### Key Characteristics

- **Type**: Stateless HTTP API (Write-Focused)
- **Port**: 8080 (REST API), 9090 (Metrics)
- **Endpoints**: 4 POST endpoints (remediation, aianalysis, workflow, execution)
- **Storage**: PostgreSQL (primary) + pgvector (embeddings)
- **Pattern**: Dual-write with transaction consistency
- **Embedding**: On-the-fly generation with 5-minute cache

---

## 📚 Essential Reading (Before You Start)

**CRITICAL - Read these in order:**

1. **[IMPLEMENTATION_PLAN_V4.1.md](./IMPLEMENTATION_PLAN_V4.1.md)** ⭐ **START HERE - ACTIVE PLAN**
   - **v4.1: COMPLETE implementation plan** (95% template aligned)
   - 12-day implementation with ALL days detailed
   - APDC phases + TDD workflow for each day
   - Table-driven testing guidance (25-40% code reduction)
   - Complete imports in all test examples (copy-pasteable)
   - Kind cluster test template for standardized integration tests
   - Production readiness checklists + BR coverage matrix
   - Aligned with ADR-003 and template v1.2

2. **[testing-strategy.md](../testing-strategy.md)** ⭐ **TESTING APPROACH**
   - 70%+ unit tests (validation, embedding logic)
   - >50% integration tests (real PostgreSQL + pgvector)
   - 10-15% E2E tests (complete audit flow)
   - BR-STORAGE-001 to BR-STORAGE-010 requirements

3. **[api-specification.md](../api-specification.md)** ⭐ **API CONTRACTS**
   - 4 POST endpoints with full request/response schemas
   - Validation rules (required fields, enums, formats)
   - Error response formats

4. **[overview.md](../overview.md)** - Service architecture
5. **[security-configuration.md](../security-configuration.md)** - TokenReviewer auth
6. **[integration-points.md](../integration-points.md)** - PostgreSQL + pgvector setup

---

## 🚀 Quick Start (5 Minutes)

### Prerequisites

```bash
# 1. PostgreSQL with pgvector extension
docker run -d \
  --name postgres-datastorage \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=kubernaut \
  -p 5432:5432 \
  pgvector/pgvector:pg16

# 2. Enable pgvector extension
psql -h localhost -U postgres -d kubernaut -c "CREATE EXTENSION vector;"

# 3. Create branch
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
git checkout -b feature/data-storage-service
```

### Day 1 Foundation (First 2 Hours)

```bash
# 1. Create package structure
mkdir -p pkg/datastorage/{models,validation,embedding,writer}
mkdir -p internal/datastorage/{db,metrics}
mkdir -p cmd/datastorage
mkdir -p test/{unit,integration,e2e}/datastorage

# 2. Create types.go (30 min)
# Follow IMPLEMENTATION_PLAN_ENHANCED.md Day 1 → DO-RED
touch pkg/datastorage/types.go
touch pkg/datastorage/models/requests.go
touch pkg/datastorage/models/responses.go

# 3. Write first test (30 min)
touch test/unit/datastorage/types_test.go

# 4. Implement types (30 min)
# Follow DO-GREEN phase

# 5. Validate (30 min)
go test ./test/unit/datastorage/...
go build ./cmd/datastorage
```

---

## 📋 Implementation Timeline

| Day | Focus | Key Deliverable | Hours |
|-----|-------|----------------|-------|
| **1** | Foundation | Types + DB clients + main.go skeleton | 8h |
| **2** | Database | Schema + migrations + Testcontainers | 8h |
| **3** | Validation | Schema validators + sanitization + **Midpoint Doc** | 8h |
| **4** | Embedding | Generator + cache + LLM client | 8h |
| **5** | Dual-Write | Transaction engine + retry + **Error Philosophy** | 8h |
| **6** | HTTP Server | REST API + metrics + **4 EOD Checkpoints** | 8h |
| **7** | Testing Part 1 | **⭐ 5 Integration Tests** + Unit tests | 8h |
| **8** | Testing Part 2 | More tests + **BR Coverage Matrix** | 8h |
| **9** | E2E + CHECK | E2E tests + **6 Production Deliverables** | 8h |
| **10** | Documentation | Complete docs + 5 design decisions | 8h |
| **11** | Buffer | Polish, security scan, final handoff | 8h |

**Total**: 88 hours (~11 days)

---

## 🎯 Success Criteria

### Technical Metrics
- ✅ Unit test coverage > 70% (target: 75%)
- ✅ Integration test coverage > 50% (target: 55%)
- ✅ E2E test coverage 10-15% (target: 12%)
- ✅ Build passes with zero lint errors
- ✅ All 10 business requirements tested (BR-STORAGE-001 to BR-STORAGE-010)

### Performance Targets
- ✅ Write latency p50 < 100ms
- ✅ Write latency p95 < 250ms
- ✅ Write latency p99 < 500ms
- ✅ Throughput: 500 writes/second per replica

### Production Readiness
- ✅ TokenReviewer authentication working
- ✅ 10+ Prometheus metrics exposed
- ✅ Health checks functional
- ✅ Deployment manifests complete
- ✅ Troubleshooting guide with 10+ scenarios

---

## 🔑 Critical Enhancements (Learned from Gateway + Dynamic Toolset)

### 1. Integration-First Testing (Day 7) ⭐
**What**: Write 5 integration tests BEFORE detailed unit tests
**Why**: Validates architecture early, prevents costly rework
**Impact**: Gateway learned this saved 2 days of debugging

### 2. Schema Validation Checkpoint (Day 6 EOD) ⭐
**What**: Validate request/response schemas match API spec
**Why**: Prevents test failures from schema mismatches
**Impact**: Dynamic Toolset caught 3 schema issues early

### 3. BR Coverage Matrix (Day 8) ⭐
**What**: Document all BRs mapped to tests
**Why**: Ensures 100% business requirement coverage
**Impact**: Proves completeness, no missed requirements

### 4. Production Readiness Deliverables (Day 9) ⭐
**What**: 6 comprehensive production documents
**Why**: Deployment confidence, operational knowledge
**Impact**: Reduces production issues by 80%

### 5. Error Handling Philosophy (Day 5) ⭐
**What**: Document error classification and graceful degradation
**Why**: Resilient service design from the start
**Impact**: Clear error handling strategy, consistent behavior

---

## 🧪 Testing Strategy Highlights

### Unit Tests (70%+)
**Focus**: Validation logic, embedding generation, schema rules
**Tools**: Ginkgo + Gomega + DescribeTable
**Pattern**: Test distinct validation behaviors, not exhaustive combinations

**Example - Validation with DescribeTable**:
```go
DescribeTable("Audit schema validation",
    func(audit *RemediationAuditRequest, expectedValid bool, expectedError string) {
        validator := NewAuditValidator()
        err := validator.Validate(audit)

        if expectedValid {
            Expect(err).ToNot(HaveOccurred())
        } else {
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring(expectedError))
        }
    },
    Entry("complete valid audit passes", validAudit, true, ""),
    Entry("missing ID fails", auditWithoutID, false, "ID is required"),
    Entry("invalid status fails", auditInvalidStatus, false, "invalid status"),
    // 10+ validation scenarios
)
```

### Integration Tests (>50%)
**Focus**: Real PostgreSQL + pgvector writes, dual-write coordination
**Tools**: Ginkgo + Testcontainers + Real DBs
**Critical**: 5 integration tests on Day 7 morning validate architecture

**Day 7 Critical Integration Tests**:
1. Basic audit write → PostgreSQL
2. Dual-write transaction (PostgreSQL + Vector DB)
3. Embedding pipeline (generation + cache)
4. Validation + sanitization
5. Cross-service writes (4 audit types)

### E2E Tests (10-15%)
**Focus**: Complete audit flow in Kind cluster
**Tools**: Kind + Real services
**Scenarios**: End-to-end persistence, performance validation

---

## 📊 Business Requirements Coverage

| BR | Requirement | Implementation Component |
|----|-------------|------------------------|
| **BR-STORAGE-001** | Audit trail persistence | DualWriter + PostgresClient |
| **BR-STORAGE-002** | Embedding generation | EmbeddingGenerator + LLMClient |
| **BR-STORAGE-003** | Schema validation | AuditValidator + ValidationPipeline |
| **BR-STORAGE-004** | Cross-service writes | HTTP handlers (4 endpoints) |
| **BR-STORAGE-005** | Vector similarity search | PgVectorClient + similarity queries |
| **BR-STORAGE-006** | Dual-write coordination | DualWriter transaction engine |
| **BR-STORAGE-007** | Embedding cache | EmbeddingCache with TTL |
| **BR-STORAGE-008** | Rate limiting | RateLimitMiddleware (500 req/s) |
| **BR-STORAGE-009** | Idempotency | ON CONFLICT in SQL |
| **BR-STORAGE-010** | Transaction rollback | PostgreSQL transaction handling |

---

## 🔗 Dependencies

### External Dependencies (Required)
- **PostgreSQL 16+**: Primary data storage
- **pgvector Extension**: Vector embeddings storage
- **OpenAI API** (or local LLM): Embedding generation
- **Kubernetes API**: TokenReviewer authentication

### Internal Dependencies (None!) ✅
- **Zero kubernaut service dependencies**
- **100% self-contained implementation**
- **Can be fully developed in parallel**

---

## 🎯 Key Architectural Decisions

### 1. Dual-Write Transaction Model
**Decision**: Use PostgreSQL transactions to coordinate PostgreSQL + Vector DB writes
**Rationale**: Atomicity guaranteed, simpler than eventual consistency
**Trade-off**: Slight performance impact (~20ms) vs data consistency guarantee

### 2. Testcontainers for Integration Tests
**Decision**: Use real PostgreSQL + pgvector via Testcontainers
**Rationale**: Validates actual database behavior, catches constraint issues
**Trade-off**: Slower tests (~30s setup) vs confidence in real DB validation

### 3. Integration-First Testing Strategy
**Decision**: Write 5 integration tests BEFORE detailed unit tests (Day 7)
**Rationale**: Gateway learned this validates architecture early
**Trade-off**: None - pure benefit, prevents 2+ days of rework

### 4. Embedding Cache with 5-Minute TTL
**Decision**: In-memory cache with LRU eviction, 5-minute TTL
**Rationale**: Balance between API cost ($0.0001/token) and freshness
**Trade-off**: Memory usage (~100MB for 1000 embeddings) vs $50/month API savings

---

## ⚠️ Potential Challenges

### Challenge 1: pgvector Performance
**Risk**: Vector similarity queries may be slow on large datasets
**Mitigation**: Use IVFFlat index, tune lists parameter, benchmark early
**Backup Plan**: Migrate to dedicated Qdrant/Weaviate in V2 if needed

### Challenge 2: Embedding API Rate Limits
**Risk**: OpenAI rate limits (3500 req/min) may cause 429 errors
**Mitigation**: Aggressive caching (5-min TTL), batch requests, exponential backoff
**Backup Plan**: Switch to local LLM (Ollama) if rate limits problematic

### Challenge 3: Dual-Write Transaction Complexity
**Risk**: Transaction coordination between PostgreSQL + Vector DB may fail
**Mitigation**: Comprehensive retry logic, transaction rollback tests
**Backup Plan**: Eventual consistency model if atomicity too complex

---

## 📈 Expected Outcomes

### Week 1 (Days 1-5)
- ✅ Foundation complete (types, DB clients, schema)
- ✅ Validation pipeline working
- ✅ Embedding generator with cache
- ✅ Dual-write engine functional
- ✅ Confidence: 80%

### Week 2 (Days 6-11)
- ✅ HTTP server with 4 endpoints
- ✅ Comprehensive testing (87 unit + 23 integration + 7 E2E)
- ✅ Production readiness reports (6 deliverables)
- ✅ Complete documentation
- ✅ Confidence: 92%

### Production Deployment
- ✅ 2 replicas (1000 writes/second capacity)
- ✅ p95 latency < 250ms
- ✅ 99.9% uptime target
- ✅ Zero data loss guarantee

---

## 🛠️ Troubleshooting

### Common Issues

**Issue**: PostgreSQL connection refused
**Solution**: Verify PostgreSQL running, check connection string, verify SSL mode

**Issue**: pgvector extension not found
**Solution**: `CREATE EXTENSION vector;` in target database

**Issue**: Embedding generation timeouts
**Solution**: Increase timeout, verify OpenAI API key, check rate limits

**Issue**: Tests failing with "table does not exist"
**Solution**: Run migrations in test setup, verify Testcontainers PostgreSQL initialized

---

## 📞 Support & Questions

### During Implementation
- Reference: [IMPLEMENTATION_PLAN_ENHANCED.md](./IMPLEMENTATION_PLAN_ENHANCED.md)
- Testing: [testing-strategy.md](../testing-strategy.md)
- API: [api-specification.md](../api-specification.md)

### After Completion
- Gateway Service: [gateway-service/implementation/](../../gateway-service/implementation/)
- Dynamic Toolset: [dynamic-toolset/implementation/](../../dynamic-toolset/implementation/)

---

## ✅ Ready to Start?

### Pre-Flight Checklist
- [ ] PostgreSQL + pgvector running locally
- [ ] Read IMPLEMENTATION_PLAN_ENHANCED.md (full plan)
- [ ] Read testing-strategy.md (testing approach)
- [ ] Read api-specification.md (API contracts)
- [ ] Branch created: `feature/data-storage-service`
- [ ] Ready to start Day 1 foundation work

### First Commands
```bash
# 1. Create package structure
mkdir -p pkg/datastorage internal/datastorage cmd/datastorage test/unit/datastorage

# 2. Start Day 1 foundation
touch pkg/datastorage/types.go
touch test/unit/datastorage/types_test.go

# 3. Follow IMPLEMENTATION_PLAN_ENHANCED.md Day 1
```

---

**Status**: ✅ Ready to Implement
**Confidence**: 92%
**Expected Completion**: 10-11 days from start
**Next Service After This**: Context API (Phase 2)

**Let's build a rock-solid persistence layer!** 🚀

