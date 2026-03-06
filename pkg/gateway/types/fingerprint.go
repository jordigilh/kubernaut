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

	"github.com/go-logr/logr"
)

// ResolveFingerprint is the single entry point for fingerprint generation across
// all gateway adapters. It encapsulates the owner resolution logic that
// was previously duplicated in KubernetesEventAdapter and PrometheusAdapter.
//
// Issue #228: Shared function ensures cross-adapter deduplication consistency.
//
// Business Requirement: BR-GATEWAY-004 (Cross-adapter deduplication)
//
// Algorithm:
//  1. If resolver is nil -> CalculateOwnerFingerprint(resource), nil
//  2. If resolver succeeds (non-empty ownerKind and ownerName) -> CalculateOwnerFingerprint(owner), nil
//  3. If resolver fails -> "", error (caller must drop the signal)
//  4. If resolver returns empty fields -> "", error
//
// Returning an error when owner resolution fails (e.g., pod deleted after
// rollout restart) prevents creating RRs with unreliable pod-level fingerprints
// that would break deduplication. The stale alert will resolve naturally when
// Prometheus stops seeing the deleted pod's metrics.
func ResolveFingerprint(ctx context.Context, resolver OwnerResolver, resource ResourceIdentifier, logger logr.Logger) (string, error) {
	if resolver == nil {
		return CalculateOwnerFingerprint(resource), nil
	}

	ownerKind, ownerName, err := resolver.ResolveTopLevelOwner(
		ctx, resource.Namespace, resource.Kind, resource.Name)
	if err != nil {
		logger.Error(err, "Owner resolution failed, dropping signal",
			"resource", resource.String())
		return "", fmt.Errorf("owner resolution failed for %s: %w", resource.String(), err)
	}

	if ownerKind == "" || ownerName == "" {
		logger.Info("Owner resolution returned empty, dropping signal",
			"resource", resource.String())
		return "", fmt.Errorf("owner resolution returned empty for %s", resource.String())
	}

	owner := ResourceIdentifier{
		Namespace: resource.Namespace,
		Kind:      ownerKind,
		Name:      ownerName,
	}
	fp := CalculateOwnerFingerprint(owner)
	logger.V(1).Info("Owner resolution succeeded",
		"resource", resource.String(), "owner", owner.String(), "fingerprint", fp[:12])
	return fp, nil
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
