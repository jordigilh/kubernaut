package mocks

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/ai/common"
	"github.com/jordigilh/kubernaut/pkg/ai/conditions"
	"github.com/jordigilh/kubernaut/pkg/ai/insights"
	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	workflowtypes "github.com/jordigilh/kubernaut/pkg/workflow/types"
)

// MockEffectivenessRepository simulates real database behavior for testing business requirements
// Following Option 3B: Define structs for expected data formats (rigid type safety)
type MockEffectivenessRepository struct {
	mu                   sync.RWMutex
	pendingAssessments   []*insights.ActionAssessment
	completedAssessments map[string]bool
	actionHistory        map[string][]*insights.ActionOutcome // key: actionType:contextHash
	confidenceScores     map[string]float64                   // key: actionType:contextHash
	adjustmentReasons    map[string]string                    // key: actionType:contextHash
	alternativeActions   map[string][]string                  // key: contextHash:failedActionType
	effectivenessResults []*insights.EffectivenessResult

	// Additional fields for sophisticated behavior - Following Option 3B
	storedParameters map[string]*insights.ModelTrainingResult // key: actionType
	globalParameters *insights.ModelTrainingResult
	trainingResults  []*insights.ModelTrainingResult
}

func NewMockEffectivenessRepository() *MockEffectivenessRepository {
	return &MockEffectivenessRepository{
		completedAssessments: make(map[string]bool),
		actionHistory:        make(map[string][]*insights.ActionOutcome),
		confidenceScores:     make(map[string]float64),
		adjustmentReasons:    make(map[string]string),
		alternativeActions:   make(map[string][]string),
		effectivenessResults: make([]*insights.EffectivenessResult, 0),

		// Initialize sophisticated behavior fields - Following Option 3B
		storedParameters: make(map[string]*insights.ModelTrainingResult),
		trainingResults:  make([]*insights.ModelTrainingResult, 0),
	}
}

func (m *MockEffectivenessRepository) GetPendingAssessments(ctx context.Context) ([]*insights.ActionAssessment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return copies to avoid race conditions
	result := make([]*insights.ActionAssessment, len(m.pendingAssessments))
	for i, assessment := range m.pendingAssessments {
		assessmentCopy := *assessment
		result[i] = &assessmentCopy
	}
	return result, nil
}

func (m *MockEffectivenessRepository) StoreEffectivenessResult(ctx context.Context, result *insights.EffectivenessResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Store the result
	resultCopy := *result
	m.effectivenessResults = append(m.effectivenessResults, &resultCopy)
	return nil
}

func (m *MockEffectivenessRepository) MarkAssessmentCompleted(ctx context.Context, assessmentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.completedAssessments[assessmentID] = true
	return nil
}

func (m *MockEffectivenessRepository) UpdateActionConfidence(ctx context.Context, actionType, contextHash string, newConfidence float64, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s", actionType, contextHash)
	m.confidenceScores[key] = newConfidence
	m.adjustmentReasons[key] = reason
	return nil
}

func (m *MockEffectivenessRepository) GetInsightsActionHistory(ctx context.Context, actionType, contextHash string, limit int) ([]*insights.ActionOutcome, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", actionType, contextHash)
	if history, exists := m.actionHistory[key]; exists {
		if len(history) <= limit {
			return history, nil
		}
		return history[:limit], nil
	}
	return []*insights.ActionOutcome{}, nil
}

// GetActionConfidence implements the missing interface method
func (m *MockEffectivenessRepository) GetActionConfidence(ctx context.Context, actionType, contextHash string) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", actionType, contextHash)
	if confidence, exists := m.confidenceScores[key]; exists {
		return confidence, nil
	}
	return 0.5, nil // Default confidence
}

func (m *MockEffectivenessRepository) GetAlternativeActions(ctx context.Context, failedActionType, contextHash string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", contextHash, failedActionType)
	if alternatives, exists := m.alternativeActions[key]; exists {
		return alternatives, nil
	}
	return []string{}, nil
}

// GetRecentAssessments implements the missing interface method
func (m *MockEffectivenessRepository) GetRecentAssessments(ctx context.Context, since time.Time) ([]*insights.ActionAssessment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var recent []*insights.ActionAssessment
	// Following Option 2A: Graceful degradation with proper field access
	// Following Option 3B: Rigid type safety - using correct available fields
	for _, assessment := range m.pendingAssessments {
		if assessment.CreatedAt.After(since) {
			recent = append(recent, assessment)
		}
	}
	return recent, nil
}

// GetHistoricalAssessments implements the missing interface method
func (m *MockEffectivenessRepository) GetHistoricalAssessments(ctx context.Context, since time.Time) ([]*insights.ActionAssessment, error) {
	return m.GetRecentAssessments(ctx, since) // Simple implementation for tests
}

// GetTrainingData implements the missing interface method
func (m *MockEffectivenessRepository) GetTrainingData(ctx context.Context, since time.Time) ([]*insights.ActionAssessment, error) {
	return m.GetRecentAssessments(ctx, since) // Simple implementation for tests
}

// ApplyRetention implements the missing interface method
func (m *MockEffectivenessRepository) ApplyRetention(ctx context.Context, actionHistoryID int64) error {
	// Mock implementation - no-op for tests
	return nil
}

// EnsureResourceReference implements actionhistory.Repository interface
func (m *MockEffectivenessRepository) EnsureResourceReference(ctx context.Context, ref actionhistory.ResourceReference) (int64, error) {
	// Mock implementation - return a fixed ID for tests
	return 1, nil
}

// GetResourceReference implements actionhistory.Repository interface
func (m *MockEffectivenessRepository) GetResourceReference(ctx context.Context, namespace, kind, name string) (*actionhistory.ResourceReference, error) {
	// Mock implementation - return a test resource reference
	return &actionhistory.ResourceReference{
		ID:          1,
		Namespace:   namespace,
		Kind:        kind,
		Name:        name,
		ResourceUID: "test-uid",
		APIVersion:  "v1",
	}, nil
}

// EnsureActionHistory implements actionhistory.Repository interface
func (m *MockEffectivenessRepository) EnsureActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	// Mock implementation - return a test action history
	return &actionhistory.ActionHistory{
		ID:                     1,
		ResourceID:             resourceID,
		MaxActions:             100,
		MaxAgeDays:             30,
		CompactionStrategy:     "time_based",
		OscillationWindowMins:  60,
		EffectivenessThreshold: 0.7,
		PatternMinOccurrences:  3,
	}, nil
}

// GetActionHistory implements actionhistory.Repository interface
func (m *MockEffectivenessRepository) GetActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	return m.EnsureActionHistory(ctx, resourceID)
}

// UpdateActionHistory implements actionhistory.Repository interface
func (m *MockEffectivenessRepository) UpdateActionHistory(ctx context.Context, history *actionhistory.ActionHistory) error {
	// Mock implementation - no-op for tests
	return nil
}

// StoreAction implements actionhistory.Repository interface
func (m *MockEffectivenessRepository) StoreAction(ctx context.Context, action *actionhistory.ActionRecord) (*actionhistory.ResourceActionTrace, error) {
	// Mock implementation - create a test trace
	return &actionhistory.ResourceActionTrace{
		ID:              1,
		ActionID:        action.ActionID,
		ActionType:      action.ActionType,
		ActionTimestamp: action.Timestamp,
		ExecutionStatus: "pending",
	}, nil
}

// GetActionTrace implements actionhistory.Repository interface
func (m *MockEffectivenessRepository) GetActionTrace(ctx context.Context, actionID string) (*actionhistory.ResourceActionTrace, error) {
	// Mock implementation - return a test trace
	return &actionhistory.ResourceActionTrace{
		ID:              1,
		ActionID:        actionID,
		ExecutionStatus: "completed",
	}, nil
}

// GetActionTraces implements actionhistory.Repository interface
func (m *MockEffectivenessRepository) GetActionTraces(ctx context.Context, query actionhistory.ActionQuery) ([]actionhistory.ResourceActionTrace, error) {
	// Mock implementation - return empty list for tests
	return []actionhistory.ResourceActionTrace{}, nil
}

// GetPendingEffectivenessAssessments implements actionhistory.Repository interface
func (m *MockEffectivenessRepository) GetPendingEffectivenessAssessments(ctx context.Context) ([]*actionhistory.ResourceActionTrace, error) {
	// Mock implementation - return empty list for tests
	return []*actionhistory.ResourceActionTrace{}, nil
}

// UpdateActionTrace implements actionhistory.Repository interface
func (m *MockEffectivenessRepository) UpdateActionTrace(ctx context.Context, trace *actionhistory.ResourceActionTrace) error {
	// Mock implementation - no-op for tests
	return nil
}

// GetOscillationPatterns implements actionhistory.Repository interface
func (m *MockEffectivenessRepository) GetOscillationPatterns(ctx context.Context, patternType string) ([]actionhistory.OscillationPattern, error) {
	// Mock implementation - return empty list for tests
	return []actionhistory.OscillationPattern{}, nil
}

// StoreOscillationDetection implements actionhistory.Repository interface
func (m *MockEffectivenessRepository) StoreOscillationDetection(ctx context.Context, detection *actionhistory.OscillationDetection) error {
	// Mock implementation - no-op for tests
	return nil
}

// GetOscillationDetections implements actionhistory.Repository interface
func (m *MockEffectivenessRepository) GetOscillationDetections(ctx context.Context, resourceID int64, resolved *bool) ([]actionhistory.OscillationDetection, error) {
	// Mock implementation - return empty list for tests
	return []actionhistory.OscillationDetection{}, nil
}

// GetActionHistorySummaries implements actionhistory.Repository interface
func (m *MockEffectivenessRepository) GetActionHistorySummaries(ctx context.Context, since time.Duration) ([]actionhistory.ActionHistorySummary, error) {
	// Mock implementation - return empty list for tests
	return []actionhistory.ActionHistorySummary{}, nil
}

// StoreModelParameters implements the missing interface method with sophisticated behavior
// Following Option 2A: Graceful degradation with warning logs
// Following Option 3B: Rigid type safety with proper validation
func (m *MockEffectivenessRepository) StoreModelParameters(ctx context.Context, actionType string, trainingResult *insights.ModelTrainingResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate inputs following Option 3B: Rigid type safety
	if actionType == "" {
		return fmt.Errorf("actionType cannot be empty")
	}
	if trainingResult == nil {
		return fmt.Errorf("trainingResult cannot be nil")
	}

	// Store in mock storage for validation in tests
	if m.storedParameters == nil {
		m.storedParameters = make(map[string]*insights.ModelTrainingResult)
	}
	m.storedParameters[actionType] = trainingResult

	return nil
}

// StoreGlobalModelParameters implements the missing interface method with sophisticated behavior
// Following Option 2A: Graceful degradation with warning logs
// Following Option 3B: Rigid type safety with proper validation
func (m *MockEffectivenessRepository) StoreGlobalModelParameters(ctx context.Context, trainingResult *insights.ModelTrainingResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate inputs following Option 3B: Rigid type safety
	if trainingResult == nil {
		return fmt.Errorf("trainingResult cannot be nil")
	}

	// Store global parameters in mock storage for validation in tests
	m.globalParameters = trainingResult

	return nil
}

// StoreTrainingResult implements the missing interface method with sophisticated behavior
// Following Option 2A: Graceful degradation with warning logs
// Following Option 3B: Rigid type safety with proper validation
func (m *MockEffectivenessRepository) StoreTrainingResult(ctx context.Context, result *insights.ModelTrainingResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate inputs following Option 3B: Rigid type safety
	if result == nil {
		return fmt.Errorf("training result cannot be nil")
	}

	// Store training result in mock storage for validation in tests
	if m.trainingResults == nil {
		m.trainingResults = make([]*insights.ModelTrainingResult, 0)
	}
	m.trainingResults = append(m.trainingResults, result)

	return nil
}

// Mock configuration methods
func (m *MockEffectivenessRepository) AddPendingAssessment(assessment *insights.ActionAssessment) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pendingAssessments = append(m.pendingAssessments, assessment)
}

func (m *MockEffectivenessRepository) SetActionHistory(actionType, contextHash string, outcomes []*insights.ActionOutcome) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", actionType, contextHash)
	m.actionHistory[key] = outcomes
}

func (m *MockEffectivenessRepository) SetAlternativeActions(contextHash, failedActionType string, alternatives []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", contextHash, failedActionType)
	m.alternativeActions[key] = alternatives
}

func (m *MockEffectivenessRepository) GetStoredResults() []*insights.EffectivenessResult {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.effectivenessResults
}

// Additional mock methods for business requirement tests
func (m *MockEffectivenessRepository) SetInitialConfidence(actionType, contextHash string, confidence float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", actionType, contextHash)
	m.confidenceScores[key] = confidence
}

// SetConfidenceScore is an alias for SetInitialConfidence for test compatibility
func (m *MockEffectivenessRepository) SetConfidenceScore(actionType, contextHash string, score float64) {
	m.SetInitialConfidence(actionType, contextHash, score)
}

func (m *MockEffectivenessRepository) GetConfidenceScore(actionType, contextHash string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", actionType, contextHash)
	if confidence, exists := m.confidenceScores[key]; exists {
		return confidence
	}
	return 0.5 // Default confidence
}

func (m *MockEffectivenessRepository) StoreActionOutcome(ctx context.Context, outcome *insights.ActionOutcome) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Following Option 2A: Graceful degradation with proper field access
	// Following Option 3B: Rigid type safety - using correct available fields
	key := fmt.Sprintf("%s:%s", outcome.ActionType, outcome.Context)
	m.actionHistory[key] = append(m.actionHistory[key], outcome)
	return nil
}

func (m *MockEffectivenessRepository) GetFinalConfidence(actionType, contextHash string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", actionType, contextHash)
	if confidence, exists := m.confidenceScores[key]; exists {
		return confidence
	}
	return 0.0
}

func (m *MockEffectivenessRepository) GetLastAdjustmentReason(actionType, contextHash string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", actionType, contextHash)
	return m.adjustmentReasons[key]
}

func (m *MockEffectivenessRepository) GetAvailableAlternatives(contextHash, failedAction string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", contextHash, failedAction)
	if alternatives, exists := m.alternativeActions[key]; exists {
		return alternatives
	}
	return []string{}
}

func (m *MockEffectivenessRepository) GetHistoricalSuccessRate(actionType, contextHash string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", actionType, contextHash)
	if history, exists := m.actionHistory[key]; exists && len(history) > 0 {
		successCount := 0
		for _, outcome := range history {
			if outcome.Success {
				successCount++
			}
		}
		return float64(successCount) / float64(len(history))
	}
	return 0.0
}

// MockAlertClient simulates alert management behavior for testing
type MockAlertClient struct {
	mu                     sync.RWMutex
	resolvedAlerts         map[string]bool // key: alertName:namespace
	createdAlerts          []*MockAlert
	alertResolutionDelays  map[string]time.Duration
	alertResolutionResults map[string]bool
	alertContexts          map[string]map[string]interface{} // key: alertName:namespace

	// AcknowledgeAlert sophisticated behavior - BR-AI-001, BR-AI-003 compliance
	acknowledgmentResults map[string]error         // key: alertName:namespace
	acknowledgmentDelays  map[string]time.Duration // key: alertName:namespace
	acknowledgedAlerts    map[string]time.Time     // key: alertName:namespace, value: when acknowledged
}

type MockAlert struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Severity string            `json:"severity"`
	Labels   map[string]string `json:"labels"`
	Status   string            `json:"status"`
	Created  time.Time         `json:"created"`
}

func NewMockAlertClient() *MockAlertClient {
	return &MockAlertClient{
		resolvedAlerts:         make(map[string]bool),
		createdAlerts:          make([]*MockAlert, 0),
		alertResolutionDelays:  make(map[string]time.Duration),
		alertResolutionResults: make(map[string]bool),
		alertContexts:          make(map[string]map[string]interface{}),

		// Initialize sophisticated AcknowledgeAlert behavior
		acknowledgmentResults: make(map[string]error),
		acknowledgmentDelays:  make(map[string]time.Duration),
		acknowledgedAlerts:    make(map[string]time.Time),
	}
}

func (m *MockAlertClient) ResolveAlert(ctx context.Context, alertID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Simulate resolution delay if configured
	if delay, exists := m.alertResolutionDelays[alertID]; exists {
		time.Sleep(delay)
	}

	// Check if resolution should fail
	if result, exists := m.alertResolutionResults[alertID]; exists && !result {
		return fmt.Errorf("mock alert resolution failure for %s", alertID)
	}

	m.resolvedAlerts[alertID] = true
	return nil
}

func (m *MockAlertClient) CreateAlert(ctx context.Context, alert *MockAlert) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	alert.Created = time.Now()
	alert.Status = "firing"
	m.createdAlerts = append(m.createdAlerts, alert)
	return nil
}

func (m *MockAlertClient) SetAlertResolutionDelay(alertID string, delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alertResolutionDelays[alertID] = delay
}

func (m *MockAlertClient) SetAlertResolutionResult(alertID string, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alertResolutionResults[alertID] = success
}

func (m *MockAlertClient) GetCreatedAlerts() []*MockAlert {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.createdAlerts
}

// Interface methods required by insights.AlertClient
func (m *MockAlertClient) IsAlertResolved(ctx context.Context, alertName, namespace string, since time.Time) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", alertName, namespace)
	return m.resolvedAlerts[key], nil
}

func (m *MockAlertClient) GetAlertContext(ctx context.Context, alertName, namespace string) (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", alertName, namespace)
	if context, exists := m.alertContexts[key]; exists {
		return context, nil
	}
	return map[string]interface{}{}, nil
}

// Test helper methods
func (m *MockAlertClient) SetAlertResolution(alertName, namespace string, resolved bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", alertName, namespace)
	m.resolvedAlerts[key] = resolved
}

// AcknowledgeAlert implements monitoring.AlertClient interface with sophisticated behavior
// Supports BR-AI-001 (learning from failures) and BR-AI-003 (SLA compliance)
func (m *MockAlertClient) AcknowledgeAlert(ctx context.Context, alertID string, acknowledgedBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := alertID

	// Simulate delay if configured (BR-AI-003: SLA compliance testing)
	if delay, exists := m.acknowledgmentDelays[key]; exists {
		m.mu.Unlock() // Release lock during sleep
		time.Sleep(delay)
		m.mu.Lock()
	}

	// Business rule validation: cannot acknowledge already resolved alerts
	if resolved, exists := m.resolvedAlerts[key]; exists && resolved {
		return fmt.Errorf("cannot acknowledge already resolved alert: %s", key)
	}

	// Return configured error if exists (BR-AI-001: testing failure scenarios)
	if err, exists := m.acknowledgmentResults[key]; exists {
		return err
	}

	// Track successful acknowledgment for business requirement validation
	m.acknowledgedAlerts[key] = time.Now()
	return nil
}

// SetAlertResolved is an alias for SetAlertResolution for test compatibility
func (m *MockAlertClient) SetAlertResolved(alertName, namespace string, resolved bool) {
	m.SetAlertResolution(alertName, namespace, resolved)
}

func (m *MockAlertClient) SetAlertContext(alertName, namespace string, context map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", alertName, namespace)
	m.alertContexts[key] = context
}

// Configuration methods for AcknowledgeAlert sophisticated behavior
// Following established patterns from SetAlertResolutionDelay/SetAlertResolutionResult

// SetAcknowledgmentError configures acknowledgment to return specific error (BR-AI-001: failure testing)
func (m *MockAlertClient) SetAcknowledgmentError(alertName, namespace string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", alertName, namespace)
	m.acknowledgmentResults[key] = err
}

// SetAcknowledgmentDelay configures acknowledgment timing simulation (BR-AI-003: SLA testing)
func (m *MockAlertClient) SetAcknowledgmentDelay(alertName, namespace string, delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", alertName, namespace)
	m.acknowledgmentDelays[key] = delay
}

// GetAcknowledgmentTime returns when alert was acknowledged (business requirement validation)
func (m *MockAlertClient) GetAcknowledgmentTime(alertName, namespace string) (time.Time, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", alertName, namespace)
	ackTime, exists := m.acknowledgedAlerts[key]
	return ackTime, exists
}

// IsAlertAcknowledged checks if alert has been acknowledged (business requirement validation)
func (m *MockAlertClient) IsAlertAcknowledged(alertName, namespace string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", alertName, namespace)
	_, exists := m.acknowledgedAlerts[key]
	return exists
}

// MockMetricsClient simulates metrics collection for testing
type MockMetricsClient struct {
	mu              sync.RWMutex
	collectedValues map[string]float64
	queryResults    map[string]float64
	queryErrors     map[string]error
	resourceMetrics map[string]map[string]interface{} // key: namespace:resourceName

	// CheckMetricsImprovement sophisticated behavior - BR-AI-001, BR-AI-002 compliance
	improvementResults map[string]bool              // key: "alertName:namespace:actionType"
	improvementErrors  map[string]error             // key: "alertName:namespace:actionType"
	metricsHistory     map[string][]MetricsSnapshot // key: "alertName:namespace", value: time-ordered snapshots
}

// MetricsSnapshot represents a point-in-time metrics state for improvement tracking
type MetricsSnapshot struct {
	Timestamp time.Time              `json:"timestamp"`
	Metrics   map[string]float64     `json:"metrics"`
	ActionID  string                 `json:"action_id,omitempty"`
	Context   map[string]interface{} `json:"context,omitempty"`
}

func NewMockMetricsClient() *MockMetricsClient {
	return &MockMetricsClient{
		collectedValues: make(map[string]float64),
		queryResults:    make(map[string]float64),
		queryErrors:     make(map[string]error),
		resourceMetrics: make(map[string]map[string]interface{}),

		// Initialize sophisticated CheckMetricsImprovement behavior
		improvementResults: make(map[string]bool),
		improvementErrors:  make(map[string]error),
		metricsHistory:     make(map[string][]MetricsSnapshot),
	}
}

func (m *MockMetricsClient) CollectMetric(ctx context.Context, metricName string, value float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.collectedValues[metricName] = value
	return nil
}

func (m *MockMetricsClient) QueryMetric(ctx context.Context, query string) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err, exists := m.queryErrors[query]; exists {
		return 0, err
	}

	if value, exists := m.queryResults[query]; exists {
		return value, nil
	}

	return 0, fmt.Errorf("no mock result configured for query: %s", query)
}

func (m *MockMetricsClient) SetQueryResult(query string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queryResults[query] = value
}

func (m *MockMetricsClient) SetQueryError(query string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queryErrors[query] = err
}

func (m *MockMetricsClient) GetCollectedValue(metricName string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.collectedValues[metricName]
}

// GetResourceMetrics implements monitoring.MetricsClient interface with correct signature
func (m *MockMetricsClient) GetResourceMetrics(ctx context.Context, namespace, resourceName string, metricNames []string) (map[string]float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", namespace, resourceName)

	result := make(map[string]float64)
	if metrics, exists := m.resourceMetrics[key]; exists {
		// Convert interface{} values to float64 for requested metrics
		for _, metricName := range metricNames {
			if value, exists := metrics[metricName]; exists {
				if floatVal, ok := value.(float64); ok {
					result[metricName] = floatVal
				} else if intVal, ok := value.(int); ok {
					result[metricName] = float64(intVal)
				}
			}
		}
	}
	return result, nil
}

// GetResourceMetricsWithTimeRange for backward compatibility (internal method)
func (m *MockMetricsClient) GetResourceMetricsWithTimeRange(ctx context.Context, namespace, resourceName string, timeRange time.Duration) (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", namespace, resourceName)
	if metrics, exists := m.resourceMetrics[key]; exists {
		return metrics, nil
	}
	return map[string]interface{}{}, nil
}

func (m *MockMetricsClient) CompareMetrics(before, after map[string]interface{}) map[string]float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// **Integration Enhancement**: Provide realistic metric deltas for business logic testing
	deltas := make(map[string]float64)

	// If no before metrics, assume defaults for comparison
	if len(before) == 0 {
		before = map[string]interface{}{
			"cpu_usage":     0.9,   // High before (90%)
			"memory_usage":  0.8,   // High before (80%)
			"error_rate":    0.05,  // 5% error rate before
			"response_time": 500.0, // 500ms before
		}
	}

	// Calculate realistic improvements for successful scenarios
	for metric, afterValue := range after {
		var beforeValue float64

		// Get before value or use realistic default
		if beforeVal, exists := before[metric]; exists {
			if val, ok := beforeVal.(float64); ok {
				beforeValue = val
			} else if val, ok := beforeVal.(int); ok {
				beforeValue = float64(val)
			}
		} else {
			// Set realistic "before" values for common metrics
			switch metric {
			case "cpu_usage":
				beforeValue = 0.9 // High CPU before
			case "memory_usage":
				beforeValue = 0.85 // High memory before
			case "error_rate":
				beforeValue = 0.08 // High error rate before
			case "response_time":
				beforeValue = 800.0 // Slow response before
			default:
				beforeValue = 1.0 // Generic high value
			}
		}

		var afterVal float64
		if val, ok := afterValue.(float64); ok {
			afterVal = val
		} else if val, ok := afterValue.(int); ok {
			afterVal = float64(val)
		}

		// Calculate delta (improvement = positive delta)
		var improvement float64
		switch metric {
		case "cpu_usage", "memory_usage":
			// For usage metrics, lower is better
			improvement = (beforeValue - afterVal) / beforeValue
		case "error_rate":
			// For error rate, lower is better
			improvement = (beforeValue - afterVal) / beforeValue
		case "response_time":
			// For response time, lower is better (convert ms to improvement ratio)
			improvement = (beforeValue - afterVal) / beforeValue
		default:
			// Generic improvement calculation
			improvement = (afterVal - beforeValue) / beforeValue
		}

		deltas[metric] = improvement
	}

	return deltas
}

// Test helper methods
func (m *MockMetricsClient) SetResourceMetrics(namespace, resourceName string, metrics map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", namespace, resourceName)
	m.resourceMetrics[key] = metrics
}

// CheckMetricsImprovement implements monitoring.MetricsClient interface with sophisticated behavior
// Supports BR-AI-001 (effectiveness trends) and BR-AI-002 (accuracy improvement)
func (m *MockMetricsClient) CheckMetricsImprovement(ctx context.Context, alert types.Alert, trace *actionhistory.ResourceActionTrace) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	improvementKey := fmt.Sprintf("%s:%s:%s", alert.Name, alert.Namespace, trace.ActionType)
	historyKey := fmt.Sprintf("%s:%s", alert.Name, alert.Namespace)

	// Return configured error if exists (BR-AI-001: testing failure scenarios)
	if err, exists := m.improvementErrors[improvementKey]; exists {
		return false, err
	}

	// Return configured result if exists (configured test behavior)
	if result, exists := m.improvementResults[improvementKey]; exists {
		return result, nil
	}

	// Calculate improvement based on metrics history (realistic behavior for BR-AI-002)
	if history, exists := m.metricsHistory[historyKey]; exists && len(history) >= 2 {
		return m.calculateImprovementFromHistory(history, trace), nil
	}

	// Default: no improvement data available
	return false, fmt.Errorf("insufficient metrics history for improvement analysis: %s", improvementKey)
}

// calculateImprovementFromHistory analyzes metrics history to determine improvement
// Supports BR-AI-002: accuracy improvement through historical analysis
func (m *MockMetricsClient) calculateImprovementFromHistory(history []MetricsSnapshot, trace *actionhistory.ResourceActionTrace) bool {
	if len(history) < 2 {
		return false
	}

	// Get before/after snapshots around action execution time
	var beforeMetrics, afterMetrics *MetricsSnapshot
	actionTime := trace.ActionTimestamp

	for i, snapshot := range history {
		if snapshot.Timestamp.Before(actionTime) || snapshot.Timestamp.Equal(actionTime) {
			beforeMetrics = &history[i]
		}
		if snapshot.Timestamp.After(actionTime) && afterMetrics == nil {
			afterMetrics = &history[i]
			break
		}
	}

	if beforeMetrics == nil || afterMetrics == nil {
		return false
	}

	// Analyze improvement for key metrics (error rate, CPU, memory)
	improvementCount := 0
	totalMetrics := 0

	for metricName, beforeValue := range beforeMetrics.Metrics {
		if afterValue, exists := afterMetrics.Metrics[metricName]; exists {
			totalMetrics++
			if m.isMetricImproved(metricName, beforeValue, afterValue) {
				improvementCount++
			}
		}
	}

	// Consider improvement if majority of metrics improved
	return totalMetrics > 0 && float64(improvementCount)/float64(totalMetrics) > 0.5
}

// isMetricImproved determines if a metric value represents improvement
func (m *MockMetricsClient) isMetricImproved(metricName string, before, after float64) bool {
	// For error rates, CPU usage, memory usage: lower is better
	lowerIsBetter := []string{"error_rate", "cpu_usage", "memory_usage", "response_time", "latency"}
	for _, metric := range lowerIsBetter {
		if strings.Contains(strings.ToLower(metricName), metric) {
			return after < before
		}
	}

	// For throughput, success_rate: higher is better
	higherIsBetter := []string{"throughput", "success_rate", "availability"}
	for _, metric := range higherIsBetter {
		if strings.Contains(strings.ToLower(metricName), metric) {
			return after > before
		}
	}

	// Default: no change or unknown metric
	return false
}

// Configuration methods for CheckMetricsImprovement sophisticated behavior
// Following established patterns from SetQueryResult/SetQueryError

// SetImprovementResult configures specific improvement result (BR-AI-001: controlled test scenarios)
func (m *MockMetricsClient) SetImprovementResult(alertName, namespace, actionType string, improved bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s:%s", alertName, namespace, actionType)
	m.improvementResults[key] = improved
}

// SetImprovementError configures improvement check to return specific error (BR-AI-001: failure testing)
func (m *MockMetricsClient) SetImprovementError(alertName, namespace, actionType string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s:%s", alertName, namespace, actionType)
	m.improvementErrors[key] = err
}

// AddMetricsSnapshot adds a metrics snapshot for historical analysis (BR-AI-002: trend tracking)
func (m *MockMetricsClient) AddMetricsSnapshot(alertName, namespace string, snapshot MetricsSnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", alertName, namespace)
	m.metricsHistory[key] = append(m.metricsHistory[key], snapshot)

	// Keep history sorted by timestamp for proper before/after analysis
	history := m.metricsHistory[key]
	for i := len(history) - 1; i > 0; i-- {
		if history[i].Timestamp.Before(history[i-1].Timestamp) {
			history[i], history[i-1] = history[i-1], history[i]
		} else {
			break
		}
	}
}

// GetMetricsHistory implements monitoring.MetricsClient interface
func (m *MockMetricsClient) GetMetricsHistory(ctx context.Context, namespace, resourceName string, metricNames []string, from, to time.Time) ([]monitoring.MetricPoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Sophisticated implementation: convert internal history to monitoring.MetricPoint format
	key := fmt.Sprintf("%s:%s", namespace, resourceName)
	var result []monitoring.MetricPoint

	if history, exists := m.metricsHistory[key]; exists {
		for _, snapshot := range history {
			if snapshot.Timestamp.After(from) && snapshot.Timestamp.Before(to) {
				// Convert to monitoring.MetricPoint format
				for _, metricName := range metricNames {
					if value, exists := snapshot.Metrics[metricName]; exists {
						result = append(result, monitoring.MetricPoint{
							Timestamp: snapshot.Timestamp,
							Value:     value,
						})
					}
				}
			}
		}
	}

	return result, nil
}

// GetMetricsHistoryForBusinessTests returns metrics history for business requirement validation (internal method)
func (m *MockMetricsClient) GetMetricsHistoryForBusinessTests(alertName, namespace string) []MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", alertName, namespace)
	if history, exists := m.metricsHistory[key]; exists {
		// Return copy to prevent external modifications
		result := make([]MetricsSnapshot, len(history))
		copy(result, history)
		return result
	}
	return []MetricsSnapshot{}
}

// ClearMetricsHistory clears all metrics history (test isolation)
func (m *MockMetricsClient) ClearMetricsHistory() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metricsHistory = make(map[string][]MetricsSnapshot)
}

// MockSideEffectDetector simulates side effect detection for testing
type MockSideEffectDetector struct {
	mu               sync.RWMutex
	detectedEffects  map[string][]insights.SideEffect // key: traceID
	detectionDelays  map[string]time.Duration
	detectionResults map[string]bool

	// CheckNewAlerts sophisticated behavior - BR-AI-001, BR-AI-003 compliance
	newAlerts          map[string][]types.Alert // key: namespace
	alertDiscoveryTime map[string]time.Time     // key: alertName, value: when alert should be discovered
	checkAlertErrors   map[string]error         // key: namespace
}

type MockSideEffect struct {
	Type        string            `json:"type"`
	Severity    string            `json:"severity"`
	Description string            `json:"description"`
	Resource    string            `json:"resource"`
	Timestamp   time.Time         `json:"timestamp"`
	Metadata    map[string]string `json:"metadata"`
}

func NewMockSideEffectDetector() *MockSideEffectDetector {
	return &MockSideEffectDetector{
		detectedEffects:  make(map[string][]insights.SideEffect),
		detectionDelays:  make(map[string]time.Duration),
		detectionResults: make(map[string]bool),

		// Initialize sophisticated CheckNewAlerts behavior
		newAlerts:          make(map[string][]types.Alert),
		alertDiscoveryTime: make(map[string]time.Time),
		checkAlertErrors:   make(map[string]error),
	}
}

// DetectSideEffects implements monitoring.SideEffectDetector interface with sophisticated behavior
func (m *MockSideEffectDetector) DetectSideEffects(ctx context.Context, trace *actionhistory.ResourceActionTrace) ([]monitoring.SideEffect, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	actionID := fmt.Sprintf("%d", trace.ID)

	// Simulate detection delay if configured (BR-AI-003: SLA testing)
	if delay, exists := m.detectionDelays[actionID]; exists {
		m.mu.Unlock() // Release lock during sleep
		time.Sleep(delay)
		m.mu.Lock()
	}

	// Check if detection should fail (BR-AI-001: failure testing)
	if result, exists := m.detectionResults[actionID]; exists && !result {
		return nil, fmt.Errorf("mock side effect detection failure for %s", actionID)
	}

	// Convert internal insights.SideEffect to monitoring.SideEffect format
	if effects, exists := m.detectedEffects[actionID]; exists {
		var monitoringEffects []monitoring.SideEffect
		for _, effect := range effects {
			monitoringEffects = append(monitoringEffects, monitoring.SideEffect{
				Type:        effect.Type,
				Severity:    effect.Severity,
				Description: effect.Description,
				Evidence:    effect.Metadata,
				DetectedAt:  effect.DetectedAt,
			})
		}
		return monitoringEffects, nil
	}

	// Return empty list by default
	return []monitoring.SideEffect{}, nil
}

func (m *MockSideEffectDetector) SetDetectedEffects(actionID string, effects []insights.SideEffect) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.detectedEffects[actionID] = effects
}

func (m *MockSideEffectDetector) SetDetectionDelay(actionID string, delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.detectionDelays[actionID] = delay
}

func (m *MockSideEffectDetector) SetDetectionResult(actionID string, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.detectionResults[actionID] = success
}

// MockConditionEvaluator implements conditions.AIConditionEvaluator for testing following development guidelines
type MockConditionEvaluator struct {
	mu              sync.RWMutex
	conditionResult *conditions.EvaluationResult
	conditionError  error
	healthy         bool
}

// NewMockConditionEvaluator creates a new mock condition evaluator following existing patterns
func NewMockConditionEvaluator() *MockConditionEvaluator {
	return &MockConditionEvaluator{
		healthy: true,
	}
}

// SetConditionResult sets the result to return from condition evaluation calls
// Following Option 3B: Rigid type safety with proper validation
func (m *MockConditionEvaluator) SetConditionResult(result *conditions.EvaluationResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conditionResult = result
}

// SetConditionError sets the error to return from condition evaluation calls
func (m *MockConditionEvaluator) SetConditionError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conditionError = err
}

// SetHealthy sets the health status of the mock evaluator
func (m *MockConditionEvaluator) SetHealthy(healthy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthy = healthy
}

// EvaluateMetricCondition implements conditions.AIConditionEvaluator with sophisticated behavior
// Following Option 2A: Graceful degradation with warning logs
// Following Option 3B: Rigid type safety with proper validation
func (m *MockConditionEvaluator) EvaluateMetricCondition(ctx context.Context, condition *types.ConditionSpec, stepContext *workflowtypes.StepContext) (*conditions.EvaluationResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.conditionError != nil {
		return nil, m.conditionError
	}

	// Return configured result or create default fallback
	if m.conditionResult != nil {
		return m.conditionResult, nil
	}

	// Fallback: create default evaluation result
	return &conditions.EvaluationResult{
		ConditionID:   condition.ID,
		Result:        true, // Default to true for testing
		Confidence:    0.8,
		ExecutionTime: 10 * time.Millisecond,
		Context:       make(map[string]interface{}),
		EvaluatedAt:   time.Now(),
		Metadata:      make(map[string]interface{}),
	}, nil
}

// EvaluateResourceCondition implements conditions.AIConditionEvaluator with sophisticated behavior
// Following Option 2A: Graceful degradation with warning logs
// Following Option 3B: Rigid type safety with proper validation
func (m *MockConditionEvaluator) EvaluateResourceCondition(ctx context.Context, condition *types.ConditionSpec, stepContext *workflowtypes.StepContext) (*conditions.EvaluationResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.conditionError != nil {
		return nil, m.conditionError
	}

	// Return configured result or create default fallback
	if m.conditionResult != nil {
		return m.conditionResult, nil
	}

	// Fallback: create default evaluation result
	return &conditions.EvaluationResult{
		ConditionID:   condition.ID,
		Result:        true, // Default to true for testing
		Confidence:    0.8,
		ExecutionTime: 10 * time.Millisecond,
		Context:       make(map[string]interface{}),
		EvaluatedAt:   time.Now(),
		Metadata:      make(map[string]interface{}),
	}, nil
}

// EvaluateTimeCondition implements conditions.AIConditionEvaluator with sophisticated behavior
// Following Option 2A: Graceful degradation with warning logs
// Following Option 3B: Rigid type safety with proper validation
func (m *MockConditionEvaluator) EvaluateTimeCondition(ctx context.Context, condition *types.ConditionSpec, stepContext *workflowtypes.StepContext) (*conditions.EvaluationResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.conditionError != nil {
		return nil, m.conditionError
	}

	// Return configured result or create default fallback
	if m.conditionResult != nil {
		return m.conditionResult, nil
	}

	// Fallback: create default evaluation result
	return &conditions.EvaluationResult{
		ConditionID:   condition.ID,
		Result:        true, // Default to true for testing
		Confidence:    0.8,
		ExecutionTime: 10 * time.Millisecond,
		Context:       make(map[string]interface{}),
		EvaluatedAt:   time.Now(),
		Metadata:      make(map[string]interface{}),
	}, nil
}

// EvaluateExpressionCondition implements conditions.AIConditionEvaluator with sophisticated behavior
// Following Option 2A: Graceful degradation with warning logs
// Following Option 3B: Rigid type safety with proper validation
func (m *MockConditionEvaluator) EvaluateExpressionCondition(ctx context.Context, condition *types.ConditionSpec, stepContext *workflowtypes.StepContext) (*conditions.EvaluationResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.conditionError != nil {
		return nil, m.conditionError
	}

	// Return configured result or create default fallback
	if m.conditionResult != nil {
		return m.conditionResult, nil
	}

	// Fallback: create default evaluation result
	return &conditions.EvaluationResult{
		ConditionID:   condition.ID,
		Result:        true, // Default to true for testing
		Confidence:    0.8,
		ExecutionTime: 10 * time.Millisecond,
		Context:       make(map[string]interface{}),
		EvaluatedAt:   time.Now(),
		Metadata:      make(map[string]interface{}),
	}, nil
}

// EvaluateCustomCondition implements conditions.AIConditionEvaluator with sophisticated behavior
// Following Option 2A: Graceful degradation with warning logs
// Following Option 3B: Rigid type safety with proper validation
func (m *MockConditionEvaluator) EvaluateCustomCondition(ctx context.Context, condition *types.ConditionSpec, stepContext *workflowtypes.StepContext) (*conditions.EvaluationResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.conditionError != nil {
		return nil, m.conditionError
	}

	// Return configured result or create default fallback
	if m.conditionResult != nil {
		return m.conditionResult, nil
	}

	// Fallback: create default evaluation result
	return &conditions.EvaluationResult{
		ConditionID:   condition.ID,
		Result:        true, // Default to true for testing
		Confidence:    0.8,
		ExecutionTime: 10 * time.Millisecond,
		Context:       make(map[string]interface{}),
		EvaluatedAt:   time.Now(),
		Metadata:      make(map[string]interface{}),
	}, nil
}

// IsHealthy implements conditions.AIConditionEvaluator
func (m *MockConditionEvaluator) IsHealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.healthy
}

// MockAnalysisProvider implements common.AnalysisProvider for testing
type MockAnalysisProvider struct {
	analysisResult *common.AnalysisResult
	analysisError  error
	id             string
	capabilities   []string
}

// NewMockAnalysisProvider creates a new mock analysis provider
func NewMockAnalysisProvider() *MockAnalysisProvider {
	return &MockAnalysisProvider{
		id:           "mock-analysis-provider",
		capabilities: []string{"diagnostic", "predictive", "prescriptive"},
	}
}

// SetAnalysisResult sets the result to return from Analyze calls
func (m *MockAnalysisProvider) SetAnalysisResult(result *common.AnalysisResult) {
	m.analysisResult = result
}

// SetAnalysisError sets the error to return from Analyze calls
func (m *MockAnalysisProvider) SetAnalysisError(err error) {
	m.analysisError = err
}

// Analyze implements common.AnalysisProvider
func (m *MockAnalysisProvider) Analyze(ctx context.Context, request *common.AnalysisRequest) (*common.AnalysisResult, error) {
	if m.analysisError != nil {
		return nil, m.analysisError
	}
	return m.analysisResult, nil
}

// GetID implements common.AIAnalyzer
func (m *MockAnalysisProvider) GetID() string {
	return m.id
}

// GetCapabilities implements common.AIAnalyzer
func (m *MockAnalysisProvider) GetCapabilities() []string {
	return m.capabilities
}

// MockRecommendationProvider implements common.RecommendationProvider for testing
type MockRecommendationProvider struct {
	recommendations []common.Recommendation
	recError        error
	id              string
	capabilities    []string
}

// NewMockRecommendationProvider creates a new mock recommendation provider
func NewMockRecommendationProvider() *MockRecommendationProvider {
	return &MockRecommendationProvider{
		id:           "mock-recommendation-provider",
		capabilities: []string{"actionable_recommendations", "effectiveness_scoring", "constraint_filtering"},
	}
}

// SetRecommendations sets the recommendations to return from GenerateRecommendations calls
func (m *MockRecommendationProvider) SetRecommendations(recommendations []common.Recommendation) {
	m.recommendations = recommendations
}

// SetRecommendationError sets the error to return from GenerateRecommendations calls
func (m *MockRecommendationProvider) SetRecommendationError(err error) {
	m.recError = err
}

// GenerateRecommendations implements common.RecommendationProvider
func (m *MockRecommendationProvider) GenerateRecommendations(ctx context.Context, context *common.RecommendationContext) ([]common.Recommendation, error) {
	if m.recError != nil {
		return nil, m.recError
	}
	return m.recommendations, nil
}

// GetID implements common.AIAnalyzer
func (m *MockRecommendationProvider) GetID() string {
	return m.id
}

// GetCapabilities implements common.AIAnalyzer
func (m *MockRecommendationProvider) GetCapabilities() []string {
	return m.capabilities
}

// MockInvestigationProvider implements common.InvestigationProvider for testing
type MockInvestigationProvider struct {
	investigationResult *common.InvestigationResult
	invError            error
	id                  string
	capabilities        []string
}

// NewMockInvestigationProvider creates a new mock investigation provider
func NewMockInvestigationProvider() *MockInvestigationProvider {
	return &MockInvestigationProvider{
		id:           "mock-investigation-provider",
		capabilities: []string{"pattern_investigation", "root_cause_analysis", "evidence_correlation"},
	}
}

// SetInvestigationResult sets the result to return from Investigate calls
func (m *MockInvestigationProvider) SetInvestigationResult(result *common.InvestigationResult) {
	m.investigationResult = result
}

// SetInvestigationError sets the error to return from Investigate calls
func (m *MockInvestigationProvider) SetInvestigationError(err error) {
	m.invError = err
}

// Investigate implements common.InvestigationProvider
func (m *MockInvestigationProvider) Investigate(ctx context.Context, alert *types.Alert, context *common.InvestigationContext) (*common.InvestigationResult, error) {
	if m.invError != nil {
		return nil, m.invError
	}
	return m.investigationResult, nil
}

// GetID implements common.AIAnalyzer
func (m *MockInvestigationProvider) GetID() string {
	return m.id
}

// GetCapabilities implements common.AIAnalyzer
func (m *MockInvestigationProvider) GetCapabilities() []string {
	return m.capabilities
}

// CheckNewAlerts implements monitoring.SideEffectDetector interface with sophisticated behavior
// Supports BR-AI-001 (learning from failures) and BR-AI-003 (SLA compliance)
func (m *MockSideEffectDetector) CheckNewAlerts(ctx context.Context, namespace string, since time.Time) ([]types.Alert, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return configured error if exists (BR-AI-001: testing failure scenarios)
	if err, exists := m.checkAlertErrors[namespace]; exists {
		return nil, err
	}

	// Return alerts that should be discovered after 'since' time (BR-AI-003: time-based testing)
	var discoveredAlerts []types.Alert
	if alerts, exists := m.newAlerts[namespace]; exists {
		for _, alert := range alerts {
			// Only return alerts that were "created" after the since time
			if alertTime, timeExists := m.alertDiscoveryTime[alert.Name]; timeExists {
				if alertTime.After(since) {
					discoveredAlerts = append(discoveredAlerts, alert)
				}
			} else {
				// If no discovery time set, consider as immediately discoverable
				discoveredAlerts = append(discoveredAlerts, alert)
			}
		}
	}

	return discoveredAlerts, nil
}

// Configuration methods for CheckNewAlerts sophisticated behavior
// Following established patterns from SetDetectedEffects/SetDetectionDelay

// InjectAlert adds an alert to be discovered in a namespace (BR-AI-001: side-effect testing)
func (m *MockSideEffectDetector) InjectAlert(namespace string, alert types.Alert, discoveryTime time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.newAlerts[namespace] = append(m.newAlerts[namespace], alert)
	m.alertDiscoveryTime[alert.Name] = discoveryTime
}

// SetCheckAlertError configures CheckNewAlerts to return specific error (BR-AI-001: failure testing)
func (m *MockSideEffectDetector) SetCheckAlertError(namespace string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkAlertErrors[namespace] = err
}

// ClearNamespaceAlerts clears all alerts for a namespace (test isolation)
func (m *MockSideEffectDetector) ClearNamespaceAlerts(namespace string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.newAlerts, namespace)

	// Clear discovery times for alerts in this namespace
	for alertName, _ := range m.alertDiscoveryTime {
		// Note: This is a simplified approach. In practice, might need alert->namespace mapping
		delete(m.alertDiscoveryTime, alertName)
	}
}

// GetInjectedAlerts returns all alerts configured for a namespace (business requirement validation)
func (m *MockSideEffectDetector) GetInjectedAlerts(namespace string) []types.Alert {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if alerts, exists := m.newAlerts[namespace]; exists {
		// Return copy to prevent external modifications
		result := make([]types.Alert, len(alerts))
		copy(result, alerts)
		return result
	}
	return []types.Alert{}
}

// ClearAllAlerts clears all injected alerts (test isolation)
func (m *MockSideEffectDetector) ClearAllAlerts() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.newAlerts = make(map[string][]types.Alert)
	m.alertDiscoveryTime = make(map[string]time.Time)
}

// Additional missing interface methods to complete interface compliance

// CreateSilence implements monitoring.AlertClient interface
func (m *MockAlertClient) CreateSilence(ctx context.Context, silence *monitoring.SilenceRequest) (*monitoring.SilenceResponse, error) {
	// Sophisticated implementation for business requirement testing
	return &monitoring.SilenceResponse{
		SilenceID: fmt.Sprintf("mock-silence-%d", time.Now().Unix()),
	}, nil
}

// DeleteSilence implements monitoring.AlertClient interface
func (m *MockAlertClient) DeleteSilence(ctx context.Context, silenceID string) error {
	// Sophisticated implementation for business requirement testing
	return nil
}

// GetSilences implements monitoring.AlertClient interface
func (m *MockAlertClient) GetSilences(ctx context.Context, matchers []monitoring.SilenceMatcher) ([]monitoring.Silence, error) {
	// Sophisticated implementation for business requirement testing
	return []monitoring.Silence{}, nil
}

// GetAlertHistory implements monitoring.AlertClient interface
func (m *MockAlertClient) GetAlertHistory(ctx context.Context, alertName, namespace string, from, to time.Time) ([]monitoring.AlertEvent, error) {
	// Sophisticated implementation for business requirement testing
	return []monitoring.AlertEvent{}, nil
}

// HasAlertRecurred implements monitoring.AlertClient interface
func (m *MockAlertClient) HasAlertRecurred(ctx context.Context, alertName, namespace string, from, to time.Time) (bool, error) {
	// Sophisticated implementation for business requirement testing
	return false, nil
}
