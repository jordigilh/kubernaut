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

// Package scopecache provides a Valkey-backed scope cache client for checking
// whether remote cluster resources are managed by Kubernaut.
// Used by GW and RO services for low-latency federated scope checking.
package scopecache

import (
	"context"
	"errors"
	"fmt"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// CacheReader abstracts the cache read operations for scope checking.
// Production implementation uses Valkey; tests use in-memory maps.
type CacheReader interface {
	// Exists checks if the given key exists in the cache.
	Exists(ctx context.Context, key string) (bool, error)
}

// Client provides scope checking against the Fleet Metadata Cache in Valkey.
// Key format: kubernaut:managed:{clusterID}:{group}/{version}/{kind}:{namespace}/{name}
type Client struct {
	reader CacheReader
}

// NewClient creates a scope cache client backed by the given CacheReader.
func NewClient(reader CacheReader) *Client {
	return &Client{reader: reader}
}

// IsManaged checks if a resource on a remote cluster is labeled kubernaut.ai/managed=true.
// Returns (true, nil) if managed, (false, nil) if not found (cache miss treated as unmanaged).
func (c *Client) IsManaged(ctx context.Context, clusterID, group, version, kind, namespace, name string) (bool, error) {
	key, err := BuildKey(clusterID, group, version, kind, namespace, name)
	if err != nil {
		return false, err
	}
	return c.reader.Exists(ctx, key)
}

// IsManagedResource implements scope.ScopeChecker for remote cluster scope checks.
// Delegates to the underlying IsManaged with fields extracted from ResourceIdentity.
func (c *Client) IsManagedResource(ctx context.Context, resource scope.ResourceIdentity) (bool, error) {
	return c.IsManaged(ctx, resource.ClusterID, resource.Group, resource.Version, resource.Kind, resource.Namespace, resource.Name)
}

// Compile-time verification that Client satisfies scope.ScopeChecker.
var _ scope.ScopeChecker = (*Client)(nil)

// ErrEmptyClusterID is returned when clusterID is empty.
var ErrEmptyClusterID = errors.New("scopecache: clusterID must not be empty")

// ErrEmptyKind is returned when kind is empty.
var ErrEmptyKind = errors.New("scopecache: kind must not be empty")

// ErrEmptyName is returned when name is empty.
var ErrEmptyName = errors.New("scopecache: name must not be empty")

// BuildKey constructs the Valkey key for a managed resource entry.
// Returns an error if required parameters (clusterID, kind, name) are empty.
func BuildKey(clusterID, group, version, kind, namespace, name string) (string, error) {
	if clusterID == "" {
		return "", ErrEmptyClusterID
	}
	if kind == "" {
		return "", ErrEmptyKind
	}
	if name == "" {
		return "", ErrEmptyName
	}
	gvr := fmt.Sprintf("%s/%s/%s", group, version, kind)
	nsName := fmt.Sprintf("%s/%s", namespace, name)
	return fmt.Sprintf("kubernaut:managed:%s:%s:%s", clusterID, gvr, nsName), nil
}
