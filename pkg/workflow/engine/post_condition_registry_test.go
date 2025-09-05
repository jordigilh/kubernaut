package engine

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestValidatorRegistry_EfficientValidation(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel) // Reduce log noise in tests

	registry := NewValidatorRegistry(log)

	tests := []struct {
		name            string
		conditions      []*PostCondition
		result          *StepResult
		expectedSuccess bool
		expectedMessage string
	}{
		{
			name:       "No conditions",
			conditions: []*PostCondition{},
			result: &StepResult{
				Success: true,
			},
			expectedSuccess: true,
			expectedMessage: "No post-conditions to validate",
		},
		{
			name: "Single success condition - pass",
			conditions: []*PostCondition{
				{
					Type:     PostConditionSuccess,
					Name:     "action_success",
					Critical: true,
					Enabled:  true,
				},
			},
			result: &StepResult{
				Success:    true,
				Duration:   30 * time.Second,
				Confidence: 0.9,
			},
			expectedSuccess: true,
			expectedMessage: "Post-condition validation succeeded: 1 passed, 0 failed (0 critical)",
		},
		{
			name: "Single success condition - fail",
			conditions: []*PostCondition{
				{
					Type:     PostConditionSuccess,
					Name:     "action_success",
					Critical: true,
					Enabled:  true,
				},
			},
			result: &StepResult{
				Success: false,
				Error:   "Something went wrong",
			},
			expectedSuccess: false,
			expectedMessage: "Post-condition validation failed: 0 passed, 1 failed (1 critical)",
		},
		{
			name: "Multiple conditions - all critical pass",
			conditions: []*PostCondition{
				{
					Type:     PostConditionSuccess,
					Name:     "action_success",
					Critical: true,
					Enabled:  true,
				},
				{
					Type:      PostConditionConfidence,
					Name:      "confidence_check",
					Threshold: floatPtr(0.8),
					Critical:  true,
					Enabled:   true,
				},
				{
					Type:      PostConditionDuration,
					Name:      "duration_check",
					Threshold: floatPtr(60.0),
					Critical:  false,
					Enabled:   true,
				},
			},
			result: &StepResult{
				Success:    true,
				Duration:   30 * time.Second,
				Confidence: 0.9,
			},
			expectedSuccess: true,
			expectedMessage: "Post-condition validation succeeded: 3 passed, 0 failed (0 critical)",
		},
		{
			name: "Mixed conditions - non-critical failure",
			conditions: []*PostCondition{
				{
					Type:     PostConditionSuccess,
					Name:     "action_success",
					Critical: true,
					Enabled:  true,
				},
				{
					Type:      PostConditionConfidence,
					Name:      "confidence_check",
					Threshold: floatPtr(0.8),
					Critical:  true,
					Enabled:   true,
				},
				{
					Type:      PostConditionDuration,
					Name:      "duration_check",
					Threshold: floatPtr(20.0), // Will fail - duration is 30s
					Critical:  false,          // Non-critical
					Enabled:   true,
				},
			},
			result: &StepResult{
				Success:    true,
				Duration:   30 * time.Second,
				Confidence: 0.9,
			},
			expectedSuccess: true, // Should still succeed because critical conditions passed
			expectedMessage: "Post-condition validation succeeded: 2 passed, 1 failed (0 critical)",
		},
		{
			name: "Critical failure",
			conditions: []*PostCondition{
				{
					Type:     PostConditionSuccess,
					Name:     "action_success",
					Critical: true,
					Enabled:  true,
				},
				{
					Type:      PostConditionConfidence,
					Name:      "confidence_check",
					Threshold: floatPtr(0.95), // Will fail - confidence is 0.9
					Critical:  true,           // Critical
					Enabled:   true,
				},
			},
			result: &StepResult{
				Success:    true,
				Duration:   30 * time.Second,
				Confidence: 0.9,
			},
			expectedSuccess: false,
			expectedMessage: "Post-condition validation failed: 1 passed, 1 failed (1 critical)",
		},
		{
			name: "Disabled conditions ignored",
			conditions: []*PostCondition{
				{
					Type:     PostConditionSuccess,
					Name:     "action_success",
					Critical: true,
					Enabled:  true,
				},
				{
					Type:      PostConditionConfidence,
					Name:      "confidence_check",
					Threshold: floatPtr(0.95), // Would fail
					Critical:  true,
					Enabled:   false, // Disabled - should be ignored
				},
			},
			result: &StepResult{
				Success:    true,
				Duration:   30 * time.Second,
				Confidence: 0.9,
			},
			expectedSuccess: true,
			expectedMessage: "Post-condition validation succeeded: 1 passed, 0 failed (0 critical)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			stepContext := &StepContext{}

			result, err := registry.ValidatePostConditions(ctx, tt.conditions, tt.result, stepContext)

			if err != nil {
				t.Errorf("ValidatePostConditions() error = %v, wantErr false", err)
				return
			}

			if result.Success != tt.expectedSuccess {
				t.Errorf("ValidatePostConditions() success = %v, want %v", result.Success, tt.expectedSuccess)
			}

			if result.Message != tt.expectedMessage {
				t.Errorf("ValidatePostConditions() message = %v, want %v", result.Message, tt.expectedMessage)
			}

			// Verify duration is tracked
			if result.TotalDuration == 0 && len(tt.conditions) > 0 {
				t.Error("ValidatePostConditions() should track total duration")
			}

			// Verify results array matches conditions count
			enabledCount := 0
			for _, cond := range tt.conditions {
				if cond.Enabled {
					enabledCount++
				}
			}
			if len(result.Results) != enabledCount {
				t.Errorf("ValidatePostConditions() results count = %d, want %d", len(result.Results), enabledCount)
			}
		})
	}
}

func TestValidatorRegistry_ExpressionValidation(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	registry := NewValidatorRegistry(log)
	ctx := context.Background()
	stepContext := &StepContext{}

	tests := []struct {
		name              string
		expression        string
		result            *StepResult
		expectedSatisfied bool
	}{
		{
			name:       "Simple success check",
			expression: "success == true",
			result: &StepResult{
				Success: true,
			},
			expectedSatisfied: true,
		},
		{
			name:       "Confidence threshold",
			expression: "confidence >= 0.8",
			result: &StepResult{
				Success:    true,
				Confidence: 0.9,
			},
			expectedSatisfied: true,
		},
		{
			name:       "Duration check",
			expression: "duration < 60",
			result: &StepResult{
				Success:  true,
				Duration: 30 * time.Second,
			},
			expectedSatisfied: true,
		},
		{
			name:       "Error check",
			expression: "error == \"\"",
			result: &StepResult{
				Success: true,
				Error:   "",
			},
			expectedSatisfied: true,
		},
		{
			name:       "Function call - has_error",
			expression: "has_error()",
			result: &StepResult{
				Success: false,
				Error:   "Something failed",
			},
			expectedSatisfied: true, // has_error() returns true when there is an error
		},
		{
			name:       "Function call - duration_seconds",
			expression: "duration_seconds() < 60",
			result: &StepResult{
				Success:  true,
				Duration: 30 * time.Second,
			},
			expectedSatisfied: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition := &PostCondition{
				Type:       PostConditionExpression,
				Name:       "expression_test",
				Expression: tt.expression,
				Critical:   true,
				Enabled:    true,
			}

			result, err := registry.ValidatePostConditions(ctx, []*PostCondition{condition}, tt.result, stepContext)

			if err != nil {
				t.Errorf("ValidatePostConditions() error = %v, wantErr false", err)
				return
			}

			if len(result.Results) != 1 {
				t.Errorf("Expected 1 result, got %d", len(result.Results))
				return
			}

			condResult := result.Results[0]
			if condResult.Satisfied != tt.expectedSatisfied {
				t.Errorf("Expression '%s' satisfied = %v, want %v. Message: %s",
					tt.expression, condResult.Satisfied, tt.expectedSatisfied, condResult.Message)
			}
		})
	}
}

func TestValidatorRegistry_ParallelPerformance(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	registry := NewValidatorRegistry(log)
	ctx := context.Background()
	stepContext := &StepContext{}

	// Create many conditions to test parallel execution
	conditions := make([]*PostCondition, 20)
	for i := 0; i < 20; i++ {
		conditions[i] = &PostCondition{
			Type:     PostConditionSuccess,
			Name:     "parallel_test",
			Critical: false,
			Enabled:  true,
		}
	}

	result := &StepResult{
		Success:    true,
		Duration:   30 * time.Second,
		Confidence: 0.9,
	}

	start := time.Now()
	validationResult, err := registry.ValidatePostConditions(ctx, conditions, result, stepContext)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("ValidatePostConditions() error = %v", err)
		return
	}

	if !validationResult.Success {
		t.Errorf("Expected success, got failure: %s", validationResult.Message)
	}

	if len(validationResult.Results) != 20 {
		t.Errorf("Expected 20 results, got %d", len(validationResult.Results))
	}

	// Parallel execution should be faster than sequential
	// For 20 simple conditions, should complete very quickly
	if elapsed > 100*time.Millisecond {
		t.Errorf("Parallel validation took too long: %v", elapsed)
	}

	t.Logf("Parallel validation of 20 conditions took: %v", elapsed)
}

func TestValidatorRegistry_CustomValidator(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	registry := NewValidatorRegistry(log)

	// Register custom validator
	customValidator := &TestCustomValidator{}
	registry.RegisterValidator(customValidator)

	ctx := context.Background()
	stepContext := &StepContext{}

	condition := &PostCondition{
		Type:     PostConditionType("custom_test"),
		Name:     "custom_validation",
		Expected: "test_value",
		Critical: true,
		Enabled:  true,
	}

	result := &StepResult{
		Success: true,
		Output: map[string]interface{}{
			"custom_field": "test_value",
		},
	}

	validationResult, err := registry.ValidatePostConditions(ctx, []*PostCondition{condition}, result, stepContext)

	if err != nil {
		t.Errorf("ValidatePostConditions() error = %v", err)
		return
	}

	if !validationResult.Success {
		t.Errorf("Expected success, got failure: %s", validationResult.Message)
	}

	if len(validationResult.Results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(validationResult.Results))
	}

	condResult := validationResult.Results[0]
	if !condResult.Satisfied {
		t.Errorf("Custom validation should have passed: %s", condResult.Message)
	}
}

// TestCustomValidator for testing custom validator registration
type TestCustomValidator struct{}

func (tcv *TestCustomValidator) ValidateCondition(ctx context.Context, condition *PostCondition, result *StepResult, stepCtx *StepContext) (*PostConditionResult, error) {
	expected := condition.Expected
	var found interface{}
	satisfied := false

	if result.Output != nil {
		if val, exists := result.Output["custom_field"]; exists {
			found = val
			satisfied = val == expected
		}
	}

	return &PostConditionResult{
		Name:      condition.Name,
		Type:      condition.Type,
		Satisfied: satisfied,
		Value:     found,
		Expected:  expected,
		Critical:  condition.Critical,
		Message:   "Custom validation completed",
	}, nil
}

func (tcv *TestCustomValidator) GetType() PostConditionType {
	return PostConditionType("custom_test")
}

func (tcv *TestCustomValidator) GetPriority() int {
	return 500
}

// Helper function
func floatPtr(f float64) *float64 {
	return &f
}
