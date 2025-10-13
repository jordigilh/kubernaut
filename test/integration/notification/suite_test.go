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
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/testutil/kind"
)

// Test suite variables
var (
	suite           *kind.IntegrationSuite
	k8sClient       kubernetes.Interface
	crClient        client.Client
	ctx             context.Context
	cancel          context.CancelFunc
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
	RunSpecs(t, "Notification Controller Integration Suite (KIND)")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	By("Connecting to existing KIND cluster")
	// Use Kind template for standardized test setup
	// Expected: KIND cluster named 'notification-test' with namespaces:
	//   - kubernaut-notifications (notification controller)
	//   - kubernaut-system (shared components)
	suite = kind.Setup("notification-test", "kubernaut-notifications", "kubernaut-system")
	k8sClient = suite.Client

	By("Registering NotificationRequest CRD scheme")
	err := notificationv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred(), "Failed to register NotificationRequest CRD scheme")

	By("Creating controller-runtime client for CRD access")
	cfg, err := config.GetConfig()
	Expect(err).NotTo(HaveOccurred(), "Failed to get KIND cluster REST config")

	crClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred(), "Failed to create controller-runtime client")
	Expect(crClient).NotTo(BeNil(), "Controller-runtime client should not be nil")

	GinkgoWriter.Println("✅ Controller-runtime client initialized for NotificationRequest CRD")

	By("Deploying mock Slack webhook server")
	deployMockSlackServer()
	GinkgoWriter.Printf("✅ Mock Slack server deployed: %s\n", slackWebhookURL)

	By("Creating Slack webhook URL secret")
	createSlackWebhookSecret()
	GinkgoWriter.Println("✅ Slack webhook secret created in kubernaut-notifications namespace")

	GinkgoWriter.Println("✅ Notification integration test environment ready!")
	GinkgoWriter.Println("")
	GinkgoWriter.Println("Prerequisites:")
	GinkgoWriter.Println("  1. KIND cluster 'notification-test' must be running")
	GinkgoWriter.Println("  2. Notification controller must be deployed")
	GinkgoWriter.Println("  3. NotificationRequest CRD must be installed")
	GinkgoWriter.Println("")
})

var _ = AfterSuite(func() {
	By("Tearing down the test environment")

	if mockSlackServer != nil {
		mockSlackServer.Close()
		GinkgoWriter.Println("✅ Mock Slack server stopped")
	}

	cancel()

	if suite != nil {
		suite.Cleanup()
	}

	GinkgoWriter.Println("✅ Cleanup complete")
})

// deployMockSlackServer creates an HTTP server that simulates Slack webhook
func deployMockSlackServer() {
	slackRequests = make([]SlackWebhookRequest, 0)

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
			w.Write([]byte("invalid_payload"))
			return
		}

		// Record request
		slackRequests = append(slackRequests, SlackWebhookRequest{
			Timestamp: time.Now(),
			Body:      body,
			Headers:   r.Header.Clone(),
		})

		GinkgoWriter.Printf("✅ Mock Slack webhook received request #%d\n", len(slackRequests))
		GinkgoWriter.Printf("   Content-Type: %s\n", r.Header.Get("Content-Type"))
		GinkgoWriter.Printf("   Body: %s\n", string(body))

		// Simulate Slack webhook response
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	slackWebhookURL = mockSlackServer.URL
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
	_ = crClient.Delete(ctx, secret)

	// Create new secret
	err := crClient.Create(ctx, secret)
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

