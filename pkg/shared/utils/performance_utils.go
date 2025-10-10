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

package utils

import (
	"time"
)

// **REFACTOR PHASE**: Extracted common performance patterns for code quality improvement

// PerformanceTier represents performance classification levels
type PerformanceTier string

const (
	PerformanceTierExcellent  PerformanceTier = "excellent"
	PerformanceTierGood       PerformanceTier = "good"
	PerformanceTierAcceptable PerformanceTier = "acceptable"
	PerformanceTierSlow       PerformanceTier = "slow"
)

// PerformanceThresholds defines standard performance thresholds
type PerformanceThresholds struct {
	ExcellentThreshold  time.Duration
	GoodThreshold       time.Duration
	AcceptableThreshold time.Duration
}

// DefaultPerformanceThresholds returns standard performance thresholds
func DefaultPerformanceThresholds() PerformanceThresholds {
	return PerformanceThresholds{
		ExcellentThreshold:  100 * time.Millisecond,
		GoodThreshold:       500 * time.Millisecond,
		AcceptableThreshold: 2 * time.Second,
	}
}

// ClassifyPerformance categorizes performance based on processing time
// **CODE QUALITY**: Extracted common pattern used across multiple components
func ClassifyPerformance(processingTime time.Duration, thresholds ...PerformanceThresholds) PerformanceTier {
	var t PerformanceThresholds
	if len(thresholds) > 0 {
		t = thresholds[0]
	} else {
		t = DefaultPerformanceThresholds()
	}

	switch {
	case processingTime < t.ExcellentThreshold:
		return PerformanceTierExcellent
	case processingTime < t.GoodThreshold:
		return PerformanceTierGood
	case processingTime < t.AcceptableThreshold:
		return PerformanceTierAcceptable
	default:
		return PerformanceTierSlow
	}
}

// CalculateEfficiency calculates efficiency based on success rate and performance metrics
// **ARCHITECTURE IMPROVEMENT**: Common efficiency calculation pattern
func CalculateEfficiency(successRate, performanceScore float64, weights ...float64) float64 {
	// Default weights: 70% success rate, 30% performance
	successWeight := 0.7
	performanceWeight := 0.3

	if len(weights) >= 2 {
		successWeight = weights[0]
		performanceWeight = weights[1]
	}

	return (successRate * successWeight) + (performanceScore * performanceWeight)
}

// CalculateBusinessValue calculates business value based on multiple factors
// **BUSINESS LOGIC ENHANCEMENT**: Common business value calculation pattern
func CalculateBusinessValue(costSavings, qualityScore, efficiencyScore float64) float64 {
	// Weighted business value calculation
	return (costSavings * 0.4) + (qualityScore * 0.4) + (efficiencyScore * 0.2)
}
