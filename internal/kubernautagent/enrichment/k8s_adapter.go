/*
Copyright 2026 Jordi Gil.

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

package enrichment

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/pkg/shared/hash"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const maxOwnerChainDepth = 10

// K8sAdapter implements K8sClient by walking ownerReferences via the dynamic client.
type K8sAdapter struct {
	dynClient dynamic.Interface
	mapper    meta.RESTMapper
	logger    logr.Logger
}

var _ K8sClient = (*K8sAdapter)(nil)

// NewK8sAdapter creates a K8sAdapter wrapping the given dynamic client and REST mapper.
func NewK8sAdapter(dynClient dynamic.Interface, mapper meta.RESTMapper) *K8sAdapter {
	return &K8sAdapter{dynClient: dynClient, mapper: mapper, logger: logr.Discard()}
}

// SetLogger configures structured logging for multi-group fallback diagnostics
// (FedRAMP AU-6 / SRE-2). Defaults to logr.Discard() if never called.
func (a *K8sAdapter) SetLogger(l logr.Logger) {
	a.logger = l
}

// GetOwnerChain walks ownerReferences from the given resource up to maxOwnerChainDepth.
// At each level, only the controller ownerReference (Controller: true) is followed,
// aligning with Gateway and SignalProcessing behavior (see #696).
// Returns the chain from the immediate owner to the root owner, excluding the starting resource.
//
// Scope-aware (#762): uses RESTMapping.Scope to determine cluster vs namespaced
// API calls, rather than relying on the namespace parameter being empty.
//
// Multi-group fallback (#1062): when apiVersion is empty and the kind exists in
// multiple API groups, tries each group until Get succeeds or all are exhausted.
func (a *K8sAdapter) GetOwnerChain(ctx context.Context, kind, name, namespace, apiVersion string) ([]OwnerChainEntry, error) {
	obj, _, err := a.getResourceWithFallback(ctx, kind, name, namespace, apiVersion)
	if err != nil {
		return nil, err
	}

	chain := make([]OwnerChainEntry, 0)
	current := obj

	for depth := 0; depth < maxOwnerChainDepth; depth++ {
		owners := current.GetOwnerReferences()
		if len(owners) == 0 {
			break
		}

		var controllerRef *metav1.OwnerReference
		for i := range owners {
			if owners[i].Controller != nil && *owners[i].Controller {
				controllerRef = &owners[i]
				break
			}
		}
		if controllerRef == nil {
			break
		}
		ownerRef := *controllerRef

		ownerMapping, err := a.resolveOwnerMapping(ownerRef)
		if err != nil {
			break
		}

		ownerNS := namespace
		if ownerMapping.Scope.Name() != meta.RESTScopeNameNamespace {
			ownerNS = ""
		}

		chain = append(chain, OwnerChainEntry{
			Kind:       ownerRef.Kind,
			Name:       ownerRef.Name,
			Namespace:  ownerNS,
			APIVersion: ownerRef.APIVersion, // #1040: capture from OwnerReference
		})

		ownerClient := a.scopedClient(ownerMapping, namespace)
		ownerObj, err := ownerClient.Get(ctx, ownerRef.Name, metav1.GetOptions{})
		if err != nil {
			break
		}
		current = ownerObj
	}

	return chain, nil
}

// GetSpecHash fetches the resource and computes a canonical resource fingerprint.
// Uses CanonicalResourceFingerprint (#765) which hashes all functional state,
// not just .spec. The method name is retained for interface compatibility.
//
// Scope-aware (#762): uses RESTMapping.Scope for cluster vs namespaced dispatch.
//
// Multi-group fallback (#1062): same as GetOwnerChain.
func (a *K8sAdapter) GetSpecHash(ctx context.Context, kind, name, namespace, apiVersion string) (string, error) {
	obj, _, err := a.getResourceWithFallback(ctx, kind, name, namespace, apiVersion)
	if err != nil {
		return "", err
	}

	h, err := hash.CanonicalResourceFingerprint(obj.Object)
	if err != nil {
		return "", fmt.Errorf("k8s adapter: compute resource fingerprint for %s/%s in %s: %w", kind, name, namespace, err)
	}
	return h, nil
}

// resettableMapper is satisfied by restmapper.DeferredDiscoveryRESTMapper
// and allows resolveGVR to invalidate stale discovery caches when CRDs are
// installed after the agent starts.
type resettableMapper interface {
	Reset()
}

// resolveMappingWithAPIVersion resolves a RESTMapping using an explicit
// apiVersion. Callers must provide a non-empty apiVersion; the empty-apiVersion
// path uses resolveMappingsAll for multi-group fallback (#1062). Issue #1040.
func (a *K8sAdapter) resolveMappingWithAPIVersion(kind, apiVersion string) (*meta.RESTMapping, error) {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return nil, fmt.Errorf("parse API version %q: %w", apiVersion, err)
	}
	mapping, err := a.mapper.RESTMapping(schema.GroupKind{Group: gv.Group, Kind: kind}, gv.Version)
	if err != nil {
		if rm, ok := a.mapper.(resettableMapper); ok {
			rm.Reset()
			mapping, err = a.mapper.RESTMapping(schema.GroupKind{Group: gv.Group, Kind: kind}, gv.Version)
		}
		if err != nil {
			return nil, fmt.Errorf("resolve mapping for %s in apiVersion %q: %w", kind, apiVersion, err)
		}
	}
	return mapping, nil
}

// getResourceWithFallback fetches a resource, trying multiple API groups when
// the kind is ambiguous and apiVersion is empty. Issue #1062.
// Returns the fetched object, the mapping that succeeded, and any error.
func (a *K8sAdapter) getResourceWithFallback(ctx context.Context, kind, name, namespace, apiVersion string) (*unstructured.Unstructured, *meta.RESTMapping, error) {
	if apiVersion != "" {
		mapping, err := a.resolveMappingWithAPIVersion(kind, apiVersion)
		if err != nil {
			return nil, nil, fmt.Errorf("k8s adapter: resolve GVR for %s: %w", kind, err)
		}
		obj, err := a.scopedClient(mapping, namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, nil, fmt.Errorf("k8s adapter: get %s/%s in %s: %w", kind, name, namespace, err)
		}
		return obj, mapping, nil
	}

	mappings, err := a.resolveMappingsAll(kind)
	if err != nil {
		return nil, nil, fmt.Errorf("k8s adapter: resolve GVR for %s: %w", kind, err)
	}

	var lastErr error
	for _, mapping := range mappings {
		obj, getErr := a.scopedClient(mapping, namespace).Get(ctx, name, metav1.GetOptions{})
		if getErr == nil {
			if len(mappings) > 1 {
				a.logger.V(1).Info("multi-group kind resolved",
					"kind", kind, "name", name, "namespace", namespace,
					"api_group", mapping.Resource.Group,
					"gvr", mapping.Resource.String(),
					"result", "success")
			}
			return obj, mapping, nil
		}
		errReason := "unknown"
		if errors.IsNotFound(getErr) {
			errReason = "NotFound"
		} else if errors.IsForbidden(getErr) {
			errReason = "Forbidden"
		}
		a.logger.V(1).Info("multi-group fallback attempt",
			"kind", kind, "name", name, "namespace", namespace,
			"api_group", mapping.Resource.Group,
			"gvr", mapping.Resource.String(),
			"result", errReason)
		lastErr = getErr
		if !errors.IsNotFound(getErr) && !errors.IsForbidden(getErr) {
			return nil, nil, fmt.Errorf("k8s adapter: get %s/%s in %s: %w", kind, name, namespace, getErr)
		}
	}
	return nil, nil, fmt.Errorf("k8s adapter: get %s/%s in %s: %w", kind, name, namespace, lastErr)
}

// resolveMappingsAll returns all possible RESTMappings for a kind, supporting
// multi-group kinds like Subscription (operators.coreos.com + messaging.knative.dev).
// Uses ResourcesFor instead of ResourceFor to avoid AmbiguousResourceError.
// Issue #1062.
func (a *K8sAdapter) resolveMappingsAll(kind string) ([]*meta.RESTMapping, error) {
	plural := strings.ToLower(kind) + "s"
	gvrs, err := a.mapper.ResourcesFor(schema.GroupVersionResource{Resource: plural})
	if err != nil {
		if rm, ok := a.mapper.(resettableMapper); ok {
			rm.Reset()
			gvrs, err = a.mapper.ResourcesFor(schema.GroupVersionResource{Resource: plural})
		}
		if err != nil {
			return nil, err
		}
	}

	mappings := make([]*meta.RESTMapping, 0, len(gvrs))
	for _, gvr := range gvrs {
		gvk, kindErr := a.mapper.KindFor(gvr)
		if kindErr != nil {
			continue
		}
		mapping, mapErr := a.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if mapErr != nil {
			continue
		}
		mappings = append(mappings, mapping)
	}
	if len(mappings) == 0 {
		return nil, fmt.Errorf("no valid mappings found for kind %s", kind)
	}
	return mappings, nil
}

// resolveOwnerMapping resolves a full RESTMapping from an ownerReference.
func (a *K8sAdapter) resolveOwnerMapping(ref metav1.OwnerReference) (*meta.RESTMapping, error) {
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		return nil, fmt.Errorf("parse API version %q: %w", ref.APIVersion, err)
	}
	return a.mapper.RESTMapping(schema.GroupKind{Group: gv.Group, Kind: ref.Kind}, gv.Version)
}

// scopedClient returns a namespaced or cluster-scoped dynamic client based on
// the RESTMapping's Scope, matching the pattern in resolver.go (#762).
func (a *K8sAdapter) scopedClient(mapping *meta.RESTMapping, namespace string) dynamic.ResourceInterface {
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		return a.dynClient.Resource(mapping.Resource).Namespace(namespace)
	}
	return a.dynClient.Resource(mapping.Resource)
}
