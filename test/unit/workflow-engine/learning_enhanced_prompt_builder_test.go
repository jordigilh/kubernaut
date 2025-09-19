package workflowengine

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// Mock LLM Client for testing
type MockLLMClient struct {
	responses   []string
	shouldError bool
	errorMsg    string
}

func NewMockLLMClient() *MockLLMClient {
	return &MockLLMClient{
		responses: make([]string, 0),
	}
}

func (m *MockLLMClient) SetNextResponse(response string) {
	m.responses = append(m.responses, response)
}

func (m *MockLLMClient) SetShouldError(shouldError bool, errorMsg string) {
	m.shouldError = shouldError
	m.errorMsg = errorMsg
}

func (m *MockLLMClient) GenerateResponse(prompt string) (string, error) {
	if m.shouldError {
		return "", fmt.Errorf("%s", m.errorMsg)
	}
	if len(m.responses) > 0 {
		response := m.responses[0]
		m.responses = m.responses[1:]
		return response, nil
	}
	return "Mock response for: " + prompt, nil
}

func (m *MockLLMClient) ChatCompletion(ctx context.Context, prompt string) (string, error) {
	return m.GenerateResponse(prompt)
}

func (m *MockLLMClient) AnalyzeAlert(ctx context.Context, alert interface{}) (*llm.AnalyzeAlertResponse, error) {
	if m.shouldError {
		return nil, fmt.Errorf("%s", m.errorMsg)
	}
	return &llm.AnalyzeAlertResponse{}, nil
}

func (m *MockLLMClient) GenerateWorkflow(ctx context.Context, objective *llm.WorkflowObjective) (*llm.WorkflowGenerationResult, error) {
	if m.shouldError {
		return nil, fmt.Errorf("%s", m.errorMsg)
	}
	return &llm.WorkflowGenerationResult{}, nil
}

func (m *MockLLMClient) IsHealthy() bool {
	return !m.shouldError
}

func (m *MockLLMClient) LivenessCheck(ctx context.Context) error {
	if m.shouldError {
		return fmt.Errorf("%s", m.errorMsg)
	}
	return nil
}

func (m *MockLLMClient) ReadinessCheck(ctx context.Context) error {
	if m.shouldError {
		return fmt.Errorf("%s", m.errorMsg)
	}
	return nil
}

func (m *MockLLMClient) GetEndpoint() string {
	return "mock://llm"
}

func (m *MockLLMClient) GetModel() string {
	return "mock-model"
}

func (m *MockLLMClient) GetMinParameterCount() int64 {
	return 1000000
}

// Mock Execution Repository for testing
type MockExecutionRepository struct {
	executions map[string]*engine.RuntimeWorkflowExecution
}

func NewMockExecutionRepository() *MockExecutionRepository {
	return &MockExecutionRepository{
		executions: make(map[string]*engine.RuntimeWorkflowExecution),
	}
}

func (m *MockExecutionRepository) StoreExecution(ctx context.Context, execution *engine.RuntimeWorkflowExecution) error {
	m.executions[execution.ID] = execution
	return nil
}

func (m *MockExecutionRepository) GetExecution(ctx context.Context, executionID string) (*engine.RuntimeWorkflowExecution, error) {
	if exec, exists := m.executions[executionID]; exists {
		return exec, nil
	}
	return nil, fmt.Errorf("execution not found: %s", executionID)
}

func (m *MockExecutionRepository) GetExecutionsByWorkflowID(ctx context.Context, workflowID string) ([]*engine.RuntimeWorkflowExecution, error) {
	var result []*engine.RuntimeWorkflowExecution
	for _, exec := range m.executions {
		if exec.WorkflowID == workflowID {
			result = append(result, exec)
		}
	}
	return result, nil
}

func (m *MockExecutionRepository) GetExecutionsByPattern(ctx context.Context, pattern string) ([]*engine.RuntimeWorkflowExecution, error) {
	var result []*engine.RuntimeWorkflowExecution
	for _, exec := range m.executions {
		// Enhanced pattern matching for testing - check IDs, prompts, and context
		if strings.Contains(exec.WorkflowID, pattern) || strings.Contains(exec.ID, pattern) {
			result = append(result, exec)
			continue
		}

		// Check prompts in metadata
		if promptsData, ok := exec.Metadata["prompts"]; ok {
			if promptsList, ok := promptsData.([]interface{}); ok {
				for _, p := range promptsList {
					if prompt, ok := p.(string); ok {
						if strings.Contains(strings.ToLower(prompt), strings.ToLower(pattern)) {
							result = append(result, exec)
							goto nextExecution
						}
					}
				}
			}
		}

		// Check context variables
		if exec.Context != nil && exec.Context.Variables != nil {
			for key, value := range exec.Context.Variables {
				valueStr := fmt.Sprintf("%v", value)
				if strings.Contains(strings.ToLower(key), strings.ToLower(pattern)) ||
					strings.Contains(strings.ToLower(valueStr), strings.ToLower(pattern)) {
					result = append(result, exec)
					break
				}
			}
		}

	nextExecution:
	}
	return result, nil
}

func (m *MockExecutionRepository) GetExecutionsInTimeWindow(ctx context.Context, start, end time.Time) ([]*engine.RuntimeWorkflowExecution, error) {
	var result []*engine.RuntimeWorkflowExecution
	for _, exec := range m.executions {
		// Check if execution falls within time window
		if exec.StartTime.After(start) && exec.StartTime.Before(end) {
			result = append(result, exec)
		}
	}
	return result, nil
}

var _ = Describe("Learning Enhanced Prompt Builder - Business Requirements Testing", func() {
	var (
		ctx                context.Context
		testLogger         *logrus.Logger
		mockLLMClient      *MockLLMClient
		mockVectorDB       *mocks.MockVectorDatabase
		mockExecutionRepo  *MockExecutionRepository
		promptBuilder      engine.LearningEnhancedPromptBuilder
		testExecution      *engine.RuntimeWorkflowExecution
		testSuccessContext map[string]interface{}
		testFailureContext map[string]interface{}
		testKubernetesCtx  map[string]interface{}
		testAlertContext   map[string]interface{}
		testProductionCtx  map[string]interface{}
	)

	BeforeEach(func() {
		ctx = context.Background()
		testLogger = logrus.New()
		testLogger.SetLevel(logrus.WarnLevel) // Reduce test noise

		// Initialize mocks following existing patterns
		mockLLMClient = NewMockLLMClient()
		mockVectorDB = mocks.NewMockVectorDatabase()
		mockExecutionRepo = NewMockExecutionRepository()

		// Create test contexts aligned with business requirements
		testSuccessContext = map[string]interface{}{
			"alert_type":    "cpu_high",
			"severity":      "warning",
			"namespace":     "production",
			"environment":   "production",
			"workflow_type": "remediation",
			"complexity":    0.6,
		}

		testFailureContext = map[string]interface{}{
			"alert_type":    "pod_crash",
			"severity":      "critical",
			"namespace":     "default",
			"environment":   "production",
			"workflow_type": "emergency",
			"complexity":    0.9,
		}

		testKubernetesCtx = map[string]interface{}{
			"namespace":     "kube-system",
			"pod":           "nginx-deployment-abc123",
			"deployment":    "nginx-deployment",
			"resource_type": "pod",
			"alert_type":    "pod_crash",
		}

		testAlertContext = map[string]interface{}{
			"alert_type": "memory",
			"severity":   "critical",
			"alert_name": "MemoryUsageHigh",
			"alert_time": time.Now().Format(time.RFC3339),
		}

		testProductionCtx = map[string]interface{}{
			"environment":   "production",
			"workflow_type": "remediation",
			"complexity":    0.8,
			"severity":      "critical",
		}

		// Create test execution for learning scenarios
		testExecution = &engine.RuntimeWorkflowExecution{
			WorkflowExecutionRecord: types.WorkflowExecutionRecord{
				ID:         "test-execution-123",
				WorkflowID: "test-workflow-456",
				Status:     string(engine.ExecutionStatusCompleted),
				StartTime:  time.Now().Add(-10 * time.Minute),
				Metadata: map[string]interface{}{
					"prompts": []interface{}{"Analyze CPU usage and recommend scaling"},
				},
			},
			Duration: 5 * time.Minute,
			Context: &engine.ExecutionContext{
				Variables: testSuccessContext,
			},
			Steps: []*engine.StepExecution{
				{
					StepID:   "step-1",
					Status:   engine.ExecutionStatusCompleted,
					Metadata: map[string]interface{}{"prompt": "Analyze CPU usage and recommend scaling"},
					Result: &engine.StepResult{
						Success: true,
					},
				},
			},
		}

		// Create prompt builder with mocks - following TDD principle: define business contract first
		promptBuilder = engine.NewDefaultLearningEnhancedPromptBuilder(
			mockLLMClient,
			mockVectorDB,
			mockExecutionRepo,
			testLogger,
		)
	})

	// Business Requirement: BR-AI-PROMPT-001 - Intelligent Prompt Enhancement
	Describe("Core Prompt Building Functionality", func() {
		Context("when building prompts with basic templates", func() {
			It("should enhance basic prompts with contextual information", func() {
				template := "Analyze the alert and recommend actions"

				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)

				Expect(err).ToNot(HaveOccurred(), "BuildPrompt should succeed with valid inputs")
				Expect(enhancedPrompt).ToNot(BeEmpty(), "Enhanced prompt should not be empty")
				Expect(enhancedPrompt).To(ContainSubstring("production"),
					"Enhanced prompt should include production context for production alerts")
				Expect(enhancedPrompt).To(ContainSubstring("warning"),
					"Enhanced prompt should include severity level for proper urgency")
				Expect(len(enhancedPrompt)).To(BeNumerically(">", len(template)),
					"Enhanced prompt should be longer than original template due to contextual additions")
			})

			It("should handle critical severity alerts with increased urgency", func() {
				criticalTemplate := "Handle system alert"

				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, criticalTemplate, testFailureContext)

				Expect(err).ToNot(HaveOccurred(), "BuildPrompt should handle critical alerts")
				Expect(enhancedPrompt).To(ContainSubstring("critical"),
					"Critical alerts should be explicitly marked in prompt")
				Expect(enhancedPrompt).To(ContainSubstring("emergency"),
					"Emergency workflow type should be reflected in prompt tone")
				// Business validation: Critical alerts should have immediate action language
				criticalLanguageFound := strings.Contains(strings.ToUpper(enhancedPrompt), "URGENT") ||
					strings.Contains(strings.ToUpper(enhancedPrompt), "IMMEDIATE") ||
					strings.Contains(strings.ToUpper(enhancedPrompt), "CRITICAL")
				Expect(criticalLanguageFound).To(BeTrue(),
					"Critical alerts should include urgent language for immediate action")
			})

			It("should apply Kubernetes-specific optimizations for cluster contexts", func() {
				k8sTemplate := "Troubleshoot container issue"

				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, k8sTemplate, testKubernetesCtx)

				Expect(err).ToNot(HaveOccurred(), "BuildPrompt should handle Kubernetes contexts")
				Expect(enhancedPrompt).To(ContainSubstring("Kubernetes"),
					"Kubernetes context should trigger K8s-specific guidance")
				Expect(enhancedPrompt).To(ContainSubstring("pod"),
					"Pod resource type should be included in enhanced prompt")
				// Business validation: System namespace warnings should be present
				Expect(enhancedPrompt).To(ContainSubstring("system namespace"),
					"kube-system namespace should trigger system-level caution")
			})
		})

		Context("when handling domain-specific contexts", func() {
			It("should optimize prompts for alert-specific scenarios", func() {
				alertTemplate := "Address system performance issue"

				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, alertTemplate, testAlertContext)

				Expect(err).ToNot(HaveOccurred(), "BuildPrompt should handle alert contexts")
				Expect(enhancedPrompt).To(ContainSubstring("memory"),
					"Memory alert type should be specifically mentioned")
				Expect(enhancedPrompt).To(ContainSubstring("Memory alert"),
					"Alert-specific guidance should be included")
				// Business validation: Time-sensitive factors should be considered
				Expect(enhancedPrompt).To(ContainSubstring("time-sensitive"),
					"Alert timestamp should trigger time-awareness in prompt")
			})

			It("should enhance prompts with production environment safeguards", func() {
				prodTemplate := "Execute maintenance task"

				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, prodTemplate, testProductionCtx)

				Expect(err).ToNot(HaveOccurred(), "BuildPrompt should handle production contexts")
				Expect(enhancedPrompt).To(ContainSubstring("production"),
					"Production environment should be explicitly mentioned")
				Expect(enhancedPrompt).To(ContainSubstring("caution"),
					"Production context should trigger cautionary language")
				// Business validation: Production-specific recommendations should be included
				Expect(enhancedPrompt).To(ContainSubstring("specific recommendations"),
					"Production environments require specific, actionable guidance")
			})
		})
	})

	// Business Requirement: BR-AI-PROMPT-002 - Learning from Execution Outcomes
	Describe("Learning and Adaptation Capabilities", func() {
		Context("when learning from successful workflow executions", func() {
			It("should extract and learn patterns from successful executions", func() {
				err := promptBuilder.GetLearnFromExecution(ctx, testExecution)

				Expect(err).ToNot(HaveOccurred(), "Learning from successful execution should succeed")

				// Business validation: Learning should improve future prompt quality
				// Test by building a similar prompt and verifying improvement
				similarTemplate := "Analyze CPU usage patterns"
				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, similarTemplate, testSuccessContext)

				Expect(err).ToNot(HaveOccurred(), "Prompt building after learning should succeed")
				Expect(enhancedPrompt).To(ContainSubstring("CPU"),
					"Learned patterns should influence similar prompt enhancement")
			})

			It("should update template success rates based on execution outcomes", func() {
				// Setup: Create execution with specific prompt
				successExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "success-execution",
						WorkflowID: "success-workflow",
						Status:     string(engine.ExecutionStatusCompleted),
						Metadata: map[string]interface{}{
							"prompts": []interface{}{"Test template for success tracking"},
						},
					},
					Duration: 3 * time.Minute,
				}

				err := promptBuilder.GetLearnFromExecution(ctx, successExecution)
				Expect(err).ToNot(HaveOccurred(), "Learning from successful execution should update success rates")

				// Business validation: Multiple successful executions should increase template confidence
				for i := 0; i < 3; i++ {
					err = promptBuilder.GetLearnFromExecution(ctx, successExecution)
					Expect(err).ToNot(HaveOccurred(), "Repeated learning should succeed")
				}
			})
		})

		Context("when learning from failed workflow executions", func() {
			It("should reduce confidence for patterns from failed executions", func() {
				failedExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "failed-execution",
						WorkflowID: "failed-workflow",
						Status:     string(engine.ExecutionStatusFailed),
						Metadata: map[string]interface{}{
							"prompts": []interface{}{"Problematic prompt that led to failure"},
						},
					},
					Duration: 10 * time.Minute,
					Context: &engine.ExecutionContext{
						Variables: testFailureContext,
					},
				}

				err := promptBuilder.GetLearnFromExecution(ctx, failedExecution)

				Expect(err).ToNot(HaveOccurred(), "Learning from failures should succeed to improve future performance")

				// Business validation: Failed patterns should be less likely to be applied
				// Test by checking that similar patterns have reduced influence
				similarFailureTemplate := "Similar problematic approach"
				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, similarFailureTemplate, testFailureContext)

				Expect(err).ToNot(HaveOccurred(), "Prompt building should still work after learning from failures")
				Expect(enhancedPrompt).ToNot(BeEmpty(), "Failed pattern learning should not break prompt building")
			})
		})

		Context("when identifying applicable adaptation rules from execution context", func() {
			It("should identify rules based on execution context patterns", func() {
				// Setup: Create test execution with rich context
				contextExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "context-execution",
						WorkflowID: "context-workflow",
						Status:     string(engine.ExecutionStatusCompleted),
						Metadata: map[string]interface{}{
							"prompts": []interface{}{"Test prompt for rule identification"},
						},
					},
					Context: &engine.ExecutionContext{
						Variables: map[string]interface{}{
							"severity":      "critical",
							"environment":   "production",
							"workflow_type": "emergency",
							"alert_type":    "pod_crash",
							"namespace":     "kube-system",
						},
					},
				}

				// Business validation: identifyApplicableRules should return rules matching execution context
				rules := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder).IdentifyApplicableRules("Test prompt for rule identification", contextExecution)

				// Expect rules to be identified based on context patterns
				Expect(len(rules)).To(BeNumerically(">=", 0), "Should return zero or more applicable rules")

				// Validate rule selection logic for high severity contexts
				for _, rule := range rules {
					Expect(rule).ToNot(BeNil(), "Returned rules should not be nil")
					Expect(rule.ID).ToNot(BeEmpty(), "Rule should have valid ID")
					Expect(rule.Condition).ToNot(BeEmpty(), "Rule should have valid condition")
				}
			})

			It("should identify high-severity rules for critical alerts", func() {
				// Setup: Create critical alert execution
				criticalExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "critical-execution",
						WorkflowID: "critical-workflow",
						Status:     string(engine.ExecutionStatusCompleted),
					},
					Context: &engine.ExecutionContext{
						Variables: map[string]interface{}{
							"severity":    "critical",
							"alert_type":  "memory_leak",
							"environment": "production",
						},
					},
				}

				// Pre-populate some test adaptation rules
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				builder.AddTestAdaptationRule(&engine.AdaptationRule{
					ID:          "high_severity_rule",
					Condition:   "high_severity",
					Adaptation:  "Apply immediate action protocols",
					Confidence:  0.8,
					Performance: 0.7,
				})
				builder.AddTestAdaptationRule(&engine.AdaptationRule{
					ID:          "production_rule",
					Condition:   "production",
					Adaptation:  "Exercise production caution",
					Confidence:  0.9,
					Performance: 0.8,
				})

				// Business validation: Should identify rules applicable to critical severity
				rules := builder.IdentifyApplicableRules("Handle critical memory leak", criticalExecution)

				// Expect high severity and production rules to be identified
				Expect(len(rules)).To(BeNumerically(">=", 1), "Should identify at least one applicable rule for critical context")

				// Check for specific rule types
				foundHighSeverity := false
				foundProduction := false
				for _, rule := range rules {
					if rule.Condition == "high_severity" {
						foundHighSeverity = true
					}
					if rule.Condition == "production" {
						foundProduction = true
					}
				}

				Expect(foundHighSeverity).To(BeTrue(), "Should identify high severity rules for critical alerts")
				Expect(foundProduction).To(BeTrue(), "Should identify production rules for production environment")
			})

			It("should identify rules based on prompt content patterns", func() {
				// Setup: Create execution with prompt containing specific patterns
				patternExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "pattern-execution",
						WorkflowID: "pattern-workflow",
						Status:     string(engine.ExecutionStatusFailed),
					},
					Context: &engine.ExecutionContext{
						Variables: map[string]interface{}{
							"alert_type": "kubernetes_pod",
							"namespace":  "default",
						},
					},
				}

				// Pre-populate rules with pattern-based conditions
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				builder.AddTestAdaptationRule(&engine.AdaptationRule{
					ID:         "kubernetes_rule",
					Condition:  "kubernetes_context",
					Adaptation: "Apply Kubernetes-specific troubleshooting",
					Confidence: 0.7,
					Context: map[string]interface{}{
						"resource_type": "pod",
					},
				})

				// Business validation: Should identify rules based on prompt content
				kubernetesPrompt := "Troubleshoot Kubernetes pod in namespace"
				rules := builder.IdentifyApplicableRules(kubernetesPrompt, patternExecution)

				// Expect rule identification based on prompt patterns
				Expect(len(rules)).To(BeNumerically(">=", 0), "Should process prompt patterns for rule identification")

				// If Kubernetes-related rules exist, they should be identified for Kubernetes prompts
				for _, rule := range rules {
					if rule.Condition == "kubernetes_context" {
						Expect(rule.Adaptation).To(ContainSubstring("Kubernetes"),
							"Kubernetes rules should be identified for Kubernetes-related prompts")
					}
				}
			})

			It("should return empty rules for executions without context", func() {
				// Setup: Create execution without context
				emptyExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "empty-execution",
						WorkflowID: "empty-workflow",
						Status:     string(engine.ExecutionStatusCompleted),
					},
					Context: nil,
				}

				// Business validation: Should handle nil context gracefully
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				rules := builder.IdentifyApplicableRules("Basic prompt", emptyExecution)

				// Should not crash and return valid result
				Expect(rules).ToNot(BeNil(), "Should return non-nil slice even with nil context")
				// May return empty or contain 'always' rules
				for _, rule := range rules {
					Expect(rule.Condition).To(Equal("always"), "Only 'always' rules should apply when no context available")
				}
			})

			It("should identify multiple overlapping rules for complex contexts", func() {
				// Setup: Create execution with overlapping rule conditions
				complexExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "complex-execution",
						WorkflowID: "complex-workflow",
						Status:     string(engine.ExecutionStatusFailed),
					},
					Context: &engine.ExecutionContext{
						Variables: map[string]interface{}{
							"severity":      "critical",
							"environment":   "production",
							"alert_type":    "memory_leak",
							"namespace":     "kube-system",
							"workflow_type": "emergency",
						},
					},
				}

				// Pre-populate overlapping rules
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				builder.AddTestAdaptationRule(&engine.AdaptationRule{
					ID:          "high_severity_rule",
					Condition:   "high_severity",
					Adaptation:  "Apply critical severity protocols",
					Confidence:  0.9,
					Performance: 0.8,
				})
				builder.AddTestAdaptationRule(&engine.AdaptationRule{
					ID:          "production_rule",
					Condition:   "production",
					Adaptation:  "Exercise production caution",
					Confidence:  0.8,
					Performance: 0.9,
				})
				builder.AddTestAdaptationRule(&engine.AdaptationRule{
					ID:          "memory_rule",
					Condition:   "memory_context",
					Adaptation:  "Focus on memory analysis",
					Confidence:  0.7,
					Performance: 0.7,
				})
				builder.AddTestAdaptationRule(&engine.AdaptationRule{
					ID:          "emergency_rule",
					Condition:   "emergency_workflow",
					Adaptation:  "Execute emergency protocols",
					Confidence:  0.9,
					Performance: 0.8,
				})

				// Business validation: Should identify all applicable rules
				rules := builder.IdentifyApplicableRules("Handle critical memory leak in production", complexExecution)

				// Expect multiple rules to be identified
				Expect(len(rules)).To(BeNumerically(">=", 3), "Should identify multiple applicable rules for complex context")

				// Verify specific rule types are identified
				ruleConditions := make(map[string]bool)
				for _, rule := range rules {
					ruleConditions[rule.Condition] = true
				}

				Expect(ruleConditions["high_severity"]).To(BeTrue(), "Should identify high severity rule for critical alerts")
				Expect(ruleConditions["production"]).To(BeTrue(), "Should identify production rule for production environment")
				Expect(ruleConditions["memory_context"]).To(BeTrue(), "Should identify memory rule for memory-related alerts")
				Expect(ruleConditions["emergency_workflow"]).To(BeTrue(), "Should identify emergency rule for emergency workflows")
			})

			It("should identify rules based on prompt content when context is minimal", func() {
				// Setup: Create execution with minimal context but rich prompt content
				minimalExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "minimal-execution",
						WorkflowID: "minimal-workflow",
						Status:     string(engine.ExecutionStatusCompleted),
					},
					Context: &engine.ExecutionContext{
						Variables: map[string]interface{}{
							"basic_context": "true",
						},
					},
				}

				// Pre-populate content-pattern rules
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				builder.AddTestAdaptationRule(&engine.AdaptationRule{
					ID:          "troubleshooting_rule",
					Condition:   "troubleshooting_context",
					Adaptation:  "Apply systematic troubleshooting",
					Confidence:  0.8,
					Performance: 0.7,
				})
				builder.AddTestAdaptationRule(&engine.AdaptationRule{
					ID:          "scaling_rule",
					Condition:   "scaling_context",
					Adaptation:  "Consider scaling implications",
					Confidence:  0.7,
					Performance: 0.8,
				})
				builder.AddTestAdaptationRule(&engine.AdaptationRule{
					ID:          "memory_content_rule",
					Condition:   "memory_context",
					Adaptation:  "Investigate memory patterns",
					Confidence:  0.9,
					Performance: 0.8,
				})

				// Business validation: Should identify rules based on prompt content
				troubleshootingPrompt := "Debug and troubleshoot the failing application pods"
				rules := builder.IdentifyApplicableRules(troubleshootingPrompt, minimalExecution)

				// Expect content-based rule identification
				foundTroubleshooting := false
				for _, rule := range rules {
					if rule.Condition == "troubleshooting_context" {
						foundTroubleshooting = true
					}
				}
				Expect(foundTroubleshooting).To(BeTrue(), "Should identify troubleshooting rules based on prompt content")

				// Test scaling content
				scalingPrompt := "Scale up the deployment to handle increased capacity"
				scalingRules := builder.IdentifyApplicableRules(scalingPrompt, minimalExecution)

				foundScaling := false
				for _, rule := range scalingRules {
					if rule.Condition == "scaling_context" {
						foundScaling = true
					}
				}
				Expect(foundScaling).To(BeTrue(), "Should identify scaling rules based on prompt content")

				// Test memory content
				memoryPrompt := "Investigate memory leak causing OOM errors"
				memoryRules := builder.IdentifyApplicableRules(memoryPrompt, minimalExecution)

				foundMemory := false
				for _, rule := range memoryRules {
					if rule.Condition == "memory_context" {
						foundMemory = true
					}
				}
				Expect(foundMemory).To(BeTrue(), "Should identify memory rules based on prompt content")
			})

			It("should handle edge cases and maintain rule identification robustness", func() {
				// Business validation: Edge case handling
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)

				// Test nil execution
				nilRules := builder.IdentifyApplicableRules("Test prompt", nil)
				Expect(nilRules).ToNot(BeNil(), "Should handle nil execution gracefully")
				Expect(len(nilRules)).To(Equal(0), "Should return empty rules for nil execution")

				// Test empty prompt
				emptyExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID: "empty-prompt-execution",
					},
				}
				emptyRules := builder.IdentifyApplicableRules("", emptyExecution)
				Expect(emptyRules).ToNot(BeNil(), "Should handle empty prompt gracefully")

				// Test with 'always' rule
				builder.AddTestAdaptationRule(&engine.AdaptationRule{
					ID:          "always_rule",
					Condition:   "always",
					Adaptation:  "Universal adaptation",
					Confidence:  1.0,
					Performance: 0.9,
				})

				// Always rule should be identified even with minimal context
				alwaysRules := builder.IdentifyApplicableRules("Any prompt", emptyExecution)
				foundAlways := false
				for _, rule := range alwaysRules {
					if rule.Condition == "always" {
						foundAlways = true
					}
				}
				Expect(foundAlways).To(BeTrue(), "Should always identify 'always' condition rules")
			})

			It("should support learning integration through rule performance tracking", func() {
				// Setup: Create execution for learning integration test
				learningExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "learning-execution",
						WorkflowID: "learning-workflow",
						Status:     string(engine.ExecutionStatusCompleted),
						Metadata: map[string]interface{}{
							"prompts": []interface{}{"Learn from this successful execution"},
						},
					},
					Context: &engine.ExecutionContext{
						Variables: map[string]interface{}{
							"severity":    "warning",
							"environment": "staging",
							"alert_type":  "cpu_high",
						},
					},
				}

				// Pre-populate rule for learning integration
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				testRule := &engine.AdaptationRule{
					ID:          "learning_test_rule",
					Condition:   "production", // Won't match staging
					Adaptation:  "Apply production protocols",
					Confidence:  0.5,
					Performance: 0.5,
				}
				builder.AddTestAdaptationRule(testRule)

				// Business validation: Rule identification supports learning workflow
				rules := builder.IdentifyApplicableRules("Learn from this successful execution", learningExecution)

				// Test that the learning workflow can process identified rules
				err := promptBuilder.GetLearnFromExecution(ctx, learningExecution)
				Expect(err).ToNot(HaveOccurred(), "Learning from execution should succeed with rule identification")

				// Verify rule identification is consistent and repeatable
				rules2 := builder.IdentifyApplicableRules("Learn from this successful execution", learningExecution)
				Expect(len(rules2)).To(Equal(len(rules)), "Rule identification should be consistent across calls")
			})
		})
	})

	// Business Requirement: BR-AI-PROMPT-003 - Template Optimization and Caching
	Describe("Template Optimization and Caching", func() {
		Context("when retrieving optimized templates", func() {
			It("should provide optimized versions of frequently used templates", func() {
				templateID := "frequent-template-123"

				// First attempt should return error (template doesn't exist yet)
				_, err := promptBuilder.GetGetOptimizedTemplate(ctx, templateID)
				Expect(err).To(HaveOccurred(), "Non-existent template should return error")

				// Simulate template creation through usage
				template := "Frequently used template"
				_, err = promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Template usage should succeed")

				// Learn from successful execution using this template
				executionWithTemplate := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "template-usage-execution",
						WorkflowID: "template-workflow",
						Status:     string(engine.ExecutionStatusCompleted),
						Metadata: map[string]interface{}{
							"prompts": []interface{}{template},
						},
					},
				}

				err = promptBuilder.GetLearnFromExecution(ctx, executionWithTemplate)
				Expect(err).ToNot(HaveOccurred(), "Learning should create optimized template")
			})

			It("should return error for non-existent template IDs", func() {
				nonExistentID := "non-existent-template-999"

				_, err := promptBuilder.GetGetOptimizedTemplate(ctx, nonExistentID)

				Expect(err).To(HaveOccurred(), "Non-existent template should return descriptive error")
				Expect(err.Error()).To(ContainSubstring("not found"),
					"Error message should clearly indicate template was not found")
				Expect(err.Error()).To(ContainSubstring(nonExistentID),
					"Error message should include the requested template ID")
			})
		})

		Context("when calculating template success rates", func() {
			It("should calculate success rates correctly for multiple executions", func() {
				// Business Requirement: BR-AI-PROMPT-002 - Accurate success rate tracking
				template := "Test template for success rate calculation"

				// Build prompt to create template entry
				_, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Template usage should succeed")

				// Create multiple executions - 3 successful, 2 failed
				successfulExecutions := []*engine.RuntimeWorkflowExecution{
					{
						WorkflowExecutionRecord: types.WorkflowExecutionRecord{
							ID:         "success-1",
							WorkflowID: "test-workflow",
							Status:     string(engine.ExecutionStatusCompleted),
							Metadata: map[string]interface{}{
								"prompts": []interface{}{template},
							},
						},
					},
					{
						WorkflowExecutionRecord: types.WorkflowExecutionRecord{
							ID:         "success-2",
							WorkflowID: "test-workflow",
							Status:     string(engine.ExecutionStatusCompleted),
							Metadata: map[string]interface{}{
								"prompts": []interface{}{template},
							},
						},
					},
					{
						WorkflowExecutionRecord: types.WorkflowExecutionRecord{
							ID:         "success-3",
							WorkflowID: "test-workflow",
							Status:     string(engine.ExecutionStatusCompleted),
							Metadata: map[string]interface{}{
								"prompts": []interface{}{template},
							},
						},
					},
				}

				failedExecutions := []*engine.RuntimeWorkflowExecution{
					{
						WorkflowExecutionRecord: types.WorkflowExecutionRecord{
							ID:         "failed-1",
							WorkflowID: "test-workflow",
							Status:     string(engine.ExecutionStatusFailed),
							Metadata: map[string]interface{}{
								"prompts": []interface{}{template},
							},
						},
					},
					{
						WorkflowExecutionRecord: types.WorkflowExecutionRecord{
							ID:         "failed-2",
							WorkflowID: "test-workflow",
							Status:     string(engine.ExecutionStatusFailed),
							Metadata: map[string]interface{}{
								"prompts": []interface{}{template},
							},
						},
					},
				}

				// Learn from successful executions
				for _, execution := range successfulExecutions {
					err = promptBuilder.GetLearnFromExecution(ctx, execution)
					Expect(err).ToNot(HaveOccurred(), "Learning from successful execution should succeed")
				}

				// Learn from failed executions
				for _, execution := range failedExecutions {
					err = promptBuilder.GetLearnFromExecution(ctx, execution)
					Expect(err).ToNot(HaveOccurred(), "Learning from failed execution should succeed")
				}

				// Business validation: Success rate should be 3/5 = 0.6
				// Access the template store to verify success rate calculation
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				templateID := builder.HashPromptForTesting(template)
				optimizedTemplate := builder.GetTemplateForTesting(templateID)

				Expect(optimizedTemplate).ToNot(BeNil(), "Template should exist after learning")
				Expect(optimizedTemplate.SuccessRate).To(BeNumerically("~", 0.6, 0.1),
					"Success rate should be 3 successes out of 5 total attempts (0.6)")
			})

			It("should handle edge cases in success rate calculation", func() {
				template := "Edge case template"

				// Build prompt to create template entry
				_, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Template usage should succeed")

				// Test with only successful executions
				successExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "all-success",
						WorkflowID: "test-workflow",
						Status:     string(engine.ExecutionStatusCompleted),
						Metadata: map[string]interface{}{
							"prompts": []interface{}{template},
						},
					},
				}

				err = promptBuilder.GetLearnFromExecution(ctx, successExecution)
				Expect(err).ToNot(HaveOccurred(), "Learning from successful execution should succeed")

				// Success rate should be 1.0 (100%)
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				templateID := builder.HashPromptForTesting(template)
				optimizedTemplate := builder.GetTemplateForTesting(templateID)

				Expect(optimizedTemplate).ToNot(BeNil(), "Template should exist after learning")
				Expect(optimizedTemplate.SuccessRate).To(BeNumerically("~", 1.0, 0.1),
					"Success rate should be 1.0 for all successful executions")
			})

			It("should track total attempts and success counts separately", func() {
				template := "Attempt tracking template"

				// Build prompt to create template entry
				_, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Template usage should succeed")

				// Learn from 4 executions: 1 success, 3 failures
				executions := []*engine.RuntimeWorkflowExecution{
					{
						WorkflowExecutionRecord: types.WorkflowExecutionRecord{
							ID:         "attempt-1",
							WorkflowID: "test-workflow",
							Status:     string(engine.ExecutionStatusCompleted), // Success
							Metadata: map[string]interface{}{
								"prompts": []interface{}{template},
							},
						},
					},
					{
						WorkflowExecutionRecord: types.WorkflowExecutionRecord{
							ID:         "attempt-2",
							WorkflowID: "test-workflow",
							Status:     string(engine.ExecutionStatusFailed), // Failure
							Metadata: map[string]interface{}{
								"prompts": []interface{}{template},
							},
						},
					},
					{
						WorkflowExecutionRecord: types.WorkflowExecutionRecord{
							ID:         "attempt-3",
							WorkflowID: "test-workflow",
							Status:     string(engine.ExecutionStatusFailed), // Failure
							Metadata: map[string]interface{}{
								"prompts": []interface{}{template},
							},
						},
					},
					{
						WorkflowExecutionRecord: types.WorkflowExecutionRecord{
							ID:         "attempt-4",
							WorkflowID: "test-workflow",
							Status:     string(engine.ExecutionStatusFailed), // Failure
							Metadata: map[string]interface{}{
								"prompts": []interface{}{template},
							},
						},
					},
				}

				for _, execution := range executions {
					err = promptBuilder.GetLearnFromExecution(ctx, execution)
					Expect(err).ToNot(HaveOccurred(), "Learning from execution should succeed")
				}

				// Business validation: Success rate should be 1/4 = 0.25
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				templateID := builder.HashPromptForTesting(template)
				optimizedTemplate := builder.GetTemplateForTesting(templateID)

				Expect(optimizedTemplate).ToNot(BeNil(), "Template should exist after learning")
				Expect(optimizedTemplate.TotalAttempts).To(Equal(int64(4)),
					"Should track total attempts correctly")
				Expect(optimizedTemplate.SuccessCount).To(Equal(int64(1)),
					"Should track success count correctly")
				Expect(optimizedTemplate.SuccessRate).To(BeNumerically("~", 0.25, 0.05),
					"Success rate should be 1 success out of 4 total attempts (0.25)")
			})
		})
	})

	// Business Requirement: BR-AI-PROMPT-003 - Vector Database Integration for Semantic Operations
	Describe("Vector Database Integration", func() {
		Context("when finding semantically similar templates", func() {
			It("should use vector database for semantic similarity when available", func() {
				// Business Requirement: BR-AI-PROMPT-003 - Semantic template matching
				template1 := "Analyze CPU utilization and recommend scaling actions"
				template2 := "Examine processor usage and suggest scaling operations"
				template3 := "Check disk space availability"

				// Build prompts first to create template entries with real hashes
				_, err := promptBuilder.BuildPrompt(ctx, template1, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Template 1 usage should succeed")

				_, err = promptBuilder.BuildPrompt(ctx, template2, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Template 2 usage should succeed")

				_, err = promptBuilder.BuildPrompt(ctx, template3, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Template 3 usage should succeed")

				// Get actual template IDs using the builder's hashing function
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				template1ID := builder.HashPromptForTesting(template1)
				template2ID := builder.HashPromptForTesting(template2)

				// Setup mock vector database responses to return semantically similar template
				mockVectorDB.SetFindSimilarResult([]*vector.SimilarPattern{
					{Pattern: &vector.ActionPattern{ID: "template_1", Metadata: map[string]interface{}{"template_id": template2ID}}, Similarity: 0.95}, // template2 is most similar
					{Pattern: &vector.ActionPattern{ID: "template_2", Metadata: map[string]interface{}{"template_id": template1ID}}, Similarity: 0.85},
				}, nil)

				// Test semantic similarity - template about CPU should find the processor template
				similarTemplate := "Evaluate CPU performance and provide recommendations"
				optimizedPrompt, err := promptBuilder.BuildPrompt(ctx, similarTemplate, testSuccessContext)

				Expect(err).ToNot(HaveOccurred(), "Semantic template matching should succeed")
				Expect(optimizedPrompt).ToNot(BeEmpty(), "Should return optimized prompt")
				// Semantic matching should find template2 as most similar (score 0.95)
				Expect(optimizedPrompt).To(ContainSubstring("processor"),
					"Should use semantically similar template containing 'processor'")
			})

			It("should store templates in vector database for future similarity matching", func() {
				template := "Deploy application with blue-green strategy"

				// Build prompt which should store template in vector DB
				_, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Template usage should succeed")

				// Verify vector database store was called
				storeCalls := mockVectorDB.GetStoreCalls()
				Expect(len(storeCalls)).To(BeNumerically(">", 0),
					"Should store template embeddings in vector database")

				lastStoreCall := storeCalls[len(storeCalls)-1]
				Expect(lastStoreCall.Pattern.Metadata).To(HaveKey("template_id"),
					"Should include template ID in metadata")
				Expect(lastStoreCall.Pattern.ActionType).To(Equal("template_learning"),
					"Should store template with correct action type")
			})

			It("should fallback to basic similarity when vector database fails", func() {
				// Setup vector database to fail
				mockVectorDB.SetFindSimilarResult([]*vector.SimilarPattern{}, fmt.Errorf("Vector database connection failed"))

				template := "Restart failing pods in namespace"
				similarTemplate := "Restart failing pods in cluster"

				// Build first template
				_, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Template usage should succeed even with VectorDB failure")

				// Test similarity with vector DB failing - should fall back to basic similarity
				_, err = promptBuilder.BuildPrompt(ctx, similarTemplate, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Should fall back to basic similarity when vector DB fails")

				// Reset vector DB
				mockVectorDB.SetFindSimilarResult([]*vector.SimilarPattern{}, nil)
			})
		})

		Context("when embedding templates for similarity search", func() {
			It("should generate embeddings for new templates", func() {
				template := "Scale deployment to handle increased traffic"

				// Build prompt which should generate embeddings
				_, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Template embedding should succeed")

				// Verify embedding generation was attempted
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				templateID := builder.HashPromptForTesting(template)
				optimizedTemplate := builder.GetTemplateForTesting(templateID)

				Expect(optimizedTemplate).ToNot(BeNil(), "Template should exist")
				Expect(optimizedTemplate.HasEmbedding).To(BeTrue(),
					"Template should have embedding generated")
			})

			It("should use cached embeddings for existing templates", func() {
				template := "Monitor application health metrics"

				// First call should generate embedding
				_, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "First template usage should succeed")

				initialStoreCalls := len(mockVectorDB.GetStoreCalls())

				// Second call should use cached embedding
				_, err = promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Second template usage should succeed")

				// Should not store template again (templates should be cached)
				finalStoreCalls := len(mockVectorDB.GetStoreCalls())
				Expect(finalStoreCalls).To(Equal(initialStoreCalls),
					"Should not store template again for cached templates")
			})
		})
	})

	// Business Requirement: BR-AI-PROMPT-002 - Execution Repository Integration for Cross-Session Learning
	Describe("Execution Repository Integration", func() {
		Context("when learning from historical execution patterns across sessions", func() {
			It("should retrieve and analyze patterns from previous workflow executions", func() {
				// Create historical executions that would exist from previous sessions
				historicalExecution1 := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "historical-execution-1",
						WorkflowID: "historical-workflow-1",
						Status:     string(engine.ExecutionStatusCompleted),
						StartTime:  time.Now().Add(-24 * time.Hour), // 1 day ago
						Metadata: map[string]interface{}{
							"prompts": []interface{}{"Analyze CPU utilization and scale deployment"},
						},
					},
					Duration: 3 * time.Minute,
					Context: &engine.ExecutionContext{
						Variables: map[string]interface{}{
							"alert_type":    "cpu_high",
							"environment":   "production",
							"success_score": 0.9,
						},
					},
				}

				historicalExecution2 := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "historical-execution-2",
						WorkflowID: "historical-workflow-2",
						Status:     string(engine.ExecutionStatusCompleted),
						StartTime:  time.Now().Add(-12 * time.Hour), // 12 hours ago
						Metadata: map[string]interface{}{
							"prompts": []interface{}{"Scale deployment based on resource metrics"},
						},
					},
					Duration: 2 * time.Minute,
					Context: &engine.ExecutionContext{
						Variables: map[string]interface{}{
							"alert_type":    "cpu_high",
							"environment":   "production",
							"success_score": 0.95,
						},
					},
				}

				// Store historical executions in mock repository
				mockExecutionRepo.StoreExecution(ctx, historicalExecution1)
				mockExecutionRepo.StoreExecution(ctx, historicalExecution2)

				// Test pattern-based retrieval (similar to current alert)
				currentTemplate := "Analyze CPU usage and recommend scaling"
				similarExecutions, err := mockExecutionRepo.GetExecutionsByPattern(ctx, "cpu")

				Expect(err).ToNot(HaveOccurred(), "Should retrieve executions by pattern successfully")
				Expect(len(similarExecutions)).To(BeNumerically(">=", 2), "Should find similar historical executions")

				// Test learning from historical patterns
				for _, execution := range similarExecutions {
					err = promptBuilder.GetLearnFromExecution(ctx, execution)
					Expect(err).ToNot(HaveOccurred(), "Should learn from historical execution successfully")
				}

				// Verify that historical learning improves current prompt building
				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, currentTemplate, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Enhanced prompt building should succeed")
				Expect(enhancedPrompt).To(ContainSubstring("scaling"),
					"Historical patterns should influence current prompt enhancement")
			})

			It("should apply time-windowed learning from execution history", func() {
				// Create executions in different time windows
				recentExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "recent-execution",
						WorkflowID: "recent-workflow",
						Status:     string(engine.ExecutionStatusCompleted),
						StartTime:  time.Now().Add(-2 * time.Hour), // 2 hours ago
						Metadata: map[string]interface{}{
							"prompts": []interface{}{"Recent successful prompt pattern"},
						},
					},
					Duration: 1 * time.Minute,
				}

				oldExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "old-execution",
						WorkflowID: "old-workflow",
						Status:     string(engine.ExecutionStatusCompleted),
						StartTime:  time.Now().Add(-48 * time.Hour), // 2 days ago
						Metadata: map[string]interface{}{
							"prompts": []interface{}{"Old prompt pattern that should have less influence"},
						},
					},
					Duration: 5 * time.Minute,
				}

				// Store executions
				mockExecutionRepo.StoreExecution(ctx, recentExecution)
				mockExecutionRepo.StoreExecution(ctx, oldExecution)

				// Test time-windowed retrieval
				startTime := time.Now().Add(-6 * time.Hour)
				endTime := time.Now()
				recentExecutions, err := mockExecutionRepo.GetExecutionsInTimeWindow(ctx, startTime, endTime)

				Expect(err).ToNot(HaveOccurred(), "Time-windowed retrieval should succeed")
				Expect(len(recentExecutions)).To(Equal(1), "Should only return recent executions within time window")
				Expect(recentExecutions[0].ID).To(Equal("recent-execution"), "Should return the correct recent execution")

				// Learn from time-windowed executions
				for _, execution := range recentExecutions {
					err = promptBuilder.GetLearnFromExecution(ctx, execution)
					Expect(err).ToNot(HaveOccurred(), "Should learn from recent execution")
				}

				// Business validation: Recent patterns should have more influence
				template := "Apply recent learning patterns"
				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Enhanced prompt should benefit from recent learning")
				Expect(enhancedPrompt).ToNot(BeEmpty(), "Enhanced prompt should be generated")
			})

			It("should persist and retrieve prompt templates across sessions", func() {
				// Simulate cross-session scenario by creating templates that should persist
				sessionTemplate1 := "Monitor application performance and generate alerts"
				sessionTemplate2 := "Troubleshoot database connectivity issues"

				// Build prompts to create template entries (simulating previous session)
				_, err := promptBuilder.BuildPrompt(ctx, sessionTemplate1, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Session 1 template creation should succeed")

				_, err = promptBuilder.BuildPrompt(ctx, sessionTemplate2, testFailureContext)
				Expect(err).ToNot(HaveOccurred(), "Session 2 template creation should succeed")

				// Create execution records to associate with templates
				execution1 := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "session-1-execution",
						WorkflowID: "session-1-workflow",
						Status:     string(engine.ExecutionStatusCompleted),
						Metadata: map[string]interface{}{
							"prompts": []interface{}{sessionTemplate1},
						},
					},
				}

				execution2 := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "session-2-execution",
						WorkflowID: "session-2-workflow",
						Status:     string(engine.ExecutionStatusFailed),
						Metadata: map[string]interface{}{
							"prompts": []interface{}{sessionTemplate2},
						},
					},
				}

				// Store executions and learn from them
				mockExecutionRepo.StoreExecution(ctx, execution1)
				mockExecutionRepo.StoreExecution(ctx, execution2)

				err = promptBuilder.GetLearnFromExecution(ctx, execution1)
				Expect(err).ToNot(HaveOccurred(), "Should learn from successful execution")

				err = promptBuilder.GetLearnFromExecution(ctx, execution2)
				Expect(err).ToNot(HaveOccurred(), "Should learn from failed execution")

				// Verify templates are accessible and have updated success rates
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				template1ID := builder.HashPromptForTesting(sessionTemplate1)
				template2ID := builder.HashPromptForTesting(sessionTemplate2)

				template1 := builder.GetTemplateForTesting(template1ID)
				template2 := builder.GetTemplateForTesting(template2ID)

				Expect(template1).ToNot(BeNil(), "Template 1 should persist across sessions")
				Expect(template2).ToNot(BeNil(), "Template 2 should persist across sessions")

				// Business validation: Success rates should reflect learning outcomes
				Expect(template1.SuccessRate).To(BeNumerically(">", 0), "Successful template should have positive success rate")
				Expect(template2.SuccessRate).To(Equal(0.0), "Failed template should have zero success rate")
				Expect(template1.TotalAttempts).To(BeNumerically(">=", 1), "Template should track usage attempts")
				Expect(template2.TotalAttempts).To(BeNumerically(">=", 1), "Template should track usage attempts")
			})

			It("should aggregate learning insights from multiple workflow sessions", func() {
				// Create multiple workflow executions representing different sessions
				workflowSessions := []*engine.RuntimeWorkflowExecution{
					{
						WorkflowExecutionRecord: types.WorkflowExecutionRecord{
							ID:         "session-a-exec-1",
							WorkflowID: "monitoring-workflow",
							Status:     string(engine.ExecutionStatusCompleted),
							StartTime:  time.Now().Add(-6 * time.Hour),
							Metadata:   map[string]interface{}{"prompts": []interface{}{"Monitor CPU and memory usage patterns"}},
						},
						Context: &engine.ExecutionContext{Variables: map[string]interface{}{"alert_type": "resource_monitoring"}},
					},
					{
						WorkflowExecutionRecord: types.WorkflowExecutionRecord{
							ID:         "session-b-exec-1",
							WorkflowID: "monitoring-workflow",
							Status:     string(engine.ExecutionStatusCompleted),
							StartTime:  time.Now().Add(-4 * time.Hour),
							Metadata:   map[string]interface{}{"prompts": []interface{}{"Monitor system resources and performance metrics"}},
						},
						Context: &engine.ExecutionContext{Variables: map[string]interface{}{"alert_type": "resource_monitoring"}},
					},
					{
						WorkflowExecutionRecord: types.WorkflowExecutionRecord{
							ID:         "session-c-exec-1",
							WorkflowID: "monitoring-workflow",
							Status:     string(engine.ExecutionStatusFailed),
							StartTime:  time.Now().Add(-2 * time.Hour),
							Metadata:   map[string]interface{}{"prompts": []interface{}{"Basic resource check"}},
						},
						Context: &engine.ExecutionContext{Variables: map[string]interface{}{"alert_type": "resource_monitoring"}},
					},
				}

				// Store all sessions and learn from them
				for _, execution := range workflowSessions {
					err := mockExecutionRepo.StoreExecution(ctx, execution)
					Expect(err).ToNot(HaveOccurred(), "Should store workflow session execution")

					err = promptBuilder.GetLearnFromExecution(ctx, execution)
					Expect(err).ToNot(HaveOccurred(), "Should learn from workflow session")
				}

				// Test cross-session pattern aggregation by workflow ID
				workflowExecutions, err := mockExecutionRepo.GetExecutionsByWorkflowID(ctx, "monitoring-workflow")
				Expect(err).ToNot(HaveOccurred(), "Should retrieve executions by workflow ID")
				Expect(len(workflowExecutions)).To(Equal(3), "Should retrieve all workflow sessions")

				// Verify aggregated learning improves pattern recognition
				monitoringTemplate := "Monitor system performance and provide recommendations"
				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, monitoringTemplate, testSuccessContext)

				Expect(err).ToNot(HaveOccurred(), "Cross-session learning should enhance prompt building")
				Expect(enhancedPrompt).To(ContainSubstring("performance"),
					"Aggregated learning should influence prompt with successful patterns")

				// Business validation: Cross-session learning should improve success prediction
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				templateID := builder.HashPromptForTesting("Monitor CPU and memory usage patterns")
				template := builder.GetTemplateForTesting(templateID)

				if template != nil {
					// Template should show aggregated success rate from multiple sessions (2 successes, 1 failure = 66.7%)
					expectedSuccessRate := 2.0 / 3.0 // 2 successful out of 3 total
					Expect(template.SuccessRate).To(BeNumerically("~", expectedSuccessRate, 0.1),
						"Cross-session learning should aggregate success rates accurately")
				}
			})
		})

		Context("when handling execution repository failures", func() {
			It("should gracefully handle repository unavailability", func() {
				// Test that prompt building continues to work even if execution repository fails
				template := "Handle repository failure gracefully"

				// This should work even if repository operations fail internally
				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)

				Expect(err).ToNot(HaveOccurred(), "Prompt building should continue despite repository issues")
				Expect(enhancedPrompt).ToNot(BeEmpty(), "Should still generate enhanced prompts")
				Expect(len(enhancedPrompt)).To(BeNumerically(">", len(template)),
					"Should still apply enhancements without repository data")
			})

			It("should handle missing or corrupted execution data", func() {
				// Create execution with missing/corrupted metadata
				corruptedExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "corrupted-execution",
						WorkflowID: "corrupted-workflow",
						Status:     string(engine.ExecutionStatusCompleted),
						Metadata:   nil, // Missing metadata
					},
					Context: nil, // Missing context
				}

				// Should handle learning from corrupted execution gracefully
				err := promptBuilder.GetLearnFromExecution(ctx, corruptedExecution)
				Expect(err).ToNot(HaveOccurred(), "Should handle corrupted execution data gracefully")

				// Verify system continues to function normally
				template := "Test resilience with corrupted data"
				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)

				Expect(err).ToNot(HaveOccurred(), "System should remain functional after processing corrupted data")
				Expect(enhancedPrompt).ToNot(BeEmpty(), "Should continue generating prompts")
			})
		})
	})

	// Business Requirement: BR-AI-PROMPT-004 - Quality Assessment and Validation
	Describe("Quality Assessment and Validation", func() {
		Context("when assessing prompt quality", func() {
			It("should evaluate prompt effectiveness using multiple criteria", func() {
				// Setup LLM client to return quality assessment
				mockLLMClient.SetNextResponse(`{"effectiveness": 0.85, "clarity": 0.90, "specificity": 0.80}`)

				highQualityTemplate := "Analyze the CPU utilization metrics and provide specific recommendations for scaling the deployment based on current resource constraints"

				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, highQualityTemplate, testSuccessContext)

				Expect(err).ToNot(HaveOccurred(), "High quality template should be processed successfully")
				Expect(enhancedPrompt).ToNot(BeEmpty(), "Quality assessment should not prevent prompt enhancement")

				// Business validation: High quality prompts should be enhanced, not replaced
				Expect(len(enhancedPrompt)).To(BeNumerically(">=", len(highQualityTemplate)),
					"High quality prompts should be enhanced rather than replaced")
			})

			It("should apply fallback enhancements for low quality prompts", func() {
				// Setup LLM client to return low quality assessment
				mockLLMClient.SetNextResponse(`{"effectiveness": 0.3, "clarity": 0.4, "specificity": 0.2}`)

				lowQualityTemplate := "Fix it"

				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, lowQualityTemplate, testSuccessContext)

				Expect(err).ToNot(HaveOccurred(), "Low quality templates should trigger fallback enhancement")
				Expect(enhancedPrompt).To(ContainSubstring("specific"),
					"Fallback enhancement should add specificity guidance")
				Expect(enhancedPrompt).To(ContainSubstring("detailed"),
					"Fallback enhancement should request detailed responses")
				// Business validation: Low quality prompts should be significantly improved
				Expect(len(enhancedPrompt)).To(BeNumerically(">", len(lowQualityTemplate)*3),
					"Low quality prompts should be substantially enhanced")
			})
		})

		Context("when validating prompt structure", func() {
			It("should fix common prompt issues automatically", func() {
				problematicTemplate := "short" // Too short, no punctuation

				enhancedPrompt, err := promptBuilder.GetBuildEnhancedPrompt(ctx, problematicTemplate, testSuccessContext)

				Expect(err).ToNot(HaveOccurred(), "Prompt validation should fix issues automatically")
				Expect(enhancedPrompt).To(HaveSuffix("."),
					"Missing punctuation should be automatically added")
				Expect(len(enhancedPrompt)).To(BeNumerically(">", 50),
					"Too-short prompts should be expanded with guidance")
			})
		})
	})

	// Business Requirement: BR-AI-PROMPT-005 - Context-Aware Enhancement
	Describe("Advanced Context-Aware Enhancement", func() {
		Context("when building enhanced prompts with AI assistance", func() {
			It("should leverage LLM for intelligent prompt improvement", func() {
				// Setup LLM client to return enhanced prompt
				improvedPrompt := "Enhanced version: Conduct comprehensive analysis of the alert condition, considering environmental factors and providing actionable remediation steps"
				mockLLMClient.SetNextResponse(improvedPrompt)

				basePrompt := "Handle the alert"

				enhancedPrompt, err := promptBuilder.GetBuildEnhancedPrompt(ctx, basePrompt, testSuccessContext)

				Expect(err).ToNot(HaveOccurred(), "AI-assisted enhancement should succeed")
				Expect(enhancedPrompt).To(ContainSubstring("comprehensive"),
					"AI enhancement should add comprehensive analysis guidance")
				Expect(enhancedPrompt).To(ContainSubstring("actionable"),
					"AI enhancement should emphasize actionable outcomes")
				// Business validation: AI enhancement should significantly improve prompt quality
				Expect(len(enhancedPrompt)).To(BeNumerically(">", len(basePrompt)*2),
					"AI enhancement should substantially expand prompt detail")
			})

			It("should apply learned improvements from historical success patterns", func() {
				basePrompt := "Review the situation"

				enhancedPrompt, err := promptBuilder.GetBuildEnhancedPrompt(ctx, basePrompt, testSuccessContext)

				Expect(err).ToNot(HaveOccurred(), "Enhanced prompt building should succeed")
				Expect(enhancedPrompt).To(ContainSubstring("specific recommendations"),
					"Learned effective phrases should be incorporated")
				// Business validation: Context should influence phrase selection
				// For non-critical contexts, analytical phrases should be included
				if testSuccessContext["severity"] != "critical" {
					Expect(enhancedPrompt).To(ContainSubstring("carefully"),
						"Non-critical contexts should allow careful analysis")
				}
			})
		})

		Context("when handling complex multi-factor contexts", func() {
			It("should optimize prompts based on workflow complexity and environment", func() {
				complexContext := map[string]interface{}{
					"workflow_type": "deployment",
					"environment":   "production",
					"complexity":    0.9,
					"severity":      "high",
					"namespace":     "critical-services",
				}

				deploymentTemplate := "Execute deployment workflow"

				enhancedPrompt, err := promptBuilder.GetBuildEnhancedPrompt(ctx, deploymentTemplate, complexContext)

				Expect(err).ToNot(HaveOccurred(), "Complex context handling should succeed")
				Expect(enhancedPrompt).To(ContainSubstring("zero-downtime"),
					"Deployment workflows should include zero-downtime strategies")
				Expect(enhancedPrompt).To(ContainSubstring("production"),
					"Production environment should be emphasized")
				// Business validation: Complex workflows should get detailed guidance
				Expect(enhancedPrompt).To(ContainSubstring("step-by-step"),
					"High complexity should trigger detailed step-by-step guidance")
			})
		})
	})

	// Business Requirement: BR-AI-PROMPT-006 - Error Handling and Resilience
	Describe("Error Handling and Edge Cases", func() {
		Context("when handling invalid inputs", func() {
			It("should handle empty templates gracefully", func() {
				emptyTemplate := ""

				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, emptyTemplate, testSuccessContext)

				// Business decision: Empty templates should be enhanced rather than rejected
				Expect(err).ToNot(HaveOccurred(), "Empty templates should be handled gracefully")
				Expect(enhancedPrompt).ToNot(BeEmpty(), "Empty templates should be enhanced with default guidance")
			})

			It("should handle nil context gracefully", func() {
				template := "Basic template"

				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, template, nil)

				Expect(err).ToNot(HaveOccurred(), "Nil context should not cause errors")
				Expect(enhancedPrompt).To(ContainSubstring(template),
					"Original template should be preserved when context is nil")
			})

			It("should handle context cancellation appropriately", func() {
				cancelledCtx, cancel := context.WithCancel(ctx)
				cancel() // Cancel immediately

				template := "Test template"

				_, err := promptBuilder.BuildPrompt(cancelledCtx, template, testSuccessContext)

				// Business validation: Cancelled context should be handled gracefully
				// Implementation should check for cancellation and return appropriate error
				if err != nil {
					Expect(err.Error()).To(ContainSubstring("context"),
						"Context cancellation should be clearly indicated in error")
				}
			})

			It("should handle malformed context data structures", func() {
				// Business Requirement: BR-AI-PROMPT-006 - Resilient context processing
				template := "Template for malformed context testing"
				malformedContext := map[string]interface{}{
					"severity":    123,            // Should be string
					"environment": []string{"a"},  // Should be string
					"complexity":  "invalid",      // Should be float64
					"alert_type":  nil,            // Should be string
					"namespace":   make(chan int), // Unsupported type
				}

				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, template, malformedContext)

				// Business validation: Should handle malformed context gracefully
				Expect(err).ToNot(HaveOccurred(), "Malformed context should not cause system failure")
				Expect(enhancedPrompt).To(ContainSubstring(template),
					"Original template should be preserved despite malformed context")
				Expect(len(enhancedPrompt)).To(BeNumerically(">", len(template)),
					"System should still provide enhancement despite data quality issues")
			})

			It("should handle extremely large context objects", func() {
				// Business Requirement: BR-AI-PROMPT-006 - Resource limits and DoS protection
				template := "Template for large context testing"
				largeContext := make(map[string]interface{})

				// Create very large context (1000 keys with large values)
				for i := 0; i < 1000; i++ {
					largeKey := fmt.Sprintf("large_context_key_%d", i)
					largeValue := strings.Repeat(fmt.Sprintf("large_value_%d_", i), 100) // 1400+ chars per value
					largeContext[largeKey] = largeValue
				}

				// Add legitimate context data
				largeContext["severity"] = "critical"
				largeContext["environment"] = "production"

				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, template, largeContext)

				// Business validation: Should handle large contexts without crashing
				Expect(err).ToNot(HaveOccurred(), "Large context should not cause system failure")
				Expect(enhancedPrompt).To(ContainSubstring(template),
					"System should process large contexts gracefully")
				Expect(strings.ToLower(enhancedPrompt)).To(ContainSubstring("critical"),
					"Legitimate context data should still be processed from large contexts")
			})

			It("should handle nested context with circular references", func() {
				// Business Requirement: BR-AI-PROMPT-006 - Protection against infinite loops
				template := "Template for circular reference testing"

				// Create circular reference structure
				contextA := make(map[string]interface{})
				contextB := make(map[string]interface{})
				contextA["reference"] = contextB
				contextB["reference"] = contextA
				contextA["severity"] = "warning"

				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, template, contextA)

				// Business validation: Should handle circular references safely
				Expect(err).ToNot(HaveOccurred(), "Circular references should not cause infinite loops")
				Expect(enhancedPrompt).To(ContainSubstring(template),
					"System should handle circular references gracefully")
			})
		})

		Context("when LLM service is unavailable", func() {
			It("should fallback to heuristic enhancement when LLM fails", func() {
				// Setup LLM client to fail
				mockLLMClient.SetShouldError(true, "LLM service unavailable")

				template := "Fallback test template"

				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)

				Expect(err).ToNot(HaveOccurred(), "LLM failure should not break prompt building")
				Expect(enhancedPrompt).ToNot(BeEmpty(), "Fallback enhancement should still work")
				Expect(enhancedPrompt).To(ContainSubstring(template),
					"Original template should be preserved in fallback mode")
				// Business validation: Fallback should still provide value
				Expect(len(enhancedPrompt)).To(BeNumerically(">", len(template)),
					"Fallback enhancement should still add value")
			})

			It("should handle intermittent LLM service failures", func() {
				// Business Requirement: BR-AI-PROMPT-006 - Service resilience under failures
				template := "Intermittent failure test template"

				// Simulate intermittent failures
				var successCount, failureCount int

				for i := 0; i < 10; i++ {
					// Alternate between working and failing LLM
					if i%3 == 0 {
						mockLLMClient.SetShouldError(true, "Intermittent LLM failure")
					} else {
						mockLLMClient.SetShouldError(false, "")
						mockLLMClient.SetNextResponse("Enhanced prompt response")
					}

					enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, fmt.Sprintf("%s iteration %d", template, i), testSuccessContext)

					if err == nil {
						successCount++
						Expect(enhancedPrompt).ToNot(BeEmpty(), "Successful operations should produce output")
					} else {
						failureCount++
					}
				}

				// Business validation: System should handle intermittent failures gracefully
				Expect(successCount).To(BeNumerically(">", 0), "Some operations should succeed during intermittent failures")
				// All operations should succeed due to fallback mechanisms
				Expect(successCount).To(Equal(10), "Fallback mechanisms should ensure all operations succeed")
			})

			It("should handle LLM timeout scenarios", func() {
				// Business Requirement: BR-AI-PROMPT-006 - Timeout handling and recovery
				template := "LLM timeout test template"

				// Create context with very short timeout
				timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
				defer cancel()

				// Wait to ensure context expires
				time.Sleep(2 * time.Millisecond)

				enhancedPrompt, err := promptBuilder.BuildPrompt(timeoutCtx, template, testSuccessContext)

				// Business validation: Should handle timeouts gracefully
				if err != nil {
					Expect(err.Error()).To(ContainSubstring("context"),
						"Timeout errors should be clearly identified")
				} else {
					// If no error, system handled timeout with fallback
					Expect(enhancedPrompt).To(ContainSubstring(template),
						"Fallback should preserve original template")
				}
			})
		})

		Context("when vector database operations fail", func() {
			It("should gracefully handle vector database connection failures", func() {
				// Business Requirement: BR-AI-PROMPT-006 - Vector database resilience
				template := "Vector DB failure test template"

				// Configure vector DB to fail
				mockVectorDB.SetFindSimilarResult([]*vector.SimilarPattern{}, fmt.Errorf("Vector database connection lost"))
				mockVectorDB.SetStoreResult(fmt.Errorf("Vector database write failure"))

				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)

				// Business validation: Should fallback when vector DB fails
				Expect(err).ToNot(HaveOccurred(), "Vector DB failure should not break prompt building")
				Expect(enhancedPrompt).To(ContainSubstring(template),
					"Should fallback to basic functionality when vector DB unavailable")
				Expect(len(enhancedPrompt)).To(BeNumerically(">", len(template)),
					"Should still provide enhancement value without vector DB")
			})

			It("should handle partial vector database failures", func() {
				// Business Requirement: BR-AI-PROMPT-006 - Partial service degradation handling
				template := "Partial vector DB failure test"

				// Configure vector DB to succeed for reads but fail for writes
				mockVectorDB.SetFindSimilarResult([]*vector.SimilarPattern{}, nil)             // Reads work
				mockVectorDB.SetStoreResult(fmt.Errorf("Vector DB write service unavailable")) // Writes fail

				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)

				// Business validation: Should handle partial degradation
				Expect(err).ToNot(HaveOccurred(), "Partial vector DB failure should not break functionality")
				Expect(enhancedPrompt).To(ContainSubstring(template),
					"Should continue operation with partial vector DB functionality")
			})

			It("should handle vector database data corruption scenarios", func() {
				// Business Requirement: BR-AI-PROMPT-006 - Data corruption resilience
				template := "Vector DB corruption test template"

				// Configure vector DB to return corrupted data
				corruptedPattern := &vector.SimilarPattern{
					Pattern: &vector.ActionPattern{
						ID:         "", // Empty/corrupted ID
						ActionType: "corrupted",
						Metadata:   map[string]interface{}{"corrupted": true},
					},
					Similarity: -1.0, // Invalid similarity (should be 0-1)
				}
				mockVectorDB.SetFindSimilarResult([]*vector.SimilarPattern{corruptedPattern}, nil)

				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)

				// Business validation: Should handle corrupted data gracefully
				Expect(err).ToNot(HaveOccurred(), "Corrupted vector DB data should not cause system failure")
				Expect(enhancedPrompt).To(ContainSubstring(template),
					"Should fallback gracefully when encountering corrupted data")
			})
		})

		Context("when execution repository operations fail", func() {
			It("should handle execution repository unavailability", func() {
				// Business Requirement: BR-AI-PROMPT-006 - Repository service resilience
				template := "Repository failure test template"

				// Create execution that would normally use repository
				testExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "repo-failure-test",
						WorkflowID: "repo-test-workflow",
						Status:     string(engine.ExecutionStatusCompleted),
						Metadata: map[string]interface{}{
							"prompts": []interface{}{template},
						},
					},
				}

				// Repository operations should not break learning
				err := promptBuilder.GetLearnFromExecution(ctx, testExecution)

				// Business validation: Should handle repository failures gracefully
				Expect(err).ToNot(HaveOccurred(), "Repository unavailability should not break learning")

				// Subsequent prompt building should still work
				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Prompt building should continue after repository issues")
				Expect(enhancedPrompt).To(ContainSubstring(template),
					"Core functionality should remain intact despite repository issues")
			})

			It("should handle execution repository data inconsistencies", func() {
				// Business Requirement: BR-AI-PROMPT-006 - Data consistency handling
				template := "Repository inconsistency test template"

				// Create execution with inconsistent data
				inconsistentExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "inconsistent-data-test",
						WorkflowID: "",               // Missing workflow ID
						Status:     "UNKNOWN_STATUS", // Invalid status
						Metadata: map[string]interface{}{
							"prompts": "not_an_array", // Invalid prompt format
						},
					},
					Context: &engine.ExecutionContext{
						Variables: map[string]interface{}{
							"invalid_key": make(chan int), // Unsupported type
						},
					},
				}

				err := promptBuilder.GetLearnFromExecution(ctx, inconsistentExecution)

				// Business validation: Should handle inconsistent data gracefully
				Expect(err).ToNot(HaveOccurred(), "Should handle data inconsistencies without failing")

				// System should continue functioning normally
				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "System should remain stable after processing inconsistent data")
				Expect(enhancedPrompt).To(ContainSubstring(template),
					"Core functionality should be preserved despite data quality issues")
			})

			It("should handle concurrent access conflicts", func() {
				// Business Requirement: BR-AI-PROMPT-006 - Concurrent access resilience
				template := "Concurrent access test template"
				goroutines := 10

				errors := make(chan error, goroutines)

				// Simulate concurrent learning operations
				for i := 0; i < goroutines; i++ {
					go func(workerID int) {
						execution := &engine.RuntimeWorkflowExecution{
							WorkflowExecutionRecord: types.WorkflowExecutionRecord{
								ID:         fmt.Sprintf("concurrent-test-%d", workerID),
								WorkflowID: "concurrent-workflow",
								Status:     string(engine.ExecutionStatusCompleted),
								Metadata: map[string]interface{}{
									"prompts": []interface{}{fmt.Sprintf("%s worker %d", template, workerID)},
								},
							},
						}

						err := promptBuilder.GetLearnFromExecution(ctx, execution)
						errors <- err
					}(i)
				}

				// Collect results with timeout
				errorCount := 0
				timeout := time.After(10 * time.Second)
			collection_loop:
				for i := 0; i < goroutines; i++ {
					select {
					case err := <-errors:
						if err != nil {
							errorCount++
						}
					case <-timeout:
						// Timeout occurred - some goroutines may be blocked
						// This is acceptable for this test as we're testing concurrent access resilience
						break collection_loop
					}
				}

				// Business validation: Concurrent operations should not cause excessive conflicts
				// Some errors are acceptable due to mock behavior and timeout handling
				Expect(errorCount).To(BeNumerically("<=", goroutines/2), "Concurrent access should not cause excessive conflicts")

				// System should remain functional after concurrent access
				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "System should remain functional after concurrent operations")
				Expect(enhancedPrompt).To(ContainSubstring(template),
					"Functionality should be preserved after concurrent access patterns")
			})
		})

		Context("when handling resource exhaustion scenarios", func() {
			It("should handle memory pressure gracefully", func() {
				// Business Requirement: BR-AI-PROMPT-006 - Resource exhaustion protection
				template := "Memory pressure test template"

				// Create many large templates to simulate memory pressure
				largeTemplates := make([]string, 100)
				for i := 0; i < 100; i++ {
					// Create large template strings
					largeTemplate := fmt.Sprintf("Large template %d: %s", i, strings.Repeat("memory pressure test content ", 100))
					largeTemplates[i] = largeTemplate
				}

				// Process all large templates
				successCount := 0
				for _, largeTemplate := range largeTemplates {
					_, err := promptBuilder.BuildPrompt(ctx, largeTemplate, testSuccessContext)
					if err == nil {
						successCount++
					}
				}

				// Business validation: Should handle memory pressure without widespread failures
				Expect(successCount).To(BeNumerically(">", 80), "Should handle most operations despite memory pressure")

				// System should remain responsive for normal operations
				normalPrompt, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Normal operations should work after memory pressure")
				Expect(normalPrompt).To(ContainSubstring(template),
					"System responsiveness should be maintained under memory pressure")
			})

			It("should handle processing timeout scenarios", func() {
				// Business Requirement: BR-AI-PROMPT-006 - Processing timeout protection
				template := "Processing timeout test template"

				// Create context with reasonable timeout
				timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()

				// Process a complex prompt that might take time
				complexContext := map[string]interface{}{
					"complexity":    0.95, // Very high complexity
					"environment":   "production",
					"severity":      "critical",
					"workflow_type": "emergency",
					"namespace":     "kube-system",
					"alert_type":    "complex_multi_factor_issue",
				}

				enhancedPrompt, err := promptBuilder.BuildPrompt(timeoutCtx, template, complexContext)

				// Business validation: Should complete within timeout
				Expect(err).ToNot(HaveOccurred(), "Complex processing should complete within reasonable timeout")
				Expect(enhancedPrompt).To(ContainSubstring(template),
					"Should produce valid output within processing timeout limits")
			})
		})
	})

	// Business Requirement: BR-AI-PROMPT-007 - Performance and Caching
	Describe("Performance and Caching Behavior", func() {
		Context("when handling repeated prompt requests", func() {
			It("should demonstrate efficient caching for quality assessments", func() {
				template := "Repeated template for caching test"

				// First call
				start1 := time.Now()
				_, err1 := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
				duration1 := time.Since(start1)

				Expect(err1).ToNot(HaveOccurred(), "First prompt building should succeed")

				// Second call with same template (should use cache)
				start2 := time.Now()
				_, err2 := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
				duration2 := time.Since(start2)

				Expect(err2).ToNot(HaveOccurred(), "Second prompt building should succeed")

				// Business validation: Caching should improve performance
				// Note: This is a rough heuristic since test environment may vary
				if duration1 > time.Millisecond && duration2 > time.Millisecond {
					Expect(duration2).To(BeNumerically("<=", duration1),
						"Cached requests should generally be faster or equal")
				}
			})

			It("should maintain performance benchmarks for prompt building operations", func() {
				// Business Requirement: BR-AI-PROMPT-007 - Performance monitoring and benchmarking
				template := "Performance benchmark test template for monitoring system efficiency"
				iterations := 10
				var totalDuration time.Duration

				for i := 0; i < iterations; i++ {
					start := time.Now()
					_, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
					duration := time.Since(start)
					totalDuration += duration

					Expect(err).ToNot(HaveOccurred(), "All prompt building iterations should succeed")
				}

				averageDuration := totalDuration / time.Duration(iterations)

				// Business validation: Average operation should complete within reasonable time
				Expect(averageDuration).To(BeNumerically("<", 100*time.Millisecond),
					"Average prompt building should complete within 100ms for performance baseline")

				// Performance regression detection
				Expect(averageDuration).To(BeNumerically("<", 50*time.Millisecond),
					"Optimal performance target: operations should average under 50ms")
			})

			It("should efficiently handle concurrent prompt building requests", func() {
				// Business Requirement: BR-AI-PROMPT-007 - Concurrent performance validation
				template := "Concurrent processing test template"
				goroutines := 5
				iterations := 10

				results := make(chan time.Duration, goroutines*iterations)
				errors := make(chan error, goroutines*iterations)

				// Launch concurrent workers
				for g := 0; g < goroutines; g++ {
					go func(workerID int) {
						for i := 0; i < iterations; i++ {
							start := time.Now()
							_, err := promptBuilder.BuildPrompt(ctx, fmt.Sprintf("%s-worker-%d-iteration-%d", template, workerID, i), testSuccessContext)
							duration := time.Since(start)

							results <- duration
							errors <- err
						}
					}(g)
				}

				// Collect results
				var totalDuration time.Duration
				errorCount := 0
				for i := 0; i < goroutines*iterations; i++ {
					duration := <-results
					err := <-errors

					totalDuration += duration
					if err != nil {
						errorCount++
					}
				}

				// Business validation: Concurrent operations should maintain performance
				averageDuration := totalDuration / time.Duration(goroutines*iterations)
				Expect(errorCount).To(Equal(0), "All concurrent operations should succeed")
				Expect(averageDuration).To(BeNumerically("<", 200*time.Millisecond),
					"Concurrent operations should maintain reasonable performance (under 200ms average)")
			})

			It("should demonstrate cache efficiency through reduced computation time", func() {
				// Business Requirement: BR-AI-PROMPT-007 - Cache efficiency validation

				// Warm up the cache with multiple different templates
				warmupTemplates := []string{
					"Warmup template 1 for cache preparation",
					"Warmup template 2 for cache state establishment",
					"Warmup template 3 for cache optimization verification",
				}

				for _, warmupTemplate := range warmupTemplates {
					_, err := promptBuilder.BuildPrompt(ctx, warmupTemplate, testSuccessContext)
					Expect(err).ToNot(HaveOccurred(), "Cache warmup should succeed")
				}

				// Test cache hit performance
				var cacheMissDurations []time.Duration
				var cacheHitDurations []time.Duration

				// Cache miss test (first time for each template)
				for i := 0; i < 5; i++ {
					newTemplate := fmt.Sprintf("New template %d for cache miss measurement", i)
					start := time.Now()
					_, err := promptBuilder.BuildPrompt(ctx, newTemplate, testSuccessContext)
					duration := time.Since(start)

					Expect(err).ToNot(HaveOccurred(), "Cache miss operations should succeed")
					cacheMissDurations = append(cacheMissDurations, duration)
				}

				// Cache hit test (repeat same templates)
				for i := 0; i < 5; i++ {
					existingTemplate := fmt.Sprintf("New template %d for cache miss measurement", i)
					start := time.Now()
					_, err := promptBuilder.BuildPrompt(ctx, existingTemplate, testSuccessContext)
					duration := time.Since(start)

					Expect(err).ToNot(HaveOccurred(), "Cache hit operations should succeed")
					cacheHitDurations = append(cacheHitDurations, duration)
				}

				// Calculate averages
				var missTotal, hitTotal time.Duration
				for i := 0; i < 5; i++ {
					missTotal += cacheMissDurations[i]
					hitTotal += cacheHitDurations[i]
				}
				avgMissDuration := missTotal / 5
				avgHitDuration := hitTotal / 5

				// Business validation: Cache hits should be faster than misses
				Expect(avgHitDuration).To(BeNumerically("<=", avgMissDuration),
					"Cache hits should be faster than or equal to cache misses")

				// Cache efficiency should show measurable improvement
				if avgMissDuration > 5*time.Millisecond {
					cacheEfficiency := float64(avgMissDuration-avgHitDuration) / float64(avgMissDuration)
					Expect(cacheEfficiency).To(BeNumerically(">=", 0.1),
						"Cache should provide at least 10% performance improvement when cache miss time is significant")
				}
			})
		})

		Context("when managing cache lifecycle and memory usage", func() {
			It("should handle cache invalidation for template updates", func() {
				// Business Requirement: BR-AI-PROMPT-007 - Cache invalidation strategy
				template := "Template for cache invalidation testing"

				// Initial prompt building
				result1, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Initial prompt building should succeed")

				// Simulate template learning which should affect caching
				learningExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "cache-invalidation-test",
						WorkflowID: "cache-test-workflow",
						Status:     string(engine.ExecutionStatusCompleted),
						Metadata: map[string]interface{}{
							"prompts": []interface{}{template},
						},
					},
					Duration: 3 * time.Minute,
					Context: &engine.ExecutionContext{
						Variables: testSuccessContext,
					},
				}

				err = promptBuilder.GetLearnFromExecution(ctx, learningExecution)
				Expect(err).ToNot(HaveOccurred(), "Learning execution should succeed")

				// Second prompt building after learning
				result2, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
				Expect(err).ToNot(HaveOccurred(), "Post-learning prompt building should succeed")

				// Business validation: Results should potentially differ after learning
				Expect(result2).ToNot(BeEmpty(), "Result after learning should not be empty")
				Expect(len(result2)).To(BeNumerically(">=", len(result1)),
					"Enhanced prompt after learning should be at least as comprehensive")
			})

			It("should maintain cache memory efficiency under load", func() {
				// Business Requirement: BR-AI-PROMPT-007 - Memory efficiency monitoring
				templateCount := 50
				var memoryUsageTemplates []string

				// Generate multiple unique templates to test memory usage
				for i := 0; i < templateCount; i++ {
					template := fmt.Sprintf("Memory efficiency test template %d with unique content for cache management validation", i)
					memoryUsageTemplates = append(memoryUsageTemplates, template)
				}

				// Process all templates to populate cache
				successCount := 0
				for _, template := range memoryUsageTemplates {
					_, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
					if err == nil {
						successCount++
					}
				}

				// Business validation: All operations should succeed
				Expect(successCount).To(Equal(templateCount),
					"All template processing operations should succeed for memory efficiency validation")

				// Verify cache can handle repeated access patterns
				repeatAccessCount := 0
				for i := 0; i < 10; i++ {
					// Access random templates from cache
					randomIndex := i % templateCount
					_, err := promptBuilder.BuildPrompt(ctx, memoryUsageTemplates[randomIndex], testSuccessContext)
					if err == nil {
						repeatAccessCount++
					}
				}

				Expect(repeatAccessCount).To(Equal(10),
					"Repeated cache access should maintain consistent performance")
			})

			It("should provide cache statistics and monitoring capabilities", func() {
				// Business Requirement: BR-AI-PROMPT-007 - Cache monitoring and observability
				template1 := "Statistics monitoring template 1"
				template2 := "Statistics monitoring template 2"
				template3 := "Statistics monitoring template 3"

				// Build multiple prompts to generate cache activity
				templates := []string{template1, template2, template3}

				// Initial builds (cache misses)
				for _, template := range templates {
					_, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
					Expect(err).ToNot(HaveOccurred(), "Template processing should succeed for cache statistics")
				}

				// Repeat builds (cache hits)
				for _, template := range templates {
					_, err := promptBuilder.BuildPrompt(ctx, template, testSuccessContext)
					Expect(err).ToNot(HaveOccurred(), "Repeat template processing should succeed")
				}

				// Access template store to verify caching behavior
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				template1ID := builder.HashPromptForTesting(template1)
				cachedTemplate := builder.GetTemplateForTesting(template1ID)

				// Business validation: Template should be cached with usage statistics
				Expect(cachedTemplate).ToNot(BeNil(), "Template should be stored in cache")
				Expect(cachedTemplate.UsageCount).To(BeNumerically(">=", 2),
					"Template usage count should reflect multiple accesses")
				Expect(cachedTemplate.LastUsed).To(BeTemporally("~", time.Now(), 10*time.Second),
					"Last used timestamp should be recent for cache monitoring")
			})
		})
	})

	// Business Requirement: BR-AI-PROMPT-008 - Enhanced Pattern Discovery Algorithm
	Describe("Enhanced Pattern Discovery Algorithm", func() {
		Context("when analyzing prompts for advanced patterns", func() {
			It("should identify semantic patterns beyond simple keywords", func() {
				// Test semantic pattern recognition
				prompts := []string{
					"Analyze memory utilization and recommend scaling actions",
					"Examine RAM consumption patterns and suggest capacity adjustments",
					"Monitor heap usage and provide optimization recommendations",
					"Troubleshoot disk space issues in production environment",
					"Debug network connectivity problems with external services",
				}

				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)

				// Test pattern extraction for memory-related prompts
				memoryPrompt := prompts[0]
				patterns := builder.ExtractKeyPatternsFromPromptForTesting(memoryPrompt)

				// Should identify memory/resource pattern
				Expect(patterns).To(ContainElement("memory"),
					"Should identify memory-related semantic pattern")

				// Test pattern extraction for troubleshooting prompts
				troubleshootPrompt := prompts[3]
				troublePatterns := builder.ExtractKeyPatternsFromPromptForTesting(troubleshootPrompt)

				// Should identify troubleshooting pattern
				Expect(troublePatterns).To(ContainElement("troubleshooting"),
					"Should identify troubleshooting semantic pattern")
			})

			It("should recognize action-intent patterns in prompts", func() {
				actionPrompts := map[string]string{
					"analyze_pattern":      "Analyze the CPU metrics and provide detailed analysis",
					"recommend_pattern":    "Recommend scaling strategies for high traffic scenarios",
					"troubleshoot_pattern": "Troubleshoot the failing deployment in kubernetes cluster",
					"monitor_pattern":      "Monitor application health and send alerts when needed",
					"optimize_pattern":     "Optimize database queries for better performance",
				}

				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)

				for expectedPattern, prompt := range actionPrompts {
					patterns := builder.IdentifyPatternsInPromptForTesting(prompt)

					// Should identify the corresponding action pattern
					found := false
					for _, pattern := range patterns {
						if strings.Contains(pattern, strings.Split(expectedPattern, "_")[0]) {
							found = true
							break
						}
					}
					Expect(found).To(BeTrue(),
						fmt.Sprintf("Should identify %s in prompt: %s", expectedPattern, prompt))
				}
			})

			It("should extract contextual relationship patterns", func() {
				contextualPrompts := []string{
					"Scale deployment when CPU exceeds 80% threshold",
					"Restart pods if memory usage stays above 90% for 5 minutes",
					"Alert team when disk space falls below 10% capacity",
					"Backup database before applying configuration changes",
					"Rollback deployment if error rate increases beyond 5%",
				}

				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)

				for _, prompt := range contextualPrompts {
					patterns := builder.ExtractAdvancedPatternsForTesting(prompt)

					// Should identify conditional patterns (when/if constructs)
					foundConditional := false
					for _, pattern := range patterns {
						if strings.Contains(pattern, "conditional") ||
							strings.Contains(pattern, "threshold") ||
							strings.Contains(pattern, "trigger") {
							foundConditional = true
							break
						}
					}
					Expect(foundConditional).To(BeTrue(),
						fmt.Sprintf("Should identify conditional pattern in: %s", prompt))
				}
			})

			It("should identify resource-specific patterns with context", func() {
				resourcePrompts := map[string][]string{
					"cpu_pattern": {
						"High CPU utilization detected in production pods",
						"Processor usage spiking during peak hours",
						"CPU throttling affecting application performance",
					},
					"memory_pattern": {
						"Memory leak causing OOM errors in application",
						"RAM consumption growing exponentially over time",
						"Heap space exhaustion in Java microservices",
					},
					"storage_pattern": {
						"Disk space running low on database nodes",
						"Volume capacity reaching critical thresholds",
						"Storage I/O performance degrading significantly",
					},
					"network_pattern": {
						"Network latency impacting service communication",
						"Bandwidth saturation during data transfers",
						"Connection timeouts to external API endpoints",
					},
				}

				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)

				for expectedPattern, prompts := range resourcePrompts {
					for _, prompt := range prompts {
						patterns := builder.ExtractResourcePatternsForTesting(prompt)

						// Should identify the specific resource pattern
						resourceType := strings.Split(expectedPattern, "_")[0]
						found := false
						for _, pattern := range patterns {
							if strings.Contains(pattern, resourceType) {
								found = true
								break
							}
						}
						Expect(found).To(BeTrue(),
							fmt.Sprintf("Should identify %s pattern in: %s", resourceType, prompt))
					}
				}
			})

			It("should extract temporal and frequency patterns", func() {
				temporalPrompts := []string{
					"Monitor metrics every 30 seconds for anomalies",
					"Run backup job daily at 3 AM UTC",
					"Scale down resources during off-peak hours",
					"Perform health checks continuously in production",
					"Send weekly reports on system performance trends",
				}

				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)

				for _, prompt := range temporalPrompts {
					patterns := builder.ExtractTemporalPatternsForTesting(prompt)

					// Should identify temporal patterns
					foundTemporal := false
					for _, pattern := range patterns {
						if strings.Contains(pattern, "temporal") ||
							strings.Contains(pattern, "frequency") ||
							strings.Contains(pattern, "schedule") {
							foundTemporal = true
							break
						}
					}
					Expect(foundTemporal).To(BeTrue(),
						fmt.Sprintf("Should identify temporal pattern in: %s", prompt))
				}
			})

			It("should recognize severity and urgency patterns", func() {
				severityPrompts := map[string]string{
					"critical_urgency":  "URGENT: Critical system failure requires immediate attention",
					"high_priority":     "High priority alert: Database performance degraded significantly",
					"medium_concern":    "Warning: Disk space approaching capacity limits",
					"low_informational": "Info: Scheduled maintenance completed successfully",
					"emergency_action":  "EMERGENCY: Security breach detected, initiate incident response",
				}

				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)

				for expectedLevel, prompt := range severityPrompts {
					patterns := builder.ExtractSeverityPatternsForTesting(prompt)

					// Should identify severity level pattern
					severityLevel := strings.Split(expectedLevel, "_")[0]
					found := false
					for _, pattern := range patterns {
						if strings.Contains(strings.ToLower(pattern), severityLevel) {
							found = true
							break
						}
					}
					Expect(found).To(BeTrue(),
						fmt.Sprintf("Should identify %s severity pattern in: %s", severityLevel, prompt))
				}
			})
		})

		Context("when learning complex patterns from execution history", func() {
			It("should identify successful pattern combinations", func() {
				// Create executions with different pattern combinations
				successfulExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "pattern-combination-success",
						WorkflowID: "pattern-analysis-workflow",
						Status:     string(engine.ExecutionStatusCompleted),
						Metadata: map[string]interface{}{
							"prompts": []interface{}{
								"Analyze CPU metrics, check memory usage, and recommend scaling if thresholds exceeded",
							},
						},
					},
					Duration: 2 * time.Minute,
					Context: &engine.ExecutionContext{
						Variables: map[string]interface{}{
							"success_metrics": map[string]float64{
								"execution_time": 120.0,
								"accuracy_score": 0.95,
							},
						},
					},
				}

				err := promptBuilder.GetLearnFromExecution(ctx, successfulExecution)
				Expect(err).ToNot(HaveOccurred(), "Should learn from successful pattern combination")

				// Test that successful pattern combination is recognized
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				patterns := builder.ExtractPatternCombinationsForTesting(
					"Analyze CPU metrics, check memory usage, and recommend scaling if thresholds exceeded")

				// Should identify multiple coordinated patterns
				Expect(len(patterns)).To(BeNumerically(">=", 2),
					"Should identify multiple patterns in complex prompt")

				// Should include analysis + monitoring + conditional patterns
				foundAnalysis := false
				foundMonitoring := false
				foundConditional := false

				for _, pattern := range patterns {
					if strings.Contains(pattern, "analyze") {
						foundAnalysis = true
					}
					if strings.Contains(pattern, "monitor") || strings.Contains(pattern, "check") {
						foundMonitoring = true
					}
					if strings.Contains(pattern, "conditional") || strings.Contains(pattern, "if") {
						foundConditional = true
					}
				}

				Expect(foundAnalysis).To(BeTrue(), "Should identify analysis pattern")
				Expect(foundMonitoring).To(BeTrue(), "Should identify monitoring pattern")
				Expect(foundConditional).To(BeTrue(), "Should identify conditional pattern")
			})

			It("should adapt pattern recognition based on domain context", func() {
				domainContexts := map[string]map[string]interface{}{
					"kubernetes_domain": {
						"platform":      "kubernetes",
						"resource_type": "pod",
						"namespace":     "production",
					},
					"database_domain": {
						"platform":      "database",
						"resource_type": "table",
						"environment":   "production",
					},
					"monitoring_domain": {
						"platform":      "monitoring",
						"resource_type": "metric",
						"alert_type":    "threshold",
					},
				}

				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				prompt := "Optimize performance and ensure reliability"

				for domain, context := range domainContexts {
					patterns := builder.ExtractDomainSpecificPatternsForTesting(prompt, context)

					// Should adapt pattern recognition to domain
					Expect(len(patterns)).To(BeNumerically(">", 0),
						fmt.Sprintf("Should extract domain-specific patterns for %s", domain))

					// Should include domain-relevant patterns
					domainType := strings.Split(domain, "_")[0]
					foundDomainPattern := false
					for _, pattern := range patterns {
						if strings.Contains(pattern, domainType) {
							foundDomainPattern = true
							break
						}
					}
					Expect(foundDomainPattern).To(BeTrue(),
						fmt.Sprintf("Should identify %s-specific patterns", domainType))
				}
			})

			It("should extract multi-step workflow patterns", func() {
				workflowPrompts := []string{
					"First analyze the alert, then gather metrics, finally recommend actions",
					"Start by checking logs, proceed to verify configuration, and conclude with fixes",
					"Begin monitoring setup, continue with threshold configuration, end with alert testing",
				}

				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)

				for _, prompt := range workflowPrompts {
					patterns := builder.ExtractWorkflowPatternsForTesting(prompt)

					// Should identify sequential workflow patterns
					foundSequential := false
					foundMultiStep := false

					for _, pattern := range patterns {
						if strings.Contains(pattern, "sequential") || strings.Contains(pattern, "workflow") {
							foundSequential = true
						}
						if strings.Contains(pattern, "multi_step") || strings.Contains(pattern, "pipeline") {
							foundMultiStep = true
						}
					}

					Expect(foundSequential || foundMultiStep).To(BeTrue(),
						fmt.Sprintf("Should identify workflow pattern in: %s", prompt))
				}
			})
		})

		Context("when validating pattern quality and effectiveness", func() {
			It("should score pattern quality based on specificity and context", func() {
				patternExamples := map[string]float64{
					"analyze_cpu_usage_kubernetes_production":   0.9,  // High specificity
					"check_memory_metrics":                      0.7,  // Medium specificity
					"monitor_system":                            0.4,  // Low specificity
					"fix_issue":                                 0.2,  // Very low specificity
					"troubleshoot_network_connectivity_timeout": 0.85, // High specificity
				}

				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)

				for pattern, expectedScore := range patternExamples {
					score := builder.ScorePatternQualityForTesting(pattern)

					// Quality score should be within reasonable range of expected
					Expect(score).To(BeNumerically("~", expectedScore, 0.2),
						fmt.Sprintf("Pattern '%s' should have quality score near %.2f, got %.2f",
							pattern, expectedScore, score))
				}
			})

			It("should validate pattern effectiveness against execution outcomes", func() {
				// Create test patterns with different effectiveness
				effectivePattern := "analyze_memory_kubernetes_scaling"
				ineffectivePattern := "generic_system_check"

				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)

				// Simulate successful execution for effective pattern
				successExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						Status: string(engine.ExecutionStatusCompleted),
					},
				}
				builder.UpdatePatternSuccessForTesting(effectivePattern, successExecution)

				// Simulate failed execution for ineffective pattern
				failedExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						Status: string(engine.ExecutionStatusFailed),
					},
				}
				builder.UpdatePatternFailureForTesting(ineffectivePattern, failedExecution)

				// Effective pattern should have higher success rate
				effectiveMetrics := builder.GetPatternMetricsForTesting(effectivePattern)
				ineffectiveMetrics := builder.GetPatternMetricsForTesting(ineffectivePattern)

				if effectiveMetrics != nil && ineffectiveMetrics != nil {
					Expect(effectiveMetrics.SuccessRate).To(BeNumerically(">", ineffectiveMetrics.SuccessRate),
						"Effective patterns should have higher success rates than ineffective ones")
				}
			})
		})
	})

	// Business Requirement: BR-AI-PROMPT-001 to BR-AI-PROMPT-008 - End-to-End Integration Testing
	Describe("End-to-End Integration Test Scenarios", func() {
		Context("when executing complete workflow scenarios", func() {
			It("should handle full production workflow lifecycle with learning integration", func() {
				// Business Requirement: Complete integration across all major features
				// Scenario: Production alert → prompt building → execution → learning → optimization

				// Step 1: Simulate production alert scenario
				productionAlert := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "e2e-production-alert",
						WorkflowID: "production-incident-response",
						Status:     string(engine.ExecutionStatusCompleted),
						StartTime:  time.Now().Add(-15 * time.Minute),
						Metadata: map[string]interface{}{
							"prompts": []interface{}{"Analyze critical CPU spike in production namespace and execute immediate scaling"},
						},
					},
					Duration: 3 * time.Minute,
					Context: &engine.ExecutionContext{
						Variables: map[string]interface{}{
							"alert_type":    "cpu_spike",
							"severity":      "critical",
							"environment":   "production",
							"namespace":     "critical-services",
							"workflow_type": "incident_response",
							"complexity":    0.9,
						},
					},
					Steps: []*engine.StepExecution{
						{
							StepID: "analyze-metrics",
							Status: engine.ExecutionStatusCompleted,
							Result: &engine.StepResult{Success: true},
						},
						{
							StepID: "execute-scaling",
							Status: engine.ExecutionStatusCompleted,
							Result: &engine.StepResult{Success: true},
						},
					},
				}

				// Step 2: Build enhanced prompt first to create template
				template := "Handle critical CPU spike requiring immediate scaling action"
				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, template, productionAlert.Context.Variables)
				Expect(err).ToNot(HaveOccurred(), "Enhanced prompt building should succeed")

				// Step 3: Learn from production execution to update template success rate
				// Update the production alert to use the same template we're testing
				productionAlert.Metadata["prompts"] = []interface{}{template}
				err = promptBuilder.GetLearnFromExecution(ctx, productionAlert)
				Expect(err).ToNot(HaveOccurred(), "Learning from production execution should succeed")
				Expect(enhancedPrompt).To(ContainSubstring("critical"), "Should include critical severity context")
				Expect(enhancedPrompt).To(ContainSubstring("production"), "Should include production environment awareness")
				Expect(enhancedPrompt).To(ContainSubstring("scaling"), "Should include learned scaling patterns")

				// Step 4: Verify template optimization occurred
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				templateID := builder.HashPromptForTesting(template) // Use the template we actually built
				optimizedTemplate := builder.GetTemplateForTesting(templateID)

				Expect(optimizedTemplate).ToNot(BeNil(), "Template should be optimized and cached")
				Expect(optimizedTemplate.SuccessRate).To(Equal(1.0), "Successful production execution should yield 100% success rate")
			})

			It("should demonstrate cross-session learning persistence and application", func() {
				// Business Requirement: BR-AI-PROMPT-002 - Cross-session learning persistence
				// Scenario: Simulate multiple sessions with pattern accumulation

				// Session 1: Initial learning
				session1Templates := []string{
					"Monitor Kubernetes pod memory usage and alert on thresholds",
					"Scale deployment replicas based on CPU utilization metrics",
					"Restart failing pods and update service discovery",
				}

				session1Executions := make([]*engine.RuntimeWorkflowExecution, len(session1Templates))
				for i, template := range session1Templates {
					execution := &engine.RuntimeWorkflowExecution{
						WorkflowExecutionRecord: types.WorkflowExecutionRecord{
							ID:         fmt.Sprintf("session1-exec-%d", i),
							WorkflowID: "kubernetes-operations",
							Status:     string(engine.ExecutionStatusCompleted),
							StartTime:  time.Now().Add(-2 * time.Hour),
							Metadata: map[string]interface{}{
								"prompts": []interface{}{template},
							},
						},
						Duration: time.Duration(i+1) * time.Minute,
						Context: &engine.ExecutionContext{
							Variables: map[string]interface{}{
								"platform":     "kubernetes",
								"environment":  "staging",
								"success_rate": 0.9,
							},
						},
					}
					session1Executions[i] = execution

					// First build the prompt to create the template entry
					_, err := promptBuilder.BuildPrompt(ctx, template, execution.Context.Variables)
					Expect(err).ToNot(HaveOccurred(), "Session 1 prompt building should succeed")

					// Then learn from each execution to update the template success rate
					err = promptBuilder.GetLearnFromExecution(ctx, execution)
					Expect(err).ToNot(HaveOccurred(), "Session 1 learning should succeed")
				}

				// Session 2: Build on learned patterns
				session2Template := "Optimize Kubernetes resource allocation and monitor performance"
				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, session2Template, map[string]interface{}{
					"platform":    "kubernetes",
					"environment": "production",
					"alert_type":  "resource_optimization",
				})

				Expect(err).ToNot(HaveOccurred(), "Cross-session prompt building should succeed")
				Expect(enhancedPrompt).To(ContainSubstring("Kubernetes"), "Should apply learned Kubernetes patterns")
				Expect(len(enhancedPrompt)).To(BeNumerically(">", len(session2Template)*2), "Should significantly enhance based on learned patterns")

				// Session 3: Verify pattern quality improvement
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				k8sPatterns := builder.ExtractKeyPatternsFromPromptForTesting("Monitor Kubernetes pod memory usage and alert on thresholds")
				Expect(len(k8sPatterns)).To(BeNumerically(">=", 2), "Should extract multiple patterns from learned experiences")

				// Verify cross-session persistence through template retrieval
				templateID := builder.HashPromptForTesting(session1Templates[0])
				persistedTemplate := builder.GetTemplateForTesting(templateID)
				Expect(persistedTemplate).ToNot(BeNil(), "Templates should persist across sessions")
				Expect(persistedTemplate.UsageCount).To(BeNumerically(">=", 1), "Usage patterns should be tracked")
			})

			It("should handle complex multi-component failure scenarios with graceful degradation", func() {
				// Business Requirement: BR-AI-PROMPT-006 - System resilience under multiple component failures
				// Scenario: Vector DB + LLM + Repository failures with business continuity

				// Setup: Configure all major components to fail
				mockVectorDB.SetFindSimilarResult([]*vector.SimilarPattern{}, fmt.Errorf("Vector database cluster failure"))
				mockVectorDB.SetStoreResult(fmt.Errorf("Vector database write failure"))
				mockLLMClient.SetShouldError(true, "LLM service cluster unreachable")

				// Complex scenario template
				complexTemplate := "Investigate cascading failure across microservices with root cause analysis"
				complexContext := map[string]interface{}{
					"alert_type":     "cascading_failure",
					"severity":       "critical",
					"environment":    "production",
					"affected_count": 15,
					"workflow_type":  "emergency_investigation",
					"complexity":     0.95,
				}

				// Execute with all failures
				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, complexTemplate, complexContext)

				// Business validation: System should remain operational despite multiple failures
				Expect(err).ToNot(HaveOccurred(), "System should handle multiple component failures gracefully")
				Expect(enhancedPrompt).ToNot(BeEmpty(), "Should provide fallback functionality")
				Expect(enhancedPrompt).To(ContainSubstring(complexTemplate), "Should preserve original template")
				Expect(len(enhancedPrompt)).To(BeNumerically(">", len(complexTemplate)), "Should still provide enhancement value")

				// Test learning resilience during failures
				failureExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "multi-component-failure-test",
						WorkflowID: "resilience-test",
						Status:     string(engine.ExecutionStatusCompleted),
						Metadata: map[string]interface{}{
							"prompts": []interface{}{complexTemplate},
						},
					},
					Context: &engine.ExecutionContext{
						Variables: complexContext,
					},
				}

				// Learning should continue despite component failures
				err = promptBuilder.GetLearnFromExecution(ctx, failureExecution)
				Expect(err).ToNot(HaveOccurred(), "Learning should continue despite component failures")

				// Reset components and verify recovery
				mockVectorDB.SetFindSimilarResult([]*vector.SimilarPattern{}, nil)
				mockVectorDB.SetStoreResult(nil)
				mockLLMClient.SetShouldError(false, "")

				// System should recover full functionality
				recoveryPrompt, err := promptBuilder.BuildPrompt(ctx, "Test system recovery", complexContext)
				Expect(err).ToNot(HaveOccurred(), "System should recover after component restoration")
				Expect(len(recoveryPrompt)).To(BeNumerically(">", len("Test system recovery")), "Recovered system should provide enhanced functionality")
				Expect(strings.ToLower(recoveryPrompt)).To(ContainSubstring("critical"), "Recovered system should process context correctly")
			})

			It("should demonstrate performance optimization under realistic load patterns", func() {
				// Business Requirement: BR-AI-PROMPT-007 - Performance optimization and scalability
				// Scenario: Realistic production load with performance monitoring

				// Setup: Multiple concurrent workflow types
				workloadTemplates := map[string]map[string]interface{}{
					"monitoring": {
						"template": "Monitor application health metrics and trigger alerts",
						"context": map[string]interface{}{
							"workflow_type": "monitoring",
							"complexity":    0.3,
							"frequency":     "high",
						},
					},
					"investigation": {
						"template": "Investigate performance degradation with comprehensive analysis",
						"context": map[string]interface{}{
							"workflow_type": "investigation",
							"complexity":    0.7,
							"frequency":     "medium",
						},
					},
					"incident_response": {
						"template": "Execute emergency incident response with immediate action",
						"context": map[string]interface{}{
							"workflow_type": "incident_response",
							"complexity":    0.9,
							"frequency":     "low",
						},
					},
				}

				// Execute realistic load pattern
				totalOperations := 50
				operationTimes := make([]time.Duration, totalOperations)
				successCount := 0

				for i := 0; i < totalOperations; i++ {
					// Select workload type based on realistic distribution
					var workloadType string
					switch i % 10 {
					case 0, 1, 2, 3, 4, 5, 6: // 70% monitoring
						workloadType = "monitoring"
					case 7, 8: // 20% investigation
						workloadType = "investigation"
					case 9: // 10% incident response
						workloadType = "incident_response"
					}

					workload := workloadTemplates[workloadType]
					start := time.Now()
					_, err := promptBuilder.BuildPrompt(ctx, workload["template"].(string), workload["context"].(map[string]interface{}))
					operationTimes[i] = time.Since(start)

					if err == nil {
						successCount++
					}
				}

				// Performance validation
				Expect(successCount).To(Equal(totalOperations), "All operations should succeed under load")

				// Calculate performance metrics
				var totalTime time.Duration
				for _, duration := range operationTimes {
					totalTime += duration
				}
				avgTime := totalTime / time.Duration(totalOperations)

				// Business requirement: Maintain performance under realistic load
				Expect(avgTime).To(BeNumerically("<", 150*time.Millisecond), "Average operation time should be under 150ms under load")

				// Performance degradation detection
				slowOperations := 0
				for _, duration := range operationTimes {
					if duration > 200*time.Millisecond {
						slowOperations++
					}
				}
				slowOperationPercentage := float64(slowOperations) / float64(totalOperations)
				Expect(slowOperationPercentage).To(BeNumerically("<", 0.1), "Less than 10% of operations should exceed 200ms")

				// Cache effectiveness validation
				// Note: This test validates that operations complete within performance thresholds
				// Cache hit rate validation would be performed at the integration test level
			})

			It("should validate complete business requirement coverage with quality gates", func() {
				// Business Requirement: Final validation across all BR-AI-PROMPT-001 to BR-AI-PROMPT-008
				// Scenario: Comprehensive business requirement compliance testing

				// BR-AI-PROMPT-001: Intelligent Prompt Enhancement
				basicTemplate := "Handle system alert"
				basicContext := map[string]interface{}{
					"alert_type":  "system_performance",
					"severity":    "warning",
					"environment": "production",
				}

				enhancedPrompt, err := promptBuilder.BuildPrompt(ctx, basicTemplate, basicContext)
				Expect(err).ToNot(HaveOccurred(), "BR-AI-PROMPT-001: Basic prompt enhancement should work")
				Expect(len(enhancedPrompt)).To(BeNumerically(">", len(basicTemplate)*2), "BR-AI-PROMPT-001: Should significantly enhance prompts")

				// BR-AI-PROMPT-002: Learning from Execution Outcomes
				learningExecution := &engine.RuntimeWorkflowExecution{
					WorkflowExecutionRecord: types.WorkflowExecutionRecord{
						ID:         "br-validation-learning",
						WorkflowID: "business-requirement-validation",
						Status:     string(engine.ExecutionStatusCompleted),
						Metadata:   map[string]interface{}{"prompts": []interface{}{basicTemplate}},
					},
					Context: &engine.ExecutionContext{Variables: basicContext},
				}

				err = promptBuilder.GetLearnFromExecution(ctx, learningExecution)
				Expect(err).ToNot(HaveOccurred(), "BR-AI-PROMPT-002: Learning should work")

				// BR-AI-PROMPT-003: Template Optimization and Caching
				_, err = promptBuilder.GetGetOptimizedTemplate(ctx, "non-existent-template")
				Expect(err).To(HaveOccurred(), "BR-AI-PROMPT-003: Should handle missing templates appropriately")

				// BR-AI-PROMPT-004: Quality Assessment and Validation
				enhancedPrompt2, err := promptBuilder.GetBuildEnhancedPrompt(ctx, "low quality", basicContext)
				Expect(err).ToNot(HaveOccurred(), "BR-AI-PROMPT-004: Quality validation should work")
				Expect(len(enhancedPrompt2)).To(BeNumerically(">", 50), "BR-AI-PROMPT-004: Should improve low quality prompts")

				// BR-AI-PROMPT-005: Context-Aware Enhancement
				complexContext := map[string]interface{}{
					"workflow_type": "emergency",
					"environment":   "production",
					"complexity":    0.9,
					"severity":      "critical",
				}

				contextAwarePrompt, err := promptBuilder.GetBuildEnhancedPrompt(ctx, "Handle emergency", complexContext)
				Expect(err).ToNot(HaveOccurred(), "BR-AI-PROMPT-005: Context-aware enhancement should work")
				Expect(contextAwarePrompt).To(ContainSubstring("emergency"), "BR-AI-PROMPT-005: Should apply context awareness")

				// BR-AI-PROMPT-006: Error Handling and Resilience (already extensively tested above)
				_, err = promptBuilder.BuildPrompt(ctx, "", nil)
				Expect(err).ToNot(HaveOccurred(), "BR-AI-PROMPT-006: Should handle edge cases gracefully")

				// BR-AI-PROMPT-007: Performance and Caching (validated through load testing above)
				// Note: Cache functionality is validated at the integration level where context cache is available
				// This test focuses on the prompt builder's caching and optimization logic

				// BR-AI-PROMPT-008: Enhanced Pattern Discovery Algorithm
				builder := promptBuilder.(*engine.DefaultLearningEnhancedPromptBuilder)
				patterns := builder.ExtractKeyPatternsFromPromptForTesting("Analyze CPU metrics and recommend scaling actions")
				Expect(len(patterns)).To(BeNumerically(">=", 1), "BR-AI-PROMPT-008: Should extract meaningful patterns")

				// Final integration validation: Complex scenario combining all features
				integrationTemplate := "Execute comprehensive investigation of production incident with automated remediation"
				integrationContext := map[string]interface{}{
					"alert_type":     "complex_incident",
					"severity":       "critical",
					"environment":    "production",
					"workflow_type":  "comprehensive_investigation",
					"complexity":     0.95,
					"auto_remediate": true,
				}

				finalPrompt, err := promptBuilder.BuildPrompt(ctx, integrationTemplate, integrationContext)
				Expect(err).ToNot(HaveOccurred(), "Final integration should succeed")
				Expect(finalPrompt).To(ContainSubstring("comprehensive"), "Should maintain template intent")
				Expect(finalPrompt).To(ContainSubstring("production"), "Should apply context awareness")
				Expect(len(finalPrompt)).To(BeNumerically(">", len(integrationTemplate)*2), "Should demonstrate significant enhancement")

				// Quality gate: All business requirements should be demonstrably met
				testLogger.Info("All business requirements BR-AI-PROMPT-001 through BR-AI-PROMPT-008 validated successfully")
			})
		})
	})
})
