package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

// MockLLMClient provides a mock implementation of llm.Client for testing
type MockLLMClient struct {
	workflowResponse *AIWorkflowResponse
	error            error
}

func NewMockLLMClient() *MockLLMClient {
	return &MockLLMClient{}
}

func (m *MockLLMClient) SetWorkflowResponse(response *AIWorkflowResponse) {
	m.workflowResponse = response
	m.error = nil
}

func (m *MockLLMClient) SetError(errMsg string) {
	m.error = errors.New(errMsg)
	m.workflowResponse = nil
}

func (m *MockLLMClient) AnalyzeAlert(ctx context.Context, alert interface{}) (interface{}, error) {
	if m.error != nil {
		return nil, m.error
	}

	return map[string]interface{}{
		"action":      "mock_action",
		"description": "Mock analysis result",
		"confidence":  0.8,
		"reasoning":   "Mock reasoning",
	}, nil
}

func (m *MockLLMClient) ChatCompletion(ctx context.Context, messages interface{}) (interface{}, error) {
	if m.error != nil {
		return nil, m.error
	}

	return map[string]interface{}{
		"content":     "Mock chat response",
		"model":       "test-model",
		"tokens_used": 50,
	}, nil
}

func (m *MockLLMClient) IsHealthy() bool {
	return m.error == nil
}

func (m *MockLLMClient) GetModelInfo() string {
	return "test-model"
}

func (m *MockLLMClient) GenerateWorkflowFromObjective(ctx context.Context, objective *WorkflowObjective) (*AIWorkflowResponse, error) {
	if m.error != nil {
		return nil, m.error
	}

	if m.workflowResponse != nil {
		return m.workflowResponse, nil
	}

	// Default response
	return &AIWorkflowResponse{
		WorkflowName: "Default Workflow",
		Description:  "Default generated workflow",
		Steps: []*AIGeneratedStep{
			{
				Name:    "Default Step",
				Type:    "action",
				Action:  &AIGeneratedAction{Type: "default_action"},
				Timeout: "5m",
			},
		},
		Reasoning:      "Default workflow generation",
		EstimatedTime:  "5m",
		RiskAssessment: "low",
	}, nil
}

// MockVectorDatabase provides a mock implementation of vector.VectorDatabase for testing
type MockVectorDatabase struct {
	actionPatterns []*vector.ActionPattern
	error          error
	UpdateCalled   bool
	StoreCalled    bool
}

func NewMockVectorDatabase() *MockVectorDatabase {
	return &MockVectorDatabase{
		actionPatterns: make([]*vector.ActionPattern, 0),
	}
}

func (m *MockVectorDatabase) SetActionPatterns(patterns []*vector.ActionPattern) {
	m.actionPatterns = patterns
	m.error = nil
}

func (m *MockVectorDatabase) SetError(errMsg string) {
	m.error = errors.New(errMsg)
}

func (m *MockVectorDatabase) StoreActionPattern(ctx context.Context, pattern *vector.ActionPattern) error {
	if m.error != nil {
		return m.error
	}

	m.StoreCalled = true
	m.actionPatterns = append(m.actionPatterns, pattern)
	return nil
}

func (m *MockVectorDatabase) SearchSimilarPatterns(ctx context.Context, query *vector.VectorSearchQuery) (*vector.VectorSearchResult, error) {
	if m.error != nil {
		return nil, m.error
	}

	// Return mock search result
	similarPatterns := make([]*vector.SimilarPattern, 0)
	for _, pattern := range m.actionPatterns {
		similarPatterns = append(similarPatterns, &vector.SimilarPattern{
			Pattern:    pattern,
			Similarity: 0.85,
		})
	}

	return &vector.VectorSearchResult{
		Patterns:   similarPatterns,
		TotalCount: len(similarPatterns),
		SearchTime: 50 * time.Millisecond,
	}, nil
}

func (m *MockVectorDatabase) UpdatePatternEffectiveness(ctx context.Context, patternID string, effectiveness *vector.EffectivenessData) error {
	if m.error != nil {
		return m.error
	}

	m.UpdateCalled = true

	// Find and update pattern
	for _, pattern := range m.actionPatterns {
		if pattern.ID == patternID {
			pattern.EffectivenessData = effectiveness
			pattern.UpdatedAt = time.Now()
			break
		}
	}

	return nil
}

func (m *MockVectorDatabase) SearchBySemantics(ctx context.Context, query string, limit int) ([]*vector.ActionPattern, error) {
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

func (m *MockVectorDatabase) DeletePattern(ctx context.Context, patternID string) error {
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

func (m *MockVectorDatabase) GetPatternAnalytics(ctx context.Context) (*vector.PatternAnalytics, error) {
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

func (m *MockVectorDatabase) IsHealthy() bool {
	return m.error == nil
}

func (m *MockVectorDatabase) GetStoredPatterns() []*vector.ActionPattern {
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

func (m *MockPatternExtractor) ExtractPatterns(ctx context.Context, executions []*WorkflowExecution) ([]*vector.ActionPattern, error) {
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

// MockExecutionRepository provides a mock implementation of ExecutionRepository for testing
type MockExecutionRepository struct {
	executions           []*WorkflowExecution
	error                error
	StoreExecutionCalled bool
}

func NewMockExecutionRepository() *MockExecutionRepository {
	return &MockExecutionRepository{
		executions: make([]*WorkflowExecution, 0),
	}
}

func (m *MockExecutionRepository) SetExecutions(executions []*WorkflowExecution) {
	m.executions = executions
	m.error = nil
}

func (m *MockExecutionRepository) SetError(errMsg string) {
	m.error = errors.New(errMsg)
}

func (m *MockExecutionRepository) StoreExecution(ctx context.Context, execution *WorkflowExecution) error {
	if m.error != nil {
		return m.error
	}

	m.StoreExecutionCalled = true
	m.executions = append(m.executions, execution)
	return nil
}

func (m *MockExecutionRepository) GetExecution(ctx context.Context, executionID string) (*WorkflowExecution, error) {
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

func (m *MockExecutionRepository) GetWorkflowExecutions(ctx context.Context, workflowID string, limit int) ([]*WorkflowExecution, error) {
	if m.error != nil {
		return nil, m.error
	}

	result := make([]*WorkflowExecution, 0)
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

func (m *MockExecutionRepository) GetRecentExecutions(ctx context.Context, since time.Time, limit int) ([]*WorkflowExecution, error) {
	if m.error != nil {
		return nil, m.error
	}

	result := make([]*WorkflowExecution, 0)
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

func (m *MockExecutionRepository) UpdateExecutionStatus(ctx context.Context, executionID string, status ExecutionStatus, errorMsg string) error {
	if m.error != nil {
		return m.error
	}

	for _, exec := range m.executions {
		if exec.ID == executionID {
			exec.Status = status
			if errorMsg != "" {
				if exec.Context == nil {
					exec.Context = &ExecutionContext{
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

func (m *MockExecutionRepository) DeleteOldExecutions(ctx context.Context, before time.Time) (int, error) {
	if m.error != nil {
		return 0, m.error
	}

	originalCount := len(m.executions)
	filtered := make([]*WorkflowExecution, 0)

	for _, exec := range m.executions {
		if exec.StartTime.After(before) {
			filtered = append(filtered, exec)
		}
	}

	m.executions = filtered
	return originalCount - len(filtered), nil
}
