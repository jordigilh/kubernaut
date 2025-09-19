package vector

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/sirupsen/logrus"

	// PostgreSQL driver
	_ "github.com/lib/pq"
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

	// Create base embedding service
	var baseService EmbeddingGenerator
	var err error

	switch embeddingConfig.Service {
	case "local", "":
		f.log.WithField("dimension", dimension).Info("Using local embedding service")
		baseService = NewLocalEmbeddingService(dimension, f.log)

	case "hybrid":
		// Create local service as fallback
		localService := NewLocalEmbeddingService(dimension, f.log)

		// Try to create external service for hybrid approach
		var externalService EmbeddingGenerator

		// For now, we don't have external services implemented
		// In the future, we could add OpenAI, HuggingFace, etc.
		f.log.Info("Using hybrid embedding service with local fallback")
		baseService = NewHybridEmbeddingService(localService, externalService, f.log)

	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY environment variable is required for OpenAI embedding service")
		}
		f.log.Info("Creating OpenAI embedding service")
		externalService := NewOpenAIEmbeddingService(apiKey, nil, f.log)
		baseService = NewEmbeddingGeneratorAdapter(externalService)

	case "huggingface":
		apiKey := os.Getenv("HUGGINGFACE_API_KEY")
		f.log.Info("Creating HuggingFace embedding service")
		externalService := NewHuggingFaceEmbeddingService(apiKey, nil, f.log)
		baseService = NewEmbeddingGeneratorAdapter(externalService)

	default:
		return nil, fmt.Errorf("unsupported embedding service: %s", embeddingConfig.Service)
	}

	// Add caching if enabled
	if f.config.Cache.Enabled && f.config.Cache.CacheType != "" {
		cache, err := f.createEmbeddingCache()
		if err != nil {
			f.log.WithError(err).Warn("Failed to create embedding cache, continuing without cache")
			return baseService, nil
		}

		f.log.WithFields(logrus.Fields{
			"cache_type": f.config.Cache.CacheType,
			"ttl":        f.config.Cache.TTL,
			"max_size":   f.config.Cache.MaxSize,
		}).Info("Embedding service caching enabled")

		return NewCachedEmbeddingService(baseService, cache, f.config.Cache.TTL, f.log), nil
	}

	return baseService, err
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
		// Create separate connection with provided credentials
		separateDB, err := f.createSeparatePostgreSQLConnection(pgConfig)
		if err != nil {
			f.log.WithError(err).Warn("Failed to create separate PostgreSQL connection, falling back to main connection")
			database = f.db
		} else {
			database = separateDB
			f.log.Info("Created separate PostgreSQL connection for vector database")
		}
	}

	f.log.WithFields(logrus.Fields{
		"backend": "postgresql",
		"lists":   pgConfig.IndexLists,
	}).Info("Creating PostgreSQL vector database")

	return NewPostgreSQLVectorDatabase(database, embeddingService, f.log), nil
}

func (f *VectorDatabaseFactory) createPineconeVectorDatabase(embeddingService EmbeddingGenerator) (VectorDatabase, error) {
	apiKey := os.Getenv("PINECONE_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("PINECONE_API_KEY environment variable is required for Pinecone vector database")
	}

	environment := os.Getenv("PINECONE_ENVIRONMENT")
	if environment == "" {
		environment = "us-west1-gcp-free" // Default free tier environment
	}

	indexName := os.Getenv("PINECONE_INDEX")
	if indexName == "" {
		indexName = "kubernaut" // Default index name
	}

	config := &PineconeConfig{
		Environment: environment,
		IndexName:   indexName,
		Dimensions:  embeddingService.GetEmbeddingDimension(),
		Metric:      "cosine",
		MaxRetries:  3,
		Timeout:     30 * time.Second,
		BatchSize:   100,
	}

	f.log.WithFields(logrus.Fields{
		"environment": environment,
		"index_name":  indexName,
		"dimensions":  config.Dimensions,
	}).Info("Creating Pinecone vector database")

	// Create external service adapter
	externalEmbeddingService := &ExternalEmbeddingAdapter{standard: embeddingService}
	externalDB := NewPineconeVectorDatabase(apiKey, config, externalEmbeddingService, f.log)
	return NewExternalVectorDatabaseAdapter(externalDB, embeddingService), nil
}

func (f *VectorDatabaseFactory) createWeaviateVectorDatabase(embeddingService EmbeddingGenerator) (VectorDatabase, error) {
	baseURL := os.Getenv("WEAVIATE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080" // Default local Weaviate instance
	}

	apiKey := os.Getenv("WEAVIATE_API_KEY") // Optional for local instances

	className := os.Getenv("WEAVIATE_CLASS")
	if className == "" {
		className = "KubernautVector" // Default class name
	}

	config := &WeaviateConfig{
		BaseURL:    baseURL,
		ClassName:  className,
		APIKey:     apiKey,
		MaxRetries: 3,
		Timeout:    30 * time.Second,
		BatchSize:  100,
	}

	f.log.WithFields(logrus.Fields{
		"base_url":    baseURL,
		"class_name":  className,
		"has_api_key": apiKey != "",
	}).Info("Creating Weaviate vector database")

	// Create external service adapter
	externalEmbeddingService := &ExternalEmbeddingAdapter{standard: embeddingService}
	externalDB := NewWeaviateVectorDatabase(config, externalEmbeddingService, f.log)
	return NewExternalVectorDatabaseAdapter(externalDB, embeddingService), nil
}

// createEmbeddingCache creates an embedding cache based on configuration
func (f *VectorDatabaseFactory) createEmbeddingCache() (EmbeddingCache, error) {
	if f.config == nil || !f.config.Cache.Enabled {
		return nil, fmt.Errorf("cache configuration is nil or disabled")
	}

	switch f.config.Cache.CacheType {
	case "memory":
		f.log.WithField("max_size", f.config.Cache.MaxSize).Info("Creating memory embedding cache")
		return NewMemoryEmbeddingCache(f.config.Cache.MaxSize, f.log), nil

	case "redis":
		// Default Redis configuration for integration tests
		redisAddr := "localhost:6380"
		redisPassword := "integration_redis_password"
		redisDB := 0

		// Allow override from environment variables
		if envAddr := getEnvOrDefault("REDIS_ADDR", ""); envAddr != "" {
			redisAddr = envAddr
		}
		if envPassword := getEnvOrDefault("REDIS_PASSWORD", ""); envPassword != "" {
			redisPassword = envPassword
		}

		f.log.WithFields(logrus.Fields{
			"redis_addr": redisAddr,
			"redis_db":   redisDB,
		}).Info("Creating Redis embedding cache")

		return NewRedisEmbeddingCache(redisAddr, redisPassword, redisDB, f.log)

	default:
		return nil, fmt.Errorf("unsupported cache type: %s", f.config.Cache.CacheType)
	}
}

// getEnvOrDefault gets environment variable with fallback
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
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
			Enabled:   true,           // BR-CONFIG-01: Enable caching by default for production
			TTL:       24 * time.Hour, // 24 hour TTL for production use
			MaxSize:   1000,           // 1000 cached embeddings
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

	// Backend-specific validation and defaults application
	switch config.Backend {
	case "postgresql", "postgres":
		// BR-CONFIG-01: Apply defaults for graceful handling of minimal configs
		if config.PostgreSQL.IndexLists == 0 {
			config.PostgreSQL.IndexLists = 100 // Default from GetDefaultConfig()
		}
		// Default to UseMainDB if not explicitly set to false
		if !config.PostgreSQL.UseMainDB && config.PostgreSQL.Host == "" {
			config.PostgreSQL.UseMainDB = true
		}
		if config.PostgreSQL.IndexLists < 1 || config.PostgreSQL.IndexLists > 1000 {
			return fmt.Errorf("PostgreSQL index lists must be between 1 and 1000, got %d", config.PostgreSQL.IndexLists)
		}
	case "pinecone":
		if config.Pinecone.APIKey == "" {
			return fmt.Errorf("pinecone API key is required")
		}
		if config.Pinecone.IndexName == "" {
			return fmt.Errorf("pinecone index name is required")
		}
	case "weaviate":
		if config.Weaviate.Host == "" {
			return fmt.Errorf("weaviate host is required")
		}
		if config.Weaviate.Class == "" {
			return fmt.Errorf("weaviate class name is required")
		}
	}

	return nil
}

// createSeparatePostgreSQLConnection creates a separate PostgreSQL connection with custom credentials
func (f *VectorDatabaseFactory) createSeparatePostgreSQLConnection(pgConfig config.PostgreSQLVectorConfig) (*sql.DB, error) {
	// Build connection string from config
	connStr := f.buildPostgreSQLConnectionString(pgConfig)

	f.log.WithFields(logrus.Fields{
		"host":     pgConfig.Host,
		"database": pgConfig.Database,
		"user":     pgConfig.Username,
		"port":     pgConfig.Port,
	}).Debug("Creating separate PostgreSQL connection for vector database")

	// Open database connection
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open PostgreSQL connection: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	// Test the connection
	if err := db.Ping(); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			f.log.WithError(closeErr).Error("Failed to close database connection during ping failure cleanup")
		}
		return nil, fmt.Errorf("failed to ping PostgreSQL database: %w", err)
	}

	f.log.Info("Successfully created separate PostgreSQL connection for vector database")
	return db, nil
}

// buildPostgreSQLConnectionString builds a PostgreSQL connection string from config
func (f *VectorDatabaseFactory) buildPostgreSQLConnectionString(pgConfig config.PostgreSQLVectorConfig) string {
	// Use reasonable defaults if not specified
	host := pgConfig.Host
	if host == "" {
		host = "localhost"
	}

	port := pgConfig.Port
	if port == "" {
		port = "5432"
	}

	database := pgConfig.Database
	if database == "" {
		database = "kubernaut"
	}

	user := pgConfig.Username
	if user == "" {
		user = "postgres"
	}

	password := pgConfig.Password

	// Build connection string
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, database)

	return connStr
}
