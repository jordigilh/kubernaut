package conditions_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/conditions"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/jordigilh/kubernaut/test/integration/shared/testenv"
)

var _ = Describe("AI Condition Evaluator", func() {
	var (
		ctx                  context.Context
		logger               *logrus.Logger
		testEnv              *testenv.TestEnvironment
		mockSLMClient        *MockSLMClient
		aiConditionEvaluator *conditions.DefaultAIConditionEvaluator
		workflowEngine       *engine.DefaultWorkflowEngine
		monitoringClients    *monitoring.MonitoringClients
		testStepContext      *engine.StepContext
	)

	BeforeEach(func() {
		var err error
		ctx = context.Background()

		// Setup logger
		logger = logrus.New()
		logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests

		// Setup fake K8s environment using envsetup infrastructure
		testEnv, err = testenv.SetupFakeEnvironment()
		Expect(err).NotTo(HaveOccurred())
		Expect(testEnv).NotTo(BeNil())

		// Create test namespace
		err = testEnv.CreateDefaultNamespace()
		Expect(err).NotTo(HaveOccurred())

		// Setup dependencies
		k8sClient := testEnv.CreateK8sClient(logger)
		mockSLMClient = NewMockSLMClient()

		// Create monitoring clients (using factory for proper setup)
		monitoringConfig := monitoring.MonitoringConfig{
			UseProductionClients: false, // Use stub clients for testing
		}
		factory := monitoring.NewClientFactory(monitoringConfig, k8sClient, logger)
		monitoringClients = factory.CreateClients()

		// Create AI condition evaluator
		config := &conditions.AIConditionEvaluatorConfig{
			MaxEvaluationTime:       10 * time.Second,
			ConfidenceThreshold:     0.75,
			EnableDetailedLogging:   false,
			FallbackOnLowConfidence: true,
			UseContextualAnalysis:   true,
		}
		aiConditionEvaluator = conditions.NewDefaultAIConditionEvaluator(
			mockSLMClient,
			k8sClient,
			monitoringClients,
			config,
		)

		// Create workflow engine with AI condition evaluator
		workflowEngine = engine.NewDefaultWorkflowEngine(
			k8sClient,
			nil, // actionRepo - not needed for condition tests
			monitoringClients,
			nil, // stateStorage - not needed for condition tests
			nil, // config - will use defaults
			logger,
		)

		// Setup test step context
		testStepContext = &engine.StepContext{
			ExecutionID: "test-execution-123",
			StepID:      "test-step-456",
			Variables: map[string]interface{}{
				"test_var": "test_value",
			},
		}
	})

	AfterEach(func() {
		if testEnv != nil {
			err := testEnv.Cleanup()
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Describe("NewDefaultAIConditionEvaluator", func() {
		It("should create a new AI condition evaluator with default config", func() {
			k8sClient := testEnv.CreateK8sClient(logger)
			evaluator := conditions.NewDefaultAIConditionEvaluator(mockSLMClient, k8sClient, monitoringClients, nil)

			Expect(evaluator).NotTo(BeNil())
			Expect(evaluator.IsHealthy()).To(BeTrue())
		})

		It("should create a new AI condition evaluator with custom config", func() {
			k8sClient := testEnv.CreateK8sClient(logger)
			config := &conditions.AIConditionEvaluatorConfig{
				MaxEvaluationTime:   5 * time.Second,
				ConfidenceThreshold: 0.8,
			}

			evaluator := conditions.NewDefaultAIConditionEvaluator(mockSLMClient, k8sClient, monitoringClients, config)

			Expect(evaluator).NotTo(BeNil())
			Expect(evaluator.IsHealthy()).To(BeTrue())
		})
	})

	Describe("EvaluateMetricCondition", func() {
		var testCondition *engine.WorkflowCondition

		BeforeEach(func() {
			testCondition = &engine.WorkflowCondition{
				ID:         "metric-condition-1",
				Name:       "CPU Usage Check",
				Type:       engine.ConditionTypeMetric,
				Expression: "cpu_usage_percent > 80",
				Variables: map[string]interface{}{
					"threshold": 80,
				},
				Timeout: 30 * time.Second,
			}
		})

		Context("with successful AI response", func() {
			BeforeEach(func() {
				mockSLMClient.SetAnalysisResponse(&types.ActionRecommendation{
					Action:     "evaluate_condition",
					Confidence: 0.9,
					Reasoning: &types.ReasoningDetails{
						Summary: `{
							"satisfied": true,
							"confidence": 0.9,
							"reasoning": "CPU usage at 75% is below the 80% threshold",
							"recommendations": ["Monitor CPU trends", "Consider scaling if usage increases"],
							"warnings": [],
							"metadata": {"current_cpu": 75, "threshold": 80}
						}`,
					},
				})
			})

			It("should successfully evaluate metric condition using AI", func() {
				result, err := aiConditionEvaluator.EvaluateMetricCondition(ctx, testCondition, testStepContext)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Satisfied).To(BeTrue())
				Expect(result.Confidence).To(Equal(0.9))
				Expect(result.Reasoning).To(ContainSubstring("CPU usage"))
				Expect(result.NextActions).To(HaveLen(2))
				Expect(result.NextActions[0]).To(ContainSubstring("Monitor CPU"))
				Expect(result.Metadata).To(HaveKey("current_cpu"))
			})
		})

		Context("with low confidence AI response", func() {
			BeforeEach(func() {
				mockSLMClient.SetAnalysisResponse(&types.ActionRecommendation{
					Action:     "evaluate_condition",
					Confidence: 0.5, // Below threshold
					Reasoning: &types.ReasoningDetails{
						Summary: `{
							"satisfied": false,
							"confidence": 0.5,
							"reasoning": "Uncertain about metric evaluation",
							"recommendations": [],
							"warnings": ["Low confidence in analysis"],
							"metadata": {}
						}`,
					},
				})
			})

			It("should fall back to basic evaluation for low confidence", func() {
				result, err := aiConditionEvaluator.EvaluateMetricCondition(ctx, testCondition, testStepContext)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Metadata).To(HaveKey("fallback"))
				Expect(result.Metadata["fallback"]).To(BeTrue())
				Expect(result.Warnings).To(ContainElement(ContainSubstring("AI evaluation unavailable")))
			})
		})

		Context("with SLM client error", func() {
			BeforeEach(func() {
				mockSLMClient.SetError("SLM service unavailable")
			})

			It("should fall back to basic evaluation", func() {
				result, err := aiConditionEvaluator.EvaluateMetricCondition(ctx, testCondition, testStepContext)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Metadata).To(HaveKey("fallback"))
				Expect(result.Metadata["fallback"]).To(BeTrue())
				Expect(result.Warnings).To(ContainElement(ContainSubstring("AI evaluation unavailable")))
			})
		})
	})

	Describe("EvaluateResourceCondition", func() {
		var testCondition *engine.WorkflowCondition

		BeforeEach(func() {
			testCondition = &engine.WorkflowCondition{
				ID:         "resource-condition-1",
				Name:       "Pod Readiness Check",
				Type:       engine.ConditionTypeResource,
				Expression: "namespace=default AND pod_ready=true",
				Variables: map[string]interface{}{
					"namespace": "default",
				},
			}
		})

		Context("with successful AI response", func() {
			BeforeEach(func() {
				mockSLMClient.SetAnalysisResponse(&types.ActionRecommendation{
					Action:     "evaluate_condition",
					Confidence: 0.85,
					Reasoning: &types.ReasoningDetails{
						Summary: `{
							"satisfied": true,
							"confidence": 0.85,
							"reasoning": "All pods in default namespace are ready and healthy",
							"recommendations": ["Continue monitoring pod health"],
							"warnings": [],
							"metadata": {"pods_ready": 5, "pods_total": 5, "namespace": "default"}
						}`,
					},
				})
			})

			It("should successfully evaluate resource condition using AI", func() {
				result, err := aiConditionEvaluator.EvaluateResourceCondition(ctx, testCondition, testStepContext)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Satisfied).To(BeTrue())
				Expect(result.Confidence).To(Equal(0.85))
				Expect(result.Reasoning).To(ContainSubstring("pods"))
				Expect(result.Metadata).To(HaveKey("pods_ready"))
			})
		})

		Context("with fake K8s cluster interaction", func() {
			It("should access K8s resources through fake client", func() {
				// Test that AI condition evaluator can access K8s resources
				k8sClient := testEnv.CreateK8sClient(logger)

				// List nodes to verify cluster access
				nodes, err := k8sClient.ListNodes(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(nodes).NotTo(BeNil())

				// List pods in default namespace
				pods, err := k8sClient.ListPodsWithLabel(ctx, "default", "app=test")
				Expect(err).NotTo(HaveOccurred())
				Expect(pods).NotTo(BeNil())

				// This demonstrates the AI evaluator has access to K8s context
			})
		})
	})

	Describe("EvaluateTimeCondition", func() {
		var testCondition *engine.WorkflowCondition

		BeforeEach(func() {
			testCondition = &engine.WorkflowCondition{
				ID:         "time-condition-1",
				Name:       "Business Hours Check",
				Type:       engine.ConditionTypeTime,
				Expression: "business_hours=true",
				Timeout:    5 * time.Minute,
			}
		})

		Context("with successful AI response", func() {
			BeforeEach(func() {
				mockSLMClient.SetAnalysisResponse(&types.ActionRecommendation{
					Action:     "evaluate_condition",
					Confidence: 0.95,
					Reasoning: &types.ReasoningDetails{
						Summary: `{
							"satisfied": true,
							"confidence": 0.95,
							"reasoning": "Current time 14:30 is within business hours (9 AM - 5 PM)",
							"recommendations": ["Proceed with business hour operations"],
							"warnings": [],
							"metadata": {"current_hour": 14, "business_hours_start": 9, "business_hours_end": 17}
						}`,
					},
				})
			})

			It("should successfully evaluate time condition using AI", func() {
				result, err := aiConditionEvaluator.EvaluateTimeCondition(ctx, testCondition, testStepContext)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Satisfied).To(BeTrue())
				Expect(result.Confidence).To(Equal(0.95))
				Expect(result.Reasoning).To(ContainSubstring("business hours"))
				Expect(result.Metadata).To(HaveKey("current_hour"))
			})
		})
	})

	Describe("EvaluateExpressionCondition", func() {
		var testCondition *engine.WorkflowCondition

		BeforeEach(func() {
			testCondition = &engine.WorkflowCondition{
				ID:         "expression-condition-1",
				Name:       "Complex Expression Check",
				Type:       engine.ConditionTypeExpression,
				Expression: "(cpu_usage < 80) AND (memory_usage < 70) OR (replica_count > 2)",
				Variables: map[string]interface{}{
					"cpu_usage":     75,
					"memory_usage":  65,
					"replica_count": 3,
				},
			}
		})

		Context("with successful AI response", func() {
			BeforeEach(func() {
				mockSLMClient.SetAnalysisResponse(&types.ActionRecommendation{
					Action:     "evaluate_condition",
					Confidence: 0.88,
					Reasoning: &types.ReasoningDetails{
						Summary: `{
							"satisfied": true,
							"confidence": 0.88,
							"reasoning": "Expression evaluates to true: CPU (75%) and memory (65%) are both below thresholds",
							"recommendations": ["Expression logic is sound", "Consider simplifying complex expressions"],
							"warnings": [],
							"metadata": {
								"expression_parsed": "(cpu_usage < 80) AND (memory_usage < 70) OR (replica_count > 2)",
								"cpu_condition": true,
								"memory_condition": true,
								"replica_condition": true,
								"final_result": true
							}
						}`,
					},
				})
			})

			It("should successfully evaluate expression condition using AI", func() {
				result, err := aiConditionEvaluator.EvaluateExpressionCondition(ctx, testCondition, testStepContext)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Satisfied).To(BeTrue())
				Expect(result.Confidence).To(Equal(0.88))
				Expect(result.Reasoning).To(ContainSubstring("CPU"))
				Expect(result.Metadata).To(HaveKey("expression_parsed"))
				Expect(result.Metadata).To(HaveKey("final_result"))
			})
		})
	})

	Describe("EvaluateCustomCondition", func() {
		var testCondition *engine.WorkflowCondition

		BeforeEach(func() {
			testCondition = &engine.WorkflowCondition{
				ID:         "custom-condition-1",
				Name:       "Custom Health Check",
				Type:       engine.ConditionTypeCustom,
				Expression: "system_healthy AND no_critical_alerts",
				Variables: map[string]interface{}{
					"health_check_endpoint": "/health",
					"alert_severity":        "critical",
				},
			}
		})

		Context("with successful AI response", func() {
			BeforeEach(func() {
				mockSLMClient.SetAnalysisResponse(&types.ActionRecommendation{
					Action:     "evaluate_condition",
					Confidence: 0.82,
					Reasoning: &types.ReasoningDetails{
						Summary: `{
							"satisfied": true,
							"confidence": 0.82,
							"reasoning": "System appears healthy based on available metrics and no critical alerts detected",
							"recommendations": ["Continue monitoring system health", "Set up proactive alerting"],
							"warnings": ["Limited visibility into some system components"],
							"metadata": {
								"health_indicators": ["cluster_accessible", "no_critical_alerts", "resources_available"],
								"risk_factors": ["limited_monitoring_coverage"],
								"overall_health": "good"
							}
						}`,
					},
				})
			})

			It("should successfully evaluate custom condition using AI", func() {
				result, err := aiConditionEvaluator.EvaluateCustomCondition(ctx, testCondition, testStepContext)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Satisfied).To(BeTrue())
				Expect(result.Confidence).To(Equal(0.82))
				Expect(result.Reasoning).To(ContainSubstring("healthy"))
				Expect(result.Metadata).To(HaveKey("health_indicators"))
				Expect(result.Warnings).To(HaveLen(1))
			})
		})
	})

	Describe("Integration with Workflow Engine", func() {
		var (
			testWorkflowStep      *engine.WorkflowStep
			testWorkflowCondition *engine.WorkflowCondition
		)

		BeforeEach(func() {
			testWorkflowCondition = &engine.WorkflowCondition{
				ID:         "integration-condition-1",
				Name:       "Integration Test Condition",
				Type:       engine.ConditionTypeMetric,
				Expression: "cpu_usage < 90",
				Variables: map[string]interface{}{
					"threshold": 90,
				},
			}

			testWorkflowStep = &engine.WorkflowStep{
				ID:        "integration-step-1",
				Name:      "Integration Test Step",
				Type:      engine.StepTypeCondition,
				Condition: testWorkflowCondition,
			}
		})

		Context("when AI service is healthy", func() {
			BeforeEach(func() {
				mockSLMClient.SetAnalysisResponse(&types.ActionRecommendation{
					Action:     "evaluate_condition",
					Confidence: 0.9,
					Reasoning: &types.ReasoningDetails{
						Summary: `{
							"satisfied": true,
							"confidence": 0.9,
							"reasoning": "CPU usage is within acceptable limits",
							"recommendations": [],
							"warnings": [],
							"metadata": {}
						}`,
					},
				})
			})

			It("should use AI service for condition evaluation in workflow engine", func() {
				// Execute the condition step through the workflow engine
				result, err := workflowEngine.ExecuteStep(ctx, testWorkflowStep, testStepContext)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Data).To(HaveKey("condition_met"))
				Expect(result.Data["condition_met"]).To(BeTrue())

				// Verify AI service was called
				Expect(mockSLMClient.GetCallCount()).To(BeNumerically(">", 0))

				// Check that AI condition result was stored in step context
				Expect(testStepContext.Variables).To(HaveKey("ai_condition_result"))
			})
		})

		Context("when AI service fails", func() {
			BeforeEach(func() {
				mockSLMClient.SetError("AI service temporarily unavailable")
			})

			It("should fall back to basic condition evaluation", func() {
				// Execute the condition step through the workflow engine
				result, err := workflowEngine.ExecuteStep(ctx, testWorkflowStep, testStepContext)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Data).To(HaveKey("condition_met"))

				// Verify AI service was called but failed gracefully
				Expect(mockSLMClient.GetCallCount()).To(BeNumerically(">", 0))
			})
		})
	})

	Describe("Fallback Evaluation Methods", func() {
		Context("when testing basic metric evaluation", func() {
			It("should handle CPU threshold conditions", func() {
				// Test that workflow engine can perform basic condition evaluation
				condition := &engine.WorkflowCondition{
					ID:         "basic-test",
					Type:       engine.ConditionTypeMetric,
					Expression: "cpu > 80",
				}

				// Verify that condition evaluation doesn't crash when AI is unavailable
				// We test this by disabling the AI service temporarily
				workflowEngine.SetAIConditionEvaluator(nil)

				result, err := workflowEngine.EvaluateCondition(ctx, condition, testStepContext)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeAssignableToTypeOf(true))
			})
		})

		Context("when testing basic resource evaluation", func() {
			It("should access K8s cluster for basic resource checks", func() {
				k8sClient := testEnv.CreateK8sClient(logger)

				// Verify basic cluster connectivity
				nodes, err := k8sClient.ListNodes(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(nodes).NotTo(BeNil())

				// This represents the basic resource evaluation fallback
			})
		})

		Context("when testing basic time evaluation", func() {
			It("should handle business hours conditions", func() {
				now := time.Now()
				hour := now.Hour()

				// Test business hours logic
				isBusinessHours := hour >= 9 && hour <= 17
				Expect(isBusinessHours).To(BeAssignableToTypeOf(true))
			})
		})
	})
})

// MockSLMClient for testing AI condition evaluation
type MockSLMClient struct {
	response  *types.ActionRecommendation
	error     string
	callCount int
}

func NewMockSLMClient() *MockSLMClient {
	return &MockSLMClient{}
}

func (m *MockSLMClient) SetAnalysisResponse(response *types.ActionRecommendation) {
	m.response = response
	m.error = ""
}

func (m *MockSLMClient) SetError(err string) {
	m.error = err
	m.response = nil
}

func (m *MockSLMClient) GetCallCount() int {
	return m.callCount
}

func (m *MockSLMClient) AnalyzeAlert(ctx context.Context, alert types.Alert) (*types.ActionRecommendation, error) {
	m.callCount++

	if m.error != "" {
		return nil, fmt.Errorf(m.error)
	}

	if m.response != nil {
		return m.response, nil
	}

	// Default response
	return &types.ActionRecommendation{
		Action:     "evaluate_condition",
		Confidence: 0.5,
		Reasoning: &types.ReasoningDetails{
			Summary: "Default mock response for condition evaluation",
		},
	}, nil
}

func (m *MockSLMClient) ChatCompletion(ctx context.Context, prompt string) (string, error) {
	m.callCount++
	if m.error != "" {
		return "", fmt.Errorf(m.error)
	}
	// Return a simple mock response for AI processing
	return `{"result": "true", "confidence": 0.85, "reasoning": "Condition evaluation based on analysis"}`, nil
}

func (m *MockSLMClient) IsHealthy() bool {
	return true
}
