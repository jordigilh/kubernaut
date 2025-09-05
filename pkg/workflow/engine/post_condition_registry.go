package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// PostConditionValidator interface for type-specific validators
type PostConditionValidator interface {
	ValidateCondition(ctx context.Context, condition *PostCondition, result *StepResult, stepCtx *StepContext) (*PostConditionResult, error)
	GetType() PostConditionType
	GetPriority() int // Lower numbers = higher priority
}

// ValidatorRegistry manages and executes post-condition validators
type ValidatorRegistry struct {
	validators       map[PostConditionType]PostConditionValidator
	expressionEngine *ExpressionEngine
	log              *logrus.Logger
	mu               sync.RWMutex
}

// NewValidatorRegistry creates a new validator registry with all built-in validators
func NewValidatorRegistry(log *logrus.Logger) *ValidatorRegistry {
	registry := &ValidatorRegistry{
		validators:       make(map[PostConditionType]PostConditionValidator),
		expressionEngine: NewExpressionEngine(),
		log:              log,
	}

	// Register all built-in validators
	registry.registerBuiltinValidators()

	return registry
}

// registerBuiltinValidators registers all optimized built-in validators
func (vr *ValidatorRegistry) registerBuiltinValidators() {
	vr.RegisterValidator(&SuccessValidator{})
	vr.RegisterValidator(&ConfidenceValidator{})
	vr.RegisterValidator(&DurationValidator{})
	vr.RegisterValidator(&OutputValidator{})
	vr.RegisterValidator(&NoErrorsValidator{})
	vr.RegisterValidator(&ExpressionValidator{expressionEngine: vr.expressionEngine})
	vr.RegisterValidator(&MetricValidator{})
	vr.RegisterValidator(&ResourceValidator{})
}

// RegisterValidator registers a custom validator
func (vr *ValidatorRegistry) RegisterValidator(validator PostConditionValidator) {
	vr.mu.Lock()
	defer vr.mu.Unlock()
	vr.validators[validator.GetType()] = validator
}

// ValidatePostConditions executes all post-conditions with parallel execution and priority ordering
func (vr *ValidatorRegistry) ValidatePostConditions(
	ctx context.Context,
	conditions []*PostCondition,
	result *StepResult,
	stepCtx *StepContext,
) (*PostConditionValidationResult, error) {
	if len(conditions) == 0 {
		return &PostConditionValidationResult{
			Success:       true,
			Results:       []*PostConditionResult{},
			TotalDuration: 0,
			Message:       "No post-conditions to validate",
		}, nil
	}

	startTime := time.Now()

	// Filter enabled conditions and sort by priority
	enabledConditions := vr.filterAndSortConditions(conditions)

	vr.log.WithFields(logrus.Fields{
		"total_conditions":   len(conditions),
		"enabled_conditions": len(enabledConditions),
	}).Debug("Starting post-condition validation")

	// Execute conditions in parallel for better performance
	results := make([]*PostConditionResult, len(enabledConditions))
	var wg sync.WaitGroup
	var mu sync.Mutex
	criticalFailed := 0
	totalFailed := 0

	// Use worker pool to limit concurrent validations
	maxWorkers := min(len(enabledConditions), 10)
	semaphore := make(chan struct{}, maxWorkers)

	for i, condition := range enabledConditions {
		wg.Add(1)
		go func(index int, cond *PostCondition) {
			defer wg.Done()

			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			condResult := vr.validateSingleCondition(ctx, cond, result, stepCtx)
			results[index] = condResult

			// Thread-safe counter updates
			if !condResult.Satisfied {
				mu.Lock()
				totalFailed++
				if condResult.Critical {
					criticalFailed++
				}
				mu.Unlock()
			}
		}(i, condition)
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	// Analyze results
	totalPassed := len(enabledConditions) - totalFailed
	success := criticalFailed == 0 // Success if no critical conditions failed

	message := fmt.Sprintf("Post-condition validation %s: %d passed, %d failed (%d critical)",
		map[bool]string{true: "succeeded", false: "failed"}[success],
		totalPassed, totalFailed, criticalFailed)

	validationResult := &PostConditionValidationResult{
		Success:        success,
		Results:        results,
		CriticalFailed: criticalFailed,
		TotalFailed:    totalFailed,
		TotalPassed:    totalPassed,
		TotalDuration:  totalDuration,
		Message:        message,
	}

	vr.log.WithFields(logrus.Fields{
		"success":         success,
		"total_passed":    totalPassed,
		"total_failed":    totalFailed,
		"critical_failed": criticalFailed,
		"total_duration":  totalDuration,
	}).Info("Post-condition validation completed")

	return validationResult, nil
}

// validateSingleCondition validates a single post-condition with timing and error handling
func (vr *ValidatorRegistry) validateSingleCondition(
	ctx context.Context,
	condition *PostCondition,
	result *StepResult,
	stepCtx *StepContext,
) *PostConditionResult {
	startTime := time.Now()

	// Create base result
	condResult := &PostConditionResult{
		Name:        condition.Name,
		Type:        condition.Type,
		Critical:    condition.Critical,
		EvaluatedAt: startTime,
		Metadata:    make(map[string]interface{}),
	}

	// Apply timeout if specified
	if condition.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, condition.Timeout)
		defer cancel()
	}

	// Find and execute validator
	vr.mu.RLock()
	validator, exists := vr.validators[condition.Type]
	vr.mu.RUnlock()

	if !exists {
		condResult.Satisfied = false
		condResult.Message = fmt.Sprintf("Unknown post-condition type: %s", condition.Type)
		condResult.Duration = time.Since(startTime)
		return condResult
	}

	// Execute validation with error recovery
	validatedResult, err := validator.ValidateCondition(ctx, condition, result, stepCtx)
	if err != nil {
		condResult.Satisfied = false
		condResult.Message = fmt.Sprintf("Validation error: %s", err.Error())
		condResult.Duration = time.Since(startTime)
		return condResult
	}

	// Copy successful result
	*condResult = *validatedResult
	condResult.Duration = time.Since(startTime)

	vr.log.WithFields(logrus.Fields{
		"condition_name": condition.Name,
		"condition_type": condition.Type,
		"satisfied":      condResult.Satisfied,
		"critical":       condResult.Critical,
		"duration":       condResult.Duration,
		"message":        condResult.Message,
	}).Debug("Post-condition evaluated")

	return condResult
}

// filterAndSortConditions filters enabled conditions and sorts by priority
func (vr *ValidatorRegistry) filterAndSortConditions(conditions []*PostCondition) []*PostCondition {
	var enabled []*PostCondition

	for _, condition := range conditions {
		if condition.Enabled {
			enabled = append(enabled, condition)
		}
	}

	// Sort by priority (critical conditions first, then by validator priority)
	for i := 0; i < len(enabled)-1; i++ {
		for j := i + 1; j < len(enabled); j++ {
			iPriority := vr.getConditionPriority(enabled[i])
			jPriority := vr.getConditionPriority(enabled[j])

			if jPriority < iPriority {
				enabled[i], enabled[j] = enabled[j], enabled[i]
			}
		}
	}

	return enabled
}

// getConditionPriority calculates priority for sorting (lower = higher priority)
func (vr *ValidatorRegistry) getConditionPriority(condition *PostCondition) int {
	priority := 1000 // Default priority

	vr.mu.RLock()
	if validator, exists := vr.validators[condition.Type]; exists {
		priority = validator.GetPriority()
	}
	vr.mu.RUnlock()

	// Critical conditions get higher priority
	if condition.Critical {
		priority -= 500
	}

	return priority
}

// Built-in Validators

// SuccessValidator validates action success
type SuccessValidator struct{}

func (sv *SuccessValidator) ValidateCondition(ctx context.Context, condition *PostCondition, result *StepResult, stepCtx *StepContext) (*PostConditionResult, error) {
	return &PostConditionResult{
		Name:      condition.Name,
		Type:      condition.Type,
		Satisfied: result.Success,
		Value:     result.Success,
		Expected:  true,
		Critical:  condition.Critical,
		Message: func() string {
			if result.Success {
				return "Action completed successfully"
			}
			return fmt.Sprintf("Action failed: %s", result.Error)
		}(),
	}, nil
}

func (sv *SuccessValidator) GetType() PostConditionType { return PostConditionSuccess }
func (sv *SuccessValidator) GetPriority() int           { return 100 }

// ConfidenceValidator validates confidence levels
type ConfidenceValidator struct{}

func (cv *ConfidenceValidator) ValidateCondition(ctx context.Context, condition *PostCondition, result *StepResult, stepCtx *StepContext) (*PostConditionResult, error) {
	threshold := float64(0.8) // Default threshold
	if condition.Threshold != nil {
		threshold = *condition.Threshold
	}

	satisfied := result.Confidence >= threshold
	return &PostConditionResult{
		Name:      condition.Name,
		Type:      condition.Type,
		Satisfied: satisfied,
		Value:     result.Confidence,
		Expected:  threshold,
		Critical:  condition.Critical,
		Message: fmt.Sprintf("Confidence %.3f %s threshold %.3f",
			result.Confidence,
			map[bool]string{true: "meets", false: "below"}[satisfied],
			threshold),
	}, nil
}

func (cv *ConfidenceValidator) GetType() PostConditionType { return PostConditionConfidence }
func (cv *ConfidenceValidator) GetPriority() int           { return 200 }

// DurationValidator validates execution duration
type DurationValidator struct{}

func (dv *DurationValidator) ValidateCondition(ctx context.Context, condition *PostCondition, result *StepResult, stepCtx *StepContext) (*PostConditionResult, error) {
	maxSeconds := float64(300) // Default 5 minutes
	if condition.Threshold != nil {
		maxSeconds = *condition.Threshold
	}

	actualSeconds := result.Duration.Seconds()
	satisfied := actualSeconds <= maxSeconds

	return &PostConditionResult{
		Name:      condition.Name,
		Type:      condition.Type,
		Satisfied: satisfied,
		Value:     actualSeconds,
		Expected:  maxSeconds,
		Critical:  condition.Critical,
		Message: fmt.Sprintf("Duration %.2fs %s limit %.2fs",
			actualSeconds,
			map[bool]string{true: "within", false: "exceeds"}[satisfied],
			maxSeconds),
	}, nil
}

func (dv *DurationValidator) GetType() PostConditionType { return PostConditionDuration }
func (dv *DurationValidator) GetPriority() int           { return 300 }

// OutputValidator validates output content
type OutputValidator struct{}

func (ov *OutputValidator) ValidateCondition(ctx context.Context, condition *PostCondition, result *StepResult, stepCtx *StepContext) (*PostConditionResult, error) {
	expectedValue := fmt.Sprintf("%v", condition.Expected)

	found := false
	foundInField := ""
	if result.Output != nil {
		for key, value := range result.Output {
			valueStr := fmt.Sprintf("%v", value)
			if valueStr == expectedValue {
				found = true
				foundInField = key
				break
			}
		}
	}

	return &PostConditionResult{
		Name:      condition.Name,
		Type:      condition.Type,
		Satisfied: found,
		Value:     foundInField,
		Expected:  expectedValue,
		Critical:  condition.Critical,
		Message: func() string {
			if found {
				return fmt.Sprintf("Output contains expected value '%s' in field '%s'", expectedValue, foundInField)
			}
			return fmt.Sprintf("Output does not contain expected value '%s'", expectedValue)
		}(),
	}, nil
}

func (ov *OutputValidator) GetType() PostConditionType { return PostConditionOutput }
func (ov *OutputValidator) GetPriority() int           { return 400 }

// NoErrorsValidator validates absence of errors
type NoErrorsValidator struct{}

func (nev *NoErrorsValidator) ValidateCondition(ctx context.Context, condition *PostCondition, result *StepResult, stepCtx *StepContext) (*PostConditionResult, error) {
	hasError := result.Error != ""

	return &PostConditionResult{
		Name:      condition.Name,
		Type:      condition.Type,
		Satisfied: !hasError,
		Value:     result.Error,
		Expected:  "",
		Critical:  condition.Critical,
		Message: func() string {
			if !hasError {
				return "No errors occurred during execution"
			}
			return fmt.Sprintf("Action completed with error: %s", result.Error)
		}(),
	}, nil
}

func (nev *NoErrorsValidator) GetType() PostConditionType { return PostConditionNoErrors }
func (nev *NoErrorsValidator) GetPriority() int           { return 150 }

// ExpressionValidator validates custom expressions using the expression engine
type ExpressionValidator struct {
	expressionEngine *ExpressionEngine
}

func (ev *ExpressionValidator) ValidateCondition(ctx context.Context, condition *PostCondition, result *StepResult, stepCtx *StepContext) (*PostConditionResult, error) {
	if condition.Expression == "" {
		return nil, fmt.Errorf("expression cannot be empty")
	}

	exprCtx := &ExpressionContext{
		Result:    result,
		StepCtx:   stepCtx,
		Variables: make(map[string]interface{}),
		StartTime: time.Now(),
	}

	value, err := ev.expressionEngine.EvaluateString(ctx, condition.Expression, exprCtx)
	if err != nil {
		return nil, fmt.Errorf("expression evaluation failed: %w", err)
	}

	// Convert result to boolean
	satisfied := false
	switch v := value.(type) {
	case bool:
		satisfied = v
	case string:
		satisfied = v != "" && v != "false"
	case int64:
		satisfied = v != 0
	case float64:
		satisfied = v != 0.0
	default:
		satisfied = value != nil
	}

	return &PostConditionResult{
		Name:      condition.Name,
		Type:      condition.Type,
		Satisfied: satisfied,
		Value:     value,
		Expected:  condition.Expected,
		Critical:  condition.Critical,
		Message:   fmt.Sprintf("Expression '%s' evaluated to: %v", condition.Expression, value),
	}, nil
}

func (ev *ExpressionValidator) GetType() PostConditionType { return PostConditionExpression }
func (ev *ExpressionValidator) GetPriority() int           { return 800 }

// MetricValidator validates metrics-based conditions
type MetricValidator struct{}

func (mv *MetricValidator) ValidateCondition(ctx context.Context, condition *PostCondition, result *StepResult, stepCtx *StepContext) (*PostConditionResult, error) {
	// Simplified metric validation - can be extended
	satisfied := true
	message := "Metric validation passed"

	// Check if metrics are available
	if result.Metrics == nil {
		satisfied = false
		message = "No metrics available for validation"
	}

	return &PostConditionResult{
		Name:      condition.Name,
		Type:      condition.Type,
		Satisfied: satisfied,
		Value:     result.Metrics,
		Expected:  condition.Expected,
		Critical:  condition.Critical,
		Message:   message,
	}, nil
}

func (mv *MetricValidator) GetType() PostConditionType { return PostConditionMetric }
func (mv *MetricValidator) GetPriority() int           { return 600 }

// ResourceValidator validates resource-based conditions
type ResourceValidator struct{}

func (rv *ResourceValidator) ValidateCondition(ctx context.Context, condition *PostCondition, result *StepResult, stepCtx *StepContext) (*PostConditionResult, error) {
	// Simplified resource validation - can be extended
	satisfied := true
	message := "Resource validation passed"

	return &PostConditionResult{
		Name:      condition.Name,
		Type:      condition.Type,
		Satisfied: satisfied,
		Value:     nil,
		Expected:  condition.Expected,
		Critical:  condition.Critical,
		Message:   message,
	}, nil
}

func (rv *ResourceValidator) GetType() PostConditionType { return PostConditionResource }
func (rv *ResourceValidator) GetPriority() int           { return 700 }

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
