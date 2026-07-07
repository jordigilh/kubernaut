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

package acm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/readiness"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// localAlwaysFalse isolates the remote (ACM) path under test by ensuring the
// local scope checker never claims a resource as managed.
type localAlwaysFalse struct{}

func (l *localAlwaysFalse) IsManagedResource(_ context.Context, _ scope.ResourceIdentity) (bool, error) {
	return false, nil
}

// IT-ACM-054-001
//
// Pyramid Invariant: IT proves wiring.
// This test exercises the production factory dispatch path:
//
//	fleet.NewScopeChecker (factory, BackendACM)
//	  -> FederatedScopeChecker (local/remote router)
//	    -> acm.Client (production GraphQL client)
//	      -> real HTTP transport
//	        -> fake ACM Search GraphQL server (httptest)
//
// The fake GraphQL server returns controlled responses so the test can verify
// that the factory correctly wires acm.Client and that the full dispatch path
// produces the correct scope decision.
//
// Wiring Manifest:
//
//	acm.Client               -> pkg/fleet/acm/client.go        -> IT-ACM-054-001
//	Factory BackendACM       -> pkg/fleet/scope_factory.go      -> IT-ACM-054-001
//	FederatedScopeChecker    -> pkg/fleet/federated_checker.go  -> IT-ACM-054-001
var _ = Describe("ACM Search Factory Wiring (BR-INTEGRATION-065)", Ordered, Label("acm", "integration"), func() {
	var (
		graphQLServer *httptest.Server
		fedChecker    scope.ScopeChecker
		nextCount     int
	)

	BeforeAll(func() {
		By("Starting fake ACM Search GraphQL server")
		graphQLServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.Method).To(Equal(http.MethodPost), "ACM Search uses POST for GraphQL")
			Expect(r.URL.Path).To(Equal("/searchapi/graphql"), "endpoint path must match ACM convention")

			w.Header().Set("Content-Type", "application/json")
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"searchResult": []map[string]interface{}{
						{"count": nextCount},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))

		By("Creating FederatedScopeChecker via production factory with BackendACM")
		tokenPath := filepath.Join(GinkgoT().TempDir(), "token")
		Expect(os.WriteFile(tokenPath, []byte("it-acm-054-001-token"), 0o600)).To(Succeed())
		cfg := fleet.FleetConfig{
			Enabled:   true,
			Backend:   fleet.BackendACM,
			Endpoint:  graphQLServer.URL,
			TokenPath: tokenPath,
		}
		var err error
		fedChecker, err = fleet.NewScopeChecker(&localAlwaysFalse{}, cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred(),
			"fleet.NewScopeChecker with BackendACM must succeed — factory must wire acm.Client")
	})

	AfterAll(func() {
		if graphQLServer != nil {
			graphQLServer.Close()
		}
	})

	// AC-4 Information Flow Enforcement: proves that the production factory
	// correctly dispatches through acm.Client to the ACM Search backend, and
	// the positive scope decision flows back through FederatedScopeChecker.
	Describe("IT-ACM-054-001 [AC-4]: factory BackendACM dispatches through acm.Client", func() {
		It("Sub-case A: managed resource (count=1) returns managed=true through full factory path", func() {
			nextCount = 1

			managed, err := fedChecker.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east",
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Namespace: "default",
				Name:      "nginx",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(),
				"AC-4: managed resource must return true through factory -> "+
					"FederatedScopeChecker -> acm.Client -> GraphQL server path")
		})

		It("Sub-case B: unmanaged resource (count=0) returns managed=false through full factory path", func() {
			nextCount = 0

			managed, err := fedChecker.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east",
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Namespace: "orphan-ns",
				Name:      "no-such-deploy",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse(),
				"AC-4: absent resource must return false through the full production path")
		})
	})
})

// IT-ACM-054-002
//
// #1556: ACM Search mandatorily requires bearer-token auth. Before this fix,
// fleet.NewScopeChecker's BackendACM case never composed an Authorization
// header, regardless of configuration. This is the Wiring Manifest proof for
// that composition: factory -> auth.AuthTransport -> acm.Client -> real HTTP
// wire, exercised through the same production entry point (NewScopeChecker)
// as UT-FLEET-FAC-008 — kept as a distinct IT per CHECKPOINT W (Wiring
// Manifest requires an explicit IT test ID per wiring point) rather than
// relying on the UT tier alone.
var _ = Describe("ACM Search Factory Bearer-Token Auth (BR-INTEGRATION-065, #1556)", Label("acm", "integration"), func() {
	It("IT-ACM-054-002 [SC-7,IA-5]: factory-composed transport sends Authorization: Bearer <token> on the real wire", func() {
		var gotAuthHeader string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuthHeader = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":1}]}}`))
		}))
		defer server.Close()

		tokenPath := filepath.Join(GinkgoT().TempDir(), "token")
		Expect(os.WriteFile(tokenPath, []byte("it-acm-054-002-token"), 0o600)).To(Succeed())

		cfg := fleet.FleetConfig{
			Enabled:   true,
			Backend:   fleet.BackendACM,
			Endpoint:  server.URL,
			TokenPath: tokenPath,
		}
		checker, err := fleet.NewScopeChecker(&localAlwaysFalse{}, cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred())

		_, err = checker.IsManagedResource(context.Background(), scope.ResourceIdentity{
			ClusterID: "prod-east", Kind: "Deployment", Namespace: "default", Name: "nginx",
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(gotAuthHeader).To(Equal("Bearer it-acm-054-002-token"),
			"SC-7/IA-5: the production factory dispatch path must authenticate "+
				"every ACM Search request with the configured bearer token")
	})
})

// IT-ACM-054-003
//
// #1556: the readiness gate (pkg/fleet/readiness.ScopeCheckerProber) relies on
// acm.Client.Ping to surface real backend errors so /readyz correctly flips to
// NotReady when ACM Search rejects the request. This proves that an
// authentication failure (401, e.g. expired/rotated ServiceAccount token) is
// not silently swallowed — the service must fail closed, not silently degrade
// to unauthenticated (and therefore always-failing) scope checks.
var _ = Describe("ACM Search Factory Readiness Fail-Closed on 401 (BR-INTEGRATION-065, #1556)", Label("acm", "integration"), func() {
	It("IT-ACM-054-003 [AC-4,IA-5]: Ping surfaces a 401 from ACM Search instead of swallowing it", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer it-acm-054-003-valid-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":0}]}}`))
		}))
		defer server.Close()

		tokenPath := filepath.Join(GinkgoT().TempDir(), "token")
		Expect(os.WriteFile(tokenPath, []byte("it-acm-054-003-wrong-token"), 0o600)).To(Succeed())

		cfg := fleet.FleetConfig{
			Enabled:   true,
			Backend:   fleet.BackendACM,
			Endpoint:  server.URL,
			TokenPath: tokenPath,
		}
		checker, err := fleet.NewScopeChecker(&localAlwaysFalse{}, cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred())

		fed, ok := checker.(*fleet.FederatedScopeChecker)
		Expect(ok).To(BeTrue(), "factory must return *fleet.FederatedScopeChecker for BackendACM")
		pinger, ok := fed.Remote().(readiness.Pinger)
		Expect(ok).To(BeTrue(), "the ACM backend must satisfy the readiness.Pinger interface "+
			"(this is what wireFleetReadinessGate relies on in cmd/gateway/main.go and cmd/remediationorchestrator/main.go)")

		err = pinger.Ping(context.Background())
		Expect(err).To(HaveOccurred(),
			"AC-4/IA-5: readiness gate must fail closed when ACM Search rejects the bearer token")
		Expect(err.Error()).To(ContainSubstring("401"),
			"the surfaced error must reflect the real 401 from ACM Search, not a swallowed/generic failure")
	})
})

// IT-ACM-054-004
//
// Pyramid Invariant closure for the Ping() query-shape fix (#1556 spike,
// 2026-07-07, live ACM 2.16.2 on OCP): UT-ACM-054-013 proves the corrected
// query shape in isolation (acm.Client constructed directly); this proves
// the SAME fix through the real production wiring chain — factory ->
// FederatedScopeChecker.Remote() -> readiness.Pinger -> acm.Client.Ping() —
// i.e. exactly what cmd/gateway/main.go and cmd/remediationorchestrator/main.go
// wire into the readiness gate. The fake server mirrors real ACM Search's
// rejection of empty/nil filter sets, so this also regression-guards against
// Ping() ever reverting to an empty filter while going through the actual
// dispatch path, not just the unit-level client.
var _ = Describe("ACM Search Factory Readiness Reports Ready When Healthy (BR-INTEGRATION-065, #1556)", Label("acm", "integration"), func() {
	It("IT-ACM-054-004 [AC-4,IA-5]: Ping succeeds through the full production wiring chain against a healthy, authenticated backend", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer it-acm-054-004-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			body, _ := io.ReadAll(r.Body)
			var req map[string]interface{}
			_ = json.Unmarshal(body, &req)
			vars, _ := req["variables"].(map[string]interface{})
			inputs, _ := vars["input"].([]interface{})

			hasFilter := false
			if len(inputs) > 0 {
				if input, ok := inputs[0].(map[string]interface{}); ok {
					if filters, ok := input["filters"].([]interface{}); ok && len(filters) > 0 {
						hasFilter = true
					}
				}
			}

			w.Header().Set("Content-Type", "application/json")
			if !hasFilter {
				// Mirrors real ACM Search v2.16.2's rejection of empty/nil
				// filter sets, confirmed against a live cluster.
				_, _ = w.Write([]byte(`{"errors":[{"message":"query input must contain a filter or keyword"}],"data":{"searchResult":[{"count":null}]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":105}]}}`))
		}))
		defer server.Close()

		tokenPath := filepath.Join(GinkgoT().TempDir(), "token")
		Expect(os.WriteFile(tokenPath, []byte("it-acm-054-004-token"), 0o600)).To(Succeed())

		cfg := fleet.FleetConfig{
			Enabled:   true,
			Backend:   fleet.BackendACM,
			Endpoint:  server.URL,
			TokenPath: tokenPath,
		}
		checker, err := fleet.NewScopeChecker(&localAlwaysFalse{}, cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred())

		fed, ok := checker.(*fleet.FederatedScopeChecker)
		Expect(ok).To(BeTrue(), "factory must return *fleet.FederatedScopeChecker for BackendACM")
		pinger, ok := fed.Remote().(readiness.Pinger)
		Expect(ok).To(BeTrue(), "the ACM backend must satisfy the readiness.Pinger interface "+
			"(this is what wireFleetReadinessGate relies on in cmd/gateway/main.go and cmd/remediationorchestrator/main.go)")

		err = pinger.Ping(context.Background())
		Expect(err).ToNot(HaveOccurred(),
			"AC-4/IA-5: the readiness gate must report the ACM backend as ready through "+
				"the real production wiring chain when it is healthy and authenticated — "+
				"a query-shape regression here would cause every ACM-backed deployment's "+
				"/readyz to permanently report NotReady")
	})
})
