package types

import "time"

// Common types used across multiple packages in the kubernaut system.
// These types were consolidated to eliminate duplication and provide
// consistent data structures throughout the codebase.

// UtilizationTrend represents resource utilization trend analysis data.
// This type consolidates identical definitions from:
// - pkg/intelligence/patterns/pattern_discovery_helpers.go
// - pkg/intelligence/learning/time_series_analyzer.go
type UtilizationTrend struct {
	ResourceType       string  `json:"resource_type"`
	TrendDirection     string  `json:"trend_direction"`
	GrowthRate         float64 `json:"growth_rate"`
	SeasonalVariation  float64 `json:"seasonal_variation"`
	PeakUtilization    float64 `json:"peak_utilization"`
	AverageUtilization float64 `json:"average_utilization"`
	EfficiencyScore    float64 `json:"efficiency_score"`
}

// ConfidenceInterval represents statistical confidence interval data.
// This type consolidates identical definitions from:
// - pkg/intelligence/patterns/pattern_discovery_helpers.go
// - pkg/intelligence/learning/time_series_analyzer.go
type ConfidenceInterval struct {
	Level float64   `json:"level"` // e.g., 0.95 for 95% confidence
	Lower []float64 `json:"lower"`
	Upper []float64 `json:"upper"`
}

// ResourceUsageData represents resource usage metrics.
// This type consolidates identical definitions from:
// - pkg/intelligence/patterns/pattern_discovery_helpers.go
// - pkg/workflow/types/core.go
type ResourceUsageData struct {
	CPUUsage     float64 `json:"cpu_usage"`
	MemoryUsage  float64 `json:"memory_usage"`
	NetworkUsage float64 `json:"network_usage"`
	StorageUsage float64 `json:"storage_usage"`
}

// ValidationResult represents the result of a validation operation.
// This type provides consistent validation result structure.
type ValidationResult struct {
	RuleID    string                 `json:"rule_id"`
	Type      ValidationType         `json:"type"`
	Passed    bool                   `json:"passed"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details"`
	Timestamp time.Time              `json:"timestamp"`
}

// ValidationType represents the type of validation being performed.
type ValidationType string

const (
	ValidationTypeStructural  ValidationType = "structural"
	ValidationTypeSemantic    ValidationType = "semantic"
	ValidationTypePerformance ValidationType = "performance"
	ValidationTypeSecurity    ValidationType = "security"
)
