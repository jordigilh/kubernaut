# DataStorage & HAPI E2E Tests - OpenAPI Client Migration Triage

**Date**: January 7, 2026  
**Priority**: 🔶 **MEDIUM** - Technical Debt (V1.0 Blocker per DD-API-001)  
**Scope**: E2E test compliance with OpenAPI client mandate  
**Status**: 📋 **TRIAGE COMPLETE** - Ready for implementation

---

## 📋 **Executive Summary**

**Problem**: DataStorage and HAPI E2E tests are using **raw HTTP calls** instead of **generated OpenAPI clients**, violating DD-API-001 mandate.

**Inconsistency Detected**: All other services (RemediationOrchestrator, WorkflowExecution, AIAnalysis, SignalProcessing) have already migrated their E2E tests to use OpenAPI clients, but **DataStorage's own E2E tests** and **HAPI E2E tests** were never migrated.

**Business Impact**: 
- ❌ No type safety - tests can break silently when API changes
- ❌ No contract validation - missing parameters not caught
- ❌ Manual JSON parsing - brittle and error-prone
- ❌ Inconsistent patterns - confusing for developers

**Recommendation**: Migrate both services to use OpenAPI clients for E2E tests.

---

## 🚨 **DD-API-001: OpenAPI Client Mandate**

### **Rule**
**ALL** services MUST use the generated OpenAPI client for API communication. Direct HTTP is **FORBIDDEN**.

**Authority**: `docs/architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md`

### **Why This Rule Exists**

| Aspect | Raw HTTP (`http.Get`) | OpenAPI Client | Impact |
|---|---|---|---|
| **Type Safety** | ❌ No compile-time validation | ✅ Type-safe structs | Runtime errors vs compile errors |
| **API Contract** | ❌ Manual URL construction | ✅ Auto-generated from spec | Breaking changes caught early |
| **Maintainability** | ❌ Fragile string concat | ✅ Method calls | Refactoring nightmares |
| **Documentation** | ❌ Undocumented expectations | ✅ Self-documenting code | Knowledge loss |
| **Auth Integration** | ❌ Manual header injection | ✅ Transport-based auth | Auth pattern consistency |

### **Compliance Status Across Services**

| Service | Integration Tests | E2E Tests | Status |
|---|---|---|---|
| **RemediationOrchestrator** | ✅ OpenAPI client | ✅ OpenAPI client | ✅ **COMPLIANT** (Fixed Dec 26, 2025) |
| **WorkflowExecution** | ✅ OpenAPI client | ✅ OpenAPI client | ✅ **COMPLIANT** (Fixed Dec 20, 2025) |
| **AIAnalysis** | ✅ OpenAPI client | ✅ OpenAPI client | ✅ **COMPLIANT** (Fixed Jan 2, 2026) |
| **SignalProcessing** | ✅ OpenAPI client | ✅ OpenAPI client | ✅ **COMPLIANT** (Always used) |
| **Gateway** | ✅ OpenAPI client | ✅ OpenAPI client | ✅ **COMPLIANT** |
| **Notification** | ✅ OpenAPI client | ✅ OpenAPI client | ✅ **COMPLIANT** |
| **AuthWebhook** | ✅ OpenAPI client | N/A | ✅ **COMPLIANT** |
| **DataStorage** | ✅ OpenAPI client | ❌ **Raw HTTP** | ❌ **NON-COMPLIANT** |
| **HAPI** | ❌ Raw HTTP | ❌ **Raw HTTP** | ❌ **NON-COMPLIANT** |

**Pattern**: 6/8 services are fully compliant. DataStorage and HAPI are the only holdouts.

---

## 🔍 **Violation Analysis**

### **1. DataStorage E2E Tests** (Go)

**Location**: `test/e2e/datastorage/*.go`

**Current Pattern (❌ FORBIDDEN)**:
```go
// test/e2e/datastorage/01_happy_path_test.go:280-285
func postAudit(client *http.Client, audit *models.NotificationAudit) *http.Response {
    payload, err := json.Marshal(audit)
    req, err := http.NewRequest("POST", datastorageURL+"/api/v1/audit/notifications",
        bytes.NewBuffer(payload))
    req.Header.Set("Content-Type", "application/json")
    resp, err := client.Do(req)
    // ... manual error handling
}
```

**Problems**:
- ❌ Manual URL construction (`datastorageURL+"/api/v1/audit/notifications"`)
- ❌ Manual JSON marshaling/unmarshaling
- ❌ No type safety (uses `models.NotificationAudit` but no OpenAPI validation)
- ❌ Manual header setting (no auth transport integration)
- ❌ Brittle error handling

**Correct Pattern (✅ REQUIRED)**:
```go
// Use OpenAPI client with auth transport
import dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"

// Create client with mock transport for E2E (no oauth-proxy in test)
mockTransport := testutil.NewMockUserTransport("datastorage-e2e@test")
dsClient, err := dsgen.NewClientWithResponses(
    datastorageURL,
    dsgen.WithHTTPClient(&http.Client{Transport: mockTransport}),
)

// Type-safe API call
resp, err := dsClient.CreateAuditEventWithResponse(ctx, dsgen.CreateAuditEventJSONRequestBody{
    EventType:     "remediation.started",
    EventCategory: "remediation",
    // ... type-safe fields from OpenAPI spec
})

// Type-safe response handling
if resp.JSON201 != nil {
    eventID := resp.JSON201.EventId
    // ... work with typed response
}
```

**Files Affected** (12 E2E test files):
1. `test/e2e/datastorage/01_happy_path_test.go`
2. `test/e2e/datastorage/02_dlq_fallback_test.go`
3. `test/e2e/datastorage/03_query_api_timeline_test.go`
4. `test/e2e/datastorage/04_workflow_search_test.go`
5. `test/e2e/datastorage/06_workflow_search_audit_test.go`
6. `test/e2e/datastorage/07_workflow_version_management_test.go`
7. `test/e2e/datastorage/08_workflow_search_edge_cases_test.go`
8. `test/e2e/datastorage/09_event_type_jsonb_comprehensive_test.go`
9. `test/e2e/datastorage/10_malformed_event_rejection_test.go`
10. `test/e2e/datastorage/11_connection_pool_exhaustion_test.go`
11. `test/e2e/datastorage/helpers.go`
12. `test/e2e/datastorage/datastorage_e2e_suite_test.go`

**Estimated Violations**: ~25-30 raw HTTP calls across 12 files

---

### **2. HAPI E2E Tests** (Python)

**Location**: `holmesgpt-api/tests/e2e/*.py`

**Current Pattern (❌ FORBIDDEN)**:
```python
# holmesgpt-api/tests/e2e/test_workflow_selection_e2e.py:71-78
from fastapi.testclient import TestClient

@pytest.fixture(scope="module")
def e2e_client(mock_llm_e2e_server):
    """FastAPI test client configured for E2E testing."""
    from src.main import app
    return TestClient(app)

# Tests use TestClient which is essentially raw HTTP
response = e2e_client.post("/api/v1/incident/analyze", json=request_data)
```

**Problems**:
- ❌ Uses `TestClient` (in-memory HTTP, not OpenAPI client)
- ❌ No type safety from OpenAPI spec
- ❌ Manual JSON dict construction (no Pydantic models from spec)
- ❌ No contract validation
- ❌ Different pattern than Go services

**Correct Pattern (✅ REQUIRED)**:
```python
# Use generated OpenAPI client
from holmesgpt_api_client import ApiClient, Configuration
from holmesgpt_api_client.api.incident_analysis_api import IncidentAnalysisApi
from holmesgpt_api_client.models.incident_request import IncidentRequest

# Configure client for E2E environment
config = Configuration(host="http://localhost:18120")
# For oauth-proxy protected environments, inject SA token:
# config.access_token = open('/var/run/secrets/kubernetes.io/serviceaccount/token').read()

api_client = ApiClient(configuration=config)
incident_api = IncidentAnalysisApi(api_client)

# Type-safe API call using Pydantic models
request = IncidentRequest(
    incident_id="test-123",
    remediation_id="rem-456",
    signal_type="PodCrashLoopBackOff",
    # ... type-safe fields from OpenAPI spec
)

# Type-safe response
response = incident_api.incident_analyze_endpoint_api_v1_incident_analyze_post(request)
assert response.selected_workflow is not None
```

**Files Affected** (10 E2E test files):
1. `holmesgpt-api/tests/e2e/test_workflow_selection_e2e.py`
2. `holmesgpt-api/tests/e2e/test_recovery_endpoint_e2e.py`
3. `holmesgpt-api/tests/e2e/test_audit_pipeline_e2e.py`
4. `holmesgpt-api/tests/e2e/test_workflow_catalog_e2e.py`
5. `holmesgpt-api/tests/e2e/test_workflow_catalog_data_storage_integration.py`
6. `holmesgpt-api/tests/e2e/test_workflow_catalog_container_image_integration.py`
7. `holmesgpt-api/tests/e2e/test_real_llm_integration.py`
8. `holmesgpt-api/tests/e2e/test_mock_llm_edge_cases_e2e.py`
9. `holmesgpt-api/tests/e2e/conftest.py`

**Estimated Violations**: ~15-20 TestClient usages across 9 files

---

## 📊 **Impact Analysis**

### **DataStorage E2E Tests**

**Current Problems**:
1. **Type Safety**: No compile-time validation of request/response structures
2. **API Contract**: Manual URL construction can get out of sync with OpenAPI spec
3. **Auth Integration**: Manual header injection instead of using `testutil.MockUserTransport`
4. **Maintainability**: Each test reinvents HTTP calling logic
5. **Error Detection**: API changes won't be caught until runtime

**Benefits of Migration**:
1. ✅ **Compile-time safety**: Type errors caught during `go build`
2. ✅ **Auth consistency**: Uses same `testutil.MockUserTransport` as other services
3. ✅ **Contract validation**: OpenAPI spec changes force test updates
4. ✅ **Code reduction**: ~30% less boilerplate code
5. ✅ **Pattern consistency**: Matches 6 other services

### **HAPI E2E Tests**

**Current Problems**:
1. **Pattern Inconsistency**: Only service using `TestClient` for E2E tests
2. **No Type Safety**: Dict-based requests instead of Pydantic models
3. **No Contract Validation**: OpenAPI spec not validated at test time
4. **Auth Testing**: Can't test oauth-proxy integration (TestClient is in-memory)

**Benefits of Migration**:
1. ✅ **Type safety**: Pydantic models from OpenAPI spec
2. ✅ **Real HTTP**: Tests actual HTTP stack (not in-memory)
3. ✅ **Auth testing**: Can test with mock headers or real tokens
4. ✅ **Pattern consistency**: Matches DataStorage Python client pattern
5. ✅ **Contract validation**: OpenAPI client ensures request/response compliance

---

## 🎯 **Implementation Plan**

### **Phase 1: DataStorage E2E Tests** (Go)

**Effort**: 4-6 hours  
**Priority**: HIGH (service owner should fix own tests first)

#### **Step 1: Update Suite Setup** (30 min)
```go
// test/e2e/datastorage/datastorage_e2e_suite_test.go

var (
    dsClient *dsgen.ClientWithResponses  // ✅ ADD
    httpClient *http.Client              // ❌ REMOVE (replaced by dsClient)
)

BeforeAll(func() {
    // Create OpenAPI client with mock transport
    mockTransport := testutil.NewMockUserTransport("datastorage-e2e@test")
    dsClient, err = dsgen.NewClientWithResponses(
        dataStorageURL,
        dsgen.WithHTTPClient(&http.Client{Transport: mockTransport}),
    )
    Expect(err).ToNot(HaveOccurred())
})
```

#### **Step 2: Migrate Helper Functions** (1 hour)
```go
// test/e2e/datastorage/helpers.go

// ❌ REMOVE: postAudit(client *http.Client, audit *models.NotificationAudit)
// ✅ ADD:
func createAuditEvent(ctx context.Context, event dsgen.CreateAuditEventJSONRequestBody) *dsgen.AuditEvent {
    resp, err := dsClient.CreateAuditEventWithResponse(ctx, event)
    Expect(err).ToNot(HaveOccurred())
    Expect(resp.JSON201).ToNot(BeNil())
    return resp.JSON201
}

func queryAuditEvents(ctx context.Context, correlationID string, eventCategory string) []dsgen.AuditEvent {
    limit := 100
    resp, err := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
        CorrelationId: &correlationID,
        EventCategory: &eventCategory,
        Limit:         &limit,
    })
    Expect(err).ToNot(HaveOccurred())
    Expect(resp.JSON200).ToNot(BeNil())
    return resp.JSON200.Events
}
```

#### **Step 3: Migrate Individual Test Files** (2-3 hours)
- Convert each `http.Get`, `http.Post` call to OpenAPI client method
- Replace `map[string]interface{}` with typed structs
- Update assertions to use typed fields
- Remove manual JSON marshaling/unmarshaling

#### **Step 4: Verification** (30 min)
```bash
# Run E2E tests to verify migration
cd test/e2e/datastorage
ginkgo run . -v

# Verify no raw HTTP imports remain
grep -r "http.Get\|http.Post\|http.NewRequest" *.go | grep -v "// Example" | wc -l
# Should be 0
```

---

### **Phase 2: HAPI E2E Tests** (Python)

**Effort**: 3-4 hours  
**Priority**: MEDIUM (less critical, but good for consistency)

#### **Step 1: Update Fixtures** (30 min)
```python
# holmesgpt-api/tests/e2e/conftest.py

import pytest
from holmesgpt_api_client import ApiClient, Configuration
from holmesgpt_api_client.api.incident_analysis_api import IncidentAnalysisApi
from holmesgpt_api_client.api.recovery_analysis_api import RecoveryAnalysisApi

@pytest.fixture(scope="module")
def hapi_config():
    """OpenAPI client configuration for E2E tests."""
    config = Configuration(host="http://localhost:18120")
    # For oauth-proxy environments, inject token:
    # config.access_token = get_serviceaccount_token()
    return config

@pytest.fixture(scope="module")
def incident_api(hapi_config):
    """OpenAPI incident analysis API client."""
    api_client = ApiClient(configuration=hapi_config)
    return IncidentAnalysisApi(api_client)

@pytest.fixture(scope="module")
def recovery_api(hapi_config):
    """OpenAPI recovery analysis API client."""
    api_client = ApiClient(configuration=hapi_config)
    return RecoveryAnalysisApi(api_client)
```

#### **Step 2: Migrate Test Files** (2-3 hours)
```python
# Before (❌ FORBIDDEN):
def test_incident_analysis(e2e_client):
    response = e2e_client.post("/api/v1/incident/analyze", json={
        "incident_id": "test-123",
        "signal_type": "PodCrashLoopBackOff",
        # ... manual dict construction
    })
    assert response.status_code == 200
    data = response.json()
    assert data["selected_workflow"] is not None

# After (✅ REQUIRED):
def test_incident_analysis(incident_api):
    from holmesgpt_api_client.models.incident_request import IncidentRequest
    
    request = IncidentRequest(
        incident_id="test-123",
        remediation_id="rem-456",
        signal_type="PodCrashLoopBackOff",
        # ... type-safe Pydantic model
    )
    
    response = incident_api.incident_analyze_endpoint_api_v1_incident_analyze_post(request)
    assert response.selected_workflow is not None
```

#### **Step 3: Update Mock Configuration** (30 min)
- Ensure mock LLM server works with real HTTP client (not just TestClient)
- May need to adjust port configuration

#### **Step 4: Verification** (30 min)
```bash
# Run E2E tests to verify migration
cd holmesgpt-api
pytest tests/e2e/ -v

# Verify no TestClient usage remains (except for legacy tests)
grep -r "TestClient" tests/e2e/*.py | wc -l
# Should be minimal (only in conftest if needed for backwards compat)
```

---

## 📈 **Success Metrics**

### **DataStorage**
- ✅ Zero raw HTTP calls in E2E tests (`grep -r "http.Get\|http.Post" test/e2e/datastorage/*.go | wc -l == 0`)
- ✅ All E2E tests pass with OpenAPI client
- ✅ ~30% code reduction in test files
- ✅ Auth transport pattern consistent with other services

### **HAPI**
- ✅ Zero TestClient usage in E2E tests (except legacy/deprecated tests)
- ✅ All E2E tests use OpenAPI client with Pydantic models
- ✅ Real HTTP testing (not in-memory)
- ✅ Can test oauth-proxy integration (future)

---

## 🗓️ **Timeline**

| Phase | Task | Effort | Priority |
|---|---|---|---|
| **Phase 1.1** | DataStorage suite setup | 30 min | HIGH |
| **Phase 1.2** | DataStorage helpers | 1 hour | HIGH |
| **Phase 1.3** | DataStorage test files | 2-3 hours | HIGH |
| **Phase 1.4** | DataStorage verification | 30 min | HIGH |
| **Phase 2.1** | HAPI fixtures | 30 min | MEDIUM |
| **Phase 2.2** | HAPI test files | 2-3 hours | MEDIUM |
| **Phase 2.3** | HAPI mock config | 30 min | MEDIUM |
| **Phase 2.4** | HAPI verification | 30 min | MEDIUM |
| **Total** | | **7-10 hours** | |

**Recommendation**: 
- **Week 1**: DataStorage E2E migration (4-6 hours)
- **Week 2**: HAPI E2E migration (3-4 hours)

---

## 🔗 **Related Documentation**

- **DD-API-001**: OpenAPI Client Mandate (decision document)
- **DD-AUTH-005**: DataStorage Client Authentication Pattern
- **DD-AUTH-006**: HAPI OAuth-Proxy Integration
- **RemediationOrchestrator E2E Fix**: `docs/handoff/DD_API_001_RO_E2E_COMPLIANCE_DEC_26_2025.md`
- **WorkflowExecution E2E Fix**: `docs/handoff/WE_P1_RAW_HTTP_REFACTOR_PROGRESS_DEC_20_2025.md`
- **AIAnalysis E2E Fix**: `docs/handoff/AA_E2E_FINAL_RESOLUTION_JAN_02_2026.md`

---

## ✅ **Next Steps**

1. **Review this triage** with team
2. **Prioritize**: DataStorage first (service owner responsibility)
3. **Create tasks** in issue tracker
4. **Assign owners**: 
   - DataStorage E2E: DataStorage team
   - HAPI E2E: HAPI/AIAnalysis team
5. **Execute migrations** following implementation plan
6. **Update DD-API-001 compliance matrix** after completion

---

## 📝 **Approval Required**

- [ ] **Technical Lead**: Approve migration approach
- [ ] **DataStorage Team**: Accept Phase 1 ownership
- [ ] **HAPI Team**: Accept Phase 2 ownership
- [ ] **QA**: Approve testing plan

---

**Status**: 📋 **READY FOR IMPLEMENTATION**  
**Document Owner**: AI Assistant  
**Created**: January 7, 2026  
**Authority**: DD-API-001 (OpenAPI Client Mandate)

