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

// IT-FLEET-READY-AF-001: cmd/apifrontend must wire a readiness.Gate from
// the resilient client produced by buildFleetReaderDeps into
// backendDeps.FleetReady(), consumed by mcp_a2a_handlers.go's depsReady ->
// buildHealthMux's /readyz — the actual production entry point. Without
// it, an unreachable Fleet MCP Gateway only logs an error (BR-FLEET-054's
// previous fail-open behavior) instead of flipping the whole pod NotReady
// (ADR-068, #1553). AF has no scope-checker (confirmed via search: no
// ScopeChecker references anywhere in cmd/apifrontend), so its gate
// carries an MCPClientProber and, when available, a
// ClusterRegistryProber.
//
// Test Plan: Wave 5 of the fail-closed Fleet readiness gate rollout
// (#1553).

func TestWireFleetReadinessGate_AF_Reachable_ReadyImmediately(t *testing.T) {
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

	gate := wireFleetReadinessGate(ctx, fleetClient, nil, logr.Discard())
	if gate == nil {
		t.Fatal("IT-FLEET-READY-AF-001a: readiness.Gate must always be wired when buildFleetReaderDeps calls it")
	}
	t.Cleanup(gate.Stop)

	if err := gate.Check(httptest.NewRequest("GET", "/readyz", nil)); err != nil {
		t.Fatalf("IT-FLEET-READY-AF-001a: gate must report ready immediately when the MCP Gateway is reachable, got: %v", err)
	}
}

func TestWireFleetReadinessGate_AF_Unreachable_NotReady(t *testing.T) {
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

	gate := wireFleetReadinessGate(ctx, fleetClient, nil, logr.Discard())
	if gate == nil {
		t.Fatal("IT-FLEET-READY-AF-001b: readiness.Gate must still be wired (and report NotReady) when the MCP Gateway is unreachable")
	}
	t.Cleanup(gate.Stop)

	if err := gate.Check(httptest.NewRequest("GET", "/readyz", nil)); err == nil {
		t.Fatal("BR-FLEET-054 / #1553: gate must report NotReady when the configured MCP Gateway is " +
			"unreachable, so Kubernetes removes the pod from Service endpoints (pod-wide fail closed)")
	}
}
