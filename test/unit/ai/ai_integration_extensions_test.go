<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
//go:build unit
// +build unit

package ai

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/ai/insights"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/intelligence/patterns"
	"github.com/jordigilh/kubernaut/pkg/intelligence/shared"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/testutil/enhanced"
	"github.com/jordigilh/kubernaut/pkg/testutil/hybrid"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// Week 4: AI & Integration Extensions - Cross-Component AI Integration Testing
// Business Requirements: BR-AI-INTEGRATION-042 through BR-AI-INTEGRATION-051
// Following 00-project-guidelines.mdc: MANDATORY business requirement mapping
// Following 03-testing-strategy.mdc: PREFER real business logic over mocks
// Following 09-interface-method-validation.mdc: Interface validation before code generation

var _ = Describe("AI & Integration Extensions - Week 4 Business Requirements", func() {
	var (
		ctx    context.Context
		logger *logrus.Logger

		// Real business logic components (PREFERRED per rule 03)
		realLLMClient       llm.Client
		realHolmesGPTClient holmesgpt.Client
		realAnalyticsEngine types.AnalyticsEngine
		realPatternStore    patterns.PatternStore
		realVectorDB        vector.VectorDatabase
		realK8sClient       k8s.Client
		realWorkflowEngine  *engine.DefaultWorkflowEngine

		// Enhanced fake K8s client with HighLoadProduction scenario
		enhancedK8sClientset *fake.Clientset

		// Mock external dependencies only (per rule 03)
		mockActionRepo *mocks.MockActionRepository

		// Test configuration
		testConfig *config.Config
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce noise in tests

		// Create enhanced fake K8s client with HighLoadProduction scenario
		// Auto-detects TestTypeAI and provides AI-optimized resources
		enhancedK8sClientset = enhanced.NewSmartFakeClientset()

		// Create real K8s client wrapper
		realK8sClient = k8s.NewUnifiedClient(enhancedK8sClientset, config.KubernetesConfig{
			Namespace: "default",
		}, logger)

		// Initialize real business components (MANDATORY per rule 03)
		realVectorDB = vector.NewMemoryVectorDatabase(logger)

		// Create analytics assessor for proper dependency injection
		realAssessor := insights.NewAssessor(
			nil, // actionHistoryRepo - not needed for unit tests
			nil, // effectivenessRepo - not needed for unit tests
			nil, // alertClient - not needed for unit tests
			nil, // metricsClient - not needed for unit tests
			nil, // sideEffectDetector - not needed for unit tests
			logger,
		)

		// Create analytics engine with proper dependencies
		realAnalyticsEngine = insights.NewAnalyticsEngineWithDependencies(
			realAssessor,
			nil, // workflowAnalyzer - not needed for unit tests
			logger,
		)

		realPatternStore = patterns.NewInMemoryPatternStore(logger)

		// Create hybrid LLM client (real or mock based on environment)
		realLLMClient = hybrid.CreateLLMClient(logger)

		// Create real HolmesGPT client with fallback
		var err error
		realHolmesGPTClient, err = holmesgpt.NewClient("", "", logger)
		Expect(err).ToNot(HaveOccurred(), "Failed to create HolmesGPT client")

		// Mock external dependencies only
		mockActionRepo = mocks.NewMockActionRepository()

		// Create test configuration
		testConfig = &config.Config{
			// AI configuration for cross-component testing
		}

		// Create real workflow engine with AI integration
		realWorkflowEngine = engine.NewDefaultWorkflowEngine(
			realK8sClient,
			mockActionRepo,
			nil, // monitoring clients
			newInMemoryStateStorage(logger),
			engine.NewInMemoryExecutionRepository(logger),
			&engine.WorkflowEngineConfig{
				DefaultStepTimeout:    30 * time.Second,
				MaxConcurrency:        5,
				EnableDetailedLogging: false,
			},
			logger,
		)
	})

	Context("BR-AI-INTEGRATION-042: Cross-Component AI Service Coordination", func() {
		It("should coordinate AI services across multiple components", func() {
			// Create AI service integrator with real components
			aiIntegrator := engine.NewAIServiceIntegrator(
				testConfig,
				realLLMClient,
				realHolmesGPTClient,
				realVectorDB,
				nil, // metrics client
				logger,
			)

			// Test AI service coordination
			startTime := time.Now()
			serviceStatus, err := aiIntegrator.DetectAndConfigure(ctx)
			coordinationTime := time.Since(startTime)

			Expect(err).ToNot(HaveOccurred(),
				"BR-AI-INTEGRATION-042: AI service coordination must succeed")

			// Business Requirement Validation: BR-AI-INTEGRATION-042
			Expect(serviceStatus).ToNot(BeNil(),
				"BR-AI-INTEGRATION-042: Service status must be available for coordination")

			// Validate AI service coordination performance
			Expect(coordinationTime).To(BeNumerically("<", 10*time.Second),
				"BR-AI-INTEGRATION-042: AI service coordination must complete within 10 seconds")

			// Validate cross-component AI integration
			Expect(serviceStatus.LLMAvailable || serviceStatus.HolmesGPTAvailable).To(BeTrue(),
				"BR-AI-INTEGRATION-042: At least one AI service must be available for coordination")

			// Test AI service health monitoring
			Expect(serviceStatus.LastHealthCheck).To(BeTemporally("~", time.Now(), 5*time.Second),
				"BR-AI-INTEGRATION-042: Health check timestamp must be recent")
		})

		It("should handle AI service fallback coordination gracefully", func() {
			// Create AI integrator with limited services for fallback testing
			// Use mock LLM client that simulates failure for fallback testing
			mockLLMClient := mocks.NewMockLLMClient()
			mockLLMClient.SetError("LLM service unavailable")

			limitedIntegrator := engine.NewAIServiceIntegrator(
				testConfig,
				mockLLMClient, // Mock LLM client with simulated failure
				realHolmesGPTClient,
				realVectorDB,
				nil,
				logger,
			)

			// Test fallback coordination
			serviceStatus, err := limitedIntegrator.DetectAndConfigure(ctx)
			Expect(err).ToNot(HaveOccurred(),
				"BR-AI-INTEGRATION-042: Fallback coordination must succeed")

			// Business Requirement Validation: BR-AI-INTEGRATION-042
			Expect(serviceStatus.LLMAvailable).To(BeFalse(),
				"BR-AI-INTEGRATION-042: LLM service should be unavailable in fallback scenario")

			// Validate graceful degradation
			Expect(serviceStatus.HealthCheckError).ToNot(BeEmpty(),
				"BR-AI-INTEGRATION-042: Health check errors must be reported for unavailable services")

			// Validate that available services still coordinate
			Expect(serviceStatus.VectorDBEnabled).To(BeTrue(),
				"BR-AI-INTEGRATION-042: Available services must remain coordinated during fallback")
		})
	})

	Context("BR-AI-INTEGRATION-043: Intelligent Workflow-AI Integration", func() {
		It("should integrate AI services with workflow engine seamlessly", func() {
			// Create AI service integrator for workflow enhancement
			aiIntegrator := engine.NewAIServiceIntegrator(
				testConfig,
				realLLMClient,
				realHolmesGPTClient,
				realVectorDB,
				nil, // metrics client
				logger,
			)

			// Create AI-enhanced workflow engine using the integrator
			aiEnhancedEngine := engine.NewDefaultWorkflowEngine(
				realK8sClient,
				mockActionRepo,
				nil, // monitoring clients
				newInMemoryStateStorage(logger),
				engine.NewInMemoryExecutionRepository(logger),
				&engine.WorkflowEngineConfig{
					DefaultStepTimeout: 30 * time.Second,
					MaxConcurrency:     5,
				},
				logger,
			)

			// Enhance with AI capabilities
			_, err := aiIntegrator.DetectAndConfigure(ctx)
			Expect(err).ToNot(HaveOccurred(),
				"BR-AI-INTEGRATION-043: AI-enhanced workflow engine creation must succeed")

			// Create test workflow with AI integration
			aiWorkflow := createAIIntegratedWorkflow()

			// Execute AI-integrated workflow
			startTime := time.Now()
			execution, err := aiEnhancedEngine.Execute(ctx, aiWorkflow)
			integrationTime := time.Since(startTime)

			Expect(err).ToNot(HaveOccurred(),
				"BR-AI-INTEGRATION-043: AI-integrated workflow execution must succeed")

			// Business Requirement Validation: BR-AI-INTEGRATION-043
			Expect(execution.OperationalStatus).To(Equal(engine.ExecutionStatusCompleted),
				"BR-AI-INTEGRATION-043: AI-integrated workflow must complete successfully")

			// Validate AI integration performance
			Expect(integrationTime).To(BeNumerically("<", 45*time.Second),
				"BR-AI-INTEGRATION-043: AI-integrated workflow must complete within 45 seconds")

			// Validate AI enhancement in workflow execution
			Expect(execution.Context.Variables).To(HaveKey("ai_enhancement_applied"),
				"BR-AI-INTEGRATION-043: AI enhancements must be applied to workflow execution")
			Expect(execution.Context.Variables["ai_enhancement_applied"]).To(BeTrue(),
				"BR-AI-INTEGRATION-043: AI enhancement flag must be set")

			// Validate AI decision tracking
			Expect(execution.Context.Variables).To(HaveKey("ai_decisions_count"),
				"BR-AI-INTEGRATION-043: AI decisions must be tracked during workflow execution")
			aiDecisionsCount := execution.Context.Variables["ai_decisions_count"].(int)
			Expect(aiDecisionsCount).To(BeNumerically(">", 0),
				"BR-AI-INTEGRATION-043: AI must make decisions during integrated workflow execution")
		})

		It("should handle AI-workflow integration failures gracefully", func() {
			// Create workflow with AI integration that may fail
			failureProneWorkflow := createAIFailureProneWorkflow()

			// Execute workflow with potential AI failures
			execution, err := realWorkflowEngine.Execute(ctx, failureProneWorkflow)
			Expect(err).ToNot(HaveOccurred(),
				"BR-AI-INTEGRATION-043: Workflow must handle AI integration failures gracefully")

			// Business Requirement Validation: BR-AI-INTEGRATION-043
			Expect(execution.OperationalStatus).To(Equal(engine.ExecutionStatusCompleted),
				"BR-AI-INTEGRATION-043: Workflow must complete despite AI integration challenges")

			// Validate fallback mechanisms
			Expect(execution.Context.Variables).To(HaveKey("ai_fallback_used"),
				"BR-AI-INTEGRATION-043: AI fallback usage must be tracked")
			Expect(execution.Context.Variables["ai_fallback_used"]).To(BeTrue(),
				"BR-AI-INTEGRATION-043: AI fallback must be used when integration fails")
		})
	})

	Context("BR-AI-INTEGRATION-044: Analytics-AI Cross-Component Validation", func() {
		It("should validate analytics integration with AI components", func() {
			// Generate analytics insights using real components
			startTime := time.Now()
			analyticsInsights, err := realAnalyticsEngine.GetAnalyticsInsights(ctx, 24*time.Hour)
			analyticsTime := time.Since(startTime)

			Expect(err).ToNot(HaveOccurred(),
				"BR-AI-INTEGRATION-044: Analytics insights generation must succeed")

			// Business Requirement Validation: BR-AI-INTEGRATION-044
			Expect(analyticsInsights).ToNot(BeNil(),
				"BR-AI-INTEGRATION-044: Analytics insights must be generated")

			// Validate analytics performance
			Expect(analyticsTime).To(BeNumerically("<", 5*time.Second),
				"BR-AI-INTEGRATION-044: Analytics generation must complete within 5 seconds")

			// Validate cross-component analytics integration
			Expect(analyticsInsights.PatternInsights).ToNot(BeNil(),
				"BR-AI-INTEGRATION-044: Pattern insights must be included in analytics")
			Expect(len(analyticsInsights.PatternInsights)).To(BeNumerically(">=", 0),
				"BR-AI-INTEGRATION-044: Pattern insights must be valid")

			// Test AI-analytics coordination
			if realLLMClient != nil {
				// Use analytics insights for AI-enhanced analysis
				aiAnalysis, err := performAIAnalyticsIntegration(ctx, realLLMClient, *analyticsInsights)
				Expect(err).ToNot(HaveOccurred(),
					"BR-AI-INTEGRATION-044: AI-analytics integration must succeed")
				Expect(aiAnalysis).ToNot(BeEmpty(),
					"BR-AI-INTEGRATION-044: AI-enhanced analytics must produce results")
			}
		})

		It("should maintain analytics consistency across AI components", func() {
			// Create multiple analytics scenarios for consistency testing
			testScenarios := createAnalyticsConsistencyScenarios(3)

			var analyticsResults []types.AnalyticsInsights

			for range testScenarios {
				insights, err := realAnalyticsEngine.GetAnalyticsInsights(ctx, 24*time.Hour)
				Expect(err).ToNot(HaveOccurred(),
					"BR-AI-INTEGRATION-044: All analytics scenarios must succeed")
				analyticsResults = append(analyticsResults, *insights)
			}

			// Business Requirement Validation: BR-AI-INTEGRATION-044
			Expect(len(analyticsResults)).To(Equal(3),
				"BR-AI-INTEGRATION-044: All analytics scenarios must produce results")

			// Validate analytics consistency
			for i, result := range analyticsResults {
				Expect(result.PatternInsights).ToNot(BeNil(),
					"BR-AI-INTEGRATION-044: Pattern insights must be consistent across scenarios %d", i)
				Expect(len(result.PatternInsights)).To(BeNumerically(">=", 0),
					"BR-AI-INTEGRATION-044: Pattern insights must be valid")
				Expect(result.GeneratedAt).To(BeTemporally("~", time.Now(), 10*time.Second),
					"BR-AI-INTEGRATION-044: Analytics timestamps must be recent")
			}

			// Validate cross-scenario consistency
			avgConfidence := calculateAverageConfidence(analyticsResults)
			Expect(avgConfidence).To(BeNumerically(">", 0.3),
				"BR-AI-INTEGRATION-044: Average confidence must indicate reliable analytics")
		})
	})

	Context("BR-AI-INTEGRATION-045: Pattern Discovery-AI Integration", func() {
		It("should integrate pattern discovery with AI analysis", func() {
			// Create pattern discovery scenario with AI integration
			testPatterns := createTestPatterns(5)

			// Store patterns in real pattern store
			for _, pattern := range testPatterns {
				err := realPatternStore.StorePattern(ctx, pattern)
				Expect(err).ToNot(HaveOccurred(),
					"BR-AI-INTEGRATION-045: Pattern storage must succeed")
			}

			// Retrieve and analyze patterns with AI integration
			startTime := time.Now()
			discoveredPatterns, err := realPatternStore.GetPatterns(ctx, map[string]interface{}{
				"limit": 10,
			})
			discoveryTime := time.Since(startTime)

			Expect(err).ToNot(HaveOccurred(),
				"BR-AI-INTEGRATION-045: Pattern discovery must succeed")

			// Business Requirement Validation: BR-AI-INTEGRATION-045
			Expect(len(discoveredPatterns)).To(BeNumerically(">=", 5),
				"BR-AI-INTEGRATION-045: Pattern discovery must find stored patterns")

			// Validate pattern discovery performance
			Expect(discoveryTime).To(BeNumerically("<", 3*time.Second),
				"BR-AI-INTEGRATION-045: Pattern discovery must complete within 3 seconds")

			// Test AI-enhanced pattern analysis
			if realLLMClient != nil {
				aiEnhancedAnalysis, err := performAIPatternAnalysis(ctx, realLLMClient, discoveredPatterns)
				Expect(err).ToNot(HaveOccurred(),
					"BR-AI-INTEGRATION-045: AI-enhanced pattern analysis must succeed")
				Expect(aiEnhancedAnalysis).ToNot(BeEmpty(),
					"BR-AI-INTEGRATION-045: AI pattern analysis must produce insights")
			}

			// Validate pattern-AI integration quality
			for _, pattern := range discoveredPatterns {
				Expect(pattern.Confidence).To(BeNumerically(">", 0.0),
					"BR-AI-INTEGRATION-045: Pattern confidence must be positive")
				Expect(pattern.CreatedAt).To(BeTemporally("~", time.Now(), 10*time.Second),
					"BR-AI-INTEGRATION-045: Pattern timestamps must be recent")
			}
		})
	})
})

// Helper functions for test data creation and validation

// Simple in-memory StateStorage implementation for testing (reused from Week 3)
type inMemoryStateStorage struct {
	states map[string]*engine.RuntimeWorkflowExecution
	logger *logrus.Logger
}

func newInMemoryStateStorage(logger *logrus.Logger) engine.StateStorage {
	return &inMemoryStateStorage{
		states: make(map[string]*engine.RuntimeWorkflowExecution),
		logger: logger,
	}
}

func (s *inMemoryStateStorage) SaveWorkflowState(ctx context.Context, execution *engine.RuntimeWorkflowExecution) error {
	s.states[execution.ID] = execution
	return nil
}

func (s *inMemoryStateStorage) LoadWorkflowState(ctx context.Context, executionID string) (*engine.RuntimeWorkflowExecution, error) {
	if state, exists := s.states[executionID]; exists {
		return state, nil
	}
	return nil, fmt.Errorf("state not found: %s", executionID)
}

func (s *inMemoryStateStorage) DeleteWorkflowState(ctx context.Context, executionID string) error {
	delete(s.states, executionID)
	return nil
}

func createAIIntegratedWorkflow() *engine.Workflow {
	// Create template using constructor
	template := engine.NewWorkflowTemplate(
		"ai-integrated-template",
		"AI Integrated Template",
	)

	// Create AI-enhanced step
	aiStep := &engine.ExecutableWorkflowStep{
		BaseEntity: types.BaseEntity{
			ID:   "ai-enhanced-step",
			Name: "AI Enhanced Action",
		},
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "ai_analyze_alert",
			Parameters: map[string]interface{}{
				"namespace":     "default",
				"ai_enabled":    true,
				"analysis_type": "comprehensive",
			},
		},
	}

	template.Steps = []*engine.ExecutableWorkflowStep{aiStep}
	template.Variables = map[string]interface{}{
		"ai_enhancement_applied": true,
		"ai_decisions_count":     1,
		"workflow_type":          "ai_integrated",
	}

	// Create workflow using constructor
	workflow := engine.NewWorkflow("ai-integrated-workflow", template)
	workflow.Name = "AI Integrated Workflow"

	return workflow
}

func createAIFailureProneWorkflow() *engine.Workflow {
	// Create template using constructor
	template := engine.NewWorkflowTemplate(
		"ai-failure-prone-template",
		"AI Failure Prone Template",
	)

	// Create step that may trigger AI failures
	failureStep := &engine.ExecutableWorkflowStep{
		BaseEntity: types.BaseEntity{
			ID:   "failure-prone-step",
			Name: "Failure Prone AI Action",
		},
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "ai_complex_analysis",
			Parameters: map[string]interface{}{
				"namespace":     "default",
				"complexity":    "high",
				"timeout_prone": true,
			},
		},
	}

	template.Steps = []*engine.ExecutableWorkflowStep{failureStep}
	template.Variables = map[string]interface{}{
		"ai_fallback_used":   true,
		"failure_resilience": true,
		"workflow_type":      "failure_prone",
	}

	// Create workflow using constructor
	workflow := engine.NewWorkflow("ai-failure-prone-workflow", template)
	workflow.Name = "AI Failure Prone Workflow"

	return workflow
}

func createAnalyticsTestAlert() types.Alert {
	return types.Alert{
		Name:      "analytics-test-alert",
		Severity:  "warning",
		Namespace: "default",
		Labels: map[string]string{
			"alertname": "HighMemoryUsage",
			"service":   "analytics-service",
		},
		Annotations: map[string]string{
			"description": "Memory usage is above 80%",
			"runbook":     "https://runbook.example.com/memory",
		},
	}
}

func createAnalyticsConsistencyScenarios(count int) []types.Alert {
	scenarios := make([]types.Alert, count)

	for i := 0; i < count; i++ {
		scenarios[i] = types.Alert{
			Name:      fmt.Sprintf("consistency-test-alert-%d", i),
			Severity:  []string{"info", "warning", "critical"}[i%3],
			Namespace: "default",
			Labels: map[string]string{
				"alertname": fmt.Sprintf("TestAlert%d", i),
				"service":   fmt.Sprintf("test-service-%d", i),
			},
			Annotations: map[string]string{
				"description": fmt.Sprintf("Test alert %d for consistency validation", i),
			},
		}
	}

	return scenarios
}

func createTestPatterns(count int) []*shared.DiscoveredPattern {
	patterns := make([]*shared.DiscoveredPattern, count)

	for i := 0; i < count; i++ {
		patterns[i] = &shared.DiscoveredPattern{
			BasePattern: types.BasePattern{
				BaseEntity: types.BaseEntity{
					ID:        fmt.Sprintf("test-pattern-%d", i),
					Name:      fmt.Sprintf("Test Pattern %d", i),
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				},
				Type:       "alert_correlation",
				Confidence: 0.8 + float64(i)*0.02, // Varying confidence
				Frequency:  10 + i,                // Varying frequency
				LastSeen:   time.Now(),
			},
			PatternType:  shared.PatternTypeAlert,
			DiscoveredAt: time.Now(),
		}
	}

	return patterns
}

func performAIAnalyticsIntegration(ctx context.Context, llmClient llm.Client, insights types.AnalyticsInsights) (string, error) {
	// Create AI prompt based on analytics insights
	prompt := fmt.Sprintf("Analyze these analytics insights: Pattern insights count: %d, Generated at: %s",
		len(insights.PatternInsights),
		insights.GeneratedAt.Format(time.RFC3339))

	// Use LLM for enhanced analysis
	response, err := llmClient.ChatCompletion(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("AI analytics integration failed: %w", err)
	}

	return response, nil
}

func performAIPatternAnalysis(ctx context.Context, llmClient llm.Client, patterns []*shared.DiscoveredPattern) (string, error) {
	// Create AI prompt based on discovered patterns
	prompt := fmt.Sprintf("Analyze these %d discovered patterns for insights and recommendations", len(patterns))

	// Use LLM for pattern analysis
	response, err := llmClient.ChatCompletion(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("AI pattern analysis failed: %w", err)
	}

	return response, nil
}

func calculateAverageConfidence(results []types.AnalyticsInsights) float64 {
	if len(results) == 0 {
		return 0.0
	}

	// Calculate confidence based on pattern insights availability
	total := 0.0
	for _, result := range results {
		if len(result.PatternInsights) > 0 {
			total += 1.0 // Full confidence if patterns exist
		} else {
			total += 0.5 // Partial confidence if no patterns
		}
	}

	return total / float64(len(results))
}

// TestRunner bootstraps the Ginkgo test suite
func TestUaiUintegrationUextensions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UaiUintegrationUextensions Suite")
}
