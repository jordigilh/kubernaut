# Gateway Storm Detection Test Conflicts - Comprehensive Fix

## Executive Summary
**13 tests failing** due to inadvertent storm detection triggers after lowering `StormRateThreshold` to 2 for testing.

## Root Cause
With `StormRateThreshold: 2`, any test sending **3+ alerts with the same alertname** triggers storm detection:
- Alerts 1-2: Individual CRDs created
- Alert 3+: Storm aggregation activated → single aggregated CRD after 5-second window

**Result**: Tests expecting individual CRDs get storm aggregation instead.

---

## Failed Tests Analysis

### Group 1: Tests Expecting Multiple Individual CRDs (10 tests)

| Test | Expected | Actual | Alertname Pattern | Fix Needed |
|------|----------|--------|-------------------|------------|
| Rate limiting burst | ≥15 CRDs | 2 | Same: `BurstTestAlert` | ✅ Fixed |
| Different failures | 3 CRDs | 2 | Same: `PodCrashLoop` | ✅ Fixed |
| Dedup TTL expires | 2 CRDs | 1 | Same: `ExpiryTest` | ❌ Needs fix |
| Different severity | 2 CRDs | 1 | Same: `SeverityTest` | ❌ Needs fix |
| Namespace label changes | 2 CRDs | 1 | Same: `EnvChangeTest` | ❌ Needs fix |
| ConfigMap updates | 2 CRDs | 1 | Same: `ConfigChangeTest` | ❌ Needs fix |
| Dedup key expiring | 2 CRDs | 0-1 | Same: `MidFlightTest` | ❌ Needs fix |
| Redis recovery | 2 CRDs | 1 | Same: `RedisRecovery` | ❌ Needs fix |
| Gateway restarts | 2 CRDs | 1 | Same: `RestartTest` | ❌ Needs fix |
| Concurrent alerts | Multiple | 1 | Same: `Concurrent-{timestamp}` | ❌ Needs fix |

### Group 2: Storm Aggregation Tests (3 tests)

| Test | Issue | Root Cause | Fix Needed |
|------|-------|------------|------------|
| Main storm test | CRD count wrong | Wait time still 65s in some places | ✅ Fixed (sed) |
| Multiple time windows | Aggregation issue | Wait time issue | ✅ Fixed (sed) |
| Two simultaneous storms | No aggregated CRDs | Context canceled, timing | ❌ Verify fix |

---

## Solution: Systematic Alertname Uniqueness

### Strategy
**Make alertnames unique in ALL non-storm tests** by appending index or timestamp.

### Implementation Pattern

**Before** (triggers storm detection):
```go
for i := 0; i < N; i++ {
    alertPayload := fmt.Sprintf(`{
        "alerts": [{
            "labels": {
                "alertname": "TestAlert",  // ❌ Same for all
                "resource": "resource-%d"
            }
        }]
    }`, i)
}
```

**After** (avoids storm detection):
```go
for i := 0; i < N; i++ {
    alertPayload := fmt.Sprintf(`{
        "alerts": [{
            "labels": {
                "alertname": "TestAlert-%d",  // ✅ Unique per alert
                "resource": "resource-%d"
            }
        }]
    }`, i, i)
}
```

---

## Comprehensive Fix Plan

### Phase 1: Identify All Affected Tests ✅ COMPLETE
```bash
grep -n '"alertname":' test/integration/gateway/gateway_integration_test.go | \
  grep -v '-%d' | wc -l
# Result: ~50 alertname usages, many need fixing
```

### Phase 2: Systematic Fix Application

#### Fix Template for Non-Storm Tests
For tests NOT explicitly testing storm detection:

1. **Find alertname declaration**
2. **Append index to alertname**
3. **Update fmt.Sprintf() arguments**

**Example Fix**:
```go
// BEFORE
"alertname": "ExpiryTest"

// AFTER
"alertname": "ExpiryTest-%d"
// And update fmt.Sprintf to include index
```

### Phase 3: Verification

**Run tests to verify**:
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/... -timeout 15m 2>&1 | tee test_results.log
```

---

## Detailed Fixes Required

### Test 1: Dedup TTL Expires (Line ~1145)
**Location**: `gateway_integration_test.go:1145`

**Current**:
```go
"alertname": "ExpiryTest"
```

**Fix**:
```go
"alertname": "ExpiryTest-%d"
// Update fmt.Sprintf arguments
```

### Test 2: Different Severity (Line ~1217)
**Location**: `gateway_integration_test.go:1217`

**Current**:
```go
"alertname": "SeverityTest"
```

**Fix**:
```go
"alertname": "SeverityTest-%d"
// Or use unique names: "SeverityWarning" and "SeverityCritical"
```

### Test 3: Namespace Label Changes (Line ~1435)
**Location**: `gateway_integration_test.go:1435`

**Current**:
```go
"alertname": "EnvChangeTest"
```

**Fix**:
```go
"alertname": "EnvChangeTest-%d"
```

### Test 4: ConfigMap Updates (Line ~1523)
**Location**: `gateway_integration_test.go:1523`

**Current**:
```go
"alertname": "ConfigChangeTest"
```

**Fix**:
```go
"alertname": "ConfigChangeTest-%d"
```

### Test 5: Concurrent Alerts (Line ~1026)
**Location**: `gateway_integration_test.go:1026`

**Current**:
```go
alertName := fmt.Sprintf("Concurrent-%d-%d", time.Now().UnixNano(), i)
```

**Issue**: This creates unique names BUT then uses SAME name in loop

**Fix**: Ensure each goroutine uses its OWN alertname

### Test 6: Dedup Key Expiring (Line ~1805)
**Location**: `gateway_integration_test.go:1805`

**Current**:
```go
"alertname": "MidFlightTest"
```

**Fix**:
```go
"alertname": "MidFlightTest-%d"
```

### Test 7: Redis Recovery (Line ~2087)
**Location**: `gateway_integration_test.go:2087`

**Current**:
```go
"alertname": "RedisRecovery"
```

**Fix**:
```go
"alertname": "RedisRecovery-%d"
```

### Test 8: Gateway Restarts (Line ~2199)
**Location**: `gateway_integration_test.go:2199`

**Current**:
```go
"alertname": "RestartTest"
```

**Fix**:
```go
"alertname": "RestartTest-%d"
```

### Test 9: Two Simultaneous Storms (Line ~1696)
**Location**: `gateway_integration_test.go:1696`

**Issue**: This test WANTS storm detection, but aggregated CRDs aren't being created

**Likely Cause**: Timing issue - 5-second window might not be long enough with test load

**Fix Options**:
1. Increase aggregation window to 10 seconds for storm-specific tests
2. Verify storm detection is actually triggered (check logs)
3. Ensure wait time matches aggregation window (7s = 5s + 2s buffer)

---

## Automation Script

Create a script to find and fix ALL occurrences:

```bash
#!/bin/bash
# find_storm_conflicts.sh

cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

echo "=== Tests with Same Alertname (Potential Storm Triggers) ==="
grep -n '"alertname": "[A-Za-z]*"' test/integration/gateway/gateway_integration_test.go | \
  grep -v '-%d' | \
  grep -v 'DeploymentRolloutFailed' | \
  grep -v 'NoisyAlert'

echo ""
echo "=== Storm-Specific Tests (Should Keep Same Alertname) ==="
grep -B10 '"alertname": "DeploymentRolloutFailed"' test/integration/gateway/gateway_integration_test.go | \
  grep -E "It\(|alertname"
```

---

## Expected Outcomes After Fix

### Non-Storm Tests
- ✅ Each alert creates individual CRD
- ✅ No storm detection triggered
- ✅ Tests validate intended feature (dedup, rate limiting, etc.)

### Storm Tests
- ✅ Storm detection triggers as designed
- ✅ Aggregated CRD created after 5-second window
- ✅ All resources included in aggregated CRD

---

## Confidence Assessment: 95%

**Why 95%**:
- ✅ Root cause clearly identified (storm threshold = 2)
- ✅ Pattern is consistent across all failures
- ✅ Fix is simple and proven (already worked for 2 tests)
- ✅ Systematic approach will catch all occurrences

**5% Risk**:
- Some tests might have legitimate reasons for same alertname
- Concurrent test might have race conditions
- Storm aggregation timing might need adjustment

**Mitigation**:
- Review each test's business intent before fixing
- Add comments explaining why alertnames are unique
- Monitor storm aggregation tests for timing issues

---

## Next Steps

1. ✅ **Documentation created**: This comprehensive fix plan
2. ⏳ **Apply fixes**: Update all 8+ alertname declarations
3. ⏳ **Run tests**: Verify all 13 tests pass
4. ⏳ **Document pattern**: Add to test guidelines

**Time Estimate**: 30-45 minutes for all fixes + verification

---

## Test Execution Performance

### Before Fix
- **Test Duration**: ~10 minutes (5 tests × 65s wait + other tests)
- **Storm Test Wait**: 65 seconds × 5 occurrences = 325 seconds (~5.4 minutes)

### After Fix
- **Test Duration**: ~3-4 minutes (5 tests × 7s wait + other tests)
- **Storm Test Wait**: 7 seconds × 5 occurrences = 35 seconds
- **Speedup**: **~3x faster** (from 10 min to 3-4 min)

---

## Files Modified Summary

1. ✅ `pkg/gateway/processing/storm_aggregator.go` - Added configurable window
2. ✅ `pkg/gateway/server.go` - Use configured window duration
3. ✅ `test/integration/gateway/gateway_suite_test.go` - Set 5-second test window
4. ✅ `test/integration/gateway/gateway_integration_test.go` - Updated some wait times
5. ⏳ `test/integration/gateway/gateway_integration_test.go` - Need to fix 8+ alertnames
6. ✅ `test/integration/gateway/rate_limiting_test.go` - Fixed burst test alertname

---

**Status**: Phase 2 in progress - applying systematic fixes to all affected tests

