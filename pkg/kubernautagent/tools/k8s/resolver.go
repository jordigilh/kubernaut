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

package k8s

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
)

// ResourceResolver abstracts Kubernetes get/list operations so that tools
// are decoupled from the concrete client implementation. The dynamic client
// implementation supports any resource type the cluster knows about, including CRDs.
type ResourceResolver interface {
	Get(ctx context.Context, kind, name, namespace string) (interface{}, error)
	List(ctx context.Context, kind, namespace string) (interface{}, error)
}

type dynamicResourceResolver struct {
	client    dynamic.Interface
	mapper    meta.RESTMapper
	kindIndex map[string]schema.GroupKind
}

// NewDynamicResolver creates a ResourceResolver backed by the Kubernetes dynamic client.
// The kindIndex maps lowercase kind strings to canonical GroupKind values for
// case-insensitive resolution (e.g. "replicaset" -> {Group:"apps", Kind:"ReplicaSet"}).
func NewDynamicResolver(client dynamic.Interface, mapper meta.RESTMapper, kindIndex map[string]schema.GroupKind) ResourceResolver {
	return &dynamicResourceResolver{client: client, mapper: mapper, kindIndex: kindIndex}
}

// BuildKindIndex builds a case-insensitive kind-to-GroupKind index from the
// cluster's discovery API. Keys are lowercase kind strings; values are the
// canonical GroupKind as reported by the API server.
func BuildKindIndex(disc discovery.DiscoveryInterface) (map[string]schema.GroupKind, error) {
	_, apiResourceLists, err := disc.ServerGroupsAndResources()
	if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
		return nil, fmt.Errorf("discovering API resources: %w", err)
	}
	index := make(map[string]schema.GroupKind)
	for _, list := range apiResourceLists {
		gv, parseErr := schema.ParseGroupVersion(list.GroupVersion)
		if parseErr != nil {
			continue
		}
		for _, r := range list.APIResources {
			lowerKind := strings.ToLower(r.Kind)
			if _, exists := index[lowerKind]; !exists {
				index[lowerKind] = schema.GroupKind{Group: gv.Group, Kind: r.Kind}
			}
		}
	}
	return index, nil
}

func (r *dynamicResourceResolver) resolveMapping(kind string) (*meta.RESTMapping, error) {
	lowerKind := strings.ToLower(kind)
	if gk, found := r.kindIndex[lowerKind]; found {
		return r.mapper.RESTMapping(gk)
	}
	return r.mapper.RESTMapping(schema.GroupKind{Kind: kind})
}

func (r *dynamicResourceResolver) Get(ctx context.Context, kind, name, namespace string) (interface{}, error) {
	mapping, err := r.resolveMapping(kind)
	if err != nil {
		return nil, fmt.Errorf("unsupported kind %q: %w", kind, err)
	}
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		return r.client.Resource(mapping.Resource).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	}
	return r.client.Resource(mapping.Resource).Get(ctx, name, metav1.GetOptions{})
}

func (r *dynamicResourceResolver) List(ctx context.Context, kind, namespace string) (interface{}, error) {
	mapping, err := r.resolveMapping(kind)
	if err != nil {
		return nil, fmt.Errorf("unsupported kind for list %q: %w", kind, err)
	}
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		return r.client.Resource(mapping.Resource).Namespace(namespace).List(ctx, metav1.ListOptions{})
	}
	return r.client.Resource(mapping.Resource).List(ctx, metav1.ListOptions{})
}
