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
func BuildDegradedContext(signal *signalprocessingv1alpha1.SignalData) *signalprocessingv1alpha1.KubernetesContext {
	ctx := &signalprocessingv1alpha1.KubernetesContext{
		DegradedMode: true,
		Namespace: &signalprocessingv1alpha1.NamespaceContext{
			Name:        signal.TargetResource.Namespace,
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
	}

	// Use signal labels as namespace labels fallback
	if signal.Labels != nil {
		for k, v := range signal.Labels {
			ctx.Namespace.Labels[k] = v
		}
	}

	// Use signal annotations as namespace annotations fallback
	if signal.Annotations != nil {
		for k, v := range signal.Annotations {
			ctx.Namespace.Annotations[k] = v
		}
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
	if ctx.Namespace != nil {
		if err := validateLabels(ctx.Namespace.Labels, "namespace labels"); err != nil {
			return err
		}
		// Validate namespace annotations (annotations can be longer, just check count)
		if len(ctx.Namespace.Annotations) > MaxLabels {
			return fmt.Errorf("namespace annotations count %d exceeds maximum %d", len(ctx.Namespace.Annotations), MaxLabels)
		}
	}

	// NOTE: CustomLabels validation is handled by the Rego engine (DD-WORKFLOW-001 v1.9)
	// which enforces: max 10 keys, 5 values/key, 63 char keys, 100 char values
	// Skip redundant validation here.

	// Validate workload labels and annotations if present (Issue #113: unified Workload field)
	if ctx.Workload != nil {
		if err := validateLabels(ctx.Workload.Labels, "workload labels"); err != nil {
			return err
		}
		if len(ctx.Workload.Annotations) > MaxLabels {
			return fmt.Errorf("workload annotations count %d exceeds maximum %d", len(ctx.Workload.Annotations), MaxLabels)
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
