package conditions

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

// AIPerformanceMetrics tracks comprehensive AI performance data
type AIPerformanceMetrics struct {
	// Request metrics
	TotalRequests      int64         `json:"total_requests"`
	SuccessfulRequests int64         `json:"successful_requests"`
	FailedRequests     int64         `json:"failed_requests"`
	AverageLatency     time.Duration `json:"average_latency"`
	P95Latency         time.Duration `json:"p95_latency"`
	P99Latency         time.Duration `json:"p99_latency"`

	// Quality metrics
	AverageQualityScore  float64          `json:"average_quality_score"`
	QualityDistribution  map[string]int64 `json:"quality_distribution"` // excellent, good, fair, poor
	ResponseValidityRate float64          `json:"response_validity_rate"`

	// Content metrics
	AverageResponseLength int     `json:"average_response_length"`
	AverageStepsGenerated float64 `json:"average_steps_generated"`
	PatternUsageRate      float64 `json:"pattern_usage_rate"`
	SafetyComplianceRate  float64 `json:"safety_compliance_rate"`

	// Error analysis
	ErrorsByType     map[string]int64 `json:"errors_by_type"`
	ErrorsByProvider map[string]int64 `json:"errors_by_provider"`
	RetrySuccessRate float64          `json:"retry_success_rate"`

	// Temporal metrics
	LastUpdated  time.Time                `json:"last_updated"`
	DailyMetrics map[string]*DailyMetrics `json:"daily_metrics"`
	HourlyTrends []float64                `json:"hourly_trends"`
}

// DailyMetrics tracks daily performance data
type DailyMetrics struct {
	Date                string         `json:"date"`
	Requests            int64          `json:"requests"`
	SuccessRate         float64        `json:"success_rate"`
	AverageLatency      time.Duration  `json:"average_latency"`
	AverageQualityScore float64        `json:"average_quality_score"`
	TopErrors           []ErrorSummary `json:"top_errors"`
}

// ErrorSummary provides error analysis
type ErrorSummary struct {
	Type        string    `json:"type"`
	Count       int64     `json:"count"`
	Percentage  float64   `json:"percentage"`
	LastSeen    time.Time `json:"last_seen"`
	SampleError string    `json:"sample_error"`
}

// AIResponseQuality represents the quality assessment of an AI response
type AIResponseQuality struct {
	OverallScore     float64                `json:"overall_score"`
	Completeness     float64                `json:"completeness"`
	Accuracy         float64                `json:"accuracy"`
	Safety           float64                `json:"safety"`
	Relevance        float64                `json:"relevance"`
	Clarity          float64                `json:"clarity"`
	Innovation       float64                `json:"innovation"`
	Issues           []QualityIssue         `json:"issues"`
	Strengths        []string               `json:"strengths"`
	ScoringBreakdown map[string]interface{} `json:"scoring_breakdown"`
}

// QualityIssue represents a specific quality concern
type QualityIssue struct {
	Type        string  `json:"type"`
	Severity    string  `json:"severity"` // critical, major, minor
	Description string  `json:"description"`
	Impact      float64 `json:"impact"`
	Suggestion  string  `json:"suggestion"`
}

// AIMetricsCollector collects and analyzes AI performance metrics
type AIMetricsCollector struct {
	metrics   *AIPerformanceMetrics
	latencies []time.Duration
	qualities []float64
	responses []*AIResponseRecord
	mu        sync.RWMutex
	log       *logrus.Logger

	// Configuration
	config *MetricsConfig
}

// AIResponseRecord tracks individual AI response details
type AIResponseRecord struct {
	ID               string                 `json:"id"`
	Timestamp        time.Time              `json:"timestamp"`
	Provider         string                 `json:"provider"`
	Model            string                 `json:"model"`
	PromptLength     int                    `json:"prompt_length"`
	ResponseLength   int                    `json:"response_length"`
	Latency          time.Duration          `json:"latency"`
	Success          bool                   `json:"success"`
	Error            string                 `json:"error,omitempty"`
	Quality          *AIResponseQuality     `json:"quality,omitempty"`
	ValidationPassed bool                   `json:"validation_passed"`
	StepsGenerated   int                    `json:"steps_generated"`
	PatternsUsed     int                    `json:"patterns_used"`
	SafetyFlags      []string               `json:"safety_flags"`
	RetryAttempts    int                    `json:"retry_attempts"`
	Context          map[string]interface{} `json:"context"`
}

// MetricsConfig configures the metrics collection behavior
type MetricsConfig struct {
	EnableDetailedTracking    bool          `yaml:"enable_detailed_tracking" default:"true"`
	MaxRecordHistory          int           `yaml:"max_record_history" default:"10000"`
	MetricsRetentionDays      int           `yaml:"metrics_retention_days" default:"30"`
	QualityCheckInterval      time.Duration `yaml:"quality_check_interval" default:"1h"`
	PerformanceAlertThreshold float64       `yaml:"performance_alert_threshold" default:"0.8"`
	EnableRealTimeAlerts      bool          `yaml:"enable_realtime_alerts" default:"true"`
}

// NewAIMetricsCollector creates a new AI metrics collector
func NewAIMetricsCollector(config *MetricsConfig, log *logrus.Logger) *AIMetricsCollector {
	if config == nil {
		config = &MetricsConfig{
			EnableDetailedTracking:    true,
			MaxRecordHistory:          10000,
			MetricsRetentionDays:      30,
			QualityCheckInterval:      time.Hour,
			PerformanceAlertThreshold: 0.8,
			EnableRealTimeAlerts:      true,
		}
	}

	return &AIMetricsCollector{
		metrics: &AIPerformanceMetrics{
			QualityDistribution: make(map[string]int64),
			ErrorsByType:        make(map[string]int64),
			ErrorsByProvider:    make(map[string]int64),
			DailyMetrics:        make(map[string]*DailyMetrics),
			HourlyTrends:        make([]float64, 24),
		},
		latencies: make([]time.Duration, 0),
		qualities: make([]float64, 0),
		responses: make([]*AIResponseRecord, 0),
		config:    config,
		log:       log,
	}
}

// RecordAIRequest records metrics for an AI request
func (amc *AIMetricsCollector) RecordAIRequest(ctx context.Context, record *AIResponseRecord) {
	amc.mu.Lock()
	defer amc.mu.Unlock()

	// Update basic metrics
	amc.metrics.TotalRequests++

	if record.Success {
		amc.metrics.SuccessfulRequests++
	} else {
		amc.metrics.FailedRequests++

		// Track error by type
		errorType := amc.categorizeError(record.Error)
		amc.metrics.ErrorsByType[errorType]++
		amc.metrics.ErrorsByProvider[record.Provider]++
	}

	// Update latency metrics
	amc.latencies = append(amc.latencies, record.Latency)
	amc.updateLatencyMetrics()

	// Update quality metrics if available
	if record.Quality != nil {
		amc.qualities = append(amc.qualities, record.Quality.OverallScore)
		amc.updateQualityMetrics(record.Quality)
	}

	// Update content metrics
	amc.updateContentMetrics(record)

	// Store detailed record if enabled
	if amc.config.EnableDetailedTracking {
		amc.responses = append(amc.responses, record)

		// Trim old records if necessary
		if len(amc.responses) > amc.config.MaxRecordHistory {
			amc.responses = amc.responses[len(amc.responses)-amc.config.MaxRecordHistory:]
		}
	}

	// Update daily metrics
	amc.updateDailyMetrics(record)

	// Update hourly trends
	hour := record.Timestamp.Hour()
	if record.Success && record.Quality != nil {
		amc.metrics.HourlyTrends[hour] = record.Quality.OverallScore
	}

	amc.metrics.LastUpdated = time.Now()

	// Check for performance alerts
	if amc.config.EnableRealTimeAlerts {
		amc.checkPerformanceAlerts(record)
	}
}

// updateLatencyMetrics updates latency-related metrics
func (amc *AIMetricsCollector) updateLatencyMetrics() {
	if len(amc.latencies) == 0 {
		return
	}

	// Calculate average latency
	total := time.Duration(0)
	for _, latency := range amc.latencies {
		total += latency
	}
	amc.metrics.AverageLatency = total / time.Duration(len(amc.latencies))

	// Calculate percentiles
	sorted := make([]time.Duration, len(amc.latencies))
	copy(sorted, amc.latencies)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	p95Index := int(float64(len(sorted)) * 0.95)
	p99Index := int(float64(len(sorted)) * 0.99)

	if p95Index >= len(sorted) {
		p95Index = len(sorted) - 1
	}
	if p99Index >= len(sorted) {
		p99Index = len(sorted) - 1
	}

	amc.metrics.P95Latency = sorted[p95Index]
	amc.metrics.P99Latency = sorted[p99Index]
}

// updateQualityMetrics updates quality-related metrics
func (amc *AIMetricsCollector) updateQualityMetrics(quality *AIResponseQuality) {
	if len(amc.qualities) == 0 {
		return
	}

	// Calculate average quality score
	total := 0.0
	for _, score := range amc.qualities {
		total += score
	}
	amc.metrics.AverageQualityScore = total / float64(len(amc.qualities))

	// Update quality distribution
	qualityLevel := amc.categorizeQuality(quality.OverallScore)
	amc.metrics.QualityDistribution[qualityLevel]++
}

// updateContentMetrics updates content-related metrics
func (amc *AIMetricsCollector) updateContentMetrics(record *AIResponseRecord) {
	// Update average response length
	totalResponses := amc.metrics.TotalRequests
	if totalResponses == 1 {
		amc.metrics.AverageResponseLength = record.ResponseLength
	} else {
		currentAvg := float64(amc.metrics.AverageResponseLength)
		newAvg := (currentAvg*float64(totalResponses-1) + float64(record.ResponseLength)) / float64(totalResponses)
		amc.metrics.AverageResponseLength = int(newAvg)
	}

	// Update average steps generated
	if record.StepsGenerated > 0 {
		if totalResponses == 1 {
			amc.metrics.AverageStepsGenerated = float64(record.StepsGenerated)
		} else {
			currentAvg := amc.metrics.AverageStepsGenerated
			newAvg := (currentAvg*float64(totalResponses-1) + float64(record.StepsGenerated)) / float64(totalResponses)
			amc.metrics.AverageStepsGenerated = newAvg
		}
	}

	// Update pattern usage rate
	if record.PatternsUsed > 0 {
		successfulWithPatterns := float64(0)
		successfulTotal := float64(amc.metrics.SuccessfulRequests)

		for _, resp := range amc.responses {
			if resp.Success && resp.PatternsUsed > 0 {
				successfulWithPatterns++
			}
		}

		if successfulTotal > 0 {
			amc.metrics.PatternUsageRate = successfulWithPatterns / successfulTotal
		}
	}

	// Update safety compliance rate
	if len(record.SafetyFlags) == 0 {
		successfulSafe := float64(0)
		successfulTotal := float64(amc.metrics.SuccessfulRequests)

		for _, resp := range amc.responses {
			if resp.Success && len(resp.SafetyFlags) == 0 {
				successfulSafe++
			}
		}

		if successfulTotal > 0 {
			amc.metrics.SafetyComplianceRate = successfulSafe / successfulTotal
		}
	}

	// Update validation rate
	validationSuccess := float64(0)
	totalValidations := float64(amc.metrics.TotalRequests)

	for _, resp := range amc.responses {
		if resp.ValidationPassed {
			validationSuccess++
		}
	}

	if totalValidations > 0 {
		amc.metrics.ResponseValidityRate = validationSuccess / totalValidations
	}
}

// updateDailyMetrics updates daily performance metrics
func (amc *AIMetricsCollector) updateDailyMetrics(record *AIResponseRecord) {
	dateKey := record.Timestamp.Format("2006-01-02")

	dailyMetric, exists := amc.metrics.DailyMetrics[dateKey]
	if !exists {
		dailyMetric = &DailyMetrics{
			Date:      dateKey,
			Requests:  0,
			TopErrors: make([]ErrorSummary, 0),
		}
		amc.metrics.DailyMetrics[dateKey] = dailyMetric
	}

	dailyMetric.Requests++

	// Update daily success rate
	successfulToday := int64(0)
	totalToday := dailyMetric.Requests

	// Count successful requests for today
	for _, resp := range amc.responses {
		if resp.Timestamp.Format("2006-01-02") == dateKey && resp.Success {
			successfulToday++
		}
	}

	if totalToday > 0 {
		dailyMetric.SuccessRate = float64(successfulToday) / float64(totalToday)
	}

	// Update daily average latency and quality
	if record.Success {
		if record.Quality != nil {
			// Exponential moving average for daily quality
			alpha := 2.0 / (float64(successfulToday) + 1.0)
			if successfulToday == 1 {
				dailyMetric.AverageQualityScore = record.Quality.OverallScore
			} else {
				dailyMetric.AverageQualityScore = dailyMetric.AverageQualityScore*(1-alpha) + record.Quality.OverallScore*alpha
			}
		}

		// Exponential moving average for daily latency
		alpha := 2.0 / (float64(successfulToday) + 1.0)
		if successfulToday == 1 {
			dailyMetric.AverageLatency = record.Latency
		} else {
			dailyMetric.AverageLatency = time.Duration(float64(dailyMetric.AverageLatency)*(1-alpha) + float64(record.Latency)*alpha)
		}
	}
}

// categorizeError categorizes an error into a type
func (amc *AIMetricsCollector) categorizeError(errorMsg string) string {
	if errorMsg == "" {
		return "unknown"
	}

	// Simple categorization - in production, this would be more sophisticated
	switch {
	case contains(errorMsg, "timeout", "deadline"):
		return "timeout"
	case contains(errorMsg, "network", "connection"):
		return "network"
	case contains(errorMsg, "parse", "json", "unmarshal"):
		return "parsing"
	case contains(errorMsg, "validation", "invalid"):
		return "validation"
	case contains(errorMsg, "rate limit", "quota"):
		return "rate_limit"
	case contains(errorMsg, "authentication", "unauthorized"):
		return "auth"
	default:
		return "other"
	}
}

// categorizeQuality categorizes a quality score into a level
func (amc *AIMetricsCollector) categorizeQuality(score float64) string {
	switch {
	case score >= 0.9:
		return "excellent"
	case score >= 0.7:
		return "good"
	case score >= 0.5:
		return "fair"
	default:
		return "poor"
	}
}

// checkPerformanceAlerts checks for performance issues and logs alerts
func (amc *AIMetricsCollector) checkPerformanceAlerts(record *AIResponseRecord) {
	// Check success rate alert
	if amc.metrics.TotalRequests >= 10 {
		successRate := float64(amc.metrics.SuccessfulRequests) / float64(amc.metrics.TotalRequests)
		if successRate < amc.config.PerformanceAlertThreshold {
			amc.log.WithFields(logrus.Fields{
				"success_rate":   successRate,
				"threshold":      amc.config.PerformanceAlertThreshold,
				"total_requests": amc.metrics.TotalRequests,
			}).Warn("AI performance alert: Success rate below threshold")
		}
	}

	// Check quality score alert
	if record.Quality != nil && record.Quality.OverallScore < amc.config.PerformanceAlertThreshold {
		amc.log.WithFields(logrus.Fields{
			"quality_score": record.Quality.OverallScore,
			"threshold":     amc.config.PerformanceAlertThreshold,
			"request_id":    record.ID,
		}).Warn("AI performance alert: Quality score below threshold")
	}

	// Check latency alert (if above 95th percentile significantly)
	if len(amc.latencies) >= 20 && record.Latency > amc.metrics.P95Latency*2 {
		amc.log.WithFields(logrus.Fields{
			"latency":     record.Latency,
			"p95_latency": amc.metrics.P95Latency,
			"request_id":  record.ID,
		}).Warn("AI performance alert: High latency detected")
	}
}

// EvaluateResponseQuality evaluates the quality of an AI response
func (amc *AIMetricsCollector) EvaluateResponseQuality(ctx context.Context, response *engine.AIWorkflowResponse, objective *engine.WorkflowObjective) *AIResponseQuality {
	quality := &AIResponseQuality{
		Issues:           make([]QualityIssue, 0),
		Strengths:        make([]string, 0),
		ScoringBreakdown: make(map[string]interface{}),
	}

	// Evaluate completeness
	quality.Completeness = amc.evaluateCompleteness(response, objective)

	// Evaluate accuracy
	quality.Accuracy = amc.evaluateAccuracy(response, objective)

	// Evaluate safety
	quality.Safety = amc.evaluateSafety(response)

	// Evaluate relevance
	quality.Relevance = amc.evaluateRelevance(response, objective)

	// Evaluate clarity
	quality.Clarity = amc.evaluateClarity(response)

	// Evaluate innovation
	quality.Innovation = amc.evaluateInnovation(response)

	// Calculate overall score (weighted average)
	quality.OverallScore = (quality.Completeness*0.25 +
		quality.Accuracy*0.25 +
		quality.Safety*0.20 +
		quality.Relevance*0.15 +
		quality.Clarity*0.10 +
		quality.Innovation*0.05)

	// Store scoring breakdown
	quality.ScoringBreakdown["completeness"] = quality.Completeness
	quality.ScoringBreakdown["accuracy"] = quality.Accuracy
	quality.ScoringBreakdown["safety"] = quality.Safety
	quality.ScoringBreakdown["relevance"] = quality.Relevance
	quality.ScoringBreakdown["clarity"] = quality.Clarity
	quality.ScoringBreakdown["innovation"] = quality.Innovation

	return quality
}

// Helper evaluation methods
func (amc *AIMetricsCollector) evaluateCompleteness(response *engine.AIWorkflowResponse, objective *engine.WorkflowObjective) float64 {
	score := 1.0

	// Check required fields
	if response.WorkflowName == "" {
		score -= 0.2
		amc.addQualityIssue(&AIResponseQuality{}, "missing_workflow_name", "major", "Workflow name is missing", 0.2, "Add a descriptive workflow name")
	}

	if response.Description == "" {
		score -= 0.1
	}

	if len(response.Steps) == 0 {
		score -= 0.5
		amc.addQualityIssue(&AIResponseQuality{}, "no_steps", "critical", "No workflow steps provided", 0.5, "Add at least one workflow step")
	}

	if response.EstimatedTime == "" {
		score -= 0.1
	}

	if response.RiskAssessment == "" {
		score -= 0.1
	}

	return max(0.0, score)
}

func (amc *AIMetricsCollector) evaluateAccuracy(response *engine.AIWorkflowResponse, objective *engine.WorkflowObjective) float64 {
	score := 1.0

	// Check if steps align with objective type
	if objective.Type == "memory_optimization" {
		hasMemoryAction := false
		for _, step := range response.Steps {
			if step.Action != nil && (step.Action.Type == "increase_resources" || step.Action.Type == "scale_deployment") {
				hasMemoryAction = true
				break
			}
		}
		if !hasMemoryAction {
			score -= 0.3
		}
	}

	// Check for logical step ordering
	hasValidation := false
	hasAction := false
	for i, step := range response.Steps {
		if step.Action != nil && step.Action.Type == "validate" {
			hasValidation = true
		}
		if step.Action != nil && step.Action.Type != "validate" {
			hasAction = true
			// Validation should come before action
			if hasAction && !hasValidation && i > 0 {
				score -= 0.1
			}
		}
	}

	return max(0.0, score)
}

func (amc *AIMetricsCollector) evaluateSafety(response *engine.AIWorkflowResponse) float64 {
	score := 1.0

	// Check for safety measures
	hasValidation := false
	hasRollback := false
	hasConfirmation := false

	for _, step := range response.Steps {
		if step.Action != nil {
			switch step.Action.Type {
			case "validate":
				hasValidation = true
			case "rollback", "rollback_deployment":
				hasRollback = true
			}
		}
		if step.Type == "condition" {
			hasConfirmation = true
		}
	}

	if !hasValidation {
		score -= 0.2
	}
	if !hasRollback {
		score -= 0.1
	}
	if !hasConfirmation {
		score -= 0.1
	}

	return max(0.0, score)
}

func (amc *AIMetricsCollector) evaluateRelevance(response *engine.AIWorkflowResponse, objective *engine.WorkflowObjective) float64 {
	score := 1.0

	// Check if workflow addresses the objective
	relevantSteps := 0
	for _, step := range response.Steps {
		if amc.isStepRelevant(step, objective) {
			relevantSteps++
		}
	}

	if len(response.Steps) > 0 {
		relevanceRatio := float64(relevantSteps) / float64(len(response.Steps))
		score = relevanceRatio
	}

	return score
}

func (amc *AIMetricsCollector) evaluateClarity(response *engine.AIWorkflowResponse) float64 {
	score := 1.0

	// Check for clear step names and descriptions
	for _, step := range response.Steps {
		if step.Name == "" {
			score -= 0.1
		}
		if len(step.Name) < 5 {
			score -= 0.05
		}
	}

	// Check for clear workflow description
	if len(response.Description) < 20 {
		score -= 0.1
	}

	return max(0.0, score)
}

func (amc *AIMetricsCollector) evaluateInnovation(response *engine.AIWorkflowResponse) float64 {
	// Simple innovation scoring based on unique step combinations
	// In production, this would compare against historical patterns
	score := 0.5 // Base innovation score

	uniqueActions := make(map[string]bool)
	for _, step := range response.Steps {
		if step.Action != nil {
			uniqueActions[step.Action.Type] = true
		}
	}

	// Bonus for diverse action types
	if len(uniqueActions) > 3 {
		score += 0.3
	} else if len(uniqueActions) > 2 {
		score += 0.2
	}

	// Bonus for parallel steps
	hasParallel := false
	for _, step := range response.Steps {
		if step.Type == "parallel" {
			hasParallel = true
			break
		}
	}
	if hasParallel {
		score += 0.2
	}

	return min(1.0, score)
}

// Helper methods
func (amc *AIMetricsCollector) addQualityIssue(quality *AIResponseQuality, issueType, severity, description string, impact float64, suggestion string) {
	quality.Issues = append(quality.Issues, QualityIssue{
		Type:        issueType,
		Severity:    severity,
		Description: description,
		Impact:      impact,
		Suggestion:  suggestion,
	})
}

func (amc *AIMetricsCollector) isStepRelevant(step *engine.AIGeneratedStep, objective *engine.WorkflowObjective) bool {
	if step.Action == nil {
		return false
	}

	// Simple relevance checking based on objective type
	switch objective.Type {
	case "memory_optimization":
		return step.Action.Type == "increase_resources" || step.Action.Type == "scale_deployment" || step.Action.Type == "validate"
	case "performance_optimization":
		return step.Action.Type == "scale_deployment" || step.Action.Type == "increase_resources" || step.Action.Type == "restart_pod"
	case "cost_optimization":
		return step.Action.Type == "scale_deployment" || step.Action.Type == "decrease_resources" || step.Action.Type == "cleanup_resources"
	default:
		return true // Assume relevant if we don't know the type
	}
}

// Utility functions
func contains(str string, substrings ...string) bool {
	for _, substr := range substrings {
		if len(str) >= len(substr) {
			for i := 0; i <= len(str)-len(substr); i++ {
				if str[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// GetMetrics returns the current performance metrics
func (amc *AIMetricsCollector) GetMetrics() *AIPerformanceMetrics {
	amc.mu.RLock()
	defer amc.mu.RUnlock()

	// Return a copy to prevent concurrent access issues
	metricsCopy := *amc.metrics
	return &metricsCopy
}

// GetDetailedReport generates a comprehensive performance report
func (amc *AIMetricsCollector) GetDetailedReport() map[string]interface{} {
	amc.mu.RLock()
	defer amc.mu.RUnlock()

	report := make(map[string]interface{})

	// Basic metrics
	report["summary"] = map[string]interface{}{
		"total_requests":     amc.metrics.TotalRequests,
		"success_rate":       float64(amc.metrics.SuccessfulRequests) / float64(amc.metrics.TotalRequests),
		"average_quality":    amc.metrics.AverageQualityScore,
		"average_latency":    amc.metrics.AverageLatency.String(),
		"p95_latency":        amc.metrics.P95Latency.String(),
		"pattern_usage_rate": amc.metrics.PatternUsageRate,
		"safety_compliance":  amc.metrics.SafetyComplianceRate,
	}

	// Error analysis
	report["errors"] = map[string]interface{}{
		"by_type":            amc.metrics.ErrorsByType,
		"by_provider":        amc.metrics.ErrorsByProvider,
		"retry_success_rate": amc.metrics.RetrySuccessRate,
	}

	// Quality distribution
	report["quality_distribution"] = amc.metrics.QualityDistribution

	// Recent trends
	report["trends"] = map[string]interface{}{
		"daily_metrics": amc.metrics.DailyMetrics,
		"hourly_trends": amc.metrics.HourlyTrends,
	}

	// Recommendations
	recommendations := amc.generateRecommendations()
	report["recommendations"] = recommendations

	return report
}

// generateRecommendations generates performance improvement recommendations
func (amc *AIMetricsCollector) generateRecommendations() []string {
	recommendations := make([]string, 0)

	// Success rate recommendations
	if amc.metrics.TotalRequests > 0 {
		successRate := float64(amc.metrics.SuccessfulRequests) / float64(amc.metrics.TotalRequests)
		if successRate < 0.9 {
			recommendations = append(recommendations, "Consider reviewing error patterns and improving prompt engineering to increase success rate")
		}
	}

	// Quality recommendations
	if amc.metrics.AverageQualityScore < 0.8 {
		recommendations = append(recommendations, "Quality scores are below optimal - consider prompt optimization or model fine-tuning")
	}

	// Latency recommendations
	if amc.metrics.P95Latency > 30*time.Second {
		recommendations = append(recommendations, "High latency detected - consider optimizing prompts or using faster models")
	}

	// Pattern usage recommendations
	if amc.metrics.PatternUsageRate < 0.3 {
		recommendations = append(recommendations, "Low pattern usage rate - ensure pattern discovery is working effectively")
	}

	// Safety recommendations
	if amc.metrics.SafetyComplianceRate < 0.95 {
		recommendations = append(recommendations, "Safety compliance below target - review safety validation rules")
	}

	return recommendations
}
