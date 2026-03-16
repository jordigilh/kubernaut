package signalprocessing

import (
	"context"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/evaluator"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// mockPolicyEvaluator implements controller.PolicyEvaluator for unit tests.
// ADR-060: Replaces mockEnvironmentClassifier, mockPriorityAssigner, and severity/labels mocks.
type mockPolicyEvaluator struct {
	EvaluateEnvironmentFunc  func(ctx context.Context, input evaluator.PolicyInput) (*signalprocessingv1alpha1.EnvironmentClassification, error)
	EvaluatePriorityFunc     func(ctx context.Context, input evaluator.PolicyInput) (*signalprocessingv1alpha1.PriorityAssignment, error)
	EvaluateSeverityFunc     func(ctx context.Context, input evaluator.PolicyInput) (*evaluator.SeverityResult, error)
	EvaluateCustomLabelsFunc func(ctx context.Context, input evaluator.PolicyInput) (map[string][]string, error)
	PolicyHash               string
}

func (m *mockPolicyEvaluator) EvaluateEnvironment(ctx context.Context, input evaluator.PolicyInput) (*signalprocessingv1alpha1.EnvironmentClassification, error) {
	if m.EvaluateEnvironmentFunc != nil {
		return m.EvaluateEnvironmentFunc(ctx, input)
	}
	return &signalprocessingv1alpha1.EnvironmentClassification{
		Environment:  "production",
		Source:       "mock",
		ClassifiedAt: metav1.Now(),
	}, nil
}

func (m *mockPolicyEvaluator) EvaluatePriority(ctx context.Context, input evaluator.PolicyInput) (*signalprocessingv1alpha1.PriorityAssignment, error) {
	if m.EvaluatePriorityFunc != nil {
		return m.EvaluatePriorityFunc(ctx, input)
	}
	return &signalprocessingv1alpha1.PriorityAssignment{
		Priority:   "P1",
		Source:     "mock",
		AssignedAt: metav1.Now(),
	}, nil
}

func (m *mockPolicyEvaluator) EvaluateSeverity(ctx context.Context, input evaluator.PolicyInput) (*evaluator.SeverityResult, error) {
	if m.EvaluateSeverityFunc != nil {
		return m.EvaluateSeverityFunc(ctx, input)
	}
	return &evaluator.SeverityResult{
		Severity:   "high",
		Source:     "mock",
		PolicyHash: m.PolicyHash,
	}, nil
}

func (m *mockPolicyEvaluator) EvaluateCustomLabels(ctx context.Context, input evaluator.PolicyInput) (map[string][]string, error) {
	if m.EvaluateCustomLabelsFunc != nil {
		return m.EvaluateCustomLabelsFunc(ctx, input)
	}
	return make(map[string][]string), nil
}

func (m *mockPolicyEvaluator) GetPolicyHash() string {
	if m.PolicyHash != "" {
		return m.PolicyHash
	}
	return "mock-policy-hash"
}

func newDefaultMockPolicyEvaluator() *mockPolicyEvaluator {
	return &mockPolicyEvaluator{}
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
	}

	// If client provided, enrich with real K8s data from fake client
	if m.Client != nil {
		if signal.TargetResource.Namespace != "" {
			ns := &corev1.Namespace{}
			nsKey := client.ObjectKey{Name: signal.TargetResource.Namespace}
			if err := m.Client.Get(ctx, nsKey, ns); err == nil {
				k8sCtx.Namespace.Labels = ns.Labels
				k8sCtx.Namespace.Annotations = ns.Annotations
			}
		}

		// Enrich Pod details
		if signal.TargetResource.Kind == "Pod" {
			pod := &corev1.Pod{}
			podKey := client.ObjectKey{
				Name:      signal.TargetResource.Name,
				Namespace: signal.TargetResource.Namespace,
			}
			if err := m.Client.Get(ctx, podKey, pod); err == nil {
				k8sCtx.Workload = &signalprocessingv1alpha1.WorkloadDetails{
					Kind:   "Pod",
					Name:   pod.Name,
					Labels: pod.Labels,
				}
			}
		}
	}

	return k8sCtx, nil
}

// newDefaultMockK8sEnricher creates a mock that returns minimal K8s context.
func newDefaultMockK8sEnricher() *mockK8sEnricher {
	return &mockK8sEnricher{}
}

// newMockK8sEnricherWithClient creates a mock that enriches using the provided fake client.
func newMockK8sEnricherWithClient(c client.Client) *mockK8sEnricher {
	return &mockK8sEnricher{Client: c}
}
