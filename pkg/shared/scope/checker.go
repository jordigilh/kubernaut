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

// ScopeState is the typed outcome of a scope evaluation. Only the two
// verified states are authoritative business decisions; every other state is
// unavailable for authorization and must fail closed.
type ScopeState string

const (
	ScopeVerifiedManaged   ScopeState = "verified_managed"
	ScopeVerifiedUnmanaged ScopeState = "verified_unmanaged"
	ScopeUnavailable       ScopeState = "unavailable"
	ScopeUnsupported       ScopeState = "unsupported"
	ScopeInvalid           ScopeState = "invalid"
	ScopeUnauthorized      ScopeState = "unauthorized"
)

// ScopeResult carries a typed scope outcome across a boundary that otherwise
// cannot safely represent cache misses, source uncertainty, or invalid input
// as managed=false. Managed is non-nil only for the two verified states.
type ScopeResult struct {
	State      ScopeState  `json:"state"`
	Managed    *bool       `json:"managed"`
	Provenance ScopeSource `json:"provenance,omitempty"`
	Reason     string      `json:"reason,omitempty"`
	Retryable  bool        `json:"retryable,omitempty"`
}

// TypedScopeChecker is the additive typed contract used by FMC's internal
// presence-aware scope path. ScopeChecker remains the existing boolean
// compatibility seam until all GW/RO/AF callers are migrated.
type TypedScopeChecker interface {
	Check(ctx context.Context, resource ResourceIdentity) (ScopeResult, error)
}

// ResourceIdentity uniquely identifies a Kubernetes resource, optionally on a remote cluster.
// Replaces positional string parameters across all scope checking interfaces.
//
// ClusterID is empty for local/hub cluster resources.
// Group and Version are optional only for local compatibility callers. Remote
// checks must provide the explicit group/version because Kind alone is not a
// unique Kubernetes identity.
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
	// IsManagedResource returns an evaluated managed decision. A false result
	// with a nil error means the resource is verified as unmanaged; an error
	// means the scope decision could not be completed and must remain visible
	// to the caller.
	IsManagedResource(ctx context.Context, resource ResourceIdentity) (bool, error)
}

// Compile-time interface compliance: Manager implements ScopeChecker.
var _ ScopeChecker = (*Manager)(nil)
