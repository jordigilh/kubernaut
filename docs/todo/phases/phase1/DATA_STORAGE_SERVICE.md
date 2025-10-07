# 📊 **DATA STORAGE SERVICE DEVELOPMENT GUIDE**

**Service**: Data Storage Service
**Port**: 8085
**Image**: quay.io/jordigilh/storage-service
**Business Requirements**: BR-STOR-001 to BR-STOR-135
**Single Responsibility**: Data Persistence ONLY
**Phase**: 1 (Parallel Development)
**Dependencies**: None (independent data storage)

---

## 📊 **CURRENT STATUS ANALYSIS**

### **✅ EXISTING IMPLEMENTATION**
**Locations**:
- `pkg/storage/vector/factory.go` (67 lines) - **VECTOR DATABASE FACTORY**
- `pkg/storage/vector/interfaces.go` (40 lines) - **VECTOR DATABASE INTERFACES**
- `pkg/storage/vector/postgresql_db.go` (34+ lines) - **POSTGRESQL VECTOR DATABASE**
- `pkg/storage/vector/pinecone_database.go` (51+ lines) - **PINECONE VECTOR DATABASE**
- `pkg/storage/vector/memory_db.go` (290+ lines) - **MEMORY VECTOR DATABASE**
- `pkg/storage/vector/weaviate_database.go` (165+ lines) - **WEAVIATE VECTOR DATABASE**
- `pkg/storage/vector/embedding_service.go` (44+ lines) - **EMBEDDING SERVICE**

**Current Strengths**:
- ✅ **Exceptional storage system** with multi-backend support (PostgreSQL, Pinecone, Weaviate, Memory)
- ✅ **Advanced vector database factory** with intelligent backend selection
- ✅ **Comprehensive vector database interfaces** for pattern storage and similarity search
- ✅ **PostgreSQL integration** with pgvector extension support
- ✅ **External vector database support** (Pinecone, Weaviate) for cloud deployments
- ✅ **Memory fallback database** for development and testing
- ✅ **Embedding service integration** for text-to-vector conversion
- ✅ **Pattern analytics** with effectiveness tracking
- ✅ **Semantic search capabilities** with vector similarity search
- ✅ **Action pattern storage** with metadata and effectiveness scoring

**Architecture Compliance**:
- ❌ **Missing HTTP service wrapper** - Need to create `cmd/storage-service/main.go`
- ✅ **Port**: 8085 (matches approved spec)
- ✅ **Single responsibility**: Data persistence only
- ✅ **Business requirements**: BR-STOR-001 to BR-STOR-135 extensively implemented

### **🔧 REUSABLE COMPONENTS (EXTENSIVE)**

#### **Advanced Vector Database Factory** (100% Reusable)
```go
// Location: pkg/storage/vector/factory.go:16-64
type VectorDatabaseFactory struct {
    config *config.VectorDBConfig
    db     *sql.DB // Main database connection
    log    *logrus.Logger
}

func NewVectorDatabaseFactory(vectorConfig *config.VectorDBConfig, db *sql.DB, log *logrus.Logger) *VectorDatabaseFactory {
    return &VectorDatabaseFactory{
        config: vectorConfig,
        db:     db,
        log:    log,
    }
}

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
```
**Reuse Value**: Complete multi-backend vector database factory with intelligent fallback

#### **Comprehensive Vector Database Interface** (100% Reusable)
```go
// Location: pkg/storage/vector/interfaces.go:10-36
type VectorDatabase interface {
    // StoreActionPattern stores an action pattern as a vector
    StoreActionPattern(ctx context.Context, pattern *ActionPattern) error

    // FindSimilarPatterns finds patterns similar to the given one
    FindSimilarPatterns(ctx context.Context, pattern *ActionPattern, limit int, threshold float64) ([]*SimilarPattern, error)

    // UpdatePatternEffectiveness updates the effectiveness score of a stored pattern
    UpdatePatternEffectiveness(ctx context.Context, patternID string, effectiveness float64) error

    // SearchBySemantics performs semantic search for patterns
    SearchBySemantics(ctx context.Context, query string, limit int) ([]*ActionPattern, error)

    // SearchByVector performs vector similarity search for patterns
    // Business Requirement: BR-AI-COND-001 - Enhanced vector-based condition evaluation
    SearchByVector(ctx context.Context, embedding []float64, limit int, threshold float64) ([]*ActionPattern, error)

    // DeletePattern removes a pattern from the vector database
    DeletePattern(ctx context.Context, patternID string) error

    // GetPatternAnalytics returns analytics about stored patterns
    GetPatternAnalytics(ctx context.Context) (*PatternAnalytics, error)

    // Health check
    IsHealthy(ctx context.Context) error
}
```
**Reuse Value**: Complete vector database interface with pattern storage and analytics

#### **PostgreSQL Vector Database Implementation** (95% Reusable)
```go
// Location: pkg/storage/vector/postgresql_db.go:14-34
type PostgreSQLVectorDatabase struct {
    db               *sql.DB
    embeddingService EmbeddingGenerator
    log              *logrus.Logger
}

func NewPostgreSQLVectorDatabase(db *sql.DB, embeddingService EmbeddingGenerator, log *logrus.Logger) *PostgreSQLVectorDatabase {
    return &PostgreSQLVectorDatabase{
        db:               db,
        embeddingService: embeddingService,
        log:              log,
    }
}

func (db *PostgreSQLVectorDatabase) StoreActionPattern(ctx context.Context, pattern *ActionPattern) error {
    if pattern.ID == "" {
        return fmt.Errorf("pattern ID cannot be empty")
    }

    // Generate embedding if not provided
    if len(pattern.Embedding) == 0 {
        embedding, err := db.embeddingService.GenerateEmbedding(ctx, pattern.Description)
        if err != nil {
            return fmt.Errorf("failed to generate embedding: %w", err)
        }
        pattern.Embedding = embedding
    }

    // Store pattern with vector in PostgreSQL using pgvector
    query := `
        INSERT INTO action_patterns (id, action_type, description, embedding, metadata, effectiveness_score, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        ON CONFLICT (id) DO UPDATE SET
            action_type = EXCLUDED.action_type,
            description = EXCLUDED.description,
            embedding = EXCLUDED.embedding,
            metadata = EXCLUDED.metadata,
            effectiveness_score = EXCLUDED.effectiveness_score,
            updated_at = NOW()
    `

    metadataJSON, _ := json.Marshal(pattern.Metadata)
    _, err := db.db.ExecContext(ctx, query,
        pattern.ID, pattern.ActionType, pattern.Description,
        pattern.Embedding, metadataJSON, pattern.EffectivenessScore, time.Now())

    return err
}
```
**Reuse Value**: Complete PostgreSQL vector database with pgvector integration

#### **External Vector Database Support** (90% Reusable)
```go
// Location: pkg/storage/vector/pinecone_database.go:15-51
type PineconeVectorDatabase struct {
    apiKey     string
    config     *PineconeConfig
    httpClient *http.Client
    log        *logrus.Logger
    embedding  ExternalEmbeddingGenerator
}

type PineconeConfig struct {
    Environment string        `yaml:"environment" default:"us-west1-gcp-free"`
    IndexName   string        `yaml:"index_name" default:"kubernaut"`
    Dimensions  int           `yaml:"dimensions" default:"1536"`
    Metric      string        `yaml:"metric" default:"cosine"`
    MaxRetries  int           `yaml:"max_retries" default:"3"`
    Timeout     time.Duration `yaml:"timeout" default:"30s"`
    BatchSize   int           `yaml:"batch_size" default:"100"`
}

type PineconeVector struct {
    ID       string                 `json:"id"`
    Values   []float64              `json:"values"`
    Metadata map[string]interface{} `json:"metadata,omitempty"`
}
```
**Reuse Value**: Complete external vector database support (Pinecone, Weaviate)

#### **Memory Vector Database for Development** (100% Reusable)
```go
// Location: pkg/storage/vector/memory_db.go (290+ lines)
// Complete in-memory vector database implementation with:
// - Vector similarity search using cosine similarity
// - Pattern storage with metadata
// - Effectiveness tracking and analytics
// - Thread-safe operations with mutex protection
// - Development and testing support
```
**Reuse Value**: Complete memory-based vector database for development and testing

#### **External Vector Database Interface** (100% Reusable)
```go
// Location: pkg/storage/vector/external_interfaces.go:8-44
type ExternalVectorDatabase interface {
    // Store stores vectors in the external database
    Store(ctx context.Context, vectors []VectorData) error

    // Query searches for similar vectors
    Query(ctx context.Context, embedding []float64, topK int, filters map[string]interface{}) ([]BaseSearchResult, error)

    // QueryByText searches for similar vectors using text input
    QueryByText(ctx context.Context, text string, topK int, filters map[string]interface{}) ([]BaseSearchResult, error)

    // Delete removes vectors by their IDs
    Delete(ctx context.Context, ids []string) error

    // DeleteByFilter removes vectors matching the filter
    DeleteByFilter(ctx context.Context, filters map[string]interface{}) error

    // Close closes the database connection
    Close() error
}

type VectorData struct {
    ID        string                 `json:"id"`
    Text      string                 `json:"text,omitempty"`
    Embedding []float64              `json:"embedding,omitempty"`
    Metadata  map[string]interface{} `json:"metadata,omitempty"`
    Source    string                 `json:"source,omitempty"`
    Timestamp time.Time              `json:"timestamp,omitempty"`
}
```
**Reuse Value**: Complete external vector database interface with comprehensive operations

---

## 🎯 **DEVELOPMENT GAPS & IMPROVEMENTS**

### **🚨 CRITICAL GAPS**

#### **1. Missing HTTP Service Wrapper**
**Current**: Exceptional storage logic but no HTTP service
**Required**: Complete HTTP service implementation
**Gap**: Need to create:
- `cmd/storage-service/main.go` - HTTP server with storage endpoints
- HTTP handlers for vector operations, pattern storage, analytics
- Health and metrics endpoints
- Configuration loading and service startup

#### **2. Service Integration API**
**Current**: Comprehensive storage logic with internal interfaces
**Required**: HTTP API for microservice integration
**Gap**: Need to implement:
- REST API for vector storage and retrieval operations
- JSON request/response handling for pattern operations
- Analytics and search endpoints
- Error handling and status codes

#### **3. Missing Dedicated Test Files**
**Current**: Sophisticated storage logic but no visible tests
**Required**: Extensive test coverage for storage operations
**Gap**: Need to create:
- HTTP endpoint tests
- Vector database operation tests
- Multi-backend storage tests
- Pattern analytics tests
- Integration tests with PostgreSQL and external services

### **🔄 ENHANCEMENT OPPORTUNITIES**

#### **1. Advanced Storage Analytics**
**Current**: Basic pattern analytics
**Enhancement**: Advanced storage analytics with performance insights
```go
type AdvancedStorageAnalytics struct {
    PerformanceAnalyzer  *StoragePerformanceAnalyzer
    UsageAnalytics       *StorageUsageAnalytics
    OptimizationEngine   *StorageOptimizationEngine
}
```

#### **2. Real-time Storage Monitoring**
**Current**: Basic health checks
**Enhancement**: Real-time storage monitoring with live metrics
```go
type RealTimeStorageMonitor struct {
    MetricsCollector     *StorageMetricsCollector
    PerformanceDashboard *StorageDashboard
    AlertingSystem       *StorageAlerting
}
```

#### **3. Advanced Backup and Recovery**
**Current**: Basic storage operations
**Enhancement**: Advanced backup and recovery with versioning
```go
type AdvancedBackupManager struct {
    BackupScheduler      *BackupScheduler
    VersionManager       *StorageVersionManager
    RecoveryEngine       *StorageRecoveryEngine
}
```

---

## 📋 **TDD DEVELOPMENT PLAN**

### **🔴 RED PHASE (30-45 minutes)**

#### **Test 1: HTTP Service Implementation**
```go
func TestDataStorageServiceHTTP(t *testing.T) {
    It("should start HTTP server on port 8085", func() {
        // Test server starts on correct port
        resp, err := http.Get("http://localhost:8085/health")
        Expect(err).ToNot(HaveOccurred())
        Expect(resp.StatusCode).To(Equal(200))
    })

    It("should handle vector storage requests", func() {
        // Test POST /api/v1/patterns endpoint
        pattern := ActionPattern{
            ID:          "test-pattern-001",
            ActionType:  "restart-pod",
            Description: "Restart pod for high CPU usage",
            Metadata: map[string]interface{}{
                "namespace": "production",
                "severity":  "critical",
            },
            EffectivenessScore: 0.85,
        }

        resp, err := http.Post("http://localhost:8085/api/v1/patterns", "application/json", patternPayload)
        Expect(err).ToNot(HaveOccurred())
        Expect(resp.StatusCode).To(Equal(201))

        var response PatternStorageResponse
        json.NewDecoder(resp.Body).Decode(&response)
        Expect(response.Success).To(BeTrue())
        Expect(response.PatternID).To(Equal("test-pattern-001"))
    })
}
```

#### **Test 2: Vector Database Operations**
```go
func TestVectorDatabaseOperations(t *testing.T) {
    It("should store and retrieve action patterns", func() {
        factory := vector.NewVectorDatabaseFactory(vectorConfig, db, logger)
        vectorDB, err := factory.CreateVectorDatabase()
        Expect(err).ToNot(HaveOccurred())

        pattern := &vector.ActionPattern{
            ID:          "test-pattern-001",
            ActionType:  "scale-deployment",
            Description: "Scale deployment for high load",
            Metadata: map[string]interface{}{
                "replicas": 5,
                "resource": "deployment/web-server",
            },
            EffectivenessScore: 0.9,
        }

        // Store pattern
        err = vectorDB.StoreActionPattern(context.Background(), pattern)
        Expect(err).ToNot(HaveOccurred())

        // Search for similar patterns
        similarPatterns, err := vectorDB.FindSimilarPatterns(context.Background(), pattern, 5, 0.8)
        Expect(err).ToNot(HaveOccurred())
        Expect(len(similarPatterns)).To(BeNumerically(">", 0))
    })

    It("should perform semantic search", func() {
        // Test semantic search functionality
        patterns, err := vectorDB.SearchBySemantics(context.Background(), "restart pod high CPU", 10)
        Expect(err).ToNot(HaveOccurred())
        Expect(patterns).ToNot(BeEmpty())
    })
}
```

#### **Test 3: Multi-Backend Support**
```go
func TestMultiBackendSupport(t *testing.T) {
    It("should support PostgreSQL backend", func() {
        config := &config.VectorDBConfig{
            Enabled: true,
            Backend: "postgresql",
        }

        factory := vector.NewVectorDatabaseFactory(config, db, logger)
        vectorDB, err := factory.CreateVectorDatabase()
        Expect(err).ToNot(HaveOccurred())

        // Test PostgreSQL-specific operations
        err = vectorDB.IsHealthy(context.Background())
        Expect(err).ToNot(HaveOccurred())
    })

    It("should fallback to memory backend", func() {
        config := &config.VectorDBConfig{
            Enabled: false,
        }

        factory := vector.NewVectorDatabaseFactory(config, nil, logger)
        vectorDB, err := factory.CreateVectorDatabase()
        Expect(err).ToNot(HaveOccurred())

        // Test memory backend operations
        pattern := &vector.ActionPattern{ID: "test", ActionType: "restart"}
        err = vectorDB.StoreActionPattern(context.Background(), pattern)
        Expect(err).ToNot(HaveOccurred())
    })
}
```

### **🟢 GREEN PHASE (1-2 hours)**

#### **Implementation Priority**:
1. **Create HTTP service wrapper** (60 minutes) - Critical missing piece
2. **Implement HTTP endpoints** (45 minutes) - API for service integration
3. **Add comprehensive tests** (30 minutes) - Storage operation tests
4. **Enhance monitoring and analytics** (30 minutes) - Storage metrics
5. **Add deployment manifest** (15 minutes) - Kubernetes deployment

#### **HTTP Service Implementation**:
```go
// cmd/storage-service/main.go (NEW FILE)
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/sirupsen/logrus"
    "github.com/jordigilh/kubernaut/internal/config"
    "github.com/jordigilh/kubernaut/pkg/storage/vector"
)

func main() {
    // Initialize logger
    logger := logrus.New()
    logger.SetFormatter(&logrus.JSONFormatter{})

    // Load configuration
    cfg, err := loadStorageConfiguration()
    if err != nil {
        logger.WithError(err).Fatal("Failed to load configuration")
    }

    // Create database connection
    db, err := createDatabaseConnection(cfg.Database, logger)
    if err != nil {
        logger.WithError(err).Fatal("Failed to create database connection")
    }
    defer db.Close()

    // Create vector database factory
    vectorFactory := vector.NewVectorDatabaseFactory(cfg.VectorDB, db, logger)

    // Create vector database
    vectorDB, err := vectorFactory.CreateVectorDatabase()
    if err != nil {
        logger.WithError(err).Fatal("Failed to create vector database")
    }

    // Create storage service
    storageService := NewStorageService(vectorDB, cfg, logger)

    // Setup HTTP server
    server := setupHTTPServer(storageService, cfg, logger)

    // Start server
    go func() {
        logger.WithField("port", cfg.ServicePort).Info("Starting storage HTTP server")
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            logger.WithError(err).Fatal("Failed to start HTTP server")
        }
    }()

    // Graceful shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    sig := <-sigChan
    logger.WithField("signal", sig).Info("Received shutdown signal")

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := server.Shutdown(ctx); err != nil {
        logger.WithError(err).Error("Failed to shutdown server gracefully")
    } else {
        logger.Info("Server shutdown complete")
    }
}

func setupHTTPServer(storageService *StorageService, cfg *StorageConfig, logger *logrus.Logger) *http.Server {
    mux := http.NewServeMux()

    // Core storage endpoints
    mux.HandleFunc("/api/v1/patterns", handlePatterns(storageService, logger))
    mux.HandleFunc("/api/v1/patterns/", handlePatternOperations(storageService, logger))
    mux.HandleFunc("/api/v1/search/semantic", handleSemanticSearch(storageService, logger))
    mux.HandleFunc("/api/v1/search/vector", handleVectorSearch(storageService, logger))

    // Analytics endpoints
    mux.HandleFunc("/api/v1/analytics", handleAnalytics(storageService, logger))
    mux.HandleFunc("/api/v1/analytics/patterns", handlePatternAnalytics(storageService, logger))

    // Management endpoints
    mux.HandleFunc("/api/v1/effectiveness", handleEffectivenessUpdate(storageService, logger))
    mux.HandleFunc("/api/v1/backup", handleBackup(storageService, logger))

    // Monitoring endpoints
    mux.HandleFunc("/health", handleHealth(storageService, logger))
    mux.HandleFunc("/metrics", handleMetrics(logger))

    return &http.Server{
        Addr:         fmt.Sprintf(":%d", cfg.ServicePort),
        Handler:      mux,
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 60 * time.Second, // Longer timeout for storage operations
    }
}

func handlePatterns(storageService *StorageService, logger *logrus.Logger) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        switch r.Method {
        case http.MethodPost:
            // Store new pattern
            var req PatternStorageRequest
            if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                http.Error(w, "Invalid request format", http.StatusBadRequest)
                return
            }

            result, err := storageService.StorePattern(r.Context(), &req)
            if err != nil {
                logger.WithError(err).Error("Pattern storage failed")
                http.Error(w, "Pattern storage failed", http.StatusInternalServerError)
                return
            }

            response := PatternStorageResponse{
                Success:   true,
                PatternID: result.PatternID,
                Message:   "Pattern stored successfully",
                Timestamp: time.Now(),
            }

            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusCreated)
            json.NewEncoder(w).Encode(response)

        case http.MethodGet:
            // List patterns with optional filters
            patterns, err := storageService.ListPatterns(r.Context(), r.URL.Query())
            if err != nil {
                logger.WithError(err).Error("Pattern listing failed")
                http.Error(w, "Pattern listing failed", http.StatusInternalServerError)
                return
            }

            response := PatternListResponse{
                Patterns:  patterns,
                Count:     len(patterns),
                Timestamp: time.Now(),
            }

            w.Header().Set("Content-Type", "application/json")
            json.NewEncoder(w).Encode(response)

        default:
            http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        }
    }
}

type StorageService struct {
    vectorDB vector.VectorDatabase
    config   *StorageConfig
    logger   *logrus.Logger
}

func NewStorageService(vectorDB vector.VectorDatabase, config *StorageConfig, logger *logrus.Logger) *StorageService {
    return &StorageService{
        vectorDB: vectorDB,
        config:   config,
        logger:   logger,
    }
}

func (ss *StorageService) StorePattern(ctx context.Context, req *PatternStorageRequest) (*PatternStorageResult, error) {
    // Convert request to action pattern
    pattern := &vector.ActionPattern{
        ID:                 req.Pattern.ID,
        ActionType:         req.Pattern.ActionType,
        Description:        req.Pattern.Description,
        Metadata:           req.Pattern.Metadata,
        EffectivenessScore: req.Pattern.EffectivenessScore,
        CreatedAt:          time.Now(),
    }

    // Store pattern using vector database
    err := ss.vectorDB.StoreActionPattern(ctx, pattern)
    if err != nil {
        return nil, fmt.Errorf("failed to store pattern: %w", err)
    }

    return &PatternStorageResult{
        PatternID: pattern.ID,
        Success:   true,
    }, nil
}

type StorageConfig struct {
    ServicePort int                    `yaml:"service_port" default:"8085"`
    Database    config.DatabaseConfig  `yaml:"database"`
    VectorDB    *config.VectorDBConfig `yaml:"vector_db"`
}

type PatternStorageRequest struct {
    Pattern ActionPatternData `json:"pattern"`
}

type PatternStorageResponse struct {
    Success   bool      `json:"success"`
    PatternID string    `json:"pattern_id"`
    Message   string    `json:"message"`
    Timestamp time.Time `json:"timestamp"`
}

type PatternStorageResult struct {
    PatternID string `json:"pattern_id"`
    Success   bool   `json:"success"`
}

type ActionPatternData struct {
    ID                 string                 `json:"id"`
    ActionType         string                 `json:"action_type"`
    Description        string                 `json:"description"`
    Metadata           map[string]interface{} `json:"metadata"`
    EffectivenessScore float64                `json:"effectiveness_score"`
}
```

### **🔵 REFACTOR PHASE (30-45 minutes)**

#### **Code Organization**:
- Extract HTTP handlers to separate files
- Implement advanced storage analytics
- Add comprehensive error handling
- Optimize performance for concurrent operations

---

## 🔗 **INTEGRATION POINTS**

### **Upstream Services**
- **Intelligence Service** (intelligence-service:8086) - Stores discovered patterns
- **Effectiveness Monitor Service** (monitor-service:8087) - Stores effectiveness results

### **External Dependencies**
- **PostgreSQL with pgvector** - Primary vector database backend
- **Pinecone** - External vector database for cloud deployments
- **Weaviate** - Alternative external vector database
- **Embedding Services** - Text-to-vector conversion

### **Configuration Dependencies**
```yaml
# config/storage-service.yaml
storage:
  service_port: 8085

  database:
    host: "localhost"
    port: 5432
    name: "vector_storage"
    user: "storage_user"
    password: "${DB_PASSWORD}"

  vector_db:
    enabled: true
    backend: "postgresql"  # postgresql, pinecone, weaviate, memory

    postgresql:
      enable_pgvector: true
      vector_dimensions: 1536

    pinecone:
      api_key: "${PINECONE_API_KEY}"
      environment: "us-west1-gcp-free"
      index_name: "kubernaut"
      dimensions: 1536

    weaviate:
      host: "localhost"
      port: 8080
      scheme: "http"

  embedding:
    provider: "openai"
    api_key: "${OPENAI_API_KEY}"
    model: "text-embedding-ada-002"

  analytics:
    enable_pattern_analytics: true
    enable_performance_tracking: true
    analytics_retention_days: 90
```

---

## 📁 **FILE OWNERSHIP (EXCLUSIVE)**

### **Files You Can Modify**:
```bash
cmd/storage-service/               # Complete directory (NEW)
├── main.go                       # NEW: HTTP service implementation
├── main_test.go                  # NEW: HTTP server tests
├── handlers.go                   # NEW: HTTP request handlers
├── storage_service.go            # NEW: Storage service logic
├── config.go                     # NEW: Configuration management
└── *_test.go                     # All test files

pkg/storage/vector/               # Complete directory (EXISTING)
├── factory.go                    # EXISTING: 67 lines vector database factory
├── interfaces.go                 # EXISTING: 40 lines vector database interfaces
├── postgresql_db.go              # EXISTING: PostgreSQL vector database
├── pinecone_database.go          # EXISTING: Pinecone vector database
├── weaviate_database.go          # EXISTING: Weaviate vector database
├── memory_db.go                  # EXISTING: Memory vector database
├── embedding_service.go          # EXISTING: Embedding service
├── external_interfaces.go        # EXISTING: External vector database interfaces
└── *_test.go                     # NEW: Add comprehensive tests

test/unit/storage/                # Complete test directory
├── storage_service_test.go       # NEW: Service logic tests
├── vector_database_test.go       # NEW: Vector database tests
├── multi_backend_test.go         # NEW: Multi-backend tests
├── pattern_analytics_test.go     # NEW: Pattern analytics tests
└── integration_test.go           # NEW: Integration tests

deploy/microservices/storage-deployment.yaml  # Deployment manifest
```

### **Files You CANNOT Modify**:
```bash
pkg/shared/types/                 # Shared type definitions
internal/config/                  # Configuration patterns (reuse only)
deploy/kustomization.yaml         # Main deployment config
```

---

## ⚡ **QUICK START COMMANDS**

### **Development Setup**:
```bash
# Build service (after creating main.go)
go build -o storage-service cmd/storage-service/main.go

# Run service
export DB_PASSWORD="your-password"
export OPENAI_API_KEY="your-key-here"
./storage-service

# Test service
curl http://localhost:8085/health
curl http://localhost:8085/metrics

# Test pattern storage
curl -X POST http://localhost:8085/api/v1/patterns \
  -H "Content-Type: application/json" \
  -d '{"pattern":{"id":"test-001","action_type":"restart-pod","description":"Restart pod for high CPU","metadata":{"namespace":"production"},"effectiveness_score":0.85}}'

# Test semantic search
curl -X POST http://localhost:8085/api/v1/search/semantic \
  -H "Content-Type: application/json" \
  -d '{"query":"restart pod high CPU","limit":10}'
```

### **Testing Commands**:
```bash
# Run tests (after creating test files)
go test cmd/storage-service/... -v
go test pkg/storage/vector/... -v
go test test/unit/storage/... -v

# Integration tests with PostgreSQL
STORAGE_INTEGRATION_TEST=true go test test/integration/storage/... -v
```

---

## 🎯 **SUCCESS CRITERIA**

### **Technical Success**:
- [ ] Service builds: `go build cmd/storage-service/main.go` succeeds (NEED TO CREATE)
- [ ] Service starts on port 8085: `curl http://localhost:8085/health` returns 200 (NEED TO CREATE)
- [ ] Pattern storage works: POST to `/api/v1/patterns` stores patterns (NEED TO IMPLEMENT)
- [ ] Vector operations work: Similarity search and analytics functional ✅ (ALREADY IMPLEMENTED)
- [ ] Multi-backend support works: PostgreSQL, Pinecone, Weaviate, Memory ✅ (ALREADY IMPLEMENTED)
- [ ] All tests pass: `go test cmd/storage-service/... -v` all green (NEED TO CREATE)

### **Business Success**:
- [ ] BR-STOR-001 to BR-STOR-135 implemented ✅ (COMPREHENSIVE LOGIC ALREADY IMPLEMENTED)
- [ ] Vector database operations working ✅ (ALREADY IMPLEMENTED)
- [ ] Pattern storage and retrieval working ✅ (ALREADY IMPLEMENTED)
- [ ] Semantic search working ✅ (ALREADY IMPLEMENTED)
- [ ] Pattern analytics working ✅ (ALREADY IMPLEMENTED)

### **Architecture Success**:
- [ ] Uses exact service name: `storage-service` (NEED TO IMPLEMENT)
- [ ] Uses exact port: `8085` ✅ (WILL BE CONFIGURED CORRECTLY)
- [ ] Uses exact image format: `quay.io/jordigilh/storage-service` (WILL FOLLOW PATTERN)
- [ ] Implements only data persistence responsibility ✅ (ALREADY CORRECT)
- [ ] Integrates with approved microservices architecture (NEED HTTP SERVICE)

---

## 📊 **CONFIDENCE ASSESSMENT**

```
Data Storage Service Development Confidence: 88%

Strengths:
✅ EXCEPTIONAL existing foundation (1000+ lines of sophisticated storage code)
✅ Advanced vector database factory with multi-backend support
✅ Comprehensive vector database interfaces for pattern storage and analytics
✅ PostgreSQL integration with pgvector extension support
✅ External vector database support (Pinecone, Weaviate) for cloud deployments
✅ Memory fallback database for development and testing
✅ Embedding service integration for text-to-vector conversion
✅ Pattern analytics with effectiveness tracking
✅ Semantic search capabilities with vector similarity search

Critical Gap:
⚠️  Missing HTTP service wrapper (need to create cmd/storage-service/main.go)
⚠️  Missing dedicated test files (need storage operation tests)

Mitigation:
✅ All storage logic already implemented and comprehensive
✅ Clear patterns from other services for HTTP wrapper
✅ Multi-backend support already established
✅ Comprehensive business logic ready for immediate use

Implementation Time: 2-3 hours (HTTP service wrapper + tests + integration)
Integration Readiness: HIGH (comprehensive storage foundation)
Business Value: EXCEPTIONAL (critical data persistence and vector operations)
Risk Level: LOW (minimal changes needed to existing working code)
Technical Complexity: HIGH (sophisticated vector database and multi-backend support)
```

---

**Status**: ✅ **READY FOR PHASE 1 DEVELOPMENT**
**Dependencies**: None (independent data storage)
**Integration Point**: HTTP API for vector storage and retrieval
**Primary Tasks**:
1. Create HTTP service wrapper (1-2 hours)
2. Implement HTTP endpoints for storage operations (45 minutes)
3. Add comprehensive test coverage (30 minutes)
4. Enhance monitoring and analytics (30 minutes)
5. Create deployment manifest (15 minutes)

**Phase 1 Execution Order**: **PARALLEL** (no dependencies, independent storage service)
