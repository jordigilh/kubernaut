// Package rego provides OPA Rego policy evaluation for SignalProcessing.
// This file implements CustomLabels extraction with security wrapper.
// BR-SP-102: CustomLabels Rego Extraction
// BR-SP-103: CustomLabels Validation Limits
// DD-WORKFLOW-001 v1.9: Security wrapper and validation limits
package rego

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/open-policy-agent/opa/v1/rego"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Validation limits per DD-WORKFLOW-001 v1.9
const (
	MaxKeys         = 10  // Max number of label keys (subdomains)
	MaxValuesPerKey = 5   // Max values per key
	MaxKeyLength    = 63  // K8s label key compatibility
	MaxValueLength  = 100 // Prompt efficiency
)

// Reserved prefixes that are stripped for security (DD-WORKFLOW-001 v1.9)
var ReservedPrefixes = []string{
	"kubernaut.ai/",
	"system/",
}

// LabelInput contains all data available to Rego policies for CustomLabels extraction.
// See: HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md v3.2
type LabelInput struct {
	Namespace      NamespaceContext           `json:"namespace"`
	Pod            PodContext                 `json:"pod,omitempty"`
	Deployment     DeploymentContext          `json:"deployment,omitempty"`
	Node           NodeContext                `json:"node,omitempty"`
	Signal         SignalContext              `json:"signal,omitempty"`
	DetectedLabels map[string]interface{}     `json:"detected_labels,omitempty"`
}

// NamespaceContext contains namespace information for Rego input.
type NamespaceContext struct {
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// PodContext contains pod information for Rego input.
type PodContext struct {
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// DeploymentContext contains deployment information for Rego input.
type DeploymentContext struct {
	Name        string            `json:"name"`
	Replicas    int32             `json:"replicas"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// NodeContext contains node information for Rego input.
type NodeContext struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
}

// SignalContext contains signal information for Rego input.
type SignalContext struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Source   string `json:"source"`
}

// CustomLabelsExtractor extracts CustomLabels via Rego policies from ConfigMap.
// DD-WORKFLOW-001 v1.9: Security wrapper and validation limits.
type CustomLabelsExtractor struct {
	client        client.Client
	logger        logr.Logger
	preparedQuery *rego.PreparedEvalQuery
	policyLoaded  bool
}

// NewCustomLabelsExtractor creates a new CustomLabels extractor.
func NewCustomLabelsExtractor(c client.Client, logger logr.Logger) *CustomLabelsExtractor {
	return &CustomLabelsExtractor{
		client: c,
		logger: logger.WithName("customlabels-extractor"),
	}
}

// LoadPolicy loads the Rego policy from ConfigMap and prepares it for evaluation.
// Looks for ConfigMap "signal-processing-policies" in namespace "kubernaut-system".
func (e *CustomLabelsExtractor) LoadPolicy(ctx context.Context) error {
	// Load ConfigMap
	cm := &corev1.ConfigMap{}
	if err := e.client.Get(ctx, client.ObjectKey{
		Namespace: "kubernaut-system",
		Name:      "signal-processing-policies",
	}, cm); err != nil {
		return fmt.Errorf("failed to load policy ConfigMap: %w", err)
	}

	// Get policy content
	policyContent, ok := cm.Data["labels.rego"]
	if !ok {
		return fmt.Errorf("labels.rego not found in ConfigMap")
	}

	// Prepare Rego query
	prepared, err := rego.New(
		rego.Query("data.signalprocessing.labels.labels"),
		rego.Module("policy.rego", policyContent),
	).PrepareForEval(ctx)

	if err != nil {
		return fmt.Errorf("failed to prepare Rego policy: %w", err)
	}

	e.preparedQuery = &prepared
	e.policyLoaded = true

	e.logger.Info("Rego policy loaded from ConfigMap", "policySize", len(policyContent))
	return nil
}

// Extract evaluates the Rego policy and returns validated CustomLabels.
// Applies security wrapper and validation limits per DD-WORKFLOW-001 v1.9.
func (e *CustomLabelsExtractor) Extract(ctx context.Context, input *LabelInput) (map[string][]string, error) {
	if !e.policyLoaded || e.preparedQuery == nil {
		return make(map[string][]string), fmt.Errorf("policy not loaded, call LoadPolicy first")
	}

	// Convert input to map for Rego
	inputMap := map[string]interface{}{
		"namespace": map[string]interface{}{
			"name":        input.Namespace.Name,
			"labels":      input.Namespace.Labels,
			"annotations": input.Namespace.Annotations,
		},
	}

	if input.Pod.Name != "" {
		inputMap["pod"] = map[string]interface{}{
			"name":        input.Pod.Name,
			"labels":      input.Pod.Labels,
			"annotations": input.Pod.Annotations,
		}
	}

	if input.Deployment.Name != "" {
		inputMap["deployment"] = map[string]interface{}{
			"name":        input.Deployment.Name,
			"replicas":    input.Deployment.Replicas,
			"labels":      input.Deployment.Labels,
			"annotations": input.Deployment.Annotations,
		}
	}

	if input.Node.Name != "" {
		inputMap["node"] = map[string]interface{}{
			"name":   input.Node.Name,
			"labels": input.Node.Labels,
		}
	}

	if input.Signal.Type != "" {
		inputMap["signal"] = map[string]interface{}{
			"type":     input.Signal.Type,
			"severity": input.Signal.Severity,
			"source":   input.Signal.Source,
		}
	}

	// Evaluate Rego query
	results, err := e.preparedQuery.Eval(ctx, rego.EvalInput(inputMap))
	if err != nil {
		e.logger.Error(err, "Rego evaluation failed")
		return make(map[string][]string), nil // Non-fatal, return empty map
	}

	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return make(map[string][]string), nil
	}

	// Convert result to map[string][]string
	rawResult := results[0].Expressions[0].Value
	converted, err := e.convertResult(rawResult)
	if err != nil {
		e.logger.Error(err, "Failed to convert Rego result")
		return make(map[string][]string), nil
	}

	// Apply security wrapper and validation
	validated := e.validateAndSanitize(converted)

	e.logger.V(1).Info("CustomLabels extracted",
		"rawKeys", len(converted),
		"validatedKeys", len(validated))

	return validated, nil
}

// convertResult converts Rego result to map[string][]string.
func (e *CustomLabelsExtractor) convertResult(rawResult interface{}) (map[string][]string, error) {
	result := make(map[string][]string)

	if rawResult == nil {
		return result, nil
	}

	rawMap, ok := rawResult.(map[string]interface{})
	if !ok {
		return result, fmt.Errorf("unexpected Rego result type: %T (expected map)", rawResult)
	}

	for key, value := range rawMap {
		switch v := value.(type) {
		case []interface{}:
			strSlice := make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok {
					strSlice = append(strSlice, s)
				}
			}
			result[key] = strSlice
		case []string:
			result[key] = v
		}
	}

	return result, nil
}

// validateAndSanitize enforces validation limits and security per DD-WORKFLOW-001 v1.9.
func (e *CustomLabelsExtractor) validateAndSanitize(labels map[string][]string) map[string][]string {
	result := make(map[string][]string)

	keyCount := 0
	for key, values := range labels {
		// Check key count limit
		if keyCount >= MaxKeys {
			e.logger.Info("CustomLabels key limit reached, truncating",
				"maxKeys", MaxKeys, "totalKeys", len(labels))
			break
		}

		// Skip system labels (security wrapper)
		if isSystemLabel(key) {
			e.logger.V(1).Info("CustomLabels system label stripped", "key", key)
			continue
		}

		// Skip reserved prefixes
		if hasReservedPrefix(key) {
			e.logger.V(1).Info("CustomLabels reserved prefix stripped", "key", key)
			continue
		}

		// Truncate key if too long
		validKey := key
		if len(key) > MaxKeyLength {
			e.logger.V(1).Info("CustomLabels key truncated",
				"key", key, "maxLength", MaxKeyLength)
			validKey = key[:MaxKeyLength]
		}

		// Validate and truncate values
		var validValues []string
		for i, value := range values {
			if i >= MaxValuesPerKey {
				e.logger.V(1).Info("CustomLabels values limit reached",
					"key", validKey, "maxValues", MaxValuesPerKey)
				break
			}
			validValue := value
			if len(value) > MaxValueLength {
				e.logger.V(1).Info("CustomLabels value truncated",
					"key", validKey, "maxLength", MaxValueLength)
				validValue = value[:MaxValueLength]
			}
			validValues = append(validValues, validValue)
		}

		if len(validValues) > 0 {
			result[validKey] = validValues
			keyCount++
		}
	}

	return result
}

// isSystemLabel checks if the key is a reserved system label.
func isSystemLabel(key string) bool {
	for _, sysLabel := range SystemLabels {
		if key == sysLabel {
			return true
		}
	}
	return false
}

// hasReservedPrefix checks if the key has a reserved prefix.
func hasReservedPrefix(key string) bool {
	for _, prefix := range ReservedPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

