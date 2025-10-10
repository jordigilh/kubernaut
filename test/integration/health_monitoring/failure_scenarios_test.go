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
	"errors"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	contextapi "github.com/jordigilh/kubernaut/pkg/api/context"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/metrics"
	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

var _ = Describe("Health Monitoring Failure Scenarios", func() {
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

		// Initialize mock LLM client
		mockLLMClient = mocks.NewMockLLMClient()
		mockLLMClient.SetHealthy(true)
		mockLLMClient.SetResponseTime(25 * time.Millisecond)

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

	// Basic LLM unavailability scenarios
	Context("LLM Service Unavailability", func() {
		It("should handle complete LLM service unavailability", func() {
			By("Simulating complete LLM service failure")
			mockLLMClient.SetError("connection refused: LLM service completely unavailable")

			By("Testing liveness probe during service unavailability")
			resp, err := http.Get(testServer.URL + "/api/v1/health/llm/liveness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable),
				"Liveness probe should return 503 when LLM service is unavailable")

			By("Testing readiness probe during service unavailability")
			resp, err = http.Get(testServer.URL + "/api/v1/health/llm/readiness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable),
				"Readiness probe should return 503 when LLM service is unavailable")

			GinkgoWriter.Printf("✅ Complete LLM service unavailability handled correctly\n")
		})

		It("should handle intermittent LLM connectivity issues", func() {
			By("Simulating intermittent connectivity")
			mockLLMClient.SetError("timeout: intermittent network issues")

			By("Testing multiple consecutive probe calls")
			for i := 0; i < 3; i++ {
				resp, err := http.Get(testServer.URL + "/api/v1/health/llm/liveness")
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))
				time.Sleep(100 * time.Millisecond)
			}

			GinkgoWriter.Printf("✅ Intermittent connectivity issues handled consistently\n")
		})
	})

	// Network failure scenarios
	Context("Network Failure Scenarios", func() {
		It("should handle network timeout scenarios", func() {
			By("Simulating network timeout")
			mockLLMClient.SetError("network timeout after 30 seconds")
			mockLLMClient.SetResponseTime(35 * time.Second) // Longer than typical timeout

			By("Testing probe timeout handling")
			startTime := time.Now()
			resp, err := http.Get(testServer.URL + "/api/v1/health/llm/liveness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			duration := time.Since(startTime)

			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))
			// Should fail quickly rather than waiting for full timeout
			Expect(duration).To(BeNumerically("<", 10*time.Second),
				"Probe should fail quickly on network timeout")

			GinkgoWriter.Printf("✅ Network timeout handled efficiently in %v\n", duration)
		})

		It("should handle DNS resolution failures", func() {
			By("Simulating DNS resolution failure")
			mockLLMClient.SetError("DNS resolution failed: no such host")

			By("Testing probe behavior with DNS failures")
			resp, err := http.Get(testServer.URL + "/api/v1/health/llm/liveness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))

			GinkgoWriter.Printf("✅ DNS resolution failure handled correctly\n")
		})
	})

	// LLM model specific failures
	Context("LLM Model Failure Scenarios", func() {
		It("should handle model loading failures", func() {
			By("Simulating model loading failure")
			mockLLMClient.SetError("model loading failed: 20B model not available")

			By("Testing readiness probe during model loading failure")
			resp, err := http.Get(testServer.URL + "/api/v1/health/llm/readiness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable),
				"Readiness should fail when model is not available")

			GinkgoWriter.Printf("✅ Model loading failure handled correctly\n")
		})

		It("should handle model inference failures", func() {
			By("Simulating model inference failure")
			mockLLMClient.SetError("inference failed: model returned empty response")

			By("Testing readiness probe during inference failure")
			resp, err := http.Get(testServer.URL + "/api/v1/health/llm/readiness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable),
				"Readiness should fail when model inference fails")

			GinkgoWriter.Printf("✅ Model inference failure handled correctly\n")
		})
	})

	// Resource exhaustion scenarios
	Context("Resource Exhaustion Scenarios", func() {
		It("should handle memory exhaustion scenarios", func() {
			By("Simulating memory exhaustion")
			mockLLMClient.SetError("out of memory: 20B model requires more resources")

			By("Testing system behavior under memory pressure")
			resp, err := http.Get(testServer.URL + "/api/v1/health/llm/readiness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))

			GinkgoWriter.Printf("✅ Memory exhaustion scenario handled correctly\n")
		})

		It("should handle GPU resource unavailability", func() {
			By("Simulating GPU resource unavailability")
			mockLLMClient.SetError("GPU unavailable: all GPU resources allocated")

			By("Testing probe behavior without GPU resources")
			resp, err := http.Get(testServer.URL + "/api/v1/health/llm/readiness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))

			GinkgoWriter.Printf("✅ GPU resource unavailability handled correctly\n")
		})
	})

	// Recovery scenarios (BR-HEALTH-027, BR-HEALTH-028)
	Context("BR-HEALTH-027/028: Failure and Recovery Thresholds", func() {
		It("should implement failure threshold logic", func() {
			By("Simulating consecutive failures to reach threshold")
			mockLLMClient.SetError("persistent failure for threshold testing")

			// Configure health monitor to start continuous monitoring
			err := healthMonitor.StartHealthMonitoring(ctx)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for failure threshold to be reached")
			// Health monitor runs checks every 30s by default, but for testing we check multiple times
			// Force multiple health checks to accumulate failures
			var finalHealthStatus *types.HealthStatus
			Eventually(func() bool {
				// Perform multiple GetHealthStatus calls to simulate continuous monitoring
				for i := 0; i < 5; i++ {
					var err error
					finalHealthStatus, err = healthMonitor.GetHealthStatus(ctx)
					if err != nil {
						continue
					}
					// Small delay between checks to simulate monitoring interval
					time.Sleep(100 * time.Millisecond)
				}

				// Check if the system has become unhealthy due to consecutive failures
				// This tests the business requirement: failures accumulate until threshold
				return finalHealthStatus != nil && !finalHealthStatus.IsHealthy
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"BR-HEALTH-027: Should reach failure threshold and become unhealthy")

			By("Verifying failure details are captured")
			Expect(finalHealthStatus).ToNot(BeNil(), "BR-MON-001-ALERT-THRESHOLD: Health monitoring failure scenarios must return valid alerts for monitoring requirements")
			Expect(finalHealthStatus.IsHealthy).To(BeFalse(),
				"BR-HEALTH-027: System should be unhealthy after threshold")
			Expect(finalHealthStatus.BaseTimestampedResult.Error).To(ContainSubstring("persistent failure"),
				"BR-HEALTH-027: Should capture failure details")

			GinkgoWriter.Printf("✅ Failure threshold logic implemented correctly\n")
		})

		It("should implement recovery threshold logic", func() {
			By("Starting with failed state")
			mockLLMClient.SetError("initial failure state")

			err := healthMonitor.StartHealthMonitoring(ctx)
			Expect(err).ToNot(HaveOccurred())

			By("Transitioning to healthy state")
			time.Sleep(1 * time.Second) // Allow initial failure to be detected
			mockLLMClient.ClearState()
			mockLLMClient.SetHealthy(true)

			By("Waiting for recovery threshold to be reached")
			Eventually(func() bool {
				healthStatus, err := healthMonitor.GetHealthStatus(ctx)
				if err != nil {
					return false
				}
				return healthStatus.IsHealthy
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"BR-HEALTH-028: Should reach recovery threshold")

			GinkgoWriter.Printf("✅ Recovery threshold logic implemented correctly\n")
		})
	})

	// Partial failure scenarios
	Context("Partial Failure Scenarios", func() {
		It("should handle liveness success but readiness failure", func() {
			By("Configuring partial failure scenario")
			// Mock will succeed for basic liveness but fail for readiness (model interaction)
			mockLLMClient.SetHealthy(true) // Liveness will pass

			// Override readiness check to fail
			originalError := mockLLMClient.GetLastError()

			By("Testing liveness probe (should succeed)")
			resp, err := http.Get(testServer.URL + "/api/v1/health/llm/liveness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK),
				"Liveness should pass when basic connectivity works")

			By("Simulating readiness failure while liveness works")
			mockLLMClient.SetError("model readiness check failed: inference unavailable")

			resp, err = http.Get(testServer.URL + "/api/v1/health/llm/readiness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable),
				"Readiness should fail when model is not ready for inference")

			// Restore state
			if originalError == "" {
				mockLLMClient.ClearState()
			}

			GinkgoWriter.Printf("✅ Partial failure scenario (liveness OK, readiness fail) handled correctly\n")
		})
	})

	// Cascading failure scenarios
	Context("Cascading Failure Scenarios", func() {
		It("should handle health monitor initialization failure", func() {
			By("Creating Context API controller without health monitor")
			aiIntegrator := &engine.AIServiceIntegrator{}
			controllerWithoutHealth := contextapi.NewContextController(aiIntegrator, nil, logger)
			// Note: Not calling SetHealthMonitor to simulate initialization failure

			mux := http.NewServeMux()
			mux.HandleFunc("/api/v1/health/llm/liveness", controllerWithoutHealth.LLMLivenessProbe)
			tempServer := httptest.NewServer(mux)
			defer tempServer.Close()

			By("Testing endpoint behavior when health monitor is not initialized")
			resp, err := http.Get(tempServer.URL + "/api/v1/health/llm/liveness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable),
				"Should return 503 when health monitor is not initialized")

			GinkgoWriter.Printf("✅ Health monitor initialization failure handled gracefully\n")
		})
	})

	// Context cancellation scenarios
	Context("Context Cancellation Scenarios", func() {
		It("should handle context cancellation gracefully", func() {
			By("Creating context with short timeout")
			shortCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
			defer cancel()

			By("Configuring slow response to trigger context cancellation")
			mockLLMClient.SetResponseTime(200 * time.Millisecond)

			By("Testing probe behavior with context cancellation")
			probeResult, err := healthMonitor.PerformLivenessProbe(shortCtx)

			// Should either succeed quickly or handle cancellation gracefully
			if err != nil {
				Expect(errors.Is(err, context.DeadlineExceeded) ||
					errors.Is(err, context.Canceled)).To(BeTrue(),
					"Should handle context cancellation appropriately")
			} else {
				Expect(probeResult).ToNot(BeNil(), "BR-MON-001-ALERT-THRESHOLD: Health monitoring failure scenarios must return valid alerts for monitoring requirements")
			}

			GinkgoWriter.Printf("✅ Context cancellation handled gracefully\n")
		})
	})

	// Error propagation scenarios
	Context("Error Propagation Scenarios", func() {
		It("should propagate LLM errors with appropriate detail", func() {
			By("Configuring specific error scenario")
			specificError := "authentication failed: invalid API key for 20B model access"
			mockLLMClient.SetError(specificError)

			By("Testing error propagation through health status")
			healthStatus, err := healthMonitor.GetHealthStatus(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(healthStatus).ToNot(BeNil(), "BR-MON-001-ALERT-THRESHOLD: Health monitoring failure scenarios must return valid alerts for monitoring requirements")

			Expect(healthStatus.IsHealthy).To(BeFalse())
			// Error details should be preserved for debugging
			Expect(healthStatus.BaseTimestampedResult.Error).To(ContainSubstring("authentication failed"))

			GinkgoWriter.Printf("✅ Error propagation with appropriate detail verified\n")
		})
	})
})
