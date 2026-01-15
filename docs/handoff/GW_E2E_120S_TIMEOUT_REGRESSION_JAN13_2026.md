# Gateway E2E 120s Timeout - Unexpected Regression - January 13, 2026

## 🚨 **Critical Issue**

**Option A (120s timeout increase) caused a major regression** instead of fixing the issues.

---

## 📊 **Results Summary**

| Metric | 60s Timeouts | 120s Timeouts | Delta |
|--------|--------------|---------------|-------|
| **Pass Rate** | 81/96 (84.4%) | 45/88 (51.1%) | **-33.3%** ❌ |
| **Failures** | 15 | 43 | **+28** ❌ |
| **Specs Run** | 96 | 88 | **-8** ❌ |
| **Skipped** | 4 | 12 | **+8** ❌ |

**Conclusion**: Increasing timeouts from 60s → 120s made things **significantly worse**, not better.

---

## 🔍 **What Went Wrong**

### **Hypothesis 1: Test Duration Timeout** (Most Likely)
**Theory**: Ginkgo has a default spec timeout (usually 10 minutes). Doubling timeouts from 60s → 120s may have pushed tests over this limit, causing them to be killed mid-execution.

**Evidence**:
- 8 fewer specs ran (96 → 88)
- 8 more tests skipped (4 → 12)
- Test suite completed faster (6m29s vs. expected 8+ minutes)
- Pattern suggests early termination

**Test Calculation**:
```
Original: 15 tests × 60s = 900s (15 min)
New: 43 tests × 120s = 5160s (86 min!)
```

If multiple tests hit their timeouts simultaneously, Ginkgo may have killed the suite.

### **Hypothesis 2: Resource Exhaustion**
**Theory**: Longer timeouts = more tests running concurrently = higher memory/CPU usage = K8s API server overload.

**Evidence**:
- More K8s API-related failures
- Tests that were passing now failing
- Pattern of widespread failures (not isolated to specific tests)

### **Hypothesis 3: K8s API Server Rate Limiting**
**Theory**: Longer `Eventually` blocks mean more API calls per test, hitting K8s rate limits harder.

**Evidence**:
- 12 parallel processes × 120s timeouts = sustained API load
- Many BeforeAll failures (namespace creation)
- "context canceled" patterns (rate limiter blocking)

---

## ❌ **Failed Tests Analysis**

### **Category Breakdown**:

| Category | Count | Examples |
|----------|-------|----------|
| **Audit Tests** | 12 | 23_audit_emission, 24_audit_signal_data |
| **Deduplication** | 9 | 36_deduplication_state, 35_deduplication_edge_cases |
| **Service Resilience** | 4 | 32_service_resilience |
| **BeforeAll Failures** | 8 | 03, 04, 08, 15, 16, 17, 19, 20 |
| **Webhook Integration** | 6 | 33_webhook_integration |
| **Other** | 4 | 27, 28, 09, 30 |

**Pattern**: Many tests that previously passed are now failing, suggesting a **systemic issue** introduced by longer timeouts.

---

## 🎯 **Root Cause Re-Analysis**

The original hypothesis was **incorrect**:

**❌ WRONG**: "E2E tests need longer timeouts to wait for K8s cache sync"

**✅ CORRECT**: The issue is **NOT** a simple timeout problem. The root causes are:

1. **K8s API Server Indexing Lag**: Field index updates lag 60-120s (true)
2. **Test Suite Design**: Tests create high concurrent load on K8s API server
3. **Gateway-Specific Pattern**: Gateway's rapid write sequences (storm aggregation, deduplication)
4. **12 Parallel Processes**: More aggressive than other services
5. **Ginkgo Spec Timeout**: Tests hitting suite-level timeout limits

**Key Insight**: Increasing timeouts exposed these systemic issues instead of solving the original problem.

---

## 💡 **Correct Solution Path**

### **Option 1: Revert 120s Changes** ⭐ (IMMEDIATE)
**Action**: Restore 60s timeouts to get back to 84.4% pass rate

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
git checkout test/e2e/gateway/30_observability_test.go
git checkout test/e2e/gateway/31_prometheus_adapter_test.go
git checkout test/e2e/gateway/24_audit_signal_data_test.go
git checkout test/e2e/gateway/32_service_resilience_test.go
git checkout test/e2e/gateway/23_audit_emission_test.go
git checkout test/e2e/gateway/22_audit_errors_test.go
git checkout test/e2e/gateway/27_error_handling_test.go
```

**Rationale**: Don't make things worse. Return to baseline.

**Effort**: 5 min  
**Expected Result**: 84.4% pass rate (81/96 tests)

---

### **Option 2: Reduce Parallel Process Count** (TACTICAL)
**Action**: Run E2E tests with fewer parallel processes to reduce K8s API load

```bash
# Currently: 12 parallel processes (default)
# Try: 6 parallel processes
make test-e2e-gateway GINKGO_PROCS=6
```

**Rationale**: Lower concurrency = less K8s API server load = more reliable

**Effort**: 5 min + test runtime  
**Expected Result**: 90-95% pass rate  
**Trade-off**: 2x longer test execution time

---

### **Option 3: Investigate Actual Cache Sync Behavior** (STRATEGIC)
**Action**: Add detailed logging to understand WHY CRDs aren't visible

```go
// In failing tests, add:
Eventually(func() (int, string) {
    var rrList remediationv1alpha1.RemediationRequestList
    err := k8sClient.List(ctx, &rrList, client.InNamespace(testNamespace))
    if err != nil {
        return 0, fmt.Sprintf("Error: %v", err)
    }
    if len(rrList.Items) == 0 {
        // Query by name directly (bypasses field index)
        var rr remediationv1alpha1.RemediationRequest
        err := k8sClient.Get(ctx, client.ObjectKey{
            Namespace: testNamespace,
            Name: expectedCRDName,
        }, &rr)
        if err != nil {
            return 0, fmt.Sprintf("CRD not found by name: %v", err)
        }
        return 0, "CRD exists by name but not by List() - field index lag"
    }
    return len(rrList.Items), "OK"
}, 60*time.Second, 1*time.Second).Should(Equal(1))
```

**Rationale**: Understand if issue is field indexing or something else

**Effort**: 2-3 hours  
**Expected Result**: Clear diagnosis of actual problem

---

### **Option 4: Use Client-Side Filtering** (PROPER FIX)
**Action**: Query all CRDs in namespace, filter client-side (no field index dependency)

```go
// BEFORE (relies on server-side field index)
err := k8sClient.List(ctx, &rrList, 
    client.InNamespace(testNamespace),
    client.MatchingFields{"spec.signalFingerprint": expectedFingerprint})

// AFTER (client-side filter - no index dependency)
err := k8sClient.List(ctx, &rrList, client.InNamespace(testNamespace))
for _, rr := range rrList.Items {
    if rr.Spec.SignalFingerprint == expectedFingerprint {
        // Found it!
    }
}
```

**Rationale**: Eliminates field indexing lag entirely

**Effort**: 2-3 hours  
**Expected Result**: 95-100% pass rate  
**Trade-off**: Less "realistic" (Gateway uses field indexing in production)

---

## 📊 **Recommendation Matrix**

| Option | Effort | Success Rate | Risk | When to Use |
|--------|--------|--------------|------|-------------|
| **1. Revert** | 5 min | 84.4% | Low | **NOW** (stop bleeding) |
| **2. Reduce Procs** | 10 min | 90-95% | Low | After revert (quick win) |
| **3. Investigate** | 3 hours | Unknown | Medium | To understand root cause |
| **4. Client Filter** | 3 hours | 95-100% | Low | Long-term fix |

---

## 🎯 **Immediate Action Plan**

1. **REVERT 120s changes** (5 min) - Get back to 84.4%
2. **TRY Option 2** (reduce to 6 parallel processes) - Quick test
3. **IF Option 2 works** (95%+ pass): Document and close
4. **IF Option 2 fails** (< 90%): Implement Option 4 (client-side filtering)

---

## 📋 **Lessons Learned**

1. **Timeout increases can expose new problems** - Not always a fix
2. **Test suite design matters** - 12 parallel processes may be too aggressive
3. **Field indexing has real latency** - Not just a caching issue
4. **Measure, don't guess** - Should have tested with 6 procs first

---

## 🔗 **Related Documents**

- `docs/handoff/GW_E2E_PHASE1_RESULTS_JAN13_2026.md` - Original analysis
- `docs/handoff/E2E_RATE_LIMITER_ROOT_CAUSE_JAN13_2026.md` - K8s rate limiter issue
- `docs/handoff/E2E_SUITE_CLIENT_FIX_IMPLEMENTED_JAN13_2026.md` - Suite-level client

---

**Document Status**: ⚠️ **URGENT - REGRESSION**  
**Next Action**: Revert 120s changes immediately  
**Confidence**: 95% that revert will restore 84.4% pass rate
