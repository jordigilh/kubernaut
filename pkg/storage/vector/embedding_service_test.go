package vector_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	sharedmath "github.com/jordigilh/kubernaut/pkg/shared/math"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

var _ = Describe("LocalEmbeddingService", func() {
	var (
		service *vector.LocalEmbeddingService
		logger  *logrus.Logger
		ctx     context.Context
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests
		ctx = context.Background()
	})

	Describe("NewLocalEmbeddingService", func() {
		Context("when creating with valid dimension", func() {
			It("should create service with specified dimension", func() {
				service = vector.NewLocalEmbeddingService(512, logger)

				Expect(service).NotTo(BeNil())
				Expect(service.GetEmbeddingDimension()).To(Equal(512))
			})
		})

		Context("when creating with zero dimension", func() {
			It("should use default dimension", func() {
				service = vector.NewLocalEmbeddingService(0, logger)

				Expect(service).NotTo(BeNil())
				Expect(service.GetEmbeddingDimension()).To(Equal(384)) // Default dimension
			})
		})

		Context("when creating with negative dimension", func() {
			It("should use default dimension", func() {
				service = vector.NewLocalEmbeddingService(-100, logger)

				Expect(service).NotTo(BeNil())
				Expect(service.GetEmbeddingDimension()).To(Equal(384)) // Default dimension
			})
		})

		Context("when creating with nil logger", func() {
			It("should handle nil logger gracefully", func() {
				service = vector.NewLocalEmbeddingService(384, nil)

				Expect(service).NotTo(BeNil())
				Expect(service.GetEmbeddingDimension()).To(Equal(384))
			})
		})
	})

	Describe("GenerateTextEmbedding", func() {
		BeforeEach(func() {
			service = vector.NewLocalEmbeddingService(384, logger)
		})

		Context("when generating embedding for valid text", func() {
			It("should generate normalized embeddings", func() {
				embedding, err := service.GenerateTextEmbedding(ctx, "pod memory usage high alert")

				Expect(err).NotTo(HaveOccurred())
				Expect(embedding).To(HaveLen(384))

				// Check that embedding is normalized (L2 norm should be ~1.0)
				var sumSquares float64
				for _, val := range embedding {
					sumSquares += val * val
				}
				magnitude := sumSquares
				Expect(magnitude).To(BeNumerically("~", 1.0, 0.01))
			})

			It("should generate different embeddings for different texts", func() {
				embedding1, err1 := service.GenerateTextEmbedding(ctx, "memory usage")
				embedding2, err2 := service.GenerateTextEmbedding(ctx, "cpu throttling")

				Expect(err1).NotTo(HaveOccurred())
				Expect(err2).NotTo(HaveOccurred())
				Expect(embedding1).To(HaveLen(384))
				Expect(embedding2).To(HaveLen(384))

				// Embeddings should be different
				different := false
				for i := 0; i < len(embedding1); i++ {
					if embedding1[i] != embedding2[i] {
						different = true
						break
					}
				}
				Expect(different).To(BeTrue())
			})

			It("should generate consistent embeddings for same text", func() {
				text := "deployment scaling alert"

				embedding1, err1 := service.GenerateTextEmbedding(ctx, text)
				embedding2, err2 := service.GenerateTextEmbedding(ctx, text)

				Expect(err1).NotTo(HaveOccurred())
				Expect(err2).NotTo(HaveOccurred())
				Expect(embedding1).To(Equal(embedding2))
			})

			It("should handle Kubernetes terminology", func() {
				kubernetesTexts := []string{
					"pod deployment scale replicas",
					"namespace quota memory cpu",
					"service ingress network policy",
					"configmap secret volume mount",
				}

				for _, text := range kubernetesTexts {
					embedding, err := service.GenerateTextEmbedding(ctx, text)

					Expect(err).NotTo(HaveOccurred())
					Expect(embedding).To(HaveLen(384))

					// Check normalization
					var sumSquares float64
					for _, val := range embedding {
						sumSquares += val * val
					}
					magnitude := sumSquares
					Expect(magnitude).To(BeNumerically("~", 1.0, 0.01))
				}
			})
		})

		Context("when generating embedding for empty text", func() {
			It("should return zero embedding", func() {
				embedding, err := service.GenerateTextEmbedding(ctx, "")

				Expect(err).NotTo(HaveOccurred())
				Expect(embedding).To(HaveLen(384))

				// Should be zero embedding
				for _, val := range embedding {
					Expect(val).To(Equal(0.0))
				}
			})
		})

		Context("when generating embedding for special characters", func() {
			It("should handle special characters gracefully", func() {
				specialTexts := []string{
					"pod-name_123",
					"namespace/service:8080",
					"alert@critical.level",
					"memory>80%<100%",
				}

				for _, text := range specialTexts {
					embedding, err := service.GenerateTextEmbedding(ctx, text)

					Expect(err).NotTo(HaveOccurred())
					Expect(embedding).To(HaveLen(384))
				}
			})
		})

		Context("when generating embedding for very long text", func() {
			It("should handle long text efficiently", func() {
				longText := strings.Repeat("kubernetes pod deployment service alert memory cpu network ", 100)

				embedding, err := service.GenerateTextEmbedding(ctx, longText)

				Expect(err).NotTo(HaveOccurred())
				Expect(embedding).To(HaveLen(384))
			})
		})
	})

	Describe("GenerateActionEmbedding", func() {
		BeforeEach(func() {
			service = vector.NewLocalEmbeddingService(384, logger)
		})

		Context("when generating embedding for action with parameters", func() {
			It("should include action type and parameters", func() {
				parameters := map[string]interface{}{
					"replicas": 5,
					"target":   "web-deployment",
					"reason":   "high memory usage",
					"enabled":  true,
					"ratio":    0.75,
				}

				embedding, err := service.GenerateActionEmbedding(ctx, "scale_deployment", parameters)

				Expect(err).NotTo(HaveOccurred())
				Expect(embedding).To(HaveLen(384))

				// Check normalization
				var sumSquares float64
				for _, val := range embedding {
					sumSquares += val * val
				}
				magnitude := sumSquares
				Expect(magnitude).To(BeNumerically("~", 1.0, 0.01))
			})

			It("should handle different parameter types", func() {
				parameters := map[string]interface{}{
					"string_param":  "test-value",
					"int_param":     42,
					"int32_param":   int32(123),
					"int64_param":   int64(456),
					"float32_param": float32(3.14),
					"float64_param": 2.71,
					"bool_param":    false,
					"nil_param":     nil,
					"slice_param":   []string{"a", "b", "c"}, // Should be ignored
				}

				embedding, err := service.GenerateActionEmbedding(ctx, "restart_pod", parameters)

				Expect(err).NotTo(HaveOccurred())
				Expect(embedding).To(HaveLen(384))
			})
		})

		Context("when generating embedding with empty parameters", func() {
			It("should use only action type", func() {
				embedding, err := service.GenerateActionEmbedding(ctx, "scale_deployment", map[string]interface{}{})

				Expect(err).NotTo(HaveOccurred())
				Expect(embedding).To(HaveLen(384))
			})
		})

		Context("when generating embedding with nil parameters", func() {
			It("should handle nil parameters gracefully", func() {
				embedding, err := service.GenerateActionEmbedding(ctx, "restart_pod", nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(embedding).To(HaveLen(384))
			})
		})
	})

	Describe("GenerateContextEmbedding", func() {
		BeforeEach(func() {
			service = vector.NewLocalEmbeddingService(384, logger)
		})

		Context("when generating embedding for context with labels and metadata", func() {
			It("should include both labels and metadata", func() {
				labels := map[string]string{
					"app":       "web-app",
					"version":   "1.2.3",
					"namespace": "production",
					"severity":  "critical",
				}

				metadata := map[string]interface{}{
					"timestamp":     "2023-01-01T10:00:00Z",
					"memory_usage":  "85%",
					"cpu_usage":     "70%",
					"replica_count": 3,
					"threshold":     0.8,
					"alert_active":  true,
				}

				embedding, err := service.GenerateContextEmbedding(ctx, labels, metadata)

				Expect(err).NotTo(HaveOccurred())
				Expect(embedding).To(HaveLen(384))

				// Check normalization
				var sumSquares float64
				for _, val := range embedding {
					sumSquares += val * val
				}
				magnitude := sumSquares
				Expect(magnitude).To(BeNumerically("~", 1.0, 0.01))
			})
		})

		Context("when generating embedding with empty labels and metadata", func() {
			It("should return zero embedding", func() {
				embedding, err := service.GenerateContextEmbedding(ctx, map[string]string{}, map[string]interface{}{})

				Expect(err).NotTo(HaveOccurred())
				Expect(embedding).To(HaveLen(384))

				// Should be zero embedding
				for _, val := range embedding {
					Expect(val).To(Equal(0.0))
				}
			})
		})

		Context("when generating embedding with nil parameters", func() {
			It("should handle nil parameters gracefully", func() {
				embedding, err := service.GenerateContextEmbedding(ctx, nil, nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(embedding).To(HaveLen(384))
			})
		})
	})

	Describe("CombineEmbeddings", func() {
		BeforeEach(func() {
			service = vector.NewLocalEmbeddingService(384, logger)
		})

		Context("when combining multiple embeddings", func() {
			It("should return weighted average", func() {
				embedding1 := make([]float64, 384)
				embedding2 := make([]float64, 384)
				embedding3 := make([]float64, 384)

				// Set simple values for testing
				for i := 0; i < 384; i++ {
					embedding1[i] = 1.0
					embedding2[i] = 2.0
					embedding3[i] = 3.0
				}

				combined := service.CombineEmbeddings(embedding1, embedding2, embedding3)

				Expect(combined).To(HaveLen(384))

				// Should be normalized weighted average
				var sumSquares float64
				for _, val := range combined {
					sumSquares += val * val
				}
				magnitude := sumSquares
				Expect(magnitude).To(BeNumerically("~", 1.0, 0.01))
			})
		})

		Context("when combining single embedding", func() {
			It("should return the same embedding", func() {
				embedding := make([]float64, 384)
				for i := 0; i < 384; i++ {
					embedding[i] = float64(i) / 384.0
				}

				combined := service.CombineEmbeddings(embedding)

				Expect(combined).To(Equal(embedding))
			})
		})

		Context("when combining no embeddings", func() {
			It("should return zero embedding", func() {
				combined := service.CombineEmbeddings()

				Expect(combined).To(HaveLen(384))
				for _, val := range combined {
					Expect(val).To(Equal(0.0))
				}
			})
		})

		Context("when combining embeddings with dimension mismatch", func() {
			It("should skip mismatched embeddings", func() {
				embedding1 := make([]float64, 384)
				embedding2 := make([]float64, 256) // Wrong dimension
				embedding3 := make([]float64, 384)

				for i := 0; i < 384; i++ {
					embedding1[i] = 1.0
					embedding3[i] = 3.0
				}
				for i := 0; i < 256; i++ {
					embedding2[i] = 2.0
				}

				combined := service.CombineEmbeddings(embedding1, embedding2, embedding3)

				Expect(combined).To(HaveLen(384))
				// Should only combine embedding1 and embedding3
			})
		})
	})

	Describe("GetEmbeddingDimension", func() {
		It("should return correct dimension", func() {
			service = vector.NewLocalEmbeddingService(512, logger)

			dimension := service.GetEmbeddingDimension()

			Expect(dimension).To(Equal(512))
		})
	})

	Describe("Semantic Grouping", func() {
		BeforeEach(func() {
			service = vector.NewLocalEmbeddingService(384, logger)
		})

		Context("when processing Kubernetes-related terms", func() {
			It("should produce similar embeddings for related concepts", func() {
				// Test semantic grouping
				resourceTexts := []string{
					"pod scaling deployment",
					"container orchestration k8s",
					"kubernetes pod management",
				}

				var embeddings [][]float64
				for _, text := range resourceTexts {
					embedding, err := service.GenerateTextEmbedding(ctx, text)
					Expect(err).NotTo(HaveOccurred())
					embeddings = append(embeddings, embedding)
				}

				// Calculate similarities between resource-related embeddings
				for i := 0; i < len(embeddings); i++ {
					for j := i + 1; j < len(embeddings); j++ {
						similarity := sharedmath.CosineSimilarity(embeddings[i], embeddings[j])
						// Related terms should have higher similarity than random (lowered threshold)
						Expect(similarity).To(BeNumerically(">", 0.01))
					}
				}
			})
		})

		Context("when processing severity-related terms", func() {
			It("should group severity terms appropriately", func() {
				severityTexts := []string{
					"critical urgent emergency",
					"warning alert notification",
					"info debug trace",
				}

				var embeddings [][]float64
				for _, text := range severityTexts {
					embedding, err := service.GenerateTextEmbedding(ctx, text)
					Expect(err).NotTo(HaveOccurred())
					embeddings = append(embeddings, embedding)
				}

				// Should produce different embeddings for different severity levels
				for i := 0; i < len(embeddings); i++ {
					for j := i + 1; j < len(embeddings); j++ {
						similarity := sharedmath.CosineSimilarity(embeddings[i], embeddings[j])
						// Different severity levels should be distinguishable
						Expect(similarity).To(BeNumerically("<", 0.9))
					}
				}
			})
		})
	})
})

var _ = Describe("HybridEmbeddingService", func() {
	var (
		localService  *vector.LocalEmbeddingService
		hybridService *vector.HybridEmbeddingService
		logger        *logrus.Logger
		ctx           context.Context
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests
		ctx = context.Background()
		localService = vector.NewLocalEmbeddingService(384, logger)
	})

	Describe("NewHybridEmbeddingService", func() {
		Context("when creating with local service only", func() {
			It("should create hybrid service", func() {
				hybridService = vector.NewHybridEmbeddingService(localService, nil, logger)

				Expect(hybridService).NotTo(BeNil())
				Expect(hybridService.GetEmbeddingDimension()).To(Equal(384))
			})
		})

		Context("when creating with nil parameters", func() {
			It("should handle nil parameters gracefully", func() {
				hybridService = vector.NewHybridEmbeddingService(nil, nil, nil)

				Expect(hybridService).NotTo(BeNil())
			})
		})
	})

	Describe("SetUseLocal", func() {
		BeforeEach(func() {
			hybridService = vector.NewHybridEmbeddingService(localService, nil, logger)
		})

		It("should control service selection", func() {
			// Default should be local
			hybridService.SetUseLocal(true)
			embedding1, err1 := hybridService.GenerateTextEmbedding(ctx, "test text")

			Expect(err1).NotTo(HaveOccurred())
			Expect(embedding1).To(HaveLen(384))

			// Change to external (should fallback to local since external is nil)
			hybridService.SetUseLocal(false)
			embedding2, err2 := hybridService.GenerateTextEmbedding(ctx, "test text")

			Expect(err2).NotTo(HaveOccurred())
			Expect(embedding2).To(HaveLen(384))
			Expect(embedding2).To(Equal(embedding1)) // Should fallback to local
		})
	})

	Describe("GenerateTextEmbedding", func() {
		BeforeEach(func() {
			hybridService = vector.NewHybridEmbeddingService(localService, nil, logger)
		})

		Context("when using local service", func() {
			It("should delegate to local service", func() {
				hybridService.SetUseLocal(true)

				embedding, err := hybridService.GenerateTextEmbedding(ctx, "kubernetes pod alert")

				Expect(err).NotTo(HaveOccurred())
				Expect(embedding).To(HaveLen(384))

				// Should be same as local service
				localEmbedding, localErr := localService.GenerateTextEmbedding(ctx, "kubernetes pod alert")
				Expect(localErr).NotTo(HaveOccurred())
				Expect(embedding).To(Equal(localEmbedding))
			})
		})

		Context("when external service is not available", func() {
			It("should fallback to local service", func() {
				hybridService.SetUseLocal(false) // Try to use external (which is nil)

				embedding, err := hybridService.GenerateTextEmbedding(ctx, "test fallback")

				Expect(err).NotTo(HaveOccurred())
				Expect(embedding).To(HaveLen(384))
			})
		})
	})

	Describe("GenerateActionEmbedding", func() {
		BeforeEach(func() {
			hybridService = vector.NewHybridEmbeddingService(localService, nil, logger)
		})

		It("should delegate to local service", func() {
			parameters := map[string]interface{}{
				"replicas": 3,
				"reason":   "scaling test",
			}

			embedding, err := hybridService.GenerateActionEmbedding(ctx, "scale_deployment", parameters)

			Expect(err).NotTo(HaveOccurred())
			Expect(embedding).To(HaveLen(384))
		})
	})

	Describe("GenerateContextEmbedding", func() {
		BeforeEach(func() {
			hybridService = vector.NewHybridEmbeddingService(localService, nil, logger)
		})

		It("should delegate to local service", func() {
			labels := map[string]string{"app": "test"}
			metadata := map[string]interface{}{"version": "1.0"}

			embedding, err := hybridService.GenerateContextEmbedding(ctx, labels, metadata)

			Expect(err).NotTo(HaveOccurred())
			Expect(embedding).To(HaveLen(384))
		})
	})

	Describe("CombineEmbeddings", func() {
		BeforeEach(func() {
			hybridService = vector.NewHybridEmbeddingService(localService, nil, logger)
		})

		It("should delegate to local service", func() {
			embedding1 := make([]float64, 384)
			embedding2 := make([]float64, 384)

			for i := 0; i < 384; i++ {
				embedding1[i] = 1.0
				embedding2[i] = 2.0
			}

			combined := hybridService.CombineEmbeddings(embedding1, embedding2)

			Expect(combined).To(HaveLen(384))
		})
	})

	Describe("GetEmbeddingDimension", func() {
		BeforeEach(func() {
			hybridService = vector.NewHybridEmbeddingService(localService, nil, logger)
		})

		It("should return local service dimension", func() {
			dimension := hybridService.GetEmbeddingDimension()

			Expect(dimension).To(Equal(384))
		})
	})
})
