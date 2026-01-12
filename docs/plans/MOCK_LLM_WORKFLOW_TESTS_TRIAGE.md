# Mock LLM Migration - Workflow Test Failures Triage

**Date**: January 12, 2026  
**Status**: Root cause identified, fix required in Phase 7  
**Related**: Mock LLM Migration Phase 6-7

## Executive Summary

4 tests are failing due to the **incident analysis endpoint** still using the **embedded mock mode** instead of the standalone Mock LLM service. This is NOT a Python import bug - it's incomplete migration cleanup.

## Failing Tests (4/4)

| Test | Status | Error |
|------|--------|-------|
| `test_incident_analysis_calls_workflow_search_tool` | ❌ FAIL | Tool not called (0 tool_calls) |
| `test_incident_with_detected_labels_passes_to_tool` | ❌ FAIL | Tool not called (0 tool_calls) |
| `test_recovery_analysis_calls_workflow_search_tool` | ❌ FAIL | Tool not called (0 tool_calls) |
| `test_complete_incident_to_recovery_flow_e2e` | ❌ FAIL | `selected_workflow=None` |

## Root Cause Analysis

### Evidence from E2E Logs

**Recovery Endpoint** (✅ Working):
```
'has_tool_calls': True, 'tool_call_count': 1
```

**Incident Endpoint** (❌ Failing):
```
'event': 'mock_mode_active', 
'message': 'Returning deterministic mock response with audit (MOCK_LLM_MODE=true)'
```

### Code Location

**File**: `holmesgpt-api/src/extensions/incident/llm_integration.py`  
**Lines**: 208-214

```python
from src.mock_responses import is_mock_mode_enabled, generate_mock_incident_response
if is_mock_mode_enabled():
    logger.info({
        "event": "mock_mode_active",
        "incident_id": incident_id,
        "message": "Returning deterministic mock response with audit (MOCK_LLM_MODE=true)"
    })
```

### Why It Fails

1. **Embedded mock mode** returns **hardcoded responses** without tool calls
2. **Standalone Mock LLM** returns **proper OpenAI-format responses** with tool calls
3. Incident endpoint checks `MOCK_LLM_MODE=true` → uses old embedded mode
4. Recovery endpoint uses standalone Mock LLM → works correctly

## Impact Assessment

| Metric | Value |
|--------|-------|
| **Tests Affected** | 4 (all incident analysis tests) |
| **Pass Rate Impact** | -10% (from 100% to 90%) |
| **Business Impact** | Medium - E2E coverage incomplete |
| **Technical Debt** | High - duplicate mock systems coexist |

## Solution

### Phase 7: Remove Embedded Mock Code

Remove the embedded mock mode from incident endpoint:

1. **Delete**: `is_mock_mode_enabled()` check in `llm_integration.py`
2. **Delete**: `generate_mock_incident_response()` function
3. **Update**: Always use HolmesGPT SDK (which calls standalone Mock LLM)
4. **Validate**: All 4 tests should pass after removal

### Implementation Complexity

- **Effort**: 15 minutes (simple deletion + verification)
- **Risk**: Low (recovery endpoint already migrated successfully)
- **Testing**: E2E tests will validate immediately

## Timeline

- **Triage**: ✅ Complete (January 12, 2026)
- **Phase 7 Fix**: ⏳ Pending (next task)
- **Validation**: ⏳ Pending (re-run E2E tests)

## Success Criteria

After Phase 7 cleanup:
- ✅ All 4 tests pass
- ✅ 100% E2E pass rate (41/41 tests)
- ✅ No embedded mock code remains in HAPI
- ✅ Only standalone Mock LLM used for E2E tests

## Related Commits

- **Parser fixes**: 60f39d7ed, 7bec44671 (fixed 7 recovery tests)
- **Phase 7**: TBD (will fix these 4 incident tests)

## Notes

- This is **NOT** a Python import bug as originally suspected
- This is **incomplete migration** - some code still references old system
- Phase 7 cleanup will **remove all embedded mock code**, fixing these tests

---

**Conclusion**: Straightforward fix in Phase 7. Remove embedded mock code from incident endpoint, all tests will pass.
