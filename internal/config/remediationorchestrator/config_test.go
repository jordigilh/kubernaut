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

package remediationorchestrator_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	config "github.com/jordigilh/kubernaut/internal/config/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/fleet"
)

func TestRemediationOrchestratorConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RemediationOrchestrator Config Test Suite")
}

// ADR-068/BR-FLEET-054: RO relies on BOTH FleetConfig capabilities to
// operate correctly: Backend/Endpoint (federated scope-check, wired via
// fleet.NewScopeChecker into the routing engine) AND MCPGatewayEndpoint
// (remote reads, wired via buildFleetReaderFactory into
// Reconciler.SetReaderFactory for CapturePreRemediationHash). Configuring
// only one leaves RO silently degraded to local-only behavior for
// fleet-routed RemediationRequests.
var _ = Describe("BR-FLEET-054/ADR-068: Fleet full-federation validation", func() {
	It("should accept default config with fleet disabled", func() {
		cfg := config.DefaultConfig()
		Expect(cfg.Validate()).ToNot(HaveOccurred())
	})

	It("should reject fleet enabled with only Backend/Endpoint (no MCPGatewayEndpoint)", func() {
		cfg := config.DefaultConfig()
		cfg.Fleet = fleet.FleetConfig{
			Enabled:  true,
			Backend:  "fleetmetadatacache",
			Endpoint: "http://fmc:8080",
		}

		err := cfg.Validate()
		Expect(err).To(HaveOccurred(),
			"RO cannot operate without degradation unless both fleet capabilities are configured")
		Expect(err.Error()).To(ContainSubstring("mcpGatewayEndpoint"))
	})

	It("should reject fleet enabled with only MCPGatewayEndpoint (no Backend/Endpoint)", func() {
		cfg := config.DefaultConfig()
		cfg.Fleet = fleet.FleetConfig{
			Enabled:            true,
			MCPGatewayEndpoint: "http://mcp-gateway:8080/mcp",
			MCPGatewayType:     fleet.GatewayEAIGW,
		}

		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("backend"))
	})

	It("should accept fleet enabled with both capabilities fully configured", func() {
		cfg := config.DefaultConfig()
		cfg.Fleet = fleet.FleetConfig{
			Enabled:            true,
			Backend:            "fleetmetadatacache",
			Endpoint:           "http://fmc:8080",
			MCPGatewayEndpoint: "http://mcp-gateway:8080/mcp",
			MCPGatewayType:     fleet.GatewayEAIGW,
		}

		Expect(cfg.Validate()).ToNot(HaveOccurred())
	})

	It("should accept fleet disabled regardless of partial config", func() {
		cfg := config.DefaultConfig()
		cfg.Fleet = fleet.FleetConfig{
			Enabled: false,
			Backend: "fleetmetadatacache",
		}

		Expect(cfg.Validate()).ToNot(HaveOccurred())
	})
})
