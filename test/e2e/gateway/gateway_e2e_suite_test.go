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
	"os/exec"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Test suite for Gateway E2E tests
// This suite sets up a complete production-like environment:
// - Kind cluster (2 nodes: 1 control-plane + 1 worker)
// - Redis Master-Replica (1 master + 1 replica)
// - Gateway service (deployed to Kind cluster)
//
// NOTE: AlertManager is NOT deployed - tests send payloads directly to Gateway endpoint

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

	// Shared Gateway configuration (deployed ONCE for all tests)
	gatewayNamespace string = "gateway-e2e"
	gatewayURL       string = "http://localhost:8080" // Port-forwarded from gateway-service

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
	logger.Info("Creating Kind cluster and deploying shared Gateway...")
	logger.Info("  • Kind cluster (2 nodes: control-plane + worker)")
	logger.Info("  • RemediationRequest CRD (cluster-wide)")
	logger.Info("  • Gateway Docker image (build + load)")
	logger.Info("  • Shared Gateway + Redis (gateway-e2e namespace)")
	logger.Info("  • Kubeconfig: ~/.kube/gateway-kubeconfig")
	logger.Info("")
	logger.Info("Note: All tests share the same Gateway instance")
	logger.Info("      Each test creates unique namespace for CRDs only")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Set cluster configuration
	clusterName = "gateway-e2e"
	homeDir, err := os.UserHomeDir()
	Expect(err).ToNot(HaveOccurred())
	kubeconfigPath = fmt.Sprintf("%s/.kube/gateway-kubeconfig", homeDir)

	// Delete any existing cluster first to ensure clean state
	logger.Info("Checking for existing cluster...")
	err = infrastructure.DeleteGatewayCluster(clusterName, kubeconfigPath, GinkgoWriter)
	if err != nil {
		logger.Warn("Failed to delete existing cluster (may not exist)", zap.Error(err))
	}

	// Create Kind cluster (ONCE for all tests)
	err = infrastructure.CreateGatewayCluster(clusterName, kubeconfigPath, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred())

	// Set KUBECONFIG environment variable
	err = os.Setenv("KUBECONFIG", kubeconfigPath)
	Expect(err).ToNot(HaveOccurred())

	// Deploy shared Gateway + Redis (ONCE for all tests)
	logger.Info("Deploying shared Gateway + Redis...")
	err = infrastructure.DeployTestServices(ctx, gatewayNamespace, kubeconfigPath, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred())

	// Wait for Gateway pod to be ready before starting port-forward
	logger.Info("⏳ Waiting for Gateway pod to be ready...")
	waitCmd := exec.Command("kubectl", "wait",
		"-n", gatewayNamespace,
		"--for=condition=ready",
		"pod",
		"-l", "app=gateway",
		"--timeout=120s",
		"--kubeconfig", kubeconfigPath)
	waitCmd.Stdout = GinkgoWriter
	waitCmd.Stderr = GinkgoWriter
	err = waitCmd.Run()
	Expect(err).ToNot(HaveOccurred(), "Gateway pod did not become ready")
	logger.Info("✅ Gateway pod is ready")

	// Start kubectl port-forward for Gateway service
	logger.Info("🔌 Starting port-forward to Gateway service...")
	portForwardCmd := exec.CommandContext(ctx, "kubectl", "port-forward",
		"-n", gatewayNamespace,
		"service/gateway-service",
		"8080:8080",
		"--kubeconfig", kubeconfigPath)
	portForwardCmd.Stdout = GinkgoWriter
	portForwardCmd.Stderr = GinkgoWriter
	
	err = portForwardCmd.Start()
	Expect(err).ToNot(HaveOccurred(), "Failed to start port-forward")
	logger.Info("✅ Port-forward started (localhost:8080 -> gateway-service:8080)")

	// Give port-forward a moment to establish connection
	time.Sleep(2 * time.Second)

	// Wait for Gateway HTTP endpoint to be responsive
	logger.Info("⏳ Waiting for shared Gateway HTTP endpoint to be responsive...")
	httpClient := &http.Client{Timeout: 10 * time.Second}
	Eventually(func() error {
		resp, err := httpClient.Get(gatewayURL + "/health")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("health check returned status %d", resp.StatusCode)
		}
		return nil
	}, 60*time.Second, 2*time.Second).Should(Succeed(), "Shared Gateway HTTP endpoint did not become responsive")
	logger.Info("✅ Shared Gateway is ready")

	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Cluster Setup Complete - Shared Gateway Ready")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info(fmt.Sprintf("  • Cluster: %s", clusterName))
	logger.Info(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))
	logger.Info(fmt.Sprintf("  • Gateway Namespace: %s", gatewayNamespace))
	logger.Info(fmt.Sprintf("  • Gateway URL: %s", gatewayURL))
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

// CleanupRedisForTest flushes Redis to ensure test isolation
// This should be called in each test's AfterAll to prevent cross-test interference
func CleanupRedisForTest(namespace string) error {
	// Use kubectl to exec into Redis pod and flush DB
	// This ensures each test starts with clean Redis state
	return infrastructure.FlushRedis(ctx, gatewayNamespace, kubeconfigPath, GinkgoWriter)
}

// GenerateUniqueAlertName creates a unique alert name for test isolation
// Format: <baseName>-<timestamp>-<process>
func GenerateUniqueAlertName(baseName string) string {
	return fmt.Sprintf("%s-%d-p%d", baseName, GinkgoRandomSeed(), GinkgoParallelProcess())
}

// GenerateUniqueNamespace creates a unique namespace name for test isolation
// Format: <prefix>-<timestamp>
func GenerateUniqueNamespace(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, GinkgoRandomSeed())
}
