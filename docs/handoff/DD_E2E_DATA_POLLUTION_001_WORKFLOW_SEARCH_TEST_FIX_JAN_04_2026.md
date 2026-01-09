# DD-E2E-DATA-POLLUTION-001: Workflow Search Test Data Isolation Fix

**Date:** January 4, 2026
**Author:** AI Assistant
**Status:** ✅ FIXED
**Design Decision:** DD-E2E-DATA-POLLUTION-001

---

## 🔍 Issue Summary

**Test:** `test/e2e/datastorage/06_workflow_search_audit_test.go`
**Symptom:** Test failure: "Should find exactly 1 workflow matching the exact filter criteria (DD-TESTING-001)"
**Expected:** 1 workflow
**Actual:** 5 workflows
**Root Cause:** Test data pollution in shared database across parallel Ginkgo processes

---

## 📊 Root Cause Analysis

### Problem: Shared Database Without Process Isolation

DataStorage E2E tests run 12 parallel Ginkgo processes sharing a SINGLE PostgreSQL database:

```
Process 1: Creates workflow with labels { signal_type: "OOMKilled", severity: "critical", ... }
Process 2: Creates workflow with labels { signal_type: "OOMKilled", severity: "critical", ... }
Process 3: Creates workflow with labels { signal_type: "OOMKilled", severity: "critical", ... }
...
Process 7: Creates workflow with labels { signal_type: "OOMKilled", severity: "critical", ... }

Process 7 searches for: { signal_type: "OOMKilled", severity: "critical", ... }
Result: Finds workflows from processes 1, 2, 3, 5, 7 = 5 total workflows ❌
```

### Why `testID` Didn't Provide Isolation

```go
testID = fmt.Sprintf("audit-e2e-%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())
workflowID = fmt.Sprintf("wf-audit-test-%s", testID)
// Creates: wf-audit-test-audit-e2e-7-1767570294839150000 (unique workflow_name)

// But searches by LABELS, not workflow_name:
searchRequest := map[string]interface{}{
    "filters": map[string]interface{}{
        "signal_type": "OOMKilled",  // ❌ NOT UNIQUE - all processes use same labels
        "severity":    "critical",
        "component":   "deployment",
    },
}
```

**Key Insight:** The `workflow_name` was unique, but the search was filtering by **labels**, which were identical across all parallel processes.

---

## ✅ Solution: Unique Labels Per Parallel Process

### Design Decision: DD-E2E-DATA-POLLUTION-001

**Decision:** Make `signal_type` label unique per parallel process to guarantee test isolation in shared database environments.

**Format:** `OOMKilled-p{process_num}` (e.g., `OOMKilled-p7` for process 7)

### Implementation

**1. Introduce `uniqueSignalType` variable:**

```go
// DD-E2E-DATA-POLLUTION-001: Use unique signal_type per parallel process
// to prevent cross-contamination in shared database
uniqueSignalType := fmt.Sprintf("OOMKilled-p%d", GinkgoParallelProcess())
```

**2. Update workflow labels:**

```go
"labels": map[string]interface{}{
    "signal_type": uniqueSignalType, // mandatory - unique per process
    "severity":    "critical",       // mandatory
    "environment": "production",     // mandatory
    "priority":    "P0",             // mandatory
    "component":   "deployment",     // mandatory
},
```

**3. Update search filters:**

```go
searchRequest := map[string]interface{}{
    "remediation_id": remediationID,
    "filters": map[string]interface{}{
        "signal_type": uniqueSignalType, // mandatory - unique per process
        "severity":    "critical",       // mandatory
        "component":   "deployment",     // mandatory
        "environment": "production",     // mandatory
        "priority":    "P0",             // mandatory
    },
    "top_k": 5,
}
```

**4. Update assertion:**

```go
// DD-E2E-DATA-POLLUTION-001: Verify unique signal_type per parallel process
Expect(filters["signal_type"]).To(Equal(uniqueSignalType), "Filters should capture unique signal_type")
```

---

## 📈 Benefits

✅ **Guaranteed Isolation:** Each parallel process searches only its own workflows
✅ **No Database Cleanup Needed:** Workflows naturally isolated by labels
✅ **Realistic Test Execution:** Tests can run truly in parallel without interference
✅ **Minimal Code Changes:** Only 4 locations modified
✅ **Debugging-Friendly:** Failed tests don't leave polluted data for subsequent runs

---

## 🔧 Alternative Approaches Considered

### Option A: Database Cleanup in AfterAll
- ❌ Requires DELETE permissions in tests
- ❌ Removes debugging evidence when tests fail
- ❌ Doesn't prevent in-flight pollution during parallel execution

### Option C: Pre-Test Cleanup in BeforeAll
- ❌ Doesn't prevent cross-process pollution during test execution
- ❌ Requires complex synchronization between parallel processes
- ❌ Race condition: Process 7 cleanup could delete Process 3's in-progress workflow

**Winner: Option B** - Unique labels per process (implemented)

---

## 🧪 Validation

### Before Fix:
```
Process 1: workflow_name=wf-audit-test-audit-e2e-1-..., labels={signal_type: "OOMKilled"}
Process 7: workflow_name=wf-audit-test-audit-e2e-7-..., labels={signal_type: "OOMKilled"}
Process 7 search: signal_type="OOMKilled" → Finds 5 workflows ❌
```

### After Fix:
```
Process 1: workflow_name=wf-audit-test-audit-e2e-1-..., labels={signal_type: "OOMKilled-p1"}
Process 7: workflow_name=wf-audit-test-audit-e2e-7-..., labels={signal_type: "OOMKilled-p7"}
Process 7 search: signal_type="OOMKilled-p7" → Finds 1 workflow ✅
```

### Expected Test Result:
```
Expect(resultsData["total_found"]).To(Equal(float64(1)),
    "Should find exactly 1 workflow matching the exact filter criteria (DD-TESTING-001)")
✅ PASS
```

---

## 📝 Files Modified

- `test/e2e/datastorage/06_workflow_search_audit_test.go`:
  - Lines 127-131: Added `uniqueSignalType` variable
  - Lines 173-180: Updated workflow labels to use `uniqueSignalType`
  - Lines 209-221: Updated search filters to use `uniqueSignalType`
  - Lines 344-349: Updated assertion to verify `uniqueSignalType`

---

## 🎯 Impact

**Scope:** DataStorage E2E workflow search audit test only
**Risk:** LOW - Test-only change, no production code affected
**Testing:** E2E test suite validates fix

---

## 📚 Related Design Decisions

- **DD-TESTING-001:** Audit event testing standards (compliance maintained)
- **DD-TEST-001:** E2E test architecture using shared services (root cause of shared database)

---

## 🔗 References

- **Test File:** `test/e2e/datastorage/06_workflow_search_audit_test.go`
- **Business Requirements:** BR-AUDIT-023 through BR-AUDIT-028
- **Ginkgo Parallel Execution:** https://onsi.github.io/ginkgo/#parallel-specs

---

**Confidence:** 95%
**Justification:** Guaranteed isolation through unique labels per process. Simple, deterministic, and requires no cleanup logic. Only risk is if production code assumes specific `signal_type` values in tests, but labels are test data.



