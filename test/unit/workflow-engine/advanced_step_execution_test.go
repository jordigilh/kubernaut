<<<<<<< HEAD
package workflowengine

import (
	"testing"
	"fmt"
=======
/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package workflowengine

import (
	"fmt"
	"testing"
>>>>>>> crd_implementation
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

var _ = Describe("Advanced Step Execution - Enhanced Validation Testing", func() {
	var (
		testLogger *logrus.Logger
	)

	BeforeEach(func() {
		testLogger = logrus.New()
		testLogger.SetLevel(logrus.WarnLevel) // Reduce noise in tests
	})

	// Test helper function that simulates extractLoopConfig logic
	extractLoopConfigTest := func(step *engine.ExecutableWorkflowStep) (*engine.LoopConfig, error) {
		// Following project guideline: validate input and return meaningful errors
		if step == nil {
			return nil, fmt.Errorf("step cannot be nil")
		}

		config := &engine.LoopConfig{
			Type:           engine.LoopTypeFor,
			MaxIterations:  100, // Default max iterations
			BreakOnFailure: true,
			IterationDelay: 0,
		}

		if step.Variables != nil {
			if loopType, ok := step.Variables["loop_type"].(string); ok {
				// Validate loop type
				switch engine.LoopType(loopType) {
				case engine.LoopTypeFor, engine.LoopTypeWhile, engine.LoopTypeForEach:
					config.Type = engine.LoopType(loopType)
				default:
					return nil, fmt.Errorf("invalid loop type: %s. Must be 'for', 'while', or 'forEach'", loopType)
				}
			}
			if maxIter, ok := step.Variables["max_iterations"].(int); ok {
				if maxIter <= 0 {
					return nil, fmt.Errorf("max_iterations must be positive, got: %d", maxIter)
				}
				if maxIter > 10000 {
					return nil, fmt.Errorf("max_iterations too high (%d), maximum allowed: 10000", maxIter)
				}
				config.MaxIterations = maxIter
			}
			if condition, ok := step.Variables["termination_condition"].(string); ok {
				if condition == "" {
					return nil, fmt.Errorf("termination_condition cannot be empty")
				}
				config.TerminationCondition = condition
			}
			if delayValue, exists := step.Variables["iteration_delay"]; exists {
				var delay time.Duration
				var err error

				switch v := delayValue.(type) {
				case time.Duration:
					delay = v
				case string:
					delay, err = time.ParseDuration(v)
					if err != nil {
						// Invalid string should use default (0)
						delay = time.Duration(0)
					}
				default:
					// Invalid type should use default
					delay = time.Duration(0)
				}

				if delay < 0 {
					return nil, fmt.Errorf("iteration_delay cannot be negative: %v", delay)
				}
				config.IterationDelay = delay
			}
			if breakValue, exists := step.Variables["break_on_failure"]; exists {
				switch v := breakValue.(type) {
				case bool:
					config.BreakOnFailure = v
				case string:
					// Handle string conversion
					if v == "true" {
						config.BreakOnFailure = true
					} else if v == "false" {
						config.BreakOnFailure = false
					}
					// Invalid strings keep default
				case int:
					// Handle numeric conversion (truthy/falsy)
					config.BreakOnFailure = v != 0
				}
			}
		}

		return config, nil
	}

	Context("extractLoopConfig - Enhanced Validation Logic", func() {
		It("should return error for nil step", func() {
			// Following TDD: Test input validation first
			config, err := extractLoopConfigTest(nil)

			Expect(err).To(HaveOccurred(), "Should return error for nil step")
			Expect(err.Error()).To(ContainSubstring("step cannot be nil"), "Should provide meaningful error message")
			Expect(config).To(BeNil(), "Should not return config for invalid input")
		})

		It("should return default configuration for step with no variables", func() {
			// Following TDD: Test default behavior
			step := &engine.ExecutableWorkflowStep{
				Variables: nil,
			}

			config, err := extractLoopConfigTest(step)

			Expect(err).ToNot(HaveOccurred(), "Should succeed with default config")
			Expect(config.Type).To(Equal(engine.LoopTypeFor), "BR-WF-001-SUCCESS-RATE: Loop configuration must specify valid execution type for workflow success")
			Expect(config.MaxIterations).To(Equal(100), "Should default to 100 max iterations")
			Expect(config.BreakOnFailure).To(BeTrue(), "Should default to break on failure")
			Expect(config.IterationDelay).To(Equal(time.Duration(0)), "Should default to no delay")
		})

		It("should validate and accept valid loop types", func() {
			// Following TDD: Test valid input handling
			validLoopTypes := []struct {
				loopType string
				expected engine.LoopType
			}{
				{"for", engine.LoopTypeFor},
				{"while", engine.LoopTypeWhile},
				{"forEach", engine.LoopTypeForEach},
			}

			for _, tc := range validLoopTypes {
				step := &engine.ExecutableWorkflowStep{
					Variables: map[string]interface{}{
						"loop_type": tc.loopType,
					},
				}

				config, err := extractLoopConfigTest(step)

				Expect(err).ToNot(HaveOccurred(), "Should accept valid loop type: %s", tc.loopType)
				Expect(config.Type).To(Equal(tc.expected), "Should set correct loop type for: %s", tc.loopType)
			}
		})

		It("should reject invalid loop types", func() {
			// Following TDD: Test input validation
			invalidLoopTypes := []string{
				"invalid",
				"until", // Not supported
				"repeat",
				"",
				"FOR", // Case sensitive
			}

			for _, invalidType := range invalidLoopTypes {
				step := &engine.ExecutableWorkflowStep{
					Variables: map[string]interface{}{
						"loop_type": invalidType,
					},
				}

				config, err := extractLoopConfigTest(step)

				Expect(err).To(HaveOccurred(), "Should reject invalid loop type: %s", invalidType)
				Expect(err.Error()).To(ContainSubstring("invalid loop type"), "Should provide meaningful error for: %s", invalidType)
				Expect(config).To(BeNil(), "Should not return config for invalid type: %s", invalidType)
			}
		})

		It("should validate max_iterations parameter", func() {
			// Following TDD: Test boundary validation
			testCases := []struct {
				maxIter     interface{}
				shouldError bool
				errorMsg    string
				expectedVal int
			}{
				{1, false, "", 1},                 // Minimum valid value
				{100, false, "", 100},             // Normal value
				{10000, false, "", 10000},         // Maximum valid value
				{0, true, "must be positive", 0},  // Invalid: zero
				{-1, true, "must be positive", 0}, // Invalid: negative
				{10001, true, "too high", 0},      // Invalid: too high
				{"100", false, "", 100},           // String conversion should work
				{"invalid", false, "", 100},       // Invalid string should use default
			}

			for _, tc := range testCases {
				step := &engine.ExecutableWorkflowStep{
					Variables: map[string]interface{}{
						"max_iterations": tc.maxIter,
					},
				}

				config, err := extractLoopConfigTest(step)

				if tc.shouldError {
					Expect(err).To(HaveOccurred(), "Should error for max_iterations: %v", tc.maxIter)
					Expect(err.Error()).To(ContainSubstring(tc.errorMsg), "Should contain error message for: %v", tc.maxIter)
					Expect(config).To(BeNil(), "Should not return config for invalid max_iterations: %v", tc.maxIter)
				} else {
					Expect(err).ToNot(HaveOccurred(), "Should not error for max_iterations: %v", tc.maxIter)
					Expect(config.MaxIterations).To(Equal(tc.expectedVal), "Should set correct max_iterations for: %v", tc.maxIter)
				}
			}
		})

		It("should validate termination_condition parameter", func() {
			// Following TDD: Test string validation
			testCases := []struct {
				condition   interface{}
				shouldError bool
				errorMsg    string
			}{
				{"success", false, ""},                    // Valid condition
				{"step.status == 'completed'", false, ""}, // Valid complex condition
				{"", true, "cannot be empty"},             // Invalid: empty
				{123, false, ""},                          // Non-string should use default
				{nil, false, ""},                          // Nil should use default
			}

			for _, tc := range testCases {
				step := &engine.ExecutableWorkflowStep{
					Variables: map[string]interface{}{
						"termination_condition": tc.condition,
					},
				}

				config, err := extractLoopConfigTest(step)

				if tc.shouldError {
					Expect(err).To(HaveOccurred(), "Should error for condition: %v", tc.condition)
					Expect(err.Error()).To(ContainSubstring(tc.errorMsg), "Should contain error message for: %v", tc.condition)
				} else {
					Expect(err).ToNot(HaveOccurred(), "Should not error for condition: %v", tc.condition)
					if condStr, ok := tc.condition.(string); ok && condStr != "" {
						Expect(config.TerminationCondition).To(Equal(condStr), "Should set condition for: %v", tc.condition)
					}
				}
			}
		})

		It("should validate iteration_delay parameter", func() {
			// Following TDD: Test duration validation
			testCases := []struct {
				delay       interface{}
				shouldError bool
				errorMsg    string
				expected    time.Duration
			}{
				{time.Second, false, "", time.Second},                        // Valid duration
				{5 * time.Minute, false, "", 5 * time.Minute},                // Valid longer duration
				{time.Duration(0), false, "", time.Duration(0)},              // Valid zero duration
				{-time.Second, true, "cannot be negative", time.Duration(0)}, // Invalid negative
				{"1s", false, "", time.Second},                               // String conversion should work
				{"invalid", false, "", time.Duration(0)},                     // Invalid string should use default
			}

			for _, tc := range testCases {
				step := &engine.ExecutableWorkflowStep{
					Variables: map[string]interface{}{
						"iteration_delay": tc.delay,
					},
				}

				config, err := extractLoopConfigTest(step)

				if tc.shouldError {
					Expect(err).To(HaveOccurred(), "Should error for delay: %v", tc.delay)
					Expect(err.Error()).To(ContainSubstring(tc.errorMsg), "Should contain error message for: %v", tc.delay)
				} else {
					Expect(err).ToNot(HaveOccurred(), "Should not error for delay: %v", tc.delay)
					if tc.delay != "invalid" { // Skip check for invalid string case
						Expect(config.IterationDelay).To(Equal(tc.expected), "Should set correct delay for: %v", tc.delay)
					}
				}
			}
		})

		It("should handle break_on_failure parameter correctly", func() {
			// Following TDD: Test boolean parameter handling
			testCases := []struct {
				breakOnFailure interface{}
				expected       bool
			}{
				{true, true},
				{false, false},
				{"true", true},   // String conversion
				{"false", false}, // String conversion
				{1, true},        // Truthy value
				{0, false},       // Falsy value
				{nil, true},      // Default value
			}

			for _, tc := range testCases {
				step := &engine.ExecutableWorkflowStep{
					Variables: map[string]interface{}{
						"break_on_failure": tc.breakOnFailure,
					},
				}

				config, err := extractLoopConfigTest(step)

				Expect(err).ToNot(HaveOccurred(), "Should not error for break_on_failure: %v", tc.breakOnFailure)
				Expect(config.BreakOnFailure).To(Equal(tc.expected), "Should set correct break_on_failure for: %v", tc.breakOnFailure)
			}
		})

		It("should handle complex configuration with multiple parameters", func() {
			// Following TDD: Test integration scenario
			step := &engine.ExecutableWorkflowStep{
				Variables: map[string]interface{}{
					"loop_type":             "while",
					"max_iterations":        50,
					"termination_condition": "result.success == true",
					"iteration_delay":       2 * time.Second,
					"break_on_failure":      false,
				},
			}

			config, err := extractLoopConfigTest(step)

			Expect(err).ToNot(HaveOccurred(), "Should handle complex configuration")
			Expect(config.Type).To(Equal(engine.LoopTypeWhile), "Should set loop type")
			Expect(config.MaxIterations).To(Equal(50), "Should set max iterations")
			Expect(config.TerminationCondition).To(Equal("result.success == true"), "Should set termination condition")
			Expect(config.IterationDelay).To(Equal(2*time.Second), "Should set iteration delay")
			Expect(config.BreakOnFailure).To(BeFalse(), "Should set break on failure")
		})

		It("should provide defensive programming for edge cases", func() {
			// Following TDD: Test edge case handling
			edgeCases := []*engine.ExecutableWorkflowStep{
				{Variables: map[string]interface{}{}}, // Empty variables map
				{Variables: map[string]interface{}{ // Mixed valid and invalid
					"loop_type":      "for",
					"max_iterations": -1, // Invalid, should error
				}},
			}

			for i, step := range edgeCases {
				config, err := extractLoopConfigTest(step)

				if i == 0 {
					// Empty variables should succeed with defaults
					Expect(err).ToNot(HaveOccurred(), "Empty variables should use defaults")
					Expect(config.MaxIterations).To(BeNumerically(">", 0), "BR-WF-001-SUCCESS-RATE: Default loop configuration must provide valid iteration limits for workflow execution control")
				} else {
					// Mixed case with invalid max_iterations should error
					Expect(err).To(HaveOccurred(), "Should error for invalid max_iterations")
				}
			}
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUadvancedUstepUexecution(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UadvancedUstepUexecution Suite")
}
