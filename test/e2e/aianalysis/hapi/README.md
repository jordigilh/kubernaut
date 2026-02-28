# HAPI End-to-End (E2E) Tests

**Status**: ⏸️ Infrastructure Setup Required
**Pattern**: HTTP API testing with containerized HAPI service
**Infrastructure**: Kind cluster + containerized HAPI + Data Storage + PostgreSQL + Redis

---

## 🎯 **Purpose**

These tests validate HAPI's complete HTTP API surface (FastAPI endpoints) in a production-like environment. Unlike integration tests that call Python business functions directly, E2E tests treat HAPI as a black box and test via HTTP requests.

**Defense-in-Depth Layer**: Tier 3 (E2E) - Black-box testing of complete system

---

## 📋 **Test Files (Moved from Integration)**

### 1. `test_custom_labels_e2e.py` (5 tests)

**Business Requirement**: DD-HAPI-001 (Custom Labels Support)
**Tests**: E2E-HAPI-001-01 through E2E-HAPI-001-05

| Test | Business Outcome |
|------|------------------|
| test_incident_request_with_custom_labels_in_enrichment_results | Custom labels flow through incident analysis pipeline |
| test_incident_request_without_custom_labels | System handles requests without custom labels |
| test_incident_request_with_empty_custom_labels | System handles empty custom labels |

**Pattern**:
```python
# E2E: HTTP call to containerized HAPI
client = HolmesgptApiClient(base_url="http://hapi-service:8080")
response = client.incident_analyze(request_data)
assert response.status_code == 200
```

### 2. `test_mock_llm_mode_e2e.py` (13 tests)

**Business Requirement**: BR-AI-001 (LLM Integration)
**Tests**: E2E-HAPI-002-01 through E2E-HAPI-002-13

| Test | Business Outcome |
|------|------------------|
| test_incident_endpoint_returns_200_in_mock_mode | HAPI responds successfully in mock LLM mode |
| test_incident_response_has_aianalysis_required_fields | Response includes all required AIAnalysis fields |
| test_incident_response_workflow_has_required_fields | Workflow suggestions are complete |
| test_incident_response_is_deterministic | Mock mode produces consistent responses |
| test_incident_validation_still_enforced_in_mock_mode | Request validation remains active |
| test_incident_mock_response_indicates_mock_mode | Response indicates mock mode active |
| test_incident_different_signal_types_produce_different_workflows | Signal types affect workflow selection |
---

## 🏗️ **Infrastructure Requirements**

### Required Services (in Kind)

1. **HAPI Service** (containerized)
   - Docker image: `kubernaut-hapi:latest`
   - Port: 8080
   - Environment: `MOCK_LLM=true`, `DATA_STORAGE_URL=http://data-storage:8080`

2. **Data Storage Service**
   - Docker image: `kubernaut-data-storage:latest`
   - Port: 8080

3. **PostgreSQL**
   - Port: 5432 (internal)
   - Database: `kubernaut`

4. **Redis**
   - Port: 6379 (internal)

### Infrastructure Setup (TODO - Separate Session)

```bash
# 1. Create HAPI Dockerfile
# Location: holmesgpt-api/Dockerfile

# 2. Create K8s manifests
# Location: test/e2e/aianalysis/hapi/manifests/
#   - deployment.yaml
#   - service.yaml
#   - configmap.yaml

# 3. Add to Kind deployment automation
# Location: test/e2e/aianalysis/suite_test.go
#   - Deploy HAPI alongside AIAnalysis

# 4. Run E2E tests
make test-e2e-aianalysis
```

---

## 🔧 **Current Status**

**Why These Tests Are NOT Running**:
1. ❌ HAPI Dockerfile does not exist
2. ❌ K8s manifests for HAPI not created
3. ❌ Kind deployment automation not configured
4. ❌ No E2E test runner configured for Python tests

**Previous Location**: These tests were in `holmesgpt-api/tests/integration/` but were blocking because they expected HAPI service to be running via HTTP, which is not the integration test pattern (integration tests call business logic directly).

**Correct Classification**: These are E2E tests (black-box HTTP testing), not integration tests.

---

## 📊 **Test Execution Pattern**

### Integration Tests (Different Pattern)
```python
# Integration: Direct Python function call
from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool

tool = SearchWorkflowCatalogTool(data_storage_url="http://localhost:18094")
result = tool._search_workflows(...)  # Business logic
```

### E2E Tests (This Directory)
```python
# E2E: HTTP call to containerized HAPI
from holmesgpt_api_client import HolmesgptApiClient

client = HolmesgptApiClient(base_url="http://hapi-service:8080")
response = client.incident_analyze(...)  # FastAPI endpoint
```

**Key Difference**: E2E tests treat HAPI as a black box (HTTP only), integration tests call internal Python functions.

---

## 🎯 **Next Steps (Separate Session)**

1. **Create HAPI Dockerfile**
   - Base image: Python 3.11
   - Install dependencies from `pyproject.toml`
   - Copy source code
   - Expose port 8080
   - CMD: `uvicorn src.main:app --host 0.0.0.0 --port 8080`

2. **Create K8s Manifests**
   - Deployment: HAPI pods with resource limits
   - Service: ClusterIP service on port 8080
   - ConfigMap: HAPI configuration (DATA_STORAGE_URL, MOCK_LLM, etc.)

3. **Integrate with AIAnalysis E2E Suite**
   - Add HAPI deployment to `suite_test.go`
   - Configure test runner for Python E2E tests
   - Add to `make test-e2e-aianalysis` target

4. **Verify E2E Tests Pass**
   - Run `make test-e2e-aianalysis`
   - Expected: 18 E2E tests passing

---

## 📚 **References**

- [HAPI Integration Test Plan](../../../../holmesgpt-api/tests/TEST_PLAN_HAPI_INTEGRATION_V1_0.md) - Complete test plan
- [AIAnalysis E2E Tests](../recovery_integration_test.go) - Go E2E example
- [Data Storage E2E Tests](../../datastorage/http_api_test.go) - Containerized service pattern
- [DD-TEST-002](../../../../docs/architecture/decisions/DD-TEST-002-integration-test-container-orchestration.md) - Integration pattern
- [Testing Strategy](../../../../.cursor/rules/03-testing-strategy.mdc) - Defense-in-depth

---

**Document Version**: 1.0.0
**Last Updated**: December 24, 2025
**Status**: ⏸️ Infrastructure setup pending (separate session)



