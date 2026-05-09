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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
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
	logger    logr.Logger
}

// resettableMapper is implemented by mappers that support Reset() for cache
// invalidation (e.g. restmapper.DeferredDiscoveryRESTMapper).
type resettableMapper interface {
	Reset()
}

// NewDynamicResolver creates a ResourceResolver backed by the Kubernetes dynamic client.
// The kindIndex maps lowercase kind strings to canonical GroupKind values for
// case-insensitive resolution (e.g. "replicaset" -> {Group:"apps", Kind:"ReplicaSet"}).
func NewDynamicResolver(client dynamic.Interface, mapper meta.RESTMapper, kindIndex map[string]schema.GroupKind, logger logr.Logger) ResourceResolver {
	return &dynamicResourceResolver{client: client, mapper: mapper, kindIndex: kindIndex, logger: logger}
}

// BuildKindIndex builds a case-insensitive kind-to-GroupKind index from the
// cluster's discovery API. Keys are lowercase kind strings; values are the
// canonical GroupKind as reported by the API server.
// The index serves as a fallback for resolveMappings when the REST mapper's
// ResourcesFor fails (e.g. partial discovery). It records the first group
// encountered per kind, so multi-group kinds use the mapper path instead.
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

// resolveGroupKind extracts the GroupKind for a kind string using the kindIndex
// for API group hints. Falls back to GroupKind with empty group (core API) when
// the kind is not in the index.
func (r *dynamicResourceResolver) resolveGroupKind(kind string) schema.GroupKind {
	lowerKind := strings.ToLower(kind)
	if gk, found := r.kindIndex[lowerKind]; found {
		return gk
	}
	return schema.GroupKind{Kind: kind}
}

// resolveMappings returns all possible RESTMappings for a kind, supporting
// multi-group kinds like Subscription (operators.coreos.com + messaging.knative.dev)
// and multi-version kinds like AuthorizationPolicy (security.istio.io/v1beta1 + v1).
// Uses ResourcesFor to discover all API groups, with resettableMapper retry.
// Falls back to RESTMappings(GroupKind) when ResourcesFor fails entirely,
// returning ALL versions for the GroupKind instead of a single preferred version.
// Issue #1064 follow-up: RESTMapping (singular) returns AmbiguousKindError on
// MultiRESTMapper for multi-version kinds; RESTMappings (plural) handles this.
func (r *dynamicResourceResolver) resolveMappings(kind string) ([]*meta.RESTMapping, error) {
	resource := strings.ToLower(kind)
	gvrs, err := r.mapper.ResourcesFor(schema.GroupVersionResource{Resource: resource})
	if err != nil {
		if rm, ok := r.mapper.(resettableMapper); ok {
			rm.Reset()
			gvrs, err = r.mapper.ResourcesFor(schema.GroupVersionResource{Resource: resource})
		}
		if err != nil {
			gk := r.resolveGroupKind(kind)
			fallbackMappings, fallbackErr := r.mapper.RESTMappings(gk)
			if fallbackErr != nil || len(fallbackMappings) == 0 {
				return nil, fmt.Errorf("unsupported kind %q: %w", kind, fallbackErr)
			}
			r.logger.V(1).Info("multi-version kind resolved via fallback",
				"kind", kind,
				"group", gk.Group,
				"versions", len(fallbackMappings),
				"source", "RESTMappings")
			return fallbackMappings, nil
		}
	}

	mappings := make([]*meta.RESTMapping, 0, len(gvrs))
	for _, gvr := range gvrs {
		gvk, kindErr := r.mapper.KindFor(gvr)
		if kindErr != nil {
			continue
		}
		mapping, mapErr := r.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if mapErr != nil {
			continue
		}
		mappings = append(mappings, mapping)
	}
	if len(mappings) == 0 {
		return nil, fmt.Errorf("no valid mappings found for kind %q", kind)
	}
	return mappings, nil
}

func (r *dynamicResourceResolver) scopedClient(mapping *meta.RESTMapping, namespace string) dynamic.ResourceInterface {
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		return r.client.Resource(mapping.Resource).Namespace(namespace)
	}
	return r.client.Resource(mapping.Resource)
}

func (r *dynamicResourceResolver) Get(ctx context.Context, kind, name, namespace string) (interface{}, error) {
	mappings, err := r.resolveMappings(kind)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for _, mapping := range mappings {
		obj, getErr := r.scopedClient(mapping, namespace).Get(ctx, name, metav1.GetOptions{})
		if getErr == nil {
			if len(mappings) > 1 {
				r.logger.V(1).Info("multi-group kind resolved",
					"kind", kind, "name", name, "namespace", namespace,
					"api_group", mapping.Resource.Group,
					"gvr", mapping.Resource.String(),
					"result", "success")
			}
			return obj, nil
		}
		lastErr = getErr
		if !errors.IsNotFound(getErr) && !errors.IsForbidden(getErr) {
			return nil, fmt.Errorf("get %s/%s in %s: %w", kind, name, namespace, getErr)
		}
	}
	return nil, fmt.Errorf("get %s/%s in %s: %w", kind, name, namespace, lastErr)
}

func (r *dynamicResourceResolver) List(ctx context.Context, kind, namespace string) (interface{}, error) {
	mappings, err := r.resolveMappings(kind)
	if err != nil {
		return nil, err
	}

	var lastResult interface{}
	for _, mapping := range mappings {
		result, listErr := r.scopedClient(mapping, namespace).List(ctx, metav1.ListOptions{})
		if listErr != nil {
			if !errors.IsNotFound(listErr) && !errors.IsForbidden(listErr) {
				return nil, fmt.Errorf("list %s in %s: %w", kind, namespace, listErr)
			}
			continue
		}
		lastResult = result
		if len(result.Items) > 0 {
			if len(mappings) > 1 {
				r.logger.V(1).Info("multi-group kind resolved",
					"kind", kind, "namespace", namespace,
					"api_group", mapping.Resource.Group,
					"gvr", mapping.Resource.String(),
					"result", "success",
					"items", len(result.Items))
			}
			return result, nil
		}
	}
	if lastResult != nil {
		return lastResult, nil
	}
	return nil, fmt.Errorf("list %s in %s: no results from any API group", kind, namespace)
}
