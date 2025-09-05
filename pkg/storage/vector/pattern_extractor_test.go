package vector_test

import (
	"context"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

var _ = Describe("DefaultPatternExtractor", func() {
	var (
		extractor *vector.DefaultPatternExtractor
		logger    *logrus.Logger
		ctx       context.Context
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.FatalLevel)                         // Suppress logs during tests
		extractor = vector.NewDefaultPatternExtractor(nil, logger) // No embedding generator for simple tests
		ctx = context.Background()
	})

	Describe("NewDefaultPatternExtractor", func() {
		It("should create a new pattern extractor", func() {
			extractor := vector.NewDefaultPatternExtractor(nil, logger)
			Expect(extractor).NotTo(BeNil())
		})
	})

	Describe("ExtractPattern", func() {
		Context("when extracting from a valid action trace", func() {
			It("should extract a complete pattern", func() {
				trace := createTestActionTrace()

				pattern, err := extractor.ExtractPattern(ctx, trace)

				Expect(err).NotTo(HaveOccurred())
				Expect(pattern).NotTo(BeNil())
				Expect(pattern.ID).NotTo(BeEmpty())
				Expect(pattern.ActionType).To(Equal("scale_deployment"))
				Expect(pattern.AlertName).To(Equal("HighMemoryUsage"))
				Expect(pattern.AlertSeverity).To(Equal("warning"))
				Expect(pattern.Namespace).To(Equal("production"))
				Expect(pattern.ResourceType).To(Equal("deployment"))
				Expect(pattern.ResourceName).To(Equal("web-app"))
			})

			It("should generate an embedding", func() {
				trace := createTestActionTrace()

				pattern, err := extractor.ExtractPattern(ctx, trace)

				Expect(err).NotTo(HaveOccurred())
				Expect(pattern.Embedding).NotTo(BeEmpty())
				Expect(len(pattern.Embedding)).To(Equal(128)) // Default simple embedding size
			})

			It("should extract effectiveness data when available", func() {
				trace := createTestActionTrace()
				effectiveness := 0.85
				assessedAt := time.Now()
				trace.EffectivenessScore = &effectiveness
				trace.EffectivenessAssessedAt = &assessedAt

				pattern, err := extractor.ExtractPattern(ctx, trace)

				Expect(err).NotTo(HaveOccurred())
				Expect(pattern.EffectivenessData).NotTo(BeNil())
				Expect(pattern.EffectivenessData.Score).To(Equal(0.85))
				Expect(pattern.EffectivenessData.LastAssessed).To(Equal(assessedAt))
				Expect(pattern.EffectivenessData.SuccessCount).To(Equal(1))
				Expect(pattern.EffectivenessData.FailureCount).To(Equal(0))
			})

			It("should handle low effectiveness scores as failures", func() {
				trace := createTestActionTrace()
				effectiveness := 0.3 // Low effectiveness
				trace.EffectivenessScore = &effectiveness

				pattern, err := extractor.ExtractPattern(ctx, trace)

				Expect(err).NotTo(HaveOccurred())
				Expect(pattern.EffectivenessData).NotTo(BeNil())
				Expect(pattern.EffectivenessData.Score).To(Equal(0.3))
				Expect(pattern.EffectivenessData.SuccessCount).To(Equal(0))
				Expect(pattern.EffectivenessData.FailureCount).To(Equal(1))
			})

			It("should extract execution duration", func() {
				trace := createTestActionTrace()
				duration := 5000 // 5 seconds in milliseconds
				trace.ExecutionDurationMs = &duration

				pattern, err := extractor.ExtractPattern(ctx, trace)

				Expect(err).NotTo(HaveOccurred())
				Expect(pattern.EffectivenessData).NotTo(BeNil())
				Expect(pattern.EffectivenessData.AverageExecutionTime).To(Equal(5 * time.Second))
			})

			It("should extract contextual factors", func() {
				trace := createTestActionTrace()

				pattern, err := extractor.ExtractPattern(ctx, trace)

				Expect(err).NotTo(HaveOccurred())
				Expect(pattern.EffectivenessData).NotTo(BeNil())
				Expect(pattern.EffectivenessData.ContextualFactors).NotTo(BeEmpty())
				Expect(pattern.EffectivenessData.ContextualFactors).To(HaveKey("model_confidence"))
			})

			It("should extract pre and post conditions", func() {
				trace := createTestActionTrace()

				pattern, err := extractor.ExtractPattern(ctx, trace)

				Expect(err).NotTo(HaveOccurred())
				Expect(pattern.PreConditions).NotTo(BeEmpty())
				Expect(pattern.PreConditions).To(HaveKey("alert_name"))
				Expect(pattern.PreConditions).To(HaveKey("alert_severity"))

				Expect(pattern.PostConditions).NotTo(BeEmpty())
				Expect(pattern.PostConditions).To(HaveKey("execution_status"))
			})

			It("should extract metadata", func() {
				trace := createTestActionTrace()

				pattern, err := extractor.ExtractPattern(ctx, trace)

				Expect(err).NotTo(HaveOccurred())
				Expect(pattern.Metadata).NotTo(BeEmpty())
				Expect(pattern.Metadata).To(HaveKey("action_id"))
				Expect(pattern.Metadata).To(HaveKey("model_used"))
				Expect(pattern.Metadata).To(HaveKey("model_confidence"))
			})
		})

		Context("when action trace is nil", func() {
			It("should return an error", func() {
				_, err := extractor.ExtractPattern(ctx, nil)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("action trace cannot be nil"))
			})
		})

		Context("when extracting from different action types", func() {
			It("should correctly identify resource types for different actions", func() {
				testCases := []struct {
					actionType       string
					expectedResource string
				}{
					{"scale_deployment", "deployment"},
					{"rollback_deployment", "deployment"},
					{"restart_pod", "pod"},
					{"delete_pod", "pod"},
					{"scale_statefulset", "statefulset"},
					{"drain_node", "node"},
					{"increase_resources", "resource"},
					{"unknown_action", "unknown"},
				}

				for _, tc := range testCases {
					trace := createTestActionTrace()
					trace.ActionType = tc.actionType

					pattern, err := extractor.ExtractPattern(ctx, trace)

					Expect(err).NotTo(HaveOccurred())
					Expect(pattern.ResourceType).To(Equal(tc.expectedResource),
						"Action type %s should map to resource type %s", tc.actionType, tc.expectedResource)
				}
			})
		})

		Context("when extracting resource names from different sources", func() {
			It("should prefer action parameters over alert labels", func() {
				trace := createTestActionTrace()
				// Set conflicting resource names
				trace.ActionParameters = actionhistory.JSONMap{
					"deployment": "param-deployment",
				}
				trace.AlertLabels = actionhistory.JSONMap{
					"deployment": "label-deployment",
				}

				pattern, err := extractor.ExtractPattern(ctx, trace)

				Expect(err).NotTo(HaveOccurred())
				Expect(pattern.ResourceName).To(Equal("param-deployment"))
			})

			It("should fallback to alert labels when no action parameters", func() {
				trace := createTestActionTrace()
				trace.ActionParameters = nil
				trace.AlertLabels = actionhistory.JSONMap{
					"app": "label-app",
				}

				pattern, err := extractor.ExtractPattern(ctx, trace)

				Expect(err).NotTo(HaveOccurred())
				Expect(pattern.ResourceName).To(Equal("label-app"))
			})

			It("should fallback to alert name when no other sources", func() {
				trace := createTestActionTrace()
				trace.ActionParameters = nil
				trace.AlertLabels = nil
				trace.AlertName = "FallbackAlert"

				pattern, err := extractor.ExtractPattern(ctx, trace)

				Expect(err).NotTo(HaveOccurred())
				Expect(pattern.ResourceName).To(Equal("FallbackAlert"))
			})
		})
	})

	Describe("ExtractFeatures", func() {
		Context("when extracting features from a pattern", func() {
			It("should extract comprehensive features", func() {
				pattern := createTestPatternForExtractor()

				features, err := extractor.ExtractFeatures(ctx, pattern)

				Expect(err).NotTo(HaveOccurred())
				Expect(features).NotTo(BeEmpty())

				// Check for expected feature categories
				Expect(features).To(HaveKey("action_type_hash"))
				Expect(features).To(HaveKey("alert_severity"))
				Expect(features).To(HaveKey("resource_type_hash"))
				Expect(features).To(HaveKey("namespace_criticality"))
				Expect(features).To(HaveKey("parameter_count"))
				Expect(features).To(HaveKey("context_label_count"))
			})

			It("should handle different alert severities", func() {
				severityTests := []struct {
					severity      string
					expectedScore float64
				}{
					{"critical", 1.0},
					{"warning", 0.7},
					{"info", 0.3},
					{"", 0.0},
					{"unknown", 0.0},
				}

				for _, test := range severityTests {
					pattern := createTestPatternForExtractor()
					pattern.AlertSeverity = test.severity

					features, err := extractor.ExtractFeatures(ctx, pattern)

					Expect(err).NotTo(HaveOccurred())
					Expect(features["alert_severity"]).To(Equal(test.expectedScore),
						"Severity %s should map to score %f", test.severity, test.expectedScore)
				}
			})

			It("should handle different namespace criticalities", func() {
				namespaceTests := []struct {
					namespace     string
					expectedScore float64
				}{
					{"production", 1.0},
					{"staging", 0.8},
					{"kube-system", 0.9},
					{"monitoring", 0.6},
					{"development", 0.5},
					{"default", 0.4},
					{"unknown-ns", 0.5}, // Default for unknown
				}

				for _, test := range namespaceTests {
					pattern := createTestPatternForExtractor()
					pattern.Namespace = test.namespace

					features, err := extractor.ExtractFeatures(ctx, pattern)

					Expect(err).NotTo(HaveOccurred())
					Expect(features["namespace_criticality"]).To(Equal(test.expectedScore),
						"Namespace %s should map to criticality %f", test.namespace, test.expectedScore)
				}
			})

			It("should extract time-based features", func() {
				pattern := createTestPatternForExtractor()
				testTime := time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC) // Friday 2:30 PM
				pattern.CreatedAt = testTime

				features, err := extractor.ExtractFeatures(ctx, pattern)

				Expect(err).NotTo(HaveOccurred())
				Expect(features).To(HaveKey("hour_of_day"))
				Expect(features).To(HaveKey("day_of_week"))

				// 14/24 = 0.583...
				Expect(features["hour_of_day"]).To(BeNumerically("~", 14.0/24.0, 0.01))
				// Friday is weekday 5, so 5/7 = 0.714...
				Expect(features["day_of_week"]).To(BeNumerically("~", 5.0/7.0, 0.01))
			})

			It("should extract effectiveness-based features when available", func() {
				pattern := createTestPatternForExtractor()
				pattern.EffectivenessData = &vector.EffectivenessData{
					Score:                0.85,
					SuccessCount:         8,
					FailureCount:         2,
					AverageExecutionTime: 45 * time.Second,
				}

				features, err := extractor.ExtractFeatures(ctx, pattern)

				Expect(err).NotTo(HaveOccurred())
				Expect(features).To(HaveKey("effectiveness_score"))
				Expect(features).To(HaveKey("success_rate"))
				Expect(features).To(HaveKey("execution_time_log"))

				Expect(features["effectiveness_score"]).To(Equal(0.85))
				Expect(features["success_rate"]).To(Equal(0.8)) // 8/(8+2) = 0.8
			})
		})
	})

	Describe("CalculateSimilarity", func() {
		Context("when comparing patterns with same embeddings", func() {
			It("should return high similarity", func() {
				pattern1 := createTestPatternForExtractor()
				pattern1.Embedding = []float64{1.0, 0.0, 0.0}

				pattern2 := createTestPatternForExtractor()
				pattern2.Embedding = []float64{1.0, 0.0, 0.0}

				similarity := extractor.CalculateSimilarity(pattern1, pattern2)

				Expect(similarity).To(BeNumerically("~", 1.0, 0.01))
			})
		})

		Context("when comparing patterns with different embeddings", func() {
			It("should return lower similarity", func() {
				pattern1 := createTestPatternForExtractor()
				pattern1.Embedding = []float64{1.0, 0.0, 0.0}

				pattern2 := createTestPatternForExtractor()
				pattern2.Embedding = []float64{0.0, 1.0, 0.0}

				similarity := extractor.CalculateSimilarity(pattern1, pattern2)

				Expect(similarity).To(BeNumerically("<", 0.5))
			})
		})

		Context("when patterns have different embedding dimensions", func() {
			It("should return zero similarity", func() {
				pattern1 := createTestPatternForExtractor()
				pattern1.Embedding = []float64{1.0, 0.0, 0.0}

				pattern2 := createTestPatternForExtractor()
				pattern2.Embedding = []float64{1.0, 0.0} // Different dimension

				similarity := extractor.CalculateSimilarity(pattern1, pattern2)

				Expect(similarity).To(Equal(0.0))
			})
		})

		Context("when patterns have same context", func() {
			It("should boost similarity score", func() {
				pattern1 := createTestPatternForExtractor()
				pattern1.Embedding = []float64{0.8, 0.6, 0.0} // Not perfect match
				pattern1.ActionType = "scale_deployment"
				pattern1.AlertSeverity = "critical"
				pattern1.Namespace = "production"
				pattern1.ResourceType = "deployment"

				pattern2 := createTestPatternForExtractor()
				pattern2.Embedding = []float64{0.7, 0.7, 0.0} // Not perfect match
				pattern2.ActionType = "scale_deployment"      // Same
				pattern2.AlertSeverity = "critical"           // Same
				pattern2.Namespace = "production"             // Same
				pattern2.ResourceType = "deployment"          // Same

				similarityWithContext := extractor.CalculateSimilarity(pattern1, pattern2)

				// Create patterns with same embeddings but different context
				pattern3 := createTestPatternForExtractor()
				pattern3.Embedding = []float64{0.8, 0.6, 0.0}
				pattern3.ActionType = "restart_pod" // Different
				pattern3.AlertSeverity = "warning"  // Different
				pattern3.Namespace = "development"  // Different
				pattern3.ResourceType = "pod"       // Different

				pattern4 := createTestPatternForExtractor()
				pattern4.Embedding = []float64{0.7, 0.7, 0.0}
				pattern4.ActionType = "delete_pod" // Different
				pattern4.AlertSeverity = "info"    // Different
				pattern4.Namespace = "staging"     // Different
				pattern4.ResourceType = "service"  // Different

				similarityWithoutContext := extractor.CalculateSimilarity(pattern3, pattern4)

				// Context similarity should boost the score
				Expect(similarityWithContext).To(BeNumerically(">", similarityWithoutContext))
			})
		})
	})

	Describe("GenerateEmbedding", func() {
		Context("when no embedding generator is provided", func() {
			It("should generate a simple embedding", func() {
				pattern := createTestPatternForExtractor()

				embedding, err := extractor.GenerateEmbedding(ctx, pattern)

				Expect(err).NotTo(HaveOccurred())
				Expect(embedding).NotTo(BeEmpty())
				Expect(len(embedding)).To(Equal(128)) // Default simple embedding size

				// Verify normalization (embedding should be unit vector)
				var norm float64
				for _, val := range embedding {
					norm += val * val
				}
				norm = math.Sqrt(norm)
				Expect(norm).To(BeNumerically("~", 1.0, 0.01))
			})
		})

		Context("when patterns with same characteristics", func() {
			It("should generate similar embeddings", func() {
				pattern1 := createTestPatternForExtractor()
				pattern1.ActionType = "scale_deployment"
				pattern1.AlertName = "HighMemoryUsage"

				pattern2 := createTestPatternForExtractor()
				pattern2.ActionType = "scale_deployment"
				pattern2.AlertName = "HighMemoryUsage"

				embedding1, err1 := extractor.GenerateEmbedding(ctx, pattern1)
				embedding2, err2 := extractor.GenerateEmbedding(ctx, pattern2)

				Expect(err1).NotTo(HaveOccurred())
				Expect(err2).NotTo(HaveOccurred())

				// Calculate cosine similarity
				var dotProduct, norm1, norm2 float64
				for i := 0; i < len(embedding1); i++ {
					dotProduct += embedding1[i] * embedding2[i]
					norm1 += embedding1[i] * embedding1[i]
					norm2 += embedding2[i] * embedding2[i]
				}
				similarity := dotProduct / (math.Sqrt(norm1) * math.Sqrt(norm2))

				Expect(similarity).To(BeNumerically(">", 0.9)) // Should be very similar
			})
		})
	})
})

// Helper functions

func createTestActionTrace() *actionhistory.ResourceActionTrace {
	now := time.Now()
	executionStart := now.Add(-2 * time.Minute)
	executionEnd := now.Add(-1 * time.Minute)

	effectivenessScore := 0.8
	effectivenessAssessedAt := now.Add(-30 * time.Minute)

	return &actionhistory.ResourceActionTrace{
		ID:                 1,
		ActionID:           "test-action-123",
		ActionType:         "scale_deployment",
		ActionTimestamp:    executionStart,
		ExecutionStartTime: &executionStart,
		ExecutionEndTime:   &executionEnd,

		AlertName:     "HighMemoryUsage",
		AlertSeverity: "warning",
		AlertLabels: actionhistory.JSONMap{
			"namespace":  "production",
			"deployment": "web-app",
			"app":        "web-application",
		},
		AlertAnnotations: actionhistory.JSONMap{
			"description": "Memory usage is high",
		},

		ModelUsed:       "granite3.1-dense:8b",
		ModelConfidence: 0.85,
		ModelReasoning:  stringPtr("Scaling deployment should reduce memory pressure"),

		ActionParameters: actionhistory.JSONMap{
			"deployment": "web-app",
			"replicas":   5,
			"reason":     "memory pressure",
		},

		ResourceStateBefore: actionhistory.JSONMap{
			"replicas": 3,
			"ready":    3,
		},
		ResourceStateAfter: actionhistory.JSONMap{
			"replicas": 5,
			"ready":    5,
		},

		ExecutionStatus: "completed",

		// Add effectiveness data so EffectivenessData gets created
		EffectivenessScore:      &effectivenessScore,
		EffectivenessAssessedAt: &effectivenessAssessedAt,

		CreatedAt: now.Add(-1 * time.Hour),
		UpdatedAt: now,
	}
}

func createTestPatternForExtractor() *vector.ActionPattern {
	return &vector.ActionPattern{
		ID:            "test-pattern-123",
		ActionType:    "scale_deployment",
		AlertName:     "HighMemoryUsage",
		AlertSeverity: "warning",
		Namespace:     "production",
		ResourceType:  "deployment",
		ResourceName:  "web-app",
		ActionParameters: map[string]interface{}{
			"replicas": 5,
			"reason":   "memory pressure",
		},
		ContextLabels: map[string]string{
			"app":     "web-application",
			"version": "1.0.0",
		},
		PreConditions: map[string]interface{}{
			"alert_name":     "HighMemoryUsage",
			"alert_severity": "warning",
		},
		PostConditions: map[string]interface{}{
			"execution_status": "completed",
		},
		EffectivenessData: &vector.EffectivenessData{
			Score:        0.8,
			SuccessCount: 1,
			FailureCount: 0,
		},
		Embedding: []float64{0.1, 0.2, 0.3, 0.4, 0.5},
		CreatedAt: time.Now().Add(-1 * time.Hour),
		UpdatedAt: time.Now(),
		Metadata: map[string]interface{}{
			"test": true,
		},
	}
}

func stringPtr(s string) *string {
	return &s
}
