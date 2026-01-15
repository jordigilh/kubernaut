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
	// PostgreSQL configuration (DataStorage dependency)
	gatewayPostgresPort      = 15437 // Per DD-TEST-001 (was 15439 - HAPI conflict)
	gatewayPostgresUser      = "gateway_test"
	gatewayPostgresPassword  = "gateway_test_password"
	gatewayPostgresDB        = "gateway_test"
	gatewayPostgresContainer = "gateway-integration-postgres"

	// Redis configuration (DataStorage DLQ) - Per DD-TEST-001
	gatewayRedisPort      = 16380
	gatewayRedisContainer = "gateway-integration-redis"
	
	// DataStorage configuration - Per DD-TEST-001
	gatewayDataStoragePort      = 18091 // Per DD-TEST-001 (was 15440 - wrong range)
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

		// Step 1: Cleanup existing containers (shared helper)
		logger.Info("[Process 1] Step 1: Cleanup existing containers")
		infrastructure.CleanupContainers([]string{
			gatewayDataStorageContainer,
			gatewayRedisContainer,
			gatewayPostgresContainer,
		}, GinkgoWriter)

		// Step 2: Create Podman network (idempotent)
		logger.Info("[Process 1] Step 2: Create Podman network")
		_ = exec.Command("podman", "network", "create", "gateway-integration-net").Run()

		// Step 3: Start PostgreSQL (shared helper)
		logger.Info("[Process 1] Step 3: Start PostgreSQL container")
		err := infrastructure.StartPostgreSQL(infrastructure.PostgreSQLConfig{
			ContainerName: gatewayPostgresContainer,
			Port:          gatewayPostgresPort,
			DBName:        gatewayPostgresDB,
			DBUser:        gatewayPostgresUser,
			DBPassword:    gatewayPostgresPassword,
			Network:       "gateway-integration-net",
		}, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "PostgreSQL start must succeed")

		err = infrastructure.WaitForPostgreSQLReady(gatewayPostgresContainer, gatewayPostgresUser, gatewayPostgresDB, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "PostgreSQL must become ready")

		// Step 4: Start Redis (shared helper)
		logger.Info("[Process 1] Step 4: Start Redis container")
		err = infrastructure.StartRedis(infrastructure.RedisConfig{
			ContainerName: gatewayRedisContainer,
			Port:          gatewayRedisPort,
			Network:       "gateway-integration-net",
		}, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Redis start must succeed")

		err = infrastructure.WaitForRedisReady(gatewayRedisContainer, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Redis must become ready")
		
		// Step 5: Apply migrations to PUBLIC schema
		logger.Info("[Process 1] Step 5: Apply database migrations")
		db, err := connectPostgreSQL()
		Expect(err).ToNot(HaveOccurred(), "PostgreSQL connection must succeed")

		err = infrastructure.ApplyMigrationsWithPropagationTo(db)
		Expect(err).ToNot(HaveOccurred(), "Migration application must succeed")
		db.Close()
		
		// Step 6: Start DataStorage (shared helper)
		logger.Info("[Process 1] Step 6: Start DataStorage service")
		imageTag := infrastructure.GenerateInfraImageName("datastorage", "gateway")
		err = infrastructure.StartDataStorage(infrastructure.IntegrationDataStorageConfig{
			ContainerName: gatewayDataStorageContainer,
			Port:          gatewayDataStoragePort,
			Network:       "gateway-integration-net",
			PostgresHost:  gatewayPostgresContainer,
			PostgresPort:  5432,  // Internal container port
			DBName:        gatewayPostgresDB,
			DBUser:        gatewayPostgresUser,
			DBPassword:    gatewayPostgresPassword,
			RedisHost:     gatewayRedisContainer,
			RedisPort:     6379,  // Internal container port
			ImageTag:      imageTag,
		}, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "DataStorage start must succeed")

		err = infrastructure.WaitForHTTPHealth(
			fmt.Sprintf("http://127.0.0.1:%d/health", gatewayDataStoragePort),
			60*time.Second,
			GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "DataStorage must become healthy")

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

		// Collect must-gather logs if tests failed (DD-TEST-DIAGNOSTICS)
		if CurrentSpecReport().Failed() {
			infrastructure.MustGatherContainerLogs("gateway", []string{
				gatewayPostgresContainer,
				gatewayRedisContainer,
				gatewayDataStorageContainer,
			}, GinkgoWriter)
		}

		cleanupInfrastructure()

		logger.Info("✅ Suite complete - All infrastructure cleaned up")
	},
)

// ============================================================================
// INFRASTRUCTURE FUNCTIONS (using shared helpers from test/infrastructure)
// ============================================================================
// Per DD-TEST-002: Sequential Startup Pattern
// Most infrastructure functions are provided by test/infrastructure/shared_integration_utils.go
// Only Gateway-specific functions are defined below

// connectPostgreSQL creates a database connection
// Used by migrations step in SynchronizedBeforeSuite
func connectPostgreSQL() (*sql.DB, error) {
	connStr := fmt.Sprintf("host=127.0.0.1 port=%d user=%s password=%s dbname=%s sslmode=disable",
		gatewayPostgresPort, gatewayPostgresUser, gatewayPostgresPassword, gatewayPostgresDB)
	
	return sql.Open("postgres", connStr)
}

// cleanupInfrastructure removes all Podman containers and networks
// Uses shared helper for standardized cleanup with retries
func cleanupInfrastructure() {
	infrastructure.CleanupContainers([]string{
		gatewayDataStorageContainer,
		gatewayRedisContainer,
		gatewayPostgresContainer,
	}, GinkgoWriter)
	
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
