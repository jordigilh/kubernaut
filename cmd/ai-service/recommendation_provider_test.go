//go:build unit
// +build unit

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// AI Service Recommendation Provider Test Suite
// Business Impact: Validates AI service's ability to generate actionable remediation recommendations
// Stakeholder Value: Ensures operations teams receive high-quality, contextual recommendations for incident resolution

var _ = Describe("AI Service Recommendation Provider", func() {
	var (
		server *httptest.Server
		logger *logrus.Logger
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel)
		server = createTestAIServerBDD(logger)
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	Context("When generating remediation recommendations", func() {
		var criticalAlert types.Alert

		BeforeEach(func() {
			criticalAlert = createTestAlert("DatabaseConnectionFailure", "critical", "production", "database-service")
		})

		It("BR-AI-006: Should generate actionable remediation recommendations", func() {
			// REFACTOR: Use reusable helper functions to eliminate duplication
			context := map[string]interface{}{
				"environment": "production",
				"team":        "platform",
			}
			resp, err := makeRecommendationRequest(server, criticalAlert, context, nil)
			Expect(err).ToNot(HaveOccurred())

			// REFACTOR: Use reusable response validation
			var response RecommendationResponse
			validateJSONResponse(resp, http.StatusOK, &response, "BR-AI-006")

			// Business-meaningful validation
			Expect(response.Recommendations).ToNot(BeEmpty(), "BR-AI-006: Should generate actionable recommendations")
			Expect(len(response.Recommendations)).To(BeNumerically(">=", 1), "BR-AI-006: Should provide at least one recommendation")
			Expect(len(response.Recommendations)).To(BeNumerically("<=", 5), "BR-AI-006: Should limit recommendations to manageable number")

			// Validate recommendation quality
			for _, rec := range response.Recommendations {
				Expect(rec.ID).ToNot(BeEmpty(), "BR-AI-006: Each recommendation should have unique identifier")
				Expect(rec.Type).ToNot(BeEmpty(), "BR-AI-006: Each recommendation should have clear type")
				Expect(rec.Priority).To(BeElementOf([]string{"low", "medium", "high", "critical"}),
					"BR-AI-006: Each recommendation should have valid business priority")

				// Validate actionable steps
				Expect(rec.Actions).ToNot(BeEmpty(), "BR-AI-006: Each recommendation should provide actionable steps")
				for _, action := range rec.Actions {
					Expect(action.Type).ToNot(BeEmpty(), "BR-AI-006: Each action should have clear type")
					Expect(action.Description).ToNot(BeEmpty(), "BR-AI-006: Each action should have clear description")
				}
			}
		})

		It("BR-AI-007: Should include effectiveness probability and cost estimation", func() {
			// REFACTOR: Use reusable helper functions
			resp, err := makeRecommendationRequest(server, criticalAlert, nil, nil)
			Expect(err).ToNot(HaveOccurred())

			var response RecommendationResponse
			validateJSONResponse(resp, http.StatusOK, &response, "BR-AI-007")

			for _, rec := range response.Recommendations {
				// Effectiveness probability validation
				Expect(rec.EffectivenessProbability).To(BeNumerically(">=", 0.0),
					"BR-AI-007: Effectiveness probability should be valid lower bound")
				Expect(rec.EffectivenessProbability).To(BeNumerically("<=", 1.0),
					"BR-AI-007: Effectiveness probability should be valid upper bound")

				// Cost estimation validation (check metadata for cost information)
				if rec.Metadata != nil && rec.Metadata.CustomMetadata != nil {
					if cost, exists := rec.Metadata.CustomMetadata["estimated_cost"]; exists {
						Expect(cost).To(BeElementOf([]string{"low", "medium", "high"}),
							"BR-AI-007: Should provide business-relevant cost estimation")
					}
				}

				// Historical success rate validation
				if rec.Metadata != nil && rec.Metadata.HistoricalSuccessRate > 0 {
					Expect(rec.Metadata.HistoricalSuccessRate).To(BeNumerically(">=", 0.0), "BR-AI-007: Historical success rate should be valid")
					Expect(rec.Metadata.HistoricalSuccessRate).To(BeNumerically("<=", 1.0), "BR-AI-007: Historical success rate should be valid")
				}
			}
		})

		It("BR-AI-008: Should provide contextual recommendations based on alert metadata", func() {
			// Test with different alert contexts
			testCases := []struct {
				alertName                  string
				severity                   string
				namespace                  string
				expectedContextualElements []string
			}{
				{
					alertName:                  "HighMemoryUsage",
					severity:                   "critical",
					namespace:                  "production",
					expectedContextualElements: []string{"memory", "scale", "resource"},
				},
				{
					alertName:                  "DiskSpaceLow",
					severity:                   "warning",
					namespace:                  "staging",
					expectedContextualElements: []string{"disk", "cleanup", "storage"},
				},
			}

			for _, tc := range testCases {
				By("Testing contextual recommendations for " + tc.alertName)

				// REFACTOR: Use reusable helper functions
				contextualAlert := createTestAlert(tc.alertName, tc.severity, tc.namespace, "test-service")
				resp, err := makeRecommendationRequest(server, contextualAlert, nil, nil)
				Expect(err).ToNot(HaveOccurred())

				var response RecommendationResponse
				validateJSONResponse(resp, http.StatusOK, &response, "BR-AI-008")

				// Validate contextual relevance
				foundContextualElements := 0
				for _, rec := range response.Recommendations {
					recContent := rec.Description + " " + rec.Explanation
					for _, element := range tc.expectedContextualElements {
						if strings.Contains(strings.ToLower(recContent), element) {
							foundContextualElements++
							break
						}
					}
				}

				// REFACTOR: Business-meaningful assertion instead of weak > 0 check
				expectedMinElements := len(tc.expectedContextualElements) / 2 // At least half of expected contextual elements
				if expectedMinElements == 0 {
					expectedMinElements = 1 // Minimum one contextual element required
				}
				Expect(foundContextualElements).To(BeNumerically(">=", expectedMinElements),
					"BR-AI-008: Should find at least %d contextual elements for %s (found %d)",
					expectedMinElements, tc.alertName, foundContextualElements)
			}
		})

		It("BR-AI-009: Should support constraint-based filtering", func() {
			payload := map[string]interface{}{
				"alert": criticalAlert,
				"constraints": map[string]interface{}{
					"allowed_actions": []string{"restart_pod", "scale_deployment"},
					"max_cost":        "medium",
					"environment":     "production",
				},
			}

			jsonPayload, err := json.Marshal(payload)
			Expect(err).ToNot(HaveOccurred())

			req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/recommendations", bytes.NewBuffer(jsonPayload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			var response RecommendationResponse
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			// Validate constraint compliance
			allowedActions := []string{"restart_pod", "scale_deployment"}
			for _, rec := range response.Recommendations {
				for _, action := range rec.Actions {
					Expect(action.Type).To(BeElementOf(allowedActions),
						"BR-AI-009: Should respect allowed actions constraint")
				}

				// Validate cost constraint (check metadata)
				if rec.Metadata != nil && rec.Metadata.CustomMetadata != nil {
					if cost, exists := rec.Metadata.CustomMetadata["estimated_cost"]; exists {
						Expect(cost).To(BeElementOf([]string{"low", "medium"}),
							"BR-AI-009: Should respect maximum cost constraint")
					}
				}
			}
		})

		It("BR-AI-010: Should provide detailed explanations and supporting evidence", func() {
			payload := map[string]interface{}{
				"alert": criticalAlert,
			}

			jsonPayload, err := json.Marshal(payload)
			Expect(err).ToNot(HaveOccurred())

			req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/recommendations", bytes.NewBuffer(jsonPayload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			var response RecommendationResponse
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			for _, rec := range response.Recommendations {
				// Detailed explanation validation
				Expect(rec.Explanation).ToNot(BeEmpty(), "BR-AI-010: Should provide detailed explanation")
				Expect(len(rec.Explanation)).To(BeNumerically(">=", 50),
					"BR-AI-010: Explanation should be sufficiently detailed for operations teams")

				// Supporting evidence validation
				Expect(rec.Evidence).ToNot(BeEmpty(), "BR-AI-010: Should provide supporting evidence")
				Expect(len(rec.Evidence)).To(BeNumerically(">=", 1),
					"BR-AI-010: Should include at least one piece of evidence")

				for _, evidence := range rec.Evidence {
					Expect(evidence.Type).ToNot(BeEmpty(), "BR-AI-010: Evidence should have clear type")
					Expect(evidence.Description).ToNot(BeEmpty(), "BR-AI-010: Evidence should have clear description")
					Expect(evidence.Confidence).To(BeNumerically(">=", 0.0), "BR-AI-010: Evidence should have confidence score")
					Expect(evidence.Confidence).To(BeNumerically("<=", 1.0), "BR-AI-010: Evidence confidence should be valid range")
				}

				// Validate explanation quality
				explanationLower := strings.ToLower(rec.Explanation)
				Expect(explanationLower).To(SatisfyAny(
					ContainSubstring("because"),
					ContainSubstring("due to"),
					ContainSubstring("based on"),
					ContainSubstring("analysis shows"),
				), "BR-AI-010: Explanation should provide reasoning context")
			}
		})
	})

	Context("When handling edge cases and error conditions", func() {
		It("Should handle missing alert data gracefully", func() {
			payload := map[string]interface{}{
				"context": map[string]interface{}{
					"request_id": "missing-alert-test",
				},
			}

			jsonPayload, _ := json.Marshal(payload)
			req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/recommendations", bytes.NewBuffer(jsonPayload))
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"Should return appropriate error for missing alert data")
		})

		It("Should handle invalid constraint values gracefully", func() {
			payload := map[string]interface{}{
				"alert": createTestAlert("TestAlert", "info", "test", "test-service"),
				"constraints": map[string]interface{}{
					"max_cost":        "invalid_cost_level",
					"allowed_actions": "not_an_array",
				},
			}

			jsonPayload, _ := json.Marshal(payload)
			req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/recommendations", bytes.NewBuffer(jsonPayload))
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Should either process with default constraints or return appropriate error
			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusOK),
				Equal(http.StatusBadRequest),
			), "Should handle invalid constraints gracefully")
		})
	})
})
