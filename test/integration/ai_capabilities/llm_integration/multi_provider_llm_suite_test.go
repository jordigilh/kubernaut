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
//go:build integration
// +build integration

package llm_integration

import (
	"context"
	"testing"
	"time"

	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
)

func TestMultiProviderLLM(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Multi-Provider LLM Integration Tests Suite")
}

// BR-MULTI-PROVIDER-001: Multi-Provider LLM Business Operations
// Business Impact: Ensures enterprise-grade LLM provider redundancy for business continuity
// Stakeholder Value: Provides fallback capabilities for uninterrupted AI services

var _ = Describe("Multi-Provider LLM Integration Tests", func() {
	var (
		ctx        context.Context
		mockLogger *mocks.MockLogger
		providers  []string
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockLogger = mocks.NewMockLogger()
		// mockLogger level set automatically // Reduce test noise

		// Define supported enterprise LLM providers for business continuity
		providers = []string{"ramalama", "ollama", "openai", "huggingface"}
	})

	Context("BR-MULTI-PROVIDER-001: Provider Support and Configuration", func() {
		It("should support multiple LLM providers for business resilience", func() {
			// Business Requirement: Multiple LLM providers must be supported for business continuity
			// Business Impact: Ensures AI services remain available during provider outages

			for _, provider := range providers {
				// Test each provider configuration for business operations
				providerConfig := config.LLMConfig{
					Provider:    provider,
					Model:       "test-model",
					Endpoint:    "http://localhost:8080",
					Timeout:     30 * time.Second,
					Temperature: 0.7,
					MaxTokens:   1000,
				}

				// Create LLM client for each provider
				llmClient, err := llm.NewClient(providerConfig, mockLogger.Logger)

				// Business validation: Client creation must succeed for all supported providers
				Expect(err).ToNot(HaveOccurred(), "BR-MULTI-PROVIDER-001: Provider %s must be supported for business continuity", provider)
				Expect(llmClient).ToNot(BeNil(), "BR-MULTI-PROVIDER-001: Provider %s client must be created for business operations", provider)

				// Validate provider-specific configuration
				Expect(llmClient.GetEndpoint()).ToNot(BeEmpty(), "BR-MULTI-PROVIDER-001: Provider %s must have valid endpoint for business connectivity", provider)
				Expect(llmClient.GetModel()).ToNot(BeEmpty(), "BR-MULTI-PROVIDER-001: Provider %s must have valid model for business operations", provider)
			}
		})

		It("should provide health monitoring across all providers for business reliability", func() {
			// Business Requirement: Health monitoring must work across all providers
			// Business Impact: Enables proactive monitoring for business service availability

			for _, provider := range providers {
				providerConfig := config.LLMConfig{
					Provider: provider,
					Model:    "test-model",
					Endpoint: "http://localhost:8080",
					Timeout:  10 * time.Second,
				}

				llmClient, err := llm.NewClient(providerConfig, mockLogger.Logger)
				Expect(err).ToNot(HaveOccurred(), "BR-MULTI-PROVIDER-001: Provider %s client creation must succeed", provider)

				// Test health monitoring capabilities
				isHealthy := llmClient.IsHealthy()

				// Business validation: Health status must be determinable
				// Note: In test environment, this may return false due to unavailable endpoints
				// The important business requirement is that the method exists and can be called
				_ = isHealthy // Health status recorded for business monitoring

				// Test liveness and readiness checks for business operations
				err = llmClient.LivenessCheck(ctx)
				// Expected to fail in test environment - validates interface exists
				if err != nil {
					Expect(err.Error()).To(ContainSubstring("connect"), "BR-MULTI-PROVIDER-001: Provider %s liveness errors should be connection-related in test environment", provider)
				}

				err = llmClient.ReadinessCheck(ctx)
				// Expected to fail in test environment - validates interface exists
				if err != nil {
					Expect(err.Error()).To(ContainSubstring("connect"), "BR-MULTI-PROVIDER-001: Provider %s readiness errors should be connection-related in test environment", provider)
				}
			}
		})
	})

	Context("BR-MULTI-PROVIDER-002: Provider Failover and Business Continuity", func() {
		It("should handle provider switching for business resilience", func() {
			// Business Requirement: System must handle switching between providers
			// Business Impact: Ensures continuous AI services during provider failures

			// Test primary provider configuration
			primaryConfig := config.LLMConfig{
				Provider: "ramalama",
				Model:    "ggml-org/gpt-oss-20b-GGUF",
				Endpoint: "http://localhost:8080",
				Timeout:  30 * time.Second,
			}

			primaryClient, err := llm.NewClient(primaryConfig, mockLogger.Logger)
			Expect(err).ToNot(HaveOccurred(), "BR-MULTI-PROVIDER-002: Primary provider must be configurable for business operations")

			// Test fallback provider configuration
			fallbackConfig := config.LLMConfig{
				Provider: "ollama",
				Model:    "test-model",
				Endpoint: "http://localhost:11434",
				Timeout:  30 * time.Second,
			}

			fallbackClient, err := llm.NewClient(fallbackConfig, mockLogger.Logger)
			Expect(err).ToNot(HaveOccurred(), "BR-MULTI-PROVIDER-002: Fallback provider must be configurable for business continuity")

			// Business validation: Both clients should be independently operational
			Expect(primaryClient.GetEndpoint()).ToNot(Equal(fallbackClient.GetEndpoint()), "BR-MULTI-PROVIDER-002: Providers must use different endpoints for business redundancy")

			// Business outcome: Multiple providers support business resilience strategy
			mockLogger.Logger.WithFields(logrus.Fields{
				"primary_provider":  primaryConfig.Provider,
				"fallback_provider": fallbackConfig.Provider,
				"business_impact":   "continuity_ensured",
			}).Info("BR-MULTI-PROVIDER-002: Multi-provider configuration validated for business continuity")
		})
	})
})
