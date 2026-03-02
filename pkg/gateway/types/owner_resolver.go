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

import "context"

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
	// ResolveTopLevelOwner traverses the ownerReference chain to find the top-level
	// controller (e.g., Deployment, StatefulSet, DaemonSet).
	//
	// Parameters:
	// - ctx: Context for cancellation/timeout
	// - namespace: Resource namespace
	// - kind: Resource kind (e.g., "Pod")
	// - name: Resource name (e.g., "payment-api-789abc")
	//
	// Returns:
	// - ownerKind: Top-level owner kind (e.g., "Deployment")
	// - ownerName: Top-level owner name (e.g., "payment-api")
	// - err: Resolution error (RBAC, timeout, not found). Callers should fall back
	//   to resource-level fingerprinting on error.
	ResolveTopLevelOwner(ctx context.Context, namespace, kind, name string) (ownerKind string, ownerName string, err error)
}
