//go:build unit
// +build unit

package workflowengine

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

// BR-VALID-PRE-001, BR-VALID-PRE-002, BR-VALID-PRE-003: Pre-Condition Validation Unit Testing - Pyramid Testing (70% Unit Coverage)
// Business Impact: Validates pre-condition validation capabilities for reliable workflow execution
// Stakeholder Value: Operations teams can trust workflow validation and conditional execution
var _ = Describe("BR-VALID-PRE-001, BR-VALID-PRE-002, BR-VALID-PRE-003: Pre-Condition Validation Unit Testing", func() {
	var (
		// Mock ONLY external dependencies per pyramid principles
		mockLogger *logrus.Logger

		// Use REAL business logic components
		validationRegistry *engine.PreConditionValidationRegistry

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Mock external dependencies only
		mockLogger = logrus.New()
		mockLogger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

		// Create REAL business logic component for pre-condition validation testing
		validationRegistry = engine.NewPreConditionValidationRegistry(mockLogger)
	})

	AfterEach(func() {
		cancel()
	})

	// BR-VALID-PRE-001: Environment-based Workflow Step Filtering
	Context("BR-VALID-PRE-001: Environment-based Workflow Step Filtering", func() {
		It("should filter workflow steps based on environment conditions", func() {
			// Business Scenario: System filters workflow steps based on deployment environment
			// Business Impact: Ensures environment-appropriate workflow execution

			// Create environment-based validation criteria
			productionCriteria := createEnvironmentValidationCriteria("production", false)
			productionStepContext := createEnvironmentStepContext("production", 9)

			// Test REAL business logic for production environment validation
			productionResult := validationRegistry.ValidatePreConditions(ctx, productionCriteria, productionStepContext)

			// Validate REAL business environment filtering outcomes
			Expect(productionResult).ToNot(BeNil(),
				"BR-VALID-PRE-001: Environment validation must produce results")
			Expect(productionResult.Success).To(BeTrue(),
				"BR-VALID-PRE-001: Production environment validation must succeed for production context")
			Expect(productionResult.TotalRules).To(Equal(len(productionCriteria.Rules)),
				"BR-VALID-PRE-001: Must validate all environment rules")
			Expect(productionResult.PassedRules).To(BeNumerically(">", 0),
				"BR-VALID-PRE-001: Production context must pass production-only rules")

			// Test non-production environment filtering
			developmentCriteria := createEnvironmentValidationCriteria("development", false)
			developmentStepContext := createEnvironmentStepContext("development", 5)

			developmentResult := validationRegistry.ValidatePreConditions(ctx, developmentCriteria, developmentStepContext)

			// Validate development environment filtering
			Expect(developmentResult.Success).To(BeTrue(),
				"BR-VALID-PRE-001: Development environment validation must succeed for development context")

			// Test cross-environment validation (production rules in development)
			crossEnvResult := validationRegistry.ValidatePreConditions(ctx, productionCriteria, developmentStepContext)

			// Production-only rules should fail in development environment
			Expect(crossEnvResult.Success).To(BeFalse(),
				"BR-VALID-PRE-001: Production-only rules must fail in development environment")
			Expect(crossEnvResult.FailedRules).To(BeNumerically(">", 0),
				"BR-VALID-PRE-001: Must track failed environment-based rules")

			// Business Value: Environment-based filtering ensures appropriate workflow execution
		})

		It("should handle environment-specific validation rules correctly", func() {
			// Business Scenario: System applies different validation rules per environment
			// Business Impact: Ensures environment-appropriate safety and compliance

			environments := []struct {
				name         string
				expectedPass bool
				ruleType     string
			}{
				{"production", true, "production_only"},
				{"staging", false, "production_only"},
				{"development", false, "production_only"},
				{"production", false, "non_production_only"},
				{"staging", true, "non_production_only"},
				{"development", true, "non_production_only"},
			}

			for _, env := range environments {
				By(fmt.Sprintf("Testing %s environment with %s rule", env.name, env.ruleType))

				// Create environment-specific validation
				criteria := createSpecificEnvironmentCriteria(env.ruleType)
				stepContext := createEnvironmentStepContext(env.name, 8)

				// Test REAL business logic for environment-specific validation
				result := validationRegistry.ValidatePreConditions(ctx, criteria, stepContext)

				// Validate environment-specific outcomes
				Expect(result).ToNot(BeNil(),
					"BR-VALID-PRE-001: Environment-specific validation must produce results")

				if env.expectedPass {
					Expect(result.Success).To(BeTrue(),
						"BR-VALID-PRE-001: %s rule must pass in %s environment", env.ruleType, env.name)
				} else {
					Expect(result.Success).To(BeFalse(),
						"BR-VALID-PRE-001: %s rule must fail in %s environment", env.ruleType, env.name)
				}
			}

			// Business Value: Environment-specific rules ensure proper deployment safety
		})
	})

	// BR-VALID-PRE-002: Priority-based Step Execution Control
	Context("BR-VALID-PRE-002: Priority-based Step Execution Control", func() {
		It("should control step execution based on priority levels", func() {
			// Business Scenario: System controls workflow step execution based on business priority
			// Business Impact: Ensures high-priority operations execute when appropriate

			// Create priority-based validation criteria
			highPriorityCriteria := createPriorityValidationCriteria("high_priority_only", false)

			// Test high-priority context (should pass)
			highPriorityContext := createPriorityStepContext("production", 9) // Priority 9 = high
			highPriorityResult := validationRegistry.ValidatePreConditions(ctx, highPriorityCriteria, highPriorityContext)

			// Validate REAL business high-priority validation outcomes
			Expect(highPriorityResult).ToNot(BeNil(),
				"BR-VALID-PRE-002: Priority validation must produce results")
			Expect(highPriorityResult.Success).To(BeTrue(),
				"BR-VALID-PRE-002: High-priority context must pass high-priority rules")
			Expect(highPriorityResult.PassedRules).To(Equal(1),
				"BR-VALID-PRE-002: High-priority rule must pass for high-priority context")

			// Test low-priority context (should fail)
			lowPriorityContext := createPriorityStepContext("production", 3) // Priority 3 = low
			lowPriorityResult := validationRegistry.ValidatePreConditions(ctx, highPriorityCriteria, lowPriorityContext)

			// Validate low-priority filtering
			Expect(lowPriorityResult.Success).To(BeFalse(),
				"BR-VALID-PRE-002: Low-priority context must fail high-priority rules")
			Expect(lowPriorityResult.FailedRules).To(Equal(1),
				"BR-VALID-PRE-002: High-priority rule must fail for low-priority context")

			// Business Value: Priority-based control ensures critical operations execute appropriately
		})

		It("should handle different priority thresholds correctly", func() {
			// Business Scenario: System applies different priority thresholds for business operations
			// Business Impact: Enables flexible priority-based workflow control

			priorityTests := []struct {
				contextPriority int
				expectedPass    bool
				description     string
			}{
				{10, true, "maximum priority"},
				{9, true, "high priority"},
				{8, true, "high priority threshold"},
				{7, false, "medium-high priority"},
				{5, false, "medium priority"},
				{3, false, "low priority"},
				{1, false, "minimum priority"},
			}

			criteria := createPriorityValidationCriteria("high_priority_only", false)

			for _, test := range priorityTests {
				By(fmt.Sprintf("Testing priority %d (%s)", test.contextPriority, test.description))

				// Create priority-specific context
				stepContext := createPriorityStepContext("production", test.contextPriority)

				// Test REAL business logic for priority-based validation
				result := validationRegistry.ValidatePreConditions(ctx, criteria, stepContext)

				// Validate priority-based outcomes
				if test.expectedPass {
					Expect(result.Success).To(BeTrue(),
						"BR-VALID-PRE-002: Priority %d must pass high-priority validation", test.contextPriority)
				} else {
					Expect(result.Success).To(BeFalse(),
						"BR-VALID-PRE-002: Priority %d must fail high-priority validation", test.contextPriority)
				}
			}

			// Business Value: Flexible priority thresholds enable fine-grained control
		})
	})

	// BR-VALID-PRE-003: Context-aware Validation Processing
	Context("BR-VALID-PRE-003: Context-aware Validation Processing", func() {
		It("should perform context-aware validation with comprehensive step context", func() {
			// Business Scenario: System performs validation based on comprehensive execution context
			// Business Impact: Enables intelligent validation based on runtime conditions

			// Create comprehensive validation criteria
			contextAwareCriteria := createContextAwareValidationCriteria()
			comprehensiveContext := createComprehensiveStepContext()

			// Test REAL business logic for context-aware validation
			result := validationRegistry.ValidatePreConditions(ctx, contextAwareCriteria, comprehensiveContext)

			// Validate REAL business context-aware validation outcomes
			Expect(result).ToNot(BeNil(),
				"BR-VALID-PRE-003: Context-aware validation must produce results")
			Expect(result.TotalRules).To(BeNumerically(">", 0),
				"BR-VALID-PRE-003: Must validate context-aware rules")
			Expect(result.TotalDuration).To(BeNumerically(">", 0),
				"BR-VALID-PRE-003: Must track validation duration for business monitoring")

			// Validate context utilization
			Expect(len(result.Results)).To(Equal(result.TotalRules),
				"BR-VALID-PRE-003: Must provide results for all validation rules")

			// Validate individual rule results
			for _, ruleResult := range result.Results {
				Expect(ruleResult.RuleName).ToNot(BeEmpty(),
					"BR-VALID-PRE-003: Each rule result must have identifiable name")
				Expect(ruleResult.Duration).To(BeNumerically(">=", 0),
					"BR-VALID-PRE-003: Each rule must track execution duration")
			}

			// Business Value: Context-aware validation enables intelligent workflow control
		})

		It("should handle validation timeouts gracefully", func() {
			// Business Scenario: System handles validation timeouts for business reliability
			// Business Impact: Ensures system reliability with slow validation operations

			// Create validation criteria with short timeout
			timeoutCriteria := createTimeoutValidationCriteria(100 * time.Millisecond)
			stepContext := createComprehensiveStepContext()

			// Test REAL business logic for timeout handling
			result := validationRegistry.ValidatePreConditions(ctx, timeoutCriteria, stepContext)

			// Validate REAL business timeout handling outcomes
			Expect(result).ToNot(BeNil(),
				"BR-VALID-PRE-003: Timeout validation must produce results")
			Expect(result.TotalDuration).To(BeNumerically("<=", 200*time.Millisecond),
				"BR-VALID-PRE-003: Validation must respect timeout for business reliability")

			// Business Value: Timeout handling ensures system responsiveness
		})

		It("should handle strict mode validation correctly", func() {
			// Business Scenario: System applies strict validation mode for critical operations
			// Business Impact: Ensures critical operations meet all validation requirements

			// Create strict mode validation criteria
			strictCriteria := createStrictModeValidationCriteria()
			stepContext := createMixedValidationStepContext() // Some rules will fail

			// Test REAL business logic for strict mode validation
			result := validationRegistry.ValidatePreConditions(ctx, strictCriteria, stepContext)

			// Validate REAL business strict mode outcomes
			Expect(result).ToNot(BeNil(),
				"BR-VALID-PRE-003: Strict mode validation must produce results")

			if result.FailedRules > 0 {
				Expect(result.CriticalFailed).To(BeTrue(),
					"BR-VALID-PRE-003: Strict mode must mark any failure as critical")
				Expect(result.Success).To(BeFalse(),
					"BR-VALID-PRE-003: Strict mode must fail if any rule fails")
			}

			// Business Value: Strict mode ensures critical operations meet all requirements
		})

		It("should handle empty validation criteria gracefully", func() {
			// Business Scenario: System handles workflows with no pre-conditions gracefully
			// Business Impact: Ensures system reliability for simple workflows

			// Test with nil criteria
			nilResult := validationRegistry.ValidatePreConditions(ctx, nil, createComprehensiveStepContext())

			// Validate graceful handling of nil criteria
			Expect(nilResult).ToNot(BeNil(),
				"BR-VALID-PRE-003: Must handle nil criteria gracefully")
			Expect(nilResult.Success).To(BeTrue(),
				"BR-VALID-PRE-003: Nil criteria should result in successful validation")
			Expect(nilResult.TotalRules).To(Equal(0),
				"BR-VALID-PRE-003: Nil criteria should have zero rules")
			Expect(nilResult.Message).To(ContainSubstring("No pre-conditions"),
				"BR-VALID-PRE-003: Must provide clear message for no pre-conditions")

			// Test with empty criteria
			emptyCriteria := &engine.ValidationCriteria{
				Rules:      []*engine.ValidationRule{},
				StrictMode: false,
				Timeout:    0,
			}
			emptyResult := validationRegistry.ValidatePreConditions(ctx, emptyCriteria, createComprehensiveStepContext())

			// Validate graceful handling of empty criteria
			Expect(emptyResult.Success).To(BeTrue(),
				"BR-VALID-PRE-003: Empty criteria should result in successful validation")
			Expect(emptyResult.TotalRules).To(Equal(0),
				"BR-VALID-PRE-003: Empty criteria should have zero rules")

			// Business Value: Graceful handling ensures system reliability
		})
	})

	// Integration Testing: Combined Validation Scenarios
	Context("Combined Validation Scenarios", func() {
		It("should handle complex validation scenarios with multiple rule types", func() {
			// Business Scenario: System handles complex workflows with multiple validation types
			// Business Impact: Enables sophisticated workflow control for complex business operations

			// Create complex validation criteria combining all rule types
			complexCriteria := createComplexValidationCriteria()
			complexContext := createComplexStepContext("production", 9)

			// Test REAL business logic for complex validation
			result := validationRegistry.ValidatePreConditions(ctx, complexCriteria, complexContext)

			// Validate REAL business complex validation outcomes
			Expect(result).ToNot(BeNil(),
				"Combined validation must produce results for complex scenarios")
			Expect(result.TotalRules).To(BeNumerically(">", 2),
				"Complex validation must include multiple rule types")

			// Validate that different rule types are processed
			ruleTypes := make(map[string]bool)
			for _, ruleResult := range result.Results {
				ruleTypes[ruleResult.RuleName] = true
			}
			Expect(len(ruleTypes)).To(BeNumerically(">=", 2),
				"Complex validation must process different rule types")

			// Business Value: Complex validation enables sophisticated business workflow control
		})
	})
})

// Helper functions for pre-condition validation testing
// These create realistic test data for REAL business logic validation

func createEnvironmentValidationCriteria(environment string, strictMode bool) *engine.ValidationCriteria {
	rules := []*engine.ValidationRule{
		{
			Name:       "production_environment_check",
			Expression: "production_only",
			ErrorMsg:   "Step should only run in production environment",
			Severity:   "high",
		},
		{
			Name:       "environment_safety_check",
			Expression: "always_true", // Always passes for basic safety
			ErrorMsg:   "Basic environment safety validation",
			Severity:   "medium",
		},
	}

	return &engine.ValidationCriteria{
		Rules:      rules,
		StrictMode: strictMode,
		Timeout:    5 * time.Second,
	}
}

func createEnvironmentStepContext(environment string, priority int) *engine.StepContext {
	return &engine.StepContext{
		ExecutionID: "env-test-execution-001",
		StepID:      "env-test-step-001",
		Variables: map[string]interface{}{
			"environment":            environment,
			"priority":               priority,
			"step_type":              "environment_validation",
			"deployment_environment": environment,
			"validation_context":     "environment_based",
		},
	}
}

func createSpecificEnvironmentCriteria(ruleType string) *engine.ValidationCriteria {
	return &engine.ValidationCriteria{
		Rules: []*engine.ValidationRule{
			{
				Name:       fmt.Sprintf("%s_rule", ruleType),
				Expression: ruleType,
				ErrorMsg:   fmt.Sprintf("Validates %s condition", ruleType),
				Severity:   "high",
			},
		},
		StrictMode: false,
		Timeout:    3 * time.Second,
	}
}

func createPriorityValidationCriteria(priorityRule string, strictMode bool) *engine.ValidationCriteria {
	return &engine.ValidationCriteria{
		Rules: []*engine.ValidationRule{
			{
				Name:       "priority_check",
				Expression: priorityRule,
				ErrorMsg:   "Validates step priority meets requirements",
				Severity:   "high",
			},
		},
		StrictMode: strictMode,
		Timeout:    3 * time.Second,
	}
}

func createPriorityStepContext(environment string, priority int) *engine.StepContext {
	return &engine.StepContext{
		ExecutionID: "priority-test-execution-001",
		StepID:      "priority-test-step-001",
		Variables: map[string]interface{}{
			"environment":        environment,
			"priority":           priority,
			"step_type":          "priority_validation",
			"business_priority":  priority,
			"validation_context": "priority_based",
		},
	}
}

func createContextAwareValidationCriteria() *engine.ValidationCriteria {
	return &engine.ValidationCriteria{
		Rules: []*engine.ValidationRule{
			{
				Name:       "context_environment_check",
				Expression: "production_only",
				ErrorMsg:   "Context-aware environment validation",
				Severity:   "high",
			},
			{
				Name:       "context_priority_check",
				Expression: "high_priority_only",
				ErrorMsg:   "Context-aware priority validation",
				Severity:   "medium",
			},
			{
				Name:       "context_safety_check",
				Expression: "always_true",
				ErrorMsg:   "Context-aware safety validation",
				Severity:   "low",
			},
		},
		StrictMode: false,
		Timeout:    10 * time.Second,
	}
}

func createComprehensiveStepContext() *engine.StepContext {
	return &engine.StepContext{
		ExecutionID: "comprehensive-test-execution-001",
		StepID:      "comprehensive-test-step-001",
		Variables: map[string]interface{}{
			"environment":          "production",
			"priority":             9,
			"step_type":            "comprehensive_validation",
			"business_unit":        "platform-engineering",
			"cost_center":          "infrastructure",
			"compliance_tier":      "high",
			"validation_context":   "comprehensive",
			"business_criticality": "high",
			"sla_tier":             "gold",
			"monitoring_enabled":   true,
		},
	}
}

func createTimeoutValidationCriteria(timeout time.Duration) *engine.ValidationCriteria {
	return &engine.ValidationCriteria{
		Rules: []*engine.ValidationRule{
			{
				Name:       "timeout_test_rule",
				Expression: "always_true",
				ErrorMsg:   "Rule for testing timeout behavior",
				Severity:   "medium",
			},
		},
		StrictMode: false,
		Timeout:    timeout,
	}
}

func createStrictModeValidationCriteria() *engine.ValidationCriteria {
	return &engine.ValidationCriteria{
		Rules: []*engine.ValidationRule{
			{
				Name:       "strict_rule_1",
				Expression: "always_true",
				ErrorMsg:   "Rule that always passes",
				Severity:   "high",
			},
			{
				Name:       "strict_rule_2",
				Expression: "always_false",
				ErrorMsg:   "Rule that always fails",
				Severity:   "high",
			},
		},
		StrictMode: true, // Any failure is critical
		Timeout:    5 * time.Second,
	}
}

func createMixedValidationStepContext() *engine.StepContext {
	return &engine.StepContext{
		ExecutionID: "mixed-validation-execution-001",
		StepID:      "mixed-validation-step-001",
		Variables: map[string]interface{}{
			"environment":        "production",
			"priority":           9,
			"step_type":          "mixed_validation",
			"validation_context": "mixed",
			"test_scenario":      "strict_mode",
		},
	}
}

func createComplexValidationCriteria() *engine.ValidationCriteria {
	return &engine.ValidationCriteria{
		Rules: []*engine.ValidationRule{
			{
				Name:       "complex_environment_check",
				Expression: "production_only",
				ErrorMsg:   "Complex environment validation",
				Severity:   "high",
			},
			{
				Name:       "complex_priority_check",
				Expression: "high_priority_only",
				ErrorMsg:   "Complex priority validation",
				Severity:   "high",
			},
			{
				Name:       "complex_safety_check",
				Expression: "always_true",
				ErrorMsg:   "Complex safety validation",
				Severity:   "medium",
			},
		},
		StrictMode: false,
		Timeout:    15 * time.Second,
	}
}

func createComplexStepContext(environment string, priority int) *engine.StepContext {
	return &engine.StepContext{
		ExecutionID: "complex-validation-execution-001",
		StepID:      "complex-validation-step-001",
		Variables: map[string]interface{}{
			"environment":          environment,
			"priority":             priority,
			"step_type":            "complex_validation",
			"business_unit":        "platform-engineering",
			"cost_center":          "infrastructure",
			"compliance_tier":      "high",
			"security_level":       "strict",
			"monitoring_level":     "comprehensive",
			"validation_context":   "complex",
			"business_criticality": "high",
			"sla_tier":             "platinum",
			"audit_required":       true,
			"backup_enabled":       true,
		},
	}
}
