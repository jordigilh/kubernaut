package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/fleet/acm"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

var _ = Describe("BR-SCOPE-001 / BR-SECURITY-016: approved local-only Console inventory list", func() {
	It("IT-AF-2584-REMOTE-LIST-001 [BR-SCOPE-001]: rejects a non-local inventory list without contacting a remote reader or falling back locally", func() {
		var requests atomic.Int32
		search := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			http.Error(w, "unexpected remote inventory request", http.StatusInternalServerError)
		}))
		DeferCleanup(search.Close)
		local := &consoleInventoryStub{page: successfulPage()}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory: local, RemoteInventory: acm.NewClient(search.URL), Authorizer: &consoleAuthorizer{allow: true},
		})
		Expect(err).NotTo(HaveOccurred())
		readHandler := handler.NewConsolePolicyReadHandler(svc, logr.Discard())
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=prod-east&kind=Deployment", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		recorder := httptest.NewRecorder()

		readHandler.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(recorder.Body.String()).To(ContainSubstring(`"state":"unavailable"`))
		Expect(recorder.Body.String()).NotTo(ContainSubstring(`"items"`))
		Expect(requests.Load()).To(BeZero())
		Expect(local.listCalls).To(BeZero())
	})
	It("IT-AF-2593-DENY-001 [BR-SECURITY-016]: denies a remote inventory read before contacting ACM", func() {
		var requests atomic.Int32
		search := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			http.Error(w, "unexpected Search request", http.StatusInternalServerError)
		}))
		DeferCleanup(search.Close)
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			RemoteInventory: acm.NewClient(search.URL), Authorizer: &consoleAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())
		readHandler := handler.NewConsolePolicyReadHandler(svc, logr.Discard())
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=prod-east", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		recorder := httptest.NewRecorder()

		readHandler.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusForbidden))
		Expect(recorder.Body.String()).To(ContainSubstring(`"state":"forbidden"`))
		Expect(requests.Load()).To(BeZero())
	})
	It("IT-AF-2584-REMOTE-LIST-002 [BR-SECURITY-016]: keeps a remote ACM list unavailable instead of exposing a partial Search page", func() {
		var requests atomic.Int32
		search := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			_, err := io.WriteString(w, `{"data":{"searchResult":[{"count":2,"items":[{"cluster":"prod-east","apiversion":"v1","kind":"Namespace","name":"sensitive-namespace","label":"kubernaut.ai/managed=true"}]}]}}`)
			Expect(err).NotTo(HaveOccurred())
		}))
		DeferCleanup(search.Close)
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			RemoteInventory: acm.NewClient(search.URL), Authorizer: &consoleAuthorizer{allow: true},
		})
		Expect(err).NotTo(HaveOccurred())
		readHandler := handler.NewConsolePolicyReadHandler(svc, logr.Discard())
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=prod-east", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		recorder := httptest.NewRecorder()

		readHandler.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(recorder.Body.String()).To(ContainSubstring(`"state":"unavailable"`))
		Expect(recorder.Body.String()).NotTo(ContainSubstring("sensitive-namespace"))
		Expect(recorder.Body.String()).NotTo(ContainSubstring(`"items"`))
		Expect(requests.Load()).To(BeZero())
	})
	It("IT-AF-2593-PUBLIC-METADATA-001 [BR-API-001]: returns observed time and backend freshness metadata", func() {
		readHandler := newConsoleReadHandler(&consoleAuthorizer{allow: true}, &consoleInventoryStub{page: successfulPage()})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		recorder := httptest.NewRecorder()

		readHandler.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusOK))
		var result map[string]json.RawMessage
		Expect(json.Unmarshal(recorder.Body.Bytes(), &result)).To(Succeed())
		Expect(result).To(HaveKey("observed_at"))
		Expect(result).To(HaveKey("freshness"))
		Expect(result).To(HaveKey("items"))
	})

	It("IT-AF-2593-PUBLIC-METADATA-002 [BR-API-001]: exposes stale freshness without rejecting a complete result", func() {
		page := successfulPage()
		page.Freshness.State = inventory.FreshnessStale
		readHandler := newConsoleReadHandler(&consoleAuthorizer{allow: true}, &consoleInventoryStub{page: page})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		recorder := httptest.NewRecorder()

		readHandler.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusOK))
		Expect(recorder.Body.String()).To(ContainSubstring(`"state":"success"`))
		Expect(recorder.Body.String()).To(ContainSubstring(`"state":"stale"`))
	})

	It("IT-AF-2593-PUBLIC-METADATA-003 [BR-API-001]: returns policy-scope observed time and freshness metadata", func() {
		page := inventory.Page{
			State:        inventory.StateEmpty,
			ObservedAt:   time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC),
			Freshness:    inventory.Freshness{State: inventory.FreshnessStale, AsOf: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)},
			Completeness: inventory.CompletenessComplete,
		}
		readHandler := newConsoleReadHandler(&consoleAuthorizer{allow: true}, &consoleInventoryStub{page: page})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/policies/scope?cluster_id=local", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		recorder := httptest.NewRecorder()

		readHandler.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusOK))
		var result map[string]json.RawMessage
		Expect(json.Unmarshal(recorder.Body.Bytes(), &result)).To(Succeed())
		Expect(result).To(HaveKey("observedAt"))
		Expect(result).To(HaveKey("freshness"))
		Expect(string(result["state"])).To(Equal(`"empty"`))
	})
	It("returns an RFC 7807 error for duplicated filters", func() {
		readHandler := newConsoleReadHandler(&consoleAuthorizer{allow: true}, &consoleInventoryStub{page: successfulPage()})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local&cluster_id=other", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		recorder := httptest.NewRecorder()

		readHandler.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusBadRequest))
		Expect(recorder.Header().Get("Content-Type")).To(Equal("application/problem+json"))
		Expect(recorder.Body.String()).To(ContainSubstring("invalid Console read filter"))
	})

	It("serializes a successful authenticated inventory page", func() {
		readHandler := newConsoleReadHandler(&consoleAuthorizer{allow: true}, &consoleInventoryStub{page: successfulPage()})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		recorder := httptest.NewRecorder()

		readHandler.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusOK))
		var result inventory.Page
		Expect(json.Unmarshal(recorder.Body.Bytes(), &result)).To(Succeed())
		Expect(result.State).To(Equal(inventory.StateSuccess))
		Expect(result.Items).To(HaveLen(1))
	})

	It("IT-AF-2592-RESPONSE-001 [BR-SAFETY-001]: rejects an oversized serialized Console projection before writing a success body", func() {
		page := successfulPage()
		const marker = "oversized-console-projection-"
		page.Items[0].Name = marker + strings.Repeat("x", int(policyread.DefaultInventoryResponseBytes))
		readHandler := newConsoleReadHandler(&consoleAuthorizer{allow: true}, &consoleInventoryStub{page: page})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		recorder := httptest.NewRecorder()

		readHandler.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(recorder.Body.String()).To(ContainSubstring(`"state":"unavailable"`))
		Expect(recorder.Body.String()).NotTo(ContainSubstring(marker))
		Expect(recorder.Body.String()).NotTo(ContainSubstring(`"items"`))
	})

	It("rejects service accounts without reading inventory", func() {
		reader := &consoleInventoryStub{page: successfulPage()}
		readHandler := newConsoleReadHandler(&consoleAuthorizer{allow: true}, reader)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "system:serviceaccount:ns:af", IsServiceAccount: true}))
		recorder := httptest.NewRecorder()

		readHandler.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusForbidden))
		Expect(recorder.Body.String()).To(ContainSubstring(`"state":"forbidden"`))
		Expect(reader.listCalls).To(Equal(0))
	})

	It("maps exact target denial to not_found without leaking the target", func() {
		resource := successfulPage().Items[0]
		resource.Name = "sensitive-target"
		reader := &consoleInventoryStub{page: successfulPage(), resource: resource}
		readHandler := newConsoleReadHandler(&consoleAuthorizer{allow: false, capabilityAllow: true}, reader)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources/"+resource.ID+"?cluster_id=local", http.NoBody)
		req.SetPathValue("resourceID", resource.ID)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		recorder := httptest.NewRecorder()

		readHandler.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusNotFound))
		Expect(recorder.Body.String()).To(ContainSubstring(`"state":"not_found"`))
		Expect(recorder.Body.String()).NotTo(ContainSubstring("sensitive-target"))
	})
})

func newConsoleReadHandler(authorizer policyread.Authorizer, reader policyread.InventoryReader) http.Handler {
	service, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
		LocalInventory:  reader,
		CurrentPolicies: &consoleCurrentPolicyStub{},
		Authorizer:      authorizer,
	})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return handler.NewConsolePolicyReadHandler(service, logrDiscard())
}

func successfulPage() inventory.Page {
	observedAt := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	return inventory.Page{
		State:        inventory.StateSuccess,
		ObservedAt:   observedAt,
		Freshness:    inventory.Freshness{State: inventory.FreshnessFresh, AsOf: observedAt},
		Completeness: inventory.CompletenessComplete,
		Items: []inventory.Resource{{
			ID:        inventory.CanonicalResourceID(scope.ResourceIdentity{ClusterID: "local", Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "payments", Name: "checkout"}),
			ClusterID: "local",
			Group:     "apps",
			Version:   "v1",
			Kind:      "Deployment",
			Namespace: "payments",
			Name:      "checkout",
			Scope:     inventory.Scope{Included: true, Source: inventory.ScopeSourceResourceLabel, ResourceLabel: inventory.LabelTrue},
		}},
	}
}

type consoleInventoryStub struct {
	page      inventory.Page
	resource  inventory.Resource
	listCalls int
	getCalls  int
}

type consoleCurrentPolicyStub struct{}

func (*consoleCurrentPolicyStub) ReadCurrentPolicies(context.Context, string) ([]policyread.PolicyMetadata, error) {
	return []policyread.PolicyMetadata{{
		Type:          "aiAnalysis",
		ConfigMapName: "aianalysis-policy",
		ConfigMapKey:  "approval.rego",
		Revision:      "sha256:ai",
	}}, nil
}

func (s *consoleInventoryStub) List(context.Context, inventory.Filters) (inventory.Page, error) {
	s.listCalls++
	return s.page, nil
}

func (s *consoleInventoryStub) Get(context.Context, string, string) (inventory.Resource, error) {
	s.getCalls++
	if s.resource.ID == "" {
		return inventory.Resource{}, inventory.ErrNotFound
	}
	return s.resource, nil
}

type consoleAuthorizer struct {
	allow           bool
	capabilityAllow bool
}

func (a *consoleAuthorizer) Check(context.Context, string, []string, string) (bool, error) {
	if !a.capabilityAllow {
		return a.allow, nil
	}
	return true, nil
}

func (a *consoleAuthorizer) CheckTarget(context.Context, string, []string, auth.TargetAccess) (bool, error) {
	return a.allow, nil
}

func logrDiscard() logr.Logger { return logr.Discard() }

var _ policyread.InventoryReader = (*consoleInventoryStub)(nil)
var _ policyread.Authorizer = (*consoleAuthorizer)(nil)
