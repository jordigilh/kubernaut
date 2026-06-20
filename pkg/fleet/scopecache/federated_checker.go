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

package scopecache

import (
	"context"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// Compile-time interface compliance.
var _ scope.ScopeChecker = (*FederatedScopeChecker)(nil)
var _ scope.FederatedScopeChecker = (*FederatedScopeChecker)(nil)
var _ scope.UnifiedScopeChecker = (*FederatedScopeChecker)(nil)

// FederatedScopeChecker implements scope.ScopeChecker by routing checks:
//   - Empty clusterID (local hub): delegates to the local ScopeChecker (existing K8s-based)
//   - Non-empty clusterID (remote): delegates to the RemoteScopeResolver
//
// The RemoteScopeResolver is a pluggable interface. Two implementations exist:
//   - Valkey-backed Client (FMC syncs labels) — for environments without ACM
//   - ACM Search GraphQL — for ACM environments (avoids FMC + Valkey)
//
// This allows GW and RO to transparently handle both local and remote resources
// using the existing ScopeChecker interface (ADR-065, ADR-068).
type FederatedScopeChecker struct {
	local          scope.ScopeChecker
	remoteResolver RemoteScopeResolver
	logger         logr.Logger
}

// NewFederatedScopeChecker creates a federated checker that routes by cluster context.
// localChecker handles hub cluster checks, remoteResolver handles fleet checks via
// the pluggable RemoteScopeResolver interface.
func NewFederatedScopeChecker(localChecker scope.ScopeChecker, remoteResolver RemoteScopeResolver, logger logr.Logger) *FederatedScopeChecker {
	return &FederatedScopeChecker{
		local:          localChecker,
		remoteResolver: remoteResolver,
		logger:         logger.WithName("federated-scope"),
	}
}

// NewFederatedScopeCheckerFromAddr is a convenience factory that creates a federated checker
// backed by a Valkey RemoteScopeResolver at the given address. Reduces boilerplate in cmd/ wiring.
func NewFederatedScopeCheckerFromAddr(localChecker scope.ScopeChecker, valkeyAddr string, logger logr.Logger) *FederatedScopeChecker {
	reader := NewValkeyCacheReader(valkeyAddr)
	cacheClient := NewClient(reader)
	return NewFederatedScopeChecker(localChecker, cacheClient, logger)
}

// IsManaged implements scope.ScopeChecker. Since the standard interface does not
// include clusterID, this method always delegates to the local checker.
// For remote cluster checks, use IsManagedOnCluster directly.
func (f *FederatedScopeChecker) IsManaged(ctx context.Context, namespace, kind, name string) (bool, error) {
	return f.local.IsManaged(ctx, namespace, kind, name)
}

// IsManagedResource implements scope.UnifiedScopeChecker using ResourceIdentity.
// Routes by ClusterID: empty → local checker, non-empty → remote resolver.
func (f *FederatedScopeChecker) IsManagedResource(ctx context.Context, resource scope.ResourceIdentity) (bool, error) {
	if resource.ClusterID == "" {
		return f.local.IsManaged(ctx, resource.Namespace, resource.Kind, resource.Name)
	}

	managed, err := f.remoteResolver.IsManaged(ctx, resource.ClusterID, resource.Group, resource.Version, resource.Kind, resource.Namespace, resource.Name)
	if err != nil {
		f.logger.V(1).Info("remote scope resolver error, falling back to unmanaged",
			"cluster", resource.ClusterID, "namespace", resource.Namespace,
			"kind", resource.Kind, "name", resource.Name, "error", err)
		return false, nil
	}
	return managed, nil
}

// IsManagedOnCluster checks scope for a resource on a specific cluster.
// Empty clusterID routes to local checker; non-empty routes to RemoteScopeResolver.
// Deprecated: Use IsManagedResource with ResourceIdentity instead.
func (f *FederatedScopeChecker) IsManagedOnCluster(ctx context.Context, clusterID, namespace, kind, name string) (bool, error) {
	if clusterID == "" {
		return f.local.IsManaged(ctx, namespace, kind, name)
	}

	managed, err := f.remoteResolver.IsManaged(ctx, clusterID, "", "", kind, namespace, name)
	if err != nil {
		f.logger.V(1).Info("remote scope resolver error, falling back to unmanaged",
			"cluster", clusterID, "namespace", namespace, "kind", kind, "name", name, "error", err)
		return false, nil
	}
	return managed, nil
}
