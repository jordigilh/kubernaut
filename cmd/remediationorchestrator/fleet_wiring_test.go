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
	"testing"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	crfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	config "github.com/jordigilh/kubernaut/internal/config/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

// ---------------------------------------------------------------------------
// IT-RO-054-001: cmd/remediationorchestrator must wire a fleet.ReaderFactory
// into Reconciler.SetReaderFactory from Config.Fleet — this is the actual
// production entry point (buildReconciler). Without it,
// readerForHash(ctx, clusterID) silently falls back to the local hub
// cluster reader (internal/controller/remediationorchestrator/
// config_accessors.go:71-76), so CapturePreRemediationHash computes the
// pre-remediation resource fingerprint against the WRONG cluster for any
// fleet-routed RemediationRequest — corrupting the EA hash-comparison used
// for effectiveness assessment. buildReconciler already wires
// fleet.NewScopeChecker (Backend/Endpoint) via buildRoutingEngine, which is
// why this second, independent gap (MCPGatewayEndpoint reader factory)
// stayed invisible.
// ---------------------------------------------------------------------------

func TestBuildFleetReaderFactory_Disabled_NoOp(t *testing.T) {
	t.Parallel()

	localClient := crfake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	cfg := config.DefaultConfig() // Fleet.Enabled defaults to false

	rf, fc, err := buildFleetReaderFactory(context.Background(), localClient, cfg, logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rf != nil {
		t.Error("IT-RO-054-001: fleet.ReaderFactory must remain nil when fleet is disabled")
	}
	if fc != nil {
		t.Error("IT-RO-054-001: *mcpclient.ResilientClient must remain nil when fleet is disabled")
	}
}

// TestBuildFleetReaderFactory_Enabled_WiresReaderFactory is IT-RO-054-001:
// proves cmd/remediationorchestrator actually constructs a
// fleet.ReaderFactory from Config.Fleet when federation is enabled and the
// MCP Gateway is reachable — the real production dispatch path that
// buildReconciler passes to Reconciler.SetReaderFactory.
func TestBuildFleetReaderFactory_Enabled_WiresReaderFactory(t *testing.T) {
	t.Parallel()

	gw := mockgw.NewMockGateway()
	t.Cleanup(gw.Close)

	localClient := crfake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	cfg := config.DefaultConfig()
	cfg.Fleet.Enabled = true
	cfg.Fleet.MCPGatewayEndpoint = gw.URL()
	cfg.Fleet.MCPGatewayType = registry.GatewayEAIGW

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rf, fc, err := buildFleetReaderFactory(ctx, localClient, cfg, logr.Discard())
	if err != nil {
		t.Fatalf("IT-RO-054-001: unexpected error wiring fleet reader factory: %v", err)
	}
	if fc != nil {
		t.Cleanup(func() { _ = fc.Close() })
	}
	if rf == nil {
		t.Fatal("IT-RO-054-001: fleet.ReaderFactory must be wired from Config.Fleet when fleet is enabled — " +
			"without it, CapturePreRemediationHash silently reads the local hub cluster for fleet-routed " +
			"RemediationRequests (BR-FLEET-054)")
	}
	if fc == nil {
		t.Error("IT-RO-054-001: *mcpclient.ResilientClient must be returned so main() can close it on " +
			"graceful shutdown (mirrors GW's registerAdapters contract, cmd/gateway/main.go:164-169)")
	}

	// The wired factory must actually be usable end-to-end: empty clusterID
	// resolves to the local manager client (matches fleet.ReaderFactory's
	// documented contract, pkg/fleet/reader_factory.go:25-27).
	reader, err := rf.ReaderFor(context.Background(), "")
	if err != nil {
		t.Fatalf("ReaderFor(\"\") returned unexpected error: %v", err)
	}
	if reader != localClient {
		t.Error("IT-RO-054-001: ReaderFor(\"\") must return the local manager client for empty clusterID")
	}
}

// TestBuildFleetReaderFactory_EnabledUnreachableEndpoint_DegradesGracefully
// pins the fail-open contract for an unreachable Fleet MCP Gateway endpoint
// (mirrors GW's/EM's equivalent tests): a connectivity failure must never
// error out of buildFleetReaderFactory — RO degrades to hub-only mode
// instead of blocking startup.
func TestBuildFleetReaderFactory_EnabledUnreachableEndpoint_DegradesGracefully(t *testing.T) {
	t.Parallel()

	localClient := crfake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	cfg := config.DefaultConfig()
	cfg.Fleet.Enabled = true
	cfg.Fleet.MCPGatewayEndpoint = "http://127.0.0.1:1/unreachable"
	cfg.Fleet.MCPGatewayType = registry.GatewayEAIGW

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rf, fc, err := buildFleetReaderFactory(ctx, localClient, cfg, logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error for an unreachable Fleet MCP Gateway endpoint: %v", err)
	}
	if rf != nil {
		t.Error("IT-RO-054-001: fleet.ReaderFactory must remain nil when the Fleet MCP Gateway is unreachable")
	}
	if fc != nil {
		t.Error("IT-RO-054-001: *mcpclient.ResilientClient must remain nil when the Fleet MCP Gateway is unreachable")
	}
}
