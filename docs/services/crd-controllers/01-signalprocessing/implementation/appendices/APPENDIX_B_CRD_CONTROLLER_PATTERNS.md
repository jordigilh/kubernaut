# Appendix B: CRD Controller Patterns

**Part of**: Signal Processing Implementation Plan V1.23
**Parent Document**: [IMPLEMENTATION_PLAN.md](../IMPLEMENTATION_PLAN.md)
**Last Updated**: 2025-12-04
**Template Source**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md v3.0

---

## 📚 CRD Controller Variant

**Applicability**: Signal Processing is a CRD controller (5 out of 12 V1 services are CRD controllers)

**CRD Controllers in V1**:
- **SignalProcessing** (renamed from RemediationProcessor) ← THIS SERVICE
- AIAnalysis
- WorkflowExecution
- KubernetesExecutor (DEPRECATED - ADR-025)
- RemediationOrchestrator

---

## 🔷 CRD API Group Standard

**Reference**: [DD-CRD-001: API Group Domain Selection](../../../../../architecture/decisions/DD-CRD-001-api-group-domain-selection.md)

All Kubernaut CRDs use the **`.ai` domain** for AIOps branding:

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: SignalProcessing
```

**Decision Rationale** (per DD-CRD-001):
1. **K8sGPT Precedent**: AI K8s projects use `.ai` (e.g., `core.k8sgpt.ai`)
2. **Brand Alignment**: AIOps is the core value proposition - domain reflects this
3. **Differentiation**: Stands out from traditional infrastructure tooling (`.io`)
4. **Industry Trend**: AI-native platforms increasingly adopt `.ai`

**Note**: Label keys still use `kubernaut.io/` prefix (K8s label convention, not CRD API group).

### Industry Best Practices Analysis

| Project | API Group Strategy | Pattern |
|---------|-------------------|---------|
| **Tekton** | `tekton.dev/v1` | ✅ Unified - all CRDs under single domain |
| **Istio** | `istio.io/v1` | ✅ Unified - network, security, config all under `istio.io` |
| **Cert-Manager** | `cert-manager.io/v1` | ✅ Unified - certificates, issuers, challenges |
| **ArgoCD** | `argoproj.io/v1alpha1` | ✅ Unified - applications, projects, rollouts |
| **Crossplane** | `crossplane.io/v1` | ✅ Unified - compositions, providers |
| **Knative** | Multiple: `serving.knative.dev`, `eventing.knative.dev` | ⚠️ Split by domain |

**Conclusion**: 5/6 major CNCF projects use unified API groups. Splitting is only justified when projects have distinct product lines or independent release cycles.

### CRD Inventory (Unified API Group)

| CRD | API Group | Purpose |
|-----|-----------|---------|
| **SignalProcessing** | `kubernaut.ai/v1alpha1` | Context enrichment, classification |
| AIAnalysis | `kubernaut.ai/v1alpha1` | HolmesGPT RCA + workflow selection |
| WorkflowExecution | `kubernaut.ai/v1alpha1` | Ansible/K8s workflow execution |
| RemediationRequest | `remediation.kubernaut.ai/v1alpha1` | User-facing remediation entry point |

### RBAC Template for Signal Processing

```yaml
# kubebuilder markers for SignalProcessing controller
//+kubebuilder:rbac:groups=kubernaut.ai,resources=signalprocessings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernaut.ai,resources=signalprocessings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernaut.ai,resources=signalprocessings/finalizers,verbs=update

# Additional RBAC for K8s enrichment (DD-WORKFLOW-001 v2.2)
//+kubebuilder:rbac:groups="",resources=pods;deployments;replicasets;nodes;services;configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups=apps,resources=deployments;replicasets;statefulsets,verbs=get;list;watch
//+kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch
//+kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch
```

---

## CRD Controller-Specific Patterns

### 1. Reconciliation Loop Pattern

**Signal Processing Controller Structure**:
```go
package controller

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	spv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// SignalProcessingReconciler reconciles a SignalProcessing object
type SignalProcessingReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Enricher  *enricher.Enricher
	Classifier *classifier.Classifier
	Detector  *detector.LabelDetector
}

//+kubebuilder:rbac:groups=kubernaut.ai,resources=signalprocessings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernaut.ai,resources=signalprocessings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernaut.ai,resources=signalprocessings/finalizers,verbs=update

// Reconcile implements the reconciliation loop
func (r *SignalProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// 1. FETCH RESOURCE
	sp := &spv1alpha1.SignalProcessing{}
	if err := r.Get(ctx, req.NamespacedName, sp); err != nil {
		if apierrors.IsNotFound(err) {
			// Resource deleted, nothing to do
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get SignalProcessing")
		return ctrl.Result{}, err
	}

	// 2. CHECK TERMINAL STATES
	if sp.Status.Phase == "completed" || sp.Status.Phase == "failed" {
		log.Info("SignalProcessing already in terminal state", "phase", sp.Status.Phase)
		return ctrl.Result{}, nil
	}

	// 3. INITIALIZE STATUS
	if sp.Status.Phase == "" || sp.Status.Phase == "pending" {
		sp.Status.Phase = "enriching"
		now := metav1.Now()
		sp.Status.StartTime = &now

		if err := r.Status().Update(ctx, sp); err != nil {
			log.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}
	}

	// 4. BUSINESS LOGIC (enrichment, classification, detection)
	result, err := r.processSignal(ctx, sp)
	if err != nil {
		return r.handleError(ctx, sp, err)
	}

	// 5. UPDATE STATUS ON SUCCESS
	return r.updateStatusAndComplete(ctx, sp, result)
}

// SetupWithManager sets up the controller with the Manager
func (r *SignalProcessingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&spv1alpha1.SignalProcessing{}).
		Complete(r)
}
```

---

### 2. Status Update Patterns

**Signal Processing Status Update**:
```go
// updateStatusAndComplete updates resource status after successful processing
func (r *SignalProcessingReconciler) updateStatusAndComplete(
	ctx context.Context,
	sp *spv1alpha1.SignalProcessing,
	result *ProcessResult,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Update phase
	sp.Status.Phase = "completed"
	now := metav1.Now()
	sp.Status.CompletedAt = &now

	// Update enrichment results
	sp.Status.EnrichmentResults = result.EnrichmentResults

	// Update status subresource
	if err := r.Status().Update(ctx, sp); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("SignalProcessing complete",
		"name", sp.Name,
		"duration", time.Since(sp.Status.StartTime.Time),
	)
	return ctrl.Result{}, nil
}
```

---

### 3. Finalizer Pattern

**Signal Processing Finalizer** (for audit cleanup):
```go
const finalizerName = "kubernaut.ai/finalizer"

func (r *SignalProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	sp := &spv1alpha1.SignalProcessing{}
	if err := r.Get(ctx, req.NamespacedName, sp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if resource is being deleted
	if !sp.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(sp, finalizerName) {
			// Perform cleanup (e.g., audit finalization)
			if err := r.finalizeSignalProcessing(ctx, sp); err != nil {
				return ctrl.Result{}, err
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(sp, finalizerName)
			if err := r.Update(ctx, sp); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(sp, finalizerName) {
		controllerutil.AddFinalizer(sp, finalizerName)
		if err := r.Update(ctx, sp); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Continue with reconciliation...
	return r.processSignal(ctx, sp)
}
```

---

### 4. Exponential Backoff Requeue

**Signal Processing Backoff Implementation**:
```go
// calculateBackoff returns exponential backoff duration
// Attempts: 0→30s, 1→60s, 2→120s, 3→240s, 4+→480s (capped)
func calculateBackoff(attemptCount int) time.Duration {
	baseDelay := 30 * time.Second
	maxDelay := 480 * time.Second

	// Calculate exponential backoff: baseDelay * 2^attemptCount
	delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attemptCount)))

	// Cap at maximum delay
	if delay > maxDelay {
		delay = maxDelay
	}

	// Add jitter (±10%) to prevent thundering herd
	jitter := time.Duration(float64(delay) * (0.9 + 0.2*rand.Float64()))

	return jitter
}

// handleError processes errors with appropriate requeue strategy
func (r *SignalProcessingReconciler) handleError(
	ctx context.Context,
	sp *spv1alpha1.SignalProcessing,
	err error,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Determine if error is retryable
	if isRetryable(err) {
		// Increment attempt count (stored in annotation or status)
		attemptCount := getAttemptCount(sp)
		backoff := calculateBackoff(attemptCount)

		log.Info("Transient error, requeueing",
			"error", err.Error(),
			"attempt", attemptCount,
			"backoff", backoff,
		)

		return ctrl.Result{RequeueAfter: backoff}, nil
	}

	// Permanent failure - update status
	sp.Status.Phase = "failed"
	sp.Status.Error = err.Error()
	r.Status().Update(ctx, sp)

	log.Error(err, "Permanent failure, not retrying")
	return ctrl.Result{}, nil // Don't return error to prevent infinite retry
}
```

---

### 5. Phase State Machine Pattern

**Signal Processing Phases**:
```go
// Phase definitions
const (
	PhasePending     = "pending"
	PhaseEnriching   = "enriching"
	PhaseClassifying = "classifying"
	PhaseDetecting   = "detecting"
	PhaseCompleted   = "completed"
	PhaseFailed      = "failed"
)

// Phase transition validation
var validTransitions = map[string][]string{
	PhasePending:     {PhaseEnriching},
	PhaseEnriching:   {PhaseClassifying, PhaseFailed},
	PhaseClassifying: {PhaseDetecting, PhaseFailed},
	PhaseDetecting:   {PhaseCompleted, PhaseFailed},
	PhaseCompleted:   {}, // Terminal state
	PhaseFailed:      {}, // Terminal state
}

func validatePhaseTransition(current, next string) error {
	allowed, ok := validTransitions[current]
	if !ok {
		return fmt.Errorf("unknown current phase: %s", current)
	}

	for _, validNext := range allowed {
		if validNext == next {
			return nil
		}
	}

	return fmt.Errorf("invalid phase transition: %s → %s", current, next)
}
```

---

### 6. CRD Testing Patterns

**Fake Client Testing for Signal Processing**:
```go
var _ = Describe("SignalProcessing Controller", func() {
	var (
		ctx        context.Context
		reconciler *SignalProcessingReconciler
		scheme     *runtime.Scheme
		fakeClient client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = spv1alpha1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)

		// Create fake client with test resources
		sp := &spv1alpha1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-sp",
				Namespace: "default",
			},
			Spec: spv1alpha1.SignalProcessingSpec{
				SignalName:  "HighMemoryUsage",
				Severity:    "warning",
				Environment: "production",
				Priority:    "P2",
				TargetResource: spv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "my-pod",
					Namespace: "default",
				},
			},
		}

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sp).
			WithStatusSubresource(&spv1alpha1.SignalProcessing{}).
			Build()

		reconciler = &SignalProcessingReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}
	})

	It("should transition from pending to enriching", func() {
		result, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test-sp",
				Namespace: "default",
			},
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result.Requeue).To(BeFalse())

		// Verify status updated
		updated := &spv1alpha1.SignalProcessing{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-sp", Namespace: "default"}, updated)
		Expect(err).ToNot(HaveOccurred())
		Expect(updated.Status.Phase).To(Equal("enriching"))
	})
})
```

---

## CRD Controller Checklist

Use this checklist when implementing Signal Processing:

- [ ] **Reconciliation Loop**: Standard pattern with fetch → validate → process → update
- [ ] **Status Updates**: Status subresource updates with phase transitions
- [ ] **Finalizers**: Cleanup logic for resource deletion (audit finalization)
- [ ] **Exponential Backoff**: Transient error retry with jitter
- [ ] **Phase State Machine**: Valid phase transitions enforced (pending → enriching → classifying → detecting → completed)
- [ ] **RBAC Annotations**: Kubebuilder RBAC markers complete (including K8s enrichment permissions)
- [ ] **Fake Client Tests**: Unit tests use `fake.NewClientBuilder()`
- [ ] **Integration Tests**: ENVTEST with real CRDs
- [ ] **Manager Setup**: `SetupWithManager()` implemented
- [ ] **Metrics**: Controller-specific metrics (reconciliations, errors, duration)

---

## Related Documents

- [Main Implementation Plan](../IMPLEMENTATION_PLAN.md)
- [Appendix A: Integration Test Environment](APPENDIX_A_INTEGRATION_TEST_ENVIRONMENT.md)
- [Appendix C: Confidence Methodology](APPENDIX_C_CONFIDENCE_METHODOLOGY.md)
- [Appendix D: ADR/DD Reference Matrix](APPENDIX_D_ADR_DD_REFERENCE_MATRIX.md)
- [DD-006: Controller Scaffolding](../../../../../architecture/decisions/DD-006-controller-scaffolding-strategy.md)
- [ADR-004: Fake K8s Client](../../../../../architecture/decisions/ADR-004-fake-kubernetes-client.md)

