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
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// IT-GW-FLEET-010/011/012: GW Fleet Scope Dispatch tests
//
// These tests verify that ProcessSignal correctly routes scope checks:
//   - Non-empty ClusterID → remote FMC (managed resource seeded in Valkey → accepted)
//   - Empty ClusterID → local scope.Manager (managed namespace in envtest → accepted)
//   - Non-empty ClusterID with unmanaged resource → remote FMC returns false → rejected
//
// Architecture: Real FMC stack backed by the suite's shared Redis (port 16380).
// No mock scope checkers — business outcomes prove the routing.
var _ = Describe("GW Fleet Scope Dispatch (BR-INTEGRATION-065, ADR-065)", Ordered, Label("fleet", "scope", "integration"), func() {
	var (
		testLogger    logr.Logger
		gwServer      *gateway.Server
		fmcServer     *httptest.Server
		testNamespace string
	)

	BeforeAll(func() {
		testLogger = logger.WithValues("test", "fleet-scope-dispatch")

		testNamespace = helpers.CreateTestNamespace(ctx, k8sClient, "fleet-scope-int")

		valkeyAddr := fmt.Sprintf("127.0.0.1:%d", gatewayRedisPort)

		By("Seeding shared Redis with managed resources for prod-east only")
		// Cache key uses the real GVK ("apps/v1" for Deployment) matching what
		// pkg/fleet/fmc/syncer.go writes from the K8s API, and what
		// scopecache.Client.IsManagedResource now infers via scope.InferGVK when
		// the caller's ResourceIdentity leaves Group/Version empty (Issue #54).
		writer := fmc.NewValkeyWriter(valkeyAddr)
		key, err := scopecache.BuildKey("prod-east", "apps", "v1", "Deployment", "remote-ns", "api-server")
		Expect(err).ToNot(HaveOccurred())
		Expect(writer.Set(ctx, key, 5*time.Minute)).To(Succeed())
		_ = writer.Close()

		By("Creating real FMC HTTP stack")
		cacheReader := scopecache.NewValkeyCacheReader(valkeyAddr)
		scopeClient := scopecache.NewClient(cacheReader)
		clusterReg := newStaticClusterRegistry("prod-east", "staging-west")
		handler := fmc.NewHandler(scopeClient, clusterReg, testLogger)
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)
		fmcServer = httptest.NewServer(mux)

		By("Creating FederatedScopeChecker backed by real FMC + real scope.Manager")
		localChecker := scope.NewManager(k8sClient)
		remoteChecker := fmc.NewHTTPClient(fmcServer.URL)
		federatedChecker := fleet.NewFederatedScopeChecker(localChecker, remoteChecker, testLogger)

		By("Creating Gateway server with real federated scope checker")
		gwConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
		testRegistry := prometheus.NewRegistry()
		metricsInstance := metrics.NewMetricsWithRegistry(testRegistry)
		gwServer, err = gateway.NewServerForTesting(gateway.ServerTestDeps{
			Config: gwConfig, Logger: testLogger, MetricsInstance: metricsInstance,
			CtrlClient: k8sClient, AuditStore: sharedAuditStore, ScopeChecker: federatedChecker,
			Authenticator: suiteAuthenticator, Authorizer: suiteAuthorizer,
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create Gateway server")
	})

	AfterAll(func() {
		if fmcServer != nil {
			fmcServer.Close()
		}
		if testNamespace != "" {
			helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)
		}
	})

	// IT-GW-FLEET-010: Fleet signal dispatches to remote scope check.
	// Resource "prod-east/Deployment/api-server" is seeded in Valkey → FMC returns managed=true.
	// This proves the fleet (remote) path was invoked because the resource only exists in Valkey,
	// not in envtest.
	It("IT-GW-FLEET-010: should dispatch to remote scope check when signal has ClusterID", func() {
		signal := createNormalizedSignal(SignalBuilder{
			AlertName:    "RemoteHighCPU",
			Namespace:    "remote-ns",
			ResourceKind: "Deployment",
			ResourceName: "api-server",
			Severity:     "critical",
			ClusterID:    "prod-east",
		})

		response, err := gwServer.ProcessSignal(ctx, signal)
		Expect(err).ToNot(HaveOccurred())
		Expect(response.Status).To(Equal(gateway.StatusCreated),
			"IT-GW-FLEET-010: managed resource on remote cluster must be accepted (proves fleet path invoked)")
	})

	// IT-GW-FLEET-011: Local signal dispatches to scope.Manager (backward compat).
	// Namespace has kubernaut.ai/managed=true label → scope.Manager returns true.
	// This proves the local path was invoked because the namespace only exists in envtest,
	// not in Valkey.
	It("IT-GW-FLEET-011: should dispatch to IsManaged when signal has empty ClusterID", func() {
		signal := createNormalizedSignal(SignalBuilder{
			AlertName:    "LocalHighMem",
			Namespace:    testNamespace,
			ResourceKind: "Pod",
			ResourceName: "worker-1",
			Severity:     "warning",
		})

		response, err := gwServer.ProcessSignal(ctx, signal)
		Expect(err).ToNot(HaveOccurred())
		Expect(response.Status).To(Equal(gateway.StatusCreated),
			"IT-GW-FLEET-011: managed resource on local cluster must be accepted (proves local path invoked)")
	})

	// IT-GW-FLEET-012: Fleet signal rejected when resource is not managed on remote cluster.
	// staging-west is a known cluster, but StatefulSet/redis in other-ns is NOT in Valkey →
	// FMC returns managed=false → signal rejected.
	It("IT-GW-FLEET-012: should reject fleet signal when remote scope check returns false", func() {
		signal := createNormalizedSignal(SignalBuilder{
			AlertName:    "RemoteUnmanagedAlert",
			Namespace:    "other-ns",
			ResourceKind: "StatefulSet",
			ResourceName: "redis",
			Severity:     "warning",
			ClusterID:    "staging-west",
		})

		response, err := gwServer.ProcessSignal(ctx, signal)
		Expect(err).ToNot(HaveOccurred())
		Expect(response).ToNot(BeNil())
		Expect(response.Status).To(Equal(gateway.StatusRejected),
			"IT-GW-FLEET-012: unmanaged fleet signal must be rejected")
	})
})
