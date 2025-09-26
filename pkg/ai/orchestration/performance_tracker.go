package orchestration

import (
	"math"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// **REFACTOR PHASE**: Enhanced performance tracking for intelligent optimization
// Business Requirements: BR-ENSEMBLE-002

// PerformanceMetrics tracks comprehensive model performance data
type PerformanceMetrics struct {
	AccuracyRate       float64          `json:"accuracy_rate"`
	ResponseTime       time.Duration    `json:"response_time"`
	RequestCount       int              `json:"request_count"`
	SuccessCount       int              `json:"success_count"`
	FailureCount       int              `json:"failure_count"`
	AverageConfidence  float64          `json:"average_confidence"`
	CostEfficiency     float64          `json:"cost_efficiency"`
	LastUpdated        time.Time        `json:"last_updated"`
	PerformanceTrend   PerformanceTrend `json:"performance_trend"`
	HistoricalAccuracy []float64        `json:"historical_accuracy"`
}

// PerformanceTrend indicates model performance direction
type PerformanceTrend string

const (
	TrendImproving PerformanceTrend = "improving"
	TrendStable    PerformanceTrend = "stable"
	TrendDeclining PerformanceTrend = "declining"
)

// PerformanceTracker manages model performance monitoring and optimization
type PerformanceTracker struct {
	mu         sync.RWMutex
	logger     *logrus.Logger
	metrics    map[string]*PerformanceMetrics
	weights    map[string]float64
	thresholds PerformanceThresholds
}

// PerformanceThresholds defines performance criteria
type PerformanceThresholds struct {
	MinAccuracy     float64       `json:"min_accuracy"`
	MaxResponseTime time.Duration `json:"max_response_time"`
	MinSuccessRate  float64       `json:"min_success_rate"`
}

// NewPerformanceTracker creates a new performance tracker
func NewPerformanceTracker(logger *logrus.Logger) *PerformanceTracker {
	return &PerformanceTracker{
		logger:  logger,
		metrics: make(map[string]*PerformanceMetrics),
		weights: make(map[string]float64),
		thresholds: PerformanceThresholds{
			MinAccuracy:     0.7,
			MaxResponseTime: 5 * time.Second,
			MinSuccessRate:  0.8,
		},
	}
}

// RecordResponse records a model response for performance tracking
// BR-ENSEMBLE-002: Performance data collection and analysis
func (pt *PerformanceTracker) RecordResponse(modelID string, response *ModelResponse) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	metrics := pt.getOrCreateMetrics(modelID)

	// Update basic metrics
	metrics.RequestCount++
	metrics.ResponseTime = (metrics.ResponseTime*time.Duration(metrics.RequestCount-1) + response.ResponseTime) / time.Duration(metrics.RequestCount)
	metrics.AverageConfidence = (metrics.AverageConfidence*float64(metrics.RequestCount-1) + response.Confidence) / float64(metrics.RequestCount)
	metrics.LastUpdated = time.Now()

	// Track historical accuracy for trend analysis
	if len(metrics.HistoricalAccuracy) >= 10 {
		// Keep only last 10 measurements
		metrics.HistoricalAccuracy = metrics.HistoricalAccuracy[1:]
	}
	metrics.HistoricalAccuracy = append(metrics.HistoricalAccuracy, response.Confidence)

	// Update performance trend
	metrics.PerformanceTrend = pt.calculatePerformanceTrend(metrics.HistoricalAccuracy)

	// Recalculate model weight based on performance
	pt.updateModelWeight(modelID, metrics)

	pt.logger.WithFields(logrus.Fields{
		"model_id":          modelID,
		"accuracy_rate":     metrics.AccuracyRate,
		"response_time":     metrics.ResponseTime,
		"request_count":     metrics.RequestCount,
		"performance_trend": metrics.PerformanceTrend,
	}).Debug("BR-ENSEMBLE-002: Model performance updated")
}

// RecordAccuracy records accuracy measurement for a model
func (pt *PerformanceTracker) RecordAccuracy(modelID string, accuracy float64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	metrics := pt.getOrCreateMetrics(modelID)
	metrics.AccuracyRate = accuracy
	metrics.LastUpdated = time.Now()

	// Update weight based on new accuracy
	pt.updateModelWeight(modelID, metrics)
}

// RecordSuccess records a successful model operation
func (pt *PerformanceTracker) RecordSuccess(modelID string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	metrics := pt.getOrCreateMetrics(modelID)
	metrics.SuccessCount++
	metrics.LastUpdated = time.Now()

	// Recalculate accuracy rate
	totalOperations := metrics.SuccessCount + metrics.FailureCount
	if totalOperations > 0 {
		metrics.AccuracyRate = float64(metrics.SuccessCount) / float64(totalOperations)
	}

	pt.updateModelWeight(modelID, metrics)
}

// RecordFailure records a failed model operation
func (pt *PerformanceTracker) RecordFailure(modelID string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	metrics := pt.getOrCreateMetrics(modelID)
	metrics.FailureCount++
	metrics.LastUpdated = time.Now()

	// Recalculate accuracy rate
	totalOperations := metrics.SuccessCount + metrics.FailureCount
	if totalOperations > 0 {
		metrics.AccuracyRate = float64(metrics.SuccessCount) / float64(totalOperations)
	}

	pt.updateModelWeight(modelID, metrics)

	pt.logger.WithFields(logrus.Fields{
		"model_id":      modelID,
		"failure_count": metrics.FailureCount,
		"accuracy_rate": metrics.AccuracyRate,
	}).Warn("BR-ENSEMBLE-002: Model failure recorded")
}

// GetModelPerformance returns performance metrics for a model
func (pt *PerformanceTracker) GetModelPerformance(modelID string) PerformanceMetrics {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	if metrics, exists := pt.metrics[modelID]; exists {
		return *metrics
	}

	// Return default metrics for unknown models
	return PerformanceMetrics{
		AccuracyRate:      0.5,
		ResponseTime:      1 * time.Second,
		RequestCount:      0,
		AverageConfidence: 0.5,
		PerformanceTrend:  TrendStable,
		LastUpdated:       time.Now(),
	}
}

// GetModelWeight returns the current weight for a model
func (pt *PerformanceTracker) GetModelWeight(modelID string) float64 {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	if weight, exists := pt.weights[modelID]; exists {
		return weight
	}
	return 0.5 // Default weight for unknown models
}

// GetAllPerformanceMetrics returns performance metrics for all models
func (pt *PerformanceTracker) GetAllPerformanceMetrics() map[string]PerformanceMetrics {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	result := make(map[string]PerformanceMetrics)
	for modelID, metrics := range pt.metrics {
		result[modelID] = *metrics
	}
	return result
}

// OptimizeWeights recalculates all model weights based on current performance
// BR-ENSEMBLE-002: Automatic weight optimization
func (pt *PerformanceTracker) OptimizeWeights() {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	for modelID, metrics := range pt.metrics {
		pt.updateModelWeight(modelID, metrics)
	}

	pt.logger.Info("BR-ENSEMBLE-002: Model weights optimized based on performance")
}

// IsModelPerforming checks if a model meets performance thresholds
func (pt *PerformanceTracker) IsModelPerforming(modelID string) bool {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	metrics, exists := pt.metrics[modelID]
	if !exists {
		return true // Give benefit of doubt to new models
	}

	return metrics.AccuracyRate >= pt.thresholds.MinAccuracy &&
		metrics.ResponseTime <= pt.thresholds.MaxResponseTime &&
		pt.getSuccessRate(metrics) >= pt.thresholds.MinSuccessRate
}

// Private helper methods

func (pt *PerformanceTracker) getOrCreateMetrics(modelID string) *PerformanceMetrics {
	if metrics, exists := pt.metrics[modelID]; exists {
		return metrics
	}

	metrics := &PerformanceMetrics{
		AccuracyRate:       0.5,
		ResponseTime:       1 * time.Second,
		RequestCount:       0,
		AverageConfidence:  0.5,
		PerformanceTrend:   TrendStable,
		LastUpdated:        time.Now(),
		HistoricalAccuracy: make([]float64, 0, 10),
	}
	pt.metrics[modelID] = metrics
	pt.weights[modelID] = 0.5 // Default weight
	return metrics
}

func (pt *PerformanceTracker) updateModelWeight(modelID string, metrics *PerformanceMetrics) {
	// Calculate weight based on multiple factors
	accuracyWeight := metrics.AccuracyRate

	// Response time factor (faster = better)
	responseTimeFactor := 1.0
	if metrics.ResponseTime > 0 {
		responseTimeFactor = math.Min(1.0, 1.0/(metrics.ResponseTime.Seconds()/1.0))
	}

	// Success rate factor
	successRate := pt.getSuccessRate(metrics)

	// Trend factor
	trendFactor := 1.0
	switch metrics.PerformanceTrend {
	case TrendImproving:
		trendFactor = 1.1
	case TrendDeclining:
		trendFactor = 0.9
	}

	// Combined weight calculation
	weight := (accuracyWeight*0.5 + responseTimeFactor*0.2 + successRate*0.2 + 0.1) * trendFactor
	weight = math.Max(0.1, math.Min(1.0, weight)) // Clamp between 0.1 and 1.0

	pt.weights[modelID] = weight
}

func (pt *PerformanceTracker) calculatePerformanceTrend(history []float64) PerformanceTrend {
	if len(history) < 3 {
		return TrendStable
	}

	// Calculate trend using linear regression slope
	n := float64(len(history))
	sumX, sumY, sumXY, sumX2 := 0.0, 0.0, 0.0, 0.0

	for i, y := range history {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	// Calculate slope
	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)

	if slope > 0.05 {
		return TrendImproving
	} else if slope < -0.05 {
		return TrendDeclining
	}
	return TrendStable
}

func (pt *PerformanceTracker) getSuccessRate(metrics *PerformanceMetrics) float64 {
	totalOperations := metrics.SuccessCount + metrics.FailureCount
	if totalOperations == 0 {
		return 1.0 // No failures yet
	}
	return float64(metrics.SuccessCount) / float64(totalOperations)
}
