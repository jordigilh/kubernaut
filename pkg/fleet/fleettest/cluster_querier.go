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

package fleettest

import (
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// StubClusterQuerier implements registry.ClusterQuerier for unit and
// integration tests. Configure Clusters to map cluster IDs to ClusterInfo;
// an ID absent from the map behaves like an unregistered cluster (comma-ok
// false), exercising the graceful-degradation path (BR-FLEET-003, #1511).
type StubClusterQuerier struct {
	Clusters map[string]registry.ClusterInfo
}

// List returns all configured clusters.
func (s *StubClusterQuerier) List() []registry.ClusterInfo {
	out := make([]registry.ClusterInfo, 0, len(s.Clusters))
	for _, info := range s.Clusters {
		out = append(out, info)
	}
	return out
}

// Get returns the configured ClusterInfo for clusterID, or false if absent.
func (s *StubClusterQuerier) Get(clusterID string) (registry.ClusterInfo, bool) {
	info, ok := s.Clusters[clusterID]
	return info, ok
}

// WatchClusters returns a closed channel -- no events are ever emitted by this stub.
func (s *StubClusterQuerier) WatchClusters() <-chan registry.ClusterEvent {
	ch := make(chan registry.ClusterEvent)
	close(ch)
	return ch
}

// Ready always reports true for this stub.
func (s *StubClusterQuerier) Ready() bool {
	return true
}

var _ registry.ClusterQuerier = (*StubClusterQuerier)(nil)
