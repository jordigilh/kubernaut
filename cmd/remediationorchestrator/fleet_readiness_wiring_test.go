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
	"k8s.io/apimachinery/pkg/runtime"
	crfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	config "github.com/jordigilh/kubernaut/internal/config/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// IT-FLEET-READY-RO-001: cmd/remediationorchestrator must wire a
// readiness.Gate from Config.Fleet + the resilient client produced by
// buildFleetReaderFactory + the routing engine's scope checker into
// setupRemediationOrchestratorControllers's mgr.AddReadyzCheck("fleet", ...)
// call — the actual production entry point. Without it, an unreachable
// Fleet MCP Gateway or scope-check backend only logs an error
// (BR-INTEGRATION-065's previous fail-open behavior) instead of flipping
// the whole pod NotReady (ADR-068, #1553).
//
// Test Plan: Wave 3 of the fail-closed Fleet readiness gate rollout
// (#1553).

func newTestRoutingEngine(checker scope.ScopeChecker) *routing.RoutingEngine {
	fakeClient := crfake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	return routing.NewRoutingEngine(fakeClient, fakeClient, "kubernaut-system", routing.Config{}, checker)
}

func TestWireFleetReadinessGate_RO_Disabled_NoGate(t *testing.T) {
	cfg := config.DefaultConfig() // Fleet.Enabled defaults to false
	routingEngine := newTestRoutingEngine(&mocks.AlwaysManagedScopeChecker{})

	gate := wireFleetReadinessGate(context.Background(), routingEngine, nil, cfg, logr.Discard())
	if gate != nil {
		t.Fatal("IT-FLEET-READY-RO-001a: readiness.Gate must remain nil when Fleet is disabled")
	}
}

func TestWireFleetReadinessGate_RO_EnabledReachable_ReadyImmediately(t *testing.T) {
	gw := mockgw.NewMockGateway()
	t.Cleanup(gw.Close)

	cfg := config.DefaultConfig()
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

	routingEngine := newTestRoutingEngine(&mocks.AlwaysManagedScopeChecker{})

	gate := wireFleetReadinessGate(ctx, routingEngine, fleetClient, cfg, logr.Discard())
	if gate == nil {
		t.Fatal("IT-FLEET-READY-RO-001b: readiness.Gate must be wired when Fleet is enabled and a resilient client is present")
	}
	t.Cleanup(gate.Stop)

	if err := gate.Check(httptest.NewRequest("GET", "/readyz", nil)); err != nil {
		t.Fatalf("IT-FLEET-READY-RO-001b: gate must report ready immediately after Start when the MCP "+
			"Gateway is reachable, got error: %v", err)
	}
}

func TestWireFleetReadinessGate_RO_EnabledUnreachable_NotReady(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Fleet.Enabled = true
	cfg.Fleet.MCPGatewayEndpoint = "http://127.0.0.1:1/unreachable"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resilienceCfg := mcpclient.DefaultResilienceConfig()
	resilienceCfg.InitialInterval = 50 * time.Millisecond
	resilienceCfg.MaxElapsedTime = 500 * time.Millisecond
	fleetClient, connErr := mcpclient.NewResilient(ctx, cfg.Fleet.MCPGatewayEndpoint, resilienceCfg, logr.Discard())
	_ = connErr
	if fleetClient != nil {
		t.Cleanup(func() { _ = fleetClient.Close() })
	}

	routingEngine := newTestRoutingEngine(&mocks.AlwaysManagedScopeChecker{})

	gate := wireFleetReadinessGate(ctx, routingEngine, fleetClient, cfg, logr.Discard())
	if gate == nil {
		t.Fatal("IT-FLEET-READY-RO-001c: readiness.Gate must still be wired (and report NotReady) when " +
			"Fleet is enabled but the MCP Gateway is currently unreachable")
	}
	t.Cleanup(gate.Stop)

	if err := gate.Check(httptest.NewRequest("GET", "/readyz", nil)); err == nil {
		t.Fatal("BR-INTEGRATION-065 / #1553: gate must report NotReady when the configured MCP Gateway " +
			"is unreachable, so Kubernetes removes the pod from Service endpoints (pod-wide fail closed)")
	}
}
