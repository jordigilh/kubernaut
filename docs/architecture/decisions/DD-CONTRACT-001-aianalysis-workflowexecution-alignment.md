# DD-CONTRACT-001: AIAnalysis ↔ WorkflowExecution Contract Alignment

**Status**: ✅ Approved (core contract shape); ⚠️ **catalog-resolution mechanism corrected in v2.0**
**Version**: 2.0
**Date**: 2025-11-28 (v1.2); 2026-08-02 (v2.0 correction, [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806))
**Confidence**: 95%

> **v2.0 correction note**: This v1.2 document (Nov 2025) predates the current implementation and got one
> load-bearing detail wrong: it describes **"HolmesGPT-API"** resolving `workflow_id → containerImage` via a
> **Data Storage MCP search** during RCA. Neither exists today. The service is **Kubernaut Agent (KA)**, a
> native-Go rewrite, and KA owns workflow discovery **in-process** against `RemediationWorkflow`/`ActionType`
> CRDs (informer-backed cache, no HTTP/MCP call to Data Storage for this) — see
> [DD-WORKFLOW-019](DD-WORKFLOW-019-ka-owned-workflow-discovery.md) (authoritative, "Implemented" as of July
> 2026). The CRD field named `containerImage`/`containerDigest` below was also generalized and renamed to
> `executionBundle`/`executionBundleDigest` (Issue #1661 Change 11-12) once WorkflowExecution grew three
> execution engines (Tekton, native `batchv1.Job`, Ansible/AWX) instead of only Tekton. The remaining sections
> — the overall CRD contract shape, the RO pass-through principle, and the approval-orchestration flow — remain
> directionally correct and are corrected in place below rather than rewritten from scratch. For the
> `approvalRequired` vs. `needsHumanReview` two-flag decision logic (added after this document's v1.x), see
> [DD-CONTRACT-002](DD-CONTRACT-002-service-integration-contracts.md#contract-2-aianalysis--ro-status-output),
> which is kept as the single authority for that split to avoid duplicating it here.

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

**Real schema** (`api/aianalysis/v1alpha1/aianalysis_types.go`, API group `kubernaut.ai/v1alpha1` —
corrected from v1.2's `pkg/api/aianalysis/v1alpha1/types.go` / `kubernaut.io`; simplified/illustrative, see
source for the complete field list):

```go
// api/aianalysis/v1alpha1/aianalysis_types.go
package v1alpha1

// AIAnalysisStatus defines the observed state of AIAnalysis
type AIAnalysisStatus struct {
    // Phase tracks current analysis stage (no "Approving"/"Recommending" phase -
    // simplified 4-phase flow; RO handles approval orchestration, ADR-040)
    Phase string `json:"phase"`

    // RootCauseAnalysis contains RCA findings, including the LLM-determined
    // RemediationTarget (BR-KA-212) -- may differ from the signal's source resource
    RootCauseAnalysis *RootCauseAnalysis `json:"rootCauseAnalysis,omitempty"`

    // SelectedWorkflow contains the AI-selected workflow (DD-CONTRACT-002)
    // Immutable once SelectedAt is populated (Issue #1661, DD-WORKFLOW-018)
    SelectedWorkflow *SelectedWorkflow `json:"selectedWorkflow,omitempty"`

    // AlternativeWorkflows are informational only (context for approval decisions),
    // never used for automatic execution
    AlternativeWorkflows []AlternativeWorkflow `json:"alternativeWorkflows,omitempty"`

    // ApprovalRequired: Rego policy decision -- AI HAS an answer, policy requires approval
    ApprovalRequired bool `json:"approvalRequired"`

    // NeedsHumanReview: KA decision -- AI CAN'T answer (rca_incomplete,
    // no_matching_workflows, low_confidence). See DD-CONTRACT-002 for the
    // full two-flag decision logic.
    NeedsHumanReview bool `json:"needsHumanReview"`

    // WorkflowExecutionRef references the created WorkflowExecution CRD
    WorkflowExecutionRef *ObjectReference `json:"workflowExecutionRef,omitempty"`
}

// RootCauseAnalysis contains detailed RCA results
type RootCauseAnalysis struct {
    Summary             string             `json:"summary"`
    SignalType          string             `json:"signalType"`
    ContributingFactors []string           `json:"contributingFactors,omitempty"`
    // RemediationTarget: the resource the LLM determined should actually be
    // remediated (BR-KA-212) -- e.g. signal came from a Pod, target is its Deployment
    RemediationTarget   *RemediationTarget `json:"remediationTarget,omitempty"`
}

// SelectedWorkflow contains the AI-selected workflow for execution (DD-CONTRACT-002)
type SelectedWorkflow struct {
    // WorkflowSnapshot is the catalog-resolved execution snapshot, inline-embedded
    // so its field list can never drift from WorkflowExecution.Spec.WorkflowRef,
    // which embeds the identical type (Issue #1661 Change 12, DD-WORKFLOW-018).
    // Fields: WorkflowID, WorkflowName, ActionType, Version, ExecutionBundle
    // (OCI bundle ref -- renamed from "containerImage" once WorkflowExecution grew
    // 3 execution engines: Tekton/Job/Ansible), ExecutionBundleDigest, ExecutionEngine,
    // EngineConfig, ServiceAccountName, Dependencies, Resources, DeclaredParameterNames.
    sharedtypes.WorkflowSnapshot `json:",inline"`

    Confidence float64           `json:"confidence"`
    // Parameters: UPPER_SNAKE_CASE keys per DD-WORKFLOW-003
    Parameters map[string]string `json:"parameters,omitempty"`
    Rationale  string            `json:"rationale"`
}
```

### 2. WorkflowExecution CRD Spec

**Real schema** (`api/workflowexecution/v1alpha1/workflowexecution_types.go`):

```go
// api/workflowexecution/v1alpha1/workflowexecution_types.go
package v1alpha1

// WorkflowExecutionSpec defines the desired state of WorkflowExecution
type WorkflowExecutionSpec struct {
    RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

    // WorkflowRef: catalog-resolved reference, copied verbatim from
    // AIAnalysis.Status.SelectedWorkflow by RemediationOrchestrator (PASS THROUGH,
    // no re-fetch from DataStorage/KA catalog -- Issue #1661 Change 11d/11e)
    WorkflowRef WorkflowRef `json:"workflowRef"`

    Parameters map[string]string `json:"parameters"`
    Confidence float64           `json:"confidence"`
    Rationale  string            `json:"rationale,omitempty"`
}

// WorkflowRef is nothing but an inline embed of the same sharedtypes.WorkflowSnapshot
// type embedded in AIAnalysis.Status.SelectedWorkflow -- sharing one Go/CRD-schema type
// between both CRDs makes it structurally impossible for their field lists to drift
// (Issue #1661 Change 12). There is no separate "ContainerImage"/"ContainerDigest" pair
// here; ExecutionBundle/ExecutionBundleDigest live on the embedded WorkflowSnapshot.
type WorkflowRef struct {
    sharedtypes.WorkflowSnapshot `json:",inline"`
}
```

---

## Data Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         COMPLETE DATA FLOW                               │
│    (KA resolves ExecutionBundle via its own in-process workflow         │
│     discovery -- DD-WORKFLOW-019 -- NOT a Data Storage MCP search)      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌──────────────────────┐                                               │
│  │  AIAnalysis CRD      │                                               │
│  │  (async submit)      │──────► Kubernaut Agent (KA)                   │
│  └──────────────────────┘              │                                │
│                                        │                                │
│                          ┌─────────────┴─────────────┐                  │
│                          │                           │                  │
│                          ▼                           ▼                  │
│               ┌─────────────────────┐    ┌─────────────────────┐        │
│               │ KA in-process       │    │ LLM Provider        │        │
│               │ workflowcatalog     │    │ (Workflow Selection)│        │
│               │ (informer cache on  │    │ Returns:            │        │
│               │  RemediationWorkflow│    │ - workflow_id       │        │
│               │  /ActionType CRDs)  │    │ - parameters        │        │
│               │ Returns:            │    │ - confidence        │        │
│               │ - workflowId        │    │                     │        │
│               │ - executionBundle   │    │                     │        │
│               └──────────┬──────────┘    └──────────┬──────────┘        │
│                          │                           │                  │
│                          └─────────────┬─────────────┘                  │
│                                        │                                │
│                                KA combines both                         │
│                                        ▼                                │
│  ┌──────────────────────────────────────────────────────────────┐       │
│  │  AIAnalysis.Status.SelectedWorkflow (WorkflowSnapshot-embed) │       │
│  │  ├── workflowId: "<DS-assigned UUID>"                        │       │
│  │  ├── workflowName: "oomkill-increase-memory"                 │       │
│  │  ├── version: "1.0.0"                                        │       │
│  │  ├── executionBundle: "quay.io/kubernaut/oomkill:v1.0.0"    │  ◄── RESOLVED BY KA (in-process)
│  │  ├── executionBundleDigest: "sha256:abc123..."               │  ◄── RESOLVED BY KA (in-process)
│  │  ├── executionEngine: "tekton"                               │       │
│  │  ├── confidence: 0.95                                        │       │
│  │  ├── parameters:                                             │       │
│  │  │     NAMESPACE: "production"                               │       │
│  │  │     DEPLOYMENT_NAME: "payment-service"                    │       │
│  │  └── rationale: "Discovery protocol matched OOMKilled..."    │       │
│  └────────────────────────┬─────────────────────────────────────┘       │
│                           │                                             │
│                           │ RemediationOrchestrator watches             │
│                           │ (NO catalog lookup needed - pass through)   │
│                           ▼                                             │
│  ┌──────────────────────────────────────────────────────────────┐       │
│  │  WorkflowExecution.Spec (RO passes through from AIAnalysis)  │       │
│  │  ├── workflowRef: (identical WorkflowSnapshot fields)        │  ◄── PASS THROUGH
│  │  ├── parameters:                                             │       │
│  │  │     NAMESPACE: "production"                               │       │
│  │  │     DEPLOYMENT_NAME: "payment-service"                    │       │
│  │  ├── confidence: 0.95                                        │       │
│  │  └── rationale: "Discovery protocol matched..."              │       │
│  └────────────────────────┬─────────────────────────────────────┘       │
│                           │                                             │
│                           │ WorkflowExecution Controller                │
│                           ▼                                             │
│  ┌──────────────────────────────────────────────────────────────┐       │
│  │  Tekton PipelineRun (or batchv1.Job / AWX Job, per          │       │
│  │  executionEngine -- ADR-024, ADR-044)                        │       │
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

**Note**: KA's workflow discovery is a 3-step in-process protocol (`ListActions` →
`ListWorkflowsByActionType` → `GetWorkflowWithContextFilters`/`GetByID`, DD-WORKFLOW-016/DD-KA-017)
against its own informer-backed cache of `RemediationWorkflow`/`ActionType` CRDs — there is no MCP
tool call or HTTP request to Data Storage for this. Data Storage's `/api/v1/workflows*` REST surface
was retired as dead code once KA became the sole consumer (DD-WORKFLOW-019 v2.0, Phase 2g).

---

## Service Responsibilities

| Service | Responsibility |
|---------|----------------|
| **AIAnalysis Controller** | Submits investigation to KA (async submit/poll/result), stores `SelectedWorkflow` (including `executionBundle`) in status |
| **Kubernaut Agent (KA)** | Runs in-process workflow discovery against its own `RemediationWorkflow`/`ActionType` CRD cache, calls LLM, **resolves workflowId → executionBundle** (DD-WORKFLOW-019) |
| **RemediationOrchestrator** | Watches AIAnalysis, orchestrates approval flow, **passes through** to WorkflowExecution |
| **Notification Controller** | Delivers approval notifications via Alertmanager routing (DD-NOTIFICATION-001) |
| **RemediationApprovalRequest Controller** | Manages approval lifecycle, timeout expiration (ADR-040) |
| **Data Storage / AuthWebhook** | Own the `RemediationWorkflow`/`ActionType` CRD catalog data (etcd-backed); **do not** serve a search/MCP endpoint for it — retired (DD-WORKFLOW-019 v2.0 Phase 2g) |
| **WorkflowExecution Controller** | Uses `WorkflowRef.ExecutionBundle` to create the PipelineRun/Job/AWX run per `ExecutionEngine` |

### Key Design Decision (Approved 2025-11-28; mechanism corrected 2026-08-02)

**KA resolves `workflowId → executionBundle`** via its own in-process discovery cache. RO does NOT call the catalog — it passes through the resolved values from AIAnalysis.status.

**Rationale**:
1. KA already runs the discovery protocol during investigation — it has the data
2. Immutable workflows mean `executionBundle` never changes for a given `workflowId`+version
3. Simpler RO — no catalog client needed
4. Industry alignment (Temporal, Step Functions resolve at definition time)

---

## RO Pass-Through Logic

**RemediationOrchestrator** passes through the resolved workflow from AIAnalysis (no catalog lookup) —
see `pkg/remediationorchestrator/creator/workflowexecution.go` (`WorkflowExecutionCreator`) for the real
implementation. Simplified illustration of the pass-through:

```go
// pkg/remediationorchestrator/creator/workflowexecution.go (illustrative excerpt)
func (c *WorkflowExecutionCreator) buildSpec(
    aiAnalysis *aianalysisv1.AIAnalysis,
) workflowexecutionv1.WorkflowExecutionSpec {
    sw := aiAnalysis.Status.SelectedWorkflow // *SelectedWorkflow, embeds sharedtypes.WorkflowSnapshot

    return workflowexecutionv1.WorkflowExecutionSpec{
        WorkflowRef: workflowexecutionv1.WorkflowRef{
            WorkflowSnapshot: sw.WorkflowSnapshot, // PASS THROUGH -- identical embedded type, no re-fetch
        },
        Parameters: sw.Parameters, // PASS THROUGH
        Confidence: sw.Confidence,
        Rationale:  sw.Rationale,
    }
}
```

`validateSelectedWorkflow` (same file) enforces BR-ORCH-025's precondition: a missing/invalid
`selectedWorkflow` marks the `RemediationRequest` `Failed` rather than creating an incomplete
`WorkflowExecution`.

---

## Approval Integration (ADR-040, ADR-017, ADR-018)

### Key Principle: AIAnalysis Completes, RO Orchestrates

**AIAnalysis does NOT stay in "Approving" phase.** It completes its analysis (4-phase flow, no
"Approving"/"Recommending" phase) and signals one of two independent flags — see
[DD-CONTRACT-002](DD-CONTRACT-002-service-integration-contracts.md#contract-2-aianalysis--ro-status-output)
for the authoritative two-flag (`approvalRequired` vs. `needsHumanReview`) decision logic. The
RemediationOrchestrator is responsible for orchestrating whichever flow those flags trigger.

### AIAnalysis Completion (Approval Required Example)

```yaml
# AIAnalysis.Status after a selection that clears KA's own workflow-confidence
# floor but trips AIAnalysis's Rego approval-threshold policy (BR-AI-088, default 80%)
# NOTE: phase = "Completed", NOT "Approving"
status:
  phase: Completed              # ← AIAnalysis is DONE with its work
  selectedWorkflow:
    workflowId: "<DS-assigned UUID>"
    workflowName: "oomkill-increase-memory"
    confidence: 0.72            # Below the Rego policy's approval threshold
    parameters:
      NAMESPACE: "production"
  approvalRequired: true        # ← Rego decision: AI HAS an answer, policy requires approval
  needsHumanReview: false       # ← KA decision: AI COULD answer (contrast with DD-CONTRACT-002)
  whyApprovalRequired: "confidence 0.72 below Rego policy threshold 0.80"
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
| **DD-WORKFLOW-019** | KA-owned workflow discovery (authoritative for *who* resolves `workflowId → executionBundle` and *how* — corrects this document's v1.2 "HolmesGPT-API + Data Storage MCP search" claim) |
| **DD-CONTRACT-002** | Service Integration Contracts (authoritative for the `approvalRequired`/`needsHumanReview` two-flag decision logic) |
| **ADR-041** | LLM Response Contract (authoritative for `selected_workflow` format) |
| **ADR-043** | Workflow Schema Definition (authoritative for catalog schema) |
| **ADR-040** | RemediationApprovalRequest Architecture (approval lifecycle) |
| **ADR-017** | NotificationRequest Creator (RO creates notifications) |
| **ADR-018** | Approval Notification Integration (rich approval context) |
| **ADR-024**, **ADR-044** | WorkflowExecution's 3 execution engines (Tekton/Job/Ansible) and engine delegation |
| **DD-NOTIFICATION-001** | Alertmanager Routing Reuse (notification channel routing) |
| **DD-TIMEOUT-001** | Global Remediation Timeout (approval timeout: 15m default) |
| **DD-WORKFLOW-003** | Parameterized Actions (UPPER_SNAKE_CASE parameters) |
| **DD-WORKFLOW-018** | Etcd single source of truth; introduces the shared `WorkflowSnapshot` type this document now describes |
| **BR-ORCH-025** | Catalog Lookup Before WorkflowExecution |
| **BR-KA-212** | RCA-determined `RemediationTarget` |

---

## Migration Impact (Historical — v1.0, completed)

The table below documents the original v1.0 migration (Nov 2025); it is retained for historical
context and is **not** an open work item. All rows completed long before this v2.0 correction pass.

| File | Change Required |
|------|-----------------|
| `api/aianalysis/v1alpha1/aianalysis_types.go` | Replace `Recommendations` with `SelectedWorkflow` |
| `api/workflowexecution/v1alpha1/workflowexecution_types.go` | Add `WorkflowRef`, simplify `WorkflowDefinition` |
| `docs/services/crd-controllers/02-aianalysis/crd-schema.md` | Update examples |
| `docs/services/crd-controllers/03-workflowexecution/crd-schema.md` | Update examples |

---

## Confidence Assessment

| Aspect | Confidence | Rationale |
|--------|------------|-----------|
| **CRD contract shape (WorkflowSnapshot embed)** | 95% | Verified directly against current Go source (`api/aianalysis/v1alpha1`, `api/workflowexecution/v1alpha1`, `pkg/shared/types/workflow_snapshot.go`) |
| **Catalog-resolution mechanism (KA in-process, DD-WORKFLOW-019)** | 98% | DD-WORKFLOW-019 v2.0 status is "Implemented"; retirement of DS's `/api/v1/workflows*` confirmed by an E2E test (`04_workflow_endpoints_retired_test.go`) |
| **Approval integration** | 90% | Two-flag split (`approvalRequired`/`needsHumanReview`) confirmed in source; precise Rego threshold values not re-verified in this pass — see DD-CONTRACT-002 |
| **Overall** | 95% | Core contract shape and catalog-resolution ownership corrected against source; some illustrative Go snippets simplified for readability rather than reproduced verbatim |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 2.0 | 2026-08-02 | **Correction** ([Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806)): Renamed HolmesGPT-API → Kubernaut Agent (KA) throughout. Corrected catalog-resolution mechanism: KA resolves `workflowId → executionBundle` via its own in-process discovery cache (DD-WORKFLOW-019), not a Data Storage MCP search — that endpoint was retired as dead code. Fixed API group `kubernaut.io` → `kubernaut.ai`. Replaced `ContainerImage`/`ContainerDigest` field names with the real `ExecutionBundle`/`ExecutionBundleDigest` (generalized for 3 execution engines: Tekton/Job/Ansible). Corrected `SelectedWorkflow`/`WorkflowRef` Go samples to reflect the real shared `sharedtypes.WorkflowSnapshot` embed. Pointed to DD-CONTRACT-002 for the up-to-date `approvalRequired`/`needsHumanReview` two-flag logic rather than duplicating a now-superseded single-flag description. Marked the v1.0 migration table historical. |
| 1.2 | 2025-11-28 | **BREAKING**: HolmesGPT-API now resolves `workflow_id → containerImage` during MCP search. Added `containerImage` and `containerDigest` to `SelectedWorkflow`. RO no longer calls catalog - passes through from AIAnalysis. Updated data flow diagram. Removed catalog client code. |
| 1.1 | 2025-11-28 | **Approval flow clarification**: AIAnalysis completes with `approvalRequired: true`, RO orchestrates approval (creates NotificationRequest + RemediationApprovalRequest). Removed "Approving" phase from AIAnalysis. Added approval flow diagram and service responsibilities. |
| 1.0 | 2025-11-28 | Initial DD: AIAnalysis ↔ WorkflowExecution contract alignment |

