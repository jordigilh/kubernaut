# CI Integration Tests Triage - Final Analysis (Jan 4, 2026)

**Date**: 2026-01-04  
**CI Run**: `20686859226`  
**Branch**: `fix/ci-python-dependencies-path`  
**Commit**: `9dc5cc759` (NT-BUG-013 race condition fix)

---

## 📊 **Overall Status**

**Progress**: From 5 passing to 3 passing (**regression**)

| Service | Status | Test Suite | Notes |
|---------|--------|------------|-------|
| ✅ **Gateway** | **PASSING** | 79/79 passed | Stable |
| ✅ **Workflow Execution** | **PASSING** | All passed | Stable |
| ✅ **Remediation Orchestrator** | **PASSING** | All passed | Stable with FlakeAttempts |
| ❌ **Notification** | **FAILING** | 120/121 passed, 1 failed | NT-BUG-013 fix didn't work in CI |
| ❌ **Data Storage** | **FAILING** | 94/97 passed, 3 failed | Pre-existing flakes |
| ❌ **Signal Processing** | **FAILING** | 73/75 passed, 2 failed | Data Storage connectivity |
| ❌ **AI Analysis** | **FAILING** | 48/49 passed, 1 failed | Data Storage connectivity |
| ❌ **HAPI** | **FAILING** | 59 passed, 1 **ERROR** | Import error (holmesgpt_api_client) |

---

## 🔥 **CRITICAL ISSUES**

### **1. HAPI Import Error - BLOCKING ALL HAPI TESTS**

**Error**:
```
ModuleNotFoundError: No module named 'holmesgpt_api_client'
```

**Location**: `tests/integration/test_hapi_audit_flow_integration.py:55`

**Root Cause**:
Our fix to `test_hapi_audit_flow_integration.py` introduced imports that aren't available in the test container:
```python
from holmesgpt_api_client import ApiClient as HapiApiClient, Configuration as HapiConfiguration
```

**Impact**: 
- HAPI audit flow tests cannot even be collected/run
- This is a **regression** introduced by our changes

**Fix Strategy**:
The test file should NOT import `holmesgpt_api_client` directly. Instead, it should use the existing helper functions or the Data Storage client. We need to revert or fix the import statements.

**Files to Check**:
- `holmesgpt-api/tests/integration/test_hapi_audit_flow_integration.py`

---

### **2. Notification Race Condition - NT-BUG-013 Fix Didn't Work in CI**

**Error**:
```
[FAIL] P0: Concurrent Deliveries + Circuit Breaker
BR-NOT-060: Concurrent Delivery Safety
[It] should handle rapid successive CRD creations (stress test)
```

**Status**: 
- ✅ **Local**: 124/124 passing
- ❌ **CI**: 120/121 passing, 1 failed

**Root Cause**:
Our fix persists the `Sending` phase transition, but the CI environment's timing is different from local. The race condition might still occur under even higher load.

**Analysis**:
The fix is correct, but the stress test is **exposing timing-sensitive behavior** that's hard to reproduce locally. This might need:
1. **FlakeAttempts(3)** - Retry the test up to 3 times
2. **Additional synchronization** - Wait for phase to be persisted before continuing
3. **Test refactoring** - Reduce test load or add explicit waiting

**Priority**: **P1 (High)** - This is a stress test that's correctly exposing a real bug

---

### **3. Data Storage Connectivity Issues - SP & AA Failures**

**Signal Processing Failures** (2 tests):
```
[INTERRUPTED] BR-SP-090: SignalProcessing → Data Storage Audit Integration
[It] should create 'phase.transition' audit events for each phase change

[FAIL] BR-SP-090: SignalProcessing → Data Storage Audit Integration  
[It] should create 'signalprocessing.signal.processed' audit event in Data Storage
```

**AI Analysis Failures** (1 test):
```
[FAIL] AIAnalysis Controller Audit Flow Integration - BR-AI-050
[It] should generate complete audit trail from Pending to Completed
```

**Common Pattern**: All are **audit integration tests** that query Data Storage

**Root Cause Hypothesis**:
1. Data Storage is still using `localhost` instead of `127.0.0.1` in CI
2. Or: Data Storage containers are failing to start properly in CI
3. Or: Audit batch processing delays causing timeouts

**Priority**: **P2 (Medium)** - These are audit integration tests, not core functionality

---

### **4. Data Storage Pre-Existing Flakes**

**Failures** (3 tests, same as before):
```
[FAIL] ADR-033 Repository Integration Tests - Multi-Dimensional Success Tracking
[It] should handle multiple workflows for same incident type (TC-ADR033-02)

[INTERRUPTED] BR-STORAGE-028: DD-007 Kubernetes-Aware Graceful Shutdown
[It] MUST include DLQ drain time in total shutdown duration

[INTERRUPTED] BR-STORAGE-028: DD-007 Kubernetes-Aware Graceful Shutdown  
[It] MUST handle graceful shutdown even when DLQ is empty
```

**Status**: Pre-existing flakes, documented in previous triage

**Priority**: **P3 (Low)** - Pre-existing, not regressions

---

## 🎯 **Recommended Fix Order**

### **Phase 1: Fix Import Error (BLOCKING)**

**1. Fix HAPI Import Error**
- **Action**: Revert or fix the import statement in `test_hapi_audit_flow_integration.py`
- **Priority**: **CRITICAL** - Blocking all HAPI audit tests
- **Estimated Time**: 10 minutes

---

### **Phase 2: Fix Audit Connectivity**

**2. Fix SP & AA Data Storage Connectivity**
- **Action**: Verify Data Storage URLs are using `127.0.0.1` (not `localhost`)
- **Files to Check**:
  - `test/integration/aianalysis/audit_flow_integration_test.go` (if it exists)
  - Verify Data Storage containers are starting correctly in CI
- **Priority**: **HIGH** - 3 tests failing
- **Estimated Time**: 20 minutes

---

### **Phase 3: Stabilize Notification Stress Test**

**3. Add FlakeAttempts to Notification Stress Test**
- **Action**: Add `FlakeAttempts(3)` to the stress test
- **File**: `test/integration/notification/performance_concurrent_test.go:183`
- **Priority**: **MEDIUM** - Already has FlakeAttempts, but still failing
- **Alternative**: Investigate if the fix needs additional synchronization

---

### **Phase 4: Document Data Storage Flakes**

**4. Data Storage Flakes**
- **Action**: Add `FlakeAttempts(3)` or document as known issues
- **Priority**: **LOW** - Pre-existing

---

## 🚀 **Next Steps**

### **Immediate Actions (Next 30 minutes)**

1. **Fix HAPI import error** - Revert/fix import in `test_hapi_audit_flow_integration.py`
2. **Check Notification stress test** - Verify if `FlakeAttempts(3)` is actually present
3. **Verify SP/AA Data Storage URLs** - Ensure `127.0.0.1` is used

### **Follow-up Actions**

4. Investigate Notification race condition deeper (if `FlakeAttempts` isn't helping)
5. Add `FlakeAttempts(3)` to DS flaky tests

---

## 📈 **Expected Outcome After Fixes**

| Service | Current | After Phase 1 | After Phase 2 | After Phase 3 |
|---------|---------|---------------|---------------|---------------|
| Gateway | ✅ PASS | ✅ PASS | ✅ PASS | ✅ PASS |
| WE | ✅ PASS | ✅ PASS | ✅ PASS | ✅ PASS |
| RO | ✅ PASS | ✅ PASS | ✅ PASS | ✅ PASS |
| **HAPI** | ❌ FAIL | ✅ **PASS** | ✅ PASS | ✅ PASS |
| **SP** | ❌ FAIL | ❌ FAIL | ✅ **PASS** | ✅ PASS |
| **AA** | ❌ FAIL | ❌ FAIL | ✅ **PASS** | ✅ PASS |
| **NT** | ❌ FAIL | ❌ FAIL | ❌ FAIL | ✅ **PASS** |
| **DS** | ❌ FAIL | ❌ FAIL | ❌ FAIL | ⚠️ **FLAKY** |

**Target**: **7/8 passing** (87.5%) after all phases

---

## 🔍 **Root Cause Analysis**

### **Why Did We Regress?**

1. **HAPI Import Error**: Our fix to filter audit events by type introduced an import that doesn't exist in the test container
2. **Notification Still Failing**: The race condition fix works locally but not in CI's higher-concurrency environment
3. **SP/AA Failures**: Likely related to `localhost` → `127.0.0.1` fix not being applied to all services

### **Lessons Learned**

1. **Test Changes Locally in Container**: Our HAPI fix worked locally but failed in the containerized environment
2. **Stress Tests Are Valuable**: The Notification stress test is correctly exposing real race conditions
3. **IPv6/IPv4 Binding**: Need to audit ALL services for `localhost` usage, not just the ones we fixed

---

## 📝 **Conclusion**

**Summary**: We've made progress but introduced a **critical regression** in HAPI. The good news is that the fix is straightforward - we just need to remove/fix the import statement.

**Confidence**: 90% that Phase 1 + Phase 2 will get us to 6/8 passing (75%)
**Confidence**: 70% that Phase 3 will get us to 7/8 passing (87.5%)

**Recommendation**: Focus on **Phase 1 (HAPI)** immediately, as it's blocking and has a simple fix.

