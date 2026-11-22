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

package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	fmcclient "github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// TP-2661 Tier 2 (phase c): the Console read routes map the projection
// states onto their wire statuses with state-only bodies, and the non-local
// namespace-list SAR denial is a 403 with exactly one typed audit event and
// zero requests to FMC.
var _ = Describe("BR-API-001: Console inventory status mapping (TP-2661 phase c)", func() {
	serve := func(svc *policyread.Service) *httptest.ResponseRecorder {
		readHandler := handler.NewConsolePolicyReadHandler(svc, logr.Discard())
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=prod-east", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		recorder := httptest.NewRecorder()
		readHandler.ServeHTTP(recorder, req)
		return recorder
	}

	It("IT-AF-2661-002 [BR-API-001]: maps partial and unavailable to 503 state-only, forbidden to 403, not_found to 404, with no rows or cursors leaking", func() {
		By("partial -> 503 state-only")
		partial := serve(mustService(&policyread.Dependencies{
			RemoteInventory: &stateDrivingReader{page: inventory.StateOnly(inventory.StatePartial)},
			Authorizer:      &consoleAuthorizer{allow: true},
		}))
		Expect(partial.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(partial.Body.String()).To(ContainSubstring(`"state":"partial"`))
		Expect(partial.Body.String()).NotTo(ContainSubstring(`"items"`))
		Expect(partial.Body.String()).NotTo(ContainSubstring(`"next_cursor"`))

		By("unavailable -> 503 state-only")
		unavailable := serve(mustService(&policyread.Dependencies{
			RemoteInventory: &stateDrivingReader{listErr: inventory.ErrUnavailable},
			Authorizer:      &consoleAuthorizer{allow: true},
		}))
		Expect(unavailable.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(unavailable.Body.String()).To(ContainSubstring(`"state":"unavailable"`))
		Expect(unavailable.Body.String()).NotTo(ContainSubstring(`"items"`))

		By("forbidden (the non-local namespace-list SAR) -> 403 state-only")
		forbidden := serve(mustService(&policyread.Dependencies{
			RemoteInventory: &stateDrivingReader{},
			Authorizer:      &denyingSARAuthorizer{},
		}))
		Expect(forbidden.Code).To(Equal(http.StatusForbidden))
		Expect(forbidden.Body.String()).To(ContainSubstring(`"state":"forbidden"`))
		Expect(forbidden.Body.String()).NotTo(ContainSubstring(`"items"`))

		By("not_found -> 404 state-only")
		notFoundReq := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources/r-"+repeat("0", 64)+"?cluster_id=prod-east", http.NoBody)
		notFoundReq.SetPathValue("resourceID", "r-"+repeat("0", 64))
		notFoundReq = notFoundReq.WithContext(auth.WithUserIdentity(notFoundReq.Context(), &auth.UserIdentity{Username: "alice"}))
		notFound := httptest.NewRecorder()
		handler.NewConsolePolicyReadHandler(mustService(&policyread.Dependencies{
			RemoteInventory: &stateDrivingReader{getErr: inventory.ErrNotFound},
			Authorizer:      &consoleAuthorizer{allow: true},
		}), logr.Discard()).ServeHTTP(notFound, notFoundReq)
		Expect(notFound.Code).To(Equal(http.StatusNotFound))
		Expect(notFound.Body.String()).To(ContainSubstring(`"state":"not_found"`))
		Expect(notFound.Body.String()).NotTo(ContainSubstring(`"items"`))
	})

	It("IT-AF-2661-003 [AC-6, AU-3]: a denied namespace-list SAR returns 403 state-only with zero requests to FMC and exactly one typed audit event", func() {
		var fmcRequests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			fmcRequests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			Expect(json.NewEncoder(w).Encode(inventory.Page{
				State:        inventory.StateSuccess,
				ObservedAt:   time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC),
				Freshness:    inventory.Freshness{State: inventory.FreshnessFresh, AsOf: time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)},
				Completeness: inventory.CompletenessComplete,
				Items: []inventory.Resource{{
					ID: inventory.CanonicalResourceID(scope.ResourceIdentity{
						ClusterID: "prod-east", Group: "apps", Version: "v1", Kind: "Deployment",
						Namespace: "payments", Name: "checkout",
					}),
					ClusterID: "prod-east", Group: "apps", Version: "v1", Kind: "Deployment",
					Namespace: "payments", Name: "checkout", Scope: inventory.Scope{Included: true},
				}},
			})).To(Succeed())
		}))
		DeferCleanup(server.Close)

		auditor := &sarAuditRecorder{}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			RemoteInventory: fmcclient.NewHTTPClient(server.URL),
			Authorizer:      &denyingSARAuthorizer{},
			Auditor:         auditor,
		})
		Expect(err).NotTo(HaveOccurred())

		recorder := serve(svc)

		Expect(recorder.Code).To(Equal(http.StatusForbidden))
		Expect(recorder.Body.String()).To(ContainSubstring(`"state":"forbidden"`))
		Expect(recorder.Body.String()).NotTo(ContainSubstring(`"items"`))
		Expect(fmcRequests.Load()).To(BeZero(), "a denied non-local list must never reach FMC")
		Expect(auditor.events).To(HaveLen(1), "exactly one typed audit event for the read")
		Expect(auditor.events[0].Type).To(Equal(audit.EventConsoleManagedResourceListRead))
		Expect(auditor.events[0].UserID).To(Equal("alice"))
		Expect(auditor.events[0].ClusterID).To(Equal("prod-east"))
		Expect(auditor.events[0].Detail["result_state"]).To(Equal("forbidden"))
		Expect(auditor.events[0].Detail).NotTo(HaveKey("name"), "no records in the denial event")
	})
})

func mustService(deps *policyread.Dependencies) *policyread.Service {
	svc, err := policyread.NewService(policyread.Config{}, *deps)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return svc
}

func repeat(char string, count int) string {
	out := make([]byte, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, char[0])
	}
	return string(out)
}

// denyingSARAuthorizer grants every capability and every target except the
// non-local namespace-list SAR.
type denyingSARAuthorizer struct{}

func (denyingSARAuthorizer) Check(context.Context, string, []string, string) (bool, error) {
	return true, nil
}

func (denyingSARAuthorizer) CheckTarget(_ context.Context, _ string, _ []string, target auth.TargetAccess) (bool, error) {
	if target.Verb == "list" && target.Resource == "namespaces" && target.Version == "v1" {
		return false, nil
	}
	return true, nil
}

// stateDrivingReader drives a chosen projection state through the service.
type stateDrivingReader struct {
	page    inventory.Page
	listErr error
	getErr  error
}

func (r *stateDrivingReader) List(_ context.Context, _ inventory.Filters) (inventory.Page, error) {
	if r.listErr != nil {
		return inventory.StateOnly(inventory.StateUnavailable), r.listErr
	}
	return r.page, nil
}

func (r *stateDrivingReader) Get(_ context.Context, _ string, _ string) (inventory.Resource, error) {
	if r.getErr != nil {
		return inventory.Resource{}, r.getErr
	}
	return inventory.Resource{}, inventory.ErrNotFound
}

type sarAuditRecorder struct {
	events []*audit.Event
}

func (r *sarAuditRecorder) Emit(_ context.Context, event *audit.Event) {
	r.events = append(r.events, event)
}
