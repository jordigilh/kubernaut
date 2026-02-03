# HAPI Mock LLM Root Cause Analysis - Complete Investigation

**Date**: January 29, 2026  
**Issue**: 14/40 HAPI E2E test failures  
**Root Cause**: Mock LLM was being called successfully, but had wrong workflow IDs for some scenarios

---

## Executive Summary

After extensive investigation including:
- Multiple SDK parameter fixes
- Network connectivity testing
- API key troubleshooting
- Log analysis

**The Real Problem**: Mock LLM scenarios have workflow NAMES (not UUIDs) hardcoded for:
1. `network_partition` scenario: `workflow_id="wait-for-heal-v1"`
2. `DEFAULT_SCENARIO`: `workflow_id="generic-restart-v1"`

These workflows are NOT in the seeded workflow catalog, so:
- HAPI validates and finds "wait-for-heal-v1" doesn't exist
- Tests fail expecting correct UUIDs for valid workflows

---

## Investigation Journey

### What We Thought Was Wrong

1. **Hypothesis 1**: SDK using `base_url` instead of `api_base`
   - **Fix Applied**: Changed SDK to use `api_base`
   - **Result**: No change (26/40 still)

2. **Hypothesis 2**: Missing API key
   - **Fix Applied**: Added api_key to model registry from `OPENAI_API_KEY`
   - **Result**: No change (26/40 still)

3. **Hypothesis 3**: Network connectivity issues
   - **Test**: `curl http://mock-llm:8080/health` from HAPI pod
   - **Result**: ✅ Connected successfully

4. **Hypothesis 4**: LiteLLM not calling Mock LLM
   - **Evidence**: SDK debug logs showed `api_base=http://mock-llm:8080`
   - **Evidence**: LiteLLM logs showed "Completed Call, calling success_handler"
   - **Result**: Mock LLM WAS being called!

### What Actually Was Wrong

**Mock LLM logging was suppressed** via `log_message()` method, making it appear like zero requests.

**Manual curl test proved**: Mock LLM responds correctly with proper UUIDs for scenarios that have them in ConfigMap.

**The real bug**: Two Mock LLM scenarios use workflow NAMES instead of placeholder UUIDs:
- `network_partition`: `workflow_id="wait-for-heal-v1"` (line 268)
- `DEFAULT_SCENARIO`: `workflow_id="generic-restart-v1"` (line 297)

When ConfigMap doesn't have these workflows, Mock LLM returns workflow NAMES, not UUIDs.

---

## Evidence

### Curl Test Results

**OOMKilled scenario** (has UUID in ConfigMap):
```json
{
  "workflow_id": "4f30fe0a-c240-4555-abff-37fca0beb0c7",  ✅ UUID
  "confidence": 0.95
}
```

**NodeUnreachable scenario** (NO UUID in ConfigMap, fallback to DEFAULT):
```json
{
  "workflow_id": "generic-restart-v1",  ❌ Workflow NAME
  "confidence": 0.75
}
```

### HAPI Logs Confirm

```
LiteLLM completion() model= mock-model; provider = openai
Wrapper: Completed Call, calling success_handler  ← SUCCESS!
Workflow 'wait-for-heal-v1' not found in catalog  ← HAPI tries to validate
```

### ConfigMap Analysis

ConfigMap has 12 workflows seeded:
- ✅ `oomkill-increase-memory-v1`
- ✅ `generic-restart-v1`  
- ✅ `crashloop-config-fix-v1`
- ✅ `node-drain-reboot-v1`
- ❌ NO `wait-for-heal-v1` (Category F workflow)
- ❌ NO other Category F workflows

---

## The Fix

**Mock LLM Changes** (`test/services/mock-llm/src/server.py`):

1. **network_partition scenario** (line 264-276):
   ```python
   # OLD:
   workflow_id="wait-for-heal-v1",  # Hardcoded NAME
   
   # NEW:
   workflow_name="wait-for-heal-v1",  # Enable UUID loading
   workflow_id="placeholder-uuid-network-partition",  # Gets replaced
   ```

2. **DEFAULT_SCENARIO** (line 293-302):
   ```python
   # OLD:
   workflow_id="generic-restart-v1",  # Hardcoded NAME
   
   # NEW:
   workflow_name="generic-restart-v1",  # Enable UUID loading
   workflow_id="placeholder-uuid-default",  # Gets replaced
   ```

3. **Add request logging** (line 494-519):
   ```python
   # Use logger.info() instead of suppressed log_message()
   logger.info(f"📥 Mock LLM received POST {self.path}")
   ```

**Workflow Seeding**: Ensure `wait-for-heal-v1` and Category F workflows are seeded into DataStorage.

---

## Why The Investigation Was Complex

1. **Logging Suppression**: `log_message()` method hid all HTTP requests
2. **Multi-layered Architecture**: HAPI → SDK → LiteLLM → Mock LLM (many failure points)
3. **Silent Fallbacks**: Mock LLM falls back to DEFAULT_SCENARIO when no match
4. **Workflow Name vs UUID Confusion**: Some scenarios had names, some had UUIDs

---

## Lessons Learned

1. **Test logging carefully**: Suppressed logs can hide critical debugging information
2. **Validate with curl**: Direct HTTP tests bypass framework layers
3. **Check placeholder values**: Hardcoded workflow names broke UUID mapping
4. **ConfigMap content matters**: Mock LLM depends on complete workflow catalog

---

## Next Steps

1. Rebuild Mock LLM with fixes (workflow_name fields + logging)
2. Seed Category F workflows into DataStorage
3. Verify Mock LLM ConfigMap includes all workflow UUIDs
4. Re-run E2E tests (expected: 37/40 passing, 3 validation errors deferred)

---

## Files Changed

- `dependencies/holmesgpt/holmes/core/llm.py`: SDK uses `api_base`, adds api_key to model registry
- `test/services/mock-llm/src/server.py`: Fix workflow_id placeholders, add logging
- `test/infrastructure/holmesgpt_api_helpers.go`: Need to add Category F workflows for seeding
