# HAPI OpenAPI Client Migration - Complete Triage

**Date**: 2025-12-13
**Team**: HAPI
**Status**: READY FOR IMPLEMENTATION
**Priority**: HIGH (Type safety, maintainability, API contract enforcement)

---

## Executive Summary

**Total Code Analyzed**: 67 integration tests + 3 business logic files
**Migration Required**: 5 tests + 1 fixture + 3 business logic files (9 components total)
**No Migration**: 62 tests (93%)
**Estimated Effort**: 3-4 hours
**Expected Result**: 66-67/67 tests passing + type-safe business logic

---

## 1. Business Logic Migration (CRITICAL)

### 1.1 Workflow Search Tool (PRIMARY)

**File**: `holmesgpt-api/src/toolsets/workflow_catalog.py`
**Lines**: 807-812
**Current Implementation**: Manual `requests.post()` call
**Migration Priority**: **CRITICAL** - Core business logic for workflow search

**Current Code**:
```python
response = requests.post(
    f"{self._data_storage_url}/api/v1/workflows/search",
    json=request_data,
    timeout=self._http_timeout
)
response.raise_for_status()
```

**Migration Target**:
```python
from src.clients.datastorage.api.workflow_search_api import WorkflowSearchApi
from src.clients.datastorage.models import (
    WorkflowSearchRequest,
    WorkflowSearchFilters
)
from src.clients.datastorage.api_client import ApiClient
from src.clients.datastorage.configuration import Configuration

# Initialize client (once, in __init__)
config = Configuration(host=self._data_storage_url)
api_client = ApiClient(configuration=config)
self._search_api = WorkflowSearchApi(api_client)

# In _search_workflows method
filters = WorkflowSearchFilters(
    signal_type=search_filters["signal_type"],
    severity=search_filters["severity"],
    component=search_filters["component"],
    environment=search_filters["environment"],
    priority=search_filters["priority"]
)

request = WorkflowSearchRequest(
    query=query,
    filters=filters,
    top_k=top_k,
    min_similarity=0.3,
    remediation_id=self._remediation_id
)

# Type-safe API call with automatic serialization
response = self._search_api.search_workflows(
    workflow_search_request=request,
    _request_timeout=self._http_timeout
)

# Response is already parsed WorkflowSearchResponse object
api_workflows = [workflow.to_dict() for workflow in response.workflows]
```

**Benefits**:
- ✅ **Type Safety**: Compile-time validation of request/response structure
- ✅ **Auto-serialization**: No manual JSON handling
- ✅ **API Contract**: Automatic enforcement of mandatory fields
- ✅ **Error Handling**: Structured exceptions instead of generic HTTP errors
- ✅ **Documentation**: IntelliSense/autocomplete for all fields
- ✅ **Maintainability**: Schema changes auto-detected during client regeneration

**Business Requirements**:
- BR-HAPI-250: Workflow Catalog Search Tool
- BR-STORAGE-013: Semantic Search for Remediation Workflows
- DD-WORKFLOW-002: MCP Workflow Catalog Architecture
- DD-LLM-001: MCP Workflow Search Parameter Taxonomy

---

### 1.2 Audit Store (SECONDARY)

**File**: `holmesgpt-api/src/audit/buffered_store.py`
**Lines**: 332-337
**Current Implementation**: Manual `requests.post()` call
**Migration Priority**: **MEDIUM** - Audit trail is non-critical path

**Current Code**:
```python
response = requests.post(
    f"{self._url}/api/v1/audit/events",
    json=event,
    timeout=self._config.http_timeout_seconds
)
response.raise_for_status()
```

**Migration Target**:
```python
from src.clients.datastorage.api.audit_api import AuditApi  # If exists
from src.clients.datastorage.models import AuditEvent
from src.clients.datastorage.api_client import ApiClient
from src.clients.datastorage.configuration import Configuration

# Initialize client (once, in __init__)
config = Configuration(host=self._url)
api_client = ApiClient(configuration=config)
self._audit_api = AuditApi(api_client)

# In _write_single_event_with_retry method
audit_event = AuditEvent(**event)  # Type-safe event creation
self._audit_api.create_audit_event(
    audit_event=audit_event,
    _request_timeout=self._config.http_timeout_seconds
)
```

**Note**: Check if `AuditApi` exists in generated client. If not, audit can remain with `requests` for V1.0.

**Business Requirements**:
- BR-AUDIT-005: Workflow Selection Audit Trail
- ADR-038: Asynchronous Buffered Audit Trace Ingestion
- DD-AUDIT-002: Audit Shared Library Design

---

### 1.3 Workflow Response Validator (TERTIARY)

**File**: `holmesgpt-api/src/validation/workflow_response_validator.py`
**Lines**: 77-84
**Current Implementation**: Uses `DataStorageClient` wrapper (DOES NOT EXIST)
**Migration Priority**: **HIGH** - Currently broken (imports non-existent wrapper)

**Current Code**:
```python
def __init__(self, data_storage_client):
    """
    Initialize validator with Data Storage client.

    Args:
        data_storage_client: Client for Data Storage service
    """
    self.ds_client = data_storage_client
```

**Problem**: `DataStorageClient` wrapper class does not exist. Code in `incident.py` tries to import:
```python
from src.clients.datastorage.client import DataStorageClient  # DOES NOT EXIST
```

**Migration Target**:
```python
from src.clients.datastorage.api.workflows_api import WorkflowsApi
from src.clients.datastorage.api_client import ApiClient
from src.clients.datastorage.configuration import Configuration

def __init__(self, data_storage_url: str):
    """
    Initialize validator with Data Storage API client.

    Args:
        data_storage_url: Data Storage Service URL
    """
    config = Configuration(host=data_storage_url)
    api_client = ApiClient(configuration=config)
    self.workflows_api = WorkflowsApi(api_client)
```

**Files Affected**:
- `holmesgpt-api/src/validation/workflow_response_validator.py` (validator itself)
- `holmesgpt-api/src/extensions/incident.py` (lines 1143-1169, creates validator)
- `holmesgpt-api/src/extensions/recovery.py` (if it uses validator)

**Business Requirements**:
- DD-HAPI-002 v1.2: LLM Self-Correction with Workflow Validation
- BR-AI-075: Workflow Selection Contract

---

## 2. Integration Test Migration (5 tests + 1 fixture)

### 2.1 Tests Requiring Migration

All these tests make **direct `requests.post()` calls** to Data Storage and should use OpenAPI client:

#### Test 1: `test_data_storage_returns_workflows_for_valid_query`
- **File**: `holmesgpt-api/tests/integration/test_data_storage_label_integration.py`
- **Lines**: 72-93
- **Reason**: Direct DS API call for workflow search validation

#### Test 2: `test_data_storage_accepts_snake_case_signal_type`
- **File**: `holmesgpt-api/tests/integration/test_data_storage_label_integration.py`
- **Lines**: 95-116
- **Reason**: Direct DS API call testing snake_case field format

#### Test 3: `test_data_storage_accepts_custom_labels_structure`
- **File**: `holmesgpt-api/tests/integration/test_data_storage_label_integration.py`
- **Lines**: 118-139
- **Reason**: Direct DS API call testing custom_labels feature

#### Test 4: `test_data_storage_accepts_detected_labels_with_wildcard`
- **File**: `holmesgpt-api/tests/integration/test_data_storage_label_integration.py`
- **Lines**: 141-162
- **Reason**: Direct DS API call testing detected_labels with wildcard

#### Test 5: `test_direct_api_search_returns_container_image`
- **File**: `holmesgpt-api/tests/integration/test_workflow_catalog_container_image_integration.py`
- **Lines**: 42-72
- **Reason**: Direct DS API call testing container_image propagation

---

### 2.2 Fixture Requiring Migration

#### Fixture: `ensure_test_workflows`
- **File**: `holmesgpt-api/tests/integration/conftest.py`
- **Lines**: 134-178
- **Reason**: Makes direct `requests.post()` call to verify test workflows exist
- **Impact**: Used by multiple tests, migration will benefit entire test suite

---

### 2.3 Tests NOT Requiring Migration (62 tests)

These tests correctly use **HAPI endpoints** (not direct DS calls):

#### Policy-Compliant Tests (30 tests)
- `test_custom_labels_integration_dd_hapi_001.py` (8 tests) - Uses HAPI `/api/v1/recovery/analyze`
- `test_mock_llm_mode_integration.py` (13 tests) - Uses HAPI `/api/v1/incident/analyze`
- `test_recovery_dd003_integration.py` (1 test) - Uses HAPI `/api/v1/recovery/analyze`
- `test_workflow_catalog_data_storage.py` (8 tests) - Uses HAPI `/api/v1/incident/analyze`

#### Error Handling Tests (2 tests)
- `test_connection_failure_raises_meaningful_error` - Tests error handling with fake URL
- `test_data_storage_unavailable_returns_error_i3_1` - Tests error propagation

#### Container Image Tests (4 tests in same file as Test 5)
- Other tests in `test_workflow_catalog_container_image_integration.py` use HAPI endpoints

---

## 3. Migration Strategy

### Phase 1: Business Logic (CRITICAL PATH)
**Estimated Time**: 2-3 hours

1. **Workflow Search Tool** (1.5 hours)
   - Add OpenAPI client initialization in `SearchWorkflowCatalogTool.__init__()`
   - Convert `_search_workflows()` method to use `WorkflowSearchApi`
   - Update error handling for OpenAPI exceptions
   - Test with existing integration tests

2. **Workflow Response Validator** (1 hour)
   - Fix broken `DataStorageClient` import in `incident.py` and `recovery.py`
   - Update validator to use `WorkflowsApi` directly
   - Update validator instantiation in `_create_data_storage_client()`
   - Test with existing validation tests

3. **Audit Store** (30 minutes - OPTIONAL for V1.0)
   - Check if `AuditApi` exists in generated client
   - If yes, migrate; if no, defer to V2.0
   - Audit is non-critical path (fire-and-forget)

### Phase 2: Integration Tests (VALIDATION)
**Estimated Time**: 1 hour

4. **Migrate 5 Direct DS Tests** (45 minutes)
   - Convert each test to use `WorkflowSearchApi`
   - Verify all 5 mandatory filter fields are present
   - Validate response parsing

5. **Migrate `ensure_test_workflows` Fixture** (15 minutes)
   - Convert to use `WorkflowSearchApi`
   - Benefits entire test suite

---

## 4. Expected Outcomes

### 4.1 Type Safety Benefits
- **Compile-time validation**: Invalid requests caught before runtime
- **Auto-completion**: IDE support for all API fields
- **Schema enforcement**: Mandatory fields automatically validated
- **Structured errors**: Typed exceptions instead of generic HTTP errors

### 4.2 Maintainability Benefits
- **API contract**: Changes in DS API automatically detected during client regeneration
- **Documentation**: Generated docs for all models and APIs
- **Consistency**: Single source of truth for DS API integration
- **Refactoring**: Safe refactoring with type checking

### 4.3 Test Results
- **Before Migration**: 66-67/67 tests passing (current state)
- **After Migration**: 66-67/67 tests passing (same coverage, better implementation)
- **Business Logic**: Type-safe, maintainable, API-contract-enforced
- **Future Changes**: DS API changes caught during client regeneration

---

## 5. Risk Assessment

### 5.1 Low Risk
- **Tests**: Only 5 tests + 1 fixture need changes (7% of test suite)
- **Rollback**: Easy - keep old `requests` code commented out during migration
- **Validation**: Existing tests validate correctness

### 5.2 Medium Risk
- **Business Logic**: Core workflow search tool requires careful migration
- **Mitigation**: Test thoroughly with existing integration tests before deploying

### 5.3 High Risk
- **Validator**: Currently broken (imports non-existent wrapper)
- **Mitigation**: Fix is required regardless of OpenAPI migration

---

## 6. Implementation Checklist

### Business Logic Migration
- [ ] Migrate `SearchWorkflowCatalogTool._search_workflows()` to use `WorkflowSearchApi`
- [ ] Fix `WorkflowResponseValidator` broken import
- [ ] Update `_create_data_storage_client()` in `incident.py` and `recovery.py`
- [ ] Check if `AuditApi` exists, migrate `BufferedAuditStore` if available

### Test Migration
- [ ] Migrate `test_data_storage_returns_workflows_for_valid_query`
- [ ] Migrate `test_data_storage_accepts_snake_case_signal_type`
- [ ] Migrate `test_data_storage_accepts_custom_labels_structure`
- [ ] Migrate `test_data_storage_accepts_detected_labels_with_wildcard`
- [ ] Migrate `test_direct_api_search_returns_container_image`
- [ ] Migrate `ensure_test_workflows` fixture

### Validation
- [ ] Run unit tests: `pytest holmesgpt-api/tests/unit/ -v`
- [ ] Run integration tests: `pytest holmesgpt-api/tests/integration/ -v -n 4 -m requires_data_storage`
- [ ] Verify no regressions in test results
- [ ] Verify type safety with `mypy` (if configured)

---

## 7. OpenAPI Client Reference

### Available APIs (Generated)
- `WorkflowSearchApi` - Workflow search (semantic search)
- `WorkflowsApi` - Workflow CRUD operations
- `IncidentsApi` - Incident read operations
- `SuccessRateAnalyticsApi` - Success rate analytics
- `HealthApi` - Service health checks

### Key Models (Generated)
- `WorkflowSearchRequest` - Search request with filters
- `WorkflowSearchFilters` - 5 mandatory fields (signal_type, severity, component, environment, priority)
- `WorkflowSearchResponse` - Search results with workflows
- `WorkflowSearchResult` - Individual workflow result
- `RemediationWorkflow` - Full workflow details

### Client Configuration
```python
from src.clients.datastorage.configuration import Configuration
from src.clients.datastorage.api_client import ApiClient

config = Configuration(host="http://data-storage:8080")
api_client = ApiClient(configuration=config)
```

---

## 8. Authoritative References

### Business Requirements
- BR-HAPI-250: Workflow Catalog Search Tool
- BR-STORAGE-013: Semantic Search for Remediation Workflows
- BR-AUDIT-005: Workflow Selection Audit Trail
- BR-AI-075: Workflow Selection Contract

### Design Decisions
- DD-WORKFLOW-002: MCP Workflow Catalog Architecture
- DD-LLM-001: MCP Workflow Search Parameter Taxonomy
- DD-STORAGE-011: OpenAPI Client Generation
- DD-HAPI-002 v1.2: LLM Self-Correction with Workflow Validation
- ADR-031: OpenAPI Specification Standard
- ADR-038: Asynchronous Buffered Audit Trace Ingestion

### Documentation
- `api/openapi/data-storage-v1.yaml` - Authoritative OpenAPI spec
- `holmesgpt-api/src/clients/README.md` - Client generation guide
- `docs/handoff/RESPONSE_HAPI_DS_SPEC_CONSOLIDATION_COMPLETE.md` - Spec consolidation

---

## 9. Next Steps

**READY TO PROCEED**: All analysis complete, implementation can begin immediately.

**Recommended Order**:
1. Fix `WorkflowResponseValidator` broken import (CRITICAL - currently broken)
2. Migrate `SearchWorkflowCatalogTool` (CRITICAL - core business logic)
3. Migrate 5 integration tests + 1 fixture (VALIDATION)
4. Check `AuditApi` availability, migrate if exists (OPTIONAL)

**Estimated Total Time**: 3-4 hours
**Expected Result**: Type-safe, maintainable, API-contract-enforced HAPI service

---

**Status**: ✅ TRIAGE COMPLETE - READY FOR IMPLEMENTATION

