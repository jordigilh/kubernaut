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
	"sort"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
//
// Deprecated: Superseded by APIResourceRegistry.KindToGVR() (#1029).
// Retained as a nil-registry fallback for tests and the exported KindToGroup()/
// OwnerChainCacheObjects() helpers. Will be removed in a dedicated cleanup PR
// once all consumers migrate to the registry.
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
	registry       *APIResourceRegistry
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

// WithRegistry sets the APIResourceRegistry for dynamic GVR lookup and
// CRD traversal control, replacing the static kindToGroup map (#1029).
func WithRegistry(reg *APIResourceRegistry) OwnerResolverOption {
	return func(r *K8sOwnerResolver) {
		r.registry = reg
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

	// #1029 Use Case 3: Service targets are Prometheus scrape infrastructure labels,
	// not workloads. Traverse Service → selector → Pods → ownerRefs to find the
	// actual backing workload before entering the normal ownerReference chain.
	if currentKind == "Service" {
		podKind, podName, resolved := r.resolveServiceToWorkload(ctx, currentNamespace, currentName)
		if resolved {
			currentKind = podKind
			currentName = podName
		} else {
			return currentKind, currentName, nil
		}
	}

	// Track the most recent owner found (starts as the resource itself)
	topKind := currentKind
	topName := currentName
	foundOwner := false

	for i := 0; i < MaxOwnerChainDepth; i++ {
		step := r.traverseOneOwnerLevel(ctx, currentNamespace, currentKind, currentName, foundOwner, i)
		if step.err != nil {
			return "", "", step.err
		}
		if step.advanced {
			topKind, topName = step.nextKind, step.nextName
			foundOwner = true
			currentKind, currentName = step.nextKind, step.nextName
		}
		if step.stop {
			break
		}
	}

	logger.V(1).Info("Owner resolution complete",
		"inputKind", kind, "inputName", name,
		"resolvedKind", topKind, "resolvedName", topName,
		"foundOwner", foundOwner)
	return topKind, topName, nil
}

// ownerStepResult captures the outcome of a single ResolveTopLevelOwner
// traversal step (extracted to keep the driving loop's cognitive complexity
// low — see Go Anti-Pattern Checklist, unnecessary nesting).
type ownerStepResult struct {
	stop     bool  // true: caller must break out of the traversal loop
	advanced bool  // true: nextKind/nextName represent a new, resolved owner
	nextKind string
	nextName string
	err      error // non-nil: caller must abort and propagate this error
}

// traverseOneOwnerLevel performs one level of owner-chain traversal: resolve
// the GVK, skip CRD kinds, fetch metadata (with cache-miss fallback), and find
// (or trust-but-verify) the controller ownerReference. Extracted from
// ResolveTopLevelOwner's for-loop body.
func (r *K8sOwnerResolver) traverseOneOwnerLevel(
	ctx context.Context,
	currentNamespace, currentKind, currentName string,
	foundOwner bool,
	level int,
) ownerStepResult {
	logger := r.logger

	gvk, known := r.resolveGVK(currentKind)
	if !known {
		logger.V(1).Info("Unknown kind, stopping traversal", "kind", currentKind, "level", level)
		return ownerStepResult{stop: true}
	}

	// CRD kinds (not in core/apps/batch/autoscaling/policy) have
	// unpredictable owner chains — stop traversal (#1029 Option C).
	if r.registry != nil && !r.registry.IsCoreBatchAppsKind(currentKind) {
		logger.V(1).Info("CRD kind, stopping traversal", "kind", currentKind, "level", level)
		return ownerStepResult{stop: true}
	}

	obj := &metav1.PartialObjectMetadata{}
	obj.SetGroupVersionKind(gvk)
	key := client.ObjectKey{Namespace: currentNamespace, Name: currentName}

	getErr := r.getResourceMetadata(ctx, key, obj)
	if getErr != nil && r.fallbackReader != nil {
		logger.Info("Cache miss, retrying with direct API reader",
			"kind", currentKind, "name", currentName, "error", getErr, "level", level)
		getErr = r.getResourceMetadataFallback(ctx, key, obj)
	}

	if getErr != nil {
		logger.V(1).Info("Failed to fetch resource metadata",
			"kind", currentKind, "name", currentName, "error", getErr, "level", level,
			"foundOwner", foundOwner)
		if !foundOwner {
			return ownerStepResult{err: fmt.Errorf("failed to resolve owner for %s/%s: %w", currentKind, currentName, getErr)}
		}
		return ownerStepResult{stop: true}
	}

	controllerRef := findControllerOwnerRef(obj.OwnerReferences)

	logger.V(1).Info("Owner chain step",
		"level", level, "kind", currentKind, "name", currentName,
		"ownerRefsCount", len(obj.OwnerReferences),
		"hasControllerRef", controllerRef != nil,
		"foundOwner", foundOwner,
		"deletionTimestamp", obj.DeletionTimestamp)

	if controllerRef == nil {
		return r.resolveMissingControllerRef(ctx, gvk, key, currentKind, currentName, foundOwner, level)
	}

	logger.V(1).Info("Found controller owner",
		"level", level,
		"from", fmt.Sprintf("%s/%s", currentKind, currentName),
		"to", fmt.Sprintf("%s/%s", controllerRef.Kind, controllerRef.Name))

	return ownerStepResult{advanced: true, nextKind: controllerRef.Kind, nextName: controllerRef.Name}
}

// resolveMissingControllerRef handles the case where the cached lookup found
// no controller ownerReference: either trust-but-verify via the direct API
// (#284), or conclude the chain is complete. Extracted from
// traverseOneOwnerLevel to keep its cognitive complexity low.
func (r *K8sOwnerResolver) resolveMissingControllerRef(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	key client.ObjectKey,
	currentKind, currentName string,
	foundOwner bool,
	level int,
) ownerStepResult {
	if foundOwner || r.fallbackReader == nil {
		r.logger.V(1).Info("No controller owner found, chain complete",
			"kind", currentKind, "name", currentName, "level", level)
		return ownerStepResult{stop: true}
	}

	// #284: Trust-but-verify with direct API when the informer cache returns
	// stale metadata without ownerReferences (race during rollout restarts).
	verifiedRef, verifyErr := r.verifyOwnerViaFallback(ctx, gvk, key, currentKind, currentName, level)
	if verifyErr != nil {
		return ownerStepResult{err: verifyErr}
	}
	if verifiedRef != nil {
		return ownerStepResult{advanced: true, nextKind: verifiedRef.Kind, nextName: verifiedRef.Name}
	}
	return ownerStepResult{stop: true}
}

// getResourceMetadata fetches PartialObjectMetadata via the primary (cached)
// reader, bounded by ownerLookupTimeout to avoid indefinite blocking on
// informer sync (extracted from ResolveTopLevelOwner for testability).
func (r *K8sOwnerResolver) getResourceMetadata(ctx context.Context, key client.ObjectKey, obj *metav1.PartialObjectMetadata) error {
	lookupCtx, cancel := context.WithTimeout(ctx, ownerLookupTimeout)
	defer cancel()
	return r.client.Get(lookupCtx, key, obj)
}

// getResourceMetadataFallback retries a metadata fetch via the uncached
// fallback reader (direct API). Callers must only invoke this when
// r.fallbackReader is non-nil (checked by ResolveTopLevelOwner before calling).
//
// #282: Eliminates pod-level fingerprint fallback caused by cache sync lag
// after rollout restarts.
func (r *K8sOwnerResolver) getResourceMetadataFallback(ctx context.Context, key client.ObjectKey, obj *metav1.PartialObjectMetadata) error {
	fallbackCtx, cancel := context.WithTimeout(ctx, ownerLookupTimeout)
	defer cancel()
	return r.fallbackReader.Get(fallbackCtx, key, obj)
}

// findControllerOwnerRef returns the ownerReference with controller: true, or
// nil if none exists. Extracted from ResolveTopLevelOwner (used both for the
// primary cached lookup and the trust-but-verify fallback lookup).
func findControllerOwnerRef(refs []metav1.OwnerReference) *metav1.OwnerReference {
	for j := range refs {
		if refs[j].Controller != nil && *refs[j].Controller {
			return &refs[j]
		}
	}
	return nil
}

// verifyOwnerViaFallback implements the #284 "trust-but-verify" path: when the
// informer cache returns an object with no controller ownerReference, this
// re-fetches the object via the uncached direct-API reader before concluding
// the chain is genuinely standalone. This guards against a race condition
// during rollout restarts where the cache entry for a terminating Pod loses
// its ownerReferences before eviction.
//
// Returns (ref, nil) when a controller owner was found via the direct API
// (caller should continue traversal from ref), (nil, nil) when the direct API
// confirms no controller owner (caller should stop, resource is standalone),
// or (nil, err) when the resource could not be found via the direct API at
// all (likely deleted — caller should propagate the error).
func (r *K8sOwnerResolver) verifyOwnerViaFallback(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	key client.ObjectKey,
	currentKind, currentName string,
	level int,
) (*metav1.OwnerReference, error) {
	logger := r.logger

	freshObj := &metav1.PartialObjectMetadata{}
	freshObj.SetGroupVersionKind(gvk)
	freshCtx, freshCancel := context.WithTimeout(ctx, ownerLookupTimeout)
	freshErr := r.fallbackReader.Get(freshCtx, key, freshObj)
	freshCancel()

	if freshErr != nil {
		logger.Info("Resource not found via direct API, likely deleted",
			"kind", currentKind, "name", currentName, "error", freshErr, "level", level)
		return nil, fmt.Errorf("failed to resolve owner for %s/%s: not found via direct API: %w",
			currentKind, currentName, freshErr)
	}

	logger.V(1).Info("Trust-but-verify: direct API result",
		"kind", currentKind, "name", currentName,
		"freshOwnerRefsCount", len(freshObj.OwnerReferences),
		"freshDeletionTimestamp", freshObj.DeletionTimestamp, "level", level)

	controllerRef := findControllerOwnerRef(freshObj.OwnerReferences)
	if controllerRef != nil {
		logger.Info("Stale cache detected: direct API returned ownerReference missing from cache",
			"kind", currentKind, "name", currentName,
			"owner", fmt.Sprintf("%s/%s", controllerRef.Kind, controllerRef.Name), "level", level)
		return controllerRef, nil
	}

	logger.V(1).Info("Verified standalone resource via direct API (no controller owner)",
		"kind", currentKind, "name", currentName, "level", level)
	return nil, nil
}

// resolveServiceToWorkload traverses Service → spec.selector → Pods to find a
// backing Pod whose ownerReference chain leads to a workload controller.
// Returns ("Pod", podName, true) when a selector-matched Pod is found, allowing
// the caller to continue the normal ownerRef traversal from that Pod.
// Returns ("", "", false) when traversal cannot proceed (no fallbackReader,
// Service not found, no selector, no matching pods).
func (r *K8sOwnerResolver) resolveServiceToWorkload(ctx context.Context, namespace, serviceName string) (string, string, bool) {
	if r.fallbackReader == nil {
		r.logger.V(1).Info("Service-to-workload traversal skipped: no fallback reader",
			"service", serviceName)
		return "", "", false
	}

	lookupCtx, cancel := context.WithTimeout(ctx, ownerLookupTimeout)
	defer cancel()

	svc := &corev1.Service{}
	key := client.ObjectKey{Namespace: namespace, Name: serviceName}
	if err := r.fallbackReader.Get(lookupCtx, key, svc); err != nil {
		r.logger.V(1).Info("Service-to-workload traversal: failed to get Service",
			"service", serviceName, "error", err)
		return "", "", false
	}

	if len(svc.Spec.Selector) == 0 {
		r.logger.V(1).Info("Service-to-workload traversal: Service has no selector",
			"service", serviceName)
		return "", "", false
	}

	podList := &corev1.PodList{}
	listOpts := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labels.SelectorFromSet(svc.Spec.Selector),
	}
	if err := r.fallbackReader.List(lookupCtx, podList, listOpts); err != nil {
		r.logger.V(1).Info("Service-to-workload traversal: failed to list pods",
			"service", serviceName, "selector", svc.Spec.Selector, "error", err)
		return "", "", false
	}

	if len(podList.Items) == 0 {
		r.logger.V(1).Info("Service-to-workload traversal: no pods match selector",
			"service", serviceName, "selector", svc.Spec.Selector)
		return "", "", false
	}

	sort.Slice(podList.Items, func(i, j int) bool {
		return podList.Items[i].Name < podList.Items[j].Name
	})

	selectedPod := podList.Items[0].Name
	r.logger.V(1).Info("Service-to-workload traversal: resolved to pod",
		"service", serviceName, "pod", selectedPod,
		"matchingPods", len(podList.Items))
	return "Pod", selectedPod, true
}

// resolveGVK returns the GroupVersionKind for a given kind string.
// Uses the registry if available, otherwise falls back to the static kindToGroup map.
func (r *K8sOwnerResolver) resolveGVK(kind string) (schema.GroupVersionKind, bool) {
	if r.registry != nil {
		gvr, ok := r.registry.KindToGVR(kind)
		if !ok {
			return schema.GroupVersionKind{}, false
		}
		return schema.GroupVersionKind{
			Group:   gvr.Group,
			Version: gvr.Version,
			Kind:    kind,
		}, true
	}
	group, known := kindToGroup[kind]
	if !known {
		return schema.GroupVersionKind{}, false
	}
	return schema.GroupVersionKind{
		Group:   group,
		Version: "v1",
		Kind:    kind,
	}, true
}
