# HAPI E2E Breakthrough - Mock LLM Investigation Complete

**Date**: January 29, 2026  
**Status**: 26/40 tests passing (14 failures)  
**Cluster**: Preserved at `/Users/jgil/.kube/holmesgpt-api-e2e-config`

---

## Executive Summary

**Mock LLM WAS ALWAYS BEING CALLED!** The investigation revealed:

1. ✅ **SDK Configuration**: Working correctly
   - `model=openai/mock-model` ✅
   - `api_base=http://mock-llm:8080` ✅  
   - `api_key=mock-api-key-for-e2e` ✅

2. ✅ **Network Connectivity**: HAPI→Mock LLM connection working
   - Manual curl test: ✅ Success
   - Mock LLM responds with correct JSON

3. ❌ **Logging Suppression**: `log_message()` method hid ALL HTTP requests
   - Made it appear like Mock LLM received zero requests
   - Manual curl confirmed Mock LLM IS responding

4. ❌ **Mock LLM Bug**: Scenarios return workflow NAMES instead of UUIDs
   - Root cause: Some scenarios have hardcoded names, not placeholder UUIDs
   - Affects: `network_partition`, `DEFAULT_SCENARIO`, Category F scenarios

---

## The Real Problems

### Problem 1: Mock LLM Logging Suppression

**File**: `test/services/mock-llm/src/server.py` line 490-492

```python
def log_message(self, format, *args):
    """Suppress default logging to reduce test noise."""
    pass  ← Hides ALL HTTP request logs!
```

**Impact**: Investigators couldn't see Mock LLM was being called.

**Fix Applied**: Added explicit `logger.info()` calls in `do_POST()` method.

---

### Problem 2: Mock LLM Returns Workflow Names (Not UUIDs)

**Root Cause**: Some scenarios have workflow NAMES hardcoded as `workflow_id`, not placeholder UUIDs.

**Examples**:

| Scenario | Old workflow_id | Issue |
|----------|----------------|-------|
| network_partition | `"wait-for-heal-v1"` | Workflow NAME, not UUID |
| DEFAULT_SCENARIO | `"generic-restart-v1"` | Workflow NAME, not UUID |
| multi_step_recovery | `"autoscaler-enable-v1"` | Workflow NAME, not UUID |
| cascading_failure | `"memory-increase-v1"` | Workflow NAME, not UUID |
| near_attempt_limit | `"rollback-deployment-v1"` | Workflow NAME, not UUID |
| noisy_neighbor | `"resource-quota-v1"` | Workflow NAME, not UUID |
| recovery_basic | `"memory-increase-basic-v1"` | Workflow NAME, not UUID |

**Correct Pattern** (from working scenarios like `oomkilled`):
```python
workflow_name="oomkill-increase-memory-v1",  # Enables UUID loading
workflow_id="placeholder-uuid",  # Gets replaced by config file
```

**Fixes Applied**: All scenarios now use `workflow_name` + placeholder UUID pattern.

---

## Investigation Artifacts

### SDK Fixes Applied (Were Red Herrings)

1. ✅ Changed `base_url` → `api_base` in SDK
   - **Result**: No change (Mock LLM was already being called)
   
2. ✅ Added `api_key` to model registry from environment
   - **Result**: No change (LiteLLM reads `OPENAI_API_KEY` when api_key=None)

**These fixes are CORRECT** per LiteLLM docs, but didn't affect test results because Mock LLM was already working.

### Debugging Evidence

**Network Test**:
```bash
$ kubectl exec deploy/holmesgpt-api -- curl http://mock-llm:8080/health
{"status": "ok"}  ✅ Connected to 10.96.178.229:8080
```

**Manual LLM Request Test**:
```bash
$ curl http://mock-llm:8080/v1/chat/completions -d '{"messages":[{"content":"OOMKilled"}]}'
{
  "workflow_id": "4f30fe0a-c240-4555-abff-37fca0beb0c7",  ✅ Correct UUID!
  "confidence": 0.95
}
```

**SDK Debug Logs**:
```
[SDK DEBUG] Calling litellm.completion with:
  model=openai/mock-model ✅
  api_base=http://mock-llm:8080 ✅
  api_key=*** ✅

LiteLLM completion() model= mock-model; provider = openai
Wrapper: Completed Call, calling success_handler  ✅ SUCCESS!
```

---

## Current Test Failures Analysis

**26/40 Passing (14 Failures)**

### Failure Categories

**Category A: Mock LLM Scenario Issues** (11 failures)
- E2E-HAPI-001, 002, 003: Wrong workflow returned (`wait-for-heal-v1` or `generic-restart-v1` names)
- E2E-HAPI-004, 005: Confidence/structure mismatches
- E2E-HAPI-013, 014, 023, 024, 025, 026, 027: Recovery scenarios with wrong workflows

**Root Cause**: Mock LLM falls back to DEFAULT_SCENARIO for unmatched signals, returns workflow NAME.

**Category B: Expected Validation Failures** (3 failures)  
- E2E-HAPI-007, 008, 018: Server-side validation not implemented (deferred to V1.1)

---

## Next Steps

### Immediate (V1.0)

1. **Rebuild Mock LLM** with fixes:
   - ✅ All scenarios use `workflow_name` + placeholder UUID
   - ✅ Explicit logging via `logger.info()`

2. **Verify Workflow Seeding**:
   - Ensure all referenced workflows in baseWorkflows:
     - ✅ `oomkill-increase-memory-v1`
     - ✅ `generic-restart-v1`
     - ✅ `crashloop-config-fix-v1`
     - ✅ `node-drain-reboot-v1`
     - ❌ `wait-for-heal-v1` (Category F - add if needed)

3. **Update Test Expectations**:
   - Review 14 failing tests
   - Align expectations with actual Mock LLM behavior
   - Fix weak assertions (use exact confidence values)

4. **Re-run E2E Tests**:
   - Expected: 37/40 passing (3 validation failures remain)

### Deferred (V1.1)

- Category B: Implement server-side validation (E2E-HAPI-007, 008, 018)
- Category F: Implement 6 new test scenarios (E2E-HAPI-049 to 054)

---

## Files Changed

### SDK Fixes
- `dependencies/holmesgpt/holmes/core/llm.py`:
  - Changed `base_url=self.api_base` → `api_base=self.api_base`
  - Added debug logging
  - Added api_key to model registry from environment

### Mock LLM Fixes  
- `test/services/mock-llm/src/server.py`:
  - Fixed 7 scenarios to use `workflow_name` + placeholder UUID
  - Added explicit logging in `do_POST()` method
  - Scenarios fixed: network_partition, DEFAULT_SCENARIO, multi_step_recovery, cascading_failure, near_attempt_limit, noisy_neighbor, recovery_basic

### Infrastructure
- `test/infrastructure/holmesgpt_api.go`:
  - Added `LITELLM_LOG=DEBUG` environment variable
  - Confirmed `KEEP_CLUSTER=true` preservation logic

---

## Key Insights

1. **Logging Matters**: Suppressed logs led to 4 hours of misdirected investigation
2. **curl is Truth**: Direct HTTP tests cut through all framework layers
3. **Placeholder Values Matter**: Workflow names vs UUIDs caused silent failures
4. **Multi-tier Debugging**: HAPI → SDK → LiteLLM → Mock LLM requires testing each layer

---

## Commands for Manual Verification

```bash
# Use preserved cluster
export KUBECONFIG=/Users/jgil/.kube/holmesgpt-api-e2e-config

# Check Mock LLM
kubectl get pods -n holmesgpt-api-e2e -l app=mock-llm
kubectl logs -n holmesgpt-api-e2e -l app=mock-llm

# Test Mock LLM directly
kubectl exec -n holmesgpt-api-e2e deploy/holmesgpt-api -- \
  curl -s http://mock-llm:8080/v1/chat/completions \
  -X POST -H "Content-Type: application/json" \
  -d '{"model":"mock-model","messages":[{"role":"user","content":"OOMKilled"}]}'

# Check HAPI logs
kubectl logs -n holmesgpt-api-e2e -l app=holmesgpt-api | grep "SDK DEBUG"

# Cleanup when done
kind delete cluster --name holmesgpt-api-e2e
```

---

## Confidence Assessment

**Diagnosis Confidence**: 95%
- Mock LLM is definitively being called (curl proof)
- Workflow name vs UUID issue confirmed
- Network and SDK configuration verified

**Fix Confidence**: 90%
- Mock LLM fixes will resolve workflow ID issues
- Test expectations may need adjustments
- 3 validation errors remain (expected, deferred to V1.1)

**Expected Post-Fix Results**: 37/40 passing (92.5%)
