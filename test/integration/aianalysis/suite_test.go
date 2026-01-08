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

// Package aianalysis contains integration tests for the AIAnalysis controller.
// These tests verify the complete reconciliation loop with real Kubernetes API.
//
// Business Requirements:
// - BR-AI-001: AI Analysis CRD lifecycle management
// - BR-AI-002: HolmesGPT-API integration
// - BR-AI-003: Rego policy evaluation
//
// Test Strategy: Two integration test categories:
// 1. **Envtest-only tests** (this file): Use mock HAPI for fast controller testing
// 2. **Real service tests** (recovery_integration_test.go): Use real HAPI (auto-started)
//
// Defense-in-Depth (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Mock K8s client + mock HAPI
// - Integration tests (>50%): Real K8s API (envtest) + mock/real HAPI
// - E2E tests (10-15%): Real K8s API (KIND) + real HAPI
//
// Infrastructure (AUTO-STARTED in SynchronizedBeforeSuite):
// - PostgreSQL (port 15438): Persistence layer
// - Redis (port 16384): Caching layer
// - Data Storage API (port 18095): Audit trail
// - HolmesGPT API (port 18120): AI analysis service (MOCK_LLM_MODE=true)
// - All services started via podman-compose programmatically
package aianalysis

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/status"
	"github.com/jordigilh/kubernaut/pkg/audit"
	hgclient "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
	"github.com/jordigilh/kubernaut/pkg/testutil"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

var (
	ctx        context.Context
	cancel     context.CancelFunc
	testEnv    *envtest.Environment
	cfg        *rest.Config
	k8sClient  client.Client
	k8sManager ctrl.Manager
	auditStore audit.AuditStore // Audit store for DD-AUDIT-003

	// Real HAPI client for integration tests (with MOCK_LLM_MODE=true inside HAPI)
	// Per testing strategy: Only LLM is mocked (inside HAPI), all other services are real
	realHGClient *hgclient.HolmesGPTClient

	// Real Rego evaluator for integration tests
	// Per user requirement: "real rego evaluator for all 3 tiers"
	realRegoEvaluator *rego.Evaluator
	regoCtx           context.Context
	regoCancel        context.CancelFunc

	// DD-TEST-002: Unique namespace per test for parallel execution
	testNamespace string
)

func TestAIAnalysisIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AIAnalysis Controller Integration Suite (Envtest)")
}

// SynchronizedBeforeSuite runs ONCE globally before all parallel processes start
// This follows Gateway/Notification pattern for automated infrastructure startup
//
// TIMEOUT NOTE: Infrastructure startup takes ~70-90 seconds locally, but up to 3+ minutes in CI.
// CI environments (GitHub Actions) have slower container startup times, especially HAPI.
// Default Ginkgo timeout (60s) is insufficient, causing INTERRUPTED in parallel mode.
// NodeTimeout(5*time.Minute) ensures sufficient time for complete infrastructure startup in CI.
var _ = SynchronizedBeforeSuite(NodeTimeout(5*time.Minute), func(specCtx SpecContext) []byte {
	// This runs ONCE on process 1 only - creates shared infrastructure
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("AIAnalysis Integration Test Suite - Automated Setup")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("Creating test infrastructure...")
	GinkgoWriter.Println("  • envtest (in-memory K8s API server)")
	GinkgoWriter.Println("  • PostgreSQL + pgvector (port 15438)")
	GinkgoWriter.Println("  • Redis (port 16384)")
	GinkgoWriter.Println("  • Data Storage API (port 18095)")
	GinkgoWriter.Println("  • HolmesGPT API (port 18120, MOCK_LLM_MODE=true)")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	ctx, cancel = context.WithCancel(context.TODO())

	By("Starting AIAnalysis integration infrastructure (podman-compose)")
	// This starts: PostgreSQL, Redis, Immudb, DataStorage, HolmesGPT-API
	// Per DD-TEST-001 v2.2: PostgreSQL=15438, Redis=16384, Immudb=13326, DS=18095
	var err error
	dsInfra, err := infrastructure.StartDSBootstrap(infrastructure.DSBootstrapConfig{
		ServiceName:     "aianalysis",
		PostgresPort:    15438, // DD-TEST-001 v2.2
		RedisPort:       16384, // DD-TEST-001 v2.2
		DataStoragePort: 18095, // DD-TEST-001 v2.2
		MetricsPort:     19095,
		ConfigDir:       "test/integration/aianalysis/config",
	}, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Infrastructure must start successfully")
	GinkgoWriter.Println("✅ All services started and healthy (PostgreSQL, Redis, Immudb, DataStorage)")

	// Clean up infrastructure on exit
	DeferCleanup(func() {
		infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
	})

	By("Registering AIAnalysis CRD scheme")
	err = aianalysisv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By("Bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	By("Creating controller-runtime client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By("Creating required namespaces")
	// Create kubernaut-system namespace for controller
	systemNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kubernaut-system",
		},
	}
	err = k8sClient.Create(ctx, systemNs)
	Expect(err).NotTo(HaveOccurred())

	// Create default namespace for tests
	defaultNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}
	_ = k8sClient.Create(ctx, defaultNs) // May already exist

	GinkgoWriter.Println("✅ Namespaces created: kubernaut-system, default")

	By("Setting up REAL HolmesGPT-API client")
	// Integration tests use REAL HAPI service (http://localhost:18120)
	// HAPI runs with MOCK_LLM_MODE=true to avoid LLM costs (only mock allowed)
	// The HAPI service has deterministic mock responses for special SignalType values:
	// - MOCK_NO_WORKFLOW_FOUND → needs_human_review=true, reason="no_matching_workflows"
	// - MOCK_LOW_CONFIDENCE → needs_human_review=true, reason="low_confidence"
	// - Other signal types → normal successful responses
	//
	// DD-AUTH-006: Integration tests bypass oauth-proxy by injecting X-Auth-Request-User header
	// OAuth-proxy is not running in integration tests (only in E2E/production)
	hapiMockTransport := testutil.NewMockUserTransport("aianalysis-service@integration.test")
	realHGClient, err = hgclient.NewHolmesGPTClientWithTransport(hgclient.Config{
		BaseURL: "http://localhost:18120",
		Timeout: 30 * time.Second,
	}, hapiMockTransport)
	Expect(err).ToNot(HaveOccurred(), "failed to create real HAPI client")

	By("Setting up REAL Rego evaluator with production policies")
	// Per user requirement: "real rego evaluator for all 3 tiers"
	// Integration tests MUST use real Rego evaluator to validate actual policy behavior
	// Per TESTING_GUIDELINES.md: Integration tests validate business logic, not infrastructure
	policyPath := filepath.Join("..", "..", "..", "config", "rego", "aianalysis", "approval.rego")
	realRegoEvaluator = rego.NewEvaluator(rego.Config{
		PolicyPath: policyPath,
	}, ctrl.Log.WithName("rego"))

	// Create context for Rego evaluator lifecycle
	regoCtx, regoCancel = context.WithCancel(context.Background())

	// ADR-050: Startup validation required
	err = realRegoEvaluator.StartHotReload(regoCtx)
	Expect(err).NotTo(HaveOccurred(), "Production policy should load successfully in integration tests")

	GinkgoWriter.Println("✅ Real Rego evaluator initialized with production policy")
	GinkgoWriter.Printf("  • Policy path: %s\n", policyPath)
	GinkgoWriter.Printf("  • Policy hash: %s\n", realRegoEvaluator.GetPolicyHash())

	By("Setting up audit client for flow-based audit integration tests")
	// Per DD-AUDIT-003: AIAnalysis MUST generate audit traces (P0)
	// Per DD-TEST-001: AIAnalysis Data Storage port 18095
	// DD-API-001: Use OpenAPI client adapter (type-safe, contract-validated)
	// DD-AUTH-005: Integration tests use mock user transport (no oauth-proxy)
	GinkgoWriter.Println("📋 Setting up audit store...")
	auditMockTransport := testutil.NewMockUserTransport("test-aianalysis@integration.test")
	dsClient, err := audit.NewOpenAPIClientAdapterWithTransport(
		"http://127.0.0.1:18095", // AIAnalysis integration test DS port (IPv4 explicit for CI)
		5*time.Second,
		auditMockTransport, // ← Mock user header injection (simulates oauth-proxy)
	)
	Expect(err).ToNot(HaveOccurred(), "Failed to create OpenAPI client adapter")

	auditConfig := audit.DefaultConfig()
	auditConfig.FlushInterval = 100 * time.Millisecond // Faster flush for tests
	auditLogger := zap.New(zap.WriteTo(GinkgoWriter))

	auditStore, err = audit.NewBufferedStore(dsClient, auditConfig, "aianalysis", auditLogger)
	Expect(err).ToNot(HaveOccurred(), "Audit store creation must succeed for DD-AUDIT-003")
	GinkgoWriter.Println("✅ Audit store configured")

	// Create audit client for handlers
	auditClient := aiaudit.NewAuditClient(auditStore, auditLogger)

	By("Setting up the controller manager")
	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Use random port to avoid conflicts in parallel tests
		},
	})
	Expect(err).ToNot(HaveOccurred())

	By("Setting up the AIAnalysis controller with handlers")
	// DD-METRICS-001: Create metrics instance for integration testing
	testMetrics := metrics.NewMetrics()

	// Create handlers with REAL HAPI client, metrics, and REAL audit client
	// Per DD-AUDIT-003: AIAnalysis MUST generate audit traces (P0)
	// Integration tests use REAL services (only LLM inside HAPI is mocked for cost)
	investigatingHandler := handlers.NewInvestigatingHandler(realHGClient, ctrl.Log.WithName("investigating-handler"), testMetrics, auditClient)
	analyzingHandler := handlers.NewAnalyzingHandler(realRegoEvaluator, ctrl.Log.WithName("analyzing-handler"), testMetrics, auditClient)

	// Create controller with wired handlers and audit client
	// Per DD-AUDIT-003: Audit client MUST be wired for audit trail compliance
	err = (&aianalysis.AIAnalysisReconciler{
		Metrics:              testMetrics, // DD-METRICS-001: Inject metrics
		Client:               k8sManager.GetClient(),
		Scheme:               k8sManager.GetScheme(),
		Recorder:             k8sManager.GetEventRecorderFor("aianalysis-controller"),
		Log:                  ctrl.Log.WithName("aianalysis-controller"),
		StatusManager:        status.NewManager(k8sManager.GetClient()), // DD-PERF-001: Atomic status updates
		InvestigatingHandler: investigatingHandler,
		AnalyzingHandler:     analyzingHandler,
		AuditClient:          auditClient, // ✅ REAL AUDIT CLIENT WIRED
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	By("Starting the controller manager")
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	By("Waiting for controller manager to be ready")
	// Per TESTING_GUIDELINES.md: Use Eventually(), NEVER time.Sleep()
	Eventually(func() bool {
		// Check if manager's cache is synced (indicates readiness)
		return k8sManager.GetCache().WaitForCacheSync(ctx)
	}, 10*time.Second, 100*time.Millisecond).Should(BeTrue(), "Controller manager cache should sync within 10s")

	// Note: Metrics server uses dynamic port allocation (":0") to prevent conflicts
	// Port discovery is not exposed by controller-runtime Manager interface

	GinkgoWriter.Println("✅ AIAnalysis integration test environment ready!")
	GinkgoWriter.Println("")
	GinkgoWriter.Println("Environment:")
	GinkgoWriter.Println("  • ENVTEST with real Kubernetes API (etcd + kube-apiserver)")
	GinkgoWriter.Println("  • AIAnalysis CRD installed")
	GinkgoWriter.Println("  • AIAnalysis controller with REAL handlers:")
	GinkgoWriter.Println("    - InvestigatingHandler: REAL HolmesGPT-API client")
	GinkgoWriter.Println("    - AnalyzingHandler: REAL Rego evaluator (production policies)")
	GinkgoWriter.Println("  • REAL services (per TESTING_GUIDELINES.md):")
	GinkgoWriter.Println("    - PostgreSQL: localhost:15438")
	GinkgoWriter.Println("    - Redis: localhost:16384")
	GinkgoWriter.Println("    - Data Storage: http://localhost:18095")
	GinkgoWriter.Println("    - HolmesGPT API: http://localhost:18120 (mock LLM only)")
	GinkgoWriter.Println("")

	// Serialize REST config to pass to all processes
	// Each process needs to create its own k8s client
	configBytes, err := json.Marshal(struct {
		Host     string
		CAData   []byte
		CertData []byte
		KeyData  []byte
	}{
		Host:     cfg.Host,
		CAData:   cfg.CAData,
		CertData: cfg.CertData,
		KeyData:  cfg.KeyData,
	})
	Expect(err).NotTo(HaveOccurred())

	return configBytes
}, func(specCtx SpecContext, data []byte) {
	// This runs on ALL parallel processes (including process 1)
	// Each process creates its own k8s client and context
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	// Deserialize REST config from process 1
	var configData struct {
		Host     string
		CAData   []byte
		CertData []byte
		KeyData  []byte
	}
	err := json.Unmarshal(data, &configData)
	Expect(err).NotTo(HaveOccurred())

	// Register AIAnalysis CRD scheme (MUST be done before creating client)
	err = aianalysisv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Create per-process REST config
	cfg = &rest.Config{
		Host: configData.Host,
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   configData.CAData,
			CertData: configData.CertData,
			KeyData:  configData.KeyData,
		},
	}

	// Create per-process k8s client
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Create per-process context ONLY if not already set (process 1 has it from first function)
	// Process 1: ctx already set and used by controller manager - don't overwrite!
	// Processes 2-4: Need ctx for test operations
	if ctx == nil {
		ctx, cancel = context.WithCancel(context.Background())
	}

	// Create per-process REAL HAPI client (each process gets its own client)
	// Integration tests use REAL HAPI service with MOCK_LLM_MODE=true (only mock allowed)
	// DD-AUTH-006: Integration tests bypass oauth-proxy by injecting X-Auth-Request-User header
	processHapiTransport := testutil.NewMockUserTransport("aianalysis-service@integration.test")
	realHGClient, err = hgclient.NewHolmesGPTClientWithTransport(hgclient.Config{
		BaseURL: "http://localhost:18120",
		Timeout: 30 * time.Second,
	}, processHapiTransport)
	if err != nil {
		// Don't fail here - let tests fail if HAPI is not available
		GinkgoWriter.Printf("⚠️ Warning: failed to create real HAPI client: %v\n", err)
	}
})

// SynchronizedAfterSuite ensures proper cleanup in parallel execution
var _ = SynchronizedAfterSuite(func() {
	// This runs on ALL parallel processes - no cleanup needed per process
}, func() {
	// This runs ONCE on the last parallel process - cleanup shared infrastructure
	By("Stopping Rego evaluator")
	if regoCancel != nil {
		regoCancel() // Stop hot-reload goroutine
		GinkgoWriter.Println("✅ Rego evaluator stopped")
	}

	By("Flushing audit store before shutdown")
	// Per DD-007: Graceful shutdown MUST flush all audit events
	// AA-SHUTDOWN-001: Flush audit store BEFORE stopping DataStorage
	// This prevents "connection refused" errors during cleanup when the
	// background writer tries to flush buffered events after DataStorage is stopped.
	// Integration tests MUST always use real DataStorage (DD-TESTING-001)
	GinkgoWriter.Println("📋 Flushing audit store...")

	flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer flushCancel()

	if err := auditStore.Flush(flushCtx); err != nil {
		GinkgoWriter.Printf("⚠️ Warning: failed to flush audit store: %v\n", err)
	} else {
		GinkgoWriter.Println("✅ Audit store flushed (all buffered events written)")
	}

	if err := auditStore.Close(); err != nil {
		GinkgoWriter.Printf("⚠️ Warning: audit store close error: %v\n", err)
	} else {
		GinkgoWriter.Println("✅ Audit store closed")
	}

	By("Tearing down the test environment")
	cancel()

	if testEnv != nil {
		err := testEnv.Stop()
		Expect(err).NotTo(HaveOccurred())
	}

	// Check if containers should be preserved for debugging
	// Set PRESERVE_CONTAINERS=true to keep containers running after tests
	// This is useful for inspecting logs when tests fail
	preserveContainers := os.Getenv("PRESERVE_CONTAINERS") == "true"

	if preserveContainers {
		GinkgoWriter.Println("⚠️  Tests may have failed - preserving containers for debugging")
		GinkgoWriter.Println("📋 To inspect container logs:")
		GinkgoWriter.Println("   podman logs aianalysis_hapi_1")
		GinkgoWriter.Println("   podman logs aianalysis_datastorage_1")
		GinkgoWriter.Println("   podman logs aianalysis_postgres_1")
		GinkgoWriter.Println("   podman logs aianalysis_redis_1")
		GinkgoWriter.Println("📋 To manually clean up:")
		GinkgoWriter.Println("   podman stop aianalysis_hapi_1 aianalysis_datastorage_1 aianalysis_redis_1 aianalysis_postgres_1")
		GinkgoWriter.Println("   podman rm aianalysis_hapi_1 aianalysis_datastorage_1 aianalysis_redis_1 aianalysis_postgres_1")
		GinkgoWriter.Println("   podman network rm aianalysis_test-network")
	} else {
		// Infrastructure cleanup handled by DeferCleanup (StopDSBootstrap)

		By("Cleaning up infrastructure images to prevent disk space issues")
		// Prune ONLY infrastructure images for this service
		// Per DD-TEST-001 v1.1: Use label-based filtering for AIAnalysis integration compose project
		pruneCmd := exec.Command("podman", "image", "prune", "-f",
			"--filter", "label=io.podman.compose.project=aianalysis-integration")
		pruneOutput, pruneErr := pruneCmd.CombinedOutput()
		if pruneErr != nil {
			GinkgoWriter.Printf("⚠️  Failed to prune images: %v\n%s\n", pruneErr, pruneOutput)
		} else {
			GinkgoWriter.Println("✅ Infrastructure images pruned")
		}
	}

	GinkgoWriter.Println("✅ Cleanup complete")
})

// DD-TEST-002 Compliance: Unique namespace per test for parallel execution
// This enables -procs=4 parallel execution (matching Notification pattern)
// Each test gets its own namespace to prevent resource conflicts

var _ = BeforeEach(func() {
	// DD-TEST-002: Create unique namespace per test (enables parallel execution)
	// Format: test-aa-<8-char-uuid> for readability and uniqueness
	testNamespace = "test-aa-" + uuid.New().String()[:8]

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
		},
	}

	err := k8sClient.Create(context.Background(), ns)
	Expect(err).NotTo(HaveOccurred(),
		"Failed to create test namespace %s", testNamespace)

	GinkgoWriter.Printf("📦 [AA] Test namespace created: %s (DD-TEST-002 compliance)\n", testNamespace)
})

var _ = AfterEach(func() {
	// DD-TEST-002: Clean up namespace and ALL resources (instant cleanup)
	// This is MUCH faster than deleting individual AIAnalysis resources
	if testNamespace != "" {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}

		err := k8sClient.Delete(context.Background(), ns)
		if err != nil {
			GinkgoWriter.Printf("⚠️  [AA] Failed to delete namespace %s: %v\n", testNamespace, err)
		} else {
			GinkgoWriter.Printf("🗑️  [AA] Namespace %s deleted (DD-TEST-002 cleanup)\n", testNamespace)
		}
	}
})

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}
