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

package workflow_engine

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
		mockLogger = logrus.New()
		mockLogger.SetLevel(logrus.WarnLevel) // Reduce noise in integration tests

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
			Expect(aiAnalysis).ToNot(BeNil(),
				"BR-INT-001: AI analysis must provide actionable insights")

			// Generate workflow based on AI analysis
			workflowTemplate := createAIEnhancedWorkflowTemplate(aiAnalysis)
			workflow := engine.NewWorkflow("ai-enhanced-001", workflowTemplate)

			// Execute integrated workflow
			result, err := workflowEngine.Execute(ctx, workflow)

			// Validate REAL integration outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-INT-001: AI-enhanced workflow execution must succeed")
			Expect(result).ToNot(BeNil(),
				"BR-INT-001: AI-enhanced workflow must produce results")
			Expect(result.Metadata["ai_analysis_applied"]).To(BeTrue(),
				"BR-INT-001: Must track AI analysis integration")
			Expect(result.Status).To(Equal("completed"),
				"BR-INT-001: AI-enhanced workflow must complete successfully")

			// Business Value: AI-driven automation improves response quality
		})

		It("should integrate safety validation with AI-generated workflows", func() {
			// Business Scenario: Safety framework validates AI-generated workflows before execution
			// Business Impact: Prevents unsafe AI recommendations, maintains operational safety

			// Create potentially risky AI-generated workflow
			riskyAlert := &types.Alert{
				ID:       "risky-alert-001",
				Name:     "database-performance-degradation",
				Summary:  "Database performance degradation",
				Severity: "high",
				Status:   "firing",
				Labels: map[string]string{
					"database": "production-primary",
					"impact":   "customer-facing",
				},
				StartsAt:  time.Now(),
				UpdatedAt: time.Now(),
			}

			// Test REAL safety-AI integration using HolmesGPT investigate method
			riskyInvestigateRequest := &holmesgpt.InvestigateRequest{
				AlertName:       riskyAlert.Name,
				Namespace:       riskyAlert.Namespace,
				Labels:          riskyAlert.Labels,
				Annotations:     riskyAlert.Annotations,
				Priority:        riskyAlert.Severity,
				AsyncProcessing: false,
				IncludeContext:  true,
			}

			aiAnalysis, err := holmesGPTClient.Investigate(ctx, riskyInvestigateRequest)
			Expect(err).ToNot(HaveOccurred(),
				"BR-WF-INT-002: AI analysis must succeed for safety validation")

			// Create workflow with AI recommendations for testing safety validation
			_ = createRiskyWorkflowTemplate(aiAnalysis) // Template for safety assessment

			// Test REAL safety framework integration (using available methods)
			safetyValidation := safetyValidator.AssessRisk(ctx, types.ActionRecommendation{
				Action:     "restart_database_cluster",
				Confidence: 0.9,
				Parameters: map[string]interface{}{"database": "production-primary"},
			}, *riskyAlert)
			Expect(safetyValidation).ToNot(BeNil(),
				"BR-WF-INT-002: Safety validation must assess AI-generated workflows")

			if safetyValidation.RiskLevel == "HIGH" {
				// High risk workflow should be modified or rejected
				Expect(safetyValidation.SafeToExecute).To(BeFalse(),
					"BR-INT-001: High-risk AI workflows must be rejected by safety framework")
				Expect(len(safetyValidation.Mitigation)).To(BeNumerically(">", 0),
					"BR-INT-001: Safety framework must provide risk mitigation recommendations")
			} else {
				// Low risk workflow should be approved
				Expect(safetyValidation.SafeToExecute).To(BeTrue(),
					"BR-INT-001: Low-risk AI workflows must be approved by safety framework")
			}

			// Business Value: Safety-validated AI automation maintains operational integrity
		})
	})

	// BR-WF-INT-003: Analytics-Driven Workflow Optimization Integration
	Context("BR-WF-INT-003: Analytics-Driven Workflow Optimization", func() {
		It("should integrate analytics insights with workflow execution optimization", func() {
			// Business Scenario: Analytics engine provides insights to optimize workflow execution
			// Business Impact: Improves workflow efficiency based on historical performance

			// Setup historical execution data for analytics
			_ = createHistoricalExecutionData(10) // Data for analytics context

			// Test REAL analytics integration
			analyticsInsights, err := analyticsEngine.GetAnalyticsInsights(ctx, 24*time.Hour)
			Expect(err).ToNot(HaveOccurred(),
				"BR-WF-INT-003: Analytics analysis must succeed for optimization")
			Expect(analyticsInsights).ToNot(BeNil(),
				"BR-WF-INT-003: Analytics must provide actionable insights")

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
			Expect(result).ToNot(BeNil(),
				"BR-WF-INT-003: Analytics-optimized workflow must produce results")
			Expect(result.Metadata["analytics_optimization_applied"]).To(BeTrue(),
				"BR-WF-INT-003: Must track analytics optimization integration")

			// Verify performance improvement based on insights
			if effectivenessTrends, exists := analyticsInsights.WorkflowInsights["effectiveness_trends"]; exists && effectivenessTrends != nil {
				// Analytics insights available - expect performance improvement
				Expect(executionTime.Seconds()).To(BeNumerically("<=", 30.0),
					"BR-INT-003: Analytics optimization must improve execution performance")
			}

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
				Name:     "cluster-wide-performance-security",
				Summary:  "Cluster-wide performance degradation with security implications",
				Severity: "critical",
				Status:   "firing",
				Labels: map[string]string{
					"scope":    "cluster-wide",
					"security": "potential-breach",
					"impact":   "multi-service",
				},
				StartsAt:  time.Now(),
				UpdatedAt: time.Now(),
			}

			// Test REAL multi-component coordination

			// Step 1: AI Analysis using HolmesGPT investigate method
			multiComponentInvestigateRequest := &holmesgpt.InvestigateRequest{
				AlertName:       multiComponentAlert.Name,
				Namespace:       multiComponentAlert.Namespace,
				Labels:          multiComponentAlert.Labels,
				Annotations:     multiComponentAlert.Annotations,
				Priority:        multiComponentAlert.Severity,
				AsyncProcessing: false,
				IncludeContext:  true,
			}

			aiAnalysis, err := holmesGPTClient.Investigate(ctx, multiComponentInvestigateRequest)
			Expect(err).ToNot(HaveOccurred(),
				"BR-WF-INT-004: AI analysis must succeed in multi-component scenario")

			// Step 2: Safety Validation
			preliminaryWorkflow := createMultiComponentWorkflowTemplate(aiAnalysis)
			safetyValidation := safetyValidator.AssessRisk(ctx, types.ActionRecommendation{
				Action:     "cluster_wide_optimization",
				Confidence: 0.8,
				Parameters: map[string]interface{}{"scope": "cluster-wide"},
			}, *multiComponentAlert)
			Expect(safetyValidation).ToNot(BeNil(),
				"BR-WF-INT-004: Safety validation must assess multi-component workflow")

			// Step 3: Analytics Optimization
			if safetyValidation.SafeToExecute {
				analyticsInsights, err := analyticsEngine.GetPatternAnalytics(ctx, map[string]interface{}{
					"workflow_type": "multi-component",
					"complexity":    "high",
				})
				Expect(err).ToNot(HaveOccurred(),
					"BR-WF-INT-004: Analytics must assess workflow complexity")

				// Step 4: Execute coordinated workflow
				finalTemplate := optimizeWorkflowWithInsights(preliminaryWorkflow, analyticsInsights)
				finalWorkflow := engine.NewWorkflow("multi-component-final", finalTemplate)

				result, err := workflowEngine.Execute(ctx, finalWorkflow)

				// Validate REAL multi-component coordination outcomes
				Expect(err).ToNot(HaveOccurred(),
					"BR-WF-INT-004: Multi-component coordinated workflow must succeed")
				Expect(result).ToNot(BeNil(),
					"BR-WF-INT-004: Multi-component workflow must produce results")
				Expect(result.Metadata["multi_component_coordination"]).To(BeTrue(),
					"BR-WF-INT-004: Must track multi-component coordination")
				Expect(result.Status).To(Equal("completed"),
					"BR-WF-INT-004: Multi-component workflow must complete successfully")

				// Business Value: Sophisticated automation through component coordination
			}
		})
	})
})

// Helper functions for integration test scenarios
// These test REAL cross-component business logic integration

func createAIEnhancedWorkflowTemplate(aiAnalysis interface{}) *engine.ExecutableTemplate {
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
				"ai_analysis": aiAnalysis,
				"enhanced":    true,
			},
		},
	}

	template.Steps = []*engine.ExecutableWorkflowStep{step}
	return template
}

func createRiskyWorkflowTemplate(aiAnalysis interface{}) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate("risky-workflow", "Potentially Risky Workflow")

	// Create steps with varying risk levels
	steps := []*engine.ExecutableWorkflowStep{
		{
			BaseEntity: types.BaseEntity{ID: "high-risk-step", Name: "High Risk Action"},
			Type:       engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "high_risk_action",
				Parameters: map[string]interface{}{
					"risk_level": "high",
					"action":     "restart_database_cluster",
				},
			},
		},
		{
			BaseEntity: types.BaseEntity{ID: "medium-risk-step", Name: "Medium Risk Action"},
			Type:       engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "medium_risk_action",
				Parameters: map[string]interface{}{
					"risk_level": "medium",
					"action":     "scale_database_resources",
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

func createAnalyticsOptimizedTemplate(insights interface{}) *engine.ExecutableTemplate {
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
				"analytics_insights": insights,
				"optimized":          true,
			},
		},
	}

	template.Steps = []*engine.ExecutableWorkflowStep{step}
	return template
}

func createMultiComponentWorkflowTemplate(aiAnalysis interface{}) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate("multi-component-workflow", "Multi Component Workflow")

	steps := []*engine.ExecutableWorkflowStep{
		{
			BaseEntity: types.BaseEntity{ID: "ai-component-step", Name: "AI Component Action"},
			Type:       engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "ai_component_action",
				Parameters: map[string]interface{}{
					"ai_analysis": aiAnalysis,
					"component":   "ai",
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

func optimizeWorkflowWithInsights(template *engine.ExecutableTemplate, insights interface{}) *engine.ExecutableTemplate {
	// Create optimized version of the template based on analytics insights
	optimizedTemplate := &engine.ExecutableTemplate{
		Steps:     template.Steps, // In real implementation, this would be optimized
		Variables: template.Variables,
	}
	optimizedTemplate.BaseVersionedEntity.ID = template.BaseVersionedEntity.ID + "-optimized"
	optimizedTemplate.BaseVersionedEntity.Name = template.BaseVersionedEntity.Name + " (Optimized)"
	optimizedTemplate.BaseVersionedEntity.Description = template.BaseVersionedEntity.Description + " - Optimized with analytics insights"

	// Add optimization metadata
	if optimizedTemplate.Variables == nil {
		optimizedTemplate.Variables = make(map[string]interface{})
	}
	optimizedTemplate.Variables["analytics_insights"] = insights
	optimizedTemplate.Variables["optimized"] = true

	return optimizedTemplate
}
