package context

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
)

// PerformanceMonitor implements BR-CONTEXT-039 to BR-CONTEXT-043
// LLM performance monitoring and automatic adjustment
type PerformanceMonitor struct {
	config            *config.ContextOptimizationConfig
	metrics           *PerformanceMetrics
	baselines         map[string]*PerformanceBaseline
	logger            *logrus.Logger
	adjustmentHistory []AdjustmentRecord
	mu                sync.RWMutex
}

// PerformanceBaseline represents performance baselines for different context sizes
type PerformanceBaseline struct {
	ContextSizeRange     string        `json:"context_size_range"`
	BaselineQuality      float64       `json:"baseline_quality"`
	BaselineResponseTime time.Duration `json:"baseline_response_time"`
	BaselineTokenUsage   int           `json:"baseline_token_usage"`
	SampleCount          int           `json:"sample_count"`
	LastUpdated          time.Time     `json:"last_updated"`
}

// AdjustmentRecord tracks automatic adjustments made by the system
type AdjustmentRecord struct {
	Timestamp            time.Time `json:"timestamp"`
	Trigger              string    `json:"trigger"`
	OldReductionTarget   float64   `json:"old_reduction_target"`
	NewReductionTarget   float64   `json:"new_reduction_target"`
	PerformanceDeviation float64   `json:"performance_deviation"`
	ContextSize          int       `json:"context_size"`
}

// NewPerformanceMonitor creates a new performance monitor
func NewPerformanceMonitor(cfg *config.ContextOptimizationConfig, logger *logrus.Logger) *PerformanceMonitor {
	monitor := &PerformanceMonitor{
		config:            cfg,
		metrics:           NewPerformanceMetrics(),
		baselines:         make(map[string]*PerformanceBaseline),
		logger:            logger,
		adjustmentHistory: make([]AdjustmentRecord, 0),
	}

	// Initialize default baselines
	monitor.initializeBaselines()

	return monitor
}

// Monitor processes performance data and detects degradation
func (pm *PerformanceMonitor) Monitor(ctx context.Context, responseQuality float64, responseTime time.Duration, tokenUsage int, contextSize int) (*PerformanceAssessment, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Record performance metrics
	pm.metrics.RecordPerformance(responseQuality, responseTime, tokenUsage, contextSize)

	// Get relevant baseline
	baseline := pm.getRelevantBaseline(contextSize)

	// Calculate baseline deviation
	deviation := pm.calculateBaselineDeviation(responseQuality, responseTime, tokenUsage, baseline)

	// Detect performance degradation
	degradationDetected := pm.detectDegradation(deviation)

	// Determine if adjustment is needed
	adjustmentTriggered := false
	newReductionTarget := 0.0

	if degradationDetected && pm.config.PerformanceMonitoring.AutoAdjustment {
		adjustmentTriggered = true
		newReductionTarget = pm.calculateNewReductionTarget(deviation, contextSize)

		// Record adjustment
		pm.recordAdjustment("performance_degradation", 0.0, newReductionTarget, deviation, contextSize)

		pm.logger.WithFields(logrus.Fields{
			"deviation":            deviation,
			"old_reduction_target": "auto_calculated",
			"new_reduction_target": newReductionTarget,
			"context_size":         contextSize,
		}).Warn("Performance degradation detected, triggering automatic adjustment")
	}

	// Update baseline if performance is good
	if !degradationDetected && deviation >= -0.1 { // Small positive or negative deviation is acceptable
		pm.updateBaseline(contextSize, responseQuality, responseTime, tokenUsage)
	}

	return &PerformanceAssessment{
		ResponseQuality:     responseQuality,
		ResponseTime:        responseTime,
		TokenUsage:          tokenUsage,
		BaselineDeviation:   deviation,
		DegradationDetected: degradationDetected,
		AdjustmentTriggered: adjustmentTriggered,
		NewReductionTarget:  newReductionTarget,
		Metadata: map[string]interface{}{
			"baseline_quality":        baseline.BaselineQuality,
			"baseline_response_time":  baseline.BaselineResponseTime.Milliseconds(),
			"baseline_token_usage":    baseline.BaselineTokenUsage,
			"context_size_range":      baseline.ContextSizeRange,
			"sample_count":            baseline.SampleCount,
			"monitoring_enabled":      pm.config.PerformanceMonitoring.Enabled,
			"auto_adjustment_enabled": pm.config.PerformanceMonitoring.AutoAdjustment,
		},
	}, nil
}

// getRelevantBaseline finds the most appropriate baseline for the given context size
func (pm *PerformanceMonitor) getRelevantBaseline(contextSize int) *PerformanceBaseline {
	var bestBaseline *PerformanceBaseline

	// Define context size ranges
	ranges := []struct {
		key     string
		minSize int
		maxSize int
	}{
		{"small", 0, 500},
		{"medium", 501, 1500},
		{"large", 1501, 3000},
		{"xlarge", 3001, 10000},
	}

	// Find matching range
	for _, r := range ranges {
		if contextSize >= r.minSize && contextSize <= r.maxSize {
			if baseline, exists := pm.baselines[r.key]; exists {
				bestBaseline = baseline
				break
			}
		}
	}

	// Fallback to medium baseline if no match found
	if bestBaseline == nil {
		if baseline, exists := pm.baselines["medium"]; exists {
			bestBaseline = baseline
		} else {
			// Create default baseline
			bestBaseline = &PerformanceBaseline{
				ContextSizeRange:     "default",
				BaselineQuality:      0.85,
				BaselineResponseTime: 2 * time.Second,
				BaselineTokenUsage:   1000,
				SampleCount:          1,
				LastUpdated:          time.Now(),
			}
		}
	}

	return bestBaseline
}

// calculateBaselineDeviation computes how much current performance deviates from baseline
func (pm *PerformanceMonitor) calculateBaselineDeviation(quality float64, responseTime time.Duration, tokenUsage int, baseline *PerformanceBaseline) float64 {
	// Calculate deviations for each metric
	qualityDeviation := (quality - baseline.BaselineQuality) / baseline.BaselineQuality
	timeDeviation := (float64(responseTime.Milliseconds()) - float64(baseline.BaselineResponseTime.Milliseconds())) / float64(baseline.BaselineResponseTime.Milliseconds())
	tokenDeviation := (float64(tokenUsage) - float64(baseline.BaselineTokenUsage)) / float64(baseline.BaselineTokenUsage)

	// Weight the deviations (quality is most important)
	weightedDeviation := (qualityDeviation * 0.5) + (timeDeviation * -0.3) + (tokenDeviation * -0.2)

	return weightedDeviation
}

// detectDegradation determines if performance has degraded beyond acceptable thresholds
func (pm *PerformanceMonitor) detectDegradation(deviation float64) bool {
	threshold := pm.config.PerformanceMonitoring.DegradationThreshold
	if threshold == 0 {
		threshold = 0.15 // Default 15% degradation threshold
	}

	// Negative deviation indicates degradation (quality down, time/tokens up)
	return deviation < -threshold
}

// calculateNewReductionTarget determines a less aggressive reduction target
func (pm *PerformanceMonitor) calculateNewReductionTarget(deviation float64, contextSize int) float64 {
	// Calculate how much to reduce the reduction target based on degradation severity
	adjustmentFactor := 1.0 + (deviation * -2.0) // Convert negative deviation to positive adjustment

	// Base reduction targets by context size
	baseReduction := 0.4 // Default moderate reduction

	if contextSize <= 500 {
		baseReduction = 0.6 // Can reduce more for small context
	} else if contextSize >= 1500 {
		baseReduction = 0.2 // Reduce less for large context
	}

	// Apply adjustment factor (reduce the reduction when performance degrades)
	newTarget := baseReduction / adjustmentFactor

	// Ensure reasonable bounds
	if newTarget < 0.1 {
		newTarget = 0.1
	}
	if newTarget > 0.8 {
		newTarget = 0.8
	}

	return newTarget
}

// updateBaseline updates performance baselines with new good performance data
func (pm *PerformanceMonitor) updateBaseline(contextSize int, quality float64, responseTime time.Duration, tokenUsage int) {
	rangeKey := pm.getContextSizeRangeKey(contextSize)

	baseline, exists := pm.baselines[rangeKey]
	if !exists {
		// Create new baseline
		baseline = &PerformanceBaseline{
			ContextSizeRange:     rangeKey,
			BaselineQuality:      quality,
			BaselineResponseTime: responseTime,
			BaselineTokenUsage:   tokenUsage,
			SampleCount:          1,
			LastUpdated:          time.Now(),
		}
		pm.baselines[rangeKey] = baseline
	} else {
		// Update existing baseline using exponential moving average
		alpha := 0.1 // Smoothing factor
		baseline.BaselineQuality = alpha*quality + (1-alpha)*baseline.BaselineQuality
		baseline.BaselineResponseTime = time.Duration(alpha*float64(responseTime) + (1-alpha)*float64(baseline.BaselineResponseTime))
		baseline.BaselineTokenUsage = int(alpha*float64(tokenUsage) + (1-alpha)*float64(baseline.BaselineTokenUsage))
		baseline.SampleCount++
		baseline.LastUpdated = time.Now()
	}
}

// recordAdjustment records an automatic adjustment for audit purposes
func (pm *PerformanceMonitor) recordAdjustment(trigger string, oldTarget, newTarget, deviation float64, contextSize int) {
	record := AdjustmentRecord{
		Timestamp:            time.Now(),
		Trigger:              trigger,
		OldReductionTarget:   oldTarget,
		NewReductionTarget:   newTarget,
		PerformanceDeviation: deviation,
		ContextSize:          contextSize,
	}

	pm.adjustmentHistory = append(pm.adjustmentHistory, record)

	// Keep only last 100 records
	if len(pm.adjustmentHistory) > 100 {
		pm.adjustmentHistory = pm.adjustmentHistory[1:]
	}
}

// getContextSizeRangeKey returns the range key for a given context size
func (pm *PerformanceMonitor) getContextSizeRangeKey(contextSize int) string {
	switch {
	case contextSize <= 500:
		return "small"
	case contextSize <= 1500:
		return "medium"
	case contextSize <= 3000:
		return "large"
	default:
		return "xlarge"
	}
}

// initializeBaselines sets up default performance baselines
func (pm *PerformanceMonitor) initializeBaselines() {
	pm.baselines["small"] = &PerformanceBaseline{
		ContextSizeRange:     "small",
		BaselineQuality:      0.90,
		BaselineResponseTime: 1 * time.Second,
		BaselineTokenUsage:   300,
		SampleCount:          0,
		LastUpdated:          time.Now(),
	}

	pm.baselines["medium"] = &PerformanceBaseline{
		ContextSizeRange:     "medium",
		BaselineQuality:      0.85,
		BaselineResponseTime: 2 * time.Second,
		BaselineTokenUsage:   1000,
		SampleCount:          0,
		LastUpdated:          time.Now(),
	}

	pm.baselines["large"] = &PerformanceBaseline{
		ContextSizeRange:     "large",
		BaselineQuality:      0.80,
		BaselineResponseTime: 4 * time.Second,
		BaselineTokenUsage:   2500,
		SampleCount:          0,
		LastUpdated:          time.Now(),
	}

	pm.baselines["xlarge"] = &PerformanceBaseline{
		ContextSizeRange:     "xlarge",
		BaselineQuality:      0.75,
		BaselineResponseTime: 6 * time.Second,
		BaselineTokenUsage:   5000,
		SampleCount:          0,
		LastUpdated:          time.Now(),
	}
}

// GetAdjustmentHistory returns recent adjustment history
func (pm *PerformanceMonitor) GetAdjustmentHistory() []AdjustmentRecord {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Return a copy to prevent external modification
	history := make([]AdjustmentRecord, len(pm.adjustmentHistory))
	copy(history, pm.adjustmentHistory)
	return history
}

// GetBaselines returns current performance baselines
func (pm *PerformanceMonitor) GetBaselines() map[string]*PerformanceBaseline {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Return a copy to prevent external modification
	baselines := make(map[string]*PerformanceBaseline)
	for key, baseline := range pm.baselines {
		baselineCopy := *baseline
		baselines[key] = &baselineCopy
	}
	return baselines
}
