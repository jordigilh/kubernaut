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
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
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

	// #2592/#2593 (DD-FLEET-014): resolution is GVK-carrying end to end. The
	// input resource's APIVersion — determined from discovery at ingestion
	// (#2593) — flows into the traversal, each owner keeps its own
	// authoritative version, and the production resolver FAILS (error) on a
	// still-empty source GVK rather than returning a Kind-only identity.
	resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, sharedtypes.OwnerChainEntry{
		Namespace: resource.Namespace, Kind: resource.Kind, Name: resource.Name, APIVersion: resource.APIVersion,
	})
	if err != nil {
		logger.Error(err, "Owner resolution failed, dropping signal",
			"resource", resource.String())
		return "", ResourceIdentifier{}, fmt.Errorf("owner resolution failed for %s: %w", resource.String(), err)
	}

	if resolved.Kind == "" || resolved.Name == "" {
		logger.Info("Owner resolution returned empty, dropping signal",
			"resource", resource.String())
		return "", ResourceIdentifier{}, fmt.Errorf("owner resolution returned empty for %s", resource.String())
	}

	// #2650 M1 × #2593 interaction: the production resolver
	// (ownerchain.K8sOwnerResolver.ResolveTopLevelOwnerIdentity) already
	// back-fills the source GVK from its discovery registry and errors on a
	// versionless identity, so for it the seed below is redundant defense in
	// depth. The seed remains the gateway-level fail-closed safety net for
	// OwnerResolver implementations (third-party, test fakes) that return a
	// standalone identity with an empty APIVersion without erroring: the
	// input resource's apiVersion — discovery-determined per #2593 when the
	// adapter could not supply one — seeds the identity ONLY when the
	// resolved owner IS the resource; a different versionless owner stays
	// empty and is rejected by the strict identity gate downstream.
	owner := seedOwnerAPIVersion(ResourceIdentifier{
		Namespace:  resolved.Namespace,
		Kind:       resolved.Kind,
		Name:       resolved.Name,
		APIVersion: resolved.APIVersion,
	}, resource, logger)
	fp := CalculateClusterAwareFingerprint(clusterID, owner)
	logger.V(1).Info("Owner resolution succeeded",
		"resource", resource.String(), "owner", owner.String(), "fingerprint", fp[:12])
	return fp, owner, nil
}

// seedOwnerAPIVersion back-fills the resolved owner identity's apiVersion
// from the input signal resource when owner resolution could not supply one
// (#2650, M1). Original gap: resolvers whose GVK lookup missed a grouped kind
// (registry miss, nil registry) returned the standalone resource's identity
// with an empty apiVersion — exactly for the cross-group resources the group
// dimension exists for. Post-#2593, the production resolver determines the
// source GVK from discovery itself and fails closed on a versionless source
// (DD-FLEET-014), so this seed is defense in depth for resolvers that still
// return a versionless standalone identity without erroring.
//
// The seed is valid ONLY when the resolved owner IS the input resource
// (standalone resource: same kind, name and namespace) — the resource's own
// apiVersion is then trivially its owner's. When resolution found a DIFFERENT
// owner that carries no apiVersion (degenerate controllerRef — real Kubernetes
// populates it), the child's group must NOT be attributed to the owner: the
// owner keeps its empty version, which the #2592 strict identity gate rejects
// downstream (fail-closed), and the mismatch is logged so the attribution
// decision stays observable.
func seedOwnerAPIVersion(owner, resource ResourceIdentifier, logger logr.Logger) ResourceIdentifier {
	if owner.APIVersion != "" || resource.APIVersion == "" {
		return owner
	}
	if owner.Kind != resource.Kind || owner.Name != resource.Name || owner.Namespace != resource.Namespace {
		logger.Info("Owner resolution returned a different owner with empty apiVersion; refusing to seed the group from the child resource (fail-closed)",
			"resource", resource.String(), "owner", owner.String())
		return owner
	}
	owner.APIVersion = resource.APIVersion
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
