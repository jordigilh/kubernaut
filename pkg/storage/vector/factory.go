package vector

import (
	"database/sql"
	"fmt"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/sirupsen/logrus"
)

// VectorDatabaseFactory creates vector database instances based on configuration
type VectorDatabaseFactory struct {
	config *config.VectorDBConfig
	db     *sql.DB // Main database connection
	log    *logrus.Logger
}

// NewVectorDatabaseFactory creates a new factory with the given configuration
func NewVectorDatabaseFactory(vectorConfig *config.VectorDBConfig, db *sql.DB, log *logrus.Logger) *VectorDatabaseFactory {
	return &VectorDatabaseFactory{
		config: vectorConfig,
		db:     db,
		log:    log,
	}
}

// CreateVectorDatabase creates a VectorDatabase instance based on configuration
func (f *VectorDatabaseFactory) CreateVectorDatabase() (VectorDatabase, error) {
	if f.config == nil {
		f.log.Warn("Vector database configuration is nil, using memory fallback")
		return NewMemoryVectorDatabase(f.log), nil
	}

	if !f.config.Enabled {
		f.log.Info("Vector database is disabled, using memory fallback")
		return NewMemoryVectorDatabase(f.log), nil
	}

	// Create embedding service
	embeddingService, err := f.createEmbeddingService()
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding service: %w", err)
	}

	// Create vector database based on backend type
	switch f.config.Backend {
	case "postgresql", "postgres":
		return f.createPostgreSQLVectorDatabase(embeddingService)
	case "pinecone":
		return f.createPineconeVectorDatabase(embeddingService)
	case "weaviate":
		return f.createWeaviateVectorDatabase(embeddingService)
	case "memory", "":
		f.log.Info("Using memory vector database")
		return NewMemoryVectorDatabase(f.log), nil
	default:
		return nil, fmt.Errorf("unsupported vector database backend: %s", f.config.Backend)
	}
}

// CreateEmbeddingService creates an EmbeddingGenerator based on configuration
func (f *VectorDatabaseFactory) CreateEmbeddingService() (EmbeddingGenerator, error) {
	return f.createEmbeddingService()
}

// CreatePatternExtractor creates a PatternExtractor with the given embedding service
func (f *VectorDatabaseFactory) CreatePatternExtractor(embeddingService EmbeddingGenerator) PatternExtractor {
	return NewDefaultPatternExtractor(embeddingService, f.log)
}

// Private helper methods

func (f *VectorDatabaseFactory) createEmbeddingService() (EmbeddingGenerator, error) {
	if f.config == nil {
		// Return default local embedding service
		return NewLocalEmbeddingService(384, f.log), nil
	}

	embeddingConfig := f.config.EmbeddingService

	// Set default dimension if not specified
	dimension := embeddingConfig.Dimension
	if dimension <= 0 {
		dimension = 384 // Default for sentence-transformers/all-MiniLM-L6-v2
	}

	switch embeddingConfig.Service {
	case "local", "":
		f.log.WithField("dimension", dimension).Info("Using local embedding service")
		return NewLocalEmbeddingService(dimension, f.log), nil

	case "hybrid":
		// Create local service as fallback
		localService := NewLocalEmbeddingService(dimension, f.log)

		// Try to create external service for hybrid approach
		var externalService EmbeddingGenerator

		// For now, we don't have external services implemented
		// In the future, we could add OpenAI, HuggingFace, etc.
		f.log.Info("Using hybrid embedding service with local fallback")
		return NewHybridEmbeddingService(localService, externalService, f.log), nil

	case "openai":
		// TODO: Implement OpenAI embedding service
		return nil, fmt.Errorf("OpenAI embedding service not implemented yet")

	case "huggingface":
		// TODO: Implement HuggingFace embedding service
		return nil, fmt.Errorf("HuggingFace embedding service not implemented yet")

	default:
		return nil, fmt.Errorf("unsupported embedding service: %s", embeddingConfig.Service)
	}
}

func (f *VectorDatabaseFactory) createPostgreSQLVectorDatabase(embeddingService EmbeddingGenerator) (VectorDatabase, error) {
	pgConfig := f.config.PostgreSQL

	// Use main database connection by default
	var database *sql.DB
	if pgConfig.UseMainDB || (pgConfig.Host == "" && pgConfig.Database == "") {
		if f.db == nil {
			return nil, fmt.Errorf("main database connection not available")
		}
		database = f.db
		f.log.Info("Using main database connection for vector database")
	} else {
		// TODO: Create separate connection if different credentials provided
		// For now, we'll use the main connection
		database = f.db
		f.log.Warn("Separate PostgreSQL connection not implemented, using main connection")
	}

	f.log.WithFields(logrus.Fields{
		"backend": "postgresql",
		"lists":   pgConfig.IndexLists,
	}).Info("Creating PostgreSQL vector database")

	return NewPostgreSQLVectorDatabase(database, embeddingService, f.log), nil
}

func (f *VectorDatabaseFactory) createPineconeVectorDatabase(embeddingService EmbeddingGenerator) (VectorDatabase, error) {
	// TODO: Implement Pinecone vector database
	return nil, fmt.Errorf("Pinecone vector database not implemented yet")
}

func (f *VectorDatabaseFactory) createWeaviateVectorDatabase(embeddingService EmbeddingGenerator) (VectorDatabase, error) {
	// TODO: Implement Weaviate vector database
	return nil, fmt.Errorf("Weaviate vector database not implemented yet")
}

// GetDefaultConfig returns a default vector database configuration
func GetDefaultConfig() config.VectorDBConfig {
	return config.VectorDBConfig{
		Enabled: true,
		Backend: "postgresql",
		EmbeddingService: config.EmbeddingConfig{
			Service:   "local",
			Dimension: 384,
			Model:     "all-MiniLM-L6-v2",
		},
		PostgreSQL: config.PostgreSQLVectorConfig{
			UseMainDB:  true,
			IndexLists: 100,
		},
		Cache: config.VectorCacheConfig{
			Enabled:   false, // Disabled by default
			TTL:       0,     // No TTL by default
			MaxSize:   1000,  // 1000 cached embeddings
			CacheType: "memory",
		},
	}
}

// ValidateConfig validates the vector database configuration
func ValidateConfig(config *config.VectorDBConfig) error {
	if !config.Enabled {
		return nil // No validation needed if disabled
	}

	// Validate backend
	validBackends := []string{"postgresql", "postgres", "pinecone", "weaviate", "memory"}
	backendValid := false
	for _, backend := range validBackends {
		if config.Backend == backend {
			backendValid = true
			break
		}
	}
	if !backendValid {
		return fmt.Errorf("invalid backend '%s', must be one of: %v", config.Backend, validBackends)
	}

	// Validate embedding service
	validServices := []string{"local", "hybrid", "openai", "huggingface"}
	serviceValid := false
	for _, service := range validServices {
		if config.EmbeddingService.Service == service {
			serviceValid = true
			break
		}
	}
	if !serviceValid {
		return fmt.Errorf("invalid embedding service '%s', must be one of: %v", config.EmbeddingService.Service, validServices)
	}

	// Validate dimension
	if config.EmbeddingService.Dimension < 1 || config.EmbeddingService.Dimension > 4096 {
		return fmt.Errorf("embedding dimension must be between 1 and 4096, got %d", config.EmbeddingService.Dimension)
	}

	// Backend-specific validation
	switch config.Backend {
	case "postgresql", "postgres":
		if config.PostgreSQL.IndexLists < 1 || config.PostgreSQL.IndexLists > 1000 {
			return fmt.Errorf("PostgreSQL index lists must be between 1 and 1000, got %d", config.PostgreSQL.IndexLists)
		}
	case "pinecone":
		if config.Pinecone.APIKey == "" {
			return fmt.Errorf("Pinecone API key is required")
		}
		if config.Pinecone.IndexName == "" {
			return fmt.Errorf("Pinecone index name is required")
		}
	case "weaviate":
		if config.Weaviate.Host == "" {
			return fmt.Errorf("Weaviate host is required")
		}
		if config.Weaviate.Class == "" {
			return fmt.Errorf("Weaviate class name is required")
		}
	}

	return nil
}
