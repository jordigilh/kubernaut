package analytics

import "time"

// Note: AnalyticsEngine interface moved to pkg/shared/types/analytics.go for consolidation
// Use pkg/ai/insights/service.go for the comprehensive implementation

// Common analytics result types
type AnalyticsResult struct {
	Accuracy float64
	Latency  time.Duration
}
