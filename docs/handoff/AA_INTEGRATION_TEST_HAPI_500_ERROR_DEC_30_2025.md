# AIAnalysis Integration Tests - HAPI HTTP 500 Error Investigation

**Date**: December 30, 2025
**Status**: ⚠️ **BLOCKED - HAPI Returning HTTP 500**
**Test Results**: 41 Passed | **13 Failed**
**Related**: BR-HAPI-197 (Recovery Human Review)

---

## 🚨 **Problem Summary**

After migrating AIAnalysis integration tests from mock client to real HAPI service, **all recovery-related tests are failing** due to HAPI returning **HTTP 500** errors.

### **Error Message**
```
HolmesGPT-API error (HTTP 500): HolmesGPT-API recovery returned HTTP 500:
decode response: unexpected status code: 500
```

### **Impact**
- ❌ **BR-HAPI-197 tests FAIL**: Recovery human review tests timeout
- ❌ **Recovery integration tests FAIL**: All recovery endpoints return 500
- ✅ **Non-recovery tests PASS**: Investigation endpoint works correctly

---

## 📊 **Test Results**

### **Failed Tests** (13 total)
```
[FAIL] BR-HAPI-197: Recovery human review when no workflows match
[FAIL] BR-HAPI-197: Recovery human review when confidence is low
[FAIL] BR-HAPI-197: Recovery human review when not reproducible (likely)
[FAIL] Recovery Endpoint Integration - Multiple tests
[FAIL] AIAnalysis Full Reconciliation Integration - Recovery attempts
```

### **Passed Tests** (41 total)
- ✅ Investigation endpoint tests
- ✅ Non-recovery reconciliation tests
- ✅ Audit integration tests
- ✅ Metrics tests
- ✅ Error handling tests (non-recovery)

---

## 🔍 **Root Cause Analysis**

### **Hypothesis**
The real HAPI service at `http://localhost:18120` is returning HTTP 500 errors for **recovery endpoint** (`/api/v1/recovery/analyze`) but working correctly for **investigation endpoint** (`/api/v1/investigate`).

### **Evidence**
1. **Controller logs show HTTP 500**:
   ```
   HolmesGPT-API recovery returned HTTP 500: decode response: unexpected status code: 500
   ```

2. **Investigation tests pass** → Investigation endpoint works ✅

3. **All recovery tests fail** → Recovery endpoint is broken ❌

### **Potential Causes**
1. **HAPI recovery endpoint code error** (Python exception)
2. **OpenAPI spec mismatch** (regenerated spec doesn't match implementation)
3. **Missing dependencies** in recovery endpoint logic
4. **Mock response logic error** in `holmesgpt-api/src/mock_responses.py`

---

## 🔧 **Investigation Steps**

### **Step 1: Verify HAPI Service Status** ✅ DONE
```bash
# Start integration tests (HAPI auto-starts)
make test-integration-aianalysis

# During test run, check HAPI health
curl http://localhost:18120/health
```

### **Step 2: Test Recovery Endpoint Directly** ⏳ NEXT
```bash
# Test with MOCK_NO_WORKFLOW_FOUND signal type
curl -X POST http://localhost:18120/api/v1/recovery/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "incident_id": "test",
    "remediation_id": "test",
    "signal_type": "MOCK_NO_WORKFLOW_FOUND",
    "is_recovery_attempt": true,
    "recovery_attempt_number": 1
  }'
```

**Expected**: HTTP 200 with `needs_human_review=true`
**Actual**: HTTP 500 (needs investigation)

### **Step 3: Check HAPI Logs** ⏳ NEXT
```bash
# Check HAPI container logs for Python exceptions
podman logs aianalysis_hapi_1 2>&1 | grep -E "(500|ERROR|Exception)"
```

### **Step 4: Verify OpenAPI Spec** ⏳ PENDING
```bash
# Check if RecoveryResponse has required fields
cat holmesgpt-api/api/openapi.json | jq '.components.schemas.RecoveryResponse.properties | keys'

# Expected: should include "needs_human_review" and "human_review_reason"
```

---

## 📝 **Files Modified (Already Completed)**

### **Code Changes** ✅ COMPLETE
1. `test/integration/aianalysis/suite_test.go` - Uses real HAPI client
2. `test/integration/aianalysis/recovery_human_review_integration_test.go` - Removed mock configuration

### **Testing Strategy** ✅ COMPLIANT
- ✅ Unit Tests: Mocks for all dependencies
- ✅ Integration Tests: Real HAPI service (only LLM mocked inside HAPI)
- ✅ E2E Tests: Real HAPI service (only LLM mocked inside HAPI)

---

## 🎯 **Next Actions**

### **Priority 1: Diagnose HAPI HTTP 500** ⏳ IN PROGRESS
1. **Start integration tests** (HAPI auto-starts):
   ```bash
   make test-integration-aianalysis FOCUS="should handle validation errors" &
   ```

2. **While tests running, test HAPI directly**:
   ```bash
   curl -v -X POST http://localhost:18120/api/v1/recovery/analyze \
     -H "Content-Type: application/json" \
     -d '{"incident_id":"test","remediation_id":"test","signal_type":"MOCK_NO_WORKFLOW_FOUND","is_recovery_attempt":true,"recovery_attempt_number":1}'
   ```

3. **Check HAPI logs**:
   ```bash
   podman logs -f aianalysis_hapi_1 | grep -E "(500|ERROR|Exception|recovery)"
   ```

### **Priority 2: Fix HAPI Issue** ⏳ PENDING
- Depends on root cause found in Priority 1
- Likely fixes:
  - Fix Python exception in recovery endpoint
  - Update OpenAPI spec if fields are missing
  - Fix mock response logic in `mock_responses.py`

### **Priority 3: Re-run Tests** ⏳ PENDING
```bash
make test-integration-aianalysis FOCUS="BR-HAPI-197"
```

---

## 🔗 **Related Documents**

- **Migration Complete**: `docs/handoff/AA_INTEGRATION_TESTS_REAL_HAPI_DEC_30_2025.md`
- **HAPI Team Response**: `docs/shared/HAPI_RECOVERY_MOCK_EDGE_CASES_REQUEST.md`
- **BR-HAPI-197 Implementation**: `docs/handoff/AA_RECOVERY_HUMAN_REVIEW_COMPLETE_DEC_30_2025.md`
- **Testing Strategy**: `.cursor/rules/03-testing-strategy.mdc`

---

## ✅ **What's Working**

- ✅ Code compiles successfully
- ✅ Integration tests use real HAPI service (correct pattern)
- ✅ Investigation endpoint works (`/api/v1/investigate`)
- ✅ Non-recovery tests pass (41 passing tests)
- ✅ Infrastructure auto-starts correctly

## ❌ **What's Broken**

- ❌ Recovery endpoint returns HTTP 500 (`/api/v1/recovery/analyze`)
- ❌ All recovery-related tests fail (13 failures)
- ❌ BR-HAPI-197 tests cannot validate `needs_human_review` logic

---

**Confidence**: 90% that issue is in HAPI service, not AIAnalysis code
**Blocker**: HAPI recovery endpoint HTTP 500 error
**Next Action**: Diagnose HAPI logs to find Python exception causing HTTP 500

