# Workflow Catalog Tool Test Plan - Tiered Approach

**Business Requirement**: BR-STORAGE-013 - Semantic Search for Remediation Workflows
**Design Decision**: DD-WORKFLOW-002 v3.0 - MCP Workflow Catalog Architecture
**Authority**: DD-TEST-001 - Port Allocation Strategy

**Date**: 2025-11-29
**Status**: DRAFT

---

## 🎯 **Testing Scope**

```
┌─────────────────────────────────────────────────────────────┐
│ HolmesGPT-API (Python)                                      │
│   └── WorkflowCatalogTool                                   │
│         ├── Input validation                                │
│         ├── HTTP request construction                       │
│         ├── Response parsing & transformation               │
│         └── Error handling                                  │
└───────────────────────┬─────────────────────────────────────┘
                        │ HTTP POST /api/v1/workflows/search
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ Data Storage Service (Go)                                   │
│   ├── Semantic search (pgvector)                            │
│   └── Returns workflow metadata                             │
└─────────────────────────────────────────────────────────────┘
```

---

## 📊 **Test Pyramid Summary**

| Tier | Tests | Infrastructure | Coverage Target |
|------|-------|----------------|-----------------|
| **Unit** | 15 | Mocked HTTP responses | 70% - Fast, isolated |
| **Integration** | 10 | Real Data Storage Service | 20% - Contract validation |
| **E2E** | 2 | Real Data Storage + Bootstrap data | 10% - Critical paths |

**Total**: 27 tests

---

# 🧪 **TIER 1: UNIT TESTS** (70% Coverage)

**Location**: `holmesgpt-api/tests/unit/test_workflow_catalog_tool.py`
**Infrastructure**: None (mocked HTTP responses)
**Execution**: `pytest tests/unit/test_workflow_catalog_tool.py`

### Purpose
Test WorkflowCatalogTool logic **in isolation**. Mock all HTTP calls to Data Storage.

---

## Unit Test Suite 1: Input Validation (8 tests)

| ID | Test Name | What It Validates | Mock Setup |
|----|-----------|-------------------|------------|
| U1.1 | `test_empty_query_returns_error` | Empty string rejected before HTTP call | No HTTP mock needed |
| U1.2 | `test_none_query_returns_error` | None/null rejected before HTTP call | No HTTP mock needed |
| U1.3 | `test_very_long_query_truncated_or_rejected` | 10,000+ char query handled | No HTTP mock needed |
| U1.4 | `test_whitespace_only_query_returns_error` | "   \t\n" rejected | No HTTP mock needed |
| U1.5 | `test_negative_top_k_returns_error` | top_k=-1 rejected | No HTTP mock needed |
| U1.6 | `test_zero_top_k_returns_empty` | top_k=0 returns empty array | Mock returns empty |
| U1.7 | `test_excessive_top_k_capped` | top_k=10000 capped to 100 | Mock returns 100 items |
| U1.8 | `test_invalid_min_similarity_rejected` | min_similarity=2.0 rejected | No HTTP mock needed |

### Example Unit Test

```python
# tests/unit/test_workflow_catalog_tool.py

class TestInputValidation:
    """Unit tests for input validation - no external services required"""

    def test_empty_query_returns_error(self):
        """U1.1: Empty query is rejected at tool level, not sent to Data Storage"""
        # ARRANGE
        tool = SearchWorkflowCatalogTool(data_storage_url="http://mock")

        # ACT
        result = tool.invoke(params={"query": "", "top_k": 5})

        # ASSERT
        assert result.status == StructuredToolResultStatus.ERROR
        assert "query" in result.error.lower() or "empty" in result.error.lower()

    def test_negative_top_k_returns_error(self):
        """U1.5: Negative top_k rejected at tool level"""
        tool = SearchWorkflowCatalogTool(data_storage_url="http://mock")

        result = tool.invoke(params={"query": "OOMKilled", "top_k": -1})

        assert result.status == StructuredToolResultStatus.ERROR
        assert "top_k" in result.error.lower() or "invalid" in result.error.lower()
```

---

## Unit Test Suite 2: Response Transformation (5 tests)

| ID | Test Name | What It Validates | Mock Response |
|----|-----------|-------------------|---------------|
| U2.1 | `test_transforms_uuid_workflow_id` | workflow_id UUID preserved | Mock with UUID |
| U2.2 | `test_transforms_title_field` | `title` field parsed (not `name`) | Mock v3.0 response |
| U2.3 | `test_transforms_singular_signal_type` | `signal_type` string (not array) | Mock v3.0 response |
| U2.4 | `test_transforms_confidence_score` | `confidence` float parsed | Mock with 0.87 |
| U2.5 | `test_handles_null_optional_fields` | null container_image doesn't crash | Mock with nulls |

### Example Unit Test

```python
class TestResponseTransformation:
    """Unit tests for v3.0 response parsing - mocked HTTP responses"""

    @patch('requests.post')
    def test_transforms_uuid_workflow_id(self, mock_post):
        """U2.1: UUID workflow_id is correctly parsed from v3.0 response"""
        # ARRANGE
        mock_post.return_value.status_code = 200
        mock_post.return_value.json.return_value = {
            "workflows": [{
                "workflow_id": "1c7fcb0c-d22b-4e7c-b994-749dd1a591bd",
                "title": "OOMKill Fix",
                "description": "Fixes OOMKilled pods",
                "signal_type": "OOMKilled",
                "confidence": 0.92,
                "container_image": "ghcr.io/kubernaut/oomkill:v1.0.0",
                "container_digest": "sha256:abc123..."
            }],
            "total_results": 1
        }

        tool = SearchWorkflowCatalogTool(data_storage_url="http://mock")

        # ACT
        result = tool.invoke(params={"query": "OOMKilled", "top_k": 5})

        # ASSERT
        data = json.loads(result.data)
        assert data["workflows"][0]["workflow_id"] == "1c7fcb0c-d22b-4e7c-b994-749dd1a591bd"
        # Validate UUID format
        uuid.UUID(data["workflows"][0]["workflow_id"])  # Raises if invalid
```

---

## Unit Test Suite 3: Error Handling (2 tests)

| ID | Test Name | What It Validates | Mock Response |
|----|-----------|-------------------|---------------|
| U3.1 | `test_http_error_returns_structured_error` | HTTP 500 → ERROR status | Mock 500 |
| U3.2 | `test_invalid_json_returns_error` | Malformed JSON → ERROR | Mock invalid JSON |

---

# 🔗 **TIER 2: INTEGRATION TESTS** (20% Coverage)

**Location**: `holmesgpt-api/tests/integration/test_workflow_catalog_data_storage.py`
**Infrastructure**: Real Data Storage Service (Docker Compose)
**Execution**: `./setup_workflow_catalog_integration.sh && pytest tests/integration/`

### Purpose
Test **actual HTTP contract** between HolmesGPT-API and Data Storage Service.

---

## Integration Test Suite 1: Contract Validation (5 tests)

| ID | Test Name | What It Validates | Requires |
|----|-----------|-------------------|----------|
| I1.1 | `test_search_request_format_accepted` | Data Storage accepts our request format | Real Data Storage |
| I1.2 | `test_response_format_is_v3_compliant` | Response matches DD-WORKFLOW-002 v3.0 | Real Data Storage |
| I1.3 | `test_workflow_id_is_uuid_from_real_service` | Real service returns UUID format | Real Data Storage |
| I1.4 | `test_confidence_ordering_from_real_service` | Results ordered by confidence DESC | Real Data Storage |
| I1.5 | `test_container_image_digest_format` | Real digests are valid sha256 | Real Data Storage |

### Example Integration Test

```python
# tests/integration/test_workflow_catalog_data_storage.py

@pytest.mark.requires_data_storage
class TestDataStorageContractValidation:
    """Integration tests validating contract with real Data Storage"""

    def test_search_request_format_accepted(self, integration_infrastructure):
        """I1.1: Data Storage accepts our search request format"""
        # ARRANGE
        tool = SearchWorkflowCatalogTool(
            data_storage_url=integration_infrastructure["data_storage_url"]
        )

        # ACT
        result = tool.invoke(params={"query": "OOMKilled critical", "top_k": 5})

        # ASSERT - Contract validation
        assert result.status == StructuredToolResultStatus.SUCCESS, \
            f"Data Storage rejected our request: {result.error}"

    def test_response_format_is_v3_compliant(self, integration_infrastructure):
        """I1.2: Response contains all DD-WORKFLOW-002 v3.0 required fields"""
        tool = SearchWorkflowCatalogTool(
            data_storage_url=integration_infrastructure["data_storage_url"]
        )

        result = tool.invoke(params={"query": "OOMKilled", "top_k": 5})
        data = json.loads(result.data)

        # ASSERT v3.0 compliance
        assert "workflows" in data
        if data["workflows"]:
            wf = data["workflows"][0]
            required_fields = ["workflow_id", "title", "description",
                             "signal_type", "confidence"]
            for field in required_fields:
                assert field in wf, f"v3.0 required field '{field}' missing"
```

---

## Integration Test Suite 2: Semantic Search Behavior (3 tests)

| ID | Test Name | What It Validates | Requires |
|----|-----------|-------------------|----------|
| I2.1 | `test_semantic_search_returns_relevant_results` | "OOMKilled" query → OOMKilled workflows | Real Data Storage + Embeddings |
| I2.2 | `test_different_queries_return_different_results` | Different queries → different top results | Real Data Storage + Embeddings |
| I2.3 | `test_top_k_limits_results` | top_k=3 returns ≤3 results | Real Data Storage |

---

## Integration Test Suite 3: Error Propagation (2 tests)

| ID | Test Name | What It Validates | Requires |
|----|-----------|-------------------|----------|
| I3.1 | `test_data_storage_unavailable_returns_error` | Connection refused → clear error | Data Storage OFF |
| I3.2 | `test_data_storage_timeout_returns_error` | Slow response → timeout error | Delayed response mock |

---

# 🚀 **TIER 3: E2E TESTS** (10% Coverage)

**Location**: `holmesgpt-api/tests/e2e/test_workflow_catalog_e2e.py`
**Infrastructure**: Full stack (Data Storage + Bootstrap data)
**Execution**: `./setup_workflow_catalog_integration.sh && pytest tests/e2e/`

### Purpose
Test **critical user journeys** end-to-end. These simulate what the LLM actually does.

---

## E2E Test Suite: Critical Paths (2 tests)

| ID | Test Name | What It Validates | Business Outcome |
|----|-----------|-------------------|------------------|
| E1.1 | `test_oomkilled_incident_finds_memory_workflow` | Complete search flow for OOMKilled | AI can recommend correct workflow |
| E1.2 | `test_crashloop_incident_finds_restart_workflow` | Complete search flow for CrashLoop | AI can recommend correct workflow |

### Example E2E Test

```python
# tests/e2e/test_workflow_catalog_e2e.py

@pytest.mark.e2e
@pytest.mark.requires_data_storage
class TestCriticalUserJourneys:
    """E2E tests for critical workflow search scenarios"""

    def test_oomkilled_incident_finds_memory_workflow(self, integration_infrastructure):
        """
        E1.1: Complete user journey - AI searches for OOMKilled remediation

        Business Outcome: When an OOMKilled alert fires, the AI can find
        a workflow that addresses memory issues.
        """
        # ARRANGE - Simulating what the LLM does
        tool = SearchWorkflowCatalogTool(
            data_storage_url=integration_infrastructure["data_storage_url"]
        )

        # ACT - LLM calls the tool
        result = tool.invoke(params={
            "query": "OOMKilled pod memory limit exceeded critical",
            "top_k": 5
        })

        # ASSERT - Business outcome validation
        assert result.status == StructuredToolResultStatus.SUCCESS

        data = json.loads(result.data)
        assert len(data["workflows"]) > 0, \
            "AI must find at least one workflow for OOMKilled incidents"

        top_workflow = data["workflows"][0]

        # The AI should be able to present this to the operator
        assert top_workflow["title"], "Workflow must have title for AI to present"
        assert top_workflow["description"], "Workflow must have description for AI to explain"
        assert top_workflow["confidence"] > 0.0, "Workflow must have confidence score"

        # Signal type should be relevant to memory issues
        assert top_workflow["signal_type"] in ["OOMKilled", "MemoryPressure"], \
            f"Top workflow should address memory issues, got: {top_workflow['signal_type']}"

    def test_crashloop_incident_finds_restart_workflow(self, integration_infrastructure):
        """
        E1.2: Complete user journey - AI searches for CrashLoopBackOff remediation

        Business Outcome: When a CrashLoopBackOff alert fires, the AI can find
        a workflow that addresses container restart issues.
        """
        tool = SearchWorkflowCatalogTool(
            data_storage_url=integration_infrastructure["data_storage_url"]
        )

        result = tool.invoke(params={
            "query": "CrashLoopBackOff container keeps restarting high severity",
            "top_k": 5
        })

        assert result.status == StructuredToolResultStatus.SUCCESS

        data = json.loads(result.data)
        assert len(data["workflows"]) > 0, \
            "AI must find at least one workflow for CrashLoopBackOff incidents"
```

---

# 📁 **File Structure**

```
holmesgpt-api/tests/
├── unit/
│   └── test_workflow_catalog_tool.py        # 15 unit tests (mocked)
├── integration/
│   ├── conftest.py                          # Infrastructure fixtures
│   ├── test_workflow_catalog_data_storage.py # 10 integration tests
│   └── docker-compose.workflow-catalog.yml  # Test infrastructure
└── e2e/
    ├── test_workflow_catalog_e2e.py         # 2 E2E tests
    └── TEST_PLAN_WORKFLOW_CATALOG_EDGE_CASES.md  # This document
```

---

# 🎯 **Test Outcomes by Tier**

| Tier | What It Proves | Failure Means |
|------|----------------|---------------|
| **Unit** | Tool logic is correct | Bug in HolmesGPT-API code |
| **Integration** | Contract with Data Storage works | API mismatch or service issue |
| **E2E** | AI can find workflows for real incidents | System doesn't serve business need |

---

# ✅ **Implementation Checklist**

## Unit Tests (Week 1)
- [ ] U1.1-U1.8: Input validation
- [ ] U2.1-U2.5: Response transformation
- [ ] U3.1-U3.2: Error handling

## Integration Tests (Week 2)
- [ ] I1.1-I1.5: Contract validation
- [ ] I2.1-I2.3: Semantic search behavior
- [ ] I3.1-I3.2: Error propagation

## E2E Tests (Week 2)
- [ ] E1.1: OOMKilled critical path
- [ ] E1.2: CrashLoopBackOff critical path

---

# 📊 **Execution Commands**

```bash
# Run all unit tests (fast, no infrastructure)
pytest tests/unit/test_workflow_catalog_tool.py -v

# Run integration tests (requires Data Storage)
./tests/integration/setup_workflow_catalog_integration.sh
pytest tests/integration/test_workflow_catalog_data_storage.py -v

# Run E2E tests (requires full stack)
pytest tests/e2e/test_workflow_catalog_e2e.py -v

# Run all tiers
pytest tests/ -v --ignore=tests/e2e/  # Skip E2E for CI
pytest tests/ -v                       # Full suite locally
```
