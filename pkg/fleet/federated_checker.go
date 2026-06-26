/*
Copyright 2026 Jordi Gil.

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

package fleet

import (
	"context"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// Compile-time interface compliance.
var _ scope.ScopeChecker = (*FederatedScopeChecker)(nil)

// ClusterLookup checks whether a cluster is known to Kubernaut's fleet registry.
// Implemented by EAIGWRegistry/KuadrantRegistry via the adapter below.
type ClusterLookup interface {
	IsKnownCluster(clusterID string) bool
}

// FederatedScopeCheckerOption configures optional FederatedScopeChecker behavior.
type FederatedScopeCheckerOption func(*FederatedScopeChecker)

// WithClusterLookup adds a cluster-level precondition. When set, remote scope
// checks first verify the cluster is known before proceeding to resource checks.
// This implements the 3-level hierarchy: Cluster -> Resource -> Namespace.
func WithClusterLookup(lookup ClusterLookup) FederatedScopeCheckerOption {
	return func(f *FederatedScopeChecker) {
		f.clusterLookup = lookup
	}
}

// FederatedScopeChecker implements scope.ScopeChecker by routing checks:
//   - Empty ClusterID (local hub): delegates to the local ScopeChecker (always scope.Manager)
//   - Non-empty ClusterID (remote): hierarchical check:
//     1. Cluster managed? (via ClusterLookup, if configured)
//     2. Resource managed? (via remote ScopeChecker backend adapter)
//
// The remote backend is a pluggable scope.ScopeChecker. Which implementation is used
// depends on the environment — the factory selects it from FleetConfig.Backend:
//   - FMC HTTP client (GitOps environments without a federated control plane)
//   - ACM Search client (ACM environments)
//   - Rancher client (Rancher environments)
//
// FederatedScopeChecker has no knowledge of Valkey, HTTP, or any specific backend.
// It is purely a local/remote router with an optional cluster-level gate.
//
// References: ADR-068, BR-INTEGRATION-065
type FederatedScopeChecker struct {
	local         scope.ScopeChecker
	remote        scope.ScopeChecker
	clusterLookup ClusterLookup
	logger        logr.Logger
}

// NewFederatedScopeChecker creates a federated checker that routes by ClusterID.
// localChecker handles hub cluster checks (always scope.Manager).
// remoteChecker handles remote cluster checks (backend-specific adapter).
// Options can add a ClusterLookup for cluster-level precondition checking.
func NewFederatedScopeChecker(localChecker scope.ScopeChecker, remoteChecker scope.ScopeChecker, logger logr.Logger, opts ...FederatedScopeCheckerOption) *FederatedScopeChecker {
	f := &FederatedScopeChecker{
		local:  localChecker,
		remote: remoteChecker,
		logger: logger.WithName("federated-scope"),
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// IsManagedResource implements scope.ScopeChecker.
// Routes by ClusterID: empty -> local checker, non-empty -> hierarchical remote check.
//
// For remote clusters the check is hierarchical (AC-3):
//  1. Cluster check: is the cluster known? (via ClusterLookup, if configured)
//  2. Resource check: is the resource managed? (via remote backend)
func (f *FederatedScopeChecker) IsManagedResource(ctx context.Context, resource scope.ResourceIdentity) (bool, error) {
	if resource.ClusterID == "" {
		return f.local.IsManagedResource(ctx, resource)
	}

	if f.clusterLookup != nil && !f.clusterLookup.IsKnownCluster(resource.ClusterID) {
		f.logger.V(1).Info("cluster not in registry, resource not managed",
			"cluster", resource.ClusterID)
		return false, nil
	}

	managed, err := f.remote.IsManagedResource(ctx, resource)
	if err != nil {
		f.logger.V(1).Info("remote scope check error, falling back to unmanaged",
			"cluster", resource.ClusterID, "namespace", resource.Namespace,
			"kind", resource.Kind, "name", resource.Name, "error", err)
		return false, nil
	}
	return managed, nil
}
