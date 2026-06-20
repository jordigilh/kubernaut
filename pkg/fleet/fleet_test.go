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
	It("UT-FLEET-CFG-001 [CM-6]: FleetConfig provides unified configuration for all fleet services", func() {
		cfg := fleet.FleetConfig{
			Enabled:    true,
			ValkeyAddr: "valkey:6379",
		}

		Expect(cfg.Enabled).To(BeTrue())
		Expect(cfg.ValkeyAddr).To(Equal("valkey:6379"),
			"FleetConfig must expose ValkeyAddr for consistent configuration across services (CM-6)")
	})

	It("UT-FLEET-CFG-002 [CM-6]: Validate rejects empty ValkeyAddr when fleet is enabled", func() {
		cfg := fleet.FleetConfig{
			Enabled:    true,
			ValkeyAddr: "",
		}

		err := cfg.Validate()
		Expect(err).To(HaveOccurred(),
			"FleetConfig.Validate must reject empty ValkeyAddr when enabled (CM-6: configuration settings)")
	})

	It("UT-FLEET-CFG-003 [CM-6]: Validate accepts disabled fleet with empty ValkeyAddr", func() {
		cfg := fleet.FleetConfig{
			Enabled:    false,
			ValkeyAddr: "",
		}

		err := cfg.Validate()
		Expect(err).ToNot(HaveOccurred(),
			"disabled fleet should not require ValkeyAddr")
	})
})
