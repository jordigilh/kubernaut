# 🚨 TRIAGE: Integration Test Policy Violation

**Date**: December 12, 2025
**Service**: HolmesGPT API (HAPI)
**Severity**: HIGH - Policy Violation
**Status**: IDENTIFIED - Requires Remediation

---

## 📊 **Executive Summary**

**VIOLATION DETECTED**: 30 out of 67 integration tests (45%) are using `TestClient` (in-memory FastAPI testing) instead of real HTTP calls to deployed services via `podman-compose`.

**Policy Source**: `docs/development/business-requirements/TESTING_GUIDELINES.md:614`

```markdown
| Test Tier | K8s Environment | Services | Infrastructure |
|-----------|-----------------|----------|----------------|
| **Integration** | envtest | Real (podman-compose) | `podman-compose.test.yml` |
```

**Requirement**: Integration tests MUST use real services (Data Storage, PostgreSQL, Redis) via `podman-compose`. ONLY exception: Mock LLM (cost constraint).

---

## 🔍 **Detailed Analysis**

### **Test Classification**

| Category | Count | Files | Compliance |
|----------|-------|-------|------------|
| **Real HTTP (COMPLIANT)** | 37 tests | 4 files | ✅ Correct |
| **TestClient (VIOLATING)** | 30 tests | 3 files | ❌ Violation |
| **TOTAL** | 67 tests | 7 files | 55% compliant |

---

### **✅ COMPLIANT Tests (37 tests - 4 files)**

These tests correctly use `integration_infrastructure` fixture which provides real service URLs:

#### 1. `test_data_storage_label_integration.py`
- **Pattern**: `integration_infrastructure["data_storage_url"]`
- **HTTP Client**: `requests` library to real Data Storage service
- **Tests**: Workflow search, confidence scoring, edge cases
- **Status**: ✅ COMPLIANT

#### 2. `test_workflow_catalog_data_storage.py`
- **Pattern**: `integration_infrastructure["data_storage_url"]`
- **HTTP Client**: `SearchWorkflowCatalogTool` → real HTTP
- **Tests**: Request format, response validation, UUID validation
- **Status**: ✅ COMPLIANT

#### 3. `test_workflow_catalog_container_image_integration.py`
- **Pattern**: `integration_infrastructure["data_storage_url"]`
- **HTTP Client**: `SearchWorkflowCatalogTool` → real HTTP
- **Tests**: Container image/digest validation, E2E flow
- **Status**: ✅ COMPLIANT

#### 4. `test_workflow_catalog_data_storage_integration.py`
- **Pattern**: `integration_infrastructure["data_storage_url"]`
- **HTTP Client**: `SearchWorkflowCatalogTool` → real HTTP
- **Tests**: Service availability, workflow bootstrap
- **Status**: ✅ COMPLIANT

---

### **❌ VIOLATING Tests (30 tests - 3 files)**

These tests use `TestClient` (in-memory FastAPI) instead of real HTTP:

#### 1. `test_custom_labels_integration_dd_hapi_001.py` (~8 tests)
- **Current Pattern**: `client` fixture (TestClient from `tests/conftest.py`)
- **Violation**: Uses `client.post("/api/v1/incident", ...)` - in-memory
- **Should Be**: `requests.post(f"{hapi_url}/api/v1/incident", ...)` - real HTTP
- **Tests**:
  - `test_incident_request_with_custom_labels_in_enrichment_results`
  - `test_incident_request_without_custom_labels`
  - `test_incident_request_with_empty_custom_labels`
  - `test_recovery_request_with_custom_labels_in_enrichment_results`
  - `test_recovery_request_without_custom_labels`
  - `test_workflow_search_includes_custom_labels_in_request`
  - `test_custom_labels_subdomain_structure_validated`
  - `test_custom_labels_boolean_and_keyvalue_formats`

#### 2. `test_mock_llm_mode_integration.py` (~13 tests)
- **Current Pattern**: `mock_mode_client` fixture (TestClient)
- **Violation**: Uses `TestClient(app)` - in-memory
- **Should Be**: Real HTTP to HAPI service with `MOCK_LLM_MODE=true` env var
- **Tests**:
  - `test_incident_endpoint_returns_200_in_mock_mode`
  - `test_incident_response_has_aianalysis_required_fields`
  - `test_incident_response_workflow_has_required_fields`
  - `test_incident_response_is_deterministic`
  - `test_incident_validation_still_enforced_in_mock_mode`
  - `test_incident_mock_response_indicates_mock_mode`
  - `test_incident_different_signal_types_produce_different_workflows`
  - `test_recovery_endpoint_returns_200_in_mock_mode`
  - (+ 5 more recovery tests)

#### 3. `test_recovery_dd003_integration.py` (~9 tests)
- **Current Pattern**: `client` fixture (TestClient)
- **Violation**: Uses `client.post("/api/v1/recovery", ...)` - in-memory
- **Should Be**: `requests.post(f"{hapi_url}/api/v1/recovery", ...)` - real HTTP
- **Tests**:
  - `test_recovery_endpoint_accepts_previous_execution`
  - `test_recovery_endpoint_returns_metadata_for_recovery_attempt`
  - `test_recovery_endpoint_returns_strategies`
  - `test_recovery_with_detected_labels_succeeds`
  - `test_recovery_without_detected_labels_succeeds`
  - `test_incident_with_detected_labels_succeeds`
  - `test_incident_without_detected_labels_succeeds`
  - `test_recovery_rejects_invalid_recovery_attempt_number`
  - `test_recovery_rejects_missing_remediation_id`

---

## 🎯 **Remediation Plan**

### **Option A: Convert TestClient Tests to Real HTTP (RECOMMENDED)**

**Effort**: Medium (2-3 hours)
**Impact**: Full policy compliance
**Risk**: Low (existing infrastructure already works)

#### Steps:

1. **Update `tests/conftest.py`** - Add `hapi_url` fixture:
   ```python
   @pytest.fixture(scope="session")
   def hapi_service_url():
       """HAPI service URL for integration tests"""
       return os.getenv("HOLMESGPT_API_URL", "http://localhost:18120")
   ```

2. **Convert test files** (3 files):
   - Replace `client.post(...)` with `requests.post(f"{hapi_url}/...", ...)`
   - Replace `client.get(...)` with `requests.get(f"{hapi_url}/...", ...)`
   - Update assertions to handle `requests.Response` instead of `TestClient.Response`

3. **Update `podman-compose.test.yml`** (if needed):
   - Ensure HAPI service is included with port 18120
   - Set `MOCK_LLM_MODE=true` environment variable

4. **Run tests**:
   ```bash
   cd tests/integration
   ./setup_workflow_catalog_integration.sh
   python3 -m pytest tests/integration/ -v
   ```

---

### **Option B: Move TestClient Tests to `tests/unit/`**

**Effort**: Low (1 hour)
**Impact**: Reclassify tests as unit tests
**Risk**: Medium (tests may not be true unit tests)

**Analysis**: These tests validate **integration behavior** (endpoint contracts, request/response validation, cross-component interaction). Moving to `tests/unit/` would be **incorrect classification**.

**Recommendation**: ❌ NOT RECOMMENDED - These ARE integration tests

---

### **Option C: Accept Violation (Document Exception)**

**Effort**: Minimal
**Impact**: Policy remains violated
**Risk**: HIGH - Sets precedent for policy violations

**Recommendation**: ❌ NOT RECOMMENDED - Violates TESTING_GUIDELINES.md

---

## 📋 **Implementation Checklist**

### Phase 1: Infrastructure Verification
- [ ] Verify `podman-compose.test.yml` includes HAPI service
- [ ] Verify HAPI service port mapping (18120:8080)
- [ ] Verify `MOCK_LLM_MODE=true` environment variable
- [ ] Test infrastructure startup: `./tests/integration/setup_workflow_catalog_integration.sh`

### Phase 2: Test Conversion
- [ ] Convert `test_custom_labels_integration_dd_hapi_001.py` (8 tests)
- [ ] Convert `test_mock_llm_mode_integration.py` (13 tests)
- [ ] Convert `test_recovery_dd003_integration.py` (9 tests)

### Phase 3: Validation
- [ ] Run all 67 integration tests with real infrastructure
- [ ] Verify 67/67 tests pass (or expected failures documented)
- [ ] Update test documentation
- [ ] Document policy compliance

---

## 🔧 **Technical Details**

### Current Architecture (VIOLATING)

```python
# tests/conftest.py
@pytest.fixture
def client(mock_llm_server):
    from src.main import app
    return TestClient(app)  # ❌ In-memory FastAPI

# tests/integration/test_custom_labels_integration_dd_hapi_001.py
def test_incident_request(self, client):
    response = client.post("/api/v1/incident", json=payload)  # ❌ In-memory
```

### Target Architecture (COMPLIANT)

```python
# tests/conftest.py
@pytest.fixture(scope="session")
def hapi_service_url():
    return os.getenv("HOLMESGPT_API_URL", "http://localhost:18120")

# tests/integration/test_custom_labels_integration_dd_hapi_001.py
def test_incident_request(self, hapi_service_url):
    response = requests.post(
        f"{hapi_service_url}/api/v1/incident",
        json=payload
    )  # ✅ Real HTTP
```

---

## 📊 **Success Metrics**

| Metric | Current | Target |
|--------|---------|--------|
| **Policy Compliance** | 55% (37/67) | 100% (67/67) |
| **TestClient Usage** | 30 tests | 0 tests |
| **Real HTTP Tests** | 37 tests | 67 tests |
| **Infrastructure Required** | Partial | Full (podman-compose) |

---

## 🚀 **Next Steps**

1. **Immediate**: Review and approve remediation plan (Option A recommended)
2. **Short-term**: Implement test conversions (2-3 hours)
3. **Validation**: Run full integration suite with real infrastructure
4. **Documentation**: Update handoff documents with compliance status

---

## 📚 **References**

- **Policy Source**: `docs/development/business-requirements/TESTING_GUIDELINES.md:614`
- **Infrastructure Guide**: `docs/development/business-requirements/TESTING_GUIDELINES.md:561-627`
- **Integration Setup**: `holmesgpt-api/tests/integration/setup_workflow_catalog_integration.sh`
- **Test Configuration**: `holmesgpt-api/tests/integration/conftest.py`

---

## ✅ **Approval Required**

**Question**: Which remediation option should we proceed with?

- **Option A**: Convert TestClient tests to real HTTP (RECOMMENDED)
- **Option B**: Move tests to unit/ (NOT RECOMMENDED)
- **Option C**: Document exception (NOT RECOMMENDED)

**Recommendation**: **Option A** - Full policy compliance with minimal risk

