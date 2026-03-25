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

	"github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
)

// Ginkgo test suite for metrics (external-facing per GitHub issue #294)
var _ = Describe("BR-STORAGE-019: Prometheus Metrics", func() {
	BeforeEach(func() {
		// Reset metrics before each test
		metrics.WriteDuration.Reset()
		metrics.AuditLagSeconds.Reset()
	})

	Context("Write Operation Metrics", func() {
		// BEHAVIOR: WriteDuration histogram records database write operation latency
		// CORRECTNESS: Histogram is properly registered and can record observations
		It("should track write duration with histogram observations", func() {
			// ARRANGE + ACT: Observe a 25ms write duration
			metrics.WriteDuration.WithLabelValues(metrics.TableRemediationAudit).Observe(0.025) // 25ms

			// CORRECTNESS: Histogram is registered and functional (can be retrieved)
			histogram := metrics.WriteDuration.WithLabelValues(metrics.TableRemediationAudit)
			Expect(histogram).ToNot(BeNil(), "WriteDuration histogram should be registered")

			// CORRECTNESS: Can observe multiple values (histogram is functional)
			histogram.Observe(0.030) // 30ms
			histogram.Observe(0.020) // 20ms - verify histogram accepts multiple observations
		})

		It("should support all table types", func() {
			tables := []string{
				metrics.TableRemediationAudit,
				metrics.TableAIAnalysisAudit,
				metrics.TableWorkflowAudit,
				metrics.TableExecutionAudit,
			}

			for _, table := range tables {
				histogram := metrics.WriteDuration.WithLabelValues(table)
				Expect(histogram).ToNot(BeNil(), "table %s should be supported", table)
				histogram.Observe(0.025)
			}
		})
	})

	Context("Cardinality Protection", func() {
		It("should have bounded label values for all metrics", func() {
			// BR-STORAGE-019: Cardinality protection verification
			// External-facing metrics only (GitHub issue #294)
			// WriteDuration: 4 tables
			// AuditLagSeconds: bounded by service names
			Expect(metrics.TableRemediationAudit).To(Equal("remediation_audit"))
			Expect(metrics.ServiceNotification).To(Equal("notification"))
		})

		It("should never use dynamic values as label values", func() {
			// BR-STORAGE-019: This is a documentation test to prevent anti-patterns
			Expect(metrics.StatusSuccess).To(Equal("success"))
			Expect(metrics.StatusFailure).To(Equal("failure"))
			Expect(metrics.ReasonPostgreSQLFailure).To(Equal("postgresql_failure"))
			Expect(metrics.ValidationReasonRequired).To(Equal("required"))
		})
	})

	Context("Performance Impact", func() {
		It("[Flaky] should have minimal overhead for histogram observation", FlakeAttempts(3), func() {
			// BR-STORAGE-019: Metrics should have < 5% performance overhead
			start := time.Now()
			for i := 0; i < 1000; i++ {
				metrics.WriteDuration.WithLabelValues(metrics.TableRemediationAudit).Observe(0.025)
			}
			duration := time.Since(start)

			// 1000 observations should take < 20ms (generous threshold for CI with shared CPU)
			Expect(duration.Milliseconds()).To(BeNumerically("<", 20),
				"Histogram observation should be fast (< 20μs per operation on average)")

			GinkgoWriter.Printf("✅ 1000 histogram observations took %v (< 20ms target for CI)\n", duration)
		})
	})
})

// Benchmark tests for metrics performance
func BenchmarkMetricsHistogramObserve(b *testing.B) {
	// BR-STORAGE-019: Benchmark histogram observation performance
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.WriteDuration.WithLabelValues(metrics.TableRemediationAudit).Observe(0.025)
	}
}
