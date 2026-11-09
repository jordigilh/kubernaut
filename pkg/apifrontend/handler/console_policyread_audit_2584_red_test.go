package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
)

var _ = Describe("BR-AUDIT-005: final Console read outcomes", func() {
	It("IT-AF-2584-AUDIT-001: audits an authenticated malformed request", func() {
		recorder := &consoleAuditRecorder{}
		service, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory: &consoleAuditInventoryStub{page: consoleAuditPage()},
			Authorizer:     consoleAuditAuthorizer{},
			Auditor:        recorder,
		})
		Expect(err).NotTo(HaveOccurred())

		restHandler := handler.NewConsolePolicyReadHandler(service, logr.Discard())
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local&cluster_id=other", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		restRecorder := httptest.NewRecorder()

		restHandler.ServeHTTP(restRecorder, req)

		Expect(restRecorder.Code).To(Equal(http.StatusBadRequest))
		Expect(recorder.events).To(HaveLen(1))
		Expect(recorder.events[0].Detail).To(HaveKeyWithValue("result_state", string(inventory.StateUnavailable)))
	})

	It("IT-AF-2584-AUDIT-002: audits an oversized response as unavailable rather than success", func() {
		page := consoleAuditPage()
		page.Items[0].Name = "oversized-" + strings.Repeat("x", int(policyread.DefaultInventoryResponseBytes))
		recorder := &consoleAuditRecorder{}
		service, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory: &consoleAuditInventoryStub{page: page},
			Authorizer:     consoleAuditAuthorizer{},
			Auditor:        recorder,
		})
		Expect(err).NotTo(HaveOccurred())

		restHandler := handler.NewConsolePolicyReadHandler(service, logr.Discard())
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		restRecorder := httptest.NewRecorder()

		restHandler.ServeHTTP(restRecorder, req)

		Expect(restRecorder.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(recorder.events).To(HaveLen(1))
		Expect(recorder.events[0].Detail).To(HaveKeyWithValue("result_state", string(inventory.StateUnavailable)))
	})
})

type consoleAuditRecorder struct {
	events []*audit.Event
}

func (r *consoleAuditRecorder) Emit(_ context.Context, event *audit.Event) {
	r.events = append(r.events, event)
}

type consoleAuditInventoryStub struct {
	page inventory.Page
}

func (s *consoleAuditInventoryStub) List(context.Context, inventory.Filters) (inventory.Page, error) {
	return s.page, nil
}

func (*consoleAuditInventoryStub) Get(context.Context, string, string) (inventory.Resource, error) {
	return inventory.Resource{}, inventory.ErrNotFound
}

type consoleAuditAuthorizer struct{}

func (consoleAuditAuthorizer) Check(context.Context, string, []string, string) (bool, error) {
	return true, nil
}

func (consoleAuditAuthorizer) CheckTarget(context.Context, string, []string, auth.TargetAccess) (bool, error) {
	return true, nil
}

func consoleAuditPage() inventory.Page {
	return inventory.Page{
		State:        inventory.StateSuccess,
		Completeness: inventory.CompletenessComplete,
		Items: []inventory.Resource{{
			ID:        "r-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			ClusterID: "local",
			Group:     "apps",
			Version:   "v1",
			Kind:      "Deployment",
			Namespace: "payments",
			Name:      "checkout",
			Scope:     inventory.Scope{Included: true},
		}},
	}
}

var _ policyread.InventoryReader = (*consoleAuditInventoryStub)(nil)
var _ policyread.Authorizer = consoleAuditAuthorizer{}
