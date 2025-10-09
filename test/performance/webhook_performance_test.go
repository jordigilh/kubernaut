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

package performance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/integration/processor"
	"github.com/jordigilh/kubernaut/pkg/integration/webhook"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// MockProcessor for performance testing
type MockProcessor struct {
	processDelay time.Duration
	mu           sync.RWMutex
	callCount    int
}

func (m *MockProcessor) ProcessAlert(ctx context.Context, alert types.Alert) error {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()

	if m.processDelay > 0 {
		time.Sleep(m.processDelay)
	}
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

// TestBR_PERF_001_ResponseTime validates BR-PERF-001: Process webhook requests within 2 seconds
func TestBR_PERF_001_ResponseTime(t *testing.T) {
	// Business Requirement: BR-PERF-001 - Process webhook requests within 2 seconds
	// to meet performance SLA and prevent AlertManager timeout

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	mockProcessor := &MockProcessor{processDelay: 100 * time.Millisecond}

	cfg := config.WebhookConfig{
		Port: "8080",
		Path: "/alerts",
		Auth: config.WebhookAuthConfig{
			Type:  "bearer",
			Token: "test-token",
		},
	}

	handler := webhook.NewHandler(mockProcessor, cfg, logger)

	// Create test alert payload
	alertPayload := createTestAlertManagerPayload()

	// Performance Test: Measure response time
	start := time.Now()

	req := httptest.NewRequest("POST", "/alerts", bytes.NewBuffer(alertPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	recorder := httptest.NewRecorder()
	handler.HandleAlert(recorder, req)

	duration := time.Since(start)

	// Business Outcome: Response time meets SLA
	if duration > 2*time.Second {
		t.Errorf("BR-PERF-001 FAILED: Response time %v exceeds 2 second SLA", duration)
	}

	if recorder.Code != http.StatusOK {
		t.Errorf("BR-PERF-001 FAILED: Expected status 200, got %d", recorder.Code)
	}

	t.Logf("BR-PERF-001 PASSED: Response time %v (< 2s SLA)", duration)
}

// TestBR_PERF_002_ConcurrentRequests validates BR-PERF-002: Handle 1000 concurrent webhook requests
func TestBR_PERF_002_ConcurrentRequests(t *testing.T) {
	// Business Requirement: BR-PERF-002 - Handle 1000 concurrent webhook requests
	// to support high-throughput alert processing

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	mockProcessor := &MockProcessor{processDelay: 10 * time.Millisecond}

	cfg := config.WebhookConfig{
		Port: "8080",
		Path: "/alerts",
		Auth: config.WebhookAuthConfig{
			Type:  "bearer",
			Token: "test-token",
		},
	}

	handler := webhook.NewHandler(mockProcessor, cfg, logger)

	// Performance Test: 1000 concurrent requests
	const concurrentRequests = 1000
	var wg sync.WaitGroup
	var successCount int32
	var mu sync.Mutex

	alertPayload := createTestAlertManagerPayload()

	start := time.Now()

	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func(requestID int) {
			defer wg.Done()

			req := httptest.NewRequest("POST", "/alerts", bytes.NewBuffer(alertPayload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("X-Request-ID", fmt.Sprintf("perf-test-%d", requestID))

			recorder := httptest.NewRecorder()
			handler.HandleAlert(recorder, req)

			if recorder.Code == http.StatusOK {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	// Business Outcome: All concurrent requests handled successfully
	if successCount != concurrentRequests {
		t.Errorf("BR-PERF-002 FAILED: Only %d/%d requests succeeded", successCount, concurrentRequests)
	}

	// Business Outcome: Reasonable processing time for concurrent load
	if duration > 30*time.Second {
		t.Errorf("BR-PERF-002 FAILED: Total processing time %v exceeds reasonable threshold", duration)
	}

	avgResponseTime := duration / time.Duration(concurrentRequests)
	t.Logf("BR-PERF-002 PASSED: %d concurrent requests processed in %v (avg: %v per request)",
		concurrentRequests, duration, avgResponseTime)
}

// TestBR_PERF_003_HTTPProcessorClientPerformance validates processor client performance
func TestBR_PERF_003_HTTPProcessorClientPerformance(t *testing.T) {
	// Business Requirement: HTTP processor client must maintain performance
	// under load while providing resilience features

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Create mock processor service
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate processing time
		time.Sleep(50 * time.Millisecond)

		response := processor.ProcessAlertResponse{
			Success:         true,
			ProcessingTime:  "50ms",
			ActionsExecuted: 1,
			Confidence:      0.85,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	client := processor.NewHTTPProcessorClient(mockServer.URL, logger)

	// Performance Test: Multiple sequential requests
	const numRequests = 100
	start := time.Now()

	for i := 0; i < numRequests; i++ {
		alert := types.Alert{
			Name:     fmt.Sprintf("PerfTestAlert-%d", i),
			Severity: "warning",
			Status:   "firing",
		}

		err := client.ProcessAlert(context.Background(), alert)
		if err != nil {
			t.Errorf("Request %d failed: %v", i, err)
		}
	}

	duration := time.Since(start)
	avgRequestTime := duration / time.Duration(numRequests)

	// Business Outcome: Reasonable performance maintained
	if avgRequestTime > 200*time.Millisecond {
		t.Errorf("BR-PERF-003 FAILED: Average request time %v exceeds threshold", avgRequestTime)
	}

	t.Logf("BR-PERF-003 PASSED: %d requests processed in %v (avg: %v per request)",
		numRequests, duration, avgRequestTime)
}

// Helper function to create test AlertManager payload
func createTestAlertManagerPayload() []byte {
	payload := map[string]interface{}{
		"version":  "4",
		"groupKey": "test-group",
		"status":   "firing",
		"receiver": "kubernaut-webhook",
		"alerts": []map[string]interface{}{
			{
				"status": "firing",
				"labels": map[string]string{
					"alertname": "HighCPUUsage",
					"severity":  "critical",
					"instance":  "server-01",
				},
				"annotations": map[string]string{
					"description": "CPU usage is above 90%",
					"summary":     "High CPU usage detected",
				},
				"startsAt":     time.Now().Format(time.RFC3339),
				"generatorURL": "http://prometheus:9090/graph",
				"fingerprint":  "test-fingerprint-123",
			},
		},
	}

	data, _ := json.Marshal(payload)
	return data
}
