# DD-CONTRACT-001: AIAnalysis ↔ WorkflowExecution Contract Alignment

**Status**: ✅ Approved
**Version**: 1.2
**Date**: 2025-11-28
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
| **BR-AI-075** | Workflow Selection Output Format |
| **BR-AI-076** | Approval Context for Low Confidence |
| **BR-ORCH-025** | Catalog Lookup Before WorkflowExecution |
| **BR-ORCH-026** | Approval Orchestration |

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
| 1.2 | 2025-11-28 | **BREAKING**: HolmesGPT-API now resolves `workflow_id → containerImage` during MCP search. Added `containerImage` and `containerDigest` to `SelectedWorkflow`. RO no longer calls catalog - passes through from AIAnalysis. Updated data flow diagram. Removed catalog client code. |
| 1.1 | 2025-11-28 | **Approval flow clarification**: AIAnalysis completes with `approvalRequired: true`, RO orchestrates approval (creates NotificationRequest + RemediationApprovalRequest). Removed "Approving" phase from AIAnalysis. Added approval flow diagram and service responsibilities. |
| 1.0 | 2025-11-28 | Initial DD: AIAnalysis ↔ WorkflowExecution contract alignment |

