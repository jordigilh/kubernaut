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

// ========================================
// ENRICHMENT RESULTS BUILDER (AA P2.2)
// ========================================
//
// EnrichmentBuilder provides a fluent API for building EnrichmentResults test fixtures.
//
// WHY Test Builders?
// - ✅ Reduce test boilerplate (DRY principle)
// - ✅ Fluent API improves test readability
// - ✅ Default values eliminate repetitive setup
// - ✅ Easy to extend with new fields
// - ✅ Type-safe construction
//
// Usage:
//   enrichment := builders.NewEnrichment().
//       WithGitOpsManaged(true, "argocd").
//       WithPDBProtected(true).
//       WithOwnerChain("default", "Deployment", "my-app").
//       Build()
//
// Reference: AA P2.2 Refactoring

// EnrichmentBuilder builds EnrichmentResults objects for tests.
type EnrichmentBuilder struct {
	enrichment *sharedtypes.EnrichmentResults
}

// NewEnrichment creates a new builder with safe default values.
func NewEnrichment() *EnrichmentBuilder {
	return &EnrichmentBuilder{
		enrichment: &sharedtypes.EnrichmentResults{
			// Safe defaults: minimal DetectedLabels (all false, no failures)
			DetectedLabels: &sharedtypes.DetectedLabels{
				GitOpsManaged:   false,
				PDBProtected:    false,
				HPAEnabled:      false,
				Stateful:        false,
				HelmManaged:     false,
				NetworkIsolated: false,
			},
			// Optional fields nil by default
			KubernetesContext: nil,
			OwnerChain:        nil,
			CustomLabels:      nil,
		},
	}
}

// ========================================
// DETECTED LABELS SETTERS
// ========================================

// WithGitOpsManaged sets GitOps detection.
func (b *EnrichmentBuilder) WithGitOpsManaged(managed bool, tool string) *EnrichmentBuilder {
	b.ensureDetectedLabels()
	b.enrichment.DetectedLabels.GitOpsManaged = managed
	b.enrichment.DetectedLabels.GitOpsTool = tool
	return b
}

// WithPDBProtected sets PDB detection.
func (b *EnrichmentBuilder) WithPDBProtected(protected bool) *EnrichmentBuilder {
	b.ensureDetectedLabels()
	b.enrichment.DetectedLabels.PDBProtected = protected
	return b
}

// WithHPAEnabled sets HPA detection.
func (b *EnrichmentBuilder) WithHPAEnabled(enabled bool) *EnrichmentBuilder {
	b.ensureDetectedLabels()
	b.enrichment.DetectedLabels.HPAEnabled = enabled
	return b
}

// WithStateful sets stateful workload detection.
func (b *EnrichmentBuilder) WithStateful(stateful bool) *EnrichmentBuilder {
	b.ensureDetectedLabels()
	b.enrichment.DetectedLabels.Stateful = stateful
	return b
}

// WithHelmManaged sets Helm detection.
func (b *EnrichmentBuilder) WithHelmManaged(managed bool) *EnrichmentBuilder {
	b.ensureDetectedLabels()
	b.enrichment.DetectedLabels.HelmManaged = managed
	return b
}

// WithNetworkIsolated sets network isolation detection.
func (b *EnrichmentBuilder) WithNetworkIsolated(isolated bool) *EnrichmentBuilder {
	b.ensureDetectedLabels()
	b.enrichment.DetectedLabels.NetworkIsolated = isolated
	return b
}

// WithServiceMesh sets service mesh detection.
func (b *EnrichmentBuilder) WithServiceMesh(mesh string) *EnrichmentBuilder {
	b.ensureDetectedLabels()
	b.enrichment.DetectedLabels.ServiceMesh = mesh
	return b
}

// WithFailedDetections sets fields where detection failed (RBAC, timeout, etc.).
func (b *EnrichmentBuilder) WithFailedDetections(fields ...string) *EnrichmentBuilder {
	b.ensureDetectedLabels()
	b.enrichment.DetectedLabels.FailedDetections = fields
	return b
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
// OWNER CHAIN SETTERS
// ========================================

// WithOwnerChain appends an entry to the owner chain.
func (b *EnrichmentBuilder) WithOwnerChain(namespace, kind, name string) *EnrichmentBuilder {
	if b.enrichment.OwnerChain == nil {
		b.enrichment.OwnerChain = []sharedtypes.OwnerChainEntry{}
	}
	b.enrichment.OwnerChain = append(b.enrichment.OwnerChain, sharedtypes.OwnerChainEntry{
		Namespace: namespace,
		Kind:      kind,
		Name:      name,
	})
	return b
}

// WithOwnerChainEntries sets the complete owner chain.
func (b *EnrichmentBuilder) WithOwnerChainEntries(entries []sharedtypes.OwnerChainEntry) *EnrichmentBuilder {
	b.enrichment.OwnerChain = entries
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
// CONVENIENCE PRESETS
// ========================================

// WithProductionWorkload configures a typical production workload enrichment.
func (b *EnrichmentBuilder) WithProductionWorkload() *EnrichmentBuilder {
	return b.
		WithGitOpsManaged(true, "argocd").
		WithPDBProtected(true).
		WithHPAEnabled(true).
		WithNetworkIsolated(true).
		WithServiceMesh("istio")
}

// WithStagingWorkload configures a typical staging workload enrichment.
func (b *EnrichmentBuilder) WithStagingWorkload() *EnrichmentBuilder {
	return b.
		WithGitOpsManaged(true, "flux").
		WithPDBProtected(false).
		WithHPAEnabled(true).
		WithNetworkIsolated(false)
}

// WithDevelopmentWorkload configures a typical development workload enrichment.
func (b *EnrichmentBuilder) WithDevelopmentWorkload() *EnrichmentBuilder {
	return b.
		WithGitOpsManaged(false, "").
		WithPDBProtected(false).
		WithHPAEnabled(false).
		WithNetworkIsolated(false)
}

// WithStatefulWorkload configures a typical stateful workload enrichment.
func (b *EnrichmentBuilder) WithStatefulWorkload() *EnrichmentBuilder {
	return b.
		WithStateful(true).
		WithPDBProtected(true).
		WithHelmManaged(true)
}

// ========================================
// HELPER METHODS
// ========================================

func (b *EnrichmentBuilder) ensureDetectedLabels() {
	if b.enrichment.DetectedLabels == nil {
		b.enrichment.DetectedLabels = &sharedtypes.DetectedLabels{}
	}
}

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

