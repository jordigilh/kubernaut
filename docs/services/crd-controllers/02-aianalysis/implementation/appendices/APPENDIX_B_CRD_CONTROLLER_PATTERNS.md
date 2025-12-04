# AI Analysis Service - CRD Controller Patterns

**Parent Document**: [IMPLEMENTATION_PLAN_V1.0.md](../IMPLEMENTATION_PLAN_V1.0.md)
**Version**: 1.0
**Reference**: DD-006, SignalProcessing patterns

---

## 🎯 **Purpose**

This appendix documents the CRD controller patterns specific to the AIAnalysis service, following the established patterns from DD-006 and SignalProcessing.

---

## 🔄 **Reconciliation Pattern**

### **Standard Reconciliation Flow**

```go
func (r *AIAnalysisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("aianalysis", req.NamespacedName)

	// Step 1: Fetch resource
	analysis := &aianalysisv1.AIAnalysis{}
	if err := r.Get(ctx, req.NamespacedName, analysis); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("AIAnalysis not found, likely deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Step 2: Handle deletion
	if !analysis.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, analysis)
	}

	// Step 3: Ensure finalizer
	if !controllerutil.ContainsFinalizer(analysis, FinalizerName) {
		controllerutil.AddFinalizer(analysis, FinalizerName)
		if err := r.Update(ctx, analysis); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Step 4: Route to phase handler
	return r.reconcilePhase(ctx, analysis)
}
```

### **Phase-Based Reconciliation**

```go
func (r *AIAnalysisReconciler) reconcilePhase(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	switch analysis.Status.Phase {
	case "", aianalysisv1.PhasePending:
		return r.initializePhase(ctx, analysis)
	case aianalysisv1.PhaseValidating:
		return r.ValidatingHandler.Handle(ctx, analysis)
	case aianalysisv1.PhaseInvestigating:
		return r.InvestigatingHandler.Handle(ctx, analysis)
	case aianalysisv1.PhaseAnalyzing:
		return r.AnalyzingHandler.Handle(ctx, analysis)
	case aianalysisv1.PhaseRecommending:
		return r.RecommendingHandler.Handle(ctx, analysis)
	case aianalysisv1.PhaseCompleted, aianalysisv1.PhaseFailed:
		return ctrl.Result{}, nil // Terminal phases
	default:
		return ctrl.Result{}, fmt.Errorf("unknown phase: %s", analysis.Status.Phase)
	}
}
```

---

## 🏁 **Finalizer Pattern**

### **Finalizer Lifecycle**

```
CREATE → ADD_FINALIZER → PROCESS → DELETE_REQUESTED → CLEANUP → REMOVE_FINALIZER → DELETED
```

### **Implementation**

```go
const FinalizerName = "aianalysis.kubernaut.ai/finalizer"

func (r *AIAnalysisReconciler) handleDeletion(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := r.Log.WithValues("aianalysis", analysis.Name)

	if !controllerutil.ContainsFinalizer(analysis, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Perform cleanup
	log.Info("Running finalizer cleanup")
	if err := r.cleanup(ctx, analysis); err != nil {
		log.Error(err, "Cleanup failed")
		return ctrl.Result{}, err
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(analysis, FinalizerName)
	if err := r.Update(ctx, analysis); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Finalizer cleanup complete")
	return ctrl.Result{}, nil
}

func (r *AIAnalysisReconciler) cleanup(ctx context.Context, analysis *aianalysisv1.AIAnalysis) error {
	// Record audit event
	r.Recorder.Event(analysis, "Normal", "Deleted",
		fmt.Sprintf("AIAnalysis deleted (final phase: %s)", analysis.Status.Phase))

	// Additional cleanup (metrics, state, etc.)
	return nil
}
```

---

## 📊 **Status Update Pattern**

### **Idempotent Status Updates**

```go
func (r *AIAnalysisReconciler) updateStatus(ctx context.Context, analysis *aianalysisv1.AIAnalysis, phase aianalysisv1.AIAnalysisPhase, message string) error {
	// Check if update is needed (idempotency)
	if analysis.Status.Phase == phase && analysis.Status.Message == message {
		return nil
	}

	// Record transition
	previousPhase := analysis.Status.Phase
	analysis.Status.Phase = phase
	analysis.Status.Message = message
	analysis.Status.LastTransitionTime = &metav1.Time{Time: time.Now()}

	// Update status subresource
	if err := r.Status().Update(ctx, analysis); err != nil {
		if apierrors.IsConflict(err) {
			r.Log.Info("Status update conflict, will retry")
			return nil // Let next reconciliation handle it
		}
		return err
	}

	// Record metrics
	metrics.PhaseTransitions.WithLabelValues(string(previousPhase), string(phase)).Inc()

	return nil
}
```

### **Status Fields**

```go
type AIAnalysisStatus struct {
	// Phase is the current reconciliation phase
	Phase AIAnalysisPhase `json:"phase,omitempty"`

	// Message is a human-readable status message
	Message string `json:"message,omitempty"`

	// LastTransitionTime is when the phase last changed
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// RootCause is the identified root cause
	RootCause string `json:"rootCause,omitempty"`

	// Confidence is the AI confidence score (0.0-1.0)
	Confidence float64 `json:"confidence,omitempty"`

	// SelectedWorkflow is the recommended workflow
	SelectedWorkflow *SelectedWorkflow `json:"selectedWorkflow,omitempty"`

	// ApprovalRequired indicates if manual approval is needed
	ApprovalRequired bool `json:"approvalRequired,omitempty"`

	// ApprovalDecision is the policy decision
	ApprovalDecision string `json:"approvalDecision,omitempty"`

	// TargetInOwnerChain indicates if target was in owner chain
	TargetInOwnerChain *bool `json:"targetInOwnerChain,omitempty"`

	// Warnings are non-fatal warnings from analysis
	Warnings []string `json:"warnings,omitempty"`
}
```

---

## 🔁 **Requeue Pattern**

### **Requeue Decision Matrix**

| Scenario | Result | Notes |
|----------|--------|-------|
| Successful phase transition | `Requeue: true` | Process next phase |
| Transient error | `RequeueAfter: 30s` | Exponential backoff |
| Permanent error | `Requeue: false` | Set Failed phase |
| Terminal phase | `Requeue: false` | No further processing |
| Status conflict | `Requeue: true` | Immediate retry |

### **Implementation**

```go
// Successful transition - immediate requeue
return ctrl.Result{Requeue: true}, nil

// Transient error - backoff
return ctrl.Result{RequeueAfter: 30 * time.Second}, nil

// Permanent error - no requeue
analysis.Status.Phase = aianalysisv1.PhaseFailed
r.Status().Update(ctx, analysis)
return ctrl.Result{}, nil

// Terminal phase - no requeue
return ctrl.Result{}, nil
```

---

## 📢 **Event Recording Pattern**

### **Event Types**

| Event | Type | Reason | When |
|-------|------|--------|------|
| Phase transition | Normal | PhaseTransition | Phase changes |
| Validation failed | Warning | ValidationFailed | Input invalid |
| HolmesGPT error | Warning | HolmesGPTError | API call fails |
| Rego error | Warning | RegoError | Policy fails |
| Approval decision | Normal | ApprovalDecision | Policy evaluated |
| Deleted | Normal | Deleted | Finalizer runs |

### **Implementation**

```go
// Phase transition
r.Recorder.Event(analysis, "Normal", "PhaseTransition",
	fmt.Sprintf("Transitioned from %s to %s", from, to))

// Validation failure
r.Recorder.Event(analysis, "Warning", "ValidationFailed",
	fmt.Sprintf("Validation failed: %s", err.Error()))

// HolmesGPT error
r.Recorder.Event(analysis, "Warning", "HolmesGPTError",
	fmt.Sprintf("HolmesGPT-API call failed: %s", err.Error()))

// Approval decision
r.Recorder.Event(analysis, "Normal", "ApprovalDecision",
	fmt.Sprintf("Approval decision: %s (confidence: %.2f)", decision, confidence))
```

---

## 🔐 **RBAC Pattern**

### **Minimal RBAC**

```go
// +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
```

### **ClusterRole**

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: aianalysis-controller
rules:
- apiGroups: ["aianalysis.kubernaut.io"]
  resources: ["aianalyses"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["aianalysis.kubernaut.io"]
  resources: ["aianalyses/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["aianalysis.kubernaut.io"]
  resources: ["aianalyses/finalizers"]
  verbs: ["update"]
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

---

## 🧪 **Testing Pattern**

### **Unit Test with Fake Client**

```go
func TestReconcile(t *testing.T) {
	g := NewGomegaWithT(t)

	// Create fake client
	scheme := runtime.NewScheme()
	aianalysisv1.AddToScheme(scheme)

	analysis := newTestAIAnalysis("test")
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(analysis).
		WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
		Build()

	// Create reconciler
	reconciler := &AIAnalysisReconciler{
		Client: client,
		Scheme: scheme,
		Log:    ctrl.Log.WithName("test"),
	}

	// Reconcile
	req := ctrl.Request{NamespacedName: types.NamespacedName{
		Name:      analysis.Name,
		Namespace: analysis.Namespace,
	}}
	result, err := reconciler.Reconcile(context.Background(), req)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Requeue).To(BeTrue())
}
```

---

## 📚 **References**

| Document | Purpose |
|----------|---------|
| [DD-006: Controller Scaffolding](../../../../architecture/decisions/DD-006-controller-scaffolding-strategy.md) | Scaffolding patterns |
| [SignalProcessing Controller](../../01-signalprocessing/controller-implementation.md) | Reference implementation |
| [controller-implementation.md](../../controller-implementation.md) | Detailed implementation |

