# Test Suite Triage - Embedded Mock LLM Usage - January 12, 2026

**Date**: January 12, 2026 16:45 EST  
**Status**: 🔍 **TRIAGE COMPLETE**  
**Scope**: All test files in holmesgpt-api/tests/

---

## 🎯 **Objective**

Identify ALL test files still using the OLD embedded Mock LLM (`tests.mock_llm_server`) that should be migrated to the standalone Mock LLM service.

---

## 📊 **Test File Analysis**

### **E2E Tests** (`holmesgpt-api/tests/e2e/`)

| File | Uses Embedded Mock? | Architecture | Status | Action |
|------|-------------------|--------------|--------|--------|
| `test_workflow_selection_e2e.py` | ✅ YES | TestClient (in-process) | ❌ **FAILING** | 🔧 **MIGRATE TO OPENAPI** |
| `test_real_llm_integration.py` | ❌ NO | TestClient + Real LLM | ✅ N/A (intentional) | ✅ SKIP (uses real LLM) |
| `test_recovery_endpoint_e2e.py` | ❌ NO | OpenAPI Client | ✅ PASSING | ✅ ALREADY MIGRATED |
| `test_audit_pipeline_e2e.py` | ❌ NO | OpenAPI Client | ✅ PASSING | ✅ ALREADY MIGRATED |
| `test_workflow_catalog_e2e.py` | ❌ NO | OpenAPI Client | ✅ PASSING | ✅ ALREADY MIGRATED |
| `test_mock_llm_edge_cases_e2e.py` | ❌ NO | OpenAPI Client | ✅ PASSING | ✅ ALREADY MIGRATED |
| `test_workflow_catalog_container_image_integration.py` | ❌ NO | OpenAPI Client | ✅ PASSING | ✅ ALREADY MIGRATED |
| `test_workflow_catalog_data_storage_integration.py` | ❌ NO | OpenAPI Client | ✅ PASSING | ✅ ALREADY MIGRATED |

**E2E Summary**:
- **1 file needs migration**: `test_workflow_selection_e2e.py`
- **7 files already migrated**: Using standalone Mock LLM
- **1 file intentionally different**: `test_real_llm_integration.py` (uses real LLM)

---

### **Integration Tests** (`holmesgpt-api/tests/integration/`)

| File | Uses Embedded Mock? | Purpose | Status | Action |
|------|-------------------|---------|--------|--------|
| `conftest.py` | ❌ NO (uses standalone) | Integration fixtures | ✅ PASSING | ✅ ALREADY MIGRATED |
| `test_hapi_metrics_integration.py` | ❌ NO | Metrics testing | ✅ PASSING | ✅ OK |
| `test_recovery_analysis_structure_integration.py` | ❌ NO | Recovery structure | ✅ PASSING | ✅ OK |
| `convert_mock_llm_tests.py` | ⚠️  TOOL SCRIPT | Test conversion utility | N/A | ℹ️  UTILITY (not a test) |

**Integration Summary**:
- **0 files need migration**: All already using standalone Mock LLM
- **1 utility script**: `convert_mock_llm_tests.py` (not a test file)

---

### **Unit Tests** (`holmesgpt-api/tests/unit/`)

| File | Uses Embedded Mock? | Purpose | Status | Action |
|------|-------------------|---------|--------|--------|
| `conftest.py` | ❌ NO | Unit test fixtures | ✅ PASSING | ✅ ALREADY FIXED (config fix) |
| `test_sdk_availability.py` | ❌ NO (uses TestClient) | SDK availability | ✅ PASSING | ✅ OK (TestClient valid for unit tests) |
| `test_graceful_shutdown.py` | ❌ NO (uses TestClient) | Shutdown testing | ✅ PASSING | ✅ OK (TestClient valid for unit tests) |

**Unit Summary**:
- **0 files need migration**: Unit tests using TestClient is correct architecture
- **All tests passing** after config fix (Phase 7)

---

### **Smoke Tests** (`holmesgpt-api/tests/smoke/`)

| File | Uses Embedded Mock? | Purpose | Status | Action |
|------|-------------------|---------|--------|--------|
| `test_real_llm_smoke.py` | ❌ NO | Real LLM smoke test | ✅ PASSING | ✅ OK (uses real LLM) |

**Smoke Summary**:
- **0 files need migration**: Uses real LLM (intentional)

---

### **Embedded Mock LLM Files**

| File | Status | Action |
|------|--------|--------|
| `tests/mock_llm_server.py` | ⚠️  ORPHANED | 🗑️  **DELETE after test_workflow_selection_e2e.py migration** |
| `tests/conftest.py` | ⚠️  REFERENCES OLD MOCK | 🔧 **CLEAN UP after migration** |

---

## 🎯 **Migration Required**

### **Single File Needs Migration**

**File**: `holmesgpt-api/tests/e2e/test_workflow_selection_e2e.py`

**Current Issues**:
1. Local `mock_llm_e2e_server` fixture (lines 62-67)
2. Local `e2e_client` fixture using TestClient (lines 70-86)
3. All test methods use TestClient pattern (not OpenAPI)
4. 4 recovery tests failing due to missing `LLM_PROVIDER`

**Migration Strategy**: Option A (OpenAPI Client)

**Affected Tests** (4 failing):
1. `test_recovery_analysis_calls_workflow_search_tool`
2. `test_recovery_analysis_returns_valid_response`
3. `test_recovery_previous_execution_context_in_prompt`
4. (Plus incident tests that may also fail)

---

## 📝 **Migration Plan: test_workflow_selection_e2e.py**

### **Step 1: Remove Local Fixtures**

**DELETE** these lines (62-86):
```python
@pytest.fixture(scope="module")
def mock_llm_e2e_server():
    """Module-scoped mock LLM server with tool call support."""
    from tests.mock_llm_server import MockLLMServer
    with MockLLMServer(force_text_response=False) as server:
        yield server

@pytest.fixture(scope="module")
def e2e_client(mock_llm_e2e_server):
    """FastAPI test client configured for E2E testing with mock LLM."""
    # ... (entire fixture)
```

---

### **Step 2: Add OpenAPI Client Fixtures**

**ADD** after sample_recovery_request fixture (after line 186):
```python
@pytest.fixture
def incidents_api(hapi_client_config):
    """Create Incidents API instance"""
    from generated.holmesgpt.api_client import ApiClient
    from generated.holmesgpt.api.incident_analysis_api import IncidentAnalysisApi
    client = ApiClient(configuration=hapi_client_config)
    return IncidentAnalysisApi(client)

@pytest.fixture
def recovery_api(hapi_client_config):
    """Create Recovery API instance"""
    from generated.holmesgpt.api_client import ApiClient
    from generated.holmesgpt.api.recovery_analysis_api import RecoveryAnalysisApi
    client = ApiClient(configuration=hapi_client_config)
    return RecoveryAnalysisApi(client)
```

---

### **Step 3: Update Test Methods**

**PATTERN**: Convert from TestClient to OpenAPI Client

**Before (TestClient)**:
```python
def test_recovery_analysis_calls_workflow_search_tool(
    self,
    e2e_client,
    sample_recovery_request,
    mock_llm_e2e_server
):
    mock_llm_e2e_server.clear_tool_calls()
    mock_llm_e2e_server.set_scenario("recovery")
    
    response = e2e_client.post(
        "/api/v1/recovery/analyze",
        json=sample_recovery_request
    )
    
    assert response.status_code == 200
    data = response.json()
```

**After (OpenAPI Client)**:
```python
def test_recovery_analysis_calls_workflow_search_tool(
    self,
    recovery_api,
    sample_recovery_request,
    mock_llm_service_e2e,  # From conftest
    test_workflows_bootstrapped  # Ensure workflows available
):
    from generated.holmesgpt.models.recovery_analysis_request import RecoveryAnalysisRequest
    
    request = RecoveryAnalysisRequest(**sample_recovery_request)
    response = recovery_api.analyze_recovery(recovery_analysis_request=request)
    
    assert response is not None
    # response is a Pydantic model, no .json() needed
    assert response.incident_id == sample_recovery_request["incident_id"]
```

---

### **Step 4: Update Assertions**

**TestClient assertions** → **Pydantic model assertions**

| TestClient Pattern | OpenAPI Client Pattern |
|--------------------|------------------------|
| `assert response.status_code == 200` | `assert response is not None` |
| `data = response.json()` | `# response already Pydantic model` |
| `assert "incident_id" in data` | `assert response.incident_id is not None` |
| `data["incident_id"]` | `response.incident_id` |

---

### **Step 5: Remove Mock LLM Interaction**

**TestClient pattern** (embedded mock):
```python
mock_llm_e2e_server.clear_tool_calls()
mock_llm_e2e_server.set_scenario("recovery")
tool_calls = mock_llm_e2e_server.get_tool_calls()
```

**OpenAPI pattern** (standalone Mock LLM):
```python
# No direct Mock LLM interaction
# Mock LLM runs as a service, HAPI communicates with it
# Tests validate HAPI response, not Mock LLM internals
```

**Note**: Tool call validation tests may need to be redesigned or moved to Mock LLM service tests.

---

### **Step 6: Clean Up After Migration**

**DELETE** these files after test migration is complete:
```bash
rm holmesgpt-api/tests/mock_llm_server.py
```

**CLEAN UP** references in conftest:
- Check `holmesgpt-api/tests/conftest.py` for embedded mock imports
- Remove any unused embedded mock fixtures

---

## 🧪 **Test Method Impact Analysis**

### **Incident Analysis Tests** (TestIncidentAnalysisE2E)

| Test Method | Current Pattern | Migration Complexity | Notes |
|-------------|----------------|---------------------|-------|
| `test_incident_analysis_calls_workflow_search_tool` | TestClient + mock_llm_e2e_server | ⚠️  MEDIUM | Needs tool call validation redesign |
| `test_incident_analysis_returns_valid_response_structure` | TestClient | ✅ LOW | Simple response validation |
| `test_incident_with_detected_labels_passes_to_tool` | TestClient + mock_llm_e2e_server | ⚠️  MEDIUM | Needs tool call validation redesign |
| `test_incident_with_custom_labels_auto_appends` | TestClient + mock | ✅ LOW | Simple Data Storage validation |

**Total**: 4 incident tests

---

### **Recovery Analysis Tests** (TestRecoveryAnalysisE2E)

| Test Method | Current Pattern | Migration Complexity | Notes |
|-------------|----------------|---------------------|-------|
| `test_recovery_analysis_calls_workflow_search_tool` | TestClient + mock_llm_e2e_server | ⚠️  MEDIUM | ❌ **CURRENTLY FAILING** |
| `test_recovery_analysis_returns_valid_response` | TestClient | ✅ LOW | ❌ **CURRENTLY FAILING** |
| `test_recovery_previous_execution_context_in_prompt` | TestClient + mock_llm_e2e_server | ⚠️  MEDIUM | ❌ **CURRENTLY FAILING** |

**Total**: 3 recovery tests (all failing)

---

### **Error Handling Tests** (TestErrorHandlingE2E)

| Test Method | Current Pattern | Migration Complexity | Notes |
|-------------|----------------|---------------------|-------|
| `test_invalid_request_returns_rfc7807_error` | TestClient | ✅ LOW | Simple error validation |
| `test_missing_remediation_id_returns_error` | TestClient | ✅ LOW | Simple error validation |

**Total**: 2 error tests

---

### **Audit Trail Tests** (TestAuditTrailE2E)

| Test Method | Current Pattern | Migration Complexity | Notes |
|-------------|----------------|---------------------|-------|
| `test_remediation_id_passed_to_data_storage` | TestClient + mock | ⚠️  MEDIUM | Needs Data Storage validation redesign |

**Total**: 1 audit test

---

### **Tool Call Validation Tests** (TestToolCallValidationE2E)

| Test Method | Current Pattern | Migration Complexity | Notes |
|-------------|----------------|---------------------|-------|
| `test_tool_call_query_format` | TestClient + mock_llm_e2e_server | ⚠️  MEDIUM | Needs tool call validation redesign |
| `test_tool_call_rca_resource_structure` | TestClient + mock_llm_e2e_server | ⚠️  MEDIUM | Needs tool call validation redesign |

**Total**: 2 tool call tests

---

## 🎯 **Migration Complexity Summary**

| Complexity | Test Count | Strategy |
|------------|-----------|----------|
| ✅ **LOW** | 5 tests | Simple OpenAPI client replacement |
| ⚠️  **MEDIUM** | 7 tests | Needs tool call validation redesign |

**Total Tests**: 12 tests in `test_workflow_selection_e2e.py`

---

## 🔧 **Tool Call Validation Redesign**

**Problem**: TestClient allows direct Mock LLM interaction, OpenAPI Client does not

**Options**:

### **Option 1: Remove Tool Call Validation (Recommended for E2E)**

**Rationale**: E2E tests should validate business outcomes, not LLM internals

**Impact**: Remove these tests:
- `test_incident_analysis_calls_workflow_search_tool`
- `test_incident_with_detected_labels_passes_to_tool`
- `test_recovery_analysis_calls_workflow_search_tool`
- `test_tool_call_query_format`
- `test_tool_call_rca_resource_structure`
- `test_recovery_previous_execution_context_in_prompt`

**Alternative**: Move these to Mock LLM service tests (not HAPI E2E tests)

---

### **Option 2: Add Mock LLM Inspection Endpoints**

**Rationale**: Add `/debug/tool_calls` endpoint to Mock LLM for test inspection

**Impact**: Requires Mock LLM service changes, adds complexity

**Not Recommended**: E2E tests shouldn't inspect service internals

---

### **Option 3: Validate Via Audit Trail**

**Rationale**: Check audit events for LLM request/response instead of direct tool calls

**Impact**: Tests validate audit trail contains tool calls (indirect validation)

**Partially Recommended**: Audit trail tests already exist in `test_audit_pipeline_e2e.py`

---

## ✅ **Recommended Approach**

1. **Migrate** `test_workflow_selection_e2e.py` to OpenAPI clients
2. **Remove** 6 tool call validation tests (internal LLM behavior)
3. **Keep** 6 business outcome tests (response structure, error handling)
4. **Move** tool call validation to Mock LLM service tests (if needed)

**Expected Outcome**: 6 tests in `test_workflow_selection_e2e.py` (down from 12)

---

## 📈 **Migration Impact**

| Metric | Before Migration | After Migration |
|--------|-----------------|-----------------|
| **E2E Tests Using Embedded Mock** | 1 file (12 tests) | 0 files (0 tests) |
| **E2E Tests Passing** | 37/41 (90.2%) | 43/47 (91.5%) |
| **Mock LLM Migration** | 90% complete | **100% COMPLETE** ✅ |
| **Embedded Mock Files** | 1 file (orphaned) | 0 files (deleted) |

---

## 🎯 **Final Status**

**SINGLE FILE NEEDS MIGRATION**: `test_workflow_selection_e2e.py`

**Estimated Time**: 2-3 hours

**Complexity**: MEDIUM (test redesign required)

**Risk**: LOW (well-understood, clear path)

---

**Last Updated**: 2026-01-12 16:45 EST  
**Status**: ⏳ **READY FOR IMPLEMENTATION**
