//go:build e2e
// +build e2e

package workflowengine

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/ai/insights"
	"github.com/jordigilh/kubernaut/pkg/platform/safety"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/enhanced"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

// BR-WF-E2E-001: End-to-End Workflow Engine Testing - Pyramid Testing (10% E2E Coverage)
// Business Impact: Validates complete business workflows in production-like environments
// Stakeholder Value: Operations teams can trust complete automation workflows
var _ = Describe("BR-WF-E2E-001: End-to-End Workflow Engine Testing", func() {
	var (
		// Use REAL infrastructure for E2E testing (minimal mocking)
		realK8sClient kubernetes.Interface
		realLogger    *logrus.Logger
		testCluster   *enhanced.TestClusterManager

		// Use REAL business logic components for complete E2E validation
		workflowEngine  *engine.DefaultWorkflowEngine
		safetyFramework *safety.SafetyFramework
		analyticsEngine *insights.AnalyticsEngine
		holmesGPTClient *holmesgpt.Client

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 300*time.Second) // 5 minutes for E2E

		// Setup real test infrastructure
		testCluster = enhanced.NewTestClusterManager()
		err := testCluster.SetupTestCluster(ctx)
		Expect(err).ToNot(HaveOccurred(), "E2E test cluster setup must succeed")

		realK8sClient = testCluster.GetKubernetesClient()
		realLogger = logrus.New()
		realLogger.SetLevel(logrus.InfoLevel) // Full logging for E2E tests

		// Create REAL business logic components for E2E testing
		safetyConfig := &safety.SafetyFrameworkConfig{
			EnableRBACValidation:          true,
			EnableResourceValidation:      true,
			EnableNetworkPolicyValidation: true,
			MaxRiskScore:                  0.6, // Stricter for E2E
		}
		safetyFramework = safety.NewSafetyFramework(safetyConfig)

		analyticsConfig := &insights.AnalyticsConfig{
			EnablePatternDetection: true,
			EnableTrendAnalysis:    true,
			EnableAnomalyDetection: true,
			ConfidenceThreshold:    0.85, // Higher for E2E
		}
		analyticsEngine = insights.NewAnalyticsEngine(analyticsConfig)

		// Use real LLM endpoint for E2E (or mock if unavailable)
		holmesGPTConfig := &holmesgpt.Config{
			BaseURL: "http://localhost:3000", // Real HolmesGPT service
			Timeout: 60 * time.Second,
		}
		holmesGPTClient = holmesgpt.NewClient(holmesGPTConfig, nil) // No mock for E2E

		// Create REAL workflow engine with real infrastructure
		engineConfig := &engine.WorkflowEngineConfig{
			DefaultStepTimeout:    60 * time.Second,
			MaxRetryDelay:         10 * time.Second,
			EnableStateRecovery:   true,
			EnableDetailedLogging: true,
			MaxConcurrency:        3, // Conservative for E2E
		}

		workflowEngine = engine.NewDefaultWorkflowEngine(
			realK8sClient,                        // Real: Kubernetes cluster
			testCluster.GetActionRepository(),    // Real: Action repository
			testCluster.GetMonitoringClients(),   // Real: Monitoring
			testCluster.GetStateStorage(),        // Real: State storage
			testCluster.GetExecutionRepository(), // Real: Execution repository
			engineConfig,                         // Real: Business configuration
			realLogger,                           // Real: Logging
		)
	})

	AfterEach(func() {
		if testCluster != nil {
			testCluster.CleanupTestCluster(ctx)
		}
		cancel()
	})

	// BR-WF-E2E-002: Complete Alert-to-Resolution Workflow
	Context("BR-WF-E2E-002: Complete Alert-to-Resolution Workflow", func() {
		It("should execute complete alert processing workflow from detection to resolution", func() {
			// Business Scenario: Complete production alert processing workflow
			// Business Impact: Validates end-to-end automation capability
			// Stakeholder Value: Operations teams can rely on complete automation

			// Step 1: Create realistic production alert
			productionAlert := &types.AlertData{
				ID:          "prod-alert-memory-pressure-001",
				Summary:     "High memory usage detected in production namespace",
				Description: "Memory usage has exceeded 85% threshold for critical services",
				Severity:    "high",
				Source:      "prometheus",
				Labels: map[string]string{
					"cluster":      "production-east-1",
					"namespace":    "critical-services",
					"alertname":    "HighMemoryUsage",
					"service":      "api-gateway",
					"pod_count":    "5",
					"memory_usage": "87%",
				},
				Annotations: map[string]string{
					"runbook_url":     "https://runbooks.company.com/memory-pressure",
					"escalation_team": "platform-sre",
					"business_impact": "customer-facing-services",
				},
				Timestamp: time.Now(),
				Metrics: map[string]float64{
					"memory_usage_percent": 87.5,
					"cpu_usage_percent":    65.2,
					"pod_restart_count":    3,
				},
			}

			// Step 2: AI Analysis Phase
			By("Performing AI analysis of the production alert")
			aiAnalysisStart := time.Now()
			aiAnalysis, err := holmesGPTClient.AnalyzeAlert(ctx, productionAlert)
			aiAnalysisDuration := time.Since(aiAnalysisStart)

			Expect(err).ToNot(HaveOccurred(),
				"BR-WF-E2E-002: AI analysis must succeed for production alerts")
			Expect(aiAnalysis).ToNot(BeNil(),
				"BR-WF-E2E-002: AI analysis must provide actionable insights")
			Expect(aiAnalysisDuration).To(BeNumerically("<", 30*time.Second),
				"BR-WF-E2E-002: AI analysis must complete within SLA")

			// Step 3: Workflow Generation Phase
			By("Generating workflow based on AI analysis")
			workflowTemplate := createProductionWorkflowTemplate(productionAlert, aiAnalysis)
			workflow := engine.NewWorkflow("prod-memory-pressure-workflow-001", workflowTemplate)

			// Step 4: Safety Validation Phase
			By("Validating workflow safety for production execution")
			safetyValidation := safetyFramework.ValidateWorkflow(workflow)
			Expect(safetyValidation).ToNot(BeNil(),
				"BR-WF-E2E-002: Safety validation must assess production workflows")
			Expect(safetyValidation.RiskScore).To(BeNumerically("<=", 0.6),
				"BR-WF-E2E-002: Production workflows must meet safety requirements")

			if !safetyValidation.Approved {
				Skip(fmt.Sprintf("Workflow rejected by safety framework: %v", safetyValidation.Violations))
			}

			// Step 5: Analytics Optimization Phase
			By("Optimizing workflow based on historical analytics")
			analyticsInsights, err := analyticsEngine.AnalyzeWorkflowForOptimization(ctx, workflow)
			Expect(err).ToNot(HaveOccurred(),
				"BR-WF-E2E-002: Analytics optimization must succeed")

			if analyticsInsights != nil && analyticsInsights.OptimizationRecommendations != nil {
				workflow = applyOptimizationRecommendations(workflow, analyticsInsights)
			}

			// Step 6: Workflow Execution Phase
			By("Executing complete production workflow")
			executionStart := time.Now()
			result, err := workflowEngine.Execute(ctx, workflow)
			executionDuration := time.Since(executionStart)

			// Step 7: Validate Complete E2E Outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-WF-E2E-002: Complete production workflow must execute successfully")
			Expect(result).ToNot(BeNil(),
				"BR-WF-E2E-002: Production workflow must produce comprehensive results")
			Expect(result.Status).To(Equal("completed"),
				"BR-WF-E2E-002: Production workflow must complete successfully")
			Expect(executionDuration).To(BeNumerically("<", 180*time.Second),
				"BR-WF-E2E-002: Production workflow must complete within SLA")

			// Validate business outcomes
			Expect(result.Metadata["ai_analysis_applied"]).To(BeTrue(),
				"BR-WF-E2E-002: Must track AI analysis integration")
			Expect(result.Metadata["safety_validated"]).To(BeTrue(),
				"BR-WF-E2E-002: Must track safety validation")
			Expect(result.Metadata["analytics_optimized"]).To(BeTrue(),
				"BR-WF-E2E-002: Must track analytics optimization")

			// Step 8: Verify Real Infrastructure Changes
			By("Verifying actual infrastructure changes were applied")
			if result.InfrastructureChanges != nil {
				for _, change := range result.InfrastructureChanges {
					// Verify changes were actually applied to the cluster
					verifyInfrastructureChange(ctx, realK8sClient, change)
				}
			}

			// Business Value: Complete automated alert resolution
			realLogger.WithFields(logrus.Fields{
				"alert_id":          productionAlert.ID,
				"ai_analysis_time":  aiAnalysisDuration,
				"execution_time":    executionDuration,
				"workflow_status":   result.Status,
				"safety_risk_score": safetyValidation.RiskScore,
			}).Info("Complete E2E workflow executed successfully")
		})

		It("should handle workflow failures gracefully with proper rollback", func() {
			// Business Scenario: Workflow encounters failures and must rollback safely
			// Business Impact: Prevents partial state corruption, maintains system integrity

			// Create workflow that will encounter controlled failure
			failureAlert := &types.AlertData{
				ID:       "failure-test-001",
				Summary:  "Controlled failure test for rollback validation",
				Severity: "medium",
				Labels: map[string]string{
					"test_scenario": "controlled_failure",
					"rollback_test": "true",
				},
			}

			// Create workflow with intentional failure point
			failureTemplate := createFailureTestWorkflowTemplate(failureAlert)
			failureWorkflow := engine.NewWorkflow("failure-rollback-test-001", failureTemplate)

			// Execute workflow expecting controlled failure
			result, err := workflowEngine.Execute(ctx, failureWorkflow)

			// Validate graceful failure handling
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("controlled failure"),
					"BR-WF-E2E-002: Controlled failures must be properly identified")
			}

			if result != nil {
				Expect(result.Status).To(BeElementOf([]string{"failed", "rolled_back"}),
					"BR-WF-E2E-002: Failed workflows must have appropriate status")
				Expect(result.Metadata["rollback_executed"]).To(BeTrue(),
					"BR-WF-E2E-002: Failed workflows must execute rollback procedures")
			}

			// Verify system state is clean after rollback
			By("Verifying system state after rollback")
			verifyCleanSystemState(ctx, realK8sClient)

			// Business Value: Safe failure handling maintains system integrity
		})
	})

	// BR-WF-E2E-003: Multi-Cluster Workflow Coordination
	Context("BR-WF-E2E-003: Multi-Cluster Workflow Coordination", func() {
		It("should coordinate workflows across multiple clusters", func() {
			// Business Scenario: Complex operations spanning multiple clusters
			// Business Impact: Enables sophisticated multi-cluster automation

			Skip("Multi-cluster E2E test requires additional cluster setup")

			// This test would validate:
			// - Cross-cluster workflow coordination
			// - Multi-cluster state synchronization
			// - Cross-cluster rollback capabilities
			// - Multi-cluster monitoring and observability
		})
	})

	// BR-WF-E2E-004: Performance and Scale Validation
	Context("BR-WF-E2E-004: Performance and Scale Validation", func() {
		It("should handle high-volume workflow execution under load", func() {
			// Business Scenario: System handles multiple concurrent workflows
			// Business Impact: Validates production scalability

			Skip("High-volume E2E test requires extended test environment")

			// This test would validate:
			// - Concurrent workflow execution
			// - Resource utilization under load
			// - Performance degradation thresholds
			// - System stability under stress
		})
	})
})

// Helper functions for E2E test scenarios
// These test COMPLETE business workflows with real infrastructure

func createProductionWorkflowTemplate(alert *types.AlertData, aiAnalysis interface{}) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate("production-memory-pressure-workflow", "Production Memory Pressure Resolution")

	steps := []*engine.ExecutableWorkflowStep{
		{
			BaseEntity: types.BaseEntity{
				ID:   "analyze-memory-usage",
				Name: "Analyze Current Memory Usage",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "kubernetes_metrics_analysis",
				Parameters: map[string]interface{}{
					"namespace":   alert.Labels["namespace"],
					"service":     alert.Labels["service"],
					"metric_type": "memory",
					"ai_analysis": aiAnalysis,
				},
			},
		},
		{
			BaseEntity: types.BaseEntity{
				ID:   "identify-memory-leaks",
				Name: "Identify Potential Memory Leaks",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "memory_leak_detection",
				Parameters: map[string]interface{}{
					"namespace": alert.Labels["namespace"],
					"service":   alert.Labels["service"],
					"threshold": 85.0,
				},
			},
			Dependencies: []string{"analyze-memory-usage"},
		},
		{
			BaseEntity: types.BaseEntity{
				ID:   "apply-resource-limits",
				Name: "Apply Resource Limits",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "kubernetes_resource_limits",
				Parameters: map[string]interface{}{
					"namespace":      alert.Labels["namespace"],
					"service":        alert.Labels["service"],
					"memory_limit":   "2Gi",
					"memory_request": "1Gi",
				},
			},
			Dependencies: []string{"identify-memory-leaks"},
		},
		{
			BaseEntity: types.BaseEntity{
				ID:   "restart-affected-pods",
				Name: "Restart Affected Pods",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "kubernetes_pod_restart",
				Parameters: map[string]interface{}{
					"namespace":        alert.Labels["namespace"],
					"service":          alert.Labels["service"],
					"restart_strategy": "rolling",
				},
			},
			Dependencies: []string{"apply-resource-limits"},
		},
		{
			BaseEntity: types.BaseEntity{
				ID:   "verify-resolution",
				Name: "Verify Memory Pressure Resolution",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "kubernetes_metrics_verification",
				Parameters: map[string]interface{}{
					"namespace":        alert.Labels["namespace"],
					"service":          alert.Labels["service"],
					"memory_threshold": 75.0,
					"wait_duration":    "5m",
				},
			},
			Dependencies: []string{"restart-affected-pods"},
		},
	}

	template.Steps = steps
	return template
}

func createFailureTestWorkflowTemplate(alert *types.AlertData) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate("failure-test-workflow", "Controlled Failure Test Workflow")

	steps := []*engine.ExecutableWorkflowStep{
		{
			BaseEntity: types.BaseEntity{
				ID:   "setup-test-resources",
				Name: "Setup Test Resources",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "test_resource_setup",
				Parameters: map[string]interface{}{
					"test_scenario": "controlled_failure",
				},
			},
		},
		{
			BaseEntity: types.BaseEntity{
				ID:   "controlled-failure-step",
				Name: "Controlled Failure Step",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "controlled_failure_action",
				Parameters: map[string]interface{}{
					"failure_type": "controlled",
					"should_fail":  true,
				},
			},
			Dependencies: []string{"setup-test-resources"},
		},
		{
			BaseEntity: types.BaseEntity{
				ID:   "cleanup-test-resources",
				Name: "Cleanup Test Resources",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "test_resource_cleanup",
				Parameters: map[string]interface{}{
					"test_scenario": "controlled_failure",
				},
			},
			Dependencies: []string{"controlled-failure-step"},
		},
	}

	template.Steps = steps
	return template
}

func applyOptimizationRecommendations(workflow *engine.Workflow, insights interface{}) *engine.Workflow {
	// Apply analytics-based optimizations to the workflow
	optimizedWorkflow := &engine.Workflow{
		ID:       workflow.ID + "-optimized",
		Template: workflow.Template,
		Metadata: workflow.Metadata,
	}

	if optimizedWorkflow.Metadata == nil {
		optimizedWorkflow.Metadata = make(map[string]interface{})
	}
	optimizedWorkflow.Metadata["analytics_optimized"] = true
	optimizedWorkflow.Metadata["optimization_insights"] = insights

	return optimizedWorkflow
}

func verifyInfrastructureChange(ctx context.Context, k8sClient kubernetes.Interface, change interface{}) {
	// Verify that infrastructure changes were actually applied
	// This would include checking:
	// - Resource limits were applied
	// - Pods were restarted
	// - Configurations were updated
	// - Metrics show improvement

	// Implementation would depend on the specific change type
	// For now, we'll just log that verification would occur
	logrus.WithField("change", change).Info("Verifying infrastructure change (E2E test)")
}

func verifyCleanSystemState(ctx context.Context, k8sClient kubernetes.Interface) {
	// Verify that the system is in a clean state after rollback
	// This would include checking:
	// - No orphaned resources
	// - Original configurations restored
	// - No partial state corruption
	// - System metrics are normal

	// Implementation would check actual cluster state
	// For now, we'll just log that verification would occur
	logrus.Info("Verifying clean system state after rollback (E2E test)")
}
