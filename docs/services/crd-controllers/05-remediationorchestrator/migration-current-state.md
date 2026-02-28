# Current State & Migration Path

## Overview

The Remediation Orchestrator (RemediationRequest Controller) is the **central coordinator** of the multi-CRD reconciliation architecture. It is a **new implementation** (not migrating existing code) that creates and manages the lifecycle of 4 downstream CRDs:

1. **RemediationProcessing** - Signal enrichment and classification
2. **AIAnalysis** - HolmesGPT analysis and recommendation generation
3. **WorkflowExecution** - Workflow orchestration and step management
4. **KubernetesExecution** (DEPRECATED - ADR-025) - Action execution via Jobs (indirectly via WorkflowExecution)

**Key Characteristic**: This controller **creates** CRDs but **does not contain business logic**. All business logic resides in the downstream controllers.

---

## Current State

### Existing Implementation

**Location**: `internal/controller/remediation/remediationrequest_controller.go` (already implemented)
**Status**: ✅ **Functional** (Task 2.2 completed)
**Lines**: ~200 lines of controller logic

**Key Components**:
```go
// RemediationRequestReconciler orchestrates CRD creation
type RemediationRequestReconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

// Reconcile creates SignalProcessing CRD
func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Fetch RemediationRequest
    var remediationRequest remediationv1alpha1.RemediationRequest
    if err := r.Get(ctx, req.NamespacedName, &remediationRequest); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Create RemediationProcessing if it doesn't exist
    processingName := fmt.Sprintf("%s-processing", remediationRequest.Name)
    var existingProcessing remediationprocessingv1alpha1.RemediationProcessing
    err := r.Get(ctx, client.ObjectKey{
        Name:      processingName,
        Namespace: remediationRequest.Namespace,
    }, &existingProcessing)

    if errors.IsNotFound(err) {
        processing := &remediationprocessingv1alpha1.RemediationProcessing{
            ObjectMeta: metav1.ObjectMeta{
                Name:      processingName,
                Namespace: remediationRequest.Namespace,
                Labels:    mapLabels(remediationRequest),
                Annotations: mapAnnotations(remediationRequest),
            },
            Spec: mapRemediationRequestToProcessingSpec(&remediationRequest),
        }

        // Set owner reference for cascade deletion
        if err := ctrl.SetControllerReference(&remediationRequest, processing, r.Scheme); err != nil {
            return ctrl.Result{}, err
        }

        if err := r.Create(ctx, processing); err != nil {
            return ctrl.Result{}, err
        }
    }

    return ctrl.Result{}, nil
}

// SetupWithManager registers the controller
func (r *RemediationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&remediationv1alpha1.RemediationRequest{}).
        Owns(&remediationprocessingv1alpha1.RemediationProcessing{}).
        Named("remediation-remediationrequest").
        Complete(r)
}
```

**What Exists**:
- ✅ RemediationRequest CRD schema
- ✅ RemediationRequestReconciler controller
- ✅ SignalProcessing CRD creation logic
- ✅ Field mapping from RemediationRequest to RemediationProcessing
- ✅ Owner reference management
- ✅ RBAC permissions
- ✅ `cmd/remediationorchestrator/main.go` entry point

---

## What's Missing (V1 Requirements)

### 1. AIAnalysis CRD Creation (After RemediationProcessing Completes)

**Status**: ❌ Not implemented
**Business Requirement**: BR-ORCH-015
**Priority**: P0 - CRITICAL

**Required Logic**:
```go
// In RemediationRequestReconciler
func (r *RemediationRequestReconciler) reconcileAIAnalysis(
    ctx context.Context,
    remediation *remediationv1alpha1.RemediationRequest,
    processing *remediationprocessingv1alpha1.RemediationProcessing,
) error {
    // Wait for RemediationProcessing to complete
    if processing.Status.Phase != "completed" {
        return nil // Not ready yet
    }

    // Check if AIAnalysis already exists
    analysisName := fmt.Sprintf("%s-analysis", remediation.Name)
    var existingAnalysis aianalysisv1alpha1.AIAnalysis
    err := r.Get(ctx, client.ObjectKey{
        Name:      analysisName,
        Namespace: remediation.Namespace,
    }, &existingAnalysis)

    if errors.IsNotFound(err) {
        analysis := &aianalysisv1alpha1.AIAnalysis{
            ObjectMeta: metav1.ObjectMeta{
                Name:      analysisName,
                Namespace: remediation.Namespace,
                Labels: map[string]string{
                    "remediation-request": remediation.Name,
                    "signal-type":         remediation.Spec.SignalType,
                },
            },
            Spec: aianalysisv1alpha1.AIAnalysisSpec{
                RemediationRequestRef: aianalysisv1alpha1.RemediationRequestReference{
                    Name:      remediation.Name,
                    Namespace: remediation.Namespace,
                },
                // Copy enriched signal data from RemediationProcessing status
                AnalysisRequest: mapEnrichedSignalToAnalysisRequest(processing.Status.EnrichedSignal),
            },
        }

        if err := ctrl.SetControllerReference(remediation, analysis, r.Scheme); err != nil {
            return err
        }

        return r.Create(ctx, analysis)
    }

    return nil
}
```

**Estimated Effort**: 1 day

---

### 2. WorkflowExecution CRD Creation (After AIAnalysis Completes)

**Status**: ❌ Not implemented
**Business Requirement**: BR-ORCH-016
**Priority**: P0 - CRITICAL

**Required Logic**:
```go
// In RemediationRequestReconciler
func (r *RemediationRequestReconciler) reconcileWorkflowExecution(
    ctx context.Context,
    remediation *remediationv1alpha1.RemediationRequest,
    analysis *aianalysisv1alpha1.AIAnalysis,
) error {
    // Wait for AIAnalysis to complete
    if analysis.Status.Phase != "completed" {
        return nil // Not ready yet
    }

    // Check if WorkflowExecution already exists
    workflowName := fmt.Sprintf("%s-workflow", remediation.Name)
    var existingWorkflow workflowv1alpha1.WorkflowExecution
    err := r.Get(ctx, client.ObjectKey{
        Name:      workflowName,
        Namespace: remediation.Namespace,
    }, &existingWorkflow)

    if errors.IsNotFound(err) {
        workflow := &workflowv1alpha1.WorkflowExecution{
            ObjectMeta: metav1.ObjectMeta{
                Name:      workflowName,
                Namespace: remediation.Namespace,
                Labels: map[string]string{
                    "remediation-request": remediation.Name,
                    "signal-fingerprint":  remediation.Spec.SignalFingerprint,
                },
            },
            Spec: workflowv1alpha1.WorkflowExecutionSpec{
                RemediationRequestRef: workflowv1alpha1.RemediationRequestReference{
                    Name:      remediation.Name,
                    Namespace: remediation.Namespace,
                },
                // Copy recommended workflow from AIAnalysis status
                Workflow: analysis.Status.RecommendedWorkflow,
            },
        }

        if err := ctrl.SetControllerReference(remediation, workflow, r.Scheme); err != nil {
            return err
        }

        return r.Create(ctx, workflow)
    }

    return nil
}
```

**Estimated Effort**: 1 day

---

### 3. Status Watching & Phase Progression

**Status**: ❌ Not implemented
**Business Requirement**: BR-ORCH-017
**Priority**: P0 - CRITICAL

**Required Logic**:
```go
// Updated Reconcile with status watching
func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := logf.FromContext(ctx)

    // Fetch RemediationRequest
    var remediation remediationv1alpha1.RemediationRequest
    if err := r.Get(ctx, req.NamespacedName, &remediation); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Phase 1: Create RemediationProcessing
    processing, err := r.reconcileRemediationProcessing(ctx, &remediation)
    if err != nil {
        log.Error(err, "Failed to reconcile RemediationProcessing")
        return ctrl.Result{}, err
    }

    // Update status phase
    remediation.Status.Phase = "processing"
    remediation.Status.RemediationProcessingRef = &remediationv1alpha1.CRDReference{
        Name:      processing.Name,
        Namespace: processing.Namespace,
    }

    // Phase 2: Create AIAnalysis (after RemediationProcessing completes)
    if processing != nil && processing.Status.Phase == "completed" {
        analysis, err := r.reconcileAIAnalysis(ctx, &remediation, processing)
        if err != nil {
            log.Error(err, "Failed to reconcile AIAnalysis")
            return ctrl.Result{}, err
        }

        remediation.Status.Phase = "analyzing"
        remediation.Status.AIAnalysisRef = &remediationv1alpha1.CRDReference{
            Name:      analysis.Name,
            Namespace: analysis.Namespace,
        }

        // Phase 3: Create WorkflowExecution (after AIAnalysis completes)
        if analysis != nil && analysis.Status.Phase == "completed" {
            workflow, err := r.reconcileWorkflowExecution(ctx, &remediation, analysis)
            if err != nil {
                log.Error(err, "Failed to reconcile WorkflowExecution")
                return ctrl.Result{}, err
            }

            remediation.Status.Phase = "executing"
            remediation.Status.WorkflowExecutionRef = &remediationv1alpha1.CRDReference{
                Name:      workflow.Name,
                Namespace: workflow.Namespace,
            }

            // Phase 4: Monitor WorkflowExecution completion
            if workflow != nil && workflow.Status.Phase == "completed" {
                remediation.Status.Phase = "completed"
                remediation.Status.CompletedAt = metav1.Now()
            }
        }
    }

    // Update RemediationRequest status
    if err := r.Status().Update(ctx, &remediation); err != nil {
        log.Error(err, "Failed to update RemediationRequest status")
        return ctrl.Result{}, err
    }

    // Requeue if not completed
    if remediation.Status.Phase != "completed" {
        return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
    }

    return ctrl.Result{}, nil
}
```

**Estimated Effort**: 2 days

---

### 4. Updated SetupWithManager (Watch All Owned CRDs)

**Status**: ❌ Not implemented (only watches RemediationProcessing currently)
**Business Requirement**: BR-ORCH-018
**Priority**: P0 - CRITICAL

**Required Changes**:
```go
func (r *RemediationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&remediationv1alpha1.RemediationRequest{}).
        Owns(&remediationprocessingv1alpha1.RemediationProcessing{}). // ✅ Already exists
        Owns(&aianalysisv1alpha1.AIAnalysis{}).                        // ❌ Need to add
        Owns(&workflowv1alpha1.WorkflowExecution{}).                   // ❌ Need to add
        Named("remediation-remediationrequest").
        Complete(r)
}
```

**Estimated Effort**: 30 minutes

---

### 5. RBAC Permissions (All CRDs)

**Status**: ❌ Incomplete (only has RemediationProcessing permissions)
**Business Requirement**: BR-ORCH-019
**Priority**: P0 - CRITICAL

**Current RBAC**:
```go
// +kubebuilder:rbac:groups=remediation.kubernaut.io,resources=remediationrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=remediation.kubernaut.io,resources=remediationrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=remediation.kubernaut.io,resources=remediationrequests/finalizers,verbs=update
// +kubebuilder:rbac:groups=signalprocessing.kubernaut.io,resources=remediationprocessings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=signalprocessing.kubernaut.io,resources=remediationprocessings/status,verbs=get;update;patch
```

**Missing RBAC**:
```go
// AIAnalysis CRD
// +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses/status,verbs=get;update;patch

// WorkflowExecution CRD
// +kubebuilder:rbac:groups=workflowexecution.kubernaut.io,resources=workflowexecutions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=workflowexecution.kubernaut.io,resources=workflowexecutions/status,verbs=get;update;patch
```

**Estimated Effort**: 30 minutes

---

### 6. Error Handling & Retry Logic

**Status**: ❌ Basic error handling only
**Business Requirement**: BR-ORCH-020
**Priority**: P1 - HIGH

**Required Enhancements**:
```go
// Add error tracking in status
type RemediationRequestStatus struct {
    Phase string `json:"phase,omitempty"`

    // Error tracking
    ErrorMessage string `json:"errorMessage,omitempty"`
    RetryCount   int    `json:"retryCount,omitempty"`
    MaxRetries   int    `json:"maxRetries,omitempty"`

    // Phase tracking
    RemediationProcessingRef *CRDReference `json:"remediationProcessingRef,omitempty"`
    AIAnalysisRef            *CRDReference `json:"aiAnalysisRef,omitempty"`
    WorkflowExecutionRef     *CRDReference `json:"workflowExecutionRef,omitempty"`

    // Timestamps
    StartedAt   *metav1.Time `json:"startedAt,omitempty"`
    CompletedAt *metav1.Time `json:"completedAt,omitempty"`
}

// Error handling in Reconcile
func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... existing logic ...

    // Check retry limit
    if remediation.Status.RetryCount >= remediation.Status.MaxRetries {
        remediation.Status.Phase = "failed"
        remediation.Status.ErrorMessage = "max retries exceeded"
        if err := r.Status().Update(ctx, &remediation); err != nil {
            return ctrl.Result{}, err
        }
        return ctrl.Result{}, nil // Don't requeue
    }

    // On error, increment retry count
    if err != nil {
        remediation.Status.RetryCount++
        remediation.Status.ErrorMessage = err.Error()
        if updateErr := r.Status().Update(ctx, &remediation); updateErr != nil {
            return ctrl.Result{}, updateErr
        }
        return ctrl.Result{RequeueAfter: time.Duration(remediation.Status.RetryCount) * 10 * time.Second}, nil
    }

    // ... continue with normal logic ...
}
```

**Estimated Effort**: 1 day

---

### 7. Prometheus Metrics

**Status**: ❌ Not implemented
**Business Requirement**: BR-ORCH-021
**Priority**: P1 - HIGH

**Required Metrics**:
```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Counter: RemediationRequest creations
    RemediationRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_orchestrator_remediation_requests_total",
        Help: "Total number of RemediationRequest CRDs created",
    }, []string{"signal_type", "environment"})

    // Counter: CRD creations by type
    CRDCreationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_orchestrator_crd_creations_total",
        Help: "Total CRD creations by type",
    }, []string{"crd_type"}) // remediationprocessing, aianalysis, workflowexecution

    // Counter: Phase transitions
    PhaseTransitionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_orchestrator_phase_transitions_total",
        Help: "Total phase transitions",
    }, []string{"from_phase", "to_phase"})

    // Histogram: End-to-end duration
    EndToEndDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "kubernaut_orchestrator_e2e_duration_seconds",
        Help:    "End-to-end duration from RemediationRequest to WorkflowExecution completion",
        Buckets: []float64{10, 30, 60, 120, 300, 600, 1200}, // 10s to 20min
    })

    // Gauge: Active RemediationRequests by phase
    ActiveRemediationRequestsByPhase = promauto.NewGaugeVec(prometheus.GaugeOpts{
        Name: "kubernaut_orchestrator_active_requests_by_phase",
        Help: "Number of active RemediationRequests by phase",
    }, []string{"phase"}) // processing, analyzing, executing, completed
)
```

**Estimated Effort**: 1 day

---

## Migration Strategy

### Phase 1: AIAnalysis Integration (Days 1-2)

**Goal**: Add AIAnalysis CRD creation after RemediationProcessing completes

1. **Implement `reconcileAIAnalysis` function**
2. **Add status watching for RemediationProcessing completion**
3. **Update RBAC permissions for AIAnalysis**
4. **Update `SetupWithManager` to own AIAnalysis CRDs**
5. **Add unit tests for AIAnalysis creation logic**

**Deliverables**:
- AIAnalysis CRD creation logic
- RBAC updates
- Unit tests (70%+ coverage)

---

### Phase 2: WorkflowExecution Integration (Days 3-4)

**Goal**: Add WorkflowExecution CRD creation after AIAnalysis completes

1. **Implement `reconcileWorkflowExecution` function**
2. **Add status watching for AIAnalysis completion**
3. **Update RBAC permissions for WorkflowExecution**
4. **Update `SetupWithManager` to own WorkflowExecution CRDs**
5. **Add unit tests for WorkflowExecution creation logic**

**Deliverables**:
- WorkflowExecution CRD creation logic
- RBAC updates
- Unit tests (70%+ coverage)

---

### Phase 3: Status & Error Handling (Days 5-6)

**Goal**: Enhanced status tracking and error handling

1. **Implement comprehensive status updates**
2. **Add retry logic with exponential backoff**
3. **Implement phase transition tracking**
4. **Add error message recording**
5. **Add integration tests for error scenarios**

**Deliverables**:
- Enhanced status management
- Retry logic
- Integration tests

---

### Phase 4: Observability & Metrics (Days 7-8)

**Goal**: Add Prometheus metrics and structured logging

1. **Implement Prometheus metrics**
2. **Add structured logging with correlation IDs**
3. **Create Grafana dashboards**
4. **Define alert rules for critical errors**

**Deliverables**:
- Complete metrics implementation
- Grafana dashboards
- Alert rules

---

### Phase 5: Testing & Documentation (Days 9-10)

**Goal**: Comprehensive testing and documentation

1. **Unit tests**: 70%+ coverage
2. **Integration tests**: Full orchestration scenarios
3. **E2E tests**: Complete RemediationRequest lifecycle
4. **Performance tests**: Measure orchestration overhead
5. **Documentation updates**: Runbook, troubleshooting guide

**Deliverables**:
- Complete test suite
- Updated documentation
- Performance benchmarks

---

## Implementation Gaps Summary

| Component | Status | Priority | Estimated Effort |
|-----------|--------|----------|------------------|
| RemediationProcessing creation | ✅ Complete | - | - |
| AIAnalysis creation | ❌ Missing | P0 | 1 day |
| WorkflowExecution creation | ❌ Missing | P0 | 1 day |
| Status watching & progression | ❌ Missing | P0 | 2 days |
| RBAC permissions (AIAnalysis, WorkflowExecution) | ❌ Missing | P0 | 30 min |
| Error handling & retry logic | ❌ Basic only | P1 | 1 day |
| Prometheus metrics | ❌ Missing | P1 | 1 day |
| Integration tests | ❌ Missing | P1 | 2 days |
| **Total** | **30% Complete** | - | **8-10 days** |

---

## Risk Assessment

### High Risk

- **Sequential CRD creation complexity**: Must wait for each CRD to complete before creating next
  - **Mitigation**: Clear phase tracking, comprehensive status updates

### Medium Risk

- **Status watching accuracy**: Must accurately detect completion of downstream CRDs
  - **Mitigation**: Use watch predicates, implement retry logic

- **RBAC complexity**: Controller needs permissions for 4 different CRD types
  - **Mitigation**: Automated RBAC generation via Kubebuilder markers

### Low Risk

- **Owner references**: Standard Kubernetes pattern
  - **Mitigation**: Follow existing patterns from RemediationProcessing

---

## Success Criteria

- [ ] RemediationRequest creates SignalProcessing CRD (✅ Complete)
- [ ] RemediationRequest creates AIAnalysis after RemediationProcessing completes
- [ ] RemediationRequest creates WorkflowExecution after AIAnalysis completes
- [ ] Status accurately reflects current phase
- [ ] Owner references enable cascade deletion
- [ ] Error handling includes retry logic
- [ ] Prometheus metrics track orchestration flow
- [ ] 70%+ code coverage with unit tests
- [ ] Integration tests validate full orchestration
- [ ] Documentation complete and accurate

---

## References

- **Existing Code**: `internal/controller/remediation/remediationrequest_controller.go` (current implementation)
- **Task 2.2**: Completed RemediationProcessing creation logic
- **Architecture**: [Multi-CRD Reconciliation](../../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)
- **Owner References**: [ADR-005: Owner Reference Architecture](../../../architecture/decisions/005-owner-reference-architecture.md)
- **Testing Strategy**: [testing-strategy.md](./testing-strategy.md)
- **Integration Points**: [integration-points.md](./integration-points.md)
