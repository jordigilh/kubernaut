//go:build unit
// +build unit

package workflowengine

import (
	"testing"
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

// BR-ANALYTICS-001 through BR-ANALYTICS-007: Advanced Analytics Unit Testing - Pyramid Testing (70% Unit Coverage)
// Business Impact: Validates advanced analytics capabilities for AI-driven workflow optimization
// Stakeholder Value: Operations teams can trust analytics-driven automation decisions
var _ = Describe("BR-ANALYTICS-001 through BR-ANALYTICS-007: Advanced Analytics Unit Testing", func() {
	var (
		// Mock ONLY external dependencies per pyramid principles
		mockLogger *logrus.Logger

		// Use REAL business logic components
		intelligentBuilder *engine.DefaultIntelligentWorkflowBuilder

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Mock external dependencies only
		mockLogger = logrus.New()
		mockLogger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

		// Create REAL business logic component for analytics testing
		// Mock only external dependencies, use real business logic
		mockLLMClient := mocks.NewMockLLMClient()
		mockVectorDB := mocks.NewMockVectorDatabase()
		mockExecutionRepo := mocks.NewWorkflowExecutionRepositoryMock()

		// Create workflow builder using new config pattern
		config := &engine.IntelligentWorkflowBuilderConfig{
			LLMClient:       mockLLMClient,     // External: Mock
			VectorDB:        mockVectorDB,      // External: Mock
			AnalyticsEngine: nil,               // AnalyticsEngine: Not needed for testing real analytics methods
			PatternStore:    nil,               // PatternStore: Not needed for analytics tests
			ExecutionRepo:   mockExecutionRepo, // External: Mock
			Logger:          mockLogger,        // External: Mock (logging infrastructure)
		}

		var err error
		intelligentBuilder, err = engine.NewIntelligentWorkflowBuilder(config)
		Expect(err).ToNot(HaveOccurred(), "Workflow builder creation should not fail")
	})

	AfterEach(func() {
		cancel()
	})

	// BR-ANALYTICS-001: Advanced Insights Generation
	Context("BR-ANALYTICS-001: Advanced Insights Generation", func() {
		It("should generate comprehensive workflow insights using real analytics algorithms", func() {
			// Business Scenario: System analyzes workflow execution patterns to generate actionable insights
			// Business Impact: Enables data-driven workflow optimization decisions

			// Create realistic workflow for analysis
			workflow := createAnalyticsTestWorkflow("insights-generation-001")

			// Create realistic execution history for analysis
			executionHistory := createRealisticExecutionHistory(10)

			// Test REAL business logic for advanced insights generation
			insights := intelligentBuilder.GenerateAdvancedInsights(ctx, workflow, executionHistory)

			// Validate REAL business analytics outcomes
			Expect(insights).ToNot(BeNil(),
				"BR-ANALYTICS-001: Advanced insights generation must produce results")
			Expect(insights.WorkflowID).To(Equal(workflow.ID),
				"BR-ANALYTICS-001: Insights must be associated with correct workflow")
			Expect(insights.InsightType).To(Equal("comprehensive"),
				"BR-ANALYTICS-001: Must generate comprehensive insights for business analysis")
			Expect(insights.Confidence).To(BeNumerically(">=", 0.0),
				"BR-ANALYTICS-001: Must provide confidence score for business decision making")
			Expect(insights.Confidence).To(BeNumerically("<=", 1.0),
				"BR-ANALYTICS-001: Confidence score must be within valid range")
			Expect(len(insights.Insights)).To(BeNumerically(">", 0),
				"BR-ANALYTICS-001: Must generate actionable insights for business optimization")
			Expect(insights.GeneratedAt).ToNot(BeZero(),
				"BR-ANALYTICS-001: Must track insight generation time for business audit")

			// Validate insight quality for business decision making
			for _, insight := range insights.Insights {
				Expect(insight.Type).ToNot(BeEmpty(),
					"BR-ANALYTICS-001: Each insight must have a type for business categorization")
				Expect(insight.Confidence).To(BeNumerically(">=", 0.0),
					"BR-ANALYTICS-001: Each insight must have confidence for business validation")
				Expect(insight.Description).ToNot(BeEmpty(),
					"BR-ANALYTICS-001: Each insight must have description for business understanding")
			}

			// Business Value: Comprehensive insights enable data-driven optimization
		})

		It("should handle workflows with no execution history gracefully", func() {
			// Business Scenario: New workflows without historical data still need analysis
			// Business Impact: Ensures analytics work for new workflow patterns

			// Create workflow without execution history
			workflow := createAnalyticsTestWorkflow("new-workflow-001")
			emptyHistory := []*engine.RuntimeWorkflowExecution{}

			// Test REAL business logic for graceful handling
			insights := intelligentBuilder.GenerateAdvancedInsights(ctx, workflow, emptyHistory)

			// Validate REAL business analytics graceful handling
			Expect(insights).ToNot(BeNil(),
				"BR-ANALYTICS-001: Must handle workflows without history gracefully")
			Expect(insights.WorkflowID).To(Equal(workflow.ID),
				"BR-ANALYTICS-001: Must maintain workflow association even without history")
			Expect(insights.InsightType).To(Equal("basic"),
				"BR-ANALYTICS-001: Must indicate limited insights for workflows without history")
			Expect(insights.Confidence).To(Equal(0.0),
				"BR-ANALYTICS-001: Must indicate low confidence without historical data")

			// Business Value: Graceful handling ensures system reliability for new workflows
		})
	})

	// BR-ANALYTICS-002: Predictive Metrics Calculation
	Context("BR-ANALYTICS-002: Predictive Metrics Calculation", func() {
		It("should calculate predictive analytics for workflow performance", func() {
			// Business Scenario: System predicts future workflow performance based on historical data
			// Business Impact: Enables proactive optimization and capacity planning

			// Create realistic workflow for prediction
			workflow := createAnalyticsTestWorkflow("predictive-analysis-001")

			// Create realistic historical metrics for prediction
			historicalMetrics := createRealisticWorkflowMetrics(15)

			// Test REAL business logic for predictive metrics calculation
			predictiveMetrics := intelligentBuilder.CalculatePredictiveMetrics(ctx, workflow, historicalMetrics)

			// Validate REAL business predictive analytics outcomes
			Expect(predictiveMetrics).ToNot(BeNil(),
				"BR-ANALYTICS-002: Predictive metrics calculation must produce results")
			Expect(predictiveMetrics.WorkflowID).To(Equal(workflow.ID),
				"BR-ANALYTICS-002: Predictions must be associated with correct workflow")
			Expect(predictiveMetrics.PredictedSuccessRate).To(BeNumerically(">=", 0.0),
				"BR-ANALYTICS-002: Predicted success rate must be within valid range")
			Expect(predictiveMetrics.PredictedSuccessRate).To(BeNumerically("<=", 1.0),
				"BR-ANALYTICS-002: Predicted success rate must be within valid range")
			Expect(predictiveMetrics.PredictedExecutionTime).To(BeNumerically(">", 0),
				"BR-ANALYTICS-002: Must predict execution time for capacity planning")
			Expect(predictiveMetrics.ConfidenceLevel).To(BeNumerically(">=", 0.0),
				"BR-ANALYTICS-002: Must provide confidence level for business decision making")
			Expect(predictiveMetrics.ConfidenceLevel).To(BeNumerically("<=", 1.0),
				"BR-ANALYTICS-002: Confidence level must be within valid range")
			Expect(predictiveMetrics.RiskAssessment).ToNot(BeEmpty(),
				"BR-ANALYTICS-002: Must provide risk assessment for business planning")

			// Validate prediction quality for business planning
			Expect(predictiveMetrics.PredictedResourceUsage).To(BeNumerically(">=", 0.0),
				"BR-ANALYTICS-002: Resource usage prediction must be non-negative")
			Expect(predictiveMetrics.PredictedResourceUsage).To(BeNumerically("<=", 1.0),
				"BR-ANALYTICS-002: Resource usage prediction must be within valid range")

			// Business Value: Predictive metrics enable proactive capacity planning
		})

		It("should provide high confidence predictions with sufficient historical data", func() {
			// Business Scenario: System provides reliable predictions when sufficient data is available
			// Business Impact: Enables confident business decisions based on analytics

			// Create workflow with extensive historical data
			workflow := createAnalyticsTestWorkflow("high-confidence-prediction-001")
			extensiveMetrics := createRealisticWorkflowMetrics(50) // Large dataset

			// Test REAL business logic for high-confidence predictions
			predictiveMetrics := intelligentBuilder.CalculatePredictiveMetrics(ctx, workflow, extensiveMetrics)

			// Validate REAL business high-confidence prediction outcomes
			Expect(predictiveMetrics.ConfidenceLevel).To(BeNumerically(">=", 0.7),
				"BR-ANALYTICS-002: Must provide high confidence with sufficient historical data")
			Expect(predictiveMetrics.RiskAssessment).To(ContainSubstring("low"),
				"BR-ANALYTICS-002: High confidence should correlate with low risk assessment")

			// Business Value: High confidence predictions enable reliable business planning
		})
	})

	// BR-ANALYTICS-003: Performance Analytics Integration
	Context("BR-ANALYTICS-003: Performance Analytics Integration", func() {
		It("should integrate performance analytics into workflow optimization", func() {
			// Business Scenario: System uses performance analytics to optimize workflow execution
			// Business Impact: Improves operational efficiency through data-driven optimization

			// Create workflow template for optimization
			template := createAnalyticsOptimizationTemplate("performance-optimization-001")

			// Test REAL business logic for performance analytics integration
			// This tests the integration of analytics into the optimization process
			optimizedTemplate, err := intelligentBuilder.OptimizeWorkflowStructure(ctx, template)

			// Validate REAL business performance analytics integration
			Expect(err).ToNot(HaveOccurred(),
				"BR-ANALYTICS-003: Performance analytics integration must succeed")
			Expect(optimizedTemplate).ToNot(BeNil(),
				"BR-ANALYTICS-003: Must produce optimized template based on analytics")
			Expect(optimizedTemplate.Metadata["analytics_applied"]).To(BeTrue(),
				"BR-ANALYTICS-003: Must track analytics application for business monitoring")
			Expect(optimizedTemplate.Metadata["performance_optimized"]).To(BeTrue(),
				"BR-ANALYTICS-003: Must track performance optimization for business validation")

			// Validate performance improvement indicators
			if originalSteps := len(template.Steps); originalSteps > 0 {
				optimizedSteps := len(optimizedTemplate.Steps)
				Expect(optimizedSteps).To(BeNumerically(">=", originalSteps),
					"BR-ANALYTICS-003: Optimization should maintain or improve step structure")
			}

			// Business Value: Performance analytics drive measurable operational improvements
		})
	})

	// BR-ANALYTICS-004: Trend Analysis and Pattern Recognition
	Context("BR-ANALYTICS-004: Trend Analysis and Pattern Recognition", func() {
		It("should analyze execution trends and recognize performance patterns", func() {
			// Business Scenario: System identifies trends and patterns in workflow execution
			// Business Impact: Enables predictive maintenance and proactive optimization

			// Create workflow with trending execution data
			workflow := createAnalyticsTestWorkflow("trend-analysis-001")
			trendingHistory := createTrendingExecutionHistory(30) // Data with clear trends

			// Test REAL business logic for advanced insights that include trend analysis
			insights := intelligentBuilder.GenerateAdvancedInsights(ctx, workflow, trendingHistory)

			// Validate REAL business trend analysis outcomes through insights
			Expect(insights).ToNot(BeNil(),
				"BR-ANALYTICS-004: Trend analysis must produce results")
			Expect(insights.WorkflowID).To(Equal(workflow.ID),
				"BR-ANALYTICS-004: Trend analysis must be associated with correct workflow")
			Expect(len(insights.Insights)).To(BeNumerically(">", 0),
				"BR-ANALYTICS-004: Must identify trends in execution data")
			Expect(insights.Confidence).To(BeNumerically(">=", 0.0),
				"BR-ANALYTICS-004: Must provide confidence for trend analysis")

			// Validate insight quality for business trend analysis
			for _, insight := range insights.Insights {
				Expect(insight.Type).ToNot(BeEmpty(),
					"BR-ANALYTICS-004: Each insight must have a type for business categorization")
				Expect(insight.Confidence).To(BeNumerically(">=", 0.0),
					"BR-ANALYTICS-004: Each insight must have confidence for business validation")
				Expect(insight.Description).ToNot(BeEmpty(),
					"BR-ANALYTICS-004: Each insight must have description for business understanding")
			}

			// Business Value: Trend analysis enables proactive operational management
		})
	})

	// BR-ANALYTICS-005: Failure Pattern Analysis
	Context("BR-ANALYTICS-005: Failure Pattern Analysis", func() {
		It("should analyze failure patterns and provide remediation insights", func() {
			// Business Scenario: System analyzes failure patterns to prevent future failures
			// Business Impact: Reduces operational incidents through predictive failure analysis

			// Create execution history with failure patterns
			failureHistory := createFailurePatternExecutionHistory(25)
			workflow := createAnalyticsTestWorkflow("failure-analysis-001")

			// Test REAL business logic for failure pattern analysis through advanced insights
			insights := intelligentBuilder.GenerateAdvancedInsights(ctx, workflow, failureHistory)

			// Validate REAL business failure analysis outcomes
			Expect(insights).ToNot(BeNil(),
				"BR-ANALYTICS-005: Failure pattern analysis must produce results")
			Expect(len(insights.Insights)).To(BeNumerically(">", 0),
				"BR-ANALYTICS-005: Must identify failure patterns in execution data")
			Expect(insights.Metadata).ToNot(BeNil(),
				"BR-ANALYTICS-005: Must provide metadata for business analysis")

			// Validate insight quality for business failure prevention
			for _, insight := range insights.Insights {
				Expect(insight.Type).ToNot(BeEmpty(),
					"BR-ANALYTICS-005: Each insight must have failure type for business categorization")
				Expect(insight.Confidence).To(BeNumerically(">=", 0.0),
					"BR-ANALYTICS-005: Insight confidence must be meaningful for business analysis")
				Expect(insight.Description).ToNot(BeEmpty(),
					"BR-ANALYTICS-005: Each insight must have description for business understanding")
			}

			// Business Value: Failure analysis prevents operational incidents
		})
	})

	// BR-ANALYTICS-006: Resource Utilization Analytics
	Context("BR-ANALYTICS-006: Resource Utilization Analytics", func() {
		It("should analyze resource utilization patterns and optimize allocation", func() {
			// Business Scenario: System analyzes resource usage to optimize allocation
			// Business Impact: Reduces infrastructure costs through efficient resource utilization

			// Create execution history with resource utilization data
			resourceHistory := createResourceUtilizationExecutionHistory(20)
			workflow := createAnalyticsTestWorkflow("resource-analysis-001")

			// Test REAL business logic for resource utilization analysis through advanced insights
			insights := intelligentBuilder.GenerateAdvancedInsights(ctx, workflow, resourceHistory)

			// Validate REAL business resource analysis outcomes
			Expect(insights).ToNot(BeNil(),
				"BR-ANALYTICS-006: Resource utilization analysis must produce results")
			Expect(insights.WorkflowID).To(Equal(workflow.ID),
				"BR-ANALYTICS-006: Resource analysis must be associated with correct workflow")
			Expect(len(insights.Insights)).To(BeNumerically(">", 0),
				"BR-ANALYTICS-006: Must provide resource utilization insights")
			Expect(insights.Confidence).To(BeNumerically(">=", 0.0),
				"BR-ANALYTICS-006: Must provide confidence for resource analysis")

			// Validate resource insight quality
			for _, insight := range insights.Insights {
				Expect(insight.Type).ToNot(BeEmpty(),
					"BR-ANALYTICS-006: Each insight must have type for business categorization")
				Expect(insight.Confidence).To(BeNumerically(">=", 0.0),
					"BR-ANALYTICS-006: Each insight must have confidence for business validation")
				Expect(insight.Description).ToNot(BeEmpty(),
					"BR-ANALYTICS-006: Each insight must have description for business understanding")
			}

			// Business Value: Resource analytics reduce infrastructure costs
		})
	})

	// BR-ANALYTICS-007: Comprehensive Analytics Reporting
	Context("BR-ANALYTICS-007: Comprehensive Analytics Reporting", func() {
		It("should generate comprehensive analytics reports for business stakeholders", func() {
			// Business Scenario: System generates comprehensive reports for business decision making
			// Business Impact: Enables data-driven business decisions through comprehensive reporting

			// Create comprehensive execution dataset
			comprehensiveHistory := createComprehensiveExecutionHistory(40)
			workflow := createAnalyticsTestWorkflow("comprehensive-reporting-001")

			// Test REAL business logic for comprehensive reporting through advanced insights and predictive metrics
			insights := intelligentBuilder.GenerateAdvancedInsights(ctx, workflow, comprehensiveHistory)

			// Create historical metrics for predictive analysis
			historicalMetrics := createRealisticWorkflowMetrics(20)
			predictiveMetrics := intelligentBuilder.CalculatePredictiveMetrics(ctx, workflow, historicalMetrics)

			// Validate REAL business comprehensive reporting outcomes
			Expect(insights).ToNot(BeNil(),
				"BR-ANALYTICS-007: Comprehensive analytics reporting must produce insights")
			Expect(predictiveMetrics).ToNot(BeNil(),
				"BR-ANALYTICS-007: Comprehensive analytics reporting must produce predictions")
			Expect(insights.WorkflowID).To(Equal(workflow.ID),
				"BR-ANALYTICS-007: Report must be associated with correct workflow")
			Expect(insights.InsightType).To(Equal("comprehensive"),
				"BR-ANALYTICS-007: Must generate comprehensive insight type")
			Expect(insights.GeneratedAt).ToNot(BeZero(),
				"BR-ANALYTICS-007: Must track report generation time")

			// Validate comprehensive analytics components
			Expect(len(insights.Insights)).To(BeNumerically(">", 0),
				"BR-ANALYTICS-007: Must provide comprehensive insights for business monitoring")
			Expect(insights.Confidence).To(BeNumerically(">=", 0.0),
				"BR-ANALYTICS-007: Must provide confidence for business decision making")
			Expect(predictiveMetrics.PredictedSuccessRate).To(BeNumerically(">=", 0.0),
				"BR-ANALYTICS-007: Must provide predictive metrics for business planning")

			// Validate business-focused insights
			for _, insight := range insights.Insights {
				Expect(insight.Type).ToNot(BeEmpty(),
					"BR-ANALYTICS-007: Each insight must have business-meaningful type")
				Expect(insight.Confidence).To(BeNumerically(">=", 0.0),
					"BR-ANALYTICS-007: Each insight must have measurable confidence")
				Expect(insight.Description).ToNot(BeEmpty(),
					"BR-ANALYTICS-007: Each insight must have business description")
			}

			// Business Value: Comprehensive reports enable strategic business decisions
		})
	})
})

// Helper functions for advanced analytics testing
// These create realistic test data for REAL business logic validation

func createAnalyticsTestWorkflow(workflowID string) *engine.Workflow {
	template := engine.NewWorkflowTemplate("analytics-test-template", "Analytics Test Workflow")

	// Add realistic steps for analytics testing
	steps := []*engine.ExecutableWorkflowStep{
		{
			BaseEntity: types.BaseEntity{
				ID:   "analytics-step-1",
				Name: "Data Collection Step",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "data_collection",
				Parameters: map[string]interface{}{
					"analytics_enabled":  true,
					"metrics_collection": true,
				},
			},
		},
		{
			BaseEntity: types.BaseEntity{
				ID:   "analytics-step-2",
				Name: "Processing Step",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "data_processing",
				Parameters: map[string]interface{}{
					"performance_tracking": true,
					"resource_monitoring":  true,
				},
			},
			Dependencies: []string{"analytics-step-1"},
		},
	}

	template.Steps = steps
	workflow := engine.NewWorkflow(workflowID, template)

	// Add analytics-relevant metadata
	workflow.Metadata["analytics_enabled"] = true
	workflow.Metadata["performance_tracking"] = true

	return workflow
}

func createRealisticExecutionHistory(count int) []*engine.RuntimeWorkflowExecution {
	executions := make([]*engine.RuntimeWorkflowExecution, count)

	for i := 0; i < count; i++ {
		execution := engine.NewRuntimeWorkflowExecution(
			fmt.Sprintf("analytics-execution-%d", i+1),
			"analytics-workflow")

		// Simulate realistic execution patterns
		baseTime := time.Now().Add(-time.Duration(count-i) * time.Hour)
		execution.StartTime = baseTime
		endTime := baseTime.Add(time.Duration(300+i*50) * time.Second) // Varying execution times
		execution.EndTime = &endTime
		execution.Duration = endTime.Sub(baseTime)

		// Simulate success/failure patterns (80% success rate)
		if i%5 == 0 {
			execution.OperationalStatus = engine.ExecutionStatusFailed
		} else {
			execution.OperationalStatus = engine.ExecutionStatusCompleted
		}

		// Add realistic metadata for analytics
		execution.Metadata["resource_usage"] = 0.3 + float64(i%10)*0.05 // Varying resource usage
		execution.Metadata["step_count"] = 2 + i%3                      // Varying complexity
		execution.Metadata["retry_count"] = i % 3                       // Some retries

		executions[i] = execution
	}

	return executions
}

func createRealisticWorkflowMetrics(count int) []*engine.WorkflowMetrics {
	metrics := make([]*engine.WorkflowMetrics, count)

	for i := 0; i < count; i++ {
		metrics[i] = &engine.WorkflowMetrics{
			AverageExecutionTime: time.Duration(250+i*20) * time.Second, // Trending execution time
			SuccessRate:          0.75 + float64(i%10)*0.02,             // Varying success rate
			ResourceUtilization:  0.4 + float64(i%8)*0.05,               // Varying resource usage
			ErrorRate:            0.05 + float64(i%6)*0.01,              // Varying error rate
		}
	}

	return metrics
}

func createAnalyticsOptimizationTemplate(templateID string) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate(templateID, "Analytics Optimization Template")

	// Create template with optimization opportunities
	steps := []*engine.ExecutableWorkflowStep{
		{
			BaseEntity: types.BaseEntity{ID: "opt-step-1", Name: "Optimization Step 1"},
			Type:       engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "optimization_candidate",
				Parameters: map[string]interface{}{
					"cpu_intensive":  true,
					"parallelizable": true,
				},
			},
		},
		{
			BaseEntity: types.BaseEntity{ID: "opt-step-2", Name: "Optimization Step 2"},
			Type:       engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "optimization_candidate",
				Parameters: map[string]interface{}{
					"memory_intensive": true,
					"cacheable":        true,
				},
			},
		},
	}

	template.Steps = steps
	template.Metadata = map[string]interface{}{
		"optimization_enabled": true,
		"analytics_level":      "advanced",
	}

	return template
}

func createTrendingExecutionHistory(count int) []*engine.RuntimeWorkflowExecution {
	executions := make([]*engine.RuntimeWorkflowExecution, count)

	for i := 0; i < count; i++ {
		execution := engine.NewRuntimeWorkflowExecution(
			fmt.Sprintf("trending-execution-%d", i+1),
			"trending-workflow")

		// Create clear trending patterns
		baseTime := time.Now().Add(-time.Duration(count-i) * time.Hour)
		execution.StartTime = baseTime

		// Simulate degrading performance trend
		trendFactor := 1.0 + float64(i)*0.1 // Performance degrading over time
		executionTime := time.Duration(float64(300*time.Second) * trendFactor)
		endTime := baseTime.Add(executionTime)
		execution.EndTime = &endTime
		execution.Duration = executionTime

		// Success rate trending down
		if float64(i)/float64(count) > 0.7 && i%3 == 0 {
			execution.OperationalStatus = engine.ExecutionStatusFailed
		} else {
			execution.OperationalStatus = engine.ExecutionStatusCompleted
		}

		// Add trending metadata
		execution.Metadata["performance_trend"] = "degrading"
		execution.Metadata["resource_trend"] = 0.3 + float64(i)*0.02 // Increasing resource usage

		executions[i] = execution
	}

	return executions
}

func createFailurePatternExecutionHistory(count int) []*engine.RuntimeWorkflowExecution {
	executions := make([]*engine.RuntimeWorkflowExecution, count)

	for i := 0; i < count; i++ {
		execution := engine.NewRuntimeWorkflowExecution(
			fmt.Sprintf("failure-pattern-execution-%d", i+1),
			"failure-pattern-workflow")

		baseTime := time.Now().Add(-time.Duration(count-i) * time.Hour)
		execution.StartTime = baseTime
		endTime := baseTime.Add(time.Duration(200+i*30) * time.Second)
		execution.EndTime = &endTime
		execution.Duration = endTime.Sub(baseTime)

		// Create specific failure patterns
		if i%7 == 0 { // Memory-related failures
			execution.OperationalStatus = engine.ExecutionStatusFailed
			execution.Metadata["failure_type"] = "memory_exhaustion"
			execution.Metadata["failure_pattern"] = "resource_constraint"
		} else if i%11 == 0 { // Network-related failures
			execution.OperationalStatus = engine.ExecutionStatusFailed
			execution.Metadata["failure_type"] = "network_timeout"
			execution.Metadata["failure_pattern"] = "connectivity_issue"
		} else {
			execution.OperationalStatus = engine.ExecutionStatusCompleted
		}

		executions[i] = execution
	}

	return executions
}

func createResourceUtilizationExecutionHistory(count int) []*engine.RuntimeWorkflowExecution {
	executions := make([]*engine.RuntimeWorkflowExecution, count)

	for i := 0; i < count; i++ {
		execution := engine.NewRuntimeWorkflowExecution(
			fmt.Sprintf("resource-execution-%d", i+1),
			"resource-workflow")

		baseTime := time.Now().Add(-time.Duration(count-i) * time.Hour)
		execution.StartTime = baseTime
		endTime := baseTime.Add(time.Duration(180+i*40) * time.Second)
		execution.EndTime = &endTime
		execution.Duration = endTime.Sub(baseTime)
		execution.OperationalStatus = engine.ExecutionStatusCompleted

		// Add detailed resource utilization data
		execution.Metadata["cpu_usage"] = 0.2 + float64(i%10)*0.05
		execution.Metadata["memory_usage"] = 0.3 + float64(i%8)*0.06
		execution.Metadata["network_io"] = 0.1 + float64(i%6)*0.03
		execution.Metadata["disk_io"] = 0.15 + float64(i%7)*0.04
		execution.Metadata["resource_efficiency"] = 0.6 + float64(i%5)*0.08

		executions[i] = execution
	}

	return executions
}

func createComprehensiveExecutionHistory(count int) []*engine.RuntimeWorkflowExecution {
	executions := make([]*engine.RuntimeWorkflowExecution, count)

	for i := 0; i < count; i++ {
		execution := engine.NewRuntimeWorkflowExecution(
			fmt.Sprintf("comprehensive-execution-%d", i+1),
			"comprehensive-workflow")

		baseTime := time.Now().Add(-time.Duration(count-i) * time.Hour)
		execution.StartTime = baseTime
		endTime := baseTime.Add(time.Duration(240+i*35) * time.Second)
		execution.EndTime = &endTime
		execution.Duration = endTime.Sub(baseTime)

		// Mix of success and failure patterns
		if i%8 == 0 {
			execution.OperationalStatus = engine.ExecutionStatusFailed
		} else {
			execution.OperationalStatus = engine.ExecutionStatusCompleted
		}

		// Comprehensive metadata for reporting
		execution.Metadata["business_impact"] = []string{"low", "medium", "high"}[i%3]
		execution.Metadata["cost_impact"] = 10.0 + float64(i)*2.5
		execution.Metadata["user_satisfaction"] = 0.7 + float64(i%4)*0.075
		execution.Metadata["sla_compliance"] = i%9 != 0 // 88% SLA compliance
		execution.Metadata["performance_category"] = []string{"excellent", "good", "fair", "poor"}[i%4]

		executions[i] = execution
	}

	return executions
}

// TestRunner bootstraps the Ginkgo test suite
func TestUadvancedUanalyticsUunit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UadvancedUanalyticsUunit Suite")
}
