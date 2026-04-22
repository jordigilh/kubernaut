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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
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
	ctx       context.Context
	cancel    context.CancelFunc
	logger    logr.Logger   // DD-005: logr.Logger for unified logging
	k8sClient client.Client // DD-E2E-K8S-CLIENT-001: Suite-level K8s client (1 per process)

	// Cluster configuration (shared across all tests)
	clusterName      string
	kubeconfigPath   string
	gatewayURL        string // Gateway API URL for E2E tests (NodePort or port-forward)
	gatewayHealthURL  string // Gateway health URL (Issue #753: dedicated :8081 port)
	gatewayMetricsURL string // Gateway metrics URL (Issue #753: dedicated :9090 port)
	gatewayNamespace  string // Namespace where Gateway is deployed

	// Track if any test failed (for cluster cleanup decision)
	anyTestFailed bool

	// Track if BeforeSuite completed successfully (for setup failure detection)
	setupSucceeded bool

	// DD-TEST-007: E2E Coverage Mode
	coverageMode bool
)

var _ = SynchronizedBeforeSuite(NodeTimeout(10*time.Minute),
	// This runs on process 1 only - create cluster once
	func(specCtx SpecContext) []byte {
		// Initialize logger for process 1
		tempLogger := kubelog.NewLogger(kubelog.Options{
			Development: true,
			Level:       0,
			ServiceName: "gateway-e2e-test",
		})

		// DD-TEST-007: Check for coverage mode
		tempCoverageMode := os.Getenv("E2E_COVERAGE") == "true"

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
		// Per DD-E2E-PARALLEL: Use Background context directly (no cancel needed)
		// Other services (SignalProcessing, RO) use this pattern successfully
		tempCtx := context.Background()

		// Create KIND cluster with appropriate infrastructure setup
		// Using HYBRID PARALLEL setup (Dec 25, 2025)
		// Build images parallel → Create cluster → Load → Deploy
		tempLogger.Info("Creating Kind cluster with hybrid parallel infrastructure setup...")

		// Unified setup function with coverage support (consolidated from gateway_e2e_hybrid.go)
		err = infrastructure.SetupGatewayInfrastructureParallel(tempCtx, tempClusterName, tempKubeconfigPath, GinkgoWriter, tempCoverageMode)
		Expect(err).ToNot(HaveOccurred())

		// Issue #753: Wait for Gateway health endpoint (dedicated port 8081)
		tempLogger.Info("Waiting for Gateway health endpoint to be ready...")
		tempHealthURL := "http://127.0.0.1:28080" // Issue #753: dedicated health port (maps to NodePort 30180)
		httpClient := &http.Client{Timeout: 5 * time.Second}

		Eventually(func() int {
			resp, err := httpClient.Get(tempHealthURL + "/readyz")
			if err != nil {
				return 0
			}
			defer func() { _ = resp.Body.Close() }()
			return resp.StatusCode
		}, 60*time.Second, 2*time.Second).Should(Equal(http.StatusOK),
			"Gateway health endpoint should be ready within 60 seconds")

		tempLogger.Info("✅ Gateway health endpoint ready")

		tempLogger.Info("✅ Cluster created successfully")
		tempLogger.Info(fmt.Sprintf("  • Kubeconfig: %s", tempKubeconfigPath))
		tempLogger.Info("  • Process 1 will now share kubeconfig with other processes")

		// Mark setup as successful (for setup failure detection in AfterSuite)
		setupSucceeded = true

		// Return kubeconfig path to all processes
		return []byte(tempKubeconfigPath)
	},
	// This runs on ALL processes - connect to the cluster created by process 1
	func(specCtx SpecContext, data []byte) {
		kubeconfigPath = string(data)

		// DD-TEST-007: Set coverage mode for all processes
		coverageMode = os.Getenv("E2E_COVERAGE") == "true"

		// Initialize context (use simple WithCancel, will be managed by Ginkgo lifecycle)
		// Per DD-E2E-PARALLEL: Context managed through entire suite execution
		ctx, cancel = context.WithCancel(context.TODO())

		// Initialize logger for this process
		logger = kubelog.NewLogger(kubelog.Options{
			Development: true,
			Level:       0,
			ServiceName: fmt.Sprintf("gateway-e2e-test-p%d", GinkgoParallelProcess()),
		})

		// Initialize failure tracking
		anyTestFailed = false

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info(fmt.Sprintf("Gateway E2E Test Suite - Setup (Process %d)", GinkgoParallelProcess()))
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Connecting to cluster created by process 1")
		logger.Info(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))

		// Set KUBECONFIG environment variable for this process
		err := os.Setenv("KUBECONFIG", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		// DD-E2E-K8S-CLIENT-001: Create suite-level K8s client (same pattern as RO/AIAnalysis)
		// This prevents rate limiter contention by reusing 1 client per process instead of
		// creating ~100 clients (1 per test).
		logger.Info("Creating Kubernetes client for this process (DD-E2E-K8S-CLIENT-001)")
		cfg, err := config.GetConfig()
		Expect(err).ToNot(HaveOccurred(), "Failed to get kubeconfig")

		// Register RemediationRequest CRD scheme
		scheme := k8sruntime.NewScheme()
		err = remediationv1alpha1.AddToScheme(scheme)
		Expect(err).ToNot(HaveOccurred(), "Failed to add RemediationRequest CRD to scheme")
		err = corev1.AddToScheme(scheme)
		Expect(err).ToNot(HaveOccurred(), "Failed to add core/v1 to scheme")
		err = appsv1.AddToScheme(scheme)
		Expect(err).ToNot(HaveOccurred(), "Failed to add apps/v1 to scheme")

		// Create K8s client once for this process (reused across all tests)
		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).ToNot(HaveOccurred(), "Failed to create Kubernetes client")
		logger.Info("✅ Kubernetes client created for process",
			"process", GinkgoParallelProcess(),
			"pattern", "suite-level (1 per process)")

		// Set cluster configuration (shared across all processes)
		clusterName = "gateway-e2e"
		gatewayURL = "http://127.0.0.1:8080"         // Kind extraPortMapping hostPort (maps to NodePort 30080)
		gatewayHealthURL = "http://127.0.0.1:28080"  // Issue #753: dedicated health port (maps to NodePort 30180)
		gatewayMetricsURL = "http://127.0.0.1:9090"  // Issue #753: dedicated metrics port (maps to NodePort 30090)
		gatewayNamespace = "kubernaut-system"

		// BR-GATEWAY-036/037: Create suite-level authorized SA for all E2E webhook requests.
		// All E2E tests that POST to /api/v1/signals/* need a valid Bearer token
		// because the production Gateway enforces auth on all signal endpoints.
		logger.Info("Creating suite-level authorized ServiceAccount for E2E webhook auth")
		saCtx := context.Background()
		err = infrastructure.CreateE2EServiceAccountWithGatewayAccess(
			saCtx, gatewayNamespace, kubeconfigPath, "e2e-gateway-suite-sa", GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create E2E gateway auth ServiceAccount")

		e2eAuthToken, err = infrastructure.GetServiceAccountToken(
			saCtx, gatewayNamespace, "e2e-gateway-suite-sa", kubeconfigPath,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to get E2E gateway auth token")
		Expect(e2eAuthToken).ToNot(BeEmpty(), "E2E gateway auth token must not be empty")
		logger.Info("Suite-level E2E auth token created", "sa", "e2e-gateway-suite-sa")

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Setup Complete - Process ready to run tests")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info(fmt.Sprintf("  • Cluster: %s", clusterName))
		logger.Info(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))
		logger.Info(fmt.Sprintf("  • Gateway API URL: %s", gatewayURL))
		logger.Info(fmt.Sprintf("  • Gateway Health URL: %s", gatewayHealthURL))
		logger.Info(fmt.Sprintf("  • Gateway Metrics URL: %s", gatewayMetricsURL))
		logger.Info(fmt.Sprintf("  • Gateway Namespace: %s", gatewayNamespace))
		logger.Info("  • K8s Client: Suite-level (1 per process)")
		logger.Info("  • Auth Token: Suite-level (e2e-gateway-suite-sa)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
)

// Track test failures for cluster cleanup decision
var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		anyTestFailed = true
		infrastructure.MarkTestFailure(clusterName)
	}
})

var _ = SynchronizedAfterSuite(
	// This runs on ALL processes - cleanup context
	func() {
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info(fmt.Sprintf("Process %d - Cleaning up", GinkgoParallelProcess()))
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// NOTE: Do NOT cancel suite-level context here - it's needed for namespace operations
		// throughout the entire test suite execution. Context will be canceled after all
		// tests complete (in the second SynchronizedAfterSuite function).
	},
	// This runs on process 1 only - delete cluster
	func() {
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Gateway E2E Test Suite - Teardown (Process 1)")
		if coverageMode {
			logger.Info("📊 Coverage extraction and report generation...")
		}
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// DD-TEST-007: Collect E2E binary coverage BEFORE cluster deletion
		if coverageMode {
			if err := infrastructure.CollectE2EBinaryCoverage(infrastructure.E2ECoverageOptions{
				ServiceName:    "gateway",
				ClusterName:    clusterName,
				DeploymentName: "gateway",
				Namespace:      "kubernaut-system",
				KubeconfigPath: kubeconfigPath,
			}, GinkgoWriter); err != nil {
				logger.Error(err, "Failed to collect E2E binary coverage (non-fatal)")
			}
		}

		// Detect setup failure: if setupSucceeded is false, BeforeSuite failed
		setupFailed := !setupSucceeded
		if setupFailed {
			logger.Info("⚠️  Setup failure detected (setupSucceeded = false)")
		}

		// Determine cleanup strategy
		anyFailure := setupFailed || anyTestFailed || infrastructure.CheckTestFailure(clusterName)
		defer infrastructure.CleanupFailureMarker(clusterName)
		preserveCluster := os.Getenv("SKIP_CLEANUP") == "true" || os.Getenv("KEEP_CLUSTER") != ""

		if preserveCluster {
			logger.Info("⚠️  CLUSTER PRESERVED FOR DEBUGGING")
			logger.Info("To debug:")
			logger.Info(fmt.Sprintf("  export KUBECONFIG=%s", kubeconfigPath))
			logger.Info("  kubectl get namespaces | grep -E 'rate|concurrent|crd|restart'")
			logger.Info("  kubectl get pods -n <namespace>")
			logger.Info("  kubectl logs -n <namespace> deployment/gateway")
			logger.Info("To cleanup manually:")
			logger.Info(fmt.Sprintf("  kind delete cluster --name %s", clusterName))
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			return
		}

		// Delete cluster (with must-gather log export on failure)
		logger.Info("🗑️  Cleaning up cluster...")
		err := infrastructure.DeleteGatewayCluster(clusterName, kubeconfigPath, anyFailure, GinkgoWriter)
		if err != nil {
			logger.Error(err, "Failed to delete cluster")
		} else {
			logger.Info("✅ Cluster deleted successfully")
		}

		// DD-TEST-001 v1.1: Clean up service images built for Kind
		logger.Info("🧹 Cleaning up service images built for Kind (DD-TEST-001 v1.1)...")
		imageRegistry := os.Getenv("IMAGE_REGISTRY")
		imageTag := os.Getenv("IMAGE_TAG")

		// Skip cleanup when using registry images (CI/CD mode)
		if imageRegistry != "" && imageTag != "" {
			logger.Info("   ℹ️  Registry mode detected - skipping local image removal",
				"registry", imageRegistry, "tag", imageTag)
		} else if imageTag != "" {
			// Local build mode: Remove locally built images
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

// ============================================================================
// Test Namespace Helpers (Pattern: RemediationOrchestrator E2E)
// ============================================================================

// createTestNamespace creates a unique namespace for test isolation
// This prevents "namespace not found" errors that degrade the circuit breaker
// Pattern: Similar to RO E2E (test/e2e/remediationorchestrator/suite_test.go)
