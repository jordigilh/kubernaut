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

package holmesgpt_test

import (
	"testing"
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/sirupsen/logrus"
)

var _ = Describe("Enhanced HolmesGPT Provider Methods", func() {
	var (
		holmesClient holmesgpt.Client
		ctx          context.Context
		logger       *logrus.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)

		// Create HolmesGPT client with test configuration
		endpoint := "http://localhost:8090"
		apiKey := "test-key"

		var err error
		holmesClient, err = holmesgpt.NewClient(endpoint, apiKey, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	// Enhanced AI Provider Methods replacing Rule 12 violating interfaces
	Describe("Analysis Provider Methods", func() {
		It("should analyze requests replacing AnalysisProvider interface", func() {
			// Test data for analysis - BR-ANALYSIS-001
			analysisRequest := map[string]interface{}{
				"id":            "analysis-req-1",
				"subject":       "pod-crash-loop-analysis",
				"data":          map[string]interface{}{"namespace": "production", "pod": "api-server"},
				"analysis_type": "root_cause_analysis",
				"context":       map[string]interface{}{"cluster": "prod-cluster-1"},
			}

			// Enhanced holmesgpt.Client with ProvideAnalysis method
			analysisResult, err := holmesClient.ProvideAnalysis(ctx, analysisRequest)

			Expect(err).ToNot(HaveOccurred())
			Expect(analysisResult).ToNot(BeNil())
		})

		It("should get client capabilities for analysis services", func() {
			// Enhanced holmesgpt.Client with GetProviderCapabilities method
			capabilities, err := holmesClient.GetProviderCapabilities(ctx)

			Expect(err).ToNot(HaveOccurred())
			Expect(capabilities).ToNot(BeEmpty())
			Expect(capabilities).To(ContainElement("analysis"))
			Expect(capabilities).To(ContainElement("investigation"))
		})

		It("should get provider identification", func() {
			// Enhanced holmesgpt.Client with GetProviderID method
			providerID, err := holmesClient.GetProviderID(ctx)

			Expect(err).ToNot(HaveOccurred())
			Expect(providerID).ToNot(BeEmpty())
			Expect(providerID).To(Equal("holmesgpt-provider"))
		})
	})

	Describe("Recommendation Provider Methods", func() {
		It("should generate recommendations replacing RecommendationProvider interface", func() {
			// Test data for recommendation generation - BR-RECOMMENDATION-001
			recommendationContext := map[string]interface{}{
				"alert_type":    "PodCrashLooping",
				"namespace":     "production",
				"severity":      "high",
				"history":       []interface{}{},
				"cluster_state": map[string]interface{}{"node_count": 3, "available_resources": "high"},
				"user_context":  map[string]interface{}{"environment": "production", "criticality": "high"},
			}

			// Enhanced holmesgpt.Client with GenerateProviderRecommendations method
			recommendations, err := holmesClient.GenerateProviderRecommendations(ctx, recommendationContext)

			Expect(err).ToNot(HaveOccurred())
			Expect(recommendations).ToNot(BeNil())
			Expect(len(recommendations)).To(BeNumerically(">", 0))
		})

		It("should validate recommendation context", func() {
			// Test data for recommendation context validation
			recommendationContext := map[string]interface{}{
				"alert_type": "InvalidAlertType",
				"namespace":  "",
				"severity":   "unknown",
			}

			// Enhanced holmesgpt.Client with ValidateRecommendationContext method
			isValid, err := holmesClient.ValidateRecommendationContext(ctx, recommendationContext)

			Expect(err).ToNot(HaveOccurred())
			Expect(isValid).To(BeFalse())
		})

		It("should prioritize recommendations based on context", func() {
			// Test data for recommendation prioritization
			recommendations := []interface{}{
				map[string]interface{}{
					"id":         "rec-1",
					"type":       "restart_pod",
					"priority":   "high",
					"confidence": 0.9,
				},
				map[string]interface{}{
					"id":         "rec-2",
					"type":       "scale_deployment",
					"priority":   "medium",
					"confidence": 0.7,
				},
			}

			// Enhanced holmesgpt.Client with PrioritizeRecommendations method
			prioritizedRecs, err := holmesClient.PrioritizeRecommendations(ctx, recommendations)

			Expect(err).ToNot(HaveOccurred())
			Expect(prioritizedRecs).ToNot(BeNil())
			Expect(len(prioritizedRecs)).To(Equal(2))
		})
	})

	Describe("Investigation Provider Methods", func() {
		It("should investigate alerts replacing InvestigationProvider interface", func() {
			// Test data for alert investigation - BR-INVESTIGATION-001
			alert := &types.Alert{
				Name:        "pod-crash-alert",
				Namespace:   "production",
				Severity:    "high",
				Description: "Pod has been crash looping for 10 minutes",
				Labels:      map[string]string{"app": "api-server", "version": "v1.2.3"},
			}

			investigationContext := map[string]interface{}{
				"investigation_depth": "comprehensive",
				"include_history":     true,
				"analyze_patterns":    true,
				"cluster_context":     map[string]interface{}{"region": "us-west-2", "size": "large"},
			}

			// Enhanced holmesgpt.Client with InvestigateAlert method
			investigationResult, err := holmesClient.InvestigateAlert(ctx, alert, investigationContext)

			Expect(err).ToNot(HaveOccurred())
			Expect(investigationResult).ToNot(BeNil())
		})

		It("should provide investigation capabilities", func() {
			// Test investigation capability discovery
			capabilities := []string{"root_cause_analysis", "pattern_detection", "historical_correlation"}

			// Enhanced holmesgpt.Client with GetInvestigationCapabilities method
			availableCapabilities, err := holmesClient.GetInvestigationCapabilities(ctx)

			Expect(err).ToNot(HaveOccurred())
			Expect(availableCapabilities).ToNot(BeEmpty())

			for _, expectedCap := range capabilities {
				Expect(availableCapabilities).To(ContainElement(expectedCap))
			}
		})

		It("should perform deep investigation analysis", func() {
			// Test data for deep investigation
			alert := &types.Alert{
				Name:        "memory-leak-alert",
				Namespace:   "staging",
				Severity:    "medium",
				Description: "Memory usage has been steadily increasing",
			}

			investigationDepth := "deep_analysis"

			// Enhanced holmesgpt.Client with PerformDeepInvestigation method
			deepResult, err := holmesClient.PerformDeepInvestigation(ctx, alert, investigationDepth)

			Expect(err).ToNot(HaveOccurred())
			Expect(deepResult).ToNot(BeNil())
		})
	})

	Describe("Provider Service Integration", func() {
		It("should handle provider service errors gracefully", func() {
			// Test error handling in provider services
			invalidRequest := map[string]interface{}{
				"invalid_field": "invalid_value",
			}

			// All provider methods should handle errors gracefully
			_, err := holmesClient.ProvideAnalysis(ctx, invalidRequest)
			Expect(err).To(HaveOccurred())

			_, err = holmesClient.GenerateProviderRecommendations(ctx, invalidRequest)
			Expect(err).To(HaveOccurred())
		})

		It("should validate provider service health", func() {
			// Enhanced holmesgpt.Client should provide health validation for provider services
			providerHealth, err := holmesClient.ValidateProviderHealth(ctx)

			Expect(err).ToNot(HaveOccurred())
			Expect(providerHealth).ToNot(BeNil())
		})

		It("should support provider service configuration", func() {
			// Test provider service configuration
			serviceConfig := map[string]interface{}{
				"analysis_timeout":       "30s",
				"recommendation_depth":   "comprehensive",
				"investigation_parallel": true,
			}

			// Enhanced holmesgpt.Client with ConfigureProviderServices method
			err := holmesClient.ConfigureProviderServices(ctx, serviceConfig)

			Expect(err).ToNot(HaveOccurred())
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUenhancedUproviderUmethods(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UenhancedUproviderUmethods Suite")
}
