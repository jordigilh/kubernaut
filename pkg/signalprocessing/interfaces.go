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

// Package signalprocessing provides core interfaces for SignalProcessing service components.
//
// Reference: CONTROLLER_REFACTORING_PATTERN_LIBRARY.md - Pattern 6 (Interface-Based Services)
//
// This file centralizes all service interfaces to provide:
// - Single source of truth for component contracts
// - Easier testability through interface mocking
// - Clear service registry pattern
// - Reduced coupling between components
//
// TODO: Migrate existing interfaces here (Phase 2 refactoring)
// - Move K8sEnricher interface from controller to here
// - Move EnvironmentClassifier interface from controller to here
// - Move PriorityAssigner interface from controller to here
// - Document interface contracts with business requirements
// - Update all imports to reference this file
//
// Estimated effort: 4-6 hours
// Expected benefits: Centralized interface definitions, improved discoverability
package signalprocessing

import (
	"context"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// EnrichmentService provides context enrichment for signals.
// TODO: Move existing implementations to implement this interface (Phase 2 refactoring)
type EnrichmentService interface {
	Enrich(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error)
}

// ClassificationService provides environment and priority classification.
// TODO: Move existing implementations to implement this interface (Phase 2 refactoring)
type ClassificationService interface {
	ClassifyEnvironment(ctx context.Context, k8sCtx *signalprocessingv1alpha1.KubernetesContext, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.EnvironmentClassification, error)
	AssignPriority(ctx context.Context, k8sCtx *signalprocessingv1alpha1.KubernetesContext, envClass *signalprocessingv1alpha1.EnvironmentClassification, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.PriorityAssignment, error)
}

// CategorizationService provides business categorization.
// TODO: Move existing implementations to implement this interface (Phase 2 refactoring)
type CategorizationService interface {
	Categorize(k8sCtx *signalprocessingv1alpha1.KubernetesContext, envClass *signalprocessingv1alpha1.EnvironmentClassification) (*signalprocessingv1alpha1.BusinessClassification, error)
}

