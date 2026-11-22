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

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/fleet"
	fmcclient "github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// TP-2661 Tier 2 (phase c): the AF inventory reader is built from
// fleet.fmc.endpoint through production wiring (buildPolicyReadDeps), and the
// fail-closed inventory state is observable at startup.
var _ = Describe("BR-INTEGRATION-065: AF→FMC inventory dispatch wiring (TP-2661 phase c)", func() {
	newDispatchFixture := func() (fmcURL, scopeURL, token string, fmcHits, scopeHits *atomic.Int32) {
		fmcRequests := &atomic.Int32{}
		fmcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			fmcRequests.Add(1)
			Expect(r.URL.Path).To(Equal(fmcclient.InventoryPath), "the dispatch must target the FMC inventory route")
			Expect(r.URL.Query().Get("cluster_id")).To(Equal("prod-east"))
			Expect(r.Header.Get("Authorization")).To(Equal("Bearer fmc-wiring-test"))
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
		DeferCleanup(fmcServer.Close)

		// The scope adapter's endpoint must never receive an inventory request.
		scopeRequests := &atomic.Int32{}
		scopeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			scopeRequests.Add(1)
			w.WriteHeader(http.StatusTeapot)
		}))
		DeferCleanup(scopeServer.Close)

		tokenFile := filepath.Join(GinkgoT().TempDir(), "token")
		Expect(os.WriteFile(tokenFile, []byte("fmc-wiring-test"), 0o600)).To(Succeed())
		return fmcServer.URL, scopeServer.URL, tokenFile, fmcRequests, scopeRequests
	}

	listThroughProductionWiring := func(backend, fmcURL, scopeURL, token string) inventory.Page {
		cfg := &config.Config{Fleet: fleet.FleetConfig{
			Enabled:   true,
			Backend:   backend,
			Endpoint:  scopeURL,
			TokenPath: token,
			FMC:       fleet.FleetFMCConfig{Endpoint: fmcURL},
		}}
		deps := &backendDeps{}
		Expect(buildPolicyReadDeps(cfg, deps, logr.Discard())).To(Succeed())
		Expect(deps.RemoteInventory).NotTo(BeNil())
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			RemoteInventory: deps.RemoteInventory, Authorizer: projectionAllow{},
		})
		Expect(err).NotTo(HaveOccurred())
		page, err := svc.ListInventory(context.Background(), &auth.UserIdentity{Username: "alice"}, inventory.Filters{ClusterID: "prod-east"})
		Expect(err).NotTo(HaveOccurred())
		return page
	}

	It("IT-AF-2661-001 [BR-INTEGRATION-065]: AF list dispatch reaches the endpoint from fleet.fmc.endpoint, never the scope adapter's, under both backends", func() {
		By("fleet.backend: fleetmetadatacache")
		fmcURL, scopeURL, token, fmcHits, scopeHits := newDispatchFixture()
		page := listThroughProductionWiring(fleet.BackendFMC, fmcURL, scopeURL, token)
		Expect(page.State).To(Equal(inventory.StateSuccess))
		Expect(page.Items).To(HaveLen(1))
		Expect(fmcHits.Load()).To(Equal(int32(1)), "the dispatch reaches the fleet.fmc.endpoint fixture")
		Expect(scopeHits.Load()).To(BeZero(), "the inventory path never reads fleet.endpoint")

		By("fleet.backend: acm (DD-AF-018 gate removed)")
		fmcURL, scopeURL, token, fmcHits, scopeHits = newDispatchFixture()
		page = listThroughProductionWiring(fleet.BackendACM, fmcURL, scopeURL, token)
		Expect(page.State).To(Equal(inventory.StateSuccess))
		Expect(fmcHits.Load()).To(Equal(int32(1)), "backend acm wires the same FMC reader from the new key")
		Expect(scopeHits.Load()).To(BeZero(), "the inventory path never reads the ACM Search URL")
	})

	It("IT-AF-2661-004 [BR-SAFETY-001]: startup observability of the fail-closed inventory state, including the misspelled-key shape", func() {
		logger := &capturingLogSink{}
		absent := &config.Config{Fleet: fleet.FleetConfig{
			Enabled:  true,
			Backend:  fleet.BackendFMC,
			Endpoint: "https://scope-adapter.example:8443",
		}}
		deps := &backendDeps{}
		Expect(buildPolicyReadDeps(absent, deps, logr.New(logger))).To(Succeed())
		Expect(deps.RemoteInventory).To(BeNil(), "fleet.fmc.endpoint absent leaves the inventory lane unwired")
		Expect(logger.messages).To(ContainElement(ContainSubstring("fleet.fmc.endpoint is not configured")),
			"the fail-closed inventory state must be logged at startup")

		By("a misspelled key under AF's non-strict decode behaves exactly like an absent key")
		var loaded config.Config
		Expect(yaml.Unmarshal([]byte(`
fleet:
  enabled: true
  backend: fleetmetadatacache
  fmc:
    endpointURL: https://fmc.example:8443
`), &loaded)).To(Succeed())
		Expect(loaded.Fleet.FMC.Endpoint).To(BeEmpty(), "the misspelled key is silently dropped by the non-strict loader")
		logger = &capturingLogSink{}
		deps = &backendDeps{}
		Expect(buildPolicyReadDeps(&loaded, deps, logr.New(logger))).To(Succeed())
		Expect(deps.RemoteInventory).To(BeNil())
		Expect(logger.messages).To(ContainElement(ContainSubstring("fleet.fmc.endpoint is not configured")),
			"the misspelled-key shape is observable, not silent")

		By("the wired branch logs list availability and the endpoint it actually uses")
		fmcURL, scopeURL, token, _, _ := newDispatchFixture()
		logger = &capturingLogSink{}
		deps = &backendDeps{}
		wired := &config.Config{Fleet: fleet.FleetConfig{
			Enabled: true, Backend: fleet.BackendFMC, Endpoint: scopeURL, TokenPath: token,
			FMC: fleet.FleetFMCConfig{Endpoint: fmcURL},
		}}
		Expect(buildPolicyReadDeps(wired, deps, logr.New(logger))).To(Succeed())
		Expect(deps.RemoteInventory).NotTo(BeNil())
		Expect(logger.messages).To(ContainElement(ContainSubstring("dispatch through FMC")))
		Expect(logger.pairs).To(ContainElement(fmcURL), "the non-nil log names cfg.Fleet.FMC.Endpoint, not the scope adapter's endpoint")
	})
})

// capturingLogSink records Info lines and their key/value pairs for startup
// observability assertions.
type capturingLogSink struct {
	messages []string
	pairs    []string
}

func (s *capturingLogSink) Init(logr.RuntimeInfo) {}
func (s *capturingLogSink) Enabled(int) bool      { return true }
func (s *capturingLogSink) Info(_ int, msg string, kv ...interface{}) {
	s.messages = append(s.messages, msg)
	for _, value := range kv {
		if text, ok := value.(string); ok {
			s.pairs = append(s.pairs, text)
		}
	}
}
func (s *capturingLogSink) Error(error, string, ...interface{}) {}
func (s *capturingLogSink) WithValues(kv ...interface{}) logr.LogSink {
	return s
}
func (s *capturingLogSink) WithName(string) logr.LogSink { return s }
