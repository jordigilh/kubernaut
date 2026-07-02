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
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	gatewaypkg "github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

// TestRegisterAdapters_FleetDisabled is a characterization test for
// registerAdapters, extracted from main() in GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 0a. Pins the default (Fleet federation disabled) contract: both
// adapters register successfully and no Fleet resilient client is created.
// cmd/gateway had zero test coverage before this extraction.
func TestRegisterAdapters_FleetDisabled(t *testing.T) {
	srv := newTestGatewayServer(t)
	apiRegistry := adapters.NewTestAPIResourceRegistry()

	fleetClient, err := registerAdapters(context.Background(), srv, apiRegistry, testServerConfig(), logr.Discard())

	if err != nil {
		t.Fatalf("registerAdapters returned unexpected error: %v", err)
	}
	if fleetClient != nil {
		t.Fatalf("expected nil fleet client when Fleet.Enabled=false, got %v", fleetClient)
	}
}

// TestRegisterAdapters_FleetEnabledUnreachable pins the graceful-degradation
// contract for an unreachable Fleet MCP Gateway endpoint: registerAdapters
// never fails adapter registration because of a Fleet connectivity problem
// (fleetclient.NewResilient connects lazily/asynchronously, so construction
// itself does not error even when the endpoint is unreachable — matches
// main()'s original inline behavior before extraction; this test would catch
// a regression where a Fleet failure starts blocking startup or erroring).
func TestRegisterAdapters_FleetEnabledUnreachable(t *testing.T) {
	srv := newTestGatewayServer(t)
	apiRegistry := adapters.NewTestAPIResourceRegistry()

	cfg := testServerConfig()
	cfg.Fleet.Enabled = true
	cfg.Fleet.MCPGatewayEndpoint = "http://127.0.0.1:1/unreachable"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fleetClient, err := registerAdapters(ctx, srv, apiRegistry, cfg, logr.Discard())
	if fleetClient != nil {
		defer func() { _ = fleetClient.Close() }()
	}

	if err != nil {
		t.Fatalf("registerAdapters returned unexpected error for an unreachable Fleet endpoint: %v", err)
	}
}

// newTestGatewayServer builds a minimal *gateway.Server backed by a fake
// controller-runtime client, sufficient for registerAdapters (no live
// cluster or discovery required beyond the fake API resource registry).
func newTestGatewayServer(t *testing.T) *gatewaypkg.Server {
	t.Helper()

	// NewServerForTesting resolves the controller namespace via this env var
	// when no in-cluster serviceaccount file is present (matches the pattern
	// in pkg/gateway/config_reload_audit_test.go).
	t.Setenv("KUBERNAUT_CONTROLLER_NAMESPACE", "kubernaut-system")

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	metricsInstance := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())

	srv, err := gatewaypkg.NewServerForTesting(gatewaypkg.ServerTestDeps{
		Config:          testServerConfig(),
		Logger:          logr.Discard(),
		MetricsInstance: metricsInstance,
		CtrlClient:      k8sClient,
	})
	if err != nil {
		t.Fatalf("NewServerForTesting failed: %v", err)
	}
	return srv
}

func testServerConfig() *config.ServerConfig {
	return &config.ServerConfig{
		Server: config.ServerSettings{
			ListenAddr:   "127.0.0.1:0",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		Processing: config.ProcessingSettings{
			Deduplication: config.DeduplicationSettings{
				CooldownPeriod: 300 * time.Second,
			},
			Retry: config.RetrySettings{
				MaxAttempts:    3,
				InitialBackoff: 100 * time.Millisecond,
				MaxBackoff:     1 * time.Second,
			},
		},
	}
}
