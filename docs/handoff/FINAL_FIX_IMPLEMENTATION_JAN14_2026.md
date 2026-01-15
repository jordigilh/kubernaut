# Final Fix Implementation - SignalProcessing Integration Tests - Jan 14, 2026

## 🎯 **Root Cause Summary**

**Failing Test**: `should emit 'classification.decision' audit event with both external and normalized severity`
**File**: `test/integration/signalprocessing/severity_integration_test.go:278`
**Root Cause**: Uses `namespace` for client-side filtering instead of `correlation_id` for server-side filtering

---

## 🔍 **What the Test Currently Does**

### **Step 1: Creates SignalProcessing with RemediationRequestRef**

```go
// Line 222
sp := createTestSignalProcessingCRD(namespace, "test-audit-event")
sp.Spec.Signal.Severity = "Sev2"
Expect(k8sClient.Create(ctx, sp)).To(Succeed())
```

**Helper function** (line 588):
```go
func createTestSignalProcessingCRD(namespace, name string) *signalprocessingv1alpha1.SignalProcessing {
    return &signalprocessingv1alpha1.SignalProcessing{
        Spec: signalprocessingv1alpha1.SignalProcessingSpec{
            RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
                Name:      "test-rr",  // ✅ HAS parent RR reference
                Namespace: namespace,
            },
            // ... signal data ...
        },
    }
}
```

**Key Finding**: Test **DOES** set `RemediationRequestRef.Name = "test-rr"`

---

### **Step 2: Audit Events ARE Created**

From `pkg/signalprocessing/audit/client.go`:
```go
func (c *AuditClient) RecordClassificationDecision(...) {
    if sp.Spec.RemediationRequestRef.Name == "" {
        c.log.V(1).Info("Skipping classification audit - no RemediationRequestRef")
        return  // ❌ Would skip if no parent
    }
    audit.SetCorrelationID(event, sp.Spec.RemediationRequestRef.Name)  // ✅ Sets "test-rr"
    // ... create audit event ...
}
```

**Result**: Audit events ARE created with `correlation_id = "test-rr"`

---

### **Step 3: Test Queries by Namespace (BROKEN)**

```go
// Line 251
events := queryAuditEvents(ctx, namespace, "signalprocessing.classification.decision")
```

**Problem**:
- Query returns 50 events from ALL 12 parallel processes
- Client-side filters by namespace
- Events from this test may not be in first 50 results
- Filter returns 0 events

---

## ✅ **The Fix**

### **Use correlation_id Instead of namespace**

**Change Required**: Query by `correlation_id = "test-rr"` (the RemediationRequestRef name)

```go
// BEFORE (line 251):
events := queryAuditEvents(ctx, namespace, "signalprocessing.classification.decision")
g.Expect(events).ToNot(BeEmpty(), "classification.decision audit event should exist")
latestEvent := events[len(events)-1]

// AFTER:
correlationID := sp.Spec.RemediationRequestRef.Name  // "test-rr"

Eventually(func(g Gomega) {
    count := countAuditEvents("signalprocessing.classification.decision", correlationID)
    g.Expect(count).To(Equal(1), "classification.decision audit event should exist")
}, "30s", "500ms").Should(Succeed())

event, err := getLatestAuditEvent("signalprocessing.classification.decision", correlationID)
g.Expect(err).ToNot(HaveOccurred())
g.Expect(event).ToNot(BeNil())

eventData, err := eventDataToMap(event.EventData)
g.Expect(err).ToNot(HaveOccurred())
```

---

## 📝 **Detailed Implementation**

### **File**: `test/integration/signalprocessing/severity_integration_test.go`

**Change #1: Add correlation_id** (after line 224):
```go
sp := createTestSignalProcessingCRD(namespace, "test-audit-event")
sp.Spec.Signal.Severity = "Sev2"
Expect(k8sClient.Create(ctx, sp)).To(Succeed())

// NEW: Get correlation ID from RemediationRequestRef
correlationID := sp.Spec.RemediationRequestRef.Name  // "test-rr"
```

**Change #2: Replace queryAuditEvents with countAuditEvents** (line 249-278):
```go
// BEFORE:
Eventually(func(g Gomega) {
    events := queryAuditEvents(ctx, namespace, "signalprocessing.classification.decision")
    g.Expect(events).ToNot(BeEmpty(), "classification.decision audit event should exist")

    latestEvent := events[len(events)-1]
    eventData, err := eventDataToMap(latestEvent.EventData)
    g.Expect(err).ToNot(HaveOccurred(), "Event data conversion should succeed")

    // ... all assertions on eventData ...
}, "30s", "2s").Should(Succeed())

// AFTER:
// First, wait for event to exist
Eventually(func() int {
    return countAuditEvents("signalprocessing.classification.decision", correlationID)
}, "30s", "500ms").Should(Equal(1),
    "classification.decision audit event should exist")

// Then fetch and validate
event, err := getLatestAuditEvent("signalprocessing.classification.decision", correlationID)
Expect(err).ToNot(HaveOccurred(), "Audit event query must succeed")
Expect(event).ToNot(BeNil(), "Event must exist")

eventData, err := eventDataToMap(event.EventData)
Expect(err).ToNot(HaveOccurred(), "Event data conversion should succeed")

// Validate external severity is captured
Expect(eventData).To(HaveKeyWithValue("external_severity", "Sev2"),
    "Audit event should capture original external severity")

// Validate normalized severity is captured
Expect(eventData).To(HaveKey("normalized_severity"),
    "Audit event should capture normalized severity")
Expect(eventData["normalized_severity"]).To(BeElementOf([]string{"critical", "warning", "info", "unknown"}),
    "Normalized severity should be standard value")

// Validate determination source for audit trail
Expect(eventData).To(HaveKeyWithValue("determination_source", "rego-policy"),
    "Audit event should record how severity was determined")

// ✅ DD-TESTING-001 Pattern 6: Validate top-level optional fields
Expect(event.DurationMs.IsSet()).To(BeTrue(),
    "Audit event should include performance metrics")
Expect(event.DurationMs.Value).To(BeNumerically(">", 0),
    "Performance metrics should be meaningful")
```

---

## 🔧 **Why This Fix Works**

### **Before Fix**

| Aspect | Implementation | Result |
|--------|----------------|--------|
| **Query** | `queryAuditEvents(ctx, namespace, eventType)` | Gets 50 events from ALL processes |
| **Filter** | Client-side by namespace | Filters to 0 (events not in first 50) |
| **Parallel Safe** | ❌ No | Cross-process contamination |
| **Pass Rate** | 0% | Always fails |

### **After Fix**

| Aspect | Implementation | Result |
|--------|----------------|--------|
| **Query** | `countAuditEvents(eventType, correlationID)` | Gets ONLY matching events |
| **Filter** | Server-side by correlation_id | Returns exactly 1 event |
| **Parallel Safe** | ✅ Yes | No cross-process issues |
| **Pass Rate** | 100% (expected) | Matches 85 other passing tests |

---

## 📊 **Expected Outcome**

### **Before Fix**
```
✅ 85 Passed
❌ 2 Failed
Pass Rate: 97.7%
```

### **After Fix**
```
✅ 87 Passed
❌ 0 Failed
Pass Rate: 100%
```

**Why Test #2 will pass**: It was only interrupted by Test #1's failure. Once Test #1 passes, Test #2 will run to completion and pass (it already uses the correct pattern).

---

## 🚀 **Implementation Steps**

### **Step 1: Update severity_integration_test.go**

1. Add `correlationID := sp.Spec.RemediationRequestRef.Name` after line 224
2. Replace `Eventually(func(g Gomega) { events := queryAuditEvents(...) ... })` block (lines 249-278)
3. Use pattern from `audit_integration_test.go` (lines 320-326)

### **Step 2: Run Tests**

```bash
make test-integration-signalprocessing
```

**Expected**: 87/87 specs pass (100%)

### **Step 3: Verify**

Check logs for:
- ✅ No more `DEBUG queryAuditEvents: after namespace filter, filtered=0`
- ✅ All tests pass
- ✅ No interruptions

---

## 📋 **Complete Code Change**

### **File**: `test/integration/signalprocessing/severity_integration_test.go`

**Lines 222-278** (replace entire Eventually block):

```go
// GIVEN: SignalProcessing with external severity
sp := createTestSignalProcessingCRD(namespace, "test-audit-event")
sp.Spec.Signal.Severity = "Sev2"
Expect(k8sClient.Create(ctx, sp)).To(Succeed())

// Get correlation ID for audit queries
correlationID := sp.Spec.RemediationRequestRef.Name  // "test-rr"

// WHEN: Controller determines severity (wait for Classifying phase completion)
Eventually(func(g Gomega) {
    var updated signalprocessingv1alpha1.SignalProcessing
    g.Expect(k8sClient.Get(ctx, types.NamespacedName{
        Name:      sp.Name,
        Namespace: sp.Namespace,
    }, &updated)).To(Succeed())

    // Wait for severity determination
    g.Expect(updated.Status.Severity).ToNot(BeEmpty(),
        "Status.Severity should be set after Classifying phase completes")
    // Also ensure we're past Enriching phase
    g.Expect(updated.Status.Phase).ToNot(Equal(signalprocessingv1alpha1.PhaseEnriching),
        "Controller should have moved past Enriching phase")
}, "60s", "2s").Should(Succeed())

// THEN: Audit event contains both severities
flushAuditStoreAndWait()

// Wait for audit event to appear
Eventually(func() int {
    return countAuditEvents("signalprocessing.classification.decision", correlationID)
}, "30s", "500ms").Should(Equal(1),
    "BR-SP-090: SignalProcessing MUST emit exactly 1 classification.decision event")

// Fetch and validate audit event
event, err := getLatestAuditEvent("signalprocessing.classification.decision", correlationID)
Expect(err).ToNot(HaveOccurred(), "Audit event query must succeed")
Expect(event).ToNot(BeNil(), "Event must exist")

eventData, err := eventDataToMap(event.EventData)
Expect(err).ToNot(HaveOccurred(), "Event data conversion should succeed")

// Validate external severity is captured
Expect(eventData).To(HaveKeyWithValue("external_severity", "Sev2"),
    "Audit event should capture original external severity")

// Validate normalized severity is captured
Expect(eventData).To(HaveKey("normalized_severity"),
    "Audit event should capture normalized severity")
Expect(eventData["normalized_severity"]).To(BeElementOf([]string{"critical", "warning", "info", "unknown"}),
    "Normalized severity should be standard value")

// Validate determination source for audit trail
Expect(eventData).To(HaveKeyWithValue("determination_source", "rego-policy"),
    "Audit event should record how severity was determined")

// ✅ DD-TESTING-001 Pattern 6: Validate top-level optional fields
Expect(event.DurationMs.IsSet()).To(BeTrue(),
    "Audit event should include performance metrics")
Expect(event.DurationMs.Value).To(BeNumerically(">", 0),
    "Performance metrics should be meaningful")

// BUSINESS OUTCOME VERIFIED:
// ✅ Compliance auditor can trace: "Sev2 → warning via Rego policy"
// ✅ Audit trail includes both external and normalized severity
// ✅ Performance metrics tracked for severity determination latency
```

---

## ✅ **Validation Checklist**

- [ ] Code updated in `severity_integration_test.go`
- [ ] Tests run: `make test-integration-signalprocessing`
- [ ] Result: 87/87 specs pass (100%)
- [ ] No `filtered=0` in logs
- [ ] No interruptions
- [ ] Commit changes

---

## 📚 **Related Documentation**

- **Pattern Analysis**: `PATTERN_COMPARISON_PASSING_VS_FAILING_TESTS_JAN14_2026.md`
- **Triage Results**: `FINAL_TRIAGE_RESULTS_JAN14_2026.md`
- **Complete Triage**: `COMPLETE_FAILURE_TRIAGE_NAMESPACE_ISSUE_JAN14_2026.md`

---

**Date**: January 14, 2026
**Prepared By**: AI Assistant
**Status**: ✅ READY FOR IMPLEMENTATION
