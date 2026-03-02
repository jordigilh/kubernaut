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

// Package validation provides validation logic for workflow registration.
package validation

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// DependencyValidator validates that schema-declared infrastructure dependencies
// exist in the execution namespace with non-empty data.
// DD-WE-006: Used by Data Storage at registration time (Level 2 validation).
type DependencyValidator interface {
	ValidateDependencies(ctx context.Context, namespace string, deps *models.WorkflowDependencies) error
}

// K8sDependencyValidator implements DependencyValidator using a Kubernetes client.
// It checks that each declared Secret/ConfigMap exists in the target namespace
// and has non-empty .data.
type K8sDependencyValidator struct {
	client client.Reader
}

// NewK8sDependencyValidator creates a DependencyValidator backed by a Kubernetes client.
func NewK8sDependencyValidator(c client.Reader) *K8sDependencyValidator {
	return &K8sDependencyValidator{client: c}
}

// ValidateDependencies checks each declared Secret/ConfigMap exists with non-empty data.
// Returns an error describing the first missing or empty dependency found.
func (v *K8sDependencyValidator) ValidateDependencies(ctx context.Context, namespace string, deps *models.WorkflowDependencies) error {
	if deps == nil {
		return nil
	}

	for _, s := range deps.Secrets {
		var secret corev1.Secret
		key := client.ObjectKey{Name: s.Name, Namespace: namespace}
		if err := v.client.Get(ctx, key, &secret); err != nil {
			return fmt.Errorf("dependency Secret %q not found in namespace %q: %w", s.Name, namespace, err)
		}
		if len(secret.Data) == 0 {
			return fmt.Errorf("dependency Secret %q in namespace %q has empty data", s.Name, namespace)
		}
	}

	for _, cm := range deps.ConfigMaps {
		var configMap corev1.ConfigMap
		key := client.ObjectKey{Name: cm.Name, Namespace: namespace}
		if err := v.client.Get(ctx, key, &configMap); err != nil {
			return fmt.Errorf("dependency ConfigMap %q not found in namespace %q: %w", cm.Name, namespace, err)
		}
		if len(configMap.Data) == 0 && len(configMap.BinaryData) == 0 {
			return fmt.Errorf("dependency ConfigMap %q in namespace %q has empty data", cm.Name, namespace)
		}
	}

	return nil
}
