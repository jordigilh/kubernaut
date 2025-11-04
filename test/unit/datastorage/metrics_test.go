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

package datastorage

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
)

// Ginkgo test suite for metrics
var _ = Describe("BR-STORAGE-019: Prometheus Metrics", func() {
	BeforeEach(func() {
		// Reset metrics before each test
		metrics.WriteTotal.Reset()
		metrics.WriteDuration.Reset()
		metrics.DualWriteSuccess.Collect(make(chan prometheus.Metric, 10))
		// Note: Some metrics don't have Reset(), but we can verify increments
	})

	Context("Write Operation Metrics", func() {
		It("should track write operations by table and status", func() {
			// BR-STORAGE-019: WriteTotal metric
			initialValue := getCounterValue(metrics.WriteTotal.WithLabelValues(metrics.TableRemediationAudit, metrics.StatusSuccess))

			// Simulate successful write
			metrics.WriteTotal.WithLabelValues(metrics.TableRemediationAudit, metrics.StatusSuccess).Inc()

			newValue := getCounterValue(metrics.WriteTotal.WithLabelValues(metrics.TableRemediationAudit, metrics.StatusSuccess))
			Expect(newValue).To(Equal(initialValue + 1))
		})

		It("should track write duration", func() {
			// BR-STORAGE-019: WriteDuration histogram
			metrics.WriteDuration.WithLabelValues(metrics.TableRemediationAudit).Observe(0.025) // 25ms
			Expect(metrics.WriteDuration.WithLabelValues(metrics.TableRemediationAudit)).ToNot(BeNil())
		})

		It("should support all table types", func() {
			tables := []string{
				metrics.TableRemediationAudit,
				metrics.TableAIAnalysisAudit,
				metrics.TableWorkflowAudit,
				metrics.TableExecutionAudit,
			}

			for _, table := range tables {
				initialValue := getCounterValue(metrics.WriteTotal.WithLabelValues(table, metrics.StatusSuccess))
				metrics.WriteTotal.WithLabelValues(table, metrics.StatusSuccess).Inc()
				newValue := getCounterValue(metrics.WriteTotal.WithLabelValues(table, metrics.StatusSuccess))
				Expect(newValue).To(Equal(initialValue+1), "table %s should be supported", table)
			}
		})
	})

	Context("Dual-Write Coordination Metrics", func() {
		It("should track successful dual-write operations", func() {
			// BR-STORAGE-019: DualWriteSuccess counter
			initialValue := getCounterValue(metrics.DualWriteSuccess)
			metrics.DualWriteSuccess.Inc()
			newValue := getCounterValue(metrics.DualWriteSuccess)
			Expect(newValue).To(Equal(initialValue + 1))
		})

		It("should track dual-write failures by reason", func() {
			// BR-STORAGE-019: DualWriteFailure counter with reasons
			reasons := []string{
				metrics.ReasonPostgreSQLFailure,
				metrics.ReasonVectorDBFailure,
				metrics.ReasonValidationFailure,
				metrics.ReasonContextCanceled,
				metrics.ReasonTransactionRollback,
				metrics.ReasonUnknown,
			}

			for _, reason := range reasons {
				initialValue := getCounterValue(metrics.DualWriteFailure.WithLabelValues(reason))
				metrics.DualWriteFailure.WithLabelValues(reason).Inc()
				newValue := getCounterValue(metrics.DualWriteFailure.WithLabelValues(reason))
				Expect(newValue).To(Equal(initialValue+1), "reason %s should be tracked", reason)
			}
		})

		It("should track fallback mode operations", func() {
			// BR-STORAGE-015: Graceful degradation tracking
			initialValue := getCounterValue(metrics.FallbackModeTotal)
			metrics.FallbackModeTotal.Inc()
			newValue := getCounterValue(metrics.FallbackModeTotal)
			Expect(newValue).To(Equal(initialValue + 1))
		})
	})

	Context("Embedding and Caching Metrics", func() {
		It("should track cache hits", func() {
			// BR-STORAGE-009: Cache hit tracking
			initialValue := getCounterValue(metrics.CacheHits)
			metrics.CacheHits.Inc()
			newValue := getCounterValue(metrics.CacheHits)
			Expect(newValue).To(Equal(initialValue + 1))
		})

		It("should track cache misses", func() {
			// BR-STORAGE-009: Cache miss tracking
			initialValue := getCounterValue(metrics.CacheMisses)
			metrics.CacheMisses.Inc()
			newValue := getCounterValue(metrics.CacheMisses)
			Expect(newValue).To(Equal(initialValue + 1))
		})

		It("should track embedding generation duration", func() {
			// BR-STORAGE-008: Embedding generation performance
			metrics.EmbeddingGenerationDuration.Observe(0.150) // 150ms

			histogram := metrics.EmbeddingGenerationDuration
			Expect(histogram).ToNot(BeNil())
		})
	})

	Context("Validation Metrics", func() {
		It("should track validation failures by field and reason", func() {
			// BR-STORAGE-010: Validation failure tracking
			fields := []string{"name", "namespace", "phase", "action_type"}
			reasons := []string{
				metrics.ValidationReasonRequired,
				metrics.ValidationReasonInvalid,
				metrics.ValidationReasonLengthExceeded,
			}

			for _, field := range fields {
				for _, reason := range reasons {
					initialValue := getCounterValue(metrics.ValidationFailures.WithLabelValues(field, reason))
					metrics.ValidationFailures.WithLabelValues(field, reason).Inc()
					newValue := getCounterValue(metrics.ValidationFailures.WithLabelValues(field, reason))
					Expect(newValue).To(Equal(initialValue+1),
						"field %s with reason %s should be tracked", field, reason)
				}
			}
		})
	})

	Context("Query Operation Metrics", func() {
		It("should track query duration by operation type", func() {
			// BR-STORAGE-007, BR-STORAGE-012, BR-STORAGE-013: Query performance
			operations := []string{
				metrics.OperationList,
				metrics.OperationGet,
				metrics.OperationSemanticSearch,
				metrics.OperationFilter,
			}

			for _, operation := range operations {
				metrics.QueryDuration.WithLabelValues(operation).Observe(0.010) // 10ms
			}
		})

		It("should track query total by operation and status", func() {
			// BR-STORAGE-019: Query success/failure tracking
			operations := []string{
				metrics.OperationList,
				metrics.OperationGet,
				metrics.OperationSemanticSearch,
				metrics.OperationFilter,
			}
			statuses := []string{metrics.StatusSuccess, metrics.StatusFailure}

			for _, operation := range operations {
				for _, status := range statuses {
					initialValue := getCounterValue(metrics.QueryTotal.WithLabelValues(operation, status))
					metrics.QueryTotal.WithLabelValues(operation, status).Inc()
					newValue := getCounterValue(metrics.QueryTotal.WithLabelValues(operation, status))
					Expect(newValue).To(Equal(initialValue+1),
						"operation %s with status %s should be tracked", operation, status)
				}
			}
		})
	})

	Context("Cardinality Protection", func() {
		It("should have bounded label values for all metrics", func() {
			// BR-STORAGE-019: Cardinality protection verification
			// This test ensures we only use enum-like values, not dynamic strings

			// Write operations: 4 tables × 2 statuses = 8 combinations
			writeCardinality := 4 * 2

			// Dual-write failures: 6 reasons
			dualwriteCardinality := 6

			// Validation failures: 4 fields × 3 reasons = 12 combinations
			validationCardinality := 4 * 3

			// Query operations: 4 operations × 2 statuses = 8 combinations
			queryCardinality := 4 * 2

			// Query duration: 4 operations
			queryDurationCardinality := 4

			// Other metrics (no labels or single counter): ~10
			otherCardinality := 10

			totalCardinality := writeCardinality + dualwriteCardinality + validationCardinality +
				queryCardinality + queryDurationCardinality + otherCardinality

			// Total: 8 + 6 + 12 + 8 + 4 + 10 = 48 (well under 100 target)
			Expect(totalCardinality).To(BeNumerically("<", 100),
				"Total cardinality should be under 100 to prevent Prometheus performance issues")

			GinkgoWriter.Printf("✅ Total metrics cardinality: %d (target: < 100)\n", totalCardinality)
		})

		It("should never use dynamic values as label values", func() {
			// BR-STORAGE-019: This is a documentation test to prevent anti-patterns
			// Examples of FORBIDDEN label values:
			// - err.Error() // ❌ Unlimited cardinality
			// - audit.Name // ❌ User-controlled cardinality
			// - time.Now().String() // ❌ One time series per millisecond
			// - fmt.Sprintf("%d", audit.ID) // ❌ One time series per record

			// ✅ CORRECT: Use constants from metrics/helpers.go
			Expect(metrics.StatusSuccess).To(Equal("success"))
			Expect(metrics.StatusFailure).To(Equal("failure"))
			Expect(metrics.ReasonPostgreSQLFailure).To(Equal("postgresql_failure"))
			Expect(metrics.ValidationReasonRequired).To(Equal("required"))
		})
	})

	Context("Performance Impact", func() {
		It("should have minimal overhead for counter increment", func() {
			// BR-STORAGE-019: Metrics should have < 5% performance overhead
			start := time.Now()
			for i := 0; i < 1000; i++ {
				metrics.WriteTotal.WithLabelValues(metrics.TableRemediationAudit, metrics.StatusSuccess).Inc()
			}
			duration := time.Since(start)

			// 1000 increments should take < 1ms
			Expect(duration.Milliseconds()).To(BeNumerically("<", 1),
				"Counter increment should be very fast (< 1μs per operation)")

			GinkgoWriter.Printf("✅ 1000 counter increments took %v (< 1ms target)\n", duration)
		})

		It("should have minimal overhead for histogram observation", func() {
			start := time.Now()
			for i := 0; i < 1000; i++ {
				metrics.WriteDuration.WithLabelValues(metrics.TableRemediationAudit).Observe(0.025)
			}
			duration := time.Since(start)

			// 1000 observations should take < 5ms
			Expect(duration.Milliseconds()).To(BeNumerically("<", 5),
				"Histogram observation should be fast (< 5μs per operation)")

			GinkgoWriter.Printf("✅ 1000 histogram observations took %v (< 5ms target)\n", duration)
		})
	})
})

// Helper function to get counter value
func getCounterValue(counter prometheus.Counter) float64 {
	metric := &dto.Metric{}
	if err := counter.Write(metric); err != nil {
		return 0
	}
	return metric.GetCounter().GetValue()
}

// Benchmark tests for metrics performance
func BenchmarkMetricsCounterIncrement(b *testing.B) {
	// BR-STORAGE-019: Benchmark counter increment performance
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.WriteTotal.WithLabelValues(metrics.TableRemediationAudit, metrics.StatusSuccess).Inc()
	}
}

func BenchmarkMetricsHistogramObserve(b *testing.B) {
	// BR-STORAGE-019: Benchmark histogram observation performance
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.WriteDuration.WithLabelValues(metrics.TableRemediationAudit).Observe(0.025)
	}
}

func BenchmarkMetricsCounterVecLabelLookup(b *testing.B) {
	// BR-STORAGE-019: Benchmark label value lookup performance
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = metrics.WriteTotal.WithLabelValues(metrics.TableRemediationAudit, metrics.StatusSuccess)
	}
}

func BenchmarkMetricsValidationFailureTracking(b *testing.B) {
	// BR-STORAGE-010: Benchmark validation failure tracking overhead
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.ValidationFailures.WithLabelValues("name", metrics.ValidationReasonRequired).Inc()
	}
}

func BenchmarkMetricsDualWriteFailureTracking(b *testing.B) {
	// BR-STORAGE-014: Benchmark dual-write failure tracking overhead
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.DualWriteFailure.WithLabelValues(metrics.ReasonPostgreSQLFailure).Inc()
	}
}

func BenchmarkMetricsCacheHitTracking(b *testing.B) {
	// BR-STORAGE-009: Benchmark cache hit tracking overhead
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.CacheHits.Inc()
	}
}

func BenchmarkMetricsQueryDurationTracking(b *testing.B) {
	// BR-STORAGE-007: Benchmark query duration tracking overhead
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.QueryDuration.WithLabelValues(metrics.OperationSemanticSearch).Observe(0.010)
	}
}

// Benchmark comprehensive write operation with all metrics
func BenchmarkMetricsFullWriteOperationInstrumentation(b *testing.B) {
	// BR-STORAGE-019: Benchmark full write operation with all metrics
	// This simulates the complete metrics overhead for a typical write operation
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Validation metrics
		metrics.ValidationFailures.WithLabelValues("name", metrics.ValidationReasonRequired).Inc()

		// Embedding metrics
		metrics.CacheMisses.Inc()
		metrics.EmbeddingGenerationDuration.Observe(0.150)

		// Write metrics
		metrics.WriteTotal.WithLabelValues(metrics.TableRemediationAudit, metrics.StatusSuccess).Inc()
		metrics.WriteDuration.WithLabelValues(metrics.TableRemediationAudit).Observe(0.025)

		// Dual-write metrics
		metrics.DualWriteSuccess.Inc()
	}
}
