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

// Package builders provides test object builders for creating test fixtures.
// Reference: AA P2.2 Refactoring
package builders

import (
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// EnrichmentBuilder provides a fluent API for building EnrichmentResults test fixtures.
//
// ADR-056: DetectedLabels and OwnerChain removed from EnrichmentResults.
// DetectedLabels are now computed by HAPI post-RCA (see PostRCAContext).
// OwnerChain is resolved by HAPI via get_resource_context tool (ADR-055).
//
// Usage:
//
//	enrichment := builders.NewEnrichment().
//	    WithCustomLabel("constraint", "cost-constrained").
//	    Build()
type EnrichmentBuilder struct {
	enrichment *sharedtypes.EnrichmentResults
}

// NewEnrichment creates a new builder with safe default values.
func NewEnrichment() *EnrichmentBuilder {
	return &EnrichmentBuilder{
		enrichment: &sharedtypes.EnrichmentResults{
			KubernetesContext: nil,
			CustomLabels:      nil,
		},
	}
}

// ========================================
// KUBERNETES CONTEXT SETTERS
// ========================================

// WithKubernetesContext sets the full Kubernetes context.
func (b *EnrichmentBuilder) WithKubernetesContext(ctx *sharedtypes.KubernetesContext) *EnrichmentBuilder {
	b.enrichment.KubernetesContext = ctx
	return b
}

// WithNamespace sets the namespace in Kubernetes context.
func (b *EnrichmentBuilder) WithNamespace(namespace string, labels map[string]string) *EnrichmentBuilder {
	b.ensureKubernetesContext()
	b.enrichment.KubernetesContext.Namespace = namespace
	b.enrichment.KubernetesContext.NamespaceLabels = labels
	return b
}

// WithPodDetails sets pod details in Kubernetes context.
func (b *EnrichmentBuilder) WithPodDetails(pod *sharedtypes.PodDetails) *EnrichmentBuilder {
	b.ensureKubernetesContext()
	b.enrichment.KubernetesContext.PodDetails = pod
	return b
}

// WithDeploymentDetails sets deployment details in Kubernetes context.
func (b *EnrichmentBuilder) WithDeploymentDetails(deployment *sharedtypes.DeploymentDetails) *EnrichmentBuilder {
	b.ensureKubernetesContext()
	b.enrichment.KubernetesContext.DeploymentDetails = deployment
	return b
}

// WithNodeDetails sets node details in Kubernetes context.
func (b *EnrichmentBuilder) WithNodeDetails(node *sharedtypes.NodeDetails) *EnrichmentBuilder {
	b.ensureKubernetesContext()
	b.enrichment.KubernetesContext.NodeDetails = node
	return b
}

// ========================================
// CUSTOM LABELS SETTERS
// ========================================

// WithCustomLabel adds a custom label category with values.
func (b *EnrichmentBuilder) WithCustomLabel(category string, values ...string) *EnrichmentBuilder {
	if b.enrichment.CustomLabels == nil {
		b.enrichment.CustomLabels = make(map[string][]string)
	}
	b.enrichment.CustomLabels[category] = values
	return b
}

// WithCustomLabels sets all custom labels at once.
func (b *EnrichmentBuilder) WithCustomLabels(labels map[string][]string) *EnrichmentBuilder {
	b.enrichment.CustomLabels = labels
	return b
}

// ========================================
// HELPER METHODS
// ========================================

func (b *EnrichmentBuilder) ensureKubernetesContext() {
	if b.enrichment.KubernetesContext == nil {
		b.enrichment.KubernetesContext = &sharedtypes.KubernetesContext{}
	}
}

// ========================================
// BUILD
// ========================================

// Build returns the constructed EnrichmentResults.
func (b *EnrichmentBuilder) Build() sharedtypes.EnrichmentResults {
	return *b.enrichment
}
