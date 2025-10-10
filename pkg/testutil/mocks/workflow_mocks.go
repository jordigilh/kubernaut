<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package mocks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jordigilh/kubernaut/internal/actionhistory"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/intelligence/analytics"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/jordigilh/kubernaut/pkg/workflow/shared"
)

// MockLLMClient provides a mock implementation of llm.Client for testing
type MockLLMClient struct {
	workflowResponse *engine.AIWorkflowResponse
	chatResponse     string
	analysisResult   *types.ActionRecommendation
	error            error
	healthy          bool

	// Enhanced features from integration_mocks.go
	analysisResponse *AnalysisResponse
	parsedResponse   *AnalysisResponse
	analysisError    error
	parseError       error
	rawResponse      string

	// Rate limiting simulation
	rateLimitRetries int
	rateLimitDelay   time.Duration
	currentRetry     int

	// Request tracking for context enrichment testing
	lastAnalyzeAlertRequest *types.Alert
	analyzeAlertHistory     []types.Alert

	// Health monitoring fields - using shared types following guidelines
	healthStatus     types.HealthStatus
	responseTime     time.Duration
	endpoint         string
	lastError        string
	failureCount     int
	uptime           time.Duration
	downtime         time.Duration
	uptimePercentage float64
	accuracyRate     float64
	downtimeEvents   int
}

func NewMockLLMClient() *MockLLMClient {
	now := time.Now()
	return &MockLLMClient{
		healthy:          true,
		responseTime:     25 * time.Millisecond,
		endpoint:         "http://192.168.1.169:8080",
		uptime:           24 * time.Hour,
		uptimePercentage: 99.95,
		accuracyRate:     99.8,
		healthStatus: types.HealthStatus{
			BaseEntity: types.BaseEntity{
				ID:          "llm-health-monitor",
				Name:        "LLM Health Monitor",
				Description: "20B+ Model Health Monitoring",
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			BaseTimestampedResult: types.BaseTimestampedResult{
				Success:   true,
				StartTime: now,
				EndTime:   now,
				Duration:  25 * time.Millisecond,
			},
			IsHealthy:       true,
			ComponentType:   "llm-20b",
			ServiceEndpoint: "http://192.168.1.169:8080",
			ResponseTime:    25 * time.Millisecond,
			HealthMetrics: types.HealthMetrics{
				UptimePercentage: 99.95,
				TotalUptime:      24 * time.Hour,
				AccuracyRate:     99.8,
			},
		},
	}
}

func (m *MockLLMClient) SetWorkflowResponse(response *engine.AIWorkflowResponse) {
	m.workflowResponse = response
	m.error = nil
}

func (m *MockLLMClient) SetError(errMsg string) {
	m.error = errors.New(errMsg)
	m.lastError = errMsg
	m.failureCount++
	m.healthy = false
	m.workflowResponse = nil
	m.chatResponse = ""
	m.analysisResult = nil

	// Update health status with structured error information using shared types
	now := time.Now()
	m.healthStatus.IsHealthy = false
	m.healthStatus.BaseTimestampedResult.Error = errMsg
	m.healthStatus.BaseTimestampedResult.Success = false
	m.healthStatus.BaseEntity.UpdatedAt = now
	m.healthStatus.HealthMetrics.FailureCount = m.failureCount
	m.healthStatus.HealthMetrics.LastFailureTime = now
}

func (m *MockLLMClient) SetChatResponse(response string) {
	m.chatResponse = response
	m.error = nil
}

func (m *MockLLMClient) SetHealthy(healthy bool) {
	m.healthy = healthy
	now := time.Now()
	m.healthStatus.IsHealthy = healthy
	m.healthStatus.BaseEntity.UpdatedAt = now
	m.healthStatus.BaseTimestampedResult.Success = healthy

	if healthy {
		// Clear error state on recovery using shared types
		m.error = nil
		m.lastError = ""
		m.healthStatus.BaseTimestampedResult.Error = ""
		m.healthStatus.HealthMetrics.LastRecoveryTime = now
	}
}

func (m *MockLLMClient) SetAnalysisResult(result *types.ActionRecommendation) {
	m.analysisResult = result
	m.error = nil
}

// Enhanced methods from integration_mocks.go
func (m *MockLLMClient) SetAnalysisResponse(response *AnalysisResponse) {
	m.analysisResponse = response
}

func (m *MockLLMClient) SetParsedResponse(response *AnalysisResponse) {
	m.parsedResponse = response
}

func (m *MockLLMClient) SetAnalysisError(err error) {
	m.analysisError = err
}

func (m *MockLLMClient) SetParseError(err error) {
	m.parseError = err
}

func (m *MockLLMClient) SetRawResponse(response string) {
	m.rawResponse = response
}

func (m *MockLLMClient) SetRateLimitScenario(retries int, delay time.Duration) {
	m.rateLimitRetries = retries
	m.rateLimitDelay = delay
	m.currentRetry = 0
}

// Health monitoring methods - using structured types following guidelines
func (m *MockLLMClient) SetResponseTime(responseTime time.Duration) {
	now := time.Now()
	m.responseTime = responseTime
	m.healthStatus.ResponseTime = responseTime
	m.healthStatus.BaseEntity.UpdatedAt = now
	m.healthStatus.BaseTimestampedResult.EndTime = now
	m.healthStatus.BaseTimestampedResult.Duration = responseTime
}

func (m *MockLLMClient) GetResponseTime() time.Duration {
	return m.responseTime
}

func (m *MockLLMClient) SetEndpoint(endpoint string) {
	m.endpoint = endpoint
	m.healthStatus.ServiceEndpoint = endpoint
}

func (m *MockLLMClient) GetEndpoint() string {
	return m.endpoint
}

func (m *MockLLMClient) GetLastError() string {
	return m.lastError
}

func (m *MockLLMClient) GetFailureCount() int {
	return m.failureCount
}

func (m *MockLLMClient) SetUptime(uptime time.Duration) {
	m.uptime = uptime
	m.healthStatus.HealthMetrics.TotalUptime = uptime
}

func (m *MockLLMClient) GetUptime() time.Duration {
	return m.uptime
}

func (m *MockLLMClient) SetDowntime(downtime time.Duration) {
	m.downtime = downtime
	m.healthStatus.HealthMetrics.TotalDowntime = downtime
	// Recalculate uptime percentage
	total := m.uptime + m.downtime
	if total > 0 {
		m.uptimePercentage = float64(m.uptime) / float64(total) * 100
		m.healthStatus.HealthMetrics.UptimePercentage = m.uptimePercentage
	}
}

func (m *MockLLMClient) GetUptimePercentage() float64 {
	return m.uptimePercentage
}

func (m *MockLLMClient) SetAccuracyRate(rate float64) {
	m.accuracyRate = rate
	m.healthStatus.HealthMetrics.AccuracyRate = rate
}

func (m *MockLLMClient) GetAccuracyRate() float64 {
	return m.accuracyRate
}

func (m *MockLLMClient) GetDowntimeEvents() int {
	return m.downtimeEvents
}

func (m *MockLLMClient) GetHealthStatus() types.HealthStatus {
	now := time.Now()
	m.healthStatus.IsHealthy = m.healthy
	m.healthStatus.BaseEntity.UpdatedAt = now
	m.healthStatus.BaseTimestampedResult.EndTime = now
	m.healthStatus.BaseTimestampedResult.Success = m.healthy

	if !m.healthy && m.lastError != "" {
		m.healthStatus.BaseTimestampedResult.Error = m.lastError
	} else {
		m.healthStatus.BaseTimestampedResult.Error = ""
	}

	// Update health metrics
	m.healthStatus.HealthMetrics.UptimePercentage = m.uptimePercentage
	m.healthStatus.HealthMetrics.TotalUptime = m.uptime
	m.healthStatus.HealthMetrics.TotalDowntime = m.downtime
	m.healthStatus.HealthMetrics.FailureCount = m.failureCount
	m.healthStatus.HealthMetrics.DowntimeEvents = m.downtimeEvents
	m.healthStatus.HealthMetrics.AccuracyRate = m.accuracyRate

	return m.healthStatus
}

func (m *MockLLMClient) ClearState() {
	m.error = nil
	m.lastError = ""
	m.failureCount = 0
	m.healthy = true
	m.healthStatus.IsHealthy = true
	m.healthStatus.BaseTimestampedResult.Error = ""
	m.healthStatus.BaseTimestampedResult.Success = true
}

func (m *MockLLMClient) AnalyzeAlert(ctx context.Context, alert interface{}) (*llm.AnalyzeAlertResponse, error) {
	// Convert interface{} to types.Alert for internal processing
	var typedAlert types.Alert
	if a, ok := alert.(types.Alert); ok {
		typedAlert = a
	} else {
		return nil, fmt.Errorf("expected types.Alert, got %T", alert)
	}
	// Add test context logging if available
	if t, ok := ctx.Value("testing.T").(*testing.T); ok {
		t.Logf("MockLLMClient.AnalyzeAlert called with alert: %s (severity: %s)", typedAlert.Name, typedAlert.Severity)
	}

	// Simulate context cancellation in tests
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		// Continue with mock behavior
	}

	// Simulate processing delay for timeout testing
	if delay, ok := ctx.Value("mock.delay").(time.Duration); ok {
		select {
		case <-time.After(delay):
			// Continue
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Track request for context enrichment testing
	m.lastAnalyzeAlertRequest = &typedAlert
	m.analyzeAlertHistory = append(m.analyzeAlertHistory, typedAlert)

	if m.error != nil {
		return nil, m.error
	}

	// Check for configured analysis response first (from SetAnalysisResponse)
	if m.analysisResponse != nil {
		// Convert AnalysisResponse to AnalyzeAlertResponse
		metadata := make(map[string]interface{})
		if m.analysisResponse.Metadata != nil {
			metadata = m.analysisResponse.Metadata
		}
		metadata["reasoning"] = m.analysisResponse.Reasoning

		return &llm.AnalyzeAlertResponse{
			Action:     m.analysisResponse.RecommendedAction,
			Confidence: m.analysisResponse.Confidence,
			Metadata:   metadata,
		}, nil
	}

	if m.analysisResult != nil {
		// Convert ActionRecommendation to AnalyzeAlertResponse
		return &llm.AnalyzeAlertResponse{
			Action:     m.analysisResult.Action,
			Confidence: m.analysisResult.Confidence,
			Reasoning:  m.analysisResult.Reasoning,
			Parameters: m.analysisResult.Parameters,
		}, nil
	}

	// BR-LLM-013: Business requirement compliance - Calculate confidence based on business scenarios
	alertStr := fmt.Sprintf("%s %s %s", typedAlert.Name, typedAlert.Severity, typedAlert.Description)
	alertLower := strings.ToLower(alertStr)

	// Business scenario confidence mapping to meet BR-LLM-013 quality thresholds
	var confidence float64
	var complexity string
	var expectedReduction float64
	var expectedTokens int

	// Determine complexity and optimization parameters based on alert characteristics
	switch {
	case strings.Contains(alertLower, "critical") || strings.Contains(alertLower, "security") || strings.Contains(alertLower, "breach"):
		confidence = 0.86
		complexity = "critical"
		expectedReduction = 0.20
		expectedTokens = 104800
	case strings.Contains(alertLower, "network") || strings.Contains(alertLower, "partition") || strings.Contains(alertLower, "complex"):
		confidence = 0.81
		complexity = "complex"
		expectedReduction = 0.40
		expectedTokens = 78600
	case strings.Contains(alertLower, "memory") || strings.Contains(alertLower, "cpu") || strings.Contains(alertLower, "moderate"):
		confidence = 0.81
		complexity = "moderate"
		expectedReduction = 0.40
		expectedTokens = 78600
	case strings.Contains(alertLower, "disk") || strings.Contains(alertLower, "warning") || strings.Contains(alertLower, "simple"):
		confidence = 0.67
		complexity = "simple"
		expectedReduction = 0.80
		expectedTokens = 26200
	default:
		confidence = 0.8
		complexity = "moderate"
		expectedReduction = 0.40
		expectedTokens = 78600
	}

	// Create comprehensive metadata for context optimization tests
	metadata := map[string]interface{}{
		"context_optimization": map[string]interface{}{
			"strategy":             "graduated_reduction",
			"complexity_tier":      complexity,
			"reduction_percentage": expectedReduction,
			"token_limit":          expectedTokens,
			"context_types":        map[string]int{"simple": 1, "moderate": 2, "complex": 3, "critical": 4}[complexity],
		},
		"performance_integration": map[string]interface{}{
			"monitoring_enabled":    true,
			"correlation_tracking":  true,
			"degradation_threshold": 0.15,
			"auto_adjustment":       true,
		},
		"automatic_adjustment": map[string]interface{}{
			"enabled":              true,
			"adjustment_threshold": 0.15,
			"response_time_target": "500ms",
			"quality_maintenance":  true,
		},
	}

	return &llm.AnalyzeAlertResponse{
		Action:     "mock_action",
		Confidence: confidence,
		Reasoning:  &types.ReasoningDetails{Summary: "Mock analysis result with context optimization"},
		Parameters: make(map[string]interface{}),
		Metadata:   metadata,
	}, nil
}

func (m *MockLLMClient) ChatCompletion(ctx context.Context, prompt string) (string, error) {
	// Add test context logging if available
	if t, ok := ctx.Value("testing.T").(*testing.T); ok {
		t.Logf("MockLLMClient.ChatCompletion called with prompt length: %d", len(prompt))
	}

	// Simulate context cancellation in tests
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
		// Continue with mock behavior
	}

	// Simulate processing delay for timeout testing
	if delay, ok := ctx.Value("mock.delay").(time.Duration); ok {
		select {
		case <-time.After(delay):
			// Continue
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	if m.error != nil {
		return "", m.error
	}

	if m.chatResponse != "" {
		return m.chatResponse, nil
	}

	return `{"action": "default_action", "confidence": 0.7, "reasoning": "Mock response"}`, nil
}

func (m *MockLLMClient) GenerateResponse(prompt string) (string, error) {
	// Delegate to ChatCompletion for consistency
	ctx := context.Background()
	return m.ChatCompletion(ctx, prompt)
}

func (m *MockLLMClient) GenerateWorkflow(ctx context.Context, objective *llm.WorkflowObjective) (*llm.WorkflowGenerationResult, error) {
	if m.error != nil {
		return nil, m.error
	}

	if m.workflowResponse != nil {
		// Convert engine.AIWorkflowResponse to llm.WorkflowGenerationResult
		return &llm.WorkflowGenerationResult{
			WorkflowID:  fmt.Sprintf("workflow-%s", objective.ID),
			Name:        fmt.Sprintf("Mock Workflow for %s", objective.Type),
			Description: fmt.Sprintf("Mock workflow for %s objective", objective.Description),
			Steps:       []*llm.AIGeneratedStep{},
			Variables:   make(map[string]interface{}),
			Confidence:  0.8,
			Reasoning:   "Mock workflow generation",
		}, nil
	}

	return &llm.WorkflowGenerationResult{
		WorkflowID:  fmt.Sprintf("mock-workflow-%s", objective.ID),
		Name:        fmt.Sprintf("Mock Workflow for %s", objective.Type),
		Description: fmt.Sprintf("Mock workflow for %s objective", objective.Description),
		Steps: []*llm.AIGeneratedStep{
			{
				ID:   "mock-step-1",
				Name: "Mock Step",
				Type: "action",
				Action: &llm.AIStepAction{
					Type:       "mock_action",
					Parameters: make(map[string]interface{}),
				},
			},
		},
		Variables:  make(map[string]interface{}),
		Confidence: 0.8,
		Reasoning:  "Mock workflow generation",
	}, nil
}

func (m *MockLLMClient) IsHealthy() bool {
	return m.healthy
}

// Health monitoring methods implementation for MockLLMClient
// BR-HEALTH-002: MUST provide liveness and readiness probes for Kubernetes

// LivenessCheck simulates liveness probe functionality
func (m *MockLLMClient) LivenessCheck(ctx context.Context) error {
	if m.error != nil {
		return m.error
	}
	if !m.healthy {
		return fmt.Errorf("mock LLM client is not healthy")
	}
	return nil
}

// ReadinessCheck simulates readiness probe functionality
func (m *MockLLMClient) ReadinessCheck(ctx context.Context) error {
	if m.error != nil {
		return m.error
	}
	if !m.healthy {
		return fmt.Errorf("mock LLM client is not ready")
	}
	// Simulate readiness check delay
	time.Sleep(m.responseTime)
	return nil
}

// GetModel returns the configured model name
func (m *MockLLMClient) GetModel() string {
	return "ggml-org/gpt-oss-20b-GGUF" // 20B+ model for testing
}

// GetMinParameterCount returns the minimum parameter count
func (m *MockLLMClient) GetMinParameterCount() int64 {
	return 20000000000 // 20B parameters
}

func (m *MockLLMClient) GetModelInfo() string {
	return "test-model"
}

// GetLastAnalyzeAlertRequest returns the last alert analyzed (for context enrichment testing)
func (m *MockLLMClient) GetLastAnalyzeAlertRequest() *types.Alert {
	return m.lastAnalyzeAlertRequest
}

// ClearHistory clears request history for test isolation
func (m *MockLLMClient) ClearHistory() {
	m.lastAnalyzeAlertRequest = nil
	m.analyzeAlertHistory = make([]types.Alert, 0)
	m.analysisResult = nil
	m.chatResponse = ""
	m.analysisResponse = nil
	m.parsedResponse = nil
	m.rawResponse = ""
	m.currentRetry = 0
}

// Enhanced methods for improved LLM testing
func (m *MockLLMClient) AnalyzeAlertWithModel(ctx context.Context, alert *types.Alert, model string) (*AnalysisResponse, error) {
	// Simulate rate limiting if configured
	if m.rateLimitRetries > 0 && m.currentRetry < m.rateLimitRetries {
		m.currentRetry++
		time.Sleep(m.rateLimitDelay)
	}

	// Track request for context enrichment testing
	m.lastAnalyzeAlertRequest = alert
	m.analyzeAlertHistory = append(m.analyzeAlertHistory, *alert)

	if m.analysisError != nil {
		return nil, m.analysisError
	}

	// Add rate limiting metadata if simulated
	if m.rateLimitRetries > 0 && m.analysisResponse != nil {
		if m.analysisResponse.Metadata == nil {
			m.analysisResponse.Metadata = make(map[string]interface{})
		}
		m.analysisResponse.Metadata["retries_attempted"] = m.currentRetry
		m.analysisResponse.Metadata["rate_limit_handled"] = true
	}

	return m.analysisResponse, nil
}

func (m *MockLLMClient) ParseResponse(ctx context.Context, rawResponse string) (*AnalysisResponse, error) {
	if m.parseError != nil {
		return nil, m.parseError
	}
	return m.parsedResponse, nil
}

// AnalysisResponse represents an AI analysis response
type AnalysisResponse struct {
	RecommendedAction string                 `json:"recommended_action"`
	Confidence        float64                `json:"confidence"`
	Reasoning         string                 `json:"reasoning"`
	ProcessingTime    time.Duration          `json:"processing_time"`
	Metadata          map[string]interface{} `json:"metadata"`
}

// GenerateWorkflowFromObjective provides backward compatibility for engine objectives
func (m *MockLLMClient) GenerateWorkflowFromObjective(ctx context.Context, objective *engine.WorkflowObjective) (*engine.AIWorkflowResponse, error) {
	if m.error != nil {
		return nil, m.error
	}

	if m.workflowResponse != nil {
		return m.workflowResponse, nil
	}

	// Return basic mock result for testing
	return &engine.AIWorkflowResponse{}, nil
}

// Enhanced AI methods for Rule 12 compliance - mock implementations
func (m *MockLLMClient) EvaluateCondition(ctx context.Context, condition interface{}, context interface{}) (bool, error) {
	return true, m.error // Default to true for test success
}

func (m *MockLLMClient) ValidateCondition(ctx context.Context, condition interface{}) error {
	return m.error
}

func (m *MockLLMClient) CollectMetrics(ctx context.Context, execution interface{}) (map[string]float64, error) {
	return map[string]float64{"mock_metric": 1.0}, m.error
}

func (m *MockLLMClient) GetAggregatedMetrics(ctx context.Context, workflowID string, timeRange interface{}) (map[string]float64, error) {
	return map[string]float64{"aggregate_metric": 0.8}, m.error
}

func (m *MockLLMClient) RecordAIRequest(ctx context.Context, requestID string, prompt string, response string) error {
	return m.error
}

func (m *MockLLMClient) RegisterPromptVersion(ctx context.Context, version interface{}) error {
	return m.error
}

func (m *MockLLMClient) GetOptimalPrompt(ctx context.Context, objective interface{}) (interface{}, error) {
	return "mock_prompt", m.error
}

func (m *MockLLMClient) StartABTest(ctx context.Context, experiment interface{}) error {
	return m.error
}

func (m *MockLLMClient) OptimizeWorkflow(ctx context.Context, workflow interface{}, executionHistory interface{}) (interface{}, error) {
	return workflow, m.error // Return the same workflow for testing
}

func (m *MockLLMClient) SuggestOptimizations(ctx context.Context, workflow interface{}) (interface{}, error) {
	return []string{"mock_optimization_1", "mock_optimization_2"}, m.error
}

func (m *MockLLMClient) BuildPrompt(ctx context.Context, template string, context map[string]interface{}) (string, error) {
	return template + "_enhanced", m.error
}

func (m *MockLLMClient) LearnFromExecution(ctx context.Context, execution interface{}) error {
	return m.error
}

func (m *MockLLMClient) GetOptimizedTemplate(ctx context.Context, templateID string) (string, error) {
	return templateID + "_optimized", m.error
}

func (m *MockLLMClient) AnalyzePatterns(ctx context.Context, executionData []interface{}) (interface{}, error) {
	return map[string]interface{}{"patterns": []string{"pattern1", "pattern2"}}, m.error
}

func (m *MockLLMClient) PredictEffectiveness(ctx context.Context, workflow interface{}) (float64, error) {
	return 0.8, m.error
}

func (m *MockLLMClient) ClusterWorkflows(ctx context.Context, executionData []interface{}, config map[string]interface{}) (interface{}, error) {
	return []interface{}{}, m.error
}

func (m *MockLLMClient) AnalyzeTrends(ctx context.Context, executionData []interface{}, timeRange interface{}) (interface{}, error) {
	return map[string]interface{}{"trend": "upward"}, m.error
}

func (m *MockLLMClient) DetectAnomalies(ctx context.Context, executionData []interface{}) (interface{}, error) {
	return []interface{}{}, m.error
}

// MockWorkflowVectorDatabase provides a mock implementation of vector.VectorDatabase for testing
type MockWorkflowVectorDatabase struct {
	actionPatterns []*vector.ActionPattern
	error          error
	UpdateCalled   bool
	StoreCalled    bool
}

func NewMockWorkflowVectorDatabase() *MockWorkflowVectorDatabase {
	return &MockWorkflowVectorDatabase{
		actionPatterns: make([]*vector.ActionPattern, 0),
	}
}

func (m *MockWorkflowVectorDatabase) SetActionPatterns(patterns []*vector.ActionPattern) {
	m.actionPatterns = patterns
	m.error = nil
}

func (m *MockWorkflowVectorDatabase) SetError(errMsg string) {
	m.error = errors.New(errMsg)
}

func (m *MockWorkflowVectorDatabase) StoreActionPattern(ctx context.Context, pattern *vector.ActionPattern) error {
	if m.error != nil {
		return m.error
	}

	m.StoreCalled = true
	m.actionPatterns = append(m.actionPatterns, pattern)
	return nil
}

func (m *MockWorkflowVectorDatabase) SearchSimilarPatterns(ctx context.Context, query *vector.VectorSearchQuery) (*vector.PatternSearchResultSet, error) {
	if m.error != nil {
		return nil, m.error
	}

	// Return mock search result
	similarPatterns := make([]*vector.UnifiedSearchResult, 0)
	for _, pattern := range m.actionPatterns {
		similarPatterns = append(similarPatterns, &vector.UnifiedSearchResult{
			ID:    pattern.ID,
			Score: 0.85,
			Metadata: map[string]interface{}{
				"pattern": pattern,
			},
		})
	}

	return &vector.PatternSearchResultSet{
		UnifiedSearchResultSet: vector.UnifiedSearchResultSet{
			TotalCount: len(similarPatterns),
			SearchTime: 50 * time.Millisecond,
		},
		Patterns: similarPatterns,
	}, nil
}

func (m *MockWorkflowVectorDatabase) FindSimilarPatterns(ctx context.Context, pattern *vector.ActionPattern, limit int, threshold float64) ([]*vector.UnifiedSearchResult, error) {
	if m.error != nil {
		return nil, m.error
	}

	// Return similar patterns based on stored patterns
	similarPatterns := make([]*vector.UnifiedSearchResult, 0)
	for _, storedPattern := range m.actionPatterns {
		if len(similarPatterns) >= limit {
			break
		}
		similarPatterns = append(similarPatterns, &vector.UnifiedSearchResult{
			ID:    storedPattern.ID,
			Score: 0.85,
			Metadata: map[string]interface{}{
				"pattern": storedPattern,
			},
		})
	}

	return similarPatterns, nil
}

func (m *MockWorkflowVectorDatabase) UpdatePatternEffectiveness(ctx context.Context, patternID string, effectiveness float64) error {
	if m.error != nil {
		return m.error
	}

	m.UpdateCalled = true

	// Find and update pattern
	for _, pattern := range m.actionPatterns {
		if pattern.ID == patternID {
			if pattern.EffectivenessData == nil {
				pattern.EffectivenessData = &vector.EffectivenessData{}
			}
			// Update the effectiveness data with the new effectiveness value
			pattern.EffectivenessData.SuccessCount = int(effectiveness * 10)
			pattern.EffectivenessData.AverageExecutionTime = time.Duration(int64(effectiveness*100)) * time.Millisecond
			pattern.UpdatedAt = time.Now()
			break
		}
	}

	return nil
}

func (m *MockWorkflowVectorDatabase) SearchBySemantics(ctx context.Context, query string, limit int) ([]*vector.ActionPattern, error) {
	if m.error != nil {
		return nil, m.error
	}

	// Return up to 'limit' patterns
	result := make([]*vector.ActionPattern, 0)
	count := 0
	for _, pattern := range m.actionPatterns {
		if count >= limit {
			break
		}
		result = append(result, pattern)
		count++
	}

	return result, nil
}

func (m *MockWorkflowVectorDatabase) SearchByVector(ctx context.Context, embedding []float64, limit int, threshold float64) ([]*vector.ActionPattern, error) {
	if m.error != nil {
		return nil, m.error
	}

	// Return up to 'limit' patterns (similar to SearchBySemantics)
	result := make([]*vector.ActionPattern, 0)
	count := 0
	for _, pattern := range m.actionPatterns {
		if count >= limit {
			break
		}
		result = append(result, pattern)
		count++
	}

	return result, nil
}

func (m *MockWorkflowVectorDatabase) DeletePattern(ctx context.Context, patternID string) error {
	if m.error != nil {
		return m.error
	}

	// Remove pattern from mock storage
	for i, pattern := range m.actionPatterns {
		if pattern.ID == patternID {
			m.actionPatterns = append(m.actionPatterns[:i], m.actionPatterns[i+1:]...)
			break
		}
	}

	return nil
}

func (m *MockWorkflowVectorDatabase) GetPatternAnalytics(ctx context.Context) (*vector.PatternAnalytics, error) {
	if m.error != nil {
		return nil, m.error
	}

	return &vector.PatternAnalytics{
		TotalPatterns:             len(m.actionPatterns),
		PatternsByActionType:      map[string]int{"mock_action": len(m.actionPatterns)},
		PatternsBySeverity:        map[string]int{"warning": len(m.actionPatterns)},
		AverageEffectiveness:      0.8,
		TopPerformingPatterns:     []*vector.ActionPattern{},
		RecentPatterns:            m.actionPatterns,
		EffectivenessDistribution: map[string]int{"high": len(m.actionPatterns)},
		GeneratedAt:               time.Now(),
	}, nil
}

func (m *MockWorkflowVectorDatabase) IsHealthy(ctx context.Context) error {
	if m.error != nil {
		return m.error
	}
	return nil
}

func (m *MockWorkflowVectorDatabase) GetStoredPatterns() []*vector.ActionPattern {
	return m.actionPatterns
}

// MockPatternExtractor provides a mock implementation of vector.PatternExtractor for testing
type MockPatternExtractor struct {
	ExtractCalled bool
	error         error
}

func NewMockPatternExtractor() *MockPatternExtractor {
	return &MockPatternExtractor{}
}

func (m *MockPatternExtractor) SetError(errMsg string) {
	m.error = errors.New(errMsg)
}

func (m *MockPatternExtractor) ExtractPatterns(ctx context.Context, executions []*engine.RuntimeWorkflowExecution) ([]*vector.ActionPattern, error) {
	if m.error != nil {
		return nil, m.error
	}

	m.ExtractCalled = true

	// Return mock patterns based on executions
	patterns := make([]*vector.ActionPattern, 0)
	for _, exec := range executions {
		pattern := &vector.ActionPattern{
			ID:            fmt.Sprintf("pattern-%s", exec.ID),
			AlertName:     "MockAlert",
			ActionType:    "mock_action",
			AlertSeverity: "warning",
			Namespace:     "default",
			ResourceType:  "deployment",
			ResourceName:  "mock-resource",
			ActionParameters: map[string]interface{}{
				"mock_param": "mock_value",
			},
			ContextLabels: map[string]string{
				"app": "mock",
			},
			PreConditions:  map[string]interface{}{"mock_condition": true},
			PostConditions: map[string]interface{}{"mock_result": true},
			EffectivenessData: &vector.EffectivenessData{
				SuccessCount:         1,
				FailureCount:         0,
				AverageExecutionTime: 120.0,
				SideEffectsCount:     0,
				RecurrenceRate:       0.1,
				ContextualFactors:    map[string]float64{"mock_factor": 0.8},
				LastAssessed:         time.Now(),
			},
			Embedding: make([]float64, 128),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

func (m *MockPatternExtractor) ExtractPattern(ctx context.Context, trace *actionhistory.ResourceActionTrace) (*vector.ActionPattern, error) {
	if m.error != nil {
		return nil, m.error
	}

	m.ExtractCalled = true

	return &vector.ActionPattern{
		ID:            fmt.Sprintf("pattern-%d", trace.ID),
		AlertName:     "MockAlert",
		ActionType:    "mock_action",
		AlertSeverity: "warning",
		Namespace:     "default",
		ResourceType:  "deployment",
		ResourceName:  "mock-resource",
		ActionParameters: map[string]interface{}{
			"mock_param": "mock_value",
		},
		ContextLabels: map[string]string{
			"app": "mock",
		},
		PreConditions:  map[string]interface{}{"mock_condition": true},
		PostConditions: map[string]interface{}{"mock_result": true},
		Embedding:      make([]float64, 128),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}, nil
}

func (m *MockPatternExtractor) GenerateEmbedding(ctx context.Context, pattern *vector.ActionPattern) ([]float64, error) {
	if m.error != nil {
		return nil, m.error
	}

	// Return mock embedding
	embedding := make([]float64, 128)
	for i := range embedding {
		embedding[i] = 0.1 * float64(i)
	}

	return embedding, nil
}

func (m *MockPatternExtractor) ExtractFeatures(ctx context.Context, pattern *vector.ActionPattern) (map[string]interface{}, error) {
	if m.error != nil {
		return nil, m.error
	}

	return map[string]interface{}{
		"action_type":    pattern.ActionType,
		"resource_type":  pattern.ResourceType,
		"alert_severity": pattern.AlertSeverity,
		"namespace":      pattern.Namespace,
	}, nil
}

func (m *MockPatternExtractor) CalculateSimilarity(pattern1, pattern2 *vector.ActionPattern) float64 {
	// Simple mock similarity calculation
	if pattern1.ActionType == pattern2.ActionType && pattern1.ResourceType == pattern2.ResourceType {
		return 0.9
	}
	return 0.3
}

// WorkflowExecutionRepositoryMock provides a mock implementation of workflow ExecutionRepository for testing
// This mock implements the workflow engine ExecutionRepository interface
type WorkflowExecutionRepositoryMock struct {
	executions           []*engine.RuntimeWorkflowExecution
	error                error
	StoreExecutionCalled bool
}

func NewWorkflowExecutionRepositoryMock() *WorkflowExecutionRepositoryMock {
	return &WorkflowExecutionRepositoryMock{
		executions: make([]*engine.RuntimeWorkflowExecution, 0),
	}
}

func (m *WorkflowExecutionRepositoryMock) SetExecutions(executions []*engine.RuntimeWorkflowExecution) {
	m.executions = executions
	m.error = nil
}

func (m *WorkflowExecutionRepositoryMock) SetError(errMsg string) {
	m.error = errors.New(errMsg)
}

func (m *WorkflowExecutionRepositoryMock) StoreExecution(ctx context.Context, execution *engine.RuntimeWorkflowExecution) error {
	if m.error != nil {
		return m.error
	}

	m.StoreExecutionCalled = true
	m.executions = append(m.executions, execution)
	return nil
}

func (m *WorkflowExecutionRepositoryMock) GetExecution(ctx context.Context, executionID string) (*engine.RuntimeWorkflowExecution, error) {
	if m.error != nil {
		return nil, m.error
	}

	for _, exec := range m.executions {
		if exec.ID == executionID {
			return exec, nil
		}
	}

	return nil, fmt.Errorf("execution %s not found", executionID)
}

func (m *WorkflowExecutionRepositoryMock) GetWorkflowExecutions(ctx context.Context, workflowID string, limit int) ([]*engine.RuntimeWorkflowExecution, error) {
	if m.error != nil {
		return nil, m.error
	}

	result := make([]*engine.RuntimeWorkflowExecution, 0)
	count := 0

	for _, exec := range m.executions {
		if count >= limit {
			break
		}
		if exec.WorkflowID == workflowID {
			result = append(result, exec)
			count++
		}
	}

	return result, nil
}

func (m *WorkflowExecutionRepositoryMock) GetRecentExecutions(ctx context.Context, since time.Time, limit int) ([]*engine.RuntimeWorkflowExecution, error) {
	if m.error != nil {
		return nil, m.error
	}

	result := make([]*engine.RuntimeWorkflowExecution, 0)
	count := 0

	for _, exec := range m.executions {
		if count >= limit {
			break
		}
		if exec.StartTime.After(since) {
			result = append(result, exec)
			count++
		}
	}

	return result, nil
}

func (m *WorkflowExecutionRepositoryMock) GetExecutionsByPattern(ctx context.Context, patternID string) ([]*engine.RuntimeWorkflowExecution, error) {
	if m.error != nil {
		return nil, m.error
	}

	// Simple pattern matching for testing - filter executions by pattern in metadata
	result := make([]*engine.RuntimeWorkflowExecution, 0)
	for _, exec := range m.executions {
		if exec.Metadata != nil {
			if pattern, exists := exec.Metadata["pattern_id"]; exists && pattern == patternID {
				result = append(result, exec)
			}
		}
	}

	return result, nil
}

func (m *WorkflowExecutionRepositoryMock) GetExecutionsInTimeWindow(ctx context.Context, start, end time.Time) ([]*engine.RuntimeWorkflowExecution, error) {
	if m.error != nil {
		return nil, m.error
	}

	result := make([]*engine.RuntimeWorkflowExecution, 0)
	for _, exec := range m.executions {
		if exec.StartTime.After(start) && exec.StartTime.Before(end) {
			result = append(result, exec)
		}
	}

	return result, nil
}

func (m *WorkflowExecutionRepositoryMock) GetExecutionsByWorkflowID(ctx context.Context, workflowID string) ([]*engine.RuntimeWorkflowExecution, error) {
	if m.error != nil {
		return nil, m.error
	}

	result := make([]*engine.RuntimeWorkflowExecution, 0)
	for _, exec := range m.executions {
		if exec.WorkflowID == workflowID {
			result = append(result, exec)
		}
	}

	return result, nil
}

func (m *WorkflowExecutionRepositoryMock) UpdateExecutionStatus(ctx context.Context, executionID string, status engine.ExecutionStatus, errorMsg string) error {
	if m.error != nil {
		return m.error
	}

	for _, exec := range m.executions {
		if exec.ID == executionID {
			exec.OperationalStatus = status
			if errorMsg != "" {
				if exec.Context == nil {
					exec.Context = &engine.ExecutionContext{
						Variables: make(map[string]interface{}),
					}
				}
				exec.Context.Variables["error"] = errorMsg
			}
			break
		}
	}

	return nil
}

func (m *WorkflowExecutionRepositoryMock) DeleteOldExecutions(ctx context.Context, before time.Time) (int, error) {
	if m.error != nil {
		return 0, m.error
	}

	originalCount := len(m.executions)
	filtered := make([]*engine.RuntimeWorkflowExecution, 0)

	for _, exec := range m.executions {
		if exec.StartTime.After(before) {
			filtered = append(filtered, exec)
		}
	}

	m.executions = filtered
	return originalCount - len(filtered), nil
}

// MockActionRepository provides a mock implementation of actionhistory.Repository for testing
type MockActionRepository struct {
	mu                     sync.RWMutex
	resourceRefs           map[string]*actionhistory.ResourceReference // key: namespace:kind:name
	resourceRefID          int64
	actionHistories        map[int64]*actionhistory.ActionHistory
	actionTraces           map[string]*actionhistory.ResourceActionTrace
	oscillationPatterns    map[string][]actionhistory.OscillationPattern
	oscillationDetections  map[int64][]actionhistory.OscillationDetection
	actionHistorySummaries []actionhistory.ActionHistorySummary
	error                  error
}

func NewMockActionRepository() *MockActionRepository {
	return &MockActionRepository{
		resourceRefs:          make(map[string]*actionhistory.ResourceReference),
		resourceRefID:         1,
		actionHistories:       make(map[int64]*actionhistory.ActionHistory),
		actionTraces:          make(map[string]*actionhistory.ResourceActionTrace),
		oscillationPatterns:   make(map[string][]actionhistory.OscillationPattern),
		oscillationDetections: make(map[int64][]actionhistory.OscillationDetection),
	}
}

func (m *MockActionRepository) SetError(errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.error = errors.New(errMsg)
}

func (m *MockActionRepository) EnsureResourceReference(ctx context.Context, ref actionhistory.ResourceReference) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.error != nil {
		return 0, m.error
	}

	key := fmt.Sprintf("%s:%s:%s", ref.Namespace, ref.Kind, ref.Name)
	if existing, exists := m.resourceRefs[key]; exists {
		return existing.ID, nil
	}

	refCopy := ref
	refCopy.ID = m.resourceRefID
	m.resourceRefs[key] = &refCopy
	m.resourceRefID++
	return refCopy.ID, nil
}

func (m *MockActionRepository) GetResourceReference(ctx context.Context, namespace, kind, name string) (*actionhistory.ResourceReference, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	key := fmt.Sprintf("%s:%s:%s", namespace, kind, name)
	if ref, exists := m.resourceRefs[key]; exists {
		refCopy := *ref
		return &refCopy, nil
	}

	return nil, fmt.Errorf("resource reference not found: %s", key)
}

func (m *MockActionRepository) EnsureActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.error != nil {
		return nil, m.error
	}

	if history, exists := m.actionHistories[resourceID]; exists {
		return history, nil
	}

	history := &actionhistory.ActionHistory{
		ID:           resourceID,
		ResourceID:   resourceID,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		TotalActions: 0,
		LastActionAt: nil,
	}
	m.actionHistories[resourceID] = history
	return history, nil
}

func (m *MockActionRepository) GetActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	if history, exists := m.actionHistories[resourceID]; exists {
		historyCopy := *history
		return &historyCopy, nil
	}

	return nil, fmt.Errorf("action history not found for resource ID: %d", resourceID)
}

func (m *MockActionRepository) UpdateActionHistory(ctx context.Context, history *actionhistory.ActionHistory) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.error != nil {
		return m.error
	}

	historyCopy := *history
	historyCopy.UpdatedAt = time.Now()
	m.actionHistories[history.ResourceID] = &historyCopy
	return nil
}

func (m *MockActionRepository) StoreAction(ctx context.Context, action *actionhistory.ActionRecord) (*actionhistory.ResourceActionTrace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.error != nil {
		return nil, m.error
	}

	trace := &actionhistory.ResourceActionTrace{
		ID:              int64(len(m.actionTraces) + 1),
		ActionID:        action.ActionType,
		ActionType:      action.ActionType,
		AlertName:       action.Alert.Name,
		ActionTimestamp: time.Now(),
	}

	m.actionTraces[action.ActionType] = trace
	return trace, nil
}

func (m *MockActionRepository) GetActionTraces(ctx context.Context, query actionhistory.ActionQuery) ([]actionhistory.ResourceActionTrace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	traces := make([]actionhistory.ResourceActionTrace, 0)
	for _, trace := range m.actionTraces {
		traceCopy := *trace
		traces = append(traces, traceCopy)
	}
	return traces, nil
}

func (m *MockActionRepository) GetActionTrace(ctx context.Context, actionID string) (*actionhistory.ResourceActionTrace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	if trace, exists := m.actionTraces[actionID]; exists {
		traceCopy := *trace
		return &traceCopy, nil
	}

	return nil, fmt.Errorf("action trace not found: %s", actionID)
}

func (m *MockActionRepository) UpdateActionTrace(ctx context.Context, trace *actionhistory.ResourceActionTrace) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.error != nil {
		return m.error
	}

	traceCopy := *trace
	m.actionTraces[trace.ActionID] = &traceCopy
	return nil
}

func (m *MockActionRepository) GetPendingEffectivenessAssessments(ctx context.Context) ([]*actionhistory.ResourceActionTrace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	traces := make([]*actionhistory.ResourceActionTrace, 0)
	for _, trace := range m.actionTraces {
		traceCopy := *trace
		traces = append(traces, &traceCopy)
	}
	return traces, nil
}

func (m *MockActionRepository) GetOscillationPatterns(ctx context.Context, patternType string) ([]actionhistory.OscillationPattern, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	if patterns, exists := m.oscillationPatterns[patternType]; exists {
		return patterns, nil
	}
	return []actionhistory.OscillationPattern{}, nil
}

func (m *MockActionRepository) StoreOscillationDetection(ctx context.Context, detection *actionhistory.OscillationDetection) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.error != nil {
		return m.error
	}

	detectionCopy := *detection
	m.oscillationDetections[detection.ResourceID] = append(m.oscillationDetections[detection.ResourceID], detectionCopy)
	return nil
}

func (m *MockActionRepository) GetOscillationDetections(ctx context.Context, resourceID int64, resolved *bool) ([]actionhistory.OscillationDetection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	if detections, exists := m.oscillationDetections[resourceID]; exists {
		return detections, nil
	}
	return []actionhistory.OscillationDetection{}, nil
}

func (m *MockActionRepository) ApplyRetention(ctx context.Context, actionHistoryID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.error != nil {
		return m.error
	}

	// Mock implementation - just acknowledge the call
	return nil
}

func (m *MockActionRepository) GetActionHistorySummaries(ctx context.Context, since time.Duration) ([]actionhistory.ActionHistorySummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	return m.actionHistorySummaries, nil
}

// Test helper methods
func (m *MockActionRepository) SetActionHistorySummaries(summaries []actionhistory.ActionHistorySummary) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actionHistorySummaries = summaries
}

// MockStateStorage provides a mock implementation of engine.StateStorage for testing
type MockStateStorage struct {
	mu        sync.RWMutex
	states    map[string]*engine.RuntimeWorkflowExecution
	SaveCount int
	error     error

	// Business Value Tracking for BR-WF-004
	stateTransitions        []StateTransition
	statePersistenceMetrics StatePersistenceMetrics
}

// Business value types for enhanced workflow testing
type StateTransition struct {
	ExecutionID    string
	FromStep       int
	ToStep         int
	TransitionTime time.Time
	StepStatus     string
}

type StatePersistenceMetrics struct {
	TotalSaves        int
	StepProgressSaves int
	CompletionSaves   int
	ErrorStateSaves   int
	RecoveryCapable   bool
	StateIntegrityOK  bool
}

func NewMockStateStorage() *MockStateStorage {
	return &MockStateStorage{
		states:           make(map[string]*engine.RuntimeWorkflowExecution),
		stateTransitions: make([]StateTransition, 0),
		statePersistenceMetrics: StatePersistenceMetrics{
			RecoveryCapable:  true,
			StateIntegrityOK: true,
		},
	}
}

func (m *MockStateStorage) SetError(errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.error = errors.New(errMsg)
}

func (m *MockStateStorage) SaveWorkflowState(ctx context.Context, execution *engine.RuntimeWorkflowExecution) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.error != nil {
		return m.error
	}

	m.SaveCount++

	// Business Value Tracking: Track state transitions for BR-WF-004
	previousState := m.states[execution.ID]
	currentStep := execution.CurrentStep

	if previousState != nil {
		// Track step progression
		transition := StateTransition{
			ExecutionID:    execution.ID,
			FromStep:       previousState.CurrentStep,
			ToStep:         currentStep,
			TransitionTime: time.Now(),
			StepStatus:     getStepStatus(execution),
		}
		m.stateTransitions = append(m.stateTransitions, transition)
	}

	// Update persistence metrics based on execution state
	m.statePersistenceMetrics.TotalSaves++
	if execution.IsCompleted() {
		m.statePersistenceMetrics.CompletionSaves++
	} else if execution.IsFailed() {
		m.statePersistenceMetrics.ErrorStateSaves++
	} else {
		m.statePersistenceMetrics.StepProgressSaves++
	}

	executionCopy := *execution
	m.states[execution.ID] = &executionCopy
	return nil
}

// Helper function to determine step status
func getStepStatus(execution *engine.RuntimeWorkflowExecution) string {
	if execution.IsCompleted() {
		return "completed"
	} else if execution.IsFailed() {
		return "failed"
	} else {
		return "in_progress"
	}
}

func (m *MockStateStorage) LoadWorkflowState(ctx context.Context, executionID string) (*engine.RuntimeWorkflowExecution, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	if state, exists := m.states[executionID]; exists {
		stateCopy := *state
		return &stateCopy, nil
	}
	return nil, fmt.Errorf("workflow state not found: %s", executionID)
}

func (m *MockStateStorage) DeleteWorkflowState(ctx context.Context, executionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.error != nil {
		return m.error
	}

	delete(m.states, executionID)
	return nil
}

// Test helper methods
func (m *MockStateStorage) StoreState(execution *engine.RuntimeWorkflowExecution) {
	m.mu.Lock()
	defer m.mu.Unlock()
	executionCopy := *execution
	m.states[execution.ID] = &executionCopy
}

func (m *MockStateStorage) GetLastSavedState() *engine.RuntimeWorkflowExecution {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, state := range m.states {
		return state // Return the last one (simplified)
	}
	return nil
}

func (m *MockStateStorage) GetSaveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.SaveCount
}

// Business Value Accessor Methods for BR-WF-004 Testing
func (m *MockStateStorage) GetStatePersistenceMetrics() StatePersistenceMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.statePersistenceMetrics
}

func (m *MockStateStorage) GetStateTransitions() []StateTransition {
	m.mu.RLock()
	defer m.mu.RUnlock()
	transitionsCopy := make([]StateTransition, len(m.stateTransitions))
	copy(transitionsCopy, m.stateTransitions)
	return transitionsCopy
}

// BR-REL-004: RecoverWorkflowStates - Mock implementation for workflow state recovery testing
func (m *MockStateStorage) RecoverWorkflowStates(ctx context.Context) ([]*engine.RuntimeWorkflowExecution, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	// Return all stored states that are recoverable (running or failed states)
	recoverable := make([]*engine.RuntimeWorkflowExecution, 0)
	for _, state := range m.states {
		// Following guideline: Test business requirements (BR-REL-004)
		// Only recover running or failed executions (not completed ones)
		if state.OperationalStatus == engine.ExecutionStatusRunning ||
			state.OperationalStatus == engine.ExecutionStatusFailed {
			stateCopy := *state
			recoverable = append(recoverable, &stateCopy)
		}
	}

	return recoverable, nil
}

// BR-REL-004: GetStateAnalytics - Mock implementation for state analytics testing
func (m *MockStateStorage) GetStateAnalytics(ctx context.Context) (*shared.StateAnalytics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	// Calculate analytics from stored states
	totalExecutions := len(m.states)
	activeExecutions := 0
	completedExecutions := 0
	failedExecutions := 0

	for _, state := range m.states {
		switch state.OperationalStatus {
		case engine.ExecutionStatusRunning:
			activeExecutions++
		case engine.ExecutionStatusCompleted:
			completedExecutions++
		case engine.ExecutionStatusFailed:
			failedExecutions++
		}
	}

	analytics := &shared.StateAnalytics{
		TotalExecutions:      totalExecutions,
		ActiveExecutions:     activeExecutions,
		CompletedExecutions:  completedExecutions,
		FailedExecutions:     failedExecutions,
		RecoverySuccessRate:  0.95, // Mock high success rate
		AverageExecutionTime: 5 * time.Minute,
		LastUpdated:          time.Now(),
	}

	return analytics, nil
}

// BR-DATA-012: CreateCheckpoint - Mock implementation for checkpoint testing
func (m *MockStateStorage) CreateCheckpoint(ctx context.Context, execution *engine.RuntimeWorkflowExecution, name string) (*shared.WorkflowCheckpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.error != nil {
		return nil, m.error
	}

	checkpoint := &shared.WorkflowCheckpoint{
		ID:          fmt.Sprintf("checkpoint-%s-%s", execution.ID, name),
		ExecutionID: execution.ID,
		WorkflowID:  execution.WorkflowID,
		Name:        name,
		StateHash:   fmt.Sprintf("hash-%s", execution.ID),
		CreatedAt:   time.Now(),
		Metadata: map[string]string{
			"current_step": fmt.Sprintf("%d", execution.CurrentStep),
			"status":       string(execution.OperationalStatus),
		},
	}

	return checkpoint, nil
}

// BR-DATA-012: RestoreFromCheckpoint - Mock implementation for checkpoint restoration
func (m *MockStateStorage) RestoreFromCheckpoint(ctx context.Context, checkpointID string) (*engine.RuntimeWorkflowExecution, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	// Mock restoration - in real implementation would load from checkpoint storage
	return &engine.RuntimeWorkflowExecution{
		WorkflowExecutionRecord: types.WorkflowExecutionRecord{
			ID:         "restored-execution",
			WorkflowID: "restored-workflow",
			Status:     string(engine.ExecutionStatusRunning),
			StartTime:  time.Now().Add(-time.Hour),
			Metadata:   map[string]interface{}{"restored_from": checkpointID},
		},
		OperationalStatus: engine.ExecutionStatusRunning,
	}, nil
}

// BR-DATA-012: ValidateCheckpoint - Mock implementation for checkpoint validation
func (m *MockStateStorage) ValidateCheckpoint(ctx context.Context, checkpointID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return false, m.error
	}

	// Mock validation - always return true for testing
	return true, nil
}

func (m *MockStateStorage) ValidateStateProgression(executionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Business Requirement: BR-WF-001 - Support both sequential and parallel execution patterns
	// Enhanced validation for modern workflow execution patterns

	executionTransitions := []StateTransition{}
	for _, transition := range m.stateTransitions {
		if transition.ExecutionID == executionID {
			executionTransitions = append(executionTransitions, transition)
		}
	}

	if len(executionTransitions) == 0 {
		return true // No transitions means valid (initial state)
	}

	// Strategy 1: Check if all transitions are valid completions
	allCompleted := true
	for _, transition := range executionTransitions {
		if transition.StepStatus != "completed" && transition.StepStatus != "in_progress" {
			allCompleted = false
			break
		}
	}

	// Strategy 2: For parallel execution, validate that steps don't conflict
	// Instead of requiring strict sequential order, ensure logical consistency
	stepStatuses := make(map[int]string)
	for _, transition := range executionTransitions {
		// Allow multiple transitions to the same step (parallel execution)
		// but ensure final status is consistent
		if existing, exists := stepStatuses[transition.ToStep]; exists {
			// If step already has a status, ensure consistency
			if existing == "completed" && transition.StepStatus != "completed" {
				return false // Can't go from completed to another state
			}
		}
		stepStatuses[transition.ToStep] = transition.StepStatus
	}

	// Strategy 3: Validate business continuity - ensure forward progress
	// For parallel execution, we expect multiple steps to progress simultaneously
	hasForwardProgress := len(stepStatuses) > 0

	return allCompleted && hasForwardProgress
}

func (m *MockStateStorage) GetStepProgressionCount(executionID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, transition := range m.stateTransitions {
		if transition.ExecutionID == executionID {
			count++
		}
	}
	return count
}

// Business value types for enhanced workflow execution testing
type ExecutionFlowMetrics struct {
	TotalStepsExecuted    int
	SequentialSteps       int
	ParallelSteps         int
	ConditionalBranches   int
	FailedSteps           int
	SuccessfulSteps       int
	ExecutionPatternValid bool
	FlowIntegrityOK       bool
}

type StepExecutionResult struct {
	StepID           string
	ActionType       string
	StartTime        time.Time
	EndTime          time.Time
	Success          bool
	ExecutionContext map[string]interface{}
	BusinessOutcome  string
}

// MockActionExecutor provides a mock implementation of engine.ActionExecutor for testing
type MockActionExecutor struct {
	mu                sync.RWMutex
	results           []mockExecutionResult
	currentCall       int
	executionDelay    time.Duration
	executionOrder    *[]string
	concurrentTracker *[]time.Time
	executedActions   []string
	error             error

	// Business Value Tracking for BR-WF-001, BR-WF-002, BR-WF-003
	executionFlowMetrics ExecutionFlowMetrics
	stepResults          []StepExecutionResult
}

type mockExecutionResult struct {
	actionType string
	success    bool
	err        error
}

func NewMockActionExecutor() *MockActionExecutor {
	return &MockActionExecutor{
		results:     make([]mockExecutionResult, 0),
		stepResults: make([]StepExecutionResult, 0),
		executionFlowMetrics: ExecutionFlowMetrics{
			ExecutionPatternValid: true,
			FlowIntegrityOK:       true,
		},
	}
}

func (m *MockActionExecutor) SetError(errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.error = errors.New(errMsg)
}

func (m *MockActionExecutor) Execute(ctx context.Context, action *engine.StepAction, stepContext *engine.StepContext) (*engine.StepResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.error != nil {
		return nil, m.error
	}

	// Track execution order for sequential testing
	if m.executionOrder != nil && stepContext != nil {
		*m.executionOrder = append(*m.executionOrder, stepContext.StepID)
	}

	// Track concurrent execution start times
	if m.concurrentTracker != nil {
		*m.concurrentTracker = append(*m.concurrentTracker, time.Now())
	}

	// Track executed actions
	if action.Type != "" {
		m.executedActions = append(m.executedActions, action.Type)
	}

	startTime := time.Now()

	// Simulate execution delay with context cancellation support
	if m.executionDelay > 0 {
		m.mu.Unlock()
		// Use select to make delay cancellable by context
		select {
		case <-ctx.Done():
			m.mu.Lock()
			return nil, ctx.Err()
		case <-time.After(m.executionDelay):
			// Continue with normal execution
		}
		m.mu.Lock()
	}

	// Return configured result
	// Guideline #14: Use idiomatic Go patterns - omit type when inferred
	var success = true
	var executionError error

	if m.currentCall < len(m.results) {
		result := m.results[m.currentCall]
		m.currentCall++
		success = result.success
		executionError = result.err
	}

	endTime := time.Now()

	// Business Value Tracking: Record step execution result
	stepResult := StepExecutionResult{
		StepID:     stepContext.StepID,
		ActionType: action.Type,
		StartTime:  startTime,
		EndTime:    endTime,
		Success:    success && executionError == nil,
		ExecutionContext: map[string]interface{}{
			"action_parameters": action.Parameters,
		},
		BusinessOutcome: m.determineBusinessOutcome(action, success, executionError),
	}
	m.stepResults = append(m.stepResults, stepResult)

	// Update execution flow metrics
	m.executionFlowMetrics.TotalStepsExecuted++
	if success && executionError == nil {
		m.executionFlowMetrics.SuccessfulSteps++
	} else {
		m.executionFlowMetrics.FailedSteps++
	}

	if executionError != nil {
		return nil, executionError
	}

	// Return engine step result with business value metadata
	engineResult := &engine.StepResult{
		Success:   success,
		Data:      map[string]interface{}{"action_type": action.Type, "business_outcome": stepResult.BusinessOutcome},
		Variables: make(map[string]interface{}),
		Metrics:   &engine.StepMetrics{Duration: endTime.Sub(startTime)},
	}

	return engineResult, nil
}

func (m *MockActionExecutor) ValidateAction(action *engine.StepAction) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return m.error
	}

	// Accept all actions as valid for testing
	return nil
}

func (m *MockActionExecutor) GetActionType() string {
	return "kubernetes"
}

// Test configuration methods
func (m *MockActionExecutor) SetExecutionResult(actionType string, success bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results = append(m.results, mockExecutionResult{
		actionType: actionType,
		success:    success,
		err:        err,
	})
}

func (m *MockActionExecutor) SetExecutionResultAtCall(callIndex int, actionType string, success bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Ensure the slice is large enough
	for len(m.results) <= callIndex {
		m.results = append(m.results, mockExecutionResult{})
	}
	m.results[callIndex] = mockExecutionResult{
		actionType: actionType,
		success:    success,
		err:        err,
	}
}

func (m *MockActionExecutor) SetExecutionDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executionDelay = delay
}

func (m *MockActionExecutor) SetExecutionOrderCapture(orderSlice *[]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executionOrder = orderSlice
}

func (m *MockActionExecutor) SetConcurrentExecutionTracker(timeSlice *[]time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.concurrentTracker = timeSlice
}

func (m *MockActionExecutor) GetExecutedActions() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	actionsCopy := make([]string, len(m.executedActions))
	copy(actionsCopy, m.executedActions)
	return actionsCopy
}

// Helper method to determine business outcome based on action type and result
func (m *MockActionExecutor) determineBusinessOutcome(action *engine.StepAction, success bool, err error) string {
	if err != nil {
		return "execution_error"
	}
	if !success {
		return "action_failed"
	}

	// Determine business outcome based on action type
	switch action.Type {
	case "kubernetes":
		return "resource_action_completed"
	case "monitor", "monitor-resource":
		return "monitoring_action_completed"
	case "remediation", "immediate-action":
		return "remediation_action_completed"
	default:
		return "generic_action_completed"
	}
}

// Business Value Accessor Methods for BR-WF-001, BR-WF-002, BR-WF-003 Testing
func (m *MockActionExecutor) GetExecutionFlowMetrics() ExecutionFlowMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.executionFlowMetrics
}

func (m *MockActionExecutor) GetStepResults() []StepExecutionResult {
	m.mu.RLock()
	defer m.mu.RUnlock()
	resultsCopy := make([]StepExecutionResult, len(m.stepResults))
	copy(resultsCopy, m.stepResults)
	return resultsCopy
}

func (m *MockActionExecutor) GetExecutionPattern() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.stepResults) <= 1 {
		return "single_step"
	}

	// Analyze execution timing to determine pattern
	timeDifferences := make([]time.Duration, 0)
	for i := 1; i < len(m.stepResults); i++ {
		diff := m.stepResults[i].StartTime.Sub(m.stepResults[i-1].StartTime)
		timeDifferences = append(timeDifferences, diff)
	}

	// If most steps start within 50ms of each other, it's parallel
	parallelCount := 0
	for _, diff := range timeDifferences {
		if diff < 50*time.Millisecond {
			parallelCount++
		}
	}

	if parallelCount > len(timeDifferences)/2 {
		return "parallel_execution"
	}
	return "sequential_execution"
}

func (m *MockActionExecutor) ValidateStepSequence() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check if step execution follows logical workflow progression
	for i := 1; i < len(m.stepResults); i++ {
		// Each step should start after the previous step started
		if m.stepResults[i].StartTime.Before(m.stepResults[i-1].StartTime) {
			return false
		}
	}
	return true
}

// @deprecated RULE 12 VIOLATION: Mocks deprecated AIConditionEvaluator interface instead of using enhanced llm.Client
// Migration: Use enhanced llm.Client.EvaluateCondition(), llm.Client.ValidateCondition() methods directly in tests
// Business Requirements: BR-COND-001, BR-COND-005 - now served by enhanced llm.Client
//
// Replacement pattern:
//
//	Instead of: mockEvaluator := NewMockAIConditionEvaluator()
//	Use: llmClient (already has enhanced EvaluateCondition, ValidateCondition methods) with test responses
//
// MockAIConditionEvaluator provides a mock implementation of engine.AIConditionEvaluator for testing
type MockAIConditionEvaluator struct {
	mu               sync.RWMutex
	conditionResults map[string]mockConditionResult
	decisionResults  map[string]mockDecisionResult
	error            error
}

type mockConditionResult struct {
	result bool
	err    error
}

type mockDecisionResult struct {
	outcome string
	err     error
}

// @deprecated RULE 12 VIOLATION: Use enhanced llm.Client instead
// Migration: Use llmClient.EvaluateCondition() and llmClient.ValidateCondition() directly
func NewMockAIConditionEvaluator() *MockAIConditionEvaluator {
	return &MockAIConditionEvaluator{
		conditionResults: make(map[string]mockConditionResult),
		decisionResults:  make(map[string]mockDecisionResult),
	}
}

func (m *MockAIConditionEvaluator) SetError(errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.error = errors.New(errMsg)
}

func (m *MockAIConditionEvaluator) EvaluateCondition(ctx context.Context, condition *engine.ExecutableCondition, context *engine.StepContext) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return false, m.error
	}

	if result, exists := m.conditionResults[condition.Name]; exists {
		return result.result, result.err
	}
	return true, nil // Default to true
}

func (m *MockAIConditionEvaluator) ValidateCondition(ctx context.Context, condition *engine.ExecutableCondition) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return m.error
	}

	// Accept all conditions as valid for testing
	return nil
}

// Test configuration methods
func (m *MockAIConditionEvaluator) SetConditionResult(conditionName string, result bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conditionResults[conditionName] = mockConditionResult{
		result: result,
		err:    err,
	}
}

func (m *MockAIConditionEvaluator) SetDecisionResult(decisionName string, outcome string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decisionResults[decisionName] = mockDecisionResult{
		outcome: outcome,
		err:     err,
	}
}

// MockKubernetesClient provides a mock implementation of k8s.Client for testing
// Note: Only implements methods needed for workflow engine testing
type MockKubernetesClient struct {
	mu            sync.RWMutex
	error         error
	pods          map[string]*corev1.Pod // key: namespace/podName
	podOperations []string               // track operations for testing
	restartedPods map[string]bool        // track restarted pods
	deletedPods   map[string]bool        // track deleted pods
	deployments   map[string]*appsv1.Deployment
	deploymentOps []string
}

func NewMockKubernetesClient() *MockKubernetesClient {
	return &MockKubernetesClient{
		pods:          make(map[string]*corev1.Pod),
		podOperations: make([]string, 0),
		restartedPods: make(map[string]bool),
		deletedPods:   make(map[string]bool),
		deployments:   make(map[string]*appsv1.Deployment),
		deploymentOps: make([]string, 0),
	}
}

// AsK8sClient returns the mock as a k8s.Client interface (for testing purposes only)
func (m *MockKubernetesClient) AsK8sClient() k8s.Client {
	return m
}

func (m *MockKubernetesClient) SetError(errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.error = errors.New(errMsg)
}

// BasicClient methods - removed old stub implementations
// (New implementations are provided below with proper pod restart simulation)

func (m *MockKubernetesClient) ListPodsWithLabel(ctx context.Context, namespace, labelSelector string) (*corev1.PodList, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	return &corev1.PodList{Items: []corev1.Pod{}}, nil
}

func (m *MockKubernetesClient) ListAllPods(ctx context.Context, namespace string) (*corev1.PodList, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	return &corev1.PodList{Items: []corev1.Pod{}}, nil
}

// GetDeployment and ScaleDeployment - removed old stub implementations
// (New implementations are provided below with proper deployment simulation)

func (m *MockKubernetesClient) ListNodes(ctx context.Context) (*corev1.NodeList, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	return &corev1.NodeList{Items: []corev1.Node{}}, nil
}

func (m *MockKubernetesClient) GetEvents(ctx context.Context, namespace string) (*corev1.EventList, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	return &corev1.EventList{Items: []corev1.Event{}}, nil
}

func (m *MockKubernetesClient) GetResourceQuotas(ctx context.Context, namespace string) (*corev1.ResourceQuotaList, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	return &corev1.ResourceQuotaList{Items: []corev1.ResourceQuota{}}, nil
}

func (m *MockKubernetesClient) GetPodLogs(ctx context.Context, namespace, name string, since *metav1.Time) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return "", m.error
	}

	return "mock pod logs", nil
}

// AdvancedClient methods - minimal implementation for testing
func (m *MockKubernetesClient) RollbackDeployment(ctx context.Context, namespace, name string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) ExpandPVC(ctx context.Context, namespace, name string, newSize string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) DrainNode(ctx context.Context, nodeName string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) QuarantinePod(ctx context.Context, namespace, name string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) CollectDiagnostics(ctx context.Context, namespace, resource string) (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	return map[string]interface{}{"mock": "diagnostics"}, nil
}

func (m *MockKubernetesClient) CleanupStorage(ctx context.Context, namespace, podName, path string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) BackupData(ctx context.Context, namespace, resource, backupName string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) CompactStorage(ctx context.Context, namespace, resource string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) CordonNode(ctx context.Context, nodeName string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) UpdateHPA(ctx context.Context, namespace, name string, minReplicas, maxReplicas int32) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) RestartDaemonSet(ctx context.Context, namespace, name string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) RotateSecrets(ctx context.Context, namespace, secretName string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) AuditLogs(ctx context.Context, namespace, resource, scope string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) CreateHeapDump(ctx context.Context, namespace, podName, dumpPath string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) EnableDebugMode(ctx context.Context, namespace, resource, logLevel, duration string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) OptimizeResources(ctx context.Context, namespace, resource, optimizationType string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) MigrateWorkload(ctx context.Context, namespace, workloadName, targetNode string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

// Additional methods to complete the k8s.Client interface
func (m *MockKubernetesClient) UpdatePodResources(ctx context.Context, namespace, name string, resources corev1.ResourceRequirements) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) UpdateNetworkPolicy(ctx context.Context, namespace, policyName, actionType string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) RestartNetwork(ctx context.Context, component string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) ResetServiceMesh(ctx context.Context, meshType string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) FailoverDatabase(ctx context.Context, namespace, databaseName, replicaName string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) RepairDatabase(ctx context.Context, namespace, databaseName, repairType string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) ScaleStatefulSet(ctx context.Context, namespace, name string, replicas int32) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error
}

func (m *MockKubernetesClient) IsHealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.error == nil
}

// Core pod methods needed for workflow testing
func (m *MockKubernetesClient) GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	key := fmt.Sprintf("%s/%s", namespace, name)

	pod, exists := m.pods[key]
	if !exists {
		// Create a default pod if it doesn't exist
		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				UID:       "original-uid-123",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}
		m.pods[key] = pod
	}

	// If pod was deleted (simulating controller recreation), return new pod with different UID
	if m.deletedPods[key] {
		newPod := pod.DeepCopy()
		newPod.UID = "recreated-uid-789"
		newPod.Status.Phase = corev1.PodRunning
		newPod.Status.Conditions = []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			},
		}
		return newPod, nil
	}

	// If pod was explicitly restarted, return new pod with different UID
	if m.restartedPods[key] {
		newPod := pod.DeepCopy()
		newPod.UID = "restarted-uid-456"
		newPod.Status.Phase = corev1.PodRunning
		newPod.Status.Conditions = []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			},
		}
		return newPod, nil
	}

	return pod, nil
}

func (m *MockKubernetesClient) DeletePod(ctx context.Context, namespace, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.error != nil {
		return m.error
	}

	key := fmt.Sprintf("%s/%s", namespace, name)
	m.podOperations = append(m.podOperations, fmt.Sprintf("delete_pod:%s", key))
	m.deletedPods[key] = true

	return nil
}

func (m *MockKubernetesClient) RestartPod(ctx context.Context, namespace, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.error != nil {
		return m.error
	}

	key := fmt.Sprintf("%s/%s", namespace, name)
	m.podOperations = append(m.podOperations, fmt.Sprintf("restart_pod:%s", key))
	m.restartedPods[key] = true

	return nil
}

func (m *MockKubernetesClient) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.error != nil {
		return nil, m.error
	}

	key := fmt.Sprintf("%s/%s", namespace, name)
	deployment, exists := m.deployments[key]
	if !exists {
		// Create a default deployment
		replicas := int32(1)
		deployment = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
			},
			Status: appsv1.DeploymentStatus{
				ReadyReplicas: replicas,
				Replicas:      replicas,
			},
		}
		m.deployments[key] = deployment
	}

	return deployment, nil
}

func (m *MockKubernetesClient) ScaleDeployment(ctx context.Context, namespace, name string, replicas int32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.error != nil {
		return m.error
	}

	key := fmt.Sprintf("%s/%s", namespace, name)
	m.deploymentOps = append(m.deploymentOps, fmt.Sprintf("scale_deployment:%s:%d", key, replicas))

	// Update deployment replica count
	deployment, exists := m.deployments[key]
	if exists {
		deployment.Spec.Replicas = &replicas
		deployment.Status.ReadyReplicas = replicas
		deployment.Status.Replicas = replicas
	}

	return nil
}

// Test helper methods
func (m *MockKubernetesClient) GetPodOperations() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string{}, m.podOperations...)
}

func (m *MockKubernetesClient) GetDeploymentOperations() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string{}, m.deploymentOps...)
}

func (m *MockKubernetesClient) ClearOperations() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.podOperations = make([]string, 0)
	m.deploymentOps = make([]string, 0)
	m.restartedPods = make(map[string]bool)
	m.deletedPods = make(map[string]bool)
}

// AnalyticsExecutionRepositoryMock provides a mock implementation of analytics ExecutionRepository for testing
// This mock implements the analytics package ExecutionRepository interface
type AnalyticsExecutionRepositoryMock struct {
	workflowHistory         []*types.WorkflowExecutionData
	resourceUtilizationData *analytics.ResourceUtilizationData
	shouldError             bool
}

func NewAnalyticsExecutionRepositoryMock() *AnalyticsExecutionRepositoryMock {
	return &AnalyticsExecutionRepositoryMock{}
}

func (m *AnalyticsExecutionRepositoryMock) SetWorkflowHistory(data []*types.WorkflowExecutionData) {
	m.workflowHistory = data
}

func (m *AnalyticsExecutionRepositoryMock) SetResourceUtilizationData(data *analytics.ResourceUtilizationData) {
	m.resourceUtilizationData = data
}

func (m *AnalyticsExecutionRepositoryMock) SetShouldError(shouldError bool) {
	m.shouldError = shouldError
}

func (m *AnalyticsExecutionRepositoryMock) GetWorkflowHistory(ctx context.Context, timeRange analytics.TimeRange) ([]*types.WorkflowExecutionData, error) {
	if m.shouldError {
		return nil, fmt.Errorf("mock analytics execution repository error")
	}

	if m.workflowHistory != nil {
		return m.workflowHistory, nil
	}

	return []*types.WorkflowExecutionData{}, nil
}

func (m *AnalyticsExecutionRepositoryMock) GetResourceUtilizationData(ctx context.Context, timeRange analytics.TimeRange) (*analytics.ResourceUtilizationData, error) {
	if m.shouldError {
		return nil, fmt.Errorf("mock analytics execution repository error")
	}

	if m.resourceUtilizationData != nil {
		return m.resourceUtilizationData, nil
	}

	return &analytics.ResourceUtilizationData{
		TimeRange:   timeRange,
		Utilization: make(map[string][]analytics.UtilizationPoint),
	}, nil
}

// GetHistoricalData implements ml.ExecutionRepository interface
func (m *AnalyticsExecutionRepositoryMock) GetHistoricalData(ctx context.Context) ([]*types.WorkflowExecutionData, error) {
	if m.shouldError {
		return nil, fmt.Errorf("mock analytics execution repository error")
	}

	if m.workflowHistory != nil {
		return m.workflowHistory, nil
	}

	return []*types.WorkflowExecutionData{}, nil
}

// PatternDiscoveryExecutionRepositoryMock provides a mock implementation of pattern discovery ExecutionRepository for testing
// This mock implements the pattern discovery package ExecutionRepository interface
type PatternDiscoveryExecutionRepositoryMock struct {
	executionsData      []*types.RuntimeWorkflowExecution
	timeWindowCallCount int
}

func NewPatternDiscoveryExecutionRepositoryMock() *PatternDiscoveryExecutionRepositoryMock {
	return &PatternDiscoveryExecutionRepositoryMock{
		executionsData:      make([]*types.RuntimeWorkflowExecution, 0),
		timeWindowCallCount: 0,
	}
}

func (mer *PatternDiscoveryExecutionRepositoryMock) GetExecutionsInTimeWindow(ctx context.Context, start, end time.Time) ([]*types.RuntimeWorkflowExecution, error) {
	mer.timeWindowCallCount++

	// **Business Logic Fix**: Filter executions that overlap with the time window
	// **Development Principle**: Ensure functionality aligns with business requirements
	results := make([]*types.RuntimeWorkflowExecution, 0)
	for _, execution := range mer.executionsData {
		execStart := execution.StartTime
		execEnd := time.Now() // Use Now() if EndTime is nil
		if execution.EndTime != nil {
			execEnd = *execution.EndTime
		}

		// Check if execution overlaps with requested time window
		// An execution overlaps if: execStart < end AND execEnd > start
		if execStart.Before(end) && execEnd.After(start) {
			results = append(results, execution)
		}
	}
	return results, nil
}

func (mer *PatternDiscoveryExecutionRepositoryMock) SetExecutionsInTimeWindow(executions []*types.RuntimeWorkflowExecution) {
	mer.executionsData = executions
}

func (mer *PatternDiscoveryExecutionRepositoryMock) GetTimeWindowCallCount() int {
	return mer.timeWindowCallCount
}

// MockServiceIntegration provides a mock implementation of holmesgpt.ServiceIntegrationInterface for testing
// This mock implements the ServiceIntegrationInterface for HolmesGPT dynamic toolset management
// Following development guidelines: reuse existing patterns and provide strong foundation for other tests
type MockServiceIntegration struct {
	mu                    sync.RWMutex
	availableToolsets     []*holmesgpt.ToolsetConfig
	toolsetStats          holmesgpt.ToolsetStats
	serviceDiscoveryStats holmesgpt.ServiceDiscoveryStats
	healthStatus          holmesgpt.ServiceIntegrationHealth
	refreshError          error
}

// NewMockServiceIntegration creates a new mock service integration with default test data
func NewMockServiceIntegration() *MockServiceIntegration {
	now := time.Now()

	// Create default test toolsets following business requirements
	defaultToolsets := []*holmesgpt.ToolsetConfig{
		{
			Name:        "kubernetes-mock",
			ServiceType: "kubernetes",
			Description: "Mock Kubernetes toolset for testing",
			Version:     "1.0.0",
			Capabilities: []string{
				"pod_management",
				"service_discovery",
				"resource_monitoring",
			},
			Tools: []holmesgpt.HolmesGPTTool{
				{
					Name:        "get_pods",
					Description: "Get pods in namespace",
					Command:     "kubectl get pods -n ${namespace}",
					Category:    "monitoring",
				},
			},
			Priority:    100,
			Enabled:     true,
			LastUpdated: now,
		},
		{
			Name:        "prometheus-mock",
			ServiceType: "prometheus",
			Description: "Mock Prometheus toolset for testing",
			Version:     "2.0.0",
			Capabilities: []string{
				"metrics_query",
				"alerting",
				"performance_monitoring",
			},
			Tools: []holmesgpt.HolmesGPTTool{
				{
					Name:        "query_metrics",
					Description: "Query Prometheus metrics",
					Command:     "curl -G '${prometheus_url}/api/v1/query' --data-urlencode 'query=${query}'",
					Category:    "monitoring",
				},
			},
			Priority:    80,
			Enabled:     true,
			LastUpdated: now,
		},
	}

	// Create default statistics
	defaultStats := holmesgpt.ToolsetStats{
		TotalToolsets: len(defaultToolsets),
		EnabledCount:  len(defaultToolsets),
		TypeCounts: map[string]int{
			"kubernetes": 1,
			"prometheus": 1,
		},
		LastUpdate:   now,
		CacheHitRate: 0.85,
		CacheSize:    len(defaultToolsets),
	}

	// Create default service discovery stats
	defaultDiscoveryStats := holmesgpt.ServiceDiscoveryStats{
		TotalServices:     2,
		AvailableServices: 2,
		ServiceTypes: map[string]int{
			"kubernetes": 1,
			"prometheus": 1,
		},
		LastDiscovery: now,
	}

	// Create default health status
	defaultHealth := holmesgpt.ServiceIntegrationHealth{
		Healthy:                 true,
		ServiceDiscoveryHealthy: true,
		ToolsetManagerHealthy:   true,
		TotalToolsets:           len(defaultToolsets),
		EnabledToolsets:         len(defaultToolsets),
		DiscoveredServices:      2,
		AvailableServices:       2,
		LastUpdate:              now,
	}

	return &MockServiceIntegration{
		availableToolsets:     defaultToolsets,
		toolsetStats:          defaultStats,
		serviceDiscoveryStats: defaultDiscoveryStats,
		healthStatus:          defaultHealth,
	}
}

// GetAvailableToolsets returns all currently available toolsets
func (m *MockServiceIntegration) GetAvailableToolsets() []*holmesgpt.ToolsetConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return copy to avoid race conditions
	toolsets := make([]*holmesgpt.ToolsetConfig, len(m.availableToolsets))
	copy(toolsets, m.availableToolsets)
	return toolsets
}

// GetToolsetStats returns toolset statistics
func (m *MockServiceIntegration) GetToolsetStats() holmesgpt.ToolsetStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.toolsetStats
}

// GetServiceDiscoveryStats returns service discovery statistics
func (m *MockServiceIntegration) GetServiceDiscoveryStats() holmesgpt.ServiceDiscoveryStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.serviceDiscoveryStats
}

// RefreshToolsets forces a refresh of toolset configurations
func (m *MockServiceIntegration) RefreshToolsets(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.refreshError != nil {
		return m.refreshError
	}

	// Simulate refresh by updating last update time
	m.mu.RUnlock()
	m.mu.Lock()
	m.toolsetStats.LastUpdate = time.Now()
	m.healthStatus.LastUpdate = time.Now()
	m.mu.Unlock()
	m.mu.RLock()

	return nil
}

// GetHealthStatus returns the health status of the service integration
func (m *MockServiceIntegration) GetHealthStatus() holmesgpt.ServiceIntegrationHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.healthStatus
}

// Configuration methods for testing - following established mock patterns

// SetAvailableToolsets configures the available toolsets for testing
func (m *MockServiceIntegration) SetAvailableToolsets(toolsets []*holmesgpt.ToolsetConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.availableToolsets = toolsets

	// Update stats to match
	enabledCount := 0
	typeCounts := make(map[string]int)
	var lastUpdate time.Time

	for _, toolset := range toolsets {
		if toolset.Enabled {
			enabledCount++
		}
		typeCounts[toolset.ServiceType]++
		if toolset.LastUpdated.After(lastUpdate) {
			lastUpdate = toolset.LastUpdated
		}
	}

	m.toolsetStats = holmesgpt.ToolsetStats{
		TotalToolsets: len(toolsets),
		EnabledCount:  enabledCount,
		TypeCounts:    typeCounts,
		LastUpdate:    lastUpdate,
		CacheHitRate:  m.toolsetStats.CacheHitRate,
		CacheSize:     len(toolsets),
	}

	// Update health status
	m.healthStatus.TotalToolsets = len(toolsets)
	m.healthStatus.EnabledToolsets = enabledCount
	m.healthStatus.LastUpdate = lastUpdate
}

// SetToolsetStats configures custom toolset statistics for testing
func (m *MockServiceIntegration) SetToolsetStats(stats holmesgpt.ToolsetStats) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolsetStats = stats
}

// SetServiceDiscoveryStats configures custom service discovery statistics for testing
func (m *MockServiceIntegration) SetServiceDiscoveryStats(stats holmesgpt.ServiceDiscoveryStats) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.serviceDiscoveryStats = stats

	// Update health status to match
	m.healthStatus.DiscoveredServices = stats.TotalServices
	m.healthStatus.AvailableServices = stats.AvailableServices
}

// SetHealthStatus configures custom health status for testing
func (m *MockServiceIntegration) SetHealthStatus(health holmesgpt.ServiceIntegrationHealth) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthStatus = health
}

// SetRefreshError configures refresh operations to return an error for testing error conditions
func (m *MockServiceIntegration) SetRefreshError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshError = err
}

// AddToolset adds a toolset to the available toolsets for testing
func (m *MockServiceIntegration) AddToolset(toolset *holmesgpt.ToolsetConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.availableToolsets = append(m.availableToolsets, toolset)

	// Update stats
	m.toolsetStats.TotalToolsets = len(m.availableToolsets)
	if toolset.Enabled {
		m.toolsetStats.EnabledCount++
	}

	if m.toolsetStats.TypeCounts == nil {
		m.toolsetStats.TypeCounts = make(map[string]int)
	}
	m.toolsetStats.TypeCounts[toolset.ServiceType]++

	if toolset.LastUpdated.After(m.toolsetStats.LastUpdate) {
		m.toolsetStats.LastUpdate = toolset.LastUpdated
	}

	// Update health status
	m.healthStatus.TotalToolsets = len(m.availableToolsets)
	if toolset.Enabled {
		m.healthStatus.EnabledToolsets++
	}
}

// ClearToolsets removes all toolsets for testing isolation
func (m *MockServiceIntegration) ClearToolsets() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.availableToolsets = make([]*holmesgpt.ToolsetConfig, 0)
	m.toolsetStats = holmesgpt.ToolsetStats{
		TotalToolsets: 0,
		EnabledCount:  0,
		TypeCounts:    make(map[string]int),
		LastUpdate:    time.Now(),
		CacheHitRate:  0.0,
		CacheSize:     0,
	}
	m.healthStatus.TotalToolsets = 0
	m.healthStatus.EnabledToolsets = 0
}

// GetToolsetByName returns a specific toolset by name for testing validation
func (m *MockServiceIntegration) GetToolsetByName(name string) *holmesgpt.ToolsetConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, toolset := range m.availableToolsets {
		if toolset.Name == name {
			return toolset
		}
	}
	return nil
}

// CheckKubernetesConnectivity validates Kubernetes cluster connectivity for testing
// Implements ServiceIntegrationInterface.CheckKubernetesConnectivity
func (m *MockServiceIntegration) CheckKubernetesConnectivity(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// For testing, simulate connectivity check
	// Return refreshError if set to simulate connection issues
	if m.refreshError != nil {
		return fmt.Errorf("kubernetes connectivity check failed: %w", m.refreshError)
	}

	// Simulate successful connectivity check
	return nil
}
