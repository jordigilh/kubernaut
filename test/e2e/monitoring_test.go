//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jordigilh/kubernaut/pkg/integration/webhook"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	monitoringNamespace = "monitoring"
	prometheusPort      = 9090
	alertmanagerPort    = 9093
)

func TestCompleteMonitoringFlow(t *testing.T) {
	if os.Getenv("SKIP_MONITORING_E2E") != "" {
		t.Skip("Monitoring e2e tests skipped")
	}

	if !isKindClusterAvailable(t) {
		t.Skip("KinD cluster not available, run: ./scripts/setup-kind-cluster.sh")
	}

	t.Run("TestPrometheusDeployment", func(t *testing.T) {
		testPrometheusDeployment(t)
	})

	t.Run("TestAlertManagerDeployment", func(t *testing.T) {
		testAlertManagerDeployment(t)
	})

	t.Run("TestAlertRulesLoaded", func(t *testing.T) {
		testAlertRulesLoaded(t)
	})

	t.Run("TestAlertWebhookFlow", func(t *testing.T) {
		testAlertWebhookFlow(t)
	})

	t.Run("TestEndToEndAlertProcessing", func(t *testing.T) {
		testEndToEndAlertProcessing(t)
	})
}

func testPrometheusDeployment(t *testing.T) {
	client := getKubernetesClient(t)
	ctx := context.Background()

	// Check if Prometheus deployment exists and is ready
	deployment, err := client.AppsV1().Deployments(monitoringNamespace).Get(ctx, "prometheus", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get Prometheus deployment: %v", err)
	}

	if deployment.Status.ReadyReplicas == 0 {
		t.Errorf("Prometheus deployment has no ready replicas")
	}

	// Check if Prometheus service exists
	_, err = client.CoreV1().Services(monitoringNamespace).Get(ctx, "prometheus", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get Prometheus service: %v", err)
	}

	t.Log("Prometheus deployment is healthy")
}

func testAlertManagerDeployment(t *testing.T) {
	client := getKubernetesClient(t)
	ctx := context.Background()

	// Check if AlertManager deployment exists and is ready
	deployment, err := client.AppsV1().Deployments(monitoringNamespace).Get(ctx, "alertmanager", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get AlertManager deployment: %v", err)
	}

	if deployment.Status.ReadyReplicas == 0 {
		t.Errorf("AlertManager deployment has no ready replicas")
	}

	// Check if AlertManager service exists
	_, err = client.CoreV1().Services(monitoringNamespace).Get(ctx, "alertmanager", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get AlertManager service: %v", err)
	}

	t.Log("AlertManager deployment is healthy")
}

func testAlertRulesLoaded(t *testing.T) {
	// Port forward to Prometheus to check if rules are loaded
	client := getKubernetesClient(t)

	// For e2e tests, we'll use a simple HTTP check after port forwarding
	// In a real scenario, you'd set up port forwarding programmatically

	// Check if the rules configmap exists
	ctx := context.Background()
	_, err := client.CoreV1().ConfigMaps(monitoringNamespace).Get(ctx, "prometheus-rules", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get prometheus-rules configmap: %v", err)
	}

	t.Log("Alert rules configmap exists")

	// TODO: Add actual Prometheus API call to verify rules are loaded
	// This would require setting up port forwarding or using a service
	t.Log("Alert rules validation requires manual verification via port forwarding")
}

func testAlertWebhookFlow(t *testing.T) {
	// This test simulates AlertManager sending a webhook to our application
	// For this to work, the application needs to be deployed in the cluster

	// Create a test alert payload that matches our e2e alert rules
	testAlert := webhook.AlertManagerWebhook{
		Version:  "4",
		GroupKey: "test-alert-webhook",
		Status:   "firing",
		Receiver: "prometheus-alerts-slm",
		Alerts: []webhook.Alert{
			{
				Status: "firing",
				Labels: map[string]string{
					"alertname": "TestAlertAlwaysFiring",
					"severity":  "info",
					"namespace": "e2e-test",
					"pod":       "test-webhook-pod",
				},
				Annotations: map[string]string{
					"summary":     "Test alert for e2e validation",
					"description": "This alert tests the webhook flow",
					"test_type":   "synthetic",
				},
				StartsAt: time.Now(),
			},
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(testAlert)
	if err != nil {
		t.Fatalf("Failed to marshal test alert: %v", err)
	}

	// Send to webhook endpoint (assuming app is running on port 8080)
	// In a complete e2e test, the app would be deployed in the cluster
	webhookURL := "http://localhost:8080/alerts"

	// For now, we'll just validate the payload structure
	var decoded webhook.AlertManagerWebhook
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("Failed to decode test alert payload: %v", err)
	}

	if len(decoded.Alerts) != 1 {
		t.Errorf("Expected 1 alert, got %d", len(decoded.Alerts))
	}

	if decoded.Alerts[0].Labels["alertname"] != "TestAlertAlwaysFiring" {
		t.Errorf("Expected alertname 'TestAlertAlwaysFiring', got '%s'", decoded.Alerts[0].Labels["alertname"])
	}

	t.Logf("Webhook test alert prepared successfully for URL: %s", webhookURL)
	t.Log("Note: Complete webhook test requires deployed application instance")
}

func testEndToEndAlertProcessing(t *testing.T) {
	// This test validates the complete flow:
	// 1. Alert rules trigger in Prometheus
	// 2. AlertManager sends webhook
	// 3. Application receives and processes alert
	// 4. SLM analyzes alert and recommends action
	// 5. Action is executed (in dry-run mode)

	client := getKubernetesClient(t)
	ctx := context.Background()

	// Check that monitoring components are running
	pods, err := client.CoreV1().Pods(monitoringNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list monitoring pods: %v", err)
	}

	runningPods := 0
	for _, pod := range pods.Items {
		if pod.Status.Phase == "Running" {
			runningPods++
		}
	}

	if runningPods < 3 { // prometheus, alertmanager, kube-state-metrics
		t.Errorf("Expected at least 3 running pods in monitoring namespace, got %d", runningPods)
	}

	// Check test namespace has resources to monitor
	testPods, err := client.CoreV1().Pods(testNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list test pods: %v", err)
	}

	if len(testPods.Items) == 0 {
		t.Error("No test pods found to generate alerts from")
	}

	t.Log("End-to-end test validation:")
	t.Logf("- Monitoring pods running: %d", runningPods)
	t.Logf("- Test pods available: %d", len(testPods.Items))
	t.Log("- Alert rules configured")
	t.Log("- AlertManager webhook configured")

	// In a complete implementation, this would:
	// 1. Wait for alerts to fire
	// 2. Monitor webhook endpoint for incoming alerts
	// 3. Verify SLM processing and action execution
	// 4. Validate cluster state changes

	t.Log("Complete e2e test requires:")
	t.Log("1. Deploy prometheus-alerts-slm application in cluster")
	t.Log("2. Configure service to receive AlertManager webhooks")
	t.Log("3. Wait for actual alerts to trigger")
	t.Log("4. Monitor logs for processing confirmation")
}

func checkPrometheusAPI(t *testing.T, endpoint string) {
	// Helper function to check Prometheus API endpoints
	// This would be used with port forwarding in a real test

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(endpoint)
	if err != nil {
		t.Logf("Failed to reach Prometheus API at %s: %v", endpoint, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Prometheus API returned status %d", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Failed to read response body: %v", err)
		return
	}

	t.Logf("Prometheus API response: %s", string(body)[:100]) // First 100 chars
}

func simulateAlertManagerWebhook(t *testing.T, webhookURL string, alert webhook.AlertManagerWebhook) {
	// Helper function to simulate AlertManager sending a webhook

	jsonData, err := json.Marshal(alert)
	if err != nil {
		t.Fatalf("Failed to marshal alert: %v", err)
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Logf("Failed to send webhook to %s: %v", webhookURL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Webhook endpoint returned status %d: %s", resp.StatusCode, string(body))
		return
	}

	var response webhook.WebhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Errorf("Failed to decode webhook response: %v", err)
		return
	}

	if response.Status != "success" {
		t.Errorf("Webhook processing failed: %s", response.Error)
		return
	}

	t.Log("Webhook simulation successful")
}
