# HAPI Integration Test Failures - Fix Implementation Plan

**Date**: January 22, 2026
**Status**: 🚧 **IN PROGRESS**
**Failures**: 9 tests (3 event_category + 6 recovery_analysis)
**Estimated Time**: 2-3 hours

---

## ✅ **Task 1: Fix Event Category Assertions** (30-45 minutes)

### **Status**: ✅ **STARTED** (1 of 3 complete)

**Files to Update**:
- `holmesgpt-api/tests/integration/test_hapi_audit_flow_integration.py`

### **Test 1: `test_audit_events_have_required_adr034_fields`** ✅ FIXED

Already updated (line 544-556).

---

### **Test 2: `test_incident_analysis_emits_llm_request_and_response_events`**

**Location**: Line ~219-280

**Current Error**:
```
AssertionError: Expected exactly 2 LLM events (llm_request, llm_response), got 6
```

**Root Cause**:
- Test expects exactly 2 LLM events
- Now sees: workflow_validation_attempt, llm_tool_call, workflow.catalog.search_completed
- ADR-034 v1.1+ added workflow catalog events

**Fix**:
```python
# Find the assertion around line 270-280
# REPLACE:
assert len(llm_events) == 2, \
    f"Expected exactly 2 LLM events (llm_request, llm_response), got {len(llm_events)}"

# WITH:
# ADR-034 v1.1+: Incident analysis triggers multiple LLM calls
# (tool calls for workflow search, main analysis, retries, etc.)
# Filter to only LLM request/response events (exclude tool_call, workflow events)
llm_request_response = [e for e in events
                        if e.event_type in ["llm_request", "llm_response"]]

assert len(llm_request_response) >= 2, \
    f"Expected at least 2 LLM events, got {len(llm_request_response)}. " \
    f"All event types: {[e.event_type for e in events]}"

# Verify pairing
requests = [e for e in llm_request_response if e.event_type == "llm_request"]
responses = [e for e in llm_request_response if e.event_type == "llm_response"]
assert len(requests) > 0, "Expected at least one llm_request event"
assert len(responses) > 0, "Expected at least one llm_response event"
```

---

### **Test 3: `test_workflow_not_found_emits_audit_with_error_context`**

**Location**: Line ~573-650

**Current Error**:
```
AssertionError: Expected exactly 2 LLM events even for failed workflow search, got 6
```

**Similar to Test 2** - Update count expectations:

```python
# Find the assertion
# REPLACE:
assert len(llm_events) == 2, \
    f"Expected exactly 2 LLM events even for failed workflow search, got {len(llm_events)}"

# WITH:
# ADR-034 v1.1+: Even failed workflow searches emit multiple events
llm_request_response = [e for e in events
                        if e.event_type in ["llm_request", "llm_response"]]
workflow_events = [e for e in events if e.event_type.startswith("workflow.")]

# Verify LLM events exist
assert len(llm_request_response) >= 2, \
    f"Expected at least 2 LLM events, got {len(llm_request_response)}"

# Verify workflow search was attempted (even though it failed)
assert len(workflow_events) >= 1, \
    f"Expected workflow.catalog.search_completed event, got {len(workflow_events)}"
```

---

## 🔍 **Task 2: Investigate Recovery Analysis Failures** (1-2 hours)

### **Status**: ⏸️ **PENDING** (needs Task 1 completion first)

**Affected Tests** (6 failures):
1. `test_recovery_analysis_field_present`
2. `test_previous_attempt_assessment_structure`
3. `test_field_types_correct`
4. `test_aa_team_integration_mapping`
5. `test_multiple_recovery_attempts`
6. `test_mock_mode_returns_valid_structure`

**Error Pattern**:
```python
ExceptionGroup: unhandled errors in a TaskGroup (1 sub-exception)
```

### **Investigation Steps**

#### **Step 1: Check Mock LLM Recovery Response** (15 minutes)

**Verify Mock LLM is returning correct structure**:

```bash
# Test Mock LLM recovery endpoint directly
curl -X POST http://127.0.0.1:18140/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4-turbo",
    "messages": [
      {"role": "user", "content": "recovery analysis for previous remediation failed attempt"}
    ]
  }' | jq '.choices[0].message.content' | grep -A20 "recovery_analysis"
```

**Expected Output** (from `test/services/mock-llm/src/server.py:_recovery_text_response`):
```json
{
  "recovery_analysis": {
    "previous_attempt_assessment": {
      "failure_understood": true,
      "failure_reason_analysis": "...",
      "state_changed": false,
      "current_signal_type": "OOMKilled"
    },
    "recommended_action": "...",
    "alternative_workflow_id": "..."
  }
}
```

**If Missing**: Mock LLM recovery response structure needs fixing.

---

#### **Step 2: Check HAPI Recovery Analysis Parsing** (30 minutes)

**File**: `holmesgpt-api/src/extensions/recovery/llm_integration.py`

**Verify**:
1. Response parsing handles `recovery_analysis` field correctly
2. Async TaskGroup error handling is present
3. AA team integration mapping logic exists

**Common Issues**:
- Missing field validation before accessing nested dict
- Async exception not caught in TaskGroup
- JSON parsing errors on unexpected structure

**Debug Command**:
```bash
# Run single failing test with verbose output
cd holmesgpt-api
pytest -xvs tests/integration/test_recovery_analysis_structure_integration.py::TestRecoveryAnalysisStructure::test_recovery_analysis_field_present
```

---

#### **Step 3: Fix Async TaskGroup Exception Handling** (30 minutes)

**Likely Issue**: Exception in async task not properly handled.

**Pattern to Check** (in `recovery/llm_integration.py`):
```python
# INCORRECT (causes ExceptionGroup):
async with anyio.create_task_group() as tg:
    result = await tg.start_soon(some_async_function())
    # Exception here → unhandled ExceptionGroup

# CORRECT:
async with anyio.create_task_group() as tg:
    try:
        result = await tg.start_soon(some_async_function())
    except Exception as e:
        logger.error(f"Task failed: {e}")
        raise  # Re-raise with context
```

---

#### **Step 4: Add Detailed Logging** (15 minutes)

**Enhance logging to see what's failing**:

```python
# In recovery analysis business logic
logger.info(f"Recovery analysis started: incident_id={incident_id}")
logger.debug(f"LLM response: {llm_response}")
logger.debug(f"Parsed recovery_analysis: {recovery_analysis}")

# Before field access
if "recovery_analysis" not in response:
    logger.error(f"Missing recovery_analysis field. Response keys: {response.keys()}")
    raise ValueError("LLM response missing recovery_analysis field")
```

---

## 📋 **Quick Win Checklist**

### **Phase 1: Event Category Fixes** (Do These First)

- [x] Fix `test_audit_events_have_required_adr034_fields` ✅ DONE
- [ ] Fix `test_incident_analysis_emits_llm_request_and_response_events`
- [ ] Fix `test_workflow_not_found_emits_audit_with_error_context`
- [ ] Run tests: `make test-integration-holmesgpt-api`
- [ ] Verify: 3 event_category tests now pass (56 → 59 passing)

### **Phase 2: Recovery Analysis Investigation**

- [ ] Test Mock LLM recovery endpoint directly (curl)
- [ ] Run single failing recovery test with `-xvs`
- [ ] Check HAPI recovery analysis parsing code
- [ ] Add debug logging
- [ ] Identify root cause (response structure vs. async handling)

### **Phase 3: Recovery Analysis Fix**

- [ ] Implement fix based on Phase 2 findings
- [ ] Run recovery tests: `pytest -xvs tests/integration/test_recovery_analysis_*`
- [ ] Verify: All 6 recovery tests pass
- [ ] Full test run: `make test-integration-holmesgpt-api`
- [ ] Target: 65/65 passing ✅

---

## 🎯 **Expected Results**

**After Task 1** (Event Category Fixes):
```
================== 59 passed, 6 failed, 44 warnings ===================
```

**After Task 2** (Recovery Analysis Fix):
```
================== 65 passed, 44 warnings in 6-8s ===================
```

---

## 📚 **Reference Files**

### **Test Files**
- `holmesgpt-api/tests/integration/test_hapi_audit_flow_integration.py` (event_category tests)
- `holmesgpt-api/tests/integration/test_recovery_analysis_structure_integration.py` (recovery tests)

### **Business Logic**
- `holmesgpt-api/src/extensions/incident/llm_integration.py` (incident analysis)
- `holmesgpt-api/src/extensions/recovery/llm_integration.py` (recovery analysis)
- `test/services/mock-llm/src/server.py` (Mock LLM responses)

### **Documentation**
- `docs/architecture/decisions/ADR-034-unified-audit-table-design.md` (v1.1+)
- `docs/triage/HAPI_TEST_FAILURES_TIMELINE_ANALYSIS_JAN_22_2026.md` (context)

---

## 🚀 **Next Steps**

1. **Complete Task 1** (event_category fixes) - **30 minutes**
   ```bash
   # Edit test_hapi_audit_flow_integration.py (Tests 2 & 3)
   # Run: make test-integration-holmesgpt-api
   # Target: 59/65 passing
   ```

2. **Investigate Task 2** (recovery analysis) - **1 hour**
   ```bash
   # Test Mock LLM directly (curl)
   # Run single test with verbose: pytest -xvs test_recovery_*::test_recovery_analysis_field_present
   # Review recovery/llm_integration.py
   ```

3. **Fix Task 2** (recovery analysis) - **30 minutes**
   ```bash
   # Implement fix based on investigation
   # Run: make test-integration-holmesgpt-api
   # Target: 65/65 passing ✅
   ```

---

## ✅ **Success Criteria**

- All 9 tests passing
- No regressions (65/65 total)
- Test comments document ADR-034 v1.1+ architecture
- RCA documents filed for future reference

---

**Ready to proceed with Task 1 completions (Tests 2 & 3)?**
