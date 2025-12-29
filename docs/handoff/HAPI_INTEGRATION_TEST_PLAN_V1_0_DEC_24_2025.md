# HAPI Integration Test Plan V1.0 - Ready for Review

**Date**: December 24, 2025
**Status**: ✅ DRAFT COMPLETE - Awaiting Team Review
**Author**: HAPI Team
**Document**: `holmesgpt-api/tests/TEST_PLAN_HAPI_INTEGRATION_V1_0.md`

---

## 🎯 **Executive Summary**

Created comprehensive integration test plan for HolmesGPT API (HAPI) following authoritative template (NT Test Plan v1.3.0). This plan addresses the current miscategorization of 18 HTTP-based tests as integration tests and establishes a clear path to defense-in-depth testing.

**Key Insight**: Current "integration" tests are split between:
- ✅ **35 tests** calling Python business functions directly with real Data Storage (CORRECT)
- 🔴 **18 tests** making HTTP calls expecting HAPI service running (THESE ARE E2E TESTS)

**Solution**: Create 15 NEW integration tests following Data Storage pattern (direct function calls) and move 18 HTTP tests to E2E tier.

---

## 📊 **Current State → Target State**

### Before This Plan

| Tier | Tests | Pattern | Issue |
|------|-------|---------|-------|
| Unit | 569 | ✅ Mocks | Complete, no changes |
| Integration | 35 + 18 | 🔴 Mixed | 18 are actually E2E |
| E2E | 0 | ❌ Missing | No black-box testing |
| **Coverage** | **~27%** | 🔴 Low | Inadequate integration coverage |

### After This Plan

| Tier | Tests | Pattern | Status |
|------|-------|---------|--------|
| Unit | 569 | ✅ Mocks | No changes |
| Integration | **50** (35 + 15 NEW) | ✅ Direct functions | +15 NEW business logic tests |
| E2E | **18** (moved) | ✅ HTTP to containerized HAPI | Proper E2E tier |
| **Coverage** | **~100%** | ✅ Defense-in-depth | 50% integration + 50% E2E |

---

## 🧪 **Test Plan Structure**

### Defense-in-Depth Strategy

**Overlapping BR Coverage + Cumulative Code Coverage**:
- **Unit (569 tests)**: 70%+ BR coverage, ~27% code coverage
- **Integration (50 tests)**: >50% BR coverage, **50%+ code coverage** (target)
- **E2E (18 tests)**: <10% BR coverage, **50%+ code coverage** (target)

**Combined Target**: ~100% code coverage across all tiers (defense-in-depth complete)

---

## 🎯 **15 NEW Integration Tests to Create**

### Category 1: Workflow Search Business Logic (5 tests)

**BR-HAPI-250**: Workflow Catalog Search

| Test ID | Business Outcome |
|---------|------------------|
| IT-HAPI-250-01 | Workflow search with detected labels filters results |
| IT-HAPI-250-02 | Workflow search with custom labels appends to query |
| IT-HAPI-250-03 | Workflow search prioritizes by severity and signal type |
| IT-HAPI-250-04 | Workflow search respects top_k parameter |
| IT-HAPI-250-05 | Workflow search handles empty results gracefully |

**Pattern**:
```python
from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool

def test_workflow_search_with_detected_labels(data_storage_url):
    """IT-HAPI-250-01: Business outcome test"""
    tool = SearchWorkflowCatalogTool(data_storage_url=data_storage_url)
    result = tool._search_workflows(...)  # Direct function call
    assert result['total_results'] > 0
```

### Category 2: LLM Prompt Building Business Logic (3 tests)

**BR-AI-001**: LLM Context Optimization

| Test ID | Business Outcome |
|---------|------------------|
| IT-AI-001-01 | Prompt builder includes top-K workflows from Data Storage |
| IT-AI-001-02 | Prompt builder assembles Kubernetes context correctly |
| IT-AI-001-03 | Prompt builder optimizes token count under limit |

### Category 3: Audit Event Business Logic (4 tests)

**BR-AUDIT-005**: Audit Trail

| Test ID | Business Outcome |
|---------|------------------|
| IT-AUDIT-005-01 | Audit client stores LLM request events in Data Storage |
| IT-AUDIT-005-02 | Audit client stores LLM response events in Data Storage |
| IT-AUDIT-005-03 | Audit client buffers events for batch write (ADR-038) |
| IT-AUDIT-005-04 | Audit client handles Data Storage unavailability gracefully |

### Category 4: LLM Response Parsing Business Logic (3 tests)

**BR-AI-003**: LLM Self-Correction

| Test ID | Business Outcome |
|---------|------------------|
| IT-AI-003-01 | Response parser extracts JSON from LLM output |
| IT-AI-003-02 | Response parser triggers self-correction on parse failure |
| IT-AI-003-03 | Response parser converts to Pydantic models |

**Total NEW**: **15 integration tests**

---

## 🌐 **18 Tests to Move to E2E**

### Current Location: `holmesgpt-api/tests/integration/`
### New Location: `test/e2e/aianalysis/hapi_*.go`

| Source File | Tests | New E2E Tests |
|-------------|-------|---------------|
| `test_custom_labels_integration_dd_hapi_001.py` | 5 | E2E-HAPI-001-* |
| `test_mock_llm_mode_integration.py` | 13 | E2E-HAPI-002-* |

**Reason**: These tests make HTTP calls to HAPI service → E2E tests, not integration tests

**E2E Pattern**: Containerize HAPI, deploy to Kind, test via HTTP (black-box)

---

## 📅 **Implementation Timeline**

### Phase 1: NEW Integration Tests (Days 1-2)

**Day 1 (4 hours)**:
- ✅ Workflow Search (5 tests)
- ✅ LLM Prompt Building (3 tests)
- **Target**: 8 tests passing

**Day 2 (4 hours)**:
- ✅ Audit Events (4 tests)
- ✅ LLM Response Parsing (3 tests)
- **Target**: 15 tests passing

**Deliverable**: 50 integration tests passing (35 existing + 15 NEW)

### Phase 2: Move Tests to E2E (Days 3-4 - Separate Session)

**Day 3**: E2E infrastructure (Dockerfile, K8s manifests, Kind deployment)
**Day 4**: Port 18 tests to Go E2E format

**Deliverable**: 18 E2E tests passing in Kind

---

## 📊 **Success Criteria**

### Integration Tests
- [ ] **50 integration tests passing** (35 existing + 15 NEW)
- [ ] **All tests call Python functions directly** (no HTTP)
- [ ] **All tests use real Data Storage** (no mocks for external services)
- [ ] **Code coverage ≥50%** from integration tests
- [ ] **Test execution time <5 minutes**

### Test Quality
- [ ] **Every test maps to BR-* requirement** (business outcome focused)
- [ ] **Test names describe business outcomes** (not implementation details)
- [ ] **Assertions validate business behavior** (not internal state)
- [ ] **Tests are independent** (can run in any order)

### Defense-in-Depth
- [ ] **Same BRs tested at all 3 tiers** (unit → integration → E2E)
- [ ] **Code coverage approaching 100%** (combined tiers)
- [ ] **Clear tier boundaries** (business logic vs black-box)

---

## 🔗 **References**

### Authoritative Documents
- [Test Plan Template](../services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md) - NT v1.3.0
- [Test Plan Best Practices](../development/testing/TEST_PLAN_BEST_PRACTICES.md) - Guidance
- [DD-TEST-002](../architecture/decisions/DD-TEST-002-integration-test-container-orchestration.md) - Integration pattern
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc) - Defense-in-depth

### Example Implementations
- [DS Repository Tests](../../test/integration/datastorage/repository_test.go) - Direct function pattern
- [DS HTTP API Tests](../../test/integration/datastorage/http_api_test.go) - Containerized service pattern
- [AIAnalysis E2E](../../test/integration/aianalysis/recovery_integration_test.go) - Go E2E example

---

## 📋 **Next Steps**

1. **Review Test Plan** (this document)
   - Validate 15 NEW test scenarios cover business outcomes
   - Confirm timeline realistic (2 days integration + 2 days E2E)
   - Approve defense-in-depth strategy

2. **Approve for Implementation**
   - Sign-off from HAPI Team Lead
   - Sign-off from Quality Assurance
   - Sign-off from Architecture Review

3. **Begin Day 1 Implementation**
   - Create `tests/integration/test_workflow_search_business_logic.py`
   - Implement 5 workflow search tests
   - Verify tests passing with real Data Storage

4. **Continue Timeline** (Day 2 → Day 4)

---

## 🎯 **Key Takeaways**

1. **Pattern Discovery**: HAPI's current "integration" tests are split between:
   - ✅ **35 tests**: Direct function calls (CORRECT)
   - 🔴 **18 tests**: HTTP calls (ACTUALLY E2E)

2. **Solution**: Follow Data Storage pattern:
   - **Integration**: Test business logic directly with real services
   - **E2E**: Test containerized service via HTTP (black-box)

3. **Value Proposition**:
   - **Before**: 27% code coverage, mixed test patterns, no E2E
   - **After**: ~100% combined coverage, consistent patterns, full defense-in-depth

4. **Timeline**: 2 days for integration tests + 2 days for E2E (4 days total)

5. **Compliance**: Follows NT Test Plan v1.3.0 template exactly

---

**Status**: ✅ Ready for team review
**Document**: `holmesgpt-api/tests/TEST_PLAN_HAPI_INTEGRATION_V1_0.md`
**Next Action**: Team reviews test plan → approves → Day 1 implementation begins



