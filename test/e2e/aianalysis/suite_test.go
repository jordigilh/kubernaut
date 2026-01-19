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

// Package aianalysis contains E2E tests for the AIAnalysis controller.
// These tests run against a real KIND cluster with deployed services.
//
// Business Requirements:
// - BR-AI-001: Complete reconciliation lifecycle
// - BR-AI-022: Metrics endpoint validation
// - BR-AI-025: Health endpoint validation
//
// Test Strategy (per TESTING_GUIDELINES.md):
// - E2E tests use KIND cluster with real services
// - LLM is mocked in HolmesGPT-API (cost constraint)
// - Data Storage used for audit trails
//
// Port Allocation (per DD-TEST-001):
// - AIAnalysis Health: http://localhost:8184
// - AIAnalysis Metrics: http://localhost:9184
// - Data Storage: http://localhost:8081
// - HolmesGPT-API: http://localhost:8088
package aianalysis

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

func TestAIAnalysisE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AIAnalysis E2E Test Suite")
}

var (
	ctx    context.Context
	cancel context.CancelFunc
	logger logr.Logger

	// Cluster configuration
	clusterName    string
	kubeconfigPath string

	// Namespace for infrastructure (fixed)
	infraNamespace = "kubernaut-system" //nolint:unused

	// Kubernetes client
	k8sClient client.Client

	// DataStorage OpenAPI client (DD-API-001: MANDATORY)
	dsClient *dsgen.Client

	// Service URLs (per DD-TEST-001)
	healthURL  string
	metricsURL string

	// Track failures for cleanup decision
	anyTestFailed bool
)

var _ = SynchronizedBeforeSuite(
	// This runs on process 1 only - create cluster once
	func() []byte {
		// Initialize logger for process 1
		logger = kubelog.NewLogger(kubelog.Options{
			Development: true,
			Level:       0,
			ServiceName: "aianalysis-e2e-test",
		})

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("AIAnalysis E2E Test Suite - Setup (Process 1)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Setting up KIND cluster with full dependency chain:")
		logger.Info("  • PostgreSQL + Redis (Data Storage dependencies)")
		logger.Info("  • Data Storage (audit trails)")
		logger.Info("  • HolmesGPT-API (AI analysis with mock LLM)")
		logger.Info("  • AIAnalysis controller")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Set cluster configuration
		clusterName = "aianalysis-e2e"
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())
		kubeconfigPath = fmt.Sprintf("%s/.kube/aianalysis-e2e-config", homeDir)

		// Create KIND cluster with full dependency chain (ONCE for all processes)
		// Per DD-TEST-002: Use hybrid parallel setup (build images FIRST, then cluster)
		// Infrastructure deployed to kubernaut-system; tests create dynamic namespaces
		logger.Info("Creating Kind cluster with hybrid parallel setup...")
		err = infrastructure.CreateAIAnalysisClusterHybrid(clusterName, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		logger.Info("✅ Cluster created successfully")
		logger.Info(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))
		logger.Info("  • Infrastructure: kubernaut-system")
		logger.Info("  • Tests will create dynamic namespaces per test")
		logger.Info("  • DD-TEST-011: Workflows seeded and ConfigMap created in infrastructure setup")
		logger.Info("  • Mock LLM will mount ConfigMap with workflow UUIDs at startup")

		logger.Info("  • Process 1 will now share kubeconfig with other processes")

		// Return kubeconfig path to all processes
		return []byte(kubeconfigPath)
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
			ServiceName: fmt.Sprintf("aianalysis-e2e-test-p%d", GinkgoParallelProcess()),
		})

		// Initialize failure tracking
		anyTestFailed = false

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info(fmt.Sprintf("AIAnalysis E2E Test Suite - Setup (Process %d)", GinkgoParallelProcess()))
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Connecting to cluster created by process 1")
		logger.Info(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))

		// Set cluster name
		clusterName = "aianalysis-e2e"

		// Set KUBECONFIG environment variable
		err := os.Setenv("KUBECONFIG", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		// Register AIAnalysis scheme
		err = aianalysisv1alpha1.AddToScheme(scheme.Scheme)
		Expect(err).ToNot(HaveOccurred())

		// Create Kubernetes client
		cfg, err := config.GetConfig()
		Expect(err).ToNot(HaveOccurred())

		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())

		// Set service URLs (per DD-TEST-001 port allocation)
		// AIAnalysis ports: API=8084/30084, Metrics=9184/30184, Health=8184/30284
		healthURL = "http://localhost:8184"  // AIAnalysis health via NodePort 30284
		metricsURL = "http://localhost:9184" // AIAnalysis metrics via NodePort 30184

		// Wait for all services to be ready
		// Per DD-TEST-002: Coverage-instrumented binaries take longer to start
		// Increase timeout from 60s to 300s for coverage builds (5 min)
		// Initial delay to allow HTTP servers to start accepting connections
		healthTimeout := 60 * time.Second
		initialDelay := 0 * time.Second
		if os.Getenv("E2E_COVERAGE") == "true" {
			healthTimeout = 300 * time.Second // 5 minutes for coverage builds
			initialDelay = 10 * time.Second   // Give servers 10s to start
			logger.Info("Coverage build detected - using extended health check timeout (300s) with 10s initial delay")
			time.Sleep(initialDelay)
		}
		logger.Info("Waiting for services to be ready...")
		Eventually(func() bool {
			ready := checkServicesReady()
			if !ready {
				logger.V(1).Info(fmt.Sprintf("Services not ready yet, will retry... (Health: %s/healthz, Metrics: %s/metrics)", healthURL, metricsURL))
			}
			return ready
		}, healthTimeout, 3*time.Second).Should(BeTrue(), "AIAnalysis services should become ready")

		// DD-API-001: Initialize DataStorage OpenAPI client (MANDATORY)
		// Per DD-API-001: Direct HTTP to DataStorage is FORBIDDEN
		// All queries MUST use generated OpenAPI client for type safety
		dataStorageURL := "http://localhost:8091" // DataStorage NodePort 30081
		dsClient, err = dsgen.NewClient(dataStorageURL)
		if err != nil {
			logger.Error(err, "Failed to create DataStorage OpenAPI client")
			Fail(fmt.Sprintf("DD-API-001 violation: Cannot proceed without DataStorage client: %v", err))
		}
		logger.Info("✅ DataStorage OpenAPI client initialized (DD-API-001 compliant)")

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info(fmt.Sprintf("✅ Process %d ready!", GinkgoParallelProcess()))
		logger.Info(fmt.Sprintf("  • Health: %s/healthz", healthURL))
		logger.Info(fmt.Sprintf("  • Metrics: %s/metrics", metricsURL))
		logger.Info(fmt.Sprintf("  • DataStorage API: %s (OpenAPI client)", dataStorageURL))
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
		logger.Info("AIAnalysis E2E Test Suite - Teardown (Process 1)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Detect setup failure: if k8sClient is nil, BeforeSuite failed
		setupFailed := k8sClient == nil
		if setupFailed {
			logger.Info("⚠️  Setup failure detected (k8sClient is nil)")
		}

		// Determine cleanup strategy
		preserveCluster := os.Getenv("SKIP_CLEANUP") == "true" || os.Getenv("KEEP_CLUSTER") != ""

		if preserveCluster {
			logger.Info("⚠️  CLUSTER PRESERVED FOR DEBUGGING")
			logger.Info("Reason:")
			if os.Getenv("SKIP_CLEANUP") == "true" {
				logger.Info("  • SKIP_CLEANUP=true")
			}
			if os.Getenv("KEEP_CLUSTER") != "" {
				logger.Info("  • KEEP_CLUSTER set")
			}
			if setupFailed {
				logger.Info("  • Setup failed (BeforeSuite failure)")
			}
			if anyTestFailed {
				logger.Info("  • Tests failed")
			}
			logger.Info("")
			logger.Info("To debug:")
			logger.Info(fmt.Sprintf("  export KUBECONFIG=%s", kubeconfigPath))
			logger.Info("  kubectl get aianalyses -A")
			logger.Info("  kubectl logs -n kubernaut-system deployment/aianalysis-controller")
			logger.Info("")
			logger.Info("To cleanup manually:")
			logger.Info(fmt.Sprintf("  kind delete cluster --name %s", clusterName))
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			return
		}

		// Delete cluster (with must-gather log export on failure)
		// Pass true for testsFailed if EITHER setup failed OR any test failed
		anyFailure := setupFailed || anyTestFailed
		logger.Info("🗑️  Cleaning up cluster...")
		err := infrastructure.DeleteAIAnalysisCluster(clusterName, kubeconfigPath, anyFailure, GinkgoWriter)
		if err != nil {
			logger.Error(err, "Failed to delete cluster")
		}

		By("Cleaning up service images built for Kind")
		// Remove service image built for this test run
		imageTag := os.Getenv("IMAGE_TAG") // Set by build/test infrastructure
		if imageTag != "" {
			serviceName := "aianalysis"
			imageName := fmt.Sprintf("%s:%s", serviceName, imageTag)

			pruneCmd := exec.Command("podman", "rmi", imageName)
			pruneOutput, pruneErr := pruneCmd.CombinedOutput()
			if pruneErr != nil {
				logger.Info(fmt.Sprintf("⚠️  Failed to remove service image: %v\n%s", pruneErr, pruneOutput))
			} else {
				logger.Info(fmt.Sprintf("✅ Service image removed: %s", imageName))
			}
		}

		By("Pruning dangling images from Kind builds")
		// Prune any dangling images left from failed builds
		pruneCmd := exec.Command("podman", "image", "prune", "-f")
		_, _ = pruneCmd.CombinedOutput()
		logger.Info("✅ E2E cleanup complete")

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Cluster Teardown Complete")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
)

// checkServicesReady checks if all required services are healthy
func checkServicesReady() bool {
	// Check AIAnalysis controller health endpoint
	healthResp, err := http.Get(healthURL + "/healthz")
	if err != nil || healthResp.StatusCode != 200 {
		return false
	}
	defer func() { _ = healthResp.Body.Close() }()

	// Check metrics endpoint
	metricsResp, err := http.Get(metricsURL + "/metrics")
	if err != nil || metricsResp.StatusCode != 200 {
		return false
	}
	defer func() { _ = metricsResp.Body.Close() }()

	return true
}

// randomSuffix generates a unique suffix for test resource names
// Uses UUID to guarantee uniqueness across parallel processes
func randomSuffix() string {
	return uuid.New().String()[:8]
}

// createTestNamespace creates a uniquely named namespace for test isolation.
// Uses UUID to guarantee uniqueness across parallel Ginkgo processes.
func createTestNamespace(prefix string) string {
	name := fmt.Sprintf("%s-%s", prefix, uuid.New().String()[:8])
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"kubernaut.io/test": "e2e-aianalysis",
			},
		},
	}
	err := k8sClient.Create(ctx, ns)
	Expect(err).ToNot(HaveOccurred())
	return name
}
