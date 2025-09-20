package mocks

import (
	"context"
	"fmt"
	"strings"
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
			InvestigationID: "mock_investigation_001",
			Status:          "completed",
			AlertName:       "mock_alert",
			Namespace:       "mock_namespace",
			Summary:         "Mock investigation analysis: The alert indicates potential memory resource exhaustion. Consider scaling deployment or optimizing resource usage. Database connection pool may require optimization.",
			RootCause:       "Resource exhaustion detected in memory subsystem",
			Recommendations: []holmesgpt.Recommendation{
				{
					Title:       "Scale Deployment",
					Description: "Scale the deployment to handle increased load",
					ActionType:  "scale",
					Priority:    "high",
					Confidence:  0.85,
				},
			},
			StrategyInsights: &holmesgpt.StrategyInsights{
				RecommendedStrategies: []holmesgpt.StrategyRecommendation{
					{
						StrategyName:          "horizontal_scaling",
						ExpectedSuccessRate:   0.88,
						EstimatedCost:         150,
						TimeToResolve:         15 * time.Minute,
						BusinessJustification: "Scale deployment horizontally to distribute load and improve performance",
						ROI:                   0.85,
					},
					{
						StrategyName:          "memory_optimization",
						ExpectedSuccessRate:   0.75,
						EstimatedCost:         75,
						TimeToResolve:         10 * time.Minute,
						BusinessJustification: "Optimize memory usage patterns to reduce resource consumption",
						ROI:                   0.70,
					},
				},
				HistoricalSuccessRate: 0.87,
				EstimatedROI:          0.82,
				TimeToResolution:      15 * time.Minute,
				BusinessImpact:        "High impact optimization potential for deployment scaling",
				ConfidenceLevel:       0.85,
			},
			ContextUsed: map[string]interface{}{
				"mock":             "investigation_response",
				"analysis_method":  "mock_holmesgpt",
				"confidence_score": 0.85,
			},
			Timestamp:       time.Now(),
			DurationSeconds: 1.5,
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

// AnalyzeStrategies implements holmesgpt.Client interface
func (m *MockClient) AnalyzeStrategies(ctx context.Context, req *holmesgpt.StrategyAnalysisRequest) (*holmesgpt.StrategyAnalysisResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.investigateError != nil {
		return nil, m.investigateError
	}

	// Return a basic mock strategy analysis response following project guidelines
	return &holmesgpt.StrategyAnalysisResponse{
		OptimalStrategy: holmesgpt.OptimalStrategyResult{
			Name:          "mock_dynamic_strategy",
			ExpectedROI:   0.87,
			SuccessRate:   0.94,
			Justification: "Mock strategy analysis: dynamic strategy optimization shows highest success rate based on real-time patterns",
		},
		StatisticalSignificance: 0.02, // p-value < 0.05 for high confidence
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

// IdentifyPotentialStrategies implements holmesgpt.Client interface
func (m *MockClient) IdentifyPotentialStrategies(alertContext types.AlertContext) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return context-aware strategies based on alert content
	strategies := []string{}

	// Add strategies based on alert name and labels
	if strings.Contains(strings.ToLower(alertContext.Name), "cpu") ||
		strings.Contains(strings.ToLower(alertContext.Labels["resource_type"]), "cpu") {
		strategies = append(strategies, "horizontal_scaling", "cpu_optimization", "load_balancing")
	}

	if strings.Contains(strings.ToLower(alertContext.Name), "memory") ||
		strings.Contains(strings.ToLower(alertContext.Labels["resource_type"]), "memory") {
		strategies = append(strategies, "memory_scaling", "memory_optimization", "garbage_collection_tuning")
	}

	if strings.Contains(strings.ToLower(alertContext.Name), "database") ||
		strings.Contains(strings.ToLower(alertContext.Labels["component"]), "database") {
		strategies = append(strategies, "connection_pool_optimization", "query_optimization", "database_scaling")
	}

	if alertContext.Severity == "critical" {
		strategies = append(strategies, "immediate_scaling", "emergency_rollback")
	}

	// Always provide at least basic strategies
	if len(strategies) == 0 {
		strategies = []string{"horizontal_scaling", "resource_optimization", "monitoring_enhancement"}
	}

	return strategies
}

// GetRelevantHistoricalPatterns implements holmesgpt.Client interface
func (m *MockClient) GetRelevantHistoricalPatterns(alertContext types.AlertContext) map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"rolling_deployment": 0.85,
		"canary_deployment":  0.78,
		"blue_green":         0.82,
		"scaling_strategy":   0.90,
	}
}

// AnalyzeCostImpactFactors implements holmesgpt.Client interface
func (m *MockClient) AnalyzeCostImpactFactors(alertContext types.AlertContext) map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"optimization_potential": 0.75,
		"cost_reduction":         0.60,
		"resource_efficiency":    0.82,
		"scaling_cost":           150.0,
	}
}

// GetSuccessRateIndicators implements holmesgpt.Client interface
func (m *MockClient) GetSuccessRateIndicators(alertContext types.AlertContext) map[string]float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]float64{
		"historical_success": 0.88,
		"confidence_level":   0.85,
		"pattern_match":      0.92,
		"risk_assessment":    0.15,
	}
}

// Additional helper methods for test scenarios

// CreateMockInvestigationForAlert creates a mock investigation response for a specific alert
func (m *MockClient) CreateMockInvestigationForAlert(alert types.Alert) *holmesgpt.InvestigateResponse {
	return &holmesgpt.InvestigateResponse{
		InvestigationID: fmt.Sprintf("mock_inv_%s", alert.Name),
		Status:          "completed",
		AlertName:       alert.Name,
		Namespace:       alert.Namespace,
		Summary: fmt.Sprintf("Mock investigation for alert %s in namespace %s: "+
			"The alert indicates potential issues. Recommended actions include monitoring resource usage and scaling if necessary.",
			alert.Name, alert.Namespace),
		RootCause: "Potential resource issue detected",
		Recommendations: []holmesgpt.Recommendation{
			{
				Title:       "Monitor Resources",
				Description: "Monitor resource usage and scale if necessary",
				ActionType:  "monitor",
				Priority:    alert.Severity,
				Confidence:  0.8,
			},
		},
		ContextUsed: map[string]interface{}{
			"mock":           true,
			"alert_name":     alert.Name,
			"alert_severity": alert.Severity,
			"namespace":      alert.Namespace,
		},
		Timestamp:       time.Now(),
		DurationSeconds: 1.0,
	}
}

// MockDynamicToolsetManager provides a mock implementation for DynamicToolsetManager
// Following project guideline: reuse existing mock patterns
type MockDynamicToolsetManager struct {
	mu        sync.RWMutex
	toolsets  []*holmesgpt.ToolsetConfig
	enabled   bool
	setupErr  error
	updateErr error
}

// NewMockDynamicToolsetManager creates a new mock dynamic toolset manager
func NewMockDynamicToolsetManager() *MockDynamicToolsetManager {
	return &MockDynamicToolsetManager{
		enabled: true,
		toolsets: []*holmesgpt.ToolsetConfig{
			{
				Name:         "kubernetes",
				ServiceType:  "kubernetes",
				Enabled:      true,
				Capabilities: []string{"kubectl", "get", "describe"},
				LastUpdated:  time.Now(),
			},
			{
				Name:         "prometheus",
				ServiceType:  "prometheus",
				Enabled:      true,
				Capabilities: []string{"query", "metrics"},
				LastUpdated:  time.Now(),
			},
		},
	}
}

// GetAvailableToolsets returns mock toolsets
func (m *MockDynamicToolsetManager) GetAvailableToolsets() []*holmesgpt.ToolsetConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.toolsets
}

// SetupToolsets mocks toolset setup
func (m *MockDynamicToolsetManager) SetupToolsets() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.setupErr
}

// UpdateToolsets mocks toolset updates
func (m *MockDynamicToolsetManager) UpdateToolsets() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.updateErr
}

// IsEnabled returns mock enabled status
func (m *MockDynamicToolsetManager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}
