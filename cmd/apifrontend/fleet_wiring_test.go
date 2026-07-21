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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

// goconst dedup: test-fixture literals deduplicated below.
const (
	mockModel = "mock-model"
	testKey   = "test-key"
)

// backendGVRListKinds mirrors the GVR the EAIGWRegistry watches (Envoy AI
// Gateway Backend CRDs), needed for the fake dynamic client's list-kind map.
var backendGVRListKinds = map[schema.GroupVersionResource]string{
	registry.BackendGVR: "BackendList",
}

// ---------------------------------------------------------------------------
// IT-AF-054-005: cmd/apifrontend must wire FleetReaderFactory/ClusterRegistry
// from Config.Fleet — this is the actual production entry point consumed by
// AgentConfig (pkg/apifrontend/agent/root.go:134-142). Existing
// IT-AF-054-001..004 (test/integration/apifrontend/fleet) construct
// AgentConfig/ResourceReaderFactory directly, bypassing cmd/ entirely, which
// is why this wiring gap stayed invisible (Pyramid Invariant violation).
// ---------------------------------------------------------------------------

func TestBuildFleetReaderDeps_Disabled_NoOp(t *testing.T) {
	t.Parallel()

	deps := &backendDeps{}
	cfg := &config.Config{} // Fleet.Enabled defaults to false

	err := buildFleetReaderDeps(context.Background(), cfg, deps, logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deps.FleetReaderFactory != nil {
		t.Error("IT-AF-054-005: FleetReaderFactory must remain nil when fleet is disabled")
	}
	if deps.FleetClusterRegistry != nil {
		t.Error("IT-AF-054-005: FleetClusterRegistry must remain nil when fleet is disabled")
	}
}

// TestBuildFleetReaderDeps_Enabled_WiresReaderFactoryAndClusterRegistry is
// IT-AF-054-005: proves cmd/apifrontend actually constructs
// FleetReaderFactory and FleetClusterRegistry from Config.Fleet when
// federation is enabled and the MCP Gateway is reachable — the real
// production dispatch path that buildA2AHandler threads into
// agentpkg.AgentConfig. Uses the mock-mcp-gateway test double (also used by
// pkg/fleet/mcpclient's own discovery tests) so mcpclient.NewResilient's
// initial connection succeeds synchronously, isolating the wiring assertion
// from network reachability concerns.
func TestBuildFleetReaderDeps_Enabled_WiresReaderFactoryAndClusterRegistry(t *testing.T) {
	t.Parallel()

	gw := mockgw.NewMockGateway()
	t.Cleanup(gw.Close)

	dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), backendGVRListKinds)

	deps := &backendDeps{k8sDynClient: dynClient}
	cfg := &config.Config{}
	cfg.Fleet.Enabled = true
	cfg.Fleet.MCPGatewayEndpoint = gw.URL()
	cfg.Fleet.MCPGatewayType = registry.GatewayEAIGW

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := buildFleetReaderDeps(ctx, cfg, deps, logr.Discard())
	if err != nil {
		t.Fatalf("IT-AF-054-005: unexpected error wiring fleet reader deps: %v", err)
	}
	if fc := deps.FleetResilientClient(); fc != nil {
		defer func() { _ = fc.Close() }()
	}
	if deps.fleetReadinessGate != nil {
		defer deps.fleetReadinessGate.Stop()
	}

	if deps.FleetReaderFactory == nil {
		t.Error("IT-AF-054-005: FleetReaderFactory must be wired from Config.Fleet when fleet is enabled — " +
			"this is the field AgentConfig needs for kubectl_get/kubectl_list cross-cluster routing (BR-FLEET-054)")
	}
	if deps.FleetClusterRegistry == nil {
		t.Error("IT-AF-054-005: FleetClusterRegistry must be wired from Config.Fleet when fleet is enabled — " +
			"this is the field AgentConfig needs to register the list_clusters tool (BR-FLEET-054)")
	}
	if !deps.FleetReady() {
		t.Error("IT-FLEET-READY-AF-001b: FleetReady() must report true immediately when the MCP Gateway is reachable")
	}
}

// TestBuildFleetReaderDeps_EnabledUnreachableEndpoint_DegradesGracefully pins
// the fail-open contract for an unreachable Fleet MCP Gateway endpoint
// (mirrors GW's TestRegisterAdapters_FleetEnabledUnreachable): a connectivity
// failure must never error out of buildFleetReaderDeps or block AF startup
// indefinitely — the tool-routing surface (FleetReaderFactory) degrades to
// single-cluster mode instead.
//
// #1553 [readiness gate Wave 5]: unlike the pre-#1553 contract, the
// resilient client is now kept (not discarded) on an initial connection
// failure, and a readiness.Gate is still constructed around it, so
// deps.FleetReady() correctly reports NotReady (and the periodic probe
// keeps retrying) instead of the client being silently lost with no path
// back to healthy short of a restart. Mirrors the identical change made to
// GW/RO/EM/SP/WE in Waves 2-4.
func TestBuildFleetReaderDeps_EnabledUnreachableEndpoint_DegradesGracefully(t *testing.T) {
	t.Parallel()

	dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), backendGVRListKinds)

	deps := &backendDeps{k8sDynClient: dynClient}
	cfg := &config.Config{}
	cfg.Fleet.Enabled = true
	cfg.Fleet.MCPGatewayEndpoint = "http://127.0.0.1:1/unreachable"
	cfg.Fleet.MCPGatewayType = registry.GatewayEAIGW

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := buildFleetReaderDeps(ctx, cfg, deps, logr.Discard())
	if fc := deps.FleetResilientClient(); fc != nil {
		t.Cleanup(func() { _ = fc.Close() })
	}
	if err != nil {
		t.Fatalf("unexpected error for an unreachable Fleet MCP Gateway endpoint: %v", err)
	}
	if deps.FleetReaderFactory != nil {
		t.Error("IT-AF-054-005: FleetReaderFactory must remain nil when the Fleet MCP Gateway is unreachable")
	}
	if deps.FleetResilientClient() == nil {
		t.Fatal("IT-AF-1553-001: *mcpclient.ResilientClient must be kept (not discarded) when the Fleet " +
			"MCP Gateway is unreachable so the readiness gate's periodic probe can keep retrying it (#1553)")
	}
	if deps.FleetReady() {
		t.Error("IT-AF-1553-001: FleetReady() must report false when the initial connection failed")
	}
	t.Cleanup(deps.fleetReadinessGate.Stop)
}

// stubFleetClusterRegistry is a minimal registry.ClusterRegistry for testing
// AgentConfig threading without a live MCP Gateway or CRD watcher.
type stubFleetClusterRegistry struct{}

func (stubFleetClusterRegistry) List() []registry.ClusterInfo { return nil }
func (stubFleetClusterRegistry) Get(_ string) (registry.ClusterInfo, bool) {
	return registry.ClusterInfo{}, false
}
func (stubFleetClusterRegistry) WatchClusters() <-chan registry.ClusterEvent { return nil }
func (stubFleetClusterRegistry) Ready() bool                                 { return true }
func (stubFleetClusterRegistry) Start(_ context.Context) error               { return nil }
func (stubFleetClusterRegistry) Stop()                                       {}

var _ registry.ClusterRegistry = stubFleetClusterRegistry{}

// TestBuildA2AHandler_ThreadsFleetReaderFactory proves buildA2AHandler passes
// backendDeps.FleetReaderFactory/FleetClusterRegistry through to
// agentpkg.AgentConfig (mirrors the existing TestBuildA2AHandler_Threads*
// convention for other backend fields). Without this wiring, list_clusters
// would never be registered and kubectl_get/list would silently ignore
// cluster_id, exactly the production bug IT-AF-054-005 catches.
func TestBuildA2AHandler_ThreadsFleetReaderFactory(t *testing.T) {
	t.Parallel()

	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	}))
	t.Cleanup(mockLLM.Close)

	d := testHandlerDeps(func(d *handlerDeps) {
		d.Cfg.Agent.LLM.Provider = types.LLMProviderGemini
		d.Cfg.Agent.LLM.Endpoint = mockLLM.URL
		d.Cfg.Agent.LLM.Model = mockModel
		d.Cfg.Agent.LLM.APIKey = testKey
		d.Backends.FleetReaderFactory = tools.ResourceReaderFactory(
			func(_ context.Context, _ string) (tools.ResourceReader, error) {
				return &tools.DynamicResourceReader{}, nil
			})
		d.Backends.FleetClusterRegistry = stubFleetClusterRegistry{}
	})

	h, err := buildA2AHandler(context.Background(), d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("IT-AF-054-005: handler must not be nil — FleetReaderFactory/ClusterRegistry threading must not break construction")
	}
}
