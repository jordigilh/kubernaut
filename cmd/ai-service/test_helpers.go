//go:build unit
// +build unit

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// BDD-compatible test helper functions following existing patterns from pkg/testutil/

// createTestAIServerBDD creates a test AI service server for BDD testing
// Reuses existing test data factory patterns from pkg/testutil/test_data_factory.go
func createTestAIServerBDD(logger *logrus.Logger) *httptest.Server {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	}

	// Set test environment variables for consistent behavior
	os.Setenv("USE_MOCK_LLM", "true")
	os.Setenv("LOG_LEVEL", "error")

	// Create AI service with test configuration
	aiService := NewAIService(logger)

	// Initialize with test context
	ctx := context.Background()
	if err := aiService.Initialize(ctx); err != nil {
		logger.WithError(err).Fatal("Failed to initialize test AI service")
	}

	// Create test server with AI service routes
	mux := http.NewServeMux()
	aiService.RegisterRoutes(mux)
	server := httptest.NewServer(mux)

	return server
}

// createTestAlert creates a test alert following pkg/testutil patterns
func createTestAlert(name, severity, namespace, resource string) types.Alert {
	return types.Alert{
		Name:      name,
		Severity:  severity,
		Namespace: namespace,
		Resource:  resource,
		Labels: map[string]string{
			"alertname": name,
			"severity":  severity,
		},
		Annotations: map[string]string{
			"description": "Test alert for " + name,
			"summary":     "Test alert summary",
		},
	}
}

// HTTP Request Helper Functions - REUSABLE PATTERNS

// makeJSONRequest creates and executes a JSON HTTP request - REUSABLE
func makeJSONRequest(server *httptest.Server, method, endpoint string, payload interface{}) (*http.Response, error) {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, server.URL+endpoint, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	return http.DefaultClient.Do(req)
}

// makeAnalyzeAlertRequest creates analyze-alert request - REUSABLE
func makeAnalyzeAlertRequest(server *httptest.Server, alert types.Alert, context map[string]interface{}) (*http.Response, error) {
	payload := map[string]interface{}{
		"alert": alert,
	}
	if context != nil {
		payload["context"] = context
	}

	return makeJSONRequest(server, http.MethodPost, "/api/v1/analyze-alert", payload)
}

// makeRecommendationRequest creates recommendation request - REUSABLE
func makeRecommendationRequest(server *httptest.Server, alert types.Alert, context, constraints map[string]interface{}) (*http.Response, error) {
	payload := map[string]interface{}{
		"alert": alert,
	}
	if context != nil {
		payload["context"] = context
	}
	if constraints != nil {
		payload["constraints"] = constraints
	}

	return makeJSONRequest(server, http.MethodPost, "/api/v1/recommendations", payload)
}

// validateJSONResponse validates HTTP response and decodes JSON - REUSABLE
func validateJSONResponse(resp *http.Response, expectedStatus int, target interface{}, businessRequirement string) {
	defer resp.Body.Close()

	Expect(resp.StatusCode).To(Equal(expectedStatus),
		"%s: Should return expected HTTP status", businessRequirement)

	if target != nil {
		err := json.NewDecoder(resp.Body).Decode(target)
		Expect(err).ToNot(HaveOccurred(),
			"%s: Should return valid JSON response", businessRequirement)
	}
}

// createTestContext creates reusable test context - REUSABLE
func createTestContext(requestID string) map[string]interface{} {
	return map[string]interface{}{
		"request_id": requestID,
		"timestamp":  time.Now().Format(time.RFC3339),
	}
}

// Additional BDD helper functions following pkg/testutil patterns will be added as needed
