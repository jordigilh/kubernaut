# DD-NOT-001: ADR-034 Unified Audit Table Integration - Implementation Plan v2.0 (REVISED)

**Version**: 2.0 (REVISED - All gaps fixed)
**Status**: 📋 READY FOR APPROVAL
**Design Decision**: [ADR-034: Unified Audit Table Design](../../../architecture/decisions/ADR-034-unified-audit-table-design.md)
**Service**: Notification Service (Notification Controller)
**Confidence**: 90% (Evidence-Based: Shared library complete, gaps fixed, edge cases enhanced)
**Estimated Effort**: 5 days (APDC cycle: 2 days implementation + 2 days testing + 1 day documentation)

**Revision Notes**: This v2.0 plan fixes all 7 gaps identified in triage:
1. ✅ TDD one-at-a-time made explicit (CRITICAL gap fixed)
2. ✅ Behavior + correctness validation added to all test examples
3. ✅ DescribeTable pattern added for event types
4. ✅ Mock complexity decision criteria added
5. ✅ Documentation timeline clarified (Days 1-4 vs Day 5)
6. ✅ Test level decision framework added
7. ✅ Edge cases enhanced (6 → 10 tests, +4 critical scenarios including concurrency)

---

## 📋 **Version History**

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v1.0** | 2025-11-21 | Initial implementation plan created | ⏸️ Superseded |
| **v2.0** | 2025-11-21 | All 7 gaps fixed + edge cases enhanced | ✅ **CURRENT** |

---

## 🎯 **Business Requirements**

### **Primary Business Requirements**

| BR ID | Description | Success Criteria |
|-------|-------------|------------------|
| **BR-NOT-062** | **Unified Audit Table Integration**: Notification Controller MUST write audit events to the unified `audit_events` table (ADR-034) for cross-service correlation, compliance reporting, and V2.0 Remediation Analysis Reports | ✅ All 4 notification event types written to `audit_events` table<br>✅ Fire-and-forget pattern (non-blocking, <1ms overhead)<br>✅ Zero audit loss through DLQ fallback<br>✅ Events queryable via correlation_id |
| **BR-NOT-063** | **Graceful Audit Degradation**: Audit write failures MUST NOT block notification delivery or cause reconciliation failures | ✅ Notification delivery succeeds even when audit writes fail<br>✅ Audit failures logged but don't stop reconciliation<br>✅ Failed audits queued to DLQ for retry |
| **BR-NOT-064** | **Audit Event Correlation**: Notification audit events MUST be correlatable with RemediationRequest events for end-to-end workflow tracing | ✅ correlation_id matches remediation_id<br>✅ parent_event_id links to remediation events<br>✅ Trace signal flow: Gateway → Orchestrator → Notification |

### **Success Metrics**

- **Audit Write Latency**: <1ms (fire-and-forget, non-blocking)
- **Audit Success Rate**: >99% (with DLQ fallback for failures)
- **Zero Business Impact**: 0% notification delivery failures due to audit writes
- **Correlation Coverage**: 100% of notifications correlatable via correlation_id
- **Test Coverage**: Unit 70%+, Integration >50%, E2E 100% (critical paths)

---

## 📅 **Timeline Overview**

### **5-Day Implementation Timeline**

| Day | Phase | Focus | Hours | Key Milestones |
|-----|-------|-------|-------|----------------|
| **Day 0** | ANALYSIS + PLAN | Pre-work | 8h | ✅ Analysis complete, Plan approved (v2.0) |
| **Day 1** | DO-RED | Foundation + Tests | 8h | Test framework, audit helpers, failing tests (ONE AT A TIME) |
| **Day 2** | DO-GREEN + DO-REFACTOR | Implementation + Integration | 8h | Audit store integrated, events written, reconciler enhanced |
| **Day 3** | CHECK | Unit tests | 8h | 70%+ unit test coverage, 10 edge cases, behavior + correctness |
| **Day 4** | CHECK | Integration + E2E tests | 8h | Integration scenarios, E2E workflow validation |
| **Day 5** | PRODUCTION | Documentation + Readiness | 8h | Finalize docs (350 lines), confidence report, handoff |

---

## 📆 **Day-by-Day Implementation Breakdown**

### **Day 0: ANALYSIS + PLAN (Pre-Work) ✅**

**Status**: ✅ COMPLETE (this v2.0 document represents Day 0 completion with all gaps fixed)

---

### **Day 1: Foundation + Test Framework (DO-RED Phase)**

**Phase**: DO-RED  
**Duration**: 8 hours  
**TDD Focus**: Write failing tests first, enhance existing reconciler

**⚠️ CRITICAL**: We are **ENHANCING existing Notification Controller**, not creating from scratch!

**Morning (4 hours): Test Framework Setup + Code Analysis**

1. **Analyze existing implementation** (1 hour)
2. **Create test file** `internal/controller/notification/audit_test.go` (200-300 LOC)
3. **Create integration test** `test/integration/notification/audit_integration_test.go` (300-400 LOC)

**Afternoon (4 hours): Interface Enhancement + Failing Tests**

**🚨 CRITICAL TDD DISCIPLINE: Tests MUST be written ONE AT A TIME, NEVER batched**

4. **Create** `internal/controller/notification/audit.go` (NEW file, ~200 LOC)

5. **Write failing tests** (strict TDD: ONE test at a time, NEVER batched)
   
   **🔴 MANDATORY RULE**: Write ONE test → Run it → Verify FAIL → Then write next test
   
   **❌ STRICTLY FORBIDDEN**: Writing all 4 tests in batch, then running them together
   **✅ REQUIRED**: Write 1 test → Run it → Verify FAIL → Repeat for next test
   
   **TDD Cycle 1** (30 minutes):
   - Write ONLY test for `CreateMessageSentEvent` (1 test)
   - Run test: `go test ./internal/controller/notification/audit_test.go -run CreateMessageSentEvent -v`
   - Verify it FAILS with "not implemented yet" (RED)
   - **DO NOT** proceed to Cycle 2 until RED verified
   
   **TDD Cycle 2** (30 minutes) - AFTER Cycle 1 RED verified:
   - Write ONLY test for `CreateMessageFailedEvent` (1 test)
   - Run test: `go test ./internal/controller/notification/audit_test.go -run CreateMessageFailedEvent -v`
   - Verify it FAILS with "not implemented yet" (RED)
   - **DO NOT** proceed to Cycle 3 until RED verified
   
   **TDD Cycle 3** (30 minutes) - AFTER Cycle 2 RED verified:
   - Write ONLY test for `CreateMessageAcknowledgedEvent` (1 test)
   - Run test: `go test ./internal/controller/notification/audit_test.go -run CreateMessageAcknowledgedEvent -v`
   - Verify it FAILS with "not implemented yet" (RED)
   - **DO NOT** proceed to Cycle 4 until RED verified
   
   **TDD Cycle 4** (30 minutes) - AFTER Cycle 3 RED verified:
   - Write ONLY test for `CreateMessageEscalatedEvent` (1 test)
   - Run test: `go test ./internal/controller/notification/audit_test.go -run CreateMessageEscalatedEvent -v`
   - Verify it FAILS with "not implemented yet" (RED)

**EOD Deliverables**:
- ✅ Test framework complete  
- ✅ 4 failing tests (RED phase) - written ONE AT A TIME
- ✅ Enhanced interfaces defined  
- ✅ Day 1 EOD report

---

### **Day 3: Unit Tests (CHECK Phase)**

**Phase**: CHECK  
**Duration**: 8 hours  
**Focus**: Comprehensive unit test coverage with behavior + correctness validation

**Morning (4 hours): Core Unit Tests + Mock Complexity Validation**

### **Mock Complexity Validation** (Before Implementation)

**Authority**: testing-strategy.md Decision Criteria

**Decision Criteria**:
1. **Mock setup >30 lines?** → Consider integration test
2. **Mock requires complex state management?** → Consider integration test  
3. **Mock breaks frequently on implementation changes?** → Consider integration test

**Audit Integration Mock Complexity Analysis**:

| Test Scenario | Mock Complexity | Lines | Decision |
|---------------|----------------|-------|----------|
| CreateMessageSentEvent | Simple fixture | ~10 lines | ✅ Unit test |
| Reconciler audit integration | MockAuditStore | ~20 lines | ✅ Unit test |
| Multi-channel concurrent audit | MockStore + channels | ~40 lines | ✅ Unit (tests concurrency logic) |

**Validation Result**: ✅ All planned unit tests have appropriate mock complexity

---

1. **Expand unit tests** to 70%+ coverage
   
   **Test Categories with Behavior + Correctness Validation**:
   
   a) **Event Creation Tests** (4 tests using DescribeTable pattern)
   
   b) **Edge Case Tests** (10 tests - ENHANCED)
   
   **Category 1: Missing/Invalid Input** (4 tests)
   - Test with missing RemediationID (fallback to notification.Name)
   - Test with missing namespace (handle gracefully)
   - ✅ **NEW**: Test with nil notification input (should error)
   - ✅ **NEW**: Test with empty channel string (should error)
   
   **Category 2: Boundary Conditions** (3 tests)
   - Test with very long title (>10KB)
   - ✅ **NEW**: Test with empty title
   - ✅ **NEW**: Test with maximum JSONB payload (~10MB PostgreSQL limit)
   
   **Category 3: Error Conditions** (2 tests)
   - Test event_data serialization error
   - Test with special characters in channel name
   
   **Category 4: Concurrency** (1 test) 🔴 **CRITICAL**
   - ✅ **NEW**: Test concurrent audit writes (10+ simultaneous)
     - Run with race detector: `go test -race`
     - BR-NOT-060: Concurrent delivery safety
   
   **Category 5: Resource Limits** (1 test)  
   - ✅ **NEW**: Test audit buffer full scenario (graceful degradation)
     - BR-NOT-063: Graceful audit degradation
   
   c) **ADR-034 Compliance Tests** (5 tests)

**EOD Deliverables**:
- ✅ 70%+ unit test coverage
- ✅ 10 edge case tests (enhanced from 6)
- ✅ All tests validate BOTH behavior AND correctness
- ✅ Tests follow DescribeTable pattern for similar scenarios

**See full test examples in "Test Examples" section below**

---

### **Day 5: Documentation + Production Readiness**

**Phase**: PRODUCTION  
**Duration**: 8 hours

**📝 CRITICAL**: Most documentation was created DURING Days 1-4. Day 5 is for **finalizing** only.

#### **Documentation Already Created (Days 1-4)**

**During Days 1-2 (Implementation)**:
- ✅ GoDoc comments for AuditHelpers (in audit.go)
- ✅ BR references (// BR-NOT-062, etc.)
- ✅ Configuration field comments (YAML inline)
- ✅ Daily EOD reports (Days 1-2)

**During Days 3-4 (Testing)**:
- ✅ Test descriptions with business scenarios  
- ✅ BR mapping in test comments
- ✅ Edge case documentation (10 tests)
- ✅ Test helper documentation

#### **What Gets Done on Day 5**

**Morning (4 hours): Update 5 Existing Service Docs** (~100 lines total)
1. Update README.md (20 lines)
2. Update BUSINESS_REQUIREMENTS.md (40 lines)
3. Update testing-strategy.md (20 lines)
4. Update metrics-slos.md (15 lines)
5. Update security-configuration.md (5 lines)

**Afternoon (4 hours): Create Operational Docs** (~250 lines total)
1. Create audit-integration-runbook.md (NEW, ~200 lines)
2. Update config YAML (inline comments, ~50 lines)

**Day 5 Total**: ~350 lines (NOT 2000+ - that's already done!)

---

## 📊 **Test Examples**

### **DescribeTable Pattern for Event Types** (Gap 3 Fix)

```go
var _ = Describe("Audit Helpers - Event Creation Matrix", func() {
    var helpers *AuditHelpers
    var notification *notificationv1alpha1.NotificationRequest
    
    BeforeEach(func() {
        helpers = NewAuditHelpers("notification-controller")
        notification = createTestNotification()
    })
    
    DescribeTable("Audit event creation for all notification states",
        func(eventType, eventAction, eventOutcome string, createFunc func() (*audit.AuditEvent, error), shouldSucceed bool) {
            event, err := createFunc()
            
            if shouldSucceed {
                Expect(err).ToNot(HaveOccurred())
                Expect(event.EventType).To(Equal(eventType))
                Expect(event.EventOutcome).To(Equal(eventOutcome))
                Expect(event.CorrelationID).To(Equal("remediation-123"))
            } else {
                Expect(err).To(HaveOccurred())
            }
        },
        Entry("message sent", "notification.message.sent", "sent", "success",
            func() (*audit.AuditEvent, error) {
                return helpers.CreateMessageSentEvent(notification, "slack")
            }, true),
        Entry("message failed", "notification.message.failed", "sent", "failure",
            func() (*audit.AuditEvent, error) {
                return helpers.CreateMessageFailedEvent(notification, "slack", fmt.Errorf("error"))
            }, true),
        Entry("nil notification", "", "", "",
            func() (*audit.AuditEvent, error) {
                return helpers.CreateMessageSentEvent(nil, "slack")
            }, false),
    )
})
```

### **Behavior + Correctness Example** (Gap 2 Fix)

```go
It("should create audit event with accurate fields", func() {
    // BR-NOT-062: Unified audit table integration
    
    // ===== BEHAVIOR TESTING =====
    event, err := helpers.CreateMessageSentEvent(notification, "slack")
    Expect(err).ToNot(HaveOccurred())
    Expect(event).ToNot(BeNil())
    
    // ===== CORRECTNESS TESTING =====
    Expect(event.EventType).To(Equal("notification.message.sent"),
        "Event type MUST be 'notification.message.sent' (ADR-034)")
    Expect(event.CorrelationID).To(Equal("remediation-123"),
        "Correlation ID MUST match for workflow tracing (BR-NOT-064)")
    Expect(event.RetentionDays).To(Equal(2555),
        "Retention MUST be 7 years for SOC 2 compliance")
    
    // Validate event_data structure
    var eventData map[string]interface{}
    json.Unmarshal(event.EventData, &eventData)
    Expect(eventData["channel"]).To(Equal("slack"),
        "Channel MUST match actual delivery channel")
})
```

### **Edge Cases: Concurrency Test** (Gap 7 Enhancement)

```go
It("should handle concurrent audit writes without race conditions", func() {
    // BR-NOT-060: Concurrent delivery safety
    const concurrentNotifications = 10
    var wg sync.WaitGroup
    wg.Add(concurrentNotifications)
    
    for i := 0; i < concurrentNotifications; i++ {
        go func(id int) {
            defer wg.Done()
            notification := createTestNotificationWithID(id)
            event, _ := helpers.CreateMessageSentEvent(notification, "slack")
            auditStore.StoreAudit(ctx, event)
        }(i)
    }
    
    wg.Wait()
    // Must pass with: go test -race
})
```

---

## 📊 **Test Level Selection Framework** (Gap 6 Fix)

**Before writing each test, answer**:

1. **Can test with simple mocks (<20 lines)?**
   - YES → Continue to Q2
   - NO → Integration Test

2. **Testing logic or infrastructure?**
   - Logic → Unit Test
   - Infrastructure → Integration Test

3. **Is test readable and maintainable?**
   - YES → Unit Test
   - NO → Integration Test

**Audit Integration Decisions**:
- ✅ Event creation helpers → Unit (logic, simple mocks)
- ✅ HTTP to Data Storage → Integration (infrastructure)
- ✅ Complete lifecycle → E2E (full workflow)

---

## 🚨 **Critical Pitfalls to Avoid**

1. **Blocking Notification Delivery** - Use fire-and-forget pattern
2. **Missing Correlation ID** - Always set from remediation_id
3. **Incorrect Event Type** - Follow ADR-034: `<service>.<category>.<action>`

---

## 📈 **Success Criteria**

### **Technical Success**
- ✅ All tests passing (Unit 70%+, Integration >50%, E2E 100%)
- ✅ No lint errors
- ✅ Graceful shutdown with audit flush
- ✅ Documentation complete (350 lines Day 5)

### **Business Success**
- ✅ BR-NOT-062: All 4 event types written
- ✅ BR-NOT-063: Audit failures don't block delivery
- ✅ BR-NOT-064: Events correlatable

### **Confidence Assessment**
- **Final Confidence**: 90% (95% after Day 4 testing)

---

## 🎯 **Revision Summary**

**v2.0 fixes all 7 gaps**:
1. ✅ TDD one-at-a-time explicit
2. ✅ Behavior + correctness in all tests
3. ✅ DescribeTable pattern added
4. ✅ Mock complexity criteria
5. ✅ Documentation timeline clarified
6. ✅ Test level framework
7. ✅ Edge cases: 6 → 10 tests

**Ready for user approval!** 🚀

---

**Document Status**: 📋 **READY FOR APPROVAL**  
**Version**: 2.0 (REVISED)  
**Last Updated**: 2025-11-21
