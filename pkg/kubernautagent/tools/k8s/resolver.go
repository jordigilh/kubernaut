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
	"sort"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/metadata"
)

// ResourceResolver abstracts Kubernetes get/list operations so that tools
// are decoupled from the concrete client implementation. The dynamic client
// implementation supports any resource type the cluster knows about, including CRDs.
//
// The apiGroup parameter disambiguates kinds that exist in multiple API groups
// (e.g. Node in core "" vs nodes.config.openshift.io). When apiGroup is non-empty,
// resolution is filtered to that group. When empty and the kind is unambiguous
// (single mapping), it resolves automatically. When empty and ambiguous (multiple
// mappings), an error is returned listing available groups (#1311).
type ResourceResolver interface {
	Get(ctx context.Context, kind, name, namespace, apiGroup string) (interface{}, error)
	List(ctx context.Context, kind, namespace, apiGroup string) (interface{}, error)
	ListForQuery(ctx context.Context, kind, namespace, apiGroup string) (interface{}, error)
	GetByNameInCluster(ctx context.Context, kind, name, apiGroup string) (interface{}, error)
}

// DefaultResourceListLimit is sent to the API server for every bulk list.
// List tools return metadata summaries, so a large ConfigMap/Secret body cannot
// be copied into the LLM, audit, or sanitizer pipeline.
const DefaultResourceListLimit int64 = 100

// ResourceListSummary is the bounded wire result for generic bulk lists.
// It intentionally contains metadata only; use a targeted get tool for a full
// resource body.
type ResourceListSummary struct {
	APIVersion string                    `json:"apiVersion,omitempty"`
	Kind       string                    `json:"kind,omitempty"`
	Items      []ResourceMetadataSummary `json:"items"`
	ItemCount  int                       `json:"item_count"`
	TotalCount *int                      `json:"total_count,omitempty"`
	Truncated  bool                      `json:"truncated"`
}

// ResourceMetadataSummary contains only the safe metadata projection of one
// item in a bulk result.
type ResourceMetadataSummary struct {
	Metadata ResourceObjectMetadata `json:"metadata"`
}

// ResourceObjectMetadata is the bounded ObjectMeta subset exposed to tools.
type ResourceObjectMetadata struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace,omitempty"`
	UID             string            `json:"uid,omitempty"`
	ResourceVersion string            `json:"resourceVersion,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

type dynamicResourceResolver struct {
	client         dynamic.Interface
	metadataClient metadata.Interface
	mapper         meta.RESTMapper
	kindIndex      map[string]schema.GroupKind
	logger         logr.Logger
	secretObserver SecretAccessObserver
}

// SecretAccessObserver is invoked every time the resolver performs a Get or
// List that resolves to the core Secret resource, regardless of outcome
// (GAP-13, Issue #1505 — SOC2 CC7.2 / FedRAMP AU-12 detective control).
// KubernautAgent intentionally keeps broad read RBAC on Secrets for
// investigation completeness (missing RBAC degrades RCA quality — see
// docs/services/kubernaut-agent/security-configuration.md); this
// hook lets callers record every access as a dedicated, queryable audit
// event instead of narrowing that RBAC. verb is "get" or "list"; name is
// empty for List. err is the outcome of the underlying API call (nil on
// success). Implementations must not block (fire-and-forget).
type SecretAccessObserver func(ctx context.Context, verb, name, namespace string, err error)

// resettableMapper is implemented by mappers that support Reset() for cache
// invalidation (e.g. restmapper.DeferredDiscoveryRESTMapper).
type resettableMapper interface {
	Reset()
}

// ResolverOption configures optional behavior on a dynamicResourceResolver.
type ResolverOption func(*dynamicResourceResolver)

// WithSecretAccessObserver registers a callback invoked on every Get/List
// that resolves to the core Secret resource (GAP-13, Issue #1505).
func WithSecretAccessObserver(observer SecretAccessObserver) ResolverOption {
	return func(r *dynamicResourceResolver) { r.secretObserver = observer }
}

// WithMetadataClient enables Kubernetes PartialObjectMetadata list requests.
// Production wiring uses this path so large resource bodies are not fetched at
// all for bulk list tools. A dynamic-client fallback still applies the same
// server-side page limit and metadata projection for test and degraded paths.
func WithMetadataClient(client metadata.Interface) ResolverOption {
	return func(r *dynamicResourceResolver) { r.metadataClient = client }
}

// NewDynamicResolver creates a ResourceResolver backed by the Kubernetes dynamic client.
// The kindIndex maps lowercase kind strings to canonical GroupKind values for
// case-insensitive resolution (e.g. "replicaset" -> {Group:"apps", Kind:"ReplicaSet"}).
func NewDynamicResolver(client dynamic.Interface, mapper meta.RESTMapper, kindIndex map[string]schema.GroupKind, logger logr.Logger, opts ...ResolverOption) ResourceResolver {
	r := &dynamicResourceResolver{client: client, mapper: mapper, kindIndex: kindIndex, logger: logger}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// notifySecretAccess invokes the configured SecretAccessObserver only when
// mapping resolves to the core ("") group's "secrets" resource — resolved via
// the RESTMapper rather than string-matching the caller-supplied kind, so a
// differently-cased/aliased kind argument or a same-named CRD in another API
// group cannot spoof (or evade) the audit hook.
func (r *dynamicResourceResolver) notifySecretAccess(ctx context.Context, mapping *meta.RESTMapping, verb, name, namespace string, err error) {
	if r.secretObserver == nil || mapping == nil {
		return
	}
	if mapping.Resource.Group != "" || mapping.Resource.Resource != "secrets" {
		return
	}
	r.secretObserver(ctx, verb, name, namespace, err)
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

// resolveMappings returns RESTMappings for a kind, supporting multi-group kinds
// like Subscription (operators.coreos.com + messaging.knative.dev) and
// multi-version kinds like AuthorizationPolicy (security.istio.io/v1beta1 + v1).
//
// When apiGroup is non-empty, only mappings matching that group are returned.
// When apiGroup is empty and only one group matches, it is used automatically.
// When apiGroup is empty and multiple groups match, an error listing the
// available groups is returned so the caller (LLM) can retry with disambiguation (#1311).
//
// Uses ResourcesFor to discover all API groups, with resettableMapper retry.
// Falls back to RESTMappings(GroupKind) when ResourcesFor fails entirely.
// Issue #1064 follow-up: RESTMappings (plural) avoids AmbiguousKindError.
func (r *dynamicResourceResolver) resolveMappings(kind, apiGroup string) ([]*meta.RESTMapping, error) {
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
			return filterByAPIGroup(fallbackMappings, kind, apiGroup)
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
	return filterByAPIGroup(mappings, kind, apiGroup)
}

// filterByAPIGroup narrows mappings by the requested API group. When apiGroup
// is empty and only one distinct group exists, the kind is unambiguous.
// When multiple groups exist and no apiGroup is specified, an actionable error
// is returned listing the available groups so the LLM can retry (#1311).
func filterByAPIGroup(mappings []*meta.RESTMapping, kind, apiGroup string) ([]*meta.RESTMapping, error) {
	if apiGroup != "" {
		var filtered []*meta.RESTMapping
		for _, m := range mappings {
			if m.Resource.Group == apiGroup {
				filtered = append(filtered, m)
			}
		}
		if len(filtered) == 0 {
			return nil, fmt.Errorf("kind %q not found in API group %q", kind, apiGroup)
		}
		return filtered, nil
	}

	groups := distinctGroups(mappings)
	if len(groups) <= 1 {
		return mappings, nil
	}

	quotedGroups := make([]string, len(groups))
	for i, g := range groups {
		if g == "" {
			quotedGroups[i] = `"" (core API)`
		} else {
			quotedGroups[i] = `"` + g + `"`
		}
	}
	return nil, fmt.Errorf(
		"kind %q is ambiguous — it exists in %d API groups: %s. "+
			"Retry with api_group set to one of these values",
		kind, len(groups), strings.Join(quotedGroups, ", "))
}

func distinctGroups(mappings []*meta.RESTMapping) []string {
	seen := make(map[string]struct{})
	var groups []string
	for _, m := range mappings {
		g := m.Resource.Group
		if _, ok := seen[g]; !ok {
			seen[g] = struct{}{}
			groups = append(groups, g)
		}
	}
	return groups
}

func (r *dynamicResourceResolver) scopedClient(mapping *meta.RESTMapping, namespace string) dynamic.ResourceInterface {
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		return r.client.Resource(mapping.Resource).Namespace(namespace)
	}
	return r.client.Resource(mapping.Resource)
}

func (r *dynamicResourceResolver) Get(ctx context.Context, kind, name, namespace, apiGroup string) (interface{}, error) {
	mappings, err := r.resolveMappings(kind, apiGroup)
	if err != nil {
		return nil, err
	}

	var lastErr error
	var lastMapping *meta.RESTMapping
	for _, mapping := range mappings {
		lastMapping = mapping
		obj, getErr := r.scopedClient(mapping, namespace).Get(ctx, name, metav1.GetOptions{})
		if getErr == nil {
			if len(mappings) > 1 {
				r.logger.V(1).Info("multi-group kind resolved",
					"kind", kind, "name", name, "namespace", namespace,
					"api_group", mapping.Resource.Group,
					"gvr", mapping.Resource.String(),
					"result", "success")
			}
			r.notifySecretAccess(ctx, mapping, "get", name, namespace, nil)
			return obj, nil
		}
		lastErr = getErr
		if !errors.IsNotFound(getErr) && !errors.IsForbidden(getErr) {
			r.notifySecretAccess(ctx, mapping, "get", name, namespace, getErr)
			return nil, fmt.Errorf("get %s/%s in %s: %w", kind, name, namespace, getErr)
		}
	}
	r.notifySecretAccess(ctx, lastMapping, "get", name, namespace, lastErr)
	return nil, fmt.Errorf("get %s/%s in %s: %w", kind, name, namespace, lastErr)
}

// GetByNameInCluster resolves one exact-name resource with a server-side field
// selector and a one-item limit. It never lists an unrestricted collection and
// returns the full body only for the single targeted object requested by the
// caller.
func (r *dynamicResourceResolver) GetByNameInCluster(ctx context.Context, kind, name, apiGroup string) (interface{}, error) {
	mappings, err := r.resolveMappings(kind, apiGroup)
	if err != nil {
		return nil, err
	}
	options := metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", name).String(),
		Limit:         1,
	}
	var lastErr error
	var lastMapping *meta.RESTMapping
	for _, mapping := range mappings {
		lastMapping = mapping
		result, listErr := r.scopedClient(mapping, "").List(ctx, options)
		if listErr != nil {
			lastErr = listErr
			if !errors.IsNotFound(listErr) && !errors.IsForbidden(listErr) {
				r.notifySecretAccess(ctx, mapping, "list", "", "", listErr)
				return nil, fmt.Errorf("find %s/%s in cluster: %w", kind, name, listErr)
			}
			continue
		}
		if len(result.Items) == 0 {
			continue
		}
		if result.Items[0].GetName() != name {
			continue
		}
		r.notifySecretAccess(ctx, mapping, "list", "", "", nil)
		return &result.Items[0], nil
	}
	if lastErr != nil {
		r.notifySecretAccess(ctx, lastMapping, "list", "", "", lastErr)
		return nil, fmt.Errorf("resource %q named %q not found in cluster: %w", kind, name, lastErr)
	}
	err = fmt.Errorf("resource %q named %q not found in cluster", kind, name)
	r.notifySecretAccess(ctx, lastMapping, "list", "", "", err)
	return nil, err
}

// ListForQuery is the metadata-only query path for kubernetes_jq_query. Keeping
// jq on the same bounded summary contract prevents an expression such as
// `.items[].data` from fetching bulk ConfigMap/Secret bodies before the generic
// sanitizer or audit pipeline.
func (r *dynamicResourceResolver) ListForQuery(ctx context.Context, kind, namespace, apiGroup string) (interface{}, error) {
	if r.metadataClient == nil {
		return nil, fmt.Errorf("metadata-only query unavailable: metadata client not configured")
	}
	mappings, err := r.resolveMappings(kind, apiGroup)
	if err != nil {
		return nil, err
	}
	var lastSummary *ResourceListSummary
	var lastMapping *meta.RESTMapping
	for _, mapping := range mappings {
		lastMapping = mapping
		metadataList, listErr := r.listMetadata(ctx, mapping, namespace)
		if listErr != nil {
			if errors.IsNotFound(listErr) || errors.IsForbidden(listErr) {
				continue
			}
			r.notifySecretAccess(ctx, mapping, "list", "", namespace, listErr)
			return nil, fmt.Errorf("metadata-only list %s in %s: %w", kind, namespace, listErr)
		}
		summary := summarizeMetadataList(kind, metadataList)
		lastSummary = summary
		if summary.ItemCount > 0 {
			r.notifySecretAccess(ctx, mapping, "list", "", namespace, nil)
			return summary, nil
		}
	}
	if lastSummary != nil {
		r.notifySecretAccess(ctx, lastMapping, "list", "", namespace, nil)
		return lastSummary, nil
	}
	err = fmt.Errorf("metadata-only list %s in %s: no results from any API group", kind, namespace)
	r.notifySecretAccess(ctx, lastMapping, "list", "", namespace, err)
	return nil, err
}

// List returns a metadata-only summary for a bounded generic Kubernetes list.
func (r *dynamicResourceResolver) List(ctx context.Context, kind, namespace, apiGroup string) (interface{}, error) {
	mappings, err := r.resolveMappings(kind, apiGroup)
	if err != nil {
		return nil, err
	}

	var lastResult interface{}
	var lastMapping *meta.RESTMapping
	for _, mapping := range mappings {
		lastMapping = mapping
		result, listErr := r.listBoundedSummary(ctx, mapping, kind, namespace)
		if listErr != nil {
			if !errors.IsNotFound(listErr) && !errors.IsForbidden(listErr) {
				r.notifySecretAccess(ctx, mapping, "list", "", namespace, listErr)
				return nil, fmt.Errorf("list %s in %s: %w", kind, namespace, listErr)
			}
			continue
		}
		lastResult = result
		if result.ItemCount > 0 {
			if len(mappings) > 1 {
				r.logger.V(1).Info("multi-group kind resolved",
					"kind", kind, "namespace", namespace,
					"api_group", mapping.Resource.Group,
					"gvr", mapping.Resource.String(),
					"result", "success",
					"items", result.ItemCount)
			}
			r.notifySecretAccess(ctx, mapping, "list", "", namespace, nil)
			return result, nil
		}
	}
	if lastResult != nil {
		r.notifySecretAccess(ctx, lastMapping, "list", "", namespace, nil)
		return lastResult, nil
	}
	err = fmt.Errorf("list %s in %s: no results from any API group", kind, namespace)
	r.notifySecretAccess(ctx, lastMapping, "list", "", namespace, err)
	return nil, err
}

// listBoundedSummary chooses the metadata API when available and retains a
// bounded dynamic fallback only for unsupported metadata representations.
func (r *dynamicResourceResolver) listBoundedSummary(ctx context.Context, mapping *meta.RESTMapping, kind, namespace string) (*ResourceListSummary, error) {
	if r.metadataClient == nil {
		return r.listDynamicSummary(ctx, mapping, kind, namespace)
	}
	metadataList, err := r.listMetadata(ctx, mapping, namespace)
	if err != nil {
		// Some aggregated APIs do not serve PartialObjectMetadata. Fall back
		// to one bounded dynamic list so the tool remains usable; production
		// still never requests an unrestricted collection.
		if !errors.IsNotFound(err) && !errors.IsMethodNotSupported(err) && !errors.IsNotAcceptable(err) {
			return nil, err
		}
		r.logger.V(1).Info("metadata list unavailable, using bounded dynamic list",
			"gvr", mapping.Resource.String(), "error", err.Error())
		return r.listDynamicSummary(ctx, mapping, kind, namespace)
	}
	return summarizeMetadataList(kind, metadataList), nil
}

// listMetadata sends a namespace-aware or cluster-scoped metadata request.
func (r *dynamicResourceResolver) listMetadata(ctx context.Context, mapping *meta.RESTMapping, namespace string) (*metav1.PartialObjectMetadataList, error) {
	resource := r.metadataClient.Resource(mapping.Resource)
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		return resource.Namespace(namespace).List(ctx, metav1.ListOptions{Limit: DefaultResourceListLimit})
	}
	return resource.List(ctx, metav1.ListOptions{Limit: DefaultResourceListLimit})
}

// listDynamicSummary projects a bounded dynamic list when metadata negotiation
// is unsupported by the target API.
func (r *dynamicResourceResolver) listDynamicSummary(ctx context.Context, mapping *meta.RESTMapping, kind, namespace string) (*ResourceListSummary, error) {
	resourceList, err := r.scopedClient(mapping, namespace).List(ctx, metav1.ListOptions{Limit: DefaultResourceListLimit})
	if err != nil {
		return nil, err
	}
	return summarizeUnstructuredList(kind, resourceList), nil
}

// summarizeMetadataList preserves metadata and a safe server-provided count.
func summarizeMetadataList(kind string, list *metav1.PartialObjectMetadataList) *ResourceListSummary {
	items := make([]ResourceMetadataSummary, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, metadataSummary(item.Name, item.Namespace, string(item.UID), item.ResourceVersion, item.Labels))
	}
	if len(items) > int(DefaultResourceListLimit) {
		items = items[:DefaultResourceListLimit]
	}
	var totalCount *int
	if list.RemainingItemCount != nil {
		total := len(list.Items) + int(*list.RemainingItemCount)
		totalCount = &total
	} else if list.Continue == "" {
		total := len(list.Items)
		totalCount = &total
	}
	return &ResourceListSummary{
		APIVersion: list.APIVersion,
		Kind:       kind + "List",
		Items:      items,
		ItemCount:  len(items),
		TotalCount: totalCount,
		Truncated:  list.Continue != "" || len(list.Items) > len(items),
	}
}

// summarizeUnstructuredList strips bodies from the bounded dynamic fallback.
func summarizeUnstructuredList(kind string, list *unstructured.UnstructuredList) *ResourceListSummary {
	items := make([]ResourceMetadataSummary, 0, min(len(list.Items), int(DefaultResourceListLimit)))
	for _, item := range list.Items {
		items = append(items, metadataSummary(item.GetName(), item.GetNamespace(), string(item.GetUID()), item.GetResourceVersion(), item.GetLabels()))
		if len(items) == int(DefaultResourceListLimit) {
			break
		}
	}
	var totalCount *int
	if remaining := list.GetRemainingItemCount(); remaining != nil {
		total := len(list.Items) + int(*remaining)
		totalCount = &total
	} else if list.GetContinue() == "" {
		total := len(list.Items)
		totalCount = &total
	}
	return &ResourceListSummary{
		APIVersion: list.GetAPIVersion(),
		Kind:       kind + "List",
		Items:      items,
		ItemCount:  len(items),
		TotalCount: totalCount,
		Truncated:  list.GetContinue() != "" || len(list.Items) > len(items),
	}
}

// metadataSummary builds the allow-listed metadata projection for one item.
func metadataSummary(name, namespace, uid, resourceVersion string, labels map[string]string) ResourceMetadataSummary {
	return ResourceMetadataSummary{
		Metadata: ResourceObjectMetadata{
			Name: name, Namespace: namespace, UID: uid, ResourceVersion: resourceVersion,
			Labels: boundedLabels(labels),
		},
	}
}

// boundedLabels keeps label metadata useful without allowing unbounded values.
func boundedLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 32 {
		keys = keys[:32]
	}
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		value := labels[key]
		if len(value) > 256 {
			value = value[:256]
		}
		result[key] = value
	}
	return result
}
