package vector_test

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

var _ = Describe("Vector Interface Data Structures", func() {

	Describe("ActionPattern", func() {
		var pattern *vector.ActionPattern

		BeforeEach(func() {
			pattern = &vector.ActionPattern{
				ID:            "test-pattern-1",
				ActionType:    "scale_deployment",
				AlertName:     "HighMemoryUsage",
				AlertSeverity: "warning",
				Namespace:     "production",
				ResourceType:  "deployment",
				ResourceName:  "web-app",
				ActionParameters: map[string]interface{}{
					"replicas": 5,
					"strategy": "RollingUpdate",
				},
				ContextLabels: map[string]string{
					"app":     "web-app",
					"version": "1.2.3",
				},
				PreConditions: map[string]interface{}{
					"memory_usage":   ">80%",
					"ready_replicas": 3,
				},
				PostConditions: map[string]interface{}{
					"execution_status": "success",
					"new_replicas":     5,
				},
				EffectivenessData: &vector.EffectivenessData{
					Score:                0.85,
					SuccessCount:         10,
					FailureCount:         2,
					AverageExecutionTime: 45 * time.Second,
					SideEffectsCount:     1,
					RecurrenceRate:       0.15,
					CostImpact: &vector.CostImpact{
						ResourceCostDelta:   15.50,
						OperationalCost:     2.00,
						SavingsPotential:    100.00,
						CostEfficiencyRatio: 6.45,
					},
					ContextualFactors: map[string]float64{
						"hour_of_day":        14.0,
						"day_of_week":        3.0,
						"cluster_load":       0.75,
						"namespace_activity": 0.60,
					},
					LastAssessed: time.Now(),
				},
				Embedding: []float64{0.1, 0.2, 0.3, 0.4, 0.5},
				CreatedAt: time.Now().Add(-time.Hour),
				UpdatedAt: time.Now(),
				Metadata: map[string]interface{}{
					"created_by": "system",
					"version":    "1.0",
					"source":     "prometheus",
				},
			}
		})

		Context("JSON Serialization", func() {
			It("should serialize to JSON correctly", func() {
				jsonData, err := json.Marshal(pattern)

				Expect(err).NotTo(HaveOccurred())
				Expect(jsonData).NotTo(BeEmpty())

				// Verify key fields are present in JSON
				jsonString := string(jsonData)
				Expect(jsonString).To(ContainSubstring("test-pattern-1"))
				Expect(jsonString).To(ContainSubstring("scale_deployment"))
				Expect(jsonString).To(ContainSubstring("HighMemoryUsage"))
				Expect(jsonString).To(ContainSubstring("effectiveness_data"))
			})

			It("should deserialize from JSON correctly", func() {
				// First serialize
				jsonData, err := json.Marshal(pattern)
				Expect(err).NotTo(HaveOccurred())

				// Then deserialize
				var deserializedPattern vector.ActionPattern
				err = json.Unmarshal(jsonData, &deserializedPattern)

				Expect(err).NotTo(HaveOccurred())
				Expect(deserializedPattern.ID).To(Equal(pattern.ID))
				Expect(deserializedPattern.ActionType).To(Equal(pattern.ActionType))
				Expect(deserializedPattern.AlertName).To(Equal(pattern.AlertName))
				Expect(deserializedPattern.EffectivenessData.Score).To(Equal(pattern.EffectivenessData.Score))
				Expect(deserializedPattern.Embedding).To(Equal(pattern.Embedding))
			})

			It("should handle nil effectiveness data", func() {
				pattern.EffectivenessData = nil

				jsonData, err := json.Marshal(pattern)
				Expect(err).NotTo(HaveOccurred())

				var deserializedPattern vector.ActionPattern
				err = json.Unmarshal(jsonData, &deserializedPattern)
				Expect(err).NotTo(HaveOccurred())
				Expect(deserializedPattern.EffectivenessData).To(BeNil())
			})
		})

		Context("Data Validation", func() {
			It("should have valid required fields", func() {
				Expect(pattern.ID).NotTo(BeEmpty())
				Expect(pattern.ActionType).NotTo(BeEmpty())
				Expect(pattern.AlertName).NotTo(BeEmpty())
				Expect(pattern.AlertSeverity).NotTo(BeEmpty())
			})

			It("should have valid timestamps", func() {
				Expect(pattern.CreatedAt).NotTo(BeZero())
				Expect(pattern.UpdatedAt).NotTo(BeZero())
				Expect(pattern.UpdatedAt.After(pattern.CreatedAt) || pattern.UpdatedAt.Equal(pattern.CreatedAt)).To(BeTrue())
			})

			It("should have valid embedding dimensions", func() {
				if len(pattern.Embedding) > 0 {
					Expect(len(pattern.Embedding)).To(BeNumerically(">", 0))
					Expect(len(pattern.Embedding)).To(BeNumerically("<=", 4096)) // Reasonable upper bound
				}
			})
		})
	})

	Describe("EffectivenessData", func() {
		var effectivenessData *vector.EffectivenessData

		BeforeEach(func() {
			effectivenessData = &vector.EffectivenessData{
				Score:                0.75,
				SuccessCount:         8,
				FailureCount:         2,
				AverageExecutionTime: 30 * time.Second,
				SideEffectsCount:     1,
				RecurrenceRate:       0.20,
				CostImpact: &vector.CostImpact{
					ResourceCostDelta:   -10.50, // Negative means cost reduction
					OperationalCost:     1.50,
					SavingsPotential:    50.00,
					CostEfficiencyRatio: 33.33,
				},
				ContextualFactors: map[string]float64{
					"cluster_health":     0.95,
					"resource_pressure":  0.60,
					"time_of_day_factor": 0.80,
				},
				LastAssessed: time.Now(),
			}
		})

		Context("Score Validation", func() {
			It("should have score between 0 and 1", func() {
				Expect(effectivenessData.Score).To(BeNumerically(">=", 0.0))
				Expect(effectivenessData.Score).To(BeNumerically("<=", 1.0))
			})

			It("should calculate success rate correctly", func() {
				successRate := float64(effectivenessData.SuccessCount) / float64(effectivenessData.SuccessCount+effectivenessData.FailureCount)
				Expect(successRate).To(BeNumerically("~", 0.8, 0.01)) // 8/(8+2) = 0.8
			})
		})

		Context("Cost Impact Validation", func() {
			It("should have valid cost impact data", func() {
				Expect(effectivenessData.CostImpact).NotTo(BeNil())

				// Cost efficiency ratio should be positive
				Expect(effectivenessData.CostImpact.CostEfficiencyRatio).To(BeNumerically(">", 0))

				// Savings potential should be non-negative
				Expect(effectivenessData.CostImpact.SavingsPotential).To(BeNumerically(">=", 0))
			})

			It("should serialize cost impact correctly", func() {
				jsonData, err := json.Marshal(effectivenessData)
				Expect(err).NotTo(HaveOccurred())

				var deserialized vector.EffectivenessData
				err = json.Unmarshal(jsonData, &deserialized)
				Expect(err).NotTo(HaveOccurred())

				Expect(deserialized.CostImpact).NotTo(BeNil())
				Expect(deserialized.CostImpact.ResourceCostDelta).To(Equal(effectivenessData.CostImpact.ResourceCostDelta))
			})
		})

		Context("Contextual Factors", func() {
			It("should have valid contextual factors", func() {
				for factor, value := range effectivenessData.ContextualFactors {
					Expect(factor).NotTo(BeEmpty())
					Expect(value).To(BeNumerically(">=", 0.0))
					Expect(value).To(BeNumerically("<=", 1.0))
				}
			})
		})
	})

	Describe("SimilarPattern", func() {
		var similarPattern *vector.SimilarPattern

		BeforeEach(func() {
			pattern := &vector.ActionPattern{
				ID:         "similar-pattern-1",
				ActionType: "restart_pod",
				AlertName:  "PodCrashing",
			}

			similarPattern = &vector.SimilarPattern{
				Pattern:    pattern,
				Similarity: 0.92,
				Rank:       1,
			}
		})

		Context("Similarity Validation", func() {
			It("should have valid similarity score", func() {
				Expect(similarPattern.Similarity).To(BeNumerically(">=", 0.0))
				Expect(similarPattern.Similarity).To(BeNumerically("<=", 1.0))
			})

			It("should have valid rank", func() {
				Expect(similarPattern.Rank).To(BeNumerically(">=", 1))
			})

			It("should have non-nil pattern", func() {
				Expect(similarPattern.Pattern).NotTo(BeNil())
			})
		})

		Context("JSON Serialization", func() {
			It("should serialize with pattern data", func() {
				jsonData, err := json.Marshal(similarPattern)
				Expect(err).NotTo(HaveOccurred())

				jsonString := string(jsonData)
				Expect(jsonString).To(ContainSubstring("similarity"))
				Expect(jsonString).To(ContainSubstring("rank"))
				Expect(jsonString).To(ContainSubstring("pattern"))
				Expect(jsonString).To(ContainSubstring("similar-pattern-1"))
			})
		})
	})

	Describe("PatternAnalytics", func() {
		var analytics *vector.PatternAnalytics

		BeforeEach(func() {
			analytics = &vector.PatternAnalytics{
				TotalPatterns: 100,
				PatternsByActionType: map[string]int{
					"scale_deployment": 45,
					"restart_pod":      25,
					"update_config":    20,
					"drain_node":       10,
				},
				PatternsBySeverity: map[string]int{
					"critical": 20,
					"warning":  60,
					"info":     20,
				},
				AverageEffectiveness: 0.78,
				TopPerformingPatterns: []*vector.ActionPattern{
					{
						ID:                "top-1",
						ActionType:        "scale_deployment",
						EffectivenessData: &vector.EffectivenessData{Score: 0.95},
					},
					{
						ID:                "top-2",
						ActionType:        "restart_pod",
						EffectivenessData: &vector.EffectivenessData{Score: 0.90},
					},
				},
				RecentPatterns: []*vector.ActionPattern{
					{
						ID:        "recent-1",
						CreatedAt: time.Now().Add(-time.Hour),
					},
					{
						ID:        "recent-2",
						CreatedAt: time.Now().Add(-2 * time.Hour),
					},
				},
				EffectivenessDistribution: map[string]int{
					"excellent": 15,
					"very_good": 25,
					"good":      35,
					"fair":      20,
					"poor":      5,
				},
				GeneratedAt: time.Now(),
			}
		})

		Context("Data Consistency", func() {
			It("should have consistent pattern counts", func() {
				// Sum of patterns by action type should equal total patterns
				totalByActionType := 0
				for _, count := range analytics.PatternsByActionType {
					totalByActionType += count
				}
				Expect(totalByActionType).To(Equal(analytics.TotalPatterns))

				// Sum of patterns by severity should equal total patterns
				totalBySeverity := 0
				for _, count := range analytics.PatternsBySeverity {
					totalBySeverity += count
				}
				Expect(totalBySeverity).To(Equal(analytics.TotalPatterns))
			})

			It("should have valid effectiveness score", func() {
				Expect(analytics.AverageEffectiveness).To(BeNumerically(">=", 0.0))
				Expect(analytics.AverageEffectiveness).To(BeNumerically("<=", 1.0))
			})

			It("should have ordered top performing patterns", func() {
				if len(analytics.TopPerformingPatterns) > 1 {
					for i := 1; i < len(analytics.TopPerformingPatterns); i++ {
						prev := analytics.TopPerformingPatterns[i-1]
						curr := analytics.TopPerformingPatterns[i]

						if prev.EffectivenessData != nil && curr.EffectivenessData != nil {
							Expect(prev.EffectivenessData.Score).To(BeNumerically(">=", curr.EffectivenessData.Score))
						}
					}
				}
			})

			It("should have ordered recent patterns", func() {
				if len(analytics.RecentPatterns) > 1 {
					for i := 1; i < len(analytics.RecentPatterns); i++ {
						prev := analytics.RecentPatterns[i-1]
						curr := analytics.RecentPatterns[i]

						// Recent patterns should be ordered by creation time (newest first)
						Expect(prev.CreatedAt.After(curr.CreatedAt) || prev.CreatedAt.Equal(curr.CreatedAt)).To(BeTrue())
					}
				}
			})
		})

		Context("JSON Serialization", func() {
			It("should serialize complete analytics", func() {
				jsonData, err := json.Marshal(analytics)
				Expect(err).NotTo(HaveOccurred())

				var deserialized vector.PatternAnalytics
				err = json.Unmarshal(jsonData, &deserialized)
				Expect(err).NotTo(HaveOccurred())

				Expect(deserialized.TotalPatterns).To(Equal(analytics.TotalPatterns))
				Expect(deserialized.AverageEffectiveness).To(Equal(analytics.AverageEffectiveness))
				Expect(len(deserialized.TopPerformingPatterns)).To(Equal(len(analytics.TopPerformingPatterns)))
			})
		})
	})

	Describe("VectorSearchQuery", func() {
		var searchQuery *vector.VectorSearchQuery

		BeforeEach(func() {
			searchQuery = &vector.VectorSearchQuery{
				QueryText:     "memory usage scaling alert",
				QueryVector:   []float64{0.1, 0.2, 0.3, 0.4, 0.5},
				ActionTypes:   []string{"scale_deployment", "increase_resources"},
				Severities:    []string{"warning", "critical"},
				Namespaces:    []string{"production", "staging"},
				ResourceTypes: []string{"deployment", "statefulset"},
				DateRange: &vector.DateRange{
					From: time.Now().Add(-24 * time.Hour),
					To:   time.Now(),
				},
				Metadata: map[string]interface{}{
					"source":     "prometheus",
					"confidence": 0.8,
				},
				Limit:               10,
				SimilarityThreshold: 0.7,
				IncludeMetadata:     true,
			}
		})

		Context("Query Validation", func() {
			It("should have valid search parameters", func() {
				Expect(searchQuery.Limit).To(BeNumerically(">", 0))
				Expect(searchQuery.SimilarityThreshold).To(BeNumerically(">=", 0.0))
				Expect(searchQuery.SimilarityThreshold).To(BeNumerically("<=", 1.0))
			})

			It("should have valid date range", func() {
				if searchQuery.DateRange != nil {
					Expect(searchQuery.DateRange.To.After(searchQuery.DateRange.From) || searchQuery.DateRange.To.Equal(searchQuery.DateRange.From)).To(BeTrue())
				}
			})

			It("should handle either text or vector query", func() {
				// Either QueryText or QueryVector should be provided
				hasTextQuery := searchQuery.QueryText != ""
				hasVectorQuery := len(searchQuery.QueryVector) > 0

				Expect(hasTextQuery || hasVectorQuery).To(BeTrue())
			})
		})

		Context("JSON Serialization", func() {
			It("should serialize search query correctly", func() {
				jsonData, err := json.Marshal(searchQuery)
				Expect(err).NotTo(HaveOccurred())

				var deserialized vector.VectorSearchQuery
				err = json.Unmarshal(jsonData, &deserialized)
				Expect(err).NotTo(HaveOccurred())

				Expect(deserialized.QueryText).To(Equal(searchQuery.QueryText))
				Expect(deserialized.Limit).To(Equal(searchQuery.Limit))
				Expect(deserialized.SimilarityThreshold).To(Equal(searchQuery.SimilarityThreshold))
			})
		})
	})

	Describe("VectorSearchResult", func() {
		var searchResult *vector.VectorSearchResult

		BeforeEach(func() {
			patterns := []*vector.SimilarPattern{
				{
					Pattern: &vector.ActionPattern{
						ID:         "result-1",
						ActionType: "scale_deployment",
					},
					Similarity: 0.95,
					Rank:       1,
				},
				{
					Pattern: &vector.ActionPattern{
						ID:         "result-2",
						ActionType: "restart_pod",
					},
					Similarity: 0.88,
					Rank:       2,
				},
			}

			searchResult = &vector.VectorSearchResult{
				Patterns:    patterns,
				TotalCount:  2,
				SearchTime:  150 * time.Millisecond,
				QueryVector: []float64{0.1, 0.2, 0.3},
			}
		})

		Context("Result Validation", func() {
			It("should have consistent counts", func() {
				Expect(len(searchResult.Patterns)).To(Equal(searchResult.TotalCount))
			})

			It("should have ordered results by similarity", func() {
				if len(searchResult.Patterns) > 1 {
					for i := 1; i < len(searchResult.Patterns); i++ {
						prev := searchResult.Patterns[i-1]
						curr := searchResult.Patterns[i]

						Expect(prev.Similarity).To(BeNumerically(">=", curr.Similarity))
						Expect(prev.Rank).To(BeNumerically("<", curr.Rank))
					}
				}
			})

			It("should have valid search time", func() {
				Expect(searchResult.SearchTime).To(BeNumerically(">=", 0))
			})
		})

		Context("JSON Serialization", func() {
			It("should serialize search results correctly", func() {
				jsonData, err := json.Marshal(searchResult)
				Expect(err).NotTo(HaveOccurred())

				var deserialized vector.VectorSearchResult
				err = json.Unmarshal(jsonData, &deserialized)
				Expect(err).NotTo(HaveOccurred())

				Expect(deserialized.TotalCount).To(Equal(searchResult.TotalCount))
				Expect(len(deserialized.Patterns)).To(Equal(len(searchResult.Patterns)))
				Expect(deserialized.SearchTime).To(Equal(searchResult.SearchTime))
			})
		})
	})

	Describe("Data Structure Edge Cases", func() {
		Context("Empty and Nil Values", func() {
			It("should handle empty ActionPattern gracefully", func() {
				emptyPattern := &vector.ActionPattern{}

				jsonData, err := json.Marshal(emptyPattern)
				Expect(err).NotTo(HaveOccurred())

				var deserialized vector.ActionPattern
				err = json.Unmarshal(jsonData, &deserialized)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should handle nil slices and maps", func() {
				pattern := &vector.ActionPattern{
					ID:               "test",
					ActionParameters: nil,
					ContextLabels:    nil,
					Embedding:        nil,
				}

				jsonData, err := json.Marshal(pattern)
				Expect(err).NotTo(HaveOccurred())

				var deserialized vector.ActionPattern
				err = json.Unmarshal(jsonData, &deserialized)
				Expect(err).NotTo(HaveOccurred())
				Expect(deserialized.ID).To(Equal("test"))
			})
		})

		Context("Large Data Structures", func() {
			It("should handle large embeddings", func() {
				largeEmbedding := make([]float64, 2048)
				for i := 0; i < 2048; i++ {
					largeEmbedding[i] = float64(i) / 2048.0
				}

				pattern := &vector.ActionPattern{
					ID:        "large-embedding",
					Embedding: largeEmbedding,
				}

				jsonData, err := json.Marshal(pattern)
				Expect(err).NotTo(HaveOccurred())

				var deserialized vector.ActionPattern
				err = json.Unmarshal(jsonData, &deserialized)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(deserialized.Embedding)).To(Equal(2048))
			})

			It("should handle large metadata", func() {
				largeMetadata := make(map[string]interface{})
				for i := 0; i < 100; i++ {
					largeMetadata[fmt.Sprintf("key_%d", i)] = fmt.Sprintf("value_%d", i)
				}

				pattern := &vector.ActionPattern{
					ID:       "large-metadata",
					Metadata: largeMetadata,
				}

				jsonData, err := json.Marshal(pattern)
				Expect(err).NotTo(HaveOccurred())

				var deserialized vector.ActionPattern
				err = json.Unmarshal(jsonData, &deserialized)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(deserialized.Metadata)).To(Equal(100))
			})
		})
	})
})
