# HolmesGPT API (HAPI) Integration Tests - Test Plan

**Version**: 1.0.0
**Last Updated**: December 24, 2025
**Status**: DRAFT - Awaiting Review

**Business Requirements**: BR-HAPI-250 (Workflow Catalog Search), BR-AUDIT-005 (Audit Trail), BR-AI-001 (LLM Integration)
**Design Decisions**: DD-HAPI-001 (Custom Labels), DD-LLM-001 (LLM Safety), ADR-038 (Async Audit)
**Authority**: DD-TEST-002 (Integration Test Pattern), DD-TEST-001 (Port Allocation)

**Cross-References**:
- [Test Plan Best Practices](../docs/development/testing/TEST_PLAN_BEST_PRACTICES.md) - When/why to use each section
- [NT Test Plan Example](../docs/services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md) - Reference implementation
- [Data Storage Integration Tests](../test/integration/datastorage/repository_test.go) - Python/Go integration pattern
- [Testing Strategy](./.cursor/rules/03-testing-strategy.mdc) - Defense-in-depth approach

---

## 📋 **Changelog**

### Version 1.0.0 (2025-12-24)
- **INITIAL**: First comprehensive HAPI integration test plan
- **SCOPE**: Integration tests calling business logic directly (bypass FastAPI)
- **DISCOVERY**: Current 18 tests are actually E2E tests (HTTP-based)
- **PLANNED**: 15+ new integration tests calling Python functions directly
- **PLANNED**: Move 18 existing HTTP tests to E2E tier
- **RATIONALE**: Follow Data Storage pattern (repository tests vs HTTP API tests)

---

## 🎯 **Testing Scope**

```
┌─────────────────────────────────────────────────────────────────────┐
│ HAPI Business Logic (Python Functions)                              │
│   ├── Workflow Catalog Search (SearchWorkflowCatalogTool)          │
│   │   ├── Label-based filtering (detected + custom labels)         │
│   │   ├── Signal type matching                                     │
│   │   ├── Severity prioritization                                  │
│   │   └── Top-K workflow selection                                 │
│   │                                                                  │
│   ├── LLM Prompt Building                                          │
│   │   ├── Context assembly (Kubernetes + alert data)               │
│   │   ├── Workflow injection (top-K selected workflows)            │
│   │   ├── Safety guidelines (DD-LLM-001)                           │
│   │   └── Token optimization                                       │
│   │                                                                  │
│   ├── Audit Event Creation (ADR-034)                               │
│   │   ├── LLM request/response audit                               │
│   │   ├── Tool call audit                                          │
│   │   ├── Workflow validation audit                                │
│   │   └── Data Storage integration                                 │
│   │                                                                  │
│   └── LLM Response Parsing                                         │
│       ├── JSON extraction and validation                           │
│       ├── Self-correction on parse failures                        │
│       ├── Structured output conversion (Pydantic)                  │
│       └── Error handling and retry                                 │
└─────────────────────────┬────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────────┐
│ External Services (REAL in Integration Tests)                       │
│   ├── Data Storage API (http://localhost:18094)                    │
│   │   ├── Workflow search API                                      │
│   │   ├── Audit event write API                                    │
│   │   └── Health check                                             │
│   │                                                                  │
│   ├── PostgreSQL (localhost:15439)                                 │
│   │   └── Used by Data Storage (indirect)                          │
│   │                                                                  │
│   └── Redis (localhost:16387)                                      │
│       └── Used by Data Storage (indirect)                          │
└─────────────────────────────────────────────────────────────────────┘
```

**Scope**: Test HAPI business logic functions directly with real external services (Data Storage, PostgreSQL, Redis).

**Testing Pattern** (per Data Storage example):
- **Integration Tests**: Call Python business functions directly (bypass FastAPI)
- **E2E Tests**: Deploy containerized HAPI, test via HTTP (FastAPI endpoints)

**Out of Scope** (Covered by Other Test Tiers):
- ❌ Unit tests: Already complete (569/569 passing) - test with mocks
- ❌ E2E tests: Will be created separately (containerized HAPI in Kind)
- ❌ LLM API calls: Use MOCK_LLM=true (cost constraint)

---

## 📊 **Defense-in-Depth Testing Summary**

**Strategy**: Overlapping BR coverage + cumulative code coverage approaching 100% (per [03-testing-strategy.mdc](../.cursor/rules/03-testing-strategy.mdc))

### BR Coverage (Overlapping) + Code Coverage (Cumulative)

| Tier | Tests | Infrastructure | BR Coverage | Code Coverage | Status |
|------|-------|----------------|-------------|---------------|--------|
| **Unit** | 569 | None (mocked external deps) | 70%+ of ALL BRs | ~27% | ✅ 569/569 passing |
| **Integration** | 35 + **15 NEW** | Real Data Storage, PostgreSQL, Redis | >50% of ALL BRs | **50%+ target** | 🟡 35/53 passing (18 are E2E) |
| **E2E** | 0 + **18 MOVED** | Containerized HAPI in Kind | <10% BR coverage | **50%+ target** | ⏸️ To be created (separate session) |

**Defense-in-Depth Principle**:
- **BR Coverage**: Overlapping (same BRs tested at multiple tiers)
- **Code Coverage**: Cumulative (~100% combined across all tiers)
- **Current Gap**: Unit tests at 27% code coverage (business logic not fully tested with real services)
- **Target**: Integration tests should add 50%+ code coverage by testing business functions with real external services

**Example - Workflow Search with Labels (BR-HAPI-250)**:
- **Unit (27%)**: Mock Data Storage responses, test SearchWorkflowCatalogTool logic - tests `src/toolsets/workflow_catalog.py`
- **Integration (NEW)**: Real Data Storage API, test actual workflow filtering - tests same code with real PostgreSQL/Redis
- **E2E (TO BE MOVED)**: Containerized HAPI, HTTP POST to `/api/v1/incident/analyze` - tests complete flow including FastAPI layer

**Example - Audit Event Creation (BR-AUDIT-005)**:
- **Unit (27%)**: Mock Data Storage client, test audit event structure - tests `src/audit/events.py`
- **Integration (35 passing)**: Real Data Storage API, verify events persisted - tests same code with real PostgreSQL
- **E2E (TO BE MOVED)**: Containerized HAPI, verify audit trail after incident analysis - tests complete audit pipeline

If a workflow search bug exists, it must slip through **ALL 3 defense layers** to reach production!

**Current Status**:
- ✅ Unit: 569/569 passing (100%)
- 🟡 Integration: 35/53 infrastructure tests passing (18 are miscategorized E2E tests)
- ⏸️ E2E: 0 tests (to be created in separate session)

**MVP Target**:
- ✅ Unit: 569/569 passing (no changes needed)
- ✅ Integration: 50/50 passing (15 NEW business logic tests + remove 18 E2E tests)
- ⏸️ E2E: 18 tests (move from integration + containerize HAPI)

**Total Tests**: 569 unit + 50 integration + 18 E2E = **637 tests** (defense-in-depth complete)

---

# 🧪 **TIER 1: UNIT TESTS** (70%+ BR Coverage | 27% Code Coverage) - ✅ COMPLETE

**Location**: `holmesgpt-api/tests/unit/`
**Infrastructure**: None (mocked external dependencies)
**Execution**: `make test-unit` or `python3 -m pytest tests/unit/ -v`
**Status**: ✅ 569/569 passing (100% pass rate)

### Current Unit Test Coverage

| Test Suite | Tests | Status | Coverage |
|---|---|---|---|
| Audit event structure | 13 | ✅ Passing | BR-AUDIT-005 |
| LLM sanitization (secret leakage) | 46 | ✅ Passing | BR-AI-002 |
| LLM self-correction | 20 | ✅ Passing | BR-AI-003 |
| Circuit breaker (LLM timeout) | 39 | ✅ Passing | BR-AI-004 |
| Recovery strategy validation | 9 | ✅ Passing | BR-AI-005 |
| RFC 7807 error responses | 13 | ✅ Passing | DD-004 |
| Workflow response validation | Skipped | ⏸️ Skipped | BR-HAPI-250 |
| **Total** | **569** | **✅ 100%** | **70%+ BRs** |

**Code Coverage**: ~27% (per recent test run)
- **Gap**: Business logic functions tested with mocks, not exercised with real external services
- **Integration Tier Goal**: Add 50%+ code coverage by testing with real Data Storage

**MVP Assessment**: ✅ **NO NEW UNIT TESTS NEEDED** - Existing coverage is comprehensive for isolated function testing

---

# 🔗 **TIER 2: INTEGRATION TESTS** (>50% BR Coverage | 50%+ Code Coverage Target) - 🔴 INCOMPLETE

**Location**: `holmesgpt-api/tests/integration/`
**Infrastructure**: Real Data Storage API (port 18094), Real PostgreSQL (port 15439), Real Redis (port 16387)
**Execution**: `MOCK_LLM=true python3 -m pytest tests/integration/ -k "not (custom_labels or mock_llm_mode)" -v`
**Status**: 🟡 35/53 tests passing (18 tests are miscategorized E2E tests)

**Current State**:
- ✅ 35 tests call business logic directly with real Data Storage (CORRECT integration tests)
- 🔴 18 tests use HTTP client + expect HAPI service running (THESE ARE E2E TESTS)

**Testing Pattern** (per Data Storage example):
- ✅ **CORRECT**: Import `SearchWorkflowCatalogTool`, call `_search_workflows()` directly
- ✅ **CORRECT**: Import `create_llm_request_event()`, call with real Data Storage client
- ❌ **INCORRECT**: Use OpenAPI client, make HTTP POST to HAPI service (this is E2E, not integration)

---

## 📋 **Current Integration Test Status**

### Existing Tests (35 passing)

| Test File | Tests | Pattern | Status |
|-----------|-------|---------|--------|
| `test_audit_integration.py` | 5 | ✅ Direct: calls `data_storage_client.create_audit_event()` | ✅ Passing |
| `test_data_storage_label_integration.py` | 16 | ✅ Direct: calls `SearchWorkflowCatalogTool()` | ✅ Passing |
| `test_workflow_catalog_container_image_integration.py` | 5 | ✅ Direct: tests Data Storage workflow repository | ✅ Passing |
| `test_workflow_catalog_data_storage_integration.py` | 9 | ✅ Direct: tests workflow search business logic | ✅ Passing |

**Subtotal**: 35 integration tests (CORRECT pattern)

### Miscategorized Tests (18 - should be E2E)

| Test File | Tests | Pattern | Issue |
|-----------|-------|---------|-------|
| `test_custom_labels_integration_dd_hapi_001.py` | 5 | ❌ HTTP: OpenAPI client to HAPI service | E2E, not integration |
| `test_mock_llm_mode_integration.py` | 13 | ❌ HTTP: OpenAPI client to HAPI service | E2E, not integration |
| `test_recovery_dd003_integration.py` | 0 | ❌ HTTP: OpenAPI client to HAPI service | E2E, not integration |

**Subtotal**: 18 tests (MOVE to E2E tier)

---

## 🎯 **NEW Integration Tests to Create**

**Goal**: Test business logic functions directly with real external services (no FastAPI layer)

### Category 1: Workflow Search Business Logic (5 tests)

**Business Requirement**: BR-HAPI-250 (Workflow Catalog Search)

| Test ID | Test Name | Business Outcome | Pattern |
|---------|-----------|------------------|---------|
| IT-HAPI-250-01 | Workflow search with detected labels filters results | Users get workflows matching their resource's detected labels | Import `SearchWorkflowCatalogTool`, call `_search_workflows()` with detected_labels, verify returned workflows match |
| IT-HAPI-250-02 | Workflow search with custom labels appends to query | Users can filter workflows by custom organizational labels | Import `SearchWorkflowCatalogTool`, call `_search_workflows()` with custom_labels, verify Data Storage receives combined filters |
| IT-HAPI-250-03 | Workflow search prioritizes by severity and signal type | Critical OOMKilled alerts get more relevant workflows than warnings | Import `SearchWorkflowCatalogTool`, search with different severities, verify ordering |
| IT-HAPI-250-04 | Workflow search respects top_k parameter | Users receive configured number of workflow suggestions | Import `SearchWorkflowCatalogTool`, call with top_k=3, verify exactly 3 workflows returned |
| IT-HAPI-250-05 | Workflow search handles empty results gracefully | System gracefully handles no matching workflows | Import `SearchWorkflowCatalogTool`, search with non-existent label, verify empty result with no errors |

**Implementation**:
```python
# NEW: tests/integration/test_workflow_search_business_logic.py
from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool
from src.models.workflow_models import DetectedLabels

def test_workflow_search_with_detected_labels_filters_results(data_storage_url):
    """IT-HAPI-250-01: Workflow search with detected labels filters results

    Business Outcome: Users get workflows matching their resource's detected labels
    BR: BR-HAPI-250 (Workflow Catalog Search)
    """
    # Arrange: Create tool with detected labels
    tool = SearchWorkflowCatalogTool(
        data_storage_url=data_storage_url,
        detected_labels=DetectedLabels(gitOpsManaged=True, gitOpsTool="argocd")
    )

    # Act: Call business logic directly (bypass FastAPI)
    result = tool._search_workflows(
        signal_type="OOMKilled",
        severity="critical",
        top_k=5
    )

    # Assert: Verify workflows match detected labels
    assert result['total_results'] > 0
    for workflow in result['workflows']:
        # Verify workflow has gitOps compatibility
        assert workflow['detected_labels']['gitOpsManaged'] == True
```

### Category 2: LLM Prompt Building Business Logic (3 tests)

**Business Requirement**: BR-AI-001 (LLM Context Optimization)

| Test ID | Test Name | Business Outcome | Pattern |
|---------|-----------|------------------|---------|
| IT-AI-001-01 | Prompt builder includes top-K workflows from Data Storage | LLM receives relevant workflow suggestions for analysis | Import `IncidentPromptBuilder`, call with workflow IDs, verify workflows fetched from real Data Storage |
| IT-AI-001-02 | Prompt builder assembles Kubernetes context correctly | LLM receives complete resource context for analysis | Import `IncidentPromptBuilder`, call with K8s resources, verify context structure |
| IT-AI-001-03 | Prompt builder optimizes token count under limit | Users don't exceed LLM token limits | Import `IncidentPromptBuilder`, call with large context, verify token count < max_tokens |

**Implementation**:
```python
# NEW: tests/integration/test_llm_prompt_business_logic.py
from src.extensions.incident.prompt_builder import IncidentPromptBuilder

def test_prompt_builder_includes_workflows_from_datastorage(data_storage_url):
    """IT-AI-001-01: Prompt builder includes top-K workflows from Data Storage

    Business Outcome: LLM receives relevant workflow suggestions for analysis
    BR: BR-AI-001 (LLM Context Optimization)
    """
    # Arrange: Get workflow IDs from search
    workflow_ids = ["wf-001", "wf-002", "wf-003"]

    # Act: Build prompt with real Data Storage lookup
    builder = IncidentPromptBuilder(data_storage_url=data_storage_url)
    prompt = builder.build_prompt(
        signal_type="OOMKilled",
        workflow_ids=workflow_ids
    )

    # Assert: Verify workflows fetched and included
    assert "wf-001" in prompt
    assert len(prompt.split("workflow")) >= 3  # At least 3 workflows mentioned
```

### Category 3: Audit Event Business Logic (4 tests)

**Business Requirement**: BR-AUDIT-005 (Audit Trail)

| Test ID | Test Name | Business Outcome | Pattern |
|---------|-----------|------------------|---------|
| IT-AUDIT-005-01 | Audit client stores LLM request events in Data Storage | All LLM requests are audited for compliance | Import `DataStorageAuditClient`, call `store_audit()`, verify in PostgreSQL |
| IT-AUDIT-005-02 | Audit client stores LLM response events in Data Storage | All LLM responses are audited for compliance | Import `create_llm_response_event()`, store, verify in PostgreSQL |
| IT-AUDIT-005-03 | Audit client buffers events for batch write (ADR-038) | System handles high-volume audit efficiently | Import audit client, store multiple events, verify batching behavior |
| IT-AUDIT-005-04 | Audit client handles Data Storage unavailability gracefully | System degrades gracefully when audit backend fails | Import audit client, stop Data Storage, verify error handling |

**Implementation**:
```python
# NEW: tests/integration/test_audit_business_logic.py
from src.audit.client import DataStorageAuditClient
from src.audit.events import create_llm_request_event

def test_audit_client_stores_llm_request_in_datastorage(data_storage_url):
    """IT-AUDIT-005-01: Audit client stores LLM request events in Data Storage

    Business Outcome: All LLM requests are audited for compliance
    BR: BR-AUDIT-005 (Audit Trail)
    ADR: ADR-038 (Async Buffered Audit Ingestion)
    """
    # Arrange: Create audit event
    event = create_llm_request_event(
        incident_id="inc-test-001",
        remediation_id="rem-test-001",
        model="claude-3-5-sonnet",
        prompt="Test prompt",
        toolsets_enabled=["kubernetes/core"]
    )

    # Act: Store via audit client (real Data Storage)
    client = DataStorageAuditClient(data_storage_url)
    response = client.store_audit(event)

    # Assert: Verify acceptance (async processing per ADR-038)
    assert response.status == "accepted"

    # Wait for async buffer flush
    time.sleep(2)

    # Verify event in PostgreSQL (via Data Storage query API)
    stored_event = client.query_audit(correlation_id="rem-test-001")
    assert stored_event['event_type'] == "llm_request"
    assert stored_event['event_data']['model'] == "claude-3-5-sonnet"
```

### Category 4: LLM Response Parsing Business Logic (3 tests)

**Business Requirement**: BR-AI-003 (LLM Self-Correction)

| Test ID | Test Name | Business Outcome | Pattern |
|---------|-----------|------------------|---------|
| IT-AI-003-01 | Response parser extracts JSON from LLM output | System reliably parses structured LLM responses | Import `LLMResponseParser`, call with mock LLM output, verify extraction |
| IT-AI-003-02 | Response parser triggers self-correction on parse failure | System recovers from malformed LLM responses | Import parser, simulate parse error, verify retry with correction prompt |
| IT-AI-003-03 | Response parser converts to Pydantic models | System ensures type safety of LLM outputs | Import parser, parse response, verify Pydantic validation |

**Implementation**:
```python
# NEW: tests/integration/test_llm_response_parsing.py
from src.extensions.incident.result_parser import parse_incident_result

def test_response_parser_extracts_json_from_llm_output(data_storage_url):
    """IT-AI-003-01: Response parser extracts JSON from LLM output

    Business Outcome: System reliably parses structured LLM responses
    BR: BR-AI-003 (LLM Self-Correction)
    """
    # Arrange: Mock LLM response with JSON embedded
    llm_output = """
    Here is my analysis:

    ```json
    {
        "severity": "critical",
        "root_cause": "OOMKilled",
        "workflows": ["wf-001", "wf-002"]
    }
    ```

    This suggests memory exhaustion.
    """

    # Act: Parse with real Data Storage for workflow lookup
    result = parse_incident_result(
        llm_output=llm_output,
        data_storage_url=data_storage_url
    )

    # Assert: Verify JSON extraction
    assert result.severity == "critical"
    assert result.root_cause == "OOMKilled"
    assert len(result.workflows) == 2
```

---

## 🎯 **Summary of NEW Integration Tests**

| Category | Tests | Focus | External Services Used |
|----------|-------|-------|------------------------|
| Workflow Search Logic | 5 | SearchWorkflowCatalogTool with real DS | Data Storage, PostgreSQL, Redis |
| LLM Prompt Building | 3 | PromptBuilder with real workflow fetching | Data Storage, PostgreSQL |
| Audit Event Logic | 4 | Audit client with real persistence | Data Storage, PostgreSQL |
| LLM Response Parsing | 3 | Parser with real workflow validation | Data Storage, PostgreSQL |
| **Total NEW** | **15** | **Business functions + real services** | **All real** |

**Total Integration Tests After Implementation**: 35 existing + 15 new = **50 integration tests**

---

# 🌐 **TIER 3: E2E TESTS** (<10% BR Coverage | 50%+ Code Coverage Target) - ⏸️ TO BE CREATED

**Location**: `test/e2e/aianalysis/` (follow Go service pattern)
**Infrastructure**: Containerized HAPI in Kind cluster + Data Storage + PostgreSQL + Redis
**Execution**: `make test-e2e-aianalysis` (to be created)
**Status**: ⏸️ Not yet implemented (separate session)

---

## 📋 **Tests to Move from Integration to E2E**

**Current Location**: `holmesgpt-api/tests/integration/`
**New Location**: `test/e2e/aianalysis/hapi_*.go` (follow Go E2E pattern)

### E2E Test 1: Custom Labels End-to-End Flow

**Source**: `test_custom_labels_integration_dd_hapi_001.py` (5 tests)
**Business Outcome**: Custom labels flow through complete HAPI → DS pipeline
**Pattern**: HTTP POST to containerized HAPI, verify response

**Tests to Move**:
1. `test_incident_request_with_custom_labels_in_enrichment_results` → E2E-HAPI-001-01
2. `test_incident_request_without_custom_labels` → E2E-HAPI-001-02
3. `test_incident_request_with_empty_custom_labels` → E2E-HAPI-001-03
4. `test_recovery_request_with_custom_labels_in_enrichment_results` → E2E-HAPI-001-04
5. `test_recovery_request_without_custom_labels` → E2E-HAPI-001-05

### E2E Test 2: Mock LLM Mode End-to-End Flow

**Source**: `test_mock_llm_mode_integration.py` (13 tests)
**Business Outcome**: HAPI returns deterministic responses in mock mode
**Pattern**: HTTP POST to containerized HAPI with MOCK_LLM=true

**Tests to Move**:
1. `test_incident_endpoint_returns_200_in_mock_mode` → E2E-HAPI-002-01
2. `test_incident_response_has_aianalysis_required_fields` → E2E-HAPI-002-02
3. `test_incident_response_workflow_has_required_fields` → E2E-HAPI-002-03
4. `test_incident_response_is_deterministic` → E2E-HAPI-002-04
5. `test_incident_validation_still_enforced_in_mock_mode` → E2E-HAPI-002-05
6. `test_incident_mock_response_indicates_mock_mode` → E2E-HAPI-002-06
7. `test_incident_different_signal_types_produce_different_workflows` → E2E-HAPI-002-07
8. `test_recovery_endpoint_returns_200_in_mock_mode` → E2E-HAPI-002-08
9. `test_recovery_response_has_required_fields` → E2E-HAPI-002-09
10. `test_recovery_response_strategies_are_deterministic` → E2E-HAPI-002-10
11. `test_recovery_validation_enforced_in_mock_mode` → E2E-HAPI-002-11
12. `test_recovery_mock_response_indicates_mock_mode` → E2E-HAPI-002-12
13. `test_recovery_different_signal_types_produce_different_strategies` → E2E-HAPI-002-13

**Total E2E Tests**: 18 (to be containerized and moved in separate session)

---

# 📊 **Current Test Status (Pre-Implementation Baseline)**

## Test Count by Tier

| Tier | Existing | NEW | To Move | Final Target |
|------|----------|-----|---------|--------------|
| Unit | 569 | 0 | 0 | **569** |
| Integration | 35 | **15** | -18 | **50** |
| E2E | 0 | 0 | **+18** | **18** |
| **Total** | **604** | **15** | **0** | **637** |

## Test Status by Category

| Category | Unit | Integration | E2E | Total |
|----------|------|-------------|-----|-------|
| Workflow Search | ✅ (mocked) | 🔴 **5 NEW** | ⏸️ (to move) | 5 + 5 + 5 = 15 |
| LLM Prompt Building | ✅ (mocked) | 🔴 **3 NEW** | ⏸️ (to move) | 3 + 3 + 13 = 19 |
| Audit Events | ✅ 13 | ✅ 5 + 🔴 **4 NEW** | ⏸️ (to move) | 13 + 9 + 5 = 27 |
| LLM Response Parsing | ✅ 20 | 🔴 **3 NEW** | - | 20 + 3 = 23 |
| Labels Integration | ✅ (mocked) | ✅ 16 | ⏸️ **5 to move** | 16 + 16 + 5 = 37 |
| Other (sanitization, circuit breaker) | ✅ 520 | ✅ 14 | - | 520 + 14 = 534 |

## Code Coverage Impact

| Tier | Current | Target | Gap |
|------|---------|--------|-----|
| Unit | ~27% | 70%+ | No change (complete) |
| Integration | ~23% (35 tests) | **50%+** | **+27%** (15 NEW tests) |
| E2E | 0% | 50%+ | +50% (separate session) |
| **Combined** | **~50%** | **~100%** | **+50%** (this plan + E2E) |

---

# 📊 **Pre/Post Comparison (Value Proposition)**

## Before This Plan

| Metric | Value | Issue |
|--------|-------|-------|
| Integration Tests | 35 passing, 18 blocked | 18 tests miscategorized (are E2E) |
| Code Coverage | ~27% | Low integration coverage |
| Test Pattern | Mixed (35 direct, 18 HTTP) | Inconsistent with DS pattern |
| E2E Tests | 0 | No black-box testing |
| HAPI Infrastructure | Missing | Cannot run 18 HTTP tests |

**Problems**:
1. 🔴 18 integration tests blocked (need HAPI service)
2. 🔴 Low code coverage from integration tests (~27% vs 50%+ target)
3. 🔴 Inconsistent test patterns (some direct, some HTTP)
4. 🔴 No E2E testing (black-box validation missing)

## After This Plan

| Metric | Value | Improvement |
|--------|-------|-------------|
| Integration Tests | **50 passing** (35 existing + 15 NEW) | ✅ All tests call business logic directly |
| Code Coverage | **50%+** | ✅ +27% from testing business functions with real services |
| Test Pattern | **100% consistent** | ✅ All follow DS pattern (direct function calls) |
| E2E Tests | **18 tests** | ✅ Moved from integration + containerized HAPI |
| Defense-in-Depth | **Complete** | ✅ 3 layers: unit → integration → E2E |

**Benefits**:
1. ✅ Clear test tier separation (business logic vs black-box)
2. ✅ Higher code coverage (50%+ integration + 50%+ E2E = ~100% combined)
3. ✅ Consistent pattern following DS example
4. ✅ Defense-in-depth complete (3 layers)
5. ✅ Business outcomes validated at all tiers

---

# 🏗️ **Infrastructure Setup**

## Integration Test Infrastructure

**Services Required**:
- ✅ PostgreSQL (port 15439) - Already running via podman-compose
- ✅ Redis (port 16387) - Already running via podman-compose
- ✅ Data Storage API (port 18094) - Already running via podman-compose
- ❌ HAPI Service - **NOT NEEDED** (tests call Python functions directly)

**Start Infrastructure**:
```bash
cd holmesgpt-api/tests/integration
bash ./setup_workflow_catalog_integration.sh
```

**Verify Infrastructure**:
```bash
# Check Data Storage health
curl http://localhost:18094/health
# Expected: {"status":"healthy","database":"connected"}

# Check PostgreSQL
psql -h localhost -p 15439 -U kubernaut -d kubernaut -c "SELECT 1"

# Check Redis
redis-cli -h localhost -p 16387 ping
```

**Run Integration Tests**:
```bash
cd holmesgpt-api
MOCK_LLM=true python3 -m pytest tests/integration/ -v
```

## E2E Test Infrastructure

**Services Required** (separate session):
- Kind cluster
- Containerized HAPI (Dockerfile + K8s manifests)
- Containerized Data Storage
- PostgreSQL in Kind
- Redis in Kind

**Pattern**: Follow AIAnalysis E2E pattern (`test/e2e/aianalysis/`)

---

# 📋 **Test Outcomes by Tier**

## Unit Tests (569 tests)

| BR ID | Business Outcome | Test ID | Code Coverage | Status |
|-------|------------------|---------|---------------|--------|
| BR-AUDIT-005 | Audit event structure validation | UT-AUDIT-005-* | 70%+ | ✅ 13 passing |
| BR-AI-002 | Secret leakage prevention | UT-AI-002-* | 70%+ | ✅ 46 passing |
| BR-AI-003 | LLM self-correction | UT-AI-003-* | 70%+ | ✅ 20 passing |
| BR-AI-004 | Circuit breaker (timeout) | UT-AI-004-* | 70%+ | ✅ 39 passing |
| DD-004 | RFC 7807 error responses | UT-DD-004-* | 70%+ | ✅ 13 passing |

## Integration Tests (50 tests: 35 existing + 15 NEW)

| BR ID | Business Outcome | Test ID | Code Coverage | Status |
|-------|------------------|---------|---------------|--------|
| BR-HAPI-250 | Workflow search with labels | IT-HAPI-250-* | 50%+ | 🔴 5 NEW needed |
| BR-AI-001 | LLM prompt building | IT-AI-001-* | 50%+ | 🔴 3 NEW needed |
| BR-AUDIT-005 | Audit event persistence | IT-AUDIT-005-* | 50%+ | ✅ 5 + 🔴 4 NEW |
| BR-AI-003 | LLM response parsing | IT-AI-003-* | 50%+ | 🔴 3 NEW needed |

## E2E Tests (18 tests: to be moved)

| BR ID | Business Outcome | Test ID | Code Coverage | Status |
|-------|------------------|---------|---------------|--------|
| DD-HAPI-001 | Custom labels end-to-end | E2E-HAPI-001-* | 50%+ | ⏸️ 5 to move |
| BR-AI-001 | Mock LLM mode | E2E-HAPI-002-* | 50%+ | ⏸️ 13 to move |

---

# ⚙️ **Execution Commands**

## Unit Tests (No Changes)

```bash
# Run all unit tests
cd holmesgpt-api
python3 -m pytest tests/unit/ -v

# Expected: 569 passed
```

## Integration Tests (After Implementation)

```bash
# Start infrastructure (if not running)
cd holmesgpt-api/tests/integration
bash ./setup_workflow_catalog_integration.sh

# Run all integration tests
cd holmesgpt-api
MOCK_LLM=true python3 -m pytest tests/integration/ -v

# Expected: 50 passed

# Run specific categories
python3 -m pytest tests/integration/test_workflow_search_business_logic.py -v
python3 -m pytest tests/integration/test_llm_prompt_business_logic.py -v
python3 -m pytest tests/integration/test_audit_business_logic.py -v
python3 -m pytest tests/integration/test_llm_response_parsing.py -v

# Stop infrastructure
cd tests/integration
bash ./teardown_workflow_catalog_integration.sh
```

## E2E Tests (Separate Session)

```bash
# To be defined in E2E infrastructure session
make test-e2e-aianalysis
```

---

# 📅 **Day-by-Day Implementation Timeline**

## Phase 1: NEW Integration Tests (Days 1-2)

### Day 1: Workflow Search + LLM Prompt Tests (4 hours)

**Morning (2 hours)**: Workflow Search Business Logic (5 tests)
- [ ] Create `tests/integration/test_workflow_search_business_logic.py`
- [ ] IT-HAPI-250-01: Search with detected labels
- [ ] IT-HAPI-250-02: Search with custom labels
- [ ] IT-HAPI-250-03: Severity prioritization
- [ ] IT-HAPI-250-04: Top-K limiting
- [ ] IT-HAPI-250-05: Empty results handling
- [ ] Verify: 5 tests passing, data_storage_url fixture working

**Afternoon (2 hours)**: LLM Prompt Building (3 tests)
- [ ] Create `tests/integration/test_llm_prompt_business_logic.py`
- [ ] IT-AI-001-01: Workflow inclusion from DS
- [ ] IT-AI-001-02: K8s context assembly
- [ ] IT-AI-001-03: Token count optimization
- [ ] Verify: 3 tests passing, prompts valid

**End of Day 1**: 8 tests passing (5 workflow + 3 prompt)

### Day 2: Audit + Response Parsing Tests (4 hours)

**Morning (2 hours)**: Audit Event Business Logic (4 tests)
- [ ] Create `tests/integration/test_audit_business_logic.py`
- [ ] IT-AUDIT-005-01: LLM request audit storage
- [ ] IT-AUDIT-005-02: LLM response audit storage
- [ ] IT-AUDIT-005-03: Batch write buffering
- [ ] IT-AUDIT-005-04: DS unavailability handling
- [ ] Verify: 4 tests passing, audit events in PostgreSQL

**Afternoon (2 hours)**: LLM Response Parsing (3 tests)
- [ ] Create `tests/integration/test_llm_response_parsing.py`
- [ ] IT-AI-003-01: JSON extraction
- [ ] IT-AI-003-02: Self-correction on failure
- [ ] IT-AI-003-03: Pydantic model conversion
- [ ] Verify: 3 tests passing, parsing robust

**End of Day 2**: 15 NEW tests passing (total: 35 + 15 = 50 integration tests)

## Phase 2: Move Tests to E2E (Day 3 - Separate Session)

**Morning (2 hours)**: Create E2E Infrastructure
- [ ] Create `test/e2e/aianalysis/suite_test.go` (HAPI E2E suite)
- [ ] Create HAPI Dockerfile
- [ ] Create K8s manifests (deployment, service, configmap)
- [ ] Add to Kind deployment automation

**Afternoon (2 hours)**: Move Custom Labels Tests
- [ ] Port 5 tests from `test_custom_labels_integration_dd_hapi_001.py` to Go
- [ ] Verify E2E-HAPI-001-* tests passing in Kind

### Day 4: Move Mock LLM Tests (4 hours)

**Full Day**: Port Mock LLM Mode Tests
- [ ] Port 13 tests from `test_mock_llm_mode_integration.py` to Go
- [ ] Verify E2E-HAPI-002-* tests passing in Kind
- [ ] Remove original Python HTTP tests from integration/

**End of Day 4**: 18 E2E tests passing in Kind

---

# ✅ **Success Criteria**

## Integration Tests

- [ ] **50 integration tests passing** (35 existing + 15 NEW)
- [ ] **All tests call Python functions directly** (no HTTP/FastAPI)
- [ ] **All tests use real Data Storage** (no mocks for external services)
- [ ] **Code coverage ≥50%** from integration tests
- [ ] **Test execution time <5 minutes** (fast enough for CI)
- [ ] **Zero infrastructure failures** (stable podman-compose)

## Test Quality

- [ ] **Every test maps to BR-* requirement** (business outcome focused)
- [ ] **Test names describe business outcomes** (not implementation details)
- [ ] **Assertions validate business behavior** (not internal state)
- [ ] **Tests are independent** (can run in any order)
- [ ] **Clear arrange-act-assert structure** (readable and maintainable)

## Documentation

- [ ] **Test plan approved by team**
- [ ] **NEW tests have BR-* traceability** (comments in test code)
- [ ] **Integration pattern documented** (examples in README)
- [ ] **E2E migration plan documented** (separate session plan)

## Defense-in-Depth

- [ ] **Same BRs tested at all 3 tiers** (unit → integration → E2E)
- [ ] **Code coverage approaching 100%** (combined tiers)
- [ ] **Clear tier boundaries** (business logic vs black-box)
- [ ] **Consistent test patterns** (follow DS example)

---

# 📝 **Compliance Sign-Off**

## Test Plan Approval

| Role | Name | Sign-Off | Date |
|------|------|----------|------|
| **HAPI Team Lead** | [Name] | [ ] Approved | [Date] |
| **Quality Assurance** | [Name] | [ ] Approved | [Date] |
| **Architecture Review** | [Name] | [ ] Approved | [Date] |

## Verification Checklist

### Before Implementation
- [ ] Test plan reviewed against authoritative template (NT TEST_PLAN_NT_V1_0_MVP.md)
- [ ] Business requirements (BR-*) verified
- [ ] Design decisions (DD-*) referenced
- [ ] Defense-in-depth strategy confirmed
- [ ] Timeline realistic (2 days for integration, 2 days for E2E)

### After Implementation
- [ ] 50 integration tests passing (100% pass rate)
- [ ] Code coverage ≥50% from integration tests
- [ ] All tests follow DS pattern (direct function calls)
- [ ] 18 E2E tests moved and passing in Kind
- [ ] Documentation updated (README, handoff docs)

---

## 📚 **References**

### Authoritative Documents
- [03-testing-strategy.mdc](../.cursor/rules/03-testing-strategy.mdc) - Defense-in-depth testing approach
- [Test Plan Best Practices](../docs/development/testing/TEST_PLAN_BEST_PRACTICES.md) - Template guidance
- [NT Test Plan](../docs/services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md) - Reference implementation
- [DD-TEST-002](../docs/architecture/decisions/DD-TEST-002-integration-test-container-orchestration.md) - Integration test pattern

### Example Implementations
- [DS Repository Tests](../test/integration/datastorage/repository_test.go) - Direct function call pattern
- [DS HTTP API Tests](../test/integration/datastorage/http_api_test.go) - Containerized service pattern
- [AIAnalysis E2E](../test/integration/aianalysis/recovery_integration_test.go) - Go E2E example

---

**Document Version**: 1.0.0
**Status**: DRAFT - Awaiting Team Review
**Next Steps**: Review test plan → Approve → Implement Day 1 tests → Continue timeline



