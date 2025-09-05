package common

import (
	"context"
	"time"
)

// Core interfaces for AI service integration
type AnalysisProvider interface {
	AIAnalyzer
	Analyze(ctx context.Context, request *AnalysisRequest) (*AnalysisResult, error)
}

type RecommendationProvider interface {
	AIAnalyzer
	GenerateRecommendations(ctx context.Context, context *RecommendationContext) ([]Recommendation, error)
}

type InvestigationProvider interface {
	AIAnalyzer
	Investigate(ctx context.Context, alert *Alert, context *InvestigationContext) (*InvestigationResult, error)
}

// Request/Response structures for common operations
type AnalysisRequest struct {
	*AIRequest
	Subject      string      `json:"subject"`
	Data         interface{} `json:"data"`
	AnalysisType string      `json:"analysis_type"`
}

type RecommendationContext struct {
	Alert          *Alert                 `json:"alert"`
	HistoricalData interface{}            `json:"historical_data"`
	ConstraintSet  map[string]interface{} `json:"constraint_set"`
	Priority       string                 `json:"priority"`
	MaxSuggestions int                    `json:"max_suggestions"`
}

type InvestigationContext struct {
	HistoricalData interface{}            `json:"historical_data"`
	CustomPrompt   string                 `json:"custom_prompt"`
	Options        *AIOptions             `json:"options"`
	Scope          []string               `json:"scope"`
	TimeWindow     *TimeRange             `json:"time_window"`
	Context        map[string]interface{} `json:"context"`
}

type InvestigationResult struct {
	Alert           *Alert                 `json:"alert"`
	Analysis        *AnalysisResult        `json:"analysis"`
	Recommendations []Recommendation       `json:"recommendations"`
	Evidence        []Evidence             `json:"evidence"`
	Metadata        map[string]interface{} `json:"metadata"`
	ProcessingTime  time.Duration          `json:"processing_time"`
}

// Health and Status monitoring
type HealthStatus struct {
	Healthy      bool                   `json:"healthy"`
	LastCheck    time.Time              `json:"last_check"`
	ResponseTime time.Duration          `json:"response_time"`
	ErrorRate    float64                `json:"error_rate"`
	Details      map[string]interface{} `json:"details"`
	Dependencies []DependencyStatus     `json:"dependencies"`
}

type DependencyStatus struct {
	Name     string        `json:"name"`
	Healthy  bool          `json:"healthy"`
	LastPing time.Time     `json:"last_ping"`
	Latency  time.Duration `json:"latency"`
	Error    string        `json:"error,omitempty"`
}

// Metrics and monitoring
type ServiceMetrics struct {
	RequestCount   int64         `json:"request_count"`
	ErrorCount     int64         `json:"error_count"`
	AverageLatency time.Duration `json:"average_latency"`
	P95Latency     time.Duration `json:"p95_latency"`
	SuccessRate    float64       `json:"success_rate"`
	LastUpdated    time.Time     `json:"last_updated"`
}
