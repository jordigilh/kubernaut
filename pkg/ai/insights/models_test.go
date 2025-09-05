package insights_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/insights"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/test/integration/shared/testenv"
)

var _ = Describe("Analytics Models", func() {
	var (
		ctx              context.Context
		logger           *logrus.Logger
		testEnv          *testenv.TestEnvironment
		vectorDB         *vector.MemoryVectorDatabase
		similarityModel  *insights.SimilarityBasedModel
		statisticalModel *insights.StatisticalModel
		testPatterns     []*vector.ActionPattern
	)

	BeforeEach(func() {
		var err error
		ctx = context.Background()

		// Setup logger
		logger = logrus.New()
		logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests

		// Setup fake K8s environment using envsetup infrastructure
		testEnv, err = testenv.SetupFakeEnvironment()
		Expect(err).NotTo(HaveOccurred())
		Expect(testEnv).NotTo(BeNil())

		// Create test namespace
		err = testEnv.CreateDefaultNamespace()
		Expect(err).NotTo(HaveOccurred())

		// Setup vector database
		vectorDB = vector.NewMemoryVectorDatabase(logger)

		// Create models
		similarityModel = insights.NewSimilarityBasedModel(vectorDB, logger)
		statisticalModel = insights.NewStatisticalModel(logger)

		// Create test patterns
		testPatterns = createTestPatterns()

		// Store patterns in vector database
		for _, pattern := range testPatterns {
			err := vectorDB.StoreActionPattern(ctx, pattern)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	AfterEach(func() {
		if testEnv != nil {
			err := testEnv.Cleanup()
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Describe("SimilarityBasedModel", func() {
		Describe("NewSimilarityBasedModel", func() {
			It("should create a new similarity-based model", func() {
				model := insights.NewSimilarityBasedModel(vectorDB, logger)

				Expect(model).NotTo(BeNil())
				Expect(model.GetModelInfo().Name).To(Equal("Similarity-Based Predictor"))
				Expect(model.GetModelInfo().Version).To(Equal("1.0.0"))
				Expect(model.GetModelInfo().Algorithm).To(Equal("Vector Similarity with Weighted Features"))
				Expect(model.IsReady()).To(BeTrue())
			})
		})

		Describe("Train", func() {
			Context("with sufficient training data", func() {
				It("should train the model successfully", func() {
					err := similarityModel.Train(ctx, testPatterns)

					Expect(err).NotTo(HaveOccurred())

					modelInfo := similarityModel.GetModelInfo()
					Expect(modelInfo.TrainingSize).To(Equal(len(testPatterns)))
					Expect(modelInfo.Accuracy).To(BeNumerically(">", 0))
					Expect(modelInfo.Accuracy).To(BeNumerically("<=", 1))
					Expect(modelInfo.TrainedAt).NotTo(BeZero())
				})
			})

			Context("with no training data", func() {
				It("should return an error", func() {
					err := similarityModel.Train(ctx, []*vector.ActionPattern{})

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("no training data available"))
				})
			})
		})

		Describe("Predict", func() {
			BeforeEach(func() {
				err := similarityModel.Train(ctx, testPatterns)
				Expect(err).NotTo(HaveOccurred())
			})

			Context("with a pattern similar to training data", func() {
				It("should make a prediction with reasonable confidence", func() {
					// Create a pattern similar to existing ones
					testPattern := &vector.ActionPattern{
						ID:               "test-prediction",
						ActionType:       "scale_deployment",
						AlertName:        "HighMemoryUsage",
						AlertSeverity:    "warning",
						Namespace:        "production",
						ResourceType:     "Deployment",
						ResourceName:     "app-server",
						ActionParameters: map[string]interface{}{"replicas": 5},
						ContextLabels:    map[string]string{"app": "server"},
						Embedding:        []float64{0.1, 0.2, 0.8, 0.5, 0.3, 0.7, 0.4, 0.6},
						CreatedAt:        time.Now(),
					}

					prediction, err := similarityModel.Predict(ctx, testPattern)

					Expect(err).NotTo(HaveOccurred())
					Expect(prediction).NotTo(BeNil())
					Expect(prediction.PredictedScore).To(BeNumerically(">=", 0))
					Expect(prediction.PredictedScore).To(BeNumerically("<=", 1))
					Expect(prediction.Confidence).To(BeNumerically(">", 0))
					Expect(prediction.Confidence).To(BeNumerically("<=", 1))
					Expect(prediction.ModelUsed).To(Equal("Similarity-Based Predictor"))
					Expect(prediction.FactorContributions).NotTo(BeNil())
					Expect(prediction.Recommendations).NotTo(BeEmpty())
				})
			})

			Context("with a pattern very different from training data", func() {
				It("should make a prediction with low confidence", func() {
					testPattern := &vector.ActionPattern{
						ID:               "unknown-pattern",
						ActionType:       "unknown_action",
						AlertName:        "UnknownAlert",
						AlertSeverity:    "info",
						Namespace:        "unknown",
						ResourceType:     "Unknown",
						ResourceName:     "unknown-resource",
						ActionParameters: map[string]interface{}{"unknown": "param"},
						ContextLabels:    map[string]string{"unknown": "label"},
						Embedding:        []float64{0.9, 0.1, 0.1, 0.9, 0.1, 0.9, 0.1, 0.9},
						CreatedAt:        time.Now(),
					}

					prediction, err := similarityModel.Predict(ctx, testPattern)

					Expect(err).NotTo(HaveOccurred())
					Expect(prediction).NotTo(BeNil())
					Expect(prediction.Confidence).To(BeNumerically("<=", 0.7)) // Lower confidence for unknown patterns
				})
			})

			Context("with no similar patterns", func() {
				It("should return default prediction", func() {
					// Create empty vector DB to simulate no similar patterns
					emptyVectorDB := vector.NewMemoryVectorDatabase(logger)
					emptyModel := insights.NewSimilarityBasedModel(emptyVectorDB, logger)

					testPattern := &vector.ActionPattern{
						ID:         "isolated-pattern",
						ActionType: "test_action",
						AlertName:  "TestAlert",
						Embedding:  []float64{0.5, 0.5, 0.5, 0.5},
						CreatedAt:  time.Now(),
					}

					prediction, err := emptyModel.Predict(ctx, testPattern)

					Expect(err).NotTo(HaveOccurred())
					Expect(prediction.PredictedScore).To(Equal(0.5)) // Default conservative score
					Expect(prediction.Confidence).To(Equal(0.1))     // Low confidence
					Expect(prediction.RiskFactors).To(ContainElement("No historical data available"))
					Expect(prediction.Recommendations).To(ContainElement("Monitor closely as this is a new pattern"))
				})
			})
		})

		Describe("cosine similarity calculation", func() {
			It("should calculate similarity correctly", func() {
				// Test with known vectors
				vector1 := []float64{1, 0, 0}
				vector2 := []float64{1, 0, 0}
				vector3 := []float64{0, 1, 0}

				// Create a test pattern to access the method
				testPattern1 := &vector.ActionPattern{
					ID:        "test1",
					Embedding: vector1,
				}
				testPattern2 := &vector.ActionPattern{
					ID:        "test2",
					Embedding: vector2,
				}
				testPattern3 := &vector.ActionPattern{
					ID:        "test3",
					Embedding: vector3,
				}

				// Train model first to access the helper methods
				err := similarityModel.Train(ctx, []*vector.ActionPattern{testPattern1, testPattern2, testPattern3})
				Expect(err).NotTo(HaveOccurred())

				// Since the helper methods are private, we test through the public interface
				// by checking that similar vectors produce high similarity predictions
				prediction1, err := similarityModel.Predict(ctx, testPattern1)
				Expect(err).NotTo(HaveOccurred())

				prediction2, err := similarityModel.Predict(ctx, testPattern3)
				Expect(err).NotTo(HaveOccurred())

				// Identical vectors should have higher confidence than orthogonal ones
				Expect(prediction1.Confidence).To(BeNumerically(">=", prediction2.Confidence))
			})
		})
	})

	Describe("StatisticalModel", func() {
		Describe("NewStatisticalModel", func() {
			It("should create a new statistical model", func() {
				model := insights.NewStatisticalModel(logger)

				Expect(model).NotTo(BeNil())
				Expect(model.GetModelInfo().Name).To(Equal("Statistical Predictor"))
				Expect(model.GetModelInfo().Version).To(Equal("1.0.0"))
				Expect(model.GetModelInfo().Algorithm).To(Equal("Bayesian Statistical Analysis"))
				Expect(model.IsReady()).To(BeFalse()) // Not ready until trained
			})
		})

		Describe("Train", func() {
			Context("with sufficient training data", func() {
				It("should train the model successfully", func() {
					err := statisticalModel.Train(ctx, testPatterns)

					Expect(err).NotTo(HaveOccurred())
					Expect(statisticalModel.IsReady()).To(BeTrue())

					modelInfo := statisticalModel.GetModelInfo()
					Expect(modelInfo.TrainingSize).To(Equal(len(testPatterns)))
					Expect(modelInfo.Accuracy).To(BeNumerically(">", 0))
					Expect(modelInfo.Accuracy).To(BeNumerically("<=", 1))
					Expect(modelInfo.TrainedAt).NotTo(BeZero())
				})
			})

			Context("with no training data", func() {
				It("should return an error", func() {
					err := statisticalModel.Train(ctx, []*vector.ActionPattern{})

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("no training data available"))
				})
			})
		})

		Describe("Predict", func() {
			BeforeEach(func() {
				err := statisticalModel.Train(ctx, testPatterns)
				Expect(err).NotTo(HaveOccurred())
			})

			Context("with a known action type", func() {
				It("should make predictions based on statistical data", func() {
					testPattern := &vector.ActionPattern{
						ID:            "stat-test-1",
						ActionType:    "scale_deployment", // This exists in test data
						AlertSeverity: "warning",
						Namespace:     "production",
						CreatedAt:     time.Now(),
					}

					prediction, err := statisticalModel.Predict(ctx, testPattern)

					Expect(err).NotTo(HaveOccurred())
					Expect(prediction).NotTo(BeNil())
					Expect(prediction.PredictedScore).To(BeNumerically(">=", 0))
					Expect(prediction.PredictedScore).To(BeNumerically("<=", 1))
					Expect(prediction.Confidence).To(BeNumerically(">", 0))
					Expect(prediction.ModelUsed).To(Equal("Statistical Predictor"))
					Expect(prediction.FactorContributions).To(HaveKey("severity_adjustment"))
					Expect(prediction.FactorContributions).To(HaveKey("namespace_adjustment"))
				})
			})

			Context("with an unknown action type", func() {
				It("should fall back to global statistics", func() {
					testPattern := &vector.ActionPattern{
						ID:            "stat-test-unknown",
						ActionType:    "unknown_action_type",
						AlertSeverity: "info",
						Namespace:     "development",
						CreatedAt:     time.Now(),
					}

					prediction, err := statisticalModel.Predict(ctx, testPattern)

					Expect(err).NotTo(HaveOccurred())
					Expect(prediction).NotTo(BeNil())
					Expect(prediction.PredictedScore).To(BeNumerically(">=", 0))
					Expect(prediction.PredictedScore).To(BeNumerically("<=", 1))
					Expect(prediction.RiskFactors).To(ContainElement("Limited historical data for this action type"))
				})
			})

			Context("when model is not trained", func() {
				It("should return an error", func() {
					untrainedModel := insights.NewStatisticalModel(logger)

					testPattern := &vector.ActionPattern{
						ID:         "untrained-test",
						ActionType: "test_action",
					}

					_, err := untrainedModel.Predict(ctx, testPattern)

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("model has not been trained yet"))
				})
			})
		})

		Describe("contextual adjustments", func() {
			BeforeEach(func() {
				err := statisticalModel.Train(ctx, testPatterns)
				Expect(err).NotTo(HaveOccurred())
			})

			Context("with critical severity", func() {
				It("should apply negative severity adjustment", func() {
					testPattern := &vector.ActionPattern{
						ID:            "critical-test",
						ActionType:    "scale_deployment",
						AlertSeverity: "critical",
						Namespace:     "default",
						CreatedAt:     time.Now(),
					}

					prediction, err := statisticalModel.Predict(ctx, testPattern)

					Expect(err).NotTo(HaveOccurred())
					Expect(prediction.FactorContributions["severity_adjustment"]).To(BeNumerically("<", 0))
				})
			})

			Context("with production namespace", func() {
				It("should apply negative namespace adjustment", func() {
					testPattern := &vector.ActionPattern{
						ID:            "production-test",
						ActionType:    "scale_deployment",
						AlertSeverity: "warning",
						Namespace:     "production",
						CreatedAt:     time.Now(),
					}

					prediction, err := statisticalModel.Predict(ctx, testPattern)

					Expect(err).NotTo(HaveOccurred())
					Expect(prediction.FactorContributions["namespace_adjustment"]).To(BeNumerically("<", 0))
				})
			})

			Context("with weekend timing", func() {
				It("should apply negative time adjustment", func() {
					// Create a Saturday timestamp
					saturday := time.Date(2024, 1, 6, 10, 0, 0, 0, time.UTC) // Saturday

					testPattern := &vector.ActionPattern{
						ID:            "weekend-test",
						ActionType:    "scale_deployment",
						AlertSeverity: "warning",
						Namespace:     "default",
						CreatedAt:     saturday,
					}

					prediction, err := statisticalModel.Predict(ctx, testPattern)

					Expect(err).NotTo(HaveOccurred())
					Expect(prediction.FactorContributions["time_adjustment"]).To(BeNumerically("<", 0))
				})
			})
		})
	})

	Describe("Model Integration with Fake K8s", func() {
		Context("when using test environment", func() {
			It("should have access to fake Kubernetes client", func() {
				k8sClient := testEnv.CreateK8sClient(logger)
				Expect(k8sClient).NotTo(BeNil())

				// Test that we can interact with the fake cluster
				nodes, err := k8sClient.ListNodes(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(nodes).NotTo(BeNil())
			})

			It("should be able to interact with resources in fake cluster", func() {
				k8sClient := testEnv.CreateK8sClient(logger)

				// Test basic pod operations
				pods, err := k8sClient.ListPodsWithLabel(ctx, "default", "app=test")
				Expect(err).NotTo(HaveOccurred())
				Expect(pods).NotTo(BeNil())

				// Test resource quota operations
				quotas, err := k8sClient.GetResourceQuotas(ctx, "default")
				Expect(err).NotTo(HaveOccurred())
				Expect(quotas).NotTo(BeNil())

				// This simulates a realistic scenario where the analytics models
				// would be predicting effectiveness of actions on real K8s resources
			})
		})
	})
})

// Helper functions

func createTestPatterns() []*vector.ActionPattern {
	baseTime := time.Now().Add(-24 * time.Hour)

	return []*vector.ActionPattern{
		{
			ID:               "pattern-1",
			ActionType:       "scale_deployment",
			AlertName:        "HighMemoryUsage",
			AlertSeverity:    "warning",
			Namespace:        "production",
			ResourceType:     "Deployment",
			ResourceName:     "web-server",
			ActionParameters: map[string]interface{}{"replicas": 5},
			ContextLabels:    map[string]string{"app": "web", "tier": "frontend"},
			Embedding:        []float64{0.1, 0.2, 0.8, 0.5, 0.3, 0.7, 0.4, 0.6},
			CreatedAt:        baseTime,
			EffectivenessData: &vector.EffectivenessData{
				Score:                0.85,
				SuccessCount:         8,
				FailureCount:         2,
				AverageExecutionTime: 5 * time.Minute,
				SideEffectsCount:     0,
				RecurrenceRate:       0.1,
				ContextualFactors:    map[string]float64{"load": 0.7, "time_of_day": 0.5},
				LastAssessed:         baseTime.Add(10 * time.Minute),
			},
		},
		{
			ID:               "pattern-2",
			ActionType:       "restart_pod",
			AlertName:        "PodCrashing",
			AlertSeverity:    "critical",
			Namespace:        "production",
			ResourceType:     "Pod",
			ResourceName:     "api-server-123",
			ActionParameters: map[string]interface{}{"force": true},
			ContextLabels:    map[string]string{"app": "api", "tier": "backend"},
			Embedding:        []float64{0.3, 0.7, 0.2, 0.8, 0.4, 0.6, 0.5, 0.1},
			CreatedAt:        baseTime.Add(time.Hour),
			EffectivenessData: &vector.EffectivenessData{
				Score:                0.65,
				SuccessCount:         6,
				FailureCount:         4,
				AverageExecutionTime: 2 * time.Minute,
				SideEffectsCount:     2,
				RecurrenceRate:       0.3,
				ContextualFactors:    map[string]float64{"severity": 0.9, "namespace": 0.8},
				LastAssessed:         baseTime.Add(time.Hour + 5*time.Minute),
			},
		},
		{
			ID:               "pattern-3",
			ActionType:       "scale_deployment",
			AlertName:        "HighCPUUsage",
			AlertSeverity:    "warning",
			Namespace:        "staging",
			ResourceType:     "Deployment",
			ResourceName:     "worker",
			ActionParameters: map[string]interface{}{"replicas": 3},
			ContextLabels:    map[string]string{"app": "worker", "tier": "backend"},
			Embedding:        []float64{0.2, 0.3, 0.7, 0.6, 0.4, 0.8, 0.3, 0.5},
			CreatedAt:        baseTime.Add(2 * time.Hour),
			EffectivenessData: &vector.EffectivenessData{
				Score:                0.92,
				SuccessCount:         9,
				FailureCount:         1,
				AverageExecutionTime: 3 * time.Minute,
				SideEffectsCount:     0,
				RecurrenceRate:       0.05,
				ContextualFactors:    map[string]float64{"load": 0.6, "environment": 0.7},
				LastAssessed:         baseTime.Add(2*time.Hour + 8*time.Minute),
			},
		},
		{
			ID:               "pattern-4",
			ActionType:       "increase_resources",
			AlertName:        "ResourceExhaustion",
			AlertSeverity:    "critical",
			Namespace:        "production",
			ResourceType:     "Deployment",
			ResourceName:     "database",
			ActionParameters: map[string]interface{}{"cpu": "2000m", "memory": "4Gi"},
			ContextLabels:    map[string]string{"app": "db", "tier": "data"},
			Embedding:        []float64{0.6, 0.1, 0.4, 0.9, 0.2, 0.3, 0.8, 0.7},
			CreatedAt:        baseTime.Add(3 * time.Hour),
			EffectivenessData: &vector.EffectivenessData{
				Score:                0.78,
				SuccessCount:         7,
				FailureCount:         3,
				AverageExecutionTime: 8 * time.Minute,
				SideEffectsCount:     0,
				RecurrenceRate:       0.2,
				ContextualFactors:    map[string]float64{"resource_pressure": 0.9, "criticality": 0.9},
				LastAssessed:         baseTime.Add(3*time.Hour + 12*time.Minute),
			},
		},
		{
			ID:               "pattern-5",
			ActionType:       "restart_pod",
			AlertName:        "DeadlockDetected",
			AlertSeverity:    "warning",
			Namespace:        "development",
			ResourceType:     "Pod",
			ResourceName:     "test-pod-456",
			ActionParameters: map[string]interface{}{"gracePeriod": 30},
			ContextLabels:    map[string]string{"app": "test", "env": "dev"},
			Embedding:        []float64{0.4, 0.6, 0.3, 0.2, 0.9, 0.1, 0.7, 0.8},
			CreatedAt:        baseTime.Add(4 * time.Hour),
			EffectivenessData: &vector.EffectivenessData{
				Score:                0.45,
				SuccessCount:         4,
				FailureCount:         6,
				AverageExecutionTime: 15 * time.Minute,
				SideEffectsCount:     3,
				RecurrenceRate:       0.6,
				ContextualFactors:    map[string]float64{"complexity": 0.8, "environment": 0.3},
				LastAssessed:         baseTime.Add(4*time.Hour + 20*time.Minute),
			},
		},
		{
			ID:               "pattern-6",
			ActionType:       "scale_deployment",
			AlertName:        "LoadSpike",
			AlertSeverity:    "info",
			Namespace:        "production",
			ResourceType:     "Deployment",
			ResourceName:     "frontend",
			ActionParameters: map[string]interface{}{"replicas": 8},
			ContextLabels:    map[string]string{"app": "frontend", "tier": "web"},
			Embedding:        []float64{0.1, 0.8, 0.6, 0.3, 0.5, 0.9, 0.2, 0.4},
			CreatedAt:        baseTime.Add(5 * time.Hour),
			EffectivenessData: &vector.EffectivenessData{
				Score:                0.95,
				SuccessCount:         10,
				FailureCount:         0,
				AverageExecutionTime: 1 * time.Minute,
				SideEffectsCount:     0,
				RecurrenceRate:       0.0,
				ContextualFactors:    map[string]float64{"load_pattern": 0.8, "predictability": 0.9},
				LastAssessed:         baseTime.Add(5*time.Hour + 3*time.Minute),
			},
		},
	}
}
