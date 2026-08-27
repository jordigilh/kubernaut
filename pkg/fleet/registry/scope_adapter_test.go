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

package registry_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// stubClusterRegistry is a minimal registry.ClusterRegistry test double for
// exercising scope_adapter.go's adapters in isolation from any real
// watcher/informer.
type stubClusterRegistry struct {
	clusters map[string]registry.ClusterInfo
}

var _ registry.ClusterRegistry = (*stubClusterRegistry)(nil)

func (s *stubClusterRegistry) List() []registry.ClusterInfo {
	result := make([]registry.ClusterInfo, 0, len(s.clusters))
	for _, c := range s.clusters {
		result = append(result, c)
	}
	return result
}

func (s *stubClusterRegistry) Get(id string) (registry.ClusterInfo, bool) {
	info, ok := s.clusters[id]
	return info, ok
}

func (s *stubClusterRegistry) WatchClusters() <-chan registry.ClusterEvent { return nil }
func (s *stubClusterRegistry) Ready() bool                                { return true }
func (s *stubClusterRegistry) Start(_ context.Context) error              { return nil }
func (s *stubClusterRegistry) Stop()                                      {}

// #2300 Gap 3: scope_adapter.go's adapters had 0% UT coverage despite being
// the server-side access-enforcement point for cluster-scoped tool calls
// (ASVS V1.4.1: enforced at a trusted server-side point). The not-found
// branches specifically prove ASVS V4.1.3 (deny by default for unrecognized
// principals) actually holds, rather than being an untested assumption.
var _ = Describe("ClusterLookupAdapter (BR-INTEGRATION-065)", func() {
	var reg *stubClusterRegistry

	BeforeEach(func() {
		reg = &stubClusterRegistry{clusters: map[string]registry.ClusterInfo{
			"prod-east": {ID: "prod-east", ToolPrefix: "prod_east_"},
		}}
	})

	It("UT-REG-ADAPT-001 [AC-3]: IsKnownCluster returns true for a cluster tracked by the registry", func() {
		adapter := registry.NewClusterLookupAdapter(reg)
		Expect(adapter.IsKnownCluster("prod-east")).To(BeTrue())
	})

	It("UT-REG-ADAPT-002 [V4.1.3, AC-3]: IsKnownCluster returns false (deny-by-default) for an unknown cluster", func() {
		adapter := registry.NewClusterLookupAdapter(reg)
		Expect(adapter.IsKnownCluster("no-such-cluster")).To(BeFalse())
	})
})

var _ = Describe("ToolPrefixAdapter (BR-INTEGRATION-065)", func() {
	var reg *stubClusterRegistry

	BeforeEach(func() {
		reg = &stubClusterRegistry{clusters: map[string]registry.ClusterInfo{
			"prod-east": {ID: "prod-east", ToolPrefix: "prod_east_"},
		}}
	})

	It("UT-REG-ADAPT-003 [AC-3]: ToolPrefixFor returns the registry's ToolPrefix for a known cluster", func() {
		adapter := registry.NewToolPrefixAdapter(reg)
		Expect(adapter.ToolPrefixFor("prod-east")).To(Equal("prod_east_"))
	})

	It("UT-REG-ADAPT-004 [V4.1.3, AC-3]: ToolPrefixFor returns empty string (deny-by-default) for an unknown cluster", func() {
		adapter := registry.NewToolPrefixAdapter(reg)
		Expect(adapter.ToolPrefixFor("no-such-cluster")).To(Equal(""))
	})
})
