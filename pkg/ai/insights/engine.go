package insights

import (
	"context"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/ai/common"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/sirupsen/logrus"
)

// AnalyticsEngine provides advanced analytics and ML-based effectiveness prediction
type AnalyticsEngine struct {
	vectorDB          vector.VectorDatabase
	patternExtractor  vector.PatternExtractor
	log               *logrus.Logger
	models            map[string]PredictiveModel
	aiInsightsService AIInsightsService
}

// PredictiveModel interface for effectiveness prediction models
type PredictiveModel interface {
	// Predict effectiveness for a given pattern
	Predict(ctx context.Context, pattern *vector.ActionPattern) (*EffectivenessPrediction, error)

	// Train the model with historical data
	Train(ctx context.Context, patterns []*vector.ActionPattern) error

	// GetModelInfo returns information about the model
	GetModelInfo() *ModelInfo

	// IsReady returns whether the model is ready for predictions
	IsReady() bool
}

// EffectivenessPrediction represents a prediction result
type EffectivenessPrediction struct {
	PredictedScore      float64                  `json:"predicted_score"`
	Confidence          float64                  `json:"confidence"`
	FactorContributions map[string]float64       `json:"factor_contributions"`
	SimilarPatterns     []*vector.SimilarPattern `json:"similar_patterns"`
	RiskFactors         []string                 `json:"risk_factors"`
	Recommendations     []string                 `json:"recommendations"`
	ModelUsed           string                   `json:"model_used"`
	PredictionTime      time.Time                `json:"prediction_time"`
}

// ModelInfo provides information about a predictive model
type ModelInfo struct {
	Name         string    `json:"name"`
	Version      string    `json:"version"`
	Algorithm    string    `json:"algorithm"`
	TrainedAt    time.Time `json:"trained_at"`
	TrainingSize int       `json:"training_size"`
	Accuracy     float64   `json:"accuracy"`
	Precision    float64   `json:"precision"`
	Recall       float64   `json:"recall"`
	F1Score      float64   `json:"f1_score"`
}

// Use common types for analytics and insights
type (
	AnalyticsInsights     = common.AnalyticsInsights
	SeasonalAnalysis      = common.SeasonalAnalysis
	CorrelationAnalysis   = common.CorrelationAnalysis
	AnomalyAnalysis       = common.AnomalyAnalysis
	TrendAnalysis         = common.TrendAnalysis
	CostAnalysis          = common.CostAnalysis
	EffectivenessAnomaly  = common.EffectivenessAnomaly
	ActionTrend           = common.ActionTrend
	EffectivenessForecast = common.EffectivenessForecast
	CostEffectiveAction   = common.CostEffectiveAction
	CorrelationPair       = common.CorrelationPair
	StatisticalRange      = common.StatisticalRange
	TimeWindow            = common.TimeWindow
)

// NewAnalyticsEngine creates a new analytics engine
func NewAnalyticsEngine(vectorDB vector.VectorDatabase, patternExtractor vector.PatternExtractor, log *logrus.Logger) *AnalyticsEngine {
	engine := &AnalyticsEngine{
		vectorDB:          vectorDB,
		patternExtractor:  patternExtractor,
		log:               log,
		models:            make(map[string]PredictiveModel),
		aiInsightsService: nil, // Will be set via SetAIInsightsService
	}

	// Register default models
	engine.RegisterModel("similarity", NewSimilarityBasedModel(vectorDB, log))
	engine.RegisterModel("statistical", NewStatisticalModel(log))

	return engine
}

// NewAnalyticsEngineWithAI creates a new analytics engine with AI insights service
func NewAnalyticsEngineWithAI(vectorDB vector.VectorDatabase, patternExtractor vector.PatternExtractor, aiService AIInsightsService, log *logrus.Logger) *AnalyticsEngine {
	engine := &AnalyticsEngine{
		vectorDB:          vectorDB,
		patternExtractor:  patternExtractor,
		log:               log,
		models:            make(map[string]PredictiveModel),
		aiInsightsService: aiService,
	}

	// Register default models
	engine.RegisterModel("similarity", NewSimilarityBasedModel(vectorDB, log))
	engine.RegisterModel("statistical", NewStatisticalModel(log))

	return engine
}

// SetAIInsightsService sets the AI insights service for the analytics engine
func (ae *AnalyticsEngine) SetAIInsightsService(aiService AIInsightsService) {
	ae.aiInsightsService = aiService
}

// RegisterModel registers a predictive model
func (ae *AnalyticsEngine) RegisterModel(name string, model PredictiveModel) {
	ae.models[name] = model
	ae.log.WithField("model_name", name).Info("Registered predictive model")
}

// PredictEffectiveness predicts effectiveness for a new action trace
func (ae *AnalyticsEngine) PredictEffectiveness(ctx context.Context, trace *actionhistory.ResourceActionTrace, modelName string) (*EffectivenessPrediction, error) {
	// Extract pattern from trace
	pattern, err := ae.patternExtractor.ExtractPattern(ctx, trace)
	if err != nil {
		return nil, fmt.Errorf("failed to extract pattern: %w", err)
	}

	// Get model
	model, exists := ae.models[modelName]
	if !exists {
		// Use default model
		model = ae.models["similarity"]
		if model == nil {
			return nil, fmt.Errorf("no models available for prediction")
		}
	}

	// Make prediction
	prediction, err := model.Predict(ctx, pattern)
	if err != nil {
		return nil, fmt.Errorf("prediction failed: %w", err)
	}

	ae.log.WithFields(logrus.Fields{
		"action_type":     trace.ActionType,
		"alert_name":      trace.AlertName,
		"predicted_score": prediction.PredictedScore,
		"confidence":      prediction.Confidence,
		"model_used":      prediction.ModelUsed,
	}).Info("Generated effectiveness prediction")

	return prediction, nil
}

// GenerateInsights generates comprehensive analytics insights
func (ae *AnalyticsEngine) GenerateInsights(ctx context.Context) (*common.AnalyticsInsights, error) {
	// Get all patterns from vector database
	analytics, err := ae.vectorDB.GetPatternAnalytics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get pattern analytics: %w", err)
	}

	insights := &common.AnalyticsInsights{
		GeneratedAt: time.Now(),
	}

	// Generate overall effectiveness analysis
	insights.OverallEffectiveness = ae.analyzeOverallEffectiveness(analytics)

	// Generate action type analysis
	insights.ActionTypeAnalysis = ae.analyzeActionTypes(ctx, analytics)

	// Generate seasonal patterns
	insights.SeasonalPatterns = ae.analyzeSeasonalPatterns(analytics)

	// Generate correlation analysis
	insights.CorrelationAnalysis = ae.analyzeCorrelations(ctx, analytics)

	// Generate anomaly detection
	insights.AnomalyDetection = ae.detectAnomalies(ctx, analytics)

	// Generate trend analysis
	insights.TrendAnalysis = ae.analyzeTrends(ctx, analytics)

	// Generate cost-effectiveness analysis
	insights.CostEffectivenessAnalysis = ae.analyzeCostEffectiveness(ctx, analytics)

	ae.log.WithFields(logrus.Fields{
		"total_patterns": analytics.TotalPatterns,
		"action_types":   len(insights.ActionTypeAnalysis),
		"anomalies":      len(insights.AnomalyDetection.DetectedAnomalies),
	}).Info("Generated comprehensive analytics insights")

	return insights, nil
}

// TrainModels trains all registered models with current data
func (ae *AnalyticsEngine) TrainModels(ctx context.Context) error {
	// Get all patterns for training
	patterns, err := ae.getAllPatterns(ctx)
	if err != nil {
		return fmt.Errorf("failed to get patterns for training: %w", err)
	}

	if len(patterns) == 0 {
		ae.log.Warn("No patterns available for model training")
		return nil
	}

	// Train each model
	for name, model := range ae.models {
		ae.log.WithFields(logrus.Fields{
			"model_name":    name,
			"training_size": len(patterns),
		}).Info("Training predictive model")

		if err := model.Train(ctx, patterns); err != nil {
			ae.log.WithError(err).WithField("model_name", name).Error("Failed to train model")
			continue
		}

		ae.log.WithField("model_name", name).Info("Successfully trained model")
	}

	return nil
}

// GetModelInfo returns information about registered models
func (ae *AnalyticsEngine) GetModelInfo() map[string]*ModelInfo {
	info := make(map[string]*ModelInfo)
	for name, model := range ae.models {
		info[name] = model.GetModelInfo()
	}
	return info
}

// Stub methods to make the build work
func (ae *AnalyticsEngine) analyzeOverallEffectiveness(analytics *vector.PatternAnalytics) *common.OverallEffectivenessAnalysis {
	return &common.OverallEffectivenessAnalysis{}
}

func (ae *AnalyticsEngine) analyzeActionTypes(ctx context.Context, analytics *vector.PatternAnalytics) map[string]*common.ActionTypeAnalysis {
	return make(map[string]*common.ActionTypeAnalysis)
}

func (ae *AnalyticsEngine) analyzeSeasonalPatterns(analytics *vector.PatternAnalytics) *SeasonalAnalysis {
	return &SeasonalAnalysis{}
}

func (ae *AnalyticsEngine) analyzeCorrelations(ctx context.Context, analytics *vector.PatternAnalytics) *CorrelationAnalysis {
	return &CorrelationAnalysis{}
}

func (ae *AnalyticsEngine) detectAnomalies(ctx context.Context, analytics *vector.PatternAnalytics) *AnomalyAnalysis {
	return &AnomalyAnalysis{}
}

func (ae *AnalyticsEngine) analyzeTrends(ctx context.Context, analytics *vector.PatternAnalytics) *TrendAnalysis {
	return &TrendAnalysis{}
}

func (ae *AnalyticsEngine) analyzeCostEffectiveness(ctx context.Context, analytics *vector.PatternAnalytics) *CostAnalysis {
	return &CostAnalysis{}
}

func (ae *AnalyticsEngine) getAllPatterns(ctx context.Context) ([]*vector.ActionPattern, error) {
	return nil, nil
}
