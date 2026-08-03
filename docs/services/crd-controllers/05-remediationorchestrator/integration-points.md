## Integration Points

> **Note**: The Go code examples below illustrate integration patterns; several field-level
> details (e.g., `SignalProcessingSpec`/`AIAnalysisSpec`/`WorkflowExecutionSpec` literals) have
> drifted from the current CRD schemas beyond the dead-type-mixing fixed in
> [#1879](https://github.com/jordigilh/kubernaut/issues/1879). See
> [#1880](https://github.com/jordigilh/kubernaut/issues/1880) for the tracked full accuracy pass;
> verify against `api/*/v1alpha1` source before copying these snippets.

### 1. Upstream Integration: Gateway Service

**Integration Pattern**: Gateway creates RemediationRequest CRD with duplicate detection already performed

**How RemediationRequest is Created**:
```go
// In Gateway Service - Creates ONLY RemediationRequest CRD
func (g *GatewayService) HandleWebhook(ctx context.Context, payload []byte) error {
    alertFingerprint := extractFingerprint(payload)

    // 1. Check for existing active remediation (duplicate detection)
    existingRemediation, err := g.checkExistingRemediation(ctx, alertFingerprint)
    if err != nil {
        return fmt.Errorf("failed to check existing remediation: %w", err)
    }

    if existingRemediation != nil {
        return g.handleDuplicateAlert(ctx, existingRemediation, payload)
    }

    // 2. No existing remediation - create ONLY RemediationRequest CRD
    // RemediationRequest controller will create service CRDs
    requestID := generateRequestID()
    alertRemediation := &remediationv1.RemediationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("remediation-%s", requestID),
            Namespace: "kubernaut-system",
            Labels: map[string]string{
                "signal.fingerprint": signalFingerprint,
                "signal.severity":    extractSeverity(payload),
                "signal.environment": extractEnvironment(payload),
            },
        },
        Spec: remediationv1.RemediationRequestSpec{
            // Core Signal Identification
            SignalFingerprint: signalFingerprint,
            SignalName:        extractSignalName(payload),
            Severity:          extractSeverity(payload),
            Environment:       extractEnvironment(payload),
            Priority:          extractPriority(payload),
            SignalType:        extractSignalType(payload),
            SignalSource:      extractSignalSource(payload),
            TargetType:        "kubernetes", // V1 only supports Kubernetes

            // Target Resource (REQUIRED per Gateway contract)
            TargetResource: remediationv1.ResourceIdentifier{
                Kind:      extractResourceKind(payload),
                Name:      extractResourceName(payload),
                Namespace: extractResourceNamespace(payload), // Empty for cluster-scoped
            },

            // Temporal Data
            FiringTime:   extractFiringTime(payload),
            ReceivedTime: metav1.Now(),

            // Deduplication (shared type with SignalProcessing)
            Deduplication: sharedtypes.DeduplicationInfo{
                IsDuplicate:       false,
                FirstOccurrence:   metav1.Now(),
                LastOccurrence:    metav1.Now(),
                OccurrenceCount:   1,
                CorrelationID:     extractCorrelationID(payload),
            },

            // Provider Data
            ProviderData:    payload,
            OriginalPayload: payload,
        },
    }

    // Create RemediationRequest - Controller will create service CRDs
    return g.k8sClient.Create(ctx, alertRemediation)
}
```

**Note**: Gateway Service creates ONLY RemediationRequest CRD. RemediationRequest controller creates all service CRDs.

---

### 2. Downstream Integration: Service CRD Creation & Watching

**Integration Pattern**: Watch-based event-driven coordination

#### **2.1. SignalProcessing CRD Creation**

```go
// In RemediationRequestReconciler
func (r *RemediationRequestReconciler) createSignalProcessing(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
) error {
    signalProcessing := &signalprocessingv1.SignalProcessing{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-processing", remediation.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: signalprocessingv1.SignalProcessingSpec{
            RemediationRequestRef: signalprocessingv1.RemediationRequestReference{
                Name:      remediation.Name,
                Namespace: remediation.Namespace,
            },
            // DATA SNAPSHOT: Copy signal data from RemediationRequest
            Signal: signalprocessingv1.Signal{
                Fingerprint:  remediation.Spec.SignalFingerprint,
                Name:         remediation.Spec.SignalName,
                Severity:     remediation.Spec.Severity,
                Environment:  remediation.Spec.Environment,
                Priority:     remediation.Spec.Priority,
                SignalType:   remediation.Spec.SignalType,
                SignalSource: remediation.Spec.SignalSource,
                TargetType:   remediation.Spec.TargetType,
            },
            // Target Resource (REQUIRED)
            TargetResource: signalprocessingv1.ResourceIdentifier{
                Kind:      remediation.Spec.TargetResource.Kind,
                Name:      remediation.Spec.TargetResource.Name,
                Namespace: remediation.Spec.TargetResource.Namespace,
            },
            // Deduplication context (shared type)
            DeduplicationContext: remediation.Spec.Deduplication,
            // Storm detection
            StormType:   remediation.Spec.StormType,
            StormWindow: remediation.Spec.StormWindow,
        },
    }

    if err := r.Create(ctx, signalProcessing); err != nil {
        return fmt.Errorf("failed to create SignalProcessing: %w", err)
    }

    // Update RemediationRequest with SignalProcessing reference
    // NOTE: RemediationRequestStatus also has a legacy RemediationProcessingRef field
    // (kept for API compatibility), but SignalProcessingRef is the one actually
    // populated/read by the controller (pkg/remediationorchestrator/aggregator/status.go).
    remediation.Status.SignalProcessingRef = &corev1.ObjectReference{
        Name:      signalProcessing.Name,
        Namespace: signalProcessing.Namespace,
        Kind:      "SignalProcessing",
        APIVersion: signalprocessingv1.GroupVersion.String(),
    }

    return r.Status().Update(ctx, remediation)
}
```

---

#### **2.2. AIAnalysis CRD Creation**

```go
// In RemediationRequestReconciler
func (r *RemediationRequestReconciler) createAIAnalysis(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    signalProcessing *signalprocessingv1.SignalProcessing,
) error {
    // When SignalProcessing completes, create AIAnalysis with enriched data
    if signalProcessing.Status.Phase == "completed" {
        aiAnalysis := &aiv1.AIAnalysis{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("%s-analysis", remediation.Name),
                Namespace: remediation.Namespace,
                OwnerReferences: []metav1.OwnerReference{
                    *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
                },
            },
            Spec: aiv1.AIAnalysisSpec{
                RemediationRequestRef: aiv1.RemediationRequestReference{
                    Name:      remediation.Name,
                    Namespace: remediation.Namespace,
                },
                // DATA SNAPSHOT: Copy enriched signal context
                // NOTE: This AnalysisRequest literal is illustrative only and does not match
                // the current api/aianalysis/v1alpha1 shape (AnalysisRequest{SignalContext
                // SignalContextInput, AnalysisTypes []AnalysisType}; no InvestigationScope/
                // ResourceScope). SignalProcessingStatus also has no EnrichedSignal field —
                // enrichment results are split across KubernetesContext/EnvironmentClassification/
                // PriorityAssignment/BusinessClassification. Tracked for a follow-up accuracy pass.
                AnalysisRequest: aiv1.AnalysisRequest{
                    SignalContext: aiv1.SignalContextInput{
                        Fingerprint: signalProcessing.Status.EnrichedSignal.Fingerprint,
                        Severity:    signalProcessing.Status.EnrichedSignal.Severity,
                        Environment: signalProcessing.Status.EnrichedSignal.Environment,

                        // Resource targeting for Kubernaut Agent toolsets (NOT logs/metrics)
                        Namespace:    signalProcessing.Status.EnrichedSignal.Namespace,
                        ResourceKind: signalProcessing.Status.EnrichedSignal.ResourceKind,
                        ResourceName: signalProcessing.Status.EnrichedSignal.ResourceName,
                    },
                    AnalysisTypes: []aiv1.AnalysisType{"Investigation", "RootCause"},
                },
                // Kubernaut Agent (KA) investigation/tool configuration is NOT part of this spec — it is
                // owned and applied by the AIAnalysis controller / KA itself, not by Remediation Coordinator.
                // Remediation Coordinator only provides business data (targeting info, enriched context).
                // NOTE (2026-08-02, Issue #1806): this comment previously referenced a `HolmesGPTConfig` field,
                // which does not exist in the current AIAnalysis API (api/aianalysis/) — no equivalent
                // per-request config field exists today; corrected to avoid citing a fictional field.
            },
        }

        if err := r.Create(ctx, aiAnalysis); err != nil {
            return fmt.Errorf("failed to create AIAnalysis: %w", err)
        }

        // Update RemediationRequest with AIAnalysis reference
        // (Status.AIAnalysisRef is *corev1.ObjectReference, matching SignalProcessingRef/
        // WorkflowExecutionRef below — not a bespoke AIAnalysisReference type)
        remediation.Status.AIAnalysisRef = &corev1.ObjectReference{
            Name:       aiAnalysis.Name,
            Namespace:  aiAnalysis.Namespace,
            Kind:       "AIAnalysis",
            APIVersion: aiv1.GroupVersion.String(),
        }

        return r.Status().Update(ctx, remediation)
    }

    return nil
}
```

---

#### **2.3. WorkflowExecution CRD Creation**

```go
// In RemediationRequestReconciler
// Per DD-CONTRACT-001/002: RO passes through workflow data from AIAnalysis
func (r *RemediationRequestReconciler) createWorkflowExecution(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    aiAnalysis *aianalysisv1.AIAnalysis,
) error {
    // When AIAnalysis completes, create WorkflowExecution with recommendations
    if aiAnalysis.Status.Phase != "Completed" {
        return nil
    }

    // Get selected workflow from AIAnalysis (resolved by Kubernaut Agent)
    selectedWorkflow := aiAnalysis.Status.SelectedWorkflow

    workflowExecution := &workflowexecutionv1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-workflow", remediation.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: workflowexecutionv1.WorkflowExecutionSpec{
            RemediationRequestRef: workflowexecutionv1.RemediationRequestReference{
                Name:      remediation.Name,
                Namespace: remediation.Namespace,
            },
            // Target Resource (REQUIRED per DD-RO-001)
            TargetResource: buildTargetResource(remediation),

            // WorkflowRef: Pass-through from AIAnalysis.status.selectedWorkflow
            // DD-CONTRACT-001: RO does NOT perform catalog lookups
            // Kubernaut Agent resolves workflow_id to container_image
            WorkflowRef: workflowexecutionv1.WorkflowRef{
                WorkflowID:      selectedWorkflow.WorkflowID,
                ContainerImage:  selectedWorkflow.ContainerImage,   // From Kubernaut Agent
                ContainerDigest: selectedWorkflow.ContainerDigest,  // From Kubernaut Agent
            },

            // Parameters: Pass-through from AIAnalysis
            Parameters: selectedWorkflow.Parameters,

            // Execution Config — DD-WE-005 v2.0: pass through SA from catalog (e.g. RemediationWorkflow.execution.serviceAccountName → selectedWorkflow.ServiceAccountName)
            ExecutionConfig: workflowexecutionv1.ExecutionConfig{
                Timeout:            "20m",
                ServiceAccountName: selectedWorkflow.ServiceAccountName, // e.g. "my-workflow-sa"; empty → namespace default SA on PipelineRun
            },
        },
    }

    if err := r.Create(ctx, workflowExecution); err != nil {
        return fmt.Errorf("failed to create WorkflowExecution: %w", err)
    }

    // Update RemediationRequest with WorkflowExecution reference
    remediation.Status.WorkflowExecutionRef = &corev1.ObjectReference{
        Name:       workflowExecution.Name,
        Namespace:  workflowExecution.Namespace,
        Kind:       "WorkflowExecution",
        APIVersion: workflowexecutionv1.GroupVersion.String(),
    }

    return r.Status().Update(ctx, remediation)
}
```

---

#### **2.4. KubernetesExecution (DEPRECATED - ADR-025) CRD Creation**

```go
// In RemediationRequestReconciler
func (r *RemediationRequestReconciler) createKubernetesExecution(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    workflowExecution *workflowv1.WorkflowExecution,
) error {
    // When WorkflowExecution completes, create KubernetesExecution (DEPRECATED - ADR-025)
    if workflowExecution.Status.Phase == "completed" {
        kubernetesExecution := &executorv1.KubernetesExecution{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("%s-execution", remediation.Name),
                Namespace: remediation.Namespace,
                OwnerReferences: []metav1.OwnerReference{
                    *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
                },
            },
            Spec: executorv1.KubernetesExecutionSpec{
                RemediationRequestRef: executorv1.RemediationRequestReference{
                    Name:      remediation.Name,
                    Namespace: remediation.Namespace,
                },
                // DATA SNAPSHOT: Copy workflow operations
                Operations: workflowExecution.Status.Operations,
                ExecutionConfig: executorv1.ExecutionConfig{
                    DryRun:          false,
                    SafetyValidation: true,
                    Timeout:         "10m",
                },
            },
        }

        if err := r.Create(ctx, kubernetesExecution); err != nil {
            return fmt.Errorf("failed to create KubernetesExecution: %w", err)
        }

        // Update RemediationRequest with KubernetesExecution reference
        remediation.Status.KubernetesExecutionRef = &remediationv1.KubernetesExecutionReference{
            Name:      kubernetesExecution.Name,
            Namespace: kubernetesExecution.Namespace,
        }

        return r.Status().Update(ctx, remediation)
    }

    return nil
}
```

---

### 3. Watch Configuration for Event-Driven Coordination

**Controller Setup with Watches**:

```go
// In RemediationRequestReconciler SetupWithManager
func (r *RemediationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&remediationv1.RemediationRequest{}).

        // Watch SignalProcessing for completion
        Watches(
            &source.Kind{Type: &signalprocessingv1.SignalProcessing{}},
            handler.EnqueueRequestsFromMapFunc(r.signalProcessingToRemediation),
        ).

        // Watch AIAnalysis for completion
        Watches(
            &source.Kind{Type: &aiv1.AIAnalysis{}},
            handler.EnqueueRequestsFromMapFunc(r.aiAnalysisToRemediation),
        ).

        // Watch WorkflowExecution for completion
        Watches(
            &source.Kind{Type: &workflowv1.WorkflowExecution{}},
            handler.EnqueueRequestsFromMapFunc(r.workflowExecutionToRemediation),
        ).

        // Watch KubernetesExecution (DEPRECATED - ADR-025) for completion
        Watches(
            &source.Kind{Type: &executorv1.KubernetesExecution{}},
            handler.EnqueueRequestsFromMapFunc(r.kubernetesExecutionToRemediation),
        ).

        Complete(r)
}

// Map SignalProcessing to parent RemediationRequest
func (r *RemediationRequestReconciler) signalProcessingToRemediation(obj client.Object) []ctrl.Request {
    sp := obj.(*signalprocessingv1.SignalProcessing)
    return []ctrl.Request{
        {
            NamespacedName: types.NamespacedName{
                Name:      sp.Spec.RemediationRequestRef.Name,
                Namespace: sp.Spec.RemediationRequestRef.Namespace,
            },
        },
    }
}

// Map AIAnalysis to parent RemediationRequest
func (r *RemediationRequestReconciler) aiAnalysisToRemediation(obj client.Object) []ctrl.Request {
    ai := obj.(*aiv1.AIAnalysis)
    return []ctrl.Request{
        {
            NamespacedName: types.NamespacedName{
                Name:      ai.Spec.RemediationRequestRef.Name,
                Namespace: ai.Spec.RemediationRequestRef.Namespace,
            },
        },
    }
}

// Map WorkflowExecution to parent RemediationRequest
func (r *RemediationRequestReconciler) workflowExecutionToRemediation(obj client.Object) []ctrl.Request {
    we := obj.(*workflowexecutionv1.WorkflowExecution)
    return []ctrl.Request{
        {
            NamespacedName: types.NamespacedName{
                Name:      we.Spec.RemediationRequestRef.Name,
                Namespace: we.Spec.RemediationRequestRef.Namespace,
            },
        },
    }
}
```

---

### 3.5. WorkflowExecution Skipped Phase Integration (DD-RO-001)

**Design Decision Reference**: DD-RO-001 (Resource Lock Deduplication Handling)
**Business Requirements**: BR-ORCH-032, BR-ORCH-033, BR-ORCH-034

**Integration Pattern**: Watch WE status for Skipped phase

When WorkflowExecution returns `Skipped` phase (due to resource locking), RO handles it as follows:

**WE → RO Contract** (from DD-WE-001):

| Field | Direction | Description |
|-------|-----------|-------------|
| `status.phase` | WE → RO | Includes `Skipped` phase |
| `status.skipDetails.reason` | WE → RO | `ResourceBusy` or `RecentlyRemediated` |
| `status.skipDetails.conflictingWorkflow.remediationRequestRef` | WE → RO | Parent RR name (ResourceBusy) |
| `status.skipDetails.recentRemediation.remediationRequestRef` | WE → RO | Parent RR name (RecentlyRemediated) |

**Handler Implementation**:
```go
// In orchestratePhase for "executing" case
if workflowExecution.Status.Phase == "Skipped" {
    // DD-RO-001: Handle resource lock deduplication
    return r.handleWorkflowExecutionSkipped(ctx, remediation, &workflowExecution)
}
```

**Bulk Notification Contract** (RO → Notification):
When parent RR completes with duplicates, RO creates a single NotificationRequest:

```yaml
kind: NotificationRequest
spec:
  eventType: "RemediationCompleted"
  priority: "{mapped from parent RR}"
  subject: "Remediation Completed: {workflowId}"
  body: |
    Target: {targetResource}
    Result: ✅ Successful / ❌ Failed
    Duration: {duration}

    Duplicates Suppressed: {duplicateCount}
    ├─ ResourceBusy: {count}
    └─ RecentlyRemediated: {count}
  metadata:
    remediationRequestRef: "{parentRR.name}"
    workflowId: "{we.spec.workflowRef.workflowId}"
    targetResource: "{namespace/kind/name}"
    duplicateCount: "{N}"
    duplicateRefs: ["rr-002", "rr-003", ...]
```

---

### 4. External Service Integration: Notification Service

**Integration Pattern**: HTTP POST for timeout escalation

**Endpoint**: Notification Service HTTP POST `/api/v1/notify/escalation`

**Authoritative Schema**: See [Notification Payload Schema](../../../architecture/specifications/notification-payload-schema.md)

**Escalation Request** (Unified Schema):

Remediation Orchestrator uses the unified `EscalationRequest` schema with all required fields:

```go
// Full schema defined in docs/architecture/specifications/notification-payload-schema.md
// Remediation Orchestrator provides these fields for timeout escalations:
type EscalationRequest struct {
    // Source context (REQUIRED)
    RemediationRequestName      string    `json:"remediationRequestName"`
    RemediationRequestNamespace string    `json:"remediationRequestNamespace"`
    EscalatingController        string    `json:"escalatingController"`      // "remediation-orchestrator"

    // Alert context (REQUIRED - from RemediationRequest CRD)
    AlertFingerprint string `json:"alertFingerprint"`
    AlertName        string `json:"alertName"`
    Severity         string `json:"severity"`
    Environment      string `json:"environment"`
    Priority         string `json:"priority"`
    SignalType       string `json:"signalType"`

    // Resource context (REQUIRED)
    Namespace string             `json:"namespace"`
    Resource  ResourceIdentifier `json:"resource"`

    // Escalation context (REQUIRED)
    EscalationReason  string              `json:"escalationReason"`  // "timeout"
    EscalationPhase   string              `json:"escalationPhase"`   // Which phase timed out
    EscalationTime    time.Time           `json:"escalationTime"`
    EscalationDetails *EscalationDetails `json:"escalationDetails,omitempty"`

    // Temporal context (REQUIRED)
    AlertFiringTime     time.Time `json:"alertFiringTime"`
    AlertReceivedTime   time.Time `json:"alertReceivedTime"`
    RemediationDuration string    `json:"remediationDuration"`

    // Notification routing (REQUIRED)
    Channels []string `json:"channels"`  // ["slack", "email", "pagerduty"]
    Urgency  string   `json:"urgency"`   // "critical", "high", "normal"

    // External links (OPTIONAL)
    AlertmanagerURL string `json:"alertmanagerURL,omitempty"`
    GrafanaURL      string `json:"grafanaURL,omitempty"`

    // AI Analysis and recommended actions would be empty for timeout escalations
}

// EscalationDetails provides structured timeout and failure information
type EscalationDetails struct {
    TimeoutDuration string `json:"timeoutDuration,omitempty"` // e.g., "10m0s"
    Phase           string `json:"phase,omitempty"`           // Phase that timed out
    RetryCount      int    `json:"retryCount,omitempty"`      // Number of retries attempted
    FailureReason   string `json:"failureReason,omitempty"`   // Detailed failure reason
    LastError       string `json:"lastError,omitempty"`       // Last error message
}
```

**Escalation Logic** (Updated for Unified Schema):
```go
// pkg/remediationorchestrator/escalation.go
func (r *RemediationRequestReconciler) sendTimeoutEscalation(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    phase string,
) error {
    // Unmarshal K8s provider data
    var k8sData remediationv1.KubernetesProviderData
    if err := json.Unmarshal(remediation.Spec.ProviderData, &k8sData); err != nil {
        return fmt.Errorf("failed to unmarshal provider data: %w", err)
    }

    payload := notification.EscalationRequest{
        // Source context
        RemediationRequestName:      remediation.Name,
        RemediationRequestNamespace: remediation.Namespace,
        EscalatingController:        "remediation-orchestrator",

        // Alert context (from CRD)
        AlertFingerprint: remediation.Spec.SignalFingerprint,
        AlertName:        remediation.Spec.AlertName,
        Severity:         remediation.Spec.Severity,
        Environment:      remediation.Spec.Environment,
        Priority:         remediation.Spec.Priority,
        SignalType:       remediation.Spec.SignalType,

        // Resource context (from K8s provider data)
        Namespace: k8sData.Namespace,
        Resource:  convertResourceIdentifier(k8sData.Resource),

        // Escalation context
        EscalationReason: "timeout",
        EscalationPhase:  phase, // "remediation-processing", "ai-analysis", etc.
        EscalationTime:   time.Now(),
        EscalationDetails: &notification.EscalationDetails{
            TimeoutDuration: r.getPhaseTimeout(phase).String(),
            Phase:           phase,
            RetryCount:      remediation.Status.RetryCount,
        },

        // Temporal context
        AlertFiringTime:     remediation.Spec.FiringTime.Time,
        AlertReceivedTime:   remediation.Spec.ReceivedTime.Time,
        RemediationDuration: time.Since(remediation.CreationTimestamp.Time).String(),

        // Notification routing
        Channels: determineChannels(remediation.Spec.Priority),
        Urgency:  mapPriorityToUrgency(remediation.Spec.Priority),

        // External links (from K8s provider data)
        AlertmanagerURL: k8sData.AlertmanagerURL,
        GrafanaURL:      k8sData.GrafanaURL,
    }

    return r.notificationClient.SendEscalation(ctx, payload)
}

func determineChannels(priority string) []string {
    switch priority {
    case "P0":
        return []string{"pagerduty", "slack", "email"}
    case "P1":
        return []string{"slack", "email"}
    case "P2":
        return []string{"email"}
    default:
        return []string{"email"}
    }
}

func mapPriorityToUrgency(priority string) string {
    switch priority {
    case "P0":
        return "critical"
    case "P1":
        return "high"
    case "P2":
        return "normal"
    default:
        return "low"
    }
}

func convertResourceIdentifier(k8sResource remediationv1.K8sResourceIdentifier) notification.ResourceIdentifier {
    return notification.ResourceIdentifier{
        Kind:      k8sResource.Kind,
        Name:      k8sResource.Name,
        Namespace: k8sResource.Namespace,
    }
}
```

---

### 5. Database Integration: Data Storage Service

**Integration Pattern**: Audit trail persistence via HTTP POST

**Endpoint**: Data Storage Service HTTP POST `/api/v1/audit/remediation`

**Audit Record**:
```go
type RemediationAudit struct {
    AlertFingerprint    string                 `json:"alertFingerprint"`
    RemediationName     string                 `json:"remediationName"`
    OverallPhase        string                 `json:"overallPhase"`
    StartTime           time.Time              `json:"startTime"`
    CompletionTime      *time.Time             `json:"completionTime,omitempty"`
    ServiceCRDStatuses  []ServiceCRDStatus     `json:"serviceCrdStatuses"`
    TimeoutPhase        *string                `json:"timeoutPhase,omitempty"`
    FailureReason       *string                `json:"failureReason,omitempty"`
}

type ServiceCRDStatus struct {
    ServiceType    string    `json:"serviceType"` // "SignalProcessing", "AIAnalysis", etc.
    CRDName        string    `json:"crdName"`
    Phase          string    `json:"phase"`
    StartTime      time.Time `json:"startTime"`
    CompletionTime *time.Time `json:"completionTime,omitempty"`
}
```

---

### 6. Dependencies Summary

**Upstream Services**:
- **Gateway Service** - Creates RemediationRequest CRD with duplicate detection already performed (BR-WH-008)

**Downstream Services** (CRDs created and watched by RemediationRequest):
- **SignalProcessing Controller** - Enrichment & classification service
- **AIAnalysis Controller** - Kubernaut Agent investigation service
- **WorkflowExecution Controller** - Multi-step orchestration service
- **KubernetesExecution Controller** (DEPRECATED - ADR-025) - Infrastructure operations service

**External Services**:
- **Notification Service** - HTTP POST for timeout escalation
- **Data Storage Service** - HTTP POST for audit trail persistence

**Database**:
- PostgreSQL - `remediation_audit` table for long-term audit storage
- Service CRD status tracking table

---

## Downstream: Notification Service (V1.0 Approval Notifications)

**Business Requirement**: BR-ORCH-001 (RemediationOrchestrator Notification Creation)
**ADR Reference**: ADR-018 (Approval Notification V1.0 Integration)

**Integration Pattern**: CRD-based notification triggering

**Trigger**: AIAnalysis requires approval (phase = "Approving")

**CRD Created**: NotificationRequest

**Notification Details**:
- **Subject**: "🚨 Approval Required: {reason}"
- **Body**: Investigation summary, evidence, recommended actions, alternatives, approval rationale
- **Priority**: High
- **Channels**: Slack (#kubernaut-approvals), Console
- **Metadata**: RemediationRequest name, AIAnalysis name, AIApprovalRequest name, confidence score

**Ownership**: RemediationRequest owns NotificationRequest (OwnerReference for cascade deletion)

**Performance**: <2 seconds from approval phase detection to notification delivery

**Business Value**: Reduces approval miss rate from 40-60% to <5%, preventing $392K per missed approval (large enterprise, $7K/min downtime cost)

---

