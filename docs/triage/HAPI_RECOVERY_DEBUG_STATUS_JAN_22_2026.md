# HAPI Recovery Tests - Debugging Status Update

**Date**: January 22, 2026 21:55
**Status**: 🔍 **DEBUGGING IN PROGRESS**
**Current**: 6 failed, 59 passed (91% pass rate - 3 tests fixed earlier)

---

## 📊 **Session Summary**

| Phase | Failures | Tests Fixed | Pass Rate |
|-------|----------|-------------|-----------|
| **Initial** | 19 | - | 71% |
| **After Port Fix** | 9 | 10 | 86% |
| **After Event Category Fix** | 6 | 3 | 91% |
| **After Mock LLM Recovery Fix** | 6 | 0 | 91% (no change) |

---

## ✅ **What We've Fixed So Far**

### **Fix 1: Mock LLM Port Mismatch** (10 tests)
- Changed LLM_ENDPOINT from `http://127.0.0.1:8080` → `http://127.0.0.1:18140`
- File: `holmesgpt-api/tests/integration/conftest.py` line 408

### **Fix 2: Event Category Assertions** (3 tests)
- Changed `assert len(llm_events) == 2` → `assert len(llm_events) >= 2`
- Files: `holmesgpt-api/tests/integration/test_hapi_audit_flow_integration.py` (3 locations)
- Reason: ADR-034 v1.1+ tool-using LLMs emit multiple request/response pairs

---

## ⚠️ **Current Problem: Recovery Analysis Field Missing**

### **Affected Tests** (6 failures)
All in `test_recovery_analysis_structure_integration.py`:
1. `test_recovery_analysis_field_present`
2. `test_previous_attempt_assessment_structure`
3. `test_multiple_recovery_attempts`
4. `test_field_types_correct`
5. `test_aa_team_integration_mapping`
6. `test_mock_mode_returns_valid_structure`

### **Root Cause Hypothesis**

Tests call `/api/v1/recovery/analyze` endpoint and expect:
```json
{
  "recovery_analysis": {
    "previous_attempt_assessment": {
      "failure_understood": true,
      "failure_reason_analysis": "string",
      "state_changed": false,
      "current_signal_type": "OOMKilled"
    }
  }
}
```

But the response is missing the `recovery_analysis` field.

---

## 🔍 **Investigation Performed**

### **Mock LLM Analysis**

**Scenario Detection** (`test/services/mock-llm/src/server.py` line 471-476):
```python
if ("recovery" in content and ("previous remediation" in content or "failed attempt" in content)) or \
   ("workflow execution failed" in content and "recovery" in content):
    return MOCK_SCENARIOS.get("recovery", DEFAULT_SCENARIO)
```

**HAPI Recovery Prompt** (`holmesgpt-api/src/extensions/recovery/prompt_builder.py` line 345-350):
```python
prompt = f"""# Recovery Analysis Request (Attempt {attempt_number})

## ⚠️ Previous Remediation Attempt - CRITICAL CONTEXT

**This is a RECOVERY attempt**. A previous remediation was tried and FAILED.
```

**SHOULD MATCH**: ✅ Prompt contains "recovery" AND "previous remediation"

### **Mock LLM Response Modes**

Mock LLM has 3 response modes (`server.py` line 437-446):
1. **Text Response** (model starts with "text-") → Calls `_recovery_text_response()`
2. **Tool Call Response** (no tool result yet) → Phase 1: Return tool call
3. **Final Analysis** (has tool result) → Phase 2: Return final analysis

**Issue Identified**: Recovery tests use model="gpt-4-turbo", which goes through **Tool Call** path, NOT the text response path.

### **Fix Applied** (But Tests Still Failing)

Updated `_final_analysis_response()` method (`server.py` line 550-580):
```python
# Detect if this is a recovery scenario
is_recovery = "recovery" in content_text and ("previous remediation" in content_text or "previous execution" in content_text)

# BR-AI-081: Add recovery_analysis for recovery scenarios
if is_recovery:
    analysis_json["recovery_analysis"] = {
        "previous_attempt_assessment": {
            "failure_understood": True,
            "failure_reason_analysis": scenario.root_cause,
            "state_changed": False,
            "current_signal_type": scenario.signal_type
        }
    }
```

**Result**: Tests still failing (same 6 failures)

---

## 🚧 **Next Debugging Steps**

### **Step 1: Verify Mock LLM Logs**

**Problem**: Mock LLM logs at `/tmp/kubernaut-must-gather/holmesgptapi-integration-20260122-215252/holmesgptapi_mock-llm-hapi.log` show:
```
Starting Mock LLM server on 0.0.0.0:8080...
```

**Expected**: Port 18140 (not 8080)

**Action**: Verify Mock LLM container is actually starting on correct port for HAPI integration tests

### **Step 2: Check Actual Response from HAPI**

**Need**: Capture the actual JSON response from `/api/v1/recovery/analyze` to see what fields are present

**Options**:
A) Add debug logging to recovery endpoint
B) Check DataStorage audit logs for recovery analysis requests
C) Run single test with verbose output to capture response

### **Step 3: Verify HAPI Recovery Parser**

**File**: `holmesgpt-api/src/extensions/recovery/result_parser.py` line 214-262

Check if the parser is correctly extracting `recovery_analysis` from Mock LLM response:
```python
# Extract recovery-specific fields if present
recovery_analysis = structured.get("recovery_analysis", {})

# Build recovery response
response = {
    # ... other fields ...
    "recovery_analysis": recovery_analysis,  # This should be included
}
```

**Validation**: Add logging to see if `recovery_analysis` is present in `structured` dict

---

##  🎯 **Most Likely Root Causes** (Ranked)

### **1. Mock LLM Not Detecting Recovery Scenario** (60% probability)
- Detection logic too strict
- Prompt doesn't contain expected keywords
- Case-sensitivity issue (already handled with `.lower()`)

**Fix**: Broaden detection logic or add more keywords

### **2. HAPI Parser Not Including recovery_analysis** (30% probability)
- Parser extracts it from LLM response but doesn't include in API response
- Field mapping issue

**Fix**: Add recovery_analysis to response dict construction

### **3. Mock LLM JSON Format Mismatch** (10% probability)
- Mock LLM returns recovery_analysis in wrong JSON structure
- Parser regex doesn't match Mock LLM output format

**Fix**: Adjust Mock LLM JSON output or parser regex

---

## 🔧 **Recommended Next Action**

**OPTION A: Add Debug Logging** (15 minutes)

Add logging to capture:
1. Mock LLM: Log scenario detected (`_detect_scenario`)
2. Mock LLM: Log final JSON being returned (`_final_analysis_response`)
3. HAPI Parser: Log extracted `recovery_analysis` dict
4. HAPI Endpoint: Log final response being returned

**OPTION B: Manual API Test** (10 minutes)

1. Start infrastructure (DataStorage, Redis, Postgres, Mock LLM)
2. Start HAPI locally
3. `curl` the `/api/v1/recovery/analyze` endpoint with test payload
4. Inspect actual response

**OPTION C: Simplify Detection Logic** (5 minutes)

Replace strict detection with simpler check:
```python
# In Mock LLM _detect_scenario()
if "recovery_attempt_number" in str(messages) or "is_recovery_attempt" in str(messages):
    return MOCK_SCENARIOS.get("recovery", DEFAULT_SCENARIO)
```

Reason: Request JSON includes `{"is_recovery_attempt": True, "recovery_attempt_number": 1}`

---

## 💡 **Recommendation**

Try **OPTION C** first (5 minutes):
- Simplest fix
- Directly checks for recovery-specific JSON fields
- No need to rely on prompt text keywords

If that doesn't work, proceed to **OPTION A** (debug logging) to understand the actual data flow.

---

## 📚 **Files Modified So Far**

1. ✅ `holmesgpt-api/tests/integration/conftest.py` (Mock LLM port fix)
2. ✅ `holmesgpt-api/tests/integration/test_hapi_audit_flow_integration.py` (event category assertions - 3 locations)
3. ✅ `test/services/mock-llm/src/server.py` (`_final_analysis_response` - recovery detection added)

---

**Status**: Ready for next debugging iteration. Recommend trying OPTION C (simplified detection logic).
