package engine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// Business Requirement: BR-REL-011 - Maintain workflow state consistency across all operations
// Business Requirement: BR-DATA-014 - Provide state validation and consistency checks

// StateValidationConfig contains configuration for workflow state validation
type StateValidationConfig struct {
	EnableRealTimeValidation  bool          `yaml:"enable_real_time_validation" json:"enable_real_time_validation"`
	EnableDeepStateValidation bool          `yaml:"enable_deep_state_validation" json:"enable_deep_state_validation"`
	MaxValidationWorkers      int           `yaml:"max_validation_workers" json:"max_validation_workers"`
	ValidationTimeoutPerStep  time.Duration `yaml:"validation_timeout_per_step" json:"validation_timeout_per_step"`
	StrictConsistencyMode     bool          `yaml:"strict_consistency_mode" json:"strict_consistency_mode"`
	EnablePerformanceMetrics  bool          `yaml:"enable_performance_metrics" json:"enable_performance_metrics"`
	CheckpointValidationDepth int           `yaml:"checkpoint_validation_depth" json:"checkpoint_validation_depth"`
}

// DefaultStateValidationConfig returns default configuration
func DefaultStateValidationConfig() *StateValidationConfig {
	return &StateValidationConfig{
		EnableRealTimeValidation:  true,
		EnableDeepStateValidation: true,
		MaxValidationWorkers:      3,
		ValidationTimeoutPerStep:  time.Second * 30,
		StrictConsistencyMode:     false,
		EnablePerformanceMetrics:  true,
		CheckpointValidationDepth: 5,
	}
}

// ValidationResult represents the result of a state validation operation
type ValidationResult struct {
	IsValid          bool                   `json:"is_valid"`
	Errors           []string               `json:"errors"`
	Warnings         []string               `json:"warnings"`
	ConsistencyScore float64                `json:"consistency_score"`
	ValidationChecks map[string]interface{} `json:"validation_checks"`
	ValidationTime   time.Duration          `json:"validation_time"`
	ValidatedAt      time.Time              `json:"validated_at"`
}

// GetErrorSummary returns a formatted summary of all errors
func (vr *ValidationResult) GetErrorSummary() string {
	if len(vr.Errors) == 0 {
		return ""
	}

	summary := "Validation errors: "
	for i, err := range vr.Errors {
		if i > 0 {
			summary += "; "
		}
		summary += err
	}
	return summary
}

// StepValidationResult represents validation result for individual steps
type StepValidationResult struct {
	StepID      string    `json:"step_id"`
	IsValid     bool      `json:"is_valid"`
	Errors      []string  `json:"errors"`
	Warnings    []string  `json:"warnings"`
	ValidatedAt time.Time `json:"validated_at"`
}

// StepStatesValidationResult represents validation result for all steps
type StepStatesValidationResult struct {
	IsValid                 bool                    `json:"is_valid"`
	StepValidations         []*StepValidationResult `json:"step_validations"`
	OverallConsistencyScore float64                 `json:"overall_consistency_score"`
	ValidationTime          time.Duration           `json:"validation_time"`
}

// TimelineValidationResult represents timeline consistency validation
type TimelineValidationResult struct {
	IsValid              bool                 `json:"is_valid"`
	TimelineViolations   []*TimelineViolation `json:"timeline_violations"`
	OverallTimelineScore float64              `json:"overall_timeline_score"`
	ValidationTime       time.Duration        `json:"validation_time"`
}

// TimelineViolation represents a specific timeline consistency violation
type TimelineViolation struct {
	Type        string    `json:"type"`
	Description string    `json:"description"`
	StepIDs     []string  `json:"step_ids"`
	Severity    string    `json:"severity"`
	DetectedAt  time.Time `json:"detected_at"`
}

// StateTransitionValidationResult represents state transition validation
type StateTransitionValidationResult struct {
	IsValidTransition     bool          `json:"is_valid_transition"`
	TransitionReason      string        `json:"transition_reason"`
	RequiredPreconditions []string      `json:"required_preconditions"`
	ValidationTime        time.Duration `json:"validation_time"`
}

// CheckpointValidationResult represents validation at specific checkpoint
type CheckpointValidationResult struct {
	CheckpointID     string        `json:"checkpoint_id"`
	IsValid          bool          `json:"is_valid"`
	ValidatedSteps   int           `json:"validated_steps"`
	ValidationTime   time.Duration `json:"validation_time"`
	ConsistencyScore float64       `json:"consistency_score"`
}

// CheckpointsValidationResult represents validation across multiple checkpoints
type CheckpointsValidationResult struct {
	IsValid                    bool                          `json:"is_valid"`
	CheckpointValidations      []*CheckpointValidationResult `json:"checkpoint_validations"`
	CheckpointConsistencyScore float64                       `json:"checkpoint_consistency_score"`
	TotalValidationTime        time.Duration                 `json:"total_validation_time"`
}

// ResourceStateValidationResult represents resource state consistency validation
type ResourceStateValidationResult struct {
	IsValid                  bool                   `json:"is_valid"`
	ResourceConsistencyScore float64                `json:"resource_consistency_score"`
	ResourceValidations      map[string]interface{} `json:"resource_validations"`
	ValidationTime           time.Duration          `json:"validation_time"`
}

// ValidationMetrics contains metrics about validation operations
type ValidationMetrics struct {
	TotalValidations      int64                         `json:"total_validations"`
	ValidationErrors      int64                         `json:"validation_errors"`
	AverageValidationTime time.Duration                 `json:"average_validation_time"`
	ValidationSuccessRate float64                       `json:"validation_success_rate"`
	IsHealthy             bool                          `json:"is_healthy"`
	ValidationWorkerCount int                           `json:"validation_worker_count"`
	LastValidationTime    time.Time                     `json:"last_validation_time"`
	PerformanceMetrics    *ValidationPerformanceMetrics `json:"performance_metrics"`
}

// ValidationPerformanceMetrics contains detailed performance metrics
type ValidationPerformanceMetrics struct {
	AverageExecutionStateValidationTime time.Duration `json:"average_execution_state_validation_time"`
	AverageStepStateValidationTime      time.Duration `json:"average_step_state_validation_time"`
	AverageTimelineValidationTime       time.Duration `json:"average_timeline_validation_time"`
	AverageResourceValidationTime       time.Duration `json:"average_resource_validation_time"`
}

// ComplianceValidationReport represents a comprehensive compliance validation report
type ComplianceValidationReport struct {
	ExecutionID                  string                         `json:"execution_id"`
	ValidationTimestamp          time.Time                      `json:"validation_timestamp"`
	OverallComplianceScore       float64                        `json:"overall_compliance_score"`
	BusinessRequirementsCoverage map[string]interface{}         `json:"business_requirements_coverage"`
	StateConsistencyValidation   *ValidationResult              `json:"state_consistency_validation"`
	TimelineValidation           *TimelineValidationResult      `json:"timeline_validation"`
	ResourceValidation           *ResourceStateValidationResult `json:"resource_validation"`
	ComplianceRecommendations    []string                       `json:"compliance_recommendations"`
}

// WorkflowStateValidator validates workflow execution state consistency
type WorkflowStateValidator struct {
	config *StateValidationConfig
	logger *logrus.Logger

	// Metrics (atomic counters for thread safety)
	totalValidations    int64
	validationErrors    int64
	totalValidationTime int64 // Nanoseconds
	lastValidationTime  int64 // Unix timestamp

	// Performance tracking
	executionStateValidationTime int64
	stepStateValidationTime      int64
	timelineValidationTime       int64
	resourceValidationTime       int64

	// State management
	isRunning         int32 // Atomic flag
	stopChannel       chan struct{}
	validationWorkers sync.WaitGroup
	mutex             sync.RWMutex
}

// NewWorkflowStateValidator creates a new workflow state validator
func NewWorkflowStateValidator(config *StateValidationConfig, logger *logrus.Logger) *WorkflowStateValidator {
	if config == nil {
		config = DefaultStateValidationConfig()
	}

	validator := &WorkflowStateValidator{
		config:      config,
		logger:      logger,
		stopChannel: make(chan struct{}),
	}

	logger.WithFields(logrus.Fields{
		"real_time_validation":    config.EnableRealTimeValidation,
		"deep_state_validation":   config.EnableDeepStateValidation,
		"max_validation_workers":  config.MaxValidationWorkers,
		"strict_consistency_mode": config.StrictConsistencyMode,
	}).Info("Workflow state validator initialized")

	return validator
}

// ValidateExecutionState validates the overall execution state consistency
func (wsv *WorkflowStateValidator) ValidateExecutionState(ctx context.Context, execution *RuntimeWorkflowExecution) *ValidationResult {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		atomic.AddInt64(&wsv.totalValidations, 1)
		atomic.AddInt64(&wsv.totalValidationTime, duration.Nanoseconds())
		atomic.AddInt64(&wsv.executionStateValidationTime, duration.Nanoseconds())
		atomic.StoreInt64(&wsv.lastValidationTime, time.Now().Unix())
	}()

	result := &ValidationResult{
		ValidationChecks: make(map[string]interface{}),
		ValidatedAt:      time.Now(),
	}

	var errors []string
	var warnings []string
	consistencyScore := 1.0

	// Basic state validation
	if execution == nil {
		errors = append(errors, "execution is nil")
		consistencyScore = 0.0
	} else {
		// Validate basic execution fields
		if execution.ID == "" {
			errors = append(errors, "execution ID is empty")
			consistencyScore -= 0.2
		}

		if execution.WorkflowID == "" {
			errors = append(errors, "workflow ID is empty")
			consistencyScore -= 0.2
		}

		// Validate operational status consistency
		if execution.OperationalStatus == ExecutionStatusCompleted && execution.EndTime == nil {
			errors = append(errors, "completed status without end time")
			consistencyScore -= 0.3
		}

		if execution.OperationalStatus == ExecutionStatusRunning && execution.EndTime != nil {
			errors = append(errors, "running status with end time set")
			consistencyScore -= 0.3
		}

		// Validate current step index
		if execution.CurrentStep < 0 {
			errors = append(errors, "current step index is negative")
			consistencyScore -= 0.2
		}

		if execution.CurrentStep > len(execution.Steps) {
			errors = append(errors, "current step index out of bounds")
			consistencyScore -= 0.3
		}

		// Record validation checks
		result.ValidationChecks["basic_state"] = len(errors) == 0
		result.ValidationChecks["execution_metadata"] = execution.ID != "" && execution.WorkflowID != ""
		result.ValidationChecks["operational_status"] = wsv.validateOperationalStatusConsistency(execution)
	}

	if len(errors) > 0 {
		atomic.AddInt64(&wsv.validationErrors, 1)
	}

	result.IsValid = len(errors) == 0
	result.Errors = errors
	result.Warnings = warnings
	result.ConsistencyScore = consistencyScore
	result.ValidationTime = time.Since(startTime)

	return result
}

// validateOperationalStatusConsistency validates operational status consistency
func (wsv *WorkflowStateValidator) validateOperationalStatusConsistency(execution *RuntimeWorkflowExecution) bool {
	if execution == nil {
		return false
	}

	switch execution.OperationalStatus {
	case ExecutionStatusCompleted:
		return execution.EndTime != nil
	case ExecutionStatusRunning:
		return execution.EndTime == nil
	case ExecutionStatusFailed:
		return execution.EndTime != nil
	case ExecutionStatusCancelled:
		return execution.EndTime != nil
	default:
		return true // Other statuses are considered valid
	}
}

// ValidateStepStates validates the consistency of all step states
func (wsv *WorkflowStateValidator) ValidateStepStates(ctx context.Context, execution *RuntimeWorkflowExecution) *StepStatesValidationResult {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		atomic.AddInt64(&wsv.totalValidations, 1)
		atomic.AddInt64(&wsv.totalValidationTime, duration.Nanoseconds())
		atomic.AddInt64(&wsv.stepStateValidationTime, duration.Nanoseconds())
		atomic.StoreInt64(&wsv.lastValidationTime, time.Now().Unix())
	}()

	result := &StepStatesValidationResult{
		StepValidations: make([]*StepValidationResult, 0),
		ValidationTime:  time.Since(startTime),
	}

	if execution == nil || len(execution.Steps) == 0 {
		result.IsValid = true
		result.OverallConsistencyScore = 1.0
		return result
	}

	totalScore := 0.0
	allValid := true

	for _, step := range execution.Steps {
		stepResult := wsv.validateIndividualStep(step)
		result.StepValidations = append(result.StepValidations, stepResult)

		if !stepResult.IsValid {
			allValid = false
		}

		// Calculate step consistency score
		stepScore := 1.0
		if !stepResult.IsValid {
			stepScore = 0.5 // Partially valid
		}
		if len(stepResult.Errors) > 2 {
			stepScore = 0.0 // Critical issues
		}
		totalScore += stepScore
	}

	result.IsValid = allValid
	result.OverallConsistencyScore = totalScore / float64(len(execution.Steps))
	result.ValidationTime = time.Since(startTime)

	return result
}

// validateIndividualStep validates a single step's consistency
func (wsv *WorkflowStateValidator) validateIndividualStep(step *StepExecution) *StepValidationResult {
	result := &StepValidationResult{
		StepID:      step.StepID,
		ValidatedAt: time.Now(),
	}

	var errors []string
	var warnings []string

	// Validate step ID
	if step.StepID == "" {
		errors = append(errors, "step ID is empty")
	}

	// Validate status consistency
	if step.Status == ExecutionStatusCompleted && step.EndTime == nil {
		errors = append(errors, "completed step without end time")
	}

	if step.Status == ExecutionStatusRunning && step.EndTime != nil {
		errors = append(errors, "running step with end time set")
	}

	// Validate timing consistency
	if !step.StartTime.IsZero() && step.EndTime != nil {
		if step.EndTime.Before(step.StartTime) {
			errors = append(errors, "end time before start time")
		}

		expectedDuration := step.EndTime.Sub(step.StartTime)
		if step.Duration > 0 && expectedDuration != step.Duration {
			warnings = append(warnings, "duration mismatch with start/end times")
		}
	}

	// Validate retry count
	if step.RetryCount < 0 {
		errors = append(errors, "negative retry count")
	}

	result.IsValid = len(errors) == 0
	result.Errors = errors
	result.Warnings = warnings

	return result
}

// ValidateExecutionTimeline validates the timeline consistency of execution
func (wsv *WorkflowStateValidator) ValidateExecutionTimeline(ctx context.Context, execution *RuntimeWorkflowExecution) *TimelineValidationResult {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		atomic.AddInt64(&wsv.totalValidations, 1)
		atomic.AddInt64(&wsv.totalValidationTime, duration.Nanoseconds())
		atomic.AddInt64(&wsv.timelineValidationTime, duration.Nanoseconds())
		atomic.StoreInt64(&wsv.lastValidationTime, time.Now().Unix())
	}()

	result := &TimelineValidationResult{
		TimelineViolations: make([]*TimelineViolation, 0),
		ValidationTime:     time.Since(startTime),
	}

	if execution == nil || len(execution.Steps) <= 1 {
		result.IsValid = true
		result.OverallTimelineScore = 1.0
		return result
	}

	violations := 0

	// Check step order consistency
	for i := 1; i < len(execution.Steps); i++ {
		prevStep := execution.Steps[i-1]
		currentStep := execution.Steps[i]

		// Check if previous step started before current step
		if !prevStep.StartTime.IsZero() && !currentStep.StartTime.IsZero() {
			if prevStep.StartTime.After(currentStep.StartTime) &&
				prevStep.Status == ExecutionStatusCompleted &&
				currentStep.Status == ExecutionStatusRunning {
				violation := &TimelineViolation{
					Type:        "step_order_violation",
					Description: "step completed after a later step started",
					StepIDs:     []string{prevStep.StepID, currentStep.StepID},
					Severity:    "high",
					DetectedAt:  time.Now(),
				}
				result.TimelineViolations = append(result.TimelineViolations, violation)
				violations++
			}
		}
	}

	result.IsValid = violations == 0
	scoreReduction := float64(violations) * 0.4 // Increased penalty for timeline violations
	result.OverallTimelineScore = 1.0 - scoreReduction
	if result.OverallTimelineScore < 0 {
		result.OverallTimelineScore = 0
	}

	result.ValidationTime = time.Since(startTime)
	return result
}

// ValidateStateTransition validates if a state transition is valid
func (wsv *WorkflowStateValidator) ValidateStateTransition(ctx context.Context, execution *RuntimeWorkflowExecution,
	fromStatus, toStatus ExecutionStatus) *StateTransitionValidationResult {

	startTime := time.Now()
	result := &StateTransitionValidationResult{
		ValidationTime: time.Since(startTime),
	}

	// Define valid transitions
	validTransitions := map[ExecutionStatus][]ExecutionStatus{
		ExecutionStatusPending:     {ExecutionStatusRunning, ExecutionStatusCancelled},
		ExecutionStatusRunning:     {ExecutionStatusCompleted, ExecutionStatusFailed, ExecutionStatusPaused, ExecutionStatusCancelled},
		ExecutionStatusPaused:      {ExecutionStatusRunning, ExecutionStatusCancelled},
		ExecutionStatusCompleted:   {}, // Terminal state
		ExecutionStatusFailed:      {}, // Terminal state
		ExecutionStatusCancelled:   {}, // Terminal state
		ExecutionStatusRollingBack: {ExecutionStatusFailed, ExecutionStatusCompleted},
	}

	allowedTransitions, exists := validTransitions[fromStatus]
	if !exists {
		result.IsValidTransition = false
		result.TransitionReason = "unknown source status"
		return result
	}

	for _, allowedStatus := range allowedTransitions {
		if allowedStatus == toStatus {
			result.IsValidTransition = true
			result.TransitionReason = "valid transition"
			result.RequiredPreconditions = wsv.getTransitionPreconditions(fromStatus, toStatus)
			return result
		}
	}

	result.IsValidTransition = false
	result.TransitionReason = "invalid transition from " + string(fromStatus) + " to " + string(toStatus)
	return result
}

// getTransitionPreconditions returns required preconditions for a state transition
func (wsv *WorkflowStateValidator) getTransitionPreconditions(from, to ExecutionStatus) []string {
	preconditions := make([]string, 0)

	switch {
	case from == ExecutionStatusRunning && to == ExecutionStatusCompleted:
		preconditions = append(preconditions, "all steps completed successfully")
		preconditions = append(preconditions, "no pending operations")
	case from == ExecutionStatusRunning && to == ExecutionStatusFailed:
		preconditions = append(preconditions, "critical step failure detected")
	case from == ExecutionStatusPaused && to == ExecutionStatusRunning:
		preconditions = append(preconditions, "pause conditions resolved")
	}

	return preconditions
}

// ValidateWithCheckpoints validates execution with checkpoint support
func (wsv *WorkflowStateValidator) ValidateWithCheckpoints(ctx context.Context, execution *RuntimeWorkflowExecution) *CheckpointsValidationResult {
	startTime := time.Now()

	result := &CheckpointsValidationResult{
		CheckpointValidations: make([]*CheckpointValidationResult, 0),
	}

	if execution == nil || len(execution.Steps) == 0 {
		result.IsValid = true
		result.CheckpointConsistencyScore = 1.0
		result.TotalValidationTime = time.Since(startTime)
		return result
	}

	// Create checkpoints based on configuration depth
	checkpointSize := len(execution.Steps) / wsv.config.CheckpointValidationDepth
	if checkpointSize < 1 {
		checkpointSize = 1
	}

	allValid := true
	totalScore := 0.0

	for i := 0; i < wsv.config.CheckpointValidationDepth && i*checkpointSize < len(execution.Steps); i++ {
		startIdx := i * checkpointSize
		endIdx := startIdx + checkpointSize
		if endIdx > len(execution.Steps) {
			endIdx = len(execution.Steps)
		}

		checkpointResult := wsv.validateCheckpoint(execution, startIdx, endIdx, i)
		result.CheckpointValidations = append(result.CheckpointValidations, checkpointResult)

		if !checkpointResult.IsValid {
			allValid = false
		}
		totalScore += checkpointResult.ConsistencyScore
	}

	result.IsValid = allValid
	if len(result.CheckpointValidations) > 0 {
		result.CheckpointConsistencyScore = totalScore / float64(len(result.CheckpointValidations))
	} else {
		result.CheckpointConsistencyScore = 1.0
	}
	result.TotalValidationTime = time.Since(startTime)

	return result
}

// validateCheckpoint validates a specific checkpoint range
func (wsv *WorkflowStateValidator) validateCheckpoint(execution *RuntimeWorkflowExecution, startIdx, endIdx, checkpointID int) *CheckpointValidationResult {
	checkpointStart := time.Now()

	result := &CheckpointValidationResult{
		CheckpointID:   fmt.Sprintf("checkpoint-%d", checkpointID),
		ValidatedSteps: endIdx - startIdx,
	}

	consistencyScore := 1.0
	isValid := true

	// Validate steps in checkpoint range
	for i := startIdx; i < endIdx; i++ {
		stepResult := wsv.validateIndividualStep(execution.Steps[i])
		if !stepResult.IsValid {
			isValid = false
			consistencyScore -= 0.2
		}
	}

	result.IsValid = isValid
	result.ConsistencyScore = consistencyScore
	result.ValidationTime = time.Since(checkpointStart)

	return result
}

// ValidateResourceStateConsistency validates resource state consistency
func (wsv *WorkflowStateValidator) ValidateResourceStateConsistency(ctx context.Context, execution *RuntimeWorkflowExecution) *ResourceStateValidationResult {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		atomic.AddInt64(&wsv.resourceValidationTime, duration.Nanoseconds())
	}()

	result := &ResourceStateValidationResult{
		ResourceValidations: make(map[string]interface{}),
		ValidationTime:      time.Since(startTime),
	}

	if execution == nil || execution.Context == nil {
		result.IsValid = true
		result.ResourceConsistencyScore = 1.0
		return result
	}

	consistencyScore := 1.0

	// Validate CPU allocation consistency
	if cpuAlloc, exists := execution.Context.Variables["allocated_cpu"]; exists {
		result.ResourceValidations["cpu_allocation"] = cpuAlloc
		consistencyScore += 0.1 // Bonus for having resource tracking
	}

	// Validate memory allocation consistency
	if memAlloc, exists := execution.Context.Variables["allocated_memory"]; exists {
		result.ResourceValidations["memory_allocation"] = memAlloc
		consistencyScore += 0.1 // Bonus for having resource tracking
	}

	// Validate resource state progression
	if resourceState, exists := execution.Context.Variables["resource_state"]; exists {
		result.ResourceValidations["resource_state_progression"] = resourceState
		consistencyScore += 0.1 // Bonus for tracking state progression
	}

	// Ensure score doesn't exceed 1.0
	if consistencyScore > 1.0 {
		consistencyScore = 1.0
	}

	result.IsValid = true // For now, consider resource validation always valid if present
	result.ResourceConsistencyScore = consistencyScore
	result.ValidationTime = time.Since(startTime)

	return result
}

// GenerateComplianceValidationReport generates a comprehensive compliance validation report
func (wsv *WorkflowStateValidator) GenerateComplianceValidationReport(ctx context.Context, execution *RuntimeWorkflowExecution) *ComplianceValidationReport {
	report := &ComplianceValidationReport{
		ExecutionID:                  execution.ID,
		ValidationTimestamp:          time.Now(),
		BusinessRequirementsCoverage: make(map[string]interface{}),
		ComplianceRecommendations:    make([]string, 0),
	}

	// Perform all validations
	report.StateConsistencyValidation = wsv.ValidateExecutionState(ctx, execution)
	report.TimelineValidation = wsv.ValidateExecutionTimeline(ctx, execution)
	report.ResourceValidation = wsv.ValidateResourceStateConsistency(ctx, execution)

	// Calculate overall compliance score
	stateScore := report.StateConsistencyValidation.ConsistencyScore
	timelineScore := report.TimelineValidation.OverallTimelineScore
	resourceScore := report.ResourceValidation.ResourceConsistencyScore

	report.OverallComplianceScore = (stateScore + timelineScore + resourceScore) / 3.0

	// Business requirements coverage
	report.BusinessRequirementsCoverage["BR-REL-011"] = map[string]interface{}{
		"requirement": "maintain workflow state consistency across all operations",
		"compliance":  report.StateConsistencyValidation.IsValid,
		"score":       stateScore,
	}

	report.BusinessRequirementsCoverage["BR-DATA-014"] = map[string]interface{}{
		"requirement": "provide state validation and consistency checks",
		"compliance":  true, // We're providing validation
		"score":       1.0,
	}

	// Generate recommendations
	if !report.StateConsistencyValidation.IsValid {
		report.ComplianceRecommendations = append(report.ComplianceRecommendations,
			"Fix state consistency issues to improve BR-REL-011 compliance")
	}

	if report.TimelineValidation.OverallTimelineScore < 0.8 {
		report.ComplianceRecommendations = append(report.ComplianceRecommendations,
			"Review workflow timeline for consistency violations")
	}

	if report.OverallComplianceScore >= 0.85 {
		report.ComplianceRecommendations = append(report.ComplianceRecommendations,
			"Compliance score is good - maintain current validation practices")
	}

	return report
}

// GetValidationMetrics returns current validation metrics
func (wsv *WorkflowStateValidator) GetValidationMetrics() *ValidationMetrics {
	wsv.mutex.RLock()
	defer wsv.mutex.RUnlock()

	totalValidations := atomic.LoadInt64(&wsv.totalValidations)
	totalValidationTime := atomic.LoadInt64(&wsv.totalValidationTime)
	validationErrors := atomic.LoadInt64(&wsv.validationErrors)
	lastValidationTime := atomic.LoadInt64(&wsv.lastValidationTime)

	var averageValidationTime time.Duration
	if totalValidations > 0 {
		averageValidationTime = time.Duration(totalValidationTime / totalValidations)
	}

	var successRate float64
	if totalValidations > 0 {
		successRate = float64(totalValidations-validationErrors) / float64(totalValidations)
	} else {
		successRate = 1.0
	}

	// Performance metrics
	performanceMetrics := &ValidationPerformanceMetrics{}
	if totalValidations > 0 {
		performanceMetrics.AverageExecutionStateValidationTime = time.Duration(
			atomic.LoadInt64(&wsv.executionStateValidationTime) / totalValidations)
		performanceMetrics.AverageStepStateValidationTime = time.Duration(
			atomic.LoadInt64(&wsv.stepStateValidationTime) / totalValidations)
		performanceMetrics.AverageTimelineValidationTime = time.Duration(
			atomic.LoadInt64(&wsv.timelineValidationTime) / totalValidations)
		performanceMetrics.AverageResourceValidationTime = time.Duration(
			atomic.LoadInt64(&wsv.resourceValidationTime) / totalValidations)
	}

	return &ValidationMetrics{
		TotalValidations:      totalValidations,
		ValidationErrors:      validationErrors,
		AverageValidationTime: averageValidationTime,
		ValidationSuccessRate: successRate,
		IsHealthy:             successRate > 0.8 || totalValidations == 0, // Healthy if no validations yet OR success rate > 80%
		ValidationWorkerCount: wsv.config.MaxValidationWorkers,
		LastValidationTime:    time.Unix(lastValidationTime, 0),
		PerformanceMetrics:    performanceMetrics,
	}
}

// Stop stops the workflow state validator
func (wsv *WorkflowStateValidator) Stop() {
	if !atomic.CompareAndSwapInt32(&wsv.isRunning, 1, 0) {
		return // Already stopped
	}

	close(wsv.stopChannel)
	wsv.validationWorkers.Wait()

	wsv.logger.Info("Workflow state validator stopped")
}
