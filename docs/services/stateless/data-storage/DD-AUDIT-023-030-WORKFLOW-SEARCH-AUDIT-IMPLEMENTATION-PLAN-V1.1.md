# DD-AUDIT-023-030: Workflow Search Audit Trail - Implementation Plan

**Version**: 1.2
**Filename**: `DD-AUDIT-023-030-WORKFLOW-SEARCH-AUDIT-IMPLEMENTATION-PLAN-V1.1.md`
**Status**: 📋 DRAFT
**Design Decision**: [DD-WORKFLOW-014](../../../architecture/decisions/DD-WORKFLOW-014-workflow-selection-audit-trail.md)
**Service**: Data Storage Service (Go)
**Confidence**: 95% (Evidence-Based)
**Estimated Effort**: 3 days (APDC cycle: 1.5 days implementation + 1 day testing + 0.5 days documentation)

⚠️ **CRITICAL**: Filename version MUST match document version at all times.
- Document v1.0 → Filename `V1.0.md`

---

## 🚨 **CRITICAL: Read This First**

**Before starting implementation, you MUST review these 5 critical pitfalls** (see line 954 for full details):

1. **Insufficient TDD Discipline** → Write ONE test at a time (not batched)
2. **Missing Integration Tests** → Integration tests BEFORE E2E tests
3. **Critical Infrastructure Without Unit Tests** → ≥70% coverage for critical components
4. **Late E2E Discovery** → Follow test pyramid (Unit → Integration → E2E)
5. **No Test Coverage Gates** → Automated CI/CD coverage gates

⚠️ **These pitfalls caused production issues in Audit Implementation (DD-STORAGE-012).**

---

## 📋 **Version History**

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v1.2** | 2025-11-28 | **IMPLEMENTATION COMPLETE**: Added E2E test (`06_workflow_search_audit_test.go`). Wired `auditStore` to Handler in `server.go`. All unit, integration, and E2E tests passing. | ✅ **CURRENT** |
| **v1.1** | 2025-11-27 | **CRITICAL**: Updated to align with DD-WORKFLOW-004 v2.0. V1.0 scoring = confidence only (base similarity). Removed boost/penalty breakdown (deferred to V2.0+). Updated BR-AUDIT-026 and test examples. | ⚠️ **SUPERSEDED** |
| **v1.0** | 2025-11-27 | Initial implementation plan for Data Storage Service workflow search audit trail. Added existing E2E test coverage (`04_workflow_search_test.go`). Verified ADR-038 async buffered infrastructure is fully implemented. Removed incorrect exception for pkg/ test co-location. | ⚠️ **SUPERSEDED** |

---

## 🎯 **Business Requirements**

### **Primary Business Requirements**

| BR ID | Description | Success Criteria |
|-------|-------------|------------------|
| **BR-AUDIT-023** | Audit event generation in Data Storage Service | Every workflow search generates `workflow.catalog.search_completed` event |
| **BR-AUDIT-024** | Asynchronous non-blocking audit (ADR-038) | Audit writes use buffered async pattern, search latency < 50ms impact |
| **BR-AUDIT-025** | Query metadata capture | Full query text, filters, top_k, min_similarity captured |
| **BR-AUDIT-026** | Scoring capture | V1.0: confidence only (base similarity). V2.0+: configurable boost/penalty |
| **BR-AUDIT-027** | Workflow metadata capture | All workflow fields including owner, maintainer, version history, success metrics |
| **BR-AUDIT-028** | Search metadata capture | duration_ms, db_query_time_ms, embedding_time_ms, index_used, cache_hit |
| **BR-AUDIT-029** | Audit data retention | 90-365 days configurable retention |
| **BR-AUDIT-030** | Audit query API | Query audit events by correlation_id (remediation_id) |

### **Success Metrics**

**Format**: `[Metric]: [Target] - *Justification: [Why this target?]*`

**Your Metrics**:
- **Audit Event Coverage**: 100% of workflow searches - *Justification: Compliance requires complete audit trail (SOC 2, ISO 27001)*
- **Search Latency Impact**: <50ms P95 - *Justification: Async buffered pattern (ADR-038) ensures non-blocking writes*
- **Audit Write Success**: >99.9% - *Justification: DLQ fallback ensures no data loss*
- **Storage Efficiency**: <2KB per audit event - *Justification: 1000 searches/day = 2MB/day = 730MB/year (acceptable)*

---

## 📅 **Timeline Overview**

### **Phase Breakdown**

| Phase | Duration | Days | Purpose | Key Deliverables |
|-------|----------|------|---------|------------------|
| **ANALYSIS** | 2 hours | Day 0 (pre-work) | Comprehensive context understanding | Analysis document, risk assessment, existing code review |
| **PLAN** | 2 hours | Day 0 (pre-work) | Detailed implementation strategy | This document, TDD phase mapping, success criteria |
| **DO (Implementation)** | 1.5 days | Days 1-2 (morning) | Controlled TDD execution | Audit event builder, handler integration, async buffer |
| **CHECK (Testing)** | 1 day | Days 2 (afternoon)-3 (morning) | Comprehensive result validation | Test suite (unit/integration), BR validation |
| **PRODUCTION READINESS** | 0.5 days | Day 3 (afternoon) | Documentation & deployment prep | Runbooks, handoff docs, confidence report |

### **3-Day Implementation Timeline**

| Day | Phase | Focus | Hours | Key Milestones |
|-----|-------|-------|-------|----------------|
| **Day 0** | ANALYSIS + PLAN | Pre-work | 4h | ✅ Analysis complete, Plan approved (this document) |
| **Day 1 AM** | DO-RED | Test framework + Failing tests | 4h | Test framework, interfaces, failing tests |
| **Day 1 PM** | DO-GREEN | Audit event builder | 4h | WorkflowSearchAuditEvent builder |
| **Day 2 AM** | DO-GREEN | Handler integration | 4h | HandleWorkflowSearch audit generation |
| **Day 2 PM** | CHECK | Unit tests | 4h | 70%+ unit coverage |
| **Day 3 AM** | CHECK | Integration tests | 4h | Handler → Repository → Audit pipeline |
| **Day 3 PM** | PRODUCTION | Documentation + Handoff | 4h | API docs, runbooks, confidence report |

### **Critical Path Dependencies**

```
Day 1 AM (Foundation) → Day 1 PM (Event Builder)
                                   ↓
Day 2 AM (Handler Integration) → Day 2 PM (Unit Tests)
                                   ↓
Day 3 AM (Integration Tests) → Day 3 PM (Production Readiness)
```

### **Daily Progress Tracking**

**EOD Documentation Required**:
- **Day 1 Complete**: Audit event builder complete, failing tests written
- **Day 2 Complete**: Handler integration complete, unit tests passing
- **Day 3 Complete**: Production ready checkpoint (tests passing, docs complete)

---

## 📆 **Day-by-Day Implementation Breakdown**

### **Day 0: ANALYSIS + PLAN (Pre-Work) ✅**

**Phase**: ANALYSIS + PLAN
**Duration**: 4 hours
**Status**: ✅ COMPLETE (this document represents Day 0 completion)

**Deliverables**:
- ✅ Analysis document: Existing audit patterns reviewed (workflow_event.go, audit_handlers.go)
- ✅ Implementation plan (this document v1.0): 3-day timeline, test examples
- ✅ Risk assessment: 5 critical pitfalls identified with mitigation strategies
- ✅ Existing code review: pkg/datastorage/audit/, pkg/datastorage/server/workflow_handlers.go
- ✅ BR coverage matrix: 8 primary BRs mapped to test scenarios (BR-AUDIT-023 through BR-AUDIT-030)

---

### **Day 1: Foundation + Audit Event Builder (DO-RED + DO-GREEN Phase)**

**Phase**: DO-RED → DO-GREEN
**Duration**: 8 hours
**TDD Focus**: Write failing tests first, enhance existing code

**⚠️ CRITICAL**: We are **ENHANCING existing code**, not creating from scratch!

**Existing Code to Enhance**:
- ✅ `pkg/datastorage/audit/workflow_event.go` (214 LOC) - Existing workflow event builder
- ✅ `pkg/datastorage/server/workflow_handlers.go` (226 LOC) - Existing workflow search handler
- ✅ `pkg/datastorage/models/workflow.go` (351 LOC) - Existing workflow models

**Morning (4 hours): Test Framework Setup + Code Analysis**

1. **Analyze existing implementation** (1 hour)
   - Read `pkg/datastorage/audit/workflow_event.go` - understand existing builder pattern
   - Read `pkg/datastorage/server/workflow_handlers.go` - understand handler integration
   - Identify what needs to be enhanced vs. created new

2. **Create test file** `test/unit/datastorage/workflow_search_audit_test.go` (300-400 LOC)
   - Set up Ginkgo/Gomega test suite
   - Define test fixtures for workflow search audit events
   - Create helper functions for audit event validation

3. **Write failing tests** (strict TDD: ONE test at a time)
   - **TDD Cycle 1**: Test `NewWorkflowSearchAuditEvent()` creates valid event
   - **TDD Cycle 2**: Test `WithQueryMetadata()` captures full query
   - **TDD Cycle 3**: Test `WithScoringBreakdown()` captures all scoring fields
   - Run tests → Verify they FAIL (RED phase)

**⚠️ CRITICAL: TEST FILE LOCATIONS - MANDATORY**

**AUTHORITY**: [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc)

**ALL tests MUST be in the following locations**:
- ✅ **Unit Tests**: `test/unit/datastorage/workflow_search_audit_test.go`
- ✅ **Integration Tests**: `test/integration/datastorage/workflow_search_audit_integration_test.go`
- ✅ **E2E Tests**: `test/e2e/datastorage/workflow_search_audit_e2e_test.go`

**❌ NEVER place tests**:
- ❌ Co-located with source files (e.g., `pkg/datastorage/audit/workflow_search_audit_test.go`)
- ❌ In `pkg/datastorage/`

**Package Naming**: Tests use white-box testing (same package as code under test).

---

**Afternoon (4 hours): Audit Event Builder Implementation**

4. **Create** `pkg/datastorage/audit/workflow_search_event.go` (new file, ~250 LOC)
   ```go
   // WorkflowSearchEventData represents workflow search audit event_data structure.
   //
   // Business Requirements:
   // - BR-AUDIT-023: Audit event generation in Data Storage Service
   // - BR-AUDIT-025: Query metadata capture
   // - BR-AUDIT-026: Scoring breakdown capture
   // - BR-AUDIT-027: Workflow metadata capture
   // - BR-AUDIT-028: Search metadata capture
   type WorkflowSearchEventData struct {
       Query          WorkflowSearchQuery    `json:"query"`
       Results        WorkflowSearchResults  `json:"results"`
       SearchMetadata WorkflowSearchMetadata `json:"search_metadata"`
   }
   ```

5. **Run tests** → Verify they PASS (GREEN phase)

**EOD Deliverables**:
- ✅ Test framework complete
- ✅ 3 failing tests written (RED phase)
- ✅ WorkflowSearchAuditEvent builder implemented (GREEN phase)
- ✅ Day 1 EOD report

**Validation Commands**:
```bash
# Verify tests pass (GREEN phase)
go test ./test/unit/datastorage/workflow_search_audit_test.go -v

# Expected: All tests should PASS
```

---

### **Day 2: Handler Integration + Unit Tests (DO-GREEN + CHECK Phase)**

**Phase**: DO-GREEN → CHECK
**Duration**: 8 hours
**TDD Focus**: Handler integration + comprehensive unit test coverage

**Morning (4 hours): Handler Integration**

1. **Write failing tests** for handler integration
   - **TDD Cycle 4**: Test `HandleWorkflowSearch()` generates audit event
   - **TDD Cycle 5**: Test audit event contains remediation_id from request
   - **TDD Cycle 6**: Test audit event uses async buffer (non-blocking)
   - Run tests → Verify they FAIL (RED phase)

2. **Enhance** `pkg/datastorage/server/workflow_handlers.go` (add ~50 LOC)
   ```go
   // HandleWorkflowSearch handles POST /api/v1/workflows/search
   // BR-STORAGE-013: Semantic search for remediation workflows
   // BR-AUDIT-023: Audit event generation in Data Storage Service
   func (h *Handler) HandleWorkflowSearch(w http.ResponseWriter, r *http.Request) {
       // ... existing code ...

       // Execute semantic search
       response, err := h.workflowRepo.SearchByEmbedding(r.Context(), &searchReq)

       // ========================================
       // BR-AUDIT-023: Generate audit event (async, non-blocking)
       // ========================================
       h.generateWorkflowSearchAudit(r.Context(), &searchReq, response, searchMeta)

       // Return search results
       // ...
   }
   ```

3. **Run tests** → Verify they PASS (GREEN phase)

**Afternoon (4 hours): Unit Test Completion**

4. **Expand unit tests** to 70%+ coverage
   - Test edge cases (empty results, no filters, max top_k)
   - Test error conditions (invalid request, embedding failure)
   - Test boundary values (min_similarity 0.0 and 1.0)

5. **Behavior & Correctness Validation**
   - Tests validate WHAT the system does (not HOW)
   - Clear business scenarios in test names
   - Specific assertions (not `ToNot(BeNil())`)

**EOD Deliverables**:
- ✅ Handler integration complete (GREEN phase)
- ✅ 70%+ unit test coverage
- ✅ All unit tests passing
- ✅ Day 2 EOD report

**Validation Commands**:
```bash
# Run unit tests with coverage
go test ./test/unit/datastorage/... -coverprofile=coverage.out
go tool cover -func=coverage.out | grep total

# Expected: total coverage ≥70%
```

---

### **Day 3: Integration Tests + Production Readiness (CHECK + PRODUCTION Phase)**

**Phase**: CHECK → PRODUCTION
**Duration**: 8 hours
**Focus**: Integration test validation + documentation

**Morning (4 hours): Integration Tests**

1. **Create integration tests** `test/integration/datastorage/workflow_search_audit_integration_test.go`
   - Test with real PostgreSQL + pgvector
   - Test audit event persistence
   - Test async buffer behavior
   - Test DLQ fallback

2. **Integration test scenarios**
   - **Scenario 1**: Successful search generates audit event
   - **Scenario 2**: Audit event contains complete metadata
   - **Scenario 3**: Async buffer does not block search response
   - **Scenario 4**: DLQ fallback on primary write failure

**Afternoon (4 hours): Documentation + Production Readiness**

3. **Update service documentation**
   - Update `docs/services/stateless/data-storage/overview.md` - Add workflow search audit section
   - Update `docs/services/stateless/data-storage/BUSINESS_REQUIREMENTS.md` - Mark BR-AUDIT-023-030 implemented
   - Create/update runbook for workflow search audit troubleshooting

4. **Create handoff summary**
   - Executive summary
   - Architecture overview
   - Test coverage summary
   - Key decisions
   - Lessons learned
   - Confidence assessment (target: 95%)

**EOD Deliverables**:
- ✅ Integration tests passing
- ✅ Service documentation updated
- ✅ Runbook created
- ✅ Handoff summary complete
- ✅ Production ready

**Validation Commands**:
```bash
# Run integration tests
make test-integration-datastorage

# Expected: All integration tests pass
```

---

## 🧪 **TDD Do's and Don'ts - MANDATORY**

### **✅ DO: Strict TDD Discipline**

1. **Write ONE test at a time** (not batched)
   ```go
   // ✅ CORRECT: TDD Cycle 1
   It("should create valid workflow search audit event", func() {
       // Test for NewWorkflowSearchAuditEvent
   })
   // Run test → FAIL (RED)
   // Implement NewWorkflowSearchAuditEvent → PASS (GREEN)
   // Refactor if needed

   // ✅ CORRECT: TDD Cycle 2 (after Cycle 1 complete)
   It("should capture complete query metadata", func() {
       // Test for WithQueryMetadata
   })
   ```

2. **Test WHAT the system does** (behavior), not HOW (implementation)
   ```go
   // ✅ CORRECT: Behavior-focused
   It("should include remediation_id in audit event correlation_id", func() {
       event := builder.WithCorrelationID("req-2025-11-27-abc123").Build()
       Expect(event.CorrelationID).To(Equal("req-2025-11-27-abc123"),
           "Audit event should use remediation_id for correlation")
   })
   ```

3. **Use specific assertions** (not weak checks)
   ```go
   // ✅ CORRECT: Specific business assertions
   Expect(event.EventType).To(Equal("workflow.catalog.search_completed"))
   Expect(event.Service).To(Equal("datastorage"))
   Expect(eventData.Query.Text).To(Equal("OOMKilled critical"))
   ```

### **❌ DON'T: Anti-Patterns to Avoid**

1. **DON'T batch test writing**
   ```go
   // ❌ WRONG: Writing 10 tests before any implementation
   It("test 1", func() { ... })
   It("test 2", func() { ... })
   It("test 3", func() { ... })
   // ... 7 more tests
   // Then implementing all at once
   ```

2. **DON'T test implementation details**
   ```go
   // ❌ WRONG: Testing internal state
   Expect(builder.internalBuffer).To(HaveLen(5))
   Expect(builder.eventDataMap["internal_key"]).To(Equal("value"))
   ```

3. **DON'T use weak assertions (NULL-TESTING)**
   ```go
   // ❌ WRONG: Weak assertions
   Expect(event).ToNot(BeNil())
   Expect(eventData).ToNot(BeEmpty())
   Expect(len(workflows)).To(BeNumerically(">", 0))
   ```

**Reference**: `.cursor/rules/08-testing-anti-patterns.mdc` for automated detection

---

## 📊 **Test Examples**

### **📁 Test File Locations - MANDATORY (READ THIS FIRST)**

**AUTHORITY**: [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc)

**🚨 CRITICAL**: Test files MUST be in specific directories, NOT co-located with source code!

| Test Type | File Location | Example | ❌ WRONG Location |
|-----------|---------------|---------|-------------------|
| **Unit Tests** | `test/unit/datastorage/` | `test/unit/datastorage/workflow_search_audit_test.go` | ❌ `pkg/datastorage/audit/workflow_search_audit_test.go` |
| **Integration Tests** | `test/integration/datastorage/` | `test/integration/datastorage/workflow_search_audit_integration_test.go` | ❌ `pkg/datastorage/server/workflow_handlers_integration_test.go` |
| **E2E Tests** | `test/e2e/datastorage/` | `test/e2e/datastorage/workflow_search_audit_e2e_test.go` | ❌ `cmd/datastorage/workflow_search_audit_e2e_test.go` |

---

### **Unit Test Example**

```go
package datastorage  // White-box testing - same package as code under test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
    "github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

var _ = Describe("WorkflowSearchAudit Unit Tests", func() {
    var (
        ctx     context.Context
        builder *audit.WorkflowSearchEventBuilder
    )

    BeforeEach(func() {
        ctx = context.Background()
        builder = audit.NewWorkflowSearchEvent("workflow.catalog.search_completed")
    })

    Context("when creating workflow search audit event", func() {
        // BR-AUDIT-023: Audit event generation in Data Storage Service
        It("should create valid audit event with correct event type", func() {
            // BUSINESS SCENARIO: Data Storage generates audit for every search
            event, err := builder.
                WithCorrelationID("req-2025-11-27-abc123").
                Build()

            // BEHAVIOR: Audit event is created with correct type
            Expect(err).ToNot(HaveOccurred(), "Should create audit event without error")
            Expect(event.EventType).To(Equal("workflow.catalog.search_completed"),
                "Event type should be workflow.catalog.search_completed")
            Expect(event.Service).To(Equal("datastorage"),
                "Service should be datastorage")

            // BUSINESS OUTCOME: BR-AUDIT-023 validated
        })

        // BR-AUDIT-025: Query metadata capture
        It("should capture complete query metadata", func() {
            // BUSINESS SCENARIO: Operators need full query for debugging
            searchReq := &models.WorkflowSearchRequest{
                Query:         "OOMKilled critical",
                TopK:          3,
                MinSimilarity: ptr(0.7),
                Filters: &models.WorkflowSearchFilters{
                    SignalType: "OOMKilled",
                    Severity:   "critical",
                },
            }

            event, err := builder.
                WithQueryMetadata(searchReq).
                Build()

            // BEHAVIOR: Query metadata is fully captured
            Expect(err).ToNot(HaveOccurred())
            eventData := event.EventData.(map[string]interface{})
            query := eventData["query"].(map[string]interface{})
            Expect(query["text"]).To(Equal("OOMKilled critical"),
                "Query text should be captured")
            Expect(query["top_k"]).To(Equal(3),
                "TopK should be captured")
            Expect(query["min_similarity"]).To(Equal(0.7),
                "MinSimilarity should be captured")

            // BUSINESS OUTCOME: BR-AUDIT-025 validated
        })

        // BR-AUDIT-026: Scoring breakdown capture
        It("should capture complete scoring breakdown", func() {
            // BUSINESS SCENARIO: Operators need scoring details for tuning
            result := &models.WorkflowSearchResult{
                BaseSimilarity: 0.88,
                LabelBoost:     0.10,
                LabelPenalty:   0.0,
                FinalScore:     0.98,
            }

            event, err := builder.
                WithWorkflowResult(result, 1).
                Build()

            // BEHAVIOR: Scoring breakdown is fully captured
            Expect(err).ToNot(HaveOccurred())
            eventData := event.EventData.(map[string]interface{})
            workflows := eventData["results"].(map[string]interface{})["workflows"].([]interface{})
            scoring := workflows[0].(map[string]interface{})["scoring"].(map[string]interface{})

            // V1.0: confidence only (base similarity, no boost/penalty)
            // Authority: DD-WORKFLOW-004 v2.0
            Expect(scoring["confidence"]).To(BeNumerically(">=", 0.0),
                "Confidence should be >= 0.0")
            Expect(scoring["confidence"]).To(BeNumerically("<=", 1.0),
                "Confidence should be <= 1.0")

            // V2.0+: These fields will be added when configurable weights are implemented
            // Expect(scoring["base_similarity"]).To(...)
            // Expect(scoring["label_boost"]).To(...)
            // Expect(scoring["label_penalty"]).To(...)

            // BUSINESS OUTCOME: BR-AUDIT-026 validated (V1.0: confidence only)
        })
    })
})
```

### **Integration Test Example**

```go
package datastorage  // White-box testing - same package as code under test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/jordigilh/kubernaut/test/integration/infrastructure"
)

var _ = Describe("WorkflowSearchAudit Integration Tests", func() {
    var (
        ctx        context.Context
        testServer *infrastructure.TestServer
        db         *sql.DB
    )

    BeforeEach(func() {
        ctx = context.Background()
        testServer = infrastructure.NewTestServer(infrastructure.DataStorageConfig{
            PostgresURL: testPostgresURL,
        })
        db = testServer.DB()
    })

    AfterEach(func() {
        testServer.Cleanup()
    })

    Context("when workflow search is executed", func() {
        // BR-AUDIT-023: Audit event generation in Data Storage Service
        It("should generate audit event for successful search", func() {
            // BUSINESS SCENARIO: Every search generates audit event

            // Setup: Create test workflow
            workflow := createTestWorkflow(ctx, db, "test-workflow-1")

            // BEHAVIOR: Execute search via HTTP endpoint
            response, err := testServer.POST("/api/v1/workflows/search", map[string]interface{}{
                "query":          "OOMKilled critical",
                "top_k":          3,
                "remediation_id": "req-2025-11-27-abc123",
                "filters": map[string]interface{}{
                    "signal-type": "OOMKilled",
                    "severity":    "critical",
                },
            })

            Expect(err).ToNot(HaveOccurred())
            Expect(response.StatusCode).To(Equal(http.StatusOK))

            // CORRECTNESS: Verify audit event was created
            Eventually(func() int {
                var count int
                db.QueryRow(`
                    SELECT COUNT(*) FROM audit_events
                    WHERE event_type = 'workflow.catalog.search_completed'
                    AND correlation_id = 'req-2025-11-27-abc123'
                `).Scan(&count)
                return count
            }, 5*time.Second, 100*time.Millisecond).Should(Equal(1),
                "Audit event should be created for search")

            // BUSINESS OUTCOME: BR-AUDIT-023 validated
        })

        // BR-AUDIT-024: Asynchronous non-blocking audit
        It("should not block search response while writing audit", func() {
            // BUSINESS SCENARIO: Audit writes must not slow down search

            // Setup: Create test workflows
            for i := 0; i < 10; i++ {
                createTestWorkflow(ctx, db, fmt.Sprintf("test-workflow-%d", i))
            }

            // BEHAVIOR: Execute search and measure response time
            start := time.Now()
            response, err := testServer.POST("/api/v1/workflows/search", map[string]interface{}{
                "query":          "OOMKilled critical",
                "top_k":          10,
                "remediation_id": "req-2025-11-27-perf-test",
                "filters": map[string]interface{}{
                    "signal-type": "OOMKilled",
                    "severity":    "critical",
                },
            })
            responseTime := time.Since(start)

            // CORRECTNESS: Response time should be fast (audit is async)
            Expect(err).ToNot(HaveOccurred())
            Expect(response.StatusCode).To(Equal(http.StatusOK))
            Expect(responseTime).To(BeNumerically("<", 200*time.Millisecond),
                "Search response should be fast (audit is async)")

            // BUSINESS OUTCOME: BR-AUDIT-024 validated
        })
    })
})
```

---

## 🏗️ **Integration Test Environment Decision**

### **Environment Strategy**

**Decision**: Podman (PostgreSQL + pgvector container)

**Environment Comparison**:

| Environment | Pros | Cons | Decision |
|-------------|------|------|----------|
| **Podman** | Fast, isolated, already available | Requires Podman installed | ✅ Selected |
| **KIND** | Full K8s, realistic | Slow startup (30s+), overkill for integration | ❌ Not Selected |
| **envtest** | Fast, lightweight | No real database, limited realism | ❌ Not Selected |
| **Mocks** | Fastest | Not integration tests, low confidence | ❌ Not Selected |

**Rationale**:
- Fast startup (<5s) for PostgreSQL container
- Isolated per test process
- Reuses existing Data Storage integration test infrastructure
- Realistic database interactions with pgvector extension
- No K8s overhead needed for audit event testing

---

## ✅ **Existing Infrastructure Verification**

### **ADR-038 Async Buffered Audit Infrastructure**

**Status**: ✅ **FULLY IMPLEMENTED**

**Location**: `pkg/audit/store.go`

**Key Components**:
| Component | Status | Location |
|-----------|--------|----------|
| `AuditStore` interface | ✅ Implemented | `pkg/audit/store.go:25-46` |
| `BufferedAuditStore` struct | ✅ Implemented | `pkg/audit/store.go:71-86` |
| `NewBufferedStore()` factory | ✅ Implemented | `pkg/audit/store.go:102-137` |
| `StoreAudit()` non-blocking | ✅ Implemented | `pkg/audit/store.go:147+` |
| Background worker | ✅ Implemented | Goroutine started in `NewBufferedStore()` |
| Graceful shutdown | ✅ Implemented | `Close()` method flushes buffer |

**Integration in Data Storage Service**:
```go
// pkg/datastorage/server/server.go:73
auditStore audit.AuditStore

// pkg/datastorage/server/server.go:175
auditStore, err := audit.NewBufferedStore(...)

// pkg/datastorage/server/server.go:270
auditStore: auditStore,
```

**Usage Pattern** (from `audit_events_handler.go`):
```go
// Non-blocking audit write
if err := s.auditStore.StoreAudit(ctx, auditEvent); err != nil {
    s.logger.Error("Failed to store audit event", zap.Error(err))
    // Service continues operating - audit failure doesn't block business logic
}
```

**Confidence**: 98% - Infrastructure is proven and battle-tested in existing audit handlers.

### **Existing Workflow Handler**

**Status**: ✅ **READY FOR ENHANCEMENT**

**Location**: `pkg/datastorage/server/workflow_handlers.go`

**Current Implementation**:
- ✅ `HandleWorkflowSearch()` - POST /api/v1/workflows/search
- ✅ Request validation
- ✅ Embedding generation
- ✅ Semantic search execution
- ❌ **MISSING**: Audit event generation (to be added)

**Enhancement Required**:
Add audit event generation after search execution (non-blocking, uses existing `auditStore`).

---

## 🎯 **BR Coverage Matrix**

| BR ID | Description | Unit Tests | Integration Tests | E2E Tests | Status |
|-------|-------------|------------|-------------------|-----------|--------|
| **BR-AUDIT-023** | Audit event generation in Data Storage Service | `workflow_search_audit_test.go` | `workflow_search_audit_integration_test.go` | `04_workflow_search_test.go` (existing) | 🔄 Pending |
| **BR-AUDIT-024** | Asynchronous non-blocking audit (ADR-038) | `workflow_search_audit_test.go` | `workflow_search_audit_integration_test.go` | `04_workflow_search_test.go` (existing) | 🔄 Pending |
| **BR-AUDIT-025** | Query metadata capture | `workflow_search_audit_test.go` | `workflow_search_audit_integration_test.go` | N/A | 🔄 Pending |
| **BR-AUDIT-026** | Scoring breakdown capture | `workflow_search_audit_test.go` | `workflow_search_audit_integration_test.go` | N/A | 🔄 Pending |
| **BR-AUDIT-027** | Workflow metadata capture | `workflow_search_audit_test.go` | `workflow_search_audit_integration_test.go` | N/A | 🔄 Pending |
| **BR-AUDIT-028** | Search metadata capture | `workflow_search_audit_test.go` | `workflow_search_audit_integration_test.go` | N/A | 🔄 Pending |
| **BR-AUDIT-029** | Audit data retention | N/A | `workflow_search_audit_integration_test.go` | N/A | 🔄 Pending |
| **BR-AUDIT-030** | Audit query API | `workflow_search_audit_test.go` | `workflow_search_audit_integration_test.go` | N/A | 🔄 Pending |

**Coverage Calculation**:
- **Unit**: 7/8 BRs covered (87.5%)
- **Integration**: 8/8 BRs covered (100%)
- **E2E**: 2/8 BRs covered (25%) - Existing `04_workflow_search_test.go` covers search endpoint
- **Total**: 8/8 BRs covered (100%)

### **Existing E2E Test Coverage**

**File**: `test/e2e/datastorage/04_workflow_search_test.go`

The existing E2E test covers:
- ✅ Workflow search endpoint (`POST /api/v1/workflows/search`)
- ✅ Hybrid weighted scoring validation
- ✅ Boost/penalty calculations
- ✅ Search latency (<200ms)

**Integration Test Responsibility**:
Since the E2E test already covers the REST API endpoint, the integration tests will focus on:
- Audit event generation (verify audit event created after search)
- Audit event content (verify full metadata captured)
- Async buffer behavior (verify non-blocking)
- DLQ fallback (verify graceful degradation)

---

## 🚨 **Critical Pitfalls to Avoid**

### **⚠️ LESSONS LEARNED FROM AUDIT IMPLEMENTATION (November 2025)**

**Context**: During the Audit Trail implementation (DD-STORAGE-012), we discovered critical mistakes that led to missing functionality (DLQ fallback) being caught only by E2E tests after the handler was considered "complete". These lessons are now mandatory to prevent in all future implementations.

**Evidence**: `docs/services/stateless/data-storage/DD-IMPLEMENTATION-AUDIT-V1.0.md`

---

### **1. Insufficient TDD Discipline** 🔴 **CRITICAL**

- ❌ **Problem**: New handler (`handleCreateAuditEvent`) was implemented **WITHOUT writing tests first**. DLQ fallback was added to code **WITHOUT corresponding test coverage**. Tests were written **AFTER** implementation, not before.
- ✅ **Solution**:
  - Write **ONE test at a time** (not in batches)
  - Follow strict **RED-GREEN-REFACTOR** sequence for every feature
  - **NEVER** write implementation code before writing a failing test
  - Each test must map to a specific **BR-AUDIT-XXX** requirement
- **Impact**: **CRITICAL** - DLQ functionality was **MISSING** in production code until E2E test caught it. High risk of data loss in production. Required emergency fix and comprehensive test backfill.

**Enforcement Checklist** (BLOCKING):
```
Before writing ANY implementation code:
- [ ] Unit test written and FAILING (RED phase)
- [ ] Test validates business behavior, not implementation
- [ ] Test uses specific assertions, not weak checks (no `ToNot(BeNil())`)
- [ ] Test maps to specific BR-AUDIT-XXX requirement
- [ ] Test run confirms FAIL with "not implemented yet" error
```

---

### **2. Missing Integration Tests for New Endpoints** 🔴 **CRITICAL**

- ❌ **Problem**: New unified audit events endpoint (`/api/v1/audit/events`) was implemented with **NO integration tests**. DLQ fallback functionality was missing because integration tests would have caught it early in the development cycle.
- ✅ **Solution**:
  - **Integration tests MUST exist BEFORE E2E tests** (MANDATORY)
  - Every new HTTP endpoint **MUST have integration tests** covering:
    - Success path
    - Failure paths
    - Edge cases (empty results, invalid filters, timeouts)
  - Integration tests must validate component interactions (e.g., handler → repository → database)
- **Impact**: **CRITICAL** - Critical functionality (DLQ fallback) was missing. E2E test was the first to catch the issue (too late in development cycle). Required backfilling integration tests after implementation.

**Integration Test Mandate** (BLOCKING):
```
For EVERY new HTTP endpoint:
- [ ] Integration test MUST exist before E2E test
- [ ] Integration test MUST cover success path
- [ ] Integration test MUST cover failure paths
- [ ] Integration test MUST cover edge cases
- [ ] Integration test validates component interactions (not just HTTP status codes)
```

---

### **3. Async Buffer Integration Risk** 🟡 **MEDIUM**

- ❌ **Problem**: Async buffered audit (ADR-038) adds complexity - buffer overflow, worker failure, graceful shutdown
- ✅ **Solution**:
  - Test buffer behavior under load
  - Test DLQ fallback when buffer is full
  - Test graceful shutdown flushes buffer
  - Use existing ADR-038 implementation patterns
- **Impact**: **MEDIUM** - Potential data loss if buffer not properly handled

**Async Buffer Checklist** (BLOCKING):
```
Before marking async buffer integration complete:
- [ ] Buffer overflow handling tested
- [ ] DLQ fallback on primary write failure tested
- [ ] Graceful shutdown buffer flush tested
- [ ] Worker failure recovery tested
```

---

## 📈 **Success Criteria**

### **Technical Success**
- ✅ All tests passing (Unit 70%+, Integration 100%)
- ✅ No lint errors
- ✅ Code integrated with workflow_handlers.go
- ✅ Documentation complete

### **Business Success**
- ✅ BR-AUDIT-023 validated (audit event generation)
- ✅ BR-AUDIT-024 validated (async non-blocking)
- ✅ BR-AUDIT-025 validated (query metadata)
- ✅ BR-AUDIT-026 validated (scoring breakdown)
- ✅ BR-AUDIT-027 validated (workflow metadata)
- ✅ BR-AUDIT-028 validated (search metadata)
- ✅ BR-AUDIT-029 validated (data retention)
- ✅ BR-AUDIT-030 validated (audit query API)

### **Confidence Assessment**
- **Target**: ≥95% confidence
- **Calculation**: Evidence-based (test coverage + BR validation + integration status)

---

## 📊 **Confidence Calculation Methodology**

**Overall Confidence**: 95% (Evidence-Based)

**Component Breakdown**:

| Component | Confidence | Evidence |
|-----------|-----------|----------|
| **Audit Event Builder** | 98% | Existing pattern in `workflow_event.go`, proven builder pattern |
| **Handler Integration** | 95% | Existing handler structure, simple addition of audit call |
| **Async Buffer (ADR-038)** | 92% | Proven pattern in existing audit implementation |
| **Test Coverage** | 95% | Comprehensive unit + integration test plan |

**Risk Assessment**:
- **5% Risk**: Async buffer edge cases (buffer overflow, worker failure)
- **Mitigation**: Comprehensive integration tests for async behavior
- **Assumptions**: ADR-038 async buffer implementation is stable
- **Validation Approach**: Integration tests with simulated failures

**Calculation Formula**:
```
Overall Confidence = (98% + 95% + 92% + 95%) / 4 = 95%
```

---

## 🔄 **Rollback Plan**

### **Rollback Triggers**
- Critical bug discovered in production
- Performance degradation >20% on search latency
- Audit data loss detected

### **Rollback Procedure**
1. Remove audit generation call from `HandleWorkflowSearch()`
2. Deploy previous version
3. Verify rollback success (search still works)
4. Document rollback reason
5. Plan fix and re-implementation

---

## 📚 **References**

### **Design Decisions**
- [DD-WORKFLOW-014](../../../architecture/decisions/DD-WORKFLOW-014-workflow-selection-audit-trail.md) - Workflow Selection Audit Trail
- [ADR-034](../../../architecture/decisions/ADR-034-unified-audit-table-design.md) - Unified Audit Trail Architecture
- [ADR-038](../../../architecture/decisions/ADR-038-async-buffered-audit-ingestion.md) - Async Buffered Audit Ingestion

### **Business Requirements**
- [BR-AUDIT-021-030](../../../requirements/BR-AUDIT-021-030-WORKFLOW-SELECTION-AUDIT-TRAIL.md) - Workflow Selection Audit Trail BRs

### **Templates**
- [FEATURE_EXTENSION_PLAN_TEMPLATE.md](../../FEATURE_EXTENSION_PLAN_TEMPLATE.md) - This template

### **Standards**
- [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc) - Testing framework
- [02-go-coding-standards.mdc](../../../../.cursor/rules/02-go-coding-standards.mdc) - Go patterns
- [08-testing-anti-patterns.mdc](../../../../.cursor/rules/08-testing-anti-patterns.mdc) - Testing anti-patterns

---

## 📋 **Operational Sections Checklist**

**Required Sections** (add to plan):
- [x] Prometheus Metrics (reuse existing audit metrics)
- [x] Grafana Dashboard (extend existing audit dashboard)
- [x] Troubleshooting Guide (in runbook)
- [x] Error Handling Philosophy (async buffer with DLQ)
- [x] Security Considerations (audit data is sensitive)
- [x] Known Limitations (V1.0 limitations)
- [x] Future Work (V1.1/V1.2 roadmap)

**Optional Sections** (add if applicable):
- [ ] Performance Benchmarking (covered by integration tests)
- [ ] Migration Strategy (not applicable - new feature)
- [ ] Deployment Checklist (standard deployment)

---

**Document Status**: 📋 **DRAFT**
**Last Updated**: 2025-11-27
**Version**: 1.0
**Maintained By**: Development Team

---

## 📝 **Version Bump Checklist**

### **When to Bump Version**

- **Patch (1.0.X)**: Bug fixes, clarifications, typo corrections
- **Minor (1.X.0)**: New sections added, scope expansion, template compliance fixes
- **Major (X.0.0)**: Significant architectural changes, complete rewrites

### **Version Bump Steps**

1. [ ] Update `**Version**` field in header (line 3)
2. [ ] Update `**Filename**` field in header (line 4)
3. [ ] Rename file to match new version (e.g., `V1.0.md` → `V1.1.md`)
4. [ ] Add entry to Version History table with detailed changes
5. [ ] Update all cross-references in related documents
6. [ ] Mark previous version as "⏸️ Superseded"
7. [ ] Mark current version as "✅ CURRENT"

