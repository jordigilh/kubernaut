# HAPI E2E Audit Event Issue - Root Cause Analysis

**Date**: December 26, 2025
**Team**: HAPI Team
**Priority**: P1 - Test Blocker
**Status**: 🔍 ROOT CAUSE IDENTIFIED

---

## 🎯 **Problem Summary**

HAPI E2E tests are failing because audit events are not being persisted to Data Storage when running in `MOCK_LLM_MODE=true`.

**Test Failure**:
```
AssertionError: llm_request event not found. Found events: []
```

---

## 🔍 **Root Cause**

### **Code Flow Issue**

In `src/extensions/incident/llm_integration.py`:

```python
195: def analyze_incident(request_data: dict) -> dict:
196:     # ...
202:     # BR-HAPI-212: Check mock mode BEFORE any LLM-related initialization
203:     from src.mock_responses import is_mock_mode_enabled, generate_mock_incident_response
204:     if is_mock_mode_enabled():
205:         logger.info({
206:             "event": "mock_mode_active",
207:             "incident_id": incident_id,
208:             "message": "Returning deterministic mock response (MOCK_LLM_MODE=true)"
209:         })
210:         return generate_mock_incident_response(request_data)  # ❌ EARLY RETURN
211:
212:     # ... (Normal LLM flow continues below) ...
344:     # BR-AUDIT-005: Get audit store for LLM interaction tracking
345:     audit_store = get_audit_store()  # ⚠️ NEVER REACHED IN MOCK MODE
346:     # ...
395:     # AUDIT: LLM REQUEST (BR-AUDIT-005)
396:     audit_store.store_audit(create_llm_request_event(...))  # ⚠️ NEVER REACHED IN MOCK MODE
```

**Problem**: Early return at line 210 bypasses:
- Audit store initialization (line 345)
- LLM request audit event (line 396+)
- LLM response audit event (line 433+)
- Validation attempt audit event (line 493+)

---

## 📋 **Business Requirement Violation**

**BR-AUDIT-005**: Audit Trail for LLM Interactions
- **Requirement**: "All LLM interactions MUST be audited"
- **Violation**: Mock LLM responses are NOT audited
- **Impact**: E2E tests cannot verify audit event persistence

**ADR-032 §1**: Audit is MANDATORY
- **Requirement**: "Audit writes are MANDATORY, not best-effort"
- **Violation**: Mock mode silently skips audit
- **Impact**: No audit trail for E2E test scenarios

---

## 🎯 **Proposed Solutions**

### **Option A: Add Audit to Mock Response Flow** (RECOMMENDED)

**Rationale**: Mock responses represent real LLM interactions in E2E tests and should be audited.

**Implementation**:
```python
def analyze_incident(request_data: dict) -> dict:
    incident_id = request_data.get("incident_id", "")

    # Initialize audit store BEFORE mock check
    audit_store = get_audit_store()
    remediation_id = request_data.get("remediation_id", "")

    # BR-HAPI-212: Check mock mode
    if is_mock_mode_enabled():
        logger.info({
            "event": "mock_mode_active",
            "incident_id": incident_id,
            "message": "Returning deterministic mock response with audit (MOCK_LLM_MODE=true)"
        })

        # ✅ AUDIT: LLM REQUEST (even for mock)
        audit_store.store_audit(create_llm_request_event(
            incident_id=incident_id,
            remediation_id=remediation_id,
            prompt="MOCK LLM REQUEST",
            model="mock://test-model",
            provider="mock"
        ))

        # Generate mock response
        result = generate_mock_incident_response(request_data)

        # ✅ AUDIT: LLM RESPONSE (even for mock)
        audit_store.store_audit(create_llm_response_event(
            incident_id=incident_id,
            remediation_id=remediation_id,
            has_analysis=True,
            analysis_length=len(result.get("analysis", "")),
            tool_call_count=0
        ))

        # ✅ AUDIT: VALIDATION ATTEMPT (even for mock)
        audit_store.store_audit(create_validation_attempt_event(
            incident_id=incident_id,
            remediation_id=remediation_id,
            attempt=1,
            max_attempts=3,
            workflow_id=result.get("selected_workflow", {}).get("workflow_id"),
            is_valid=True,
            validation_errors=[]
        ))

        return result

    # ... (Normal LLM flow continues) ...
```

**Pros**:
- ✅ Complies with BR-AUDIT-005 (all LLM interactions audited)
- ✅ E2E tests can verify audit event persistence
- ✅ Mock responses have same audit trail as real LLM calls
- ✅ Maintains test realism

**Cons**:
- Adds ~20 lines of code to mock path
- Mock audit events have synthetic data (e.g., "MOCK LLM REQUEST")

### **Option B: Skip Audit Tests in Mock Mode**

**Rationale**: Audit events are only generated for real LLM calls, not mocks.

**Implementation**:
- Add `pytest.skip("Audit tests require real LLM, not supported in MOCK_LLM_MODE")` to audit E2E tests
- Only run audit tests in integration environment with real LLM

**Pros**:
- ✅ No code changes to `analyze_incident`
- ✅ Simpler implementation

**Cons**:
- ❌ Violates BR-AUDIT-005 (audit should be MANDATORY for ALL LLM interactions)
- ❌ E2E tests cannot verify audit event persistence
- ❌ Reduces test coverage
- ❌ Does not match production behavior (audit is always enabled)

### **Option C: Separate E2E Test Mode**

**Rationale**: Use real LLM in E2E, keep mock LLM for integration tests.

**Implementation**:
- E2E: `MOCK_LLM_MODE=false` with real LLM endpoint
- Integration: `MOCK_LLM_MODE=true` without audit tests

**Pros**:
- ✅ Tests production behavior (real LLM + audit)
- ✅ No code changes needed

**Cons**:
- ❌ Requires LLM API access in CI/CD
- ❌ LLM API costs for E2E tests
- ❌ Non-deterministic test results
- ❌ Slower test execution
- ❌ Violates BR-HAPI-212 (mock mode for integration testing)

---

## 🎯 **Recommendation**

**Choose Option A**: Add Audit to Mock Response Flow

**Justification**:
1. **Compliance**: Meets BR-AUDIT-005 requirement (all LLM interactions audited)
2. **Testing**: Enables E2E verification of audit event persistence
3. **Realism**: Mock responses mirror production audit behavior
4. **Cost**: No LLM API costs, deterministic results
5. **Simplicity**: ~20 lines of code vs complex E2E infrastructure changes

---

## 🔧 **Implementation Steps**

1. **Update `analyze_incident` function**:
   - Move `audit_store = get_audit_store()` before mock check
   - Add audit event generation to mock response path
   - Ensure all 3 event types are generated (llm_request, llm_response, workflow_validation_attempt)

2. **Update E2E tests** (already done):
   - ✅ Tests now call HTTP API `/api/v1/incident/analyze`
   - ✅ Request data includes all required fields (`signal_source`)
   - ✅ Tests wait for audit flush (6 seconds)
   - ✅ Tests query Data Storage for audit events

3. **Verify**:
   - Run HAPI E2E tests with mock mode enabled
   - Confirm audit events are persisted to Data Storage
   - Confirm 4 tests pass

---

## 📊 **Impact Analysis**

### **Files to Modify**
1. `holmesgpt-api/src/extensions/incident/llm_integration.py` - Add audit to mock path (~30 lines)
2. `holmesgpt-api/src/extensions/recovery/llm_integration.py` - Add audit to mock path (~30 lines)

### **Tests Affected**
- 4 HAPI E2E audit tests (currently failing)
- No integration/unit tests affected (they don't check audit events)

### **Risk Level**
- **LOW**: Changes are isolated to mock response path
- **No impact** on production (production uses real LLM, not mock mode)
- **Fallback**: If audit fails, existing error handling will catch it

---

## 📞 **Next Steps**

**For HAPI Team**:
1. Review Option A implementation approach
2. Implement audit event generation in mock response path
3. Run E2E tests to verify
4. Confirm all 4 audit tests pass

**For Infrastructure Team**:
- ✅ Infrastructure refactoring COMPLETE
- ✅ AIAnalysis refactoring COMPLETE
- ✅ DD-TEST-001 v1.3 documented
- ⏸️ HAPI audit issue identified, awaiting HAPI team fix

---

## 📚 **Related Documentation**

- BR-AUDIT-005: Audit Trail for LLM Interactions
- ADR-032: Mandatory Audit Requirements (v1.3)
- BR-HAPI-212: Mock LLM Mode for Integration Testing
- `docs/handoff/INFRASTRUCTURE_REFACTORING_COMPLETE_DEC_26_2025.md`

---

**Status**: 🔍 ROOT CAUSE IDENTIFIED
**Owner**: HAPI Team
**Priority**: P1 - Blocks E2E test completion

