# DD-NOT-001: ADR-034 Unified Audit Table Integration - Implementation Plan

**Version**: 1.0
**Status**: 📋 DRAFT (Awaiting User Approval)
**Design Decision**: [ADR-034: Unified Audit Table Design](../../../architecture/decisions/ADR-034-unified-audit-table-design.md)
**Service**: Notification Service (Notification Controller)
**Confidence**: 85% (Evidence-Based: Shared library complete, Notification controller production-ready)
**Estimated Effort**: 5 days (APDC cycle: 2 days implementation + 2 days testing + 1 day documentation)

---

## 📋 **Version History**

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v1.0** | 2025-11-21 | Initial implementation plan created | ✅ **CURRENT** |

---

## 🎯 **Business Requirements**

### **Primary Business Requirements**

| BR ID | Description | Success Criteria |
|-------|-------------|------------------|
| **BR-NOT-062** | **Unified Audit Table Integration**: Notification Controller MUST write audit events to the unified `audit_events` table (ADR-034) for cross-service correlation, compliance reporting, and V2.0 Remediation Analysis Reports | ✅ All 4 notification event types written to `audit_events` table<br>✅ Fire-and-forget pattern (non-blocking, <1ms overhead)<br>✅ Zero audit loss through DLQ fallback<br>✅ Events queryable via correlation_id |
| **BR-NOT-063** | **Graceful Audit Degradation**: Audit write failures MUST NOT block notification delivery or cause reconciliation failures | ✅ Notification delivery succeeds even when audit writes fail<br>✅ Audit failures logged but don't stop reconciliation<br>✅ Failed audits queued to DLQ for retry |
| **BR-NOT-064** | **Audit Event Correlation**: Notification audit events MUST be correlatable with RemediationRequest events for end-to-end workflow tracing | ✅ correlation_id matches remediation_id<br>✅ parent_event_id links to remediation events<br>✅ Trace signal flow: Gateway → Orchestrator → Notification |

### **Related Existing BRs**

| BR ID | Description | Integration Point |
|-------|-------------|-------------------|
| **BR-NOT-051** | Complete Audit Trail (CRD status) | Enhanced with unified audit table persistence |
| **BR-NOT-050** | Data Loss Prevention (CRD persistence) | Complemented with audit event persistence |
| **BR-NOT-054** | Comprehensive Observability | Extended with audit write metrics |

### **Success Metrics**

- **Audit Write Latency**: <1ms (fire-and-forget, non-blocking)
- **Audit Success Rate**: >99% (with DLQ fallback for failures)
- **Zero Business Impact**: 0% notification delivery failures due to audit writes
- **Correlation Coverage**: 100% of notifications correlatable via correlation_id
- **Test Coverage**: Unit 70%+, Integration >50%, E2E 100% (critical paths)

---

## 📅 **Timeline Overview**

### **Phase Breakdown**

| Phase | Duration | Days | Purpose | Key Deliverables |
|-------|----------|------|---------|------------------|
| **ANALYSIS** | 4 hours | Day 0 (pre-work) | Comprehensive context understanding | ✅ Analysis complete (AUDIT_TRACES_FINAL_STATUS.md), risk assessment, existing code review |
| **PLAN** | 4 hours | Day 0 (pre-work) | Detailed implementation strategy | ✅ This document, TDD phase mapping, success criteria |
| **DO (Implementation)** | 2 days | Days 1-2 | Controlled TDD execution | Audit client integration, event helpers, reconciler enhancement |
| **CHECK (Testing)** | 2 days | Days 3-4 | Comprehensive result validation | Test suite (unit/integration/E2E), BR validation |
| **PRODUCTION READINESS** | 1 day | Day 5 | Documentation & deployment prep | Updated docs, confidence report, handoff summary |

### **5-Day Implementation Timeline**

| Day | Phase | Focus | Hours | Key Milestones |
|-----|-------|-------|-------|----------------|
| **Day 0** | ANALYSIS + PLAN | Pre-work | 8h | ✅ Analysis complete (this document), Plan approved |
| **Day 1** | DO-RED | Foundation + Tests | 8h | Test framework, audit helpers, failing tests |
| **Day 2** | DO-GREEN + DO-REFACTOR | Implementation + Integration | 8h | Audit store integrated, events written, reconciler enhanced |
| **Day 3** | CHECK | Unit tests | 8h | 70%+ unit test coverage, behavior validation |
| **Day 4** | CHECK | Integration + E2E tests | 8h | Integration scenarios, E2E workflow validation |
| **Day 5** | PRODUCTION | Documentation + Readiness | 8h | Updated docs, confidence report, handoff summary |

### **Critical Path Dependencies**

```
Day 0 (Analysis + Plan) ✅ COMPLETE
    ↓
Day 1 (Foundation + Tests) → Day 2 (Implementation + Integration)
    ↓
Day 3 (Unit Tests) → Day 4 (Integration + E2E Tests)
    ↓
Day 5 (Documentation + Production Readiness)
```

### **Daily Progress Tracking**

**EOD Documentation Required**:
- **Day 1 Complete**: Foundation checkpoint (test framework ready, interfaces enhanced)
- **Day 2 Complete**: Implementation complete checkpoint (audit writes integrated)
- **Day 3 Complete**: Unit testing complete checkpoint (70%+ coverage achieved)
- **Day 4 Complete**: Integration testing complete checkpoint (E2E workflows validated)
- **Day 5 Complete**: Production ready checkpoint (handoff summary complete)

---

## 📆 **Day-by-Day Implementation Breakdown**

### **Day 0: ANALYSIS + PLAN (Pre-Work) ✅**

**Phase**: ANALYSIS + PLAN
**Duration**: 8 hours
**Status**: ✅ COMPLETE (this document represents Day 0 completion)

**Deliverables**:
- ✅ Analysis document: AUDIT_TRACES_FINAL_STATUS.md (shared library 95% complete, 1 test fix needed)
- ✅ Implementation plan (this document v1.0): 5-day timeline, test examples, BR mapping
- ✅ Risk assessment: 3 critical pitfalls identified with mitigation strategies
- ✅ Existing code review:
  - `internal/controller/notification/notificationrequest_controller.go` (reconciler)
  - `api/notification/v1alpha1/notificationrequest_types.go` (CRD spec)
  - `pkg/audit/` (shared library, 95% complete)
- ✅ BR coverage matrix: 3 new BRs (BR-NOT-062, BR-NOT-063, BR-NOT-064) mapped to test scenarios

---

### **Day 1: Foundation + Test Framework (DO-RED Phase)**

**Phase**: DO-RED
**Duration**: 8 hours
**TDD Focus**: Write failing tests first, enhance existing reconciler

**⚠️ CRITICAL**: We are **ENHANCING existing Notification Controller**, not creating from scratch!

**Existing Code to Enhance**:
- ✅ `internal/controller/notification/notificationrequest_controller.go` (448 LOC) - Reconciler with delivery logic
- ✅ `api/notification/v1alpha1/notificationrequest_types.go` (315 LOC) - CRD types
- ✅ `pkg/audit/` (1,500+ LOC) - Shared audit library (95% complete)

**Morning (4 hours): Test Framework Setup + Code Analysis**

1. **Analyze existing implementation** (1 hour)
   - Read `notificationrequest_controller.go` - understand reconciliation loop
   - Identify CRD status update points (4 triggers: sent, failed, acknowledged, escalated)
   - Review existing test patterns in `test/unit/notification/` and `test/integration/notification/`
   - Confirm `pkg/audit/` shared library API

2. **Create test file** `internal/controller/notification/audit_test.go` (200-300 LOC)
   - Set up Ginkgo/Gomega test suite for audit helpers
   - Define test fixtures for 4 notification event types
   - Create mock DataStorageClient for audit writes
   - Create helper functions for audit event validation

3. **Create integration test** `test/integration/notification/audit_integration_test.go` (300-400 LOC)
   - Set up test infrastructure (mock Data Storage Service HTTP endpoint)
   - Define integration test helpers for audit write verification
   - Set up cleanup logic for test audit events

**Afternoon (4 hours): Audit Helper Interface + Failing Tests**

4. **Create** `internal/controller/notification/audit.go` (NEW file, ~200 LOC)
   ```go
   package notification

   import (
       "context"
       "github.com/jordigilh/kubernaut/pkg/audit"
       notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
   )

   // AuditHelpers provides audit event creation for notification events
   type AuditHelpers struct {
       serviceName string
   }

   // NewAuditHelpers creates audit helpers for notification controller
   func NewAuditHelpers(serviceName string) *AuditHelpers {
       return &AuditHelpers{serviceName: serviceName}
   }

   // CreateMessageSentEvent creates audit event for notification.message.sent
   func (a *AuditHelpers) CreateMessageSentEvent(notification *notificationv1alpha1.NotificationRequest, channel string) (*audit.AuditEvent, error) {
       return nil, fmt.Errorf("not implemented yet") // RED phase
   }

   // CreateMessageFailedEvent creates audit event for notification.message.failed
   func (a *AuditHelpers) CreateMessageFailedEvent(notification *notificationv1alpha1.NotificationRequest, channel string, err error) (*audit.AuditEvent, error) {
       return nil, fmt.Errorf("not implemented yet") // RED phase
   }

   // CreateMessageAcknowledgedEvent creates audit event for notification.message.acknowledged
   func (a *AuditHelpers) CreateMessageAcknowledgedEvent(notification *notificationv1alpha1.NotificationRequest) (*audit.AuditEvent, error) {
       return nil, fmt.Errorf("not implemented yet") // RED phase
   }

   // CreateMessageEscalatedEvent creates audit event for notification.message.escalated
   func (a *AuditHelpers) CreateMessageEscalatedEvent(notification *notificationv1alpha1.NotificationRequest) (*audit.AuditEvent, error) {
       return nil, fmt.Errorf("not implemented yet") // RED phase
   }
   ```

5. **Enhance** `notificationrequest_controller.go` (add auditStore field, ~10 LOC)
   ```go
   // EXISTING struct (keep as-is):
   type NotificationRequestReconciler struct {
       client.Client
       Scheme         *runtime.Scheme
       ConsoleService *delivery.ConsoleDeliveryService
       SlackService   *delivery.SlackDeliveryService
       Sanitizer      *sanitization.Sanitizer
       CircuitBreaker *retry.CircuitBreaker

       // NEW field to add:
       AuditStore    audit.AuditStore    // NEW - Audit event store
       AuditHelpers  *AuditHelpers       // NEW - Audit event helpers
   }
   ```

6. **Write failing tests** (strict TDD: ONE test at a time)

   **TDD Cycle 1**: Test `CreateMessageSentEvent` behavior
   ```go
   // File: internal/controller/notification/audit_test.go
   package notification

   import (
       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
       "github.com/jordigilh/kubernaut/pkg/audit"
       notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
       metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
   )

   var _ = Describe("Audit Helpers", func() {
       var (
           helpers      *AuditHelpers
           notification *notificationv1alpha1.NotificationRequest
       )

       BeforeEach(func() {
           helpers = NewAuditHelpers("notification-controller")
           notification = &notificationv1alpha1.NotificationRequest{
               ObjectMeta: metav1.ObjectMeta{
                   Name:      "test-notification",
                   Namespace: "default",
               },
               Spec: notificationv1alpha1.NotificationRequestSpec{
                   RemediationID:  "remediation-123",
                   Title:          "Test Alert",
                   Severity:       "critical",
                   Channels:       []notificationv1alpha1.NotificationChannel{"slack"},
               },
           }
       })

       Context("CreateMessageSentEvent", func() {
           It("should create notification.message.sent audit event with correct fields", func() {
               // BR-NOT-062: Unified audit table integration
               event, err := helpers.CreateMessageSentEvent(notification, "slack")

               Expect(err).ToNot(HaveOccurred(), "Should create audit event without error")
               Expect(event).ToNot(BeNil(), "Event should be created")

               // Validate ADR-034 required fields
               Expect(event.EventType).To(Equal("notification.message.sent"))
               Expect(event.EventCategory).To(Equal("notification"))
               Expect(event.EventAction).To(Equal("sent"))
               Expect(event.EventOutcome).To(Equal("success"))
               Expect(event.ActorType).To(Equal("service"))
               Expect(event.ActorID).To(Equal("notification-controller"))
               Expect(event.ResourceType).To(Equal("NotificationRequest"))
               Expect(event.ResourceID).To(Equal("test-notification"))
               Expect(event.CorrelationID).To(Equal("remediation-123"))
               Expect(event.EventData).ToNot(BeNil(), "Event data should be populated")
           })
       })
   })
   ```

   **TDD Cycle 2**: Test `CreateMessageFailedEvent` behavior
   **TDD Cycle 3**: Test `CreateMessageAcknowledgedEvent` behavior
   **TDD Cycle 4**: Test `CreateMessageEscalatedEvent` behavior

   - Run tests → Verify they FAIL (RED phase)

**EOD Deliverables**:
- ✅ Test framework complete (`audit_test.go`, `audit_integration_test.go`)
- ✅ 4 failing tests (RED phase) - one per event type
- ✅ Enhanced interfaces defined (`audit.go`, reconciler with auditStore field)
- ✅ Day 1 EOD report

**Validation Commands**:
```bash
# Verify tests fail (RED phase)
go test ./internal/controller/notification/audit_test.go -v 2>&1 | grep "FAIL"

# Expected: All 4 tests should FAIL with "not implemented yet"
```

---

### **Day 2: Core Logic + Integration (DO-GREEN + DO-REFACTOR Phase)**

**Phase**: DO-GREEN + DO-REFACTOR
**Duration**: 8 hours
**TDD Focus**: Minimal implementation to pass tests, then integrate with reconciler

**Morning (4 hours): Audit Helper Implementation (DO-GREEN)**

1. **Implement** `CreateMessageSentEvent` (minimal GREEN implementation)
   ```go
   // File: internal/controller/notification/audit.go

   func (a *AuditHelpers) CreateMessageSentEvent(notification *notificationv1alpha1.NotificationRequest, channel string) (*audit.AuditEvent, error) {
       // Create event_data payload
       payload := map[string]interface{}{
           "notification_id": notification.Name,
           "channel":         channel,
           "title":           notification.Spec.Title,
           "severity":        notification.Spec.Severity,
           "delivery_status": "sent",
       }

       eventData := audit.NewEventData("notification", "message_sent", "success", payload)
       eventDataJSON, err := eventData.ToJSON()
       if err != nil {
           return nil, fmt.Errorf("failed to serialize event data: %w", err)
       }

       // Create audit event (ADR-034 format)
       event := audit.NewAuditEvent() // Auto-fills: EventID, EventVersion, EventTimestamp, RetentionDays
       event.EventType = "notification.message.sent"
       event.EventCategory = "notification"
       event.EventAction = "sent"
       event.EventOutcome = "success"
       event.ActorType = "service"
       event.ActorID = a.serviceName
       event.ResourceType = "NotificationRequest"
       event.ResourceID = notification.Name
       event.CorrelationID = notification.Spec.RemediationID
       event.Namespace = &notification.Namespace
       event.EventData = eventDataJSON

       return event, nil
   }
   ```

2. **Implement** `CreateMessageFailedEvent`, `CreateMessageAcknowledgedEvent`, `CreateMessageEscalatedEvent` (similar pattern)

3. **Run tests** → Verify they PASS (GREEN phase)

**Afternoon (4 hours): Reconciler Integration (DO-REFACTOR)**

4. **Initialize audit store in controller setup** (minimal integration)
   ```go
   // File: cmd/notification/main.go (or controller initialization)

   func setupNotificationController(mgr ctrl.Manager, config *Config, logger *zap.Logger) error {
       // Create Data Storage client
       dsClient := client.NewDataStorageClient(config.DataStorageURL)

       // Create audit store (fire-and-forget pattern)
       auditStore, err := audit.NewBufferedStore(
           dsClient,
           audit.RecommendedConfig("notification"), // 10,000 buffer, 1s flush
           "notification",
           logger,
       )
       if err != nil {
           return fmt.Errorf("failed to create audit store: %w", err)
       }

       // Create audit helpers
       auditHelpers := notification.NewAuditHelpers("notification-controller")

       // Create controller with audit store
       reconciler := &notification.NotificationRequestReconciler{
           Client:         mgr.GetClient(),
           Scheme:         mgr.GetScheme(),
           ConsoleService: consoleService,
           SlackService:   slackService,
           Sanitizer:      sanitizer,
           CircuitBreaker: circuitBreaker,
           AuditStore:     auditStore,     // NEW
           AuditHelpers:   auditHelpers,   // NEW
       }

       return reconciler.SetupWithManager(mgr)
   }
   ```

5. **Enhance Reconcile() to write audit events** (4 integration points)
   ```go
   // File: internal/controller/notification/notificationrequest_controller.go

   func (r *NotificationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
       log := log.FromContext(ctx)

       // ... existing reconciliation logic ...

       // INTEGRATION POINT 1: After successful delivery
       for _, channel := range notification.Spec.Channels {
           deliveryErr := r.deliverToChannel(ctx, notification, channel)

           if deliveryErr != nil {
               // Audit: Message failed
               r.auditMessageFailed(ctx, notification, string(channel), deliveryErr)
           } else {
               // Audit: Message sent
               r.auditMessageSent(ctx, notification, string(channel))
           }

           // ... existing status update logic ...
       }

       return ctrl.Result{}, nil
   }

   // Helper function: Audit message sent (non-blocking)
   func (r *NotificationRequestReconciler) auditMessageSent(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, channel string) {
       event, err := r.AuditHelpers.CreateMessageSentEvent(notification, channel)
       if err != nil {
           log := log.FromContext(ctx)
           log.Error(err, "Failed to create audit event", "event_type", "message.sent")
           return
       }

       // Fire-and-forget: Audit write failures don't block reconciliation (BR-NOT-063)
       if err := r.AuditStore.StoreAudit(ctx, event); err != nil {
           log := log.FromContext(ctx)
           log.Error(err, "Failed to buffer audit event", "event_type", "message.sent")
           // Continue reconciliation (audit failure is not critical)
       }
   }

   // Helper function: Audit message failed (non-blocking)
   func (r *NotificationRequestReconciler) auditMessageFailed(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, channel string, deliveryErr error) {
       event, err := r.AuditHelpers.CreateMessageFailedEvent(notification, channel, deliveryErr)
       if err != nil {
           log := log.FromContext(ctx)
           log.Error(err, "Failed to create audit event", "event_type", "message.failed")
           return
       }

       if err := r.AuditStore.StoreAudit(ctx, event); err != nil {
           log := log.FromContext(ctx)
           log.Error(err, "Failed to buffer audit event", "event_type", "message.failed")
       }
   }
   ```

6. **Add graceful shutdown audit flush**
   ```go
   // File: cmd/notification/main.go

   func main() {
       // ... existing setup ...

       // Setup signal handler for graceful shutdown
       ctx := ctrl.SetupSignalHandler()

       if err := mgr.Start(ctx); err != nil {
           setupLog.Error(err, "problem running manager")
           os.Exit(1)
       }

       // Flush remaining audit events on shutdown (DD-007: Graceful Shutdown)
       setupLog.Info("Shutting down notification controller, flushing audit events")
       if err := auditStore.Close(); err != nil {
           setupLog.Error(err, "Failed to close audit store")
       }
   }
   ```

**EOD Deliverables**:
- ✅ All 4 audit helper methods implemented (GREEN phase)
- ✅ All unit tests passing
- ✅ Reconciler enhanced with 4 audit integration points
- ✅ Graceful shutdown with audit flush implemented
- ✅ Day 2 EOD report

**Validation Commands**:
```bash
# Verify tests pass (GREEN phase)
go test ./internal/controller/notification/audit_test.go -v

# Expected: All 4 tests should PASS

# Verify reconciler builds
go build ./internal/controller/notification/...
```

---

### **Day 3: Unit Tests (CHECK Phase)**

**Phase**: CHECK
**Duration**: 8 hours
**Focus**: Comprehensive unit test coverage for audit integration

**Morning (4 hours): Core Unit Tests**

1. **Expand unit tests** to 70%+ coverage (`audit_test.go`)

   **Test Categories**:

   a) **Event Creation Tests** (4 tests - one per event type)
   - Test `CreateMessageSentEvent` with valid notification
   - Test `CreateMessageFailedEvent` with error details
   - Test `CreateMessageAcknowledgedEvent` with acknowledgment data
   - Test `CreateMessageEscalatedEvent` with escalation level

   b) **Edge Case Tests** (6 tests)
   - Test with missing RemediationID (should use notification name as correlation_id)
   - Test with missing namespace (should handle gracefully)
   - Test with very long title (should truncate or handle)
   - Test with special characters in channel name
   - Test event_data serialization error handling
   - Test with multiple channels

   c) **ADR-034 Compliance Tests** (5 tests)
   - Test all required ADR-034 fields are populated
   - Test event_timestamp is set correctly
   - Test retention_days defaults to 2555 (7 years)
   - Test correlation_id matches remediation_id
   - Test event_data JSONB structure

2. **Behavior & Correctness Validation**
   - Tests validate WHAT the system does (creates correct audit events)
   - Clear business scenarios in test names (e.g., "should create audit event when notification sent to Slack")
   - Specific assertions (validate EventType, EventCategory, EventAction, EventOutcome)

**Afternoon (4 hours): Test Refinement + Reconciler Tests**

3. **Create reconciler unit tests** (`reconciler_audit_test.go`, ~200 LOC)
   ```go
   package notification

   import (
       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
       "github.com/jordigilh/kubernaut/pkg/audit"
       notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
   )

   var _ = Describe("NotificationRequestReconciler Audit Integration", func() {
       var (
           reconciler       *NotificationRequestReconciler
           mockAuditStore   *MockAuditStore
           notification     *notificationv1alpha1.NotificationRequest
       )

       BeforeEach(func() {
           mockAuditStore = NewMockAuditStore()
           reconciler = &NotificationRequestReconciler{
               AuditStore:   mockAuditStore,
               AuditHelpers: NewAuditHelpers("notification-controller"),
           }

           notification = &notificationv1alpha1.NotificationRequest{
               ObjectMeta: metav1.ObjectMeta{
                   Name:      "test-notification",
                   Namespace: "default",
               },
               Spec: notificationv1alpha1.NotificationRequestSpec{
                   RemediationID: "remediation-123",
                   Title:         "Test Alert",
                   Severity:      "critical",
                   Channels:      []notificationv1alpha1.NotificationChannel{"slack"},
               },
           }
       })

       Context("when notification delivery succeeds", func() {
           It("should write notification.message.sent audit event", func() {
               // BR-NOT-062: Unified audit table integration
               ctx := context.Background()

               reconciler.auditMessageSent(ctx, notification, "slack")

               Expect(mockAuditStore.GetStoredEvents()).To(HaveLen(1))
               event := mockAuditStore.GetStoredEvents()[0]

               Expect(event.EventType).To(Equal("notification.message.sent"))
               Expect(event.EventOutcome).To(Equal("success"))
               Expect(event.CorrelationID).To(Equal("remediation-123"))
           })
       })

       Context("when notification delivery fails", func() {
           It("should write notification.message.failed audit event with error details", func() {
               // BR-NOT-062: Unified audit table integration
               ctx := context.Background()
               deliveryErr := fmt.Errorf("Slack webhook returned 429: rate limited")

               reconciler.auditMessageFailed(ctx, notification, "slack", deliveryErr)

               Expect(mockAuditStore.GetStoredEvents()).To(HaveLen(1))
               event := mockAuditStore.GetStoredEvents()[0]

               Expect(event.EventType).To(Equal("notification.message.failed"))
               Expect(event.EventOutcome).To(Equal("failure"))
               Expect(event.ErrorMessage).ToNot(BeNil())
               Expect(*event.ErrorMessage).To(ContainSubstring("rate limited"))
           })
       })

       Context("when audit write fails", func() {
           It("should NOT block notification delivery (graceful degradation)", func() {
               // BR-NOT-063: Graceful audit degradation
               ctx := context.Background()
               mockAuditStore.SetError(fmt.Errorf("Data Storage Service unavailable"))

               // Should not panic or return error
               Expect(func() {
                   reconciler.auditMessageSent(ctx, notification, "slack")
               }).ToNot(Panic())

               // Audit failure should be logged but not propagate
           })
       })
   })
   ```

4. **Refactor tests** for clarity
   - Use `DescribeTable` for similar scenarios (e.g., test all 4 event types with same pattern)
   - Add business requirement comments (`// BR-NOT-062`, `// BR-NOT-063`, `// BR-NOT-064`)
   - Ensure tests survive implementation changes (test behavior, not implementation)

**EOD Deliverables**:
- ✅ 70%+ unit test coverage for audit integration
- ✅ All tests passing (15+ test specs)
- ✅ Tests follow behavior/correctness protocol
- ✅ BR references in all test descriptions
- ✅ Day 3 EOD report

**Validation Commands**:
```bash
# Run unit tests with coverage
go test ./internal/controller/notification/... -coverprofile=coverage.out
go tool cover -func=coverage.out | grep total

# Expected: total coverage ≥70% for audit.go and audit integration
```

---

### **Day 4: Integration + E2E Tests (CHECK Phase)**

**Phase**: CHECK
**Duration**: 8 hours
**Focus**: Component interaction validation and end-to-end workflow

**Morning (4 hours): Integration Test Implementation**

1. **Create integration tests** (`audit_integration_test.go`, ~400 LOC)

   **Integration Test Scenarios** (>50% coverage of integration points):

   a) **Data Storage Integration** (3 tests)
   ```go
   package notification

   import (
       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
       "github.com/jordigilh/kubernaut/pkg/audit"
       "github.com/jordigilh/kubernaut/test/integration/testutil"
   )

   var _ = Describe("Notification Audit Integration Tests", func() {
       var (
           testServer     *testutil.DataStorageTestServer
           auditStore     audit.AuditStore
           reconciler     *NotificationRequestReconciler
           notification   *notificationv1alpha1.NotificationRequest
       )

       BeforeEach(func() {
           // Start mock Data Storage Service
           testServer = testutil.NewDataStorageTestServer()
           testServer.Start()

           // Create audit store with real HTTP client
           dsClient := client.NewDataStorageClient(testServer.URL)
           auditStore, _ = audit.NewBufferedStore(
               dsClient,
               audit.Config{
                   BufferSize:    100,
                   BatchSize:     10,
                   FlushInterval: 100 * time.Millisecond,
                   MaxRetries:    3,
               },
               "notification",
               logger,
           )

           reconciler = &NotificationRequestReconciler{
               AuditStore:   auditStore,
               AuditHelpers: NewAuditHelpers("notification-controller"),
           }

           notification = createTestNotification()
       })

       AfterEach(func() {
           auditStore.Close()
           testServer.Stop()
       })

       Context("when notification is sent successfully", func() {
           It("should write audit event to Data Storage Service", func() {
               // BR-NOT-062: Unified audit table integration
               ctx := context.Background()

               // Audit message sent
               reconciler.auditMessageSent(ctx, notification, "slack")

               // Wait for async write (fire-and-forget pattern)
               Eventually(func() int {
                   return len(testServer.GetReceivedAuditEvents())
               }, "2s", "100ms").Should(Equal(1))

               // Verify audit event structure
               events := testServer.GetReceivedAuditEvents()
               Expect(events[0].EventType).To(Equal("notification.message.sent"))
               Expect(events[0].CorrelationID).To(Equal("remediation-123"))
           })
       })
   })
   ```

   b) **DLQ Fallback Integration** (2 tests)
   - Test audit write when Data Storage Service is unavailable (should use DLQ)
   - Test audit event recovery from DLQ after Data Storage Service recovers

   c) **Concurrent Delivery Integration** (1 test)
   - Test multiple simultaneous notifications writing audit events (no race conditions)

2. **Infrastructure setup**
   - Ensure test Data Storage Service mock is reliable
   - Add cleanup logic (`AfterEach` with async waits for flush)
   - Handle port collisions, resource conflicts

**Afternoon (4 hours): E2E Test Implementation**

3. **Create E2E tests** (`test/e2e/notification/audit_e2e_test.go`, ~300 LOC)

   **E2E Test Scenarios** (<10% coverage, critical paths only):

   a) **Complete Notification Lifecycle** (1 test)
   ```go
   package notification

   import (
       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
       "sigs.k8s.io/controller-runtime/pkg/client"
   )

   var _ = Describe("Notification Audit E2E Tests", func() {
       var (
           k8sClient          client.Client
           dataStorageClient  *client.DataStorageClient
           notificationCR     *notificationv1alpha1.NotificationRequest
       )

       BeforeEach(func() {
           k8sClient = testEnv.GetK8sClient()
           dataStorageClient = testEnv.GetDataStorageClient()

           notificationCR = &notificationv1alpha1.NotificationRequest{
               ObjectMeta: metav1.ObjectMeta{
                   Name:      "e2e-notification-test",
                   Namespace: "default",
               },
               Spec: notificationv1alpha1.NotificationRequestSpec{
                   RemediationID: "e2e-remediation-123",
                   Title:         "E2E Test Alert",
                   Severity:      "critical",
                   Channels:      []notificationv1alpha1.NotificationChannel{"console"},
               },
           }
       })

       AfterEach(func() {
           // Cleanup CRD
           k8sClient.Delete(context.Background(), notificationCR)
       })

       It("should write audit events for complete notification lifecycle", func() {
           // BR-NOT-062: Unified audit table integration
           // BR-NOT-064: Audit event correlation
           ctx := context.Background()

           // Create NotificationRequest CRD
           Expect(k8sClient.Create(ctx, notificationCR)).To(Succeed())

           // Wait for reconciliation and delivery
           Eventually(func() string {
               updatedCR := &notificationv1alpha1.NotificationRequest{}
               k8sClient.Get(ctx, client.ObjectKeyFromObject(notificationCR), updatedCR)
               return string(updatedCR.Status.Phase)
           }, "10s", "500ms").Should(Equal("Sent"))

           // Wait for audit events to be written (fire-and-forget + flush)
           time.Sleep(2 * time.Second)

           // Query Data Storage Service for audit events
           events, err := dataStorageClient.QueryAuditEvents(ctx, &client.AuditQuery{
               CorrelationID: "e2e-remediation-123",
               EventType:     "notification.message.sent",
           })

           Expect(err).ToNot(HaveOccurred())
           Expect(events).To(HaveLen(1), "Should have 1 notification.message.sent event")

           // Verify event correlation (BR-NOT-064)
           Expect(events[0].CorrelationID).To(Equal("e2e-remediation-123"))
           Expect(events[0].ResourceType).To(Equal("NotificationRequest"))
           Expect(events[0].ResourceID).To(Equal("e2e-notification-test"))
       })
   })
   ```

   b) **Audit Event Correlation** (1 test)
   - Test correlation_id links notification events to remediation events
   - Verify trace signal flow: Gateway → Orchestrator → Notification

**EOD Deliverables**:
- ✅ 6 integration test scenarios (>50% integration coverage)
- ✅ 2 E2E test scenarios (100% critical path coverage)
- ✅ All integration and E2E tests passing
- ✅ BR validation complete (BR-NOT-062, BR-NOT-063, BR-NOT-064)
- ✅ Day 4 EOD report

**Validation Commands**:
```bash
# Run integration tests
go test ./test/integration/notification/audit_integration_test.go -v

# Run E2E tests
go test ./test/e2e/notification/audit_e2e_test.go -v

# Expected: All tests pass, audit events written and queryable
```

---

### **Day 5: Documentation + Production Readiness (PRODUCTION Phase)**

**Phase**: PRODUCTION
**Duration**: 8 hours
**Focus**: Finalize documentation and production readiness

**📝 Note**: Most documentation is created **DURING** implementation (Days 1-4). This day is for **finalizing** and **consolidating** documentation.

---

#### **Morning (4 hours): Finalize Service Documentation**

**What to Update** (these are existing service docs, not new files):

1. **Update `README.md`** (Notification service main document)
   - Add "Unified Audit Table Integration" to Features section
   - Update version to v1.1 (audit integration enhancement)
   - Add changelog entry for ADR-034 integration
   - Update architecture diagram (add audit flow: Notification → Data Storage → audit_events)

2. **Update `BUSINESS_REQUIREMENTS.md`**
   - Add new BRs:
     - BR-NOT-062: Unified Audit Table Integration
     - BR-NOT-063: Graceful Audit Degradation
     - BR-NOT-064: Audit Event Correlation
   - Mark BRs as implemented with test coverage
   - Link to implementation files (`audit.go`, `notificationrequest_controller.go`)
   - Link to test files (`audit_test.go`, `audit_integration_test.go`, `audit_e2e_test.go`)

3. **Update `testing-strategy.md`**
   - Add test examples for audit integration
   - Document test coverage: Unit 70%+, Integration >50%, E2E 100% (critical paths)
   - Add testing patterns: mock AuditStore, async write verification, DLQ fallback testing

4. **Update `metrics-slos.md`** (audit adds new metrics via pkg/audit/)
   - Document new Prometheus metrics (inherited from pkg/audit/):
     - `audit_events_buffered_total{service="notification"}`
     - `audit_events_dropped_total{service="notification"}`
     - `audit_events_written_total{service="notification"}`
     - `audit_buffer_size{service="notification"}`
   - Add Grafana dashboard panels for notification audit metrics
   - Update SLI/SLO targets (audit write success rate >99%)

5. **Update `security-configuration.md`** (no RBAC changes needed)
   - Document audit event data sanitization (inherits from notification sanitization)
   - Note: Audit events respect is_sensitive flag for PII tracking

---

#### **Afternoon (4 hours): Operational Documentation**

**What to Create/Update**:

1. **Update Configuration Guide** (`config/notification-controller.yaml` inline comments)
   ```yaml
   # Notification Controller Configuration
   notification:
     # ... existing config ...

     # Audit Configuration (NEW - ADR-034 Integration)
     audit:
       # Data Storage Service URL for audit event writes
       # Required: Yes
       # Default: http://datastorage-service.kubernaut.svc.cluster.local:8080
       dataStorageURL: http://datastorage-service.kubernaut.svc.cluster.local:8080

       # Buffer size for audit events (fire-and-forget pattern)
       # Recommendation: 10,000 for standard load, 15,000 for high-volume notifications
       # Default: 10000
       bufferSize: 10000

       # Batch size for audit event writes (PostgreSQL optimization)
       # Recommendation: Keep at 1000 (optimal for pgx batch inserts)
       # Default: 1000
       batchSize: 1000

       # Flush interval for partial batches (milliseconds)
       # Recommendation: 1000ms balances latency and efficiency
       # Default: 1000
       flushIntervalMs: 1000

       # Max retry attempts for failed audit writes
       # Recommendation: 3 retries (1s, 4s, 9s backoff)
       # Default: 3
       maxRetries: 3
   ```

2. **Create Runbook** (NEW file: `docs/services/crd-controllers/06-notification/operations/audit-integration-runbook.md`)
   - Feature overview: ADR-034 unified audit table integration
   - Configuration guide: audit config fields, defaults, tuning recommendations
   - Operational procedures:
     - Monitor audit write success rate (target >99%)
     - Handle audit buffer full alerts (increase bufferSize)
     - Investigate audit write failures (check Data Storage Service health)
   - Troubleshooting guide:
     - Problem: "Audit events dropped" → Solution: Increase bufferSize, check write throughput
     - Problem: "Audit write latency high" → Solution: Check Data Storage Service performance
     - Problem: "Notifications blocked by audit writes" → Solution: Verify graceful degradation (should never happen)
   - Monitoring and alerting:
     - Alert: `audit_events_dropped_total{service="notification"} > 0` (CRITICAL)
     - Alert: `audit_buffer_size{service="notification"} / 10000 > 0.8` (WARNING - buffer 80% full)

3. **No Migration Guide Needed** (backward compatible enhancement)
   - Existing notification functionality unchanged
   - Audit integration is additive only
   - No breaking changes

---

#### **EOD Deliverables**:
- ✅ Service documentation updated (`README.md`, `BUSINESS_REQUIREMENTS.md`, `testing-strategy.md`, `metrics-slos.md`)
- ✅ Runbook created (`audit-integration-runbook.md`)
- ✅ Configuration documented (inline comments in YAML)
- ✅ All inline code documentation complete (GoDoc comments, BR references)
- ✅ Production readiness checklist complete
- ✅ Confidence assessment (85% → 95% after testing)
- ✅ Handoff summary created
- ✅ Day 5 EOD report (FINAL)

---

#### **Documentation Checklist**

**Service Documentation** (updates to existing files):
- [ ] `README.md` - Feature added, version bumped to v1.1, changelog updated
- [ ] `BUSINESS_REQUIREMENTS.md` - 3 new BRs documented (BR-NOT-062, BR-NOT-063, BR-NOT-064), links added
- [ ] `testing-strategy.md` - Test examples added (unit/integration/E2E)
- [ ] `metrics-slos.md` - 4 new audit metrics documented
- [ ] `security-configuration.md` - Audit data sanitization documented

**Operational Documentation** (new files):
- [ ] Runbook created (`audit-integration-runbook.md`)
- [ ] Configuration guide updated (inline comments in YAML)

**Code Documentation** (inline, created during implementation):
- [ ] GoDoc comments for all public APIs (`AuditHelpers`, audit helper methods)
- [ ] BR references in code comments (`// BR-NOT-062`, etc.)
- [ ] Complex logic explained (audit event creation, fire-and-forget pattern)
- [ ] Configuration fields documented (audit config in YAML)

**Test Documentation** (inline, created during testing):
- [ ] Test descriptions with business scenarios (e.g., "should write audit event when notification sent to Slack")
- [ ] BR mapping in test comments (`// BR-NOT-062`, `// BR-NOT-063`, `// BR-NOT-064`)
- [ ] Edge cases documented (missing fields, concurrent writes, DLQ fallback)
- [ ] Test helpers documented (`MockAuditStore`, test fixtures)

---

#### **Production Readiness Checklist**

**Build Validation**:
- [ ] Code builds without errors: `go build ./internal/controller/notification/...`
- [ ] All tests pass: `go test ./internal/controller/notification/... -v`
- [ ] No lint errors: `golangci-lint run ./internal/controller/notification/...`

**Test Coverage**:
- [ ] Unit tests: 70%+ coverage achieved
- [ ] Integration tests: >50% coverage achieved (6 scenarios)
- [ ] E2E tests: 100% critical path coverage (2 scenarios)
- [ ] BR validation: All 3 new BRs validated (BR-NOT-062, BR-NOT-063, BR-NOT-064)

**Documentation Completeness**:
- [ ] All service docs updated (README, BRs, testing, metrics, security)
- [ ] Runbook created with troubleshooting guide
- [ ] Configuration documented with inline comments
- [ ] All code has GoDoc comments and BR references

**Deployment Readiness**:
- [ ] Configuration validated (audit config in `notification-controller.yaml`)
- [ ] Graceful shutdown validated (audit flush on SIGTERM)
- [ ] Metrics validated (4 audit metrics exposed on /metrics)
- [ ] Backward compatibility confirmed (no breaking changes)

**Confidence Assessment**:
- **Initial Confidence**: 85% (shared library complete, notification controller production-ready)
- **Final Confidence**: 95% (after comprehensive testing and validation)
- **Justification**: Implementation follows established patterns from pkg/audit/ shared library. All 3 new BRs validated through comprehensive test coverage (unit/integration/E2E). Fire-and-forget pattern ensures zero business impact. Risk: Minor performance impact if audit buffer fills (mitigation: monitoring + alerting).

---

## 🧪 **TDD Do's and Don'ts - MANDATORY**

### **✅ DO: Strict TDD Discipline**

1. **Write ONE test at a time** (not batched)
   ```go
   // ✅ CORRECT: TDD Cycle 1
   It("should create notification.message.sent audit event with correct fields", func() {
       // Test for CreateMessageSentEvent
   })
   // Run test → FAIL (RED)
   // Implement CreateMessageSentEvent → PASS (GREEN)
   // Refactor if needed

   // ✅ CORRECT: TDD Cycle 2 (after Cycle 1 complete)
   It("should create notification.message.failed audit event with error details", func() {
       // Test for CreateMessageFailedEvent
   })
   ```

2. **Test WHAT the system does** (behavior), not HOW (implementation)
   ```go
   // ✅ CORRECT: Behavior-focused
   It("should write audit event when notification sent successfully", func() {
       reconciler.auditMessageSent(ctx, notification, "slack")

       // Verify WHAT happened (audit event written)
       Expect(mockAuditStore.GetStoredEvents()).To(HaveLen(1))
       Expect(mockAuditStore.GetStoredEvents()[0].EventType).To(Equal("notification.message.sent"))
   })
   ```

3. **Use specific assertions** (not weak checks)
   ```go
   // ✅ CORRECT: Specific business assertions
   Expect(event.EventType).To(Equal("notification.message.sent"))
   Expect(event.EventCategory).To(Equal("notification"))
   Expect(event.EventAction).To(Equal("sent"))
   Expect(event.EventOutcome).To(Equal("success"))
   Expect(event.CorrelationID).To(Equal("remediation-123"))
   ```

### **❌ DON'T: Anti-Patterns to Avoid**

1. **DON'T batch test writing**
   ```go
   // ❌ WRONG: Writing 4 tests before any implementation
   It("test CreateMessageSentEvent", func() { ... })
   It("test CreateMessageFailedEvent", func() { ... })
   It("test CreateMessageAcknowledgedEvent", func() { ... })
   It("test CreateMessageEscalatedEvent", func() { ... })
   // Then implementing all at once
   ```

2. **DON'T test implementation details**
   ```go
   // ❌ WRONG: Testing internal audit buffer state
   Expect(auditStore.buffer).To(HaveLen(1))
   Expect(auditStore.backgroundWorkerRunning).To(BeTrue())
   ```

3. **DON'T use weak assertions (NULL-TESTING)**
   ```go
   // ❌ WRONG: Weak assertions
   Expect(event).ToNot(BeNil())
   Expect(event.EventData).ToNot(BeEmpty())
   Expect(len(events)).To(BeNumerically(">", 0))
   ```

**Reference**: `.cursor/rules/08-testing-anti-patterns.mdc` for automated detection

---

## 📊 **Test Examples**

### **📦 Package Naming Conventions - MANDATORY**

**AUTHORITY**: [TEST_PACKAGE_NAMING_STANDARD.md](../../../testing/TEST_PACKAGE_NAMING_STANDARD.md)

**CRITICAL**: ALL tests use same package name as code under test (white-box testing).

| Test Type | Package Name | NO Exceptions |
|-----------|--------------|---------------|
| **Unit Tests** | `package notification` | ✅ |
| **Integration Tests** | `package notification` | ✅ |
| **E2E Tests** | `package notification` | ✅ |

**Key Rule**: **NEVER** use `_test` suffix for ANY test type.

---

### **Unit Test Example**

```go
package notification  // White-box testing - same package as code under test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/jordigilh/kubernaut/pkg/audit"
    notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Audit Helpers Unit Tests", func() {
    var (
        ctx          context.Context
        helpers      *AuditHelpers
        notification *notificationv1alpha1.NotificationRequest
    )

    BeforeEach(func() {
        ctx = context.Background()
        helpers = NewAuditHelpers("notification-controller")
        notification = &notificationv1alpha1.NotificationRequest{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "test-notification",
                Namespace: "default",
            },
            Spec: notificationv1alpha1.NotificationRequestSpec{
                RemediationID: "remediation-123",
                Title:         "Test Alert",
                Severity:      "critical",
                Channels:      []notificationv1alpha1.NotificationChannel{"slack"},
            },
        }
    })

    Context("when creating message sent audit event", func() {
        It("should create notification.message.sent event with all ADR-034 required fields", func() {
            // BR-NOT-062: Unified audit table integration
            // BUSINESS SCENARIO: Notification successfully delivered to Slack
            event, err := helpers.CreateMessageSentEvent(notification, "slack")

            // BEHAVIOR: Should create valid audit event
            Expect(err).ToNot(HaveOccurred(), "Should create audit event without error")
            Expect(event).ToNot(BeNil(), "Event should be created")

            // CORRECTNESS: Validate ADR-034 required fields
            Expect(event.EventType).To(Equal("notification.message.sent"))
            Expect(event.EventCategory).To(Equal("notification"))
            Expect(event.EventAction).To(Equal("sent"))
            Expect(event.EventOutcome).To(Equal("success"))
            Expect(event.ActorType).To(Equal("service"))
            Expect(event.ActorID).To(Equal("notification-controller"))
            Expect(event.ResourceType).To(Equal("NotificationRequest"))
            Expect(event.ResourceID).To(Equal("test-notification"))
            Expect(event.CorrelationID).To(Equal("remediation-123"))

            // Validate event_data structure
            Expect(event.EventData).ToNot(BeNil(), "Event data should be populated")

            // Validate optional fields
            Expect(event.Namespace).ToNot(BeNil())
            Expect(*event.Namespace).To(Equal("default"))

            // BUSINESS OUTCOME: Audit event ready for persistence to unified audit_events table
        })
    })

    Context("when creating message failed audit event", func() {
        It("should create notification.message.failed event with error details", func() {
            // BR-NOT-062: Unified audit table integration
            // BUSINESS SCENARIO: Notification delivery failed due to rate limiting
            deliveryErr := fmt.Errorf("Slack webhook returned 429: rate limited")

            event, err := helpers.CreateMessageFailedEvent(notification, "slack", deliveryErr)

            // BEHAVIOR: Should create audit event with error details
            Expect(err).ToNot(HaveOccurred())
            Expect(event).ToNot(BeNil())

            // CORRECTNESS: Validate failure event fields
            Expect(event.EventType).To(Equal("notification.message.failed"))
            Expect(event.EventOutcome).To(Equal("failure"))
            Expect(event.ErrorMessage).ToNot(BeNil())
            Expect(*event.ErrorMessage).To(ContainSubstring("rate limited"))

            // BUSINESS OUTCOME: Failed delivery tracked with detailed error for debugging
        })
    })
})
```

### **Integration Test Example**

```go
package notification  // White-box testing - same package as code under test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/jordigilh/kubernaut/pkg/audit"
    "github.com/jordigilh/kubernaut/test/integration/testutil"
)

var _ = Describe("Notification Audit Integration Tests", func() {
    var (
        ctx            context.Context
        testServer     *testutil.DataStorageTestServer
        auditStore     audit.AuditStore
        reconciler     *NotificationRequestReconciler
        notification   *notificationv1alpha1.NotificationRequest
    )

    BeforeEach(func() {
        ctx = context.Background()

        // Start mock Data Storage Service
        testServer = testutil.NewDataStorageTestServer()
        testServer.Start()

        // Create audit store with real HTTP client
        dsClient := client.NewDataStorageClient(testServer.URL)
        auditStore, _ = audit.NewBufferedStore(
            dsClient,
            audit.Config{BufferSize: 100, BatchSize: 10, FlushInterval: 100 * time.Millisecond},
            "notification",
            logger,
        )

        reconciler = &NotificationRequestReconciler{
            AuditStore:   auditStore,
            AuditHelpers: NewAuditHelpers("notification-controller"),
        }

        notification = createTestNotification()
    })

    AfterEach(func() {
        // Async cleanup with wait
        auditStore.Close()

        Eventually(func() bool {
            return testServer.IsIdle()
        }, "2s", "100ms").Should(BeTrue(), "Test server should be idle before cleanup")

        testServer.Stop()
    })

    Context("when notification is sent successfully", func() {
        It("should write audit event to Data Storage Service via HTTP", func() {
            // BR-NOT-062: Unified audit table integration
            // BUSINESS SCENARIO: Notification sent, audit event written asynchronously

            // BEHAVIOR: Audit write happens asynchronously (fire-and-forget)
            reconciler.auditMessageSent(ctx, notification, "slack")

            // Wait for async write (fire-and-forget pattern with flush)
            Eventually(func() int {
                return len(testServer.GetReceivedAuditEvents())
            }, "2s", "100ms").Should(Equal(1), "Audit event should be written within 2 seconds")

            // CORRECTNESS: Verify audit event structure
            events := testServer.GetReceivedAuditEvents()
            Expect(events[0].EventType).To(Equal("notification.message.sent"))
            Expect(events[0].CorrelationID).To(Equal("remediation-123"))

            // BUSINESS OUTCOME: Audit event persisted to unified audit_events table
        })
    })

    Context("when Data Storage Service is unavailable", func() {
        It("should use DLQ fallback without blocking notification delivery", func() {
            // BR-NOT-063: Graceful audit degradation
            // BUSINESS SCENARIO: Data Storage Service down, audit writes should fail gracefully

            // Stop Data Storage Service to simulate failure
            testServer.Stop()

            // BEHAVIOR: Audit write should NOT block or panic
            Expect(func() {
                reconciler.auditMessageSent(ctx, notification, "slack")
            }).ToNot(Panic())

            // Wait for retry attempts (3 retries with exponential backoff: 1s, 4s, 9s)
            time.Sleep(15 * time.Second)

            // CORRECTNESS: DLQ should contain failed audit event
            // (Note: Requires DLQ client integration - see DD-009)

            // BUSINESS OUTCOME: Notification delivery unaffected by audit write failure
        })
    })
})
```

### **E2E Test Example**

```go
package notification  // White-box testing - same package as code under test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Notification Audit E2E Tests", func() {
    var (
        ctx                context.Context
        k8sClient          client.Client
        dataStorageClient  *client.DataStorageClient
        notificationCR     *notificationv1alpha1.NotificationRequest
    )

    BeforeEach(func() {
        ctx = context.Background()
        k8sClient = testEnv.GetK8sClient()
        dataStorageClient = testEnv.GetDataStorageClient()

        notificationCR = &notificationv1alpha1.NotificationRequest{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "e2e-notification-test",
                Namespace: "default",
            },
            Spec: notificationv1alpha1.NotificationRequestSpec{
                RemediationID: "e2e-remediation-123",
                Title:         "E2E Test Alert",
                Severity:      "critical",
                Channels:      []notificationv1alpha1.NotificationChannel{"console"},
            },
        }
    })

    AfterEach(func() {
        // Cleanup CRD with async wait
        k8sClient.Delete(ctx, notificationCR)

        Eventually(func() bool {
            err := k8sClient.Get(ctx, client.ObjectKeyFromObject(notificationCR), &notificationv1alpha1.NotificationRequest{})
            return errors.IsNotFound(err)
        }, "10s", "500ms").Should(BeTrue(), "NotificationRequest CRD should be deleted")
    })

    It("should write audit events for complete notification lifecycle and support correlation", func() {
        // BR-NOT-062: Unified audit table integration
        // BR-NOT-064: Audit event correlation
        // BUSINESS SCENARIO: Complete notification workflow from creation to delivery

        // BEHAVIOR: Create NotificationRequest CRD, controller reconciles and delivers
        Expect(k8sClient.Create(ctx, notificationCR)).To(Succeed())

        // Wait for reconciliation and delivery
        Eventually(func() string {
            updatedCR := &notificationv1alpha1.NotificationRequest{}
            k8sClient.Get(ctx, client.ObjectKeyFromObject(notificationCR), updatedCR)
            return string(updatedCR.Status.Phase)
        }, "10s", "500ms").Should(Equal("Sent"), "Notification should be sent within 10 seconds")

        // Wait for audit events to be written (fire-and-forget + flush)
        time.Sleep(2 * time.Second)

        // CORRECTNESS: Query Data Storage Service for audit events by correlation_id
        events, err := dataStorageClient.QueryAuditEvents(ctx, &client.AuditQuery{
            CorrelationID: "e2e-remediation-123",
            EventType:     "notification.message.sent",
        })

        Expect(err).ToNot(HaveOccurred())
        Expect(events).To(HaveLen(1), "Should have 1 notification.message.sent event")

        // Verify event correlation (BR-NOT-064)
        Expect(events[0].CorrelationID).To(Equal("e2e-remediation-123"))
        Expect(events[0].ResourceType).To(Equal("NotificationRequest"))
        Expect(events[0].ResourceID).To(Equal("e2e-notification-test"))
        Expect(events[0].EventOutcome).To(Equal("success"))

        // BUSINESS OUTCOME: End-to-end notification workflow with complete audit trail
        // Audit events enable V2.0 Remediation Analysis Report generation
        // Trace signal flow: Gateway → Orchestrator → Notification (via correlation_id)
    })
})
```

---

## 🎯 **BR Coverage Matrix**

| BR ID | Description | Unit Tests | Integration Tests | E2E Tests | Status |
|-------|-------------|------------|-------------------|-----------|--------|
| **BR-NOT-062** | Unified Audit Table Integration | `audit_test.go:25-150`<br>(4 event creation tests) | `audit_integration_test.go:40-120`<br>(Data Storage write tests) | `audit_e2e_test.go:30-80`<br>(Complete lifecycle test) | ✅ PLANNED |
| **BR-NOT-063** | Graceful Audit Degradation | `reconciler_audit_test.go:60-85`<br>(Audit failure test) | `audit_integration_test.go:125-160`<br>(DLQ fallback test) | N/A (covered in integration) | ✅ PLANNED |
| **BR-NOT-064** | Audit Event Correlation | `audit_test.go:155-180`<br>(correlation_id validation) | N/A (covered in E2E) | `audit_e2e_test.go:30-80`<br>(Correlation query test) | ✅ PLANNED |

**Coverage Calculation**:
- **Unit**: 3/3 BRs covered (100%)
- **Integration**: 2/3 BRs covered (67%)
- **E2E**: 2/3 BRs covered (67%)
- **Total**: 3/3 BRs covered (100% - all BRs validated in at least 2 test tiers)

---

## 🚨 **Critical Pitfalls to Avoid**

### **1. Blocking Notification Delivery with Audit Writes**
- ❌ **Problem**: Audit write failures or slowness blocking notification reconciliation
- ✅ **Solution**: Use fire-and-forget pattern from `pkg/audit/` (non-blocking, async)
- **Impact**: BR-NOT-063 violation → notification delivery failures due to audit issues

### **2. Missing Correlation ID**
- ❌ **Problem**: Audit events not correlatable with remediation workflow
- ✅ **Solution**: Always set `CorrelationID = notification.Spec.RemediationID`
- **Impact**: BR-NOT-064 violation → cannot trace signal flow for V2.0 RAR

### **3. Incorrect Event Type Format**
- ❌ **Problem**: Using inconsistent event type format (e.g., "notification_sent" vs "notification.message.sent")
- ✅ **Solution**: Follow ADR-034 convention: `<service>.<category>.<action>` (e.g., "notification.message.sent")
- **Impact**: BR-NOT-062 violation → audit events not queryable, breaks cross-service correlation

---

## 📈 **Success Criteria**

### **Technical Success**
- ✅ All tests passing (Unit 70%+, Integration >50%, E2E 100% critical paths)
- ✅ No lint errors
- ✅ Audit store integrated with reconciler (fire-and-forget pattern)
- ✅ Graceful shutdown with audit flush implemented
- ✅ Documentation complete (service docs, runbook, config)

### **Business Success**
- ✅ BR-NOT-062 validated: All 4 event types written to unified audit_events table
- ✅ BR-NOT-063 validated: Audit failures don't block notification delivery
- ✅ BR-NOT-064 validated: Events correlatable via correlation_id
- ✅ Success metrics achieved:
  - Audit write latency <1ms (fire-and-forget)
  - Audit success rate >99% (with DLQ fallback)
  - Zero notification delivery failures due to audit writes

### **Confidence Assessment**
- **Target**: ≥85% confidence
- **Calculation**: Evidence-based (shared library 95% complete + notification controller production-ready + comprehensive test coverage)
- **Final Confidence**: 95% (after Day 4 testing complete)

---

## 🔄 **Rollback Plan**

### **Rollback Triggers**
- Critical bug discovered in audit integration (e.g., audit writes blocking delivery)
- Performance degradation >5% due to audit writes (should be <1ms overhead)
- Audit write failures >1% (should be <0.1% with DLQ)

### **Rollback Procedure**
1. **Disable audit writes via feature flag** (if implemented)
   - Set `audit.enabled: false` in `notification-controller.yaml`
   - Restart controller with updated config
2. **OR Revert to previous version**
   - Deploy previous Notification Controller version (v1.0 without audit integration)
   - Verify notification delivery unaffected
3. **Verify rollback success**
   - Check notification delivery success rate (should return to >99%)
   - Verify no audit write errors in logs
4. **Document rollback reason**
   - Create incident report with root cause analysis
   - Update implementation plan with lessons learned

**Rollback Duration**: <5 minutes (config change + pod restart)

---

## 📚 **References**

### **Design Decisions**
- [ADR-034: Unified Audit Table Design](../../../architecture/decisions/ADR-034-unified-audit-table-design.md) - Unified audit_events table schema
- [ADR-038: Asynchronous Buffered Audit Ingestion](../../../architecture/decisions/ADR-038-async-buffered-audit-ingestion.md) - Fire-and-forget pattern
- [DD-AUDIT-002: Audit Shared Library Design](../../../architecture/decisions/DD-AUDIT-002-audit-shared-library-design.md) - pkg/audit/ implementation

### **Standards**
- [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc) - Testing framework (defense-in-depth)
- [02-go-coding-standards.mdc](../../../../.cursor/rules/02-go-coding-standards.mdc) - Go patterns
- [08-testing-anti-patterns.mdc](../../../../.cursor/rules/08-testing-anti-patterns.mdc) - Testing anti-patterns
- [TESTING_GUIDELINES.md](../../../../docs/development/business-requirements/TESTING_GUIDELINES.md) - BR test classification

### **Service Documentation**
- [Notification Service README](./README.md) - Service overview
- [BUSINESS_REQUIREMENTS.md](./BUSINESS_REQUIREMENTS.md) - Existing BRs (BR-NOT-050 to BR-NOT-061)
- [audit-trace-specification.md](./audit-trace-specification.md) - OLD audit spec (pre-ADR-034, uses notification_audit table)

### **Implementation Resources**
- [AUDIT_TRACES_FINAL_STATUS.md](../../../../AUDIT_TRACES_FINAL_STATUS.md) - Shared library status (95% complete)
- [pkg/audit/README.md](../../../../pkg/audit/README.md) - Shared library API documentation
- [pkg/audit/store.go](../../../../pkg/audit/store.go) - BufferedAuditStore implementation (320 LOC)

---

**Document Status**: 📋 **DRAFT (Awaiting User Approval)**
**Last Updated**: 2025-11-21
**Version**: 1.0
**Maintained By**: Kubernaut Development Team

---

## 📝 **User Approval Required**

**Before proceeding to Day 1 implementation, please confirm**:

1. ✅ **Plan Approval**: Do you approve this 5-day implementation plan?
2. ✅ **BR Approval**: Do you approve the 3 new business requirements (BR-NOT-062, BR-NOT-063, BR-NOT-064)?
3. ✅ **Timeline Approval**: Is the 5-day timeline acceptable (2 days implementation + 2 days testing + 1 day documentation)?
4. ✅ **Scope Confirmation**: Implement all 4 notification event types (sent, failed, acknowledged, escalated)?
5. ✅ **Priority**: Should this be implemented before or after Data Storage DLQ tests (Category B from earlier discussion)?

**Please respond with**:
- **APPROVED** - Proceed with Day 1 implementation
- **MODIFY** - Suggest changes to scope, timeline, or approach
- **DEFER** - Complete Data Storage DLQ tests first (Category B)

---

**Ready to implement? Awaiting your approval to proceed! 🚀**

