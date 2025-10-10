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
package types

import (
	"context"
	"time"
)

// AnalyticsInsights represents comprehensive analytics insights
// Moved from pkg/workflow/engine/interfaces.go to resolve import cycles
type AnalyticsInsights struct {
	GeneratedAt      time.Time              `json:"generated_at"`
	WorkflowInsights map[string]interface{} `json:"workflow_insights"`
	PatternInsights  map[string]interface{} `json:"pattern_insights"`
	Recommendations  []string               `json:"recommendations"`
	Metadata         map[string]interface{} `json:"metadata"`
}

// PatternAnalytics represents pattern analytics results
// Moved from pkg/workflow/engine/interfaces.go to resolve import cycles
type PatternAnalytics struct {
	TotalPatterns        int                    `json:"total_patterns"`
	AverageEffectiveness float64                `json:"average_effectiveness"`
	PatternsByType       map[string]int         `json:"patterns_by_type"`
	SuccessRateByType    map[string]float64     `json:"success_rate_by_type"`
	RecentPatterns       []*DiscoveredPattern   `json:"recent_patterns"`
	TopPerformers        []*DiscoveredPattern   `json:"top_performers"`
	FailurePatterns      []*DiscoveredPattern   `json:"failure_patterns"`
	TrendAnalysis        map[string]interface{} `json:"trend_analysis"`
}

// DiscoveredPattern represents a discovered pattern
// Moved from pkg/workflow/engine/interfaces.go to resolve import cycles
type DiscoveredPattern struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Confidence  float64                `json:"confidence"`
	Support     float64                `json:"support"`
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// AnalyticsTrendResult represents trend analysis results for analytics
// Note: TrendAnalysis is already defined in workflow.go with different fields
type AnalyticsTrendResult struct {
	Direction               string  `json:"trend_direction"`
	Strength                float64 `json:"trend_strength"`
	HistoricalAverage       float64 `json:"historical_average"`
	RecentAverage           float64 `json:"recent_average"`
	Confidence              float64 `json:"confidence"`
	StatisticalSignificance bool    `json:"statistical_significance"`
}

// TimeSeriesPoint represents effectiveness at a point in time
type TimeSeriesPoint struct {
	Timestamp     time.Time `json:"timestamp"`
	Effectiveness float64   `json:"effectiveness"`
	Count         int       `json:"count"`
}

// EffectivenessReport represents workflow effectiveness analysis results
// Simplified version to avoid import cycles
type EffectivenessReport struct {
	ID          string                 `json:"id"`
	ExecutionID string                 `json:"execution_id"`
	Score       float64                `json:"score"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// PatternInsights represents pattern analysis insights
// Simplified version to avoid import cycles
type PatternInsights struct {
	PatternID     string                 `json:"pattern_id"`
	Effectiveness float64                `json:"effectiveness"`
	UsageCount    int                    `json:"usage_count"`
	Insights      []string               `json:"insights"`
	Metrics       map[string]interface{} `json:"metrics"`
}

// AnalyticsEngine provides comprehensive analytics functionality
// Consolidates multiple analytics interfaces into a unified design
type AnalyticsEngine interface {
	// Generic analytics capability
	AnalyzeData() error

	// AI Insights Analytics (BR-AI-001, BR-AI-002)
	GetAnalyticsInsights(ctx context.Context, timeWindow time.Duration) (*AnalyticsInsights, error)
	GetPatternAnalytics(ctx context.Context, filters map[string]interface{}) (*PatternAnalytics, error)

	// Workflow-specific analytics
	AnalyzeWorkflowEffectiveness(ctx context.Context, execution *RuntimeWorkflowExecution) (*EffectivenessReport, error)
	GetPatternInsights(ctx context.Context, patternID string) (*PatternInsights, error)
}
