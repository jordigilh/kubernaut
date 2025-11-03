# Day 7: Integration Tests - Implementation Plan

**Date**: 2025-11-03  
**Duration**: 3 hours (reduced from 10h due to 1 audit table only)  
**Status**: ✅ **READY** - Analysis Complete, Plan Approved  
**Methodology**: APDC-TDD (Analysis → Plan → DO-RED → DO-GREEN → DO-REFACTOR → CHECK)

---

## 📋 **APDC ANALYSIS PHASE** ✅ **COMPLETE**

### **Business Context**
- **BR-STORAGE-001 to BR-STORAGE-020**: Validate audit write API end-to-end
- **Goal**: 100% integration test pass rate with real infrastructure
- **Success Criteria**: p95 latency <1s, correct data persistence

### **Technical Context** (Existing Patterns Found)
1. ✅ **ADR-016**: Use Podman for stateless services (not Kind)
2. ✅ **Context API Pattern**: Podman Redis + PostgreSQL integration
3. ✅ **Schema Propagation**: 2-second wait after migrations (critical lesson)
4. ✅ **Test Package**: Same package name as production code (`package datastorage`)
5. ✅ **Behavior + Correctness**: Test BOTH (pagination bug lesson)

### **Infrastructure Requirements**
- PostgreSQL 16 with pgvector (port 5433)
- Redis 7 for DLQ (port 6379)
- Migration files: `migrations/010_audit_write_api_phase1.sql`

### **Risks Identified & Mitigations**
| Risk | Mitigation |
|------|------------|
| Schema propagation timing | 2-second wait after migrations |
| Container cleanup | Stop/rm before start |
| Pagination accuracy | Direct DB query validation |

---

## 📝 **APDC PLAN PHASE** ✅ **COMPLETE**

### **TDD Strategy**

#### **RED Phase** (1h)
**Create test files FIRST** (tests define contract):
1. `test/integration/datastorage/suite_test.go` - Infrastructure setup
2. `test/integration/datastorage/repository_test.go` - Repository CRUD tests
3. `test/integration/datastorage/dlq_test.go` - DLQ fallback tests

**Test Contract**:
- Repository Create() with real PostgreSQL
- Repository GetByNotificationID() with real data
- DLQ EnqueueNotificationAudit() with real Redis
- DLQ GetDLQDepth() accuracy
- Behavior + Correctness validation

#### **GREEN Phase** (1.5h)
**Implement infrastructure** to make tests pass:
1. Podman PostgreSQL setup (pgvector enabled)
2. Podman Redis setup
3. Migration application with propagation wait
4. Cleanup logic

#### **REFACTOR Phase** (30min)
**Enhance**:
- Add performance validation (p95 latency)
- Add concurrent write tests
- Improve error messages

### **Integration Plan**

**Test Structure**:
```
test/integration/datastorage/
├── suite_test.go           # Infrastructure setup (BeforeSuite/AfterSuite)
├── repository_test.go      # Repository integration tests
└── dlq_test.go            # DLQ integration tests
```

**Infrastructure Components**:
1. PostgreSQL container (`datastorage-postgres-test`)
2. Redis container (`datastorage-redis-test`)
3. Migration application
4. Permission grants
5. Schema propagation wait

### **Success Criteria**

| Criterion | Target | Validation Method |
|-----------|--------|-------------------|
| **Test Pass Rate** | 100% | All integration tests pass |
| **Data Persistence** | 100% | Direct DB query matches write |
| **DLQ Fallback** | 100% | Redis contains failed writes |
| **Pagination Accuracy** | 100% | Total = DB COUNT(*), not len(array) |
| **Performance** | p95 <1s | Measure write latency |

### **Timeline**

| Phase | Duration | Tasks |
|-------|----------|-------|
| **RED** | 1h | Write tests FIRST (repository + DLQ) |
| **GREEN** | 1.5h | Implement Podman infrastructure |
| **REFACTOR** | 30min | Enhance performance validation |
| **Total** | **3h** | Integration tests complete |

---

## 🔴 **DO-RED PHASE** (1 hour) - NEXT

### **Task 1: Create Suite Setup** (20 min)

**File**: `test/integration/datastorage/suite_test.go`

**Contract**:
```go
package datastorage  // ← Same package as production code

var (
    db           *sql.DB
    redisClient  *redis.Client
    ctx          context.Context
)

var _ = BeforeSuite(func() {
    // Start PostgreSQL with pgvector
    // Start Redis for DLQ
    // Apply migrations
    // Wait for schema propagation (2s)
    // Verify schema
})

var _ = AfterSuite(func() {
    // Cleanup containers
})
```

### **Task 2: Repository Integration Tests** (25 min)

**File**: `test/integration/datastorage/repository_test.go`

**Contract**:
```go
var _ = Describe("NotificationAudit Repository Integration", func() {
    Context("Create", func() {
        It("should persist audit to real PostgreSQL", func() {
            // Behavior: Create returns ID
            // Correctness: Data in DB matches input
        })

        It("should handle unique constraint violation", func() {
            // Behavior: Returns RFC7807 conflict error
            // Correctness: No duplicate in DB
        })
    })

    Context("GetByNotificationID", func() {
        It("should retrieve existing audit", func() {
            // Behavior: Returns audit
            // Correctness: All fields match
        })

        It("should return not found for missing ID", func() {
            // Behavior: Returns RFC7807 not found error
        })
    })
})
```

### **Task 3: DLQ Integration Tests** (15 min)

**File**: `test/integration/datastorage/dlq_test.go`

**Contract**:
```go
var _ = Describe("DLQ Client Integration", func() {
    Context("EnqueueNotificationAudit", func() {
        It("should enqueue to real Redis Stream", func() {
            // Behavior: No error
            // Correctness: Message in Redis with correct structure
        })
    })

    Context("GetDLQDepth", func() {
        It("should return accurate count from Redis", func() {
            // Behavior: Returns count
            // Correctness: Matches Redis XLEN
        })
    })
})
```

---

## 🟢 **DO-GREEN PHASE** (1.5 hours)

### **Task 1: Infrastructure Setup** (45 min)

**Implement BeforeSuite**:
1. Start PostgreSQL container (pgvector/pgvector:pg16)
2. Start Redis container (redis:7-alpine)
3. Wait for services ready
4. Apply migration `010_audit_write_api_phase1.sql`
5. Grant permissions to test user
6. **CRITICAL**: Wait 2 seconds for schema propagation
7. Verify schema using pg_class (not information_schema)

### **Task 2: Repository Tests Implementation** (30 min)

**Make repository tests pass**:
- Use real PostgreSQL connection
- Verify data with direct SQL queries
- Test RFC7807 error responses

### **Task 3: DLQ Tests Implementation** (15 min)

**Make DLQ tests pass**:
- Use real Redis connection
- Verify messages with Redis XRANGE
- Test DLQ depth accuracy

---

## 🔄 **DO-REFACTOR PHASE** (30 minutes)

### **Enhancements**:
1. Add performance measurement (p95 latency)
2. Add concurrent write tests (10 parallel writes)
3. Improve error messages
4. Add health check validation

---

## ✅ **CHECK PHASE** (15 minutes)

### **Validation Checklist**:
- [ ] All integration tests pass (100%)
- [ ] Data persistence verified with direct DB queries
- [ ] DLQ fallback working with real Redis
- [ ] Pagination accuracy validated
- [ ] Performance meets SLA (p95 <1s)
- [ ] Infrastructure cleanup works
- [ ] No flaky tests

---

## 📊 **Confidence Assessment**

**Overall Confidence**: **100%**

**Rationale**:
1. ✅ **Existing Pattern**: Context API integration tests proven successful
2. ✅ **Clear Infrastructure**: ADR-016 mandates Podman approach
3. ✅ **Lessons Applied**: Schema propagation, pagination accuracy
4. ✅ **Reduced Scope**: 1 audit table (not 6) = simpler tests
5. ✅ **TDD Compliance**: Tests written FIRST, infrastructure SECOND

**Risk Assessment**:
- ✅ **No Blockers**: All dependencies available
- ✅ **Proven Approach**: Context API pattern works
- ⚠️ **Low Risk**: Schema propagation timing (mitigated by 2s wait)

---

## 🔗 **References**

### **Architecture Decisions**
- ADR-016: Service-Specific Integration Test Infrastructure (Podman for stateless)
- ADR-027: Multi-Architecture Container Build Strategy (UBI base images)
- DD-009: Audit Write Error Recovery (DLQ pattern)

### **Implementation Plans**
- IMPLEMENTATION_PLAN_V4.8.md: Day 7 specifications
- Context API integration tests: `test/integration/contextapi/suite_test.go`

### **Migrations**
- `migrations/010_audit_write_api_phase1.sql`: Notification audit table

---

## ✅ **Plan Approval Gate**

**Ready to Proceed**: ✅ **YES**

**Approval Criteria Met**:
- [x] Analysis complete (existing patterns identified)
- [x] Plan detailed (TDD phases defined)
- [x] Success criteria clear (100% pass rate, correctness validation)
- [x] Timeline realistic (3 hours for 1 audit table)
- [x] Risks mitigated (schema propagation, cleanup)

---

**Next Step**: Proceed to DO-RED Phase (write tests FIRST)

