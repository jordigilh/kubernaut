package insights

import (
	"context"
	"time"

	"github.com/jordigilh/kubernaut/pkg/ai/common"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

// AIInsightsService provides AI-powered analytics insights generation
type AIInsightsService interface {
	// AnalyzeCorrelations uses AI to discover correlations between effectiveness factors
	AnalyzeCorrelations(ctx context.Context, analytics *vector.PatternAnalytics) (*CorrelationAnalysis, error)

	// DetectAnomalies uses AI to identify unusual patterns in effectiveness data
	DetectAnomalies(ctx context.Context, analytics *vector.PatternAnalytics) (*AnomalyAnalysis, error)

	// AnalyzeTrends uses AI to analyze performance trends and generate forecasts
	AnalyzeTrends(ctx context.Context, analytics *vector.PatternAnalytics) (*TrendAnalysis, error)

	// AnalyzeCostEffectiveness uses AI to analyze cost-effectiveness patterns
	AnalyzeCostEffectiveness(ctx context.Context, analytics *vector.PatternAnalytics) (*CostAnalysis, error)

	// GenerateRecommendations creates actionable recommendations based on insights
	GenerateRecommendations(ctx context.Context, insights *common.AnalyticsInsights) ([]string, error)

	// IsHealthy returns the health status of the AI service
	IsHealthy() bool
}

// AIInsightsRequest represents a request for AI-powered insights
type AIInsightsRequest struct {
	Analytics           *vector.PatternAnalytics `json:"analytics"`
	AnalysisType        string                   `json:"analysis_type"`
	TimeRange           *common.TimeRange        `json:"time_range,omitempty"`
	Focus               []string                 `json:"focus,omitempty"`
	ContextualFactors   map[string]interface{}   `json:"contextual_factors,omitempty"`
	IncludeExplanations bool                     `json:"include_explanations"`
}

// AIInsightsResponse represents the response from AI insights analysis
type AIInsightsResponse struct {
	Insights        interface{}            `json:"insights"`
	Confidence      float64                `json:"confidence"`
	Explanation     string                 `json:"explanation"`
	Recommendations []string               `json:"recommendations"`
	Metadata        map[string]interface{} `json:"metadata"`
	GeneratedAt     time.Time              `json:"generated_at"`
}
