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

package policyread_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// TP-2661 Tier 1c: the non-local namespace-list SAR. authorize() is shared by
// list, exact-get and policy-scope reads, so a denied list/core/v1/namespaces
// SAR must forbid every non-local arm before the reader is consulted, with
// exactly one typed audit event per read carrying result_state=forbidden.
var _ = Describe("BR-SECURITY-016: non-local namespace-list SAR (TP-2661 phase c)", func() {
	remoteResourceID := inventory.CanonicalResourceID(scope.ResourceIdentity{
		ClusterID: "prod-east", Group: "apps", Version: "v1", Kind: "Deployment",
		Namespace: "payments", Name: "checkout",
	})

	remoteSuccessPage := inventory.Page{
		State:        inventory.StateSuccess,
		ObservedAt:   time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC),
		Freshness:    inventory.Freshness{State: inventory.FreshnessFresh, AsOf: time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)},
		Completeness: inventory.CompletenessComplete,
		Items: []inventory.Resource{{
			ID: remoteResourceID, ClusterID: "prod-east", Group: "apps", Version: "v1", Kind: "Deployment",
			Namespace: "payments", Name: "checkout", Scope: inventory.Scope{Included: true},
		}},
	}

	It("UT-AF-2661-004 [BR-SECURITY-016, AC-6]: a denied list/core/v1/namespaces SAR forbids the non-local list before the remote reader, with one typed audit event", func() {
		reader := &remoteAuthInventoryReader{page: remoteSuccessPage}
		auditor := &policyReadAuditRecorder{}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			RemoteInventory: reader,
			Authorizer:      &remoteAuthAuthorizer{denyNamespaceList: true},
			Auditor:         auditor,
		})
		Expect(err).NotTo(HaveOccurred())

		page, err := svc.ListInventory(context.Background(), user("alice", false), inventory.Filters{ClusterID: "prod-east"})

		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateForbidden))
		Expect(page.State).To(Equal(inventory.StateForbidden))
		Expect(page.Items).To(BeEmpty())
		Expect(reader.listCalls).To(BeZero(), "a denied non-local list must not reach the remote reader")
		Expect(auditor.events).To(HaveLen(1))
		Expect(auditor.events[0].Type).To(Equal(audit.EventConsoleManagedResourceListRead))
		Expect(auditor.events[0].UserID).To(Equal("alice"))
		Expect(auditor.events[0].Detail["result_state"]).To(Equal("forbidden"))
	})

	It("UT-AF-2661-004 [BR-SECURITY-016, AC-6]: the same denial forbids a non-local exact get before the remote reader", func() {
		reader := &remoteAuthInventoryReader{resource: remoteSuccessPage.Items[0]}
		auditor := &policyReadAuditRecorder{}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			RemoteInventory: reader,
			Authorizer:      &remoteAuthAuthorizer{denyNamespaceList: true},
			Auditor:         auditor,
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = svc.GetInventory(context.Background(), user("alice", false), inventory.Filters{ClusterID: "prod-east"}, remoteResourceID)

		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateForbidden))
		Expect(reader.getCalls).To(BeZero(), "a denied non-local exact get must not reach the remote reader")
		Expect(auditor.events).To(HaveLen(1))
		Expect(auditor.events[0].Type).To(Equal(audit.EventConsoleManagedResourceGetRead))
		Expect(auditor.events[0].Detail["result_state"]).To(Equal("forbidden"))
	})

	It("UT-AF-2661-004 [BR-SECURITY-016, AC-6]: the same denial forbids a non-local policy-scope read before its surviving non-local rejection", func() {
		auditor := &policyReadAuditRecorder{}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			Authorizer: &remoteAuthAuthorizer{denyNamespaceList: true},
			Auditor:    auditor,
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = svc.ListPolicyScope(context.Background(), user("alice", false), inventory.Filters{ClusterID: "prod-east"})

		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateForbidden), "the SAR denial must surface as forbidden, not the policy-scope rejection's unavailable")
		Expect(auditor.events).To(HaveLen(1))
		Expect(auditor.events[0].Type).To(Equal(audit.EventConsolePolicyScopeRead))
		Expect(auditor.events[0].Detail["result_state"]).To(Equal("forbidden"))
	})

	It("UT-AF-2661-004 [BR-SECURITY-016, AC-6]: granting the namespace-list SAR proceeds to non-local dispatch", func() {
		reader := &remoteAuthInventoryReader{page: remoteSuccessPage}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			RemoteInventory: reader,
			Authorizer:      &remoteAuthAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())

		page, err := svc.ListInventory(context.Background(), user("alice", false), inventory.Filters{ClusterID: "prod-east"})

		Expect(err).NotTo(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateSuccess))
		Expect(reader.listCalls).To(Equal(1), "a granted non-local list dispatches to the remote reader")
	})
})

// remoteAuthAuthorizer grants every capability and every target except the
// non-local namespace-list SAR when denyNamespaceList is set.
type remoteAuthAuthorizer struct {
	denyNamespaceList bool
}

func (a *remoteAuthAuthorizer) Check(context.Context, string, []string, string) (bool, error) {
	return true, nil
}

func (a *remoteAuthAuthorizer) CheckTarget(_ context.Context, _ string, _ []string, target auth.TargetAccess) (bool, error) {
	if a.denyNamespaceList && target.Verb == "list" && target.Resource == "namespaces" && target.Version == "v1" {
		return false, nil
	}
	return true, nil
}

type remoteAuthInventoryReader struct {
	page      inventory.Page
	resource  inventory.Resource
	listCalls int
	getCalls  int
}

func (r *remoteAuthInventoryReader) List(_ context.Context, _ inventory.Filters) (inventory.Page, error) {
	r.listCalls++
	return r.page, nil
}

func (r *remoteAuthInventoryReader) Get(_ context.Context, _ string, _ string) (inventory.Resource, error) {
	r.getCalls++
	return r.resource, nil
}
