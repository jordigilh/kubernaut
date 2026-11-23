package policyread_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

var _ = Describe("BR-API-001 / BR-SCOPE-001: #2584/#2592/#2593 interim inventory contract", func() {
	It("UT-AF-2592-REMOTE-FAIL-CLOSED-001 [BR-SCOPE-001]: dispatches non-local reads to the remote reader without ever falling back locally", func() {
		// TP-2661 phase c re-scope: the interim contract's non-local rejection
		// is replaced by dispatch to the wired remote reader. The no-local-
		// fallback half is kept verbatim; the old "rejects without selecting a
		// remote reader" expectation is replaced, not weakened — the boundary
		// moved from refusal to remote-only dispatch (DD-FLEET-017 D2.1a).
		remoteItem := inventory.Resource{
			ID: inventory.CanonicalResourceID(scope.ResourceIdentity{
				ClusterID: "prod-east", Group: "apps", Version: "v1", Kind: "Deployment",
				Namespace: "payments", Name: "checkout",
			}),
			ClusterID: "prod-east", Group: "apps", Version: "v1", Kind: "Deployment",
			Namespace: "payments", Name: "checkout", Scope: inventory.Scope{Included: true},
		}
		local := &interimInventoryReader{}
		remote := &interimInventoryReader{
			page: inventory.Page{
				State:        inventory.StateSuccess,
				ObservedAt:   time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC),
				Freshness:    inventory.Freshness{State: inventory.FreshnessFresh, AsOf: time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)},
				Completeness: inventory.CompletenessComplete,
				Items:        []inventory.Resource{remoteItem},
			},
		}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory:  local,
			RemoteInventory: remote,
			Authorizer:      allowAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())

		page, err := svc.ListInventory(context.Background(), user("alice", false), inventory.Filters{ClusterID: "prod-east"})

		Expect(err).NotTo(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateSuccess))
		Expect(page.Items).To(HaveLen(1))
		Expect(page.Items[0].ClusterID).To(Equal("prod-east"))
		Expect(local.listCalls).To(BeZero(), "a non-local request must not fall back to the local backend")
		Expect(remote.listCalls).To(Equal(1), "a non-local request dispatches to the remote reader")
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
