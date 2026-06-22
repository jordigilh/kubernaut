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

package gateway

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// mockFleetScopeChecker implements scope.ScopeChecker.
// It records calls for verification and returns configurable responses.
type mockFleetScopeChecker struct {
	isManagedResult      bool
	isManagedErr         error
	isManagedOnClusterFn func(ctx context.Context, clusterID, namespace, kind, name string) (bool, error)
	localCalls           int
	fleetCalls           int
}

func (m *mockFleetScopeChecker) IsManagedResource(ctx context.Context, resource scope.ResourceIdentity) (bool, error) {
	if resource.ClusterID != "" {
		m.fleetCalls++
		if m.isManagedOnClusterFn != nil {
			return m.isManagedOnClusterFn(ctx, resource.ClusterID, resource.Namespace, resource.Kind, resource.Name)
		}
		return m.isManagedResult, m.isManagedErr
	}
	m.localCalls++
	return m.isManagedResult, m.isManagedErr
}

var _ = Describe("GW Fleet Scope Dispatch (BR-INTEGRATION-065, ADR-065)", Ordered, Label("fleet", "scope", "integration"), func() {
	var (
		testLogger   logr.Logger
		gwServer     *gateway.Server
		scopeChecker *mockFleetScopeChecker
	)

	BeforeAll(func() {
		testLogger = logger.WithValues("test", "fleet-scope-dispatch")

		scopeChecker = &mockFleetScopeChecker{
			isManagedResult: true,
		}

		gwConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
		testRegistry := prometheus.NewRegistry()
		metricsInstance := metrics.NewMetricsWithRegistry(testRegistry)
		var err error
		gwServer, err = gateway.NewServerForTesting(gwConfig, testLogger, metricsInstance, k8sClient, sharedAuditStore, scopeChecker, nil, nil)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Gateway server")
	})

	// IT-GW-FLEET-010: Fleet signal dispatches to remote scope check
	It("IT-GW-FLEET-010: should dispatch to remote scope check when signal has ClusterID", func() {
		signal := createNormalizedSignal(SignalBuilder{
			AlertName:    "RemoteHighCPU",
			Namespace:    "remote-ns",
			ResourceKind: "Deployment",
			ResourceName: "api-server",
			Severity:     "critical",
			ClusterID:    "prod-east",
		})

		scopeChecker.isManagedOnClusterFn = func(_ context.Context, clusterID, namespace, kind, name string) (bool, error) {
			Expect(clusterID).To(Equal("prod-east"))
			Expect(namespace).To(Equal("remote-ns"))
			Expect(kind).To(Equal("Deployment"))
			Expect(name).To(Equal("api-server"))
			return true, nil
		}

		_, err := gwServer.ProcessSignal(ctx, signal)
		Expect(err).ToNot(HaveOccurred())
		Expect(scopeChecker.fleetCalls).To(Equal(1),
			"IT-GW-FLEET-010: fleet scope path must be invoked for signal with ClusterID")
		Expect(scopeChecker.localCalls).To(Equal(0),
			"IT-GW-FLEET-010: local scope path must NOT be invoked for fleet signal")
	})

	// IT-GW-FLEET-011: Local signal dispatches to IsManaged (backward compat)
	It("IT-GW-FLEET-011: should dispatch to IsManaged when signal has empty ClusterID", func() {
		scopeChecker.localCalls = 0
		scopeChecker.fleetCalls = 0

		signal := createNormalizedSignal(SignalBuilder{
			AlertName:    "LocalHighMem",
			Namespace:    "default",
			ResourceKind: "Pod",
			ResourceName: "worker-1",
			Severity:     "warning",
		})

		_, err := gwServer.ProcessSignal(ctx, signal)
		Expect(err).ToNot(HaveOccurred())
		Expect(scopeChecker.localCalls).To(Equal(1),
			"IT-GW-FLEET-011: local scope path must be invoked for signal without ClusterID")
		Expect(scopeChecker.fleetCalls).To(Equal(0),
			"IT-GW-FLEET-011: fleet scope path must NOT be invoked for local signal")
	})

	// IT-GW-FLEET-012: Fleet signal rejected when resource is not managed on remote cluster
	It("IT-GW-FLEET-012: should reject fleet signal when remote scope check returns false", func() {
		scopeChecker.localCalls = 0
		scopeChecker.fleetCalls = 0

		signal := createNormalizedSignal(SignalBuilder{
			AlertName:    "RemoteUnmanagedAlert",
			Namespace:    "other-ns",
			ResourceKind: "StatefulSet",
			ResourceName: "redis",
			Severity:     "warning",
			ClusterID:    "staging-west",
		})

		scopeChecker.isManagedOnClusterFn = func(_ context.Context, _, _, _, _ string) (bool, error) {
			return false, nil
		}

		response, err := gwServer.ProcessSignal(ctx, signal)
		Expect(err).ToNot(HaveOccurred())
		Expect(response).ToNot(BeNil())
		Expect(response.Status).To(Equal(gateway.StatusRejected),
			"IT-GW-FLEET-012: unmanaged fleet signal must be rejected")
		Expect(scopeChecker.fleetCalls).To(Equal(1))
	})
})
