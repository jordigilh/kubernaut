/*
Package signalprocessing provides interfaces for signal processing components.

These interfaces enable dependency injection and testability:
- K8sEnricher: Enriches Kubernetes context from cluster state
- EnvironmentClassifier: Classifies signal environment (production, staging, etc.)
- PriorityAssigner: Assigns priority based on environment and severity

Per APDC methodology, these interfaces are the contracts that unit tests mock,
while integration tests use real implementations.
*/
package signalprocessing

import (
	"context"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// K8sEnricher enriches signal context with Kubernetes cluster state.
// BR-SP-001: K8s context enrichment with caching, timeout, metrics
// This interface enables unit testing without requiring a real K8s cluster.
type K8sEnricher interface {
	// Enrich queries the Kubernetes API to build rich context for a signal.
	// Returns namespace, pod, deployment, and other relevant cluster state.
	Enrich(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error)
}

// EnvironmentClassifier determines the environment classification for a signal.
// BR-SP-051: Primary detection from namespace labels (via Rego policy)
// BR-SP-052: ConfigMap fallback (deprecated - use Rego)
// BR-SP-053: Default to "unknown" via Rego default rule
type EnvironmentClassifier interface {
	// Classify determines the environment (e.g., production, staging, development)
	// based on Kubernetes context and signal data.
	Classify(ctx context.Context, k8sCtx *signalprocessingv1alpha1.KubernetesContext, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.EnvironmentClassification, error)
}

// PriorityAssigner determines the priority for a signal based on environment and severity.
// BR-SP-070: Rego-based priority assignment
// BR-SP-071: Priority assignment is mandatory - no fallback
// BR-SP-072: Hot-reload from mounted ConfigMap via fsnotify
type PriorityAssigner interface {
	// Assign determines the priority (e.g., P0, P1, P2, P3) based on
	// Kubernetes context, environment classification, and signal data.
	Assign(ctx context.Context, k8sCtx *signalprocessingv1alpha1.KubernetesContext, envClass *signalprocessingv1alpha1.EnvironmentClassification, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.PriorityAssignment, error)
}

