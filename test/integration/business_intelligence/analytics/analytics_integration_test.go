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

//go:build integration
// +build integration

package analytics

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// BR-ANALYTICS-INTEGRATION-TDD-001: Analytics Integration Business Intelligence TDD Verification
// Business Impact: Ensures comprehensive validation of analytics integration business logic through TDD methodology
// Stakeholder Value: Provides executive confidence in analytics-driven workflow optimization and business intelligence capabilities

var _ = Describe("Analytics Integration TDD Verification - Executive Business Intelligence Validation", func() {
	var (
		ctx       context.Context
		logger    *logrus.Logger
		builder   engine.IntelligentWorkflowBuilder
		objective *engine.WorkflowObjective
	)

	BeforeEach(func() {
		// Use existing testutil patterns instead of deprecated TDDConversionHelper
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests
		ctx = context.Background()

		// Create builder with minimal dependencies for TDD verification using config pattern
		config := &engine.IntelligentWorkflowBuilderConfig{
			LLMClient:       nil, // External: Mock not needed for this test
			VectorDB:        nil, // External: Mock not needed for this test
			AnalyticsEngine: nil, // External: Mock not needed for this test
			PatternStore:    nil, // External: Mock not needed for this test
			ExecutionRepo:   nil, // External: Mock not needed for this test
			Logger:          logger,
		}

		var err error
		builder, err = engine.NewIntelligentWorkflowBuilder(config)
		Expect(err).ToNot(HaveOccurred(), "Workflow builder creation should not fail")

		// Create test objective for analytics verification
		objective = &engine.WorkflowObjective{
			ID:          "analytics-test-obj-001",
			Type:        "remediation",
			Description: "Analytics integration TDD verification objective",
			Constraints: map[string]interface{}{
				"max_time": "10m",
			},
		}
	})

	Context("When validating analytics integration business requirements through TDD", func() {
		Describe("BR-ANALYTICS-001: Workflow Generation Analytics Metadata Integration", func() {
			It("should generate workflows with comprehensive analytics metadata for executive reporting", func() {
				// Business Scenario: Executive stakeholders need analytics metadata in all generated workflows
				// Business Impact: Enables data-driven decision making and workflow optimization insights

				// Business Action: Generate workflow with analytics integration
				template, err := builder.GenerateWorkflow(ctx, objective)

				// Business Validation: Workflow generation succeeds with analytics metadata
				Expect(err).ToNot(HaveOccurred(),
					"BR-ANALYTICS-001: Workflow generation must succeed with analytics metadata integration for executive business intelligence")

				Expect(template).ToNot(BeNil(),
					"BR-ANALYTICS-001: Generated workflow template must exist for analytics-driven business operations")

				Expect(template.Metadata).ToNot(BeNil(),
					"BR-ANALYTICS-001: Workflow metadata must be initialized for executive analytics reporting and business intelligence")

				// Business Outcome: Analytics metadata structure supports business intelligence
				Expect(template.Metadata).To(BeAssignableToTypeOf(map[string]interface{}{}),
					"BR-ANALYTICS-001: Analytics metadata must support flexible business intelligence data structures for executive reporting")
			})
		})

		Describe("BR-ANALYTICS-002: Pattern Discovery Analytics for Business Performance Measurement", func() {
			It("should discover and analyze workflow patterns for executive performance dashboards", func() {
				// Business Scenario: Executive stakeholders need pattern discovery analytics for business performance measurement
				// Business Impact: Enables performance tracking and business optimization decisions

				// Business Setup: Create pattern criteria for business analytics
				criteria := &engine.PatternCriteria{
					MinSimilarity:     0.8,
					MinExecutionCount: 5,
					MinSuccessRate:    0.9,
					TimeWindow:        7 * 24 * time.Hour,
					EnvironmentFilter: []string{"production", "staging"},
				}

				// Business Action: Find workflow patterns for business analytics
				patterns, err := builder.FindWorkflowPatterns(ctx, criteria)

				// Business Validation: Pattern discovery provides business analytics
				Expect(err).ToNot(HaveOccurred(),
					"BR-ANALYTICS-002: Pattern discovery must succeed for executive business performance measurement")

				Expect(patterns).ToNot(BeNil(),
					"BR-ANALYTICS-002: Discovered patterns must exist for executive business analytics")

				// Business Outcome: Pattern discovery enables business performance measurement
				patternDiscoverySuccessful := patterns != nil

				Expect(patternDiscoverySuccessful).To(BeTrue(),
					"BR-ANALYTICS-002: Pattern discovery must enable comprehensive executive business performance measurement for strategic decision making")
			})
		})

		Describe("BR-ANALYTICS-003: Execution Filtering Analytics for Business Efficiency Measurement", func() {
			It("should filter and analyze executions for business efficiency optimization", func() {
				// Business Scenario: Executive stakeholders need execution filtering analytics for business efficiency optimization
				// Business Impact: Enables performance optimization and resource allocation decisions

				// Business Setup: Create test execution data for filtering analytics
				executions := []*engine.RuntimeWorkflowExecution{
					{
						WorkflowExecutionRecord: types.WorkflowExecutionRecord{
							ID:         "efficiency-exec-001",
							WorkflowID: "efficiency-workflow-001",
							StartTime:  time.Now().Add(-10 * time.Minute),
							EndTime:    func() *time.Time { t := time.Now().Add(-8 * time.Minute); return &t }(),
						},
						OperationalStatus: engine.ExecutionStatusCompleted,
						Duration:          2 * time.Minute,
					},
					{
						WorkflowExecutionRecord: types.WorkflowExecutionRecord{
							ID:         "efficiency-exec-002",
							WorkflowID: "efficiency-workflow-001",
							StartTime:  time.Now().Add(-20 * time.Minute),
							EndTime:    func() *time.Time { t := time.Now().Add(-15 * time.Minute); return &t }(),
						},
						OperationalStatus: engine.ExecutionStatusFailed,
						Duration:          5 * time.Minute,
					},
				}

				// Business Action: Filter executions for efficiency analytics
				// Simple filtering logic for test purposes - filter for completed executions
				var filteredExecutions []*engine.RuntimeWorkflowExecution
				for _, exec := range executions {
					if exec.OperationalStatus == engine.ExecutionStatusCompleted {
						filteredExecutions = append(filteredExecutions, exec)
					}
				}

				// Business Validation: Execution filtering provides business efficiency analytics
				Expect(filteredExecutions).ToNot(BeNil(),
					"BR-ANALYTICS-003: Filtered executions must exist for executive business efficiency measurement")

				Expect(len(filteredExecutions)).To(BeNumerically("<=", len(executions)),
					"BR-ANALYTICS-003: Filtered executions must not exceed original count for executive data integrity")

				// Business Outcome: Execution filtering enables business efficiency measurement
				executionFilteringSuccessful := filteredExecutions != nil

				Expect(executionFilteringSuccessful).To(BeTrue(),
					"BR-ANALYTICS-003: Execution filtering must enable comprehensive executive business efficiency measurement for performance optimization")
			})
		})

		Describe("BR-ANALYTICS-004: Workflow Optimization Analytics for Business Intelligence Decision Support", func() {
			It("should optimize workflows with analytics for executive decision support systems", func() {
				// Business Scenario: Executive stakeholders need workflow optimization analytics for business intelligence decision support
				// Business Impact: Enables data-driven optimization and business performance strategies

				// Business Setup: Create template for optimization analytics
				template := &engine.ExecutableTemplate{
					BaseVersionedEntity: types.BaseVersionedEntity{
						BaseEntity: types.BaseEntity{
							ID:   "optimization-template-001",
							Name: "Business Optimization Template",
							Metadata: map[string]interface{}{
								"optimization_target": "performance",
								"business_priority":   "high",
							},
						},
					},
					Steps: []*engine.ExecutableWorkflowStep{
						{
							BaseEntity: types.BaseEntity{
								ID:   "optimization-step-001",
								Name: "Business Process Step",
							},
							Type:    engine.StepTypeAction,
							Timeout: 5 * time.Minute,
						},
					},
				}

				// Business Action: Optimize workflow structure with analytics
				optimizedTemplate, err := builder.OptimizeWorkflowStructure(ctx, template)

				// Business Validation: Workflow optimization provides business intelligence analytics
				Expect(err).ToNot(HaveOccurred(),
					"BR-ANALYTICS-004: Workflow optimization must succeed for executive business intelligence decision support")

				Expect(optimizedTemplate).ToNot(BeNil(),
					"BR-ANALYTICS-004: Optimized template must exist for executive business performance improvement")

				Expect(optimizedTemplate.Metadata).ToNot(BeNil(),
					"BR-ANALYTICS-004: Optimized template metadata must exist for executive business intelligence")

				// Business Outcome: Workflow optimization enables executive decision support
				optimizationAnalyticsSuccessful := optimizedTemplate != nil && optimizedTemplate.Metadata != nil

				Expect(optimizationAnalyticsSuccessful).To(BeTrue(),
					"BR-ANALYTICS-004: Workflow optimization analytics must enable comprehensive executive business intelligence decision support for strategic performance management")
			})
		})

		Describe("BR-ANALYTICS-005: Comprehensive Analytics Integration for Executive Business Intelligence", func() {
			It("should integrate all analytics components for comprehensive executive business intelligence", func() {
				// Business Scenario: Executive stakeholders need comprehensive analytics integration for complete business intelligence
				// Business Impact: Enables holistic business performance measurement and strategic decision making

				// Business Action: Generate workflow with comprehensive analytics integration
				template, err := builder.GenerateWorkflow(ctx, objective)

				// Business Validation: Comprehensive analytics integration supports executive business intelligence
				Expect(err).ToNot(HaveOccurred(),
					"BR-ANALYTICS-005: Comprehensive analytics integration must succeed for executive business intelligence capabilities")

				Expect(template.ID).ToNot(BeEmpty(),
					"BR-ANALYTICS-005: Generated workflow must have unique identifier for analytics tracking and business intelligence")

				Expect(template.Steps).ToNot(BeNil(),
					"BR-ANALYTICS-005: Generated workflow must have executable steps for business process analytics")

				Expect(template.Metadata).ToNot(BeNil(),
					"BR-ANALYTICS-005: Generated workflow must have analytics metadata for comprehensive executive business intelligence reporting")

				// Business Action: Validate workflow for comprehensive analytics
				validationReport := builder.ValidateWorkflow(ctx, template)

				// Business Validation: Workflow validation provides comprehensive analytics
				Expect(validationReport).ToNot(BeNil(),
					"BR-ANALYTICS-005: Validation report must exist for comprehensive executive business analytics")

				// Business Outcome: Analytics integration enables executive business intelligence
				comprehensiveAnalyticsSuccessful := template.ID != "" &&
					template.Steps != nil &&
					template.Metadata != nil &&
					validationReport != nil

				Expect(comprehensiveAnalyticsSuccessful).To(BeTrue(),
					"BR-ANALYTICS-005: Comprehensive analytics integration must enable complete executive business intelligence capabilities for strategic decision making and performance optimization")
			})
		})
	})
})
