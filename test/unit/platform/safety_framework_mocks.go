package platform

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// MockSafetyValidator implements validation interfaces for safety framework testing
type MockSafetyValidator struct {
	logger *logrus.Logger

	// Mock results
	clusterAccessResult   *ClusterValidationResult
	resourceStateResult   *ResourceValidationResult
	riskAssessmentResults map[string]*RiskAssessment
	rollbackValidations   map[string]*RollbackValidationResult

	// Policy storage
	policies []SafetyPolicy

	// Audit trail storage
	auditTrail []SafetyAuditEntry
	auditMutex sync.RWMutex

	// Call tracking
	rollbackStates map[string]*RollbackState
	stateMutex     sync.RWMutex
}

// ClusterValidationResult represents cluster connectivity validation results
type ClusterValidationResult struct {
	IsValid           bool
	ConnectivityCheck bool
	PermissionLevel   string
	ErrorMessage      string
	RiskLevel         string
}

// ResourceValidationResult represents resource state validation results
type ResourceValidationResult struct {
	IsValid        bool
	ResourceExists bool
	CurrentState   string
	HealthStatus   string
	ErrorMessage   string
}

// RiskAssessment represents action risk assessment results
type RiskAssessment struct {
	RiskLevel   string
	RiskScore   float64
	RiskFactors []string
	Mitigation  *MitigationPlan
}

// MitigationPlan represents risk mitigation strategies
type MitigationPlan struct {
	RequiredApprovals int
	SafetyMeasures    []string
	RollbackPlan      string
	TimeoutOverride   time.Duration
}

// RollbackValidationResult represents rollback operation validation
type RollbackValidationResult struct {
	IsValid                  bool
	TargetRevisionExists     bool
	RollbackImpactAssessment *RollbackImpact
	EstimatedDowntime        time.Duration
	ValidationErrors         []string
	RiskLevel                string
}

// RollbackImpact represents the impact assessment of a rollback operation
type RollbackImpact struct {
	AffectedReplicas  int
	AffectedServices  []string
	DataIntegrityRisk string
	PerformanceImpact string
}

// RollbackState represents captured state information for rollback capability
type RollbackState struct {
	ActionID           string
	PreviousState      map[string]interface{}
	Timestamp          time.Time
	RollbackCapability bool
	ExpirationTime     time.Time
}

// RollbackRequest represents a rollback operation request
type RollbackRequest struct {
	Namespace      string
	ResourceName   string
	ResourceType   string
	TargetRevision int
	Reason         string
}

// SafetyPolicy represents a safety policy for action filtering
type SafetyPolicy struct {
	Name        string
	Environment string
	ActionType  string
	Rules       map[string]interface{}
}

// PolicyFilterResult represents policy filtering results
type PolicyFilterResult struct {
	Allowed           bool
	RequiredApprovals int
	PolicyViolations  []string
	ApprovalWorkflow  string
}

// SafetyAuditEntry represents an audit trail entry for safety decisions
type SafetyAuditEntry struct {
	Timestamp     time.Time
	ActionType    string
	Decision      string
	DecisionBy    string
	Reason        string
	Environment   string
	RiskLevel     string
	PolicyApplied string
}

// NewMockSafetyValidator creates a new mock safety validator
func NewMockSafetyValidator(logger *logrus.Logger) *MockSafetyValidator {
	return &MockSafetyValidator{
		logger: logger,
		clusterAccessResult: &ClusterValidationResult{
			IsValid:           true,
			ConnectivityCheck: true,
			PermissionLevel:   "admin",
			ErrorMessage:      "",
			RiskLevel:         "LOW",
		},
		resourceStateResult: &ResourceValidationResult{
			IsValid:        true,
			ResourceExists: true,
			CurrentState:   "Available",
			HealthStatus:   "Healthy",
			ErrorMessage:   "",
		},
		riskAssessmentResults: make(map[string]*RiskAssessment),
		rollbackValidations:   make(map[string]*RollbackValidationResult),
		policies:              []SafetyPolicy{},
		auditTrail:            []SafetyAuditEntry{},
		rollbackStates:        make(map[string]*RollbackState),
	}
}

// Validator interface implementation
func (m *MockSafetyValidator) ValidateClusterAccess(ctx context.Context, namespace string) *ClusterValidationResult {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	return m.clusterAccessResult
}

func (m *MockSafetyValidator) ValidateResourceState(ctx context.Context, namespace, resourceName, resourceType string) *ResourceValidationResult {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	if resourceName == "non-existent-pod" || resourceName == "non-existent-deployment" {
		return &ResourceValidationResult{
			IsValid:        false,
			ResourceExists: false,
			CurrentState:   "NotFound",
			HealthStatus:   "Unknown",
			ErrorMessage:   "resource not found",
		}
	}
	return m.resourceStateResult
}

func (m *MockSafetyValidator) AssessActionRisk(actionName, environment string, parameters map[string]interface{}) *RiskAssessment {
	// Default risk assessments based on action type
	defaultRiskAssessments := map[string]*RiskAssessment{
		"scale_deployment": {
			RiskLevel:   "LOW",
			RiskScore:   2.5,
			RiskFactors: []string{"reversible", "gradual_impact"},
		},
		"restart_pod": {
			RiskLevel:   "MEDIUM",
			RiskScore:   4.5,
			RiskFactors: []string{"service_interruption", "potential_data_loss"},
		},
		"drain_node": {
			RiskLevel:   "HIGH",
			RiskScore:   7.5,
			RiskFactors: []string{"multiple_workloads_affected", "capacity_reduction"},
		},
		"quarantine_pod": {
			RiskLevel:   "CRITICAL",
			RiskScore:   9.5,
			RiskFactors: []string{"security_isolation", "immediate_service_impact"},
		},
	}

	if assessment, exists := m.riskAssessmentResults[actionName]; exists {
		return assessment
	}

	if defaultAssessment, exists := defaultRiskAssessments[actionName]; exists {
		return defaultAssessment
	}

	// Default fallback
	return &RiskAssessment{
		RiskLevel:   "MEDIUM",
		RiskScore:   5.0,
		RiskFactors: []string{"unknown_risk_profile"},
	}
}

func (m *MockSafetyValidator) GenerateMitigationPlan(riskAssessment *RiskAssessment) *MitigationPlan {
	plan := &MitigationPlan{
		SafetyMeasures: []string{},
		RollbackPlan:   "",
	}

	switch riskAssessment.RiskLevel {
	case "LOW":
		plan.RequiredApprovals = 0
		plan.SafetyMeasures = []string{"logging_enhanced"}
	case "MEDIUM":
		plan.RequiredApprovals = 0
		plan.SafetyMeasures = []string{"validation_required", "monitoring_enhanced"}
		plan.RollbackPlan = "automatic_rollback_enabled"
	case "HIGH":
		plan.RequiredApprovals = 1
		plan.SafetyMeasures = []string{"pre_validation_required", "approval_workflow", "monitoring_enhanced"}
		plan.RollbackPlan = "automatic_rollback_with_validation"
	case "CRITICAL":
		plan.RequiredApprovals = 2
		plan.SafetyMeasures = []string{"multi_level_approval", "manual_validation", "comprehensive_monitoring"}
		plan.RollbackPlan = "manual_rollback_only"
	}

	return plan
}

func (m *MockSafetyValidator) ValidateRollbackRequest(ctx context.Context, request RollbackRequest) *RollbackValidationResult {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	if request.ResourceName == "non-existent-deployment" {
		return &RollbackValidationResult{
			IsValid:              false,
			TargetRevisionExists: false,
			ValidationErrors:     []string{"resource_not_found"},
			RiskLevel:            "BLOCKED",
		}
	}

	return &RollbackValidationResult{
		IsValid:              true,
		TargetRevisionExists: true,
		RollbackImpactAssessment: &RollbackImpact{
			AffectedReplicas:  5,
			AffectedServices:  []string{"web-service", "api-service"},
			DataIntegrityRisk: "LOW",
			PerformanceImpact: "MINIMAL",
		},
		EstimatedDowntime: 25 * time.Second,
		ValidationErrors:  []string{},
		RiskLevel:         "MEDIUM",
	}
}

func (m *MockSafetyValidator) CaptureRollbackState(ctx context.Context, namespace, resourceName, actionType string) *RollbackState {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	m.stateMutex.Lock()
	defer m.stateMutex.Unlock()

	actionID := fmt.Sprintf("rollback-%s-%d", resourceName, time.Now().Unix())

	state := &RollbackState{
		ActionID: actionID,
		PreviousState: map[string]interface{}{
			"replicas":  2,
			"version":   "v1.2.0",
			"namespace": namespace,
		},
		Timestamp:          time.Now(),
		RollbackCapability: true,
		ExpirationTime:     time.Now().Add(24 * time.Hour),
	}

	m.rollbackStates[actionID] = state
	return state
}

func (m *MockSafetyValidator) LoadPolicies(policies []SafetyPolicy) {
	m.policies = policies
}

func (m *MockSafetyValidator) ApplyPolicyFiltering(environment string, action *types.ActionRecommendation) *PolicyFilterResult {
	for _, policy := range m.policies {
		if policy.Environment == environment && policy.ActionType == action.Action {
			// Check policy rules
			if maxReplicas, exists := policy.Rules["max_replicas"]; exists {
				if replicas, ok := action.Parameters["replicas"].(float64); ok {
					if replicas > float64(maxReplicas.(int)) {
						return &PolicyFilterResult{
							Allowed:          false,
							PolicyViolations: []string{"max_replicas_exceeded"},
						}
					}
				}
			}

			// Check approval requirements
			requireApproval, _ := policy.Rules["require_approval"].(bool)
			approvals := 0
			if requireApproval {
				approvals = 1
			}

			return &PolicyFilterResult{
				Allowed:           true,
				RequiredApprovals: approvals,
				PolicyViolations:  []string{},
			}
		}
	}

	// Default allow if no matching policy
	return &PolicyFilterResult{
		Allowed:           true,
		RequiredApprovals: 0,
		PolicyViolations:  []string{},
	}
}

func (m *MockSafetyValidator) RecordSafetyDecision(entry SafetyAuditEntry) {
	m.auditMutex.Lock()
	defer m.auditMutex.Unlock()
	m.auditTrail = append(m.auditTrail, entry)
}

func (m *MockSafetyValidator) GetSafetyAuditTrail(startTime, endTime time.Time) []SafetyAuditEntry {
	m.auditMutex.RLock()
	defer m.auditMutex.RUnlock()

	var filteredEntries []SafetyAuditEntry
	for _, entry := range m.auditTrail {
		if entry.Timestamp.After(startTime) && entry.Timestamp.Before(endTime) {
			filteredEntries = append(filteredEntries, entry)
		}
	}
	return filteredEntries
}

// Mock result setters for test customization
func (m *MockSafetyValidator) SetClusterAccessResult(result *ClusterValidationResult) {
	m.clusterAccessResult = result
}

func (m *MockSafetyValidator) SetResourceStateResult(result *ResourceValidationResult) {
	m.resourceStateResult = result
}

func (m *MockSafetyValidator) SetRiskAssessment(actionName string, assessment *RiskAssessment) {
	m.riskAssessmentResults[actionName] = assessment
}

func (m *MockSafetyValidator) SetRollbackValidation(resourceName string, result *RollbackValidationResult) {
	m.rollbackValidations[resourceName] = result
}

// SetClusterConnectivityResult sets the mock result for cluster connectivity checks
// This method would be added to MockK8sClient in the main mocks file
func SetClusterConnectivityResult(connected bool, err error) {
	// Mock cluster connectivity check - this would be implemented in the real K8s client
	// For testing purposes, we'll track this in the safety validator
}

// NewValidator creates a mock validator for safety framework testing
func NewValidator(logger *logrus.Logger) *MockSafetyValidator {
	return NewMockSafetyValidator(logger)
}

// Note: In a real implementation, this would implement the actual validation.Validator interface
// The MockSafetyValidator provides a comprehensive testing interface for safety framework validation
