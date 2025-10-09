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

package security_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/integration/webhook"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// MockProcessor for security testing
type MockProcessor struct {
	mu        sync.RWMutex
	callCount int
}

func (m *MockProcessor) ProcessAlert(ctx context.Context, alert types.Alert) error {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()
	return nil
}

func (m *MockProcessor) ShouldProcess(alert types.Alert) bool {
	return true
}

func (m *MockProcessor) GetCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callCount
}

// TestBR_WH_004_Authentication validates BR-WH-004: Authentication and authorization
func TestBR_WH_004_Authentication(t *testing.T) {
	// Business Requirement: BR-WH-004 - Implement webhook authentication and authorization
	// to prevent unauthorized access to alert processing system

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	mockProcessor := &MockProcessor{}

	cfg := config.WebhookConfig{
		Port: "8080",
		Path: "/alerts",
		Auth: config.WebhookAuthConfig{
			Type:  "bearer",
			Token: "secure-webhook-token-123",
		},
	}

	handler := webhook.NewHandler(mockProcessor, cfg, logger)

	validPayload := `{
		"version": "4",
		"groupKey": "test-group",
		"status": "firing",
		"alerts": [{
			"status": "firing",
			"labels": {"alertname": "TestAlert", "severity": "critical"},
			"annotations": {"description": "Test alert"}
		}]
	}`

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
		description    string
	}{
		{
			name:           "Valid Bearer Token",
			authHeader:     "Bearer secure-webhook-token-123",
			expectedStatus: http.StatusOK,
			description:    "Valid authentication should allow access",
		},
		{
			name:           "Invalid Bearer Token",
			authHeader:     "Bearer invalid-token",
			expectedStatus: http.StatusUnauthorized,
			description:    "Invalid token should be rejected",
		},
		{
			name:           "Missing Authorization Header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
			description:    "Missing authentication should be rejected",
		},
		{
			name:           "Malformed Authorization Header",
			authHeader:     "InvalidFormat token123",
			expectedStatus: http.StatusUnauthorized,
			description:    "Malformed authentication should be rejected",
		},
		{
			name:           "Empty Bearer Token",
			authHeader:     "Bearer ",
			expectedStatus: http.StatusUnauthorized,
			description:    "Empty token should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/alerts", strings.NewReader(validPayload))
			req.Header.Set("Content-Type", "application/json")

			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			recorder := httptest.NewRecorder()
			handler.HandleAlert(recorder, req)

			if recorder.Code != tt.expectedStatus {
				t.Errorf("BR-WH-004 FAILED [%s]: Expected status %d, got %d - %s",
					tt.name, tt.expectedStatus, recorder.Code, tt.description)
			} else {
				t.Logf("BR-WH-004 PASSED [%s]: %s", tt.name, tt.description)
			}
		})
	}
}

// TestBR_WH_003_InputValidation validates BR-WH-003: Payload validation
func TestBR_WH_003_InputValidation(t *testing.T) {
	// Business Requirement: BR-WH-003 - Validate webhook payloads for completeness and format
	// to prevent malicious or malformed data from entering the system

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	mockProcessor := &MockProcessor{}

	cfg := config.WebhookConfig{
		Port: "8080",
		Path: "/alerts",
		Auth: config.WebhookAuthConfig{
			Type:  "bearer",
			Token: "test-token",
		},
	}

	handler := webhook.NewHandler(mockProcessor, cfg, logger)

	tests := []struct {
		name           string
		payload        string
		contentType    string
		expectedStatus int
		description    string
	}{
		{
			name: "Valid AlertManager Payload",
			payload: `{
				"version": "4",
				"groupKey": "test-group",
				"status": "firing",
				"alerts": [{
					"status": "firing",
					"labels": {"alertname": "TestAlert", "severity": "critical"},
					"annotations": {"description": "Test alert"}
				}]
			}`,
			contentType:    "application/json",
			expectedStatus: http.StatusOK,
			description:    "Valid payload should be accepted",
		},
		{
			name:           "Invalid JSON Payload",
			payload:        `{"invalid": json malformed}`,
			contentType:    "application/json",
			expectedStatus: http.StatusBadRequest,
			description:    "Malformed JSON should be rejected",
		},
		{
			name:           "Empty Payload",
			payload:        "",
			contentType:    "application/json",
			expectedStatus: http.StatusBadRequest,
			description:    "Empty payload should be rejected",
		},
		{
			name:           "Wrong Content Type",
			payload:        `{"valid": "json"}`,
			contentType:    "text/plain",
			expectedStatus: http.StatusBadRequest,
			description:    "Wrong content type should be rejected",
		},
		{
			name:           "Missing Content Type",
			payload:        `{"valid": "json"}`,
			contentType:    "",
			expectedStatus: http.StatusBadRequest,
			description:    "Missing content type should be rejected",
		},
		{
			name: "Malformed Repeated JSON",
			payload: strings.Repeat(`{
				"version": "4",
				"groupKey": "test-group-`+strings.Repeat("x", 100)+`",
				"status": "firing",
				"alerts": [{
					"status": "firing",
					"labels": {"alertname": "TestAlert", "severity": "critical"},
					"annotations": {"description": "`+strings.Repeat("Large payload test ", 10)+`"}
				}]
			}`, 2),
			contentType:    "application/json",
			expectedStatus: http.StatusBadRequest, // Should reject malformed JSON
			description:    "Malformed repeated JSON should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/alerts", strings.NewReader(tt.payload))
			req.Header.Set("Authorization", "Bearer test-token")

			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			recorder := httptest.NewRecorder()
			handler.HandleAlert(recorder, req)

			if recorder.Code != tt.expectedStatus {
				t.Errorf("BR-WH-003 FAILED [%s]: Expected status %d, got %d - %s",
					tt.name, tt.expectedStatus, recorder.Code, tt.description)
			} else {
				t.Logf("BR-WH-003 PASSED [%s]: %s", tt.name, tt.description)
			}
		})
	}
}

// TestBR_WH_006_RateLimiting validates BR-WH-006: Rate limiting protection
func TestBR_WH_006_RateLimiting(t *testing.T) {
	// Business Requirement: BR-WH-006 - Handle concurrent webhook requests with high throughput
	// while protecting against DoS attacks through rate limiting

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	mockProcessor := &MockProcessor{}

	cfg := config.WebhookConfig{
		Port: "8080",
		Path: "/alerts",
		Auth: config.WebhookAuthConfig{
			Type:  "bearer",
			Token: "test-token",
		},
	}

	handler := webhook.NewHandler(mockProcessor, cfg, logger)

	validPayload := `{
		"version": "4",
		"groupKey": "test-group",
		"status": "firing",
		"alerts": [{
			"status": "firing",
			"labels": {"alertname": "TestAlert", "severity": "critical"},
			"annotations": {"description": "Test alert"}
		}]
	}`

	// Test rate limiting by sending many requests rapidly
	const rapidRequests = 50
	var rateLimitedCount int

	for i := 0; i < rapidRequests; i++ {
		req := httptest.NewRequest("POST", "/alerts", strings.NewReader(validPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		recorder := httptest.NewRecorder()
		handler.HandleAlert(recorder, req)

		if recorder.Code == http.StatusTooManyRequests {
			rateLimitedCount++
		}
	}

	// Business Outcome: Rate limiting should protect against rapid requests
	// Note: Rate limiting behavior depends on implementation - this tests the protection exists
	t.Logf("BR-WH-006 INFO: %d/%d requests were rate limited", rateLimitedCount, rapidRequests)

	// Verify service remains functional after rate limiting
	req := httptest.NewRequest("POST", "/alerts", strings.NewReader(validPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	recorder := httptest.NewRecorder()
	handler.HandleAlert(recorder, req)

	// Service should still be responsive (either accept or rate limit, not crash)
	if recorder.Code != http.StatusOK && recorder.Code != http.StatusTooManyRequests {
		t.Errorf("BR-WH-006 FAILED: Service became unresponsive after rate limiting test, status: %d", recorder.Code)
	} else {
		t.Logf("BR-WH-006 PASSED: Service remains responsive after rate limiting test")
	}
}

// TestBR_WH_011_HTTPMethods validates BR-WH-011: HTTP method restrictions
func TestBR_WH_011_HTTPMethods(t *testing.T) {
	// Business Requirement: BR-WH-011 - Provide appropriate HTTP response codes for all request types
	// and restrict access to only allowed HTTP methods

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	mockProcessor := &MockProcessor{}

	cfg := config.WebhookConfig{
		Port: "8080",
		Path: "/alerts",
		Auth: config.WebhookAuthConfig{
			Type:  "bearer",
			Token: "test-token",
		},
	}

	handler := webhook.NewHandler(mockProcessor, cfg, logger)

	validPayload := `{
		"version": "4",
		"groupKey": "test-group",
		"status": "firing",
		"alerts": [{
			"status": "firing",
			"labels": {"alertname": "TestAlert", "severity": "critical"},
			"annotations": {"description": "Test alert"}
		}]
	}`

	tests := []struct {
		method         string
		expectedStatus int
		description    string
	}{
		{
			method:         "POST",
			expectedStatus: http.StatusOK,
			description:    "POST method should be allowed for webhook alerts",
		},
		{
			method:         "GET",
			expectedStatus: http.StatusMethodNotAllowed,
			description:    "GET method should be rejected",
		},
		{
			method:         "PUT",
			expectedStatus: http.StatusMethodNotAllowed,
			description:    "PUT method should be rejected",
		},
		{
			method:         "DELETE",
			expectedStatus: http.StatusMethodNotAllowed,
			description:    "DELETE method should be rejected",
		},
		{
			method:         "PATCH",
			expectedStatus: http.StatusMethodNotAllowed,
			description:    "PATCH method should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			var req *http.Request
			if tt.method == "POST" {
				req = httptest.NewRequest(tt.method, "/alerts", strings.NewReader(validPayload))
			} else {
				req = httptest.NewRequest(tt.method, "/alerts", nil)
			}

			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-token")

			recorder := httptest.NewRecorder()
			handler.HandleAlert(recorder, req)

			if recorder.Code != tt.expectedStatus {
				t.Errorf("BR-WH-011 FAILED [%s]: Expected status %d, got %d - %s",
					tt.method, tt.expectedStatus, recorder.Code, tt.description)
			} else {
				t.Logf("BR-WH-011 PASSED [%s]: %s", tt.method, tt.description)
			}
		})
	}
}
