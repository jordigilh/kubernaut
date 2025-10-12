package configmap

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// configMapBuilder implements ConfigMapBuilder
// BR-TOOLSET-029: ConfigMap builder with override preservation
type configMapBuilder struct {
	name      string
	namespace string
}

// NewConfigMapBuilder creates a new ConfigMap builder
func NewConfigMapBuilder(name, namespace string) ConfigMapBuilder {
	return &configMapBuilder{
		name:      name,
		namespace: namespace,
	}
}

// BuildConfigMap creates a new ConfigMap from toolset JSON
func (b *configMapBuilder) BuildConfigMap(ctx context.Context, toolsetJSON string) (*corev1.ConfigMap, error) {
	// Validate JSON
	if err := b.validateJSON(toolsetJSON); err != nil {
		return nil, err
	}

	// Create ConfigMap
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.name,
			Namespace: b.namespace,
			Labels:    b.standardLabels(),
			Annotations: map[string]string{
				"kubernaut.io/generated-at": time.Now().Format(time.RFC3339),
			},
		},
		Data: map[string]string{
			"toolset.json": toolsetJSON,
		},
	}

	return cm, nil
}

// BuildConfigMapWithOverrides creates a ConfigMap preserving manual overrides
// BR-TOOLSET-030: Preserve manual overrides and custom data
func (b *configMapBuilder) BuildConfigMapWithOverrides(ctx context.Context, toolsetJSON string, existing *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	// If no existing ConfigMap, create new one
	if existing == nil {
		return b.BuildConfigMap(ctx, toolsetJSON)
	}

	// Validate JSON
	if err := b.validateJSON(toolsetJSON); err != nil {
		return nil, err
	}

	// Start with existing ConfigMap structure
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        b.name,
			Namespace:   b.namespace,
			Labels:      b.mergeLabels(existing.Labels),
			Annotations: b.mergeAnnotations(existing.Annotations),
		},
		Data: b.mergeData(existing.Data, toolsetJSON),
	}

	return cm, nil
}

// DetectDrift checks if the current ConfigMap differs from the new toolset JSON
// BR-TOOLSET-031: Drift detection for reconciliation
func (b *configMapBuilder) DetectDrift(ctx context.Context, current *corev1.ConfigMap, newToolsetJSON string) bool {
	// If ConfigMap doesn't exist, there's drift
	if current == nil {
		return true
	}

	// Compare normalized JSON content
	currentJSON := current.Data["toolset.json"]
	return !b.jsonEqual(currentJSON, newToolsetJSON)
}

// validateJSON checks if the JSON is valid and non-empty
func (b *configMapBuilder) validateJSON(toolsetJSON string) error {
	if toolsetJSON == "" {
		return fmt.Errorf("toolset JSON cannot be empty")
	}

	// Parse JSON to validate
	var data interface{}
	if err := json.Unmarshal([]byte(toolsetJSON), &data); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	return nil
}

// standardLabels returns the standard labels for the ConfigMap
func (b *configMapBuilder) standardLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       b.name,
		"app.kubernetes.io/component":  "dynamic-toolset",
		"app.kubernetes.io/managed-by": "kubernaut",
	}
}

// mergeLabels merges standard labels with existing custom labels
func (b *configMapBuilder) mergeLabels(existing map[string]string) map[string]string {
	merged := b.standardLabels()

	// Preserve custom labels from existing ConfigMap
	for key, value := range existing {
		// Don't override standard labels
		if _, isStandard := merged[key]; !isStandard {
			merged[key] = value
		}
	}

	return merged
}

// mergeAnnotations merges managed annotations with existing custom annotations
func (b *configMapBuilder) mergeAnnotations(existing map[string]string) map[string]string {
	// Start with new timestamp
	merged := map[string]string{
		"kubernaut.io/generated-at": time.Now().Format(time.RFC3339),
	}

	// Preserve custom annotations from existing ConfigMap
	for key, value := range existing {
		// Preserve non-managed annotations
		if key != "kubernaut.io/generated-at" {
			merged[key] = value
		}
	}

	return merged
}

// mergeData merges managed data with existing custom data keys
func (b *configMapBuilder) mergeData(existing map[string]string, newToolsetJSON string) map[string]string {
	// Start with new toolset.json
	merged := map[string]string{
		"toolset.json": newToolsetJSON,
	}

	// Preserve custom data keys from existing ConfigMap
	for key, value := range existing {
		// Only preserve non-managed keys
		if key != "toolset.json" {
			merged[key] = value
		}
	}

	return merged
}

// jsonEqual compares two JSON strings for semantic equality (ignoring whitespace)
func (b *configMapBuilder) jsonEqual(json1, json2 string) bool {
	// Normalize JSON by unmarshaling and remarshaling
	var obj1, obj2 interface{}

	if err := json.Unmarshal([]byte(json1), &obj1); err != nil {
		return false
	}

	if err := json.Unmarshal([]byte(json2), &obj2); err != nil {
		return false
	}

	// Marshal both to compare
	bytes1, err1 := json.Marshal(obj1)
	bytes2, err2 := json.Marshal(obj2)

	if err1 != nil || err2 != nil {
		return false
	}

	return string(bytes1) == string(bytes2)
}
