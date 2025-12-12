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
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubelog "github.com/jordigilh/kubernaut/pkg/log"
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
	logger logr.Logger // DD-005: logr.Logger for unified logging

	// Cluster configuration (shared across all tests)
	clusterName      string
	kubeconfigPath   string
	gatewayURL       string // Gateway URL for E2E tests (NodePort or port-forward)
	gatewayNamespace string // Namespace where Gateway is deployed

	// Track if any test failed (for cluster cleanup decision)
	anyTestFailed bool
)

var _ = SynchronizedBeforeSuite(
	// This runs on process 1 only - create cluster once
	func() []byte {
		// Initialize logger for process 1
		tempLogger := kubelog.NewLogger(kubelog.Options{
			Development: true,
			Level:       0,
			ServiceName: "gateway-e2e-test",
		})

		tempLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		tempLogger.Info("Gateway E2E Test Suite - Setup (Process 1)")
		tempLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		tempLogger.Info("Setting up KIND cluster with Gateway dependencies:")
		tempLogger.Info("  • PostgreSQL + Redis (Data Storage dependencies)")
		tempLogger.Info("  • Data Storage (audit trails)")
		tempLogger.Info("  • Gateway service (signal ingestion)")
		tempLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Set cluster configuration
		tempClusterName := "gateway-e2e"
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())
		tempKubeconfigPath := fmt.Sprintf("%s/.kube/gateway-e2e-config", homeDir)

		// Create KIND cluster with full dependency chain (ONCE for all processes)
		tempLogger.Info("Creating Kind cluster (this runs once)...")
		err = infrastructure.CreateGatewayCluster(tempClusterName, tempKubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		// Initialize context for service deployment
		tempCtx, _ := context.WithCancel(context.Background())

		// Deploy Gateway and Redis in kubernaut-system namespace
		tempLogger.Info("Deploying Gateway services in kubernaut-system namespace...")
		tempNamespace := "kubernaut-system"
		err = infrastructure.DeployTestServices(tempCtx, tempNamespace, tempKubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		// Wait for Gateway HTTP endpoint to be ready
		tempLogger.Info("Waiting for Gateway HTTP endpoint to be ready...")
		tempURL := "http://localhost:8080"
		httpClient := &http.Client{Timeout: 5 * time.Second}
		var gatewayReady bool
		for i := 0; i < 30; i++ {
			resp, err := httpClient.Get(tempURL + "/health")
			if err == nil && resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				gatewayReady = true
				tempLogger.Info("✅ Gateway HTTP endpoint ready", "attempts", i+1)
				break
			}
			if resp != nil {
				resp.Body.Close()
			}
			time.Sleep(2 * time.Second)
		}
		Expect(gatewayReady).To(BeTrue(), "Gateway HTTP endpoint should be ready within 60 seconds")

		tempLogger.Info("✅ Cluster created successfully")
		tempLogger.Info(fmt.Sprintf("  • Kubeconfig: %s", tempKubeconfigPath))
		tempLogger.Info("  • Process 1 will now share kubeconfig with other processes")

		// Return kubeconfig path to all processes
		return []byte(tempKubeconfigPath)
	},
	// This runs on ALL processes - connect to the cluster created by process 1
	func(data []byte) {
		kubeconfigPath = string(data)

		// Initialize context
		ctx, cancel = context.WithCancel(context.Background())

		// Initialize logger for this process
		logger = kubelog.NewLogger(kubelog.Options{
			Development: true,
			Level:       0,
			ServiceName: fmt.Sprintf("gateway-e2e-test-p%d", GinkgoParallelProcess()),
		})

		// Initialize failure tracking
		anyTestFailed = false

		logger.Info(fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))
		logger.Info(fmt.Sprintf("Gateway E2E Test Suite - Setup (Process %d)", GinkgoParallelProcess()))
		logger.Info(fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))
		logger.Info(fmt.Sprintf("Connecting to cluster created by process 1"))
		logger.Info(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))

		// Set KUBECONFIG environment variable for this process
		err := os.Setenv("KUBECONFIG", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		// Set cluster configuration (shared across all processes)
		clusterName = "gateway-e2e"
		gatewayURL = "http://localhost:8080"
		gatewayNamespace = "kubernaut-system"

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Setup Complete - Process ready to run tests")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info(fmt.Sprintf("  • Cluster: %s", clusterName))
		logger.Info(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))
		logger.Info(fmt.Sprintf("  • Gateway URL: %s", gatewayURL))
		logger.Info(fmt.Sprintf("  • Gateway Namespace: %s", gatewayNamespace))
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
)

// Track test failures for cluster cleanup decision
var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		anyTestFailed = true
	}
})

var _ = SynchronizedAfterSuite(
	// This runs on ALL processes - cleanup context
	func() {
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info(fmt.Sprintf("Process %d - Cleaning up", GinkgoParallelProcess()))
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Cancel context for this process
		if cancel != nil {
			cancel()
		}
	},
	// This runs on process 1 only - delete cluster
	func() {
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Gateway E2E Test Suite - Teardown (Process 1)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Check if any test failed - preserve cluster for debugging
		if anyTestFailed || os.Getenv("SKIP_CLEANUP") == "true" || os.Getenv("KEEP_CLUSTER") != "" {
			logger.Info("⚠️  Keeping cluster alive for debugging")
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
			logger.Error(err, "Failed to delete cluster")
		}

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Cluster Teardown Complete")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
)

// Helper functions for tests
