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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet"
)

func TestFleet(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fleet Package Suite")
}

var _ = Describe("FleetConfig shared type (Phase E)", func() {
	It("UT-FLEET-CFG-001 [CM-6]: FleetConfig provides unified configuration via Backend+Endpoint", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "fmc",
			Endpoint: "http://fmc:8080",
		}

		Expect(cfg.Enabled).To(BeTrue())
		Expect(cfg.Backend).To(Equal("fmc"))
		Expect(cfg.Endpoint).To(Equal("http://fmc:8080"))
	})

	It("UT-FLEET-CFG-002 [CM-6]: Validate rejects empty Endpoint when fleet is enabled", func() {
		cfg := fleet.FleetConfig{
			Enabled: true,
			Backend: "fmc",
		}

		err := cfg.Validate()
		Expect(err).To(HaveOccurred(),
			"FleetConfig.Validate must reject empty Endpoint when enabled (CM-6)")
	})

	It("UT-FLEET-CFG-003 [CM-6]: Validate accepts disabled fleet without Backend/Endpoint", func() {
		cfg := fleet.FleetConfig{
			Enabled: false,
		}

		err := cfg.Validate()
		Expect(err).ToNot(HaveOccurred(),
			"disabled fleet should not require Backend or Endpoint")
	})
})

var _ = Describe("FleetConfig — BackendValkey removal (Phase 3)", func() {
	It("UT-SF-054-002 [CM-6]: Validate rejects BackendValkey as unsupported", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "valkey",
			Endpoint: "valkey:6379",
		}

		err := cfg.Validate()
		Expect(err).To(HaveOccurred(),
			"valkey backend must be rejected after legacy removal")
		Expect(err.Error()).To(ContainSubstring("unsupported backend"))
	})

	It("UT-SF-054-003 [CM-6]: EffectiveEndpoint returns only Endpoint, no ValkeyAddr fallback", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "fmc",
			Endpoint: "http://fmc:8080",
		}
		Expect(cfg.EffectiveEndpoint()).To(Equal("http://fmc:8080"))

		cfgEmpty := fleet.FleetConfig{
			Enabled: true,
			Backend: "fmc",
		}
		Expect(cfgEmpty.EffectiveEndpoint()).To(BeEmpty(),
			"with no Endpoint and no ValkeyAddr fallback, must return empty string")
	})
})

var _ = Describe("FleetConfig adapter pattern (Phase 2)", func() {
	It("UT-FLEET-CFG-010 [CM-6]: FleetConfig exposes Backend and Endpoint fields", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "fmc",
			Endpoint: "http://fmc.kubernaut.svc:8080",
		}

		Expect(cfg.Backend).To(Equal("fmc"))
		Expect(cfg.Endpoint).To(Equal("http://fmc.kubernaut.svc:8080"))
	})

	It("UT-FLEET-CFG-011 [CM-6]: Validate rejects empty Endpoint when fleet is enabled", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "fmc",
			Endpoint: "",
		}

		err := cfg.Validate()
		Expect(err).To(HaveOccurred(),
			"must reject empty Endpoint when fleet is enabled")
	})

	It("UT-FLEET-CFG-012 [CM-6]: Validate accepts disabled fleet without Backend/Endpoint", func() {
		cfg := fleet.FleetConfig{
			Enabled: false,
		}

		err := cfg.Validate()
		Expect(err).ToNot(HaveOccurred())
	})

	It("UT-FLEET-CFG-013 [CM-6]: Validate rejects unsupported Backend value", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "unsupported",
			Endpoint: "http://something:8080",
		}

		err := cfg.Validate()
		Expect(err).To(HaveOccurred(),
			"must reject unknown backend types")
	})

	It("UT-FLEET-CFG-014 [CM-6]: Validate accepts fmc backend", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "fmc",
			Endpoint: "http://fmc.kubernaut.svc:8080",
		}

		err := cfg.Validate()
		Expect(err).ToNot(HaveOccurred())
	})

	It("UT-FLEET-CFG-015 [CM-6]: Validate accepts acm backend", func() {
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  "acm",
			Endpoint: "https://search-api.open-cluster-management.svc:4010",
		}

		err := cfg.Validate()
		Expect(err).ToNot(HaveOccurred())
	})
})
