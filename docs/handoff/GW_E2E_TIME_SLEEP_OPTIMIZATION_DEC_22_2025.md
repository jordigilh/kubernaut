# Gateway E2E time.Sleep() Optimization Opportunities

**Date**: December 22, 2025
**Status**: ✅ **MAJOR OPTIMIZATION IDENTIFIED** - Can reduce test time by ~55 seconds
**Related**: GW_E2E_TIME_SLEEP_VIOLATIONS_TRIAGE_DEC_22_2025.md

---

## 📋 Executive Summary

**CRITICAL FINDING**: Test 14 waits **70 seconds** for TTL expiration, but E2E environment is configured with **10-second TTL**. This is a **85% time waste** (60 seconds unnecessary waiting).

**Additional Optimization**: Request staggering delays (100ms, 500ms) can potentially be reduced to 50ms for faster test execution.

**Total Time Savings**: ~55-60 seconds per full E2E suite run

---

## 🚨 CRITICAL OPTIMIZATION: TTL Wait Time (Test 14)

### Current Implementation (WASTEFUL)

**File**: `test/e2e/gateway/14_deduplication_ttl_expiration_test.go:190`

```go
testLogger.Info("Step 3: Wait for deduplication TTL to expire")
testLogger.Info("  Waiting 70 seconds for TTL expiration (configured TTL + buffer)...")
time.Sleep(70 * time.Second)  // ❌ WASTEFUL - E2E TTL is only 10s!
```

**Comment in test file (line 186-187)**:
```go
// Note: In E2E environment, the TTL is typically configured to be short (e.g., 5 seconds)
// for testing purposes. In production, this would be 5 minutes.
```

### Actual E2E Configuration (DISCOVERED)

**File**: `test/infrastructure/gateway.go:277`
```go
ttl: 10s  # Minimum allowed TTL (production: 5m)
```

**File**: `test/e2e/gateway/gateway-deployment.yaml:39`
```yaml
processing:
  deduplication:
    ttl: 10s  # Minimum allowed TTL (production: 5m)
```

**File**: `pkg/gateway/config/config.go:368`
```go
// Validation: TTL must be >= 10 seconds
if c.Processing.Deduplication.TTL > 0 && c.Processing.Deduplication.TTL < 10*time.Second {
    return NewValidationError(
        "processing.deduplication.ttl",
        c.Processing.Deduplication.TTL.String(),
        "Must be at least 10 seconds",
        "Minimum TTL of 10s ensures reliable deduplication with clock skew tolerance")
}
```

---

### Optimization Recommendation

**FIX**: Reduce wait time from 70s to 15s (10s TTL + 5s buffer)

```go
// ✅ OPTIMIZED: Match actual E2E configuration
testLogger.Info("Step 3: Wait for deduplication TTL to expire")
testLogger.Info("  Waiting 15 seconds for TTL expiration (10s TTL + 5s buffer)...")
time.Sleep(15 * time.Second)  // E2E environment uses 10s TTL (see gateway-deployment.yaml)
```

**Impact**:
- **Time Saved**: 55 seconds per test run (78% reduction)
- **Test Duration**: 70s → 15s (21% of original time)
- **Risk**: NONE - Still provides 50% buffer over actual TTL

---

## 💡 MINOR OPTIMIZATION: Request Staggering Delays

### Current Implementation

**9 instances** across 4 test files using 100ms or 500ms delays:

| Test File | Line | Current Delay | Purpose |
|---|---|---|---|
| `11_fingerprint_stability_test.go` | 349 | 100ms | Deduplication scenario |
| `13_redis_failure_graceful_degradation_test.go` | 174 | 100ms | Pre-failure burst |
| `13_redis_failure_graceful_degradation_test.go` | 263 | 500ms | During-failure burst |
| `14_deduplication_ttl_expiration_test.go` | 153 | 100ms | Pre-TTL alerts |
| `14_deduplication_ttl_expiration_test.go` | 234 | 100ms | Post-TTL alerts |
| `12_gateway_restart_recovery_test.go` | 153 | 100ms | Pre-restart alerts |
| `12_gateway_restart_recovery_test.go` | 232 | 100ms | Post-restart alerts |

**Total staggering time** (per full suite run): ~2-3 seconds

---

### Optimization Analysis

#### Option A: Reduce to 50ms (RECOMMENDED)

**Rationale**:
- Gateway processing time is typically <10ms for alert ingestion
- 50ms provides 5x safety margin
- Still creates clear temporal separation for deduplication testing
- Reduces total staggering time by 50% (~1-1.5s savings)

**Example**:
```go
for i := 0; i < alertCount; i++ {
    resp, err := httpClient.Post(/*...*/)
    // ...
    time.Sleep(50 * time.Millisecond)  // ✅ OPTIMIZED - still adequate separation
}
```

**Impact**:
- **Time Saved**: ~1-1.5 seconds per full suite run
- **Risk**: LOW - 50ms still provides ample separation for deduplication

---

#### Option B: Make Configurable (FUTURE)

**Implementation**:
```go
// test/e2e/gateway/test_config.go
const (
    // AlertStaggerDelayMS controls delay between staggered alerts in E2E tests
    // Default: 50ms (adequate for local/CI, can be increased for slow systems)
    AlertStaggerDelayMS = 50
)

// Usage in tests
time.Sleep(AlertStaggerDelayMS * time.Millisecond)
```

**Benefits**:
- Centralized control over staggering delays
- Can be adjusted via environment variable if needed
- Self-documenting test behavior

**Impact**:
- **Time Saved**: Same as Option A
- **Flexibility**: Can be tuned for slow CI systems

---

#### Option C: No Change (CONSERVATIVE)

**Rationale**:
- Current delays are already small (100-500ms)
- Minimal impact on overall test duration
- Risk-free - known to work reliably

**Impact**:
- **Time Saved**: 0 seconds
- **Risk**: NONE

---

## 📊 Optimization Impact Summary

| Optimization | Files Affected | Time Saved | Risk Level | Recommendation |
|---|---|---|---|---|
| **TTL Wait Reduction** | 1 | **~55s** | NONE | ✅ **DO IT NOW** |
| **Staggering Reduction** | 4 | ~1-1.5s | LOW | ✅ Recommended |
| **Make Configurable** | 5 | ~1-1.5s | LOW | 💡 Future Enhancement |

**Total Potential Savings**: ~56-57 seconds per full E2E suite run (70s → 15s + 1.5s)

---

## 🔧 Implementation Plan

### Phase 1: Critical Optimization (IMMEDIATE)

**Fix Test 14 TTL Wait Time**

```go
// File: test/e2e/gateway/14_deduplication_ttl_expiration_test.go

// BEFORE (line 186-190)
// Note: In E2E environment, the TTL is typically configured to be short (e.g., 5 seconds)
// for testing purposes. In production, this would be 5 minutes.
testLogger.Info("Step 3: Wait for deduplication TTL to expire")
testLogger.Info("  Waiting 70 seconds for TTL expiration (configured TTL + buffer)...")
time.Sleep(70 * time.Second)

// AFTER (OPTIMIZED)
// Note: E2E environment uses 10s TTL (see test/e2e/gateway/gateway-deployment.yaml)
// Production uses 5m TTL. This test validates TTL expiration behavior.
testLogger.Info("Step 3: Wait for deduplication TTL to expire")
testLogger.Info("  Waiting 15 seconds for TTL expiration (10s E2E TTL + 5s buffer)...")
time.Sleep(15 * time.Second)  // E2E TTL is 10s (minimum allowed per config validation)
```

**Validation**:
- [ ] Update comment to reflect actual E2E TTL (10s)
- [ ] Reduce wait time from 70s to 15s
- [ ] Run Test 14 in isolation to verify TTL expiration still works
- [ ] Run full E2E suite to check for regressions

---

### Phase 2: Minor Optimization (RECOMMENDED)

**Reduce Request Staggering Delays**

```bash
# Create script to update all staggering delays
cat > /tmp/optimize_staggering.sh << 'EOF'
#!/bin/bash
# Reduce 100ms staggering delays to 50ms across E2E tests

cd test/e2e/gateway

# Files to update
for file in 11_fingerprint_stability_test.go \
            12_gateway_restart_recovery_test.go \
            13_redis_failure_graceful_degradation_test.go \
            14_deduplication_ttl_expiration_test.go; do

    if [ -f "$file" ]; then
        # Replace 100ms with 50ms (staggering delays only)
        sed -i.bak 's/time\.Sleep(100 \* time\.Millisecond).*\/\/ .*stagger/time.Sleep(50 * time.Millisecond)  \/\/ Optimized stagger delay/' "$file"

        echo "✅ Updated $file"
    fi
done

echo "✅ Staggering delays optimized"
EOF

chmod +x /tmp/optimize_staggering.sh
```

**Validation**:
- [ ] Reduce 100ms → 50ms in 7 locations
- [ ] Keep 500ms in Test 13 (during-failure scenario - higher backpressure)
- [ ] Run affected tests individually
- [ ] Run full E2E suite to verify no flakiness

---

### Phase 3: Make Configurable (FUTURE)

**Centralize Test Timing Constants**

```go
// File: test/e2e/gateway/test_config.go (NEW)

package gateway

import (
    "os"
    "strconv"
    "time"
)

// E2E Test Configuration Constants
const (
    // DefaultAlertStaggerDelayMS is the default delay between staggered alerts
    // Can be overridden via E2E_ALERT_STAGGER_MS environment variable
    DefaultAlertStaggerDelayMS = 50

    // DefaultTTLBufferSeconds is the buffer added to TTL for expiration tests
    // E2E TTL is 10s (see gateway-deployment.yaml), buffer ensures reliable expiration
    DefaultTTLBufferSeconds = 5
)

// GetAlertStaggerDelay returns the configured stagger delay
func GetAlertStaggerDelay() time.Duration {
    if delayStr := os.Getenv("E2E_ALERT_STAGGER_MS"); delayStr != "" {
        if delayMS, err := strconv.Atoi(delayStr); err == nil {
            return time.Duration(delayMS) * time.Millisecond
        }
    }
    return DefaultAlertStaggerDelayMS * time.Millisecond
}

// GetTTLExpirationWait returns the total wait time for TTL expiration
// E2E TTL (10s) + buffer (5s) = 15s total
func GetTTLExpirationWait() time.Duration {
    return 10*time.Second + DefaultTTLBufferSeconds*time.Second
}
```

---

## ✅ Success Criteria

**Phase 1 Complete when**:
- ✅ Test 14 wait time reduced from 70s to 15s
- ✅ Test 14 passes consistently (10+ runs)
- ✅ Full E2E suite passes
- ✅ Test execution time reduced by ~55 seconds

**Phase 2 Complete when**:
- ✅ All 7 staggering delays reduced from 100ms to 50ms
- ✅ All affected tests pass consistently
- ✅ No new flakiness introduced
- ✅ Test execution time reduced by additional ~1-1.5 seconds

**Phase 3 Complete when**:
- ✅ Timing constants centralized in test_config.go
- ✅ Environment variable override implemented
- ✅ All tests use centralized configuration
- ✅ Documentation updated

---

## 📊 Before/After Comparison

| Metric | Before | After (Phase 1) | After (Phase 2) |
|---|---|---|---|
| **Test 14 TTL Wait** | 70s | 15s | 15s |
| **Total Staggering Time** | ~2-3s | ~2-3s | ~1-1.5s |
| **Total Time Savings** | - | ~55s | ~56-57s |
| **Full Suite Duration** | ~10-15min | ~9-14min | ~8.5-13.5min |

---

## 🎯 Recommendation

### Immediate Action (TODAY)

1. **Fix Test 14 TTL wait time**: 70s → 15s
   - **Priority**: P0 (CRITICAL)
   - **Impact**: Massive (55s savings, 78% reduction)
   - **Risk**: NONE
   - **Effort**: 2 minutes

### Follow-up (THIS WEEK)

2. **Optimize staggering delays**: 100ms → 50ms
   - **Priority**: P2 (NICE TO HAVE)
   - **Impact**: Minor (~1.5s savings)
   - **Risk**: LOW
   - **Effort**: 10 minutes

### Future Enhancement (V1.1)

3. **Centralize timing configuration**
   - **Priority**: P3 (FUTURE)
   - **Impact**: Maintainability, flexibility
   - **Risk**: NONE
   - **Effort**: 30 minutes

---

## 📚 References

- **TTL Configuration**: `pkg/gateway/config/config.go:297-299` (`GATEWAY_DEDUP_TTL`)
- **E2E TTL Setting**: `test/infrastructure/gateway.go:277`, `test/e2e/gateway/gateway-deployment.yaml:39`
- **TTL Validation**: `pkg/gateway/config/config.go:368-378` (minimum 10s)
- **Related Triage**: GW_E2E_TIME_SLEEP_VIOLATIONS_TRIAGE_DEC_22_2025.md

---

**Generated**: 2025-12-22
**Status**: ✅ OPTIMIZATION IDENTIFIED - READY TO IMPLEMENT
**Next Action**: Fix Test 14 TTL wait time (70s → 15s)









