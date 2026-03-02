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
	"crypto/sha256"
	"fmt"
)

// ResolveFingerprint is the single entry point for fingerprint generation across
// all gateway adapters. It encapsulates the owner resolution + fallback logic that
// was previously duplicated in KubernetesEventAdapter and PrometheusAdapter.
//
// Issue #228: Shared function ensures cross-adapter deduplication consistency.
//
// Business Requirement: BR-GATEWAY-004 (Cross-adapter deduplication)
//
// Algorithm:
//  1. If resolver is nil -> CalculateOwnerFingerprint(resource)
//  2. If resolver succeeds (non-empty ownerKind and ownerName) -> CalculateOwnerFingerprint(owner)
//  3. If resolver fails or returns empty fields -> CalculateOwnerFingerprint(resource)
func ResolveFingerprint(ctx context.Context, resolver OwnerResolver, resource ResourceIdentifier) string {
	if resolver == nil {
		return CalculateOwnerFingerprint(resource)
	}

	ownerKind, ownerName, err := resolver.ResolveTopLevelOwner(
		ctx, resource.Namespace, resource.Kind, resource.Name)
	if err == nil && ownerKind != "" && ownerName != "" {
		return CalculateOwnerFingerprint(ResourceIdentifier{
			Namespace: resource.Namespace,
			Kind:      ownerKind,
			Name:      ownerName,
		})
	}

	return CalculateOwnerFingerprint(resource)
}

// CalculateOwnerFingerprint generates a fingerprint based on the owner resource,
// without including the signal reason/identifier.
//
// Business Requirement: BR-GATEWAY-069 (Deduplication tracking)
//
// Fingerprint Algorithm:
//   - Format: SHA256(namespace:kind:name)
//   - Deterministic: Same owner -> same fingerprint regardless of event reason
//   - Used internally by ResolveFingerprint and directly by tests
//
// Examples:
//   - Pod crash event: SHA256("prod:Deployment:payment-api")
//   - OOM event from same deployment: SHA256("prod:Deployment:payment-api") -- same!
func CalculateOwnerFingerprint(resource ResourceIdentifier) string {
	input := fmt.Sprintf("%s:%s:%s",
		resource.Namespace,
		resource.Kind,
		resource.Name,
	)

	hash := sha256.Sum256([]byte(input))

	return fmt.Sprintf("%x", hash)
}
