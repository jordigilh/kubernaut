## Integration Points

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
                "signal.fingerprint": alertFingerprint,
                "alert.severity":    extractSeverity(payload),
                "alert.environment": extractEnvironment(payload),
            },
        },
        Spec: remediationv1.RemediationRequestSpec{
            AlertFingerprint: alertFingerprint,
            OriginalPayload:  payload, // Store complete alert payload
            Severity:         extractSeverity(payload),
            CreatedAt:        metav1.Now(),
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

#### **2.1. RemediationProcessing CRD Creation**

```go
// In RemediationRequestReconciler
func (r *RemediationRequestReconciler) createRemediationProcessing(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
) error {
    alertProcessing := &processingv1.RemediationProcessing{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-processing", remediation.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: processingv1.RemediationProcessingSpec{
            RemediationRequestRef: processingv1.RemediationRequestReference{
                Name:      remediation.Name,
                Namespace: remediation.Namespace,
            },
            // DATA SNAPSHOT: Copy original alert data
            Alert: processingv1.Alert{
                Fingerprint: remediation.Spec.SignalFingerprint,
                Payload:     remediation.Spec.OriginalPayload,
                Severity:    remediation.Spec.Severity,
            },
        },
    }

    if err := r.Create(ctx, alertProcessing); err != nil {
        return fmt.Errorf("failed to create RemediationProcessing: %w", err)
    }

    // Update RemediationRequest with RemediationProcessing reference
    remediation.Status.RemediationProcessingRef = &remediationv1.RemediationProcessingReference{
        Name:      alertProcessing.Name,
        Namespace: alertProcessing.Namespace,
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
    alertProcessing *processingv1.RemediationProcessing,
) error {
    // When RemediationProcessing completes, create AIAnalysis with enriched data
    if alertProcessing.Status.Phase == "completed" {
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
                // DATA SNAPSHOT: Copy enriched alert context
                AnalysisRequest: aiv1.AnalysisRequest{
                    AlertContext: aiv1.SignalContext{
                        Fingerprint:      alertProcessing.Status.EnrichedSignal.Fingerprint,
                        Severity:         alertProcessing.Status.EnrichedSignal.Severity,
                        Environment:      alertProcessing.Status.EnrichedSignal.Environment,
                        BusinessPriority: alertProcessing.Status.EnrichedSignal.BusinessPriority,

                        // Resource targeting for HolmesGPT toolsets (NOT logs/metrics)
                        Namespace:    alertProcessing.Status.EnrichedSignal.Namespace,
                        ResourceKind: alertProcessing.Status.EnrichedSignal.ResourceKind,
                        ResourceName: alertProcessing.Status.EnrichedSignal.ResourceName,

                        // Kubernetes context (small data ~8KB)
                        KubernetesContext: alertProcessing.Status.EnrichedSignal.KubernetesContext,
                    },
                    AnalysisTypes: []string{"investigation", "root-cause", "recovery-analysis"},
                    InvestigationScope: aiv1.InvestigationScope{
                        TimeWindow: "24h",
                        ResourceScope: []aiv1.ResourceScopeItem{
                            {
                                Kind:      alertProcessing.Status.EnrichedSignal.ResourceKind,
                                Namespace: alertProcessing.Status.EnrichedSignal.Namespace,
                                Name:      alertProcessing.Status.EnrichedSignal.ResourceName,
                            },
                        },
                        CorrelationDepth:          "detailed",
                        IncludeHistoricalPatterns: true,
                    },
                },
                // HolmesGPTConfig is populated by AIAnalysis controller from system defaults
                // Remediation Coordinator only provides business data (targeting info, enriched context)
                // See AIAnalysis service spec for HolmesGPT configuration management
            },
        }

        if err := r.Create(ctx, aiAnalysis); err != nil {
            return fmt.Errorf("failed to create AIAnalysis: %w", err)
        }

        // Update RemediationRequest with AIAnalysis reference
        remediation.Status.AIAnalysisRef = &remediationv1.AIAnalysisReference{
            Name:      aiAnalysis.Name,
            Namespace: aiAnalysis.Namespace,
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
func (r *RemediationRequestReconciler) createWorkflowExecution(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    aiAnalysis *aiv1.AIAnalysis,
) error {
    // When AIAnalysis completes, create WorkflowExecution with recommendations
    if aiAnalysis.Status.Phase == "completed" {
        workflowExecution := &workflowv1.WorkflowExecution{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("%s-workflow", remediation.Name),
                Namespace: remediation.Namespace,
                OwnerReferences: []metav1.OwnerReference{
                    *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
                },
            },
            Spec: workflowv1.WorkflowExecutionSpec{
                RemediationRequestRef: workflowv1.RemediationRequestReference{
                    Name:      remediation.Name,
                    Namespace: remediation.Namespace,
                },
                // DATA SNAPSHOT: Copy AI recommendations
                WorkflowDefinition: workflowv1.WorkflowDefinition{
                    Steps: aiAnalysis.Status.Recommendations[0].WorkflowSteps, // Top recommendation
                },
                ExecutionConfig: workflowv1.ExecutionConfig{
                    DryRun:           false,
                    StepTimeout:      "5m",
                    WorkflowTimeout:  "20m",
                    ConcurrencyLimit: 1,
                },
            },
        }

        if err := r.Create(ctx, workflowExecution); err != nil {
            return fmt.Errorf("failed to create WorkflowExecution: %w", err)
        }

        // Update RemediationRequest with WorkflowExecution reference
        remediation.Status.WorkflowExecutionRef = &remediationv1.WorkflowExecutionReference{
            Name:      workflowExecution.Name,
            Namespace: workflowExecution.Namespace,
        }

        return r.Status().Update(ctx, remediation)
    }

    return nil
}
```

---

#### **2.4. KubernetesExecution CRD Creation**

```go
// In RemediationRequestReconciler
func (r *RemediationRequestReconciler) createKubernetesExecution(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    workflowExecution *workflowv1.WorkflowExecution,
) error {
    // When WorkflowExecution completes, create KubernetesExecution
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

        // Watch RemediationProcessing for completion
        Watches(
            &source.Kind{Type: &processingv1.RemediationProcessing{}},
            handler.EnqueueRequestsFromMapFunc(r.alertProcessingToRemediation),
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

        // Watch KubernetesExecution for completion
        Watches(
            &source.Kind{Type: &executorv1.KubernetesExecution{}},
            handler.EnqueueRequestsFromMapFunc(r.kubernetesExecutionToRemediation),
        ).

        Complete(r)
}

// Map RemediationProcessing to parent RemediationRequest
func (r *RemediationRequestReconciler) alertProcessingToRemediation(obj client.Object) []ctrl.Request {
    ap := obj.(*processingv1.RemediationProcessing)
    return []ctrl.Request{
        {
            NamespacedName: types.NamespacedName{
                Name:      ap.Spec.RemediationRequestRef.Name,
                Namespace: ap.Spec.RemediationRequestRef.Namespace,
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

// Similar mapping functions for WorkflowExecution and KubernetesExecution...
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
    ServiceType    string    `json:"serviceType"` // "RemediationProcessing", "AIAnalysis", etc.
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
- **RemediationProcessing Controller** - Enrichment & classification service
- **AIAnalysis Controller** - HolmesGPT investigation service
- **WorkflowExecution Controller** - Multi-step orchestration service
- **KubernetesExecution Controller** - Infrastructure operations service

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

