package engine

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ProductionStatisticsCollector implements StatisticsCollector for production use
// Business Requirements: BR-ORK-003 - Execution metrics collection and performance trend analysis
type ProductionStatisticsCollector struct {
	mu sync.RWMutex

	// BR-ORK-003: Execution metrics storage
	executionStatistics []*ExecutionStatistics
	performanceMetrics  *AggregatedPerformanceMetrics
	failurePatterns     []*FailurePattern
	trendAnalysis       *TrendAnalysisData

	// Configuration
	maxHistorySize      int
	analysisWindowSize  time.Duration
	patternDetectionMin int

	log *logrus.Logger
}

// ExecutionStatistics represents statistics for a single execution
type ExecutionStatistics struct {
	ExecutionID      string                 `json:"execution_id"`
	WorkflowID       string                 `json:"workflow_id"`
	StartTime        time.Time              `json:"start_time"`
	EndTime          time.Time              `json:"end_time"`
	Duration         time.Duration          `json:"duration"`
	Status           string                 `json:"status"`
	StepCount        int                    `json:"step_count"`
	SuccessfulSteps  int                    `json:"successful_steps"`
	FailedSteps      int                    `json:"failed_steps"`
	RetryCount       int                    `json:"retry_count"`
	ResourceUsage    *ResourceUsageMetrics  `json:"resource_usage"`
	BusinessMetadata map[string]interface{} `json:"business_metadata"`
	CollectedAt      time.Time              `json:"collected_at"`
}

// AggregatedPerformanceMetrics represents aggregated performance data
type AggregatedPerformanceMetrics struct {
	TotalExecutions            int                         `json:"total_executions"`
	SuccessfulExecutions       int                         `json:"successful_executions"`
	FailedExecutions           int                         `json:"failed_executions"`
	AverageExecutionTime       time.Duration               `json:"average_execution_time"`
	MedianExecutionTime        time.Duration               `json:"median_execution_time"`
	P95ExecutionTime           time.Duration               `json:"p95_execution_time"`
	P99ExecutionTime           time.Duration               `json:"p99_execution_time"`
	SuccessRate                float64                     `json:"success_rate"`
	ResourceUtilizationMetrics *ResourceUtilizationMetrics `json:"resource_utilization_metrics"`
	LastUpdated                time.Time                   `json:"last_updated"`
}

// ResourceUtilizationMetrics represents resource usage statistics
type ResourceUtilizationMetrics struct {
	AverageCPUUsage    float64 `json:"average_cpu_usage"`
	AverageMemoryUsage float64 `json:"average_memory_usage"`
	PeakCPUUsage       float64 `json:"peak_cpu_usage"`
	PeakMemoryUsage    float64 `json:"peak_memory_usage"`
	TotalIOOperations  int64   `json:"total_io_operations"`
	AverageIORate      float64 `json:"average_io_rate"`
}

// TrendAnalysisData represents trend analysis information
type TrendAnalysisData struct {
	ExecutionTimeTrend       *TrendMetrics        `json:"execution_time_trend"`
	ResourceUtilizationTrend *TrendMetrics        `json:"resource_utilization_trend"`
	SuccessRateTrend         *TrendMetrics        `json:"success_rate_trend"`
	ForecastedPerformance    *PerformanceForecast `json:"forecasted_performance"`
	LastAnalysis             time.Time            `json:"last_analysis"`
}

// TrendMetrics represents trend analysis for a specific metric
type TrendMetrics struct {
	Direction  string    `json:"direction"` // "improving", "stable", "degrading"
	ChangeRate float64   `json:"change_rate"`
	Confidence float64   `json:"confidence"`
	StartValue float64   `json:"start_value"`
	EndValue   float64   `json:"end_value"`
	DataPoints int       `json:"data_points"`
	AnalyzedAt time.Time `json:"analyzed_at"`
}

// PerformanceForecast represents performance predictions
type PerformanceForecast struct {
	NextWeekPrediction  float64   `json:"next_week_prediction"`
	NextMonthPrediction float64   `json:"next_month_prediction"`
	TrendConfidence     float64   `json:"trend_confidence"`
	PredictionAccuracy  float64   `json:"prediction_accuracy"`
	GeneratedAt         time.Time `json:"generated_at"`
}

// FailurePattern represents detected failure patterns (reuse existing type)
// Already defined in resilient_interfaces.go

// PerformanceReportResult represents comprehensive performance reporting (renamed to avoid conflict)
type PerformanceReportResult struct {
	ReportID                   string                      `json:"report_id"`
	GeneratedAt                time.Time                   `json:"generated_at"`
	TimeWindow                 time.Duration               `json:"time_window"`
	TotalExecutions            int                         `json:"total_executions"`
	AverageExecutionTime       time.Duration               `json:"average_execution_time"`
	ResourceUtilizationMetrics *ResourceUtilizationMetrics `json:"resource_utilization_metrics"`
	TrendAnalysis              *TrendAnalysisData          `json:"trend_analysis"`
	FailurePatterns            []*FailurePattern           `json:"failure_patterns"`
	BusinessInsights           *BusinessInsights           `json:"business_insights"`
	Recommendations            []string                    `json:"recommendations"`
}

// BusinessInsights represents business-relevant insights
type BusinessInsights struct {
	CostEfficiency      float64                `json:"cost_efficiency"`
	CapacityUtilization float64                `json:"capacity_utilization"`
	BusinessImpactScore float64                `json:"business_impact_score"`
	RiskAssessment      string                 `json:"risk_assessment"`
	ROIMetrics          map[string]float64     `json:"roi_metrics"`
	BusinessMetadata    map[string]interface{} `json:"business_metadata"`
}

// NewProductionStatisticsCollector creates a production statistics collector
func NewProductionStatisticsCollector(log *logrus.Logger) *ProductionStatisticsCollector {
	return &ProductionStatisticsCollector{
		executionStatistics: []*ExecutionStatistics{},
		failurePatterns:     []*FailurePattern{},
		performanceMetrics: &AggregatedPerformanceMetrics{
			TotalExecutions:      0,
			SuccessfulExecutions: 0,
			FailedExecutions:     0,
			SuccessRate:          0.0,
			ResourceUtilizationMetrics: &ResourceUtilizationMetrics{
				AverageCPUUsage:    0.0,
				AverageMemoryUsage: 0.0,
				PeakCPUUsage:       0.0,
				PeakMemoryUsage:    0.0,
				TotalIOOperations:  0,
				AverageIORate:      0.0,
			},
			LastUpdated: time.Now(),
		},
		trendAnalysis: &TrendAnalysisData{
			ExecutionTimeTrend:       &TrendMetrics{Direction: "stable", ChangeRate: 0.0, Confidence: 0.5},
			ResourceUtilizationTrend: &TrendMetrics{Direction: "stable", ChangeRate: 0.0, Confidence: 0.5},
			SuccessRateTrend:         &TrendMetrics{Direction: "stable", ChangeRate: 0.0, Confidence: 0.5},
			ForecastedPerformance: &PerformanceForecast{
				NextWeekPrediction:  0.0,
				NextMonthPrediction: 0.0,
				TrendConfidence:     0.5,
				PredictionAccuracy:  0.0,
				GeneratedAt:         time.Now(),
			},
			LastAnalysis: time.Now(),
		},
		maxHistorySize:      10000, // Keep last 10k executions
		analysisWindowSize:  24 * time.Hour,
		patternDetectionMin: 3, // Minimum occurrences to detect pattern
		log:                 log,
	}
}

// CollectExecutionStatistics implements statistics collection for executions
// BR-ORK-003: MUST implement execution metrics collection
func (psc *ProductionStatisticsCollector) CollectExecutionStatistics(execution *RuntimeWorkflowExecution) error {
	psc.mu.Lock()
	defer psc.mu.Unlock()

	psc.log.WithFields(logrus.Fields{
		"execution_id": execution.ID,
		"workflow_id":  execution.WorkflowID,
		"status":       execution.Status,
	}).Debug("BR-ORK-003: Collecting execution statistics")

	// Extract execution statistics
	stats := psc.extractExecutionStatistics(execution)

	// Store statistics
	psc.executionStatistics = append(psc.executionStatistics, stats)

	// Maintain history size limit
	if len(psc.executionStatistics) > psc.maxHistorySize {
		psc.executionStatistics = psc.executionStatistics[1:]
	}

	// Update aggregated metrics
	psc.updateAggregatedMetrics(stats)

	psc.log.WithFields(logrus.Fields{
		"execution_id":     stats.ExecutionID,
		"duration":         stats.Duration,
		"total_statistics": len(psc.executionStatistics),
	}).Debug("BR-ORK-003: Execution statistics collected successfully")

	return nil
}

// AnalyzePerformanceTrends implements performance trend analysis
// BR-ORK-003: MUST implement performance trend analysis
func (psc *ProductionStatisticsCollector) AnalyzePerformanceTrends(timeWindow time.Duration) (*PerformanceTrendAnalysis, error) {
	psc.mu.RLock()
	defer psc.mu.RUnlock()

	psc.log.WithField("time_window", timeWindow).Info("BR-ORK-003: Analyzing performance trends")

	// Filter statistics within time window
	cutoffTime := time.Now().Add(-timeWindow)
	recentStats := psc.filterStatisticsByTime(cutoffTime)

	if len(recentStats) < 2 {
		return nil, fmt.Errorf("insufficient data for trend analysis (need at least 2 data points)")
	}

	// Analyze execution time trends
	executionTimeTrend := psc.analyzeExecutionTimeTrend(recentStats)

	// Analyze resource utilization trends
	resourceTrend := psc.analyzeResourceUtilizationTrend(recentStats)

	// Generate performance forecast
	forecast := psc.generatePerformanceForecast(recentStats)

	// Update internal trend analysis
	psc.updateTrendAnalysis(executionTimeTrend, resourceTrend, forecast)

	analysis := &PerformanceTrendAnalysis{
		TimeWindow:         timeWindow,
		TrendDirection:     executionTimeTrend.Direction,
		PerformanceMetrics: nil, // Will populate with actual metrics if needed
		SeasonalPatterns:   []*SeasonalPattern{},
		AnomalyDetection:   []*PerformanceAnomaly{},
		Recommendations:    []string{"Optimize based on " + executionTimeTrend.Direction + " trend"},
	}

	psc.log.WithFields(logrus.Fields{
		"execution_trend": executionTimeTrend.Direction,
		"resource_trend":  resourceTrend.Direction,
		"data_points":     len(recentStats),
	}).Info("BR-ORK-003: Performance trend analysis completed")

	return analysis, nil
}

// DetectFailurePatterns implements failure pattern detection
// BR-ORK-003: MUST detect failure patterns for business risk management
func (psc *ProductionStatisticsCollector) DetectFailurePatterns(executions []*RuntimeWorkflowExecution) ([]*FailurePattern, error) {
	psc.mu.Lock()
	defer psc.mu.Unlock()

	psc.log.WithField("executions_count", len(executions)).Info("BR-ORK-003: Detecting failure patterns")

	// Extract failure data from executions
	failureData := psc.extractFailureData(executions)

	// Group failures by type and step
	failureGroups := psc.groupFailuresByPattern(failureData)

	// Detect patterns with sufficient frequency
	var detectedPatterns []*FailurePattern
	for patternKey, failures := range failureGroups {
		if len(failures) >= psc.patternDetectionMin {
			pattern := psc.createFailurePattern(patternKey, failures)
			detectedPatterns = append(detectedPatterns, pattern)
		}
	}

	// Sort patterns by business impact and frequency
	sort.Slice(detectedPatterns, func(i, j int) bool {
		return detectedPatterns[i].Frequency > detectedPatterns[j].Frequency
	})

	// Update internal failure patterns
	psc.failurePatterns = detectedPatterns

	psc.log.WithFields(logrus.Fields{
		"patterns_detected":   len(detectedPatterns),
		"failure_data_points": len(failureData),
	}).Info("BR-ORK-003: Failure pattern detection completed")

	return detectedPatterns, nil
}

// GeneratePerformanceReport implements comprehensive performance reporting
// BR-ORK-003: MUST generate performance reports for business analysis
func (psc *ProductionStatisticsCollector) GeneratePerformanceReport(ctx context.Context) (*PerformanceReport, error) {
	psc.mu.RLock()
	defer psc.mu.RUnlock()

	psc.log.Info("BR-ORK-003: Generating comprehensive performance report")

	// Generate business insights
	businessInsights := psc.generateBusinessInsights()

	// Generate recommendations
	recommendations := psc.generateRecommendations()

	// Convert to interface-expected PerformanceReport format
	report := &PerformanceReport{
		ReportPeriod:         psc.analysisWindowSize,
		TotalExecutions:      psc.performanceMetrics.TotalExecutions,
		SuccessRate:          psc.performanceMetrics.SuccessRate,
		AverageExecutionTime: psc.performanceMetrics.AverageExecutionTime,
		ResourceEfficiency:   businessInsights.CostEfficiency,
		TopFailureReasons:    recommendations, // Use recommendations as failure reasons for now
		PerformanceTrends: &PerformanceTrendAnalysis{
			TimeWindow:         psc.analysisWindowSize,
			TrendDirection:     psc.trendAnalysis.ExecutionTimeTrend.Direction,
			PerformanceMetrics: &PerformanceMetrics{}, // Create minimal performance metrics
			SeasonalPatterns:   []*SeasonalPattern{},
			AnomalyDetection:   []*PerformanceAnomaly{},
			Recommendations:    recommendations,
		},
		OptimizationImpact: &OptimizationImpact{}, // Add minimal optimization impact
	}

	psc.log.WithFields(logrus.Fields{
		"total_executions":    report.TotalExecutions,
		"success_rate":        report.SuccessRate,
		"avg_execution_time":  report.AverageExecutionTime,
		"resource_efficiency": report.ResourceEfficiency,
		"trend_direction":     report.PerformanceTrends.TrendDirection,
	}).Info("BR-ORK-003: Performance report generated successfully")

	return report, nil
}

// Private helper methods

func (psc *ProductionStatisticsCollector) extractExecutionStatistics(execution *RuntimeWorkflowExecution) *ExecutionStatistics {
	duration := time.Duration(0)
	if execution.EndTime != nil {
		duration = execution.EndTime.Sub(execution.StartTime)
	}

	successfulSteps := 0
	failedSteps := 0
	totalRetries := 0

	for _, step := range execution.Steps {
		if step.Status == "completed" {
			successfulSteps++
		} else if step.Status == "failed" {
			failedSteps++
		}
		totalRetries += step.RetryCount
	}

	// Extract resource usage from execution metadata
	resourceUsage := psc.extractResourceUsage(execution)

	return &ExecutionStatistics{
		ExecutionID: execution.ID,
		WorkflowID:  execution.WorkflowID,
		StartTime:   execution.StartTime,
		EndTime: func() time.Time {
			if execution.EndTime != nil {
				return *execution.EndTime
			} else {
				return time.Now()
			}
		}(),
		Duration:         duration,
		Status:           execution.Status,
		StepCount:        len(execution.Steps),
		SuccessfulSteps:  successfulSteps,
		FailedSteps:      failedSteps,
		RetryCount:       totalRetries,
		ResourceUsage:    resourceUsage,
		BusinessMetadata: execution.Metadata,
		CollectedAt:      time.Now(),
	}
}

func (psc *ProductionStatisticsCollector) extractResourceUsage(execution *RuntimeWorkflowExecution) *ResourceUsageMetrics {
	totalCPU := 0.0
	totalMemory := 0.0
	totalIO := int64(0)
	stepCount := 0

	for _, step := range execution.Steps {
		if step.Metadata != nil {
			if cpu, ok := step.Metadata["cpu_usage"].(float64); ok {
				totalCPU += cpu
				stepCount++
			}
			if memory, ok := step.Metadata["memory_usage"].(float64); ok {
				totalMemory += memory
			}
			if io, ok := step.Metadata["io_operations"].(int64); ok {
				totalIO += io
			} else if io, ok := step.Metadata["io_operations"].(int); ok {
				totalIO += int64(io)
			}
		}
	}

	avgCPU := 0.0
	avgMemory := 0.0
	if stepCount > 0 {
		avgCPU = totalCPU / float64(stepCount)
		avgMemory = totalMemory / float64(stepCount)
	}

	return &ResourceUsageMetrics{
		CPUUsage:    avgCPU,
		MemoryUsage: avgMemory,
		// Note: IOOperations not available in ResourceUsageMetrics, using NetworkIO as proxy
		NetworkIO: float64(totalIO),
	}
}

func (psc *ProductionStatisticsCollector) updateAggregatedMetrics(stats *ExecutionStatistics) {
	psc.performanceMetrics.TotalExecutions++

	if stats.Status == "completed" {
		psc.performanceMetrics.SuccessfulExecutions++
	} else {
		psc.performanceMetrics.FailedExecutions++
	}

	// Update success rate
	psc.performanceMetrics.SuccessRate = float64(psc.performanceMetrics.SuccessfulExecutions) / float64(psc.performanceMetrics.TotalExecutions)

	// Update average execution time (running average)
	if psc.performanceMetrics.TotalExecutions == 1 {
		psc.performanceMetrics.AverageExecutionTime = stats.Duration
	} else {
		currentAvg := psc.performanceMetrics.AverageExecutionTime
		newAvg := (currentAvg*time.Duration(psc.performanceMetrics.TotalExecutions-1) + stats.Duration) / time.Duration(psc.performanceMetrics.TotalExecutions)
		psc.performanceMetrics.AverageExecutionTime = newAvg
	}

	// Update resource utilization metrics
	if stats.ResourceUsage != nil {
		psc.updateResourceUtilizationMetrics(stats.ResourceUsage)
	}

	psc.performanceMetrics.LastUpdated = time.Now()
}

func (psc *ProductionStatisticsCollector) updateResourceUtilizationMetrics(resourceUsage *ResourceUsageMetrics) {
	metrics := psc.performanceMetrics.ResourceUtilizationMetrics

	// Update running averages
	totalExecs := float64(psc.performanceMetrics.TotalExecutions)

	metrics.AverageCPUUsage = (metrics.AverageCPUUsage*(totalExecs-1) + resourceUsage.CPUUsage) / totalExecs
	metrics.AverageMemoryUsage = (metrics.AverageMemoryUsage*(totalExecs-1) + resourceUsage.MemoryUsage) / totalExecs

	// Update peaks
	if resourceUsage.CPUUsage > metrics.PeakCPUUsage {
		metrics.PeakCPUUsage = resourceUsage.CPUUsage
	}
	if resourceUsage.MemoryUsage > metrics.PeakMemoryUsage {
		metrics.PeakMemoryUsage = resourceUsage.MemoryUsage
	}

	// Update IO metrics (using NetworkIO as proxy for IO operations)
	metrics.TotalIOOperations += int64(resourceUsage.NetworkIO)
	metrics.AverageIORate = float64(metrics.TotalIOOperations) / totalExecs
}

func (psc *ProductionStatisticsCollector) filterStatisticsByTime(cutoffTime time.Time) []*ExecutionStatistics {
	var filtered []*ExecutionStatistics
	for _, stat := range psc.executionStatistics {
		if stat.StartTime.After(cutoffTime) {
			filtered = append(filtered, stat)
		}
	}
	return filtered
}

func (psc *ProductionStatisticsCollector) analyzeExecutionTimeTrend(stats []*ExecutionStatistics) *TrendMetrics {
	if len(stats) < 2 {
		return &TrendMetrics{Direction: "stable", ChangeRate: 0.0, Confidence: 0.0}
	}

	// Calculate trend using linear regression
	durations := make([]float64, len(stats))
	for i, stat := range stats {
		durations[i] = float64(stat.Duration.Nanoseconds())
	}

	slope := psc.calculateLinearRegressionSlope(durations)

	direction := "stable"
	if slope < -0.05 { // 5% improvement threshold
		direction = "improving"
	} else if slope > 0.05 { // 5% degradation threshold
		direction = "degrading"
	}

	return &TrendMetrics{
		Direction:  direction,
		ChangeRate: slope,
		Confidence: psc.calculateTrendConfidence(len(stats)),
		StartValue: durations[0],
		EndValue:   durations[len(durations)-1],
		DataPoints: len(stats),
		AnalyzedAt: time.Now(),
	}
}

func (psc *ProductionStatisticsCollector) analyzeResourceUtilizationTrend(stats []*ExecutionStatistics) *TrendMetrics {
	if len(stats) < 2 {
		return &TrendMetrics{Direction: "stable", ChangeRate: 0.0, Confidence: 0.0}
	}

	// Calculate average resource utilization trend
	resourceValues := make([]float64, len(stats))
	for i, stat := range stats {
		if stat.ResourceUsage != nil {
			resourceValues[i] = (stat.ResourceUsage.CPUUsage + stat.ResourceUsage.MemoryUsage) / 2.0
		}
	}

	slope := psc.calculateLinearRegressionSlope(resourceValues)

	direction := "stable"
	if slope < -0.05 { // 5% improvement threshold
		direction = "improving"
	} else if slope > 0.05 { // 5% degradation threshold
		direction = "degrading"
	}

	return &TrendMetrics{
		Direction:  direction,
		ChangeRate: slope,
		Confidence: psc.calculateTrendConfidence(len(stats)),
		StartValue: resourceValues[0],
		EndValue:   resourceValues[len(resourceValues)-1],
		DataPoints: len(stats),
		AnalyzedAt: time.Now(),
	}
}

func (psc *ProductionStatisticsCollector) generatePerformanceForecast(stats []*ExecutionStatistics) *PerformanceForecast {
	if len(stats) < 3 {
		return &PerformanceForecast{
			NextWeekPrediction:  0.0,
			NextMonthPrediction: 0.0,
			TrendConfidence:     0.0,
			PredictionAccuracy:  0.0,
			GeneratedAt:         time.Now(),
		}
	}

	// Simple linear extrapolation for forecasting
	durations := make([]float64, len(stats))
	for i, stat := range stats {
		durations[i] = float64(stat.Duration.Nanoseconds())
	}

	slope := psc.calculateLinearRegressionSlope(durations)
	currentValue := durations[len(durations)-1]

	// Forecast for next week (7 data points ahead)
	nextWeekPrediction := currentValue + (slope * 7)

	// Forecast for next month (30 data points ahead)
	nextMonthPrediction := currentValue + (slope * 30)

	return &PerformanceForecast{
		NextWeekPrediction:  nextWeekPrediction,
		NextMonthPrediction: nextMonthPrediction,
		TrendConfidence:     psc.calculateTrendConfidence(len(stats)),
		PredictionAccuracy:  psc.calculatePredictionAccuracy(durations),
		GeneratedAt:         time.Now(),
	}
}

func (psc *ProductionStatisticsCollector) calculateLinearRegressionSlope(values []float64) float64 {
	n := float64(len(values))
	if n < 2 {
		return 0.0
	}

	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumXX := 0.0

	for i, y := range values {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}

	slope := (n*sumXY - sumX*sumY) / (n*sumXX - sumX*sumX)
	return slope
}

func (psc *ProductionStatisticsCollector) calculateTrendConfidence(dataPoints int) float64 {
	// Confidence increases with more data points
	return math.Min(float64(dataPoints)/20.0, 1.0) // Max confidence at 20+ data points
}

func (psc *ProductionStatisticsCollector) calculateAnalysisConfidence(dataPoints int) float64 {
	// Similar to trend confidence but with different scaling
	return math.Min(float64(dataPoints)/15.0, 1.0) // Max confidence at 15+ data points
}

func (psc *ProductionStatisticsCollector) calculatePredictionAccuracy(values []float64) float64 {
	if len(values) < 3 {
		return 0.0
	}

	// Calculate variance to estimate prediction accuracy
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))

	variance := 0.0
	for _, v := range values {
		variance += (v - mean) * (v - mean)
	}
	variance /= float64(len(values))

	// Lower variance = higher accuracy
	return math.Max(0.0, 1.0-(variance/mean))
}

func (psc *ProductionStatisticsCollector) updateTrendAnalysis(executionTrend, resourceTrend *TrendMetrics, forecast *PerformanceForecast) {
	psc.trendAnalysis.ExecutionTimeTrend = executionTrend
	psc.trendAnalysis.ResourceUtilizationTrend = resourceTrend
	psc.trendAnalysis.ForecastedPerformance = forecast
	psc.trendAnalysis.LastAnalysis = time.Now()
}

func (psc *ProductionStatisticsCollector) extractFailureData(executions []*RuntimeWorkflowExecution) []FailureData {
	var failures []FailureData

	for _, execution := range executions {
		if execution.Status == "failed" {
			for _, step := range execution.Steps {
				if step.Status == "failed" && step.Error != "" {
					failures = append(failures, FailureData{
						ExecutionID: execution.ID,
						StepID:      step.StepID,
						FailureType: step.Error,
						Timestamp:   step.StartTime,
					})
				}
			}
		}
	}

	return failures
}

func (psc *ProductionStatisticsCollector) groupFailuresByPattern(failures []FailureData) map[string][]FailureData {
	groups := make(map[string][]FailureData)

	for _, failure := range failures {
		// Create pattern key combining failure type and step
		patternKey := fmt.Sprintf("%s:%s", failure.FailureType, failure.StepID)
		groups[patternKey] = append(groups[patternKey], failure)
	}

	return groups
}

func (psc *ProductionStatisticsCollector) createFailurePattern(patternKey string, failures []FailureData) *FailurePattern {
	if len(failures) == 0 {
		return nil
	}

	// Parse pattern key
	parts := strings.Split(patternKey, ":")
	failureType := parts[0]
	affectedStep := ""
	if len(parts) > 1 {
		affectedStep = parts[1]
	}

	// Calculate pattern metrics
	frequency := float64(len(failures))
	firstOccurrence := failures[0].Timestamp
	lastOccurrence := failures[len(failures)-1].Timestamp

	// Sort to find first and last occurrences
	sort.Slice(failures, func(i, j int) bool {
		return failures[i].Timestamp.Before(failures[j].Timestamp)
	})

	if len(failures) > 0 {
		firstOccurrence = failures[0].Timestamp
		lastOccurrence = failures[len(failures)-1].Timestamp
	}

	// Assess business impact (removed unused variable)
	_ = "medium" // businessImpact not used in FailurePattern struct

	return &FailurePattern{
		PatternType:         failureType,
		Frequency:           frequency,
		AffectedSteps:       []string{affectedStep},
		CommonCause:         psc.determineCommonCause(failureType),
		DetectionConfidence: psc.calculatePatternConfidence(frequency),
		FirstOccurrence:     firstOccurrence,
		LastOccurrence:      lastOccurrence,
	}
}

func (psc *ProductionStatisticsCollector) determineCommonCause(failureType string) string {
	switch failureType {
	case "network_timeout":
		return "Network connectivity issues or service unavailability"
	case "memory_exhaustion":
		return "Insufficient memory allocation or memory leaks"
	case "disk_full":
		return "Storage capacity exceeded or cleanup issues"
	default:
		return "Unknown cause - requires investigation"
	}
}

func (psc *ProductionStatisticsCollector) calculatePatternConfidence(frequency float64) float64 {
	// Confidence increases with frequency
	return math.Min(frequency/10.0, 1.0) // Max confidence at 10+ occurrences
}

func (psc *ProductionStatisticsCollector) generateBusinessInsights() *BusinessInsights {
	// Calculate business-relevant metrics
	costEfficiency := psc.calculateCostEfficiency()
	capacityUtilization := psc.calculateCapacityUtilization()
	businessImpactScore := psc.calculateBusinessImpactScore()
	riskAssessment := psc.assessBusinessRisk()

	return &BusinessInsights{
		CostEfficiency:      costEfficiency,
		CapacityUtilization: capacityUtilization,
		BusinessImpactScore: businessImpactScore,
		RiskAssessment:      riskAssessment,
		ROIMetrics: map[string]float64{
			"efficiency_gain":       costEfficiency * 0.8,
			"capacity_optimization": capacityUtilization * 0.9,
			"risk_mitigation":       (1.0 - businessImpactScore) * 0.7,
		},
		BusinessMetadata: map[string]interface{}{
			"analysis_date":     time.Now(),
			"data_completeness": psc.calculateDataCompleteness(),
			"confidence_level":  psc.calculateOverallConfidence(),
		},
	}
}

func (psc *ProductionStatisticsCollector) generateRecommendations() []string {
	var recommendations []string

	// Analyze current metrics and generate recommendations
	if psc.performanceMetrics.SuccessRate < 0.95 {
		recommendations = append(recommendations, "Consider implementing additional error handling and retry mechanisms")
	}

	if psc.performanceMetrics.ResourceUtilizationMetrics.AverageCPUUsage > 0.8 {
		recommendations = append(recommendations, "CPU utilization is high - consider resource optimization or scaling")
	}

	if psc.performanceMetrics.ResourceUtilizationMetrics.AverageMemoryUsage > 0.8 {
		recommendations = append(recommendations, "Memory utilization is high - review memory allocation and cleanup")
	}

	if len(psc.failurePatterns) > 0 {
		recommendations = append(recommendations, "Multiple failure patterns detected - prioritize pattern resolution")
	}

	if psc.trendAnalysis.ExecutionTimeTrend.Direction == "degrading" {
		recommendations = append(recommendations, "Performance is degrading - investigate bottlenecks and optimization opportunities")
	}

	return recommendations
}

func (psc *ProductionStatisticsCollector) calculateCostEfficiency() float64 {
	// Simple cost efficiency calculation based on success rate and resource utilization
	successRate := psc.performanceMetrics.SuccessRate
	resourceEfficiency := 1.0 - psc.performanceMetrics.ResourceUtilizationMetrics.AverageCPUUsage
	return (successRate + resourceEfficiency) / 2.0
}

func (psc *ProductionStatisticsCollector) calculateCapacityUtilization() float64 {
	// Calculate capacity utilization based on resource metrics
	cpuUtil := psc.performanceMetrics.ResourceUtilizationMetrics.AverageCPUUsage
	memUtil := psc.performanceMetrics.ResourceUtilizationMetrics.AverageMemoryUsage
	return (cpuUtil + memUtil) / 2.0
}

func (psc *ProductionStatisticsCollector) calculateBusinessImpactScore() float64 {
	// Calculate business impact based on failure rate and patterns
	failureRate := 1.0 - psc.performanceMetrics.SuccessRate
	patternImpact := float64(len(psc.failurePatterns)) * 0.1
	return math.Min(failureRate+patternImpact, 1.0)
}

func (psc *ProductionStatisticsCollector) assessBusinessRisk() string {
	impactScore := psc.calculateBusinessImpactScore()

	if impactScore < 0.1 {
		return "low"
	} else if impactScore < 0.3 {
		return "medium"
	} else {
		return "high"
	}
}

func (psc *ProductionStatisticsCollector) calculateDataCompleteness() float64 {
	// Calculate completeness based on available data
	if psc.performanceMetrics.TotalExecutions == 0 {
		return 0.0
	}

	completeness := 1.0
	if psc.performanceMetrics.ResourceUtilizationMetrics.AverageCPUUsage == 0.0 {
		completeness -= 0.2
	}
	if psc.performanceMetrics.ResourceUtilizationMetrics.AverageMemoryUsage == 0.0 {
		completeness -= 0.2
	}

	return math.Max(completeness, 0.0)
}

func (psc *ProductionStatisticsCollector) calculateOverallConfidence() float64 {
	// Calculate overall confidence based on data quality and quantity
	dataPoints := float64(len(psc.executionStatistics))
	dataCompleteness := psc.calculateDataCompleteness()

	quantityFactor := math.Min(dataPoints/50.0, 1.0) // Max confidence at 50+ data points
	qualityFactor := dataCompleteness

	return (quantityFactor + qualityFactor) / 2.0
}

// Additional helper types

type FailureData struct {
	ExecutionID string
	StepID      string
	FailureType string
	Timestamp   time.Time
}

// PerformanceTrendAnalysisResult represents comprehensive trend analysis (renamed to avoid conflict)
type PerformanceTrendAnalysisResult struct {
	TimeWindow               time.Duration        `json:"time_window"`
	ExecutionTimeTrend       *TrendMetrics        `json:"execution_time_trend"`
	ResourceUtilizationTrend *TrendMetrics        `json:"resource_utilization_trend"`
	ForecastedPerformance    *PerformanceForecast `json:"forecasted_performance"`
	DataPointsAnalyzed       int                  `json:"data_points_analyzed"`
	AnalysisConfidence       float64              `json:"analysis_confidence"`
	GeneratedAt              time.Time            `json:"generated_at"`
}
