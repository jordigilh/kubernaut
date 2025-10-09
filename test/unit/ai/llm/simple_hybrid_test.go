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

//go:build unit
// +build unit

package llm

import (
	"testing"
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/testutil/hybrid"
)

// BR-LLM-SIMPLE-001: Simple Environment-Aware LLM Testing
// Business Impact: Clean, maintainable AI testing with automatic environment detection
// Stakeholder Value: Developers get simple setup, operations get reliable CI/CD

var _ = Describe("Simple Hybrid LLM Testing", func() {
	var (
		ctx       context.Context
		logger    *logrus.Logger
		llmClient llm.Client
		startTime time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel)
		startTime = time.Now()
	})

	Context("BR-LLM-SIMPLE-001: Environment-Aware Client Creation", func() {
		It("should create appropriate LLM client based on environment", func() {
			// Business Scenario: One line of code should give us the right client
			// Phase 2 Requirement: Simple, clean interface with intelligent selection

			// This is all you need - one function call
			llmClient = hybrid.CreateLLMClient(logger)

			// Performance: Should be fast
			elapsed := time.Since(startTime)
			Expect(elapsed).To(BeNumerically("<", 3*time.Second),
				"BR-LLM-SIMPLE-001: Client creation should be fast")

			// Validation: Should work regardless of which client type
			Expect(llmClient).ToNot(BeNil(),
				"BR-LLM-SIMPLE-001: Should create valid client")
			Expect(llmClient.IsHealthy()).To(BeTrue(),
				"BR-LLM-SIMPLE-001: Client should be healthy")

			// Business validation: Should handle AI operations
			response, err := llmClient.ChatCompletion(ctx, "Test prompt for simple hybrid approach")
			Expect(err).ToNot(HaveOccurred(),
				"BR-LLM-SIMPLE-001: Should handle AI operations successfully")
			Expect(response).ToNot(BeEmpty(),
				"BR-LLM-SIMPLE-001: Should provide meaningful responses")

			logger.WithFields(logrus.Fields{
				"elapsed_time": elapsed,
				"response_len": len(response),
				"test_success": true,
			}).Info("Simple hybrid LLM client test completed")
		})

		It("should respect CI environment and use mock", func() {
			// Business Scenario: CI should always use mock for reliability
			// Phase 2 Requirement: Automatic CI detection

			// Simulate CI environment
			originalCI := os.Getenv("CI")
			os.Setenv("CI", "true")
			defer func() {
				if originalCI == "" {
					os.Unsetenv("CI")
				} else {
					os.Setenv("CI", originalCI)
				}
			}()

			// Create client in CI environment
			llmClient = hybrid.CreateLLMClient(logger)

			// Should still work perfectly
			Expect(llmClient).ToNot(BeNil(),
				"BR-LLM-SIMPLE-001: Should work in CI environment")
			Expect(llmClient.IsHealthy()).To(BeTrue(),
				"BR-LLM-SIMPLE-001: Should be healthy in CI")

			// Should be fast in CI (mock)
			response, err := llmClient.ChatCompletion(ctx, "CI test prompt")
			Expect(err).ToNot(HaveOccurred(),
				"BR-LLM-SIMPLE-001: Should work in CI")
			Expect(response).ToNot(BeEmpty(),
				"BR-LLM-SIMPLE-001: Should provide response in CI")

			logger.Info("CI environment handling validated")
		})

		It("should handle explicit mock preference", func() {
			// Business Scenario: Developers should be able to force mock usage
			// Phase 2 Requirement: Environment variable override

			// Set explicit mock preference
			originalUseLLM := os.Getenv("USE_REAL_LLM")
			os.Setenv("USE_REAL_LLM", "false")
			defer func() {
				if originalUseLLM == "" {
					os.Unsetenv("USE_REAL_LLM")
				} else {
					os.Setenv("USE_REAL_LLM", originalUseLLM)
				}
			}()

			// Create client with explicit mock preference
			llmClient = hybrid.CreateLLMClient(logger)

			// Should work with explicit mock
			Expect(llmClient).ToNot(BeNil(),
				"BR-LLM-SIMPLE-001: Should respect mock preference")

			response, err := llmClient.ChatCompletion(ctx, "Explicit mock test")
			Expect(err).ToNot(HaveOccurred(),
				"BR-LLM-SIMPLE-001: Mock should handle operations")
			Expect(response).ToNot(BeEmpty(),
				"BR-LLM-SIMPLE-001: Mock should provide responses")

			logger.Info("Explicit mock preference validated")
		})
	})

	Context("BR-LLM-SIMPLE-002: Performance and Reliability", func() {
		It("should meet performance targets regardless of client type", func() {
			// Business Scenario: Performance should be consistent
			// Phase 2 Requirement: <5s operations regardless of client

			llmClient = hybrid.CreateLLMClient(logger)

			// Test multiple operations for consistency
			for i := 0; i < 3; i++ {
				operationStart := time.Now()

				response, err := llmClient.ChatCompletion(ctx, "Performance test prompt")

				operationTime := time.Since(operationStart)
				Expect(operationTime).To(BeNumerically("<", 5*time.Second),
					"BR-LLM-SIMPLE-002: Operation %d should meet performance target", i+1)

				Expect(err).ToNot(HaveOccurred(),
					"BR-LLM-SIMPLE-002: Operation %d should succeed", i+1)
				Expect(response).ToNot(BeEmpty(),
					"BR-LLM-SIMPLE-002: Operation %d should provide response", i+1)
			}

			logger.Info("Performance consistency validated across multiple operations")
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUsimpleUhybrid(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UsimpleUhybrid Suite")
}
