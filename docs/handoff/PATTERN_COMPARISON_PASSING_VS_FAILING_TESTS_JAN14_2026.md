# Pattern Comparison: Passing vs Failing Tests - Jan 14, 2026

## 🎯 **Pattern Analysis: How Tests Query Audit Events**

**Total Tests**: 87 runnable specs
**Passing**: 85 tests (97.7%)
**Failing**: 2 tests (2.3%)

---

## ✅ **Passing Tests Pattern** (85 tests)

### **Query Method: correlation_id (Server-Side Filtering)**

All 85 passing tests use this pattern:

```go
// Step 1: Create RemediationRequest with unique name
rrName := "audit-test-rr-01"
rr := CreateTestRemediationRequest(rrName, ns, fingerprint, severity, targetResource)
Expect(k8sClient.Create(ctx, rr)).To(Succeed())

// Step 2: Use RR name as correlation ID
correlationID := rrName

// Step 3: Create SignalProcessing with parent RR
sp := CreateTestSignalProcessingWithParent("audit-test-sp-01", ns, rr, fingerprint, targetResource)
Expect(k8sClient.Create(ctx, sp)).To(Succeed())

// Step 4: Flush audit store
flushAuditStoreAndWait()

// Step 5: Query by correlation_id (server-side filtering)
Eventually(func() int {
    return countAuditEvents(eventType, correlationID)  // ✅ Uses correlation_id
}, 120*time.Second, 500*time.Millisecond).Should(Equal(1))

// Step 6: Fetch event details
event, err := getLatestAuditEvent(eventType, correlationID)  // ✅ Uses correlation_id
```

### **Helper Functions Used**

**`countAuditEvents(eventType, correlationID)`**:
```go
func countAuditEvents(eventType, correlationID string) int {
    params := ogenclient.QueryAuditEventsParams{
        EventType:     ogenclient.NewOptString(eventType),
        CorrelationID: ogenclient.NewOptString(correlationID),  // ✅ Server-side filter
    }

    resp, err := dsClient.QueryAuditEvents(ctx, params)
    if err != nil {
        return 0
    }
    return len(resp.Data)
}
```

**`getLatestAuditEvent(eventType, correlationID)`**:
```go
func getLatestAuditEvent(eventType, correlationID string) (*ogenclient.AuditEvent, error) {
    params := ogenclient.QueryAuditEventsParams{
        EventType:     ogenclient.NewOptString(eventType),
        CorrelationID: ogenclient.NewOptString(correlationID),  // ✅ Server-side filter
        Limit:         ogenclient.NewOptInt(1),
    }

    resp, err := dsClient.QueryAuditEvents(ctx, params)
    if err != nil {
        return nil, err
    }
    if len(resp.Data) == 0 {
        return nil, nil
    }
    return &resp.Data[0], nil
}
```

### **Why This Works**

| Aspect | Implementation | Result |
|--------|----------------|--------|
| **Unique ID** | `correlation_id = rrName` (e.g., "audit-test-rr-01") | ✅ Unique per test |
| **Filtering** | DataStorage server-side | ✅ Efficient |
| **Parallel Safety** | Only returns events for THIS test | ✅ No cross-process contamination |
| **Pagination** | Returns only matching events | ✅ No pagination issues |
| **Query Speed** | Index on correlation_id | ✅ Fast (2-32ms) |

---

## ❌ **Failing Test Pattern** (1 test)

### **Query Method: namespace (Client-Side Filtering)**

**Test**: `should emit 'classification.decision' audit event with both external and normalized severity`
**File**: `test/integration/signalprocessing/severity_integration_test.go:278`

```go
// Step 1: Create SignalProcessing WITHOUT parent RR
sp := createTestSignalProcessingCRD(namespace, "test-audit-event")
sp.Spec.Signal.Severity = "Sev2"
Expect(k8sClient.Create(ctx, sp)).To(Succeed())

// Step 2: Wait for processing
Eventually(func(g Gomega) {
    var updated signalprocessingv1alpha1.SignalProcessing
    g.Expect(k8sClient.Get(ctx, types.NamespacedName{
        Name:      sp.Name,
        Namespace: sp.Namespace,
    }, &updated)).To(Succeed())
    g.Expect(updated.Status.Severity).ToNot(BeEmpty())
}, "60s", "2s").Should(Succeed())

// Step 3: Flush audit store
flushAuditStoreAndWait()

// Step 4: Query by namespace (client-side filtering) ❌
Eventually(func(g Gomega) {
    events := queryAuditEvents(ctx, namespace, eventType)  // ❌ Uses namespace
    g.Expect(events).ToNot(BeEmpty())
    // ... assertions ...
}, "30s", "2s").Should(Succeed())
```

### **Helper Function Used**

**`queryAuditEvents(ctx, namespace, eventType)`**:
```go
func queryAuditEvents(ctx context.Context, namespace, eventType string) []ogenclient.AuditEvent {
    params := ogenclient.QueryAuditEventsParams{
        EventType: ogenclient.NewOptString(eventType),
        // ❌ NO namespace parameter to DataStorage
    }

    resp, err := dsClient.QueryAuditEvents(ctx, params)
    if err != nil {
        return []ogenclient.AuditEvent{}
    }

    // ❌ Client-side namespace filtering
    var filtered []ogenclient.AuditEvent
    for _, event := range resp.Data {
        if event.Namespace.Value == namespace {
            filtered = append(filtered, event)
        }
    }

    return filtered  // Returns 0 in parallel execution
}
```

### **Why This Fails**

| Aspect | Implementation | Result |
|--------|----------------|--------|
| **Unique ID** | Uses `namespace` (shared by multiple tests) | ❌ Not unique |
| **Filtering** | Client-side (after getting 50 events) | ❌ Inefficient |
| **Parallel Safety** | Gets events from ALL 12 processes | ❌ Cross-process contamination |
| **Pagination** | Default limit 100, returns first 50 | ❌ This test's events may not be in first 50 |
| **Query Speed** | Must fetch 50 events then filter | ❌ Slower |

---

## 🔍 **Key Differences**

### **Correlation ID vs Namespace**

| Aspect | correlation_id (Passing) | namespace (Failing) |
|--------|--------------------------|---------------------|
| **Uniqueness** | ✅ Unique per CRD | ❌ Shared across multiple CRDs |
| **Server-side Filter** | ✅ Yes (DataStorage filters) | ❌ No (client filters) |
| **Parallel Safe** | ✅ Yes (only THIS test's events) | ❌ No (gets all processes' events) |
| **Pagination** | ✅ No issue (few events per ID) | ❌ Issue (may exceed 50 limit) |
| **Used By** | 85 passing tests | 1 failing test |

### **What is correlation_id?**

From SignalProcessing audit client:
```go
// correlation_id = RemediationRequest name (if parent exists)
// OR
// correlation_id = SignalProcessing CRD name (if no parent)
```

**In passing tests**: `correlation_id = rrName` (e.g., "audit-test-rr-01")
**In failing test**: No correlation_id used, tries namespace instead

---

## 🔧 **The Fix for Failing Test**

### **Change Query Method to Match Passing Tests**

**BEFORE (broken)**:
```go
// Line 222: Create SP without parent RR
sp := createTestSignalProcessingCRD(namespace, "test-audit-event")

// Line 251: Query by namespace
events := queryAuditEvents(ctx, namespace, "signalprocessing.classification.decision")
```

**AFTER (fixed)**:
```go
// Line 222: Create SP without parent RR (no change needed)
sp := createTestSignalProcessingCRD(namespace, "test-audit-event")

// NEW: Get correlation ID from SignalProcessing name
correlationID := sp.Name  // e.g., "test-audit-event"

// Line 251: Query by correlation_id (like passing tests)
Eventually(func(g Gomega) {
    count := countAuditEvents("signalprocessing.classification.decision", correlationID)
    g.Expect(count).To(Equal(1), "classification.decision audit event should exist")
}, "30s", "500ms").Should(Succeed())

// Fetch event details
event, err := getLatestAuditEvent("signalprocessing.classification.decision", correlationID)
g.Expect(err).ToNot(HaveOccurred())
g.Expect(event).ToNot(BeNil())

// Now validate event fields
eventData, err := eventDataToMap(event.EventData)
g.Expect(err).ToNot(HaveOccurred())
// ... rest of assertions ...
```

---

## 📊 **Verification from SignalProcessing Audit Client**

**Question**: What does SignalProcessing use as correlation_id?

**Answer**: From `pkg/signalprocessing/audit/client.go`:
```go
// When parent RR exists:
correlationID = sp.Spec.RemediationRequestRef.Name

// When no parent RR:
correlationID = sp.Name
```

**For failing test**:
- No parent RemediationRequest
- `correlation_id = sp.Name = "test-audit-event"`

---

## ✅ **Pattern Recommendation**

### **Standard Pattern for SignalProcessing Audit Tests**

```go
// 1. Create test resources
sp := createTestSignalProcessingCRD(namespace, "test-audit-event")
// ... configure sp ...
Expect(k8sClient.Create(ctx, sp)).To(Succeed())

// 2. Determine correlation ID
var correlationID string
if sp.Spec.RemediationRequestRef != nil && sp.Spec.RemediationRequestRef.Name != "" {
    correlationID = sp.Spec.RemediationRequestRef.Name  // Parent RR name
} else {
    correlationID = sp.Name  // SignalProcessing CRD name
}

// 3. Wait for processing
Eventually(func() signalprocessingv1alpha1.SignalProcessingPhase {
    var updated signalprocessingv1alpha1.SignalProcessing
    k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}, &updated)
    return updated.Status.Phase
}, "60s", "1s").Should(Equal(signalprocessingv1alpha1.PhaseCompleted))

// 4. Flush audit store
flushAuditStoreAndWait()

// 5. Query by correlation_id ✅
Eventually(func() int {
    return countAuditEvents(eventType, correlationID)
}, "30s", "500ms").Should(Equal(1))

// 6. Fetch and validate
event, err := getLatestAuditEvent(eventType, correlationID)
Expect(err).ToNot(HaveOccurred())
Expect(event).ToNot(BeNil())
```

---

## 📋 **Summary Table**

| Pattern | Tests Using It | Pass Rate | Parallel Safe | Server-Side Filter |
|---------|----------------|-----------|---------------|-------------------|
| **correlation_id** | 85 tests | 100% | ✅ Yes | ✅ Yes |
| **namespace** | 1 test | 0% | ❌ No | ❌ No |

**Conclusion**: Use `correlation_id` for all audit queries in parallel tests.

---

## 🚀 **Action Items**

### **Immediate**
1. ✅ Pattern identified: Use `correlation_id` (not `namespace`)
2. ✅ Fix location identified: Line 251 in `severity_integration_test.go`
3. ⏭️ **Next**: Implement fix

### **Implementation Steps**
1. Add `correlationID := sp.Name` after line 224
2. Replace `queryAuditEvents(ctx, namespace, ...)` with `countAuditEvents(..., correlationID)`
3. Use `getLatestAuditEvent(..., correlationID)` to fetch event
4. Update assertions to use fetched event

### **Validation**
5. Run tests: Verify 100% pass rate (87/87)
6. Verify: No more `filtered=0` in logs
7. Document: Add pattern to test guidelines

---

**Date**: January 14, 2026
**Analyzed By**: AI Assistant
**Status**: ✅ PATTERN IDENTIFIED - Ready for implementation
