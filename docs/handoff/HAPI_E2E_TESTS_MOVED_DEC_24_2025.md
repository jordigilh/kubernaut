# HAPI E2E Tests Moved from Integration Tier

**Date**: December 24, 2025
**Status**: ✅ COMPLETE - Tests Moved, Infrastructure Setup Pending
**Action**: Moved 18 HTTP-based tests from integration to E2E tier

---

## 🎯 **What Happened**

### Problem Identified

During HAPI integration test plan creation, we discovered that 18 tests in `holmesgpt-api/tests/integration/` were actually **E2E tests**, not integration tests:

**Integration Test Pattern** (CORRECT):
```python
# Call Python business logic directly with real external services
from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool

tool = SearchWorkflowCatalogTool(data_storage_url="http://localhost:18094")
result = tool._search_workflows(...)  # Direct function call
```

**E2E Test Pattern** (These 18 tests):
```python
# HTTP call to HAPI service (black-box testing)
from holmesgpt_api_client import HolmesgptApiClient

client = HolmesgptApiClient(base_url="http://localhost:18120")
response = client.incident_analyze(...)  # HTTP request to FastAPI
```

**Issue**: The 18 HTTP-based tests were miscategorized as integration tests, causing them to fail when HAPI service wasn't running.

---

## 📋 **Tests Moved**

### From: `holmesgpt-api/tests/integration/`
### To: `test/e2e/aianalysis/hapi/`

| Original File | New File | Tests | Status |
|---------------|----------|-------|--------|
| `test_custom_labels_integration_dd_hapi_001.py` | `test_custom_labels_e2e.py` | 5 | ✅ Moved |
| `test_mock_llm_mode_integration.py` | `test_mock_llm_mode_e2e.py` | 13 | ✅ Moved |
| `test_recovery_dd003_integration.py` | `test_recovery_dd003_e2e.py` | 0 | ✅ Moved |

**Total**: **18 tests** moved to E2E tier

---

## 📊 **Impact on Test Counts**

### Before Move

| Tier | Tests | Status |
|------|-------|--------|
| Unit | 569 | ✅ All passing |
| Integration | 35 + 18 | 🔴 18 tests blocked (need HAPI service) |
| E2E | 0 | ❌ None |

### After Move

| Tier | Tests | Status |
|------|-------|--------|
| Unit | 569 | ✅ All passing |
| Integration | **35** | ✅ All passing (direct function calls) |
| E2E | **18** | ⏸️ Infrastructure setup pending |

---

## 🏗️ **Why These Are E2E Tests**

### Integration Test Characteristics (CORRECT - 35 tests)

✅ **Test business logic directly** (Python function calls)
✅ **Use real external services** (Data Storage, PostgreSQL, Redis)
✅ **Bypass FastAPI layer** (no HTTP, no web server)
✅ **Fast execution** (<5 minutes for all 35 tests)
✅ **No containerization needed** (run locally with podman-compose)

**Example**:
```python
# tests/integration/test_workflow_catalog_data_storage_integration.py
from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool

def test_search_workflows_with_detected_labels(data_storage_url):
    """Integration: Business logic + real Data Storage"""
    tool = SearchWorkflowCatalogTool(data_storage_url=data_storage_url)
    result = tool._search_workflows(signal_type="OOMKilled", top_k=5)
    assert result['total_results'] > 0
```

### E2E Test Characteristics (MOVED - 18 tests)

✅ **Test via HTTP API** (FastAPI endpoints)
✅ **Treat HAPI as black box** (no internal knowledge)
✅ **Require containerized HAPI** (Dockerfile + Kind deployment)
✅ **Slower execution** (container startup overhead)
✅ **Production-like environment** (Kind cluster with all services)

**Example**:
```python
# test/e2e/aianalysis/hapi/test_custom_labels_e2e.py
from holmesgpt_api_client import HolmesgptApiClient

def test_incident_with_custom_labels_e2e(hapi_service_url):
    """E2E: HTTP API + complete system"""
    client = HolmesgptApiClient(base_url=hapi_service_url)
    response = client.incident_analyze(
        signal_type="OOMKilled",
        custom_labels={"team": "platform"}
    )
    assert response.status_code == 200
    assert "custom_labels" in response.json()
```

---

## 🎯 **Next Steps (Infrastructure Setup - Separate Session)**

### Phase 1: Create HAPI Dockerfile

**Location**: `holmesgpt-api/Dockerfile`

```dockerfile
FROM python:3.11-slim

WORKDIR /app

# Install dependencies
COPY pyproject.toml poetry.lock ./
RUN pip install poetry && poetry install --no-dev

# Copy source code
COPY src/ ./src/
COPY tests/ ./tests/

# Expose FastAPI port
EXPOSE 8080

# Run HAPI service
CMD ["poetry", "run", "uvicorn", "src.main:app", "--host", "0.0.0.0", "--port", "8080"]
```

### Phase 2: Create K8s Manifests

**Location**: `test/e2e/aianalysis/hapi/manifests/`

**Files to create**:
- `deployment.yaml` - HAPI deployment with resource limits
- `service.yaml` - ClusterIP service on port 8080
- `configmap.yaml` - HAPI configuration (DATA_STORAGE_URL, MOCK_LLM, etc.)

### Phase 3: Integrate with AIAnalysis E2E Suite

**Location**: `test/e2e/aianalysis/suite_test.go`

**Add HAPI deployment**:
```go
// Deploy HAPI service to Kind
func deployHAPI(ctx context.Context) error {
    return kubectl.Apply(ctx, "hapi/manifests/deployment.yaml")
}
```

### Phase 4: Configure Python E2E Test Runner

**Location**: `test/e2e/aianalysis/hapi/conftest.py`

**Add pytest fixtures**:
```python
@pytest.fixture(scope="session")
def hapi_service_url(kind_cluster):
    """Get HAPI service URL in Kind cluster"""
    return "http://hapi-service:8080"
```

### Phase 5: Run E2E Tests

```bash
# Build HAPI Docker image
cd holmesgpt-api
docker build -t kubernaut-hapi:latest .

# Deploy to Kind and run E2E tests
cd test/e2e/aianalysis
make test-e2e-aianalysis

# Expected: 18 E2E tests passing
```

---

## 📊 **Test Plan Alignment**

This move aligns with the HAPI Integration Test Plan V1.0:

**Integration Tests (50 total)**:
- ✅ 35 existing (direct function calls with real services)
- 🔴 15 NEW to be created (workflow search, prompt building, audit, parsing)

**E2E Tests (18 total)**:
- ✅ 18 moved from integration (HTTP API testing)
- ⏸️ Infrastructure setup pending

**Defense-in-Depth Complete**:
- **Unit (569)**: Business logic with mocks
- **Integration (50)**: Business logic with real services
- **E2E (18)**: HTTP API with containerized system

---

## ✅ **Success Criteria**

### Immediate (COMPLETE)
- [x] **18 tests moved** from integration to E2E tier
- [x] **35 integration tests** remain (all passing)
- [x] **Test pattern correct** (integration = direct calls, E2E = HTTP)
- [x] **Documentation created** (README in E2E directory)

### Future (Separate Session)
- [ ] **HAPI Dockerfile** created
- [ ] **K8s manifests** created
- [ ] **Kind deployment** configured
- [ ] **18 E2E tests passing** in Kind

---

## 📚 **References**

### Documentation
- [HAPI Integration Test Plan](./HAPI_INTEGRATION_TEST_PLAN_V1_0_DEC_24_2025.md) - Complete test plan
- [HAPI E2E README](../../test/e2e/aianalysis/hapi/README.md) - E2E test documentation
- [Test Plan Template](../services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md) - NT v1.3.0

### Test Patterns
- [DS Repository Tests](../../test/integration/datastorage/repository_test.go) - Integration pattern (direct calls)
- [DS HTTP API Tests](../../test/integration/datastorage/http_api_test.go) - E2E pattern (HTTP)
- [AIAnalysis E2E](../../test/integration/aianalysis/recovery_integration_test.go) - Go E2E example

### Standards
- [DD-TEST-002](../architecture/decisions/DD-TEST-002-integration-test-container-orchestration.md) - Integration pattern
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc) - Defense-in-depth approach

---

## 🎯 **Key Takeaways**

1. **Pattern Discovery**: HAPI had 18 tests using HTTP (E2E pattern) in the integration tier (wrong place)

2. **Correct Classification**:
   - **Integration Tests**: Direct Python function calls with real external services
   - **E2E Tests**: HTTP calls to containerized HAPI (black-box testing)

3. **Defense-in-Depth**: Moving these tests enables proper 3-tier testing:
   - **Unit (569)**: Isolated function testing with mocks
   - **Integration (50)**: Business logic testing with real services
   - **E2E (18)**: Complete system testing via HTTP

4. **Next Session**: Create HAPI Dockerfile + K8s manifests + Kind deployment

---

**Status**: ✅ Tests moved successfully
**Next Action**: Separate session for E2E infrastructure setup
**Timeline**: Infrastructure setup estimated at 2 days (Days 3-4 per test plan)



