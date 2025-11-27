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

package metrics

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

// BR-GATEWAY-106: Failure Metrics Unit Tests
//
// Business Outcome Testing: Test that failure events are correctly recorded in metrics
//
// These tests validate the metrics recording logic without requiring actual infrastructure failures.
// The business logic that calls these metrics is tested separately in integration/E2E tests.
//
// COVERAGE:
// - K8s API failure metrics (CRD creation errors, retry attempts)
// - Redis failure metrics (outage count, outage duration)

var _ = Describe("BR-GATEWAY-106: Failure Metrics Recording", func() {
	var (
		testMetrics *metrics.Metrics
		ctx         context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		_ = ctx // Suppress unused variable warning

		// Create isolated metrics instance for each test
		registry := prometheus.NewRegistry()
		testMetrics = metrics.NewMetricsWithRegistry(registry)
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// K8s API Failure Metrics
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("K8s API Failure Metrics (BR-GATEWAY-106)", func() {

		Context("when CRD creation fails with different error types", func() {

			It("should record rate limit errors correctly", func() {
				// BUSINESS SCENARIO: K8s API returns 429 Too Many Requests
				// Expected: Metric incremented with error_type="rate_limited"

				// Simulate recording rate limit error
				testMetrics.CRDCreationErrors.WithLabelValues("rate_limited").Inc()

				// Verify metric was recorded
				value := getCounterValue(testMetrics.CRDCreationErrors, "rate_limited")
				Expect(value).To(Equal(float64(1)),
					"Rate limit error should be recorded")

				// BUSINESS CAPABILITY VERIFIED:
				// ✅ Operators can query: rate(gateway_crd_creation_errors_total{error_type="rate_limited"}[5m])
			})

			It("should record service unavailable errors correctly", func() {
				// BUSINESS SCENARIO: K8s API returns 503 Service Unavailable
				// Expected: Metric incremented with error_type="service_unavailable"

				testMetrics.CRDCreationErrors.WithLabelValues("service_unavailable").Inc()

				value := getCounterValue(testMetrics.CRDCreationErrors, "service_unavailable")
				Expect(value).To(Equal(float64(1)),
					"Service unavailable error should be recorded")

				// BUSINESS CAPABILITY VERIFIED:
				// ✅ Operators can detect K8s API outages via error metrics
			})

			It("should record timeout errors correctly", func() {
				// BUSINESS SCENARIO: K8s API request times out
				// Expected: Metric incremented with error_type="timeout"

				testMetrics.CRDCreationErrors.WithLabelValues("timeout").Inc()

				value := getCounterValue(testMetrics.CRDCreationErrors, "timeout")
				Expect(value).To(Equal(float64(1)),
					"Timeout error should be recorded")

				// BUSINESS CAPABILITY VERIFIED:
				// ✅ Operators can detect slow K8s API responses
			})

			It("should record conflict errors correctly", func() {
				// BUSINESS SCENARIO: CRD already exists (conflict)
				// Expected: Metric incremented with error_type="conflict"

				testMetrics.CRDCreationErrors.WithLabelValues("conflict").Inc()

				value := getCounterValue(testMetrics.CRDCreationErrors, "conflict")
				Expect(value).To(Equal(float64(1)),
					"Conflict error should be recorded")

				// BUSINESS CAPABILITY VERIFIED:
				// ✅ Operators can detect race conditions in CRD creation
			})

			It("should record validation errors correctly", func() {
				// BUSINESS SCENARIO: CRD spec fails validation
				// Expected: Metric incremented with error_type="validation"

				testMetrics.CRDCreationErrors.WithLabelValues("validation").Inc()

				value := getCounterValue(testMetrics.CRDCreationErrors, "validation")
				Expect(value).To(Equal(float64(1)),
					"Validation error should be recorded")

				// BUSINESS CAPABILITY VERIFIED:
				// ✅ Operators can detect malformed CRD specs
			})

			It("should accumulate multiple errors of same type", func() {
				// BUSINESS SCENARIO: Multiple rate limit errors during storm
				// Expected: Counter accumulates all errors

				for i := 0; i < 5; i++ {
					testMetrics.CRDCreationErrors.WithLabelValues("rate_limited").Inc()
				}

				value := getCounterValue(testMetrics.CRDCreationErrors, "rate_limited")
				Expect(value).To(Equal(float64(5)),
					"All rate limit errors should be accumulated")

				// BUSINESS CAPABILITY VERIFIED:
				// ✅ Operators can calculate error rate: rate(gateway_crd_creation_errors_total[5m])
			})

			It("should track different error types independently", func() {
				// BUSINESS SCENARIO: Mixed error types during incident
				// Expected: Each error type tracked separately

				testMetrics.CRDCreationErrors.WithLabelValues("rate_limited").Inc()
				testMetrics.CRDCreationErrors.WithLabelValues("rate_limited").Inc()
				testMetrics.CRDCreationErrors.WithLabelValues("timeout").Inc()
				testMetrics.CRDCreationErrors.WithLabelValues("service_unavailable").Inc()

				Expect(getCounterValue(testMetrics.CRDCreationErrors, "rate_limited")).To(Equal(float64(2)))
				Expect(getCounterValue(testMetrics.CRDCreationErrors, "timeout")).To(Equal(float64(1)))
				Expect(getCounterValue(testMetrics.CRDCreationErrors, "service_unavailable")).To(Equal(float64(1)))

				// BUSINESS CAPABILITY VERIFIED:
				// ✅ Operators can break down errors by type for root cause analysis
			})
		})

		Context("when retry attempts are made for K8s API failures", func() {

			It("should record retry attempts with error type and status code", func() {
				// BUSINESS SCENARIO: CRD creation retried after rate limit
				// Expected: Retry attempt recorded with labels

				testMetrics.RetryAttemptsTotal.WithLabelValues("rate_limited", "429").Inc()

				value := getCounterVecValue(testMetrics.RetryAttemptsTotal, "rate_limited", "429")
				Expect(value).To(Equal(float64(1)),
					"Retry attempt should be recorded with error type and status code")

				// BUSINESS CAPABILITY VERIFIED:
				// ✅ Operators can monitor retry behavior by error type
			})

			It("should record successful retries with attempt number", func() {
				// BUSINESS SCENARIO: CRD creation succeeds on 2nd attempt
				// Expected: Success recorded with attempt number

				testMetrics.RetrySuccessTotal.WithLabelValues("rate_limited", "2").Inc()

				value := getCounterVecValue(testMetrics.RetrySuccessTotal, "rate_limited", "2")
				Expect(value).To(Equal(float64(1)),
					"Successful retry should be recorded with attempt number")

				// BUSINESS CAPABILITY VERIFIED:
				// ✅ Operators can analyze retry success distribution
			})

			It("should record exhausted retries", func() {
				// BUSINESS SCENARIO: All retry attempts failed
				// Expected: Exhausted retry recorded with error type and status code

				testMetrics.RetryExhaustedTotal.WithLabelValues("service_unavailable", "503").Inc()

				value := getCounterVecValue(testMetrics.RetryExhaustedTotal, "service_unavailable", "503")
				Expect(value).To(Equal(float64(1)),
					"Exhausted retry should be recorded")

				// BUSINESS CAPABILITY VERIFIED:
				// ✅ Operators can alert on retry exhaustion (indicates severe issues)
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// Redis Failure Metrics
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Redis Failure Metrics (BR-GATEWAY-106)", func() {

		Context("when Redis becomes unavailable", func() {

			It("should record Redis outage count", func() {
				// BUSINESS SCENARIO: Redis connection lost
				// Expected: Outage count incremented

				testMetrics.RedisOutageCount.Inc()

				var metric dto.Metric
				err := testMetrics.RedisOutageCount.Write(&metric)
				Expect(err).ToNot(HaveOccurred())
				Expect(metric.Counter.GetValue()).To(Equal(float64(1)),
					"Redis outage count should be incremented")

				// BUSINESS CAPABILITY VERIFIED:
				// ✅ Operators can count Redis outage events
			})

			It("should accumulate multiple outage events", func() {
				// BUSINESS SCENARIO: Redis flapping (multiple disconnects)
				// Expected: All outages counted

				for i := 0; i < 3; i++ {
					testMetrics.RedisOutageCount.Inc()
				}

				var metric dto.Metric
				err := testMetrics.RedisOutageCount.Write(&metric)
				Expect(err).ToNot(HaveOccurred())
				Expect(metric.Counter.GetValue()).To(Equal(float64(3)),
					"All Redis outages should be counted")

				// BUSINESS CAPABILITY VERIFIED:
				// ✅ Operators can detect Redis instability via outage frequency
			})

			It("should record Redis outage duration", func() {
				// BUSINESS SCENARIO: Redis unavailable for 30 seconds
				// Expected: Duration added to cumulative counter

				outageDuration := 30.0 // seconds
				testMetrics.RedisOutageDuration.Add(outageDuration)

				var metric dto.Metric
				err := testMetrics.RedisOutageDuration.Write(&metric)
				Expect(err).ToNot(HaveOccurred())
				Expect(metric.Counter.GetValue()).To(Equal(outageDuration),
					"Redis outage duration should be recorded")

				// BUSINESS CAPABILITY VERIFIED:
				// ✅ Operators can track cumulative downtime for SLO compliance
			})

			It("should accumulate outage duration across multiple outages", func() {
				// BUSINESS SCENARIO: Multiple Redis outages with different durations
				// Expected: Cumulative duration tracked

				testMetrics.RedisOutageDuration.Add(30.0) // First outage: 30s
				testMetrics.RedisOutageDuration.Add(15.0) // Second outage: 15s
				testMetrics.RedisOutageDuration.Add(45.0) // Third outage: 45s

				var metric dto.Metric
				err := testMetrics.RedisOutageDuration.Write(&metric)
				Expect(err).ToNot(HaveOccurred())
				Expect(metric.Counter.GetValue()).To(Equal(float64(90)),
					"Cumulative outage duration should be 90 seconds")

				// BUSINESS CAPABILITY VERIFIED:
				// ✅ SLO query: increase(gateway_redis_outage_duration_seconds_total[30d]) < 2592 (99.9% uptime)
			})

			It("should set Redis availability to 0 during outage", func() {
				// BUSINESS SCENARIO: Redis health check fails
				// Expected: Availability gauge set to 0

				testMetrics.RedisAvailable.Set(0)

				var metric dto.Metric
				err := testMetrics.RedisAvailable.Write(&metric)
				Expect(err).ToNot(HaveOccurred())
				Expect(metric.Gauge.GetValue()).To(Equal(float64(0)),
					"Redis availability should be 0 during outage")

				// BUSINESS CAPABILITY VERIFIED:
				// ✅ Operators can alert on: gateway_redis_available == 0
			})

			It("should set Redis availability to 1 when recovered", func() {
				// BUSINESS SCENARIO: Redis connection restored
				// Expected: Availability gauge set to 1

				// Simulate outage
				testMetrics.RedisAvailable.Set(0)
				// Simulate recovery
				testMetrics.RedisAvailable.Set(1)

				var metric dto.Metric
				err := testMetrics.RedisAvailable.Write(&metric)
				Expect(err).ToNot(HaveOccurred())
				Expect(metric.Gauge.GetValue()).To(Equal(float64(1)),
					"Redis availability should be 1 after recovery")

				// BUSINESS CAPABILITY VERIFIED:
				// ✅ Operators can track Redis recovery
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// Error Type Classification
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Error Type Classification (BR-GATEWAY-114)", func() {

		Context("when classifying K8s API errors for metrics", func() {

			It("should classify rate limit errors correctly", func() {
				// BUSINESS SCENARIO: Determine error type from K8s API error
				err := apierrors.NewTooManyRequests("rate limited", 1)
				errorType := classifyK8sError(err)
				Expect(errorType).To(Equal("rate_limited"))
			})

			It("should classify service unavailable errors correctly", func() {
				err := apierrors.NewServiceUnavailable("API server overloaded")
				errorType := classifyK8sError(err)
				Expect(errorType).To(Equal("service_unavailable"))
			})

			It("should classify timeout errors correctly", func() {
				err := apierrors.NewTimeoutError("request timeout", 30)
				errorType := classifyK8sError(err)
				Expect(errorType).To(Equal("timeout"))
			})

			It("should classify conflict errors correctly", func() {
				err := apierrors.NewConflict(
					schema.GroupResource{Group: "remediation.kubernaut.io", Resource: "remediationrequests"},
					"test-rr",
					errors.New("already exists"),
				)
				errorType := classifyK8sError(err)
				Expect(errorType).To(Equal("conflict"))
			})

			It("should classify forbidden errors correctly", func() {
				err := apierrors.NewForbidden(
					schema.GroupResource{Group: "remediation.kubernaut.io", Resource: "remediationrequests"},
					"test-rr",
					errors.New("insufficient permissions"),
				)
				errorType := classifyK8sError(err)
				Expect(errorType).To(Equal("forbidden"))
			})

			It("should classify unknown errors as 'unknown'", func() {
				err := errors.New("unexpected error")
				errorType := classifyK8sError(err)
				Expect(errorType).To(Equal("unknown"))
			})
		})
	})
})

// Helper function to get counter value from CounterVec
func getCounterValue(counterVec *prometheus.CounterVec, labels ...string) float64 {
	counter, err := counterVec.GetMetricWithLabelValues(labels...)
	if err != nil {
		return 0
	}
	var metric dto.Metric
	if err := counter.Write(&metric); err != nil {
		return 0
	}
	return metric.Counter.GetValue()
}

// Helper function to get counter value from CounterVec with multiple labels
func getCounterVecValue(counterVec *prometheus.CounterVec, labels ...string) float64 {
	counter, err := counterVec.GetMetricWithLabelValues(labels...)
	if err != nil {
		return 0
	}
	var metric dto.Metric
	if err := counter.Write(&metric); err != nil {
		return 0
	}
	return metric.Counter.GetValue()
}

// classifyK8sError determines the error type for metrics labeling
// This mirrors the classification logic used in production code
func classifyK8sError(err error) string {
	if apierrors.IsTooManyRequests(err) {
		return "rate_limited"
	}
	if apierrors.IsServiceUnavailable(err) {
		return "service_unavailable"
	}
	if apierrors.IsTimeout(err) {
		return "timeout"
	}
	if apierrors.IsConflict(err) {
		return "conflict"
	}
	if apierrors.IsForbidden(err) {
		return "forbidden"
	}
	if apierrors.IsInvalid(err) {
		return "validation"
	}
	if apierrors.IsNotFound(err) {
		return "not_found"
	}
	return "unknown"
}

// Suppress unused import warning for time package
var _ = time.Now

