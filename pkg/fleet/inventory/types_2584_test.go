package inventory_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
)

func TestInventoryContract(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Inventory Contract Suite")
}

var _ = Describe("BR-SCOPE-001 / UT-AF-2584-005/007: canonical inventory contract", func() {
	It("applies a bounded default limit and rejects a widening exact filter", func() {
		normalized, err := (inventory.Filters{ClusterID: "local"}).Normalize()
		Expect(err).NotTo(HaveOccurred())
		Expect(normalized.Limit).To(Equal(inventory.DefaultLimit))

		_, err = (inventory.Filters{ClusterID: "local", ResourceID: "r-opaque", Namespace: "payments"}).Normalize()
		Expect(err).To(MatchError(ContainSubstring("resource_id cannot be combined")))
	})

	DescribeTable("rejects unbounded or malformed caller input", func(filters inventory.Filters) {
		_, err := filters.Normalize()
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("invalid inventory filter")))
	},
		Entry("missing cluster", inventory.Filters{}),
		Entry("limit above maximum", inventory.Filters{ClusterID: "local", Limit: inventory.MaxLimit + 1}),
		Entry("invalid namespace", inventory.Filters{ClusterID: "local", Namespace: "payments?all"}),
		Entry("invalid cursor", inventory.Filters{ClusterID: "local", Cursor: "../page"}),
	)
})
