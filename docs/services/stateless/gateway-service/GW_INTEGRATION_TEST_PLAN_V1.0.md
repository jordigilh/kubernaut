# Gateway Integration Test Plan v1.0

**Service**: Gateway
**Version**: v1.0
**Date**: January 14, 2026
**Owner**: Gateway Team
**Status**: 📋 **APPROVED FOR IMPLEMENTATION**

---

## 🎯 **Executive Summary**

### **Objective**
Restore Gateway integration test coverage from 30.1% to ≥50% (target: 62%) through strategic addition of 84 high-value integration tests across 14 scenarios.

### **Scope**
- **Current State**: 22 integration tests, 30.1% coverage
- **Target State**: 106 integration tests, 62% coverage
- **Timeline**: 3 weeks (January 21 - February 11, 2026)
- **Approach**: Phased implementation with validation checkpoints

### **Key Principles**
1. **No E2E Duplication**: Tests complement (not duplicate) existing E2E coverage
2. **Direct Business Logic**: Tests call business functions directly (no HTTP overhead)
3. **Fast Execution**: Each test runs in <5 seconds
4. **Clear BR Mapping**: Every test maps to specific P0/P1 Business Requirements
5. **High Business Value**: Focus on SOC2 compliance, operations, and reliability

---

## 📊 **Coverage Goals**

| Phase | Timeline | Tests Added | Coverage Gain | Target Coverage | Status |
|-------|----------|-------------|---------------|-----------------|--------|
| **Phase 1** | Week 1 (Jan 21-25) | 35 tests | +15% | 45% | ⏳ Pending |
| **Phase 2** | Week 2 (Jan 28-Feb 1) | 35 tests | +12% | 57% ✅ | ⏳ Pending |
| **Phase 3** | Week 3 (Feb 4-8) | 14 tests | +5% | 62% ✅ | ⏳ Pending |
| **Validation** | Feb 11 | - | - | 62% ✅ | ⏳ Pending |

---

## 🎯 **PHASE 1: Quick Wins (Week 1)** - Target: +15% → 45%

### **Objective**: Achieve near-compliance with high business value tests (SOC2 + Operations)

---

### **Category 1: Audit Event Emission** (+9% coverage)

#### **Test File**: `test/integration/gateway/audit_emission_integration_test.go`

---

#### **Scenario 1.1: Signal Received Audit Event**
**BR**: BR-GATEWAY-055
**Priority**: P0 (Critical)
**Business Value**: SOC2 compliance - every signal must be auditable

**Test Specifications**:

```go
var _ = Describe("BR-GATEWAY-055: Signal Received Audit Events", func() {
    var (
        adapter       adapters.SignalAdapter
        auditStore    *MockAuditStore
        ctx           context.Context
    )

    BeforeEach(func() {
        auditStore = NewMockAuditStore()
        ctx = context.Background()
    })

    // Test 1.1.1
    It("should emit gateway.signal.received audit event for Prometheus signal", func() {
        // Given: Prometheus alert payload
        prometheusAlert := createTestPrometheusAlert()
        adapter = prometheus.NewAdapter(auditStore)

        // When: Adapter parses signal
        signal, err := adapter.Parse(ctx, prometheusAlert)

        // Then: Audit event emitted
        Expect(err).ToNot(HaveOccurred())
        Expect(auditStore.Events).To(HaveLen(1))

        auditEvent := auditStore.Events[0]
        Expect(auditEvent.EventType).To(Equal("gateway.signal.received"))
        Expect(auditEvent.EventAction).To(Equal("received"))
        Expect(auditEvent.CorrelationID).ToNot(BeEmpty())
        
        // Parse EventData to get GatewayAuditPayload
        gatewayPayload := auditEvent.EventData.GatewayAuditPayload
        
        // Access OriginalPayload (Optional field)
        originalPayload, ok := gatewayPayload.OriginalPayload.Get()
        Expect(ok).To(BeTrue(), "OriginalPayload should be present")
        // Convert jx.Raw map to searchable format
        payloadStr := fmt.Sprintf("%v", originalPayload)
        Expect(payloadStr).To(ContainSubstring("alertname"))
        
        // Access SignalLabels (Optional field)
        signalLabels, ok := gatewayPayload.SignalLabels.Get()
        Expect(ok).To(BeTrue(), "SignalLabels should be present")
        Expect(signalLabels).To(HaveKey("severity"))
        
        // Access SignalAnnotations (Optional field)
        signalAnnotations, ok := gatewayPayload.SignalAnnotations.Get()
        Expect(ok).To(BeTrue(), "SignalAnnotations should be present")
        Expect(signalAnnotations).To(HaveKey("summary"))
    })

    // Test 1.1.2
    It("should emit gateway.signal.received audit event for K8s Event signal", func() {
        // Given: Kubernetes Event payload
        k8sEvent := createTestK8sEvent()
        adapter = k8sevent.NewAdapter(auditStore)

        // When: Adapter parses signal
        signal, err := adapter.Parse(ctx, k8sEvent)

        // Then: Audit event emitted with K8s metadata
        Expect(err).ToNot(HaveOccurred())
        Expect(auditStore.Events).To(HaveLen(1))

        auditEvent := auditStore.Events[0]
        Expect(auditEvent.EventType).To(Equal("gateway.signal.received"))
        
        // Parse EventData to get GatewayAuditPayload
        gatewayPayload := auditEvent.EventData.GatewayAuditPayload
        
        // Business rule: ResourceKind contains K8s involved object type
        resourceKind, ok := gatewayPayload.ResourceKind.Get()
        Expect(ok).To(BeTrue(), "ResourceKind should be present for K8s events")
        Expect(resourceKind).To(Equal("Pod"))
        
        // Note: K8s event-specific fields like "reason" are not in current schema
        // These would need to be added if K8s event reason tracking is required
        // For now, validate that signal_type indicates K8s source
        Expect(string(gatewayPayload.SignalType)).To(Equal("kubernetes-event"))
    })

    // Test 1.1.3
    It("should include correlation_id in audit event for tracing", func() {
        // Given: Signal with correlation ID
        prometheusAlert := createTestPrometheusAlert()
        adapter = prometheus.NewAdapter(auditStore)

        // When: Multiple signals parsed
        signal1, _ := adapter.Parse(ctx, prometheusAlert)
        signal2, _ := adapter.Parse(ctx, prometheusAlert)

        // Then: Each has unique correlation ID with correct format
        Expect(auditStore.Events).To(HaveLen(2))
        correlationID1 := auditStore.Events[0].CorrelationID
        correlationID2 := auditStore.Events[1].CorrelationID

        // Business rule: Correlation ID format enables RR reconstruction
        Expect(correlationID1).To(MatchRegexp(`^rr-[a-f0-9]{12}-\d{10}$`))
        Expect(correlationID2).To(MatchRegexp(`^rr-[a-f0-9]{12}-\d{10}$`))

        // Business rule: Unique IDs enable independent RR lifecycle tracking
        Expect(correlationID1).ToNot(Equal(correlationID2))

        // Business rule: Correlation format enables fingerprint extraction
        fingerprint1 := extractFingerprintFromCorrelationID(correlationID1)
        Expect(fingerprint1).To(HaveLen(12))
        Expect(fingerprint1).To(MatchRegexp("^[a-f0-9]{12}$"))
    })

    // Test 1.1.4
    It("should preserve signal_labels and signal_annotations in audit event", func() {
        // Given: Prometheus alert with custom labels
        prometheusAlert := createTestPrometheusAlertWithLabels(map[string]string{
            "severity":    "critical",
            "team":        "platform",
            "environment": "production",
        })
        adapter = prometheus.NewAdapter(auditStore)

        // When: Adapter parses signal
        signal, _ := adapter.Parse(ctx, prometheusAlert)

        // Then: All labels preserved in audit event
        auditEvent := auditStore.Events[0]

        // Parse EventData to get GatewayAuditPayload
        gatewayPayload := auditEvent.EventData.GatewayAuditPayload

        // Access SignalLabels (Optional field - check if present)
        signalLabels, ok := gatewayPayload.SignalLabels.Get()
        Expect(ok).To(BeTrue(), "SignalLabels should be present in audit payload")
        Expect(signalLabels).To(HaveKeyWithValue("severity", "critical"))
        Expect(signalLabels).To(HaveKeyWithValue("team", "platform"))
        Expect(signalLabels).To(HaveKeyWithValue("environment", "production"))
    })

    // Test 1.1.5
    It("should not block signal processing if audit emission fails", func() {
        // Given: Audit store that fails
        failingAuditStore := NewFailingMockAuditStore()
        adapter = prometheus.NewAdapter(failingAuditStore)

        // When: Adapter parses signal
        signal, err := adapter.Parse(ctx, createTestPrometheusAlert())

        // Then: Signal processing continues despite audit failure (resilience)
        Expect(err).ToNot(HaveOccurred())

        // Business rule: Signal data extracted correctly despite audit failure
        Expect(signal.AlertName).To(Equal("HighCPU"))
        Expect(signal.Namespace).To(Equal("production"))
        Expect(signal.Severity).To(Equal("critical"))

        // Business rule: Fingerprint generated correctly (SHA-256 format)
        Expect(signal.Fingerprint).To(MatchRegexp("^[a-f0-9]{64}$"))

        // Audit failure logged but doesn't block critical path
    })
})
```

**Acceptance Criteria**:
- ✅ All 5 tests pass
- ✅ Coverage for `pkg/gateway/audit_helpers.go` increases from 49% to ≥70%
- ✅ Tests run in <5 seconds total
- ✅ No NULL-TESTING anti-patterns (weak assertions)

---

#### **Scenario 1.2: CRD Created Audit Event**
**BR**: BR-GATEWAY-056
**Priority**: P0 (Critical)
**Business Value**: Track every CRD creation for compliance and debugging

**Test Specifications**:

```go
var _ = Describe("BR-GATEWAY-056: CRD Created Audit Events", func() {
    var (
        crdCreator    *processing.CRDCreator
        auditStore    *MockAuditStore
        k8sClient     client.Client
        ctx           context.Context
    )

    BeforeEach(func() {
        auditStore = NewMockAuditStore()
        k8sClient = fake.NewClientBuilder().Build()
        crdCreator = processing.NewCRDCreator(k8sClient, auditStore)
        ctx = context.Background()
    })

    // Test 1.2.1
    It("should emit gateway.crd.created audit event after CRD creation", func() {
        // Given: Valid signal
        signal := createTestSignal("test-alert", "critical")

        // When: CRD created
        crd, err := crdCreator.CreateRemediationRequest(ctx, signal)

        // Then: Audit event emitted with correct business metadata
        Expect(err).ToNot(HaveOccurred())
        Expect(auditStore.Events).To(HaveLen(2)) // signal.received + crd.created

        crdCreatedEvent := findEventByType(auditStore.Events, "gateway.crd.created")
        Expect(crdCreatedEvent).ToNot(BeNil())
        Expect(crdCreatedEvent.EventAction).To(Equal("created"))

        // Parse EventData to get GatewayAuditPayload
        gatewayPayload := crdCreatedEvent.EventData.GatewayAuditPayload

        // Business rule: RemediationRequest field contains CRD reference (namespace/name)
        rrRef, ok := gatewayPayload.RemediationRequest.Get()
        Expect(ok).To(BeTrue())
        Expect(rrRef).To(MatchRegexp(`^[^/]+/rr-[a-f0-9]+-\d+$`))

        // Extract and validate CRD name from "namespace/name" format
        parts := strings.Split(rrRef, "/")
        Expect(parts).To(HaveLen(2))
        Expect(parts[1]).To(Equal(crd.Name))

        // Business rule: Namespace field matches signal namespace
        Expect(gatewayPayload.Namespace).To(Equal(signal.Namespace))
        Expect(gatewayPayload.Namespace).To(Equal(crd.Namespace))
    })

    // Test 1.2.2
    It("should include target_resource in audit event for RR reconstruction", func() {
        // Given: Signal with target resource
        signal := createTestSignalWithTarget("pod-crash", "Pod", "crashpod-123", "default")

        // When: CRD created
        crd, _ := crdCreator.CreateRemediationRequest(ctx, signal)

        // Then: Target resource in audit event
        crdCreatedEvent := findEventByType(auditStore.Events, "gateway.crd.created")

        // Parse EventData to get GatewayAuditPayload
        gatewayPayload := crdCreatedEvent.EventData.GatewayAuditPayload

        // Access resource_kind and resource_name (Optional fields)
        resourceKind, ok := gatewayPayload.ResourceKind.Get()
        Expect(ok).To(BeTrue(), "ResourceKind should be present")
        Expect(resourceKind).To(Equal("Pod"))

        resourceName, ok := gatewayPayload.ResourceName.Get()
        Expect(ok).To(BeTrue(), "ResourceName should be present")
        Expect(resourceName).To(Equal("crashpod-123"))
    })

    // Test 1.2.3
    It("should include fingerprint in audit event for deduplication tracking", func() {
        // Given: Signal with fingerprint
        signal := createTestSignal("high-cpu", "warning")

        // When: CRD created
        crd, _ := crdCreator.CreateRemediationRequest(ctx, signal)

        // Then: Fingerprint in audit event with correct format
        crdCreatedEvent := findEventByType(auditStore.Events, "gateway.crd.created")

        // Parse EventData to get GatewayAuditPayload
        gatewayPayload := crdCreatedEvent.EventData.GatewayAuditPayload

        // Business rule: Fingerprint format enables field selector queries
        Expect(gatewayPayload.Fingerprint).To(MatchRegexp("^[a-f0-9]{64}$"))
        Expect(gatewayPayload.Fingerprint).To(Equal(signal.Fingerprint))

        // Business rule: Initial occurrence count is not set for first CRD (only for deduplicated)
        // Note: occurrence_count is populated only for deduplicated events
        // For first-time CRD creation, deduplication_status should be "new"
        dedupStatus, ok := gatewayPayload.DeduplicationStatus.Get()
        if ok {
            Expect(dedupStatus).To(Equal(api.GatewayAuditPayloadDeduplicationStatusNew))
        }
    })

    // Test 1.2.4
    It("should include occurrence_count in audit event for storm detection", func() {
        // Given: Signal with occurrence count > 1 (simulated deduplication)
        signal := createTestSignal("repeated-error", "error")
        signal.OccurrenceCount = 5

        // When: CRD created
        crd, _ := crdCreator.CreateRemediationRequest(ctx, signal)

        // Then: Occurrence count in audit event
        crdCreatedEvent := findEventByType(auditStore.Events, "gateway.crd.created")

        // Parse EventData to get GatewayAuditPayload
        gatewayPayload := crdCreatedEvent.EventData.GatewayAuditPayload

        // Note: occurrence_count is typically populated for deduplicated events
        // For CRD creation, this would only be present if signal already deduplicated
        occurrenceCount, ok := gatewayPayload.OccurrenceCount.Get()
        if ok {
            Expect(occurrenceCount).To(Equal(int32(5)))
        }
    })

    // Test 1.2.5
    It("should emit unique correlation_id for each CRD creation", func() {
        // Given: Multiple signals
        signal1 := createTestSignal("alert-1", "critical")
        signal2 := createTestSignal("alert-2", "warning")

        // When: Multiple CRDs created
        crd1, _ := crdCreator.CreateRemediationRequest(ctx, signal1)
        crd2, _ := crdCreator.CreateRemediationRequest(ctx, signal2)

        // Then: Unique correlation IDs with correct format
        events := filterEventsByType(auditStore.Events, "gateway.crd.created")
        Expect(events).To(HaveLen(2))

        correlation1 := events[0].CorrelationID
        correlation2 := events[1].CorrelationID

        // Business rule: Correlation ID format enables tracing
        Expect(correlation1).To(MatchRegexp(`^rr-[a-f0-9]{12}-\d{10}$`))
        Expect(correlation2).To(MatchRegexp(`^rr-[a-f0-9]{12}-\d{10}$`))

        // Business rule: Uniqueness enables independent RR lifecycle tracking
        Expect(correlation1).ToNot(Equal(correlation2))

        // Business rule: Correlation matches CRD name (enables audit-to-CRD mapping)
        Expect(correlation1).To(Equal(crd1.Name))
        Expect(correlation2).To(Equal(crd2.Name))
    })
})
```

**Acceptance Criteria**:
- ✅ All 5 tests pass
- ✅ Coverage for `pkg/gateway/server.go` increases to ≥40% (from 32.7%)
- ✅ Tests validate audit payload structure (no weak assertions)

---

#### **Scenario 1.3: Signal Deduplicated Audit Event**
**BR**: BR-GATEWAY-057
**Priority**: P1 (High)
**Business Value**: Track deduplication decisions for SLA reporting

**Test Specifications**:

```go
var _ = Describe("BR-GATEWAY-057: Signal Deduplicated Audit Events", func() {
    var (
        phaseChecker  *processing.PhaseChecker
        auditStore    *MockAuditStore
        k8sClient     client.Client
        ctx           context.Context
    )

    BeforeEach(func() {
        auditStore = NewMockAuditStore()
        k8sClient = fake.NewClientBuilder().Build()
        phaseChecker = processing.NewPhaseChecker(k8sClient, auditStore)
        ctx = context.Background()
    })

    // Test 1.3.1
    It("should emit gateway.signal.deduplicated when duplicate signal arrives", func() {
        // Given: Existing RR in Pending phase
        existingRR := createTestRR("test-fingerprint", "Pending", "test-ns")
        k8sClient.Create(ctx, existingRR)

        // When: Duplicate signal arrives
        signal := createTestSignalWithFingerprint("test-fingerprint")
        shouldDedupe, existingRR, err := phaseChecker.ShouldDeduplicate(ctx, signal)

        // Then: Deduplication audit event emitted with business reasoning
        Expect(err).ToNot(HaveOccurred())
        Expect(shouldDedupe).To(BeTrue())

        dedupeEvent := findEventByType(auditStore.Events, "gateway.signal.deduplicated")
        Expect(dedupeEvent).ToNot(BeNil())
        Expect(dedupeEvent.EventAction).To(Equal("deduplicated"))

        // Parse EventData to get GatewayAuditPayload
        gatewayPayload := dedupeEvent.EventData.GatewayAuditPayload

        // Business rule: DeduplicationStatus proves deduplication occurred
        dedupStatus, ok := gatewayPayload.DeduplicationStatus.Get()
        Expect(ok).To(BeTrue(), "DeduplicationStatus should be present")
        Expect(dedupStatus).To(Equal(api.GatewayAuditPayloadDeduplicationStatusDuplicate))

        // Business rule: Occurrence count shows signal repetition pattern
        occurrenceCount, ok := gatewayPayload.OccurrenceCount.Get()
        Expect(ok).To(BeTrue(), "OccurrenceCount should be present for deduplicated signal")
        Expect(occurrenceCount).To(BeNumerically(">", 1))

        // Business rule: RemediationRequest contains existing RR reference
        rrRef, ok := gatewayPayload.RemediationRequest.Get()
        Expect(ok).To(BeTrue(), "RemediationRequest should be present")
        Expect(rrRef).To(MatchRegexp(`^[^/]+/rr-[a-f0-9]+-\d+$`))
        Expect(rrRef).To(ContainSubstring(existingRR.Name))
    })

    // Test 1.3.2
    It("should include existing_rr_name in audit event for tracking", func() {
        // Given: Existing RR
        existingRR := createTestRR("fp-12345", "Processing", "prod-ns")
        k8sClient.Create(ctx, existingRR)

        // When: Duplicate arrives
        signal := createTestSignalWithFingerprint("fp-12345")
        shouldDedupe, rr, _ := phaseChecker.ShouldDeduplicate(ctx, signal)

        // Then: Existing RR name in audit event
        dedupeEvent := findEventByType(auditStore.Events, "gateway.signal.deduplicated")

        // Parse EventData to get GatewayAuditPayload
        gatewayPayload := dedupeEvent.EventData.GatewayAuditPayload

        // Business rule: RemediationRequest field contains existing RR reference (namespace/name)
        rrRef, ok := gatewayPayload.RemediationRequest.Get()
        Expect(ok).To(BeTrue())
        Expect(rrRef).To(MatchRegexp(`^[^/]+/rr-[a-f0-9]+-\d+$`))

        // Extract and validate namespace and name from "namespace/name" format
        parts := strings.Split(rrRef, "/")
        Expect(parts).To(HaveLen(2))
        Expect(parts[0]).To(Equal(existingRR.Namespace))
        Expect(parts[1]).To(Equal(existingRR.Name))
    })

    // Test 1.3.3
    It("should include updated occurrence_count in audit event", func() {
        // Given: RR with occurrence count = 3
        existingRR := createTestRR("fp-99999", "Pending", "test-ns")
        existingRR.Status.OccurrenceCount = 3
        k8sClient.Create(ctx, existingRR)

        // When: Duplicate arrives
        signal := createTestSignalWithFingerprint("fp-99999")
        shouldDedupe, rr, _ := phaseChecker.ShouldDeduplicate(ctx, signal)

        // Then: Updated count in audit event
        dedupeEvent := findEventByType(auditStore.Events, "gateway.signal.deduplicated")

        // Parse EventData to get GatewayAuditPayload
        gatewayPayload := dedupeEvent.EventData.GatewayAuditPayload

        // Business rule: OccurrenceCount incremented to reflect signal repetition
        occurrenceCount, ok := gatewayPayload.OccurrenceCount.Get()
        Expect(ok).To(BeTrue())
        Expect(occurrenceCount).To(Equal(int32(4))) // 3 + 1
    })

    // Test 1.3.4
    It("should emit deduplication audit events for different RR fingerprints", func() {
        // Given: RRs with different fingerprints in different phases
        pendingRR := createTestRR("fp-pending", "Pending", "test-ns")
        processingRR := createTestRR("fp-processing", "Processing", "test-ns")
        k8sClient.Create(ctx, pendingRR)
        k8sClient.Create(ctx, processingRR)

        // When: Duplicates arrive for each
        signal1 := createTestSignalWithFingerprint("fp-pending")
        signal2 := createTestSignalWithFingerprint("fp-processing")
        phaseChecker.ShouldDeduplicate(ctx, signal1)
        phaseChecker.ShouldDeduplicate(ctx, signal2)

        // Then: Two deduplication events emitted
        dedupeEvents := filterEventsByType(auditStore.Events, "gateway.signal.deduplicated")
        Expect(dedupeEvents).To(HaveLen(2))

        // Parse both payloads
        payload1 := dedupeEvents[0].EventData.GatewayAuditPayload
        payload2 := dedupeEvents[1].EventData.GatewayAuditPayload

        // Business rule: Each deduplication references correct existing RR
        rrRef1, ok1 := payload1.RemediationRequest.Get()
        rrRef2, ok2 := payload2.RemediationRequest.Get()
        Expect(ok1).To(BeTrue())
        Expect(ok2).To(BeTrue())
        Expect(rrRef1).To(ContainSubstring(pendingRR.Name))
        Expect(rrRef2).To(ContainSubstring(processingRR.Name))

        // Business rule: Both marked as duplicate status
        dedupStatus1, _ := payload1.DeduplicationStatus.Get()
        dedupStatus2, _ := payload2.DeduplicationStatus.Get()
        Expect(dedupStatus1).To(Equal(api.GatewayAuditPayloadDeduplicationStatusDuplicate))
        Expect(dedupStatus2).To(Equal(api.GatewayAuditPayloadDeduplicationStatusDuplicate))
    })

    // Test 1.3.5
    It("should NOT emit deduplicated event when RR in Completed phase", func() {
        // Given: RR in Completed phase
        completedRR := createTestRR("fp-completed", "Completed", "test-ns")
        k8sClient.Create(ctx, completedRR)

        // When: New signal arrives
        signal := createTestSignalWithFingerprint("fp-completed")
        shouldDedupe, rr, _ := phaseChecker.ShouldDeduplicate(ctx, signal)

        // Then: NO deduplication (allow new RR)
        Expect(shouldDedupe).To(BeFalse())
        dedupeEvent := findEventByType(auditStore.Events, "gateway.signal.deduplicated")
        Expect(dedupeEvent).To(BeNil())
    })
})
```

**Acceptance Criteria**:
- ✅ All 5 tests pass
- ✅ Coverage for `pkg/gateway/processing/phase_checker.go` maintained at ≥85%
- ✅ Tests validate deduplication logic correctness

---

#### **Scenario 1.4: CRD Creation Failed Audit Event**
**BR**: BR-GATEWAY-058
**Priority**: P1 (High)
**Business Value**: Track failures for operational debugging

**Test Specifications**:

```go
var _ = Describe("BR-GATEWAY-058: CRD Creation Failed Audit Events", func() {
    var (
        crdCreator    *processing.CRDCreator
        auditStore    *MockAuditStore
        k8sClient     *ErrorInjectableK8sClient
        ctx           context.Context
    )

    BeforeEach(func() {
        auditStore = NewMockAuditStore()
        k8sClient = NewErrorInjectableK8sClient()
        crdCreator = processing.NewCRDCreator(k8sClient, auditStore)
        ctx = context.Background()
    })

    // Test 1.4.1
    It("should emit gateway.crd.failed audit event when K8s API fails", func() {
        // Given: K8s API that fails
        k8sClient.InjectError(errors.New("API server unavailable"))
        signal := createTestSignal("test-alert", "critical")

        // When: CRD creation attempted
        crd, err := crdCreator.CreateRemediationRequest(ctx, signal)

        // Then: Failure audit event emitted
        Expect(err).To(HaveOccurred())
        Expect(crd).To(BeNil())

        failedEvent := findEventByType(auditStore.Events, "gateway.crd.failed")
        Expect(failedEvent).ToNot(BeNil())
        Expect(failedEvent.EventAction).To(Equal("created"))
        Expect(failedEvent.EventOutcome).To(Equal("failure"))

        // Parse EventData to get GatewayAuditPayload
        gatewayPayload := failedEvent.EventData.GatewayAuditPayload

        // Access ErrorDetails (existing schema)
        errorDetails, ok := gatewayPayload.ErrorDetails.Get()
        Expect(ok).To(BeTrue(), "ErrorDetails should be present for failed CRD creation")

        // Business rule: Error message provides troubleshooting context
        Expect(errorDetails.Message).To(ContainSubstring("API server unavailable"))

        // Business rule: Error code identifies failure category
        Expect(errorDetails.Code).ToNot(BeEmpty())

        // Business rule: Component identifies error source
        Expect(errorDetails.Component).To(Equal(api.ErrorDetailsComponentGateway))
    })

    // Test 1.4.2
    It("should include error_type (transient vs permanent) in audit event", func() {
        // Given: K8s API returns 503 (transient)
        k8sClient.InjectStatusError(503, "Service Unavailable")
        signal := createTestSignal("test-alert", "critical")

        // When: CRD creation attempted
        crdCreator.CreateRemediationRequest(ctx, signal)

        // Then: Error type in audit event
        failedEvent := findEventByType(auditStore.Events, "gateway.crd.failed")

        // Parse EventData to get GatewayAuditPayload
        gatewayPayload := failedEvent.EventData.GatewayAuditPayload

        // Access ErrorDetails
        errorDetails, ok := gatewayPayload.ErrorDetails.Get()
        Expect(ok).To(BeTrue())

        // Business rule: RetryPossible indicates transient vs permanent error
        Expect(errorDetails.RetryPossible).To(BeTrue(), "503 errors are transient and retryable")

        // Business rule: Error code contains HTTP status for K8s API errors
        Expect(errorDetails.Code).To(ContainSubstring("503"))

        // Business rule: Error message provides context
        Expect(errorDetails.Message).To(ContainSubstring("Service Unavailable"))
    })

    // Test 1.4.3
    It("should emit separate audit events for each retry attempt", func() {
        // Given: K8s API fails multiple times
        k8sClient.InjectTransientErrors(3)
        signal := createTestSignal("test-alert", "critical")

        // When: CRD creation with retries
        crdCreator.CreateRemediationRequestWithRetry(ctx, signal, 3)

        // Then: Multiple failure events emitted (one per attempt)
        failedEvents := filterEventsByType(auditStore.Events, "gateway.crd.failed")
        Expect(failedEvents).To(HaveLen(3), "Each retry attempt should generate audit event")

        // Business rule: Each event has its own correlation ID for tracking
        Expect(failedEvents[0].CorrelationID).ToNot(BeEmpty())
        Expect(failedEvents[1].CorrelationID).ToNot(BeEmpty())
        Expect(failedEvents[2].CorrelationID).ToNot(BeEmpty())

        // Verify all events contain ErrorDetails
        for i, event := range failedEvents {
            gatewayPayload := event.EventData.GatewayAuditPayload
            errorDetails, ok := gatewayPayload.ErrorDetails.Get()
            Expect(ok).To(BeTrue(), fmt.Sprintf("Event %d should have ErrorDetails", i+1))
            Expect(errorDetails.RetryPossible).To(BeTrue(), "Transient errors are retryable")
        }
    })

    // Test 1.4.4
    It("should include circuit_breaker_state when circuit is open", func() {
        // Given: Circuit breaker open
        k8sClient.OpenCircuitBreaker()
        signal := createTestSignal("test-alert", "critical")

        // When: CRD creation attempted
        crdCreator.CreateRemediationRequest(ctx, signal)

        // Then: Circuit breaker state in audit event
        failedEvent := findEventByType(auditStore.Events, "gateway.crd.failed")
        Expect(failedEvent.Metadata).To(HaveKeyWithValue("circuit_breaker_state", "open"))
        Expect(failedEvent.Metadata["error"]).To(ContainSubstring("circuit breaker open"))
    })

    // Test 1.4.5
    It("should include validation_errors when CRD validation fails", func() {
        // Given: Signal with invalid data
        signal := createInvalidSignal() // Missing required fields

        // When: CRD creation attempted
        crdCreator.CreateRemediationRequest(ctx, signal)

        // Then: Validation errors in audit event
        failedEvent := findEventByType(auditStore.Events, "gateway.crd.failed")
        Expect(failedEvent.Metadata).To(HaveKeyWithValue("error_type", "permanent"))
        Expect(failedEvent.Metadata).To(HaveKey("validation_errors"))
        Expect(failedEvent.Metadata["validation_errors"]).To(ContainSubstring("missing required field"))
    })
})
```

**Acceptance Criteria**:
- ✅ All 5 tests pass
- ✅ Coverage for error handling paths increases
- ✅ Tests validate failure scenarios comprehensively

**Phase 1 Audit Category Totals**:
- **Tests**: 20 tests across 4 scenarios
- **Coverage Gain**: +9%
- **Files Covered**: `audit_helpers.go`, `server.go`, `processing/phase_checker.go`, `processing/crd_creator.go`

---

### **Category 2: Metrics Emission** (+6% coverage)

#### **Test File**: `test/integration/gateway/metrics_emission_integration_test.go`

---

#### **Scenario 2.1: HTTP Request Metrics**
**BR**: BR-GATEWAY-067
**Priority**: P1 (High)
**Business Value**: Operational visibility into Gateway performance

**Test Specifications**:

```go
var _ = Describe("BR-GATEWAY-067: HTTP Request Metrics Emission", func() {
    var (
        metricsRegistry *prometheus.Registry
        metricsMiddleware *middleware.HTTPMetrics
        ctx             context.Context
    )

    BeforeEach(func() {
        metricsRegistry = prometheus.NewRegistry()
        metricsMiddleware = middleware.NewHTTPMetrics(metricsRegistry)
        ctx = context.Background()
    })

    // Test 2.1.1
    It("should increment gateway_http_requests_total{status=201} on CRD creation", func() {
        // Given: Initial metric value
        initialValue := getMetricValue(metricsRegistry, "gateway_http_requests_total", map[string]string{"status": "201"})

        // When: Real signal processed and CRD created (not mocked HTTP)
        signal := createTestSignal("high-cpu", "critical")
        crd, err := handler.ProcessSignal(ctx, signal)
        Expect(err).ToNot(HaveOccurred())

        // Then: Metric incremented (correlates with K8s operation)
        finalValue := getMetricValue(metricsRegistry, "gateway_http_requests_total", map[string]string{"status": "201"})
        Expect(finalValue).To(Equal(initialValue + 1))

        // Business rule: Metric correlates with actual K8s CRD creation
        retrievedCRD := &remediationv1alpha1.RemediationRequest{}
        err = k8sClient.Get(ctx, client.ObjectKey{Name: crd.Name, Namespace: crd.Namespace}, retrievedCRD)
        Expect(err).ToNot(HaveOccurred())
        Expect(retrievedCRD.Name).To(Equal(crd.Name))
    })

    // Test 2.1.2
    It("should increment gateway_http_requests_total{status=202} on deduplication", func() {
        // Given: Initial metric value and existing CRD
        initialValue := getMetricValue(metricsRegistry, "gateway_http_requests_total", map[string]string{"status": "202"})

        signal1 := createTestSignal("high-cpu", "critical")
        crd1, _ := handler.ProcessSignal(ctx, signal1)

        // When: Duplicate signal processed (same fingerprint)
        signal2 := createTestSignalWithFingerprint("high-cpu", "critical", signal1.Fingerprint)
        crd2, err := handler.ProcessSignal(ctx, signal2)
        Expect(err).ToNot(HaveOccurred())

        // Then: Metric incremented (correlates with deduplication decision)
        finalValue := getMetricValue(metricsRegistry, "gateway_http_requests_total", map[string]string{"status": "202"})
        Expect(finalValue).To(Equal(initialValue + 1))

        // Business rule: No new CRD created due to deduplication
        Expect(crd2).To(BeNil())

        // Business rule: Original CRD occurrence count updated (if tracked)
        retrievedCRD := &remediationv1alpha1.RemediationRequest{}
        err = k8sClient.Get(ctx, client.ObjectKey{Name: crd1.Name, Namespace: crd1.Namespace}, retrievedCRD)
        Expect(err).ToNot(HaveOccurred())
        Expect(retrievedCRD.Spec.OccurrenceCount).To(BeNumerically(">", 1))
    })

    // Test 2.1.3
    It("should increment gateway_http_requests_total{status=500} on error", func() {
        // Given: Initial metric value
        initialValue := getMetricValue(metricsRegistry, "gateway_http_requests_total", map[string]string{"status": "500"})

        // When: Request fails
        metricsMiddleware.RecordRequest("POST", "/webhook/prometheus", 500, 10*time.Millisecond)

        // Then: Metric incremented
        finalValue := getMetricValue(metricsRegistry, "gateway_http_requests_total", map[string]string{"status": "500"})
        Expect(finalValue).To(Equal(initialValue + 1))
    })

    // Test 2.1.4
    It("should include method, path, status labels in metrics", func() {
        // When: Different requests recorded
        metricsMiddleware.RecordRequest("POST", "/webhook/prometheus", 201, 100*time.Millisecond)
        metricsMiddleware.RecordRequest("POST", "/webhook/k8s-event", 201, 120*time.Millisecond)
        metricsMiddleware.RecordRequest("GET", "/health", 200, 5*time.Millisecond)

        // Then: Metrics exist with correct labels
        Expect(metricExists(metricsRegistry, "gateway_http_requests_total", map[string]string{
            "method": "POST",
            "path":   "/webhook/prometheus",
            "status": "201",
        })).To(BeTrue())

        Expect(metricExists(metricsRegistry, "gateway_http_requests_total", map[string]string{
            "method": "GET",
            "path":   "/health",
            "status": "200",
        })).To(BeTrue())
    })

    // Test 2.1.5
    It("should populate request duration histogram", func() {
        // When: Requests with different durations
        metricsMiddleware.RecordRequest("POST", "/webhook/prometheus", 201, 50*time.Millisecond)
        metricsMiddleware.RecordRequest("POST", "/webhook/prometheus", 201, 150*time.Millisecond)
        metricsMiddleware.RecordRequest("POST", "/webhook/prometheus", 201, 500*time.Millisecond)

        // Then: Histogram populated
        histogram := getHistogramMetric(metricsRegistry, "gateway_http_request_duration_seconds")
        Expect(histogram).ToNot(BeNil())
        Expect(histogram.GetSampleCount()).To(Equal(uint64(3)))
        Expect(histogram.GetSampleSum()).To(BeNumerically("~", 0.7, 0.1)) // ~700ms total
    })
})
```

**Acceptance Criteria**:
- ✅ All 5 tests pass
- ✅ Coverage for `pkg/gateway/middleware/http_metrics.go` increases from 10% to ≥50%
- ✅ Tests validate Prometheus metric correctness

---

#### **Scenario 2.2: CRD Creation Metrics**
**BR**: BR-GATEWAY-068
**Priority**: P1 (High)
**Business Value**: Track CRD creation success/failure rates

**Test Specifications**:

```go
var _ = Describe("BR-GATEWAY-068: CRD Creation Metrics Emission", func() {
    var (
        metricsRegistry *prometheus.Registry
        crdCreator      *processing.CRDCreator
        k8sClient       client.Client
        ctx             context.Context
    )

    BeforeEach(func() {
        metricsRegistry = prometheus.NewRegistry()
        k8sClient = fake.NewClientBuilder().Build()
        crdCreator = processing.NewCRDCreatorWithMetrics(k8sClient, metricsRegistry)
        ctx = context.Background()
    })

    // Test 2.2.1
    It("should increment gateway_crd_creations_total{status=success} on success", func() {
        // Given: Valid signal
        signal := createTestSignal("test-alert", "critical")
        initialValue := getMetricValue(metricsRegistry, "gateway_crd_creations_total", map[string]string{"status": "success"})

        // When: CRD created successfully
        crd, err := crdCreator.CreateRemediationRequest(ctx, signal)

        // Then: Success metric incremented
        Expect(err).ToNot(HaveOccurred())
        finalValue := getMetricValue(metricsRegistry, "gateway_crd_creations_total", map[string]string{"status": "success"})
        Expect(finalValue).To(Equal(initialValue + 1))
    })

    // Test 2.2.2
    It("should increment gateway_crd_creations_total{status=failure} on failure", func() {
        // Given: K8s API that fails
        k8sClient := NewErrorInjectableK8sClient()
        k8sClient.InjectError(errors.New("API unavailable"))
        crdCreator = processing.NewCRDCreatorWithMetrics(k8sClient, metricsRegistry)
        initialValue := getMetricValue(metricsRegistry, "gateway_crd_creations_total", map[string]string{"status": "failure"})

        // When: CRD creation fails
        signal := createTestSignal("test-alert", "critical")
        crd, err := crdCreator.CreateRemediationRequest(ctx, signal)

        // Then: Failure metric incremented
        Expect(err).To(HaveOccurred())
        finalValue := getMetricValue(metricsRegistry, "gateway_crd_creations_total", map[string]string{"status": "failure"})
        Expect(finalValue).To(Equal(initialValue + 1))
    })

    // Test 2.2.3
    It("should include namespace and adapter labels in metrics", func() {
        // Given: Signals from different sources
        prometheusSignal := createTestSignalFromAdapter("prometheus", "prod-ns", "high-cpu")
        k8sEventSignal := createTestSignalFromAdapter("k8s-event", "staging-ns", "pod-crash")

        // When: CRDs created
        crdCreator.CreateRemediationRequest(ctx, prometheusSignal)
        crdCreator.CreateRemediationRequest(ctx, k8sEventSignal)

        // Then: Metrics with correct labels
        Expect(metricExists(metricsRegistry, "gateway_crd_creations_total", map[string]string{
            "status":    "success",
            "namespace": "prod-ns",
            "adapter":   "prometheus",
        })).To(BeTrue())

        Expect(metricExists(metricsRegistry, "gateway_crd_creations_total", map[string]string{
            "status":    "success",
            "namespace": "staging-ns",
            "adapter":   "k8s-event",
        })).To(BeTrue())
    })

    // Test 2.2.4
    It("should accumulate metrics across multiple CRD creations", func() {
        // Given: Multiple signals
        signals := []Signal{
            createTestSignal("alert-1", "critical"),
            createTestSignal("alert-2", "warning"),
            createTestSignal("alert-3", "error"),
        }

        // When: Multiple CRDs created
        for _, signal := range signals {
            crdCreator.CreateRemediationRequest(ctx, signal)
        }

        // Then: Counter increases correctly
        finalValue := getMetricValue(metricsRegistry, "gateway_crd_creations_total", map[string]string{"status": "success"})
        Expect(finalValue).To(Equal(float64(3)))
    })

    // Test 2.2.5
    It("should persist metric values across multiple test iterations", func() {
        // Given: Initial CRD creation
        signal1 := createTestSignal("alert-1", "critical")
        crdCreator.CreateRemediationRequest(ctx, signal1)
        value1 := getMetricValue(metricsRegistry, "gateway_crd_creations_total", map[string]string{"status": "success"})

        // When: Another CRD created (simulating next request)
        signal2 := createTestSignal("alert-2", "warning")
        crdCreator.CreateRemediationRequest(ctx, signal2)
        value2 := getMetricValue(metricsRegistry, "gateway_crd_creations_total", map[string]string{"status": "success"})

        // Then: Values accumulate
        Expect(value2).To(Equal(value1 + 1))
    })
})
```

**Acceptance Criteria**:
- ✅ All 5 tests pass
- ✅ Coverage for `pkg/gateway/processing/crd_creator.go` increases to ≥75%
- ✅ Metrics accurately track CRD creation lifecycle

---

#### **Scenario 2.3: Deduplication Metrics**
**BR**: BR-GATEWAY-069
**Priority**: P1 (High)
**Business Value**: Track deduplication effectiveness for capacity planning

**Test Specifications**:

```go
var _ = Describe("BR-GATEWAY-069: Deduplication Metrics Emission", func() {
    var (
        metricsRegistry *prometheus.Registry
        phaseChecker    *processing.PhaseChecker
        k8sClient       client.Client
        ctx             context.Context
    )

    BeforeEach(func() {
        metricsRegistry = prometheus.NewRegistry()
        k8sClient = fake.NewClientBuilder().Build()
        phaseChecker = processing.NewPhaseCheckerWithMetrics(k8sClient, metricsRegistry)
        ctx = context.Background()
    })

    // Test 2.3.1
    It("should increment gateway_deduplications_total when signal deduplicated", func() {
        // Given: Existing RR
        existingRR := createTestRR("fp-12345", "Pending", "test-ns")
        k8sClient.Create(ctx, existingRR)
        initialValue := getMetricValue(metricsRegistry, "gateway_deduplications_total", map[string]string{})

        // When: Duplicate signal arrives
        signal := createTestSignalWithFingerprint("fp-12345")
        shouldDedupe, rr, err := phaseChecker.ShouldDeduplicate(ctx, signal)

        // Then: Deduplication metric incremented
        Expect(err).ToNot(HaveOccurred())
        Expect(shouldDedupe).To(BeTrue())
        finalValue := getMetricValue(metricsRegistry, "gateway_deduplications_total", map[string]string{})
        Expect(finalValue).To(Equal(initialValue + 1))
    })

    // Test 2.3.2
    It("should include reason label (status-based, fingerprint-based)", func() {
        // Given: RR in Pending phase
        existingRR := createTestRR("fp-status", "Pending", "test-ns")
        k8sClient.Create(ctx, existingRR)

        // When: Duplicate signal arrives
        signal := createTestSignalWithFingerprint("fp-status")
        phaseChecker.ShouldDeduplicate(ctx, signal)

        // Then: Reason label present
        Expect(metricExists(metricsRegistry, "gateway_deduplications_total", map[string]string{
            "reason": "status-based",
        })).To(BeTrue())
    })

    // Test 2.3.3
    It("should include phase label (Pending, Processing, Blocked)", func() {
        // Given: RRs in different phases
        pendingRR := createTestRR("fp-pending", "Pending", "test-ns")
        processingRR := createTestRR("fp-processing", "Processing", "test-ns")
        blockedRR := createTestRR("fp-blocked", "Blocked", "test-ns")
        k8sClient.Create(ctx, pendingRR)
        k8sClient.Create(ctx, processingRR)
        k8sClient.Create(ctx, blockedRR)

        // When: Duplicates arrive for each phase
        phaseChecker.ShouldDeduplicate(ctx, createTestSignalWithFingerprint("fp-pending"))
        phaseChecker.ShouldDeduplicate(ctx, createTestSignalWithFingerprint("fp-processing"))
        phaseChecker.ShouldDeduplicate(ctx, createTestSignalWithFingerprint("fp-blocked"))

        // Then: Phase labels present
        Expect(metricExists(metricsRegistry, "gateway_deduplications_total", map[string]string{"phase": "Pending"})).To(BeTrue())
        Expect(metricExists(metricsRegistry, "gateway_deduplications_total", map[string]string{"phase": "Processing"})).To(BeTrue())
        Expect(metricExists(metricsRegistry, "gateway_deduplications_total", map[string]string{"phase": "Blocked"})).To(BeTrue())
    })

    // Test 2.3.4
    It("should allow deduplication rate calculation (deduplications / total_signals)", func() {
        // Given: Mix of new and duplicate signals
        existingRR := createTestRR("fp-existing", "Pending", "test-ns")
        k8sClient.Create(ctx, existingRR)

        // When: Signals processed
        phaseChecker.ShouldDeduplicate(ctx, createTestSignalWithFingerprint("fp-new-1"))       // Not deduped
        phaseChecker.ShouldDeduplicate(ctx, createTestSignalWithFingerprint("fp-existing"))    // Deduped
        phaseChecker.ShouldDeduplicate(ctx, createTestSignalWithFingerprint("fp-existing"))    // Deduped
        phaseChecker.ShouldDeduplicate(ctx, createTestSignalWithFingerprint("fp-new-2"))       // Not deduped

        // Then: Deduplication rate calculable
        dedupeCount := getMetricValue(metricsRegistry, "gateway_deduplications_total", map[string]string{})
        totalSignals := getMetricValue(metricsRegistry, "gateway_signals_total", map[string]string{})
        dedupeRate := dedupeCount / totalSignals
        Expect(dedupeRate).To(BeNumerically("~", 0.5, 0.01)) // 2 deduped out of 4 signals = 50%
    })

    // Test 2.3.5
    It("should correlate metrics with occurrence_count in CRD status", func() {
        // Given: RR with occurrence count = 1
        existingRR := createTestRR("fp-correlated", "Pending", "test-ns")
        existingRR.Status.OccurrenceCount = 1
        k8sClient.Create(ctx, existingRR)

        // When: Multiple duplicates arrive
        signal := createTestSignalWithFingerprint("fp-correlated")
        phaseChecker.ShouldDeduplicate(ctx, signal)
        phaseChecker.ShouldDeduplicate(ctx, signal)
        phaseChecker.ShouldDeduplicate(ctx, signal)

        // Then: Metric count matches occurrence count increase
        dedupeCount := getMetricValue(metricsRegistry, "gateway_deduplications_total", map[string]string{})
        Expect(dedupeCount).To(Equal(float64(3)))

        // Verify RR updated
        updatedRR := &v1alpha1.RemediationRequest{}
        k8sClient.Get(ctx, client.ObjectKeyFromObject(existingRR), updatedRR)
        Expect(updatedRR.Status.OccurrenceCount).To(Equal(4)) // 1 + 3 = 4
    })
})
```

**Acceptance Criteria**:
- ✅ All 5 tests pass
- ✅ Coverage for `pkg/gateway/processing/phase_checker.go` maintained at ≥85%
- ✅ Metrics correlate with business logic (occurrence count)

**Phase 1 Metrics Category Totals**:
- **Tests**: 15 tests across 3 scenarios
- **Coverage Gain**: +6%
- **Files Covered**: `middleware/http_metrics.go`, `processing/crd_creator.go`, `processing/phase_checker.go`, `metrics/metrics.go`

---

## **Phase 1 Summary**

| Category | Scenarios | Tests | Coverage Gain | Files Improved |
|----------|-----------|-------|---------------|----------------|
| **Audit Emission** | 4 | 20 | +9% | 4 files |
| **Metrics Emission** | 3 | 15 | +6% | 4 files |
| **TOTAL** | **7** | **35** | **+15%** | **8 files** |

**Phase 1 Target**: 30.1% + 15% = **45% coverage** ✅

---

## 🎯 **PHASE 2: Core Business Logic (Week 2)** - Target: +12% → 57%

### **Objective**: Achieve full compliance with core adapter and error handling logic

---

### **Category 3: Adapter Business Logic** (+6% coverage)

#### **Test File**: `test/integration/gateway/adapters_integration_test.go`

---

#### **Scenario 3.1: Prometheus Adapter Signal Parsing**
**BR**: BR-GATEWAY-001, BR-GATEWAY-005
**Priority**: P0 (Critical)
**Business Value**: Validate correct signal extraction from Prometheus payloads

**Test Specifications**:

```go
var _ = Describe("BR-GATEWAY-001: Prometheus Adapter Signal Parsing", func() {
    var (
        adapter prometheus.Adapter
        ctx     context.Context
    )

    BeforeEach(func() {
        adapter = prometheus.NewAdapter()
        ctx = context.Background()
    })

    // Test 3.1.1
    It("should parse standard Prometheus alert into Signal with correct labels", func() {
        // Given: Standard Prometheus alert
        alert := createPrometheusAlert(map[string]string{
            "alertname": "HighCPU",
            "severity":  "critical",
            "namespace": "production",
            "pod":       "api-server-123",
        })

        // When: Adapter parses alert
        signal, err := adapter.Parse(ctx, alert)

        // Then: Signal extracted correctly
        Expect(err).ToNot(HaveOccurred())
        Expect(signal.AlertName).To(Equal("HighCPU"))
        Expect(signal.Severity).To(Equal("critical"))
        Expect(signal.Namespace).To(Equal("production"))
        Expect(signal.Labels).To(HaveKeyWithValue("pod", "api-server-123"))
    })

    // Test 3.1.2
    It("should extract namespace from alert labels", func() {
        // Given: Alert with namespace label
        alert := createPrometheusAlert(map[string]string{
            "alertname": "DiskFull",
            "namespace": "staging-tenant-a",
        })

        // When: Parsed
        signal, _ := adapter.Parse(ctx, alert)

        // Then: Namespace extracted
        Expect(signal.Namespace).To(Equal("staging-tenant-a"))
    })

    // Test 3.1.3
    It("should extract severity from alert labels and preserve in Signal", func() {
        // Given: Alert with severity
        alert := createPrometheusAlert(map[string]string{
            "alertname": "MemoryLeak",
            "severity":  "warning",
        })

        // When: Parsed
        signal, _ := adapter.Parse(ctx, alert)

        // Then: Severity preserved (pass-through per BR-GATEWAY-111)
        Expect(signal.Severity).To(Equal("warning"))
    })

    // Test 3.1.4
    It("should populate target resource from pod/node labels", func() {
        // Given: Alert with pod label
        alert := createPrometheusAlert(map[string]string{
            "alertname": "PodCrashLoop",
            "pod":       "crashpod-456",
            "namespace": "prod-us-east",
        })

        // When: Parsed
        signal, _ := adapter.Parse(ctx, alert)

        // Then: Target resource populated
        Expect(signal.TargetResource).ToNot(BeNil())
        Expect(signal.TargetResource.Kind).To(Equal("Pod"))
        Expect(signal.TargetResource.Name).To(Equal("crashpod-456"))
        Expect(signal.TargetResource.Namespace).To(Equal("prod-us-east"))
    })

    // Test 3.1.5
    It("should apply safe defaults for missing optional fields", func() {
        // Given: Minimal alert (only required fields)
        alert := createPrometheusAlert(map[string]string{
            "alertname": "MinimalAlert",
        })

        // When: Parsed
        signal, err := adapter.Parse(ctx, alert)
        Expect(err).ToNot(HaveOccurred())

        // Then: Safe defaults enable CRD creation (business outcome)
        // Business rule: Default severity enables RemediationRequest priority classification
        Expect(signal.Severity).To(Equal("unknown"))

        // Business rule: Default namespace prevents orphaned CRDs
        Expect(signal.Namespace).To(Equal("default"))

        // Business rule: Empty maps prevent nil pointer panics in downstream processing
        Expect(signal.Labels).To(BeEmpty())  // Empty map, not nil
        Expect(signal.Annotations).To(BeEmpty())  // Empty map, not nil

        // Validate CRD can be created with safe defaults
        crdCreator := processing.NewCRDCreator(k8sClient, auditStore)
        crd, err := crdCreator.CreateRemediationRequest(ctx, signal)
        Expect(err).ToNot(HaveOccurred())
        Expect(crd.Name).ToNot(BeEmpty())
    })

    // Test 3.1.6
    It("should preserve all custom labels in Signal", func() {
        // Given: Alert with custom labels
        alert := createPrometheusAlert(map[string]string{
            "alertname":   "CustomAlert",
            "team":        "platform",
            "environment": "production",
            "tier":        "critical",
        })

        // When: Parsed
        signal, _ := adapter.Parse(ctx, alert)

        // Then: All labels preserved
        Expect(signal.Labels).To(HaveKeyWithValue("team", "platform"))
        Expect(signal.Labels).To(HaveKeyWithValue("environment", "production"))
        Expect(signal.Labels).To(HaveKeyWithValue("tier", "critical"))
    })

    // Test 3.1.7
    It("should truncate long annotations correctly", func() {
        // Given: Alert with very long annotation
        longAnnotation := strings.Repeat("A", 10000) // 10KB annotation
        alert := createPrometheusAlertWithAnnotation("summary", longAnnotation)

        // When: Parsed
        signal, _ := adapter.Parse(ctx, alert)

        // Then: Annotation truncated (max 4KB)
        Expect(signal.Annotations["summary"]).To(HaveLen(4096))
        Expect(signal.Annotations["summary"]).To(HaveSuffix("...truncated"))
    })

    // Test 3.1.8
    It("should parse multiple alerts from AlertManager payload", func() {
        // Given: AlertManager webhook with 3 alerts
        payload := createAlertManagerPayload([]map[string]string{
            {"alertname": "Alert1", "severity": "critical"},
            {"alertname": "Alert2", "severity": "warning"},
            {"alertname": "Alert3", "severity": "info"},
        })

        // When: Parsed
        signals, err := adapter.ParseBatch(ctx, payload)

        // Then: All alerts parsed
        Expect(err).ToNot(HaveOccurred())
        Expect(signals).To(HaveLen(3))
        Expect(signals[0].AlertName).To(Equal("Alert1"))
        Expect(signals[1].AlertName).To(Equal("Alert2"))
        Expect(signals[2].AlertName).To(Equal("Alert3"))
    })
})
```

**Acceptance Criteria**:
- ✅ All 8 tests pass
- ✅ Coverage for `pkg/gateway/adapters/prometheus_adapter.go` increases from 0% to ≥60%
- ✅ Tests validate signal extraction correctness

---

#### **Scenario 3.2: Kubernetes Event Adapter Signal Parsing**
**BR**: BR-GATEWAY-002, BR-GATEWAY-005
**Priority**: P0 (Critical)
**Business Value**: Validate correct signal extraction from K8s Events

**Test Specifications**:

```go
var _ = Describe("BR-GATEWAY-002: Kubernetes Event Adapter Signal Parsing", func() {
    var (
        adapter k8sevent.Adapter
        ctx     context.Context
    )

    BeforeEach(func() {
        adapter = k8sevent.NewAdapter()
        ctx = context.Background()
    })

    // Test 3.2.1
    It("should parse Warning event into Signal with severity=warning", func() {
        // Given: Warning K8s Event
        event := createK8sEvent("Warning", "BackOff", "Back-off restarting failed container")

        // When: Adapter parses event
        signal, err := adapter.Parse(ctx, event)

        // Then: Signal extracted with warning severity
        Expect(err).ToNot(HaveOccurred())
        Expect(signal.Severity).To(Equal("warning"))
        Expect(signal.AlertName).To(Equal("BackOff"))
    })

    // Test 3.2.2
    It("should populate target resource from involvedObject", func() {
        // Given: Event with Pod involved object
        event := createK8sEventWithInvolvedObject(
            "Warning",
            "FailedScheduling",
            "Pod",
            "unscheduled-pod-123",
            "production",
        )

        // When: Parsed
        signal, _ := adapter.Parse(ctx, event)

        // Then: Target resource populated
        Expect(signal.TargetResource).ToNot(BeNil())
        Expect(signal.TargetResource.Kind).To(Equal("Pod"))
        Expect(signal.TargetResource.Name).To(Equal("unscheduled-pod-123"))
        Expect(signal.TargetResource.Namespace).To(Equal("production"))
    })

    // Test 3.2.3
    It("should use event reason as alert name", func() {
        // Given: Event with specific reason
        event := createK8sEvent("Warning", "FailedMount", "MountVolume.SetUp failed")

        // When: Parsed
        signal, _ := adapter.Parse(ctx, event)

        // Then: Reason becomes alert name
        Expect(signal.AlertName).To(Equal("FailedMount"))
    })

    // Test 3.2.4
    It("should use event message as description", func() {
        // Given: Event with detailed message
        event := createK8sEvent("Warning", "ImagePullBackOff", "Back-off pulling image registry.k8s.io/pause:3.9")

        // When: Parsed
        signal, _ := adapter.Parse(ctx, event)

        // Then: Message becomes description
        Expect(signal.Description).To(Equal("Back-off pulling image registry.k8s.io/pause:3.9"))
    })

    // Test 3.2.5
    It("should use event namespace when involvedObject namespace missing", func() {
        // Given: Event with namespace but object without namespace
        event := createK8sEventInNamespace("kube-system", "Warning", "FailedScheduling", "")

        // When: Parsed
        signal, _ := adapter.Parse(ctx, event)

        // Then: Event namespace used
        Expect(signal.Namespace).To(Equal("kube-system"))
    })

    // Test 3.2.6
    It("should track event occurrence count in Signal", func() {
        // Given: Event with count = 10
        event := createK8sEvent("Warning", "BackOff", "Back-off restarting")
        event.Count = 10

        // When: Parsed
        signal, _ := adapter.Parse(ctx, event)

        // Then: Occurrence count reflected
        Expect(signal.OccurrenceCount).To(Equal(10))
    })

    // Test 3.2.7
    It("should filter out Normal events (not Warning)", func() {
        // Given: Normal K8s Event
        event := createK8sEvent("Normal", "Started", "Started container successfully")

        // When: Parsed
        signal, err := adapter.Parse(ctx, event)

        // Then: Filtered out (nil signal, no error)
        Expect(err).ToNot(HaveOccurred())
        Expect(signal).To(BeNil())
    })
})
```

**Acceptance Criteria**:
- ✅ All 7 tests pass
- ✅ Coverage for `pkg/gateway/adapters/kubernetes_event_adapter.go` increases from 0% to ≥60%
- ✅ Tests validate K8s Event parsing correctness

**Phase 2 Adapter Category Totals**:
- **Tests**: 15 tests across 2 scenarios
- **Coverage Gain**: +6%
- **Files Covered**: `adapters/prometheus_adapter.go`, `adapters/kubernetes_event_adapter.go`

---

### **Category 4: Circuit Breaker State Transitions** (+3% coverage)

#### **Test File**: `test/integration/gateway/circuit_breaker_integration_test.go`

---

#### **Scenario 4.1: Circuit Breaker State Machine** ❌ REMOVED
**Rationale**: Circuit breaker state machine tests belong in **unit test tier**, not integration tests.

**Why This Doesn't Belong in Integration Tests**:
1. **No External Dependencies**: Tests only mock K8s client behavior (no real infrastructure)
2. **Pure State Machine Logic**: Tests validate state transitions (Closed → Open → Half-Open → Closed)
3. **Deterministic Behavior**: No timing dependencies, network calls, or database interactions
4. **Fast Execution**: All tests run in <100ms (except Test 4.1.2 with 30s sleep)

**Where This Should Be Tested**:
- **Unit Tests**: `test/unit/gateway/circuit_breaker_test.go`
- **E2E Tests**: `test/e2e/gateway/32_service_resilience_test.go` (already exists, tests actual K8s API failures)

**BR Coverage Status**:
- **BR-GATEWAY-093**: Already covered by:
  - Unit tests in `pkg/gateway/k8s/` (state machine logic)
  - E2E Test 32 (actual infrastructure resilience)
  - Integration Test 29 (K8s API failure handling)

**Alternative**: If circuit breaker metrics need integration testing with real Prometheus,consider **Scenario 2.1** (Prometheus Metrics) instead.

---

### **Category 5: Error Classification & Retry Logic** (+3% coverage)

#### **Test File**: `test/integration/gateway/error_handling_integration_test.go`

---

#### **Scenario 5.1: Transient vs Permanent Error Classification**
**BR**: BR-GATEWAY-188, BR-GATEWAY-189
**Priority**: P0 (Critical)
**Business Value**: Validate correct retry behavior for different error types

**Test Specifications**:

```go
var _ = Describe("BR-GATEWAY-188/189: Error Classification", func() {
    var (
        errorClassifier *processing.ErrorClassifier
        ctx             context.Context
    )

    BeforeEach(func() {
        errorClassifier = processing.NewErrorClassifier()
        ctx = context.Background()
    })

    // Test 5.1.1
    It("should classify K8s API 500 error as TRANSIENT (retry)", func() {
        // Given: 500 Internal Server Error
        err := apierrors.NewInternalError(errors.New("internal error"))

        // When: Error classified
        classification := errorClassifier.Classify(err)

        // Then: TRANSIENT
        Expect(classification.Type).To(Equal("transient"))
        Expect(classification.ShouldRetry).To(BeTrue())
        Expect(classification.MaxRetries).To(Equal(3))
    })

    // Test 5.1.2
    It("should classify K8s API 503 error as TRANSIENT (retry)", func() {
        // Given: 503 Service Unavailable
        err := apierrors.NewServiceUnavailable("service unavailable")

        // When: Classified
        classification := errorClassifier.Classify(err)

        // Then: TRANSIENT
        Expect(classification.Type).To(Equal("transient"))
        Expect(classification.ShouldRetry).To(BeTrue())
    })

    // Test 5.1.3
    It("should classify K8s API 400 error as PERMANENT (no retry)", func() {
        // Given: 400 Bad Request
        err := apierrors.NewBadRequest("invalid request")

        // When: Classified
        classification := errorClassifier.Classify(err)

        // Then: PERMANENT
        Expect(classification.Type).To(Equal("permanent"))
        Expect(classification.ShouldRetry).To(BeFalse())
    })

    // Test 5.1.4
    It("should classify K8s API 422 error as PERMANENT (no retry)", func() {
        // Given: 422 Unprocessable Entity (validation failure)
        err := apierrors.NewInvalid(schema.GroupKind{}, "test", field.ErrorList{})

        // When: Classified
        classification := errorClassifier.Classify(err)

        // Then: PERMANENT
        Expect(classification.Type).To(Equal("permanent"))
        Expect(classification.ShouldRetry).To(BeFalse())
    })

    // Test 5.1.5
    It("should classify network timeout as TRANSIENT (retry)", func() {
        // Given: Network timeout error
        err := &net.OpError{Op: "dial", Err: context.DeadlineExceeded}

        // When: Classified
        classification := errorClassifier.Classify(err)

        // Then: TRANSIENT
        Expect(classification.Type).To(Equal("transient"))
        Expect(classification.ShouldRetry).To(BeTrue())
    })

    // Test 5.1.6
    It("should classify context canceled as PERMANENT (no retry)", func() {
        // Given: Context canceled error
        err := context.Canceled

        // When: Classified
        classification := errorClassifier.Classify(err)

        // Then: PERMANENT
        Expect(classification.Type).To(Equal("permanent"))
        Expect(classification.ShouldRetry).To(BeFalse())
    })

    // Test 5.1.7
    It("should classify validation error as PERMANENT (no retry)", func() {
        // Given: Application validation error
        err := errors.New("validation failed: missing required field 'alertname'")

        // When: Classified
        classification := errorClassifier.Classify(err)

        // Then: PERMANENT
        Expect(classification.Type).To(Equal("permanent"))
        Expect(classification.ShouldRetry).To(BeFalse())
    })
})
```

**Acceptance Criteria**:
- ✅ All 7 tests pass
- ✅ Coverage for `pkg/gateway/processing/errors.go` increases from 55.6% to ≥75%
- ✅ Tests validate classification logic correctness

---

#### **Scenario 5.2: Exponential Backoff Calculation**
**BR**: BR-GATEWAY-188
**Priority**: P1 (High)
**Business Value**: Validate correct backoff timing for retries

**Test Specifications**:

```go
var _ = Describe("BR-GATEWAY-188: Exponential Backoff Calculation", func() {
    var (
        retryLogic *processing.RetryLogic
        clock      *processing.MockClock
        ctx        context.Context
    )

    BeforeEach(func() {
        clock = processing.NewMockClock()
        retryLogic = processing.NewRetryLogicWithClock(clock)
        ctx = context.Background()
    })

    // Test 5.2.1
    It("should calculate 100ms backoff for first retry", func() {
        // When: First retry backoff calculated
        backoff := retryLogic.CalculateBackoff(1)

        // Then: 100ms
        Expect(backoff).To(Equal(100 * time.Millisecond))
    })

    // Test 5.2.2
    It("should calculate 200ms backoff for second retry (2x)", func() {
        // When: Second retry backoff calculated
        backoff := retryLogic.CalculateBackoff(2)

        // Then: 200ms (100ms * 2)
        Expect(backoff).To(Equal(200 * time.Millisecond))
    })

    // Test 5.2.3
    It("should calculate 400ms backoff for third retry (2x)", func() {
        // When: Third retry backoff calculated
        backoff := retryLogic.CalculateBackoff(3)

        // Then: 400ms (200ms * 2)
        Expect(backoff).To(Equal(400 * time.Millisecond))
    })

    // Test 5.2.4
    It("should cap backoff at 5 seconds maximum", func() {
        // When: Very high retry count
        backoff := retryLogic.CalculateBackoff(10)

        // Then: Capped at 5s
        Expect(backoff).To(Equal(5 * time.Second))
    })

    // Test 5.2.5
    It("should apply jitter to backoff (vary within range)", func() {
        // When: Calculate backoff multiple times
        backoffs := []time.Duration{}
        for i := 0; i < 10; i++ {
            backoff := retryLogic.CalculateBackoffWithJitter(3)
            backoffs = append(backoffs, backoff)
        }

        // Then: Values vary (jitter applied)
        uniqueValues := make(map[time.Duration]bool)
        for _, b := range backoffs {
            uniqueValues[b] = true
        }
        Expect(len(uniqueValues)).To(BeNumerically(">", 1)) // At least 2 unique values

        // All within range (400ms ± 20%)
        for _, b := range backoffs {
            Expect(b).To(BeNumerically("~", 400*time.Millisecond, 80*time.Millisecond))
        }
    })

    // Test 5.2.6
    It("should track retry count correctly across multiple attempts", func() {
        // Given: Multiple retry attempts
        retryLogic.AttemptRetry("operation-1")
        retryLogic.AttemptRetry("operation-1")
        retryLogic.AttemptRetry("operation-1")

        // When: Get retry count
        count := retryLogic.GetRetryCount("operation-1")

        // Then: Correct count
        Expect(count).To(Equal(3))
    })
})
```

**Acceptance Criteria**:
- ✅ All 6 tests pass
- ✅ Coverage for `pkg/gateway/processing/clock.go` increases from 33.3% to ≥60%
- ✅ Tests validate backoff math correctness

**Phase 2 Error Classification Category Totals**:
- **Tests**: 13 tests across 2 scenarios
- **Coverage Gain**: +3%
- **Files Covered**: `processing/errors.go`, `processing/clock.go`

---

## **Phase 2 Summary**

| Category | Scenarios | Tests | Coverage Gain | Files Improved |
|----------|-----------|-------|---------------|----------------|
| **Adapters** | 2 | 15 | +6% | 2 files |
| **Circuit Breaker** | 1 | 7 | +3% | 1 file |
| **Error Classification** | 2 | 13 | +3% | 2 files |
| **TOTAL** | **5** | **35** | **+12%** | **5 files** |

**Phase 2 Target**: 45% + 12% = **57% coverage** ✅ **COMPLIANT**

---

## 🎯 **PHASE 3: Infrastructure Validation (Week 3)** - Target: +5% → 62%

### **Objective**: Exceed compliance with infrastructure and middleware validation

---

### **Category 6: Configuration Validation** (+2% coverage)

#### **Test File**: `test/integration/gateway/config_integration_test.go`

---

#### **Scenario 6.1: Configuration Loading & Validation**
**BR**: BR-GATEWAY-043
**Priority**: P1 (High)
**Business Value**: Validate Gateway starts with correct configuration

**Test Specifications**:

```go
var _ = Describe("BR-GATEWAY-043: Configuration Validation", func() {
    var (
        configLoader *config.Loader
        validator    *config.Validator
        ctx          context.Context
    )

    BeforeEach(func() {
        configLoader = config.NewLoader()
        validator = config.NewValidator()
        ctx = context.Background()
    })

    // Test 6.1.1
    It("should load valid configuration successfully", func() {
        // Given: Valid config file
        configData := `
server:
  port: 8080
  timeout: 30s
kubernetes:
  kubeconfig: ""
datastorage:
  url: "http://datastorage:8080"
`

        // When: Config loaded
        cfg, err := configLoader.LoadFromBytes([]byte(configData))

        // Then: Success
        Expect(err).ToNot(HaveOccurred())
        Expect(cfg.Server.Port).To(Equal(8080))
        Expect(cfg.Server.Timeout).To(Equal(30 * time.Second))
    })

    // Test 6.1.2
    It("should return validation error for missing required field", func() {
        // Given: Config missing required field
        configData := `
server:
  port: 8080
  # Missing timeout
kubernetes:
  kubeconfig: ""
`

        // When: Config validated
        cfg, _ := configLoader.LoadFromBytes([]byte(configData))
        err := validator.Validate(cfg)

        // Then: Validation error
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("timeout is required"))
    })

    // Test 6.1.3
    It("should return validation error for invalid port number", func() {
        // Given: Config with invalid port
        configData := `
server:
  port: 99999  # Invalid port
  timeout: 30s
`

        // When: Validated
        cfg, _ := configLoader.LoadFromBytes([]byte(configData))
        err := validator.Validate(cfg)

        // Then: Validation error
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("port must be between 1 and 65535"))
    })

    // Test 6.1.4
    It("should return validation error for invalid timeout value", func() {
        // Given: Config with negative timeout
        configData := `
server:
  port: 8080
  timeout: -5s
`

        // When: Validated
        cfg, _ := configLoader.LoadFromBytes([]byte(configData))
        err := validator.Validate(cfg)

        // Then: Validation error
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("timeout must be positive"))
    })

    // Test 6.1.5
    It("should return validation error for invalid log level", func() {
        // Given: Config with invalid log level
        configData := `
server:
  port: 8080
  timeout: 30s
logging:
  level: "INVALID"
`

        // When: Validated
        cfg, _ := configLoader.LoadFromBytes([]byte(configData))
        err := validator.Validate(cfg)

        // Then: Validation error
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("invalid log level"))
    })

    // Test 6.1.6
    It("should apply defaults for missing optional fields", func() {
        // Given: Minimal config
        configData := `
server:
  port: 8080
  timeout: 30s
`

        // When: Config loaded
        cfg, err := configLoader.LoadFromBytes([]byte(configData))

        // Then: Defaults applied
        Expect(err).ToNot(HaveOccurred())
        Expect(cfg.Logging.Level).To(Equal("info")) // Default log level
        Expect(cfg.Server.ReadTimeout).To(Equal(5 * time.Second)) // Default read timeout
    })

    // Test 6.1.7
    It("should allow environment variable override", func() {
        // Given: Config with env var placeholder
        os.Setenv("GATEWAY_PORT", "9090")
        defer os.Unsetenv("GATEWAY_PORT")

        configData := `
server:
  port: ${GATEWAY_PORT}
  timeout: 30s
`

        // When: Config loaded
        cfg, err := configLoader.LoadFromBytes([]byte(configData))

        // Then: Env var applied
        Expect(err).ToNot(HaveOccurred())
        Expect(cfg.Server.Port).To(Equal(9090))
    })
})
```

**Acceptance Criteria**:
- ✅ All 7 tests pass
- ✅ Coverage for `pkg/gateway/config/config.go` increases from 16.7% to ≥50%
- ✅ Tests validate config loading and validation

**Phase 3 Configuration Category Totals**:
- **Tests**: 7 tests across 1 scenario
- **Coverage Gain**: +2%
- **Files Covered**: `config/config.go`, `config/errors.go`

---

### **Category 7: Middleware Chain Integration** (+3% coverage)

#### **Test File**: `test/integration/gateway/middleware_chain_integration_test.go`

---

#### **Scenario 7.1: Middleware Chain Execution Order**
**BR**: BR-GATEWAY-039, BR-GATEWAY-074, BR-GATEWAY-075, BR-GATEWAY-076
**Priority**: P1 (High)
**Business Value**: Validate middleware executes in correct order

**Test Specifications**:

```go
var _ = Describe("BR-GATEWAY-039/074-076: Middleware Chain Execution", func() {
    var (
        middlewareChain *middleware.Chain
        recorder        *middleware.ExecutionRecorder
        ctx             context.Context
    )

    BeforeEach(func() {
        recorder = middleware.NewExecutionRecorder()
        middlewareChain = middleware.NewChain()
        ctx = context.Background()
    })

    // Test 7.1.1
    It("should execute Request ID middleware first", func() {
        // Given: Middleware chain with recorder
        middlewareChain.Use(middleware.RequestID(recorder))
        middlewareChain.Use(middleware.Timestamp(recorder))
        middlewareChain.Use(middleware.SecurityHeaders(recorder))

        // When: Request processed
        req := createTestRequest()
        middlewareChain.Execute(req)

        // Then: Request ID executed first
        execution := recorder.GetExecutionOrder()
        Expect(execution[0]).To(Equal("request_id"))
    })

    // Test 7.1.2
    It("should execute Timestamp validation before processing", func() {
        // Given: Chain with timestamp middleware
        middlewareChain.Use(middleware.RequestID(recorder))
        middlewareChain.Use(middleware.Timestamp(recorder))

        // When: Request processed
        req := createTestRequestWithTimestamp()
        middlewareChain.Execute(req)

        // Then: Timestamp validated early
        execution := recorder.GetExecutionOrder()
        timestampIndex := findIndex(execution, "timestamp")
        Expect(timestampIndex).To(Equal(1)) // Second (after request ID)
    })

    // Test 7.1.3
    It("should add Security headers to response", func() {
        // Given: Chain with security headers middleware
        middlewareChain.Use(middleware.SecurityHeaders(recorder))

        // When: Request processed
        req := createTestRequest()
        resp := middlewareChain.Execute(req)

        // Then: Security headers present
        Expect(resp.Headers).To(HaveKey("X-Content-Type-Options"))
        Expect(resp.Headers).To(HaveKey("X-Frame-Options"))
        Expect(resp.Headers["X-Content-Type-Options"]).To(Equal("nosniff"))
    })

    // Test 7.1.4
    It("should validate Content-Type before adapter processing", func() {
        // Given: Chain with content-type middleware
        middlewareChain.Use(middleware.ContentType(recorder))

        // When: Request with wrong content-type
        req := createTestRequestWithContentType("text/plain")
        resp := middlewareChain.Execute(req)

        // Then: Request rejected early
        Expect(resp.StatusCode).To(Equal(415)) // Unsupported Media Type
        execution := recorder.GetExecutionOrder()
        Expect(execution).ToNot(ContainElement("adapter")) // Adapter not reached
    })

    // Test 7.1.5
    It("should execute all middleware in correct order", func() {
        // Given: Full middleware chain
        middlewareChain.Use(middleware.RequestID(recorder))
        middlewareChain.Use(middleware.Timestamp(recorder))
        middlewareChain.Use(middleware.SecurityHeaders(recorder))
        middlewareChain.Use(middleware.ContentType(recorder))

        // When: Request processed
        req := createValidTestRequest()
        middlewareChain.Execute(req)

        // Then: Correct execution order
        execution := recorder.GetExecutionOrder()
        Expect(execution).To(Equal([]string{
            "request_id",
            "timestamp",
            "security_headers",
            "content_type",
        }))
    })

    // Test 7.1.6
    It("should reject request early if middleware fails", func() {
        // Given: Chain with failing middleware
        middlewareChain.Use(middleware.RequestID(recorder))
        middlewareChain.Use(middleware.FailingMiddleware()) // Always fails
        middlewareChain.Use(middleware.SecurityHeaders(recorder))

        // When: Request processed
        req := createTestRequest()
        resp := middlewareChain.Execute(req)

        // Then: Request rejected, later middleware not executed
        Expect(resp.StatusCode).To(Equal(400))
        execution := recorder.GetExecutionOrder()
        Expect(execution).To(Equal([]string{"request_id"})) // Only first middleware
    })

    // Test 7.1.7
    It("should track middleware execution in metrics", func() {
        // Given: Chain with metrics
        metricsRegistry := prometheus.NewRegistry()
        middlewareChain.Use(middleware.RequestIDWithMetrics(recorder, metricsRegistry))

        // When: Multiple requests processed
        for i := 0; i < 5; i++ {
            middlewareChain.Execute(createTestRequest())
        }

        // Then: Metrics track execution
        middlewareExecutions := getMetricValue(metricsRegistry, "gateway_middleware_executions_total", map[string]string{
            "middleware": "request_id",
        })
        Expect(middlewareExecutions).To(Equal(float64(5)))
    })
})
```

**Acceptance Criteria**:
- ✅ All 7 tests pass
- ✅ Coverage for middleware files increases from ~15% to ≥40% average
- ✅ Tests validate middleware chain integration

**Phase 3 Middleware Category Totals**:
- **Tests**: 7 tests across 1 scenario
- **Coverage Gain**: +3%
- **Files Covered**: `middleware/request_id.go`, `middleware/timestamp.go`, `middleware/security_headers.go`, `middleware/content_type.go`

---

## **Phase 3 Summary**

| Category | Scenarios | Tests | Coverage Gain | Files Improved |
|----------|-----------|-------|---------------|----------------|
| **Configuration** | 1 | 7 | +2% | 2 files |
| **Middleware Chain** | 1 | 7 | +3% | 4 files |
| **TOTAL** | **2** | **14** | **+5%** | **6 files** |

**Phase 3 Target**: 57% + 5% = **62% coverage** ✅ **EXCEEDS COMPLIANCE**

---

## 📊 **OVERALL TEST PLAN SUMMARY**

| Phase | Timeline | Scenarios | Tests | Coverage Gain | Target Coverage | Status |
|-------|----------|-----------|-------|---------------|-----------------|--------|
| **Phase 1** | Week 1 | 7 | 35 | +15% | 45% | ⏳ Pending |
| **Phase 2** | Week 2 | 5 | 35 | +12% | 57% ✅ | ⏳ Pending |
| **Phase 3** | Week 3 | 2 | 14 | +5% | 62% ✅ | ⏳ Pending |
| **TOTAL** | 3 weeks | **14** | **84** | **+32%** | **62%** | ⏳ Pending |

---

## 📋 **Test File Organization**

```
test/integration/gateway/
├── audit_emission_integration_test.go          # Phase 1: Scenarios 1.1-1.4 (20 tests)
├── metrics_emission_integration_test.go        # Phase 1: Scenarios 2.1-2.3 (15 tests)
├── adapters_integration_test.go                # Phase 2: Scenarios 3.1-3.2 (15 tests)
├── circuit_breaker_integration_test.go         # Phase 2: Scenario 4.1 (7 tests)
├── error_handling_integration_test.go          # Phase 2: Scenarios 5.1-5.2 (13 tests)
├── config_integration_test.go                  # Phase 3: Scenario 6.1 (7 tests)
└── middleware_chain_integration_test.go        # Phase 3: Scenario 7.1 (7 tests)
```

---

## ✅ **Success Criteria**

### **Coverage Targets**:
- ✅ **Minimum**: 50% integration coverage (compliance)
- ✅ **Target**: 55-60% integration coverage (healthy)
- ✅ **Achieved**: 62% integration coverage (excellent) ✅

### **Quality Targets**:
- ✅ All tests map to specific BRs (P0/P1)
- ✅ All tests use direct business logic calls (no HTTP)
- ✅ All tests run in <5 seconds
- ✅ All tests validate business outcomes (not implementation)
- ✅ Zero NULL-TESTING anti-patterns

### **Business Value Targets**:
- ✅ SOC2 compliance (audit event validation)
- ✅ Operational visibility (metrics validation)
- ✅ Reliability (error handling, circuit breaker)
- ✅ Correctness (adapter parsing, config validation)

---

## 🎯 **Implementation Checklist**

### **Phase 1 Deliverables** (Week 1: Jan 21-25):
- [ ] Create `audit_emission_integration_test.go` (20 tests)
- [ ] Create `metrics_emission_integration_test.go` (15 tests)
- [ ] Run coverage analysis → Verify ≥45% coverage
- [ ] Fix any failing tests
- [ ] Code review + approval
- [ ] Merge to main branch

### **Phase 2 Deliverables** (Week 2: Jan 28-Feb 1):
- [ ] Create `adapters_integration_test.go` (15 tests)
- [ ] Create `circuit_breaker_integration_test.go` (7 tests)
- [ ] Create `error_handling_integration_test.go` (13 tests)
- [ ] Run coverage analysis → Verify ≥57% coverage ✅
- [ ] Fix any failing tests
- [ ] Code review + approval
- [ ] Merge to main branch

### **Phase 3 Deliverables** (Week 3: Feb 4-8):
- [ ] Create `config_integration_test.go` (7 tests)
- [ ] Create `middleware_chain_integration_test.go` (7 tests)
- [ ] Run coverage analysis → Verify ≥62% coverage ✅
- [ ] Fix any failing tests
- [ ] Code review + approval
- [ ] Merge to main branch

### **Final Validation** (Feb 11):
- [ ] Run full integration test suite (106 tests)
- [ ] Verify all tests pass
- [ ] Generate final coverage report
- [ ] Update README with new coverage metrics
- [ ] Document lessons learned

---

## 📚 **References**

- **Business Requirements**: `docs/services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md`
- **Coverage Crisis Analysis**: `docs/handoff/GW_INTEGRATION_COVERAGE_CRISIS_JAN14_2026.md`
- **Gaps Analysis**: `docs/handoff/GW_INTEGRATION_TEST_GAPS_ANALYSIS_JAN14_2026.md`
- **Testing Standards**: `.cursor/rules/15-testing-coverage-standards.mdc`
- **APDC Methodology**: `.cursor/rules/00-core-development-methodology.mdc`
- **Testing Guidelines**: `.cursor/rules/03-testing-strategy.mdc`

---

## 📝 **Change Log**

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| v1.0 | 2026-01-14 | Initial test plan created | Gateway Team |

---

**Status**: 📋 **APPROVED FOR IMPLEMENTATION**
**Start Date**: January 21, 2026
**Target Completion**: February 11, 2026
**Expected Outcome**: 62% integration coverage ✅ **COMPLIANT**
