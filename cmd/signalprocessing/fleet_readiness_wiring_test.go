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
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	crfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/config"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
	spmetrics "github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

// IT-FLEET-READY-SP-001: cmd/signalprocessing must wire a readiness.Gate
// from the resilient MCP client produced by wireFleetMCPClient and/or the
// cluster registry produced by wireClusterRegistry into
// setupSignalProcessingReconciler's mgr.AddReadyzCheck("fleet", ...) — the
// actual production entry point. Without it, an unreachable Fleet MCP
// Gateway or cluster registry backend only logs an error
// (BR-INTEGRATION-054/BR-FLEET-003's previous fail-open behavior) instead
// of flipping the whole pod NotReady (ADR-068, #1553). Unlike GW/RO/EM, SP
// has no single "Fleet enabled" flag: the MCP client and cluster registry
// are independent, optionally-configured dependencies (confirmed via
// preflight: cfg.Fleet.Endpoint vs cfg.Fleet.MCPGatewayType).
//
// Test Plan: Wave 4 of the fail-closed Fleet readiness gate rollout
// (#1553).

var backendGVRListKinds = map[schema.GroupVersionResource]string{
	registry.BackendGVR: "BackendList",
}

// newTestEnricher builds a K8sEnricher with an isolated (non-global)
// metrics registry so repeated calls across test cases in this file don't
// panic on duplicate Prometheus collector registration.
func newTestEnricher() *enricher.K8sEnricher {
	fakeClient := crfake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	m := spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry())
	return enricher.NewK8sEnricher(fakeClient, fakeClient, logr.Discard(), m, time.Second, time.Minute)
}

// --- wireFleetMCPClient retention behavior (#1553) ---

func TestWireFleetMCPClient_Disabled_NoOp(t *testing.T) {
	cfg := config.DefaultConfig()
	localClient := crfake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()

	fc := wireFleetMCPClient(context.Background(), cfg, localClient, newTestEnricher())
	if fc != nil {
		t.Error("IT-SP-1553-001: *mcpclient.ResilientClient must remain nil when fleet.endpoint is unset")
	}
}

func TestWireFleetMCPClient_EnabledUnreachable_RetainsClient(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Fleet.Endpoint = "http://127.0.0.1:1/unreachable"
	localClient := crfake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	fc := wireFleetMCPClient(ctx, cfg, localClient, newTestEnricher())
	if fc == nil {
		t.Fatal("IT-SP-1553-001: *mcpclient.ResilientClient must be kept (not discarded) when the Fleet " +
			"MCP Gateway is unreachable so the readiness gate's periodic probe can keep retrying it (#1553)")
	}
	t.Cleanup(func() { _ = fc.Close() })
	if fc.Ready() {
		t.Error("IT-SP-1553-001: the kept client must not report Ready() when its initial connection failed")
	}
}

// --- wireClusterRegistry retention behavior (#1553) ---

func TestWireClusterRegistry_Disabled_NoOp(t *testing.T) {
	cfg := config.DefaultConfig()
	dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), backendGVRListKinds)

	cr := wireClusterRegistry(context.Background(), cfg, dynClient, newTestEnricher())
	if cr != nil {
		t.Error("IT-SP-1553-002: fleetregistry.ClusterRegistry must remain nil when fleet.mcpGatewayType is unset")
	}
}

func TestWireClusterRegistry_NilDynClient_NoOp(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Fleet.MCPGatewayType = registry.GatewayEAIGW

	cr := wireClusterRegistry(context.Background(), cfg, nil, newTestEnricher())
	if cr != nil {
		t.Error("IT-SP-1553-002: fleetregistry.ClusterRegistry must remain nil when no dynamic client is available")
	}
}

func TestWireClusterRegistry_StartFails_RetainsRegistry(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Fleet.MCPGatewayType = registry.GatewayEAIGW
	dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), backendGVRListKinds)

	// A pre-canceled context makes cache.WaitForCacheSync fail immediately
	// inside Start(), deterministically exercising the Start-failure path
	// without needing a real unreachable API server.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cr := wireClusterRegistry(ctx, cfg, dynClient, newTestEnricher())
	if cr == nil {
		t.Fatal("IT-SP-1553-002: fleetregistry.ClusterRegistry must be kept (not discarded) when Start() " +
			"fails, so the readiness gate can still gate readiness on it (#1553)")
	}
	if cr.Ready() {
		t.Error("IT-SP-1553-002: the kept registry must not report Ready() when Start() failed")
	}
}

// --- wireFleetReadinessGate wiring (#1553) ---

// fakeSPClusterRegistry is a minimal fleetregistry.ClusterRegistry test
// double for proving wireFleetReadinessGate wires a ClusterRegistryProber
// that reflects the registry's Ready() state, without needing a real
// informer sync.
type fakeSPClusterRegistry struct {
	ready bool
}

func (f *fakeSPClusterRegistry) List() []registry.ClusterInfo { return nil }
func (f *fakeSPClusterRegistry) Get(string) (registry.ClusterInfo, bool) {
	return registry.ClusterInfo{}, false
}
func (f *fakeSPClusterRegistry) WatchClusters() <-chan registry.ClusterEvent {
	ch := make(chan registry.ClusterEvent)
	close(ch)
	return ch
}
func (f *fakeSPClusterRegistry) Ready() bool                 { return f.ready }
func (f *fakeSPClusterRegistry) Start(context.Context) error { return nil }
func (f *fakeSPClusterRegistry) Stop()                       {}

var _ registry.ClusterRegistry = (*fakeSPClusterRegistry)(nil)

func TestWireFleetReadinessGate_SP_NoDeps_NoGate(t *testing.T) {
	gate := wireFleetReadinessGate(context.Background(), nil, nil, logr.Discard())
	if gate != nil {
		t.Fatal("IT-FLEET-READY-SP-001a: readiness.Gate must remain nil when neither Fleet dependency is configured")
	}
}

func TestWireFleetReadinessGate_SP_MCPClientReachable_ReadyImmediately(t *testing.T) {
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
		t.Fatal("IT-FLEET-READY-SP-001b: readiness.Gate must be wired when the resilient MCP client is present")
	}
	t.Cleanup(gate.Stop)

	if err := gate.Check(httptest.NewRequest("GET", "/readyz", nil)); err != nil {
		t.Fatalf("IT-FLEET-READY-SP-001b: gate must report ready immediately when the MCP Gateway is reachable, got: %v", err)
	}
}

func TestWireFleetReadinessGate_SP_MCPClientUnreachable_NotReady(t *testing.T) {
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
		t.Fatal("IT-FLEET-READY-SP-001c: readiness.Gate must still be wired (and report NotReady) when the MCP Gateway is unreachable")
	}
	t.Cleanup(gate.Stop)

	if err := gate.Check(httptest.NewRequest("GET", "/readyz", nil)); err == nil {
		t.Fatal("BR-INTEGRATION-054 / #1553: gate must report NotReady when the configured MCP Gateway " +
			"is unreachable, so Kubernetes removes the pod from Service endpoints (pod-wide fail closed)")
	}
}

func TestWireFleetReadinessGate_SP_ClusterRegistryOnly_ReflectsReadyState(t *testing.T) {
	notReadyRegistry := &fakeSPClusterRegistry{ready: false}

	gate := wireFleetReadinessGate(context.Background(), nil, notReadyRegistry, logr.Discard())
	if gate == nil {
		t.Fatal("IT-FLEET-READY-SP-001d: readiness.Gate must be wired when a cluster registry is present " +
			"(even without an MCP client, cfg.Fleet.MCPGatewayType and cfg.Fleet.Endpoint are independent)")
	}
	t.Cleanup(gate.Stop)

	if err := gate.Check(httptest.NewRequest("GET", "/readyz", nil)); err == nil {
		t.Fatal("IT-FLEET-READY-SP-001d: gate must report NotReady when the cluster registry hasn't synced")
	}
}
