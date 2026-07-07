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
	"net/http/httptest"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

// IT-FLEET-READY-KA-001: cmd/kubernautagent must wire a readiness.Gate from
// the resilient client produced by registerFleetTools into readinessHandler
// (via healthServersParams.FleetGate) — the actual production entry point.
// Without it, an unreachable Fleet MCP Gateway only logs an error (previous
// fail-open behavior) instead of flipping the whole pod NotReady (ADR-068
// decision #11, BR-INTEGRATION-054, #1553). KA has no scope-checker or
// cluster-registry dependency (confirmed via search: registerFleetTools only
// ever constructs an MCP client + GatewayDiscoverer), so its gate only ever
// carries an MCPClientProber.
//
// Test Plan: Wave 5 of the fail-closed Fleet readiness gate rollout (#1553).
var _ = Describe("registerFleetTools and wireFleetReadinessGate wiring (#1553)", func() {

	Describe("registerFleetTools retention behavior", func() {
		It("IT-KA-1553-001: is a no-op when fleet is disabled (no client, no tools)", func() {
			cfg := kaconfig.DefaultConfig()
			reg := registry.New()

			fc, toolNames := registerFleetTools(context.Background(), cfg, reg, logr.Discard())
			Expect(fc).To(BeNil(), "*mcpclient.ResilientClient must remain nil when fleet gatewayType/endpoint are unset")
			Expect(toolNames).To(BeNil(), "no tool names expected when fleet is disabled")
		})

		It("IT-KA-1553-001: wires the client and tools when the gateway is reachable", func() {
			gw := mockgw.NewMockGateway()
			DeferCleanup(gw.Close)

			cfg := kaconfig.DefaultConfig()
			cfg.Integrations.Fleet.Endpoint = gw.URL()
			cfg.Integrations.Fleet.GatewayType = "eaigw"
			reg := registry.New()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			DeferCleanup(cancel)

			fc, _ := registerFleetTools(ctx, cfg, reg, logr.Discard())
			Expect(fc).NotTo(BeNil(), "*mcpclient.ResilientClient must be returned when the Fleet MCP Gateway is reachable")
			DeferCleanup(func() { _ = fc.Close() })

			Expect(fc.Ready()).To(BeTrue(), "client must report Ready() when the initial connection succeeded")
		})

		It("IT-KA-1553-001: retains the client (not discarded) when the gateway is unreachable", func() {
			cfg := kaconfig.DefaultConfig()
			cfg.Integrations.Fleet.Endpoint = "http://127.0.0.1:1/unreachable"
			cfg.Integrations.Fleet.GatewayType = "eaigw"
			reg := registry.New()

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			DeferCleanup(cancel)

			fc, toolNames := registerFleetTools(ctx, cfg, reg, logr.Discard())
			Expect(fc).NotTo(BeNil(), "*mcpclient.ResilientClient must be kept (not discarded) when the Fleet "+
				"MCP Gateway is unreachable so the readiness gate's periodic probe can keep retrying it (#1553)")
			DeferCleanup(func() { _ = fc.Close() })

			Expect(fc.Ready()).To(BeFalse(), "the kept client must not report Ready() when its initial connection failed")
			Expect(toolNames).To(BeNil(), "no fleet tools should be registered when the initial connection failed")
		})
	})

	Describe("wireFleetReadinessGate wiring", func() {
		It("IT-FLEET-READY-KA-001a: remains nil when fleetClient is nil", func() {
			gate := wireFleetReadinessGate(context.Background(), nil, logr.Discard())
			Expect(gate).To(BeNil())
		})

		It("IT-FLEET-READY-KA-001b: reports ready immediately when a resilient client is present and reachable", func() {
			gw := mockgw.NewMockGateway()
			DeferCleanup(gw.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			DeferCleanup(cancel)

			resilienceCfg := mcpclient.DefaultResilienceConfig()
			resilienceCfg.MaxElapsedTime = 5 * time.Second
			fleetClient, err := mcpclient.NewResilient(ctx, gw.URL(), resilienceCfg, logr.Discard())
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = fleetClient.Close() })

			gate := wireFleetReadinessGate(ctx, fleetClient, logr.Discard())
			Expect(gate).NotTo(BeNil(), "readiness.Gate must be wired when a resilient client is present")
			DeferCleanup(gate.Stop)

			Expect(gate.Check(httptest.NewRequest("GET", "/readyz", nil))).NotTo(HaveOccurred(),
				"gate must report ready immediately when the MCP Gateway is reachable")
		})

		It("IT-FLEET-READY-KA-001c: reports NotReady when the gateway is unreachable", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			DeferCleanup(cancel)

			resilienceCfg := mcpclient.DefaultResilienceConfig()
			resilienceCfg.InitialInterval = 50 * time.Millisecond
			resilienceCfg.MaxElapsedTime = 500 * time.Millisecond
			fleetClient, connErr := mcpclient.NewResilient(ctx, "http://127.0.0.1:1/unreachable", resilienceCfg, logr.Discard())
			_ = connErr
			if fleetClient != nil {
				DeferCleanup(func() { _ = fleetClient.Close() })
			}

			gate := wireFleetReadinessGate(ctx, fleetClient, logr.Discard())
			Expect(gate).NotTo(BeNil(), "readiness.Gate must still be wired (and report NotReady) when the MCP Gateway is unreachable")
			DeferCleanup(gate.Stop)

			err := gate.Check(httptest.NewRequest("GET", "/readyz", nil))
			Expect(err).To(HaveOccurred(), "BR-INTEGRATION-054 / #1553: gate must report NotReady when the configured "+
				"MCP Gateway is unreachable, so Kubernetes removes the pod from Service endpoints (pod-wide fail closed)")
		})
	})
})
