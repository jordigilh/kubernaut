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

import (
	"context"
	"fmt"
)

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
// as managed=false. DD-FLEET-015: only State and Provenance cross the wire;
// Reason and Retryable are bounded internal diagnostics, and there is no
// boolean "managed" member for consumers to bypass the typed state.
type ScopeResult struct {
	State      ScopeState  `json:"state"`
	Provenance ScopeSource `json:"provenance,omitempty"`
	Reason     string      `json:"-"`
	Retryable  bool        `json:"-"`
}

// Validate rejects unknown states and fabricated observation provenance.
func (r ScopeResult) Validate() error {
	switch r.State {
	case ScopeVerifiedManaged:
		if r.Provenance == ScopeSourceResourceLabel || r.Provenance == ScopeSourceNamespaceLabel {
			return nil
		}
	case ScopeVerifiedUnmanaged:
		if r.Provenance == ScopeSourceResourceLabel || r.Provenance == ScopeSourceNamespaceLabel || r.Provenance == ScopeSourceNone {
			return nil
		}
	case ScopeUnavailable, ScopeUnsupported, ScopeInvalid, ScopeUnauthorized:
		if r.Provenance == ScopeSourceUnavailable {
			return nil
		}
	}
	return fmt.Errorf("scope protocol: invalid state or provenance")
}

// TypedScopeChecker is an additive contract preserved from main (#2647) for
// consumers that reference the typed lane explicitly. ScopeChecker itself is
// the typed contract (DD-FLEET-015); the boolean lane is gone (#2660 m10).
type TypedScopeChecker interface {
	Check(ctx context.Context, resource ResourceIdentity) (ScopeResult, error)
}

// ResourceIdentity uniquely identifies a Kubernetes resource, optionally on a remote cluster.
// Replaces positional string parameters across all scope checking interfaces.
//
// ClusterID is empty for local/hub cluster resources.
// Version is required; Group is empty only for the core API group. Kind alone
// is not a unique Kubernetes identity.
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
	// Check returns a completed, validated domain outcome with nil error.
	// Execution/protocol errors always override any accompanying result.
	Check(ctx context.Context, resource ResourceIdentity) (ScopeResult, error)
}

// Compile-time interface compliance: Manager implements ScopeChecker.
var _ ScopeChecker = (*Manager)(nil)
var _ TypedScopeChecker = (*Manager)(nil)
