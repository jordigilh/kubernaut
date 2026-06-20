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

package scope

import "context"

// ScopeChecker validates if a resource is within Kubernaut's management scope.
//
// This interface abstracts the scope validation logic so that:
//   - Production uses *scope.Manager (backed by metadata-only cache per ADR-053)
//   - Unit/integration tests use a mock implementation via dependency injection
//
// Both Gateway and Remediation Orchestrator inject this as a mandatory dependency.
// The pattern follows the same DI approach as processing.RetryObserver.
//
// Business Requirements:
//
//	BR-SCOPE-001: Resource Scope Management (2-level hierarchy)
//	BR-SCOPE-002: Gateway Signal Filtering
//	BR-SCOPE-010: RO Scope Blocking
//
// Architecture:
//
//	ADR-053: Resource Scope Management Architecture
type ScopeChecker interface {
	// IsManaged checks whether a Kubernetes resource is managed by Kubernaut.
	// Returns (true, nil) if managed, (false, nil) if unmanaged, or (false, error) on failure.
	//
	// The 2-level hierarchy (ADR-053):
	//  1. Resource label: kubernaut.ai/managed=true → managed
	//  2. Namespace label: kubernaut.ai/managed=true → managed
	//  3. Default: unmanaged (safe default)
	IsManaged(ctx context.Context, namespace, kind, name string) (bool, error)
}

// FederatedScopeChecker extends ScopeChecker with cluster-aware scope checking.
// Services that need to validate scope across both local and remote clusters
// accept this interface (e.g., Gateway, RO).
//
// The standard IsManaged method handles local (hub) cluster checks.
// IsManagedOnCluster routes by clusterID: empty → local, non-empty → remote.
//
// References: ADR-065, ADR-068, BR-INTEGRATION-065
type FederatedScopeChecker interface {
	ScopeChecker
	IsManagedOnCluster(ctx context.Context, clusterID, namespace, kind, name string) (bool, error)
}

// ResourceIdentity uniquely identifies a Kubernetes resource, optionally on a remote cluster.
// Replaces positional string parameters across all scope checking interfaces.
//
// ClusterID is empty for local/hub cluster resources.
// Group and Version are optional — when empty, the implementation infers them from Kind
// (matching existing scope.Manager behavior with the static kindToGroup map).
//
// References: ADR-068 (Federated Control Plane Interface), SI-10 (Input Validation)
type ResourceIdentity struct {
	ClusterID string // empty for local/hub cluster
	Group     string // API group (e.g., "apps", "" for core)
	Version   string // API version (e.g., "v1")
	Kind      string // e.g., "Deployment"
	Namespace string // empty for cluster-scoped resources
	Name      string
}

// UnifiedScopeChecker validates if a resource is within Kubernaut's management scope.
// A single method handles both local and remote clusters — the implementation
// routes internally based on ResourceIdentity.ClusterID.
//
// This interface supersedes ScopeChecker (3-string) and FederatedScopeChecker (5-string).
// During migration, both old and new interfaces coexist. Once all callers are migrated,
// UnifiedScopeChecker will be renamed to ScopeChecker and the old interfaces removed.
//
// References: ADR-068, BR-SCOPE-001, BR-SCOPE-002, BR-INTEGRATION-065
type UnifiedScopeChecker interface {
	IsManagedResource(ctx context.Context, resource ResourceIdentity) (bool, error)
}

// Compile-time interface compliance: Manager implements ScopeChecker.
var _ ScopeChecker = (*Manager)(nil)

// Compile-time interface compliance: Manager implements UnifiedScopeChecker.
var _ UnifiedScopeChecker = (*Manager)(nil)
