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

package holmesgptapi

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	hapiclient "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// Test suite for HolmesGPT API (HAPI) E2E tests
// This suite sets up a complete production-like environment:
// - Kind cluster with NodePort exposure
// - PostgreSQL 16 (for Data Storage dependency)
// - Redis (for Data Storage dependency)
// - Data Storage service (HAPI dependency)
// - Mock LLM service (for cost-effective testing)
// - HolmesGPT API service (containerized Python FastAPI)
//
// ARCHITECTURE: Standalone HAPI E2E (separate from AIAnalysis)
// - HAPI has its own E2E infrastructure
// - AIAnalysis E2E depends on HAPI (as client), not vice versa
// - Services deployed ONCE in SynchronizedBeforeSuite
// - All tests share infrastructure via NodePort
//
// E2E Test Coverage (10-15%):
// - 48 Go test scenarios (migrated from Python)
// - Black-box HTTP API testing via ogen OpenAPI client
// - Mock LLM mode for cost control
// - Business outcome validation (behavior + correctness + business impact)

func TestHolmesGPTAPIE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	// Suite timeout: 15 minutes total (5 min infra + max 5 min pytest + 5 min buffer)
	// pytest-timeout enforces 30s per test to prevent hangs
	// Pattern: Balanced timeout - fast feedback without premature termination
	SetDefaultEventuallyTimeout(15 * time.Minute)
	RunSpecs(t, "HolmesGPT API E2E Suite")
}

var (
	ctx    context.Context
	cancel context.CancelFunc
	logger logr.Logger

	// Cluster configuration (shared across all tests)
	clusterName    string
	kubeconfigPath string

	// Shared service URLs (NodePort - no port-forwarding needed)
	// Port allocations per DD-TEST-001 v2.5:
	// - HAPI: 30120 (NodePort) → 8080 (container)
	// - Data Storage: 30089 (NodePort) → 8080 (container, Host Port 8089)
	// - PostgreSQL: 30439 (NodePort) → 5432 (container)
	// - Redis: 30387 (NodePort) → 6379 (container)
	hapiURL        string // http://localhost:30120
	dataStorageURL string // http://localhost:8089

	// Shared namespace for all tests (services deployed ONCE)
	sharedNamespace string = "holmesgpt-api-e2e"

	// Shared HAPI client (authenticated with ServiceAccount)
	hapiClient *hapiclient.Client

	// Shared HAPI session client (BR-AA-HAPI-064: async submit/poll/result wrapper)
	sessionClient *hapiclient.HolmesGPTClient

	// Track if any test failed (for cluster cleanup decision)
	anyTestFailed bool

	// Track if BeforeSuite completed successfully (for setup failure detection)
	setupSucceeded bool

	// Path to project root (for test helpers)
	projectRoot string
)

var _ = SynchronizedBeforeSuite(
	// This function runs ONCE on process 1 only
	func() []byte {
		// Initialize context for process 1
		ctx, cancel = context.WithCancel(context.Background())

		// Initialize logger for process 1
		logger = kubelog.NewLogger(kubelog.DevelopmentOptions())

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("HolmesGPT API (HAPI) E2E Test Suite - Cluster Setup (ONCE - Process 1)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		logger.Info("Creating Kind cluster with NodePort exposure...")
		logger.Info("  • Kind cluster (2 nodes: control-plane + worker)")
		logger.Info("  • NodePort exposure per DD-TEST-001 v1.8:")
		logger.Info("    - HAPI: 30120 → 8080")
		logger.Info("    - Data Storage: 30089 → 8080 (Host Port 8089)")
		logger.Info("    - PostgreSQL: 30439 → 5432")
		logger.Info("    - Redis: 30387 → 6379")
		logger.Info("  • Mock LLM mode: MOCK_LLM=true")
		logger.Info("  • Kubeconfig: ~/.kube/holmesgpt-api-e2e-config")
		logger.Info("")
		logger.Info("Note: All tests share the same infrastructure via NodePort")
		logger.Info("      Python pytest tests run against deployed HAPI service")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Set cluster configuration
		clusterName = "holmesgpt-api-e2e"
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())
		// Use isolated kubeconfig path per TESTING_GUIDELINES.md
		kubeconfigPath = fmt.Sprintf("%s/.kube/holmesgpt-api-e2e-config", homeDir)

		// Get project root
		cwd, err := os.Getwd()
		Expect(err).ToNot(HaveOccurred())
		projectRoot = filepath.Join(cwd, "../../..")

		// Create HAPI E2E infrastructure
		// This creates: Kind cluster + PostgreSQL + Redis + Data Storage + HAPI
		logger.Info("🚀 Setting up HAPI E2E infrastructure...")
		logger.Info("   Expected: ~5-7 min (sequential builds to avoid OOM)")
		err = infrastructure.SetupHAPIInfrastructure(ctx, clusterName, kubeconfigPath, sharedNamespace, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		// Set service URLs (per DD-TEST-001 v2.5: DataStorage uses host port mapping)
		hapiURL = "http://localhost:30120"
		dataStorageURL = "http://localhost:8089"

		// CRITICAL: Wait for Kind port mapping to stabilize (per notification E2E pattern)
		// Pods may be ready but NodePort routing needs time to propagate with podman provider
		logger.Info("⏳ Waiting 5 seconds for Kind NodePort mapping to stabilize...")
		time.Sleep(5 * time.Second)

		// Wait for Data Storage HTTP endpoint to be responsive via NodePort
		// Reduced timeout from 180s to 90s (per notification E2E pattern)
		// With stabilization wait, this should be sufficient
		logger.Info("⏳ Waiting for Data Storage service to be ready...")
		Eventually(func() error {
			resp, err := http.Get(dataStorageURL + "/health/ready")
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("health check returned %d", resp.StatusCode)
			}
			return nil
		}, 90*time.Second, 2*time.Second).Should(Succeed(), "Data Storage health check should succeed")

		// Wait for HAPI HTTP endpoint to be responsive via NodePort
		logger.Info("⏳ Waiting for HAPI service to be ready...")
		Eventually(func() error {
			resp, err := http.Get(hapiURL + "/health")
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("health check returned %d", resp.StatusCode)
			}
			return nil
		}, 90*time.Second, 2*time.Second).Should(Succeed(), "HAPI health check should succeed")

		logger.Info("✅ HAPI E2E infrastructure ready")
		logger.Info("   HAPI URL: " + hapiURL)
		logger.Info("   Data Storage URL: " + dataStorageURL)
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// ========================================
		// Initialize HAPI Client with ServiceAccount Authentication
		// ========================================
		// Note: Workflow seeding already handled by infrastructure.SetupHAPIInfrastructure()
		// - Workflows seeded in DataStorage (Phase 4c)
		// - Mock LLM ConfigMap created with actual UUIDs (Phase 4d)
		// - No duplicate seeding needed here
		logger.Info("🔐 Initializing HAPI client with ServiceAccount authentication...")

		// DD-AUTH-014: Get ServiceAccount token for HAPI client authentication
		saToken, err := infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "holmesgpt-api-e2e-sa", kubeconfigPath)
		if err != nil {
			logger.Info("❌ Failed to get ServiceAccount token")
			Fail(fmt.Sprintf("Failed to get ServiceAccount token: %v", err))
		}

		// Create authenticated HAPI client (same pattern as AA E2E tests)
		hapiClient, err = hapiclient.NewClient(
			hapiURL,
			hapiclient.WithClient(&http.Client{
				Transport: testauth.NewServiceAccountTransport(saToken),
				Timeout:   60 * time.Second,
			}),
		)
		if err != nil {
			logger.Info("❌ Failed to create authenticated HAPI client")
			Fail(fmt.Sprintf("Failed to create authenticated HAPI client: %v", err))
		}
		logger.Info("✅ Authenticated HAPI client initialized")

		// BR-AA-HAPI-064: Create session-aware client for async submit/poll/result tests
		sessionClient, err = hapiclient.NewHolmesGPTClientWithTransport(
			hapiclient.Config{BaseURL: hapiURL},
			testauth.NewServiceAccountTransport(saToken),
		)
		if err != nil {
			logger.Info("❌ Failed to create HAPI session client")
			Fail(fmt.Sprintf("Failed to create HAPI session client: %v", err))
		}
		logger.Info("✅ HAPI session client initialized (BR-AA-HAPI-064)")

		// Mark setup as successful (for setup failure detection in AfterSuite)
		setupSucceeded = true

		return []byte(kubeconfigPath)
	},
	// This function runs on ALL processes
	func(kubeconfigBytes []byte) {
		kubeconfigPath = string(kubeconfigBytes)
		ctx, cancel = context.WithCancel(context.Background())
		logger = kubelog.NewLogger(kubelog.DevelopmentOptions())

		// Initialize URLs for all processes (per DD-TEST-001 v2.5: DataStorage uses host port mapping)
		hapiURL = "http://localhost:30120"
		dataStorageURL = "http://localhost:8089"

		// Get project root
		cwd, err := os.Getwd()
		Expect(err).ToNot(HaveOccurred())
		projectRoot = filepath.Join(cwd, "../../..")

		// Initialize HAPI client for all processes
		// DD-AUTH-014: Get ServiceAccount token for authentication
		saToken, err := infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "holmesgpt-api-e2e-sa", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "Failed to get ServiceAccount token")

		// Create authenticated HAPI client
		hapiClient, err = hapiclient.NewClient(
			hapiURL,
			hapiclient.WithClient(&http.Client{
				Transport: testauth.NewServiceAccountTransport(saToken),
				Timeout:   60 * time.Second,
			}),
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create authenticated HAPI client")

		// BR-AA-HAPI-064: Create session-aware client for async submit/poll/result tests
		sessionClient, err = hapiclient.NewHolmesGPTClientWithTransport(
			hapiclient.Config{BaseURL: hapiURL},
			testauth.NewServiceAccountTransport(saToken),
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create HAPI session client")
	},
)

// Main E2E test suite - individual test files will be imported here
// Test files:
// - incident_analysis_test.go (E2E-HAPI-001 to 008)
// - recovery_analysis_test.go (E2E-HAPI-013 to 029)
// - workflow_catalog_test.go (E2E-HAPI-030 to 044)
// - audit_pipeline_test.go (E2E-HAPI-045 to 048)

var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		anyTestFailed = true
	}
})

var _ = SynchronizedAfterSuite(
	// This runs on ALL processes - no action needed
	func() {
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Process cleanup...")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
	// This runs ONCE on process 1 - cluster teardown
	func() {
		defer cancel()

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("HAPI E2E Test Suite - Teardown (ONCE - Process 1)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Detect setup failure: if setupSucceeded is false, BeforeSuite failed
		setupFailed := !setupSucceeded
		if setupFailed {
			logger.Info("⚠️  Setup failure detected (setupSucceeded = false)")
		}

		// Determine cleanup strategy
		anyFailure := setupFailed || anyTestFailed
		preserveCluster := os.Getenv("KEEP_CLUSTER") == "true"

		// DD-TEST-007: Collect Python E2E coverage BEFORE cluster deletion
		if os.Getenv("E2E_COVERAGE") == "true" {
			logger.Info("📊 DD-TEST-007: Collecting Python E2E coverage...")
			if err := infrastructure.CollectE2EPythonCoverage(infrastructure.E2EPythonCoverageOptions{
				ServiceName:        "holmesgpt-api",
				ClusterName:        clusterName,
				DeploymentName:     "holmesgpt-api",
				Namespace:          sharedNamespace,
				KubeconfigPath:     kubeconfigPath,
				SourceDir:          "holmesgpt-api/src",
				ContainerSourceDir: "/opt/app-root/src/src",
			}, GinkgoWriter); err != nil {
				logger.Error(err, "Failed to collect Python E2E coverage (non-fatal)")
			}
		}

		if preserveCluster {
			logger.Info("⚠️  CLUSTER PRESERVED FOR DEBUGGING (KEEP_CLUSTER=true)")
			logger.Info("")
			logger.Info("To debug:")
			logger.Info("  export KUBECONFIG=" + kubeconfigPath)
			logger.Info("  kubectl get pods -n " + sharedNamespace)
			logger.Info("  kubectl logs -n " + sharedNamespace + " deployment/holmesgpt-api")
			logger.Info("")
			logger.Info("To cleanup manually:")
			logger.Info("  kind delete cluster --name " + clusterName)
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			return
		}

		// Delete cluster (with must-gather log export on failure)
		logger.Info("🧹 Deleting Kind cluster...")
		err := infrastructure.DeleteCluster(clusterName, "holmesgpt-api", anyFailure, GinkgoWriter)
		if err != nil {
			logger.Info("⚠️  Warning: Failed to delete cluster", "error", err)
		} else {
			logger.Info("✅ Cluster deleted successfully")
		}

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
)
