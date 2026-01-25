# SignalProcessing Correlation ID Test Inconsistency - Action Required

**Date**: January 14, 2026
**To**: SignalProcessing Team
**Priority**: 🚨 **P0 - Blocking Integration Tests**
**Status**: ❌ **FAILING** - 97.7% pass rate (1 test failure due to this issue)

---

## 🚨 **Executive Summary**

**Problem**: SignalProcessing integration tests use a **hardcoded RemediationRequest name** (`"test-rr"`) across all parallel test processes, causing correlation_id collisions and test failures.

**Impact**:
- ❌ 1 integration test fails consistently (severity audit event validation)
- ❌ Cannot reliably query audit events by correlation_id in parallel execution
- ❌ Violates DD-AUDIT-CORRELATION-001 uniqueness requirements

**Fix Required**: Make RemediationRequest names unique per test (e.g., `name + "-rr"`)

**Reference Standard**: [DD-AUDIT-CORRELATION-001](../architecture/decisions/DD-AUDIT-CORRELATION-001-workflowexecution-correlation-id.md)

---

## 📋 **Background: What is DD-AUDIT-CORRELATION-001?**

[DD-AUDIT-CORRELATION-001](../architecture/decisions/DD-AUDIT-CORRELATION-001-workflowexecution-correlation-id.md) is the **authoritative standard** for correlation ID usage across Kubernaut services.

### **Key Principles from DD-AUDIT-CORRELATION-001**

1. **Parent RemediationRequest Name is Root Correlation ID**:
   - Gateway generates RemediationRequest with **unique name**
   - RR name is the **root correlation ID** for entire remediation flow
   - All child CRDs (AIAnalysis, WorkflowExecution, **SignalProcessing**) reference parent RR

2. **Use `RemediationRequestRef.Name` as correlation_id**:
   - ✅ **Spec field is authoritative** (required, immutable)
   - ✅ **Guaranteed to exist** (CRD validation enforces)
   - ✅ **Maintains audit trail continuity**

3. **Correlation ID Must Be Unique**:
   - Each RemediationRequest must have a **unique name**
   - correlation_id enables linking all audit events for a single remediation flow
   - Non-unique correlation_ids break audit trail reconstruction

---

## 🔍 **Current SignalProcessing Implementation**

### **SignalProcessing Correctly Follows DD-AUDIT-CORRELATION-001** ✅

**Code**: `pkg/signalprocessing/audit/client.go:289`
```go
// Graceful degradation: skip audit if no RemediationRequestRef (test edge cases)
if sp.Spec.RemediationRequestRef.Name == "" {
    c.log.V(1).Info("Skipping classification audit - no RemediationRequestRef")
    return
}
audit.SetCorrelationID(event, sp.Spec.RemediationRequestRef.Name)  // ✅ CORRECT
```

**SignalProcessing production code is CORRECT** - it uses `RemediationRequestRef.Name` as correlation_id per DD-AUDIT-CORRELATION-001.

---

## 🚨 **The Problem: Integration Test Helper**

### **Test Helper Uses Hardcoded RR Name** ❌

**File**: `test/integration/signalprocessing/severity_integration_test.go`
**Line**: 595

```go
func createTestSignalProcessingCRD(namespace, name string) *signalprocessingv1alpha1.SignalProcessing {
    return &signalprocessingv1alpha1.SignalProcessing{
        ObjectMeta: metav1.ObjectMeta{
            Name:      name,
            Namespace: namespace,
        },
        Spec: signalprocessingv1alpha1.SignalProcessingSpec{
            RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
                Name:      "test-rr",  // ❌ HARDCODED - Same across ALL parallel tests!
                Namespace: namespace,
            },
            Signal: signalprocessingv1alpha1.SignalData{
                // ... signal data ...
            },
        },
    }
}
```

### **Impact of Hardcoded RR Name**

**Parallel Test Execution** (12 processes):
```
Process 1:  namespace=sp-severity-1-xxxxx,  correlation_id="test-rr"
Process 2:  namespace=sp-severity-2-xxxxx,  correlation_id="test-rr"
Process 3:  namespace=sp-severity-3-xxxxx,  correlation_id="test-rr"
...
Process 12: namespace=sp-severity-12-xxxxx, correlation_id="test-rr"
```

**Result**: **215+ audit events** with the same `correlation_id="test-rr"` from different test processes!

---

## 📊 **Evidence from Must-Gather Logs**

### **Audit Event Creation (Successful)** ✅

```json
{
  "event_type": "signalprocessing.classification.decision",
  "correlation_id": "test-rr",
  "namespace": "sp-severity-12-9538406f",
  "event_data": {...}
}
```

**Audit events ARE created correctly** with `correlation_id="test-rr"`.

### **Query Failure (Parallel Test Collision)** ❌

**Test queries**:
```go
// Query by correlation_id
params := ogenclient.QueryAuditEventsParams{
    EventType:     ogenclient.NewOptString("signalprocessing.classification.decision"),
    CorrelationID: ogenclient.NewOptString("test-rr"),  // Returns 215+ events!
}
resp, _ := dsClient.QueryAuditEvents(ctx, params)
// resp.Data contains events from ALL 12 parallel processes

// Test tries to filter by namespace
filteredEvents := []Event{}
for _, event := range resp.Data {
    if event.Namespace == "sp-severity-12-9538406f" {
        filteredEvents = append(filteredEvents, event)
    }
}
// Result: 0 events (test's events not in first 50 returned)
```

**Why it fails**:
1. Query returns 50 events (default limit) with `correlation_id="test-rr"`
2. Those 50 events are from **other parallel test processes**
3. Client-side namespace filter returns 0 results
4. Test fails: "Expected 1 audit event, got 0"

---

## ✅ **The Fix: Make RR Name Unique Per Test**

### **Required Change**

**File**: `test/integration/signalprocessing/severity_integration_test.go`
**Line**: 595

```go
// BEFORE (INCORRECT):
func createTestSignalProcessingCRD(namespace, name string) *signalprocessingv1alpha1.SignalProcessing {
    return &signalprocessingv1alpha1.SignalProcessing{
        Spec: signalprocessingv1alpha1.SignalProcessingSpec{
            RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
                Name:      "test-rr",  // ❌ Hardcoded - shared across tests
                Namespace: namespace,
            },
        },
    }
}

// AFTER (CORRECT):
func createTestSignalProcessingCRD(namespace, name string) *signalprocessingv1alpha1.SignalProcessing {
    // Generate unique RR name to avoid parallel test collisions
    // Per DD-AUDIT-CORRELATION-001: RR name must be unique per remediation flow
    rrName := name + "-rr"  // e.g., "test-audit-event-rr"

    return &signalprocessingv1alpha1.SignalProcessing{
        ObjectMeta: metav1.ObjectMeta{
            Name:      name,
            Namespace: namespace,
        },
        Spec: signalprocessingv1alpha1.SignalProcessingSpec{
            RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
                Name:      rrName,  // ✅ Unique per test
                Namespace: namespace,
            },
            Signal: signalprocessingv1alpha1.SignalData{
                // ... rest unchanged ...
            },
        },
    }
}
```

### **Update Test to Query by Unique correlation_id**

**File**: `test/integration/signalprocessing/severity_integration_test.go`
**Line**: 222-278 (failing test)

```go
// BEFORE (Client-side namespace filtering):
sp := createTestSignalProcessingCRD(namespace, "test-audit-event")
// ... create SP ...

flushAuditStoreAndWait()

// Query without correlation_id filter (returns all "test-rr" events)
events := queryAuditEvents(ctx, namespace, "signalprocessing.classification.decision")
// Filters client-side by namespace (fails in parallel execution)

// AFTER (Server-side correlation_id filtering):
sp := createTestSignalProcessingCRD(namespace, "test-audit-event")
sp.Spec.Signal.Severity = "Sev2"
Expect(k8sClient.Create(ctx, sp)).To(Succeed())

// Get unique correlation ID from SP
correlationID := sp.Spec.RemediationRequestRef.Name  // "test-audit-event-rr"

// Wait for processing
Eventually(func(g Gomega) {
    var updated signalprocessingv1alpha1.SignalProcessing
    g.Expect(k8sClient.Get(ctx, types.NamespacedName{
        Name:      sp.Name,
        Namespace: sp.Namespace,
    }, &updated)).To(Succeed())
    g.Expect(updated.Status.Severity).ToNot(BeEmpty())
}, "60s", "2s").Should(Succeed())

// Flush and query by unique correlation_id
flushAuditStoreAndWait()

Eventually(func() int {
    return countAuditEvents("signalprocessing.classification.decision", correlationID)
}, "30s", "500ms").Should(Equal(1))

event, err := getLatestAuditEvent("signalprocessing.classification.decision", correlationID)
Expect(err).ToNot(HaveOccurred())
Expect(event).ToNot(BeNil())

// ... rest of assertions ...
```

---

## 📋 **Implementation Checklist**

### **Phase 1: Fix Test Helper** (Required)

- [ ] Update `createTestSignalProcessingCRD()` to generate unique RR names
- [ ] Add comment referencing DD-AUDIT-CORRELATION-001
- [ ] Verify helper function signature doesn't change (no breaking changes)

### **Phase 2: Update Failing Test** (Required)

- [ ] Extract `correlationID` from `sp.Spec.RemediationRequestRef.Name`
- [ ] Replace `queryAuditEvents(ctx, namespace, eventType)` with `countAuditEvents(eventType, correlationID)`
- [ ] Update assertions to use `correlationID` instead of namespace filtering

### **Phase 3: Verify Other Tests** (Recommended)

- [ ] Search for other uses of `createTestSignalProcessingCRD()`
- [ ] Verify all tests will work with unique RR names
- [ ] Check if any tests explicitly expect `"test-rr"` (update if found)

### **Phase 4: Run Integration Tests** (Validation)

- [ ] Run `make test-integration-signalprocessing`
- [ ] Verify 100% pass rate (currently 97.7% with 1 failure)
- [ ] Check must-gather logs show unique correlation_ids

---

## 🎯 **Expected Outcome**

### **Before Fix**

| Metric | Value |
|--------|-------|
| **Hardcoded RR Name** | `"test-rr"` (shared) |
| **Events per correlation_id** | 215+ (all parallel tests) |
| **Query Efficiency** | Slow (returns 50, filters to 0) |
| **Pass Rate** | 97.7% (1 failure) |

### **After Fix**

| Metric | Value |
|--------|-------|
| **Unique RR Name** | `"test-audit-event-rr"` (per test) |
| **Events per correlation_id** | 1 (single test) |
| **Query Efficiency** | Fast (returns 1) |
| **Pass Rate** | 100% (expected) |

---

## 📚 **Reference Documents**

### **Primary Reference**

- **[DD-AUDIT-CORRELATION-001](../architecture/decisions/DD-AUDIT-CORRELATION-001-workflowexecution-correlation-id.md)**: Authoritative standard for correlation ID usage
  - Section: "Parent RR Name is Root Correlation ID"
  - Section: "Use `RemediationRequestRef.Name` as correlation_id"

### **Supporting Documentation**

- **[FINAL_ROOT_CAUSE_CORRELATION_ID_SOURCE_JAN14_2026.md](./FINAL_ROOT_CAUSE_CORRELATION_ID_SOURCE_JAN14_2026.md)**: Complete triage and analysis
- **[PATTERN_COMPARISON_PASSING_VS_FAILING_TESTS_JAN14_2026.md](./PATTERN_COMPARISON_PASSING_VS_FAILING_TESTS_JAN14_2026.md)**: Pattern comparison (85 passing tests use unique correlation_ids)
- **[CORRECTED_ROOT_CAUSE_SHARED_CORRELATION_ID_JAN14_2026.md](./CORRECTED_ROOT_CAUSE_SHARED_CORRELATION_ID_JAN14_2026.md)**: Initial analysis before full triage

---

## 🔍 **Pattern Validation: Other Services**

### **How Other Services Handle This**

**WorkflowExecution Integration Tests** (✅ CORRECT):
```go
// test/integration/workflowexecution/audit_integration_test.go
rrName := "audit-test-rr-01"  // ✅ Unique per test
rr := CreateTestRemediationRequest(rrName, ns, fingerprint, severity, targetResource)
Expect(k8sClient.Create(ctx, rr)).To(Succeed())

correlationID := rrName  // "audit-test-rr-01"

wfe := CreateTestWorkflowExecutionWithParent("audit-test-wfe-01", ns, rr, ...)
Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

// Query by unique correlation_id
count := countAuditEvents("workflow.selection.completed", correlationID)
Expect(count).To(Equal(1))
```

**Why it works**:
- ✅ RR name is **unique per test** (`"audit-test-rr-01"`)
- ✅ correlation_id is **unique** across parallel execution
- ✅ Query returns **exactly 1 event**
- ✅ **85 tests pass consistently**

**SignalProcessing Should Follow Same Pattern** ✅

---

## 💬 **FAQ**

### **Q: Why not use RemediationRequest UID instead of Name?**

**A**: DD-AUDIT-CORRELATION-001 establishes `RemediationRequestRef.Name` as the standard for 4 out of 5 services:
- SignalProcessing
- WorkflowExecution
- RemediationApprovalRequest
- Notification

Only AIAnalysis uses RR.UID (via `RemediationID` field), and documentation notes this as **INCONSISTENT** and should migrate to the standard pattern.

**Benefits of RR.Name**:
- ✅ Human-readable audit logs (`"test-audit-event-rr"` vs `"a1b2c3d4-..."`)
- ✅ Consistent with majority pattern
- ✅ Follows DD-AUDIT-CORRELATION-001 standard
- ✅ No schema changes needed

### **Q: Will this break existing tests?**

**A**: No. The change makes RR names **more unique**, not less. Tests that work with hardcoded `"test-rr"` will work even better with unique names like `"test-audit-event-rr"`.

**Potential Impact**:
- Tests that explicitly assert `correlation_id == "test-rr"` will need updating (unlikely)
- Tests using `createTestSignalProcessingCRD()` will automatically get unique RR names (no changes needed)

### **Q: Why did this work before parallel execution?**

**A**: With serial execution, only 1 test runs at a time:
- Test creates SP with `correlation_id="test-rr"`
- Test queries and gets its own event
- Test completes
- Next test starts (previous events cleared)

With **parallel execution** (12 processes):
- All 12 tests create SPs with `correlation_id="test-rr"` **simultaneously**
- Query returns 215+ events from all processes
- Client-side namespace filter fails (events from other processes)

### **Q: Should we also create actual RemediationRequest CRDs in tests?**

**A**: Not required for this fix, but it's the **better pattern** long-term:

**Current Approach** (✅ Sufficient for fix):
```go
rrName := name + "-rr"  // Generate unique RR name
sp := createTestSignalProcessingCRD(namespace, name)
// SP references non-existent RR (test-only scenario)
```

**Better Long-term Approach** (✅ Matches other services):
```go
// Create actual RemediationRequest CRD
rr := CreateTestRemediationRequest("test-audit-event-rr", namespace, ...)
Expect(k8sClient.Create(ctx, rr)).To(Succeed())

correlationID := rr.Name  // "test-audit-event-rr"

// Create SP with parent RR reference
sp := CreateTestSignalProcessingWithParent("test-audit-event", namespace, rr, ...)
Expect(k8sClient.Create(ctx, sp)).To(Succeed())
```

**Recommendation**: Fix the immediate issue first (unique RR names), then optionally refactor to match WorkflowExecution pattern.

---

## ✅ **Success Criteria**

### **Immediate Success** (Fix Applied)

- ✅ Integration test pass rate: **100%** (up from 97.7%)
- ✅ All tests use **unique correlation_ids**
- ✅ No client-side namespace filtering in audit queries
- ✅ Must-gather logs show unique `correlation_id` values per test

### **Long-term Success** (Pattern Consistency)

- ✅ SignalProcessing tests follow **same pattern** as WorkflowExecution
- ✅ All tests reference **DD-AUDIT-CORRELATION-001** in comments
- ✅ No hardcoded RR names in test helpers
- ✅ Audit query patterns documented and consistent

---

## 📞 **Support & Questions**

**For questions about this fix**:
- Review: [DD-AUDIT-CORRELATION-001](../architecture/decisions/DD-AUDIT-CORRELATION-001-workflowexecution-correlation-id.md)
- Reference: Must-gather logs in `/tmp/must-gather/signalprocessing-*`
- Compare: WorkflowExecution test patterns in `test/integration/workflowexecution/audit_integration_test.go`

**For implementation assistance**:
- See: [FINAL_FIX_IMPLEMENTATION_JAN14_2026.md](./FINAL_FIX_IMPLEMENTATION_JAN14_2026.md)
- Review: Passing test examples in `test/integration/signalprocessing/audit_integration_test.go`

---

**Document Created**: January 14, 2026
**Author**: AI Assistant (Post-Triage Analysis)
**Status**: ✅ **READY FOR IMPLEMENTATION**
**Priority**: 🚨 **P0 - Blocking Integration Tests**
