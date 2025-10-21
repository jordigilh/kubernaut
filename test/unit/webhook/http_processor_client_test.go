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

package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/integration/processor"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("BR-WH-004: HTTP Processor Client", func() {
	var (
		client     *processor.HTTPProcessorClient
		mockServer *httptest.Server
		logger     *logrus.Logger
		ctx        context.Context
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel)
		ctx = context.Background()

		// Mock processor service responses
		mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := processor.ProcessAlertResponse{ // This type doesn't exist yet - MUST fail
				Success:         true,
				ProcessingTime:  "2.5s",
				ActionsExecuted: 1,
				Confidence:      0.85,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))

		// This constructor doesn't exist yet - MUST fail
		client = processor.NewHTTPProcessorClient(mockServer.URL, logger)
	})

	AfterEach(func() {
		if mockServer != nil {
			mockServer.Close()
		}
	})

	Context("BR-WH-004: Processor Communication", func() {
		It("should successfully communicate with processor service", func() {
			alert := types.Alert{
				Name:      "TestAlert",
				Severity:  "critical",
				Status:    "firing",
				Namespace: "default",
			}

			err := client.ProcessAlert(ctx, alert)

			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle processor service failures with retry queue", func() {
			// Configure mock server to return errors
			mockServer.Close()
			mockServer = nil

			alert := types.Alert{
				Name:      "TestAlert",
				Severity:  "critical",
				Status:    "firing",
				Namespace: "default",
			}

			err := client.ProcessAlert(ctx, alert)

			// Should queue for retry, not return error immediately
			Expect(err).ToNot(HaveOccurred())
			Expect(client.GetRetryQueueSize()).To(Equal(1)) // Method doesn't exist - MUST fail
		})

		It("BR-WH-004.2: should maintain service availability during processor service failures", func() {
			// Business Requirement: Webhook service must remain available and handle
			// processor service failures gracefully without dropping alerts
			mockServer.Close()
			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))

			// Create new client with failing server
			client = processor.NewHTTPProcessorClient(mockServer.URL, logger)

			alert := types.Alert{
				Name:     "CriticalAlert",
				Severity: "critical",
				Status:   "firing",
			}

			// Business Outcome: Service continues accepting alerts despite processor failures
			for i := 0; i < 6; i++ {
				err := client.ProcessAlert(ctx, alert)
				// Service should handle failures gracefully (queue for retry)
				Expect(err).ToNot(HaveOccurred())
			}

			// Business Outcome: Service protects itself from cascading failures
			// (Implementation detail: circuit breaker, but we test the business outcome)
			err := client.ProcessAlert(ctx, alert)
			Expect(err).ToNot(HaveOccurred()) // Service remains available
		})

		It("BR-PERF-001: should process webhook requests within acceptable time limits", func() {
			// Business Requirement: Process webhook requests within 2 seconds
			// to meet performance SLA and prevent AlertManager timeout
			mockServer.Close()
			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(2 * time.Second) // Simulate slow processor service
				w.WriteHeader(http.StatusOK)
			}))

			client = processor.NewHTTPProcessorClient(mockServer.URL, logger)

			alert := types.Alert{
				Name:     "PerformanceTestAlert",
				Severity: "warning",
				Status:   "firing",
			}

			start := time.Now()
			err := client.ProcessAlert(ctx, alert)
			duration := time.Since(start)

			// Business Outcome: Service responds within acceptable time limits
			// (doesn't wait for slow processor, queues for retry instead)
			Expect(duration).To(BeNumerically("<", time.Second*2))
			Expect(err).ToNot(HaveOccurred()) // Service remains responsive
		})
	})

	Context("BR-WH-004: Service Resilience and Recovery", func() {
		It("BR-WH-004.3: should provide service health metrics for monitoring", func() {
			// Business Requirement: Service must provide operational metrics
			// for monitoring and alerting on service health
			metrics := client.GetCircuitBreakerMetrics()

			Expect(metrics).ToNot(BeNil())
			// Business Outcome: Metrics are available for operational monitoring
			Expect(metrics.FailureRate).To(BeNumerically(">=", 0))
			Expect(metrics.SuccessRate).To(BeNumerically(">=", 0))
		})

		It("BR-WH-004.4: should automatically recover from processor service outages", func() {
			// Business Requirement: Service must automatically recover when
			// processor service becomes available again
			mockServer.Close()
			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			client = processor.NewHTTPProcessorClient(mockServer.URL, logger)

			// Simulate service outage period
			for i := 0; i < 6; i++ {
				client.ProcessAlert(ctx, types.Alert{Name: "OutageAlert"})
			}

			// Wait for recovery period
			time.Sleep(100 * time.Millisecond)

			// Business Outcome: Service attempts recovery automatically
			// (Implementation uses half-open state, but we test business outcome)
			alert := types.Alert{Name: "RecoveryTestAlert"}
			err := client.ProcessAlert(ctx, alert)
			Expect(err).ToNot(HaveOccurred()) // Service attempts to recover
		})
	})

	Context("BR-WH-007: Alert Reliability and Delivery Guarantee", func() {
		It("BR-WH-007.1: should ensure no alerts are lost during processor service unavailability", func() {
			// Business Requirement: Webhook service must not lose alerts when
			// processor service is temporarily unavailable
			mockServer.Close() // Simulate processor service unavailability

			alert := types.Alert{
				Name:     "CriticalBusinessAlert",
				Severity: "critical",
				Status:   "firing",
			}

			err := client.ProcessAlert(ctx, alert)
			Expect(err).ToNot(HaveOccurred())

			// Business Outcome: Alert is preserved for later processing
			queueItems := client.GetRetryQueueItems()
			Expect(len(queueItems)).To(Equal(1))
			Expect(queueItems[0].Alert.Name).To(Equal("CriticalBusinessAlert"))
		})

		It("BR-WH-007.2: should successfully deliver alerts when processor service recovers", func() {
			// Business Requirement: Service must deliver queued alerts when
			// processor service becomes available again
			requestCount := 0
			mockServer.Close()
			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				if requestCount == 1 {
					// Simulate initial service unavailability
					w.WriteHeader(http.StatusInternalServerError)
				} else {
					// Service recovery - successful processing
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(processor.ProcessAlertResponse{Success: true})
				}
			}))
			client = processor.NewHTTPProcessorClient(mockServer.URL, logger)

			alert := types.Alert{
				Name:     "BusinessCriticalAlert",
				Severity: "critical",
				Status:   "firing",
			}

			// Alert queued during service unavailability
			client.ProcessAlert(ctx, alert)
			Expect(client.GetRetryQueueSize()).To(Equal(1))

			// Wait for retry processing
			time.Sleep(20 * time.Millisecond)

			// Business Outcome: Alert successfully delivered after service recovery
			err := client.ProcessRetryQueue(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(client.GetRetryQueueSize()).To(Equal(0)) // Alert delivered
		})

		It("BR-WH-007.3: should handle persistent processor service failures gracefully", func() {
			// Business Requirement: Service must handle cases where processor
			// service remains unavailable for extended periods without system failure
			mockServer.Close() // Persistent service unavailability

			alert := types.Alert{
				Name:     "PersistentFailureAlert",
				Severity: "warning",
				Status:   "firing",
			}

			// Initial failure
			client.ProcessAlert(ctx, alert)
			Expect(client.GetRetryQueueSize()).To(Equal(1))

			// Simulate multiple retry attempts during persistent failure
			for i := 0; i < 4; i++ { // Exceed max retries
				time.Sleep(20 * time.Millisecond)
				client.ProcessRetryQueue(ctx)
			}

			// Business Outcome: System handles persistent failures gracefully
			// (moves to dead letter queue to prevent infinite retries)
			deadLetterItems := client.GetDeadLetterQueueItems()
			Expect(len(deadLetterItems)).To(Equal(1))
			Expect(client.GetRetryQueueSize()).To(Equal(0)) // No infinite retries
		})
	})
})

// Test helper functions
func createTestAlert() types.Alert {
	return types.Alert{
		Name:        "TestAlert",
		Status:      "firing",
		Severity:    "critical",
		Description: "Test alert for HTTP processor client",
		Namespace:   "default",
		Resource:    "test-pod",
		Labels: map[string]string{
			"alertname": "TestAlert",
			"severity":  "critical",
		},
		Annotations: map[string]string{
			"description": "Test alert description",
		},
		StartsAt: time.Now(),
	}
}
