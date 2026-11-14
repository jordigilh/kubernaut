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

package fmc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
)

// InventoryPath is FMC's typed, sanitized managed-resource projection.
const InventoryPath = "/api/v1/inventory/managed-resources"

// ListManagedResources reads the canonical inventory projection from FMC.
// It never falls back to a local Kubernetes reader when FMC is unavailable.
func (c *HTTPClient) ListManagedResources(ctx context.Context, filters inventory.Filters) (inventory.Page, error) {
	normalized, err := filters.Normalize()
	if err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), err
	}

	query := url.Values{}
	query.Set("cluster_id", normalized.ClusterID)
	query.Set("limit", strconv.Itoa(normalized.Limit))
	if normalized.Namespace != "" {
		query.Set("namespace", normalized.Namespace)
	}
	if normalized.Group != "" {
		query.Set("group", normalized.Group)
	}
	if normalized.Version != "" {
		query.Set("version", normalized.Version)
	}
	if normalized.Kind != "" {
		query.Set("kind", normalized.Kind)
	}
	if normalized.Name != "" {
		query.Set("name", normalized.Name)
	}
	if normalized.ResourceID != "" {
		query.Set("resource_id", normalized.ResourceID)
	}
	if normalized.Cursor != "" {
		query.Set("cursor", normalized.Cursor)
	}

	page, err := getInventory[inventory.Page](ctx, c, InventoryPath+"?"+query.Encode())
	if err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), err
	}
	if err := page.Validate(); err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), fmt.Errorf("validate FMC inventory page: %w", err)
	}
	return page, nil
}

// List implements the shared inventory.Reader contract used by AF Console.
func (c *HTTPClient) List(ctx context.Context, filters inventory.Filters) (inventory.Page, error) {
	return c.ListManagedResources(ctx, filters)
}

// GetManagedResource reads one canonical resource by its opaque ID.
func (c *HTTPClient) GetManagedResource(ctx context.Context, clusterID, resourceID string) (inventory.Resource, error) {
	if err := inventory.ValidateResourceID(resourceID); err != nil {
		return inventory.Resource{}, err
	}

	if _, err := (inventory.Filters{ClusterID: clusterID}).Normalize(); err != nil {
		return inventory.Resource{}, err
	}
	query := url.Values{"cluster_id": []string{clusterID}}
	resource, err := getInventory[inventory.Resource](ctx, c, InventoryPath+"/"+url.PathEscape(resourceID)+"?"+query.Encode())
	if err != nil {
		return inventory.Resource{}, err
	}
	if resource.ID != resourceID || resource.ClusterID != clusterID {
		return inventory.Resource{}, fmt.Errorf("%w: FMC returned a mismatched resource identity", inventory.ErrUnavailable)
	}
	if err := resource.Validate(); err != nil {
		return inventory.Resource{}, fmt.Errorf("%w: FMC returned an invalid resource identity: %w", inventory.ErrUnavailable, err)
	}
	return resource, nil
}

// Get implements the shared inventory.Reader contract used by AF Console.
func (c *HTTPClient) Get(ctx context.Context, clusterID, resourceID string) (inventory.Resource, error) {
	return c.GetManagedResource(ctx, clusterID, resourceID)
}

func getInventory[T inventory.Page | inventory.Resource](ctx context.Context, c *HTTPClient, path string) (T, error) {
	var target T
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return target, fmt.Errorf("build FMC inventory request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return target, fmt.Errorf("%w: FMC inventory request failed: %w", inventory.ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return target, inventory.ErrNotFound
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return target, fmt.Errorf("%w: FMC inventory authorization failed", inventory.ErrUnavailable)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return target, fmt.Errorf("%w: FMC inventory returned status %d", inventory.ErrUnavailable, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&target); err != nil {
		return target, fmt.Errorf("%w: decode FMC inventory response: %w", inventory.ErrUnavailable, err)
	}
	return target, nil
}
