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

// WorkflowExecutionEvent represents real-time events during workflow execution
// This is semantically different from PatternAnalysisExecutionData which represents completed execution summaries
type WorkflowExecutionEvent struct {
	Type        string                 `json:"type"`              // Event type: "step_start", "step_complete", "error", "alert_triggered", etc.
	WorkflowID  string                 `json:"workflow_id"`       // ID of the workflow being executed
	ExecutionID string                 `json:"execution_id"`      // ID of this specific execution instance
	StepID      string                 `json:"step_id,omitempty"` // ID of current step (if applicable)
	Timestamp   time.Time              `json:"timestamp"`         // When this event occurred
	Data        map[string]interface{} `json:"data"`              // Event-specific payload (alert details, step results, etc.)
	Metrics     map[string]float64     `json:"metrics"`           // Real-time metrics snapshot at event time
	Context     map[string]interface{} `json:"context"`           // Execution context (environment, user, etc.)
}

// Removed unused PatternAnalysisExecutionData struct per development guidelines

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
// Removed unused AnalyzeRealTimeEvent function per development guidelines

// PredictWorkflowOutcome uses patterns to predict workflow success
// Removed unused PredictWorkflowOutcome function per development guidelines

// OptimizeWorkflowTemplate suggests improvements based on patterns
// Removed unused OptimizeWorkflowTemplate function per development guidelines

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
	// Apply request-specific filtering to data before analysis
	filteredData := data
	if request != nil {
		// Filter by time window if specified
		if !request.TimeRange.Start.IsZero() && !request.TimeRange.End.IsZero() {
			timeFiltered := make([]*types.WorkflowExecutionData, 0)
			for _, item := range filteredData {
				if !item.Timestamp.Before(request.TimeRange.Start) &&
					!item.Timestamp.After(request.TimeRange.End) {
					timeFiltered = append(timeFiltered, item)
				}
			}
			filteredData = timeFiltered
		}

		// Apply additional filters if specified
		if len(request.Filters) > 0 {
			additionalFiltered := make([]*types.WorkflowExecutionData, 0)
			for _, item := range filteredData {
				// Check namespace filter
				if namespaceFilter, ok := request.Filters["namespace"].(string); ok {
					if item.Metadata != nil {
						if ns, hasNs := item.Metadata["namespace"].(string); hasNs && ns == namespaceFilter {
							additionalFiltered = append(additionalFiltered, item)
						}
					}
				} else {
					additionalFiltered = append(additionalFiltered, item)
				}
			}
			filteredData = additionalFiltered
		}
	}

	var patterns []*shared.DiscoveredPattern
	var err error

	switch patternType {
	case shared.PatternTypeAlert:
		patterns, err = pde.analyzeAlertPatterns(ctx, filteredData)
	case shared.PatternTypeResource:
		patterns, err = pde.analyzeResourcePatterns(ctx, filteredData)
	case shared.PatternTypeTemporal:
		patterns, err = pde.analyzeTemporalPatterns(ctx, filteredData)
	case shared.PatternTypeFailure:
		patterns, err = pde.analyzeFailurePatterns(filteredData)
	case shared.PatternTypeOptimization:
		patterns, err = pde.analyzeOptimizationPatterns(filteredData)
	case shared.PatternTypeAnomaly:
		patterns, err = pde.analyzeAnomalyPatterns(filteredData)
	case shared.PatternTypeWorkflow:
		patterns, err = pde.analyzeWorkflowPatterns(filteredData)
	default:
		return nil, fmt.Errorf("unknown pattern type: %s", patternType)
	}

	if err != nil {
		return nil, err
	}

	// Apply request-specific limits and sorting
	if request != nil && request.MaxResults > 0 && len(patterns) > request.MaxResults {
		// Sort by confidence and take top results
		for i := 0; i < len(patterns); i++ {
			for j := i + 1; j < len(patterns); j++ {
				if patterns[i].Confidence < patterns[j].Confidence {
					patterns[i], patterns[j] = patterns[j], patterns[i]
				}
			}
		}
		patterns = patterns[:request.MaxResults]
	}

	return patterns, nil
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

// Helper methods for PatternDiscoveryEngine

// filterByConfidence filters patterns by minimum confidence threshold
func (pde *PatternDiscoveryEngine) filterByConfidence(patterns []*shared.DiscoveredPattern, minConfidence float64) []*shared.DiscoveredPattern {
	filtered := make([]*shared.DiscoveredPattern, 0)
	for _, pattern := range patterns {
		if pattern.Confidence >= minConfidence {
			filtered = append(filtered, pattern)
		}
	}
	return filtered
}

// rankPatterns ranks patterns by confidence and frequency
func (pde *PatternDiscoveryEngine) rankPatterns(patterns []*shared.DiscoveredPattern) []*shared.DiscoveredPattern {
	if len(patterns) == 0 {
		return patterns
	}

	// Sort by composite score (confidence * frequency + success rate)
	sort.Slice(patterns, func(i, j int) bool {
		scoreI := patterns[i].Confidence*0.4 + float64(patterns[i].Frequency)*0.3 + patterns[i].SuccessRate*0.3
		scoreJ := patterns[j].Confidence*0.4 + float64(patterns[j].Frequency)*0.3 + patterns[j].SuccessRate*0.3
		return scoreI > scoreJ
	})

	return patterns
}

// generateRecommendations generates pattern recommendations
func (pde *PatternDiscoveryEngine) generateRecommendations(patterns []*shared.DiscoveredPattern) []*PatternRecommendation {
	recommendations := make([]*PatternRecommendation, 0)

	for i, pattern := range patterns {
		if i >= 5 { // Limit to top 5 patterns
			break
		}

		recommendation := &PatternRecommendation{
			ID:               fmt.Sprintf("rec-%s", pattern.ID),
			Type:             string(pattern.Type),
			Title:            fmt.Sprintf("Optimize based on %s pattern", pattern.Type),
			Description:      fmt.Sprintf("Pattern: %s shows %d occurrences with %.2f confidence", pattern.Name, pattern.Frequency, pattern.Confidence),
			Impact:           pde.estimateImpact(pattern),
			Effort:           pde.estimateEffort(pattern),
			Priority:         i + 1,
			BasedOnPatterns:  []string{pattern.ID},
			EstimatedBenefit: pattern.Confidence * pattern.SuccessRate,
		}

		recommendations = append(recommendations, recommendation)
	}

	return recommendations
}

// storePattern stores a discovered pattern
func (pde *PatternDiscoveryEngine) storePattern(ctx context.Context, pattern *shared.DiscoveredPattern) error {
	if pde.patternStore == nil {
		pde.log.Warn("Pattern store not configured")
		return nil
	}

	return pde.patternStore.StorePattern(ctx, pattern)
}

// calculateConfidenceDistribution calculates confidence distribution
func (pde *PatternDiscoveryEngine) calculateConfidenceDistribution(patterns []*shared.DiscoveredPattern) map[string]int {
	distribution := map[string]int{
		"high":   0, // 0.8+
		"medium": 0, // 0.5-0.8
		"low":    0, // <0.5
	}

	for _, pattern := range patterns {
		if pattern.Confidence >= 0.8 {
			distribution["high"]++
		} else if pattern.Confidence >= 0.5 {
			distribution["medium"]++
		} else {
			distribution["low"]++
		}
	}

	return distribution
}

// extractLearningData extracts learning data from execution
func (pde *PatternDiscoveryEngine) extractLearningData(execution *types.WorkflowExecutionRecord) *shared.WorkflowLearningData {
	return &shared.WorkflowLearningData{
		ExecutionID:       execution.ID,
		TemplateID:        execution.WorkflowID,
		LearningObjective: "pattern_discovery",
		Context: map[string]interface{}{
			"success": execution.Status == string(types.ExecutionStatusCompleted),
			"duration": func() float64 {
				if execution.EndTime != nil {
					return execution.EndTime.Sub(execution.StartTime).Seconds()
				}
				return 0.0
			}(),
			"steps_executed": 0, // WorkflowExecutionRecord doesn't have Steps field
		},
	}
}

// updateExistingPatterns updates existing patterns with new data
func (pde *PatternDiscoveryEngine) updateExistingPatterns(ctx context.Context, learningData *shared.WorkflowLearningData) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	pde.mu.Lock()
	defer pde.mu.Unlock()

	if learningData == nil {
		return nil
	}

	for _, pattern := range pde.activePatterns {
		// Update pattern metrics based on learning data
		pattern.LastSeen = time.Now()
		pattern.UpdatedAt = time.Now()
		pattern.Frequency++

		// Use learning data context to update confidence if available
		if learningData.Context != nil {
			// Extract success indicator from context
			if success, ok := learningData.Context["success"].(bool); ok {
				if success {
					// Successful executions increase confidence
					pattern.Confidence = (pattern.Confidence * 0.9) + (0.1 * 1.0)
				} else {
					// Failed executions decrease confidence
					pattern.Confidence = (pattern.Confidence * 0.9) + (0.1 * 0.0)
				}
			}

			// Cap confidence between 0.1 and 0.95
			if pattern.Confidence > 0.95 {
				pattern.Confidence = 0.95
			} else if pattern.Confidence < 0.1 {
				pattern.Confidence = 0.1
			}

			// Update execution metrics if available
			if execTime, ok := learningData.Context["execution_time_ms"].(float64); ok && execTime > 0 {
				if pattern.AverageExecutionTime == 0 {
					pattern.AverageExecutionTime = time.Duration(execTime) * time.Millisecond
				} else {
					// Moving average
					newTime := time.Duration(execTime) * time.Millisecond
					pattern.AverageExecutionTime = (pattern.AverageExecutionTime*9 + newTime) / 10
				}
			}
		}
	}

	return nil
}

// checkForNewPatterns checks for emergence of new patterns
func (pde *PatternDiscoveryEngine) checkForNewPatterns(ctx context.Context, learningData *shared.WorkflowLearningData) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if learningData == nil {
		pde.log.Debug("No learning data provided for new pattern detection")
		return nil
	}

	pde.log.WithFields(logrus.Fields{
		"execution_id":  learningData.ExecutionID,
		"template_id":   learningData.TemplateID,
		"learning_type": learningData.LearningObjective,
		"context_keys":  len(learningData.Context),
	}).Debug("Checking for new patterns based on learning data")

	// Simple implementation: check if the execution represents a new template pattern
	if learningData.TemplateID != "" && pde.shouldCreateNewPattern(learningData) {
		// Create a basic pattern for tracking purposes
		newPattern := &shared.DiscoveredPattern{
			BasePattern: types.BasePattern{
				BaseEntity: types.BaseEntity{
					ID:        fmt.Sprintf("learned-pattern-%d", time.Now().Unix()),
					Name:      fmt.Sprintf("Pattern from template %s", learningData.TemplateID),
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				},
				Type:       "learned_template",
				Confidence: 0.3, // Start with low confidence for new patterns
				Frequency:  1,
				LastSeen:   time.Now(),
			},
			PatternType:  shared.PatternTypeWorkflow,
			DiscoveredAt: time.Now(),
		}

		// Add execution time if available from context
		if execTime, ok := learningData.Context["execution_time_ms"].(float64); ok && execTime > 0 {
			newPattern.AverageExecutionTime = time.Duration(execTime) * time.Millisecond
		}

		// Add to active patterns
		pde.mu.Lock()
		pde.activePatterns[newPattern.ID] = newPattern
		pde.mu.Unlock()

		pde.log.WithField("pattern_id", newPattern.ID).Info("Created new pattern from learning data")
	}

	return nil
}

// shouldCreateNewPattern determines if learning data should create a new pattern
func (pde *PatternDiscoveryEngine) shouldCreateNewPattern(learningData *shared.WorkflowLearningData) bool {
	if learningData == nil {
		return false
	}

	// Only create patterns for data with basic required fields
	if learningData.TemplateID == "" || learningData.ExecutionID == "" {
		return false
	}

	// Check context for success indicator
	success := false
	if learningData.Context != nil {
		if s, ok := learningData.Context["success"].(bool); ok {
			success = s
		}
	}

	// Check context for execution time
	execTime := float64(0)
	if learningData.Context != nil {
		if et, ok := learningData.Context["execution_time_ms"].(float64); ok {
			execTime = et
		}
	}

	// Don't create patterns for failed one-off actions unless they're significant
	if !success && execTime < 1000 {
		return false
	}

	// Create pattern if it's a successful execution or a significant failure
	return success || execTime > 5000
}

// updateVectorDatabase updates the vector database
func (pde *PatternDiscoveryEngine) updateVectorDatabase(ctx context.Context, learningData *shared.WorkflowLearningData) error {
	if pde.vectorDB == nil {
		return nil
	}

	// Create vector representation
	vector := make([]float64, 10) // Simplified vector
	for i := range vector {
		vector[i] = float64(i) * 0.1 // Placeholder values
	}

	metadata := map[string]interface{}{
		"execution_id": learningData.ExecutionID,
		"template_id":  learningData.TemplateID,
		"timestamp":    time.Now(),
	}

	return pde.vectorDB.Store(ctx, learningData.ExecutionID, vector, metadata)
}

// calculatePatternDistribution calculates pattern type distribution
func (pde *PatternDiscoveryEngine) calculatePatternDistribution(patterns []*shared.DiscoveredPattern) map[string]int {
	distribution := make(map[string]int)

	for _, pattern := range patterns {
		distribution[string(pattern.Type)]++
	}

	return distribution
}

// calculateConfidenceStats calculates confidence statistics
func (pde *PatternDiscoveryEngine) calculateConfidenceStats(patterns []*shared.DiscoveredPattern) *ConfidenceStatistics {
	if len(patterns) == 0 {
		return &ConfidenceStatistics{}
	}

	confidences := make([]float64, len(patterns))
	sum := 0.0
	min := 1.0
	max := 0.0

	for i, pattern := range patterns {
		confidences[i] = pattern.Confidence
		sum += pattern.Confidence
		if pattern.Confidence < min {
			min = pattern.Confidence
		}
		if pattern.Confidence > max {
			max = pattern.Confidence
		}
	}

	mean := sum / float64(len(patterns))

	// Calculate standard deviation
	sumSquares := 0.0
	for _, conf := range confidences {
		sumSquares += (conf - mean) * (conf - mean)
	}
	stdDev := math.Sqrt(sumSquares / float64(len(patterns)))

	// Sort for median
	sort.Float64s(confidences)
	median := confidences[len(confidences)/2]

	// Count high/low confidence
	highCount := 0
	lowCount := 0
	for _, conf := range confidences {
		if conf >= 0.8 {
			highCount++
		} else if conf < 0.5 {
			lowCount++
		}
	}

	return &ConfidenceStatistics{
		Mean:                mean,
		Median:              median,
		StandardDeviation:   stdDev,
		Min:                 min,
		Max:                 max,
		HighConfidenceCount: highCount,
		LowConfidenceCount:  lowCount,
	}
}

// analyzeTemporalTrends analyzes temporal trends in patterns
func (pde *PatternDiscoveryEngine) analyzeTemporalTrends(patterns []*shared.DiscoveredPattern) *TemporalTrendAnalysis {
	if len(patterns) == 0 {
		return &TemporalTrendAnalysis{
			OverallTrend: "stable",
		}
	}

	// Simple trend analysis based on discovery times
	recent := 0
	old := 0
	now := time.Now()

	for _, pattern := range patterns {
		if now.Sub(pattern.DiscoveredAt) < 7*24*time.Hour {
			recent++
		} else {
			old++
		}
	}

	trend := "stable"
	if recent > old*2 {
		trend = "increasing"
	} else if old > recent*2 {
		trend = "decreasing"
	}

	return &TemporalTrendAnalysis{
		OverallTrend:    trend,
		TrendStrength:   0.5, // Placeholder
		TrendConfidence: 0.7, // Placeholder
	}
}

// getTopOptimizations gets top optimization opportunities
func (pde *PatternDiscoveryEngine) getTopOptimizations(patterns []*shared.DiscoveredPattern) []*OptimizationInsight {
	insights := make([]*OptimizationInsight, 0)

	for i, pattern := range patterns {
		if i >= 5 { // Limit to top 5
			break
		}

		insight := &OptimizationInsight{
			Area:                     "workflow_efficiency",
			PotentialImprovement:     pattern.Confidence * pde.getPatternSuccessRate(pattern),
			ImplementationDifficulty: "medium",
			Priority:                 i + 1,
			AffectedWorkflows:        pde.getPatternFrequency(pattern),
			EstimatedROI:             pattern.Confidence * 2.0,
		}

		insights = append(insights, insight)
	}

	return insights
}

// analyzeOptimizationPatterns analyzes optimization patterns
func (pde *PatternDiscoveryEngine) analyzeOptimizationPatterns(data []*types.WorkflowExecutionData) ([]*shared.DiscoveredPattern, error) {
	patterns := make([]*shared.DiscoveredPattern, 0)

	// Analyze execution times for optimization opportunities
	durationMap := make(map[string][]time.Duration)

	for _, execution := range data {
		key := execution.WorkflowID
		if _, exists := durationMap[key]; !exists {
			durationMap[key] = make([]time.Duration, 0)
		}
		durationMap[key] = append(durationMap[key], execution.Duration)
	}

	// Find templates with high variance in execution time
	for templateID, durations := range durationMap {
		if len(durations) >= 5 {
			variance := pde.calculateDurationVariance(durations)
			if variance > 0.5 { // High variance threshold
				pattern := &shared.DiscoveredPattern{
					BasePattern: types.BasePattern{
						BaseEntity: types.BaseEntity{
							ID:          fmt.Sprintf("opt-pattern-%s", templateID),
							Description: "High variance in execution time",
							CreatedAt:   time.Now(),
							UpdatedAt:   time.Now(),
							Metadata:    map[string]interface{}{"pattern_type": shared.PatternTypeOptimization},
						},
						Confidence: variance,
					},
					PatternType:  shared.PatternTypeOptimization,
					DiscoveredAt: time.Now(),
				}
				patterns = append(patterns, pattern)
			}
		}
	}

	return patterns, nil
}

// analyzeAnomalyPatterns analyzes anomaly patterns
func (pde *PatternDiscoveryEngine) analyzeAnomalyPatterns(data []*types.WorkflowExecutionData) ([]*shared.DiscoveredPattern, error) {
	patterns := make([]*shared.DiscoveredPattern, 0)

	// Simple anomaly detection based on success rate
	successRate := pde.calculateOverallSuccessRate(data)

	if successRate < 0.7 { // Low success rate threshold
		pattern := &shared.DiscoveredPattern{
			BasePattern: types.BasePattern{
				BaseEntity: types.BaseEntity{
					ID:          fmt.Sprintf("anomaly-pattern-%d", time.Now().Unix()),
					Description: "Anomaly detected in real-time workflow execution",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
					Metadata:    map[string]interface{}{"pattern_type": shared.PatternTypeAnomaly},
				},
				Confidence: 1.0 - successRate,
			},
			PatternType:  shared.PatternTypeAnomaly,
			DiscoveredAt: time.Now(),
		}
		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

// analyzeWorkflowPatterns analyzes workflow effectiveness patterns
func (pde *PatternDiscoveryEngine) analyzeWorkflowPatterns(data []*types.WorkflowExecutionData) ([]*shared.DiscoveredPattern, error) {
	patterns := make([]*shared.DiscoveredPattern, 0)

	// Group by template ID
	templateGroups := make(map[string][]*types.WorkflowExecutionData)

	for _, execution := range data {
		if _, exists := templateGroups[execution.WorkflowID]; !exists {
			templateGroups[execution.WorkflowID] = make([]*types.WorkflowExecutionData, 0)
		}
		templateGroups[execution.WorkflowID] = append(templateGroups[execution.WorkflowID], execution)
	}

	// Analyze each template group
	for templateID, executions := range templateGroups {
		if len(executions) >= 5 {
			successRate := pde.calculateSuccessRateForGroup(executions)

			pattern := &shared.DiscoveredPattern{
				BasePattern: types.BasePattern{
					BaseEntity: types.BaseEntity{
						ID:          fmt.Sprintf("workflow-pattern-%s", templateID),
						Description: "Workflow effectiveness pattern",
						CreatedAt:   time.Now(),
						UpdatedAt:   time.Now(),
						Metadata:    map[string]interface{}{"pattern_type": shared.PatternTypeWorkflow},
					},
					Confidence: successRate,
				},
				PatternType:  shared.PatternTypeWorkflow,
				DiscoveredAt: time.Now(),
			}
			patterns = append(patterns, pattern)
		}
	}

	return patterns, nil
}

// analyzeFailureChains analyzes failure propagation chains
func (pde *PatternDiscoveryEngine) analyzeFailureChains(data []*types.WorkflowExecutionData) []*FailureChainAnalysis {
	chains := make([]*FailureChainAnalysis, 0)

	// Group failures by time windows
	failures := make([]*types.WorkflowExecutionData, 0)
	for _, execution := range data {
		if !execution.Success {
			failures = append(failures, execution)
		}
	}

	if len(failures) >= 2 {
		// Simple chain analysis - consecutive failures within time window
		for i := 0; i < len(failures)-1; i++ {
			timeDiff := failures[i+1].Timestamp.Sub(failures[i].Timestamp)
			if timeDiff < time.Hour { // Failures within 1 hour
				chain := &FailureChainAnalysis{
					Nodes: []*shared.FailureNode{
						{
							ID:          failures[i].ExecutionID,
							Type:        "execution_failure",
							Component:   failures[i].WorkflowID,
							FailureTime: failures[i].Timestamp,
						},
						{
							ID:          failures[i+1].ExecutionID,
							Type:        "execution_failure",
							Component:   failures[i+1].WorkflowID,
							FailureTime: failures[i+1].Timestamp,
						},
					},
					RootCause:       "Unknown",
					FinalEffect:     "Cascading failure",
					PropagationTime: timeDiff,
					Confidence:      0.6,
					Occurrences:     1,
				}
				chains = append(chains, chain)
			}
		}
	}

	return chains
}

// Helper utility methods

// extractAlertFromMetadata extracts alert information from execution metadata
func (pde *PatternDiscoveryEngine) extractAlertFromMetadata(execution *types.WorkflowExecutionData) *types.Alert {
	alertData, hasAlert := execution.Metadata["alert"]
	if !hasAlert {
		return nil
	}
	if alertMap, ok := alertData.(map[string]interface{}); ok {
		alert := &types.Alert{}
		if name, ok := alertMap["name"].(string); ok {
			alert.Name = name
		}
		if severity, ok := alertMap["severity"].(string); ok {
			alert.Severity = severity
		}
		if namespace, ok := alertMap["namespace"].(string); ok {
			alert.Namespace = namespace
		}
		if resource, ok := alertMap["resource"].(string); ok {
			alert.Resource = resource
		}
		if labels, ok := alertMap["labels"].(map[string]string); ok {
			alert.Labels = labels
		}
		return alert
	}
	return nil
}

// Helper functions for pattern field access since engine.DiscoveredPattern may not have all fields

// getPatternName returns a pattern name, using ID if Name field doesn't exist
// Removed unused getPatternName function per development guidelines

// getPatternFrequency returns pattern frequency, defaulting to 1
func (pde *PatternDiscoveryEngine) getPatternFrequency(pattern *shared.DiscoveredPattern) int {
	// Return the frequency from shared.DiscoveredPattern
	return pattern.Frequency
}

// getPatternSuccessRate returns pattern success rate, using confidence as fallback
func (pde *PatternDiscoveryEngine) getPatternSuccessRate(pattern *shared.DiscoveredPattern) float64 {
	// Return the success rate from shared.DiscoveredPattern
	return pattern.SuccessRate
}

func (pde *PatternDiscoveryEngine) estimateImpact(pattern *shared.DiscoveredPattern) string {
	frequency := pde.getPatternFrequency(pattern)
	if pattern.Confidence > 0.8 && frequency > 10 {
		return "high"
	} else if pattern.Confidence > 0.6 || frequency > 5 {
		return "medium"
	}
	return "low"
}

func (pde *PatternDiscoveryEngine) estimateEffort(pattern *shared.DiscoveredPattern) string {
	switch strings.ToLower(string(pattern.Type)) {
	case "alert":
		return "low"
	case "temporal":
		return "medium"
	case "workflow":
		return "high"
	default:
		return "medium"
	}
}

// Removed unused matchesTemporalPattern function per development guidelines

// Removed unused matchesResourcePattern function per development guidelines

// Removed unused createEventVector function per development guidelines

// Removed unused featuresToVector function per development guidelines

// Removed unused calculateDependencyDepth function per development guidelines

// Removed unused optimization functions per development guidelines:
// - optimizeTimeouts(), optimizeParallelization(), optimizeRetryPolicy()
// These were only called from the removed _applyOptimizationHint() dead code path.
// The active code path uses optimizeTimeoutsDirect(), optimizeParallelizationDirect(), optimizeRetryPolicyDirect()

func (pde *PatternDiscoveryEngine) calculateDurationVariance(durations []time.Duration) float64 {
	if len(durations) < 2 {
		return 0
	}

	// Convert to seconds and calculate variance
	seconds := make([]float64, len(durations))
	sum := 0.0

	for i, d := range durations {
		seconds[i] = d.Seconds()
		sum += seconds[i]
	}

	mean := sum / float64(len(seconds))
	sumSquares := 0.0

	for _, s := range seconds {
		sumSquares += (s - mean) * (s - mean)
	}

	variance := sumSquares / float64(len(seconds))
	return variance / (mean * mean) // Coefficient of variation
}

func (pde *PatternDiscoveryEngine) calculateOverallSuccessRate(data []*types.WorkflowExecutionData) float64 {
	if len(data) == 0 {
		return 0
	}

	successful := 0
	for _, execution := range data {
		if execution.Success {
			successful++
		}
	}

	return float64(successful) / float64(len(data))
}

func (pde *PatternDiscoveryEngine) calculateSuccessRateForGroup(executions []*types.WorkflowExecutionData) float64 {
	return pde.calculateOverallSuccessRate(executions)
}

// Data collection helper methods for PatternDiscoveryEngine

// validateAnalysisRequest validates the analysis request parameters
func (pde *PatternDiscoveryEngine) validateAnalysisRequest(request *PatternAnalysisRequest) error {
	if request == nil {
		return fmt.Errorf("analysis request cannot be nil")
	}

	// Validate time range
	if request.TimeRange.Start.IsZero() || request.TimeRange.End.IsZero() {
		return fmt.Errorf("time range must have valid start and end times")
	}

	if request.TimeRange.End.Before(request.TimeRange.Start) {
		return fmt.Errorf("time range end cannot be before start")
	}

	// Validate time range is not too large
	duration := request.TimeRange.End.Sub(request.TimeRange.Start)
	maxDuration := time.Duration(pde.config.MaxHistoryDays) * 24 * time.Hour
	if duration > maxDuration {
		return fmt.Errorf("time range too large: %v, maximum allowed: %v", duration, maxDuration)
	}

	// Validate pattern types
	if len(request.PatternTypes) == 0 {
		return fmt.Errorf("at least one pattern type must be specified")
	}

	// Validate confidence threshold
	if request.MinConfidence < 0 || request.MinConfidence > 1 {
		return fmt.Errorf("minimum confidence must be between 0 and 1, got: %f", request.MinConfidence)
	}

	return nil
}

// collectFromPatternStore collects data from the pattern store
func (pde *PatternDiscoveryEngine) collectFromPatternStore(ctx context.Context, request *PatternAnalysisRequest) ([]*types.WorkflowExecutionData, error) {
	// Create filters for pattern store query
	filters := make(map[string]interface{})

	// Add time range filters
	filters["start_time"] = request.TimeRange.Start
	filters["end_time"] = request.TimeRange.End

	// Add user-provided filters
	for k, v := range request.Filters {
		filters[k] = v
	}

	// Get patterns from store
	patterns, err := pde.patternStore.GetPatterns(ctx, filters)
	if err != nil {
		return nil, err
	}

	// Convert patterns to workflow execution data
	executions := make([]*types.WorkflowExecutionData, 0)
	for _, pattern := range patterns {
		for _, sourceExecution := range pattern.SourceExecutions {
			// Create synthetic execution data from pattern
			execution := &types.WorkflowExecutionData{
				ExecutionID: sourceExecution,
				WorkflowID:  fmt.Sprintf("template-from-pattern-%s", pattern.ID),
				Timestamp:   pattern.LastSeen,
				Duration:    pattern.AverageExecutionTime,
				Success:     pattern.SuccessRate > 0.5,
				Metrics:     make(map[string]float64),
				Metadata: map[string]interface{}{
					"alert": map[string]interface{}{
						"name":      pattern.Name,
						"severity":  "info",
						"namespace": "unknown",
						"resource":  "unknown",
					},
				},
			}
			executions = append(executions, execution)
		}
	}

	return executions, nil
}

// collectFromVectorDB collects data from the vector database
func (pde *PatternDiscoveryEngine) collectFromVectorDB(ctx context.Context, request *PatternAnalysisRequest) ([]*types.WorkflowExecutionData, error) {
	// Create a query vector based on request
	queryVector := pde.createQueryVector(request)

	// Search for similar vectors using unified interface
	resultSet, err := pde.vectorDB.Search(ctx, queryVector, 100) // Limit to 100 results
	if err != nil {
		return nil, err
	}

	executions := make([]*types.WorkflowExecutionData, 0)
	for _, result := range resultSet.Results {
		// Check if result is within time range
		if timestamp, ok := result.Metadata["timestamp"].(time.Time); ok {
			if timestamp.Before(request.TimeRange.Start) || timestamp.After(request.TimeRange.End) {
				continue
			}
		}

		// Convert vector result to execution data
		execution := pde.vectorResultToExecution(&result)
		if execution != nil {
			executions = append(executions, execution)
		}
	}

	return executions, nil
}

// collectFromHistoricalBuffer collects data from the anomaly detector's historical buffer
func (pde *PatternDiscoveryEngine) collectFromHistoricalBuffer(request *PatternAnalysisRequest) []*types.WorkflowExecutionData {
	if pde.anomalyDetector == nil {
		return []*types.WorkflowExecutionData{}
	}

	// Log the request context for historical data collection
	if request != nil {
		pde.log.WithFields(logrus.Fields{
			"time_range_start": request.TimeRange.Start,
			"time_range_end":   request.TimeRange.End,
			"filters":          request.Filters,
			"pattern_types":    request.PatternTypes,
			"max_results":      request.MaxResults,
		}).Debug("Collecting historical buffer data with request filters")

		// Apply basic filtering based on request criteria
		// In a real implementation, this would query the anomaly detector's historical buffer
		// with the provided filters
		if len(request.Filters) > 0 || !request.TimeRange.Start.IsZero() {
			pde.log.Debug("Request contains specific filters - would apply to historical buffer query")
		}
	}

	// Since engine.AnomalyDetector doesn't have historicalData field,
	// we return empty slice for now. In a real implementation, this would
	// need to be properly implemented based on the actual AnomalyDetector interface
	// and would use the request parameters to filter historical data
	return []*types.WorkflowExecutionData{}
}

// validateAndCleanData validates and cleans the collected data
func (pde *PatternDiscoveryEngine) validateAndCleanData(data []*types.WorkflowExecutionData) ([]*types.WorkflowExecutionData, []error) {
	cleaned := make([]*types.WorkflowExecutionData, 0)
	errors := make([]error, 0)

	for i, execution := range data {
		if execution == nil {
			errors = append(errors, fmt.Errorf("execution at index %d is nil", i))
			continue
		}

		// Validate required fields
		if execution.ExecutionID == "" {
			errors = append(errors, fmt.Errorf("execution at index %d has empty ExecutionID", i))
			continue
		}

		if execution.WorkflowID == "" {
			errors = append(errors, fmt.Errorf("execution %s has empty WorkflowID", execution.ExecutionID))
			continue
		}

		if execution.Timestamp.IsZero() {
			errors = append(errors, fmt.Errorf("execution %s has zero timestamp", execution.ExecutionID))
			continue
		}

		// Clean and normalize data
		cleanedExecution := pde.cleanExecutionData(execution)
		cleaned = append(cleaned, cleanedExecution)
	}

	return cleaned, errors
}

// cleanExecutionData cleans and normalizes execution data
func (pde *PatternDiscoveryEngine) cleanExecutionData(execution *types.WorkflowExecutionData) *types.WorkflowExecutionData {
	// Create cleaned version using the engine.WorkflowExecutionData structure
	cleaned := &types.WorkflowExecutionData{
		ExecutionID: strings.TrimSpace(execution.ExecutionID),
		WorkflowID:  strings.TrimSpace(execution.WorkflowID),
		Timestamp:   execution.Timestamp,
		Duration:    execution.Duration,
		Success:     execution.Success,
		Metrics:     make(map[string]float64),
		Metadata:    make(map[string]interface{}),
	}

	// Copy and clean metrics
	for k, v := range execution.Metrics {
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			cleaned.Metrics[k] = v
		}
	}

	// Clean and normalize alert data in metadata
	alert := pde.extractAlertFromMetadata(execution)
	if alert != nil {
		cleanedAlert := map[string]interface{}{
			"name":      strings.TrimSpace(alert.Name),
			"severity":  pde.normalizeSeverity(alert.Severity),
			"namespace": strings.TrimSpace(alert.Namespace),
			"resource":  strings.TrimSpace(alert.Resource),
		}
		if alert.Labels != nil {
			cleanedAlert["labels"] = alert.Labels
		}
		cleaned.Metadata["alert"] = cleanedAlert
	}

	// Copy other metadata
	for k, v := range execution.Metadata {
		if k != "alert" { // Don't overwrite cleaned alert
			cleaned.Metadata[k] = v
		}
	}

	// Validate duration
	if cleaned.Duration < 0 {
		cleaned.Duration = 0
	}
	// Cap extremely long durations (likely data errors)
	if cleaned.Duration > 24*time.Hour {
		cleaned.Duration = 24 * time.Hour
	}

	return cleaned
}

// normalizeSeverity normalizes severity strings
func (pde *PatternDiscoveryEngine) normalizeSeverity(severity string) string {
	normalized := strings.ToLower(strings.TrimSpace(severity))
	switch normalized {
	case "crit", "critical", "high":
		return "critical"
	case "warn", "warning", "medium":
		return "warning"
	case "info", "information", "low":
		return "info"
	default:
		return "info" // Default to info
	}
}

// normalizeResourceValue normalizes resource usage values to 0-1 range
// Removed unused normalizeResourceValue function per development guidelines

// applyRequestFilters applies filters from the request to the data
func (pde *PatternDiscoveryEngine) applyRequestFilters(data []*types.WorkflowExecutionData, request *PatternAnalysisRequest) []*types.WorkflowExecutionData {
	if len(request.Filters) == 0 {
		return data
	}

	filtered := make([]*types.WorkflowExecutionData, 0)

	for _, execution := range data {
		if pde.matchesFilters(execution, request.Filters) {
			filtered = append(filtered, execution)
		}
	}

	return filtered
}

// matchesFilters checks if execution matches the provided filters
func (pde *PatternDiscoveryEngine) matchesFilters(execution *types.WorkflowExecutionData, filters map[string]interface{}) bool {
	for key, value := range filters {
		switch key {
		case "template_id":
			if stringValue, ok := value.(string); ok && execution.WorkflowID != stringValue {
				return false
			}
		case "alert_name":
			alertData, hasAlert := execution.Metadata["alert"]
			if !hasAlert {
				return false
			}
			if alertMap, ok := alertData.(map[string]interface{}); ok {
				if name, ok := alertMap["name"].(string); ok {
					if stringValue, ok := value.(string); ok && name != stringValue {
						return false
					}
				}
			}
		case "severity":
			alertData, hasAlert := execution.Metadata["alert"]
			if !hasAlert {
				return false
			}
			if alertMap, ok := alertData.(map[string]interface{}); ok {
				if severity, ok := alertMap["severity"].(string); ok {
					if stringValue, ok := value.(string); ok && severity != stringValue {
						return false
					}
				}
			}
		case "namespace":
			alertData, hasAlert := execution.Metadata["alert"]
			if !hasAlert {
				return false
			}
			if alertMap, ok := alertData.(map[string]interface{}); ok {
				if namespace, ok := alertMap["namespace"].(string); ok {
					if stringValue, ok := value.(string); ok && namespace != stringValue {
						return false
					}
				}
			}
		case "success":
			if boolValue, ok := value.(bool); ok && execution.Success != boolValue {
				return false
			}
		case "min_duration":
			if durationValue, ok := value.(time.Duration); ok && execution.Duration < durationValue {
				return false
			}
		case "max_duration":
			if durationValue, ok := value.(time.Duration); ok && execution.Duration > durationValue {
				return false
			}
		}
	}

	return true
}

// createQueryVector creates a query vector from the analysis request
func (pde *PatternDiscoveryEngine) createQueryVector(request *PatternAnalysisRequest) []float64 {
	vector := make([]float64, 10) // Standard vector size

	// Encode time range as vector features
	duration := request.TimeRange.End.Sub(request.TimeRange.Start)
	vector[0] = math.Min(duration.Hours()/24.0, 1.0) // Normalize to days

	// Encode hour of day (using start time)
	vector[1] = float64(request.TimeRange.Start.Hour()) / 24.0

	// Encode day of week
	vector[2] = float64(request.TimeRange.Start.Weekday()) / 7.0

	// Encode pattern types as bitmap
	patternBits := 0.0
	for _, patternType := range request.PatternTypes {
		switch patternType {
		case shared.PatternTypeAlert:
			patternBits += 0.1
		case shared.PatternTypeResource:
			patternBits += 0.2
		case shared.PatternTypeTemporal:
			patternBits += 0.3
		case shared.PatternTypeFailure:
			patternBits += 0.4
		case shared.PatternTypeOptimization:
			patternBits += 0.5
		case shared.PatternTypeAnomaly:
			patternBits += 0.6
		case shared.PatternTypeWorkflow:
			patternBits += 0.7
		}
	}
	vector[3] = math.Min(patternBits, 1.0)

	// Encode confidence threshold
	vector[4] = request.MinConfidence

	// Fill remaining slots with filter-based features
	if nameFilter, exists := request.Filters["alert_name"]; exists {
		if name, ok := nameFilter.(string); ok {
			vector[5] = pde.hashStringToFloat(name)
		}
	}

	if severityFilter, exists := request.Filters["severity"]; exists {
		if severity, ok := severityFilter.(string); ok {
			switch severity {
			case "critical":
				vector[6] = 1.0
			case "warning":
				vector[6] = 0.75
			case "info":
				vector[6] = 0.5
			default:
				vector[6] = 0.25
			}
		}
	}

	return vector
}

// vectorResultToExecution converts a unified vector search result to execution data
func (pde *PatternDiscoveryEngine) vectorResultToExecution(result *UnifiedSearchResult) *types.WorkflowExecutionData {
	execution := &types.WorkflowExecutionData{
		ExecutionID: result.ID,
		Metrics:     make(map[string]float64),
		Metadata:    make(map[string]interface{}),
	}

	// Extract metadata
	if templateID, ok := result.Metadata["template_id"].(string); ok {
		execution.WorkflowID = templateID
	}

	if timestamp, ok := result.Metadata["timestamp"].(time.Time); ok {
		execution.Timestamp = timestamp
	}

	// Set success and duration - convert float32 Score to float64 for comparison
	execution.Success = float64(result.Score) > 0.7 // Use similarity score as success indicator
	execution.Duration = time.Minute * 5            // Default duration

	// Store alert information in metadata
	if alertName, ok := result.Metadata["alert_name"].(string); ok {
		alertData := map[string]interface{}{
			"name":      alertName,
			"severity":  "info",
			"namespace": "unknown",
			"resource":  "unknown",
		}

		if severity, ok := result.Metadata["severity"].(string); ok {
			alertData["severity"] = severity
		}
		if namespace, ok := result.Metadata["namespace"].(string); ok {
			alertData["namespace"] = namespace
		}
		if resource, ok := result.Metadata["resource"].(string); ok {
			alertData["resource"] = resource
		}

		execution.Metadata["alert"] = alertData
	}

	return execution
}

// hashStringToFloat creates a normalized hash value from a string
func (pde *PatternDiscoveryEngine) hashStringToFloat(s string) float64 {
	hash := uint32(0)
	for _, c := range s {
		hash = hash*31 + uint32(c)
	}
	return float64(hash%1000) / 1000.0 // Normalize to 0-1
}

// collectFromExecutionRepository collects data from execution repository if available
func (pde *PatternDiscoveryEngine) collectFromExecutionRepository(ctx context.Context, request *PatternAnalysisRequest) ([]*types.WorkflowExecutionData, error) {
	// Check if execution repository is available (would be injected)
	if pde.executionRepo == nil {
		return []*types.WorkflowExecutionData{}, nil
	}

	// Query executions within time range
	executions, err := pde.executionRepo.GetExecutionsInTimeWindow(ctx, request.TimeRange.Start, request.TimeRange.End)
	if err != nil {
		return nil, fmt.Errorf("failed to query execution repository: %w", err)
	}

	// Convert execution repository format to WorkflowExecutionData
	executionData := make([]*types.WorkflowExecutionData, 0)
	for _, exec := range executions {
		data := pde.convertExecutionToWorkflowData(exec)
		if data != nil {
			executionData = append(executionData, data)
		}
	}

	pde.log.WithField("count", len(executionData)).Debug("Collected executions from repository")
	return executionData, nil
}

// deduplicateExecutionData removes duplicate execution data based on execution ID
func (pde *PatternDiscoveryEngine) deduplicateExecutionData(data []*types.WorkflowExecutionData) []*types.WorkflowExecutionData {
	seen := make(map[string]bool)
	unique := make([]*types.WorkflowExecutionData, 0)

	for _, execution := range data {
		if execution == nil || execution.ExecutionID == "" {
			continue
		}

		if !seen[execution.ExecutionID] {
			seen[execution.ExecutionID] = true
			unique = append(unique, execution)
		}
	}

	return unique
}

// convertExecutionToWorkflowData converts execution repository format to WorkflowExecutionData
func (pde *PatternDiscoveryEngine) convertExecutionToWorkflowData(exec *types.RuntimeWorkflowExecution) *types.WorkflowExecutionData {
	if exec == nil {
		return nil
	}

	data := &types.WorkflowExecutionData{
		ExecutionID: exec.ID,
		WorkflowID:  exec.WorkflowID,
		Timestamp:   exec.StartTime,
		Duration:    exec.Duration,
		Metrics:     make(map[string]float64),
		Metadata:    make(map[string]interface{}),
	}

	// Set success based on execution status
	data.Success = (exec.OperationalStatus == types.ExecutionStatusCompleted)

	// Extract alert information from variables if available and store in metadata
	if exec.Variables != nil {
		if alertData, exists := exec.Variables["alert"]; exists {
			data.Metadata["alert"] = alertData
		}

		// Extract resource usage as metrics if available
		if resourceData, exists := exec.Variables["resource_usage"]; exists {
			if resourceMap, ok := resourceData.(map[string]interface{}); ok {
				if cpu, ok := resourceMap["cpu"].(float64); ok {
					data.Metrics["cpu_usage"] = cpu
				}
				if memory, ok := resourceMap["memory"].(float64); ok {
					data.Metrics["memory_usage"] = memory
				}
				if network, ok := resourceMap["network"].(float64); ok {
					data.Metrics["network_usage"] = network
				}
				if storage, ok := resourceMap["storage"].(float64); ok {
					data.Metrics["storage_usage"] = storage
				}
			}
		}
	}

	// Store execution context in metadata if available
	if exec.Input != nil {
		data.Metadata["execution_context"] = map[string]interface{}{
			"environment": exec.Input.Environment,
			"context":     exec.Input.Context,
		}
	}

	return data
}

// Enhanced empirical validation methods for pattern analysis

// validateAlertPatternEmpirical performs empirical validation using statistical tests
// Removed unused validateAlertPatternEmpirical function per development guidelines

// Removed unused bootstrapSample function per development guidelines - no callers after validateAlertPatternEmpirical removal

// calculateBootstrapScore calculates a score for bootstrap sample
// Removed unused calculateBootstrapScore function per development guidelines

// calculateChiSquareTest performs chi-square test for independence
// Removed unused calculateChiSquareTest function per development guidelines

// calculateTemporalStability measures how stable patterns are across time

// Removed unused calculateEnhancedPatternConfidence function per development guidelines

// Statistical validation methods for resource patterns

// Confidence calibration and validation methods

// Removed unused ConfidenceCalibrator struct per development guidelines

// Supporting types for confidence validation

type ConfidenceReliabilityScore struct {
	OverallReliability float64            `json:"overall_reliability"`
	DataQualityScore   float64            `json:"data_quality_score"`
	SampleSizeScore    float64            `json:"sample_size_score"`
	TemporalStability  float64            `json:"temporal_stability"`
	ValidationScore    float64            `json:"validation_score"`
	Factors            map[string]float64 `json:"factors"`
}

// Enhanced alert pattern analysis helper methods

// calculateClusterConfidence calculates confidence for alert clusters
// Removed unused calculateClusterConfidence function per development guidelines

// generateClusterName generates a descriptive name for alert clusters
// Removed unused generateClusterName function per development guidelines

// generateClusterDescription generates detailed description for clusters
// Removed unused generateClusterDescription function per development guidelines

// generateClusterOptimizationHints generates optimization hints for clusters
// Removed unused generateClusterOptimizationHints function per development guidelines

// calculateClusterMetrics calculates metrics for alert clusters
// Removed unused calculateClusterMetrics function per development guidelines

// groupAlertsByTimeWindows groups alerts into time windows
func (pde *PatternDiscoveryEngine) groupAlertsByTimeWindows(data []*types.WorkflowExecutionData, windowSize time.Duration) map[time.Time][]*types.WorkflowExecutionData {
	windows := make(map[time.Time][]*types.WorkflowExecutionData)

	for _, execution := range data {
		// Round timestamp to window boundary
		windowStart := execution.Timestamp.Truncate(windowSize)

		if _, exists := windows[windowStart]; !exists {
			windows[windowStart] = make([]*types.WorkflowExecutionData, 0)
		}
		windows[windowStart] = append(windows[windowStart], execution)
	}

	return windows
}

// analyzeAlertSequence analyzes a sequence of alerts for patterns
func (pde *PatternDiscoveryEngine) analyzeAlertSequence(alerts []*types.WorkflowExecutionData) *AlertSequenceAnalysis {
	if len(alerts) < 2 {
		return &AlertSequenceAnalysis{IsSignificant: false}
	}

	// Sort by timestamp
	sortedAlerts := make([]*types.WorkflowExecutionData, len(alerts))
	copy(sortedAlerts, alerts)
	sort.Slice(sortedAlerts, func(i, j int) bool {
		return sortedAlerts[i].Timestamp.Before(sortedAlerts[j].Timestamp)
	})

	// Calculate intervals
	intervals := make([]time.Duration, 0)
	for i := 1; i < len(sortedAlerts); i++ {
		interval := sortedAlerts[i].Timestamp.Sub(sortedAlerts[i-1].Timestamp)
		intervals = append(intervals, interval)
	}

	// Calculate average interval
	totalInterval := time.Duration(0)
	for _, interval := range intervals {
		totalInterval += interval
	}
	avgInterval := totalInterval / time.Duration(len(intervals))

	// Collect alert types
	alertTypes := make([]string, 0)
	for _, alertExecution := range sortedAlerts {
		alert := pde.extractAlertFromMetadata(alertExecution)
		if alert != nil {
			alertTypes = append(alertTypes, alert.Name)
		}
	}

	// Calculate success rate
	successful := 0
	for _, alert := range sortedAlerts {
		if alert.Success {
			successful++
		}
	}
	successRate := float64(successful) / float64(len(sortedAlerts))

	// Analyze severity escalation
	severityEscalation := pde.calculateSeverityEscalation(sortedAlerts)

	// Determine significance
	isSignificant := len(alerts) >= 3 && avgInterval < 10*time.Minute && successRate < 0.8

	confidence := 0.5
	if isSignificant {
		confidence = 0.7 + (1.0-successRate)*0.2 // Higher confidence for problematic sequences
	}

	return &AlertSequenceAnalysis{
		IsSignificant:      isSignificant,
		Confidence:         confidence,
		SuccessRate:        successRate,
		AlertTypes:         alertTypes,
		AverageInterval:    avgInterval,
		AverageDuration:    totalInterval,
		SeverityEscalation: severityEscalation,
	}
}

// calculateSeverityEscalation calculates severity escalation in alert sequence
func (pde *PatternDiscoveryEngine) calculateSeverityEscalation(alerts []*types.WorkflowExecutionData) float64 {
	if len(alerts) < 2 {
		return 0.0
	}

	severityValues := make([]float64, 0)
	for _, alertExecution := range alerts {
		alert := pde.extractAlertFromMetadata(alertExecution)
		if alert != nil {
			switch strings.ToLower(alert.Severity) {
			case "critical":
				severityValues = append(severityValues, 1.0)
			case "warning":
				severityValues = append(severityValues, 0.75)
			case "info":
				severityValues = append(severityValues, 0.5)
			default:
				severityValues = append(severityValues, 0.25)
			}
		}
	}

	if len(severityValues) < 2 {
		return 0.0
	}

	// Calculate trend (positive = escalating)
	escalation := severityValues[len(severityValues)-1] - severityValues[0]
	return escalation
}

// calculateResourceAffinity calculates resource affinity analysis
func (pde *PatternDiscoveryEngine) calculateResourceAffinity(executions []*types.WorkflowExecutionData) *ResourceAffinityAnalysis {
	if len(executions) == 0 {
		return &ResourceAffinityAnalysis{IsSignificant: false}
	}

	// Extract common properties
	firstExecution := executions[0]
	var namespace, resource, resourceType string
	alert := pde.extractAlertFromMetadata(firstExecution)
	if alert != nil {
		namespace = alert.Namespace
		resource = alert.Resource
		resourceType = resource // Simplified mapping
	}

	// Collect alert types
	alertTypeMap := make(map[string]int)
	successful := 0
	totalUtilization := 0.0
	utilizationCount := 0

	for _, execution := range executions {
		alert := pde.extractAlertFromMetadata(execution)
		if alert != nil {
			alertTypeMap[alert.Name]++
		}

		if execution.Success {
			successful++
		}

		// Extract resource usage from metrics if available
		if cpuUsage, hasCPU := execution.Metrics["cpu_usage"]; hasCPU {
			memUsage := execution.Metrics["memory_usage"]
			totalUtilization += (cpuUsage + memUsage) / 2.0
			utilizationCount++
		}
	}

	// Convert alert type map to slice
	alertTypes := make([]string, 0)
	for alertType := range alertTypeMap {
		alertTypes = append(alertTypes, alertType)
	}

	successRate := float64(successful) / float64(len(executions))
	avgUtilization := 0.0
	if utilizationCount > 0 {
		avgUtilization = totalUtilization / float64(utilizationCount)
	}

	// Determine significance
	isSignificant := len(executions) >= 5 && (successRate < 0.8 || avgUtilization > 0.8)

	confidence := 0.5
	if isSignificant {
		confidence = 0.6 + float64(len(executions))/50.0 // Higher confidence with more data
		confidence = math.Min(confidence, 0.9)
	}

	return &ResourceAffinityAnalysis{
		IsSignificant:      isSignificant,
		Confidence:         confidence,
		SuccessRate:        successRate,
		Namespace:          namespace,
		Resource:           resource,
		ResourceType:       resourceType,
		AlertTypes:         alertTypes,
		AverageUtilization: avgUtilization,
		UtilizationTrend: &types.UtilizationTrend{
			ResourceType:       resourceType,
			TrendDirection:     "variable",
			GrowthRate:         0.1, // Default values
			SeasonalVariation:  0.2,
			PeakUtilization:    avgUtilization * 1.5,
			AverageUtilization: avgUtilization,
			EfficiencyScore:    0.7,
		}, // Simplified
	}
}

// findSeverityEscalations finds severity escalation patterns
func (pde *PatternDiscoveryEngine) findSeverityEscalations(data []*types.WorkflowExecutionData) []*SeverityEscalationAnalysis {
	escalations := make([]*SeverityEscalationAnalysis, 0)

	if len(data) < 3 {
		return escalations
	}

	// Look for escalation sequences in time windows
	windowSize := 2 * time.Hour
	windows := pde.groupAlertsByTimeWindows(data, windowSize)

	for windowStart, windowAlerts := range windows {
		if len(windowAlerts) >= 3 {
			escalation := pde.analyzeEscalationWindow(windowAlerts, windowStart)
			if escalation != nil && escalation.IsSignificant {
				escalations = append(escalations, escalation)
			}
		}
	}

	return escalations
}

// analyzeEscalationWindow analyzes a time window for escalation patterns
func (pde *PatternDiscoveryEngine) analyzeEscalationWindow(alerts []*types.WorkflowExecutionData, windowStart time.Time) *SeverityEscalationAnalysis {
	if len(alerts) < 3 {
		return nil
	}

	// Use windowStart for context logging and validation
	pde.log.WithFields(logrus.Fields{
		"window_start": windowStart,
		"alert_count":  len(alerts),
	}).Debug("Analyzing escalation window")

	// Sort by timestamp
	sorted := make([]*types.WorkflowExecutionData, len(alerts))
	copy(sorted, alerts)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	// Validate that alerts fall within the expected window
	windowEnd := windowStart.Add(2 * time.Hour) // Assuming 2-hour windows from caller
	for _, alert := range sorted {
		if alert.Timestamp.Before(windowStart) || alert.Timestamp.After(windowEnd) {
			pde.log.WithFields(logrus.Fields{
				"alert_timestamp": alert.Timestamp,
				"window_start":    windowStart,
				"window_end":      windowEnd,
			}).Debug("Alert timestamp outside expected window")
		}
	}

	// Find escalation sequence
	firstAlertData := pde.extractAlertFromMetadata(sorted[0])
	lastAlertData := pde.extractAlertFromMetadata(sorted[len(sorted)-1])
	if firstAlertData == nil || lastAlertData == nil {
		return nil
	}

	startSeverity := strings.ToLower(firstAlertData.Severity)
	endSeverity := strings.ToLower(lastAlertData.Severity)

	// Check if this is actually an escalation
	startValue := pde.severityToValue(startSeverity)
	endValue := pde.severityToValue(endSeverity)

	if endValue <= startValue {
		return nil // Not an escalation
	}

	duration := sorted[len(sorted)-1].Timestamp.Sub(sorted[0].Timestamp)

	// Collect affected resources
	affectedResources := make(map[string]bool)
	resolutionCount := 0
	for _, alertExecution := range sorted {
		alert := pde.extractAlertFromMetadata(alertExecution)
		if alert != nil {
			resourceKey := fmt.Sprintf("%s/%s", alert.Namespace, alert.Resource)
			affectedResources[resourceKey] = true
		}
		if alertExecution.Success {
			resolutionCount++
		}
	}

	affectedResourceList := make([]string, 0)
	for resource := range affectedResources {
		affectedResourceList = append(affectedResourceList, resource)
	}

	resolutionRate := float64(resolutionCount) / float64(len(sorted))
	escalationSpeed := (endValue - startValue) / duration.Hours()

	// Determine significance
	isSignificant := endValue-startValue >= 0.25 && duration < 4*time.Hour && len(affectedResources) > 1

	confidence := 0.6
	if isSignificant {
		confidence = 0.7 + (endValue-startValue)*0.2
		confidence = math.Min(confidence, 0.95)
	}

	return &SeverityEscalationAnalysis{
		IsSignificant:     isSignificant,
		Confidence:        confidence,
		StartTime:         sorted[0].Timestamp,
		StartSeverity:     startSeverity,
		EndSeverity:       endSeverity,
		Duration:          duration,
		Pattern:           fmt.Sprintf("%s→%s", startSeverity, endSeverity),
		ResolutionRate:    resolutionRate,
		EscalationSpeed:   escalationSpeed,
		ResolutionTime:    duration,
		AffectedResources: affectedResourceList,
	}
}

// severityToValue converts severity string to numeric value
func (pde *PatternDiscoveryEngine) severityToValue(severity string) float64 {
	switch strings.ToLower(severity) {
	case "critical":
		return 1.0
	case "warning":
		return 0.75
	case "info":
		return 0.5
	default:
		return 0.25
	}
}

// crossValidatePattern performs cross-validation on a discovered pattern
func (pde *PatternDiscoveryEngine) crossValidatePattern(pattern *shared.DiscoveredPattern, data []*types.WorkflowExecutionData) float64 {
	if len(data) < 5 {
		return 0.5 // Default confidence for insufficient data
	}

	// Split data into training (80%) and validation (20%)
	splitIndex := int(0.8 * float64(len(data)))
	_ = data[:splitIndex] // trainingData - not used in this simplified implementation
	validationData := data[splitIndex:]

	// Count matches in validation data
	matches := 0
	for _, execution := range validationData {
		if pde.executionMatchesPattern(execution, pattern) {
			matches++
		}
	}

	// Calculate validation confidence
	matchRate := float64(matches) / float64(len(validationData))

	// Adjust confidence based on pattern type complexity
	complexityFactor := 1.0
	switch strings.ToLower(string(pattern.Type)) {
	case "alert":
		complexityFactor = 0.9 // Slightly reduce confidence for alert patterns
	case "temporal":
		complexityFactor = 0.8 // More reduction for temporal patterns
	case "failure":
		complexityFactor = 0.7 // Highest reduction for failure patterns
	}

	confidence := matchRate * complexityFactor
	return math.Max(0.1, math.Min(0.95, confidence))
}

// executionMatchesPattern checks if an execution matches a discovered pattern
func (pde *PatternDiscoveryEngine) executionMatchesPattern(execution *types.WorkflowExecutionData, pattern *shared.DiscoveredPattern) bool {
	switch strings.ToLower(string(pattern.Type)) {
	case "alert":
		return pde.executionMatchesAlertPattern(execution, pattern)
	case "temporal":
		return pde.executionMatchesTemporalPattern(execution, pattern)
	case "resource":
		return pde.executionMatchesResourcePattern(execution, pattern)
	default:
		return false
	}
}

// executionMatchesAlertPattern checks if execution matches alert pattern
func (pde *PatternDiscoveryEngine) executionMatchesAlertPattern(execution *types.WorkflowExecutionData, pattern *shared.DiscoveredPattern) bool {
	alert := pde.extractAlertFromMetadata(execution)
	if alert == nil {
		return false
	}

	// Simplified pattern matching - check if pattern ID contains alert name
	// Since we don't have AlertPatterns field, use pattern ID/Type for matching
	patternType := strings.ToLower(string(pattern.Type))
	alertName := strings.ToLower(alert.Name)

	// Basic matching based on pattern type and alert name
	return strings.Contains(patternType, "alert") &&
		(strings.Contains(pattern.ID, alertName) || strings.Contains(alertName, patternType))
}

// executionMatchesTemporalPattern checks if execution matches temporal pattern
func (pde *PatternDiscoveryEngine) executionMatchesTemporalPattern(execution *types.WorkflowExecutionData, pattern *shared.DiscoveredPattern) bool {
	if len(pattern.TemporalPatterns) == 0 {
		return false
	}

	temporalPattern := pattern.TemporalPatterns[0]

	// Check if execution time falls within pattern peak times
	for _, peakTime := range temporalPattern.PeakTimes {
		if execution.Timestamp.After(peakTime.Start) && execution.Timestamp.Before(peakTime.End) {
			return true
		}
	}

	return false
}

// executionMatchesResourcePattern checks if execution matches resource pattern
func (pde *PatternDiscoveryEngine) executionMatchesResourcePattern(execution *types.WorkflowExecutionData, pattern *shared.DiscoveredPattern) bool {
	// Check if execution has resource metrics
	hasResourceMetrics := false
	for key := range execution.Metrics {
		if strings.Contains(key, "cpu") || strings.Contains(key, "memory") ||
			strings.Contains(key, "network") || strings.Contains(key, "storage") {
			hasResourceMetrics = true
			break
		}
	}

	if !hasResourceMetrics {
		return false
	}

	// Simple check based on pattern type containing resource-related keywords
	patternType := strings.ToLower(string(pattern.Type))
	return strings.Contains(patternType, "resource") || strings.Contains(patternType, "cpu") ||
		strings.Contains(patternType, "memory") || strings.Contains(patternType, "storage")
}

// Supporting types for enhanced pattern analysis

type AlertSequenceAnalysis struct {
	IsSignificant      bool
	Confidence         float64
	SuccessRate        float64
	AlertTypes         []string
	AverageInterval    time.Duration
	AverageDuration    time.Duration
	SeverityEscalation float64
}

type ResourceAffinityAnalysis struct {
	IsSignificant      bool
	Confidence         float64
	SuccessRate        float64
	Namespace          string
	Resource           string
	ResourceType       string
	AlertTypes         []string
	AverageUtilization float64
	UtilizationTrend   *types.UtilizationTrend
}

type SeverityEscalationAnalysis struct {
	IsSignificant     bool
	Confidence        float64
	StartTime         time.Time
	StartSeverity     string
	EndSeverity       string
	Duration          time.Duration
	Pattern           string
	ResolutionRate    float64
	EscalationSpeed   float64
	ResolutionTime    time.Duration
	AffectedResources []string
}

// Confidence validation and accuracy tracking methods

// PatternAccuracyTracker tracks the accuracy of discovered patterns over time
type PatternAccuracyTracker struct {
	PatternID          string                     `json:"pattern_id"`
	CreatedAt          time.Time                  `json:"created_at"`
	TotalPredictions   int                        `json:"total_predictions"`
	CorrectPredictions int                        `json:"correct_predictions"`
	FalsePositives     int                        `json:"false_positives"`
	FalseNegatives     int                        `json:"false_negatives"`
	ConfidenceHistory  []ConfidenceDataPoint      `json:"confidence_history"`
	AccuracyMetrics    *PatternAccuracyMetrics    `json:"accuracy_metrics"`
	ValidationResults  []*PatternValidationResult `json:"validation_results"`
	LastValidated      time.Time                  `json:"last_validated"`
	PerformanceTrend   string                     `json:"performance_trend"` // "improving", "declining", "stable"
}

// ConfidenceDataPoint represents a point in confidence tracking
type ConfidenceDataPoint struct {
	Timestamp           time.Time              `json:"timestamp"`
	PredictedConfidence float64                `json:"predicted_confidence"`
	ActualOutcome       bool                   `json:"actual_outcome"`
	ContextFactors      map[string]interface{} `json:"context_factors"`
}

// PatternAccuracyMetrics contains comprehensive accuracy metrics
type PatternAccuracyMetrics struct {
	Precision             float64   `json:"precision"`              // TP / (TP + FP)
	Recall                float64   `json:"recall"`                 // TP / (TP + FN)
	F1Score               float64   `json:"f1_score"`               // 2 * (Precision * Recall) / (Precision + Recall)
	Accuracy              float64   `json:"accuracy"`               // (TP + TN) / (TP + TN + FP + FN)
	ConfidenceCorrelation float64   `json:"confidence_correlation"` // How well confidence predicts outcomes
	CalibrationError      float64   `json:"calibration_error"`      // Average difference between confidence and accuracy
	AverageConfidence     float64   `json:"average_confidence"`
	ConfidenceStdDev      float64   `json:"confidence_std_dev"`
	SampleSize            int       `json:"sample_size"`
	LastUpdated           time.Time `json:"last_updated"`
}
