/*
Copyright 2026 Jordi Gil.

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

// Package effectivenessmonitor contains integration tests for the EffectivenessMonitor controller.
// These tests use ENVTEST with real Kubernetes API (etcd + kube-apiserver).
//
// Defense-in-Depth Strategy (per TESTING_GUIDELINES.md):
// - Unit tests (>=80% unit-testable): Business logic in isolation (test/unit/effectivenessmonitor/)
// - Integration tests (>=80% integration-testable): Infrastructure interaction (this file)
// - E2E tests (>=80% E2E): Complete workflow validation (test/e2e/)
//
// External Dependencies:
// - DataStorage: Real PostgreSQL + Redis + DataStorage container (DD-TEST-001)
// - Prometheus: httptest.NewServer mock (per TESTING_GUIDELINES.md Section 4a)
// - AlertManager: httptest.NewServer mock (per TESTING_GUIDELINES.md Section 4a)
//
// Test Execution (parallel, 4 procs):
//
//	ginkgo -p --procs=4 ./test/integration/effectivenessmonitor/...
//
// MANDATORY: All tests use unique namespaces for parallel execution isolation.
package effectivenessmonitor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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

	// Import CRD types that EM interacts with
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"

	// Import EM controller and client
	controller "github.com/jordigilh/kubernaut/internal/controller/effectivenessmonitor"
	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
	"github.com/jordigilh/kubernaut/test/infrastructure"

	// Import audit infrastructure (ADR-032)
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
	"github.com/jordigilh/kubernaut/test/shared/integration"
)

// Test constants for timeout and polling intervals
const (
	timeout  = 120 * time.Second
	interval = 250 * time.Millisecond

	// EMIntegrationDataStoragePort is the DataStorage API port for EM integration tests
	// Per DD-TEST-001 v2.8: Each service gets a unique port to enable parallel test execution
	EMIntegrationDataStoragePort = 18092
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

	// DD-AUTH-014: Authenticated DataStorage clients (audit + OpenAPI with ServiceAccount tokens)
	dsClients *integration.AuthenticatedDataStorageClients

	// dataStorageBaseURL is the base URL for DataStorage API calls
	dataStorageBaseURL = fmt.Sprintf("http://127.0.0.1:%d", EMIntegrationDataStoragePort)

	// httptest mocks for external services (per-process, ephemeral ports)
	mockProm *infrastructure.MockPrometheus
	mockAM   *infrastructure.MockAlertManager

	// EM controller metrics (per-process)
	controllerMetrics *emmetrics.Metrics
)

func TestEffectivenessMonitorIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EffectivenessMonitor Controller Integration Suite (ENVTEST + httptest Mocks)")
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
	// ======================================================================

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("PHASE 1: Infrastructure Setup (DD-TEST-010 + DD-AUTH-014)")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("Starting shared infrastructure...")
	GinkgoWriter.Println("  • Shared envtest (for DataStorage auth)")
	GinkgoWriter.Println("  • PostgreSQL (port 15434)")
	GinkgoWriter.Println("  • Redis (port 16383)")
	GinkgoWriter.Printf("  • Data Storage API (port %d)\n", EMIntegrationDataStoragePort)
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	var err error

	// DD-AUTH-014: Start shared envtest for DataStorage auth
	By("Starting shared envtest for DataStorage authentication (DD-AUTH-014)")
	_ = os.Setenv("KUBEBUILDER_CONTROLPLANE_START_TIMEOUT", "60s")

	sharedTestEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		ControlPlane: envtest.ControlPlane{
			APIServer: &envtest.APIServer{
				SecureServing: envtest.SecureServing{
					ListenAddr: envtest.ListenAddr{
						Address: "127.0.0.1", // Force IPv4 binding
					},
				},
			},
		},
	}
	sharedCfg, err := sharedTestEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(sharedCfg).NotTo(BeNil())

	GinkgoWriter.Printf("✅ Shared envtest started at %s\n", sharedCfg.Host)

	DeferCleanup(func() {
		By("Stopping shared envtest")
		err := sharedTestEnv.Stop()
		Expect(err).NotTo(HaveOccurred())
	})

	// DD-AUTH-014: Create ServiceAccount + RBAC in shared envtest for DataStorage auth
	By("Creating ServiceAccount with DataStorage RBAC in shared envtest")
	authConfig, err := infrastructure.CreateIntegrationServiceAccountWithDataStorageAccess(
		sharedCfg,
		"effectivenessmonitor-ds-client",
		"default",
		GinkgoWriter,
	)
	Expect(err).ToNot(HaveOccurred())
	GinkgoWriter.Println("✅ ServiceAccount + RBAC created in shared envtest")

	By("Starting EM integration infrastructure (DD-TEST-002)")
	dsCfg := infrastructure.NewDSBootstrapConfigWithAuth(
		"effectivenessmonitor",
		15434, 16383, EMIntegrationDataStoragePort, 19092,
		"test/integration/effectivenessmonitor/config",
		authConfig,
	)
	dsInfra, err := infrastructure.StartDSBootstrap(dsCfg, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Infrastructure must start successfully")
	GinkgoWriter.Println("✅ All external services started and healthy")

	DeferCleanup(func() {
		_ = infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
	})

	GinkgoWriter.Println("✅ Phase 1 complete - infrastructure ready for all processes")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// DD-AUTH-014: Serialize token for Phase 2 DataStorage client
	return []byte(authConfig.Token)
}, func(data []byte) {
	// ======================================================================
	// PHASE 2: PER-PROCESS CONTROLLER SETUP (ISOLATED)
	// ======================================================================
	// Runs on ALL parallel processes (including process 1)
	// Each process gets its own:
	// - envtest (isolated K8s API server for controller)
	// - k8sManager (isolated controller instance)
	// - httptest mocks (Prometheus + AlertManager, ephemeral ports)
	// - Controller reconciler (isolated event handlers)
	// ======================================================================

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("PHASE 2: Per-Process Controller Setup (DD-TEST-010)")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// DD-AUTH-014: Deserialize ServiceAccount token from Phase 1
	token := string(data)
	GinkgoWriter.Printf("DD-AUTH-014: ServiceAccount token received (%d bytes)\n", len(token))

	// Create per-process context for controller lifecycle
	ctx, cancel = context.WithCancel(context.Background())

	By("Registering CRD schemes for EM")
	// EffectivenessAssessment (EM watches these)
	err := eav1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// RemediationRequest (needed for ownerRef GC and correlation)
	err = remediationv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By("Bootstrapping per-process envtest with ALL CRDs")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	By("Creating controller-runtime client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By("Creating required namespaces")
	for _, nsName := range []string{"kubernaut-system", "default"} {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: nsName},
		}
		err = k8sClient.Create(ctx, ns)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
	}
	GinkgoWriter.Println("✅ Namespaces created: kubernaut-system, default")

	// ========================================
	// HTTPTEST MOCKS: Prometheus + AlertManager
	// Per TESTING_GUIDELINES.md Section 4a: httptest.NewServer for INT tier
	// Each process gets its own in-process mock (ephemeral port, no collision)
	// ========================================
	By("Starting httptest mocks for Prometheus and AlertManager")
	// Configure mock Prometheus with a default query response containing metric
	// samples so the reconciler's assessMetrics() can complete (empty results would
	// cause infinite requeue as "no data available yet").
	now := float64(time.Now().Unix())
	preRemediationTime := now - 60 // 60 seconds ago (pre-remediation)
	mockProm = infrastructure.NewMockPrometheus(infrastructure.MockPrometheusConfig{
		Ready:   true,
		Healthy: true,
		QueryResponse: infrastructure.NewPromVectorResponse(
			map[string]string{"__name__": "container_cpu_usage_seconds_total", "namespace": "default"},
			0.25, // 25% CPU -- baseline metric for assessment
			now,
		),
		// QueryRangeResponse is used by the reconciler's assessMetrics() via QueryRange().
		// It requires >= 2 samples (pre- and post-remediation) to compute a metric score.
		// Pre-remediation: 0.50 (50% CPU), Post-remediation: 0.25 (25% CPU) → improvement.
		QueryRangeResponse: infrastructure.NewPromMatrixResponse(
			map[string]string{"__name__": "container_cpu_usage_seconds_total", "namespace": "default"},
			[][]interface{}{
				{preRemediationTime, "0.500000"}, // pre-remediation: 50% CPU
				{now, "0.250000"},                 // post-remediation: 25% CPU (improvement)
			},
		),
	})
	GinkgoWriter.Printf("✅ Mock Prometheus started at %s (with default metric data)\n", mockProm.URL())

	// Configure mock AlertManager with no active alerts (all resolved),
	// which maps to alert_score = 1.0 in the alert scorer.
	mockAM = infrastructure.NewMockAlertManager(infrastructure.MockAlertManagerConfig{
		Ready:   true,
		Healthy: true,
	})
	GinkgoWriter.Printf("✅ Mock AlertManager started at %s\n", mockAM.URL())

	DeferCleanup(func() {
		mockProm.Close()
		mockAM.Close()
	})

	By("Setting up the controller manager")
	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: ":0", // Dynamic port allocation
		},
	})
	Expect(err).ToNot(HaveOccurred())

	// ========================================
	// AUDIT STORE INITIALIZATION
	// ========================================
	By("Setting up authenticated DataStorage clients (DD-AUTH-014)")
	dsClients = integration.NewAuthenticatedDataStorageClients(
		dataStorageBaseURL,
		token,
		5*time.Second,
	)
	GinkgoWriter.Printf("✅ Authenticated DataStorage clients created (token: %d bytes)\n", len(token))

	auditLogger := ctrl.Log.WithName("audit")
	auditConfig := audit.DefaultConfig()
	auditConfig.FlushInterval = 1 * time.Second // Fast flush for tests
	auditStore, err = audit.NewBufferedStore(
		dsClients.AuditClient,
		auditConfig,
		"effectiveness-monitor",
		auditLogger,
	)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create audit store - ensure DataStorage is running at %s", dataStorageBaseURL))

	// ========================================
	// EM CONTROLLER SETUP
	// ========================================
	By("Initializing EffectivenessMonitor metrics (DD-METRICS-001)")
	controllerMetrics = emmetrics.NewMetrics()
	GinkgoWriter.Println("✅ EM metrics initialized and registered")

	By("Wiring real HTTP clients to httptest mocks (Prometheus + AlertManager)")
	promClient := emclient.NewPrometheusHTTPClient(mockProm.URL(), 5*time.Second)
	amClient := emclient.NewAlertManagerHTTPClient(mockAM.URL(), 5*time.Second)
	GinkgoWriter.Printf("✅ Prometheus HTTP client → %s\n", mockProm.URL())
	GinkgoWriter.Printf("✅ AlertManager HTTP client → %s\n", mockAM.URL())

	reconciler := controller.NewReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		k8sManager.GetEventRecorderFor("effectivenessmonitor-controller"),
		controllerMetrics,
		promClient,
		amClient,
		nil, // AuditManager: nil for integration tests (audit tested in E2E)
		func() controller.ReconcilerConfig {
			c := controller.DefaultReconcilerConfig()
			c.PrometheusEnabled = true
			c.AlertManagerEnabled = true
			return c
		}(),
	)
	err = reconciler.SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	By("Starting the controller manager")
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	// Wait for manager cache to sync
	GinkgoWriter.Println("⏳ Waiting for controller manager cache to sync...")
	cacheSyncCtx, cacheSyncCancel := context.WithTimeout(ctx, 60*time.Second)
	defer cacheSyncCancel()

	if !k8sManager.GetCache().WaitForCacheSync(cacheSyncCtx) {
		Fail("Failed to sync controller manager cache within 60 seconds")
	}

	time.Sleep(2 * time.Second)
	GinkgoWriter.Println("✅ Controller manager cache synced and ready")

	GinkgoWriter.Println("✅ EffectivenessMonitor integration test environment ready!")
	GinkgoWriter.Println("")
	GinkgoWriter.Println("Environment:")
	GinkgoWriter.Println("  • ENVTEST with real Kubernetes API (etcd + kube-apiserver)")
	GinkgoWriter.Println("  • CRDs installed: EffectivenessAssessment, RemediationRequest")
	GinkgoWriter.Println("  • EM Controller running")
	GinkgoWriter.Printf("  • Mock Prometheus: %s\n", mockProm.URL())
	GinkgoWriter.Printf("  • Mock AlertManager: %s\n", mockAM.URL())
	GinkgoWriter.Println("  • REAL services available:")
	GinkgoWriter.Println("    - PostgreSQL: localhost:15434")
	GinkgoWriter.Println("    - Redis: localhost:16383")
	GinkgoWriter.Printf("    - Data Storage: %s\n", dataStorageBaseURL)
	GinkgoWriter.Println("")
	GinkgoWriter.Println("✅ Phase 2 complete - per-process controller ready")
})

// SynchronizedAfterSuite ensures proper cleanup in parallel execution
var _ = SynchronizedAfterSuite(func() {
	// ======================================================================
	// PHASE 1: PER-PROCESS CLEANUP (RUNS ON ALL PROCESSES)
	// ======================================================================
	By("Tearing down per-process test environment")

	// Stop controller manager
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

		_ = auditStore.Flush(flushCtx)
		_ = auditStore.Close()
	}
}, func() {
	// ======================================================================
	// PHASE 2: SHARED INFRASTRUCTURE CLEANUP (RUNS ONCE)
	// ======================================================================
	By("Cleaning up shared infrastructure")

	GinkgoWriter.Println("📦 Collecting container logs for post-mortem analysis...")
	infrastructure.MustGatherContainerLogs("effectivenessmonitor", []string{
		"effectivenessmonitor_datastorage_test",
		"effectivenessmonitor_postgres_test",
		"effectivenessmonitor_redis_test",
	}, GinkgoWriter)

	By("Cleaning up infrastructure images (DD-TEST-001 v1.1)")
	pruneCmd := exec.Command("podman", "image", "prune", "-f",
		"--filter", "label=io.podman.compose.project=effectivenessmonitor-integration")
	pruneOutput, pruneErr := pruneCmd.CombinedOutput()
	if pruneErr != nil {
		GinkgoWriter.Printf("⚠️  Failed to prune images: %v\n%s\n", pruneErr, pruneOutput)
	} else {
		GinkgoWriter.Println("✅ Infrastructure images pruned")
	}

	GinkgoWriter.Println("✅ Cleanup complete")
})

// ============================================================================
// TEST HELPER FUNCTIONS (for parallel execution isolation)
// ============================================================================

// createTestNamespace creates a managed test namespace for test isolation.
func createTestNamespace(prefix string) string {
	return helpers.CreateTestNamespace(ctx, k8sClient, prefix)
}

// deleteTestNamespace cleans up a test namespace.
func deleteTestNamespace(ns string) {
	helpers.DeleteTestNamespace(ctx, k8sClient, ns)
}

// createEffectivenessAssessment creates an EA CRD for testing.
// ValidityDeadline is NOT set in spec - the EM controller computes it on first reconciliation
// as EA.creationTimestamp + config.ValidityWindow.
func createEffectivenessAssessment(namespace, name, correlationID string) *eav1.EffectivenessAssessment {
	ea := &eav1.EffectivenessAssessment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"kubernaut.ai/correlation-id": correlationID,
			},
		},
		Spec: eav1.EffectivenessAssessmentSpec{
			CorrelationID: correlationID,
			TargetResource: eav1.TargetResource{
				Kind:      "Deployment",
				Name:      "test-app",
				Namespace: namespace,
			},
			Config: eav1.EAConfig{
				StabilizationWindow: metav1.Duration{Duration: 1 * time.Second}, // Very short for tests
				ScoringThreshold:    0.5,
				PrometheusEnabled:   true,
				AlertManagerEnabled: true,
			},
		},
	}
	Expect(k8sClient.Create(ctx, ea)).To(Succeed())
	GinkgoWriter.Printf("✅ Created EffectivenessAssessment: %s/%s (correlationID: %s)\n", namespace, name, correlationID)
	return ea
}

// fetchEA retrieves the current state of an EA from the API server.
// Returns the fetched EA. Fails the test if the EA cannot be found.
func fetchEA(namespace, name string) *eav1.EffectivenessAssessment {
	ea := &eav1.EffectivenessAssessment{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, ea)).To(Succeed())
	return ea
}

// waitForEAPhase polls until the EA reaches the expected phase or times out.
// Returns the final fetched EA for further assertions.
func waitForEAPhase(namespace, name, expectedPhase string) *eav1.EffectivenessAssessment {
	var fetched *eav1.EffectivenessAssessment
	Eventually(func(g Gomega) {
		fetched = &eav1.EffectivenessAssessment{}
		g.Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, fetched)).To(Succeed())
		g.Expect(fetched.Status.Phase).To(Equal(expectedPhase),
			fmt.Sprintf("EA %s/%s should reach %s phase", namespace, name, expectedPhase))
	}, timeout, interval).Should(Succeed())
	return fetched
}

// createExpiredEffectivenessAssessment creates an EA with an already-expired validity deadline.
// ValidityDeadline is computed by the EM controller on first reconcile. To test expired behavior,
// we create the EA normally, then patch the status with an expired ValidityDeadline before the
// controller's first reconcile (or we race with it). Using Status().Update() ensures the
// reconciler will see the expired deadline on its next reconcile.
func createExpiredEffectivenessAssessment(namespace, name, correlationID string) *eav1.EffectivenessAssessment {
	ea := &eav1.EffectivenessAssessment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"kubernaut.ai/correlation-id": correlationID,
			},
		},
		Spec: eav1.EffectivenessAssessmentSpec{
			CorrelationID: correlationID,
			TargetResource: eav1.TargetResource{
				Kind:      "Deployment",
				Name:      "test-app",
				Namespace: namespace,
			},
			Config: eav1.EAConfig{
				StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
				ScoringThreshold:    0.5,
				PrometheusEnabled:   true,
				AlertManagerEnabled: true,
			},
		},
	}
	Expect(k8sClient.Create(ctx, ea)).To(Succeed())
	// Patch status with expired ValidityDeadline so reconciler treats it as expired
	expired := metav1.NewTime(time.Now().Add(-1 * time.Minute))
	ea.Status.ValidityDeadline = &expired
	Expect(k8sClient.Status().Update(ctx, ea)).To(Succeed())
	GinkgoWriter.Printf("✅ Created expired EffectivenessAssessment: %s/%s\n", namespace, name)
	return ea
}
