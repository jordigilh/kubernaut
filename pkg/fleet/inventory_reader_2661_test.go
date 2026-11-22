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

	"github.com/jordigilh/kubernaut/pkg/fleet"
	fmcclient "github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
)

// Tier 1c (TP-2661 §8): the `fleet.fmc.endpoint` key selects the FMC
// inventory endpoint for the AF Console reader. The scope adapter's
// `fleet.endpoint` (cfg.Fleet.EffectiveEndpoint()) is untouched (DD-FLEET-017
// D2.1a), and the inventory path never inherits it, the auto-derived FMC URL,
// or the ACM Search URL.
var _ = Describe("BR-INTEGRATION-065: fleet.fmc.endpoint selects the AF inventory reader", func() {
	It("UT-AF-CFG-2661-001 [BR-INTEGRATION-065]: the fleet.fmc.endpoint key parses and selects the FMC inventory endpoint, never fleet.endpoint", func() {
		var fmcRequests atomic.Int32
		fmc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			fmcRequests.Add(1)
			Expect(r.URL.Path).To(Equal(fmcclient.InventoryPath), "the reader must target the FMC inventory route")
			w.Header().Set("Content-Type", "application/json")
			Expect(json.NewEncoder(w).Encode(inventory.Page{
				State:        inventory.StateEmpty,
				ObservedAt:   time.Now().UTC(),
				Freshness:    inventory.Freshness{State: inventory.FreshnessFresh, AsOf: time.Now().UTC()},
				Completeness: inventory.CompletenessComplete,
			})).To(Succeed())
		}))
		DeferCleanup(fmc.Close)
		var scopeRequests atomic.Int32
		scopeAdapter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			scopeRequests.Add(1)
			w.WriteHeader(http.StatusTeapot)
		}))
		DeferCleanup(scopeAdapter.Close)

		var parsed struct {
			Fleet fleet.FleetConfig `yaml:"fleet"`
		}
		Expect(yaml.Unmarshal([]byte(`
fleet:
  enabled: true
  backend: fleetmetadatacache
  endpoint: `+scopeAdapter.URL+`
  fmc:
    endpoint: `+fmc.URL+`
`), &parsed)).To(Succeed())
		cfg := parsed.Fleet
		Expect(cfg.Enabled).To(BeTrue())
		Expect(cfg.FMC.Endpoint).To(Equal(fmc.URL), "fleet.fmc.endpoint must parse as its own key")
		token := filepath.Join(GinkgoT().TempDir(), "token")
		Expect(os.WriteFile(token, []byte("fmc-test"), 0o600)).To(Succeed())
		cfg.TokenPath = token

		// D2.1a: the scope adapter's endpoint resolution is unchanged.
		Expect(cfg.EffectiveEndpoint()).To(Equal(scopeAdapter.URL))

		reader, err := fleet.NewInventoryReader(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(reader).NotTo(BeNil(), "the inventory reader is built from fleet.fmc.endpoint")

		// Selection, not just construction: the read must land on the FMC
		// fixture, and the scope adapter's endpoint must receive nothing.
		page, err := reader.List(context.Background(), inventory.Filters{ClusterID: "prod-east"})
		Expect(err).NotTo(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateEmpty))
		Expect(fmcRequests.Load()).To(Equal(int32(1)), "the inventory read reaches the fleet.fmc.endpoint fixture")
		Expect(scopeRequests.Load()).To(BeZero(), "the inventory path never reads fleet.endpoint")
	})

	It("UT-AF-CFG-2661-002 [BR-INTEGRATION-065, BR-SAFETY-001]: fleet.fmc.endpoint absent under backend fmc fails closed, never inheriting another endpoint", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  fleet.BackendFMC,
			Endpoint: "https://scope-adapter.example:8443",
		}
		reader, err := fleet.NewInventoryReader(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(reader).To(BeNil(), "without fleet.fmc.endpoint the inventory path must fail closed to no reader")
	})

	It("UT-AF-CFG-2661-003 [BR-INTEGRATION-065, BR-SAFETY-001]: backend acm builds the FMC reader from fleet.fmc.endpoint and never a direct ACM Search client", func() {
		configured := fleet.FleetConfig{
			Enabled:   true,
			Backend:   fleet.BackendACM,
			Endpoint:  "https://acm-search.example:4010",
			TokenPath: "/etc/apifrontend/acm/token",
			FMC:       fleet.FleetFMCConfig{Endpoint: "https://fmc.example:8443"},
		}
		reader, err := fleet.NewInventoryReader(configured)
		Expect(err).NotTo(HaveOccurred())
		Expect(reader).NotTo(BeNil(), "with fleet.fmc.endpoint configured the FMC reader is wired under backend acm (DD-AF-018 gate removed)")

		unconfigured := configured
		unconfigured.FMC.Endpoint = ""
		reader, err = fleet.NewInventoryReader(unconfigured)
		Expect(err).NotTo(HaveOccurred())
		Expect(reader).To(BeNil(), "without fleet.fmc.endpoint the inventory path must fail closed even under backend acm")
	})
})
