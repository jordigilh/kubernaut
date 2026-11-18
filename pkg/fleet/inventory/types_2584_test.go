package inventory_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
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
		Entry("legacy resource ID", inventory.Filters{ClusterID: "local", ResourceID: "opaque-id"}),
	)

	It("rejects legacy and malformed resource IDs", func() {
		validID := "r-" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

		Expect(inventory.ValidateResourceID(validID)).To(Succeed())
		for _, resourceID := range []string{
			"opaque-id",
			"r-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde",
			"r-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0",
			"r-0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef",
		} {
			Expect(inventory.ValidateResourceID(resourceID)).To(MatchError(ContainSubstring("invalid inventory filter")))
		}
	})

	It("validates canonical identity before hashing", func() {
		identity := scope.ResourceIdentity{
			ClusterID: "prod-east",
			Group:     "apps",
			Version:   "v1",
			Kind:      "Deployment",
			Namespace: "payments",
			Name:      "checkout",
		}
		resourceID, err := inventory.CanonicalResourceIDChecked(identity)
		Expect(err).NotTo(HaveOccurred())
		Expect(resourceID).To(MatchRegexp(`^r-[0-9a-f]{64}$`))
		Expect(inventory.CanonicalResourceID(identity)).To(Equal(resourceID))

		for _, invalid := range []scope.ResourceIdentity{
			{ClusterID: "prod-east", Group: "apps", Kind: "Deployment", Namespace: "payments", Name: "checkout"},
			{ClusterID: "prod-east", Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "payments" + "\x00", Name: "checkout"},
			{ClusterID: "prod-east", Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "payments", Name: "checkout" + "/other"},
			{ClusterID: "prod-east", Group: "apps", Version: "v1", Kind: "Namespace", Namespace: "payments", Name: "cluster-config"},
		} {
			_, err := inventory.CanonicalResourceIDChecked(invalid)
			Expect(err).To(MatchError(ContainSubstring("invalid canonical resource identity")))
			Expect(inventory.CanonicalResourceID(invalid)).To(BeEmpty())
		}
	})

	Describe("Page.Validate", func() {
		validResource := func() inventory.Resource {
			identity := scope.ResourceIdentity{
				ClusterID: "prod-east",
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Namespace: "payments",
				Name:      "checkout",
			}
			return inventory.Resource{
				ID:        inventory.CanonicalResourceID(identity),
				ClusterID: identity.ClusterID,
				Group:     identity.Group,
				Version:   identity.Version,
				Kind:      identity.Kind,
				Namespace: identity.Namespace,
				Name:      identity.Name,
			}
		}

		It("requires complete success pages to contain validated items", func() {
			resource := validResource()
			page := inventory.Page{
				State:        inventory.StateSuccess,
				Completeness: inventory.CompletenessComplete,
				Items:        []inventory.Resource{resource},
			}
			Expect(page.Validate()).To(Succeed())

			page.Items = nil
			Expect(page.Validate()).To(MatchError(ContainSubstring("successful page must contain at least one item")))
		})

		It("requires complete empty pages and partial state-only failure pages", func() {
			empty := inventory.Page{State: inventory.StateEmpty, Completeness: inventory.CompletenessComplete}
			Expect(empty.Validate()).To(Succeed())

			empty.Completeness = inventory.CompletenessPartial
			Expect(empty.Validate()).To(MatchError(ContainSubstring("empty page must be complete")))

			failure := inventory.Page{State: inventory.StateUnavailable, Completeness: inventory.CompletenessPartial}
			Expect(failure.Validate()).To(Succeed())

			failure.Completeness = inventory.CompletenessComplete
			Expect(failure.Validate()).To(MatchError(ContainSubstring("state-only page must be partial")))
		})

		It("rejects an item whose ID does not match its identity and a bad cursor", func() {
			resource := validResource()
			page := inventory.Page{
				State:        inventory.StateSuccess,
				Completeness: inventory.CompletenessComplete,
				Items:        []inventory.Resource{resource},
			}
			resource.ID = "r-" + "1111111111111111111111111111111111111111111111111111111111111111"
			page.Items[0] = resource
			Expect(page.Validate()).To(MatchError(ContainSubstring("does not match canonical identity")))

			resource = validResource()
			page.Items[0] = resource
			page.NextCursor = "opaque-id"
			Expect(page.Validate()).To(MatchError(ContainSubstring("next_cursor is not a valid resource ID")))
		})
	})
})
