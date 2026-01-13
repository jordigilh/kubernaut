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
// DD-TEST-010: Multi-Controller Architecture (Controller-Per-Process Pattern)
// Infrastructure (AUTO-STARTED in Phase 1, process 1 only):
// - PostgreSQL (port 15438): Persistence layer
// - Redis (port 16384): Caching layer
// - Data Storage API (port 18095): Audit trail
// - Mock LLM Service (port 18141): Standalone OpenAI-compatible mock (AIAnalysis-specific)
// - HolmesGPT API (port 18120): AI analysis service (uses Mock LLM at 18141)
//
// Per-Process Setup (Phase 2, all processes):
// - envtest: In-memory Kubernetes API server (per process)
// - Controller Manager: AIAnalysis reconciler (per process)
// - Handlers: Investigating, Analyzing (per process)
// - Metrics: Isolated Prometheus registry (per process)
// - Audit Store: Buffered audit client (per process)
package aianalysis

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

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
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// DD-TEST-010: Per-process variables (no shared state between processes)
var (
	ctx        context.Context
	cancel     context.CancelFunc
	testEnv    *envtest.Environment
	cfg        *rest.Config
	k8sClient  client.Client
	k8sManager ctrl.Manager
	auditStore audit.AuditStore

	// Per-process HAPI client (each process gets its own)
	realHGClient *hgclient.HolmesGPTClient

	// Per-process Rego evaluator
	realRegoEvaluator *rego.Evaluator
	regoCtx           context.Context
	regoCancel        context.CancelFunc

	// DD-TEST-002: Unique namespace per test for parallel execution
	testNamespace string

	// DD-METRICS-001: Per-process isolated Prometheus registry
	testRegistry *prometheus.Registry
	testMetrics  *metrics.Metrics

	// DD-TEST-010: Per-process reconciler instance for metrics access
	// WorkflowExecution pattern: Store reconciler to access metrics directly
	reconciler *aianalysis.AIAnalysisReconciler

	// DD-TEST-010: Track infrastructure for cleanup (shared reference)
	dsInfra *infrastructure.DSBootstrapInfra
)

func TestAIAnalysisIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AIAnalysis Controller Integration Suite (Envtest)")
}

// DD-TEST-010: Multi-Controller Architecture
// Phase 1: Infrastructure ONLY (Process 1 ONLY)
// Phase 2: Per-Process Controller Environment (ALL Processes)
//
// TIMEOUT NOTE: Infrastructure startup takes ~70-90 seconds locally, but up to 3+ minutes in CI.
// CI environments (GitHub Actions) have slower container startup times, especially HAPI.
// Default Ginkgo timeout (60s) is insufficient, causing INTERRUPTED in parallel mode.
// NodeTimeout(5*time.Minute) ensures sufficient time for complete infrastructure startup in CI.
var _ = SynchronizedBeforeSuite(NodeTimeout(5*time.Minute), func(specCtx SpecContext) []byte {
	// ═══════════════════════════════════════════════════════════════════════════════
	// Phase 1: Infrastructure ONLY (Process 1 ONLY)
	// ═══════════════════════════════════════════════════════════════════════════════
	// Per DD-TEST-010: Phase 1 starts ONLY shared infrastructure containers
	// NO envtest, NO controller, NO metrics - these are created per-process in Phase 2
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("AIAnalysis Integration - DD-TEST-010 Multi-Controller Pattern")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("Phase 1: Infrastructure Startup (process 1 only)")
	GinkgoWriter.Println("  • PostgreSQL + pgvector (port 15438)")
	GinkgoWriter.Println("  • Redis (port 16384)")
	GinkgoWriter.Println("  • Data Storage API (port 18095)")
	GinkgoWriter.Println("  • Mock LLM Service (port 18141 - AIAnalysis-specific)")
	GinkgoWriter.Println("  • HolmesGPT-API HTTP service (port 18120, uses Mock LLM)")
	GinkgoWriter.Println("")
	GinkgoWriter.Println("Phase 2 will create PER-PROCESS (all processes):")
	GinkgoWriter.Println("  • envtest (in-memory K8s API server)")
	GinkgoWriter.Println("  • Controller manager + AIAnalysis reconciler")
	GinkgoWriter.Println("  • Handlers, metrics, audit store")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	By("Starting AIAnalysis integration infrastructure (PostgreSQL, Redis, DataStorage)")
	// Per DD-TEST-001 v2.2: PostgreSQL=15438, Redis=16384, DS=18095
	var err error
	dsInfra, err = infrastructure.StartDSBootstrap(infrastructure.DSBootstrapConfig{
		ServiceName:     "aianalysis",
		PostgresPort:    15438, // DD-TEST-001 v2.2
		RedisPort:       16384, // DD-TEST-001 v2.2
		DataStoragePort: 18095, // DD-TEST-001 v2.2
		MetricsPort:     19095,
		ConfigDir:       "test/integration/aianalysis/config",
	}, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Infrastructure must start successfully")
	GinkgoWriter.Println("✅ DataStorage infrastructure started (PostgreSQL, Redis, DataStorage)")

	// Clean up infrastructure on exit
	DeferCleanup(func() {
		infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
	})

	By("Building Mock LLM image (DD-TEST-004 unique tag)")
	// Per DD-TEST-004: Generate unique image tag to prevent collisions
	mockLLMImageName, err := infrastructure.BuildMockLLMImage(specCtx, "aianalysis", GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Mock LLM image must build successfully")
	Expect(mockLLMImageName).ToNot(BeEmpty(), "Mock LLM image name must be returned")
	GinkgoWriter.Printf("✅ Mock LLM image built: %s\n", mockLLMImageName)

	By("Starting Mock LLM service (replaces HAPI embedded mock logic)")
	// Per DD-TEST-001 v2.3: Port 18141 (AIAnalysis-specific, unique from HAPI's 18140)
	// Per MOCK_LLM_MIGRATION_PLAN.md v1.3.0: Standalone service for test isolation
	mockLLMConfig := infrastructure.GetMockLLMConfigForAIAnalysis()
	mockLLMConfig.ImageTag = mockLLMImageName // Use the built image tag
	mockLLMContainerID, err := infrastructure.StartMockLLMContainer(specCtx, mockLLMConfig, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Mock LLM container must start successfully")
	Expect(mockLLMContainerID).ToNot(BeEmpty(), "Mock LLM container ID must be returned")
	GinkgoWriter.Printf("✅ Mock LLM service started and healthy (port %d)\n", mockLLMConfig.Port)

	// Clean up Mock LLM on exit
	DeferCleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		if err := infrastructure.StopMockLLMContainer(stopCtx, mockLLMConfig, GinkgoWriter); err != nil {
			GinkgoWriter.Printf("⚠️  Failed to stop Mock LLM container: %v\n", err)
		}
	})

	By("Starting HolmesGPT-API HTTP service (programmatically)")
	// AA integration tests use OpenAPI HAPI client (HTTP-based)
	// DD-TEST-001 v2.3: HAPI port 18120, Mock LLM port 18141
	// MOCK_LLM_MODE removed - now uses standalone Mock LLM service
	projectRoot := filepath.Join("..", "..", "..") // test/integration/aianalysis -> project root
	hapiConfigDir, err := filepath.Abs("hapi-config")
	Expect(err).ToNot(HaveOccurred(), "Failed to get absolute path for hapi-config")
	hapiConfig := infrastructure.GenericContainerConfig{
		Name:    "aianalysis_hapi_test",
		Image:   infrastructure.GenerateInfraImageName("holmesgpt-api", "aianalysis"),
		Network: "aianalysis_test_network",
		Ports:   map[int]int{8080: 18120}, // container:host
		Env: map[string]string{
			"LLM_ENDPOINT":     infrastructure.GetMockLLMEndpoint(mockLLMConfig), // http://127.0.0.1:18141
			"LLM_MODEL":        "mock-model",
			"DATA_STORAGE_URL": "http://aianalysis_datastorage_test:8080", // Container-to-container communication
			"PORT":             "8080",
			"LOG_LEVEL":        "DEBUG",
		},
		Volumes: map[string]string{
			hapiConfigDir: "/etc/holmesgpt:ro", // Mount HAPI config directory
		},
		BuildContext:    projectRoot,
		BuildDockerfile: "holmesgpt-api/Dockerfile.e2e", // E2E Dockerfile: minimal dependencies, no lib64 issues
		HealthCheck: &infrastructure.HealthCheckConfig{
			URL:     "http://127.0.0.1:18120/health",
			Timeout: 300 * time.Second, // HAPI build takes ~100s (Python wheels, dependencies)
		},
	}
	hapiContainer, err := infrastructure.StartGenericContainer(hapiConfig, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "HAPI container must start successfully")
	GinkgoWriter.Printf("✅ HolmesGPT-API started at http://127.0.0.1:18120 (container: %s)\n", hapiContainer.ID)
	GinkgoWriter.Printf("   Using Mock LLM at %s\n", infrastructure.GetMockLLMEndpoint(mockLLMConfig))

	// Clean up HAPI container on exit
	DeferCleanup(func() {
		// Capture HAPI logs before stopping container (for HTTP 500 RCA)
		GinkgoWriter.Println("\n📋 Capturing HAPI container logs before cleanup:")
		logsCmd := exec.Command("podman", "logs", "--tail", "100", hapiContainer.Name)
		logsCmd.Stdout = GinkgoWriter
		logsCmd.Stderr = GinkgoWriter
		_ = logsCmd.Run()
		GinkgoWriter.Println("")

		infrastructure.StopGenericContainer(hapiContainer, GinkgoWriter)
	})

	GinkgoWriter.Println("✅ Infrastructure startup complete (Phase 1)")
	GinkgoWriter.Println("  Phase 2 will now run on ALL processes (per-process controller setup)")
	GinkgoWriter.Println("")

	// DD-TEST-010: Share NOTHING between processes
	// Each process creates its own envtest, controller, handlers, metrics
	return []byte{}
}, func(specCtx SpecContext, data []byte) {
	// ═══════════════════════════════════════════════════════════════════════════════
	// Phase 2: Per-Process Controller Environment (ALL Processes)
	// ═══════════════════════════════════════════════════════════════════════════════
	// Per DD-TEST-010: Each process gets its own controller, envtest, metrics, etc.
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	processNum := GinkgoParallelProcess()
	GinkgoWriter.Printf("━━━ [Process %d] Phase 2: Per-Process Controller Setup ━━━\n", processNum)

	By(fmt.Sprintf("[Process %d] Creating per-process context", processNum))
	ctx, cancel = context.WithCancel(context.Background())

	By(fmt.Sprintf("[Process %d] Registering AIAnalysis CRD scheme", processNum))
	err := aianalysisv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By(fmt.Sprintf("[Process %d] Bootstrapping per-process envtest environment", processNum))
	// DD-TEST-010: Each process gets its OWN Kubernetes API server (envtest)
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

	By(fmt.Sprintf("[Process %d] Creating per-process K8s client", processNum))
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By(fmt.Sprintf("[Process %d] Creating per-process namespaces", processNum))
	// Create kubernaut-system namespace for controller
	systemNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "kubernaut-system"},
	}
	err = k8sClient.Create(ctx, systemNs)
	Expect(err).NotTo(HaveOccurred())

	// Create default namespace for tests
	defaultNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
	}
	_ = k8sClient.Create(ctx, defaultNs) // May already exist

	By(fmt.Sprintf("[Process %d] Setting up per-process controller manager", processNum))
	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Random port per process (no conflicts)
		},
	})
	Expect(err).ToNot(HaveOccurred())

	By(fmt.Sprintf("[Process %d] Creating per-process isolated metrics registry", processNum))
	// DD-METRICS-001: Each process needs isolated Prometheus registry
	testRegistry = prometheus.NewRegistry()
	testMetrics = metrics.NewMetricsWithRegistry(testRegistry)

	By(fmt.Sprintf("[Process %d] Creating per-process audit store", processNum))
	// Each process connects to shared DataStorage (from Phase 1)
	// but maintains its own buffer and client
	auditMockTransport := testauth.NewMockUserTransport(
		fmt.Sprintf("test-aianalysis@integration.test-p%d", processNum),
	)
	dsClient, err := audit.NewOpenAPIClientAdapterWithTransport(
		"http://127.0.0.1:18095", // AIAnalysis integration test DS port (IPv4 explicit for CI)
		5*time.Second,
		auditMockTransport,
	)
	Expect(err).ToNot(HaveOccurred(), "Failed to create OpenAPI client adapter")

	auditConfig := audit.DefaultConfig()
	auditConfig.FlushInterval = 100 * time.Millisecond // Faster flush for tests
	auditLogger := zap.New(zap.WriteTo(GinkgoWriter))

	auditStore, err = audit.NewBufferedStore(dsClient, auditConfig, "aianalysis", auditLogger)
	Expect(err).ToNot(HaveOccurred(), "Audit store creation must succeed for DD-AUDIT-003")

	// Create audit client for handlers
	auditClient := aiaudit.NewAuditClient(auditStore, auditLogger)

	By(fmt.Sprintf("[Process %d] Setting up per-process HAPI client", processNum))
	// Each process gets its own HAPI HTTP client (connects to shared HAPI service)
	hapiMockTransport := testauth.NewMockUserTransport(
		fmt.Sprintf("aianalysis-service@integration.test-p%d", processNum),
	)
	realHGClient, err = hgclient.NewHolmesGPTClientWithTransport(hgclient.Config{
		BaseURL: "http://localhost:18120",
		Timeout: 30 * time.Second,
	}, hapiMockTransport)
	Expect(err).ToNot(HaveOccurred(), "failed to create real HAPI client")

	By(fmt.Sprintf("[Process %d] Setting up per-process Rego evaluator", processNum))
	// Per user requirement: "real rego evaluator for all 3 tiers"
	policyPath := filepath.Join("..", "..", "..", "config", "rego", "aianalysis", "approval.rego")
	realRegoEvaluator = rego.NewEvaluator(rego.Config{
		PolicyPath: policyPath,
	}, ctrl.Log.WithName("rego"))

	// Create context for Rego evaluator lifecycle
	regoCtx, regoCancel = context.WithCancel(context.Background())

	// ADR-050: Startup validation required
	err = realRegoEvaluator.StartHotReload(regoCtx)
	Expect(err).NotTo(HaveOccurred(), "Production policy should load successfully in integration tests")

	By(fmt.Sprintf("[Process %d] Setting up per-process controller with handlers", processNum))
	// Create handlers with REAL HAPI client, metrics, and REAL audit client
	investigatingHandler := handlers.NewInvestigatingHandler(realHGClient, ctrl.Log.WithName("investigating-handler"), testMetrics, auditClient)
	analyzingHandler := handlers.NewAnalyzingHandler(realRegoEvaluator, ctrl.Log.WithName("analyzing-handler"), testMetrics, auditClient)

	// Create per-process controller instance and STORE IT (WorkflowExecution pattern)
	// Storing reconciler enables tests to access metrics via reconciler.Metrics
	reconciler = &aianalysis.AIAnalysisReconciler{
		Metrics:              testMetrics, // DD-METRICS-001: Per-process metrics
		Client:               k8sManager.GetClient(),
		Scheme:               k8sManager.GetScheme(),
		Recorder:             k8sManager.GetEventRecorderFor("aianalysis-controller"),
		Log:                  ctrl.Log.WithName("aianalysis-controller"),
		StatusManager:        status.NewManager(k8sManager.GetClient(), k8sManager.GetAPIReader()), // DD-PERF-001 + AA-HAPI-001: Cache-bypassed refetch
		InvestigatingHandler: investigatingHandler,
		AnalyzingHandler:     analyzingHandler,
		AuditClient:          auditClient,
	}
	err = reconciler.SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	By(fmt.Sprintf("[Process %d] Starting per-process controller manager", processNum))
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	By(fmt.Sprintf("[Process %d] Waiting for per-process controller manager to be ready", processNum))
	Eventually(func() bool {
		return k8sManager.GetCache().WaitForCacheSync(ctx)
	}, 10*time.Second, 100*time.Millisecond).Should(BeTrue(), "Controller manager cache should sync within 10s")

	GinkgoWriter.Printf("✅ [Process %d] Controller ready\n", processNum)
	GinkgoWriter.Printf("  • envtest: %s\n", cfg.Host)
	GinkgoWriter.Printf("  • Controller: AIAnalysisReconciler\n")
	GinkgoWriter.Printf("  • Metrics: Isolated registry (per-process)\n")
	GinkgoWriter.Printf("  • Audit: Buffered store → DataStorage\n")
	GinkgoWriter.Println("")
})

// SynchronizedAfterSuite ensures proper cleanup in parallel execution
var _ = SynchronizedAfterSuite(func() {
	// This runs on ALL parallel processes - cleanup per-process resources
	processNum := GinkgoParallelProcess()
	GinkgoWriter.Printf("[Process %d] Cleaning up per-process resources...\n", processNum)

	By(fmt.Sprintf("[Process %d] Stopping Rego evaluator", processNum))
	if regoCancel != nil {
		regoCancel() // Stop hot-reload goroutine
	}

	By(fmt.Sprintf("[Process %d] Flushing audit store", processNum))
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer flushCancel()

	if auditStore != nil {
		if err := auditStore.Flush(flushCtx); err != nil {
			GinkgoWriter.Printf("⚠️  [Process %d] Failed to flush audit store: %v\n", processNum, err)
		}
		if err := auditStore.Close(); err != nil {
			GinkgoWriter.Printf("⚠️  [Process %d] Audit store close error: %v\n", processNum, err)
		}
	}

	By(fmt.Sprintf("[Process %d] Stopping controller manager", processNum))
	if cancel != nil {
		cancel()
	}

	By(fmt.Sprintf("[Process %d] Tearing down envtest environment", processNum))
	if testEnv != nil {
		err := testEnv.Stop()
		if err != nil {
			GinkgoWriter.Printf("⚠️  [Process %d] Failed to stop envtest: %v\n", processNum, err)
		}
	}

	GinkgoWriter.Printf("✅ [Process %d] Per-process cleanup complete\n", processNum)
}, func() {
	// This runs ONCE on the last parallel process - cleanup shared infrastructure
	GinkgoWriter.Println("━━━ [Last Process] Cleaning up shared infrastructure ━━━")

	// Check if containers should be preserved for debugging
	preserveContainers := os.Getenv("PRESERVE_CONTAINERS") == "true"

	if preserveContainers {
		GinkgoWriter.Println("⚠️  Tests may have failed - preserving containers for debugging")
		GinkgoWriter.Println("📋 To inspect container logs:")
		GinkgoWriter.Println("   podman logs aianalysis_hapi_test")
		GinkgoWriter.Println("   podman logs aianalysis_datastorage_test")
		GinkgoWriter.Println("   podman logs aianalysis_postgres_test")
		GinkgoWriter.Println("   podman logs aianalysis_redis_test")
		GinkgoWriter.Println("📋 To manually clean up:")
		GinkgoWriter.Println("   podman stop aianalysis_hapi_test aianalysis_datastorage_test aianalysis_redis_test aianalysis_postgres_test")
		GinkgoWriter.Println("   podman rm aianalysis_hapi_test aianalysis_datastorage_test aianalysis_redis_test aianalysis_postgres_test")
		GinkgoWriter.Println("   podman network rm aianalysis_test_network")
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

	GinkgoWriter.Println("✅ Shared infrastructure cleanup complete")
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
