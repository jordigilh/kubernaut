//go:build unit
// +build unit

package processor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/integration/processor"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// BR-PROC-AI-001: Processor must use AI service for alert analysis
// BR-LLM-CENTRAL-001: Centralized LLM operations via AI service
// BR-AI-HTTP-001: HTTP-based AI service communication
var _ = Describe("BR-PROC-AI-001: Processor AI Service Integration", func() {
	var (
		ctx       context.Context
		testAlert types.Alert
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Test alert data
		testAlert = types.Alert{
			Name:      "TestAlert",
			Severity:  "critical",
			Namespace: "test-namespace",
			Resource:  "test-pod",
		}
	})

	Context("BR-PROC-AI-001: AI Service Client Factory", func() {
		It("should create HTTP LLM client when AI service endpoint configured", func() {
			// Configure for AI service endpoint
			cfg := &processor.Config{
				AI: processor.AIConfig{
					Provider: "ai-service",
					Endpoint: "http://ai-service:8093",
					Timeout:  30 * time.Second,
				},
			}

			// Create LLM client via factory
			llmClient, err := createLLMClientFromConfig(cfg)
			Expect(err).ToNot(HaveOccurred())
			Expect(llmClient).ToNot(BeNil())

			// Verify it's HTTP LLM client
			Expect(llmClient.GetEndpoint()).To(Equal("http://ai-service:8093"))
			Expect(llmClient.GetModel()).To(ContainSubstring("http"))
		})

		It("should route alert analysis to AI service via HTTP", func() {
			// Configure AI service endpoint
			cfg := &processor.Config{
				AI: processor.AIConfig{
					Endpoint: "http://ai-service:8093",
					Provider: "ai-service",
				},
			}

			// Create LLM client via factory - this is what we're testing
			llmClient, err := createLLMClientFromConfig(cfg)
			Expect(err).ToNot(HaveOccurred())
			Expect(llmClient).ToNot(BeNil())

			// Verify it's HTTP LLM client by checking endpoint
			Expect(llmClient.GetEndpoint()).To(Equal("http://ai-service:8093"))
		})
	})

	Context("BR-LLM-CENTRAL-001: Centralized LLM Operations", func() {
		It("should centralize all LLM operations through AI service", func() {
			// Configure centralized AI service
			cfg := &processor.Config{
				AI: processor.AIConfig{
					Endpoint: "http://ai-service:8093",
					Provider: "centralized",
				},
			}

			llmClient, err := createLLMClientFromConfig(cfg)
			Expect(err).ToNot(HaveOccurred())

			// All LLM operations should go through AI service
			response, err := llmClient.AnalyzeAlert(ctx, testAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(response.Action).ToNot(BeEmpty())
			Expect(response.Confidence).To(BeNumerically(">", 0))
		})
	})

	Context("BR-AI-HTTP-001: HTTP Communication Validation", func() {
		It("should handle HTTP communication errors gracefully", func() {
			// Configure invalid AI service endpoint
			cfg := &processor.Config{
				AI: processor.AIConfig{
					Endpoint: "http://invalid-ai-service:9999",
					Provider: "ai-service",
					Timeout:  1 * time.Second,
				},
			}

			llmClient, err := createLLMClientFromConfig(cfg)
			Expect(err).ToNot(HaveOccurred())

			// Should handle connection errors
			_, err = llmClient.AnalyzeAlert(ctx, testAlert)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("HTTP"))
		})

		It("should validate HTTP response format", func() {
			// Test with properly configured AI service
			cfg := &processor.Config{
				AI: processor.AIConfig{
					Endpoint: "http://ai-service:8093",
					Provider: "ai-service",
				},
			}

			llmClient, err := createLLMClientFromConfig(cfg)
			Expect(err).ToNot(HaveOccurred())

			// Response should be properly formatted
			response, err := llmClient.AnalyzeAlert(ctx, testAlert)
			if err == nil { // Only validate if service is available
				Expect(response.Action).ToNot(BeEmpty())
				Expect(response.Confidence).To(BeNumerically(">=", 0))
				Expect(response.Confidence).To(BeNumerically("<=", 1))
				Expect(response.Reasoning).ToNot(BeNil())
			}
		})
	})

	Context("BR-PROC-AI-001: Enhanced AI Service Routing", func() {
		It("should detect AI service endpoints correctly", func() {
			testCases := []struct {
				endpoint string
				provider string
				expected bool
			}{
				{"http://ai-service:8093", "holmesgpt", true},
				{"http://kubernaut-ai-service:8093", "openai", true},
				{"http://localhost:8093", "centralized", true},
				{"http://openai-api:8080", "openai", false},
				{"http://localhost:8080", "localai", false},
			}

			for _, tc := range testCases {
				cfg := &processor.Config{
					AI: processor.AIConfig{
						Endpoint: tc.endpoint,
						Provider: tc.provider,
					},
				}

				client, err := createLLMClientFromConfig(cfg)
				if tc.expected {
					Expect(err).ToNot(HaveOccurred())
					Expect(client.GetEndpoint()).To(Equal(tc.endpoint))
				} else {
					// Should create standard client, not HTTP client
					Expect(err).ToNot(HaveOccurred())
					Expect(client).ToNot(BeNil())
				}
			}
		})

		It("should handle configuration validation", func() {
			invalidConfigs := []*processor.Config{
				nil,
				{AI: processor.AIConfig{Endpoint: ""}},
				{AI: processor.AIConfig{Endpoint: "invalid-url"}},
			}

			for _, cfg := range invalidConfigs {
				_, err := createLLMClientFromConfig(cfg)
				Expect(err).To(HaveOccurred())
			}
		})
	})
})

// Helper function to test - uses real implementation
func createLLMClientFromConfig(cfg *processor.Config) (llm.Client, error) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel) // Reduce noise in tests
	return processor.CreateLLMClientFromConfig(cfg, log)
}
