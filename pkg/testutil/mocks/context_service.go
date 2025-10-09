/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mocks

import (
	"context"
)

// MockContextService implements a mock context service for testing
type MockContextService struct {
	complexityAssessment   map[string]interface{}
	optimizationStrategy   string
	performanceMonitoring  bool
	correlationTracking    bool
	degradationThreshold   float64
	autoAdjustmentEnabled  bool
	baselinePerformance    float64
	performanceDegradation bool
	degradationLevel       float64
	state                  map[string]interface{}
}

// NewMockContextService creates a new mock context service
func NewMockContextService() *MockContextService {
	return &MockContextService{
		complexityAssessment:   make(map[string]interface{}),
		state:                  make(map[string]interface{}),
		performanceMonitoring:  false,
		correlationTracking:    false,
		degradationThreshold:   0.15,
		autoAdjustmentEnabled:  false,
		baselinePerformance:    0.85,
		performanceDegradation: false,
		degradationLevel:       0.0,
	}
}

// SetComplexityAssessment sets the complexity assessment for alerts
func (m *MockContextService) SetComplexityAssessment(complexity string, minContextTypes int) {
	m.complexityAssessment["complexity"] = complexity
	m.complexityAssessment["min_context_types"] = minContextTypes
}

// SetOptimizationStrategy sets the optimization strategy
func (m *MockContextService) SetOptimizationStrategy(strategy string) {
	m.optimizationStrategy = strategy
}

// SetPerformanceMonitoring enables/disables performance monitoring
func (m *MockContextService) SetPerformanceMonitoring(enabled bool) {
	m.performanceMonitoring = enabled
}

// SetCorrelationTracking enables/disables correlation tracking
func (m *MockContextService) SetCorrelationTracking(enabled bool) {
	m.correlationTracking = enabled
}

// SetDegradationThreshold sets the performance degradation threshold
func (m *MockContextService) SetDegradationThreshold(threshold float64) {
	m.degradationThreshold = threshold
}

// SetAutoAdjustmentEnabled enables/disables automatic adjustment
func (m *MockContextService) SetAutoAdjustmentEnabled(enabled bool) {
	m.autoAdjustmentEnabled = enabled
}

// SetBaselinePerformance sets the baseline performance level
func (m *MockContextService) SetBaselinePerformance(baseline float64) {
	m.baselinePerformance = baseline
}

// SetPerformanceDegradation sets performance degradation state
func (m *MockContextService) SetPerformanceDegradation(degraded bool, level float64) {
	m.performanceDegradation = degraded
	m.degradationLevel = level
}

// ClearState clears all mock state
func (m *MockContextService) ClearState() {
	m.complexityAssessment = make(map[string]interface{})
	m.state = make(map[string]interface{})
	m.optimizationStrategy = ""
	m.performanceMonitoring = false
	m.correlationTracking = false
	m.degradationThreshold = 0.15
	m.autoAdjustmentEnabled = false
	m.baselinePerformance = 0.85
	m.performanceDegradation = false
	m.degradationLevel = 0.0
}

// AssessComplexity assesses the complexity of an alert
func (m *MockContextService) AssessComplexity(ctx context.Context, alertType string) (string, error) {
	if complexity, exists := m.complexityAssessment["complexity"]; exists {
		return complexity.(string), nil
	}

	// Default complexity assessment based on alert type
	switch alertType {
	case "DiskSpaceWarning":
		return "simple", nil
	case "HighMemoryUsage":
		return "moderate", nil
	case "NetworkPartition":
		return "complex", nil
	case "SecurityBreach":
		return "critical", nil
	default:
		return "moderate", nil
	}
}

// GetOptimizationStrategy returns the current optimization strategy
func (m *MockContextService) GetOptimizationStrategy() string {
	return m.optimizationStrategy
}

// IsPerformanceMonitoringEnabled returns whether performance monitoring is enabled
func (m *MockContextService) IsPerformanceMonitoringEnabled() bool {
	return m.performanceMonitoring
}

// IsCorrelationTrackingEnabled returns whether correlation tracking is enabled
func (m *MockContextService) IsCorrelationTrackingEnabled() bool {
	return m.correlationTracking
}

// GetDegradationThreshold returns the performance degradation threshold
func (m *MockContextService) GetDegradationThreshold() float64 {
	return m.degradationThreshold
}

// IsAutoAdjustmentEnabled returns whether automatic adjustment is enabled
func (m *MockContextService) IsAutoAdjustmentEnabled() bool {
	return m.autoAdjustmentEnabled
}

// GetBaselinePerformance returns the baseline performance level
func (m *MockContextService) GetBaselinePerformance() float64 {
	return m.baselinePerformance
}

// IsPerformanceDegraded returns whether performance is currently degraded
func (m *MockContextService) IsPerformanceDegraded() (bool, float64) {
	return m.performanceDegradation, m.degradationLevel
}

// MonitorPerformance monitors performance and detects degradation
func (m *MockContextService) MonitorPerformance(ctx context.Context) error {
	// Mock implementation - always returns nil
	return nil
}

// AdjustContext adjusts context based on performance monitoring
func (m *MockContextService) AdjustContext(ctx context.Context, currentTokens int) (int, error) {
	if m.performanceDegradation {
		// Increase context when degradation is detected
		adjustmentFactor := 1.0 + m.degradationLevel
		return int(float64(currentTokens) * adjustmentFactor), nil
	}
	return currentTokens, nil
}
