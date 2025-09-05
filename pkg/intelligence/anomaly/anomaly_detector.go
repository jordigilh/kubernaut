package anomaly

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/jordigilh/kubernaut/pkg/intelligence/patterns"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

// AnomalyDetectionResult contains the results of batch anomaly detection
type AnomalyDetectionResult struct {
	Anomalies       []*AnomalyResult         `json:"anomalies"`
	Summary         *DetectionSummary        `json:"summary"`
	BaselineHealth  *BaselineHealthReport    `json:"baseline_health"`
	TrendAnalysis   *TimeSeriesTrendAnalysis `json:"trend_analysis"`
	Recommendations []*SystemRecommendation  `json:"recommendations"`
	AnalyzedPeriod  TimeRange                `json:"analyzed_period"`
}

// AnomalyDetector detects anomalous patterns in workflow execution data
type AnomalyDetector struct {
	config           *patterns.PatternDiscoveryConfig
	log              *logrus.Logger
	baselineModels   map[string]*BaselineModel
	detectionMethods []DetectionMethod
	alertThresholds  map[string]float64
	historicalData   *HistoricalDataBuffer
}

// BaselineModel represents normal behavior patterns
type BaselineModel struct {
	ID               string                       `json:"id"`
	Type             string                       `json:"type"` // "statistical", "temporal", "behavioral"
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
	DataPoints       int                          `json:"data_points"`
	Statistics       *BaselineStatistics          `json:"statistics"`
	TemporalPatterns map[string]*TemporalBaseline `json:"temporal_patterns"`
	Parameters       map[string]interface{}       `json:"parameters"`
	Confidence       float64                      `json:"confidence"`
}

// TemporalBaseline contains time-based baseline patterns
type TemporalBaseline struct {
	Pattern     string             `json:"pattern"`     // "hourly", "daily", "weekly"
	Expected    map[string]float64 `json:"expected"`    // Expected values for time periods
	Variance    map[string]float64 `json:"variance"`    // Variance for each time period
	Seasonality float64            `json:"seasonality"` // Seasonal strength
	Trend       float64            `json:"trend"`       // Trend coefficient
}

// DetectionMethod defines an anomaly detection algorithm
type DetectionMethod struct {
	Name        string                 `json:"name"`
	Algorithm   string                 `json:"algorithm"` // "statistical", "isolation_forest", "one_class_svm"
	Parameters  map[string]interface{} `json:"parameters"`
	Sensitivity float64                `json:"sensitivity"` // 0-1, higher = more sensitive
	Enabled     bool                   `json:"enabled"`
}

// WorkflowExecutionEvent represents a real-time workflow event
type WorkflowExecutionEvent struct {
	Type        string                 `json:"type"`
	WorkflowID  string                 `json:"workflow_id"`
	ExecutionID string                 `json:"execution_id"`
	Timestamp   time.Time              `json:"timestamp"`
	Data        map[string]interface{} `json:"data"`
	Metrics     map[string]float64     `json:"metrics"`
	Context     map[string]interface{} `json:"context"`
}

// AnomalyResult contains detection results
type AnomalyResult struct {
	ID              string                   `json:"id"`
	Type            string                   `json:"type"`     // "execution", "pattern", "temporal", "resource"
	Severity        string                   `json:"severity"` // "low", "medium", "high", "critical"
	DetectedAt      time.Time                `json:"detected_at"`
	Event           *WorkflowExecutionEvent  `json:"event"`
	BaselineModel   string                   `json:"baseline_model"`
	DetectionMethod string                   `json:"detection_method"`
	AnomalyScore    float64                  `json:"anomaly_score"` // 0-1, higher = more anomalous
	Confidence      float64                  `json:"confidence"`
	Description     string                   `json:"description"`
	Impact          *AnomalyImpactAssessment `json:"impact"`
	Recommendations []*AnomalyRecommendation `json:"recommendations"`
	Metadata        map[string]interface{}   `json:"metadata"`
}

// AnomalyImpactAssessment evaluates the impact of an anomaly
type AnomalyImpactAssessment struct {
	Scope             string        `json:"scope"` // "workflow", "namespace", "cluster"
	AffectedResources []string      `json:"affected_resources"`
	BusinessImpact    string        `json:"business_impact"` // "none", "low", "medium", "high", "critical"
	TechnicalImpact   string        `json:"technical_impact"`
	EstimatedCost     float64       `json:"estimated_cost"`
	RecoveryTime      time.Duration `json:"recovery_time"`
}

// AnomalyRecommendation suggests actions for handling anomalies
type AnomalyRecommendation struct {
	Action        string  `json:"action"`
	Description   string  `json:"description"`
	Priority      int     `json:"priority"`
	Urgency       string  `json:"urgency"` // "immediate", "urgent", "normal", "low"
	AutomateIt    bool    `json:"automate_it"`
	EstimatedCost float64 `json:"estimated_cost"`
}

// HistoricalDataBuffer maintains recent execution data for baseline updates
type HistoricalDataBuffer struct {
	maxSize int
	data    []*engine.WorkflowExecutionData
	index   int
	full    bool
}

// DetectionSummary provides overview of detection results
type DetectionSummary struct {
	TotalAnomalies    int            `json:"total_anomalies"`
	SeverityBreakdown map[string]int `json:"severity_breakdown"`
	TypeBreakdown     map[string]int `json:"type_breakdown"`
	DetectionRate     float64        `json:"detection_rate"`
	FalsePositiveRate float64        `json:"false_positive_rate"`
	Coverage          float64        `json:"coverage"`
}

// BaselineHealthReport assesses the health of baseline models
type BaselineHealthReport struct {
	ModelsEvaluated    int                `json:"models_evaluated"`
	HealthyModels      int                `json:"healthy_models"`
	StaleModels        int                `json:"stale_models"`
	InaccurateModels   int                `json:"inaccurate_models"`
	ModelHealth        map[string]float64 `json:"model_health"` // Model ID -> health score
	RecommendedActions []string           `json:"recommended_actions"`
}

// SystemRecommendation suggests system-level improvements
type SystemRecommendation struct {
	Type           string `json:"type"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	Impact         string `json:"impact"`
	Effort         string `json:"effort"`
	Priority       int    `json:"priority"`
	Implementation string `json:"implementation"`
}

// TimeSeriesTrendAnalysis contains trend analysis results
type TimeSeriesTrendAnalysis struct {
	Direction    string    `json:"direction"` // "increasing", "decreasing", "stable"
	Slope        float64   `json:"slope"`
	Confidence   float64   `json:"confidence"`
	StartValue   float64   `json:"start_value"`
	EndValue     float64   `json:"end_value"`
	TrendPeriod  TimeRange `json:"trend_period"`
	Significance float64   `json:"significance"`
}

// TimeRange represents a time period
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// BaselineStatistics contains statistical baseline information
type BaselineStatistics struct {
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"std_dev"`
	Median float64 `json:"median"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Q1     float64 `json:"q1"`
	Q3     float64 `json:"q3"`
	IQR    float64 `json:"iqr"`
}

// NewAnomalyDetector creates a new anomaly detector
func NewAnomalyDetector(config *patterns.PatternDiscoveryConfig, log *logrus.Logger) *AnomalyDetector {
	ad := &AnomalyDetector{
		config:          config,
		log:             log,
		baselineModels:  make(map[string]*BaselineModel),
		alertThresholds: make(map[string]float64),
		historicalData:  NewHistoricalDataBuffer(1000), // Keep last 1000 executions
	}

	ad.initializeDetectionMethods()
	ad.initializeAlertThresholds()

	return ad
}

// DetectAnomaly analyzes a single event for anomalies
func (ad *AnomalyDetector) DetectAnomaly(event *WorkflowExecutionEvent) *AnomalyResult {
	ad.log.WithFields(logrus.Fields{
		"event_type":   event.Type,
		"execution_id": event.ExecutionID,
		"workflow_id":  event.WorkflowID,
	}).Debug("Detecting anomalies in event")

	// Find applicable baseline models
	models := ad.findApplicableModels(event)
	if len(models) == 0 {
		ad.log.Debug("No applicable baseline models found")
		return nil
	}

	var bestAnomaly *AnomalyResult
	highestScore := 0.0

	// Run detection methods
	for _, method := range ad.detectionMethods {
		if !method.Enabled {
			continue
		}

		for _, model := range models {
			anomaly := ad.runDetectionMethod(event, model, method)
			if anomaly != nil && anomaly.AnomalyScore > highestScore {
				highestScore = anomaly.AnomalyScore
				bestAnomaly = anomaly
			}
		}
	}

	// Enhance anomaly with impact assessment and recommendations
	if bestAnomaly != nil {
		bestAnomaly.Impact = ad.assessImpact(bestAnomaly, event)
		bestAnomaly.Recommendations = ad.generateRecommendations(bestAnomaly)
	}

	return bestAnomaly
}

// DetectBatchAnomalies analyzes multiple executions for anomalies
func (ad *AnomalyDetector) DetectBatchAnomalies(ctx context.Context, executions []*engine.WorkflowExecutionData) (*AnomalyDetectionResult, error) {
	ad.log.WithField("executions", len(executions)).Info("Detecting batch anomalies")

	if len(executions) == 0 {
		return &AnomalyDetectionResult{
			Anomalies: []*AnomalyResult{},
			Summary:   &DetectionSummary{},
		}, nil
	}

	// Update baselines with recent data
	if err := ad.updateBaselines(executions); err != nil {
		ad.log.WithError(err).Warn("Failed to update baselines")
	}

	anomalies := make([]*AnomalyResult, 0)

	// Convert executions to events and detect anomalies
	for _, execution := range executions {
		event := ad.convertExecutionToEvent(execution)
		if anomaly := ad.DetectAnomaly(event); anomaly != nil {
			anomalies = append(anomalies, anomaly)
		}
	}

	// Analyze trends
	trendAnalysis, err := ad.analyzeTrends(executions)
	if err != nil {
		ad.log.WithError(err).Warn("Failed to analyze trends")
	}

	// Generate summary
	summary := ad.generateSummary(anomalies, len(executions))

	// Assess baseline health
	baselineHealth := ad.assessBaselineHealth()

	// Generate system recommendations
	systemRecommendations := ad.generateSystemRecommendations(anomalies, trendAnalysis)

	result := &AnomalyDetectionResult{
		Anomalies:       anomalies,
		Summary:         summary,
		BaselineHealth:  baselineHealth,
		TrendAnalysis:   trendAnalysis,
		Recommendations: systemRecommendations,
		AnalyzedPeriod: TimeRange{
			Start: executions[0].Timestamp,
			End:   executions[len(executions)-1].Timestamp,
		},
	}

	return result, nil
}

// UpdateBaseline updates baseline models with new execution data
func (ad *AnomalyDetector) UpdateBaseline(executions []*engine.WorkflowExecutionData) error {
	return ad.updateBaselines(executions)
}

// GetBaselineModels returns current baseline models
func (ad *AnomalyDetector) GetBaselineModels() map[string]*BaselineModel {
	models := make(map[string]*BaselineModel)
	for id, model := range ad.baselineModels {
		models[id] = model
	}
	return models
}

// Private methods

func (ad *AnomalyDetector) initializeDetectionMethods() {
	ad.detectionMethods = []DetectionMethod{
		{
			Name:      "Statistical Outlier Detection",
			Algorithm: "statistical",
			Parameters: map[string]interface{}{
				"z_threshold":   3.0,
				"iqr_threshold": 1.5,
			},
			Sensitivity: 0.7,
			Enabled:     true,
		},
		{
			Name:      "Temporal Anomaly Detection",
			Algorithm: "temporal",
			Parameters: map[string]interface{}{
				"window_size":      24,             // hours
				"seasonal_periods": []int{24, 168}, // daily, weekly
			},
			Sensitivity: 0.6,
			Enabled:     true,
		},
		{
			Name:      "Behavioral Anomaly Detection",
			Algorithm: "behavioral",
			Parameters: map[string]interface{}{
				"pattern_threshold": 0.8,
				"deviation_factor":  2.0,
			},
			Sensitivity: 0.5,
			Enabled:     true,
		},
	}
}

func (ad *AnomalyDetector) initializeAlertThresholds() {
	ad.alertThresholds = map[string]float64{
		"execution_failure_rate": 0.2, // 20% failure rate triggers alert
		"duration_increase":      2.0, // 2x normal duration
		"resource_spike":         3.0, // 3x normal resource usage
		"frequency_anomaly":      5.0, // 5x normal execution frequency
	}
}

func (ad *AnomalyDetector) findApplicableModels(event *WorkflowExecutionEvent) []*BaselineModel {
	models := make([]*BaselineModel, 0)

	for _, model := range ad.baselineModels {
		if ad.isModelApplicable(event, model) {
			models = append(models, model)
		}
	}

	return models
}

func (ad *AnomalyDetector) isModelApplicable(event *WorkflowExecutionEvent, model *BaselineModel) bool {
	// Check if model is recent enough
	if time.Since(model.UpdatedAt) > 7*24*time.Hour {
		return false
	}

	// Check if model has sufficient data
	if model.DataPoints < 20 {
		return false
	}

	// Check if event type matches model scope
	modelScope, exists := model.Parameters["scope"]
	if exists {
		if scope, ok := modelScope.(string); ok {
			if eventScope, hasScope := event.Context["scope"]; hasScope {
				if eventScope != scope {
					return false
				}
			}
		}
	}

	return true
}

func (ad *AnomalyDetector) runDetectionMethod(event *WorkflowExecutionEvent, model *BaselineModel, method DetectionMethod) *AnomalyResult {
	switch method.Algorithm {
	case "statistical":
		return ad.detectStatisticalAnomaly(event, model, method)
	case "temporal":
		return ad.detectTemporalAnomaly(event, model, method)
	case "behavioral":
		return ad.detectBehavioralAnomaly(event, model, method)
	default:
		return nil
	}
}

func (ad *AnomalyDetector) detectStatisticalAnomaly(event *WorkflowExecutionEvent, model *BaselineModel, method DetectionMethod) *AnomalyResult {
	zThreshold := method.Parameters["z_threshold"].(float64)

	// Check key metrics for statistical anomalies
	for metricName, value := range event.Metrics {
		if baseline := model.Statistics.Mean; baseline > 0 {
			zScore := math.Abs(value-baseline) / model.Statistics.StdDev

			if zScore > zThreshold {
				severity := ad.calculateSeverity(zScore, zThreshold)

				return &AnomalyResult{
					ID:              fmt.Sprintf("stat-anomaly-%d", time.Now().Unix()),
					Type:            "statistical",
					Severity:        severity,
					DetectedAt:      time.Now(),
					Event:           event,
					BaselineModel:   model.ID,
					DetectionMethod: method.Name,
					AnomalyScore:    math.Min(zScore/zThreshold, 1.0),
					Confidence:      model.Confidence,
					Description:     fmt.Sprintf("Statistical anomaly in %s: value %.2f (z-score: %.2f)", metricName, value, zScore),
					Metadata: map[string]interface{}{
						"metric":    metricName,
						"value":     value,
						"baseline":  baseline,
						"z_score":   zScore,
						"threshold": zThreshold,
					},
				}
			}
		}
	}

	return nil
}

func (ad *AnomalyDetector) detectTemporalAnomaly(event *WorkflowExecutionEvent, model *BaselineModel, method DetectionMethod) *AnomalyResult {
	if model.TemporalPatterns == nil {
		return nil
	}

	hour := event.Timestamp.Hour()
	weekday := event.Timestamp.Weekday()

	// Check hourly pattern
	if hourlyPattern, exists := model.TemporalPatterns["hourly"]; exists {
		hourKey := fmt.Sprintf("hour_%d", hour)
		if expectedValue, hasExpected := hourlyPattern.Expected[hourKey]; hasExpected {
			if variance, hasVariance := hourlyPattern.Variance[hourKey]; hasVariance {

				// Use execution success as the temporal metric
				actualValue := 0.0
				if success, exists := event.Data["success"]; exists {
					if successBool, ok := success.(bool); ok && successBool {
						actualValue = 1.0
					}
				}

				// Calculate deviation
				deviation := math.Abs(actualValue - expectedValue)
				threshold := 2 * math.Sqrt(variance) // 2 sigma threshold

				if deviation > threshold {
					severity := ad.calculateSeverity(deviation, threshold)

					return &AnomalyResult{
						ID:              fmt.Sprintf("temporal-anomaly-%d", time.Now().Unix()),
						Type:            "temporal",
						Severity:        severity,
						DetectedAt:      time.Now(),
						Event:           event,
						BaselineModel:   model.ID,
						DetectionMethod: method.Name,
						AnomalyScore:    math.Min(deviation/threshold, 1.0),
						Confidence:      hourlyPattern.Seasonality,
						Description:     fmt.Sprintf("Temporal anomaly at hour %d: expected %.2f, got %.2f", hour, expectedValue, actualValue),
						Metadata: map[string]interface{}{
							"hour":      hour,
							"weekday":   weekday.String(),
							"expected":  expectedValue,
							"actual":    actualValue,
							"deviation": deviation,
							"threshold": threshold,
						},
					}
				}
			}
		}
	}

	return nil
}

func (ad *AnomalyDetector) detectBehavioralAnomaly(event *WorkflowExecutionEvent, model *BaselineModel, method DetectionMethod) *AnomalyResult {
	patternThreshold := method.Parameters["pattern_threshold"].(float64)

	// Analyze execution patterns (simplified)
	if workflowID, exists := event.Data["workflow_id"]; exists {
		if templateID, exists := event.Data["template_id"]; exists {
			// Check if this workflow-template combination is unusual

			// Get recent execution frequency for this combination
			recentCount := ad.getRecentExecutionCount(workflowID.(string), templateID.(string))

			// Compare with baseline frequency
			baselineFreq := 1.0 // Default baseline
			if baselineData, exists := model.Parameters["workflow_frequencies"]; exists {
				if freqMap, ok := baselineData.(map[string]float64); ok {
					key := fmt.Sprintf("%s:%s", workflowID, templateID)
					if freq, hasFreq := freqMap[key]; hasFreq {
						baselineFreq = freq
					}
				}
			}

			// Calculate anomaly score based on frequency deviation
			frequencyRatio := float64(recentCount) / baselineFreq
			if frequencyRatio > 1.0/patternThreshold || frequencyRatio < patternThreshold {
				severity := "medium"
				if frequencyRatio > 3.0 || frequencyRatio < 0.3 {
					severity = "high"
				}

				return &AnomalyResult{
					ID:              fmt.Sprintf("behavioral-anomaly-%d", time.Now().Unix()),
					Type:            "behavioral",
					Severity:        severity,
					DetectedAt:      time.Now(),
					Event:           event,
					BaselineModel:   model.ID,
					DetectionMethod: method.Name,
					AnomalyScore:    math.Min(math.Abs(1.0-frequencyRatio), 1.0),
					Confidence:      0.7, // Medium confidence for behavioral detection
					Description:     fmt.Sprintf("Behavioral anomaly: workflow execution frequency %.2fx normal", frequencyRatio),
					Metadata: map[string]interface{}{
						"workflow_id":     workflowID,
						"template_id":     templateID,
						"recent_count":    recentCount,
						"baseline_freq":   baselineFreq,
						"frequency_ratio": frequencyRatio,
					},
				}
			}
		}
	}

	return nil
}

func (ad *AnomalyDetector) updateBaselines(executions []*engine.WorkflowExecutionData) error {
	// Add data to historical buffer
	for _, execution := range executions {
		ad.historicalData.Add(execution)
	}

	// Update or create baseline models
	if err := ad.updateStatisticalBaseline(executions); err != nil {
		return fmt.Errorf("failed to update statistical baseline: %w", err)
	}

	if err := ad.updateTemporalBaseline(executions); err != nil {
		return fmt.Errorf("failed to update temporal baseline: %w", err)
	}

	if err := ad.updateBehavioralBaseline(executions); err != nil {
		return fmt.Errorf("failed to update behavioral baseline: %w", err)
	}

	return nil
}

func (ad *AnomalyDetector) updateStatisticalBaseline(executions []*engine.WorkflowExecutionData) error {
	modelID := "statistical_baseline"

	// Extract metrics
	durations := make([]float64, 0)
	successRate := 0.0
	resourceUsages := make(map[string][]float64)

	for _, execution := range executions {
		// Use Duration field directly from execution
		if execution.Duration > 0 {
			durations = append(durations, execution.Duration.Minutes())
		}

		// Use Success field directly
		if execution.Success {
			successRate += 1.0
		}

		// Extract resource usage from Metrics if available
		if execution.Metrics != nil {
			if _, exists := resourceUsages["cpu"]; !exists {
				resourceUsages["cpu"] = make([]float64, 0)
				resourceUsages["memory"] = make([]float64, 0)
				resourceUsages["network"] = make([]float64, 0)
				resourceUsages["storage"] = make([]float64, 0)
			}
			if cpu, exists := execution.Metrics["cpu_usage"]; exists {
				resourceUsages["cpu"] = append(resourceUsages["cpu"], cpu)
			}
			if memory, exists := execution.Metrics["memory_usage"]; exists {
				resourceUsages["memory"] = append(resourceUsages["memory"], memory)
			}
			if network, exists := execution.Metrics["network_usage"]; exists {
				resourceUsages["network"] = append(resourceUsages["network"], network)
			}
			if storage, exists := execution.Metrics["storage_usage"]; exists {
				resourceUsages["storage"] = append(resourceUsages["storage"], storage)
			}
		}
	}

	if len(durations) == 0 {
		return nil
	}

	successRate /= float64(len(executions))

	// Calculate statistics
	stats := &BaselineStatistics{
		Mean:   statMean(durations),
		StdDev: statStdDev(durations),
		Median: 0.0, // Will be calculated below
		Min:    statMin(durations),
		Max:    statMax(durations),
	}

	// Calculate quartiles
	sortedDurations := make([]float64, len(durations))
	copy(sortedDurations, durations)
	sort.Float64s(sortedDurations)

	n := len(sortedDurations)
	if n >= 4 {
		stats.Q1 = sortedDurations[n/4]
		stats.Q3 = sortedDurations[3*n/4]
		stats.IQR = stats.Q3 - stats.Q1
		stats.Median = sortedDurations[n/2]
	}

	// Create or update model
	model := &BaselineModel{
		ID:         modelID,
		Type:       "statistical",
		UpdatedAt:  time.Now(),
		DataPoints: len(executions),
		Statistics: stats,
		Parameters: map[string]interface{}{
			"success_rate": successRate,
		},
		Confidence: math.Min(float64(len(executions))/100.0, 1.0), // Higher confidence with more data
	}

	if existingModel, exists := ad.baselineModels[modelID]; exists {
		model.CreatedAt = existingModel.CreatedAt
		model.DataPoints += existingModel.DataPoints
	} else {
		model.CreatedAt = time.Now()
	}

	ad.baselineModels[modelID] = model
	return nil
}

func (ad *AnomalyDetector) updateTemporalBaseline(executions []*engine.WorkflowExecutionData) error {
	modelID := "temporal_baseline"

	// Group executions by hour and weekday
	hourlyData := make(map[int][]float64)
	weeklyData := make(map[time.Weekday][]float64)

	for _, execution := range executions {
		hour := execution.Timestamp.Hour()
		weekday := execution.Timestamp.Weekday()

		successValue := 0.0
		if execution.Success {
			successValue = 1.0
		}

		if _, exists := hourlyData[hour]; !exists {
			hourlyData[hour] = make([]float64, 0)
		}
		hourlyData[hour] = append(hourlyData[hour], successValue)

		if _, exists := weeklyData[weekday]; !exists {
			weeklyData[weekday] = make([]float64, 0)
		}
		weeklyData[weekday] = append(weeklyData[weekday], successValue)
	}

	// Calculate hourly patterns
	hourlyPattern := &TemporalBaseline{
		Pattern:  "hourly",
		Expected: make(map[string]float64),
		Variance: make(map[string]float64),
	}

	for hour, values := range hourlyData {
		if len(values) > 0 {
			hourKey := fmt.Sprintf("hour_%d", hour)
			hourlyPattern.Expected[hourKey] = statMean(values)
			hourlyPattern.Variance[hourKey] = statVariance(values)
		}
	}

	// Calculate seasonality strength (simplified)
	hourlyMeans := make([]float64, 0)
	for _, mean := range hourlyPattern.Expected {
		hourlyMeans = append(hourlyMeans, mean)
	}
	if len(hourlyMeans) > 1 {
		hourlyPattern.Seasonality = statVariance(hourlyMeans)
	}

	// Create temporal patterns map
	temporalPatterns := map[string]*TemporalBaseline{
		"hourly": hourlyPattern,
	}

	// Create or update model
	model := &BaselineModel{
		ID:               modelID,
		Type:             "temporal",
		UpdatedAt:        time.Now(),
		DataPoints:       len(executions),
		TemporalPatterns: temporalPatterns,
		Confidence:       math.Min(float64(len(executions))/200.0, 1.0),
	}

	if existingModel, exists := ad.baselineModels[modelID]; exists {
		model.CreatedAt = existingModel.CreatedAt
		model.DataPoints += existingModel.DataPoints
	} else {
		model.CreatedAt = time.Now()
	}

	ad.baselineModels[modelID] = model
	return nil
}

func (ad *AnomalyDetector) updateBehavioralBaseline(executions []*engine.WorkflowExecutionData) error {
	modelID := "behavioral_baseline"

	// Analyze workflow execution patterns
	workflowFreq := make(map[string]int)
	templateFreq := make(map[string]int)

	for _, execution := range executions {
		// Extract alert info from metadata if available
		if execution.Metadata != nil {
			if alertName, exists := execution.Metadata["alert_name"]; exists {
				workflowKey := fmt.Sprintf("alert:%s", alertName)
				workflowFreq[workflowKey]++
			}
		}

		// Use WorkflowID as template identifier
		templateKey := fmt.Sprintf("template:%s", execution.WorkflowID)
		templateFreq[templateKey]++
	}

	// Convert to frequencies (executions per day)
	timeSpan := 1.0 // days
	if len(executions) > 1 {
		timeSpan = executions[len(executions)-1].Timestamp.Sub(executions[0].Timestamp).Hours() / 24.0
	}

	workflowFrequencies := make(map[string]float64)
	for key, count := range workflowFreq {
		workflowFrequencies[key] = float64(count) / timeSpan
	}

	// Create or update model
	model := &BaselineModel{
		ID:         modelID,
		Type:       "behavioral",
		UpdatedAt:  time.Now(),
		DataPoints: len(executions),
		Parameters: map[string]interface{}{
			"workflow_frequencies": workflowFrequencies,
			"time_span_days":       timeSpan,
		},
		Confidence: math.Min(float64(len(executions))/150.0, 1.0),
	}

	if existingModel, exists := ad.baselineModels[modelID]; exists {
		model.CreatedAt = existingModel.CreatedAt
		model.DataPoints += existingModel.DataPoints
	} else {
		model.CreatedAt = time.Now()
	}

	ad.baselineModels[modelID] = model
	return nil
}

func (ad *AnomalyDetector) convertExecutionToEvent(execution *engine.WorkflowExecutionData) *WorkflowExecutionEvent {
	event := &WorkflowExecutionEvent{
		Type:        "workflow_execution",
		ExecutionID: execution.ExecutionID,
		Timestamp:   execution.Timestamp,
		Data:        make(map[string]interface{}),
		Metrics:     make(map[string]float64),
		Context:     make(map[string]interface{}),
	}

	// Add execution data from main fields
	event.Data["success"] = execution.Success
	event.Metrics["duration_minutes"] = execution.Duration.Minutes()

	// Copy metrics from execution
	if execution.Metrics != nil {
		for key, value := range execution.Metrics {
			event.Metrics[key] = value
		}
	}

	// Copy metadata and context
	if execution.Metadata != nil {
		for key, value := range execution.Metadata {
			event.Context[key] = value
		}

		// Extract common fields from metadata
		if alertName, exists := execution.Metadata["alert_name"]; exists {
			event.Data["alert_name"] = alertName
		}
		if alertSeverity, exists := execution.Metadata["alert_severity"]; exists {
			event.Data["alert_severity"] = alertSeverity
		}
		if namespace, exists := execution.Metadata["namespace"]; exists {
			event.Context["namespace"] = namespace
		}
		if resource, exists := execution.Metadata["resource"]; exists {
			event.Context["resource"] = resource
		}
	}

	event.Data["workflow_id"] = execution.WorkflowID
	event.Data["template_id"] = execution.WorkflowID // Use WorkflowID as template ID

	return event
}

// Helper methods

func (ad *AnomalyDetector) calculateSeverity(score, threshold float64) string {
	ratio := score / threshold
	if ratio > 4.0 {
		return "critical"
	} else if ratio > 2.5 {
		return "high"
	} else if ratio > 1.5 {
		return "medium"
	}
	return "low"
}

func (ad *AnomalyDetector) getRecentExecutionCount(workflowID, templateID string) int {
	// This would query recent execution history
	// For now, return a placeholder
	return 1
}

func (ad *AnomalyDetector) assessImpact(anomaly *AnomalyResult, event *WorkflowExecutionEvent) *AnomalyImpactAssessment {
	impact := &AnomalyImpactAssessment{
		Scope:             "workflow",
		AffectedResources: []string{},
		BusinessImpact:    "low",
		TechnicalImpact:   "low",
		EstimatedCost:     0.0,
		RecoveryTime:      5 * time.Minute,
	}

	// Adjust impact based on anomaly severity
	switch anomaly.Severity {
	case "critical":
		impact.BusinessImpact = "high"
		impact.TechnicalImpact = "high"
		impact.RecoveryTime = 30 * time.Minute
	case "high":
		impact.BusinessImpact = "medium"
		impact.TechnicalImpact = "medium"
		impact.RecoveryTime = 15 * time.Minute
	case "medium":
		impact.BusinessImpact = "low"
		impact.TechnicalImpact = "medium"
		impact.RecoveryTime = 10 * time.Minute
	}

	// Add affected resources from event context
	if namespace, exists := event.Context["namespace"]; exists {
		impact.AffectedResources = append(impact.AffectedResources, fmt.Sprintf("namespace:%s", namespace))
	}
	if resource, exists := event.Context["resource"]; exists {
		impact.AffectedResources = append(impact.AffectedResources, fmt.Sprintf("resource:%s", resource))
	}

	return impact
}

func (ad *AnomalyDetector) generateRecommendations(anomaly *AnomalyResult) []*AnomalyRecommendation {
	recommendations := make([]*AnomalyRecommendation, 0)

	switch anomaly.Type {
	case "statistical":
		recommendations = append(recommendations, &AnomalyRecommendation{
			Action:      "investigate_metrics",
			Description: "Investigate the root cause of unusual metric values",
			Priority:    1,
			Urgency:     "normal",
			AutomateIt:  false,
		})

	case "temporal":
		recommendations = append(recommendations, &AnomalyRecommendation{
			Action:      "check_scheduling",
			Description: "Review workflow scheduling and timing patterns",
			Priority:    2,
			Urgency:     "normal",
			AutomateIt:  true,
		})

	case "behavioral":
		recommendations = append(recommendations, &AnomalyRecommendation{
			Action:      "review_automation",
			Description: "Review automated triggering rules and frequency controls",
			Priority:    1,
			Urgency:     "urgent",
			AutomateIt:  false,
		})
	}

	// Add severity-based recommendations
	if anomaly.Severity == "critical" || anomaly.Severity == "high" {
		recommendations = append(recommendations, &AnomalyRecommendation{
			Action:      "immediate_investigation",
			Description: "Immediate investigation required due to high severity",
			Priority:    0,
			Urgency:     "immediate",
			AutomateIt:  false,
		})
	}

	return recommendations
}

func (ad *AnomalyDetector) analyzeTrends(executions []*engine.WorkflowExecutionData) (*TimeSeriesTrendAnalysis, error) {
	if len(executions) < 10 {
		return nil, fmt.Errorf("insufficient data for trend analysis")
	}

	// Extract success rates over time windows
	windowSize := 24 * time.Hour
	windows := make(map[time.Time]float64)

	for _, execution := range executions {
		windowStart := execution.Timestamp.Truncate(windowSize)
		if _, exists := windows[windowStart]; !exists {
			windows[windowStart] = 0.0
		}

		if execution.Success {
			windows[windowStart] += 1.0
		}
	}

	// Convert to time series
	timestamps := make([]time.Time, 0)
	values := make([]float64, 0)

	for timestamp, successCount := range windows {
		timestamps = append(timestamps, timestamp)
		values = append(values, successCount)
	}

	// Sort by timestamp
	for i := 0; i < len(timestamps)-1; i++ {
		for j := i + 1; j < len(timestamps); j++ {
			if timestamps[i].After(timestamps[j]) {
				timestamps[i], timestamps[j] = timestamps[j], timestamps[i]
				values[i], values[j] = values[j], values[i]
			}
		}
	}

	// Calculate linear trend
	x := make([]float64, len(values))
	for i := range x {
		x[i] = float64(i)
	}

	// Simple linear regression
	n := float64(len(x))
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumXX := 0.0

	for i := 0; i < len(x); i++ {
		sumX += x[i]
		sumY += values[i]
		sumXY += x[i] * values[i]
		sumXX += x[i] * x[i]
	}

	slope := (n*sumXY - sumX*sumY) / (n*sumXX - sumX*sumX)

	direction := "stable"
	if math.Abs(slope) > 0.1 {
		if slope > 0 {
			direction = "increasing"
		} else {
			direction = "decreasing"
		}
	}

	// Calculate R-squared for confidence
	yMean := sumY / n
	ssRes := 0.0
	ssTot := 0.0
	for i := 0; i < len(values); i++ {
		predicted := slope*x[i] + (sumY-slope*sumX)/n
		diff1 := values[i] - predicted
		ssRes += diff1 * diff1
		diff2 := values[i] - yMean
		ssTot += diff2 * diff2
	}

	confidence := 0.0
	if ssTot > 0 {
		confidence = math.Max(0, 1.0-(ssRes/ssTot))
	}

	trend := &TimeSeriesTrendAnalysis{
		Direction:    direction,
		Slope:        slope,
		Confidence:   confidence,
		StartValue:   values[0],
		EndValue:     values[len(values)-1],
		TrendPeriod:  TimeRange{Start: timestamps[0], End: timestamps[len(timestamps)-1]},
		Significance: math.Abs(slope) * confidence,
	}

	return trend, nil
}

func (ad *AnomalyDetector) generateSummary(anomalies []*AnomalyResult, totalExecutions int) *DetectionSummary {
	summary := &DetectionSummary{
		TotalAnomalies:    len(anomalies),
		SeverityBreakdown: make(map[string]int),
		TypeBreakdown:     make(map[string]int),
		DetectionRate:     0.0,
		Coverage:          1.0, // Assume full coverage for now
	}

	for _, anomaly := range anomalies {
		summary.SeverityBreakdown[anomaly.Severity]++
		summary.TypeBreakdown[anomaly.Type]++
	}

	if totalExecutions > 0 {
		summary.DetectionRate = float64(len(anomalies)) / float64(totalExecutions)
	}

	return summary
}

func (ad *AnomalyDetector) assessBaselineHealth() *BaselineHealthReport {
	report := &BaselineHealthReport{
		ModelsEvaluated:    len(ad.baselineModels),
		ModelHealth:        make(map[string]float64),
		RecommendedActions: make([]string, 0),
	}

	healthyCount := 0
	staleCount := 0
	inaccurateCount := 0

	for id, model := range ad.baselineModels {
		health := ad.calculateModelHealth(model)
		report.ModelHealth[id] = health

		if health > 0.8 {
			healthyCount++
		} else if time.Since(model.UpdatedAt) > 7*24*time.Hour {
			staleCount++
		} else if model.Confidence < 0.5 {
			inaccurateCount++
		}
	}

	report.HealthyModels = healthyCount
	report.StaleModels = staleCount
	report.InaccurateModels = inaccurateCount

	// Generate recommendations
	if staleCount > 0 {
		report.RecommendedActions = append(report.RecommendedActions, "Update stale baseline models with recent data")
	}
	if inaccurateCount > 0 {
		report.RecommendedActions = append(report.RecommendedActions, "Improve accuracy of baseline models by collecting more training data")
	}

	return report
}

func (ad *AnomalyDetector) calculateModelHealth(model *BaselineModel) float64 {
	health := 0.0

	// Factor 1: Recency (50% weight)
	daysSinceUpdate := time.Since(model.UpdatedAt).Hours() / 24.0
	recencyScore := math.Max(0, 1.0-daysSinceUpdate/30.0) // Degrade over 30 days
	health += 0.5 * recencyScore

	// Factor 2: Data volume (30% weight)
	dataScore := math.Min(float64(model.DataPoints)/100.0, 1.0) // Saturate at 100 data points
	health += 0.3 * dataScore

	// Factor 3: Confidence (20% weight)
	health += 0.2 * model.Confidence

	return health
}

func (ad *AnomalyDetector) generateSystemRecommendations(anomalies []*AnomalyResult, trends *TimeSeriesTrendAnalysis) []*SystemRecommendation {
	recommendations := make([]*SystemRecommendation, 0)

	// Analyze anomaly patterns
	highSeverityCount := 0
	for _, anomaly := range anomalies {
		if anomaly.Severity == "critical" || anomaly.Severity == "high" {
			highSeverityCount++
		}
	}

	if highSeverityCount > 0 {
		recommendations = append(recommendations, &SystemRecommendation{
			Type:           "alerting",
			Title:          "Improve Anomaly Response",
			Description:    fmt.Sprintf("Detected %d high-severity anomalies. Consider implementing automated response workflows.", highSeverityCount),
			Impact:         "high",
			Effort:         "medium",
			Priority:       1,
			Implementation: "Configure automated remediation for common anomaly patterns",
		})
	}

	// Trend-based recommendations
	if trends != nil && trends.Direction == "decreasing" && trends.Significance > 0.3 {
		recommendations = append(recommendations, &SystemRecommendation{
			Type:           "performance",
			Title:          "Address Performance Degradation",
			Description:    "Detected declining trend in workflow success rates",
			Impact:         "medium",
			Effort:         "high",
			Priority:       2,
			Implementation: "Investigate root causes of declining performance and implement corrective measures",
		})
	}

	return recommendations
}

// HistoricalDataBuffer implementation

func NewHistoricalDataBuffer(maxSize int) *HistoricalDataBuffer {
	return &HistoricalDataBuffer{
		maxSize: maxSize,
		data:    make([]*engine.WorkflowExecutionData, maxSize),
		index:   0,
		full:    false,
	}
}

func (hdb *HistoricalDataBuffer) Add(execution *engine.WorkflowExecutionData) {
	hdb.data[hdb.index] = execution
	hdb.index = (hdb.index + 1) % hdb.maxSize

	if hdb.index == 0 {
		hdb.full = true
	}
}

func (hdb *HistoricalDataBuffer) GetRecent(count int) []*engine.WorkflowExecutionData {
	if count <= 0 {
		return []*engine.WorkflowExecutionData{}
	}

	size := hdb.maxSize
	if !hdb.full {
		size = hdb.index
	}

	if count > size {
		count = size
	}

	result := make([]*engine.WorkflowExecutionData, count)
	for i := 0; i < count; i++ {
		idx := (hdb.index - 1 - i + hdb.maxSize) % hdb.maxSize
		result[i] = hdb.data[idx]
	}

	return result
}

// Simple statistical functions to replace gonum dependencies
func statMean(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}

func statMin(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	minimum := data[0]
	for _, v := range data {
		if v < minimum {
			minimum = v
		}
	}
	return minimum
}

func statMax(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	maximum := data[0]
	for _, v := range data {
		if v > maximum {
			maximum = v
		}
	}
	return maximum
}

func statStdDev(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	m := statMean(data)
	sum := 0.0
	for _, v := range data {
		sum += (v - m) * (v - m)
	}
	return math.Sqrt(sum / float64(len(data)))
}

func statVariance(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	m := statMean(data)
	sum := 0.0
	for _, v := range data {
		sum += (v - m) * (v - m)
	}
	return sum / float64(len(data))
}
