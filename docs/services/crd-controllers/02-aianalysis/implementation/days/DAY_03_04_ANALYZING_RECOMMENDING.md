# Days 3-4: AnalyzingHandler & RecommendingHandler

> **Note (ADR-056/ADR-055):** References to `EnrichmentResults.DetectedLabels` and `EnrichmentResults.OwnerChain` in this document are historical. These fields were removed per ADR-056 and ADR-055.

**Part of**: AI Analysis Implementation Plan V1.0
**Parent Document**: [IMPLEMENTATION_PLAN_V1.0.md](../../IMPLEMENTATION_PLAN_V1.0.md)
**Duration**: 12-16 hours (2 days)
**Target Confidence**: 78% (Day 4 Midpoint)
**Version**: v1.3

**Changelog**:
- **v1.3** (2025-12-05): PolicyInput schema alignment with implementation plan
  - ✅ **Extended PolicyInput**: Added all fields from IMPLEMENTATION_PLAN_V1.0.md lines 1756-1785
    - Signal context: `SignalType`, `Severity`, `BusinessPriority`
    - Target resource: `Kind`, `Name`, `Namespace`
    - Recovery context: `IsRecoveryAttempt`, `RecoveryAttemptNumber`
  - ✅ **Recovery Rules**: Test policy now includes recovery scenario rules (3+ attempts, high severity)
  - ✅ **Tests Added**: 13 new tests for extended PolicyInput fields and recovery scenarios
  - 📏 **Reference**: `pkg/aianalysis/rego/evaluator.go`, `test/unit/aianalysis/testdata/policies/approval.rego`
- **v1.2** (2025-12-05): OPA v1 Rego syntax update
  - ✅ **OPA v1 Syntax**: All Rego policies MUST use `if` keyword and `:=` operator
  - ✅ **Import**: Use `import rego.v1` or `github.com/open-policy-agent/opa/v1/rego`
  - ✅ **Breaking Change**: Old syntax without `if` will cause parse errors
  - 📏 **Reference**: `test/unit/aianalysis/testdata/policies/approval.rego`
- **v1.1** (2025-12-05): Architecture clarification from HolmesGPT-API team
  - ✅ **Day 4 Simplification**: RecommendingHandler is now a status finalizer (NO separate HAPI call)
  - ✅ **Workflow Data**: Already captured in InvestigatingHandler from `/incident/analyze`
  - ✅ **CRD Schema**: Added `AlternativeWorkflows` field for audit/operator context
  - ✅ **Key Principle**: "Alternatives are for CONTEXT, not EXECUTION"
  - 📏 **Reference**: [AIANALYSIS_TO_HOLMESGPT_API_TEAM.md](../../../../handoff/AIANALYSIS_TO_HOLMESGPT_API_TEAM.md) Q12-Q13
- **v1.0** (2025-12-04): Initial document

---

## 🔔 Architecture Clarification (Dec 5, 2025)

> **IMPORTANT**: Per HolmesGPT-API team response to Q12-Q13:
>
> | Endpoint | Returns | Phase |
> |----------|---------|-------|
> | `/api/v1/incident/analyze` | RCA + `selected_workflow` + `alternative_workflows` | **Investigating** |
> | N/A (local evaluation) | `approvalRequired` via Rego policy | **Analyzing** |
> | N/A (finalize status) | Phase=Completed, populate ApprovalContext | **Recommending** |
>
> **Key Insight**: The `/incident/analyze` endpoint returns ALL analysis results in ONE call.
> - `InvestigatingHandler` captures: RCA, SelectedWorkflow, AlternativeWorkflows, TargetInOwnerChain, Warnings
> - `AnalyzingHandler` evaluates: Rego approval policy using data already in status
> - `RecommendingHandler` finalizes: Status, populates ApprovalContext, transitions to Completed
>
> **Day 4 code samples below are OUTDATED** - RecommendingHandler does NOT call HolmesGPT-API.
> See updated implementation in `pkg/aianalysis/handlers/recommending.go`.

---

## ⚠️ OPA v1 Rego Syntax Requirement

> **CRITICAL**: All Rego policies in this project MUST use OPA v1 syntax.

### What Changed in OPA v1?

| Aspect | Old Syntax (v0) | New Syntax (v1) |
|--------|-----------------|-----------------|
| **Package import** | `"github.com/open-policy-agent/opa/rego"` | `"github.com/open-policy-agent/opa/v1/rego"` |
| **Default values** | `default x = false` | `default x := false` |
| **Rule bodies** | `rule { condition }` | `rule if { condition }` |
| **Assignment** | `x = value { cond }` | `x := value if { cond }` |

### Example: Correct OPA v1 Policy

```rego
package aianalysis.approval

import rego.v1  # Optional but recommended

# Default with := operator
default require_approval := false

# Helper rule with 'if' keyword
is_production if {
    input.environment == "production"
}

# Main rule with 'if' keyword
require_approval if {
    is_production
    not input.target_in_owner_chain
}

# Assignment rule with := and if
reason := "Target not in owner chain" if {
    is_production
    not input.target_in_owner_chain
}
```

### Common Error

If you see this error:
```
rego_parse_error: `if` keyword is required before rule body
```

It means your Rego policy is using old v0 syntax. Add `if` before all rule bodies.

---

## 🎯 Day 3 Objectives: AnalyzingHandler

| Objective | Priority | BR Reference |
|-----------|----------|--------------|
| Implement Rego policy evaluator | P0 | BR-AI-011 |
| Implement AnalyzingHandler | P0 | BR-AI-012 |
| Handle approval determination | P0 | BR-AI-013 |
| Graceful degradation for Rego failures | P0 | BR-AI-014 |

---

## 🔴 Day 3 TDD RED Phase: AnalyzingHandler Tests

### Rego Evaluator Tests

```go
// test/unit/aianalysis/rego_evaluator_test.go
package aianalysis

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

var _ = Describe("RegoEvaluator", func() {
    var evaluator *rego.Evaluator

    BeforeEach(func() {
        evaluator = rego.NewEvaluator(rego.Config{
            PolicyDir: "testdata/policies",
        })
    })

    // BR-AI-011: Policy evaluation
    Describe("Evaluate", func() {
        Context("with valid policy and input", func() {
            It("should return approval decision - BR-AI-011", func() {
                input := &rego.PolicyInput{
                    Environment:        "production",
                    TargetInOwnerChain: true,
                    DetectedLabels: map[string]interface{}{
                        "gitOpsManaged": true,
                        "pdbProtected":  true,
                    },
                    FailedDetections: []string{},
                    Warnings:         []string{},
                }

                result, err := evaluator.Evaluate(ctx, input)

                Expect(err).NotTo(HaveOccurred())
                Expect(result).NotTo(BeNil())
                Expect(result.ApprovalRequired).To(BeAssignableToTypeOf(true))
            })
        })

        // BR-AI-013: Approval scenarios
        DescribeTable("determines approval requirement",
            func(env string, targetInChain bool, failedDetections []string, expectedApproval bool) {
                input := &rego.PolicyInput{
                    Environment:        env,
                    TargetInOwnerChain: targetInChain,
                    FailedDetections:   failedDetections,
                }

                result, err := evaluator.Evaluate(ctx, input)

                Expect(err).NotTo(HaveOccurred())
                Expect(result.ApprovalRequired).To(Equal(expectedApproval))
            },
            // Production + data quality issues = approval required
            Entry("production + target not in chain", "production", false, nil, true),
            Entry("production + failed detections", "production", true, []string{"gitOpsManaged"}, true),

            // Non-production = auto-approve
            Entry("development + any state", "development", false, []string{"gitOpsManaged"}, false),
            Entry("staging + any state", "staging", true, nil, false),

            // Production + good data = auto-approve
            Entry("production + clean state", "production", true, nil, false),
        )

        // BR-AI-014: Graceful degradation
        Context("when policy file is missing", func() {
            BeforeEach(func() {
                evaluator = rego.NewEvaluator(rego.Config{
                    PolicyDir: "nonexistent/path",
                })
            })

            It("should default to manual approval", func() {
                result, err := evaluator.Evaluate(ctx, &rego.PolicyInput{})

                // Should not error - graceful degradation
                Expect(err).NotTo(HaveOccurred())
                Expect(result.ApprovalRequired).To(BeTrue())
                Expect(result.Degraded).To(BeTrue())
            })
        })

        // BR-AI-014: Syntax error handling
        Context("when policy has syntax error", func() {
            BeforeEach(func() {
                evaluator = rego.NewEvaluator(rego.Config{
                    PolicyDir: "testdata/invalid_policies",
                })
            })

            It("should default to manual approval", func() {
                result, err := evaluator.Evaluate(ctx, &rego.PolicyInput{})

                Expect(err).NotTo(HaveOccurred())
                Expect(result.ApprovalRequired).To(BeTrue())
                Expect(result.Degraded).To(BeTrue())
                Expect(result.Reason).To(ContainSubstring("policy evaluation failed"))
            })
        })
    })
})
```

### AnalyzingHandler Tests

```go
// test/unit/aianalysis/analyzing_handler_test.go
package aianalysis

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

var _ = Describe("AnalyzingHandler", func() {
    var (
        handler      *handlers.AnalyzingHandler
        mockEvaluator *MockRegoEvaluator
    )

    BeforeEach(func() {
        mockEvaluator = NewMockRegoEvaluator()
        handler = handlers.NewAnalyzingHandler(
            handlers.WithRegoEvaluator(mockEvaluator),
            handlers.WithLogger(testLogger),
        )
    })

    // BR-AI-012: Analyzing phase handling
    Describe("Handle", func() {
        Context("when Rego evaluation succeeds", func() {
            BeforeEach(func() {
                mockEvaluator.SetResult(&rego.PolicyResult{
                    ApprovalRequired: true,
                    Reason:           "Production environment requires approval",
                    Degraded:         false,
                })
            })

            It("should transition to Recommending phase", func() {
                analysis := analysisInPhase(aianalysisv1.PhaseAnalyzing)

                result, err := handler.Handle(ctx, analysis)

                Expect(err).NotTo(HaveOccurred())
                Expect(result.Requeue).To(BeTrue())
                Expect(analysis.Status.Phase).To(Equal(aianalysisv1.PhaseRecommending))
                Expect(analysis.Status.ApprovalRequired).To(BeTrue())
            })
        })

        // BR-AI-014: Degraded mode
        Context("when Rego evaluation fails gracefully", func() {
            BeforeEach(func() {
                mockEvaluator.SetResult(&rego.PolicyResult{
                    ApprovalRequired: true, // Safe default
                    Reason:           "Policy evaluation failed - defaulting to manual approval",
                    Degraded:         true,
                })
            })

            It("should continue with safe default", func() {
                analysis := analysisInPhase(aianalysisv1.PhaseAnalyzing)

                result, err := handler.Handle(ctx, analysis)

                Expect(err).NotTo(HaveOccurred())
                Expect(analysis.Status.ApprovalRequired).To(BeTrue())
                Expect(analysis.Status.DegradedMode).To(BeTrue())
            })
        })
    })
})
```

---

## 🟢 Day 3 GREEN Phase: Rego Evaluator & AnalyzingHandler

### Rego Evaluator

> **⚠️ OPA v1 REQUIRED**: Use `github.com/open-policy-agent/opa/v1/rego` package.
> Rego policies MUST use v1 syntax with `if` keyword and `:=` operator.

```go
// pkg/aianalysis/rego/evaluator.go
package rego

import (
    "context"
    "fmt"
    "os"

    "github.com/open-policy-agent/opa/v1/rego" // OPA v1 - REQUIRED
)

// Config for Rego evaluator
type Config struct {
    PolicyDir string
}

// PolicyInput represents input to Rego policy
type PolicyInput struct {
    Environment        string                 `json:"environment"`
    TargetInOwnerChain bool                   `json:"target_in_owner_chain"`
    DetectedLabels     map[string]interface{} `json:"detected_labels"`
    CustomLabels       map[string][]string    `json:"custom_labels"`
    FailedDetections   []string               `json:"failed_detections"`
    Warnings           []string               `json:"warnings"`
}

// PolicyResult represents Rego policy evaluation result
type PolicyResult struct {
    ApprovalRequired bool
    Reason           string
    Degraded         bool
}

// Evaluator evaluates Rego policies
type Evaluator struct {
    policyDir string
    query     *rego.PreparedEvalQuery
}

// NewEvaluator creates a new Rego evaluator
func NewEvaluator(cfg Config) *Evaluator {
    return &Evaluator{
        policyDir: cfg.PolicyDir,
    }
}

// Evaluate evaluates the approval policy
func (e *Evaluator) Evaluate(ctx context.Context, input *PolicyInput) (*PolicyResult, error) {
    // Try to load and evaluate policy
    query, err := rego.New(
        rego.Query("data.aianalysis.approval"),
        rego.Load([]string{e.policyDir}, nil),
    ).PrepareForEval(ctx)

    if err != nil {
        // Graceful degradation: policy load failed
        return &PolicyResult{
            ApprovalRequired: true, // Safe default
            Reason:           fmt.Sprintf("Policy evaluation failed: %v - defaulting to manual approval", err),
            Degraded:         true,
        }, nil
    }

    // Evaluate policy
    results, err := query.Eval(ctx, rego.EvalInput(input))
    if err != nil {
        return &PolicyResult{
            ApprovalRequired: true,
            Reason:           fmt.Sprintf("Policy evaluation error: %v", err),
            Degraded:         true,
        }, nil
    }

    // Parse results
    if len(results) == 0 || len(results[0].Expressions) == 0 {
        return &PolicyResult{
            ApprovalRequired: true,
            Reason:           "No policy result - defaulting to manual approval",
            Degraded:         true,
        }, nil
    }

    // Extract approval decision
    resultMap, ok := results[0].Expressions[0].Value.(map[string]interface{})
    if !ok {
        return &PolicyResult{
            ApprovalRequired: true,
            Reason:           "Invalid policy result format",
            Degraded:         true,
        }, nil
    }

    approvalRequired, _ := resultMap["require_approval"].(bool)
    reason, _ := resultMap["reason"].(string)

    return &PolicyResult{
        ApprovalRequired: approvalRequired,
        Reason:           reason,
        Degraded:         false,
    }, nil
}
```

### AnalyzingHandler

```go
// pkg/aianalysis/handlers/analyzing.go
package handlers

import (
    "context"

    "github.com/go-logr/logr"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
    ctrl "sigs.k8s.io/controller-runtime"
)

// AnalyzingHandler handles the Analyzing phase
type AnalyzingHandler struct {
    log       logr.Logger
    evaluator rego.EvaluatorInterface
}

// NewAnalyzingHandler creates a new AnalyzingHandler
func NewAnalyzingHandler(opts ...Option) *AnalyzingHandler {
    h := &AnalyzingHandler{}
    for _, opt := range opts {
        opt(h)
    }
    return h
}

// Phase returns the phase this handler processes
func (h *AnalyzingHandler) Phase() aianalysisv1.Phase {
    return aianalysisv1.PhaseAnalyzing
}

// Handle evaluates Rego policies and determines approval requirement
func (h *AnalyzingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    // Build policy input
    input := h.buildPolicyInput(analysis)

    // Evaluate policy
    result, err := h.evaluator.Evaluate(ctx, input)
    if err != nil {
        // This shouldn't happen due to graceful degradation in evaluator
        analysis.Status.Phase = aianalysisv1.PhaseFailed
        analysis.Status.Message = fmt.Sprintf("Rego evaluation failed: %v", err)
        return ctrl.Result{}, nil
    }

    // Store results
    analysis.Status.ApprovalRequired = result.ApprovalRequired
    analysis.Status.ApprovalReason = result.Reason
    if result.Degraded {
        analysis.Status.DegradedMode = true
        h.log.Info("Operating in degraded mode due to policy evaluation failure",
            "reason", result.Reason,
        )
    }

    // Transition to Recommending phase
    analysis.Status.Phase = aianalysisv1.PhaseRecommending
    return ctrl.Result{Requeue: true}, nil
}

func (h *AnalyzingHandler) buildPolicyInput(analysis *aianalysisv1.AIAnalysis) *rego.PolicyInput {
    spec := &analysis.Spec.SignalContext
    status := &analysis.Status

    input := &rego.PolicyInput{
        Environment: spec.Environment,
        Warnings:    status.Warnings,
    }

    // Add targetInOwnerChain from status
    if status.TargetInOwnerChain != nil {
        input.TargetInOwnerChain = *status.TargetInOwnerChain
    }

    // Add detected labels and failed detections
    if spec.EnrichmentResults != nil && spec.EnrichmentResults.DetectedLabels != nil {
        input.DetectedLabels = h.convertDetectedLabels(spec.EnrichmentResults.DetectedLabels)
        input.FailedDetections = spec.EnrichmentResults.DetectedLabels.FailedDetections
    }

    // Add custom labels
    if spec.EnrichmentResults != nil {
        input.CustomLabels = spec.EnrichmentResults.CustomLabels
    }

    return input
}
```

---

## 🎯 Day 4 Objectives: RecommendingHandler + Midpoint

> **⚠️ UPDATED (Dec 5, 2025)**: RecommendingHandler is now a status finalizer.
> Workflow data is already captured by InvestigatingHandler from `/incident/analyze`.
> RecommendingHandler does NOT call HolmesGPT-API.

| Objective | Priority | BR Reference | Notes |
|-----------|----------|--------------|-------|
| Implement RecommendingHandler | P0 | BR-AI-016 | **Status finalizer only** |
| ~~Query workflow recommendations~~ | ~~P0~~ | ~~BR-AI-017~~ | **DONE in Investigating** |
| Validate workflow exists in status | P0 | BR-AI-018 | From InvestigatingHandler |
| Populate ApprovalContext | P0 | BR-AI-019 | If approvalRequired=true |
| Transition to Completed/Failed | P0 | BR-AI-020 | Final phase |
| **Midpoint checkpoint** | P0 | — | |

---

## 🔴 Day 4 TDD RED Phase: RecommendingHandler Tests

> **⚠️ OUTDATED CODE SAMPLES**: The code below shows the original plan which called HolmesGPT-API.
> Per Dec 5, 2025 architecture clarification, RecommendingHandler is a **status finalizer only**.
> See updated implementation approach in the "Architecture Clarification" section at the top.

<details>
<summary>📜 Original Code Samples (OUTDATED - Click to expand)</summary>

```go
// test/unit/aianalysis/recommending_handler_test.go
// ⚠️ OUTDATED: RecommendingHandler no longer calls HAPI
package aianalysis

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
)

var _ = Describe("RecommendingHandler", func() {
    var (
        handler         *handlers.RecommendingHandler
        mockHGClient    *MockHolmesGPTClient
    )

    BeforeEach(func() {
        mockHGClient = NewMockHolmesGPTClient()
        handler = handlers.NewRecommendingHandler(
            handlers.WithHolmesGPTClient(mockHGClient),
            handlers.WithLogger(testLogger),
        )
    })

    // BR-AI-016: Workflow recommendation
    Describe("Handle", func() {
        Context("with successful workflow selection", func() {
            BeforeEach(func() {
                mockHGClient.SetRecoveryResponse(&client.RecoveryResponse{
                    RecommendedWorkflows: []client.WorkflowRecommendation{
                        {
                            WorkflowID:   "wf-restart-pod",
                            DisplayName:  "Restart Pod",
                            Confidence:   0.92,
                            Rationale:    "Pod is stuck in CrashLoopBackOff",
                            Parameters:   map[string]string{"NAMESPACE": "default"},
                        },
                    },
                    AlternativeWorkflows: []client.WorkflowRecommendation{
                        {
                            WorkflowID:   "wf-scale-deployment",
                            DisplayName:  "Scale Deployment",
                            Confidence:   0.75,
                            Rationale:    "Consider scaling if restart fails",
                        },
                    },
                }, nil)
            })

            It("should store recommendations and complete", func() {
                analysis := analysisInPhase(aianalysisv1.PhaseRecommending)

                result, err := handler.Handle(ctx, analysis)

                Expect(err).NotTo(HaveOccurred())
                Expect(result.Requeue).To(BeFalse()) // Final phase
                Expect(analysis.Status.Phase).To(Equal(aianalysisv1.PhaseCompleted))
                Expect(analysis.Status.Recommendations).To(HaveLen(1))
                Expect(analysis.Status.Recommendations[0].WorkflowID).To(Equal("wf-restart-pod"))
                Expect(analysis.Status.AlternativeWorkflows).To(HaveLen(1))
            })
        })

        // BR-AI-018: No recommendations scenario
        Context("with no matching workflows", func() {
            BeforeEach(func() {
                mockHGClient.SetRecoveryResponse(&client.RecoveryResponse{
                    RecommendedWorkflows: []client.WorkflowRecommendation{},
                    Message:              "No suitable workflows found",
                }, nil)
            })

            It("should complete with empty recommendations", func() {
                analysis := analysisInPhase(aianalysisv1.PhaseRecommending)

                result, err := handler.Handle(ctx, analysis)

                Expect(err).NotTo(HaveOccurred())
                Expect(analysis.Status.Phase).To(Equal(aianalysisv1.PhaseCompleted))
                Expect(analysis.Status.Recommendations).To(BeEmpty())
                Expect(analysis.Status.Message).To(ContainSubstring("No suitable workflows"))
            })
        })

        // Error handling
        Context("when HolmesGPT-API returns error", func() {
            BeforeEach(func() {
                mockHGClient.SetRecoveryResponse(nil, &client.APIError{StatusCode: 503})
            })

            It("should retry on transient errors", func() {
                analysis := analysisInPhase(aianalysisv1.PhaseRecommending)

                result, err := handler.Handle(ctx, analysis)

                Expect(err).To(HaveOccurred())
                Expect(result.RequeueAfter).To(BeNumerically(">", 0))
            })
        })
    })
})
```

</details>

---

## 🟢 Day 4 GREEN Phase: RecommendingHandler

> **⚠️ OUTDATED CODE SAMPLES**: See "Architecture Clarification" section at the top.
> RecommendingHandler is now a status finalizer that does NOT call HolmesGPT-API.

<details>
<summary>📜 Original Code Samples (OUTDATED - Click to expand)</summary>

```go
// pkg/aianalysis/handlers/recommending.go
// ⚠️ OUTDATED: RecommendingHandler no longer calls HAPI
package handlers

import (
    "context"
    "fmt"

    "github.com/go-logr/logr"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/client"
    ctrl "sigs.k8s.io/controller-runtime"
)

// RecommendingHandler handles the Recommending phase
type RecommendingHandler struct {
    log      logr.Logger
    hgClient client.HolmesGPTClientInterface
}

// NewRecommendingHandler creates a new RecommendingHandler
func NewRecommendingHandler(opts ...Option) *RecommendingHandler {
    h := &RecommendingHandler{}
    for _, opt := range opts {
        opt(h)
    }
    return h
}

// Phase returns the phase this handler processes
func (h *RecommendingHandler) Phase() aianalysisv1.Phase {
    return aianalysisv1.PhaseRecommending
}

// Handle queries for workflow recommendations
func (h *RecommendingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    // Build recovery request
    req := h.buildRecoveryRequest(analysis)

    // Call HolmesGPT-API for recovery suggestions
    resp, err := h.hgClient.GetRecoverySuggestions(ctx, req)
    if err != nil {
        return h.handleError(ctx, analysis, err)
    }

    // Store recommendations
    h.storeRecommendations(analysis, resp)

    // Mark as completed
    analysis.Status.Phase = aianalysisv1.PhaseCompleted
    analysis.Status.CompletedAt = metav1.Now()

    return ctrl.Result{}, nil // Final phase - no requeue
}

func (h *RecommendingHandler) buildRecoveryRequest(analysis *aianalysisv1.AIAnalysis) *client.RecoveryRequest {
    spec := &analysis.Spec.SignalContext
    status := &analysis.Status

    return &client.RecoveryRequest{
        IncidentContext: status.Investigation.Analysis,
        Environment:     spec.Environment,
        DetectedLabels:  h.convertDetectedLabels(spec.EnrichmentResults),
        CustomLabels:    spec.EnrichmentResults.CustomLabels,
    }
}

func (h *RecommendingHandler) storeRecommendations(analysis *aianalysisv1.AIAnalysis, resp *client.RecoveryResponse) {
    // Store primary recommendations
    for _, rec := range resp.RecommendedWorkflows {
        analysis.Status.Recommendations = append(analysis.Status.Recommendations,
            aianalysisv1.WorkflowRecommendation{
                WorkflowID:  rec.WorkflowID,
                DisplayName: rec.DisplayName,
                Confidence:  rec.Confidence,
                Rationale:   rec.Rationale,
                Parameters:  rec.Parameters,
            })
    }

    // Store alternatives
    for _, alt := range resp.AlternativeWorkflows {
        analysis.Status.AlternativeWorkflows = append(analysis.Status.AlternativeWorkflows,
            aianalysisv1.WorkflowRecommendation{
                WorkflowID:  alt.WorkflowID,
                DisplayName: alt.DisplayName,
                Confidence:  alt.Confidence,
                Rationale:   alt.Rationale,
            })
    }

    // Store message if no recommendations
    if len(resp.RecommendedWorkflows) == 0 && resp.Message != "" {
        analysis.Status.Message = resp.Message
    }
}

func (h *RecommendingHandler) handleError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) (ctrl.Result, error) {
    // Use same retry logic as InvestigatingHandler
    var apiErr *client.APIError
    if errors.As(err, &apiErr) && apiErr.IsTransient() {
        if analysis.Status.RetryCount >= MaxRetries {
            analysis.Status.Phase = aianalysisv1.PhaseFailed
            analysis.Status.Message = fmt.Sprintf("Workflow recommendation failed: %v", err)
            return ctrl.Result{}, nil
        }
        delay := calculateBackoff(analysis.Status.RetryCount)
        analysis.Status.RetryCount++
        return ctrl.Result{RequeueAfter: delay}, err
    }

    analysis.Status.Phase = aianalysisv1.PhaseFailed
    analysis.Status.Message = fmt.Sprintf("Workflow recommendation error: %v", err)
    return ctrl.Result{}, nil
}
```

</details>

---

## ✅ Updated Day 4: RecommendingHandler (Status Finalizer)

Per Dec 5, 2025 architecture clarification, RecommendingHandler is simplified:

```go
// pkg/aianalysis/handlers/recommending.go
// RecommendingHandler finalizes status - NO HolmesGPT-API call needed
// Workflow data already captured by InvestigatingHandler
package handlers

import (
    "context"

    "github.com/go-logr/logr"
    ctrl "sigs.k8s.io/controller-runtime"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/internal/controller/aianalysis"
)

// RecommendingHandler finalizes the AIAnalysis status
// No HolmesGPT-API call - workflow data already in status from InvestigatingHandler
type RecommendingHandler struct {
    log logr.Logger
}

// NewRecommendingHandler creates a new RecommendingHandler
func NewRecommendingHandler(log logr.Logger) *RecommendingHandler {
    return &RecommendingHandler{
        log: log.WithName("recommending-handler"),
    }
}

// Handle finalizes the AIAnalysis status
func (h *RecommendingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    h.log.Info("Finalizing analysis", "name", analysis.Name)

    // Validate workflow exists (captured by InvestigatingHandler)
    if analysis.Status.SelectedWorkflow == nil {
        analysis.Status.Phase = aianalysis.PhaseFailed
        analysis.Status.Message = "No workflow selected - investigation may have failed"
        analysis.Status.Reason = "NoWorkflowSelected"
        return ctrl.Result{}, nil
    }

    // Populate ApprovalContext if approval is required
    if analysis.Status.ApprovalRequired {
        h.populateApprovalContext(analysis)
    }

    // Transition to Completed
    analysis.Status.Phase = aianalysis.PhaseCompleted
    analysis.Status.Message = "Analysis complete"

    return ctrl.Result{}, nil // Final phase - no requeue
}

func (h *RecommendingHandler) populateApprovalContext(analysis *aianalysisv1.AIAnalysis) {
    if analysis.Status.ApprovalContext == nil {
        analysis.Status.ApprovalContext = &aianalysisv1.ApprovalContext{}
    }

    ctx := analysis.Status.ApprovalContext
    ctx.Reason = analysis.Status.ApprovalReason
    ctx.ConfidenceScore = analysis.Status.SelectedWorkflow.Confidence
    ctx.WhyApprovalRequired = analysis.Status.ApprovalReason

    // Set confidence level based on score
    switch {
    case ctx.ConfidenceScore >= 0.8:
        ctx.ConfidenceLevel = "high"
    case ctx.ConfidenceScore >= 0.6:
        ctx.ConfidenceLevel = "medium"
    default:
        ctx.ConfidenceLevel = "low"
    }

    // Include investigation summary from RCA
    if analysis.Status.RootCauseAnalysis != nil {
        ctx.InvestigationSummary = analysis.Status.RootCauseAnalysis.Summary
    }
}
```

**Key Changes**:
- ❌ No HolmesGPT-API call (workflow already in status)
- ✅ Validates `SelectedWorkflow` exists
- ✅ Populates `ApprovalContext` if `ApprovalRequired=true`
- ✅ Transitions to `Completed` phase

---

## ⭐ Day 4 Midpoint Checkpoint

### Validation Checklist

```bash
# Build verification
go build ./pkg/aianalysis/...

# Run all unit tests
go test -v ./test/unit/aianalysis/...

# Coverage check (target: 70%+)
go test -coverprofile=coverage.out ./pkg/aianalysis/...
go tool cover -func=coverage.out | grep total

# Lint check
golangci-lint run ./pkg/aianalysis/...
```

### Midpoint Confidence Assessment

| Component | Expected | Actual | Notes |
|-----------|----------|--------|-------|
| Reconciler | 90% | — | Core loop working |
| ValidatingHandler | 95% | — | Complete |
| InvestigatingHandler | 90% | — | HolmesGPT integration working |
| AnalyzingHandler | 85% | — | Rego evaluation working |
| RecommendingHandler | 80% | — | Workflow recommendations working |
| **Overall** | **78%** | — | Midpoint target |

### EOD Documentation

Create: `docs/services/crd-controllers/02-aianalysis/implementation/phase0/02-day4-midpoint.md`

```markdown
# Day 4 Midpoint - AIAnalysis

**Date**: YYYY-MM-DD
**Confidence**: XX%
**Status**: ✅ On Track / ⚠️ Needs Attention / ❌ Blocked

## Summary
All 4 phase handlers implemented with unit tests.

## Completed
- ✅ ValidatingHandler with FailedDetections validation
- ✅ InvestigatingHandler with HolmesGPT-API integration
- ✅ AnalyzingHandler with Rego policy evaluation
- ✅ RecommendingHandler with workflow recommendations

## Test Results
- Unit tests: XX passing
- Coverage: XX%
- Lint errors: 0

## Outstanding Items
- Integration tests (Day 5-7)
- E2E tests (Day 8)
- Metrics implementation (Day 5-6)

## Risks
- [Risk 1]: [Mitigation]

## Days 5-7 Plan
1. Error handling refinement
2. Metrics implementation
3. Integration tests with KIND
4. Full reconciliation loop tests
```

---

## 📚 Related Documents

- [DAY_02_INVESTIGATING_HANDLER.md](./DAY_02_INVESTIGATING_HANDLER.md) - Previous day
- [DAY_05_07_INTEGRATION_TESTING.md](./DAY_05_07_INTEGRATION_TESTING.md) - Next phase
- [APPENDIX_D_TESTING_PATTERNS.md](../appendices/APPENDIX_D_TESTING_PATTERNS.md) - Test patterns

