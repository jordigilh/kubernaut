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

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	weconfig "github.com/jordigilh/kubernaut/pkg/workflowexecution/config"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

// IT-FLEET-READY-WE-001: cmd/workflowexecution must wire a readiness.Gate
// from Config.Fleet + the resilient client produced by buildClientFactory
// into registerHealthChecks's mgr.AddReadyzCheck("fleet", ...) — the
// actual production entry point. Without it, an unreachable Fleet MCP
// Gateway only logs an error (BR-FLEET-054's previous fail-open behavior)
// instead of flipping the whole pod NotReady (ADR-068, #1553). WE has no
// scope-checker (confirmed via search: no ScopeChecker references
// anywhere in cmd/workflowexecution), so its gate only ever carries an
// MCPClientProber.
//
// Test Plan: Wave 4 of the fail-closed Fleet readiness gate rollout
// (#1553).

// --- buildClientFactory retention behavior (#1553) ---

func TestBuildClientFactory_Disabled_NoOp(t *testing.T) {
	cfg := weconfig.DefaultConfig()
	localClient := crfake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()

	_, fc := buildClientFactory(context.Background(), cfg, localClient, logr.Discard())
	if fc != nil {
		t.Error("IT-WE-1553-001: *mcpclient.ResilientClient must remain nil when fleet.endpoint is unset")
	}
}

func TestBuildClientFactory_EnabledReachable_WiresMCPFactory(t *testing.T) {
	gw := mockgw.NewMockGateway()
	t.Cleanup(gw.Close)

	cfg := weconfig.DefaultConfig()
	cfg.Fleet.Endpoint = gw.URL()
	localClient := crfake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, fc := buildClientFactory(ctx, cfg, localClient, logr.Discard())
	if fc == nil {
		t.Fatal("IT-WE-1553-001: *mcpclient.ResilientClient must be returned when the Fleet MCP Gateway is reachable")
	}
	t.Cleanup(func() { _ = fc.Close() })
	if !fc.Ready() {
		t.Error("IT-WE-1553-001: client must report Ready() when the initial connection succeeded")
	}
}

func TestBuildClientFactory_EnabledUnreachable_RetainsClient(t *testing.T) {
	cfg := weconfig.DefaultConfig()
	cfg.Fleet.Endpoint = "http://127.0.0.1:1/unreachable"
	localClient := crfake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, fc := buildClientFactory(ctx, cfg, localClient, logr.Discard())
	if fc == nil {
		t.Fatal("IT-WE-1553-001: *mcpclient.ResilientClient must be kept (not discarded) when the Fleet " +
			"MCP Gateway is unreachable so the readiness gate's periodic probe can keep retrying it (#1553)")
	}
	t.Cleanup(func() { _ = fc.Close() })
	if fc.Ready() {
		t.Error("IT-WE-1553-001: the kept client must not report Ready() when its initial connection failed")
	}
}

// --- wireFleetReadinessGate wiring (#1553) ---

func TestWireFleetReadinessGate_WE_Disabled_NoGate(t *testing.T) {
	gate := wireFleetReadinessGate(context.Background(), nil, logr.Discard())
	if gate != nil {
		t.Fatal("IT-FLEET-READY-WE-001a: readiness.Gate must remain nil when fleetResilientClient is nil")
	}
}

func TestWireFleetReadinessGate_WE_EnabledReachable_ReadyImmediately(t *testing.T) {
	gw := mockgw.NewMockGateway()
	t.Cleanup(gw.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resilienceCfg := mcpclient.DefaultResilienceConfig()
	resilienceCfg.MaxElapsedTime = 5 * time.Second
	fleetClient, err := mcpclient.NewResilient(ctx, gw.URL(), resilienceCfg, logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error connecting to mock MCP Gateway: %v", err)
	}
	t.Cleanup(func() { _ = fleetClient.Close() })

	gate := wireFleetReadinessGate(ctx, fleetClient, logr.Discard())
	if gate == nil {
		t.Fatal("IT-FLEET-READY-WE-001b: readiness.Gate must be wired when a resilient client is present")
	}
	t.Cleanup(gate.Stop)

	if err := gate.Check(httptest.NewRequest("GET", "/readyz", nil)); err != nil {
		t.Fatalf("IT-FLEET-READY-WE-001b: gate must report ready immediately when the MCP Gateway is reachable, got: %v", err)
	}
}

func TestWireFleetReadinessGate_WE_EnabledUnreachable_NotReady(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resilienceCfg := mcpclient.DefaultResilienceConfig()
	resilienceCfg.InitialInterval = 50 * time.Millisecond
	resilienceCfg.MaxElapsedTime = 500 * time.Millisecond
	fleetClient, connErr := mcpclient.NewResilient(ctx, "http://127.0.0.1:1/unreachable", resilienceCfg, logr.Discard())
	_ = connErr
	if fleetClient != nil {
		t.Cleanup(func() { _ = fleetClient.Close() })
	}

	gate := wireFleetReadinessGate(ctx, fleetClient, logr.Discard())
	if gate == nil {
		t.Fatal("IT-FLEET-READY-WE-001c: readiness.Gate must still be wired (and report NotReady) when the MCP Gateway is unreachable")
	}
	t.Cleanup(gate.Stop)

	if err := gate.Check(httptest.NewRequest("GET", "/readyz", nil)); err == nil {
		t.Fatal("BR-FLEET-054 / #1553: gate must report NotReady when the configured MCP Gateway is " +
			"unreachable, so Kubernetes removes the pod from Service endpoints (pod-wide fail closed)")
	}
}
