# SignalProcessing Audit Query Pattern - CORRECT APPROACH
**Date**: January 10, 2026
**Purpose**: Document the correct pattern for querying audit events via DataStorage HTTP API

---

## ✅ **CORRECT Pattern: HTTP API + Flush**

### **Step 1: Flush Buffered Events**
Always flush the audit store before querying to ensure events are written:

```go
By("Flushing audit store to ensure events are written to DataStorage")
flushCtx, flushCancel := context.WithTimeout(ctx, 5*time.Second)
defer flushCancel()

err := auditStore.Flush(flushCtx)
Expect(err).ToNot(HaveOccurred(), "Audit store flush must succeed")
```

### **Step 2: Query via HTTP API**
Use the ogen client to query DataStorage's HTTP API:

```go
By("Querying DataStorage HTTP API for audit events")
params := ogenclient.QueryAuditEventsParams{
    EventType:     ogenclient.NewOptString(spaudit.EventTypeSignalProcessed),
    CorrelationID: ogenclient.NewOptString(correlationID),
}

resp, err := dsClient.QueryAuditEvents(ctx, params)
Expect(err).ToNot(HaveOccurred(), "Query must succeed")
Expect(resp.Events).To(HaveLen(1), "Should have exactly 1 event")
```

### **Step 3: Validate Event Data**
Verify the event contents match expectations:

```go
event := resp.Events[0]
Expect(event.EventType).To(Equal(spaudit.EventTypeSignalProcessed))
Expect(event.EventCategory).To(Equal("signalprocessing"))
Expect(event.EventAction).To(Equal("processed"))
Expect(event.EventOutcome).To(Equal("success"))
Expect(event.CorrelationID).To(Equal(correlationID))

// Validate JSONB event_data
var eventData map[string]interface{}
err = json.Unmarshal(event.EventData, &eventData)
Expect(err).ToNot(HaveOccurred())
Expect(eventData).To(HaveKey("signal_fingerprint"))
```

---

## 🔄 **Complete Example: Signal Processed Test**

```go
It("should create 'signalprocessing.signal.processed' audit event", func() {
    By("1. Creating test resources")
    ns := &corev1.Namespace{
        ObjectMeta: metav1.ObjectMeta{
            Name: "test-ns-" + uuid.New().String()[:8],
            Labels: map[string]string{
                "kubernaut.ai/environment": "production",
            },
        },
    }
    Expect(k8sClient.Create(ctx, ns)).To(Succeed())

    rr := &remediationv1alpha1.RemediationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-rr-" + uuid.New().String()[:8],
            Namespace: ns.Name,
        },
        Spec: remediationv1alpha1.RemediationRequestSpec{
            Fingerprint: "fp-test-123",
        },
    }
    Expect(k8sClient.Create(ctx, rr)).To(Succeed())

    correlationID := "rr-" + uuid.New().String()[:8]

    sp := &signalprocessingv1alpha1.SignalProcessing{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-sp-" + uuid.New().String()[:8],
            Namespace: ns.Name,
        },
        Spec: signalprocessingv1alpha1.SignalProcessingSpec{
            CorrelationID: correlationID,
            ParentRemediationRequest: signalprocessingv1alpha1.ParentRemediationRequest{
                Name: rr.Name,
            },
            Fingerprint: "fp-test-123",
            Namespace:   ns.Name,
        },
    }
    Expect(k8sClient.Create(ctx, sp)).To(Succeed())

    By("2. Waiting for processing to complete")
    Eventually(func() string {
        var updated signalprocessingv1alpha1.SignalProcessing
        err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated)
        if err != nil {
            return ""
        }
        return updated.Status.Phase
    }, timeout, interval).Should(Equal("Completed"))

    By("3. Flushing audit store before querying")
    flushCtx, flushCancel := context.WithTimeout(ctx, 5*time.Second)
    defer flushCancel()

    err := auditStore.Flush(flushCtx)
    Expect(err).ToNot(HaveOccurred(), "Audit store flush must succeed")

    By("4. Querying DataStorage HTTP API for audit event")
    Eventually(func() int {
        params := ogenclient.QueryAuditEventsParams{
            EventType:     ogenclient.NewOptString(spaudit.EventTypeSignalProcessed),
            CorrelationID: ogenclient.NewOptString(correlationID),
        }

        resp, err := dsClient.QueryAuditEvents(ctx, params)
        if err != nil {
            GinkgoWriter.Printf("Query error: %v\n", err)
            return 0
        }
        return len(resp.Events)
    }, timeout, interval).Should(Equal(1), "Should have exactly 1 'signal.processed' event")

    By("5. Validating event data")
    params := ogenclient.QueryAuditEventsParams{
        EventType:     ogenclient.NewOptString(spaudit.EventTypeSignalProcessed),
        CorrelationID: ogenclient.NewOptString(correlationID),
    }

    resp, err := dsClient.QueryAuditEvents(ctx, params)
    Expect(err).ToNot(HaveOccurred())
    Expect(resp.Events).To(HaveLen(1))

    event := resp.Events[0]
    Expect(event.EventType).To(Equal(spaudit.EventTypeSignalProcessed))
    Expect(event.EventCategory).To(Equal("signalprocessing"))
    Expect(event.EventAction).To(Equal("processed"))
    Expect(event.EventOutcome).To(Equal("success"))
    Expect(event.CorrelationID).To(Equal(correlationID))
    Expect(event.ResourceType).To(Equal("SignalProcessing"))
    Expect(event.ResourceID).To(Equal(sp.Name))
    Expect(event.Namespace).To(Equal(ogenclient.NewOptString(ns.Name)))

    // Validate JSONB event_data
    var eventData map[string]interface{}
    err = json.Unmarshal(event.EventData, &eventData)
    Expect(err).ToNot(HaveOccurred())
    Expect(eventData).To(HaveKey("signal_fingerprint"))
    Expect(eventData["signal_fingerprint"]).To(Equal("fp-test-123"))
})
```

---

## 🚫 **WRONG Pattern: Direct SQL (DO NOT USE)**

```go
// ❌ WRONG: Violates service boundaries
err := testDB.QueryRow(`
    SELECT COUNT(*)
    FROM audit_events
    WHERE event_type = $1
      AND correlation_id = $2
`, eventType, correlationID).Scan(&eventCount)
```

**Why This Is Wrong**:
- Violates service boundaries
- Doesn't test production behavior (which uses HTTP API)
- Creates tight coupling to DataStorage's internal schema
- Bypasses the actual integration point (HTTP API)

---

## 📋 **Helper Function Pattern**

Create reusable helpers for common query patterns:

```go
// Helper: Count events by type and correlation ID
func countAuditEvents(ctx context.Context, eventType, correlationID string) int {
    params := ogenclient.QueryAuditEventsParams{
        EventType:     ogenclient.NewOptString(eventType),
        CorrelationID: ogenclient.NewOptString(correlationID),
    }

    resp, err := dsClient.QueryAuditEvents(ctx, params)
    if err != nil {
        GinkgoWriter.Printf("Query error: %v\n", err)
        return 0
    }
    return len(resp.Events)
}

// Helper: Get latest event by type and correlation ID
func getLatestAuditEvent(ctx context.Context, eventType, correlationID string) (*ogenclient.AuditEvent, error) {
    params := ogenclient.QueryAuditEventsParams{
        EventType:     ogenclient.NewOptString(eventType),
        CorrelationID: ogenclient.NewOptString(correlationID),
        Limit:         ogenclient.NewOptInt(1),
    }

    resp, err := dsClient.QueryAuditEvents(ctx, params)
    if err != nil {
        return nil, err
    }

    if len(resp.Events) == 0 {
        return nil, fmt.Errorf("no events found")
    }

    return &resp.Events[0], nil
}

// Usage in tests
It("should create audit event", func() {
    // ... create resources and wait for completion ...

    By("Flushing audit store")
    flushCtx, flushCancel := context.WithTimeout(ctx, 5*time.Second)
    defer flushCancel()
    err := auditStore.Flush(flushCtx)
    Expect(err).ToNot(HaveOccurred())

    By("Verifying event count")
    Eventually(func() int {
        return countAuditEvents(ctx, spaudit.EventTypeSignalProcessed, correlationID)
    }, timeout, interval).Should(Equal(1))

    By("Validating event data")
    event, err := getLatestAuditEvent(ctx, spaudit.EventTypeSignalProcessed, correlationID)
    Expect(err).ToNot(HaveOccurred())
    Expect(event.EventOutcome).To(Equal("success"))
})
```

---

## ⚡ **Key Principles**

1. **Always Flush First**: Call `auditStore.Flush()` before querying
2. **Use HTTP API**: Query via `dsClient.QueryAuditEvents()`, not SQL
3. **Use Eventually**: Events may take time to be written, use polling
4. **Service Boundaries**: SignalProcessing should never access DataStorage's database
5. **Production Parity**: Tests should mirror production behavior (HTTP API)

---

## 🔗 **References**

- **Ogen Client**: `pkg/datastorage/ogen-client/oas_client_gen.go`
- **Query Params**: `pkg/datastorage/ogen-client/oas_parameters_gen.go:1105`
- **Service Boundaries**: `docs/handoff/SP_INTEGRATION_CRITICAL_ARCHITECTURE_VIOLATION.md`

---

**Status**: ✅ AUTHORITATIVE PATTERN
**Use This**: For all SignalProcessing (and other service) audit queries
