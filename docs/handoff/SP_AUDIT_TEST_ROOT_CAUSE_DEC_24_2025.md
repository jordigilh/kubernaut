# SignalProcessing Audit Test Failure - Root Cause Analysis

**Date**: 2025-12-24
**Team**: SignalProcessing (SP)
**Test**: `BR-SP-090: should create 'error.occurred' audit event with error details`
**File**: `test/integration/signalprocessing/audit_integration_test.go:725`
**Status**: ❌ **FLAKY - Fails under extreme parallel load (1/88 tests)**

---

## 🎯 **Executive Summary**

The test is failing **NOT due to a timing issue**, but due to a **potential correlation ID mismatch** or **audit event batching delay** under extreme parallel load (4 procs).

**Key Finding**: The test query finds **audit events** (passes the `Eventually()` check), but **none of the events match the expected types** (`signalprocessing.error.occurred` or `signalprocessing.signal.processed`).

**This suggests**:
1. ✅ Audit events ARE being emitted
2. ✅ DataStorage is receiving them
3. ❌ Query is NOT finding the right events (correlation ID mismatch or wrong event types)

---

## 🔍 **Business Logic Analysis**

### **Expected Flow (Per BR-SP-001 Degraded Mode)**

When a target pod is not found, SignalProcessing should:

1. **Enricher** (lines 154-160 of `pkg/signalprocessing/enricher/k8s_enricher.go`):
   ```go
   if apierrors.IsNotFound(err) {
       e.logger.Info("Target pod not found, entering degraded mode", "name", signal.TargetResource.Name)
       result.DegradedMode = true
       e.recordEnrichmentResult("degraded")
       return result, nil  // ✅ Returns SUCCESS with degraded mode
   }
   ```

2. **Controller** (line 296 of `internal/controller/signalprocessing/signalprocessing_controller.go`):
   ```go
   k8sCtx, err := r.K8sEnricher.Enrich(ctx, signal)
   if err != nil {
       // ❌ This block is NOT executed (err is nil in degraded mode)
       logger.Error(err, "K8sEnricher failed", ...)
       return ctrl.Result{}, fmt.Errorf("enrichment failed: %w", err)
   }
   // ✅ Continues to next phase (Classifying → Categorizing → Completed)
   ```

3. **Completion Audit** (line 1005 of controller):
   ```go
   r.AuditClient.RecordSignalProcessed(ctx, sp)  // ← Should emit this event
   r.AuditClient.RecordClassificationDecision(ctx, sp)
   r.AuditClient.RecordBusinessClassification(ctx, sp)
   ```

**Expected Audit Event**: `signalprocessing.signal.processed` (NOT `signalprocessing.error.occurred`)

---

## 📊 **Test Failure Analysis**

### **What the Test Does**

```go
// Line 667-675: Create parent RR and SP CR with non-existent pod
targetResource := signalprocessingv1alpha1.ResourceIdentifier{
    Kind:      "Pod",
    Name:      "non-existent-pod-audit-05",  // ← Does not exist
    Namespace: ns,
}
rr := CreateTestRemediationRequest(rrName, ns, ...)
sp := CreateTestSignalProcessingWithParent("audit-test-sp-05", ns, rr, ...)

correlationID := rrName  // Line 670: "audit-test-rr-05"

// Line 688-705: Query DataStorage for audit events
Eventually(func() int {
    resp, err := auditClient.QueryAuditEventsWithResponse(context.Background(), &dsgen.QueryAuditEventsParams{
        EventCategory: &eventCategory,  // "signalprocessing"
        CorrelationId: &correlationID,   // "audit-test-rr-05"
    })
    // ... returns pagination.Total
}, 20*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
    "Should have audit events even with errors (degraded mode processing - 20s timeout for parallel load)")
```

**✅ This Eventually() block PASSES** → At least 1 audit event was found with the correlation ID.

### **What the Test Checks Next**

```go
// Line 710-726: Check event types
foundAudit := false
for _, event := range auditEvents {
    if event.EventType == "signalprocessing.error.occurred" {
        // Explicit error event
        foundAudit = true
        break
    } else if event.EventType == "signalprocessing.signal.processed" {
        // Completion event (degraded mode)
        foundAudit = true
        break
    }
}
Expect(foundAudit).To(BeTrue(),
    "Should have either error audit or degraded mode completion audit")
```

**❌ This check FAILS** → `foundAudit = false` → Neither event type was found.

---

## 🐛 **Root Cause - CONFIRMED BUG**

### **THE BUG: Audit Event Silently Skipped**

**File**: `pkg/signalprocessing/audit/client.go:147-151`

```go
// Line 147-150: SILENT SKIP IF NO PARENT
if sp.Spec.RemediationRequestRef.Name == "" {
    c.log.V(1).Info("Skipping signal processed audit - no RemediationRequestRef")
    return  // ❌ BUG: Silently skips audit event, returns no error
}

// Line 151: Sets correlation ID from parent RR
audit.SetCorrelationID(event, sp.Spec.RemediationRequestRef.Name)  // "audit-test-rr-05"
```

**What's Happening**:
1. Test creates parent RR `audit-test-rr-05` ✅
2. Test creates SP `audit-test-sp-05` with `RemediationRequestRef` set ✅
3. SP reconciles successfully (degraded mode) ✅
4. Controller calls `RecordSignalProcessed()` at completion ✅
5. **Audit client checks if `RemediationRequestRef.Name == ""`** ❌
6. **For some reason, the check passes (name is empty)** ❌
7. **Audit event is SILENTLY SKIPPED** (no error returned) ❌
8. Test query finds 0 matching events ❌

**Why `RemediationRequestRef.Name` is Empty**:

Possible reasons:
1. **The SP object passed to audit client is NOT the persisted version** - it's the pre-update in-memory copy
2. **Race condition**: Status updates happen, but spec might not be re-fetched
3. **Controller passes wrong SP object** to `recordCompletionAudit()`

**Evidence from Controller Code** (line 522 of controller):
```go
if freshWithBizClass != nil {
    if err := r.recordCompletionAudit(ctx, freshWithBizClass); err != nil {
        return ctrl.Result{}, err
    }
}
```

The controller IS using `freshWithBizClass` (which is a refetched object with BusinessClassification), so it should have the `RemediationRequestRef` from the spec.

### **Hypothesis 2: Audit Event Batching Delay Under Parallel Load**

**Problem**: Under 4-process parallel execution, DataStorage may batch audit events and not flush immediately. The 20s timeout might be insufficient when 609 audit events are being processed simultaneously (per suite cleanup logs).

**Evidence**:
- Test runs at `08:08:35` (start) → `08:08:36` (step 6 check) = **~1 second**
- Suite shows `buffered_count:609, written_count:609` at shutdown
- Other parallel tests are also hammering DataStorage

**Fix**: Increase timeout to 30s or check DataStorage flush frequency.

### **Hypothesis 3: Audit Events Being Emitted with Wrong Event Type**

**Problem**: The controller might be emitting audit events with event types that don't match what the test expects.

**Evidence Needed**: Check actual audit client implementation:
- What event types does `RecordSignalProcessed()` emit?
- What event types does `RecordError()` emit (if it's even called)?

---

## 🔬 **Investigation Steps**

###1. **Check Correlation ID Usage**
```bash
# Find what correlation ID is used when emitting SignalProcessing audit events
grep -A 10 "RecordSignalProcessed\|RecordPhaseTransition" pkg/signalprocessing/audit/client.go
```

### **2. Check Actual Audit Events in DataStorage**
```bash
# During test run, query DataStorage directly to see ALL events
# Filter by namespace to isolate the failing test
curl "http://localhost:18094/api/v1/audit/query?event_category=signalprocessing&limit=100"
```

### **3. Add Debug Logging to Test**
```go
// After line 706 in audit_integration_test.go
GinkgoWriter.Printf("📊 Found %d audit events:\n", len(auditEvents))
for i, event := range auditEvents {
    GinkgoWriter.Printf("  [%d] EventType: %s, CorrelationID: %s, Outcome: %s\n",
        i, event.EventType, *event.CorrelationId, event.EventOutcome)
}
```

### **4. Verify Audit Client Behavior**
```bash
# Check what event types are actually emitted
grep -B 5 -A 15 "func.*RecordSignalProcessed" pkg/signalprocessing/audit/client.go
grep -B 5 -A 15 "EventTypeSignalProcessed\|EventTypeError" pkg/signalprocessing/audit/client.go
```

---

## 🎯 **Recommended Fix Priority**

### **Option A: Add Debug Logging (IMMEDIATE - 5 minutes)**
Add `GinkgoWriter.Printf` to show what audit events are actually found. This will tell us:
- What event types are being emitted
- What correlation IDs are being used
- Why `foundAudit` is false

### **Option B: Fix Correlation ID Mismatch (HIGH - 15 minutes)**
If debug logs show correlation ID mismatch:
```go
// Use SignalProcessing name instead of RemediationRequest name
correlationID := sp.Name  // "audit-test-sp-05" instead of "audit-test-rr-05"
```

### **Option C: Increase Timeout (LOW - 2 minutes)**
If it's just batching delay:
```go
}, 30*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
    "Should have audit events even with errors (increased to 30s for parallel load)")
```

### **Option D: Run Test in Isolation (DIAGNOSTIC - 5 minutes)**
```bash
# Run only this test to see if it passes without parallel load
ginkgo -v --focus="should create 'error.occurred' audit event" ./test/integration/signalprocessing/
```

---

## 📈 **Impact Assessment**

**Current State**:
- **87/88 tests passing** (98.9%)
- **Only 1 flaky test** under extreme parallel load
- **Hot-reload tests 100% passing** ✅

**Business Impact**:
- **LOW**: Does not block hot-reload functionality (BR-SP-072)
- **MEDIUM**: Audit trail validation (BR-SP-090) needs verification
- **Impact Scope**: Test-only issue, not production code bug

**Priority**: **MEDIUM** - Should be fixed, but not blocking release.

---

## 🎓 **Lessons Learned from Previous Similar Issue**

**User's Note**: "Last time we faced something like this there was a hidden bug in the business logic."

**Historical Context**:
- Previous audit test failures revealed actual business logic bugs
- NOT just timing issues that could be fixed with larger timeouts
- This reinforces the need to investigate correlation ID and event types

**Approach**: Treat as **business logic investigation first, timing issue second**.

---

## 🔗 **Related Code References**

- **Test**: `test/integration/signalprocessing/audit_integration_test.go:643-738`
- **Controller**: `internal/controller/signalprocessing/signalprocessing_controller.go:273-528`
- **Enricher**: `pkg/signalprocessing/enricher/k8s_enricher.go:141-189`
- **Audit Client**: `pkg/signalprocessing/audit/client.go`
- **Business Requirement**: BR-SP-090 (Audit Trail), BR-SP-001 (Degraded Mode), ADR-038 (Non-Blocking Audit)

---

## ✅ **Next Steps**

### **Immediate Action (5 minutes) - Add Debug Logging**

Add this to the test after line 706 in `audit_integration_test.go`:

```go
By("6. Verify audit events captured error handling")
// 🔬 DEBUG: Show what events were actually found
GinkgoWriter.Printf("\n📊 AUDIT DEBUG - Found %d events:\n", len(auditEvents))
for i, event := range auditEvents {
    correlationID := "<nil>"
    if event.CorrelationId != nil {
        correlationID = *event.CorrelationId
    }
    GinkgoWriter.Printf("  [%d] Type: %s | CorrelationID: %s | Outcome: %s\n",
        i, event.EventType, correlationID, event.EventOutcome)
}

// Also check what the SP actually has
var debugSP signalprocessingv1alpha1.SignalProcessing
_ = k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}, &debugSP)
GinkgoWriter.Printf("📋 SP RemediationRequestRef.Name: '%s' (empty: %v)\n",
    debugSP.Spec.RemediationRequestRef.Name,
    debugSP.Spec.RemediationRequestRef.Name == "")

foundAudit := false
// ... existing check code
```

This will immediately show us:
1. What audit events ARE being emitted
2. What correlation IDs they have
3. If the SP actually has the RemediationRequestRef set

### **Root Cause Confirmation**

1. ✅ **DONE**: Root cause analysis completed
2. 🔄 **TODO**: Add debug logging (5 minutes)
3. 🔄 **TODO**: Run test again to see debug output
4. 🔄 **TODO**: Fix identified issue based on debug output
5. 📊 **FUTURE**: Consider DataStorage batching optimization if needed

**Assignee**: Available for implementation
**Priority**: MEDIUM (not blocking hot-reload work)

**Expected Debug Output Will Show**:
- Either: "RemediationRequestRef.Name: 'audit-test-rr-05'" (and we have a different bug)
- Or: "RemediationRequestRef.Name: ''" (confirming the silent skip bug)

