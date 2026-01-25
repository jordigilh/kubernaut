# CORRECTED Root Cause: Shared correlation_id Across Parallel Tests - Jan 14, 2026

## 🚨 **CRITICAL CORRECTION**

**Previous Analysis**: ❌ INCORRECT - Assumed correlation_id would be unique
**Actual Root Cause**: ✅ **Multiple parallel tests share the SAME correlation_id = "test-rr"**

---

## 🎯 **The REAL Problem**

### **What the Failing Test Does**

```go
// Line 222: Creates SignalProcessing with hardcoded RR name
sp := createTestSignalProcessingCRD(namespace, "test-audit-event")
```

**Helper function** (line 595):
```go
func createTestSignalProcessingCRD(namespace, name string) *signalprocessingv1alpha1.SignalProcessing {
    return &signalprocessingv1alpha1.SignalProcessing{
        Spec: signalprocessingv1alpha1.SignalProcessingSpec{
            RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
                Name:      "test-rr",  // ❌ HARDCODED - Same across ALL parallel tests!
                Namespace: namespace,
            },
            // ...
        },
    }
}
```

### **Evidence from Logs**

**Test creates**:
```
SignalProcessing name: "test-audit-event"
Namespace: "sp-severity-12-9538406f"
RemediationRequestRef.Name: "test-rr"  ← Used as correlation_id
```

**Audit events created**:
```json
{
  "correlation_id": "test-rr",  ← From RemediationRequestRef.Name
  "namespace": "sp-severity-12-9538406f",
  "event_type": "signalprocessing.classification.decision"
}
```

**But**: Logs show **1203 events** with `correlation_id":"test-rr"` across **12+ different namespaces**!

---

## 🔍 **Why correlation_id is NOT Unique**

### **Parallel Test Execution**

**12 parallel test processes** all use the same helper function:
```
Process 1:  namespace=sp-severity-1-xxxxx,  RR name="test-rr" → correlation_id="test-rr"
Process 2:  namespace=sp-severity-2-xxxxx,  RR name="test-rr" → correlation_id="test-rr"
...
Process 12: namespace=sp-severity-12-xxxxx, RR name="test-rr" → correlation_id="test-rr"
```

**Result**: **Hundreds of events** with `correlation_id="test-rr"` from different tests!

---

## 🆚 **Comparison: Failing Test vs Passing Tests**

### **❌ Failing Test Pattern**

```go
// Uses HARDCODED RR name
sp := createTestSignalProcessingCRD(namespace, "test-audit-event")
// RemediationRequestRef.Name = "test-rr" (hardcoded in helper)
// correlation_id = "test-rr" (shared across all parallel tests)
```

**Problem**: `correlation_id="test-rr"` is **NOT unique** - shared by 12+ parallel tests!

---

### **✅ Passing Tests Pattern**

```go
// Uses UNIQUE RR name per test
rrName := "audit-test-rr-01"  // Unique per test
rr := CreateTestRemediationRequest(rrName, ns, fingerprint, severity, targetResource)
Expect(k8sClient.Create(ctx, rr)).To(Succeed())

correlationID := rrName  // "audit-test-rr-01" (unique)

sp := CreateTestSignalProcessingWithParent("audit-test-sp-01", ns, rr, fingerprint, targetResource)
// RemediationRequestRef.Name = "audit-test-rr-01" (unique)
// correlation_id = "audit-test-rr-01" (unique per test)
```

**Why it works**: `correlation_id="audit-test-rr-01"` is **UNIQUE** to this test!

---

## 📊 **Evidence**

### **From Logs**

```bash
# Count events with correlation_id="test-rr"
grep -c "correlation_id\":\"test-rr\"" /tmp/sp-integration-test-triage-verification.log
# Result: 1203 events!

# Count different namespaces using "test-rr"
grep "RemediationRequestRef.Name='test-rr'" | grep -o "namespace=sp-severity-[0-9]+" | sort -u | wc -l
# Result: 12+ namespaces (all parallel test processes)
```

### **Why Query Fails**

```go
// Test queries by correlation_id="test-rr"
count := countAuditEvents("signalprocessing.classification.decision", "test-rr")
// Returns: Hundreds of events from ALL 12 parallel processes!

// But test uses namespace filter:
events := queryAuditEvents(ctx, namespace, eventType)
// Tries to filter 50 events by namespace "sp-severity-12-xxxxx"
// But those 50 events are from OTHER processes (sp-severity-1, sp-severity-2, etc.)
// Result: filtered=0
```

---

## ✅ **The CORRECT Fix**

### **Option A: Make RR Name Unique** (Recommended)

**Change the helper function to use unique RR names**:

```go
// BEFORE (line 595):
func createTestSignalProcessingCRD(namespace, name string) *signalprocessingv1alpha1.SignalProcessing {
    return &signalprocessingv1alpha1.SignalProcessing{
        Spec: signalprocessingv1alpha1.SignalProcessingSpec{
            RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
                Name:      "test-rr",  // ❌ Hardcoded - shared across tests
                Namespace: namespace,
            },
            // ...
        },
    }
}

// AFTER:
func createTestSignalProcessingCRD(namespace, name string) *signalprocessingv1alpha1.SignalProcessing {
    // Generate unique RR name based on SP name
    rrName := name + "-rr"  // e.g., "test-audit-event-rr"

    return &signalprocessingv1alpha1.SignalProcessing{
        Spec: signalprocessingv1alpha1.SignalProcessingSpec{
            RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
                Name:      rrName,  // ✅ Unique per test
                Namespace: namespace,
            },
            // ...
        },
    }
}
```

**Then in the test**:
```go
sp := createTestSignalProcessingCRD(namespace, "test-audit-event")
// RemediationRequestRef.Name = "test-audit-event-rr" (unique)

correlationID := sp.Spec.RemediationRequestRef.Name  // "test-audit-event-rr"

// Query by unique correlation_id
Eventually(func() int {
    return countAuditEvents("signalprocessing.classification.decision", correlationID)
}, "30s", "500ms").Should(Equal(1))
```

---

### **Option B: Create Actual RemediationRequest** (Like Passing Tests)

**Match the pattern used by 85 passing tests**:

```go
// Create actual RemediationRequest with unique name
rrName := "test-audit-event-rr"
targetResource := signalprocessingv1alpha1.ResourceIdentifier{
    Kind:      "Pod",
    Name:      "test-pod",
    Namespace: namespace,
}
rr := CreateTestRemediationRequest(rrName, namespace, fingerprint, "critical", targetResource)
Expect(k8sClient.Create(ctx, rr)).To(Succeed())

correlationID := rrName  // "test-audit-event-rr" (unique)

// Create SignalProcessing with parent RR
sp := CreateTestSignalProcessingWithParent("test-audit-event", namespace, rr, fingerprint, targetResource)
sp.Spec.Signal.Severity = "Sev2"
Expect(k8sClient.Create(ctx, sp)).To(Succeed())

// Query by unique correlation_id
Eventually(func() int {
    return countAuditEvents("signalprocessing.classification.decision", correlationID)
}, "30s", "500ms").Should(Equal(1))
```

---

## 📋 **Summary of Findings**

| Aspect | Previous Analysis | Corrected Analysis |
|--------|-------------------|-------------------|
| **correlation_id source** | ✅ Correct (RR name) | ✅ Confirmed |
| **Uniqueness** | ❌ Assumed unique | ✅ NOT unique - shared! |
| **Root cause** | ❌ Namespace filter | ✅ Shared correlation_id |
| **Fix** | ❌ Use correlation_id | ✅ Make correlation_id unique |

### **Why Previous Analysis Was Wrong**

1. ❌ Assumed `createTestSignalProcessingCRD` would create unique RR names
2. ❌ Didn't check if "test-rr" was hardcoded
3. ❌ Didn't verify uniqueness across parallel tests
4. ✅ Correctly identified namespace filter issue (symptom)
5. ❌ Missed the deeper cause (shared correlation_id)

---

## 🔧 **Recommended Implementation**

### **Step 1: Fix Helper Function**

**File**: `test/integration/signalprocessing/severity_integration_test.go`

**Line 595** - Update `createTestSignalProcessingCRD`:
```go
func createTestSignalProcessingCRD(namespace, name string) *signalprocessingv1alpha1.SignalProcessing {
    // Generate unique RR name to avoid parallel test collisions
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

### **Step 2: Update Test to Use correlation_id**

**Line 222-278** - Update test:
```go
sp := createTestSignalProcessingCRD(namespace, "test-audit-event")
sp.Spec.Signal.Severity = "Sev2"
Expect(k8sClient.Create(ctx, sp)).To(Succeed())

// Get unique correlation ID
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

// Flush and query
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

## ✅ **Expected Outcome**

**Before Fix**:
- `correlation_id = "test-rr"` (shared by 12+ tests)
- Query returns hundreds of events
- Namespace filter returns 0
- Test fails

**After Fix**:
- `correlation_id = "test-audit-event-rr"` (unique)
- Query returns exactly 1 event
- No namespace filter needed
- Test passes

---

## 📊 **Impact**

| Metric | Before | After |
|--------|--------|-------|
| **correlation_id uniqueness** | ❌ Shared | ✅ Unique |
| **Events per correlation_id** | 1203 | 1 |
| **Query efficiency** | Slow (filter 50) | Fast (return 1) |
| **Pass rate** | 97.7% | 100% (expected) |

---

**Date**: January 14, 2026
**Corrected By**: AI Assistant (after user feedback)
**Status**: ✅ ROOT CAUSE CORRECTED - Ready for implementation
