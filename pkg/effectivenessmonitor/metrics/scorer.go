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

package metrics

import (
	"fmt"
	"math"

	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
)

// MetricComparison represents a pre/post remediation metric comparison.
type MetricComparison struct {
	// Name identifies the metric being compared (e.g., "cpu_usage", "memory_usage").
	Name string
	// PreValue is the metric value before remediation.
	PreValue float64
	// PostValue is the metric value after remediation.
	PostValue float64
	// LowerIsBetter indicates whether a lower value means improvement.
	// true for CPU/memory usage; false for availability/throughput.
	LowerIsBetter bool
}

// ComparisonResult contains the scored outcome of a metric comparison.
type ComparisonResult struct {
	// Component is the full ComponentResult for status/audit.
	Component types.ComponentResult
	// PerMetricScores contains the individual metric scores.
	PerMetricScores []MetricScore
}

// MetricScore contains the score for a single metric comparison.
type MetricScore struct {
	// Name of the metric.
	Name string
	// Score is the normalized improvement score (0.0-1.0).
	Score float64
	// Improved indicates whether this specific metric improved.
	Improved bool
}

// Scorer evaluates metric comparison results and produces a score.
// This contains pure scoring logic (no I/O to Prometheus).
type Scorer interface {
	// Score evaluates the metric comparisons and returns a ComponentResult.
	// The score is the average of individual metric improvement scores.
	// An empty comparisons slice results in a nil score (not assessed).
	Score(comparisons []MetricComparison) ComparisonResult
}

// scorer is the concrete implementation of metric comparison Scorer.
type scorer struct{}

// NewScorer creates a new metric comparison scorer.
func NewScorer() Scorer {
	return &scorer{}
}

// Score evaluates the metric comparisons.
//
// Scoring Logic (BR-EM-003):
//   - Each metric is scored based on relative improvement (0.0-1.0)
//   - For LowerIsBetter metrics: improvement = (pre - post) / pre, clamped to [0, 1]
//   - For HigherIsBetter metrics: improvement = (post - pre) / pre, clamped to [0, 1]
//   - Overall score is the average of all individual metric scores
//   - No change in metrics -> 0.0
//   - Degradation -> 0.0 (clamped, not negative)
//   - Empty comparisons -> not assessed (nil score)
func (s *scorer) Score(comparisons []MetricComparison) ComparisonResult {
	result := ComparisonResult{
		Component: types.ComponentResult{
			Component: types.ComponentMetrics,
		},
	}

	// Empty comparisons -> not assessed
	if len(comparisons) == 0 {
		result.Component.Assessed = false
		result.Component.Score = nil
		result.Component.Details = "no metrics to compare"
		return result
	}

	result.Component.Assessed = true
	result.PerMetricScores = make([]MetricScore, 0, len(comparisons))

	totalScore := 0.0
	improvedCount := 0

	for _, comp := range comparisons {
		metricScore := computeMetricScore(comp)
		result.PerMetricScores = append(result.PerMetricScores, metricScore)
		totalScore += metricScore.Score
		if metricScore.Improved {
			improvedCount++
		}
	}

	avgScore := totalScore / float64(len(comparisons))
	result.Component.Score = &avgScore
	result.Component.Details = fmt.Sprintf("%d of %d metrics improved, average score %.2f",
		improvedCount, len(comparisons), avgScore)

	return result
}

// computeMetricScore calculates the improvement score for a single metric.
func computeMetricScore(comp MetricComparison) MetricScore {
	ms := MetricScore{
		Name: comp.Name,
	}

	// Handle zero pre-value (avoid division by zero)
	if comp.PreValue == 0 {
		// If pre was 0 and post is 0 -> no change -> 0.0
		// If pre was 0 and post > 0 for LowerIsBetter -> degradation -> 0.0
		// If pre was 0 and post > 0 for HigherIsBetter -> improvement, but can't normalize
		if comp.PostValue == 0 {
			ms.Score = 0.0
			ms.Improved = false
			return ms
		}
		if comp.LowerIsBetter {
			// Was 0, now higher -> degradation
			ms.Score = 0.0
			ms.Improved = false
		} else {
			// Was 0, now higher -> improvement (cap at 1.0)
			ms.Score = 1.0
			ms.Improved = true
		}
		return ms
	}

	var improvement float64
	if comp.LowerIsBetter {
		// Lower is better: improvement = (pre - post) / pre
		improvement = (comp.PreValue - comp.PostValue) / math.Abs(comp.PreValue)
	} else {
		// Higher is better: improvement = (post - pre) / pre
		improvement = (comp.PostValue - comp.PreValue) / math.Abs(comp.PreValue)
	}

	// Clamp to [0, 1]
	if improvement < 0 {
		improvement = 0
	}
	if improvement > 1 {
		improvement = 1
	}

	ms.Score = improvement
	ms.Improved = improvement > 0

	return ms
}
