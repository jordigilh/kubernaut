# HAPI Comprehensive Test Triage - December 26, 2025

**Date**: December 26, 2025
**Service**: HolmesGPT API (HAPI)
**Team**: HAPI Team
**Status**: 🟡 **FIXES IN PROGRESS**

---

## 📊 **Executive Summary**

**Findings**:
1. ✅ **E2E Tests**: 1 test failure fixed (mock response schema mismatch)
2. ❌ **HAPI Integration Tests**: Audit tests follow FORBIDDEN anti-pattern (HIGH priority)
3. ⚠️  **Metrics Tests**: Missing integration tests (MEDIUM priority)
4. ✅ **Go Services**: ALL CLEAN - Anti-pattern already fixed (Dec 2025)

**Actions Required**:
1. **Immediate**: Verify E2E test fix (mock response schema)
2. **High Priority**: Refactor HAPI audit integration tests to flow-based pattern
3. **Medium Priority**: Add metrics integration tests

**Cross-Service Context**:
- **Go Services**: All 6 services already refactored to flow-based pattern (Dec 2025)
- **Python Services**: Only HAPI has audit integration anti-pattern
- **See**: `GO_SERVICES_AUDIT_ANTI_PATTERN_TRIAGE_DEC_26_2025.md`

---

## 🧪 **Test Tier Status**

### **Unit Tests**: ✅ **PASSING**

```
572 passed, 8 xfailed in 45.67s
```

**Status**: ALL GOOD
- 8 xfailed tests are V1.1 features (PostExec endpoint) - intentionally deferred
- See: `docs/handoff/HAPI_XFAILED_TESTS_TRIAGE_DEC_26_2025.md`

---

### **Integration Tests**: ❌ **ANTI-PATTERN DETECTED**

```
49 passed in 65.23s
```

**Status**: TESTS PASS BUT DON'T VALIDATE BUSINESS LOGIC

#### Issue 1: Audit Integration Tests Follow Anti-Pattern

**File**: `holmesgpt-api/tests/integration/test_audit_integration.py`
**Lines**: 68-408
**Tests Affected**: 6 tests

**Problem**:
- ❌ Tests manually create audit events
- ❌ Tests directly call Data Storage API
- ❌ Tests verify Data Storage accepts events (wrong responsibility)
- ❌ Tests DON'T trigger HAPI business logic
- ❌ **0% coverage of HAPI audit integration**

**Required Action**: DELETE and rewrite as flow-based tests

**See**: `docs/handoff/HAPI_AUDIT_INTEGRATION_ANTI_PATTERN_TRIAGE_DEC_26_2025.md`

**Pattern Comparison**:

| What to Test | ❌ Current Pattern | ✅ Required Pattern |
|--------------|-------------------|---------------------|
| **Action** | `data_storage_client.create_audit_event(event)` | `requests.post("/api/v1/incident/analyze", data)` |
| **Verification** | Event accepted by DS | Audit event emitted by HAPI |
| **Validates** | DS API works | HAPI emits audits |
| **Coverage** | 0% of HAPI audit integration | 100% of HAPI audit integration |

#### Issue 2: Metrics Integration Tests Missing → ✅ **CREATED**

**Expected**: Integration tests that:
1. Make HTTP requests to HAPI endpoints
2. Verify metrics are recorded (e.g., `http_requests_total`, `llm_request_duration_seconds`)
3. Query `/metrics` endpoint to verify metrics are exposed

**Status**: ✅ **COMPLETE** - 11 flow-based metrics tests created

**File**: `holmesgpt-api/tests/integration/test_hapi_metrics_integration.py`

**Tests Created**:
- 3 HTTP request metrics tests
- 2 LLM metrics tests
- 2 metrics aggregation tests
- 2 /metrics endpoint availability tests
- 1 business metrics test (informational)
- 1 content type test

**See**: `docs/handoff/HAPI_METRICS_INTEGRATION_TESTS_CREATED_DEC_26_2025.md`

**Priority**: MEDIUM (completed per user request)

---

### **E2E Tests**: 🟡 **FIX IN PROGRESS**

**Previous Run**:
```
1 failed, 6 passed, 1 skipped in 30.03s
```

**Failing Test**:
- `test_mock_llm_edge_cases_e2e.py::test_max_retries_exhausted_returns_validation_history`

**Root Cause**: Mock response and test assertions used incorrect field names for `ValidationAttempt` model

**Fixes Applied**:
1. ✅ Updated mock response in `src/mock_responses.py` (lines 476-495)
   - `attempt_number` → `attempt`
   - `validation_passed` → `is_valid`
   - `failure_reason` → `errors` (list)
   - Added `timestamp` field
2. ✅ Updated test assertions in `test_mock_llm_edge_cases_e2e.py` (lines 192-195)
3. ✅ Updated audit logging in `src/audit/buffered_store.py` (lines 356-360)
   - `response.event_id` → `response.status` (field doesn't exist per ADR-038)

**Expected After Fix**: All 7 functional tests should pass

**See**:
- `docs/handoff/HAPI_E2E_TEST_TRIAGE_DEC_26_2025.md`
- `docs/handoff/HAPI_ALL_TIERS_STATUS_DEC_26_2025.md`

---

## 🚨 **Priority Matrix**

| Issue | Priority | Impact | Merge Blocker |
|-------|----------|--------|---------------|
| **E2E mock response fix** | P0 | Tests don't pass | ✅ YES |
| **Audit integration anti-pattern** | P1 | False confidence, 0% coverage | ✅ YES |
| **Metrics integration missing** | P2 | Missing coverage | ⚠️  RECOMMENDED |

---

## 📋 **Detailed Findings**

### 1. E2E Test Failure: Mock Response Schema Mismatch

**Root Cause**: `ValidationAttempt` Pydantic model expects specific field names, but mock response used different names.

**ValidationAttempt Model** (`src/models/incident_models.py` lines 214-231):
```python
class ValidationAttempt(BaseModel):
    attempt: int           # ✅ Correct
    workflow_id: Optional[str]
    is_valid: bool        # ✅ Correct
    errors: List[str]     # ✅ Correct (list, not string)
    timestamp: str        # ✅ Required
```

**Mock Response Before** (`src/mock_responses.py`):
```python
validation_history = [
    {
        "attempt_number": 1,          # ❌ Wrong
        "validation_passed": False,   # ❌ Wrong
        "failure_reason": "..."       # ❌ Wrong (should be list)
        # Missing timestamp
    }
]
```

**Mock Response After**:
```python
validation_history = [
    {
        "attempt": 1,                 # ✅ Correct
        "is_valid": False,            # ✅ Correct
        "errors": ["..."],            # ✅ Correct (list)
        "timestamp": timestamp        # ✅ Added
    }
]
```

**Test Assertions Before**:
```python
assert "attempt_number" in attempt      # ❌ Wrong field name
assert "validation_passed" in attempt   # ❌ Wrong field name
```

**Test Assertions After**:
```python
assert "attempt" in attempt           # ✅ Correct
assert "is_valid" in attempt          # ✅ Correct
assert "errors" in attempt            # ✅ Added
assert "timestamp" in attempt         # ✅ Added
```

**Status**: ✅ FIXED, awaiting E2E rerun to verify

---

### 2. Audit Integration Anti-Pattern

**Detection**: All 6 audit integration tests follow the FORBIDDEN pattern documented in [TESTING_GUIDELINES.md](../development/business-requirements/TESTING_GUIDELINES.md#anti-pattern-direct-audit-infrastructure-testing).

**Anti-Pattern Signature**:
```python
# ❌ WRONG: These are the red flags
1. Import create_*_event helpers
2. Call event = create_llm_request_event(...)
3. Call data_storage_client.create_audit_event(event)
4. Assert response.status == "accepted"
5. NO HTTP request to HAPI business endpoints
```

**Tests to Delete**:
1. `TestLLMRequestAuditEvent::test_llm_request_event_stored_in_ds` (lines 78-127)
2. `TestLLMResponseAuditEvent::test_llm_response_event_stored_in_ds` (lines 139-179)
3. `TestLLMToolCallAuditEvent::test_llm_tool_call_event_stored_in_ds` (lines 191-231)
4. `TestWorkflowValidationAuditEvent::test_workflow_validation_event_stored_in_ds` (lines 244-288)
5. `TestWorkflowValidationAuditEvent::test_workflow_validation_final_attempt_with_human_review` (lines 290-325)
6. `TestAuditEventSchemaValidation::test_all_event_types_have_required_adr034_fields` (lines 338-408)

**Total Lines to Delete**: ~250 lines

**Replacement Pattern**:
```python
# ✅ CORRECT: Flow-based pattern
def test_incident_analysis_emits_llm_request_and_response_events(
    hapi_base_url, data_storage_url
):
    # 1. Trigger business operation
    response = requests.post(
        f"{hapi_base_url}/api/v1/incident/analyze",
        json=incident_request
    )
    assert response.status_code == 200

    # 2. Wait for processing
    time.sleep(2)  # ADR-038: buffered audit flush

    # 3. Verify audit events emitted as side effect
    events = query_audit_events_by_correlation_id(
        data_storage_url,
        correlation_id=remediation_id
    )

    # 4. Validate audit events
    llm_request_events = [e for e in events if e.event_type == "llm_request"]
    assert len(llm_request_events) == 1
    assert llm_request_events[0].correlation_id == remediation_id
```

**Action Items**:
1. Create new file: `test_hapi_audit_flow_integration.py`
2. Implement 6+ flow-based tests covering:
   - Incident analysis audit trail (llm_request, llm_response, llm_tool_call, workflow_validation)
   - Recovery analysis audit trail (llm_request, llm_response)
   - Error scenarios (failure outcomes)
3. Delete anti-pattern tests from `test_audit_integration.py`
4. Keep helper functions and fixtures for reuse

**Reference Implementations**:
- SignalProcessing: `test/integration/signalprocessing/audit_integration_test.go`
- Gateway: `test/integration/gateway/audit_integration_test.go`

**See**: `docs/handoff/HAPI_AUDIT_INTEGRATION_ANTI_PATTERN_TRIAGE_DEC_26_2025.md`

---

### 3. Metrics Integration Tests Missing

**Expected Tests**:

```python
# test/integration/test_hapi_metrics_integration.py

class TestHAPIMetricsIntegration:
    """Integration tests for HAPI metrics instrumentation."""

    def test_incident_analysis_records_http_request_metrics(
        self, hapi_base_url
    ):
        """Verify HTTP request metrics are recorded for incident analysis."""
        # Make HTTP request
        response = requests.post(f"{hapi_base_url}/api/v1/incident/analyze", ...)
        assert response.status_code == 200

        # Query /metrics endpoint
        metrics_response = requests.get(f"{hapi_base_url}/metrics")
        metrics_text = metrics_response.text

        # Verify metrics are present
        assert "http_requests_total" in metrics_text
        assert "http_request_duration_seconds" in metrics_text
        assert 'method="POST"' in metrics_text
        assert 'path="/api/v1/incident/analyze"' in metrics_text

    def test_llm_request_duration_recorded(self, hapi_base_url):
        """Verify LLM request duration metric is recorded."""
        # Make request
        requests.post(f"{hapi_base_url}/api/v1/incident/analyze", ...)

        # Verify metric
        metrics = requests.get(f"{hapi_base_url}/metrics").text
        assert "llm_request_duration_seconds" in metrics
```

**Why This is Better Than Current Audit Tests**:
1. ✅ Triggers business operations (HTTP requests)
2. ✅ Verifies HAPI actually records metrics
3. ✅ Validates `/metrics` endpoint exposes metrics
4. ✅ Tests HAPI behavior, not infrastructure

**Priority**: MEDIUM (E2E tests also cover metrics, but integration tests provide faster feedback)

---

## 🎯 **Recommended Action Plan**

### Phase 1: Unblock Merge (P0) ⏰ **IMMEDIATE**

1. ✅ **Verify E2E test fix**
   ```bash
   cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
   make test-e2e-holmesgpt-api
   ```
   **Expected**: All 7 functional tests pass

2. ⏳ **Monitor E2E run** (currently in progress)
   - Log: `/tmp/hapi_e2e_fixed_run.log`
   - PID: Check `ps aux | grep test-e2e-holmesgpt-api`

### Phase 2: Fix Integration Tests (P1) ⚠️ **HIGH PRIORITY**

1. **Create flow-based audit tests** (~2-3 hours)
   - New file: `holmesgpt-api/tests/integration/test_hapi_audit_flow_integration.py`
   - 6+ flow-based tests
   - See template in triage document

2. **Delete anti-pattern tests** (~15 minutes)
   - Remove lines 68-408 from `test_audit_integration.py`
   - Keep imports, fixtures, helpers (lines 1-67)

3. **Verify coverage** (~30 minutes)
   - Run: `make test-integration-holmesgpt`
   - Verify: Each audit event type has flow-based test
   - Verify: Tests catch missing audit integration

**Estimated Total**: 3-4 hours

### Phase 3: Add Metrics Tests (P2) ✅ **COMPLETE**

1. ✅ **Created metrics integration tests** (completed)
   - New file: `holmesgpt-api/tests/integration/test_hapi_metrics_integration.py`
   - 11 metrics tests created
   - Follows pattern from audit flow-based tests

**Status**: ✅ COMPLETE

---

## 📚 **Reference Documents**

### E2E Tests
- **Triage**: `docs/handoff/HAPI_E2E_TEST_TRIAGE_DEC_26_2025.md`
- **Status**: `docs/handoff/HAPI_ALL_TIERS_STATUS_DEC_26_2025.md`
- **Xfailed Tests**: `docs/handoff/HAPI_XFAILED_TESTS_TRIAGE_DEC_26_2025.md`
- **Confidence Assessment**: `docs/handoff/CONFIDENCE_ASSESSMENT_DELETE_XFAILED_TESTS_DEC_26_2025.md`

### Integration Tests
- **Audit Anti-Pattern**: `docs/handoff/HAPI_AUDIT_INTEGRATION_ANTI_PATTERN_TRIAGE_DEC_26_2025.md`
- **System-Wide Triage**: `docs/handoff/AUDIT_INFRASTRUCTURE_TESTING_ANTI_PATTERN_TRIAGE_DEC_26_2025.md`

### Guidelines
- **Testing Guidelines**: `docs/development/business-requirements/TESTING_GUIDELINES.md#anti-pattern-direct-audit-infrastructure-testing`
- **APDC Methodology**: `docs/development/business-requirements/TESTING_GUIDELINES.md`

---

## 🎊 **Summary**

### ✅ **What's Working**
- Unit tests: 100% passing (572 tests)
- E2E tests: Fix applied, awaiting verification
- Integration tests: All passing (but need refactoring)

### ❌ **What Needs Work**
- ~~**Audit integration tests**: Follow anti-pattern, provide false confidence~~ ✅ **FIXED**
- ~~**Metrics integration tests**: Missing entirely~~ ✅ **CREATED**

### 🎯 **Next Steps**
1. ⏳ **Verify E2E fix** (in progress - recovery schema fix)
2. ✅ **Refactor audit integration tests** (COMPLETE - 7 flow-based tests)
3. ✅ **Add metrics integration tests** (COMPLETE - 11 flow-based tests)
4. ⏳ **Run integration tests** (verify audit + metrics tests pass)

---

**Document Version**: 1.0
**Last Updated**: December 26, 2025
**Status**: Triage complete, fixes in progress
**Next Review**: After E2E tests complete

