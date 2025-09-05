package vector_test

import (
	"database/sql"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

var _ = Describe("VectorDatabaseFactory", func() {
	var (
		factory *vector.VectorDatabaseFactory
		logger  *logrus.Logger
		db      *sql.DB
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests
		db = nil                           // Use nil for most tests
	})

	Describe("NewVectorDatabaseFactory", func() {
		It("should create a new factory instance", func() {
			config := &config.VectorDBConfig{
				Enabled: true,
				Backend: "memory",
			}

			factory = vector.NewVectorDatabaseFactory(config, db, logger)

			Expect(factory).NotTo(BeNil())
		})

		It("should handle nil parameters gracefully", func() {
			factory = vector.NewVectorDatabaseFactory(nil, nil, nil)

			Expect(factory).NotTo(BeNil())
		})
	})

	Describe("CreateVectorDatabase", func() {
		Context("when vector database is disabled", func() {
			BeforeEach(func() {
				config := &config.VectorDBConfig{
					Enabled: false,
				}
				factory = vector.NewVectorDatabaseFactory(config, db, logger)
			})

			It("should return memory database fallback", func() {
				vectorDB, err := factory.CreateVectorDatabase()

				Expect(err).NotTo(HaveOccurred())
				Expect(vectorDB).NotTo(BeNil())
			})
		})

		Context("when backend is memory", func() {
			BeforeEach(func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "memory",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "local",
						Dimension: 384,
					},
				}
				factory = vector.NewVectorDatabaseFactory(config, db, logger)
			})

			It("should create memory vector database", func() {
				vectorDB, err := factory.CreateVectorDatabase()

				Expect(err).NotTo(HaveOccurred())
				Expect(vectorDB).NotTo(BeNil())
			})
		})

		Context("when backend is postgresql", func() {
			BeforeEach(func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "postgresql",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "local",
						Dimension: 384,
					},
					PostgreSQL: config.PostgreSQLVectorConfig{
						UseMainDB:  true,
						IndexLists: 100,
					},
				}
				factory = vector.NewVectorDatabaseFactory(config, db, logger)
			})

			It("should require database connection", func() {
				vectorDB, err := factory.CreateVectorDatabase()

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("main database connection not available"))
				Expect(vectorDB).To(BeNil())
			})
		})

		Context("when backend is unsupported", func() {
			BeforeEach(func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "unsupported",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "local",
						Dimension: 384,
					},
				}
				factory = vector.NewVectorDatabaseFactory(config, db, logger)
			})

			It("should return an error", func() {
				vectorDB, err := factory.CreateVectorDatabase()

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unsupported vector database backend"))
				Expect(vectorDB).To(BeNil())
			})
		})

		Context("when pinecone backend is configured", func() {
			BeforeEach(func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "pinecone",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "local",
						Dimension: 384,
					},
				}
				factory = vector.NewVectorDatabaseFactory(config, db, logger)
			})

			It("should return not implemented error", func() {
				vectorDB, err := factory.CreateVectorDatabase()

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Pinecone vector database not implemented yet"))
				Expect(vectorDB).To(BeNil())
			})
		})

		Context("when weaviate backend is configured", func() {
			BeforeEach(func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "weaviate",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "local",
						Dimension: 384,
					},
				}
				factory = vector.NewVectorDatabaseFactory(config, db, logger)
			})

			It("should return not implemented error", func() {
				vectorDB, err := factory.CreateVectorDatabase()

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Weaviate vector database not implemented yet"))
				Expect(vectorDB).To(BeNil())
			})
		})
	})

	Describe("CreateEmbeddingService", func() {
		Context("when embedding service is local", func() {
			BeforeEach(func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "memory",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "local",
						Dimension: 384,
						Model:     "all-MiniLM-L6-v2",
					},
				}
				factory = vector.NewVectorDatabaseFactory(config, db, logger)
			})

			It("should create local embedding service", func() {
				embeddingService, err := factory.CreateEmbeddingService()

				Expect(err).NotTo(HaveOccurred())
				Expect(embeddingService).NotTo(BeNil())
				Expect(embeddingService.GetEmbeddingDimension()).To(Equal(384))
			})
		})

		Context("when embedding service is hybrid", func() {
			BeforeEach(func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "memory",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "hybrid",
						Dimension: 768,
						Model:     "test-model",
					},
				}
				factory = vector.NewVectorDatabaseFactory(config, db, logger)
			})

			It("should create hybrid embedding service", func() {
				embeddingService, err := factory.CreateEmbeddingService()

				Expect(err).NotTo(HaveOccurred())
				Expect(embeddingService).NotTo(BeNil())
				Expect(embeddingService.GetEmbeddingDimension()).To(Equal(768))
			})
		})

		Context("when embedding service is unsupported", func() {
			BeforeEach(func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "memory",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "unsupported",
						Dimension: 384,
					},
				}
				factory = vector.NewVectorDatabaseFactory(config, db, logger)
			})

			It("should return an error", func() {
				embeddingService, err := factory.CreateEmbeddingService()

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unsupported embedding service"))
				Expect(embeddingService).To(BeNil())
			})
		})
	})

	Describe("GetDefaultConfig", func() {
		It("should return valid default configuration", func() {
			defaultConfig := vector.GetDefaultConfig()

			Expect(defaultConfig.Enabled).To(BeTrue())
			Expect(defaultConfig.Backend).To(Equal("postgresql"))
			Expect(defaultConfig.EmbeddingService.Service).To(Equal("local"))
			Expect(defaultConfig.EmbeddingService.Dimension).To(Equal(384))
			Expect(defaultConfig.EmbeddingService.Model).To(Equal("all-MiniLM-L6-v2"))
			Expect(defaultConfig.PostgreSQL.UseMainDB).To(BeTrue())
			Expect(defaultConfig.PostgreSQL.IndexLists).To(Equal(100))
			Expect(defaultConfig.Cache.Enabled).To(BeFalse())
			Expect(defaultConfig.Cache.MaxSize).To(Equal(1000))
			Expect(defaultConfig.Cache.CacheType).To(Equal("memory"))
		})
	})

	Describe("ValidateConfig", func() {
		Context("when config is disabled", func() {
			It("should validate successfully", func() {
				config := &config.VectorDBConfig{
					Enabled: false,
				}

				err := vector.ValidateConfig(config)

				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when config has valid settings", func() {
			It("should validate successfully", func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "postgresql",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "local",
						Dimension: 384,
					},
					PostgreSQL: config.PostgreSQLVectorConfig{
						IndexLists: 100,
					},
				}

				err := vector.ValidateConfig(config)

				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when backend is invalid", func() {
			It("should return validation error", func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "invalid_backend",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "local",
						Dimension: 384,
					},
				}

				err := vector.ValidateConfig(config)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid backend"))
			})
		})

		Context("when embedding service is invalid", func() {
			It("should return validation error", func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "postgresql",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "invalid_service",
						Dimension: 384,
					},
				}

				err := vector.ValidateConfig(config)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid embedding service"))
			})
		})

		Context("when embedding dimension is invalid", func() {
			It("should return validation error for too small dimension", func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "postgresql",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "local",
						Dimension: 0,
					},
				}

				err := vector.ValidateConfig(config)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("embedding dimension must be between 1 and 4096"))
			})

			It("should return validation error for too large dimension", func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "postgresql",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "local",
						Dimension: 5000,
					},
				}

				err := vector.ValidateConfig(config)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("embedding dimension must be between 1 and 4096"))
			})
		})

		Context("when PostgreSQL configuration is invalid", func() {
			It("should return validation error for invalid index lists", func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "postgresql",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "local",
						Dimension: 384,
					},
					PostgreSQL: config.PostgreSQLVectorConfig{
						IndexLists: 0,
					},
				}

				err := vector.ValidateConfig(config)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("PostgreSQL index lists must be between 1 and 1000"))
			})
		})

		Context("when Pinecone configuration is incomplete", func() {
			It("should return validation error for missing API key", func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "pinecone",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "local",
						Dimension: 384,
					},
					Pinecone: config.PineconeConfig{
						APIKey:    "",
						IndexName: "test-index",
					},
				}

				err := vector.ValidateConfig(config)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Pinecone API key is required"))
			})

			It("should return validation error for missing index name", func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "pinecone",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "local",
						Dimension: 384,
					},
					Pinecone: config.PineconeConfig{
						APIKey:    "test-key",
						IndexName: "",
					},
				}

				err := vector.ValidateConfig(config)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Pinecone index name is required"))
			})
		})

		Context("when Weaviate configuration is incomplete", func() {
			It("should return validation error for missing host", func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "weaviate",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "local",
						Dimension: 384,
					},
					Weaviate: config.WeaviateConfig{
						Host:  "",
						Class: "TestClass",
					},
				}

				err := vector.ValidateConfig(config)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Weaviate host is required"))
			})

			It("should return validation error for missing class name", func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "weaviate",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "local",
						Dimension: 384,
					},
					Weaviate: config.WeaviateConfig{
						Host:  "localhost:8080",
						Class: "",
					},
				}

				err := vector.ValidateConfig(config)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Weaviate class name is required"))
			})
		})
	})

	Describe("Edge Cases and Error Scenarios", func() {
		Context("when factory is created with nil config", func() {
			BeforeEach(func() {
				factory = vector.NewVectorDatabaseFactory(nil, db, logger)
			})

			It("should handle nil config gracefully", func() {
				// Should not panic
				vectorDB, err := factory.CreateVectorDatabase()

				// Should return an error or fallback behavior
				if err != nil {
					Expect(vectorDB).To(BeNil())
				} else {
					Expect(vectorDB).NotTo(BeNil())
				}
			})
		})

		Context("when creating embedding service with invalid dimension", func() {
			BeforeEach(func() {
				config := &config.VectorDBConfig{
					Enabled: true,
					Backend: "memory",
					EmbeddingService: config.EmbeddingConfig{
						Service:   "local",
						Dimension: -1, // Invalid dimension
					},
				}
				factory = vector.NewVectorDatabaseFactory(config, db, logger)
			})

			It("should handle invalid dimension gracefully", func() {
				embeddingService, err := factory.CreateEmbeddingService()

				// Should either return error or use default dimension
				if err == nil {
					Expect(embeddingService).NotTo(BeNil())
					Expect(embeddingService.GetEmbeddingDimension()).To(BeNumerically(">", 0))
				}
			})
		})
	})

	Describe("Integration with Different Backends", func() {
		Context("when switching backends", func() {
			It("should create different database types for different backends", func() {
				backends := []string{"memory", "postgresql", "pinecone", "weaviate"}

				for _, backend := range backends {
					config := &config.VectorDBConfig{
						Enabled: true,
						Backend: backend,
						EmbeddingService: config.EmbeddingConfig{
							Service:   "local",
							Dimension: 384,
						},
					}

					factory = vector.NewVectorDatabaseFactory(config, db, logger)

					vectorDB, err := factory.CreateVectorDatabase()

					switch backend {
					case "memory":
						Expect(err).NotTo(HaveOccurred())
						Expect(vectorDB).NotTo(BeNil())
					case "postgresql":
						// Should fail without database connection
						Expect(err).To(HaveOccurred())
					case "pinecone", "weaviate":
						// Should fail as not implemented
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("not implemented"))
					}
				}
			})
		})
	})
})
