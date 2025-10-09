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

//go:build unit
// +build unit

package workflowengine

import (
	"testing"
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// BR-WF-GEN-001: Workflow Generation Validation Unit Tests
// Business Impact: Prevents invalid workflow generation that could cause system failures
// Stakeholder Value: Ensures reliable automated workflow creation for operations team
var _ = Describe("Workflow Generation Validation - Unit Tests", func() {
	var (
		builder      *engine.DefaultIntelligentWorkflowBuilder
		mockLLM      *mocks.MockLLMClient
		mockVectorDB *mocks.MockVectorDatabase
		ctx          context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockLogger := mocks.NewMockLogger()

		// Create mock dependencies following TDD guidelines
		mockLLM = mocks.NewMockLLMClient()
		mockVectorDB = mocks.NewMockVectorDatabase()

		// Create builder with real business logic using new config pattern
		config := &engine.IntelligentWorkflowBuilderConfig{
			LLMClient:       mockLLM,
			VectorDB:        mockVectorDB,
			AnalyticsEngine: nil, // analyticsEngine
			PatternStore:    nil, // patternStore
			ExecutionRepo:   nil, // executionRepo
			Logger:          mockLogger.Logger,
		}

		var err error
		builder, err = engine.NewIntelligentWorkflowBuilder(config)
		Expect(err).ToNot(HaveOccurred(), "Workflow builder creation should not fail")
		Expect(builder).ToNot(BeNil(), "Workflow builder should be created successfully")
	})

	// BR-WF-GEN-001: Core workflow generation function validation
	Describe("addValidationSteps Function Validation", func() {
		It("should add validation steps with valid action types only", func() {
			// Business Requirement: BR-WF-GEN-001-VALIDATION
			// Generated validation steps must use only approved action types to prevent validation failures

			// Arrange: Create empty template
			template := &engine.ExecutableTemplate{
				BaseVersionedEntity: types.BaseVersionedEntity{
					BaseEntity: types.BaseEntity{
						ID:   "test-template-validation",
						Name: "Test Template for Validation Steps",
					},
					Version: "1.0.0",
				},
				Steps: []*engine.ExecutableWorkflowStep{},
			}

			originalStepCount := len(template.Steps)

			// Act: Add validation steps using the actual business function
			builder.AddValidationSteps(template) // Public method that calls addValidationSteps internally

			// Assert: Validation steps should be added
			Expect(len(template.Steps)).To(BeNumerically(">", originalStepCount),
				"BR-WF-GEN-001-VALIDATION: addValidationSteps should add validation steps to template")

			// CRITICAL: Validate all generated steps have valid action types
			validActionTypes := builder.GetAvailableActionTypes()
			for i, step := range template.Steps {
				if step.Action != nil {
					Expect(validActionTypes).To(ContainElement(step.Action.Type),
						"BR-WF-GEN-001-VALIDATION: Generated step[%d] action type '%s' must be in valid action types list",
						i, step.Action.Type)

					// Verify the step has proper business structure
					Expect(step.ID).ToNot(BeEmpty(), "Generated validation step must have ID")
					Expect(step.Name).ToNot(BeEmpty(), "Generated validation step must have name")
					Expect(step.Timeout).To(BeNumerically(">", 0), "Generated validation step must have timeout")
				}
			}
		})

		It("should add validation steps with required retry policy for safety compliance", func() {
			// Business Requirement: BR-WF-GEN-001-SAFETY
			// Validation steps with certain action types must have retry policies for safety compliance

			template := &engine.ExecutableTemplate{
				BaseVersionedEntity: types.BaseVersionedEntity{
					BaseEntity: types.BaseEntity{ID: "test-template-retry", Name: "Test Template for Retry Policy"},
					Version:    "1.0.0",
				},
				Steps: []*engine.ExecutableWorkflowStep{},
			}

			// Act: Add validation steps
			builder.AddValidationSteps(template)

			// Assert: Steps that require retry policy should have one
			for _, step := range template.Steps {
				if step.Action != nil && builder.ShouldHaveRetryPolicy(step) {
					Expect(step.RetryPolicy).ToNot(BeNil(),
						"BR-WF-GEN-001-SAFETY: Step with action type '%s' must have retry policy for safety compliance",
						step.Action.Type)

					// Validate retry policy structure
					Expect(step.RetryPolicy.MaxRetries).To(BeNumerically(">", 0), "Retry policy must have positive max retries")
					Expect(step.RetryPolicy.Delay).To(BeNumerically(">", 0), "Retry policy must have positive delay")
				}
			}
		})
	})

	// BR-WF-GEN-002: AI Fallback workflow generation validation
	Describe("generateFallbackWorkflowResponse Function Validation", func() {
		It("should generate fallback workflows with valid action types only", func() {
			// Business Requirement: BR-WF-GEN-002-FALLBACK
			// AI fallback workflows must use only valid action types to prevent validation failures

			testResponses := []string{
				"Invalid JSON response from AI",
				"Node-level network issues detected, draining node for maintenance",
				"Database connection timeout requires immediate attention",
				"{invalid json structure}",
			}

			validActionTypes := builder.GetAvailableActionTypes()

			for _, response := range testResponses {
				// Act: Generate fallback workflow
				fallbackWorkflow := builder.GenerateFallbackWorkflowResponse(response)

				// Assert: All steps must have valid action types
				Expect(fallbackWorkflow).ToNot(BeNil(), "Fallback workflow should be generated")
				Expect(fallbackWorkflow.Steps).ToNot(BeEmpty(), "Fallback workflow should have steps")

				for i, step := range fallbackWorkflow.Steps {
					if step.Action != nil {
						Expect(validActionTypes).To(ContainElement(step.Action.Type),
							"BR-WF-GEN-002-FALLBACK: Fallback step[%d] action type '%s' must be valid for response: %s",
							i, step.Action.Type, response[:min(len(response), 50)])

						// Verify fallback step business structure
						Expect(step.Name).ToNot(BeEmpty(), "Fallback step must have name")
						Expect(step.Type).To(Equal("action"), "Fallback step must be action type")
					}
				}
			}
		})

		It("should generate safe fallback workflows without problematic dependencies", func() {
			// Business Requirement: BR-WF-GEN-002-SAFETY
			// Fallback workflows must not have circular or invalid dependencies

			response := "Complex AI parsing failure scenario"

			// Act: Generate fallback workflow
			fallbackWorkflow := builder.GenerateFallbackWorkflowResponse(response)

			// Assert: Fallback workflow should be dependency-safe
			stepIDs := make(map[string]bool)
			for _, step := range fallbackWorkflow.Steps {
				stepIDs[step.Name] = true
			}

			for _, step := range fallbackWorkflow.Steps {
				// Dependencies should either be empty or refer to valid steps
				for _, dep := range step.Dependencies {
					if dep != "" {
						Expect(stepIDs).To(HaveKey(dep),
							"BR-WF-GEN-002-SAFETY: Step dependency '%s' must refer to valid step in fallback workflow", dep)
					}
				}
			}
		})
	})

	// BR-WF-GEN-003: Validate workflow generation creates valid templates
	Describe("Template Generation Validation", func() {
		It("should generate templates with valid structure and action types", func() {
			// Business Requirement: BR-WF-GEN-003-STRUCTURE
			// Generated templates should have valid structure and use only valid action types

			// Create a test objective for workflow generation
			objective := &engine.WorkflowObjective{
				ID:          "test-objective-001",
				Type:        "remediation",
				Description: "Fix pod that keeps restarting in production",
				Priority:    1, // High priority (integer)
				Constraints: map[string]interface{}{
					"severity":    "critical",
					"component":   "api-server",
					"risk_level":  "high",
					"environment": "production",
					"namespace":   "default",
				},
				Status:    "active",
				Progress:  0.0,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			// Act: Generate workflow template
			template, err := builder.GenerateWorkflow(ctx, objective)

			// Assert: Template generation succeeds
			Expect(err).ToNot(HaveOccurred(), "Template generation should succeed")
			Expect(template).ToNot(BeNil(), "Generated template should not be nil")
			Expect(template.Steps).ToNot(BeEmpty(), "Generated template should have steps")

			// CRITICAL: All generated steps must have valid action types
			validActionTypes := builder.GetAvailableActionTypes()
			for i, step := range template.Steps {
				if step.Action != nil {
					Expect(validActionTypes).To(ContainElement(step.Action.Type),
						"BR-WF-GEN-003-STRUCTURE: Step[%d] action type '%s' must be valid",
						i, step.Action.Type)
				}
			}
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUworkflowUgenerationUvalidation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UworkflowUgenerationUvalidation Suite")
}
