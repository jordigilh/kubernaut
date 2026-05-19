package tools_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("DiscoveryCache", func() {
	It("UT-AF-WP-045: returns cached response within TTL", func() {
		cache := tools.NewDiscoveryCache(60 * time.Second)
		entry := &tools.DiscoverWorkflowsResult{
			Workflows: []tools.WorkflowDetail{{WorkflowID: "wf-1", Name: "Cached"}},
			Count:     1,
		}
		cache.Set("wf-1", entry)

		got, ok := cache.Get("wf-1")
		Expect(ok).To(BeTrue())
		Expect(got.Count).To(Equal(1))
		Expect(got.Workflows[0].Name).To(Equal("Cached"))
	})

	It("UT-AF-WP-046: evicts after TTL expiry", func() {
		cache := tools.NewDiscoveryCache(1 * time.Millisecond)
		entry := &tools.DiscoverWorkflowsResult{
			Workflows: []tools.WorkflowDetail{{WorkflowID: "wf-1"}},
			Count:     1,
		}
		cache.Set("wf-1", entry)

		time.Sleep(5 * time.Millisecond)

		_, ok := cache.Get("wf-1")
		Expect(ok).To(BeFalse())
	})
})
