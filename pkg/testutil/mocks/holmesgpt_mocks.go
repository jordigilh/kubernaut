package mocks

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// MockClient provides a test implementation following existing mock patterns
// Reuses established mock patterns from effectiveness_mocks.go
type MockClient struct {
	mu               sync.RWMutex
	healthy          bool
	healthError      error
	investigateError error

	// Request tracking for business requirement validation
	lastInvestigateRequest *holmesgpt.InvestigateRequest
	investigateHistory     []*holmesgpt.InvestigateRequest

	// Response configuration
	investigateResponse *holmesgpt.InvestigateResponse

	// Toolset management for dynamic context orchestration
	registeredToolsets []string
}

// NewMockClient creates a new mock client following existing patterns
func NewMockClient() *MockClient {
	return &MockClient{
		healthy: true,
		investigateResponse: &holmesgpt.InvestigateResponse{
			Investigation: "Mock investigation analysis: The alert indicates potential resource exhaustion. Consider scaling deployment or optimizing resource usage.",
			Context: map[string]interface{}{
				"mock":             "investigation_response",
				"analysis_method":  "mock_holmesgpt",
				"confidence_score": 0.85,
			},
		},
		investigateHistory: make([]*holmesgpt.InvestigateRequest, 0),
		registeredToolsets: []string{"kubernaut-toolset"},
	}
}

// Investigate implements holmesgpt.Client interface
func (m *MockClient) Investigate(ctx context.Context, request *holmesgpt.InvestigateRequest) (*holmesgpt.InvestigateResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Track requests for business requirement validation
	m.lastInvestigateRequest = request
	m.investigateHistory = append(m.investigateHistory, request)

	if m.investigateError != nil {
		return nil, m.investigateError
	}

	return m.investigateResponse, nil
}

// GetHealth implements holmesgpt.Client interface
func (m *MockClient) GetHealth(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.healthError
}

// Mock configuration methods following existing patterns

// SetHealthError configures health check error
func (m *MockClient) SetHealthError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthError = err
}

// SetInvestigateError configures investigate error for testing error conditions
func (m *MockClient) SetInvestigateError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.investigateError = err
}

// SetInvestigateResponse configures custom investigate response
func (m *MockClient) SetInvestigateResponse(response *holmesgpt.InvestigateResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.investigateResponse = response
}

// SetHealthy configures health status (needed for workflow-engine tests)
func (m *MockClient) SetHealthy(healthy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthy = healthy
}

// GetLastInvestigateRequest returns the last investigation request for validation
func (m *MockClient) GetLastInvestigateRequest() *holmesgpt.InvestigateRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastInvestigateRequest
}

// GetInvestigateHistory returns all investigation requests for business requirement testing
func (m *MockClient) GetInvestigateHistory() []*holmesgpt.InvestigateRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return copy to avoid race conditions
	history := make([]*holmesgpt.InvestigateRequest, len(m.investigateHistory))
	copy(history, m.investigateHistory)
	return history
}

// ClearHistory clears request history for test isolation
func (m *MockClient) ClearHistory() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.investigateHistory = make([]*holmesgpt.InvestigateRequest, 0)
	m.lastInvestigateRequest = nil
}

// AnalyzeRemediationStrategies implements holmesgpt.Client interface
func (m *MockClient) AnalyzeRemediationStrategies(ctx context.Context, req *holmesgpt.StrategyAnalysisRequest) (*holmesgpt.StrategyAnalysisResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.investigateError != nil {
		return nil, m.investigateError
	}

	// Return a basic mock strategy analysis response
	return &holmesgpt.StrategyAnalysisResponse{
		OptimalStrategy: holmesgpt.OptimalStrategyResult{
			Name:          "mock_scaling_strategy",
			ExpectedROI:   0.85,
			SuccessRate:   0.92,
			Justification: "Mock strategy analysis: scaling approach shows highest success rate based on historical data",
		},
		StatisticalSignificance: 0.03, // p-value < 0.05
	}, nil
}

// GetHistoricalPatterns implements holmesgpt.Client interface
func (m *MockClient) GetHistoricalPatterns(ctx context.Context, req *holmesgpt.PatternRequest) (*holmesgpt.PatternResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.investigateError != nil {
		return nil, m.investigateError
	}

	// Return a basic mock pattern response
	return &holmesgpt.PatternResponse{
		Patterns: []holmesgpt.HistoricalPattern{
			{
				PatternID:             "mock_pattern_001",
				StrategyName:          "mock_scaling_strategy",
				HistoricalSuccessRate: 0.88,
				OccurrenceCount:       5,
				AvgResolutionTime:     15 * time.Minute,
				BusinessContext:       "Mock pattern: recurring memory pressure in similar workloads",
			},
		},
		TotalPatterns:   1,
		ConfidenceLevel: 0.85,
	}, nil
}

// Additional helper methods for test scenarios

// CreateMockInvestigationForAlert creates a mock investigation response for a specific alert
func (m *MockClient) CreateMockInvestigationForAlert(alert types.Alert) *holmesgpt.InvestigateResponse {
	return &holmesgpt.InvestigateResponse{
		Investigation: fmt.Sprintf("Mock investigation for alert %s in namespace %s: "+
			"The alert indicates potential issues. Recommended actions include monitoring resource usage and scaling if necessary.",
			alert.Name, alert.Namespace),
		Context: map[string]interface{}{
			"mock":           true,
			"alert_name":     alert.Name,
			"alert_severity": alert.Severity,
			"namespace":      alert.Namespace,
		},
	}
}
