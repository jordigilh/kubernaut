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
	"strings"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/shared/k8s/ownerchain"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// OwnerResolverWithAPIVersion is the optional APIVersion-aware extension of
// OwnerResolver (#2592). Implementations that can preserve the top-level
// owner's apiVersion through the chain (e.g. ownerchain.K8sOwnerResolver)
// satisfy it structurally; ResolveFingerprintWithCluster prefers it so the
// resolved signal target carries a complete GVK. Resolvers that do not
// implement it leave the owner's APIVersion empty — the strict scope gate
// then rejects the Kind-only identity instead of guessing (fail-closed).
type OwnerResolverWithAPIVersion interface {
	ResolveTopLevelOwnerWithAPIVersion(ctx context.Context, namespace, kind, name string) (ownerchain.OwnerResolution, error)
}

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

	// #2592: prefer the APIVersion-aware resolution so the owner target keeps
	// its complete GVK. The plain method remains the fallback for resolvers
	// that cannot supply it; their targets carry no apiVersion and are
	// rejected by the strict identity gate rather than inferred.
	if avResolver, ok := resolver.(OwnerResolverWithAPIVersion); ok {
		resolution, err := avResolver.ResolveTopLevelOwnerWithAPIVersion(ctx, resource.Namespace, resource.Kind, resource.Name)
		if err != nil {
			logger.Error(err, "Owner resolution failed, dropping signal",
				"resource", resource.String())
			return "", ResourceIdentifier{}, fmt.Errorf("owner resolution failed for %s: %w", resource.String(), err)
		}
		if resolution.Kind == "" || resolution.Name == "" {
			logger.Info("Owner resolution returned empty, dropping signal",
				"resource", resource.String())
			return "", ResourceIdentifier{}, fmt.Errorf("owner resolution returned empty for %s", resource.String())
		}
		owner := seedOwnerAPIVersion(ResourceIdentifier{
			Namespace:  resource.Namespace,
			Kind:       resolution.Kind,
			Name:       resolution.Name,
			APIVersion: resolution.APIVersion,
		}, resource, logger)
		fp := CalculateClusterAwareFingerprint(clusterID, owner)
		logger.V(1).Info("Owner resolution succeeded",
			"resource", resource.String(), "owner", owner.String(), "fingerprint", fp[:12])
		return fp, owner, nil
	}

	// INVARIANT (#2650): this plain-resolver fallback is unreachable in
	// production today — the only production OwnerResolver,
	// ownerchain.K8sOwnerResolver, implements OwnerResolverWithAPIVersion, so
	// the branch above always wins. It exists for third-party/test resolvers
	// that only implement the plain interface; their targets carry no
	// apiVersion (empty group segment) and are rejected by the strict scope
	// gate rather than guessed (fail-closed).
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

// seedOwnerAPIVersion back-fills the resolved owner identity's apiVersion
// from the input signal resource when owner resolution could not supply one
// (#2650, M1). The gap: nil-registry test paths and registry misses for kinds
// unknown to discovery (new CRD before the 5-min refresh, transient discovery
// miss) make resolveGVK fall back to the static kindToGroup map, which misses
// grouped kinds — resolution.APIVersion comes back empty exactly for the
// cross-group resources the group dimension exists for.
//
// The owner's own apiVersion from OwnerReferences always wins (it is only
// empty here), so this seed only fires for standalone resources where the
// resolution IS the resource. When an owner WAS found yet carries no version
// (degenerate controllerRef-without-apiVersion — real Kubernetes populates
// it), the seed is logged so the attribution stays observable rather than
// silent.
func seedOwnerAPIVersion(owner, resource ResourceIdentifier, logger logr.Logger) ResourceIdentifier {
	if owner.APIVersion != "" || resource.APIVersion == "" {
		return owner
	}
	owner.APIVersion = resource.APIVersion
	if owner.Kind != resource.Kind || owner.Name != resource.Name {
		logger.Info("Owner resolution returned empty apiVersion; seeded group dimension from signal resource",
			"resource", resource.String(), "owner", owner.String())
	}
	return owner
}

// CalculateOwnerFingerprint generates a fingerprint based on the owner resource,
// without including the signal reason/identifier.
//
// Business Requirement: BR-GATEWAY-069 (Deduplication tracking)
//
// Fingerprint Algorithm:
//   - Format: SHA256("group:namespace:kind:name") — the group segment comes
//     from the resource's apiVersion (#2650, BR-GATEWAY-004)
//   - Deterministic: Same owner -> same fingerprint regardless of event reason
//   - Used internally by ResolveFingerprint and directly by tests
//
// Examples:
//   - Pod crash event: SHA256("apps:prod:Deployment:payment-api")
//   - OOM event from same deployment: SHA256("apps:prod:Deployment:payment-api") -- same!
func CalculateOwnerFingerprint(resource ResourceIdentifier) string {
	return CalculateClusterAwareFingerprint("", resource)
}

// ClusterLabelKey is the label key used by Thanos/Alertmanager to identify the source cluster.
// Prometheus federation (Thanos) adds this as an external label on all metrics.
const ClusterLabelKey = "cluster"

// CalculateClusterAwareFingerprint generates a fingerprint that includes the
// cluster dimension for multi-cluster federation (BR-INTEGRATION-065) and the
// API-group dimension for cross-group deduplication (BR-GATEWAY-004, #2650).
//
// When clusterID is empty, the result is identical to CalculateOwnerFingerprint
// modulo the group dimension (both helpers share this implementation).
// When clusterID is non-empty, it is prepended to produce a unique hash per cluster.
//
// Algorithm:
//   - clusterID == "": SHA256("group:namespace:kind:name")
//   - clusterID != "": SHA256("clusterID:group:namespace:kind:name") — cluster-aware
//
// The group segment comes from groupSegment(resource.APIVersion): an empty
// apiVersion yields the empty group (legitimate prometheus ambiguous-registry
// state, residual #2066 gap); the version within a group is deliberately
// excluded — apps/v1 and apps/v2 are the same dedup target (#2650 decision).
//
// NO migration / backward compatibility: clean installs only (maintainer
// decision, #2650). Fingerprints persisted before #2650 do not match.
func CalculateClusterAwareFingerprint(clusterID string, resource ResourceIdentifier) string {
	// Single input builder: the optional cluster dimension is prepended to
	// the always-present group:namespace:kind:name segments (#2650: the group
	// comes from the resource's apiVersion, never the version itself).
	segments := make([]string, 0, 5)
	if clusterID != "" {
		segments = append(segments, clusterID)
	}
	segments = append(segments,
		groupSegment(resource.APIVersion),
		resource.Namespace,
		resource.Kind,
		resource.Name,
	)

	hash := sha256.Sum256([]byte(strings.Join(segments, ":")))
	return fmt.Sprintf("%x", hash)
}

// groupSegment extracts the API group dimension of a fingerprint input from a
// Kubernetes apiVersion string (#2650).
//
//   - empty apiVersion -> empty segment: the prometheus ambiguous/zero-candidate
//     registry path legitimately reaches the fingerprint with no group
//     (DD-KA-006 "absence is not an error"; known residual #2650 gap)
//   - well-formed apiVersion -> its group (core "v1" -> empty group)
//   - malformed apiVersion -> the raw string, NOT the empty segment. Bare
//     schema.ParseGroupVersion silently accepts slash-less group names
//     ("apps" -> {Group:"", Version:"apps"}), which would collapse a real
//     group onto the empty-group segment. The canonical scope.ParseAPIVersion
//     rejects those; defense-in-depth keeps the raw value distinct.
//     Unreachable in production: every ingress path validates apiVersion
//     (AF: validate.APIVersion + scope.ApplyAPIVersion; k8s event adapter:
//     parse-time validation per #2592; prometheus: derived from registry GVRs),
//     so malformed input can only arrive via direct calls.
func groupSegment(apiVersion string) string {
	if apiVersion == "" {
		return ""
	}
	group, _, err := scope.ParseAPIVersion(apiVersion)
	if err != nil {
		return apiVersion
	}
	return group
}
