package testutil

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/jordigilh/kubernaut/pkg/intelligence/learning"
	"github.com/jordigilh/kubernaut/pkg/intelligence/patterns"
	"github.com/jordigilh/kubernaut/pkg/intelligence/shared"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// IntelligenceTestDataFactory provides standardized test data creation for intelligence tests
type IntelligenceTestDataFactory struct{}

// NewIntelligenceTestDataFactory creates a new test data factory for intelligence tests
func NewIntelligenceTestDataFactory() *IntelligenceTestDataFactory {
	return &IntelligenceTestDataFactory{}
}

// CreateWorkflowExecutionData creates test workflow execution data
func (f *IntelligenceTestDataFactory) CreateWorkflowExecutionData(count int, successRate float64) []*engine.EngineWorkflowExecutionData {
	data := make([]*engine.EngineWorkflowExecutionData, count)
	successCount := int(float64(count) * successRate)

	for i := 0; i < count; i++ {
		data[i] = &engine.EngineWorkflowExecutionData{
			ExecutionID: f.generateExecutionID(i),
			WorkflowID:  f.generateWorkflowID(i),
			Timestamp:   time.Now().Add(-time.Duration(i) * time.Hour),
			Duration:    time.Duration(100+i*10) * time.Millisecond,
			Success:     i < successCount,
			Metrics: map[string]float64{
				"cpu_usage":    0.1 + float64(i)*0.01,
				"memory_usage": 0.2 + float64(i)*0.02,
				"alert_count":  float64(rand.Intn(10)),
				"duration_ms":  float64(100 + i*10),
			},
			Metadata: map[string]interface{}{
				"node":           f.generateNodeName(i),
				"alert_name":     f.generateAlertName(i),
				"alert_severity": f.generateAlertSeverity(i),
				"namespace":      f.generateNamespace(i),
			},
		}
	}
	return data
}

// CreateBalancedWorkflowExecutionData creates balanced test data with equal success/failure rates
func (f *IntelligenceTestDataFactory) CreateBalancedWorkflowExecutionData(count int) []*engine.EngineWorkflowExecutionData {
	return f.CreateWorkflowExecutionData(count, 0.5)
}

// CreateHighSuccessWorkflowExecutionData creates test data with high success rate
func (f *IntelligenceTestDataFactory) CreateHighSuccessWorkflowExecutionData(count int) []*engine.EngineWorkflowExecutionData {
	return f.CreateWorkflowExecutionData(count, 0.9)
}

// CreateLowSuccessWorkflowExecutionData creates test data with low success rate
func (f *IntelligenceTestDataFactory) CreateLowSuccessWorkflowExecutionData(count int) []*engine.EngineWorkflowExecutionData {
	return f.CreateWorkflowExecutionData(count, 0.1)
}

// CreateMLModel creates a test ML model
func (f *IntelligenceTestDataFactory) CreateMLModel() *learning.MLModel {
	return &learning.MLModel{
		ID:        "test-model-1",
		Type:      "classification",
		Version:   1,
		TrainedAt: time.Now(),
		Accuracy:  0.85,
		Features:  []string{"cpu_usage", "memory_usage", "alert_count", "duration"},
		Parameters: map[string]interface{}{
			"learning_rate": 0.01,
			"epochs":        100,
			"batch_size":    32,
		},
		Weights: []float64{0.1, 0.2, 0.3, 0.4},
		Bias:    0.05,
	}
}

// CreateHighAccuracyMLModel creates a high-accuracy ML model for testing
func (f *IntelligenceTestDataFactory) CreateHighAccuracyMLModel() *learning.MLModel {
	model := f.CreateMLModel()
	model.ID = "high-accuracy-model"
	model.Accuracy = 0.95
	return model
}

// CreateLowAccuracyMLModel creates a low-accuracy ML model for testing
func (f *IntelligenceTestDataFactory) CreateLowAccuracyMLModel() *learning.MLModel {
	model := f.CreateMLModel()
	model.ID = "low-accuracy-model"
	model.Accuracy = 0.55
	return model
}

// CreatePatternAnalysisRequest creates a test pattern analysis request
func (f *IntelligenceTestDataFactory) CreatePatternAnalysisRequest() *patterns.PatternAnalysisRequest {
	return &patterns.PatternAnalysisRequest{
		AnalysisType: "comprehensive",
		Filters: map[string]interface{}{
			"min_executions": 10,
			"max_age_hours":  168,
		},
	}
}

// CreatePatternAnalysisResult creates a test pattern analysis result
func (f *IntelligenceTestDataFactory) CreatePatternAnalysisResult() *patterns.PatternAnalysisResult {
	return &patterns.PatternAnalysisResult{
		Patterns: []*shared.DiscoveredPattern{
			f.CreateDiscoveredPattern("temporal", 0.92),
			f.CreateDiscoveredPattern("resource", 0.85),
			f.CreateDiscoveredPattern("failure", 0.78),
		},
	}
}

// CreateDiscoveredPattern creates a test discovered pattern
func (f *IntelligenceTestDataFactory) CreateDiscoveredPattern(patternType string, confidence float64) *shared.DiscoveredPattern {
	return &shared.DiscoveredPattern{
		BasePattern: types.BasePattern{
			BaseEntity: types.BaseEntity{
				ID:          fmt.Sprintf("%s-pattern-1", patternType),
				Name:        fmt.Sprintf("%s Pattern", patternType),
				Description: fmt.Sprintf("Detected %s pattern with 25 occurrences", patternType),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				Metadata:    map[string]interface{}{"pattern_type": patternType},
			},
			Type:                 patternType,
			Confidence:           confidence,
			Frequency:            25,
			SuccessRate:          0.84,
			AverageExecutionTime: 150 * time.Second,
			LastSeen:             time.Now().Add(-1 * time.Hour),
			Tags:                 []string{"test", patternType},
			Metrics: map[string]float64{
				"avg_duration":         2.5,
				"success_rate":         0.84,
				"correlation_strength": confidence,
			},
		},
		DiscoveredAt: time.Now().Add(-7 * 24 * time.Hour),
	}
}

// CreateLearningMetrics creates test learning metrics
func (f *IntelligenceTestDataFactory) CreateLearningMetrics() *patterns.LearningMetrics {
	metrics := patterns.NewLearningMetrics()
	metrics.TotalAnalyses = 100
	metrics.TotalExecutions = 1000
	metrics.PatternsDiscovered = 15
	metrics.AverageConfidence = 0.85
	metrics.LearningRate = 0.01
	metrics.LastUpdated = time.Now()

	metrics.PerformanceMetrics = map[string]float64{
		"accuracy":        0.85,
		"precision":       0.82,
		"recall":          0.88,
		"f1_score":        0.85,
		"processing_time": 120.5,
		"memory_usage":    512.0,
		"model_size":      1024.0,
	}

	return metrics
}

// CreatePatternDiscoveryConfig creates test pattern discovery configuration
func (f *IntelligenceTestDataFactory) CreatePatternDiscoveryConfig() *patterns.PatternDiscoveryConfig {
	return &patterns.PatternDiscoveryConfig{
		MinExecutionsForPattern: 10,
		MaxHistoryDays:          90,
		SimilarityThreshold:     0.85,
		PredictionConfidence:    0.7,
		ClusteringEpsilon:       0.3,
		MinClusterSize:          5,
		MaxConcurrentAnalysis:   10,
		PatternCacheSize:        1000,
		EnableRealTimeDetection: true,
	}
}

// CreateEnhancedPatternDiscoveryConfig creates enhanced pattern discovery configuration
func (f *IntelligenceTestDataFactory) CreateEnhancedPatternDiscoveryConfig() *patterns.PatternDiscoveryConfig {
	return &patterns.PatternDiscoveryConfig{
		MinExecutionsForPattern: 50,
		MaxHistoryDays:          365,
		SimilarityThreshold:     0.95,
		PredictionConfidence:    0.9,
		ClusteringEpsilon:       0.1,
		MinClusterSize:          20,
		MaxConcurrentAnalysis:   20,
		PatternCacheSize:        5000,
		EnableRealTimeDetection: true,
	}
}

// CreateWorkflowFeatures creates test workflow features
func (f *IntelligenceTestDataFactory) CreateWorkflowFeatures() *shared.WorkflowFeatures {
	return &shared.WorkflowFeatures{}
}

// CreateWorkflowPrediction creates test workflow prediction
func (f *IntelligenceTestDataFactory) CreateWorkflowPrediction() *shared.WorkflowPrediction {
	return &shared.WorkflowPrediction{
		Confidence: 0.87,
	}
}

// Helper methods for generating test data
func (f *IntelligenceTestDataFactory) generateExecutionID(i int) string {
	return fmt.Sprintf("exec-%d-%d", time.Now().Unix(), i)
}

func (f *IntelligenceTestDataFactory) generateWorkflowID(i int) string {
	workflows := []string{"workflow-1", "workflow-2", "workflow-3", "workflow-4"}
	return workflows[i%len(workflows)]
}

func (f *IntelligenceTestDataFactory) generateNodeName(i int) string {
	nodes := []string{"node-1", "node-2", "node-3"}
	return nodes[i%len(nodes)]
}

func (f *IntelligenceTestDataFactory) generateAlertName(i int) string {
	alerts := []string{"HighCPUUsage", "HighMemoryUsage", "DiskSpaceWarning", "NetworkLatency"}
	return alerts[i%len(alerts)]
}

func (f *IntelligenceTestDataFactory) generateAlertSeverity(i int) string {
	severities := []string{"critical", "warning", "info"}
	return severities[i%len(severities)]
}

func (f *IntelligenceTestDataFactory) generateNamespace(i int) string {
	namespaces := []string{"production", "staging", "development", "monitoring"}
	return namespaces[i%len(namespaces)]
}

// CreateMLAnalyzerConfig creates test ML analyzer configuration
func (f *IntelligenceTestDataFactory) CreateMLAnalyzerConfig() *learning.MLConfig {
	return &learning.MLConfig{
		MinExecutionsForPattern: 10,
		MaxHistoryDays:          90,
		SimilarityThreshold:     0.85,
		ClusteringEpsilon:       0.3,
		MinClusterSize:          5,
		FeatureWindowSize:       50,
		PredictionConfidence:    0.7,
	}
}

// CreateTimeSeriesData creates time series test data
func (f *IntelligenceTestDataFactory) CreateTimeSeriesData(points int, pattern string) []map[string]interface{} {
	data := make([]map[string]interface{}, points)
	baseTime := time.Now().Add(-time.Duration(points) * time.Hour)

	for i := 0; i < points; i++ {
		var value float64
		switch pattern {
		case "increasing":
			value = 0.5 + float64(i)*0.01 + rand.Float64()*0.1
		case "decreasing":
			value = 1.0 - float64(i)*0.01 + rand.Float64()*0.1
		case "oscillating":
			value = 0.5 + 0.3*float64(i%24)/24.0 + rand.Float64()*0.1
		case "stable":
			value = 0.7 + rand.Float64()*0.05
		default:
			value = rand.Float64()
		}

		data[i] = map[string]interface{}{
			"timestamp": baseTime.Add(time.Duration(i) * time.Hour),
			"value":     value,
			"pattern":   pattern,
			"index":     i,
		}
	}

	return data
}
