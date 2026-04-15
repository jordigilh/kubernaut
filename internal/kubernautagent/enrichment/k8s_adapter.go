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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/meta"
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
func (a *K8sAdapter) GetOwnerChain(ctx context.Context, kind, name, namespace string) ([]OwnerChainEntry, error) {
	gvr, err := a.resolveGVR(kind)
	if err != nil {
		return nil, fmt.Errorf("k8s adapter: resolve GVR for %s: %w", kind, err)
	}

	var resourceClient dynamic.ResourceInterface
	if namespace != "" {
		resourceClient = a.dynClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceClient = a.dynClient.Resource(gvr)
	}

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
		chain = append(chain, OwnerChainEntry{
			Kind:      ownerRef.Kind,
			Name:      ownerRef.Name,
			Namespace: namespace,
		})

		ownerGVR, err := a.resolveOwnerGVR(ownerRef)
		if err != nil {
			break
		}

		var ownerClient dynamic.ResourceInterface
		if namespace != "" {
			ownerClient = a.dynClient.Resource(ownerGVR).Namespace(namespace)
		} else {
			ownerClient = a.dynClient.Resource(ownerGVR)
		}

		ownerObj, err := ownerClient.Get(ctx, ownerRef.Name, metav1.GetOptions{})
		if err != nil {
			break
		}
		current = ownerObj
	}

	return chain, nil
}

// GetSpecHash fetches the resource and computes a canonical SHA-256 hash of its .spec field.
// Returns an empty string (not an error) when the resource has no .spec (e.g. ConfigMaps, Nodes).
func (a *K8sAdapter) GetSpecHash(ctx context.Context, kind, name, namespace string) (string, error) {
	gvr, err := a.resolveGVR(kind)
	if err != nil {
		return "", fmt.Errorf("k8s adapter: resolve GVR for spec hash of %s: %w", kind, err)
	}

	var resourceClient dynamic.ResourceInterface
	if namespace != "" {
		resourceClient = a.dynClient.Resource(gvr).Namespace(namespace)
	} else {
		resourceClient = a.dynClient.Resource(gvr)
	}

	obj, err := resourceClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("k8s adapter: get %s/%s in %s for spec hash: %w", kind, name, namespace, err)
	}

	spec, ok := obj.Object["spec"]
	if !ok {
		return "", nil
	}

	specMap, ok := spec.(map[string]interface{})
	if !ok {
		return "", nil
	}

	h, err := hash.CanonicalSpecHash(specMap)
	if err != nil {
		return "", fmt.Errorf("k8s adapter: compute spec hash for %s/%s in %s: %w", kind, name, namespace, err)
	}
	return h, nil
}

func (a *K8sAdapter) resolveGVR(kind string) (schema.GroupVersionResource, error) {
	plural := strings.ToLower(kind) + "s"
	gvr, err := a.mapper.ResourceFor(schema.GroupVersionResource{Resource: plural})
	if err != nil {
		return schema.GroupVersionResource{}, err
	}
	return gvr, nil
}

func (a *K8sAdapter) resolveOwnerGVR(ref metav1.OwnerReference) (schema.GroupVersionResource, error) {
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("parse API version %q: %w", ref.APIVersion, err)
	}
	mapping, err := a.mapper.RESTMapping(schema.GroupKind{Group: gv.Group, Kind: ref.Kind}, gv.Version)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}
	return mapping.Resource, nil
}
