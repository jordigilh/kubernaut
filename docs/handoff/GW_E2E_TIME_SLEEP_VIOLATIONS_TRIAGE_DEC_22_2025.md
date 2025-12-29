# Gateway E2E time.Sleep() Violations Triage

**Date**: December 22, 2025
**Status**: 🔴 **3 VIOLATIONS FOUND** - Requires immediate fix
**Authoritative Source**: [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md#-timesleep-is-absolutely-forbidden-in-tests)

---

## 📋 Executive Summary

Triaged all Gateway E2E tests for `time.Sleep()` usage per TESTING_GUIDELINES.md mandatory policy. Found **3 violations** and **10 acceptable uses**.

**CRITICAL VIOLATIONS**:
1. **Test 11 (Fingerprint Stability)**: Line 159 - Waiting between alerts
2. **Test 20 (Security Headers)**: Line 255 - Waiting for metrics update
3. **Suite Setup (gateway_e2e_suite_test.go)**: Line 130 - Waiting for Gateway readiness

**Acceptable Uses**: 10 instances (intentional request staggering for storm scenarios, TTL timing tests)

---

## 🚨 VIOLATIONS - MUST FIX

### Violation 1: Test 11 - Fingerprint Stability (Line 159)

**File**: `test/e2e/gateway/11_fingerprint_stability_test.go`

**Code**:
```go
testLogger.Info("✅ First alert sent", "fingerprint", firstFingerprint)

// Wait a moment before sending second alert
time.Sleep(500 * time.Millisecond)  // ❌ VIOLATION

testLogger.Info("Step 2: Send identical alert again")
```

**Why Violation**: Waiting for an arbitrary duration before sending the second alert instead of verifying that the first alert was processed.

**Fix**:
```go
testLogger.Info("✅ First alert sent", "fingerprint", firstFingerprint)

// ✅ CORRECT: Wait for first alert to be processed
Eventually(func() bool {
    // Check that gateway is ready for next request
    // Could check health endpoint or verify first fingerprint persisted
    resp, err := httpClient.Get(gatewayURL + "/health")
    if err != nil {
        return false
    }
    resp.Body.Close()
    return resp.StatusCode == http.StatusOK
}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

testLogger.Info("Step 2: Send identical alert again")
```

**Impact**: MEDIUM - Could cause flaky test if first alert processing takes longer than 500ms

---

### Violation 2: Test 20 - Security Headers (Line 255)

**File**: `test/e2e/gateway/20_security_headers_test.go`

**Code**:
```go
Expect(resp.StatusCode).To(Equal(http.StatusCreated))

testLogger.Info("Step 3: Fetch updated metrics")
time.Sleep(1 * time.Second) // ❌ VIOLATION - Allow metrics to be updated

metricsResp2, err := httpClient.Get(gatewayURL + "/metrics")
Expect(err).ToNot(HaveOccurred())
```

**Why Violation**: Waiting for metrics to update instead of polling until metrics change.

**Fix**:
```go
Expect(resp.StatusCode).To(Equal(http.StatusCreated))

testLogger.Info("Step 3: Fetch updated metrics")

// ✅ CORRECT: Poll metrics until count increases
var initialCount, updatedCount int
Eventually(func() bool {
    metricsResp, err := httpClient.Get(gatewayURL + "/metrics")
    if err != nil {
        return false
    }
    defer metricsResp.Body.Close()

    body, _ := io.ReadAll(metricsResp.Body)
    metricsStr := string(body)

    // Extract count from gateway_http_request_duration_seconds_count metric
    // Simple regex or string matching to find the current count
    re := regexp.MustCompile(`gateway_http_request_duration_seconds_count\{[^}]*\}\s+(\d+)`)
    matches := re.FindStringSubmatch(metricsStr)
    if len(matches) > 1 {
        count, _ := strconv.Atoi(matches[1])
        if initialCount == 0 {
            initialCount = count
            return false
        }
        updatedCount = count
        return updatedCount > initialCount
    }
    return false
}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
    "Metrics should update after request")

metricsResp2, err := httpClient.Get(gatewayURL + "/metrics")
```

**Impact**: HIGH - Metrics update is asynchronous, fixed 1s wait may be too short on slow systems

---

### Violation 3: Suite Setup - Gateway Readiness (Line 130)

**File**: `test/e2e/gateway/gateway_e2e_suite_test.go`

**Code**:
```go
for i := 0; i < 30; i++ {
    resp, err := http.Get(gatewayURL + "/health")
    if err == nil && resp.StatusCode == http.StatusOK {
        gatewayReady = true
        break
    }
    if resp != nil {
        resp.Body.Close()
    }
    time.Sleep(2 * time.Second)  // ❌ VIOLATION
}
Expect(gatewayReady).To(BeTrue(), "Gateway HTTP endpoint should be ready within 60 seconds")
```

**Why Violation**: Manual polling loop instead of using Eventually() pattern.

**Fix**:
```go
// ✅ CORRECT: Use Eventually() for Gateway readiness
Eventually(func() int {
    resp, err := http.Get(gatewayURL + "/health")
    if err != nil {
        return 0
    }
    defer resp.Body.Close()
    return resp.StatusCode
}, 60*time.Second, 2*time.Second).Should(Equal(http.StatusOK),
    "Gateway HTTP endpoint should be ready within 60 seconds")

tempLogger.Info("✅ Gateway HTTP endpoint is ready")
```

**Impact**: CRITICAL - This is in suite setup, affects ALL tests. Current pattern works but violates best practices.

---

## ✅ ACCEPTABLE USES (10 instances)

### Category 1: Intentional Request Staggering (Storm Scenarios)

**Policy**: ✅ Acceptable per TESTING_GUIDELINES.md - "Staggering requests for specific test scenario"

**Instances**:

#### Test 11 - Fingerprint Stability (Line 349)
**File**: `test/e2e/gateway/11_fingerprint_stability_test.go`
```go
for i := 0; i < alertCount; i++ {
    Eventually(func() error {
        resp, err := httpClient.Post(/*...*/)
        // ...
    }, 10*time.Second, 1*time.Second).Should(Succeed())

    testLogger.Info(fmt.Sprintf("  Sent alert %d/%d", i+1, alertCount))
    time.Sleep(100 * time.Millisecond)  // ✅ ACCEPTABLE - intentional stagger
}
```
**Rationale**: Intentionally staggering alerts to create controlled deduplication scenario.

---

#### Test 13 - Redis Failure (Lines 174, 263)
**File**: `test/e2e/gateway/13_redis_failure_graceful_degradation_test.go`

**Line 174** (Pre-failure burst):
```go
for i := 1; i <= preFailureAlerts; i++ {
    resp, err := httpClient.Post(/*...*/)
    // ...
    time.Sleep(100 * time.Millisecond)  // ✅ ACCEPTABLE - burst scenario
}
```

**Line 263** (During-failure burst):
```go
for i := 1; i <= failureAlerts; i++ {
    resp, err := httpClient.Post(/*...*/)
    // ...
    time.Sleep(500 * time.Millisecond)  // ✅ ACCEPTABLE - burst scenario
}
```
**Rationale**: Creating controlled burst pattern to test Redis failure handling and backpressure.

---

#### Test 14 - TTL Expiration (Lines 153, 234)
**File**: `test/e2e/gateway/14_deduplication_ttl_expiration_test.go`

**Line 153** (Pre-TTL alerts):
```go
for i := 0; i < alertCount; i++ {
    resp, err := httpClient.Post(/*...*/)
    // ...
    time.Sleep(100 * time.Millisecond)  // ✅ ACCEPTABLE - staggered burst
}
```

**Line 234** (Post-TTL alerts):
```go
for i := 0; i < alertCount; i++ {
    resp, err := httpClient.Post(/*...*/)
    // ...
    time.Sleep(100 * time.Millisecond)  // ✅ ACCEPTABLE - staggered burst
}
```
**Rationale**: Creating staggered alert pattern to test deduplication before and after TTL expiration.

---

#### Test 12 - Gateway Restart (Lines 153, 232)
**File**: `test/e2e/gateway/12_gateway_restart_recovery_test.go`

**Line 153** (Pre-restart alerts):
```go
for i := 0; i < alertCount; i++ {
    resp, err := httpClient.Post(/*...*/)
    // ...
    time.Sleep(100 * time.Millisecond)  // ✅ ACCEPTABLE - staggered burst
}
```

**Line 232** (Post-restart alerts):
```go
for i := 0; i < alertCount; i++ {
    resp, err := httpClient.Post(/*...*/)
    // ...
    time.Sleep(100 * time.Millisecond)  // ✅ ACCEPTABLE - staggered burst
}
```
**Rationale**: Creating controlled alert pattern to test Gateway state recovery after restart.

---

### Category 2: Testing Timing Behavior (TTL Expiration)

**Policy**: ✅ Acceptable per TESTING_GUIDELINES.md - "Testing timing behavior"

#### Test 14 - TTL Expiration (Line 190)
**File**: `test/e2e/gateway/14_deduplication_ttl_expiration_test.go`
```go
testLogger.Info("Step 3: Wait for deduplication TTL to expire")
testLogger.Info("  Waiting 70 seconds for TTL expiration (configured TTL + buffer)...")
time.Sleep(70 * time.Second)  // ✅ ACCEPTABLE - testing TTL timing behavior
```
**Rationale**: Explicitly testing that TTL expiration works after 60 seconds. The sleep IS the test - verifying time-based behavior.

**Note**: This is followed by Eventually() to verify alert processing, which is correct.

---

## 📊 Summary Statistics

| Category | Count | Status |
|---|---|---|
| **VIOLATIONS** | 3 | 🔴 Must fix |
| **Acceptable (Staggering)** | 9 | ✅ Keep as-is |
| **Acceptable (Timing)** | 1 | ✅ Keep as-is |
| **Total time.Sleep() calls** | 13 | - |

---

## 🔧 Remediation Plan

### Priority Order

1. **P0 (CRITICAL)**: Fix Suite Setup (gateway_e2e_suite_test.go:130)
   - **Impact**: Affects all tests, violates best practices in core infrastructure
   - **Effort**: 5 minutes
   - **Fix**: Replace manual loop with Eventually()

2. **P1 (HIGH)**: Fix Test 20 (20_security_headers_test.go:255)
   - **Impact**: Flaky metrics validation, high chance of false positives/negatives
   - **Effort**: 15 minutes
   - **Fix**: Poll metrics until count increases

3. **P2 (MEDIUM)**: Fix Test 11 (11_fingerprint_stability_test.go:159)
   - **Impact**: Minor flakiness risk, well-bounded by Eventually() wrapper
   - **Effort**: 10 minutes
   - **Fix**: Wait for gateway health check instead of arbitrary sleep

---

## 📝 Implementation Checklist

- [ ] **Fix gateway_e2e_suite_test.go:130** (P0)
  - [ ] Replace manual loop with Eventually()
  - [ ] Test suite setup completes successfully
  - [ ] All E2E tests still pass

- [ ] **Fix 20_security_headers_test.go:255** (P1)
  - [ ] Implement metrics polling with Eventually()
  - [ ] Add regex/parsing for metric count extraction
  - [ ] Verify metrics update correctly

- [ ] **Fix 11_fingerprint_stability_test.go:159** (P2)
  - [ ] Add health check polling between alerts
  - [ ] Verify fingerprint stability test still passes
  - [ ] Confirm deduplication behavior unchanged

- [ ] **Run full Gateway E2E suite** to validate fixes
  - [ ] All tests pass
  - [ ] No new flakiness introduced
  - [ ] Test duration remains reasonable

- [ ] **Update GW_E2E_STRUCTURED_PAYLOAD_MIGRATION_COMPLETE_DEC_22_2025.md**
  - [ ] Add section on time.Sleep() violations fixed
  - [ ] Document total refactoring scope (9 tests + 3 sleep violations)

---

## 🎯 Success Criteria

**COMPLETE when**:
- ✅ All 3 violations fixed
- ✅ No new `time.Sleep()` violations introduced
- ✅ Full Gateway E2E suite passes
- ✅ Test execution time not significantly increased
- ✅ No flakiness introduced by changes

---

## 📚 References

- **Authoritative Policy**: [TESTING_GUIDELINES.md - time.Sleep() is ABSOLUTELY FORBIDDEN](../../development/business-requirements/TESTING_GUIDELINES.md#-timesleep-is-absolutely-forbidden-in-tests)
- **Eventually() Best Practices**: TESTING_GUIDELINES.md lines 619-694
- **Acceptable Sleep Use Cases**: TESTING_GUIDELINES.md lines 729-760
- **Related Work**: GW_E2E_STRUCTURED_PAYLOAD_MIGRATION_COMPLETE_DEC_22_2025.md

---

**Generated**: 2025-12-22
**Status**: 🔴 VIOLATIONS IDENTIFIED - AWAITING FIX
**Next Action**: Fix P0 violation in gateway_e2e_suite_test.go









