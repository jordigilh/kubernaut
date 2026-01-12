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

package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/notification"
	"github.com/jordigilh/kubernaut/pkg/audit"
	notificationaudit "github.com/jordigilh/kubernaut/pkg/notification/audit"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	notificationmetrics "github.com/jordigilh/kubernaut/pkg/notification/metrics"
	notificationstatus "github.com/jordigilh/kubernaut/pkg/notification/status"
	"github.com/jordigilh/kubernaut/pkg/shared/circuitbreaker"
	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	"github.com/sony/gobreaker"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	ctx             context.Context
	cancel          context.CancelFunc
	testEnv         *envtest.Environment
	cfg             *rest.Config
	k8sClient       client.Client
	k8sAPIReader    client.Reader // DD-STATUS-001: Cache-bypassed reader for test assertions
	k8sManager      ctrl.Manager
	testNamespace   string // Default test namespace (kubernaut-notifications)
	mockSlackServer *httptest.Server
	slackWebhookURL string
	slackRequests   []SlackWebhookRequest
	slackRequestsMu sync.Mutex // Thread-safe access for parallel test execution (4 procs)

	// Audit store for testing controller audit emission (Defense-in-Depth Layer 4)
	realAuditStore audit.AuditStore // REAL audit store (DD-AUDIT-003 mandate compliance)

	// DeliveryOrchestrator exposed for mock service injection in tests
	// Tests can RegisterChannel() and UnregisterChannel() to inject mocks
	deliveryOrchestrator *delivery.Orchestrator

	// Original console and slack services for restoration after mock tests
	originalConsoleService *delivery.ConsoleDeliveryService
	originalSlackService   *delivery.SlackDeliveryService

	// orchestratorMockLock serializes mock registration/test/cleanup to prevent logical races
	// sync.Map provides thread-safety, but tests still need to lock the sequence:
	// register mocks → create NotificationRequest → validate → restore original services
	orchestratorMockLock sync.Mutex
)

// SlackWebhookRequest captures mock Slack webhook calls
// Includes TestID for correlation in parallel test execution (4 procs)
type SlackWebhookRequest struct {
	Timestamp time.Time
	Body      []byte
	Headers   http.Header
	TestID    string // Correlation ID for filtering requests per-test in parallel execution
}

// cleanupPodmanComposeInfrastructure removes containers and images from integration tests
// DD-TEST-001 v1.1: Prevents disk space exhaustion from stale containers/images
func cleanupPodmanComposeInfrastructure() {
	GinkgoWriter.Println("🗑️  DD-TEST-001 v1.1: Cleaning up podman-compose infrastructure...")

	// Get project name from podman-compose file (defaults to directory name)
	projectName := "notification"

	// Containers to remove (prefixed with project name)
	containers := []string{
		fmt.Sprintf("%s-datastorage-1", projectName),
		fmt.Sprintf("%s_datastorage_1", projectName), // Alternative naming
		fmt.Sprintf("%s-postgres-1", projectName),
		fmt.Sprintf("%s_postgres_1", projectName),
		fmt.Sprintf("%s-redis-1", projectName),
		fmt.Sprintf("%s_redis_1", projectName),
	}

	// Remove containers
	for _, container := range containers {
		cmd := exec.Command("podman", "rm", "-f", container)
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Ignore error if container doesn't exist
			if !contains(string(output), "no such container") {
				GinkgoWriter.Printf("   ⚠️  Failed to remove container %s: %v\n", container, err)
			}
		} else {
			GinkgoWriter.Printf("   ✅ Container removed: %s\n", container)
		}
	}

	// Prune dangling images from podman-compose builds
	pruneCmd := exec.Command("podman", "image", "prune", "-f")
	pruneOutput, pruneErr := pruneCmd.CombinedOutput()
	if pruneErr != nil {
		GinkgoWriter.Printf("   ⚠️  Failed to prune dangling images: %v (output: %s)\n", pruneErr, string(pruneOutput))
	} else {
		GinkgoWriter.Println("   ✅ Dangling images pruned")
	}

	GinkgoWriter.Println("✅ DD-TEST-001 v1.1: podman-compose cleanup complete")
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsInner(s, substr)))
}

func containsInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestNotificationIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Notification Controller Integration Suite (Envtest)")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	// Phase 1: Runs ONCE on parallel process #1
	// Start integration infrastructure (PostgreSQL, Redis, DataStorage)
	// This runs once to avoid container name collisions when TEST_PROCS > 1
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("Starting Notification integration infrastructure (Process #1 only, DD-TEST-002)")
	// This starts: PostgreSQL, Redis, Immudb, DataStorage
	// Per DD-TEST-001 v2.2: PostgreSQL=15440, Redis=16385, Immudb=13328, DS=18096
	dsInfra, err := infrastructure.StartDSBootstrap(infrastructure.DSBootstrapConfig{
		ServiceName:     "notification",
		PostgresPort:    15440, // DD-TEST-001 v2.2
		RedisPort:       16385, // DD-TEST-001 v2.2
		DataStoragePort: 18096, // DD-TEST-001 v2.2
		MetricsPort:     19096,
		ConfigDir:       "test/integration/notification/config",
	}, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred(), "Failed to start Notification integration infrastructure")
	GinkgoWriter.Println("✅ Notification integration infrastructure started (PostgreSQL, Redis, Immudb, DataStorage - shared across all processes)")

	// Clean up infrastructure on exit
	DeferCleanup(func() {
		infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
	})

	return []byte{} // No data to share between processes
}, func(data []byte) {
	// Phase 2: Runs on ALL parallel processes (receives data from phase 1)
	// Set up envtest, K8s client, and controller manager per process
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("Registering NotificationRequest CRD scheme")
	err := notificationv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	By("Bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	By("Creating controller-runtime client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By("Creating cache-bypassed API reader for test assertions (DD-STATUS-001)")
	// DD-STATUS-001: Create a DelegatingClient that bypasses cache for NotificationRequest
	// This ensures tests read fresh status updates from the API server
	uncachedClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	k8sAPIReader = uncachedClient // Use as API reader for test assertions
	GinkgoWriter.Println("  ✅ Cache-bypassed API reader initialized (DD-STATUS-001)")

	By("Creating namespaces for testing")
	// Create kubernaut-notifications namespace for controller
	testNamespace = "kubernaut-notifications"
	notifNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
		},
	}
	err = k8sClient.Create(ctx, notifNs)
	Expect(err).NotTo(HaveOccurred())

	// DD-TEST-002: Using kubernaut-notifications as default test namespace
	// Tests can create their own unique namespaces if needed for isolation
	GinkgoWriter.Printf("✅ Namespace created: %s (controller)\n", testNamespace)
	GinkgoWriter.Println("📦 Tests use this namespace by default (DD-TEST-002 compliance)")

	By("Deploying mock Slack webhook server")
	deployMockSlackServer()
	GinkgoWriter.Printf("✅ Mock Slack server deployed: %s\n", slackWebhookURL)

	By("Creating Slack webhook URL secret")
	createSlackWebhookSecret()
	GinkgoWriter.Println("✅ Slack webhook secret created in kubernaut-notifications namespace")

	By("Setting up the controller manager")
	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Use random port to avoid conflicts in parallel tests
		},
	})
	Expect(err).ToNot(HaveOccurred())

	By("Setting up the Notification controller with delivery services")
	// Create console delivery service (no logger needed per DD-013 logging standard)
	consoleService := delivery.NewConsoleDeliveryService()
	originalConsoleService = consoleService // Save for restoration after mock tests

	// Create Slack delivery service with mock webhook URL
	slackService := delivery.NewSlackDeliveryService(slackWebhookURL)
	originalSlackService = slackService // Save for restoration after mock tests

	// Create sanitizer
	sanitizer := sanitization.NewSanitizer()

	// Create audit manager for controller audit emission (BR-NOT-062)
	auditManager := notificationaudit.NewManager("notification-controller")

	// Create REAL audit store using Data Storage service (DD-AUDIT-003 mandate)
	// Per 03-testing-strategy.mdc: Integration tests MUST use real services (no mocks)
	dataStorageURL := os.Getenv("DATA_STORAGE_URL")
	if dataStorageURL == "" {
		dataStorageURL = "http://127.0.0.1:18096" // NT integration port (IPv4 explicit for CI, DD-TEST-001 v1.1)
	}

	// Verify Data Storage is healthy (infrastructure should have started it)
	// DS TEAM PATTERN: Use Eventually() with 30s timeout for health check
	// Rationale: Cold start on macOS Podman can take 15-20s (per DS team testing)
	Eventually(func() int {
		// Use 127.0.0.1 instead of localhost (DS team recommendation)
		resp, err := http.Get(dataStorageURL + "/health")
		if err != nil {
			GinkgoWriter.Printf("  DataStorage health check failed: %v\n", err)
			return 0
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}, "30s", "1s").Should(Equal(http.StatusOK),
		"❌ DataStorage failed to become healthy after infrastructure startup at %s\n"+
			"Per DD-AUDIT-003: Audit infrastructure is MANDATORY\n"+
			"Per DD-TEST-002: Infrastructure should be started programmatically by Go code", dataStorageURL)

	// Create Data Storage client with OpenAPI generated client (DD-API-001)
	// DD-AUTH-005: Integration tests use mock user transport (no oauth-proxy)
	mockTransport := testauth.NewMockUserTransport("test-notification@integration.test")
	dsClient, err := audit.NewOpenAPIClientAdapterWithTransport(
		dataStorageURL,
		5*time.Second,
		mockTransport, // ← Mock user header injection (simulates oauth-proxy)
	)
	Expect(err).ToNot(HaveOccurred(), "Failed to create Data Storage client")

	// Create REAL buffered audit store
	realAuditStore, err = audit.NewBufferedStore(
		dsClient,
		audit.DefaultConfig(),
		"notification-controller",
		ctrl.Log.WithName("audit"),
	)
	Expect(err).ToNot(HaveOccurred(), "Failed to create real audit store")

	// Pattern 1: Create Metrics recorder (DD-METRICS-001)
	metricsRecorder := notificationmetrics.NewPrometheusRecorder()
	GinkgoWriter.Println("  ✅ Metrics recorder initialized (Pattern 1)")

	// Pattern 2: Create Status Manager for centralized status updates
	// DD-STATUS-001: Pass API reader to bypass cache for fresh refetches
	statusManager := notificationstatus.NewManager(k8sManager.GetClient(), k8sManager.GetAPIReader())
	GinkgoWriter.Println("  ✅ Status Manager initialized (Pattern 2 + DD-STATUS-001)")

	// DD-NOT-007: Registration Pattern (MANDATORY)
	// Pattern 3: Create Delivery Orchestrator with NO channel parameters
	// Assign to global variable for test mock injection
	deliveryOrchestrator = delivery.NewOrchestrator(
		sanitizer,
		metricsRecorder,
		statusManager,
		ctrl.Log.WithName("delivery-orchestrator"),
	)

	// DD-NOT-007: Register only channels needed for integration tests
	deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), consoleService)
	deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelSlack), slackService)
	// Note: fileService and logService NOT registered (E2E only)

	GinkgoWriter.Println("  ✅ Delivery Orchestrator initialized (DD-NOT-007 Registration Pattern)")
	GinkgoWriter.Println("  ✅ DeliveryOrchestrator exposed for test mock injection")

	// Create circuit breaker for integration tests
	// Per BR-NOT-055: Circuit breaker provides per-channel isolation
	circuitBreakerManager := circuitbreaker.NewManager(gobreaker.Settings{
		MaxRequests: 2,
		Interval:    10 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			// Update metrics on state change
			if metricsRecorder != nil {
				metricsRecorder.UpdateCircuitBreakerState(name, to)
			}
		},
	})
	GinkgoWriter.Println("  ✅ Circuit Breaker Manager initialized (BR-NOT-055)")

	// Create controller with all dependencies including REAL audit (Defense-in-Depth Layer 4)
	// Patterns 1-3: Metrics, StatusManager, DeliveryOrchestrator wired in
	err = (&notification.NotificationRequestReconciler{
		Client:               k8sManager.GetClient(),
		Scheme:               k8sManager.GetScheme(),
		ConsoleService:       consoleService,
		SlackService:         slackService,
		Sanitizer:            sanitizer,
		CircuitBreaker:       circuitBreakerManager, // BR-NOT-055: Circuit breaker with gobreaker
		AuditStore:           realAuditStore,        // ✅ REAL audit store (mandate compliance)
		AuditManager:         auditManager,
		Metrics:              metricsRecorder,                                           // Pattern 1: Metrics (DD-METRICS-001)
		Recorder:             k8sManager.GetEventRecorderFor("notification-controller"), // Pattern 1: EventRecorder
		StatusManager:        statusManager,                                             // Pattern 2: Status Manager
		DeliveryOrchestrator: deliveryOrchestrator,                                      // Pattern 3: Delivery Orchestrator
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	By("Starting the controller manager")
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	// Per TESTING_GUIDELINES.md v2.0.0: Use Eventually(), never time.Sleep()
	// Wait for manager cache to sync and be ready to handle requests
	By("Waiting for controller manager to be ready")
	Eventually(func() error {
		// Verify manager is ready by checking if we can list CRDs
		list := &notificationv1alpha1.NotificationRequestList{}
		return k8sClient.List(ctx, list)
	}, 10*time.Second, 500*time.Millisecond).Should(Succeed(),
		"Controller manager cache should sync within 10 seconds")

	// Note: Metrics server uses dynamic port allocation (":0") to prevent conflicts
	// Port discovery is not exposed by controller-runtime Manager interface

	GinkgoWriter.Println("✅ Notification integration test environment ready!")
	GinkgoWriter.Println("")
	GinkgoWriter.Println("Environment:")
	GinkgoWriter.Println("  • Envtest with real Kubernetes API (etcd + kube-apiserver)")
	GinkgoWriter.Println("  • NotificationRequest CRD installed")
	GinkgoWriter.Println("  • Notification controller running")
	GinkgoWriter.Println("  • Mock Slack webhook server ready")
	GinkgoWriter.Println("")
})

var _ = SynchronizedAfterSuite(func() {
	// Phase 1: Runs on ALL parallel processes (per-process cleanup)
	By("Tearing down per-process test environment")

	// Close REAL audit store to flush remaining events (DD-AUDIT-003)
	// NT-SHUTDOWN-001: Flush audit store BEFORE stopping DataStorage
	// This prevents "connection refused" errors during cleanup when the
	// background writer tries to flush buffered events after DataStorage is stopped.
	// Integration tests MUST always use real DataStorage (DD-TESTING-001)
	By("Flushing audit store before infrastructure shutdown")

	flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer flushCancel()

	err := realAuditStore.Flush(flushCtx)
	if err != nil {
		GinkgoWriter.Printf("⚠️  Warning: Failed to flush audit store: %v\n", err)
	} else {
		GinkgoWriter.Println("✅ Audit store flushed (all buffered events written)")
	}

	By("Closing audit store")
	err = realAuditStore.Close()
	if err != nil {
		GinkgoWriter.Printf("⚠️  Warning: Failed to close audit store: %v\n", err)
	} else {
		GinkgoWriter.Println("✅ Audit store closed")
	}

	if mockSlackServer != nil {
		mockSlackServer.Close()
		GinkgoWriter.Println("✅ Mock Slack server stopped")
	}

	cancel()

	err = testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())

	GinkgoWriter.Println("✅ Per-process cleanup complete")
}, func() {
	// Phase 2: Runs ONCE on parallel process #1 (shared infrastructure cleanup)
	// This ensures DataStorage is only stopped AFTER all processes finish
	By("Stopping shared DataStorage infrastructure (DD-TEST-002)")

	// Check if any tests failed (for debugging)
	// If tests failed, preserve infrastructure for triaging
	skipCleanup := os.Getenv("SKIP_CLEANUP_ON_FAILURE") == "true"

	if skipCleanup {
		GinkgoWriter.Println("")
		GinkgoWriter.Println("⚠️  SKIP_CLEANUP_ON_FAILURE=true detected")
		GinkgoWriter.Println("⚠️  Preserving infrastructure containers for triaging")
		GinkgoWriter.Println("")
		GinkgoWriter.Println("To inspect:")
		GinkgoWriter.Println("  podman ps -a | grep notification")
		GinkgoWriter.Println("  podman logs notification-datastorage-1")
		GinkgoWriter.Println("  podman logs notification-postgres-1")
		GinkgoWriter.Println("")
		GinkgoWriter.Println("To cleanup manually:")
		GinkgoWriter.Println("  podman stop notification-datastorage-1 notification-postgres-1 notification-redis-1")
		GinkgoWriter.Println("  podman rm notification-datastorage-1 notification-postgres-1 notification-redis-1")
		GinkgoWriter.Println("")
	} else {
		// DD-TEST-001 v1.1: Clean up infrastructure containers
		// NT-SHUTDOWN-001: Safe to stop now - all processes flushed audit events
		By("Cleaning up integration test infrastructure (Go-bootstrapped)")
		cleanupPodmanComposeInfrastructure()
		GinkgoWriter.Println("✅ Shared infrastructure cleanup complete")
	}
})

// DD-TEST-002 Compliance: Unique namespace per test for parallel execution
// This enables -procs=4 parallel execution (3x speed improvement)
// Note: testNamespace is declared in package-level var block (line 74)

var _ = BeforeEach(func() {
	// DD-TEST-002: Create unique namespace per test (enables parallel execution)
	// Format: test-<8-char-uuid> for readability and uniqueness
	testNamespace = fmt.Sprintf("test-%s", uuid.New().String()[:8])

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
		},
	}

	Expect(k8sClient.Create(ctx, ns)).To(Succeed(),
		"Should create unique test namespace for isolation (DD-TEST-002)")

	GinkgoWriter.Printf("📦 Test namespace created: %s (DD-TEST-002 compliance)\n", testNamespace)
})

var _ = AfterEach(func() {
	// DD-TEST-002: Clean up namespace and ALL resources (instant cleanup)
	// This is MUCH faster than deleting individual notifications
	if testNamespace != "" {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}

		err := k8sClient.Delete(ctx, ns)
		if err != nil && !apierrors.IsNotFound(err) {
			GinkgoWriter.Printf("⚠️  Failed to delete namespace %s: %v\n", testNamespace, err)
		} else {
			GinkgoWriter.Printf("🗑️  Namespace %s deleted (DD-TEST-002 cleanup)\n", testNamespace)
		}
	}

	// NT-TEST-002 Fix: Reset mock Slack server state after each test
	// Prevents test pollution when tests configure failure modes
	if ConfigureFailureMode != nil {
		// Reset to success mode (none, 0 failures, 503 status code)
		ConfigureFailureMode("none", 0, http.StatusServiceUnavailable)
	}
	// Also clear request history for clean slate
	resetSlackRequests()
})

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
//
// This function streamlines the process by finding the required binaries, similar to
// setting the 'KUBEBUILDER_ASSETS' environment variable. To ensure the binaries are
// properly set up, run 'make setup-envtest' beforehand.
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

// deployMockSlackServer creates an HTTP server that simulates Slack webhook
// Thread-safe for parallel test execution (4 procs) - uses mutex for slackRequests
func deployMockSlackServer() {
	slackRequestsMu.Lock()
	slackRequests = make([]SlackWebhookRequest, 0)
	slackRequestsMu.Unlock()

	// Variable to control mock behavior for testing failure scenarios
	var (
		failureMode       string // "always", "first-N", "none"
		failureCount      int    // How many times to fail before succeeding
		currentFailures   int    // Counter for failures
		failureStatusCode int    // Status code to return on failure
	)

	// Default: normal operation
	failureMode = "none"
	failureStatusCode = http.StatusServiceUnavailable

	mockSlackServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			GinkgoWriter.Printf("❌ Failed to read Slack webhook body: %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Validate JSON
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			GinkgoWriter.Printf("❌ Invalid JSON in Slack webhook: %v\n", err)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("invalid_payload"))
			return
		}

		// Check failure mode for testing retry logic
		if failureMode == "always" {
			GinkgoWriter.Printf("⚠️  Mock Slack webhook configured to always fail (503)\n")
			w.WriteHeader(failureStatusCode)
			_, _ = w.Write([]byte("service_unavailable"))
			return
		}

		if failureMode == "first-N" && currentFailures < failureCount {
			currentFailures++
			GinkgoWriter.Printf("⚠️  Mock Slack webhook failure %d/%d (503)\n", currentFailures, failureCount)
			w.WriteHeader(failureStatusCode)
			_, _ = w.Write([]byte("service_unavailable"))
			return
		}

		// Handle special failure modes for edge case testing
		if failureMode == "malformed-json" {
			GinkgoWriter.Println("⚠️  Mock Slack webhook returning malformed JSON")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{invalid json response}"))
			return
		}

		if failureMode == "empty-response" {
			GinkgoWriter.Println("⚠️  Mock Slack webhook returning empty response body")
			w.WriteHeader(http.StatusOK)
			// Don't write anything - empty body
			return
		}

		// Extract test correlation ID from webhook payload (for parallel test isolation)
		// Slack webhook format uses blocks array with nested text
		testID := "unknown"
		if blocks, ok := payload["blocks"].([]interface{}); ok && len(blocks) > 0 {
			if firstBlock, ok := blocks[0].(map[string]interface{}); ok {
				if textObj, ok := firstBlock["text"].(map[string]interface{}); ok {
					if text, ok := textObj["text"].(string); ok {
						testID = text // Extract subject from first block (contains unique test identifier)
					}
				}
			}
		}

		// Record successful request (thread-safe for parallel tests)
		slackRequestsMu.Lock()
		slackRequests = append(slackRequests, SlackWebhookRequest{
			Timestamp: time.Now(),
			Body:      body,
			Headers:   r.Header.Clone(),
			TestID:    testID, // For filtering in parallel execution
		})
		requestCount := len(slackRequests)
		slackRequestsMu.Unlock()

		GinkgoWriter.Printf("✅ Mock Slack webhook received request #%d\n", requestCount)
		GinkgoWriter.Printf("   Content-Type: %s\n", r.Header.Get("Content-Type"))

		// Simulate Slack webhook response
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	slackWebhookURL = mockSlackServer.URL

	// Helper functions to configure mock behavior (exposed via closure)
	// These will be used in tests to simulate different failure scenarios

	// ConfigureFailureMode allows tests to configure mock server failure behavior
	ConfigureFailureMode = func(mode string, count int, statusCode int) {
		failureMode = mode
		failureCount = count
		currentFailures = 0
		failureStatusCode = statusCode
		GinkgoWriter.Printf("🔧 Mock Slack server configured: mode=%s, count=%d, statusCode=%d\n",
			mode, count, statusCode)
	}
}

// ConfigureFailureMode is set by deployMockSlackServer to allow tests to configure failure behavior
var ConfigureFailureMode func(mode string, count int, statusCode int)

// MockSlackServer represents a per-test mock Slack webhook server
// CRITICAL: Each test gets its own isolated mock server to prevent test pollution
type MockSlackServer struct {
	Server          *httptest.Server
	WebhookURL      string
	Requests        []SlackWebhookRequest
	RequestsMu      sync.Mutex
	FailureMode     string // "none", "always", "first-N"
	FailureCount    int    // How many times to fail before succeeding
	CurrentFailures int    // Counter for failures
	FailureStatus   int    // HTTP status code to return on failure
}

// createMockSlackServer creates an isolated mock Slack webhook server for a single test
// Returns server instance with dedicated request tracking (prevents test pollution)
func createMockSlackServer() *MockSlackServer { //nolint:unused
	mock := &MockSlackServer{
		Requests:      make([]SlackWebhookRequest, 0),
		FailureMode:   "none",
		FailureStatus: http.StatusServiceUnavailable,
	}

	mock.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			GinkgoWriter.Printf("❌ Failed to read Slack webhook body: %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Validate JSON
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			GinkgoWriter.Printf("❌ Invalid JSON in Slack webhook: %v\n", err)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("invalid_payload"))
			return
		}

		// Check failure mode for testing retry logic
		mock.RequestsMu.Lock()
		mode := mock.FailureMode
		failCount := mock.FailureCount
		currentFail := mock.CurrentFailures
		failStatus := mock.FailureStatus
		mock.RequestsMu.Unlock()

		if mode == "always" {
			GinkgoWriter.Printf("⚠️  Mock Slack webhook configured to always fail (%d)\n", failStatus)
			w.WriteHeader(failStatus)
			_, _ = w.Write([]byte("service_unavailable"))
			return
		}

		if mode == "first-N" && currentFail < failCount {
			mock.RequestsMu.Lock()
			mock.CurrentFailures++
			current := mock.CurrentFailures
			mock.RequestsMu.Unlock()

			GinkgoWriter.Printf("⚠️  Mock Slack webhook failure %d/%d (%d)\n", current, failCount, failStatus)
			w.WriteHeader(failStatus)
			_, _ = w.Write([]byte("service_unavailable"))
			return
		}

		// Extract test correlation ID from webhook payload
		testID := "unknown"
		if blocks, ok := payload["blocks"].([]interface{}); ok && len(blocks) > 0 {
			if firstBlock, ok := blocks[0].(map[string]interface{}); ok {
				if textObj, ok := firstBlock["text"].(map[string]interface{}); ok {
					if text, ok := textObj["text"].(string); ok {
						testID = text
					}
				}
			}
		}

		// Record successful request (thread-safe, per-test isolated)
		mock.RequestsMu.Lock()
		mock.Requests = append(mock.Requests, SlackWebhookRequest{
			Timestamp: time.Now(),
			Body:      body,
			Headers:   r.Header.Clone(),
			TestID:    testID,
		})
		requestCount := len(mock.Requests)
		mock.RequestsMu.Unlock()

		GinkgoWriter.Printf("✅ Mock Slack webhook received request #%d (testID: %s)\n", requestCount, testID)

		// Simulate Slack webhook response
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	mock.WebhookURL = mock.Server.URL
	return mock
}

// ConfigureFailure sets failure behavior for MockSlackServer
func (m *MockSlackServer) ConfigureFailure(mode string, count int, statusCode int) {
	m.RequestsMu.Lock()
	defer m.RequestsMu.Unlock()

	m.FailureMode = mode
	m.FailureCount = count
	m.CurrentFailures = 0
	m.FailureStatus = statusCode

	GinkgoWriter.Printf("🔧 Mock Slack server configured: mode=%s, count=%d, statusCode=%d\n", mode, count, statusCode)
}

// GetRequests returns a copy of all recorded requests (thread-safe)
func (m *MockSlackServer) GetRequests() []SlackWebhookRequest {
	m.RequestsMu.Lock()
	defer m.RequestsMu.Unlock()

	copy := make([]SlackWebhookRequest, len(m.Requests))
	for i, req := range m.Requests {
		copy[i] = req
	}
	return copy
}

// Reset clears all recorded requests and resets failure mode
func (m *MockSlackServer) Reset() {
	m.RequestsMu.Lock()
	defer m.RequestsMu.Unlock()

	m.Requests = make([]SlackWebhookRequest, 0)
	m.FailureMode = "none"
	m.CurrentFailures = 0
}

// getSlackRequestsCopy returns a thread-safe copy of slackRequests for parallel test execution
// Filters by testIdentifier (substring match on TestID) for test isolation in parallel runs
func getSlackRequestsCopy(testIdentifier string) []SlackWebhookRequest {
	slackRequestsMu.Lock()
	defer slackRequestsMu.Unlock()

	// Filter requests for this specific test
	var filtered []SlackWebhookRequest
	for _, req := range slackRequests {
		// Match by substring in TestID (which contains the notification subject/body)
		if testIdentifier == "" || len(req.TestID) == 0 {
			// No filtering - return all (backward compat)
			filtered = append(filtered, req)
		} else {
			// Filter by test identifier
			bodyStr := string(req.Body)
			if len(req.TestID) > 0 && (req.TestID == testIdentifier ||
				containsSubstring(req.TestID, testIdentifier) ||
				containsSubstring(bodyStr, testIdentifier)) {
				filtered = append(filtered, req)
			}
		}
	}
	return filtered
}

// containsSubstring is a helper for case-sensitive substring matching
func containsSubstring(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) &&
		(s == substr || stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// waitForReconciliationComplete waits for controller to fully complete reconciliation
// CRITICAL: Prevents "not found" errors when tests delete CRDs before controller finishes
func waitForReconciliationComplete(ctx context.Context, client client.Client, name, namespace string, expectedPhase notificationv1alpha1.NotificationPhase, timeout time.Duration) error {
	return wait.PollImmediate(500*time.Millisecond, timeout, func() (bool, error) {
		notif := &notificationv1alpha1.NotificationRequest{}
		err := client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, notif)
		if err != nil {
			return false, err
		}

		// Check phase matches AND CompletionTime is set (controller finished)
		if notif.Status.Phase == expectedPhase && notif.Status.CompletionTime != nil {
			return true, nil
		}

		// For non-terminal phases, just check phase
		if expectedPhase == notificationv1alpha1.NotificationPhasePending ||
			expectedPhase == notificationv1alpha1.NotificationPhaseSending {
			return notif.Status.Phase == expectedPhase, nil
		}

		return false, nil
	})
}

// deleteAndWait deletes a NotificationRequest and waits for it to be fully removed
// CRITICAL: Prevents test pollution by ensuring complete cleanup before next test
func deleteAndWait(ctx context.Context, client client.Client, notif *notificationv1alpha1.NotificationRequest, timeout time.Duration) error {
	// Delete the CRD
	if err := client.Delete(ctx, notif); err != nil {
		return err
	}

	// Wait for deletion to complete
	return wait.PollImmediate(100*time.Millisecond, timeout, func() (bool, error) {
		err := client.Get(ctx, types.NamespacedName{
			Name:      notif.Name,
			Namespace: notif.Namespace,
		}, &notificationv1alpha1.NotificationRequest{})

		if err != nil {
			// Object not found = deletion complete
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			// Other error
			return false, err
		}

		// Still exists, keep waiting
		return false, nil
	})
}

// getSlackRequestCount returns the count of Slack requests (thread-safe)
func getSlackRequestCount() int { //nolint:unused
	slackRequestsMu.Lock()
	defer slackRequestsMu.Unlock()
	return len(slackRequests)
}

// resetSlackRequests clears the slackRequests slice (thread-safe for parallel tests)
func resetSlackRequests() {
	slackRequestsMu.Lock()
	defer slackRequestsMu.Unlock()
	slackRequests = make([]SlackWebhookRequest, 0)
}

// createSlackWebhookSecret creates the Secret containing Slack webhook URL
func createSlackWebhookSecret() {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "notification-slack-webhook",
			Namespace:  "kubernaut-notifications",
			Generation: 1, // K8s increments on create/update
		},
		StringData: map[string]string{
			"webhook-url": slackWebhookURL,
		},
	}

	// Delete existing secret if it exists (idempotent)
	_ = k8sClient.Delete(ctx, secret)

	// Per TESTING_GUIDELINES.md v2.0.0: Use Eventually(), never time.Sleep()
	// Verify deletion completed before creating new secret
	Eventually(func() bool {
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      secret.Name,
			Namespace: secret.Namespace,
		}, &corev1.Secret{})
		return apierrors.IsNotFound(err)
	}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(),
		"Secret deletion should complete within 5 seconds")

	// Create new secret
	err := k8sClient.Create(ctx, secret)
	Expect(err).ToNot(HaveOccurred(), "Failed to create Slack webhook secret")

	GinkgoWriter.Printf("✅ Slack webhook secret created with URL: %s\n", slackWebhookURL)
}
