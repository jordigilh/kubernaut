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

package fleet_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

var _ = Describe("NewScopeChecker factory (BR-INTEGRATION-065)", func() {
	var local *mockLocalChecker

	BeforeEach(func() {
		local = &mockLocalChecker{managed: map[string]bool{}}
	})

	It("UT-FLEET-FAC-001: disabled fleet returns local checker unchanged", func() {
		cfg := fleet.FleetConfig{Enabled: false}
		checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred())
		Expect(checker).To(BeIdenticalTo(local),
			"disabled fleet must return the exact same local checker instance")
	})

	It("UT-FLEET-FAC-002: enabled fleet with empty endpoint and non-FMC backend returns local checker", func() {
		cfg := fleet.FleetConfig{Enabled: true, Backend: "acm", Endpoint: ""}
		checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred())
		Expect(checker).To(BeIdenticalTo(local),
			"empty endpoint for non-FMC backend must fall back to local checker")
	})

	It("UT-FLEET-FAC-003 [AC-4]: BackendFMC with HTTP endpoint returns FederatedScopeChecker using FMC HTTP client", func() {
		cfg := fleet.FleetConfig{Enabled: true, Backend: "fleetmetadatacache", Endpoint: "http://fmc:8080"}
		checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred())
		Expect(checker).ToNot(BeIdenticalTo(local),
			"fmc backend must wrap local checker with FederatedScopeChecker")

		_, isFederated := checker.(*fleet.FederatedScopeChecker)
		Expect(isFederated).To(BeTrue(),
			"factory must return *fleet.FederatedScopeChecker for fmc backend")
	})

	It("UT-FLEET-FAC-004 [CM-6]: BackendValkey is rejected as unsupported", func() {
		cfg := fleet.FleetConfig{Enabled: true, Backend: "valkey", Endpoint: "valkey:6379"}
		checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported backend"))
		Expect(checker).To(BeNil(),
			"factory must reject legacy valkey backend")
	})

	It("UT-FLEET-FAC-005 [AC-4]: BackendACM with endpoint returns FederatedScopeChecker using ACM client", func() {
		cfg := fleet.FleetConfig{
			Enabled:   true,
			Backend:   "acm",
			Endpoint:  "https://search-api:4010",
			TokenPath: "/etc/gateway/acm-token/token",
		}
		checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred())
		Expect(checker).ToNot(BeIdenticalTo(local),
			"acm backend must wrap local checker with FederatedScopeChecker")

		_, isFederated := checker.(*fleet.FederatedScopeChecker)
		Expect(isFederated).To(BeTrue(),
			"factory must return *fleet.FederatedScopeChecker for acm backend")
	})

	It("UT-FLEET-FAC-006: unknown backend returns error", func() {
		cfg := fleet.FleetConfig{Enabled: true, Backend: "unknown", Endpoint: "http://example:8080"}
		checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported backend"))
		Expect(checker).To(BeNil())
	})

	It("UT-FLEET-FAC-007 [CM-6]: empty backend with endpoint returns unsupported backend error", func() {
		cfg := fleet.FleetConfig{Enabled: true, Endpoint: "http://fmc:8080"}
		checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
		Expect(err).To(HaveOccurred(),
			"empty backend with endpoint must fail validation in factory")
		Expect(checker).To(BeNil())
	})

	// #1556: proves the factory actually composes the bearer-token transport,
	// not just that it returns a FederatedScopeChecker (UT-FLEET-FAC-005 only
	// checks the type, not the wire behavior).
	It("UT-FLEET-FAC-008 [SC-7,IA-5]: BackendACM with TokenPath set sends Authorization: Bearer <token> to the ACM endpoint", func() {
		var gotAuthHeader string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuthHeader = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":0}]}}`))
		}))
		defer server.Close()

		tokenPath := filepath.Join(GinkgoT().TempDir(), "token")
		Expect(os.WriteFile(tokenPath, []byte("test-acm-token"), 0o600)).To(Succeed())

		cfg := fleet.FleetConfig{
			Enabled:   true,
			Backend:   "acm",
			Endpoint:  server.URL,
			TokenPath: tokenPath,
		}
		checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred())

		_, err = checker.IsManagedResource(context.Background(), scope.ResourceIdentity{
			ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(gotAuthHeader).To(Equal("Bearer test-acm-token"),
			"SC-7/IA-5: factory-composed ACM client must authenticate every request "+
				"with the configured bearer token")
	})

	// #1556 defense-in-depth: FleetConfig.Validate() hard-requires TokenPath for
	// BackendACM (UT-FLEET-CFG-070), so this state is unreachable through normal
	// config loading. This test locks in the factory's own fail-safe behavior
	// for direct struct construction that bypasses Validate() (e.g. a future
	// caller or test helper) — it must never send a partial/malformed
	// Authorization header, only either a correct one or none at all.
	It("UT-FLEET-FAC-009 [AC-4]: BackendACM without TokenPath (Validate bypassed) sends no Authorization header", func() {
		var gotAuthHeader string
		sawRequest := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sawRequest = true
			gotAuthHeader = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":0}]}}`))
		}))
		defer server.Close()

		cfg := fleet.FleetConfig{Enabled: true, Backend: "acm", Endpoint: server.URL}
		checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred())

		_, err = checker.IsManagedResource(context.Background(), scope.ResourceIdentity{
			ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(sawRequest).To(BeTrue(), "request must still reach the ACM endpoint")
		Expect(gotAuthHeader).To(BeEmpty(),
			"AC-4: without a configured TokenPath the factory must never send a "+
				"partial or malformed Authorization header")
	})
})
