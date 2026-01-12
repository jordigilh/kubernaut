# Recovery Test Failure Triage - January 12, 2026

**Date**: January 12, 2026 16:30 EST  
**Status**: 🔍 **ROOT CAUSE IDENTIFIED**  
**Failed Tests**: 4 recovery endpoint tests in `test_workflow_selection_e2e.py`

---

## 🎯 **Root Cause: Test File Not Migrated**

**Critical Finding**: `test_workflow_selection_e2e.py` is **still using the OLD embedded Mock LLM**!

### **Evidence**

```python
# holmesgpt-api/tests/e2e/test_workflow_selection_e2e.py:62-67
@pytest.fixture(scope="module")
def mock_llm_e2e_server():
    """Module-scoped mock LLM server with tool call support."""
    from tests.mock_llm_server import MockLLMServer  # ← OLD EMBEDDED MOCK!
    
    with MockLLMServer(force_text_response=False) as server:
        yield server
```

### **Why This Failed**

1. **Outdated Fixture**: Test file defines local `mock_llm_e2e_server` fixture using embedded mock
2. **TestClient Architecture**: Uses FastAPI TestClient (in-process) instead of real HTTP calls
3. **Wrong Mock LLM**: Connects to embedded mock, not standalone Mock LLM service
4. **Missing ENV Vars**: TestClient environment doesn't have proper `LLM_PROVIDER` config

---

## 📊 **Test Architecture Comparison**

| Test File | Architecture | Status |
|-----------|-------------|--------|
| **test_recovery_endpoint_e2e.py** | OpenAPI Client → Real HAPI → Real Mock LLM | ✅ **PASSING** (37 tests) |
| **test_audit_pipeline_e2e.py** | OpenAPI Client → Real HAPI → Real Mock LLM | ✅ **PASSING** |
| **test_workflow_selection_e2e.py** | TestClient → In-Process HAPI → Embedded Mock | ❌ **FAILING** (4 tests) |

---

## 🔍 **Failing Tests**

All 4 failures are in `test_workflow_selection_e2e.py`:

1. `TestRecoveryAnalysisE2E::test_recovery_analysis_calls_workflow_search_tool`
2. `TestRecoveryAnalysisE2E::test_recovery_analysis_returns_valid_response`
3. `TestRecoveryAnalysisE2E::test_recovery_previous_execution_context_in_prompt`
4. `TestRecoveryEndpointE2EEndToEndFlow::test_complete_incident_to_recovery_flow_e2e`

**Error Pattern**:
```
litellm.exceptions.BadRequestError: LLM Provider NOT provided. 
Pass in the LLM provider you are trying to call. You passed model=mock-model
```

---

## 🛠️ **Solution: Migrate Test File**

### **Option A: Use OpenAPI Client (Recommended for E2E)**

**Goal**: Make `test_workflow_selection_e2e.py` use real HAPI service like other E2E tests

**Changes Needed**:
1. Remove local `mock_llm_e2e_server` and `e2e_client` fixtures
2. Use `hapi_client_config` from conftest (connects to real HAPI)
3. Use OpenAPI client for API calls (not TestClient)
4. Update test methods to use OpenAPI client instead of TestClient

**Benefits**:
- True E2E testing (real HTTP, real services)
- Consistent with other E2E tests
- Uses standalone Mock LLM service
- Tests actual deployment configuration

**Drawbacks**:
- Requires HAPI service running in Kind
- Slightly slower than TestClient

---

### **Option B: Keep TestClient, Update Fixtures**

**Goal**: Keep TestClient but use standalone Mock LLM service

**Changes Needed**:
1. Update `mock_llm_e2e_server` to return standalone Mock LLM URL
2. Update `e2e_client` to use correct environment variables
3. Ensure `LLM_PROVIDER=openai` is set before FastAPI app import

**Benefits**:
- Faster test execution (in-process)
- No external services needed

**Drawbacks**:
- Not true E2E (doesn't test deployment)
- More like integration tests than E2E tests
- Different architecture from other E2E tests

---

### **Option C: Move to Integration Tests**

**Goal**: Reclassify these as integration tests, not E2E

**Changes Needed**:
1. Move file to `tests/integration/`
2. Keep TestClient architecture
3. Update fixtures to use real external Mock LLM container
4. Mark as integration tests, not E2E

**Benefits**:
- Correct classification (TestClient = integration, not E2E)
- Consistent with testing strategy
- Can run independently

**Drawbacks**:
- File relocation required
- Test naming changes

---

## ✅ **Recommended Solution: Option A (OpenAPI Client)**

**Rationale**:
1. **Consistency**: Aligns with other E2E tests
2. **True E2E**: Tests actual deployment architecture
3. **Mock LLM Migration**: Uses standalone Mock LLM service
4. **Maintainability**: One testing pattern for all E2E tests

---

## 📝 **Implementation Plan: Option A**

### **Step 1: Update Fixtures**

**Remove** local fixtures from `test_workflow_selection_e2e.py`:
```python
# DELETE THESE:
@pytest.fixture(scope="module")
def mock_llm_e2e_server():
    ...

@pytest.fixture(scope="module")
def e2e_client(mock_llm_e2e_server):
    ...
```

**Add** OpenAPI client fixtures (similar to `test_recovery_endpoint_e2e.py`):
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

### **Step 2: Update Test Methods**

**Original (TestClient)**:
```python
def test_recovery_analysis_calls_workflow_search_tool(
    self,
    e2e_client,
    sample_recovery_request,
    mock_llm_e2e_server
):
    response = e2e_client.post(
        "/api/v1/recovery/analyze",
        json=sample_recovery_request
    )
```

**Updated (OpenAPI Client)**:
```python
def test_recovery_analysis_calls_workflow_search_tool(
    self,
    recovery_api,
    sample_recovery_request,
    mock_llm_service_e2e  # Use conftest fixture
):
    from generated.holmesgpt.models.recovery_analysis_request import RecoveryAnalysisRequest
    
    request = RecoveryAnalysisRequest(**sample_recovery_request)
    response = recovery_api.analyze_recovery(recovery_analysis_request=request)
```

---

### **Step 3: Update Assertions**

**Original (TestClient)**:
```python
assert response.status_code == 200
data = response.json()
```

**Updated (OpenAPI Client)**:
```python
assert response is not None
# response is already a Pydantic model, no .json() needed
assert response.selected_workflow is not None
```

---

### **Step 4: Remove Embedded Mock Dependency**

**After migration, DELETE** the old embedded mock:
```bash
# These files can be removed after test migration:
rm holmesgpt-api/tests/mock_llm_server.py
```

---

## 🧪 **Validation Steps**

1. **Update fixtures** in `test_workflow_selection_e2e.py`
2. **Update test methods** to use OpenAPI clients
3. **Run E2E tests**: `make test-e2e-holmesgpt-api`
4. **Verify all 41 tests pass** (37 existing + 4 migrated)
5. **Delete embedded mock** after validation

---

## 📈 **Expected Outcome**

| Metric | Current | After Fix |
|--------|---------|-----------|
| **Passing E2E Tests** | 37/41 (90.2%) | 41/41 (100%) ✅ |
| **Mock LLM Architecture** | Mixed (embedded + standalone) | 100% standalone ✅ |
| **Test Consistency** | Inconsistent (TestClient + OpenAPI) | 100% OpenAPI ✅ |
| **Mock LLM Migration** | 90% complete | **100% COMPLETE** ✅ |

---

## 🎯 **Priority Level**

**HIGH** - Blocks Mock LLM migration completion

**Estimated Time**: 1-2 hours

**Risk**: LOW (well-understood problem, clear solution)

---

## 📁 **Related Documents**

- **E2E Test Results**: `docs/plans/E2E_TEST_RESULTS_JAN12_2026.md`
- **Mock LLM Migration Plan**: `docs/plans/MOCK_LLM_MIGRATION_PLAN.md`
- **Infrastructure Fix**: `docs/plans/E2E_INFRASTRUCTURE_FAILURE_JAN12_2026.md`

---

**Last Updated**: 2026-01-12 16:30 EST  
**Status**: ⏳ **AWAITING FIX** - Root cause identified, solution designed
