# DD-CONTRACT-002: Service Integration Contracts

**Status**: ✅ Approved
**Version**: 1.3
**Date**: 2025-12-01 (v1.2); 2026-08-02 (v1.3 correction, [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806))
**Confidence**: 95%

> **v1.3 correction note (2026-08-02)**: Following up on the 2026-08-01 partial pass noted below, this
> revision finishes the job: **Contract 3 is rewritten** to reflect [DD-WORKFLOW-019](DD-WORKFLOW-019-ka-owned-workflow-discovery.md)
> (KA owns workflow discovery in-process; the Data Storage MCP search endpoint it used to describe is
> retired dead code), and all remaining "HolmesGPT-API" references throughout Contracts 1/4/5/6, the
> data-flow summary table, and the validation checklist are renamed to **Kubernaut Agent (KA)**. The
> obsolete "HolmesGPT-API Amendment Required" section (a v1.0 TODO that was completed and shipped long
> ago, in a different shape — `executionBundle`/`executionBundleDigest`, not `container_image`/`container_digest`)
> is removed. The `kubernaut.io` API group used in Contract 1/4 YAML samples is corrected to `kubernaut.ai`.
>
> **2026-08-01 note (partial pass, retained for history)**: The `BR-HAPI-*` IDs and human-review contract
> language in "Contract 2" have been corrected to `BR-KA-*` / `remediationTarget` to match
> [DD-KA-006](DD-KA-006-remediation-target-in-rca.md).

---

## Purpose

This document defines the **authoritative API contracts** between AIAnalysis, RemediationOrchestrator (RO), and WorkflowExecution services. No implementation should proceed without these contracts being clear.

---

## Integration Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    SERVICE INTEGRATION SEQUENCE                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────┐     creates      ┌─────────────┐                           │
│  │     RO      │ ───────────────► │ AIAnalysis  │                           │
│  └─────────────┘                  └──────┬──────┘                           │
│        │                                 │                                  │
│        │ watches                         │ calls Kubernaut Agent (KA)       │
│        │ status                          │ updates status                   │
│        │                                 ▼                                  │
│        │                          ┌──────────────┐                          │
│        │◄─────────────────────────│ AIAnalysis   │                          │
│        │  reads selectedWorkflow  │ .status      │                          │
│        │  reads approvalRequired  └──────────────┘                          │
│        │                                                                    │
│        │ IF approvalRequired == true:                                       │
│        │   creates NotificationRequest                                      │
│        │   creates RemediationApprovalRequest                               │
│        │   waits for approval decision                                      │
│        │                                                                    │
│        │ THEN (if approved or no approval needed):                          │
│        │   reads executionBundle from AIAnalysis.status.selectedWorkflow    │
│        │   (NO catalog lookup - KA resolved it via its own in-process cache)│
│        │                                                                    │
│        ▼                                                                    │
│  ┌─────────────┐     creates      ┌──────────────────┐                      │
│  │     RO      │ ───────────────► │ WorkflowExecution │                     │
│  └─────────────┘                  └────────┬─────────┘                      │
│        │                                   │                                │
│        │ watches                           │ creates PipelineRun            │
│        │ status                            │ watches PipelineRun            │
│        │                                   │ updates status                 │
│        │                                   ▼                                │
│        │                          ┌──────────────────┐                      │
│        │◄─────────────────────────│ WorkflowExecution │                     │
│        │  reads phase             │ .status           │                     │
│        │  reads failureReason     └──────────────────┘                      │
│        │                                                                    │
│        ▼                                                                    │
│  RO creates NotificationRequest (success/failure)                           │
│  RO updates RemediationRequest.status                                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Contract 1: RO → AIAnalysis (Creation)

### What RO Creates

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: AIAnalysis
metadata:
  name: aianalysis-<remediation-id>
  namespace: kubernaut-system
  ownerReferences:
    - apiVersion: kubernaut.ai/v1alpha1
      kind: RemediationRequest
      name: <remediation-request-name>
      uid: <remediation-request-uid>
      controller: true
spec:
  # REQUIRED: Parent reference for audit trail
  remediationRequestRef:
    name: string           # RemediationRequest name
    namespace: string      # kubernaut-system

  # REQUIRED: Self-contained analysis request
  analysisRequest:
    signalContext:
      fingerprint: string  # Signal fingerprint
      severity: string     # critical, warning, info
      environment: string  # production, staging, dev
      businessPriority: string  # p0, p1, p2

      # Complete enriched payload from SignalProcessing
      enrichedPayload:
        originalSignal: object    # Labels, annotations
        kubernetesContext: object # Pod, deployment, node details
        businessContext: object   # Owner, criticality, SLA

    analysisTypes:
      - investigation
      - root-cause
      - workflow-selection

    investigationScope:
      timeWindow: string      # e.g., "24h"
      correlationDepth: string # basic, detailed
```

### Contract Guarantees

| Field | Type | Required | RO Provides |
|-------|------|----------|-------------|
| `spec.remediationRequestRef.name` | string | ✅ | RemediationRequest name |
| `spec.remediationRequestRef.namespace` | string | ✅ | `kubernaut-system` |
| `spec.analysisRequest.signalContext.fingerprint` | string | ✅ | From SignalProcessing |
| `spec.analysisRequest.signalContext.severity` | string | ✅ | From SignalProcessing |
| `spec.analysisRequest.signalContext.enrichedPayload` | object | ✅ | Snapshot from SignalProcessing.status |
| `ownerReferences` | array | ✅ | RemediationRequest as owner |

---

## Contract 2: AIAnalysis → RO (Status Output)

### What RO Reads from AIAnalysis.status

```yaml
status:
  # REQUIRED: Phase for workflow control
  phase: string  # Pending, Investigating, Analyzing, Completed, Failed

  # REQUIRED: Human review flag (from KA - BR-KA-197, BR-KA-212)
  # Set by KA when AI cannot produce reliable result
  needsHumanReview: bool           # true = AI can't answer (RCA incomplete)
  humanReviewReason: string        # Why review needed (when needsHumanReview=true)

  # REQUIRED (when phase=Completed): Selected workflow
  # NOTE: executionBundle resolved by KA's own in-process discovery cache, NOT a
  # Data Storage MCP search (DD-WORKFLOW-019) -- see Contract 3 below
  selectedWorkflow:
    workflowId: string           # Catalog lookup key (DS-assigned UUID)
    workflowName: string         # Human-readable name (e.g., "oomkill-increase-memory")
    version: string               # Workflow version (e.g., "1.0.0")
    executionBundle: string      # OCI bundle ref (resolved by KA; renamed from "containerImage")
    executionBundleDigest: string # For audit trail (resolved by KA)
    executionEngine: string      # "tekton" | "job" | "ansible" (ADR-024, ADR-044)
    confidence: float64          # 0.0-1.0
    parameters:                  # map[string]string - UPPER_SNAKE_CASE keys
      NAMESPACE: string
      DEPLOYMENT_NAME: string
      # ... other workflow-specific params
    rationale: string            # Why this workflow was selected
    actionType: string           # DD-WORKFLOW-016 taxonomy (e.g., "ScaleReplicas", "RestartPod")

  # REQUIRED: Approval signal (from AIAnalysis Rego policies)
  # Set by AIAnalysis when policy requires approval for high-risk remediation
  approvalRequired: bool    # true = AI has answer, policy requires approval
  approvalReason: string    # Why approval needed (when approvalRequired=true)

  # OPTIONAL: Rich context for approval
  approvalContext:
    investigationSummary: string
    evidenceCollected: []string
    alternativesConsidered: []AlternativeWorkflow

  # REQUIRED (when phase=Completed): RCA-determined target resource (BR-KA-212, DD-KA-006)
  rootCauseAnalysis:
    remediationTarget:
      kind: string           # e.g., "Deployment"
      apiVersion: string     # e.g., "apps/v1" (optional - static mapping fallback for core resources)
      name: string           # e.g., "payment-api"
      namespace: string      # e.g., "production" (optional for cluster-scoped resources)
```

### Contract Guarantees

| Field | Type | Required | RO Expects |
|-------|------|----------|------------|
| `status.phase` | string | ✅ | One of: Pending, Investigating, Analyzing, Completed, Failed |
| `status.needsHumanReview` | bool | ✅ | KA decision: AI can't answer (BR-KA-197, BR-KA-212) |
| `status.humanReviewReason` | string | ✅ (when needsHumanReview=true) | One of `rca_incomplete`, `no_matching_workflows`, `low_confidence` (real enum, `api/aianalysis/v1alpha1/aianalysis_types.go`) |
| `status.selectedWorkflow.workflowId` | string | ✅ (when Completed) | Valid workflow identifier |
| `status.selectedWorkflow.version` | string | ✅ (when Completed) | Semantic version |
| `status.selectedWorkflow.executionBundle` | string | ✅ (when Completed) | OCI bundle reference (from KA's in-process discovery cache) |
| `status.selectedWorkflow.executionBundleDigest` | string | ✅ (when Completed) | Bundle digest (from KA's in-process discovery cache) |
| `status.selectedWorkflow.confidence` | float64 | ✅ (when Completed) | 0.0 to 1.0 |
| `status.selectedWorkflow.parameters` | map[string]string | ✅ (when Completed) | UPPER_SNAKE_CASE keys |
| `status.selectedWorkflow.actionType` | string | ✅ (when Completed) | DD-WORKFLOW-016 taxonomy (e.g., "ScaleReplicas") |
| `status.approvalRequired` | bool | ✅ | Rego decision: Policy requires approval for high-risk remediation |
| `status.rootCauseAnalysis.remediationTarget` | object | ✅ (when Completed) | RCA-determined target (BR-KA-212, DD-KA-006) |

### RO Decision Logic (Updated for Two-Flag Architecture)

**CRITICAL DISTINCTION** (BR-KA-197, BR-KA-212):
- **`needsHumanReview`** (KA decision) = AI **can't** answer → NotificationRequest
- **`approvalRequired`** (Rego decision) = AI **has** answer, policy requires approval → RemediationApprovalRequest

```go
// pkg/remediationorchestrator/reconciler.go
func (r *Reconciler) handleAIAnalysisCompleted(ctx context.Context, aiAnalysis *v1alpha1.AIAnalysis) error {
    // 1. Check if KA couldn't produce reliable result (BR-KA-197, BR-KA-212)
    if aiAnalysis.Status.NeedsHumanReview {
        // Create NotificationRequest (manual investigation needed)
        // AI can't answer: incomplete RCA, workflow validation failed, etc.
        return r.createManualReviewNotification(ctx, aiAnalysis)
    }

    // 2. Check if Rego policy requires approval (existing behavior)
    if aiAnalysis.Status.ApprovalRequired {
        // Create NotificationRequest for approval notification
        // Create RemediationApprovalRequest for approval tracking
        // AI has answer, but policy requires human approval
        return r.initiateApprovalFlow(ctx, aiAnalysis)
    }

    // 3. No review or approval needed - proceed to automatic execution
    return r.createWorkflowExecution(ctx, aiAnalysis)
}
```

---

## Contract 3: KA's In-Process Workflow Discovery (DD-WORKFLOW-019)

**NOTE**: There is no HTTP/MCP contract here — that is precisely what this rewrite corrects. The
v1.2 version of this section described `HolmesGPT-API` calling `POST /api/v1/workflows/search` on
Data Storage. Neither exists today:

- The service is **Kubernaut Agent (KA)**, a native-Go rewrite of the old Python HolmesGPT-API.
- Data Storage's `/api/v1/workflows*` REST surface was **retired as dead code** once KA became the
  sole production consumer of the discovery protocol (DD-WORKFLOW-019 v2.0, Issue #1677 Phase 2g).
  Its retirement is proven by `test/e2e/datastorage/04_workflow_endpoints_retired_test.go`.
- Workflow catalog data itself still lives outside KA — as `RemediationWorkflow`/`ActionType` CRDs
  (etcd-backed, reconciled by `authwebhook`) — but KA reads it directly via its own
  `controller-runtime` informer cache (`internal/kubernautagent/tools/custom/tools.go`,
  `internal/kubernautagent/workflowcatalog`), not through any other service's API.

### KA's 3-step in-process discovery protocol (DD-WORKFLOW-016 / DD-KA-017)

No network call — this is a Go function call chain against KA's own cache:

```go
// internal/kubernautagent/tools/custom/tools.go (illustrative — see source for exact signatures)
actions := catalog.ListActions(ctx, filters)                          // step 1
workflows := catalog.ListWorkflowsByActionType(ctx, actionType, ctx2)  // step 2
workflow := catalog.GetWorkflowWithContextFilters(ctx, workflowID, filters) // step 3 (or GetByID)
```

`workflow` is a `sharedtypes.WorkflowSnapshot` (`WorkflowID`, `WorkflowName`, `ActionType`, `Version`,
`ExecutionBundle`, `ExecutionBundleDigest`, `ExecutionEngine`, `EngineConfig`, `ServiceAccountName`,
`Dependencies`, `Resources`, `DeclaredParameterNames`) — the same type embedded in both
`AIAnalysis.Status.SelectedWorkflow` and `WorkflowExecution.Spec.WorkflowRef` (see DD-CONTRACT-001 v2.0).
KA includes this snapshot (plus LLM-selected `parameters`/`confidence`/`rationale`) when it returns the
investigation result to the AIAnalysis controller via its async `GetSessionResult()` API
(`pkg/agentclient`) — not via any Data Storage response.

### Contract Guarantees

| Field | Type | Required | KA Extracts From |
|-------|------|----------|-------------------|
| `executionBundle` | string | ✅ | Its own informer-cached `RemediationWorkflow` CRD, not an HTTP response |
| `executionBundleDigest` | string | ✅ | Same |
| `dependencies`/`resources`/`declaredParameterNames` | array/map | ✅ | Same — catalog-authoritative, validated by KA before returning (Issue #1661 Change 11a) |

### Error Handling (by KA)

| Condition | Meaning | KA Action |
|-----------|---------|-----------|
| Found | Match in cache | Select best match, include in response |
| No matches | `ListWorkflowsByActionType`/`GetByID` returns empty | Set `AIAnalysis.Status.NeedsHumanReview=true`, `HumanReviewReason="no_matching_workflows"` |
| Cache not yet synced | Informer hasn't completed initial `List`/`Watch` | KA blocks on `WaitForCacheSync` at startup — not a per-request failure mode |

---

## Contract 4: RO → WorkflowExecution (Creation)

### What RO Creates

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: workflow-<remediation-id>
  namespace: kubernaut-system
  ownerReferences:
    - apiVersion: kubernaut.ai/v1alpha1
      kind: RemediationRequest
      name: <remediation-request-name>
      uid: <remediation-request-uid>
      controller: true
  labels:
    kubernaut.ai/remediation-request: <remediation-request-name>
    kubernaut.ai/workflow-id: <workflow-id>
spec:
  # REQUIRED: Parent reference
  remediationRequestRef:
    name: string
    namespace: string
    apiVersion: kubernaut.ai/v1alpha1
    kind: RemediationRequest

  # REQUIRED: Workflow reference (PASS THROUGH from AIAnalysis -- identical
  # embedded sharedtypes.WorkflowSnapshot type, no re-fetch from KA/DataStorage)
  workflowRef:
    workflowId: string            # From AIAnalysis.status.selectedWorkflow.workflowId
    workflowName: string          # From AIAnalysis.status.selectedWorkflow.workflowName
    version: string                # From AIAnalysis.status.selectedWorkflow.version
    executionBundle: string       # PASS THROUGH from AIAnalysis.status.selectedWorkflow.executionBundle
    executionBundleDigest: string # PASS THROUGH from AIAnalysis.status.selectedWorkflow.executionBundleDigest
    executionEngine: string       # PASS THROUGH -- "tekton" | "job" | "ansible"

  # REQUIRED: Resource lock target (DD-WE-001), format "namespace/kind/name"
  targetResource: string

  # OPTIONAL: Remote-cluster execution via MCP Gateway (BR-FLEET-054); empty = local/hub cluster
  clusterID: string

  # OPTIONAL: Parameters from LLM
  parameters:                 # map[string]string - copied from AIAnalysis
    NAMESPACE: string
    DEPLOYMENT_NAME: string

  # OPTIONAL: Audit trail
  confidence: float64         # From AIAnalysis.status.selectedWorkflow.confidence
  rationale: string           # From AIAnalysis.status.selectedWorkflow.rationale

  # OPTIONAL: Execution config (ServiceAccountName instead lives on workflowRef -- Issue #1661 Change 11c/11f)
  executionConfig:
    timeout: duration          # Tekton PipelineRun timeout; default: RemediationRequest global timeout or 30m
```

### Contract Guarantees

| Field | Type | Required | Source |
|-------|------|----------|--------|
| `spec.workflowRef.workflowId` | string | ✅ | AIAnalysis.status.selectedWorkflow.workflowId |
| `spec.workflowRef.version` | string | ✅ | AIAnalysis.status.selectedWorkflow.version |
| `spec.workflowRef.executionBundle` | string | ✅ | AIAnalysis.status.selectedWorkflow.executionBundle (PASS THROUGH) |
| `spec.workflowRef.executionBundleDigest` | string | ✅ | AIAnalysis.status.selectedWorkflow.executionBundleDigest (PASS THROUGH) |
| `spec.parameters` | map[string]string | ✅ | AIAnalysis.status.selectedWorkflow.parameters |
| `spec.confidence` | float64 | ✅ | AIAnalysis.status.selectedWorkflow.confidence |

### Key Design Decision (DD-CONTRACT-001 v2.0, DD-WORKFLOW-019)

```
✅ CORRECT: RO passes through all fields from AIAnalysis.status.selectedWorkflow
            KA already resolved executionBundle via its own in-process discovery cache
```

RO does NOT call Data Storage API. KA resolves `workflowId → executionBundle` via its own
in-process discovery cache (DD-WORKFLOW-019) and includes it in the AIAnalysis status.

---

## Contract 5: WorkflowExecution → RO (Status Output)

### What RO Reads from WorkflowExecution.status

```yaml
status:
  # REQUIRED: Phase for workflow control
  phase: string  # Pending, Running, Completed, Failed

  # Timing
  startTime: timestamp
  completionTime: timestamp
  duration: string        # e.g., "3m30s"

  # PipelineRun reference
  pipelineRunRef:
    name: string          # Tekton PipelineRun name

  # PipelineRun status summary
  pipelineRunStatus:
    status: string        # Unknown, True, False
    reason: string        # Succeeded, Failed, Running
    message: string       # Human-readable message
    completedTasks: int
    totalTasks: int

  # Failure info (when phase=Failed)
  failureReason: string   # Why execution failed
```

### Contract Guarantees

| Field | Type | Required | RO Expects |
|-------|------|----------|------------|
| `status.phase` | string | ✅ | One of: Pending, Running, Completed, Failed |
| `status.completionTime` | timestamp | When terminal | Set when Completed/Failed |
| `status.failureReason` | string | When Failed | Explains failure |

### RO Decision Logic

```go
// pkg/remediationorchestrator/reconciler.go
func (r *Reconciler) handleWorkflowExecutionStatus(ctx context.Context, wfe *v1alpha1.WorkflowExecution) error {
    switch wfe.Status.Phase {
    case "Pending", "Running":
        // Still executing - requeue and check again
        return ctrl.Result{RequeueAfter: 10 * time.Second}, nil

    case "Completed":
        // Success! Create success notification, update RemediationRequest
        return r.handleExecutionSuccess(ctx, wfe)

    case "Failed":
        // Failure - evaluate recovery or escalate
        return r.handleExecutionFailure(ctx, wfe)
    }
}
```

---

## Contract 6: WorkflowExecution → Tekton (PipelineRun)

### What WorkflowExecution Creates

```yaml
apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  name: <workflow-execution-name>-run
  namespace: kubernaut-system
  ownerReferences:
    - apiVersion: kubernaut.ai/v1alpha1
      kind: WorkflowExecution
      name: <workflow-execution-name>
      uid: <workflow-execution-uid>
      controller: true
  labels:
    kubernaut.ai/workflow-execution: <workflow-execution-name>
    kubernaut.ai/workflow-id: <workflow-id>
spec:
  pipelineRef:
    resolver: bundles
    params:
      - name: bundle
        value: <executionBundle>  # From spec.workflowRef.executionBundle
      - name: name
        value: <workflowId>       # From spec.workflowRef.workflowId

  params:                        # Copied from spec.parameters
    - name: NAMESPACE
      value: <value>
    - name: DEPLOYMENT_NAME
      value: <value>

  timeouts:
    pipeline: <timeout>          # From spec.executionConfig.timeout

  taskRunTemplate:
    serviceAccountName: <sa>     # From spec.workflowRef.serviceAccountName (Issue #1661 Change 11c/11f -- NOT executionConfig)
```

**Note**: This is the Tekton-engine path only. When `spec.workflowRef.executionEngine` is `job` or
`ansible`, WorkflowExecution creates a native `batchv1.Job` or an AWX Job Template run instead
(ADR-024, ADR-044) — the PipelineRun shape above does not apply.

### Contract Guarantees

| PipelineRun Field | Source |
|-------------------|--------|
| `spec.pipelineRef.params[bundle].value` | `WorkflowExecution.spec.workflowRef.executionBundle` |
| `spec.pipelineRef.params[name].value` | `WorkflowExecution.spec.workflowRef.workflowId` |
| `spec.params` | `WorkflowExecution.spec.parameters` |
| `spec.timeouts.pipeline` | `WorkflowExecution.spec.executionConfig.timeout` |
| `spec.taskRunTemplate.serviceAccountName` | `WorkflowExecution.spec.workflowRef.serviceAccountName` |

---

## Summary: Data Flow Table

| Step | Source | Destination | Data Transferred |
|------|--------|-------------|------------------|
| 1 | RO | AIAnalysis.spec | remediationRequestRef, signalContext |
| 2 | AIAnalysis Controller | Kubernaut Agent (KA) | analysisRequest (async `SubmitInvestigation()`) |
| 3 | KA | its own in-process cache (`RemediationWorkflow`/`ActionType` CRDs) | 3-step discovery protocol (DD-WORKFLOW-019) — no network hop |
| 4 | KA | LLM | prompt with workflow options |
| 5 | LLM | KA | selected workflowId + parameters |
| 6 | AIAnalysis Controller | KA | poll/result (`GetSessionResult()`, `pkg/agentclient`) |
| 7 | KA | AIAnalysis.status | selectedWorkflow (WorkflowSnapshot: workflowId, actionType, executionBundle, params, confidence) |
| 8 | RO | WorkflowExecution.spec | workflowRef (PASS THROUGH), parameters |
| 9 | WorkflowExecution | PipelineRun/Job/AWX run spec | bundle, params (per `executionEngine`) |
| 10 | Tekton/K8s/AWX | WorkflowExecution (watched) | Succeeded/Failed |
| 11 | WorkflowExecution | WorkflowExecution.status | phase, failureReason |
| 12 | RO | RemediationRequest.status | overallPhase |

---

## Validation Checklist

Before implementing any service, verify:

### Kubernaut Agent (KA)
- [ ] Runs the 3-step discovery protocol against its own informer-cached `RemediationWorkflow`/`ActionType` CRDs (DD-WORKFLOW-019) — does **not** call a Data Storage search endpoint
- [ ] Includes `executionBundle` and `executionBundleDigest` in the `WorkflowSnapshot` returned to AIAnalysis
- [ ] Returns complete workflow info (not just `workflowId`) — `WorkflowName`/`ActionType`/`Dependencies`/`Resources`/`DeclaredParameterNames` too

### AIAnalysis Controller
- [ ] Populates `status.selectedWorkflow` (embedded `WorkflowSnapshot`) from KA's session result
- [ ] Populates `status.selectedWorkflow.parameters` with UPPER_SNAKE_CASE keys
- [ ] Sets `status.approvalRequired = true` per the Rego policy decision (not a hardcoded threshold in this contract — see BR-AI-088)
- [ ] Sets `status.needsHumanReview = true` when KA can't answer (`rca_incomplete`/`no_matching_workflows`/`low_confidence`)
- [ ] Sets `status.phase = Completed` (never "Approving")

### RemediationOrchestrator
- [ ] Does NOT call Data Storage or KA for workflow resolution
- [ ] Passes through the embedded `WorkflowSnapshot` from AIAnalysis.status.selectedWorkflow unchanged
- [ ] Copies `parameters` from AIAnalysis to WorkflowExecution unchanged
- [ ] Handles approval flow before creating WorkflowExecution

### WorkflowExecution Controller
- [ ] Dispatches to the engine named in `spec.workflowRef.executionEngine` (Tekton `PipelineRef` with `resolver: bundles`, native `batchv1.Job`, or AWX Job Template)
- [ ] Passes `executionBundle` as the appropriate engine-specific image/bundle reference
- [ ] Passes all `parameters` as engine-specific params/env
- [ ] Does NOT orchestrate steps (the engine does)

---

## Related Documents

| Document | Purpose |
|----------|---------|
| **DD-CONTRACT-001 v2.0** | AIAnalysis ↔ WorkflowExecution schema alignment (`executionBundle` resolution, `WorkflowSnapshot` embed) |
| **DD-WORKFLOW-019** | KA-owned workflow discovery (authoritative for Contract 3's in-process mechanism) |
| **ADR-041** | LLM Response Contract (selectedWorkflow format) |
| **ADR-043** | Workflow Schema Definition (catalog format) |
| **ADR-024**, **ADR-044** | 3 execution engines (Tekton/Job/Ansible), engine delegation |
| **DD-WORKFLOW-003** | Parameter naming (UPPER_SNAKE_CASE) |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.3 | 2026-08-02 | **Correction** ([Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806)): Rewrote Contract 3 to describe KA's real in-process discovery protocol (DD-WORKFLOW-019) instead of a Data Storage MCP search endpoint that was retired as dead code. Renamed remaining HolmesGPT-API → Kubernaut Agent (KA) references throughout Contracts 1/4/5/6, the data-flow summary, and the validation checklist. Fixed `kubernaut.io` → `kubernaut.ai` API group. Replaced `containerImage`/`containerDigest` with the real `executionBundle`/`executionBundleDigest` (`sharedtypes.WorkflowSnapshot`). Removed the obsolete "HolmesGPT-API Amendment Required" section (its TODO shipped long ago, in this different field shape). Corrected Contract 4/6 to note the 3 execution engines (Tekton/Job/Ansible), not just Tekton. |
| 1.2 | 2025-12-01 | Fixed internal inconsistencies: Updated integration diagram and Contract 4 to consistently state RO passes through containerImage from AIAnalysis.status (no catalog lookup). Removed stale "FROM CATALOG LOOKUP" comments. |
| 1.1 | 2025-11-28 | Updated contracts for HolmesGPT-API containerImage resolution (DD-CONTRACT-001 v1.2). RO no longer calls Data Storage. Added HolmesGPT-API amendment requirement. |
| 1.0 | 2025-11-28 | Initial authoritative integration contracts |


