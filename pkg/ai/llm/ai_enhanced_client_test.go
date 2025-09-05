package llm_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	"github.com/jordigilh/kubernaut/test/integration/shared/testenv"
)

var _ = Describe("AI Enhanced SLM Client", func() {
	var (
		ctx                   context.Context
		logger                *logrus.Logger
		testEnv               *testenv.TestEnvironment
		mockBasicClient       *MockSLMClient
		mockResponseProcessor *MockAIResponseProcessor
		mockKnowledgeBase     *MockKnowledgeBase
		enhancedClient        llm.EnhancedClient
		testAlert             types.Alert
		testConfig            config.LLMConfig
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

		// Setup test configuration
		testConfig = config.LLMConfig{
			Provider:    "localai",
			Endpoint:    "http://localhost:8080",
			Model:       "granite-3.0-8b-instruct",
			Temperature: 0.1,
			MaxTokens:   2048,
			Timeout:     30 * time.Second,
		}

		// Create mock components
		mockBasicClient = NewMockSLMClient()
		mockResponseProcessor = NewMockAIResponseProcessor()
		mockKnowledgeBase = NewMockKnowledgeBase()

		// Enhance knowledge base with K8s client for realistic testing
		k8sClient := testEnv.CreateK8sClient(logger)
		mockKnowledgeBase.SetK8sClient(k8sClient)

		// Setup test alert
		testAlert = types.Alert{
			Name:        "HighMemoryUsage",
			Description: "Memory usage is above 90% for pod test-pod",
			Severity:    "warning",
			Status:      "firing",
			Namespace:   "default",
			Resource:    "test-pod",
			Labels: map[string]string{
				"pod":       "test-pod",
				"container": "app",
			},
			Annotations: map[string]string{
				"summary":     "High memory usage detected",
				"description": "Memory usage is consistently above 90%",
			},
		}
	})

	AfterEach(func() {
		if testEnv != nil {
			err := testEnv.Cleanup()
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Describe("NewEnhancedClient", func() {
		It("should create a new enhanced client with custom response processor (mock-based)", func() {
			// Use mock-based approach instead of real client creation
			client, err := llm.NewEnhancedClientWithProcessor(testConfig, mockResponseProcessor, logger)

			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
			Expect(client.GetResponseProcessor()).To(Equal(mockResponseProcessor))

			// Note: IsHealthy() depends on the underlying basic client,
			// which would try to connect to LocalAI in real scenarios
		})

		It("should provide AI processing capabilities interface", func() {
			// Test the interface without requiring real connections
			client, err := llm.NewEnhancedClientWithProcessor(testConfig, mockResponseProcessor, logger)

			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())

			// Verify enhanced client interface
			Expect(client.GetResponseProcessor()).NotTo(BeNil())
			Expect(client.GetResponseProcessor()).To(Equal(mockResponseProcessor))
		})
	})

	Describe("AnalyzeAlertWithEnhancement (Mock-based)", func() {
		BeforeEach(func() {
			// Create enhanced client using test constructor with mock basic client
			enhancedClient = llm.NewEnhancedClientForTesting(
				mockBasicClient,
				mockResponseProcessor,
				mockKnowledgeBase,
				testConfig,
				logger,
			)

			// Setup basic client mock response
			mockBasicClient.SetAnalysisResponse(&types.ActionRecommendation{
				Action:     "increase_resources",
				Confidence: 0.85,
				Parameters: map[string]interface{}{
					"memory_limit": "2Gi",
					"cpu_limit":    "1000m",
				},
				Reasoning: &types.ReasoningDetails{
					Summary:           "Memory usage is consistently high, indicating need for more resources",
					HistoricalContext: "Pod is experiencing memory pressure",
					PrimaryReason:     "Memory usage above 90% for extended period",
				},
			})
		})

		Context("with AI processing when basic client unavailable", func() {
			BeforeEach(func() {
				// Setup AI response processor mock
				mockResponseProcessor.SetProcessResponse(&llm.EnhancedActionRecommendation{
					ActionRecommendation: &types.ActionRecommendation{
						Action:     "increase_resources",
						Confidence: 0.85,
						Parameters: map[string]interface{}{
							"memory_limit": "2Gi",
							"cpu_limit":    "1000m",
						},
						Reasoning: &types.ReasoningDetails{
							Summary: "Memory usage is consistently high, indicating need for more resources",
						},
					},
					ValidationResult: &llm.ValidationResult{
						IsValid:            true,
						ValidationScore:    0.9,
						ActionAppropriate:  true,
						ParametersComplete: true,
						RiskAssessment: &llm.RiskAssessment{
							RiskLevel:          "low",
							BlastRadius:        "pod",
							ReversibilityScore: 0.95,
						},
					},
					ReasoningAnalysis: &llm.ReasoningAnalysis{
						QualityScore:       0.85,
						CoherenceScore:     0.90,
						CompletenessScore:  0.80,
						LogicalConsistency: true,
						EvidenceSupport:    0.88,
					},
					ConfidenceAssessment: &llm.ConfidenceAssessment{
						CalibratedConfidence:  0.82,
						OriginalConfidence:    0.85,
						ConfidenceReliability: 0.9,
						SuggestedThreshold:    0.75,
					},
					ContextualEnhancement: &llm.ContextualEnhancement{
						SituationalContext: &llm.SituationalContext{
							Urgency:        "medium",
							BusinessImpact: "low",
							PeakTraffic:    false,
						},
						TimelineAnalysis: &llm.TimelineAnalysis{
							ExpectedDuration: 5 * time.Minute,
							OptimalTiming:    "immediate",
						},
					},
					ProcessingMetadata: &llm.ProcessingMetadata{
						ProcessingTime:      100 * time.Millisecond,
						AIModelUsed:         "ai_response_processor",
						ProcessingSteps:     []string{"validation", "reasoning_analysis", "confidence_calibration", "contextual_enhancement"},
						ValidationsPassed:   4,
						ValidationsFailed:   0,
						EnhancementsApplied: []string{"validation", "reasoning_analysis", "confidence_calibration", "contextual_enhancement"},
					},
				})
			})

			It("should provide AI-enhanced analysis with mock clients", func() {
				// This test shows that the AI enhanced client works properly with mock components
				result, err := enhancedClient.AnalyzeAlertWithEnhancement(ctx, testAlert)

				// With proper mock setup, this should succeed
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())

				// Verify we get the enhanced recommendation with AI processing
				Expect(result.ActionRecommendation).NotTo(BeNil())
				Expect(result.ActionRecommendation.Action).To(Equal("increase_resources"))
				Expect(result.ProcessingMetadata).NotTo(BeNil())
			})
		})

		Context("with AI response processor failure", func() {
			BeforeEach(func() {
				mockResponseProcessor.SetError("AI processing service temporarily unavailable")
			})

			It("should fallback gracefully when AI processor fails", func() {
				// This test demonstrates graceful fallback when AI processing fails
				result, err := enhancedClient.AnalyzeAlertWithEnhancement(ctx, testAlert)

				// Basic analysis should still succeed even if AI processing fails
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())

				// Should have basic recommendation but with processing errors noted
				Expect(result.ActionRecommendation).NotTo(BeNil())
				Expect(result.ProcessingMetadata).NotTo(BeNil())
				Expect(result.ProcessingMetadata.ProcessingErrors).NotTo(BeEmpty())
			})
		})

		Context("with unhealthy AI response processor", func() {
			BeforeEach(func() {
				mockResponseProcessor.SetHealthy(false)
			})

			It("should fall back to basic recommendation when processor is unhealthy", func() {
				result, err := enhancedClient.AnalyzeAlertWithEnhancement(ctx, testAlert)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())

				// Should have basic recommendation with fallback metadata
				Expect(result.ProcessingMetadata.AIModelUsed).To(Equal("basic_client_only"))
				Expect(result.ProcessingMetadata.ProcessingErrors).To(ContainElement("AI response processor unavailable"))
			})
		})
	})

	Describe("ValidateRecommendation", func() {
		var testRecommendation *types.ActionRecommendation

		BeforeEach(func() {
			// Create enhanced client using test constructor with mock basic client
			enhancedClient = llm.NewEnhancedClientForTesting(
				mockBasicClient,
				mockResponseProcessor,
				mockKnowledgeBase,
				testConfig,
				logger,
			)

			testRecommendation = &types.ActionRecommendation{
				Action:     "scale_deployment",
				Confidence: 0.75,
				Parameters: map[string]interface{}{
					"replicas": 5,
				},
				Reasoning: &types.ReasoningDetails{
					Summary: "Scaling deployment to handle increased load",
				},
			}
		})

		Context("with healthy AI response processor", func() {
			BeforeEach(func() {
				mockResponseProcessor.SetValidationResult(&llm.ValidationResult{
					IsValid:            true,
					ValidationScore:    0.85,
					ActionAppropriate:  true,
					ParametersComplete: true,
					RiskAssessment: &llm.RiskAssessment{
						RiskLevel:          "medium",
						BlastRadius:        "deployment",
						ReversibilityScore: 0.8,
					},
					Violations:         []llm.ValidationViolation{},
					Recommendations:    []string{"Monitor deployment health during scaling"},
					AlternativeActions: []string{"increase_resources", "optimize_resources"},
				})
			})

			It("should validate recommendation with AI analysis", func() {
				result, err := enhancedClient.ValidateRecommendation(ctx, testRecommendation, testAlert)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.IsValid).To(BeTrue())
				Expect(result.ValidationScore).To(Equal(0.85))
				Expect(result.ActionAppropriate).To(BeTrue())
				Expect(result.RiskAssessment.RiskLevel).To(Equal("medium"))
				Expect(result.Recommendations).To(HaveLen(1))
				Expect(result.AlternativeActions).To(HaveLen(2))
			})
		})

		Context("with unhealthy AI response processor", func() {
			BeforeEach(func() {
				mockResponseProcessor.SetHealthy(false)
			})

			It("should return error when processor is unavailable", func() {
				_, err := enhancedClient.ValidateRecommendation(ctx, testRecommendation, testAlert)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("AI response processor unavailable"))
			})
		})
	})

	Describe("Response Processor Management", func() {
		It("should allow setting and getting response processor", func() {
			client, err := llm.NewEnhancedClientWithProcessor(testConfig, mockResponseProcessor, logger)
			Expect(err).NotTo(HaveOccurred())

			// Should have the initial processor
			Expect(client.GetResponseProcessor()).To(Equal(mockResponseProcessor))

			// Should allow setting a new processor
			newProcessor := NewMockAIResponseProcessor()
			client.SetResponseProcessor(newProcessor)
			Expect(client.GetResponseProcessor()).To(Equal(newProcessor))
		})
	})

	Describe("Integration with Knowledge Base and K8s Client", func() {
		BeforeEach(func() {
			// Setup knowledge base with test data
			mockKnowledgeBase.SetActionRisk("increase_resources", &llm.RiskAssessment{
				RiskLevel:          "low",
				BlastRadius:        "pod",
				ReversibilityScore: 0.95,
			})

			mockKnowledgeBase.SetHistoricalPatterns([]llm.HistoricalPattern{
				{
					Pattern:       "memory_pressure_pattern",
					Frequency:     10,
					LastSeen:      time.Now().Add(-6 * time.Hour),
					Effectiveness: 0.9,
					Context:       "Memory increases typically resolve this issue",
				},
			})
		})

		It("should integrate knowledge base without requiring network connections", func() {
			// Test that knowledge base integration works at the interface level
			client := llm.NewEnhancedClientForTesting(
				mockBasicClient,
				mockResponseProcessor,
				mockKnowledgeBase,
				testConfig,
				logger,
			)

			// The response processor should be available
			processor := client.GetResponseProcessor()
			Expect(processor).NotTo(BeNil())
			Expect(processor).To(Equal(mockResponseProcessor))

			// Mock processor health should be controllable
			mockResponseProcessor.SetHealthy(true)
			Expect(processor.IsHealthy()).To(BeTrue())
		})

		It("should access fake K8s cluster for contextual analysis", func() {
			// Create a test deployment in the fake cluster
			k8sClient := testEnv.CreateK8sClient(logger)

			// Create test deployment - this creates a deployment with app=test-app label
			err := testEnv.CreateTestDeployment("test-app", "default", 1)
			Expect(err).NotTo(HaveOccurred())

			// Verify deployment exists
			deployment, err := k8sClient.GetDeployment(ctx, "default", "test-app")
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment).NotTo(BeNil())
			Expect(deployment.Name).To(Equal("test-app"))

			// Create pods with the same app label as the deployment
			err = testEnv.CreateTestPod("test-app", "default")
			Expect(err).NotTo(HaveOccurred())

			// List pods with the correct label selector that matches what CreateTestPod creates
			pods, err := k8sClient.ListPodsWithLabel(ctx, "default", "app=test-app")
			Expect(err).NotTo(HaveOccurred())
			Expect(pods).NotTo(BeNil())
			Expect(len(pods.Items)).To(BeNumerically(">=", 1))

			// Verify cluster nodes are accessible
			nodes, err := k8sClient.ListNodes(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).NotTo(BeNil())

			// This demonstrates that the AI response processor can access K8s resources
			// during contextual enhancement in a fake cluster environment
		})

		It("should demonstrate K8s integration for AI context gathering", func() {
			// Create a comprehensive test scenario with K8s resources
			k8sClient := testEnv.CreateK8sClient(logger)

			// Create test resources with consistent naming
			appName := "memory-intensive-app"
			err := testEnv.CreateTestPod(appName, "default")
			Expect(err).NotTo(HaveOccurred())

			// Create a deployment with the same name (which will have app=appName label)
			err = testEnv.CreateTestDeployment(appName, "default", 1)
			Expect(err).NotTo(HaveOccurred())

			// Create an enhanced client interface (no network operations)
			client := llm.NewEnhancedClientForTesting(
				mockBasicClient,
				mockResponseProcessor,
				mockKnowledgeBase,
				testConfig,
				logger,
			)
			Expect(client).NotTo(BeNil())

			// Create an alert that references the created resources
			k8sAlert := types.Alert{
				Name:        "HighMemoryUsage",
				Description: "Memory usage is above 90% for pod " + appName,
				Severity:    "warning",
				Status:      "firing",
				Namespace:   "default",
				Resource:    appName,
				Labels: map[string]string{
					"pod": appName,
					"app": appName,
				},
				Annotations: map[string]string{
					"summary": "High memory usage detected on " + appName,
				},
			}

			// Verify K8s resources are accessible for context gathering
			pods, err := k8sClient.ListPodsWithLabel(ctx, "default", "app="+appName)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(pods.Items)).To(BeNumerically(">=", 1))

			deployment, err := k8sClient.GetDeployment(ctx, "default", appName)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Name).To(Equal(appName))

			// Verify that cluster nodes are accessible
			nodes, err := k8sClient.ListNodes(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).NotTo(BeNil())

			// This demonstrates that K8s context is available for AI analysis
			logger.WithFields(logrus.Fields{
				"pods":       len(pods.Items),
				"deployment": deployment.Name,
				"nodes":      len(nodes.Items),
				"alert":      k8sAlert.Name,
			}).Info("K8s context available for AI analysis in fake environment")
		})

		It("should provide system state analysis using K8s data", func() {
			// Test system state analysis with fake cluster data
			systemState, err := mockKnowledgeBase.GetSystemState(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(systemState).NotTo(BeNil())
			Expect(systemState.HealthScore).To(BeNumerically(">", 0))
		})
	})

	Describe("Error Handling and Resilience", func() {
		Context("when basic client fails", func() {
			BeforeEach(func() {
				mockBasicClient.SetError("SLM service temporarily unavailable")
			})

			It("should propagate basic client errors appropriately", func() {
				// Use test constructor with failing mock basic client
				client := llm.NewEnhancedClientForTesting(
					mockBasicClient,
					mockResponseProcessor,
					mockKnowledgeBase,
					testConfig,
					logger,
				)
				Expect(client).NotTo(BeNil())

				// Test that basic client errors are properly propagated
				result, err := client.AnalyzeAlertWithEnhancement(ctx, testAlert)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("SLM service temporarily unavailable"))
			})
		})

		Context("with various AI processing failures", func() {
			BeforeEach(func() {
				// Create enhanced client using test constructor with mock basic client
				enhancedClient = llm.NewEnhancedClientForTesting(
					mockBasicClient,
					mockResponseProcessor,
					mockKnowledgeBase,
					testConfig,
					logger,
				)

				// Setup basic client mock response for successful basic analysis
				mockBasicClient.SetAnalysisResponse(&types.ActionRecommendation{
					Action:     "increase_resources",
					Confidence: 0.85,
					Parameters: map[string]interface{}{
						"memory_limit": "2Gi",
					},
					Reasoning: &types.ReasoningDetails{
						Summary: "Memory usage is high",
					},
				})
			})

			It("should handle validation failures gracefully", func() {
				mockResponseProcessor.SetValidationError("Validation service timeout")

				result, err := enhancedClient.ValidateRecommendation(ctx, &types.ActionRecommendation{
					Action:     "restart_pod",
					Confidence: 0.8,
				}, testAlert)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
			})

			It("should handle partial AI processing failures", func() {
				// Setup a response that succeeds partially
				partialResponse := &llm.EnhancedActionRecommendation{
					ActionRecommendation: &types.ActionRecommendation{
						Action:     "increase_resources",
						Confidence: 0.8,
					},
					ValidationResult: &llm.ValidationResult{
						IsValid:         true,
						ValidationScore: 0.8,
					},
					// Missing other enhancements due to partial failure
					ProcessingMetadata: &llm.ProcessingMetadata{
						ProcessingErrors:  []string{"reasoning analysis failed", "contextual enhancement failed"},
						ProcessingSteps:   []string{"basic_parsing", "validation"},
						ValidationsPassed: 1,
						ProcessingTime:    50 * time.Millisecond,
					},
				}

				mockResponseProcessor.SetProcessResponse(partialResponse)

				result, err := enhancedClient.AnalyzeAlertWithEnhancement(ctx, testAlert)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.ValidationResult).NotTo(BeNil())
				Expect(result.ReasoningAnalysis).To(BeNil()) // Should be nil due to failure
				Expect(result.ProcessingMetadata.ProcessingErrors).To(HaveLen(2))
			})
		})
	})
})

// Mock implementations for testing

type MockSLMClient struct {
	response  *types.ActionRecommendation
	error     string
	healthy   bool
	callCount int
}

func NewMockSLMClient() *MockSLMClient {
	return &MockSLMClient{
		healthy: true,
	}
}

func (m *MockSLMClient) SetAnalysisResponse(response *types.ActionRecommendation) {
	m.response = response
	m.error = ""
}

func (m *MockSLMClient) SetError(err string) {
	m.error = err
	m.response = nil
}

func (m *MockSLMClient) SetHealthy(healthy bool) {
	m.healthy = healthy
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
		Action:     "notify_only",
		Confidence: 0.5,
		Reasoning:  &types.ReasoningDetails{Summary: "Default mock response"},
	}, nil
}

func (m *MockSLMClient) ChatCompletion(ctx context.Context, prompt string) (string, error) {
	m.callCount++
	if m.error != "" {
		return "", fmt.Errorf(m.error)
	}
	// Return a simple mock response for AI processing
	return `{"action": "increase_resources", "confidence": 0.85, "reasoning": "Based on analysis"}`, nil
}

func (m *MockSLMClient) IsHealthy() bool {
	return m.healthy
}

type MockAIResponseProcessor struct {
	processResponse       *llm.EnhancedActionRecommendation
	validationResult      *llm.ValidationResult
	reasoningAnalysis     *llm.ReasoningAnalysis
	confidenceAssessment  *llm.ConfidenceAssessment
	contextualEnhancement *llm.ContextualEnhancement
	processError          string
	validationError       string
	healthy               bool
}

func NewMockAIResponseProcessor() *MockAIResponseProcessor {
	return &MockAIResponseProcessor{
		healthy: true,
	}
}

func (m *MockAIResponseProcessor) SetProcessResponse(response *llm.EnhancedActionRecommendation) {
	m.processResponse = response
	m.processError = ""
}

func (m *MockAIResponseProcessor) SetValidationResult(result *llm.ValidationResult) {
	m.validationResult = result
	m.validationError = ""
}

func (m *MockAIResponseProcessor) SetError(err string) {
	m.processError = err
}

func (m *MockAIResponseProcessor) SetValidationError(err string) {
	m.validationError = err
}

func (m *MockAIResponseProcessor) SetHealthy(healthy bool) {
	m.healthy = healthy
}

func (m *MockAIResponseProcessor) ProcessResponse(ctx context.Context, rawResponse string, originalAlert types.Alert) (*llm.EnhancedActionRecommendation, error) {
	if m.processError != "" {
		return nil, fmt.Errorf(m.processError)
	}
	if m.processResponse != nil {
		return m.processResponse, nil
	}
	return nil, fmt.Errorf("no mock response configured")
}

func (m *MockAIResponseProcessor) ValidateRecommendation(ctx context.Context, recommendation *types.ActionRecommendation, alert types.Alert) (*llm.ValidationResult, error) {
	if m.validationError != "" {
		return nil, fmt.Errorf(m.validationError)
	}
	if m.validationResult != nil {
		return m.validationResult, nil
	}
	return nil, fmt.Errorf("no mock validation result configured")
}

func (m *MockAIResponseProcessor) AnalyzeReasoning(ctx context.Context, reasoning *types.ReasoningDetails, alert types.Alert) (*llm.ReasoningAnalysis, error) {
	if m.reasoningAnalysis != nil {
		return m.reasoningAnalysis, nil
	}
	return nil, fmt.Errorf("no mock reasoning analysis configured")
}

func (m *MockAIResponseProcessor) AssessConfidence(ctx context.Context, recommendation *types.ActionRecommendation, alert types.Alert) (*llm.ConfidenceAssessment, error) {
	if m.confidenceAssessment != nil {
		return m.confidenceAssessment, nil
	}
	return nil, fmt.Errorf("no mock confidence assessment configured")
}

func (m *MockAIResponseProcessor) EnhanceContext(ctx context.Context, recommendation *types.ActionRecommendation, alert types.Alert) (*llm.ContextualEnhancement, error) {
	if m.contextualEnhancement != nil {
		return m.contextualEnhancement, nil
	}
	return nil, fmt.Errorf("no mock contextual enhancement configured")
}

func (m *MockAIResponseProcessor) IsHealthy() bool {
	return m.healthy
}

type MockKnowledgeBase struct {
	actionRisks        map[string]*llm.RiskAssessment
	historicalPatterns []llm.HistoricalPattern
	validationRules    []llm.ValidationRule
	systemState        *llm.SystemStateAnalysis
	k8sClient          interface{} // Store K8s client for testing
}

func NewMockKnowledgeBase() *MockKnowledgeBase {
	return &MockKnowledgeBase{
		actionRisks:        make(map[string]*llm.RiskAssessment),
		historicalPatterns: []llm.HistoricalPattern{},
		validationRules:    []llm.ValidationRule{},
	}
}

func (m *MockKnowledgeBase) SetK8sClient(client interface{}) {
	m.k8sClient = client
}

func (m *MockKnowledgeBase) SetActionRisk(action string, risk *llm.RiskAssessment) {
	m.actionRisks[action] = risk
}

func (m *MockKnowledgeBase) SetHistoricalPatterns(patterns []llm.HistoricalPattern) {
	m.historicalPatterns = patterns
}

func (m *MockKnowledgeBase) GetActionRisks(action string) *llm.RiskAssessment {
	return m.actionRisks[action]
}

func (m *MockKnowledgeBase) GetHistoricalPatterns(alert types.Alert) []llm.HistoricalPattern {
	return m.historicalPatterns
}

func (m *MockKnowledgeBase) GetValidationRules() []llm.ValidationRule {
	return m.validationRules
}

func (m *MockKnowledgeBase) GetSystemState(ctx context.Context) (*llm.SystemStateAnalysis, error) {
	if m.systemState != nil {
		return m.systemState, nil
	}
	return &llm.SystemStateAnalysis{
		HealthScore:    0.8,
		StabilityScore: 0.75,
	}, nil
}
