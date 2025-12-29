# TRIAGE: Storm Detection Test Failure - Detailed Analysis

**Date**: 2025-12-13
**Test**: BR-GATEWAY-013: Storm Detection
**Status**: 🔍 **ROOT CAUSE IDENTIFIED**

---

## 🚨 **FAILURE DETAILS**

### **Test Location**
- File: `test/integration/gateway/webhook_integration_test.go:425`
- Test: "aggregates multiple related alerts into single storm CRD"
- Context: BR-GATEWAY-013: Storm Detection

### **Failure Message**
```
[FAILED] Should find RemediationRequest with process_id=1
Expected <*v1alpha1.RemediationRequest | 0x0>: nil not to be nil
```

---

## 🔍 **ROOT CAUSE ANALYSIS**

### **The Problem**
The test searches for a RemediationRequest by looking for a `process_id` label in `SignalLabels`:

```go
// Line 418-426
processIDStr := fmt.Sprintf("%d", processID)
for i := range crdList.Items {
    if crdList.Items[i].Spec.SignalLabels["process_id"] == processIDStr {
        testRR = &crdList.Items[i]
        break
    }
}
Expect(testRR).ToNot(BeNil(),
    "Should find RemediationRequest with process_id=%s", processIDStr)
```

### **The Issue**
The alert payload sent by the test **DOES NOT include a `process_id` label**:

```json
{
  "alerts": [{
    "status": "firing",
    "labels": {
      "alertname": "PodNotReady",
      "severity": "critical",
      "namespace": "%s",
      "pod": "app-pod-p%d-%d",      ← Uses processID here
      "node": "worker-node-03"
    },
    ...
  }]
}
```

**Analysis**:
- The `processID` is embedded in the `pod` label value: `"app-pod-p%d-%d"`
- But the test searches for a separate `process_id` label in `SignalLabels`
- The Gateway correctly maps alert labels to `SignalLabels` (line 362 in `crd_creator.go`)
- Since there's no `process_id` label in the alert, it won't be in `SignalLabels`

---

## 🔍 **HOW SIGNALLABELS WORKS**

### **Gateway Label Mapping Flow**

1. **Prometheus Adapter** receives alert payload
2. **Extracts labels** from alert:
   ```go
   labels := MergeLabels(alert.Labels, webhook.CommonLabels)
   ```
3. **Creates NormalizedSignal** with labels:
   ```go
   Labels: labels  // Contains: alertname, severity, namespace, pod, node
   ```
4. **CRD Creator** maps to RemediationRequest:
   ```go
   SignalLabels: c.truncateLabelValues(signal.Labels)
   ```

**Result**: `SignalLabels` contains exactly what was in the alert payload:
- `alertname`: "PodNotReady"
- `severity`: "critical"
- `namespace`: test namespace
- `pod`: "app-pod-p1-5" (example)
- `node`: "worker-node-03"

**Missing**: `process_id` (never sent in alert)

---

## 🎯 **WHY THE TEST FAILS**

### **Test Expectation vs Reality**

| Aspect | Test Expects | Reality |
|--------|-------------|---------|
| **Label Name** | `process_id` | Not in alert payload |
| **Label Location** | `SignalLabels["process_id"]` | Doesn't exist |
| **Process ID** | Separate label | Embedded in `pod` label value |

### **The Mismatch**
```go
// Test sends this:
"pod": "app-pod-p1-5"  // processID=1, i=5

// Test searches for this:
SignalLabels["process_id"] == "1"  // ❌ Doesn't exist
```

---

## 🔍 **PARALLEL EXECUTION CONTEXT**

### **Why process_id Was Intended**
The test uses `GinkgoParallelProcess()` to support parallel test execution:

```go
processID := GinkgoParallelProcess()  // Returns 1, 2, 3, 4 for parallel processes
```

**Intent**: Each parallel process should be able to find its own RemediationRequest by filtering on `process_id`.

**Problem**: The `process_id` is only embedded in the `pod` label value, not as a separate label.

---

## 💡 **FIX OPTIONS**

### **Option A: Add process_id Label to Alert Payload** ✅ **RECOMMENDED**
**Change**: Add `process_id` as a separate label in the alert payload

**Implementation**:
```go
payload := []byte(fmt.Sprintf(`{
  "alerts": [{
    "status": "firing",
    "labels": {
      "alertname": "PodNotReady",
      "severity": "critical",
      "namespace": "%s",
      "pod": "app-pod-p%d-%d",
      "node": "worker-node-03",
      "process_id": "%d"           ← ADD THIS
    },
    ...
  }]
}`, testNamespace, processID, i, processID))
```

**Pros**:
- ✅ Minimal change (1 line)
- ✅ Test logic remains clear
- ✅ Supports parallel execution
- ✅ No changes to production code

**Cons**:
- None

---

### **Option B: Search by Pod Label Pattern**
**Change**: Search for RemediationRequest by matching pod label pattern

**Implementation**:
```go
podPrefix := fmt.Sprintf("app-pod-p%d-", processID)
for i := range crdList.Items {
    if pod, ok := crdList.Items[i].Spec.SignalLabels["pod"]; ok {
        if strings.HasPrefix(pod, podPrefix) {
            testRR = &crdList.Items[i]
            break
        }
    }
}
```

**Pros**:
- ✅ No change to payload
- ✅ Uses existing labels

**Cons**:
- ❌ More complex logic
- ❌ String parsing in test
- ❌ Less clear intent

---

### **Option C: Search by Fingerprint**
**Change**: Calculate expected fingerprint and search by that

**Implementation**:
```go
// Calculate expected fingerprint
expectedFingerprint := calculateFingerprint("PodNotReady", testNamespace, "Pod", fmt.Sprintf("app-pod-p%d-1", processID))
for i := range crdList.Items {
    if crdList.Items[i].Spec.SignalFingerprint == expectedFingerprint {
        testRR = &crdList.Items[i]
        break
    }
}
```

**Pros**:
- ✅ Uses canonical identifier
- ✅ No payload change

**Cons**:
- ❌ Requires fingerprint calculation logic in test
- ❌ Couples test to fingerprint implementation
- ❌ Complex for parallel execution (need to know which alert to check)

---

### **Option D: Use Eventually() for CRD Availability**
**Change**: Replace `time.Sleep()` with `Eventually()` to wait for CRD

**Implementation**:
```go
// Wait for CRD to be created and available
Eventually(func() *remediationv1alpha1.RemediationRequest {
    var crdList remediationv1alpha1.RemediationRequestList
    err := k8sClient.Client.List(ctx, &crdList,
        client.InNamespace(testNamespace),
        client.MatchingFields{})
    if err != nil {
        return nil
    }

    processIDStr := fmt.Sprintf("%d", processID)
    for i := range crdList.Items {
        if crdList.Items[i].Spec.SignalLabels["process_id"] == processIDStr {
            return &crdList.Items[i]
        }
    }
    return nil
}, 30*time.Second, 1*time.Second).ShouldNot(BeNil())
```

**Pros**:
- ✅ Better synchronization
- ✅ Removes hardcoded sleep

**Cons**:
- ❌ Still requires Option A (adding process_id label)
- ❌ More complex test code

---

## 🎯 **RECOMMENDED FIX**

### **Combination: Option A + Option D**

**Step 1**: Add `process_id` label to alert payload
**Step 2**: Replace `time.Sleep()` with `Eventually()`

**Implementation**:
```go
// Step 1: Add process_id to payload
payload := []byte(fmt.Sprintf(`{
  "alerts": [{
    "status": "firing",
    "labels": {
      "alertname": "PodNotReady",
      "severity": "critical",
      "namespace": "%s",
      "pod": "app-pod-p%d-%d",
      "node": "worker-node-03",
      "process_id": "%d"
    },
    "annotations": {
      "summary": "Pod not ready after node failure"
    },
    "startsAt": "2025-10-22T14:00:00Z"
  }]
}`, testNamespace, processID, i, processID))

// Step 2: Use Eventually() instead of time.Sleep()
Eventually(func() *remediationv1alpha1.RemediationRequest {
    var crdList remediationv1alpha1.RemediationRequestList
    err := k8sClient.Client.List(ctx, &crdList,
        client.InNamespace(testNamespace),
        client.MatchingFields{})
    if err != nil {
        return nil
    }

    processIDStr := fmt.Sprintf("%d", processID)
    for i := range crdList.Items {
        if crdList.Items[i].Spec.SignalLabels["process_id"] == processIDStr {
            return &crdList.Items[i]
        }
    }
    return nil
}, 30*time.Second, 1*time.Second).ShouldNot(BeNil(),
    "Should find RemediationRequest with process_id=%d", processID)

testRR := ... // result from Eventually
```

**Benefits**:
- ✅ Fixes the root cause (missing label)
- ✅ Improves test reliability (Eventually vs sleep)
- ✅ Supports parallel execution
- ✅ Clear and maintainable
- ✅ No production code changes

---

## 🔍 **ADDITIONAL FINDINGS**

### **Race Condition in Storm Aggregation**
During test runs, this error was logged:

```json
{
  "level":"info",
  "msg":"Failed to update storm aggregation status (async, DD-GATEWAY-013)",
  "error":"remediationrequests.remediation.kubernaut.ai \"rr-99aec35babb6-1765648164\" not found",
  "fingerprint":"99aec35babb653c93671c9d0606e51e6d0e34699a9de4ea50fcf9f01fda04606",
  "rr":"rr-99aec35babb6-1765648164",
  "occurrenceCount":2,
  "threshold":5
}
```

**Analysis**:
- Gateway attempts to update storm aggregation status asynchronously
- The RemediationRequest may not exist yet (or was already deleted)
- This is logged as INFO (not ERROR), suggesting it's expected behavior
- The async update is a "best effort" operation

**Impact**: None on business functionality, but suggests potential timing issues in status updates.

---

## 📊 **SUMMARY**

### **Root Cause**
Test searches for `SignalLabels["process_id"]` but the alert payload doesn't include a `process_id` label.

### **Impact**
- 1 integration test failing (0.9% failure rate)
- No production impact (feature works correctly)
- Test expectation doesn't match payload structure

### **Recommended Fix**
Add `process_id` label to alert payload + use `Eventually()` for synchronization

### **Effort**
- **Low**: 2-line change in test file
- **Risk**: None (test-only change)
- **Duration**: 5 minutes

---

## 🎯 **NEXT STEPS**

1. ✅ Root cause identified
2. ⏳ Implement Option A + Option D (recommended fix)
3. ⏳ Run test to verify fix
4. ⏳ Update test documentation

---

**Status**: ✅ **ROOT CAUSE IDENTIFIED** - Ready for implementation

