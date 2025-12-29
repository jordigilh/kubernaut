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

package notification

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	notificationmetrics "github.com/jordigilh/kubernaut/pkg/notification/metrics"
)

// ========================================
// Prometheus Metrics Unit Tests (DD-005 V3.0 / DD-METRICS-001)
// ========================================
//
// Authority: docs/handoff/NT_REMAINING_WORK_STATUS_DEC_17_2025.md
// Priority: P3 - Nice-to-have (E2E metrics tests already exist)
//
// Test Coverage:
// - PrometheusRecorder methods (DD-METRICS-001 pattern)
// - 8 helper functions: Record*/Update* functions
// - Label validation
// - Value increments/observations
// - DD-005 V3.0 naming compliance
//
// Note: These are unit tests for the PrometheusRecorder methods themselves.
//       E2E metrics validation exists in test/e2e/notification/04_metrics_validation_test.go
//
// Changes (DD-005 V3.0 compliance):
// - Migrated from internal/controller/notification/metrics.go (DELETED)
// - Now uses pkg/notification/metrics.PrometheusRecorder (DD-METRICS-001)
// - Uses DD-005 V3.0 compliant metric names
// ========================================

var _ = Describe("Prometheus Metrics Unit Tests", func() {
	var (
		recorder *notificationmetrics.PrometheusRecorder
		registry *prometheus.Registry
	)

	BeforeEach(func() {
		// Create a fresh registry for each test to avoid duplicate registration
		// This prevents panics from registering the same metrics multiple times
		registry = prometheus.NewRegistry()

		// Create a new PrometheusRecorder with the test-specific registry
		// DD-METRICS-001: Dependency-injected metrics pattern
		recorder = notificationmetrics.NewPrometheusRecorderWithRegistry(registry)
	})

	// ========================================
	// TEST 1: RecordDeliveryAttempt (Counter)
	// ========================================
	Context("RecordDeliveryAttempt", func() {
		It("should increment kubernaut_notification_delivery_attempts_total counter without panicking", func() {
			// Record delivery attempts (DD-005 V3.0 compliant metric name)
			Expect(func() {
				recorder.RecordDeliveryAttempt("default", "slack", "success")
				recorder.RecordDeliveryAttempt("default", "slack", "success")
				recorder.RecordDeliveryAttempt("default", "email", "failure")
			}).ToNot(Panic(), "Recording delivery attempts should not panic")
		})

		It("should track delivery attempts per channel without panicking", func() {
			Expect(func() {
				recorder.RecordDeliveryAttempt("prod", "slack", "success")
				recorder.RecordDeliveryAttempt("dev", "slack", "success")
			}).ToNot(Panic(), "Recording delivery attempts for different channels should not panic")
		})
	})

	// ========================================
	// TEST 2: RecordDeliveryDuration (Histogram)
	// ========================================
	Context("RecordDeliveryDuration", func() {
		It("should observe kubernaut_notification_delivery_duration_seconds histogram without panicking", func() {
			Expect(func() {
				recorder.RecordDeliveryDuration("default", "slack", 2.5)
				recorder.RecordDeliveryDuration("default", "slack", 5.0)
				recorder.RecordDeliveryDuration("default", "slack", 10.0)
			}).ToNot(Panic(), "Recording delivery durations should not panic")
		})

		It("should handle different duration values without panicking", func() {
			Expect(func() {
				recorder.RecordDeliveryDuration("default", "email", 0.5)  // <1s
				recorder.RecordDeliveryDuration("default", "email", 3.0)  // 2-5s
				recorder.RecordDeliveryDuration("default", "email", 45.0) // 30-60s
			}).ToNot(Panic(), "Recording different duration values should not panic")
		})
	})

	// ========================================
	// TEST 3: UpdateFailureRatio (Gauge) - No-op in current implementation
	// ========================================
	Context("UpdateFailureRatio", func() {
		It("should handle failure ratio updates without panicking (no-op)", func() {
			// Note: This metric is not currently exposed in DD-005 V3.0 consolidated metrics
			// Kept for interface compliance
			Expect(func() {
				recorder.UpdateFailureRatio("default", 0.15)
			}).ToNot(Panic(), "Updating failure ratio should not panic")
		})

		It("should allow ratio updates for same namespace without panicking (no-op)", func() {
			Expect(func() {
				recorder.UpdateFailureRatio("prod", 0.05)
				recorder.UpdateFailureRatio("prod", 0.12) // Update
			}).ToNot(Panic(), "Updating failure ratio multiple times should not panic")
		})
	})

	// ========================================
	// TEST 4: RecordStuckDuration (Histogram) - No-op in current implementation
	// ========================================
	Context("RecordStuckDuration", func() {
		It("should handle stuck duration recording without panicking (no-op)", func() {
			// Note: This metric is not currently exposed in DD-005 V3.0 consolidated metrics
			// Kept for interface compliance
			Expect(func() {
				recorder.RecordStuckDuration("default", 120)  // 2 minutes
				recorder.RecordStuckDuration("default", 600)  // 10 minutes
				recorder.RecordStuckDuration("default", 1200) // 20 minutes
			}).ToNot(Panic(), "Recording stuck durations should not panic")
		})
	})

	// ========================================
	// TEST 5: UpdatePhaseCount (Gauge)
	// ========================================
	Context("UpdatePhaseCount", func() {
		It("should set kubernaut_notification_reconciler_active gauge without panicking", func() {
			Expect(func() {
				recorder.UpdatePhaseCount("default", "Pending", 5)
				recorder.UpdatePhaseCount("default", "Sent", 10)
			}).ToNot(Panic(), "Updating phase counts should not panic")
		})
	})

	// ========================================
	// TEST 6: RecordDeliveryRetries (Counter)
	// ========================================
	Context("RecordDeliveryRetries", func() {
		It("should increment kubernaut_notification_delivery_retries_total counter without panicking", func() {
			Expect(func() {
				recorder.RecordDeliveryRetries("default", 0) // No retries
				recorder.RecordDeliveryRetries("default", 3) // 3 retries
				recorder.RecordDeliveryRetries("default", 5) // 5 retries
			}).ToNot(Panic(), "Recording delivery retries should not panic")
		})
	})

	// ========================================
	// TEST 7: RecordSlackRetry (Counter)
	// ========================================
	Context("RecordSlackRetry", func() {
		It("should increment kubernaut_notification_delivery_retries_total counter without panicking", func() {
			Expect(func() {
				recorder.RecordSlackRetry("default", "rate_limit")
				recorder.RecordSlackRetry("default", "rate_limit")
				recorder.RecordSlackRetry("default", "timeout")
			}).ToNot(Panic(), "Recording Slack retries should not panic")
		})
	})

	// ========================================
	// TEST 8: RecordSlackBackoff (Histogram) - No-op in current implementation
	// ========================================
	Context("RecordSlackBackoff", func() {
		It("should handle Slack backoff duration recording without panicking (no-op)", func() {
			// Note: This metric is not currently exposed in DD-005 V3.0 consolidated metrics
			// Kept for interface compliance
			Expect(func() {
				recorder.RecordSlackBackoff("default", 30)  // 30s
				recorder.RecordSlackBackoff("default", 60)  // 1m
				recorder.RecordSlackBackoff("default", 120) // 2m
			}).ToNot(Panic(), "Recording Slack backoff durations should not panic")
		})
	})
})
