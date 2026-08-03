## Integration Points

> **Note**: The Go code excerpts below are drawn directly from the current production source
> (linked inline) and simplified for readability (error handling, logging, and some struct
> fields are elided with `// ...`). Where a snippet is illustrative rather than a direct
> excerpt, it is marked as such. Verify against the linked source before depending on exact
> field names for new code.

### 1. Upstream Integration: Gateway Service

**Integration Pattern**: Gateway creates RemediationRequest CRD; deduplication/dispatch (new
vs. existing) happens in Gateway's `processing` pipeline before this is called.

**How RemediationRequest is Created** (excerpted from [`pkg/gateway/processing/crd_creator.go`](../../../../pkg/gateway/processing/crd_creator.go)):

```go
// CRDCreator.CreateRemediationRequest builds and persists the RemediationRequest CRD
// from a normalized signal (adapter-agnostic; Prometheus/K8s-event/etc. all funnel
// through the same NormalizedSignal shape upstream of this call).
func (c *CRDCreator) CreateRemediationRequest(
    ctx context.Context,
    signal *types.NormalizedSignal,
) (*remediationv1alpha1.RemediationRequest, error) {
    // V1.0 is Kubernetes-only: reject signals missing resource Kind/Name with HTTP 400.
    if err := c.validateResourceInfo(signal); err != nil {
        return nil, err
    }

    crdName := generateCRDName(signal.Fingerprint) // "rr-{fingerprint[:12]}-{uuid[:8]}"
    rr := c.buildRemediationRequestCRD(crdName, signal)

    if err := c.createCRDWithRetry(ctx, rr, signal); err != nil {
        if k8serrors.IsAlreadyExists(err) {
            return c.recoverExistingCRDOnConflict(ctx, crdName, signal, startTime)
        }
        return nil, c.buildCreationFailureError(err, crdName, signal, startTime)
    }
    return rr, nil
}

// buildRemediationRequestCRD constructs the (unpersisted) CRD. ADR-057: created in the
// controller namespace, not the signal's own namespace.
func (c *CRDCreator) buildRemediationRequestCRD(crdName string, signal *types.NormalizedSignal) *remediationv1alpha1.RemediationRequest {
    return &remediationv1alpha1.RemediationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      crdName,
            Namespace: c.controllerNamespace,
            Labels: map[string]string{
                "app.kubernetes.io/managed-by": "gateway-service",
                "app.kubernetes.io/component":  "remediation",
            },
        },
        Spec: remediationv1alpha1.RemediationRequestSpec{
            // Core signal identification
            SignalFingerprint: signal.Fingerprint,
            SignalName:        signal.SignalName,

            // Classification — Environment/Priority are NOT here; SignalProcessing
            // classifies and owns those in its own Status (see §2.1).
            Severity:     signal.Severity,
            SignalType:   signal.SourceType,
            SignalSource: signal.Source,
            TargetType:   "kubernetes", // V1.0 supports Kubernetes only

            ClusterID: signal.ClusterID, // ADR-065: multi-cluster federation

            // Target resource (REQUIRED; validated above)
            TargetResource: c.buildTargetResource(signal), // ResourceIdentifier{Kind, Name, Namespace}

            FiringTime:   metav1.NewTime(c.getFiringTime(signal)), // falls back to ReceivedTime if unset
            ReceivedTime: metav1.NewTime(signal.ReceivedTime),

            SignalLabels:      c.truncateLabelValues(signal.Labels),
            SignalAnnotations: c.truncateAnnotationValues(signal.Annotations),

            // Provider-specific JSON (e.g. {"namespace": ..., "labels": ...}); NOT
            // base64-encoded — both this and OriginalPayload are plain `string` fields.
            ProviderData:    string(c.buildProviderData(signal)),
            OriginalPayload: string(signal.RawPayload),

            // Deduplication lives in RR.Status (Gateway-owned section), not Spec —
            // there is no spec-level Deduplication field.
        },
    }
}
```

**Note**: Gateway creates ONLY the RemediationRequest CRD. There are no `AlertName`,
`Environment`, `Priority`, `StormType`/`StormWindow`, or spec-level `Deduplication` fields on
`RemediationRequestSpec` — Environment/Priority are classified downstream by SignalProcessing
(see §2.1) and there is no storm-detection field anywhere on this CRD. The Remediation
Orchestrator (this service) reconciles the new RR and creates all downstream service CRDs.

---

### 2. Downstream Integration: Service CRD Creation & Watching

**Integration Pattern**: Dedicated `creator.*Creator` components (in
[`pkg/remediationorchestrator/creator/`](../../../../pkg/remediationorchestrator/creator/)),
invoked by the reconciler's phase-handling code, build and persist each child CRD with an
owner reference back to the parent RemediationRequest (cascade deletion, BR-ORCH-031) and
idempotent get-before-create semantics (safe to call again on requeue).

#### **2.1. SignalProcessing CRD Creation**

Excerpted from [`pkg/remediationorchestrator/creator/signalprocessing.go`](../../../../pkg/remediationorchestrator/creator/signalprocessing.go):

```go
// SignalProcessingCreator.Create is idempotent: a pre-existing CRD of the deterministic
// name is reused rather than re-created.
func (c *SignalProcessingCreator) Create(ctx context.Context, rr *remediationv1.RemediationRequest) (string, error) {
    name := fmt.Sprintf("sp-%s", rr.Name)
    // ... idempotency check (Get, return existing name if found) ...

    sp := c.buildSignalProcessing(rr, name)
    if err := controllerutil.SetControllerReference(rr, sp, c.scheme); err != nil {
        return "", fmt.Errorf("failed to set owner reference: %w", err)
    }
    if err := c.client.Create(ctx, sp); err != nil {
        return "", fmt.Errorf("failed to create SignalProcessing: %w", err)
    }
    return name, nil
}

// buildSignalProcessing constructs the CRD with data pass-through from rr (BR-ORCH-025).
func (c *SignalProcessingCreator) buildSignalProcessing(rr *remediationv1.RemediationRequest, name string) *signalprocessingv1.SignalProcessing {
    return &signalprocessingv1.SignalProcessing{
        ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: rr.Namespace},
        Spec: signalprocessingv1.SignalProcessingSpec{
            // Local ObjectReference type (not corev1.ObjectReference) — SignalProcessing
            // defines its own lightweight reference shape.
            RemediationRequestRef: signalprocessingv1.ObjectReference{
                APIVersion: remediationv1.GroupVersion.String(),
                Kind:       "RemediationRequest",
                Name:       rr.Name,
                Namespace:  rr.Namespace,
                UID:        string(rr.UID),
            },
            // Signal is a flat SignalData struct (not a bespoke "Signal" type) — field names
            // are Type/Source, not SignalType/SignalSource, and TargetResource nests inside it.
            Signal: signalprocessingv1.SignalData{
                Fingerprint:    rr.Spec.SignalFingerprint,
                Name:           rr.Spec.SignalName,
                Severity:       rr.Spec.Severity,
                Type:           rr.Spec.SignalType,
                Source:         rr.Spec.SignalSource,
                ClusterID:      rr.Spec.ClusterID,
                TargetType:     rr.Spec.TargetType,
                Labels:         rr.Spec.SignalLabels,
                Annotations:    rr.Spec.SignalAnnotations,
                FiringTime:     &rr.Spec.FiringTime,
                ReceivedTime:   rr.Spec.ReceivedTime,
                ProviderData:   rr.Spec.ProviderData,
                TargetResource: c.buildTargetResource(rr), // ResourceIdentifier{Kind, Name, Namespace}
            },
            // No DeduplicationContext, StormType, or StormWindow fields exist on
            // SignalProcessingSpec — storm detection is not implemented at this layer.
        },
    }
}
```

SignalProcessing then classifies Environment/Priority and enriches Kubernetes context
independently; those results land on `SignalProcessingStatus` (`EnvironmentClassification`,
`PriorityAssignment`, `KubernetesContext`, `BusinessClassification`), which the next step reads.

---

#### **2.2. AIAnalysis CRD Creation**

Excerpted from [`pkg/remediationorchestrator/creator/aianalysis.go`](../../../../pkg/remediationorchestrator/creator/aianalysis.go):

```go
// AIAnalysisCreator.Create uses the now-completed SignalProcessing to build the
// enriched analysis request. Same idempotent get-before-create pattern as §2.1.
func (c *AIAnalysisCreator) Create(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
    sp *signalprocessingv1.SignalProcessing,
) (string, error) {
    name := fmt.Sprintf("ai-%s", rr.Name)
    // ... idempotency check ...
    ai := c.buildAIAnalysis(rr, sp, name)
    if err := controllerutil.SetControllerReference(rr, ai, c.scheme); err != nil {
        return "", fmt.Errorf("failed to set owner reference: %w", err)
    }
    return name, c.client.Create(ctx, ai)
}

func (c *AIAnalysisCreator) buildAIAnalysis(rr *remediationv1.RemediationRequest, sp *signalprocessingv1.SignalProcessing, name string) *aianalysisv1.AIAnalysis {
    return &aianalysisv1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: rr.Namespace},
        Spec: aianalysisv1.AIAnalysisSpec{
            // corev1.ObjectReference (same type used by WorkflowExecutionRef/
            // SignalProcessingRef on RemediationRequest.Status) — not a bespoke type.
            RemediationRequestRef: corev1.ObjectReference{
                APIVersion: remediationv1.GroupVersion.String(),
                Kind:       "RemediationRequest",
                Name:       rr.Name,
                Namespace:  rr.Namespace,
                UID:        rr.UID,
            },
            RemediationID: rr.Name, // DD-AUDIT-CORRELATION-001
            AnalysisRequest: aianalysisv1.AnalysisRequest{
                SignalContext: c.buildSignalContext(rr, sp),
                AnalysisTypes: []aianalysisv1.AnalysisType{
                    aianalysisv1.AnalysisTypeInvestigation,
                    aianalysisv1.AnalysisTypeRootCause,
                    aianalysisv1.AnalysisTypeWorkflowSelection,
                },
            },
            ClusterID: rr.Spec.ClusterID,
        },
    }
}

// buildSignalContext: Environment/Priority/SignalType are read from SP's *Status*
// (SP now owns this classification), not from RR.Spec — those fields were removed
// from RemediationRequestSpec.
func (c *AIAnalysisCreator) buildSignalContext(rr *remediationv1.RemediationRequest, sp *signalprocessingv1.SignalProcessing) aianalysisv1.SignalContextInput {
    environment := "Unknown" // default when SP hasn't classified yet
    if sp.Status.EnvironmentClassification != nil && sp.Status.EnvironmentClassification.Environment != "" {
        environment = string(sp.Status.EnvironmentClassification.Environment)
    }
    priority := "P2" // default when SP hasn't classified yet
    if sp.Status.PriorityAssignment != nil && sp.Status.PriorityAssignment.Priority != "" {
        priority = string(sp.Status.PriorityAssignment.Priority)
    }
    signalType := sp.Status.SignalName // BR-SP-106: normalized (e.g. "PredictedOOMKill" -> "OOMKilled")
    if signalType == "" {
        signalType = rr.Spec.SignalType // fallback for backwards compatibility
    }

    return aianalysisv1.SignalContextInput{
        Fingerprint:      rr.Spec.SignalFingerprint,
        Severity:         sp.Status.Severity, // DD-SEVERITY-001: Rego-normalized, not rr.Spec.Severity
        SignalName:       signalType,
        SignalMode:       sp.Status.SignalMode, // BR-AI-084: proactive/reactive, for KA prompt switching
        Environment:      environment,
        BusinessPriority: priority,
        TargetResource: aianalysisv1.TargetResource{
            Kind: rr.Spec.TargetResource.Kind, Name: rr.Spec.TargetResource.Name,
            Namespace: rr.Spec.TargetResource.Namespace, APIVersion: rr.Spec.TargetResource.APIVersion,
        },
        EnrichmentResults: c.buildEnrichmentResults(sp), // KubernetesContext + BusinessClassification from sp.Status
        SignalAnnotations: rr.Spec.SignalAnnotations,
        Cluster:           sp.Status.ClusterClassification, // BR-FLEET-003, optional
    }
}
```

There is no `SignalProcessingStatus.EnrichedSignal` field, and `AIAnalysisSpec` has no
`HolmesGPTConfig`/per-request KA tool-config field — Kubernaut Agent's own investigation
configuration is owned by AIAnalysis's controller/KA itself, not passed in by RO.

---

#### **2.3. WorkflowExecution CRD Creation**

Excerpted from [`pkg/remediationorchestrator/creator/workflowexecution.go`](../../../../pkg/remediationorchestrator/creator/workflowexecution.go):

```go
// WorkflowExecutionCreator.Create fails closed (validateSelectedWorkflow) if AIAnalysis's
// selected workflow is missing any Required field before ever attempting CRD creation —
// an incompletely-enriched snapshot must not silently reach an executor.
func (c *WorkflowExecutionCreator) Create(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
    ai *aianalysisv1.AIAnalysis,
) (string, error) {
    if err := validateSelectedWorkflow(ai); err != nil {
        return "", err
    }
    name := fmt.Sprintf("we-%s", rr.Name)
    // ... idempotency check ...
    we := c.buildWorkflowExecution(rr, ai, name)
    if err := controllerutil.SetControllerReference(rr, we, c.scheme); err != nil {
        return "", fmt.Errorf("failed to set owner reference: %w", err)
    }
    return name, c.client.Create(ctx, we)
}

func (c *WorkflowExecutionCreator) buildWorkflowExecution(rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis, name string) *workflowexecutionv1.WorkflowExecution {
    return &workflowexecutionv1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: rr.Namespace},
        Spec: workflowexecutionv1.WorkflowExecutionSpec{
            RemediationRequestRef: corev1.ObjectReference{ /* same shape as §2.2 */ },

            // WorkflowRef inline-embeds sharedtypes.WorkflowSnapshot — pure field-for-field
            // pass-through from AIAnalysis.Status.SelectedWorkflow (DD-WORKFLOW-018). There
            // is NO ContainerImage/ContainerDigest on this type; the real fields are
            // ExecutionBundle/ExecutionBundleDigest (OCI bundle reference), plus
            // ExecutionEngine/ServiceAccountName/ActionType/Dependencies/Resources.
            WorkflowRef: buildWorkflowRef(ai.Status.SelectedWorkflow),

            // TargetResource is a plain "namespace/kind/name" STRING (not a struct) —
            // format used for resource locking (DD-WE-001). BR-KA-212: prefers the LLM's
            // RootCauseAnalysis.RemediationTarget over rr.Spec.TargetResource when the LLM
            // identified a different (usually higher-level, e.g. Deployment vs Pod) target.
            TargetResource: resolveTargetResource(rr, ai),

            ClusterID:  rr.Spec.ClusterID,
            Parameters: ai.Status.SelectedWorkflow.Parameters,
            Confidence: ai.Status.SelectedWorkflow.Confidence,
            Rationale:  ai.Status.SelectedWorkflow.Rationale,

            // ExecutionConfig carries ONLY a timeout — ServiceAccountName now lives on
            // WorkflowRef (WorkflowSnapshot.ServiceAccountName), not here.
            ExecutionConfig: c.buildExecutionConfig(rr), // *ExecutionConfig{Timeout *metav1.Duration}, nil if unset
        },
    }
}
```

**`ExecutionConfig`** (from `api/workflowexecution/v1alpha1`) is a single-field struct
(`Timeout *metav1.Duration`) sourced from `rr.Status.TimeoutConfig.Executing` when set —
there is no `ServiceAccountName` field here (DD-WORKFLOW-018 moved it onto `WorkflowRef`).

---

#### **2.4. KubernetesExecution (REMOVED — ADR-025)**

`KubernetesExecution` was fully removed per ADR-025 and has **zero remaining Go references**
anywhere in this repository — no types, no controller, no creator, no watch. WorkflowExecution
is the terminal executor CRD for remediation actions (via Tekton/Ansible/Job execution
engines, resolved from `WorkflowRef.ExecutionEngine`). This subsection is retained only as a
historical marker for anyone searching prior design docs that still reference
`KubernetesExecution` or `executorv1`.

---

### 3. Watch Configuration for Event-Driven Coordination

**Integration Pattern**: Owner-reference-based watches (`Owns()`), not manual
`Watches()` + `EnqueueRequestsFromMapFunc` mapping functions — since every child CRD created
in §2 carries a controller owner reference back to the parent RemediationRequest
(`controllerutil.SetControllerReference`), controller-runtime's built-in owner-reference
resolution enqueues the parent automatically on any child status change.

Excerpted from [`internal/controller/remediationorchestrator/reconcile_loop.go`](../../../../internal/controller/remediationorchestrator/reconcile_loop.go):

```go
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Field indexes used by routing/lookup logic (not watches, but registered here):
    // - spec.signalFingerprint on RemediationRequest (O(1) consecutive-failure lookups, BR-ORCH-042)
    // - spec.targetResource on WorkflowExecution (O(1) centralized-routing queries, DD-RO-002)
    // - spec.remediationRequestRef.name on every child CRD kind (O(1) parent lookups, Issue #91)
    if err := registerFingerprintIndex(mgr); err != nil {
        return err
    }
    if err := registerWFETargetResourceIndex(mgr); err != nil {
        return err
    }
    if err := registerChildCRDIndexes(mgr); err != nil {
        return err
    }

    return ctrl.NewControllerManagedBy(mgr).
        For(&remediationv1.RemediationRequest{}).
        Owns(&signalprocessingv1.SignalProcessing{}).
        Owns(&aianalysisv1.AIAnalysis{}).
        Owns(&workflowexecutionv1.WorkflowExecution{}).
        Owns(&remediationv1.RemediationApprovalRequest{}).
        Owns(&notificationv1.NotificationRequest{}).      // BR-ORCH-029/030: notification lifecycle
        Owns(&eav1.EffectivenessAssessment{}).             // ADR-EM-001: EffectivenessAssessed condition
        // GenerationChangedPredicate is intentionally NOT applied — child CRD *status*
        // changes (not just spec/generation changes) must trigger reconciliation.
        Complete(r)
}
```

There is no `KubernetesExecution` watch (see §2.4) and no per-kind
`{kind}ToRemediation` mapping functions — `Owns()` handles that resolution internally via the
child's owner reference.

---

### 3.5. Blocking / Routing (DD-RO-002 — Centralized Routing Responsibility)

**Design Decision Reference**: [DD-RO-002](../../../architecture/decisions/DD-RO-002-centralized-routing-responsibility.md),
[DD-RO-002-ADDENDUM](../../../architecture/decisions/DD-RO-002-ADDENDUM-blocked-phase-semantics.md)
(supersedes the older per-WFE `Skipped`-phase model in
[DD-RO-001](../../../architecture/decisions/DD-RO-001-resource-lock-deduplication-handling.md),
which is now historical)
**Business Requirements**: BR-ORCH-032, BR-ORCH-033, BR-ORCH-034, BR-ORCH-042

**Integration Pattern**: RO evaluates ALL blocking conditions **before** creating the
WorkflowExecution CRD — not by watching a WFE `Skipped` status after the fact. See
[`pkg/remediationorchestrator/routing/blocking.go`](../../../../pkg/remediationorchestrator/routing/blocking.go).
`WorkflowExecutionStatus` has no `Skipped` phase and no `SkipDetails` field — WorkflowExecution
is a pure executor with no routing logic of its own (V1.0 simplification per DD-RO-002).

When RO determines a block condition applies, it sets `RemediationRequest.Status.OverallPhase =
"Blocked"` directly (a non-terminal phase — Gateway keeps deduplicating on the same fingerprint
while blocked) along with a typed `BlockReason`:

| `BlockReason` | Meaning | Resolution |
|---|---|---|
| `ConsecutiveFailures` | 3+ consecutive failures for this fingerprint | 1-hour cooldown, then retry (BR-ORCH-042) |
| `DuplicateInProgress` | Another active RR has the same fingerprint | Inherits the original's outcome on completion |
| `ResourceBusy` | Another WorkflowExecution is running on the same target | Proceeds once the target frees up |
| `RecentlyRemediated` | Same workflow+target executed recently | Cooldown (default 5 min), then proceeds (DD-WE-001) |
| `ExponentialBackoff` | Pre-execution failures require a backoff window | Retries after backoff (1m/2m/4m/8m capped at 10m, DD-WE-004) |
| `UnmanagedResource` | Target lacks the `kubernaut.ai/managed=true` label/namespace | Retries with backoff until labeled or RR times out (BR-SCOPE-001) |
| `IneffectiveChain` | Consecutive remediations for this target haven't improved health | Escalates to human review via NotificationRequest |

`RemediationRequest.Status` carries the full explanation (`BlockReason`, `BlockMessage`,
`BlockedUntil`, `BlockingWorkflowExecution`) as the single source of truth — this is a change
from the pre-V1.0 model where skip details were split across `WorkflowExecution.Status`.

For non-`IneffectiveChain` blocks, RO also creates a `NotificationRequest` via
`NotificationCreator.CreateBlockNotification` — `Escalation`/High priority for persistent
blocks (`ConsecutiveFailures`, `UnmanagedResource`) that need investigation, `StatusUpdate`/Low
priority for transient/auto-clearing ones. See §4 for the notification mechanism itself.

---

### 4. Notification Integration: NotificationRequest CRD (not HTTP)

**Integration Pattern**: CRD creation, not an HTTP call. RO never POSTs to a Notification
Service HTTP endpoint for any notification type (approval, completion, escalation, block,
manual-review, self-resolved, bulk-duplicate) — it creates a `NotificationRequest` CRD, owned
by the RemediationRequest (cascade deletion), which the separate Notification service
reconciles and delivers via its own routing rules (BR-NOT-065 — RO does not set `Channels`).

Excerpted from [`pkg/remediationorchestrator/creator/notification.go`](../../../../pkg/remediationorchestrator/creator/notification.go)
— the escalation path used for terminal remediation failures (analogous builders exist for
approval, completion, block, manual-review, self-resolved, and bulk-duplicate notifications;
`internal/controller/remediationorchestrator/timeout_handling.go` builds a similar
`Escalation`-type request inline for timeout events):

```go
// CreateEscalationNotification creates an Escalation NotificationRequest for terminal
// remediation failures (transitionToFailed / transitionToFailedTerminal).
func (c *NotificationCreator) CreateEscalationNotification(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
    escCtx *EscalationContext, // {FailurePhase, FailureReason, BlockReason, Message}
) (string, error) {
    name := fmt.Sprintf("nr-escalation-%s", rr.Name)
    // ... idempotency check ...

    nr := &notificationv1.NotificationRequest{
        ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: rr.Namespace},
        Spec: notificationv1.NotificationRequestSpec{
            // BR-NOT-064: parent reference for audit correlation/lineage tracking
            RemediationRequestRef: &corev1.ObjectReference{
                APIVersion: remediationv1.GroupVersion.String(),
                Kind: "RemediationRequest", Name: rr.Name, Namespace: rr.Namespace, UID: rr.UID,
            },
            ClusterID: rr.Spec.ClusterID,
            Type:      notificationv1.NotificationTypeEscalation,
            Priority:  notificationv1.NotificationPriorityHigh,
            Severity:  rr.Spec.Severity,
            Subject:   fmt.Sprintf("🚨 Remediation Failed: %s (phase: %s)", rr.Spec.SignalName, escCtx.FailurePhase),
            Body:      c.buildEscalationBody(rr, escCtx), // plain Markdown string, not a typed schema
        },
    }

    if err := controllerutil.SetControllerReference(rr, nr, c.scheme); err != nil {
        return "", fmt.Errorf("failed to set owner reference: %w", err)
    }
    if err := c.client.Create(ctx, nr); err != nil {
        return "", fmt.Errorf("failed to create NotificationRequest: %w", err)
    }
    return name, nil
    // Caller (reconciler) appends the created name to rr.Status.NotificationRequestRefs (BR-ORCH-035).
}
```

There is no `EscalationRequest` Go type, no `/api/v1/notify/escalation` HTTP endpoint, and no
`Channels`/`Urgency`/`EscalationDetails` fields anywhere in this flow — `NotificationRequestSpec`
is a much simpler `{RemediationRequestRef, ClusterID, Type, Priority, Subject, Body, Severity,
Phase, ReviewSource, Context}` shape (see
[`api/notification/v1alpha1/notificationrequest_types.go`](../../../../api/notification/v1alpha1/notificationrequest_types.go)),
with an optional structured `Context` (`LineageContext`, `WorkflowContext`, `AnalysisContext`,
`ReviewContext`, `VerificationContext`, `DedupContext`) used for programmatic routing/rendering
downstream, alongside the human-readable `Body` Markdown.

**Downstream: Approval Notifications (V1.0)**

**Business Requirement**: BR-ORCH-001 (approval notification), BR-ORCH-026 (approval
orchestration, ADR-040)

**Trigger**: `RemediationRequest.Status.OverallPhase == "AwaitingApproval"` (the
`RemediationPhase` enum value is `AwaitingApproval`, not `"Approving"`)

Two CRDs are created at this point, by two different creators:
- `creator.ApprovalCreator.Create` creates the `RemediationApprovalRequest` (RAR) CRD itself
  (the object an operator approves/rejects against; ADR-040, BR-ORCH-026).
- `creator.NotificationCreator.CreateApprovalNotification` creates the human-facing
  `NotificationRequest` (High priority) announcing it, with `Context.Lineage`/`Workflow`/
  `Analysis` carrying the RemediationRequest name, AIAnalysis name, selected workflow ID, and
  approval reason for structured rendering (e.g. Slack).

Both are owned by the RemediationRequest (cascade deletion via `SetControllerReference`).

---

### 5. Audit Integration: Unified Audit Events (ADR-034), not a bespoke table

**Integration Pattern**: Typed `api.AuditEventRequest` objects (ogen-generated from
DataStorage's OpenAPI spec), asynchronously buffered and batch-POSTed — not a synchronous,
per-call HTTP request to a `RemediationAudit`-shaped endpoint.

Excerpted from [`internal/controller/remediationorchestrator/audit_events.go`](../../../../internal/controller/remediationorchestrator/audit_events.go)
(one of many call sites — lifecycle-started, phase-transition, completion, failure, routing-blocked,
approval-requested/decided, and EA/workflow-created events all follow the same
build-then-store pattern):

```go
// Every audit call site follows this two-step pattern: build a typed event via
// auditManager (pkg/remediationorchestrator/audit), then hand it to auditStore.
event, err := r.auditManager.BuildRemediationCreatedEvent(
    correlationID, // RemediationRequest.Name — the universal correlation key across all services
    rr.Namespace,
    rr.Name,
    rr.Spec.ClusterID, // DD-AUDIT-003 v2.2: fleet cluster provenance (SOC2 CC8.1)
    timeoutConfig,
)
if err != nil {
    logger.Error(err, "Failed to build remediation created audit event")
    return
}
if err := r.auditStore.StoreAudit(ctx, event); err != nil {
    logger.Error(err, "Failed to store remediation created audit event")
    // Fail-open by design: StoreAudit failures are logged, not propagated —
    // audit degradation must never block remediation business logic.
}
```

`auditManager.Build*Event` (see [`pkg/remediationorchestrator/audit/manager.go`](../../../../pkg/remediationorchestrator/audit/manager.go))
populates a `*api.AuditEventRequest` per the ADR-034 schema (`event_id`, `event_type`,
`event_category`, `event_action`, `event_outcome`, `actor_type`/`actor_id`, `correlation_id`,
`namespace`, typed `event_data`). `auditStore.StoreAudit` (see
[`pkg/audit/store.go`](../../../../pkg/audit/store.go)) is **non-blocking**: it appends to an
in-memory buffer and returns immediately; a background worker flushes the buffer via the
generated OpenAPI client to `POST /api/v1/audit/events/batch` on DataStorage (see
[`pkg/audit/openapi_client_adapter.go`](../../../../pkg/audit/openapi_client_adapter.go)). If
the buffer is full, the event is dropped and an error is returned to the (already
non-propagating) caller — graceful degradation, never a blocked reconcile loop.

There is no `RemediationAudit`/`ServiceCRDStatus` Go type, and no
`POST /api/v1/audit/remediation` endpoint — that shape predates the ADR-034 unified
`audit_events` table and was never implemented as described.

---

### 6. Dependencies Summary

**Upstream Services**:
- **Gateway Service** — Creates RemediationRequest CRD (§1)

**Downstream Services** (child CRDs created and owned by RemediationRequest, §2–§3):
- **SignalProcessing Controller** — Enrichment & classification (owns Environment/Priority/KubernetesContext)
- **AIAnalysis Controller** — Kubernaut Agent (KA) investigation & workflow selection
- **WorkflowExecution Controller** — Remediation execution (Tekton/Ansible/Job, per `ExecutionEngine`)
- **RemediationApprovalRequest** (no separate controller — reconciled by RO itself) — Human approval gate (ADR-040)
- **NotificationRequest** — Delivered by the Notification service; created for approval, completion, escalation, block, manual-review, self-resolved, and bulk-duplicate events (§4)
- **EffectivenessAssessment Controller** — Post-remediation effectiveness verification (ADR-EM-001)
- ~~KubernetesExecution Controller~~ — Removed (ADR-025); see §2.4

**External Services**:
- **DataStorage Service** — `POST /api/v1/audit/events(/batch)` for ADR-034 audit persistence (§5); no HTTP call for notifications (those are CRD-based, §4)

**Database** (owned by DataStorage, not directly by RO):
- PostgreSQL — unified `audit_events` table (ADR-034), queryable by `correlation_id` for full remediation-lifecycle reconstruction (SOC2 CC8.1)
