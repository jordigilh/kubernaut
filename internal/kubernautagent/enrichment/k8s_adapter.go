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

	"github.com/jordigilh/kubernaut/pkg/shared/hash"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const maxOwnerChainDepth = 10

// K8sAdapter implements K8sClient by walking ownerReferences via the dynamic client.
type K8sAdapter struct {
	dynClient dynamic.Interface
	mapper    meta.RESTMapper
}

var _ K8sClient = (*K8sAdapter)(nil)

// NewK8sAdapter creates a K8sAdapter wrapping the given dynamic client and REST mapper.
func NewK8sAdapter(dynClient dynamic.Interface, mapper meta.RESTMapper) *K8sAdapter {
	return &K8sAdapter{dynClient: dynClient, mapper: mapper}
}

// GetOwnerChain walks ownerReferences from the given resource up to maxOwnerChainDepth.
// At each level, only the controller ownerReference (Controller: true) is followed,
// aligning with Gateway and SignalProcessing behavior (see #696).
// Returns the chain from the immediate owner to the root owner, excluding the starting resource.
//
// Scope-aware (#762): uses RESTMapping.Scope to determine cluster vs namespaced
// API calls, rather than relying on the namespace parameter being empty.
func (a *K8sAdapter) GetOwnerChain(ctx context.Context, kind, name, namespace string) ([]OwnerChainEntry, error) {
	mapping, err := a.resolveMapping(kind)
	if err != nil {
		return nil, fmt.Errorf("k8s adapter: resolve GVR for %s: %w", kind, err)
	}

	resourceClient := a.scopedClient(mapping, namespace)

	obj, err := resourceClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s adapter: get %s/%s in %s: %w", kind, name, namespace, err)
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
			Kind:      ownerRef.Kind,
			Name:      ownerRef.Name,
			Namespace: ownerNS,
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
func (a *K8sAdapter) GetSpecHash(ctx context.Context, kind, name, namespace string) (string, error) {
	mapping, err := a.resolveMapping(kind)
	if err != nil {
		return "", fmt.Errorf("k8s adapter: resolve GVR for spec hash of %s: %w", kind, err)
	}

	resourceClient := a.scopedClient(mapping, namespace)

	obj, err := resourceClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("k8s adapter: get %s/%s in %s for spec hash: %w", kind, name, namespace, err)
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

// resolveMapping returns the full RESTMapping for a Kind, including GVR and Scope.
// Includes resettableMapper retry for CRDs installed after startup.
func (a *K8sAdapter) resolveMapping(kind string) (*meta.RESTMapping, error) {
	plural := strings.ToLower(kind) + "s"
	gvr, err := a.mapper.ResourceFor(schema.GroupVersionResource{Resource: plural})
	if err != nil {
		if rm, ok := a.mapper.(resettableMapper); ok {
			rm.Reset()
			gvr, err = a.mapper.ResourceFor(schema.GroupVersionResource{Resource: plural})
		}
		if err != nil {
			return nil, err
		}
	}
	gvk, err := a.mapper.KindFor(gvr)
	if err != nil {
		return nil, fmt.Errorf("resolve kind for %s: %w", gvr, err)
	}
	return a.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
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
