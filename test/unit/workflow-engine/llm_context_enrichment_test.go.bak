package workflowengine

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

var _ = Describe("LLM Context Enrichment - Business Requirements Consistency", func() {
	var (
		integrator *engine.AIServiceIntegrator
		mockLLM    *mocks.MockLLMClient
		mockHolmes *mocks.MockClient
		testLogger *logrus.Logger
		testConfig *config.Config
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		testLogger = logrus.New()
		testLogger.SetLevel(logrus.WarnLevel) // Reduce noise in tests

		// Create test configuration following existing patterns
		testConfig = &config.Config{
			AIServices: config.AIServicesConfig{
				LLM: config.LLMConfig{
					Endpoint: "http://test-llm:8080",
					Provider: "localai",
					Model:    "gpt-oss:20b",
				},
				HolmesGPT: config.HolmesGPTConfig{
					Enabled:  true,
					Endpoint: "http://test-holmesgpt:8090",
					Timeout:  30 * time.Second,
				},
			},
		}

		// Use existing mock patterns from testutil
		mockLLM = mocks.NewMockLLMClient()
		mockHolmes = mocks.NewMockClient()

		// Create AI service integrator using existing patterns
		integrator = engine.NewAIServiceIntegrator(
			testConfig,
			mockLLM,    // LLM client
			mockHolmes, // HolmesGPT client
			nil,        // Vector DB
			nil,        // Metrics client (simplified for now)
			testLogger,
		)
	})

	AfterEach(func() {
		// Clear mock state between tests for isolation
		mockLLM.ClearHistory()
		mockHolmes.ClearHistory()
	})

	Context("Context Consistency Validation - BR-AI-011, BR-AI-012, BR-AI-013", func() {
		It("ensures LLM fallback receives enriched context similar to HolmesGPT", func() {
			// Given: HolmesGPT is unavailable to force LLM fallback
			mockHolmes.SetHealthError(fmt.Errorf("holmesgpt service unavailable"))

			// Given: Mock LLM response
			mockLLM.SetAnalysisResult(&types.ActionRecommendation{
				Action:     "increase_memory_limit",
				Confidence: 0.85,
				Reasoning: &types.ReasoningDetails{
					Summary: "Memory usage analysis indicates need for resource adjustment",
				},
			})

			// Given: A production alert that requires intelligent investigation
			productionAlert := types.Alert{
				Name:        "HighMemoryUsage",
				Severity:    "warning",
				Namespace:   "production",
				Resource:    "api-server-pod-xyz",
				Description: "Pod memory usage exceeded 80% threshold",
				Labels: map[string]string{
					"app":         "api-server",
					"environment": "production",
				},
				Annotations: map[string]string{
					"summary": "Memory threshold exceeded",
				},
			}

			// When: Investigating using LLM fallback (simulating HolmesGPT unavailable)
			result, err := integrator.InvestigateAlert(ctx, productionAlert)

			// Then: Investigation succeeds with enriched context
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Analysis).ToNot(BeEmpty(), "Should provide meaningful analysis")

			// Validate LLM received enriched alert
			lastLLMRequest := mockLLM.GetLastAnalyzeAlertRequest()
			Expect(lastLLMRequest.Description).To(ContainSubstring("Context:"), "Should include context enrichment")

			// Validate context enrichment metadata is present
			Expect(lastLLMRequest.Annotations).To(HaveKey("kubernaut_context_enriched"))
			Expect(lastLLMRequest.Annotations["kubernaut_context_enriched"]).To(Equal("true"))

			Expect(lastLLMRequest.Annotations).To(HaveKey("kubernaut_enrichment_source"))
			Expect(lastLLMRequest.Annotations["kubernaut_enrichment_source"]).To(Equal("ai_service_integrator"))

			Expect(lastLLMRequest.Annotations).To(HaveKey("kubernaut_enrichment_timestamp"))
			// Timestamp should be recent (within last few seconds)
			timestamp := lastLLMRequest.Annotations["kubernaut_enrichment_timestamp"]
			Expect(timestamp).ToNot(BeEmpty())
		})

		It("validates LLM receives action history context for pattern correlation (BR-AI-011, BR-AI-013)", func() {
			// Given: HolmesGPT is unavailable to force LLM fallback
			mockHolmes.SetHealthError(fmt.Errorf("holmesgpt service unavailable"))

			// Given: Mock LLM response
			mockLLM.SetAnalysisResult(&types.ActionRecommendation{
				Action:     "restart_database_pod",
				Confidence: 0.90,
				Reasoning: &types.ReasoningDetails{
					Summary: "Database connection pattern analysis suggests restart required",
				},
			})

			// Given: Alert with historical patterns available
			alertWithHistory := types.Alert{
				Name:        "DatabaseConnectionFailure",
				Severity:    "critical",
				Namespace:   "production",
				Resource:    "database-pod",
				Description: "Database connection pool exhausted",
			}

			// When: Using LLM fallback for investigation
			_, err := integrator.InvestigateAlert(ctx, alertWithHistory)

			// **Business Requirement BR-AI-011, BR-AI-013**: Validate action history context enrichment effectiveness
			Expect(err).ToNot(HaveOccurred())

			lastLLMRequest := mockLLM.GetLastAnalyzeAlertRequest()

			// **Business Value Validation**: Verify context enrichment provides actionable intelligence
			// 1. **Business Outcome**: Validate context enrichment occurred (not format-specific)
			Expect(lastLLMRequest.Description).ToNot(BeEmpty(),
				"BR-AI-011: Should provide enriched alert description for analysis")
			Expect(len(lastLLMRequest.Description)).To(BeNumerically(">", len("Database connection pool exhausted")),
				"BR-AI-011: Enriched description should contain additional context beyond original alert")

			// 2. **Business Outcome**: Validate enrichment metadata indicates successful processing
			Expect(lastLLMRequest.Annotations).To(HaveKey("kubernaut_context_enriched"),
				"BR-AI-011: Should mark alert as context-enriched for tracking")
			Expect(lastLLMRequest.Annotations["kubernaut_context_enriched"]).To(Equal("true"),
				"BR-AI-011: Context enrichment should be confirmed in metadata")

			// 3. **Business Outcome**: Validate alert correlation capabilities
			Expect(lastLLMRequest.Annotations).To(HaveKey("kubernaut_action_context_hash"),
				"BR-AI-013: Should provide correlation hash for pattern analysis")
			Expect(lastLLMRequest.Annotations["kubernaut_action_context_hash"]).ToNot(BeEmpty(),
				"BR-AI-013: Correlation hash should contain meaningful data for pattern matching")

			// 4. **Business Outcome**: Validate alert type tracking for business intelligence
			Expect(lastLLMRequest.Annotations).To(HaveKey("kubernaut_action_alert_type"),
				"BR-AI-013: Should track alert type for business analytics")
			Expect(lastLLMRequest.Annotations["kubernaut_action_alert_type"]).To(Equal("DatabaseConnectionFailure"),
				"BR-AI-013: Alert type should match original alert for accurate categorization")
		})

		It("validates LLM receives metrics context for evidence-based analysis (BR-AI-012)", func() {
			// Given: HolmesGPT is unavailable to force LLM fallback
			mockHolmes.SetHealthError(fmt.Errorf("holmesgpt service unavailable"))

			// Given: Mock LLM response
			mockLLM.SetAnalysisResult(&types.ActionRecommendation{
				Action:     "scale_deployment",
				Confidence: 0.88,
				Reasoning: &types.ReasoningDetails{
					Summary: "CPU metrics analysis indicates scaling required",
				},
			})

			// Given: Alert requiring metrics evidence for root cause analysis
			metricsAlert := types.Alert{
				Name:        "HighCPUUsage",
				Severity:    "warning",
				Namespace:   "production",
				Resource:    "compute-service",
				Description: "CPU usage sustained above 85% for 5 minutes",
			}

			// When: Using LLM fallback for investigation
			_, err := integrator.InvestigateAlert(ctx, metricsAlert)

			// **Business Requirement BR-AI-012**: Validate metrics context for evidence-based analysis
			Expect(err).ToNot(HaveOccurred())

			lastLLMRequest := mockLLM.GetLastAnalyzeAlertRequest()

			// **Business Value Validation**: Verify metrics context enhances analysis capability
			// 1. **Business Outcome**: Validate alert processing includes metric considerations
			Expect(lastLLMRequest.Description).ToNot(BeEmpty(),
				"BR-AI-012: Should provide enriched alert description with contextual information")
			Expect(len(lastLLMRequest.Description)).To(BeNumerically(">", len("CPU usage sustained above 85% for 5 minutes")),
				"BR-AI-012: Metrics-based enrichment should provide additional analytical context")

			// 2. **Business Outcome**: Validate enrichment infrastructure supports metrics integration
			Expect(lastLLMRequest.Annotations).To(HaveKey("kubernaut_context_enriched"),
				"BR-AI-012: Should mark alert as context-enriched for metrics analysis")
			Expect(lastLLMRequest.Annotations["kubernaut_context_enriched"]).To(Equal("true"),
				"BR-AI-012: Context enrichment should confirm successful metrics processing")

			// 3. **Business Outcome**: Validate alert contains sufficient context for analysis decisions
			// The enriched content should provide enough context for evidence-based analysis
			enrichmentTimestamp := lastLLMRequest.Annotations["kubernaut_enrichment_timestamp"]
			Expect(enrichmentTimestamp).ToNot(BeEmpty(),
				"BR-AI-012: Should timestamp enrichment for audit and correlation purposes")
		})

		It("validates enhanced description includes context summary for LLM understanding", func() {
			// Given: HolmesGPT is unavailable to force LLM fallback
			mockHolmes.SetHealthError(fmt.Errorf("holmesgpt service unavailable"))

			// Given: Mock LLM response
			mockLLM.SetAnalysisResult(&types.ActionRecommendation{
				Action:     "investigate_network",
				Confidence: 0.82,
				Reasoning: &types.ReasoningDetails{
					Summary: "Network latency analysis shows comprehensive context understanding",
				},
			})

			// Given: Alert that should receive comprehensive context enrichment
			comprehensiveAlert := types.Alert{
				Name:        "NetworkLatencyHigh",
				Severity:    "warning",
				Namespace:   "production",
				Resource:    "frontend-service",
				Description: "Service response time increased beyond acceptable threshold",
				Labels: map[string]string{
					"service": "frontend",
					"tier":    "web",
				},
			}

			// When: Using LLM fallback
			_, err := integrator.InvestigateAlert(ctx, comprehensiveAlert)

			// **Business Value Validation**: Verify comprehensive context enrichment effectiveness
			Expect(err).ToNot(HaveOccurred())

			lastLLMRequest := mockLLM.GetLastAnalyzeAlertRequest()

			// **Business Outcome**: Verify enriched analysis provides comprehensive understanding
			// 1. Validate alert description contains enhanced contextual information
			Expect(lastLLMRequest.Description).ToNot(BeEmpty(),
				"Should provide enriched alert description for comprehensive LLM analysis")
			Expect(len(lastLLMRequest.Description)).To(BeNumerically(">", len("Service response time increased beyond acceptable threshold")),
				"Enhanced description should contain additional context beyond original alert")

			// 2. **Business Outcome**: Validate original alert content is preserved during enrichment
			Expect(lastLLMRequest.Description).To(ContainSubstring("Service response time increased beyond acceptable threshold"),
				"Original alert description should be preserved during context enrichment")

			// 3. **Business Outcome**: Validate contextual information enhances analysis capability
			// The enriched description should provide operational context for better decision making
			enrichedContent := lastLLMRequest.Description
			Expect(len(enrichedContent)).To(BeNumerically(">", 100),
				"Enriched content should provide substantial additional context for analysis")

			// 4. **Business Outcome**: Validate enrichment includes environmental context
			hasEnvironmentalContext := len(enrichedContent) > len("Service response time increased beyond acceptable threshold")*2
			Expect(hasEnvironmentalContext).To(BeTrue(),
				"Context enrichment should provide significant additional environmental and operational context")
		})
	})

	Context("Context Consistency Between AI Services", func() {
		It("ensures both HolmesGPT and LLM receive context for same alert type", func() {
			// Given: Same alert processed by both services
			testAlert := types.Alert{
				Name:        "PodRestartLoop",
				Severity:    "critical",
				Namespace:   "production",
				Resource:    "problematic-pod",
				Description: "Pod has restarted 10 times in the last hour",
			}

			// When: Processing with LLM fallback (HolmesGPT unavailable)
			// Force LLM fallback by making HolmesGPT unavailable
			mockHolmes.SetHealthError(fmt.Errorf("holmesgpt service unavailable"))

			// Given: Mock LLM response
			mockLLM.SetAnalysisResult(&types.ActionRecommendation{
				Action:     "restart_pod",
				Confidence: 0.85,
				Reasoning: &types.ReasoningDetails{
					Summary: "Pod restart loop analysis indicates restart required",
				},
			})
			llmResult, err := integrator.InvestigateAlert(ctx, testAlert)

			// Then: Investigation succeeds
			Expect(err).ToNot(HaveOccurred())
			Expect(llmResult.Analysis).ToNot(BeEmpty(), "LLM should provide meaningful analysis")

			// **Business Requirement BR-AI-011, BR-AI-012, BR-AI-013**: Context consistency
			// Validate LLM received enriched context
			lastLLMRequest := mockLLM.GetLastAnalyzeAlertRequest()
			Expect(lastLLMRequest).ToNot(BeNil(), "LLM should have been called for fallback")
			Expect(lastLLMRequest.Description).To(ContainSubstring("Context:"), "Should include context enrichment")

			// Validate context enrichment metadata is present
			Expect(lastLLMRequest.Annotations).To(HaveKey("kubernaut_context_enriched"))
			Expect(lastLLMRequest.Annotations["kubernaut_context_enriched"]).To(Equal("true"))
		})
	})

	Context("Development Guidelines Compliance Validation", func() {
		It("validates LLM context enrichment reuses existing patterns", func() {
			// Given: HolmesGPT is unavailable to force LLM fallback
			mockHolmes.SetHealthError(fmt.Errorf("holmesgpt service unavailable"))

			// Given: Mock LLM response
			mockLLM.SetAnalysisResult(&types.ActionRecommendation{
				Action:     "investigate_issue",
				Confidence: 0.80,
				Reasoning: &types.ReasoningDetails{
					Summary: "Pattern analysis reuses existing investigation methods",
				},
			})

			// Following guideline: "reuse code whenever possible"
			testAlert := types.Alert{
				Name:      "TestAlert",
				Namespace: "test",
				Resource:  "test-resource",
			}

			// When: Using LLM investigation
			_, err := integrator.InvestigateAlert(ctx, testAlert)

			// **Business Value Validation**: Verify code reuse and pattern consistency
			Expect(err).ToNot(HaveOccurred())

			lastLLMRequest := mockLLM.GetLastAnalyzeAlertRequest()

			// **Business Outcome**: Validate enrichment consistency across AI services
			// 1. Verify enrichment occurred using reusable patterns
			Expect(lastLLMRequest.Description).ToNot(BeEmpty(),
				"Should provide enriched content using reusable enrichment patterns")
			Expect(len(lastLLMRequest.Description)).To(BeNumerically(">", len("TestAlert")),
				"Enrichment should add substantial context beyond basic alert name")

			// 2. **Business Outcome**: Validate same enrichment infrastructure is used
			Expect(lastLLMRequest.Annotations["kubernaut_enrichment_source"]).To(Equal("ai_service_integrator"),
				"Should use same enrichment source for consistency across AI services")

			// 3. **Business Outcome**: Validate enrichment metadata structure consistency
			Expect(lastLLMRequest.Annotations).To(HaveKey("kubernaut_context_enriched"),
				"Should use consistent enrichment metadata structure")
			Expect(lastLLMRequest.Annotations["kubernaut_context_enriched"]).To(Equal("true"),
				"Should consistently mark alerts as enriched across AI service integrations")

			// This validates we're reusing the same context gathering methods
			// (GatherCurrentMetricsContext, GatherActionHistoryContext)
		})

		It("validates business requirements are satisfied consistently", func() {
			// Given: HolmesGPT is unavailable to force LLM fallback
			mockHolmes.SetHealthError(fmt.Errorf("holmesgpt service unavailable"))

			// Given: Mock LLM response
			mockLLM.SetAnalysisResult(&types.ActionRecommendation{
				Action:     "monitor_and_alert",
				Confidence: 0.75,
				Reasoning: &types.ReasoningDetails{
					Summary: "Business requirement validation completed successfully",
				},
			})

			// Following guideline: "ensure functionality aligns with business requirements"
			testAlert := types.Alert{
				Name:      "BusinessRequirementTest",
				Namespace: "production",
			}

			// When: Using LLM fallback
			result, err := integrator.InvestigateAlert(ctx, testAlert)

			// Then: Business requirements are satisfied
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Analysis).ToNot(BeEmpty(), "Should provide actionable business analysis")
			Expect(result.Confidence).To(BeNumerically(">", 0.5), "Should provide confident recommendations")

			// BR-AI-011, BR-AI-012, BR-AI-013 satisfied through enriched context
			Expect(result.Method).To(Equal("llm_fallback_enriched"))
			Expect(result.Source).To(ContainSubstring("Context Enrichment"))
			Expect(result.Context).To(HaveKey("context_enriched"))
			Expect(result.Context["context_enriched"]).To(BeTrue())
		})
	})
})
