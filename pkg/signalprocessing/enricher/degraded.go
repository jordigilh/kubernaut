/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package enricher provides Kubernetes context enrichment for signal processing.
// This file implements degraded mode fallback per DD-4: K8s Enrichment Failure Handling.
package enricher

import (
	"fmt"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

const (
	// DegradedModeConfidence is the confidence score for degraded context
	DegradedModeConfidence = 0.5

	// MaxLabels is the maximum number of labels allowed in context
	MaxLabels = 100

	// MaxLabelValueLength is the maximum length of a label value (K8s limit is 63)
	MaxLabelValueLength = 63
)

// BuildDegradedContext creates a fallback KubernetesContext from signal labels
// when K8s API enrichment fails.
//
// DD-4: K8s Enrichment Failure Handling
// - Use signal labels as fallback context
// - Set DegradedMode to true
// - Set Confidence to 0.5
func BuildDegradedContext(signal *signalprocessingv1alpha1.SignalData) *signalprocessingv1alpha1.KubernetesContext {
	ctx := &signalprocessingv1alpha1.KubernetesContext{
		DegradedMode: true,
		Confidence:   DegradedModeConfidence,
	}

	// Use signal labels as namespace labels fallback
	if signal.Labels != nil {
		ctx.NamespaceLabels = make(map[string]string)
		for k, v := range signal.Labels {
			ctx.NamespaceLabels[k] = v
		}
	} else {
		ctx.NamespaceLabels = make(map[string]string)
	}

	// Use signal annotations as namespace annotations fallback
	if signal.Annotations != nil {
		ctx.NamespaceAnnotations = make(map[string]string)
		for k, v := range signal.Annotations {
			ctx.NamespaceAnnotations[k] = v
		}
	} else {
		ctx.NamespaceAnnotations = make(map[string]string)
	}

	return ctx
}

// ValidateContextSize validates that the KubernetesContext doesn't exceed size limits.
//
// Risk #6 mitigation: Add context size validation
// - Maximum 100 labels per map
// - Maximum 63 characters per label value (K8s limit)
func ValidateContextSize(ctx *signalprocessingv1alpha1.KubernetesContext) error {
	if ctx == nil {
		return nil
	}

	// Validate namespace labels
	if err := validateLabels(ctx.NamespaceLabels, "namespace labels"); err != nil {
		return err
	}

	// Validate namespace annotations (annotations can be longer, just check count)
	if len(ctx.NamespaceAnnotations) > MaxLabels {
		return fmt.Errorf("namespace annotations count %d exceeds maximum %d", len(ctx.NamespaceAnnotations), MaxLabels)
	}

	// NOTE: CustomLabels validation is handled by the Rego engine (DD-WORKFLOW-001 v1.9)
	// which enforces: max 10 keys, 5 values/key, 63 char keys, 100 char values
	// Skip redundant validation here.

	// Validate pod labels if present
	if ctx.Pod != nil {
		if err := validateLabels(ctx.Pod.Labels, "pod labels"); err != nil {
			return err
		}
	}

	// Validate deployment labels if present
	if ctx.Deployment != nil {
		if err := validateLabels(ctx.Deployment.Labels, "deployment labels"); err != nil {
			return err
		}
	}

	return nil
}

// validateLabels checks that a label map doesn't exceed size limits.
func validateLabels(labels map[string]string, name string) error {
	if labels == nil {
		return nil
	}

	if len(labels) > MaxLabels {
		return fmt.Errorf("%s count %d exceeds maximum %d", name, len(labels), MaxLabels)
	}

	for k, v := range labels {
		if len(v) > MaxLabelValueLength {
			return fmt.Errorf("%s value for key %q exceeds maximum length %d", name, k, MaxLabelValueLength)
		}
	}

	return nil
}

