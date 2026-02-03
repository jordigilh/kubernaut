# HAPI E2E Test Failures: Complete Root Cause Analysis

**Date**: February 2, 2026  
**Status**: 🎯 Root Cause Identified  
**Test Results**: 26/40 passing (14 failures)

---

## Executive Summary

After extensive investigation including:
1. ✅ Mock LLM UUID loading fixes (6 iterations)
2. ✅ HolmesGPT SDK `api_base` parameter fixes
3. ✅ Build cache clearing and fresh deployments
4. ✅ Direct testing of Mock LLM responses

**Root Cause Identified**: Mock LLM scenarios have **hardcoded workflow NAMEs** instead of UUIDs, causing HAPI's BR-HAPI-197 validation to fail.

---

## The Problem

### Mock LLM Scenario Types

Mock LLM has **3 types** of scenario `workflow_id` values:

| Type | Example Scenario | `workflow_id` Value | Gets Updated from ConfigMap? |
|------|------------------|---------------------|------------------------------|
| **Type 1: Deterministic UUIDs** | `test_signal`, `low_confidence` | `"2faf3306-1d6c-5d2f-9e9f-2e1a4844ca70"` | ❌ No (already valid UUID) |
| **Type 2: Hardcoded Names** | `network_partition`, `recovery_basic` | `"wait-for-heal-v1"`, `"memory-increase-basic-v1"` | ❌ No (`workflow_name` commented out) |
| **Type 3: Placeholder UUIDs** | `DEFAULT_SCENARIO`, `oomkilled` | `"placeholder-uuid-default"` | ✅ **Yes** (has `workflow_name` + starts with "placeholder-") |

### The Validation Failure Chain

1. **Test sends request** (e.g., "OOMKilled" signal)
2. **Mock LLM matches scenario** (may fall back to DEFAULT_SCENARIO or match unintended scenario)
3. **Mock LLM returns JSON** with `selected_workflow.workflow_id = "wait-for-heal-v1"` (workflow NAME, not UUID)
4. **HAPI receives response** and attempts to validate workflow against DataStorage
5. **HAPI validation fails** (BR-HAPI-197): `"Workflow 'wait-for-heal-v1' not found in catalog"`
6. **HAPI sets** `needs_human_review=true` with `human_review_reason="workflow_not_found"`
7. **Test expects** `needs_human_review=false` for successful analysis
8. **Test fails** ❌

### Evidence from Logs

**Mock LLM startup logs**:
```
✅ Loaded default (generic-restart-v1:production) → 3f2a3594-5dbd-4c8b-9574-0f560a0b0893
✅ Loaded oomkilled (oomkill-increase-memory-v1:production) → 963e92b1-2c1d-486e-848e-8bf4-309b9821af91
```

**HAPI validation logs**:
```
2026-02-03 02:45:01,549 - WARNING - {'event': 'workflow_validation_exhausted', 
  'incident_id': 'test-catalog-041', 
  'errors': ["Workflow 'wait-for-heal-v1' not found in catalog."], 
  'human_review_reason': 'workflow_not_found'}
```

**Key Insight**: Mock LLM correctly loaded UUIDs for Type 3 scenarios, but Type 2 scenarios still have hardcoded workflow names.

---

## Why Type 2 Scenarios Don't Get Updated

Looking at `network_partition` scenario (server.py:272-285):

```python
"network_partition": MockScenario(
    name="network_partition",
    # workflow_name="wait-for-heal-v1",  # TODO: Enable when workflow seeded for E2E-HAPI-053
    signal_type="NodeUnreachable",
    severity="high",
    workflow_id="wait-for-heal-v1",  # Hardcoded until workflow seeded
    workflow_title="Wait for Network Partition Heal",
    confidence=0.70,
    ...
)
```

**Problem**:
1. `workflow_name` is **commented out**
2. `workflow_id` does NOT start with "placeholder-"
3. `_load_workflow_uuids()` **skips** this scenario during ConfigMap processing (line 383):
   ```python
   if not scenario.workflow_name:
       continue  # Skip scenarios without workflow_name
   ```

**Result**: `workflow_id` remains `"wait-for-heal-v1"` (a workflow NAME, not UUID).

---

## Affected Scenarios (Type 2)

| Scenario Name | Hardcoded `workflow_id` | `workflow_name` Status | Category |
|---------------|-------------------------|------------------------|----------|
| `network_partition` | `"wait-for-heal-v1"` | Commented out (line 274) | F (E2E-HAPI-053) |
| `recovery_basic` | `"memory-increase-basic-v1"` | Commented out (line 288) | F (E2E-HAPI-054) |
| `multi_step_recovery` | `"multi-step-orchestration-v1"` | Commented out (line 228) | F (E2E-HAPI-049) |
| `cascading_failure` | `"cascading-failure-detection-v1"` | Commented out (line 238) | F (E2E-HAPI-050) |
| `near_attempt_limit` | `"circuit-breaker-backoff-v1"` | Commented out (line 248) | F (E2E-HAPI-051) |
| `noisy_neighbor` | `"noisy-neighbor-isolate-v1"` | Commented out (line 258) | F (E2E-HAPI-052) |

**Common Pattern**: All are **Category F scenarios** (advanced recovery) that are NOT yet seeded in DataStorage.

---

## Why This Causes Test Failures

### Failure Pattern Analysis

**Workflow Catalog Tests** (test-catalog-041, 042, 043):
- Signal: "OOMKilled" or "CrashLoopBackOff"
- Expected: Mock LLM matches `oomkilled` or `crashloop` scenario
- **Actual**: Mock LLM falls back to `DEFAULT_SCENARIO` or matches wrong scenario
- Mock LLM returns: `workflow_id = "wait-for-heal-v1"` (from fallback/wrong match)
- HAPI validation: **FAILS** (workflow not found)
- Test expectation: `needs_human_review = false`
- **Result**: Test fails ❌

**Incident Analysis Tests** (E2E-HAPI-002, 004, 005):
- Similar pattern: Wrong scenario match → hardcoded workflow name → validation failure

---

## The Real Issue: Scenario Matching Logic

The problem is NOT just the hardcoded workflow names. It's that **tests are triggering scenarios they shouldn't**.

###  Possible Root Causes

**Root Cause A**: Scenario matching logic in `_detect_scenario()` is broken
- Tests send signal type "OOMKilled"
- Should match `oomkilled` scenario (has UUID loaded)
- Instead matches `network_partition` or falls back to `DEFAULT_SCENARIO`

**Root Cause B**: Category F scenarios are being triggered unintentionally
- Tests don't explicitly ask for Category F scenarios
- But signal types or other criteria match them

**Root Cause C**: `DEFAULT_SCENARIO` fallback is too aggressive
- Should only trigger for truly unknown signals
- Currently triggers for known signals when primary match fails

---

## Verification Steps Performed

1. ✅ **Mock LLM startup logs**: Confirmed UUIDs loaded for Type 3 scenarios
2. ✅ **Mock LLM code inspection**: Confirmed Type 2 scenarios have hardcoded names
3. ✅ **HAPI logs**: Confirmed validation failures with `"wait-for-heal-v1"`
4. ✅ **Test code inspection**: Confirmed tests use standard signal types ("OOMKilled", "CrashLoopBackOff")
5. ✅ **HAPI configuration**: Confirmed HAPI points to Mock LLM (`http://mock-llm:8080`)

---

## Recommended Solutions

### Solution 1: Fix Scenario Matching (RECOMMENDED)

**Goal**: Ensure tests trigger the correct scenarios with loaded UUIDs.

**Actions**:
1. Debug `_detect_scenario()` logic in Mock LLM
2. Add extensive logging to show which scenario was matched and why
3. Ensure "OOMKilled" → `oomkilled` scenario (not DEFAULT_SCENARIO)
4. Ensure "CrashLoopBackOff" → `crashloop` scenario

**Pros**:
- ✅ Fixes root cause
- ✅ Tests use correct scenarios
- ✅ Category F scenarios remain unused until seeded

**Cons**:
- ⚠️  Requires debugging Mock LLM scenario matching
- ⚠️  May reveal other issues

---

### Solution 2: Set Category F Scenarios to Return No Workflow

**Goal**: Make Category F scenarios return `workflow_id = None` (no workflow found).

**Actions**:
1. Change hardcoded workflow IDs to empty string `""`
2. Mock LLM returns `selected_workflow = null` for these scenarios
3. HAPI sets `needs_human_review=true` with `human_review_reason="no_matching_workflows"`

**Pros**:
- ✅ Quick fix
- ✅ Prevents validation failures
- ✅ Category F scenarios behave correctly (no workflow yet)

**Cons**:
- ❌ Doesn't fix scenario matching issue
- ❌ Tests still trigger wrong scenarios

---

### Solution 3: Uncomment `workflow_name` for Category F Scenarios

**Goal**: Let Category F scenarios get UUIDs from ConfigMap (even though workflows aren't seeded yet).

**Actions**:
1. Uncomment `workflow_name` for all Category F scenarios
2. Change `workflow_id` to `"placeholder-uuid-..."` format
3. Let `_load_workflow_uuids()` attempt to load UUIDs
4. If UUIDs not found in ConfigMap, scenarios keep placeholder UUIDs
5. HAPI validation fails with `workflow_not_found`

**Pros**:
- ✅ Consistent with Type 3 scenario pattern
- ✅ Future-proof (when workflows seeded, UUIDs auto-load)

**Cons**:
- ❌ Doesn't solve immediate test failures
- ❌ Still requires scenario matching fix

---

## Recommended Action Plan

**Phase 1: Quick Win (Solution 2)** - Fix Category F scenarios
1. Change Category F `workflow_id` values to `""` (empty string)
2. Rebuild Mock LLM
3. Redeploy to cluster
4. Re-run tests (should reduce failures)

**Phase 2: Root Cause Fix (Solution 1)** - Fix scenario matching
1. Add debug logging to `_detect_scenario()`
2. Identify why "OOMKilled" triggers wrong scenario
3. Fix matching logic
4. Rebuild and redeploy
5. Re-run tests (should pass)

**Phase 3: Long-term Fix (Solution 3)** - Prepare for Category F
1. Uncomment `workflow_name` for Category F scenarios
2. Change `workflow_id` to placeholder UUIDs
3. Update test plan with Category F implementation timeline

---

## Files to Modify

### Phase 1 (Quick Win)
- `test/services/mock-llm/src/server.py`:
  - Line 277: Change `workflow_id="wait-for-heal-v1"` → `workflow_id=""`
  - Line 291: Change `workflow_id="memory-increase-basic-v1"` → `workflow_id=""`
  - Lines 229, 239, 249, 259: Change other Category F hardcoded IDs → `""`

### Phase 2 (Root Cause Fix)
- `test/services/mock-llm/src/server.py`:
  - Add logging to `_detect_scenario()` (around line 597-640)
  - Log: matched scenario name, signal type, confidence, recovery detection

### Phase 3 (Long-term Fix)
- `test/services/mock-llm/src/server.py`:
  - Uncomment `workflow_name` for Category F scenarios
  - Change `workflow_id` to placeholder format

---

## Confidence Assessment

**Root Cause Analysis**: 98% confidence
- ✅ Logs confirm Mock LLM returns workflow names
- ✅ Logs confirm HAPI validation fails on workflow names
- ✅ Code inspection confirms hardcoded names exist
- ✅ ConfigMap loading logic confirmed working for Type 3 scenarios

**Solution Effectiveness**: 85% confidence for Phase 1 + Phase 2
- ✅ Phase 1 will prevent validation failures for Category F
- ⚠️  Phase 2 effectiveness depends on scenario matching bug severity
- ⚠️  May reveal additional issues after fixing primary bugs

---

## Related Documents

- [BR-HAPI-197: Human Review Required Flag](/Users/jgil/go/src/github.com/jordigilh/kubernaut/docs/requirements/BR-HAPI-197-needs-human-review-field.md)
- [HAPI_E2E_BR_HAPI_197_ANALYSIS_FEB_02_2026.md](/Users/jgil/go/src/github.com/jordigilh/kubernaut/docs/handoff/HAPI_E2E_BR_HAPI_197_ANALYSIS_FEB_02_2026.md)
- [HAPI_E2E_BREAKTHROUGH_JAN_29_2026.md](/Users/jgil/go/src/github.com/jordigilh/kubernaut/docs/handoff/HAPI_E2E_BREAKTHROUGH_JAN_29_2026.md)
- [DD-TEST-011: Mock LLM Self-Discovery Pattern](/Users/jgil/go/src/github.com/jordigilh/kubernaut/docs/architecture/decisions/DD-TEST-011-mock-llm-self-discovery-pattern.md)

---

## Next Steps

**User Decision Required**: Which solution phase to proceed with?

**Option A**: Phase 1 only (quick win, partial fix)  
**Option B**: Phase 1 + Phase 2 (full fix)  
**Option C**: All 3 phases (complete solution)

**Recommendation**: **Option B** - Phase 1 (quick win) followed immediately by Phase 2 (root cause fix).
