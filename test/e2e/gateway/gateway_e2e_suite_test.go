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

	// DD-TEST-007: E2E Coverage Mode
	coverageMode bool
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

		// DD-TEST-007: Check for coverage mode
		tempCoverageMode := os.Getenv("COVERAGE_MODE") == "true"

		tempLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		tempLogger.Info("Gateway E2E Test Suite - Setup (Process 1)")
		if tempCoverageMode {
			tempLogger.Info("📊 E2E COVERAGE MODE ENABLED (DD-TEST-007)")
		}
		tempLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		tempLogger.Info("Setting up KIND cluster with Gateway dependencies:")
		tempLogger.Info("  • PostgreSQL + Redis (Data Storage dependencies)")
		tempLogger.Info("  • Data Storage (audit trails)")
		tempLogger.Info("  • Gateway service (signal ingestion)")
		if tempCoverageMode {
			tempLogger.Info("  • Coverage instrumentation (GOFLAGS=-cover)")
		}
		tempLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Set cluster configuration
		tempClusterName := "gateway-e2e"
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())
		tempKubeconfigPath := fmt.Sprintf("%s/.kube/gateway-e2e-config", homeDir)

		// Initialize context for infrastructure setup
		tempCtx, _ := context.WithCancel(context.Background())

		// Create KIND cluster with appropriate infrastructure setup
		// Using HYBRID PARALLEL setup (Dec 25, 2025)
		// Build images parallel → Create cluster → Load → Deploy
		tempLogger.Info("Creating Kind cluster with hybrid parallel infrastructure setup...")
		if tempCoverageMode {
			// Use coverage-enabled HYBRID setup (DD-TEST-007)
			// Hybrid approach: Build images in parallel → Create cluster when ready → Load → Deploy
			// Benefits: Fast builds (parallel) + No cluster timeout (created after builds) + Reliable
			err = infrastructure.SetupGatewayInfrastructureHybridWithCoverage(tempCtx, tempClusterName, tempKubeconfigPath, GinkgoWriter)
		} else {
			// Use standard parallel setup
			err = infrastructure.SetupGatewayInfrastructureParallel(tempCtx, tempClusterName, tempKubeconfigPath, GinkgoWriter)
		}
		Expect(err).ToNot(HaveOccurred())

		// Wait for Gateway HTTP endpoint to be ready
		tempLogger.Info("Waiting for Gateway HTTP endpoint to be ready...")
		tempURL := "http://localhost:8080" // Kind extraPortMapping hostPort (maps to NodePort 30080)
		httpClient := &http.Client{Timeout: 5 * time.Second}

		// Use Eventually() instead of manual loop (per TESTING_GUIDELINES.md)
		Eventually(func() int {
			resp, err := httpClient.Get(tempURL + "/health")
			if err != nil {
				return 0
			}
			defer func() { _ = resp.Body.Close() }()
			return resp.StatusCode
		}, 60*time.Second, 2*time.Second).Should(Equal(http.StatusOK),
			"Gateway HTTP endpoint should be ready within 60 seconds")

		tempLogger.Info("✅ Gateway HTTP endpoint ready")

		tempLogger.Info("✅ Cluster created successfully")
		tempLogger.Info(fmt.Sprintf("  • Kubeconfig: %s", tempKubeconfigPath))
		tempLogger.Info("  • Process 1 will now share kubeconfig with other processes")

		// Return kubeconfig path to all processes
		return []byte(tempKubeconfigPath)
	},
	// This runs on ALL processes - connect to the cluster created by process 1
	func(data []byte) {
		kubeconfigPath = string(data)

		// DD-TEST-007: Set coverage mode for all processes
		coverageMode = os.Getenv("COVERAGE_MODE") == "true"

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
		gatewayURL = "http://localhost:8080" // Kind extraPortMapping hostPort (maps to NodePort 30080)
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
		if coverageMode {
			logger.Info("📊 Coverage extraction and report generation...")
		}
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// DD-TEST-007: Extract coverage data if in coverage mode
		if coverageMode {
			logger.Info("📊 Extracting E2E coverage data (DD-TEST-007)...")

			// Step 1: Scale down Gateway to trigger graceful shutdown and coverage flush
			logger.Info("  Step 1: Scaling down Gateway for coverage flush...")
			if err := infrastructure.ScaleDownGatewayForCoverage(kubeconfigPath, GinkgoWriter); err != nil {
				logger.Error(err, "Failed to scale down Gateway for coverage")
			}

			// Step 2: Extract coverage from Kind node
			logger.Info("  Step 2: Extracting coverage from Kind node...")
			coverDir := "coverdata"
			if err := infrastructure.ExtractCoverageFromKind(clusterName, coverDir, GinkgoWriter); err != nil {
				logger.Error(err, "Failed to extract coverage from Kind")
			}

			// Step 3: Generate coverage report
			logger.Info("  Step 3: Generating coverage report...")
			if err := infrastructure.GenerateCoverageReport(coverDir, GinkgoWriter); err != nil {
				logger.Error(err, "Failed to generate coverage report")
			}

			logger.Info("✅ E2E coverage extraction complete")
		}

		// Determine cleanup strategy
		preserveCluster := os.Getenv("SKIP_CLEANUP") == "true" || os.Getenv("KEEP_CLUSTER") != ""

		if preserveCluster {
			logger.Info("⚠️  CLUSTER PRESERVED FOR DEBUGGING")
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

		// Delete cluster (with must-gather log export on failure)
		logger.Info("🗑️  Cleaning up cluster...")
		err := infrastructure.DeleteGatewayCluster(clusterName, kubeconfigPath, anyTestFailed, GinkgoWriter)
		if err != nil {
			logger.Error(err, "Failed to delete cluster")
		} else {
			logger.Info("✅ Cluster deleted successfully")
		}

		// DD-TEST-001 v1.1: Clean up service images built for Kind
		logger.Info("🧹 Cleaning up service images built for Kind (DD-TEST-001 v1.1)...")
		imageTag := os.Getenv("IMAGE_TAG") // Set by build/test infrastructure
		if imageTag != "" {
			imageName := fmt.Sprintf("gateway:%s", imageTag)
			pruneCmd := exec.Command("podman", "rmi", imageName)
			pruneOutput, pruneErr := pruneCmd.CombinedOutput()
			if pruneErr != nil {
				logger.Info("⚠️  Failed to remove service image", "error", pruneErr, "output", string(pruneOutput))
			} else {
				logger.Info("   ✅ Service image removed", "image", imageName)
			}
		} else {
			logger.Info("   ℹ️  IMAGE_TAG not set, skipping service image cleanup")
		}

		// DD-TEST-001 v1.1: Prune dangling images from Kind builds
		logger.Info("🧹 Pruning dangling images from Kind builds...")
		pruneCmd := exec.Command("podman", "image", "prune", "-f")
		_, _ = pruneCmd.CombinedOutput()
		logger.Info("   ✅ Dangling images pruned")

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Cluster Teardown Complete")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
)

// Helper functions for tests
