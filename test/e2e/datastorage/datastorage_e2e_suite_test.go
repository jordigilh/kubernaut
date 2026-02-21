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

package datastorage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	_ "github.com/jackc/pgx/v5/stdlib"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Test suite for Data Storage E2E tests
// This suite sets up a complete production-like environment:
// - Kind cluster (2 nodes: 1 control-plane + 1 worker) with NodePort exposure
// - PostgreSQL 16 (V1.0 label-only, for workflow catalog)
// - Redis (for DLQ fallback)
// - Data Storage service (deployed to Kind cluster)
//
// ARCHITECTURE: Uses SHARED deployment pattern (like Gateway E2E tests)
// - Services deployed ONCE in SynchronizedBeforeSuite
// - All tests share the same infrastructure via NodePort (no port-forwarding)
// - Eliminates kubectl port-forward instability
// - Faster execution, no per-test deployment overhead
//
// E2E Test Coverage (10-15%):
// - Scenario 1: Happy Path - Complete remediation audit trail
// - Scenario 2: DLQ Fallback - Data Storage Service outage recovery
// - Scenario 3: Query API - Timeline retrieval with filtering
// - Scenario 4: Workflow Search - Hybrid weighted scoring
// - Scenario 5: [REMOVED] Embedding Service (V1.0: label-only architecture)
// - Scenario 6: Workflow Search Audit Trail - Audit event generation

func TestDataStorageE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Storage E2E Suite")
}

var (
	ctx    context.Context
	cancel context.CancelFunc
	logger logr.Logger

	// Cluster configuration (shared across all tests)
	clusterName    string
	kubeconfigPath string

	// Shared service URLs (NodePort - no port-forwarding needed)
	// These are set in SynchronizedBeforeSuite and available to all tests
	dataStorageURL string // http://localhost:28090 (NodePort 30081 mapped via Kind extraPortMappings per DD-TEST-001)
	postgresURL    string // localhost:25433 (NodePort 30432 mapped via Kind extraPortMappings per DD-TEST-001)

	// DSClient is the shared authenticated OpenAPI client for E2E tests (DD-AUTH-014)
	//
	// USAGE PATTERN (DD-AUTH-014 - Zero Trust):
	//   - Use DSClient for functional tests (audit, workflow, metrics)
	//   - Create custom clients for authorization tests (SAR scenarios)
	//
	// This client is authenticated with the shared E2E ServiceAccount
	// (datastorage-e2e-client) which has full CRUD RBAC permissions.
	//
	// Authority: DD-API-001 (OpenAPI Client Mandate)
	// Authority: DD-AUTH-014 (Middleware-based Authentication)
	DSClient *dsgen.Client

	// AuthHTTPClient is an authenticated HTTP client for tests requiring raw HTTP calls
	// (e.g., 409 Conflict responses not yet in OpenAPI spec, or detailed response inspection)
	//
	// Authority: DD-AUTH-014 (Middleware-based Authentication)
	AuthHTTPClient *http.Client

	// Shared PostgreSQL connection for E2E test verification
	// NOTE: E2E tests should prefer API verification over direct DB access
	// This is provided for tests migrated from integration that require DB verification
	testDB *sql.DB

	// Shared namespace for all tests (services deployed ONCE)
	sharedNamespace string = "datastorage-e2e"

	// Track if any test failed (for cluster cleanup decision)
	anyTestFailed bool

	// Coverage mode detection (DD-TEST-007: E2E Coverage Capture Standard)
	coverageMode bool
	coverDir     string = "./coverdata"
)

// Note: Helper functions (generateUniqueNamespace, createNamespace, deleteNamespace, etc.)
// are defined in helpers.go to avoid duplication

var _ = SynchronizedBeforeSuite(
	// This function runs ONCE on process 1 only
	func() []byte {
		// Initialize context for process 1
		ctx, cancel = context.WithCancel(context.Background())

		// Initialize logger for process 1 (DD-005 v2.0: logr.Logger migration)
		logger = kubelog.NewLogger(kubelog.DevelopmentOptions())

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Data Storage E2E Test Suite - Cluster Setup (ONCE - Process 1)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// DD-TEST-007: E2E Coverage Capture Standard
		// Detect if coverage mode is enabled via E2E_COVERAGE environment variable
		coverageMode = os.Getenv("E2E_COVERAGE") == "true"
		if coverageMode {
			logger.Info("📊 DD-TEST-007: E2E Coverage mode ENABLED")
			// Create coverage directory for Go 1.20+ binary profiling
			if err := os.MkdirAll(coverDir, 0777); err != nil {
				logger.Info("⚠️  Failed to create coverage directory", "error", err)
			} else {
				logger.Info("   ✅ Coverage directory created", "path", coverDir)
				logger.Info("   💡 Coverage data will be extracted from Kind node after tests")
			}
		} else {
			logger.Info("📊 DD-TEST-007: E2E Coverage mode DISABLED (set E2E_COVERAGE=true to enable)")
		}

		logger.Info("Creating Kind cluster with NodePort exposure...")
		logger.Info("  • Kind cluster (2 nodes: control-plane + worker)")
		logger.Info("  • NodePort exposure: Data Storage (30081→8081), PostgreSQL (30432→5432)")
		logger.Info("  • PostgreSQL 16 (V1.0 label-only, workflow catalog, SOC2 audit storage)")
		logger.Info("  • Redis (DLQ fallback)")
		logger.Info("  • Data Storage Docker image (build + load)")
		logger.Info("  • Kubeconfig: ~/.kube/datastorage-e2e-config")
		logger.Info("")
		logger.Info("Note: All tests share the same infrastructure via NodePort")
		logger.Info("      No kubectl port-forward needed - eliminates instability")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Set cluster configuration
		clusterName = "datastorage-e2e"
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())
		// Use isolated kubeconfig path per TESTING_GUIDELINES.md section "Kubeconfig Isolation Policy"
		// Convention: ~/.kube/{serviceName}-e2e-config (NEVER ~/.kube/config)
		kubeconfigPath = fmt.Sprintf("%s/.kube/datastorage-e2e-config", homeDir)

		// Create infrastructure with parallel setup (ONCE for all tests)
		// This uses parallel optimization: Build image | PostgreSQL | Redis run concurrently
		// Saves ~1 minute per E2E run (~23% faster)
		logger.Info("🚀 Setting up DataStorage E2E infrastructure (PARALLEL MODE)...")
		logger.Info("   Expected: ~3.6 min (vs ~4.7 min sequential)")
		// Generate unique image name per DD-TEST-001 compliant naming
		dataStorageImage := infrastructure.GenerateInfraImageName("datastorage", "datastorage-e2e")
		err = infrastructure.SetupDataStorageInfrastructureParallel(ctx, clusterName, kubeconfigPath, sharedNamespace, dataStorageImage, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		// Wait for Data Storage HTTP endpoint to be responsive via NodePort
		logger.Info("⏳ Waiting for Data Storage NodePort to be responsive...")
		tempClient := &http.Client{Timeout: 10 * time.Second}
		Eventually(func() error {
			resp, err := tempClient.Get("http://localhost:28090/health") // Per DD-TEST-001 (NodePort 30081 → host 28090)
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("health check returned status %d", resp.StatusCode)
			}
			return nil
		}, 120*time.Second, 2*time.Second).Should(Succeed(), "Data Storage NodePort did not become responsive")
		logger.Info("✅ Data Storage is ready via NodePort (localhost:28090)")

		// DD-API-001 + DD-AUTH-014: Initialize OpenAPI client with ServiceAccount authentication
		logger.Info("📋 DD-API-001 + DD-AUTH-014: Creating ServiceAccount for E2E tests...")
		e2eSAName := "datastorage-e2e-client"
		testNamespace := "datastorage-e2e"
		err = infrastructure.CreateE2EServiceAccountWithDataStorageAccess(
			ctx,
			testNamespace,
			kubeconfigPath,
			e2eSAName,
			GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create E2E ServiceAccount")

		// Get token for E2E ServiceAccount
		var e2eToken string
		e2eToken, err = infrastructure.GetServiceAccountToken(
			ctx,
			testNamespace,
			e2eSAName,
			kubeconfigPath,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to get E2E ServiceAccount token")
		logger.Info("✅ E2E ServiceAccount created with DataStorage access", "name", e2eSAName)

		logger.Info("📋 DD-API-001 + DD-AUTH-014: Creating shared authenticated clients for E2E tests...")
		saTransport := testauth.NewServiceAccountTransport(e2eToken)
		httpClient := &http.Client{
			Timeout:   20 * time.Second, // DD-AUTH-014: 20s timeout for 12 parallel processes with SAR middleware (API server tuned, see kind-datastorage-config.yaml)
			Transport: saTransport,
		}
		DSClient, err = dsgen.NewClient(
			"http://localhost:28090",
			dsgen.WithClient(httpClient),
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create DataStorage OpenAPI client")

		// Also export authenticated HTTP client for tests needing raw HTTP (non-spec responses)
		AuthHTTPClient = &http.Client{
			Timeout:   10 * time.Second,
			Transport: saTransport,
		}
		logger.Info("✅ Shared authenticated clients created (DD-AUTH-014)",
			"baseURL", "http://localhost:28090",
			"pattern", "Use DSClient for spec-compliant APIs, AuthHTTPClient for non-spec responses (409, etc)")

		// Note: Certificate warm-up is SKIPPED in suite setup
		// Rationale: cert-manager is installed per-test-suite (e.g., SOC2 tests),
		// not in global infrastructure. Each test suite that needs cert-manager
		// will install and warm it up in its BeforeAll block.
		// This keeps suite setup fast and avoids unnecessary cert-manager dependency
		// for tests that don't need digital signatures.
		logger.Info("📋 Certificate generation: Delegated to test suites (SOC2 tests install cert-manager)")

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Cluster Setup Complete - Broadcasting to all processes")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Cluster configuration", "cluster", clusterName, "kubeconfig", kubeconfigPath)
		logger.Info("Service URLs (per DD-TEST-001)", "dataStorage", "http://localhost:28090", "postgresql", "localhost:25433")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Return kubeconfig path and ServiceAccount token to all processes
		setupData := map[string]string{
			"kubeconfig": kubeconfigPath,
			"token":      e2eToken,
		}
		setupJSON, err := json.Marshal(setupData)
		Expect(err).ToNot(HaveOccurred(), "Failed to marshal setup data")
		return setupJSON
	},
	// This function runs on ALL processes (including process 1)
	func(data []byte) {
		// Initialize context
		ctx, cancel = context.WithCancel(context.Background())

		// Initialize logger for this process (DD-005 v2.0: logr.Logger migration)
		logger = kubelog.NewLogger(kubelog.DevelopmentOptions())

		// Initialize failure tracking
		anyTestFailed = false

		// Receive kubeconfig path and ServiceAccount token from process 1
		var setupData map[string]string
		err := json.Unmarshal(data, &setupData)
		Expect(err).ToNot(HaveOccurred(), "Failed to unmarshal setup data")
		kubeconfigPath = setupData["kubeconfig"]
		e2eToken := setupData["token"]
		clusterName = "datastorage-e2e"

		// Set shared URLs - NodePort or port-forward depending on Kind provider
		// Per DD-TEST-001: DataStorage E2E uses ports 25433-28139
		// Kind with Docker: extraPortMappings work (localhost:25433)
		// Kind with Podman: extraPortMappings DON'T work, need port-forward
		processID := GinkgoParallelProcess()

		// Try NodePort first (works with Docker) - Per DD-TEST-001 lines 106-127
		dataStorageURL = "http://localhost:28090"
		postgresURL = "postgresql://slm_user:test_password@localhost:25433/action_history?sslmode=disable"

		// Test if NodePort is accessible (check PostgreSQL connection)
		testDB, err = sql.Open("pgx", postgresURL)
		nodePortWorks := false
		if err == nil {
			if err := testDB.Ping(); err == nil {
				nodePortWorks = true
				logger.Info("✅ NodePort accessible (Docker provider) - testDB ready", "process", processID)
				// Keep testDB open for use by E2E tests (closed in AfterSuite)
			} else {
				_ = testDB.Close()
				testDB = nil
			}
		} else {
			testDB = nil
		}

		// If NodePort doesn't work, use kubectl port-forward (Podman)
		if !nodePortWorks {
			logger.Info("⚠️  NodePort not accessible (Podman provider) - starting port-forward", "process", processID)

			// Start port-forward for PostgreSQL (background process)
			// Use process-specific ports to avoid conflicts in parallel execution
			// Per DD-TEST-001: Base ports 25433 (PostgreSQL), 28090 (DataStorage)
			pgLocalPort := 25433 + (processID * 100)
			dsLocalPort := 28090 + (processID * 100)

			// PostgreSQL port-forward
			go func() {
				cmd := exec.Command("kubectl", "port-forward",
					"--kubeconfig", kubeconfigPath,
					"-n", "datastorage-e2e",
					"svc/postgresql",
					fmt.Sprintf("%d:5432", pgLocalPort))
				if err := cmd.Run(); err != nil {
					logger.Error(err, "PostgreSQL port-forward failed", "process", processID)
				}
			}()

			// DataStorage port-forward
			go func() {
				cmd := exec.Command("kubectl", "port-forward",
					"--kubeconfig", kubeconfigPath,
					"-n", "datastorage-e2e",
					"svc/datastorage",
					fmt.Sprintf("%d:8080", dsLocalPort))
				if err := cmd.Run(); err != nil {
					logger.Error(err, "DataStorage port-forward failed", "process", processID)
				}
			}()

			// Per TESTING_GUIDELINES.md: Use Eventually() to verify port-forward is ready
			Eventually(func() bool {
				// Test port is accessible
				conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", dsLocalPort), 500*time.Millisecond)
				if err != nil {
					return false
				}
				_ = conn.Close()
				return true
			}, 30*time.Second, 1*time.Second).Should(BeTrue(), "Port-forward should be established")

			// Update URLs to use process-specific ports
			dataStorageURL = fmt.Sprintf("http://localhost:%d", dsLocalPort)
			postgresURL = fmt.Sprintf("postgresql://slm_user:test_password@localhost:%d/action_history?sslmode=disable", pgLocalPort)

			// Connect to PostgreSQL via port-forward
			testDB, err = sql.Open("pgx", postgresURL)
			if err != nil {
				logger.Error(err, "Failed to open PostgreSQL connection via port-forward")
			} else if err := testDB.Ping(); err != nil {
				logger.Error(err, "Failed to ping PostgreSQL via port-forward")
				_ = testDB.Close()
				testDB = nil
			}

			logger.Info("✅ Port-forward established", "process", processID,
				"dataStorageURL", dataStorageURL,
				"postgresURL", postgresURL,
				"testDB", testDB != nil)
		}

		logger.Info("🔌 URLs configured",
			"process", processID,
			"dataStorageURL", dataStorageURL,
			"postgresURL", postgresURL,
			"method", map[bool]string{true: "NodePort", false: "port-forward"}[nodePortWorks])

		// DD-API-001 + DD-AUTH-014: Initialize shared authenticated OpenAPI client for this process
		logger.Info("📋 DD-API-001 + DD-AUTH-014: Creating shared authenticated OpenAPI client for process", "process", processID)
		saTransport := testauth.NewServiceAccountTransport(e2eToken)
		httpClient := &http.Client{
			Timeout:   20 * time.Second, // DD-AUTH-014: 20s timeout for 12 parallel processes with SAR middleware
			Transport: saTransport,
		}
		DSClient, err = dsgen.NewClient(
			dataStorageURL,
			dsgen.WithClient(httpClient),
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create DataStorage OpenAPI client")

		// Also export authenticated HTTP client for tests needing raw HTTP (non-spec responses)
		AuthHTTPClient = &http.Client{
			Timeout:   10 * time.Second,
			Transport: saTransport,
		}

		logger.Info("✅ Shared authenticated clients created (DD-AUTH-014)",
			"process", processID,
			"baseURL", dataStorageURL,
			"pattern", "Use DSClient for spec-compliant APIs, AuthHTTPClient for non-spec responses (409, etc)")

		// Note: We do NOT set KUBECONFIG environment variable to avoid affecting other tests
		// All kubectl commands must use --kubeconfig flag explicitly
		logger.Info("Process ready", "process", processID, "kubeconfig", kubeconfigPath)
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
	// This function runs on ALL processes (cleanup per-process resources)
	func() {
		processID := GinkgoParallelProcess()
		logger.Info("Process cleanup complete",
			"process", processID,
			"hadFailures", anyTestFailed)

		// Close PostgreSQL connection for this process
		if testDB != nil {
			_ = testDB.Close()
		}

		// Cancel context for this process
		if cancel != nil {
			cancel()
		}

		// Sync logger for this process (DD-005 v2.0: use kubelog.Sync)
		kubelog.Sync(logger)
	},
	// This function runs ONCE on process 1 only (cleanup shared resources)
	func() {
		// Re-initialize logger for final cleanup (DD-005 v2.0: logr.Logger migration)
		logger = kubelog.NewLogger(kubelog.DevelopmentOptions())

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Data Storage E2E Test Suite - Cleanup (Process 1)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Detect setup failure: if DSClient is nil, BeforeSuite failed
		setupFailed := DSClient == nil
		if setupFailed {
			logger.Info("⚠️  Setup failure detected (DSClient is nil)")
		}

		// Check if we should keep the cluster for debugging
		// Note: In parallel execution, anyTestFailed may not capture all process failures
		// Use KEEP_CLUSTER=always to force preservation, or check test exit code
		keepCluster := os.Getenv("KEEP_CLUSTER")

		// In SynchronizedAfterSuite, we're in process 1 which may not have run failing tests
		// The safest approach: always export logs if ANY process reported failures
		// We'll check this by looking at the captured anyTestFailed flag from process cleanup
		// Also check for setup failures (BeforeSuite failures)
		suiteFailed := setupFailed || anyTestFailed || infrastructure.CheckTestFailure(clusterName) || keepCluster == "true" || keepCluster == "always"
		defer infrastructure.CleanupFailureMarker(clusterName)

		// DD-TEST-007: Collect E2E binary coverage BEFORE cluster deletion
		if coverageMode {
			if err := infrastructure.CollectE2EBinaryCoverage(infrastructure.E2ECoverageOptions{
				ServiceName:    "datastorage",
				ClusterName:    clusterName,
				DeploymentName: "datastorage",
				Namespace:      sharedNamespace,
				KubeconfigPath: kubeconfigPath,
			}, GinkgoWriter); err != nil {
				logger.Error(err, "Failed to collect E2E binary coverage (non-fatal)")
			}
		}

		if suiteFailed {
			logger.Info("⚠️  Test failure detected - collecting diagnostic information...")

			// Export cluster logs (like must-gather) BEFORE preserving cluster
			logger.Info("📋 Exporting cluster logs (Kind must-gather)...")
			logsDir := "/tmp/datastorage-e2e-logs-" + time.Now().Format("20060102-150405")
			exportCmd := exec.Command("kind", "export", "logs", logsDir, "--name", clusterName)
			if exportOutput, exportErr := exportCmd.CombinedOutput(); exportErr != nil {
				logger.Error(exportErr, "Failed to export Kind logs",
					"output", string(exportOutput),
					"logs_dir", logsDir)
			} else {
				logger.Info("✅ Cluster logs exported successfully",
					"logs_dir", logsDir)
				logger.Info("📁 Logs include: pod logs, node logs, kubelet logs, and more")

				// Extract and display DataStorage server logs for immediate analysis
				dsLogPattern := logsDir + "/*/datastorage-e2e_data-storage-service-*/*.log"
				findCmd := exec.Command("sh", "-c", "ls "+dsLogPattern+" 2>/dev/null | head -1")
				if logPath, err := findCmd.Output(); err == nil && len(logPath) > 0 {
					logPathStr := strings.TrimSpace(string(logPath))
					logger.Info("📄 DataStorage server log location", "path", logPathStr)

					// Display last 100 lines of server log
					tailCmd := exec.Command("tail", "-100", logPathStr)
					if tailOutput, tailErr := tailCmd.CombinedOutput(); tailErr == nil {
						logger.Info("═══════════════════════════════════════════════════════════")
						logger.Info("📋 DATASTORAGE SERVER LOG (Last 100 lines)")
						logger.Info("═══════════════════════════════════════════════════════════")
						logger.Info(string(tailOutput))
						logger.Info("═══════════════════════════════════════════════════════════")
					}
				}
			}

			logger.Info("⚠️  Keeping cluster for debugging (KEEP_CLUSTER=true or test failed)")
			logger.Info("Cluster details for debugging",
				"cluster", clusterName,
				"kubeconfig", kubeconfigPath,
				"dataStorageURL", dataStorageURL,
				"postgresURL", postgresURL,
				"logs_exported", logsDir)
			logger.Info("To delete the cluster manually: kind delete cluster --name " + clusterName)
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			return
		}

		// Delete Kind cluster (no log export needed - tests passed)
		logger.Info("🗑️  Deleting Kind cluster...")
		if err := infrastructure.DeleteCluster(clusterName, "datastorage", false, GinkgoWriter); err != nil {
			logger.Error(err, "Failed to delete cluster")
		} else {
			logger.Info("✅ Cluster deleted successfully")
		}

		// DD-TEST-001 v1.1: Clean up service images built for Kind
		logger.Info("🧹 DD-TEST-001 v1.1: Cleaning up service images...")
		imageRegistry := os.Getenv("IMAGE_REGISTRY")
		imageTag := os.Getenv("IMAGE_TAG")

		// Skip cleanup when using registry images (CI/CD mode)
		// In registry mode, images are pulled (not built locally), so local removal fails
		if imageRegistry != "" && imageTag != "" {
			logger.Info("ℹ️  Registry mode detected - skipping local image removal",
				"registry", imageRegistry, "tag", imageTag)
		} else if imageTag != "" {
			// Local build mode: Remove locally built images
			serviceName := "datastorage"
			imageName := fmt.Sprintf("%s:%s", serviceName, imageTag)

			pruneCmd := exec.Command("podman", "rmi", imageName)
			pruneOutput, pruneErr := pruneCmd.CombinedOutput()
			if pruneErr != nil {
				logger.Info("⚠️  Failed to remove service image (may not exist)",
					"image", imageName,
					"error", pruneErr,
					"output", string(pruneOutput))
			} else {
				logger.Info("✅ Service image removed", "image", imageName, "saved", "~200-500MB")
			}
		} else {
			logger.Info("⚠️  IMAGE_TAG not set, skipping service image cleanup")
		}

		// Prune dangling images from Kind builds
		logger.Info("🧹 Pruning dangling images from Kind builds...")
		pruneDanglingCmd := exec.Command("podman", "image", "prune", "-f")
		_, _ = pruneDanglingCmd.CombinedOutput()
		logger.Info("✅ Dangling images pruned")

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
)
