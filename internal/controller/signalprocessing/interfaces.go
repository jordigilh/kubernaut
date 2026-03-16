/*
Package signalprocessing provides interfaces for signal processing components.

These interfaces enable dependency injection and testability:
- K8sEnricher: Enriches Kubernetes context from cluster state
- PolicyEvaluator: Unified Rego evaluator for environment, severity, priority, custom labels

Per APDC methodology, these interfaces are the contracts that unit tests mock,
while integration tests use real implementations.
*/
package signalprocessing

import (
	"context"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/evaluator"
)

// K8sEnricher enriches signal context with Kubernetes cluster state.
// BR-SP-001: K8s context enrichment with caching, timeout, metrics
// This interface enables unit testing without requiring a real K8s cluster.
type K8sEnricher interface {
	// Enrich queries the Kubernetes API to build rich context for a signal.
	// Returns namespace, pod, deployment, and other relevant cluster state.
	Enrich(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error)
}

// PolicyEvaluator provides unified Rego policy evaluation for all classification rules.
// ADR-060: Replaces EnvironmentClassifier, PriorityAssigner, SeverityClassifier, and RegoEngine
// with a single evaluator backed by one policy.rego file.
//
// BR-SP-051: Environment classification
// BR-SP-070: Priority assignment
// BR-SP-102: CustomLabels extraction
// BR-SP-105: Severity determination
type PolicyEvaluator interface {
	EvaluateEnvironment(ctx context.Context, input evaluator.PolicyInput) (*signalprocessingv1alpha1.EnvironmentClassification, error)
	EvaluatePriority(ctx context.Context, input evaluator.PolicyInput) (*signalprocessingv1alpha1.PriorityAssignment, error)
	EvaluateSeverity(ctx context.Context, input evaluator.PolicyInput) (*evaluator.SeverityResult, error)
	EvaluateCustomLabels(ctx context.Context, input evaluator.PolicyInput) (map[string][]string, error)
	GetPolicyHash() string
}

