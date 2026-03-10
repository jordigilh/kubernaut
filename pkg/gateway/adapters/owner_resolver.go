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

package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MaxOwnerChainDepth limits traversal to prevent infinite loops (circular refs).
// Same constant as pkg/signalprocessing/ownerchain/builder.go:MaxOwnerChainDepth.
const MaxOwnerChainDepth = 5

// ownerLookupTimeout prevents indefinite blocking if the informer cache
// cannot sync for a resource kind (e.g., missing RBAC for list/watch).
// Same pattern as pkg/shared/scope/manager.go:scopeLookupTimeout.
const ownerLookupTimeout = 5 * time.Second

// kindToGroup maps Kubernetes resource kinds to their API groups.
// Same mapping as pkg/shared/scope/manager.go:kindToGroup.
// Shared pattern across Gateway scope management and owner chain resolution.
var kindToGroup = map[string]string{
	"Pod":              "",
	"Node":             "",
	"Service":          "",
	"ConfigMap":        "",
	"Secret":           "",
	"Namespace":        "",
	"PersistentVolume": "",
	"ReplicaSet":       "apps",
	"Deployment":       "apps",
	"StatefulSet":      "apps",
	"DaemonSet":        "apps",
	"Job":              "batch",
	"CronJob":          "batch",
}

// KindToGroup returns a copy of the authoritative kind-to-API-group mapping
// used for owner chain resolution. This is the single source of truth for which
// Kubernetes resource kinds the Gateway must be able to look up.
//
// Exported so the cache configuration and tests can reference it without
// duplicating the list.
func KindToGroup() map[string]string {
	result := make(map[string]string, len(kindToGroup))
	for k, v := range kindToGroup {
		result[k] = v
	}
	return result
}

// OwnerChainCacheObjects returns cache.ByObject entries for every kind in
// kindToGroup using PartialObjectMetadata. These entries configure the
// controller-runtime cache to run metadata-only informers for all resource
// kinds needed by K8sOwnerResolver and ScopeManager.
//
// Business Requirements:
//   - BR-GATEWAY-004: Signal Fingerprinting (owner-chain-based deduplication)
//   - ADR-053: Resource Scope Management (metadata-only informer cache)
//
// Fix for #270: Without these entries the cache only watches RemediationRequest,
// causing OwnerResolver lookups to fail and fingerprints to fall back to pod-level.
func OwnerChainCacheObjects() map[client.Object]cache.ByObject {
	entries := make(map[client.Object]cache.ByObject, len(kindToGroup))
	for kind, group := range kindToGroup {
		obj := &metav1.PartialObjectMetadata{}
		obj.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   group,
			Version: "v1",
			Kind:    kind,
		})
		entries[obj] = cache.ByObject{}
	}
	return entries
}

// K8sOwnerResolver resolves the top-level controller owner of a Kubernetes
// resource by traversing ownerReferences using the metadata-only informer cache.
//
// This implementation reuses the same controller-runtime cached client (ctrlClient)
// that the scope management Manager (ADR-053) uses. When Get() is called with
// PartialObjectMetadata, controller-runtime uses metadata-only informers — so
// owner chain resolution is a pure cache lookup with zero API server calls.
//
// #282: An optional fallbackReader (uncached apiReader) is used when the
// informer cache misses a resource (e.g., newly created pods after rollout restart).
// Without this, the resolver falls back to pod-level fingerprinting, defeating dedup.
//
// #284: Trust-but-verify — when the cache returns a resource without controller
// ownerReferences and no owner has been found yet, the resolver re-checks with
// the direct API. This catches stale PartialObjectMetadata from the informer
// cache during rollout restarts where the cached entry loses ownerReferences.
//
// Business Requirements:
//   - BR-GATEWAY-004: Signal Fingerprinting (owner-chain-based deduplication for K8s events)
//
// Architecture:
//   - ADR-053: Resource Scope Management (shared metadata-only informer cache)
//   - Same pattern as pkg/signalprocessing/ownerchain/builder.go (SP uses full client)
type K8sOwnerResolver struct {
	client         client.Reader
	fallbackReader client.Reader
	logger         logr.Logger
}

// NewK8sOwnerResolver creates a new owner resolver backed by the metadata-only
// informer cache. Pass the same ctrlClient used by scope.NewManager().
// A logr.Logger is required for diagnostics (the gateway does not use
// controller-runtime's global logger, so log.FromContext is unreliable).
// An optional fallback reader (uncached apiReader) can be provided via
// WithFallbackReader to handle cache misses for newly created resources (#282).
func NewK8sOwnerResolver(c client.Reader, logger logr.Logger, opts ...OwnerResolverOption) *K8sOwnerResolver {
	r := &K8sOwnerResolver{
		client: c,
		logger: logger.WithName("owner-resolver"),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// OwnerResolverOption configures optional behavior on K8sOwnerResolver.
type OwnerResolverOption func(*K8sOwnerResolver)

// WithFallbackReader sets an uncached client.Reader used when the primary
// (informer-backed) reader cannot find a resource. This eliminates pod-level
// fingerprint fallback caused by cache sync lag after rollout restarts (#282).
func WithFallbackReader(reader client.Reader) OwnerResolverOption {
	return func(r *K8sOwnerResolver) {
		r.fallbackReader = reader
	}
}

// ResolveTopLevelOwner traverses ownerReferences to find the top-level controller.
//
// Algorithm (same as pkg/signalprocessing/ownerchain/builder.go:Build):
//  1. Fetch resource metadata via cached PartialObjectMetadata
//  2. Find ownerReference with controller: true
//  3. Repeat until no more controller owners or MaxOwnerChainDepth reached
//  4. Return the last owner found (top-level controller)
//
// Graceful degradation:
//   - Unknown kind (not in kindToGroup): returns error (caller falls back to involvedObject)
//   - Resource not found: returns error (pod may have been deleted)
//   - No ownerReferences: returns the resource itself (standalone pod, bare deployment)
//   - Max depth reached: returns deepest owner found (prevents infinite loops)
func (r *K8sOwnerResolver) ResolveTopLevelOwner(ctx context.Context, namespace, kind, name string) (ownerKind, ownerName string, err error) {
	logger := r.logger

	currentNamespace := namespace
	currentKind := kind
	currentName := name

	// Track the most recent owner found (starts as the resource itself)
	topKind := kind
	topName := name
	foundOwner := false

	for i := 0; i < MaxOwnerChainDepth; i++ {
		// Check if we know this kind
		if _, known := kindToGroup[currentKind]; !known {
			logger.V(1).Info("Unknown kind, stopping traversal",
				"kind", currentKind, "level", i)
			break
		}

		gvk := schema.GroupVersionKind{
			Group:   kindToGroup[currentKind],
			Version: "v1",
			Kind:    currentKind,
		}

		obj := &metav1.PartialObjectMetadata{}
		obj.SetGroupVersionKind(gvk)

		// Use timeout to prevent indefinite blocking on informer sync
		lookupCtx, cancel := context.WithTimeout(ctx, ownerLookupTimeout)
		key := client.ObjectKey{Namespace: currentNamespace, Name: currentName}
		getErr := r.client.Get(lookupCtx, key, obj)
		cancel()

		// #282: On cache miss, retry with the uncached fallback reader (direct API).
		if getErr != nil && r.fallbackReader != nil {
			logger.Info("Cache miss, retrying with direct API reader",
				"kind", currentKind, "name", currentName, "error", getErr, "level", i)
			fallbackCtx, fallbackCancel := context.WithTimeout(ctx, ownerLookupTimeout)
			getErr = r.fallbackReader.Get(fallbackCtx, key, obj)
			fallbackCancel()
		}

		if getErr != nil {
			logger.V(1).Info("Failed to fetch resource metadata",
				"kind", currentKind, "name", currentName, "error", getErr, "level", i,
				"foundOwner", foundOwner)
			if !foundOwner {
				return "", "", fmt.Errorf("failed to resolve owner for %s/%s: %w", currentKind, currentName, getErr)
			}
			break
		}

		// Find controller owner
		var controllerRef *metav1.OwnerReference
		for j := range obj.OwnerReferences {
			if obj.OwnerReferences[j].Controller != nil && *obj.OwnerReferences[j].Controller {
				controllerRef = &obj.OwnerReferences[j]
				break
			}
		}

		logger.V(1).Info("Owner chain step",
			"level", i, "kind", currentKind, "name", currentName,
			"ownerRefsCount", len(obj.OwnerReferences),
			"hasControllerRef", controllerRef != nil,
			"foundOwner", foundOwner,
			"deletionTimestamp", obj.DeletionTimestamp)

		if controllerRef == nil {
			// #284: Trust-but-verify with direct API when the informer cache
			// returns stale metadata without ownerReferences. This race
			// condition occurs during rollout restarts when the cache entry
			// for a terminating Pod loses its ownerReferences.
			if !foundOwner && r.fallbackReader != nil {
				freshObj := &metav1.PartialObjectMetadata{}
				freshObj.SetGroupVersionKind(gvk)
				freshCtx, freshCancel := context.WithTimeout(ctx, ownerLookupTimeout)
				freshErr := r.fallbackReader.Get(freshCtx, key, freshObj)
				freshCancel()

				if freshErr != nil {
					logger.Info("Resource not found via direct API, likely deleted",
						"kind", currentKind, "name", currentName, "error", freshErr, "level", i)
					return "", "", fmt.Errorf("failed to resolve owner for %s/%s: not found via direct API: %w",
						currentKind, currentName, freshErr)
				}

				logger.V(1).Info("Trust-but-verify: direct API result",
					"kind", currentKind, "name", currentName,
					"freshOwnerRefsCount", len(freshObj.OwnerReferences),
					"freshDeletionTimestamp", freshObj.DeletionTimestamp, "level", i)

				for j := range freshObj.OwnerReferences {
					if freshObj.OwnerReferences[j].Controller != nil && *freshObj.OwnerReferences[j].Controller {
						controllerRef = &freshObj.OwnerReferences[j]
						break
					}
				}

				if controllerRef != nil {
					logger.Info("Stale cache detected: direct API returned ownerReference missing from cache",
						"kind", currentKind, "name", currentName,
						"owner", fmt.Sprintf("%s/%s", controllerRef.Kind, controllerRef.Name), "level", i)
					topKind = controllerRef.Kind
					topName = controllerRef.Name
					foundOwner = true
					currentKind = controllerRef.Kind
					currentName = controllerRef.Name
					continue
				}

				logger.V(1).Info("Verified standalone resource via direct API (no controller owner)",
					"kind", currentKind, "name", currentName, "level", i)
			} else {
				logger.V(1).Info("No controller owner found, chain complete",
					"kind", currentKind, "name", currentName, "level", i)
			}
			break
		}

		// Move up the chain
		topKind = controllerRef.Kind
		topName = controllerRef.Name
		foundOwner = true

		logger.V(1).Info("Found controller owner",
			"level", i,
			"from", fmt.Sprintf("%s/%s", currentKind, currentName),
			"to", fmt.Sprintf("%s/%s", controllerRef.Kind, controllerRef.Name))

		currentKind = controllerRef.Kind
		currentName = controllerRef.Name
		// Namespace stays the same (ownerReferences are namespace-scoped)
	}

	logger.V(1).Info("Owner resolution complete",
		"inputKind", kind, "inputName", name,
		"resolvedKind", topKind, "resolvedName", topName,
		"foundOwner", foundOwner)
	return topKind, topName, nil
}
