# RESPONSE: Mock Mode Verification Request - HAPI to AIAnalysis Team

**From**: HAPI Team
**To**: AIAnalysis Team
**Date**: 2025-12-13
**Priority**: HIGH (Unblocks E2E implementation)
**Re**: [REQUEST_HAPI_MOCK_WORKFLOW_RESPONSE_ENHANCEMENT.md](REQUEST_HAPI_MOCK_WORKFLOW_RESPONSE_ENHANCEMENT.md)

---

## 🎯 Executive Summary

**FINDING**: HAPI mock responses **already include workflow selection data** via BR-HAPI-212 (Mock LLM Mode).

**Critical Discovery**: Mock mode activation requires `MOCK_LLM_MODE=true` environment variable. If this isn't set in your E2E environment, HAPI will attempt to call the real LLM instead of returning mock responses.

**Request**: Please verify mock mode is properly configured in AA E2E environment **before** we implement field enhancements.

---

## 🔍 What We Found

### Good News: Mock Infrastructure Already Exists ✅

HAPI has a comprehensive mock response system (BR-HAPI-212) that includes:

```python
# holmesgpt-api/src/mock_responses.py (lines 300-327)
"selected_workflow": {
    "workflow_id": "mock-oomkill-increase-memory-v1",
    "title": "OOMKill Recovery - Increase Memory Limits (MOCK)",
    "version": "1.0.0",
    "containerImage": "kubernaut/mock-workflow-...",
    "confidence": 0.92,
    "rationale": "Mock selection based on OOMKilled signal type",
    "parameters": { ... }
},
"alternative_workflows": [ ... ],
"warnings": [ ... ],
"needs_human_review": false
```

```python
# Recovery endpoint (lines 617-652)
"recovery_analysis": {
    "previous_attempt_assessment": {
        "workflow_id": previous_workflow_id,
        "failure_understood": true,
        "failure_reason_analysis": "Mock analysis...",
        "state_changed": false,
        "current_signal_type": signal_type
    },
    "root_cause_refinement": "..."
},
"selected_workflow": { ... }
```

### Potential Issue: Mock Mode Not Enabled ⚠️

**Mock mode activation code** (line 782-789 in `incident.py`):

```python
from src.mock_responses import is_mock_mode_enabled, generate_mock_incident_response
if is_mock_mode_enabled():
    logger.info({
        "event": "mock_mode_active",
        "incident_id": incident_id,
        "message": "Returning deterministic mock response (MOCK_LLM_MODE=true)"
    })
    return generate_mock_incident_response(request_data)

# If mock mode NOT enabled, code continues to real LLM call...
```

**Mock mode check** (line 42-51 in `mock_responses.py`):

```python
def is_mock_mode_enabled() -> bool:
    """
    Check if mock LLM mode is enabled via environment variable.

    BR-HAPI-212: Mock mode for integration testing.

    Returns:
        True if MOCK_LLM_MODE=true (case-insensitive)
    """
    return os.getenv("MOCK_LLM_MODE", "").lower() == "true"
```

---

## 🚨 Critical Question: Is Mock Mode Enabled in Your E2E Environment?

### What to Check

**1. AA E2E HAPI Deployment Manifest**

Check your Kubernetes deployment/pod spec for HAPI in the AA E2E environment:

```yaml
# Expected configuration
apiVersion: apps/v1
kind: Deployment
metadata:
  name: holmesgpt-api
  namespace: kubernaut-system
spec:
  template:
    spec:
      containers:
      - name: holmesgpt-api
        image: kubernaut-holmesgpt-api:test
        env:
        - name: MOCK_LLM_MODE
          value: "true"          # ⬅️ MUST BE SET TO "true"
        - name: LLM_MODEL
          value: "mock-model"    # ⬅️ Can be any value when mock mode enabled
        - name: LLM_PROVIDER
          value: "openai"        # ⬅️ Can be any value when mock mode enabled
```

**2. HAPI Pod Logs in AA E2E**

When mock mode is **enabled**, you should see this log on every analyze request:

```json
{
  "event": "mock_mode_active",
  "incident_id": "...",
  "message": "Returning deterministic mock response (MOCK_LLM_MODE=true)"
}
```

When mock mode is **NOT enabled**, you'll see LLM initialization logs instead:

```json
{
  "event": "llm_client_initialized",
  "provider": "openai",
  "model": "gpt-4"
}
```

---

## 📊 Diagnostic Commands

### Check HAPI Environment Variables

```bash
# Get HAPI pod name
export KUBECONFIG=~/.kube/aianalysis-e2e-config
HAPI_POD=$(kubectl get pods -n kubernaut-system -l app=holmesgpt-api -o jsonpath='{.items[0].metadata.name}')

# Check if MOCK_LLM_MODE is set
kubectl exec -n kubernaut-system $HAPI_POD -- env | grep MOCK_LLM_MODE

# Expected output:
# MOCK_LLM_MODE=true
```

### Check HAPI Logs During Test

```bash
# Watch HAPI logs while running E2E test
kubectl logs -n kubernaut-system -f deployment/holmesgpt-api | grep -E "mock_mode|selected_workflow"

# Expected output (if mock mode enabled):
# {"event": "mock_mode_active", "incident_id": "...", ...}
# {"event": "mock_incident_response_generated", "mock_workflow_id": "mock-oomkill-increase-memory-v1", ...}
```

### Test HAPI Mock Mode Manually

```bash
# Get HAPI service URL (from inside cluster or port-forward)
kubectl port-forward -n kubernaut-system svc/holmesgpt-api 18120:8080

# Test incident endpoint
curl -X POST http://localhost:18120/api/v1/incident/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "incident_id": "test-mock-verification",
    "signal_type": "OOMKilled",
    "resource_namespace": "default",
    "resource_name": "test-pod",
    "resource_kind": "Pod"
  }' | jq '{
    has_selected_workflow: (.selected_workflow != null),
    workflow_id: .selected_workflow.workflow_id,
    warnings: .warnings
  }'

# Expected output (if mock mode enabled):
# {
#   "has_selected_workflow": true,
#   "workflow_id": "mock-oomkill-increase-memory-v1",
#   "warnings": [
#     "MOCK_MODE: This response is deterministic for integration testing (BR-HAPI-212)",
#     "MOCK_MODE: No LLM was called - response based on signal_type matching"
#   ]
# }

# If mock mode NOT enabled, you'll see real LLM response or error about missing LLM config
```

---

## 🎯 Verification Checklist

Please confirm the following:

### Environment Configuration
- [ ] `MOCK_LLM_MODE=true` is set in HAPI deployment manifest
- [ ] HAPI pods have restarted after environment variable change (if you just added it)
- [ ] HAPI pod logs show `"event": "mock_mode_active"` on analyze requests

### Response Structure
- [ ] HAPI returns `selected_workflow` object (not null)
- [ ] HAPI returns `recovery_analysis` object for recovery endpoint
- [ ] Response includes mock mode warnings

### AA Controller Logs
- [ ] AA controller receives response from HAPI (no timeout)
- [ ] AA controller logs show it **received** `selected_workflow` data
- [ ] If controller still fails, note the **exact error message**

---

## 🔧 Two Possible Scenarios

### Scenario A: Mock Mode IS Enabled ✅

**Evidence**:
- HAPI logs show `"mock_mode_active"`
- HAPI returns responses with `selected_workflow`
- Manual curl test shows mock workflow IDs

**Issue**: Field naming/structure mismatch between HAPI and AA expectations

**Next Steps**: HAPI will implement field enhancements (1-2 hours)
- Add `name` alias for `title`
- Add `reasoning` alias for `rationale`
- Add `labels` object to `selected_workflow`
- Flatten/enhance `recovery_analysis` structure

---

### Scenario B: Mock Mode NOT Enabled ❌

**Evidence**:
- HAPI logs show LLM initialization
- HAPI returns errors about missing LLM config
- No mock mode warnings in responses

**Issue**: Environment variable not configured

**Next Steps**: AA team fixes deployment (5 minutes)
1. Add `MOCK_LLM_MODE: "true"` to HAPI deployment
2. Restart HAPI pods
3. Rerun E2E tests
4. **Expected result**: Tests should pass without any HAPI code changes

---

## 📋 Information We Need from AA Team

Please provide the following to help us diagnose:

### 1. Environment Variable Confirmation
```bash
# Output of this command:
kubectl exec -n kubernaut-system $HAPI_POD -- env | grep -E "MOCK_LLM_MODE|LLM_MODEL|LLM_PROVIDER"
```

### 2. HAPI Logs Sample
```bash
# First 50 lines of HAPI logs during E2E test:
kubectl logs -n kubernaut-system deployment/holmesgpt-api --tail=50
```

### 3. Manual API Test Result
```bash
# Output of the curl test from "Test HAPI Mock Mode Manually" section above
```

### 4. AA Controller Error Logs
```bash
# The exact error message from AA controller when it fails:
kubectl logs -n kubernaut-system deployment/aianalysis-controller | grep -A 5 "No workflow selected"
```

---

## 📊 Decision Matrix

| Mock Mode Status | HAPI Returns Data? | Next Action |
|---|---|---|
| ✅ Enabled | ✅ Yes | HAPI implements field enhancements (1-2 hrs) |
| ✅ Enabled | ❌ No | HAPI investigates mock response bug |
| ❌ Not Enabled | ❌ No | AA team adds env var, restart pods (5 min) |
| ❓ Unknown | ❓ Unknown | AA team runs diagnostic commands above |

---

## 🚀 Recommended Next Steps

### Step 1: AA Team Verification (15 minutes)
1. Run the diagnostic commands above
2. Provide the 4 pieces of information requested
3. Determine which scenario (A or B) applies

### Step 2: Coordination Call (Optional, 15 minutes)
If diagnostics are unclear, let's schedule a quick sync to:
- Review logs together
- Test HAPI endpoints live
- Identify exact integration issue

### Step 3: Implementation (varies)
- **Scenario A**: HAPI implements field enhancements (1-2 hours)
- **Scenario B**: AA team fixes environment variable (5 minutes)

---

## 💡 Why This Matters

**If mock mode is NOT enabled**:
- ❌ HAPI tries to call real LLM (expensive, slow, non-deterministic)
- ❌ Without LLM API keys, HAPI returns errors
- ❌ Field enhancements won't help if mock responses aren't being generated
- ✅ **5-minute fix** (add env var) vs 2-hour implementation

**If mock mode IS enabled**:
- ✅ HAPI returns deterministic mock responses
- ❌ AA controller can't parse responses due to field naming
- ✅ **1-2 hour fix** (add alias fields) will unblock 9/22 E2E tests

**Critical**: We need to know which scenario we're in before proceeding!

---

## 📞 How to Respond

Please create a reply document: `RESPONSE_AA_MOCK_MODE_VERIFICATION_RESULTS.md`

**Include**:
1. ✅ or ❌ for each item in "Verification Checklist"
2. The 4 diagnostic outputs requested
3. Which scenario (A or B) you believe applies
4. Any additional context or error messages

**Timeline**:
- Diagnostics: 15 minutes
- Response document: 15 minutes
- **Total**: 30 minutes

Once we have your verification results, we'll know the fastest path to unblock your E2E tests!

---

## 🔗 Related Documents

- **Your Original Request**: [REQUEST_HAPI_MOCK_WORKFLOW_RESPONSE_ENHANCEMENT.md](REQUEST_HAPI_MOCK_WORKFLOW_RESPONSE_ENHANCEMENT.md)
- **Our Triage Analysis**: [TRIAGE_AA_MOCK_RESPONSE_REQUEST.md](TRIAGE_AA_MOCK_RESPONSE_REQUEST.md)
- **Mock Response Implementation**: `holmesgpt-api/src/mock_responses.py` (lines 204-664)
- **BR-HAPI-212**: Mock LLM Mode for Integration Testing

---

## ⏱️ Quick Summary

**What We're Asking**:
- Run 4 diagnostic commands
- Tell us if `MOCK_LLM_MODE=true` is set
- Share HAPI logs and error messages

**Why It Matters**:
- Determines if this is a 5-minute fix (env var) or 2-hour fix (field enhancements)
- Prevents wasted implementation effort if root cause is different

**Expected Response Time**: 30 minutes

**Next Steps**: Based on your verification results, we'll either:
1. Help you fix environment configuration (5 min), OR
2. Implement field enhancements (1-2 hours)

---

**Status**: ⏸️ **Awaiting AA Team Verification Results**
**Priority**: HIGH (Blocks 41% of AA E2E tests)
**Created**: 2025-12-13

Thank you for your collaboration! Let's get those E2E tests passing! 🚀

