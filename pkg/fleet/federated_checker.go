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
	"fmt"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// Compile-time interface compliance.
var _ scope.ScopeChecker = (*FederatedScopeChecker)(nil)
var _ scope.TypedScopeChecker = (*FederatedScopeChecker)(nil)

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

// FederatedScopeChecker implements scope.ScopeChecker for attributed Fleet checks:
//   - Empty ClusterID: invalid; never assumes local scope
//   - Non-empty ClusterID (including hub): hierarchical remote check:
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
// The non-Fleet factory returns the local checker without this wrapper.
//
// References: ADR-068, BR-INTEGRATION-065
type FederatedScopeChecker struct {
	remote        scope.ScopeChecker
	clusterLookup ClusterLookup
	logger        logr.Logger
}

// NewFederatedScopeChecker creates an attributed Fleet checker.
// remoteChecker handles every explicit cluster ID (backend-specific adapter).
// Options can add a ClusterLookup for cluster-level precondition checking.
//
// There is deliberately no local checker parameter: DD-FLEET-015 forbids an
// empty or unknown Fleet attribution from falling back to local scope, and a
// stored-but-unused local checker implied exactly that fallback.
func NewFederatedScopeChecker(remoteChecker scope.ScopeChecker, logger logr.Logger, opts ...FederatedScopeCheckerOption) *FederatedScopeChecker {
	f := &FederatedScopeChecker{
		remote: remoteChecker,
		logger: logger.WithName("federated-scope"),
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// Remote returns the remote (backend-specific) scope.ScopeChecker used for
// non-empty ClusterID checks. Exposed so callers (e.g. the readiness gate,
// #1553) can probe the remote backend's reachability via a type assertion
// to readiness.Pinger without FederatedScopeChecker depending on that
// package.
func (f *FederatedScopeChecker) Remote() scope.ScopeChecker {
	return f.remote
}

// IsManagedResource is the boolean compatibility lane kept from main
// (#2647). It routes by ClusterID through the typed lane and collapses the
// outcome: only a verified managed=true returns true; every other outcome
// (verified unmanaged, invalid, unsupported, unavailable) fails closed as
// false with the distinguishing error where one exists.
func (f *FederatedScopeChecker) IsManagedResource(ctx context.Context, resource scope.ResourceIdentity) (bool, error) {
	result, err := f.Check(ctx, resource)
	if err != nil {
		return false, err
	}
	if result.State == scope.ScopeVerifiedManaged {
		return true, nil
	}
	if result.State == scope.ScopeVerifiedUnmanaged {
		return false, nil
	}
	if result.Reason != "" {
		return false, fmt.Errorf("fleet scope: check did not complete: %s", result.Reason)
	}
	return false, fmt.Errorf("fleet scope: check did not complete: %s", result.State)
}

// Check dispatches only explicitly attributed Fleet requests to the statically
// typed remote backend. Non-Fleet requests use the local checker at the factory.
// DD-FLEET-015 forbids an empty Fleet attribution from falling back locally.
func (f *FederatedScopeChecker) Check(ctx context.Context, resource scope.ResourceIdentity) (scope.ScopeResult, error) {
	if err := ctx.Err(); err != nil {
		return unavailableScopeResult("scope_canceled"), err
	}
	if resource.ClusterID == "" {
		return scope.ScopeResult{State: scope.ScopeInvalid, Provenance: scope.ScopeSourceUnavailable, Reason: "missing_cluster_attribution"}, nil
	}

	if _, err := scope.GVKForIdentity(resource); err != nil {
		return scope.ScopeResult{ //nolint:nilerr // DD-FLEET-015: invalid GVK is a completed domain outcome
			State:      scope.ScopeInvalid,
			Provenance: scope.ScopeSourceUnavailable,
			Reason:     "invalid_remote_gvk",
		}, nil
	}
	if f.clusterLookup != nil && !f.clusterLookup.IsKnownCluster(resource.ClusterID) {
		return unavailableScopeResult("cluster_unknown"), nil
	}
	result, err := f.remote.Check(ctx, resource)
	if ctx.Err() != nil {
		return unavailableScopeResult("scope_canceled"), ctx.Err()
	}
	if err != nil {
		return unavailableScopeResult("remote_scope_failed"), err
	}
	if err := result.Validate(); err != nil {
		return unavailableScopeResult("remote_protocol_error"), fmt.Errorf("fleet scope: %w", err)
	}
	return result, nil
}

func unavailableScopeResult(reason string) scope.ScopeResult {
	return scope.ScopeResult{State: scope.ScopeUnavailable, Provenance: scope.ScopeSourceUnavailable, Reason: reason, Retryable: true}
}

