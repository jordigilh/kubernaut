package policyread_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
)

var _ = Describe("BR-SECURITY-016: bounded local list authorization", func() {
	It("UT-AF-2584-RBAC-001: authorizes each eligible local GVR without wildcard resource attributes", func() {
		authorizer := &recordingAuthorizer{}
		service, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory: &inventoryStub{page: inventory.Page{State: inventory.StateEmpty, Completeness: inventory.CompletenessComplete}},
			Authorizer:     authorizer,
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = service.ListInventory(context.Background(), &auth.UserIdentity{Username: "alice"}, inventory.Filters{ClusterID: "local"})

		Expect(err).NotTo(HaveOccurred())
		Expect(authorizer.targets).NotTo(BeEmpty())
		for _, target := range authorizer.targets {
			Expect(target.Group).NotTo(Equal("*"))
			Expect(target.Version).NotTo(Equal("*"))
			Expect(target.Resource).NotTo(Equal("*"))
		}
	})
})

type recordingAuthorizer struct {
	targets []auth.TargetAccess
}

func (a *recordingAuthorizer) Check(context.Context, string, []string, string) (bool, error) {
	return true, nil
}

func (a *recordingAuthorizer) CheckTarget(_ context.Context, _ string, _ []string, target auth.TargetAccess) (bool, error) {
	a.targets = append(a.targets, target)
	return true, nil
}

var _ policyread.Authorizer = (*recordingAuthorizer)(nil)
