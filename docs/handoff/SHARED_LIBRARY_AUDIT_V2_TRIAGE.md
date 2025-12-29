# Shared Library Audit V2.0 Migration - Implementation Triage

**Date**: December 14, 2025
**Priority**: P0 - CRITICAL (Blocks All Teams)
**Total Effort**: 8-12 hours (Platform Team)
**Status**: 🚨 **READY TO EXECUTE** - All Teams Coordinated

---

## 📋 **Executive Summary**

All teams have coordinated to **HOLD** their individual updates. Platform team has clearance to update the shared library (`pkg/audit/`) for all teams simultaneously.

**Scope**: 30 files impacted across 6 services + shared library + tests
**Strategy**: Phased implementation with validation gates
**Risk**: HIGH (affects all services) - Mitigated by comprehensive testing

---

## 🎯 **Coordination Status**

✅ **Gateway Team**: Holding updates, ready for shared library changes
✅ **WorkflowExecution Team**: Holding updates, ready for shared library changes
✅ **Data Storage Team**: Holding updates, coordinated on self-auditing scope
✅ **SignalProcessing Team**: Holding updates
✅ **AIAnalysis Team**: Holding updates
✅ **RemediationOrchestrator Team**: Holding updates
✅ **Notification Team**: Holding updates

**Platform Team**: GO for shared library updates

---

## 📊 **Impact Analysis**

### **Files Requiring Changes** (30 Total)

#### **Shared Library** (`pkg/audit/`) - 6 files
| File | Change Type | Lines | Risk | Priority |
|------|-------------|-------|------|----------|
| `pkg/audit/store.go` | Modify | ~100 | CRITICAL | P0 |
| `pkg/audit/event.go` | Delete/Deprecate | -300 | CRITICAL | P0 |
| `pkg/audit/internal_client.go` | Modify | ~50 | HIGH | P0 |
| `pkg/audit/http_client.go` | Modify | ~30 | HIGH | P0 |
| `pkg/audit/helpers.go` | ✅ Created | +150 | LOW | DONE |
| `pkg/audit/*_test.go` | Modify | ~200 | MEDIUM | P1 |

#### **Adapter Layer** (`pkg/datastorage/audit/`) - 3 files
| File | Change Type | Lines | Risk | Priority |
|------|-------------|-------|------|----------|
| `openapi_adapter.go` | Delete | -267 | HIGH | P0 |
| `workflow_catalog_event.go` | Modify | ~30 | MEDIUM | P1 |
| `workflow_search_event.go` | Modify | ~30 | MEDIUM | P1 |

#### **Services** (21 files across 6 services)
- **WorkflowExecution**: 3 files
- **Gateway**: 1 file
- **SignalProcessing**: 2 files
- **AIAnalysis**: 1 file
- **RemediationOrchestrator**: 1 file
- **Notification**: 3 files
- **Data Storage**: 10 files (includes self-auditing changes)

---

## 🔄 **Implementation Strategy**

### **Phase-Gate Approach**

```
Phase 1: Shared Library Core (2-3 hours) → VALIDATION GATE
    ↓
Phase 2: Adapter & HTTP Client (1-2 hours) → VALIDATION GATE
    ↓
Phase 3: Service Updates (Per-Service, 30-60 min each) → VALIDATION GATE
    ↓
Phase 4: Test Updates (2-3 hours) → VALIDATION GATE
    ↓
Phase 5: Integration & E2E (1-2 hours) → FINAL VALIDATION
```

**Validation Gates**: Must pass before proceeding to next phase
- ✅ Compiles successfully
- ✅ All unit tests pass
- ✅ No linter errors
- ✅ No breaking API changes (except expected ones)

---

## 📝 **Phase 1: Shared Library Core Updates**

**Duration**: 2-3 hours
**Risk**: CRITICAL
**Validation**: Build + Unit Tests

### **1.1 Update `pkg/audit/store.go`**

**Changes Required**:

```go
// IMPORT ADDITIONS
import (
    dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

// INTERFACE CHANGES (Lines 25-59)
type AuditStore interface {
    // OLD: StoreAudit(ctx context.Context, event *AuditEvent) error
    // NEW:
    StoreAudit(ctx context.Context, event *dsgen.AuditEventRequest) error
    Close() error
}

type DataStorageClient interface {
    // OLD: StoreBatch(ctx context.Context, events []*AuditEvent) error
    // NEW:
    StoreBatch(ctx context.Context, events []*dsgen.AuditEventRequest) error
}

type DLQClient interface {
    // OLD: EnqueueAuditEvent(ctx context.Context, event *AuditEvent, originalError error) error
    // NEW:
    EnqueueAuditEvent(ctx context.Context, event *dsgen.AuditEventRequest, originalError error) error
}

// STRUCT CHANGES (Lines 94-111)
type BufferedAuditStore struct {
    // OLD: buffer chan *AuditEvent
    // NEW:
    buffer chan *dsgen.AuditEventRequest

    client    DataStorageClient
    dlqClient DLQClient
    // ... rest unchanged
}

// METHOD UPDATES (Lines 209-239)
func (s *BufferedAuditStore) StoreAudit(ctx context.Context, event *dsgen.AuditEventRequest) error {
    // OLD: Validate with event.Validate()
    // NEW: Validate OpenAPI required fields
    if event.EventType == "" {
        return fmt.Errorf("event_type is required")
    }
    if event.EventCategory == "" {
        return fmt.Errorf("event_category is required")
    }
    if event.EventAction == "" {
        return fmt.Errorf("event_action is required")
    }
    if event.CorrelationId == "" {
        return fmt.Errorf("correlation_id is required")
    }

    // Rest of method unchanged (select on buffer, etc.)
}

// BACKGROUND WRITER UPDATES (Lines 307-338)
func (s *BufferedAuditStore) backgroundWriter() {
    // Change batch type
    // OLD: batch := make([]*AuditEvent, 0, s.config.BatchSize)
    // NEW:
    batch := make([]*dsgen.AuditEventRequest, 0, s.config.BatchSize)

    // Rest of method unchanged
}

// WRITE BATCH UPDATES (Lines 353-418)
func (s *BufferedAuditStore) writeBatchWithRetry(batch []*dsgen.AuditEventRequest) {
    // Method signature changed, rest of logic unchanged
}

// DLQ ENQUEUE UPDATES (Lines 427-449)
func (s *BufferedAuditStore) enqueueBatchToDLQ(ctx context.Context, batch []*dsgen.AuditEventRequest, originalError error) {
    // Method signature changed, rest of logic unchanged
}
```

**Validation**:
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go build ./pkg/audit/...
# Should compile successfully
```

---

### **1.2 Deprecate `pkg/audit/event.go`**

**Option A: Delete Entirely** (RECOMMENDED for clean break)
```bash
rm pkg/audit/event.go
```

**Option B: Mark Deprecated** (if gradual migration needed)
```go
// Add to top of file:
// DEPRECATED: Use dsgen.AuditEventRequest from pkg/datastorage/client directly.
// This file will be removed in the next release.
// Migration guide: docs/handoff/AUDIT_REFACTORING_V2_COMPLETE_CHANGES_SUMMARY.md
```

**Recommendation**: Option A (Delete) - We have all teams coordinated, clean break is safer

---

### **1.3 Update `pkg/audit/internal_client.go`**

**Current Implementation**:
```go
func (c *InternalAuditClient) StoreBatch(ctx context.Context, events []*AuditEvent) error {
    // Convert audit.AuditEvent to repository type
}
```

**New Implementation**:
```go
func (c *InternalAuditClient) StoreBatch(ctx context.Context, events []*dsgen.AuditEventRequest) error {
    // Convert dsgen.AuditEventRequest to repository type
    for _, event := range events {
        repoEvent := &repository.AuditEvent{
            EventType:      event.EventType,
            EventCategory:  event.EventCategory,
            EventAction:    event.EventAction,
            EventOutcome:   string(event.EventOutcome),
            EventTimestamp: event.EventTimestamp,
            ActorType:      ptrToString(event.ActorType),
            ActorID:        ptrToString(event.ActorId),
            ResourceType:   ptrToString(event.ResourceType),
            ResourceID:     ptrToString(event.ResourceId),
            CorrelationID:  event.CorrelationId,
            Namespace:      ptrToString(event.Namespace),
            ClusterName:    ptrToString(event.ClusterName),
            EventData:      event.EventData, // Already map[string]interface{}
            DurationMs:     ptrToInt(event.DurationMs),
            Severity:       ptrToString(event.Severity),
        }
        // ... insert to DB
    }
}

// Helper function
func ptrToString(s *string) string {
    if s == nil {
        return ""
    }
    return *s
}

func ptrToInt(i *int) int {
    if i == nil {
        return 0
    }
    return *i
}
```

---

### **1.4 Update `pkg/audit/http_client.go`**

**Current**:
```go
func (c *HTTPDataStorageClient) StoreBatch(ctx context.Context, events []*audit.AuditEvent) error {
    // ...
}
```

**New**:
```go
func (c *HTTPDataStorageClient) StoreBatch(ctx context.Context, events []*dsgen.AuditEventRequest) error {
    // Use OpenAPI client directly
    for _, event := range events {
        _, err := c.client.CreateAuditEvent(ctx, *event)
        if err != nil {
            return err
        }
    }
    return nil
}
```

---

### **Phase 1 Validation Gate**

**Checklist**:
- [ ] `pkg/audit/store.go` compiles
- [ ] `pkg/audit/event.go` deleted (or deprecated)
- [ ] `pkg/audit/internal_client.go` compiles
- [ ] `pkg/audit/http_client.go` compiles
- [ ] `pkg/audit/helpers.go` exists (already done ✅)
- [ ] Run: `go build ./pkg/audit/...` → SUCCESS
- [ ] All changes committed to feature branch

**If validation fails**: Rollback and fix before proceeding

---

## 📝 **Phase 2: Adapter & Client Updates**

**Duration**: 1-2 hours
**Risk**: HIGH
**Validation**: Build + Unit Tests

### **2.1 Delete `pkg/datastorage/audit/openapi_adapter.go`**

```bash
rm pkg/datastorage/audit/openapi_adapter.go
```

**Impact**: Services must create OpenAPI client directly

**Migration Pattern** (for each service):
```go
// OLD
import dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
dsClient, err := dsaudit.NewOpenAPIAuditClient(dataStorageURL, 10*time.Second)

// NEW
import dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
httpClient := &http.Client{Timeout: 10 * time.Second}
dsClient, err := dsgen.NewClient(dataStorageURL, dsgen.WithHTTPClient(httpClient))

// Create wrapper that implements DataStorageClient
type OpenAPIAuditClient struct {
    Client *dsgen.Client
}

func (c *OpenAPIAuditClient) StoreBatch(ctx context.Context, events []*dsgen.AuditEventRequest) error {
    for _, event := range events {
        _, err := c.Client.CreateAuditEvent(ctx, *event)
        if err != nil {
            return err
        }
    }
    return nil
}

auditClient := &OpenAPIAuditClient{Client: dsClient}
auditStore, err := audit.NewBufferedStore(auditClient, auditConfig, "servicename", logger)
```

---

### **2.2 Update Workflow Catalog Audit Helpers**

**Files**:
- `pkg/datastorage/audit/workflow_catalog_event.go`
- `pkg/datastorage/audit/workflow_search_event.go`

**Changes**:
```go
// OLD
func NewWorkflowSearchAuditEvent(...) (*audit.AuditEvent, error) {
    event := audit.NewAuditEvent()
    // ... set fields
}

// NEW
func NewWorkflowSearchAuditEvent(...) (*dsgen.AuditEventRequest, error) {
    event := audit.NewAuditEventRequest()
    // ... set fields using helpers
    audit.SetEventType(event, "datastorage.workflow.searched")
    audit.SetEventCategory(event, "workflow_catalog")
    // ... etc
}
```

---

### **Phase 2 Validation Gate**

**Checklist**:
- [ ] `openapi_adapter.go` deleted
- [ ] Workflow audit helpers updated
- [ ] Run: `go build ./pkg/datastorage/audit/...` → SUCCESS
- [ ] All changes committed

---

## 📝 **Phase 3: Service-by-Service Updates**

**Duration**: 30-60 minutes per service (parallel possible)
**Risk**: MEDIUM per service
**Validation**: Service builds + unit tests pass

### **Per-Service Update Pattern**

**For each service (WE, Gateway, SignalProcessing, AIAnalysis, RO, Notification)**:

1. **Update audit event creation**:
```go
// OLD
event := audit.NewAuditEvent()
event.EventType = "service.action"
event.EventCategory = "category"
event.EventAction = "action"
event.EventOutcome = "success"
event.ActorType = "service"
event.ActorID = "service-name"
event.ResourceType = "Resource"
event.ResourceID = resourceID
event.CorrelationID = correlationID
event.EventData = eventDataJSON

// NEW
event := audit.NewAuditEventRequest()
audit.SetEventType(event, "service.action")
audit.SetEventCategory(event, "category")
audit.SetEventAction(event, "action")
audit.SetEventOutcome(event, audit.OutcomeSuccess)
audit.SetActor(event, "service", "service-name")
audit.SetResource(event, "Resource", resourceID)
audit.SetCorrelationID(event, correlationID)

// EventData: Use map directly or convert from CommonEnvelope
envelope := audit.NewEventData("service", "action", "success", payloadMap)
eventDataMap, _ := audit.EnvelopeToMap(envelope)
audit.SetEventData(event, eventDataMap)
```

2. **Update main.go client instantiation** (see Phase 2.1 pattern)

3. **Validate**:
```bash
cd service-directory
go build ./...
go test ./... -short  # Unit tests only
```

---

### **Service Update Order** (Recommended)

1. **WorkflowExecution** (30-60 min) - Team ready, good test coverage
2. **Gateway** (30-60 min) - Team already started
3. **Notification** (30-60 min) - Simple service
4. **SignalProcessing** (45-60 min) - Custom audit client
5. **AIAnalysis** (30-45 min) - Straightforward
6. **RemediationOrchestrator** (30-45 min) - Similar to others
7. **Data Storage** (60-90 min) - Self-auditing + workflow catalog

**Parallel Execution**: Services 1-6 can be updated in parallel by different team members

---

### **Phase 3 Validation Gate** (Per Service)

**Checklist (Per Service)**:
- [ ] Audit event creation updated
- [ ] Main.go client updated
- [ ] Service builds: `go build ./cmd/servicename`
- [ ] Unit tests pass: `go test ./internal/... -short`
- [ ] No linter errors
- [ ] Changes committed to feature branch

---

## 📝 **Phase 4: Test Updates**

**Duration**: 2-3 hours
**Risk**: MEDIUM
**Validation**: All tests pass

### **4.1 Update Unit Tests**

**Pattern for all `*_test.go` files**:
```go
// OLD
func createTestEvent() *audit.AuditEvent {
    event := audit.NewAuditEvent()
    event.EventType = "test.event"
    // ...
    return event
}

// NEW
func createTestEvent() *dsgen.AuditEventRequest {
    event := audit.NewAuditEventRequest()
    audit.SetEventType(event, "test.event")
    // ...
    return event
}
```

**Files to Update** (~10 files):
- `test/unit/audit/store_test.go`
- `test/unit/audit/internal_client_test.go`
- `test/unit/audit/event_test.go` (DELETE if event.go deleted)
- `test/unit/workflowexecution/controller_test.go`
- `test/unit/signalprocessing/audit_client_test.go`
- `test/unit/aianalysis/audit_client_test.go`
- `test/unit/notification/audit_test.go`
- `test/unit/datastorage/workflow_audit_test.go`
- `test/unit/datastorage/dlq/client_test.go`

---

### **4.2 Update Integration Tests**

**CRITICAL**: Integration tests must use REAL OpenAPI client, not mocks

**Pattern**:
```go
// OLD (Mock)
type testableAuditStore struct {
    mu     sync.Mutex
    events []audit.AuditEvent  // ← MOCK
}

// NEW (Real OpenAPI Client)
var (
    dataStorageClient *dsgen.Client
    auditStore        audit.AuditStore
)

BeforeSuite(func() {
    // Setup Data Storage
    // ...

    // Create REAL OpenAPI client
    httpClient := &http.Client{Timeout: 10 * time.Second}
    dataStorageClient, err = dsgen.NewClient(dataStorageURL, dsgen.WithHTTPClient(httpClient))
    Expect(err).ToNot(HaveOccurred())

    // Create audit client wrapper
    auditClient := &testutil.OpenAPIAuditClient{Client: dataStorageClient}
    auditStore, err = audit.NewBufferedStore(auditClient, audit.DefaultConfig(), "test-service", logger)
    Expect(err).ToNot(HaveOccurred())
})
```

**Files to Update** (~8 files):
- `test/integration/workflowexecution/suite_test.go`
- `test/integration/workflowexecution/audit_datastorage_test.go`
- `test/integration/notification/suite_test.go`
- `test/integration/notification/audit_integration_test.go`
- `test/integration/datastorage/*_test.go` (multiple files)

---

### **Phase 4 Validation Gate**

**Checklist**:
- [ ] All unit tests pass: `go test ./test/unit/...`
- [ ] All integration tests pass: `go test ./test/integration/...`
- [ ] Integration tests use REAL OpenAPI client (not mocks)
- [ ] No test failures
- [ ] Changes committed

---

## 📝 **Phase 5: E2E & Final Validation**

**Duration**: 1-2 hours
**Risk**: LOW (if previous phases passed)
**Validation**: Full system integration

### **5.1 Update E2E Tests**

**Files**:
- `test/e2e/notification/01_notification_lifecycle_audit_test.go`
- Any other E2E tests using audit

**Changes**: Same pattern as integration tests (use real OpenAPI client)

---

### **5.2 Full System Build**

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Build all services
make build

# Run all tests
make test

# Run linters
make lint
```

---

### **Phase 5 Final Validation Gate**

**Checklist**:
- [ ] Full system builds: `make build` → SUCCESS
- [ ] All unit tests pass: `make test-unit`
- [ ] All integration tests pass: `make test-integration`
- [ ] All E2E tests pass: `make test-e2e`
- [ ] No linter errors: `make lint`
- [ ] Documentation updated (DD-AUDIT-002 already done ✅)
- [ ] Migration guide created
- [ ] All teams notified

---

## 🚨 **Rollback Plan**

If any validation gate fails:

**Option 1: Fix Forward** (Preferred if issue is minor)
- Debug and fix the issue
- Re-run validation
- Continue

**Option 2: Rollback** (If major issue discovered)
```bash
git reset --hard HEAD~N  # N = number of commits to rollback
git clean -fd
```

**Communication**:
- Immediately notify all teams
- Explain the issue
- Provide ETA for fix or alternative approach

---

## 📊 **Risk Mitigation**

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Compilation failures | MEDIUM | HIGH | Validation gates after each phase |
| Test failures | MEDIUM | HIGH | Comprehensive test updates in Phase 4 |
| Runtime errors | LOW | CRITICAL | Integration tests use real clients |
| Performance degradation | LOW | MEDIUM | Monitor metrics post-deployment |
| Team coordination failure | LOW | HIGH | All teams already coordinated ✅ |

---

## 📅 **Timeline**

**Optimistic** (Everything goes smoothly):
- Phase 1: 2 hours
- Phase 2: 1 hour
- Phase 3: 4 hours (parallel)
- Phase 4: 2 hours
- Phase 5: 1 hour
- **Total**: 8-10 hours (1-2 days)

**Realistic** (Some debugging needed):
- Phase 1: 3 hours
- Phase 2: 2 hours
- Phase 3: 6 hours (parallel)
- Phase 4: 3 hours
- Phase 5: 2 hours
- **Total**: 12-16 hours (2-3 days)

**Conservative** (Unexpected issues):
- Add 50% buffer: 18-24 hours (3-4 days)

---

## ✅ **Success Criteria**

**Must Have** (Non-Negotiable):
- [ ] All services build successfully
- [ ] All tests pass (unit + integration + E2E)
- [ ] No linter errors
- [ ] No runtime errors in integration tests
- [ ] Audit events successfully written to Data Storage

**Should Have** (Important):
- [ ] Integration tests use real OpenAPI client
- [ ] No mock audit stores in integration tests
- [ ] Performance metrics unchanged
- [ ] All teams can build and test independently

**Nice to Have** (Optional):
- [ ] Code coverage maintained or improved
- [ ] Documentation examples updated
- [ ] Migration guide created

---

## 📝 **Communication Plan**

### **Before Starting**
- [x] Notify all teams: Migration starting
- [x] Create feature branch: `feat/audit-v2-shared-library`
- [ ] Update status document: Work in progress

### **During Implementation**
- [ ] Update after each phase completion
- [ ] Notify teams if issues discovered
- [ ] Daily status updates in team channel

### **After Completion**
- [ ] Notify all teams: Shared library ready
- [ ] Provide migration guide
- [ ] Schedule team sync to answer questions
- [ ] Update DD-AUDIT-002 (already done ✅)

---

## 🎯 **Next Steps**

**Immediate Actions**:
1. ✅ Create feature branch
2. ✅ Checkpoint current state
3. → Start Phase 1 (Shared Library Core)

**Ready to Execute**: All teams coordinated, triage complete

---

**Document Status**: ✅ Triage Complete - Ready for Implementation
**Author**: WorkflowExecution Team (AI Assistant)
**Coordinator**: Platform Team
**Timeline**: 8-12 hours (1-2 days)
**Risk Level**: HIGH (mitigated by phased approach)
**Recommendation**: ✅ **PROCEED** - Begin Phase 1 immediately



