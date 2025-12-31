# AIAnalysis Recovery Human Review - Debug Findings

**Date**: December 30, 2025
**Status**: 🔍 **ROOT CAUSE IDENTIFIED**
**Priority**: P0 - Blocks BR-HAPI-197 completion

---

## 🎯 **Summary**

Added debug logging to trace the `needs_human_review` field through the entire request/response flow. **Found that the AA service is correctly sending `signal_type: "MOCK_NO_WORKFLOW_FOUND"` to HAPI, but HAPI is returning `needs_human_review: false` instead of `true`.**

---

## ✅ **Debug Logging Added**

### **1. Request Builder** (`pkg/aianalysis/handlers/request_builder.go`)
```go
b.log.Info("🔍 DEBUG: Reading from CRD",
    "crdName", analysis.Name,
    "spec.SignalType", spec.SignalType,
    "previousExecutionsCount", len(analysis.Spec.PreviousExecutions),
)

b.log.Info("🔍 DEBUG: Building recovery request",
    "signalType", spec.SignalType,
    "signalTypeQuoted", fmt.Sprintf("%q", spec.SignalType),
    "isRecoveryAttempt", true,
    "recoveryAttemptNumber", analysis.Spec.RecoveryAttemptNumber,
)
```

### **2. Response Processor** (`pkg/aianalysis/handlers/response_processor.go`)
```go
p.log.Info("🔍 DEBUG: Recovery response received from HAPI",
    "NeedsHumanReview.Set", resp.NeedsHumanReview.Set,
    "NeedsHumanReview.Value", resp.NeedsHumanReview.Value,
    "HumanReviewReason.Set", resp.HumanReviewReason.Set,
    "HumanReviewReason.Null", resp.HumanReviewReason.Null,
    "HumanReviewReason.Value", resp.HumanReviewReason.Value,
    "needsHumanReview_computed", needsHumanReview,
)
```

---

## 🔍 **Test Results**

### **Test: "Recovery human review when no workflows match"**

**CRD Created by Test:**
```
spec.SignalType: "MOCK_NO_WORKFLOW_FOUND"  ← ✅ CORRECT
previousExecutions[0].OriginalRCA.SignalType: "CrashLoopBackOff"  ← Previous execution
```

**Request Builder Logs:**
```
🔍 DEBUG: Reading from CRD
  crdName: recovery-hr-no-wf-1767140195247362000-1767140126-1
  spec.SignalType: MOCK_NO_WORKFLOW_FOUND  ← ✅ CORRECT
  previousExecutionsCount: 1

🔍 DEBUG: Building recovery request
  signalType: MOCK_NO_WORKFLOW_FOUND  ← ✅ CORRECT
  signalTypeQuoted: "MOCK_NO_WORKFLOW_FOUND"
  isRecoveryAttempt: true
  recoveryAttemptNumber: 1
```

**Response Processor Logs:**
```
🔍 DEBUG: Recovery response received from HAPI
  NeedsHumanReview.Set: false  ← ❌ WRONG! Should be true
  NeedsHumanReview.Value: false  ← ❌ WRONG! Should be true
  HumanReviewReason.Set: false
  HumanReviewReason.Null: false
  HumanReviewReason.Value: ""
  needsHumanReview_computed: false

Processing successful recovery response
  canRecover: true
  confidence: 0.8
  warningsCount: 0
  hasSelectedWorkflow: true
  needsHumanReview: false  ← ❌ WRONG!
```

---

## 🚨 **Root Cause**

**The AA service is correctly sending `signal_type: "MOCK_NO_WORKFLOW_FOUND"` to HAPI, but HAPI is returning the default mock response (with `needs_human_review: false`) instead of the edge case response (with `needs_human_review: true`).**

### **Possible Causes**

1. **HAPI Mock Logic Not Receiving `signal_type`**
   - Python code might be reading from wrong field
   - Field might be nested in request object
   - Case sensitivity issue (HAPI checks `signal_type.upper()`)

2. **HAPI OpenAPI Validation Stripping Field**
   - Pydantic validation might be rejecting the field
   - Field might be marked as not required and getting dropped
   - FastAPI might be excluding unset fields

3. **HTTP Serialization Issue**
   - Go client might not be serializing `OptNilString` correctly
   - JSON encoding might be omitting the field
   - HTTP middleware might be transforming the request

---

## 🔧 **Next Steps**

### **Option A: Add HAPI Logging (Recommended)**

Add logging to HAPI's recovery endpoint to see what it's receiving:

```python
# holmesgpt-api/src/extensions/recovery/endpoint.py
@router.post("/analyze", response_model=RecoveryResponse, response_model_exclude_unset=False)
async def recovery_analyze_endpoint(request: RecoveryRequest) -> RecoveryResponse:
    logger.info(f"🔍 DEBUG: Recovery request received: signal_type={request.signal_type!r}")
    request_data = request.model_dump() if hasattr(request, 'model_dump') else request.dict()
    logger.info(f"🔍 DEBUG: Request dict: signal_type={request_data.get('signal_type')!r}")
    result = await analyze_recovery(request_data)
    logger.info(f"🔍 DEBUG: Response: needs_human_review={result.needs_human_review}, reason={result.human_review_reason}")
    return result
```

### **Option B: Capture HTTP Traffic**

Use `tcpdump` to capture the actual HTTP request/response:

```bash
# Terminal 1: Capture traffic
sudo tcpdump -i lo0 -A 'tcp port 18120' -w /tmp/hapi_traffic.pcap

# Terminal 2: Run test
make test-integration-aianalysis FOCUS="Recovery human review when no workflows match"

# Terminal 3: Analyze
tcpdump -A -r /tmp/hapi_traffic.pcap | grep -A 20 "signal_type"
```

### **Option C: Test with Direct HTTP Call**

Verify HAPI works with exact same JSON structure:

```bash
# Extract exact JSON from Go client logs (add logging to holmesgpt/client/holmesgpt.go)
# Then test with curl
curl -X POST http://localhost:18120/api/v1/recovery/analyze \
  -H "Content-Type: application/json" \
  -d @/tmp/exact_request.json
```

---

## 📊 **Verification Checklist**

- [x] ✅ AA test creates CRD with correct `SignalType: "MOCK_NO_WORKFLOW_FOUND"`
- [x] ✅ Request builder reads correct value from CRD
- [x] ✅ Request builder sends correct value to HAPI
- [ ] ❓ HAPI receives `signal_type: "MOCK_NO_WORKFLOW_FOUND"` in request
- [ ] ❓ HAPI mock logic recognizes the edge case
- [ ] ❓ HAPI returns `needs_human_review: true` in response
- [x] ❌ Go client deserializes response with `NeedsHumanReview.Set: false`

---

## 🎯 **Expected vs. Actual**

| Layer | Expected | Actual | Status |
|---|---|---|---|
| **Test CRD** | `SignalType: "MOCK_NO_WORKFLOW_FOUND"` | `SignalType: "MOCK_NO_WORKFLOW_FOUND"` | ✅ |
| **Request Builder** | Sends `signal_type: "MOCK_NO_WORKFLOW_FOUND"` | Sends `signal_type: "MOCK_NO_WORKFLOW_FOUND"` | ✅ |
| **HAPI Receives** | `signal_type: "MOCK_NO_WORKFLOW_FOUND"` | ❓ Unknown | ❓ |
| **HAPI Returns** | `needs_human_review: true` | `needs_human_review: false` | ❌ |
| **Go Client** | `NeedsHumanReview.Set: true` | `NeedsHumanReview.Set: false` | ❌ |

---

## 💡 **Hypothesis**

**Most Likely**: HAPI's Python code is not receiving the `signal_type` field from the JSON request, so it's using the default value `"Unknown"` and returning the default mock response.

**Why This Might Happen**:
1. **Pydantic Validation**: If `signal_type` is marked as optional and not provided, Pydantic might not include it in `request_data`
2. **Field Exclusion**: FastAPI might be excluding unset optional fields
3. **Serialization**: Go's `OptNilString` might not be serializing correctly when `Set: true` but `Null: false`

---

## 🔍 **Recommended Investigation Path**

1. **Add HAPI logging** (5 minutes)
2. **Run single test** (5 minutes)
3. **Check HAPI logs** to see what `signal_type` value it receives
4. **If HAPI receives correct value**: Check mock logic
5. **If HAPI receives wrong/missing value**: Check Go client serialization

---

**Status**: Waiting for HAPI logging to identify what signal_type value HAPI actually receives.

