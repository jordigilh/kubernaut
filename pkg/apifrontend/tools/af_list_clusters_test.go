package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// stubClusterRegistry implements registry.ClusterRegistry for testing.
type stubClusterRegistry struct {
	clusters []registry.ClusterInfo
}

func (s *stubClusterRegistry) List() []registry.ClusterInfo    { return s.clusters }
func (s *stubClusterRegistry) Get(id string) (registry.ClusterInfo, bool) {
	for _, c := range s.clusters {
		if c.ID == id {
			return c, true
		}
	}
	return registry.ClusterInfo{}, false
}
func (s *stubClusterRegistry) WatchClusters() <-chan registry.ClusterEvent { return nil }
func (s *stubClusterRegistry) Ready() bool                                { return true }
func (s *stubClusterRegistry) Start(_ context.Context) error              { return nil }
func (s *stubClusterRegistry) Stop()                                      {}

var _ registry.ClusterRegistry = (*stubClusterRegistry)(nil)

var _ = Describe("list_clusters tool [BR-FLEET-054, AC-3]", func() {

	It("UT-AF-054-LC-001: returns all registered clusters", func() {
		reg := &stubClusterRegistry{
			clusters: []registry.ClusterInfo{
				{ID: "cluster-east", Name: "Production US-East"},
				{ID: "cluster-west", Name: "Production US-West"},
			},
		}

		result, err := tools.HandleListClusters(context.Background(), reg)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(2))
		Expect(result.Clusters).To(HaveLen(2))
		Expect(result.Clusters[0].ID).To(Equal("cluster-east"))
		Expect(result.Clusters[0].Name).To(Equal("Production US-East"))
		Expect(result.Clusters[1].ID).To(Equal("cluster-west"))
		Expect(result.Clusters[1].Name).To(Equal("Production US-West"))
	})

	It("UT-AF-054-LC-002: returns empty list when no clusters registered", func() {
		reg := &stubClusterRegistry{clusters: nil}

		result, err := tools.HandleListClusters(context.Background(), reg)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
		Expect(result.Clusters).To(BeEmpty())
	})

	It("UT-AF-054-LC-003: nil registry returns error", func() {
		_, err := tools.HandleListClusters(context.Background(), nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("fleet management is not configured"))
	})
})
