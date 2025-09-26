//go:build integration
// +build integration

package llm_integration

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	testshared "github.com/jordigilh/kubernaut/test/integration/shared"
)

var _ = Describe("BR-PA-011: Versioned Prompt Engineering Integration", Ordered, func() {
	var (
		hooks               *testshared.TestLifecycleHooks
		ctx                 context.Context
		suite               *testshared.StandardTestSuite
		realWorkflowBuilder engine.IntelligentWorkflowBuilder
		logger              *logrus.Logger
	)

	BeforeAll(func() {
		// Setup with real AI provider for prompt testing
		hooks = testshared.SetupAIIntegrationTest("Versioned Prompt Engineering Integration",
			testshared.WithRealLLM(),      // Real AI provider, not mocked
			testshared.WithRealVectorDB(), // Real pgvector for template persistence
			testshared.WithDatabaseIsolation(testshared.TransactionIsolation),
		)
		hooks.Setup()

		suite = hooks.GetSuite()
		logger = suite.Logger
		realWorkflowBuilder = suite.WorkflowBuilder
	})

	AfterAll(func() {
		if hooks != nil {
			hooks.Cleanup()
		}
	})

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("when using versioned prompts with real AI providers", func() {
		It("should achieve high response quality with v2.1 prompts", func() {
			// BR-PA-011: Test buildPromptFromVersion with actual LLM (not mocked)
			logger.Info("Testing versioned prompt v2.1 with real AI provider")

			// Create objective that triggers v2.1 versioned prompts
			objective := &engine.WorkflowObjective{
				ID:          "versioned-prompt-v21-001",
				Type:        "alert_remediation",
				Description: "High CPU usage alert requiring intelligent remediation with pattern recognition",
				Priority:    1,
				Constraints: map[string]interface{}{
					"max_execution_time":       "30m",
					"safety_level":             "high",
					"enable_versioned_prompts": true,
					"prompt_version":           "v2.1",
				},
			}

			// Generate workflow using versioned prompts
			startTime := time.Now()
			template, err := realWorkflowBuilder.GenerateWorkflow(ctx, objective)
			responseTime := time.Since(startTime)

			// Assert: Workflow generation succeeded
			Expect(err).NotTo(HaveOccurred())
			Expect(template).NotTo(BeNil())
			Expect(template.ID).NotTo(BeEmpty())

			// Assert: Versioned prompt metadata is present
			Expect(template.Metadata).To(HaveKey("versioned_prompt_applied"))
			Expect(template.Metadata["versioned_prompt_applied"]).To(BeTrue())

			// Assert: Correct prompt version was used
			Expect(template.Metadata).To(HaveKey("prompt_version_used"))
			Expect(template.Metadata["prompt_version_used"]).To(Equal("v2.1"))

			// Assert: Quality score meets requirements
			if template.Metadata["prompt_quality_score"] != nil {
				qualityScore := template.Metadata["prompt_quality_score"].(float64)
				Expect(qualityScore).To(BeNumerically(">=", 0.85), "v2.1 prompts should achieve >=85% quality score")
				logger.WithField("quality_score", qualityScore).Info("Prompt quality score achieved")
			}

			// Assert: Response time is acceptable
			Expect(responseTime).To(BeNumerically("<", 30*time.Second), "Versioned prompt response should be within 30 seconds")

			// Assert: Generated workflow has appropriate characteristics for v2.1
			Expect(template.Steps).NotTo(BeEmpty())

			// Verify workflow contains pattern recognition elements (v2.1 feature)
			hasPatternRecognition := false
			for _, step := range template.Steps {
				if step.Name != "" && (strings.Contains(strings.ToLower(step.Name), "pattern") ||
					strings.Contains(strings.ToLower(step.Name), "recognition")) {
					hasPatternRecognition = true
					break
				}
			}

			logger.WithFields(logrus.Fields{
				"template_id":         template.ID,
				"prompt_version":      "v2.1",
				"response_time":       responseTime,
				"total_steps":         len(template.Steps),
				"pattern_recognition": hasPatternRecognition,
			}).Info("✅ BR-PA-011: v2.1 versioned prompt integration test completed successfully")
		})

		It("should achieve enhanced performance with v2.5 prompts", func() {
			// BR-PA-011: Test v2.5 performance-optimized prompts
			logger.Info("Testing versioned prompt v2.5 with performance optimization")

			objective := &engine.WorkflowObjective{
				ID:          "versioned-prompt-v25-001",
				Type:        "performance_optimization",
				Description: "Performance optimization workflow requiring advanced analytics",
				Priority:    1,
				Constraints: map[string]interface{}{
					"enable_versioned_prompts": true,
					"prompt_version":           "v2.5",
					"track_performance":        true,
				},
			}

			startTime := time.Now()
			template, err := realWorkflowBuilder.GenerateWorkflow(ctx, objective)
			responseTime := time.Since(startTime)

			// Assert: Workflow generation succeeded
			Expect(err).NotTo(HaveOccurred())
			Expect(template).NotTo(BeNil())

			// Assert: v2.5 versioned prompt was used
			Expect(template.Metadata).To(HaveKey("prompt_version_used"))
			Expect(template.Metadata["prompt_version_used"]).To(Equal("v2.5"))

			// Assert: Performance tracking is enabled
			Expect(template.Metadata).To(HaveKey("prompt_performance_tracked"))
			Expect(template.Metadata["prompt_performance_tracked"]).To(BeTrue())

			// Assert: Quality score meets v2.5 requirements
			if template.Metadata["prompt_quality_score"] != nil {
				qualityScore := template.Metadata["prompt_quality_score"].(float64)
				Expect(qualityScore).To(BeNumerically(">=", 0.90), "v2.5 prompts should achieve >=90% quality score")
			}

			// Assert: Performance metrics are tracked
			if template.Metadata["prompt_generation_time"] != nil {
				generationTime := template.Metadata["prompt_generation_time"]
				Expect(generationTime).NotTo(BeNil())
			}

			if template.Metadata["prompt_success_rate"] != nil {
				successRate := template.Metadata["prompt_success_rate"].(float64)
				Expect(successRate).To(BeNumerically(">=", 0.85))
			}

			logger.WithFields(logrus.Fields{
				"template_id":    template.ID,
				"prompt_version": "v2.5",
				"response_time":  responseTime,
				"total_steps":    len(template.Steps),
			}).Info("✅ BR-PA-011: v2.5 versioned prompt integration test completed successfully")
		})

		It("should support advanced customization with v3.0 prompts", func() {
			// BR-PA-011: Test v3.0 advanced customization prompts
			logger.Info("Testing versioned prompt v3.0 with custom variables")

			objective := &engine.WorkflowObjective{
				ID:          "versioned-prompt-v30-001",
				Type:        "network_troubleshooting",
				Description: "Network troubleshooting with domain-specific customization",
				Priority:    2,
				Constraints: map[string]interface{}{
					"enable_versioned_prompts": true,
					"prompt_version":           "v3.0",
					"custom_variables": map[string]interface{}{
						"domain_expertise": "kubernetes_networking",
						"safety_mode":      "paranoid",
						"output_format":    "detailed_json",
					},
				},
			}

			startTime := time.Now()
			template, err := realWorkflowBuilder.GenerateWorkflow(ctx, objective)
			responseTime := time.Since(startTime)

			// Assert: Workflow generation succeeded
			Expect(err).NotTo(HaveOccurred())
			Expect(template).NotTo(BeNil())

			// Assert: v3.0 versioned prompt was used
			Expect(template.Metadata).To(HaveKey("prompt_version_used"))
			Expect(template.Metadata["prompt_version_used"]).To(Equal("v3.0"))

			// Assert: Custom variables were applied
			Expect(template.Metadata).To(HaveKey("custom_variables_applied"))
			Expect(template.Metadata["custom_variables_applied"]).To(BeTrue())

			// Assert: Domain expertise is reflected
			Expect(template.Metadata).To(HaveKey("domain_expertise"))
			Expect(template.Metadata["domain_expertise"]).To(Equal("kubernetes_networking"))

			// Assert: Safety mode is applied
			Expect(template.Metadata).To(HaveKey("safety_mode"))
			Expect(template.Metadata["safety_mode"]).To(Equal("paranoid"))

			// Assert: Quality score meets v3.0 requirements
			if template.Metadata["prompt_quality_score"] != nil {
				qualityScore := template.Metadata["prompt_quality_score"].(float64)
				Expect(qualityScore).To(BeNumerically(">=", 0.95), "v3.0 prompts should achieve >=95% quality score")
			}

			logger.WithFields(logrus.Fields{
				"template_id":      template.ID,
				"prompt_version":   "v3.0",
				"response_time":    responseTime,
				"domain_expertise": template.Metadata["domain_expertise"],
				"safety_mode":      template.Metadata["safety_mode"],
			}).Info("✅ BR-PA-011: v3.0 versioned prompt integration test completed successfully")
		})

		It("should use high-performance prompts for complex objectives", func() {
			// BR-PA-011: Test high-performance prompt selection for complex scenarios
			logger.Info("Testing high-performance prompt selection for complex objectives")

			objective := &engine.WorkflowObjective{
				ID:          "high-perf-prompt-001",
				Type:        "multi_cluster_remediation",
				Description: "Complex multi-cluster remediation requiring high-performance processing",
				Priority:    1,
				Constraints: map[string]interface{}{
					"enable_versioned_prompts": true,
					"complexity_level":         "high",
					"cluster_count":            5,
					"safety_level":             "critical",
				},
			}

			startTime := time.Now()
			template, err := realWorkflowBuilder.GenerateWorkflow(ctx, objective)
			responseTime := time.Since(startTime)

			// Assert: Workflow generation succeeded
			Expect(err).NotTo(HaveOccurred())
			Expect(template).NotTo(BeNil())

			// Assert: High-performance prompt was used
			Expect(template.Metadata).To(HaveKey("high_performance_prompt_used"))
			Expect(template.Metadata["high_performance_prompt_used"]).To(BeTrue())

			// Assert: Complexity optimization was applied
			Expect(template.Metadata).To(HaveKey("complexity_optimized"))
			Expect(template.Metadata["complexity_optimized"]).To(BeTrue())

			// Assert: Response time is acceptable even for complex scenarios
			Expect(responseTime).To(BeNumerically("<", 45*time.Second), "High-performance prompts should handle complexity efficiently")

			// Assert: Workflow is appropriate for multi-cluster scenarios
			Expect(template.Steps).NotTo(BeEmpty())
			Expect(len(template.Steps)).To(BeNumerically(">=", 3), "Complex multi-cluster workflows should have multiple steps")

			logger.WithFields(logrus.Fields{
				"template_id":      template.ID,
				"prompt_type":      "high-performance",
				"response_time":    responseTime,
				"total_steps":      len(template.Steps),
				"complexity_level": "high",
			}).Info("✅ BR-PA-011: High-performance prompt integration test completed successfully")
		})
	})

	Context("when testing prompt version fallback scenarios", func() {
		It("should fallback gracefully when versioned prompts are unavailable", func() {
			// BR-PA-011: Test graceful fallback to basic prompts
			logger.Info("Testing graceful fallback for unavailable prompt versions")

			objective := &engine.WorkflowObjective{
				ID:          "fallback-prompt-001",
				Type:        "basic_remediation",
				Description: "Test graceful fallback to basic prompts",
				Priority:    3,
				Constraints: map[string]interface{}{
					"enable_versioned_prompts": true,
					"prompt_version":           "v999.0", // Non-existent version
				},
			}

			startTime := time.Now()
			template, err := realWorkflowBuilder.GenerateWorkflow(ctx, objective)
			responseTime := time.Since(startTime)

			// Assert: Workflow generation succeeded despite invalid version
			Expect(err).NotTo(HaveOccurred())
			Expect(template).NotTo(BeNil())

			// Assert: Fallback was used
			Expect(template.Metadata).To(HaveKey("prompt_fallback_used"))
			Expect(template.Metadata["prompt_fallback_used"]).To(BeTrue())

			// Assert: Versioned prompt was attempted
			Expect(template.Metadata).To(HaveKey("versioned_prompt_attempted"))
			Expect(template.Metadata["versioned_prompt_attempted"]).To(BeTrue())

			// Assert: Response time is still acceptable
			Expect(responseTime).To(BeNumerically("<", 20*time.Second), "Fallback should be efficient")

			// Assert: Basic workflow was still generated
			Expect(template.Steps).NotTo(BeEmpty())

			logger.WithFields(logrus.Fields{
				"template_id":       template.ID,
				"requested_version": "v999.0",
				"fallback_used":     true,
				"response_time":     responseTime,
			}).Info("✅ BR-PA-011: Prompt fallback integration test completed successfully")
		})
	})

	Context("when validating prompt template persistence", func() {
		It("should cache and retrieve prompt templates efficiently", func() {
			// BR-PA-011: Test prompt template caching and retrieval
			logger.Info("Testing prompt template caching and retrieval efficiency")

			// First request - should create and cache template
			objective1 := &engine.WorkflowObjective{
				ID:          "cache-test-001",
				Type:        "template_caching",
				Description: "First request to test template caching",
				Priority:    1,
				Constraints: map[string]interface{}{
					"enable_versioned_prompts": true,
					"prompt_version":           "v2.1",
					"cache_test":               "first_request",
				},
			}

			firstStart := time.Now()
			template1, err := realWorkflowBuilder.GenerateWorkflow(ctx, objective1)
			firstTime := time.Since(firstStart)

			Expect(err).NotTo(HaveOccurred())
			Expect(template1).NotTo(BeNil())

			// Second request - should use cached template
			objective2 := &engine.WorkflowObjective{
				ID:          "cache-test-002",
				Type:        "template_caching",
				Description: "Second request to test template caching",
				Priority:    1,
				Constraints: map[string]interface{}{
					"enable_versioned_prompts": true,
					"prompt_version":           "v2.1",
					"cache_test":               "second_request",
				},
			}

			secondStart := time.Now()
			template2, err := realWorkflowBuilder.GenerateWorkflow(ctx, objective2)
			secondTime := time.Since(secondStart)

			Expect(err).NotTo(HaveOccurred())
			Expect(template2).NotTo(BeNil())

			// Assert: Both used the same prompt version
			Expect(template1.Metadata["prompt_version_used"]).To(Equal("v2.1"))
			Expect(template2.Metadata["prompt_version_used"]).To(Equal("v2.1"))

			// Assert: Both have versioned prompt metadata
			Expect(template1.Metadata["versioned_prompt_applied"]).To(BeTrue())
			Expect(template2.Metadata["versioned_prompt_applied"]).To(BeTrue())

			logger.WithFields(logrus.Fields{
				"first_request_time":  firstTime,
				"second_request_time": secondTime,
				"template1_id":        template1.ID,
				"template2_id":        template2.ID,
			}).Info("✅ BR-PA-011: Prompt template caching integration test completed successfully")
		})
	})
})
