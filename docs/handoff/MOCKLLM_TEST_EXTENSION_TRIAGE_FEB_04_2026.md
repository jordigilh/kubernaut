# MockLLM Test Extension Triage - HAPI & AIAnalysis Integration

**Date**: February 4, 2026  
**Status**: 📋 Analysis & Recommendation  
**Confidence Threshold**: ≥90% for recommendations  
**Scope**: HAPI E2E + AA Integration test scenarios  

---

## 🎯 Executive Summary

### Current State

**MockLLM**: ✅ Operational (17 scenarios implemented)  
**HAPI E2E**: ✅ 40/43 tests passing (100%), ~2,700 lines  
**AA Integration**: ✅ 84 tests passing (100%), ~5,600 lines  
**Total Test Coverage**: 750+ tests across unit/integration/E2E

### Objective

Identify high-confidence (≥90%) test scenarios that:
1. Leverage MockLLM for realistic business scenarios
2. Ensure HAPI-AA integration works correctly
3. Cover untested/undertested Business Requirements
4. Add significant value without duplication

---

## 📊 Current Coverage Analysis

### MockLLM Scenarios (17 Implemented)

| Category | Scenario | Signal Type | Confidence | Used By |
|----------|----------|-------------|------------|---------|
| **Happy Path** | oomkilled | OOMKilled | 0.95 | HAPI E2E, AA INT |
| | crashloop | CrashLoopBackOff | 0.90 | HAPI E2E, AA INT |
| | node_not_ready | NodeNotReady | 0.88 | HAPI E2E |
| **Recovery** | recovery | OOMKilled (recovery) | 0.85 | HAPI E2E (recovery) |
| | recovery_basic | OOMKilled (recovery) | 0.85 | Future use |
| **Edge Cases** | no_workflow_found | MOCK_NO_WORKFLOW_FOUND | 0.0 | HAPI E2E (3 tests) |
| | low_confidence | MOCK_LOW_CONFIDENCE | 0.35 | HAPI E2E, AA INT |
| | problem_resolved | MOCK_PROBLEM_RESOLVED | 0.85 | HAPI E2E |
| | max_retries_exhausted | MOCK_MAX_RETRIES_EXHAUSTED | 0.0 | HAPI E2E |
| | rca_incomplete | MOCK_RCA_INCOMPLETE | 0.88 | HAPI E2E |
| **Advanced** | multi_step_recovery | InsufficientResources | 0.85 | ⏸️ Pending workflows |
| | cascading_failure | CrashLoopBackOff | 0.75 | ⏸️ Pending workflows |
| | near_attempt_limit | DatabaseConnectionError | 0.90 | ⏸️ Pending workflows |
| | noisy_neighbor | HighDatabaseLatency | 0.80 | ⏸️ Pending workflows |
| | network_partition | NodeUnreachable | 0.70 | ⏸️ Pending workflows |
| **Test Infra** | test_signal | TestSignal | 0.90 | AA INT (shutdown tests) |
| | default | Unknown | 0.75 | Fallback |

**Total**: 17 scenarios (11 active, 6 pending workflow seeding)

---

### HAPI E2E Test Coverage (40/43 tests)

#### Category Breakdown

| Category | Tests | Coverage | Gaps |
|----------|-------|----------|------|
| **Incident Analysis** (BR-HAPI-197, BR-HAPI-002, BR-AI-075, BR-HAPI-200) | 9/9 | ✅ 100% | None |
| **Recovery Analysis** (BR-AI-080, BR-AI-081, BR-HAPI-197, BR-HAPI-212) | 18/18 | ✅ 100% | None |
| **Workflow Catalog** (BR-STORAGE-013, BR-AI-075) | 13/13 | ✅ 100% | None |
| **Infrastructure** | 3 (skipped) | N/A | Intentional |

#### Tests by Business Requirement

**Fully Covered**:
- ✅ BR-HAPI-197: Human review scenarios (E2E-HAPI-001, 002, 003)
- ✅ BR-HAPI-002: Happy path scenarios (E2E-HAPI-004)
- ✅ BR-AI-075: Response structure validation (E2E-HAPI-005, 006)
- ✅ BR-HAPI-200: Error handling (E2E-HAPI-007, 008)
- ✅ BR-AI-080/081: Recovery scenarios (E2E-HAPI-013-029)
- ✅ BR-HAPI-212: Recovery endpoint special cases (E2E-HAPI-023-025)
- ✅ BR-STORAGE-013: Workflow catalog (E2E-HAPI-030-042)

---

### AIAnalysis Integration Test Coverage (84 tests)

#### Test Files & Focus Areas

| Test File | Tests | Focus | MockLLM Usage |
|-----------|-------|-------|---------------|
| **holmesgpt_integration_test.go** | ~20 | HAPI response parsing | ✅ Full |
| **audit_flow_integration_test.go** | ~15 | Audit event generation | ✅ Full |
| **audit_provider_data_integration_test.go** | ~12 | Provider data capture | ✅ Full |
| **recovery_integration_test.go** | ~10 | Recovery flow | ✅ Full |
| **recovery_human_review_integration_test.go** | ~8 | Recovery human review | ✅ Full |
| **error_handling_integration_test.go** | ~6 | Error scenarios | ⚠️ Partial |
| **rego_integration_test.go** | ~5 | Rego policy evaluation | ❌ None |
| **graceful_shutdown_test.go** | ~4 | Shutdown handling | ✅ Full |
| **reconciliation_test.go** | ~2 | Basic reconciliation | ✅ Full |
| **metrics_integration_test.go** | ~2 | Metrics emission | ❌ None |

#### Business Requirements Coverage

**Fully Covered**:
- ✅ BR-AI-001: HolmesGPT integration & investigation
- ✅ BR-AI-003: Confidence scoring
- ✅ BR-AI-007: Remediation recommendations
- ✅ BR-AI-012: Root cause analysis
- ✅ BR-AI-075: Workflow selection output format
- ✅ BR-AI-080/081: Recovery tracking & failure context

**Partially Covered**:
- ⚠️ BR-AI-028: Rego approval policy (no MockLLM scenarios)
- ⚠️ BR-AI-029: Rego policy evaluation (no LLM integration tests)
- ⚠️ BR-AI-076: Approval context population (basic coverage)

**Gaps Identified**:
- ❌ BR-AI-082: Historical context for learning (deferred to v2.0)
- ❌ BR-AI-083: Recovery investigation flow end-to-end validation

---

## 🔍 Gap Analysis - High Confidence Opportunities (≥90%)

### Gap Category 1: **HAPI-AA Integration Scenarios**

#### GAP-001: **Alternative Workflows End-to-End Flow** ✅ **Confidence: 95%**

**Description**: Validate that `alternative_workflows` from HAPI correctly populate AIAnalysis `approvalContext` for operator decision-making.

**Business Requirements**: BR-AI-076 (Approval Context), BR-AUDIT-005 Gap #4 (Alternative Workflows)

**Current Coverage**:
- ✅ HAPI E2E-002: Validates HAPI returns `alternative_workflows`
- ✅ AA INT `audit_provider_data`: Validates `alternative_workflows` in audit events
- ❌ **Missing**: End-to-end flow showing alternatives → approvalContext → operator notification

**Recommended Test**: **IT-AA-085: Alternative Workflows Populate Approval Context**

**Test Scenario**:
```go
Context("BR-AI-076: Alternative workflows in approval context", func() {
    It("IT-AA-085: Should populate approval context with alternatives from HAPI", func() {
        // ARRANGE: Signal with low confidence (triggers alternatives)
        signal := &kubernautv1.Signal{
            Spec: kubernautv1.SignalSpec{
                SignalType: "MOCK_LOW_CONFIDENCE", // Mock LLM returns alternatives
                Severity:   "high",
                // ... other fields
            },
        }
        
        // ACT: Reconcile AIAnalysis
        // HAPI returns:
        // - confidence: 0.35 (low, <0.8)
        // - selected_workflow: generic-restart-v1
        // - alternative_workflows: [alt1, alt2]
        result := reconcileAIAnalysis(signal)
        
        // ASSERT: Approval context populated with alternatives
        Expect(result.Status.ApprovalRequired).To(BeTrue())
        Expect(result.Status.ApprovalContext).ToNot(BeNil())
        Expect(result.Status.ApprovalContext.AlternativesConsidered).ToNot(BeEmpty())
        Expect(len(result.Status.ApprovalContext.AlternativesConsidered)).To(Equal(2))
        
        // BUSINESS IMPACT: Operator sees alternatives in approval UI
        // Reduces approval time by providing context
    })
})
```

**MockLLM Support**: ✅ Already exists (`low_confidence` scenario)

**Implementation Effort**: **Low** (2-3 hours)
- Add `approvalContext.AlternativesConsidered` population in `analyzing.go`
- Add test case to AA integration suite
- Validate with existing HAPI E2E test

---

#### GAP-002: **Human Review Reason Propagation** ✅ **Confidence: 93%**

**Description**: Validate that HAPI `human_review_reason` correctly triggers AA approval routing with proper reason codes.

**Business Requirements**: BR-HAPI-200 (Structured Human Review Reasons), BR-AI-028 (Auto-Approve or Flag for Manual Review)

**Current Coverage**:
- ✅ HAPI E2E-001/003: Validates HAPI returns `human_review_reason`
- ✅ AA INT: Basic approval routing tests
- ❌ **Missing**: Validation of reason code mapping (HAPI enum → AA status)

**Recommended Test**: **IT-AA-086: Human Review Reason Code Mapping**

**Test Scenario**:
```go
Context("BR-HAPI-200, BR-AI-028: Human review reason propagation", func() {
    It("IT-AA-086: Maps HAPI human_review_reason to AA approval status", func() {
        testCases := []struct {
            scenario       string
            signalType     string
            expectedReason string
            expectedFlag   bool
        }{
            {
                scenario:       "no_workflow_found",
                signalType:     "MOCK_NO_WORKFLOW_FOUND",
                expectedReason: "no_matching_workflows",
                expectedFlag:   true,
            },
            {
                scenario:       "low_confidence",
                signalType:     "MOCK_LOW_CONFIDENCE",
                expectedReason: "low_confidence",
                expectedFlag:   true,
            },
            {
                scenario:       "llm_parsing_error",
                signalType:     "MOCK_MAX_RETRIES_EXHAUSTED",
                expectedReason: "llm_parsing_error",
                expectedFlag:   true,
            },
        }
        
        for _, tc := range testCases {
            By(fmt.Sprintf("Testing %s scenario", tc.scenario))
            
            // ACT: Reconcile AIAnalysis with scenario signal
            result := reconcileAIAnalysis(createSignal(tc.signalType))
            
            // ASSERT: Reason code correctly mapped
            Expect(result.Status.ApprovalRequired).To(Equal(tc.expectedFlag))
            Expect(result.Status.ApprovalReason).To(ContainSubstring(tc.expectedReason))
        }
    })
})
```

**MockLLM Support**: ✅ All scenarios exist

**Implementation Effort**: **Low** (2-3 hours)

---

#### GAP-003: **Recovery Attempt Number Validation** ✅ **Confidence: 92%**

**Description**: Validate that recovery attempt tracking works correctly across multiple failures (BR-AI-080: Track Previous Execution Attempts).

**Business Requirements**: BR-AI-080 (Track Previous Execution Attempts), BR-AI-081 (Pass Failure Context to LLM)

**Current Coverage**:
- ✅ HAPI E2E-013-029: Recovery scenarios
- ✅ AA INT `recovery_integration_test.go`: Basic recovery flow
- ❌ **Missing**: Multi-attempt recovery sequence validation (attempt 1 → fail → attempt 2 → fail → attempt 3)

**Recommended Test**: **IT-AA-087: Multi-Attempt Recovery Tracking**

**Test Scenario**:
```go
Context("BR-AI-080, BR-AI-081: Recovery attempt tracking", func() {
    It("IT-AA-087: Tracks recovery attempts across multiple failures", func() {
        // ARRANGE: Simulate 3 consecutive recovery attempts
        attempts := []struct {
            attemptNum      int
            failedWorkflow  string
            expectedStrategy string
        }{
            {1, "workflow-123-initial", "memory-increase-v1"},
            {2, "memory-increase-v1", "memory-optimize-v1"},
            {3, "memory-optimize-v1", "rollback-deployment-v1"},
        }
        
        for _, attempt := range attempts {
            By(fmt.Sprintf("Recovery attempt %d", attempt.attemptNum))
            
            // ACT: Create recovery AIAnalysis with previous failure
            result := reconcileRecoveryAIAnalysis(createRecoverySignal(
                attemptNum: attempt.attemptNum,
                failedWorkflow: attempt.failedWorkflow,
            ))
            
            // ASSERT: Correct attempt number and strategy
            Expect(result.Status.RecoveryAttemptNumber).To(Equal(attempt.attemptNum))
            Expect(result.Status.SelectedWorkflow.WorkflowID).To(ContainSubstring(attempt.expectedStrategy))
            
            // BUSINESS IMPACT: System learns from failures and adjusts strategy
        }
    })
})
```

**MockLLM Support**: ✅ `recovery` scenario exists, can be extended

**Implementation Effort**: **Medium** (4-6 hours)
- Requires state management across test iterations
- May need new MockLLM scenario for attempt tracking

---

### Gap Category 2: **Rego Policy Integration with LLM Scenarios**

#### GAP-004: **Rego Policy with Confidence Thresholds** ✅ **Confidence: 94%**

**Description**: Validate Rego approval policies work correctly with varying confidence scores from MockLLM.

**Business Requirements**: BR-AI-028 (Auto-Approve or Flag for Manual Review), BR-AI-029 (Rego Policy Evaluation)

**Current Coverage**:
- ✅ AA Unit: `rego_evaluator_test.go` (26 tests, no LLM)
- ✅ AA INT: `rego_integration_test.go` (5 tests, no MockLLM)
- ❌ **Missing**: Rego policy evaluation with real HAPI responses

**Recommended Test**: **IT-AA-088: Rego Policy with MockLLM Confidence Scores**

**Test Scenario**:
```go
Context("BR-AI-028, BR-AI-029: Rego policy with LLM confidence", func() {
    It("IT-AA-088: Evaluates Rego policy with MockLLM confidence scores", func() {
        testCases := []struct {
            signalType        string
            expectedConfidence float64
            expectedApproval  bool
            policy            string
        }{
            {
                signalType:        "OOMKilled", // Mock: 0.95
                expectedConfidence: 0.95,
                expectedApproval:  false, // Auto-approve (high confidence)
                policy:            "default",
            },
            {
                signalType:        "MOCK_LOW_CONFIDENCE", // Mock: 0.35
                expectedConfidence: 0.35,
                expectedApproval:  true, // Require approval (low confidence)
                policy:            "default",
            },
            {
                signalType:        "CrashLoopBackOff", // Mock: 0.90, but production env
                expectedConfidence: 0.90,
                expectedApproval:  true, // Require approval (production safety)
                policy:            "production_safety",
            },
        }
        
        for _, tc := range testCases {
            By(fmt.Sprintf("Testing %s with %s policy", tc.signalType, tc.policy))
            
            // ARRANGE: Load Rego policy
            loadRegoPolicy(tc.policy)
            
            // ACT: Reconcile AIAnalysis (HAPI returns MockLLM confidence)
            result := reconcileAIAnalysis(createSignal(tc.signalType))
            
            // ASSERT: Rego policy correctly evaluates confidence
            Expect(result.Status.SelectedWorkflow.Confidence).To(BeNumerically("~", tc.expectedConfidence, 0.01))
            Expect(result.Status.ApprovalRequired).To(Equal(tc.expectedApproval))
        }
    })
})
```

**MockLLM Support**: ✅ Multiple confidence scenarios exist

**Implementation Effort**: **Medium** (4-6 hours)
- Requires Rego policy test fixtures
- Integration of AA rego evaluator with real HAPI calls

---

### Gap Category 3: **Error Handling & Resilience**

#### GAP-005: **HAPI Timeout Handling** ✅ **Confidence: 91%**

**Description**: Validate AA handles HAPI timeout gracefully (BR-AI-001: HolmesGPT Integration).

**Current Coverage**:
- ✅ AA INT: `error_handling_integration_test.go` (6 tests, basic errors)
- ❌ **Missing**: HAPI timeout simulation with MockLLM

**Recommended Test**: **IT-AA-089: HAPI Timeout Graceful Degradation**

**Test Scenario**:
```go
Context("BR-AI-001: HAPI timeout handling", func() {
    It("IT-AA-089: Gracefully handles HAPI timeout", func() {
        // ARRANGE: Configure Mock LLM with artificial delay
        // (Mock LLM could support slow_response scenario: 90s delay)
        
        // ACT: Reconcile AIAnalysis with 60s HAPI timeout
        result, err := reconcileAIAnalysisWithTimeout(
            createSignal("OOMKilled"),
            timeout: 60*time.Second,
        )
        
        // ASSERT: Timeout handled gracefully
        Expect(err).ToNot(HaveOccurred()) // No panic
        Expect(result.Status.Phase).To(Equal("Error"))
        Expect(result.Status.ErrorMessage).To(ContainSubstring("timeout"))
        
        // BUSINESS IMPACT: System remains stable during LLM provider outages
    })
})
```

**MockLLM Support**: ⚠️ **Needs new scenario** (`slow_response`)

**Implementation Effort**: **Medium** (5-7 hours)
- Requires MockLLM enhancement for delay simulation
- AA timeout configuration changes

---

#### GAP-006: **HAPI HTTP 500 Error Handling** ✅ **Confidence: 90%**

**Description**: Validate AA handles HAPI HTTP 500 errors with retry logic (BR-AI-001).

**Current Coverage**:
- ✅ AA INT: `error_handling_integration_test.go` (basic errors)
- ❌ **Missing**: HTTP 500 simulation

**Recommended Test**: **IT-AA-090: HAPI HTTP 500 Error Retry**

**Test Scenario**:
```go
Context("BR-AI-001: HAPI HTTP 500 error handling", func() {
    It("IT-AA-090: Retries HAPI call on HTTP 500", func() {
        // ARRANGE: Configure Mock LLM to return HTTP 500 twice, then 200
        // (Mock LLM could support failure_then_success scenario)
        
        // ACT: Reconcile AIAnalysis (AA should retry)
        result := reconcileAIAnalysis(createSignal("OOMKilled"))
        
        // ASSERT: Eventually succeeds after retries
        Expect(result.Status.Phase).To(Equal("Completed"))
        Expect(result.Status.SelectedWorkflow).ToNot(BeNil())
        
        // BUSINESS IMPACT: Resilience to transient LLM provider failures
    })
})
```

**MockLLM Support**: ⚠️ **Needs new scenario** (`failure_then_success`)

**Implementation Effort**: **Medium** (5-7 hours)

---

### Gap Category 4: **Metrics & Observability**

#### GAP-007: **Metrics Validation with MockLLM** ✅ **Confidence: 92%**

**Description**: Validate AA metrics correctly track HAPI investigation outcomes (BR-AI-011: Investigation Metrics).

**Current Coverage**:
- ✅ AA INT: `metrics_integration_test.go` (2 tests, basic)
- ❌ **Missing**: Metrics validation with real HAPI responses

**Recommended Test**: **IT-AA-091: Metrics Track HAPI Investigation Outcomes**

**Test Scenario**:
```go
Context("BR-AI-011: Investigation metrics", func() {
    It("IT-AA-091: Metrics track HAPI investigation outcomes", func() {
        testCases := []struct {
            signalType        string
            expectedOutcome   string
            expectedConfidence string
        }{
            {"OOMKilled", "success", "high"}, // confidence >= 0.8
            {"MOCK_LOW_CONFIDENCE", "needs_review", "low"}, // confidence < 0.5
            {"MOCK_NO_WORKFLOW_FOUND", "needs_review", "none"}, // confidence = 0
        }
        
        for _, tc := range testCases {
            // ACT: Reconcile AIAnalysis
            reconcileAIAnalysis(createSignal(tc.signalType))
            
            // ASSERT: Metrics correctly incremented
            metrics := getPrometheusMetrics()
            Expect(metrics["aianalysis_investigations_total"]).To(HaveKeyWithValue(
                map[string]string{"status": tc.expectedOutcome, "confidence": tc.expectedConfidence},
                BeNumerically(">", 0),
            ))
        }
    })
})
```

**MockLLM Support**: ✅ All scenarios exist

**Implementation Effort**: **Low** (2-3 hours)

---

## 📊 Recommendations Summary

### High Confidence (≥90%) Recommendations

| Gap ID | Test ID | Description | Confidence | Effort | MockLLM Support | Priority |
|--------|---------|-------------|------------|--------|-----------------|----------|
| **GAP-001** | IT-AA-085 | Alternative workflows → approval context | 95% | Low (2-3h) | ✅ Exists | **P0** |
| **GAP-002** | IT-AA-086 | Human review reason code mapping | 93% | Low (2-3h) | ✅ Exists | **P0** |
| **GAP-003** | IT-AA-087 | Multi-attempt recovery tracking | 92% | Medium (4-6h) | ⚠️ Needs extension | **P1** |
| **GAP-004** | IT-AA-088 | Rego policy with LLM confidence | 94% | Medium (4-6h) | ✅ Exists | **P0** |
| **GAP-007** | IT-AA-091 | Metrics track HAPI outcomes | 92% | Low (2-3h) | ✅ Exists | **P1** |
| **GAP-005** | IT-AA-089 | HAPI timeout handling | 91% | Medium (5-7h) | ❌ New scenario | **P2** |
| **GAP-006** | IT-AA-090 | HAPI HTTP 500 error retry | 90% | Medium (5-7h) | ❌ New scenario | **P2** |

**Total Recommended Tests**: 7 tests (5 P0/P1, 2 P2)  
**Total Effort**: ~27-40 hours (1 sprint)

---

### MockLLM Scenarios to Add (For Medium-Priority Gaps)

| Scenario Name | Signal Type | Purpose | Effort |
|---------------|-------------|---------|--------|
| `slow_response` | Any | Simulate HAPI timeout (90s delay) | 2h |
| `failure_then_success` | Any | Simulate transient HTTP 500 → 200 | 3h |
| `multi_attempt_tracking` | OOMKilled | Recovery attempt 1 → 2 → 3 | 4h |

**Total MockLLM Effort**: ~9 hours

---

## 🎯 Implementation Plan

### Phase 1: High-Impact, Low-Effort (P0 Priority)

**Tests**: GAP-001, GAP-002, GAP-004  
**Effort**: ~10-15 hours  
**Business Value**: Validates critical HAPI-AA integration contracts

**Deliverables**:
1. ✅ IT-AA-085: Alternative workflows → approval context
2. ✅ IT-AA-086: Human review reason code mapping
3. ✅ IT-AA-088: Rego policy with LLM confidence

---

### Phase 2: Medium-Impact, Medium-Effort (P1 Priority)

**Tests**: GAP-003, GAP-007  
**Effort**: ~6-9 hours  
**Business Value**: Validates recovery tracking and observability

**Deliverables**:
1. ✅ IT-AA-087: Multi-attempt recovery tracking (requires MockLLM extension)
2. ✅ IT-AA-091: Metrics track HAPI outcomes

---

### Phase 3: Resilience & Error Handling (P2 Priority)

**Tests**: GAP-005, GAP-006  
**Effort**: ~10-14 hours  
**Business Value**: Validates system resilience to HAPI failures

**Deliverables**:
1. ✅ IT-AA-089: HAPI timeout handling (requires new MockLLM scenario)
2. ✅ IT-AA-090: HAPI HTTP 500 error retry (requires new MockLLM scenario)
3. ✅ MockLLM scenarios: `slow_response`, `failure_then_success`

---

## 📈 Expected Impact

### Test Coverage Improvement

**Current**:
- HAPI E2E: 40/43 tests (93%)
- AA Integration: 84 tests

**After Implementation**:
- HAPI E2E: 40/43 tests (unchanged)
- AA Integration: **91 tests** (+7 tests, +8.3%)
- **Total**: 757+ tests (+7)

### Business Requirements Coverage

**Before**:
- BR-AI-076 (Approval Context): ⚠️ Partial
- BR-AI-028/029 (Rego Policy): ⚠️ No LLM integration
- BR-AI-080/081 (Recovery): ⚠️ Single-attempt only

**After**:
- BR-AI-076: ✅ **Complete** (GAP-001)
- BR-AI-028/029: ✅ **Complete** (GAP-004)
- BR-AI-080/081: ✅ **Complete** (GAP-003)
- BR-HAPI-200: ✅ **Validated end-to-end** (GAP-002)

### Confidence Level

**Overall Triage Confidence**: **93%** (weighted average)

**Reasoning**:
- ✅ All gaps backed by existing BRs
- ✅ 5/7 tests use existing MockLLM scenarios (no new dependencies)
- ✅ Clear acceptance criteria and business impact
- ✅ Builds on 100% passing test suites

---

## ❌ **Gaps NOT Recommended** (<90% Confidence)

### Deferred Recommendations

| Gap Description | Confidence | Reason |
|-----------------|------------|--------|
| BR-AI-002: Multiple analysis types | 60% | Feature deferred to v2.0 per DD-AIANALYSIS-005 |
| BR-AI-082: Historical context for learning | 50% | No current implementation or BRs |
| Post-execution analysis tests | 40% | Endpoint not exposed per DD-017 |
| Multi-cluster scenarios | 30% | Out of scope for v1.0 |
| Performance/load testing with MockLLM | 70% | Requires infrastructure changes |

---

## 🔍 Alternative: Extend HAPI E2E Instead?

**Question**: Should we extend HAPI E2E tests instead of AA integration tests?

**Analysis**:
- HAPI E2E: **40/43 tests (93%)** - already excellent coverage
- AA Integration: **84 tests** - good coverage, but missing HAPI-AA integration validation
- **Recommendation**: Focus on **AA Integration** to validate end-to-end contracts

**Rationale**:
1. HAPI E2E validates HAPI service correctness ✅
2. AA Integration validates HAPI-AA contract ⚠️ (gaps identified)
3. MockLLM already supports both test suites ✅
4. Business value: Ensuring integration works correctly is critical for production

---

## ✅ Acceptance Criteria for Recommendations

### For Each New Test

**Must Have**:
1. ✅ Maps to specific Business Requirement(s)
2. ✅ Uses existing or easily-addable MockLLM scenario
3. ✅ Has clear acceptance criteria
4. ✅ Validates end-to-end behavior (not just unit logic)
5. ✅ Includes business impact statement

**Quality Gates**:
1. ✅ Test passes on first run (no flakiness)
2. ✅ Test execution time < 10 seconds
3. ✅ Clear failure messages
4. ✅ No infrastructure changes required

---

## 📚 References

### Business Requirements

**HAPI**:
- BR-HAPI-197: Human review scenarios
- BR-HAPI-200: Structured human review reasons
- BR-HAPI-212: Recovery endpoint special cases

**AIAnalysis**:
- BR-AI-001: HolmesGPT integration
- BR-AI-028: Auto-approve or flag for manual review
- BR-AI-029: Rego policy evaluation
- BR-AI-076: Approval context for low confidence
- BR-AI-080: Track previous execution attempts
- BR-AI-081: Pass failure context to LLM

### Design Documents

- DD-AIANALYSIS-005: Multiple Analysis Types Deferral
- ADR-045: AIAnalysis ↔ HolmesGPT-API Contract
- ADR-018: Approval Notification Integration
- DD-AUTH-014: ServiceAccount Token Authentication

### Existing Test Suites

- `test/e2e/holmesgpt-api/` (HAPI E2E)
- `test/integration/aianalysis/` (AA Integration)
- `test/services/mock-llm/src/server.py` (MockLLM scenarios)

---

## 🎯 Next Steps

### Immediate Actions (Phase 1)

1. ✅ **User Review & Approval**: Validate triage findings
2. ⏳ **Implement GAP-001** (IT-AA-085): Alternative workflows test
3. ⏳ **Implement GAP-002** (IT-AA-086): Human review reason test
4. ⏳ **Implement GAP-004** (IT-AA-088): Rego policy + LLM test
5. ⏳ **Validate**: Run full AA integration suite (91 tests expected)

### Follow-Up (Phase 2-3)

- Phase 2: Implement GAP-003, GAP-007 (P1 priority)
- Phase 3: Implement GAP-005, GAP-006 + new MockLLM scenarios (P2 priority)

---

**Status**: 📋 **Ready for Review**  
**Confidence**: 93% (weighted average across all recommendations)  
**Business Value**: **High** (validates critical HAPI-AA integration contracts)  
**Risk**: **Low** (builds on 100% passing test suites, no breaking changes)

**Recommendation**: **Proceed with Phase 1 implementation** (GAP-001, GAP-002, GAP-004)
