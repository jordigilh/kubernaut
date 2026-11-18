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

package types

import (
	"context"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// OwnerResolver resolves the top-level controller owner of a Kubernetes resource.
//
// This interface enables adapters to resolve the owner chain
// (e.g., Pod -> ReplicaSet -> Deployment) for fingerprinting purposes.
// By fingerprinting at the owner level (e.g., Deployment), events from different
// pods of the same Deployment are correctly deduplicated.
//
// Business Requirement: BR-GATEWAY-004 (Cross-adapter deduplication consistency)
//
// Issue #228: Moved from adapters package to types package so that all adapters
// share the same interface and ResolveFingerprint can accept it directly.
type OwnerResolver interface {
	// ResolveTopLevelOwnerIdentity traverses the controller-owner chain and
	// returns the complete top-level identity, or an error (unknown GVK, RBAC,
	// timeout, not found or deleted). Callers drop the signal on error to avoid
	// unreliable child-level fingerprints. The implementation retains cached
	// metadata reads and direct-API trust-but-verify (#284).
	// DD-FLEET-014: return each resolved identity's own authoritative version;
	// never copy a child's version to an owner, including a same-kind owner.
	ResolveTopLevelOwnerIdentity(ctx context.Context, resource sharedtypes.OwnerChainEntry) (sharedtypes.OwnerChainEntry, error)
}
