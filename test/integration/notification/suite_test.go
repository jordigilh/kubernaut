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
	"path/filepath"
	"testing"
	"time"

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

	"github.com/sirupsen/logrus"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	notificationctrl "github.com/jordigilh/kubernaut/internal/controller/notification"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	"github.com/jordigilh/kubernaut/pkg/notification/sanitization"
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
)

// SlackWebhookRequest captures mock Slack webhook calls
type SlackWebhookRequest struct {
	Timestamp time.Time
	Body      []byte
	Headers   http.Header
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
	})
	Expect(err).ToNot(HaveOccurred())

	By("Setting up the Notification controller with delivery services")
	// Create test logger
	testLogger := logrus.New()
	testLogger.SetOutput(GinkgoWriter)
	testLogger.SetLevel(logrus.InfoLevel)

	// Create console delivery service
	consoleService := delivery.NewConsoleDeliveryService(testLogger)

	// Create Slack delivery service with mock webhook URL
	slackService := delivery.NewSlackDeliveryService(slackWebhookURL)

	// Create sanitizer
	sanitizer := sanitization.NewSanitizer()

	// Create controller with all dependencies
	err = (&notificationctrl.NotificationRequestReconciler{
		Client:         k8sManager.GetClient(),
		Scheme:         k8sManager.GetScheme(),
		ConsoleService: consoleService,
		SlackService:   slackService,
		Sanitizer:      sanitizer,
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
func deployMockSlackServer() {
	slackRequests = make([]SlackWebhookRequest, 0)

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

		// Record successful request
		slackRequests = append(slackRequests, SlackWebhookRequest{
			Timestamp: time.Now(),
			Body:      body,
			Headers:   r.Header.Clone(),
		})

		GinkgoWriter.Printf("✅ Mock Slack webhook received request #%d\n", len(slackRequests))
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

// resetSlackRequests clears the mock server request history
func resetSlackRequests() {
	slackRequests = make([]SlackWebhookRequest, 0)
	GinkgoWriter.Println("🔄 Mock Slack request history cleared")
}

// getSlackRequestCount returns the number of Slack webhook calls
func getSlackRequestCount() int {
	return len(slackRequests)
}

// getLastSlackRequest returns the most recent Slack webhook request
func getLastSlackRequest() *SlackWebhookRequest {
	if len(slackRequests) == 0 {
		return nil
	}
	return &slackRequests[len(slackRequests)-1]
}

// waitForNotificationPhase waits for notification to reach expected phase
func waitForNotificationPhase(ctx context.Context, name, namespace string, expectedPhase notificationv1alpha1.NotificationPhase, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		notif := &notificationv1alpha1.NotificationRequest{}
		err := k8sClient.Get(ctx, client.ObjectKey{
			Name:      name,
			Namespace: namespace,
		}, notif)

		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if notif.Status.Phase == expectedPhase {
			return nil
		}

		GinkgoWriter.Printf("   Waiting for phase %s... (current: %s)\n", expectedPhase, notif.Status.Phase)
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for notification %s/%s to reach phase %s", namespace, name, expectedPhase)
}
