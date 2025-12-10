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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
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
	k8sManager      ctrl.Manager
	mockSlackServer *httptest.Server
	slackWebhookURL string
	slackRequests   []SlackWebhookRequest
	slackRequestsMu sync.Mutex // Thread-safe access for parallel test execution (4 procs)

	// Audit store for testing controller audit emission (Defense-in-Depth Layer 4)
	testAuditStore *TestAuditStore
)

// SlackWebhookRequest captures mock Slack webhook calls
// Includes TestID for correlation in parallel test execution (4 procs)
type SlackWebhookRequest struct {
	Timestamp time.Time
	Body      []byte
	Headers   http.Header
	TestID    string // Correlation ID for filtering requests per-test in parallel execution
}

func TestNotificationIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Notification Controller Integration Suite (Envtest)")
}

var _ = BeforeSuite(func() {
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

	By("Creating namespaces for testing")
	// Create kubernaut-notifications namespace for controller
	notifNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kubernaut-notifications",
		},
	}
	err = k8sClient.Create(ctx, notifNs)
	Expect(err).NotTo(HaveOccurred())

	// Create default namespace for tests
	defaultNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}
	_ = k8sClient.Create(ctx, defaultNs) // May already exist

	GinkgoWriter.Println("✅ Namespaces created: kubernaut-notifications, default")

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

	// Create Slack delivery service with mock webhook URL
	slackService := delivery.NewSlackDeliveryService(slackWebhookURL)

	// Create sanitizer
	sanitizer := sanitization.NewSanitizer()

	// Create audit helpers for controller audit emission (BR-NOT-062)
	auditHelpers := notification.NewAuditHelpers("notification-controller")

	// Create mock audit store for testing audit emission
	// This captures audit events emitted by the controller during reconciliation
	testAuditStore = NewTestAuditStore()

	// Create controller with all dependencies including audit (Defense-in-Depth Layer 4)
	err = (&notification.NotificationRequestReconciler{
		Client:         k8sManager.GetClient(),
		Scheme:         k8sManager.GetScheme(),
		ConsoleService: consoleService,
		SlackService:   slackService,
		Sanitizer:      sanitizer,
		AuditStore:     testAuditStore,
		AuditHelpers:   auditHelpers,
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	By("Starting the controller manager")
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	// Wait for manager to be ready
	time.Sleep(2 * time.Second)

	GinkgoWriter.Println("✅ Notification integration test environment ready!")
	GinkgoWriter.Println("")
	GinkgoWriter.Println("Environment:")
	GinkgoWriter.Println("  • Envtest with real Kubernetes API (etcd + kube-apiserver)")
	GinkgoWriter.Println("  • NotificationRequest CRD installed")
	GinkgoWriter.Println("  • Notification controller running")
	GinkgoWriter.Println("  • Mock Slack webhook server ready")
	GinkgoWriter.Println("")
})

var _ = AfterSuite(func() {
	By("Tearing down the test environment")

	if mockSlackServer != nil {
		mockSlackServer.Close()
		GinkgoWriter.Println("✅ Mock Slack server stopped")
	}

	cancel()

	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())

	GinkgoWriter.Println("✅ Cleanup complete")
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
func createMockSlackServer() *MockSlackServer {
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
func getSlackRequestCount() int {
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
			Name:      "notification-slack-webhook",
			Namespace: "kubernaut-notifications",
		},
		StringData: map[string]string{
			"webhook-url": slackWebhookURL,
		},
	}

	// Delete existing secret if it exists (idempotent)
	_ = k8sClient.Delete(ctx, secret)

	// Wait a moment for deletion to complete
	time.Sleep(100 * time.Millisecond)

	// Create new secret
	err := k8sClient.Create(ctx, secret)
	Expect(err).ToNot(HaveOccurred(), "Failed to create Slack webhook secret")

	GinkgoWriter.Printf("✅ Slack webhook secret created with URL: %s\n", slackWebhookURL)
}

// ========================================
// TEST AUDIT STORE (Defense-in-Depth Layer 4)
// ========================================
//
// TestAuditStore captures audit events emitted by the controller during reconciliation.
// This enables testing that the controller calls audit methods at the right lifecycle points:
// - notification.message.sent on successful delivery
// - notification.message.failed on failed delivery
//
// See: BR-NOT-062, BR-NOT-063, BR-NOT-064

// TestAuditStore implements audit.AuditStore for testing
type TestAuditStore struct {
	events []*audit.AuditEvent
	mu     sync.Mutex
	closed bool
}

// NewTestAuditStore creates a new test audit store
func NewTestAuditStore() *TestAuditStore {
	return &TestAuditStore{
		events: []*audit.AuditEvent{},
	}
}

// StoreAudit stores an audit event (implements audit.AuditStore)
func (s *TestAuditStore) StoreAudit(ctx context.Context, event *audit.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

// Close closes the audit store (implements audit.AuditStore)
func (s *TestAuditStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// GetEvents returns all captured audit events (for test assertions)
func (s *TestAuditStore) GetEvents() []*audit.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*audit.AuditEvent, len(s.events))
	copy(result, s.events)
	return result
}

// GetEventsByType returns events filtered by event type (for test assertions)
func (s *TestAuditStore) GetEventsByType(eventType string) []*audit.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*audit.AuditEvent
	for _, e := range s.events {
		if e.EventType == eventType {
			result = append(result, e)
		}
	}
	return result
}

// GetEventsByResourceID returns events filtered by resource ID (for test assertions)
func (s *TestAuditStore) GetEventsByResourceID(resourceID string) []*audit.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*audit.AuditEvent
	for _, e := range s.events {
		if e.ResourceID == resourceID {
			result = append(result, e)
		}
	}
	return result
}

// Clear removes all captured events (for test isolation)
func (s *TestAuditStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = []*audit.AuditEvent{}
}

// EventCount returns the number of captured events
func (s *TestAuditStore) EventCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}
