package workflowengine

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// TDD Phase 2 Implementation: Objective Analysis Function Activation
// Following project guideline: Write failing tests first, then implement to pass
// Business Requirements: BR-IWB-XXX Objective analysis for intelligent workflow building

var _ = Describe("Objective Analysis Function Activation - TDD Phase 2", func() {
	var (
		builder *engine.DefaultIntelligentWorkflowBuilder
		logger  *logrus.Logger
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce test noise

		// Create intelligent workflow builder using new config pattern
		config := &engine.IntelligentWorkflowBuilderConfig{
			LLMClient:       nil,
			VectorDB:        nil,
			AnalyticsEngine: nil,
			PatternStore:    nil,
			ExecutionRepo:   nil,
			Logger:          logger,
		}

		var err error
		builder, err = engine.NewIntelligentWorkflowBuilder(config)
		Expect(err).ToNot(HaveOccurred(), "Workflow builder creation should not fail")
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
				ID:          "obj-integration-001",
				Type:        "remediation",
				Description: "IntegratedOptimization",
				Priority:    1, // high priority as int
				Constraints: map[string]interface{}{
					"max_cost":          "$200",
					"min_performance":   "95%",
					"rollback_time":     "2m",
					"cost_sensitive":    true,
					"resource_type":     "memory",
					"optimization_goal": "cost_performance",
				},
				Status:    "pending",
				Progress:  0.0,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			// Act: Analyze objective using correct signature (description, constraints)
			analysisResult := builder.AnalyzeObjective(objective.Description, objective.Constraints)

			// Assert: Should provide analysis that can integrate with other functions
			Expect(analysisResult).ToNot(BeNil(), "Should provide analysis")
			Expect(analysisResult.Keywords).ToNot(BeEmpty(), "Should extract keywords")
			Expect(analysisResult.ActionTypes).ToNot(BeEmpty(), "Should identify action types")
			Expect(analysisResult.Complexity).To(BeNumerically(">", 0), "Should assess complexity")

			// Analysis should provide meaningful insights for integration
			Expect(analysisResult.Keywords).To(ContainElement("memory"), "Should identify memory keyword")
			Expect(analysisResult.ActionTypes).To(ContainElement("optimization"), "Should identify optimization action")
			Expect(analysisResult.Recommendation).To(ContainSubstring("cost"), "Should consider cost constraints")
		})

		It("should provide risk assessment and mitigation strategies", func() {
			// Arrange: Create high-risk objective
			objective := &engine.WorkflowObjective{
				ID:          "obj-risky-001",
				Type:        "migration",
				Description: "ProductionDatabaseMigration",
				Priority:    1, // critical priority as int
				Constraints: map[string]interface{}{
					"max_downtime":       "30s",
					"data_integrity":     "required",
					"rollback_required":  true,
					"database_type":      "postgresql",
					"data_size":          "500GB",
					"downtime_sensitive": true,
					"business_critical":  true,
				},
				Status:    "pending",
				Progress:  0.0,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			// Act: Analyze high-risk objective using correct signature
			analysisResult := builder.AnalyzeObjective(objective.Description, objective.Constraints)

			// Assert: Should provide comprehensive risk assessment in basic fields
			Expect(analysisResult).ToNot(BeNil(), "Should provide analysis")
			Expect(analysisResult.RiskLevel).To(Equal("high"), "Should identify high risk level")
			Expect(analysisResult.Keywords).To(ContainElement("database"), "Should identify database keyword")
			Expect(analysisResult.ActionTypes).To(ContainElement("migration"), "Should identify migration action")
			Expect(analysisResult.Complexity).To(BeNumerically(">", 0.7), "Should assess high complexity")

			// Should provide recommendations considering risk factors
			Expect(analysisResult.Recommendation).To(ContainSubstring("rollback"), "Should mention rollback strategy")
			Expect(analysisResult.Priority).To(Equal(1), "Should assign high priority")
		})

		It("should handle edge cases and provide fallback analysis", func() {
			// Arrange: Create edge case objectives
			minimalObjective := &engine.WorkflowObjective{
				ID:          "obj-minimal-001",
				Type:        "unknown",
				Description: "MinimalObjective",
				Priority:    3, // medium priority
				Status:      "pending",
				Progress:    0.0,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			emptyConstraintsObjective := &engine.WorkflowObjective{
				ID:          "obj-empty-001",
				Type:        "remediation",
				Description: "EmptyConstraintsObjective",
				Priority:    2,
				Constraints: map[string]interface{}{},
				Status:      "pending",
				Progress:    0.0,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			// Act: Analyze edge case objectives using correct signature
			minimalAnalysis := builder.AnalyzeObjective(minimalObjective.Description, minimalObjective.Constraints)
			emptyAnalysis := builder.AnalyzeObjective(emptyConstraintsObjective.Description, emptyConstraintsObjective.Constraints)

			// Assert: Should provide fallback analysis for edge cases
			Expect(minimalAnalysis).ToNot(BeNil(), "Should handle minimal objective")
			Expect(minimalAnalysis.Keywords).ToNot(BeEmpty(), "Should extract at least basic keywords")
			Expect(minimalAnalysis.Complexity).To(BeNumerically(">=", 0), "Should provide some complexity assessment")

			Expect(emptyAnalysis).ToNot(BeNil(), "Should handle empty constraints")
			Expect(emptyAnalysis.ActionTypes).ToNot(BeEmpty(), "Should identify default action types")
			Expect(emptyAnalysis.RiskLevel).ToNot(BeEmpty(), "Should provide default risk assessment")
		})
	})

	Describe("Integration with intelligent workflow building", func() {
		It("should provide analysis that enhances workflow template generation", func() {
			// Arrange: Create objective for workflow generation
			objective := &engine.WorkflowObjective{
				ID:          "obj-workflow-001",
				Type:        "remediation",
				Description: "WorkflowGenerationTest",
				Priority:    1, // high priority
				Constraints: map[string]interface{}{
					"alert_type":    "resource_constraint",
					"resource_type": "memory",
					"namespace":     "production",
					"complexity":    "medium",
				},
				Status:    "pending",
				Progress:  0.0,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			// Act: Analyze objective for workflow generation using correct signature
			analysisResult := builder.AnalyzeObjective(objective.Description, objective.Constraints)

			// Assert: Should provide analysis that enhances workflow generation
			Expect(analysisResult).ToNot(BeNil(), "Should provide analysis")
			Expect(analysisResult.Keywords).To(ContainElement("memory"), "Should identify memory keyword")
			Expect(analysisResult.ActionTypes).To(ContainElement("scaling"), "Should suggest scaling actions")
			Expect(analysisResult.Complexity).To(BeNumerically(">", 0), "Should assess complexity")

			// Should provide recommendations suitable for workflow building
			Expect(analysisResult.Recommendation).To(ContainSubstring("resource"), "Should consider resource constraints")
			Expect(analysisResult.RiskLevel).ToNot(BeEmpty(), "Should assess risk for workflow planning")
		})
	})
})
