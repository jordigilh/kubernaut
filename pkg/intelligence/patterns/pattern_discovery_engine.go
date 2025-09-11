package patterns

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/intelligence/shared"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

// MachineLearningAnalyzer interface defines ML analysis capabilities
type MachineLearningAnalyzer interface {
	PredictOutcome(features *shared.WorkflowFeatures, patterns []*shared.DiscoveredPattern) (*shared.WorkflowPrediction, error)
	UpdateModel(learningData *shared.WorkflowLearningData) error
	GetModelCount() int
	GetModels() map[string]*MLModel
}

// ExecutionRepository interface defines execution data access
type ExecutionRepository interface {
	GetExecutionsInTimeWindow(ctx context.Context, start, end time.Time) ([]*types.RuntimeWorkflowExecution, error)
}

// PatternDiscoveryEngine analyzes historical data to discover patterns and optimize workflows
type PatternDiscoveryEngine struct {
	patternStore     PatternStore
	vectorDB         PatternVectorDatabase
	executionRepo    ExecutionRepository
	mlAnalyzer       MachineLearningAnalyzer
	timeSeriesEngine types.TimeSeriesAnalyzer
	clusteringEngine types.ClusteringEngine
	anomalyDetector  types.AnomalyDetector
	log              *logrus.Logger
	config           *PatternDiscoveryConfig
	mu               sync.RWMutex
	activePatterns   map[string]*shared.DiscoveredPattern
	learningMetrics  *LearningMetrics
}

// PatternDiscoveryConfig configures the pattern discovery engine
type PatternDiscoveryConfig struct {
	// Data Collection
	MinExecutionsForPattern int           `yaml:"min_executions_for_pattern" default:"10"`
	MaxHistoryDays          int           `yaml:"max_history_days" default:"90"`
	SamplingInterval        time.Duration `yaml:"sampling_interval" default:"1h"`

	// Pattern Detection
	SimilarityThreshold float64 `yaml:"similarity_threshold" default:"0.85"`
	ClusteringEpsilon   float64 `yaml:"clustering_epsilon" default:"0.3"`
	MinClusterSize      int     `yaml:"min_cluster_size" default:"5"`

	// Machine Learning
	ModelUpdateInterval  time.Duration `yaml:"model_update_interval" default:"24h"`
	FeatureWindowSize    int           `yaml:"feature_window_size" default:"50"`
	PredictionConfidence float64       `yaml:"prediction_confidence" default:"0.7"`

	// Performance
	MaxConcurrentAnalysis   int  `yaml:"max_concurrent_analysis" default:"10"`
	PatternCacheSize        int  `yaml:"pattern_cache_size" default:"1000"`
	EnableRealTimeDetection bool `yaml:"enable_realtime_detection" default:"true"`
}

// DiscoveredPattern is defined in shared package

// PatternType is defined in shared package

// AlertPattern is defined in shared package

// MetricPattern represents patterns in resource metrics
type MetricPattern struct {
	MetricName      string    `json:"metric_name"`
	Pattern         string    `json:"pattern"` // "increasing", "decreasing", "oscillating", "stable"
	Threshold       float64   `json:"threshold"`
	Anomalies       []float64 `json:"anomalies"`
	TrendConfidence float64   `json:"trend_confidence"`
}

// ResourcePattern is defined in shared package

// TemporalPattern is defined in shared package

// FailurePattern is defined in shared package

// OptimizationHint is defined in shared package

// PatternAnalysisRequest defines analysis parameters
type PatternAnalysisRequest struct {
	AnalysisType  string                 `json:"analysis_type"`
	TimeRange     PatternTimeRange       `json:"time_range"`
	Filters       map[string]interface{} `json:"filters"`
	PatternTypes  []shared.PatternType   `json:"pattern_types"`
	MinConfidence float64                `json:"min_confidence"`
	MaxResults    int                    `json:"max_results"`
}

// PatternAnalysisResult contains discovered patterns
type PatternAnalysisResult struct {
	RequestID       string                      `json:"request_id"`
	Patterns        []*shared.DiscoveredPattern `json:"patterns"`
	AnalysisMetrics *AnalysisMetrics            `json:"analysis_metrics"`
	Recommendations []*PatternRecommendation    `json:"recommendations"`
	NextAnalysis    *time.Time                  `json:"next_analysis,omitempty"`
}

// NewPatternDiscoveryEngine creates a new pattern discovery engine
func NewPatternDiscoveryEngine(
	patternStore PatternStore,
	vectorDB PatternVectorDatabase,
	executionRepo ExecutionRepository,
	mlAnalyzer MachineLearningAnalyzer,
	timeSeriesEngine types.TimeSeriesAnalyzer,
	clusteringEngine types.ClusteringEngine,
	anomalyDetector types.AnomalyDetector,
	config *PatternDiscoveryConfig,
	log *logrus.Logger,
) *PatternDiscoveryEngine {
	// Use shared constructor pattern for default configuration
	defaultConfig := PatternDiscoveryConfig{
		MinExecutionsForPattern: 10,
		MaxHistoryDays:          90,
		SamplingInterval:        time.Hour,
		SimilarityThreshold:     0.85,
		ClusteringEpsilon:       0.3,
		MinClusterSize:          5,
		ModelUpdateInterval:     24 * time.Hour,
		FeatureWindowSize:       50,
		PredictionConfidence:    0.7,
		MaxConcurrentAnalysis:   10,
		PatternCacheSize:        1000,
		EnableRealTimeDetection: true,
	}

	if config == nil {
		config = &defaultConfig
	}

	engine := &PatternDiscoveryEngine{
		patternStore:     patternStore,
		vectorDB:         vectorDB,
		executionRepo:    executionRepo,
		mlAnalyzer:       mlAnalyzer,
		timeSeriesEngine: timeSeriesEngine,
		clusteringEngine: clusteringEngine,
		anomalyDetector:  anomalyDetector,
		config:           config,
		log:              log,
		activePatterns:   make(map[string]*shared.DiscoveredPattern),
		learningMetrics:  NewLearningMetrics(),
	}

	return engine
}

// DiscoverPatterns analyzes historical data to find patterns
func (pde *PatternDiscoveryEngine) DiscoverPatterns(ctx context.Context, request *PatternAnalysisRequest) (*PatternAnalysisResult, error) {
	pde.log.WithFields(logrus.Fields{
		"analysis_type": request.AnalysisType,
		"time_range":    request.TimeRange,
		"pattern_types": request.PatternTypes,
	}).Info("Starting pattern discovery analysis")

	startTime := time.Now()

	// Collect historical data
	historicalData, err := pde.collectHistoricalData(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to collect historical data: %w", err)
	}

	pde.log.WithField("data_points", len(historicalData)).Info("Historical data collected")

	// Perform different types of analysis based on request
	patterns := make([]*shared.DiscoveredPattern, 0)

	for _, patternType := range request.PatternTypes {
		typePatterns, err := pde.analyzePatternType(ctx, historicalData, patternType, request)
		if err != nil {
			pde.log.WithError(err).WithField("pattern_type", patternType).Error("Pattern analysis failed")
			continue
		}
		patterns = append(patterns, typePatterns...)
	}

	// Filter by confidence threshold
	filteredPatterns := pde.filterByConfidence(patterns, request.MinConfidence)

	// Rank and limit results
	rankedPatterns := pde.rankPatterns(filteredPatterns)
	if request.MaxResults > 0 && len(rankedPatterns) > request.MaxResults {
		rankedPatterns = rankedPatterns[:request.MaxResults]
	}

	// Generate recommendations
	recommendations := pde.generateRecommendations(rankedPatterns)

	// Store discovered patterns
	for _, pattern := range rankedPatterns {
		if err := pde.storePattern(ctx, pattern); err != nil {
			pde.log.WithError(err).WithField("pattern_id", pattern.ID).Error("Failed to store pattern")
		}
	}

	analysisTime := time.Since(startTime)

	result := &PatternAnalysisResult{
		RequestID: fmt.Sprintf("analysis-%d", time.Now().Unix()),
		Patterns:  rankedPatterns,
		AnalysisMetrics: &AnalysisMetrics{
			DataPointsAnalyzed:     len(historicalData),
			PatternsFound:          len(rankedPatterns),
			AnalysisTime:           analysisTime,
			ConfidenceDistribution: pde.calculateConfidenceDistribution(rankedPatterns),
		},
		Recommendations: recommendations,
	}

	// Update learning metrics
	pde.learningMetrics.RecordAnalysis(result)

	pde.log.WithFields(logrus.Fields{
		"patterns_found":  len(rankedPatterns),
		"recommendations": len(recommendations),
		"analysis_time":   analysisTime,
	}).Info("Pattern discovery analysis completed")

	return result, nil
}

// AnalyzeRealTimeEvent processes events in real-time for pattern detection
func (pde *PatternDiscoveryEngine) AnalyzeRealTimeEvent(ctx context.Context, event *WorkflowExecutionEvent) ([]*shared.DiscoveredPattern, error) {
	if !pde.config.EnableRealTimeDetection {
		return nil, nil
	}

	pde.log.WithFields(logrus.Fields{
		"event_type":   event.Type,
		"workflow_id":  event.WorkflowID,
		"execution_id": event.ExecutionID,
	}).Debug("Analyzing real-time event for patterns")

	// Quick pattern matching against known patterns
	matchingPatterns := make([]*shared.DiscoveredPattern, 0)

	pde.mu.RLock()
	for _, pattern := range pde.activePatterns {
		if pde.eventMatchesPattern(event, pattern) {
			// Update pattern metrics
			pattern.LastSeen = time.Now()
			pattern.Frequency++
			matchingPatterns = append(matchingPatterns, pattern)
		}
	}
	pde.mu.RUnlock()

	// Check for anomalies using the new DetectAnomaly method
	if pde.anomalyDetector != nil {
		// Convert event to WorkflowExecutionData properly
		executionData := &types.WorkflowExecutionData{
			ExecutionID: event.ExecutionID,
			WorkflowID:  event.WorkflowID,
			Timestamp:   event.Timestamp,
			Success:     event.Type != "error", // Determine success based on event type
			Metrics:     event.Metrics,
			Metadata:    event.Context,
		}

		anomaly, err := pde.anomalyDetector.DetectAnomaly(ctx, executionData, nil)
		if err == nil && anomaly != nil {
			anomalyPattern := pde.createAnomalyPattern(event, anomaly)
			matchingPatterns = append(matchingPatterns, anomalyPattern)
		}
	}

	// Update vector embeddings for future similarity searches
	if err := pde.updateVectorEmbeddings(ctx, event); err != nil {
		pde.log.WithError(err).Warn("Failed to update vector embeddings")
	}

	return matchingPatterns, nil
}

// PredictWorkflowOutcome uses patterns to predict workflow success
func (pde *PatternDiscoveryEngine) PredictWorkflowOutcome(ctx context.Context, template *types.TemplateSpec, alert *types.Alert) (*shared.WorkflowPrediction, error) {
	pde.log.WithFields(logrus.Fields{
		"template_id": template.ID,
		"alert_name":  alert.Name,
	}).Info("Predicting workflow outcome")

	// Extract features from current context
	features := pde.extractPredictionFeatures(template, alert)

	// Find similar historical patterns
	similarPatterns, err := pde.findSimilarPatterns(ctx, features)
	if err != nil {
		return nil, fmt.Errorf("failed to find similar patterns: %w", err)
	}

	if len(similarPatterns) == 0 {
		return &shared.WorkflowPrediction{
			Confidence:         0.5,
			SuccessProbability: 0.5,
			Reason:             "No historical patterns found",
		}, nil
	}

	// Use ML model for prediction (similarPatterns should already be shared.DiscoveredPattern)
	prediction, err := pde.mlAnalyzer.PredictOutcome(features, similarPatterns)
	if err != nil {
		return nil, fmt.Errorf("ML prediction failed: %w", err)
	}

	// Enhance prediction with pattern insights
	prediction.SimilarPatterns = len(similarPatterns)
	prediction.OptimizationSuggestions = pde.generateOptimizationSuggestions(similarPatterns, template)

	return prediction, nil
}

// OptimizeWorkflowTemplate suggests improvements based on patterns
func (pde *PatternDiscoveryEngine) OptimizeWorkflowTemplate(ctx context.Context, template *types.TemplateSpec) (*OptimizedWorkflowTemplate, error) {
	pde.log.WithField("template_id", template.ID).Info("Optimizing workflow template")

	// Find patterns related to this template type
	relevantPatterns, err := pde.findRelevantPatterns(ctx, template)
	if err != nil {
		return nil, fmt.Errorf("failed to find relevant patterns: %w", err)
	}

	optimizedTemplate := &OptimizedWorkflowTemplate{
		OriginalTemplate:  template,
		OptimizedTemplate: template, // Keep as shared template
		Optimizations:     make([]*TemplateOptimization, 0),
	}

	// Apply optimizations based on patterns
	for _, pattern := range relevantPatterns {
		for _, hint := range pattern.OptimizationHints {
			optimization := pde.applyOptimizationHintSafe(optimizedTemplate.OptimizedTemplate, hint, pattern)
			if optimization != nil {
				optimizedTemplate.Optimizations = append(optimizedTemplate.Optimizations, optimization)
			}
		}
	}

	// Calculate optimization impact
	optimizedTemplate.ImpactEstimate = pde.calculateOptimizationImpact(optimizedTemplate.Optimizations)

	return optimizedTemplate, nil
}

// LearnFromExecution updates patterns based on execution results
func (pde *PatternDiscoveryEngine) LearnFromExecution(ctx context.Context, execution *types.RuntimeWorkflowExecution) error {
	pde.log.WithFields(logrus.Fields{
		"execution_id": execution.ID,
		"template_id":  execution.WorkflowID,
		"success":      execution.OperationalStatus == types.ExecutionStatusCompleted,
	}).Info("Learning from workflow execution")

	// Extract learning data from execution
	learningData := pde.extractLearningData(&execution.WorkflowExecutionRecord)

	// Update existing patterns
	if err := pde.updateExistingPatterns(ctx, learningData); err != nil {
		pde.log.WithError(err).Error("Failed to update existing patterns")
	}

	// Check for new pattern emergence
	if err := pde.checkForNewPatterns(ctx, learningData); err != nil {
		pde.log.WithError(err).Error("Failed to check for new patterns")
	}

	// Update ML models
	if err := pde.mlAnalyzer.UpdateModel(learningData); err != nil {
		pde.log.WithError(err).Error("Failed to update ML model")
	}

	// Update vector embeddings
	if err := pde.updateVectorDatabase(ctx, learningData); err != nil {
		pde.log.WithError(err).Error("Failed to update vector database")
	}

	// Record learning metrics
	pde.learningMetrics.RecordExecution(&execution.WorkflowExecutionRecord)

	return nil
}

// GetPatternInsights provides insights about discovered patterns
func (pde *PatternDiscoveryEngine) GetPatternInsights(ctx context.Context, filters map[string]interface{}) (*PatternInsights, error) {
	patterns, err := pde.patternStore.GetPatterns(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to get patterns: %w", err)
	}

	insights := &PatternInsights{
		TotalPatterns:       len(patterns),
		PatternDistribution: pde.calculatePatternDistribution(patterns),
		ConfidenceStats:     pde.calculateConfidenceStats(patterns),
		TemporalTrends:      pde.analyzeTemporalTrends(patterns),
		TopOptimizations:    pde.getTopOptimizations(patterns),
		LearningMetrics:     pde.learningMetrics,
	}

	return insights, nil
}

// Private helper methods

func (pde *PatternDiscoveryEngine) collectHistoricalData(ctx context.Context, request *PatternAnalysisRequest) ([]*types.WorkflowExecutionData, error) {
	pde.log.WithFields(logrus.Fields{
		"time_range":    request.TimeRange,
		"analysis_type": request.AnalysisType,
		"filters":       request.Filters,
	}).Info("Collecting historical workflow execution data")

	// Validate request parameters
	if err := pde.validateAnalysisRequest(request); err != nil {
		return nil, fmt.Errorf("invalid analysis request: %w", err)
	}

	// Collect data from multiple sources with error handling
	var allData []*types.WorkflowExecutionData
	var collectionErrors []error

	// Try to collect from pattern store first
	if pde.patternStore != nil {
		storeData, err := pde.collectFromPatternStore(ctx, request)
		if err != nil {
			collectionErrors = append(collectionErrors, fmt.Errorf("pattern store collection failed: %w", err))
			pde.log.WithError(err).Warn("Failed to collect from pattern store")
		} else {
			allData = append(allData, storeData...)
			pde.log.WithField("count", len(storeData)).Debug("Collected data from pattern store")
		}
	}

	// Try to collect from vector database
	if pde.vectorDB != nil {
		vectorData, err := pde.collectFromVectorDB(ctx, request)
		if err != nil {
			collectionErrors = append(collectionErrors, fmt.Errorf("vector DB collection failed: %w", err))
			pde.log.WithError(err).Warn("Failed to collect from vector database")
		} else {
			allData = append(allData, vectorData...)
			pde.log.WithField("count", len(vectorData)).Debug("Collected data from vector database")
		}
	}

	// Try to collect from execution repository
	if pde.executionRepo != nil {
		repoData, err := pde.collectFromExecutionRepository(ctx, request)
		if err != nil {
			collectionErrors = append(collectionErrors, fmt.Errorf("execution repository collection failed: %w", err))
			pde.log.WithError(err).Warn("Failed to collect from execution repository")
		} else {
			allData = append(allData, repoData...)
			pde.log.WithField("count", len(repoData)).Debug("Collected data from execution repository")
		}
	}

	// Use historical data buffer as fallback
	if len(allData) == 0 && pde.anomalyDetector != nil {
		bufferData := pde.collectFromHistoricalBuffer(request)
		allData = append(allData, bufferData...)
		pde.log.WithField("count", len(bufferData)).Debug("Collected data from historical buffer")
	}

	// Deduplicate data by execution ID
	allData = pde.deduplicateExecutionData(allData)
	pde.log.WithField("after_dedup", len(allData)).Debug("Deduplicated execution data")

	// Validate and clean collected data
	cleanData, validationErrors := pde.validateAndCleanData(allData)
	if len(validationErrors) > 0 {
		pde.log.WithField("validation_errors", len(validationErrors)).Warn("Data validation issues found")
		for _, err := range validationErrors {
			pde.log.WithError(err).Debug("Data validation error")
		}
	}

	// Apply filters
	filteredData := pde.applyRequestFilters(cleanData, request)

	// Check if we have sufficient data
	if len(filteredData) < pde.config.MinExecutionsForPattern {
		return nil, fmt.Errorf("insufficient historical data: found %d executions, need at least %d",
			len(filteredData), pde.config.MinExecutionsForPattern)
	}

	// Log collection summary
	pde.log.WithFields(logrus.Fields{
		"total_collected":   len(allData),
		"after_validation":  len(cleanData),
		"after_filtering":   len(filteredData),
		"collection_errors": len(collectionErrors),
		"validation_errors": len(validationErrors),
	}).Info("Historical data collection completed")

	return filteredData, nil
}

func (pde *PatternDiscoveryEngine) analyzePatternType(ctx context.Context, data []*types.WorkflowExecutionData, patternType shared.PatternType, request *PatternAnalysisRequest) ([]*shared.DiscoveredPattern, error) {
	switch patternType {
	case shared.PatternTypeAlert:
		return pde.analyzeAlertPatterns(ctx, data)
	case shared.PatternTypeResource:
		return pde.analyzeResourcePatterns(ctx, data)
	case shared.PatternTypeTemporal:
		return pde.analyzeTemporalPatterns(ctx, data)
	case shared.PatternTypeFailure:
		return pde.analyzeFailurePatterns(data)
	case shared.PatternTypeOptimization:
		return pde.analyzeOptimizationPatterns(data)
	case shared.PatternTypeAnomaly:
		return pde.analyzeAnomalyPatterns(data)
	case shared.PatternTypeWorkflow:
		return pde.analyzeWorkflowPatterns(data)
	default:
		return nil, fmt.Errorf("unknown pattern type: %s", patternType)
	}
}

func (pde *PatternDiscoveryEngine) analyzeAlertPatterns(ctx context.Context, data []*types.WorkflowExecutionData) ([]*shared.DiscoveredPattern, error) {
	pde.log.WithField("data_count", len(data)).Debug("Analyzing alert patterns")
	patterns := make([]*shared.DiscoveredPattern, 0)

	if len(data) < pde.config.MinExecutionsForPattern {
		return patterns, nil
	}

	// Multi-layered alert pattern analysis
	var clusterPatterns, temporalPatterns, resourcePatterns, severityPatterns []*shared.DiscoveredPattern

	// 1. Alert clustering analysis with improved validation
	if pde.clusteringEngine != nil {
		// Use the new ClusterAlerts method for alert-specific clustering
		alertClusters, err := pde.clusteringEngine.ClusterAlerts(ctx, data, &types.PatternDiscoveryConfig{
			MinSupport:      pde.config.SimilarityThreshold,
			MinConfidence:   pde.config.PredictionConfidence,
			MaxPatterns:     pde.config.PatternCacheSize,
			TimeWindowHours: pde.config.MaxHistoryDays * 24,
		})
		if err == nil {
			clusterPatterns = pde.processAlertClusters(alertClusters)
			patterns = append(patterns, clusterPatterns...)
		}
	}

	// 2. Temporal correlation analysis
	temporalPatterns = pde.analyzeAlertTemporalCorrelations(data)
	patterns = append(patterns, temporalPatterns...)

	// 3. Resource affinity analysis
	resourcePatterns = pde.analyzeAlertResourceAffinity(data)
	patterns = append(patterns, resourcePatterns...)

	// 4. Severity progression analysis
	severityPatterns = pde.analyzeAlertSeverityProgressions(data)
	patterns = append(patterns, severityPatterns...)

	// Validate and enhance discovered patterns
	validatedPatterns := pde.validateAlertPatterns(patterns, data)

	pde.log.WithFields(logrus.Fields{
		"discovered_patterns": len(patterns),
		"validated_patterns":  len(validatedPatterns),
		"cluster_patterns":    len(clusterPatterns),
		"temporal_patterns":   len(temporalPatterns),
		"resource_patterns":   len(resourcePatterns),
		"severity_patterns":   len(severityPatterns),
	}).Info("Alert pattern analysis completed")

	return validatedPatterns, nil
}

// processAlertClusters processes alert cluster results into patterns
func (pde *PatternDiscoveryEngine) processAlertClusters(alertClusters []*types.AlertCluster) []*shared.DiscoveredPattern {
	patterns := make([]*shared.DiscoveredPattern, 0)

	for _, cluster := range alertClusters {
		if len(cluster.Members) >= pde.config.MinClusterSize {
			// Create the pattern using the new AlertCluster structure
			pattern := &shared.DiscoveredPattern{}
			pattern.ID = cluster.ID
			pattern.Type = string(shared.PatternTypeAlert)
			pattern.Name = fmt.Sprintf("Alert Cluster: %s", cluster.AlertType)
			pattern.Description = fmt.Sprintf("Alert cluster with %d members (cohesion: %.2f)", len(cluster.Members), cluster.Cohesion)
			pattern.Confidence = cluster.Cohesion
			pattern.Frequency = cluster.Frequency
			pattern.PatternType = shared.PatternTypeAlert
			pattern.AlertPatterns = []*shared.AlertPattern{{
				AlertTypes:    []string{cluster.AlertType},
				LabelPatterns: pde.extractLabelsFromCluster(cluster),
				TimeWindow:    time.Hour, // Default time window
				Correlation:   pde.calculateAlertCorrelationFromCluster(cluster),
			}}
			pattern.CreatedAt = time.Now()
			patterns = append(patterns, pattern)
		}
	}

	return patterns
}

// analyzeAlertTemporalCorrelations finds alerts that occur in temporal sequences
func (pde *PatternDiscoveryEngine) analyzeAlertTemporalCorrelations(data []*types.WorkflowExecutionData) []*shared.DiscoveredPattern {
	patterns := make([]*shared.DiscoveredPattern, 0)

	// Group alerts by time windows
	timeWindows := pde.groupAlertsByTimeWindows(data, 30*time.Minute)

	for windowStart, windowAlerts := range timeWindows {
		if len(windowAlerts) >= 3 { // Need at least 3 alerts for temporal pattern
			sequence := pde.analyzeAlertSequence(windowAlerts)
			if sequence.IsSignificant {
				pattern := &shared.DiscoveredPattern{}
				pattern.ID = fmt.Sprintf("temporal-alert-%d", windowStart.Unix())
				pattern.Type = string(shared.PatternTypeAlert)
				pattern.PatternType = shared.PatternTypeAlert
				pattern.Name = "Temporal Alert Sequence"
				pattern.Description = fmt.Sprintf("Alert sequence pattern with %d alerts in 30min window", len(windowAlerts))
				pattern.Confidence = sequence.Confidence
				pattern.Frequency = 1 // Single occurrence for now
				pattern.SuccessRate = sequence.SuccessRate
				pattern.AlertPatterns = []*shared.AlertPattern{{
					AlertTypes: sequence.AlertTypes,
					TimeWindow: 30 * time.Minute,
				}}
				pattern.TemporalPatterns = []*shared.TemporalPattern{{
					Pattern:       "sequence",
					PeakTimes:     []shared.PatternTimeRange{{Start: windowStart, End: windowStart.Add(30 * time.Minute)}},
					CycleDuration: sequence.AverageDuration,
				}}
				pattern.CreatedAt = time.Now()
				pattern.Metrics = map[string]float64{
					"sequence_length":     float64(len(windowAlerts)),
					"average_interval":    sequence.AverageInterval.Seconds(),
					"severity_escalation": sequence.SeverityEscalation,
				}
				patterns = append(patterns, pattern)
			}
		}
	}

	return patterns
}

// analyzeAlertResourceAffinity finds alerts that frequently affect the same resources
func (pde *PatternDiscoveryEngine) analyzeAlertResourceAffinity(data []*types.WorkflowExecutionData) []*shared.DiscoveredPattern {
	patterns := make([]*shared.DiscoveredPattern, 0)

	// Group by resource
	resourceGroups := make(map[string][]*types.WorkflowExecutionData)

	for _, execution := range data {
		alert := pde.extractAlertFromMetadata(execution)
		if alert != nil {
			resourceKey := fmt.Sprintf("%s/%s", alert.Namespace, alert.Resource)
			if _, exists := resourceGroups[resourceKey]; !exists {
				resourceGroups[resourceKey] = make([]*types.WorkflowExecutionData, 0)
			}
			resourceGroups[resourceKey] = append(resourceGroups[resourceKey], execution)
		}
	}

	for resourceKey, executions := range resourceGroups {
		if len(executions) >= pde.config.MinClusterSize {
			affinity := pde.calculateResourceAffinity(executions)
			if affinity.IsSignificant {
				pattern := &shared.DiscoveredPattern{}
				pattern.ID = fmt.Sprintf("resource-affinity-%s", strings.ReplaceAll(resourceKey, "/", "-"))
				pattern.Type = string(shared.PatternTypeAlert)
				pattern.PatternType = shared.PatternTypeAlert
				pattern.Name = fmt.Sprintf("Resource Affinity: %s", resourceKey)
				pattern.Description = fmt.Sprintf("Resource %s shows high alert affinity", resourceKey)
				pattern.Confidence = affinity.Confidence
				pattern.Frequency = len(executions)
				pattern.SuccessRate = affinity.SuccessRate
				pattern.AlertPatterns = []*shared.AlertPattern{{
					Namespaces: []string{affinity.Namespace},
					Resources:  []string{affinity.Resource},
					AlertTypes: affinity.AlertTypes,
				}}
				pattern.ResourcePatterns = []*shared.ResourcePattern{{
					ResourceType: affinity.ResourceType,
				}}
				pattern.CreatedAt = time.Now()
				pattern.Metrics = map[string]float64{
					"alert_frequency":      float64(len(executions)),
					"failure_rate":         1.0 - affinity.SuccessRate,
					"resource_utilization": affinity.AverageUtilization,
				}
				patterns = append(patterns, pattern)
			}
		}
	}

	return patterns
}

// analyzeAlertSeverityProgressions finds patterns in severity escalation
func (pde *PatternDiscoveryEngine) analyzeAlertSeverityProgressions(data []*types.WorkflowExecutionData) []*shared.DiscoveredPattern {
	patterns := make([]*shared.DiscoveredPattern, 0)

	// Sort by timestamp
	sortedData := make([]*types.WorkflowExecutionData, len(data))
	copy(sortedData, data)
	sort.Slice(sortedData, func(i, j int) bool {
		return sortedData[i].Timestamp.Before(sortedData[j].Timestamp)
	})

	// Look for severity escalation sequences
	escalations := pde.findSeverityEscalations(sortedData)

	for _, escalation := range escalations {
		if escalation.IsSignificant {
			pattern := &shared.DiscoveredPattern{}
			pattern.ID = fmt.Sprintf("severity-escalation-%d", escalation.StartTime.Unix())
			pattern.Type = string(shared.PatternTypeAlert)
			pattern.PatternType = shared.PatternTypeAlert
			pattern.Name = "Severity Escalation Pattern"
			pattern.Description = fmt.Sprintf("Escalation from %s to %s over %v", escalation.StartSeverity, escalation.EndSeverity, escalation.Duration)
			pattern.Confidence = escalation.Confidence
			pattern.Frequency = 1
			pattern.SuccessRate = escalation.ResolutionRate
			pattern.AlertPatterns = []*shared.AlertPattern{{
				SeverityPattern: escalation.Pattern,
				TimeWindow:      escalation.Duration,
			}}
			pattern.FailurePatterns = []*shared.FailurePattern{{
				PropagationTime:    escalation.Duration,
				AffectedComponents: escalation.AffectedResources,
			}}
			pattern.CreatedAt = time.Now()
			pattern.Metrics = map[string]float64{
				"escalation_speed":   escalation.EscalationSpeed,
				"resolution_time":    escalation.ResolutionTime.Seconds(),
				"affected_resources": float64(len(escalation.AffectedResources)),
			}
			patterns = append(patterns, pattern)
		}
	}

	return patterns
}

// validateAlertPatterns validates discovered alert patterns against the data
func (pde *PatternDiscoveryEngine) validateAlertPatterns(patterns []*shared.DiscoveredPattern, data []*types.WorkflowExecutionData) []*shared.DiscoveredPattern {
	validated := make([]*shared.DiscoveredPattern, 0)

	for _, pattern := range patterns {
		// Validate pattern confidence through cross-validation
		confidence := pde.crossValidatePattern(pattern, data)

		// Update pattern confidence with validation results
		pattern.Confidence = (pattern.Confidence + confidence) / 2.0

		// Only keep patterns with sufficient confidence
		if pattern.Confidence >= pde.config.PredictionConfidence {
			// Add validation metadata
			pattern.Metrics["validation_confidence"] = confidence
			pattern.Metrics["validation_samples"] = float64(len(data))
			pattern.UpdatedAt = time.Now()

			validated = append(validated, pattern)
		}
	}

	return validated
}

func (pde *PatternDiscoveryEngine) analyzeResourcePatterns(ctx context.Context, data []*types.WorkflowExecutionData) ([]*shared.DiscoveredPattern, error) {
	// Implement resource utilization pattern analysis
	// Analyze CPU, memory, disk, network patterns
	// Identify scaling triggers and capacity patterns
	patterns := make([]*shared.DiscoveredPattern, 0)

	// Use the new AnalyzeResourceTrends method for resource-specific analysis
	timeRange := types.TimeRange{Start: time.Now().Add(-24 * time.Hour), End: time.Now()}

	// Analyze different resource types
	resourceTypes := []string{"cpu", "memory", "disk", "network"}
	var resourceAnalysis []*types.ResourceTrendAnalysis

	for _, resourceType := range resourceTypes {
		analysis, err := pde.timeSeriesEngine.AnalyzeResourceTrends(ctx, data, resourceType, timeRange)
		if err != nil {
			pde.log.WithError(err).WithField("resource_type", resourceType).Warn("Failed to analyze resource trends")
			continue
		}
		resourceAnalysis = append(resourceAnalysis, analysis)
	}

	for _, analysis := range resourceAnalysis {
		if analysis.Significance > pde.config.SimilarityThreshold {
			pattern := &shared.DiscoveredPattern{}
			pattern.ID = fmt.Sprintf("resource-pattern-%s-%d", analysis.ResourceType, time.Now().Unix())
			pattern.Type = string(shared.PatternTypeResource)
			pattern.PatternType = shared.PatternTypeResource
			pattern.Name = fmt.Sprintf("Resource Pattern: %s (%s trend)", analysis.ResourceType, analysis.TrendDirection)
			pattern.Confidence = analysis.Confidence
			pattern.Frequency = analysis.Occurrences
			pattern.ResourcePatterns = []*shared.ResourcePattern{{
				ResourceType:   analysis.ResourceType,
				MetricPatterns: pde.convertMetricPatterns(analysis.MetricPatterns),
			}}
			pattern.CreatedAt = time.Now()
			patterns = append(patterns, pattern)
		}
	}

	return patterns, nil
}

func (pde *PatternDiscoveryEngine) analyzeTemporalPatterns(ctx context.Context, data []*types.WorkflowExecutionData) ([]*shared.DiscoveredPattern, error) {
	// Implement temporal pattern analysis
	// Find daily, weekly, monthly cycles
	// Detect burst patterns and seasonal variations
	patterns := make([]*shared.DiscoveredPattern, 0)

	// Use the new DetectTemporalPatterns method for comprehensive temporal analysis
	timeRange := types.TimeRange{Start: time.Now().Add(-168 * time.Hour), End: time.Now()} // 7 days
	temporalAnalyses, err := pde.timeSeriesEngine.DetectTemporalPatterns(ctx, data, timeRange)
	if err != nil {
		return patterns, err
	}

	for _, analysis := range temporalAnalyses {
		if analysis.Confidence < pde.config.SimilarityThreshold {
			continue
		}
		pattern := &shared.DiscoveredPattern{}
		pattern.ID = fmt.Sprintf("temporal-pattern-%d", time.Now().Unix())
		pattern.Type = string(shared.PatternTypeTemporal)
		pattern.PatternType = shared.PatternTypeTemporal
		pattern.Name = fmt.Sprintf("Temporal Pattern: %s", analysis.PatternType)
		pattern.Confidence = analysis.Confidence
		pattern.TemporalPatterns = []*shared.TemporalPattern{{
			Pattern:         analysis.PatternType,
			PeakTimes:       pde.convertPatternTimeRanges(analysis.PeakTimes),
			SeasonalFactors: analysis.SeasonalFactors,
			CycleDuration:   analysis.CycleDuration,
		}}
		pattern.CreatedAt = time.Now()
		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

func (pde *PatternDiscoveryEngine) analyzeFailurePatterns(data []*types.WorkflowExecutionData) ([]*shared.DiscoveredPattern, error) {
	// Implement failure chain analysis
	// Identify cascading failures and root causes
	// Map failure propagation patterns
	patterns := make([]*shared.DiscoveredPattern, 0)

	failureAnalysis := pde.analyzeFailureChains(data)

	for _, chain := range failureAnalysis {
		if len(chain.Nodes) >= 2 { // At least 2 nodes for a meaningful chain
			pattern := &shared.DiscoveredPattern{
				BasePattern: types.BasePattern{
					BaseEntity: types.BaseEntity{
						ID:          fmt.Sprintf("failure-pattern-%d", time.Now().Unix()),
						Name:        fmt.Sprintf("Failure Chain: %s → %s", chain.RootCause, chain.FinalEffect),
						Description: fmt.Sprintf("Detected failure chain with %d nodes", len(chain.Nodes)),
						CreatedAt:   time.Now(),
						UpdatedAt:   time.Now(),
						Metadata:    map[string]interface{}{"pattern_type": shared.PatternTypeFailure},
					},
					Confidence: chain.Confidence,
					Frequency:  chain.Occurrences,
				},
				PatternType: shared.PatternTypeFailure,
				FailurePatterns: []*shared.FailurePattern{{
					FailureChain:       pde.convertFailureNodes(chain.Nodes),
					RootCause:          chain.RootCause,
					PropagationTime:    chain.PropagationTime,
					AffectedComponents: chain.AffectedComponents,
				}},
				DiscoveredAt: time.Now(),
			}
			patterns = append(patterns, pattern)
		}
	}

	return patterns, nil
}

// Additional helper types and methods...

type AlertClusterGroup struct {
	Members      []*types.WorkflowExecutionData
	AlertTypes   []string
	Namespaces   []string
	Resources    []string
	TimeWindow   time.Duration
	CommonLabels map[string]string
	Confidence   float64
	SuccessRate  float64
}

// ResourceTrendAnalysis and TemporalAnalysis now consolidated in pkg/intelligence/shared/types.go

type FailureChainAnalysis struct {
	Nodes              []*shared.FailureNode
	RootCause          string
	FinalEffect        string
	PropagationTime    time.Duration
	AffectedComponents []string
	Confidence         float64
	Occurrences        int
}

// Supporting interfaces and types...

type PatternStore interface {
	StorePattern(ctx context.Context, pattern *shared.DiscoveredPattern) error
	GetPatterns(ctx context.Context, filters map[string]interface{}) ([]*shared.DiscoveredPattern, error)
	UpdatePattern(ctx context.Context, pattern *shared.DiscoveredPattern) error
	DeletePattern(ctx context.Context, patternID string) error
}

// PatternVectorDatabase interface for pattern discovery - uses the unified vector database interface
// This interface should align with the main VectorDatabase interface in pkg/storage/vector
type PatternVectorDatabase interface {
	Store(ctx context.Context, id string, vector []float64, metadata map[string]interface{}) error
	Search(ctx context.Context, vector []float64, limit int) (*UnifiedSearchResultSet, error)
	Update(ctx context.Context, id string, vector []float64, metadata map[string]interface{}) error
}

// VectorSearchResult is deprecated - use UnifiedSearchResult from the vector package instead
type VectorSearchResult = UnifiedSearchResult

// UnifiedSearchResultSet type alias for integration with the unified vector system
type UnifiedSearchResultSet = vector.UnifiedSearchResultSet

// UnifiedSearchResult type alias for integration with the unified vector system
type UnifiedSearchResult = vector.UnifiedSearchResult

// Placeholder methods that need proper implementation

func (pde *PatternDiscoveryEngine) convertClustersToAlertGroups(clusters []*types.WorkflowCluster) []*AlertClusterGroup {
	groups := make([]*AlertClusterGroup, 0, len(clusters))
	for _, cluster := range clusters {
		group := &AlertClusterGroup{
			Members:      cluster.Members,
			AlertTypes:   []string{"generic"},
			Namespaces:   []string{"default"},
			Resources:    []string{"unknown"},
			TimeWindow:   time.Hour,
			CommonLabels: make(map[string]string),
			Confidence:   0.7,
			SuccessRate:  0.8,
		}
		groups = append(groups, group)
	}
	return groups
}

// applyOptimizationHintSafe applies optimization hints directly without unnecessary conversions
func (pde *PatternDiscoveryEngine) applyOptimizationHintSafe(template *types.TemplateSpec, hint *shared.OptimizationHint, pattern *shared.DiscoveredPattern) *TemplateOptimization {
	optimization := &TemplateOptimization{
		ID:                  fmt.Sprintf("opt-%s-%d", hint.Type, time.Now().Unix()),
		Type:                hint.Type,
		Description:         hint.Description,
		Rationale:           hint.ActionSuggestion,
		ExpectedImprovement: hint.ImpactEstimate,
		RiskLevel:           pde.calculateRiskLevel(hint),
	}

	// Apply specific optimizations based on hint type
	switch hint.Type {
	case "timeout_optimization":
		pde.optimizeTimeoutsDirect(template, optimization)
	case "step_parallelization":
		pde.optimizeParallelizationDirect(template, optimization)
	case "retry_policy":
		pde.optimizeRetryPolicyDirect(template, optimization)
	default:
		optimization.Description = fmt.Sprintf("Generic optimization: %s", hint.Description)
	}

	return optimization
}

// calculateRiskLevel determines risk level for optimization
func (pde *PatternDiscoveryEngine) calculateRiskLevel(hint *shared.OptimizationHint) string {
	if hint.ImplementationCost > 0.8 {
		return "high"
	} else if hint.ImplementationCost > 0.5 {
		return "medium"
	}
	return "low"
}

// Direct optimization methods that work with types.TemplateSpec
func (pde *PatternDiscoveryEngine) optimizeTimeoutsDirect(template *types.TemplateSpec, optimization *TemplateOptimization) {
	// Record timeout optimization without modifying the template
	optimization.Description = fmt.Sprintf("Optimize timeouts for template %s", template.Name)
	optimization.ExpectedImprovement = 0.15 // 15% improvement estimate
}

func (pde *PatternDiscoveryEngine) optimizeParallelizationDirect(template *types.TemplateSpec, optimization *TemplateOptimization) {
	// Analyze steps that can be parallelized
	parallelizable := 0
	for _, step := range template.Steps {
		if len(step.Dependencies) == 0 {
			parallelizable++
		}
	}
	optimization.Description = fmt.Sprintf("Identified %d steps that can be parallelized", parallelizable)
	optimization.ExpectedImprovement = 0.3 // 30% improvement estimate
}

func (pde *PatternDiscoveryEngine) optimizeRetryPolicyDirect(template *types.TemplateSpec, optimization *TemplateOptimization) {
	optimization.Description = "Optimize retry policies for better reliability"
	optimization.ExpectedImprovement = 0.2 // 20% improvement estimate
}

// validatePatternConfidence provides simple confidence validation for shared.DiscoveredPattern
func (pde *PatternDiscoveryEngine) validatePatternConfidence(pattern *shared.DiscoveredPattern, data []*types.WorkflowExecutionData) float64 {
	if len(data) < 5 {
		return 0.5 // Default confidence for insufficient data
	}

	// Simple validation based on pattern frequency and type
	baseConfidence := pattern.Confidence

	// Adjust based on pattern type
	switch pattern.PatternType {
	case shared.PatternTypeAlert:
		baseConfidence *= 0.9 // Alert patterns are generally reliable
	case shared.PatternTypeTemporal:
		baseConfidence *= 0.8 // Temporal patterns need more validation
	case shared.PatternTypeFailure:
		baseConfidence *= 0.7 // Failure patterns are complex
	default:
		baseConfidence *= 0.75
	}

	// Adjust based on frequency
	if pattern.Frequency > 10 {
		baseConfidence *= 1.1
	} else if pattern.Frequency < 3 {
		baseConfidence *= 0.8
	}

	return math.Max(0.1, math.Min(0.95, baseConfidence))
}

// convertFailureNodes converts local FailureNode to shared.FailureNode
func (pde *PatternDiscoveryEngine) convertFailureNodes(nodes []*shared.FailureNode) []*shared.FailureNode {
	converted := make([]*shared.FailureNode, len(nodes))
	for i, node := range nodes {
		converted[i] = &shared.FailureNode{
			ID:           node.ID,
			Type:         node.Type,
			Component:    node.Component,
			FailureTime:  node.FailureTime,
			RecoveryTime: node.RecoveryTime,
			Impact:       node.Impact,
			RootCause:    node.RootCause,
			Dependencies: node.Dependencies,
			Metadata:     node.Metadata,
		}
	}
	return converted
}

// convertPatternTimeRanges converts types.PatternTimeRange to shared.PatternTimeRange
func (pde *PatternDiscoveryEngine) convertPatternTimeRanges(ranges []types.PatternTimeRange) []shared.PatternTimeRange {
	converted := make([]shared.PatternTimeRange, len(ranges))
	for i, r := range ranges {
		converted[i] = shared.PatternTimeRange{
			Start: r.Start,
			End:   r.End,
		}
	}
	return converted
}

// convertMetricPatterns converts types.MetricPattern to shared.MetricPattern
func (pde *PatternDiscoveryEngine) convertMetricPatterns(patterns map[string]*types.MetricPattern) map[string]*shared.MetricPattern {
	converted := make(map[string]*shared.MetricPattern)
	for key, pattern := range patterns {
		converted[key] = &shared.MetricPattern{
			MetricName:      pattern.MetricName,
			Pattern:         pattern.Pattern,
			Threshold:       pattern.AverageValue, // Map average to threshold
			Anomalies:       []float64{},          // Initialize empty anomalies
			TrendConfidence: pattern.Confidence,   // Map confidence to trend confidence
		}
	}
	return converted
}

// Helper methods for new alert cluster processing

// extractLabelsFromCluster extracts common labels from cluster members
func (pde *PatternDiscoveryEngine) extractLabelsFromCluster(cluster *types.AlertCluster) map[string]string {
	if cluster.Centroid == nil {
		return make(map[string]string)
	}

	labels := make(map[string]string)
	if centroidLabels, ok := cluster.Centroid["labels"].(map[string]string); ok {
		return centroidLabels
	}

	return labels
}

// calculateAlertCorrelationFromCluster calculates correlation from new cluster format
func (pde *PatternDiscoveryEngine) calculateAlertCorrelationFromCluster(cluster *types.AlertCluster) *shared.AlertCorrelation {
	return &shared.AlertCorrelation{
		PrimaryAlert:     cluster.AlertType,
		CorrelatedAlerts: []string{}, // Could be expanded based on cluster analysis
		CorrelationScore: cluster.Cohesion,
		TimeWindow:       30 * time.Minute,
		Direction:        "concurrent",
		Confidence:       cluster.Cohesion,
	}
}
