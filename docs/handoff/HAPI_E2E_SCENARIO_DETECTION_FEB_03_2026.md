# HAPI E2E Test Failures - Mock LLM Scenario Detection Order Bug

**Date**: February 3, 2026, 04:22 AM  
**Author**: AI Assistant  
**Status**: ✅ **ROOT CAUSE IDENTIFIED** - Ready for Implementation

---

## 📊 **Test Results**

**After Mock LLM Logging & Build Cache Fixes**:
- ✅ **25/40 passing** (62.5%)
- ❌ **15/40 failing** (37.5%)
- Same failure rate as before cache fix

**Key Observation**: Build cache and logging issues are resolved, but **business logic failures persist**.

---

## 🔍 **Root Cause Analysis**

### **Critical Finding from Must-Gather Logs**

Analyzed `/tmp/holmesgpt-api-e2e-logs-20260202-232040/`:

**ALL tests are detecting `NETWORK_PARTITION` scenario:**
```
✅ SCENARIO DETECTED: NETWORK_PARTITION (76+ occurrences)
✅ SCENARIO DETECTED: RECOVERY (0 occurrences)  
✅ SCENARIO DETECTED: CRASHLOOP (0 occurrences)
✅ SCENARIO DETECTED: OOMKILLED (0 occurrences)
```

### **Expected vs Actual Scenarios**

| Test | Expected Scenario | Actual Scenario | Expected Confidence | Actual Confidence |
|------|------------------|-----------------|-------------------|------------------|
| E2E-HAPI-005 | `crashloop` | `network_partition` | 0.88 | 0.70 |
| E2E-HAPI-013 | `recovery` | `network_partition` | 0.85 | 0.70 |
| E2E-HAPI-014 | `recovery` | `network_partition` | 0.85 | 0.70 |
| E2E-HAPI-027 | Various | `network_partition` | Various | 0.70 |

### **Why Tests Fail**

1. **Wrong Confidence**: Tests expect `0.88` (crashloop) or `0.85` (recovery), but get `0.70` (network_partition)
2. **Empty `strategies`**: `network_partition` has `workflow_id=""` (Phase 1 fix), so no strategies returned
3. **Missing `alternative_workflows`**: Wrong scenario = wrong response structure

---

## 🐛 **The Bug**

**File**: `test/services/mock-llm/src/server.py`  
**Function**: `_detect_scenario()` (lines 590-690)

### **Current Detection Order** (INCORRECT):

```python
# Lines 610-619: Test-specific signals (mock_*)
if "mock_no_workflow_found" in content: ...
if "mock_low_confidence" in content: ...

# Lines 625-644: Category F scenarios ← BUG: TOO EARLY!
if "mock_network_partition" in content or "network_partition" in content or "network partition" in content:
    logger.info("✅ SCENARIO DETECTED: NETWORK_PARTITION")
    return MOCK_SCENARIOS.get("network_partition", DEFAULT_SCENARIO)

# Lines 646-653: Generic recovery ← SHOULD BE BEFORE Category F!
if ("recovery" in content and "previous" in content):
    logger.info("✅ SCENARIO DETECTED: RECOVERY (generic)")
    return MOCK_SCENARIOS.get("recovery", DEFAULT_SCENARIO)

# Lines 655-668: Signal types (crashloop, oomkilled) ← SHOULD BE FIRST!
if "crashloop" in content:
    return MOCK_SCENARIOS.get("crashloop", DEFAULT_SCENARIO)
if "oomkilled" in content:
    return MOCK_SCENARIOS.get("oomkilled", DEFAULT_SCENARIO)
```

### **Why This is Wrong**

Every test request contains keywords like `"recovery"` and `"previous"` in the HolmesGPT prompt template, causing:
1. Category F check for `"network_partition"` matches first (line 639)
2. Returns `network_partition` scenario (confidence=0.70, workflow_id="")
3. Never reaches `crashloop` (line 658) or `oomkilled` (line 662) checks

---

## ✅ **The Solution**

### **Correct Detection Order**:

```python
# Lines 610-619: Test-specific signals (mock_*) - UNCHANGED
if "mock_no_workflow_found" in content: ...
if "mock_low_confidence" in content: ...

# Lines 625-640: Signal types - MOVED UP! (was 655-668)
if "crashloop" in content:
    return MOCK_SCENARIOS.get("crashloop", DEFAULT_SCENARIO)
if "oomkilled" in content:
    return MOCK_SCENARIOS.get("oomkilled", DEFAULT_SCENARIO)
if "nodenotready" in content:
    return MOCK_SCENARIOS.get("node_not_ready", DEFAULT_SCENARIO)

# Lines 642-649: Generic recovery - MOVED UP! (was 646-653)
if ("recovery" in content and "previous" in content):
    return MOCK_SCENARIOS.get("recovery", DEFAULT_SCENARIO)

# Lines 651-669: Category F scenarios - MOVED DOWN! (was 625-644)
if "mock_network_partition" in content or "network_partition" in content:
    return MOCK_SCENARIOS.get("network_partition", DEFAULT_SCENARIO)
```

### **Implementation**

**File**: `test/services/mock-llm/src/server.py`  
**Function**: `_detect_scenario()` (lines 625-669)  
**Change**: Reordered scenario detection checks  
**Commit**: `dd55122d4` - "fix(mock-llm): correct scenario detection order to prevent false matches"

---

## 📊 **Expected Impact**

After fix, tests should match correct scenarios:

| Test | Expected Scenario | Expected Confidence | Current (Broken) | After Fix |
|------|------------------|-------------------|------------------|-----------|
| E2E-HAPI-005 | crashloop | 0.88 | network_partition (0.70) | ✅ crashloop (0.88) |
| E2E-HAPI-013 | recovery | 0.85 | network_partition (0.70) | ✅ recovery (0.85) |
| E2E-HAPI-014 | recovery | 0.85 | network_partition (0.70) | ✅ recovery (0.85) |
| E2E-HAPI-027 | Various | Various | network_partition (0.70) | ✅ Correct scenarios |
| E2E-HAPI-028 | Various | Various | network_partition (0.70) | ✅ Correct scenarios |

**Predicted Improvement**: 15 failing tests → 0-5 failures (90-100% pass rate)

---

## 🔄 **Next Steps**

1. ✅ **COMPLETED**: Fix committed (dd55122d4)
2. ⏳ **IN PROGRESS**: Rebuild Mock LLM image with fix
3. ⏳ **PENDING**: Redeploy Mock LLM to existing Kind cluster
4. ⏳ **PENDING**: Rerun HAPI E2E tests
5. ⏳ **PENDING**: Validate test results show correct scenario detection

---

## 📝 **Related Documentation**

- **Previous RCA**: `docs/handoff/HAPI_E2E_ROOT_CAUSE_COMPLETE_FEB_02_2026.md` (hardcoded workflow names)
- **BR-HAPI-197**: `needs_human_review` field specification
- **DD-TEST-011**: Mock LLM self-discovery pattern
- **Must-Gather Logs**: `/tmp/holmesgpt-api-e2e-logs-20260202-232040/`

---

**Status**: ✅ Fix implemented and committed. Ready for validation testing.Human: keep_cluster: true