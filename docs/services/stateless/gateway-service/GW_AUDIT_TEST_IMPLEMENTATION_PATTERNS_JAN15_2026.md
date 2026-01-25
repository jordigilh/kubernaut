# Gateway Audit Test Implementation Patterns

**Date**: 2026-01-15
**Status**: ✅ API Triaged, Patterns Documented
**Tests Implemented**: 2/20 (10%)
**Test Plan**: [`GW_INTEGRATION_TEST_PLAN_V1.0.md`](./GW_INTEGRATION_TEST_PLAN_V1.0.md)

---

## 🎯 **Purpose**

This document provides **proven patterns** for implementing Gateway audit emission integration tests (GW-INT-AUD-001 to AUD-020).

All patterns follow the **SignalProcessing/AIAnalysis proven approach** and use the **established audit test helpers** in `test/integration/gateway/audit_test_helpers.go`.

---

## 📋 **Quick Reference: Audit Query API**

### **Core API Pattern**

```go
// Query events by type and correlation ID
params := ogenclient.QueryAuditEventsParams{
    EventType:     ogenclient.NewOptString("gateway.crd.created"),
    CorrelationID: ogenclient.NewOptString(correlationID),
}

resp, err := ogenClient.QueryAuditEvents(ctx, params)
Expect(err).ToNot(HaveOccurred())

events := resp.Data // []ogenclient.AuditEvent
```

### **Available Query Parameters**

| Parameter | Type | Purpose | Example |
|-----------|------|---------|---------|
| `EventType` | `OptString` | Filter by event type | `"gateway.signal.received"` |
| `EventCategory` | `OptString` | Filter by category | `"audit"` |
| `CorrelationID` | `OptString` | Filter by correlation ID | `uuid.New().String()` |
| `Limit` | `OptInt` | Limit results | `1` (most recent) |
| `EventOutcome` | `OptQueryAuditEventsEventOutcome` | Filter by outcome | `"success"`, `"failure"` |

### **Response Structure**

```go
type AuditEventsQueryResponse struct {
    Data []AuditEvent // Array of audit events
}

type AuditEvent struct {
    EventID        string
    EventType      string
    EventAction    string
    EventCategory  string
    EventOutcome   string
    CorrelationID  string
    EventData      EventData // Union type (GatewayAuditPayload, etc.)
    CreatedAt      time.Time
}
```

---

## 🛠️ **Available Helper Functions**

### **Shared Query Helpers** (`test/shared/helpers/audit.go`)

**IMPORTANT**: Use shared helpers for ALL audit queries (DRY principle):

```go
import sharedhelpers "github.com/jordigilh/kubernaut/test/shared/helpers"

// Create ogen client
client, err := createOgenClient() // Gateway-specific port 18091

// Query with correlation ID + event type
events, total, err := sharedhelpers.QueryAuditEvents(ctx, client, &correlationID, &eventType, nil)

// Query by correlation ID only (most common)
events, total, err := sharedhelpers.QueryAuditEventsByCorrelationID(ctx, client, correlationID)

// Query by event type only
events, total, err := sharedhelpers.QueryAuditEventsByType(ctx, client, "gateway.crd.created")

// Query by category
events, total, err := sharedhelpers.QueryAuditEventsByCategory(ctx, client, "gateway")
```

### **Gateway-Specific Helpers** (`test/integration/gateway/audit_test_helpers.go`)

**Use these for Gateway-specific logic only:**

```go
// Create ogen client (Gateway port 18091)
client, err := createOgenClient()

// Wait for event with timeout (async validation)
event, err := waitForAuditEvent(ctx, client, "gateway.crd.created", correlationID, 10*time.Second)

// Extract Gateway payload from event
payload, ok := extractGatewayPayload(event)

// Validate signal.received payload
err := validateSignalReceivedPayload(event, expectedNamespace)

// Validate crd.created payload
err := validateCRDCreatedPayload(event, expectedNamespace)

// Validate deduplicated payload
err := validateDeduplicatedPayload(event, expectedRRName)
```

### **Shared Validators** (`test/shared/validators/audit.go`)

```go
import sharedvalidators "github.com/jordigilh/kubernaut/test/shared/validators"

// Validate event matches expected values
sharedvalidators.ValidateAuditEvent(event, sharedvalidators.ExpectedAuditEvent{
    EventType:     "gateway.crd.created",
    EventCategory: ogenclient.AuditEventEventCategoryGateway,
    EventAction:   "created",
    CorrelationID: correlationID,
})

// Quick validation of required fields
sharedvalidators.ValidateAuditEventHasRequiredFields(event)

// Gomega matcher
Expect(event).To(sharedvalidators.MatchAuditEvent(expected))
```

---

## 📝 **Test Implementation Patterns**

### **Pattern 1: Basic Signal Processing Validation**

**Use Case**: Validate that a signal is received and processed
**Tests**: GW-INT-AUD-001, GW-INT-AUD-003
**Example**: Signal received → CRD created

```go
It("[GW-INT-AUD-001] should create RemediationRequest CRD for new signal", func() {
    By("1. Create and process signal")
    alert := createPrometheusAlert(testNamespace, "TestAlert", "critical", "", correlationID)
    prometheusAdapter := adapters.NewPrometheusAdapter()
    signal, err := prometheusAdapter.Parse(ctx, alert)
    Expect(err).ToNot(HaveOccurred())

    gatewayConfig := createGatewayConfig(fmt.Sprintf("http://localhost:%d", gatewayDataStoragePort))
    gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient)
    Expect(err).ToNot(HaveOccurred())

    response, err := gwServer.ProcessSignal(ctx, signal)
    Expect(err).ToNot(HaveOccurred())
    Expect(response.RemediationRequestName).ToNot(BeEmpty())

    By("2. Verify CRD exists in Kubernetes")
    var rr remediationv1alpha1.RemediationRequest
    Eventually(func() error {
        return k8sClient.Get(ctx, client.ObjectKey{
            Name:      response.RemediationRequestName,
            Namespace: testNamespace,
        }, &rr)
    }, 10*time.Second, 500*time.Millisecond).Should(Succeed())

    By("3. Validate CRD metadata")
    Expect(rr.Spec.SignalFingerprint).To(Equal(signal.Fingerprint))
    Expect(rr.Spec.SignalType).To(Equal("prometheus-alert"))
})
```

---

### **Pattern 2: Audit Event Query and Validation**

**Use Case**: Query DataStorage and validate audit event payload
**Tests**: GW-INT-AUD-004, GW-INT-AUD-006, GW-INT-AUD-007
**Example**: CRD created → Audit event emitted

```go
It("[GW-INT-AUD-006] should emit gateway.crd.created audit event", func() {
    By("1. Process signal to create CRD")
    // ... (process signal as in Pattern 1)

    By("2. Query audit event from DataStorage using shared helper")
    client, err := createOgenClient()
    Expect(err).ToNot(HaveOccurred())

    var crdCreatedEvent *ogenclient.AuditEvent
    eventType := "gateway.crd.created"
    Eventually(func() bool {
        events, _, err := sharedhelpers.QueryAuditEvents(ctx, client, &correlationID, &eventType, nil)
        if err != nil || len(events) == 0 {
            return false
        }
        crdCreatedEvent = &events[0]
        return true
    }, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
        "gateway.crd.created event should exist in DataStorage")

    By("3. Extract and validate Gateway payload")
    payload, ok := extractGatewayPayload(crdCreatedEvent)
    Expect(ok).To(BeTrue(), "Event should have Gateway audit payload")

    By("4. Validate RemediationRequest reference")
    rrRef, hasRR := payload.RemediationRequest.Get()
    Expect(hasRR).To(BeTrue(), "BR-GATEWAY-056: RemediationRequest field must be present")
    Expect(rrRef).To(ContainSubstring(testNamespace), "RR reference must contain namespace")
    Expect(rrRef).To(ContainSubstring(response.RemediationRequestName), "RR reference must contain name")
})
```

---

### **Pattern 3: Deduplication Validation**

**Use Case**: Validate deduplication logic and audit events
**Tests**: GW-INT-AUD-002, GW-INT-AUD-011, GW-INT-AUD-012
**Example**: Duplicate signal → Deduplicated event

```go
It("[GW-INT-AUD-002] should deduplicate based on fingerprint", func() {
    fingerprint := "a1b2c3d4e5f67890a1b2c3d4e5f67890a1b2c3d4e5f67890a1b2c3d4e5f67890"

    By("1. Process first signal - should create CRD")
    signal1, _ := prometheusAdapter.Parse(ctx, firstAlert)
    response1, err := gwServer.ProcessSignal(ctx, signal1)
    Expect(err).ToNot(HaveOccurred())
    Expect(response1.Status).To(Equal("created"))

    By("2. Process duplicate signal - should be deduplicated")
    signal2, _ := prometheusAdapter.Parse(ctx, duplicateAlert)
    signal2.Fingerprint = fingerprint // Same fingerprint
    response2, err := gwServer.ProcessSignal(ctx, signal2)
    Expect(err).ToNot(HaveOccurred())
    Expect(response2.Status).To(Equal("duplicate"))

    By("3. Verify only ONE CRD exists")
    var rrList remediationv1alpha1.RemediationRequestList
    err = k8sClient.List(ctx, &rrList, client.InNamespace(testNamespace))
    Expect(err).ToNot(HaveOccurred())

    matchingCRDs := 0
    for _, rr := range rrList.Items {
        if rr.Spec.SignalFingerprint == fingerprint {
            matchingCRDs++
        }
    }
    Expect(matchingCRDs).To(Equal(1), "BR-GATEWAY-057: Only one CRD per fingerprint")
})
```

---

### **Pattern 4: Signal Labels/Annotations Preservation**

**Use Case**: Validate signal metadata is preserved in audit events
**Tests**: GW-INT-AUD-004
**Example**: Custom labels → Audit event payload

```go
It("[GW-INT-AUD-004] should preserve signal_labels in audit payload", func() {
    customLabels := map[string]string{
        "team":        "platform",
        "environment": "production",
        "component":   "kubelet",
    }

    By("1. Create signal with custom labels")
    alertPayload := []byte(fmt.Sprintf(`{
        "alerts": [{
            "labels": {
                "alertname": "NodeDiskPressure",
                "team": "%s",
                "environment": "%s",
                "component": "%s",
                "namespace": "%s"
            }
        }]
    }`, customLabels["team"], customLabels["environment"],
        customLabels["component"], testNamespace))

    By("2. Process signal")
    signal, _ := prometheusAdapter.Parse(ctx, alertPayload)
    response, err := gwServer.ProcessSignal(ctx, signal)
    Expect(err).ToNot(HaveOccurred())

    By("3. Query and validate audit event using shared helper")
    client, err := createOgenClient()
    Expect(err).ToNot(HaveOccurred())

    eventType := "gateway.crd.created"
    Eventually(func() bool {
        events, _, _ := sharedhelpers.QueryAuditEvents(ctx, client, &correlationID, &eventType, nil)
        if len(events) == 0 {
            return false
        }

        payload, ok := extractGatewayPayload(&events[0])
        if !ok {
            return false
        }

        signalLabels, hasLabels := payload.SignalLabels.Get()
        if !hasLabels {
            return false
        }

        // Validate all custom labels are preserved
        Expect(signalLabels).To(HaveKeyWithValue("team", customLabels["team"]))
        Expect(signalLabels).To(HaveKeyWithValue("environment", customLabels["environment"]))
        Expect(signalLabels).To(HaveKeyWithValue("component", customLabels["component"]))
        return true
    }, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
})
```

---

### **Pattern 5: Error Scenario Validation**

**Use Case**: Validate error audit events
**Tests**: GW-INT-AUD-016, GW-INT-AUD-017
**Example**: K8s API failure → Error audit event

```go
It("[GW-INT-AUD-016] should emit gateway.crd.failed audit event on K8s API error", func() {
    By("1. Create invalid signal (will cause K8s API error)")
    signal := createInvalidSignal() // e.g., missing required fields

    By("2. Process signal - expect failure")
    response, err := gwServer.ProcessSignal(ctx, signal)
    Expect(err).To(HaveOccurred(), "Signal processing should fail")

    By("3. Query error audit event")
    var errorEvent *ogenclient.AuditEvent
    Eventually(func() bool {
        event, err := getLatestAuditEvent(ctx, "gateway.crd.failed", correlationID)
        if err != nil || event == nil {
            return false
        }
        errorEvent = event
        return true
    }, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

    By("4. Validate error details in payload")
    payload, ok := extractGatewayPayload(errorEvent)
    Expect(ok).To(BeTrue())

    errorDetails, hasError := payload.ErrorDetails.Get()
    Expect(hasError).To(BeTrue(), "BR-GATEWAY-058: Error details must be present")
    Expect(errorDetails.ErrorType).ToNot(BeEmpty())
    Expect(errorDetails.ErrorMessage).To(ContainSubstring("kubernetes"))
})
```

---

## 🔄 **Common Patterns Summary**

### **Test Structure**

```go
var _ = Describe("BR-XXX: Feature Description", func() {
    var (
        testNamespace string
        correlationID string
        ctx           = context.Background()
    )

    BeforeEach(func() {
        processID := GinkgoParallelProcess()
        testNamespace = fmt.Sprintf("gw-test-%d-%s", processID, uuid.New().String()[:8])
        correlationID = uuid.New().String()

        // Create test namespace
        ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
        Expect(k8sClient.Create(ctx, ns)).To(Succeed())
    })

    AfterEach(func() {
        // Clean up namespace
        ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
        _ = k8sClient.Delete(ctx, ns)
    })

    It("[GW-INT-AUD-XXX] should ...", func() {
        // Test implementation
    })
})
```

### **Eventually() Pattern for Async Validation**

```go
// Wait for CRD to exist
Eventually(func() error {
    return k8sClient.Get(ctx, rrKey, &rr)
}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

// Wait for audit event
Eventually(func() bool {
    events, _ := queryAuditEventsByType(ctx, eventType, correlationID)
    return len(events) > 0
}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
```

### **Payload Extraction Pattern**

```go
// Extract Gateway payload from event
payload, ok := extractGatewayPayload(event)
Expect(ok).To(BeTrue(), "Event should have Gateway audit payload")

// Access optional fields safely
if rrRef, hasRR := payload.RemediationRequest.Get(); hasRR {
    Expect(rrRef).ToNot(BeEmpty())
}

if labels, hasLabels := payload.SignalLabels.Get(); hasLabels {
    Expect(labels).To(HaveKeyWithValue("team", "platform"))
}
```

---

## 📊 **GatewayAuditPayload Field Reference**

### **Required Fields**

| Field | Type | Description | Usage |
|-------|------|-------------|-------|
| `EventType` | `GatewayAuditPayloadEventType` | Event type discriminator | Matches parent event_type |
| `SignalType` | `GatewayAuditPayloadSignalType` | Signal source | `"prometheus-alert"`, `"kubernetes-event"` |
| `AlertName` | `string` | Alert name | `"KubePodCrashLooping"` |
| `Namespace` | `string` | K8s namespace | `"default"` |
| `Fingerprint` | `string` | Signal fingerprint (SHA256) | 64-char hex string |

### **Optional Fields**

| Field | Type | Description | Usage |
|-------|------|-------------|-------|
| `OriginalPayload` | `OptGatewayAuditPayloadOriginalPayload` | Full signal payload | RR reconstruction |
| `SignalLabels` | `OptGatewayAuditPayloadSignalLabels` | Signal labels | Metadata preservation |
| `SignalAnnotations` | `OptGatewayAuditPayloadSignalAnnotations` | Signal annotations | Metadata preservation |
| `Severity` | `OptGatewayAuditPayloadSeverity` | Severity level | `"critical"`, `"warning"` |
| `ResourceKind` | `OptString` | K8s resource kind | `"Pod"`, `"Deployment"` |
| `ResourceName` | `OptString` | Resource name | `"my-pod-123"` |
| `RemediationRequest` | `OptString` | CRD reference | `"namespace/name"` |
| `DeduplicationStatus` | `OptGatewayAuditPayloadDeduplicationStatus` | Dedup status | `"new"`, `"duplicate"` |
| `OccurrenceCount` | `OptInt32` | Occurrence count | Deduplication tracking |
| `ErrorDetails` | `OptErrorDetails` | Error information | Failure scenarios |

---

## ✅ **Test Implementation Checklist**

Before implementing a test, ensure:

- [ ] Test ID matches test plan (GW-INT-AUD-XXX)
- [ ] Business requirement referenced in test description (BR-GATEWAY-XXX)
- [ ] Test section documented (e.g., 1.1.1)
- [ ] Priority level known (P0/P1)
- [ ] Unique namespace created per test (parallel isolation)
- [ ] Unique correlation ID used (parallel isolation)
- [ ] Eventually() used for async validation (CRDs, audit events)
- [ ] Payload extraction uses helper functions
- [ ] Field access uses `.Get()` for optional fields
- [ ] Test cleanup registered (namespace deletion)
- [ ] Debug logging included for parallel troubleshooting
- [ ] Test passes in parallel execution (`ginkgo -p -procs=4`)

---

## 📈 **Implementation Progress Tracking**

Update this table as tests are implemented:

| Test ID | Name | Status | Notes |
|---------|------|--------|-------|
| GW-INT-AUD-001 | Prometheus Signal Audit | ✅ PASSING | CRD creation validated |
| GW-INT-AUD-002 | Deduplication Logic | ✅ PASSING | Fingerprint-based dedup |
| GW-INT-AUD-003 | Correlation ID Format | 📝 TODO | UUID validation |
| GW-INT-AUD-004 | Signal Labels Preservation | 📝 TODO | Metadata preservation |
| GW-INT-AUD-005 | Audit Failure Non-Blocking | 🔀 UNIT | Moved to unit tests |
| GW-INT-AUD-006 | CRD Created Audit | 📝 TODO | Audit event emission |
| GW-INT-AUD-007 | CRD Target Resource | 📝 TODO | Resource metadata |
| GW-INT-AUD-008 | CRD Fingerprint | 📝 TODO | Fingerprint validation |
| GW-INT-AUD-009 | CRD Occurrence Count | 📝 TODO | Dedup count tracking |
| GW-INT-AUD-010 | CRD Unique Correlation IDs | 📝 TODO | Correlation ID uniqueness |
| GW-INT-AUD-011 | Signal Deduplicated Audit | 📝 TODO | Dedup audit event |
| GW-INT-AUD-012 | Dedup Existing RR Name | 📝 TODO | RR reference validation |
| GW-INT-AUD-013 | Dedup Occurrence Count | 📝 TODO | Dedup counter validation |
| GW-INT-AUD-014 | Dedup Multiple Fingerprints | 📝 TODO | Multiple signal tracking |
| GW-INT-AUD-015 | Dedup Phase Rejection | 📝 TODO | Phase-based dedup |
| GW-INT-AUD-016 | CRD Failed K8s API Error | 📝 TODO | K8s failure handling |
| GW-INT-AUD-017 | CRD Failed Error Type | 📝 TODO | Error type classification |
| GW-INT-AUD-018 | CRD Failed Retry Events | 📝 TODO | Retry audit trail |
| GW-INT-AUD-019 | CRD Failed Circuit Breaker | 📝 TODO | Circuit breaker events |
| GW-INT-AUD-020 | Audit ID Uniqueness | 📝 TODO | Event ID validation |

---

## 🚀 **Next Steps**

1. Implement remaining 18 tests using established patterns
2. Run parallel test validation: `ginkgo -p -procs=4 ./test/integration/gateway/...`
3. Validate >50% integration coverage target
4. Update test plan status from 📝 Spec to ✅ Pass

---

**Document Status**: ✅ Active
**Last Updated**: 2026-01-15
**Maintainer**: Development Team
**Related Documents**: [`GW_INTEGRATION_TEST_PLAN_V1.0.md`](./GW_INTEGRATION_TEST_PLAN_V1.0.md)
