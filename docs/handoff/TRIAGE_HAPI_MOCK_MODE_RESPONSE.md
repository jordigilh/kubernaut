# TRIAGE: HAPI Mock Mode Response Verification

**Date**: 2025-12-13  
**From**: AIAnalysis Team  
**To**: HAPI Team  
**Re**: [RESPONSE_HAPI_TO_AA_MOCK_MODE_VERIFICATION.md](RESPONSE_HAPI_TO_AA_MOCK_MODE_VERIFICATION.md)  
**Status**: ✅ **MOCK_LLM_MODE IS SET - FIELD COMPATIBILITY CONFIRMED**

---

## 🎯 **Executive Summary**

**FINDING 1**: ✅ `MOCK_LLM_MODE=true` **IS SET** in AA E2E HAPI deployment  
**FINDING 2**: ✅ HAPI mock response fields **MATCH** AIAnalysis expectations  
**FINDING 3**: ❌ Mock mode **NOT ACTIVATING** despite environment variable being set  

**Root Cause**: Unknown - requires enhanced logging to diagnose

**Recommended Action**: **HAPI team adds diagnostic logging** → AA team reruns E2E → Identify exact failure point

---

## ✅ **Verification Results**

### **1. Environment Variable Configuration** ✅

**Source**: `test/infrastructure/aianalysis.go:627-631`

```go
env:
- name: LLM_PROVIDER
  value: mock
- name: LLM_MODEL
  value: mock://test-model
- name: MOCK_LLM_MODE
  value: "true"          // ✅ EXPLICITLY SET
- name: DATASTORAGE_URL
  value: http://datastorage:8080
```

**HAPI Mock Mode Check** (from `mock_responses.py:42-51`):
```python
def is_mock_mode_enabled() -> bool:
    return os.getenv("MOCK_LLM_MODE", "").lower() == "true"
```

**Match Analysis**:
- AA sets: `"true"` (lowercase string in quotes)
- HAPI checks: `.lower() == "true"` (lowercase comparison)
- **Result**: ✅ Should match perfectly

**Status**: ✅ **CONFIRMED** - Environment variable is correctly configured

---

### **2. Field Compatibility** ✅

**HAPI Mock Response** (from `mock_responses.py:300-307`):
```python
"selected_workflow": {
    "workflow_id": scenario.workflow_id,
    "title": scenario.workflow_title,
    "version": "1.0.0",
    "containerImage": "kubernaut/...",
    "confidence": scenario.confidence,
    "rationale": "Mock selection...",
    "parameters": parameters
}
```

**AIAnalysis Struct** (from `holmesgpt.go:253-268`):
```go
type SelectedWorkflow struct {
    WorkflowID      string            `json:"workflow_id"`     // ✅
    Version         string            `json:"version"`         // ✅
    ContainerImage  string            `json:"containerImage"`  // ✅
    ContainerDigest string            `json:"containerDigest"` // Optional
    Confidence      float64           `json:"confidence"`      // ✅
    Parameters      map[string]string `json:"parameters"`      // ✅
    Rationale       string            `json:"rationale"`       // ✅
}
```

**Field Mapping**:
| HAPI Field | AA Field | Status |
|------------|----------|--------|
| `workflow_id` | `WorkflowID` (`json:"workflow_id"`) | ✅ Match |
| `version` | `Version` (`json:"version"`) | ✅ Match |
| `containerImage` | `ContainerImage` (`json:"containerImage"`) | ✅ Match |
| `confidence` | `Confidence` (`json:"confidence"`) | ✅ Match |
| `rationale` | `Rationale` (`json:"rationale"`) | ✅ Match |
| `parameters` | `Parameters` (`json:"parameters"`) | ✅ Match |
| `title` | (not in AA struct) | ✅ OK (extra fields ignored) |

**Status**: ✅ **CONFIRMED** - All fields compatible, no naming conflicts

---

### **3. Mock Mode Activation** ❌

**Expected in HAPI Logs** (from `incident.py:784-788`):
```python
logger.info({
    "event": "mock_mode_active",
    "incident_id": incident_id,
    "message": "Returning deterministic mock response (MOCK_LLM_MODE=true)"
})
```

**Actual in HAPI Logs** (from E2E test run):
```
INFO:  10.244.1.6:49204 - "POST /api/v1/incident/analyze HTTP/1.1" 200 OK
INFO:  10.244.1.6:56266 - "POST /api/v1/recovery/analyze HTTP/1.1" 200 OK
```

**Missing**:
- ❌ No `"mock_mode_active"` events
- ❌ No `"mock_incident_response_generated"` events
- ❌ Only HTTP access logs visible

**Status**: ❌ **NOT CONFIRMED** - Mock mode appears not to be activating

---

## 🔍 **Root Cause Analysis**

### **The Mystery**

| Component | Status | Evidence |
|-----------|--------|----------|
| **Env Var Set** | ✅ | `test/infrastructure/aianalysis.go:630` |
| **Env Var Format** | ✅ | `"true"` matches HAPI check |
| **Field Names** | ✅ | All JSON fields match |
| **Mock Code Exists** | ✅ | `mock_responses.py` has complete implementation |
| **Mock Mode Activating** | ❌ | No log events in HAPI output |
| **Controller Gets Workflow** | ❌ | "No workflow selected" error |

**Conclusion**: Environment variable is set correctly, fields match correctly, but mock mode isn't activating.

### **Possible Causes**

#### **1. Environment Variable Not Reaching Python Process** (HIGH)
- Kubernetes set env var in deployment
- But Python process doesn't see it
- **Test**: `kubectl exec $HAPI_POD -- env | grep MOCK`
- **Cannot verify**: Cluster was cleaned up after test

#### **2. Different Code Path Being Executed** (MEDIUM)
- HAPI might have multiple entry points
- Mock mode check might not be in the path
- **Test**: Check if all endpoints call `is_mock_mode_enabled()`
- **Verify**: recovery.py also needs mock mode check

#### **3. Mock Response Generation Failing** (LOW)
- Mock mode activates
- But `generate_mock_incident_response()` returns incomplete data
- **Test**: Would see "mock_mode_active" logs but still fail
- **Not the case**: No mock mode logs at all

---

## 🚀 **Recommended Solution: Enhanced Logging**

### **Why Logging First**

HAPI team correctly asks for verification before implementing changes. We've verified:
- ✅ Environment variable is set
- ✅ Field names match

But we **cannot verify** (cluster cleaned up):
- ❓ Whether env var reaches Python process
- ❓ Whether mock mode check returns true
- ❓ What response HAPI actually generates

**Enhanced logging will show all of this in the next E2E run.**

### **Proposed Logging Enhancement**

**File**: `holmesgpt-api/src/extensions/incident.py`  
**Location**: Lines 782-789 (before mock mode check)

```python
# Diagnostic logging for AA E2E troubleshooting
import os
logger.info({
    "event": "mock_mode_diagnostic",
    "MOCK_LLM_MODE_raw": os.getenv("MOCK_LLM_MODE"),
    "MOCK_LLM_MODE_lower": os.getenv("MOCK_LLM_MODE", "").lower(),
    "is_mock_enabled_result": is_mock_mode_enabled(),
    "incident_id": incident_id
})

if is_mock_mode_enabled():
    logger.info({
        "event": "mock_mode_ACTIVATED",
        "incident_id": incident_id,
        "message": "Returning deterministic mock response"
    })
    response = generate_mock_incident_response(request_data)
    
    # Log response structure
    logger.info({
        "event": "mock_response_structure",
        "has_selected_workflow": response.get("selected_workflow") is not None,
        "workflow_id": response.get("selected_workflow", {}).get("workflow_id"),
        "response_keys": list(response.keys())
    })
    return response
else:
    logger.info({
        "event": "mock_mode_NOT_ACTIVATED",
        "reason": "is_mock_mode_enabled() returned False"
    })
```

**Same for**: `holmesgpt-api/src/extensions/recovery.py` (if it has mock mode check)

---

## 📊 **Expected Diagnostic Results**

### **Scenario A: Mock Mode Working** ✅
```json
{"event": "mock_mode_diagnostic", "MOCK_LLM_MODE_raw": "true", "is_mock_enabled_result": true}
{"event": "mock_mode_ACTIVATED", "incident_id": "..."}
{"event": "mock_response_structure", "has_selected_workflow": true, "workflow_id": "mock-oomkill-increase-memory-v1"}
```
**Meaning**: Mock mode is working, AA controller JSON parsing issue  
**Next Step**: AA team investigates client code

### **Scenario B: Mock Mode Not Activating** ❌
```json
{"event": "mock_mode_diagnostic", "MOCK_LLM_MODE_raw": null, "is_mock_enabled_result": false}
{"event": "mock_mode_NOT_ACTIVATED", "reason": "is_mock_mode_enabled() returned False"}
```
**Meaning**: Environment variable not reaching Python  
**Next Step**: AA team investigates Kubernetes deployment

### **Scenario C: Partial Activation** ⚠️
```json
{"event": "mock_mode_diagnostic", "MOCK_LLM_MODE_raw": "true", "is_mock_enabled_result": true}
{"event": "mock_mode_ACTIVATED"}
{"event": "mock_response_structure", "has_selected_workflow": false}
```
**Meaning**: Mock mode works but response generation issue  
**Next Step**: HAPI team investigates `generate_mock_incident_response()`

---

## 🎯 **Action Plan**

### **HAPI Team (30-60 minutes)**
1. ✅ Add enhanced logging to `incident.py` and `recovery.py`
2. ✅ Rebuild HAPI image locally: `podman build -t kubernaut-holmesgpt-api:test -f holmesgpt-api/Dockerfile .`
3. ✅ Test locally with `MOCK_LLM_MODE=true`
4. ✅ Confirm logging works
5. ✅ Create response document: `RESPONSE_HAPI_ENHANCED_LOGGING_DEPLOYED.md`

### **AA Team (30 minutes - After HAPI Deployment)**
1. ✅ Pull latest HAPI changes
2. ✅ Rerun E2E tests: `make test-e2e-aianalysis`
3. ✅ Capture enhanced HAPI logs: `kubectl logs -n kubernaut-system -f deployment/holmesgpt-api | tee /tmp/hapi-enhanced-logs.txt`
4. ✅ Analyze diagnostic events
5. ✅ Create response document: `RESPONSE_AA_ENHANCED_HAPI_DIAGNOSTIC_RESULTS.md`

### **Joint Analysis (15 minutes)**
1. Review enhanced logs together
2. Identify exact failure point
3. Implement targeted fix

---

## 📁 **Documents to Create**

### **HAPI Team**
**Document**: `RESPONSE_HAPI_ENHANCED_LOGGING_DEPLOYED.md`
**Contents**:
- ✅ Logging code added to incident.py and recovery.py
- ✅ Local testing results with MOCK_LLM_MODE=true
- ✅ Confirmation that mock mode activates locally
- ✅ Sample log output showing diagnostic events

### **AA Team** (After E2E Rerun)
**Document**: `RESPONSE_AA_ENHANCED_HAPI_DIAGNOSTIC_RESULTS.md`
**Contents**:
- ✅ Enhanced logs from E2E test run
- ✅ Mock mode activation status (A, B, or C)
- ✅ Root cause identification
- ✅ Recommended next steps

---

## ✅ **Summary**

**Verified** ✅:
- `MOCK_LLM_MODE=true` is set in AA E2E deployment
- HAPI field names match AIAnalysis expectations
- No field enhancement needed (fields already compatible)

**Not Verified** ❌:
- Whether mock mode actually activates
- Whether env var reaches Python process
- What response HAPI returns

**Recommendation**: Enhanced logging (30-60 min) will definitively identify root cause and allow targeted fix.

**Expected Timeline**:
- HAPI logging: 30-60 min
- AA E2E rerun: 30 min
- Analysis: 15 min
- **Total**: 75-105 minutes to root cause

---

**Status**: ✅ **TRIAGE COMPLETE - ENHANCED LOGGING RECOMMENDED**  
**Next**: HAPI team implements diagnostic logging  
**Confidence**: 90% that logging will reveal root cause
