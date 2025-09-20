package workflowengine

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// TDD Phase 2 Implementation: Objective Analysis Function Activation
// Following project guideline: Write failing tests first, then implement to pass
// Business Requirements: BR-IWB-XXX Objective analysis for intelligent workflow building

var _ = Describe("Objective Analysis Function Activation - TDD Phase 2", func() {
	var (
		builder *engine.DefaultIntelligentWorkflowBuilder
		logger  *logrus.Logger
		ctx     context.Context
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
		ctx = context.Background()

		// Create intelligent workflow builder
		builder = engine.NewIntelligentWorkflowBuilder(nil, nil, nil, nil, nil, nil, logger)
	})

	Describe("BR-IWB-XXX: analyzeObjective activation", func() {
		It("should analyze remediation objective and return comprehensive analysis", func() {
			// Arrange: Create remediation objective
			objective := &engine.WorkflowObjective{
				ID:          "obj-remediation-001",
				Type:        "remediation",
				Description: "Remediate high memory usage in production pods",
				Priority:    1, // high priority as int
				Constraints: map[string]interface{}{
					"alert_name":       "HighMemoryUsage",
					"severity":         "critical",
					"namespace":        "production",
					"resource_type":    "memory",
					"threshold":        "90%",
					"duration":         "10m",
					"max_downtime":     "5m",
					"rollback_enabled": true,
					"safety_level":     "high",
				},
				Status:    "pending",
				Progress:  0.0,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			// Act: Analyze objective (THIS WILL FAIL until we activate it)
			analysisResult := builder.AnalyzeObjective(objective.Description, objective.Constraints)

			// Assert: Should return comprehensive objective analysis
			Expect(analysisResult).ToNot(BeNil(), "Should return analysis result")
			Expect(analysisResult.Keywords).ToNot(BeEmpty(), "Should extract keywords")
			Expect(analysisResult.ActionTypes).ToNot(BeEmpty(), "Should identify action types")
			Expect(analysisResult.Complexity).To(BeNumerically(">", 0), "Should assess complexity")
			Expect(analysisResult.Priority).To(BeNumerically(">=", 1), "Should provide reasonable priority")

			// Should identify key characteristics
			Expect(analysisResult.Keywords).To(ContainElement("memory"), "Should identify memory keyword")
			Expect(analysisResult.ActionTypes).To(ContainElement("scaling"), "Should identify scaling action")
			Expect(analysisResult.RiskLevel).ToNot(BeEmpty(), "Should assess risk level")

			// Should suggest appropriate strategies
			Expect(analysisResult.Recommendation).ToNot(BeEmpty(), "Should provide recommendation")
			Expect(analysisResult.Constraints).ToNot(BeEmpty(), "Should include constraints")
		})

		It("should adapt analysis based on objective type and priority", func() {
			// Arrange: Create different objective types
			highPriorityObjective := &engine.WorkflowObjective{
				ID:          "obj-critical-001",
				Type:        "incident_response",
				Description: "CriticalServiceRestore",
				Priority:    1, // critical priority as int
				Constraints: map[string]interface{}{
					"service_down":    true,
					"impact_level":    "high",
					"customer_facing": true,
				},
				Status:    "pending",
				Progress:  0.0,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			lowPriorityObjective := &engine.WorkflowObjective{
				ID:          "obj-maintenance-001",
				Type:        "maintenance",
				Description: "RoutineMaintenance",
				Priority:    5, // low priority as int
				Constraints: map[string]interface{}{
					"scheduled":       true,
					"impact_level":    "minimal",
					"customer_facing": false,
				},
				Status:    "pending",
				Progress:  0.0,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			// Act: Analyze different objective types
			criticalAnalysis := builder.AnalyzeObjective(highPriorityObjective.Description, highPriorityObjective.Constraints)
			maintenanceAnalysis := builder.AnalyzeObjective(lowPriorityObjective.Description, lowPriorityObjective.Constraints)

			// Assert: Should adapt analysis based on type and priority
			Expect(criticalAnalysis.Complexity).To(BeNumerically(">", maintenanceAnalysis.Complexity),
				"Critical objectives should have higher complexity")
			Expect(criticalAnalysis.Priority).To(BeNumerically("<", maintenanceAnalysis.Priority),
				"Critical should have higher priority (lower number)")
			Expect(criticalAnalysis.RiskLevel).To(Equal("high"), "Critical should have high risk level")

			// Critical objectives should have more detailed analysis
			Expect(len(criticalAnalysis.Keywords)).To(BeNumerically(">=", len(maintenanceAnalysis.Keywords)),
				"Critical should have more keywords")
			Expect(criticalAnalysis.RiskLevel).To(Equal("high"),
				"Critical objectives should have high risk assessment")
		})

		It("should identify resource constraints and optimization opportunities", func() {
			// Arrange: Create resource-constrained objective
			objective := &engine.WorkflowObjective{
				ID:          "obj-resource-001",
				Type:        "optimization",
				Description: "ResourceOptimization",
				Priority:    3, // medium priority as int
				Constraints: map[string]interface{}{
					"resource_type":         "cpu",
					"utilization":           "85%",
					"cost_optimization":     true,
					"performance_target":    "improve_20%",
					"budget_limit":          "$500",
					"performance_sla":       "99.9%",
					"max_resource_increase": "50%",
				},
				Status:    "pending",
				Progress:  0.0,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			// Act: Analyze resource optimization objective
			analysisResult := builder.AnalyzeObjective(objective.Description, objective.Constraints)

			// Assert: Should identify resource constraints and opportunities
			Expect(analysisResult.Keywords).To(ContainElement("cpu"), "Should identify CPU keyword")
			Expect(analysisResult.ActionTypes).To(ContainElement("optimization"), "Should identify optimization action")
			Expect(analysisResult.Complexity).To(BeNumerically(">", 0), "Should assess complexity")

			// Should identify constraint-aware strategies
			Expect(analysisResult.Recommendation).To(ContainSubstring("scaling"), "Should recommend scaling strategies")
			Expect(analysisResult.RiskLevel).ToNot(BeEmpty(), "Should assess risk level")

			// Should respect budget constraints
			Expect(analysisResult.Priority).To(BeNumerically(">=", 1), "Should provide priority assessment")
		})

		It("should integrate with existing constraint extraction and cost optimization", func() {
			// Arrange: Create objective that should work with existing functions
			objective := &engine.WorkflowObjective{
				BaseEntity: types.BaseEntity{
					ID:   "obj-integration-001",
					Name: "IntegratedOptimization",
				},
				Type:     "remediation",
				Priority: "high",
				Context: map[string]interface{}{
					"cost_sensitive":    true,
					"resource_type":     "memory",
					"optimization_goal": "cost_performance",
				},
				Constraints: map[string]interface{}{
					"max_cost":        "$200",
					"min_performance": "95%",
					"rollback_time":   "2m",
				},
			}

			// Act: Analyze objective and test integration with existing functions
			analysisResult := builder.AnalyzeObjective(ctx, objective)

			// Test integration with Phase 1 activated functions
			constraints := builder.ExtractConstraintsFromObjective(objective)

			// Should work with cost optimization (if available)
			template := &engine.ExecutableTemplate{
				BaseVersionedEntity: types.BaseVersionedEntity{
					BaseEntity: types.BaseEntity{ID: "template-001"},
				},
			}
			optimizedTemplate := builder.ApplyCostOptimizationConstraints(template, constraints)

			// Assert: Should integrate with existing constraint and optimization functions
			Expect(analysisResult).ToNot(BeNil(), "Should provide analysis")
			Expect(constraints).ToNot(BeNil(), "Should work with constraint extraction")
			Expect(optimizedTemplate).ToNot(BeNil(), "Should work with cost optimization")

			// Analysis should inform constraint extraction
			Expect(analysisResult.RecommendedStrategies).To(ContainElement("cost_optimization"),
				"Should recommend cost optimization")
			Expect(len(constraints)).To(BeNumerically(">", 0), "Should extract constraints")
		})

		It("should provide risk assessment and mitigation strategies", func() {
			// Arrange: Create high-risk objective
			objective := &engine.WorkflowObjective{
				BaseEntity: types.BaseEntity{
					ID:   "obj-risky-001",
					Name: "ProductionDatabaseMigration",
				},
				Type:     "migration",
				Priority: "critical",
				Context: map[string]interface{}{
					"database_type":      "postgresql",
					"data_size":          "500GB",
					"downtime_sensitive": true,
					"business_critical":  true,
				},
				Constraints: map[string]interface{}{
					"max_downtime":      "30s",
					"data_integrity":    "required",
					"rollback_required": true,
				},
			}

			// Act: Analyze high-risk objective
			analysisResult := builder.AnalyzeObjective(ctx, objective)

			// Assert: Should provide comprehensive risk assessment
			Expect(analysisResult.RiskAssessment).ToNot(BeNil(), "Should provide risk assessment")
			Expect(analysisResult.RiskAssessment.Level).To(Equal("high"), "Should identify high risk")
			Expect(analysisResult.RiskAssessment.Factors).ToNot(BeEmpty(), "Should identify risk factors")
			Expect(analysisResult.RiskAssessment.Factors).To(ContainElement("data_integrity"),
				"Should identify data integrity risk")
			Expect(analysisResult.RiskAssessment.Factors).To(ContainElement("downtime_sensitive"),
				"Should identify downtime risk")

			// Should recommend risk mitigation strategies
			Expect(analysisResult.MitigationStrategies).ToNot(BeEmpty(), "Should provide mitigation strategies")
			Expect(analysisResult.MitigationStrategies).To(ContainElement("backup_verification"),
				"Should recommend backup verification")
			Expect(analysisResult.MitigationStrategies).To(ContainElement("rollback_plan"),
				"Should recommend rollback planning")
		})

		It("should handle edge cases and provide fallback analysis", func() {
			// Arrange: Create edge case objectives
			minimalObjective := &engine.WorkflowObjective{
				BaseEntity: types.BaseEntity{
					ID:   "obj-minimal-001",
					Name: "MinimalObjective",
				},
				Type: "unknown",
			}

			emptyContextObjective := &engine.WorkflowObjective{
				BaseEntity: types.BaseEntity{
					ID:   "obj-empty-001",
					Name: "EmptyContextObjective",
				},
				Type:    "remediation",
				Context: map[string]interface{}{},
			}

			// Act: Analyze edge case objectives
			minimalAnalysis := builder.AnalyzeObjective(ctx, minimalObjective)
			emptyAnalysis := builder.AnalyzeObjective(ctx, emptyContextObjective)

			// Assert: Should provide fallback analysis for edge cases
			Expect(minimalAnalysis).ToNot(BeNil(), "Should handle minimal objective")
			Expect(minimalAnalysis.AnalysisType).To(Equal("general"), "Should use general analysis for unknown type")
			Expect(minimalAnalysis.Confidence).To(BeNumerically(">=", 0.3), "Should provide reasonable fallback confidence")

			Expect(emptyAnalysis).ToNot(BeNil(), "Should handle empty context")
			Expect(emptyAnalysis.RecommendedStrategies).ToNot(BeEmpty(), "Should provide default strategies")
			Expect(emptyAnalysis.RecommendedStrategies).To(ContainElement("general_remediation"),
				"Should include general remediation strategy")
		})
	})

	Describe("Integration with intelligent workflow building", func() {
		It("should provide analysis that enhances workflow template generation", func() {
			// Arrange: Create objective for workflow generation
			objective := &engine.WorkflowObjective{
				BaseEntity: types.BaseEntity{
					ID:   "obj-workflow-001",
					Name: "WorkflowGenerationTest",
				},
				Type:     "remediation",
				Priority: "high",
				Context: map[string]interface{}{
					"alert_type":    "resource_constraint",
					"resource_type": "memory",
					"namespace":     "production",
					"complexity":    "medium",
				},
			}

			// Act: Analyze objective for workflow generation
			analysisResult := builder.AnalyzeObjective(ctx, objective)

			// Assert: Should provide analysis that enhances workflow generation
			Expect(analysisResult.WorkflowHints).ToNot(BeEmpty(), "Should provide workflow generation hints")
			Expect(analysisResult.WorkflowHints).To(HaveKey("step_types"), "Should suggest step types")
			Expect(analysisResult.WorkflowHints).To(HaveKey("execution_order"), "Should suggest execution order")

			// Should provide template selection guidance
			Expect(analysisResult.TemplateRecommendations).ToNot(BeEmpty(), "Should recommend templates")
			Expect(analysisResult.TemplateRecommendations).To(ContainElement("resource_scaling_template"),
				"Should recommend appropriate templates")
		})
	})
})
