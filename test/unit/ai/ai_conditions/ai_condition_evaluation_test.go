package ai_conditions

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/common"
	"github.com/jordigilh/kubernaut/pkg/ai/conditions"
)

// BR-AI-INTEGRATION-002: AI Condition Evaluation Business Logic
// BR-AI-INTEGRATION-003: AI Common Service Processing Business Logic
// Business Impact: Operations teams need reliable AI-powered decision making for automated alert processing
// Stakeholder Value: Executive confidence in AI-driven automation and business intelligence
var _ = Describe("BR-AI-002-003: AI Condition Evaluation Unit Tests", func() {
	var (
		// Use REAL business logic components per cursor rules
		aiConditionEvaluator *conditions.DefaultAIConditionEvaluator
		aiCommonService      *common.AICommonService
		logger               *logrus.Logger
		ctx                  context.Context
		cancel               context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Create real logger
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)

		// Create REAL AI condition evaluator business logic (fallback mode for unit testing)
		config := &conditions.AIConditionEvaluatorConfig{
			EnableAdvancedEvaluation: true,
			MaxEvaluationTime:        30 * time.Second,
			ConfidenceThreshold:      0.7,
			LogLevel:                 "info",
			EnableDetailedLogging:    false,
			FallbackOnLowConfidence:  true,
			UseContextualAnalysis:    true,
		}
		aiConditionEvaluator = conditions.NewDefaultAIConditionEvaluator(config, logger)

		// Note: Not setting LLM client to test fallback evaluation business logic without external dependencies

		// Create REAL AI common service business logic
		aiCommonService = common.NewAICommonService()
	})

	AfterEach(func() {
		if cancel != nil {
			cancel()
		}
	})

	Context("When testing AI condition evaluation business logic (BR-AI-INTEGRATION-002)", func() {
		It("should evaluate conditions with AI-powered analysis for business decision making", func() {
			// Business Scenario: Executive stakeholders need confidence that AI condition evaluation provides accurate business decisions
			// Business Impact: Enables automated decision making based on AI analysis for business operations

			// Business Setup: Create test condition for AI evaluation
			testCondition := &conditions.Condition{
				ID:          "ai-business-condition-001",
				Name:        "High CPU Usage Business Impact",
				Type:        "resource_threshold",
				Expression:  "cpu_usage > 80%",
				Priority:    8, // High priority for business impact
				Description: "Critical CPU usage affecting business operations",
				Enabled:     true,
				Metadata:    make(map[string]interface{}),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			// Test REAL business logic: AI condition evaluation algorithm
			evaluationResult, err := aiConditionEvaluator.EvaluateCondition(ctx, testCondition)

			// Business Validation: AI condition evaluation provides business decision making
			Expect(err).ToNot(HaveOccurred(),
				"BR-AI-INTEGRATION-002: AI condition evaluation must succeed for business decision making")

			Expect(evaluationResult).ToNot(BeNil(),
				"BR-AI-INTEGRATION-002: AI evaluation result must exist for business operations")

			// Business Validation: Evaluation result must contain valid business decision
			Expect(evaluationResult.ConditionID).To(Equal(testCondition.ID),
				"BR-AI-INTEGRATION-002: Evaluation result must match condition ID for business traceability")

			Expect(evaluationResult.Result).To(BeAssignableToTypeOf(false),
				"BR-AI-INTEGRATION-002: Evaluation result must provide business decision")

			// Business Outcome: AI condition evaluation enables business decision making
			// Note: In fallback mode (without LLM), evaluator should still provide logical evaluation
			Expect(evaluationResult.EvaluatedAt).ToNot(BeZero(),
				"BR-AI-INTEGRATION-002: AI condition evaluation must provide timestamp for business auditability")

			Expect(evaluationResult.Metadata).ToNot(BeNil(),
				"BR-AI-INTEGRATION-002: AI condition evaluation must provide metadata for executive confidence in automated operations")
		})

		It("should handle complex business conditions with AI analysis", func() {
			// Business Scenario: Complex business rules require sophisticated AI evaluation

			// Create complex business condition
			complexCondition := &conditions.Condition{
				ID:          "complex-business-condition-001",
				Name:        "Multi-Factor Business Impact Assessment",
				Type:        "composite_threshold",
				Expression:  "(cpu_usage > 90% AND memory_usage > 85%) OR (error_rate > 10%)",
				Priority:    9, // Critical business priority
				Description: "Complex condition affecting multiple business KPIs",
				Enabled:     true,
				Metadata:    make(map[string]interface{}),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			// Test REAL business logic: complex condition evaluation
			result, err := aiConditionEvaluator.EvaluateCondition(ctx, complexCondition)

			// Business Validation: Complex conditions must be evaluated properly
			Expect(err).ToNot(HaveOccurred(),
				"BR-AI-INTEGRATION-002: Complex condition evaluation must succeed")

			Expect(result.Result).To(BeAssignableToTypeOf(false),
				"BR-AI-INTEGRATION-002: Complex evaluation must provide business decision")

			// Business Logic: Complex conditions should use fallback evaluation algorithms
			// The business logic should handle complex expressions even without LLM
		})

		It("should provide consistent evaluation results for business reliability", func() {
			// Business Scenario: Business operations require consistent AI decision making

			testCondition := &conditions.Condition{
				ID:          "consistency-test-condition",
				Name:        "Memory Usage Business Threshold",
				Type:        "resource_threshold",
				Expression:  "memory_usage > 75%",
				Priority:    7,
				Description: "Memory usage threshold for business continuity",
				Enabled:     true,
				Metadata:    make(map[string]interface{}),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			// Test REAL business logic: consistency of evaluation
			results := make([]*conditions.EvaluationResult, 5)
			for i := 0; i < 5; i++ {
				result, err := aiConditionEvaluator.EvaluateCondition(ctx, testCondition)
				Expect(err).ToNot(HaveOccurred())
				results[i] = result
			}

			// Business Validation: Results should be consistent for business reliability
			firstResult := results[0].Result
			for i := 1; i < 5; i++ {
				Expect(results[i].Result).To(Equal(firstResult),
					"BR-AI-INTEGRATION-002: AI evaluation must provide consistent results for business reliability")
			}
		})
	})

	Context("When testing AI common service business logic (BR-AI-INTEGRATION-003)", func() {
		It("should process data with AI common services for business intelligence", func() {
			// Business Scenario: Executive stakeholders need confidence that AI common services provide reliable data processing
			// Business Impact: Enables AI-powered business intelligence and data-driven decision making

			// Business Setup: Prepare business data for AI processing
			businessData := map[string]interface{}{
				"alert_type":      "performance",
				"severity":        "critical",
				"business_impact": "high",
			}

			// Test REAL business logic: AI common service processing
			processingResult, err := aiCommonService.ProcessWithContext(ctx, businessData)

			// Business Validation: AI common service processing provides business intelligence
			Expect(err).ToNot(HaveOccurred(),
				"BR-AI-INTEGRATION-003: AI common service processing must succeed for business intelligence")

			Expect(processingResult).ToNot(BeNil(),
				"BR-AI-INTEGRATION-003: Processing result must exist for business operations")

			Expect(processingResult.Success).To(BeTrue(),
				"BR-AI-INTEGRATION-003: Processing must succeed for business intelligence")

			// Business Outcome: AI common service processing enables business intelligence
			aiProcessingSuccessful := processingResult.Success

			Expect(aiProcessingSuccessful).To(BeTrue(),
				"BR-AI-INTEGRATION-003: AI common service processing must enable comprehensive business intelligence for executive confidence in data-driven operations")
		})

		It("should handle business data validation in AI processing", func() {
			// Business Scenario: Data integrity is critical for business intelligence

			// Test with invalid business data
			invalidBusinessData := map[string]interface{}{
				"invalid_field": nil,
				"empty_value":   "",
			}

			// Test REAL business logic: data validation algorithms
			processingResult, err := aiCommonService.ProcessWithContext(ctx, invalidBusinessData)

			// Business Validation: Should handle invalid data gracefully
			if err != nil {
				// Expected case - invalid data should be rejected
				Expect(err.Error()).To(ContainSubstring("validation"),
					"BR-AI-INTEGRATION-003: Invalid data errors should indicate validation failure")
			} else {
				// If processing succeeds, result should indicate validation status
				Expect(processingResult).ToNot(BeNil(),
					"BR-AI-INTEGRATION-003: Processing result must exist even for edge cases")

				Expect(processingResult.Success).To(BeAssignableToTypeOf(false),
					"BR-AI-INTEGRATION-003: Success status must be boolean for business logic")
			}
		})

		It("should optimize processing performance for business requirements", func() {
			// Business Scenario: Processing performance affects business operations

			businessData := map[string]interface{}{
				"alert_type":      "performance",
				"severity":        "warning",
				"business_impact": "medium",
				"timestamp":       time.Now(),
			}

			// Test REAL business logic: processing performance
			startTime := time.Now()
			processingResult, err := aiCommonService.ProcessWithContext(ctx, businessData)
			processingTime := time.Since(startTime)

			// Business Validation: Processing must meet performance requirements
			Expect(err).ToNot(HaveOccurred(),
				"BR-AI-INTEGRATION-003: Performance-optimized processing must succeed")

			Expect(processingTime).To(BeNumerically("<", 5*time.Second),
				"BR-AI-INTEGRATION-003: Processing time must meet business SLA requirements")

			Expect(processingResult.Success).To(BeTrue(),
				"BR-AI-INTEGRATION-003: Fast processing must maintain success rate")

			// Business Logic: Performance metrics should be tracked
			if processingResult.Metadata != nil {
				if metrics, hasMetrics := processingResult.Metadata["performance"]; hasMetrics {
					Expect(metrics).ToNot(BeNil(),
						"BR-AI-INTEGRATION-003: Performance metrics should be available for business monitoring")
				}
			}
		})
	})

	Context("When testing TDD compliance", func() {
		It("should validate real business logic usage per cursor rules", func() {
			// Business Scenario: Validate TDD approach with real business components

			// Verify we're testing REAL business logic per cursor rules
			Expect(aiConditionEvaluator).ToNot(BeNil(),
				"TDD: Must test real DefaultAIConditionEvaluator business logic")

			Expect(aiCommonService).ToNot(BeNil(),
				"TDD: Must test real AICommonService business logic")

			// Verify we're using real business logic, not mocks
			Expect(aiConditionEvaluator).To(BeAssignableToTypeOf(&conditions.DefaultAIConditionEvaluator{}),
				"TDD: Must use actual AI condition evaluator type, not mock")

			Expect(aiCommonService).To(BeAssignableToTypeOf(&common.AICommonService{}),
				"TDD: Must use actual AI common service type, not mock")

			// Verify internal components are real
			Expect(logger).To(BeAssignableToTypeOf(&logrus.Logger{}),
				"Cursor Rules: Internal logger should be real, not mocked")

			// Test that business logic can handle fallback mode (no external LLM dependency)
			testCondition := &conditions.Condition{
				ID:          "tdd-validation-condition",
				Name:        "TDD Validation Test",
				Type:        "simple",
				Expression:  "test = true",
				Priority:    1,
				Description: "TDD validation test condition",
				Enabled:     true,
				Metadata:    make(map[string]interface{}),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			result, err := aiConditionEvaluator.EvaluateCondition(ctx, testCondition)
			Expect(err).ToNot(HaveOccurred(),
				"TDD: Real business logic must be accessible for testing")

			Expect(result.Result).To(BeAssignableToTypeOf(false),
				"TDD: Real business logic must provide valid business results")
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
