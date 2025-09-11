//go:build integration

package workflow_engine

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// IntegrationTestConfig holds configuration for integration tests
type IntegrationTestConfig struct {
	// SLM Configuration
	LLMProvider  string
	LLMModel     string
	LLMEndpoint  string
	SkipSLMTests bool

	// Test Control
	SkipPerformanceTests bool
	SkipSlowTests        bool
	TestTimeout          time.Duration
	LogLevel             string
}

// LoadIntegrationTestConfig loads configuration from environment variables
func LoadIntegrationTestConfig() IntegrationTestConfig {
	config := IntegrationTestConfig{
		// Default SLM settings
		LLMProvider: getEnvWithDefault("LLM_PROVIDER", "ollama"),
		LLMModel:    getEnvWithDefault("LLM_MODEL", "granite3.1-dense:8b"),
		LLMEndpoint: getEnvWithDefault("LLM_ENDPOINT", "http://localhost:11434"),

		// Test control settings
		SkipSLMTests:         getBoolEnv("SKIP_SLM_TESTS", false),
		SkipPerformanceTests: getBoolEnv("SKIP_PERFORMANCE_TESTS", false),
		SkipSlowTests:        getBoolEnv("SKIP_SLOW_TESTS", false),
		TestTimeout:          getDurationEnv("TEST_TIMEOUT", 5*time.Minute),
		LogLevel:             getEnvWithDefault("LOG_LEVEL", "info"),
	}

	return config
}

// PerformanceReport tracks performance metrics during integration tests
type PerformanceReport struct {
	TotalTests         int
	PassedTests        int
	TotalResponseTime  time.Duration
	WorkflowGeneration []PerformanceMetric
	Validation         []PerformanceMetric
	Simulation         []PerformanceMetric
	Learning           []PerformanceMetric
	startTime          time.Time
}

type PerformanceMetric struct {
	Duration  time.Duration
	Success   bool
	Timestamp time.Time
}

func NewPerformanceReport() *PerformanceReport {
	return &PerformanceReport{
		startTime:          time.Now(),
		WorkflowGeneration: make([]PerformanceMetric, 0),
		Validation:         make([]PerformanceMetric, 0),
		Simulation:         make([]PerformanceMetric, 0),
		Learning:           make([]PerformanceMetric, 0),
	}
}

func (pr *PerformanceReport) RecordWorkflowGeneration(duration time.Duration, success bool) {
	pr.WorkflowGeneration = append(pr.WorkflowGeneration, PerformanceMetric{
		Duration:  duration,
		Success:   success,
		Timestamp: time.Now(),
	})
}

func (pr *PerformanceReport) RecordValidation(duration time.Duration, success bool) {
	pr.Validation = append(pr.Validation, PerformanceMetric{
		Duration:  duration,
		Success:   success,
		Timestamp: time.Now(),
	})
}

func (pr *PerformanceReport) RecordSimulation(duration time.Duration, success bool) {
	pr.Simulation = append(pr.Simulation, PerformanceMetric{
		Duration:  duration,
		Success:   success,
		Timestamp: time.Now(),
	})
}

func (pr *PerformanceReport) RecordLearning(duration time.Duration, success bool) {
	pr.Learning = append(pr.Learning, PerformanceMetric{
		Duration:  duration,
		Success:   success,
		Timestamp: time.Now(),
	})
}

func (pr *PerformanceReport) RecordTestCompletion() {
	// This could write metrics to a file or monitoring system
}

func (pr *PerformanceReport) ForceGC() {
	runtime.GC()
	runtime.GC() // Call twice for better cleanup
}

// IntegrationVectorDatabase provides a vector database implementation for integration testing
type IntegrationVectorDatabase struct {
	patterns map[string]*vector.ActionPattern
}

func NewIntegrationVectorDatabase() *IntegrationVectorDatabase {
	return &IntegrationVectorDatabase{
		patterns: make(map[string]*vector.ActionPattern),
	}
}

func (ivd *IntegrationVectorDatabase) StoreActionPattern(ctx context.Context, pattern *vector.ActionPattern) error {
	ivd.patterns[pattern.ID] = pattern
	return nil
}

func (ivd *IntegrationVectorDatabase) FindSimilarPatterns(ctx context.Context, pattern *vector.ActionPattern, limit int, threshold float64) ([]*vector.SimilarPattern, error) {
	// Simple implementation for testing
	similar := make([]*vector.SimilarPattern, 0)
	count := 0
	for _, storedPattern := range ivd.patterns {
		if count >= limit {
			break
		}
		similar = append(similar, &vector.SimilarPattern{
			Pattern:    storedPattern,
			Similarity: 0.8, // Mock similarity score
		})
		count++
	}
	return similar, nil
}

func (ivd *IntegrationVectorDatabase) UpdatePatternEffectiveness(ctx context.Context, patternID string, effectiveness float64) error {
	if pattern, exists := ivd.patterns[patternID]; exists {
		if pattern.EffectivenessData != nil {
			pattern.EffectivenessData.Score = effectiveness
		}
	}
	return nil
}

func (ivd *IntegrationVectorDatabase) SearchBySemantics(ctx context.Context, query string, limit int) ([]*vector.ActionPattern, error) {
	// Simple implementation for testing - return stored patterns
	patterns := make([]*vector.ActionPattern, 0)
	count := 0
	for _, pattern := range ivd.patterns {
		if count >= limit {
			break
		}
		patterns = append(patterns, pattern)
		count++
	}
	return patterns, nil
}

func (ivd *IntegrationVectorDatabase) DeletePattern(ctx context.Context, patternID string) error {
	delete(ivd.patterns, patternID)
	return nil
}

func (ivd *IntegrationVectorDatabase) GetPatternAnalytics(ctx context.Context) (*vector.PatternAnalytics, error) {
	topPatterns := make([]*vector.ActionPattern, 0)
	recentPatterns := make([]*vector.ActionPattern, 0)

	for _, pattern := range ivd.patterns {
		topPatterns = append(topPatterns, pattern)
		recentPatterns = append(recentPatterns, pattern)
		if len(topPatterns) >= 5 {
			break
		}
	}

	return &vector.PatternAnalytics{
		TotalPatterns:             len(ivd.patterns),
		PatternsByActionType:      map[string]int{"scale_deployment": 5, "restart_pod": 3},
		PatternsBySeverity:        map[string]int{"high": 2, "medium": 4, "low": 2},
		AverageEffectiveness:      0.85,
		TopPerformingPatterns:     topPatterns,
		RecentPatterns:            recentPatterns,
		EffectivenessDistribution: map[string]int{"0.8-1.0": 6, "0.6-0.8": 2},
		GeneratedAt:               time.Now(),
	}, nil
}

func (ivd *IntegrationVectorDatabase) IsHealthy(ctx context.Context) error {
	// Always healthy for integration tests
	return nil
}

func (ivd *IntegrationVectorDatabase) GetStoredPatterns() []*vector.ActionPattern {
	patterns := make([]*vector.ActionPattern, 0, len(ivd.patterns))
	for _, pattern := range ivd.patterns {
		patterns = append(patterns, pattern)
	}
	return patterns
}

// Enhanced mock SLM client with realistic responses for integration testing
type MockSLMClientWithRealisticResponses struct {
	responses map[string]*engine.AIWorkflowResponse
}

func NewMockSLMClientWithRealisticResponses() *MockSLMClientWithRealisticResponses {
	client := &MockSLMClientWithRealisticResponses{
		responses: make(map[string]*engine.AIWorkflowResponse),
	}

	// Pre-configure realistic responses for different scenarios
	client.responses["memory_optimization"] = &engine.AIWorkflowResponse{
		WorkflowName: "Memory Optimization Workflow",
		Description:  "Comprehensive memory optimization for Kubernetes deployment",
		Steps: []*engine.AIGeneratedStep{
			{
				Name: "Collect Current Memory Metrics",
				Type: "action",
				Action: &engine.AIGeneratedAction{
					Type: "collect_diagnostics",
					Parameters: map[string]interface{}{
						"metrics":  []string{"memory", "cpu", "network"},
						"duration": "60s",
					},
				},
				Timeout: "2m",
			},
			{
				Name: "Analyze Memory Usage Patterns",
				Type: "action",
				Action: &engine.AIGeneratedAction{
					Type: "collect_diagnostics",
					Parameters: map[string]interface{}{
						"analysis_type": "memory_pattern",
						"window":        "24h",
					},
				},
				Dependencies: []string{"Collect Current Memory Metrics"},
				Timeout:      "1m",
			},
			{
				Name: "Increase Memory Limits",
				Type: "action",
				Action: &engine.AIGeneratedAction{
					Type: "increase_resources",
					Parameters: map[string]interface{}{
						"memory":   "2Gi",
						"strategy": "gradual",
					},
				},
				Dependencies: []string{"Analyze Memory Usage Patterns"},
				Timeout:      "3m",
			},
		},
		EstimatedTime:  "6m",
		RiskAssessment: "medium",
		Reasoning:      "Memory optimization requires careful analysis and gradual resource increases to avoid service disruption.",
	}

	client.responses["crashloop"] = &engine.AIWorkflowResponse{
		WorkflowName: "Pod Crashloop Recovery Workflow",
		Description:  "Systematic approach to diagnose and resolve pod crashloop issues",
		Steps: []*engine.AIGeneratedStep{
			{
				Name: "Collect Pod Logs",
				Type: "action",
				Action: &engine.AIGeneratedAction{
					Type: "collect_diagnostics",
					Parameters: map[string]interface{}{
						"log_lines":        1000,
						"include_previous": true,
					},
				},
				Timeout: "1m",
			},
			{
				Name: "Check Resource Constraints",
				Type: "action",
				Action: &engine.AIGeneratedAction{
					Type: "collect_diagnostics",
					Parameters: map[string]interface{}{
						"resource_check":  true,
						"limits_analysis": true,
					},
				},
				Timeout: "30s",
			},
			{
				Name: "Restart Pod with Debug Mode",
				Type: "action",
				Action: &engine.AIGeneratedAction{
					Type: "restart_pod",
					Parameters: map[string]interface{}{
						"debug_mode":   true,
						"safe_restart": true,
					},
				},
				Dependencies: []string{"Collect Pod Logs", "Check Resource Constraints"},
				Timeout:      "2m",
			},
		},
		EstimatedTime:  "4m",
		RiskAssessment: "low",
		Reasoning:      "Crashloop recovery focuses on diagnostics first, then safe restart procedures.",
	}

	return client
}

func (m *MockSLMClientWithRealisticResponses) AnalyzeAlert(ctx context.Context, alert types.Alert) (*types.ActionRecommendation, error) {
	return &types.ActionRecommendation{
		Action:     "mock_action",
		Confidence: 0.85,
		Reasoning: &types.ReasoningDetails{
			Summary: "Integration test mock analysis",
		},
		Parameters: make(map[string]interface{}),
	}, nil
}

func (m *MockSLMClientWithRealisticResponses) ChatCompletion(ctx context.Context, prompt string) (string, error) {
	// Simple mock chat completion
	return "Mock chat completion response for integration testing", nil
}

func (m *MockSLMClientWithRealisticResponses) IsHealthy() bool {
	return true
}

func (m *MockSLMClientWithRealisticResponses) GetModelInfo() string {
	return "mock-realistic-model-1.0-integration-test"
}

func (m *MockSLMClientWithRealisticResponses) GenerateWorkflowFromObjective(ctx context.Context, objective *engine.WorkflowObjective) (*engine.AIWorkflowResponse, error) {
	// Return appropriate response based on objective type
	if response, exists := m.responses[objective.Type]; exists {
		return response, nil
	}

	// Default response for unknown types
	return &engine.AIWorkflowResponse{
		WorkflowName: "Generic Workflow",
		Description:  "Default workflow for integration testing",
		Steps: []*engine.AIGeneratedStep{
			{
				Name: "Diagnostic Step",
				Type: "action",
				Action: &engine.AIGeneratedAction{
					Type: "collect_diagnostics",
					Parameters: map[string]interface{}{
						"basic_check": true,
					},
				},
				Timeout: "1m",
			},
		},
		EstimatedTime:  "2m",
		RiskAssessment: "low",
		Reasoning:      "Generic workflow for unspecified objectives",
	}, nil
}

// Enhanced mock pattern extractor for integration testing
type MockPatternExtractorForIntegration struct{}

func NewMockPatternExtractor() *MockPatternExtractorForIntegration {
	return &MockPatternExtractorForIntegration{}
}

func (m *MockPatternExtractorForIntegration) ExtractPatterns(ctx context.Context, data interface{}) ([]*vector.ActionPattern, error) {
	// Return some mock patterns for testing
	return []*vector.ActionPattern{
		{
			ID:               "integration-pattern-1",
			ActionType:       "workflow",
			AlertName:        "mock-alert",
			AlertSeverity:    "medium",
			Namespace:        "default",
			ResourceType:     "deployment",
			ResourceName:     "test-app",
			ActionParameters: make(map[string]interface{}),
			ContextLabels:    make(map[string]string),
			PreConditions:    make(map[string]interface{}),
			PostConditions:   make(map[string]interface{}),
			EffectivenessData: &vector.EffectivenessData{
				Score:                0.8,
				SuccessCount:         5,
				FailureCount:         1,
				AverageExecutionTime: time.Minute * 2,
				SideEffectsCount:     0,
				RecurrenceRate:       0.1,
				ContextualFactors:    make(map[string]float64),
				LastAssessed:         time.Now(),
			},
			Embedding: []float64{0.1, 0.2, 0.3},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}, nil
}

func (m *MockPatternExtractorForIntegration) ExtractPattern(ctx context.Context, trace *actionhistory.ResourceActionTrace) (*vector.ActionPattern, error) {
	// Simple mock pattern extraction
	return &vector.ActionPattern{
		ID:               "extracted-pattern-1",
		ActionType:       "mock_action",
		AlertName:        "mock-alert",
		AlertSeverity:    "medium",
		Namespace:        "default",
		ResourceType:     "deployment",
		ResourceName:     "mock-app",
		ActionParameters: make(map[string]interface{}),
		ContextLabels:    make(map[string]string),
		PreConditions:    make(map[string]interface{}),
		PostConditions:   make(map[string]interface{}),
		EffectivenessData: &vector.EffectivenessData{
			Score:                0.8,
			SuccessCount:         3,
			FailureCount:         0,
			AverageExecutionTime: time.Minute,
			SideEffectsCount:     0,
			RecurrenceRate:       0.0,
			ContextualFactors:    make(map[string]float64),
			LastAssessed:         time.Now(),
		},
		Embedding: []float64{0.2, 0.4, 0.6},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

func (m *MockPatternExtractorForIntegration) GenerateEmbedding(ctx context.Context, pattern *vector.ActionPattern) ([]float64, error) {
	// Simple mock embedding generation
	return []float64{0.1, 0.2, 0.3, 0.4, 0.5}, nil
}

func (m *MockPatternExtractorForIntegration) ExtractFeatures(ctx context.Context, pattern *vector.ActionPattern) (map[string]float64, error) {
	// Simple mock feature extraction
	features := map[string]float64{
		"alert_severity":        0.7,
		"effectiveness_score":   0.8,
		"hour_of_day":           12.0,
		"namespace_criticality": 0.9,
	}
	return features, nil
}

func (m *MockPatternExtractorForIntegration) CalculateSimilarity(pattern1, pattern2 *vector.ActionPattern) float64 {
	// Simple mock similarity calculation
	if pattern1.ActionType == pattern2.ActionType {
		return 0.8
	}
	return 0.3
}

// Utility functions

func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}
