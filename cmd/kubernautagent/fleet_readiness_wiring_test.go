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
	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/readiness"
	infrastructure "github.com/jordigilh/kubernaut/test/infrastructure"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

// eaigwGatewayType is the fleet.GatewayType value for the Envoy AI Gateway
// backend, used across this file's fleet readiness wiring specs (goconst
// dedup).
const eaigwGatewayType = "eaigw"

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
		It("IT-KA-1553-001: is a no-op when fleet is disabled (no client, no resolver)", func() {
			cfg := kaconfig.DefaultConfig()

			fc, resolver := registerFleetTools(context.Background(), cfg, logr.Discard())
			Expect(fc).To(BeNil(), "*mcpclient.ResilientClient must remain nil when fleet gatewayType/endpoint are unset")
			Expect(resolver).To(BeNil(), "no FleetOverlayResolver expected when fleet is disabled")
		})

		It("IT-KA-1553-001: wires the client and a fleet overlay resolver when the gateway is reachable", func() {
			gw := mockgw.NewMockGateway()
			DeferCleanup(gw.Close)

			cfg := kaconfig.DefaultConfig()
			cfg.Integrations.Fleet.Endpoint = gw.URL()
			cfg.Integrations.Fleet.GatewayType = eaigwGatewayType

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			DeferCleanup(cancel)

			fc, resolver := registerFleetTools(ctx, cfg, logr.Discard())
			Expect(fc).NotTo(BeNil(), "*mcpclient.ResilientClient must be returned when the Fleet MCP Gateway is reachable")
			DeferCleanup(func() { _ = fc.Close() })
			Expect(resolver).NotTo(BeNil(), "DD-FLEET-005: registerFleetTools must return a FleetOverlayResolver when the gateway is reachable")

			Expect(fc.Ready()).To(BeTrue(), "client must report Ready() when the initial connection succeeded")
		})

		It("IT-KA-1553-001: retains the client and still returns a self-healing resolver when the gateway is unreachable (issue #2315)", func() {
			cfg := kaconfig.DefaultConfig()
			cfg.Integrations.Fleet.Endpoint = "http://127.0.0.1:1/unreachable"
			cfg.Integrations.Fleet.GatewayType = eaigwGatewayType

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			DeferCleanup(cancel)

			fc, resolver := registerFleetTools(ctx, cfg, logr.Discard())
			Expect(fc).NotTo(BeNil(), "*mcpclient.ResilientClient must be kept (not discarded) when the Fleet "+
				"MCP Gateway is unreachable so the readiness gate's periodic probe can keep retrying it (#1553)")
			DeferCleanup(func() { _ = fc.Close() })

			Expect(fc.Ready()).To(BeFalse(), "the kept client must not report Ready() when its initial connection failed")

			// Issue #2315: a fleet overlay resolver must still be returned
			// even when the initial connect fails -- it resolves the live
			// session via SessionProvider on every Overlay() call, so it
			// self-heals once fc reconnects in the background instead of
			// leaving fleet tools permanently unavailable until a pod
			// restart. Calling it now (while still disconnected) must
			// return a clear transient error, not panic and not silently
			// succeed with stale/no data.
			Expect(resolver).NotTo(BeNil(), "issue #2315: registerFleetTools must always return a self-healing "+
				"FleetOverlayResolver once fleet mode is configured, even when the initial connect attempt failed")

			_, overlayErr := resolver.Overlay(ctx, "remote-cluster")
			Expect(overlayErr).To(HaveOccurred(), "Overlay must return a clear transient error while the gateway is still disconnected")
		})

		It("IT-KA-2315-001: the resolver self-heals once the gateway becomes reachable, without a restart", func() {
			gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("remote-cluster"))
			DeferCleanup(gw.Close)

			proxy, err := infrastructure.NewInterruptibleProxy(gw.URL()[len("http://"):])
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(proxy.Close)
			proxy.Pause()

			cfg := kaconfig.DefaultConfig()
			cfg.Integrations.Fleet.Endpoint = "http://" + proxy.Addr()
			cfg.Integrations.Fleet.GatewayType = eaigwGatewayType
			cfg.Integrations.Fleet.Resilience = fleet.FleetResilienceConfig{
				InitialInterval: 50 * time.Millisecond,
				MaxInterval:     100 * time.Millisecond,
				MaxElapsedTime:  500 * time.Millisecond,
				ConnectTimeout:  200 * time.Millisecond,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			DeferCleanup(cancel)

			fc, resolver := registerFleetTools(ctx, cfg, logr.Discard())
			Expect(fc).NotTo(BeNil())
			DeferCleanup(func() { _ = fc.Close() })
			Expect(resolver).NotTo(BeNil())
			Expect(fc.Ready()).To(BeFalse(), "must start disconnected: the proxy is paused")

			_, overlayErr := resolver.Overlay(ctx, "remote-cluster")
			Expect(overlayErr).To(HaveOccurred(), "must fail while the gateway is unreachable through the paused proxy")

			// The periodic Fleet readiness gate (wireFleetReadinessGate in
			// production) is what actually drives ResilientClient's
			// background reconnect via MCPClientProber -- NewResilient
			// itself only retries during its own initial connect call, per
			// its own doc comment. Reproduce that same wiring here with a
			// short interval instead of production's 15s
			// fleetReadinessProbeInterval, so the test doesn't need to wait
			// that long.
			gate := readiness.NewGate(100*time.Millisecond, logr.Discard(), &readiness.MCPClientProber{Client: fc})
			gate.Start(ctx)
			DeferCleanup(gate.Stop)

			proxy.Resume()

			Eventually(func() bool { return fc.Ready() }, 10*time.Second, 100*time.Millisecond).Should(BeTrue(),
				"ResilientClient must reconnect in the background once the proxy resumes forwarding")

			Eventually(func() error {
				_, err := resolver.Overlay(ctx, "remote-cluster")
				return err
			}, 10*time.Second, 100*time.Millisecond).ShouldNot(HaveOccurred(),
				"issue #2315: the SAME resolver returned at startup must succeed once the gateway becomes reachable, with no restart")
		})

		// Issue #2262 Phase 2: proves a chart-shaped
		// Config.Integrations.Fleet.Resilience override actually reaches the
		// real mcpclient.NewResilient call inside registerFleetTools
		// (cmd/kubernautagent/toolregistry.go), not just
		// mcpclient.ResilienceConfigFromFleet in isolation (already
		// unit-tested by UT-FLEET-RES-013/014). Asserts via
		// ResilientClient.ResilienceConfig() rather than timing, so the test
		// is deterministic.
		It("issue #2262: a Resilience override reaches the real NewResilient call", func() {
			cfg := kaconfig.DefaultConfig()
			cfg.Integrations.Fleet.Endpoint = "http://127.0.0.1:1/unreachable"
			cfg.Integrations.Fleet.GatewayType = eaigwGatewayType
			cfg.Integrations.Fleet.Resilience = fleet.FleetResilienceConfig{
				ConnectTimeout:       7 * time.Second,
				DiscoverProbeTimeout: 3 * time.Second,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			DeferCleanup(cancel)

			fc, _ := registerFleetTools(ctx, cfg, logr.Discard())
			Expect(fc).NotTo(BeNil(), "*mcpclient.ResilientClient must be kept (not discarded) when the Fleet MCP Gateway is unreachable (#1553)")
			DeferCleanup(func() { _ = fc.Close() })

			got := fc.ResilienceConfig()
			want := mcpclient.ResilienceConfigFromFleet(cfg.Integrations.Fleet.Resilience)
			Expect(got).To(Equal(want), "issue #2262 Phase 2: Config.Integrations.Fleet.Resilience did not reach the real "+
				"NewResilient call inside registerFleetTools")
			Expect(got.ConnectTimeout).To(Equal(7 * time.Second))
			Expect(got.DiscoverProbeTimeout).To(Equal(3 * time.Second))

			defaults := mcpclient.DefaultResilienceConfig()
			Expect(got.InitialInterval).To(Equal(defaults.InitialInterval), "fields left unset in the override must keep mcpclient.DefaultResilienceConfig()'s values")
			Expect(got.MaxInterval).To(Equal(defaults.MaxInterval))
			Expect(got.MaxElapsedTime).To(Equal(defaults.MaxElapsedTime))
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
