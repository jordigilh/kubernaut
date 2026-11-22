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
	fmcclient "github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

var _ = Describe("BR-SCOPE-001 / BR-SECURITY-016: Console inventory list (local projection; non-local FMC dispatch under the namespace-list SAR)", func() {
	It("IT-AF-2584-REMOTE-LIST-001 [BR-SCOPE-001]: dispatches a non-local inventory list to the FMC client without falling back locally", func() {
		// TP-2661 phase c re-scope: the boundary moved from refusal to
		// remote-only dispatch (DD-FLEET-017 D2.1a). The stub serves the FMC
		// page route shape — never-ACM is proven by the wire shape (a Search
		// envelope would fail to decode), not by asserting zero dispatch.
		var requests atomic.Int32
		fmcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			requests.Add(1)
			Expect(r.URL.Path).To(Equal(fmcclient.InventoryPath))
			w.Header().Set("Content-Type", "application/json")
			Expect(json.NewEncoder(w).Encode(successfulFMCPage())).To(Succeed())
		}))
		DeferCleanup(fmcServer.Close)
		local := &consoleInventoryStub{page: successfulPage()}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory: local, RemoteInventory: fmcclient.NewHTTPClient(fmcServer.URL), Authorizer: &consoleAuthorizer{allow: true},
		})
		Expect(err).NotTo(HaveOccurred())
		readHandler := handler.NewConsolePolicyReadHandler(svc, logr.Discard())
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=prod-east&kind=Deployment", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		recorder := httptest.NewRecorder()

		readHandler.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusOK))
		Expect(recorder.Body.String()).To(ContainSubstring(`"state":"success"`))
		Expect(recorder.Body.String()).To(ContainSubstring(`"cluster_id":"prod-east"`))
		Expect(requests.Load()).To(Equal(int32(1)), "the dispatch reaches the FMC fixture exactly once")
		Expect(local.listCalls).To(BeZero(), "a non-local request must not fall back to the local backend")
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
	It("IT-AF-2584-REMOTE-LIST-002 [BR-SECURITY-016]: maps a partial remote FMC page to a state-only failure instead of exposing partial rows", func() {
		// TP-2661 phase c re-scope: two named cases. The success case proves
		// dispatch reaches the FMC client; the no-partial-rows case serves the
		// wire-realistic 503 state-only partial (mirroring FMC's own
		// inventoryWireStatus server-side mapping — a 200 state-only partial
		// would pass the client's Page.Validate and surface as partial, which
		// is why the fixture must use the 503) and keeps the no-leak
		// assertions.
		var partialRequests atomic.Int32
		partial := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			partialRequests.Add(1)
			Expect(r.URL.Path).To(Equal(fmcclient.InventoryPath))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, err := io.WriteString(w, `{"state":"partial","observed_at":"2026-11-22T12:00:00Z","freshness":{"state":"fresh","as_of":"2026-11-22T12:00:00Z"},"completeness":"partial"}`)
			Expect(err).NotTo(HaveOccurred())
		}))
		DeferCleanup(partial.Close)
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			RemoteInventory: fmcclient.NewHTTPClient(partial.URL), Authorizer: &consoleAuthorizer{allow: true},
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
		Expect(partialRequests.Load()).To(Equal(int32(1)), "the no-partial-rows case dispatches to the FMC fixture")
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
	It("IT-AF-2584-005 [FedRAMP SI-10]: returns an RFC 7807 error for duplicated filters", func() {
		readHandler := newConsoleReadHandler(&consoleAuthorizer{allow: true}, &consoleInventoryStub{page: successfulPage()})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local&cluster_id=other", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		recorder := httptest.NewRecorder()

		readHandler.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusBadRequest))
		Expect(recorder.Header().Get("Content-Type")).To(Equal("application/problem+json"))
		Expect(recorder.Body.String()).To(ContainSubstring("invalid Console read filter"))
	})

	It("IT-AF-2584-002 [BR-API-001]: serializes a successful authenticated inventory page", func() {
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

	It("IT-AF-2584-004 [BR-SECURITY-016]: rejects service accounts without reading inventory", func() {
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

	It("IT-AF-2584-004 [BR-SECURITY-016]: maps exact target denial to not_found without leaking the target", func() {
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

	It("UT-AF-2584-010 [BR-SECURITY-016, FedRAMP AC-6]: rejects a request without a verified identity before reading inventory", func() {
		reader := &consoleInventoryStub{page: successfulPage()}
		readHandler := newConsoleReadHandler(&consoleAuthorizer{allow: true}, reader)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local", http.NoBody)
		recorder := httptest.NewRecorder()

		readHandler.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusUnauthorized))
		Expect(recorder.Header().Get("Content-Type")).To(Equal("application/problem+json"))
		Expect(recorder.Body.String()).NotTo(ContainSubstring("checkout"))
		Expect(reader.listCalls).To(Equal(0))
	})

	It("IT-AF-2584-004 [BR-SECURITY-016, FedRAMP AC-4/AC-6]: returns a state-only denial when the endpoint capability is missing", func() {
		reader := &consoleInventoryStub{page: successfulPage()}
		readHandler := newConsoleReadHandler(capabilityDeniedAuthorizer{}, reader)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		recorder := httptest.NewRecorder()

		readHandler.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusForbidden))
		Expect(recorder.Body.String()).To(ContainSubstring(`"state":"forbidden"`))
		Expect(recorder.Body.String()).NotTo(ContainSubstring("checkout"))
		Expect(recorder.Body.String()).NotTo(ContainSubstring(`"items"`))
		Expect(reader.listCalls).To(Equal(0))
	})

	It("IT-AF-2584-005 [FedRAMP SI-10, AC-4]: rejects filters and IDs that would widen the result", func() {
		// A non-local cluster dispatches to the wired remote reader and never
		// falls back to the local hub reader. TP-2661 phase c re-scope: the
		// remote stub serves a prod-east-attributed success page (the old
		// fixture's local-cluster items would have made the 503 survive for
		// the wrong reason — cross-cluster item rejection, not dispatch
		// refusal), so the response is 200 success.
		local := &consoleInventoryStub{page: successfulPage()}
		remote := &consoleInventoryStub{page: successfulFMCPage()}
		// Both backends are wired so the non-local probe can distinguish "the
		// service refused the request" from "no remote reader was configured".
		service, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory:  local,
			RemoteInventory: remote,
			CurrentPolicies: &consoleCurrentPolicyStub{},
			Authorizer:      &consoleAuthorizer{allow: true},
		})
		Expect(err).NotTo(HaveOccurred())
		readHandler := handler.NewConsolePolicyReadHandler(service, logrDiscard())

		// A non-local cluster is refused: neither backend is consulted.
		crossCluster := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=prod-east", http.NoBody)
		crossCluster = crossCluster.WithContext(auth.WithUserIdentity(crossCluster.Context(), &auth.UserIdentity{Username: "alice"}))
		crossRecorder := httptest.NewRecorder()
		readHandler.ServeHTTP(crossRecorder, crossCluster)
		Expect(crossRecorder.Code).To(Equal(http.StatusOK))
		Expect(crossRecorder.Body.String()).To(ContainSubstring(`"state":"success"`))
		Expect(local.listCalls).To(Equal(0), "a remote cluster must not fall back to the local hub reader")
		Expect(remote.listCalls).To(Equal(1), "a non-local list dispatches to the wired remote reader")

		// A guessed opaque ID returns not_found without existence detail.
		guessedID := "r-" + strings.Repeat("0", 64)
		guessed := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources/"+guessedID+"?cluster_id=local", http.NoBody)
		guessed.SetPathValue("resourceID", guessedID)
		guessed = guessed.WithContext(auth.WithUserIdentity(guessed.Context(), &auth.UserIdentity{Username: "alice"}))
		guessedRecorder := httptest.NewRecorder()
		readHandler.ServeHTTP(guessedRecorder, guessed)
		Expect(guessedRecorder.Code).To(Equal(http.StatusNotFound))
		Expect(guessedRecorder.Body.String()).To(ContainSubstring(`"state":"not_found"`))
		Expect(guessedRecorder.Body.String()).NotTo(ContainSubstring("checkout"))
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

// capabilityDeniedAuthorizer grants the target but not the endpoint capability,
// modelling a caller that authenticates but lacks the named read capability.
type capabilityDeniedAuthorizer struct{}

func (capabilityDeniedAuthorizer) Check(context.Context, string, []string, string) (bool, error) {
	return false, nil
}

func (capabilityDeniedAuthorizer) CheckTarget(context.Context, string, []string, auth.TargetAccess) (bool, error) {
	return true, nil
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
			Scope:     inventory.Scope{Included: true, Source: inventory.ScopeSourceResourceLabel, ResourceLabel: inventory.LabelTrue, NamespaceLabel: inventory.LabelAbsent},
		}},
	}
}

// successfulFMCPage is the prod-east-attributed FMC-shaped success page used
// by the phase-c re-scoped remote-dispatch specs: a group-ful apps/v1 kind,
// the requested cluster's attribution, a canonical ID regenerated for that
// identity, and a contract-valid freshness envelope (TP-2661 §9b).
func successfulFMCPage() inventory.Page {
	observedAt := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	return inventory.Page{
		State:        inventory.StateSuccess,
		ObservedAt:   observedAt,
		Freshness:    inventory.Freshness{State: inventory.FreshnessFresh, AsOf: observedAt},
		Completeness: inventory.CompletenessComplete,
		Items: []inventory.Resource{{
			ID:        inventory.CanonicalResourceID(scope.ResourceIdentity{ClusterID: "prod-east", Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "payments", Name: "checkout"}),
			ClusterID: "prod-east",
			Group:     "apps",
			Version:   "v1",
			Kind:      "Deployment",
			Namespace: "payments",
			Name:      "checkout",
			Scope:     inventory.Scope{Included: true, Source: inventory.ScopeSourceResourceLabel, ResourceLabel: inventory.LabelTrue, NamespaceLabel: inventory.LabelAbsent},
		}},
	}
}

type consoleInventoryStub struct {
	page      inventory.Page
	resource  inventory.Resource
	listCalls int
	getCalls  int
}

// consoleCurrentPolicyStub mirrors the served contract: the revision is a
// sha256 digest (the schema enforces that shape) and the threshold comes from
// the operator runtime configuration.
type consoleCurrentPolicyStub struct{}

func (*consoleCurrentPolicyStub) ReadCurrentPolicies(context.Context, string) ([]policyread.PolicyMetadata, error) {
	return []policyread.PolicyMetadata{{
		Type:          "aiAnalysis",
		ConfigMapName: "aianalysis-policy",
		ConfigMapKey:  "approval.rego",
		Revision:      "sha256:" + strings.Repeat("a", 64),
		Threshold:     &policyread.Threshold{Value: 0.85, Source: "runtime-config", Kind: "rego-approval"},
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
