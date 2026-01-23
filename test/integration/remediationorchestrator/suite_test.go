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

// Package remediationorchestrator_test contains integration tests for the RemediationOrchestrator controller.
// These tests use ENVTEST with real Kubernetes API (etcd + kube-apiserver).
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic in isolation (test/unit/remediationorchestrator/)
// - Integration tests (>50%): Infrastructure interaction, microservices coordination (this file)
// - E2E tests (10-15%): Complete workflow validation (test/e2e/remediationorchestrator/)
//
// Test Execution (parallel, 4 procs):
//
//	ginkgo -p --procs=4 ./test/integration/remediationorchestrator/...
//
// MANDATORY: All tests use unique namespaces for parallel execution isolation.
package remediationorchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	// Import ALL CRD types that RO interacts with
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

	// Import RO controller
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/test/infrastructure"

	// Import audit infrastructure (ADR-032)
	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	// Child CRD controllers NOT imported - Phase 1 integration tests use manual control
	// Real controller testing happens in:
	//   - Service integration tests (test/integration/{service}/)
	//   - RO segmented E2E tests (Phase 2 - future)
)

// Test constants for timeout and polling intervals
const (
	timeout  = 120 * time.Second // Increased for CI environment (resource contention can slow reconciliation)
	interval = 250 * time.Millisecond

	// ROIntegrationDataStoragePort is the DataStorage API port for RO integration tests
	// Per DD-TEST-001 v2.2: Each service gets a unique port to enable parallel test execution
	ROIntegrationDataStoragePort = 18140
)

// Package-level variables for test environment
var (
	ctx        context.Context
	cancel     context.CancelFunc
	testEnv    *envtest.Environment
	cfg        *rest.Config
	k8sClient  client.Client
	k8sManager ctrl.Manager
	auditStore audit.AuditStore
	dsClient   *ogenclient.Client // OpenAPI client for DataStorage queries

	// dataStorageBaseURL is the base URL for DataStorage API calls
	// Uses ROIntegrationDataStoragePort to avoid brittle hardcoded ports (DD-TEST-001 v2.2)
	dataStorageBaseURL = fmt.Sprintf("http://127.0.0.1:%d", ROIntegrationDataStoragePort)
)

func TestRemediationOrchestratorIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RemediationOrchestrator Controller Integration Suite (ENVTEST + AIAnalysis Pattern)")
}

// SynchronizedBeforeSuite runs ONCE globally before all parallel processes start
// DD-TEST-010 Pattern: Multi-Controller (per-process controller isolation)
// Phase 1: Infrastructure ONLY (shared across all processes)
// Phase 2: Per-process controller setup (isolated envtest + controller instances)
var _ = SynchronizedBeforeSuite(func() []byte {
	// ======================================================================
	// PHASE 1: INFRASTRUCTURE SETUP (ONCE - GLOBAL)
	// ======================================================================
	// Runs ONCE on process 1 only
	// Starts shared infrastructure (PostgreSQL, Redis, DataStorage)
	// Does NOT start controllers (moved to Phase 2 for per-process isolation)
	//
	// DD-TEST-010: Multi-Controller Pattern for Parallel Test Execution
	// Authority: docs/architecture/decisions/DD-TEST-010-multi-controller-pattern.md
	// ======================================================================

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("PHASE 1: Infrastructure Setup (DD-TEST-010)")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("Starting shared infrastructure...")
	GinkgoWriter.Println("  • PostgreSQL (port 15435)")
	GinkgoWriter.Println("  • Redis (port 16381)")
	GinkgoWriter.Println("  • Data Storage API (port 18140)")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	var err error

	By("Starting RO integration infrastructure (DD-TEST-002)")
	dsInfra, err := infrastructure.StartDSBootstrap(infrastructure.DSBootstrapConfig{
		ServiceName:     "remediationorchestrator",
		PostgresPort:    15435,
		RedisPort:       16381,
		DataStoragePort: ROIntegrationDataStoragePort,
		MetricsPort:     19140,
		ConfigDir:       "test/integration/remediationorchestrator/config",
	}, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Infrastructure must start successfully")
	GinkgoWriter.Println("✅ All external services started and healthy")

	DeferCleanup(func() {
		_ = infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
	})

	GinkgoWriter.Println("✅ Phase 1 complete - infrastructure ready for all processes")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// DD-TEST-010: Phase 1 returns empty - no controller state to share
	return []byte{}
}, func(data []byte) {
	// ======================================================================
	// PHASE 2: PER-PROCESS CONTROLLER SETUP (ISOLATED)
	// ======================================================================
	// Runs on ALL parallel processes (including process 1)
	// Each process gets its own:
	// - envtest (isolated K8s API server)
	// - k8sManager (isolated controller instance)
	// - Controller reconciler (isolated event handlers)
	//
	// DD-TEST-010: Multi-Controller Pattern for Parallel Test Execution
	// Authority: docs/architecture/decisions/DD-TEST-010-multi-controller-pattern.md
	// ======================================================================

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("PHASE 2: Per-Process Controller Setup (DD-TEST-010)")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Create per-process context for controller lifecycle
	ctx, cancel = context.WithCancel(context.Background())

	By("Registering ALL CRD schemes for RO orchestration")

	// RemediationRequest and RemediationApprovalRequest (RO owns these)
	err := remediationv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// SignalProcessing (RO creates these)
	err = signalprocessingv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// AIAnalysis (RO creates these)
	err = aianalysisv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// WorkflowExecution (RO creates these)
	err = workflowexecutionv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// NotificationRequest (RO creates these)
	err = notificationv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By("Bootstrapping per-process envtest with ALL CRDs")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	// KUBEBUILDER_ASSETS is set by Makefile via setup-envtest dependency

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
	if err != nil && !apierrors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}

	// Create default namespace for tests
	defaultNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}
	_ = k8sClient.Create(ctx, defaultNs) // May already exist

	GinkgoWriter.Println("✅ Namespaces created: kubernaut-system, default")

	By("Setting up the controller manager")
	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: ":0", // Dynamic port allocation (prevents conflicts with E2E tests)
		},
	})
	Expect(err).ToNot(HaveOccurred())

	By("Setting up the RemediationOrchestrator controller")
	// Create RO reconciler with manager client, scheme, and audit store
	// Per ADR-032 §1: Audit is MANDATORY for P0 services (RO is P0)
	// Integration tests use real DataStorage API at dataStorageBaseURL (DD-TEST-001 v2.2)
	// DD-API-001: Use OpenAPI client adapter (type-safe, contract-validated)
	// DD-AUTH-005: Integration tests use mock user transport (no oauth-proxy)
	// Note: Using 127.0.0.1 instead of "localhost" to force IPv4
	// (macOS sometimes resolves localhost to ::1 IPv6, which may not be accessible)
	mockTransport := testauth.NewMockUserTransport("test-remediationorchestrator@integration.test")
	dataStorageClient, err := audit.NewOpenAPIClientAdapterWithTransport(
		dataStorageBaseURL,
		5*time.Second,
		mockTransport, // ← Mock user header injection (simulates oauth-proxy)
	)
	Expect(err).ToNot(HaveOccurred(), "Failed to create Data Storage client")

	// Create OpenAPI client for direct DataStorage queries in tests
	// Uses same mock transport for consistency with audit store
	httpClient := &http.Client{Transport: mockTransport, Timeout: 5 * time.Second}
	dsClient, err = ogenclient.NewClient(dataStorageBaseURL, ogenclient.WithClient(httpClient))
	Expect(err).ToNot(HaveOccurred(), "Failed to create OpenAPI DataStorage client for queries")

	auditLogger := ctrl.Log.WithName("audit")
	auditConfig := audit.DefaultConfig()
	auditConfig.FlushInterval = 1 * time.Second // Fast flush for tests (override default)
	auditStore, err = audit.NewBufferedStore(dataStorageClient, auditConfig, "remediation-orchestrator", auditLogger)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create audit store - ensure DataStorage is running at %s", dataStorageBaseURL))

	By("Initializing RemediationOrchestrator metrics (DD-METRICS-001)")
	// Per DD-METRICS-001: Metrics must be initialized and injected for integration tests
	// This enables metrics validation tests (M-INT-1 through M-INT-6)
	roMetrics := rometrics.NewMetrics()
	GinkgoWriter.Println("✅ RO metrics initialized and registered")

	// Create routing engine for blocking logic (BR-ORCH-042)
	// DD-STATUS-001: Pass apiReader for cache-bypassed routing queries
	routingEngine := routing.NewRoutingEngine(
		k8sManager.GetClient(),
		k8sManager.GetAPIReader(), // Cache-bypassed reader for fresh routing decisions
		"", // No namespace filter - all namespaces
		routing.Config{
			ConsecutiveFailureThreshold: 3,
			ConsecutiveFailureCooldown:  3600, // 1 hour
			RecentlyRemediatedCooldown:  300,  // 5 minutes
			ExponentialBackoffBase:      60,   // 1 minute
			ExponentialBackoffMax:       3600, // 1 hour
		},
	)

	reconciler := controller.NewReconciler(
		k8sManager.GetClient(),
		k8sManager.GetAPIReader(), // DD-STATUS-001: API reader for cache-bypassed status refetches
		k8sManager.GetScheme(),
		auditStore,                 // Real audit store for ADR-032 compliance
		nil,                        // No EventRecorder for integration tests
		roMetrics,                  // DD-METRICS-001: Real metrics for integration tests (enables M-INT-1 through M-INT-6)
		controller.TimeoutConfig{}, // Use defaults: Global=1h, Processing=5m, Analyzing=10m, Executing=30m (BR-ORCH-027/028)
		routingEngine,              // Real routing engine for integration tests (BR-ORCH-042)
	)
	err = reconciler.SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	By("Configuring child CRD test strategy")

	// ========================================
	// INTEGRATION TEST STRATEGY = PHASE 1 (Manual Control)
	// ========================================
	// Per RO_E2E_ARCHITECTURE_TRIAGE.md: Integration tests validate RO's
	// orchestration logic in isolation using manual child CRD control.
	//
	// 3-Phase Testing Strategy:
	//   Phase 1 (Integration): RO ONLY, tests manually control child CRDs
	//   Phase 2 (Segmented E2E): RO + ONE real service per segment
	//   Phase 3 (Full E2E): ALL services together
	//
	// This is Phase 1 - no child controllers running.
	// ========================================

	// 1. SignalProcessing Controller - NOT STARTED
	// Tests manually create and update SignalProcessing CRDs
	// Real SP controller testing: test/integration/signalprocessing/ and E2E Segment 2
	GinkgoWriter.Println("ℹ️  SignalProcessing controller NOT started (Phase 1: manual control)")

	// 2. AIAnalysis Controller - NOT STARTED
	// Tests manually create and update AIAnalysis CRDs
	// Real AA controller testing: test/integration/aianalysis/ and E2E Segment 3
	GinkgoWriter.Println("ℹ️  AIAnalysis controller NOT started (Phase 1: manual control)")

	// 3. WorkflowExecution Controller - NOT STARTED
	// Tests manually create and update WorkflowExecution CRDs
	// Real WE controller testing: test/integration/workflowexecution/ and E2E Segment 4
	// Note: WE controller requires Tekton PipelineRun CRDs (not available in envtest)
	GinkgoWriter.Println("ℹ️  WorkflowExecution controller NOT started (Phase 1: manual control)")

	// 4. NotificationRequest Controller - NOT STARTED
	// Tests manually create and update NotificationRequest CRDs
	// Real NR controller testing: test/integration/notification/ and E2E Segment 5
	GinkgoWriter.Println("ℹ️  NotificationRequest controller NOT started (Phase 1: manual control)")

	GinkgoWriter.Println("✅ Phase 1 integration test strategy configured (RO controller only)")

	By("Starting the controller manager")
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	// Wait for manager cache to sync (ensures controllers receive CRD events)
	// Critical: Without cache sync, controllers won't receive Create/Update/Delete events
	// and tests will timeout waiting for status updates (80% confidence this fixes 35+ failures)
	GinkgoWriter.Println("⏳ Waiting for controller manager cache to sync...")

	// Wait for cache to sync with timeout
	// Note: Increased from 10s to 60s after observing SignalProcessing and RemediationApprovalRequest
	// informer sync timeouts on Dec 25, 2025. With 6+ CRDs loading, 10s was insufficient.
	cacheSyncCtx, cacheSyncCancel := context.WithTimeout(ctx, 60*time.Second)
	defer cacheSyncCancel()

	if !k8sManager.GetCache().WaitForCacheSync(cacheSyncCtx) {
		Fail("Failed to sync controller manager cache within 60 seconds")
	}

	// Give controllers additional time to initialize watches and start reconciliation loops
	time.Sleep(2 * time.Second)

	GinkgoWriter.Println("✅ Controller manager cache synced and ready")

	// Note: Metrics server uses dynamic port allocation (":0") to prevent conflicts
	// Port discovery is not exposed by controller-runtime Manager interface
	// Metrics testing should be done at unit test level or via E2E with known ports

	GinkgoWriter.Println("✅ RemediationOrchestrator integration test environment ready!")
	GinkgoWriter.Println("")
	GinkgoWriter.Println("Environment:")
	GinkgoWriter.Println("  • ENVTEST with real Kubernetes API (etcd + kube-apiserver)")
	GinkgoWriter.Println("  • ALL CRDs installed:")
	GinkgoWriter.Println("    - RemediationRequest")
	GinkgoWriter.Println("    - RemediationApprovalRequest")
	GinkgoWriter.Println("    - SignalProcessing")
	GinkgoWriter.Println("    - AIAnalysis")
	GinkgoWriter.Println("    - WorkflowExecution")
	GinkgoWriter.Println("    - NotificationRequest")
	GinkgoWriter.Println("  • ALL Controllers running:")
	GinkgoWriter.Println("    - RemediationOrchestrator (RO)")
	GinkgoWriter.Println("    - SignalProcessing (SP)")
	GinkgoWriter.Println("    - AIAnalysis (AI)")
	GinkgoWriter.Println("    - WorkflowExecution (WE)")
	GinkgoWriter.Println("    - NotificationRequest (NOT)")
	GinkgoWriter.Println("  • REAL services available:")
	GinkgoWriter.Println("    - PostgreSQL: localhost:15435")
	GinkgoWriter.Println("    - Redis: localhost:16381")
	GinkgoWriter.Printf("    - Data Storage: %s\n", dataStorageBaseURL)
	GinkgoWriter.Println("")
	GinkgoWriter.Println("✅ Phase 2 complete - per-process controller ready")
})

// SynchronizedAfterSuite ensures proper cleanup in parallel execution
// DD-TEST-010: Per-process cleanup (Phase 1) + shared infrastructure cleanup (Phase 2)
var _ = SynchronizedAfterSuite(func() {
	// ======================================================================
	// PHASE 1: PER-PROCESS CLEANUP (RUNS ON ALL PROCESSES)
	// ======================================================================
	// Each parallel process cleans up its own controller and envtest instance
	//
	// DD-TEST-010: Multi-Controller Pattern Cleanup
	// ======================================================================
	By("Tearing down per-process test environment")

	// Stop controller manager (graceful shutdown)
	if cancel != nil {
		cancel()
	}

	// Stop per-process envtest
	if testEnv != nil {
		err := testEnv.Stop()
		if err != nil {
			GinkgoWriter.Printf("⚠️  Warning: Failed to stop envtest: %v\n", err)
		} else {
			GinkgoWriter.Println("✅ Per-process envtest stopped")
		}
	}

	// Close per-process audit store
	if auditStore != nil {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer flushCancel()

		_ = auditStore.Flush(flushCtx) // Best effort
		_ = auditStore.Close()         // Best effort
	}
}, func() {
	// ======================================================================
	// PHASE 2: SHARED INFRASTRUCTURE CLEANUP (RUNS ONCE)
	// ======================================================================
	// Runs ONCE on the last parallel process to cleanup shared infrastructure
	//
	// DD-TEST-010: Infrastructure cleanup after all controllers stopped
	// ======================================================================
	By("Cleaning up shared infrastructure")

	// DD-TEST-DIAGNOSTICS: Must-gather container logs for post-mortem analysis
	// ALWAYS collect logs - failures may have occurred on other parallel processes
	// The overhead is minimal (~2s) and logs are invaluable for debugging flaky tests
	GinkgoWriter.Println("📦 Collecting container logs for post-mortem analysis...")
	infrastructure.MustGatherContainerLogs("remediationorchestrator", []string{
		"remediationorchestrator_datastorage_test",
		"remediationorchestrator_postgres_test",
		"remediationorchestrator_redis_test",
	}, GinkgoWriter)

	// Infrastructure cleanup handled by DeferCleanup (StopDSBootstrap)
	// No action needed here - DeferCleanup will stop PostgreSQL, Redis, DataStorage

	By("Cleaning up infrastructure images (DD-TEST-001 v1.1)")
	pruneCmd := exec.Command("podman", "image", "prune", "-f",
		"--filter", "label=io.podman.compose.project=remediationorchestrator-integration")
	pruneOutput, pruneErr := pruneCmd.CombinedOutput()
	if pruneErr != nil {
		GinkgoWriter.Printf("⚠️  Failed to prune images: %v\n%s\n", pruneErr, pruneOutput)
	} else {
		GinkgoWriter.Println("✅ Infrastructure images pruned")
	}

	GinkgoWriter.Println("✅ Cleanup complete - all per-process controllers stopped, shared infrastructure cleaned")
})
// ============================================================================
// TEST HELPER FUNCTIONS (for parallel execution isolation)
// ============================================================================

// createTestNamespace creates a unique namespace for test isolation in parallel execution.
// MANDATORY per 03-testing-strategy.mdc: Each test must use unique identifiers.
// Using UUID for guaranteed uniqueness in parallel execution (12 procs)
func createTestNamespace(prefix string) string {
	ns := fmt.Sprintf("%s-%s", prefix, uuid.New().String()[:13])
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
		},
	}
	Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
	GinkgoWriter.Printf("✅ Created test namespace: %s\n", ns)
	return ns
}

// deleteTestNamespace initiates namespace cleanup without blocking.
// Namespace deletion happens asynchronously - no need to wait since:
// 1. Each test uses unique namespace name (UUID-based)
// 2. Cluster teardown (SynchronizedAfterSuite) handles final cleanup
// 3. No cross-test namespace conflicts due to unique naming
func deleteTestNamespace(ns string) {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
		},
	}
	err := k8sClient.Delete(ctx, namespace)
	if err != nil && !apierrors.IsNotFound(err) {
		GinkgoWriter.Printf("⚠️  Warning: Failed to initiate deletion for namespace %s: %v\n", ns, err)
		return
	}

	GinkgoWriter.Printf("🗑️  Initiated async deletion for namespace: %s (cleanup continues in background)\n", ns)
}
// createRemediationRequest creates a RemediationRequest for testing.
func createRemediationRequest(namespace, name string) *remediationv1.RemediationRequest {
	now := metav1.Now()
	rr := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: remediationv1.RemediationRequestSpec{
			// Valid 64-char hex fingerprint (SHA256 format per CRD validation)
			// UNIQUE per test to avoid routing deduplication (DD-RO-002)
			// Using SHA256(UUID) for guaranteed uniqueness in parallel execution (12 procs)
			SignalFingerprint: func() string {
				h := sha256.Sum256([]byte(uuid.New().String()))
				return hex.EncodeToString(h[:])
			}(),
			SignalName:        "TestHighMemoryAlert",
			Severity:          "critical",
			SignalType:        "prometheus",
			TargetType:        "kubernetes",
			TargetResource: remediationv1.ResourceIdentifier{
				Kind:      "Deployment",
				Name:      "test-app",
				Namespace: namespace,
			},
			FiringTime:   now,
			ReceivedTime: now,
			Deduplication: sharedtypes.DeduplicationInfo{
				FirstOccurrence: now,
				LastOccurrence:  now,
				OccurrenceCount: 1,
			},
		},
	}
	Expect(k8sClient.Create(ctx, rr)).To(Succeed())
	GinkgoWriter.Printf("✅ Created RemediationRequest: %s/%s\n", namespace, name)
	return rr
}
// updateSPStatus updates the SignalProcessing status to simulate completion.
func updateSPStatus(namespace, name string, phase signalprocessingv1.SignalProcessingPhase, severity ...string) error {
	sp := &signalprocessingv1.SignalProcessing{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, sp); err != nil {
		return err
	}

	sp.Status.Phase = phase
	if phase == signalprocessingv1.PhaseCompleted {
		now := metav1.Now()
		sp.Status.CompletionTime = &now
		// DD-SEVERITY-001: Set normalized severity for downstream services (AIAnalysis, RO)
		// This simulates what SignalProcessing Rego policy would do in real environment
		// Default to "critical" if not specified, or use provided severity
		severityValue := "critical" // Default normalized severity
		if len(severity) > 0 && severity[0] != "" {
			severityValue = severity[0] // Use test-specific severity (e.g., "high", "medium", "low")
		}
		sp.Status.Severity = severityValue // Normalized severity (required for AIAnalysis creation)
		// Set environment classification for downstream use
		// Per SP Team Response (2025-12-10): ClassifiedAt is REQUIRED when struct is set
		// V1.1 Note: Confidence field removed per DD-SP-001 V1.1 (redundant with source)
		sp.Status.EnvironmentClassification = &signalprocessingv1.EnvironmentClassification{
			Environment:  "production",
			Source:       "test",
			ClassifiedAt: now, // REQUIRED per SP CRD schema
		}
		// Per SP Team Response (2025-12-10): AssignedAt is REQUIRED when struct is set
		// V1.1 Note: Confidence field removed per DD-SP-001 V1.1 (redundant with source)
		sp.Status.PriorityAssignment = &signalprocessingv1.PriorityAssignment{
			Priority:   "P1",
			Source:     "test",
			AssignedAt: now, // REQUIRED per SP CRD schema
		}
	}

	return k8sClient.Status().Update(ctx, sp)
}
// GenerateTestFingerprint creates a unique 64-character fingerprint for tests.
// This prevents test pollution where multiple tests using the same hardcoded fingerprint
// cause the routing engine to see failures from other tests (BR-ORCH-042, DD-RO-002).
func GenerateTestFingerprint(namespace string, suffix ...string) string {
	input := namespace
	if len(suffix) > 0 {
		input += "-" + suffix[0]
	}
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])[:64]
}
