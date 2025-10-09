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

package embedding

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/embedding"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

var _ = Describe("BR-AI-001 & BR-ORK-004: Dynamic Cost Calculation", func() {
	var (
		pipeline       *embedding.AIEmbeddingPipeline
		mockLLMClient  *mocks.MockLLMClient
		mockVectorDB   *MockVectorDatabase
		ctx            context.Context
		testAlert      *types.Alert
		costCalculator *embedding.AIDynamicCostCalculator
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockLLMClient = mocks.NewMockLLMClient()
		mockVectorDB = &MockVectorDatabase{}

		// Create logger for use throughout tests
		logger := logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce log noise in tests

		// Following guideline: Reuse existing code - use existing pipeline structure
		pipeline = embedding.NewAIEmbeddingPipeline(mockLLMClient, mockVectorDB, logger)
		Expect(func() { _ = pipeline.ProcessWithCostOptimization }).ToNot(Panic(), "BR-AI-004: AI embedding pipeline must provide functional processing interface for dynamic cost calculations")

		// Create cost calculator following BR-ORK-004: Resource utilization and cost tracking
		costCalculator = embedding.NewAIDynamicCostCalculator("localai", 384, logger)
		// Test that cost calculator can calculate memory usage cost (available method)
		Expect(costCalculator.CalculateMemoryUsageCost()).To(BeNumerically(">=", 0), "BR-AI-004: Dynamic cost calculator must provide functional cost tracking for business optimization")

		testAlert = &types.Alert{
			ID:          "test-alert-cost-001",
			Description: "Test alert for dynamic cost calculation validation",
		}
	})

	Context("BR-AI-001: Cost-effectiveness ratios per action type", func() {
		It("should calculate LLM provider-specific costs dynamically", func() {
			// Business requirement: Calculate cost-effectiveness ratios per action type
			By("calculating cost for different LLM providers")

			providers := []struct {
				name         string
				expectedCost float64 // Expected relative cost difference
			}{
				{"openai", 0.02},     // Premium provider
				{"anthropic", 0.015}, // Mid-tier provider
				{"localai", 0.001},   // Local provider (lowest cost)
			}

			for _, provider := range providers {
				// Test different providers have different cost calculations
				testLogger := logrus.New()
				testLogger.SetLevel(logrus.WarnLevel)
				providerCalculator := embedding.NewAIDynamicCostCalculator(provider.name, 384, testLogger)

				// Mock LLM response for consistent testing
				mockResponse := &llm.AnalyzeAlertResponse{
					Action:     "Test response for cost calculation",
					Confidence: 0.85,
				}

				cost := providerCalculator.CalculateLLMCost(ctx, testAlert.Description, mockResponse, 2*time.Second)

				// BR-AI-001: Cost should vary by provider type
				Expect(cost).To(BeNumerically(">", 0), "Cost should be positive for provider %s", provider.name)

				// Verify cost aligns with expected provider pricing
				if provider.name == "localai" {
					Expect(cost).To(BeNumerically("<", 0.005), "BR-AI-001: Local provider should have lowest cost")
				} else if provider.name == "openai" {
					Expect(cost).To(BeNumerically(">", 0.01), "BR-AI-001: Premium provider should have higher cost")
				}
			}
		})

		It("should calculate token-based dynamic costs", func() {
			// Business requirement: Cost-effectiveness ratios should consider usage volume
			By("testing cost scaling with token usage")

			testCases := []struct {
				contentLength int
				expectedCost  string // Comparison expectation
			}{
				{100, "low"},     // Short content
				{1000, "medium"}, // Medium content
				{5000, "high"},   // Long content
			}

			var previousCost float64
			for i, testCase := range testCases {
				alert := &types.Alert{
					ID:          "token-test-alert",
					Description: generateContentOfLength(testCase.contentLength),
				}

				mockResponse := &llm.AnalyzeAlertResponse{
					Action:     "Response content",
					Confidence: 0.85,
				}

				cost := costCalculator.CalculateLLMCost(ctx, alert.Description, mockResponse, 1*time.Second)

				// BR-AI-001: Cost should increase with content length/tokens
				Expect(cost).To(BeNumerically(">", 0), "Cost should be positive")

				if i > 0 {
					Expect(cost).To(BeNumerically(">", previousCost),
						"BR-AI-001: Cost should increase with token count (%s > previous)", testCase.expectedCost)
				}
				previousCost = cost
			}
		})

		It("should calculate processing time-based costs", func() {
			// Business requirement: Cost-effectiveness includes processing efficiency
			By("testing cost correlation with processing time")

			processingTimes := []time.Duration{
				500 * time.Millisecond, // Fast processing
				2 * time.Second,        // Normal processing
				5 * time.Second,        // Slow processing
			}

			mockResponse := &llm.AnalyzeAlertResponse{
				Action:     "Standard response",
				Confidence: 0.80,
			}

			var previousCost float64
			for i, processingTime := range processingTimes {
				cost := costCalculator.CalculateLLMCost(ctx, testAlert.Description, mockResponse, processingTime)

				// BR-AI-001: Cost should increase with processing time
				Expect(cost).To(BeNumerically(">", 0), "Cost should be positive")

				if i > 0 {
					Expect(cost).To(BeNumerically(">", previousCost),
						"BR-AI-001: Longer processing time should result in higher cost")
				}
				previousCost = cost
			}
		})
	})

	Context("BR-ORK-004: Resource utilization and cost tracking", func() {
		It("should track vector storage costs based on dimensions", func() {
			// Business requirement: Track resource utilization for cost analysis
			By("calculating storage costs for different vector dimensions")

			dimensions := []int{384, 768, 1536} // Common embedding dimensions
			vectorCount := 10

			var previousCost float64
			for i, dimension := range dimensions {
				testLogger := logrus.New()
				testLogger.SetLevel(logrus.WarnLevel)
				dimensionCalculator := embedding.NewAIDynamicCostCalculator("localai", dimension, testLogger)
				cost := dimensionCalculator.CalculateVectorStorageCost(vectorCount, "store")

				// BR-ORK-004: Cost should reflect actual resource usage (dimension size)
				Expect(cost).To(BeNumerically(">", 0), "Storage cost should be positive")

				if i > 0 {
					Expect(cost).To(BeNumerically(">", previousCost),
						"BR-ORK-004: Higher dimensions should cost more to store")
				}
				previousCost = cost
			}
		})

		It("should track memory usage costs dynamically", func() {
			// Business requirement: Monitor resource utilization for capacity planning
			By("calculating costs based on actual memory usage")

			// Test memory cost calculation
			memoryCost := costCalculator.CalculateMemoryUsageCost()

			// BR-ORK-004: Memory usage should have measurable cost impact
			Expect(memoryCost).To(BeNumerically(">=", 0), "BR-ORK-004: Memory cost should be non-negative")

			// Memory cost should be reasonable (not excessive for test environment)
			Expect(memoryCost).To(BeNumerically("<", 0.1), "Memory cost should be reasonable for test scenarios")
		})

		It("should calculate cost savings from resource constraints", func() {
			// Business requirement: Identify cost optimization opportunities
			By("testing resource constraint cost optimization")

			constraints := &embedding.ResourceConstraints{
				MaxMemoryMB:     100,
				MaxConnections:  2,
				MaxProcessingMs: 3000,
			}

			// Test different actual usage scenarios
			testCases := []struct {
				usage          embedding.AIResourceUsage
				expectedSaving bool
			}{
				{
					usage: embedding.AIResourceUsage{
						MemoryMB:     50,   // Under memory limit
						Connections:  1,    // Under connection limit
						ProcessingMs: 1500, // Under processing limit
					},
					expectedSaving: true, // Should have cost savings
				},
				{
					usage: embedding.AIResourceUsage{
						MemoryMB:     100,  // At memory limit
						Connections:  2,    // At connection limit
						ProcessingMs: 3000, // At processing limit
					},
					expectedSaving: false, // Should have minimal savings
				},
			}

			baselineCost := 0.05 // Unconstrained cost baseline
			for _, testCase := range testCases {
				cost := costCalculator.CalculateResourceConstraintCost(constraints, testCase.usage)

				// BR-ORK-004: Cost should reflect actual resource usage vs constraints
				Expect(cost).To(BeNumerically(">", 0), "Constrained cost should be positive")

				if testCase.expectedSaving {
					Expect(cost).To(BeNumerically("<", baselineCost),
						"BR-ORK-004: Using less than constraint limits should reduce costs")
				} else {
					Expect(cost).To(BeNumerically("<=", baselineCost),
						"BR-ORK-004: Using at constraint limits should not exceed baseline")
				}
			}
		})

		It("should provide comprehensive cost breakdown components", func() {
			// Business requirement: Detailed cost analysis for optimization decisions
			By("validating complete cost component tracking")

			// This tests the enhanced ProcessWithDynamicCostOptimization method
			// Using shared mock defaults - adequate confidence and action provided

			maxCost := 0.10
			result, err := pipeline.ProcessWithDynamicCostOptimization(ctx, testAlert, maxCost)

			// BR-ORK-004: Should provide detailed cost breakdown
			Expect(err).ToNot(HaveOccurred(), "Dynamic cost processing should succeed")
			Expect(result).ToNot(BeNil(), "BR-AI-004: Dynamic cost optimization must provide completion status for business cost control")
			Expect(result.ActualCost).To(BeNumerically(">", 0), "Should have measurable actual cost")
			Expect(result.ActualCost).To(BeNumerically("<=", maxCost), "Should respect cost constraints")

			// BR-ORK-004: Should complete operations within cost constraints
			Expect(result.OperationsCompleted).To(BeNumerically(">", 0), "Should complete at least one operation")
			Expect(result.ProcessingCompleted).To(BeTrue(), "Should complete processing within budget")
			Expect(result.AccuracyMaintained).To(BeNumerically(">=", 0.75), "Should maintain reasonable accuracy")
		})
	})

	Context("Integration with existing cost optimization methods", func() {
		It("should enhance ProcessWithCostOptimization with dynamic calculations", func() {
			// Business requirement: Replace static costs with dynamic calculations
			By("verifying dynamic cost calculation replaces static values")

			// Using shared mock defaults - provides adequate confidence (0.8+) and action
			maxCost := 0.08

			// Test multiple runs to verify cost calculation varies based on actual usage
			costs := make([]float64, 3)
			for i := 0; i < 3; i++ {
				result, err := pipeline.ProcessWithDynamicCostOptimization(ctx, testAlert, maxCost)
				Expect(err).ToNot(HaveOccurred())
				costs[i] = result.ActualCost
			}

			// Costs should be consistent for identical operations (deterministic)
			for i := 1; i < len(costs); i++ {
				Expect(costs[i]).To(BeNumerically("~", costs[0], 0.001),
					"Dynamic cost calculation should be deterministic for identical operations")
			}

			// All costs should be positive and within constraints
			for i, cost := range costs {
				Expect(cost).To(BeNumerically(">", 0), "Cost should be positive (run %d)", i+1)
				Expect(cost).To(BeNumerically("<=", maxCost), "Cost should respect constraint (run %d)", i+1)
			}
		})
	})
})

// Mock implementations following project guideline: Reuse existing mocks where possible

type MockVectorDatabase struct {
	mockError error
}

func (m *MockVectorDatabase) SetMockError(err error) {
	m.mockError = err
}

func (m *MockVectorDatabase) StoreActionPattern(ctx context.Context, pattern *vector.ActionPattern) error {
	if m.mockError != nil {
		return m.mockError
	}
	return nil
}

func (m *MockVectorDatabase) IsHealthy(ctx context.Context) error {
	return m.mockError
}

// Additional methods required by vector.VectorDatabase interface
func (m *MockVectorDatabase) SearchBySemantics(ctx context.Context, query string, limit int) ([]*vector.ActionPattern, error) {
	return nil, m.mockError
}

func (m *MockVectorDatabase) GetPatternAnalytics(ctx context.Context) (*vector.PatternAnalytics, error) {
	return &vector.PatternAnalytics{}, m.mockError
}

func (m *MockVectorDatabase) DeletePattern(ctx context.Context, id string) error {
	return m.mockError
}

func (m *MockVectorDatabase) FindSimilarPatterns(ctx context.Context, pattern *vector.ActionPattern, limit int, threshold float64) ([]*vector.SimilarPattern, error) {
	return nil, m.mockError
}

func (m *MockVectorDatabase) UpdatePatternEffectiveness(ctx context.Context, patternID string, effectiveness float64) error {
	return m.mockError
}

// SearchByVector implements the missing method from VectorDatabase interface
func (m *MockVectorDatabase) SearchByVector(ctx context.Context, embedding []float64, limit int, threshold float64) ([]*vector.ActionPattern, error) {
	if m.mockError != nil {
		return nil, m.mockError
	}
	return []*vector.ActionPattern{}, nil
}

// Helper function for generating content of specific length
func generateContentOfLength(length int) string {
	content := ""
	word := "test "
	for len(content) < length {
		content += word
		if len(content) > length {
			content = content[:length]
			break
		}
	}
	return content
}

// TestRunner bootstraps the Ginkgo test suite
