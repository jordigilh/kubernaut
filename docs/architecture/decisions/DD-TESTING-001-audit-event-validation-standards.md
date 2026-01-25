# DD-TESTING-001: Audit Event Validation Standards

**Status**: ✅ **APPROVED** (Authoritative Standard)
**Date**: January 2, 2026
**Last Reviewed**: January 2, 2026
**Version**: 1.0
**Confidence**: 98%
**Authority Level**: SYSTEM-WIDE - Mandatory for all service test suites

---

## 🎯 **Overview**

This design decision establishes **mandatory standards for validating audit events in all test tiers** (unit, integration, E2E). It provides authoritative guidance to prevent common audit validation bugs that hide critical issues.

**Key Principle**: Audit event validation must be **deterministic, type-safe, and comprehensive**. Tests must validate the **exact expected count** of each event type and their **structured content** using OpenAPI-generated clients.

**Scope**: All services that generate audit events (per DD-AUDIT-003):
- Gateway Service
- Signal Processing Controller
- AI Analysis Controller
- Remediation Execution Controller
- Remediation Orchestrator Controller
- Notification Service

**Decision Summary**:
- ✅ **MANDATORY**: Use OpenAPI-generated audit client (DD-API-001 compliance)
- ✅ **MANDATORY**: Validate exact event counts per event type (deterministic)
- ✅ **MANDATORY**: Validate event_data structured content (per DD-AUDIT-004)
- ❌ **FORBIDDEN**: Raw HTTP calls to Data Storage API
- ❌ **FORBIDDEN**: Non-deterministic count validation (`BeNumerically(">=")`)
- ❌ **FORBIDDEN**: `time.Sleep()` for event polling (use `Eventually()`)

---

## 📋 **Table of Contents**

1. [Context & Problem](#context--problem)
2. [Requirements](#requirements)
3. [Decision](#decision)
4. [Mandatory Validation Patterns](#mandatory-validation-patterns)
5. [Anti-Patterns (FORBIDDEN)](#anti-patterns-forbidden)
6. [Implementation Examples](#implementation-examples)
7. [Test Tier Requirements](#test-tier-requirements)
8. [Related Decisions](#related-decisions)

---

## 🎯 **Context & Problem**

### **Challenge**

During AI Analysis E2E test development (January 2026), critical audit validation bugs were discovered:

1. ⚠️ **DD-API-001 Violations**: Tests used raw HTTP instead of OpenAPI client
2. ⚠️ **Non-Deterministic Validation**: Tests used `BeNumerically(">=", N)` hiding duplicate events
3. ⚠️ **time.Sleep() Violations**: Tests used blocking sleeps instead of `Eventually()`
4. ⚠️ **Incomplete Event Validation**: Tests only checked event existence, not content
5. ⚠️ **Hidden Bugs**: All 4 issues above allowed tests to "pass" while hiding real bugs

### **Business Impact**

- **Compliance Risk**: Audit events missing or duplicated, violating SOC 2/ISO 27001
- **Debugging Failures**: Incomplete audit trails prevent root cause analysis
- **False Confidence**: Tests pass but audit system is broken
- **Production Incidents**: Audit gaps discovered only in production
- **Tech Debt**: Non-standard patterns across 6 services

### **Key Question**

**What are the mandatory standards for validating audit events in all test tiers?**

---

## 📋 **Requirements**

### **Functional Requirements**

| Requirement | Description | Priority |
|-------------|-------------|----------|
| **Type Safety** | Use OpenAPI-generated client for all audit queries | ⭐⭐⭐⭐⭐ |
| **Deterministic Counts** | Validate exact expected count per event type | ⭐⭐⭐⭐⭐ |
| **Structured Content** | Validate event_data fields per DD-AUDIT-004 | ⭐⭐⭐⭐⭐ |
| **Event Category** | Validate event_category matches service | ⭐⭐⭐⭐ |
| **Event Outcome** | Validate event_outcome (success/failure) | ⭐⭐⭐⭐ |
| **Correlation ID** | Validate all events share correlation_id | ⭐⭐⭐⭐ |
| **Timestamp Validation** | Validate event_timestamp is set and valid | ⭐⭐⭐ |

### **Non-Functional Requirements**

| Requirement | Description | Priority |
|-------------|-------------|----------|
| **No time.Sleep()** | Use `Eventually()` for async polling | ⭐⭐⭐⭐⭐ |
| **No Raw HTTP** | DD-API-001 compliance mandatory | ⭐⭐⭐⭐⭐ |
| **Consistent Patterns** | Same validation approach across all services | ⭐⭐⭐⭐ |
| **Maintainability** | Reusable helper functions for common patterns | ⭐⭐⭐ |

---

## 🎯 **Decision**

### **Decision Statement**

**ALL service tests (unit, integration, E2E) MUST validate audit events using the following mandatory patterns:**

1. **OpenAPI Client Usage** (DD-API-001):
   - Use `dsgen.ClientWithResponses` for all Data Storage queries
   - Use `dsgen.QueryAuditEventsParams` for type-safe parameter passing
   - Never use raw HTTP calls to Data Storage API

2. **Deterministic Count Validation**:
   - Use `Equal(N)` to validate exact expected count per event type
   - Never use `BeNumerically(">=", N)` (hides duplicate events)
   - Count events by `event_type` to detect missing/duplicate events

3. **Structured Content Validation**:
   - Validate `event_data` fields match DD-AUDIT-004 payload schemas
   - Validate `event_category` matches service category
   - Validate `event_outcome` reflects operation result
   - Validate `correlation_id` is consistent across events

4. **Async Polling Pattern**:
   - Use `Eventually()` with polling for event appearance
   - Never use `time.Sleep()` to wait for events
   - Timeout after reasonable duration (30-60s)

### **Confidence Assessment**

**Confidence**: 98%

**Justification**:
- ✅ Proven effective during AIAnalysis E2E test development
- ✅ Prevents all 4 classes of bugs discovered in original implementation
- ✅ Aligns with DD-API-001 (OpenAPI client mandate)
- ✅ Aligns with DD-AUDIT-004 (structured event_data)
- ✅ Follows Ginkgo/Gomega best practices (`Eventually()` over `time.Sleep()`)
- ⚠️ Minor risk: Adds initial setup complexity for test infrastructure

---

## 🔧 **Mandatory Validation Patterns**

### **Pattern 1: OpenAPI Client Setup (DD-API-001 Compliance)**

**MANDATORY**: Initialize OpenAPI-generated client in test suite setup.

```go
// ✅ CORRECT: OpenAPI client initialization (test/e2e/aianalysis/suite_test.go)
var (
    dsClient *dsgen.ClientWithResponses // OpenAPI-generated client
)

var _ = BeforeSuite(func() {
    // Initialize Data Storage OpenAPI client
    dataStorageURL := "http://localhost:8091"
    var err error
    dsClient, err = dsgen.NewClientWithResponses(dataStorageURL)
    if err != nil {
        Fail(fmt.Sprintf("DD-API-001 violation: Cannot proceed without DataStorage client: %v", err))
    }
})
```

**❌ FORBIDDEN: Raw HTTP client**

```go
// ❌ FORBIDDEN: Direct HTTP bypasses OpenAPI spec validation
httpClient := &http.Client{}
queryURL := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s", dataStorageURL, correlationID)
resp, err := httpClient.Get(queryURL)
```

**Violation Detection**: Pre-commit hook MUST reject any test code using `http.Client` for Data Storage queries.

---

### **Pattern 2: Type-Safe Query Helper**

**MANDATORY**: Create helper function using OpenAPI-generated types.

```go
// ✅ CORRECT: Type-safe query helper
func queryAuditEvents(
    correlationID string,
    eventType *string,
) ([]dsgen.AuditEvent, error) {
    limit := 100

    params := &dsgen.QueryAuditEventsParams{
        CorrelationId: &correlationID,
        Limit:         &limit,
    }

    if eventType != nil {
        params.EventType = eventType
    }

    resp, err := dsClient.QueryAuditEventsWithResponse(context.Background(), params)
    if err != nil {
        return nil, fmt.Errorf("failed to query DataStorage: %w", err)
    }

    if resp.JSON200 == nil {
        return nil, fmt.Errorf("DataStorage returned non-200: %d", resp.StatusCode())
    }

    if resp.JSON200.Data == nil {
        return []dsgen.AuditEvent{}, nil
    }

    return *resp.JSON200.Data, nil
}
```

---

### **Pattern 3: Async Event Polling with Eventually()**

**MANDATORY**: Use `Eventually()` to poll for events, never `time.Sleep()`.

```go
// ✅ CORRECT: Eventually() with polling
func waitForAuditEvents(
    correlationID string,
    eventType string,
    minCount int,
) []dsgen.AuditEvent {
    var events []dsgen.AuditEvent

    Eventually(func() int {
        var err error
        events, err = queryAuditEvents(correlationID, &eventType)
        if err != nil {
            GinkgoWriter.Printf("⏳ Audit query error: %v\n", err)
            return 0
        }
        return len(events)
    }, 60*time.Second, 2*time.Second).Should(BeNumerically(">=", minCount),
        fmt.Sprintf("Should have at least %d %s events for correlation %s", minCount, eventType, correlationID))

    return events
}
```

**❌ FORBIDDEN: time.Sleep() polling**

```go
// ❌ FORBIDDEN: Blocking sleep is non-deterministic and slow
time.Sleep(15 * time.Second)
events, _ := queryAuditEvents(correlationID, &eventType)
```

---

### **Pattern 4: Deterministic Event Count Validation**

**MANDATORY**: Validate exact expected count per event type using `Equal()`.

```go
// ✅ CORRECT: Deterministic count validation per event type
It("should audit all AIAnalysis lifecycle events", func() {
    // ... create AIAnalysis and wait for completion ...

    By("Querying all audit events for this analysis")
    allEvents, err := queryAuditEvents(remediationID, nil)
    Expect(err).NotTo(HaveOccurred())

    By("Counting events by event_type")
    eventCounts := countEventsByType(allEvents)

    By("Validating exact expected counts per event type")
    // Per DD-AUDIT-003: AIAnalysis emits exactly these events
    Expect(eventCounts["aianalysis.phase.transition"]).To(Equal(3),
        "Expected exactly 3 phase transitions: Pending→Investigating→Analyzing→Completed")
    Expect(eventCounts["aianalysis.rego.evaluation"]).To(Equal(1),
        "Expected exactly 1 Rego evaluation per analysis")
    Expect(eventCounts["aianalysis.approval.decision"]).To(Equal(1),
        "Expected exactly 1 approval decision per analysis")
    Expect(eventCounts["aianalysis.analysis.completed"]).To(Equal(1),
        "Expected exactly 1 completion event")
})

// Helper function to count events by type
func countEventsByType(events []dsgen.AuditEvent) map[string]int {
    counts := make(map[string]int)
    for _, event := range events {
        counts[event.EventType]++
    }
    return counts
}
```

**❌ FORBIDDEN: Non-deterministic count validation**

```go
// ❌ FORBIDDEN: BeNumerically(">=") hides duplicate events
Expect(len(phaseEvents)).To(BeNumerically(">=", 3),
    "Should have at least 3 phase transition events") // Could be 3, 4, 5... BUG HIDDEN!
```

---

### **Pattern 5: Structured event_data Validation**

**MANDATORY**: Validate event_data fields per DD-AUDIT-004 payload schemas.

```go
// ✅ CORRECT: Validate structured event_data fields
It("should validate Rego evaluation event_data structure", func() {
    // ... wait for Rego evaluation event ...

    regoEvents := waitForAuditEvents(remediationID, "aianalysis.rego.evaluation", 1)
    event := regoEvents[0]

    // Cast event_data to map for validation
    eventData, ok := event.EventData.(map[string]interface{})
    Expect(ok).To(BeTrue(), "event_data should be a JSON object")

    // Per DD-AUDIT-004: RegoEvaluationPayload structure
    Expect(eventData).To(HaveKey("outcome"), "Should record policy outcome")
    Expect(eventData).To(HaveKey("degraded"), "Should record degraded mode flag")
    Expect(eventData).To(HaveKey("duration_ms"), "Should record evaluation duration")

    // Validate field values
    outcome := eventData["outcome"].(string)
    Expect([]string{"approved", "requires_approval"}).To(ContainElement(outcome))

    degraded := eventData["degraded"].(bool)
    Expect(degraded).To(BeFalse(), "Rego should not run in degraded mode in E2E")

    durationMs := int(eventData["duration_ms"].(float64))
    Expect(durationMs).To(BeNumerically(">", 0), "Duration should be positive")
})
```

---

### **Pattern 6: Top-Level Optional Field Validation**

**MANDATORY**: Validate top-level optional fields (duration_ms, error_code, error_message) when business requirements specify them.

**Why This Pattern?**
Services emit audit data in **TWO locations**:
1. **Top-level fields** (database columns: `duration_ms`, `error_code`, `error_message`)
2. **event_data payload** (JSONB: structured per DD-AUDIT-004)

Both MUST be validated to ensure complete audit trail integrity.

```go
// ✅ CORRECT: Validate top-level DurationMs field (BR-SP-090: Performance Tracking)
It("should capture enrichment duration at top-level for performance tracking", func() {
    // ... wait for enrichment event ...

    enrichmentEvents := waitForAuditEvents(correlationID, "signalprocessing.enrichment.completed", 1)
    event := enrichmentEvents[0]

    // Validate top-level duration_ms field (stored in database column)
    durationMs, hasDuration := event.DurationMs.Get()
    Expect(hasDuration).To(BeTrue(), "BR-SP-090: Duration MUST be captured for performance tracking")
    Expect(durationMs).To(BeNumerically(">", 0), "Duration should be positive")

    // ALSO validate duration in event_data payload (per DD-AUDIT-004)
    payload := event.EventData.SignalProcessingEnrichmentPayload
    Expect(payload.DurationMs.Value).To(Equal(durationMs), "Top-level and payload durations should match")
})
```

**❌ FORBIDDEN: Only validating event_data payload**

```go
// ❌ INCOMPLETE: Only checks payload, misses top-level field (database bug could go undetected)
payload := event.EventData.AIAnalysisHolmesGPTCallPayload
Expect(payload.DurationMs).To(BeNumerically(">", 0)) // event.DurationMs NOT validated!
```

**When to Apply This Pattern**:
- ✅ **Performance Tracking** (BR-SP-090): Validate `duration_ms` for timed operations
- ✅ **Error Tracking**: Validate `error_code` and `error_message` for failure events
- ✅ **Query API Compliance**: Ensures DataStorage Query API returns all fields

**Detected Bug**: SignalProcessing tests discovered DataStorage Query API was missing `duration_ms`, `error_code`, `error_message` from SELECT clause (January 11, 2026). AIAnalysis tests only validated payload, missing the database-level bug.

---

### **Pattern 7: Event Category and Outcome Validation**

**MANDATORY**: Validate event_category and event_outcome for all events.

```go
// ✅ CORRECT: Validate event metadata
It("should validate event category and outcome", func() {
    // ... wait for events ...

    allEvents, err := queryAuditEvents(remediationID, nil)
    Expect(err).NotTo(HaveOccurred())

    for _, event := range allEvents {
        // Validate event_category matches service
        Expect(string(event.EventCategory)).To(Equal("analysis"),
            "AIAnalysis events must have event_category='analysis'")

        // Validate event_outcome is valid
        outcome := string(event.EventOutcome)
        Expect([]string{"success", "failure"}).To(ContainElement(outcome),
            "event_outcome must be 'success' or 'failure'")

        // Validate timestamp is set
        Expect(event.EventTimestamp).NotTo(BeZero(),
            "event_timestamp must be set")

        // Validate correlation_id matches
        Expect(event.CorrelationId).To(Equal(remediationID),
            "All events must share the same correlation_id")
    }
})
```

---

## 🚫 **Anti-Patterns (FORBIDDEN)**

### **Anti-Pattern 1: Raw HTTP Instead of OpenAPI Client**

**❌ FORBIDDEN** (DD-API-001 Violation)

```go
// ❌ FORBIDDEN: Bypasses type safety and spec validation
httpClient := &http.Client{}
queryURL := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s", dataStorageURL, correlationID)
resp, err := httpClient.Get(queryURL)
```

**Why Forbidden**:
- Bypasses OpenAPI spec validation (hides contract bugs)
- No compile-time type checking
- Schema drift undetected
- Violates DD-API-001 (mandatory OpenAPI client usage)

**Enforcement**: Pre-commit hook MUST reject any test using `http.Client` for Data Storage.

---

### **Anti-Pattern 2: Non-Deterministic Count Validation**

**❌ FORBIDDEN** (Hides Duplicate Events)

```go
// ❌ FORBIDDEN: Hides duplicate/missing events
Expect(len(regoEvents)).To(BeNumerically(">=", 1)) // Could be 1, 2, 3... BUG HIDDEN!
```

**Why Forbidden**:
- Hides duplicate event bugs (test passes with 2 events when 1 expected)
- Hides missing event bugs (test passes with 0 events if other events exist)
- Non-deterministic tests create false confidence

**Correct Pattern**: Use `Equal(N)` for exact expected count per event type.

---

### **Anti-Pattern 3: time.Sleep() for Event Polling**

**❌ FORBIDDEN** (TESTING_GUIDELINES.md Violation)

```go
// ❌ FORBIDDEN: Blocking sleep is slow and non-deterministic
time.Sleep(15 * time.Second)
events, _ := queryAuditEvents(correlationID, &eventType)
Expect(events).ToNot(BeEmpty())
```

**Why Forbidden**:
- Non-deterministic (events may not appear in fixed time)
- Slows test execution (fixed 15s wait even if events appear in 1s)
- Violates Ginkgo/Gomega best practices

**Correct Pattern**: Use `Eventually()` with polling interval.

---

### **Anti-Pattern 4: Weak Null-Testing Assertions**

**❌ FORBIDDEN** (Null-Testing Anti-Pattern)

```go
// ❌ FORBIDDEN: Weak assertions don't validate business logic
Expect(events).ToNot(BeEmpty()) // Only checks existence
Expect(len(events)).To(BeNumerically(">", 0)) // Only checks count > 0
Expect(event.EventData).ToNot(BeNil()) // Only checks not nil
```

**Why Forbidden**:
- Doesn't validate business requirements (exact count, correct fields)
- Allows tests to "pass" with incomplete/duplicate events
- Violates TDD principle of validating business outcomes

**Correct Pattern**: Validate exact expected values and structured content.

---

### **Anti-Pattern 5: Missing event_data Field Validation**

**❌ FORBIDDEN** (Incomplete Validation)

```go
// ❌ FORBIDDEN: Only checks event existence, not content
It("should audit Rego evaluation", func() {
    regoEvents := waitForAuditEvents(remediationID, "aianalysis.rego.evaluation", 1)
    Expect(regoEvents).ToNot(BeEmpty()) // Missing event_data validation!
})
```

**Why Forbidden**:
- Doesn't verify DD-AUDIT-004 payload schema compliance
- Event could have empty/incorrect event_data and test passes
- Incomplete audit trail validation

**Correct Pattern**: Validate all required event_data fields per DD-AUDIT-004.

---

### **Anti-Pattern 6: Invalid event_category Enum Values in Unit Tests**

**❌ FORBIDDEN** (OpenAPI Validation Failure)

```go
// ❌ FORBIDDEN: Uses invalid event_category value
func createTestEvent() *ogenclient.AuditEventRequest {
    event := audit.NewAuditEventRequest()
    audit.SetEventCategory(event, "test") // INVALID - not in OpenAPI enum!
    // ... rest of event setup
    return event
}
```

**Why Forbidden**:
- **OpenAPI validation rejects invalid enum values** at runtime
- Tests fail with validation errors instead of testing business logic
- Since embedded OpenAPI specs are regenerated for server-side validation, **ALL audit events** (including test fixtures) are validated against the schema
- Using placeholder values like `"test"` causes cryptic validation errors

**Impact**: Tests that were passing before OpenAPI schema regeneration will suddenly fail with:
```
ERROR: Invalid audit event (OpenAPI validation)
Error at "/event_category": value is not one of the allowed values
Value: "test"
```

**Valid `event_category` Enum Values** (per OpenAPI schema):
- `"gateway"` - Gateway Service
- `"notification"` - Notification Service
- `"analysis"` - AI Analysis Service
- `"signalprocessing"` - Signal Processing Service
- `"workflow"` - Workflow Catalog Service
- `"workflowexecution"` - WorkflowExecution Controller
- `"orchestration"` - Remediation Orchestrator Service
- `"webhook"` - Authentication Webhook Service

**Correct Pattern**:
```go
// ✅ CORRECT: Uses valid event_category from OpenAPI enum
func createTestEvent() *ogenclient.AuditEventRequest {
    event := audit.NewAuditEventRequest()
    audit.SetEventCategory(event, "gateway") // DD-TESTING-001: Valid enum value
    // ... rest of event setup
    return event
}
```

**Enforcement**: Unit tests MUST use valid enum values for all OpenAPI-validated fields.

---

## 💡 **Implementation Examples**

### **Example 1: AIAnalysis E2E Full Audit Trail Test**

```go
var _ = Describe("Audit Trail E2E", Label("e2e", "audit"), func() {
    It("should audit complete AIAnalysis lifecycle", func() {
        By("Creating AIAnalysis CR")
        suffix := randomSuffix()
        namespace := createTestNamespace("audit-lifecycle")
        analysis := &aianalysisv1alpha1.AIAnalysis{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "e2e-audit-" + suffix,
                Namespace: namespace,
            },
            Spec: aianalysisv1alpha1.AIAnalysisSpec{
                RemediationID: "rr-" + suffix,
                Severity:      "critical",
                Environment:   "development",
            },
        }
        Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

        By("Waiting for reconciliation to complete")
        Eventually(func() string {
            _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
            return string(analysis.Status.Phase)
        }, 10*time.Second, 500*time.Millisecond).Should(Equal("Completed"))

        remediationID := analysis.Spec.RemediationID

        By("Querying all audit events for this analysis")
        allEvents, err := queryAuditEvents(remediationID, nil)
        Expect(err).NotTo(HaveOccurred())
        Expect(allEvents).ToNot(BeEmpty(), "Should have audit events")

        By("Counting events by event_type (deterministic validation)")
        eventCounts := countEventsByType(allEvents)

        By("Validating exact expected counts per event type")
        Expect(eventCounts["aianalysis.phase.transition"]).To(Equal(3),
            "Expected exactly 3 phase transitions")
        Expect(eventCounts["aianalysis.rego.evaluation"]).To(Equal(1),
            "Expected exactly 1 Rego evaluation")
        Expect(eventCounts["aianalysis.approval.decision"]).To(Equal(1),
            "Expected exactly 1 approval decision")
        Expect(eventCounts["aianalysis.analysis.completed"]).To(Equal(1),
            "Expected exactly 1 completion event")

        By("Validating event category for all events")
        for _, event := range allEvents {
            Expect(string(event.EventCategory)).To(Equal("analysis"),
                "All AIAnalysis events must have event_category='analysis'")
        }

        By("Validating Rego evaluation event_data structure")
        regoEvents := waitForAuditEvents(remediationID, "aianalysis.rego.evaluation", 1)
        regoEvent := regoEvents[0]
        eventData, ok := regoEvent.EventData.(map[string]interface{})
        Expect(ok).To(BeTrue())

        // Per DD-AUDIT-004: RegoEvaluationPayload
        Expect(eventData).To(HaveKey("outcome"))
        Expect(eventData).To(HaveKey("degraded"))
        Expect(eventData).To(HaveKey("duration_ms"))

        outcome := eventData["outcome"].(string)
        Expect([]string{"approved", "requires_approval"}).To(ContainElement(outcome))
    })
})
```

---

## 🧪 **Test Tier Requirements**

### **Unit Tests (Business Logic Layer)**

**Scope**: Test audit client method calls, not actual Data Storage persistence.

**MANDATORY Validations**:
- ✅ Verify audit methods called with correct parameters
- ✅ Validate event_data payload structure matches DD-AUDIT-004
- ✅ Verify error handling for audit failures
- ✅ **Use valid `event_category` enum values** (see below)

**⚠️ CRITICAL: Valid Event Category Values**

Unit tests that create audit events **MUST** use valid `event_category` values from the OpenAPI schema enum:
- `"gateway"` - Gateway Service
- `"notification"` - Notification Service
- `"analysis"` - AI Analysis Service
- `"signalprocessing"` - Signal Processing Service
- `"workflow"` - Workflow Catalog Service
- `"workflowexecution"` - WorkflowExecution Controller
- `"orchestration"` - Remediation Orchestrator Service
- `"webhook"` - Authentication Webhook Service

**❌ FORBIDDEN**: Using placeholder values like `"test"` will cause OpenAPI validation failures.

**Rationale**: Since we regenerate embedded OpenAPI specs for server-side validation, all audit events (including test fixtures) are validated against the schema. Using invalid enum values causes tests to fail with validation errors.

**Example Fix**:
```go
// ❌ BAD - Uses invalid event_category
audit.SetEventCategory(event, "test")

// ✅ GOOD - Uses valid event_category from enum
audit.SetEventCategory(event, "gateway") // DD-TESTING-001: Valid enum value
```

**Example**:

```go
It("should call audit client with correct Rego evaluation payload", func() {
    mockAuditClient := testutil.NewMockAuditClient()
    handler := NewAnalyzingHandler(mockAuditClient, regoEvaluator)

    // Execute handler logic
    err := handler.Handle(ctx, analysis)
    Expect(err).NotTo(HaveOccurred())

    // Verify audit method called
    Expect(mockAuditClient.RecordRegoEvaluationCalled).To(BeTrue())

    // Verify payload structure
    payload := mockAuditClient.LastRegoPayload
    Expect(payload.Outcome).To(Equal("approved"))
    Expect(payload.Degraded).To(BeFalse())
    Expect(payload.DurationMs).To(BeNumerically(">", 0))
})
```

---

### **Integration Tests (Component + Infrastructure)**

**Scope**: Test audit events persisted to Data Storage (real PostgreSQL).

**MANDATORY Validations**:
- ✅ Use OpenAPI client to query Data Storage
- ✅ Validate events persisted to database
- ✅ Validate event counts per event type (deterministic)
- ✅ Validate event_data structure per DD-AUDIT-004

**Example**:

```go
It("should persist Rego evaluation events to Data Storage", func() {
    // Integration test setup with real Data Storage + PostgreSQL
    analysis := createTestAIAnalysis()

    Eventually(func() string {
        _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
        return string(analysis.Status.Phase)
    }, 10*time.Second).Should(Equal("Completed"))

    // Query Data Storage using OpenAPI client
    regoEvents := waitForAuditEvents(analysis.Spec.RemediationID, "aianalysis.rego.evaluation", 1)

    // Validate exact count
    Expect(len(regoEvents)).To(Equal(1))

    // Validate event_data structure
    eventData := regoEvents[0].EventData.(map[string]interface{})
    Expect(eventData).To(HaveKey("outcome"))
    Expect(eventData).To(HaveKey("degraded"))
})
```

---

### **E2E Tests (Full System in Kind Cluster)**

**Scope**: Test complete audit trail in production-like environment.

**MANDATORY Validations**:
- ✅ Use OpenAPI client for all Data Storage queries
- ✅ Validate complete audit trail (all expected events present)
- ✅ Validate exact event counts per event type
- ✅ Validate event_data structure per DD-AUDIT-004
- ✅ Validate event_category and event_outcome
- ✅ Validate correlation_id consistency across events
- ✅ Validate timestamps are set and valid

**Example**: See "Example 1: AIAnalysis E2E Full Audit Trail Test" above.

---

## 🔗 **Related Decisions**

| Decision | Relationship | Description |
|----------|-------------|-------------|
| **DD-API-001** | MANDATORY PREREQUISITE | OpenAPI client mandatory for REST API communication |
| **DD-AUDIT-003** | DEFINES SCOPE | Specifies which services generate audit events |
| **DD-AUDIT-004** | DEFINES STRUCTURE | Structured types for event_data payloads |
| **DD-AUDIT-002** | INFRASTRUCTURE | Audit shared library design |
| **ADR-032** | FOUNDATIONAL | Data Access Layer isolation |
| **ADR-034** | SCHEMA AUTHORITY | Unified audit table design |
| **ADR-038** | IMPLEMENTATION | Async buffered audit ingestion |

---

## 📊 **Success Metrics**

### **Compliance Metrics**

| Metric | Target | Measurement |
|--------|--------|-------------|
| **OpenAPI Client Usage** | 100% | All audit queries use `dsgen.ClientWithResponses` |
| **Deterministic Counts** | 100% | All event count validations use `Equal(N)` |
| **Structured Validation** | 100% | All tests validate event_data per DD-AUDIT-004 |
| **No time.Sleep()** | 100% | All async polling uses `Eventually()` |
| **No Raw HTTP** | 100% | Zero raw HTTP calls to Data Storage in tests |

### **Quality Metrics**

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Bug Detection Rate** | >95% | Tests catch audit bugs before production |
| **False Negative Rate** | <1% | Tests don't pass when audit is broken |
| **Test Maintainability** | High | Standard patterns reduce maintenance burden |

---

## 🚀 **Implementation Checklist**

### **For New Tests** (MANDATORY)

- [ ] Initialize OpenAPI client in test suite setup
- [ ] Create `queryAuditEvents()` helper using OpenAPI types
- [ ] Create `waitForAuditEvents()` helper with `Eventually()`
- [ ] Create `countEventsByType()` helper for deterministic counts
- [ ] Validate exact event counts per event type using `Equal(N)`
- [ ] Validate event_data structure per DD-AUDIT-004
- [ ] Validate event_category and event_outcome
- [ ] Validate correlation_id consistency
- [ ] No raw HTTP calls to Data Storage
- [ ] No `time.Sleep()` for event polling
- [ ] Add test to CI pipeline

### **For Existing Tests** (MIGRATION)

- [ ] Replace raw HTTP with OpenAPI client
- [ ] Replace `BeNumerically(">=")` with `Equal()` for event counts
- [ ] Replace `time.Sleep()` with `Eventually()`
- [ ] Add event_data structure validation
- [ ] Add event_category and event_outcome validation
- [ ] Verify all tests pass with stricter validation

---

## 📝 **Appendix: Full Helper Function Reference**

### **A1: queryAuditEvents() - Type-Safe Query**

```go
// queryAuditEvents queries Data Storage for audit events using OpenAPI client.
//
// Parameters:
//   - correlationID: Correlation ID to filter events
//   - eventType: Optional event type filter
//
// Returns: Array of audit events (OpenAPI-generated types)
func queryAuditEvents(
    correlationID string,
    eventType *string,
) ([]dsgen.AuditEvent, error) {
    limit := 100

    params := &dsgen.QueryAuditEventsParams{
        CorrelationId: &correlationID,
        Limit:         &limit,
    }

    if eventType != nil {
        params.EventType = eventType
    }

    resp, err := dsClient.QueryAuditEventsWithResponse(context.Background(), params)
    if err != nil {
        return nil, fmt.Errorf("failed to query DataStorage: %w", err)
    }

    if resp.JSON200 == nil {
        return nil, fmt.Errorf("DataStorage returned non-200: %d", resp.StatusCode())
    }

    if resp.JSON200.Data == nil {
        return []dsgen.AuditEvent{}, nil
    }

    return *resp.JSON200.Data, nil
}
```

### **A2: waitForAuditEvents() - Async Polling**

```go
// waitForAuditEvents polls Data Storage until events appear.
//
// Parameters:
//   - correlationID: Correlation ID to filter events
//   - eventType: Event type to filter
//   - minCount: Minimum expected count
//
// Returns: Array of audit events
func waitForAuditEvents(
    correlationID string,
    eventType string,
    minCount int,
) []dsgen.AuditEvent {
    var events []dsgen.AuditEvent

    Eventually(func() int {
        var err error
        events, err = queryAuditEvents(correlationID, &eventType)
        if err != nil {
            GinkgoWriter.Printf("⏳ Audit query error: %v\n", err)
            return 0
        }
        return len(events)
    }, 60*time.Second, 2*time.Second).Should(BeNumerically(">=", minCount),
        fmt.Sprintf("Should have at least %d %s events for correlation %s", minCount, eventType, correlationID))

    return events
}
```

### **A3: countEventsByType() - Deterministic Counts**

```go
// countEventsByType counts occurrences of each event type in the given events.
//
// Returns: map[eventType]count
func countEventsByType(events []dsgen.AuditEvent) map[string]int {
    counts := make(map[string]int)
    for _, event := range events {
        counts[event.EventType]++
    }
    return counts
}
```

---

## 🎯 **Conclusion**

This design decision establishes **mandatory, authoritative standards** for validating audit events across all Kubernaut services. These patterns prevent common bugs, ensure DD-API-001 compliance, and provide deterministic, maintainable test suites.

**Key Takeaways**:
- ✅ **Always** use OpenAPI-generated client (DD-API-001)
- ✅ **Always** validate exact event counts per type (`Equal(N)`)
- ✅ **Always** validate event_data structure (DD-AUDIT-004)
- ✅ **Always** use `Eventually()` for async polling
- ❌ **Never** use raw HTTP for Data Storage queries
- ❌ **Never** use `BeNumerically(">=")` for event counts
- ❌ **Never** use `time.Sleep()` for event polling

**Enforcement**: Pre-commit hooks MUST reject code violating these standards.

**Next Steps**:
1. Migrate all existing tests to these patterns
2. Update CI pipeline to enforce DD-API-001 compliance
3. Add pre-commit hooks to reject anti-patterns
4. Document service-specific event types in DD-AUDIT-003

---

**Document Metadata**:
- **Authors**: Kubernaut Core Team
- **Reviewers**: QA Team, AIAnalysis Team, Notification Team
- **Related ADRs**: ADR-032, ADR-034, ADR-038
- **Related DDs**: DD-API-001, DD-AUDIT-002, DD-AUDIT-003, DD-AUDIT-004
- **Version History**: v1.0 (2026-01-02) - Initial authoritative standard

