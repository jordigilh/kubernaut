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

package middleware

import (
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	gatewayMetrics "github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	gatewayMiddleware "github.com/jordigilh/kubernaut/pkg/gateway/middleware"
)

var _ = Describe("HTTPMetrics Middleware", func() {
	var (
		metrics  *gatewayMetrics.Metrics
		registry *prometheus.Registry
		router   *chi.Mux
	)

	BeforeEach(func() {
		// Create custom registry for test isolation
		// Each test gets a fresh registry to avoid metric registration conflicts
		registry = prometheus.NewRegistry()
		metrics = gatewayMetrics.NewMetricsWithRegistry(registry)

		// Create router with middleware
		router = chi.NewRouter()
		router.Use(gatewayMiddleware.HTTPMetrics(metrics))
	})

	Describe("Request Duration Tracking", func() {
		It("should track request duration with correct labels", func() {
			// Arrange: Add test handler
			router.Get("/test", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("ok"))
			})

			// Act: Make request
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Assert: Verify response
			Expect(w.Code).To(Equal(http.StatusOK))

			// Assert: Verify metric recorded
			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			// Find the duration metric
			var found bool
			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_http_request_duration_seconds" {
					found = true
					Expect(mf.GetType()).To(Equal(dto.MetricType_HISTOGRAM))

					// Verify labels
					metrics := mf.GetMetric()
					Expect(metrics).ToNot(BeEmpty())

					// Check first metric has correct labels
					labels := metrics[0].GetLabel()
					labelMap := make(map[string]string)
					for _, label := range labels {
						labelMap[label.GetName()] = label.GetValue()
					}

					Expect(labelMap["endpoint"]).To(Equal("/test"))
					Expect(labelMap["method"]).To(Equal("GET"))
					Expect(labelMap["status"]).To(Equal("200"))

					// Verify histogram has observations
					histogram := metrics[0].GetHistogram()
					Expect(histogram.GetSampleCount()).To(BeNumerically(">", 0))
				}
			}
			Expect(found).To(BeTrue(), "gateway_http_request_duration_seconds metric should exist")
		})

		It("should track different status codes", func() {
			// Arrange: Add handlers for different status codes
			router.Get("/ok", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			router.Get("/error", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			})

			// Act: Make requests
			req1 := httptest.NewRequest("GET", "/ok", nil)
			w1 := httptest.NewRecorder()
			router.ServeHTTP(w1, req1)

			req2 := httptest.NewRequest("GET", "/error", nil)
			w2 := httptest.NewRecorder()
			router.ServeHTTP(w2, req2)

			// Assert: Verify both recorded
			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var foundOK, foundError bool
			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_http_request_duration_seconds" {
					for _, metric := range mf.GetMetric() {
						labels := metric.GetLabel()
						for _, label := range labels {
							if label.GetName() == "status" {
								if label.GetValue() == "200" {
									foundOK = true
								}
								if label.GetValue() == "500" {
									foundError = true
								}
							}
						}
					}
				}
			}

			Expect(foundOK).To(BeTrue(), "Should track 200 status code")
			Expect(foundError).To(BeTrue(), "Should track 500 status code")
		})

		It("should track different HTTP methods", func() {
			// Arrange: Add handlers for different methods
			router.Get("/test", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			router.Post("/test", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
			})

			// Act: Make requests
			req1 := httptest.NewRequest("GET", "/test", nil)
			w1 := httptest.NewRecorder()
			router.ServeHTTP(w1, req1)

			req2 := httptest.NewRequest("POST", "/test", nil)
			w2 := httptest.NewRecorder()
			router.ServeHTTP(w2, req2)

			// Assert: Verify both methods tracked
			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var foundGET, foundPOST bool
			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_http_request_duration_seconds" {
					for _, metric := range mf.GetMetric() {
						labels := metric.GetLabel()
						for _, label := range labels {
							if label.GetName() == "method" {
								if label.GetValue() == "GET" {
									foundGET = true
								}
								if label.GetValue() == "POST" {
									foundPOST = true
								}
							}
						}
					}
				}
			}

			Expect(foundGET).To(BeTrue(), "Should track GET method")
			Expect(foundPOST).To(BeTrue(), "Should track POST method")
		})
	})

	Describe("Nil-Safe Behavior", func() {
		It("should handle nil metrics gracefully", func() {
			// Arrange: Router with nil metrics
			router := chi.NewRouter()
			router.Use(gatewayMiddleware.HTTPMetrics(nil))
			router.Get("/test", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Act: Make request (should not panic)
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()

			// Assert: Should not panic
			Expect(func() {
				router.ServeHTTP(w, req)
			}).ToNot(Panic())

			Expect(w.Code).To(Equal(http.StatusOK))
		})
	})
})

var _ = Describe("InFlightRequests Middleware", func() {
	var (
		metrics  *gatewayMetrics.Metrics
		registry *prometheus.Registry
		router   *chi.Mux
	)

	BeforeEach(func() {
		// Create custom registry for test isolation
		// Each test gets a fresh registry to avoid metric registration conflicts
		registry = prometheus.NewRegistry()
		metrics = gatewayMetrics.NewMetricsWithRegistry(registry)

		// Create router with middleware
		router = chi.NewRouter()
		router.Use(gatewayMiddleware.InFlightRequests(metrics))
	})

	Describe("In-Flight Request Tracking", func() {
		It("should increment gauge on request start", func() {
			// Arrange: Add blocking handler
			requestStarted := make(chan bool)
			requestComplete := make(chan bool)

			router.Get("/test", func(w http.ResponseWriter, r *http.Request) {
				requestStarted <- true
				<-requestComplete // Block until test signals
				w.WriteHeader(http.StatusOK)
			})

			// Act: Start request in background
			go func() {
				req := httptest.NewRequest("GET", "/test", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
			}()

			// Wait for request to start
			<-requestStarted

			// Assert: Gauge should be incremented
			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var found bool
			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_http_requests_in_flight" {
					found = true
					Expect(mf.GetType()).To(Equal(dto.MetricType_GAUGE))
					metrics := mf.GetMetric()
					Expect(metrics).ToNot(BeEmpty())
					Expect(metrics[0].GetGauge().GetValue()).To(Equal(float64(1)))
				}
			}
			Expect(found).To(BeTrue(), "gateway_http_requests_in_flight metric should exist")

			// Cleanup: Signal request to complete
			requestComplete <- true
		})

		It("should decrement gauge on request end", func() {
			// Arrange: Add simple handler
			router.Get("/test", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Act: Make request (completes immediately)
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Assert: Gauge should be back to 0
			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var found bool
			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_http_requests_in_flight" {
					found = true
					metrics := mf.GetMetric()
					Expect(metrics).ToNot(BeEmpty())
					Expect(metrics[0].GetGauge().GetValue()).To(Equal(float64(0)))
				}
			}
			Expect(found).To(BeTrue(), "gateway_http_requests_in_flight metric should exist")
		})
	})

	Describe("Nil-Safe Behavior", func() {
		It("should handle nil metrics gracefully", func() {
			// Arrange: Router with nil metrics
			router := chi.NewRouter()
			router.Use(gatewayMiddleware.InFlightRequests(nil))
			router.Get("/test", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Act: Make request (should not panic)
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()

			// Assert: Should not panic
			Expect(func() {
				router.ServeHTTP(w, req)
			}).ToNot(Panic())

			Expect(w.Code).To(Equal(http.StatusOK))
		})
	})
})
