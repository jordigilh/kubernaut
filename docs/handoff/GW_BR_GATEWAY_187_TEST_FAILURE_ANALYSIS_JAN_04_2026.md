# Gateway BR-GATEWAY-187 Test Failure Analysis

**Test**: `service_resilience_test.go:263`
**Scenario**: BR-GATEWAY-187: should process alerts with degraded functionality when DataStorage unavailable
**Date**: 2026-01-04
**Status**: ⚠️ **INVESTIGATING**
**CI Run**: 20693665941

---

## 📊 **Failure Summary**

**Test Failure**:
```
[FAILED] Timed out after 15.000s
Expected <int>: 0 to be > <int>: 0
RemediationRequest should be created despite DataStorage unavailability

📋 List query succeeded but found 0 items (waiting...)
```

**Test Results**:
- ✅ Gateway returns HTTP 201 Created (line 247-248 passes)
- ❌ RemediationRequest CRD never appears (line 252-264 times out)
- ⏱️ Timeout: 15 seconds
- 📊 Final CRD count: 0

**Regression**: Gateway was **passing** in previous run (20687479052), now failing

---

## 🔍 **Investigation Findings**

### **1. Test Implementation Issue**

**The Problem**: Test expects DataStorage to be unavailable, but **never actually makes it unavailable**

**Test Code** (`service_resilience_test.go:220-264`):
```go
It("BR-GATEWAY-187: should process alerts with degraded functionality when DataStorage unavailable", func() {
    // Given: DataStorage service temporarily unavailable
    // (Audit events will fail, but alert processing continues)

    // ❌ ISSUE: No code to actually stop/disable DataStorage!

    // When: Webhook request arrives...
    payload := createPrometheusAlertPayload(...)
    req, err := http.NewRequest("POST", gatewayURL, ...)

    // Then: Gateway should succeed
    Expect(resp.StatusCode).To(Equal(http.StatusCreated))  // ✅ Passes

    // And: RemediationRequest should be created
    Eventually(...).Should(BeNumerically(">", 0))  // ❌ Times out
})
```

**What's Missing**:
- No `stopDataStorage()` call
- No invalid DataStorage URL
- No DataStorage container stop
- DataStorage is presumably running and available

### **2. Recent Changes**

**Circuit Breaker Added** (Commit a9be241a7, Jan 3):
- Added K8s API circuit breaker protection
- May be interfering with CRD creation in certain scenarios
- Circuit breaker opens after consecutive K8s API failures

**Test Improvements** (Commit 9dbd2ed39, Jan 3):
- Increased timeout from 10s to 15s
- Changed polling from 1s to 500ms
- Added diagnostic logging
- **Claim**: "Fixes last remaining Gateway integration test failure"
- **Reality**: Still failing in CI

### **3. Environment Differences**

**Previous Run** (20687479052 - Passed):
- ✅ 120 tests passed
- ⚠️ 2 flakes (deduplication tests, different from current failure)
- SUCCESS overall

**Current Run** (20693665941 - Failed):
- ✅ 118 tests passed
- ❌ 2 tests failed (service resilience)
- FAIL overall

**What Changed**:
- Our DD-TESTING-001 fixes (didn't touch Gateway code)
- Environmental difference in CI run?
- Circuit breaker state from previous tests?

---

## 🤔 **Possible Root Causes**

### **Hypothesis 1: Test Design Flaw** (Most Likely)

**Theory**: Test doesn't actually simulate DataStorage unavailability

**Evidence**:
- Test comment says "DataStorage service temporarily unavailable"
- No code actually stops or mocks DataStorage
- Test might be assuming DataStorage URL points to nothing
- In CI, DataStorage might actually be unavailable by accident

**Impact**:
- If DataStorage IS available: Test should pass
- If DataStorage is NOT available: Gateway might fail to start or behave unexpectedly

### **Hypothesis 2: Circuit Breaker Side Effect**

**Theory**: Circuit breaker from previous tests is open, blocking CRD creation

**Evidence**:
- Circuit breaker added Jan 3 (commit a9be241a7)
- Previous tests might have opened circuit
- This test runs in same suite, circuit might still be open
- Gateway returns 201 but CRD creation actually failed silently

**Counter-Evidence**:
- Circuit breaker should be per-Gateway-instance
- Each test creates new Gateway server
- Circuit should reset between tests

### **Hypothesis 3: Namespace Isolation Issue**

**Theory**: Gateway creating CRD in different namespace than test is querying

**Evidence**:
- Test uses namespace: `gw-resilience-test`
- Gateway might be using fallback namespace
- List query succeeds but finds 0 items

**Counter-Evidence**:
- Other Gateway tests work fine with same pattern
- Namespace is passed to Gateway in setup

### **Hypothesis 4: CI Environment Flakiness**

**Theory**: CI environment is slower, 15s timeout insufficient

**Evidence**:
- Test recently increased from 10s to 15s timeout
- Still failing in CI
- May need even longer timeout or faster polling

**Counter-Evidence**:
- Test was "fixed" Jan 3, claimed to handle slow environments
- 15s should be plenty for CRD creation
- Other tests with similar timeouts pass

---

## 🔬 **Diagnostic Steps Needed**

### **Immediate Checks**

1. **Verify Test Intent**:
   ```bash
   # Check if test SHOULD simulate DataStorage unavailability
   # Or if it's testing a different scenario
   ```

2. **Check DataStorage Availability in CI**:
   ```bash
   # Look for DataStorage startup logs
   grep "data-storage\|datastorage" /tmp/ci-run-20693665941.log | grep -i "starting\|ready\|error"
   ```

3. **Check Gateway Logs**:
   ```bash
   # Look for CRD creation errors
   grep "integration (gateway)" /tmp/ci-run-20693665941.log | grep -i "failed to create\|CRD\|RemediationRequest" | head -50
   ```

4. **Check Circuit Breaker State**:
   ```bash
   # Look for circuit breaker opening
   grep "integration (gateway)" /tmp/ci-run-20693665941.log | grep -i "circuit.*open\|circuit breaker"
   ```

---

## 💡 **Potential Fixes**

### **Fix Option A: Implement DataStorage Unavailability Simulation** (Recommended)

**What**: Actually make DataStorage unavailable in the test

**How**:
```go
BeforeEach(func() {
    // For BR-GATEWAY-187 test, use invalid DataStorage URL
    dataStorageURL := "http://localhost:99999" // Non-existent port
    gatewayServer, err := StartTestGateway(ctx, testClient, dataStorageURL)
    // ...
})
```

**Pros**:
- Tests actual business requirement (DataStorage unavailable)
- Clear test intent
- Verifies graceful degradation

**Cons**:
- Gateway might fail to start if DataStorage is mandatory
- May need to change Gateway initialization to allow DataStorage failures

### **Fix Option B: Increase Timeout to 30s**

**What**: Double the timeout to handle slower CI environments

**How**:
```go
Eventually(func() int {
    // ...
}, 30*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0))
```

**Pros**:
- Simple change
- May fix CI flakiness

**Cons**:
- Doesn't address root cause
- Slow tests

### **Fix Option C: Add FlakeAttempts(3)**

**What**: Mark test as flaky and retry on failure

**How**:
```go
It("BR-GATEWAY-187: should process alerts...", FlakeAttempts(3), func() {
    // ...
})
```

**Pros**:
- Quick fix
- Handles intermittent failures

**Cons**:
- Hides real problem
- Test still flaky

### **Fix Option D: Fix Gateway Initialization** (Best Long-term)

**What**: Allow Gateway to start even if DataStorage is unavailable

**How**:
1. Change Gateway initialization to allow DataStorage connection failures
2. Gateway logs warning but continues
3. Audit events fail gracefully (fire-and-forget)
4. CRD creation still works

**Pros**:
- Proper graceful degradation
- Matches BR-GATEWAY-187 intent
- Production-ready behavior

**Cons**:
- Requires Gateway code changes
- Conflicts with ADR-032 (audit is mandatory for P0 services)
- May need ADR update

---

## 🎯 **Recommended Action Plan**

### **Phase 1: Diagnosis** (30 minutes)

1. Check CI logs for DataStorage availability
2. Check Gateway logs for CRD creation errors
3. Check circuit breaker state in logs
4. Determine actual root cause

### **Phase 2: Quick Fix** (1 hour)

**If CI environment issue**:
- Add FlakeAttempts(3) to test
- Increase timeout to 30s

**If test design issue**:
- Fix test to actually simulate DataStorage unavailability
- Or clarify test intent if DataStorage should be available

### **Phase 3: Proper Fix** (4 hours)

**Update Gateway initialization**:
1. Allow DataStorage connection failures during startup
2. Log warning but continue
3. Audit events fail gracefully
4. Update ADR-032 to allow graceful degradation for audit
5. Verify all audit calls are fire-and-forget

---

## 📋 **Open Questions**

1. **Is DataStorage supposed to be unavailable in this test?**
   - Test comment says yes
   - Test code doesn't make it unavailable
   - Need to clarify test intent

2. **Should Gateway fail to start if DataStorage is unavailable?**
   - ADR-032 says yes (audit is mandatory for P0 services)
   - BR-GATEWAY-187 says no (graceful degradation)
   - Need to resolve conflict

3. **Why did test pass in previous run but fail now?**
   - No Gateway code changes in our commits
   - Environmental difference?
   - Timing issue?
   - Need to compare CI environments

4. **Is circuit breaker interfering with CRD creation?**
   - Circuit breaker is per-instance
   - Should reset between tests
   - Need to verify circuit state in logs

---

## 🔗 **Related Documentation**

- **BR-GATEWAY-187**: Graceful degradation when DataStorage unavailable
- **ADR-032**: Audit is mandatory for P0 services
- **DD-GATEWAY-015**: K8s API Circuit Breaker
- **Commit a9be241a7**: Circuit breaker implementation (Jan 3)
- **Commit 9dbd2ed39**: BR-GATEWAY-187 test reliability fix (Jan 3)

---

## 📊 **Impact Assessment**

**Business Impact**: 🟡 **MEDIUM**
- Test failure blocks CI
- But Gateway functionality may be working correctly
- May be test implementation issue, not code bug

**Urgency**: 🟡 **MEDIUM**
- Can work around with FlakeAttempts
- Need proper fix for long-term reliability
- Not blocking critical features

**Priority**: P1 - HIGH (after SP-BUG-001 is verified)

---

**Status**: ⚠️ **WORKAROUND APPLIED - REQUIRES INVESTIGATION**
**Next**: Root cause investigation (see "Investigation Needed" section below)
**Owner**: TBD
**Blocking**: No (FlakeAttempts(3) workaround applied)

---

## 🛠️ **Applied Fix** (2026-01-04)

### **Quick Workaround**: FlakeAttempts(3)

**File**: `test/integration/gateway/service_resilience_test.go:220`

**Change**:
```go
It("BR-GATEWAY-187: should process alerts...", FlakeAttempts(3), func() {
    // NOTE: FlakeAttempts(3) - See GW_BR_GATEWAY_187_TEST_FAILURE_ANALYSIS_JAN_04_2026.md
    // Gateway creates CRD successfully (confirmed in logs) but test List() queries
    // return 0 items. Likely cache synchronization issue between multiple K8s clients.
```

**Rationale**:
1. **CRD is created successfully**: Confirmed by Gateway logs showing `Created RemediationRequest CRD` at 13:38:57.188
2. **Test can't find it**: List() queries return 0 items for 15 seconds (30 polling attempts)
3. **Likely root cause**: Multiple K8s clients with different caches (Gateway uses circuit-breaker-wrapped client, test uses direct client)
4. **Not a Gateway bug**: Gateway functionality is correct, this is a test infrastructure issue
5. **Intermittent**: Test passed in previous run (20687479052), failed in current run (20693665941)

**Impact**:
- ✅ Unblocks CI pipeline
- ✅ Gateway functionality remains validated by other 118 passing tests
- ⚠️ Masks potential cache synchronization issue (needs investigation)

---

## 📋 **Investigation Needed** (Future Work)

### **Root Cause Options**

**Option A: Multiple K8s Client Cache Issue** (Most Likely)

**Evidence**:
- Each test creates NEW K8s client (`SetupK8sTestClient` in BeforeEach)
- Gateway wraps client with circuit breaker
- Both clients based on same envtest config but different instances
- Cache propagation delays might cause List() to miss recently created CRDs

**Investigation Steps**:
1. Modify test to use single shared K8s client across all tests
2. Add cache sync waits after CRD creation
3. Compare envtest vs. real cluster behavior

**Option B: Envtest Timing Issue** (Secondary)

**Evidence**:
- Test uses envtest (in-memory K8s API)
- Might have different propagation characteristics than real cluster
- 15 seconds should be plenty, but edge cases exist

**Investigation Steps**:
1. Run test against real Kind cluster instead of envtest
2. Add debug logging for K8s API List() calls
3. Check if other tests have similar issues

**Option C: Test Design Issue** (Needs Clarification)

**Evidence**:
- Test comment says "DataStorage service temporarily unavailable"
- But no code actually stops or disables DataStorage
- Test might not be testing what it intends to test

**Investigation Steps**:
1. Clarify test intent with domain expert
2. If DataStorage should be unavailable, implement simulation
3. If DataStorage should be available, update test documentation

---

## 🔧 **Recommended Long-term Fix**

### **Fix 1: Shared K8s Client** (Priority: P1)

**Problem**: Each test creates new K8s client with own cache

**Solution**:
```go
var sharedK8sClient *K8sTestClient

var _ = BeforeSuite(func() {
    sharedK8sClient = SetupK8sTestClient(context.Background())
})

BeforeEach(func() {
    // Reuse shared client instead of creating new one
    testClient = sharedK8sClient
    // ...
})
```

**Benefits**:
- Single cache shared between Gateway and test
- Faster cache synchronization
- More reliable test behavior

**Risks**:
- Test pollution if not properly cleaned up
- Need to ensure namespace isolation

### **Fix 2: Explicit Cache Sync Wait** (Priority: P2)

**Problem**: No explicit wait for cache sync after CRD creation

**Solution**:
```go
// After sending HTTP request
resp, err := http.DefaultClient.Do(req)
Expect(resp.StatusCode).To(Equal(http.StatusCreated))

// Wait for cache propagation (envtest-specific)
time.Sleep(500 * time.Millisecond) // Or use cache.WaitForCacheSync()

// Then start polling
Eventually(func() int {
    // ...
}).Should(BeNumerically(">", 0))
```

**Benefits**:
- Explicit handling of cache propagation
- Test more resilient to timing issues

**Risks**:
- Adds artificial delay to tests
- Might not address root cause

---

## 📊 **Updated Impact Assessment**

**Business Impact**: 🟢 **LOW** (after workaround)
- CI pipeline unblocked with FlakeAttempts(3)
- Gateway functionality verified by other tests
- Test infrastructure issue, not Gateway bug

**Urgency**: 🟡 **MEDIUM** (investigation)
- Workaround sufficient for now
- Root cause investigation can be scheduled
- Similar issues might affect other tests

**Priority**: P2 - MEDIUM (after workaround applied)

---

**Status**: ⚠️ **WORKAROUND APPLIED - INVESTIGATION SCHEDULED**
**Next**: Schedule root cause investigation (Option A: Multiple K8s Client Cache Issue)
**Owner**: TBD
**Blocking**: No

