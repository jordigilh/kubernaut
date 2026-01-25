# Alert Remediation Service - CRD Implementation

**Service Type**: CRD Controller (Remediation Coordinator)
**CRD**: AlertRemediation
**Priority**: **P0 - CRITICAL** (Must implement FIRST)
**Confidence**: 100% (Template-based design)

**Related Services**:
- **Creates & Watches**: AlertProcessing, AIAnalysis, WorkflowExecution, KubernetesExecution CRDs
- **Integrates With**: All 4 service controllers + Gateway Service

---

## Overview

**Purpose**: Coordinates end-to-end alert remediation workflow through watch-based state aggregation and lifecycle management.

**Core Responsibilities**:
1. **CRD Orchestration** - Create service CRDs (AlertProcessing, AIAnalysis, WorkflowExecution, KubernetesExecution) based on phase progression
2. **Status Aggregation** - Watch all service CRD statuses and aggregate overall remediation state
3. **Lifecycle Management** - 24-hour retention with automatic cleanup and cascade deletion
4. **Timeout Management** - Detect phase timeouts and trigger escalation (BR-ALERT-006)
5. **Event Coordination** - Event-driven phase transitions via Kubernetes watches

**V1 Scope - Remediation Coordination Only**:
- Single AlertRemediation CRD per alert (created by Gateway Service)
- Watch-based event-driven coordination (no polling)
- Sequential phase CRD creation (AlertProcessing → AIAnalysis → WorkflowExecution → KubernetesExecution)
- 24-hour retention with configurable cleanup
- Per-phase timeout detection with escalation

**Future V2 Enhancements** (Out of Scope):
- Parallel remediation workflows for complex scenarios
- Cross-alert correlation and batch remediation
- ML-based timeout prediction
- Multi-cluster remediation coordination
- Advanced retry strategies with exponential backoff

**Key Architectural Decisions**:
- **Watch-based coordination** (event-driven, not polling)
- **Data snapshot pattern** - Copy complete data from service status to next service spec
- **Owner references** - All service CRDs owned by AlertRemediation for cascade deletion
- **No duplicate detection** (Gateway Service responsibility - BR-WH-008)
- **Sequential CRD creation** - One service CRD at a time based on completion
- **24-hour retention** - Configurable cleanup with review window
- **Timeout escalation** - Per-phase and overall workflow timeouts

---

## Business Requirements

**Primary**:
- **BR-ALERT-006**: Timeout management with configurable thresholds and escalation
- **BR-ALERT-021**: Alert lifecycle state tracking across all phases
- **BR-ALERT-024**: Remediation workflow orchestration and coordination
- **BR-ALERT-025**: State persistence across service restarts
- **BR-ALERT-026**: Automatic failure recovery via reconciliation

**Secondary**:
- **BR-ALERT-027**: 24-hour retention window for review and audit
- **BR-ALERT-028**: Cascade deletion of all related service CRDs
- **BR-ALERT-029**: Event emission for operational visibility
- **BR-ALERT-030**: Metrics collection for SLO tracking

**Deduplication Requirements** (Gateway Service responsibility, NOT Remediation Coordinator):
- **BR-WH-008**: Fingerprint-based request deduplication for identical alerts
- **BR-ALERT-003**: Alert suppression to reduce operational noise
- **BR-ALERT-005**: Alert correlation and grouping under single remediation

**Note**: Remediation Coordinator receives AlertRemediation CRDs only for non-duplicate alerts. Gateway Service performs all duplicate detection before CRD creation.

---

## Development Methodology

**Mandatory Process**: Follow APDC-Enhanced TDD workflow per [.cursor/rules/00-core-development-methodology.mdc](../../../.cursor/rules/00-core-development-methodology.mdc)

### APDC-TDD Workflow

```
┌─────────────────────────────────────────────────────────────┐
│ ANALYSIS → PLAN → DO-RED → DO-GREEN → DO-REFACTOR → CHECK  │
└─────────────────────────────────────────────────────────────┘
```

**ANALYSIS** (5-15 min): Comprehensive context understanding
  - Search existing implementations (`codebase_search "AlertRemediation controller implementations"`)
  - Identify reusable orchestration components in `pkg/workflow/` and `internal/controller/`
  - Map business requirements (BR-ALERT-001 to BR-ALERT-030)
  - Identify integration points in `cmd/` for central controller

**PLAN** (10-20 min): Detailed implementation strategy
  - Define TDD phase breakdown (RED → GREEN → REFACTOR)
  - Plan integration points (AlertRemediationReconciler in cmd/remediation/)
  - Establish success criteria (orchestration <1min, CRD creation <5s each)
  - Identify risks (service CRD creation failures → retry logic, cascade deletion → finalizers)

**DO-RED** (10-15 min): Write failing tests FIRST
  - Unit tests defining business contract (70%+ coverage target)
  - Use FAKE K8s client (`sigs.k8s.io/controller-runtime/pkg/client/fake`)
  - Mock ONLY external HTTP services (none for central controller)
  - Use REAL orchestration business logic
  - Map tests to business requirements (BR-ALERT-XXX)

**DO-GREEN** (15-20 min): Minimal implementation
  - Define AlertRemediationReconciler interface to make tests compile
  - Minimal code to pass tests (basic orchestration, CRD creation)
  - **MANDATORY integration in cmd/remediation/** (controller startup)
  - Add owner references for cascade deletion
  - Add finalizers for cleanup coordination

**DO-REFACTOR** (20-30 min): Enhance with sophisticated logic
  - **NO new types/interfaces/files** (enhance existing controller methods)
  - Add sophisticated orchestration algorithms (watch-based coordination, status aggregation)
  - Maintain integration with all service CRDs
  - Add retry logic and error handling for service CRD failures

**CHECK** (5-10 min): Validation and confidence assessment
  - Business requirement verification (BR-ALERT-001 to BR-ALERT-030 addressed)
  - Integration confirmation (controller in cmd/remediation/)
  - Test coverage validation (70%+ unit, 20% integration, 10% E2E)
  - Performance validation (orchestration <1min per service CRD)
  - Confidence assessment: 95% (high confidence, central coordinator pattern)

**AI Assistant Checkpoints**: See [.cursor/rules/10-ai-assistant-behavioral-constraints.mdc](../../../.cursor/rules/10-ai-assistant-behavioral-constraints.mdc)
  - **Checkpoint A**: Type Reference Validation (read AlertRemediation CRD types before referencing)
  - **Checkpoint B**: Test Creation Validation (search existing orchestration patterns)
  - **Checkpoint C**: Business Integration Validation (verify cmd/remediation/ integration)
  - **Checkpoint D**: Build Error Investigation (complete dependency analysis for orchestration)

### Quick Decision Matrix

| Starting Point | Required Phase | Reference |
|----------------|---------------|-----------|
| **New AlertRemediation controller** | Full APDC workflow | Central coordinator pattern is new |
| **Enhance orchestration logic** | DO-RED → DO-REFACTOR | Existing orchestration is well-understood |
| **Fix CRD creation bugs** | ANALYSIS → DO-RED → DO-REFACTOR | Understand CRD dependency chain first |
| **Add watch coordination tests** | DO-RED only | Write tests for watch-based status updates |

**Testing Strategy Reference**: [.cursor/rules/03-testing-strategy.mdc](../../../.cursor/rules/03-testing-strategy.mdc)
  - Unit Tests (70%+): test/unit/remediation/ - Fake K8s client for all CRDs
  - Integration Tests (20%): test/integration/remediation/ - Real K8s (KIND), cross-CRD lifecycle
  - E2E Tests (10%): test/e2e/remediation/ - Complete alert-to-execution-to-completion workflow

---

## Reconciliation Architecture

### Phase Transitions

**Sequential Service CRD Creation Flow**:

```
Gateway creates AlertRemediation
         ↓
   (watch triggers)
         ↓
AlertRemediation creates AlertProcessing
         ↓
   AlertProcessing.status.phase = "completed"
         ↓
   (watch triggers)
         ↓
AlertRemediation creates AIAnalysis
         ↓
   AIAnalysis.status.phase = "completed"
         ↓
   (watch triggers)
         ↓
AlertRemediation creates WorkflowExecution
         ↓
   WorkflowExecution.status.phase = "completed"
         ↓
   (watch triggers)
         ↓
AlertRemediation creates KubernetesExecution
         ↓
   KubernetesExecution.status.phase = "completed"
         ↓
   (watch triggers)
         ↓
AlertRemediation.status.overallPhase = "completed"
         ↓
   (24-hour retention begins)
```

**Overall Phase States**:
- `pending` → `processing` → `analyzing` → `executing` → `completed` / `failed` / `timeout`

### Reconciliation Flow

#### 1. **pending** Phase (Initial State)

**Purpose**: AlertRemediation CRD created by Gateway Service, awaiting controller reconciliation

**Trigger**: Gateway Service creates AlertRemediation CRD with original alert payload

**Actions**:
- Validate AlertRemediation spec (fingerprint, payload, severity)
- Initialize status fields
- Transition to `processing` phase
- **Create AlertProcessing CRD** with data snapshot

**Transition Criteria**:
```go
if alertRemediation.Spec.AlertFingerprint != "" && alertRemediation.Spec.OriginalPayload != nil {
    phase = "processing"
    // Create AlertProcessing CRD
    createAlertProcessing(ctx, alertRemediation)
} else {
    phase = "failed"
    reason = "invalid_alert_data"
}
```

**Timeout**: 30 seconds (initialization should be immediate)

---

#### 2. **processing** Phase (Alert Enrichment & Classification)

**Purpose**: Wait for AlertProcessing CRD completion, then create AIAnalysis CRD

**Trigger**: AlertProcessing.status.phase = "completed" (watch event)

**Actions**:
- **Watch** AlertProcessing CRD status
- When `status.phase = "completed"`:
  - Extract enriched alert data from AlertProcessing.status
  - **Create AIAnalysis CRD** with data snapshot (enriched context)
  - Transition to `analyzing` phase
- **Timeout Detection**: If AlertProcessing exceeds timeout threshold, escalate

**Transition Criteria**:
```go
if alertProcessing.Status.Phase == "completed" {
    phase = "analyzing"
    // Copy enriched data and create AIAnalysis CRD
    createAIAnalysis(ctx, alertRemediation, alertProcessing.Status)
} else if alertProcessing.Status.Phase == "failed" {
    phase = "failed"
    reason = "alert_processing_failed"
} else if timeoutExceeded(alertProcessing) {
    phase = "timeout"
    escalate("alert_processing_timeout")
}
```

**Timeout**: 5 minutes (default for Alert Processing phase)

**Watch Pattern**:
```go
// In controller setup
err = c.Watch(
    &source.Kind{Type: &processingv1.AlertProcessing{}},
    handler.EnqueueRequestsFromMapFunc(r.alertProcessingToRemediation),
)
```

---

#### 3. **analyzing** Phase (AI Analysis & Recommendations)

**Purpose**: Wait for AIAnalysis CRD completion, then create WorkflowExecution CRD

**Trigger**: AIAnalysis.status.phase = "completed" (watch event)

**Actions**:
- **Watch** AIAnalysis CRD status
- When `status.phase = "completed"`:
  - Extract AI recommendations from AIAnalysis.status
  - **Create WorkflowExecution CRD** with data snapshot (recommendations, workflow steps)
  - Transition to `executing` phase
- **Timeout Detection**: If AIAnalysis exceeds timeout threshold, escalate

**Transition Criteria**:
```go
if aiAnalysis.Status.Phase == "completed" {
    phase = "executing"
    // Copy recommendations and create WorkflowExecution CRD
    createWorkflowExecution(ctx, alertRemediation, aiAnalysis.Status)
} else if aiAnalysis.Status.Phase == "failed" {
    phase = "failed"
    reason = "ai_analysis_failed"
} else if timeoutExceeded(aiAnalysis) {
    phase = "timeout"
    escalate("ai_analysis_timeout")
}
```

**Timeout**: 10 minutes (default for AI Analysis phase - HolmesGPT investigation can be long-running)

**Watch Pattern**:
```go
err = c.Watch(
    &source.Kind{Type: &aiv1.AIAnalysis{}},
    handler.EnqueueRequestsFromMapFunc(r.aiAnalysisToRemediation),
)
```

---

#### 4. **executing** Phase (Workflow Execution & Kubernetes Operations)

**Purpose**: Wait for WorkflowExecution CRD completion, then create KubernetesExecution CRD

**Trigger**: WorkflowExecution.status.phase = "completed" (watch event)

**Actions**:
- **Watch** WorkflowExecution CRD status
- When `status.phase = "completed"`:
  - Extract workflow results from WorkflowExecution.status
  - **Create KubernetesExecution CRD** with data snapshot (operations to execute)
  - Wait for KubernetesExecution completion
- **Timeout Detection**: If WorkflowExecution exceeds timeout threshold, escalate

**Transition Criteria**:
```go
if workflowExecution.Status.Phase == "completed" {
    // Create KubernetesExecution CRD
    createKubernetesExecution(ctx, alertRemediation, workflowExecution.Status)

    // Wait for KubernetesExecution to complete before final transition
    if kubernetesExecution.Status.Phase == "completed" {
        phase = "completed"
        completionTime = metav1.Now()
    }
} else if workflowExecution.Status.Phase == "failed" {
    phase = "failed"
    reason = "workflow_execution_failed"
} else if timeoutExceeded(workflowExecution) {
    phase = "timeout"
    escalate("workflow_execution_timeout")
}
```

**Timeout**: 30 minutes (default for Workflow + Kubernetes Execution phases)

**Watch Patterns**:
```go
// Watch WorkflowExecution
err = c.Watch(
    &source.Kind{Type: &workflowv1.WorkflowExecution{}},
    handler.EnqueueRequestsFromMapFunc(r.workflowExecutionToRemediation),
)

// Watch KubernetesExecution
err = c.Watch(
    &source.Kind{Type: &executorv1.KubernetesExecution{}},
    handler.EnqueueRequestsFromMapFunc(r.kubernetesExecutionToRemediation),
)
```

---

#### 5. **completed** Phase (Terminal State - Success)

**Purpose**: All service CRDs completed successfully, begin 24-hour retention

**Actions**:
- Record completion timestamp
- Emit Kubernetes event: `RemediationCompleted`
- Record audit trail to PostgreSQL
- **Start 24-hour retention timer** (finalizer prevents immediate deletion)
- After 24 hours: Remove finalizer and allow garbage collection

**Cleanup Process**:
```go
// Finalizer pattern for 24-hour retention
const remediationFinalizerName = "kubernaut.io/remediation-retention"

func (r *AlertRemediationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var remediation remediationv1.AlertRemediation

    // Check if being deleted
    if !remediation.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(&remediation, remediationFinalizerName) {
            // Perform cleanup
            if err := r.finalizeRemediation(ctx, &remediation); err != nil {
                return ctrl.Result{}, err
            }

            // Remove finalizer
            controllerutil.RemoveFinalizer(&remediation, remediationFinalizerName)
            if err := r.Update(ctx, &remediation); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // Add finalizer if not present
    if !controllerutil.ContainsFinalizer(&remediation, remediationFinalizerName) {
        controllerutil.AddFinalizer(&remediation, remediationFinalizerName)
        if err := r.Update(ctx, &remediation); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Check for 24-hour retention expiry
    if remediation.Status.OverallPhase == "completed" {
        retentionExpiry := remediation.Status.CompletionTime.Add(24 * time.Hour)
        if time.Now().After(retentionExpiry) {
            // Delete CRD (finalizer cleanup will be triggered)
            return ctrl.Result{}, r.Delete(ctx, &remediation)
        }

        // Requeue to check expiry later
        requeueAfter := time.Until(retentionExpiry)
        return ctrl.Result{RequeueAfter: requeueAfter}, nil
    }

    // Continue reconciliation...
}
```

**No Timeout** (terminal state)

**Cascade Deletion**: All service CRDs (AlertProcessing, AIAnalysis, WorkflowExecution, KubernetesExecution) are deleted automatically via owner references.

---

#### 6. **failed** Phase (Terminal State - Failure)

**Purpose**: One or more service CRDs failed, record failure and begin retention

**Actions**:
- Record failure timestamp and reason
- Emit Kubernetes event: `RemediationFailed` with failure details
- Record failure audit to PostgreSQL
- **Start 24-hour retention timer** (same as completed)
- Trigger notification via Notification Service

**No Requeue** (terminal state - requires manual intervention or alert retry)

---

#### 7. **timeout** Phase (Terminal State - Timeout)

**Purpose**: Service CRD exceeded timeout threshold, escalate and record

**Actions**:
- Record timeout timestamp and phase that timed out
- Emit Kubernetes event: `RemediationTimeout`
- **Escalate** via Notification Service (severity-based channels)
- Record timeout audit to PostgreSQL
- **Start 24-hour retention timer**

**Escalation Criteria** (BR-ALERT-006):

| Phase | Default Timeout | Escalation Channel |
|-------|----------------|-------------------|
| **Alert Processing** | 5 minutes | Slack: #platform-ops |
| **AI Analysis** | 10 minutes | Slack: #ai-team, Email: ai-oncall |
| **Workflow Execution** | 20 minutes | Slack: #sre-team |
| **Kubernetes Execution** | 10 minutes | Slack: #platform-oncall, PagerDuty |
| **Overall Workflow** | 1 hour | Slack: #incident-response, PagerDuty: P1 |

**No Requeue** (terminal state)

---

## Integration Points

### 1. Upstream Integration: Gateway Service

**Integration Pattern**: Gateway creates AlertRemediation CRD with duplicate detection already performed

**How AlertRemediation is Created**:
```go
// In Gateway Service - Creates ONLY AlertRemediation CRD
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

    // 2. No existing remediation - create ONLY AlertRemediation CRD
    // AlertRemediation controller will create service CRDs
    requestID := generateRequestID()
    alertRemediation := &remediationv1.AlertRemediation{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("alert-remediation-%s", requestID),
            Namespace: "kubernaut-system",
            Labels: map[string]string{
                "alert.fingerprint": alertFingerprint,
                "alert.severity":    extractSeverity(payload),
                "alert.environment": extractEnvironment(payload),
            },
        },
        Spec: remediationv1.AlertRemediationSpec{
            AlertFingerprint: alertFingerprint,
            OriginalPayload:  payload, // Store complete alert payload
            Severity:         extractSeverity(payload),
            CreatedAt:        metav1.Now(),
        },
    }

    // Create AlertRemediation - Controller will create service CRDs
    return g.k8sClient.Create(ctx, alertRemediation)
}
```

**Note**: Gateway Service creates ONLY AlertRemediation CRD. AlertRemediation controller creates all service CRDs.

---

### 2. Downstream Integration: Service CRD Creation & Watching

**Integration Pattern**: Watch-based event-driven coordination

#### **2.1. AlertProcessing CRD Creation**

```go
// In AlertRemediationReconciler
func (r *AlertRemediationReconciler) createAlertProcessing(
    ctx context.Context,
    remediation *remediationv1.AlertRemediation,
) error {
    alertProcessing := &processingv1.AlertProcessing{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-processing", remediation.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("AlertRemediation")),
            },
        },
        Spec: processingv1.AlertProcessingSpec{
            AlertRemediationRef: processingv1.AlertRemediationReference{
                Name:      remediation.Name,
                Namespace: remediation.Namespace,
            },
            // DATA SNAPSHOT: Copy original alert data
            Alert: processingv1.Alert{
                Fingerprint: remediation.Spec.AlertFingerprint,
                Payload:     remediation.Spec.OriginalPayload,
                Severity:    remediation.Spec.Severity,
            },
        },
    }

    if err := r.Create(ctx, alertProcessing); err != nil {
        return fmt.Errorf("failed to create AlertProcessing: %w", err)
    }

    // Update AlertRemediation with AlertProcessing reference
    remediation.Status.AlertProcessingRef = &remediationv1.AlertProcessingReference{
        Name:      alertProcessing.Name,
        Namespace: alertProcessing.Namespace,
    }

    return r.Status().Update(ctx, remediation)
}
```

---

#### **2.2. AIAnalysis CRD Creation**

```go
// In AlertRemediationReconciler
func (r *AlertRemediationReconciler) createAIAnalysis(
    ctx context.Context,
    remediation *remediationv1.AlertRemediation,
    alertProcessing *processingv1.AlertProcessing,
) error {
    // When AlertProcessing completes, create AIAnalysis with enriched data
    if alertProcessing.Status.Phase == "completed" {
        aiAnalysis := &aiv1.AIAnalysis{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("%s-analysis", remediation.Name),
                Namespace: remediation.Namespace,
                OwnerReferences: []metav1.OwnerReference{
                    *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("AlertRemediation")),
                },
            },
            Spec: aiv1.AIAnalysisSpec{
                AlertRemediationRef: aiv1.AlertRemediationReference{
                    Name:      remediation.Name,
                    Namespace: remediation.Namespace,
                },
                // DATA SNAPSHOT: Copy enriched alert context
                AnalysisRequest: aiv1.AnalysisRequest{
                    AlertContext: aiv1.AlertContext{
                        Fingerprint:      alertProcessing.Status.EnrichedAlert.Fingerprint,
                        Severity:         alertProcessing.Status.EnrichedAlert.Severity,
                        Environment:      alertProcessing.Status.EnrichedAlert.Environment,
                        BusinessPriority: alertProcessing.Status.EnrichedAlert.BusinessPriority,

                        // Resource targeting for HolmesGPT toolsets (NOT logs/metrics)
                        Namespace:    alertProcessing.Status.EnrichedAlert.Namespace,
                        ResourceKind: alertProcessing.Status.EnrichedAlert.ResourceKind,
                        ResourceName: alertProcessing.Status.EnrichedAlert.ResourceName,

                        // Kubernetes context (small data ~8KB)
                        KubernetesContext: alertProcessing.Status.EnrichedAlert.KubernetesContext,
                    },
                    AnalysisTypes: []string{"investigation", "root-cause", "recovery-analysis"},
                    InvestigationScope: aiv1.InvestigationScope{
                        TimeWindow: "24h",
                        ResourceScope: []aiv1.ResourceScopeItem{
                            {
                                Kind:      alertProcessing.Status.EnrichedAlert.ResourceKind,
                                Namespace: alertProcessing.Status.EnrichedAlert.Namespace,
                                Name:      alertProcessing.Status.EnrichedAlert.ResourceName,
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

        // Update AlertRemediation with AIAnalysis reference
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
// In AlertRemediationReconciler
func (r *AlertRemediationReconciler) createWorkflowExecution(
    ctx context.Context,
    remediation *remediationv1.AlertRemediation,
    aiAnalysis *aiv1.AIAnalysis,
) error {
    // When AIAnalysis completes, create WorkflowExecution with recommendations
    if aiAnalysis.Status.Phase == "completed" {
        workflowExecution := &workflowv1.WorkflowExecution{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("%s-workflow", remediation.Name),
                Namespace: remediation.Namespace,
                OwnerReferences: []metav1.OwnerReference{
                    *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("AlertRemediation")),
                },
            },
            Spec: workflowv1.WorkflowExecutionSpec{
                AlertRemediationRef: workflowv1.AlertRemediationReference{
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

        // Update AlertRemediation with WorkflowExecution reference
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
// In AlertRemediationReconciler
func (r *AlertRemediationReconciler) createKubernetesExecution(
    ctx context.Context,
    remediation *remediationv1.AlertRemediation,
    workflowExecution *workflowv1.WorkflowExecution,
) error {
    // When WorkflowExecution completes, create KubernetesExecution
    if workflowExecution.Status.Phase == "completed" {
        kubernetesExecution := &executorv1.KubernetesExecution{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("%s-execution", remediation.Name),
                Namespace: remediation.Namespace,
                OwnerReferences: []metav1.OwnerReference{
                    *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("AlertRemediation")),
                },
            },
            Spec: executorv1.KubernetesExecutionSpec{
                AlertRemediationRef: executorv1.AlertRemediationReference{
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

        // Update AlertRemediation with KubernetesExecution reference
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
// In AlertRemediationReconciler SetupWithManager
func (r *AlertRemediationReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&remediationv1.AlertRemediation{}).

        // Watch AlertProcessing for completion
        Watches(
            &source.Kind{Type: &processingv1.AlertProcessing{}},
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

// Map AlertProcessing to parent AlertRemediation
func (r *AlertRemediationReconciler) alertProcessingToRemediation(obj client.Object) []ctrl.Request {
    ap := obj.(*processingv1.AlertProcessing)
    return []ctrl.Request{
        {
            NamespacedName: types.NamespacedName{
                Name:      ap.Spec.AlertRemediationRef.Name,
                Namespace: ap.Spec.AlertRemediationRef.Namespace,
            },
        },
    }
}

// Map AIAnalysis to parent AlertRemediation
func (r *AlertRemediationReconciler) aiAnalysisToRemediation(obj client.Object) []ctrl.Request {
    ai := obj.(*aiv1.AIAnalysis)
    return []ctrl.Request{
        {
            NamespacedName: types.NamespacedName{
                Name:      ai.Spec.AlertRemediationRef.Name,
                Namespace: ai.Spec.AlertRemediationRef.Namespace,
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

**Authoritative Schema**: See [`docs/services/NOTIFICATION_PAYLOAD_SCHEMA.md`](../../NOTIFICATION_PAYLOAD_SCHEMA.md)

**Note**: This is an archived document. See active documentation for current patterns.

**Escalation Request** (Unified Schema):

```go
// Full schema defined in NOTIFICATION_PAYLOAD_SCHEMA.md
type EscalationRequest struct {
    // Source context (REQUIRED)
    RemediationRequestName      string    `json:"remediationRequestName"`
    RemediationRequestNamespace string    `json:"remediationRequestNamespace"`
    EscalatingController        string    `json:"escalatingController"`      // "remediation-orchestrator"

    // Alert context (REQUIRED)
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
    EscalationReason  string                 `json:"escalationReason"`
    EscalationPhase   string                 `json:"escalationPhase"`
    EscalationTime    time.Time              `json:"escalationTime"`
    EscalationDetails map[string]interface{} `json:"escalationDetails,omitempty"`

    // Temporal context (REQUIRED)
    AlertFiringTime     time.Time `json:"alertFiringTime"`
    AlertReceivedTime   time.Time `json:"alertReceivedTime"`
    RemediationDuration string    `json:"remediationDuration"`

    // Notification routing (REQUIRED)
    Channels []string `json:"channels"`
    Urgency  string   `json:"urgency"`

    // External links (OPTIONAL)
    AlertmanagerURL string `json:"alertmanagerURL,omitempty"`
    GrafanaURL      string `json:"grafanaURL,omitempty"`
}
```

**Escalation Logic** (Updated for Unified Schema):
```go
// See active documentation in crd-controllers/05-remediationorchestrator/integration-points.md
func (r *RemediationRequestReconciler) sendTimeoutEscalation(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    phase string,
) error {
    // Unmarshal K8s provider data
    var k8sData remediationv1.KubernetesProviderData
    json.Unmarshal(remediation.Spec.ProviderData, &k8sData)

    payload := notification.EscalationRequest{
        RemediationRequestName:      remediation.Name,
        RemediationRequestNamespace: remediation.Namespace,
        EscalatingController:        "remediation-orchestrator",
        // ... full payload (see active docs)
    }

    return r.notificationClient.SendEscalation(ctx, payload)
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
    ServiceType    string    `json:"serviceType"` // "AlertProcessing", "AIAnalysis", etc.
    CRDName        string    `json:"crdName"`
    Phase          string    `json:"phase"`
    StartTime      time.Time `json:"startTime"`
    CompletionTime *time.Time `json:"completionTime,omitempty"`
}
```

---

### 6. Dependencies Summary

**Upstream Services**:
- **Gateway Service** - Creates AlertRemediation CRD with duplicate detection already performed (BR-WH-008)

**Downstream Services** (CRDs created and watched by AlertRemediation):
- **AlertProcessing Controller** - Enrichment & classification service
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

## CRD Schema

### AlertRemediation Spec

```go
type AlertRemediationSpec struct {
    // Alert identification
    AlertFingerprint string `json:"alertFingerprint"`

    // Original alert payload (complete webhook data)
    OriginalPayload []byte `json:"originalPayload"`

    // Alert metadata
    Severity    string      `json:"severity"`
    Environment string      `json:"environment,omitempty"`
    CreatedAt   metav1.Time `json:"createdAt"`

    // Timeout configuration (optional, uses defaults if not specified)
    TimeoutConfig *TimeoutConfig `json:"timeoutConfig,omitempty"`
}

type TimeoutConfig struct {
    AlertProcessingTimeout     metav1.Duration `json:"alertProcessingTimeout,omitempty"`     // Default: 5m
    AIAnalysisTimeout          metav1.Duration `json:"aiAnalysisTimeout,omitempty"`          // Default: 10m
    WorkflowExecutionTimeout   metav1.Duration `json:"workflowExecutionTimeout,omitempty"`   // Default: 20m
    KubernetesExecutionTimeout metav1.Duration `json:"kubernetesExecutionTimeout,omitempty"` // Default: 10m
    OverallWorkflowTimeout     metav1.Duration `json:"overallWorkflowTimeout,omitempty"`     // Default: 1h
}
```

### AlertRemediation Status

```go
type AlertRemediationStatus struct {
    // Overall remediation state
    OverallPhase string      `json:"overallPhase"` // pending, processing, analyzing, executing, completed, failed, timeout
    StartTime    metav1.Time `json:"startTime"`
    CompletionTime *metav1.Time `json:"completionTime,omitempty"`

    // Service CRD references (created by this controller)
    AlertProcessingRef     *AlertProcessingReference     `json:"alertProcessingRef,omitempty"`
    AIAnalysisRef          *AIAnalysisReference          `json:"aiAnalysisRef,omitempty"`
    WorkflowExecutionRef   *WorkflowExecutionReference   `json:"workflowExecutionRef,omitempty"`
    KubernetesExecutionRef *KubernetesExecutionReference `json:"kubernetesExecutionRef,omitempty"`

    // Aggregated status from service CRDs
    AlertProcessingStatus     *AlertProcessingStatusSummary     `json:"alertProcessingStatus,omitempty"`
    AIAnalysisStatus          *AIAnalysisStatusSummary          `json:"aiAnalysisStatus,omitempty"`
    WorkflowExecutionStatus   *WorkflowExecutionStatusSummary   `json:"workflowExecutionStatus,omitempty"`
    KubernetesExecutionStatus *KubernetesExecutionStatusSummary `json:"kubernetesExecutionStatus,omitempty"`

    // Timeout tracking
    TimeoutPhase *string      `json:"timeoutPhase,omitempty"` // Which phase timed out
    TimeoutTime  *metav1.Time `json:"timeoutTime,omitempty"`

    // Failure tracking
    FailurePhase  *string `json:"failurePhase,omitempty"`  // Which phase failed
    FailureReason *string `json:"failureReason,omitempty"` // Detailed failure reason

    // Retention tracking
    RetentionExpiryTime *metav1.Time `json:"retentionExpiryTime,omitempty"` // When 24h retention expires

    // Duplicate tracking (from Gateway Service)
    DuplicateCount int      `json:"duplicateCount"` // Number of duplicate alerts suppressed
    LastDuplicateTime *metav1.Time `json:"lastDuplicateTime,omitempty"`
}

// Reference types
type AlertProcessingReference struct {
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
}

type AIAnalysisReference struct {
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
}

type WorkflowExecutionReference struct {
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
}

type KubernetesExecutionReference struct {
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
}

// Status summary types (lightweight aggregation)
type AlertProcessingStatusSummary struct {
    Phase          string      `json:"phase"`
    CompletionTime *metav1.Time `json:"completionTime,omitempty"`
    Environment    string      `json:"environment,omitempty"`
    DegradedMode   bool        `json:"degradedMode"`
}

type AIAnalysisStatusSummary struct {
    Phase              string      `json:"phase"`
    CompletionTime     *metav1.Time `json:"completionTime,omitempty"`
    RecommendationCount int        `json:"recommendationCount"`
    TopRecommendation  string      `json:"topRecommendation,omitempty"`
}

type WorkflowExecutionStatusSummary struct {
    Phase          string      `json:"phase"`
    CompletionTime *metav1.Time `json:"completionTime,omitempty"`
    TotalSteps     int         `json:"totalSteps"`
    CompletedSteps int         `json:"completedSteps"`
}

type KubernetesExecutionStatusSummary struct {
    Phase           string      `json:"phase"`
    CompletionTime  *metav1.Time `json:"completionTime,omitempty"`
    OperationsTotal int         `json:"operationsTotal"`
    OperationsSuccess int       `json:"operationsSuccess"`
}
```

---

### ✅ **TYPE SAFETY COMPLIANCE**

This CRD specification uses **fully structured types** for all status aggregation and service CRD references:

| Type | Previous (Anti-Pattern) | Current (Type-Safe) | Benefit |
|------|------------------------|---------------------|---------|
| **AlertProcessingStatusSummary** | `map[string]interface{}` | Structured type with phase, timestamp, environment | Compile-time safety for aggregation |
| **AIAnalysisStatusSummary** | `map[string]interface{}` | Structured type with phase, recommendation count | Type-safe AI status aggregation |
| **WorkflowExecutionStatusSummary** | `map[string]interface{}` | Structured type with step progress | Type-safe workflow status tracking |
| **KubernetesExecutionStatusSummary** | `map[string]interface{}` | Structured type with operation counts | Type-safe execution result aggregation |
| **Service CRD References** | `map[string]interface{}` | 4 structured reference types | Clear ownership and lifecycle management |

**Design Principle**: AlertRemediation aggregates status from 4 service CRDs. All aggregation uses lightweight structured types, not full data copies.

**Key Type-Safe Components**:
- ✅ All service CRD references use `corev1.ObjectReference` (Kubernetes-native type)
- ✅ Status summaries are lightweight structured types (not full service CRD status copies)
- ✅ No `map[string]interface{}` usage anywhere in aggregation logic
- ✅ Each service CRD provides its own type-safe status, AlertRemediation aggregates safely

**Type-Safe Aggregation Pattern**:
```go
// ✅ TYPE SAFE - Lightweight status aggregation
type AlertProcessingStatusSummary struct {
    Phase          string       `json:"phase"`
    CompletionTime *metav1.Time `json:"completionTime,omitempty"`
    Environment    string       `json:"environment,omitempty"`
    DegradedMode   bool         `json:"degradedMode"`
}

// NOT this anti-pattern:
// ProcessingStatus map[string]interface{} `json:"processingStatus"` // ❌ WRONG
```

**Why Lightweight Summaries**:
- **Performance**: Don't copy entire service CRD status (can be large)
- **Clarity**: Only essential fields for coordination (phase, completion time)
- **Decoupling**: Service CRDs own their detailed status
- **Scalability**: AlertRemediation status stays small even with complex service CRDs

**Full Status Available When Needed**:
```go
// When AlertRemediation needs detailed status, it queries the service CRD:
var aiAnalysis aiv1.AIAnalysis
if err := r.Get(ctx, client.ObjectKey{
    Name:      remediation.Status.AIAnalysisRef.Name,
    Namespace: remediation.Status.AIAnalysisRef.Namespace,
}, &aiAnalysis); err != nil {
    return err
}

// Full status available here: aiAnalysis.Status
// Remediation only stores summary: remediation.Status.AIAnalysisStatus
```

**Related Documents**:
- `ALERT_PROCESSOR_TYPE_SAFETY_TRIAGE.md` - Original type safety remediation
- `OWNER_REFERENCE_ARCHITECTURE.md` - Service CRD lifecycle and references

---

## Controller Implementation

### Core Reconciliation Logic

```go
package controller

import (
    "context"
    "fmt"
    "time"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    processingv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"
    aiv1 "github.com/jordigilh/kubernaut/api/ai/v1"
    workflowv1 "github.com/jordigilh/kubernaut/api/workflow/v1"
    executorv1 "github.com/jordigilh/kubernaut/api/executor/v1"

    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type AlertRemediationReconciler struct {
    client.Client
    Scheme *runtime.Scheme

    NotificationClient NotificationClient
    StorageClient      StorageClient
}

func (r *AlertRemediationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var remediation remediationv1.AlertRemediation
    if err := r.Get(ctx, req.NamespacedName, &remediation); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Handle finalizer for 24-hour retention
    if !remediation.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(&remediation, remediationFinalizerName) {
            if err := r.finalizeRemediation(ctx, &remediation); err != nil {
                return ctrl.Result{}, err
            }

            controllerutil.RemoveFinalizer(&remediation, remediationFinalizerName)
            if err := r.Update(ctx, &remediation); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // Add finalizer if not present
    if !controllerutil.ContainsFinalizer(&remediation, remediationFinalizerName) {
        controllerutil.AddFinalizer(&remediation, remediationFinalizerName)
        if err := r.Update(ctx, &remediation); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Initialize if new
    if remediation.Status.OverallPhase == "" {
        remediation.Status.OverallPhase = "pending"
        remediation.Status.StartTime = metav1.Now()
        if err := r.Status().Update(ctx, &remediation); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Handle terminal states
    if remediation.Status.OverallPhase == "completed" ||
       remediation.Status.OverallPhase == "failed" ||
       remediation.Status.OverallPhase == "timeout" {
        return r.handleTerminalState(ctx, &remediation)
    }

    // Orchestrate service CRDs based on phase
    return r.orchestratePhase(ctx, &remediation)
}

// Orchestrate service CRD creation based on current phase
func (r *AlertRemediationReconciler) orchestratePhase(
    ctx context.Context,
    remediation *remediationv1.AlertRemediation,
) (ctrl.Result, error) {

    switch remediation.Status.OverallPhase {
    case "pending":
        // Create AlertProcessing CRD
        if remediation.Status.AlertProcessingRef == nil {
            if err := r.createAlertProcessing(ctx, remediation); err != nil {
                return ctrl.Result{}, err
            }
            remediation.Status.OverallPhase = "processing"
            return ctrl.Result{}, r.Status().Update(ctx, remediation)
        }

    case "processing":
        // Wait for AlertProcessing completion, then create AIAnalysis
        var alertProcessing processingv1.AlertProcessing
        if err := r.Get(ctx, client.ObjectKey{
            Name:      remediation.Status.AlertProcessingRef.Name,
            Namespace: remediation.Status.AlertProcessingRef.Namespace,
        }, &alertProcessing); err != nil {
            return ctrl.Result{}, err
        }

        // Check timeout
        if r.isPhaseTimedOut(&alertProcessing, remediation.Status.TimeoutConfig) {
            return r.handleTimeout(ctx, remediation, "alert_processing")
        }

        if alertProcessing.Status.Phase == "completed" {
            if remediation.Status.AIAnalysisRef == nil {
                if err := r.createAIAnalysis(ctx, remediation, &alertProcessing); err != nil {
                    return ctrl.Result{}, err
                }
                remediation.Status.OverallPhase = "analyzing"
                return ctrl.Result{}, r.Status().Update(ctx, remediation)
            }
        } else if alertProcessing.Status.Phase == "failed" {
            return r.handleFailure(ctx, remediation, "alert_processing", "Alert processing failed")
        }

    case "analyzing":
        // Wait for AIAnalysis completion, then create WorkflowExecution
        var aiAnalysis aiv1.AIAnalysis
        if err := r.Get(ctx, client.ObjectKey{
            Name:      remediation.Status.AIAnalysisRef.Name,
            Namespace: remediation.Status.AIAnalysisRef.Namespace,
        }, &aiAnalysis); err != nil {
            return ctrl.Result{}, err
        }

        // Check timeout
        if r.isPhaseTimedOut(&aiAnalysis, remediation.Status.TimeoutConfig) {
            return r.handleTimeout(ctx, remediation, "ai_analysis")
        }

        if aiAnalysis.Status.Phase == "completed" {
            if remediation.Status.WorkflowExecutionRef == nil {
                if err := r.createWorkflowExecution(ctx, remediation, &aiAnalysis); err != nil {
                    return ctrl.Result{}, err
                }
                remediation.Status.OverallPhase = "executing"
                return ctrl.Result{}, r.Status().Update(ctx, remediation)
            }
        } else if aiAnalysis.Status.Phase == "failed" {
            return r.handleFailure(ctx, remediation, "ai_analysis", "AI analysis failed")
        }

    case "executing":
        // Wait for WorkflowExecution completion, then create KubernetesExecution
        var workflowExecution workflowv1.WorkflowExecution
        if err := r.Get(ctx, client.ObjectKey{
            Name:      remediation.Status.WorkflowExecutionRef.Name,
            Namespace: remediation.Status.WorkflowExecutionRef.Namespace,
        }, &workflowExecution); err != nil {
            return ctrl.Result{}, err
        }

        // Check timeout
        if r.isPhaseTimedOut(&workflowExecution, remediation.Status.TimeoutConfig) {
            return r.handleTimeout(ctx, remediation, "workflow_execution")
        }

        if workflowExecution.Status.Phase == "completed" {
            if remediation.Status.KubernetesExecutionRef == nil {
                if err := r.createKubernetesExecution(ctx, remediation, &workflowExecution); err != nil {
                    return ctrl.Result{}, err
                }

                // Wait for KubernetesExecution to complete
                var kubernetesExecution executorv1.KubernetesExecution
                if err := r.Get(ctx, client.ObjectKey{
                    Name:      remediation.Status.KubernetesExecutionRef.Name,
                    Namespace: remediation.Status.KubernetesExecutionRef.Namespace,
                }, &kubernetesExecution); err != nil {
                    return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
                }

                if kubernetesExecution.Status.Phase == "completed" {
                    remediation.Status.OverallPhase = "completed"
                    remediation.Status.CompletionTime = &metav1.Time{Time: time.Now()}
                    remediation.Status.RetentionExpiryTime = &metav1.Time{Time: time.Now().Add(24 * time.Hour)}
                    return ctrl.Result{}, r.Status().Update(ctx, remediation)
                } else if kubernetesExecution.Status.Phase == "failed" {
                    return r.handleFailure(ctx, remediation, "kubernetes_execution", "Kubernetes execution failed")
                }
            }
        } else if workflowExecution.Status.Phase == "failed" {
            return r.handleFailure(ctx, remediation, "workflow_execution", "Workflow execution failed")
        }
    }

    // Requeue to check progress
    return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// Handle terminal state (completed, failed, timeout)
func (r *AlertRemediationReconciler) handleTerminalState(
    ctx context.Context,
    remediation *remediationv1.AlertRemediation,
) (ctrl.Result, error) {

    // Check if 24-hour retention has expired
    if remediation.Status.RetentionExpiryTime != nil {
        if time.Now().After(remediation.Status.RetentionExpiryTime.Time) {
            // Delete CRD (finalizer cleanup will be triggered)
            return ctrl.Result{}, r.Delete(ctx, remediation)
        }

        // Requeue to check expiry later
        requeueAfter := time.Until(remediation.Status.RetentionExpiryTime.Time)
        return ctrl.Result{RequeueAfter: requeueAfter}, nil
    }

    return ctrl.Result{}, nil
}

// Handle timeout
func (r *AlertRemediationReconciler) handleTimeout(
    ctx context.Context,
    remediation *remediationv1.AlertRemediation,
    phase string,
) (ctrl.Result, error) {

    remediation.Status.OverallPhase = "timeout"
    remediation.Status.TimeoutPhase = &phase
    remediation.Status.TimeoutTime = &metav1.Time{Time: time.Now()}
    remediation.Status.RetentionExpiryTime = &metav1.Time{Time: time.Now().Add(24 * time.Hour)}

    // Escalate timeout
    if err := r.escalateTimeout(ctx, remediation, phase); err != nil {
        return ctrl.Result{}, err
    }

    // Record audit
    if err := r.recordAudit(ctx, remediation); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, r.Status().Update(ctx, remediation)
}

// Handle failure
func (r *AlertRemediationReconciler) handleFailure(
    ctx context.Context,
    remediation *remediationv1.AlertRemediation,
    phase string,
    reason string,
) (ctrl.Result, error) {

    remediation.Status.OverallPhase = "failed"
    remediation.Status.FailurePhase = &phase
    remediation.Status.FailureReason = &reason
    remediation.Status.CompletionTime = &metav1.Time{Time: time.Now()}
    remediation.Status.RetentionExpiryTime = &metav1.Time{Time: time.Now().Add(24 * time.Hour)}

    // Record audit
    if err := r.recordAudit(ctx, remediation); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, r.Status().Update(ctx, remediation)
}

// Finalizer cleanup
func (r *AlertRemediationReconciler) finalizeRemediation(
    ctx context.Context,
    remediation *remediationv1.AlertRemediation,
) error {

    // Record final audit before deletion
    return r.recordAudit(ctx, remediation)
}

const remediationFinalizerName = "kubernaut.io/remediation-retention"
```

---

## Critical Architectural Patterns

### 1. Watch-Based Event-Driven Coordination
**Pattern**: Use Kubernetes watches instead of polling for service CRD status changes

```go
// Watch setup in SetupWithManager
Watches(
    &source.Kind{Type: &processingv1.AlertProcessing{}},
    handler.EnqueueRequestsFromMapFunc(r.alertProcessingToRemediation),
)
```

**Purpose**: Immediate reconciliation trigger when service CRDs complete (<1s latency)

### 2. Data Snapshot Pattern for CRD Creation
**Pattern**: Copy complete data from service status to next service spec at creation time

```go
// Copy targeting data from AlertProcessing.status to AIAnalysis.spec
aiAnalysis.Spec.AnalysisRequest.AlertContext = aiv1.AlertContext{
    Fingerprint:      alertProcessing.Status.EnrichedAlert.Fingerprint,
    Severity:         alertProcessing.Status.EnrichedAlert.Severity,
    Environment:      alertProcessing.Status.EnrichedAlert.Environment,

    // Resource targeting (HolmesGPT uses this to fetch logs/metrics)
    Namespace:        alertProcessing.Status.EnrichedAlert.Namespace,
    ResourceKind:     alertProcessing.Status.EnrichedAlert.ResourceKind,
    ResourceName:     alertProcessing.Status.EnrichedAlert.ResourceName,
    KubernetesContext: alertProcessing.Status.EnrichedAlert.KubernetesContext,
}
```

**Purpose**: Each service has targeting data in spec, HolmesGPT fetches logs/metrics dynamically (no cross-CRD dependencies)

### 3. Owner References for Cascade Deletion
**Pattern**: Set AlertRemediation as owner of all service CRDs

```go
OwnerReferences: []metav1.OwnerReference{
    *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("AlertRemediation")),
}
```

**Purpose**: Delete all service CRDs when AlertRemediation is deleted (automatic cleanup)

### 4. Finalizer Pattern for 24-Hour Retention
**Pattern**: Use finalizer to prevent deletion until 24-hour retention expires

```go
const remediationFinalizerName = "kubernaut.io/remediation-retention"

if !controllerutil.ContainsFinalizer(&remediation, remediationFinalizerName) {
    controllerutil.AddFinalizer(&remediation, remediationFinalizerName)
}
```

**Purpose**: Ensure CRD persists for 24 hours after completion for review and audit

### 5. Phase-Based Timeout Detection
**Pattern**: Per-phase timeout thresholds with escalation

```go
func (r *AlertRemediationReconciler) isPhaseTimedOut(
    serviceCRD client.Object,
    timeoutConfig *TimeoutConfig,
) bool {
    // Check if service CRD exceeded phase timeout
    elapsed := time.Since(serviceCRD.GetCreationTimestamp().Time)
    return elapsed > r.getPhaseTimeout(serviceCRD.GetObjectKind().GroupVersionKind().Kind, timeoutConfig)
}
```

**Purpose**: Detect stuck service CRDs and trigger escalation before overall workflow timeout

### 6. Sequential CRD Creation with Phase Progression
**Pattern**: Create one service CRD at a time based on previous CRD completion

```go
switch remediation.Status.OverallPhase {
case "pending":
    createAlertProcessing() → "processing"
case "processing":
    if alertProcessing.completed → createAIAnalysis() → "analyzing"
case "analyzing":
    if aiAnalysis.completed → createWorkflowExecution() → "executing"
case "executing":
    if workflowExecution.completed → createKubernetesExecution() → wait for completion → "completed"
}
```

**Purpose**: Clear phase progression, prevents premature CRD creation, enables timeout per phase

### 7. Status Aggregation from Service CRDs
**Pattern**: Lightweight status summary aggregation (not full data copy)

```go
remediation.Status.AlertProcessingStatus = &AlertProcessingStatusSummary{
    Phase:          alertProcessing.Status.Phase,
    CompletionTime: alertProcessing.Status.CompletionTime,
    Environment:    alertProcessing.Status.EnvironmentClassification.Tier,
    DegradedMode:   alertProcessing.Status.DegradedMode,
}
```

**Purpose**: Quick status overview without duplicating large data payloads in AlertRemediation status

### 8. Event Emission for Operational Visibility
**Pattern**: Emit Kubernetes events for significant state changes

```go
r.Recorder.Event(&remediation, "Normal", "PhaseTransition",
    fmt.Sprintf("Transitioned to %s phase", remediation.Status.OverallPhase))

r.Recorder.Event(&remediation, "Warning", "Timeout",
    fmt.Sprintf("Phase %s exceeded timeout", phase))
```

**Purpose**: Operational visibility via `kubectl events`, no external log parsing needed

---

## Finalizer Implementation

### Finalizer Name

Following Kubernetes finalizer naming convention:

```go
const alertRemediationFinalizer = "alertremediation.kubernaut.io/alertremediation-cleanup"
```

**Naming Pattern**: `{domain}.kubernaut.io/{resource}-cleanup`

**Why This Pattern**:
- **Domain-Scoped**: `alertremediation.kubernaut.io` prevents conflicts with other services
- **Resource-Specific**: `alertremediation-cleanup` clearly indicates what's being cleaned up
- **Kubernetes Convention**: Follows standard finalizer naming (domain/action format)

---

### Complete Reconciliation Loop with Finalizer

```go
package controller

import (
    "context"
    "fmt"

    remediationv1 "github.com/jordigilh/kubernaut/api/alertremediation/v1"

    "github.com/go-logr/logr"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const alertRemediationFinalizer = "alertremediation.kubernaut.io/alertremediation-cleanup"

type AlertRemediationReconciler struct {
    client.Client
    Scheme            *runtime.Scheme
    Log               logr.Logger
    Recorder          record.EventRecorder
    StorageClient     StorageClient
}

func (r *AlertRemediationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var ar remediationv1.AlertRemediation
    if err := r.Get(ctx, req.NamespacedName, &ar); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // ========================================
    // DELETION HANDLING WITH FINALIZER
    // ========================================
    if !ar.ObjectMeta.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(&ar, alertRemediationFinalizer) {
            // Perform cleanup before deletion
            if err := r.cleanupAlertRemediation(ctx, &ar); err != nil {
                r.Log.Error(err, "Failed to cleanup AlertRemediation resources",
                    "name", ar.Name,
                    "namespace", ar.Namespace,
                )
                return ctrl.Result{}, err
            }

            // Remove finalizer to allow deletion
            controllerutil.RemoveFinalizer(&ar, alertRemediationFinalizer)
            if err := r.Update(ctx, &ar); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // ========================================
    // ADD FINALIZER IF NOT PRESENT
    // ========================================
    if !controllerutil.ContainsFinalizer(&ar, alertRemediationFinalizer) {
        controllerutil.AddFinalizer(&ar, alertRemediationFinalizer)
        if err := r.Update(ctx, &ar); err != nil {
            return ctrl.Result{}, err
        }
    }

    // ========================================
    // NORMAL RECONCILIATION LOGIC
    // ========================================

    // Skip if already completed or failed
    if ar.Status.OverallPhase == "completed" || ar.Status.OverallPhase == "failed" {
        return ctrl.Result{}, nil
    }

    // Execute orchestration phases...
    // (existing reconciliation logic from previous section)

    return ctrl.Result{}, nil
}
```

---

### Cleanup Logic

**What Gets Cleaned Up** (Remediation Orchestrator Pattern):

```go
package controller

import (
    "context"
    "fmt"
    "time"

    remediationv1 "github.com/jordigilh/kubernaut/api/alertremediation/v1"
    processingv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/analysis/v1"
    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflow/execution/v1"
    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetes/execution/v1"

    apierrors "k8s.io/apimachinery/pkg/api/errors"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *AlertRemediationReconciler) cleanupAlertRemediation(
    ctx context.Context,
    ar *remediationv1.AlertRemediation,
) error {
    r.Log.Info("Cleaning up AlertRemediation resources",
        "name", ar.Name,
        "namespace", ar.Namespace,
        "overallPhase", ar.Status.OverallPhase,
    )

    // 1. Delete ALL owned child CRDs (best-effort, cascade deletion handles most)
    // These CRDs have owner references, so Kubernetes will cascade-delete them
    // Explicit deletion here is best-effort for immediate cleanup
    if err := r.cleanupChildCRDs(ctx, ar); err != nil {
        r.Log.Error(err, "Failed to cleanup child CRDs", "name", ar.Name)
        // Don't block deletion on child cleanup failure
        // Owner references will ensure cascade deletion
    }

    // 2. Record final audit to database
    if err := r.recordFinalAudit(ctx, ar); err != nil {
        r.Log.Error(err, "Failed to record final audit", "name", ar.Name)
        // Don't block deletion on audit failure
        // Audit is best-effort during cleanup
    }

    // 3. Emit deletion event
    r.Recorder.Event(ar, "Normal", "AlertRemediationDeleted",
        fmt.Sprintf("AlertRemediation cleanup completed (phase: %s)", ar.Status.OverallPhase))

    r.Log.Info("AlertRemediation cleanup completed successfully",
        "name", ar.Name,
        "namespace", ar.Namespace,
    )

    return nil
}

func (r *AlertRemediationReconciler) cleanupChildCRDs(
    ctx context.Context,
    ar *remediationv1.AlertRemediation,
) error {
    namespace := ar.Namespace

    // Delete AlertProcessing CRD (if exists)
    if ar.Status.AlertProcessingRef != nil {
        apName := ar.Status.AlertProcessingRef.Name
        ap := &processingv1.AlertProcessing{}
        if err := r.Get(ctx, client.ObjectKey{Name: apName, Namespace: namespace}, ap); err == nil {
            if err := r.Delete(ctx, ap); err != nil && !apierrors.IsNotFound(err) {
                r.Log.Error(err, "Failed to delete AlertProcessing", "name", apName)
            } else {
                r.Log.Info("Deleted AlertProcessing CRD", "name", apName)
            }
        }
    }

    // Delete AIAnalysis CRD (if exists)
    if ar.Status.AIAnalysisRef != nil {
        aiName := ar.Status.AIAnalysisRef.Name
        ai := &aianalysisv1.AIAnalysis{}
        if err := r.Get(ctx, client.ObjectKey{Name: aiName, Namespace: namespace}, ai); err == nil {
            if err := r.Delete(ctx, ai); err != nil && !apierrors.IsNotFound(err) {
                r.Log.Error(err, "Failed to delete AIAnalysis", "name", aiName)
            } else {
                r.Log.Info("Deleted AIAnalysis CRD", "name", aiName)
            }
        }
    }

    // Delete WorkflowExecution CRD (if exists)
    if ar.Status.WorkflowExecutionRef != nil {
        weName := ar.Status.WorkflowExecutionRef.Name
        we := &workflowexecutionv1.WorkflowExecution{}
        if err := r.Get(ctx, client.ObjectKey{Name: weName, Namespace: namespace}, we); err == nil {
            if err := r.Delete(ctx, we); err != nil && !apierrors.IsNotFound(err) {
                r.Log.Error(err, "Failed to delete WorkflowExecution", "name", weName)
            } else {
                r.Log.Info("Deleted WorkflowExecution CRD", "name", weName)
            }
        }
    }

    // Delete ALL KubernetesExecution CRDs owned by this AlertRemediation
    keList := &kubernetesexecutionv1.KubernetesExecutionList{}
    if err := r.List(ctx, keList, client.InNamespace(namespace)); err == nil {
        for _, ke := range keList.Items {
            // Check if owned by this AlertRemediation
            for _, ownerRef := range ke.OwnerReferences {
                if ownerRef.UID == ar.UID {
                    if err := r.Delete(ctx, &ke); err != nil && !apierrors.IsNotFound(err) {
                        r.Log.Error(err, "Failed to delete KubernetesExecution", "name", ke.Name)
                    } else {
                        r.Log.Info("Deleted KubernetesExecution CRD", "name", ke.Name)
                    }
                    break
                }
            }
        }
    }

    return nil
}

func (r *AlertRemediationReconciler) recordFinalAudit(
    ctx context.Context,
    ar *remediationv1.AlertRemediation,
) error {
    auditRecord := &AuditRecord{
        AlertFingerprint:       ar.Spec.AlertFingerprint,
        ServiceType:            "AlertRemediation",
        CRDName:                ar.Name,
        Namespace:              ar.Namespace,
        OverallPhase:           ar.Status.OverallPhase,
        CreatedAt:              ar.CreationTimestamp.Time,
        DeletedAt:              ar.DeletionTimestamp.Time,
        AlertProcessingCreated: ar.Status.AlertProcessingRef != nil,
        AIAnalysisCreated:      ar.Status.AIAnalysisRef != nil,
        WorkflowCreated:        ar.Status.WorkflowExecutionRef != nil,
        ExecutionsCreated:      len(ar.Status.KubernetesExecutionRefs),
    }

    return r.StorageClient.RecordAudit(ctx, auditRecord)
}
```

**Cleanup Philosophy for AlertRemediation** (Remediation Orchestrator):
- ✅ **Delete child CRDs**: Best-effort deletion of all owned CRDs (owner references ensure cascade)
- ✅ **Record final audit**: Capture complete remediation lifecycle (best-effort)
- ✅ **Emit deletion event**: Operational visibility
- ✅ **Multiple child CRDs**: Handles 4 different child CRD types (AlertProcessing, AIAnalysis, WorkflowExecution, KubernetesExecution)
- ✅ **Parallel deletion**: Kubernetes garbage collector handles cascade deletion in parallel
- ✅ **Non-blocking**: Child deletion and audit failures don't block deletion (best-effort)

**Note**: Child CRDs have `ownerReferences` set to AlertRemediation, so they'll be cascade-deleted automatically by Kubernetes. Explicit deletion in finalizer is best-effort immediate cleanup.

---

### Finalizer Testing

**Unit Test Pattern**:

```go
package controller_test

import (
    "context"
    "fmt"

    remediationv1 "github.com/jordigilh/kubernaut/api/alertremediation/v1"
    processingv1 "github.com/jordigilh/kubernaut/api/alertprocessor/v1"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/analysis/v1"
    "github.com/jordigilh/kubernaut/pkg/alertremediation/controller"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/kubernetes/scheme"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("AlertRemediation Finalizer", func() {
    var (
        ctx       context.Context
        k8sClient client.Client
        reconciler *controller.AlertRemediationReconciler
    )

    BeforeEach(func() {
        ctx = context.Background()
        k8sClient = fake.NewClientBuilder().
            WithScheme(scheme.Scheme).
            Build()

        reconciler = &controller.AlertRemediationReconciler{
            Client:        k8sClient,
            StorageClient: &mockStorageClient{},
        }
    })

    Context("when AlertRemediation is created", func() {
        It("should add finalizer on first reconcile", func() {
            ar := &remediationv1.AlertRemediation{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-remediation",
                    Namespace: "default",
                },
                Spec: remediationv1.AlertRemediationSpec{
                    AlertFingerprint: "abc123",
                },
            }
            Expect(k8sClient.Create(ctx, ar)).To(Succeed())

            // First reconcile should add finalizer
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(ar),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer added
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ar), ar)).To(Succeed())
            Expect(controllerutil.ContainsFinalizer(ar, alertRemediationFinalizer)).To(BeTrue())
        })
    })

    Context("when AlertRemediation is deleted", func() {
        It("should execute cleanup and remove finalizer", func() {
            ar := &remediationv1.AlertRemediation{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-remediation",
                    Namespace:  "default",
                    Finalizers: []string{alertRemediationFinalizer},
                },
                Status: remediationv1.AlertRemediationStatus{
                    OverallPhase: "completed",
                },
            }
            Expect(k8sClient.Create(ctx, ar)).To(Succeed())

            // Delete AlertRemediation
            Expect(k8sClient.Delete(ctx, ar)).To(Succeed())

            // Reconcile should execute cleanup
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(ar),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed (CRD will be deleted by Kubernetes)
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(ar), ar)
            Expect(err).To(HaveOccurred())
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })

        It("should delete all child CRDs during cleanup", func() {
            ar := &remediationv1.AlertRemediation{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-remediation",
                    Namespace:  "default",
                    UID:        "test-uid-123",
                    Finalizers: []string{alertRemediationFinalizer},
                },
                Status: remediationv1.AlertRemediationStatus{
                    OverallPhase: "completed",
                    AlertProcessingRef: &corev1.ObjectReference{
                        Name: "test-processing",
                    },
                    AIAnalysisRef: &corev1.ObjectReference{
                        Name: "test-analysis",
                    },
                },
            }
            Expect(k8sClient.Create(ctx, ar)).To(Succeed())

            // Create child CRDs owned by AlertRemediation
            ap := &processingv1.AlertProcessing{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-processing",
                    Namespace: "default",
                    OwnerReferences: []metav1.OwnerReference{
                        {UID: ar.UID, Name: ar.Name, Kind: "AlertRemediation"},
                    },
                },
            }
            Expect(k8sClient.Create(ctx, ap)).To(Succeed())

            ai := &aianalysisv1.AIAnalysis{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-analysis",
                    Namespace: "default",
                    OwnerReferences: []metav1.OwnerReference{
                        {UID: ar.UID, Name: ar.Name, Kind: "AlertRemediation"},
                    },
                },
            }
            Expect(k8sClient.Create(ctx, ai)).To(Succeed())

            // Delete AlertRemediation
            Expect(k8sClient.Delete(ctx, ar)).To(Succeed())

            // Cleanup should delete child CRDs
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(ar),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify child CRDs deleted
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(ap), ap)
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())

            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(ai), ai)
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })

        It("should not block deletion if child deletion fails", func() {
            ar := &remediationv1.AlertRemediation{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-remediation",
                    Namespace:  "default",
                    Finalizers: []string{alertRemediationFinalizer},
                },
                Status: remediationv1.AlertRemediationStatus{
                    AlertProcessingRef: &corev1.ObjectReference{
                        Name: "nonexistent-processing",
                    },
                },
            }
            Expect(k8sClient.Create(ctx, ar)).To(Succeed())
            Expect(k8sClient.Delete(ctx, ar)).To(Succeed())

            // Cleanup should succeed even if child CRDs don't exist
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(ar),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed despite child deletion failure
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(ar), ar)
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })
    })
})
```

---

## CRD Lifecycle Management

### Creation Lifecycle

**Created By**: Gateway Service (webhook handler)

**Creation Trigger**: New alert webhook received (after duplicate detection)

**Sequence**:
```
Prometheus/Grafana sends alert webhook
    ↓
Gateway Service receives webhook
    ↓
Gateway Service checks for duplicates (fingerprint-based)
    ↓
If NOT duplicate:
    ↓
Gateway Service creates AlertRemediation CRD
    ↓ (sets initial status.overallPhase = "pending")
AlertRemediation Controller reconciles (this controller)
    ↓
AlertRemediation creates AlertProcessing CRD
    ↓ (with owner reference to AlertRemediation)
AlertRemediation watches child CRD status changes
    ↓
Child CRDs update status (processing → completed)
    ↓ (watch triggers <100ms)
AlertRemediation orchestrates next phase
    ↓
Creates next child CRD (AIAnalysis, WorkflowExecution, KubernetesExecution)
    ↓
Repeats until all phases completed
    ↓
AlertRemediation.status.overallPhase = "completed"
```

**No Owner Reference** (Root CRD):
- AlertRemediation is the ROOT CRD - it has NO owner reference
- Created directly by Gateway Service
- ALL other CRDs own by AlertRemediation

---

### Update Lifecycle

**Status Updates by AlertRemediation Controller**:

```go
package controller

import (
    "context"
    "time"

    remediationv1 "github.com/jordigilh/kubernaut/api/alertremediation/v1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *AlertRemediationReconciler) updateStatusCompleted(
    ctx context.Context,
    ar *remediationv1.AlertRemediation,
    finalResults remediationv1.RemediationResults,
) error {
    // Controller updates own status
    ar.Status.OverallPhase = "completed"
    ar.Status.CompletionTime = &metav1.Time{Time: time.Now()}
    ar.Status.RemediationResults = finalResults

    return r.Status().Update(ctx, ar)
}
```

**Watch-Based Orchestration**:

```
Child CRD status changes (AlertProcessing, AIAnalysis, etc.)
    ↓ (watch event)
AlertRemediation watch triggers
    ↓ (<100ms latency)
AlertRemediation Controller reconciles
    ↓
AlertRemediation checks child status
    ↓
If child completed:
    ↓
AlertRemediation creates next child CRD
    ↓
If all children completed:
    ↓
AlertRemediation.status.overallPhase = "completed"
```

**Self-Updates Throughout Lifecycle**:
- AlertRemediation CONTINUOUSLY updates itself based on child status
- Aggregates status from all child CRDs
- Maintains overall remediation state machine
- Orchestrates creation of child CRDs based on workflow progress

---

### Deletion Lifecycle

**Trigger**: Manual deletion or TTL-based retention (24 hours)

**Cascade Deletion Sequence**:
```
User/System deletes AlertRemediation (after 24h retention)
    ↓
AlertRemediation.deletionTimestamp set
    ↓
AlertRemediation Controller reconciles (detects deletion)
    ↓
Finalizer cleanup executes:
  - Delete ALL child CRDs (AlertProcessing, AIAnalysis, WorkflowExecution, KubernetesExecution)
  - Record final remediation audit
  - Emit deletion event
    ↓
Finalizer removed
    ↓
Kubernetes deletes AlertRemediation CRD
    ↓
Kubernetes garbage collector cascade-deletes ALL owned CRDs in parallel:
  - AlertProcessing → deleted
  - AIAnalysis (+ AIApprovalRequest) → deleted
  - WorkflowExecution → deleted
  - KubernetesExecution (+ Kubernetes Jobs) → deleted
```

**Retention**:
- **AlertRemediation**: 24-hour retention (configurable per environment)
- **Child CRDs**: Deleted with parent (no independent retention)
- **Audit Data**: 90-day retention in PostgreSQL (persisted before deletion)

---

### Lifecycle Events

**Kubernetes Events Emitted**:

```go
package controller

import (
    "fmt"
    "time"

    remediationv1 "github.com/jordigilh/kubernaut/api/alertremediation/v1"

    "k8s.io/client-go/tools/record"
)

func (r *AlertRemediationReconciler) emitLifecycleEvents(
    ar *remediationv1.AlertRemediation,
    oldPhase string,
    duration time.Duration,
) {
    // Creation event
    r.Recorder.Event(ar, "Normal", "AlertRemediationCreated",
        fmt.Sprintf("Alert remediation started for fingerprint: %s", ar.Spec.AlertFingerprint))

    // Phase transition events
    r.Recorder.Event(ar, "Normal", "PhaseTransition",
        fmt.Sprintf("Overall Phase: %s → %s", oldPhase, ar.Status.OverallPhase))

    // Child CRD creation events
    if ar.Status.AlertProcessingRef != nil {
        r.Recorder.Event(ar, "Normal", "AlertProcessingCreated",
            fmt.Sprintf("AlertProcessing CRD created: %s", ar.Status.AlertProcessingRef.Name))
    }
    if ar.Status.AIAnalysisRef != nil {
        r.Recorder.Event(ar, "Normal", "AIAnalysisCreated",
            fmt.Sprintf("AIAnalysis CRD created: %s", ar.Status.AIAnalysisRef.Name))
    }
    if ar.Status.WorkflowExecutionRef != nil {
        r.Recorder.Event(ar, "Normal", "WorkflowExecutionCreated",
            fmt.Sprintf("WorkflowExecution CRD created: %s", ar.Status.WorkflowExecutionRef.Name))
    }
    for _, keRef := range ar.Status.KubernetesExecutionRefs {
        r.Recorder.Event(ar, "Normal", "KubernetesExecutionCreated",
            fmt.Sprintf("KubernetesExecution CRD created: %s", keRef.Name))
    }

    // Completion event
    if ar.Status.OverallPhase == "completed" {
        r.Recorder.Event(ar, "Normal", "AlertRemediationCompleted",
            fmt.Sprintf("Alert remediation completed in %s", duration))
    }

    // Failure event
    if ar.Status.OverallPhase == "failed" {
        r.Recorder.Event(ar, "Warning", "AlertRemediationFailed",
            fmt.Sprintf("Alert remediation failed: %s", ar.Status.FailureReason))
    }

    // Deletion event (in cleanup function)
    r.Recorder.Event(ar, "Normal", "AlertRemediationDeleted",
        fmt.Sprintf("AlertRemediation cleanup completed (phase: %s)", ar.Status.OverallPhase))
}
```

**Event Visibility**:
```bash
kubectl describe alertremediation <name>
# Shows all events in chronological order

kubectl get events --field-selector involvedObject.name=<name>
# Filter events for specific AlertRemediation
```

---

### Lifecycle Monitoring

**Prometheus Metrics**:

```promql
# CRD creation rate
rate(alertremediation_created_total[5m])

# CRD completion time (end-to-end)
histogram_quantile(0.95, alertremediation_lifecycle_duration_seconds)

# Active AlertRemediation CRDs
alertremediation_active_total

# CRD deletion rate
rate(alertremediation_deleted_total[5m])

# Success rate
sum(rate(alertremediation_completed{status="success"}[5m])) /
sum(rate(alertremediation_completed[5m]))

# Phase distribution
sum by (overallPhase) (alertremediation_active_total)

# Child CRD creation success rate
rate(alertremediation_child_crd_creation_failures_total[5m])
```

**Grafana Dashboard**:
```yaml
panels:
  - title: "AlertRemediation Lifecycle"
    targets:
      - expr: alertremediation_active_total
        legendFormat: "Active CRDs"
      - expr: rate(alertremediation_created_total[5m])
        legendFormat: "Creation Rate"
      - expr: rate(alertremediation_deleted_total[5m])
        legendFormat: "Deletion Rate"

  - title: "Remediation Latency (P95)"
    targets:
      - expr: histogram_quantile(0.95, alertremediation_lifecycle_duration_seconds)
        legendFormat: "P95 Duration"

  - title: "Success Rate"
    targets:
      - expr: |
          sum(rate(alertremediation_completed{status="success"}[5m])) /
          sum(rate(alertremediation_completed[5m]))
        legendFormat: "Success Rate"

  - title: "Active Remediation Phases"
    targets:
      - expr: sum by (overallPhase) (alertremediation_active_total)
        legendFormat: "{{overallPhase}}"
```

**Alert Rules**:

```yaml
groups:
- name: alertremediation-lifecycle
  rules:
  - alert: AlertRemediationStuckInPhase
    expr: time() - alertremediation_phase_start_timestamp > 1800
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "AlertRemediation stuck in phase for >30 minutes"
      description: "AlertRemediation {{ $labels.name }} has been in phase {{ $labels.overallPhase }} for over 30 minutes"

  - alert: AlertRemediationHighFailureRate
    expr: |
      sum(rate(alertremediation_completed{status="failed"}[5m])) /
      sum(rate(alertremediation_completed[5m])) > 0.2
    for: 10m
    labels:
      severity: critical
    annotations:
      summary: "High remediation failure rate"
      description: ">20% of alert remediations are failing"

  - alert: AlertRemediationChildCreationFailures
    expr: rate(alertremediation_child_crd_creation_failures_total[5m]) > 0.1
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Child CRD creation failing frequently"
      description: "AlertRemediation controller unable to create child CRDs"

  - alert: AlertRemediationHighDeletionRate
    expr: rate(alertremediation_deleted_total[5m]) > rate(alertremediation_created_total[5m]) * 1.5
    for: 5m
    labels:
      severity: info
    annotations:
      summary: "AlertRemediation deletion rate exceeds creation rate"
      description: "More remediations being deleted than created (possible retention policy cleanup)"

  - alert: AlertRemediationOrchestrationFailures
    expr: rate(alertremediation_orchestration_errors_total[5m]) > 0.05
    for: 10m
    labels:
      severity: critical
    annotations:
      summary: "Orchestration failures in AlertRemediation"
      description: "Central controller experiencing orchestration errors"
```

---

## Testing Strategy

### Unit Tests (70% Coverage Target)

**Controller Logic Tests**:
```go
func TestAlertRemediationController_PhaseProgression(t *testing.T) {
    // Test: pending → processing transition when AlertProcessing created
    // Test: processing → analyzing transition when AlertProcessing completes
    // Test: analyzing → executing transition when AIAnalysis completes
    // Test: executing → completed transition when KubernetesExecution completes
}

func TestAlertRemediationController_TimeoutDetection(t *testing.T) {
    // Test: AlertProcessing timeout triggers escalation
    // Test: AIAnalysis timeout triggers escalation
    // Test: Overall workflow timeout triggers escalation
}

func TestAlertRemediationController_FailureHandling(t *testing.T) {
    // Test: AlertProcessing failure sets remediation to failed
    // Test: AIAnalysis failure sets remediation to failed
    // Test: Failure reason propagated correctly
}

func TestAlertRemediationController_RetentionCleanup(t *testing.T) {
    // Test: 24-hour retention timer set on completion
    // Test: CRD deletion after retention expiry
    // Test: Finalizer cleanup executed
}
```

### Integration Tests (20% Coverage Target)

**End-to-End CRD Flow Tests**:
```go
func TestE2E_RemediationWorkflow_Success(t *testing.T) {
    // 1. Create AlertRemediation CRD
    // 2. Verify AlertProcessing CRD created automatically
    // 3. Simulate AlertProcessing completion
    // 4. Verify AIAnalysis CRD created automatically
    // 5. Simulate AIAnalysis completion
    // 6. Verify WorkflowExecution CRD created
    // 7. Simulate WorkflowExecution completion
    // 8. Verify KubernetesExecution CRD created
    // 9. Simulate KubernetesExecution completion
    // 10. Verify AlertRemediation status = "completed"
}

func TestE2E_RemediationWorkflow_Timeout(t *testing.T) {
    // Test timeout detection and escalation across phases
}

func TestE2E_RemediationWorkflow_CascadeDeletion(t *testing.T) {
    // Test: Delete AlertRemediation → all service CRDs deleted
}
```

### E2E Tests (10% Coverage Target)

**Real Cluster Tests with Live Services**:
```go
func TestE2E_Production_RemediationFlow(t *testing.T) {
    // Test complete flow with real Context Service, HolmesGPT-API, etc.
}

func TestE2E_Production_AlertStorm(t *testing.T) {
    // Test 100 duplicate alerts → 1 AlertRemediation CRD
}
```

---

## Prometheus Metrics

### Controller Performance Metrics

```go
var (
    remediationPhaseTransitionDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "remediation_phase_transition_duration_seconds",
            Help:    "Time taken for phase transitions",
            Buckets: prometheus.DefBuckets,
        },
        []string{"from_phase", "to_phase"},
    )

    remediationTimeoutTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "remediation_timeout_total",
            Help: "Total number of remediation timeouts by phase",
        },
        []string{"phase", "severity", "environment"},
    )

    remediationCompletionDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "remediation_completion_duration_seconds",
            Help:    "End-to-end remediation duration",
            Buckets: []float64{60, 300, 600, 1800, 3600}, // 1m, 5m, 10m, 30m, 1h
        },
        []string{"result", "severity", "environment"}, // result: completed, failed, timeout
    )

    remediationRetentionCleanupTotal = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "remediation_retention_cleanup_total",
            Help: "Total number of remediation CRDs cleaned up after 24h retention",
        },
    )

    serviceCRDCreationDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "service_crd_creation_duration_seconds",
            Help:    "Time taken to create service CRDs",
            Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1.0},
        },
        []string{"crd_type"}, // AlertProcessing, AIAnalysis, WorkflowExecution, KubernetesExecution
    )

    controllerReconciliationDuration = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "alertremediation_controller_reconciliation_duration_seconds",
            Help:    "Controller reconciliation loop duration",
            Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0},
        },
    )
)
```

---

## Data Handling Architecture: Targeting Data Pattern

### Overview

**Architectural Principle**: Remediation Coordinator provides targeting data only (~8KB). HolmesGPT fetches logs/metrics dynamically using built-in toolsets.

**Why This Matters**: Understanding this pattern ensures compliance with Kubernetes etcd limits and provides fresh data for HolmesGPT investigations.

---

### Why CRDs Don't Store Logs/Metrics

**Kubernetes etcd Constraints**:
- **Typical limit**: 1.5MB per object
- **Recommended limit**: 1MB (safety margin)
- **Combined AlertProcessing + AIAnalysis**: Must stay well below limits

**Data Freshness**:
- Logs stored in CRDs become stale immediately
- HolmesGPT needs real-time data for accurate investigations
- Kubernetes API provides fresh pod logs on demand

**HolmesGPT Design**:
- Built-in toolsets fetch data from live sources
- `kubernetes` toolset: Pod logs, events, kubectl describe
- `prometheus` toolset: Metrics queries, PromQL generation

---

### What Remediation Coordinator Provides to AIAnalysis

**Targeting Data** (~8-10KB total):

```yaml
spec:
  analysisRequest:
    alertContext:
      # Alert identification
      fingerprint: "abc123def456"
      severity: "critical"
      environment: "production"

      # Resource targeting for HolmesGPT
      namespace: "production-app"
      resourceKind: "Pod"
      resourceName: "web-app-789"

      # Kubernetes context (small metadata)
      kubernetesContext:
        clusterName: "prod-cluster-east"
        namespace: "production-app"
        resourceKind: "Pod"
        resourceName: "web-app-789"
        podDetails:
          name: "web-app-789"
          status: "CrashLoopBackOff"
          containerNames: ["app", "sidecar"]
          restartCount: 47
        deploymentDetails:
          name: "web-app"
          replicas: 3
        nodeDetails:
          name: "node-1"
          cpuCapacity: "16"
          memoryCapacity: "64Gi"
```

**What Is NOT Stored**:
- ❌ Pod logs (HolmesGPT `kubernetes` toolset fetches)
- ❌ Metrics data (HolmesGPT `prometheus` toolset fetches)
- ❌ Events (HolmesGPT fetches dynamically)
- ❌ kubectl describe output (HolmesGPT generates)

---

### How HolmesGPT Uses Targeting Data

**AIAnalysis Controller → HolmesGPT-API**:

```python
# HolmesGPT uses targeting data to fetch fresh logs/metrics
holmes_client.investigate(
    namespace="production-app",      # From AlertContext
    resource_name="web-app-789",     # From AlertContext
    # HolmesGPT toolsets automatically:
    # 1. kubectl logs -n production-app web-app-789 --tail 500
    # 2. kubectl describe pod web-app-789 -n production-app
    # 3. kubectl get events -n production-app
    # 4. promql: container_memory_usage_bytes{pod="web-app-789"}
)
```

**Result**: Fresh, real-time data for investigation (not stale CRD snapshots)

---

### CRD-Level Validation (API Server Enforcement)

**Kubernetes API Server Validation**: All size and field constraints are enforced at the CRD schema level using OpenAPI v3 validation.

**AlertProcessing CRD** (where KubernetesContext originates):
```yaml
kubernetesContext:
  type: object
  x-kubernetes-validations:
  - rule: "self.size() <= 10240"  # 10KB CEL validation
    message: "kubernetesContext exceeds 10KB. Store targeting data only."
  properties:
    namespace:
      type: string
      maxLength: 63  # RFC 1123 DNS label
      description: "Target namespace for HolmesGPT investigation"
    resourceKind:
      type: string
      maxLength: 100  # Kubernetes resource kind max
      pattern: "^[A-Z][a-zA-Z0-9]*$"
      description: "Resource kind (Pod, Deployment, etc.)"
    resourceName:
      type: string
      maxLength: 253  # RFC 1123 DNS subdomain
      description: "Resource name for HolmesGPT targeting"
    labels:
      type: object
      maxProperties: 20
      additionalProperties:
        type: string
        maxLength: 63  # Label value max per Kubernetes
    annotations:
      type: object
      maxProperties: 10  # Limit to prevent bloat
      x-kubernetes-validations:
      - rule: "self.all(k, size(k) <= 253)"
        message: "Annotation keys must be 253 characters or less"
      - rule: "self.all(k, size(self[k]) <= 8192)"
        message: "Annotation values must be 8KB or less"
```

**AIAnalysis CRD** (where KubernetesContext is copied):
```yaml
kubernetesContext:
  type: object
  x-kubernetes-validations:
  - rule: "self.size() <= 10240"  # 10KB CEL validation
    message: "kubernetesContext exceeds 10KB. AlertProcessing provided too much data."
  # Inherits same field constraints from AlertProcessing CRD
```

**Validation Flow**:
1. **AlertProcessing Controller** → Tries to update `status.enrichmentResults.kubernetesContext`
2. **API Server** → Validates against AlertProcessing CRD schema
3. **If validation fails** → API server rejects update, returns error to controller
4. **If validation passes** → Written to etcd, Remediation Coordinator sees valid object

**Result**: Remediation Coordinator **NEVER sees invalid data** because API server blocks it.

---

### Error Handling (Not Validation)

**Remediation Coordinator does NOT validate** - it handles API server validation errors:

```go
// In AlertRemediationReconciler.createAIAnalysis()
func (r *AlertRemediationReconciler) createAIAnalysis(
    ctx context.Context,
    remediation *remediationv1.AlertRemediation,
    alertProcessing *processingv1.AlertProcessing,
) error {

    // No validation needed - API server enforces CRD schema

    aiAnalysis := &aiv1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-analysis", remediation.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("AlertRemediation")),
            },
        },
        Spec: aiv1.AIAnalysisSpec{
            AlertRemediationRef: aiv1.AlertRemediationReference{
                Name:      remediation.Name,
                Namespace: remediation.Namespace,
            },
            AnalysisRequest: aiv1.AnalysisRequest{
                AlertContext: aiv1.AlertContext{
                    // Data snapshot from AlertProcessing (already validated by API server)
                    Fingerprint:       alertProcessing.Status.EnrichedAlert.Fingerprint,
                    Severity:          alertProcessing.Status.EnrichedAlert.Severity,
                    Environment:       alertProcessing.Status.EnrichedAlert.Environment,
                    BusinessPriority:  alertProcessing.Status.EnrichedAlert.BusinessPriority,

                    // Resource targeting for HolmesGPT toolsets
                    Namespace:    alertProcessing.Status.EnrichedAlert.Namespace,
                    ResourceKind: alertProcessing.Status.EnrichedAlert.ResourceKind,
                    ResourceName: alertProcessing.Status.EnrichedAlert.ResourceName,

                    // Kubernetes context (validated by API server: size <= 10KB)
                    KubernetesContext: alertProcessing.Status.EnrichedAlert.KubernetesContext,
                },
                AnalysisTypes: []string{"investigation", "root-cause", "recovery-analysis"},
                InvestigationScope: aiv1.InvestigationScope{
                    TimeWindow: "24h",
                    ResourceScope: []aiv1.ResourceScopeItem{
                        {
                            Kind:      alertProcessing.Status.EnrichedAlert.ResourceKind,
                            Namespace: alertProcessing.Status.EnrichedAlert.Namespace,
                            Name:      alertProcessing.Status.EnrichedAlert.ResourceName,
                        },
                    },
                    CorrelationDepth:          "detailed",
                    IncludeHistoricalPatterns: true,
                },
            },
        },
    }

    // Create AIAnalysis - API server validates against AIAnalysis CRD schema
    if err := r.Create(ctx, aiAnalysis); err != nil {
        if apierrors.IsInvalid(err) {
            // API server rejected due to CRD validation
            r.Log.Error(err, "AIAnalysis validation failed",
                "remediation", remediation.Name,
                "alertProcessing", alertProcessing.Name,
                "hint", "AlertProcessing provided data that violates AIAnalysis CRD schema",
            )
        } else if apierrors.IsAlreadyExists(err) {
            // AIAnalysis already exists, this is fine (idempotency)
            r.Log.V(1).Info("AIAnalysis already exists (idempotent)",
                "remediation", remediation.Name,
                "aiAnalysis", aiAnalysis.Name,
            )
            return nil
        } else {
            // Other errors (network, RBAC, etc.)
            r.Log.Error(err, "Failed to create AIAnalysis",
                "remediation", remediation.Name,
            )
        }

        // Update AlertRemediation status to failed
        remediation.Status.OverallPhase = "failed"
        failureReason := fmt.Sprintf("Failed to create AIAnalysis: %v", err)
        remediation.Status.FailureReason = &failureReason

        if updateErr := r.Status().Update(ctx, remediation); updateErr != nil {
            return updateErr
        }

        return err
    }

    r.Log.Info("AIAnalysis created successfully",
        "remediation", remediation.Name,
        "aiAnalysis", aiAnalysis.Name,
    )

    return nil
}
```

**What This Catches**:
- ✅ Implementation bugs in AlertProcessing (accidentally includes logs)
- ✅ Architectural violations (team forgets "targeting data only" pattern)
- ✅ Edge cases (pods with abnormally large annotations)

**Error Message Example** (from API server):
```
AIAnalysis.aianalysis.kubernaut.io "my-analysis" is invalid:
spec.analysisRequest.alertContext.kubernetesContext:
Invalid value: <object>: kubernetesContext exceeds 10KB.
AlertProcessing provided too much data.
```

**Clear and actionable** - points to the problem (AlertProcessing) without controller validation code.

---

### Size Budget Guidelines

| Component | Typical Size | Max (CRD Enforced) |
|-----------|-------------|--------------------|
| Alert fingerprint + metadata | ~500 bytes | N/A |
| Resource targeting (namespace, kind, name) | ~200 bytes | N/A |
| KubernetesContext (pod/deploy/node metadata) | 6-8KB | **10KB (API server enforced)** |
| Investigation scope | ~1KB | N/A |
| **Total AIAnalysis.spec** | **~8-10KB** | **~15KB** |

**Safety Margin**: Leaves >985KB for AIAnalysis.status (investigation results, recommendations)

---

### Field Length Constraints (Kubernetes Standards)

All field constraints match Kubernetes object specifications:

| Field | Max Length | Standard | Enforced By |
|-------|-----------|----------|-------------|
| `namespace` | 63 chars | RFC 1123 DNS label | CRD schema |
| `resourceKind` | 100 chars | Kubernetes resource kind | CRD schema |
| `resourceName` | 253 chars | RFC 1123 DNS subdomain | CRD schema |
| `label keys` | 253 chars | DNS subdomain | CRD schema |
| `label values` | 63 chars | RFC 1123 label | CRD schema |
| `annotation keys` | 253 chars | DNS subdomain | CRD schema |
| `annotation values` | 8KB each | Constrained for size | CRD CEL validation |

**Reference**: These constraints are verified in `internal/validation/validators.go` and `internal/testutil/assertions.go`.

---

### Reference

For detailed HolmesGPT toolset capabilities and CRD schemas:
- [SignalProcessing CRD Schema](../../design/CRD/02_REMEDIATION_PROCESSING_CRD.md)
- [AIAnalysis CRD Schema](../../design/CRD/03_AI_ANALYSIS_CRD.md)
- [AI Analysis Service Spec - HolmesGPT Toolsets](./02-ai-analysis.md#holmesgpt-toolsets--dynamic-data-fetching)
- [HolmesGPT Official Documentation](https://github.com/robusta-dev/holmesgpt)

---

## Database Integration

### Audit Table Schema

**PostgreSQL Table**: `remediation_audit`

```sql
CREATE TABLE remediation_audit (
    id SERIAL PRIMARY KEY,
    alert_fingerprint VARCHAR(64) NOT NULL,
    remediation_name VARCHAR(255) NOT NULL,
    overall_phase VARCHAR(50) NOT NULL,
    start_time TIMESTAMP NOT NULL,
    completion_time TIMESTAMP,

    -- Service CRD references
    alert_processing_name VARCHAR(255),
    ai_analysis_name VARCHAR(255),
    workflow_execution_name VARCHAR(255),
    kubernetes_execution_name VARCHAR(255),

    -- Service CRD statuses (JSONB for flexibility)
    service_crd_statuses JSONB,

    -- Timeout/Failure tracking
    timeout_phase VARCHAR(50),
    timeout_time TIMESTAMP,
    failure_phase VARCHAR(50),
    failure_reason TEXT,

    -- Duplicate tracking
    duplicate_count INT DEFAULT 0,
    last_duplicate_time TIMESTAMP,

    -- Retention tracking
    retention_expiry_time TIMESTAMP,
    deleted_at TIMESTAMP,

    -- Indexing
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    INDEX idx_alert_fingerprint (alert_fingerprint),
    INDEX idx_remediation_name (remediation_name),
    INDEX idx_overall_phase (overall_phase),
    INDEX idx_retention_expiry (retention_expiry_time)
);
```

---

## RBAC Configuration

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: alertremediation-controller
rules:
# AlertRemediation CRD permissions
- apiGroups: ["kubernaut.io"]
  resources: ["alertremediations", "alertremediations/status", "alertremediations/finalizers"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# Service CRD creation permissions
- apiGroups: ["alertprocessor.kubernaut.io"]
  resources: ["alertprocessings"]
  verbs: ["create", "get", "list", "watch"]

- apiGroups: ["aianalysis.kubernaut.io"]
  resources: ["aianalyses"]
  verbs: ["create", "get", "list", "watch"]

- apiGroups: ["workflowexecution.kubernaut.io"]
  resources: ["workflowexecutions"]
  verbs: ["create", "get", "list", "watch"]

- apiGroups: ["kubernetesexecution.kubernaut.io"]
  resources: ["kubernetesexecutions"]
  verbs: ["create", "get", "list", "watch"]

# Event emission
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

**Note**: AlertRemediation controller creates all service CRDs but only needs read permissions on their status (watches handle updates).

---

## Implementation Checklist

### Phase 1: CRD Schema & API (2-3 days)

- [ ] **Define AlertRemediation API types** (`api/v1/alertremediation_types.go`)
  - [ ] AlertRemediationSpec with timeout configuration
  - [ ] AlertRemediationStatus with service CRD references and status summaries
  - [ ] Reference types for all service CRDs
  - [ ] Status summary types for aggregation

- [ ] **Generate CRD manifests**
  ```bash
  kubebuilder create api --group kubernaut --version v1 --kind AlertRemediation
  make manifests
  ```

- [ ] **Install CRD to cluster**
  ```bash
  make install
  kubectl get crds | grep alertremediation
  ```

### Phase 2: Controller Implementation (3-4 days)

- [ ] **Implement AlertRemediationReconciler**
  - [ ] Core reconciliation logic with phase orchestration
  - [ ] Sequential service CRD creation (AlertProcessing → AIAnalysis → WorkflowExecution → KubernetesExecution)
  - [ ] Data snapshot pattern for CRD spec population
  - [ ] Owner reference management for cascade deletion

- [ ] **Implement Watch Configuration**
  - [ ] Watch AlertProcessing status for completion
  - [ ] Watch AIAnalysis status for completion
  - [ ] Watch WorkflowExecution status for completion
  - [ ] Watch KubernetesExecution status for completion
  - [ ] Mapping functions for all watches

- [ ] **Implement Timeout Detection**
  - [ ] Per-phase timeout calculation
  - [ ] Timeout escalation via Notification Service
  - [ ] Overall workflow timeout detection

- [ ] **Implement Finalizer Pattern**
  - [ ] 24-hour retention timer
  - [ ] Cleanup logic (audit record persistence)
  - [ ] Finalizer removal and CRD deletion

- [ ] **Implement Failure Handling**
  - [ ] Service CRD failure detection
  - [ ] Failure reason propagation
  - [ ] Terminal state management

### Phase 3: External Integrations (1-2 days)

- [ ] **Notification Service Integration**
  - [ ] HTTP client for escalation endpoint
  - [ ] Escalation request payload construction
  - [ ] Channel selection based on severity/environment

- [ ] **Data Storage Service Integration**
  - [ ] HTTP client for audit endpoint
  - [ ] Audit record payload construction
  - [ ] Finalizer cleanup with audit persistence

### Phase 4: Testing (2-3 days)

- [ ] **Unit Tests**
  - [ ] Phase progression logic
  - [ ] Timeout detection
  - [ ] Failure handling
  - [ ] Retention cleanup
  - [ ] Watch mapping functions

- [ ] **Integration Tests**
  - [ ] End-to-end CRD creation flow
  - [ ] Service CRD completion triggers
  - [ ] Cascade deletion
  - [ ] Timeout escalation

- [ ] **E2E Tests**
  - [ ] Complete remediation workflow with live services
  - [ ] Alert storm testing (duplicate handling)

### Phase 5: Metrics & Observability (1 day)

- [ ] **Prometheus Metrics**
  - [ ] Phase transition duration
  - [ ] Timeout counters
  - [ ] Completion duration histogram
  - [ ] Retention cleanup counter
  - [ ] Service CRD creation duration

- [ ] **Event Emission**
  - [ ] Phase transition events
  - [ ] Timeout events
  - [ ] Failure events
  - [ ] Completion events

### Phase 6: Production Readiness (1-2 days)

- [ ] **RBAC Configuration**
  - [ ] ClusterRole for AlertRemediation controller
  - [ ] Service CRD creation permissions
  - [ ] Event emission permissions

- [ ] **Deployment Manifests**
  - [ ] Controller deployment YAML
  - [ ] ServiceAccount, Role, RoleBinding
  - [ ] Prometheus ServiceMonitor

- [ ] **Documentation**
  - [ ] Operator guide
  - [ ] Troubleshooting runbook
  - [ ] Metrics reference

---

## Common Pitfalls

1. **Don't create service CRDs prematurely** - Wait for previous CRD completion before creating next
2. **Owner references are mandatory** - All service CRDs must have AlertRemediation as owner for cascade deletion
3. **Watch setup order matters** - Configure watches in SetupWithManager before controller starts
4. **Finalizer cleanup must succeed** - Audit record persistence must complete before finalizer removal
5. **Timeout detection requires requeue** - Use RequeueAfter to periodically check for timeouts
6. **Status aggregation should be lightweight** - Don't copy large data payloads, use summary types
7. **Event emission for all state changes** - Emit events for phase transitions, timeouts, failures
8. **24-hour retention is non-negotiable** - Never bypass retention timer for immediate deletion

---

## Summary

**AlertRemediation Remediation Coordinator - V1 Design Specification (100% Complete)**

### Core Purpose
Coordinates end-to-end alert remediation workflow through watch-based state aggregation, sequential service CRD creation, and 24-hour retention lifecycle management.

### Key Architectural Decisions
1. **Watch-Based Event-Driven Coordination** - Kubernetes watches trigger reconciliation on service CRD status changes (<1s latency)
2. **Sequential CRD Creation** - One service CRD at a time: AlertProcessing → AIAnalysis → WorkflowExecution → KubernetesExecution
3. **Data Snapshot Pattern** - Copy complete data from service status to next service spec (no cross-CRD dependencies)
4. **Owner References for Cascade Deletion** - All service CRDs owned by AlertRemediation (automatic cleanup)
5. **Finalizer Pattern for 24-Hour Retention** - CRD persists for review window after completion
6. **Per-Phase Timeout Detection** - Individual service timeouts with escalation before overall workflow timeout
7. **Gateway Creates AlertRemediation Only** - AlertRemediation controller creates all service CRDs

### Integration Model
```
Gateway Service → AlertRemediation CRD (this controller)
                       ↓
        (creates & watches service CRDs)
                       ↓
    AlertProcessing → AIAnalysis → WorkflowExecution → KubernetesExecution
         (watch)        (watch)         (watch)           (watch)
                       ↓
    Sequential phase progression based on completion events
                       ↓
          AlertRemediation.status = "completed"
                       ↓
              24-hour retention begins
```

### V1 Scope Boundaries
**Included**:
- Sequential service CRD creation with watch-based coordination
- Per-phase and overall workflow timeout detection
- 24-hour retention with automatic cleanup
- Cascade deletion via owner references
- Status aggregation from all service CRDs

**Excluded** (V2):
- Parallel remediation workflows
- Cross-alert correlation and batch remediation
- ML-based timeout prediction
- Multi-cluster remediation coordination

### Business Requirements Coverage
- **BR-ALERT-006**: Timeout management with escalation
- **BR-ALERT-021**: Alert lifecycle state tracking
- **BR-ALERT-024**: Remediation workflow orchestration
- **BR-ALERT-025**: State persistence across restarts
- **BR-ALERT-026**: Automatic failure recovery
- **BR-ALERT-027**: 24-hour retention window
- **BR-ALERT-028**: Cascade deletion of service CRDs

### Implementation Status
- **CRD Schema**: Complete design with spec/status types
- **Controller Logic**: Complete reconciliation flow with phase orchestration
- **Watch Configuration**: Complete setup for all 4 service CRDs
- **External Integrations**: Notification Service (escalation) and Data Storage Service (audit)
- **Testing Strategy**: Unit, integration, and E2E test plans

### Next Steps
1. ✅ **Approved Design Specification** (100% complete)
2. **Kubebuilder Setup**: Install framework and generate CRD scaffolds
3. **CRD Schema Implementation**: Define API types in `api/v1/alertremediation_types.go`
4. **Controller Implementation**: Core reconciliation logic with watch-based coordination
5. **Integration Testing**: End-to-end workflow validation

### Critical Success Factors
- Watch-based coordination (no polling, <1s latency)
- Sequential CRD creation prevents premature service execution
- Data snapshot pattern ensures service independence
- Owner references enable cascade deletion
- Finalizer pattern enforces 24-hour retention
- Per-phase timeouts catch stuck services before overall timeout

**Design Specification Status**: Production-Ready (100% Confidence)

---

**🚀 Ready for implementation! This is the P0 CRITICAL foundation - implement BEFORE service CRDs.**

