# DD-AIANALYSIS-001: Rego Policy Loading Strategy

## Status
**✅ APPROVED** (2025-11-29)
**Updated**: 2025-12-05 (OPA v1 syntax requirement)
**Follows**: Gateway pattern (DD-GATEWAY-XXX)
**Confidence**: 95%

### ⚠️ OPA v1 Syntax Requirement (Dec 2025)

All Rego policies MUST use OPA v1 syntax:
- **Package**: `github.com/open-policy-agent/opa/v1/rego`
- **Default**: Use `:=` operator (`default x := false`)
- **Rules**: Use `if` keyword (`rule if { condition }`)
- **Import**: Optionally add `import rego.v1` at top of policy

## Context & Problem

The AIAnalysis Controller needs to evaluate Rego policies to determine whether AI-recommended remediations can be auto-approved or require manual review. The question is: **how should policies be loaded and evaluated?**

**Key Requirements**:
- **BR-AI-026**: Configurable approval thresholds
- **BR-AI-027**: Environment-specific approval rules
- **BR-AI-028**: Risk tolerance-based decisions
- **ADR-041**: Rego policies receive pre-fetched data (no external API calls)

## Decision

**Follow the Gateway pattern**: Load Rego policies from files mounted from ConfigMaps.

### Gateway Pattern Reference

The Gateway service already implements Rego-based priority assignment:

```go
// pkg/gateway/processing/priority.go
type PriorityEngine struct {
    regoQuery *rego.PreparedEvalQuery
    logger    logr.Logger
}

func NewPriorityEngineWithRego(policyPath string, logger logr.Logger) (*PriorityEngine, error) {
    policyContent, err := os.ReadFile(policyPath)
    if err != nil {
        return nil, fmt.Errorf("failed to read Rego policy file: %w", err)
    }

    query, err := rego.New(
        rego.Query("data.kubernaut.gateway.priority.priority"),
        rego.Module("priority.rego", string(policyContent)),
    ).PrepareForEval(context.Background())
    // ...
}
```

### AIAnalysis Implementation

```go
// internal/controller/aianalysis/approval_policy.go

package aianalysis

import (
    "context"
    "fmt"
    "os"

    "github.com/go-logr/logr"
    "github.com/open-policy-agent/opa/v1/rego"
)

// ApprovalDecision represents the Rego policy output
type ApprovalDecision string

const (
    ApprovalDecisionApproved       ApprovalDecision = "approved"
    ApprovalDecisionManualReview   ApprovalDecision = "manual_review_required"
    ApprovalDecisionDenied         ApprovalDecision = "denied"
)

// ApprovalPolicyEngine evaluates Rego policies for remediation approval
//
// Design Decision: DD-AIANALYSIS-001
// Pattern: Follows Gateway PriorityEngine pattern
//
// Approval decisions affect:
// - Auto-approved: WorkflowExecution created immediately
// - Manual review: RO notifies operators (V1.0) or creates ApprovalRequest (V1.1)
// - Denied: Remediation stopped, operators notified
type ApprovalPolicyEngine struct {
    regoQuery *rego.PreparedEvalQuery
    logger    logr.Logger
}

// ApprovalInput is the input structure for Rego policy evaluation
// ADR-041: Rego receives pre-fetched data, no external API calls
type ApprovalInput struct {
    // AI Recommendation context
    Recommendation struct {
        WorkflowID     string  `json:"workflow_id"`
        Confidence     float64 `json:"confidence"`
        RiskAssessment string  `json:"risk_assessment"` // "low", "medium", "high"
        Severity       string  `json:"severity"`        // "critical", "high", "medium", "low"
    } `json:"recommendation"`

    // Business context (from SignalProcessing enrichment)
    BusinessContext struct {
        Environment      string `json:"environment"`       // "production", "staging", "development"
        Priority         string `json:"priority"`          // "P0", "P1", "P2", "P3"
        RiskTolerance    string `json:"risk_tolerance"`    // "low", "medium", "high"
        BusinessCategory string `json:"business_category"` // "critical", "standard", "background"
    } `json:"business_context"`

    // Workflow metadata (from workflow catalog)
    WorkflowMetadata struct {
        Version         string   `json:"version"`
        SafetyLevel     string   `json:"safety_level"`     // "safe", "moderate", "risky"
        RequiresApproval bool    `json:"requires_approval"`
        AllowedEnvs     []string `json:"allowed_envs"`
    } `json:"workflow_metadata"`

    // Historical context (optional - for success rate fallback)
    HistoricalContext struct {
        PreviousSuccessRate float64 `json:"previous_success_rate"` // 0.0-1.0
        TotalAttempts       int     `json:"total_attempts"`
    } `json:"historical_context"`

    // Recovery context (if this is a recovery attempt)
    RecoveryContext struct {
        IsRecoveryAttempt     bool   `json:"is_recovery_attempt"`
        RecoveryAttemptNumber int    `json:"recovery_attempt_number"`
        PreviousFailureReason string `json:"previous_failure_reason"` // Kubernetes reason code
    } `json:"recovery_context"`
}

// NewApprovalPolicyEngine creates a new approval policy engine
//
// Parameters:
//   - policyPath: Path to Rego policy file (typically mounted from ConfigMap)
//   - logger: Structured logger
//
// ConfigMap Deployment Pattern:
//   ConfigMap: ai-approval-policies (namespace: kubernaut-system)
//   Mount Path: /etc/kubernaut/policies/approval.rego
func NewApprovalPolicyEngine(policyPath string, logger logr.Logger) (*ApprovalPolicyEngine, error) {
    policyContent, err := os.ReadFile(policyPath)
    if err != nil {
        return nil, fmt.Errorf("failed to read Rego policy file: %w", err)
    }

    // Prepare Rego query
    // Query path: data.kubernaut.aianalysis.approval.decision
    query, err := rego.New(
        rego.Query("data.kubernaut.aianalysis.approval.decision"),
        rego.Module("approval.rego", string(policyContent)),
    ).PrepareForEval(context.Background())

    if err != nil {
        return nil, fmt.Errorf("failed to prepare Rego policy: %w", err)
    }

    logger.Info("Rego policy loaded successfully for approval decisions",
        "policy_path", policyPath,
    )

    return &ApprovalPolicyEngine{
        regoQuery: &query,
        logger:    logger,
    }, nil
}

// Evaluate evaluates the approval policy with the given input
//
// Returns:
//   - ApprovalDecision: "approved", "manual_review_required", or "denied"
//   - error: Evaluation errors (policy failure, invalid input, etc.)
//
// Fallback Behavior:
//   If Rego evaluation fails, returns "manual_review_required" (safe default)
func (e *ApprovalPolicyEngine) Evaluate(ctx context.Context, input ApprovalInput) (ApprovalDecision, error) {
    results, err := e.regoQuery.Eval(ctx, rego.EvalInput(input))
    if err != nil {
        e.logger.Error(err, "Rego policy evaluation failed, using safe default",
            "workflow_id", input.Recommendation.WorkflowID)
        return ApprovalDecisionManualReview, nil // Safe default
    }

    if len(results) == 0 || len(results[0].Expressions) == 0 {
        e.logger.Error(nil, "Rego evaluation returned no results, using safe default",
            "workflow_id", input.Recommendation.WorkflowID)
        return ApprovalDecisionManualReview, nil // Safe default
    }

    decision, ok := results[0].Expressions[0].Value.(string)
    if !ok {
        return ApprovalDecisionManualReview, fmt.Errorf(
            "rego evaluation returned non-string: %T", results[0].Expressions[0].Value)
    }

    // Validate decision value
    switch ApprovalDecision(decision) {
    case ApprovalDecisionApproved, ApprovalDecisionManualReview, ApprovalDecisionDenied:
        e.logger.V(1).Info("Approval decision evaluated",
            "workflow_id", input.Recommendation.WorkflowID,
            "decision", decision,
            "confidence", input.Recommendation.Confidence,
            "environment", input.BusinessContext.Environment,
        )
        return ApprovalDecision(decision), nil
    default:
        return ApprovalDecisionManualReview, fmt.Errorf(
            "rego returned invalid decision: %s (expected approved/manual_review_required/denied)", decision)
    }
}
```

### Rego Policy Example

> **Note (#225)**: The confidence threshold is now configurable via `input.confidence_threshold`.
> The Rego policy defines a built-in default (e.g., 0.8) that operators can override
> by setting `rego.confidenceThreshold` in the AIAnalysis controller config. Rego policies
> should use the `confidence_threshold` variable instead of hardcoding threshold values.
> See the production policy (`config/rego/aianalysis/approval.rego`) for the canonical pattern.

```rego
# ConfigMap: ai-approval-policies
# Namespace: kubernaut-system
# Mount Path: /etc/kubernaut/policies/approval.rego

package kubernaut.aianalysis.approval

import rego.v1

# Default decision: require manual review
default decision := "manual_review_required"

# #225: Configurable confidence threshold — operators override via input.confidence_threshold
default confidence_threshold := 0.8

confidence_threshold := input.confidence_threshold if {
    input.confidence_threshold
}

# ============================================================
# AUTO-APPROVED: High confidence in non-production
# ============================================================
decision := "approved" if {
    input.recommendation.confidence >= confidence_threshold
    input.business_context.environment != "production"
    input.workflow_metadata.safety_level == "safe"
}

# AUTO-APPROVED: High confidence + high risk tolerance in production
decision := "approved" if {
    input.recommendation.confidence >= 0.95
    input.business_context.environment == "production"
    input.business_context.risk_tolerance == "high"
    input.workflow_metadata.safety_level in ["safe", "moderate"]
}

# AUTO-APPROVED: Recovery attempt with safe workflow
decision := "approved" if {
    input.recovery_context.is_recovery_attempt == true
    input.recovery_context.recovery_attempt_number <= 2
    input.workflow_metadata.safety_level == "safe"
    input.recommendation.confidence >= 0.80
}

# ============================================================
# MANUAL REVIEW REQUIRED: Medium confidence or risky workflows
# ============================================================
decision := "manual_review_required" if {
    input.recommendation.confidence >= 0.70
    input.recommendation.confidence < 0.8
    input.business_context.environment == "production"
}

decision := "manual_review_required" if {
    input.workflow_metadata.safety_level == "risky"
    input.business_context.environment == "production"
}

decision := "manual_review_required" if {
    input.workflow_metadata.requires_approval == true
}

# ============================================================
# DENIED: Low confidence or forbidden scenarios
# ============================================================
decision := "denied" if {
    input.recommendation.confidence < 0.50
}

decision := "denied" if {
    input.business_context.environment == "production"
    not env_allowed(input.business_context.environment)
}

decision := "denied" if {
    input.recovery_context.is_recovery_attempt == true
    input.recovery_context.recovery_attempt_number > 3
}

# ============================================================
# HELPER RULES
# ============================================================
env_allowed(env) if {
    env in input.workflow_metadata.allowed_envs
}

env_allowed(env) if {
    count(input.workflow_metadata.allowed_envs) == 0  # No restrictions
}
```

### Kubernetes Deployment

```yaml
# ConfigMap containing Rego policy
apiVersion: v1
kind: ConfigMap
metadata:
  name: ai-approval-policies
  namespace: kubernaut-system
data:
  approval.rego: |
    package kubernaut.aianalysis.approval

    import rego.v1

    default decision := "manual_review_required"

    # ... policy rules ...
---
# AIAnalysis Controller Deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aianalysis-controller
  namespace: kubernaut-system
spec:
  template:
    spec:
      containers:
      - name: controller
        image: kubernaut/aianalysis-controller:v1.0.0
        args:
        - --approval-policy-path=/etc/kubernaut/policies/approval.rego
        volumeMounts:
        - name: approval-policies
          mountPath: /etc/kubernaut/policies
          readOnly: true
      volumes:
      - name: approval-policies
        configMap:
          name: ai-approval-policies
```

## Alternatives Considered

### Alternative A: Embedded Policies (Rejected)
**Approach**: Compile Rego policies into the controller binary

**Pros**:
- ✅ No external dependencies
- ✅ Faster startup

**Cons**:
- ❌ Requires redeployment to change policies
- ❌ No per-environment customization
- ❌ Doesn't follow Gateway pattern

**Confidence**: 40% (rejected)

### Alternative B: OPA Server (Rejected)
**Approach**: Deploy OPA as a separate service, AIAnalysis calls OPA API

**Pros**:
- ✅ Centralized policy management
- ✅ Policy hot-reload

**Cons**:
- ❌ Additional infrastructure complexity
- ❌ Network latency for each evaluation
- ❌ Single point of failure
- ❌ Overkill for V1.0 (single policy)

**Confidence**: 50% (rejected for V1.0, may revisit for V2)

### Alternative C: ConfigMap File Mount (Approved)
**Approach**: Mount ConfigMap as file, load at controller startup

**Pros**:
- ✅ Consistent with Gateway pattern
- ✅ No additional infrastructure
- ✅ Policy changes via ConfigMap update + pod restart
- ✅ Per-namespace customization possible

**Cons**:
- ⚠️ Requires pod restart for policy changes
  - **Mitigation**: Use rolling deployments

**Confidence**: 95% (approved)

## Consequences

### Positive
- ✅ Consistent pattern across Kubernaut services
- ✅ No additional infrastructure dependencies
- ✅ Policies are GitOps-friendly (ConfigMaps)
- ✅ Easy to test (unit tests for Rego policies)

### Negative
- ⚠️ Pod restart required for policy changes
  - **Mitigation**: Rolling deployment strategy
- ⚠️ No real-time policy hot-reload
  - **Mitigation**: Acceptable for V1.0, revisit if needed

### Neutral
- 🔄 OPA library dependency in controller
- 🔄 ConfigMap management required

## Integration with AIAnalysis Controller

### Controller Initialization

```go
func (r *AIAnalysisReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Load approval policy engine
    policyPath := os.Getenv("APPROVAL_POLICY_PATH")
    if policyPath == "" {
        policyPath = "/etc/kubernaut/policies/approval.rego"
    }

    engine, err := NewApprovalPolicyEngine(policyPath, r.Log)
    if err != nil {
        return fmt.Errorf("failed to initialize approval policy engine: %w", err)
    }
    r.approvalPolicyEngine = engine

    return ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha1.AIAnalysis{}).
        Complete(r)
}
```

### Reconciliation Usage

```go
func (r *AIAnalysisReconciler) reconcileAnalyzing(ctx context.Context, analysis *v1alpha1.AIAnalysis) (ctrl.Result, error) {
    // ... get HolmesGPT recommendation ...

    // Build approval input from enriched context
    input := ApprovalInput{
        Recommendation: struct{...}{
            WorkflowID:     recommendation.WorkflowID,
            Confidence:     recommendation.Confidence,
            RiskAssessment: recommendation.RiskAssessment,
            Severity:       recommendation.Severity,
        },
        BusinessContext: struct{...}{
            Environment:      analysis.Spec.EnrichmentResults.KubernetesContext.Environment,
            Priority:         analysis.Spec.EnrichmentResults.Priority,
            RiskTolerance:    analysis.Spec.EnrichmentResults.RiskTolerance,
            BusinessCategory: analysis.Spec.EnrichmentResults.BusinessCategory,
        },
        // ... populate other fields ...
    }

    // Evaluate Rego policy
    decision, err := r.approvalPolicyEngine.Evaluate(ctx, input)
    if err != nil {
        return ctrl.Result{}, fmt.Errorf("approval policy evaluation failed: %w", err)
    }

    // Update AIAnalysis status with decision
    analysis.Status.ApprovalDecision = string(decision)
    analysis.Status.ApprovalReason = r.buildApprovalReason(decision, input)

    // Transition based on decision
    switch decision {
    case ApprovalDecisionApproved:
        analysis.Status.Phase = "completed"
        analysis.Status.ApprovalStatus = "auto_approved"
    case ApprovalDecisionManualReview:
        analysis.Status.Phase = "completed"
        analysis.Status.ApprovalStatus = "manual_review_required"
    case ApprovalDecisionDenied:
        analysis.Status.Phase = "completed"
        analysis.Status.ApprovalStatus = "denied"
    }

    return ctrl.Result{}, r.Status().Update(ctx, analysis)
}
```

## Related Decisions

| Decision | Relationship |
|----------|-------------|
| DD-GATEWAY-XXX | Pattern source - Gateway PriorityEngine |
| ADR-041 | Alignment - Rego receives pre-fetched data |
| DD-RECOVERY-002 | Integration - Recovery context in approval input |

## Validation Checklist

- [ ] ApprovalPolicyEngine implemented following Gateway pattern
- [ ] ApprovalInput struct matches Rego policy input schema
- [ ] Rego policy examples created and tested
- [ ] ConfigMap deployment manifest created
- [ ] Controller initialization updated
- [ ] Unit tests for policy evaluation
- [ ] Integration tests for approval flow

