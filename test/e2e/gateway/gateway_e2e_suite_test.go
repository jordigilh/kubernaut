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
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	gatewayURL       string // Gateway URL for E2E tests (NodePort or port-forward)
	gatewayNamespace string // Namespace where Gateway is deployed

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
		// Per DD-E2E-PARALLEL: Use Background context directly (no cancel needed)
		// Other services (SignalProcessing, RO) use this pattern successfully
		tempCtx := context.Background()

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
		tempURL := "http://127.0.0.1:8080" // Kind extraPortMapping hostPort (maps to NodePort 30080) - Use 127.0.0.1 for CI/CD IPv4 compatibility
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

		// Mark setup as successful (for setup failure detection in AfterSuite)
		setupSucceeded = true

		// Return kubeconfig path to all processes
		return []byte(tempKubeconfigPath)
	},
	// This runs on ALL processes - connect to the cluster created by process 1
	func(specCtx SpecContext, data []byte) {
		kubeconfigPath = string(data)

		// DD-TEST-007: Set coverage mode for all processes
		coverageMode = os.Getenv("COVERAGE_MODE") == "true"

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

		logger.Info(fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))
		logger.Info(fmt.Sprintf("Gateway E2E Test Suite - Setup (Process %d)", GinkgoParallelProcess()))
		logger.Info(fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))
		logger.Info(fmt.Sprintf("Connecting to cluster created by process 1"))
		logger.Info(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))

		// Set KUBECONFIG environment variable for this process
		err := os.Setenv("KUBECONFIG", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		// DD-E2E-K8S-CLIENT-001: Create suite-level K8s client (same pattern as RO/AIAnalysis)
		// This prevents rate limiter contention by reusing 1 client per process instead of
		// creating ~100 clients (1 per test). See docs/handoff/E2E_RATE_LIMITER_ROOT_CAUSE_JAN13_2026.md
		logger.Info("Creating Kubernetes client for this process (DD-E2E-K8S-CLIENT-001)")
		cfg, err := config.GetConfig()
		Expect(err).ToNot(HaveOccurred(), "Failed to get kubeconfig")

		// Register RemediationRequest CRD scheme
		scheme := k8sruntime.NewScheme()
		err = remediationv1alpha1.AddToScheme(scheme)
		Expect(err).ToNot(HaveOccurred(), "Failed to add RemediationRequest CRD to scheme")
		err = corev1.AddToScheme(scheme)
		Expect(err).ToNot(HaveOccurred(), "Failed to add core/v1 to scheme")

		// Create K8s client once for this process (reused across all tests)
		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).ToNot(HaveOccurred(), "Failed to create Kubernetes client")
		logger.Info("✅ Kubernetes client created for process",
			"process", GinkgoParallelProcess(),
			"pattern", "suite-level (1 per process)")

		// Set cluster configuration (shared across all processes)
		clusterName = "gateway-e2e"
		gatewayURL = "http://127.0.0.1:8080" // Kind extraPortMapping hostPort (maps to NodePort 30080) - Use 127.0.0.1 for CI/CD IPv4 compatibility
		gatewayNamespace = "kubernaut-system"

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Setup Complete - Process ready to run tests")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info(fmt.Sprintf("  • Cluster: %s", clusterName))
		logger.Info(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))
		logger.Info(fmt.Sprintf("  • Gateway URL: %s", gatewayURL))
		logger.Info(fmt.Sprintf("  • Gateway Namespace: %s", gatewayNamespace))
		logger.Info(fmt.Sprintf("  • K8s Client: Suite-level (1 per process)"))
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

		// Detect setup failure: if setupSucceeded is false, BeforeSuite failed
		setupFailed := !setupSucceeded
		if setupFailed {
			logger.Info("⚠️  Setup failure detected (setupSucceeded = false)")
		}

		// Determine cleanup strategy
		anyFailure := setupFailed || anyTestFailed
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
		err := infrastructure.DeleteGatewayCluster(clusterName, kubeconfigPath, anyFailure, GinkgoWriter)
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

// ============================================================================
// Test Namespace Helpers (Pattern: RemediationOrchestrator E2E)
// ============================================================================

// createTestNamespace creates a unique namespace for test isolation
// This prevents "namespace not found" errors that degrade the circuit breaker
// Pattern: Similar to RO E2E (test/e2e/remediationorchestrator/suite_test.go)
func createTestNamespace(prefix string) string {
	// Generate unique name with UUID component (Pattern: RO E2E)
	// UUID guarantees uniqueness even for parallel tests in same second
	name := fmt.Sprintf("%s-%d-%s",
		prefix,
		GinkgoParallelProcess(),
		uuid.New().String()[:8])

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"kubernaut.io/test": "e2e-gateway",
			},
		},
	}
	// Use fresh context for namespace operations to avoid cancellation issues
	// Per DD-E2E-PARALLEL: Namespace operations should not be affected by test-level timeouts
	nsCtx := context.Background()
	err := k8sClient.Create(nsCtx, ns)
	Expect(err).ToNot(HaveOccurred())
	return name
}

// deleteTestNamespace cleans up test namespace after test completion
func deleteTestNamespace(name string) {
	// Guard against empty namespace names (can happen if BeforeEach fails/cancels)
	if name == "" {
		return
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	err := k8sClient.Delete(ctx, ns)
	if err != nil && !apierrors.IsNotFound(err) {
		GinkgoWriter.Printf("⚠️  Failed to delete namespace %s: %v\n", name, err)
	}
}
