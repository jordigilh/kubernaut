## Testing Strategy

**Version**: v2.3
**Last Updated**: 2025-12-09

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| **v2.4** | 2026-02-05 | **BR-AI-084 Predictive Signal Mode**: Added test scenarios for RO SignalMode copy (UT-RO-106-001/002), AA request builder (UT-AA-084-001/002), HAPI prompt builder (UT-HAPI-084-001/002/003), and integration tests (IT-RO-084-001, IT-HAPI-084-001) |
| **v2.3** | 2025-12-09 | **TESTING_GUIDELINES Compliance Fixes**: Fixed integration coverage target (20% → >50%); Removed BR- prefixes from unit test examples; Updated E2E path to `test/e2e/aianalysis/`; Added integration test file table; Updated HolmesGPT focus to MockHolmesGPTClient per HAPI team |
| **v2.2** | 2025-12-06 | **Day 6 Implementation Alignment**: Updated unit test coverage (70% → 87.6% achieved); Added edge case test patterns for ERROR_HANDLING_PHILOSOPHY.md; Updated test file list; Added confidence level classification tests |
| v2.1 | 2025-12-04 | **TESTING_GUIDELINES Alignment**: Added Quality Gates, Success Metrics; Removed V1.1 deferred code (BR-AI-040, BR-AI-050); Fixed BR range references |
| v2.0 | 2025-11-30 | Added TESTING_GUIDELINES.md reference; Updated port allocation per DD-TEST-001 |
| v1.0 | 2025-10-15 | Initial specification |

---

## References

| Document | Purpose |
|----------|---------|
| [TESTING_GUIDELINES.md](../../../development/business-requirements/TESTING_GUIDELINES.md) | **AUTHORITATIVE** - Testing methodology and standards |
| [.cursor/rules/03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc) | Testing framework rules |
| [DD-TEST-001](../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md) | Port allocation for E2E tests |

### Testing Pyramid

Following Kubernaut's defense-in-depth testing strategy:

| Test Type | Target Coverage | Focus | Confidence |
|-----------|----------------|-------|------------|
| **Unit Tests** | 70%+ | Controller logic, AI analysis phases, Rego policies | 85-90% |
| **Integration Tests** | >50% | CRD interactions, HolmesGPT API integration, approval workflows | 80-85% |
| **E2E Tests** | 10-15% | Complete investigation flow, real cluster scenarios | 90-95% |

**Rationale**: CRD controllers require high integration test coverage (>50%) to validate Kubernetes API interactions, CRD lifecycle management, HolmesGPT API integration, and watch-based coordination patterns that cannot be adequately tested in unit tests alone.

### Unit Tests (Primary Coverage Layer)

**Test Directory**: [test/unit/aianalysis/](../../../test/unit/aianalysis/)
**Coverage Target**: 70%+ → **87.6% Achieved**
**Confidence**: 85-90%
**Execution**: `make test-unit-aianalysis`
**Test Count**: 149 tests (8 test files)

**Testing Strategy**: Use fake K8s client for compile-time API safety. Mock ONLY HolmesGPT HTTP API. Use REAL business logic (Rego policy engine, approval workflow).

**Test Files (Day 6 Complete)**:
| File | Focus | Tests |
|------|-------|-------|
| `controller_test.go` | CRD lifecycle, phase transitions | ~15 |
| `investigating_handler_test.go` | HAPI integration, human review, retry | ~45 |
| `analyzing_handler_test.go` | Rego evaluation, approval context | ~35 |
| `rego_evaluator_test.go` | Policy evaluation, graceful degradation | ~15 |
| `holmesgpt_client_test.go` | HTTP client, error handling | ~10 |
| `metrics_test.go` | Prometheus metrics registration | ~8 |
| `audit_client_test.go` | Audit event generation | ~8 |
| `error_types_test.go` | Error categorization (ERROR_HANDLING_PHILOSOPHY.md) | ~13 |

**Edge Case Coverage (Business Value)**:
| Category | Coverage | Business Value |
|----------|----------|----------------|
| Error Types | 100% | Operators distinguish transient vs permanent failures |
| Confidence Levels | 100% | Quick assessment of AI confidence (high/medium/low) |
| Human Review Mapping | 100% | All 6 HAPI enum values + 11 warning fallbacks |
| Retry Mechanism | 89% | Graceful handling of malformed annotations |
| Validation History | 100% | Timestamp parsing with graceful fallback |

**AI/ML-Specific Test Patterns**:

```go
package aianalysis

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "context"
    "time"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1"
    approvalv1 "github.com/jordigilh/kubernaut/api/approval/v1"
    "github.com/jordigilh/kubernaut/internal/controller"
    "github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
    "github.com/jordigilh/kubernaut/pkg/ai/rego"
    "github.com/jordigilh/kubernaut/pkg/testutil"
    "github.com/jordigilh/kubernaut/pkg/testutil/mocks"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// ✅ CORRECT: Unit tests use function/component names, NOT BR- prefixes
// BR- prefixes belong in E2E/Business Requirement tests only (per TESTING_GUIDELINES.md)
var _ = Describe("AIAnalysis Controller", func() {
    var (
        fakeK8sClient       client.Client
        scheme              *runtime.Scheme
        mockHolmesGPT       *mocks.MockHolmesGPT
        regoEngine          *rego.Engine      // REAL business logic
        reconciler          *controller.AIAnalysisReconciler
        ctx                 context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        scheme = testutil.NewTestScheme()

        fakeK8sClient = fake.NewClientBuilder().
            WithScheme(scheme).
            WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
            Build()

        // Mock ONLY external HolmesGPT API
        mockHolmesGPT = mocks.NewMockHolmesGPT()

        // Use REAL Rego policy engine
        regoEngine = rego.NewEngine(testutil.LoadTestRegoPolicy())

        reconciler = &controller.AIAnalysisReconciler{
            Client:       fakeK8sClient,
            Scheme:       scheme,
            HolmesGPT:    mockHolmesGPT,
            RegoEngine:   regoEngine,  // Real business logic
        }
    })

    // Unit test: validates implementation correctness
    Context("HolmesGPT Investigation Phase", func() {
        It("should investigate alert and generate recommendations with confidence scores", func() {
            // Setup test AIAnalysis CRD
            aia := &aianalysisv1.AIAnalysis{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "aia-test-oom",
                    Namespace: "default",
                },
                Spec: aianalysisv1.AIAnalysisSpec{
                    TargetingData: aianalysisv1.TargetingData{
                        Alert: aianalysisv1.Alert{
                            Fingerprint: "oom-webapp-prod",
                            Namespace:   "production",
                            Labels: map[string]string{
                                "alertname": "PodOOMKilled",
                            },
                        },
                    },
                    InvestigationTimeout: 60 * time.Second,
                },
            }

            Expect(fakeK8sClient.Create(ctx, aia)).To(Succeed())

            // Mock HolmesGPT response with deterministic AI recommendations
            mockHolmesGPT.On("Investigate", ctx, aia.Spec.TargetingData.Alert).Return(
                &holmesgpt.InvestigationResult{
                    RootCause: "Memory leak in webapp container due to unclosed database connections",
                    Recommendations: []holmesgpt.Recommendation{
                        {
                            Action:      "restart-pod",
                            Rationale:   "Immediate relief by restarting leaking pod",
                            Confidence:  0.92,
                            Priority:    1,
                        },
                        {
                            Action:      "increase-memory-limit",
                            Rationale:   "Prevent future OOMs while fixing leak",
                            Confidence:  0.85,
                            Priority:    2,
                        },
                        {
                            Action:      "gitops-pr-fix-leak",
                            Rationale:   "Long-term fix for database connection leak",
                            Confidence:  0.78,
                            Priority:    3,
                        },
                    },
                    ContextUsed: &holmesgpt.ContextMetadata{
                        // NOTE: TokensUsed removed from AIAnalysis - HAPI owns LLM cost observability
                        ProcessingTime:    "3.2s",
                        ModelVersion:      "gpt-4o-2024-05-13",
                    },
                }, nil,
            )

            // Reconcile to trigger investigation
            result, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest(aia))
            Expect(err).ToNot(HaveOccurred())
            Expect(result.Requeue).To(BeFalse())

            // Verify AIAnalysis status updated with recommendations
            var updatedAIA aianalysisv1.AIAnalysis
            Expect(fakeK8sClient.Get(ctx, client.ObjectKeyFromObject(aia), &updatedAIA)).To(Succeed())

            Expect(updatedAIA.Status.Phase).To(Equal("Approving"))
            Expect(updatedAIA.Status.RootCause).To(ContainSubstring("Memory leak"))
            Expect(updatedAIA.Status.Recommendations).To(HaveLen(3))
            Expect(updatedAIA.Status.Recommendations[0].Confidence).To(BeNumerically(">", 0.90))
        })
    })

    // Unit test: validates Rego policy logic (no BR- prefix)
    Context("Rego Policy Evaluation for Auto-Approval", func() {
        DescribeTable("auto-approval policy decisions",
            func(environment string, action string, confidence float64, expectedDecision string) {
                // Create AIAnalysis with investigation complete
                aia := testutil.NewTestAIAnalysis("aia-policy-test")
                aia.Spec.TargetingData.Environment = environment
                aia.Status.Recommendations = []aianalysisv1.Recommendation{
                    {Action: action, Confidence: confidence},
                }
                Expect(fakeK8sClient.Create(ctx, aia)).To(Succeed())

                // Reconcile to trigger Rego policy evaluation
                result, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest(aia))
                Expect(err).ToNot(HaveOccurred())

                // Verify policy decision
                var updatedAIA aianalysisv1.AIAnalysis
                Expect(fakeK8sClient.Get(ctx, client.ObjectKeyFromObject(aia), &updatedAIA)).To(Succeed())
                Expect(updatedAIA.Status.ApprovalStatus).To(Equal(expectedDecision))
            },
            // Auto-approve cases
            Entry("non-prod + high confidence → auto-approve", "dev", "restart-pod", 0.92, "Approved"),
            Entry("non-prod + medium confidence → auto-approve", "staging", "scale-deployment", 0.85, "Approved"),

            // Manual approval cases
            Entry("prod + any action → manual", "production", "restart-pod", 0.95, "PendingApproval"),
            Entry("any env + low confidence → manual", "dev", "delete-pvc", 0.70, "PendingApproval"),
        )
    })

    // ═══════════════════════════════════════════════════════════════════════════
    // V1.1 DEFERRED TESTS: AIApprovalRequest CRD
    // ═══════════════════════════════════════════════════════════════════════════
    // The following test scenarios are OUT OF V1.0 SCOPE per BR_MAPPING.md v1.3:
    //   - BR-AI-040: AIApprovalRequest CRD Creation for Manual Approval
    //   - BR-AI-050: AIApprovalRequest Watch for Approval Decision
    //
    // V1.0 Approach: Approval signaling to RO via status.approvalRequired flag
    // V1.1 Scope: Will implement dedicated AIApprovalRequest CRD workflow
    //
    // Reference: docs/services/crd-controllers/02-aianalysis/BR_MAPPING.md
    // ═══════════════════════════════════════════════════════════════════════════
})
```

---

### Integration Tests (Cross-Component Validation)

**Test Directory**: [test/integration/aianalysis/](../../../test/integration/aianalysis/)
**Coverage Target**: >50% (microservices mandate per 03-testing-strategy.mdc)
**Confidence**: 80-85%
**Execution**: `make test-integration-aianalysis`
**Current Status**: 34/43 tests passing (audit tests blocked by Data Storage batch endpoint)

**Focus Areas**:
- MockHolmesGPTClient for deterministic responses (per HAPI team guidance Dec 9, 2025)
- CRD lifecycle with running controller (envtest)
- Rego policy evaluation with mock evaluator
- Metrics registration via registry inspection (DD-TEST-001)
- Audit event generation (blocked by Data Storage batch endpoint)

**Test Files**:
| File | Tests | Focus |
|------|-------|-------|
| `reconciliation_test.go` | 4 | 4-phase flow, approval scenarios |
| `holmesgpt_integration_test.go` | 12 | MockHolmesGPTClient scenarios |
| `rego_integration_test.go` | 2 | Policy evaluation |
| `metrics_integration_test.go` | 7 | Registry inspection |
| `audit_integration_test.go` | 9 | Audit events (blocked) |

---

### E2E Tests (Complete Workflow Validation)

**Test Directory**: [test/e2e/aianalysis/](../../../../test/e2e/aianalysis/)
**Coverage Target**: 10-15%
**Confidence**: 90-95%
**Execution**: `make test-e2e-aianalysis`

**Test Scenarios**:
1. Auto-approval flow: Investigation → auto-approve → WorkflowExecution creation
2. Manual approval flow: Investigation → pending → operator approval → WorkflowExecution
3. Rejection flow: Investigation → operator rejection → escalation notification

---

### Service-Specific Test Validation

**AI/ML Testing Challenges**:
- ✅ **Non-Deterministic AI**: Mock HolmesGPT with deterministic responses
- ✅ **Rego Policy Complexity**: Use real Rego engine with comprehensive test policies
- ✅ **Approval Workflow States**: Table-driven tests for state transitions
- ✅ **Confidence Thresholds**: Boundary testing (0.79, 0.80, 0.81)

**Unique Metrics to Validate**:
- Average recommendation confidence
- Auto-approval rate (target: 40-60%)
- HolmesGPT API latency (P95 <60s)

---

## 🎯 Test Level Selection: Maintainability First

**Principle**: Prioritize maintainability and simplicity when choosing between unit, integration, and e2e tests.

### Decision Framework

```mermaid
flowchart TD
    Start[New AI Test Scenario] --> Question1{Can test with<br/>simple mocks?<br/>&lt;20 lines setup}

    Question1 -->|Yes| Question2{Testing AI logic<br/>or infrastructure?}
    Question1 -->|No| Integration[Integration Test]

    Question2 -->|AI Logic| Question3{Test readable<br/>and maintainable?}
    Question2 -->|Infrastructure| Integration

    Question3 -->|Yes| Unit[Unit Test]
    Question3 -->|No| Integration

    Unit --> Validate1[✅ Use Unit Test]
    Integration --> Validate2{Complete AI<br/>workflow across<br/>services?}

    Validate2 -->|Yes| E2E[E2E Test]
    Validate2 -->|No| Validate3[✅ Use Integration Test]

    E2E --> Validate4[✅ Use E2E Test]

    style Unit fill:#90EE90
    style Integration fill:#87CEEB
    style E2E fill:#FFB6C1
```

### Test at Unit Level WHEN

- ✅ Scenario can be tested with **simple HolmesGPT mock** (deterministic AI responses)
- ✅ Focus is on **AI business logic** (Rego policy evaluation, confidence scoring, approval decisions)
- ✅ Setup is **straightforward** (< 20 lines of mock configuration)
- ✅ Test remains **readable and maintainable** with mocking

**AIAnalysis Unit Test Examples**:
- Rego policy evaluation (auto-approval vs manual-approval logic)
- Confidence score calculation and threshold enforcement
- Approval workflow state machine transitions
- HolmesGPT response parsing and validation
- Recommendation filtering and ranking

---

### Move to Integration Level WHEN

- ✅ Scenario requires **CRD watch-based coordination** (AIAnalysis → AIApprovalRequest creation)
- ✅ Validating **real HolmesGPT API behavior** (actual AI service integration)
- ✅ Unit test would require **excessive mocking** (>50 lines of approval workflow mocks)
- ✅ Integration test is **simpler to understand** and maintain
- ✅ Testing **real Rego policy engine** with complex policy files

**AIAnalysis Integration Test Examples**:
- Complete CRD reconciliation loop with real K8s API
- HolmesGPT API integration with real AI service (dev environment)
- **EnrichmentData consumption from RemediationProcessing (DD-001: Alternative 2 - BR-WF-RECOVERY-011)**:
  - 📋 **Design Decision**: [DD-001](../../../architecture/DESIGN_DECISIONS.md#dd-001-recovery-context-enrichment-alternative-2) - AIAnalysis reads from spec only
  - Verify AIAnalysis reads enrichment from `spec.enrichmentData` (NO API calls)
  - Validate all contexts available: monitoring + business + recovery (if recovery attempt)
  - Confirm fresh contexts for recovery attempts (not stale from initial attempt)
  - Test prompt enrichment with all available contexts
- Child CRD creation (AIAnalysis → AIApprovalRequest)
- Approval workflow coordination across multiple CRDs
- Status watch patterns and phase transitions in real cluster

---

### Move to E2E Level WHEN

- ✅ Testing **complete investigation-to-workflow journey** (Alert → AIAnalysis → WorkflowExecution)
- ✅ Validating **AI-driven remediation** with real HolmesGPT recommendations
- ✅ Lower-level tests **cannot reproduce AI behavior** (e.g., multi-turn AI conversations)

**AIAnalysis E2E Test Examples**:
- Complete AI investigation pipeline (end-to-end)
- AI-recommended workflow execution validation
- Manual approval escalation flows
- Production-like HolmesGPT failure scenarios (timeout → fallback → recovery)

---

## 🧭 Maintainability Decision Criteria

**Ask these 5 questions before implementing a unit test:**

### 1. Mock Complexity
**Question**: Will mock setup be >30 lines?
- ✅ **YES** → Consider integration test
- ❌ **NO** → Unit test acceptable

**AIAnalysis Example**:
```go
// ❌ COMPLEX: 60+ lines of approval workflow mock setup
mockApprovalWorkflow.On("CreateRequest", ...).Return(complexApprovalMock)
mockApprovalWorkflow.On("WatchStatus", ...).Return(complexWatchMock)
// ... 50+ more lines of state transition mocking
// BETTER: Integration test with real approval CRDs
```

---

### 2. Readability
**Question**: Would a new developer understand this test in 2 minutes?
- ✅ **YES** → Unit test is good
- ❌ **NO** → Consider higher test level

**AIAnalysis Example**:
```go
// ✅ READABLE: Clear AI policy test
It("should auto-approve high-confidence recommendations", func() {
    recommendations := []Recommendation{
        {Action: "restart-pod", Confidence: 0.95},
    }
    decision, err := regoEngine.Evaluate(recommendations)
    Expect(err).ToNot(HaveOccurred())
    Expect(decision.Outcome).To(Equal("auto-approve"))
})
```

---

### 3. Fragility
**Question**: Does test break when internal implementation changes?
- ✅ **YES** → Move to integration test (testing implementation, not behavior)
- ❌ **NO** → Unit test is appropriate

**AIAnalysis Example**:
```go
// ❌ FRAGILE: Breaks if we change internal AI call sequence
Expect(reconciler.holmesGPTCallCount).To(Equal(2))

// ✅ STABLE: Tests AI business behavior, not implementation
Expect(aia.Status.Phase).To(Equal("completed"))
Expect(aia.Status.Recommendations).To(HaveLen(3))
```

---

### 4. Real Value
**Question**: Is this testing AI logic or infrastructure interaction?
- **AI Logic** → Unit test
- **Infrastructure** → Integration test

**AIAnalysis Decision**:
- **Unit**: Rego policy evaluation, confidence scoring (AI logic)
- **Integration**: CRD approval workflow coordination (infrastructure)

---

### 5. Maintenance Cost
**Question**: How much effort to maintain this vs integration test?
- **Lower cost** → Choose that option

**AIAnalysis Example**:
- **Unit test with 80-line approval mock**: HIGH maintenance (breaks on workflow changes)
- **Integration test with real CRDs**: LOW maintenance (automatically adapts to CRD changes)

---

## 🎯 Realistic vs. Exhaustive Testing

**Principle**: Test realistic AI scenarios necessary to validate business requirements - not more, not less.

### AIAnalysis: Requirement-Driven Coverage

**Business Requirement Analysis** (31 V1.0 BRs per BR_MAPPING.md v1.3):
- BR-AI-001 to BR-AI-020: Core AI Investigation & Analysis (15 BRs)
- BR-AI-021 to BR-AI-033: Quality Assurance + Data Management (8 BRs)
- BR-AI-075 to BR-AI-076: Workflow Selection (2 BRs)
- BR-AI-080 to BR-AI-083: Recovery Flow (4 BRs)
- BR-AI-026 to BR-AI-030: Approval & Policy (5 BRs) — *Note: BR-AI-051-053 deferred to V2.0+*

| AI Dimension | Realistic Values | Test Strategy |
|---|---|---|
| **Recommendation Confidence** | 0.0-0.79, 0.80-0.89, 0.90-1.0 | Test distinct approval behaviors, not all decimals |
| **Action Types** | restart, scale, update, migrate | Test policy logic for each type |
| **Approval Decisions** | auto-approve, manual-approve, reject | Test Rego policy outcomes |
| **Investigation Depth** | quick, standard, deep | Test different HolmesGPT analysis levels |

**Total Possible Combinations**: 10 × 4 × 3 × 3 = 360 combinations
**Distinct Business Behaviors**: 31 behaviors (per BR_MAPPING.md v1.3)
**Tests Needed**: ~41 unit tests (see Test Count Summary in implementation plan)

---

### ✅ DO: Test Distinct AI Behaviors Using DescribeTable

**BEST PRACTICE**: Use Ginkgo's `DescribeTable` for AI policy and confidence threshold testing.

```go
// ✅ GOOD: Tests distinct AI approval behaviors using data table
var _ = Describe("BR-AI-030: Rego Policy Evaluation", func() {
    DescribeTable("Approval decision based on confidence and action type",
        func(action string, confidence float64, environment string, expectedDecision string, expectedApproval string) {
            // Single test function handles all policy scenarios
            recommendations := []Recommendation{
                {Action: action, Confidence: confidence},
            }
            context := PolicyContext{Environment: environment}

            decision, err := regoEngine.Evaluate(recommendations, context)
            Expect(err).ToNot(HaveOccurred())
            Expect(decision.Outcome).To(Equal(expectedDecision))
            Expect(decision.ApprovalType).To(Equal(expectedApproval))
        },
        // BR-AI-030.1: High confidence + safe action + production → auto-approve
        Entry("high confidence restart in production → auto-approve",
            "restart-pod", 0.95, "production",
            "approved", "auto"),

        // BR-AI-030.2: High confidence + risky action + production → manual-approve
        Entry("high confidence migrate in production → manual-approve",
            "migrate-workload", 0.95, "production",
            "approved", "manual"),

        // BR-AI-030.3: Medium confidence + safe action + production → manual-approve
        Entry("medium confidence restart in production → manual-approve",
            "restart-pod", 0.85, "production",
            "approved", "manual"),

        // BR-AI-030.4: Low confidence + any action → reject
        Entry("low confidence recommendation → reject",
            "restart-pod", 0.75, "production",
            "rejected", "none"),

        // BR-AI-030.5: High confidence + any action + dev → auto-approve
        Entry("high confidence action in dev → auto-approve",
            "migrate-workload", 0.85, "dev",
            "approved", "auto"),

        // BR-AI-030.6: Boundary test at 0.80 confidence threshold
        Entry("confidence at threshold 0.80 → manual-approve",
            "restart-pod", 0.80, "production",
            "approved", "manual"),
    )
})
```

**Why DescribeTable is Better for AI Testing**:
- ✅ 6 policy scenarios in single function (vs. 6 separate It blocks)
- ✅ Change Rego policy logic once, all tests adapt
- ✅ Clear approval decision matrix visible
- ✅ Easy to add new action types or confidence thresholds
- ✅ Perfect for testing AI confidence boundaries

---

### ❌ DON'T: Test Redundant AI Combinations

```go
// ❌ BAD: Redundant confidence tests that validate SAME policy logic
It("should handle 0.90 confidence", func() {})
It("should handle 0.91 confidence", func() {})
It("should handle 0.92 confidence", func() {})
It("should handle 0.93 confidence", func() {})
It("should handle 0.94 confidence", func() {})
// All 5 tests validate SAME high-confidence approval logic
// BETTER: One test at boundary (0.90), one at typical (0.95)

// ❌ BAD: Exhaustive action type combinations
It("should handle restart-pod + production + 0.95 + auto", func() {})
It("should handle restart-pod + production + 0.95 + manual", func() {})
// ... 358 more combinations for every possible scenario
// These don't test DISTINCT Rego policy behaviors
```

---

### Decision Criteria: Is This AI Test Necessary?

Ask these 4 questions:

1. **Does this test validate a distinct Rego policy rule?**
   - ✅ YES: High confidence + safe action → auto-approve (BR-AI-030.1)
   - ❌ NO: Testing confidence 0.95 vs 0.96 (same policy outcome)

2. **Does this AI scenario actually occur in production?**
   - ✅ YES: HolmesGPT recommends restart-pod with 0.85 confidence
   - ❌ NO: Testing confidence value 0.73948572 (unrealistic precision)

3. **Would this test catch a Rego policy bug the other tests wouldn't?**
   - ✅ YES: Boundary test at exactly 0.80 confidence threshold
   - ❌ NO: Testing 20 different action types with same confidence

4. **Is this testing AI policy behavior or implementation variation?**
   - ✅ AI Policy: Confidence threshold enforcement affects approval decision
   - ❌ Implementation: Internal HolmesGPT API retry count

**If answer is "NO" to all 4 questions** → Skip the test, it adds maintenance cost without AI value

---

### AIAnalysis Test Coverage Example with DescribeTable

**BR-AI-030: Rego Policy Evaluation (10 distinct behaviors)**

```go
Describe("BR-AI-030: Rego Policy Approval Logic", func() {
    // ANALYSIS: 4 action types × 10 confidence levels × 3 environments = 120 combinations
    // REQUIREMENT ANALYSIS: Only 10 distinct Rego policy behaviors per BR-AI-030
    // TEST STRATEGY: Use DescribeTable for 10 behaviors + 2 boundary conditions

    DescribeTable("Rego policy approval decisions",
        func(action string, confidence float64, env string, alertSeverity string,
             expectedOutcome string, expectedApproval string, expectedReason string) {
            // Single test function for all Rego policy evaluation
            input := PolicyInput{
                Recommendation: Recommendation{Action: action, Confidence: confidence},
                Context:        PolicyContext{Environment: env, SignalSeverity: signalSeverity},
            }

            result, err := regoEngine.Evaluate(input)
            Expect(err).ToNot(HaveOccurred())
            Expect(result.Outcome).To(Equal(expectedOutcome))
            Expect(result.ApprovalType).To(Equal(expectedApproval))
            Expect(result.Reason).To(ContainSubstring(expectedReason))
        },
        // Behavior 1: High confidence + safe + prod + critical → auto-approve
        Entry("critical alert with high confidence safe action → auto-approve",
            "restart-pod", 0.95, "production", "critical",
            "approved", "auto", "high confidence and safe action"),

        // Behavior 2: High confidence + risky + prod → manual-approve
        Entry("risky action requires manual approval despite high confidence",
            "migrate-workload", 0.95, "production", "high",
            "approved", "manual", "risky action requires manual review"),

        // Behavior 3: Medium confidence + safe + prod → manual-approve
        Entry("medium confidence requires manual approval",
            "restart-pod", 0.85, "production", "high",
            "approved", "manual", "confidence below auto-approval threshold"),

        // Behavior 4: Low confidence → reject
        Entry("low confidence recommendation rejected",
            "restart-pod", 0.75, "production", "warning",
            "rejected", "none", "confidence too low"),

        // Behavior 5: Dev environment → relaxed policy
        Entry("dev environment allows auto-approval for most actions",
            "update-config", 0.85, "dev", "warning",
            "approved", "auto", "dev environment allows experimentation"),

        // Behavior 6: Warning severity → stricter approval
        Entry("warning alerts require higher confidence",
            "scale-replicas", 0.85, "production", "warning",
            "approved", "manual", "warning severity requires caution"),

        // Behavior 7: Info severity → auto-approve with lower threshold
        Entry("info alerts can auto-approve with lower confidence",
            "adjust-limits", 0.82, "production", "info",
            "approved", "auto", "low-risk info alert"),

        // Behavior 8: Staging environment → moderate policy
        Entry("staging environment allows some auto-approvals",
            "restart-pod", 0.88, "staging", "high",
            "approved", "auto", "staging allows tested actions"),

        // Boundary 1: Exactly at 0.80 confidence threshold
        Entry("confidence at 0.80 threshold (manual-approve boundary)",
            "restart-pod", 0.80, "production", "high",
            "approved", "manual", "at confidence threshold"),

        // Boundary 2: Exactly at 0.90 auto-approval threshold
        Entry("confidence at 0.90 auto-approval threshold",
            "restart-pod", 0.90, "production", "critical",
            "approved", "auto", "meets auto-approval threshold"),
    )

    // Result: 10 Entry() lines cover 10 Rego policy behaviors
    // NOT testing all 120 combinations - only distinct policy rules
    // Coverage: 100% of Rego policy rules, not 100% of input combinations
    // Maintenance: Change Rego policy once, all tests adapt
})
```

**Benefits for AI/ML Testing**:
- ✅ **10 Rego policy rules tested in ~15 lines** (vs. ~200 lines with separate Its)
- ✅ **Single policy evaluation** - Rego changes apply to all tests
- ✅ **Clear AI decision matrix** - approval logic immediately visible
- ✅ **Easy to add AI behaviors** - new Entry for new Rego rules
- ✅ **90% less maintenance** for complex AI policy testing

---

## ⚠️ Anti-Patterns to AVOID

### ❌ OVER-EXTENDED UNIT TESTS (Forbidden)

**Problem**: Excessive mocking (>50 lines) makes AI tests unmaintainable

```go
// ❌ BAD: 80+ lines of approval workflow mock setup
var _ = Describe("Complex Approval Scenario", func() {
    BeforeEach(func() {
        // 80+ lines of complex approval CRD mocking
        mockApprovalCRD.On("Create", ...).Return(complexMock)
        mockApprovalCRD.On("Watch", ...).Return(watchMock)
        // ... 70+ more lines of approval state mocking
        // THIS SHOULD BE AN INTEGRATION TEST
    })
})
```

**Solution**: Move to integration test with real approval CRDs

```go
// ✅ GOOD: Integration test with real approval workflow
var _ = Describe("BR-INTEGRATION-AI-020: Approval Workflow", func() {
    It("should create and track AIApprovalRequest CRD", func() {
        // 15 lines with real K8s API - much clearer
        Expect(k8sClient.Create(ctx, aiApprovalRequest)).To(Succeed())

        Eventually(func() string {
            k8sClient.Get(ctx, approvalKey, aiApprovalRequest)
            return aiApprovalRequest.Status.State
        }).Should(Equal("approved"))
    })
})
```

**Threshold**: If mock setup >30 lines OR total test >50 lines → Consider integration test

---

### ❌ WRONG TEST LEVEL (Forbidden)

**Problem**: Testing HolmesGPT API behavior in unit tests

```go
// ❌ BAD: Testing real HolmesGPT API in unit test
It("should handle HolmesGPT API timeouts", func() {
    // Complex mocking of HTTP timeouts, retries, backoff
    // Real API behavior - belongs in integration test
})
```

**Solution**: Use integration test for real API behavior

```go
// ✅ GOOD: Integration test for HolmesGPT API
It("should handle HolmesGPT API timeouts gracefully", func() {
    // Test with real HolmesGPT API - validates actual timeout behavior
})
```

---

### ❌ REDUNDANT COVERAGE (Forbidden)

**Problem**: Testing same Rego policy at multiple levels

```go
// ❌ BAD: Testing exact same policy logic at all 3 levels
// Unit test: Rego policy auto-approval logic
// Integration test: Rego policy auto-approval logic (duplicate)
// E2E test: Rego policy auto-approval logic (duplicate)
// NO additional value from integration/e2e coverage
```

**Solution**: Test Rego policy in unit tests, test COORDINATION in integration tests

```go
// ✅ GOOD: Each level tests distinct aspect
// Unit test: Rego policy correctness (auto-approve vs manual-approve)
// Integration test: Rego policy + approval CRD creation coordination
// E2E test: Rego policy + approval + workflow execution (complete AI-driven flow)
// Each level adds unique AI workflow value
```

---

## 🎯 Quality Gates (per TESTING_GUIDELINES.md)

### Business Requirement Tests Must:
- [ ] **Map to documented business requirements** (BR-AI-XXX IDs from [BR_MAPPING.md](./BR_MAPPING.md))
- [ ] **Be understandable by non-technical stakeholders** (describe business outcome, not implementation)
- [ ] **Measure business value** (accuracy, performance, cost reduction)
- [ ] **Use realistic data and scenarios** (production-like alert fingerprints, confidence scores)
- [ ] **Validate end-to-end outcomes** (investigation → recommendation → approval decision)
- [ ] **Include business success criteria** (SLA compliance, auto-approval rate targets)

### Unit Tests Must:
- [ ] **Focus on implementation correctness** (Rego policy logic, confidence scoring)
- [ ] **Execute quickly** (<10ms per test average)
- [ ] **Have minimal external dependencies** (mock HolmesGPT API only)
- [ ] **Test edge cases and error conditions** (timeout, malformed response, low confidence)
- [ ] **Provide clear developer feedback** (specific assertion messages)
- [ ] **Maintain high code coverage** (70%+ for core AI logic)

### Integration Tests Must:
- [ ] **Validate CRD lifecycle** (create → reconcile → status update)
- [ ] **Test real Kubernetes API interactions** (with envtest or Kind)
- [ ] **Cover cross-component coordination** (AIAnalysis ↔ SignalProcessing data flow)
- [ ] **Use Eventually() for async operations** (30s timeout, 100ms poll)

---

## 📊 Success Metrics (per TESTING_GUIDELINES.md)

### Target Metrics

| Metric | Target | Achieved | Measurement Command |
|--------|--------|----------|---------------------|
| **Unit Test Execution** | <10ms average per test | ✅ ~5ms | `go test -v ./test/unit/aianalysis/... -json \| jq '.Elapsed'` |
| **Unit Test Coverage** | 70%+ | **87.6%** ✅ | `go test ./test/unit/aianalysis/... -coverprofile=coverage.out -coverpkg=./pkg/aianalysis/...` |
| **BR Test Ratio** | 90%+ tests validate BRs | ✅ ~95% | Manual audit: `grep -c "BR-" test/unit/aianalysis/*_test.go` |
| **Integration Coverage** | >50% CRD operations | TBD (Day 7) | `make test-integration-aianalysis --coverage` |
| **E2E Critical Paths** | 100% coverage | TBD (Day 8) | 3 scenarios: auto-approve, manual-approve, rejection |

### AI-Specific Metrics

| Metric | Target | Achieved | Purpose |
|--------|--------|----------|---------|
| **Rego Policy Coverage** | 100% of rules | ✅ 100% | All approval decision paths tested |
| **Confidence Threshold Tests** | 3 boundary tests | ✅ 9 tests | 0.6, 0.79, 0.80, 0.90, 0.95 boundaries |
| **HolmesGPT Mock Realism** | Production-like responses | ✅ | Token counts, latency simulation |
| **FailedDetections Validation** | All 7 valid fields | ✅ | `gitOpsManaged`, `pdbProtected`, etc. |
| **Human Review Enum Mapping** | All 6 values | ✅ 100% | BR-HAPI-197 enum-to-enum mapping |
| **Error Type Coverage** | All 3 types | ✅ 100% | TransientError, PermanentError, ValidationError |

### Edge Case Test Patterns (Day 6)

**Error Handling Philosophy Coverage** (ERROR_HANDLING_PHILOSOPHY.md):

```go
// ✅ IMPLEMENTED: Error type identification tests
Describe("Error Type Identification", func() {
    It("should identify transient errors using errors.As", func() {
        err := aianalysis.NewTransientError("timeout", nil)
        var transientErr *aianalysis.TransientError
        Expect(errors.As(err, &transientErr)).To(BeTrue())
    })
})
```

**Confidence Level Classification** (BR-AI-019):

```go
// ✅ IMPLEMENTED: DescribeTable for confidence boundaries
DescribeTable("should populate ApprovalContext with correct confidence level",
    func(confidenceScore float64, expectedLevel string) {
        analysis.Status.SelectedWorkflow.Confidence = confidenceScore
        _, err := handler.Handle(ctx, analysis)
        Expect(analysis.Status.ApprovalContext.ConfidenceLevel).To(Equal(expectedLevel))
    },
    Entry("0.80 → high (at threshold)", 0.80, "high"),
    Entry("0.79 → medium (just below)", 0.79, "medium"),
    Entry("0.60 → medium (at threshold)", 0.60, "medium"),
    Entry("0.59 → low (just below)", 0.59, "low"),
)
```

**Retry Mechanism Edge Cases** (BR-AI-021):

```go
// ✅ IMPLEMENTED: Malformed annotation handling
It("should handle malformed retry count annotation (treats as 0)", func() {
    analysis.Annotations = map[string]string{
        "kubernaut.ai/retry-count": "not-a-number",
    }
    mockClient.WithAPIError(503, "Service Unavailable")
    _, err := handler.Handle(ctx, analysis)
    Expect(analysis.Annotations["kubernaut.ai/retry-count"]).To(Equal("1"))
})
```

### Quality Indicators

| Indicator | Good | Warning | Critical |
|-----------|------|---------|----------|
| **Test Flakiness** | <1% | 1-5% | >5% |
| **Mock Complexity** | <30 lines | 30-50 lines | >50 lines (move to integration) |
| **Test Readability** | Understood in 2 min | 2-5 min | >5 min (refactor needed) |

---

## BR-AI-084: Predictive Signal Mode Prompt Strategy

**References**: [BR-AI-084](../../../requirements/BR-AI-084-predictive-signal-mode-prompt-strategy.md), [ADR-054](../../../architecture/decisions/ADR-054-predictive-signal-mode-classification.md)

### RO Unit Tests

**Test File**: `test/unit/remediationorchestrator/aianalysis_creator_test.go` (extend)

| Test ID | Scenario | Status |
|---------|----------|--------|
| UT-RO-106-001 | buildSignalContext copies SignalMode from SP status to AA spec | ✅ Passed |
| UT-RO-106-002 | buildSignalContext reads SignalType from SP status (not RR spec) | ✅ Passed |

### AA Unit Tests

**Test File**: `test/unit/aianalysis/request_builder_test.go` (new)

| Test ID | Scenario | Status |
|---------|----------|--------|
| UT-AA-084-001 | BuildIncidentRequest passes signalMode = reactive | ✅ Passed |
| UT-AA-084-002 | BuildIncidentRequest passes signalMode = predictive | ✅ Passed |

### HAPI Unit Tests (Python)

**Test File**: `holmesgpt-api/tests/test_prompt_builder.py` (new or extend)

| Test ID | Scenario | Status |
|---------|----------|--------|
| UT-HAPI-084-001 | Prompt contains reactive RCA directive when signal_mode = reactive | ✅ Passed |
| UT-HAPI-084-002 | Prompt contains predictive prevention directive when signal_mode = predictive | ✅ Passed |
| UT-HAPI-084-003 | Default to reactive when signal_mode is absent | ✅ Passed |

### Integration Tests

| Test ID | Scenario | Status |
|---------|----------|--------|
| IT-RO-084-001 | RO copies SignalMode from SP to AA spec (full CRD lifecycle) | ✅ Passed |
| IT-HAPI-084-001 | Mock LLM detects predictive mode and returns prevention-focused response | ✅ Passed |

### E2E Tests

| Test ID | Scenario | Test File | Status |
|---------|----------|-----------|--------|
| E2E-SP-106-001 | SP classifies PredictedOOMKill as predictive + normalizes to OOMKilled | `test/e2e/signalprocessing/50_predictive_signal_mode_test.go` | ✅ Passed |
| E2E-SP-106-002 | SP defaults standard OOMKilled to reactive mode | `test/e2e/signalprocessing/50_predictive_signal_mode_test.go` | ✅ Passed |
| E2E-RO-106-001 | RO propagates signalMode=predictive from SP status to AA spec | `test/e2e/remediationorchestrator/predictive_signal_mode_e2e_test.go` | ✅ Passed |
| E2E-AA-084-001 | AA processes predictive signal mode through to Mock LLM | `test/e2e/aianalysis/07_predictive_signal_mode_test.go` | ✅ Passed |
| E2E-AA-084-002 | AA processes reactive signal mode (standard flow) | `test/e2e/aianalysis/07_predictive_signal_mode_test.go` | ✅ Passed |
| E2E-HAPI-055 | HAPI returns predictive-aware analysis for signal_mode=predictive | `test/e2e/holmesgpt-api/predictive_signal_mode_test.go` | ✅ Passed |
| E2E-HAPI-056 | HAPI returns standard RCA for signal_mode=reactive | `test/e2e/holmesgpt-api/predictive_signal_mode_test.go` | ✅ Passed |
| E2E-HAPI-057 | HAPI defaults to reactive when signal_mode is absent | `test/e2e/holmesgpt-api/predictive_signal_mode_test.go` | ✅ Passed |

---
