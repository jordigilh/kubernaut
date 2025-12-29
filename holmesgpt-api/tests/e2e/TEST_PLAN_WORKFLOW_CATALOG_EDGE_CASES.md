# Workflow Catalog Tool Test Plan - Defense-in-Depth Approach

**Version**: 1.3.0
**Last Updated**: December 22, 2025
**Status**: TEMPLATE

**Cross-References**:
- [Test Plan Best Practices](../../../docs/development/testing/TEST_PLAN_BEST_PRACTICES.md) - When/why to use each section
- [NT Test Plan Example](../../../docs/services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md) - Complete implementation reference

**Business Requirement**: BR-STORAGE-013 - Semantic Search for Remediation Workflows
**Design Decision**: DD-WORKFLOW-002 v3.0 - MCP Workflow Catalog Architecture
**Authority**: DD-TEST-001 - Port Allocation Strategy

---

## 📋 **Changelog**

### Version 1.3.0 (2025-12-22)
- **ADDED**: Current Test Status section (know what's done vs. new)
- **ADDED**: Pre/Post Comparison section (quantify value proposition)
- **ADDED**: Day-by-Day Timeline section (actionable execution plan)
- **ADDED**: Infrastructure Setup section (reproducible environment setup)
- **ADDED**: Cross-references to Best Practices and NT example
- **GUIDANCE**: Added inline comments for when to use each section

### Version 1.2.0 (2025-12-22)
- **UPDATED**: Code coverage targets to 70%/50%/50% based on empirical DS/SP data
- **CLARIFIED**: E2E code coverage target increased from 10-20% to 50%
- **ADDED**: Example showing same code tested at multiple layers

### Version 1.1.0 (2025-12-22)
- **CORRECTED**: Changed "Test Pyramid" to "Defense-in-Depth Testing" per Kubernaut testing strategy
- **CLARIFIED**: BR coverage is overlapping (70% + 50% + 10%), not mutually exclusive
- **ADDED**: Code coverage vs BR coverage distinction
- **ADDED**: Defense-in-depth principle explanation (same code tested at multiple layers)

### Version 1.0.0 (2025-11-29)
- Initial test plan for Workflow Catalog Tool
- 27 tests across 3 tiers (15 unit + 10 integration + 2 E2E)

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

## 📊 **Defense-in-Depth Testing Summary**

**Strategy**: Overlapping BR coverage + cumulative code coverage approaching 100% (per Kubernaut testing strategy)

| Tier | Tests | Infrastructure | BR Coverage | Code Coverage | What It Validates |
|------|-------|----------------|-------------|---------------|-------------------|
| **Unit** | 15 | Mocked HTTP responses | 70%+ of ALL BRs | 70%+ | Algorithm correctness, edge cases, error handling |
| **Integration** | 10 | Real Data Storage Service | >50% of ALL BRs | 50% | Contract validation, real service behavior |
| **E2E** | 2 | Real Data Storage + Bootstrap data | <10% BR coverage | 50% | Critical paths, full tool invocation flow |

**Total**: 27 tests

**Defense-in-Depth Principle**:
- **BR Coverage**: Overlapping (same BRs tested at multiple tiers)
- **Code Coverage**: Cumulative (70% + 50% + 50% = ~100% combined, with 50%+ tested in all 3 tiers)
- **Empirical Validation**: DataStorage and SignalProcessing demonstrate E2E achieves 50%+ code coverage

**Example - Response Transformation**:
- **Unit (70%)**: Mock HTTP responses, test parsing logic - tests `parse_response()` function
- **Integration (50%)**: Real Data Storage API, test v3.0 contract - tests same parsing with real responses
- **E2E (50%)**: Full tool invocation, test complete flow - tests same parsing in LLM context

If response parsing has a bug, it must slip through **ALL 3 defense layers** to reach production!

---

## 📊 **Current Test Status**

> **GUIDANCE**: Use this section when adding new tests to existing codebase. Shows stakeholders what's already done vs. new work needed. See [NT Test Plan](../../../docs/services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md#current-unit-test-coverage) for example.

### Pre-[Feature Name] Status

| Test Suite | Tests | Status | Coverage |
|---|---|---|---|
| [Suite name 1] | [N] | ✅ Passing | BR-XXX-XXX |
| [Suite name 2] | [M] | ✅ Passing | BR-YYY-YYY |
| [Suite name 3] | [P] | ✅ Passing | BR-ZZZ-ZZZ |

**Total Existing Tests**: [N+M+P] tests ([X] unit + [Y] integration + [Z] E2E)
**Pass Rate**: ✅ 100% (or specify failures)

### Assessment

**Unit Tests**: ✅ **NO NEW UNIT TESTS NEEDED** - Existing coverage is comprehensive
- OR -
**Unit Tests**: ⏸️ **[N] NEW UNIT TESTS NEEDED** - [Reason why]

**Integration Tests**: ✅ **NO NEW INTEGRATION TESTS NEEDED** - Existing coverage is sufficient
- OR -
**Integration Tests**: ⏸️ **[N] NEW INTEGRATION TESTS NEEDED** - [Reason why]

**E2E Tests**: ⏸️ **[N] NEW E2E TESTS NEEDED** - [Critical paths to validate]

---

# 🧪 **TIER 1: UNIT TESTS** (70%+ BR Coverage | 70%+ Code Coverage)

**Location**: `[path/to/unit/tests]`
**Infrastructure**: None (mocked external dependencies)
**Execution**: `[command to run unit tests]`

## 🏗️ **Unit Test Infrastructure Setup**

> **GUIDANCE**: Optional section. Include if unit tests require special setup (e.g., fixtures, test data). Most unit tests need no infrastructure. See [Best Practices](../../../docs/development/testing/TEST_PLAN_BEST_PRACTICES.md#infrastructure-setup-when-to-include) for guidance.

### Prerequisites

- [Prerequisite 1, e.g., Python 3.11+, Go 1.22+]
- [Prerequisite 2, e.g., pytest, Ginkgo/Gomega]

### Setup Commands

```bash
# [Setup step 1]
[command]

# [Setup step 2]
[command]

# Run unit tests
[test execution command]
```

---

### Purpose
Test [component name] logic **in isolation**. Mock all external dependencies.

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

# 🔗 **TIER 2: INTEGRATION TESTS** (>50% BR Coverage | 50% Code Coverage)

**Location**: `[path/to/integration/tests]`
**Infrastructure**: [Real services needed, e.g., "Real Kubernetes (envtest)", "Real Data Storage (Docker)", etc.]
**Execution**: `[command to run integration tests]`

## 🏗️ **Integration Test Infrastructure Setup**

> **GUIDANCE**: MANDATORY for integration tests. Integration tests require real services (envtest, Docker, etc.). See [NT Test Plan](../../../docs/services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md#-tier-2-integration-tests-50-br-coverage--50-code-coverage---complete) for Kubernetes example.

### Prerequisites

- [Real service 1, e.g., "Kubernetes cluster (envtest or Kind)"]
- [Real service 2, e.g., "Data Storage Service"]
- [Other dependencies, e.g., "Docker Compose", "PostgreSQL"]

### Setup Commands

```bash
# 1. [Start infrastructure]
[command, e.g., "docker-compose up -d"]

# 2. [Verify services are ready]
[command, e.g., "curl http://localhost:8080/health"]

# 3. [Run integration tests]
[test execution command]

# 4. [Cleanup]
[command, e.g., "docker-compose down"]
```

### Infrastructure Validation

```bash
# Verify all integration prerequisites are met
[validation command]

# Expected checks:
# ✅ [Service 1] accessible
# ✅ [Service 2] responding
# ✅ [Dependency 3] configured
```

---

### Purpose
Test **actual integration behavior** between [component A] and [component B].

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

# 🚀 **TIER 3: E2E TESTS** (<10% BR Coverage | 50% Code Coverage)

**Location**: `[path/to/e2e/tests]`
**Infrastructure**: [Full stack description, e.g., "Real Kubernetes (Kind) + All controllers deployed"]
**Execution**: `[command to run E2E tests]`

## 🏗️ **E2E Infrastructure Setup**

> **GUIDANCE**: MANDATORY for E2E tests. E2E tests require production-like environment. See [NT Test Plan](../../../docs/services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md#%EF%B8%8F-e2e-infrastructure-setup) for Kubernetes example.

### Prerequisites

- [Full stack component 1, e.g., "Kind cluster running"]
- [Full stack component 2, e.g., "All controllers deployed"]
- [Full stack component 3, e.g., "CRDs registered"]
- [Test data, e.g., "Bootstrap data loaded"]

### Setup Commands

```bash
# 1. [Create cluster or start services]
[command, e.g., "make kind-up"]

# 2. [Deploy application]
[command, e.g., "make deploy"]

# 3. [Verify deployment]
[command, e.g., "kubectl get pods -n kubernaut-system"]

# Expected output:
# [Expected pod status]

# 4. [Load test data]
[command, e.g., "make bootstrap-test-data"]

# 5. [Run E2E tests]
[test execution command]
```

### Infrastructure Validation

```bash
# Verify all E2E prerequisites are met
[validation command]

# Expected checks:
# ✅ [Cluster/services] accessible
# ✅ [Application] deployed
# ✅ [CRDs/schemas] registered
# ✅ [Test data] loaded
```

---

### Purpose
Test **critical user journeys** end-to-end. Validate business outcomes in production-like environment.

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

| Tier | What It Proves | Failure Means | Code Coverage |
|------|----------------|---------------|---------------|
| **Unit** | [Component] logic is correct | Bug in [component] code | 70%+ |
| **Integration** | [Service A] ↔ [Service B] integration works | API mismatch or integration issue | 50% |
| **E2E** | [Business outcome] works end-to-end | System doesn't serve business need | 50% |

---

## 🎉 **Expected Outcomes**

> **GUIDANCE**: Use this section to quantify value proposition for stakeholders. Shows confidence improvement and test coverage increase. See [NT Test Plan](../../../docs/services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md#-expected-outcomes) for example.

### Pre-[Feature Name] Status:
- ✅ [N] tests passing ([X] unit + [Y] integration + [Z] E2E)
- ✅ 100% pass rate across all tiers (or specify pass rate)
- ✅ BR-XXX-XXX, BR-YYY-YYY validated
- ✅ [X]% confidence for production

### Post-[Feature Name] Status (Target):
- ✅ [N+M] tests passing ([X+A] unit + [Y+B] integration + [Z+C] E2E)
- ✅ 100% pass rate across all tiers (target)
- ✅ BR-XXX-XXX, BR-YYY-YYY, BR-ZZZ-ZZZ validated (additional BRs)
- ✅ [Y]% confidence for production

### Confidence Improvement:
- **Before [Feature]**: [X]% confidence for production
- **After [Feature]**: [Y]% confidence for production
- **Improvement**: +[Y-X]% confidence increase

**Rationale**: [Explain why confidence improves, e.g., "Critical retry logic validated in real cluster", "Multi-channel fanout tested end-to-end"]

---

# ✅ **Implementation Checklist**

> **GUIDANCE**: Use week-level checklist for high-level tracking. For detailed day-by-day execution, see "Execution Timeline" section below.

## [Phase 1 Name, e.g., "Unit Tests"] (Week 1)
- [ ] [Test group 1, e.g., "U1.1-U1.8: Input validation"]
- [ ] [Test group 2, e.g., "U2.1-U2.5: Response transformation"]
- [ ] [Test group 3, e.g., "U3.1-U3.2: Error handling"]

## [Phase 2 Name, e.g., "Integration Tests"] (Week 2)
- [ ] [Test group 1, e.g., "I1.1-I1.5: Contract validation"]
- [ ] [Test group 2, e.g., "I2.1-I2.3: Semantic search behavior"]
- [ ] [Test group 3, e.g., "I3.1-I3.2: Error propagation"]

## [Phase 3 Name, e.g., "E2E Tests"] (Week 2)
- [ ] [Test 1, e.g., "E1.1: OOMKilled critical path"]
- [ ] [Test 2, e.g., "E1.2: CrashLoopBackOff critical path"]

---

## ⏱️ **Execution Timeline**

> **GUIDANCE**: Use this section for actionable day-by-day execution plan. More detailed than checklist. See [NT Test Plan](../../../docs/services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md#%EF%B8%8F-execution-timeline) for example with specific tasks per day.

### Week 1: [Phase Name, e.g., "Core Feature Tests"]

| Day | Task | Time | Owner | Deliverable |
|---|---|---|---|---|
| **Day 1** | [Test name or group] | [X] day | [Team/Person] | [What's delivered] |
| **Day 2 AM** | [Test name or group] | 0.5 day | [Team/Person] | [What's delivered] |
| **Day 2 PM** | [Test name or group] | 0.5 day | [Team/Person] | [What's delivered] |
| **Day 3** | [Test name or group] | [X] day | [Team/Person] | [What's delivered] |

### Week 2: [Phase Name, e.g., "Validation & Documentation"]

| Day | Task | Time | Owner | Deliverable |
|---|---|---|---|---|
| **Day 4** | [Task name] | 0.5 day | [Team/Person] | [What's delivered] |
| **Day 4** | [Task name] | 0.5 day | [Team/Person] | [What's delivered] |
| **Day 5** | [Task name] | [X] day | [Team/Person] | [What's delivered] |

**Total Time**: **[X] days**

**Critical Path**: [Identify dependencies, e.g., "E2E tests require integration tests to pass first"]

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
