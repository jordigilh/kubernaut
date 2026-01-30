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
	"os"
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
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
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
	// Port Configuration - Per DD-TEST-001: Port Allocation Strategy
	gatewayPostgresPort     = 15437 // PostgreSQL port
	gatewayRedisPort        = 16380 // Redis port
	gatewayDataStoragePort  = 18091 // DataStorage HTTP API port
	gatewayMetricsPort      = 19091 // DataStorage metrics port
)

var (
	// Shared infrastructure (Phase 1 - Process 1 only)
	dsInfra *infrastructure.DSBootstrapInfra

	// Per-process resources (Phase 2 - All processes)
	ctx              context.Context
	cancel           context.CancelFunc
	k8sClient        client.Client
	logger           logr.Logger
	testEnv          *envtest.Environment
	k8sConfig        *rest.Config
	dsClient         audit.DataStorageClient // Per-process DataStorage client
	sharedAuditStore audit.AuditStore        // Shared audit store (background flusher runs continuously)
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
		
	// DD-AUTH-014: Create envtest FIRST (DataStorage middleware needs kubeconfig)
	logger.Info("[Process 1] Creating envtest for DataStorage authentication...")
	sharedTestEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			"../../../config/crd/bases",
		},
		ErrorIfCRDPathMissing: true,
	}
	
	sharedK8sConfig, err := sharedTestEnv.Start()
	Expect(err).ToNot(HaveOccurred(), "envtest should start successfully")
	Expect(sharedK8sConfig).ToNot(BeNil(), "K8s config should not be nil")
	logger.Info("[Process 1] ✅ envtest started", "api", sharedK8sConfig.Host)
	
	// Write kubeconfig to temporary file for DataStorage container
	kubeconfigPath, err := infrastructure.WriteEnvtestKubeconfigToFile(sharedK8sConfig, "gateway-integration")
	Expect(err).ToNot(HaveOccurred(), "Failed to write envtest kubeconfig")
	logger.Info("[Process 1] ✅ envtest kubeconfig written", "path", kubeconfigPath)
	
	// DD-AUTH-014: Create ServiceAccount with DataStorage access for integration tests
	// This replaces MockUserTransport with real Bearer token authentication
	logger.Info("[Process 1] Creating ServiceAccount for DataStorage authentication...")
	authConfig, err := infrastructure.CreateIntegrationServiceAccountWithDataStorageAccess(
		sharedK8sConfig,
		"gateway-integration-sa",
		"default",
		GinkgoWriter,
	)
	Expect(err).ToNot(HaveOccurred(), "Failed to create ServiceAccount")
	logger.Info("[Process 1] ✅ ServiceAccount created with Bearer token")

		logger.Info("[Process 1] Starting DataStorage infrastructure (PostgreSQL, Redis, DataStorage)...")

		// Use unified infrastructure bootstrap (per DD-TEST-002)
		// This handles: PostgreSQL, Redis, Migrations, DataStorage
		// DD-AUTH-014: Helper function ensures auth is properly configured
		cfg := infrastructure.NewDSBootstrapConfigWithAuth(
			"gateway",
			gatewayPostgresPort, gatewayRedisPort, gatewayDataStoragePort, gatewayMetricsPort,
			"test/integration/gateway/config",
			authConfig,
		)
		dsInfra, err = infrastructure.StartDSBootstrap(cfg, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Infrastructure must start successfully")

	// Store shared envtest for cleanup
	dsInfra.SharedTestEnv = sharedTestEnv

	logger.Info("✅ Phase 1 complete - DataStorage infrastructure ready for all processes")
	// Pass ServiceAccount token to all processes for authenticated DataStorage client
	// Note: DataStorage health check now includes auth readiness (DD-AUTH-014)
	// StartDSBootstrap waits for /health to return 200, which includes auth middleware validation
	return []byte(authConfig.Token)
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

		// DD-AUTH-014: Create authenticated DataStorage client using ServiceAccount token
		// This replaces MockUserTransport (X-Auth-Request-User) with real Bearer token auth
		logger.Info(fmt.Sprintf("[Process %d] Creating authenticated DataStorage client", processNum))
		
		// Extract ServiceAccount token from Phase 1
		saToken := string(data)
		authTransport := testauth.NewServiceAccountTransport(saToken)

		var err error
		dsClient, err = audit.NewOpenAPIClientAdapterWithTransport(
			fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort),
			5*time.Second,
			authTransport, // ✅ Bearer token authentication (DD-AUTH-014)
		)
		Expect(err).ToNot(HaveOccurred(), "DataStorage client creation must succeed")
		logger.Info(fmt.Sprintf("[Process %d] ✅ Authenticated DataStorage client created", processNum))

		// Create SHARED audit store (used by all Gateway servers in this process)
		// DD-AUDIT-003: ONE audit store per process, background flusher runs continuously
		// This prevents Gateway's per-test server creation from losing buffered events
		logger.Info(fmt.Sprintf("[Process %d] Creating shared audit store", processNum))
		auditConfig := audit.RecommendedConfig("gateway-test")
		sharedAuditStore, err = audit.NewBufferedStore(dsClient, auditConfig, "gateway-test", logger)
		Expect(err).ToNot(HaveOccurred(), "Shared audit store creation must succeed")
		logger.Info(fmt.Sprintf("[Process %d] ✅ Shared audit store created (continuous background flusher)", processNum))

		// Create per-process envtest
		logger.Info(fmt.Sprintf("[Process %d] Creating per-process envtest", processNum))

		// KUBEBUILDER_ASSETS is set by Makefile via setup-envtest dependency
		Expect(os.Getenv("KUBEBUILDER_ASSETS")).ToNot(BeEmpty(), "KUBEBUILDER_ASSETS must be set by Makefile (test-integration-% → setup-envtest)")

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

		// DD-TEST-DIAGNOSTICS: Must-gather container logs for post-mortem analysis
		// ALWAYS collect logs - failures may have occurred on other parallel processes
		// The overhead is minimal (~2s) and logs are invaluable for debugging flaky tests
		if dsInfra != nil {
			GinkgoWriter.Println("📦 Collecting container logs for post-mortem analysis...")
			infrastructure.MustGatherContainerLogs("gateway", []string{
				dsInfra.DataStorageContainer,
				dsInfra.PostgresContainer,
				dsInfra.RedisContainer,
			}, GinkgoWriter)
		}

	// Use unified cleanup (same pattern as AIAnalysis/SignalProcessing)
	if dsInfra != nil {
		_ = infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
	}

		logger.Info("✅ Suite complete - All infrastructure cleaned up")
	},
)

// ============================================================================
// TEST HELPER FUNCTIONS
// ============================================================================

// getKubernetesClient returns the shared K8s client
// This is used by test helpers and Gateway initialization
func getKubernetesClient() client.Client {
	if k8sClient == nil {
		fmt.Fprintf(os.Stderr, "ERROR: K8s client not initialized\n")
		return nil
	}
	return k8sClient
}
