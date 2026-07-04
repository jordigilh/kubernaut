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
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

// IT-FLEET-READY-GW-001: cmd/gateway must wire a readiness.Gate from
// Config.Fleet + the resilient client produced by registerAdapters into
// srv.SetFleetReadinessGate — this is the actual production entry point.
// Without it, an unreachable Fleet MCP Gateway or scope-check backend only
// logs an error (BR-INTEGRATION-065's previous fail-open behavior) instead
// of flipping the whole pod NotReady (ADR-068, #1553).
//
// Test Plan: Wave 2 of the fail-closed Fleet readiness gate rollout (#1553).

func TestWireFleetReadinessGate_Disabled_NoGate(t *testing.T) {
	srv := newTestGatewayServer(t)
	cfg := testServerConfig() // Fleet.Enabled defaults to false

	gate := wireFleetReadinessGate(context.Background(), srv, nil, cfg, logr.Discard())
	if gate != nil {
		t.Fatal("IT-FLEET-READY-GW-001a: readiness.Gate must remain nil when Fleet is disabled — " +
			"readinessHandler must not gain a fleet dependency it was never configured with")
	}
}

// TestWireFleetReadinessGate_EnabledReachable_ReadyImmediately proves the
// production wiring: a real mock MCP Gateway that's up at startup produces
// a Gate whose Check() reports ready without any additional polling delay
// (Gate.Start probes synchronously).
func TestWireFleetReadinessGate_EnabledReachable_ReadyImmediately(t *testing.T) {
	gw := mockgw.NewMockGateway()
	t.Cleanup(gw.Close)

	srv := newTestGatewayServer(t)
	cfg := testServerConfig()
	cfg.Fleet.Enabled = true
	cfg.Fleet.MCPGatewayEndpoint = gw.URL()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resilienceCfg := mcpclient.DefaultResilienceConfig()
	resilienceCfg.MaxElapsedTime = 5 * time.Second
	fleetClient, err := mcpclient.NewResilient(ctx, gw.URL(), resilienceCfg, logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error connecting to mock MCP Gateway: %v", err)
	}
	t.Cleanup(func() { _ = fleetClient.Close() })

	gate := wireFleetReadinessGate(ctx, srv, fleetClient, cfg, logr.Discard())
	if gate == nil {
		t.Fatal("IT-FLEET-READY-GW-001b: readiness.Gate must be wired when Fleet is enabled and a resilient client is present")
	}
	t.Cleanup(gate.Stop)

	if err := gate.Check(httptest.NewRequest("GET", "/readyz", nil)); err != nil {
		t.Fatalf("IT-FLEET-READY-GW-001b: gate must report ready immediately after Start when the MCP Gateway "+
			"is reachable, got error: %v", err)
	}
}

// TestWireFleetReadinessGate_EnabledUnreachable_NotReady pins the new
// fail-closed contract (replacing the old fail-open one pinned by
// TestRegisterAdapters_FleetEnabledUnreachable in main_helpers_test.go):
// when the MCP Gateway is configured but unreachable, the wired gate must
// report NotReady so /readyz fails closed instead of silently degrading.
func TestWireFleetReadinessGate_EnabledUnreachable_NotReady(t *testing.T) {
	srv := newTestGatewayServer(t)
	cfg := testServerConfig()
	cfg.Fleet.Enabled = true
	cfg.Fleet.MCPGatewayEndpoint = "http://127.0.0.1:1/unreachable"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resilienceCfg := mcpclient.DefaultResilienceConfig()
	resilienceCfg.InitialInterval = 50 * time.Millisecond
	resilienceCfg.MaxElapsedTime = 500 * time.Millisecond
	fleetClient, connErr := mcpclient.NewResilient(ctx, cfg.Fleet.MCPGatewayEndpoint, resilienceCfg, logr.Discard())
	_ = connErr // fail-open at the client-construction level is expected; the gate is what must fail closed
	if fleetClient != nil {
		t.Cleanup(func() { _ = fleetClient.Close() })
	}

	gate := wireFleetReadinessGate(ctx, srv, fleetClient, cfg, logr.Discard())
	if gate == nil {
		t.Fatal("IT-FLEET-READY-GW-001c: readiness.Gate must still be wired (and report NotReady) when Fleet is " +
			"enabled but the MCP Gateway is currently unreachable — a nil gate would silently skip the " +
			"readiness check entirely, reintroducing the fail-open bug")
	}
	t.Cleanup(gate.Stop)

	if err := gate.Check(httptest.NewRequest("GET", "/readyz", nil)); err == nil {
		t.Fatal("BR-INTEGRATION-065 / #1553: gate must report NotReady when the configured MCP Gateway " +
			"is unreachable, so Kubernetes removes the pod from Service endpoints (pod-wide fail closed)")
	}
}
