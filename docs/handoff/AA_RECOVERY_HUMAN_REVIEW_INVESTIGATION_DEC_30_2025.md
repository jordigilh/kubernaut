# AIAnalysis Recovery Human Review - Investigation Summary

**Date**: December 30, 2025
**Status**: ⚠️ **BLOCKED - Requires Investigation**
**Priority**: P0 - Blocks BR-HAPI-197 completion

---

## 🎯 **Objective**

Implement BR-HAPI-197: AIAnalysis should handle `needs_human_review=true` from HAPI recovery responses and transition to `Failed` phase with appropriate `Reason` and `SubReason`.

---

## ✅ **What We've Completed**

### 1. **HAPI Team Delivered (December 30, 2025)**
- ✅ Updated `holmesgpt-api/api/openapi.json` to include:
  - `needs_human_review` (boolean, default: false)
  - `human_review_reason` (string, nullable) with enum values
- ✅ Confirmed mock responses in `holmesgpt-api/src/mock_responses.py` correctly set fields for edge cases:
  - `MOCK_NO_WORKFLOW_FOUND` → `needs_human_review=true`, `human_review_reason="no_matching_workflows"`
  - `MOCK_LOW_CONFIDENCE` → `needs_human_review=true`, `human_review_reason="low_confidence"`
  - `MOCK_NOT_REPRODUCIBLE` → `can_recover=false`

### 2. **AA Team Completed**
- ✅ Regenerated Go client: `make generate-holmesgpt-client`
  - Confirmed `RecoveryResponse` now has:
    - `NeedsHumanReview OptBool`
    - `HumanReviewReason OptNilString`
- ✅ Implemented AA service logic in `pkg/aianalysis/handlers/response_processor.go`:
  - Added `needsHumanReview := GetOptBoolValue(resp.NeedsHumanReview)` check
  - Added `handleWorkflowResolutionFailureFromRecovery()` method
  - Maps `human_review_reason` enum to `SubReason`
- ✅ Created integration tests in `test/integration/aianalysis/recovery_human_review_integration_test.go`:
  - 3 test cases for different human review scenarios
  - 1 test case for normal recovery flow
- ✅ Created E2E test in `test/e2e/aianalysis/04_recovery_flow_test.go`:
  - Full CRD lifecycle validation for recovery human review

---

## ❌ **Current Problem**

### **Symptom**
Integration tests fail because AIAnalysis transitions to `Completed` instead of `Failed` when HAPI returns `needs_human_review=true`.

### **Test Failures**
```
[FAIL] BR-HAPI-197: Recovery human review when no workflows match
  Expected Phase: Failed
  Actual Phase: Completed

[FAIL] BR-HAPI-197: Recovery human review when confidence is low
  Expected Phase: Failed
  Actual Phase: Completed

[FAIL] BR-HAPI-197: Normal recovery flow without human review
  (Also failing - needs investigation)
```

### **Controller Logs Show**
```
Processing successful recovery response
  canRecover: true
  confidence: 0.8
  hasSelectedWorkflow: true
  needsHumanReview: false  ← WRONG! Should be true
```

---

## 🔍 **Investigation Results**

### **What We've Verified**

#### ✅ **HAPI Mock Responses Are Correct**
```bash
$ curl -X POST http://localhost:18120/api/v1/recovery/analyze \
  -H "Content-Type: application/json" \
  -d '{"incident_id": "test", "remediation_id": "test", "signal_type": "MOCK_NO_WORKFLOW_FOUND", ...}'

{
  "needs_human_review": true,           ← ✅ CORRECT
  "human_review_reason": "no_matching_workflows",  ← ✅ CORRECT
  "selected_workflow": null,
  "analysis_confidence": 0.0
}
```

#### ✅ **Go Client Can Deserialize Correctly**
```bash
$ go run /tmp/test_json_unmarshal.go
NeedsHumanReview.Set: true      ← ✅ CORRECT
NeedsHumanReview.Value: true    ← ✅ CORRECT
HumanReviewReason.Set: true     ← ✅ CORRECT
HumanReviewReason.Value: no_matching_workflows  ← ✅ CORRECT
```

#### ✅ **OpenAPI Spec Is Correct**
```json
// holmesgpt-api/api/openapi.json
"RecoveryResponse": {
  "properties": {
    ...
    "needs_human_review": {
      "type": "boolean",
      "default": false
    },
    "human_review_reason": {
      "anyOf": [{"type": "string"}, {"type": "null"}]
    }
  }
}
```

#### ✅ **Request Builder Sends SignalType**
```go
// pkg/aianalysis/handlers/request_builder.go:124
req.SignalType.SetTo(spec.SignalType)  ← ✅ Sends MOCK_NO_WORKFLOW_FOUND
```

### **What's Still Unknown**

#### ❓ **Why Controller Logs Show `needsHumanReview: false`**

**Possible Causes:**
1. **HAPI Not Receiving Correct Signal Type**
   - Integration test might not be sending `MOCK_NO_WORKFLOW_FOUND` correctly
   - Request builder might be transforming/sanitizing the signal type
   - HAPI might be case-sensitive or have whitespace issues

2. **HAPI Response Not Including Fields**
   - FastAPI `response_model_exclude_unset=False` might not be working
   - Pydantic model might have field exclusion rules
   - HTTP response might be missing fields even though Python dict has them

3. **Go Client Not Deserializing During HTTP Call**
   - `ogen` client might have different behavior than `json.Unmarshal`
   - HTTP headers might affect deserialization
   - Response wrapper might be stripping fields

4. **Integration Test Infrastructure Issue**
   - Old HAPI instance might still be running (we killed PID 88007)
   - Integration tests might be using a different HAPI endpoint
   - Network/proxy might be caching old responses

---

## 🔧 **Recommended Next Steps**

### **Step 1: Verify Signal Type is Sent Correctly**
```bash
# Add debug logging to request_builder.go
log.Info("Building recovery request",
    "signalType", spec.SignalType,
    "rawSignalType", fmt.Sprintf("%q", spec.SignalType))

# Re-run tests and check logs
make test-integration-aianalysis FOCUS="BR-HAPI-197"
```

### **Step 2: Verify HAPI Receives Correct Signal Type**
```bash
# Add debug logging to holmesgpt-api/src/extensions/recovery/endpoint.py
logger.info(f"Recovery request received: signal_type={request.signal_type!r}")

# Re-run tests and check HAPI logs
```

### **Step 3: Verify HAPI Returns Fields in HTTP Response**
```bash
# Add response logging to holmesgpt-api/src/extensions/recovery/endpoint.py
logger.info(f"Recovery response: needs_human_review={result.needs_human_review}, reason={result.human_review_reason}")

# Re-run tests and check HAPI logs
```

### **Step 4: Verify Go Client Receives Fields**
```bash
# Add debug logging to pkg/holmesgpt/client/holmesgpt.go
log.Printf("Recovery response received: needs_human_review=%v, reason=%v",
    response.NeedsHumanReview, response.HumanReviewReason)

# Re-run tests and check controller logs
```

### **Step 5: Compare HTTP Responses**
```bash
# Capture HTTP response during integration test
tcpdump -i lo0 -A 'tcp port 18120' > /tmp/hapi_traffic.txt

# Run single test
make test-integration-aianalysis FOCUS="Recovery human review when no workflows match"

# Analyze captured traffic
grep -A 50 "needs_human_review" /tmp/hapi_traffic.txt
```

---

## 📊 **Test Status**

| Test Tier | Status | Details |
|---|---|---|
| **Unit Tests** | ✅ Pass | Helper functions work correctly |
| **Integration Tests** | ❌ Fail | 3/4 BR-HAPI-197 tests failing |
| **E2E Tests** | ❓ Unknown | Not yet run (blocked on integration) |

---

## 🎯 **Success Criteria**

Integration tests will pass when:
1. ✅ Controller logs show `needsHumanReview: true` for `MOCK_NO_WORKFLOW_FOUND`
2. ✅ AIAnalysis transitions to `Phase: Failed`
3. ✅ `Status.Reason` is `WorkflowResolutionFailed`
4. ✅ `Status.SubReason` is `NoMatchingWorkflows`
5. ✅ `Status.Message` contains human review indication

---

## 📝 **Key Files**

### **HAPI (Python)**
- `holmesgpt-api/api/openapi.json` - OpenAPI spec with fields
- `holmesgpt-api/src/models/recovery_models.py` - Pydantic model
- `holmesgpt-api/src/mock_responses.py` - Mock logic (lines 739-798, 801-870)
- `holmesgpt-api/src/extensions/recovery/endpoint.py` - FastAPI endpoint

### **AA Service (Go)**
- `pkg/holmesgpt/client/oas_schemas_gen.go` - Generated Go client (lines 2591-2614)
- `pkg/aianalysis/handlers/response_processor.go` - Recovery response processing (lines 162-195, 414-473)
- `pkg/aianalysis/handlers/request_builder.go` - Recovery request building (lines 101-137)
- `pkg/aianalysis/handlers/generated_helpers.go` - OptBool helper (lines 30-35)

### **Tests**
- `test/integration/aianalysis/recovery_human_review_integration_test.go` - Integration tests (3 failing)
- `test/e2e/aianalysis/04_recovery_flow_test.go` - E2E test (not yet run)

---

## 🚀 **Next Actions**

**Immediate (P0)**:
1. Add debug logging to all 4 layers (test → request builder → HAPI → response processor)
2. Re-run single failing test with full logging
3. Identify where `needs_human_review=true` is being lost
4. Fix the issue
5. Verify all 3 BR-HAPI-197 integration tests pass
6. Run E2E test to confirm full lifecycle

**Follow-up (P1)**:
- Document root cause in this file
- Add regression test to prevent recurrence
- Update HAPI/AA integration documentation

---

## 💡 **Hypothesis**

**Most Likely**: HAPI is not receiving the correct `signal_type` value during integration tests, so it's returning the default mock response (which has `needs_human_review=false`) instead of the edge case response.

**Evidence**:
- Direct curl with `MOCK_NO_WORKFLOW_FOUND` works ✅
- Integration test specifies `SignalType: "MOCK_NO_WORKFLOW_FOUND"` ✅
- Controller logs show `needsHumanReview: false` ❌

**Gap**: We haven't verified what signal_type value HAPI actually receives during the integration test.

---

**Status**: Waiting for debug logging to identify where `needs_human_review=true` is being lost.

