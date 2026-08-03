# AI Analysis Service - Reconciliation Phases

**Version**: v2.3
**Last Updated**: 2026-08-02
**Status**: ✅ V1.0 Scope Defined

---

## Changelog

| Version | Date | Changes | Reference |
|---------|------|---------|-----------|
| **v2.3** | 2026-08-02 | **#1806 CORRECTION**: Fixed the remaining stale sections not covered by the prior validation-focused pass (#1847): Phase 2 (Investigating) rewritten from a single 60s synchronous HolmesGPT-API call to the real async submit/poll/result session flow against Kubernaut Agent (KA) with a 25-minute wall-clock cap (BR-AA-KA-064); corrected the Phase Overview table/diagram, Phase Timeout Configuration section, Rego policy input/output examples (`require_approval`/`reason`, not `AUTO_APPROVE`/`MANUAL_APPROVAL_REQUIRED`), Recovery Attempts section (`isRecoveryAttempt`/`previousExecutions` do not exist on the current CRD spec), and the Metrics section (the referenced `kubernaut_aianalysis_phase_duration_seconds` metric does not exist; see `metrics-slos.md` for the real 4 metrics) | #1806, BR-AA-KA-064 |
| **v2.2** | 2025-12-09 | **V1.0 COMPLIANCE AUDIT**: (1) Timeout should be `spec.TimeoutConfig` not annotation (pending RO clarification); (2) Recovery attempts must use `/api/v1/recovery/analyze` endpoint (pending HAPI confirmation); (3) Recovery fields must be passed to HAPI | `NOTICE_AIANALYSIS_V1_COMPLIANCE_GAPS.md`, `REQUEST_RO_TIMEOUT_PASSTHROUGH_CLARIFICATION.md` (both docs/handoff/, deleted in the repo-wide non-authoritative docs purge) |
| v2.1 | 2025-12-06 | **BR-KA-197**: Added `SubReason` field for granular failure tracking; Removed `Recommending` from Phase enum; Added failure taxonomy | BR-KA-197, DD-HAPI-002 v1.2 |
| v2.0 | 2025-11-30 | **REGENERATED**: Removed "Approving" phase (V1.0); Removed BR-AI-051-053 (dependency validation); Simplified to 4-phase flow; Added DetectedLabels/CustomLabels handling | DD-RECOVERY-002, BR_MAPPING v1.2 |
| v1.1 | 2025-10-20 | Added approval context population | ADR-018 |
| v1.0 | 2025-10-15 | Initial specification | - |

---

## Phase Overview (V1.0)

### Phase Transitions

```
Pending → Investigating → Analyzing → Completed
   ↓           ↓              ↓           ↓
(<1s)   (async, ≤25min cap)  (≤5s)   (terminal)
```

**Note**: The "Approving" phase is **deferred to V1.1**. In V1.0, approval decisions are made during the Analyzing phase, and the Remediation Orchestrator (RO) handles notification.

**Note (Investigating)**: Investigating is not a single blocking call — it is an async submit/poll/result session against Kubernaut Agent (KA) spanning many reconciles (see [Phase 2](#phase-2-investigating) below). "≤25min" is the wall-clock cap on the whole session (`DefaultMaxInvestigationDuration`), not a per-call timeout.

### Phase Summary

| Phase | Purpose | Timeout | Key Actions |
|-------|---------|---------|-------------|
| **Pending** | Validation | <1s | Validate spec, add finalizer |
| **Investigating** | AI Analysis | 25min wall-clock cap (async session) | Submit investigation to Kubernaut Agent (KA), poll session until `completed`/`failed`, fetch result |
| **Analyzing** | Policy Evaluation | 5s | Evaluate Rego policies, validate workflow exists in catalog |
| **Completed** | Terminal | N/A | Output ready for RO consumption |

---

## Phase 1: Pending

**Purpose**: Initial validation and setup

**Timeout**: Immediate (<1s)

### Actions

1. **Validate Spec**
   - Ensure `enrichmentResults` is present (required)
   - Validate `signalContext` structure
   - Check parent references

2. **Add Finalizer**
   - Add `kubernaut.ai/cleanup` finalizer
   - Enables cleanup on deletion

3. **Initialize Status**
   - Set `status.phase = "Pending"`
   - Record `status.startTime`

### Transition Criteria

```go
if specValid && finalizerAdded {
    status.Phase = "Investigating"
    status.PhaseTransitions["Investigating"] = metav1.Now()
}
```

### Example Status After Pending

```yaml
status:
  phase: "Investigating"
  startTime: "2025-11-30T10:00:00Z"
  phaseTransitions:
    Pending: "2025-11-30T10:00:00Z"
    Investigating: "2025-11-30T10:00:01Z"
```

---

## Phase 2: Investigating

**Purpose**: AI-powered investigation via **Kubernaut Agent (KA)**, using an async submit/poll/result session (BR-AA-KA-064) — **not** a single synchronous HTTP call.

**Timeout**: 25 minutes wall-clock cap on the whole session (`DefaultMaxInvestigationDuration` in `pkg/aianalysis/handlers/constants.go`, configurable via `WithMaxInvestigationDuration(d time.Duration)` on the `InvestigatingHandler`) — not a per-call timeout.

### Actions

The `InvestigatingHandler` (`pkg/aianalysis/handlers/investigating.go`) dispatches on whether `status.kaSession` already has an active session ID:

1. **Submit** (no session yet, or session lost — see error handling below)
   - Build `agentclient.IncidentRequest` from `spec.analysisRequest.signalContext` (`RequestBuilder.BuildIncidentRequest`), including `enrichmentResults.customLabels` and `enrichmentResults.businessClassification`
   - Call `AgentClientInterface.SubmitInvestigation(ctx, req)` → `POST /api/v1/incident/analyze` → KA responds `202 Accepted` with a `session_id`
   - Store the session ID in `status.kaSession`; requeue for the first poll after `DefaultSessionPollInterval` (15s)

2. **Poll** (session ID present)
   - Call `AgentClientInterface.PollSession(ctx, sessionID)` → `GET /api/v1/incident/session/{id}` on each subsequent reconcile
   - Session status `pending`/`investigating`: requeue after the poll interval
   - Session status `user_driving`: a human operator has taken over the session (DD-INTERACTIVE-002); continue polling, same 25-minute cap still applies
   - Session status `completed`: call `GetSessionResult(ctx, sessionID)` to fetch the final `IncidentResponse`
   - Session status `failed`/`cancelled`: transition to `Failed`

3. **Receive Result** (`agentclient.IncidentResponse`)
   - `selectedWorkflow`: workflow ID/name, action type, parameters, and an embedded `WorkflowSnapshot` (`executionBundle` reference, not a raw `containerImage`)
   - `confidence`: 0.0-1.0 score
   - `rootCauseAnalysis`: includes `remediationTarget` (the LLM-identified resource) and `postRCAContext` (KA-computed `detectedLabels`/`ownerChain`)
   - `reasoning`/`warnings`: human-readable explanation and any degraded-detection warnings

### Transition Criteria

```go
switch status.Status { // agentclient.SessionStatusResult.Status
case "completed":
    result, err := h.kaClient.GetSessionResult(ctx, session.ID)
    // ... process result via ResponseProcessor ...
    status.Phase = "Analyzing"
    status.PhaseTransitions["Analyzing"] = metav1.Now()
case "failed", "cancelled":
    status.Phase = "Failed"
case "pending", "investigating", "user_driving":
    // requeue and poll again; checkInvestigationTimeout() enforces the 25min cap
default:
    // treat unknown status as still-in-progress; poll again
}
```

If the wall-clock session age exceeds `DefaultMaxInvestigationDuration` (25 minutes), `checkInvestigationTimeout` transitions the analysis to `Failed` regardless of the current session status.

### Session Regeneration on 404

If `PollSession` returns `404` (KA restarted and lost session state), the handler clears `status.kaSession.id` and re-submits a new investigation from scratch (BR-AA-KA-064.5) rather than failing immediately.

### Error Handling

| Error | Action | Retry |
|-------|--------|-------|
| Kubernaut Agent (KA) unavailable | Retry with exponential backoff | Yes (transient) |
| `PollSession` returns 404 (session lost) | Clear session ID, re-submit | Yes (BR-AA-KA-064.5) |
| 25-minute wall-clock cap exceeded | Mark as `Failed` | No |
| Invalid/malformed KA response | Mark as `Failed` | No |

### BR-KA-197: Human Review Required Handling

When the Kubernaut Agent (KA) result indicates `needs_human_review=true` (or equivalent warnings), the controller MUST:

1. **Fail immediately** - Do not proceed to Analyzing phase
2. **Set structured failure** - Use `Reason` + `SubReason` fields
3. **Emit metrics** - Track failure reason for observability

```go
if response.NeedsHumanReview {
    status.Phase = "Failed"
    status.Reason = "WorkflowResolutionFailed"  // Umbrella category
    status.SubReason = mapWarningsToSubReason(response.Warnings)  // Specific cause
    status.Message = strings.Join(response.Warnings, "; ")
    // Store partial response for operator context
}
```

### SubReason Mapping

| Kubernaut Agent (KA) Response Trigger | SubReason |
|-----------------------|-----------|
| Workflow Not Found | `WorkflowNotFound` |
| Container Image Mismatch | `ImageMismatch` |
| Parameter Validation Failed | `ParameterValidationFailed` |
| No Workflows Matched | `NoMatchingWorkflows` |
| Low Confidence (<70%) | `LowConfidence` |
| LLM Parsing Error | `LLMParsingError` |

---

## Phase 3: Analyzing

**Purpose**: Rego policy evaluation and workflow validation

**Timeout**: 5 seconds

### Actions

1. **Load Rego Approval Policies**
   - ConfigMap: `ai-approval-policies` in `kubernaut-system`
   - Evaluate with investigation result and context

2. **Build Policy Input** (`pkg/aianalysis/rego/evaluator.go` `PolicyInput`, fields grouped to avoid a "God struct" per the Go Anti-Pattern Checklist)
   ```go
   type PolicyInput struct {
       SignalContext       SignalContextInput       `json:"signal_context"`        // signal_type, severity, environment, business_priority
       TargetResource      TargetResourceInput      `json:"target_resource"`       // kind, name, namespace
       Classification      ClassificationInput      `json:"classification"`        // detected_labels, custom_labels, business_classification
       KAResponse          KAResponseInput          `json:"ka_response"`           // confidence, warnings, failed_detections
       RemediationTarget   *RemediationTargetInput  `json:"remediation_target,omitempty"` // ADR-055: LLM-identified target (kind/name/namespace)
       ConfidenceThreshold *float64                 `json:"confidence_threshold,omitempty"`
       Identity            *IdentityInput           `json:"identity,omitempty"`    // interactive sessions only (BR-AI-085)
       ActionType          string                   `json:"action_type"`           // catalog-authoritative (DD-WORKFLOW-016)
   }
   ```
   > Field grouping (`SignalContext`/`Classification`/`KAResponse`) is Go-side organization only (avoids a "God struct"); `buildRegoInputMap` still flattens these to top-level Rego input keys, e.g. `input.environment`, `input.detected_labels`, `input.confidence` — **not** `input.classification.detected_labels`.

3. **Evaluate Approval Decision**
   - The compiled policy returns a `{"require_approval": bool, "reason": string}` object (`PolicyResult`), not a `decision` string — `AUTO_APPROVE`/`MANUAL_APPROVAL_REQUIRED` never existed as literal Rego output values in this codebase
   - `require_approval = false`: Confidence ≥80%, low-risk environment (auto-approved)
   - `require_approval = true`: Confidence <80%, production, high-risk action, or `identity` absent (autonomous alert-driven flow defaults to requiring approval)

4. **Validate Workflow Response** (Defense-in-Depth)

   > **Note**: Per [DD-KA-001](../../../architecture/decisions/DD-KA-001-workflow-response-validation-architecture.md)
   > (formerly `DD-HAPI-002`) v1.1, primary validation happens in **Kubernaut Agent (KA)**, which is the sole
   > validation authority — AIAnalysis performs no separate re-validation pass. KA has **no LLM tool-calling
   > framework by design** ([DD-KA-019](../../../architecture/decisions/DD-KA-019-go-rewrite-design/DD-KA-019-go-rewrite-design.md));
   > there is no `validate_workflow_parameters` tool the LLM invokes. Instead, KA's Go orchestration code
   > (`internal/kubernautagent/parser/validator.go`) validates the LLM's *returned* workflow selection
   > programmatically immediately after each response, and on failure re-prompts the LLM with a rendered
   > error (`validation_error.tmpl`) as the next conversation turn — up to 3 total attempts before setting
   > `needs_human_review: true`. See [BR-KA-191](../../../requirements/BR-KA-191-workflow-parameter-validation.md)
   > for the full validate-then-reprompt design and acceptance criteria.

   | Validation | Performed by | AIAnalysis re-check? |
   |------------|---------|---------------------|
   | `workflowId` exists in catalog (hallucination detection) | ✅ **KA** (`validator.Validate`, self-corrects via re-prompt) | ❌ None (KA is sole authority) |
   | Required fields, types, enums, numeric bounds, regex patterns, `dependsOn` | ✅ **KA** (`validator.Validate`, self-corrects via re-prompt) | ❌ None (KA is sole authority) |
   | Undeclared/hallucinated parameters | ✅ **KA** (silently stripped, recorded for LLM feedback — not a hard failure) | ❌ None |
   | `schemaImage` (OCI image reference) resolution | Data Storage, at workflow registration time (separate from per-investigation validation) | N/A — registration-time, not investigation-time |

   **Rationale** ([DD-KA-001](../../../architecture/decisions/DD-KA-001-workflow-response-validation-architecture.md)):
   - If validation fails at KA → LLM can self-correct in the same investigation session (good UX)
   - Historical alternative (re-validating at AIAnalysis or the Workflow Engine) would force a full RCA restart on every validation failure (poor UX) — this was the original `BR-WE-001` design, since retired in favor of KA being the sole validator

### Transition Criteria

```go
if policyEvaluated && workflowValidated {
    status.SelectedWorkflow = investigationResult.SelectedWorkflow
    status.ApprovalRequired = policyResult.ApprovalRequired // rego.PolicyResult.ApprovalRequired
    status.ApprovalReason = policyResult.Reason
    status.Phase = "Completed"
    status.CompletionTime = metav1.Now()
}
```

### Rego Policy Example

```rego
package aianalysis.approval

import rego.v1

default require_approval := false

# ADR-055 / BR-AI-085-005: default-deny when the LLM didn't identify a
# remediation target — always require approval regardless of confidence.
require_approval if {
    not input.remediation_target
}

# Production + low confidence → require approval
require_approval if {
    lower(input.environment) == "production"
    input.confidence < 0.8
}

reason := "Auto-approved by policy" if not require_approval
default reason := "Manual approval required by policy"
```

> This is an illustrative simplification. The real policies (`pkg/aianalysis/testdata/policies/approval.rego`, `test/integration/aianalysis/testdata/policies/approval.rego`) use scored risk factors, a configurable `confidence_threshold`, and additional rules for infrastructure-provisioning action types and sensitive resource kinds — see [Rego Policy Examples](./REGO_POLICY_EXAMPLES.md).

---

## Phase 4: Completed

**Purpose**: Terminal state - output ready for RO consumption

**Timeout**: None (terminal)

### Completion Reason Taxonomy

Per K8s convention, `status.reason` covers all terminal states (success and failure).
When analysis completes successfully with a workflow, `reason` is set to `AnalysisCompleted`:

| Reason | Terminal Phase | Description |
|--------|---------------|-------------|
| `AnalysisCompleted` | Completed | Successful analysis with workflow selected |
| `WorkflowNotNeeded` | Completed | Success — problem self-resolved or not actionable |

### Status After Completion

```yaml
status:
  phase: "Completed"
  reason: "AnalysisCompleted"
  completionTime: "2025-11-30T10:00:45Z"

  # Workflow recommendation (from Kubernaut Agent (KA))
  # workflowSnapshot fields (executionBundle, etc.) are inline-embedded, not nested
  selectedWorkflow:
    workflowId: "wf-memory-increase-v2"
    executionBundle: "ghcr.io/kubernaut/workflows/memory-increase:v2.1.0"
    parameters:
      targetDeployment: "payment-api"
      memoryIncrease: "512Mi"
      namespace: "production"
    confidence: 0.87
    reasoning: "Historical success rate 92% for similar OOM scenarios"

  # Approval decision (from Rego policy)
  approvalRequired: true
  approvalReason: "Production environment requires manual approval"

  # Investigation summary (for operator context)
  investigationSummary: "OOMKilled due to memory leak in payment processing coroutine"

  # Phase timing
  phaseTransitions:
    Pending: "2025-11-30T10:00:00Z"
    Investigating: "2025-11-30T10:00:01Z"
    Analyzing: "2025-11-30T10:00:40Z"
    Completed: "2025-11-30T10:00:45Z"
```

### What Happens Next (RO Responsibility)

1. **RO watches** `AIAnalysis.status.phase == "Completed"`
2. **RO checks** `status.approvalRequired`:
   - **If false**: Create WorkflowExecution CRD immediately
   - **If true**: RO creates a `RemediationApprovalRequest` CRD and a notification (Slack/Console), then waits for operator approval before creating WorkflowExecution

---

## Phase 5: Failed

**Purpose**: Terminal failure state with structured reason

**Timeout**: None (terminal)

### Failure Reason Taxonomy (BR-KA-197)

AIAnalysis uses a structured taxonomy with `reason` (umbrella category) and `subReason` (specific cause).
For the complete set of valid `reason` values see `AIAnalysisReason` in `api/aianalysis/v1alpha1/aianalysis_types.go`.

| Reason (Umbrella) | SubReason | Description |
|-------------------|-----------|-------------|
| `WorkflowResolutionFailed` | `WorkflowNotFound` | LLM hallucinated a workflow that doesn't exist |
| `WorkflowResolutionFailed` | `ImageMismatch` | LLM provided wrong container image |
| `WorkflowResolutionFailed` | `ParameterValidationFailed` | Parameters don't conform to schema |
| `WorkflowResolutionFailed` | `NoMatchingWorkflows` | Catalog has no matching workflows |
| `WorkflowResolutionFailed` | `LowConfidence` | AI confidence below 70% threshold |
| `WorkflowResolutionFailed` | `LLMParsingError` | Cannot parse LLM response |
| `NoWorkflowSelected` | — | Investigation completed but no workflow in status |
| `RegoEvaluationError` | — | Rego policy evaluation failed unexpectedly |
| `TransientError` | Various | Temporary failure, retry recommended |
| `APIError` | Various | Permanent API/LLM error |

### Failed Status Example

```yaml
status:
  phase: "Failed"
  reason: "WorkflowResolutionFailed"
  subReason: "WorkflowNotFound"
  message: "Workflow validation failed: workflow 'restart-pod-v1' not found in catalog"

  # Partial response preserved for operator context
  selectedWorkflow:
    workflowId: "restart-pod-v1"  # Invalid - not in catalog
    confidence: 0.85
    reasoning: "Historical success with similar OOM scenarios"

  phaseTransitions:
    Pending: "2025-12-06T10:00:00Z"
    Investigating: "2025-12-06T10:00:01Z"
    Failed: "2025-12-06T10:00:05Z"
```

### What Happens Next (RO Responsibility on Failure)

1. **RO watches** `AIAnalysis.status.phase == "Failed"`
2. **RO checks** `status.reason`:
   - **If `WorkflowResolutionFailed`**: Notify operator, require manual intervention
   - **If `TransientError`**: May trigger recovery attempt (up to max retries)
3. **No WorkflowExecution** is created for failed AIAnalysis

---

## Recovery Attempts

> **⚠️ Correction (#1806)**: This section previously described `spec.isRecoveryAttempt`,
> `spec.recoveryAttemptNumber`, and `spec.previousExecutions` fields. As of the current
> `AIAnalysisSpec` (`api/aianalysis/v1alpha1/aianalysis_types.go`), **none of these fields
> exist** — there is no recovery-attempt input contract on the CRD spec today. The recovery/retry
> flow design (`DD-RECOVERY-002`) was archived/deleted when the recovery flow itself was
> deprecated (Issue #180) — recovery now flows through Gateway re-fire (BR-AA-KA-064.9), not a
> dedicated AIAnalysis spec field. If/when a recovery-attempt input contract ships, it should be
> documented here against the real spec fields rather than the illustrative example below.

---

## CRD-Based Coordination

### Watch Patterns

```
RemediationOrchestrator
    ↓ (watches SignalProcessing completion)
SignalProcessing.status.phase == "Completed"
    ↓ (creates AIAnalysis with enrichmentResults)
AIAnalysis CRD created
    ↓ (AIAnalysis controller watches)
AIAnalysis.status.phase == "Completed"
    ↓ (RO watches for completion)
RemediationOrchestrator creates WorkflowExecution (if approved)
```

### No Direct HTTP Calls Between Controllers

**Correct Pattern**: CRD status updates + Kubernetes watches

**Benefits**:
- ✅ **Reliability**: Status persists in etcd
- ✅ **Observability**: `kubectl get aianalysis` shows state
- ✅ **Decoupling**: Controllers don't know about each other's endpoints

---

## Phase Timeout Configuration

### Default Timeouts

| Phase | Default | Configurable |
|-------|---------|--------------|
| Pending | Immediate | No |
| Investigating | 25 minutes (wall-clock cap on the async KA session) | Yes — via `WithMaxInvestigationDuration(d time.Duration)` functional option on `InvestigatingHandler` (controller-wide, not per-CRD-instance) |
| Analyzing | 5s | No (not currently configurable) |
| Completed | N/A | No |

### ⚠️ Correction (#1806)

> This section previously described a deprecated annotation-based timeout
> (`kubernaut.ai/investigating-timeout`) and a "target implementation" spec-field timeout
> (`spec.timeoutConfig` / `AIAnalysisTimeoutConfig`). Neither drives real behavior today:
> - The annotation was never implemented anywhere in the codebase.
> - `spec.timeoutConfig`/`AIAnalysisTimeoutConfig` **is** defined on `AIAnalysisSpec`
>   (`api/aianalysis/v1alpha1/aianalysis_types.go`) and populated by RO from
>   `RemediationRequest.Status.TimeoutConfig.AIAnalysisTimeout`, but it is **not read** by
>   `InvestigatingHandler` or `AnalyzingHandler` — there are zero references to it outside the
>   type definition. It is populated on the wire but currently unwired/unconsumed.
>
> The timeout that is actually enforced is `DefaultMaxInvestigationDuration` (25 minutes,
> `pkg/aianalysis/handlers/constants.go`), a wall-clock cap on the whole async KA session,
> checked via `checkInvestigationTimeout` (`pkg/aianalysis/handlers/investigating.go`) and
> configurable only at handler-construction time via `WithMaxInvestigationDuration`, not
> per-CRD-instance.

---

## Metrics

> **⚠️ Correction (#1806)**: This section previously described `kubernaut_aianalysis_phase_duration_seconds`
> and `kubernaut_aianalysis_phase_transitions_total` metrics. **Neither exists** in
> `pkg/aianalysis/metrics/metrics.go` — there are no generic phase-duration or
> phase-transition metrics, and no client-side Kubernaut Agent (KA) latency metric (KA tracks
> its own server-side metrics). The AIAnalysis controller emits exactly 4 metrics; see
> [metrics-slos.md](./metrics-slos.md) for the authoritative list, label sets, SLO
> definitions, and alert rules.

### Key Metrics

| Metric | Purpose |
|--------|---------|
| `aianalysis_confidence_score_distribution{signal_type}` | Distribution of AI confidence scores (Analyzing phase) |
| `aianalysis_rego_evaluations_total{outcome,degraded}` | Rego policy evaluation outcomes/reliability |
| `aianalysis_approval_decisions_total{decision,environment}` | Auto-approve vs. manual-approval rate |
| `aianalysis_failures_total{reason,sub_reason}` | Failure rate by reason/sub-reason taxonomy |

---

## Related Documents

| Document | Purpose |
|----------|---------|
| [Overview](./overview.md) | Service architecture |
| [Controller Implementation](./controller-implementation.md) | Reconciler logic |
| [Rego Policy Examples](./REGO_POLICY_EXAMPLES.md) | Approval policy patterns |
| `DD-RECOVERY-002` | Recovery flow design (archived/deleted, Issue #180 recovery-flow deprecation; no longer on disk) |
