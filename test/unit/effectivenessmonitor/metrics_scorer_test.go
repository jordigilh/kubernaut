/*
Copyright 2026 Jordi Gil.

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

package effectivenessmonitor

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
)

var _ = Describe("Metric Comparison Scorer (BR-EM-003)", func() {

	var scorer metrics.Scorer

	BeforeEach(func() {
		scorer = metrics.NewScorer()
	})

	// ========================================
	// UT-EM-MC-001: CPU improved -> score > 0
	// ========================================
	Describe("Score (UT-EM-MC-001 through UT-EM-MC-008)", func() {

		It("UT-EM-MC-001: should score CPU improvement (0.95 -> 0.30)", func() {
			comparisons := []metrics.MetricComparison{
				{
					Name:          "cpu_usage",
					PreValue:      0.95,
					PostValue:     0.30,
					LowerIsBetter: true,
				},
			}

			result := scorer.Score(comparisons)
			Expect(result.Component.Assessed).To(BeTrue())
			Expect(result.Component.Score).ToNot(BeNil())
			Expect(*result.Component.Score).To(BeNumerically(">", 0.0))
			Expect(result.Component.Component).To(Equal(types.ComponentMetrics))
			// Improvement = (0.95 - 0.30) / 0.95 ≈ 0.684
			Expect(*result.Component.Score).To(BeNumerically("~", 0.684, 0.01))
		})

		// UT-EM-MC-002: Memory improved
		It("UT-EM-MC-002: should score memory improvement (512 -> 128)", func() {
			comparisons := []metrics.MetricComparison{
				{
					Name:          "memory_usage_bytes",
					PreValue:      512,
					PostValue:     128,
					LowerIsBetter: true,
				},
			}

			result := scorer.Score(comparisons)
			Expect(result.Component.Assessed).To(BeTrue())
			Expect(*result.Component.Score).To(BeNumerically(">", 0.0))
			// Improvement = (512 - 128) / 512 = 0.75
			Expect(*result.Component.Score).To(BeNumerically("~", 0.75, 0.01))
		})

		// UT-EM-MC-003: No change in metrics -> 0.0
		It("UT-EM-MC-003: should return 0.0 when no change in metrics", func() {
			comparisons := []metrics.MetricComparison{
				{
					Name:          "cpu_usage",
					PreValue:      0.50,
					PostValue:     0.50,
					LowerIsBetter: true,
				},
			}

			result := scorer.Score(comparisons)
			Expect(result.Component.Assessed).To(BeTrue())
			Expect(*result.Component.Score).To(Equal(0.0))
		})

		// UT-EM-MC-004: Metrics degraded -> 0.0 (clamped)
		It("UT-EM-MC-004: should clamp degraded metrics to 0.0", func() {
			comparisons := []metrics.MetricComparison{
				{
					Name:          "cpu_usage",
					PreValue:      0.30,
					PostValue:     0.95, // Degraded
					LowerIsBetter: true,
				},
			}

			result := scorer.Score(comparisons)
			Expect(result.Component.Assessed).To(BeTrue())
			Expect(*result.Component.Score).To(Equal(0.0))
		})

		// UT-EM-MC-005: Empty results -> not assessed
		It("UT-EM-MC-005: should return not assessed when no comparisons", func() {
			result := scorer.Score([]metrics.MetricComparison{})

			Expect(result.Component.Assessed).To(BeFalse())
			Expect(result.Component.Score).To(BeNil())
			Expect(result.Component.Details).To(ContainSubstring("no metrics"))
		})

		// UT-EM-MC-006: Prometheus disabled -> caller skips, not scorer's responsibility
		// This is tested at reconciler level, not scorer level.
		// The scorer only deals with already-fetched metric data.

		// UT-EM-MC-007: Partial data -> use available metrics only
		It("UT-EM-MC-007: should use available metrics only when partial data", func() {
			comparisons := []metrics.MetricComparison{
				{
					Name:          "cpu_usage",
					PreValue:      0.90,
					PostValue:     0.30,
					LowerIsBetter: true,
				},
				// memory_usage is missing (not returned by Prometheus)
				// Only cpu is available
			}

			result := scorer.Score(comparisons)
			Expect(result.Component.Assessed).To(BeTrue())
			Expect(*result.Component.Score).To(BeNumerically(">", 0.0))
			Expect(result.PerMetricScores).To(HaveLen(1))
		})

		// UT-EM-MC-008: Mixed improvement/degradation -> average score
		It("UT-EM-MC-008: should average scores for mixed improvement/degradation", func() {
			comparisons := []metrics.MetricComparison{
				{
					Name:          "cpu_usage",
					PreValue:      1.0,
					PostValue:     0.5, // 50% improvement
					LowerIsBetter: true,
				},
				{
					Name:          "memory_usage",
					PreValue:      0.5,
					PostValue:     0.8, // Degraded -> 0.0
					LowerIsBetter: true,
				},
			}

			result := scorer.Score(comparisons)
			Expect(result.Component.Assessed).To(BeTrue())
			// CPU: (1.0-0.5)/1.0 = 0.5, Memory: degraded = 0.0
			// Average: (0.5 + 0.0) / 2 = 0.25
			Expect(*result.Component.Score).To(BeNumerically("~", 0.25, 0.01))
			Expect(result.PerMetricScores).To(HaveLen(2))
		})

		// Edge case: HigherIsBetter metric
		It("should handle HigherIsBetter metrics (e.g., availability)", func() {
			comparisons := []metrics.MetricComparison{
				{
					Name:          "availability",
					PreValue:      0.90,
					PostValue:     0.99,
					LowerIsBetter: false,
				},
			}

			result := scorer.Score(comparisons)
			Expect(result.Component.Assessed).To(BeTrue())
			// Improvement = (0.99 - 0.90) / 0.90 = 0.1
			Expect(*result.Component.Score).To(BeNumerically("~", 0.1, 0.01))
		})

		// Edge case: improvement > 100% capped at 1.0
		It("should cap improvement at 1.0", func() {
			comparisons := []metrics.MetricComparison{
				{
					Name:          "cpu_usage",
					PreValue:      0.01,
					PostValue:     0.00, // 100% improvement
					LowerIsBetter: true,
				},
			}

			result := scorer.Score(comparisons)
			Expect(*result.Component.Score).To(Equal(1.0))
		})

		// Edge case: pre-value zero
		It("should handle zero pre-value gracefully", func() {
			comparisons := []metrics.MetricComparison{
				{
					Name:          "cpu_usage",
					PreValue:      0.0,
					PostValue:     0.5,
					LowerIsBetter: true,
				},
			}

			result := scorer.Score(comparisons)
			Expect(result.Component.Assessed).To(BeTrue())
			// Pre was 0, now 0.5, LowerIsBetter -> degradation -> 0.0
			Expect(*result.Component.Score).To(Equal(0.0))
		})

		// Edge case: both values zero
		It("should handle both values zero", func() {
			comparisons := []metrics.MetricComparison{
				{
					Name:          "error_rate",
					PreValue:      0.0,
					PostValue:     0.0,
					LowerIsBetter: true,
				},
			}

			result := scorer.Score(comparisons)
			Expect(result.Component.Assessed).To(BeTrue())
			Expect(*result.Component.Score).To(Equal(0.0))
		})

		// Per-metric scores populated
		It("should populate per-metric scores", func() {
			comparisons := []metrics.MetricComparison{
				{
					Name:          "cpu_usage",
					PreValue:      1.0,
					PostValue:     0.5,
					LowerIsBetter: true,
				},
				{
					Name:          "memory_usage",
					PreValue:      1000,
					PostValue:     500,
					LowerIsBetter: true,
				},
			}

			result := scorer.Score(comparisons)
			Expect(result.PerMetricScores).To(HaveLen(2))

			// CPU: 50% improvement
			Expect(result.PerMetricScores[0].Name).To(Equal("cpu_usage"))
			Expect(result.PerMetricScores[0].Score).To(BeNumerically("~", 0.5, 0.01))
			Expect(result.PerMetricScores[0].Improved).To(BeTrue())

			// Memory: 50% improvement
			Expect(result.PerMetricScores[1].Name).To(Equal("memory_usage"))
			Expect(result.PerMetricScores[1].Score).To(BeNumerically("~", 0.5, 0.01))
			Expect(result.PerMetricScores[1].Improved).To(BeTrue())
		})
	})
})
