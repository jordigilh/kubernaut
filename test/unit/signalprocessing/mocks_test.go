package signalprocessing

import (
	"context"
	"strings"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// mockEnvironmentClassifier implements controller.EnvironmentClassifier for unit tests.
// It returns configurable results without requiring Rego policies.
type mockEnvironmentClassifier struct {
	// ClassifyFunc allows tests to customize behavior
	ClassifyFunc func(ctx context.Context, k8sCtx *signalprocessingv1alpha1.KubernetesContext, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.EnvironmentClassification, error)
}

// Classify implements EnvironmentClassifier interface.
func (m *mockEnvironmentClassifier) Classify(ctx context.Context, k8sCtx *signalprocessingv1alpha1.KubernetesContext, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.EnvironmentClassification, error) {
	if m.ClassifyFunc != nil {
		return m.ClassifyFunc(ctx, k8sCtx, signal)
	}
	// Default behavior: return production environment
	return &signalprocessingv1alpha1.EnvironmentClassification{
		Environment:  "production",
		Source:       "mock",
		ClassifiedAt: metav1.Now(),
	}, nil
}

// mockPriorityAssigner implements controller.PriorityAssigner for unit tests.
// It returns configurable results without requiring Rego policies.
type mockPriorityAssigner struct {
	// AssignFunc allows tests to customize behavior
	AssignFunc func(ctx context.Context, k8sCtx *signalprocessingv1alpha1.KubernetesContext, envClass *signalprocessingv1alpha1.EnvironmentClassification, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.PriorityAssignment, error)
}

// Assign implements PriorityAssigner interface.
func (m *mockPriorityAssigner) Assign(ctx context.Context, k8sCtx *signalprocessingv1alpha1.KubernetesContext, envClass *signalprocessingv1alpha1.EnvironmentClassification, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.PriorityAssignment, error) {
	if m.AssignFunc != nil {
		return m.AssignFunc(ctx, k8sCtx, envClass, signal)
	}
	// Default behavior: return P1 priority
	return &signalprocessingv1alpha1.PriorityAssignment{
		Priority:   "P1",
		Source:     "mock",
		AssignedAt: metav1.Now(),
	}, nil
}

// newDefaultMockEnvironmentClassifier creates a mock that returns production environment.
func newDefaultMockEnvironmentClassifier() *mockEnvironmentClassifier {
	return &mockEnvironmentClassifier{}
}

// newDefaultMockPriorityAssigner creates a mock that returns P1 priority.
func newDefaultMockPriorityAssigner() *mockPriorityAssigner {
	return &mockPriorityAssigner{}
}

// mockK8sEnricher implements controller.K8sEnricher for unit tests.
// It provides realistic enrichment data using the fake K8s client.
type mockK8sEnricher struct {
	// Client is the K8s client to query for enrichment data (optional)
	Client client.Client
	// EnrichFunc allows tests to customize behavior
	EnrichFunc func(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error)
}

// Enrich implements K8sEnricher interface.
// Provides realistic enrichment by querying the fake K8s client.
func (m *mockK8sEnricher) Enrich(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error) {
	if m.EnrichFunc != nil {
		return m.EnrichFunc(ctx, signal)
	}

	// Default behavior: Enrich using fake K8s client (mimics real enricher)
	k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
		Namespace: &signalprocessingv1alpha1.NamespaceContext{
			Name: signal.TargetResource.Namespace,
		},
		DetectedLabels: &signalprocessingv1alpha1.DetectedLabels{},
	}

	// If client provided, enrich with real K8s data from fake client
	if m.Client != nil {
		// Enrich namespace context with labels and detect patterns
		if signal.TargetResource.Namespace != "" {
			ns := &corev1.Namespace{}
			nsKey := client.ObjectKey{Name: signal.TargetResource.Namespace}
			if err := m.Client.Get(ctx, nsKey, ns); err == nil {
				k8sCtx.Namespace.Labels = ns.Labels
				k8sCtx.Namespace.Annotations = ns.Annotations

				// Detect production environment from labels
				if ns.Labels["env"] == "production" || ns.Labels["environment"] == "production" {
					k8sCtx.DetectedLabels.IsProduction = true
				}

				// Detect GitOps management from annotations
				for annKey := range ns.Annotations {
					if strings.HasPrefix(annKey, "argocd.argoproj.io/") || strings.HasPrefix(annKey, "fluxcd.io/") {
						k8sCtx.DetectedLabels.GitOpsManaged = true
						break
					}
				}
			}
		}

		// Detect PDB for Pod resources
		if signal.TargetResource.Kind == "Pod" {
			pod := &corev1.Pod{}
			podKey := client.ObjectKey{
				Name:      signal.TargetResource.Name,
				Namespace: signal.TargetResource.Namespace,
			}
			if err := m.Client.Get(ctx, podKey, pod); err == nil {
				// Store pod labels for PDB matching
				k8sCtx.Pod = &signalprocessingv1alpha1.PodDetails{
					Labels: pod.Labels,
				}

				// Check for PDB matching pod labels
				pdbList := &policyv1.PodDisruptionBudgetList{}
				if err := m.Client.List(ctx, pdbList, client.InNamespace(signal.TargetResource.Namespace)); err == nil {
					for _, pdb := range pdbList.Items {
						// Simple label matching (real enricher does more sophisticated matching)
						if matchesLabels(pdb.Spec.Selector.MatchLabels, pod.Labels) {
							k8sCtx.DetectedLabels.HasPDB = true
							break
						}
					}
				}
			}
		}

		// Detect HPA for Deployment resources
		if signal.TargetResource.Kind == "Deployment" {
			hpaList := &autoscalingv2.HorizontalPodAutoscalerList{}
			if err := m.Client.List(ctx, hpaList, client.InNamespace(signal.TargetResource.Namespace)); err == nil {
				for _, hpa := range hpaList.Items {
					if hpa.Spec.ScaleTargetRef.Name == signal.TargetResource.Name {
						k8sCtx.DetectedLabels.HasHPA = true
						break
					}
				}
			}
		}
	}

	return k8sCtx, nil
}

// matchesLabels checks if selector labels match target labels (simplified version).
func matchesLabels(selector, target map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if target[k] != v {
			return false
		}
	}
	return true
}

// newDefaultMockK8sEnricher creates a mock that returns minimal K8s context.
func newDefaultMockK8sEnricher() *mockK8sEnricher {
	return &mockK8sEnricher{}
}

// newMockK8sEnricherWithClient creates a mock that enriches using the provided fake client.
func newMockK8sEnricherWithClient(c client.Client) *mockK8sEnricher {
	return &mockK8sEnricher{Client: c}
}
