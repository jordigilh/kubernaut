# DD-CONTRACT-002: Service Integration Contracts

**Status**: ✅ Approved
**Version**: 1.2
**Date**: 2025-12-01
**Confidence**: 98%

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
│        │ watches                         │ calls HolmesGPT-API              │
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
│        │   reads containerImage from AIAnalysis.status.selectedWorkflow     │
│        │   (NO catalog lookup - HolmesGPT-API resolved during MCP search)   │
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
apiVersion: kubernaut.io/v1alpha1
kind: AIAnalysis
metadata:
  name: aianalysis-<remediation-id>
  namespace: kubernaut-system
  ownerReferences:
    - apiVersion: kubernaut.io/v1alpha1
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

  # REQUIRED: Human review flag (from HAPI - BR-HAPI-197, BR-HAPI-212)
  # Set by HAPI when AI cannot produce reliable result
  needsHumanReview: bool           # true = AI can't answer (RCA incomplete)
  humanReviewReason: string        # Why review needed (when needsHumanReview=true)

  # REQUIRED (when phase=Completed): Selected workflow
  # NOTE: containerImage resolved by HolmesGPT-API during MCP search (DD-CONTRACT-001 v1.2)
  selectedWorkflow:
    workflowId: string       # Catalog lookup key (e.g., "oomkill-increase-memory")
    version: string          # Workflow version (e.g., "1.0.0")
    containerImage: string   # OCI bundle (resolved by HolmesGPT-API)
    containerDigest: string  # For audit trail (resolved by HolmesGPT-API)
    confidence: float64      # 0.0-1.0
    parameters:              # map[string]string - UPPER_SNAKE_CASE keys
      NAMESPACE: string
      DEPLOYMENT_NAME: string
      # ... other workflow-specific params
    rationale: string        # Why this workflow was selected
    actionType: string       # DD-WORKFLOW-016 taxonomy (e.g., "ScaleReplicas", "RestartPod")

  # REQUIRED: Approval signal (from AIAnalysis Rego policies)
  # Set by AIAnalysis when policy requires approval for high-risk remediation
  approvalRequired: bool    # true = AI has answer, policy requires approval
  approvalReason: string    # Why approval needed (when approvalRequired=true)

  # OPTIONAL: Rich context for approval
  approvalContext:
    investigationSummary: string
    evidenceCollected: []string
    alternativesConsidered: []AlternativeWorkflow

  # REQUIRED (when phase=Completed): RCA-determined target resource (BR-HAPI-212, DD-HAPI-006)
  rootCauseAnalysis:
    targetResource:
      kind: string           # e.g., "Deployment"
      apiVersion: string     # e.g., "apps/v1" (optional - static mapping fallback for core resources)
      name: string           # e.g., "payment-api"
      namespace: string      # e.g., "production" (optional for cluster-scoped resources)
```

### Contract Guarantees

| Field | Type | Required | RO Expects |
|-------|------|----------|------------|
| `status.phase` | string | ✅ | One of: Pending, Investigating, Analyzing, Completed, Failed |
| `status.needsHumanReview` | bool | ✅ | HAPI decision: AI can't answer (BR-HAPI-197, BR-HAPI-212) |
| `status.humanReviewReason` | string | ✅ (when needsHumanReview=true) | Why review needed (e.g., "rca_incomplete", "workflow_not_found") |
| `status.selectedWorkflow.workflowId` | string | ✅ (when Completed) | Valid workflow identifier |
| `status.selectedWorkflow.version` | string | ✅ (when Completed) | Semantic version |
| `status.selectedWorkflow.containerImage` | string | ✅ (when Completed) | OCI bundle reference (from HolmesGPT-API) |
| `status.selectedWorkflow.containerDigest` | string | ✅ (when Completed) | Image digest (from HolmesGPT-API) |
| `status.selectedWorkflow.confidence` | float64 | ✅ (when Completed) | 0.0 to 1.0 |
| `status.selectedWorkflow.parameters` | map[string]string | ✅ (when Completed) | UPPER_SNAKE_CASE keys |
| `status.selectedWorkflow.actionType` | string | ✅ (when Completed) | DD-WORKFLOW-016 taxonomy (e.g., "ScaleReplicas") |
| `status.approvalRequired` | bool | ✅ | Rego decision: Policy requires approval for high-risk remediation |
| `status.rootCauseAnalysis.targetResource` | object | ✅ (when Completed) | RCA-determined target (BR-HAPI-212, DD-HAPI-006) |

### RO Decision Logic (Updated for Two-Flag Architecture)

**CRITICAL DISTINCTION** (BR-HAPI-197, BR-HAPI-212):
- **`needsHumanReview`** (HAPI decision) = AI **can't** answer → NotificationRequest
- **`approvalRequired`** (Rego decision) = AI **has** answer, policy requires approval → RemediationApprovalRequest

```go
// pkg/remediationorchestrator/reconciler.go
func (r *Reconciler) handleAIAnalysisCompleted(ctx context.Context, aiAnalysis *v1alpha1.AIAnalysis) error {
    // 1. Check if HAPI couldn't produce reliable result (BR-HAPI-197, BR-HAPI-212)
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

## Contract 3: HolmesGPT-API → Data Storage (MCP Workflow Search)

**NOTE**: This contract is between HolmesGPT-API and Data Storage, NOT RO.
RO does not call Data Storage for workflow resolution (DD-CONTRACT-001 v1.2).

### API Call (by HolmesGPT-API during MCP search)

```
POST /api/v1/workflows/search
```

### Request

```json
{
  "query": "OOMKilled memory limit exceeded",
  "labels": {
    "signal_type": "OOMKilled",
    "severity": "critical"
  },
  "limit": 5
}
```

### Response

```json
{
  "workflows": [
    {
      "workflow_id": "oomkill-increase-memory",
      "version": "1.0.0",
      "container_image": "quay.io/kubernaut/workflow-oomkill:v1.0.0",
      "container_digest": "sha256:abc123def456...",
      "parameters": [
        {"name": "NAMESPACE", "type": "string", "required": true},
        {"name": "DEPLOYMENT_NAME", "type": "string", "required": true}
      ],
      "labels": {
        "signal_type": "OOMKilled",
        "severity": "critical"
      },
      "similarity_score": 0.95
    }
  ]
}
```

### Contract Guarantees

| Field | Type | Required | HolmesGPT-API Extracts |
|-------|------|----------|------------------------|
| `container_image` | string | ✅ | Included in AIAnalysis.status.selectedWorkflow |
| `container_digest` | string | ✅ | Included in AIAnalysis.status.selectedWorkflow |
| `parameters` | array | ✅ | Used to validate LLM-selected parameters |

### Error Handling (by HolmesGPT-API)

| Status | Meaning | HolmesGPT-API Action |
|--------|---------|----------------------|
| 200 | Found | Select best match, include in response |
| 404 | No matches | Return error to AIAnalysis controller |
| 5xx | Server error | Retry with backoff |

---

## Contract 4: RO → WorkflowExecution (Creation)

### What RO Creates

```yaml
apiVersion: kubernaut.io/v1alpha1
kind: WorkflowExecution
metadata:
  name: workflow-<remediation-id>
  namespace: kubernaut-system
  ownerReferences:
    - apiVersion: kubernaut.io/v1alpha1
      kind: RemediationRequest
      name: <remediation-request-name>
      uid: <remediation-request-uid>
      controller: true
  labels:
    kubernaut.io/remediation-request: <remediation-request-name>
    kubernaut.io/workflow-id: <workflow-id>
spec:
  # REQUIRED: Parent reference
  remediationRequestRef:
    name: string
    namespace: string
    apiVersion: kubernaut.io/v1alpha1
    kind: RemediationRequest

  # REQUIRED: Workflow reference (PASS THROUGH from AIAnalysis)
  workflowRef:
    workflowId: string        # From AIAnalysis.status.selectedWorkflow.workflowId
    version: string           # From AIAnalysis.status.selectedWorkflow.version
    containerImage: string    # PASS THROUGH from AIAnalysis.status.selectedWorkflow.containerImage
    containerDigest: string   # PASS THROUGH from AIAnalysis.status.selectedWorkflow.containerDigest

  # REQUIRED: Parameters from LLM
  parameters:                 # map[string]string - copied from AIAnalysis
    NAMESPACE: string
    DEPLOYMENT_NAME: string

  # REQUIRED: Audit trail
  confidence: float64         # From AIAnalysis.status.selectedWorkflow.confidence
  rationale: string           # From AIAnalysis.status.selectedWorkflow.rationale

  # OPTIONAL: Execution config
  executionConfig:
    timeout: duration         # Default: 30m
    serviceAccountName: string # Default: kubernaut-workflow-runner
```

### Contract Guarantees

| Field | Type | Required | Source |
|-------|------|----------|--------|
| `spec.workflowRef.workflowId` | string | ✅ | AIAnalysis.status.selectedWorkflow.workflowId |
| `spec.workflowRef.version` | string | ✅ | AIAnalysis.status.selectedWorkflow.version |
| `spec.workflowRef.containerImage` | string | ✅ | AIAnalysis.status.selectedWorkflow.containerImage (PASS THROUGH) |
| `spec.workflowRef.containerDigest` | string | ✅ | AIAnalysis.status.selectedWorkflow.containerDigest (PASS THROUGH) |
| `spec.parameters` | map[string]string | ✅ | AIAnalysis.status.selectedWorkflow.parameters |
| `spec.confidence` | float64 | ✅ | AIAnalysis.status.selectedWorkflow.confidence |

### Key Design Decision (DD-CONTRACT-001 v1.2)

```
✅ CORRECT: RO passes through all fields from AIAnalysis.status.selectedWorkflow
            HolmesGPT-API already resolved containerImage during MCP search
```

RO does NOT call Data Storage API. HolmesGPT-API resolves `workflow_id → container_image` during MCP search and includes it in the AIAnalysis status.

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
    - apiVersion: kubernaut.io/v1alpha1
      kind: WorkflowExecution
      name: <workflow-execution-name>
      uid: <workflow-execution-uid>
      controller: true
  labels:
    kubernaut.io/workflow-execution: <workflow-execution-name>
    kubernaut.io/workflow-id: <workflow-id>
spec:
  pipelineRef:
    resolver: bundles
    params:
      - name: bundle
        value: <containerImage>  # From spec.workflowRef.containerImage
      - name: name
        value: <workflowId>      # From spec.workflowRef.workflowId

  params:                        # Copied from spec.parameters
    - name: NAMESPACE
      value: <value>
    - name: DEPLOYMENT_NAME
      value: <value>

  timeouts:
    pipeline: <timeout>          # From spec.executionConfig.timeout

  taskRunTemplate:
    serviceAccountName: <sa>     # From spec.executionConfig.serviceAccountName
```

### Contract Guarantees

| PipelineRun Field | Source |
|-------------------|--------|
| `spec.pipelineRef.params[bundle].value` | `WorkflowExecution.spec.workflowRef.containerImage` |
| `spec.pipelineRef.params[name].value` | `WorkflowExecution.spec.workflowRef.workflowId` |
| `spec.params` | `WorkflowExecution.spec.parameters` |
| `spec.timeouts.pipeline` | `WorkflowExecution.spec.executionConfig.timeout` |

---

## Summary: Data Flow Table

| Step | Source | Destination | Data Transferred |
|------|--------|-------------|------------------|
| 1 | RO | AIAnalysis.spec | remediationRequestRef, signalContext |
| 2 | AIAnalysis Controller | HolmesGPT-API | analysisRequest |
| 3 | HolmesGPT-API | Data Storage API | MCP workflow search |
| 4 | Data Storage API | HolmesGPT-API | workflows (incl. containerImage, containerDigest) |
| 5 | HolmesGPT-API | LLM | prompt with workflow options |
| 6 | LLM | HolmesGPT-API | selected workflow_id + parameters |
| 7 | HolmesGPT-API | AIAnalysis.status | selectedWorkflow (workflowId, actionType, containerImage, params, confidence) |
| 8 | RO | WorkflowExecution.spec | workflowRef (PASS THROUGH), parameters |
| 9 | WorkflowExecution | PipelineRun.spec | bundle, params |
| 10 | Tekton | PipelineRun.status | Succeeded/Failed |
| 11 | WorkflowExecution | WorkflowExecution.status | phase, failureReason |
| 12 | RO | RemediationRequest.status | overallPhase |

---

## Validation Checklist

Before implementing any service, verify:

### HolmesGPT-API (Amended Requirement)
- [ ] Queries Data Storage API for MCP workflow search
- [ ] Includes `containerImage` and `containerDigest` in response to AIAnalysis
- [ ] Returns complete workflow info (not just workflow_id)

### AIAnalysis Controller
- [ ] Receives `containerImage` and `containerDigest` from HolmesGPT-API
- [ ] Populates `status.selectedWorkflow.containerImage` (from HolmesGPT-API)
- [ ] Populates `status.selectedWorkflow.containerDigest` (from HolmesGPT-API)
- [ ] Populates `status.selectedWorkflow.parameters` with UPPER_SNAKE_CASE keys
- [ ] Sets `status.approvalRequired = true` when confidence < 0.80
- [ ] Sets `status.phase = Completed` (never "Approving")

### RemediationOrchestrator
- [ ] Does NOT call Data Storage API for workflow resolution
- [ ] Passes through `containerImage` from AIAnalysis.status.selectedWorkflow
- [ ] Passes through `containerDigest` from AIAnalysis.status.selectedWorkflow
- [ ] Copies `parameters` from AIAnalysis to WorkflowExecution unchanged
- [ ] Handles approval flow before creating WorkflowExecution

### WorkflowExecution Controller
- [ ] Creates PipelineRun with `resolver: bundles`
- [ ] Passes `containerImage` as bundle parameter
- [ ] Passes all `parameters` as PipelineRun params
- [ ] Does NOT orchestrate steps (Tekton does)

---

## Related Documents

| Document | Purpose |
|----------|---------|
| **DD-CONTRACT-001 v1.2** | AIAnalysis ↔ WorkflowExecution schema alignment (containerImage resolution) |
| **ADR-041** | LLM Response Contract (selectedWorkflow format) |
| **ADR-043** | Workflow Schema Definition (catalog format) |
| **ADR-044** | Engine Delegation (Tekton handles steps) |
| **DD-WORKFLOW-003** | Parameter naming (UPPER_SNAKE_CASE) |

---

## HolmesGPT-API Amendment Required

**TODO**: HolmesGPT-API must be amended to include `container_image` and `container_digest` in its response to AIAnalysis.

### Current Response (Pre-Amendment)
```json
{
  "selected_workflow": {
    "workflow_id": "oomkill-increase-memory",
    "confidence": 0.95,
    "parameters": {...}
  }
}
```

### Required Response (Post-Amendment)
```json
{
  "selected_workflow": {
    "workflow_id": "oomkill-increase-memory",
    "version": "1.0.0",
    "container_image": "quay.io/kubernaut/workflow-oomkill:v1.0.0",
    "container_digest": "sha256:abc123...",
    "confidence": 0.95,
    "parameters": {...},
    "rationale": "..."
  }
}
```

**Implementation Effort**: ~2-4 hours (HolmesGPT-API already queries catalog for MCP search)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.2 | 2025-12-01 | Fixed internal inconsistencies: Updated integration diagram and Contract 4 to consistently state RO passes through containerImage from AIAnalysis.status (no catalog lookup). Removed stale "FROM CATALOG LOOKUP" comments. |
| 1.1 | 2025-11-28 | Updated contracts for HolmesGPT-API containerImage resolution (DD-CONTRACT-001 v1.2). RO no longer calls Data Storage. Added HolmesGPT-API amendment requirement. |
| 1.0 | 2025-11-28 | Initial authoritative integration contracts |


