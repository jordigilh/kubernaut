# DD-STORAGE-012: Data Storage Audit Integration - Implementation Plan

**Version**: 1.0
**Status**: 📋 DRAFT
**Design Decision**: DD-STORAGE-012 (Self-Auditing Pattern)
**Service**: Data Storage Service
**Confidence**: 85% (Evidence-Based)
**Estimated Effort**: 2 days (APDC cycle: 1 day implementation + 0.5 days testing + 0.5 days documentation)

---

## 📋 **Version History**

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v1.0** | 2025-11-20 | Initial implementation plan created | ✅ **CURRENT** |

---

## 🎯 **Business Requirements**

### **Primary Business Requirements**

| BR ID | Description | Success Criteria |
|-------|-------------|------------------|
| **BR-STORAGE-012** | Data Storage Service must generate audit traces for its own operations (self-auditing) | All write operations (success/failure/DLQ) generate audit events |
| **BR-STORAGE-013** | Audit traces must not create circular dependencies (cannot call REST API) | Uses `InternalAuditClient` with direct PostgreSQL writes |
| **BR-STORAGE-014** | Audit writes must not block business operations | Uses asynchronous buffered writes via `pkg/audit/` |

### **Success Metrics**

- **Audit Coverage**: 100% of write operations (success, failure, DLQ fallback)
- **Performance Impact**: <5ms latency overhead per write operation
- **Reliability**: 99.9% audit event capture rate (with DLQ fallback)

---

## 📅 **Timeline Overview**

### **Phase Breakdown**

| Phase | Duration | Days | Purpose | Key Deliverables |
|-------|----------|------|---------|------------------|
| **ANALYSIS** | 2 hours | Day 0 (pre-work) | Comprehensive context understanding | Analysis document, risk assessment, existing code review |
| **PLAN** | 2 hours | Day 0 (pre-work) | Detailed implementation strategy | This document, TDD phase mapping, success criteria |
| **DO (Implementation)** | 1 day | Day 1 | Controlled TDD execution | `InternalAuditClient`, audit integration, metrics |
| **CHECK (Testing)** | 0.5 days | Day 2 (morning) | Comprehensive result validation | Unit tests (70%+), integration tests |
| **PRODUCTION READINESS** | 0.5 days | Day 2 (afternoon) | Documentation & deployment prep | Runbooks, handoff docs, confidence report |

### **2-Day Implementation Timeline**

| Day | Phase | Focus | Hours | Key Milestones |
|-----|-------|-------|-------|----------------|
| **Day 0** | ANALYSIS + PLAN | Pre-work | 4h | ✅ Analysis complete, Plan approved (this document) |
| **Day 1** | DO-RED → DO-GREEN → DO-REFACTOR | Implementation | 8h | `InternalAuditClient`, audit integration, tests passing |
| **Day 2 (AM)** | CHECK | Testing | 4h | Unit tests (70%+), integration tests passing |
| **Day 2 (PM)** | PRODUCTION | Documentation | 4h | Runbooks, metrics dashboard, handoff summary |

### **Critical Path Dependencies**

```
Day 0 (Analysis + Plan) → Day 1 (Implementation) → Day 2 (Testing + Documentation)
                                                   ↓
                                    pkg/audit/ library (✅ Already Complete)
```

### **Daily Progress Tracking**

**EOD Documentation Required**:
- **Day 0 Complete**: Analysis and plan approved (this document)
- **Day 1 Complete**: Implementation complete, all tests passing
- **Day 2 Complete**: Production ready, handoff summary delivered

---

## 📆 **Day-by-Day Implementation Breakdown**

### **Day 0: ANALYSIS + PLAN (Pre-Work) ✅**

**Phase**: ANALYSIS + PLAN
**Duration**: 4 hours
**Status**: ✅ COMPLETE (this document represents Day 0 completion)

**Deliverables**:
- ✅ Analysis document: Circular dependency problem identified, `InternalAuditClient` solution validated
- ✅ Implementation plan (this document v1.0): 2-day timeline, test examples
- ✅ Risk assessment: 3 critical pitfalls identified with mitigation strategies
- ✅ Existing code review: `pkg/audit/store.go`, `pkg/datastorage/server/audit_events_handler.go`
- ✅ BR coverage matrix: 3 primary BRs mapped to test scenarios

**Key Analysis Findings**:

1. **Circular Dependency Problem** (from DD-AUDIT-002):
   - Data Storage Service cannot call its own REST API to audit operations
   - Solution: `InternalAuditClient` writes directly to PostgreSQL (bypasses HTTP)

2. **Existing Infrastructure** (from `pkg/audit/`):
   - ✅ `BufferedAuditStore` already implemented (asynchronous, buffered writes)
   - ✅ `AuditEvent` model already defined (ADR-034 compliant)
   - ✅ Metrics already implemented (`audit_events_buffered_total`, etc.)
   - ⏸️ `InternalAuditClient` NOT yet implemented (new work)

3. **Integration Points** (from `pkg/datastorage/server/`):
   - ✅ `audit_events_handler.go` - Unified audit events endpoint (already has DLQ fallback)
   - ✅ `server.go` - Server initialization (need to add audit store)
   - ⏸️ Audit calls NOT yet added to write operations

4. **Audit Points Identified**:
   - **Success**: After successful write to `audit_events` table
   - **Failure**: After write failure, before DLQ fallback
   - **DLQ Fallback**: When event goes to DLQ (Redis)

---

### **Day 1: Implementation (DO-RED → DO-GREEN → DO-REFACTOR)**

**Phase**: DO (RED → GREEN → REFACTOR)
**Duration**: 8 hours
**TDD Focus**: Write tests first, implement minimal logic, then enhance

**⚠️ CRITICAL**: We are **ENHANCING existing code**, not creating from scratch!

**Existing Code to Enhance**:
- ✅ `pkg/audit/store.go` (300 LOC) - Add `InternalAuditClient` interface implementation
- ✅ `pkg/datastorage/server/audit_events_handler.go` (400 LOC) - Add audit calls
- ✅ `pkg/datastorage/server/server.go` (500 LOC) - Initialize audit store

---

#### **Morning (4 hours): Foundation + InternalAuditClient (DO-RED + DO-GREEN)**

**Hour 1-2: Create `InternalAuditClient` (DO-RED)**

1. **Create test file** `pkg/audit/internal_client_test.go` (150 LOC)
   ```go
   package audit

   import (
       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
       "github.com/DATA-DOG/go-sqlmock"
   )

   var _ = Describe("InternalAuditClient", func() {
       var (
           ctx    context.Context
           client DataStorageClient
           db     *sql.DB
           mock   sqlmock.Sqlmock
       )

       BeforeEach(func() {
           ctx = context.Background()
           db, mock, _ = sqlmock.New()
           client = NewInternalAuditClient(db)
       })

       Context("when storing audit batch", func() {
           It("should write directly to PostgreSQL without REST API", func() {
               // BR-STORAGE-013: Must not create circular dependency
               events := []*AuditEvent{
                   {
                       EventType:     "datastorage.audit.written",
                       EventCategory: "storage",
                       EventAction:   "written",
                       EventOutcome:  "success",
                       CorrelationID: "test-correlation-id",
                       // ... other required fields
                   },
               }

               // Expect direct SQL INSERT (not HTTP call)
               mock.ExpectExec("INSERT INTO audit_events").
                   WillReturnResult(sqlmock.NewResult(1, 1))

               err := client.StoreBatch(ctx, events)

               Expect(err).ToNot(HaveOccurred())
               Expect(mock.ExpectationsWereMet()).To(Succeed())
           })

           It("should handle batch write failures gracefully", func() {
               // BR-STORAGE-014: Must not block business operations
               events := []*AuditEvent{/* ... */}

               mock.ExpectExec("INSERT INTO audit_events").
                   WillReturnError(fmt.Errorf("database connection lost"))

               err := client.StoreBatch(ctx, events)

               Expect(err).To(HaveOccurred())
               Expect(err.Error()).To(ContainSubstring("database connection lost"))
           })
       })
   })
   ```

2. **Run tests** → Verify they FAIL (RED phase)
   ```bash
   go test ./pkg/audit/internal_client_test.go -v 2>&1 | grep "FAIL"
   # Expected: "undefined: NewInternalAuditClient"
   ```

**Hour 3-4: Implement `InternalAuditClient` (DO-GREEN)**

3. **Create** `pkg/audit/internal_client.go` (~100 LOC)
   ```go
   package audit

   import (
       "context"
       "database/sql"
       "fmt"
       "time"
   )

   // InternalAuditClient writes audit events directly to PostgreSQL
   // Used by Data Storage Service to avoid circular dependency (cannot call its own REST API)
   type InternalAuditClient struct {
       db *sql.DB
   }

   // NewInternalAuditClient creates a new internal audit client
   func NewInternalAuditClient(db *sql.DB) DataStorageClient {
       return &InternalAuditClient{db: db}
   }

   // StoreBatch writes audit events directly to PostgreSQL (bypasses REST API)
   func (c *InternalAuditClient) StoreBatch(ctx context.Context, events []*AuditEvent) error {
       if len(events) == 0 {
           return nil
       }

       // Begin transaction
       tx, err := c.db.BeginTx(ctx, nil)
       if err != nil {
           return fmt.Errorf("failed to begin transaction: %w", err)
       }
       defer tx.Rollback()

       // Prepare INSERT statement (batch insert for performance)
       stmt, err := tx.PrepareContext(ctx, `
           INSERT INTO audit_events (
               event_id, event_version, event_timestamp, event_type, event_category,
               event_action, event_outcome, actor_type, actor_id, resource_type,
               resource_id, correlation_id, event_data, retention_days, is_sensitive
           ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
       `)
       if err != nil {
           return fmt.Errorf("failed to prepare statement: %w", err)
       }
       defer stmt.Close()

       // Insert each event
       for _, event := range events {
           _, err := stmt.ExecContext(ctx,
               event.EventID,
               event.EventVersion,
               event.EventTimestamp,
               event.EventType,
               event.EventCategory,
               event.EventAction,
               event.EventOutcome,
               event.ActorType,
               event.ActorID,
               event.ResourceType,
               event.ResourceID,
               event.CorrelationID,
               event.EventData,
               event.RetentionDays,
               event.IsSensitive,
           )
           if err != nil {
               return fmt.Errorf("failed to insert audit event: %w", err)
           }
       }

       // Commit transaction
       if err := tx.Commit(); err != nil {
           return fmt.Errorf("failed to commit transaction: %w", err)
       }

       return nil
   }
   ```

4. **Run tests** → Verify they PASS (GREEN phase)
   ```bash
   go test ./pkg/audit/internal_client_test.go -v
   # Expected: All tests PASS
   ```

---

#### **Afternoon (4 hours): Audit Integration (DO-GREEN + DO-REFACTOR)**

**Hour 5-6: Integrate Audit Store in Server (DO-GREEN)**

5. **Enhance** `pkg/datastorage/server/server.go` (~20 LOC added)
   ```go
   // Add to Server struct
   type Server struct {
       // ... existing fields ...
       auditStore audit.AuditStore  // NEW: Audit store for self-auditing
   }

   // Update NewServer to initialize audit store
   func NewServer(config *Config, logger *slog.Logger) (*Server, error) {
       // ... existing initialization ...

       // Initialize internal audit client (bypasses REST API)
       internalClient := audit.NewInternalAuditClient(db)
       auditStore := audit.NewBufferedStore(
           internalClient,
           audit.DefaultConfig(),
           logger.With(slog.String("component", "audit")),
       )

       return &Server{
           // ... existing fields ...
           auditStore: auditStore,
       }, nil
   }

   // Update Shutdown to flush audit events
   func (s *Server) Shutdown(ctx context.Context) error {
       // ... existing shutdown logic ...

       // Flush remaining audit events
       if err := s.auditStore.Close(); err != nil {
           s.logger.Error("Failed to flush audit events", slog.Any("error", err))
       }

       return nil
   }
   ```

6. **Write integration test** `test/integration/datastorage/audit_integration_test.go` (~200 LOC)
   ```go
   package datastorage

   import (
       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
   )

   var _ = Describe("Data Storage Audit Integration", func() {
       Context("when server initializes", func() {
           It("should create audit store with internal client", func() {
               // BR-STORAGE-013: Must use InternalAuditClient (not REST API)
               server, err := NewServer(testConfig, testLogger)
               Expect(err).ToNot(HaveOccurred())
               Expect(server.auditStore).ToNot(BeNil())
           })
       })

       Context("when server shuts down", func() {
           It("should flush remaining audit events", func() {
               // BR-STORAGE-014: Must not lose audit events during shutdown
               server, _ := NewServer(testConfig, testLogger)

               // Simulate audit events in buffer
               // ... add events ...

               err := server.Shutdown(context.Background())
               Expect(err).ToNot(HaveOccurred())

               // Verify events were flushed to database
               // ... query audit_events table ...
           })
       })
   })
   ```

**Hour 7-8: Add Audit Calls to Write Operations (DO-REFACTOR)**

7. **Enhance** `pkg/datastorage/server/audit_events_handler.go` (~30 LOC added)
   ```go
   func (s *Server) handleCreateAuditEvent(w http.ResponseWriter, r *http.Request) {
       // ... existing validation logic ...

       // Attempt to write to PostgreSQL
       if err := s.repository.StoreAuditEvent(ctx, &repoEvent); err != nil {
           s.logger.Error("Failed to store audit event", slog.Any("error", err))

           // 🆕 AUDIT: Write failure (before DLQ fallback)
           s.auditWriteFailure(ctx, eventMap, err)

           // Try DLQ fallback
           if dlqErr := s.dlqClient.EnqueueAuditEvent(ctx, &modelEvent); dlqErr != nil {
               s.logger.Error("DLQ fallback failed", slog.Any("error", dlqErr))
               s.respondWithError(w, http.StatusServiceUnavailable, "Storage and DLQ unavailable")
               return
           }

           // 🆕 AUDIT: DLQ fallback success
           s.auditDLQFallback(ctx, eventMap)

           w.WriteHeader(http.StatusAccepted) // 202 Accepted
           return
       }

       // 🆕 AUDIT: Write success
       s.auditWriteSuccess(ctx, eventMap)

       w.WriteHeader(http.StatusCreated) // 201 Created
   }

   // 🆕 NEW: Audit helper functions
   func (s *Server) auditWriteSuccess(ctx context.Context, eventMap map[string]interface{}) {
       auditEvent := audit.NewAuditEvent()
       auditEvent.EventType = "datastorage.audit.written"
       auditEvent.EventCategory = "storage"
       auditEvent.EventAction = "written"
       auditEvent.EventOutcome = "success"
       auditEvent.ActorType = "service"
       auditEvent.ActorID = "datastorage"
       auditEvent.ResourceType = "AuditEvent"
       auditEvent.ResourceID = eventMap["event_id"].(string)
       auditEvent.CorrelationID = eventMap["correlation_id"].(string)
       auditEvent.EventData, _ = json.Marshal(map[string]interface{}{
           "version":   "1.0",
           "service":   "datastorage",
           "operation": "audit_written",
           "status":    "success",
           "payload": map[string]interface{}{
               "event_type": eventMap["event_type"],
               "actor_id":   eventMap["actor_id"],
           },
       })

       // Non-blocking audit (async buffered)
       if err := s.auditStore.StoreAudit(ctx, auditEvent); err != nil {
           s.logger.Warn("Failed to audit write success", slog.Any("error", err))
       }
   }

   func (s *Server) auditWriteFailure(ctx context.Context, eventMap map[string]interface{}, writeErr error) {
       auditEvent := audit.NewAuditEvent()
       auditEvent.EventType = "datastorage.audit.failed"
       auditEvent.EventCategory = "storage"
       auditEvent.EventAction = "write_failed"
       auditEvent.EventOutcome = "failure"
       auditEvent.ActorType = "service"
       auditEvent.ActorID = "datastorage"
       auditEvent.ResourceType = "AuditEvent"
       auditEvent.ResourceID = eventMap["event_id"].(string)
       auditEvent.CorrelationID = eventMap["correlation_id"].(string)
       auditEvent.ErrorMessage = &writeErr.Error()
       auditEvent.EventData, _ = json.Marshal(map[string]interface{}{
           "version":   "1.0",
           "service":   "datastorage",
           "operation": "audit_write_failed",
           "status":    "failure",
           "payload": map[string]interface{}{
               "event_type": eventMap["event_type"],
               "error":      writeErr.Error(),
           },
       })

       if err := s.auditStore.StoreAudit(ctx, auditEvent); err != nil {
           s.logger.Warn("Failed to audit write failure", slog.Any("error", err))
       }
   }

   func (s *Server) auditDLQFallback(ctx context.Context, eventMap map[string]interface{}) {
       auditEvent := audit.NewAuditEvent()
       auditEvent.EventType = "datastorage.dlq.fallback"
       auditEvent.EventCategory = "storage"
       auditEvent.EventAction = "dlq_fallback"
       auditEvent.EventOutcome = "success"
       auditEvent.ActorType = "service"
       auditEvent.ActorID = "datastorage"
       auditEvent.ResourceType = "AuditEvent"
       auditEvent.ResourceID = eventMap["event_id"].(string)
       auditEvent.CorrelationID = eventMap["correlation_id"].(string)
       auditEvent.EventData, _ = json.Marshal(map[string]interface{}{
           "version":   "1.0",
           "service":   "datastorage",
           "operation": "dlq_fallback",
           "status":    "success",
           "payload": map[string]interface{}{
               "event_type": eventMap["event_type"],
               "reason":     "postgresql_unavailable",
           },
       })

       if err := s.auditStore.StoreAudit(ctx, auditEvent); err != nil {
           s.logger.Warn("Failed to audit DLQ fallback", slog.Any("error", err))
       }
   }
   ```

8. **Write unit tests** `pkg/datastorage/server/audit_events_handler_test.go` (~150 LOC added)
   ```go
   Context("when auditing write operations", func() {
       It("should audit successful writes", func() {
           // BR-STORAGE-012: Must audit all write operations
           // ... test logic ...
       })

       It("should audit write failures", func() {
           // BR-STORAGE-012: Must audit failures
           // ... test logic ...
       })

       It("should audit DLQ fallback", func() {
           // BR-STORAGE-012: Must audit DLQ fallback
           // ... test logic ...
       })

       It("should not block business operations if audit fails", func() {
           // BR-STORAGE-014: Audit failures must not block writes
           // ... test logic ...
       })
   })
   ```

**EOD Deliverables**:
- ✅ `InternalAuditClient` implemented and tested
- ✅ Audit store integrated in server
- ✅ Audit calls added to write operations
- ✅ All unit tests passing
- ✅ Day 1 EOD report

**Validation Commands**:
```bash
# Verify all tests pass
go test ./pkg/audit/... -v
go test ./pkg/datastorage/server/... -v

# Verify no lint errors
golangci-lint run ./pkg/audit/... ./pkg/datastorage/server/...

# Expected: All tests PASS, no lint errors
```

---

### **Day 2 (Morning): Testing (CHECK Phase)**

**Phase**: CHECK
**Duration**: 4 hours
**Focus**: Comprehensive unit and integration test coverage

**Hour 1-2: Expand Unit Tests (70%+ coverage)**

1. **Expand unit tests** for edge cases
   - Test batch insert with 1000 events
   - Test transaction rollback on failure
   - Test concurrent audit calls
   - Test audit during server shutdown

2. **Run coverage analysis**
   ```bash
   go test ./pkg/audit/... -coverprofile=coverage.out
   go tool cover -func=coverage.out | grep total
   # Target: ≥70% coverage
   ```

**Hour 3-4: Integration Tests**

3. **Create integration test** `test/integration/datastorage/audit_self_auditing_test.go` (~300 LOC)
   ```go
   package datastorage

   import (
       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
   )

   var _ = Describe("Data Storage Self-Auditing Integration", func() {
       Context("when writing audit events", func() {
           It("should generate audit traces for successful writes", func() {
               // BR-STORAGE-012: Must audit all write operations

               // Write audit event via REST API
               payload := map[string]interface{}{
                   "event_type":     "gateway.signal.received",
                   "event_category": "signal",
                   "event_action":   "received",
                   "event_outcome":  "success",
                   "correlation_id": "test-correlation-id",
                   // ... other required fields ...
               }

               resp := postAuditEvent(payload)
               Expect(resp.StatusCode).To(Equal(201))

               // Verify audit trace was generated
               Eventually(func() int {
                   return countAuditEvents("datastorage.audit.written", "test-correlation-id")
               }, "5s", "100ms").Should(Equal(1))
           })

           It("should generate audit traces for write failures", func() {
               // BR-STORAGE-012: Must audit failures

               // Simulate PostgreSQL failure
               stopPostgreSQL()
               defer startPostgreSQL()

               payload := map[string]interface{}{/* ... */}
               resp := postAuditEvent(payload)
               Expect(resp.StatusCode).To(Equal(202)) // DLQ fallback

               // Verify failure audit trace was generated
               Eventually(func() int {
                   return countAuditEvents("datastorage.audit.failed", "test-correlation-id")
               }, "5s", "100ms").Should(Equal(1))

               // Verify DLQ fallback audit trace was generated
               Eventually(func() int {
                   return countAuditEvents("datastorage.dlq.fallback", "test-correlation-id")
               }, "5s", "100ms").Should(Equal(1))
           })

           It("should not block writes if audit fails", func() {
               // BR-STORAGE-014: Audit failures must not block business operations

               // Simulate audit buffer full (drop audit events)
               fillAuditBuffer()

               payload := map[string]interface{}{/* ... */}
               resp := postAuditEvent(payload)
               Expect(resp.StatusCode).To(Equal(201)) // Write still succeeds
           })
       })

       Context("when server shuts down", func() {
           It("should flush remaining audit events", func() {
               // BR-STORAGE-014: Must not lose audit events during shutdown

               // Write multiple audit events
               for i := 0; i < 100; i++ {
                   postAuditEvent(generatePayload(i))
               }

               // Graceful shutdown
               shutdownServer()

               // Verify all audit traces were flushed
               count := countAuditEvents("datastorage.audit.written", "")
               Expect(count).To(Equal(100))
           })
       })
   })
   ```

4. **Run integration tests**
   ```bash
   make test-integration-datastorage
   # Expected: All integration tests pass
   ```

**EOD Deliverables**:
- ✅ Unit test coverage ≥70%
- ✅ Integration tests passing
- ✅ All edge cases covered

---

### **Day 2 (Afternoon): Production Readiness (PRODUCTION Phase)**

**Phase**: PRODUCTION
**Duration**: 4 hours
**Focus**: Documentation, metrics, and handoff

**Hour 1-2: Documentation**

1. **Update** `docs/services/stateless/data-storage/BUSINESS_REQUIREMENTS.md`
   - Add BR-STORAGE-012, BR-STORAGE-013, BR-STORAGE-014
   - Mark as implemented
   - Link to implementation files

2. **Update** `docs/services/stateless/data-storage/README.md`
   - Add "Self-Auditing" section
   - Document audit event types
   - Document `InternalAuditClient` pattern

3. **Create** `docs/services/stateless/data-storage/operations/audit-runbook.md`
   - Audit event types and meanings
   - Troubleshooting guide
   - Common issues and solutions
   - Monitoring and alerting

**Hour 3: Metrics Dashboard**

4. **Update Grafana dashboard** (if exists)
   - Add panel for `audit_events_buffered_total`
   - Add panel for `audit_events_dropped_total`
   - Add panel for `audit_write_duration_seconds`
   - Add alerts for high drop rate

**Hour 4: Handoff Summary**

5. **Create handoff summary** `docs/services/stateless/data-storage/DD-STORAGE-012-HANDOFF.md`
   - Executive summary
   - Architecture overview (InternalAuditClient pattern)
   - Key decisions (why not REST API)
   - Lessons learned
   - Known limitations
   - Future work

**EOD Deliverables**:
- ✅ Documentation complete
- ✅ Metrics dashboard updated
- ✅ Runbook created
- ✅ Handoff summary delivered

---

## 🧪 **TDD Do's and Don'ts - MANDATORY**

### **✅ DO: Strict TDD Discipline**

1. **Write ONE test at a time** (not batched)
   ```go
   // ✅ CORRECT: TDD Cycle 1
   It("should write audit events directly to PostgreSQL", func() {
       // Test InternalAuditClient.StoreBatch
   })
   // Run test → FAIL (RED)
   // Implement StoreBatch → PASS (GREEN)
   // Refactor if needed
   ```

2. **Test WHAT the system does** (behavior), not HOW (implementation)
   ```go
   // ✅ CORRECT: Behavior-focused
   It("should not block business operations if audit fails", func() {
       // Simulate audit buffer full
       // Write audit event
       // Verify write still succeeds (201 Created)
   })
   ```

3. **Use specific assertions** (not weak checks)
   ```go
   // ✅ CORRECT: Specific business assertions
   Expect(resp.StatusCode).To(Equal(201))
   Expect(auditEvent.EventType).To(Equal("datastorage.audit.written"))
   ```

### **❌ DON'T: Anti-Patterns to Avoid**

1. **DON'T batch test writing**
   ```go
   // ❌ WRONG: Writing 10 tests before any implementation
   It("test 1", func() { ... })
   It("test 2", func() { ... })
   // ... 8 more tests
   // Then implementing all at once
   ```

2. **DON'T test implementation details**
   ```go
   // ❌ WRONG: Testing internal buffer state
   Expect(auditStore.buffer).To(HaveLen(5))
   ```

3. **DON'T use weak assertions (NULL-TESTING)**
   ```go
   // ❌ WRONG: Weak assertions
   Expect(auditEvent).ToNot(BeNil())
   Expect(auditEvents).ToNot(BeEmpty())
   ```

**Reference**: `.cursor/rules/08-testing-anti-patterns.mdc` for automated detection

---

## 📊 **Test Examples**

### **Unit Test Example**

```go
package audit  // White-box testing - same package as code under test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/DATA-DOG/go-sqlmock"
)

var _ = Describe("InternalAuditClient Unit Tests", func() {
    var (
        ctx    context.Context
        client DataStorageClient
        db     *sql.DB
        mock   sqlmock.Sqlmock
    )

    BeforeEach(func() {
        ctx = context.Background()
        db, mock, _ = sqlmock.New()
        client = NewInternalAuditClient(db)
    })

    Context("when storing audit batch", func() {
        It("should write directly to PostgreSQL without REST API", func() {
            // BUSINESS SCENARIO: Data Storage Service audits its own operations
            // BR-STORAGE-013: Must not create circular dependency (no REST API calls)

            events := []*AuditEvent{
                {
                    EventType:     "datastorage.audit.written",
                    EventCategory: "storage",
                    EventAction:   "written",
                    EventOutcome:  "success",
                    CorrelationID: "test-correlation-id",
                    // ... other required fields
                },
            }

            // BEHAVIOR: Writes directly to PostgreSQL (not HTTP)
            mock.ExpectBegin()
            mock.ExpectPrepare("INSERT INTO audit_events")
            mock.ExpectExec("INSERT INTO audit_events").
                WillReturnResult(sqlmock.NewResult(1, 1))
            mock.ExpectCommit()

            err := client.StoreBatch(ctx, events)

            // CORRECTNESS: Write succeeds without REST API call
            Expect(err).ToNot(HaveOccurred())
            Expect(mock.ExpectationsWereMet()).To(Succeed())

            // BUSINESS OUTCOME: Circular dependency avoided
            // This validates BR-STORAGE-013: No REST API calls to self
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
)

var _ = Describe("Data Storage Audit Integration Tests", func() {
    var (
        ctx        context.Context
        testServer *Server
        testDB     *sql.DB
    )

    BeforeEach(func() {
        ctx = context.Background()
        testDB = setupTestDatabase()
        testServer = NewServer(testConfig, testLogger)
    })

    AfterEach(func() {
        testServer.Shutdown(ctx)
        cleanupTestDatabase(testDB)
    })

    Context("when writing audit events", func() {
        It("should generate audit traces for successful writes", func() {
            // BUSINESS SCENARIO: Data Storage Service audits successful writes
            // BR-STORAGE-012: Must audit all write operations

            // BEHAVIOR: Write audit event, verify audit trace generated
            payload := map[string]interface{}{
                "event_type":     "gateway.signal.received",
                "event_category": "signal",
                "event_action":   "received",
                "event_outcome":  "success",
                "correlation_id": "test-correlation-id",
            }

            resp := postAuditEvent(testServer, payload)
            Expect(resp.StatusCode).To(Equal(201))

            // CORRECTNESS: Audit trace exists in database
            Eventually(func() int {
                return countAuditEvents(testDB, "datastorage.audit.written", "test-correlation-id")
            }, "5s", "100ms").Should(Equal(1))

            // BUSINESS OUTCOME: Self-auditing working correctly
            // This validates BR-STORAGE-012: All write operations audited
        })
    })
})
```

---

## 🎯 **BR Coverage Matrix**

| BR ID | Description | Unit Tests | Integration Tests | E2E Tests | Status |
|-------|-------------|------------|-------------------|-----------|--------|
| **BR-STORAGE-012** | Data Storage Service must generate audit traces for its own operations | `internal_client_test.go`, `audit_events_handler_test.go` | `audit_self_auditing_test.go` | N/A (internal feature) | ⏸️ |
| **BR-STORAGE-013** | Audit traces must not create circular dependencies | `internal_client_test.go` | `audit_self_auditing_test.go` | N/A | ⏸️ |
| **BR-STORAGE-014** | Audit writes must not block business operations | `audit_events_handler_test.go` | `audit_self_auditing_test.go` | N/A | ⏸️ |

**Coverage Calculation**:
- **Unit**: 3/3 BRs covered (100%)
- **Integration**: 3/3 BRs covered (100%)
- **E2E**: 0/3 BRs covered (0% - internal feature, no E2E needed)
- **Total**: 3/3 BRs covered (100%)

---

## 🚨 **Critical Pitfalls to Avoid**

### **1. Circular Dependency (REST API Self-Call)**
- ❌ **Problem**: Data Storage Service cannot call its own REST API to audit operations (infinite loop)
- ✅ **Solution**: Use `InternalAuditClient` that writes directly to PostgreSQL (bypasses HTTP layer)
- **Impact**: Critical - would cause infinite recursion and service crash

### **2. Blocking Business Operations**
- ❌ **Problem**: Synchronous audit writes could block business operations if database is slow
- ✅ **Solution**: Use `pkg/audit/BufferedAuditStore` with asynchronous buffered writes
- **Impact**: High - would degrade write performance by 50-100ms per operation

### **3. Audit Event Loss During Shutdown**
- ❌ **Problem**: Buffered audit events could be lost if server shuts down before flushing
- ✅ **Solution**: Call `auditStore.Close()` in graceful shutdown (DD-007 pattern)
- **Impact**: Medium - would lose audit traces for last 1-5 seconds of operations

---

## 📈 **Success Criteria**

### **Technical Success**
- ✅ All tests passing (Unit 70%+, Integration >50%)
- ✅ No lint errors
- ✅ `InternalAuditClient` implemented and tested
- ✅ Audit calls integrated in write operations
- ✅ Documentation complete

### **Business Success**
- ✅ BR-STORAGE-012 validated (all write operations audited)
- ✅ BR-STORAGE-013 validated (no circular dependency)
- ✅ BR-STORAGE-014 validated (non-blocking audit writes)
- ✅ Success metrics achieved (100% audit coverage, <5ms latency overhead)

### **Confidence Assessment**
- **Target**: ≥85% confidence
- **Calculation**: Evidence-based (test coverage + BR validation + integration status)
- **Current**: 85% (based on existing `pkg/audit/` infrastructure and clear implementation path)

---

## 🔄 **Rollback Plan**

### **Rollback Triggers**
- Critical bug discovered in audit integration
- Performance degradation >10% in write operations
- Circular dependency detected

### **Rollback Procedure**
1. Remove audit calls from `audit_events_handler.go` (3 function calls)
2. Remove `auditStore` field from `Server` struct
3. Remove `InternalAuditClient` initialization from `NewServer`
4. Deploy previous version
5. Verify rollback success (write operations work without audit)
6. Document rollback reason

**Rollback Time**: <30 minutes (simple code removal)

---

## 📚 **References**

### **Design Decisions**
- **ADR-034**: [Unified Audit Table Design](../../../architecture/decisions/ADR-034-unified-audit-table-design.md) - Event sourcing pattern
- **DD-AUDIT-002**: [Audit Shared Library Design](../../../architecture/decisions/DD-AUDIT-002-audit-shared-library-design.md) - `InternalAuditClient` pattern (lines 1386-1491)
- **DD-AUDIT-003**: [Service Audit Trace Requirements](../../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) - Data Storage audit requirements
- **DD-007**: [Graceful Shutdown Pattern](../../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md) - 4-step shutdown (ensures audit flush)

### **Standards**
- [03-testing-strategy.mdc](../../../.cursor/rules/03-testing-strategy.mdc) - Testing framework
- [02-go-coding-standards.mdc](../../../.cursor/rules/02-go-coding-standards.mdc) - Go patterns
- [08-testing-anti-patterns.mdc](../../../.cursor/rules/08-testing-anti-patterns.mdc) - Testing anti-patterns

### **Existing Infrastructure**
- `pkg/audit/store.go` - `BufferedAuditStore` implementation
- `pkg/audit/event.go` - `AuditEvent` model
- `pkg/datastorage/server/audit_events_handler.go` - Unified audit events endpoint

---

**Document Status**: 📋 **DRAFT**
**Last Updated**: 2025-11-20
**Version**: 1.0
**Maintained By**: Development Team

---

## 📝 **Next Steps**

1. **Get approval** for this implementation plan
2. **Execute Day 1**: Implement `InternalAuditClient` and integrate audit calls
3. **Execute Day 2**: Complete testing and documentation
4. **Handoff**: Deliver production-ready self-auditing feature

**Ready to implement?** Start with Day 1 (Implementation) and follow the APDC-TDD methodology!

