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

package registry

// ClusterLookupAdapter adapts a ClusterRegistry to the fleet.ClusterLookup
// interface used by FederatedScopeChecker for cluster-level precondition checks.
type ClusterLookupAdapter struct {
	registry ClusterRegistry
}

// NewClusterLookupAdapter wraps a ClusterRegistry as a fleet.ClusterLookup.
func NewClusterLookupAdapter(registry ClusterRegistry) *ClusterLookupAdapter {
	return &ClusterLookupAdapter{registry: registry}
}

// IsKnownCluster returns true if the cluster is known to the registry.
func (a *ClusterLookupAdapter) IsKnownCluster(clusterID string) bool {
	_, found := a.registry.Get(clusterID)
	return found
}

// ToolPrefixAdapter adapts a ClusterRegistry to ToolPrefixResolver,
// returning the ToolPrefix stored in ClusterInfo for a given cluster ID.
type ToolPrefixAdapter struct {
	registry ClusterRegistry
}

// Compile-time interface compliance.
var _ ToolPrefixResolver = (*ToolPrefixAdapter)(nil)

// NewToolPrefixAdapter wraps a ClusterRegistry as a ToolPrefixResolver.
func NewToolPrefixAdapter(registry ClusterRegistry) *ToolPrefixAdapter {
	return &ToolPrefixAdapter{registry: registry}
}

// ToolPrefixFor returns the ToolPrefix for the given cluster, or empty if unknown.
func (a *ToolPrefixAdapter) ToolPrefixFor(clusterID string) string {
	info, found := a.registry.Get(clusterID)
	if !found {
		return ""
	}
	return info.ToolPrefix
}
