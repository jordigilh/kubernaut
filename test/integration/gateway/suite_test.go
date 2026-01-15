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
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	_ "github.com/lib/pq" // PostgreSQL driver
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	testauth "github.com/jordigilh/kubernaut/pkg/testutil/auth"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// ========================================
// Gateway Integration Test Suite - WITH DataStorage Infrastructure
// ========================================
//
// PURPOSE:
// This suite runs Gateway business logic tests WITH real DataStorage infrastructure.
// Tests call ProcessSignal() directly and verify audit events in real PostgreSQL.
//
// INFRASTRUCTURE (Podman):
// - PostgreSQL (port 15439): Audit event persistence
// - DataStorage API (port 15440): Audit trail service
//
// KEY CHARACTERISTICS:
// - ✅ Real DataStorage (PostgreSQL + DataStorage API in Podman)
// - ✅ Per-process DataStorage client (parallel execution safe)
// - ✅ Direct ProcessSignal() calls (no HTTP layer)
// - ✅ Shared K8s client (envtest per process)
// - ✅ Real audit event validation
// - ❌ NO HTTP server/middleware
// - ❌ NO Kind cluster (uses envtest)
//
// PARALLEL EXECUTION:
// - Phase 1: Start shared infrastructure (Process 1 only)
// - Phase 2: Create per-process clients (ALL processes)
// - Each process has its own DataStorage client
// - Each process has its own envtest K8s API
//
// ========================================

const (
	// PostgreSQL configuration
	gatewayPostgresPort     = 15439
	gatewayPostgresUser     = "gateway_test"
	gatewayPostgresPassword = "gateway_test_password"
	gatewayPostgresDB       = "gateway_test"
	gatewayPostgresContainer = "gateway-integration-postgres"
	
	// DataStorage configuration
	gatewayDataStoragePort      = 15440
	gatewayDataStorageContainer = "gateway-integration-datastorage"
)

var (
	// Per-process resources (Phase 2)
	ctx       context.Context
	cancel    context.CancelFunc
	k8sClient client.Client
	logger    logr.Logger
	testEnv   *envtest.Environment
	k8sConfig *rest.Config
	dsClient  *audit.OpenAPIClientAdapter // ← NEW: Per-process DataStorage client
)

func TestGatewayIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Integration Test Suite")
}

var _ = SynchronizedBeforeSuite(
	// ============================================================================
	// PHASE 1: Start shared infrastructure (Process 1 ONLY)
	// ============================================================================
	func() []byte {
		logger := zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
		
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Gateway Integration Suite - PHASE 1: Infrastructure Setup")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("[Process 1] Starting shared Podman infrastructure...")
		
		// 1. Preflight checks
		logger.Info("[Process 1] Step 1: Preflight checks")
		err := preflightCheck()
		Expect(err).ToNot(HaveOccurred(), "Preflight checks must pass")
		
		// 2. Create Podman network
		logger.Info("[Process 1] Step 2: Create Podman network")
		err = createPodmanNetwork()
		Expect(err).ToNot(HaveOccurred(), "Podman network creation must succeed")
		
		// 3. Start PostgreSQL
		logger.Info("[Process 1] Step 3: Start PostgreSQL container")
		err = startPostgreSQL()
		Expect(err).ToNot(HaveOccurred(), "PostgreSQL start must succeed")
		
		// 4. Apply migrations to PUBLIC schema
		logger.Info("[Process 1] Step 4: Apply database migrations")
		db, err := connectPostgreSQL()
		Expect(err).ToNot(HaveOccurred(), "PostgreSQL connection must succeed")
		
		err = infrastructure.ApplyMigrationsWithPropagationTo(db)
		Expect(err).ToNot(HaveOccurred(), "Migration application must succeed")
		db.Close()
		
		// 5. Start DataStorage service
		logger.Info("[Process 1] Step 5: Start DataStorage service")
		err = startDataStorageService()
		Expect(err).ToNot(HaveOccurred(), "DataStorage service start must succeed")
		
		logger.Info("✅ Phase 1 complete - Infrastructure ready for all processes")
		return []byte("ready")
	},
	
	// ============================================================================
	// PHASE 2: Connect to infrastructure (ALL processes)
	// ============================================================================
	func(data []byte) {
		processNum := GinkgoParallelProcess()
		logger = zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
		
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info(fmt.Sprintf("Gateway Integration Suite - PHASE 2: Process %d Setup", processNum))
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		
		// Create root context
		ctx, cancel = context.WithCancel(context.Background())
		
		// CRITICAL: Create per-process DataStorage client
		logger.Info(fmt.Sprintf("[Process %d] Creating per-process DataStorage client", processNum))
		mockTransport := testauth.NewMockUserTransport(
			fmt.Sprintf("test-gateway@integration.test-p%d", processNum),
		)
		
		var err error
		dsClient, err = audit.NewOpenAPIClientAdapterWithTransport(
			fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort),
			5*time.Second,
			mockTransport,
		)
		Expect(err).ToNot(HaveOccurred(), "DataStorage client creation must succeed")
		logger.Info(fmt.Sprintf("[Process %d] ✅ DataStorage client created", processNum))
		
		// Create per-process envtest
		logger.Info(fmt.Sprintf("[Process %d] Creating per-process envtest", processNum))
		
		// Set KUBEBUILDER_ASSETS if not already set
		if os.Getenv("KUBEBUILDER_ASSETS") == "" {
			cmd := exec.Command("go", "run", "sigs.k8s.io/controller-runtime/tools/setup-envtest@latest", "use", "-p", "path")
			output, err := cmd.Output()
			if err != nil {
				logger.Error(err, "Failed to get KUBEBUILDER_ASSETS path")
				Expect(err).ToNot(HaveOccurred(), "Should get KUBEBUILDER_ASSETS path from setup-envtest")
			}
			assetsPath := strings.TrimSpace(string(output))
			_ = os.Setenv("KUBEBUILDER_ASSETS", assetsPath)
		}
		
		// Create envtest with CRD auto-installation
		testEnv = &envtest.Environment{
			CRDDirectoryPaths: []string{
				"../../../config/crd/bases", // Relative path from test/integration/gateway/
			},
			ErrorIfCRDPathMissing: true,
		}
		
		k8sConfig, err = testEnv.Start()
		Expect(err).ToNot(HaveOccurred(), "envtest should start successfully")
		Expect(k8sConfig).ToNot(BeNil(), "K8s config should not be nil")
		
		// Disable rate limiting for in-memory K8s API
		k8sConfig.RateLimiter = nil
		k8sConfig.QPS = 1000
		k8sConfig.Burst = 2000
		
		logger.Info(fmt.Sprintf("[Process %d] ✅ envtest started", processNum), "api", k8sConfig.Host)
		
		// Create scheme with RemediationRequest CRD
		scheme := k8sruntime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(remediationv1alpha1.AddToScheme(scheme)).To(Succeed())
		
		// Create K8s client
		k8sClient, err = client.New(k8sConfig, client.Options{Scheme: scheme})
		Expect(err).ToNot(HaveOccurred(), "Failed to create K8s client")
		
		logger.Info(fmt.Sprintf("[Process %d] ✅ K8s client created", processNum))
		logger.Info(fmt.Sprintf("[Process %d] ✅ Suite setup complete", processNum))
	},
)

var _ = SynchronizedAfterSuite(
	// ============================================================================
	// PHASE 1: Per-process cleanup (ALL processes)
	// ============================================================================
	func() {
		processNum := GinkgoParallelProcess()
		logger.Info(fmt.Sprintf("[Process %d] Cleaning up per-process resources...", processNum))
		
		if cancel != nil {
			cancel()
		}
		
		// Stop envtest
		if testEnv != nil {
			err := testEnv.Stop()
			if err != nil {
				logger.Error(err, "Failed to stop envtest")
			}
		}
		
		logger.Info(fmt.Sprintf("[Process %d] ✅ Cleanup complete", processNum))
	},
	
	// ============================================================================
	// PHASE 2: Shared infrastructure cleanup (Process 1 ONLY)
	// ============================================================================
	func() {
		logger := zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Gateway Integration Suite - Infrastructure Cleanup")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		
		cleanupInfrastructure()
		
		logger.Info("✅ Suite complete - All infrastructure cleaned up")
	},
)

// ============================================================================
// INFRASTRUCTURE FUNCTIONS
// ============================================================================

// preflightCheck validates environment before running tests
func preflightCheck() error {
	// Check Podman availability
	cmd := exec.Command("podman", "version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("podman not available: %w", err)
	}
	
	// Check for port conflicts
	ports := []int{gatewayPostgresPort, gatewayDataStoragePort}
	for _, port := range ports {
		cmd := exec.Command("lsof", "-i", fmt.Sprintf(":%d", port))
		if err := cmd.Run(); err == nil {
			return fmt.Errorf("port %d is already in use", port)
		}
	}
	
	return nil
}

// createPodmanNetwork creates a Podman network for containers
func createPodmanNetwork() error {
	// Check if network already exists
	cmd := exec.Command("podman", "network", "exists", "gateway-integration-net")
	if err := cmd.Run(); err == nil {
		return nil // Network already exists
	}
	
	// Create network
	cmd = exec.Command("podman", "network", "create", "gateway-integration-net")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create Podman network: %w: %s", err, output)
	}
	
	return nil
}

// startPostgreSQL starts PostgreSQL container
func startPostgreSQL() error {
	// Remove existing container if any
	_ = exec.Command("podman", "rm", "-f", gatewayPostgresContainer).Run()
	
	cmd := exec.Command("podman", "run", "-d",
		"--name", gatewayPostgresContainer,
		"--network", "gateway-integration-net",
		"-p", fmt.Sprintf("%d:5432", gatewayPostgresPort),
		"-e", fmt.Sprintf("POSTGRES_USER=%s", gatewayPostgresUser),
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", gatewayPostgresPassword),
		"-e", fmt.Sprintf("POSTGRES_DB=%s", gatewayPostgresDB),
		"postgres:16-alpine",
	)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start PostgreSQL: %w: %s", err, output)
	}
	
	// Wait for PostgreSQL to be ready
	time.Sleep(5 * time.Second)
	
	// Verify connection
	for i := 0; i < 30; i++ {
		db, err := connectPostgreSQL()
		if err == nil {
			db.Close()
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	
	return fmt.Errorf("PostgreSQL failed to become ready after 30s")
}

// connectPostgreSQL creates a database connection
func connectPostgreSQL() (*sql.DB, error) {
	connStr := fmt.Sprintf("host=127.0.0.1 port=%d user=%s password=%s dbname=%s sslmode=disable",
		gatewayPostgresPort, gatewayPostgresUser, gatewayPostgresPassword, gatewayPostgresDB)
	
	return sql.Open("postgres", connStr)
}

// startDataStorageService starts DataStorage API container
func startDataStorageService() error {
	// Remove existing container if any
	_ = exec.Command("podman", "rm", "-f", gatewayDataStorageContainer).Run()
	
	// Build DataStorage image if not exists
	cmd := exec.Command("make", "docker-build-datastorage")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to build DataStorage image: %w", err)
	}
	
	cmd = exec.Command("podman", "run", "-d",
		"--name", gatewayDataStorageContainer,
		"--network", "gateway-integration-net",
		"-p", fmt.Sprintf("%d:8080", gatewayDataStoragePort),
		"-e", fmt.Sprintf("POSTGRES_HOST=%s", gatewayPostgresContainer),
		"-e", "POSTGRES_PORT=5432",
		"-e", fmt.Sprintf("POSTGRES_USER=%s", gatewayPostgresUser),
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", gatewayPostgresPassword),
		"-e", fmt.Sprintf("POSTGRES_DB=%s", gatewayPostgresDB),
		"-e", "REDIS_HOST=",  // Gateway doesn't use Redis
		"kubernaut-datastorage:latest",
	)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start DataStorage: %w: %s", err, output)
	}
	
	// Wait for DataStorage to be ready
	time.Sleep(5 * time.Second)
	
	// Verify DataStorage is responding
	for i := 0; i < 30; i++ {
		cmd := exec.Command("curl", "-f", fmt.Sprintf("http://127.0.0.1:%d/health", gatewayDataStoragePort))
		if err := cmd.Run(); err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	
	return fmt.Errorf("DataStorage failed to become ready after 30s")
}

// cleanupInfrastructure removes all Podman containers and networks
func cleanupInfrastructure() {
	// Stop containers
	_ = exec.Command("podman", "rm", "-f", gatewayDataStorageContainer).Run()
	_ = exec.Command("podman", "rm", "-f", gatewayPostgresContainer).Run()
	
	// Remove network
	_ = exec.Command("podman", "network", "rm", "gateway-integration-net").Run()
}

// getKubernetesClient returns the shared K8s client
// This is used by test helpers and Gateway initialization
func getKubernetesClient() client.Client {
	if k8sClient == nil {
		fmt.Fprintf(os.Stderr, "ERROR: K8s client not initialized\n")
		return nil
	}
	return k8sClient
}

// getDataStorageClient returns the per-process DataStorage client
func getDataStorageClient() *audit.OpenAPIClientAdapter {
	if dsClient == nil {
		fmt.Fprintf(os.Stderr, "ERROR: DataStorage client not initialized\n")
		return nil
	}
	return dsClient
}
