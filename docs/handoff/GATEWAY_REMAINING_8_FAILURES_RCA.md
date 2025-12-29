# ⚠️ SUPERSEDED - See GATEWAY_SERVICE_HANDOFF.md

**This document has been superseded by [`GATEWAY_SERVICE_HANDOFF.md`](./GATEWAY_SERVICE_HANDOFF.md)**

**Superseded Date**: December 13, 2025
**Reason**: Consolidated into comprehensive service handoff document

---

# Gateway Remaining 8 Test Failures - Root Cause Analysis

**Date**: 2025-12-12 (Night)
**Status**: 🔴 **8/99 Tests Failing** (92% Pass Rate)
**Improvement**: ✅ Fixed 1 test (was 9 failing, now 8)
**Priority**: 🔴 HIGH - v1.0 Blocker

---

## 📊 **Test Results Summary**

### **Pass Rate Progress**:
- **Before Tonight**: 90/99 (91%)
- **After Infra Fix**: 91/99 (92%) ✅ +1%
- **Target**: 99/99 (100%)

### **Failing Test Categories**:
1. ❌ **Audit Integration** (3 tests) - Data Storage connectivity
2. ❌ **Storm Detection** (1 test) - Storm aggregation logic
3. ❌ **Phase State Handling** (2 tests) - Cancelled/Unknown state logic
4. ❌ **Concurrent Load** (1 test) - Rate limiter/capacity
5. ❌ **Storm Metrics** (1 test) - Observability validation

---

## 🔍 **Failure Analysis**

### **FAILURE GROUP 1: Audit Integration (3 tests)** 🔴 CRITICAL

#### **Test 1**: `audit_integration_test.go:197` - signal.received
**Error**: `Timed out after 10.001s. Expected <int>: 0 to be >= <int>: 1`

#### **Test 2**: `audit_integration_test.go:296` - signal.deduplicated
#### **Test 3**: `audit_integration_test.go:383` - storm.detected

**Common Root Cause**: Audit events not reaching Data Storage

**Evidence from Logs**:
```
{"level":"error","ts":1765507586.479978,"logger":"audit-store","caller":"audit/store.go:364",
"msg":"Failed to write audit batch","attempt":1,"batch_size":1,
"error":"network error: Post \"http://localhost:8080/api/v1/audit/events/batch\":
dial tcp [::1]:8080: connect: connection refused"}
```

**Analysis**:
- ❌ Audit store is STILL trying to connect to `localhost:8080`
- ❌ Despite fixing `createTestGatewayServer()`, some code path is still using hardcoded URL
- ⚠️  Tests expect audit events in Data Storage, but events are being dropped

**Hypothesis**:
The issue is that `StartTestGateway()` passes `dataStorageURL` correctly, but somewhere in the Gateway server initialization, the config is being overridden or not passed through to the audit store.

**Fix Strategy**:
1. Trace config flow from `StartTestGateway()` → `gateway.NewServerWithK8sClient()` → `audit.NewBufferedStore()`
2. Verify `cfg.Infrastructure.DataStorageURL` is correctly set
3. Check if audit store client is using correct URL

**Priority**: 🔴 **CRITICAL** - These are P0 compliance tests

**Estimated Fix Time**: 30-60 minutes

---

### **FAILURE GROUP 2: Storm Detection (1 test)** 🟡 MEDIUM

#### **Test**: `webhook_integration_test.go` - "aggregates multiple related alerts into single storm CRD"

**Error**: Not shown in summary, need detailed logs

**Hypothesis**:
- Storm aggregation window timing issue
- Async status update race condition
- Storm threshold not being met in test timing

**Fix Strategy**:
1. Review storm detection logic in `pkg/gateway/processing/storm_detector.go`
2. Check test expectations vs actual storm window timing
3. May need to increase wait time or adjust test thresholds

**Priority**: 🟡 **MEDIUM** - Feature test, not blocking core functionality

**Estimated Fix Time**: 1-2 hours

---

### **FAILURE GROUP 3: Phase State Handling (2 tests)** 🟡 MEDIUM

#### **Test 1**: `deduplication_state_test.go:483` - Cancelled state
**Expected**: Should treat Cancelled as new incident (retry remediation)

#### **Test 2**: `deduplication_state_test.go:556` - Unknown state
**Expected**: Should treat unknown state as duplicate (conservative fail-safe)

**Root Cause**: `IsTerminalPhase()` logic not handling these edge cases

**Current Implementation** (`phase_checker.go`):
```go
func IsTerminalPhase(phase remediationv1alpha1.RemediationPhase) bool {
    switch phase {
    case remediationv1alpha1.PhaseCompleted,
         remediationv1alpha1.PhaseFailed,
         remediationv1alpha1.PhaseTimedOut,
         remediationv1alpha1.PhaseSkipped:
        return true
    default:
        return false
    }
}
```

**Issue**:
- ❌ `Cancelled` is NOT listed as terminal
- ❌ Unknown states return `false` (not terminal)

**Business Logic Question**:
- Should `Cancelled` be terminal? (Test expects: NO - should retry)
- Should unknown states be terminal? (Test expects: NO - should deduplicate)

**Fix Strategy**:
1. Review DD-GATEWAY-009 and DD-GATEWAY-011 for phase handling rules
2. Update `IsTerminalPhase()` or add separate logic for these cases
3. May need to add `IsDuplicateAllowed()` method for more nuanced logic

**Priority**: 🟡 **MEDIUM** - Edge case handling

**Estimated Fix Time**: 1-2 hours

---

### **FAILURE GROUP 4: Concurrent Load (1 test)** 🟡 MEDIUM

#### **Test**: `graceful_shutdown_foundation_test.go` - "50 concurrent requests"

**Error**: Not shown in summary, need detailed logs

**Evidence from Earlier Logs**:
```
I1211 21:46:19.680094   75070 request.go:752] "Waited before sending request"
delay="1.200354667s" reason="client-side throttling, not priority and fairness"
```

**Root Cause**: K8s client rate limiting causing delays

**Analysis**:
- envtest K8s API has rate limiter enabled
- 50 concurrent requests hitting rate limit
- Tests timing out or experiencing throttling

**Fix Strategy**:
1. Check if rate limiter is disabled in test setup (suite_test.go)
2. Increase rate limiter settings: `k8sConfig.QPS = 1000, Burst = 2000`
3. OR: Reduce concurrent request count to 20-30 for tests
4. OR: Add retry logic with backoff

**Priority**: 🟡 **MEDIUM** - Performance test

**Estimated Fix Time**: 30-60 minutes

---

### **FAILURE GROUP 5: Storm Metrics (1 test)** 🟡 MEDIUM

#### **Test**: `observability_test.go:298` - Storm detection metric

**Error**: Not shown in summary, need detailed logs

**Hypothesis**:
- Storm not being detected in test (threshold not met)
- Metric not being recorded
- Timing issue - metric checked before storm detected

**Fix Strategy**:
1. Verify storm detection is actually triggered
2. Check metric registration in `pkg/gateway/metrics/metrics.go`
3. Add synchronization - wait for storm detection before checking metric
4. May need to adjust test thresholds to ensure storm is triggered

**Priority**: 🟡 **MEDIUM** - Observability test

**Estimated Fix Time**: 1 hour

---

## 🎯 **Fix Priority & Sequence**

### **Priority 1: Audit Integration** 🔴 CRITICAL
**Why**: P0 compliance requirement, blocks 3 tests
**Fix Time**: 30-60 min
**Impact**: High - audit trail is business critical

### **Priority 2: Concurrent Load** 🟡 HIGH
**Why**: Quick fix, unblocks graceful shutdown testing
**Fix Time**: 30-60 min
**Impact**: Medium - performance validation

### **Priority 3: Phase State Handling** 🟡 MEDIUM
**Why**: Edge case logic, needs design review
**Fix Time**: 1-2 hours
**Impact**: Medium - edge case coverage

### **Priority 4: Storm Detection** 🟡 MEDIUM
**Why**: Complex timing issue
**Fix Time**: 1-2 hours
**Impact**: Low - feature test

### **Priority 5: Storm Metrics** 🟡 MEDIUM
**Why**: Depends on storm detection fix
**Fix Time**: 1 hour
**Impact**: Low - observability validation

---

## 📋 **Detailed Investigation Needed**

### **For Audit Tests**:
```bash
# Check audit store initialization
grep -A 20 "NewBufferedStore" pkg/gateway/server.go

# Check config flow
grep -A 10 "DataStorageURL" pkg/gateway/server.go pkg/gateway/config/config.go

# Check if URL is being overridden
grep "localhost:8080" pkg/gateway/*.go pkg/audit/*.go
```

### **For Storm Detection**:
```bash
# Check storm detection logs
grep "storm" /tmp/gateway-test-after-infra-fix.log | head -50

# Check storm window timing
grep "AggregationWindow" test/integration/gateway/*.go
```

### **For Phase State**:
```bash
# Review phase handling rules
cat docs/architecture/decisions/DD-GATEWAY-009*.md
cat docs/architecture/decisions/DD-GATEWAY-011*.md

# Check current phase logic
cat pkg/gateway/processing/phase_checker.go
```

---

## 💡 **Quick Wins vs. Complex Fixes**

### **Quick Wins** (Total: 2-3 hours):
1. ✅ Audit Integration - Config tracing (30-60 min)
2. ✅ Concurrent Load - Rate limiter adjustment (30-60 min)
3. ✅ Storm Metrics - Add wait logic (1 hour)

### **Complex Fixes** (Total: 2-4 hours):
1. ⏳ Phase State Handling - Design review + logic update (1-2 hours)
2. ⏳ Storm Detection - Timing/threshold tuning (1-2 hours)

---

## 🎯 **Target Outcomes**

### **By Next Test Run**:
- ✅ Audit tests: 3/3 passing (91 → 94 total)
- ✅ Concurrent load: 1/1 passing (94 → 95 total)
- ✅ Storm metrics: 1/1 passing (95 → 96 total)
- ⏳ Phase state: 0-2/2 passing (96 → 96-98 total)
- ⏳ Storm detection: 0-1/1 passing (96-98 → 96-99 total)

### **Best Case**: 99/99 (100%)
### **Likely Case**: 96-98/99 (97-99%)
### **Acceptable Case**: 94/99 (95%)

---

## 🚨 **Risk Assessment**

### **Low Risk Fixes**:
- ✅ Audit integration (config issue)
- ✅ Concurrent load (rate limiter)

### **Medium Risk Fixes**:
- ⚠️  Phase state handling (may need business logic clarification)
- ⚠️  Storm metrics (dependency on storm detection)

### **High Risk Fixes**:
- 🔴 Storm detection (complex timing, may be flaky)

---

## 📝 **Next Actions**

1. ⏳ **Investigate audit store URL** - Trace config flow
2. ⏳ **Fix rate limiter** - Increase QPS/Burst in test setup
3. ⏳ **Review phase logic** - Check DD documents
4. ⏳ **Analyze storm logs** - Understand timing issues
5. ⏳ **Implement fixes** - Priority order
6. ⏳ **Re-run tests** - Validate improvements
7. ⏳ **Document results** - Final status report

---

**Created**: 2025-12-12 ~10:15 PM
**Target**: Fix 5-8 tests by morning
**Confidence**: 85% (High confidence for audit/load, medium for others)
**Next Update**: After Priority 1 fix (audit integration)






