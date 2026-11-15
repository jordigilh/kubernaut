package policyread_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
)

var _ = Describe("BR-API-001 / BR-SCOPE-001: #2584/#2592/#2593 interim inventory contract", func() {
	It("UT-AF-2592-REMOTE-FAIL-CLOSED-001 [BR-SCOPE-001]: rejects non-local reads without selecting a remote reader or falling back locally", func() {
		local := &interimInventoryReader{
			page: inventory.Page{State: inventory.StateSuccess, Completeness: inventory.CompletenessComplete},
		}
		remote := &interimInventoryReader{
			page: inventory.Page{State: inventory.StateSuccess, Completeness: inventory.CompletenessComplete},
		}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory:  local,
			RemoteInventory: remote,
			Authorizer:      allowAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())

		page, err := svc.ListInventory(context.Background(), user("alice", false), inventory.Filters{ClusterID: "prod-east"})

		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateUnavailable))
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())
		Expect(local.listCalls).To(BeZero(), "a non-local request must not fall back to the local backend")
		Expect(remote.listCalls).To(BeZero(), "the interim contract keeps ACM and FMC unavailable")
	})

	It("UT-AF-2592-POLICY-REMOTE-001 [BR-SAFETY-001]: rejects non-local policy-scope listing without selecting a remote reader", func() {
		local := &interimInventoryReader{}
		remote := &interimInventoryReader{}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory: local, RemoteInventory: remote, Authorizer: allowAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())

		page, err := svc.ListPolicyScope(context.Background(), user("alice", false), inventory.Filters{ClusterID: "prod-east"})

		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())
		Expect(local.listCalls).To(BeZero())
		Expect(remote.listCalls).To(BeZero())
	})

	It("UT-AF-2592-INCOMPLETE-001 [BR-SAFETY-001]: converts an incomplete local page into a state-only partial result", func() {
		reader := &interimInventoryReader{page: inventory.Page{
			State:        inventory.StateSuccess,
			Completeness: inventory.CompletenessPartial,
			NextCursor:   "r-next",
			Items: []inventory.Resource{{
				ID:        "r-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				ClusterID: "local",
				Version:   "v1",
				Kind:      "Deployment",
				Name:      "checkout",
				Scope:     inventory.Scope{Included: true},
			}},
		}}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory: reader,
			Authorizer:     allowAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())

		page, err := svc.ListInventory(context.Background(), user("alice", false), inventory.Filters{ClusterID: "local"})

		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StatePartial))
		Expect(page.State).To(Equal(inventory.StatePartial))
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())
	})

	It("UT-AF-2592-BUDGET-001 [BR-SAFETY-001]: turns a request deadline failure into a state-only unavailable result", func() {
		reader := &interimInventoryReader{
			listErr: context.DeadlineExceeded,
		}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory: reader,
			Authorizer:     allowAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())

		page, err := svc.ListInventory(context.Background(), user("alice", false), inventory.Filters{ClusterID: "local"})

		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateUnavailable))
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())
	})
})

type interimInventoryReader struct {
	page      inventory.Page
	listErr   error
	listCalls int
}

func (r *interimInventoryReader) List(context.Context, inventory.Filters) (inventory.Page, error) {
	r.listCalls++
	if r.listErr != nil {
		return inventory.StateOnly(inventory.StateUnavailable), r.listErr
	}
	return r.page, nil
}

func (r *interimInventoryReader) Get(context.Context, string, string) (inventory.Resource, error) {
	return inventory.Resource{}, inventory.ErrNotFound
}

var _ policyread.InventoryReader = (*interimInventoryReader)(nil)
