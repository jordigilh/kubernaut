/*
Copyright 2025 Jordi Gil.

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
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Test suite for Gateway E2E tests
// This suite sets up a complete production-like environment:
// - Kind cluster (4 nodes: 1 control-plane + 3 workers)
// - Redis Sentinel HA (1 master + 2 replicas + 3 Sentinels)
// - Prometheus AlertManager (for webhook testing)
// - Gateway service (deployed to Kind cluster)

func TestGatewayE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway E2E Suite")
}

var (
	ctx    context.Context
	cancel context.CancelFunc
	logger *zap.Logger

	// Cluster configuration (shared across all tests)
	clusterName    string
	kubeconfigPath string

	// Track if any test failed (for cluster cleanup decision)
	anyTestFailed bool
)

var _ = BeforeSuite(func() {
	// Initialize context
	ctx, cancel = context.WithCancel(context.Background())

	// Initialize logger
	var err error
	logger, err = zap.NewDevelopment()
	Expect(err).ToNot(HaveOccurred())

	// Initialize failure tracking
	anyTestFailed = false

	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Gateway E2E Test Suite - Cluster Setup (ONCE)")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Creating Kind cluster for all E2E tests...")
	logger.Info("  • Kind cluster (2 nodes: control-plane + worker)")
	logger.Info("  • RemediationRequest CRD (cluster-wide)")
	logger.Info("  • Gateway Docker image (build + load)")
	logger.Info("  • Kubeconfig: ~/.kube/gateway-kubeconfig")
	logger.Info("")
	logger.Info("Note: Each test will deploy its own services in a unique namespace")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Set cluster configuration
	clusterName = "gateway-e2e"
	homeDir, err := os.UserHomeDir()
	Expect(err).ToNot(HaveOccurred())
	kubeconfigPath = fmt.Sprintf("%s/.kube/gateway-kubeconfig", homeDir)

	// Create Kind cluster (ONCE for all tests)
	err = infrastructure.CreateGatewayCluster(clusterName, kubeconfigPath, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred())

	// Set KUBECONFIG environment variable
	err = os.Setenv("KUBECONFIG", kubeconfigPath)
	Expect(err).ToNot(HaveOccurred())

	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Cluster Setup Complete - Tests can now deploy services per-namespace")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info(fmt.Sprintf("  • Cluster: %s", clusterName))
	logger.Info(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})

// Track test failures for cluster cleanup decision
var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		anyTestFailed = true
	}
})

var _ = AfterSuite(func() {
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Gateway E2E Test Suite - Cluster Teardown")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Check if any test failed - preserve cluster for debugging
	if anyTestFailed || os.Getenv("SKIP_CLEANUP") == "true" {
		logger.Warn("⚠️  Test FAILED - Keeping cluster alive for debugging")
		logger.Info("To debug:")
		logger.Info(fmt.Sprintf("  export KUBECONFIG=%s", kubeconfigPath))
		logger.Info("  kubectl get namespaces | grep -E 'storm|rate|concurrent|crd|restart'")
		logger.Info("  kubectl get pods -n <namespace>")
		logger.Info("  kubectl logs -n <namespace> deployment/gateway")
		logger.Info("To cleanup manually:")
		logger.Info(fmt.Sprintf("  kind delete cluster --name %s", clusterName))
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		return
	}

	// All tests passed - cleanup cluster
	logger.Info("✅ All tests passed - cleaning up cluster...")
	err := infrastructure.DeleteGatewayCluster(clusterName, kubeconfigPath, GinkgoWriter)
	if err != nil {
		logger.Warn("Failed to delete cluster", zap.Error(err))
	}

	// Cancel context
	if cancel != nil {
		cancel()
	}

	// Sync logger
	if logger != nil {
		_ = logger.Sync()
	}

	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Cluster Teardown Complete")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})

// Helper functions for tests
