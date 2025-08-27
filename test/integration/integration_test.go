//go:build integration
// +build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/slm"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/sirupsen/logrus"
)

func TestSLMIntegration(t *testing.T) {
	// Skip if Ollama is not available
	if os.Getenv("SKIP_INTEGRATION") != "" {
		t.Skip("Integration tests skipped")
	}

	// Create logger
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create SLM configuration
	cfg := config.SLMConfig{
		Provider:    "localai",
		Endpoint:    "http://localhost:11434",
		Model:       "granite3.1-dense:8b",
		Temperature: 0.3,
		MaxTokens:   500,
		Timeout:     30 * time.Second,
		RetryCount:  2,
	}

	// Create SLM client
	client, err := slm.NewClient(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create SLM client: %v", err)
	}

	// Test health check
	if !client.IsHealthy() {
		t.Skip("Ollama is not healthy, skipping integration test")
	}

	// Create test alert
	testAlert := types.Alert{
		Name:        "HighMemoryUsage",
		Status:      "firing",
		Severity:    "warning",
		Description: "Pod is using 95% of memory limit",
		Namespace:   "production",
		Resource:    "web-app-pod-123",
		Labels: map[string]string{
			"alertname":  "HighMemoryUsage",
			"severity":   "warning",
			"namespace":  "production",
			"pod":        "web-app-pod-123",
			"deployment": "web-app",
		},
		Annotations: map[string]string{
			"description": "Pod is using 95% of memory limit",
			"summary":     "High memory usage detected",
		},
		StartsAt: time.Now().Add(-5 * time.Minute),
	}

	// Test alert analysis
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	recommendation, err := client.AnalyzeAlert(ctx, testAlert)
	if err != nil {
		t.Fatalf("Failed to analyze alert: %v", err)
	}

	// Validate recommendation
	if recommendation.Action == "" {
		t.Errorf("Expected non-empty action, got empty")
	}

	validActions := []string{"scale_deployment", "restart_pod", "increase_resources", "notify_only"}
	actionValid := false
	for _, action := range validActions {
		if recommendation.Action == action {
			actionValid = true
			break
		}
	}

	if !actionValid {
		t.Errorf("Invalid action: %s, expected one of %v", recommendation.Action, validActions)
	}

	if recommendation.Confidence < 0.0 || recommendation.Confidence > 1.0 {
		t.Errorf("Invalid confidence: %f, expected between 0.0 and 1.0", recommendation.Confidence)
	}

	t.Logf("Integration test successful - Action: %s, Confidence: %.2f",
		recommendation.Action, recommendation.Confidence)
}

func TestKubernetesIntegration(t *testing.T) {
	// Skip if OpenShift is not available
	if os.Getenv("SKIP_K8S_INTEGRATION") != "" {
		t.Skip("Kubernetes integration tests skipped")
	}

	// This test would require actual cluster access
	// For now, we'll create a basic test that validates the manifests

	// Check if test deployment manifest exists
	manifestPath := filepath.Join("..", "manifests", "test-deployment.yaml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Errorf("Test deployment manifest not found: %s", manifestPath)
	}

	t.Log("Kubernetes integration test placeholder - would test actual cluster operations")
}

func TestEndToEndFlow(t *testing.T) {
	// Skip if components are not available
	if os.Getenv("SKIP_E2E") != "" {
		t.Skip("End-to-end tests skipped")
	}

	// This would test the complete flow:
	// 1. Send webhook to handler
	// 2. Process alert through processor
	// 3. Analyze with SLM
	// 4. Execute action via executor
	// 5. Verify result

	t.Log("End-to-end test placeholder - would test complete alert processing flow")
}
