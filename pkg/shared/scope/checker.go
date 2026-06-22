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

// ScopeChecker validates if a resource is within Kubernaut's management scope.
// A single method handles both local and remote clusters — the implementation
// routes internally based on ResourceIdentity.ClusterID.
//
// Production implementations:
//   - *scope.Manager: local K8s label checks (ADR-053)
//   - *fleet.FederatedScopeChecker: routes local/remote via factory (ADR-068)
//
// Both Gateway and Remediation Orchestrator inject this as a mandatory dependency.
//
// Business Requirements:
//
//	BR-SCOPE-001: Resource Scope Management (2-level hierarchy)
//	BR-SCOPE-002: Gateway Signal Filtering
//	BR-SCOPE-010: RO Scope Blocking
//	BR-INTEGRATION-065: Multi-cluster federation scope resolution
//
// Architecture:
//
//	ADR-053: Resource Scope Management Architecture
//	ADR-068: Federated Control Plane Interface
type ScopeChecker interface {
	IsManagedResource(ctx context.Context, resource ResourceIdentity) (bool, error)
}

// Compile-time interface compliance: Manager implements ScopeChecker.
var _ ScopeChecker = (*Manager)(nil)
