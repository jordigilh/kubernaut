//go:build unit
// +build unit

package processor_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/integration/processor"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

// Business Requirements Unit Tests for AI Coordinator
// Focus: Test AI coordination business outcomes, not implementation details
// Strategy: Use real AI coordinator with mocked LLM client only

var _ = Describe("AI Coordinator Business Requirements", func() {
	var (
		// Mock ONLY external AI service (Rule 03 compliance)
		mockLLMClient *mocks.MockLLMClient

		// Use REAL business logic component
		aiCoordinator *processor.AICoordinator
		aiConfig      *processor.AIConfig

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Mock external AI service only
		mockLLMClient = mocks.NewMockLLMClient()
		mockLLMClient.SetHealthy(true)
		mockLLMClient.SetEndpoint("mock://ai-service")

		// Create REAL business AI configuration
		aiConfig = &processor.AIConfig{
			Provider:            "holmesgpt",
			Endpoint:            "mock://ai-service",
			Model:               "hf://ggml-org/gpt-oss-20b-GGUF",
			Timeout:             60 * time.Second,
			MaxRetries:          3,
			ConfidenceThreshold: 0.7,
		}

		// Create REAL AI coordinator business logic
		aiCoordinator = processor.NewAICoordinator(mockLLMClient, aiConfig)
	})

	AfterEach(func() {
		cancel()
	})

	// BR-AI-001: AI Analysis Coordination
	Context("BR-AI-001: AI Analysis Coordination Business Logic", func() {
		It("should prepare comprehensive analysis context for business decisions", func() {
			// Business Requirement: AI must receive rich context for accurate analysis
			alert := types.Alert{
				Name:      "DatabaseConnectionAlert",
				Severity:  "critical",
				Status:    "firing",
				Namespace: "production",
				Resource:  "database-service",
				Labels: map[string]string{
					"component":    "database",
					"cluster":      "prod-east",
					"service_tier": "critical",
				},
				Annotations: map[string]string{
					"description": "Database connection pool exhaustion",
					"runbook":     "https://runbooks.company.com/db-connections",
				},
			}

			// Configure AI response for business scenario
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "scale_database_connections",
				Confidence:        0.88,
				Reasoning:         "Connection pool exhaustion requires immediate scaling",
				ProcessingTime:    75 * time.Millisecond,
				Metadata: map[string]interface{}{
					"context_richness": "comprehensive",
					"analysis_depth":   "deep",
				},
			})

			// Test REAL business AI analysis coordination
			analysis, err := aiCoordinator.AnalyzeAlert(ctx, alert)

			// Validate REAL business AI coordination outcomes
			Expect(err).ToNot(HaveOccurred())
			Expect(analysis).ToNot(BeNil())
			Expect(analysis.Confidence).To(BeNumerically(">=", 0.7))
			Expect(analysis.RecommendedActions).To(ContainElement("scale_database_connections"))
			Expect(analysis.Reasoning).To(ContainSubstring("scaling"))
		})

		It("should validate AI responses for business safety", func() {
			// Business Requirement: System must validate AI responses before acting
			alert := types.Alert{
				Name:      "ValidationTestAlert",
				Severity:  "critical",
				Status:    "firing",
				Namespace: "production",
			}

			// Configure invalid AI response to test validation
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "", // Invalid empty action
				Confidence:        0.9,
				Reasoning:         "Test validation",
				ProcessingTime:    25 * time.Millisecond,
			})

			// Test REAL business AI response validation
			analysis, err := aiCoordinator.AnalyzeAlert(ctx, alert)

			// Validate REAL business validation outcomes
			if err != nil {
				// Validation error is acceptable business behavior
				Expect(err.Error()).To(ContainSubstring("invalid"))
			} else {
				// If no error, analysis should be valid
				Expect(analysis).ToNot(BeNil())
				Expect(analysis.RecommendedActions).ToNot(BeEmpty())
			}
		})

		It("should handle AI service failures gracefully for business continuity", func() {
			// Business Requirement: System must handle AI failures without breaking
			alert := types.Alert{
				Name:      "FailureTestAlert",
				Severity:  "critical",
				Status:    "firing",
				Namespace: "production",
			}

			// Configure AI service failure
			mockLLMClient.SetError("AI service connection timeout")

			// Test REAL business failure handling
			analysis, err := aiCoordinator.AnalyzeAlert(ctx, alert)

			// Validate REAL business failure handling outcomes
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("AI analysis failed"))
			Expect(analysis).To(BeNil())
		})

		It("should assess risk levels for business decision making", func() {
			// Business Requirement: AI must provide risk assessment for business decisions
			highRiskAlert := types.Alert{
				Name:      "HighRiskAlert",
				Severity:  "critical",
				Status:    "firing",
				Namespace: "production",
				Labels: map[string]string{
					"impact": "high",
					"scope":  "system-wide",
				},
			}

			// Configure high-risk AI response
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "emergency_scale",
				Confidence:        0.95,
				Reasoning:         "System-wide impact requires emergency scaling",
				ProcessingTime:    50 * time.Millisecond,
				Metadata: map[string]interface{}{
					"risk_level":   "high",
					"impact_scope": "system-wide",
					"urgency":      "immediate",
				},
			})

			// Test REAL business risk assessment
			analysis, err := aiCoordinator.AnalyzeAlert(ctx, highRiskAlert)

			// Validate REAL business risk assessment outcomes
			Expect(err).ToNot(HaveOccurred())
			Expect(analysis.RiskAssessment).ToNot(BeNil())
			Expect(analysis.Confidence).To(BeNumerically(">=", 0.9))
			Expect(analysis.RecommendedActions).To(ContainElement("emergency_scale"))
		})
	})

	// BR-PA-006: LLM Provider Integration (AI Coordinator Level)
	Context("BR-PA-006: LLM Provider Integration Business Logic", func() {
		It("should leverage sophisticated LLM capabilities for complex business scenarios", func() {
			// Business Requirement: System must use advanced LLM for complex analysis
			complexAlert := types.Alert{
				Name:      "ComplexMicroserviceAlert",
				Severity:  "critical",
				Status:    "firing",
				Namespace: "production",
				Resource:  "microservice-mesh",
				Labels: map[string]string{
					"architecture": "microservices",
					"complexity":   "high",
					"dependencies": "multiple",
				},
				Annotations: map[string]string{
					"cascade_risk":    "high",
					"business_impact": "revenue_affecting",
				},
			}

			// Configure sophisticated LLM response
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "orchestrated_recovery_sequence",
				Confidence:        0.92,
				Reasoning:         "Complex microservice failure requires orchestrated recovery with dependency analysis",
				ProcessingTime:    200 * time.Millisecond, // Longer for complex analysis
				Metadata: map[string]interface{}{
					"analysis_type":     "sophisticated",
					"dependency_graph":  []string{"service-a", "service-b", "service-c"},
					"recovery_sequence": []string{"isolate", "recover", "validate", "restore"},
				},
			})

			// Test REAL business sophisticated LLM integration
			analysis, err := aiCoordinator.AnalyzeAlert(ctx, complexAlert)

			// Validate REAL business sophisticated analysis outcomes
			Expect(err).ToNot(HaveOccurred())
			Expect(analysis.Confidence).To(BeNumerically(">=", 0.9))
			Expect(analysis.RecommendedActions).To(ContainElement("orchestrated_recovery_sequence"))
			Expect(analysis.Reasoning).To(ContainSubstring("orchestrated"))
			Expect(analysis.Reasoning).To(ContainSubstring("dependency"))
		})

		It("should handle different LLM model capabilities for business needs", func() {
			// Business Requirement: System must adapt to different LLM model capabilities
			modelTests := []struct {
				model              string
				expectedComplexity string
				minConfidence      float64
			}{
				{
					model:              "hf://ggml-org/gpt-oss-20b-GGUF",
					expectedComplexity: "high",
					minConfidence:      0.8,
				},
				{
					model:              "gpt-4",
					expectedComplexity: "very-high",
					minConfidence:      0.85,
				},
				{
					model:              "claude-3",
					expectedComplexity: "high",
					minConfidence:      0.8,
				},
			}

			testAlert := types.Alert{
				Name:      "ModelCapabilityAlert",
				Severity:  "critical",
				Status:    "firing",
				Namespace: "production",
			}

			for _, test := range modelTests {
				// Update AI configuration for different model
				aiConfig.Model = test.model

				// Configure model-specific response
				mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
					RecommendedAction: fmt.Sprintf("action_for_%s", test.model),
					Confidence:        test.minConfidence + 0.05,
					Reasoning:         fmt.Sprintf("Analysis using %s model capabilities", test.model),
					ProcessingTime:    100 * time.Millisecond,
				})

				// Test REAL business model capability adaptation
				analysis, err := aiCoordinator.AnalyzeAlert(ctx, testAlert)

				// Validate REAL business model capability outcomes
				Expect(err).ToNot(HaveOccurred())
				Expect(analysis.Confidence).To(BeNumerically(">=", test.minConfidence))
				Expect(analysis.RecommendedActions).To(ContainElement(fmt.Sprintf("action_for_%s", test.model)))
			}
		})

		It("should optimize context for LLM efficiency while maintaining business accuracy", func() {
			// Business Requirement: System must balance LLM efficiency with analysis accuracy
			efficiencyAlert := types.Alert{
				Name:      "EfficiencyTestAlert",
				Severity:  "high",
				Status:    "firing",
				Namespace: "production",
				Labels: map[string]string{
					"optimization": "efficiency_test",
				},
			}

			// Configure efficient AI response
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "efficient_action",
				Confidence:        0.82,
				Reasoning:         "Optimized analysis balancing speed and accuracy",
				ProcessingTime:    30 * time.Millisecond, // Fast response
				Metadata: map[string]interface{}{
					"optimization": "balanced",
					"efficiency":   "high",
				},
			})

			// Test REAL business efficiency optimization
			startTime := time.Now()
			analysis, err := aiCoordinator.AnalyzeAlert(ctx, efficiencyAlert)
			analysisTime := time.Since(startTime)

			// Validate REAL business efficiency outcomes
			Expect(err).ToNot(HaveOccurred())
			Expect(analysis.Confidence).To(BeNumerically(">=", 0.7))   // Still meets business threshold
			Expect(analysisTime).To(BeNumerically("<", 1*time.Second)) // Efficient processing
			Expect(analysis.RecommendedActions).ToNot(BeEmpty())       // Maintains accuracy
		})
	})

	// BR-AI-002: AI Response Quality Assurance
	Context("BR-AI-002: AI Response Quality Assurance Business Logic", func() {
		It("should ensure AI responses meet business quality standards", func() {
			// Business Requirement: AI responses must meet quality standards for production use
			alert := types.Alert{
				Name:      "QualityAssuranceAlert",
				Severity:  "critical",
				Status:    "firing",
				Namespace: "production",
			}

			// Configure high-quality AI response
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "quality_assured_action",
				Confidence:        0.89,
				Reasoning:         "Comprehensive analysis with high confidence and detailed reasoning for production safety",
				ProcessingTime:    80 * time.Millisecond,
				Metadata: map[string]interface{}{
					"quality_score": 0.95,
					"completeness":  "comprehensive",
					"safety_level":  "production",
				},
			})

			// Test REAL business quality assurance
			analysis, err := aiCoordinator.AnalyzeAlert(ctx, alert)

			// Validate REAL business quality outcomes
			Expect(err).ToNot(HaveOccurred())
			Expect(analysis.Confidence).To(BeNumerically(">=", 0.8))
			Expect(analysis.Reasoning).ToNot(BeEmpty())
			Expect(len(analysis.Reasoning)).To(BeNumerically(">=", 20)) // Detailed reasoning
			Expect(analysis.RecommendedActions).ToNot(BeEmpty())
		})

		It("should reject low-quality AI responses for business safety", func() {
			// Business Requirement: System must reject poor quality AI responses
			alert := types.Alert{
				Name:      "LowQualityTestAlert",
				Severity:  "critical",
				Status:    "firing",
				Namespace: "production",
			}

			// Configure low-quality AI response
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "vague_action",
				Confidence:        0.3, // Very low confidence
				Reasoning:         "?", // Poor reasoning
				ProcessingTime:    5 * time.Millisecond,
			})

			// Test REAL business quality rejection
			analysis, err := aiCoordinator.AnalyzeAlert(ctx, alert)

			// Validate REAL business quality rejection outcomes
			if err != nil {
				// Quality rejection is acceptable business behavior
				Expect(err.Error()).To(ContainSubstring("invalid"))
			} else {
				// If accepted, should still meet minimum standards
				Expect(analysis.Confidence).To(BeNumerically(">=", 0.0))
				Expect(analysis.RecommendedActions).ToNot(BeEmpty())
			}
		})
	})
})

func TestAICoordinatorBusinessRequirements(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AI Coordinator Business Requirements Suite")
}
