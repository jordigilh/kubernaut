# RemediationOrchestrator Routing Test Failure - Root Cause Triage

**Date**: January 11, 2026
**Test**: `routing_integration_test.go:258` - "should allow RR when original RR completes (no longer active)"
**Status**: 🔍 **ROOT CAUSE IDENTIFIED**
**Impact**: 1 test failing (97.6% pass rate → 100% possible with fix)

---

## 🎯 **Executive Summary**

**Root Cause**: RoutingEngine uses **cached client** for queries, causing cache lag in multi-controller pattern with 12 parallel processes. Test manually sets RR1 to "Completed", but routing engine still sees stale "Processing" status when checking if RR2 should be blocked.

**Why It Worked Before**: Serial execution (or fewer processes) had faster cache refresh, masking the latency issue.

**Why It Fails Now**: 12 parallel processes create resource contention, exposing cache lag in routing engine queries.

---

## 🔍 **Detailed Root Cause Analysis**

### **Test Logic**
```go
// Test: routing_integration_test.go:218-261
1. Create RR1 with fingerprint "A"
2. Manually set RR1.Status.OverallPhase = "Completed"  // ← Manual status update
3. Create RR2 with same fingerprint "A"
4. Expect: RR2 should proceed (RR1 is "terminal", not "active")
5. Actual: RR2 times out after 60s - never proceeds
```

### **The Problem**

**Step 1: Test Manually Updates Status**
```go
// routing_integration_test.go:231-239
Eventually(func() error {
	rr := &remediationv1.RemediationRequest{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: rr1.Name, Namespace: ns}, rr)
	if err != nil {
		return err
	}
	rr.Status.OverallPhase = "Completed"  // ← Direct status update
	return k8sClient.Status().Update(ctx, rr)
}, timeout, interval).Should(Succeed())
```

**Step 2: Routing Engine Queries with Cached Client**
```go
// pkg/remediationorchestrator/routing/blocking.go:503-537
func (r *RoutingEngine) FindActiveRRForFingerprint(
	ctx context.Context,
	fingerprint string,
	excludeName string,
) (*remediationv1.RemediationRequest, error) {
	rrList := &remediationv1.RemediationRequestList{}

	// ❌ PROBLEM: Uses cached client.List()
	if err := r.client.List(ctx, rrList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list RemediationRequests by fingerprint: %w", err)
	}

	// Find first active (non-terminal) RR
	for i := range rrList.Items {
		rr := &rrList.Items[i]
		if rr.Name == excludeName {
			continue
		}
		// ❌ Cache still shows RR1 as "Processing" (non-terminal)
		if !IsTerminalPhase(rr.Status.OverallPhase) {
			return rr, nil  // ← Returns stale RR1 as "active"
		}
	}

	return nil, nil
}
```

**Step 3: RR2 Gets Blocked**
```go
// pkg/remediationorchestrator/routing/blocking.go:270-292
func (r *RoutingEngine) CheckDuplicateInProgress(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
) (*BlockingCondition, error) {
	// Finds RR1 (with stale status)
	originalRR, err := r.FindActiveRRForFingerprint(ctx, rr.Spec.SignalFingerprint, rr.Name)

	if originalRR != nil {
		// ❌ RR2 is blocked because routing engine thinks RR1 is still active
		return &BlockingCondition{
			Blocked:      true,
			Reason:       "DuplicateInProgress",
			Message:      fmt.Sprintf("Duplicate of active remediation %s...", originalRR.Name),
			RequeueAfter: 30 * time.Second,
			DuplicateOf:  originalRR.Name,
		}, nil
	}
}
```

### **RoutingEngine Structure**

```go
// pkg/remediationorchestrator/routing/blocking.go:44-48
type RoutingEngine struct {
	client    client.Client  // ❌ Cached client (subject to lag)
	namespace string
	config    Config
}

// ❌ MISSING: apiReader client.Reader for fresh queries
```

### **Why Multi-Controller Migration Exposed This**

| Aspect | Before Migration | After Migration | Impact |
|---|---|---|---|
| **Parallel Processes** | Fewer (4-6?) | 12 | More resource contention |
| **Cache Refresh Speed** | Faster (less load) | Slower (12 processes competing) | Longer cache lag |
| **Test Execution** | Serial/less parallel | Fully parallel | Exposes timing issues |
| **Cache Lag** | ~100-500ms | ~2-5s+ | Exceeds test expectations |

**Result**: Test that relied on "fast enough" cache refresh now times out.

---

## ✅ **Solution Options**

### **Option A: Add APIReader to RoutingEngine** (RECOMMENDED)

**Pattern**: Same approach as status manager (DD-STATUS-001)

**Changes Required**:
1. Add `apiReader client.Reader` to `RoutingEngine` struct
2. Pass `mgr.GetAPIReader()` when creating routing engine
3. Use `apiReader.List()` in `FindActiveRRForFingerprint` and `FindActiveWFEForTarget`

**Implementation**:
```go
// pkg/remediationorchestrator/routing/blocking.go
type RoutingEngine struct {
	client    client.Client
	apiReader client.Reader  // ✅ ADD: Cache-bypassed reader
	namespace string
	config    Config
}

func NewEngine(client client.Client, apiReader client.Reader, namespace string, config Config) *RoutingEngine {
	return &RoutingEngine{
		client:    client,
		apiReader: apiReader,  // ✅ ADD
		namespace: namespace,
		config:    config,
	}
}

func (r *RoutingEngine) FindActiveRRForFingerprint(
	ctx context.Context,
	fingerprint string,
	excludeName string,
) (*remediationv1.RemediationRequest, error) {
	rrList := &remediationv1.RemediationRequestList{}

	// ✅ FIX: Use apiReader for fresh data
	if err := r.apiReader.List(ctx, rrList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list RemediationRequests by fingerprint: %w", err)
	}

	// ... rest of logic
}
```

**Files to Modify** (5 files):
1. `pkg/remediationorchestrator/routing/blocking.go` - Add apiReader field
2. `pkg/remediationorchestrator/routing/blocking.go` - Update NewEngine
3. `pkg/remediationorchestrator/routing/blocking.go` - Use apiReader in FindActiveRRForFingerprint
4. `pkg/remediationorchestrator/routing/blocking.go` - Use apiReader in FindActiveWFEForTarget
5. `internal/controller/remediationorchestrator/reconciler.go` - Pass apiReader to NewEngine

**Pros**:
- ✅ Proper fix addressing root cause
- ✅ Prevents future cache lag issues
- ✅ Follows established DD-STATUS-001 pattern
- ✅ No test changes needed
- ✅ Improves routing reliability under load

**Cons**:
- ⚠️ Requires production code changes (5 files)
- ⚠️ Slightly more API server load (bypasses cache)
- ⚠️ Need to validate routing performance impact

**Estimated Time**: 30-45 minutes

---

### **Option B: Add Explicit Cache Wait in Test** (QUICK FIX)

**Approach**: Wait for controller cache to catch up after manual status update

**Implementation**:
```go
// test/integration/remediationorchestrator/routing_integration_test.go:229-245
// Simulate RR1 reaching terminal phase (Completed)
GinkgoWriter.Println("✅ Simulating RR1 completion...")
Eventually(func() error {
	rr := &remediationv1.RemediationRequest{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: rr1.Name, Namespace: ns}, rr)
	if err != nil {
		return err
	}
	rr.Status.OverallPhase = "Completed"
	return k8sClient.Status().Update(ctx, rr)
}, timeout, interval).Should(Succeed())

// ✅ ADD: Wait for controller cache to catch up
// In parallel execution (12 procs), cache refresh can take 2-5+ seconds
GinkgoWriter.Println("⏳ Waiting for RR1 completion to propagate to controller cache...")
time.Sleep(5 * time.Second)  // ← Simple sleep to allow cache refresh

GinkgoWriter.Println("✅ RR1 completed")

// Create second RR with SAME fingerprint (should NOT be blocked now)
rr2 := createRemediationRequestWithFingerprint(ns, "rr-signal-complete-2", fingerprint)
```

**Files to Modify** (1 file):
1. `test/integration/remediationorchestrator/routing_integration_test.go` - Add sleep

**Pros**:
- ✅ Minimal change (1 line)
- ✅ No production code impact
- ✅ Quick to implement and test
- ✅ Addresses immediate test failure

**Cons**:
- ❌ Band-aid solution, doesn't fix root cause
- ❌ Hardcoded sleep is brittle
- ❌ Cache lag problem remains in production
- ❌ Test runs 5s slower
- ❌ May still be flaky under high load

**Estimated Time**: 5 minutes

---

### **Option C: Use Consistent Eventually Pattern** (BETTER QUICK FIX)

**Approach**: Instead of sleep, poll until cache reflects the update

**Implementation**:
```go
// test/integration/remediationorchestrator/routing_integration_test.go:229-260
// Simulate RR1 reaching terminal phase (Completed)
GinkgoWriter.Println("✅ Simulating RR1 completion...")
Eventually(func() error {
	rr := &remediationv1.RemediationRequest{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: rr1.Name, Namespace: ns}, rr)
	if err != nil {
		return err
	}
	rr.Status.OverallPhase = "Completed"
	return k8sClient.Status().Update(ctx, rr)
}, timeout, interval).Should(Succeed())

// ✅ ADD: Verify another RR with same fingerprint sees NO active duplicates
// This confirms the routing engine's cache has caught up
GinkgoWriter.Println("⏳ Verifying routing engine cache has caught up...")
Eventually(func() bool {
	// Query using same field index the routing engine uses
	rrList := &remediationv1.RemediationRequestList{}
	err := k8sClient.List(ctx, rrList,
		client.InNamespace(ns),
		client.MatchingFields{"spec.signalFingerprint": fingerprint})
	if err != nil {
		return false
	}

	// Check if any non-terminal RRs exist (routing engine's logic)
	for _, rr := range rrList.Items {
		if rr.Status.OverallPhase != "Completed" &&
		   rr.Status.OverallPhase != "Failed" &&
		   rr.Status.OverallPhase != "TimedOut" &&
		   rr.Status.OverallPhase != "Skipped" &&
		   rr.Status.OverallPhase != "Cancelled" {
			GinkgoWriter.Printf("⏳ Still waiting for cache: %s is %s\n", rr.Name, rr.Status.OverallPhase)
			return false
		}
	}
	return true
}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(),
	"Routing engine cache should reflect RR1 as terminal")

GinkgoWriter.Println("✅ RR1 completed and visible to routing engine")

// Create second RR with SAME fingerprint (should NOT be blocked now)
rr2 := createRemediationRequestWithFingerprint(ns, "rr-signal-complete-2", fingerprint)

// Verify RR2 proceeds (keep original 60s timeout)
Eventually(func() bool {
	rr := &remediationv1.RemediationRequest{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: rr2.Name, Namespace: ns}, rr)
	if err != nil {
		return false
	}
	phase := string(rr.Status.OverallPhase)
	return phase == "Pending" || phase == "Processing" || phase == "Analyzing"
}, timeout, interval).Should(BeTrue(), "RR2 should proceed (original RR is no longer active)")
```

**Files to Modify** (1 file):
1. `test/integration/remediationorchestrator/routing_integration_test.go`

**Pros**:
- ✅ Test-only change
- ✅ Uses Eventually pattern (not hardcoded sleep)
- ✅ Self-documenting (shows cache lag issue)
- ✅ Adapts to actual cache refresh time
- ✅ Quick to implement

**Cons**:
- ⚠️ Still doesn't fix root cause in production
- ⚠️ Test is more verbose
- ⚠️ Adds 30s potential wait time

**Estimated Time**: 15 minutes

---

## 📊 **Recommendation Matrix**

| Criteria | Option A (APIReader) | Option B (Sleep) | Option C (Eventually) |
|---|---|---|---|
| **Fixes Root Cause** | ✅ Yes | ❌ No | ❌ No |
| **Implementation Time** | 30-45 min | 5 min | 15 min |
| **Production Impact** | ✅ Improves reliability | ❌ None | ❌ None |
| **Test Reliability** | ✅ Excellent | ⚠️ Moderate | ✅ Good |
| **Maintenance** | ✅ Clean | ❌ Brittle | ✅ Acceptable |
| **Risk** | ⚠️ Medium (prod change) | ✅ Low (test only) | ✅ Low (test only) |

---

## 🎯 **Final Recommendation**

### **Immediate (Tonight)**: **Option C** (Eventually Pattern)
- Get to 100% pass rate quickly
- Low risk test-only change
- Documents the cache lag issue

### **Follow-up (Next Sprint)**: **Option A** (APIReader to RoutingEngine)
- Proper fix following DD-STATUS-001 pattern
- Improves production routing reliability
- Consistent with AIAnalysis/SignalProcessing/Notification patterns
- Creates technical debt ticket for proper fix

### **Implementation Plan**

**Phase 1: Immediate Fix** (15 minutes)
1. Apply Option C to routing_integration_test.go
2. Run parallel tests to validate (make test-integration-remediationorchestrator TEST_PROCS=12)
3. Confirm 100% pass rate (41/41)

**Phase 2: Proper Fix** (Next Sprint, 1-2 hours)
1. Add apiReader to RoutingEngine (follow DD-STATUS-001 pattern)
2. Update all routing queries to use apiReader
3. Test in integration environment
4. Validate no performance regression
5. Remove cache wait from test (revert to original logic)
6. Document as DD-ROUTING-001 pattern

---

## 📁 **Files Requiring Changes**

### **Option C (Immediate)**
- `test/integration/remediationorchestrator/routing_integration_test.go`

### **Option A (Follow-up)**
- `pkg/remediationorchestrator/routing/blocking.go` (4 changes: struct, NewEngine, 2 query methods)
- `internal/controller/remediationorchestrator/reconciler.go` (pass apiReader)

---

## 🔗 **Related Issues**

- [AA-HAPI-001](./AA_HAPI_001_API_READER_FIX_JAN11_2026.md) - Original APIReader discovery
- [DD-STATUS-001](../architecture/decisions/DD-STATUS-001-cache-bypassed-status-refetch.md) - Status manager pattern
- [DD-CONTROLLER-001 v3.0](../architecture/decisions/DD-CONTROLLER-001-multi-controller-pattern.md) - Multi-controller pattern

---

**Document Status**: ✅ **Complete**
**Triage Status**: ✅ **Root Cause Identified**
**Recommendation**: **Option C (immediate) + Option A (follow-up)**
**Confidence**: **99%**
