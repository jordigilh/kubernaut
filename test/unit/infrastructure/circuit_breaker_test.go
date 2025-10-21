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

package infrastructure

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	infrahttp "github.com/jordigilh/kubernaut/pkg/infrastructure/http"
	"github.com/sirupsen/logrus"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCircuitBreaker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Circuit Breaker Suite")
}

var _ = Describe("Circuit Breaker", func() {

	// Business Requirement: BR-EXTERNAL-001 - External API circuit breakers with rate limiting
	Context("BR-EXTERNAL-001: External API Circuit Breaker and Rate Limiting", func() {
		var (
			circuitBreaker *infrahttp.CircuitBreaker
			logger         *logrus.Logger
			testServer     *httptest.Server
			successCount   int64
			failureCount   int64
		)

		BeforeEach(func() {
			logger = logrus.New()
			logger.SetLevel(logrus.ErrorLevel) // Reduce noise during tests

			// Reset counters
			atomic.StoreInt64(&successCount, 0)
			atomic.StoreInt64(&failureCount, 0)

			// Create test server with controllable responses
			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/success":
					atomic.AddInt64(&successCount, 1)
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("success"))
				case "/failure":
					atomic.AddInt64(&failureCount, 1)
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte("error"))
				case "/slow":
					time.Sleep(200 * time.Millisecond) // Slower than request timeout
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))

			// Create circuit breaker with test configuration
			config := &infrahttp.CircuitBreakerConfig{
				FailureThreshold:    3,
				RecoveryTimeout:     100 * time.Millisecond,
				SuccessThreshold:    2,
				RequestTimeout:      50 * time.Millisecond,
				RequestsPerSecond:   100, // Increase for tests
				BurstLimit:          50,  // Increase for tests
				HealthCheckInterval: 50 * time.Millisecond,
				HealthCheckPath:     "/health",
				EnableMetrics:       true,
				MetricsInterval:     100 * time.Millisecond,
			}

			circuitBreaker = infrahttp.NewCircuitBreaker("test-circuit", config, &http.Client{}, logger)
		})

		AfterEach(func() {
			if circuitBreaker != nil {
				circuitBreaker.Stop()
			}
			if testServer != nil {
				testServer.Close()
			}
		})

		It("should initialize with closed state and default configuration", func() {
			// Business Validation: Circuit breaker should start in closed state
			Expect(circuitBreaker.GetState()).To(Equal(infrahttp.StateClosed))
			Expect(circuitBreaker.IsHealthy()).To(BeTrue())

			metrics := circuitBreaker.GetMetrics()
			Expect(metrics.State).To(Equal(infrahttp.StateClosed))
			Expect(metrics.TotalRequests).To(Equal(int64(0)))
			Expect(metrics.HealthScore).To(BeNumerically(">=", 0.0))
		})

		It("should handle successful requests and maintain closed state", func() {
			// Act: Make successful requests
			for i := 0; i < 5; i++ {
				req, err := http.NewRequest("GET", testServer.URL+"/success", nil)
				Expect(err).ToNot(HaveOccurred())

				resp, err := circuitBreaker.Do(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				defer func() {
					if closeErr := resp.Body.Close(); closeErr != nil {
						// Log error in test context but don't fail the test
						GinkgoWriter.Printf("Warning: failed to close response body: %v\n", closeErr)
					}
				}()
			}

			// Business Validation: Should remain in closed state with good metrics
			Expect(circuitBreaker.GetState()).To(Equal(infrahttp.StateClosed))
			Expect(circuitBreaker.IsHealthy()).To(BeTrue())

			metrics := circuitBreaker.GetMetrics()
			Expect(metrics.TotalRequests).To(Equal(int64(5)))
			Expect(metrics.SuccessfulRequests).To(Equal(int64(5)))
			Expect(metrics.FailedRequests).To(Equal(int64(0)))
			Expect(metrics.HealthScore).To(BeNumerically(">", 0.8))
		})

		It("should open circuit after consecutive failures", func() {
			// Act: Make requests that will fail
			for i := 0; i < 3; i++ { // Exactly the threshold
				req, err := http.NewRequest("GET", testServer.URL+"/failure", nil)
				Expect(err).ToNot(HaveOccurred())

				resp, err := circuitBreaker.Do(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
				if closeErr := resp.Body.Close(); closeErr != nil {
					GinkgoWriter.Printf("Warning: failed to close response body: %v\n", closeErr)
				}
			}

			// Business Validation: Circuit should be open after threshold failures
			Expect(circuitBreaker.GetState()).To(Equal(infrahttp.StateOpen))
			Expect(circuitBreaker.IsHealthy()).To(BeFalse())

			metrics := circuitBreaker.GetMetrics()
			Expect(metrics.ConsecutiveFailures).To(BeNumerically(">=", 3))
			Expect(metrics.FailedRequests).To(BeNumerically(">=", 3))
		})

		It("should implement rate limiting correctly", func() {
			// Act: Make rapid requests beyond rate limit
			const rapidRequests = 20
			successfulRequests := 0
			rateLimitedRequests := 0

			for i := 0; i < rapidRequests; i++ {
				req, err := http.NewRequest("GET", testServer.URL+"/success", nil)
				Expect(err).ToNot(HaveOccurred())

				resp, err := circuitBreaker.Do(req)
				if err != nil && err.Error() == "rate limit exceeded" {
					rateLimitedRequests++
				} else {
					Expect(err).ToNot(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					if closeErr := resp.Body.Close(); closeErr != nil {
						GinkgoWriter.Printf("Warning: failed to close response body: %v\n", closeErr)
					}
					successfulRequests++
				}
			}

			// Business Validation: Rate limiting should prevent some requests
			Expect(rateLimitedRequests).To(BeNumerically(">", 0))
			Expect(successfulRequests).To(BeNumerically("<=", 15)) // Should be limited

			metrics := circuitBreaker.GetMetrics()
			Expect(metrics.RateLimitHits).To(BeNumerically(">", 0))
			Expect(metrics.RejectedRequests).To(BeNumerically(">", 0))
		})

		It("should transition to half-open after recovery timeout", func() {
			// Arrange: Open the circuit with failures
			for i := 0; i < 3; i++ {
				req, err := http.NewRequest("GET", testServer.URL+"/failure", nil)
				Expect(err).ToNot(HaveOccurred())

				resp, err := circuitBreaker.Do(req)
				Expect(err).ToNot(HaveOccurred())
				if closeErr := resp.Body.Close(); closeErr != nil {
					GinkgoWriter.Printf("Warning: failed to close response body: %v\n", closeErr)
				}
			}

			Expect(circuitBreaker.GetState()).To(Equal(infrahttp.StateOpen))

			// Act: Wait for recovery timeout
			time.Sleep(150 * time.Millisecond) // Longer than recovery timeout

			// Make a successful request to trigger half-open transition
			req, err := http.NewRequest("GET", testServer.URL+"/success", nil)
			Expect(err).ToNot(HaveOccurred())

			resp, err := circuitBreaker.Do(req)
			Expect(err).ToNot(HaveOccurred())
			if closeErr := resp.Body.Close(); closeErr != nil {
				GinkgoWriter.Printf("Warning: failed to close response body: %v\n", closeErr)
			}

			// Business Validation: Should be in half-open state
			Expect(circuitBreaker.GetState()).To(Equal(infrahttp.StateHalfOpen))
		})

		It("should close circuit after successful recovery in half-open state", func() {
			// Arrange: Put circuit in half-open state
			// First open it
			for i := 0; i < 3; i++ {
				req, err := http.NewRequest("GET", testServer.URL+"/failure", nil)
				Expect(err).ToNot(HaveOccurred())
				resp, err := circuitBreaker.Do(req)
				Expect(err).ToNot(HaveOccurred())
				if closeErr := resp.Body.Close(); closeErr != nil {
					GinkgoWriter.Printf("Warning: failed to close response body: %v\n", closeErr)
				}
			}

			// Wait for recovery and make successful request to get to half-open
			time.Sleep(150 * time.Millisecond)
			req, err := http.NewRequest("GET", testServer.URL+"/success", nil)
			Expect(err).ToNot(HaveOccurred())
			resp, err := circuitBreaker.Do(req)
			Expect(err).ToNot(HaveOccurred())
			if closeErr := resp.Body.Close(); closeErr != nil {
				GinkgoWriter.Printf("Warning: failed to close response body: %v\n", closeErr)
			}

			Expect(circuitBreaker.GetState()).To(Equal(infrahttp.StateHalfOpen))

			// Act: Make enough successful requests to close circuit
			for i := 0; i < 2; i++ { // Success threshold is 2
				req, err := http.NewRequest("GET", testServer.URL+"/success", nil)
				Expect(err).ToNot(HaveOccurred())
				resp, err := circuitBreaker.Do(req)
				Expect(err).ToNot(HaveOccurred())
				if closeErr := resp.Body.Close(); closeErr != nil {
					GinkgoWriter.Printf("Warning: failed to close response body: %v\n", closeErr)
				}
			}

			// Business Validation: Circuit should be closed after successful recovery
			Expect(circuitBreaker.GetState()).To(Equal(infrahttp.StateClosed))
			Expect(circuitBreaker.IsHealthy()).To(BeTrue())
		})

		It("should handle request timeouts correctly", func() {
			// Act: Make request to slow endpoint
			req, err := http.NewRequest("GET", testServer.URL+"/slow", nil)
			Expect(err).ToNot(HaveOccurred())

			start := time.Now()
			resp, err := circuitBreaker.Do(req)
			duration := time.Since(start)

			// Business Validation: Should timeout within configured limit
			Expect(err).To(HaveOccurred())
			Expect(resp).To(BeNil())
			Expect(duration).To(BeNumerically("~", 50*time.Millisecond, 30*time.Millisecond))

			metrics := circuitBreaker.GetMetrics()
			Expect(metrics.FailedRequests).To(BeNumerically(">", 0))
		})

		It("should handle concurrent requests safely", func() {
			// Act: Concurrent requests to test thread safety
			const numGoroutines = 20
			const requestsPerGoroutine = 25

			var wg sync.WaitGroup
			var successfulRequests int64
			var failedRequests int64
			var rateLimitedRequests int64

			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					for j := 0; j < requestsPerGoroutine; j++ {
						// Mix success and failure requests
						endpoint := "/success"
						if j%5 == 0 { // 20% failure rate
							endpoint = "/failure"
						}

						req, err := http.NewRequest("GET", testServer.URL+endpoint, nil)
						if err != nil {
							continue
						}

						resp, err := circuitBreaker.Do(req)
						if err != nil {
							if err.Error() == "rate limit exceeded" {
								atomic.AddInt64(&rateLimitedRequests, 1)
							} else {
								atomic.AddInt64(&failedRequests, 1)
							}
						} else {
							atomic.AddInt64(&successfulRequests, 1)
							if closeErr := resp.Body.Close(); closeErr != nil {
								GinkgoWriter.Printf("Warning: failed to close response body: %v\n", closeErr)
							}
						}

						time.Sleep(time.Microsecond * 50) // Small delay to prevent overwhelming
					}
				}(i)
			}

			// Wait for completion with timeout
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()

			select {
			case <-done:
				// Success
			case <-time.After(30 * time.Second):
				Fail("Test timed out - possible deadlock in circuit breaker")
			}

			// Business Validation: All operations should complete without corruption
			finalSuccessful := atomic.LoadInt64(&successfulRequests)
			finalFailed := atomic.LoadInt64(&failedRequests)
			finalRateLimited := atomic.LoadInt64(&rateLimitedRequests)

			Expect(finalSuccessful + finalFailed + finalRateLimited).To(BeNumerically(">", 0))

			metrics := circuitBreaker.GetMetrics()
			Expect(metrics.TotalRequests).To(BeNumerically(">", 0))

			// Verify metrics consistency
			totalFromMetrics := metrics.SuccessfulRequests + metrics.FailedRequests + metrics.RejectedRequests
			Expect(totalFromMetrics).To(Equal(metrics.TotalRequests))
		})

		It("should calculate accurate health scores", func() {
			// Arrange: Create mixed success/failure scenario
			successfulCount := 8
			failureCount := 2

			// Act: Make mixed requests
			for i := 0; i < successfulCount; i++ {
				req, err := http.NewRequest("GET", testServer.URL+"/success", nil)
				Expect(err).ToNot(HaveOccurred())
				resp, err := circuitBreaker.Do(req)
				Expect(err).ToNot(HaveOccurred())
				if closeErr := resp.Body.Close(); closeErr != nil {
					GinkgoWriter.Printf("Warning: failed to close response body: %v\n", closeErr)
				}
			}

			for i := 0; i < failureCount; i++ {
				req, err := http.NewRequest("GET", testServer.URL+"/failure", nil)
				Expect(err).ToNot(HaveOccurred())
				resp, err := circuitBreaker.Do(req)
				Expect(err).ToNot(HaveOccurred())
				if closeErr := resp.Body.Close(); closeErr != nil {
					GinkgoWriter.Printf("Warning: failed to close response body: %v\n", closeErr)
				}
			}

			// Business Validation: Health score should reflect success rate
			metrics := circuitBreaker.GetMetrics()
			expectedSuccessRate := float64(successfulCount) / float64(successfulCount+failureCount)

			Expect(metrics.HealthScore).To(BeNumerically("~", expectedSuccessRate, 0.1))
			Expect(metrics.HealthScore).To(BeNumerically(">", 0.7)) // Should be healthy
			Expect(circuitBreaker.IsHealthy()).To(BeTrue())
		})

		It("should reset circuit state correctly", func() {
			// Arrange: Open the circuit
			for i := 0; i < 3; i++ {
				req, err := http.NewRequest("GET", testServer.URL+"/failure", nil)
				Expect(err).ToNot(HaveOccurred())
				resp, err := circuitBreaker.Do(req)
				Expect(err).ToNot(HaveOccurred())
				if closeErr := resp.Body.Close(); closeErr != nil {
					GinkgoWriter.Printf("Warning: failed to close response body: %v\n", closeErr)
				}
			}

			Expect(circuitBreaker.GetState()).To(Equal(infrahttp.StateOpen))

			// Act: Reset circuit
			circuitBreaker.Reset()

			// Business Validation: Circuit should be closed and healthy
			Expect(circuitBreaker.GetState()).To(Equal(infrahttp.StateClosed))
			Expect(circuitBreaker.IsHealthy()).To(BeTrue())

			metrics := circuitBreaker.GetMetrics()
			Expect(metrics.ConsecutiveFailures).To(Equal(int32(0)))
			Expect(metrics.ConsecutiveSuccesses).To(Equal(int32(0)))
		})

		It("should provide accurate metrics reporting", func() {
			// Act: Perform various operations
			operations := []struct {
				path     string
				expected int
			}{
				{"/success", http.StatusOK},
				{"/success", http.StatusOK},
				{"/failure", http.StatusInternalServerError},
				{"/success", http.StatusOK},
			}

			for _, op := range operations {
				req, err := http.NewRequest("GET", testServer.URL+op.path, nil)
				Expect(err).ToNot(HaveOccurred())
				resp, err := circuitBreaker.Do(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(op.expected))
				if closeErr := resp.Body.Close(); closeErr != nil {
					GinkgoWriter.Printf("Warning: failed to close response body: %v\n", closeErr)
				}
			}

			// Business Validation: Metrics should be accurate
			metrics := circuitBreaker.GetMetrics()
			Expect(metrics.TotalRequests).To(Equal(int64(4)))
			Expect(metrics.SuccessfulRequests).To(Equal(int64(3)))
			Expect(metrics.FailedRequests).To(Equal(int64(1)))
			Expect(metrics.AverageResponseTime).To(BeNumerically(">", 0))
			Expect(metrics.LastSuccessTime).To(BeTemporally("~", time.Now(), time.Second))
			Expect(metrics.LastFailureTime).To(BeTemporally("~", time.Now(), time.Second))
		})
	})
})
