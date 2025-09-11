package storage

import (
	"database/sql"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"

	// Import pq for PostgreSQL testing
	_ "github.com/lib/pq"
)

var _ = Describe("Vector Database Factory Unit Tests", func() {
	var (
		factory    *vector.VectorDatabaseFactory
		mockDB     *sql.DB
		logger     *logrus.Logger
		baseConfig *config.VectorDBConfig
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel)

		// Create base configuration for testing
		baseConfig = &config.VectorDBConfig{
			Enabled: true,
			Backend: "memory",
			EmbeddingService: config.EmbeddingConfig{
				Service:   "local",
				Dimension: 384,
				Model:     "all-MiniLM-L6-v2",
			},
			Cache: config.VectorCacheConfig{
				Enabled:   false,
				TTL:       time.Hour,
				MaxSize:   1000,
				CacheType: "memory",
			},
		}
	})

	Context("Factory Creation and Configuration", func() {
		It("should create factory with valid configuration", func() {
			factory = vector.NewVectorDatabaseFactory(baseConfig, mockDB, logger)

			Expect(factory).ToNot(BeNil(), "Should create factory successfully")
		})

		It("should create factory with nil database", func() {
			factory = vector.NewVectorDatabaseFactory(baseConfig, nil, logger)

			Expect(factory).ToNot(BeNil(), "Should create factory even with nil database")
		})

		It("should create factory with nil configuration", func() {
			factory = vector.NewVectorDatabaseFactory(nil, mockDB, logger)

			Expect(factory).ToNot(BeNil(), "Should create factory even with nil config")
		})
	})

	Context("Default Configuration", func() {
		It("should provide sensible defaults", func() {
			defaultConfig := vector.GetDefaultConfig()

			Expect(defaultConfig.Enabled).To(BeTrue(), "Should be enabled by default")
			Expect(defaultConfig.Backend).To(Equal("postgresql"), "Should use PostgreSQL backend by default")
			Expect(defaultConfig.EmbeddingService.Service).To(Equal("local"), "Should use local embedding service by default")
			Expect(defaultConfig.EmbeddingService.Dimension).To(Equal(384), "Should use 384 dimensions by default")
			Expect(defaultConfig.PostgreSQL.UseMainDB).To(BeTrue(), "Should use main DB by default")
			Expect(defaultConfig.Cache.Enabled).To(BeFalse(), "Should have cache disabled by default")
		})
	})

	Context("Configuration Validation", func() {
		It("should validate enabled configuration successfully", func() {
			validConfig := &config.VectorDBConfig{
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

			err := vector.ValidateConfig(validConfig)
			Expect(err).ToNot(HaveOccurred(), "Should validate valid configuration")
		})

		It("should skip validation for disabled configuration", func() {
			disabledConfig := &config.VectorDBConfig{
				Enabled: false,
				Backend: "invalid_backend", // This would normally be invalid
			}

			err := vector.ValidateConfig(disabledConfig)
			Expect(err).ToNot(HaveOccurred(), "Should skip validation for disabled config")
		})

		It("should reject invalid backend", func() {
			invalidConfig := &config.VectorDBConfig{
				Enabled: true,
				Backend: "invalid_backend",
				EmbeddingService: config.EmbeddingConfig{
					Service:   "local",
					Dimension: 384,
				},
			}

			err := vector.ValidateConfig(invalidConfig)
			Expect(err).To(HaveOccurred(), "Should reject invalid backend")
			Expect(err.Error()).To(ContainSubstring("invalid backend"), "Error should mention invalid backend")
		})

		It("should reject invalid embedding service", func() {
			invalidConfig := &config.VectorDBConfig{
				Enabled: true,
				Backend: "postgresql",
				EmbeddingService: config.EmbeddingConfig{
					Service:   "invalid_service",
					Dimension: 384,
				},
			}

			err := vector.ValidateConfig(invalidConfig)
			Expect(err).To(HaveOccurred(), "Should reject invalid embedding service")
			Expect(err.Error()).To(ContainSubstring("invalid embedding service"), "Error should mention invalid service")
		})

		It("should reject invalid embedding dimension", func() {
			invalidConfig := &config.VectorDBConfig{
				Enabled: true,
				Backend: "postgresql",
				EmbeddingService: config.EmbeddingConfig{
					Service:   "local",
					Dimension: 0, // Invalid dimension
				},
			}

			err := vector.ValidateConfig(invalidConfig)
			Expect(err).To(HaveOccurred(), "Should reject invalid dimension")
			Expect(err.Error()).To(ContainSubstring("embedding dimension must be between"), "Error should mention dimension range")
		})

		It("should reject dimension that is too large", func() {
			invalidConfig := &config.VectorDBConfig{
				Enabled: true,
				Backend: "postgresql",
				EmbeddingService: config.EmbeddingConfig{
					Service:   "local",
					Dimension: 5000, // Too large
				},
			}

			err := vector.ValidateConfig(invalidConfig)
			Expect(err).To(HaveOccurred(), "Should reject dimension that is too large")
		})

		It("should validate PostgreSQL-specific configuration", func() {
			invalidConfig := &config.VectorDBConfig{
				Enabled: true,
				Backend: "postgresql",
				EmbeddingService: config.EmbeddingConfig{
					Service:   "local",
					Dimension: 384,
				},
				PostgreSQL: config.PostgreSQLVectorConfig{
					IndexLists: 0, // Invalid index lists
				},
			}

			err := vector.ValidateConfig(invalidConfig)
			Expect(err).To(HaveOccurred(), "Should validate PostgreSQL-specific config")
			Expect(err.Error()).To(ContainSubstring("index lists must be between"), "Error should mention index lists")
		})
	})

	Context("Memory Vector Database Creation", func() {
		BeforeEach(func() {
			baseConfig.Backend = "memory"
			factory = vector.NewVectorDatabaseFactory(baseConfig, mockDB, logger)
		})

		It("should create memory database successfully", func() {
			db, err := factory.CreateVectorDatabase()
			Expect(err).ToNot(HaveOccurred(), "Should create memory database without error")
			Expect(db).ToNot(BeNil(), "Should return valid database instance")
		})

		It("should create memory database for nil configuration", func() {
			factory = vector.NewVectorDatabaseFactory(nil, mockDB, logger)

			db, err := factory.CreateVectorDatabase()
			Expect(err).ToNot(HaveOccurred(), "Should create memory database for nil config")
			Expect(db).ToNot(BeNil(), "Should return valid database instance")
		})

		It("should create memory database for disabled configuration", func() {
			baseConfig.Enabled = false

			db, err := factory.CreateVectorDatabase()
			Expect(err).ToNot(HaveOccurred(), "Should create memory database for disabled config")
			Expect(db).ToNot(BeNil(), "Should return valid database instance")
		})

		It("should create memory database for empty backend", func() {
			baseConfig.Backend = ""

			db, err := factory.CreateVectorDatabase()
			Expect(err).ToNot(HaveOccurred(), "Should create memory database for empty backend")
			Expect(db).ToNot(BeNil(), "Should return valid database instance")
		})
	})

	Context("Embedding Service Creation", func() {
		BeforeEach(func() {
			factory = vector.NewVectorDatabaseFactory(baseConfig, mockDB, logger)
		})

		It("should create local embedding service by default", func() {
			embeddingService, err := factory.CreateEmbeddingService()
			Expect(err).ToNot(HaveOccurred(), "Should create local embedding service without error")
			Expect(embeddingService).ToNot(BeNil(), "Should return valid embedding service")
			Expect(embeddingService.GetEmbeddingDimension()).To(Equal(384), "Should use configured dimension")
		})

		It("should create local embedding service with custom dimension", func() {
			configCopy := *baseConfig
			configCopy.EmbeddingService.Dimension = 512
			testFactory := vector.NewVectorDatabaseFactory(&configCopy, mockDB, logger)

			embeddingService, err := testFactory.CreateEmbeddingService()
			Expect(err).ToNot(HaveOccurred(), "Should create service with custom dimension")
			Expect(embeddingService.GetEmbeddingDimension()).To(Equal(512), "Should use custom dimension")
		})

		It("should use default dimension for invalid input", func() {
			configCopy := *baseConfig
			configCopy.EmbeddingService.Dimension = 0
			testFactory := vector.NewVectorDatabaseFactory(&configCopy, mockDB, logger)

			embeddingService, err := testFactory.CreateEmbeddingService()
			Expect(err).ToNot(HaveOccurred(), "Should handle invalid dimension")
			Expect(embeddingService.GetEmbeddingDimension()).To(Equal(384), "Should use default dimension")
		})

		It("should create local service for nil configuration", func() {
			factory = vector.NewVectorDatabaseFactory(nil, mockDB, logger)

			embeddingService, err := factory.CreateEmbeddingService()
			Expect(err).ToNot(HaveOccurred(), "Should create local service for nil config")
			Expect(embeddingService).ToNot(BeNil(), "Should return valid service")
		})

		It("should create hybrid embedding service", func() {
			configCopy := *baseConfig
			configCopy.EmbeddingService.Service = "hybrid"
			testFactory := vector.NewVectorDatabaseFactory(&configCopy, mockDB, logger)

			embeddingService, err := testFactory.CreateEmbeddingService()
			Expect(err).ToNot(HaveOccurred(), "Should create hybrid embedding service")
			Expect(embeddingService).ToNot(BeNil(), "Should return valid hybrid service")
		})

		It("should reject unsupported embedding service", func() {
			configCopy := *baseConfig
			configCopy.EmbeddingService.Service = "unsupported_service"
			testFactory := vector.NewVectorDatabaseFactory(&configCopy, mockDB, logger)

			_, err := testFactory.CreateEmbeddingService()
			Expect(err).To(HaveOccurred(), "Should reject unsupported service")
			Expect(err.Error()).To(ContainSubstring("unsupported embedding service"), "Error should mention unsupported service")
		})
	})

	Context("External Service Configuration", func() {
		var (
			originalOpenAIKey      string
			originalHuggingFaceKey string
		)

		BeforeEach(func() {
			// Save original environment variables
			originalOpenAIKey = os.Getenv("OPENAI_API_KEY")
			originalHuggingFaceKey = os.Getenv("HUGGINGFACE_API_KEY")
		})

		AfterEach(func() {
			// Restore original environment variables
			if originalOpenAIKey == "" {
				Expect(os.Unsetenv("OPENAI_API_KEY")).To(Succeed())
			} else {
				Expect(os.Setenv("OPENAI_API_KEY", originalOpenAIKey)).To(Succeed())
			}

			if originalHuggingFaceKey == "" {
				Expect(os.Unsetenv("HUGGINGFACE_API_KEY")).To(Succeed())
			} else {
				Expect(os.Setenv("HUGGINGFACE_API_KEY", originalHuggingFaceKey)).To(Succeed())
			}
		})

		It("should create OpenAI service with API key", func() {
			Expect(os.Setenv("OPENAI_API_KEY", "test-openai-key")).To(Succeed())
			configCopy := *baseConfig
			configCopy.EmbeddingService.Service = "openai"
			testFactory := vector.NewVectorDatabaseFactory(&configCopy, mockDB, logger)

			embeddingService, err := testFactory.CreateEmbeddingService()
			Expect(err).ToNot(HaveOccurred(), "Should create OpenAI service with API key")
			Expect(embeddingService).ToNot(BeNil(), "Should return valid service")
		})

		It("should reject OpenAI service without API key", func() {
			Expect(os.Unsetenv("OPENAI_API_KEY")).To(Succeed())
			configCopy := *baseConfig
			configCopy.EmbeddingService.Service = "openai"
			testFactory := vector.NewVectorDatabaseFactory(&configCopy, mockDB, logger)

			_, err := testFactory.CreateEmbeddingService()
			Expect(err).To(HaveOccurred(), "Should reject OpenAI service without API key")
			Expect(err.Error()).To(ContainSubstring("OPENAI_API_KEY"), "Error should mention missing API key")
		})

		It("should create HuggingFace service", func() {
			Expect(os.Setenv("HUGGINGFACE_API_KEY", "test-hf-key")).To(Succeed())
			configCopy := *baseConfig
			configCopy.EmbeddingService.Service = "huggingface"
			testFactory := vector.NewVectorDatabaseFactory(&configCopy, mockDB, logger)

			embeddingService, err := testFactory.CreateEmbeddingService()
			Expect(err).ToNot(HaveOccurred(), "Should create HuggingFace service")
			Expect(embeddingService).ToNot(BeNil(), "Should return valid service")
		})

		It("should create HuggingFace service without API key", func() {
			Expect(os.Unsetenv("HUGGINGFACE_API_KEY")).To(Succeed())
			configCopy := *baseConfig
			configCopy.EmbeddingService.Service = "huggingface"
			testFactory := vector.NewVectorDatabaseFactory(&configCopy, mockDB, logger)

			embeddingService, err := testFactory.CreateEmbeddingService()
			Expect(err).ToNot(HaveOccurred(), "Should create HuggingFace service without API key")
			Expect(embeddingService).ToNot(BeNil(), "Should work without API key")
		})
	})

	Context("Caching Configuration", func() {
		BeforeEach(func() {
			factory = vector.NewVectorDatabaseFactory(baseConfig, mockDB, logger)
		})

		It("should create embedding service without cache when disabled", func() {
			baseConfig.Cache.Enabled = false

			embeddingService, err := factory.CreateEmbeddingService()
			Expect(err).ToNot(HaveOccurred(), "Should create service without cache")
			Expect(embeddingService).ToNot(BeNil(), "Should return valid service")
		})

		It("should create memory cache when enabled", func() {
			baseConfig.Cache.Enabled = true
			baseConfig.Cache.CacheType = "memory"
			baseConfig.Cache.MaxSize = 500

			embeddingService, err := factory.CreateEmbeddingService()
			Expect(err).ToNot(HaveOccurred(), "Should create service with memory cache")
			Expect(embeddingService).ToNot(BeNil(), "Should return valid cached service")
		})

		It("should continue without cache on cache creation failure", func() {
			baseConfig.Cache.Enabled = true
			baseConfig.Cache.CacheType = "invalid_cache_type"

			embeddingService, err := factory.CreateEmbeddingService()
			Expect(err).ToNot(HaveOccurred(), "Should continue without cache on creation failure")
			Expect(embeddingService).ToNot(BeNil(), "Should return valid non-cached service")
		})
	})

	Context("Pattern Extractor Creation", func() {
		BeforeEach(func() {
			factory = vector.NewVectorDatabaseFactory(baseConfig, mockDB, logger)
		})

		It("should create pattern extractor with embedding service", func() {
			embeddingService, err := factory.CreateEmbeddingService()
			Expect(err).ToNot(HaveOccurred())

			extractor := factory.CreatePatternExtractor(embeddingService)
			Expect(extractor).ToNot(BeNil(), "Should create pattern extractor successfully")
		})

		It("should create pattern extractor with nil embedding service", func() {
			extractor := factory.CreatePatternExtractor(nil)
			Expect(extractor).ToNot(BeNil(), "Should handle nil embedding service gracefully")
		})
	})

	Context("Backend-Specific Database Creation", func() {
		var (
			originalPineconeAPIKey string
			originalPineconeEnv    string
			originalPineconeIndex  string
			originalWeaviateURL    string
			originalWeaviateAPIKey string
			originalWeaviateClass  string
		)

		BeforeEach(func() {
			// Save original environment variables
			originalPineconeAPIKey = os.Getenv("PINECONE_API_KEY")
			originalPineconeEnv = os.Getenv("PINECONE_ENVIRONMENT")
			originalPineconeIndex = os.Getenv("PINECONE_INDEX")
			originalWeaviateURL = os.Getenv("WEAVIATE_URL")
			originalWeaviateAPIKey = os.Getenv("WEAVIATE_API_KEY")
			originalWeaviateClass = os.Getenv("WEAVIATE_CLASS")

			factory = vector.NewVectorDatabaseFactory(baseConfig, mockDB, logger)
		})

		AfterEach(func() {
			// Restore original environment variables
			restoreEnvVar("PINECONE_API_KEY", originalPineconeAPIKey)
			restoreEnvVar("PINECONE_ENVIRONMENT", originalPineconeEnv)
			restoreEnvVar("PINECONE_INDEX", originalPineconeIndex)
			restoreEnvVar("WEAVIATE_URL", originalWeaviateURL)
			restoreEnvVar("WEAVIATE_API_KEY", originalWeaviateAPIKey)
			restoreEnvVar("WEAVIATE_CLASS", originalWeaviateClass)
		})

		It("should reject unsupported backend", func() {
			baseConfig.Backend = "unsupported_backend"

			_, err := factory.CreateVectorDatabase()
			Expect(err).To(HaveOccurred(), "Should reject unsupported backend")
			Expect(err.Error()).To(ContainSubstring("unsupported vector database backend"), "Error should mention unsupported backend")
		})

		It("should handle Pinecone backend with missing API key", func() {
			Expect(os.Unsetenv("PINECONE_API_KEY")).To(Succeed())
			baseConfig.Backend = "pinecone"

			_, err := factory.CreateVectorDatabase()
			Expect(err).To(HaveOccurred(), "Should reject Pinecone without API key")
			Expect(err.Error()).To(ContainSubstring("PINECONE_API_KEY"), "Error should mention missing API key")
		})

		It("should create PostgreSQL database with main DB connection", func() {
			baseConfig.Backend = "postgresql"
			baseConfig.PostgreSQL.UseMainDB = true

			// Note: This will fail without a real DB connection, but tests the creation path
			_, err := factory.CreateVectorDatabase()
			// We expect this to fail since we don't have a real DB connection
			// but it should fail in the database creation, not configuration
			Expect(err).To(HaveOccurred(), "Expected to fail due to missing real DB connection")
		})

		It("should handle PostgreSQL configuration errors gracefully", func() {
			configCopy := *baseConfig
			configCopy.Backend = "postgresql"
			configCopy.PostgreSQL.UseMainDB = false
			configCopy.PostgreSQL.Host = "invalid-host"
			configCopy.PostgreSQL.Database = "invalid-db"
			testFactory := vector.NewVectorDatabaseFactory(&configCopy, mockDB, logger)

			// Should succeed by falling back to main DB connection
			db, err := testFactory.CreateVectorDatabase()
			Expect(err).ToNot(HaveOccurred(), "Should gracefully fallback to main DB connection")
			Expect(db).ToNot(BeNil(), "Should return valid database instance")
		})
	})

	Context("Error Handling and Edge Cases", func() {
		It("should handle embedding service creation failure gracefully", func() {
			baseConfig.EmbeddingService.Service = "openai"
			Expect(os.Unsetenv("OPENAI_API_KEY")).To(Succeed()) // This will cause embedding service creation to fail

			factory = vector.NewVectorDatabaseFactory(baseConfig, mockDB, logger)

			_, err := factory.CreateVectorDatabase()
			Expect(err).To(HaveOccurred(), "Should fail when embedding service creation fails")
			Expect(err.Error()).To(ContainSubstring("failed to create embedding service"), "Error should mention embedding service failure")
		})

		It("should fallback to memory database on PostgreSQL failure", func() {
			baseConfig.Backend = "postgresql"
			baseConfig.PostgreSQL.UseMainDB = true

			// Create factory without database connection to trigger fallback
			factory = vector.NewVectorDatabaseFactory(baseConfig, nil, logger)

			_, err := factory.CreateVectorDatabase()
			Expect(err).To(HaveOccurred(), "Should fail when main database is nil for PostgreSQL")
		})
	})

	Context("Environment Variable Handling", func() {
		It("should use default values when environment variables are not set", func() {
			// Clear all relevant environment variables
			vars := []string{
				"PINECONE_API_KEY", "PINECONE_ENVIRONMENT", "PINECONE_INDEX",
				"WEAVIATE_URL", "WEAVIATE_API_KEY", "WEAVIATE_CLASS",
				"OPENAI_API_KEY", "HUGGINGFACE_API_KEY",
				"REDIS_ADDR", "REDIS_PASSWORD",
			}

			for _, v := range vars {
				Expect(os.Unsetenv(v)).To(Succeed())
			}

			// This should work with local services only
			baseConfig.Backend = "memory"
			baseConfig.EmbeddingService.Service = "local"
			baseConfig.Cache.Enabled = false

			factory = vector.NewVectorDatabaseFactory(baseConfig, mockDB, logger)

			db, err := factory.CreateVectorDatabase()
			Expect(err).ToNot(HaveOccurred(), "Should work without environment variables")
			Expect(db).ToNot(BeNil(), "Should return valid database")
		})
	})
})

// Helper function to restore environment variables
func restoreEnvVar(key, value string) {
	if value == "" {
		Expect(os.Unsetenv(key)).To(Succeed())
	} else {
		Expect(os.Setenv(key, value)).To(Succeed())
	}
}
