# HAPI Integration Test Plan - Ready for Review

**Date**: December 24, 2025
**Status**: ✅ COMPLETE - Awaiting Team Review and Approval
**Next Step**: Team reviews test plan → approves → Day 1 implementation begins

---

## 🎯 **Executive Summary**

Created comprehensive HAPI integration test plan following authoritative NT Test Plan v1.3.0 template. This plan:
1. ✅ Identifies 15 NEW integration tests to create (business logic + real services)
2. ✅ Moves 18 HTTP-based tests from integration to E2E tier (correct classification)
3. ✅ Establishes defense-in-depth testing strategy (unit → integration → E2E)
4. ✅ Provides 4-day implementation timeline with clear milestones

---

## 📋 **What Was Delivered**

### 1. Comprehensive Test Plan Document

**Location**: `holmesgpt-api/tests/TEST_PLAN_HAPI_INTEGRATION_V1_0.md`

**Sections** (following NT template):
- ✅ Header & Metadata (version, status, cross-references)
- ✅ Changelog (v1.0.0)
- ✅ Testing Scope (architecture diagram, in/out of scope)
- ✅ Defense-in-Depth Summary (BR coverage + code coverage strategy)
- ✅ Tier 1: Unit Tests (569 tests - no changes needed)
- ✅ Tier 2: Integration Tests (35 existing + 15 NEW to create)
- ✅ Tier 3: E2E Tests (18 moved from integration)
- ✅ Current Test Status (pre-implementation baseline)
- ✅ Pre/Post Comparison (value proposition)
- ✅ Infrastructure Setup (podman-compose for integration, Kind for E2E)
- ✅ Test Outcomes by Tier (BR mapping)
- ✅ Execution Commands (make targets and pytest)
- ✅ Day-by-Day Timeline (4 days total)
- ✅ Success Criteria (measurable outcomes)
- ✅ Compliance Sign-Off (approval checklist)

### 2. 18 Tests Moved from Integration to E2E

**From**: `holmesgpt-api/tests/integration/`
**To**: `test/e2e/aianalysis/hapi/`

| File | Tests | Status |
|------|-------|--------|
| `test_custom_labels_e2e.py` | 5 | ✅ Moved |
| `test_mock_llm_mode_e2e.py` | 13 | ✅ Moved |
| `test_recovery_dd003_e2e.py` | 0 | ✅ Moved |

**Rationale**: These tests make HTTP calls to HAPI service → E2E tests, not integration tests

### 3. E2E Test Documentation

**Location**: `test/e2e/aianalysis/hapi/README.md`

**Contents**:
- ✅ Purpose and defense-in-depth layer
- ✅ Test file descriptions (business outcomes)
- ✅ Infrastructure requirements (Dockerfile, K8s manifests, Kind)
- ✅ Current status (infrastructure setup pending)
- ✅ Test execution pattern (integration vs E2E)
- ✅ Next steps (separate session)

### 4. Handoff Documentation

**Location**: `docs/handoff/`

**Files**:
- ✅ `HAPI_INTEGRATION_TEST_PLAN_V1_0_DEC_24_2025.md` - Executive summary
- ✅ `HAPI_E2E_TESTS_MOVED_DEC_24_2025.md` - Test move details
- ✅ `HAPI_TEST_PLAN_READY_FOR_REVIEW_DEC_24_2025.md` - This document

---

## 📊 **Test Plan Highlights**

### Current State → Target State

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| **Integration Tests** | 35 + 18 (blocked) | **50** (35 + 15 NEW) | ✅ Clear pattern |
| **E2E Tests** | 0 | **18** (moved + containerized) | ✅ Black-box testing |
| **Code Coverage** | ~27% | **~100%** (combined tiers) | ✅ Defense-in-depth |
| **Test Pattern** | Mixed | **100% consistent** | ✅ Follow DS example |

### 15 NEW Integration Tests (Business Outcomes)

| Category | Tests | Focus |
|----------|-------|-------|
| Workflow Search Logic | 5 | SearchWorkflowCatalogTool with real DS |
| LLM Prompt Building | 3 | PromptBuilder with real workflow fetching |
| Audit Event Logic | 4 | Audit client with real persistence |
| LLM Response Parsing | 3 | Parser with real workflow validation |

**Pattern**: All tests call Python business functions directly (bypass FastAPI), use real Data Storage/PostgreSQL/Redis.

### Defense-in-Depth Strategy

| Tier | Tests | BR Coverage | Code Coverage | Pattern |
|------|-------|-------------|---------------|---------|
| **Unit** | 569 | 70%+ | ~27% | Mocked external deps |
| **Integration** | 50 | >50% | **50%+** | Direct functions + real services |
| **E2E** | 18 | <10% | **50%+** | HTTP API + containerized system |

**Combined**: ~100% code coverage with 3 layers of defense

---

## 📅 **Implementation Timeline**

### Phase 1: NEW Integration Tests (Days 1-2)

**Day 1 (4 hours)**: Workflow Search + LLM Prompt
- Create 5 workflow search tests
- Create 3 LLM prompt building tests
- **Target**: 8 tests passing

**Day 2 (4 hours)**: Audit + Response Parsing
- Create 4 audit event tests
- Create 3 LLM response parsing tests
- **Target**: 15 tests passing

**Deliverable**: 50 integration tests passing (35 existing + 15 NEW)

### Phase 2: E2E Infrastructure (Days 3-4 - Separate Session)

**Day 3 (4 hours)**: E2E Infrastructure
- Create HAPI Dockerfile
- Create K8s manifests (deployment, service, configmap)
- Add to Kind deployment automation

**Day 4 (4 hours)**: Python E2E Test Runner
- Configure pytest for E2E tests
- Add to `make test-e2e-aianalysis` target
- **Target**: 18 E2E tests passing in Kind

**Deliverable**: 18 E2E tests passing in Kind

---

## 🎯 **Next Steps**

### Step 1: Team Review (Now)

**Reviewers**:
- [ ] HAPI Team Lead - Validate 15 NEW test scenarios
- [ ] Quality Assurance - Confirm defense-in-depth strategy
- [ ] Architecture Review - Approve test plan structure

**Review Questions**:
1. Do the 15 NEW test scenarios cover business outcomes adequately?
2. Is the timeline realistic (2 days integration + 2 days E2E)?
3. Does the defense-in-depth strategy align with project standards?
4. Are there any missing test categories?

### Step 2: Approval (After Review)

**Sign-Off**: Update compliance section in test plan document
- [ ] HAPI Team Lead: Approved / Changes Requested
- [ ] Quality Assurance: Approved / Changes Requested
- [ ] Architecture Review: Approved / Changes Requested

### Step 3: Day 1 Implementation (After Approval)

**Morning (2 hours)**:
```bash
# Create workflow search tests
cd holmesgpt-api
touch tests/integration/test_workflow_search_business_logic.py

# Implement 5 tests (IT-HAPI-250-01 through IT-HAPI-250-05)
# Verify: pytest tests/integration/test_workflow_search_business_logic.py -v
```

**Afternoon (2 hours)**:
```bash
# Create LLM prompt building tests
touch tests/integration/test_llm_prompt_business_logic.py

# Implement 3 tests (IT-AI-001-01 through IT-AI-001-03)
# Verify: pytest tests/integration/test_llm_prompt_business_logic.py -v
```

**End of Day 1**: 8 tests passing

### Step 4: Continue Timeline (Days 2-4)

Follow test plan Day 2 → Day 3 → Day 4

---

## 📚 **Key Documents**

### Test Plan (Main Document)
📄 **`holmesgpt-api/tests/TEST_PLAN_HAPI_INTEGRATION_V1_0.md`**
- Complete test plan with all sections
- 15 NEW test specifications (with business outcomes)
- Day-by-day implementation timeline
- Success criteria and sign-off checklist

### E2E Test Documentation
📄 **`test/e2e/aianalysis/hapi/README.md`**
- E2E test purpose and pattern
- Infrastructure requirements (Dockerfile, K8s manifests)
- Next steps for separate session

### Handoff Documents
📄 **`docs/handoff/HAPI_INTEGRATION_TEST_PLAN_V1_0_DEC_24_2025.md`**
- Executive summary of test plan
- Current state → target state comparison
- 15 NEW test categories with examples

📄 **`docs/handoff/HAPI_E2E_TESTS_MOVED_DEC_24_2025.md`**
- Why 18 tests were moved from integration to E2E
- Test pattern differences (direct function calls vs HTTP)
- Infrastructure setup next steps

---

## ✅ **Success Criteria for Review**

### Test Plan Quality
- [x] **Follows NT template v1.3.0** (all mandatory sections present)
- [x] **15 NEW tests map to BR-*** (business outcome focused)
- [x] **Timeline realistic** (4 days with clear milestones)
- [x] **Defense-in-depth complete** (3-tier strategy)
- [x] **Infrastructure documented** (podman-compose + Kind)

### Test Pattern Consistency
- [x] **Integration tests** call Python functions directly (35 existing + 15 NEW)
- [x] **E2E tests** use HTTP to containerized HAPI (18 moved)
- [x] **Follows Data Storage example** (repository tests vs HTTP API tests)

### Documentation Completeness
- [x] **Test plan document** (comprehensive, ready for implementation)
- [x] **E2E README** (infrastructure setup guidance)
- [x] **Handoff documents** (executive summaries)
- [x] **Cross-references** (NT template, DS examples, DD-TEST-002)

---

## 🎯 **Key Takeaways**

1. **Pattern Discovery**: HAPI's current "integration" tests are split:
   - ✅ **35 tests**: Direct function calls (CORRECT)
   - 🔴 **18 tests**: HTTP calls (ACTUALLY E2E)

2. **Solution**: Follow Data Storage pattern:
   - **Integration**: Test business logic directly with real services
   - **E2E**: Test containerized service via HTTP (black-box)

3. **Value Proposition**:
   - **Before**: 27% code coverage, mixed patterns, no E2E
   - **After**: ~100% combined coverage, consistent patterns, full defense-in-depth

4. **Timeline**: 2 days integration + 2 days E2E = 4 days total

5. **Next Action**: Team reviews test plan → approves → Day 1 implementation begins

---

**Status**: ✅ Test plan ready for team review
**Document**: `holmesgpt-api/tests/TEST_PLAN_HAPI_INTEGRATION_V1_0.md`
**Next Action**: Team review → approval → Day 1 implementation
**Timeline**: 4 days (2 days integration + 2 days E2E in separate session)



