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

package acm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// ListManagedResources returns ACM Search results as the canonical sanitized
// inventory model. Only allowlisted identity and managed-label fields are
// copied out of ACM's unstructured Map scalar.
func (c *Client) ListManagedResources(ctx context.Context, filters inventory.Filters) (inventory.Page, error) {
	normalized, err := filters.Normalize()
	if err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), err
	}
	page, err := c.listScopedInventory(ctx, normalized)
	if errors.Is(err, errInventoryPartial) {
		return inventory.StateOnly(inventory.StatePartial), nil
	}
	return page, err
}

// List implements the shared inventory.Reader contract used by AF Console.
func (c *Client) List(ctx context.Context, filters inventory.Filters) (inventory.Page, error) {
	return c.ListManagedResources(ctx, filters)
}

// GetManagedResource resolves one opaque resource ID through the same ACM
// Search projection used for list reads. Canonical IDs are one-way hashes, so
// the adapter compares them locally after a bounded inventory traversal.
func (c *Client) GetManagedResource(ctx context.Context, clusterID, resourceID string) (inventory.Resource, error) {
	if err := inventory.ValidateResourceID(resourceID); err != nil {
		return inventory.Resource{}, err
	}
	if _, err := (inventory.Filters{ClusterID: clusterID}).Normalize(); err != nil {
		return inventory.Resource{}, err
	}
	page, err := c.ListManagedResources(ctx, inventory.Filters{ClusterID: clusterID, ResourceID: resourceID, Limit: 1})
	if err != nil {
		return inventory.Resource{}, err
	}
	if page.State != inventory.StateSuccess && page.State != inventory.StateEmpty {
		return inventory.Resource{}, inventory.ErrUnavailable
	}
	if len(page.Items) == 0 {
		return inventory.Resource{}, inventory.ErrNotFound
	}
	if page.Items[0].ID != resourceID {
		return inventory.Resource{}, fmt.Errorf("%w: ACM returned a mismatched resource ID", inventory.ErrUnavailable)
	}
	return page.Items[0], nil
}

// Get implements the shared inventory.Reader contract used by AF Console.
func (c *Client) Get(ctx context.Context, clusterID, resourceID string) (inventory.Resource, error) {
	return c.GetManagedResource(ctx, clusterID, resourceID)
}

// Each Search query is bounded. A full batch is ambiguous on ACM versions
// whose count reflects only the returned items, so it fails closed.
const (
	inventorySearchBound = 1000
	namespaceKind        = "Namespace"
)

var errInventoryPartial = errors.New("ACM inventory traversal incomplete")

func (c *Client) listScopedInventory(ctx context.Context, filters inventory.Filters) (inventory.Page, error) {
	base := searchOptions{clusterID: filters.ClusterID, limit: inventorySearchBound}
	namespaces, err := c.managedInventoryNamespaces(ctx, base, filters.Namespace)
	if err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), err
	}
	resources, err := c.explicitInventoryResources(ctx, base, filters, namespaces)
	if err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), err
	}
	if err := c.collectNamespaceResources(ctx, base, filters, namespaces, resources); err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), err
	}
	return inventoryResultPage(resources, namespaces, filters)
}

func (c *Client) managedInventoryNamespaces(ctx context.Context, base searchOptions, namespace string) (map[string]inventory.LabelState, error) {
	query := base
	query.kind, query.managed = namespaceKind, true
	query.name = namespace
	namespaces, err := c.boundedInventorySearch(ctx, query)
	if err != nil {
		return nil, err
	}
	labels := make(map[string]inventory.LabelState, len(namespaces))
	for _, namespace := range namespaces {
		if namespace.Kind != namespaceKind || namespace.Namespace != "" || namespace.Scope.ResourceLabel != inventory.LabelTrue {
			return nil, fmt.Errorf("%w: ACM returned an invalid namespace", inventory.ErrUnavailable)
		}
		if _, exists := labels[namespace.Name]; exists {
			return nil, fmt.Errorf("%w: ACM returned duplicate namespaces", inventory.ErrUnavailable)
		}
		labels[namespace.Name] = namespace.Scope.ResourceLabel
	}
	return labels, nil
}

func (c *Client) explicitInventoryResources(ctx context.Context, base searchOptions, filters inventory.Filters, labels map[string]inventory.LabelState) (map[string]inventory.Resource, error) {
	query := inventorySearchFilters(base, filters)
	query.managed = true
	explicit, err := c.boundedInventorySearch(ctx, query)
	if err != nil {
		return nil, err
	}
	resources := make(map[string]inventory.Resource, len(explicit))
	for _, resource := range explicit {
		if resource.Scope.ResourceLabel != inventory.LabelTrue {
			return nil, fmt.Errorf("%w: ACM returned a non-matching managed resource", inventory.ErrUnavailable)
		}
		if _, exists := resources[resource.ID]; exists {
			return nil, fmt.Errorf("%w: ACM returned duplicate managed resources", inventory.ErrUnavailable)
		}
		resources[resource.ID] = resource
		if resource.Namespace == "" || scope.IsClusterScopedKind(resource.Kind) {
			continue
		}
		if _, exists := labels[resource.Namespace]; exists {
			continue
		}
		lookup := base
		lookup.kind, lookup.name = namespaceKind, resource.Namespace
		matches, searchErr := c.boundedInventorySearch(ctx, lookup)
		if searchErr != nil {
			return nil, searchErr
		}
		if len(matches) != 1 || matches[0].Kind != namespaceKind || matches[0].Name != resource.Namespace || matches[0].Namespace != "" {
			return nil, fmt.Errorf("%w: ACM namespace is not uniquely visible", inventory.ErrUnavailable)
		}
		labels[resource.Namespace] = matches[0].Scope.ResourceLabel
	}
	return resources, nil
}

func (c *Client) collectNamespaceResources(ctx context.Context, base searchOptions, filters inventory.Filters, labels map[string]inventory.LabelState, resources map[string]inventory.Resource) error {
	for name, state := range labels {
		if state != inventory.LabelTrue || (filters.Namespace != "" && filters.Namespace != name) {
			continue
		}
		query := inventorySearchFilters(base, filters)
		query.namespace = name
		items, err := c.boundedInventorySearch(ctx, query)
		if err != nil {
			return err
		}
		for _, resource := range items {
			if resource.Namespace != name {
				return fmt.Errorf("%w: ACM returned a mismatched namespace", inventory.ErrUnavailable)
			}
			if previous, exists := resources[resource.ID]; exists && previous.Scope.ResourceLabel != resource.Scope.ResourceLabel {
				return fmt.Errorf("%w: ACM returned inconsistent resource labels", inventory.ErrUnavailable)
			}
			resources[resource.ID] = resource
		}
		if len(resources) >= inventorySearchBound {
			return errInventoryPartial
		}
	}
	return nil
}

func inventorySearchFilters(base searchOptions, filters inventory.Filters) searchOptions {
	base.namespace = filters.Namespace
	base.group = filters.Group
	base.version = filters.Version
	base.kind = filters.Kind
	base.name = filters.Name
	return base
}

func inventoryResultPage(resources map[string]inventory.Resource, namespaces map[string]inventory.LabelState, filters inventory.Filters) (inventory.Page, error) {
	items := make([]inventory.Resource, 0, len(resources))
	for _, resource := range resources {
		clusterScoped := resource.Namespace == "" || scope.IsClusterScopedKind(resource.Kind)
		namespaceState := inventory.LabelAbsent
		if !clusterScoped {
			var exists bool
			namespaceState, exists = namespaces[resource.Namespace]
			if !exists {
				return inventory.StateOnly(inventory.StateUnavailable), fmt.Errorf("%w: ACM namespace is not visible", inventory.ErrUnavailable)
			}
		}
		resolution := scope.ResolveLabels(labelsForState(resource.Scope.ResourceLabel), labelsForState(namespaceState), clusterScoped)
		resource.Scope = inventory.Scope{Included: resolution.Included, ResourceLabel: inventory.LabelState(resolution.ResourceLabel), NamespaceLabel: inventory.LabelState(resolution.NamespaceLabel), Source: inventory.ScopeSource(resolution.Source), ClusterScoped: resolution.ClusterScoped}
		if resolution.Included && matchesInventoryFilters(resource, filters) {
			items = append(items, resource)
		}
	}
	inventory.SortResources(items)
	if filters.Cursor != "" {
		found := false
		for i := range items {
			if items[i].ID == filters.Cursor {
				items = items[i+1:]
				found = true
				break
			}
		}
		if !found {
			return inventory.StateOnly(inventory.StateUnavailable), fmt.Errorf("%w: cursor is not present in the current snapshot", inventory.ErrInvalidFilter)
		}
	}
	page := inventory.Page{State: inventory.StateSuccess, ObservedAt: time.Now().UTC(), Completeness: inventory.CompletenessComplete}
	switch {
	case len(items) == 0:
		page.State = inventory.StateEmpty
	case len(items) > filters.Limit:
		page.Items = items[:filters.Limit]
		page.NextCursor = page.Items[len(page.Items)-1].ID
	default:
		page.Items = items
	}
	return page, nil
}

func (c *Client) boundedInventorySearch(ctx context.Context, options searchOptions) ([]inventory.Resource, error) {
	page, err := c.searchInventory(ctx, options)
	if err != nil {
		return nil, err
	}
	if (page.State != inventory.StateSuccess && page.State != inventory.StateEmpty) || len(page.Items) >= options.limit {
		return nil, errInventoryPartial
	}
	return page.Items, nil
}

func labelsForState(state inventory.LabelState) map[string]string {
	switch state {
	case inventory.LabelTrue:
		return map[string]string{scope.ManagedLabelKey: "true"}
	case inventory.LabelFalse:
		return map[string]string{scope.ManagedLabelKey: "false"}
	case inventory.LabelInvalid:
		return map[string]string{scope.ManagedLabelKey: "invalid"}
	default:
		return nil
	}
}

func matchesInventoryFilters(resource inventory.Resource, filters inventory.Filters) bool {
	return (filters.ResourceID == "" || resource.ID == filters.ResourceID) &&
		(filters.Namespace == "" || resource.Namespace == filters.Namespace) &&
		(filters.Group == "" || resource.Group == filters.Group) &&
		(filters.Version == "" || resource.Version == filters.Version) &&
		(filters.Kind == "" || resource.Kind == filters.Kind) &&
		(filters.Name == "" || resource.Name == filters.Name)
}

type searchOptions struct {
	clusterID string
	limit     int
	managed   bool
	namespace string
	group     string
	version   string
	kind      string
	name      string
}

func (c *Client) searchInventory(ctx context.Context, options searchOptions) (inventory.Page, error) {
	body, err := buildSearchRequestBody(options)
	if err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), fmt.Errorf("%w: build ACM inventory request: %w", inventory.ErrUnavailable, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+graphQLPath, bytes.NewReader(body))
	if err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), fmt.Errorf("%w: build ACM inventory request: %w", inventory.ErrUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), fmt.Errorf("%w: ACM inventory request failed: %w", inventory.ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return inventory.StateOnly(inventory.StateUnavailable), fmt.Errorf("%w: ACM inventory authorization failed", inventory.ErrUnavailable)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return inventory.StateOnly(inventory.StateUnavailable), fmt.Errorf("%w: ACM inventory returned status %d", inventory.ErrUnavailable, resp.StatusCode)
	}

	var gqlResp graphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), fmt.Errorf("%w: decode ACM inventory response: %w", inventory.ErrUnavailable, err)
	}
	return pageFromSearchResponse(gqlResp, options.clusterID)
}

func buildSearchRequestBody(options searchOptions) ([]byte, error) {
	filters := make([]searchFilter, 0, 7)
	if options.managed {
		filters = append(filters, searchFilter{Property: "label", Values: []string{"kubernaut.ai/managed=true"}})
	}
	appendFilter := func(property, value string) {
		if value != "" {
			filters = append(filters, searchFilter{Property: property, Values: []string{value}})
		}
	}
	appendFilter("cluster", options.clusterID)
	appendFilter("namespace", options.namespace)
	appendFilter("apigroup", options.group)
	appendFilter("apiversion", options.version)
	appendFilter("kind", options.kind)
	appendFilter("name", options.name)

	reqBody := graphQLRequest{
		Query: SearchQuery,
		Variables: map[string]interface{}{
			"input": []searchInput{{Filters: filters, Limit: options.limit}},
		},
	}
	body, err := json.Marshal(reqBody)
	return body, err
}

func pageFromSearchResponse(gqlResp graphQLResponse, requestedClusterID string) (inventory.Page, error) {
	if len(gqlResp.Errors) > 0 || gqlResp.Data == nil || len(gqlResp.Data.SearchResult) == 0 {
		return inventory.StateOnly(inventory.StateUnavailable), fmt.Errorf("%w: ACM Search inventory query failed", inventory.ErrUnavailable)
	}
	result := gqlResp.Data.SearchResult[0]
	if result.Count > len(result.Items) {
		return inventory.StateOnly(inventory.StatePartial), nil
	}
	if result.Count < len(result.Items) {
		return inventory.StateOnly(inventory.StateUnavailable), fmt.Errorf("%w: ACM inventory count contradicts returned items", inventory.ErrUnavailable)
	}
	observedAt := time.Now().UTC()
	if result.Count == 0 || len(result.Items) == 0 {
		page := inventory.StateOnly(inventory.StateEmpty)
		page.ObservedAt = observedAt
		return page, nil
	}

	items := make([]inventory.Resource, 0, len(result.Items))
	for _, item := range result.Items {
		resource, err := resourceFromSearchItem(item, requestedClusterID, observedAt)
		if err != nil {
			return inventory.StateOnly(inventory.StateUnavailable), err
		}
		items = append(items, resource)
	}
	inventory.SortResources(items)
	page := inventory.Page{
		State:        inventory.StateSuccess,
		ObservedAt:   observedAt,
		Completeness: inventory.CompletenessComplete,
		Items:        items,
	}
	if err := page.Validate(); err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), fmt.Errorf("validate ACM inventory page: %w", err)
	}
	return page, nil
}

func resourceFromSearchItem(item map[string]json.RawMessage, requestedClusterID string, observedAt time.Time) (inventory.Resource, error) {
	clusterID := firstString(item, "cluster", "cluster_id")
	resource := inventory.Resource{
		ClusterID:  clusterID,
		Group:      firstString(item, "apigroup", "group"),
		Version:    firstString(item, "apiversion", "version"),
		Kind:       firstString(item, "kind"),
		Namespace:  firstString(item, "namespace"),
		Name:       firstString(item, "name"),
		ObservedAt: observedAt,
	}
	if resource.ClusterID == "" || resource.Kind == "" || resource.Name == "" || resource.Version == "" {
		return inventory.Resource{}, fmt.Errorf("%w: ACM inventory item is missing canonical identity", inventory.ErrUnavailable)
	}
	if resource.ClusterID != requestedClusterID {
		return inventory.Resource{}, fmt.Errorf("%w: ACM inventory item cluster does not match requested cluster", inventory.ErrUnavailable)
	}
	resource.ID = inventory.CanonicalResourceID(scope.ResourceIdentity{
		ClusterID: resource.ClusterID,
		Group:     resource.Group,
		Version:   resource.Version,
		Kind:      resource.Kind,
		Namespace: resource.Namespace,
		Name:      resource.Name,
	})
	if err := inventory.ValidateResourceID(resource.ID); err != nil {
		return inventory.Resource{}, fmt.Errorf("%w: ACM inventory item has invalid resource ID", inventory.ErrUnavailable)
	}
	resource.Scope = inventory.Scope{
		Included:       true,
		ResourceLabel:  managedLabelState(item),
		NamespaceLabel: labelState(item, "label.kubernaut.ai/managed.namespace", "label.namespace.kubernaut.ai/managed"),
		Source:         inventory.ScopeSourceResourceLabel,
		ClusterScoped:  resource.Namespace == "" || scope.IsClusterScopedKind(resource.Kind),
	}
	if resource.Scope.ResourceLabel != inventory.LabelTrue {
		resource.Scope.Included = false
		resource.Scope.Source = inventory.ScopeSourceNone
	}
	return resource, nil
}

func firstString(item map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		if raw, ok := item[key]; ok {
			var value string
			if json.Unmarshal(raw, &value) == nil {
				return value
			}
		}
	}
	return ""
}

func managedLabelState(item map[string]json.RawMessage) inventory.LabelState {
	if _, ok := item["label"]; ok {
		labels := searchManagedLabels(item)
		switch labels[scope.ManagedLabelKey] {
		case "true":
			return inventory.LabelTrue
		case "false":
			return inventory.LabelFalse
		case "":
			return inventory.LabelAbsent
		default:
			return inventory.LabelInvalid
		}
	}
	return labelState(item,
		"label.kubernaut.ai/managed",
		"labels.kubernaut.ai/managed",
		"label.kubernaut.ai~1managed",
	)
}

func labelState(item map[string]json.RawMessage, keys ...string) inventory.LabelState {
	value := firstString(item, keys...)
	switch value {
	case "true":
		return inventory.LabelTrue
	case "false":
		return inventory.LabelFalse
	case "":
		return inventory.LabelAbsent
	default:
		return inventory.LabelInvalid
	}
}
