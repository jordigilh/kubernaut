# SOC2 Audit - Implementation Plan

**Version**: 1.1.0
**Created**: December 20, 2025
**Last Updated**: January 4, 2026
**Status**: Authoritative - Ready to Start
**Authority**: [AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025](../../handoff/AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md)
**Business Requirement**: BR-AUDIT-005 v2.0 (SOC 2 Type II + RR Reconstruction)
**Test Plan**: [SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md](./SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md)
**Priority**: **P0** - V1.0 Release Blocker

---

## 📋 **Changelog**

### Version 1.1.0 (2026-01-04)
- ✅ **MOVED**: From `docs/handoff/` to `docs/development/SOC2/` (authoritative location)
- ✅ **UPDATED**: Cross-references to test plan in new location
- ✅ **UPDATED**: Effort estimates to include test implementation time

### Version 1.0.0 (2025-12-20)
- Initial implementation plan

---

## 🎯 **Executive Summary**

**Goal**: SOC2 Type II MVP compliance with RemediationRequest reconstruction capability

**User Requirements (Dec 18, 2025)**:
- ✅ **SOC2 Type II compliance** - Enterprise audit framework (proof of concept)
- ✅ **RR Reconstruction** - 100% accurate reconstruction from audit traces
- ⚠️ **MVP Approach** - Don't over-deliver, extend based on feedback

**Key Principle**: "SOC2 is just enough to proof that we can deliver an enterprise audit framework"

---

## 🧪 **Development Methodology - APDC-TDD (MANDATORY)**

**CRITICAL**: This implementation MUST follow APDC-enhanced TDD methodology per workspace rules.

### **APDC Framework**

| Phase | Duration | Purpose | Deliverable |
|-------|----------|---------|-------------|
| **Analyze** | 5-10 min | Understand existing patterns, identify gaps | Gap analysis, existing code review |
| **Plan** | 10-15 min | Design test scenarios and implementation approach | Test plan reference, acceptance criteria |
| **Do-RED** | 40% of implementation time | Write FAILING tests FIRST | Failing test suite |
| **Do-GREEN** | 40% of implementation time | Minimal implementation to pass tests | Passing test suite |
| **Do-REFACTOR** | 20% of implementation time | Enhance code quality | Optimized, clean code |
| **Check** | 5-10 min | Validate business requirements achieved | Validation checklist |

### **TDD Workflow - MANDATORY SEQUENCE**

**Every day MUST follow this order:**

```
Analyze → Plan → RED (tests first) → GREEN (implementation) → REFACTOR → Check
```

**Example: Day 1 (Gateway Signal Data)**

**WRONG Order** ❌:
```
1. Add fields to struct
2. Update event emission
3. Manual testing
4. Write tests  ← TOO LATE!
```

**CORRECT Order** ✅:
```
1. Analyze: Review existing Gateway audit code (5 min)
2. Plan: Design test scenarios from test plan (10 min)
3. RED: Write failing integration tests (3 hours)
   - Test expects `original_payload` field
   - Test expects `signal_labels` field
   - Test expects `signal_annotations` field
   - Run tests → ALL FAIL (expected)
4. GREEN: Add fields and update emission (4 hours)
   - Add `original_payload` to struct
   - Add `signal_labels` to struct
   - Add `signal_annotations` to struct
   - Update event emission
   - Run tests → ALL PASS
5. REFACTOR: Optimize if needed (1 hour)
6. Check: Verify Gap #1-3 closed (10 min)
```

### **TDD Validation Commands**

**After RED phase** (tests MUST fail):
```bash
# Day 1: Gateway tests should fail (no implementation yet)
go test ./test/integration/gateway/audit_signal_data_integration_test.go -v -p 4
# Expected: FAIL (this is CORRECT in RED phase)
```

**After GREEN phase** (tests MUST pass):
```bash
# Day 1: Gateway tests should pass (implementation complete)
go test ./test/integration/gateway/audit_signal_data_integration_test.go -v -p 4
# Expected: PASS (this is CORRECT in GREEN phase)
```

---

## 🚫 **CRITICAL: Forbidden Test Patterns**

### **time.Sleep() is ABSOLUTELY FORBIDDEN**

**MANDATORY**: ALL asynchronous operations MUST use `Eventually()`, NEVER `time.Sleep()`.

**Rationale**: `time.Sleep()` causes flaky tests, slow test suites, and false confidence.

#### **❌ FORBIDDEN Pattern**:
```go
// ❌ WRONG: Sleeping to wait for audit event
time.Sleep(5 * time.Second)
events, _ := dsClient.QueryAuditEvents(correlationID)
Expect(len(events)).To(BeNumerically(">=", 1))  // May fail on slow CI
```

#### **✅ REQUIRED Pattern**:
```go
// ✅ CORRECT: Eventually() with deterministic validation
Eventually(func() int {
    events, err := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
        EventType:     &eventType,
        CorrelationId: &correlationID,
    })
    if err != nil || events.JSON200 == nil {
        return 0
    }
    return *events.JSON200.Pagination.Total
}, 30*time.Second, 1*time.Second).Should(Equal(1),
    "Should find exactly 1 audit event within 30 seconds")
```

#### **Timeout Guidelines**:

| Test Tier | Typical Timeout | Interval | Rationale |
|-----------|-----------------|----------|-----------|
| Integration | 30-60 seconds | 1-2 seconds | Real K8s API + Data Storage |
| E2E | 2-5 minutes | 5-10 seconds | Full infrastructure + network delays |

#### **Why No Exceptions?**

1. ✅ **Reliability**: `Eventually()` retries until condition is met
2. ✅ **Speed**: Returns immediately when condition satisfied
3. ✅ **Clarity**: Failure messages show what condition was not met
4. ✅ **CI Stability**: Works across different machine speeds

---

### **Skip() is ABSOLUTELY FORBIDDEN**

**MANDATORY**: Tests MUST fail when dependencies are unavailable, NEVER skip.

**Rationale**: Skipped tests show "green" but don't validate anything.

#### **❌ FORBIDDEN Pattern**:
```go
// ❌ WRONG: Skipping when Data Storage unavailable
BeforeEach(func() {
    resp, err := http.Get(dataStorageURL + "/health")
    if err != nil {
        Skip("Data Storage not available")  // ← FORBIDDEN!
    }
})
```

#### **✅ REQUIRED Pattern**:
```go
// ✅ CORRECT: Fail with clear error message
BeforeEach(func() {
    resp, err := http.Get(dataStorageURL + "/health")
    Expect(err).ToNot(HaveOccurred(),
        "REQUIRED: Data Storage not available at %s\n"+
        "  Per DD-AUDIT-003: This service MUST have audit capability\n"+
        "  Start with: podman-compose -f podman-compose.test.yml up -d",
        dataStorageURL)
    Expect(resp.StatusCode).To(Equal(http.StatusOK))
})
```

#### **Why No Exceptions?**

1. ✅ **Architectural Enforcement**: If service can run without Data Storage, audit is optional (violates DD-AUDIT-003)
2. ✅ **CI Integrity**: Skipped tests mean features are not validated
3. ✅ **Developer Discipline**: Forces proper infrastructure setup
4. ✅ **Compliance**: Audit trails are compliance-critical - can't be skipped

---

### **Test Business Logic, NOT Infrastructure**

**CRITICAL PRINCIPLE**: Integration tests MUST test **service behavior (business logic)**, NOT **infrastructure (audit/metrics libraries)**.

#### **✅ CORRECT Pattern: Business Logic with Audit Side Effects**

```go
// ✅ CORRECT: Test business operation, verify audit as side effect
var _ = Describe("Gateway Signal Processing Audit", func() {
    It("should emit audit event when processing signal", func() {
        // Step 1: Trigger BUSINESS OPERATION
        signal := &corev1.Event{
            ObjectMeta: metav1.ObjectMeta{Name: "test-signal"},
            Reason:     "OOMKilled",
        }

        resp, err := http.Post(gatewayURL+"/webhook/signals", "application/json", marshalEvent(signal))
        Expect(err).ToNot(HaveOccurred())
        Expect(resp.StatusCode).To(Equal(http.StatusAccepted))

        correlationID := extractCorrelationID(resp)

        // Step 2: Verify AUDIT SIDE EFFECT
        Eventually(func() int {
            events, _ := dsClient.QueryAuditEvents(correlationID, "gateway.signal.received")
            return len(events)
        }, 30*time.Second, 1*time.Second).Should(Equal(1),
            "Gateway should emit audit event during signal processing")

        // Step 3: Validate AUDIT CONTENT
        events, _ := dsClient.QueryAuditEvents(correlationID, "gateway.signal.received")
        Expect(events[0].EventData).To(HaveKey("original_payload"))
        Expect(events[0].EventData).To(HaveKey("signal_labels"))
    })
})
```

#### **❌ WRONG Pattern: Testing Audit Infrastructure**

```go
// ❌ FORBIDDEN: Directly calling audit store (tests infrastructure, not business logic)
var _ = Describe("Audit Infrastructure Tests", func() {
    It("should write audit event to Data Storage", func() {
        // ❌ WRONG: Manually creating audit event
        event := audit.NewAuditEventRequest()
        audit.SetEventType(event, "gateway.signal.received")

        // ❌ WRONG: Directly calling audit store
        err := auditStore.StoreAudit(ctx, event)
        Expect(err).NotTo(HaveOccurred())

        // ❌ WRONG: Testing audit persistence (DataStorage's responsibility)
        Eventually(func() int {
            events, _ := dsClient.QueryAuditEvents(correlationID)
            return len(events)
        }).Should(Equal(1))
    })
})
```

#### **Pattern Comparison**

| Aspect | ❌ Wrong Pattern | ✅ Correct Pattern |
|--------|-----------------|-------------------|
| **Primary Action** | `auditStore.StoreAudit()` | `http.Post()` or `k8sClient.Create()` |
| **What's Validated** | Audit persistence works | Service emits audits during business flow |
| **Test Ownership** | Should be in DataStorage tests | Correctly in service tests |
| **Failure Detection** | Won't catch missing audit calls | Catches missing audit integration |

#### **Reference Implementations**

**Correct Examples** (to follow):
- SignalProcessing: `test/integration/signalprocessing/audit_integration_test.go`
- Gateway: `test/integration/gateway/audit_integration_test.go`

**What NOT to do**:
- Don't manually create audit events in tests
- Don't directly call `auditStore.StoreAudit()` or `dsClient.StoreBatch()`
- Don't test audit buffering/batching (that's `pkg/audit` responsibility)

---

## 📊 **Test Tier Priority Matrix**

Use this matrix to determine which tier to test each feature:

| Feature | Unit | Integration | E2E | Rationale |
|---------|------|-------------|-----|-----------|
| **Audit event fields correct** | ⬜ | ✅ | ⬜ | Needs real Data Storage for OpenAPI validation |
| **Audit client wired** | ⬜ | ⬜ | ✅ | Must verify in deployed environment |
| **Event data structure** | ⬜ | ✅ | ⬜ | OpenAPI client validation with real DS |
| **RR reconstruction logic** | ✅ | ✅ | ⬜ | Algorithm in unit, full flow in integration |
| **Cross-service audit flow** | ⬜ | ⬜ | ✅ | Gateway → ... → Orchestrator |

### **SOC2 Week 1 Test Distribution**

| Day | Feature | Unit | Integration | E2E | Test Tier Justification |
|-----|---------|------|-------------|-----|------------------------|
| **Day 1** | Gateway signal data | ⬜ | ✅ (3 specs) | ✅ (2 specs) | Integration: Real DS for field validation<br>E2E: Verify in Kind cluster |
| **Day 2** | AI provider data | ⬜ | ✅ (3 specs) | ✅ (2 specs) | Integration: Real Holmes API mock<br>E2E: Full CRD reconciliation |
| **Day 3** | Workflow refs | ⬜ | ✅ (3 specs) | ✅ (2 specs) | Integration: Real envtest<br>E2E: Tekton pipeline execution |
| **Day 4** | Error details | ⬜ | ✅ (8 specs) | ✅ (4 specs) | Integration: Real error scenarios<br>E2E: Cross-service error propagation |
| **Day 5** | RR reconstruction | ✅ (algorithm) | ✅ (10 specs) | ✅ (5 specs) | Unit: Reconstruction algorithm<br>Integration: Full lifecycle<br>E2E: TTL expiration scenario |
| **Day 6** | Validation | ⬜ | ✅ (5 specs) | ✅ (3 specs) | Execute all test suites |

**Total Week 1**: 1 unit + 32 integration + 18 E2E = **51 specs**

### **Legend**:
- ✅ Test here
- ⬜ Don't test here

---

## 📊 **Implementation Status by Service**

| Service | Audit Integration | RR Fields | SOC2 Ready | Status |
|---------|-------------------|-----------|------------|--------|
| **Gateway** | ✅ Yes (DD-API-001) | ❌ 0/3 fields | ⚠️ Partial | **NEEDS WORK** |
| **Signal Processing** | ✅ Yes | ⚠️ N/A | ⚠️ Partial | **NEEDS WORK** |
| **AI Analysis** | ✅ Yes | ❌ 0/1 field | ⚠️ Partial | **NEEDS WORK** |
| **Workflow Execution** | ✅ Yes | ❌ 0/2 fields | ⚠️ Partial | **NEEDS WORK** |
| **Remediation Orchestrator** | ✅ Yes | ❌ 0/1 field | ⚠️ Partial | **NEEDS WORK** |
| **Notification** | ✅ Yes | ⚠️ N/A | ✅ Ready | **COMPLIANT** |
| **Data Storage** | ✅ Yes (is audit service) | ⚠️ N/A | ✅ Ready | **COMPLIANT** |
| **HolmesGPT API** | ✅ Yes | ⚠️ N/A | ✅ Ready | **COMPLIANT** |

**Overall Status**: **20% Complete** (2/8 services fully compliant)

---

## 🚨 **8 Critical Implementation Gaps**

### **Gap 1-3: Gateway Service - Signal Data** ❌ **BLOCKER**

**Missing Fields**:
- `event_data.original_payload` (2-5KB per event)
- `event_data.signal_labels` (0.5-2KB)
- `event_data.signal_annotations` (0.5-2KB)

**Impact**:
- ❌ Cannot reconstruct `spec.originalPayload`
- ❌ Cannot reconstruct `spec.signalLabels`
- ❌ Cannot reconstruct `spec.signalAnnotations`
- ❌ 40% of RR reconstruction BLOCKED

**Files to Modify**:
- `pkg/gateway/signal_processor.go`
- `pkg/gateway/audit_types.go`

**Effort**: **Day 1** (9 hours: 6h implementation + 3h integration tests)

---

### **Gap 4: AI Analysis Service - Provider Data** ❌ **BLOCKER**

**Missing Field**:
- `event_data.provider_data` (1-3KB per event)

**Impact**:
- ❌ Cannot reconstruct `spec.aiAnalysis.providerData`
- ❌ Holmes/AI provider response lost
- ❌ 20% of RR reconstruction BLOCKED

**Files to Modify**:
- `internal/controller/aianalysis/controller.go`
- `internal/controller/aianalysis/audit.go`

**Effort**: **Day 2** (9 hours: 6h implementation + 3h integration tests)

---

### **Gap 5-6: Workflow Execution - References** ❌ **BLOCKER**

**Missing Fields**:
- `event_data.selected_workflow_ref` (200B per event)
- `event_data.execution_ref` (200B per event)

**Impact**:
- ❌ Cannot reconstruct `status.selectedWorkflowRef`
- ❌ Cannot reconstruct `status.executionRef`
- ❌ 25% of RR reconstruction BLOCKED

**Files to Modify**:
- `internal/controller/workflowexecution/workflow_selector.go`
- `internal/controller/workflowexecution/controller.go`
- `internal/controller/workflowexecution/audit.go`

**Effort**: **Day 3** (5 hours: 3h implementation + 2h integration tests)

---

### **Gap 7: Error Details - All Services** ⚠️ **PARTIAL**

**Missing Enhanced Error Information**:
- Structured error details in `*.failure` events
- Retry information
- Component-specific error codes

**Impact**:
- ⚠️ Can reconstruct basic `status.error`
- ❌ Cannot reconstruct detailed error context
- ⚠️ 10% of RR reconstruction quality degraded

**Files to Modify**:
- `pkg/shared/audit/error_types.go` (new)
- `pkg/gateway/audit_errors.go`
- `internal/controller/aianalysis/audit_errors.go`
- `internal/controller/workflowexecution/audit_errors.go`
- `internal/controller/remediationorchestrator/audit/error_helpers.go`

**Effort**: **Day 4** (10 hours: 6h implementation + 4h integration tests for 4 services)

---

### **Gap 8: Orchestrator - TimeoutConfig** ❌ **BLOCKER**

**Missing Field**:
- `event_data.timeout_config` (100-200B per event)

**Impact**:
- ❌ Cannot reconstruct `status.timeoutConfig`
- ❌ Custom timeout configurations lost
- ❌ 5% of RR reconstruction BLOCKED (affects custom timeouts)

**Files to Modify**:
- `internal/controller/remediationorchestrator/audit/helpers.go`
- `internal/controller/remediationorchestrator/controller.go`

**Effort**: **Day 5 Part 1** (5 hours: 3h implementation + 2h integration tests)

---

## 🎯 **Week 1 Work Breakdown**

### **Day 1: Gateway Signal Data Capture** (9 hours)

**Goal**: Capture complete signal data for RR reconstruction (Gap #1-3)

**Test Plan Reference**: [SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md](./SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md) Section 1.2

**Gaps Addressed**: #1 (`original_payload`), #2 (`signal_labels`), #3 (`signal_annotations`)

---

#### **Phase 1: Analyze** (10 min)

**Tasks**:
1. Review existing Gateway audit integration: `pkg/gateway/audit_types.go`
2. Review existing event emission: `pkg/gateway/signal_processor.go`
3. Identify where new fields should be added to `GatewayEventData` struct

**Expected Findings**:
- `GatewayEventData` struct exists but lacks 3 RR fields
- `gateway.signal.received` event already emitted, needs enrichment
- Audit store integration already wired (DD-API-001 compliant)

---

#### **Phase 2: Plan** (10 min)

**Implementation Strategy**:
- Enhance existing `GatewayEventData` struct (not create new)
- Update existing event emission logic
- Use OpenAPI client for audit queries in tests

**Acceptance Criteria** (from BR-AUDIT-005):
- ✅ `event_data.original_payload` captured (full K8s Event)
- ✅ `event_data.signal_labels` captured (map[string]string)
- ✅ `event_data.signal_annotations` captured (map[string]string)

---

#### **Phase 3: Do-RED** (3 hours) - WRITE TESTS FIRST

**Task 1.1**: Create failing integration test file (3 hours)

**File**: `test/integration/gateway/audit_signal_data_integration_test.go` (NEW)

**Audit Validation Requirements** (MANDATORY per DD-TESTING-001):

**OpenAPI Client Usage** (DD-API-001):
```go
import dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"

// Type-safe OpenAPI client
dsClient, err := dsgen.NewClientWithResponses(dataStorageURL)
Expect(err).ToNot(HaveOccurred())
```

**Test Structure** (follows test plan Section 1.2):
```go
var _ = Describe("BR-AUDIT-005: Gateway Signal Data Integration", func() {
    var (
        dsClient     *dsgen.ClientWithResponses
        ctx          context.Context
        gatewayURL   string
    )

    BeforeEach(func() {
        ctx = context.Background()
        gatewayURL = os.Getenv("GATEWAY_URL")
        dataStorageURL := os.Getenv("DATA_STORAGE_URL")

        var err error
        dsClient, err = dsgen.NewClientWithResponses(dataStorageURL)
        Expect(err).ToNot(HaveOccurred())

        // ✅ REQUIRED: Fail if Data Storage unavailable (NO Skip())
        resp, err := http.Get(dataStorageURL + "/health")
        Expect(err).ToNot(HaveOccurred(),
            "REQUIRED: Data Storage not available at %s\n"+
            "  Start with: podman-compose -f podman-compose.test.yml up -d",
            dataStorageURL)
        Expect(resp.StatusCode).To(Equal(http.StatusOK))
    })

    Context("Gap #1-3: Complete Signal Data Capture", func() {
        It("should capture all 3 fields when Gateway processes K8s Event", func() {
            // Given: K8s Event with labels and annotations
            signal := &corev1.Event{
                ObjectMeta: metav1.ObjectMeta{
                    Name: "test-signal",
                    Labels: map[string]string{
                        "app": "nginx",
                        "tier": "frontend",
                    },
                    Annotations: map[string]string{
                        "description": "OOMKilled",
                        "runbook_url": "https://example.com/runbook",
                    },
                },
                Reason: "OOMKilled",
                Message: "Container was OOM killed",
            }

            // When: Gateway processes signal
            resp, err := http.Post(
                gatewayURL+"/webhook/kubernetes-events",
                "application/json",
                marshalEvent(signal),
            )
            Expect(err).ToNot(HaveOccurred())
            Expect(resp.StatusCode).To(Equal(http.StatusAccepted))

            correlationID := extractCorrelationID(resp)

            // Then: Audit event should have 3 fields (TEST WILL FAIL initially)
            eventType := "gateway.signal.received"

            // ✅ REQUIRED: Use Eventually(), NEVER time.Sleep()
            Eventually(func() int {
                resp, err := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
                    EventType:     &eventType,
                    CorrelationId: &correlationID,
                })
                if err != nil || resp.JSON200 == nil {
                    return 0
                }
                return *resp.JSON200.Pagination.Total
            }, 30*time.Second, 1*time.Second).Should(Equal(1),
                "Should find exactly 1 audit event within 30 seconds")

            // Query final events for validation
            resp, err := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
                EventType:     &eventType,
                CorrelationId: &correlationID,
            })
            Expect(err).ToNot(HaveOccurred())
            Expect(resp.JSON200).ToNot(BeNil())

            events := *resp.JSON200.Data
            Expect(len(events)).To(Equal(1))

            eventData := events[0].EventData.(map[string]interface{})

            // Gap #1: original_payload (WILL FAIL - field doesn't exist yet)
            Expect(eventData).To(HaveKey("original_payload"))
            originalPayload := eventData["original_payload"].(map[string]interface{})
            Expect(originalPayload["reason"]).To(Equal("OOMKilled"))
            Expect(originalPayload["message"]).To(Equal("Container was OOM killed"))

            // Gap #2: signal_labels (WILL FAIL - field doesn't exist yet)
            Expect(eventData).To(HaveKey("signal_labels"))
            labels := eventData["signal_labels"].(map[string]interface{})
            Expect(labels["app"]).To(Equal("nginx"))
            Expect(labels["tier"]).To(Equal("frontend"))

            // Gap #3: signal_annotations (WILL FAIL - field doesn't exist yet)
            Expect(eventData).To(HaveKey("signal_annotations"))
            annotations := eventData["signal_annotations"].(map[string]interface{})
            Expect(annotations["description"]).To(Equal("OOMKilled"))
            Expect(annotations["runbook_url"]).To(Equal("https://example.com/runbook"))
        })
    })
})
```

**Validation Command** (tests MUST fail in RED phase):
```bash
# Parallel execution with 4 concurrent processes (standard)
go test ./test/integration/gateway/audit_signal_data_integration_test.go -v -p 4

# Expected output: FAIL (fields missing - this is CORRECT in RED phase)
```

---

#### **Phase 4: Do-GREEN** (4 hours) - MINIMAL IMPLEMENTATION

**Task 1.2**: Add fields to `GatewayEventData` struct (1 hour)

**File**: `pkg/gateway/audit_types.go`

```go
type GatewayEventData struct {
    // Existing fields...
    SignalType    string    `json:"signal_type"`
    SignalSource  string    `json:"signal_source"`
    ReceivedAt    time.Time `json:"received_at"`

    // NEW: Gap #1-3 fields for RR reconstruction
    OriginalPayload   interface{}       `json:"original_payload"`     // Gap #1
    SignalLabels      map[string]string `json:"signal_labels"`        // Gap #2
    SignalAnnotations map[string]string `json:"signal_annotations"`   // Gap #3
}
```

**Task 1.3**: Update event emission (2 hours)

**File**: `pkg/gateway/signal_processor.go`

```go
func (p *SignalProcessor) emitAuditEvent(ctx context.Context, signal *corev1.Event) error {
    eventData := &GatewayEventData{
        // Existing fields...
        SignalType:   signal.Type,
        SignalSource: signal.Source.Host,
        ReceivedAt:   time.Now(),

        // NEW: Populate Gap #1-3 fields for RR reconstruction
        OriginalPayload:   signal,                    // Gap #1: Full K8s Event
        SignalLabels:      signal.Labels,             // Gap #2: Labels map
        SignalAnnotations: signal.Annotations,        // Gap #3: Annotations map
    }

    return p.auditStore.StoreAudit(ctx, &audit.AuditEventRequest{
        EventType:     "gateway.signal.received",
        EventCategory: "gateway",
        EventAction:   "signal.received",
        EventOutcome:  "success",
        ActorType:     "system",
        ActorID:       "gateway",
        ResourceType:  "KubernetesEvent",
        ResourceName:  signal.Name,
        CorrelationID: extractCorrelationID(ctx),
        EventData:     eventData,
    })
}
```

**Task 1.4**: Manual smoke test (1 hour)

```bash
# Start infrastructure
podman-compose -f podman-compose.test.yml up -d

# Send test signal
curl -X POST http://localhost:8080/webhook/kubernetes-events \
  -H "Content-Type: application/json" \
  -d @test-event.json

# Verify in Data Storage (OpenAPI client)
curl "http://localhost:8080/api/v1/audit/events?event_type=gateway.signal.received" | jq '.data[0].event_data'
```

**Validation Command** (tests MUST pass in GREEN phase):
```bash
# Parallel execution with 4 concurrent processes (standard)
go test ./test/integration/gateway/audit_signal_data_integration_test.go -v -p 4

# Expected output: PASS (all 3 fields present)
```

---

#### **Phase 5: Do-REFACTOR** (1 hour)

**Optimization Tasks**:
- Add defensive nil checks for labels/annotations
- Optimize field extraction if needed
- Improve error messages for missing fields
- Add field size logging for monitoring

**File**: `pkg/gateway/signal_processor.go`

```go
func (p *SignalProcessor) emitAuditEvent(ctx context.Context, signal *corev1.Event) error {
    // Defensive nil checks for optional fields
    labels := signal.Labels
    if labels == nil {
        labels = make(map[string]string)
    }

    annotations := signal.Annotations
    if annotations == nil {
        annotations = make(map[string]string)
    }

    eventData := &GatewayEventData{
        // Existing fields...
        OriginalPayload:   signal,
        SignalLabels:      labels,
        SignalAnnotations: annotations,
    }

    // Log field sizes for monitoring
    p.logger.Debug("Audit event field sizes",
        "payload_size", len(fmt.Sprintf("%v", signal)),
        "labels_count", len(labels),
        "annotations_count", len(annotations),
    )

    return p.auditStore.StoreAudit(ctx, eventData)
}
```

**Validation**: Run tests again - should still pass:
```bash
go test ./test/integration/gateway/audit_signal_data_integration_test.go -v -p 4
```

---

#### **Phase 6: Check** (10 min)

**Validation Checklist**:
- ✅ Gap #1 closed: `original_payload` captured in `gateway.signal.received` events
- ✅ Gap #2 closed: `signal_labels` captured in `gateway.signal.received` events
- ✅ Gap #3 closed: `signal_annotations` captured in `gateway.signal.received` events
- ✅ Integration tests passing (3 specs from test plan Section 1.2)
- ✅ OpenAPI client used for all audit queries
- ✅ No `time.Sleep()` in tests (used `Eventually()`)
- ✅ No `Skip()` in tests (fails if Data Storage unavailable)
- ✅ Tests validate business logic (signal processing), not infrastructure

**BR-AUDIT-005 Progress**: 40% complete (3/8 RR fields captured)

**Files Modified**:
- ✅ `pkg/gateway/audit_types.go` (added 3 fields)
- ✅ `pkg/gateway/signal_processor.go` (updated emission)
- ✅ `test/integration/gateway/audit_signal_data_integration_test.go` (new test file)

---

### **Day 2: AI Analysis Provider Data Capture** (9 hours)

**Goal**: Capture AI provider response for RR reconstruction (Gap #4)

**Test Plan Reference**: [SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md](./SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md) Section 2.2

**Gaps Addressed**: #4 (`provider_data`)

---

#### **Phase 1: Analyze** (10 min)

**Tasks**:
1. Review existing AI Analysis audit integration: `internal/controller/aianalysis/audit.go`
2. Review HolmesGPT response handling: `internal/controller/aianalysis/controller.go`
3. Identify where provider data should be captured

**Expected Findings**:
- `aianalysis.analysis.completed` event exists but lacks provider data
- HolmesGPT response available in controller reconciliation
- Need to extract full provider response, not just confidence score

---

#### **Phase 2: Plan** (10 min)

**Implementation Strategy**:
- Add `ProviderData` field to existing audit event structure
- Capture full Holmes response (not just confidence)
- Use OpenAPI client for audit validation in tests

**Acceptance Criteria** (from BR-AUDIT-005):
- ✅ `event_data.provider_data` captured (full Holmes response)
- ✅ Provider name (`holmesgpt`) included
- ✅ Confidence score and analysis details included

---

#### **Phase 3: Do-RED** (3 hours) - WRITE TESTS FIRST

**Task 2.1**: Create failing integration test file (3 hours)

**File**: `test/integration/aianalysis/audit_provider_data_integration_test.go` (NEW)

**Test Structure** (follows test plan Section 2.2):
```go
var _ = Describe("BR-AUDIT-005: AI Analysis Provider Data Integration", func() {
    var (
        dsClient     *dsgen.ClientWithResponses
        ctx          context.Context
        holmesClient *holmesgpt.Client
        k8sClient    client.Client
    )

    BeforeEach(func() {
        ctx = context.Background()
        dataStorageURL := os.Getenv("DATA_STORAGE_URL")

        var err error
        dsClient, err = dsgen.NewClientWithResponses(dataStorageURL)
        Expect(err).ToNot(HaveOccurred())

        // ✅ REQUIRED: Fail if Data Storage unavailable
        resp, err := http.Get(dataStorageURL + "/health")
        Expect(err).ToNot(HaveOccurred(),
            "REQUIRED: Data Storage not available\n"+
            "  Start with: podman-compose -f podman-compose.test.yml up -d")
        Expect(resp.StatusCode).To(Equal(http.StatusOK))

        // Setup Holmes client mock
        holmesClient = holmesgpt.NewMockClient()
        k8sClient = createTestK8sClient()
    })

    Context("Gap #4: AI Provider Data Capture", func() {
        It("should capture provider data when AI analysis completes", func() {
            // Given: RemediationRequest for AI analysis
            rr := &v1alpha1.RemediationRequest{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-rr",
                    Namespace: "default",
                },
                Spec: v1alpha1.RemediationRequestSpec{
                    OriginalPayload: map[string]interface{}{
                        "alert": "OOMKilled",
                        "pod":   "nginx-abc123",
                    },
                },
            }
            err := k8sClient.Create(ctx, rr)
            Expect(err).ToNot(HaveOccurred())

            // When: AI Analysis Controller processes RR
            // (Controller will call Holmes API and emit audit event)

            correlationID := string(rr.UID)
            eventType := "aianalysis.analysis.completed"

            // ✅ REQUIRED: Use Eventually(), NEVER time.Sleep()
            Eventually(func() int {
                resp, err := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
                    EventType:     &eventType,
                    CorrelationId: &correlationID,
                })
                if err != nil || resp.JSON200 == nil {
                    return 0
                }
                return *resp.JSON200.Pagination.Total
            }, 60*time.Second, 2*time.Second).Should(Equal(1),
                "Should find exactly 1 audit event within 60 seconds")

            // Then: Audit event should have provider_data (WILL FAIL initially)
            resp, err := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
                EventType:     &eventType,
                CorrelationId: &correlationID,
            })
            Expect(err).ToNot(HaveOccurred())
            Expect(resp.JSON200).ToNot(BeNil())

            events := *resp.JSON200.Data
            Expect(len(events)).To(Equal(1))

            eventData := events[0].EventData.(map[string]interface{})

            // Gap #4: provider_data (WILL FAIL - field doesn't exist yet)
            Expect(eventData).To(HaveKey("provider_data"))
            providerData := eventData["provider_data"].(map[string]interface{})

            // Validate provider information
            Expect(providerData).To(HaveKey("provider"))
            Expect(providerData["provider"]).To(Equal("holmesgpt"))

            // Validate analysis details
            Expect(providerData).To(HaveKey("confidence"))
            Expect(providerData["confidence"]).To(BeNumerically(">", 0))
            Expect(providerData["confidence"]).To(BeNumerically("<=", 1))

            // Validate full response captured
            Expect(providerData).To(HaveKey("analysis"))
            Expect(providerData).To(HaveKey("recommended_actions"))
        })
    })
})
```

**Validation Command** (tests MUST fail in RED phase):
```bash
# Parallel execution with 4 concurrent processes
go test ./test/integration/aianalysis/audit_provider_data_integration_test.go -v -p 4

# Expected: FAIL (provider_data field missing - CORRECT in RED phase)
```

---

#### **Phase 4: Do-GREEN** (4 hours) - MINIMAL IMPLEMENTATION

**Task 2.2**: Add `provider_data` field to audit event (1 hour)

**File**: `internal/controller/aianalysis/audit.go`

```go
type AIAnalysisAuditEvent struct {
    // Existing fields...
    RequestPayload    interface{} `json:"request_payload"`
    AnalysisStartedAt time.Time   `json:"analysis_started_at"`
    AnalysisDuration  string      `json:"analysis_duration"`

    // NEW: Gap #4 field for RR reconstruction
    ProviderData interface{} `json:"provider_data"` // Gap #4: Full Holmes response
}
```

**Task 2.3**: Update event emission in controller (2 hours)

**File**: `internal/controller/aianalysis/controller.go`

```go
func (r *AIAnalysisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... existing logic ...

    // Call HolmesGPT
    holmesResponse, err := r.holmesClient.Analyze(ctx, rr.Spec.OriginalPayload)
    if err != nil {
        return ctrl.Result{}, err
    }

    // Update RR status
    rr.Status.AIAnalysis = &v1alpha1.AIAnalysisResult{
        Provider:   "holmesgpt",
        Confidence: holmesResponse.Confidence,
        // ... other fields ...
    }

    // NEW: Emit audit event with provider data
    err = r.auditClient.CreateAuditEvent(ctx, &dsgen.AuditEvent{
        EventType:     "aianalysis.analysis.completed",
        EventCategory: "analysis",
        EventAction:   "analysis.completed",
        EventOutcome:  "success",
        ActorType:     "service",
        ActorId:       "aianalysis-controller",
        ResourceType:  "RemediationRequest",
        ResourceName:  rr.Name,
        CorrelationId: string(rr.UID),
        EventData: map[string]interface{}{
            "analysis_started_at": startTime,
            "analysis_duration":   time.Since(startTime).String(),

            // Gap #4: Full Holmes response for RR reconstruction
            "provider_data": map[string]interface{}{
                "provider":            "holmesgpt",
                "confidence":          holmesResponse.Confidence,
                "analysis":            holmesResponse.Analysis,
                "recommended_actions": holmesResponse.RecommendedActions,
                "raw_response":        holmesResponse, // Full response
            },
        },
    })

    return ctrl.Result{}, err
}
```

**Task 2.4**: Manual smoke test (1 hour)

```bash
# Start infrastructure
podman-compose -f podman-compose.test.yml up -d

# Create RemediationRequest
kubectl apply -f - <<EOF
apiVersion: remediation.kubernaut.ai/v1alpha1
kind: RemediationRequest
metadata:
  name: test-rr
spec:
  originalPayload:
    alert: "OOMKilled"
    pod: "nginx-abc123"
EOF

# Wait for AI analysis
kubectl get rr test-rr -o jsonpath='{.status.aiAnalysis}'

# Verify audit event
curl "http://localhost:8080/api/v1/audit/events?event_type=aianalysis.analysis.completed" | jq '.data[0].event_data.provider_data'
```

**Validation Command** (tests MUST pass in GREEN phase):
```bash
go test ./test/integration/aianalysis/audit_provider_data_integration_test.go -v -p 4

# Expected: PASS (provider_data field present)
```

---

#### **Phase 5: Do-REFACTOR** (1 hour)

**Optimization Tasks**:
- Add defensive nil checks for Holmes response
- Add provider data size logging
- Optimize large response handling
- Add error handling for malformed responses

**File**: `internal/controller/aianalysis/controller.go`

```go
func (r *AIAnalysisReconciler) buildProviderData(holmesResponse *holmesgpt.Response) map[string]interface{} {
    if holmesResponse == nil {
        return map[string]interface{}{
            "provider": "holmesgpt",
            "error":    "nil response",
        }
    }

    providerData := map[string]interface{}{
        "provider":            "holmesgpt",
        "confidence":          holmesResponse.Confidence,
        "analysis":            holmesResponse.Analysis,
        "recommended_actions": holmesResponse.RecommendedActions,
    }

    // Log provider data size for monitoring
    dataSize := len(fmt.Sprintf("%v", providerData))
    r.logger.Debug("Provider data size", "bytes", dataSize)

    return providerData
}
```

**Validation**: Run tests again - should still pass:
```bash
go test ./test/integration/aianalysis/audit_provider_data_integration_test.go -v -p 4
```

---

#### **Phase 6: Check** (10 min)

**Validation Checklist**:
- ✅ Gap #4 closed: `provider_data` captured in `aianalysis.analysis.completed` events
- ✅ Integration tests passing (3 specs from test plan Section 2.2)
- ✅ OpenAPI client used for all audit queries
- ✅ No `time.Sleep()` in tests (used `Eventually()`)
- ✅ No `Skip()` in tests
- ✅ Tests validate business logic (AI analysis), not infrastructure

**BR-AUDIT-005 Progress**: 60% complete (4/8 RR fields captured)

**Files Modified**:
- ✅ `internal/controller/aianalysis/audit.go` (added provider_data field)
- ✅ `internal/controller/aianalysis/controller.go` (updated emission)
- ✅ `test/integration/aianalysis/audit_provider_data_integration_test.go` (new test file)

---

### **Day 3: Workflow Execution - Selection & Execution Refs** (5 hours)

**Goal**: Capture workflow selection and execution references (Gap #5-6)

**Test Plan Reference**: [SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md](./SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md) Section 3.2

**Gaps Addressed**: #5 (`selected_workflow_ref`), #6 (`execution_ref`)

---

#### **Phase 1: Analyze** (10 min)

**Tasks**:
1. Review workflow selection logic: `internal/controller/workflowexecution/workflow_selector.go`
2. Review execution start logic: `internal/controller/workflowexecution/controller.go`
3. Identify where to capture both workflow ref and execution ref

**Expected Findings**:
- Workflow selection happens before execution creation
- `workflow.selection.completed` event needs `selected_workflow_ref`
- `execution.workflow.started` event needs `execution_ref`
- Both refs are CRD references (name, namespace, UID)

---

#### **Phase 2: Plan** (10 min)

**Implementation Strategy**:
- Add `selected_workflow_ref` to workflow selection event
- Add `execution_ref` to execution start event
- Use OpenAPI client for audit validation in tests

**Acceptance Criteria** (from BR-AUDIT-005):
- ✅ `event_data.selected_workflow_ref` captured (workflow CRD reference)
- ✅ `event_data.execution_ref` captured (execution CRD reference)

---

#### **Phase 3: Do-RED** (2 hours) - WRITE TESTS FIRST

**Task 3.1**: Create failing integration test file (2 hours)

**File**: `test/integration/workflowexecution/audit_refs_integration_test.go` (NEW)

**Test Structure** (follows test plan Section 3.2):
```go
var _ = Describe("BR-AUDIT-005: Workflow Execution Refs Integration", func() {
    var (
        dsClient  *dsgen.ClientWithResponses
        ctx       context.Context
        k8sClient client.Client
        testEnv   *envtest.Environment
    )

    BeforeEach(func() {
        ctx = context.Background()
        dataStorageURL := os.Getenv("DATA_STORAGE_URL")

        var err error
        dsClient, err = dsgen.NewClientWithResponses(dataStorageURL)
        Expect(err).ToNot(HaveOccurred())

        // ✅ REQUIRED: Fail if Data Storage unavailable
        resp, err := http.Get(dataStorageURL + "/health")
        Expect(err).ToNot(HaveOccurred(),
            "REQUIRED: Data Storage not available")
        Expect(resp.StatusCode).To(Equal(http.StatusOK))

        // Setup envtest for K8s API
        testEnv, k8sClient = setupEnvtest()
    })

    AfterEach(func() {
        testEnv.Stop()
    })

    Context("Gap #5-6: Workflow and Execution References", func() {
        It("should capture both refs during workflow execution lifecycle", func() {
            // Given: RemediationWorkflow and RemediationRequest
            workflow := &v1alpha1.RemediationWorkflow{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-workflow",
                    Namespace: "default",
                },
                Spec: v1alpha1.RemediationWorkflowSpec{
                    Steps: []v1alpha1.WorkflowStep{
                        {Name: "diagnose", Type: "diagnostic"},
                    },
                },
            }
            err := k8sClient.Create(ctx, workflow)
            Expect(err).ToNot(HaveOccurred())

            rr := &v1alpha1.RemediationRequest{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-rr",
                    Namespace: "default",
                },
                Spec: v1alpha1.RemediationRequestSpec{
                    OriginalPayload: map[string]interface{}{"alert": "OOMKilled"},
                },
            }
            err = k8sClient.Create(ctx, rr)
            Expect(err).ToNot(HaveOccurred())

            correlationID := string(rr.UID)

            // When: Workflow is selected and execution starts
            // (Controller will emit both events)

            // Then: Should have workflow.selection.completed event with selected_workflow_ref
            selectionEventType := "workflow.selection.completed"

            // ✅ REQUIRED: Use Eventually(), NEVER time.Sleep()
            Eventually(func() int {
                resp, err := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
                    EventType:     &selectionEventType,
                    CorrelationId: &correlationID,
                })
                if err != nil || resp.JSON200 == nil {
                    return 0
                }
                return *resp.JSON200.Pagination.Total
            }, 60*time.Second, 2*time.Second).Should(Equal(1),
                "Should find workflow selection event")

            // Validate Gap #5: selected_workflow_ref (WILL FAIL initially)
            resp, err := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
                EventType:     &selectionEventType,
                CorrelationId: &correlationID,
            })
            Expect(err).ToNot(HaveOccurred())
            Expect(resp.JSON200).ToNot(BeNil())

            selectionEvents := *resp.JSON200.Data
            Expect(len(selectionEvents)).To(Equal(1))

            selectionData := selectionEvents[0].EventData.(map[string]interface{})

            // Gap #5: selected_workflow_ref (WILL FAIL - field doesn't exist yet)
            Expect(selectionData).To(HaveKey("selected_workflow_ref"))
            workflowRef := selectionData["selected_workflow_ref"].(map[string]interface{})
            Expect(workflowRef["name"]).To(Equal("test-workflow"))
            Expect(workflowRef["namespace"]).To(Equal("default"))
            Expect(workflowRef["uid"]).ToNot(BeEmpty())

            // Then: Should have execution.workflow.started event with execution_ref
            executionEventType := "execution.workflow.started"

            Eventually(func() int {
                resp, err := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
                    EventType:     &executionEventType,
                    CorrelationId: &correlationID,
                })
                if err != nil || resp.JSON200 == nil {
                    return 0
                }
                return *resp.JSON200.Pagination.Total
            }, 60*time.Second, 2*time.Second).Should(Equal(1),
                "Should find execution started event")

            // Validate Gap #6: execution_ref (WILL FAIL initially)
            resp, err = dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
                EventType:     &executionEventType,
                CorrelationId: &correlationID,
            })
            Expect(err).ToNot(HaveOccurred())
            Expect(resp.JSON200).ToNot(BeNil())

            executionEvents := *resp.JSON200.Data
            Expect(len(executionEvents)).To(Equal(1))

            executionData := executionEvents[0].EventData.(map[string]interface{})

            // Gap #6: execution_ref (WILL FAIL - field doesn't exist yet)
            Expect(executionData).To(HaveKey("execution_ref"))
            execRef := executionData["execution_ref"].(map[string]interface{})
            Expect(execRef["name"]).ToNot(BeEmpty())
            Expect(execRef["namespace"]).To(Equal("default"))
            Expect(execRef["uid"]).ToNot(BeEmpty())
        })
    })
})
```

**Validation Command** (tests MUST fail in RED phase):
```bash
go test ./test/integration/workflowexecution/audit_refs_integration_test.go -v -p 4

# Expected: FAIL (refs missing - CORRECT in RED phase)
```

---

#### **Phase 4: Do-GREEN** (2 hours) - MINIMAL IMPLEMENTATION

**Task 3.2**: Add `selected_workflow_ref` to workflow selection event (1 hour)

**File**: `internal/controller/workflowexecution/workflow_selector.go`

```go
func (s *WorkflowSelector) EmitSelectionAuditEvent(
    ctx context.Context,
    rr *v1alpha1.RemediationRequest,
    selectedWorkflow *v1alpha1.RemediationWorkflow,
) error {
    return s.auditClient.CreateAuditEvent(ctx, &dsgen.AuditEvent{
        EventType:     "workflow.selection.completed",
        EventCategory: "workflow",
        EventAction:   "selection.completed",
        EventOutcome:  "success",
        ActorType:     "service",
        ActorId:       "workflowexecution-controller",
        ResourceType:  "RemediationRequest",
        ResourceName:  rr.Name,
        CorrelationId: string(rr.UID),
        EventData: map[string]interface{}{
            // Gap #5: Workflow CRD reference for RR reconstruction
            "selected_workflow_ref": map[string]interface{}{
                "name":      selectedWorkflow.Name,
                "namespace": selectedWorkflow.Namespace,
                "uid":       string(selectedWorkflow.UID),
            },
            "selection_criteria": "ai_recommendation", // existing field
        },
    })
}
```

**Task 3.3**: Add `execution_ref` to execution start event (1 hour)

**File**: `internal/controller/workflowexecution/controller.go`

```go
func (r *WorkflowExecutionReconciler) EmitExecutionStartAuditEvent(
    ctx context.Context,
    rr *v1alpha1.RemediationRequest,
    execution *v1alpha1.WorkflowExecution,
) error {
    return r.auditClient.CreateAuditEvent(ctx, &dsgen.AuditEvent{
        EventType:     "execution.workflow.started",
        EventCategory: "workflow",
        EventAction:   "execution.started",
        EventOutcome:  "success",
        ActorType:     "service",
        ActorId:       "workflowexecution-controller",
        ResourceType:  "RemediationRequest",
        ResourceName:  rr.Name,
        CorrelationId: string(rr.UID),
        EventData: map[string]interface{}{
            // Gap #6: Execution CRD reference for RR reconstruction
            "execution_ref": map[string]interface{}{
                "name":      execution.Name,
                "namespace": execution.Namespace,
                "uid":       string(execution.UID),
            },
            "workflow_ref": execution.Spec.WorkflowRef, // existing field
        },
    })
}
```

**Validation Command** (tests MUST pass in GREEN phase):
```bash
go test ./test/integration/workflowexecution/audit_refs_integration_test.go -v -p 4

# Expected: PASS (both refs present)
```

---

#### **Phase 5: Do-REFACTOR** (30 min)

**Optimization Tasks**:
- Extract ref building to helper function
- Add defensive nil checks
- Improve error messages

**File**: `internal/controller/workflowexecution/audit_helpers.go` (NEW)

```go
package workflowexecution

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// BuildCRDReference creates a standard CRD reference for audit events
func BuildCRDReference(obj metav1.Object) map[string]interface{} {
    if obj == nil {
        return map[string]interface{}{
            "error": "nil object",
        }
    }

    return map[string]interface{}{
        "name":      obj.GetName(),
        "namespace": obj.GetNamespace(),
        "uid":       string(obj.GetUID()),
    }
}
```

**Updated Files** (use helper):
- `workflow_selector.go`: Use `BuildCRDReference(selectedWorkflow)`
- `controller.go`: Use `BuildCRDReference(execution)`

**Validation**: Run tests again - should still pass:
```bash
go test ./test/integration/workflowexecution/audit_refs_integration_test.go -v -p 4
```

---

#### **Phase 6: Check** (10 min)

**Validation Checklist**:
- ✅ Gap #5 closed: `selected_workflow_ref` captured in `workflow.selection.completed` events
- ✅ Gap #6 closed: `execution_ref` captured in `execution.workflow.started` events
- ✅ Integration tests passing (3 specs from test plan Section 3.2)
- ✅ OpenAPI client used for all audit queries
- ✅ No `time.Sleep()` in tests (used `Eventually()`)
- ✅ No `Skip()` in tests
- ✅ Tests validate business logic (workflow selection/execution), not infrastructure

**BR-AUDIT-005 Progress**: 80% complete (6/8 RR fields captured)

**Files Modified**:
- ✅ `internal/controller/workflowexecution/workflow_selector.go` (added selected_workflow_ref)
- ✅ `internal/controller/workflowexecution/controller.go` (added execution_ref)
- ✅ `internal/controller/workflowexecution/audit_helpers.go` (new helper)
- ✅ `test/integration/workflowexecution/audit_refs_integration_test.go` (new test file)

---

### **Day 4: Error Details Standardization** (10 hours)

**Goal**: Enhance error capture across all services (Gap #7)

**Test Plan Reference**: [SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md](./SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md) Section 4.2

**Gaps Addressed**: #7 (`error_details` standardization across 4 services)

---

#### **Phase 1: Analyze** (10 min)

**Tasks**:
1. Review error handling: `pkg/gateway/audit_errors.go`, `internal/controller/aianalysis/audit_errors.go`, `internal/controller/workflowexecution/audit_errors.go`, `internal/controller/remediationorchestrator/audit/error_helpers.go`
2. Identify inconsistent error capture patterns across services
3. Review existing `*.failure` event types in DD-AUDIT-003

**Expected Findings**:
- Error details captured inconsistently (different field names, structures)
- Need standardized error structure for RR reconstruction (`.status.error` field)
- 4 services emit failure events: Gateway, AI Analysis, Workflow Execution, Orchestrator

---

#### **Phase 2: Plan** (15 min)

**Implementation Strategy**:
- Create shared error type: `pkg/shared/audit/error_types.go`
- Update error emission in 4 services to use shared type
- Use OpenAPI client for audit validation in integration tests

**Acceptance Criteria** (from BR-AUDIT-005):
- ✅ Standard `ErrorDetails` structure defined
- ✅ `error_details` field in all `*.failure` events
- ✅ 4 services updated: Gateway, AI Analysis, Workflow Execution, Orchestrator

**Standard Error Structure**:
```go
type ErrorDetails struct {
    Message       string   `json:"message"`
    Code          string   `json:"code"`
    Component     string   `json:"component"`
    RetryPossible bool     `json:"retry_possible"`
    StackTrace    []string `json:"stack_trace,omitempty"`
}
```

---

#### **Phase 3: Do-RED** (4 hours) - WRITE TESTS FIRST

**Task 4.1**: Create failing integration tests for 4 services (4 hours: 1h each)

**Files** (NEW):
- `test/integration/gateway/audit_errors_test.go`
- `test/integration/aianalysis/audit_errors_test.go`
- `test/integration/workflowexecution/audit_errors_test.go`
- `test/integration/remediationorchestrator/audit_errors_test.go`

**Audit Validation Requirements** (MANDATORY per DD-TESTING-001):

**OpenAPI Client Usage** (DD-API-001):
```go
import dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"

dsClient, err := dsgen.NewClientWithResponses(dataStorageURL)
Expect(err).ToNot(HaveOccurred())
```

**Test Pattern** (example for Gateway):
```go
var _ = Describe("BR-AUDIT-005: Gateway Error Audit", func() {
    var (
        dsClient   *dsgen.ClientWithResponses
        ctx        context.Context
        gatewayURL string
    )

    BeforeEach(func() {
        ctx = context.Background()
        gatewayURL = os.Getenv("GATEWAY_URL")
        dataStorageURL := os.Getenv("DATA_STORAGE_URL")

        var err error
        dsClient, err = dsgen.NewClientWithResponses(dataStorageURL)
        Expect(err).ToNot(HaveOccurred())

        // ✅ REQUIRED: Fail if Data Storage unavailable
        resp, err := http.Get(dataStorageURL + "/health")
        Expect(err).ToNot(HaveOccurred(),
            "REQUIRED: Data Storage not available")
        Expect(resp.StatusCode).To(Equal(http.StatusOK))
    })

    Context("Gap #7: Standardized Error Details", func() {
        It("should emit standardized error details on signal processing failure", func() {
            // Given: Invalid signal that will cause error
            invalidSignal := []byte(`{"invalid": "json structure"}`)

            // When: Gateway processes invalid signal (business operation)
            resp, err := http.Post(
                gatewayURL+"/webhook/kubernetes-events",
                "application/json",
                bytes.NewBuffer(invalidSignal),
            )
            Expect(err).ToNot(HaveOccurred())

            correlationID := extractCorrelationID(resp)
            eventType := "gateway.signal.failed"

            // Then: Should have error event with standardized error_details
            // ✅ REQUIRED: Use Eventually(), NEVER time.Sleep()
            Eventually(func() int {
                resp, _ := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
                    EventType:     &eventType,
                    CorrelationId: &correlationID,
                })
                if resp.JSON200 == nil {
                    return 0
                }
                return *resp.JSON200.Pagination.Total
            }, 30*time.Second, 1*time.Second).Should(Equal(1),
                "Should find exactly 1 error event")

            // Validate Gap #7: error_details (WILL FAIL - not standardized yet)
            resp, _ := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
                EventType:     &eventType,
                CorrelationId: &correlationID,
            })

            events := *resp.JSON200.Data
            Expect(len(events)).To(Equal(1))

            eventData := events[0].EventData.(map[string]interface{})

            // Standard error fields (WILL FAIL - not standardized yet)
            Expect(eventData).To(HaveKey("error_details"))
            errorDetails := eventData["error_details"].(map[string]interface{})

            Expect(errorDetails).To(HaveKey("message"))
            Expect(errorDetails["message"]).ToNot(BeEmpty())

            Expect(errorDetails).To(HaveKey("code"))
            Expect(errorDetails["code"]).ToNot(BeEmpty())

            Expect(errorDetails).To(HaveKey("component"))
            Expect(errorDetails["component"]).To(Equal("gateway"))

            Expect(errorDetails).To(HaveKey("retry_possible"))
            Expect(errorDetails["retry_possible"]).To(BeFalse()) // Invalid JSON not retryable
        })
    })
})
```

**Similar tests for**:
- AI Analysis: Test Holmes API failure
- Workflow Execution: Test workflow not found error
- Orchestrator: Test timeout configuration error

**Validation Command** (all tests MUST fail in RED phase):
```bash
# Test all 4 services in parallel (4 concurrent processes)
go test ./test/integration/gateway/audit_errors_test.go \
       ./test/integration/aianalysis/audit_errors_test.go \
       ./test/integration/workflowexecution/audit_errors_test.go \
       ./test/integration/remediationorchestrator/audit_errors_test.go \
       -v -p 4

# Expected: FAIL (error_details not standardized - CORRECT in RED phase)
```

---

#### **Phase 4: Do-GREEN** (4 hours) - MINIMAL IMPLEMENTATION

**Task 4.2**: Create shared error type (30 min)

**File**: `pkg/shared/audit/error_types.go` (NEW)

```go
package audit

// ErrorDetails provides standardized error structure for audit events
// Used in RR reconstruction (Gap #7)
type ErrorDetails struct {
    Message       string   `json:"message"`
    Code          string   `json:"code"`
    Component     string   `json:"component"`
    RetryPossible bool     `json:"retry_possible"`
    StackTrace    []string `json:"stack_trace,omitempty"`
}

// NewErrorDetails creates a standardized error detail
func NewErrorDetails(component, code, message string, retryPossible bool) *ErrorDetails {
    return &ErrorDetails{
        Message:       message,
        Code:          code,
        Component:     component,
        RetryPossible: retryPossible,
    }
}
```

**Task 4.3**: Update Gateway error events (50 min)

**File**: `pkg/gateway/audit_errors.go`

```go
package gateway

import "github.com/jordigilh/kubernaut/pkg/shared/audit"

func (p *SignalProcessor) emitErrorAuditEvent(ctx context.Context, err error, signal *corev1.Event) error {
    // Gap #7: Standardized error details for RR reconstruction
    errorDetails := audit.NewErrorDetails(
        "gateway",                   // component
        "SIGNAL_PROCESSING_FAILED",  // code
        err.Error(),                 // message
        isRetryable(err),            // retry_possible
    )

    return p.auditClient.CreateAuditEvent(ctx, &dsgen.AuditEvent{
        EventType:     "gateway.signal.failed",
        EventCategory: "gateway",
        EventAction:   "signal.failed",
        EventOutcome:  "failure",
        ActorType:     "system",
        ActorId:       "gateway",
        ResourceType:  "KubernetesEvent",
        ResourceName:  signal.Name,
        CorrelationId: extractCorrelationID(ctx),
        EventData: map[string]interface{}{
            "error_details": errorDetails, // Gap #7: Standardized error
            "signal_type":   signal.Type,
        },
    })
}

func isRetryable(err error) bool {
    // Classify errors as retryable or not
    // e.g., network errors = true, validation errors = false
    return false // Conservative default
}
```

**Task 4.4**: Update AI Analysis error events (50 min)

**File**: `internal/controller/aianalysis/audit_errors.go`

```go
import "github.com/jordigilh/kubernaut/pkg/shared/audit"

func (r *AIAnalysisReconciler) emitErrorAuditEvent(ctx context.Context, rr *v1alpha1.RemediationRequest, err error) error {
    errorDetails := audit.NewErrorDetails(
        "aianalysis",           // component
        "ANALYSIS_FAILED",      // code
        err.Error(),            // message
        isHolmesAPIRetryable(err), // retry_possible
    )

    return r.auditClient.CreateAuditEvent(ctx, &dsgen.AuditEvent{
        EventType:     "aianalysis.analysis.failed",
        EventCategory: "analysis",
        EventAction:   "analysis.failed",
        EventOutcome:  "failure",
        ActorType:     "service",
        ActorId:       "aianalysis-controller",
        ResourceType:  "RemediationRequest",
        ResourceName:  rr.Name,
        CorrelationId: string(rr.UID),
        EventData: map[string]interface{}{
            "error_details": errorDetails, // Gap #7
        },
    })
}
```

**Task 4.5**: Update Workflow Execution error events (50 min)

**File**: `internal/controller/workflowexecution/audit_errors.go`

**Task 4.6**: Update Orchestrator error events (50 min)

**File**: `internal/controller/remediationorchestrator/audit/error_helpers.go`

**Validation Command** (tests MUST pass in GREEN phase):
```bash
go test ./test/integration/{gateway,aianalysis,workflowexecution,remediationorchestrator}/audit_errors_test.go -v -p 4

# Expected: PASS (standardized error_details present)
```

---

#### **Phase 5: Do-REFACTOR** (1 hour)

**Optimization Tasks**:
- Add stack trace capture for critical errors
- Add error categorization helper
- Improve retry determination logic
- Add error code constants

**File**: `pkg/shared/audit/error_types.go`

```go
// Error code constants for consistency
const (
    ErrCodeInvalidPayload      = "INVALID_PAYLOAD"
    ErrCodeAPIFailure          = "API_FAILURE"
    ErrCodeResourceNotFound    = "RESOURCE_NOT_FOUND"
    ErrCodeTimeout             = "TIMEOUT"
    ErrCodeAuthenticationFailed = "AUTHENTICATION_FAILED"
)

// NewErrorDetailsWithStackTrace captures stack trace for debugging
func NewErrorDetailsWithStackTrace(component, code, message string, retryPossible bool, err error) *ErrorDetails {
    details := NewErrorDetails(component, code, message, retryPossible)

    // Capture stack trace for non-transient errors
    if !retryPossible && err != nil {
        details.StackTrace = captureStackTrace(err)
    }

    return details
}

func captureStackTrace(err error) []string {
    // Extract stack from error (implementation details)
    return []string{} // placeholder
}

// IsRetryableError determines if error allows retry
func IsRetryableError(err error) bool {
    // Network errors, timeouts = retryable
    // Validation errors, auth failures = not retryable
    // Implementation details...
    return false
}
```

**Validation**: Run tests again - should still pass:
```bash
go test ./test/integration/{gateway,aianalysis,workflowexecution,remediationorchestrator}/audit_errors_test.go -v -p 4
```

---

#### **Phase 6: Check** (10 min)

**Validation Checklist**:
- ✅ Gap #7 closed: Standardized `error_details` in all `*.failure` events
- ✅ 4 services updated: Gateway, AI Analysis, Workflow Execution, Orchestrator
- ✅ Integration tests passing (8 specs from test plan Section 4.2)
- ✅ OpenAPI client used for all audit queries
- ✅ No `time.Sleep()` in tests (used `Eventually()`)
- ✅ No `Skip()` in tests
- ✅ Tests validate business logic (error scenarios during operations), not infrastructure
- ✅ Parallel execution standard (`-p 4`) followed

**BR-AUDIT-005 Progress**: 90% complete (7/8 RR fields captured)

**Files Modified**:
- ✅ `pkg/shared/audit/error_types.go` (new shared error type)
- ✅ `pkg/gateway/audit_errors.go` (standardized errors)
- ✅ `internal/controller/aianalysis/audit_errors.go` (standardized errors)
- ✅ `internal/controller/workflowexecution/audit_errors.go` (standardized errors)
- ✅ `internal/controller/remediationorchestrator/audit/error_helpers.go` (standardized errors)
- ✅ 4 new integration test files

---

### **Day 5: TimeoutConfig Capture & RR Reconstruction** (11 hours)

**Goal**: Complete 100% RR reconstruction coverage (Gap #8 + full validation)

**Test Plan Reference**: [SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md](./SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md) Section 5.2

**Gaps Addressed**: #8 (`timeout_config`) + validate all 8 fields reconstructable

---

#### **Phase 1: Analyze** (10 min)

**Tasks**:
1. Review Orchestrator RR creation: `internal/controller/remediationorchestrator/controller.go`
2. Review audit helpers: `internal/controller/remediationorchestrator/audit/helpers.go`
3. Confirm all 7 previous gaps (Days 1-4) are implemented
4. Review RR reconstruction algorithm from DD-AUDIT-004

**Expected Findings**:
- `orchestration.remediation.created` event missing `timeout_config`
- All other 7 fields should be capturable from previous days
- Need to implement full RR reconstruction logic
- Reconstruction algorithm defined in DD-AUDIT-004

---

#### **Phase 2: Plan** (15 min)

**Implementation Strategy Part 1** (TimeoutConfig):
- Add `timeout_config` to Orchestrator audit event
- Capture custom timeout settings from RR spec

**Implementation Strategy Part 2** (RR Reconstruction):
- Implement reconstruction algorithm from DD-AUDIT-004
- Query audit events by correlation_id
- Reassemble RR CRD from 8 captured fields
- Use OpenAPI client for all audit queries

**Acceptance Criteria** (from BR-AUDIT-005):
- ✅ Gap #8: `event_data.timeout_config` captured
- ✅ 100% RR reconstruction: All 8 fields reassembled correctly
- ✅ Integration test validates full lifecycle reconstruction
- ✅ E2E test validates reconstruction after CRD TTL expiration

---

#### **Phase 3: Do-RED** (6 hours) - WRITE TESTS FIRST

**Task 5.1**: TimeoutConfig integration test (1 hour)

**File**: `test/integration/remediationorchestrator/audit_timeout_config_test.go` (NEW)

```go
var _ = Describe("BR-AUDIT-005: Orchestrator TimeoutConfig Audit", func() {
    var (
        dsClient  *dsgen.ClientWithResponses
        ctx       context.Context
        k8sClient client.Client
    )

    BeforeEach(func() {
        ctx = context.Background()
        dataStorageURL := os.Getenv("DATA_STORAGE_URL")

        var err error
        dsClient, err = dsgen.NewClientWithResponses(dataStorageURL)
        Expect(err).ToNot(HaveOccurred())

        // ✅ REQUIRED: Fail if Data Storage unavailable
        resp, err := http.Get(dataStorageURL + "/health")
        Expect(err).ToNot(HaveOccurred(),
            "REQUIRED: Data Storage not available")
        Expect(resp.StatusCode).To(Equal(http.StatusOK))

        k8sClient = createTestK8sClient()
    })

    Context("Gap #8: TimeoutConfig Capture", func() {
        It("should capture timeout config when RR is created", func() {
            // Given: RemediationRequest with custom timeout
            rr := &v1alpha1.RemediationRequest{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-rr",
                    Namespace: "default",
                },
                Spec: v1alpha1.RemediationRequestSpec{
                    TimeoutConfig: &v1alpha1.TimeoutConfig{
                        OverallTimeout:  300, // 5 minutes
                        StepTimeout:     60,  // 1 minute
                        ApprovalTimeout: 120, // 2 minutes
                    },
                },
            }
            err := k8sClient.Create(ctx, rr)
            Expect(err).ToNot(HaveOccurred())

            correlationID := string(rr.UID)
            eventType := "orchestration.remediation.created"

            // When: Orchestrator processes RR creation

            // Then: Should have timeout_config in audit event
            // ✅ REQUIRED: Use Eventually()
            Eventually(func() int {
                resp, _ := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
                    EventType:     &eventType,
                    CorrelationId: &correlationID,
                })
                if resp.JSON200 == nil {
                    return 0
                }
                return *resp.JSON200.Pagination.Total
            }, 60*time.Second, 2*time.Second).Should(Equal(1))

            // Validate Gap #8: timeout_config (WILL FAIL initially)
            resp, _ := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
                EventType:     &eventType,
                CorrelationId: &correlationID,
            })

            events := *resp.JSON200.Data
            eventData := events[0].EventData.(map[string]interface{})

            Expect(eventData).To(HaveKey("timeout_config"))
            timeoutConfig := eventData["timeout_config"].(map[string]interface{})

            Expect(timeoutConfig["overall_timeout"]).To(Equal(float64(300)))
            Expect(timeoutConfig["step_timeout"]).To(Equal(float64(60)))
            Expect(timeoutConfig["approval_timeout"]).To(Equal(float64(120)))
        })
    })
})
```

**Task 5.2**: Full RR reconstruction integration test (4 hours)

**File**: `test/integration/datastorage/rr_reconstruction_test.go` (NEW)

```go
var _ = Describe("BR-AUDIT-005: RemediationRequest Reconstruction", func() {
    var (
        dsClient  *dsgen.ClientWithResponses
        ctx       context.Context
        k8sClient client.Client
        testEnv   *envtest.Environment
    )

    BeforeEach(func() {
        ctx = context.Background()
        dataStorageURL := os.Getenv("DATA_STORAGE_URL")

        var err error
        dsClient, err = dsgen.NewClientWithResponses(dataStorageURL)
        Expect(err).ToNot(HaveOccurred())

        // ✅ REQUIRED: Fail if Data Storage unavailable
        resp, err := http.Get(dataStorageURL + "/health")
        Expect(err).ToNot(HaveOccurred(),
            "REQUIRED: Data Storage not available")
        Expect(resp.StatusCode).To(Equal(http.StatusOK))

        testEnv, k8sClient = setupEnvtest()
    })

    AfterEach(func() {
        testEnv.Stop()
    })

    Context("Full RR Lifecycle Reconstruction", func() {
        It("should reconstruct 100% of RR from audit traces", func() {
            // Given: Complete RR lifecycle (Gateway → Orchestrator)
            originalRR := createCompleteRR()
            err := k8sClient.Create(ctx, originalRR)
            Expect(err).ToNot(HaveOccurred())

            // Wait for full lifecycle to complete
            Eventually(func() bool {
                rr := &v1alpha1.RemediationRequest{}
                err := k8sClient.Get(ctx, client.ObjectKeyFromObject(originalRR), rr)
                return err == nil && rr.Status.Phase == "Completed"
            }, 5*time.Minute, 10*time.Second).Should(BeTrue())

            // Delete RR (simulating TTL expiration)
            err = k8sClient.Delete(ctx, originalRR)
            Expect(err).ToNot(HaveOccurred())

            // When: Reconstruct RR from audit traces
            correlationID := string(originalRR.UID)
            reconstructedRR := reconstructRRFromAudit(ctx, dsClient, correlationID)

            // Then: Validate all 8 fields reconstructed (100% coverage)

            // Gap #1: original_payload
            Expect(reconstructedRR.Spec.OriginalPayload).To(Equal(originalRR.Spec.OriginalPayload))

            // Gap #2: signal_labels
            Expect(reconstructedRR.Spec.SignalLabels).To(Equal(originalRR.Spec.SignalLabels))

            // Gap #3: signal_annotations
            Expect(reconstructedRR.Spec.SignalAnnotations).To(Equal(originalRR.Spec.SignalAnnotations))

            // Gap #4: provider_data
            Expect(reconstructedRR.Spec.AIAnalysis.ProviderData).To(Equal(originalRR.Spec.AIAnalysis.ProviderData))

            // Gap #5: selected_workflow_ref
            Expect(reconstructedRR.Status.SelectedWorkflowRef).To(Equal(originalRR.Status.SelectedWorkflowRef))

            // Gap #6: execution_ref
            Expect(reconstructedRR.Status.ExecutionRef).To(Equal(originalRR.Status.ExecutionRef))

            // Gap #7: error (if any)
            if originalRR.Status.Error != nil {
                Expect(reconstructedRR.Status.Error).To(Equal(originalRR.Status.Error))
            }

            // Gap #8: timeout_config
            Expect(reconstructedRR.Status.TimeoutConfig).To(Equal(originalRR.Status.TimeoutConfig))

            // Confidence: 100% reconstruction achieved
        })
    })
})

// reconstructRRFromAudit implements DD-AUDIT-004 reconstruction algorithm
func reconstructRRFromAudit(ctx context.Context, dsClient *dsgen.ClientWithResponses, correlationID string) *v1alpha1.RemediationRequest {
    rr := &v1alpha1.RemediationRequest{}

    // Step 1: Query all events for correlation_id
    resp, err := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
        CorrelationId: &correlationID,
        Limit:         ptr.To(100),
    })
    Expect(err).ToNot(HaveOccurred())
    Expect(resp.JSON200).ToNot(BeNil())

    events := *resp.JSON200.Data

    // Step 2: Extract fields by event type (per DD-AUDIT-004)
    for _, event := range events {
        eventData := event.EventData.(map[string]interface{})

        switch event.EventType {
        case "gateway.signal.received":
            // Gaps #1-3
            rr.Spec.OriginalPayload = eventData["original_payload"]
            rr.Spec.SignalLabels = convertToStringMap(eventData["signal_labels"])
            rr.Spec.SignalAnnotations = convertToStringMap(eventData["signal_annotations"])

        case "aianalysis.analysis.completed":
            // Gap #4
            rr.Spec.AIAnalysis = &v1alpha1.AIAnalysisResult{
                ProviderData: eventData["provider_data"],
            }

        case "workflow.selection.completed":
            // Gap #5
            rr.Status.SelectedWorkflowRef = convertToObjectRef(eventData["selected_workflow_ref"])

        case "execution.workflow.started":
            // Gap #6
            rr.Status.ExecutionRef = convertToObjectRef(eventData["execution_ref"])

        case "orchestration.remediation.created":
            // Gap #8
            rr.Status.TimeoutConfig = convertToTimeoutConfig(eventData["timeout_config"])

        default:
            // Check for failure events (Gap #7)
            if strings.HasSuffix(event.EventType, ".failed") {
                rr.Status.Error = convertToErrorDetails(eventData["error_details"])
            }
        }
    }

    return rr
}
```

**Task 5.3**: E2E reconstruction test in Kind cluster (1 hour)

**File**: `test/e2e/datastorage/rr_reconstruction_e2e_test.go` (NEW)

**Validation Command** (tests MUST fail in RED phase):
```bash
# Parallel execution (4 concurrent processes)
go test ./test/integration/remediationorchestrator/audit_timeout_config_test.go \
       ./test/integration/datastorage/rr_reconstruction_test.go \
       ./test/e2e/datastorage/rr_reconstruction_e2e_test.go \
       -v -p 4

# Expected: FAIL (timeout_config missing, reconstruction incomplete - CORRECT in RED phase)
```

---

#### **Phase 4: Do-GREEN** (3 hours) - MINIMAL IMPLEMENTATION

**Task 5.4**: Add timeout_config to Orchestrator event (1 hour)

**File**: `internal/controller/remediationorchestrator/audit/helpers.go`

```go
func BuildRemediationCreatedAuditEvent(rr *v1alpha1.RemediationRequest) *dsgen.AuditEvent {
    return &dsgen.AuditEvent{
        EventType:     "orchestration.remediation.created",
        EventCategory: "orchestration",
        EventAction:   "remediation.created",
        EventOutcome:  "success",
        ActorType:     "service",
        ActorId:       "remediationorchestrator-controller",
        ResourceType:  "RemediationRequest",
        ResourceName:  rr.Name,
        CorrelationId: string(rr.UID),
        EventData: map[string]interface{}{
            // Gap #8: TimeoutConfig for RR reconstruction
            "timeout_config": map[string]interface{}{
                "overall_timeout":  rr.Status.TimeoutConfig.OverallTimeout,
                "step_timeout":     rr.Status.TimeoutConfig.StepTimeout,
                "approval_timeout": rr.Status.TimeoutConfig.ApprovalTimeout,
            },
            "remediation_type": rr.Spec.RemediationType,
        },
    }
}
```

**Task 5.5**: Implement reconstruction helpers (2 hours)

**File**: `test/integration/datastorage/reconstruction_helpers.go` (NEW)

```go
package datastorage

import (
    dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
    v1alpha1 "github.com/jordigilh/kubernaut/api/v1alpha1"
)

// Helper functions for DD-AUDIT-004 reconstruction algorithm
func convertToStringMap(data interface{}) map[string]string {
    // Implementation
    return nil
}

func convertToObjectRef(data interface{}) *corev1.ObjectReference {
    // Implementation
    return nil
}

func convertToTimeoutConfig(data interface{}) *v1alpha1.TimeoutConfig {
    // Implementation
    return nil
}

func convertToErrorDetails(data interface{}) *v1alpha1.ErrorDetails {
    // Implementation
    return nil
}
```

**Validation Command** (tests MUST pass in GREEN phase):
```bash
go test ./test/integration/remediationorchestrator/audit_timeout_config_test.go \
       ./test/integration/datastorage/rr_reconstruction_test.go \
       ./test/e2e/datastorage/rr_reconstruction_e2e_test.go \
       -v -p 4

# Expected: PASS (100% RR reconstruction achieved)
```

---

#### **Phase 5: Do-REFACTOR** (1 hour)

**Optimization Tasks**:
- Add reconstruction confidence scoring
- Optimize audit query performance
- Add field validation during reconstruction
- Improve error handling for missing fields

**File**: `test/integration/datastorage/reconstruction_helpers.go`

```go
// ReconstructionResult includes confidence score
type ReconstructionResult struct {
    RR         *v1alpha1.RemediationRequest
    Confidence float64  // 0.0-1.0 (1.0 = 100% complete)
    MissingFields []string
}

func ReconstructRRFromAuditWithConfidence(
    ctx context.Context,
    dsClient *dsgen.ClientWithResponses,
    correlationID string,
) (*ReconstructionResult, error) {
    rr := &v1alpha1.RemediationRequest{}
    fieldsFound := 0
    totalFields := 8
    missing := []string{}

    // ... reconstruction logic ...

    // Calculate confidence
    if rr.Spec.OriginalPayload != nil {
        fieldsFound++
    } else {
        missing = append(missing, "original_payload")
    }

    // ... check all 8 fields ...

    confidence := float64(fieldsFound) / float64(totalFields)

    return &ReconstructionResult{
        RR:            rr,
        Confidence:    confidence,
        MissingFields: missing,
    }, nil
}
```

**Validation**: Run tests again - should still pass:
```bash
go test ./test/integration/datastorage/rr_reconstruction_test.go -v -p 4
```

---

#### **Phase 6: Check** (10 min)

**Validation Checklist**:
- ✅ Gap #8 closed: `timeout_config` captured in `orchestration.remediation.created` events
- ✅ 100% RR reconstruction: All 8 fields successfully reassembled from audit traces
- ✅ Integration tests passing (10 specs from test plan Section 5.2)
- ✅ E2E test passing (5 specs from test plan Section 5.3)
- ✅ OpenAPI client used for all audit queries
- ✅ No `time.Sleep()` in tests (used `Eventually()`)
- ✅ No `Skip()` in tests
- ✅ Tests validate business logic (full RR lifecycle), not infrastructure
- ✅ Parallel execution standard (`-p 4`) followed
- ✅ Reconstruction algorithm per DD-AUDIT-004 implemented

**BR-AUDIT-005 Progress**: 100% complete (8/8 RR fields captured + reconstruction validated)

**SOC2 Type II Compliance**: ✅ **ACHIEVED**

**Files Modified**:
- ✅ `internal/controller/remediationorchestrator/audit/helpers.go` (added timeout_config)
- ✅ `test/integration/remediationorchestrator/audit_timeout_config_test.go` (new test file)
- ✅ `test/integration/datastorage/rr_reconstruction_test.go` (new reconstruction test)
- ✅ `test/integration/datastorage/reconstruction_helpers.go` (new helper functions)
- ✅ `test/e2e/datastorage/rr_reconstruction_e2e_test.go` (new E2E test)

---

### **Day 6: Comprehensive Validation & Documentation** (4-5 hours)

**Goal**: Validate 100% RR reconstruction + document SOC2 compliance achievement

**Test Plan Reference**: [SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md](./SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md) Validation Section

**Validation Scope**: All 8 gaps + 50 test specs (32 integration + 18 E2E)

---

#### **Phase 1: Analyze** (15 min)

**Tasks**:
1. Review implementation status from Days 1-5
2. Confirm all 8 gaps are code-complete
3. Review test plan coverage (50 total specs)
4. Identify validation checkpoints

**Expected Findings**:
- ✅ Days 1-5 implementation complete
- ✅ 8/8 gaps closed (100% code complete)
- ✅ 50 test specs ready for execution
- ⏳ Need to validate end-to-end integration

---

#### **Phase 2: Plan** (10 min)

**Validation Strategy**:
1. **Integration Tests**: Execute all 32 integration tests (parallel: 4 processes)
2. **E2E Tests**: Execute all 18 E2E tests (parallel: 4 processes)
3. **Manual Validation**: RR reconstruction smoke test
4. **Documentation**: Compliance reporting
5. **PR Preparation**: Code review readiness

**Success Criteria** (from BR-AUDIT-005):
- ✅ 100% test pass rate (50/50 specs)
- ✅ 100% RR reconstruction accuracy
- ✅ SOC2 Type II compliance documented

---

#### **Phase 3: Execute Validation** (3 hours)

**Task 6.1**: Integration Test Validation (2 hours)

**Execute all integration tests with parallel execution**:
```bash
# Integration tests with 4 concurrent processes (standard)
go test ./test/integration/gateway/audit_signal_data_integration_test.go \
       ./test/integration/aianalysis/audit_provider_data_integration_test.go \
       ./test/integration/workflowexecution/audit_refs_integration_test.go \
       ./test/integration/gateway/audit_errors_test.go \
       ./test/integration/aianalysis/audit_errors_test.go \
       ./test/integration/workflowexecution/audit_errors_test.go \
       ./test/integration/remediationorchestrator/audit_errors_test.go \
       ./test/integration/remediationorchestrator/audit_timeout_config_test.go \
       ./test/integration/datastorage/rr_reconstruction_test.go \
       -v -p 4 -timeout=30m

# Expected: 32/32 specs passing
```

**Validation Requirements** (MANDATORY):
- ✅ OpenAPI client used in all tests
- ✅ `Eventually()` used (NO `time.Sleep()`)
- ✅ Tests fail explicitly (NO `Skip()`)
- ✅ Business logic validated (not infrastructure)
- ✅ Deterministic event counts (`Equal(N)`)
- ✅ Structured event_data validation

**Files Validated**:
- `test/integration/gateway/audit_signal_data_integration_test.go` (3 specs)
- `test/integration/aianalysis/audit_provider_data_integration_test.go` (3 specs)
- `test/integration/workflowexecution/audit_refs_integration_test.go` (3 specs)
- `test/integration/gateway/audit_errors_test.go` (2 specs)
- `test/integration/aianalysis/audit_errors_test.go` (2 specs)
- `test/integration/workflowexecution/audit_errors_test.go` (2 specs)
- `test/integration/remediationorchestrator/audit_errors_test.go` (2 specs)
- `test/integration/remediationorchestrator/audit_timeout_config_test.go` (3 specs)
- `test/integration/datastorage/rr_reconstruction_test.go` (10 specs)
- `test/integration/datastorage/audit_query_test.go` (2 specs)

**Task 6.2**: E2E Test Validation (1 hour)

**Execute all E2E tests with parallel execution**:
```bash
# E2E tests with 4 concurrent processes (standard)
go test ./test/e2e/datastorage/rr_reconstruction_e2e_test.go \
       ./test/e2e/gateway/audit_gateway_orchestrator_flow_test.go \
       -v -p 4 -timeout=60m

# Expected: 18/18 specs passing
```

**Validation Requirements**:
- ✅ Full Kind cluster deployed
- ✅ All services running (Gateway, AI Analysis, Workflow Execution, Orchestrator, Data Storage)
- ✅ Real Kubernetes resources (not mocks)
- ✅ 100% RR reconstruction from audit traces

**Files Validated**:
- `test/e2e/datastorage/rr_reconstruction_e2e_test.go` (5 specs)
- `test/e2e/gateway/audit_gateway_orchestrator_flow_test.go` (2 specs)
- `test/e2e/datastorage/06_workflow_search_audit_test.go` (3 specs)
- Other E2E audit tests (8 specs)

---

#### **Phase 4: Document Compliance** (1 hour)

**Task 6.3**: Create compliance validation report (45 min)

**File**: `docs/compliance/SOC2_AUDIT_RR_RECONSTRUCTION_VALIDATION_REPORT.md` (NEW)

**Content** (template):
```markdown
# SOC2 Audit - RR Reconstruction Validation Report

**Date**: [Current Date]
**Status**: ✅ VALIDATED - 100% RR Reconstruction Achieved
**Business Requirement**: BR-AUDIT-005 v2.0

## Executive Summary

This report validates the successful implementation of SOC2 Type II enterprise-grade
audit capabilities with 100% RemediationRequest (RR) CRD reconstruction accuracy.

**Achievement**: All 8 critical gaps closed, validated by 50 automated tests.

## Test Execution Results

### Integration Tests (32 specs)
- ✅ Gateway signal data capture: 3/3 specs passing
- ✅ AI Analysis provider data capture: 3/3 specs passing
- ✅ Workflow Execution refs capture: 3/3 specs passing
- ✅ Error details standardization: 8/8 specs passing
- ✅ TimeoutConfig capture: 3/3 specs passing
- ✅ RR reconstruction algorithm: 10/10 specs passing
- ✅ Audit query validation: 2/2 specs passing

**Total**: 32/32 passing (100%)

### E2E Tests (18 specs)
- ✅ Full RR reconstruction: 5/5 specs passing
- ✅ Gateway-to-Orchestrator flow: 2/2 specs passing
- ✅ Cross-service audit flow: 3/3 specs passing
- ✅ Other E2E audit scenarios: 8/8 specs passing

**Total**: 18/18 passing (100%)

### Field Coverage Validation

| # | Field | Source Event | Status |
|---|-------|--------------|--------|
| 1 | `.spec.originalPayload` | `gateway.signal.received` | ✅ VALIDATED |
| 2 | `.spec.signalLabels` | `gateway.signal.received` | ✅ VALIDATED |
| 3 | `.spec.signalAnnotations` | `gateway.signal.received` | ✅ VALIDATED |
| 4 | `.spec.aiAnalysis.providerData` | `aianalysis.analysis.completed` | ✅ VALIDATED |
| 5 | `.status.selectedWorkflowRef` | `workflow.selection.completed` | ✅ VALIDATED |
| 6 | `.status.executionRef` | `execution.workflow.started` | ✅ VALIDATED |
| 7 | `.status.error` | `*.failure` | ✅ VALIDATED |
| 8 | `.status.timeoutConfig` | `orchestration.remediation.created` | ✅ VALIDATED |

**Reconstruction Accuracy**: 100% (8/8 fields)

## SOC2 Compliance Status

**BR-AUDIT-005 v2.0 Requirements**:
- ✅ 100% RR CRD reconstruction from audit traces
- ✅ Event sourcing pattern implemented (ADR-034)
- ✅ Tamper-evident audit logs (immutable, append-only)
- ✅ 7+ years retention support (partitioned by date)
- ✅ Legal hold mechanism ready (retention_days field)
- ✅ Query API with RBAC (OpenAPI schema)

**Compliance Level**: 92% SOC2 Type II ready (enterprise-grade)

## Authority Documents

- [ADR-034](../../architecture/decisions/ADR-034-unified-audit-table-design.md): Unified audit table design
- [DD-AUDIT-003](../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md): Service audit requirements
- [DD-AUDIT-004](../../architecture/decisions/DD-AUDIT-004-RR-RECONSTRUCTION-FIELD-MAPPING.md): RR field mapping
- [DD-TESTING-001](../../architecture/decisions/DD-TESTING-001-audit-event-validation-standards.md): Validation standards

## Sign-Off

- **Developer**: [Name]
- **Tech Lead**: [Name]
- **Date**: [Date]
- **Status**: ✅ APPROVED FOR PRODUCTION
```

**Task 6.4**: Update DD-AUDIT-004 with validation status (15 min)

**File**: `docs/architecture/decisions/DD-AUDIT-004-RR-RECONSTRUCTION-FIELD-MAPPING.md`

**Update** (append to document):
```markdown
## Implementation Status

**Status**: ✅ **COMPLETE** (100% validated)
**Completed**: [Date]
**Validation**: See [SOC2_AUDIT_RR_RECONSTRUCTION_VALIDATION_REPORT.md](../../compliance/SOC2_AUDIT_RR_RECONSTRUCTION_VALIDATION_REPORT.md)

### Test Coverage
- **Integration Tests**: 32/32 passing (100%)
- **E2E Tests**: 18/18 passing (100%)
- **Reconstruction Accuracy**: 100% (8/8 fields)

### Compliance Achievement
- **BR-AUDIT-005 v2.0**: ✅ COMPLETE
- **SOC2 Type II**: 92% ready (enterprise-grade)
```

---

#### **Phase 5: PR Preparation** (30 min)

**Task 6.5**: Prepare code for review

**Pre-PR Checklist**:
```bash
# 1. Run full test suite with parallel execution
make test-all -j 4

# Expected: All tests passing

# 2. Run linter
golangci-lint run --timeout=10m

# Expected: 0 new issues

# 3. Verify no breaking changes
git diff main...HEAD --stat

# 4. Run audit-specific tests
go test ./test/integration/... ./test/e2e/... -v -p 4 -tags=audit

# Expected: 50/50 specs passing
```

**Task 6.6**: Create PR with detailed description

**PR Template**:
```markdown
# SOC2 Compliance: 100% RR Reconstruction from Audit Traces

## 🎯 Business Requirement
**BR-AUDIT-005 v2.0**: Enterprise-Grade Audit Integrity and Compliance

## 📊 Implementation Summary
- ✅ 8 critical gaps closed across 5 services
- ✅ 50 automated tests (32 integration + 18 E2E)
- ✅ 100% RR CRD reconstruction accuracy validated
- ✅ SOC2 Type II compliance: 92% ready (enterprise-grade)

## 📋 Changes by Service

### Gateway Service
- Added `original_payload`, `signal_labels`, `signal_annotations` to audit events
- Files: `pkg/gateway/signal_processor.go`, `pkg/gateway/audit_types.go`

### AI Analysis Service
- Added `provider_data` (Holmes response) to audit events
- Files: `internal/controller/aianalysis/controller.go`, `internal/controller/aianalysis/audit.go`

### Workflow Execution Service
- Added `selected_workflow_ref` and `execution_ref` to audit events
- Files: `internal/controller/workflowexecution/workflow_selector.go`, `internal/controller/workflowexecution/controller.go`

### Remediation Orchestrator
- Added `timeout_config` to audit events
- Files: `internal/controller/remediationorchestrator/audit/helpers.go`

### Shared
- Standardized error details structure across all services
- Files: `pkg/shared/audit/error_types.go`

## 🧪 Test Results

### Parallel Test Execution (4 concurrent processes)
```bash
go test ./test/integration/... ./test/e2e/... -v -p 4 -timeout=60m
```

- ✅ **Integration Tests**: 32/32 passing (100%)
- ✅ **E2E Tests**: 18/18 passing (100%)
- ✅ **Total Runtime**: ~15 minutes (parallelized)

### TDD Compliance
- ✅ All tests written FIRST (RED phase)
- ✅ Implementation followed (GREEN phase)
- ✅ Optimization applied (REFACTOR phase)
- ✅ OpenAPI client used (type-safe audit queries)
- ✅ No `time.Sleep()` (all use `Eventually()`)
- ✅ No `Skip()` (tests fail explicitly)

## 📚 Authority Documents
- [ADR-034](docs/architecture/decisions/ADR-034-unified-audit-table-design.md): Unified audit table design
- [DD-AUDIT-003](docs/architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md): Service audit requirements (v1.4)
- [DD-AUDIT-004](docs/architecture/decisions/DD-AUDIT-004-RR-RECONSTRUCTION-FIELD-MAPPING.md): RR field mapping (✅ Complete)
- [SOC2 Implementation Plan](docs/development/SOC2/SOC2_AUDIT_IMPLEMENTATION_PLAN.md): Work breakdown
- [SOC2 Test Plan](docs/development/SOC2/SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md): Test scenarios

## ✅ Validation Report
See [SOC2_AUDIT_RR_RECONSTRUCTION_VALIDATION_REPORT.md](docs/compliance/SOC2_AUDIT_RR_RECONSTRUCTION_VALIDATION_REPORT.md) for complete validation details.

## 🔒 SOC2 Compliance Achievement
**Status**: 92% SOC2 Type II ready (enterprise-grade)

**Capabilities**:
- ✅ 100% RR reconstruction from audit traces
- ✅ Event sourcing pattern (immutable, append-only)
- ✅ 7+ years retention support
- ✅ Legal hold mechanism
- ✅ RBAC-enabled query API
- ✅ Tamper-evident audit logs

**Ready For**: SOC 2 Type II audit, ISO 27001 certification
```

---

#### **Phase 6: Check** (15 min)

**Validation Checklist**:
- ✅ All 50 test specs passing (32 integration + 18 E2E)
- ✅ 100% RR reconstruction accuracy validated
- ✅ Compliance validation report created
- ✅ DD-AUDIT-004 updated with implementation status
- ✅ PR created with detailed description
- ✅ All authority documents updated
- ✅ Parallel execution standard (`-p 4`) followed throughout
- ✅ TDD methodology compliance validated
- ✅ OpenAPI client usage validated
- ✅ No anti-patterns (`time.Sleep()`, `Skip()`)

**BR-AUDIT-005 Status**: ✅ **100% COMPLETE**

**SOC2 Type II Status**: ✅ **92% READY** (enterprise-grade)

**Week 1 Completion**: ✅ **ALL 8 GAPS CLOSED + VALIDATED**

**Files Created/Modified**:
- ✅ `docs/compliance/SOC2_AUDIT_RR_RECONSTRUCTION_VALIDATION_REPORT.md` (new compliance report)
- ✅ `docs/architecture/decisions/DD-AUDIT-004-RR-RECONSTRUCTION-FIELD-MAPPING.md` (updated with validation status)
- ✅ Pull request created with comprehensive description

**Acceptance**:
- ✅ All tests passing
- ✅ No linter errors
- ✅ PR description follows template
- ✅ Ready for review

---

**Day 6 Total**: **4-5 hours**

---

## 📊 **Effort Summary**

### **Week 1: RR Reconstruction**

| Day | Focus | Implementation | Tests | Validation | Total Hours | Cumulative |
|-----|-------|---------------|-------|------------|-------------|------------|
| **Day 1** | Gateway signal data | 6h | 3h | - | 9h | 9h |
| **Day 2** | AI Analysis provider data | 6h | 3h | - | 9h | 18h |
| **Day 3** | Workflow execution refs | 3h | 2h | - | 5h | 23h |
| **Day 4** | Error details standardization | 6h | 4h | - | 10h | 33h |
| **Day 5** | TimeoutConfig + RR reconstruction | 3h | 8h | - | 11h | 44h |
| **Day 6** | Comprehensive validation | - | - | 4-5h | 4-5h | 48-49h |

**Week 1 Total**: **48-49 hours** (6 days for 1 developer)

### **Week 2-3: Operator Attribution**

| Days | Focus | Owner | Implementation | Tests | Total Hours | Cumulative |
|------|-------|-------|---------------|-------|-------------|------------|
| **Days 7-8** | Shared library + WE block clearance | WE Team | 14h | 6h | 20h | 68-69h |
| **Days 9-10** | RAR approval webhook | RO Team | 12h | 4h | 16h | 84-85h |
| **Days 11-12** | Workflow catalog webhook | DS Team | 12h | 4h | 16h | 100-101h |
| **Days 13-14** | Notification cancellation webhook | NT Team | 12h | 4h | 16h | 116-117h |
| **Days 15-16** | E2E testing & SOC2 compliance | All Teams | - | 16h | 16h | 132-133h |

**Week 2-3 Total**: **84 hours** (10.5 days for 1 developer per team, or parallelizable across 4 teams)

### **Overall SOC2 Implementation**

**Total Effort**: **132-133 hours** (16.5 days)
**Timeline**: **January 6-27, 2026** (3 weeks)
**Parallelization**: Week 2-3 can leverage 4 teams (WE, RO, DS, NT)
**Priority**: Quality and completeness over speed

**Timeline**:
- **Week 1 (Days 1-6)**: January 6-13, 2026 (RR Reconstruction)
- **Week 2-3 (Days 7-16)**: January 14-27, 2026 (Operator Attribution)

---

## 🎯 **Success Metrics**

### **Week 1 Completion Metrics**

**RR Reconstruction Accuracy**:
- **Before**: 10% (only basic fields)
- **After Week 1**: 100% (all 8 critical fields)
- **Target**: ✅ **100% Accurate Reconstruction**

**Compliance Score**:
- **Before**: 20% SOC2 compliance (basic audit trail)
- **After Week 1**: 65% SOC2 compliance (RR reconstruction only)
- **Target**: Partial compliance (missing operator attribution)

### **Week 2-3 Completion Metrics**

**Operator Attribution**:
- **Before**: 0% (no operator identity in audit events)
- **After Week 2-3**: 100% (all 5 operator actions captured)
- **Target**: ✅ **SOC2 CC8.1 Compliance**

**Final Compliance Score**:
- **Before**: 65% (Week 1 complete)
- **After Week 2-3**: 100% SOC2 Type II compliance
- **Target**: ✅ **Full SOC 2 Type II Readiness**

### **Enterprise Value**
- ✅ Complete audit trail: RR reconstruction + operator attribution
- ✅ SOC2 CC8.1 compliance (attribution requirement met)
- ✅ Production-ready audit system with authenticated user tracking
- ✅ Compliance documentation for auditors and sales conversations
- ✅ Foundation for future compliance extensions (ISO 27001, NIST 800-53)

---

## 🔗 **Related Documents**

### **Authority Documents**
- [AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md](../../handoff/AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md) - Complete 10.5-day plan
- [DD-AUDIT-004-RR-RECONSTRUCTION-FIELD-MAPPING.md](../../architecture/decisions/DD-AUDIT-004-RR-RECONSTRUCTION-FIELD-MAPPING.md) - Field mapping specification
- [ADR-034-unified-audit-table-design.md](../../architecture/decisions/ADR-034-unified-audit-table-design.md) - Audit table schema
- [DECISION_100_PERCENT_RR_RECONSTRUCTION_DEC_18_2025.md](../../handoff/DECISION_100_PERCENT_RR_RECONSTRUCTION_DEC_18_2025.md) - 100% coverage decision

### **Business Requirements**
- [BR-AUDIT-005 v2.0](../../requirements/11_SECURITY_ACCESS_CONTROL.md) - Enterprise audit integrity

### **Test Plan**
- [SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md](./SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md) - Week 1 test plan (RR reconstruction)

### **Week 2-3 Extension**
- [TRIAGE_OPERATOR_ACTIONS_SOC2_EXTENSION.md](./TRIAGE_OPERATOR_ACTIONS_SOC2_EXTENSION.md) - Operator action auditing (approved)

---

## 📝 **Next Steps**

### **Week 1: RR Reconstruction (Days 1-6)**
1. ✅ **This Plan**: SOC2 V1.0 MVP implementation work identified
2. ⏳ **Start Implementation**: Begin Day 1 (Gateway signal data capture)
3. ⏳ **Daily Standups**: Track progress against 6-day plan (5 days implementation + 1 day validation)
4. ⏳ **End of Day 5**: All 8 critical gaps closed, integration/E2E tests written
5. ⏳ **Day 6 Validation**: Execute all tests, manual RR reconstruction, compliance documentation
6. ⏳ **End of Day 6**: PR ready for review, 100% RR reconstruction validated

### **Week 2-3: Operator Action Auditing (Days 7-16)**

Detailed plan in: [TRIAGE_OPERATOR_ACTIONS_SOC2_EXTENSION.md](./TRIAGE_OPERATOR_ACTIONS_SOC2_EXTENSION.md)

#### **Days 7-8: Shared Webhook Library + WE Block Clearance** (20 hours, WE Team)

**Part 1: Shared Library** (12 hours):
1. ⏳ Create `pkg/authwebhook/` shared library
2. ⏳ Implement user identity extraction (`ExtractUser()`)
3. ⏳ Implement audit wrapper (`AuditClient`)
4. ⏳ Write 18 unit tests

**Part 2: WorkflowExecution Block Clearance** (8 hours):
5. ⏳ Update WorkflowExecution CRD schema (add `blockClearanceRequest`, `blockClearance` fields)
6. ⏳ Update DD-WEBHOOK-001 v1.1 (add WorkflowExecution CRD)
7. ⏳ Scaffold WorkflowExecution webhook (operator-sdk)
8. ⏳ Implement `ValidateUpdate()` for block clearance requests
9. ⏳ Emit `workflowexecution.block.cleared` audit event with operator identity
10. ⏳ Update WorkflowExecution controller to detect `blockClearance` and unblock
11. ⏳ Write integration tests with authenticated operators

**Deliverable**: ✅ Reusable authentication library + WE block clearance webhook

#### **Days 9-10: RAR Approval Webhook** (16 hours, RO Team)
1. ⏳ Update DD-WEBHOOK-001 v1.1 (add RemediationApprovalRequest CRD)
2. ⏳ Scaffold RemediationApprovalRequest webhook (operator-sdk)
3. ⏳ Implement `ValidateUpdate()` for approval/rejection
4. ⏳ Wire to existing `orchestrator.approval.*` events (add operator identity)
5. ⏳ Write integration tests with authenticated users
6. ⏳ **Deliverable**: RAR webhook with operator attribution

#### **Days 11-12: Workflow Catalog Webhook** (16 hours, Data Storage Team)
1. ⏳ Update DD-WEBHOOK-001 v1.1 (add RemediationWorkflow CRD)
2. ⏳ Scaffold RemediationWorkflow webhook (operator-sdk)
3. ⏳ Implement `ValidateCreate()` for workflow creation
4. ⏳ Wire to existing `datastorage.workflow.*` events
5. ⏳ **Deliverable**: Workflow catalog with operator attribution

#### **Days 13-14: Notification Cancellation Webhook** (16 hours, Notification Team)
1. ⏳ Update DD-WEBHOOK-001 v1.1 (add NotificationRequest CRD)
2. ⏳ Update DD-AUDIT-003 v1.4 (add `notification.request.cancelled` event)
3. ⏳ Scaffold NotificationRequest webhook (operator-sdk)
4. ⏳ Implement `ValidateDelete()` for cancellation
5. ⏳ Emit `notification.request.cancelled` audit event with operator identity
6. ⏳ Wire to audit system via `pkg/authwebhook`
7. ⏳ Write integration tests with authenticated operators
8. ⏳ **Deliverable**: Notification cancellation with operator attribution

#### **Days 15-16: E2E Testing & SOC2 Compliance** (16 hours, All Teams)
1. ⏳ E2E test: Operator clears WorkflowExecution block → audit event captured
2. ⏳ E2E test: Operator approves RAR → audit event captured
3. ⏳ E2E test: Operator cancels NotificationRequest → audit event captured
4. ⏳ E2E test: Operator creates workflow → audit event captured
5. ⏳ E2E test: Operator disables workflow → audit event captured
6. ⏳ SOC2 CC8.1 compliance documentation
7. ⏳ Create compliance validation report
8. ⏳ **Deliverable**: 100% SOC2 Type II compliance (RR reconstruction + 5 operator actions)

### **Post-Week 3: Final System Validation**
- Full system E2E testing with operator actions
- OOMKill scenario with complete audit trail (RR reconstruction + operator actions)
- SOC2 compliance evidence package for auditors

---

**Document Status**: ✅ **READY TO START - FULL SOC2 COMPLIANCE (WEEK 1-3)**
**Implementation Work**: **READY FOR IMPLEMENTATION** (Days 1-16)
**User Approval**: Time is not a constraint - prioritizing quality and completeness
**Scope**:
- **Week 1 (Days 1-6)**: RR Reconstruction (48-49 hours)
- **Week 2-3 (Days 7-16)**: Operator Attribution (84 hours, 5 operator actions)
- **Total**: 132-133 hours (16.5 days)

**Next Action**: Begin Day 1 implementation (Gateway signal data capture)

**Timeline**:
- **Week 1 (Days 1-5)**: January 6-10, 2026 (Implementation + Tests)
- **Week 1 (Day 6)**: January 13, 2026 (Validation + Documentation)
- **Week 2-3 (Days 7-16)**: January 14-27, 2026 (Operator Actions + Webhooks)

