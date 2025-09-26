//go:build integration
// +build integration

package shared

import (
	"context"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/intelligence/patterns"
	"github.com/jordigilh/kubernaut/pkg/intelligence/shared"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

// =============================================================================
// CONSOLIDATED MOCK IMPLEMENTATIONS (removes duplicates across test files)
// =============================================================================

// StandardPatternExtractor - the canonical pattern extractor for tests
type StandardPatternExtractor struct {
	logger          *logrus.Logger
	embedding       []float64
	features        map[string]float64
	similarity      float64
	patternIDPrefix string
}

// PatternExtractorConfig holds configuration for mock behavior
type PatternExtractorConfig struct {
	Embedding       []float64
	Features        map[string]float64
	Similarity      float64
	PatternIDPrefix string
}

// NewStandardPatternExtractor creates the standard pattern extractor for tests
func NewStandardPatternExtractor(logger *logrus.Logger) *StandardPatternExtractor {
	return &StandardPatternExtractor{
		logger:          logger,
		embedding:       []float64{0.1, 0.2, 0.3, 0.4, 0.5},
		features:        map[string]float64{"complexity": 0.5, "success_rate": 0.8, "avg_duration": 10.0},
		similarity:      0.8,
		patternIDPrefix: "test-pattern",
	}
}

// NewConfigurablePatternExtractor creates a configurable pattern extractor for advanced tests
func NewConfigurablePatternExtractor(logger *logrus.Logger, config *PatternExtractorConfig) *StandardPatternExtractor {
	extractor := &StandardPatternExtractor{logger: logger}
	if config != nil {
		extractor.embedding = config.Embedding
		extractor.features = config.Features
		extractor.similarity = config.Similarity
		extractor.patternIDPrefix = config.PatternIDPrefix
	}
	return extractor
}

func (s *StandardPatternExtractor) ExtractPattern(ctx context.Context, trace *actionhistory.ResourceActionTrace) (*vector.ActionPattern, error) {
	return &vector.ActionPattern{
		ID: fmt.Sprintf("%s-%d", s.patternIDPrefix, trace.ID),
	}, nil
}

func (s *StandardPatternExtractor) GenerateEmbedding(ctx context.Context, pattern *vector.ActionPattern) ([]float64, error) {
	return s.embedding, nil
}

func (s *StandardPatternExtractor) ExtractFeatures(ctx context.Context, pattern *vector.ActionPattern) (map[string]float64, error) {
	return s.features, nil
}

func (s *StandardPatternExtractor) CalculateSimilarity(pattern1, pattern2 *vector.ActionPattern) float64 {
	return s.similarity
}

// StandardPatternStore - the canonical pattern store for tests
type StandardPatternStore struct {
	patterns map[string]*shared.DiscoveredPattern
	logger   *logrus.Logger
}

// NewStandardPatternStore creates the standard pattern store for tests
func NewStandardPatternStore(logger *logrus.Logger) *StandardPatternStore {
	return &StandardPatternStore{
		patterns: make(map[string]*shared.DiscoveredPattern),
		logger:   logger,
	}
}

func (s *StandardPatternStore) StorePattern(ctx context.Context, pattern *shared.DiscoveredPattern) error {
	s.patterns[pattern.ID] = pattern
	s.logger.WithField("pattern_id", pattern.ID).Debug("Stored test pattern")
	return nil
}

// StoreEnginePattern stores a sharedtypes.DiscoveredPattern (for engine.PatternStore interface)
func (s *StandardPatternStore) StoreEnginePattern(ctx context.Context, pattern *sharedtypes.DiscoveredPattern) error {
	// Convert sharedtypes.DiscoveredPattern to shared.DiscoveredPattern for internal storage
	sharedPattern := &shared.DiscoveredPattern{
		BasePattern: sharedtypes.BasePattern{
			BaseEntity: sharedtypes.BaseEntity{
				ID:          pattern.ID,
				Description: pattern.Description,
				Metadata:    pattern.Metadata,
			},
			Confidence: pattern.Confidence,
		},
		PatternType: shared.PatternType(pattern.Type),
	}
	return s.StorePattern(ctx, sharedPattern)
}

// GetPattern retrieves a single pattern by ID (implements engine.PatternStore interface)
func (s *StandardPatternStore) GetPattern(ctx context.Context, patternID string) (*sharedtypes.DiscoveredPattern, error) {
	// Convert from shared.DiscoveredPattern to sharedtypes.DiscoveredPattern
	if pattern, exists := s.patterns[patternID]; exists {
		sharedTypesPattern := &sharedtypes.DiscoveredPattern{
			ID:          pattern.ID,
			Type:        string(pattern.PatternType),
			Confidence:  pattern.Confidence,
			Support:     0.8, // Default support value
			Description: pattern.Description,
			Metadata:    pattern.Metadata,
		}
		return sharedTypesPattern, nil
	}
	return nil, fmt.Errorf("pattern not found: %s", patternID)
}

// ListPatterns retrieves patterns by type (implements engine.PatternStore interface)
func (s *StandardPatternStore) ListPatterns(ctx context.Context, patternType string) ([]*sharedtypes.DiscoveredPattern, error) {
	var results []*sharedtypes.DiscoveredPattern
	for _, pattern := range s.patterns {
		if patternType == "" || string(pattern.PatternType) == patternType {
			sharedTypesPattern := &sharedtypes.DiscoveredPattern{
				ID:          pattern.ID,
				Type:        string(pattern.PatternType),
				Confidence:  pattern.Confidence,
				Support:     0.8, // Default support value
				Description: pattern.Description,
				Metadata:    pattern.Metadata,
			}
			results = append(results, sharedTypesPattern)
		}
	}
	return results, nil
}

func (s *StandardPatternStore) GetPatterns(ctx context.Context, filters map[string]interface{}) ([]*shared.DiscoveredPattern, error) {
	var results []*shared.DiscoveredPattern
	for _, pattern := range s.patterns {
		results = append(results, pattern)
	}
	return results, nil
}

func (s *StandardPatternStore) UpdatePattern(ctx context.Context, pattern *shared.DiscoveredPattern) error {
	s.patterns[pattern.ID] = pattern
	return nil
}

func (s *StandardPatternStore) DeletePattern(ctx context.Context, patternID string) error {
	delete(s.patterns, patternID)
	return nil
}

// StandardMLAnalyzer - the canonical ML analyzer for tests
type StandardMLAnalyzer struct {
	logger            *logrus.Logger
	prediction        *shared.WorkflowPrediction
	modelCount        int
	models            map[string]*patterns.MLModel
	shouldReturnError bool
}

// MLAnalyzerConfig holds configuration for ML analyzer mock behavior
type MLAnalyzerConfig struct {
	Prediction        *shared.WorkflowPrediction
	ModelCount        int
	Models            map[string]*patterns.MLModel
	ShouldReturnError bool
}

// NewStandardMLAnalyzer creates the standard ML analyzer for tests
func NewStandardMLAnalyzer(logger *logrus.Logger) *StandardMLAnalyzer {
	return &StandardMLAnalyzer{
		logger: logger,
		prediction: &shared.WorkflowPrediction{
			SuccessProbability: 0.85,
			ExpectedDuration:   5 * time.Minute,
			Confidence:         0.75,
			Reason:             "Standard test prediction based on historical patterns",
		},
		modelCount:        1,
		models:            make(map[string]*patterns.MLModel),
		shouldReturnError: false,
	}
}

// NewConfigurableMLAnalyzer creates a configurable ML analyzer for advanced tests
func NewConfigurableMLAnalyzer(logger *logrus.Logger, config *MLAnalyzerConfig) *StandardMLAnalyzer {
	analyzer := &StandardMLAnalyzer{logger: logger}
	if config != nil {
		analyzer.prediction = config.Prediction
		analyzer.modelCount = config.ModelCount
		analyzer.models = config.Models
		analyzer.shouldReturnError = config.ShouldReturnError
	}
	return analyzer
}

func (s *StandardMLAnalyzer) PredictOutcome(features *shared.WorkflowFeatures, patterns []*shared.DiscoveredPattern) (*shared.WorkflowPrediction, error) {
	if s.shouldReturnError {
		return nil, fmt.Errorf("mock ML analyzer error")
	}
	return s.prediction, nil
}

func (s *StandardMLAnalyzer) UpdateModel(learningData *shared.WorkflowLearningData) error {
	if s.shouldReturnError {
		return fmt.Errorf("mock model update error")
	}
	s.logger.WithField("learning_data", "test").Debug("Updated ML model with test data")
	return nil
}

func (s *StandardMLAnalyzer) GetModelCount() int {
	return s.modelCount
}

func (s *StandardMLAnalyzer) GetModels() map[string]*patterns.MLModel {
	return s.models
}

// StandardTimeSeriesAnalyzer - the canonical time series analyzer for tests
type StandardTimeSeriesAnalyzer struct {
	logger                *logrus.Logger
	trendAnalysis         *sharedtypes.TrendAnalysis
	anomalies             []*engine.AnomalyResult
	forecastResult        *engine.ForecastResult
	resourceTrendAnalysis *sharedtypes.ResourceTrendAnalysis
	temporalPatterns      []*sharedtypes.TemporalPatternAnalysis
	shouldReturnError     bool
}

// TimeSeriesAnalyzerConfig holds configuration for time series analyzer mock behavior
type TimeSeriesAnalyzerConfig struct {
	TrendAnalysis         *sharedtypes.TrendAnalysis
	Anomalies             []*engine.AnomalyResult
	ForecastResult        *engine.ForecastResult
	ResourceTrendAnalysis *sharedtypes.ResourceTrendAnalysis
	TemporalPatterns      []*sharedtypes.TemporalPatternAnalysis
	ShouldReturnError     bool
}

// NewStandardTimeSeriesAnalyzer creates the standard time series analyzer for tests
func NewStandardTimeSeriesAnalyzer(logger *logrus.Logger) *StandardTimeSeriesAnalyzer {
	return &StandardTimeSeriesAnalyzer{
		logger: logger,
		trendAnalysis: &sharedtypes.TrendAnalysis{
			Direction:  "stable",
			Slope:      0.1,
			Confidence: 0.8,
		},
		anomalies:      []*engine.AnomalyResult{},
		forecastResult: &engine.ForecastResult{Confidence: 0.8},
		resourceTrendAnalysis: &sharedtypes.ResourceTrendAnalysis{
			ResourceType:   "default",
			Confidence:     0.8,
			Significance:   0.7,
			Occurrences:    0,
			MetricPatterns: make(map[string]*sharedtypes.MetricPattern),
		},
		temporalPatterns: []*sharedtypes.TemporalPatternAnalysis{
			{
				PatternType:     "daily",
				Confidence:      0.8,
				PeakTimes:       []sharedtypes.PatternTimeRange{},
				SeasonalFactors: make(map[string]float64),
				CycleDuration:   24 * time.Hour,
			},
		},
		shouldReturnError: false,
	}
}

// NewConfigurableTimeSeriesAnalyzer creates a configurable time series analyzer for advanced tests
func NewConfigurableTimeSeriesAnalyzer(logger *logrus.Logger, config *TimeSeriesAnalyzerConfig) *StandardTimeSeriesAnalyzer {
	analyzer := &StandardTimeSeriesAnalyzer{logger: logger}
	if config != nil {
		analyzer.trendAnalysis = config.TrendAnalysis
		analyzer.anomalies = config.Anomalies
		analyzer.forecastResult = config.ForecastResult
		analyzer.resourceTrendAnalysis = config.ResourceTrendAnalysis
		analyzer.temporalPatterns = config.TemporalPatterns
		analyzer.shouldReturnError = config.ShouldReturnError
	}
	return analyzer
}

func (s *StandardTimeSeriesAnalyzer) AnalyzeTrends(ctx context.Context, data []*sharedtypes.WorkflowExecutionData, timeRange sharedtypes.TimeRange) (*sharedtypes.TrendAnalysis, error) {
	if s.shouldReturnError {
		return nil, fmt.Errorf("mock time series analyzer error")
	}
	return s.trendAnalysis, nil
}

func (s *StandardTimeSeriesAnalyzer) DetectAnomalies(ctx context.Context, data []*engine.EngineWorkflowExecutionData) ([]*engine.AnomalyResult, error) {
	if s.shouldReturnError {
		return nil, fmt.Errorf("mock anomaly detection error")
	}
	return s.anomalies, nil
}

func (s *StandardTimeSeriesAnalyzer) ForecastMetrics(ctx context.Context, data []*engine.EngineWorkflowExecutionData, horizonHours int) (*engine.ForecastResult, error) {
	if s.shouldReturnError {
		return nil, fmt.Errorf("mock forecast error")
	}
	return s.forecastResult, nil
}

func (s *StandardTimeSeriesAnalyzer) AnalyzeResourceTrends(ctx context.Context, data []*sharedtypes.WorkflowExecutionData, resourceType string, timeRange sharedtypes.TimeRange) (*sharedtypes.ResourceTrendAnalysis, error) {
	if s.shouldReturnError {
		return nil, fmt.Errorf("mock resource trends error")
	}

	// Create a copy with the requested resource type
	result := *s.resourceTrendAnalysis
	result.ResourceType = resourceType
	result.Occurrences = len(data)

	return &result, nil
}

func (s *StandardTimeSeriesAnalyzer) DetectTemporalPatterns(ctx context.Context, data []*sharedtypes.WorkflowExecutionData, timeRange sharedtypes.TimeRange) ([]*sharedtypes.TemporalPatternAnalysis, error) {
	if s.shouldReturnError {
		return nil, fmt.Errorf("mock temporal patterns error")
	}
	return s.temporalPatterns, nil
}

// StandardClusteringEngine - the canonical clustering engine for tests
type StandardClusteringEngine struct {
	logger *logrus.Logger
}

// NewStandardClusteringEngine creates the standard clustering engine for tests
func NewStandardClusteringEngine(logger *logrus.Logger) *StandardClusteringEngine {
	return &StandardClusteringEngine{logger: logger}
}

func (s *StandardClusteringEngine) ClusterWorkflows(ctx context.Context, executions []*sharedtypes.WorkflowExecutionData, config *sharedtypes.PatternDiscoveryConfig) ([]*sharedtypes.WorkflowCluster, error) {
	// Return a single cluster for simplicity in tests
	return []*sharedtypes.WorkflowCluster{
		{
			ID:      "test_cluster_1",
			Members: executions,
		},
	}, nil
}

func (s *StandardClusteringEngine) FindSimilarWorkflows(ctx context.Context, workflow *engine.Workflow, limit int) ([]*engine.SimilarWorkflow, error) {
	// Return empty similar workflows for standard tests
	return []*engine.SimilarWorkflow{}, nil
}

func (s *StandardClusteringEngine) ClusterAlerts(ctx context.Context, data []*sharedtypes.WorkflowExecutionData, config *sharedtypes.PatternDiscoveryConfig) ([]*sharedtypes.AlertCluster, error) {
	// Return a single test alert cluster
	return []*sharedtypes.AlertCluster{
		{
			ID:        "test_alert_cluster_1",
			AlertType: "test_alert",
			Members:   data,
			Centroid:  make(map[string]interface{}),
			Cohesion:  0.8,
		},
	}, nil
}

// StandardAnomalyDetector - the canonical anomaly detector for tests
type StandardAnomalyDetector struct {
	logger *logrus.Logger
}

// NewStandardAnomalyDetector creates the standard anomaly detector for tests
func NewStandardAnomalyDetector(logger *logrus.Logger) *StandardAnomalyDetector {
	return &StandardAnomalyDetector{logger: logger}
}

func (s *StandardAnomalyDetector) DetectAnomalies(ctx context.Context, data []*sharedtypes.WorkflowExecutionData, baseline []*sharedtypes.WorkflowExecutionData) ([]*sharedtypes.AnomalyResult, error) {
	// Return no anomalies for standard tests
	return []*sharedtypes.AnomalyResult{}, nil
}

func (s *StandardAnomalyDetector) GetBaseline(ctx context.Context, workflowType string) (*engine.BaselineStatistics, error) {
	return &engine.BaselineStatistics{}, nil
}

func (s *StandardAnomalyDetector) UpdateBaseline(ctx context.Context, executions []*engine.EngineWorkflowExecutionData) (*engine.BaselineStatistics, error) {
	return &engine.BaselineStatistics{}, nil
}

func (s *StandardAnomalyDetector) DetectAnomaly(ctx context.Context, execution *sharedtypes.WorkflowExecutionData, baseline []*sharedtypes.WorkflowExecutionData) (*sharedtypes.AnomalyResult, error) {
	// Return no anomaly for standard tests
	return &sharedtypes.AnomalyResult{
		ID:          "test-anomaly-1",
		Timestamp:   time.Now(),
		Severity:    "low",
		Description: "No anomaly detected in test execution",
	}, nil
}

// =============================================================================
// FACTORY FUNCTIONS FOR COMMON MOCK SETUPS
// =============================================================================

// CreateStandardPatternEngine creates a pattern engine with standard mocks
func CreateStandardPatternEngine(logger *logrus.Logger) *patterns.PatternDiscoveryEngine {
	// RULE 12 COMPLIANCE: Using enhanced llm.Client instead of deprecated ClusteringEngine
	return patterns.NewPatternDiscoveryEngine(
		NewStandardPatternStore(logger),
		nil, // Vector DB optional
		nil, // Execution repo optional
		NewStandardMLAnalyzer(logger),
		NewStandardTimeSeriesAnalyzer(logger),
		NewTestSLMClient(), // RULE 12 COMPLIANCE: Using enhanced llm.Client
		NewStandardAnomalyDetector(logger),
		&patterns.PatternDiscoveryConfig{
			MinExecutionsForPattern: 3,
			SimilarityThreshold:     0.7,
			EnableRealTimeDetection: true,
			MaxConcurrentAnalysis:   5,
		},
		logger,
	)
}

// CreatePerformancePatternEngine creates pattern engine optimized for performance tests
func CreatePerformancePatternEngine(logger *logrus.Logger) *patterns.PatternDiscoveryEngine {
	// RULE 12 COMPLIANCE: Using enhanced llm.Client instead of deprecated ClusteringEngine
	return patterns.NewPatternDiscoveryEngine(
		NewStandardPatternStore(logger),
		nil, // Vector DB optional
		nil, // Execution repo optional
		NewStandardMLAnalyzer(logger),
		NewStandardTimeSeriesAnalyzer(logger),
		NewTestSLMClient(), // RULE 12 COMPLIANCE: Using enhanced llm.Client
		NewStandardAnomalyDetector(logger),
		&patterns.PatternDiscoveryConfig{
			MinExecutionsForPattern: 3,
			SimilarityThreshold:     0.7,
			MaxConcurrentAnalysis:   20, // Higher concurrency for performance tests
			EnableRealTimeDetection: true,
			PatternCacheSize:        1000,
		},
		logger,
	)
}

// CreateMinimalPatternEngine creates a minimal pattern engine for fast tests
func CreateMinimalPatternEngine(logger *logrus.Logger) *patterns.PatternDiscoveryEngine {
	// RULE 12 COMPLIANCE: Using enhanced llm.Client instead of deprecated ClusteringEngine
	return patterns.NewPatternDiscoveryEngine(
		NewStandardPatternStore(logger),
		nil, // Vector DB optional
		nil, // Execution repo optional
		NewStandardMLAnalyzer(logger),
		NewStandardTimeSeriesAnalyzer(logger),
		NewTestSLMClient(), // RULE 12 COMPLIANCE: Using enhanced llm.Client
		NewStandardAnomalyDetector(logger),
		&patterns.PatternDiscoveryConfig{
			MinExecutionsForPattern: 1, // Lower threshold for fast tests
			SimilarityThreshold:     0.5,
			MaxConcurrentAnalysis:   1, // Single threaded for predictable tests
			EnableRealTimeDetection: false,
			PatternCacheSize:        10,
		},
		logger,
	)
}

// =============================================================================
// USAGE EXAMPLES FOR CONFIGURABLE MOCKS
// =============================================================================
//
// The mocks now support two patterns:
//
// 1. SIMPLE STATIC USAGE (backward compatible):
//    analyzer := NewStandardMLAnalyzer(logger)
//    // Returns default static values, good for basic tests
//
// 2. CONFIGURABLE USAGE (for advanced testing):
//    config := &MLAnalyzerConfig{
//        Prediction: &shared.WorkflowPrediction{
//            SuccessProbability: 0.95, // Custom value
//            Confidence: 0.9,
//        },
//        ShouldReturnError: true, // Test error scenarios
//    }
//    analyzer := NewConfigurableMLAnalyzer(logger, config)
//
// 3. ERROR TESTING:
//    config := &TimeSeriesAnalyzerConfig{
//        ShouldReturnError: true,
//    }
//    analyzer := NewConfigurableTimeSeriesAnalyzer(logger, config)
//    // All methods will now return errors for testing error handling
//
// 4. SPECIFIC SCENARIO TESTING:
//    config := &PatternExtractorConfig{
//        Embedding: []float64{0.9, 0.8, 0.7}, // High similarity embedding
//        Similarity: 0.95,                    // High similarity for clustering tests
//    }
//    extractor := NewConfigurablePatternExtractor(logger, config)
//
// =============================================================================
