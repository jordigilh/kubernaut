# DD-RECOVERY-002: Direct AIAnalysis Recovery Flow

## Status
**✅ APPROVED** (2025-11-29)
**Supersedes**: DD-001 (Recovery Context Enrichment)
**Confidence**: 95%

## Context & Problem

When a WorkflowExecution fails, the system must initiate a recovery attempt. The original design (DD-001) proposed creating a new SignalProcessing CRD for recovery to re-enrich the context. However, this introduces unnecessary overhead:

1. **Redundant Enrichment**: The signal hasn't changed—only the execution failed
2. **Stale RCA**: Re-running SignalProcessing would re-categorize a signal that's already been analyzed
3. **Lost Failure Context**: The critical information is WHAT failed and WHY, not the original signal

**Key Insight**: Recovery is fundamentally different from initial incident handling. The AI needs to know what was **tried and failed**, not re-discover the original problem.

## Decision

**For recovery scenarios, Remediation Orchestrator creates AIAnalysis CRD directly**, skipping SignalProcessing.

### Flow Comparison

#### Initial Incident Flow (Unchanged)
```
Signal → Gateway → RemediationRequest
    ↓
SignalProcessing (enriches)
    ↓
AIAnalysis (RCA + workflow selection via /incident/analyze)
    ↓
WorkflowExecution
```

#### Recovery Flow (NEW - Direct to AIAnalysis)
```
WorkflowExecution FAILS
    ↓
RO detects failure, extracts failure context
    ↓
AIAnalysis (recovery) ← RO creates directly with:
  - Original enriched context (from first SignalProcessing)
  - Failure context (from failed WorkflowExecution)
  - Recovery attempt number
    ↓
HolmesGPT /recovery/analyze endpoint
    ↓
New WorkflowExecution (alternative approach)
```

## Rationale

### Why Skip SignalProcessing for Recovery?

| Concern | SignalProcessing (Initial) | Recovery (Direct to AIAnalysis) |
|---------|---------------------------|--------------------------------|
| **Signal State** | Unknown, needs categorization | Already categorized |
| **Context** | Needs enrichment | Already enriched (reuse) |
| **Kubernetes Context** | Fresh from cluster | Fresh from cluster (AIAnalysis gets current state) |
| **Historical Context** | N/A | Previous attempt is the history |
| **AI Goal** | Understand the problem | Understand why solution failed |

### Single Responsibility Principle

- **SignalProcessing**: Categorizes and enriches NEW signals
- **AIAnalysis**: Performs AI analysis (initial RCA OR recovery analysis)
- **WorkflowExecution**: Executes workflows

Recovery doesn't introduce a new signal—it's the SAME signal with additional context about what didn't work.

## Implementation

### AIAnalysis CRD Schema Extension

```go
type AIAnalysisSpec struct {
    // ... existing fields ...

    // Recovery fields (populated by RO when IsRecoveryAttempt=true)
    IsRecoveryAttempt     bool               `json:"isRecoveryAttempt,omitempty"`
    RecoveryAttemptNumber int                `json:"recoveryAttemptNumber,omitempty"`

    // SLICE of ALL previous executions (allows LLM to see complete history)
    // Ordered chronologically: index 0 = first attempt, last index = most recent
    // LLM can: avoid repeating failures, learn from patterns, retry earlier approaches
    PreviousExecutions    []PreviousExecution `json:"previousExecutions,omitempty"`

    // Enriched context (copied from original SignalProcessing)
    // Used for BOTH initial and recovery attempts
    EnrichmentResults     EnrichmentResults `json:"enrichmentResults"`
}

// PreviousExecution contains structured failure context
// Uses Kubernetes reason codes as API contract
type PreviousExecution struct {
    // Reference to failed WorkflowExecution
    WorkflowExecutionRef string `json:"workflowExecutionRef"`

    // Original RCA from initial AIAnalysis
    OriginalRCA RCAResult `json:"originalRCA"`

    // Selected workflow that was executed
    SelectedWorkflow SelectedWorkflowSummary `json:"selectedWorkflow"`

    // Structured failure information (Kubernetes reason codes)
    Failure ExecutionFailure `json:"failure"`
}

// RCAResult summarizes the original root cause analysis
type RCAResult struct {
    Summary             string   `json:"summary"`
    Severity            string   `json:"severity"`
    SignalType          string   `json:"signalType"`          // What RCA determined
    ContributingFactors []string `json:"contributingFactors"`
}

// SelectedWorkflowSummary describes what was attempted
type SelectedWorkflowSummary struct {
    WorkflowID     string            `json:"workflowId"`
    Version        string            `json:"version"`
    ContainerImage string            `json:"containerImage"`
    Parameters     map[string]string `json:"parameters"`
    Rationale      string            `json:"rationale"`
}

// ExecutionFailure uses Kubernetes reason codes as structured contract
type ExecutionFailure struct {
    // Which step failed (0-indexed)
    FailedStepIndex int    `json:"failedStepIndex"`
    FailedStepName  string `json:"failedStepName"`

    // Kubernetes reason code (structured - NOT natural language)
    // Examples: "OOMKilled", "DeadlineExceeded", "ImagePullBackOff",
    //           "InsufficientCPU", "FailedMount", "Unauthorized"
    Reason string `json:"reason"`

    // Human-readable message (for logging/debugging only)
    Message string `json:"message"`

    // Exit code if applicable
    ExitCode *int32 `json:"exitCode,omitempty"`

    // Timing
    FailedAt      metav1.Time `json:"failedAt"`
    ExecutionTime string      `json:"executionTime"` // e.g., "2m34s"
}
```

### AIAnalysis Controller Decision Logic

```go
func (r *AIAnalysisReconciler) callHolmesGPT(ctx context.Context, analysis *v1alpha1.AIAnalysis) (*HolmesGPTResponse, error) {
    if analysis.Spec.IsRecoveryAttempt {
        // Recovery: Call /recovery/analyze with ALL previous execution history
        return r.holmesGPTClient.RecoveryAnalyze(ctx, &RecoveryRequest{
            IncidentID:            analysis.Spec.IncidentID,
            RemediationID:         analysis.Spec.RemediationID,
            EnrichmentResults:     analysis.Spec.EnrichmentResults,
            PreviousExecutions:    analysis.Spec.PreviousExecutions, // Full history
            RecoveryAttemptNumber: analysis.Spec.RecoveryAttemptNumber,
        })
    }

    // Initial: Call /incident/analyze
    return r.holmesGPTClient.IncidentAnalyze(ctx, &IncidentRequest{
        IncidentID:        analysis.Spec.IncidentID,
        RemediationID:     analysis.Spec.RemediationID,
        EnrichmentResults: analysis.Spec.EnrichmentResults,
    })
}
```

### Remediation Orchestrator Recovery Logic

```go
func (r *RemediationOrchestratorReconciler) handleWorkflowFailure(
    ctx context.Context,
    rr *v1alpha1.RemediationRequest,
    failedWE *v1alpha1.WorkflowExecution,
    originalAI *v1alpha1.AIAnalysis,
    originalSP *v1alpha1.SignalProcessing,
) error {
    // Check retry limit
    attemptNumber := failedWE.Spec.AttemptNumber + 1
    if attemptNumber > r.maxRecoveryAttempts {
        return r.notifyAndStop(ctx, rr, "max recovery attempts exceeded")
    }

    // Create AIAnalysis directly (skip SignalProcessing)
    recoveryAI := &v1alpha1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-recovery-%d", rr.Name, attemptNumber),
            Namespace: rr.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(rr, v1alpha1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: v1alpha1.AIAnalysisSpec{
            RemediationID: rr.Name,
            IncidentID:    originalAI.Spec.IncidentID,

            // Recovery flags
            IsRecoveryAttempt:     true,
            RecoveryAttemptNumber: attemptNumber,

            // Reuse original enriched context
            EnrichmentResults: originalSP.Status.EnrichmentResults,

            // Build COMPLETE history of all previous executions
            // This gives LLM full context to avoid repeating failures
            PreviousExecutions: r.buildExecutionHistory(originalAI, failedWE),
        },
    }

    return r.Create(ctx, recoveryAI)
}

// buildExecutionHistory creates a chronologically ordered slice of all previous attempts
// Index 0 = first attempt, last index = most recent failed attempt
func (r *RemediationOrchestratorReconciler) buildExecutionHistory(
    originalAI *v1alpha1.AIAnalysis,
    failedWE *v1alpha1.WorkflowExecution,
) []v1alpha1.PreviousExecution {
    // Start with any existing history from previous recovery AIAnalysis
    var history []v1alpha1.PreviousExecution
    if originalAI.Spec.IsRecoveryAttempt && len(originalAI.Spec.PreviousExecutions) > 0 {
        history = append(history, originalAI.Spec.PreviousExecutions...)
    }

    // Append the current failed execution
    history = append(history, v1alpha1.PreviousExecution{
        WorkflowExecutionRef: failedWE.Name,
        OriginalRCA: v1alpha1.RCAResult{
            Summary:             originalAI.Status.RootCauseAnalysis.Summary,
            Severity:            originalAI.Status.RootCauseAnalysis.Severity,
            SignalType:          originalAI.Status.RootCauseAnalysis.SignalType,
            ContributingFactors: originalAI.Status.RootCauseAnalysis.ContributingFactors,
        },
        SelectedWorkflow: v1alpha1.SelectedWorkflowSummary{
            WorkflowID:     failedWE.Spec.WorkflowID,
            Version:        failedWE.Spec.Version,
            ContainerImage: failedWE.Spec.ContainerImage,
            Parameters:     failedWE.Spec.Parameters,
            Rationale:      originalAI.Status.SelectedWorkflow.Rationale,
        },
        Failure: extractFailureContext(failedWE),
    })

    return history
}

func extractFailureContext(we *v1alpha1.WorkflowExecution) v1alpha1.ExecutionFailure {
    failedStep := we.Status.FailedStep
    return v1alpha1.ExecutionFailure{
        FailedStepIndex: failedStep.Index,
        FailedStepName:  failedStep.Name,
        Reason:          failedStep.Reason,   // Kubernetes reason code
        Message:         failedStep.Message,
        ExitCode:        failedStep.ExitCode,
        FailedAt:        failedStep.FailedAt,
        ExecutionTime:   we.Status.Duration,
    }
}
```

## Recovery Prompt Design

See: [DD-RECOVERY-003-recovery-prompt-design.md](DD-RECOVERY-003-recovery-prompt-design.md)

**Key Principles**:
1. **Reuse incident prompt structure** - Consistency for the LLM
2. **Add "Previous Attempt" section** - What was tried, what failed
3. **Expect signal type may have changed** - Workflow execution may have altered cluster state
4. **Start from failure point** - Don't re-investigate the original problem
5. **Use Kubernetes reason codes** - Structured contract, not natural language

## Consequences

### Positive
- ✅ **Faster recovery**: Skips redundant SignalProcessing (~1 minute saved)
- ✅ **Better AI context**: LLM knows exactly what was tried and why it failed
- ✅ **Clear separation**: SignalProcessing = new signals, AIAnalysis = AI analysis
- ✅ **Structured failure data**: Kubernetes reason codes provide reliable contract
- ✅ **Immutable audit trail**: Each AIAnalysis CRD captures complete attempt context

### Negative
- ⚠️ AIAnalysis CRD spec grows larger for recovery scenarios
  - **Mitigation**: Fields are optional, only populated for recovery
- ⚠️ RO must maintain references to original SignalProcessing and AIAnalysis
  - **Mitigation**: Owner references provide automatic lookup

### Neutral
- 🔄 Two HolmesGPT endpoints: `/incident/analyze` and `/recovery/analyze`
- 🔄 AIAnalysis controller has branching logic based on `IsRecoveryAttempt`

## Related Decisions

| Decision | Relationship |
|----------|-------------|
| DD-001 | **SUPERSEDED** by this decision for recovery flow |
| DD-CONTRACT-002 | Aligned - uses same enrichment structures |
| ADR-041 | Extended - recovery prompt follows same contract |
| DD-WORKFLOW-002 | Aligned - remediation_id correlation maintained |

## Validation Checklist

- [ ] AIAnalysis CRD schema updated with recovery fields
- [ ] HolmesGPT-API RecoveryRequest model updated
- [ ] Recovery prompt design implemented
- [ ] RO recovery logic implemented
- [ ] Integration tests for recovery flow
- [ ] E2E tests for recovery scenarios

## Review Triggers

Revisit this decision if:
- Recovery success rate drops below 70%
- LLM struggles to differentiate initial vs recovery context
- Kubernetes reason codes prove insufficient for failure classification
- V2 introduces multi-attempt parallel recovery

