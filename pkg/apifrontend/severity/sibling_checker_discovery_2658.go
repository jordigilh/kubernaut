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

package severity

import (
	"context"
	"fmt"
	"sort"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// ResourceLister is the narrow subset of the discovery client the cross-group
// sibling check needs (#2658). *discovery.DiscoveryClient satisfies it.
//
// It is deliberately not a meta.RESTMapper: the mapper can only be asked about
// a Kind through a guessed plural resource name, which is exactly what fails
// for irregular plurals (Gateway -> "gatewaies"). Discovery answers with the
// real plural, group and scope.
type ResourceLister interface {
	ServerGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error)
}

// kindResource is one served (kind, group) pair discovered from the API server.
type kindResource struct {
	group      string
	gvr        schema.GroupVersionResource
	namespaced bool
}

// discoverySiblingChecker is the preferred CrossGroupSiblingChecker: it
// enumerates a Kind's API groups from exact discovery data, so irregular
// plurals resolve and no plural is ever guessed. It keeps the same fail-closed
// contract as the mapper-backed checker.
type discoverySiblingChecker struct {
	lister ResourceLister
	client dynamic.Interface
	logger logr.Logger
}

// NewDiscoveryCrossGroupSiblingChecker builds the discovery-backed
// CrossGroupSiblingChecker (#2658). Prefer it over the mapper-backed
// constructor wherever a discovery client exists: it resolves Kinds whose real
// plural the apimachinery guess cannot derive.
func NewDiscoveryCrossGroupSiblingChecker(lister ResourceLister, client dynamic.Interface, logger logr.Logger) CrossGroupSiblingChecker {
	return &discoverySiblingChecker{lister: lister, client: client, logger: logger}
}

// CheckCrossGroupSiblings reports the API groups (other than targetGroup) in
// which a resource with the same namespace/kind/name exists.
func (c *discoverySiblingChecker) CheckCrossGroupSiblings(ctx context.Context, namespace, kind, name, targetGroup string) ([]string, error) {
	if c.lister == nil || c.client == nil {
		return nil, fmt.Errorf("%w: discovery client or Kubernetes client not configured", ErrGroupEnumerationUnavailable)
	}
	served, err := c.servedKinds(kind)
	if err != nil {
		return nil, err
	}
	candidates := make([]kindResource, 0, len(served))
	seenGroups := make(map[string]struct{}, len(served))
	for _, resource := range served {
		if resource.group == targetGroup {
			continue
		}
		if _, done := seenGroups[resource.group]; done {
			continue // one existence check per group (multiple versions)
		}
		seenGroups[resource.group] = struct{}{}
		candidates = append(candidates, resource)
	}
	if len(candidates) == 0 {
		return nil, nil // single-group Kind: no cross-group sibling possible
	}

	siblings := make([]string, 0, len(candidates))
	for _, resource := range candidates {
		exists, existsErr := c.exists(ctx, resource, namespace, name)
		if existsErr != nil {
			return nil, existsErr
		}
		if exists {
			siblings = append(siblings, resource.group)
		}
	}
	sort.Strings(siblings)
	return siblings, nil
}

// servedKinds returns every group in which kind is served. A discovery failure
// or a Kind the server does not serve is ErrGroupEnumerationUnavailable: "I
// could not enumerate" must never be read as "there is no sibling".
func (c *discoverySiblingChecker) servedKinds(kind string) ([]kindResource, error) {
	_, lists, err := c.lister.ServerGroupsAndResources()
	if err != nil {
		return nil, fmt.Errorf("%w: discovery: %w", ErrGroupEnumerationUnavailable, err)
	}
	served := make([]kindResource, 0, len(lists))
	for _, list := range lists {
		if list == nil {
			continue
		}
		groupVersion, parseErr := schema.ParseGroupVersion(list.GroupVersion)
		if parseErr != nil {
			continue
		}
		for _, resource := range list.APIResources {
			if resource.Kind != kind || resource.Name == "" || containsSlash(resource.Name) {
				continue
			}
			served = append(served, kindResource{
				group:      groupVersion.Group,
				gvr:        schema.GroupVersionResource{Group: groupVersion.Group, Version: groupVersion.Version, Resource: resource.Name},
				namespaced: resource.Namespaced,
			})
		}
	}
	if len(served) == 0 {
		return nil, fmt.Errorf("%w: kind %q is not served by any API group", ErrGroupEnumerationUnavailable, kind)
	}
	return served, nil
}

// exists issues a single existence GET for a discovered resource.
func (c *discoverySiblingChecker) exists(ctx context.Context, resource kindResource, namespace, name string) (bool, error) {
	var getter dynamic.ResourceInterface = c.client.Resource(resource.gvr)
	if resource.namespaced {
		if namespace == "" {
			return false, nil
		}
		getter = c.client.Resource(resource.gvr).Namespace(namespace)
	}
	if _, err := getter.Get(ctx, name, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		c.logger.Info("cross-group sibling read failed",
			"kind", resource.gvr.Resource, "group", resource.group, "namespace", namespace, "name", name, "error", err.Error())
		return false, fmt.Errorf("read candidate sibling %s: %w", resource.gvr.String(), err)
	}
	return true, nil
}

// containsSlash filters discovery subresources ("deployments/status"), which
// are not addressable identities.
func containsSlash(resource string) bool {
	for i := 0; i < len(resource); i++ {
		if resource[i] == '/' {
			return true
		}
	}
	return false
}

// compile-time guard: the mapper-based checker must keep satisfying the
// interface alongside the discovery-based one.
var _ CrossGroupSiblingChecker = (*restMapperSiblingChecker)(nil)
var _ CrossGroupSiblingChecker = (*discoverySiblingChecker)(nil)
