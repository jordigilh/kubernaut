# DD-CONTRACT-001: AIAnalysis ↔ WorkflowExecution Contract Alignment

**Status**: ✅ Approved
**Version**: 1.3
**Date**: 2025-12-01
**Confidence**: 98%

---

## Context

The AIAnalysis and WorkflowExecution services have evolved independently, creating a contract misalignment between:

1. **ADR-041**: LLM Response Contract (defines `selected_workflow` with `workflow_id` + `parameters`)
2. **ADR-043**: Workflow Schema Definition (defines workflow catalog schema)
3. **Current CRD Schemas**: AIAnalysis uses `recommendations[].action`, not `workflow_id`

This DD aligns all schemas with the authoritative LLM contract (ADR-041).

---

## Problem Statement

**Current AIAnalysis.Status** (misaligned):
```yaml
recommendations:
- id: "rec-001"
  action: "increase-memory-limit"  # ❌ Not aligned with ADR-041
  parameters:
    newMemoryLimit: "1Gi"
```

**ADR-041 LLM Response** (authoritative):
```json
{
  "selected_workflow": {
    "workflow_id": "oomkill-increase-memory",
    "version": "1.0.0",
    "confidence": 0.95,
    "parameters": {
      "NAMESPACE": "production",
      "DEPLOYMENT_NAME": "payment-service"
    }
  }
}
```

---

## Decision

Align all CRD schemas with ADR-041 and ADR-043.

### 1. AIAnalysis CRD Status

**Updated Schema**:
```go
// pkg/api/aianalysis/v1alpha1/types.go
package v1alpha1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AIAnalysisStatus defines the observed state of AIAnalysis
type AIAnalysisStatus struct {
    // Phase tracks current analysis stage
    // NOTE: AIAnalysis does NOT have "Approving" phase - it completes with approvalRequired=true
    // RemediationOrchestrator handles the approval orchestration (ADR-040)
    // +kubebuilder:validation:Enum=Pending;Investigating;Analyzing;Completed;Failed
    Phase string `json:"phase"`

    // PhaseTransitions records timestamps for each phase
    PhaseTransitions map[string]metav1.Time `json:"phaseTransitions,omitempty"`

    // InvestigationResult contains HolmesGPT investigation findings
    InvestigationResult *InvestigationResult `json:"investigationResult,omitempty"`

    // SelectedWorkflow contains the LLM-selected workflow (per ADR-041)
    // Populated after successful investigation and analysis
    SelectedWorkflow *SelectedWorkflow `json:"selectedWorkflow,omitempty"`

    // AlternativeWorkflows contains backup options (per ADR-041)
    AlternativeWorkflows []AlternativeWorkflow `json:"alternativeWorkflows,omitempty"`

    // ApprovalRequired indicates if manual approval is needed
    // Triggers RemediationOrchestrator to create RemediationApprovalRequest (ADR-040)
    ApprovalRequired bool `json:"approvalRequired,omitempty"`

    // ApprovalReason explains why approval is required
    ApprovalReason string `json:"approvalReason,omitempty"`

    // WorkflowExecutionRef references the created WorkflowExecution CRD
    WorkflowExecutionRef *ObjectReference `json:"workflowExecutionRef,omitempty"`

    // Conditions provide detailed status information
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// SelectedWorkflow represents the LLM's workflow selection (per ADR-041)
// NOTE: containerImage and containerDigest are resolved by HolmesGPT-API during MCP search
// RO passes these through to WorkflowExecution (no separate catalog lookup needed)
type SelectedWorkflow struct {
    // WorkflowID is the catalog lookup key
    // +kubebuilder:validation:Required
    WorkflowID string `json:"workflowId"`

    // Version of the selected workflow
    Version string `json:"version,omitempty"`

    // ContainerImage is the OCI bundle reference (resolved by HolmesGPT-API from catalog)
    // +kubebuilder:validation:Required
    ContainerImage string `json:"containerImage"`

    // ContainerDigest for audit trail and reproducibility (resolved by HolmesGPT-API)
    ContainerDigest string `json:"containerDigest,omitempty"`

    // Confidence score from MCP search (0.0-1.0)
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=1
    Confidence float64 `json:"confidence"`

    // Parameters populated by LLM based on RCA (per DD-WORKFLOW-003)
    // Keys are UPPER_SNAKE_CASE per Tekton convention
    Parameters map[string]string `json:"parameters"`

    // Rationale explains why this workflow was selected
    Rationale string `json:"rationale"`
}

// AlternativeWorkflow represents backup workflow options
type AlternativeWorkflow struct {
    WorkflowID string  `json:"workflowId"`
    Version    string  `json:"version,omitempty"`
    Confidence float64 `json:"confidence"`
    Rationale  string  `json:"rationale,omitempty"`
}
```

### 2. WorkflowExecution CRD Spec

**Updated Schema**:
```go
// pkg/api/workflowexecution/v1alpha1/types.go
package v1alpha1

import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkflowExecutionSpec defines the desired state of WorkflowExecution
type WorkflowExecutionSpec struct {
    // RemediationRequestRef references the parent RemediationRequest CRD
    RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

    // WorkflowRef contains the workflow catalog reference
    // Resolved from AIAnalysis.Status.SelectedWorkflow
    WorkflowRef WorkflowRef `json:"workflowRef"`

    // Parameters from LLM selection (per DD-WORKFLOW-003)
    // Keys are UPPER_SNAKE_CASE for Tekton PipelineRun params
    Parameters map[string]string `json:"parameters"`

    // Confidence score from LLM (for audit trail)
    Confidence float64 `json:"confidence"`

    // Rationale from LLM (for audit trail)
    Rationale string `json:"rationale,omitempty"`

    // ExecutionStrategy specifies how to execute the workflow
    ExecutionStrategy ExecutionStrategy `json:"executionStrategy"`
}

// WorkflowRef contains catalog-resolved workflow reference
type WorkflowRef struct {
    // WorkflowID is the catalog lookup key
    WorkflowID string `json:"workflowId"`

    // Version of the workflow
    Version string `json:"version"`

    // ContainerImage resolved from workflow catalog (Data Storage API)
    // OCI bundle reference for Tekton PipelineRun
    ContainerImage string `json:"containerImage"`

    // ContainerDigest for audit trail and reproducibility
    ContainerDigest string `json:"containerDigest,omitempty"`
}
```

---

## Data Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         COMPLETE DATA FLOW                               │
│                  (HolmesGPT-API resolves containerImage)                 │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌──────────────────────┐                                               │
│  │  AIAnalysis CRD      │                                               │
│  │  spec.analysisReq    │──────► HolmesGPT-API                          │
│  └──────────────────────┘              │                                │
│                                        │                                │
│                          ┌─────────────┴─────────────┐                  │
│                          │                           │                  │
│                          ▼                           ▼                  │
│               ┌─────────────────────┐    ┌─────────────────────┐        │
│               │ Data Storage API    │    │ LLM Provider        │        │
│               │ (MCP Workflow Search)│    │ (Workflow Selection)│        │
│               │ Returns:            │    │ Returns:            │        │
│               │ - workflow_id       │    │ - workflow_id       │        │
│               │ - container_image   │    │ - parameters        │        │
│               │ - container_digest  │    │ - confidence        │        │
│               └──────────┬──────────┘    └──────────┬──────────┘        │
│                          │                           │                  │
│                          └─────────────┬─────────────┘                  │
│                                        │                                │
│                          HolmesGPT-API combines both                    │
│                                        ▼                                │
│  ┌──────────────────────────────────────────────────────────────┐       │
│  │  AIAnalysis.Status.SelectedWorkflow                          │       │
│  │  ├── workflowId: "oomkill-increase-memory"                   │       │
│  │  ├── version: "1.0.0"                                        │       │
│  │  ├── containerImage: "quay.io/kubernaut/oomkill:v1.0.0"     │  ◄── RESOLVED BY HolmesGPT-API
│  │  ├── containerDigest: "sha256:abc123..."                     │  ◄── RESOLVED BY HolmesGPT-API
│  │  ├── confidence: 0.95                                        │       │
│  │  ├── parameters:                                             │       │
│  │  │     NAMESPACE: "production"                               │       │
│  │  │     DEPLOYMENT_NAME: "payment-service"                    │       │
│  │  └── rationale: "MCP search matched OOMKilled signal..."     │       │
│  └────────────────────────┬─────────────────────────────────────┘       │
│                           │                                             │
│                           │ RemediationOrchestrator watches             │
│                           │ (NO catalog lookup needed - pass through)   │
│                           ▼                                             │
│  ┌──────────────────────────────────────────────────────────────┐       │
│  │  WorkflowExecution.Spec (RO passes through from AIAnalysis)  │       │
│  │  ├── workflowRef:                                            │       │
│  │  │     workflowId: "oomkill-increase-memory"                 │       │
│  │  │     version: "1.0.0"                                      │       │
│  │  │     containerImage: "quay.io/kubernaut/oomkill:v1.0.0"   │  ◄── PASS THROUGH
│  │  │     containerDigest: "sha256:abc123..."                   │  ◄── PASS THROUGH
│  │  ├── parameters:                                             │       │
│  │  │     NAMESPACE: "production"                               │       │
│  │  │     DEPLOYMENT_NAME: "payment-service"                    │       │
│  │  ├── confidence: 0.95                                        │       │
│  │  └── rationale: "MCP search matched..."                      │       │
│  └────────────────────────┬─────────────────────────────────────┘       │
│                           │                                             │
│                           │ WorkflowExecution Controller                │
│                           ▼                                             │
│  ┌──────────────────────────────────────────────────────────────┐       │
│  │  Tekton PipelineRun                                          │       │
│  │  ├── pipelineRef:                                            │       │
│  │  │     resolver: bundles                                     │       │
│  │  │     params:                                               │       │
│  │  │       - name: bundle                                      │       │
│  │  │         value: "quay.io/kubernaut/oomkill:v1.0.0"        │       │
│  │  └── params:                                                 │       │
│  │        - name: NAMESPACE                                     │       │
│  │          value: "production"                                 │       │
│  │        - name: DEPLOYMENT_NAME                               │       │
│  │          value: "payment-service"                            │       │
│  └──────────────────────────────────────────────────────────────┘       │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Service Responsibilities

| Service | Responsibility |
|---------|----------------|
| **AIAnalysis Controller** | Calls HolmesGPT-API, stores `SelectedWorkflow` (including containerImage) in status |
| **HolmesGPT-API** | Queries catalog for MCP search, calls LLM, **resolves workflow_id → containerImage** |
| **RemediationOrchestrator** | Watches AIAnalysis, orchestrates approval flow, **passes through** to WorkflowExecution |
| **Notification Controller** | Delivers approval notifications via Alertmanager routing (DD-NOTIFICATION-001) |
| **RemediationApprovalRequest Controller** | Manages approval lifecycle, timeout expiration (ADR-040) |
| **Data Storage Service** | Provides `/api/v1/workflows/search` for MCP (queried by HolmesGPT-API) |
| **WorkflowExecution Controller** | Uses `WorkflowRef.ContainerImage` to create Tekton PipelineRun |

### Key Design Decision (Approved 2025-11-28)

**HolmesGPT-API resolves `workflow_id → containerImage`** during MCP search. RO does NOT call the catalog - it passes through the resolved values from AIAnalysis.status.

**Rationale**:
1. HolmesGPT-API already queries catalog for MCP search - it has the data
2. Immutable workflows mean containerImage never changes for a given workflow_id+version
3. Simpler RO - no catalog client needed
4. Industry alignment (Temporal, Step Functions resolve at definition time)

---

## RO Pass-Through Logic

**RemediationOrchestrator** passes through the resolved workflow from AIAnalysis (no catalog lookup):

```go
// pkg/remediationorchestrator/reconciler.go
package remediationorchestrator

import (
    "context"

    kubernautv1alpha1 "github.com/jordigilh/kubernaut/api/v1alpha1"
    ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) createWorkflowExecution(
    ctx context.Context,
    aiAnalysis *kubernautv1alpha1.AIAnalysis,
    remediationRequest *kubernautv1alpha1.RemediationRequest,
) error {
    // Pass through - no catalog lookup needed
    // HolmesGPT-API already resolved containerImage during MCP search
    wfe := &kubernautv1alpha1.WorkflowExecution{
        ObjectMeta: ctrl.ObjectMeta{
            Name:      fmt.Sprintf("workflow-%s", remediationRequest.Name),
            Namespace: remediationRequest.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediationRequest, kubernautv1alpha1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: kubernautv1alpha1.WorkflowExecutionSpec{
            RemediationRequestRef: corev1.ObjectReference{
                Name:      remediationRequest.Name,
                Namespace: remediationRequest.Namespace,
            },
            WorkflowRef: kubernautv1alpha1.WorkflowRef{
                WorkflowID:      aiAnalysis.Status.SelectedWorkflow.WorkflowID,
                Version:         aiAnalysis.Status.SelectedWorkflow.Version,
                ContainerImage:  aiAnalysis.Status.SelectedWorkflow.ContainerImage,  // PASS THROUGH
                ContainerDigest: aiAnalysis.Status.SelectedWorkflow.ContainerDigest, // PASS THROUGH
            },
            Parameters: aiAnalysis.Status.SelectedWorkflow.Parameters, // PASS THROUGH
            Confidence: aiAnalysis.Status.SelectedWorkflow.Confidence,
            Rationale:  aiAnalysis.Status.SelectedWorkflow.Rationale,
        },
    }

    return r.Create(ctx, wfe)
}
```

---

## Recovery Data Flow (v1.3)

### Key Principle: RO Mediates All Communication

**AIAnalysis and WorkflowExecution have NO direct relationship.** The RemediationOrchestrator (RO) is the sole coordinator:

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                        RO AS THE ONLY COORDINATOR                             │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  RemediationRequest                                                          │
│        │                                                                     │
│        │ owns (ownerRef)                                                     │
│        ▼                                                                     │
│  ┌──────────────┐                     ┌────────────────────┐                │
│  │  AIAnalysis  │◀────── RO ─────────▶│ WorkflowExecution  │                │
│  └──────────────┘       creates       └────────────────────┘                │
│        │                 &                      │                            │
│        │               watches                  │                            │
│        │                                        │                            │
│        ▼                                        ▼                            │
│  status.selectedWorkflow              status.failureDetails                  │
│        │                                        │                            │
│        │                                        │                            │
│        └────────────────► RO ◄─────────────────┘                            │
│                           │                                                  │
│                           │ On WE failure:                                   │
│                           │ 1. Read WE.status.failureDetails                 │
│                           │ 2. Create NEW AIAnalysis with:                   │
│                           │    - isRecoveryAttempt=true                      │
│                           │    - previousExecutions[] populated              │
│                           │    - Natural language summary included           │
│                           │                                                  │
│  NO DIRECT LINK: AIAnalysis and WE never reference each other               │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

### WorkflowExecution Failure Output (v3.0)

When a workflow fails, WE populates `status.failureDetails`:

```yaml
# WorkflowExecution.status (on failure)
status:
  phase: Failed
  failureDetails:
    failedTaskIndex: 1
    failedTaskName: "apply-memory-increase"
    failedStepName: "kubectl-patch"
    reason: "Forbidden"                    # K8s-style reason code
    message: "RBAC denied: cannot patch deployments.apps"
    exitCode: 1
    failedAt: "2025-12-01T10:15:45Z"
    executionTimeBeforeFailure: "45s"
    naturalLanguageSummary: |
      Task 'apply-memory-increase' (step 2 of 3) failed after 45s with Forbidden error.
      The service account 'kubernaut-workflow-runner' lacks required RBAC permissions.
      Recommendation: Grant patch permission or use an alternative workflow.
```

### RO Recovery Flow

When RO detects `WorkflowExecution.Status.Phase == "Failed"`:

```go
// pkg/remediationorchestrator/reconciler.go
func (r *Reconciler) handleWorkflowExecutionFailed(
    ctx context.Context,
    rr *v1alpha1.RemediationRequest,
    we *v1alpha1.WorkflowExecution,
    originalAIA *v1alpha1.AIAnalysis,
) error {
    // Check max retry limit
    recoveryAttemptNum := len(originalAIA.Spec.PreviousExecutions) + 1
    if recoveryAttemptNum > rr.Spec.MaxRecoveryAttempts {
        // Mark as permanently failed
        return r.markRemediationFailed(ctx, rr, "Max recovery attempts exceeded")
    }

    // Build previous execution record from WE failure details
    prevExec := v1alpha1.PreviousExecution{
        WorkflowExecutionRef: we.Name,
        OriginalRCA: v1alpha1.OriginalRCA{
            Summary:    originalAIA.Status.RootCause,
            SignalType: originalAIA.Spec.AnalysisRequest.SignalContext.SignalType,
            Severity:   originalAIA.Spec.AnalysisRequest.SignalContext.Severity,
        },
        SelectedWorkflow: v1alpha1.SelectedWorkflowSummary{
            WorkflowID:     we.Spec.WorkflowRef.WorkflowID,
            Version:        we.Spec.WorkflowRef.Version,
            ContainerImage: we.Spec.WorkflowRef.ContainerImage,
            Rationale:      we.Spec.Rationale,
        },
        Failure: v1alpha1.ExecutionFailure{
            FailedStepIndex:   we.Status.FailureDetails.FailedTaskIndex,
            FailedStepName:    we.Status.FailureDetails.FailedTaskName,
            Reason:            we.Status.FailureDetails.Reason,
            Message:           we.Status.FailureDetails.Message,
            ExitCode:          we.Status.FailureDetails.ExitCode,
            FailedAt:          we.Status.FailureDetails.FailedAt,
            ExecutionTime:     we.Status.FailureDetails.ExecutionTimeBeforeFailure,
        },
        // Include natural language for LLM context
        NaturalLanguageSummary: we.Status.FailureDetails.NaturalLanguageSummary,
    }

    // Accumulate all previous executions (not just the last one)
    allPreviousExecutions := append(originalAIA.Spec.PreviousExecutions, prevExec)

    // Create recovery AIAnalysis
    recoveryAIA := &v1alpha1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("aianalysis-recovery-%s-%d", rr.Name, recoveryAttemptNum),
            Namespace: rr.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(rr, v1alpha1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: v1alpha1.AIAnalysisSpec{
            RemediationRequestRef: corev1.ObjectReference{
                Name:      rr.Name,
                Namespace: rr.Namespace,
            },
            RemediationID:         rr.Name,
            AnalysisRequest:       originalAIA.Spec.AnalysisRequest, // Reuse enriched context
            IsRecoveryAttempt:     true,
            RecoveryAttemptNumber: recoveryAttemptNum,
            PreviousExecutions:    allPreviousExecutions,
        },
    }

    return r.Create(ctx, recoveryAIA)
}
```

### Data Contract: WE → RO → AIAnalysis

| WE Status Field | RO Mapping | AIAnalysis Spec Field |
|-----------------|------------|----------------------|
| `failureDetails.failedTaskIndex` | Direct | `previousExecutions[].failure.failedStepIndex` |
| `failureDetails.failedTaskName` | Direct | `previousExecutions[].failure.failedStepName` |
| `failureDetails.reason` | Direct | `previousExecutions[].failure.reason` |
| `failureDetails.message` | Direct | `previousExecutions[].failure.message` |
| `failureDetails.exitCode` | Direct | `previousExecutions[].failure.exitCode` |
| `failureDetails.failedAt` | Direct | `previousExecutions[].failure.failedAt` |
| `failureDetails.executionTimeBeforeFailure` | Direct | `previousExecutions[].failure.executionTime` |
| `failureDetails.naturalLanguageSummary` | Direct | `previousExecutions[].naturalLanguageSummary` |

### Failure Reason Codes

Standardized K8s-style reason codes for deterministic recovery decisions:

| Reason Code | Description | Recovery Hint |
|-------------|-------------|---------------|
| `OOMKilled` | Container killed due to memory limits | Increase task memory or use lighter workflow |
| `DeadlineExceeded` | Timeout reached | Increase timeout or use faster workflow |
| `Forbidden` | RBAC/permission failure | Grant permissions or use alternative workflow |
| `ResourceExhausted` | Cluster resource limits | Wait for resources or reduce resource requests |
| `ConfigurationError` | Invalid parameters | Fix parameters (should be caught by validation) |
| `ImagePullBackOff` | Cannot pull container image | Fix image reference or credentials |
| `Unknown` | Unclassified failure | Manual investigation required |

---

## Approval Integration (ADR-040, ADR-017, ADR-018)

### Key Principle: AIAnalysis Completes, RO Orchestrates

**AIAnalysis does NOT stay in "Approving" phase.** It completes its analysis and signals approval is needed via `approvalRequired: true`. The RemediationOrchestrator is responsible for orchestrating the approval workflow.

### AIAnalysis Completion (Low Confidence)

When `AIAnalysis.Status.SelectedWorkflow.Confidence < 0.80`:

```yaml
# AIAnalysis.Status after low-confidence selection
# NOTE: phase = "Completed", NOT "Approving"
status:
  phase: Completed              # ← AIAnalysis is DONE with its work
  selectedWorkflow:
    workflowId: "oomkill-increase-memory"
    confidence: 0.65            # Below 80% threshold
    parameters:
      NAMESPACE: "production"
  approvalRequired: true        # ← Signal for RO to orchestrate approval
  approvalReason: "Confidence 65% below 80% threshold"
  approvalContext:              # ← Rich context for operator decision
    investigationSummary: "Memory leak detected..."
    evidenceCollected:
      - "OOMKilled events in last 24h"
      - "Memory growth 50MB/hour"
    alternativesConsidered:
      - workflowId: "oomkill-restart-pods"
        confidence: 0.45
```

### RemediationOrchestrator Approval Flow

When RO detects `AIAnalysis.Status.approvalRequired == true`:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    RO APPROVAL ORCHESTRATION FLOW                        │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  1. AIAnalysis completes with approvalRequired: true                    │
│     ↓                                                                   │
│  2. RemediationOrchestrator watches AIAnalysis.status                   │
│     IF phase == "Completed" AND approvalRequired == true:               │
│     ↓                                                                   │
│  3. RO creates NotificationRequest CRD (per ADR-017/ADR-018)            │
│     → Notification Controller delivers to Slack/PagerDuty              │
│     → Operators receive approval request notification                   │
│     ↓                                                                   │
│  4. RO creates RemediationApprovalRequest CRD (per ADR-040)             │
│     → Sets spec.aiAnalysisRef.name                                      │
│     → Sets spec.requiredBy (timeout deadline, default 15m)              │
│     ↓                                                                   │
│  5. RemediationApprovalRequest Controller manages lifecycle             │
│     → Detects timeout expiration                                        │
│     → Updates status.decision on operator action                        │
│     ↓                                                                   │
│  6. RO watches RemediationApprovalRequest.status.decision               │
│     ├── "Approved" → Lookup catalog, create WorkflowExecution           │
│     ├── "Rejected" → Mark RemediationRequest as Rejected                │
│     └── "Expired"  → Mark RemediationRequest as Expired                 │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Service Responsibilities (Approval Flow)

| Service | Responsibility |
|---------|----------------|
| **AIAnalysis** | Complete analysis, set `approvalRequired: true`, populate `approvalContext` |
| **RemediationOrchestrator** | Create NotificationRequest + RemediationApprovalRequest, watch for decision |
| **Notification Controller** | Deliver approval notification to operators (Alertmanager routing) |
| **RemediationApprovalRequest Controller** | Manage approval lifecycle, timeout expiration |

### Why AIAnalysis Doesn't Stay in "Approving"

1. **Separation of Concerns**: AIAnalysis does AI analysis, RO does orchestration
2. **Clean Completion**: AIAnalysis has a clear terminal state (Completed/Failed)
3. **Reusability**: AIAnalysis doesn't need to know about approval mechanics
4. **Testability**: Each component can be tested independently

---

## Related Documents

| Document | Relationship |
|----------|--------------|
| **ADR-041** | LLM Response Contract (authoritative for `selected_workflow` format) |
| **ADR-043** | Workflow Schema Definition (authoritative for catalog schema) |
| **ADR-040** | RemediationApprovalRequest Architecture (approval lifecycle) |
| **ADR-017** | NotificationRequest Creator (RO creates notifications) |
| **ADR-018** | Approval Notification Integration (rich approval context) |
| **DD-NOTIFICATION-001** | Alertmanager Routing Reuse (notification channel routing) |
| **DD-TIMEOUT-001** | Global Remediation Timeout (approval timeout: 15m default) |
| **DD-WORKFLOW-003** | Parameterized Actions (UPPER_SNAKE_CASE parameters) |
| **DD-WORKFLOW-005** | Automated Schema Extraction (V1.0/V1.1 registration) |
| **DD-RECOVERY-002** | Direct AIAnalysis Recovery Flow |
| **DD-RECOVERY-003** | Recovery Prompt Design (K8s reason codes) |
| **BR-AI-075** | Workflow Selection Output Format |
| **BR-AI-076** | Approval Context for Low Confidence |
| **BR-ORCH-025** | Catalog Lookup Before WorkflowExecution |
| **BR-ORCH-026** | Approval Orchestration |
| **BR-WE-001** | Defense-in-Depth Parameter Validation |
| **BR-HAPI-260** | Primary Parameter Validation (HolmesGPT-API) |

---

## Migration Impact

| File | Change Required |
|------|-----------------|
| `api/aianalysis/v1alpha1/types.go` | Replace `Recommendations` with `SelectedWorkflow` |
| `api/workflowexecution/v1alpha1/types.go` | Add `WorkflowRef`, simplify `WorkflowDefinition` |
| `docs/services/crd-controllers/02-aianalysis/crd-schema.md` | Update examples |
| `docs/services/crd-controllers/03-workflowexecution/crd-schema.md` | Update examples |
| AIAnalysis implementation plan | Update to use new schema |
| WorkflowExecution implementation plan | Update to use `WorkflowRef` |

---

## Confidence Assessment

| Aspect | Confidence | Rationale |
|--------|------------|-----------|
| **ADR-041 alignment** | 98% | LLM contract tested and working |
| **Catalog integration** | 95% | DD-WORKFLOW-005 + ADR-043 define clear schema |
| **Approval integration** | 99% | ADR-040 already approved and well-designed |
| **Parameter format** | 95% | DD-WORKFLOW-003 defines UPPER_SNAKE_CASE |
| **Overall** | 95% | Strong foundation, clear contracts |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.3 | 2025-12-01 | **Recovery Data Flow**: Added comprehensive recovery flow documentation. WE now provides `status.failureDetails` with structured failure information (`failedTaskIndex`, `failedTaskName`, `reason` enum, `naturalLanguageSummary`). Documented RO as sole coordinator between AIAnalysis and WE (no direct relationship). Added K8s-style failure reason codes. Added WE→RO→AIAnalysis data mapping table. Added RO recovery logic code example. |
| 1.2 | 2025-11-28 | **BREAKING**: HolmesGPT-API now resolves `workflow_id → containerImage` during MCP search. Added `containerImage` and `containerDigest` to `SelectedWorkflow`. RO no longer calls catalog - passes through from AIAnalysis. Updated data flow diagram. Removed catalog client code. |
| 1.1 | 2025-11-28 | **Approval flow clarification**: AIAnalysis completes with `approvalRequired: true`, RO orchestrates approval (creates NotificationRequest + RemediationApprovalRequest). Removed "Approving" phase from AIAnalysis. Added approval flow diagram and service responsibilities. |
| 1.0 | 2025-11-28 | Initial DD: AIAnalysis ↔ WorkflowExecution contract alignment |

