package testutil

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

// Mock testing constants - following development guidelines: eliminate magic values
const (
	// Default mock values
	DefaultMockConfidence     = 0.8
	DefaultMockHealthScore    = 0.8
	DefaultMockStabilityScore = 0.75
	DefaultMockRiskScore      = 0.9
	DefaultValidationScore    = 0.8
	DefaultQualityScore       = 0.8
	DefaultMockResourceID     = 1
	DefaultMockHistorySize    = 1000
	DefaultMockTimeout        = 30

	// Mock response templates
	DefaultMockResponse       = `{"action": "default_action", "confidence": 0.7, "reasoning": "Mock response"}`
	DefaultMockAction         = "notify_only"
	DefaultMockRiskLevel      = "low"
	DefaultMockUrgency        = "medium"
	DefaultMockBusinessImpact = "low"
	DefaultMockEnvironment    = "test"
	DefaultMockStatus         = "healthy"
	DefaultMockPriority       = "normal"
)

// MockFactory provides centralized mock object creation for comprehensive testing
// Business Requirements: Supports testing for BR-PA-001 through BR-PA-013 (alert processing),
// BR-AP-001 through BR-AP-025 (alert pipeline), BR-AI-001 through BR-AI-020 (AI/ML capabilities),
// BR-WF-001 through BR-WF-020 (workflow execution), and monitoring/metrics validation
// Following development guidelines: consolidate mock creation to improve maintainability and consistency
type MockFactory struct {
	logger *logrus.Logger
}

// NewMockFactory creates a new mock factory
func NewMockFactory(logger *logrus.Logger) *MockFactory {
	return &MockFactory{logger: logger}
}

// =============================================================================
// SLM CLIENT MOCKS
// Business Requirements: Support testing for BR-PA-006 through BR-PA-010 (AI decision making),
// BR-AI-001 through BR-AI-005 (analytics), enabling validation of LLM client integration
// =============================================================================

// StandardMockSLMClient provides a consistent mock SLM client across all tests
type StandardMockSLMClient struct {
	response  *types.ActionRecommendation
	error     string
	healthy   bool
	callCount int
}

// NewStandardMockSLMClient creates the standard SLM client mock
func (f *MockFactory) NewStandardMockSLMClient() *StandardMockSLMClient {
	return &StandardMockSLMClient{
		healthy: true,
	}
}

func (m *StandardMockSLMClient) SetAnalysisResponse(response *types.ActionRecommendation) {
	m.response = response
	m.error = ""
}

func (m *StandardMockSLMClient) SetError(err string) {
	m.error = err
	m.response = nil
}

func (m *StandardMockSLMClient) SetHealthy(healthy bool) {
	m.healthy = healthy
}

func (m *StandardMockSLMClient) GetCallCount() int {
	return m.callCount
}

func (m *StandardMockSLMClient) AnalyzeAlert(ctx context.Context, alert types.Alert) (*types.ActionRecommendation, error) {
	m.callCount++

	if m.error != "" {
		return nil, fmt.Errorf(m.error)
	}

	if m.response != nil {
		return m.response, nil
	}

	// Default response - following development guidelines: use constants instead of magic values
	return &types.ActionRecommendation{
		Action:     DefaultMockAction,
		Confidence: 0.5,
		Reasoning:  &types.ReasoningDetails{Summary: "Standard mock response"},
	}, nil
}

func (m *StandardMockSLMClient) ChatCompletion(ctx context.Context, prompt string) (string, error) {
	m.callCount++
	if m.error != "" {
		return "", fmt.Errorf(m.error)
	}
	return DefaultMockResponse, nil
}

func (m *StandardMockSLMClient) IsHealthy() bool {
	return m.healthy
}

// =============================================================================
// AI RESPONSE PROCESSOR MOCKS
// =============================================================================

type StandardMockAIResponseProcessor struct {
	processResponse    *types.EnhancedActionRecommendation
	validationResult   *types.LLMValidationResult
	reasoningAnalysis  *types.ReasoningDetails
	confidenceScore    float64
	contextualMetadata map[string]interface{}
	processError       string
	validationError    string
	healthy            bool
}

func (f *MockFactory) NewStandardMockAIResponseProcessor() *StandardMockAIResponseProcessor {
	return &StandardMockAIResponseProcessor{
		healthy: true,
	}
}

func (m *StandardMockAIResponseProcessor) SetProcessResponse(response *types.EnhancedActionRecommendation) {
	m.processResponse = response
	m.processError = ""
}

func (m *StandardMockAIResponseProcessor) SetValidationResult(result *types.LLMValidationResult) {
	m.validationResult = result
	m.validationError = ""
}

func (m *StandardMockAIResponseProcessor) SetError(err string) {
	m.processError = err
}

func (m *StandardMockAIResponseProcessor) SetValidationError(err string) {
	m.validationError = err
}

func (m *StandardMockAIResponseProcessor) SetHealthy(healthy bool) {
	m.healthy = healthy
}

func (m *StandardMockAIResponseProcessor) ProcessResponse(ctx context.Context, rawResponse string, originalAlert types.Alert) (*types.EnhancedActionRecommendation, error) {
	if m.processError != "" {
		return nil, fmt.Errorf(m.processError)
	}
	if m.processResponse != nil {
		return m.processResponse, nil
	}
	return nil, fmt.Errorf("no mock response configured")
}

func (m *StandardMockAIResponseProcessor) ValidateRecommendation(ctx context.Context, recommendation *types.ActionRecommendation, alert types.Alert) (*types.LLMValidationResult, error) {
	if m.validationError != "" {
		return nil, fmt.Errorf(m.validationError)
	}
	if m.validationResult != nil {
		return m.validationResult, nil
	}
	return &types.LLMValidationResult{
		ValidationScore: DefaultValidationScore,
		RiskAssessment: &types.LLMRiskAssessment{
			RiskLevel:          DefaultMockRiskLevel,
			ReversibilityScore: DefaultMockRiskScore,
		},
	}, nil
}

func (m *StandardMockAIResponseProcessor) AnalyzeReasoning(ctx context.Context, reasoning *types.ReasoningDetails, alert types.Alert) (*types.ReasoningDetails, error) {
	if m.reasoningAnalysis != nil {
		return m.reasoningAnalysis, nil
	}
	return &types.ReasoningDetails{
		Summary:       "Mock reasoning analysis",
		PrimaryReason: "Test analysis",
		ConfidenceFactors: map[string]float64{
			"quality":      DefaultQualityScore,
			"coherence":    DefaultQualityScore,
			"completeness": DefaultQualityScore,
		},
	}, nil
}

func (m *StandardMockAIResponseProcessor) AssessConfidence(ctx context.Context, recommendation *types.ActionRecommendation, alert types.Alert) (float64, error) {
	if m.confidenceScore > 0 {
		return m.confidenceScore, nil
	}
	return DefaultMockConfidence, nil // Default mock confidence score
}

func (m *StandardMockAIResponseProcessor) EnhanceContext(ctx context.Context, recommendation *types.ActionRecommendation, alert types.Alert) (map[string]interface{}, error) {
	if m.contextualMetadata != nil {
		return m.contextualMetadata, nil
	}
	return map[string]interface{}{
		"urgency":        DefaultMockUrgency,
		"businessImpact": DefaultMockBusinessImpact,
		"environment":    DefaultMockEnvironment,
		"priority":       DefaultMockPriority,
	}, nil
}

func (m *StandardMockAIResponseProcessor) IsHealthy() bool {
	return m.healthy
}

// =============================================================================
// REPOSITORY MOCKS
// Business Requirements: Support testing for BR-AP-021 through BR-AP-025 (alert lifecycle management),
// BR-AI-005 (maintain analysis history), enabling validation of action history and persistence
// =============================================================================

type StandardMockRepository struct {
	traces []actionhistory.ResourceActionTrace
	error  string
}

func (f *MockFactory) NewStandardMockRepository() *StandardMockRepository {
	return &StandardMockRepository{
		traces: make([]actionhistory.ResourceActionTrace, 0),
	}
}

func (m *StandardMockRepository) SetError(err string) {
	m.error = err
}

func (m *StandardMockRepository) AddTrace(trace actionhistory.ResourceActionTrace) {
	m.traces = append(m.traces, trace)
}

func (m *StandardMockRepository) StoreResourceActionTrace(ctx context.Context, trace *actionhistory.ResourceActionTrace) error {
	if m.error != "" {
		return fmt.Errorf(m.error)
	}
	m.traces = append(m.traces, *trace)
	return nil
}

func (m *StandardMockRepository) GetResourceActionTrace(ctx context.Context, id string) (*actionhistory.ResourceActionTrace, error) {
	if m.error != "" {
		return nil, fmt.Errorf(m.error)
	}

	for _, trace := range m.traces {
		if trace.ActionID == id {
			return &trace, nil
		}
	}
	return nil, fmt.Errorf("trace not found")
}

func (m *StandardMockRepository) GetActionTrace(ctx context.Context, actionID string) (*actionhistory.ResourceActionTrace, error) {
	return m.GetResourceActionTrace(ctx, actionID)
}

func (m *StandardMockRepository) GetResourceActionTraces(ctx context.Context, namespace, resource string, limit int) ([]actionhistory.ResourceActionTrace, error) {
	if m.error != "" {
		return nil, fmt.Errorf(m.error)
	}
	return m.traces, nil
}

func (m *StandardMockRepository) GetActionTraces(ctx context.Context, query actionhistory.ActionQuery) ([]actionhistory.ResourceActionTrace, error) {
	if m.error != "" {
		return nil, fmt.Errorf(m.error)
	}
	return m.traces, nil
}

func (m *StandardMockRepository) GetActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	if m.error != "" {
		return nil, fmt.Errorf(m.error)
	}
	// Return a basic mock action history - following development guidelines: use constants
	return &actionhistory.ActionHistory{
		ID:         DefaultMockResourceID,
		ResourceID: resourceID,
		MaxActions: DefaultMockHistorySize,
	}, nil
}

func (m *StandardMockRepository) GetPendingEffectivenessAssessments(ctx context.Context) ([]*actionhistory.ResourceActionTrace, error) {
	if m.error != "" {
		return nil, fmt.Errorf(m.error)
	}
	// Convert traces to pointers
	result := make([]*actionhistory.ResourceActionTrace, len(m.traces))
	for i := range m.traces {
		result[i] = &m.traces[i]
	}
	return result, nil
}

func (m *StandardMockRepository) GetOscillationPatterns(ctx context.Context, patternType string) ([]actionhistory.OscillationPattern, error) {
	if m.error != "" {
		return nil, fmt.Errorf(m.error)
	}
	return []actionhistory.OscillationPattern{}, nil
}

func (m *StandardMockRepository) StoreOscillationDetection(ctx context.Context, detection *actionhistory.OscillationDetection) error {
	if m.error != "" {
		return fmt.Errorf(m.error)
	}
	return nil
}

func (m *StandardMockRepository) GetOscillationDetections(ctx context.Context, resourceID int64, resolved *bool) ([]actionhistory.OscillationDetection, error) {
	if m.error != "" {
		return nil, fmt.Errorf(m.error)
	}
	return []actionhistory.OscillationDetection{}, nil
}

func (m *StandardMockRepository) ApplyRetention(ctx context.Context, actionHistoryID int64) error {
	if m.error != "" {
		return fmt.Errorf(m.error)
	}
	return nil
}

func (m *StandardMockRepository) GetActionHistorySummaries(ctx context.Context, since time.Duration) ([]actionhistory.ActionHistorySummary, error) {
	if m.error != "" {
		return nil, fmt.Errorf(m.error)
	}
	return []actionhistory.ActionHistorySummary{}, nil
}

func (m *StandardMockRepository) EnsureResourceReference(ctx context.Context, ref actionhistory.ResourceReference) (int64, error) {
	if m.error != "" {
		return 0, fmt.Errorf(m.error)
	}
	return 1, nil
}

func (m *StandardMockRepository) GetResourceReference(ctx context.Context, namespace, kind, name string) (*actionhistory.ResourceReference, error) {
	if m.error != "" {
		return nil, fmt.Errorf(m.error)
	}
	return &actionhistory.ResourceReference{
		ID:        1,
		Kind:      kind,
		Name:      name,
		Namespace: namespace,
	}, nil
}

func (m *StandardMockRepository) EnsureActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	if m.error != "" {
		return nil, fmt.Errorf(m.error)
	}
	return &actionhistory.ActionHistory{
		ID:         DefaultMockResourceID,
		ResourceID: resourceID,
		MaxActions: DefaultMockHistorySize,
	}, nil
}

func (m *StandardMockRepository) UpdateActionHistory(ctx context.Context, history *actionhistory.ActionHistory) error {
	if m.error != "" {
		return fmt.Errorf(m.error)
	}
	return nil
}

func (m *StandardMockRepository) StoreAction(ctx context.Context, action *actionhistory.ActionRecord) (*actionhistory.ResourceActionTrace, error) {
	if m.error != "" {
		return nil, fmt.Errorf(m.error)
	}
	trace := actionhistory.ResourceActionTrace{
		ID:       1,
		ActionID: generateMockActionID(),
	}
	m.traces = append(m.traces, trace)
	return &trace, nil
}

func (m *StandardMockRepository) UpdateActionTrace(ctx context.Context, trace *actionhistory.ResourceActionTrace) error {
	if m.error != "" {
		return fmt.Errorf(m.error)
	}
	return nil
}

// =============================================================================
// VECTOR DATABASE MOCKS
// =============================================================================

type StandardMockVectorDatabase struct {
	patterns map[string]*vector.ActionPattern
	error    string
}

func (f *MockFactory) NewStandardMockVectorDatabase() *StandardMockVectorDatabase {
	return &StandardMockVectorDatabase{
		patterns: make(map[string]*vector.ActionPattern),
	}
}

func (m *StandardMockVectorDatabase) SetError(err string) {
	m.error = err
}

func (m *StandardMockVectorDatabase) AddPattern(pattern *vector.ActionPattern) {
	m.patterns[pattern.ID] = pattern
}

func (m *StandardMockVectorDatabase) StoreActionPattern(ctx context.Context, pattern *vector.ActionPattern) error {
	if m.error != "" {
		return fmt.Errorf(m.error)
	}
	m.patterns[pattern.ID] = pattern
	return nil
}

func (m *StandardMockVectorDatabase) SearchSimilarPatterns(ctx context.Context, pattern *vector.ActionPattern, limit int, threshold float64) ([]*vector.ActionPattern, error) {
	if m.error != "" {
		return nil, fmt.Errorf(m.error)
	}

	results := make([]*vector.ActionPattern, 0)
	for _, p := range m.patterns {
		results = append(results, p)
	}
	return results, nil
}

func (m *StandardMockVectorDatabase) GetActionPattern(ctx context.Context, id string) (*vector.ActionPattern, error) {
	if m.error != "" {
		return nil, fmt.Errorf(m.error)
	}

	if pattern, exists := m.patterns[id]; exists {
		return pattern, nil
	}
	return nil, fmt.Errorf("pattern not found")
}

func (m *StandardMockVectorDatabase) ListActionPatterns(ctx context.Context, limit int) ([]*vector.ActionPattern, error) {
	if m.error != "" {
		return nil, fmt.Errorf(m.error)
	}

	results := make([]*vector.ActionPattern, 0)
	for _, p := range m.patterns {
		results = append(results, p)
	}
	return results, nil
}

func (m *StandardMockVectorDatabase) DeleteActionPattern(ctx context.Context, id string) error {
	if m.error != "" {
		return fmt.Errorf(m.error)
	}
	delete(m.patterns, id)
	return nil
}

func (m *StandardMockVectorDatabase) Close() error {
	return nil
}

// =============================================================================
// KNOWLEDGE BASE MOCKS
// =============================================================================

type StandardMockKnowledgeBase struct {
	actionRisks        map[string]*types.LLMRiskAssessment
	historicalPatterns []map[string]interface{}
	validationRules    []map[string]interface{}
	systemState        map[string]interface{}
	k8sClient          interface{}
}

func (f *MockFactory) NewStandardMockKnowledgeBase() *StandardMockKnowledgeBase {
	return &StandardMockKnowledgeBase{
		actionRisks:        make(map[string]*types.LLMRiskAssessment),
		historicalPatterns: []map[string]interface{}{},
		validationRules:    []map[string]interface{}{},
		systemState:        make(map[string]interface{}),
	}
}

func (m *StandardMockKnowledgeBase) SetK8sClient(client interface{}) {
	m.k8sClient = client
}

func (m *StandardMockKnowledgeBase) SetActionRisk(action string, risk *types.LLMRiskAssessment) {
	m.actionRisks[action] = risk
}

func (m *StandardMockKnowledgeBase) SetHistoricalPatterns(patterns []map[string]interface{}) {
	m.historicalPatterns = patterns
}

func (m *StandardMockKnowledgeBase) GetActionRisks(action string) *types.LLMRiskAssessment {
	if risk, exists := m.actionRisks[action]; exists {
		return risk
	}
	return &types.LLMRiskAssessment{
		RiskLevel:          DefaultMockRiskLevel,
		ReversibilityScore: DefaultMockRiskScore,
	}
}

func (m *StandardMockKnowledgeBase) GetHistoricalPatterns(alert types.Alert) []map[string]interface{} {
	return m.historicalPatterns
}

func (m *StandardMockKnowledgeBase) GetValidationRules() []map[string]interface{} {
	return m.validationRules
}

func (m *StandardMockKnowledgeBase) GetSystemState(ctx context.Context) (map[string]interface{}, error) {
	if m.systemState != nil {
		return m.systemState, nil
	}
	return map[string]interface{}{
		"healthScore":    DefaultMockHealthScore,
		"stabilityScore": DefaultMockStabilityScore,
		"environment":    DefaultMockEnvironment,
		"status":         DefaultMockStatus,
	}, nil
}

// =============================================================================
// UTILITY FUNCTIONS - Following development guidelines: reuse code whenever possible
// =============================================================================

// generateMockID creates a unique mock ID with the specified prefix
// Following development guidelines: avoid duplicating structure names and reuse code
func generateMockID(prefix string) string {
	return prefix + "-" + uuid.New().String()
}

// ID generation convenience functions using the consolidated approach
func generateMockActionID() string { return generateMockID("mock-action") }
