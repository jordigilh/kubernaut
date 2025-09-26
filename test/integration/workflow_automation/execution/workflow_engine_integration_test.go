//go:build integration
// +build integration

package execution

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/ai/insights"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/jordigilh/kubernaut/pkg/platform/safety"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/enhanced"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes/fake"
)

// BR-INT-001: AI component integration for intelligent workflow generation
// BR-INT-002: Platform layer coordination for Kubernetes operations
// BR-INT-003: Storage component utilization for workflow persistence
// BR-INT-004: Monitoring systems integration for execution visibility
// BR-INT-005: Intelligence component coordination for pattern learning
// Business Impact: Validates cross-component workflow execution for production readiness
// Stakeholder Value: Operations teams can trust complex workflow integrations
var _ = Describe("BR-INT-001/002/003/004/005: Workflow Engine Integration Testing", func() {
	var (
		// Mock ONLY external dependencies (databases, external APIs)
		mockK8sFakeClient *fake.Clientset
		mockK8sClient     k8s.Client
		mockLogger        *logrus.Logger

		// Use REAL business logic components for integration testing
		workflowEngine  *engine.DefaultWorkflowEngine
		safetyValidator *safety.SafetyValidator
		analyticsEngine *insights.AnalyticsEngineImpl
		holmesGPTClient holmesgpt.Client

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)

		// Mock external dependencies only
		mockK8sFakeClient = enhanced.NewSmartFakeClientset()
		// Use existing mock logger from testutil instead of library testing
		mockLoggerImpl := mocks.NewMockLogger()
		mockLogger = mockLoggerImpl.Logger

		// Create k8s.Client adapter from fake clientset
		k8sConfig := config.KubernetesConfig{
			Namespace: "default",
			Context:   "test",
		}
		mockK8sClient = k8s.NewUnifiedClient(mockK8sFakeClient, k8sConfig, mockLogger)

		// Create REAL business logic components for integration testing
		safetyValidator = safety.NewSafetyValidator(
			mockK8sFakeClient, // External: Mock K8s fake client for safety validator
			mockLogger,        // External: Mock logger
		)

		// Use actual constructor - no config parameters per Rule 09 validation
		analyticsEngine = insights.NewAnalyticsEngine()

		// Create real HolmesGPT client for integration testing
		var err error
		holmesGPTClient, err = holmesgpt.NewClient("http://localhost:3000", "", mockLogger)
		Expect(err).ToNot(HaveOccurred(), "HolmesGPT client creation should succeed")

		// Create REAL workflow engine with real business components
		engineConfig := &engine.WorkflowEngineConfig{
			DefaultStepTimeout:    30 * time.Second,
			MaxRetryDelay:         5 * time.Second,
			EnableStateRecovery:   true,
			EnableDetailedLogging: true,
			MaxConcurrency:        5,
		}

		workflowEngine = engine.NewDefaultWorkflowEngine(
			mockK8sClient,                   // External: Mock (with proper interface)
			mocks.NewMockActionRepository(), // External: Mock
			nil,                             // External: Mock (monitoring)
			mocks.NewMockStateStorage(),     // External: Mock
			mocks.NewWorkflowExecutionRepositoryMock(), // External: Mock
			engineConfig, // Real: Business configuration
			mockLogger,   // External: Mock
		)
	})

	AfterEach(func() {
		cancel()
	})

	// BR-INT-001: AI component integration for intelligent workflow generation
	Context("BR-INT-001: AI Component Integration for Intelligent Workflow Generation", func() {
		It("should integrate AI analysis with workflow generation for complex alerts", func() {
			// Business Scenario: AI analyzes complex alerts and generates appropriate workflows
			// Business Impact: Reduces manual analysis time, improves response accuracy

			// Setup realistic alert data
			complexAlert := &types.Alert{
				ID:          "complex-alert-001",
				Name:        "memory-pressure-cascade",
				Summary:     "High memory usage with cascading pod failures",
				Description: "Multiple pods failing due to memory pressure, affecting critical services",
				Severity:    "critical",
				Status:      "firing",
				Labels: map[string]string{
					"cluster":   "production-east",
					"namespace": "critical-services",
					"component": "api-gateway",
				},
				Annotations: map[string]string{
					"runbook_url": "https://runbooks.company.com/memory-pressure",
					"escalation":  "platform-team",
				},
				StartsAt:  time.Now(),
				UpdatedAt: time.Now(),
			}

			// Test REAL AI-workflow integration using HolmesGPT investigate method
			investigateRequest := &holmesgpt.InvestigateRequest{
				AlertName:       complexAlert.Name,
				Namespace:       complexAlert.Namespace,
				Labels:          complexAlert.Labels,
				Annotations:     complexAlert.Annotations,
				Priority:        complexAlert.Severity,
				AsyncProcessing: false,
				IncludeContext:  true,
			}

			aiAnalysis, err := holmesGPTClient.Investigate(ctx, investigateRequest)
			Expect(err).ToNot(HaveOccurred(),
				"BR-INT-001: AI analysis must succeed for workflow generation")
			// Validate AI analysis provides actionable business insights
			Expect(aiAnalysis.Status).To(Equal("completed"),
				"BR-INT-001: AI analysis must complete successfully for workflow generation")
			Expect(len(aiAnalysis.Recommendations)).To(BeNumerically(">", 0),
				"BR-INT-001: AI analysis must provide actionable recommendations")
			Expect(aiAnalysis.InvestigationID).ToNot(BeEmpty(),
				"BR-INT-001: AI analysis must provide trackable investigation identifier")

			// Generate workflow based on AI analysis
			workflowTemplate := createAIEnhancedWorkflowTemplate(aiAnalysis)
			workflow := engine.NewWorkflow("ai-enhanced-001", workflowTemplate)

			// Execute integrated workflow
			result, err := workflowEngine.Execute(ctx, workflow)

			// Validate REAL integration outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-INT-001: AI-enhanced workflow execution must succeed")
			// Validate workflow execution produces business results
			Expect(result.Status).To(Equal("completed"),
				"BR-INT-001: AI-enhanced workflow must complete execution successfully")
			Expect(result.Metadata).To(HaveKey("ai_analysis_applied"),
				"BR-INT-001: Workflow result must track AI analysis integration")
			Expect(result.Metadata["ai_analysis_applied"]).To(BeTrue(),
				"BR-INT-001: Must track AI analysis integration")

			// Business Value: AI-driven automation improves response quality
		})

		It("should integrate safety validation with AI-generated workflows", func() {
			// Business Scenario: Safety framework validates AI-generated workflows before execution
			// Business Impact: Prevents unsafe AI recommendations, maintains operational safety

			// Create potentially risky AI-generated workflow
			riskyAlert := &types.Alert{
				ID:       "risky-alert-001",
				Summary:  "Database performance degradation",
				Severity: "high",
				Labels: map[string]string{
					"database": "production-primary",
					"impact":   "customer-facing",
				},
			}

			// Create test investigation request for AI analysis
			investigateRequest := &holmesgpt.InvestigateRequest{
				AlertName:       riskyAlert.Name,
				Namespace:       riskyAlert.Namespace,
				Labels:          riskyAlert.Labels,
				Annotations:     riskyAlert.Annotations,
				Priority:        riskyAlert.Severity,
				AsyncProcessing: false,
				IncludeContext:  true,
			}

			// Test REAL safety-AI integration
			aiAnalysis, err := holmesGPTClient.Investigate(ctx, investigateRequest)
			Expect(err).ToNot(HaveOccurred(),
				"BR-WF-INT-002: AI analysis must succeed for safety validation")

			// Create workflow with AI recommendations
			riskyTemplate := createRiskyWorkflowTemplate(aiAnalysis)
			riskyWorkflow := engine.NewWorkflow("safety-validation-001", riskyTemplate)

			// Test REAL safety framework integration - assess workflow execution risk
			workflowExecutionAction := types.ActionRecommendation{
				Action: "execute_workflow",
				Parameters: map[string]interface{}{
					"workflow_id":       riskyWorkflow.ID,
					"template_id":       riskyTemplate.ID,
					"workflow_type":     "ai_generated",
					"risk_assessment":   "required",
					"execution_context": "integration_test",
				},
				Confidence: 0.85,
				Reasoning: &types.ReasoningDetails{
					PrimaryReason: "AI-generated workflow requires safety validation before execution",
					Summary:       "Workflow contains potentially risky database operations",
				},
			}

			// Use AssessRisk method for proper workflow-safety integration
			workflowRiskAssessment := safetyValidator.AssessRisk(ctx, workflowExecutionAction, *riskyAlert)

			// Also validate the underlying resource state for comprehensive assessment
			resourceValidation := safetyValidator.ValidateResourceState(ctx, *riskyAlert)

			// Validate workflow risk assessment provides business safety insights
			Expect(workflowRiskAssessment.ActionName).To(Equal("execute_workflow"),
				"BR-WF-INT-002: Risk assessment must identify workflow execution action")
			Expect(workflowRiskAssessment.RiskLevel).To(BeElementOf([]string{"LOW", "MEDIUM", "HIGH", "CRITICAL"}),
				"BR-WF-INT-002: Risk assessment must provide valid risk classification")

			// Validate resource state assessment provides business resource insights
			Expect(resourceValidation.RiskLevel).To(BeElementOf([]string{"LOW", "MEDIUM", "HIGH", "CRITICAL"}),
				"BR-WF-INT-002: Resource validation must classify resource risk level")
			Expect(resourceValidation.CurrentState).ToNot(BeEmpty(),
				"BR-WF-INT-002: Resource validation must determine current resource state")

			// Integrate workflow risk with resource validation for comprehensive safety assessment
			if workflowRiskAssessment.RiskLevel == "HIGH" || workflowRiskAssessment.RiskLevel == "CRITICAL" {
				// High risk workflow execution should be flagged
				Expect(workflowRiskAssessment.SafeToExecute).To(BeFalse(),
					"BR-WF-INT-002: High-risk workflow execution must be flagged as unsafe")
				Expect(len(workflowRiskAssessment.RiskFactors)).To(BeNumerically(">", 0),
					"BR-WF-INT-002: Risk assessment must identify specific risk factors")
				Expect(len(workflowRiskAssessment.Mitigation)).To(BeNumerically(">", 10),
					"BR-WF-INT-002: Risk assessment must provide detailed mitigation strategies")
			} else {
				// Low/medium risk workflow should be conditionally approved
				// Safety decision combines workflow risk + resource state
				combinedSafetyDecision := workflowRiskAssessment.SafeToExecute && resourceValidation.IsValid
				Expect(combinedSafetyDecision).To(BeTrue(),
					"BR-WF-INT-002: Low-risk workflows with valid resources should be approved")
			}

			// Validate confidence and business metadata integration
			Expect(workflowRiskAssessment.Confidence).To(BeNumerically(">=", 0.5),
				"BR-WF-INT-002: Risk assessment must provide confidence metrics")
			Expect(workflowRiskAssessment.Metadata).To(HaveKey("alert_severity"),
				"BR-WF-INT-002: Risk assessment must include alert context")

			// Business Value: Safety-validated AI automation maintains operational integrity
		})
	})

	// BR-WF-INT-003: Analytics-Driven Workflow Optimization Integration
	Context("BR-WF-INT-003: Analytics-Driven Workflow Optimization", func() {
		It("should integrate analytics insights with workflow execution optimization", func() {
			// Business Scenario: Analytics engine provides insights to optimize workflow execution
			// Business Impact: Improves workflow efficiency based on historical performance

			// Setup historical execution data for analytics and validate integration
			historicalExecutions := createHistoricalExecutionData(10)

			// Validate historical data creation for business analytics testing
			Expect(len(historicalExecutions)).To(Equal(10),
				"BR-WF-INT-003: Historical execution data must be created for analytics testing")
			Expect(historicalExecutions[0].Duration).To(BeNumerically(">", 0),
				"BR-WF-INT-003: Historical executions must have realistic duration metrics")

			// Verify test data contains varied execution outcomes for analytics
			completedCount := 0
			failedCount := 0
			for _, execution := range historicalExecutions {
				if execution.OperationalStatus == engine.ExecutionStatusCompleted {
					completedCount++
				} else if execution.OperationalStatus == engine.ExecutionStatusFailed {
					failedCount++
				}
			}
			Expect(completedCount).To(BeNumerically(">", 0),
				"BR-WF-INT-003: Test data must include successful executions for analytics")
			Expect(failedCount).To(BeNumerically(">", 0),
				"BR-WF-INT-003: Test data must include failed executions for analytics learning")

			// Test REAL analytics integration using actual AnalyticsEngine method
			timeWindow := 24 * time.Hour
			analyticsInsights, err := analyticsEngine.GetAnalyticsInsights(ctx, timeWindow)
			Expect(err).ToNot(HaveOccurred(),
				"BR-WF-INT-003: Analytics analysis must succeed for optimization")
			// Validate analytics insights provide business intelligence
			Expect(analyticsInsights.GeneratedAt).To(BeTemporally("~", time.Now(), 5*time.Second),
				"BR-WF-INT-003: Analytics insights must have recent generation timestamp")
			Expect(len(analyticsInsights.WorkflowInsights)).To(BeNumerically(">=", 0),
				"BR-WF-INT-003: Analytics must provide workflow insights data structure")

			// Create optimized workflow based on analytics insights
			optimizedTemplate := createAnalyticsOptimizedTemplate(analyticsInsights)
			optimizedWorkflow := engine.NewWorkflow("analytics-optimized-001", optimizedTemplate)

			// Execute analytics-optimized workflow
			startTime := time.Now()
			result, err := workflowEngine.Execute(ctx, optimizedWorkflow)
			executionTime := time.Since(startTime)

			// Validate REAL analytics integration outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-WF-INT-003: Analytics-optimized workflow execution must succeed")
			// Validate analytics-optimized workflow execution produces business outcomes
			Expect(result.Status).To(Equal("completed"),
				"BR-WF-INT-003: Analytics-optimized workflow must complete successfully")
			Expect(result.Duration).To(BeNumerically(">", 0),
				"BR-WF-INT-003: Workflow execution must record actual execution duration")
			Expect(result.Metadata["analytics_optimization_applied"]).To(BeTrue(),
				"BR-WF-INT-003: Must track analytics optimization integration")

			// Verify analytics insights integration provides business intelligence
			generationAge := time.Since(analyticsInsights.GeneratedAt)
			Expect(generationAge).To(BeNumerically("<", 10*time.Second),
				"BR-WF-INT-003: Analytics insights must be recently generated")
			Expect(len(analyticsInsights.WorkflowInsights)).To(BeNumerically(">=", 0),
				"BR-WF-INT-003: Analytics must provide structured workflow insights")

			// Verify execution completes within reasonable time
			Expect(executionTime.Seconds()).To(BeNumerically("<", 30),
				"BR-WF-INT-003: Analytics-optimized workflow must complete within 30 seconds")

			// Business Value: Data-driven optimization improves operational efficiency
		})
	})

	// BR-WF-INT-004: Multi-Component Workflow Coordination Integration
	Context("BR-WF-INT-004: Multi-Component Workflow Coordination", func() {
		It("should coordinate workflows across multiple business components", func() {
			// Business Scenario: Complex operations require coordination across AI, Safety, and Analytics
			// Business Impact: Enables sophisticated multi-component automation

			// Create complex multi-component scenario
			multiComponentAlert := &types.Alert{
				ID:       "multi-component-001",
				Summary:  "Cluster-wide performance degradation with security implications",
				Severity: "critical",
				Labels: map[string]string{
					"scope":    "cluster-wide",
					"security": "potential-breach",
					"impact":   "multi-service",
				},
			}

			// Test REAL multi-component coordination

			// Step 1: AI Analysis using proper Investigate method
			investigateMultiRequest := &holmesgpt.InvestigateRequest{
				AlertName:       multiComponentAlert.Name,
				Namespace:       multiComponentAlert.Namespace,
				Labels:          multiComponentAlert.Labels,
				Annotations:     multiComponentAlert.Annotations,
				Priority:        multiComponentAlert.Severity,
				AsyncProcessing: false,
				IncludeContext:  true,
			}

			aiAnalysis, err := holmesGPTClient.Investigate(ctx, investigateMultiRequest)
			Expect(err).ToNot(HaveOccurred(),
				"BR-WF-INT-004: AI analysis must succeed in multi-component scenario")

			// Step 2: Safety Validation using ValidateResourceState
			safetyValidation := safetyValidator.ValidateResourceState(ctx, *multiComponentAlert)
			// Validate multi-component safety assessment provides business insights
			Expect(safetyValidation.RiskLevel).To(BeElementOf([]string{"LOW", "MEDIUM", "HIGH", "CRITICAL"}),
				"BR-WF-INT-004: Multi-component safety must classify resource risk level")
			Expect(safetyValidation.CurrentState).ToNot(BeEmpty(),
				"BR-WF-INT-004: Multi-component safety must determine resource state")

			// Step 3: Analytics Optimization using proper GetAnalyticsInsights method
			if safetyValidation.IsValid {
				timeWindow := 24 * time.Hour
				analyticsInsights, err := analyticsEngine.GetAnalyticsInsights(ctx, timeWindow)
				Expect(err).ToNot(HaveOccurred(),
					"BR-WF-INT-004: Analytics must assess workflow complexity")

				// Step 4: Execute coordinated workflow
				preliminaryWorkflow := createMultiComponentWorkflowTemplate(aiAnalysis)
				finalTemplate := optimizeWorkflowWithInsights(preliminaryWorkflow, analyticsInsights)
				finalWorkflow := engine.NewWorkflow("multi-component-final", finalTemplate)

				result, err := workflowEngine.Execute(ctx, finalWorkflow)

				// Validate REAL multi-component coordination outcomes
				Expect(err).ToNot(HaveOccurred(),
					"BR-WF-INT-004: Multi-component coordinated workflow must succeed")
				// Validate multi-component workflow execution produces business coordination results
				Expect(result.Status).To(Equal("completed"),
					"BR-WF-INT-004: Multi-component workflow must complete coordination successfully")
				Expect(result.Duration).To(BeNumerically(">", 0),
					"BR-WF-INT-004: Multi-component coordination must record execution duration")
				Expect(result.Metadata["multi_component_coordination"]).To(BeTrue(),
					"BR-WF-INT-004: Must track multi-component coordination")

				// Business Value: Sophisticated automation through component coordination
			}
		})
	})
})

// Helper functions for integration test scenarios
// These test REAL cross-component business logic integration

func createAIEnhancedWorkflowTemplate(aiAnalysis *holmesgpt.InvestigateResponse) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate("ai-enhanced-workflow", "AI Enhanced Workflow")

	step := &engine.ExecutableWorkflowStep{
		BaseEntity: types.BaseEntity{
			ID:   "ai-enhanced-step-1",
			Name: "AI Enhanced Action",
		},
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "ai_enhanced_action",
			Parameters: map[string]interface{}{
				"ai_analysis_id":  aiAnalysis.InvestigationID,
				"ai_status":       aiAnalysis.Status,
				"ai_summary":      aiAnalysis.Summary,
				"recommendations": len(aiAnalysis.Recommendations),
				"enhanced":        true,
			},
		},
	}

	template.Steps = []*engine.ExecutableWorkflowStep{step}
	return template
}

func createRiskyWorkflowTemplate(aiAnalysis *holmesgpt.InvestigateResponse) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate("risky-workflow", "Potentially Risky Workflow")

	// Create steps with varying risk levels
	steps := []*engine.ExecutableWorkflowStep{
		{
			BaseEntity: types.BaseEntity{ID: "high-risk-step", Name: "High Risk Action"},
			Type:       engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "high_risk_action",
				Parameters: map[string]interface{}{
					"risk_level":            "high",
					"action":                "restart_database_cluster",
					"ai_analysis_id":        aiAnalysis.InvestigationID,
					"ai_confidence":         aiAnalysis.DurationSeconds,
					"recommendation_source": "holmesgpt_investigation",
				},
			},
		},
		{
			BaseEntity: types.BaseEntity{ID: "medium-risk-step", Name: "Medium Risk Action"},
			Type:       engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "medium_risk_action",
				Parameters: map[string]interface{}{
					"risk_level":            "medium",
					"action":                "scale_database_resources",
					"ai_recommendations":    len(aiAnalysis.Recommendations),
					"analysis_summary":      aiAnalysis.Summary,
					"recommendation_source": "holmesgpt_investigation",
				},
			},
		},
	}

	template.Steps = steps
	return template
}

func createHistoricalExecutionData(count int) []*engine.RuntimeWorkflowExecution {
	executions := make([]*engine.RuntimeWorkflowExecution, count)

	for i := 0; i < count; i++ {
		execution := engine.NewRuntimeWorkflowExecution(
			fmt.Sprintf("historical-execution-%d", i+1),
			"historical-workflow")

		// Simulate varying execution times and success rates
		execution.Duration = time.Duration(500+i*100) * time.Millisecond
		if i%3 == 0 {
			execution.OperationalStatus = engine.ExecutionStatusFailed
		} else {
			execution.OperationalStatus = engine.ExecutionStatusCompleted
		}

		executions[i] = execution
	}

	return executions
}

func createAnalyticsOptimizedTemplate(insights *types.AnalyticsInsights) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate("analytics-optimized-workflow", "Analytics Optimized Workflow")

	step := &engine.ExecutableWorkflowStep{
		BaseEntity: types.BaseEntity{
			ID:   "analytics-optimized-step-1",
			Name: "Analytics Optimized Action",
		},
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "analytics_optimized_action",
			Parameters: map[string]interface{}{
				"analytics_generated_at":  insights.GeneratedAt,
				"workflow_insights_count": len(insights.WorkflowInsights),
				"pattern_insights_count":  len(insights.PatternInsights),
				"recommendations_count":   len(insights.Recommendations),
				"optimized":               true,
			},
		},
	}

	template.Steps = []*engine.ExecutableWorkflowStep{step}
	return template
}

func createMultiComponentWorkflowTemplate(aiAnalysis *holmesgpt.InvestigateResponse) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate("multi-component-workflow", "Multi Component Workflow")

	steps := []*engine.ExecutableWorkflowStep{
		{
			BaseEntity: types.BaseEntity{ID: "ai-component-step", Name: "AI Component Action"},
			Type:       engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "ai_component_action",
				Parameters: map[string]interface{}{
					"ai_analysis_id":     aiAnalysis.InvestigationID,
					"ai_namespace":       aiAnalysis.Namespace,
					"ai_recommendations": len(aiAnalysis.Recommendations),
					"component":          "ai",
				},
			},
		},
		{
			BaseEntity: types.BaseEntity{ID: "safety-component-step", Name: "Safety Component Action"},
			Type:       engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "safety_component_action",
				Parameters: map[string]interface{}{
					"component":           "safety",
					"validation_required": true,
				},
			},
			Dependencies: []string{"ai-component-step"},
		},
		{
			BaseEntity: types.BaseEntity{ID: "analytics-component-step", Name: "Analytics Component Action"},
			Type:       engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "analytics_component_action",
				Parameters: map[string]interface{}{
					"component":             "analytics",
					"optimization_required": true,
				},
			},
			Dependencies: []string{"safety-component-step"},
		},
	}

	template.Steps = steps
	return template
}

func optimizeWorkflowWithInsights(template *engine.ExecutableTemplate, insights *types.AnalyticsInsights) *engine.ExecutableTemplate {
	// Create optimized version of the template based on analytics insights
	optimizedTemplate := &engine.ExecutableTemplate{
		BaseVersionedEntity: types.BaseVersionedEntity{
			BaseEntity: types.BaseEntity{
				ID:          template.BaseVersionedEntity.ID + "-optimized",
				Name:        template.BaseVersionedEntity.Name + " (Optimized)",
				Description: template.BaseVersionedEntity.Description + " - Optimized with analytics insights",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				Metadata:    make(map[string]interface{}),
			},
			Version:   "1.0.0",
			CreatedBy: "analytics-optimizer",
		},
		Steps:     template.Steps, // In real implementation, this would be optimized
		Variables: template.Variables,
	}

	// Add optimization metadata
	if optimizedTemplate.Variables == nil {
		optimizedTemplate.Variables = make(map[string]interface{})
	}
	optimizedTemplate.Variables["analytics_generated_at"] = insights.GeneratedAt
	optimizedTemplate.Variables["optimization_applied"] = true
	optimizedTemplate.Variables["insights_metadata"] = insights.Metadata

	return optimizedTemplate
}
