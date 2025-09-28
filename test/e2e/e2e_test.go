//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jordigilh/kubernaut/pkg/integration/webhook"
)

const (
	webhookURL = "http://localhost:8080/alerts"
	healthURL  = "http://localhost:8080/health"
)

func TestAlertToAction(t *testing.T) {
	// Skip if application is not running
	if os.Getenv("SKIP_E2E") != "" {
		t.Skip("End-to-end tests skipped")
	}

	// Check if application is healthy
	if !isApplicationHealthy() {
		t.Skip("Application is not running, skipping E2E test")
	}

	// Load test alert payload
	alertPayload := webhook.AlertManagerWebhook{
		Version:  "4",
		GroupKey: "e2e-test-group",
		Status:   "firing",
		Receiver: "kubernaut",
		Alerts: []webhook.AlertManagerAlert{
			{
				Status: "firing",
				Labels: map[string]string{
					"alertname":  "HighMemoryUsage",
					"severity":   "warning",
					"namespace":  "e2e-test",
					"pod":        "test-pod-e2e",
					"deployment": "test-deployment",
				},
				Annotations: map[string]string{
					"description": "E2E test alert for memory usage",
					"summary":     "Test alert for end-to-end validation",
				},
				StartsAt: time.Now(),
			},
		},
	}

	// Send alert to webhook
	jsonData, err := json.Marshal(alertPayload)
	if err != nil {
		t.Fatalf("Failed to marshal alert payload: %v", err)
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Failed to send alert to webhook: %v", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var webhookResp webhook.WebhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&webhookResp); err != nil {
		t.Fatalf("Failed to decode webhook response: %v", err)
	}

	if webhookResp.Status != "success" {
		t.Errorf("Expected success status, got: %s (error: %s)", webhookResp.Status, webhookResp.Error)
	}

	t.Logf("E2E test successful - processed alert with response: %s", webhookResp.Message)

	// In a real E2E test, we would also verify:
	// 1. SLM analysis was performed
	// 2. Action was executed (in dry-run mode)
	// 3. Metrics were updated
	// 4. Logs contain expected entries
}

func TestHealthEndpoint(t *testing.T) {
	if os.Getenv("SKIP_E2E") != "" {
		t.Skip("End-to-end tests skipped")
	}

	resp, err := http.Get(healthURL)
	if err != nil {
		t.Fatalf("Failed to check health endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Health endpoint returned status %d", resp.StatusCode)
	}

	var healthResp webhook.WebhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
		t.Fatalf("Failed to decode health response: %v", err)
	}

	if healthResp.Status != "healthy" {
		t.Errorf("Expected healthy status, got: %s", healthResp.Status)
	}
}

func TestInvalidAlert(t *testing.T) {
	if os.Getenv("SKIP_E2E") != "" {
		t.Skip("End-to-end tests skipped")
	}

	if !isApplicationHealthy() {
		t.Skip("Application is not running, skipping E2E test")
	}

	// Send invalid JSON
	invalidPayload := []byte("invalid json")

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(invalidPayload))
	if err != nil {
		t.Fatalf("Failed to send invalid alert: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

func TestLoadTest(t *testing.T) {
	if os.Getenv("SKIP_LOAD_TEST") != "" {
		t.Skip("Load tests skipped")
	}

	if !isApplicationHealthy() {
		t.Skip("Application is not running, skipping load test")
	}

	// Create test alert
	alertPayload := webhook.AlertManagerWebhook{
		Version:  "4",
		GroupKey: "load-test-group",
		Status:   "firing",
		Receiver: "kubernaut",
		Alerts: []webhook.AlertManagerAlert{
			{
				Status: "firing",
				Labels: map[string]string{
					"alertname": "LoadTestAlert",
					"severity":  "info",
					"namespace": "load-test",
				},
				Annotations: map[string]string{
					"description": "Load test alert",
				},
				StartsAt: time.Now(),
			},
		},
	}

	jsonData, err := json.Marshal(alertPayload)
	if err != nil {
		t.Fatalf("Failed to marshal alert payload: %v", err)
	}

	// Send multiple requests concurrently
	concurrency := 10
	requests := 50

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	results := make(chan error, requests)

	for i := 0; i < concurrency; i++ {
		go func() {
			for j := 0; j < requests/concurrency; j++ {
				select {
				case <-ctx.Done():
					results <- ctx.Err()
					return
				default:
					resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
					if err != nil {
						results <- err
						continue
					}
					resp.Body.Close()

					if resp.StatusCode != http.StatusOK {
						results <- fmt.Errorf("unexpected status: %d", resp.StatusCode)
						continue
					}

					results <- nil
				}
			}
		}()
	}

	// Collect results
	successCount := 0
	errorCount := 0

	for i := 0; i < requests; i++ {
		select {
		case err := <-results:
			if err != nil {
				errorCount++
				t.Logf("Request failed: %v", err)
			} else {
				successCount++
			}
		case <-time.After(30 * time.Second):
			t.Fatalf("Load test timed out")
		}
	}

	t.Logf("Load test completed - Success: %d, Errors: %d", successCount, errorCount)

	if errorCount > requests/10 { // Allow 10% error rate
		t.Errorf("Too many errors in load test: %d/%d", errorCount, requests)
	}
}

func isApplicationHealthy() bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(healthURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
