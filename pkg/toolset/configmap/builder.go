package configmap

import (
	"context"

	corev1 "k8s.io/api/core/v1"
)

// ConfigMapBuilder builds and manages Kubernetes ConfigMaps for toolset configurations
// BR-TOOLSET-029: ConfigMap builder with override preservation
type ConfigMapBuilder interface {
	// BuildConfigMap creates a new ConfigMap from toolset JSON
	BuildConfigMap(ctx context.Context, toolsetJSON string) (*corev1.ConfigMap, error)

	// BuildConfigMapWithOverrides creates a ConfigMap preserving manual overrides from existing ConfigMap
	// BR-TOOLSET-030: Preserve manual overrides and custom data
	BuildConfigMapWithOverrides(ctx context.Context, toolsetJSON string, existing *corev1.ConfigMap) (*corev1.ConfigMap, error)

	// DetectDrift checks if the current ConfigMap differs from the new toolset JSON
	// BR-TOOLSET-031: Drift detection for reconciliation
	DetectDrift(ctx context.Context, current *corev1.ConfigMap, newToolsetJSON string) bool
}
