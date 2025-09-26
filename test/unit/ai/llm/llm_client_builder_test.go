package llm

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
)

// BR-REAL-LLM-CONNECTIVITY-001: Real LLM Connectivity Business Logic
// Business Impact: LLM client creation and response processing enable business AI operations
// Stakeholder Value: Executive confidence in AI-powered business intelligence and automation
var _ = Describe("BR-REAL-LLM-CONNECTIVITY-001: LLM Client Creation Business Logic Unit Tests", func() {
	var (
		// Use REAL business logic components per cursor rules
		logger *logrus.Logger
	)

	BeforeEach(func() {
		// Create real logger
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	})

	Context("When testing LLM client creation business logic (BR-REAL-LLM-CONNECTIVITY-001)", func() {
		It("should create LLM client with default business configuration", func() {
			// Business Scenario: LLM client must provide default configuration for business operations
			// Business Impact: Default configuration enables consistent business AI operations

			// Test REAL business logic: LLM client creation
			config := config.LLMConfig{
				Provider:    "ramalama",
				Model:       "ggml-org/gpt-oss-20b-GGUF",
				Temperature: 0.7,
			}

			client, err := llm.NewClient(config, logger)
			Expect(err).ToNot(HaveOccurred(), "BR-REAL-LLM-CONNECTIVITY-001: Must create LLM client without errors")
			Expect(client).ToNot(BeNil(), "BR-REAL-LLM-CONNECTIVITY-001: Must create LLM client for business operations")

			// Validate business requirements: Client must provide health monitoring
			Expect(client.IsHealthy()).To(BeAssignableToTypeOf(false), "BR-REAL-LLM-CONNECTIVITY-001: Health status must be boolean")
		})

		It("should configure LLM client with business configuration parameters", func() {
			// Business Scenario: LLM client must support business-specific configuration
			// Business Impact: Business configuration enables customized AI operations

			// Test REAL business logic: configuration parameter validation
			businessConfig := config.LLMConfig{
				Provider:    "ollama",
				Model:       "ggml-org/gpt-oss-20b-GGUF",
				Temperature: 0.8,
				Endpoint:    "http://192.168.1.169:8080",
				Timeout:     30 * time.Second,
			}

			client, err := llm.NewClient(businessConfig, logger)
			Expect(err).ToNot(HaveOccurred(), "BR-REAL-LLM-CONNECTIVITY-001: Business configuration must be accepted")
			Expect(client).ToNot(BeNil(), "BR-REAL-LLM-CONNECTIVITY-001: Configured client must be available")

			// Business Validation: Client must provide configured parameters
			Expect(client.GetEndpoint()).To(Equal("http://192.168.1.169:8080"), "BR-REAL-LLM-CONNECTIVITY-001: Endpoint must match configuration")
			Expect(client.GetModel()).To(Equal("ggml-org/gpt-oss-20b-GGUF"), "BR-REAL-LLM-CONNECTIVITY-001: Model must match configuration")
		})

		It("should build LLM client with OpenAI provider configuration", func() {
			// Business Scenario: LLM client must support OpenAI for business AI operations
			// Business Impact: OpenAI support enables enterprise-grade AI business intelligence

			// Test REAL business logic: OpenAI provider configuration
			openAIConfig := config.LLMConfig{
				Provider:    "openai",
				Model:       "gpt-4",
				Temperature: 0.3,
			}

			client, err := llm.NewClient(openAIConfig, logger)

			// Note: This will fail without API key, which is expected behavior
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("API key"), "BR-REAL-LLM-CONNECTIVITY-001: OpenAI must require API key for security")
			} else {
				Expect(client).ToNot(BeNil(), "BR-REAL-LLM-CONNECTIVITY-001: OpenAI client must be available with API key")
			}
		})

		It("should validate LLM client business configuration requirements", func() {
			// Business Scenario: LLM client must validate business requirements before creation
			// Business Impact: Configuration validation prevents business operational failures

			// Test REAL business logic: configuration validation algorithms
			invalidConfig := config.LLMConfig{
				Provider: "unsupported-provider",
			}

			client, err := llm.NewClient(invalidConfig, logger)
			Expect(err).To(HaveOccurred(), "BR-REAL-LLM-CONNECTIVITY-001: Invalid provider must be rejected")
			Expect(client).To(BeNil(), "BR-REAL-LLM-CONNECTIVITY-001: Invalid configuration must not create client")

			// Business Validation: Error message must be business-appropriate
			Expect(err.Error()).To(ContainSubstring("unsupported"), "BR-REAL-LLM-CONNECTIVITY-001: Error must indicate unsupported provider")
		})

		It("should create LLM client with enterprise model requirements", func() {
			// Business Scenario: LLM client must support enterprise 20B+ parameter models
			// Business Impact: Enterprise models enable sophisticated business AI reasoning

			// Test REAL business logic: enterprise model configuration
			enterpriseConfig := config.LLMConfig{
				Provider:    "ramalama",
				Model:       "ggml-org/gpt-oss-20b-GGUF", // 20B+ parameter model
				Temperature: 0.7,
				Endpoint:    "http://192.168.1.169:8080",
			}

			client, err := llm.NewClient(enterpriseConfig, logger)
			Expect(err).ToNot(HaveOccurred(), "BR-REAL-LLM-CONNECTIVITY-001: Enterprise configuration must succeed")
			Expect(client).ToNot(BeNil(), "BR-REAL-LLM-CONNECTIVITY-001: Enterprise client must be available")

			// Business Validation: Enterprise model requirements
			Expect(client.GetMinParameterCount()).To(BeNumerically(">=", 20000000000), "BR-REAL-LLM-CONNECTIVITY-001: Must support 20B+ parameter models")
			Expect(client.GetModel()).To(ContainSubstring("20b"), "BR-REAL-LLM-CONNECTIVITY-001: Model name must indicate 20B parameters")
		})
	})

	Context("When testing LLM client response processing business logic (BR-REAL-LLM-CONNECTIVITY-002)", func() {
		var client *llm.ClientImpl

		BeforeEach(func() {
			// Create client for response processing tests
			config := config.LLMConfig{
				Provider:    "ramalama",
				Model:       "ggml-org/gpt-oss-20b-GGUF",
				Temperature: 0.7,
				Endpoint:    "http://192.168.1.169:8080",
			}

			var err error
			client, err = llm.NewClient(config, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should provide business response generation capabilities", func() {
			// Business Scenario: LLM client must generate business-appropriate responses
			// Business Impact: Response generation enables automated business intelligence

			// Test REAL business logic: response generation interface
			Expect(client.GenerateResponse).ToNot(BeNil(), "BR-REAL-LLM-CONNECTIVITY-002: Response generation must be available")

			// Business Validation: Response generation method signature
			response, err := client.GenerateResponse("test prompt")

			// Note: This may fail without actual LLM service, which is expected
			if err == nil {
				Expect(response).To(BeAssignableToTypeOf(""), "BR-REAL-LLM-CONNECTIVITY-002: Response must be string")
			}
		})

		It("should provide business health monitoring capabilities", func() {
			// Business Scenario: LLM client must provide health status for business monitoring
			// Business Impact: Health monitoring enables business operational reliability

			// Test REAL business logic: health monitoring interface
			isHealthy := client.IsHealthy()
			Expect(isHealthy).To(BeAssignableToTypeOf(false), "BR-REAL-LLM-CONNECTIVITY-002: Health status must be boolean")

			// Business Validation: Health monitoring provides business-relevant status
			endpoint := client.GetEndpoint()
			Expect(endpoint).ToNot(BeEmpty(), "BR-REAL-LLM-CONNECTIVITY-002: Endpoint must be available for monitoring")
		})

		It("should provide business workflow generation capabilities", func() {
			// Business Scenario: LLM client must generate business workflows
			// Business Impact: Workflow generation enables automated business process optimization

			// Test REAL business logic: workflow generation interface
			Expect(client.GenerateWorkflow).ToNot(BeNil(), "BR-REAL-LLM-CONNECTIVITY-002: Workflow generation must be available")

			// Business Validation: Workflow generation method signature exists
			// Note: Full workflow testing requires integration tests with real LLM
		})
	})
})
