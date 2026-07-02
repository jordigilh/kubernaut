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
// Returns:
//   - fingerprint: SHA256 hash string for deduplication
//   - resolvedResource: the top-level owner ResourceIdentifier (callers should use
//     this as the signal's Resource to ensure AI workflow matching uses the workload,
//     not intermediate routing abstractions like Service)
//   - err: non-nil when the signal must be dropped
//
// Algorithm:
//  1. If resolver is nil -> (CalculateClusterAwareFingerprint(clusterID, resource), resource, nil)
//  2. If resolver succeeds -> (CalculateClusterAwareFingerprint(clusterID, owner), owner, nil)
//  3. If resolver fails -> ("", zero, error) — caller must drop the signal
//  4. If resolver returns empty fields -> ("", zero, error)
//
// Returning an error when owner resolution fails (e.g., pod deleted after
// rollout restart) prevents creating RRs with unreliable pod-level fingerprints
// that would break deduplication. The stale alert will resolve naturally when
// Prometheus stops seeing the deleted pod's metrics.
func ResolveFingerprint(ctx context.Context, resolver OwnerResolver, resource ResourceIdentifier, logger logr.Logger) (string, ResourceIdentifier, error) {
	return ResolveFingerprintWithCluster(ctx, "", resolver, resource, logger)
}

// ResolveFingerprintWithCluster is the cluster-aware variant of ResolveFingerprint.
// It includes clusterID in the fingerprint hash for multi-cluster deduplication (BR-INTEGRATION-065).
// When clusterID is empty, behavior is identical to ResolveFingerprint.
func ResolveFingerprintWithCluster(ctx context.Context, clusterID string, resolver OwnerResolver, resource ResourceIdentifier, logger logr.Logger) (string, ResourceIdentifier, error) {
	if resolver == nil {
		return CalculateClusterAwareFingerprint(clusterID, resource), resource, nil
	}

	ownerKind, ownerName, err := resolver.ResolveTopLevelOwner(
		ctx, resource.Namespace, resource.Kind, resource.Name)
	if err != nil {
		logger.Error(err, "Owner resolution failed, dropping signal",
			"resource", resource.String())
		return "", ResourceIdentifier{}, fmt.Errorf("owner resolution failed for %s: %w", resource.String(), err)
	}

	if ownerKind == "" || ownerName == "" {
		logger.Info("Owner resolution returned empty, dropping signal",
			"resource", resource.String())
		return "", ResourceIdentifier{}, fmt.Errorf("owner resolution returned empty for %s", resource.String())
	}

	owner := ResourceIdentifier{
		Namespace: resource.Namespace,
		Kind:      ownerKind,
		Name:      ownerName,
	}
	fp := CalculateClusterAwareFingerprint(clusterID, owner)
	logger.V(1).Info("Owner resolution succeeded",
		"resource", resource.String(), "owner", owner.String(), "fingerprint", fp[:12])
	return fp, owner, nil
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
	return CalculateClusterAwareFingerprint("", resource)
}

// ClusterLabelKey is the label key used by Thanos/Alertmanager to identify the source cluster.
// Prometheus federation (Thanos) adds this as an external label on all metrics.
const ClusterLabelKey = "cluster"

// CalculateClusterAwareFingerprint generates a fingerprint that includes the cluster dimension
// for multi-cluster federation (BR-INTEGRATION-065).
//
// When clusterID is empty, the result is identical to CalculateOwnerFingerprint (backward compatible).
// When clusterID is non-empty, it is prepended to produce a unique hash per cluster.
//
// Algorithm:
//   - clusterID == "": SHA256("namespace:kind:name") — same as legacy
//   - clusterID != "": SHA256("clusterID:namespace:kind:name") — cluster-aware
func CalculateClusterAwareFingerprint(clusterID string, resource ResourceIdentifier) string {
	var input string
	if clusterID == "" {
		input = fmt.Sprintf("%s:%s:%s",
			resource.Namespace,
			resource.Kind,
			resource.Name,
		)
	} else {
		input = fmt.Sprintf("%s:%s:%s:%s",
			clusterID,
			resource.Namespace,
			resource.Kind,
			resource.Name,
		)
	}

	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash)
}
