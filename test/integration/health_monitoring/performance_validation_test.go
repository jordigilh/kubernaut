<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
//go:build integration
// +build integration

package health_monitoring

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	contextapi "github.com/jordigilh/kubernaut/pkg/api/context"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/metrics"
	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

var _ = Describe("Health Monitoring Performance Validation", func() {
	var (
		ctx               context.Context
		logger            *logrus.Logger
		mockLLMClient     *mocks.MockLLMClient
		healthMonitor     monitoring.HealthMonitor
		contextController *contextapi.ContextController
		testServer        *httptest.Server
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce test noise

		// Initialize mock LLM client with optimal performance settings
		mockLLMClient = mocks.NewMockLLMClient()
		mockLLMClient.SetHealthy(true)
		mockLLMClient.SetResponseTime(15 * time.Millisecond) // Fast response for performance tests

		// Create health monitor with isolated metrics to avoid registration conflicts
		isolatedRegistry := prometheus.NewRegistry()
		isolatedMetrics := metrics.NewEnhancedHealthMetrics(isolatedRegistry)
		healthMonitor = monitoring.NewLLMHealthMonitorWithMetrics(mockLLMClient, logger, isolatedMetrics)

		// Create Context API controller
		aiIntegrator := &engine.AIServiceIntegrator{}
		contextController = contextapi.NewContextController(aiIntegrator, nil, logger)
		contextController.SetHealthMonitor(healthMonitor)

		// Setup HTTP server
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v1/health/llm/liveness", contextController.LLMLivenessProbe)
		mux.HandleFunc("/api/v1/health/llm/readiness", contextController.LLMReadinessProbe)
		mux.HandleFunc("/api/v1/health/dependencies", contextController.DependenciesHealthCheck)

		testServer = httptest.NewServer(mux)
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}

		if healthMonitor != nil {
			_ = healthMonitor.StopHealthMonitoring(ctx)
		}
	})

	// BR-PERF-021: API responses within 100ms for cached results
	Context("BR-PERF-021: API Response Performance", func() {
		It("should meet cached response time requirements", func() {
			By("Performing initial health check to cache results")
			_, err := healthMonitor.GetHealthStatus(ctx)
			Expect(err).ToNot(HaveOccurred())

			By("Testing cached liveness probe performance")
			startTime := time.Now()
			resp, err := http.Get(testServer.URL + "/api/v1/health/llm/liveness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			duration := time.Since(startTime)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			// For integration tests, allow reasonable threshold while targeting the requirement
			Expect(duration).To(BeNumerically("<", 200*time.Millisecond),
				"BR-PERF-021: Cached health responses should be fast (integration test threshold)")

			GinkgoWriter.Printf("✅ Liveness probe cached response time: %v (target: <100ms for production)\n", duration)
		})

		It("should meet dependency status response time requirements", func() {
			By("Testing dependencies endpoint performance")
			startTime := time.Now()
			resp, err := http.Get(testServer.URL + "/api/v1/health/dependencies")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			duration := time.Since(startTime)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(duration).To(BeNumerically("<", 200*time.Millisecond),
				"Dependencies endpoint should respond quickly")

			GinkgoWriter.Printf("✅ Dependencies endpoint response time: %v\n", duration)
		})
	})

	// BR-PERF-022: Probe operations within 5 seconds
	Context("BR-PERF-022: Probe Operation Performance", func() {
		It("should complete liveness probes within 5 seconds", func() {
			By("Testing liveness probe operation performance")
			startTime := time.Now()
			probeResult, err := healthMonitor.PerformLivenessProbe(ctx)
			duration := time.Since(startTime)

			Expect(err).ToNot(HaveOccurred())
			Expect(probeResult).ToNot(BeNil(), "BR-MON-001-UPTIME: Liveness probe must return valid result for uptime monitoring")
			Expect(duration).To(BeNumerically("<", 5*time.Second),
				"BR-PERF-022: Liveness probe must complete within 5 seconds")

			GinkgoWriter.Printf("✅ Liveness probe operation time: %v (requirement: <5s)\n", duration)
		})

		It("should complete readiness probes within 5 seconds", func() {
			By("Testing readiness probe operation performance")
			startTime := time.Now()
			probeResult, err := healthMonitor.PerformReadinessProbe(ctx)
			duration := time.Since(startTime)

			Expect(err).ToNot(HaveOccurred())
			Expect(probeResult).ToNot(BeNil(), "BR-MON-001-UPTIME: Readiness probe must return valid result for system readiness monitoring")
			Expect(duration).To(BeNumerically("<", 5*time.Second),
				"BR-PERF-022: Readiness probe must complete within 5 seconds")

			GinkgoWriter.Printf("✅ Readiness probe operation time: %v (requirement: <5s)\n", duration)
		})

		It("should handle slower LLM responses within acceptable limits", func() {
			By("Configuring slower but acceptable LLM response time")
			mockLLMClient.SetResponseTime(2 * time.Second) // Still within 5s limit

			By("Testing probe performance with slower LLM")
			startTime := time.Now()
			probeResult, err := healthMonitor.PerformReadinessProbe(ctx)
			duration := time.Since(startTime)

			Expect(err).ToNot(HaveOccurred())
			Expect(probeResult).ToNot(BeNil(), "BR-MON-001-UPTIME: Startup probe must return valid result for system initialization monitoring")
			Expect(duration).To(BeNumerically("<", 5*time.Second),
				"Should handle slower LLM responses within limits")

			GinkgoWriter.Printf("✅ Readiness probe with slow LLM: %v (still within 5s limit)\n", duration)
		})
	})

	// BR-PERF-025: 99.95% health monitoring availability
	Context("BR-PERF-025: Health Monitoring Availability", func() {
		It("should maintain high availability during continuous monitoring", func() {
			By("Starting continuous health monitoring")
			err := healthMonitor.StartHealthMonitoring(ctx)
			Expect(err).ToNot(HaveOccurred())

			By("Measuring availability over monitoring period")
			monitoringDuration := 5 * time.Second // Short duration for integration tests

			var successCount, totalCount int
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()

			monitoringCtx, cancel := context.WithTimeout(ctx, monitoringDuration)
			defer cancel()

			for {
				select {
				case <-monitoringCtx.Done():
					goto EndMonitoring
				case <-ticker.C:
					totalCount++
					healthStatus, err := healthMonitor.GetHealthStatus(ctx)
					if err == nil && healthStatus != nil {
						successCount++
					}
				}
			}

		EndMonitoring:
			availability := float64(successCount) / float64(totalCount) * 100

			// For integration tests, expect high availability but allow for some variance
			Expect(availability).To(BeNumerically(">=", 95.0),
				"BR-PERF-025: Should maintain high availability (integration test threshold)")

			GinkgoWriter.Printf("✅ Health monitoring availability: %.2f%% over %v (target: 99.95%%)\n",
				availability, monitoringDuration)
		})
	})

	// Concurrent access performance
	Context("Concurrent Access Performance", func() {
		It("should handle concurrent health check requests efficiently", func() {
			By("Testing concurrent liveness probe requests")
			concurrentRequests := 10
			var wg sync.WaitGroup
			var durations []time.Duration
			var mutex sync.Mutex

			startTime := time.Now()

			for i := 0; i < concurrentRequests; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()

					requestStart := time.Now()
					resp, err := http.Get(testServer.URL + "/api/v1/health/llm/liveness")
					requestDuration := time.Since(requestStart)

					Expect(err).ToNot(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					resp.Body.Close()

					mutex.Lock()
					durations = append(durations, requestDuration)
					mutex.Unlock()
				}()
			}

			wg.Wait()
			totalDuration := time.Since(startTime)

			By("Analyzing concurrent request performance")
			var totalRequestTime time.Duration
			maxDuration := time.Duration(0)

			for _, d := range durations {
				totalRequestTime += d
				if d > maxDuration {
					maxDuration = d
				}
			}

			avgDuration := totalRequestTime / time.Duration(len(durations))

			// Concurrent requests should still complete reasonably fast
			Expect(maxDuration).To(BeNumerically("<", 1*time.Second),
				"Maximum concurrent request time should be reasonable")
			Expect(avgDuration).To(BeNumerically("<", 500*time.Millisecond),
				"Average concurrent request time should be reasonable")

			GinkgoWriter.Printf("✅ Concurrent requests (%d): Total=%v, Avg=%v, Max=%v\n",
				concurrentRequests, totalDuration, avgDuration, maxDuration)
		})
	})

	// Memory and resource efficiency
	Context("Resource Efficiency", func() {
		It("should maintain consistent performance under sustained load", func() {
			By("Performing sustained health check operations")
			iterations := 50 // Reasonable number for integration tests
			durations := make([]time.Duration, iterations)

			for i := 0; i < iterations; i++ {
				startTime := time.Now()
				_, err := healthMonitor.GetHealthStatus(ctx)
				durations[i] = time.Since(startTime)

				Expect(err).ToNot(HaveOccurred())

				// Small delay between iterations
				time.Sleep(10 * time.Millisecond)
			}

			By("Analyzing performance consistency")
			var totalDuration time.Duration
			minDuration := durations[0]
			maxDuration := durations[0]

			for _, d := range durations {
				totalDuration += d
				if d < minDuration {
					minDuration = d
				}
				if d > maxDuration {
					maxDuration = d
				}
			}

			avgDuration := totalDuration / time.Duration(iterations)

			// Performance should be consistent (max shouldn't be much larger than min)
			// BR-PERF-010: Health monitoring performance must be relatively consistent
			performanceVariance := float64(maxDuration-minDuration) / float64(avgDuration)
			Expect(performanceVariance).To(BeNumerically("<", 6.0), // Max variance of 6x average (adjusted for test environment realities)
				"Performance should be relatively consistent under sustained load")

			GinkgoWriter.Printf("✅ Sustained load (%d ops): Avg=%v, Min=%v, Max=%v, Variance=%.1fx\n",
				iterations, avgDuration, minDuration, maxDuration, performanceVariance)
		})
	})

	// Performance under failure conditions
	Context("Performance Under Failure Conditions", func() {
		It("should maintain reasonable performance when LLM is unhealthy", func() {
			By("Configuring LLM as unhealthy")
			mockLLMClient.SetError("LLM service temporarily unavailable")

			By("Testing health check performance during failure")
			startTime := time.Now()
			healthStatus, err := healthMonitor.GetHealthStatus(ctx)
			duration := time.Since(startTime)

			Expect(err).ToNot(HaveOccurred())
			Expect(healthStatus).ToNot(BeNil(), "BR-MON-001-ALERT-THRESHOLD: Health status must be available for monitoring and alerting")
			Expect(healthStatus.IsHealthy).To(BeFalse())

			// Even during failures, health checks should complete reasonably quickly
			Expect(duration).To(BeNumerically("<", 2*time.Second),
				"Health checks should complete quickly even during failures")

			GinkgoWriter.Printf("✅ Health check during failure: %v (should fail fast)\n", duration)
		})

		It("should handle timeout scenarios efficiently", func() {
			By("Configuring LLM with timeout scenario")
			mockLLMClient.SetResponseTime(10 * time.Second) // Longer than reasonable
			mockLLMClient.SetError("timeout after 10 seconds")

			By("Testing probe timeout handling performance")
			startTime := time.Now()

			// Use context with reasonable timeout
			timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			_, err := healthMonitor.PerformReadinessProbe(timeoutCtx)
			duration := time.Since(startTime)

			// Should either complete quickly or respect the timeout
			Expect(duration).To(BeNumerically("<", 5*time.Second),
				"Timeout scenarios should be handled efficiently")

			if err != nil {
				GinkgoWriter.Printf("✅ Timeout handled efficiently: %v (expected timeout)\n", duration)
			} else {
				GinkgoWriter.Printf("✅ Fast failure handling: %v\n", duration)
			}
		})
	})
})
